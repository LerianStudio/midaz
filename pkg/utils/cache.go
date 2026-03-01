// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package utils

import (
	"fmt"
	"strings"

	"github.com/google/uuid"

	"github.com/LerianStudio/midaz/v3/pkg/shard"
)

// Balance sync Redis key constants.
const (
	BalanceSyncScheduleKey = "schedule:{transactions}:balance-sync"
	BalanceSyncLockPrefix  = "lock:{transactions}:balance-sync:"
)

const (
	beginningKey = "{"
	keySeparator = ":"
	endKey       = "}"
)

// TransactionInternalKey returns a key with the following format to be used on redis cluster:
// "transaction:{transactions}:organizationID:ledgerID:key".
func TransactionInternalKey(organizationID, ledgerID uuid.UUID, key string) string {
	var builder strings.Builder

	builder.WriteString("transaction")
	builder.WriteString(keySeparator)
	builder.WriteString(beginningKey)
	builder.WriteString("transactions")
	builder.WriteString(endKey)
	builder.WriteString(keySeparator)
	builder.WriteString(organizationID.String())
	builder.WriteString(keySeparator)
	builder.WriteString(ledgerID.String())
	builder.WriteString(keySeparator)
	builder.WriteString(key)

	return builder.String()
}

// BalanceInternalKey returns a key with the following format to be used on redis cluster:
// "balance:{transactions}:organizationID:ledgerID:key".
func BalanceInternalKey(organizationID, ledgerID uuid.UUID, key string) string {
	var builder strings.Builder

	builder.WriteString("balance")
	builder.WriteString(keySeparator)
	builder.WriteString(beginningKey)
	builder.WriteString("transactions")
	builder.WriteString(endKey)
	builder.WriteString(keySeparator)
	builder.WriteString(organizationID.String())
	builder.WriteString(keySeparator)
	builder.WriteString(ledgerID.String())
	builder.WriteString(keySeparator)
	builder.WriteString(key)

	return builder.String()
}

// IdempotencyReverseKey returns a key with the following format to be used on redis cluster:
// "idempotency_reverse:{organizationID:ledgerID}:transactionID"
// This key maps a transactionID to its idempotency key for reverse lookups.
func IdempotencyReverseKey(organizationID, ledgerID uuid.UUID, transactionID string) string {
	var builder strings.Builder

	builder.WriteString("idempotency_reverse")
	builder.WriteString(keySeparator)
	builder.WriteString(beginningKey)
	builder.WriteString(organizationID.String())
	builder.WriteString(keySeparator)
	builder.WriteString(ledgerID.String())
	builder.WriteString(endKey)
	builder.WriteString(keySeparator)
	builder.WriteString(transactionID)

	return builder.String()
}

// IdempotencyInternalKey returns a key with the following format to be used on redis cluster:
// "idempotency:{organizationID:ledgerID:key}".
func IdempotencyInternalKey(organizationID, ledgerID uuid.UUID, key string) string {
	var builder strings.Builder

	builder.WriteString("idempotency")
	builder.WriteString(keySeparator)
	builder.WriteString(beginningKey)
	builder.WriteString(organizationID.String())
	builder.WriteString(keySeparator)
	builder.WriteString(ledgerID.String())
	builder.WriteString(keySeparator)
	builder.WriteString(key)
	builder.WriteString(endKey)

	return builder.String()
}

// AccountingRoutesInternalKey returns a key with the following format to be used on redis cluster:
// "accounting_routes:{organizationID:ledgerID:key}".
func AccountingRoutesInternalKey(organizationID, ledgerID, key uuid.UUID) string {
	var builder strings.Builder

	builder.WriteString("accounting_routes")
	builder.WriteString(keySeparator)
	builder.WriteString(beginningKey)
	builder.WriteString(organizationID.String())
	builder.WriteString(keySeparator)
	builder.WriteString(ledgerID.String())
	builder.WriteString(keySeparator)
	builder.WriteString(key.String())
	builder.WriteString(endKey)

	return builder.String()
}

