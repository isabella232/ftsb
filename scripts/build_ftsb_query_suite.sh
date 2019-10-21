#!/bin/bash

DATASET="enwiki-latest-abstract1"

MAX_QUERIES=100
WORKERS=8
DEBUG=3

REGENERATE_QUERIES="true"
if [[ "${1}" == "true" ]]; then
  REGENERATE_QUERIES="true"
fi

if [ ! -f /tmp/$DATASET.xml ]; then
  echo "Dataset not found locally. Aborting."
  exit 1
else
  echo "Dataset found locally at /tmp/$DATASET.xml"
  for queryName in "simple-1word-query" "2word-union-query" "2word-intersection-query"; do
    echo "generating query: $queryName"
    if [ ! -f /tmp/redisearch-queries-$DATASET-$queryName-100K-queries-1-0-0.gz ] || [[ "$REGENERATE_QUERIES" == "true" ]]; then
      echo "ftsb_generate_queries file for $queryName not found. Issuing ftsb_generate_queries."
      ftsb_generate_queries -query-type=$queryName \
        -queries $MAX_QUERIES -input-file /tmp/$DATASET.xml \
        -seed 12345 \
        -debug $DEBUG \
        -output-file /tmp/redisearch-queries-$DATASET-$queryName-100K-queries-1-0-0

      cat /tmp/redisearch-queries-$DATASET-$queryName-100K-queries-1-0-0 |
        gzip >/tmp/redisearch-queries-$DATASET-$queryName-100K-queries-1-0-0.gz
    else
      echo "query file for $queryName found at /tmp/redisearch-queries-$DATASET-$queryName-100K-queries-1-0-0.gz."
    fi
  done
fi
