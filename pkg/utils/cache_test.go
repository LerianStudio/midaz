package utils

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
)

func TestGenericInternalKeyWithContext(t *testing.T) {
	tests := []struct {
		name           string
		keyName        string
		contextName    string
		organizationID string
		ledgerID       string
		key            string
		expected       string
	}{
		{
			name:           "standard transaction key",
			keyName:        "transaction",
			contextName:    "transactions",
			organizationID: "org-123",
			ledgerID:       "ledger-456",
			key:            "txn-789",
			expected:       "transaction:{transactions}:org-123:ledger-456:txn-789",
		},
		{
			name:           "balance key",
			keyName:        "balance",
			contextName:    "transactions",
			organizationID: "org-abc",
			ledgerID:       "ledger-def",
			key:            "bal-xyz",
			expected:       "balance:{transactions}:org-abc:ledger-def:bal-xyz",
		},
		{
			name:           "empty key component",
			keyName:        "test",
			contextName:    "ctx",
			organizationID: "",
			ledgerID:       "",
			key:            "",
			expected:       "test:{ctx}:::",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := GenericInternalKeyWithContext(tt.keyName, tt.contextName, tt.organizationID, tt.ledgerID, tt.key)
			assert.Equal(t, tt.expected, result)
		})
	}
}

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

func TestGenericInternalKey(t *testing.T) {
	tests := []struct {
		name           string
		keyName        string
		organizationID string
		ledgerID       string
		key            string
		expected       string
	}{
		{
			name:           "idempotency key",
			keyName:        "idempotency",
			organizationID: "org-123",
			ledgerID:       "ledger-456",
			key:            "idem-key",
			expected:       "idempotency:org-123:ledger-456:idem-key",
		},
		{
			name:           "accounting routes key",
			keyName:        "accounting_routes",
			organizationID: "org-abc",
			ledgerID:       "ledger-def",
			key:            "route-xyz",
			expected:       "accounting_routes:org-abc:ledger-def:route-xyz",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := GenericInternalKey(tt.keyName, tt.organizationID, tt.ledgerID, tt.key)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestIdempotencyInternalKey(t *testing.T) {
	orgID := uuid.MustParse("00000000-0000-0000-0000-000000000001")
	ledgerID := uuid.MustParse("00000000-0000-0000-0000-000000000002")

	result := IdempotencyInternalKey(orgID, ledgerID, "idem-123")

	expected := "idempotency:00000000-0000-0000-0000-000000000001:00000000-0000-0000-0000-000000000002:idem-123"
	assert.Equal(t, expected, result)
}

func TestAccountingRoutesInternalKey(t *testing.T) {
	orgID := uuid.MustParse("00000000-0000-0000-0000-000000000001")
	ledgerID := uuid.MustParse("00000000-0000-0000-0000-000000000002")
	routeID := uuid.MustParse("00000000-0000-0000-0000-000000000003")

	result := AccountingRoutesInternalKey(orgID, ledgerID, routeID)

	expected := "accounting_routes:00000000-0000-0000-0000-000000000001:00000000-0000-0000-0000-000000000002:00000000-0000-0000-0000-000000000003"
	assert.Equal(t, expected, result)
}

func TestRedisKeyConstants(t *testing.T) {
	// Verify constants are set to expected values
	assert.Equal(t, "schedule:{transactions}:balance-sync", BalanceSyncScheduleKey)
	assert.Equal(t, "lock:{transactions}:balance-sync:", BalanceSyncLockPrefix)
}
