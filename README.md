[![license](https://img.shields.io/github/license/RediSearch/ftsb.svg)](https://github.com/RediSearch/ftsb)
[![CircleCI](https://circleci.com/gh/RediSearch/ftsb/tree/master.svg?style=svg)](https://circleci.com/gh/RediSearch/ftsb/tree/master)
[![GitHub issues](https://img.shields.io/github/release/RediSearch/ftsb.svg)](https://github.com/RediSearch/ftsb/releases/latest)
[![Codecov](https://codecov.io/gh/RediSearch/ftsb/branch/master/graph/badge.svg)](https://codecov.io/gh/RediSearch/ftsb)
[![Go Report Card](https://goreportcard.com/badge/github.com/RediSearch/ftsb)](https://goreportcard.com/report/github.com/RediSearch/ftsb)
[![GoDoc](https://godoc.org/github.com/RediSearch/ftsb?status.svg)](https://godoc.org/github.com/RediSearch/ftsb)

# Full-Text Search Benchmark (FTSB)
 [![Forum](https://img.shields.io/badge/Forum-RediSearch-blue)](https://forum.redislabs.com/c/modules/redisearch/) 
 [![Gitter](https://badges.gitter.im/RedisLabs/RediSearch.svg)](https://gitter.im/RedisLabs/RediSearch?utm_source=badge&utm_medium=badge&utm_campaign=pr-badge)

This repo contains code for benchmarking full text search databases,
including RediSearch.
This code is based on a fork of work initially made public by TSBS
at https://github.com/timescale/tsbs.

Current databases supported:

+ RediSearch

## Overview

The **Full-Text Search Benchmark (FTSB)** is a collection of Python and Go
programs that are used to generate datasets(Python) and then benchmark(Go) read
and write performance of various databases. 
The intent is to make the
FTSB extensible so that a variety of use cases (e.g., wikipedia, ecommerce, jsondata,
etc.), query types, and databases can be included and benchmarked.  
To this end we hope to help prospective database administrators find the
best database for their needs and their workloads.   
Further, if you
are the developer of a Full-Text Search database and want to include your
database in the FTSB, feel free to open a pull request to add it!

## What the FTSB tests

FTSB is used to benchmark bulk load performance and
query execution performance. 
To accomplish this in a fair way, the data to be inserted and the
queries to run are pre-generated and native Go clients are used
wherever possible to connect to each database.


## Current use cases

Currently, FTSB supports three use cases:
 - **ecommerce-inventory**, From a base dataset of [10K fashion products on Amazon.com](https://data.world/promptcloud/fashion-products-on-amazon-com/workspace/file?filename=amazon_co-ecommerce_sample.csv) which are then multiplexed by categories, sellers, and countries to produce larger datasets > 1M docs. This benchmark focuses on updates and aggregate performance, splitting into Reads (FT.AGGREGATE), Cursor Reads (FT.CURSOR), and Updates (FT.ADD) the performance numbers. 
 The use case generates an index with 10 TAG fields (3 sortable and 1 non indexed), and 16 NUMERIC sortable non indexed fields per document.
 The aggregate queries are designed to be extremely costly both on computation and network TX, given that on each query we're aggregating and filtering over a large portion of the dataset while additionally loading 21 fields. 
 Both the update and read rates can be adjusted.
 
 
 - **enwiki-abstract**, From English-language [Wikipedia:Database](https://en.wikipedia.org/wiki/Wikipedia:Database_download) page abstracts. This use case generates
3 TEXT fields per document.


 - **enwiki-pages**, From English-language [Wikipedia:Database](https://en.wikipedia.org/wiki/Wikipedia:Database_download) last page revisions, containing processed metadata  extracted from the full Wikipedia XML dump.
 This use case generates 4 TEXT fields ( 2 sortable ), 1 sortable TAG field, and 6 sortable NUMERIC fields per document.
              
              
                                                                                                                                                                                                                   
## Installation

FTSB is a collection of Go programs (with some auxiliary bash and Python
scripts). The easiest way to get and install the Go programs is to use
`go get` and then `go install`:
```bash
# Fetch FTSB and its dependencies
go get github.com/RediSearch/ftsb
cd $GOPATH/src/github.com/RediSearch/ftsb/cmd

# Install desired binaries. At a minimum this includes ftsb_generate_data,
# ftsb_generate_queries, one ftsb_load_* binary, and one ftsb_run_queries_*
# binary:
cd $GOPATH/src/github.com/RediSearch/ftsb/cmd
make
```

## How to use FTSB

Using FTSB for benchmarking involves 2 phases: data and query
generation, and query execution.

### Data and query generation

So that benchmarking results are not affected by generating data or
queries on-the-fly, with FTSB you generate the data and queries you want
to benchmark first, and then you can (re-)use it as input to the
benchmarking phases.

#### Data generation

Variables needed:
1. a use case. E.g., `enwiki-abstract` (currently `ecommerce-inventory`, `enwiki-abstract` and `enwiki-pages`)
1. the file from which to read the data from, compliant with the use case. E.g. `enwiki-latest-abstract1.xml.gz`
1. and which database(s) you want to generate for. E.g., `redisearch` (currently only `redisearch`)
1. the number of queries to generate. E.g., `100000`
1. the type of query you'd like to generate. E.g., `2word-intersection-query`
1. the seed to pass to the Pseudorandom number generator. By passing the same seed you always generated the same deterministic dataset. E.g., `12345`
1. and the stop-words to discard on query generation. When searching, stop-words are ignored and treated as if they were not sent to the query processor. Therefore, to be 100% correct we need to prevent those words to enter a query. This list of stop-words should match the one used for the index creation. We use as default the [RediSearch list of stop-words](https://oss.redislabs.com/redisearch/Stopwords.html), namely `a,is,the,an,and,are,as,at,be,but,by,for,if,in,into,it,no,not,of,on,or,such,that,their,then,there,these,they,this,to,was,will,with`

For the last step there are numerous queries to choose from, which are
listed in [Appendix I](#appendix-i-query-types). 

### Benchmarking query execution performance

To measure query execution performance in FTSB, you first need to load
the data using the previous section and generate the queries as
described earlier. Once the data is loaded and the queries are generated,
just use the corresponding `ftsb_run_queries_` binary for the database
being tested:
```bash
ftsb_run_queries_redisearch \
       -file /tmp/redisearch-queries-enwiki-latest-abstract1-2word-intersection-query-100K-queries-1-0-0 \
       -max-queries 100000 -workers 8 -print-interval 20000 
```

#### Sustainable Throughput benchmark
To really understand a system behavior we also can't relay solely on doing the full percentile analysis while stressing the system to it's maximum RPS. 

We need to be able to compare the behavior under different throughput and/or configurations, to be able to get the best "Sustainable Throughput: The throughput achieved while safely maintaining service levels.
 To enabling full percentile spectrum and Sustainable Throughput analysis you can use:
- `--hdr-latencies` : enable writing the High Dynamic Range (HDR) Histogram of Response Latencies to the file with the name specified by this. By default no file will be saved.
- `--max-rps` : enable limiting the rate of queries per second, 0 = no limit. By default no limit is specified and the binaries will stress the DB up to the maximum. A normal "modus operandi" would be to initially stress the system ( no limit on RPS) and afterwards that we know the limit vary with lower rps configurations.

#### Level of parallel queries 
You can change the value of the `--workers` flag to
control the level of parallel queries run at the same time. 

#### Understanding the output 
The resulting stdout output will look similar to this:
```text
(...)
after 80000 queries with 16 workers:
All queries                                                                                               :
+ Query execution latency:
	min:     0.33 ms,  mean:    34.05 ms, q25:    18.13 ms, med(q50):    18.13 ms, q75:    18.13 ms, q99:   158.38 ms, max:   581.23 ms, stddev:    50.28ms, sum: 2724.082 sec, count: 80000

+ Query response size(number docs) statistics:
	min(q0):   350.81 docs, q25:   350.81 docs, med(q50):   350.81 docs, q75:   350.81 docs, q99: 45839.32 docs, max(q100): 252995.00 docs, sum: 176735188 docs

RediSearch 2 Word Intersection Query - English-language Wikipedia:Database page abstracts (random in set words).:
+ Query execution latency:
	min:     0.33 ms,  mean:    34.05 ms, q25:    18.13 ms, med(q50):    18.13 ms, q75:    18.13 ms, q99:   158.38 ms, max:   581.23 ms, stddev:    50.28ms, sum: 2724.082 sec, count: 80000

+ Query response size(number docs) statistics:
	min(q0):   350.81 docs, q25:   350.81 docs, med(q50):   350.81 docs, q75:   350.81 docs, q99: 45839.32 docs, max(q100): 252995.00 docs, sum: 176735188 docs


after 90000 queries with 16 workers:
All queries                                                                                               :
+ Query execution latency:
	min:     0.33 ms,  mean:    35.32 ms, q25:    18.29 ms, med(q50):    18.29 ms, q75:    18.29 ms, q99:   157.98 ms, max:   581.23 ms, stddev:    51.84ms, sum: 3178.594 sec, count: 90000

+ Query response size(number docs) statistics:
	min(q0):   346.37 docs, q25:   346.37 docs, med(q50):   346.37 docs, q75:   346.37 docs, q99: 45593.99 docs, max(q100): 252995.00 docs, sum: 210779012 docs

RediSearch 2 Word Intersection Query - English-language Wikipedia:Database page abstracts (random in set words).:
+ Query execution latency:
	min:     0.33 ms,  mean:    35.32 ms, q25:    18.29 ms, med(q50):    18.29 ms, q75:    18.29 ms, q99:   157.98 ms, max:   581.23 ms, stddev:    51.84ms, sum: 3178.594 sec, count: 90000

+ Query response size(number docs) statistics:
	min(q0):   346.37 docs, q25:   346.37 docs, med(q50):   346.37 docs, q75:   346.37 docs, q99: 45593.99 docs, max(q100): 252995.00 docs, sum: 210779012 docs


++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++
Run complete after 100000 queries with 16 workers:
All queries                                                                                               :
+ Query execution latency:
	min:     0.33 ms,  mean:    36.24 ms, q25:    18.43 ms, med(q50):    18.43 ms, q75:    18.43 ms, q99:   158.22 ms, max:   581.23 ms, stddev:    52.98ms, sum: 3624.437 sec, count: 100000

+ Query response size(number docs) statistics:
	min(q0):   341.94 docs, q25:   341.94 docs, med(q50):   341.94 docs, q75:   341.94 docs, q99: 45312.15 docs, max(q100): 252995.00 docs, sum: 242417188 docs

RediSearch 2 Word Intersection Query - English-language Wikipedia:Database page abstracts (random in set words).:
+ Query execution latency:
	min:     0.33 ms,  mean:    36.24 ms, q25:    18.43 ms, med(q50):    18.43 ms, q75:    18.43 ms, q99:   158.22 ms, max:   581.23 ms, stddev:    52.98ms, sum: 3624.437 sec, count: 100000

+ Query response size(number docs) statistics:
	min(q0):   341.94 docs, q25:   341.94 docs, med(q50):   341.94 docs, q75:   341.94 docs, q99: 45312.15 docs, max(q100): 252995.00 docs, sum: 242417188 docs

Took:  226.577 sec
```


## Appendix I: Query types <a name="appendix-i-query-types"></a>

### Appendix I.I - English-language [Wikipedia:Database](https://en.wikipedia.org/wiki/Wikipedia:Database_download) page abstracts.
#### Full text search queries
|Query type|Description|Example|Status|
|:---|:---|:---|:---|
|simple-1word-query| Simple 1 Word Query | `Abraham` | :heavy_check_mark:
|2word-union-query| 2 Word Union Query | `Abraham Lincoln` | :heavy_check_mark:
|2word-intersection-query| 2 Word Intersection Query| `Abraham`&#124;`Lincoln` | :heavy_check_mark:
|exact-3word-match| Exact 3 Word Match| `"President Abraham Lincoln"` |:heavy_multiplication_x:
|autocomplete-1100-top3| Autocomplete -1100 Top 2-3 Letter Prefixes|  | :heavy_multiplication_x:
|2field-2word-intersection-query| 2 Fields, one word each, Intersection query | `@text_field1: text_value1 @text_field2: text_value2` | :heavy_multiplication_x:
|2field-1word-intersection-1numeric-range-query| 2 Fields, one text and another numeric, Intersection and numeric range query | `@text_field: text_value @numeric_field:[{min} {max}]` |:heavy_multiplication_x:

#### Spell Check queries

Performs spelling correction on a query, returning suggestions for misspelled terms.
To simmulate misspelled terms, for each word a deterministic random number of edits in the range 0..Min(word.length/2 , 4) is chosen. 


For each edit a random type of edit (delete, insert random char, replace with random char, switch adjacent chars).

|Query type|Description|Example|Status|
|:---|:---|:---|:---|
| simple-1word-spellcheck | Simple 1 Word Spell Check Query | `FT.SPELLCHECK {index} reids DISTANCE 1` | :heavy_check_mark:

#### Autocomplete queries
|Query type|Description|Example|Status|
|:---|:---|:---|:---|
| |  | `` | :heavy_multiplication_x:


#### Aggregate queries

Aggregations are a way to process the results of a search query, group, sort and transform them - and extract analytic insights from them. Much like aggregation queries in other databases and search engines, they can be used to create analytics reports, or perform Faceted Search style queries. 

|Query type|Description|Clauses included|Status|
|:---|:---|:---|:---|
| |  | `` | :heavy_multiplication_x:

#### Synonym queries
|Query type|Description|Example|Status|
|:---|:---|:---|:---|
| |  | `` | :heavy_multiplication_x:


### Appendix I.II - English-language [Wikipedia:Database](https://en.wikipedia.org/wiki/Wikipedia:Database_download) last page revisions.

#### Aggregate queries

Aggregations are a way to process the results of a search query, group, sort and transform them - and extract analytic insights from them. Much like aggregation queries in other databases and search engines, they can be used to create analytics reports, or perform Faceted Search style queries. 

|Query #|Query type|Description| Status|
|:---|:---|:---|:---|
| 1 | agg-1-editor-1year-exact-page-contributions-by-day |  One year period, Exact Number of contributions by day, ordered chronologically, for a given editor [(supplemental docs)](docs/redisearch.md#Q1) | :heavy_check_mark:
| 2 | agg-2-*-1month-exact-distinct-editors-by-hour | One month period, Exact Number of distinct editors contributions by hour, ordered chronologically  [(supplemental docs)](docs/redisearch.md#Q2) |:heavy_check_mark:
| 3 | agg-3-*-1month-approximate-distinct-editors-by-hour | One month period, Approximate Number of distinct editors contributions by hour, ordered chronologically  [(supplemental docs)](docs/redisearch.md#Q3) | :heavy_check_mark:
| 4 | agg-4-*-1day-approximate-page-contributions-by-5minutes-by-editor-username | One day period, Approximate Number of contributions by 5minutes interval by editor username, ordered first chronologically and second alphabetically by Revision editor username  [(supplemental docs)](docs/redisearch.md#Q4) |:heavy_check_mark:
| 5 | agg-5-*-1month-approximate-top10-editor-usernames | One month period, Approximate All time Top 10 Revision editor usernames. [(supplemental docs)](docs/redisearch.md#Q5) | :heavy_check_mark:
| 6 | agg-6-*-1month-approximate-top10-editor-usernames-by-namespace |  One month period, Approximate All time Top 10 Revision editor usernames by number of Revisions broken by namespace (TAG field) [(supplemental docs)](docs/redisearch.md#Q6) | :heavy_check_mark:
| 7 | agg-7-*-1month-avg-revision-content-length-by-editor-username |  One month period, Top 10 editor username by average revision content [(supplemental docs)](docs/redisearch.md#Q7) | :heavy_check_mark:
| 8 | agg-8-editor-approximate-avg-editor-contributions-by-year |  Approximate average number of contributions by year each editor makes [(supplemental docs)](docs/redisearch.md#Q8) | :heavy_check_mark:



    
