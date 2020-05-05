package main

import (
	"github.com/RediSearch/ftsb/load"
	"github.com/RediSearch/redisearch-go/redisearch"
	"github.com/mediocregopher/radix"
	"log"
	"sync"
	"time"
)

type processor struct {
	dbc              *dbCreator
	rows             chan string
	insertsChan      chan uint64
	totalLatencyChan chan uint64
	updatesChan      chan uint64
	deletesChan      chan uint64
	totalBytesChan   chan uint64
	wg               *sync.WaitGroup
	client           *redisearch.Client
	insertedDocIds   []string
	updatedDocIds    []string
	deletedDocIds    []string
	vanillaClient    *radix.Pool
}

func (p *processor) Init(_ int, _ bool) {
	p.client = redisearch.NewClient(host, loader.DatabaseName())
	var err error = nil
	p.vanillaClient, err = radix.NewPool("tcp", host, 1, radix.PoolPipelineWindow(0, 0))
	if err != nil {
		log.Fatalf("Error preparing for redisearch ingestion, while creating new pool. error = %v", err)
	}
}

// using random between [0,1) to determine whether it is an delete,update, or insert
// DELETE IF BETWEEN [0,deleteLimit)
// UPDATE IF BETWEEN [deleteLimit,updateLimit)
// INSERT IF BETWEEN [updateLimit,1)
func connectionProcessor(p *processor, pipeline uint64, updateRate float64, deleteRate float64, noSaveOption bool, updatePartial bool, updateCondition string, useHashes bool) {
	var documents = make([]redisearch.Document, 0)
	var documentHashes = make([][]string, 0)

	pipelinePos := uint64(0)
	insertCount := uint64(0)
	totalBytes := uint64(0)

	deleteUpperLimit := 0.0
	updateUpperLimit := deleteUpperLimit + updateRate

	updateOpts := redisearch.IndexingOptions{
		Language:         "",
		NoSave:           noSaveOption,
		Replace:          true,
		Partial:          updatePartial,
		ReplaceCondition: updateCondition,
	}

	indexingOpts := redisearch.DefaultIndexingOptions
	indexingOpts.NoSave = noSaveOption

	for row := range p.rows {
		if useHashes == false {
			doc := rowToRSDocument(row)
			if doc != nil {
				totalBytes, pipelinePos, documents, insertCount = ftaddInsertWorkflow(p, pipeline, doc, totalBytes, deleteUpperLimit, updateUpperLimit, pipelinePos, indexingOpts, documents, insertCount, updateOpts)
			}
		} else {
			_, args, bytelen, _ := rowToHash(row)
			totalBytes += bytelen
			documentHashes = append(documentHashes, args)
		}
	}
	// In the there are still documents to be processed
	if useHashes == false && pipelinePos != 0 && len(documents) > 0 {
		// Index the document. The API accepts multiple documents at a time
		processorIndexInsertDocuments(p, indexingOpts, documents, totalBytes, insertCount)
		documents, insertCount, pipelinePos, totalBytes = LocalCountersReset()
	}
	p.wg.Done()
}

func processorIndexUpdateDocument(p *processor, updateOpts redisearch.IndexingOptions, doc *redisearch.Document, totalBytes uint64) {
	start := time.Now()
	if err := p.client.IndexOptions(updateOpts, *doc); err != nil {
		log.Fatalf("failed: %s\n", err)
	}
	took := uint64(time.Since(start).Milliseconds())
	p.updatesChan <- 1
	updateCommonChannels(p, took, totalBytes)
}

func processorIndexInsertDocuments(p *processor, opts redisearch.IndexingOptions, documents []redisearch.Document, bytesCount uint64, insertCount uint64) () {
	start := time.Now()
	if err := p.client.IndexOptions(opts, documents...); err != nil {
		log.Fatalf("failed: %s\n", err)
	}
	took := uint64(time.Since(start).Milliseconds())
	p.insertsChan <- insertCount
	updateCommonChannels(p, took, bytesCount)
}

func updateCommonChannels(p *processor, took uint64, bytesCount uint64) {
	p.totalLatencyChan <- took
	p.totalBytesChan <- bytesCount
}

// ProcessBatch reads eventsBatches which contain rows of databuild for FT.ADD redis command string
func (p *processor) ProcessBatch(b load.Batch, doLoad bool, updateRate, deleteRate float64, useHashes bool) (uint64, uint64, uint64, uint64, uint64, uint64) {
	events := b.(*eventsBatch)
	rowCnt := uint64(len(events.rows))
	metricCnt := uint64(0)
	updateCount := uint64(0)
	deleteCount := uint64(0)
	totalLatency := uint64(0)
	totalBytes := uint64(0)
	if doLoad {
		buflen := rowCnt + 1

		p.insertsChan = make(chan uint64, buflen)
		p.updatesChan = make(chan uint64, buflen)
		p.deletesChan = make(chan uint64, buflen)
		p.totalLatencyChan = make(chan uint64, buflen)
		p.totalBytesChan = make(chan uint64, buflen)

		p.wg = &sync.WaitGroup{}
		p.rows = make(chan string, buflen)
		p.wg.Add(1)
		go connectionProcessor(p, pipeline, updateRate, deleteRate, noSave, replacePartial, replacePartialCondition, useHashes)
		for _, row := range events.rows {
			p.rows <- row
		}
		close(p.rows)
		p.wg.Wait()
		close(p.insertsChan)
		close(p.updatesChan)
		close(p.deletesChan)
		close(p.totalLatencyChan)
		close(p.totalBytesChan)

		for val := range p.insertsChan {
			metricCnt += val
		}
		for val := range p.updatesChan {
			updateCount += val
		}
		for val := range p.deletesChan {
			deleteCount += val
		}
		for val := range p.totalLatencyChan {
			totalLatency += val
		}
		for val := range p.totalBytesChan {
			totalBytes += val
		}

	}
	events.rows = events.rows[:0]
	ePool.Put(events)
	return metricCnt, rowCnt, updateCount, deleteCount, totalLatency, totalBytes
}

func (p *processor) Close(_ bool) {
}
