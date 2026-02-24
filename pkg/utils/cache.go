// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package utils

import (
	"strings"

	"github.com/google/uuid"
)

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
// "transaction:{transactions}:organizationID:ledgerID:key"
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
// "balance:{transactions}:organizationID:ledgerID:key"
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
// "idempotency:{organizationID:ledgerID:key}"
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
// "accounting_routes:{organizationID:ledgerID:key}"
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

// WriteBehindTransactionKey returns a key with the following format to be used on redis cluster:
// "wb_transaction:{organizationID:ledgerID:transactionID}"
// This key is used to store transaction data in the write-behind cache before persistence.
// The transactionID is included inside the hash tag so keys distribute evenly across Redis Cluster
// slots. Co-location via {orgID:ledgerID} is not needed here because write-behind keys are always
// accessed individually (SET/GET/DEL), never in multi-key operations.
func WriteBehindTransactionKey(organizationID, ledgerID uuid.UUID, transactionID string) string {
	var builder strings.Builder

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
