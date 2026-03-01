// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package redis

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	libLog "github.com/LerianStudio/lib-commons/v2/commons/log"
	libRedis "github.com/LerianStudio/lib-commons/v2/commons/redis"
	libZap "github.com/LerianStudio/lib-commons/v2/commons/zap"

	"github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/LerianStudio/midaz/v3/pkg/shard"
	pkgTransaction "github.com/LerianStudio/midaz/v3/pkg/transaction"
	"github.com/LerianStudio/midaz/v3/pkg/utils"
)

var (
	errWrappedRedisNil = fmt.Errorf("wrapped: %s", redis.Nil.Error())
	errUppercaseNil    = errors.New("REDIS: NIL")
	errBoom            = errors.New("boom")
)

func testLogger(t *testing.T) libLog.Logger {
	t.Helper()

	logger, err := libZap.InitializeLoggerWithError()
	require.NoError(t, err)

	return logger
}

func TestIsRedisNilError(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		err  error
		want bool
	}{
		{
			name: "direct redis nil",
			err:  redis.Nil,
			want: true,
		},
		{
			name: "wrapped redis nil",
			err:  errWrappedRedisNil,
			want: false,
		},
		{
			name: "native wrapped redis nil",
			err:  fmt.Errorf("wrap: %w", redis.Nil),
			want: true,
		},
		{
			name: "uppercase redis nil",
			err:  errUppercaseNil,
			want: false,
		},
		{
			name: "other error",
			err:  errBoom,
			want: false,
		},
		{
			name: "nil error",
			err:  nil,
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, isRedisNilError(tt.err))
		})
	}
}

// failOnCallRedisClient is a stub that fails the test if any Redis method is called.
// Used to verify that NOTED status triggers early return without Redis interaction.
type failOnCallRedisClient struct {
	t *testing.T
	redis.UniversalClient
}

func (f *failOnCallRedisClient) fail(method string) {
	f.t.Fatalf("Redis client method %q was called unexpectedly. "+
		"This likely means the NOTED status early return was removed or modified. "+
		"For NOTED transactions, the Lua script should be skipped entirely.", method)
}

// Override commonly used methods to detect unexpected calls.
func (f *failOnCallRedisClient) Eval(ctx context.Context, script string, keys []string, args ...any) *redis.Cmd {
	f.fail("Eval")
	return nil
}

func (f *failOnCallRedisClient) EvalSha(ctx context.Context, sha1 string, keys []string, args ...any) *redis.Cmd {
	f.fail("EvalSha")
	return nil
}

func (f *failOnCallRedisClient) ScriptLoad(ctx context.Context, script string) *redis.StringCmd {
	f.fail("ScriptLoad")
	return nil
}

func newFailOnCallConnection(t *testing.T) *libRedis.RedisConnection {
	t.Helper()

	return &libRedis.RedisConnection{
		Client:    &failOnCallRedisClient{t: t},
		Connected: true,
	}
}

func createBalanceOperation(organizationID, ledgerID uuid.UUID, alias, assetCode, operation string, amount, available decimal.Decimal) mmodel.BalanceOperation {
	balanceID := libCommons.GenerateUUIDv7().String()
	accountID := libCommons.GenerateUUIDv7().String()
	balanceKey := "default"

	return mmodel.BalanceOperation{
		Balance: &mmodel.Balance{
			ID:             balanceID,
			OrganizationID: organizationID.String(),
			LedgerID:       ledgerID.String(),
			AccountID:      accountID,
			Alias:          alias,
			Key:            balanceKey,
			AssetCode:      assetCode,
			Available:      available,
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
			Asset:     assetCode,
			Value:     amount,
			Operation: operation,
		},
		InternalKey: utils.BalanceInternalKey(organizationID, ledgerID, balanceKey),
	}
}

