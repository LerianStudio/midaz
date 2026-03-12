// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package balance

import (
	"context"
	"fmt"
	"sort"
	"strings"

	libCommons "github.com/LerianStudio/lib-commons/v3/commons"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/google/uuid"
)

// BalanceCompositeKey uniquely identifies a balance for aggregation purposes.
// This key is used to deduplicate balance updates by grouping them.
type BalanceCompositeKey struct {
	OrganizationID uuid.UUID
	LedgerID       uuid.UUID
	AccountID      string
	AssetCode      string
	PartitionKey   string
}

// String returns a string representation of the composite key.
// Format: "orgID:ledgerID:accountID:assetCode:partitionKey"
func (k BalanceCompositeKey) String() string {
	return fmt.Sprintf("%s:%s:%s:%s:%s",
		k.OrganizationID.String(),
		k.LedgerID.String(),
		k.AccountID,
		k.AssetCode,
		k.PartitionKey,
	)
}

// BalanceCompositeKeyFromRedisKey extracts a composite key from a Redis balance key.
// Redis key format: "balance:{transactions}:orgID:ledgerID:alias#partitionKey"
func BalanceCompositeKeyFromRedisKey(redisKey string) (BalanceCompositeKey, error) {
	// Expected format: balance:{transactions}:orgID:ledgerID:alias#key
	parts := strings.Split(redisKey, ":")
	if len(parts) < 5 {
		return BalanceCompositeKey{}, fmt.Errorf("invalid redis key format: expected 5 parts, got %d", len(parts))
	}

	// Parts: [balance, {transactions}, orgID, ledgerID, alias#key]
	orgID, err := uuid.Parse(parts[2])
	if err != nil {
		return BalanceCompositeKey{}, fmt.Errorf("invalid organization ID at position 2: %w", err)
	}

	ledgerID, err := uuid.Parse(parts[3])
	if err != nil {
		return BalanceCompositeKey{}, fmt.Errorf("invalid ledger ID at position 3: %w", err)
	}

	// The last part contains alias#partitionKey
	aliasAndKey := parts[4]
	aliasParts := strings.Split(aliasAndKey, "#")

	alias := aliasParts[0]
	partitionKey := "default"

	if len(aliasParts) > 1 {
		partitionKey = aliasParts[1]
	}

	return BalanceCompositeKey{
		OrganizationID: orgID,
		LedgerID:       ledgerID,
		AccountID:      alias,
		AssetCode:      "", // Will be populated from balance data
		PartitionKey:   partitionKey,
	}, nil
}

// AggregatedBalance holds a balance with its Redis key for batch operations.
type AggregatedBalance struct {
	RedisKey string
	Balance  *mmodel.BalanceRedis
	Key      BalanceCompositeKey
}

// BalanceAggregator defines the interface for aggregating balance updates.
type BalanceAggregator interface {
	// Aggregate takes a slice of balances and returns deduplicated balances
	// with only the highest version per composite key.
	// The result is sorted by composite key for deterministic output.
	Aggregate(ctx context.Context, balances []*AggregatedBalance) []*AggregatedBalance
}

// InMemoryAggregator implements BalanceAggregator using in-memory map.
// It groups balances by composite key and retains only the highest version.
type InMemoryAggregator struct{}

// NewInMemoryAggregator creates a new in-memory balance aggregator.
func NewInMemoryAggregator() *InMemoryAggregator {
	return &InMemoryAggregator{}
}

// Aggregate groups balances by composite key and returns only the highest version per key.
// The result is sorted by composite key string for deterministic output.
//
// Algorithm:
// 1. For each balance, compute its composite key (populating AssetCode from balance data)
// 2. If key not seen, store balance
// 3. If key seen, compare versions and keep higher (equal versions keep first encountered)
// 4. Return deduplicated list sorted by composite key
//
// Note: This method intentionally mutates the input AggregatedBalance.Key.AssetCode field
// when it is empty and the balance data contains the asset code. This avoids allocation
// overhead from cloning and is safe because the input balances are not reused after aggregation.
//
// Time complexity: O(n + m log m) where n is input size and m is unique keys
// Space complexity: O(m) where m is number of unique composite keys
func (a *InMemoryAggregator) Aggregate(ctx context.Context, balances []*AggregatedBalance) []*AggregatedBalance {
	//nolint:dogsled // standard pattern used throughout codebase
	_, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	_, span := tracer.Start(ctx, "aggregator.aggregate")
	defer span.End()

	if len(balances) == 0 {
		return []*AggregatedBalance{}
	}

	// Map: composite key string -> AggregatedBalance with highest version
	grouped := make(map[string]*AggregatedBalance, len(balances))

	for _, ab := range balances {
		// Skip nil balances (may occur if Redis key expired between schedule read and value fetch)
		if ab == nil || ab.Balance == nil {
			continue
		}

		// Populate AssetCode from balance data if not already set.
		// This intentionally mutates the input to avoid cloning overhead.
		// Safe because input balances are not reused after aggregation.
		if ab.Key.AssetCode == "" && ab.Balance.AssetCode != "" {
			ab.Key.AssetCode = ab.Balance.AssetCode
		}

		keyStr := ab.Key.String()

		existing, found := grouped[keyStr]
		if !found {
			// First time seeing this key
			grouped[keyStr] = ab
			continue
		}

		// Keep the one with higher version (equal versions keep first encountered)
		if ab.Balance.Version > existing.Balance.Version {
			grouped[keyStr] = ab
		}
	}

	// Convert map to slice
	result := make([]*AggregatedBalance, 0, len(grouped))
	for _, ab := range grouped {
		result = append(result, ab)
	}

	// Sort by composite key for deterministic output
	sort.Slice(result, func(i, j int) bool {
		return result[i].Key.String() < result[j].Key.String()
	})

	return result
}

// Ensure InMemoryAggregator implements BalanceAggregator at compile time
var _ BalanceAggregator = (*InMemoryAggregator)(nil)
