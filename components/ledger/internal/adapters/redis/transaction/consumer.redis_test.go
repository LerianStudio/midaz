// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package redis

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	libCommons "github.com/LerianStudio/lib-commons/v4/commons"
	tmcore "github.com/LerianStudio/lib-commons/v4/commons/tenant-manager/core"
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

// =============================================================================
// TEST STUBS
// =============================================================================

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

// recordingRedisClient is a stub that records the keys passed to Redis operations.
// It embeds redis.UniversalClient (nil) and overrides the methods we need to capture.
type recordingRedisClient struct {
	t            *testing.T
	setCalls     []recordedSetCall
	getCalls     []string
	delCalls     []string
	incrCalls    []string
	mgetCalls    [][]string
	hsetCalls    []recordedHSetCall
	hgetCalls    []recordedHGetCall
	hdelCalls    []recordedHDelCall
	hgetAllCalls []string
	// getReturnVal overrides the default "test-value" returned by Get/GetBytes.
	// Set this when the test requires a specific string (e.g. valid JSON for ListBalanceByKey).
	getReturnVal string
	redis.UniversalClient
}

type recordedSetCall struct {
	Key   string
	Value any
	TTL   time.Duration
}

type recordedHSetCall struct {
	Key    string
	Values []any
}

type recordedHGetCall struct {
	Key   string
	Field string
}

type recordedHDelCall struct {
	Key    string
	Fields []string
}

func (r *recordingRedisClient) Set(ctx context.Context, key string, value any, expiration time.Duration) *redis.StatusCmd {
	r.setCalls = append(r.setCalls, recordedSetCall{Key: key, Value: value, TTL: expiration})

	cmd := redis.NewStatusCmd(ctx)
	cmd.SetVal("OK")

	return cmd
}

func (r *recordingRedisClient) SetNX(ctx context.Context, key string, value any, expiration time.Duration) *redis.BoolCmd {
	r.setCalls = append(r.setCalls, recordedSetCall{Key: key, Value: value, TTL: expiration})

	cmd := redis.NewBoolCmd(ctx)
	cmd.SetVal(true)

	return cmd
}

func (r *recordingRedisClient) Get(ctx context.Context, key string) *redis.StringCmd {
	r.getCalls = append(r.getCalls, key)

	val := "test-value"
	if r.getReturnVal != "" {
		val = r.getReturnVal
	}

	cmd := redis.NewStringCmd(ctx)
	cmd.SetVal(val)

	return cmd
}

func (r *recordingRedisClient) Del(ctx context.Context, keys ...string) *redis.IntCmd {
	r.delCalls = append(r.delCalls, keys...)

	cmd := redis.NewIntCmd(ctx)
	cmd.SetVal(1)

	return cmd
}

func (r *recordingRedisClient) Incr(ctx context.Context, key string) *redis.IntCmd {
	r.incrCalls = append(r.incrCalls, key)

	cmd := redis.NewIntCmd(ctx)
	cmd.SetVal(1)

	return cmd
}

func (r *recordingRedisClient) MGet(ctx context.Context, keys ...string) *redis.SliceCmd {
	r.mgetCalls = append(r.mgetCalls, keys)

	// Return string values matching the number of keys
	vals := make([]any, len(keys))
	for i := range keys {
		vals[i] = "value-" + keys[i]
	}

	cmd := redis.NewSliceCmd(ctx)
	cmd.SetVal(vals)

	return cmd
}

func (r *recordingRedisClient) HSet(ctx context.Context, key string, values ...any) *redis.IntCmd {
	r.hsetCalls = append(r.hsetCalls, recordedHSetCall{Key: key, Values: values})

	cmd := redis.NewIntCmd(ctx)
	cmd.SetVal(1)

	return cmd
}

func (r *recordingRedisClient) HGet(ctx context.Context, key, field string) *redis.StringCmd {
	r.hgetCalls = append(r.hgetCalls, recordedHGetCall{Key: key, Field: field})

	cmd := redis.NewStringCmd(ctx)
	cmd.SetVal("test-queue-data")

	return cmd
}

func (r *recordingRedisClient) HDel(ctx context.Context, key string, fields ...string) *redis.IntCmd {
	r.hdelCalls = append(r.hdelCalls, recordedHDelCall{Key: key, Fields: fields})

	cmd := redis.NewIntCmd(ctx)
	cmd.SetVal(1)

	return cmd
}

func (r *recordingRedisClient) HGetAll(ctx context.Context, key string) *redis.MapStringStringCmd {
	r.hgetAllCalls = append(r.hgetAllCalls, key)

	cmd := redis.NewMapStringStringCmd(ctx)
	cmd.SetVal(map[string]string{"field1": "val1"})

	return cmd
}

