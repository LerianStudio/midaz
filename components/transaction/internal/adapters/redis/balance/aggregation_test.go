// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package balance

import (
	"context"
	"sync"
	"testing"

	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
)

func TestBalanceCompositeKey_String(t *testing.T) {
	t.Parallel()

	orgID := uuid.MustParse("11111111-1111-1111-1111-111111111111")
	ledgerID := uuid.MustParse("22222222-2222-2222-2222-222222222222")

	tests := []struct {
		name     string
		key      BalanceCompositeKey
		expected string
	}{
		{
			name: "full composite key",
			key: BalanceCompositeKey{
				OrganizationID: orgID,
				LedgerID:       ledgerID,
				AccountID:      "acc-123",
				AssetCode:      "USD",
				PartitionKey:   "default",
			},
			expected: "11111111-1111-1111-1111-111111111111:22222222-2222-2222-2222-222222222222:acc-123:USD:default",
		},
		{
			name: "different partition",
			key: BalanceCompositeKey{
				OrganizationID: orgID,
				LedgerID:       ledgerID,
				AccountID:      "acc-456",
				AssetCode:      "BRL",
				PartitionKey:   "custom",
			},
			expected: "11111111-1111-1111-1111-111111111111:22222222-2222-2222-2222-222222222222:acc-456:BRL:custom",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			result := tt.key.String()
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestBalanceCompositeKeyFromRedisKey(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		redisKey    string
		wantOrgID   uuid.UUID
		wantLedger  uuid.UUID
		wantAccount string
		wantPartKey string
		wantErr     bool
	}{
		{
			name:        "valid key format with partition",
			redisKey:    "balance:{transactions}:11111111-1111-1111-1111-111111111111:22222222-2222-2222-2222-222222222222:@account#default",
			wantOrgID:   uuid.MustParse("11111111-1111-1111-1111-111111111111"),
			wantLedger:  uuid.MustParse("22222222-2222-2222-2222-222222222222"),
			wantAccount: "@account",
			wantPartKey: "default",
			wantErr:     false,
		},
		{
			name:        "valid key format without explicit partition",
			redisKey:    "balance:{transactions}:11111111-1111-1111-1111-111111111111:22222222-2222-2222-2222-222222222222:@account",
			wantOrgID:   uuid.MustParse("11111111-1111-1111-1111-111111111111"),
			wantLedger:  uuid.MustParse("22222222-2222-2222-2222-222222222222"),
			wantAccount: "@account",
			wantPartKey: "default",
			wantErr:     false,
		},
		{
			name:     "invalid format - too few parts",
			redisKey: "invalid-key",
			wantErr:  true,
		},
		{
			name:     "invalid organization ID",
			redisKey: "balance:{transactions}:not-a-uuid:22222222-2222-2222-2222-222222222222:@account#default",
			wantErr:  true,
		},
		{
			name:     "invalid ledger ID",
			redisKey: "balance:{transactions}:11111111-1111-1111-1111-111111111111:not-a-uuid:@account#default",
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			key, err := BalanceCompositeKeyFromRedisKey(tt.redisKey)

			if tt.wantErr {
				assert.Error(t, err)
				return
			}

			assert.NoError(t, err)
			assert.Equal(t, tt.wantOrgID, key.OrganizationID)
			assert.Equal(t, tt.wantLedger, key.LedgerID)
			assert.Equal(t, tt.wantAccount, key.AccountID)
			assert.Equal(t, tt.wantPartKey, key.PartitionKey)
		})
	}
}

func TestInMemoryAggregator_Aggregate(t *testing.T) {
	t.Parallel()

	orgID := uuid.MustParse("11111111-1111-1111-1111-111111111111")
	ledgerID := uuid.MustParse("22222222-2222-2222-2222-222222222222")

	tests := []struct {
		name             string
		input            []*AggregatedBalance
		expectedCount    int
		expectedVersions map[string]int64 // composite key -> expected version
	}{
		{
			name:          "empty input returns empty",
			input:         []*AggregatedBalance{},
			expectedCount: 0,
		},
		{
			name: "single balance unchanged",
			input: []*AggregatedBalance{
				{
					RedisKey: "key1",
					Balance: &mmodel.BalanceRedis{
						ID:        "bal-1",
						Alias:     "@acc1",
						AssetCode: "USD",
						Version:   5,
					},
					Key: BalanceCompositeKey{
						OrganizationID: orgID,
						LedgerID:       ledgerID,
						AccountID:      "@acc1",
						AssetCode:      "USD",
						PartitionKey:   "default",
					},
				},
			},
			expectedCount: 1,
			expectedVersions: map[string]int64{
				orgID.String() + ":" + ledgerID.String() + ":@acc1:USD:default": 5,
			},
		},
		{
			name: "same balance multiple versions keeps highest",
			input: []*AggregatedBalance{
				{
					RedisKey: "key1",
					Balance: &mmodel.BalanceRedis{
						ID:        "bal-1",
						Alias:     "@acc1",
						AssetCode: "USD",
						Version:   5,
					},
					Key: BalanceCompositeKey{
						OrganizationID: orgID,
						LedgerID:       ledgerID,
						AccountID:      "@acc1",
						AssetCode:      "USD",
						PartitionKey:   "default",
					},
				},
				{
					RedisKey: "key1",
					Balance: &mmodel.BalanceRedis{
						ID:        "bal-1",
						Alias:     "@acc1",
						AssetCode: "USD",
						Version:   10,
					},
					Key: BalanceCompositeKey{
						OrganizationID: orgID,
						LedgerID:       ledgerID,
						AccountID:      "@acc1",
						AssetCode:      "USD",
						PartitionKey:   "default",
					},
				},
				{
					RedisKey: "key1",
					Balance: &mmodel.BalanceRedis{
						ID:        "bal-1",
						Alias:     "@acc1",
						AssetCode: "USD",
						Version:   3,
					},
					Key: BalanceCompositeKey{
						OrganizationID: orgID,
						LedgerID:       ledgerID,
						AccountID:      "@acc1",
						AssetCode:      "USD",
						PartitionKey:   "default",
					},
				},
			},
			expectedCount: 1,
			expectedVersions: map[string]int64{
				orgID.String() + ":" + ledgerID.String() + ":@acc1:USD:default": 10,
			},
		},
		{
			name: "different balances kept separately",
			input: []*AggregatedBalance{
				{
					RedisKey: "key1",
					Balance: &mmodel.BalanceRedis{
						ID:        "bal-1",
						Alias:     "@acc1",
						AssetCode: "USD",
						Version:   5,
					},
					Key: BalanceCompositeKey{
						OrganizationID: orgID,
						LedgerID:       ledgerID,
						AccountID:      "@acc1",
						AssetCode:      "USD",
						PartitionKey:   "default",
					},
				},
				{
					RedisKey: "key2",
					Balance: &mmodel.BalanceRedis{
						ID:        "bal-2",
						Alias:     "@acc2",
						AssetCode: "USD",
						Version:   3,
					},
					Key: BalanceCompositeKey{
						OrganizationID: orgID,
						LedgerID:       ledgerID,
						AccountID:      "@acc2",
						AssetCode:      "USD",
						PartitionKey:   "default",
					},
				},
			},
			expectedCount: 2,
			expectedVersions: map[string]int64{
				orgID.String() + ":" + ledgerID.String() + ":@acc1:USD:default": 5,
				orgID.String() + ":" + ledgerID.String() + ":@acc2:USD:default": 3,
			},
		},
		{
			name: "nil balance entries skipped",
			input: []*AggregatedBalance{
				{
					RedisKey: "key1",
					Balance:  nil,
					Key: BalanceCompositeKey{
						OrganizationID: orgID,
						LedgerID:       ledgerID,
						AccountID:      "@acc1",
						AssetCode:      "USD",
						PartitionKey:   "default",
					},
				},
				{
					RedisKey: "key2",
					Balance: &mmodel.BalanceRedis{
						ID:        "bal-2",
						Alias:     "@acc2",
						AssetCode: "USD",
						Version:   3,
					},
					Key: BalanceCompositeKey{
						OrganizationID: orgID,
						LedgerID:       ledgerID,
						AccountID:      "@acc2",
						AssetCode:      "USD",
						PartitionKey:   "default",
					},
				},
			},
			expectedCount: 1,
		},
		{
			name: "nil AggregatedBalance entry skipped",
			input: []*AggregatedBalance{
				nil,
				{
					RedisKey: "key2",
					Balance: &mmodel.BalanceRedis{
						ID:        "bal-2",
						Alias:     "@acc2",
						AssetCode: "USD",
						Version:   3,
					},
					Key: BalanceCompositeKey{
						OrganizationID: orgID,
						LedgerID:       ledgerID,
						AccountID:      "@acc2",
						AssetCode:      "USD",
						PartitionKey:   "default",
					},
				},
			},
			expectedCount: 1,
		},
		{
			name: "populates AssetCode from balance data when key has empty AssetCode",
			input: []*AggregatedBalance{
				{
					RedisKey: "key1",
					Balance: &mmodel.BalanceRedis{
						ID:        "bal-1",
						Alias:     "@acc1",
						AssetCode: "USD",
						Version:   5,
					},
					Key: BalanceCompositeKey{
						OrganizationID: orgID,
						LedgerID:       ledgerID,
						AccountID:      "@acc1",
						AssetCode:      "", // Empty - should be populated from balance
						PartitionKey:   "default",
					},
				},
				{
					RedisKey: "key2",
					Balance: &mmodel.BalanceRedis{
						ID:        "bal-2",
						Alias:     "@acc1",
						AssetCode: "BRL",
						Version:   3,
					},
					Key: BalanceCompositeKey{
						OrganizationID: orgID,
						LedgerID:       ledgerID,
						AccountID:      "@acc1",
						AssetCode:      "", // Empty - should be populated from balance
						PartitionKey:   "default",
					},
				},
			},
			expectedCount: 2, // Different assets should NOT collide
			expectedVersions: map[string]int64{
				orgID.String() + ":" + ledgerID.String() + ":@acc1:USD:default": 5,
				orgID.String() + ":" + ledgerID.String() + ":@acc1:BRL:default": 3,
			},
		},
	}

	aggregator := NewInMemoryAggregator()

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctx := context.Background()
			result := aggregator.Aggregate(ctx, tt.input)

			assert.Len(t, result, tt.expectedCount)

			if tt.expectedVersions != nil {
				for _, ab := range result {
					expectedVersion, ok := tt.expectedVersions[ab.Key.String()]
					if ok {
						assert.Equal(t, expectedVersion, ab.Balance.Version,
							"version mismatch for key %s", ab.Key.String())
					}
				}
			}
		})
	}
}

