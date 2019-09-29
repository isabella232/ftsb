// tsbs_run_queries_siridb speed tests SiriDB using requests from stdin or file
//

// This program has no knowledge of the internals of the endpoint.
package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/RediSearch/ftsb/query"
	"github.com/RediSearch/redisearch-go/redisearch"
	_ "github.com/lib/pq"
)

// Program option vars:
var (
	host  string
	index string

	showExplain bool
	//	scale        uint64
)

// Global vars:
var (
	runner *query.BenchmarkRunner
)

var (
	client *redisearch.Client
)

// Parse args:
func init() {
	runner = query.NewBenchmarkRunner()

	flag.StringVar(&host, "host", "localhost:6379", "Redis host address and port")
	flag.StringVar(&index, "index", "idx1", "RediSearch index")
	flag.Parse()
	client = redisearch.NewClient(host, index)
}

func main() {
	runner.Run(&query.RediSearchPool, newProcessor)
}

type queryExecutorOptions struct {
	showExplain   bool
	debug         bool
	printResponse bool
}

type Processor struct {
	opts          *queryExecutorOptions
	Metrics       chan uint64
	ResponseSizes chan uint64
	Wg            *sync.WaitGroup
}

func newProcessor() query.Processor { return &Processor{} }

func (p *Processor) Init(numWorker int, wg *sync.WaitGroup, m chan uint64, rs chan uint64) {
	p.Wg = wg
	p.Metrics = m
	p.ResponseSizes = rs

	p.opts = &queryExecutorOptions{
		showExplain:   showExplain,
		debug:         runner.DebugLevel() > 0,
		printResponse: runner.DoPrintResponses(),
	}
}

