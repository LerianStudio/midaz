package utils

import (
	"strings"
)

const beginningKey = "{"
const keySeparator = ":"
const endKey = "}"

// GenericInternalKey returns a key with the following format to be used on redis cluster:
// "name:{contextName}:key"
func GenericInternalKey(name, contextName, key string) string {
	var builder strings.Builder

	builder.WriteString(name)
	builder.WriteString(keySeparator)
	builder.WriteString(beginningKey)
	builder.WriteString(contextName)
	builder.WriteString(endKey)
	builder.WriteString(keySeparator)
	builder.WriteString(key)

	return builder.String()
}

// TransactionInternalKey returns a key with the following format to be used on redis cluster:
// "transaction:{contextName}:key"
func TransactionInternalKey(key string) string {
	transaction := GenericInternalKey("transaction", "transactions", key)

	return transaction
}

// BalanceInternalKey returns a key with the following format to be used on redis cluster:
// "balance:{contextName}:key"
func BalanceInternalKey(key string) string {
	balance := GenericInternalKey("balance", "transactions", key)

	return balance
}