// PendingTransactionLockKey returns a key with the following format to be used on redis cluster:
// "pending_transaction:{transaction}:organizationID:ledgerID:transactionID"
// This key is used to lock pending transactions during commit/cancel operations.
func PendingTransactionLockKey(organizationID, ledgerID uuid.UUID, transactionID string) string {
	var builder strings.Builder

	builder.WriteString("pending_transaction")
	builder.WriteString(keySeparator)
	builder.WriteString(beginningKey)
	builder.WriteString("transaction")
	builder.WriteString(endKey)
	builder.WriteString(keySeparator)
	builder.WriteString(organizationID.String())
	builder.WriteString(keySeparator)
	builder.WriteString(ledgerID.String())
	builder.WriteString(keySeparator)
	builder.WriteString(transactionID)

	return builder.String()
}

// RedisConsumerLockKey returns a key with the following format to be used on redis cluster:
// "redis_consumer_lock:{organizationID:ledgerID}:transactionID"
// This key is used to prevent duplicate processing of the same transaction across multiple pods.
func RedisConsumerLockKey(organizationID, ledgerID uuid.UUID, transactionID string) string {
	var builder strings.Builder

	builder.WriteString("redis_consumer_lock")
	builder.WriteString(keySeparator)
	builder.WriteString(beginningKey)
	builder.WriteString(organizationID.String())
	builder.WriteString(keySeparator)
	builder.WriteString(ledgerID.String())
	builder.WriteString(endKey)
	builder.WriteString(keySeparator)
	builder.WriteString(transactionID)

	return builder.String()
}

// --------------------------------------------------------------------------
// Shard-aware key generation (Phase 2A)
//
// These functions use {shard_N} hash tags instead of {transactions}.
// Each shard's keys land on a different Redis Cluster hash slot,
// enabling parallel Lua execution across N shards.
// --------------------------------------------------------------------------.

// BalanceShardKey returns a shard-aware balance key:
// "balance:{shard_N}:organizationID:ledgerID:aliasKey"
//
// The hash tag {shard_N} ensures this key lands on the correct Redis Cluster
// shard node, enabling atomic Lua EVAL with other keys on the same shard.
// Negative shardID values are clamped to 0.
func BalanceShardKey(shardID int, organizationID, ledgerID uuid.UUID, aliasKey string) string {
	if shardID < 0 {
		shardID = 0
	}

	var builder strings.Builder

	builder.WriteString("balance")
	builder.WriteString(keySeparator)
	builder.WriteString(shard.HashTag(shardID))
	builder.WriteString(keySeparator)
	builder.WriteString(organizationID.String())
	builder.WriteString(keySeparator)
	builder.WriteString(ledgerID.String())
	builder.WriteString(keySeparator)
	builder.WriteString(aliasKey)

	return builder.String()
}

// TransactionShardKey returns a shard-aware transaction backup key:
// "transaction:{shard_N}:organizationID:ledgerID:transactionID"
// Negative shardID values are clamped to 0.
func TransactionShardKey(shardID int, organizationID, ledgerID uuid.UUID, transactionID string) string {
	if shardID < 0 {
		shardID = 0
	}

	var builder strings.Builder

	builder.WriteString("transaction")
	builder.WriteString(keySeparator)
	builder.WriteString(shard.HashTag(shardID))
	builder.WriteString(keySeparator)
	builder.WriteString(organizationID.String())
	builder.WriteString(keySeparator)
	builder.WriteString(ledgerID.String())
	builder.WriteString(keySeparator)
	builder.WriteString(transactionID)

	return builder.String()
}

// BackupQueueShardKey returns a shard-aware backup queue key:
// "backup_queue:{shard_N}"
//
// Each shard has its own backup queue hash so the key co-locates
// with the balance keys on the same Redis Cluster node.
// Negative shardID values are clamped to 0.
func BackupQueueShardKey(shardID int) string {
	if shardID < 0 {
		shardID = 0
	}

	return "backup_queue:" + shard.HashTag(shardID)
}

