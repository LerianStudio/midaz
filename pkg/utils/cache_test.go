package utils

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
)

func TestTransactionInternalKey(t *testing.T) {
	orgID := uuid.MustParse("00000000-0000-0000-0000-000000000001")
	ledgerID := uuid.MustParse("00000000-0000-0000-0000-000000000002")

	result := TransactionInternalKey(orgID, ledgerID, "my-key")

	expected := "transaction:{transactions}:00000000-0000-0000-0000-000000000001:00000000-0000-0000-0000-000000000002:my-key"
	assert.Equal(t, expected, result)
}

func TestBalanceInternalKey(t *testing.T) {
	orgID := uuid.MustParse("00000000-0000-0000-0000-000000000001")
	ledgerID := uuid.MustParse("00000000-0000-0000-0000-000000000002")

	result := BalanceInternalKey(orgID, ledgerID, "balance-key")

	expected := "balance:{transactions}:00000000-0000-0000-0000-000000000001:00000000-0000-0000-0000-000000000002:balance-key"
	assert.Equal(t, expected, result)
}

func TestIdempotencyInternalKey(t *testing.T) {
	orgID := uuid.MustParse("00000000-0000-0000-0000-000000000001")
	ledgerID := uuid.MustParse("00000000-0000-0000-0000-000000000002")

	result := IdempotencyInternalKey(orgID, ledgerID, "idem-123")

	expected := "idempotency:{00000000-0000-0000-0000-000000000001:00000000-0000-0000-0000-000000000002:idem-123}"
	assert.Equal(t, expected, result)
}

func TestAccountingRoutesInternalKey(t *testing.T) {
	orgID := uuid.MustParse("00000000-0000-0000-0000-000000000001")
	ledgerID := uuid.MustParse("00000000-0000-0000-0000-000000000002")
	routeID := uuid.MustParse("00000000-0000-0000-0000-000000000003")

	result := AccountingRoutesInternalKey(orgID, ledgerID, routeID)

	expected := "accounting_routes:{00000000-0000-0000-0000-000000000001:00000000-0000-0000-0000-000000000002:00000000-0000-0000-0000-000000000003}"
	assert.Equal(t, expected, result)
}

func TestRedisKeyConstants(t *testing.T) {
	// Verify constants are set to expected values
	assert.Equal(t, "schedule:{transactions}:balance-sync", BalanceSyncScheduleKey)
	assert.Equal(t, "lock:{transactions}:balance-sync:", BalanceSyncLockPrefix)
}
