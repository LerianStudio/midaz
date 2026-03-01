// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package utils

import (
	"fmt"
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

// --------------------------------------------------------------------------
// Shard-aware key function tests (Phase 2A)
// --------------------------------------------------------------------------.

func TestBalanceShardKey(t *testing.T) {
	t.Parallel()

	orgID := uuid.MustParse("550e8400-e29b-41d4-a716-446655440000")
	ledgerID := uuid.MustParse("6ba7b810-9dad-11d1-80b4-00c04fd430c8")

	tests := []struct {
		name     string
		shardID  int
		aliasKey string
		expected string
	}{
		{
			name:     "shard 0",
			shardID:  0,
			aliasKey: "alice#default",
			expected: "balance:{shard_0}:550e8400-e29b-41d4-a716-446655440000:6ba7b810-9dad-11d1-80b4-00c04fd430c8:alice#default",
		},
		{
			name:     "shard 7 (max in 8-shard cluster)",
			shardID:  7,
			aliasKey: "@external/USD#default",
			expected: "balance:{shard_7}:550e8400-e29b-41d4-a716-446655440000:6ba7b810-9dad-11d1-80b4-00c04fd430c8:@external/USD#default",
		},
		{
			name:     "empty aliasKey",
			shardID:  3,
			aliasKey: "",
			expected: "balance:{shard_3}:550e8400-e29b-41d4-a716-446655440000:6ba7b810-9dad-11d1-80b4-00c04fd430c8:",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			result := BalanceShardKey(tt.shardID, orgID, ledgerID, tt.aliasKey)

			assert.Equal(t, tt.expected, result)
			assert.Contains(t, result, "{shard_", "must contain shard hash tag")
		})
	}
}

func TestTransactionShardKey(t *testing.T) {
	t.Parallel()

	orgID := uuid.MustParse("550e8400-e29b-41d4-a716-446655440000")
	ledgerID := uuid.MustParse("6ba7b810-9dad-11d1-80b4-00c04fd430c8")

	tests := []struct {
		name          string
		shardID       int
		transactionID string
		expected      string
	}{
		{
			name:          "shard 0",
			shardID:       0,
			transactionID: "tx-abc",
			expected:      "transaction:{shard_0}:550e8400-e29b-41d4-a716-446655440000:6ba7b810-9dad-11d1-80b4-00c04fd430c8:tx-abc",
		},
		{
			name:          "shard 5",
			shardID:       5,
			transactionID: "550e8400-e29b-41d4-a716-446655440099",
			expected:      "transaction:{shard_5}:550e8400-e29b-41d4-a716-446655440000:6ba7b810-9dad-11d1-80b4-00c04fd430c8:550e8400-e29b-41d4-a716-446655440099",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			result := TransactionShardKey(tt.shardID, orgID, ledgerID, tt.transactionID)

			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestBackupQueueShardKey(t *testing.T) {
	t.Parallel()

	tests := []struct {
		shardID  int
		expected string
	}{
		{0, "backup_queue:{shard_0}"},
		{1, "backup_queue:{shard_1}"},
		{7, "backup_queue:{shard_7}"},
		{15, "backup_queue:{shard_15}"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tt.expected, BackupQueueShardKey(tt.shardID))
		})
	}
}

func TestBalanceSyncScheduleShardKey(t *testing.T) {
	t.Parallel()

	tests := []struct {
		shardID  int
		expected string
	}{
		{0, "schedule:{shard_0}:balance-sync"},
		{3, "schedule:{shard_3}:balance-sync"},
		{7, "schedule:{shard_7}:balance-sync"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tt.expected, BalanceSyncScheduleShardKey(tt.shardID))
		})
	}
}

func TestBalanceSyncLockShardPrefix(t *testing.T) {
	t.Parallel()

	tests := []struct {
		shardID  int
		expected string
	}{
		{0, "lock:{shard_0}:balance-sync:"},
		{4, "lock:{shard_4}:balance-sync:"},
		{7, "lock:{shard_7}:balance-sync:"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tt.expected, BalanceSyncLockShardPrefix(tt.shardID))
		})
	}
}

func TestShardKeyConsistency(t *testing.T) {
	t.Parallel()

	// All shard-aware keys for the same shardID must use the same hash tag.
	// This guarantees Redis Cluster co-locates them on the same node.
	orgID := uuid.MustParse("550e8400-e29b-41d4-a716-446655440000")
	ledgerID := uuid.MustParse("6ba7b810-9dad-11d1-80b4-00c04fd430c8")

	for shardID := 0; shardID < 8; shardID++ {
		balanceKey := BalanceShardKey(shardID, orgID, ledgerID, "alice#default")
		txKey := TransactionShardKey(shardID, orgID, ledgerID, "tx-123")
		backupKey := BackupQueueShardKey(shardID)
		scheduleKey := BalanceSyncScheduleShardKey(shardID)
		lockPrefix := BalanceSyncLockShardPrefix(shardID)

		tag := fmt.Sprintf("{shard_%d}", shardID)

		assert.Contains(t, balanceKey, tag, "balance key shard %d", shardID)
		assert.Contains(t, txKey, tag, "tx key shard %d", shardID)
		assert.Contains(t, backupKey, tag, "backup key shard %d", shardID)
		assert.Contains(t, scheduleKey, tag, "schedule key shard %d", shardID)
		assert.Contains(t, lockPrefix, tag, "lock prefix shard %d", shardID)
	}
}

