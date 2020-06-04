package main

import (
	"fmt"
	"github.com/RediSearch/ftsb/load"
	"github.com/mediocregopher/radix"
	"log"
	"strings"
	"sync"
	"time"
)

type processor struct {
	dbc            *dbCreator
	rows           chan string
	cmdChan        chan load.Stat
	wg             *sync.WaitGroup
	vanillaClient  *radix.Pool
	vanillaCluster *radix.Cluster
}

func (p *processor) Init(workerNumber int, _ bool, totalWorkers int) {
	var err error = nil
	if clusterMode {
		poolFunc := func(network, addr string) (radix.Client, error) {
			return radix.NewPool(network, addr, 1, radix.PoolPipelineWindow(time.Duration(PoolPipelineWindow*float64(time.Millisecond)), PoolPipelineConcurrency))
		}
		p.vanillaCluster, err = radix.NewCluster([]string{host}, radix.ClusterPoolFunc(poolFunc))
		if err != nil {
			log.Fatalf("Error preparing for redisearch ingestion, while creating new cluster connection. error = %v", err)

		}
	} else {

		p.vanillaClient, err = radix.NewPool("tcp", host, 1, radix.PoolPipelineWindow(time.Duration(PoolPipelineWindow*float64(time.Millisecond)), PoolPipelineConcurrency))
		if err != nil {
			log.Fatalf("Error preparing for redisearch ingestion, while creating new pool. error = %v", err)

		}

	}

}

// using random between [0,1) to determine whether it is an delete,update, or insert
// DELETE IF BETWEEN [0,deleteLimit)
// UPDATE IF BETWEEN [deleteLimit,updateLimit)
// INSERT IF BETWEEN [updateLimit,1)
func connectionProcessor(p *processor) {
	for row := range p.rows {
		cmdType, cmd, docFields, bytelen, _ := rowToHash(row)
		sendFlatCmd(p, cmdType, cmd, docFields, bytelen, 1)
	}

	p.wg.Done()
}

func getRxLen(v interface{}) (res uint64) {
	res = 0
	switch x := v.(type) {
	case []string:
		for _, i := range x {
			res += uint64(len(i))
		}
	case string:
		res += uint64(len(x))
	default:
		res = 0
	}
	return
}

func sendFlatCmd(p *processor, cmdType, cmd string, docfields []string, txBytesCount uint64, insertCount uint64) {
	var err error = nil
	var rcv interface{}
	rxBytesCount := uint64(0)
	took := uint64(0)
	start := time.Now()
	if clusterMode {
		err = p.vanillaCluster.Do(radix.FlatCmd(&rcv, cmd, docfields[0], docfields[1:]))
	} else {
		err = p.vanillaClient.Do(radix.FlatCmd(&rcv, cmd, docfields[0], docfields[1:]))
	}
	if err != nil {
		issuedCommand := fmt.Sprintf("%s %s %s", cmd, docfields[0], strings.Join(docfields[1:], " "))
		extendedError := fmt.Errorf("%s failed:%v\nIssued command: %s", cmd, err, issuedCommand)
		log.Fatal(extendedError)
	}
	took += uint64(time.Since(start).Microseconds())
	rxBytesCount += getRxLen(rcv)
	stat := load.NewStat().AddEntry([]byte(cmdType), took, false, false, txBytesCount, rxBytesCount)

	if cmd == "FT.AGGREGATE" && rcv != nil {
		var aggreply []interface{}
		aggreply = rcv.([]interface{})
		cursor_id := aggreply[1].(int64)
		cursor_cmds := uint64(0)
		for cursor_id != 0 {
			start := time.Now()
			if clusterMode {
				err = p.vanillaCluster.Do(radix.FlatCmd(&aggreply, "FT.CURSOR", "READ", docfields[0], cursor_id))
			} else {
				err = p.vanillaClient.Do(radix.FlatCmd(&aggreply, "FT.CURSOR", "READ", docfields[0], cursor_id))
			}
			if err != nil {
				issuedCommand := fmt.Sprintf("FT.CURSOR READ %s %d", docfields[0], cursor_id)
				extendedError := fmt.Errorf("%s failed:%v\nIssued command: %s", "FT.CURSOR", err, issuedCommand)
				log.Fatal(extendedError)
			}
			took += uint64(time.Since(start).Microseconds())
			rxBytesCount += getRxLen(rcv)
			stat.AddCmdStatEntry(*load.NewCmdStat([]byte("CURSOR_READ"), took, false, false, txBytesCount, rxBytesCount))
			cursor_id = 0
			if len(aggreply) == 2 {
				cursor_id = aggreply[1].(int64)
			}
			cursor_cmds++
		}
		//if cursor_cmds > 0 {
		//	p.readCursorCountChan <- cursor_cmds
		//}
	}
	p.cmdChan <- *stat

}

// ProcessBatch reads eventsBatches which contain rows of databuild for FT.ADD redis command string
func (p *processor) ProcessBatch(b load.Batch, doLoad bool) (outstat load.Stat) {
	outstat = *load.NewStat()
	events := b.(*eventsBatch)
	rowCnt := uint64(len(events.rows))
	if doLoad {
		buflen := rowCnt + 1

		p.cmdChan = make(chan load.Stat, buflen)
		p.wg = &sync.WaitGroup{}
		p.rows = make(chan string, buflen)
		p.wg.Add(1)
		go connectionProcessor(p)
		for _, row := range events.rows {
			p.rows <- row
		}
		close(p.rows)
		p.wg.Wait()

		close(p.cmdChan)

		for cmdStat := range p.cmdChan {
			outstat.Merge(cmdStat)
		}
	}
	events.rows = events.rows[:0]
	ePool.Put(events)
	return
}

func (p *processor) Close(_ bool) {
}

func rowToHash(row string) (cmdType string, cmd string, args []string, bytelen uint64, err error) {

	argsStr := strings.Split(row, ",")

	// we need at least the cmdType and command
	if len(argsStr) >= 2 {
		cmdType = argsStr[0]
		cmd = argsStr[1]
		args = argsStr[2:]
		bytelen = uint64(len(row)) - uint64(len(cmdType))
	}

	return
}