// testClientProvider wraps a redis.UniversalClient to implement redisClientProvider.
type testClientProvider struct {
	client redis.UniversalClient
}

func (p *testClientProvider) GetClient(_ context.Context) (redis.UniversalClient, error) {
	return p.client, nil
}

func newRecordingConnection(t *testing.T) (*testClientProvider, *recordingRedisClient) {
	t.Helper()

	client := &recordingRedisClient{t: t}

	return &testClientProvider{client: client}, client
}

// scriptCapturingRedisClient extends recordingRedisClient with the ability to capture
// Lua script calls (EvalSha / Eval / ScriptExists) so tests can assert which KEYS
// and ARGV values were passed to the Redis scripting interface.
// noscriptErr is a locally-defined Redis error that satisfies the redis.Error interface.
// script.Run() calls HasErrorPrefix(err, "NOSCRIPT") to decide whether to fall back
// from EvalSha to Eval. Returning this error from EvalSha forces that fallback.
type noscriptErr struct{}

func (noscriptErr) Error() string { return "NOSCRIPT No matching script. Please use EVAL." }
func (noscriptErr) RedisError()   {}

type scriptCapturingRedisClient struct {
	recordingRedisClient
	evalShaCalls    []recordedScriptCall
	evalCalls       []recordedScriptCall
	scriptExistsVal bool
}

type recordedScriptCall struct {
	Script string
	Keys   []string
	Args   []any
}

func (s *scriptCapturingRedisClient) EvalSha(ctx context.Context, sha1Hash string, keys []string, args ...any) *redis.Cmd {
	s.evalShaCalls = append(s.evalShaCalls, recordedScriptCall{Script: sha1Hash, Keys: keys, Args: args})

	// Return a NOSCRIPT error so that Run() falls back to Eval, giving us a second
	// capture point and ensuring a deterministic code path in unit tests.
	cmd := redis.NewCmd(ctx)
	cmd.SetErr(noscriptErr{})

	return cmd
}

func (s *scriptCapturingRedisClient) EvalRO(ctx context.Context, script string, keys []string, args ...any) *redis.Cmd {
	return s.Eval(ctx, script, keys, args...)
}

func (s *scriptCapturingRedisClient) EvalShaRO(ctx context.Context, sha1Hash string, keys []string, args ...any) *redis.Cmd {
	return s.EvalSha(ctx, sha1Hash, keys, args...)
}

func (s *scriptCapturingRedisClient) Eval(ctx context.Context, script string, keys []string, args ...any) *redis.Cmd {
	s.evalCalls = append(s.evalCalls, recordedScriptCall{Script: script, Keys: keys, Args: args})

	// Return a placeholder result — tests verify KEYS/ARGS before this point.
	// ProcessBalanceAtomicOperation expects JSON; GetBalanceSyncKeys expects []any; RemoveBalanceSyncKey ignores result.
	cmd := redis.NewCmd(ctx)
	cmd.SetVal([]any{})

	return cmd
}

func (s *scriptCapturingRedisClient) ScriptExists(ctx context.Context, hashes ...string) *redis.BoolSliceCmd {
	vals := make([]bool, len(hashes))
	for i := range vals {
		vals[i] = s.scriptExistsVal
	}

	cmd := redis.NewBoolSliceCmd(ctx)
	cmd.SetVal(vals)

	return cmd
}

func (s *scriptCapturingRedisClient) ScriptLoad(ctx context.Context, script string) *redis.StringCmd {
	cmd := redis.NewStringCmd(ctx)
	cmd.SetVal("dummy-sha1")

	return cmd
}

// capturedScriptKeys returns the KEYS slice from the first Eval call.
// Tests use this to assert namespacing was applied to Lua KEYS.
func (s *scriptCapturingRedisClient) capturedScriptKeys() []string {
	if len(s.evalCalls) == 0 {
		return nil
	}

	return s.evalCalls[0].Keys
}

// capturedScriptArgs returns the args slice from the first Eval call.
func (s *scriptCapturingRedisClient) capturedScriptArgs() []any {
	if len(s.evalCalls) == 0 {
		return nil
	}

	return s.evalCalls[0].Args
}

func newScriptCapturingConnection(t *testing.T) (*testClientProvider, *scriptCapturingRedisClient) {
	t.Helper()

	client := &scriptCapturingRedisClient{}
	client.recordingRedisClient.t = t

	return &testClientProvider{client: client}, client
}

// =============================================================================
// TEST HELPERS
// =============================================================================

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

