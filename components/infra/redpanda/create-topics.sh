#!/usr/bin/env sh

set -eu

BROKER="${REDPANDA_BROKER:-redpanda:9092}"

create_topic() {
  topic="$1"
  partitions="$2"
  retention_ms="$3"

  rpk topic create "$topic" \
    --brokers "$BROKER" \
    --partitions "$partitions" \
    --replicas 1 \
    --topic-config "retention.ms=$retention_ms" || true
}

create_topic "ledger.balance.operations" 16 2592000000
create_topic "ledger.balance.create" 8 604800000
create_topic "ledger.transaction.events" 12 604800000
create_topic "ledger.audit.log" 8 31536000000
create_topic "ledger.balance.create.retry" 8 86400000
create_topic "ledger.balance.create.dlt" 4 -1
create_topic "ledger.balance.operations.retry" 8 86400000
create_topic "ledger.balance.operations.dlt" 4 -1

echo "Redpanda topics ready"
