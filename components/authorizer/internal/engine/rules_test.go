// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package engine

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v3/components/authorizer/internal/wal"
	"github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/LerianStudio/midaz/v3/pkg/shard"
	authorizerv1 "github.com/LerianStudio/midaz/v3/proto/authorizer/v1"
)

func TestValidateBalanceRulesNilGuards(t *testing.T) {
	ok, code, msg := validateBalanceRules(nil, &authorizerv1.BalanceOperation{}, 0, 0, 0, 0)
	require.False(t, ok)
	require.Equal(t, RejectionInternalError, code)
	require.NotEmpty(t, msg)

	ok, code, msg = validateBalanceRules(&Balance{}, nil, 0, 0, 0, 0)
	require.False(t, ok)
	require.Equal(t, RejectionInternalError, code)
	require.NotEmpty(t, msg)
}

func TestValidateBalanceRulesSufficientFundsAndEligibility(t *testing.T) {
	balance := &Balance{AllowSending: true, AllowReceiving: true}

	ok, code, _ := validateBalanceRules(balance, &authorizerv1.BalanceOperation{Operation: constant.DEBIT}, 100, 0, -1, 0)
	require.False(t, ok)
	require.Equal(t, RejectionInsufficientFunds, code)

	balance.AllowSending = false

	ok, code, _ = validateBalanceRules(balance, &authorizerv1.BalanceOperation{Operation: constant.DEBIT}, 100, 0, 90, 0)
	require.False(t, ok)
	require.Equal(t, RejectionAccountIneligible, code)

	balance.AllowSending = true
	balance.AllowReceiving = false

	ok, code, _ = validateBalanceRules(balance, &authorizerv1.BalanceOperation{Operation: constant.CREDIT}, 100, 0, 110, 0)
	require.False(t, ok)
	require.Equal(t, RejectionAccountIneligible, code)
}

func TestValidateBalanceRulesExternalAccountBehavior(t *testing.T) {
	balance := &Balance{IsExternal: true, AllowSending: true, AllowReceiving: true}

	ok, code, _ := validateBalanceRules(balance, &authorizerv1.BalanceOperation{Operation: constant.CREDIT}, -100, 0, 1, 0)
	require.False(t, ok)
	require.Equal(t, RejectionInsufficientFunds, code)

	ok, code, _ = validateBalanceRules(balance, &authorizerv1.BalanceOperation{Operation: constant.RELEASE}, -100, 0, -100, -1)
	require.False(t, ok)
	require.Equal(t, RejectionAmountExceedsHold, code)
}

func TestResolveOperationShardsSingleShard(t *testing.T) {
	router := shard.NewRouter(8)
	eng := New(router, wal.NewNoopWriter())

	defer eng.Close()

	ops := []*authorizerv1.BalanceOperation{
		{AccountAlias: "@alice", BalanceKey: constant.DefaultBalanceKey, Operation: constant.DEBIT},
		{AccountAlias: "@alice", BalanceKey: constant.DefaultBalanceKey, Operation: constant.CREDIT},
	}

	shardOps := eng.ResolveOperationShards(ops)
	require.Len(t, shardOps, 1, "same alias+key should resolve to a single shard")

	expectedShard := router.ResolveBalance("@alice", constant.DefaultBalanceKey)
	require.Contains(t, shardOps, expectedShard)
	require.Len(t, shardOps[expectedShard], 2)
}

func TestResolveOperationShardsMultipleShards(t *testing.T) {
	router := shard.NewRouter(8)
	eng := New(router, wal.NewNoopWriter())

	defer eng.Close()

	ops := []*authorizerv1.BalanceOperation{
		{AccountAlias: "@alice", BalanceKey: constant.DefaultBalanceKey, Operation: constant.DEBIT},
		{AccountAlias: "@bob", BalanceKey: constant.DefaultBalanceKey, Operation: constant.CREDIT},
	}

	shardOps := eng.ResolveOperationShards(ops)

	aliceShard := router.ResolveBalance("@alice", constant.DefaultBalanceKey)
	bobShard := router.ResolveBalance("@bob", constant.DefaultBalanceKey)

	if aliceShard == bobShard {
		// If they happen to land on the same shard, that's fine -- 1 entry with 2 ops.
		require.Len(t, shardOps, 1)
		require.Len(t, shardOps[aliceShard], 2)
	} else {
		require.Len(t, shardOps, 2, "different aliases should resolve to different shards")
		require.Contains(t, shardOps, aliceShard)
		require.Contains(t, shardOps, bobShard)
	}
}