// =============================================================================
// UNIT TESTS — NOTED Status Early Return
// =============================================================================

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
				conn: newFailOnCallConnection(t),
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
		conn: nil,
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
		conn: newMockZAddNXConnection(mockClient),
	}

	err := repo.ScheduleBalanceSyncBatch(context.Background(), []redis.Z{})

	assert.NoError(t, err, "Empty batch should return nil without error")
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
		conn: newMockZAddNXConnection(mockClient),
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
		conn: newMockZAddNXConnection(mockClient),
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
		conn: newMockZAddNXConnection(mockClient),
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
		conn: newMockZAddNXConnection(mockClient),
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
		conn: newMockZAddNXConnection(mockClient),
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
		conn: nil,
	}

	// Empty input should return 0 without any Redis call
	count, err := repo.RemoveBalanceSyncKeysBatch(context.Background(), []SyncKey{})

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
		conn: newMockEvalConnection(mockClient),
	}

	count, err := repo.RemoveBalanceSyncKeysBatch(context.Background(), []SyncKey{})

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
		conn: newMockEvalConnection(mockClient),
	}

	count, err := repo.RemoveBalanceSyncKeysBatch(context.Background(), []SyncKey{{Key: "balance:key1", Score: 100}})

	require.NoError(t, err)
	assert.Equal(t, int64(1), count)
	assert.NotEmpty(t, capturedScript, "Lua script should be passed")
	assert.Equal(t, []string{utils.BalanceSyncScheduleKey}, capturedKeys, "KEYS[1] should be the schedule key")
	// ARGV contract: [lockPrefix, member1, score1]
	assert.Equal(t, []any{
		utils.BalanceSyncLockPrefix,
		"balance:key1",
		"100",
	}, capturedArgs, "ARGV should be [lockPrefix, member, score_as_string]")
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
		conn: newMockEvalConnection(mockClient),
	}

	count, err := repo.RemoveBalanceSyncKeysBatch(context.Background(), []SyncKey{
		{Key: "key1", Score: 100},
		{Key: "key2", Score: 200},
		{Key: "key3", Score: 300},
	})

	require.NoError(t, err)
	assert.Equal(t, int64(3), count)
	// ARGV contract: [lockPrefix, member1, score1, member2, score2, member3, score3]
	assert.Equal(t, []any{
		utils.BalanceSyncLockPrefix,
		"key1", "100",
		"key2", "200",
		"key3", "300",
	}, capturedArgs, "ARGV should alternate member/score pairs after lock prefix")
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
		conn: newMockEvalConnection(mockClient),
	}

	count, err := repo.RemoveBalanceSyncKeysBatch(context.Background(), []SyncKey{
		{Key: "key1", Score: 100},
		{Key: "key2", Score: 200},
		{Key: "key3", Score: 300},
	})

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
		conn: newMockEvalConnection(mockClient),
	}

	count, err := repo.RemoveBalanceSyncKeysBatch(context.Background(), []SyncKey{{Key: "balance:key1", Score: 0}})

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
		conn: newMockEvalConnection(mockClient),
	}

	count, err := repo.RemoveBalanceSyncKeysBatch(context.Background(), []SyncKey{{Key: "balance:key1", Score: 0}})

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
		conn: newMockEvalConnection(mockClient),
	}

	_, err := repo.RemoveBalanceSyncKeysBatch(context.Background(), []SyncKey{{Key: "balance:key1", Score: 0}})

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
		conn: newMockEvalConnection(mockClient),
	}

	count, err := repo.RemoveBalanceSyncKeysBatch(context.Background(), []SyncKey{{Key: "nonexistent:key1", Score: 0}, {Key: "nonexistent:key2", Score: 0}})

	require.NoError(t, err)
	assert.Equal(t, int64(0), count, "Should return 0 when no keys were removed")
}

func TestRemoveBalanceSyncKeysBatch_InterfaceCompliance(t *testing.T) {
	type BatchRemover interface {
		RemoveBalanceSyncKeysBatch(ctx context.Context, keys []SyncKey) (int64, error)
	}

	var _ BatchRemover = (*RedisConsumerRepository)(nil)
}

// =============================================================================
// UNIT TESTS — Redis Key Namespacing
// =============================================================================

