#!/usr/bin/env sh

set -eu

BROKER="${REDPANDA_BROKER:-redpanda:9092}"
STRICT_PARTITION_MATCH="${REDPANDA_STRICT_PARTITION_MATCH:-false}"

# rpk v24+ replaced --brokers with -X brokers=...
RPK_BROKER_FLAG="-X brokers=${BROKER}"

current_partition_count() {
  topic="$1"

  # Extract partition count from the SUMMARY section (e.g. "PARTITIONS  128").
  # Falls back to counting individual partition rows for older rpk versions.
  count="$(rpk topic describe "$topic" $RPK_BROKER_FLAG 2>/dev/null \
    | awk '/^PARTITIONS/ { print $2; found=1; exit }
           END { if (!found) print 0 }')"

  if [ "${count:-0}" -gt 0 ] 2>/dev/null; then
    echo "$count"
  else
    rpk topic describe "$topic" $RPK_BROKER_FLAG 2>/dev/null | awk '
      NR > 1 && $1 ~ /^[0-9]+$/ { count++ }
      END { print count + 0 }
    '
  fi
}

ensure_topic() {
  topic="$1"
  partitions="$2"
  retention_ms="$3"

  if rpk topic describe "$topic" $RPK_BROKER_FLAG >/dev/null 2>&1; then
    current_partitions="$(current_partition_count "$topic")"

    if [ "$current_partitions" -lt "$partitions" ]; then
      add_count=$((partitions - current_partitions))
      rpk topic add-partitions "$topic" \
        $RPK_BROKER_FLAG \
        --num "$add_count"
    elif [ "$current_partitions" -gt "$partitions" ]; then
      echo "Warning: topic '$topic' has $current_partitions partitions but desired is $partitions."
      echo "Redpanda cannot decrease partition count in-place; preserving existing partition count."

      if [ "$STRICT_PARTITION_MATCH" = "true" ]; then
        echo "Strict partition match enabled (REDPANDA_STRICT_PARTITION_MATCH=true); aborting startup."
        exit 1
      fi
    fi

    rpk topic alter-config "$topic" \
      $RPK_BROKER_FLAG \
      --set "retention.ms=$retention_ms"

    return
  fi

  rpk topic create "$topic" \
    $RPK_BROKER_FLAG \
    --partitions "$partitions" \
    --replicas 1 \
    --topic-config "retention.ms=$retention_ms"
}

ensure_topic "ledger.balance.operations" 8 2592000000
ensure_topic "ledger.balance.create" 8 604800000
ensure_topic "ledger.transaction.events" 12 604800000
ensure_topic "ledger.audit.log" 8 31536000000
ensure_topic "ledger.balance.create.retry" 8 86400000
ensure_topic "ledger.balance.create.dlt" 4 -1
ensure_topic "ledger.balance.operations.retry" 8 86400000
ensure_topic "ledger.balance.operations.dlt" 4 -1
ensure_topic "authorizer.cross-shard.commits" 4 604800000

echo "Redpanda topics ready"