func TestInMemoryAggregator_Aggregate_EqualVersions(t *testing.T) {
	t.Parallel()

	orgID := uuid.MustParse("11111111-1111-1111-1111-111111111111")
	ledgerID := uuid.MustParse("22222222-2222-2222-2222-222222222222")

	// Test that equal versions keep first encountered (documented behavior)
	input := []*AggregatedBalance{
		{
			RedisKey: "key1-first",
			Balance: &mmodel.BalanceRedis{
				ID:        "bal-first",
				Alias:     "@acc1",
				AssetCode: "USD",
				Version:   5,
			},
			Key: BalanceCompositeKey{
				OrganizationID: orgID,
				LedgerID:       ledgerID,
				AccountID:      "@acc1",
				AssetCode:      "USD",
				PartitionKey:   "default",
			},
		},
		{
			RedisKey: "key1-second",
			Balance: &mmodel.BalanceRedis{
				ID:        "bal-second",
				Alias:     "@acc1",
				AssetCode: "USD",
				Version:   5, // Same version
			},
			Key: BalanceCompositeKey{
				OrganizationID: orgID,
				LedgerID:       ledgerID,
				AccountID:      "@acc1",
				AssetCode:      "USD",
				PartitionKey:   "default",
			},
		},
	}

	aggregator := NewInMemoryAggregator()
	ctx := context.Background()
	result := aggregator.Aggregate(ctx, input)

	assert.Len(t, result, 1)
	// First encountered should be kept when versions are equal
	assert.Equal(t, "bal-first", result[0].Balance.ID)
	assert.Equal(t, "key1-first", result[0].RedisKey)
}