// TestProcessBalanceAtomicOperation_NotedStatus verifies that NOTED status triggers early return
// without executing the Lua script. Uses fail-on-call stub to detect unexpected Redis calls.
func TestProcessBalanceAtomicOperation_NotedStatus(t *testing.T) {
	testCases := []struct {
		name           string
		balanceAliases []string
		balanceAmounts []decimal.Decimal
		operations     []string
	}{
		{
			name:           "single balance returns unchanged",
			balanceAliases: []string{"@sender"},
			balanceAmounts: []decimal.Decimal{decimal.NewFromInt(1000)},
			operations:     []string{constant.DEBIT},
		},
		{
			name:           "multiple balances all returned unchanged",
			balanceAliases: []string{"@sender", "@receiver"},
			balanceAmounts: []decimal.Decimal{decimal.NewFromInt(1000), decimal.NewFromInt(500)},
			operations:     []string{constant.DEBIT, constant.CREDIT},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Arrange - fail-on-call connection ensures Redis is never used for NOTED status
			repo := &RedisConsumerRepository{
				conn:               newFailOnCallConnection(t),
				balanceSyncEnabled: true,
			}

			organizationID := libCommons.GenerateUUIDv7()
			ledgerID := libCommons.GenerateUUIDv7()
			transactionID := libCommons.GenerateUUIDv7()

			// Build balance operations
			balanceOps := make([]mmodel.BalanceOperation, len(tc.balanceAliases))
			for i, alias := range tc.balanceAliases {
				balanceOps[i] = createBalanceOperation(
					organizationID, ledgerID,
					alias, "USD",
					tc.operations[i],
					decimal.NewFromInt(100), // debit/credit amount (irrelevant for NOTED)
					tc.balanceAmounts[i],
				)
			}

			ctx := context.Background()

			// Act - with NOTED status, Lua script should be skipped entirely
			balances, err := repo.ProcessBalanceAtomicOperation(
				ctx,
				organizationID, ledgerID, transactionID,
				constant.NOTED,
				false,
				balanceOps,
			)

			// Assert
			require.NoError(t, err, "NOTED status should not return error")
			require.NotNil(t, balances, "balances should not be nil")
			require.Len(t, balances, len(tc.balanceAliases), "should return all balances")

			for i, bal := range balances {
				// Verify alias and values unchanged
				assert.Equal(t, tc.balanceAliases[i], bal.Alias, "alias should match input")
				assert.True(t, bal.Available.Equal(tc.balanceAmounts[i]),
					"available should be unchanged (Lua script was skipped), got %s", bal.Available)

				// Verify same pointer (no copy/modification)
				assert.Same(t, balanceOps[i].Balance, bal,
					"returned balance should be same pointer as input (early return, no processing)")
			}
		})
	}
}

func TestProcessBalanceAtomicOperation_PersistsLegacyUnscaledPayload(t *testing.T) {
	mini, err := miniredis.Run()
	require.NoError(t, err)

	defer mini.Close()

	repo, err := NewConsumerRedis(&libRedis.RedisConnection{
		Address: []string{mini.Addr()},
		Logger:  testLogger(t),
	}, false, nil)
	require.NoError(t, err)

	organizationID := uuid.New()
	ledgerID := uuid.New()
	transactionID := uuid.New()
	aliasWithKey := "@legacy#default"
	internalKey := utils.BalanceInternalKey(organizationID, ledgerID, aliasWithKey)

	balanceOp := mmodel.BalanceOperation{
		Balance: &mmodel.Balance{
			ID:             libCommons.GenerateUUIDv7().String(),
			OrganizationID: organizationID.String(),
			LedgerID:       ledgerID.String(),
			AccountID:      libCommons.GenerateUUIDv7().String(),
			Alias:          "@legacy",
			Key:            "default",
			AssetCode:      "USD",
			Available:      decimal.RequireFromString("1000.50"),
			OnHold:         decimal.Zero,
			Version:        1,
			AccountType:    "deposit",
			AllowSending:   true,
			AllowReceiving: true,
		},
		Alias: aliasWithKey,
		Amount: pkgTransaction.Amount{
			Asset:     "USD",
			Value:     decimal.RequireFromString("1.25"),
			Operation: constant.DEBIT,
		},
		InternalKey: internalKey,
	}

	_, err = repo.ProcessBalanceAtomicOperation(
		context.Background(),
		organizationID,
		ledgerID,
		transactionID,
		constant.CREATED,
		false,
		[]mmodel.BalanceOperation{balanceOp},
	)
	require.NoError(t, err)

	rawPayload, err := mini.Get(internalKey)
	require.NoError(t, err)
	assert.NotContains(t, rawPayload, "\"scale\"")
	assert.NotContains(t, rawPayload, "\"Scale\"")

	var persisted mmodel.BalanceRedis
	require.NoError(t, json.Unmarshal([]byte(rawPayload), &persisted))
	assert.True(t, persisted.Available.Equal(decimal.RequireFromString("999.25")))
	assert.Equal(t, int64(2), persisted.Version)
}

