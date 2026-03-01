#!/usr/bin/env bash

set -euo pipefail

redpanda_container="${BENCH_REDPANDA_CONTAINER:-midaz-redpanda}"
postgres_container="${BENCH_POSTGRES_CONTAINER:-midaz-postgres-primary}"
consumer_group="${BENCH_CONSUMER_GROUP:-midaz-balance-projector}"
org_id="${BENCH_ORGANIZATION_ID:-}"
namespace="${BENCH_NAMESPACE:-}"

if [ -z "$org_id" ]; then
  if [ -n "$namespace" ]; then
    org_id=$(docker exec "$postgres_container" psql -U midaz -d onboarding -v ns="$namespace" -Atc "SELECT id FROM organization WHERE legal_name = ('Lerian Bench ' || :'ns' || ' Org 1') AND deleted_at IS NULL ORDER BY created_at DESC LIMIT 1" | xargs || true)
  else
    org_id=$(docker exec "$postgres_container" psql -U midaz -d onboarding -Atc "SELECT id FROM organization WHERE legal_name LIKE 'Lerian Bench api\\_% Org 1' AND deleted_at IS NULL ORDER BY created_at DESC LIMIT 1" | xargs || true)
  fi

  if [ -n "$org_id" ]; then
    echo "Auto-discovered BENCH_ORGANIZATION_ID=${org_id}"
  else
    echo "Error: BENCH_ORGANIZATION_ID is required (or set BENCH_NAMESPACE for auto-discovery)"
    exit 1
  fi
fi

uuid_regex='^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[89abAB][0-9a-fA-F]{3}-[0-9a-fA-F]{12}$'
if [[ ! "$org_id" =~ $uuid_regex ]]; then
  echo "Error: BENCH_ORGANIZATION_ID must be a valid UUID"
  exit 1
fi

max_wait_seconds="${BENCH_VALIDATE_MAX_WAIT_SECONDS:-60}"
enforce_lag_zero="${BENCH_VALIDATE_ENFORCE_LAG_ZERO:-false}"

if [ "$enforce_lag_zero" = "true" ]; then
  echo "Waiting for consumer lag to drain (group=${consumer_group})..."
  lag=0
  for i in $(seq 1 "$max_wait_seconds"); do
    lag=$(docker exec "$redpanda_container" rpk group describe "$consumer_group" --format json 2>/dev/null | jq 'if type == "array" then ((.[0].total_lag) // ((.[0].partitions // []) | map(.lag // 0) | add // 0)) else (.total_lag // ((.partitions // []) | map(.lag // 0) | add // 0)) end')
    if [ "$lag" -eq 0 ]; then
      echo "Consumer lag drained after ${i}s"
      break
    fi
    sleep 1
  done

  if [ "$lag" -ne 0 ]; then
    echo "Error: consumer lag is still ${lag} after ${max_wait_seconds}s"
    exit 1
  fi
else
  lag=$(docker exec "$redpanda_container" rpk group describe "$consumer_group" --format json 2>/dev/null | jq 'if type == "array" then ((.[0].total_lag) // ((.[0].partitions // []) | map(.lag // 0) | add // 0)) else (.total_lag // ((.partitions // []) | map(.lag // 0) | add // 0)) end')
  if [ "$lag" -eq 0 ]; then
    echo "Consumer lag is already zero (group=${consumer_group})"
  else
    echo "Warning: consumer lag is ${lag} (group=${consumer_group}), continuing because BENCH_VALIDATE_ENFORCE_LAG_ZERO=false"
  fi
fi

tx_count=$(docker exec "$postgres_container" psql -U midaz -d transaction -Atc "SELECT count(*) FROM \"transaction\" WHERE organization_id = '$org_id'" | xargs)
op_count=$(docker exec "$postgres_container" psql -U midaz -d transaction -Atc "SELECT count(*) FROM operation WHERE organization_id = '$org_id'" | xargs)

debit_sum=$(docker exec "$postgres_container" psql -U midaz -d transaction -Atc "SELECT COALESCE(SUM(amount), 0) FROM operation WHERE type = 'DEBIT' AND organization_id = '$org_id'" | xargs)
credit_sum=$(docker exec "$postgres_container" psql -U midaz -d transaction -Atc "SELECT COALESCE(SUM(amount), 0) FROM operation WHERE type = 'CREDIT' AND organization_id = '$org_id'" | xargs)

echo "Transactions in Postgres: ${tx_count}"
echo "Operations in Postgres:   ${op_count}"
echo "Total debits:            ${debit_sum}"
echo "Total credits:           ${credit_sum}"

expected_transactions="${BENCH_EXPECTED_TRANSACTIONS:-}"
if [ -n "$expected_transactions" ] && [ "$tx_count" != "$expected_transactions" ]; then
  echo "Error: transaction count mismatch (expected=${expected_transactions}, actual=${tx_count})"
  exit 1
fi

if [ "$debit_sum" = "$credit_sum" ]; then
  echo "OK: double-entry invariant verified"
else
  echo "Error: double-entry mismatch (debits != credits)"
  exit 1
fi