func TestInMemoryAggregator_Aggregate_DeterministicOrder(t *testing.T) {
	t.Parallel()

	orgID := uuid.MustParse("11111111-1111-1111-1111-111111111111")
	ledgerID := uuid.MustParse("22222222-2222-2222-2222-222222222222")

	// Create balances in random order
	input := []*AggregatedBalance{
		{
			RedisKey: "key-z",
			Balance: &mmodel.BalanceRedis{
				ID:        "bal-z",
				Alias:     "@acc-z",
				AssetCode: "USD",
				Version:   1,
			},
			Key: BalanceCompositeKey{
				OrganizationID: orgID,
				LedgerID:       ledgerID,
				AccountID:      "@acc-z",
				AssetCode:      "USD",
				PartitionKey:   "default",
			},
		},
		{
			RedisKey: "key-a",
			Balance: &mmodel.BalanceRedis{
				ID:        "bal-a",
				Alias:     "@acc-a",
				AssetCode: "USD",
				Version:   2,
			},
			Key: BalanceCompositeKey{
				OrganizationID: orgID,
				LedgerID:       ledgerID,
				AccountID:      "@acc-a",
				AssetCode:      "USD",
				PartitionKey:   "default",
			},
		},
		{
			RedisKey: "key-m",
			Balance: &mmodel.BalanceRedis{
				ID:        "bal-m",
				Alias:     "@acc-m",
				AssetCode: "USD",
				Version:   3,
			},
			Key: BalanceCompositeKey{
				OrganizationID: orgID,
				LedgerID:       ledgerID,
				AccountID:      "@acc-m",
				AssetCode:      "USD",
				PartitionKey:   "default",
			},
		},
	}

	aggregator := NewInMemoryAggregator()
	ctx := context.Background()

	// Run multiple times to verify deterministic order
	for i := 0; i < 10; i++ {
		result := aggregator.Aggregate(ctx, input)

		assert.Len(t, result, 3)
		// Should be sorted by composite key (account ID is the varying part)
		assert.Equal(t, "@acc-a", result[0].Key.AccountID, "first should be @acc-a")
		assert.Equal(t, "@acc-m", result[1].Key.AccountID, "second should be @acc-m")
		assert.Equal(t, "@acc-z", result[2].Key.AccountID, "third should be @acc-z")
	}
}

