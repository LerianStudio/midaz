package utils

import (
	"strings"

	"github.com/google/uuid"
)

const beginningKey = "{"
const keySeparator = ":"
const endKey = "}"

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

// WriteBehindParentKey returns a key with the following format to be used on redis cluster:
// "wb_parent_transaction:{organizationID:ledgerID:parentTransactionID}"
// This key is used as an atomic lock to prevent duplicate reversals of the same transaction.
// The parentTransactionID is included inside the hash tag for even slot distribution (same rationale
// as WriteBehindTransactionKey — these keys are accessed individually, no multi-key operations).
func WriteBehindParentKey(organizationID, ledgerID uuid.UUID, parentTransactionID string) string {
	var builder strings.Builder

	builder.WriteString("wb_parent_transaction")
	builder.WriteString(keySeparator)
	builder.WriteString(beginningKey)
	builder.WriteString(organizationID.String())
	builder.WriteString(keySeparator)
	builder.WriteString(ledgerID.String())
	builder.WriteString(keySeparator)
	builder.WriteString(parentTransactionID)
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
