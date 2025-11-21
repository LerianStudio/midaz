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

// GenericInternalKey returns a key with the following format to be used without context (no redis cluster)
// "name:organizationID:ledgerID:key"
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

// IdempotencyInternalKey returns a key with the following format to be used on redis cluster:
// "idempotency:{organizationID:ledgerID:key}"
func IdempotencyInternalKey(organizationID, ledgerID uuid.UUID, key string) string {
	idempotency := GenericInternalKey("idempotency", organizationID.String(), ledgerID.String(), key)

	return idempotency
}

// AccountingRoutesInternalKey returns a key with the following format to be used on redis cluster:
// "accounting_routes:{organizationID:ledgerID:key}"
func AccountingRoutesInternalKey(organizationID, ledgerID, key uuid.UUID) string {
	accountingRoutes := GenericInternalKey("accounting_routes", organizationID.String(), ledgerID.String(), key.String())

	return accountingRoutes
}