func TestInMemoryAggregator_Aggregate_ConcurrentSafety(t *testing.T) {
	t.Parallel()

	orgID := uuid.MustParse("11111111-1111-1111-1111-111111111111")
	ledgerID := uuid.MustParse("22222222-2222-2222-2222-222222222222")

	aggregator := NewInMemoryAggregator()

	// Create a shared input slice
	input := make([]*AggregatedBalance, 100)
	for i := 0; i < 100; i++ {
		input[i] = &AggregatedBalance{
			RedisKey: "key-" + uuid.New().String(),
			Balance: &mmodel.BalanceRedis{
				ID:        "bal-" + uuid.New().String(),
				Alias:     "@acc-" + uuid.New().String(),
				AssetCode: "USD",
				Version:   int64(i),
			},
			Key: BalanceCompositeKey{
				OrganizationID: orgID,
				LedgerID:       ledgerID,
				AccountID:      "@acc-" + uuid.New().String(),
				AssetCode:      "USD",
				PartitionKey:   "default",
			},
		}
	}

	// Run concurrent aggregations
	var wg sync.WaitGroup
	results := make([][]*AggregatedBalance, 10)

	for i := 0; i < 10; i++ {
		wg.Add(1)

		go func(idx int) {
			defer wg.Done()

			ctx := context.Background()
			results[idx] = aggregator.Aggregate(ctx, input)
		}(i)
	}

	wg.Wait()

	// Verify all goroutines completed without panic
	for i, result := range results {
		assert.NotNil(t, result, "result %d should not be nil", i)
		assert.Equal(t, 100, len(result), "result %d should have 100 elements", i)
	}
}

func TestInMemoryAggregator_Aggregate_LargeBatch(t *testing.T) {
	t.Parallel()

	orgID := uuid.MustParse("11111111-1111-1111-1111-111111111111")
	ledgerID := uuid.MustParse("22222222-2222-2222-2222-222222222222")

	// Create 150 unique balances
	input := make([]*AggregatedBalance, 150)
	for i := 0; i < 150; i++ {
		accountID := "@acc-" + uuid.New().String()
		input[i] = &AggregatedBalance{
			RedisKey: "key-" + accountID,
			Balance: &mmodel.BalanceRedis{
				ID:        "bal-" + accountID,
				Alias:     accountID,
				AssetCode: "USD",
				Version:   int64(i + 1),
			},
			Key: BalanceCompositeKey{
				OrganizationID: orgID,
				LedgerID:       ledgerID,
				AccountID:      accountID,
				AssetCode:      "USD",
				PartitionKey:   "default",
			},
		}
	}

	aggregator := NewInMemoryAggregator()
	ctx := context.Background()
	result := aggregator.Aggregate(ctx, input)

	assert.Len(t, result, 150, "should preserve all 150 unique balances")

	// Verify all balances are present and sorted
	for i := 1; i < len(result); i++ {
		assert.True(t, result[i-1].Key.String() < result[i].Key.String(),
			"results should be sorted by composite key")
	}
}
