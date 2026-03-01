#!/usr/bin/env bash

set -euo pipefail

redpanda_container="${BENCH_REDPANDA_CONTAINER:-midaz-redpanda}"
consumer_group="${BENCH_CONSUMER_GROUP:-midaz-balance-projector}"

echo "timestamp,topic,partition,lag,committed_offset,end_offset"

while true; do
  docker exec "$redpanda_container" rpk group describe "$consumer_group" --format json 2>/dev/null | \
    jq -r --arg ts "$(date +%s)" '.partitions[]? | [$ts, .topic, .partition, .lag, .committed_offset, .end_offset] | @csv'
  sleep 2
done
