// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package shardrouting

import (
	"context"
	"testing"

	"github.com/LerianStudio/midaz/v3/pkg/shard"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestResolveBalanceShard(t *testing.T) {
	t.Parallel()

	orgID := uuid.New()
	ledgerID := uuid.New()

	tests := []struct {
		name       string
		router     *shard.Router
		alias      string
		balanceKey string
		wantErr    bool
		errContain string
		wantShard  int
	}{
		{
			name:       "both nil returns zero and no error",
			router:     nil,
			alias:      "@user_1",
			balanceKey: "default",
			wantErr:    false,
			wantShard:  0,
		},
		{
			name:       "zero-value router returns invalid shard count error",
			router:     &shard.Router{},
			alias:      "@user_1",
			balanceKey: "default",
			wantErr:    true,
			errContain: "invalid shard count",
		},
		{
			name:       "router only falls back to ResolveBalance for regular alias",
			router:     shard.NewRouter(4),
			alias:      "@user_1",
			balanceKey: "default",
			wantErr:    false,
		},
		{
			name:       "router only resolves external alias with presplit key",
			router:     shard.NewRouter(8),
			alias:      "@external/USD",
			balanceKey: "shard_3",
			wantErr:    false,
			wantShard:  3,
		},
		{
			name:       "router only with regular alias returns hash-based shard",
			router:     shard.NewRouter(8),
			alias:      "@alice",
			balanceKey: "default",
			wantErr:    false,
		},
		{
			name:       "router only with external alias and default key falls back to hash",
			router:     shard.NewRouter(8),
			alias:      "@external/BTC",
			balanceKey: "default",
			wantErr:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			gotShard, err := ResolveBalanceShard(
				context.Background(),
				tt.router,
				nil, // manager is nil for these router-only tests
				orgID, ledgerID,
				tt.alias, tt.balanceKey,
			)

			if tt.wantErr {
				require.Error(t, err)
				if tt.errContain != "" {
					require.Contains(t, err.Error(), tt.errContain)
				}

				return
			}

			require.NoError(t, err)

			// For tests that specify an exact expected shard, check it
			if tt.wantShard != 0 || (tt.router == nil) {
				require.Equal(t, tt.wantShard, gotShard)
			}

			// For router-only fallback cases, verify consistency with Router.ResolveBalance
			if tt.router != nil && tt.wantShard == 0 && !tt.wantErr {
				expected := tt.router.ResolveBalance(tt.alias, tt.balanceKey)
				require.Equal(t, expected, gotShard,
					"ResolveBalanceShard should match Router.ResolveBalance for alias=%q key=%q",
					tt.alias, tt.balanceKey,
				)
			}
		})
	}
}

func TestResolveBalanceShard_BothNil(t *testing.T) {
	t.Parallel()

	gotShard, err := ResolveBalanceShard(
		context.Background(),
		nil, nil,
		uuid.New(), uuid.New(),
		"@user_1", "default",
	)

	require.NoError(t, err)
	require.Equal(t, 0, gotShard, "both nil should return shard 0")
}

func TestResolveBalanceShard_ZeroValueRouter_InvalidShardCount(t *testing.T) {
	t.Parallel()

	// A zero-value Router{} has shardCount == 0, triggering the early guard.
	zeroRouter := &shard.Router{}
	require.Equal(t, 0, zeroRouter.ShardCount(), "zero-value Router should have ShardCount 0")

	gotShard, err := ResolveBalanceShard(
		context.Background(),
		zeroRouter, nil,
		uuid.New(), uuid.New(),
		"@user_1", "default",
	)

	require.Error(t, err)
	require.Contains(t, err.Error(), "invalid shard count")
	require.Equal(t, 0, gotShard)
}

func TestResolveBalanceShard_RouterFallback_Deterministic(t *testing.T) {
	t.Parallel()

	router := shard.NewRouter(8)
	orgID := uuid.New()
	ledgerID := uuid.New()

	alias := "@user_deterministic"
	balanceKey := "default"

	shard1, err1 := ResolveBalanceShard(context.Background(), router, nil, orgID, ledgerID, alias, balanceKey)
	require.NoError(t, err1)

	shard2, err2 := ResolveBalanceShard(context.Background(), router, nil, orgID, ledgerID, alias, balanceKey)
	require.NoError(t, err2)

	require.Equal(t, shard1, shard2, "ResolveBalanceShard should be deterministic")
}

func TestResolveBalanceShard_RouterFallback_ShardInRange(t *testing.T) {
	t.Parallel()

	shardCount := 4
	router := shard.NewRouter(shardCount)
	orgID := uuid.New()
	ledgerID := uuid.New()

	for i := 0; i < 100; i++ {
		alias := "@user_" + uuid.New().String()

		gotShard, err := ResolveBalanceShard(
			context.Background(), router, nil, orgID, ledgerID, alias, "default",
		)

		require.NoError(t, err)
		require.GreaterOrEqual(t, gotShard, 0, "shard should be >= 0")
		require.Less(t, gotShard, shardCount, "shard should be < shardCount")
	}
}

func TestResolveBalanceShard_RouterFallback_ExternalPresplit(t *testing.T) {
	t.Parallel()

	router := shard.NewRouter(8)
	orgID := uuid.New()
	ledgerID := uuid.New()

	gotShard, err := ResolveBalanceShard(
		context.Background(), router, nil, orgID, ledgerID,
		"@external/USD", "shard_5",
	)

	require.NoError(t, err)
	require.Equal(t, 5, gotShard, "external with shard_5 key should resolve to shard 5")
}

func TestResolveBalanceShard_RouterFallback_ExternalOutOfRange(t *testing.T) {
	t.Parallel()

	router := shard.NewRouter(4)
	orgID := uuid.New()
	ledgerID := uuid.New()

	// shard_7 is out of range for a 4-shard router, falls back to alias hash
	gotShard, err := ResolveBalanceShard(
		context.Background(), router, nil, orgID, ledgerID,
		"@external/USD", "shard_7",
	)

	require.NoError(t, err)

	expected := router.ResolveBalance("@external/USD", "shard_7")
	require.Equal(t, expected, gotShard,
		"external with out-of-range shard key should fall back to alias hash",
	)
}

func TestResolveBalanceShard_NilManager_NilRouter(t *testing.T) {
	t.Parallel()

	gotShard, err := ResolveBalanceShard(
		context.Background(), nil, nil,
		uuid.New(), uuid.New(),
		"@anything", "any_key",
	)

	require.NoError(t, err)
	require.Equal(t, 0, gotShard)
}

func TestResolveBalanceShard_RouterOnly_SecondGuardNeverReached(t *testing.T) {
	t.Parallel()

	// When manager is nil and router has valid ShardCount, the second
	// ShardCount() <= 0 check inside the router block (line 41) is
	// redundant because the first guard (line 23) already covers it.
	// This test exercises the happy path through the router-only branch.
	router := shard.NewRouter(2)
	orgID := uuid.New()
	ledgerID := uuid.New()

	gotShard, err := ResolveBalanceShard(
		context.Background(), router, nil, orgID, ledgerID,
		"@user_abc", "default",
	)

	require.NoError(t, err)
	require.GreaterOrEqual(t, gotShard, 0)
	require.Less(t, gotShard, 2)
	require.Equal(t, router.ResolveBalance("@user_abc", "default"), gotShard)
}
