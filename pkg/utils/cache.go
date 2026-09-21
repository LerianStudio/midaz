// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package utils

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"

	"github.com/google/uuid"

	"github.com/LerianStudio/midaz/v4/internal/cachepolicy"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
)

const (
	BalanceSyncScheduleKey       = "schedule:" + cachepolicy.HashTag + ":balance-sync-v2"
	BalanceSyncScheduleKeyLegacy = "schedule:" + cachepolicy.HashTag + ":balance-sync"
	BalanceSyncLockPrefix        = "lock:" + cachepolicy.HashTag + ":balance-sync:"
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
	builder.WriteString(cachepolicy.HashTag)
	builder.WriteString(keySeparator)
	builder.WriteString(organizationID.String())
	builder.WriteString(keySeparator)
	builder.WriteString(ledgerID.String())
	builder.WriteString(keySeparator)
	builder.WriteString(key)

	return builder.String()
}

// TransactionApplyMarkerKey returns a key with the following format to be used on redis cluster:
// "transaction_apply_marker:{transactions}:organizationID:ledgerID:transactionID:STATUS"
//
// The key is the idempotency identity of ONE execution of the balance atomic script.
// Status is part of the identity because the same transaction reaches the script more
// than once legitimately: a pending create posts as PENDING and its commit posts as
// APPROVED under the SAME transaction id, so a key without the status would make the
// commit look like a replay of the create.
//
// Status is uppercased so the key is stable regardless of the casing the caller holds.
//
// The {transactions} hash tag is the SAME literal tag BalanceInternalKey uses, so the
// marker lands in the slot the balance keys already occupy and can be read and written
// inside the same multi-key EVAL that mutates them.
func TransactionApplyMarkerKey(organizationID, ledgerID uuid.UUID, transactionID, status string) string {
	var builder strings.Builder

	// "transaction_apply_marker:{transactions}:" + 2×UUID + 3×":" + id + status
	builder.Grow(115 + len(transactionID) + len(status))

	builder.WriteString("transaction_apply_marker")
	builder.WriteString(keySeparator)
	builder.WriteString(cachepolicy.HashTag)
	builder.WriteString(keySeparator)
	builder.WriteString(organizationID.String())
	builder.WriteString(keySeparator)
	builder.WriteString(ledgerID.String())
	builder.WriteString(keySeparator)
	builder.WriteString(transactionID)
	builder.WriteString(keySeparator)
	builder.WriteString(strings.ToUpper(status))

	return builder.String()
}

// OppositeTerminalStatus returns the terminal status that contradicts the given
// one: APPROVED for CANCELED and CANCELED for APPROVED. Any other status —
// PENDING, CREATED, NOTED — has no opposite and yields an empty string, because
// only the two terminal transitions of a pending can re-execute each other.
//
// Comparison is case-insensitive so callers holding either casing resolve the
// same pair, matching TransactionApplyMarkerKey's own uppercasing.
func OppositeTerminalStatus(status string) string {
	switch strings.ToUpper(status) {
	case constant.APPROVED:
		return constant.CANCELED
	case constant.CANCELED:
		return constant.APPROVED
	default:
		return ""
	}
}

// TransactionApplyMarkerOppositeKey returns the apply marker key of the terminal
// status that contradicts the given one, or an empty string when the status has
// no opposite. The key is the evidence a commit uses to detect that the same
// transaction was already canceled, and vice versa.
func TransactionApplyMarkerOppositeKey(organizationID, ledgerID uuid.UUID, transactionID, status string) string {
	opposite := OppositeTerminalStatus(status)
	if opposite == "" {
		return ""
	}

	return TransactionApplyMarkerKey(organizationID, ledgerID, transactionID, opposite)
}

// BalanceInternalKey returns a key with the following format to be used on redis cluster:
// "balance:{transactions}:organizationID:ledgerID:key"
func BalanceInternalKey(organizationID, ledgerID uuid.UUID, key string) string {
	var builder strings.Builder

	builder.Grow(97 + len(key)) // "balance:{transactions}:" + 2×UUID + ":" + key

	builder.WriteString("balance")
	builder.WriteString(keySeparator)
	builder.WriteString(cachepolicy.HashTag)
	builder.WriteString(keySeparator)
	builder.WriteString(organizationID.String())
	builder.WriteString(keySeparator)
	builder.WriteString(ledgerID.String())
	builder.WriteString(keySeparator)
	builder.WriteString(key)

	return builder.String()
}

// AccountBlockExceptionInternalKey returns a key with the following format to be used on redis cluster:
// "account_block_exception:{transactions}:organizationID:ledgerID:exceptionID"
//
// The {transactions} hash tag is the SAME literal tag BalanceInternalKey uses, so an
// exception key and the balance keys of any account land in one Redis Cluster slot.
// That co-location is load-bearing: the exception is validated and deleted inside the
// same multi-key EVAL that mutates the balances, and a cross-slot EVAL is illegal.
func AccountBlockExceptionInternalKey(organizationID, ledgerID, exceptionID uuid.UUID) string {
	var builder strings.Builder

	builder.Grow(151) // "account_block_exception:{transactions}:" + 3×UUID + 2×":"

	builder.WriteString("account_block_exception")
	builder.WriteString(keySeparator)
	builder.WriteString(beginningKey)
	builder.WriteString("transactions")
	builder.WriteString(endKey)
	builder.WriteString(keySeparator)
	builder.WriteString(organizationID.String())
	builder.WriteString(keySeparator)
	builder.WriteString(ledgerID.String())
	builder.WriteString(keySeparator)
	builder.WriteString(exceptionID.String())

	return builder.String()
}

