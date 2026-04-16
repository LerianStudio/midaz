#!/usr/bin/env sh
#
# Redpanda topic provisioning for Midaz.
#
# D6 hardening: this script is now ACL-aware. When REDPANDA_SASL_ENABLED=true,
# it creates tenant-scoped ACLs so the authorizer, ledger, and transaction
# services only have the operations they need on the topics they own. Without
# ACLs, any authenticated client can read/write every topic.
#
# Production note on replicas: the dev compose runs a single Redpanda broker,
# so --replicas=1 is the only valid value here. Production deployments MUST
# set REDPANDA_REPLICATION_FACTOR=3 (or higher) in the broker cluster config
# so that losing a single node does not lose committed records. The
# authorizer.cross-shard.commits and ledger.audit.log topics are the two most
# critical — they hold the 2PC journal and immutable audit log respectively,
# and must survive broker failure.

set -eu

BROKER="${REDPANDA_BROKER:-redpanda:9092}"
STRICT_PARTITION_MATCH="${REDPANDA_STRICT_PARTITION_MATCH:-false}"
# Single-broker dev default. In production set REDPANDA_REPLICATION_FACTOR=3.
REPLICATION_FACTOR="${REDPANDA_REPLICATION_FACTOR:-1}"

# SASL admin credentials (optional). When unset, ACL provisioning is skipped
# and the cluster runs in open mode — acceptable for local dev, not for prod.
SASL_ENABLED="${REDPANDA_SASL_ENABLED:-false}"
SASL_ADMIN_USER="${REDPANDA_SASL_ADMIN_USER:-}"
SASL_ADMIN_PASSWORD="${REDPANDA_SASL_ADMIN_PASSWORD:-}"
SASL_MECHANISM="${REDPANDA_SASL_MECHANISM:-SCRAM-SHA-256}"

# rpk v24+ replaced --brokers with -X brokers=...
RPK_BROKER_FLAG="-X brokers=${BROKER}"

# Auth flags are appended to every rpk invocation when SASL is on. The admin
# user is expected to be a Redpanda superuser (declared via superusers:[] in
# redpanda.yaml) so it can manage topics and ACLs.
RPK_AUTH_FLAGS=""
if [ "$SASL_ENABLED" = "true" ]; then
  if [ -z "$SASL_ADMIN_USER" ] || [ -z "$SASL_ADMIN_PASSWORD" ]; then
    echo "REDPANDA_SASL_ENABLED=true requires REDPANDA_SASL_ADMIN_USER and REDPANDA_SASL_ADMIN_PASSWORD" >&2
    exit 1
  fi
  RPK_AUTH_FLAGS="-X user=${SASL_ADMIN_USER} -X pass=${SASL_ADMIN_PASSWORD} -X sasl.mechanism=${SASL_MECHANISM}"
fi

current_partition_count() {
  topic="$1"

  # Extract partition count from the SUMMARY section (e.g. "PARTITIONS  128").
  # Falls back to counting individual partition rows for older rpk versions.
  count="$(rpk topic describe "$topic" $RPK_BROKER_FLAG $RPK_AUTH_FLAGS 2>/dev/null \
    | awk '/^PARTITIONS/ { print $2; found=1; exit }
           END { if (!found) print 0 }')"

  if [ "${count:-0}" -gt 0 ] 2>/dev/null; then
    echo "$count"
  else
    rpk topic describe "$topic" $RPK_BROKER_FLAG $RPK_AUTH_FLAGS 2>/dev/null | awk '
      NR > 1 && $1 ~ /^[0-9]+$/ { count++ }
      END { print count + 0 }
    '
  fi
}

ensure_topic() {
  topic="$1"
  partitions="$2"
  retention_ms="$3"

  if rpk topic describe "$topic" $RPK_BROKER_FLAG $RPK_AUTH_FLAGS >/dev/null 2>&1; then
    current_partitions="$(current_partition_count "$topic")"

    if [ "$current_partitions" -lt "$partitions" ]; then
      add_count=$((partitions - current_partitions))
      rpk topic add-partitions "$topic" \
        $RPK_BROKER_FLAG $RPK_AUTH_FLAGS \
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
      $RPK_BROKER_FLAG $RPK_AUTH_FLAGS \
      --set "retention.ms=$retention_ms"

    return
  fi

  rpk topic create "$topic" \
    $RPK_BROKER_FLAG $RPK_AUTH_FLAGS \
    --partitions "$partitions" \
    --replicas "$REPLICATION_FACTOR" \
    --topic-config "retention.ms=$retention_ms"
}

