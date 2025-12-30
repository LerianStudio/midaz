package utils

import (
	"strings"

	"github.com/google/uuid"
)

const BalanceSyncScheduleKey = "schedule:{transactions}:balance-sync"
const BalanceSyncLockPrefix = "lock:{transactions}:balance-sync:"

const beginningKey = "{"
const keySeparator = ":"
const endKey = "}"

// GenericInternalKeyWithContext returns a key with the following format to be used on redis cluster:
// "name:{contextName}:organizationID:ledgerID:key"
func GenericInternalKeyWithContext(name, contextName, organizationID, ledgerID, key string) string {
	var builder strings.Builder

	builder.WriteString(name)
	builder.WriteString(keySeparator)
	builder.WriteString(beginningKey)
	builder.WriteString(contextName)
	builder.WriteString(endKey)
	builder.WriteString(keySeparator)
	builder.WriteString(organizationID)
	builder.WriteString(keySeparator)
	builder.WriteString(ledgerID)
	builder.WriteString(keySeparator)
	builder.WriteString(key)

	return builder.String()
}

// TransactionInternalKey returns a key with the following format to be used on redis cluster:
// "transaction:{contextName}:organizationID:ledgerID:key"
func TransactionInternalKey(organizationID, ledgerID uuid.UUID, key string) string {
	transaction := GenericInternalKeyWithContext("transaction", "transactions", organizationID.String(), ledgerID.String(), key)

	return transaction
}

// BalanceInternalKey returns a key with the following format to be used on redis cluster:
// "balance:{contextName}:organizationID:ledgerID:key"
func BalanceInternalKey(organizationID, ledgerID uuid.UUID, key string) string {
	balance := GenericInternalKeyWithContext("balance", "transactions", organizationID.String(), ledgerID.String(), key)

	return balance
}

// GenericInternalKey returns a key with the following format to be used on non-cluster Redis:
// "name:{organizationID}:{ledgerID}:{key}"
func GenericInternalKey(name, organizationID, ledgerID, key string) string {
	var builder strings.Builder

	builder.WriteString(name)
	builder.WriteString(keySeparator)
	builder.WriteString(organizationID)
	builder.WriteString(keySeparator)
	builder.WriteString(ledgerID)
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

// IdempotencyInternalKey returns a non-contextual key (no cluster):
// "idempotency:{organizationID}:{ledgerID}:{key}"
func IdempotencyInternalKey(organizationID, ledgerID uuid.UUID, key string) string {
	idempotency := GenericInternalKey("idempotency", organizationID.String(), ledgerID.String(), key)

	return idempotency
}

// AccountingRoutesInternalKey returns a non-contextual key (no cluster):
// "accounting_routes:{organizationID}:{ledgerID}:{key}"
func AccountingRoutesInternalKey(organizationID, ledgerID, key uuid.UUID) string {
	accountingRoutes := GenericInternalKey("accounting_routes", organizationID.String(), ledgerID.String(), key.String())

	return accountingRoutes
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
