// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package utils

import (
	"strings"
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

func TestAccountBlockExceptionInternalKey(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		organizationID uuid.UUID
		ledgerID       uuid.UUID
		exceptionID    uuid.UUID
		expected       string
	}{
		{
			name:           "standard exception key",
			organizationID: uuid.MustParse("550e8400-e29b-41d4-a716-446655440000"),
			ledgerID:       uuid.MustParse("6ba7b810-9dad-11d1-80b4-00c04fd430c8"),
			exceptionID:    uuid.MustParse("018f2c1e-6a3b-7c4d-8e5f-0a1b2c3d4e5f"),
			expected:       "account_block_exception:{transactions}:550e8400-e29b-41d4-a716-446655440000:6ba7b810-9dad-11d1-80b4-00c04fd430c8:018f2c1e-6a3b-7c4d-8e5f-0a1b2c3d4e5f",
		},
		{
			name:           "nil UUIDs (zero value)",
			organizationID: uuid.Nil,
			ledgerID:       uuid.Nil,
			exceptionID:    uuid.Nil,
			expected:       "account_block_exception:{transactions}:00000000-0000-0000-0000-000000000000:00000000-0000-0000-0000-000000000000:00000000-0000-0000-0000-000000000000",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tt.expected,
				AccountBlockExceptionInternalKey(tt.organizationID, tt.ledgerID, tt.exceptionID))
		})
	}
}

// TestAccountBlockExceptionInternalKey_SharesBalanceHashSlot locks the
// co-location invariant the transaction EVAL depends on: an exception key and
// the balance keys of the same ledger carry the IDENTICAL hash tag, so a
// multi-key EVAL over both is legal in Redis Cluster. A key builder that
// dropped or renamed the tag would route the exception to another slot and make
// the consumption script fail at runtime, in cluster mode only.
func TestAccountBlockExceptionInternalKey_SharesBalanceHashSlot(t *testing.T) {
	t.Parallel()

	orgID := uuid.MustParse("550e8400-e29b-41d4-a716-446655440000")
	ledgerID := uuid.MustParse("6ba7b810-9dad-11d1-80b4-00c04fd430c8")

	exceptionKey := AccountBlockExceptionInternalKey(orgID, ledgerID, uuid.New())
	balanceKey := BalanceInternalKey(orgID, ledgerID, "@fraud_account#default")

	assert.Equal(t, hashTagOf(t, balanceKey), hashTagOf(t, exceptionKey),
		"the exception key must share the balance keys' hash slot")
}

// hashTagOf extracts the "{...}" hash tag Redis Cluster keys a slot on.
func hashTagOf(t *testing.T, key string) string {
	t.Helper()

	start := strings.Index(key, "{")
	end := strings.Index(key, "}")

	if start < 0 || end < start {
		t.Fatalf("key %q carries no hash tag", key)
	}

	return key[start : end+1]
}