// BalanceSyncScheduleShardKey returns a shard-aware balance sync schedule key:
// "schedule:{shard_N}:balance-sync"
// Negative shardID values are clamped to 0.
func BalanceSyncScheduleShardKey(shardID int) string {
	if shardID < 0 {
		shardID = 0
	}

	return fmt.Sprintf("schedule:%s:balance-sync", shard.HashTag(shardID))
}

// BalanceSyncLockShardPrefix returns a shard-aware balance sync lock prefix:
// "lock:{shard_N}:balance-sync:"
// Negative shardID values are clamped to 0.
func BalanceSyncLockShardPrefix(shardID int) string {
	if shardID < 0 {
		shardID = 0
	}

	return fmt.Sprintf("lock:%s:balance-sync:", shard.HashTag(shardID))
}

// ShardRoutingKey returns the routing override table key for an org/ledger pair:
// "shard_routing:{organizationID:ledgerID}".
func ShardRoutingKey(organizationID, ledgerID uuid.UUID) string {
	return fmt.Sprintf("shard_routing:{%s:%s}", organizationID.String(), ledgerID.String())
}

// ShardRoutingUpdatesChannel returns the pub/sub channel for routing updates:
// "shard_routing_updates:{organizationID:ledgerID}".
func ShardRoutingUpdatesChannel(organizationID, ledgerID uuid.UUID) string {
	return fmt.Sprintf("shard_routing_updates:{%s:%s}", organizationID.String(), ledgerID.String())
}

// MigrationLockKey returns the migration freeze key for an account alias:
// "migration:{organizationID:ledgerID}:alias".
func MigrationLockKey(organizationID, ledgerID uuid.UUID, alias string) string {
	return fmt.Sprintf("migration:{%s:%s}:%s", organizationID.String(), ledgerID.String(), alias)
}

// ShardMetricsKey returns the shard load metric key:
// "shard_metrics:{shard_N}"
// Negative shardID values are clamped to 0.
func ShardMetricsKey(shardID int) string {
	if shardID < 0 {
		shardID = 0
	}

	return "shard_metrics:" + shard.HashTag(shardID)
}

// ShardHotAccountsBucketKey returns the shard rolling hot-account counters key:
// "shard_hot_accounts_bucket:{shard_N}"
// Negative shardID values are clamped to 0.
func ShardHotAccountsBucketKey(shardID int) string {
	if shardID < 0 {
		shardID = 0
	}

	return "shard_hot_accounts_bucket:" + shard.HashTag(shardID)
}

// ShardRebalanceStateKey returns the global rebalance state key.
func ShardRebalanceStateKey() string {
	return "shard_rebalance:{global}:state"
}

// ShardRebalanceShardCooldownKey returns a shard-level migration cooldown key.
// Negative shardID values are clamped to 0.
func ShardRebalanceShardCooldownKey(shardID int) string {
	if shardID < 0 {
		shardID = 0
	}

	return fmt.Sprintf("shard_rebalance:%s:cooldown", shard.HashTag(shardID))
}

// ShardRebalanceAccountCooldownKey returns an account-level anti-thrash cooldown key.
func ShardRebalanceAccountCooldownKey(organizationID, ledgerID uuid.UUID, alias string) string {
	return fmt.Sprintf("shard_rebalance_account:{%s:%s}:%s", organizationID.String(), ledgerID.String(), alias)
}

// ShardIsolationSetKey returns the set key of dedicated accounts hosted by a shard.
// Negative shardID values are clamped to 0.
func ShardIsolationSetKey(shardID int) string {
	if shardID < 0 {
		shardID = 0
	}

	return "shard_isolation:" + shard.HashTag(shardID)
}

// ShardIsolationAccountKey returns the dedicated shard marker for an account.
func ShardIsolationAccountKey(organizationID, ledgerID uuid.UUID, alias string) string {
	return fmt.Sprintf("shard_isolation_account:{%s:%s}:%s", organizationID.String(), ledgerID.String(), alias)
}
