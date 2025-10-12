package utils

import (
	"strings"

	"github.com/google/uuid"
)

// Redis key formatting constants used to build cluster-aware cache keys.
// The curly braces ensure all related keys hash to the same slot in Redis Cluster.
const (
	beginningKey = "{"
	keySeparator = ":"
	endKey       = "}"
)

// GenericInternalKey returns a key with the following format to be used on redis cluster:
// "name:{contextName}:organizationID:ledgerID:key"
func GenericInternalKey(name, contextName, organizationID, ledgerID, key string) string {
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
	transaction := GenericInternalKey("transaction", "transactions", organizationID.String(), ledgerID.String(), key)

	return transaction
}

// BalanceInternalKey returns a key with the following format to be used on redis cluster:
// "balance:{contextName}:organizationID:ledgerID:key"
func BalanceInternalKey(organizationID, ledgerID uuid.UUID, key string) string {
	balance := GenericInternalKey("balance", "transactions", organizationID.String(), ledgerID.String(), key)

	return balance
}
