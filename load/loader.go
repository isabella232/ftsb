package load

import (
	"bufio"
	"flag"
	"fmt"
	"log"
	"math"
	"os"
	"sync"
	"sync/atomic"
	"time"
)

const (
	// defaultBatchSize - default size of batches to be inserted
	defaultBatchSize = 10000
	defaultReadSize  = 4 << 20 // 4 MB

	// WorkerPerQueue is the value for assigning each worker its own queue of batches
	WorkerPerQueue = 0
	// SingleQueue is the value for using a single shared queue across all workers
	SingleQueue = 1

	errDBExistsFmt = "database \"%s\" exists: aborting."
)

// change for more useful testing
var (
	printFn = fmt.Printf
	fatal   = log.Fatalf
)

// Benchmark is an interface that represents the skeleton of a program
// needed to run an insert or load benchmark.
type Benchmark interface {
	// GetPointDecoder returns the PointDecoder to use for this Benchmark
	GetPointDecoder(br *bufio.Reader) PointDecoder

	// GetBatchFactory returns the BatchFactory to use for this Benchmark
	GetBatchFactory() BatchFactory

	// GetPointIndexer returns the PointIndexer to use for this Benchmark
	GetPointIndexer(maxPartitions uint) PointIndexer

	// GetProcessor returns the Processor to use for this Benchmark
	GetProcessor() Processor

	// GetDBCreator returns the DBCreator to use for this Benchmark
	GetDBCreator() DBCreator
}

// BenchmarkRunner is responsible for initializing and storing common
// flags across all database systems and ultimately running a supplied Benchmark
type BenchmarkRunner struct {
	// flag fields
	dbName          string
	batchSize       uint
	workers         uint
	limit           uint64
	doLoad          bool
	doCreateDB      bool
	doAbortOnExist  bool
	reportingPeriod time.Duration
	fileName        string
	insertRate      float64
	updateRate      float64
	deleteRate      float64

	// non-flag fields
	br          *bufio.Reader
	insertCount uint64
	updateCount uint64
	deleteCount uint64
	rowCnt      uint64
}

func (l *BenchmarkRunner) InsertRate() float64 {
	return l.insertRate
}

func (l *BenchmarkRunner) DeleteRate() float64 {
	return l.deleteRate
}

func (l *BenchmarkRunner) UpdateRate() float64 {
	return l.updateRate
}

var loader = &BenchmarkRunner{}

// GetBenchmarkRunner returns the singleton BenchmarkRunner for use in a benchmark program
// with a default batch size
func GetBenchmarkRunner() *BenchmarkRunner {
	return GetBenchmarkRunnerWithBatchSize(defaultBatchSize)
}

// GetBenchmarkRunnerWithBatchSize returns the singleton BenchmarkRunner for use in a benchmark program
// with specified batch size.
func GetBenchmarkRunnerWithBatchSize(batchSize uint) *BenchmarkRunner {
	// fill flag fields of BenchmarkRunner struct
	flag.StringVar(&loader.dbName, "index", "idx1", "Name of index")
	flag.UintVar(&loader.batchSize, "batch-size", batchSize, "Number of items to batch together in a single insert")
	flag.UintVar(&loader.workers, "workers", 8, "Number of parallel clients inserting")
	flag.Uint64Var(&loader.limit, "limit", 0, "Number of items to insert (0 = all of them).")
	flag.BoolVar(&loader.doLoad, "do-load", true, "Whether to write data. Set this flag to false to check input read speed.")
	flag.BoolVar(&loader.doCreateDB, "do-create-db", true, "Whether to create the database. Disable on all but one client if running on a multi client setup.")
	flag.BoolVar(&loader.doAbortOnExist, "do-abort-on-exist", false, "Whether to abort if a database with the given name already exists.")
	flag.DurationVar(&loader.reportingPeriod, "reporting-period", 1*time.Second, "Period to report write stats")
	flag.StringVar(&loader.fileName, "file", "", "File name to read data from")
	flag.Float64Var(&loader.updateRate, "update-rate", 0, "Set the update rate ( between 0-1 ) for Documents being ingested")
	flag.Float64Var(&loader.deleteRate, "delete-rate", 0, "Set the delete rate ( between 0-1 ) for Documents being ingested")
	return loader
}