func (p *Processor) ProcessQuery(q query.Query, isWarm bool) ([]*query.Stat, error) {

	// No need to run again for EXPLAIN
	if isWarm && p.opts.showExplain {
		return nil, nil
	}
	tq := q.(*query.RediSearch)
	total := 0
	took := 0.0
	timedOut := false

	qry := string(tq.RedisQuery)

	t := strings.Split(qry, ",")
	if len(t) < 2 {
		log.Fatalf("The query has not the correct format ", qry)
	}
	command := t[0]
	if p.opts.debug {
		fmt.Println(strings.Join(t, " "))
	}

	switch command {
	case "FT.AGGREGATE":
		queryNum := t[1]
		query := redisearch.NewAggregateQuery()
		switch queryNum {
		case "1":
			//1) One year period, Exact Number of contributions by day, ordered chronologically, for a given editor

			query = query.SetQuery(redisearch.NewQuery(fmt.Sprintf( "@CURRENT_REVISION_EDITOR_USERNAME:%s @CURRENT_REVISION_TIMESTAMP:[%s %s]",t[2],t[3],t[4] ))).
				SetMax(365).
				Apply(*redisearch.NewProjection("@CURRENT_REVISION_TIMESTAMP - (@CURRENT_REVISION_TIMESTAMP % 86400)", "day")).
				GroupBy(*redisearch.NewGroupBy("@day").
					Reduce(*redisearch.NewReducerAlias(redisearch.GroupByReducerCount, []string{"@ID"}, "num_contributions"))).
				SortBy([]redisearch.SortingKey{*redisearch.NewSortingKeyDir("@day", false)}).
				Apply(*redisearch.NewProjection("timefmt(@day)", "day"))

		case "2":
			//2) One month period, Exact Number of distinct editors contributions by hour, ordered chronologically
			query = query.
				SetMax(720).
				Apply(*redisearch.NewProjection("@CURRENT_REVISION_TIMESTAMP - (@CURRENT_REVISION_TIMESTAMP % 3600)", "hour")).
				GroupBy(*redisearch.NewGroupBy("@hour").
					Reduce(*redisearch.NewReducerAlias(redisearch.GroupByReducerCount, []string{"@CURRENT_REVISION_EDITOR_USERNAME"}, "num_distinct_editors"))).
				SortBy([]redisearch.SortingKey{*redisearch.NewSortingKeyDir("@hour", false)}).
				Apply(*redisearch.NewProjection("timefmt(@hour)", "hour"))

		case "3":
			//3) One month period, Approximate Number of distinct editors contributions by hour, ordered chronologically
			query = query.
				SetMax(720).
				Apply(*redisearch.NewProjection("@CURRENT_REVISION_TIMESTAMP - (@CURRENT_REVISION_TIMESTAMP % 3600)", "hour")).
				GroupBy(*redisearch.NewGroupBy("@hour").
					Reduce(*redisearch.NewReducerAlias(redisearch.GroupByReducerCountDistinctish, []string{"@CURRENT_REVISION_EDITOR_USERNAME"}, "num_distinct_editors"))).
				SortBy([]redisearch.SortingKey{*redisearch.NewSortingKeyDir("@hour", false)}).
				Apply(*redisearch.NewProjection("timefmt(@hour)", "hour"))

		case "4":
			//4) One day period, Approximate Number of contributions by 5minutes interval by editor username, ordered first chronologically and second alphabetically by Revision editor username
			query = query.
				SetMax(288).
				Apply(*redisearch.NewProjection("@CURRENT_REVISION_TIMESTAMP - (@CURRENT_REVISION_TIMESTAMP % 300)", "fiveMinutes")).
				GroupBy(*redisearch.NewGroupByFields([]string{"@fiveMinutes", "@CURRENT_REVISION_EDITOR_USERNAME"}).
					Reduce(*redisearch.NewReducerAlias(redisearch.GroupByReducerCountDistinctish, []string{"@ID"}, "num_contributions"))).
				Filter("@CURRENT_REVISION_EDITOR_USERNAME !=\"\"").
				SortBy([]redisearch.SortingKey{*redisearch.NewSortingKeyDir("@fiveMinutes", true), *redisearch.NewSortingKeyDir("@CURRENT_REVISION_EDITOR_USERNAME", false)}).
				Apply(*redisearch.NewProjection("timefmt(@fiveMinutes)", "fiveMinutes"))

		case "5":
			//5) Aproximate All time Top 10 Revision editor usernames
			query = query.
				GroupBy(*redisearch.NewGroupBy("@CURRENT_REVISION_EDITOR_USERNAME").
					Reduce(*redisearch.NewReducerAlias(redisearch.GroupByReducerCountDistinctish, []string{"@ID"}, "num_contributions"))).
				Filter("@CURRENT_REVISION_EDITOR_USERNAME !=\"\"").
				SortBy([]redisearch.SortingKey{*redisearch.NewSortingKeyDir("@num_contributions", true)}).
				Limit(0, 10)

		case "6":
			//6) Aproximate All time Top 10 Revision editor usernames by number of Revisions broken by namespace (TAG field)
			query = query.GroupBy(*redisearch.NewGroupByFields([]string{"@NAMESPACE", "@CURRENT_REVISION_EDITOR_USERNAME"}).
				Reduce(*redisearch.NewReducerAlias(redisearch.GroupByReducerCountDistinctish, []string{"@ID"}, "num_contributions"))).
				Filter("@CURRENT_REVISION_EDITOR_USERNAME !=\"\"").
				SortBy([]redisearch.SortingKey{*redisearch.NewSortingKeyDir("@NAMESPACE", true), *redisearch.NewSortingKeyDir("@num_contributions", true)}).
				Limit(0, 10)

		case "7":
			//7) Top 10 editor username by average revision content
			query = query.GroupBy(*redisearch.NewGroupByFields([]string{"@NAMESPACE", "@CURRENT_REVISION_EDITOR_USERNAME"}).
				Reduce(*redisearch.NewReducerAlias(redisearch.GroupByReducerAvg, []string{"@CURRENT_REVISION_CONTENT_LENGTH"}, "avg_rcl"))).
				SortBy([]redisearch.SortingKey{*redisearch.NewSortingKeyDir("@avg_rcl", false)}).
				Limit(0, 10)

		case "8":
			//8) Approximate average number of contributions a specific each editor makes
			query = query.SetQuery(redisearch.NewQuery("@CURRENT_REVISION_EDITOR_USERNAME:" + t[2])).
				SetMax(365).
				Apply(*redisearch.NewProjection("@CURRENT_REVISION_TIMESTAMP - (@CURRENT_REVISION_TIMESTAMP % 86400)", "day")).
				GroupBy(*redisearch.NewGroupBy("@day").
					Reduce(*redisearch.NewReducerAlias(redisearch.GroupByReducerCount, []string{"@ID"}, "num_contributions"))).
				SortBy([]redisearch.SortingKey{*redisearch.NewSortingKeyDir("@day", false)}).
				Apply(*redisearch.NewProjection("timefmt(@day)", "day"))

		default:
			log.Fatalf("FT.AGGREGATE queryNum (%d) query not supported yet.", queryNum)
		}

		start := time.Now()
		res, total, err := client.Aggregate(query)
		took = float64(time.Since(start).Nanoseconds()) / 1e6
		timedOut = p.handleResponseAggregate(err, timedOut, t, res, total)

	case "FT.SPELLCHECK":
		rediSearchQuery := redisearch.NewQuery(t[1])
		distance, err := strconv.Atoi(t[2])
		if err != nil {
			log.Fatalf("Error converting distance. Error message:|%s|\n", err)
		}
		rediSearchSpellCheckOptions := redisearch.NewSpellCheckOptions(distance)
		start := time.Now()
		suggs, total, err := client.SpellCheck(rediSearchQuery, rediSearchSpellCheckOptions)
		took = float64(time.Since(start).Nanoseconds()) / 1e6
		timedOut = p.handleResponseSpellCheck(err, timedOut, t, suggs, total)

	case "FT.SEARCH":
		rediSearchQuery := redisearch.NewQuery(t[1])
		start := time.Now()
		docs, total, err := client.Search(rediSearchQuery)
		took = float64(time.Since(start).Nanoseconds()) / 1e6
		timedOut = p.handleResponseDocs(err, timedOut, t, docs, total)

	default:
		log.Fatalf("Command not supported yet.", command)
	}

	stat := query.GetStat()
	stat.Init(q.HumanLabelName(), took, uint64(total), timedOut, t[1])

	return []*query.Stat{stat}, nil
}