// TestKeyNamespacing_SimpleKeyMethods verifies that Set, SetNX, Get, Del, Incr,
// SetBytes, and GetBytes namespace their key parameter when a tenantId is present
// in the context, and leave keys unchanged when no tenantId is present.
func TestKeyNamespacing_SimpleKeyMethods(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		tenantID    string
		originalKey string
		expectedKey string
	}{
		{
			name:        "with_tenant_id_key_is_namespaced",
			tenantID:    "test-tenant",
			originalKey: "my:key",
			expectedKey: "tenant:test-tenant:my:key",
		},
		{
			name:        "without_tenant_id_key_is_unchanged",
			tenantID:    "",
			originalKey: "my:key",
			expectedKey: "my:key",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			conn, recorder := newRecordingConnection(t)
			repo := &RedisConsumerRepository{conn: conn}

			ctx := context.Background()
			if tc.tenantID != "" {
				ctx = tmcore.ContextWithTenantID(ctx, tc.tenantID)
			}

			// Test Set
			err := repo.Set(ctx, tc.originalKey, "val", 1)
			require.NoError(t, err)
			require.Len(t, recorder.setCalls, 1, "Set should record one call")
			assert.Equal(t, tc.expectedKey, recorder.setCalls[0].Key,
				"Set: Redis should receive the namespaced key")

			// Reset recorder for next method
			recorder.setCalls = nil

			// Test SetNX
			_, err = repo.SetNX(ctx, tc.originalKey, "val", 1)
			require.NoError(t, err)
			require.Len(t, recorder.setCalls, 1, "SetNX should record one call")
			assert.Equal(t, tc.expectedKey, recorder.setCalls[0].Key,
				"SetNX: Redis should receive the namespaced key")

			// Test Get
			_, err = repo.Get(ctx, tc.originalKey)
			require.NoError(t, err)
			require.Len(t, recorder.getCalls, 1, "Get should record one call")
			assert.Equal(t, tc.expectedKey, recorder.getCalls[0],
				"Get: Redis should receive the namespaced key")

			// Test Del
			err = repo.Del(ctx, tc.originalKey)
			require.NoError(t, err)
			require.Len(t, recorder.delCalls, 1, "Del should record one call")
			assert.Equal(t, tc.expectedKey, recorder.delCalls[0],
				"Del: Redis should receive the namespaced key")

			// Test Incr
			repo.Incr(ctx, tc.originalKey)
			require.Len(t, recorder.incrCalls, 1, "Incr should record one call")
			assert.Equal(t, tc.expectedKey, recorder.incrCalls[0],
				"Incr: Redis should receive the namespaced key")

			// Reset recorder for bytes methods
			recorder.setCalls = nil
			recorder.getCalls = nil

			// Test SetBytes
			err = repo.SetBytes(ctx, tc.originalKey, []byte("data"), 1)
			require.NoError(t, err)
			require.Len(t, recorder.setCalls, 1, "SetBytes should record one call")
			assert.Equal(t, tc.expectedKey, recorder.setCalls[0].Key,
				"SetBytes: Redis should receive the namespaced key")

			// Test GetBytes
			_, err = repo.GetBytes(ctx, tc.originalKey)
			require.NoError(t, err)
			require.Len(t, recorder.getCalls, 1, "GetBytes should record one call")
			assert.Equal(t, tc.expectedKey, recorder.getCalls[0],
				"GetBytes: Redis should receive the namespaced key")
		})
	}
}

// TestKeyNamespacing_MGet verifies that MGet namespaces ALL keys in the slice
// when a tenantId is present, and that results are keyed by ORIGINAL (non-namespaced) keys.
func TestKeyNamespacing_MGet(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		tenantID     string
		originalKeys []string
		expectedKeys []string
	}{
		{
			name:         "with_tenant_id_all_keys_namespaced_results_keyed_by_original",
			tenantID:     "test-tenant",
			originalKeys: []string{"key:1", "key:2", "key:3"},
			expectedKeys: []string{"tenant:test-tenant:key:1", "tenant:test-tenant:key:2", "tenant:test-tenant:key:3"},
		},
		{
			name:         "without_tenant_id_keys_unchanged",
			tenantID:     "",
			originalKeys: []string{"key:1", "key:2"},
			expectedKeys: []string{"key:1", "key:2"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			conn, recorder := newRecordingConnection(t)
			repo := &RedisConsumerRepository{conn: conn}

			ctx := context.Background()
			if tc.tenantID != "" {
				ctx = tmcore.ContextWithTenantID(ctx, tc.tenantID)
			}

			result, err := repo.MGet(ctx, tc.originalKeys)
			require.NoError(t, err)

			// Verify namespaced keys were sent to Redis
			require.Len(t, recorder.mgetCalls, 1, "MGet should record one call")
			assert.Equal(t, tc.expectedKeys, recorder.mgetCalls[0],
				"MGet: Redis should receive namespaced keys")

			// Verify results are keyed by ORIGINAL keys
			for _, origKey := range tc.originalKeys {
				_, exists := result[origKey]
				assert.True(t, exists,
					"MGet result should be keyed by original key %q, not namespaced key", origKey)
			}

			// Verify no namespaced keys leak into result map
			if tc.tenantID != "" {
				for resultKey := range result {
					assert.NotContains(t, resultKey, "tenant:"+tc.tenantID+":",
						"MGet result keys should NOT contain tenant prefix")
				}
			}
		})
	}
}

