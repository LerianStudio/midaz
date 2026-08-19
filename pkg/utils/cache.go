// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package utils

import (
	"strings"

	"github.com/google/uuid"
)

const (
	BalanceSyncScheduleKey       = "schedule:{transactions}:balance-sync-v2"
	BalanceSyncScheduleKeyLegacy = "schedule:{transactions}:balance-sync"
	BalanceSyncLockPrefix        = "lock:{transactions}:balance-sync:"
	TracerOutcomeScheduleKey     = "schedule:{transactions}:tracer-outcomes-v2"
	TracerOutcomeActiveKey       = "active:{transactions}:tracer-outcomes-v2"
	TracerOutcomeDispatcherLock  = "lock:{transactions}:tracer-outcomes-v2"
	// TracerOutcomeTenantRegistry is deployment-global rather than tenant-
	// prefixed: its only payload is opaque tenant IDs and a monotonic generation
	// used to discover tenant-scoped outboxes after a process restart. It shares
	// the transaction hash tag so PREPARED, active index, delivery schedule, and
	// discovery registration commit in one Redis Cluster Lua command.
	TracerOutcomeTenantRegistry = "registry:{transactions}:tracer-outcome-tenants"
)

const (
	beginningKey = "{"
	keySeparator = ":"
	endKey       = "}"
)

