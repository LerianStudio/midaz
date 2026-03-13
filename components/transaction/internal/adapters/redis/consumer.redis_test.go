// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package redis

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	libCommons "github.com/LerianStudio/lib-commons/v4/commons"
	"github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	pkgTransaction "github.com/LerianStudio/midaz/v3/pkg/transaction"
	"github.com/LerianStudio/midaz/v3/pkg/utils"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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

// Override commonly used methods to detect unexpected calls
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

func newFailOnCallConnection(t *testing.T) *staticRedisProvider {
	t.Helper()

	return &staticRedisProvider{client: &failOnCallRedisClient{t: t}}
}

func createBalanceOperation(organizationID, ledgerID uuid.UUID, alias, assetCode, operation string, amount, available decimal.Decimal) mmodel.BalanceOperation {
	balanceID := uuid.Must(libCommons.GenerateUUIDv7()).String()
	accountID := uuid.Must(libCommons.GenerateUUIDv7()).String()
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

// mockZAddNXClient is a mock Redis client for testing ScheduleBalanceSyncBatch.
type mockZAddNXClient struct {
	redis.UniversalClient
	zAddNXFunc func(ctx context.Context, key string, members ...redis.Z) *redis.IntCmd
}

func (m *mockZAddNXClient) ZAddNX(ctx context.Context, key string, members ...redis.Z) *redis.IntCmd {
	if m.zAddNXFunc != nil {
		return m.zAddNXFunc(ctx, key, members...)
	}

	return redis.NewIntCmd(ctx)
}

func newMockZAddNXConnection(client *mockZAddNXClient) *staticRedisProvider {
	return &staticRedisProvider{client: client}
}

// mockEvalClient is a mock Redis client for testing RemoveBalanceSyncKeysBatch.
type mockEvalClient struct {
	redis.UniversalClient
	evalFunc func(ctx context.Context, script string, keys []string, args ...any) *redis.Cmd
}

func (m *mockEvalClient) Eval(ctx context.Context, script string, keys []string, args ...any) *redis.Cmd {
	if m.evalFunc != nil {
		return m.evalFunc(ctx, script, keys, args...)
	}

	return redis.NewCmd(ctx)
}

func newMockEvalConnection(client *mockEvalClient) *staticRedisProvider {
	return &staticRedisProvider{client: client}
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

			organizationID := uuid.Must(libCommons.GenerateUUIDv7())
			ledgerID := uuid.Must(libCommons.GenerateUUIDv7())
			transactionID := uuid.Must(libCommons.GenerateUUIDv7())

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
			require.Len(t, balances.Before, len(tc.balanceAliases), "should return all balances")

			for i, bal := range balances.Before {
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

// =============================================================================
// UNIT TESTS - ScheduleBalanceSyncBatch
// =============================================================================

func TestScheduleBalanceSyncBatch_EmptyInput(t *testing.T) {
	// Create a repository with nil connection to test early return
	repo := &RedisConsumerRepository{
		conn:               nil,
		balanceSyncEnabled: true,
	}

	// Empty input should return nil without any Redis call
	err := repo.ScheduleBalanceSyncBatch(context.Background(), []redis.Z{})

	assert.NoError(t, err, "Empty batch should return nil without error")
}

func TestScheduleBalanceSyncBatch_EmptyInput_NoRedisCall(t *testing.T) {
	// Create mock that fails if called
	mockClient := &mockZAddNXClient{
		zAddNXFunc: func(_ context.Context, _ string, _ ...redis.Z) *redis.IntCmd {
			t.Fatal("ZAddNX should not be called for empty input")

			return nil
		},
	}

	repo := &RedisConsumerRepository{
		conn:               newMockZAddNXConnection(mockClient),
		balanceSyncEnabled: true,
	}

	err := repo.ScheduleBalanceSyncBatch(context.Background(), []redis.Z{})

	assert.NoError(t, err, "Empty batch should return nil without error")
}

func TestScheduleBalanceSyncBatch_BalanceSyncDisabled_NoRedisCall(t *testing.T) {
	// Create mock that fails if ZAddNX is called - regression test for feature toggle
	mockClient := &mockZAddNXClient{
		zAddNXFunc: func(_ context.Context, _ string, _ ...redis.Z) *redis.IntCmd {
			t.Fatal("ZAddNX should not be called when balanceSyncEnabled is false")

			return nil
		},
	}

	repo := &RedisConsumerRepository{
		conn:               newMockZAddNXConnection(mockClient),
		balanceSyncEnabled: false, // Feature toggle disabled
	}

	// Non-empty members - should still skip Redis call due to disabled feature
	members := []redis.Z{
		{Score: float64(time.Now().Unix()), Member: "balance:key1"},
		{Score: float64(time.Now().Unix()), Member: "balance:key2"},
	}

	err := repo.ScheduleBalanceSyncBatch(context.Background(), members)

	assert.NoError(t, err, "Disabled feature should return nil without error")
}

func TestScheduleBalanceSyncBatch_SingleMember(t *testing.T) {
	var capturedKey string

	var capturedMembers []redis.Z

	mockClient := &mockZAddNXClient{
		zAddNXFunc: func(_ context.Context, key string, members ...redis.Z) *redis.IntCmd {
			capturedKey = key
			capturedMembers = members

			cmd := redis.NewIntCmd(context.Background())
			cmd.SetVal(1) // 1 member added

			return cmd
		},
	}

	repo := &RedisConsumerRepository{
		conn:               newMockZAddNXConnection(mockClient),
		balanceSyncEnabled: true,
	}

	members := []redis.Z{
		{Score: float64(time.Now().Unix()), Member: "balance:key1"},
	}

	err := repo.ScheduleBalanceSyncBatch(context.Background(), members)

	require.NoError(t, err)
	assert.NotEmpty(t, capturedKey, "Schedule key should be set")
	require.Len(t, capturedMembers, 1, "Should have 1 member")
	assert.Equal(t, "balance:key1", capturedMembers[0].Member)
}

func TestScheduleBalanceSyncBatch_MultipleMembers(t *testing.T) {
	var capturedMembers []redis.Z

	mockClient := &mockZAddNXClient{
		zAddNXFunc: func(_ context.Context, _ string, members ...redis.Z) *redis.IntCmd {
			capturedMembers = members

			cmd := redis.NewIntCmd(context.Background())
			cmd.SetVal(int64(len(members))) // All members added

			return cmd
		},
	}

	repo := &RedisConsumerRepository{
		conn:               newMockZAddNXConnection(mockClient),
		balanceSyncEnabled: true,
	}

	now := time.Now().Unix()
	members := []redis.Z{
		{Score: float64(now + 100), Member: "balance:key1"},
		{Score: float64(now + 200), Member: "balance:key2"},
		{Score: float64(now + 300), Member: "balance:key3"},
	}

	err := repo.ScheduleBalanceSyncBatch(context.Background(), members)

	require.NoError(t, err)
	assert.Len(t, capturedMembers, 3, "Should have 3 members")
}

func TestScheduleBalanceSyncBatch_RedisError(t *testing.T) {
	expectedError := errors.New("redis connection refused")

	mockClient := &mockZAddNXClient{
		zAddNXFunc: func(_ context.Context, _ string, _ ...redis.Z) *redis.IntCmd {
			cmd := redis.NewIntCmd(context.Background())
			cmd.SetErr(expectedError)

			return cmd
		},
	}

	repo := &RedisConsumerRepository{
		conn:               newMockZAddNXConnection(mockClient),
		balanceSyncEnabled: true,
	}

	members := []redis.Z{
		{Score: float64(time.Now().Unix()), Member: "balance:key1"},
	}

	err := repo.ScheduleBalanceSyncBatch(context.Background(), members)

	require.Error(t, err)
	assert.ErrorIs(t, err, expectedError)
}

func TestScheduleBalanceSyncBatch_PartialAdd(t *testing.T) {
	// Test that ZAddNX returns count of NEW members (existing members not counted)
	mockClient := &mockZAddNXClient{
		zAddNXFunc: func(_ context.Context, _ string, members ...redis.Z) *redis.IntCmd {
			cmd := redis.NewIntCmd(context.Background())
			// Simulate 2 out of 3 members already existing
			cmd.SetVal(1) // Only 1 new member added

			return cmd
		},
	}

	repo := &RedisConsumerRepository{
		conn:               newMockZAddNXConnection(mockClient),
		balanceSyncEnabled: true,
	}

	members := []redis.Z{
		{Score: float64(time.Now().Unix()), Member: "balance:key1"},
		{Score: float64(time.Now().Unix()), Member: "balance:key2"},
		{Score: float64(time.Now().Unix()), Member: "balance:key3"},
	}

	// Should not error even if not all members were added (NX behavior)
	err := repo.ScheduleBalanceSyncBatch(context.Background(), members)

	require.NoError(t, err)
}

func TestScheduleBalanceSyncBatch_DeduplicatesWithMinScore(t *testing.T) {
	var capturedMembers []redis.Z

	mockClient := &mockZAddNXClient{
		zAddNXFunc: func(_ context.Context, _ string, members ...redis.Z) *redis.IntCmd {
			capturedMembers = members

			cmd := redis.NewIntCmd(context.Background())
			cmd.SetVal(int64(len(members)))

			return cmd
		},
	}

	repo := &RedisConsumerRepository{
		conn:               newMockZAddNXConnection(mockClient),
		balanceSyncEnabled: true,
	}

	// Input has duplicate members with different scores
	// key1 appears twice: score 200 and score 100 (100 should win as minimum)
	// key2 appears twice: score 50 and score 150 (50 should win as minimum)
	members := []redis.Z{
		{Score: 200, Member: "balance:key1"},
		{Score: 100, Member: "balance:key1"}, // duplicate with lower score
		{Score: 50, Member: "balance:key2"},
		{Score: 150, Member: "balance:key2"}, // duplicate with higher score
		{Score: 300, Member: "balance:key3"}, // unique
	}

	err := repo.ScheduleBalanceSyncBatch(context.Background(), members)

	require.NoError(t, err)
	assert.Len(t, capturedMembers, 3, "Should have 3 unique members after deduplication")

	// Build map from captured members for easier assertion
	scoreByMember := make(map[string]float64)
	for _, m := range capturedMembers {
		memberStr, ok := m.Member.(string)
		require.True(t, ok, "Member should be a string, got %T", m.Member)
		scoreByMember[memberStr] = m.Score
	}

	assert.Equal(t, float64(100), scoreByMember["balance:key1"], "key1 should have minimum score 100")
	assert.Equal(t, float64(50), scoreByMember["balance:key2"], "key2 should have minimum score 50")
	assert.Equal(t, float64(300), scoreByMember["balance:key3"], "key3 should have score 300")
}

// =============================================================================
// UNIT TESTS - RemoveBalanceSyncKeysBatch
// =============================================================================

func TestRemoveBalanceSyncKeysBatch_EmptyInput(t *testing.T) {
	// Create a repository with nil connection to test early return
	repo := &RedisConsumerRepository{
		conn:               nil,
		balanceSyncEnabled: true,
	}

	// Empty input should return 0 without any Redis call
	count, err := repo.RemoveBalanceSyncKeysBatch(context.Background(), []string{})

	assert.NoError(t, err, "Empty keys should return nil error")
	assert.Equal(t, int64(0), count, "Empty keys should return 0 count")
}

func TestRemoveBalanceSyncKeysBatch_EmptyInput_NoRedisCall(t *testing.T) {
	// Create mock that fails if called
	mockClient := &mockEvalClient{
		evalFunc: func(_ context.Context, _ string, _ []string, _ ...any) *redis.Cmd {
			t.Fatal("Eval should not be called for empty input")

			return nil
		},
	}

	repo := &RedisConsumerRepository{
		conn:               newMockEvalConnection(mockClient),
		balanceSyncEnabled: true,
	}

	count, err := repo.RemoveBalanceSyncKeysBatch(context.Background(), []string{})

	assert.NoError(t, err)
	assert.Equal(t, int64(0), count)
}

func TestRemoveBalanceSyncKeysBatch_SingleKey(t *testing.T) {
	var capturedScript string

	var capturedKeys []string

	var capturedArgs []any

	mockClient := &mockEvalClient{
		evalFunc: func(_ context.Context, script string, keys []string, args ...any) *redis.Cmd {
			capturedScript = script
			capturedKeys = keys
			capturedArgs = args

			cmd := redis.NewCmd(context.Background())
			cmd.SetVal(int64(1)) // 1 key removed

			return cmd
		},
	}

	repo := &RedisConsumerRepository{
		conn:               newMockEvalConnection(mockClient),
		balanceSyncEnabled: true,
	}

	count, err := repo.RemoveBalanceSyncKeysBatch(context.Background(), []string{"balance:key1"})

	require.NoError(t, err)
	assert.Equal(t, int64(1), count)
	assert.NotEmpty(t, capturedScript, "Lua script should be passed")
	assert.Len(t, capturedKeys, 1, "Should have 1 key (schedule key)")
	assert.Len(t, capturedArgs, 2, "Should have lock prefix + 1 member")
}

func TestRemoveBalanceSyncKeysBatch_MultipleKeys(t *testing.T) {
	var capturedArgs []any

	mockClient := &mockEvalClient{
		evalFunc: func(_ context.Context, _ string, _ []string, args ...any) *redis.Cmd {
			capturedArgs = args

			cmd := redis.NewCmd(context.Background())
			cmd.SetVal(int64(3)) // All 3 keys removed

			return cmd
		},
	}

	repo := &RedisConsumerRepository{
		conn:               newMockEvalConnection(mockClient),
		balanceSyncEnabled: true,
	}

	count, err := repo.RemoveBalanceSyncKeysBatch(context.Background(), []string{"key1", "key2", "key3"})

	require.NoError(t, err)
	assert.Equal(t, int64(3), count)
	// Args should be: [lockPrefix, key1, key2, key3]
	assert.Len(t, capturedArgs, 4, "Should have lock prefix + 3 members")
}

func TestRemoveBalanceSyncKeysBatch_PartialRemoval(t *testing.T) {
	// Test when some keys did not exist in the schedule
	mockClient := &mockEvalClient{
		evalFunc: func(_ context.Context, _ string, _ []string, _ ...any) *redis.Cmd {
			cmd := redis.NewCmd(context.Background())
			// Only 2 out of 3 keys existed and were removed
			cmd.SetVal(int64(2))

			return cmd
		},
	}

	repo := &RedisConsumerRepository{
		conn:               newMockEvalConnection(mockClient),
		balanceSyncEnabled: true,
	}

	count, err := repo.RemoveBalanceSyncKeysBatch(context.Background(), []string{"key1", "key2", "key3"})

	require.NoError(t, err)
	assert.Equal(t, int64(2), count, "Should return actual count of removed keys")
}

func TestRemoveBalanceSyncKeysBatch_RedisError(t *testing.T) {
	expectedError := errors.New("redis script error")

	mockClient := &mockEvalClient{
		evalFunc: func(_ context.Context, _ string, _ []string, _ ...any) *redis.Cmd {
			cmd := redis.NewCmd(context.Background())
			cmd.SetErr(expectedError)

			return cmd
		},
	}

	repo := &RedisConsumerRepository{
		conn:               newMockEvalConnection(mockClient),
		balanceSyncEnabled: true,
	}

	count, err := repo.RemoveBalanceSyncKeysBatch(context.Background(), []string{"balance:key1"})

	require.Error(t, err)
	assert.ErrorIs(t, err, expectedError)
	assert.Equal(t, int64(0), count)
}

func TestRemoveBalanceSyncKeysBatch_UnexpectedResultType(t *testing.T) {
	mockClient := &mockEvalClient{
		evalFunc: func(_ context.Context, _ string, _ []string, _ ...any) *redis.Cmd {
			cmd := redis.NewCmd(context.Background())
			// Return string instead of int64 - unexpected type
			cmd.SetVal("not an int64")

			return cmd
		},
	}

	repo := &RedisConsumerRepository{
		conn:               newMockEvalConnection(mockClient),
		balanceSyncEnabled: true,
	}

	count, err := repo.RemoveBalanceSyncKeysBatch(context.Background(), []string{"balance:key1"})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "unexpected result type")
	assert.Equal(t, int64(0), count)
}

func TestRemoveBalanceSyncKeysBatch_LuaScriptContent(t *testing.T) {
	// Verify the Lua script is embedded and contains expected commands
	require.NotEmpty(t, removeBalanceSyncKeysBatchScript, "Lua script should be embedded and not empty")

	// Verify script contains expected commands
	assert.True(t, strings.Contains(removeBalanceSyncKeysBatchScript, "ZREM"),
		"Lua script should contain ZREM command for schedule removal")

	assert.True(t, strings.Contains(removeBalanceSyncKeysBatchScript, "DEL"),
		"Lua script should contain DEL command for lock cleanup")

	assert.True(t, strings.Contains(removeBalanceSyncKeysBatchScript, "KEYS[1]"),
		"Lua script should reference KEYS[1] for schedule key")

	assert.True(t, strings.Contains(removeBalanceSyncKeysBatchScript, "ARGV"),
		"Lua script should reference ARGV for lock prefix and members")
}

func TestRemoveBalanceSyncKeysBatch_ScriptUsesCorrectPattern(t *testing.T) {
	var capturedScript string

	mockClient := &mockEvalClient{
		evalFunc: func(_ context.Context, script string, _ []string, _ ...any) *redis.Cmd {
			capturedScript = script

			cmd := redis.NewCmd(context.Background())
			cmd.SetVal(int64(1))

			return cmd
		},
	}

	repo := &RedisConsumerRepository{
		conn:               newMockEvalConnection(mockClient),
		balanceSyncEnabled: true,
	}

	_, err := repo.RemoveBalanceSyncKeysBatch(context.Background(), []string{"balance:key1"})

	require.NoError(t, err)

	// Verify the embedded script is passed to Eval
	assert.Equal(t, removeBalanceSyncKeysBatchScript, capturedScript,
		"Should pass the embedded Lua script to Eval")
}

func TestRemoveBalanceSyncKeysBatch_ZeroKeysRemoved(t *testing.T) {
	// Test when keys don't exist in the schedule
	mockClient := &mockEvalClient{
		evalFunc: func(_ context.Context, _ string, _ []string, _ ...any) *redis.Cmd {
			cmd := redis.NewCmd(context.Background())
			cmd.SetVal(int64(0)) // No keys removed (they didn't exist)

			return cmd
		},
	}

	repo := &RedisConsumerRepository{
		conn:               newMockEvalConnection(mockClient),
		balanceSyncEnabled: true,
	}

	count, err := repo.RemoveBalanceSyncKeysBatch(context.Background(), []string{"nonexistent:key1", "nonexistent:key2"})

	require.NoError(t, err)
	assert.Equal(t, int64(0), count, "Should return 0 when no keys were removed")
}

func TestRemoveBalanceSyncKeysBatch_InterfaceCompliance(t *testing.T) {
	type BatchRemover interface {
		RemoveBalanceSyncKeysBatch(ctx context.Context, keys []string) (int64, error)
	}

	var _ BatchRemover = (*RedisConsumerRepository)(nil)
}