// TestKeyNamespacing_QueueOperations verifies that AddMessageToQueue, ReadMessageFromQueue,
// ReadAllMessagesFromQueue, and RemoveMessageFromQueue namespace BOTH the queue key
// and the field/message key when a tenantId is present.
func TestKeyNamespacing_QueueOperations(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		tenantID       string
		messageKey     string
		expectedQueue  string
		expectedMsgKey string
	}{
		{
			name:           "with_tenant_id_both_queue_and_field_namespaced",
			tenantID:       "test-tenant",
			messageKey:     "tx:abc-123",
			expectedQueue:  "tenant:test-tenant:" + TransactionBackupQueue,
			expectedMsgKey: "tenant:test-tenant:tx:abc-123",
		},
		{
			name:           "without_tenant_id_both_unchanged",
			tenantID:       "",
			messageKey:     "tx:abc-123",
			expectedQueue:  TransactionBackupQueue,
			expectedMsgKey: "tx:abc-123",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			conn, recorder := newRecordingConnection(t)
			repo := &RedisConsumerRepository{conn: conn}

			ctx := context.Background()
			if tc.tenantID != "" {
				ctx = tmcore.ContextWithTenantID(ctx, tc.tenantID)
			}

			// Test AddMessageToQueue
			err := repo.AddMessageToQueue(ctx, tc.messageKey, []byte("data"))
			require.NoError(t, err)
			require.Len(t, recorder.hsetCalls, 1, "AddMessageToQueue should record one HSet call")
			assert.Equal(t, tc.expectedQueue, recorder.hsetCalls[0].Key,
				"AddMessageToQueue: queue key should be namespaced")
			require.Len(t, recorder.hsetCalls[0].Values, 2, "HSet values should have field+value pair")
			assert.Equal(t, tc.expectedMsgKey, recorder.hsetCalls[0].Values[0],
				"AddMessageToQueue: message key (field) should be namespaced")

			// Test ReadMessageFromQueue
			_, err = repo.ReadMessageFromQueue(ctx, tc.messageKey)
			require.NoError(t, err)
			require.Len(t, recorder.hgetCalls, 1, "ReadMessageFromQueue should record one HGet call")
			assert.Equal(t, tc.expectedQueue, recorder.hgetCalls[0].Key,
				"ReadMessageFromQueue: queue key should be namespaced")
			assert.Equal(t, tc.expectedMsgKey, recorder.hgetCalls[0].Field,
				"ReadMessageFromQueue: message key (field) should be namespaced")

			// Test ReadAllMessagesFromQueue
			_, err = repo.ReadAllMessagesFromQueue(ctx)
			require.NoError(t, err)
			require.Len(t, recorder.hgetAllCalls, 1, "ReadAllMessagesFromQueue should record one HGetAll call")
			assert.Equal(t, tc.expectedQueue, recorder.hgetAllCalls[0],
				"ReadAllMessagesFromQueue: queue key should be namespaced")

			// Test RemoveMessageFromQueue
			err = repo.RemoveMessageFromQueue(ctx, tc.messageKey)
			require.NoError(t, err)
			require.Len(t, recorder.hdelCalls, 1, "RemoveMessageFromQueue should record one HDel call")
			assert.Equal(t, tc.expectedQueue, recorder.hdelCalls[0].Key,
				"RemoveMessageFromQueue: queue key should be namespaced")
			require.Len(t, recorder.hdelCalls[0].Fields, 1, "HDel should have one field")
			assert.Equal(t, tc.expectedMsgKey, recorder.hdelCalls[0].Fields[0],
				"RemoveMessageFromQueue: message key (field) should be namespaced")
		})
	}
}