// TransactionInternalKey returns a key with the following format to be used on redis cluster:
// "transaction:{transactions}:organizationID:ledgerID:key"
func TransactionInternalKey(organizationID, ledgerID uuid.UUID, key string) string {
	var builder strings.Builder

	builder.Grow(101 + len(key)) // "transaction:{transactions}:" + 2×UUID + ":" + key

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

// TransactionBalanceExecutionKey is the same-slot attempt lease consumed
// atomically by the balance Lua when it records an economic outcome.
func TransactionBalanceExecutionKey(organizationID, ledgerID, transactionID uuid.UUID) string {
	return TransactionInternalKey(organizationID, ledgerID, transactionID.String()+":balance-execution")
}

// TransactionPendingBalanceExecutionKey isolates the initial hold mutation
// from the later commit/cancel mutation of the same transaction.
func TransactionPendingBalanceExecutionKey(organizationID, ledgerID, transactionID uuid.UUID) string {
	return TransactionInternalKey(organizationID, ledgerID, transactionID.String()+":pending-hold:balance-execution")
}

// TransactionBalanceOutcomeKey stores the immutable economic outcome written
// by the same Lua command that mutates the transaction's balances.
func TransactionBalanceOutcomeKey(organizationID, ledgerID, transactionID uuid.UUID) string {
	return TransactionInternalKey(organizationID, ledgerID, transactionID.String()+":balance-outcome")
}

// TransactionPendingBalanceOutcomeKey stores the immutable initial-hold fact;
// the terminal lifecycle keeps the standard transaction outcome key.
func TransactionPendingBalanceOutcomeKey(organizationID, ledgerID, transactionID uuid.UUID) string {
	return TransactionInternalKey(organizationID, ledgerID, transactionID.String()+":pending-hold:balance-outcome")
}

// TransactionTracerOutcomeKey stores the durable delivery projection written
// from the balance outcome in the same Redis Lua command.
func TransactionTracerOutcomeKey(organizationID, ledgerID, transactionID uuid.UUID) string {
	return TransactionInternalKey(organizationID, ledgerID, transactionID.String()+":tracer-outcome-v2")
}

var tracerOutcomeIDNamespace = uuid.MustParse("87533e65-910a-5b37-88dd-fb26f25ce75c")

// TransactionTracerOutcomeID derives the stable delivery identity used across
// lost responses and dispatcher retries.
func TransactionTracerOutcomeID(transactionID uuid.UUID) uuid.UUID {
	return uuid.NewSHA1(tracerOutcomeIDNamespace, transactionID[:])
}

// TransactionPersistenceTombstoneKey stores the append-only proof that the
// exact Redis economic outcome was durably handed off to PostgreSQL. It shares
// the transaction slot so terminal cleanup and tombstone publication are one
// atomic operation.
func TransactionPersistenceTombstoneKey(organizationID, ledgerID, transactionID uuid.UUID) string {
	return TransactionInternalKey(organizationID, ledgerID, transactionID.String()+":persistence-tombstone")
}

// TransactionPendingPersistenceTombstoneKey keeps replay proof for the hold
// phase without occupying the commit/cancel terminal tombstone identity.
func TransactionPendingPersistenceTombstoneKey(organizationID, ledgerID, transactionID uuid.UUID) string {
	return TransactionInternalKey(organizationID, ledgerID, transactionID.String()+":pending-hold:persistence-tombstone")
}

// BalanceInternalKey returns a key with the following format to be used on redis cluster:
// "balance:{transactions}:organizationID:ledgerID:key"
func BalanceInternalKey(organizationID, ledgerID uuid.UUID, key string) string {
	var builder strings.Builder

	builder.Grow(97 + len(key)) // "balance:{transactions}:" + 2×UUID + ":" + key

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

	builder.Grow(96 + len(transactionID)) // "idempotency_reverse:{" + 2×UUID + "}:" + transactionID

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
// "idempotency:{organizationID:ledgerID:key}"
func IdempotencyInternalKey(organizationID, ledgerID uuid.UUID, key string) string {
	var builder strings.Builder

	builder.Grow(88 + len(key)) // "idempotency:{" + 2×UUID + ":" + key + "}"

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

// RevertIdempotencyHashSource is the stable preimage for the final,
// origin-scoped Redis idempotency barrier. The PostgreSQL claim is the durable
// authority; this value keeps concurrent final/bridge pods on the same
// per-origin Redis slot without coupling identity to economic payload content.
func RevertIdempotencyHashSource(originTransactionID uuid.UUID) string {
	return "revert\x00" + originTransactionID.String()
}

// AccountingRoutesInternalKey returns a key with the following format to be used on redis cluster:
// "accounting_routes:{organizationID:ledgerID:key}"
func AccountingRoutesInternalKey(organizationID, ledgerID, key uuid.UUID) string {
	var builder strings.Builder

	builder.Grow(130) // "accounting_routes:{" + 3×UUID + 2×":" + "}"

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

	builder.Grow(108 + len(transactionID)) // "pending_transaction:{transaction}:" + 2×UUID + ":" + transactionID

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
//
// Deprecated: This per-transaction lock has been replaced by the cycle-level lock
// (RedisConsumerCycleLockKey). Retained for reference during rolling deployments
// where old pods may still hold per-transaction locks.
func RedisConsumerLockKey(organizationID, ledgerID uuid.UUID, transactionID string) string {
	var builder strings.Builder

	builder.Grow(96 + len(transactionID)) // "redis_consumer_lock:{" + 2×UUID + "}:" + transactionID

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

// RedisConsumerCycleLockKey returns the distributed lock key used for leader election
// in the Redis backup queue consumer. Only one pod acquires this lock per processing
// cycle, eliminating N×M SetNX calls (N pods × M messages) in favor of N×1.
// Format: "lock:{transactions}:backup-consumer-cycle"
// The {transactions} hash tag ensures the key routes to the correct Redis Cluster slot.
func RedisConsumerCycleLockKey() string {
	return "lock:{transactions}:backup-consumer-cycle"
}

// LedgerSettingsInternalKey returns a key with the following format to be used on redis cluster:
// "ledger_settings:{organizationID:ledgerID}"
func LedgerSettingsInternalKey(organizationID, ledgerID uuid.UUID) string {
	var builder strings.Builder

	builder.Grow(91) // "ledger_settings:{" + 2×UUID + ":}"

	builder.WriteString("ledger_settings")
	builder.WriteString(keySeparator)
	builder.WriteString(beginningKey)
	builder.WriteString(organizationID.String())
	builder.WriteString(keySeparator)
	builder.WriteString(ledgerID.String())
	builder.WriteString(endKey)

	return builder.String()
}

// WriteBehindTransactionKey returns a key with the following format to be used on redis cluster:
// "wb_transaction:{organizationID:ledgerID:transactionID}"
// This key is used to store transaction data in the write-behind cache before persistence.
// The transactionID is included inside the hash tag so keys distribute evenly across Redis Cluster
// slots. Co-location via {orgID:ledgerID} is not needed here because write-behind keys are always
// accessed individually (SET/GET/DEL), never in multi-key operations.
func WriteBehindTransactionKey(organizationID, ledgerID uuid.UUID, transactionID string) string {
	var builder strings.Builder

	builder.Grow(91 + len(transactionID)) // "wb_transaction:{" + 2×UUID + ":" + transactionID + "}"

	builder.WriteString("wb_transaction")
	builder.WriteString(keySeparator)
	builder.WriteString(beginningKey)
	builder.WriteString(organizationID.String())
	builder.WriteString(keySeparator)
	builder.WriteString(ledgerID.String())
	builder.WriteString(keySeparator)
	builder.WriteString(transactionID)
	builder.WriteString(endKey)

	return builder.String()
}