// DatabaseName returns the value of the --db-name flag (name of the database to store data)
func (l *BenchmarkRunner) DatabaseName() string {
	return l.dbName
}

// RunBenchmark takes in a Benchmark b, a bufio.Reader br, and holders for number of metrics and rows
// and uses those to run the load benchmark
func (l *BenchmarkRunner) RunBenchmark(b Benchmark, workQueues uint) {
	l.br = l.GetBufferedReader()

	// Create required DB
	cleanupFn := l.useDBCreator(b.GetDBCreator())
	defer cleanupFn()

	channels := l.createChannels(workQueues)
	l.insertRate = 1.0 - l.updateRate - l.deleteRate
	// Launch all worker processes in background
	var wg sync.WaitGroup
	for i := 0; i < int(l.workers); i++ {
		wg.Add(1)
		go l.work(b, &wg, channels[i%len(channels)], i)
	}

	// Start scan process - actual data read process
	start := time.Now()
	l.scan(b, channels)

	// After scan process completed (no more data to come) - begin shutdown process

	// Close all communication channels to/from workers
	for _, c := range channels {
		c.close()
	}

	// Wait for all workers to finish
	wg.Wait()
	end := time.Now()

	l.summary(end.Sub(start))
}

// GetBufferedReader returns the buffered Reader that should be used by the loader
func (l *BenchmarkRunner) GetBufferedReader() *bufio.Reader {
	if l.br == nil {
		if len(l.fileName) > 0 {
			// Read from specified file
			file, err := os.Open(l.fileName)
			if err != nil {
				fatal("cannot open file for read %s: %v", l.fileName, err)
				return nil
			}
			l.br = bufio.NewReaderSize(file, defaultReadSize)
		} else {
			// Read from STDIN
			l.br = bufio.NewReaderSize(os.Stdin, defaultReadSize)
		}
	}
	return l.br
}

// useDBCreator handles a DBCreator by running it according to flags set by the
// user. The function returns a function that the caller should defer or run
// when the benchmark is finished
func (l *BenchmarkRunner) useDBCreator(dbc DBCreator) func() {
	// Empty function to 'defer' from caller
	closeFn := func() {}

	if l.doLoad {
		// DBCreator should still be Init'd even if -do-create-db is false since
		// it can initialize the connecting session
		dbc.Init()

		switch dbcc := dbc.(type) {
		case DBCreatorCloser:
			closeFn = dbcc.Close
		}

		// Check whether required DB already exists
		exists := dbc.DBExists(l.dbName)
		if exists && l.doAbortOnExist {
			panic(fmt.Sprintf(errDBExistsFmt, l.dbName))
		}

		// Create required DB if need be
		// In case DB already exists - delete it
		if l.doCreateDB {
			if exists {
				err := dbc.RemoveOldDB(l.dbName)
				if err != nil {
					panic(err)
				}
			}
			err := dbc.CreateDB(l.dbName)
			if err != nil {
				panic(err)
			}
		}

		switch dbcp := dbc.(type) {
		case DBCreatorPost:
			dbcp.PostCreateDB(l.dbName)
		}
	}
	return closeFn
}

// createChannels create channels from which workers would receive tasks
// Number of workers may be different from number of channels, thus we may have
// multiple workers per channel
func (l *BenchmarkRunner) createChannels(workQueues uint) []*duplexChannel {
	// Result - channels to be created
	channels := []*duplexChannel{}

	// How many work queues should be created?
	workQueuesToCreate := workQueues
	if workQueues == WorkerPerQueue {
		workQueuesToCreate = l.workers
	} else if workQueues > l.workers {
		panic(fmt.Sprintf("cannot have more work queues (%d) than workers (%d)", workQueues, l.workers))
	}

	// How many workers would be served by each queue?
	workersPerQueue := int(math.Ceil(float64(l.workers) / float64(workQueuesToCreate)))

	// Create duplex communication channels
	for i := uint(0); i < workQueuesToCreate; i++ {
		channels = append(channels, newDuplexChannel(workersPerQueue))
	}

	return channels
}