// TestKeyNamespacing_ListBalanceByKey verifies that ListBalanceByKey namespaces
// the internally-built key (BalanceInternalKey) before sending it to Redis.
// ListBalanceByKey namespaces the internally-built key.
func TestKeyNamespacing_ListBalanceByKey(t *testing.T) {
	t.Parallel()

	organizationID := uuid.MustParse("00000000-0000-0000-0000-000000000001")
	ledgerID := uuid.MustParse("00000000-0000-0000-0000-000000000002")
	balanceKey := "default"

	rawInternalKey := utils.BalanceInternalKey(organizationID, ledgerID, balanceKey)

	// Build minimal valid BalanceRedis JSON so json.Unmarshal succeeds.
	balanceRedisJSON, err := json.Marshal(mmodel.BalanceRedis{
		ID:        "bal-001",
		AccountID: "acc-001",
		Alias:     "@test",
		Key:       balanceKey,
		AssetCode: "USD",
		Available: decimal.NewFromInt(1000),
	})
	require.NoError(t, err)

	tests := []struct {
		name        string
		tenantID    string
		expectedKey string
	}{
		{
			name:        "with_tenant_id_internal_key_is_namespaced",
			tenantID:    "test-tenant",
			expectedKey: "tenant:test-tenant:" + rawInternalKey,
		},
		{
			name:        "without_tenant_id_internal_key_is_unchanged",
			tenantID:    "",
			expectedKey: rawInternalKey,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			conn, recorder := newRecordingConnection(t)
			// Configure the Get stub to return valid BalanceRedis JSON.
			recorder.getReturnVal = string(balanceRedisJSON)

			repo := &RedisConsumerRepository{conn: conn}

			ctx := context.Background()
			if tc.tenantID != "" {
				ctx = tmcore.ContextWithTenantID(ctx, tc.tenantID)
			}

			_, err := repo.ListBalanceByKey(ctx, organizationID, ledgerID, balanceKey)
			require.NoError(t, err)

			require.Len(t, recorder.getCalls, 1, "ListBalanceByKey should perform exactly one Redis GET")
			assert.Equal(t, tc.expectedKey, recorder.getCalls[0],
				"ListBalanceByKey: the internal key sent to Redis should be namespaced (or unchanged when no tenant)")
		})
	}
}

// TestKeyNamespacing_ProcessBalanceAtomicOperation verifies that:
//   - The Lua KEYS slice (backup queue, transaction key, balance-sync schedule key) is namespaced.
//   - The per-balance InternalKey placed in ARGV is namespaced.
//
// ProcessBalanceAtomicOperation namespaces KEYS for Lua and balance internal keys in ARGV.
func TestKeyNamespacing_ProcessBalanceAtomicOperation(t *testing.T) {
	t.Parallel()

	organizationID := uuid.MustParse("00000000-0000-0000-0000-000000000003")
	ledgerID := uuid.MustParse("00000000-0000-0000-0000-000000000004")
	transactionID := uuid.MustParse("00000000-0000-0000-0000-000000000005")
	balanceKeyStr := "default"

	rawInternalKey := utils.BalanceInternalKey(organizationID, ledgerID, balanceKeyStr)
	rawTxKey := utils.TransactionInternalKey(organizationID, ledgerID, transactionID.String())
	rawScheduleKey := utils.BalanceSyncScheduleKey

	tests := []struct {
		name                   string
		tenantID               string
		expectedBackupQueue    string
		expectedTransactionKey string
		expectedScheduleKey    string
		expectedBalanceArgvKey string
	}{
		{
			name:                   "with_tenant_id_all_lua_keys_and_argv_namespaced",
			tenantID:               "test-tenant",
			expectedBackupQueue:    "tenant:test-tenant:" + TransactionBackupQueue,
			expectedTransactionKey: "tenant:test-tenant:" + rawTxKey,
			expectedScheduleKey:    "tenant:test-tenant:" + rawScheduleKey,
			expectedBalanceArgvKey: "tenant:test-tenant:" + rawInternalKey,
		},
		{
			name:                   "without_tenant_id_all_keys_unchanged",
			tenantID:               "",
			expectedBackupQueue:    TransactionBackupQueue,
			expectedTransactionKey: rawTxKey,
			expectedScheduleKey:    rawScheduleKey,
			expectedBalanceArgvKey: rawInternalKey,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			conn, scripter := newScriptCapturingConnection(t)
			repo := &RedisConsumerRepository{conn: conn}

			ctx := context.Background()
			if tc.tenantID != "" {
				ctx = tmcore.ContextWithTenantID(ctx, tc.tenantID)
			}

			balanceOp := mmodel.BalanceOperation{
				Balance: &mmodel.Balance{
					ID:             uuid.Must(libCommons.GenerateUUIDv7()).String(),
					OrganizationID: organizationID.String(),
					LedgerID:       ledgerID.String(),
					AccountID:      uuid.Must(libCommons.GenerateUUIDv7()).String(),
					Alias:          "@sender",
					Key:            balanceKeyStr,
					AssetCode:      "USD",
					Available:      decimal.NewFromInt(1000),
					OnHold:         decimal.Zero,
					Version:        1,
					AccountType:    "deposit",
					AllowSending:   true,
					AllowReceiving: true,
				},
				Alias: "@sender",
				Amount: pkgTransaction.Amount{
					Asset:     "USD",
					Value:     decimal.NewFromInt(100),
					Operation: constant.DEBIT,
				},
				InternalKey: rawInternalKey,
			}

			// The Lua script will fail (empty result), but we only care about the KEYS and ARGV.
			// The error is expected; we assert key namespacing before it returns.
			_, _ = repo.ProcessBalanceAtomicOperation(
				ctx,
				organizationID, ledgerID, transactionID,
				constant.APPROVED,
				false,
				[]mmodel.BalanceOperation{balanceOp},
			)

			// Verify the Eval call was recorded (script.Run falls back from EvalSha to Eval).
			require.NotEmpty(t, scripter.evalCalls,
				"ProcessBalanceAtomicOperation should have invoked Eval (via script.Run NOSCRIPT fallback)")

			luaKeys := scripter.capturedScriptKeys()
			require.Len(t, luaKeys, 3, "Lua script must receive exactly 3 KEYS: backup queue, transaction key, balance sync schedule")

			assert.Equal(t, tc.expectedBackupQueue, luaKeys[0],
				"KEYS[1] (backup queue) should be namespaced")
			assert.Equal(t, tc.expectedTransactionKey, luaKeys[1],
				"KEYS[2] (transaction key) should be namespaced")
			assert.Equal(t, tc.expectedScheduleKey, luaKeys[2],
				"KEYS[3] (balance sync schedule key) should be namespaced")

			// Verify InternalKey in ARGV is namespaced.
			// ARGV layout (from implementation): [scheduleSync, prefixedInternalKey, isPending, transactionStatus,
			//   amount.Operation, amount.Value, alias, balance.ID, available, onHold, version,
			//   accountType, accountID, assetCode, allowSending, allowReceiving, balance.Key, ...]
			// The first ARGV element is scheduleSync (int), the second is the prefixedInternalKey.
			luaArgs := scripter.capturedScriptArgs()
			require.True(t, len(luaArgs) >= 2, "Lua ARGV must have at least 2 elements")
			assert.Equal(t, tc.expectedBalanceArgvKey, luaArgs[1],
				"ARGV[2] (balance InternalKey) should be namespaced")
		})
	}
}

