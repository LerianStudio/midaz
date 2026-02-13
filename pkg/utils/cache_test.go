package utils

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
)

func TestIdempotencyReverseKey(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		organizationID uuid.UUID
		ledgerID       uuid.UUID
		transactionID  string
		expected       string
	}{
		{
			name:           "standard reverse key",
			organizationID: uuid.MustParse("550e8400-e29b-41d4-a716-446655440000"),
			ledgerID:       uuid.MustParse("6ba7b810-9dad-11d1-80b4-00c04fd430c8"),
			transactionID:  "tx-123",
			expected:       "idempotency_reverse:{550e8400-e29b-41d4-a716-446655440000:6ba7b810-9dad-11d1-80b4-00c04fd430c8}:tx-123",
		},
		{
			name:           "nil UUID (zero value)",
			organizationID: uuid.Nil,
			ledgerID:       uuid.Nil,
			transactionID:  "tx-456",
			expected:       "idempotency_reverse:{00000000-0000-0000-0000-000000000000:00000000-0000-0000-0000-000000000000}:tx-456",
		},
		{
			name:           "empty transaction ID",
			organizationID: uuid.MustParse("550e8400-e29b-41d4-a716-446655440000"),
			ledgerID:       uuid.MustParse("6ba7b810-9dad-11d1-80b4-00c04fd430c8"),
			transactionID:  "",
			expected:       "idempotency_reverse:{550e8400-e29b-41d4-a716-446655440000:6ba7b810-9dad-11d1-80b4-00c04fd430c8}:",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			result := IdempotencyReverseKey(tt.organizationID, tt.ledgerID, tt.transactionID)

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
			expected:       "idempotency:{550e8400-e29b-41d4-a716-446655440000:6ba7b810-9dad-11d1-80b4-00c04fd430c8:request-123}",
		},
		{
			name:           "nil UUID (zero value)",
			organizationID: uuid.Nil,
			ledgerID:       uuid.Nil,
			key:            "request-456",
			expected:       "idempotency:{00000000-0000-0000-0000-000000000000:00000000-0000-0000-0000-000000000000:request-456}",
		},
		{
			name:           "empty key",
			organizationID: uuid.MustParse("550e8400-e29b-41d4-a716-446655440000"),
			ledgerID:       uuid.MustParse("6ba7b810-9dad-11d1-80b4-00c04fd430c8"),
			key:            "",
			expected:       "idempotency:{550e8400-e29b-41d4-a716-446655440000:6ba7b810-9dad-11d1-80b4-00c04fd430c8:}",
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
			expected:       "accounting_routes:{550e8400-e29b-41d4-a716-446655440000:6ba7b810-9dad-11d1-80b4-00c04fd430c8:6ba7b811-9dad-11d1-80b4-00c04fd430c8}",
		},
		{
			name:           "nil UUID (zero value)",
			organizationID: uuid.Nil,
			ledgerID:       uuid.Nil,
			key:            uuid.Nil,
			expected:       "accounting_routes:{00000000-0000-0000-0000-000000000000:00000000-0000-0000-0000-000000000000:00000000-0000-0000-0000-000000000000}",
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

func TestPendingTransactionLockKey(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		organizationID uuid.UUID
		ledgerID       uuid.UUID
		transactionID  string
		expected       string
	}{
		{
			name:           "standard pending transaction lock key",
			organizationID: uuid.MustParse("550e8400-e29b-41d4-a716-446655440000"),
			ledgerID:       uuid.MustParse("6ba7b810-9dad-11d1-80b4-00c04fd430c8"),
			transactionID:  "tx-123",
			expected:       "pending_transaction:{transaction}:550e8400-e29b-41d4-a716-446655440000:6ba7b810-9dad-11d1-80b4-00c04fd430c8:tx-123",
		},
		{
			name:           "nil UUID (zero value)",
			organizationID: uuid.Nil,
			ledgerID:       uuid.Nil,
			transactionID:  "tx-456",
			expected:       "pending_transaction:{transaction}:00000000-0000-0000-0000-000000000000:00000000-0000-0000-0000-000000000000:tx-456",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			result := PendingTransactionLockKey(tt.organizationID, tt.ledgerID, tt.transactionID)

			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestRedisConsumerLockKey(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		organizationID uuid.UUID
		ledgerID       uuid.UUID
		transactionID  string
		expected       string
	}{
		{
			name:           "standard redis consumer lock key",
			organizationID: uuid.MustParse("550e8400-e29b-41d4-a716-446655440000"),
			ledgerID:       uuid.MustParse("6ba7b810-9dad-11d1-80b4-00c04fd430c8"),
			transactionID:  "tx-123",
			expected:       "redis_consumer_lock:{550e8400-e29b-41d4-a716-446655440000:6ba7b810-9dad-11d1-80b4-00c04fd430c8}:tx-123",
		},
		{
			name:           "nil UUID (zero value)",
			organizationID: uuid.Nil,
			ledgerID:       uuid.Nil,
			transactionID:  "tx-456",
			expected:       "redis_consumer_lock:{00000000-0000-0000-0000-000000000000:00000000-0000-0000-0000-000000000000}:tx-456",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			result := RedisConsumerLockKey(tt.organizationID, tt.ledgerID, tt.transactionID)

			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestBatchIdempotencyKey(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		organizationID uuid.UUID
		ledgerID       uuid.UUID
		key            string
		expected       string
	}{
		{
			name:           "standard batch idempotency key",
			organizationID: uuid.MustParse("550e8400-e29b-41d4-a716-446655440000"),
			ledgerID:       uuid.MustParse("6ba7b810-9dad-11d1-80b4-00c04fd430c8"),
			key:            "batch-request-123",
			expected:       "batch_idempotency:{550e8400-e29b-41d4-a716-446655440000:6ba7b810-9dad-11d1-80b4-00c04fd430c8:batch}:batch-request-123",
		},
		{
			name:           "nil UUID (zero value)",
			organizationID: uuid.Nil,
			ledgerID:       uuid.Nil,
			key:            "batch-request-456",
			expected:       "batch_idempotency:{00000000-0000-0000-0000-000000000000:00000000-0000-0000-0000-000000000000:batch}:batch-request-456",
		},
		{
			name:           "empty key",
			organizationID: uuid.MustParse("550e8400-e29b-41d4-a716-446655440000"),
			ledgerID:       uuid.MustParse("6ba7b810-9dad-11d1-80b4-00c04fd430c8"),
			key:            "",
			expected:       "batch_idempotency:{550e8400-e29b-41d4-a716-446655440000:6ba7b810-9dad-11d1-80b4-00c04fd430c8:batch}:",
		},
		{
			name:           "different tenants produce different keys",
			organizationID: uuid.MustParse("11111111-1111-1111-1111-111111111111"),
			ledgerID:       uuid.MustParse("22222222-2222-2222-2222-222222222222"),
			key:            "same-key",
			expected:       "batch_idempotency:{11111111-1111-1111-1111-111111111111:22222222-2222-2222-2222-222222222222:batch}:same-key",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			result := BatchIdempotencyKey(tt.organizationID, tt.ledgerID, tt.key)

			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestBatchIdempotencyKey_TenantIsolation(t *testing.T) {
	t.Parallel()

	// Verify that the same idempotency key for different tenants produces different internal keys
	org1 := uuid.MustParse("11111111-1111-1111-1111-111111111111")
	ledger1 := uuid.MustParse("22222222-2222-2222-2222-222222222222")

	org2 := uuid.MustParse("33333333-3333-3333-3333-333333333333")
	ledger2 := uuid.MustParse("44444444-4444-4444-4444-444444444444")

	sameKey := "same-idempotency-key"

	key1 := BatchIdempotencyKey(org1, ledger1, sameKey)
	key2 := BatchIdempotencyKey(org2, ledger2, sameKey)

	// Keys must be different to ensure tenant isolation
	assert.NotEqual(t, key1, key2, "Same idempotency key for different tenants must produce different internal keys")

	// Verify both contain the tenant-specific hash tag
	assert.Contains(t, key1, "{11111111-1111-1111-1111-111111111111:22222222-2222-2222-2222-222222222222:batch}")
	assert.Contains(t, key2, "{33333333-3333-3333-3333-333333333333:44444444-4444-4444-4444-444444444444:batch}")
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