func TestShardRoutingAndMigrationKeys(t *testing.T) {
	t.Parallel()

	orgID := uuid.MustParse("550e8400-e29b-41d4-a716-446655440000")
	ledgerID := uuid.MustParse("6ba7b810-9dad-11d1-80b4-00c04fd430c8")

	assert.Equal(t,
		"shard_routing:{550e8400-e29b-41d4-a716-446655440000:6ba7b810-9dad-11d1-80b4-00c04fd430c8}",
		ShardRoutingKey(orgID, ledgerID),
	)

	assert.Equal(t,
		"shard_routing_updates:{550e8400-e29b-41d4-a716-446655440000:6ba7b810-9dad-11d1-80b4-00c04fd430c8}",
		ShardRoutingUpdatesChannel(orgID, ledgerID),
	)

	assert.Equal(t,
		"migration:{550e8400-e29b-41d4-a716-446655440000:6ba7b810-9dad-11d1-80b4-00c04fd430c8}:@alice",
		MigrationLockKey(orgID, ledgerID, "@alice"),
	)
}

func TestNegativeShardIDClampedToZero(t *testing.T) {
	t.Parallel()

	orgID := uuid.MustParse("550e8400-e29b-41d4-a716-446655440000")
	ledgerID := uuid.MustParse("6ba7b810-9dad-11d1-80b4-00c04fd430c8")

	t.Run("BalanceShardKey clamps negative shardID", func(t *testing.T) {
		t.Parallel()

		result := BalanceShardKey(-1, orgID, ledgerID, "alice#default")
		expected := BalanceShardKey(0, orgID, ledgerID, "alice#default")

		assert.Equal(t, expected, result)
		assert.Contains(t, result, "{shard_0}")
		assert.NotContains(t, result, "{shard_-1}")
	})

	t.Run("TransactionShardKey clamps negative shardID", func(t *testing.T) {
		t.Parallel()

		result := TransactionShardKey(-5, orgID, ledgerID, "tx-abc")
		expected := TransactionShardKey(0, orgID, ledgerID, "tx-abc")

		assert.Equal(t, expected, result)
		assert.Contains(t, result, "{shard_0}")
	})

	t.Run("BackupQueueShardKey clamps negative shardID", func(t *testing.T) {
		t.Parallel()

		assert.Equal(t, "backup_queue:{shard_0}", BackupQueueShardKey(-3))
	})

	t.Run("BalanceSyncScheduleShardKey clamps negative shardID", func(t *testing.T) {
		t.Parallel()

		assert.Equal(t, "schedule:{shard_0}:balance-sync", BalanceSyncScheduleShardKey(-1))
	})

	t.Run("BalanceSyncLockShardPrefix clamps negative shardID", func(t *testing.T) {
		t.Parallel()

		assert.Equal(t, "lock:{shard_0}:balance-sync:", BalanceSyncLockShardPrefix(-2))
	})

	t.Run("ShardMetricsKey clamps negative shardID", func(t *testing.T) {
		t.Parallel()

		assert.Equal(t, "shard_metrics:{shard_0}", ShardMetricsKey(-10))
	})

	t.Run("ShardHotAccountsBucketKey clamps negative shardID", func(t *testing.T) {
		t.Parallel()

		assert.Equal(t, "shard_hot_accounts_bucket:{shard_0}", ShardHotAccountsBucketKey(-1))
	})

	t.Run("ShardRebalanceShardCooldownKey clamps negative shardID", func(t *testing.T) {
		t.Parallel()

		assert.Equal(t, "shard_rebalance:{shard_0}:cooldown", ShardRebalanceShardCooldownKey(-7))
	})

	t.Run("ShardIsolationSetKey clamps negative shardID", func(t *testing.T) {
		t.Parallel()

		assert.Equal(t, "shard_isolation:{shard_0}", ShardIsolationSetKey(-99))
	})
}

func TestShardRebalanceAndMetricsKeys(t *testing.T) {
	t.Parallel()

	orgID := uuid.MustParse("550e8400-e29b-41d4-a716-446655440000")
	ledgerID := uuid.MustParse("6ba7b810-9dad-11d1-80b4-00c04fd430c8")

	assert.Equal(t, "shard_metrics:{shard_3}", ShardMetricsKey(3))
	assert.Equal(t, "shard_hot_accounts_bucket:{shard_3}", ShardHotAccountsBucketKey(3))
	assert.Equal(t, "shard_rebalance:{global}:state", ShardRebalanceStateKey())
	assert.Equal(t, "shard_rebalance:{shard_2}:cooldown", ShardRebalanceShardCooldownKey(2))
	assert.Equal(t,
		"shard_rebalance_account:{550e8400-e29b-41d4-a716-446655440000:6ba7b810-9dad-11d1-80b4-00c04fd430c8}:@alice",
		ShardRebalanceAccountCooldownKey(orgID, ledgerID, "@alice"),
	)
	assert.Equal(t, "shard_isolation:{shard_4}", ShardIsolationSetKey(4))
	assert.Equal(t,
		"shard_isolation_account:{550e8400-e29b-41d4-a716-446655440000:6ba7b810-9dad-11d1-80b4-00c04fd430c8}:@alice",
		ShardIsolationAccountKey(orgID, ledgerID, "@alice"),
	)
}
