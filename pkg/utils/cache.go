package utils

import (
	"strings"

	"github.com/google/uuid"
)

const beginningKey = "{"
const keySeparator = ":"
const endKey = "}"

// GenericInternalKey returns a key with the following format to be used on redis cluster:
// "name:{organizationID:ledgerID:key}"
func GenericInternalKey(name, organizationID, ledgerID, key string) string {
	var builder strings.Builder

	builder.WriteString(name)
	builder.WriteString(keySeparator)
	builder.WriteString(beginningKey)
	builder.WriteString(organizationID)
	builder.WriteString(keySeparator)
	builder.WriteString(ledgerID)
	builder.WriteString(endKey)
	builder.WriteString(keySeparator)
	builder.WriteString(key)

	return builder.String()
}

// TransactionInternalKey returns a key with the following format to be used on redis cluster:
// "transaction:{organizationID:ledgerID}:key"
func TransactionInternalKey(organizationID, ledgerID uuid.UUID, key string) string {
	transaction := GenericInternalKey("transaction", organizationID.String(), ledgerID.String(), key)

	return transaction
}

// BalanceInternalKey returns a key with the following format to be used on redis cluster:
// "balance:{organizationID:ledgerID}:key"
func BalanceInternalKey(organizationID, ledgerID, key string) string {
	balance := GenericInternalKey("balance", organizationID, ledgerID, key)

	return balance
}
