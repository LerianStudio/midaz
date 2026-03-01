// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package redis

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	goredis "github.com/redis/go-redis/v9"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	libCommons "github.com/LerianStudio/lib-commons/v2/commons"

	"github.com/LerianStudio/midaz/v3/pkg"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/LerianStudio/midaz/v3/pkg/shard"
	pkgTransaction "github.com/LerianStudio/midaz/v3/pkg/transaction"
	"github.com/LerianStudio/midaz/v3/pkg/utils"
)

var (
	errLuaInsufficientFunds = errors.New("ERR 0018")
	errLuaBalanceNotFound   = errors.New("ERR 0061")
	errLuaPrecisionOverflow = errors.New("ERR 0142")
	errLuaRandomRedis       = errors.New("ERR some random Redis error")
)

type overrideLookupClient struct {
	goredis.UniversalClient
	value string
	err   error
}

func (c *overrideLookupClient) HGet(_ context.Context, _, _ string) *goredis.StringCmd {
	if c.err != nil {
		return goredis.NewStringResult("", c.err)
	}

	return goredis.NewStringResult(c.value, nil)
}

// =============================================================================
// UNIT TESTS — extractShardID
// =============================================================================.

func TestExtractShardID(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		key       string
		wantID    int
		wantFound bool
	}{
		{"shard 0", "balance:{shard_0}:org:ledger:alias", 0, true},
		{"shard 7", "backup_queue:{shard_7}", 7, true},
		{"shard 15", "schedule:{shard_15}:balance-sync", 15, true},
		{"legacy key (no shard)", "balance:{transactions}:org:ledger:alias", 0, false},
		{"no hash tag", "some-random-key", 0, false},
		{"malformed: missing closing brace", "balance:{shard_3:org", 0, false},
		{"malformed: non-numeric", "balance:{shard_abc}:org", 0, false},
		{"empty string", "", 0, false},
		{"shard prefix but empty number", "x:{shard_}", 0, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			gotID, gotFound := extractShardID(tt.key)

			assert.Equal(t, tt.wantFound, gotFound, "found mismatch for key %q", tt.key)

			if gotFound {
				assert.Equal(t, tt.wantID, gotID, "shard ID mismatch for key %q", tt.key)
			}
		})
	}
}

// =============================================================================
// UNIT TESTS — parseLuaStringArray
// =============================================================================.

func TestParseLuaStringArray(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input any
		want  []string
	}{
		{"nil", nil, nil},
		{"string slice", []string{"a", "b", "c"}, []string{"a", "b", "c"}},
		{"any slice of strings", []any{"x", "y"}, []string{"x", "y"}},
		{"any slice of bytes", []any{[]byte("hello"), []byte("world")}, []string{"hello", "world"}},
		{"any slice mixed", []any{"str", []byte("bytes"), 42}, []string{"str", "bytes", "42"}},
		{"empty any slice", []any{}, []string{}},
		{"wrong type (int)", 42, nil},
		{"wrong type (bool)", true, nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := parseLuaStringArray(tt.input)

			assert.Equal(t, tt.want, got)
		})
	}
}

func TestSplitAliasAndBalanceKey(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		input     string
		wantAlias string
		wantKey   string
	}{
		{name: "alias with explicit key", input: "@external/USD#shard_3", wantAlias: "@external/USD", wantKey: "shard_3"},
		{name: "alias without key", input: "@alice", wantAlias: "@alice", wantKey: "default"},
		{name: "alias with empty key", input: "@alice#", wantAlias: "@alice", wantKey: "default"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			gotAlias, gotKey := shard.SplitAliasAndBalanceKey(tt.input)

			assert.Equal(t, tt.wantAlias, gotAlias)
			assert.Equal(t, tt.wantKey, gotKey)
		})
	}
}

func TestActiveShardCount(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		repo           *RedisConsumerRepository
		expectedShards int
	}{
		{
			name:           "sharding disabled returns zero",
			repo:           &RedisConsumerRepository{shardingEnabled: false, shardRouter: nil},
			expectedShards: 0,
		},
		{
			name:           "nil router returns zero",
			repo:           &RedisConsumerRepository{shardingEnabled: true, shardRouter: nil},
			expectedShards: 0,
		},
		{
			name:           "valid router returns shard count",
			repo:           &RedisConsumerRepository{shardingEnabled: true, shardRouter: shard.NewRouter(8)},
			expectedShards: 8,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tt.expectedShards, tt.repo.activeShardCount())
		})
	}
}

// =============================================================================
// UNIT TESTS — shardSortWeight
// =============================================================================.

func TestShardSortWeight(t *testing.T) {
	t.Parallel()

	assert.Equal(t, 0, shardSortWeight(true, false), "debit-only = 0 (first)")
	assert.Equal(t, 1, shardSortWeight(true, true), "mixed = 1 (second)")
	assert.Equal(t, 2, shardSortWeight(false, true), "credit-only = 2 (last)")
	assert.Equal(t, 2, shardSortWeight(false, false), "neither = 2 (last)")
}