func TestResolveOperationShardsNilAndEmpty(t *testing.T) {
	eng := New(shard.NewRouter(8), wal.NewNoopWriter())
	defer eng.Close()

	result := eng.ResolveOperationShards(nil)
	require.NotNil(t, result)
	require.Empty(t, result)

	result = eng.ResolveOperationShards([]*authorizerv1.BalanceOperation{})
	require.NotNil(t, result)
	require.Empty(t, result)

	var nilEng *Engine

	result = nilEng.ResolveOperationShards([]*authorizerv1.BalanceOperation{{AccountAlias: "@alice"}})
	require.NotNil(t, result)
	require.Empty(t, result)
}

func TestCountShardsForOperations(t *testing.T) {
	router := shard.NewRouter(8)
	eng := New(router, wal.NewNoopWriter())

	defer eng.Close()

	t.Run("nil returns 0", func(t *testing.T) {
		require.Equal(t, 0, eng.CountShardsForOperations(nil))
	})

	t.Run("empty slice returns 0", func(t *testing.T) {
		require.Equal(t, 0, eng.CountShardsForOperations([]*authorizerv1.BalanceOperation{}))
	})

	t.Run("nil engine returns 0", func(t *testing.T) {
		var nilEng *Engine
		require.Equal(t, 0, nilEng.CountShardsForOperations([]*authorizerv1.BalanceOperation{
			{AccountAlias: "@alice", BalanceKey: constant.DefaultBalanceKey},
		}))
	})

	t.Run("single balance returns 1 shard", func(t *testing.T) {
		ops := []*authorizerv1.BalanceOperation{
			{AccountAlias: "@alice", BalanceKey: constant.DefaultBalanceKey},
		}
		require.Equal(t, 1, eng.CountShardsForOperations(ops))
	})

	t.Run("same balance dedup returns 1 shard", func(t *testing.T) {
		ops := []*authorizerv1.BalanceOperation{
			{AccountAlias: "@alice", BalanceKey: constant.DefaultBalanceKey},
			{AccountAlias: "@alice", BalanceKey: constant.DefaultBalanceKey},
		}
		require.Equal(t, 1, eng.CountShardsForOperations(ops))
	})

	t.Run("multiple balances returns correct shard count", func(t *testing.T) {
		// Find two aliases on different shards.
		aliceShard := router.ResolveBalance("@alice", constant.DefaultBalanceKey)
		bobShard := router.ResolveBalance("@bob", constant.DefaultBalanceKey)

		ops := []*authorizerv1.BalanceOperation{
			{AccountAlias: "@alice", BalanceKey: constant.DefaultBalanceKey},
			{AccountAlias: "@bob", BalanceKey: constant.DefaultBalanceKey},
		}

		count := eng.CountShardsForOperations(ops)
		if aliceShard == bobShard {
			require.Equal(t, 1, count)
		} else {
			require.Equal(t, 2, count)
		}
	})
}

func TestValidateOnHoldRule(t *testing.T) {
	tests := []struct {
		name      string
		op        string
		preAvail  int64
		preHold   int64
		postAvail int64
		postHold  int64
		rawAmount int64
		wantOK    bool
		wantCode  string
	}{
		{name: "valid debit", op: constant.DEBIT, preAvail: 100, preHold: 10, postAvail: 90, postHold: 10, rawAmount: 10, wantOK: true},
		{name: "debit exceeds hold envelope", op: constant.DEBIT, preAvail: 100, preHold: 95, postAvail: 90, postHold: 95, rawAmount: 10, wantOK: false, wantCode: RejectionAmountExceedsHold},
		{name: "credit cannot produce negative hold", op: constant.CREDIT, preAvail: 100, preHold: 10, postAvail: 110, postHold: -1, rawAmount: 10, wantOK: false, wantCode: RejectionAmountExceedsHold},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ok, code, _ := validateOnHoldRule(tt.preAvail, tt.preHold, tt.postAvail, tt.postHold, tt.op, tt.rawAmount)
			require.Equal(t, tt.wantOK, ok)
			require.Equal(t, tt.wantCode, code)
		})
	}
}
