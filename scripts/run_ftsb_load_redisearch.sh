#!/bin/bash

DATASET="enwiki-latest-abstract1"

DEBUG=${DEBUG:-0}
MAX_INSERTS=${MAX_INSERTS:-0}
BATCH_SIZE=${BATCH_SIZE:-1000}
PIPELINE=${PIPELINE:-100}

PRINT_INTERVAL=100000

# DB IP
IP=${IP:-"localhost"}

# DB PORT
PORT=${PORT:-6379}

HOST="$IP:$PORT"

# Index to load the data into
IDX=${IDX:-"idx1"}

# How many queries would be run
MAX_QUERIES=${MAX_QUERIES:-100000}

# How many concurrent worker would run queries - match num of cores, or default to 8
WORKERS=${WORKERS:-$(grep -c ^processor /proc/cpuinfo 2>/dev/null || echo 8)}

echo ""
echo "---------------------------------------------------------------------------------"
echo "1) $DATASET"
echo "---------------------------------------------------------------------------------"

# create the index
redis-cli ft.create idx1 SCHEMA \
  TITLE TEXT WEIGHT 5 \
  URL TEXT WEIGHT 5 \
  ABSTRACT TEXT WEIGHT 1

if [ -f /tmp/ftsb_generate_data-$DATASET-redisearch.gz ]; then
  cat /tmp/ftsb_generate_data-$DATASET-redisearch.gz |
    gunzip |
    ftsb_load_redisearch -workers $WORKERS -reporting-period 1s \
      -index=$IDX \
      -host=$HOST -limit=${MAX_INSERTS} \
      -debug=$DEBUG 2>&1 | tee ~/redisearch-load-$DATASET-workers-$WORKERS-pipeline-$PIPELINE.txt
else
  echo "dataset file not found at /tmp/ftsb_generate_data-$DATASET-redisearch.gz"
fi

redis-cli ft.info $IDX
