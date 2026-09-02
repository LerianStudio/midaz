// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

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

func TestLedgerSettingsInternalKey(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		organizationID uuid.UUID
		ledgerID       uuid.UUID
		expected       string
	}{
		{
			name:           "standard ledger settings key",
			organizationID: uuid.MustParse("11111111-1111-1111-1111-111111111111"),
			ledgerID:       uuid.MustParse("22222222-2222-2222-2222-222222222222"),
			expected:       "ledger_settings:{11111111-1111-1111-1111-111111111111:22222222-2222-2222-2222-222222222222}",
		},
		{
			name:           "nil UUID (zero value)",
			organizationID: uuid.Nil,
			ledgerID:       uuid.Nil,
			expected:       "ledger_settings:{00000000-0000-0000-0000-000000000000:00000000-0000-0000-0000-000000000000}",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			result := LedgerSettingsInternalKey(tt.organizationID, tt.ledgerID)

			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestRedisConsumerCycleLockKey(t *testing.T) {
	t.Parallel()

	result := RedisConsumerCycleLockKey()

	assert.Equal(t, "lock:{transactions}:backup-consumer-cycle", result,
		"cycle lock key must use {transactions} hash tag for Redis Cluster routing")

	// Call twice to ensure deterministic output (no randomness).
	assert.Equal(t, result, RedisConsumerCycleLockKey(),
		"cycle lock key must be deterministic across calls")
}

func TestCacheKeyConstants(t *testing.T) {
	t.Parallel()

	t.Run("BalanceSyncScheduleKey format", func(t *testing.T) {
		t.Parallel()

		assert.Equal(t, "schedule:{transactions}:balance-sync-v2", BalanceSyncScheduleKey)
		assert.Equal(t, "schedule:{transactions}:balance-sync", BalanceSyncScheduleKeyLegacy)
	})

	t.Run("BalanceSyncLockPrefix format", func(t *testing.T) {
		t.Parallel()

		assert.Equal(t, "lock:{transactions}:balance-sync:", BalanceSyncLockPrefix)
	})
}

func TestAccountExceptionsInternalKey(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		organizationID uuid.UUID
		ledgerID       uuid.UUID
		accountID      uuid.UUID
		expected       string
	}{
		{
			name:           "standard account exceptions key",
			organizationID: uuid.MustParse("550e8400-e29b-41d4-a716-446655440000"),
			ledgerID:       uuid.MustParse("6ba7b810-9dad-11d1-80b4-00c04fd430c8"),
			accountID:      uuid.MustParse("01965ed9-7fa4-75b2-8872-fc9e8509ab0a"),
			expected:       "account_exceptions:{550e8400-e29b-41d4-a716-446655440000:6ba7b810-9dad-11d1-80b4-00c04fd430c8}:01965ed9-7fa4-75b2-8872-fc9e8509ab0a",
		},
		{
			name:           "nil UUIDs (zero value)",
			organizationID: uuid.Nil,
			ledgerID:       uuid.Nil,
			accountID:      uuid.Nil,
			expected:       "account_exceptions:{00000000-0000-0000-0000-000000000000:00000000-0000-0000-0000-000000000000}:00000000-0000-0000-0000-000000000000",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			result := AccountExceptionsInternalKey(tt.organizationID, tt.ledgerID, tt.accountID)

			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestAccountExceptionsInternalKey_HashTagCoLocation asserts the {orgID:ledgerID}
// hash tag frames the ledger scope so a ledger's exception keys share one Redis
// Cluster slot, and that the accountID sits OUTSIDE the tag (the per-account
// discriminator), mirroring IdempotencyReverseKey.
func TestAccountExceptionsInternalKey_HashTagCoLocation(t *testing.T) {
	t.Parallel()

	organizationID := uuid.MustParse("550e8400-e29b-41d4-a716-446655440000")
	ledgerID := uuid.MustParse("6ba7b810-9dad-11d1-80b4-00c04fd430c8")
	accountA := uuid.MustParse("01965ed9-7fa4-75b2-8872-fc9e8509ab0a")
	accountB := uuid.MustParse("01965ed9-7fa4-75b2-8872-fc9e8509abff")

	keyA := AccountExceptionsInternalKey(organizationID, ledgerID, accountA)
	keyB := AccountExceptionsInternalKey(organizationID, ledgerID, accountB)

	tagA := keyA[len("account_exceptions:") : len(keyA)-len(accountA.String())-1]
	tagB := keyB[len("account_exceptions:") : len(keyB)-len(accountB.String())-1]

	assert.Equal(t, "{550e8400-e29b-41d4-a716-446655440000:6ba7b810-9dad-11d1-80b4-00c04fd430c8}", tagA)
	assert.Equal(t, tagA, tagB, "same ledger scope must share one hash tag (one slot)")
}