func (p *Processor) handleResponseDocs(err error, timedOut bool, t []string, docs []redisearch.Document, total int) bool {
	if err != nil {
		if err.Error() == "Command timed out" {
			timedOut = true
			fmt.Fprintln(os.Stderr, "Command timed out. Used query: ", t)
		} else {
			log.Fatalf("Command failed:%v\tError message:%v\tString Error message:|%s|\n", docs, err, err.Error())
		}
	} else {
		if p.opts.printResponse {
			fmt.Println("\nRESPONSE: ", total)
		}
	}
	return timedOut
}

func (p *Processor) handleResponseSpellCheck(err error, timedOut bool, t []string, suggs []redisearch.MisspelledTerm, total int) bool {
	if err != nil {
		if err.Error() == "Command timed out" {
			timedOut = true
			fmt.Fprintln(os.Stderr, "Command timed out. Used query: ", t)
		} else {
			log.Fatalf("Command failed:%v\tError message:%v\tString Error message:|%s|\n", suggs, err, err.Error())
		}
	} else {
		if p.opts.printResponse {
			fmt.Println("\nRESPONSE: ", total)
		}
	}
	return timedOut
}

func (p *Processor) handleResponseAggregate(err error, timedOut bool, t []string, aggs [][]string, total int) bool {
	if err != nil {
		if err.Error() == "Command timed out" {
			timedOut = true
			fmt.Fprintln(os.Stderr, "Command timed out. Used query: ", t)
		} else {
			log.Fatalf("Command failed:%v\tError message:%v\tString Error message:|%s|\n", aggs, err, err.Error())
		}
	} else {
		if p.opts.printResponse {
			fmt.Println("\nRESPONSE: ", total)
		}
	}
	return timedOut
}
