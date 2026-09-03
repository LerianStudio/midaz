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

// AccountExceptionsInternalKey returns a key with the following format to be used on redis cluster:
// "account_exceptions:{organizationID:ledgerID}:accountID"
//
// This is the first cache key owned by the account-block exception feature. It mirrors the
// IdempotencyReverseKey shape: the {organizationID:ledgerID} hash tag co-locates a ledger's
// exception keys on one Redis Cluster slot, while the accountID sits outside the tag as the
// per-account discriminator. No hash tag {transactions} is used because no Lua multi-key
// operation ever touches this key — it is only ever read, written or deleted individually.
func AccountExceptionsInternalKey(organizationID, ledgerID, accountID uuid.UUID) string {
	var builder strings.Builder

	builder.Grow(131) // "account_exceptions:{" + 2×UUID + "}:" + UUID

	builder.WriteString("account_exceptions")
	builder.WriteString(keySeparator)
	builder.WriteString(beginningKey)
	builder.WriteString(organizationID.String())
	builder.WriteString(keySeparator)
	builder.WriteString(ledgerID.String())
	builder.WriteString(endKey)
	builder.WriteString(keySeparator)
	builder.WriteString(accountID.String())

	return builder.String()
}

// BlockedAccountsHydratedMember is the reserved member that marks a
// blocked-accounts SET as fully hydrated from the source of truth
// (account.blocked in the onboarding PostgreSQL).
//
// It shares the SET with account IDs written as canonical uuid.UUID.String(),
// so the value is deliberately one that can never parse as a UUID: a member
// that did would be read as a blocked account and would block it forever.
//
// The sentinel is written LAST by the hydration path. A SET missing it is by
// definition partially hydrated and MUST be treated as "unknown", never as
// "nothing is blocked" — that is what makes the index fail-closed.
const BlockedAccountsHydratedMember = "__hydrated__"

// BlockedAccountsInternalKey returns a key with the following format to be used on redis cluster:
// "blocked_accounts:{transactions}:organizationID:ledgerID"
//
// The SET behind this key is the enforcement index for account blocking: a
// derived, reconstructible projection of account.blocked (the durable source of
// truth lives in the onboarding PostgreSQL). Members are account IDs in
// canonical uuid.UUID.String() form, plus the BlockedAccountsHydratedMember
// sentinel.
//
// The {transactions} hash tag is load-bearing, not cosmetic: the balance atomic
// Lua script reads this SET with SISMEMBER in the same invocation that mutates
// the balance keys of the ledger, and Redis Cluster only allows a script to span
// keys of one slot. A different tag would make that script invalid in cluster
// mode.
//
// This key carries NO TTL, by invariant. Expiring it would silently drop every
// blocked account from the index — a fail-OPEN unblock nobody requested. Only an
// explicit unblock (SREM) removes a member.
func BlockedAccountsInternalKey(organizationID, ledgerID uuid.UUID) string {
	var builder strings.Builder

	builder.Grow(105) // "blocked_accounts:{transactions}:" + 2×UUID + ":"

	builder.WriteString("blocked_accounts")
	builder.WriteString(keySeparator)
	builder.WriteString(beginningKey)
	builder.WriteString("transactions")
	builder.WriteString(endKey)
	builder.WriteString(keySeparator)
	builder.WriteString(organizationID.String())
	builder.WriteString(keySeparator)
	builder.WriteString(ledgerID.String())

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
