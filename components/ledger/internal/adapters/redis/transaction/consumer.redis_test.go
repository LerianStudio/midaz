// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package redis

import (
	"context"
	"testing"
	"time"

	libCommons "github.com/LerianStudio/lib-commons/v5/commons"
	libLog "github.com/LerianStudio/lib-commons/v5/commons/log"
	"github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/LerianStudio/midaz/v3/pkg/mtransaction"
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
	// ProcessBalanceAtomicOperation expects JSON; GetBalanceSyncKeys expects []any.
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
		Amount: mtransaction.Amount{
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
// UNIT TESTS
// =============================================================================
//
// This file only contains unit tests for pure functions and business logic
// branches that do not require a real Redis connection. Thin wrappers around
// Redis commands (Set, Get, Del, SetBytes, etc.) are covered by integration
// tests with testcontainers — see consumer.redis_integration_test.go.

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

				// The returned balance is a clone (not the same pointer) to avoid
				// mutating the caller's BalanceOperation.
				assert.NotSame(t, balanceOps[i].Balance, bal,
					"returned balance should be a clone, not the same pointer as input")
			}
		})
	}
}

// =============================================================================
// UNIT TESTS - ScheduleBalanceSyncBatch (algorithm only)
// =============================================================================

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
// UNIT TESTS — parseSyncKeysFromLuaResult (pure function)
// =============================================================================

func TestParseSyncKeysFromLuaResult_ValidPairs(t *testing.T) {
	t.Parallel()

	res := []any{"balance:key1", "1000.5", "balance:key2", "2000.75"}
	out, err := parseSyncKeysFromLuaResult(res, libLog.NewNop(), context.Background())

	require.NoError(t, err)
	require.Len(t, out, 2)
	assert.Equal(t, "balance:key1", out[0].Key)
	assert.Equal(t, 1000.5, out[0].Score)
	assert.Equal(t, "balance:key2", out[1].Key)
	assert.Equal(t, 2000.75, out[1].Score)
}

func TestParseSyncKeysFromLuaResult_BadScoreSkippedOthersContinue(t *testing.T) {
	t.Parallel()

	// Second pair has an unparseable score; first and third should still be returned.
	res := []any{"key1", "100", "key2", "not-a-number", "key3", "300"}
	out, err := parseSyncKeysFromLuaResult(res, libLog.NewNop(), context.Background())

	require.NoError(t, err)
	require.Len(t, out, 2, "bad score should be skipped, not block other keys")
	assert.Equal(t, "key1", out[0].Key)
	assert.Equal(t, "key3", out[1].Key)
}

func TestParseSyncKeysFromLuaResult_OddElementCount(t *testing.T) {
	t.Parallel()

	// Trailing member without a score partner is ignored.
	res := []any{"key1", "100", "orphan-key"}
	out, err := parseSyncKeysFromLuaResult(res, libLog.NewNop(), context.Background())

	require.NoError(t, err)
	require.Len(t, out, 1, "orphan member without score should be silently ignored")
	assert.Equal(t, "key1", out[0].Key)
}

func TestParseSyncKeysFromLuaResult_UnexpectedResultType(t *testing.T) {
	t.Parallel()

	// A non-slice result (e.g., int64) should return an error.
	_, err := parseSyncKeysFromLuaResult(int64(42), libLog.NewNop(), context.Background())

	require.Error(t, err)
	assert.Contains(t, err.Error(), "unexpected result type")
}

func TestParseSyncKeysFromLuaResult_EmptyResult(t *testing.T) {
	t.Parallel()

	out, err := parseSyncKeysFromLuaResult([]any{}, libLog.NewNop(), context.Background())

	require.NoError(t, err)
	assert.Empty(t, out)
}

func TestParseSyncKeysFromLuaResult_ByteSliceElements(t *testing.T) {
	t.Parallel()

	// Some Redis drivers return []byte instead of string.
	res := []any{[]byte("key1"), []byte("500")}
	out, err := parseSyncKeysFromLuaResult(res, libLog.NewNop(), context.Background())

	require.NoError(t, err)
	require.Len(t, out, 1)
	assert.Equal(t, "key1", out[0].Key)
	assert.Equal(t, float64(500), out[0].Score)
}