// =============================================================================
// UNIT TESTS — classifyLuaError
// =============================================================================.

func TestClassifyLuaError(t *testing.T) {
	t.Parallel()

	repo := &RedisConsumerRepository{}

	t.Run("insufficient funds (0018) returns UnprocessableOperationError", func(t *testing.T) {
		t.Parallel()

		result := repo.classifyLuaError(errLuaInsufficientFunds)

		var unprocessable pkg.UnprocessableOperationError
		require.ErrorAs(t, result, &unprocessable)
		assert.Equal(t, "0018", unprocessable.Code)
	})

	t.Run("balance update failed (0061) returns EntityNotFoundError", func(t *testing.T) {
		t.Parallel()

		result := repo.classifyLuaError(errLuaBalanceNotFound)

		var notFound pkg.EntityNotFoundError
		require.ErrorAs(t, result, &notFound)
		assert.Equal(t, "0061", notFound.Code)
	})

	t.Run("precision overflow (0142) returns UnprocessableOperationError", func(t *testing.T) {
		t.Parallel()

		result := repo.classifyLuaError(errLuaPrecisionOverflow)

		var unprocessable pkg.UnprocessableOperationError
		require.ErrorAs(t, result, &unprocessable)
		assert.Equal(t, "0142", unprocessable.Code)
	})

	t.Run("unknown error passes through unchanged", func(t *testing.T) {
		t.Parallel()

		result := repo.classifyLuaError(errLuaRandomRedis)

		assert.Equal(t, errLuaRandomRedis, result, "unknown errors should pass through")
	})
}

// =============================================================================
// UNIT TESTS — buildOperationArgs
// =============================================================================.

func TestBuildOperationArgs_NilBalance(t *testing.T) {
	t.Parallel()

	repo := &RedisConsumerRepository{}

	ops := []mmodel.BalanceOperation{
		{
			Balance: nil,
			Alias:   "@sender",
			Amount: pkgTransaction.Amount{
				Asset:     "USD",
				Value:     decimal.NewFromInt(100),
				Operation: "DEBIT",
			},
		},
	}

	args, err := repo.buildOperationArgs(ops, 0, "ACTIVE", 2)

	assert.Nil(t, args, "should return nil args")
	require.Error(t, err, "should return error for nil balance")
	assert.Contains(t, err.Error(), "nil balance", "error should mention nil balance")
}

func TestBuildOperationArgs_ValidOperation(t *testing.T) {
	t.Parallel()

	repo := &RedisConsumerRepository{}

	orgID := uuid.New()
	ledgerID := uuid.New()

	ops := []mmodel.BalanceOperation{
		newTestOp(orgID, ledgerID, "@sender", "DEBIT", decimal.NewFromInt(100)),
	}

	args, err := repo.buildOperationArgs(ops, 0, "ACTIVE", 2)

	require.NoError(t, err, "should succeed for valid operation")
	// 16 args per operation
	assert.Len(t, args, 16, "should have 16 args per operation")

	// Verify the scaled integer amounts (scale=2: 100 → 10000, 1000 → 100000)
	assert.Equal(t, int64(10000), args[4], "amount should be scaled to integer")
	assert.Equal(t, int64(100000), args[7], "available should be scaled to integer")
}

// =============================================================================
// UNIT TESTS — parseBalanceResults
// =============================================================================.

func TestParseBalanceResults_EmptyArray(t *testing.T) {
	t.Parallel()

	repo := &RedisConsumerRepository{}
	mapBalances := map[string]*mmodel.Balance{}

	balances, err := repo.parseBalanceResults(context.Background(), "[]", mapBalances)

	require.NoError(t, err)
	assert.Empty(t, balances, "empty JSON array should return empty slice")
}

func TestParseBalanceResults_UnknownAlias(t *testing.T) {
	t.Parallel()

	repo := &RedisConsumerRepository{}
	// No entries in map — all aliases from Lua are unknown
	mapBalances := map[string]*mmodel.Balance{}

	jsonResult := `[{"Alias":"@unknown","ID":"abc-123","AccountID":"def-456","Available":500,"OnHold":0,"Version":2,"AccountType":"deposit","AssetCode":"USD","Scale":2}]`

	balances, err := repo.parseBalanceResults(context.Background(), jsonResult, mapBalances)

	require.NoError(t, err)
	assert.Empty(t, balances, "unknown alias should be skipped")
}

func TestParseBalanceResults_InvalidJSON(t *testing.T) {
	t.Parallel()

	repo := &RedisConsumerRepository{}

	_, err := repo.parseBalanceResults(context.Background(), "not json", map[string]*mmodel.Balance{})

	assert.Error(t, err, "invalid JSON should return error")
}