func TestProcessBalanceAtomicOperation_ReleaseApprovedMatchesGoRecompute(t *testing.T) {
	mini, err := miniredis.Run()
	require.NoError(t, err)

	defer mini.Close()

	repo, err := NewConsumerRedis(&libRedis.RedisConnection{
		Address: []string{mini.Addr()},
		Logger:  testLogger(t),
	}, false, nil)
	require.NoError(t, err)

	organizationID := uuid.New()
	ledgerID := uuid.New()
	transactionID := uuid.New()
	aliasWithKey := "@release#default"
	operationAlias := "0#@release#default"
	internalKey := utils.BalanceInternalKey(organizationID, ledgerID, aliasWithKey)

	amount := pkgTransaction.Amount{
		Asset:           "USD",
		Value:           decimal.RequireFromString("20"),
		Operation:       constant.RELEASE,
		TransactionType: constant.APPROVED,
	}

	balanceOp := mmodel.BalanceOperation{
		Balance: &mmodel.Balance{
			ID:             libCommons.GenerateUUIDv7().String(),
			OrganizationID: organizationID.String(),
			LedgerID:       ledgerID.String(),
			AccountID:      libCommons.GenerateUUIDv7().String(),
			Alias:          "@release",
			Key:            "default",
			AssetCode:      "USD",
			Available:      decimal.RequireFromString("100"),
			OnHold:         decimal.RequireFromString("30"),
			Version:        5,
			AccountType:    "deposit",
			AllowSending:   true,
			AllowReceiving: true,
		},
		Alias:       operationAlias,
		Amount:      amount,
		InternalKey: internalKey,
	}

	returnedBalances, err := repo.ProcessBalanceAtomicOperation(
		context.Background(),
		organizationID,
		ledgerID,
		transactionID,
		constant.APPROVED,
		true,
		[]mmodel.BalanceOperation{balanceOp},
	)
	require.NoError(t, err)
	require.Len(t, returnedBalances, 1)

	expectedPostMutation, err := pkgTransaction.OperateBalances(amount, *returnedBalances[0].ToTransactionBalance())
	require.NoError(t, err)

	rawPayload, err := mini.Get(internalKey)
	require.NoError(t, err)

	var persisted mmodel.BalanceRedis
	require.NoError(t, json.Unmarshal([]byte(rawPayload), &persisted))

	assert.True(t, persisted.Available.Equal(expectedPostMutation.Available))
	assert.True(t, persisted.OnHold.Equal(expectedPostMutation.OnHold))
	assert.Equal(t, expectedPostMutation.Version, persisted.Version)
}

func TestCheckOrAcquireIdempotencyKey_AcquiresOnMissingKey(t *testing.T) {
	mini, err := miniredis.Run()
	require.NoError(t, err)
	t.Cleanup(mini.Close)

	repo, err := NewConsumerRedis(&libRedis.RedisConnection{
		Address: []string{mini.Addr()},
		Logger:  testLogger(t),
	}, false, nil)
	require.NoError(t, err)

	ctx := context.Background()
	key := "idempotency:test"

	existingValue, acquired, err := repo.CheckOrAcquireIdempotencyKey(ctx, key, 5*time.Second)
	require.NoError(t, err)
	assert.True(t, acquired)
	assert.Empty(t, existingValue)

	existingValue, acquired, err = repo.CheckOrAcquireIdempotencyKey(ctx, key, 5*time.Second)
	require.NoError(t, err)
	assert.False(t, acquired)
	assert.Empty(t, existingValue)
}