// scan launches any needed reporting mechanism and proceeds to scan input data
// to distribute to workers
func (l *BenchmarkRunner) scan(b Benchmark, channels []*duplexChannel) uint64 {
	// Start background reporting process
	// TODO why it is here? May be it could be moved one level up?
	if l.reportingPeriod.Nanoseconds() > 0 {
		go l.report(l.reportingPeriod)
	}

	// Scan incoming data
	return scanWithIndexer(channels, l.batchSize, l.limit, l.br, b.GetPointDecoder(l.br), b.GetBatchFactory(), b.GetPointIndexer(uint(len(channels))))
}

// work is the processing function for each worker in the loader
func (l *BenchmarkRunner) work(b Benchmark, wg *sync.WaitGroup, c *duplexChannel, workerNum int) {

	// Prepare processor
	proc := b.GetProcessor()
	proc.Init(workerNum, l.doLoad)

	// Process batches coming from duplexChannel.toWorker queue
	// and send ACKs into duplexChannel.toScanner queue
	for b := range c.toWorker {
		metricCnt, rowCnt, updateCount, deleteCount := proc.ProcessBatch(b, l.doLoad, l.updateRate, l.deleteRate)
		atomic.AddUint64(&l.insertCount, metricCnt)
		atomic.AddUint64(&l.updateCount, updateCount)
		atomic.AddUint64(&l.deleteCount, deleteCount)
		atomic.AddUint64(&l.rowCnt, rowCnt)
		c.sendToScanner()
	}

	// Close proc if necessary
	switch c := proc.(type) {
	case ProcessorCloser:
		c.Close(l.doLoad)
	}

	wg.Done()
}

// summary prints the summary of statistics from loading
func (l *BenchmarkRunner) summary(took time.Duration) {

	insertCount := atomic.LoadUint64(&l.insertCount)
	updateCount := atomic.LoadUint64(&l.updateCount)
	deleteCount := atomic.LoadUint64(&l.deleteCount)
	metricRate := float64(insertCount+updateCount+deleteCount) / float64(took.Seconds())

	printFn("\nSummary:\n")
	printFn("loaded %d Documents in %0.3fsec with %d workers (mean rate %0.2f docs/sec)\n", l.insertCount, took.Seconds(), l.workers, metricRate)
}

// report handles periodic reporting of loading stats
func (l *BenchmarkRunner) report(period time.Duration) {
	start := time.Now()
	prevTime := start
	prevInsertCount := uint64(0)
	prevUpdateCount := uint64(0)
	prevDeleteCount := uint64(0)

	printFn("%-12s , %-12s , %-12s , %-12s , %-15s , %-15s , %-15s\n", "time", "inserts/sec", "updates/sec", "deletes/sec", "current ops/sec", "docs total", "overall ops/sec")
	for now := range time.NewTicker(period).C {
		insertCount := atomic.LoadUint64(&l.insertCount)
		updateCount := atomic.LoadUint64(&l.updateCount)
		deleteCount := atomic.LoadUint64(&l.deleteCount)

		sinceStart := now.Sub(start)
		took := now.Sub(prevTime)
		insertRate := float64(insertCount-prevInsertCount) / float64(took.Seconds())
		updateRate := float64(updateCount-prevUpdateCount) / float64(took.Seconds())
		deleteRate := float64(deleteCount-prevDeleteCount) / float64(took.Seconds())
		insertUpdateDeleteRate := insertRate + updateRate + deleteRate
		overallOpsRate := float64(insertCount+updateCount+deleteCount) / float64(sinceStart.Seconds())

		printFn("%-12d , %-12.1f , %-12.1f , %-12.1f , %-15.1f , %-15d , %-15.1f\n", now.Unix(), insertRate, updateRate, deleteRate, insertUpdateDeleteRate, insertCount, overallOpsRate)

		prevInsertCount = insertCount
		prevUpdateCount = updateCount
		prevDeleteCount = deleteCount
		prevTime = now
	}
}