func TestParseBalanceResults_UnsupportedType(t *testing.T) {
	t.Parallel()

	repo := &RedisConsumerRepository{}

	_, err := repo.parseBalanceResults(context.Background(), 12345, map[string]*mmodel.Balance{})

	require.Error(t, err, "unsupported result type should return error")
	assert.Contains(t, err.Error(), "unexpected result type")
}

func TestResolveBalanceShardWithOverrides_ZeroShardRouterDoesNotPanic(t *testing.T) {
	t.Parallel()

	repo := &RedisConsumerRepository{shardRouter: &shard.Router{}}
	client := &overrideLookupClient{value: "5"}

	resolved := repo.resolveBalanceShardWithOverrides(context.Background(), client, uuid.New(), uuid.New(), "@alice", "default")

	assert.Equal(t, 0, resolved)
}

func TestResolveBalanceShardWithOverrides_IgnoresOutOfRangeOverride(t *testing.T) {
	t.Parallel()

	repo := &RedisConsumerRepository{shardRouter: shard.NewRouter(8)}
	client := &overrideLookupClient{value: "99"}
	alias := "@alice"
	balanceKey := "default"

	expected := repo.shardRouter.ResolveBalance(alias, balanceKey)
	resolved := repo.resolveBalanceShardWithOverrides(context.Background(), client, uuid.New(), uuid.New(), alias, balanceKey)

	assert.Equal(t, expected, resolved)
}

// =============================================================================
// UNIT TESTS — resolveBackupQueueForKey
// =============================================================================.

func TestResolveBackupQueueForKey(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name            string
		shardingEnabled bool
		shardRouter     *shard.Router
		fieldKey        string
		wantQueue       string
	}{
		{
			name:            "normal shard key routes to shard-specific backup queue",
			shardingEnabled: true,
			shardRouter:     shard.NewRouter(8),
			fieldKey:        "transaction:{shard_3}:org:ledger:tx",
			wantQueue:       utils.BackupQueueShardKey(3),
		},
		{
			name:            "legacy key with {transactions} tag routes to legacy backup queue",
			shardingEnabled: true,
			shardRouter:     shard.NewRouter(8),
			fieldKey:        "transaction:{transactions}:org:ledger:tx",
			wantQueue:       TransactionBackupQueue,
		},
		{
			name:            "malformed shard key (non-numeric suffix) falls through to legacy queue",
			shardingEnabled: true,
			shardRouter:     shard.NewRouter(8),
			fieldKey:        "transaction:{shard_abc}:org:ledger:tx",
			wantQueue:       TransactionBackupQueue,
		},
		{
			name:            "empty key falls through to legacy queue",
			shardingEnabled: true,
			shardRouter:     shard.NewRouter(8),
			fieldKey:        "",
			wantQueue:       TransactionBackupQueue,
		},
		{
			name:            "key without shard pattern falls through to legacy queue",
			shardingEnabled: true,
			shardRouter:     shard.NewRouter(8),
			fieldKey:        "some:random:key",
			wantQueue:       TransactionBackupQueue,
		},
		{
			name:            "sharding disabled always returns legacy queue even for valid shard key",
			shardingEnabled: false,
			shardRouter:     shard.NewRouter(8),
			fieldKey:        "transaction:{shard_3}:org:ledger:tx",
			wantQueue:       TransactionBackupQueue,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			repo := &RedisConsumerRepository{
				shardingEnabled: tt.shardingEnabled,
				shardRouter:     tt.shardRouter,
			}

			got := repo.resolveBackupQueueForKey(tt.fieldKey)

			assert.Equal(t, tt.wantQueue, got)
		})
	}
}

// =============================================================================
// HELPERS
// =============================================================================.

func newTestOp(orgID, ledgerID uuid.UUID, alias, operation string, amount decimal.Decimal) mmodel.BalanceOperation {
	balanceID := libCommons.GenerateUUIDv7().String()
	accountID := libCommons.GenerateUUIDv7().String()

	return mmodel.BalanceOperation{
		Balance: &mmodel.Balance{
			ID:             balanceID,
			OrganizationID: orgID.String(),
			LedgerID:       ledgerID.String(),
			AccountID:      accountID,
			Alias:          alias,
			Key:            "default",
			AssetCode:      "USD",
			Available:      decimal.NewFromInt(1000),
			OnHold:         decimal.Zero,
			Version:        1,
			AccountType:    "deposit",
			AllowSending:   true,
			AllowReceiving: true,
			CreatedAt:      time.Now(),
			UpdatedAt:      time.Now(),
		},
		Alias: alias,
		Amount: pkgTransaction.Amount{
			Asset:     "USD",
			Value:     amount,
			Operation: operation,
		},
		InternalKey: utils.BalanceInternalKey(orgID, ledgerID, "default"),
	}
}
