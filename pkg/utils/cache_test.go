package utils

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
)

func TestGenericInternalKeyWithContext(t *testing.T) {
	t.Parallel()

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
			name:           "standard key format",
			keyName:        "balance",
			contextName:    "transactions",
			organizationID: "org-123",
			ledgerID:       "ledger-456",
			key:            "account-789",
			expected:       "balance:{transactions}:org-123:ledger-456:account-789",
		},
		{
			name:           "transaction context",
			keyName:        "transaction",
			contextName:    "transactions",
			organizationID: "org-abc",
			ledgerID:       "ledger-def",
			key:            "tx-ghi",
			expected:       "transaction:{transactions}:org-abc:ledger-def:tx-ghi",
		},
		{
			name:           "with UUID strings",
			keyName:        "cache",
			contextName:    "mycontext",
			organizationID: "550e8400-e29b-41d4-a716-446655440000",
			ledgerID:       "6ba7b810-9dad-11d1-80b4-00c04fd430c8",
			key:            "6ba7b811-9dad-11d1-80b4-00c04fd430c8",
			expected:       "cache:{mycontext}:550e8400-e29b-41d4-a716-446655440000:6ba7b810-9dad-11d1-80b4-00c04fd430c8:6ba7b811-9dad-11d1-80b4-00c04fd430c8",
		},
		{
			name:           "empty key value",
			keyName:        "test",
			contextName:    "ctx",
			organizationID: "org",
			ledgerID:       "ledger",
			key:            "",
			expected:       "test:{ctx}:org:ledger:",
		},
		{
			name:           "all empty strings",
			keyName:        "",
			contextName:    "",
			organizationID: "",
			ledgerID:       "",
			key:            "",
			expected:       ":{}:::",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			result := GenericInternalKeyWithContext(tt.keyName, tt.contextName, tt.organizationID, tt.ledgerID, tt.key)

			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestTransactionInternalKey(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		organizationID uuid.UUID
		ledgerID       uuid.UUID
		key            string
		expected       string
	}{
		{
			name:           "standard transaction key",
			organizationID: uuid.MustParse("550e8400-e29b-41d4-a716-446655440000"),
			ledgerID:       uuid.MustParse("6ba7b810-9dad-11d1-80b4-00c04fd430c8"),
			key:            "tx-123",
			expected:       "transaction:{transactions}:550e8400-e29b-41d4-a716-446655440000:6ba7b810-9dad-11d1-80b4-00c04fd430c8:tx-123",
		},
		{
			name:           "nil UUID (zero value)",
			organizationID: uuid.Nil,
			ledgerID:       uuid.Nil,
			key:            "tx-456",
			expected:       "transaction:{transactions}:00000000-0000-0000-0000-000000000000:00000000-0000-0000-0000-000000000000:tx-456",
		},
		{
			name:           "empty key",
			organizationID: uuid.MustParse("550e8400-e29b-41d4-a716-446655440000"),
			ledgerID:       uuid.MustParse("6ba7b810-9dad-11d1-80b4-00c04fd430c8"),
			key:            "",
			expected:       "transaction:{transactions}:550e8400-e29b-41d4-a716-446655440000:6ba7b810-9dad-11d1-80b4-00c04fd430c8:",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			result := TransactionInternalKey(tt.organizationID, tt.ledgerID, tt.key)

			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestBalanceInternalKey(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		organizationID uuid.UUID
		ledgerID       uuid.UUID
		key            string
		expected       string
	}{
		{
			name:           "standard balance key",
			organizationID: uuid.MustParse("550e8400-e29b-41d4-a716-446655440000"),
			ledgerID:       uuid.MustParse("6ba7b810-9dad-11d1-80b4-00c04fd430c8"),
			key:            "account-123",
			expected:       "balance:{transactions}:550e8400-e29b-41d4-a716-446655440000:6ba7b810-9dad-11d1-80b4-00c04fd430c8:account-123",
		},
		{
			name:           "nil UUID (zero value)",
			organizationID: uuid.Nil,
			ledgerID:       uuid.Nil,
			key:            "account-456",
			expected:       "balance:{transactions}:00000000-0000-0000-0000-000000000000:00000000-0000-0000-0000-000000000000:account-456",
		},
		{
			name:           "empty key",
			organizationID: uuid.MustParse("550e8400-e29b-41d4-a716-446655440000"),
			ledgerID:       uuid.MustParse("6ba7b810-9dad-11d1-80b4-00c04fd430c8"),
			key:            "",
			expected:       "balance:{transactions}:550e8400-e29b-41d4-a716-446655440000:6ba7b810-9dad-11d1-80b4-00c04fd430c8:",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			result := BalanceInternalKey(tt.organizationID, tt.ledgerID, tt.key)

			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestGenericInternalKey(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		keyName        string
		organizationID string
		ledgerID       string
		key            string
		expected       string
	}{
		{
			name:           "standard non-cluster key",
			keyName:        "idempotency",
			organizationID: "org-123",
			ledgerID:       "ledger-456",
			key:            "request-789",
			expected:       "idempotency:org-123:ledger-456:request-789",
		},
		{
			name:           "accounting routes format",
			keyName:        "accounting_routes",
			organizationID: "550e8400-e29b-41d4-a716-446655440000",
			ledgerID:       "6ba7b810-9dad-11d1-80b4-00c04fd430c8",
			key:            "6ba7b811-9dad-11d1-80b4-00c04fd430c8",
			expected:       "accounting_routes:550e8400-e29b-41d4-a716-446655440000:6ba7b810-9dad-11d1-80b4-00c04fd430c8:6ba7b811-9dad-11d1-80b4-00c04fd430c8",
		},
		{
			name:           "empty key value",
			keyName:        "test",
			organizationID: "org",
			ledgerID:       "ledger",
			key:            "",
			expected:       "test:org:ledger:",
		},
		{
			name:           "all empty strings",
			keyName:        "",
			organizationID: "",
			ledgerID:       "",
			key:            "",
			expected:       ":::",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			result := GenericInternalKey(tt.keyName, tt.organizationID, tt.ledgerID, tt.key)

			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestIdempotencyInternalKey(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		organizationID uuid.UUID
		ledgerID       uuid.UUID
		key            string
		expected       string
	}{
		{
			name:           "standard idempotency key",
			organizationID: uuid.MustParse("550e8400-e29b-41d4-a716-446655440000"),
			ledgerID:       uuid.MustParse("6ba7b810-9dad-11d1-80b4-00c04fd430c8"),
			key:            "request-123",
			expected:       "idempotency:550e8400-e29b-41d4-a716-446655440000:6ba7b810-9dad-11d1-80b4-00c04fd430c8:request-123",
		},
		{
			name:           "nil UUID (zero value)",
			organizationID: uuid.Nil,
			ledgerID:       uuid.Nil,
			key:            "request-456",
			expected:       "idempotency:00000000-0000-0000-0000-000000000000:00000000-0000-0000-0000-000000000000:request-456",
		},
		{
			name:           "empty key",
			organizationID: uuid.MustParse("550e8400-e29b-41d4-a716-446655440000"),
			ledgerID:       uuid.MustParse("6ba7b810-9dad-11d1-80b4-00c04fd430c8"),
			key:            "",
			expected:       "idempotency:550e8400-e29b-41d4-a716-446655440000:6ba7b810-9dad-11d1-80b4-00c04fd430c8:",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			result := IdempotencyInternalKey(tt.organizationID, tt.ledgerID, tt.key)

			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestAccountingRoutesInternalKey(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		organizationID uuid.UUID
		ledgerID       uuid.UUID
		key            uuid.UUID
		expected       string
	}{
		{
			name:           "standard accounting routes key",
			organizationID: uuid.MustParse("550e8400-e29b-41d4-a716-446655440000"),
			ledgerID:       uuid.MustParse("6ba7b810-9dad-11d1-80b4-00c04fd430c8"),
			key:            uuid.MustParse("6ba7b811-9dad-11d1-80b4-00c04fd430c8"),
			expected:       "accounting_routes:550e8400-e29b-41d4-a716-446655440000:6ba7b810-9dad-11d1-80b4-00c04fd430c8:6ba7b811-9dad-11d1-80b4-00c04fd430c8",
		},
		{
			name:           "nil UUID (zero value)",
			organizationID: uuid.Nil,
			ledgerID:       uuid.Nil,
			key:            uuid.Nil,
			expected:       "accounting_routes:00000000-0000-0000-0000-000000000000:00000000-0000-0000-0000-000000000000:00000000-0000-0000-0000-000000000000",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			result := AccountingRoutesInternalKey(tt.organizationID, tt.ledgerID, tt.key)

			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestCacheKeyConstants(t *testing.T) {
	t.Parallel()

	t.Run("BalanceSyncScheduleKey format", func(t *testing.T) {
		t.Parallel()

		assert.Equal(t, "schedule:{transactions}:balance-sync", BalanceSyncScheduleKey)
	})

	t.Run("BalanceSyncLockPrefix format", func(t *testing.T) {
		t.Parallel()

		assert.Equal(t, "lock:{transactions}:balance-sync:", BalanceSyncLockPrefix)
	})
}