func TestPreWarmExternalBalances_ShardedCoverage(t *testing.T) {
	mini, err := miniredis.Run()
	require.NoError(t, err)
	t.Cleanup(mini.Close)

	router := shard.NewRouter(8)
	repo, err := NewConsumerRedis(&libRedis.RedisConnection{
		Address: []string{mini.Addr()},
		Logger:  testLogger(t),
	}, false, router)
	require.NoError(t, err)

	organizationID := uuid.New()
	ledgerID := uuid.New()
	alias := "@external/USD"
	balanceKey := shard.ExternalBalanceKey(3)

	balance := &mmodel.Balance{
		ID:             uuid.New().String(),
		Alias:          alias,
		Key:            balanceKey,
		AccountID:      uuid.New().String(),
		AssetCode:      "USD",
		Available:      decimal.NewFromInt(100),
		OnHold:         decimal.Zero,
		Version:        1,
		AccountType:    "external",
		AllowSending:   true,
		AllowReceiving: true,
	}

	coverage, err := repo.PreWarmExternalBalances(context.Background(), organizationID, ledgerID, []*mmodel.Balance{balance}, time.Hour)
	require.NoError(t, err)

	entry, ok := coverage[alias]
	require.True(t, ok)
	assert.Equal(t, 1, entry.CoveredShards)
	assert.Equal(t, 8, entry.ExpectedShards)

	aliasWithKey := alias + "#" + balanceKey
	redisKey := utils.BalanceShardKey(router.ResolveBalance(alias, balanceKey), organizationID, ledgerID, aliasWithKey)
	assert.True(t, mini.Exists(redisKey))
}

func TestNormalizeRedisTTL(t *testing.T) {
	assert.Equal(t, time.Duration(0), normalizeRedisTTL(0))
	assert.Equal(t, 5*time.Second, normalizeRedisTTL(5*time.Second))
	assert.Equal(t, int64(1), normalizeRedisTTLSeconds(0))
	assert.Equal(t, int64(5), normalizeRedisTTLSeconds(5*time.Second))
	assert.Equal(t, int64(6), normalizeRedisTTLSeconds(5500*time.Millisecond))
}

func TestSetPipeline_ValidatesInputSizes(t *testing.T) {
	repo := &RedisConsumerRepository{}

	err := repo.SetPipeline(context.Background(), []string{"k1"}, []string{"v1", "v2"}, []time.Duration{time.Second})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "equal length")
}

func TestSetPipeline_SuccessPath(t *testing.T) {
	mini, err := miniredis.Run()
	require.NoError(t, err)

	defer mini.Close()

	conn := &libRedis.RedisConnection{
		Client:    redis.NewClient(&redis.Options{Addr: mini.Addr()}),
		Connected: true,
	}

	repo, err := NewConsumerRedis(conn, false, nil)
	require.NoError(t, err)

	err = repo.SetPipeline(
		context.Background(),
		[]string{"k1", "k2"},
		[]string{"v1", "v2"},
		[]time.Duration{2 * time.Second, 3 * time.Second},
	)
	require.NoError(t, err)

	v1, err := mini.Get("k1")
	require.NoError(t, err)
	v2, err := mini.Get("k2")
	require.NoError(t, err)
	assert.Equal(t, "v1", v1)
	assert.Equal(t, "v2", v2)
	assert.Greater(t, mini.TTL("k1"), time.Duration(0))
	assert.Greater(t, mini.TTL("k2"), time.Duration(0))
}

func TestParseLuaStringArray_BaseCases(t *testing.T) {
	assert.Equal(t, []string{"a", "b"}, parseLuaStringArray([]any{"a", []byte("b")}))
	assert.Equal(t, []string{"x", "y"}, parseLuaStringArray([]string{"x", "y"}))
	assert.Nil(t, parseLuaStringArray(123))
}