// TestKeyNamespacing_GetBalanceSyncKeys verifies that GetBalanceSyncKeys namespaces
// the schedule key (KEYS[1]) and passes the namespaced lock prefix in ARGV.
// GetBalanceSyncKeys namespaces schedule key and lock prefix.
func TestKeyNamespacing_GetBalanceSyncKeys(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name                    string
		tenantID                string
		expectedScheduleKey     string
		expectedLockPrefixInArg string
	}{
		{
			name:                    "with_tenant_id_schedule_key_and_lock_prefix_namespaced",
			tenantID:                "test-tenant",
			expectedScheduleKey:     "tenant:test-tenant:" + utils.BalanceSyncScheduleKey,
			expectedLockPrefixInArg: "tenant:test-tenant:" + utils.BalanceSyncLockPrefix,
		},
		{
			name:                    "without_tenant_id_schedule_key_and_lock_prefix_unchanged",
			tenantID:                "",
			expectedScheduleKey:     utils.BalanceSyncScheduleKey,
			expectedLockPrefixInArg: utils.BalanceSyncLockPrefix,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			conn, scripter := newScriptCapturingConnection(t)
			repo := &RedisConsumerRepository{conn: conn}

			ctx := context.Background()
			if tc.tenantID != "" {
				ctx = tmcore.ContextWithTenantID(ctx, tc.tenantID)
			}

			// The script result will be empty []any — that is the expected fallback from the stub.
			_, _ = repo.GetBalanceSyncKeys(ctx, 10)

			require.NotEmpty(t, scripter.evalCalls,
				"GetBalanceSyncKeys should have invoked Eval (via script.Run NOSCRIPT fallback)")

			luaKeys := scripter.capturedScriptKeys()
			require.Len(t, luaKeys, 1, "claim_balance_sync_keys.lua receives exactly 1 KEY (schedule key)")
			assert.Equal(t, tc.expectedScheduleKey, luaKeys[0],
				"KEYS[1] (balance sync schedule key) should be namespaced")

			// The lock prefix is passed as the third ARGV (after limit and TTL).
			luaArgs := scripter.capturedScriptArgs()
			require.True(t, len(luaArgs) >= 3, "ARGV must contain at least: limit, ttl, lockPrefix")
			assert.Equal(t, tc.expectedLockPrefixInArg, luaArgs[2],
				"ARGV[3] (lock prefix) should be namespaced")
		})
	}
}