// AccountClosingMarkerKey returns a key with the following format to be used on redis cluster:
// "account-closing:{transactions}:organizationID:ledgerID:accountID"
//
// The marker holds the token of the administrative attempt that is deciding or
// finalizing a closing. It carries no releasing TTL: an attempt whose outcome is
// unknown must keep the account protected until the write is resolved, and an
// expiry would hand that decision to the clock.
//
// The {transactions} hash tag is the SAME literal tag BalanceInternalKey uses, so
// the marker of an account lands in the slot its balance keys already occupy.
func AccountClosingMarkerKey(organizationID, ledgerID, accountID uuid.UUID) string {
	return accountProtectionKey("account-closing", organizationID, ledgerID, accountID)
}

// AccountClosedMarkerKey returns a key with the following format to be used on redis cluster:
// "account-closed:{transactions}:organizationID:ledgerID:accountID"
//
// The marker holds the confirmed closing instant and is a NEGATIVE cache only:
// PostgreSQL stays the source of truth, so its expiry never reopens anything. It
// spares the authoritative read for an account that was already proven closed.
func AccountClosedMarkerKey(organizationID, ledgerID, accountID uuid.UUID) string {
	return accountProtectionKey("account-closed", organizationID, ledgerID, accountID)
}

// AccountAdminOwnershipKey returns a key with the following format to be used on redis cluster:
// "account-admin-ownership:{transactions}:organizationID:ledgerID:accountID"
//
// The key holds the token of the administrative operation that currently owns the
// account: a closing, a balance creation or deletion, or a cache-miss admission.
// It is the coordination point those operations share so a closing validates a
// stable balance list, and it exists only while an operation is active or awaiting
// reconciliation.
func AccountAdminOwnershipKey(organizationID, ledgerID, accountID uuid.UUID) string {
	return accountProtectionKey("account-admin-ownership", organizationID, ledgerID, accountID)
}

// accountProtectionKey assembles one account-scoped protection key. The scope is
// always complete — organization, ledger and account — so a key can never be
// resolved from the account alone, and it never carries a balance alias suffix.
func accountProtectionKey(namespace string, organizationID, ledgerID, accountID uuid.UUID) string {
	var builder strings.Builder

	// namespace + ":" + "{transactions}" + 3×UUID + 3×":"
	builder.Grow(len(namespace) + 126)

	builder.WriteString(namespace)
	builder.WriteString(keySeparator)
	builder.WriteString(cachepolicy.HashTag)
	builder.WriteString(keySeparator)
	builder.WriteString(organizationID.String())
	builder.WriteString(keySeparator)
	builder.WriteString(ledgerID.String())
	builder.WriteString(keySeparator)
	builder.WriteString(accountID.String())

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

// AtomicTransactionBatchIdempotencyInternalKey returns the private Redis key
// used by atomic transaction batches. The effective client key is always hashed
// before entering the namespace, so raw idempotency keys never become Redis key
// material. The batch discriminator keeps this format isolated from singular
// transaction idempotency. Organization and ledger remain explicit path
// components while the shared transaction hash tag co-locates batch state with
// accounting receipt/recovery evidence for cluster-safe refusal cleanup.
func AtomicTransactionBatchIdempotencyInternalKey(organizationID, ledgerID uuid.UUID, effectiveKey string) string {
	digest := sha256.Sum256([]byte(effectiveKey))
	hashedKey := hex.EncodeToString(digest[:])

	return AtomicTransactionBatchIdempotencyInternalKeyPrefix(organizationID, ledgerID) + hashedKey
}

// AtomicTransactionBatchIdempotencyInternalKeyPrefix returns the scoped prefix
// shared by batch idempotency records. It is exposed for strict execution-index
// pointer validation; callers must append exactly one lowercase SHA-256 digest.
func AtomicTransactionBatchIdempotencyInternalKeyPrefix(organizationID, ledgerID uuid.UUID) string {
	var builder strings.Builder
	builder.Grow(117) // "idempotency_atomic_batch:{transactions}:" + 2×UUID + ":"

	builder.WriteString("idempotency_atomic_batch")
	builder.WriteString(keySeparator)
	builder.WriteString(cachepolicy.HashTag)
	builder.WriteString(keySeparator)
	builder.WriteString(organizationID.String())
	builder.WriteString(keySeparator)
	builder.WriteString(ledgerID.String())
	builder.WriteString(keySeparator)

	return builder.String()
}

// AtomicTransactionBatchExecutionIndexInternalKey maps one accounting
// execution to its batch idempotency record. Its hash tag matches the record
// key so handoff and finalization can update both atomically in Redis Cluster.
func AtomicTransactionBatchExecutionIndexInternalKey(organizationID, ledgerID, executionID uuid.UUID) string {
	var builder strings.Builder
	builder.Grow(154) // "idempotency_atomic_batch_execution:{transactions}:" + 3×UUID

	builder.WriteString("idempotency_atomic_batch_execution")
	builder.WriteString(keySeparator)
	builder.WriteString(cachepolicy.HashTag)
	builder.WriteString(keySeparator)
	builder.WriteString(organizationID.String())
	builder.WriteString(keySeparator)
	builder.WriteString(ledgerID.String())
	builder.WriteString(keySeparator)
	builder.WriteString(executionID.String())

	return builder.String()
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
	return "lock:" + cachepolicy.HashTag + ":backup-consumer-cycle"
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