# ensure_acl grants a single operation on a single topic to a principal.
# Idempotent: rpk acl create exits 0 if the ACL already exists.
ensure_acl() {
  principal="$1"
  topic="$2"
  operation="$3"

  if [ "$SASL_ENABLED" != "true" ]; then
    return
  fi

  rpk acl create \
    $RPK_BROKER_FLAG $RPK_AUTH_FLAGS \
    --allow-principal "User:${principal}" \
    --operation "$operation" \
    --topic "$topic" \
    || echo "Warning: failed to create ACL principal=${principal} topic=${topic} op=${operation}"
}

# ensure_consumer_group_acl grants read on a consumer group.
ensure_consumer_group_acl() {
  principal="$1"
  group="$2"

  if [ "$SASL_ENABLED" != "true" ]; then
    return
  fi

  rpk acl create \
    $RPK_BROKER_FLAG $RPK_AUTH_FLAGS \
    --allow-principal "User:${principal}" \
    --operation read \
    --operation describe \
    --group "$group" \
    || echo "Warning: failed to create group ACL principal=${principal} group=${group}"
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

# Tenant-scoped ACLs (only applied when SASL is enabled; principals must exist
# as SASL users on the broker). Service principals are named after the
# component they represent so audit logs can attribute operations back to a
# specific service.
#
# - authorizer: owns cross-shard 2PC commits + may publish/consume ledger.*
#   during recovery.
# - ledger: produces balance/transaction/audit events.
# - transaction: consumes balance.create + balance.operations + retry/dlt.
#
# Cross-service read permissions exist because the consumer service tails
# every ledger.* topic today; tighten this list once ownership is split.

# --- Authorizer principal ---
ensure_acl "${REDPANDA_AUTHORIZER_PRINCIPAL:-midaz-authorizer}" "authorizer.cross-shard.commits" write
ensure_acl "${REDPANDA_AUTHORIZER_PRINCIPAL:-midaz-authorizer}" "authorizer.cross-shard.commits" read
ensure_acl "${REDPANDA_AUTHORIZER_PRINCIPAL:-midaz-authorizer}" "authorizer.cross-shard.commits" describe
ensure_consumer_group_acl "${REDPANDA_AUTHORIZER_PRINCIPAL:-midaz-authorizer}" "authorizer-cross-shard-recovery"

# --- Ledger principal (producer on ledger.*) ---
for topic in ledger.balance.operations ledger.balance.create ledger.transaction.events ledger.audit.log \
             ledger.balance.create.retry ledger.balance.operations.retry; do
  ensure_acl "${REDPANDA_LEDGER_PRINCIPAL:-midaz-ledger}" "$topic" write
  ensure_acl "${REDPANDA_LEDGER_PRINCIPAL:-midaz-ledger}" "$topic" describe
done

# --- Transaction/Consumer principal (consumer on ledger.*) ---
for topic in ledger.balance.operations ledger.balance.create ledger.transaction.events \
             ledger.balance.create.retry ledger.balance.create.dlt \
             ledger.balance.operations.retry ledger.balance.operations.dlt; do
  ensure_acl "${REDPANDA_CONSUMER_PRINCIPAL:-midaz-consumer}" "$topic" read
  ensure_acl "${REDPANDA_CONSUMER_PRINCIPAL:-midaz-consumer}" "$topic" describe
  # Consumer republishes failed records to .retry / .dlt — needs write on those.
  case "$topic" in
    *.retry|*.dlt)
      ensure_acl "${REDPANDA_CONSUMER_PRINCIPAL:-midaz-consumer}" "$topic" write
      ;;
  esac
done
ensure_consumer_group_acl "${REDPANDA_CONSUMER_PRINCIPAL:-midaz-consumer}" "midaz-balance-projector"

echo "Redpanda topics ready"
if [ "$SASL_ENABLED" = "true" ]; then
  echo "SASL ACLs provisioned (mechanism=${SASL_MECHANISM})"
else
  echo "SASL disabled — cluster is in OPEN mode. Set REDPANDA_SASL_ENABLED=true for production."
fi