// TestKeyNamespacing_RemoveBalanceSyncKey verifies that RemoveBalanceSyncKey namespaces
// the schedule key (KEYS[1]) and passes the namespaced lock prefix in ARGV.
// RemoveBalanceSyncKey namespaces schedule key and lock prefix.
func TestKeyNamespacing_RemoveBalanceSyncKey(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name                    string
		tenantID                string
		expectedScheduleKey     string
		expectedLockPrefixInArg string
	}{
		{
			name:                    "with_tenant_id_schedule_key_and_lock_prefix_namespaced",
			tenantID:                "test-tenant",
			expectedScheduleKey:     "tenant:test-tenant:" + utils.BalanceSyncScheduleKey,
			expectedLockPrefixInArg: "tenant:test-tenant:" + utils.BalanceSyncLockPrefix,
		},
		{
			name:                    "without_tenant_id_schedule_key_and_lock_prefix_unchanged",
			tenantID:                "",
			expectedScheduleKey:     utils.BalanceSyncScheduleKey,
			expectedLockPrefixInArg: utils.BalanceSyncLockPrefix,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			conn, scripter := newScriptCapturingConnection(t)
			repo := &RedisConsumerRepository{conn: conn}

			ctx := context.Background()
			if tc.tenantID != "" {
				ctx = tmcore.ContextWithTenantID(ctx, tc.tenantID)
			}

			member := "balance:{transactions}:org:ledger:default"
			_ = repo.RemoveBalanceSyncKey(ctx, member)

			require.NotEmpty(t, scripter.evalCalls,
				"RemoveBalanceSyncKey should have invoked Eval (via script.Run NOSCRIPT fallback)")

			luaKeys := scripter.capturedScriptKeys()
			require.Len(t, luaKeys, 1, "unschedule_synced_balance.lua receives exactly 1 KEY (schedule key)")
			assert.Equal(t, tc.expectedScheduleKey, luaKeys[0],
				"KEYS[1] (balance sync schedule key) should be namespaced")

			// ARGV layout: [member, lockPrefix]
			luaArgs := scripter.capturedScriptArgs()
			require.True(t, len(luaArgs) >= 2, "ARGV must contain: member and lockPrefix")
			assert.Equal(t, tc.expectedLockPrefixInArg, luaArgs[1],
				"ARGV[2] (lock prefix) should be namespaced")
		})
	}
}

// TestKeyNamespacing_BackwardsCompatible_NoTenantInContext is a focused regression test
// confirming that all namespaced methods are backwards compatible: when no tenantID
// is present in the context, every Redis key is IDENTICAL to the key that was used
// before the namespacing change.
func TestKeyNamespacing_BackwardsCompatible_NoTenantInContext(t *testing.T) {
	t.Parallel()

	// Use context.Background() — no tenantID injected — simulates single-tenant mode.
	ctx := context.Background()

	t.Run("simple_key_methods_unchanged", func(t *testing.T) {
		t.Parallel()

		conn, recorder := newRecordingConnection(t)
		repo := &RedisConsumerRepository{conn: conn}

		originalKey := "my:original:key"

		_ = repo.Set(ctx, originalKey, "v", 1)
		assert.Equal(t, originalKey, recorder.setCalls[0].Key, "Set: key must be unchanged without tenant")

		recorder.setCalls = nil

		_, _ = repo.SetNX(ctx, originalKey, "v", 1)
		assert.Equal(t, originalKey, recorder.setCalls[0].Key, "SetNX: key must be unchanged without tenant")

		_, _ = repo.Get(ctx, originalKey)
		assert.Equal(t, originalKey, recorder.getCalls[0], "Get: key must be unchanged without tenant")

		_ = repo.Del(ctx, originalKey)
		assert.Equal(t, originalKey, recorder.delCalls[0], "Del: key must be unchanged without tenant")

		repo.Incr(ctx, originalKey)
		assert.Equal(t, originalKey, recorder.incrCalls[0], "Incr: key must be unchanged without tenant")
	})

	t.Run("mget_keys_unchanged", func(t *testing.T) {
		t.Parallel()

		conn, recorder := newRecordingConnection(t)
		repo := &RedisConsumerRepository{conn: conn}

		originalKeys := []string{"key:a", "key:b"}
		_, _ = repo.MGet(ctx, originalKeys)

		require.Len(t, recorder.mgetCalls, 1)
		assert.Equal(t, originalKeys, recorder.mgetCalls[0],
			"MGet: keys must be unchanged without tenant")
	})

	t.Run("queue_operations_keys_unchanged", func(t *testing.T) {
		t.Parallel()

		conn, recorder := newRecordingConnection(t)
		repo := &RedisConsumerRepository{conn: conn}

		msgKey := "tx:orig-key"
		_ = repo.AddMessageToQueue(ctx, msgKey, []byte("data"))

		require.Len(t, recorder.hsetCalls, 1)
		assert.Equal(t, TransactionBackupQueue, recorder.hsetCalls[0].Key,
			"AddMessageToQueue: queue key must be unchanged without tenant")
		assert.Equal(t, msgKey, recorder.hsetCalls[0].Values[0],
			"AddMessageToQueue: message field key must be unchanged without tenant")
	})
}
