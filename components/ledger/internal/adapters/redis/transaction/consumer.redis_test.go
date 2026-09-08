// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package redis

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"testing"
	"time"

	libCommons "github.com/LerianStudio/lib-commons/v7/commons"
	tmcore "github.com/LerianStudio/lib-commons/v7/commons/tenant-manager/core"
	tmvalkey "github.com/LerianStudio/lib-commons/v7/commons/tenant-manager/valkey"
	libLog "github.com/LerianStudio/lib-observability/v4/log"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/trace/noop"

	"github.com/LerianStudio/midaz/v4/pkg"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	"github.com/LerianStudio/midaz/v4/pkg/mtransaction"
	"github.com/LerianStudio/midaz/v4/pkg/utils"
)

// =============================================================================
// TEST STUBS
// =============================================================================

func TestBalanceAtomicScriptErrorContract(t *testing.T) {
	_, span := noop.NewTracerProvider().Tracer("test").Start(context.Background(), "test")
	defer span.End()

	supported := map[string]struct {
		sentinel error
		entity   string
	}{
		"0018": {constant.ErrInsufficientFunds, "validateBalance"},
		"0019": {constant.ErrAccountIneligibility, "validateBalance"},
		"0139": {constant.ErrTransactionBackupCacheRetrievalFailed, "validateBalance"},
		"0167": {constant.ErrOverdraftLimitExceeded, "validateBalance"},
		"0174": {constant.ErrStaleBalanceVersion, "validateBalance"},
		"0502": {constant.ErrAccountBlocked, "validateBalance"},
		"0508": {constant.ErrAccountBlockExceptionInvalid, constant.EntityTransaction},
	}
	actual := map[string]bool{}
	for _, match := range regexp.MustCompile(`redis\.error_reply\("([0-9]{4})"\)`).FindAllStringSubmatch(balanceAtomicOperationLua, -1) {
		actual[match[1]] = true
	}
	var expectedCodes, actualCodes []string
	for code, expected := range supported {
		expectedCodes = append(expectedCodes, code)
		t.Run(code, func(t *testing.T) {
			assert.Equal(t, pkg.ValidateBusinessError(expected.sentinel, expected.entity), mapBalanceAtomicScriptError(span, errors.New(code)))
			assert.Equal(t, pkg.ValidateBusinessError(expected.sentinel, expected.entity), mapBalanceAtomicScriptError(span, balanceScriptReply("ERR "+code)))
		})
	}
	for code := range actual {
		actualCodes = append(actualCodes, code)
	}
	sort.Strings(expectedCodes)
	sort.Strings(actualCodes)
	assert.Equal(t, expectedCodes, actualCodes)

	for _, message := range []string{"0061", "0098", "ERR ERR 0018", "ERR 0018 details", "ERR script line 0018", "connection reset 0167", "BALANCE_LIMIT_INVALID", "BALANCE_LIMIT_NORMALIZATION_REQUIRED:[\"0018\"]"} {
		err := errors.New(message)
		assert.Same(t, err, mapBalanceAtomicScriptError(span, err))
	}
}

func TestBalanceSettingsSerializationCanonicalDecimal(t *testing.T) {
	for _, tc := range []struct{ input, expected string }{
		{"1e3", "1000"}, {"+0001.2500", "1.25"}, {"000", "0"}, {"-0.00", "0"}, {".50", "0.5"},
	} {
		t.Run(tc.input, func(t *testing.T) {
			original := tc.input
			settings := &mmodel.BalanceSettings{OverdraftLimit: &original}
			_, _, canonical, _, err := resolveBalanceSettingsArgs(settings)
			require.NoError(t, err)
			assert.Equal(t, tc.expected, canonical)
			assert.Equal(t, tc.input, original)
		})
	}
	for _, input := range []string{"", "NaN", "broken"} {
		_, _, _, _, err := resolveBalanceSettingsArgs(&mmodel.BalanceSettings{OverdraftLimit: &input})
		require.Error(t, err)
	}
}

type balanceScriptReply string

func (e balanceScriptReply) Error() string { return string(e) }
func (balanceScriptReply) RedisError()     {}

type limitRepairClient struct {
	redis.UniversalClient
	atomicCalls    int
	repairCalls    int
	requestRepairs int
	atomicError    error
	replyPrefix    string
	atomicArgs     [][]any
	repairedKeys   []string
}

func (c *limitRepairClient) EvalSha(ctx context.Context, sha string, keys []string, args ...any) *redis.Cmd {
	cmd := redis.NewCmd(ctx)
	if sha == normalizeBalanceLimitScript.Hash() {
		c.repairCalls++
		c.repairedKeys = append(c.repairedKeys, keys...)
		cmd.SetVal(int64(2))
		return cmd
	}
	c.atomicCalls++
	c.atomicArgs = append(c.atomicArgs, args)
	if c.atomicError != nil {
		cmd.SetErr(c.atomicError)
	} else if c.atomicCalls <= c.requestRepairs {
		cmd.SetErr(balanceScriptReply(c.replyPrefix + balanceLimitRepairPrefix + `["balance:a","balance:b"]`))
	} else {
		cmd.SetVal("done")
	}
	return cmd
}

func (c *limitRepairClient) Get(ctx context.Context, key string) *redis.StringCmd {
	cmd := redis.NewStringCmd(ctx)
	cmd.SetVal(`{"OverdraftLimit":"1e3"}`)
	return cmd
}

func TestBalanceLimitRepairWholeBatchBound(t *testing.T) {
	args := make([]any, 2*luaArgsPerOperation)
	args[0], args[luaArgsPerOperation] = "balance:a", "balance:b"
	for _, tc := range []struct {
		name                                     string
		requests, expectedAtomic, expectedRepair int
		failure                                  error
		wantError                                bool
		replyPrefix                              string
	}{
		{name: "no repair", expectedAtomic: 1},
		{name: "repairs every key", requests: 1, expectedAtomic: 2, expectedRepair: 2},
		{name: "framed reply repairs every key", requests: 1, expectedAtomic: 2, expectedRepair: 2, replyPrefix: "ERR "},
		{name: "third pass succeeds", requests: 3, expectedAtomic: 4, expectedRepair: 6},
		{name: "conflict never converges", requests: 4, expectedAtomic: 4, expectedRepair: 6, wantError: true},
		{name: "transport never retried", failure: errors.New(balanceLimitRepairPrefix + `["balance:a"]`), expectedAtomic: 1, wantError: true},
		{name: "framed transport never retried", failure: errors.New("ERR " + balanceLimitRepairPrefix + `["balance:a"]`), expectedAtomic: 1, wantError: true},
		{name: "runtime never retried", failure: balanceScriptReply("ERR line 0018"), expectedAtomic: 1, wantError: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			client := &limitRepairClient{requestRepairs: tc.requests, atomicError: tc.failure, replyPrefix: tc.replyPrefix}
			result, err := (&RedisConsumerRepository{}).runBalanceAtomicScript(context.Background(), client, nil, args)
			if tc.wantError {
				require.Error(t, err)
				if tc.failure != nil {
					assert.Equal(t, tc.failure, err)
					assert.ErrorIs(t, err, tc.failure)
				}
			} else {
				require.NoError(t, err)
				assert.Equal(t, "done", result)
			}
			assert.Equal(t, tc.expectedAtomic, client.atomicCalls)
			assert.Equal(t, tc.expectedRepair, client.repairCalls)
			for _, batch := range client.atomicArgs {
				assert.Equal(t, args, batch, "each execution must contain the complete batch")
			}
			for i := 0; i < len(client.repairedKeys); i += 2 {
				assert.Equal(t, []string{"balance:a", "balance:b"}, client.repairedKeys[i:i+2])
			}
		})
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	client := &limitRepairClient{}
	_, err := (&RedisConsumerRepository{}).runBalanceAtomicScript(ctx, client, nil, args)
	require.ErrorIs(t, err, context.Canceled)
	assert.Zero(t, client.atomicCalls)
}

func TestBalanceLimitRepairReplyValidation(t *testing.T) {
	args := make([]any, luaArgsPerOperation)
	args[0] = "balance:a"
	for _, payload := range []string{`null`, `{}`, `[]`, `["unrelated"]`, `[1]`, `broken`} {
		_, _, err := decodeBalanceLimitRepairKeys(balanceScriptReply(balanceLimitRepairPrefix+payload), args)
		require.Error(t, err)
	}
	keys, required, err := decodeBalanceLimitRepairKeys(balanceScriptReply(balanceLimitRepairPrefix+`["balance:a","balance:a"]`), args)
	require.NoError(t, err)
	assert.True(t, required)
	assert.Equal(t, []string{"balance:a"}, keys)
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
	getErr       error
	mgetValues   map[string]any
	mgetErr      error
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
	if r.getErr != nil {
		cmd := redis.NewStringCmd(ctx)
		cmd.SetErr(r.getErr)

		return cmd
	}

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
	if r.mgetErr != nil {
		cmd := redis.NewSliceCmd(ctx)
		cmd.SetErr(r.mgetErr)

		return cmd
	}

	// Return string values matching the number of keys
	vals := make([]any, len(keys))
	for i, key := range keys {
		if r.mgetValues != nil {
			vals[i] = r.mgetValues[key]
		} else {
			vals[i] = "value-" + key
		}
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
	err    error
}

func (p *testClientProvider) GetClient(_ context.Context) (redis.UniversalClient, error) {
	return p.client, p.err
}

func newRecordingConnection(t *testing.T) (*testClientProvider, *recordingRedisClient) {
	t.Helper()

	client := &recordingRedisClient{t: t}

	return &testClientProvider{client: client}, client
}

func TestListBalanceByKeyMapsOverdraftUsed(t *testing.T) {
	t.Parallel()

	provider, client := newRecordingConnection(t)
	client.getReturnVal = `{
		"id":"0198f06e-8c2b-7d3b-9dc9-7334b8f60d90",
		"accountId":"0198f06e-8c2b-7d3b-9dc9-7334b8f60d91",
		"alias":"@alice",
		"key":"default",
		"assetCode":"USD",
		"available":"0",
		"onHold":"0",
		"version":7,
		"accountType":"deposit",
		"allowSending":1,
		"allowReceiving":1,
		"overdraftUsed":"50.00"
	}`

	repo := &RedisConsumerRepository{conn: provider}
	balance, err := repo.ListBalanceByKey(context.Background(), uuid.New(), uuid.New(), "@alice#default")

	require.NoError(t, err)
	require.NotNil(t, balance)
	assert.True(t, balance.OverdraftUsed.Equal(decimal.NewFromInt(50)))
}

// TestListBalanceByKeyOverdraftUsedEmptyIsZeroMalformedFailsClosed locks the cached
// OverdraftUsed contract: the pre-overdraft snapshot shape (empty) reads as zero debt, while an
// unreadable value is reported so a caller guarding money cannot mistake it for zero.
func TestListBalanceByKeyOverdraftUsedEmptyIsZeroMalformedFailsClosed(t *testing.T) {
	t.Parallel()

	for _, testCase := range []struct {
		name          string
		overdraftUsed string
		wantErr       bool
	}{
		{name: "empty", overdraftUsed: ""},
		{name: "malformed", overdraftUsed: "not-a-number", wantErr: true},
	} {
		testCase := testCase
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			provider, client := newRecordingConnection(t)
			client.getReturnVal = `{
				"id":"0198f06e-8c2b-7d3b-9dc9-7334b8f60d90",
				"accountId":"0198f06e-8c2b-7d3b-9dc9-7334b8f60d91",
				"alias":"@alice",
				"key":"default",
				"assetCode":"USD",
				"available":"0",
				"onHold":"0",
				"version":7,
				"accountType":"deposit",
				"allowSending":1,
				"allowReceiving":1,
				"overdraftUsed":"` + testCase.overdraftUsed + `"
			}`

			repo := &RedisConsumerRepository{conn: provider}
			balance, err := repo.ListBalanceByKey(context.Background(), uuid.New(), uuid.New(), "@alice#default")

			if testCase.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), "invalid cached balance field OverdraftUsed")
				assert.Nil(t, balance)

				return
			}

			require.NoError(t, err)
			require.NotNil(t, balance)
			assert.True(t, balance.OverdraftUsed.IsZero())
		})
	}
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

const (
	legacyBalanceFixture = `{
		"ID":"0198f06e-8c2b-7d3b-9dc9-7334b8f60d80","Alias":"@legacy","Key":"reserve",
		"AccountID":"0198f06e-8c2b-7d3b-9dc9-7334b8f60d81","AssetCode":"USD","AccountType":"deposit",
		"Direction":"credit","Available":"100.25","OnHold":"2.5","Version":7,
		"AllowSending":1,"AllowReceiving":0,"AllowOverdraft":1,"OverdraftLimitEnabled":1,
		"OverdraftUsed":"3.75","OverdraftLimit":"1000","BalanceScope":"transactional"
	}`
	dualBalanceFixture = `{
		"SchemaVersion":2,
		"ID":"0198f06e-8c2b-7d3b-9dc9-7334b8f60d82","Alias":"@dual","Key":"default",
		"AccountID":"0198f06e-8c2b-7d3b-9dc9-7334b8f60d83","AssetCode":"BRL","AccountType":"checking",
		"Direction":"debit","Available":"200.125","OnHold":"4.25","Version":9007199254740993,
		"AllowSending":0,"AllowReceiving":1,"AllowOverdraft":0,"OverdraftLimitEnabled":1,
		"OverdraftUsed":"8.5","OverdraftLimit":"2500","BalanceScope":"internal",
		"id":"0198f06e-8c2b-7d3b-9dc9-7334b8f60d92","alias":"@ignored","key":"ignored",
		"accountId":"0198f06e-8c2b-7d3b-9dc9-7334b8f60d93","assetCode":"EUR","accountType":"ignored",
		"direction":"credit","available":"999","onHold":"999","version":"8",
		"allowSending":true,"allowReceiving":false,"allowOverdraft":true,"overdraftLimitEnabled":false,
		"overdraftUsed":"999","overdraftLimit":"999","balanceScope":"transactional"
	}`
	newOnlyBalanceFixture = `{
		"SchemaVersion":2,
		"id":"0198f06e-8c2b-7d3b-9dc9-7334b8f60d84","alias":"@new","key":"settled",
		"accountId":"0198f06e-8c2b-7d3b-9dc9-7334b8f60d85","assetCode":"EUR","accountType":"wallet",
		"direction":"credit","available":"300.375","onHold":"6.75","version":"9007199254740995",
		"allowSending":true,"allowReceiving":true,"allowOverdraft":true,"overdraftLimitEnabled":true,
		"overdraftUsed":"12.125","overdraftLimit":"1e3","balanceScope":"transactional"
	}`
	historicalLowerBalanceFixture = `{
		"id":"0198f06e-8c2b-7d3b-9dc9-7334b8f60d86","alias":"","key":"default",
		"accountId":"0198f06e-8c2b-7d3b-9dc9-7334b8f60d87","assetCode":"GBP","accountType":"deposit",
		"direction":"","available":"400.5","onHold":"8.125","version":11,
		"allowSending":1,"allowReceiving":0,"allowOverdraft":0,"overdraftLimitEnabled":0,
		"overdraftUsed":"","overdraftLimit":"","balanceScope":""
	}`
	legacyNumericMoneyFixture = `{
		"ID":"0198f06e-8c2b-7d3b-9dc9-7334b8f60d88","Alias":"@numeric","Key":"precision",
		"AccountID":"0198f06e-8c2b-7d3b-9dc9-7334b8f60d89","AssetCode":"USD","AccountType":"deposit",
		"Direction":"credit","Available":9007199254740993.125,"OnHold":2.75,"Version":13,
		"AllowSending":1,"AllowReceiving":1,"AllowOverdraft":1,"OverdraftLimitEnabled":1,
		"OverdraftUsed":3.5,"OverdraftLimit":"1000","BalanceScope":"transactional"
	}`
	legacyNoncanonicalMoneyFixture = `{
		"ID":"0198f06e-8c2b-7d3b-9dc9-7334b8f60d8a","Alias":"@noncanonical","Key":"legacy",
		"AccountID":"0198f06e-8c2b-7d3b-9dc9-7334b8f60d8b","AssetCode":"BRL","AccountType":"checking",
		"Direction":"debit","Available":"100.00","OnHold":"010.00","Version":14,
		"AllowSending":1,"AllowReceiving":0,"AllowOverdraft":1,"OverdraftLimitEnabled":1,
		"OverdraftUsed":"1e1","OverdraftLimit":"1000","BalanceScope":"internal"
	}`
)

func expectedBalanceRedis(
	id, alias, key, accountID, assetCode, accountType, direction, available, onHold string,
	version int64,
	allowSending, allowReceiving, allowOverdraft, overdraftLimitEnabled int,
	overdraftUsed, overdraftLimit, balanceScope string,
) *mmodel.BalanceRedis {
	return &mmodel.BalanceRedis{
		ID: id, Alias: alias, Key: key, AccountID: accountID, AssetCode: assetCode, AccountType: accountType,
		Direction: direction, Available: decimal.RequireFromString(available), OnHold: decimal.RequireFromString(onHold),
		Version: version, AllowSending: allowSending, AllowReceiving: allowReceiving,
		AllowOverdraft: allowOverdraft, OverdraftLimitEnabled: overdraftLimitEnabled,
		OverdraftUsed: overdraftUsed, OverdraftLimit: overdraftLimit, BalanceScope: balanceScope,
	}
}

func balanceReadFixtures() []struct {
	name string
	raw  string
	want *mmodel.BalanceRedis
} {
	return []struct {
		name string
		raw  string
		want *mmodel.BalanceRedis
	}{
		{
			name: "uppercase legacy",
			raw:  legacyBalanceFixture,
			want: expectedBalanceRedis(
				"0198f06e-8c2b-7d3b-9dc9-7334b8f60d80", "@legacy", "reserve",
				"0198f06e-8c2b-7d3b-9dc9-7334b8f60d81", "USD", "deposit", "credit",
				"100.25", "2.5", 7, 1, 0, 1, 1, "3.75", "1000", "transactional",
			),
		},
		{
			name: "schema two dual uses authoritative legacy fields",
			raw:  dualBalanceFixture,
			want: expectedBalanceRedis(
				"0198f06e-8c2b-7d3b-9dc9-7334b8f60d82", "@dual", "default",
				"0198f06e-8c2b-7d3b-9dc9-7334b8f60d83", "BRL", "checking", "debit",
				"200.125", "4.25", 9007199254740993, 0, 1, 0, 1, "8.5", "2500", "internal",
			),
		},
		{
			name: "schema two new only preserves int64 and canonicalizes limit in memory",
			raw:  newOnlyBalanceFixture,
			want: expectedBalanceRedis(
				"0198f06e-8c2b-7d3b-9dc9-7334b8f60d84", "@new", "settled",
				"0198f06e-8c2b-7d3b-9dc9-7334b8f60d85", "EUR", "wallet", "credit",
				"300.375", "6.75", 9007199254740995, 1, 1, 1, 1, "12.125", "1000", "transactional",
			),
		},
		{
			name: "historical lowercase dto accepts integer flags and empty defaults",
			raw:  historicalLowerBalanceFixture,
			want: expectedBalanceRedis(
				"0198f06e-8c2b-7d3b-9dc9-7334b8f60d86", "", "default",
				"0198f06e-8c2b-7d3b-9dc9-7334b8f60d87", "GBP", "deposit", "",
				"400.5", "8.125", 11, 1, 0, 0, 0, "0", "0", "transactional",
			),
		},
		{
			name: "uppercase legacy accepts exact numeric money",
			raw:  legacyNumericMoneyFixture,
			want: expectedBalanceRedis(
				"0198f06e-8c2b-7d3b-9dc9-7334b8f60d88", "@numeric", "precision",
				"0198f06e-8c2b-7d3b-9dc9-7334b8f60d89", "USD", "deposit", "credit",
				"9007199254740993.125", "2.75", 13, 1, 1, 1, 1, "3.5", "1000", "transactional",
			),
		},
		{
			name: "uppercase legacy normalizes valid noncanonical money strings",
			raw:  legacyNoncanonicalMoneyFixture,
			want: expectedBalanceRedis(
				"0198f06e-8c2b-7d3b-9dc9-7334b8f60d8a", "@noncanonical", "legacy",
				"0198f06e-8c2b-7d3b-9dc9-7334b8f60d8b", "BRL", "checking", "debit",
				"100.00", "010.00", 14, 1, 0, 1, 1, "10", "1000", "internal",
			),
		},
	}
}

func TestListBalanceByKeyDecodesSupportedCacheFormats(t *testing.T) {
	organizationID := uuid.MustParse("0198f06e-8c2b-7d3b-9dc9-7334b8f60da0")
	ledgerID := uuid.MustParse("0198f06e-8c2b-7d3b-9dc9-7334b8f60da1")

	for _, tc := range balanceReadFixtures() {
		t.Run(tc.name, func(t *testing.T) {
			provider, client := newRecordingConnection(t)
			client.getReturnVal = tc.raw
			repo := &RedisConsumerRepository{conn: provider}
			ctx := tmcore.ContextWithTenantID(context.Background(), "tenant-reader")

			got, err := repo.ListBalanceByKey(ctx, organizationID, ledgerID, tc.want.Key)
			require.NoError(t, err)
			require.NotNil(t, got)
			assert.Equal(t, tc.want.ID, got.ID)
			assert.Equal(t, tc.want.AccountID, got.AccountID)
			assert.Equal(t, tc.want.Alias, got.Alias)
			assert.Equal(t, tc.want.AssetCode, got.AssetCode)
			assert.Equal(t, tc.want.Available, got.Available)
			assert.Equal(t, tc.want.OnHold, got.OnHold)
			assert.Equal(t, tc.want.Version, got.Version)
			assert.Equal(t, tc.want.AccountType, got.AccountType)
			assert.Equal(t, tc.want.AllowSending == 1, got.AllowSending)
			assert.Equal(t, tc.want.AllowReceiving == 1, got.AllowReceiving)
			assert.Equal(t, tc.want.Key, got.Key)
			assert.Equal(t, organizationID.String(), got.OrganizationID)
			assert.Equal(t, ledgerID.String(), got.LedgerID)

			baseKey := utils.BalanceInternalKey(organizationID, ledgerID, tc.want.Key)
			wantKey, keyErr := tmvalkey.GetKey("tenant-reader", baseKey)
			require.NoError(t, keyErr)
			assert.Equal(t, []string{wantKey}, client.getCalls)
		})
	}
}

func TestListBalanceByKeyPreservesReadErrors(t *testing.T) {
	organizationID := uuid.MustParse("0198f06e-8c2b-7d3b-9dc9-7334b8f60da0")
	ledgerID := uuid.MustParse("0198f06e-8c2b-7d3b-9dc9-7334b8f60da1")
	malformedAuthoritative := `{
		"SchemaVersion":2,"ID":"bad","id":"0198f06e-8c2b-7d3b-9dc9-7334b8f60d84",
		"AccountID":"0198f06e-8c2b-7d3b-9dc9-7334b8f60d85","accountId":"0198f06e-8c2b-7d3b-9dc9-7334b8f60d85"
	}`

	for _, tc := range []struct {
		name        string
		providerErr error
		getErr      error
		raw         string
		want        error
	}{
		{name: "redis miss", getErr: redis.Nil, want: redis.Nil},
		{name: "redis transport", getErr: errors.New("redis unavailable")},
		{name: "client transport", providerErr: errors.New("client unavailable")},
		{name: "malformed authoritative field", raw: malformedAuthoritative},
	} {
		t.Run(tc.name, func(t *testing.T) {
			client := &recordingRedisClient{t: t, getErr: tc.getErr, getReturnVal: tc.raw}
			provider := &testClientProvider{client: client, err: tc.providerErr}
			got, err := (&RedisConsumerRepository{conn: provider}).ListBalanceByKey(
				context.Background(), organizationID, ledgerID, "default",
			)
			require.Error(t, err)
			assert.Nil(t, got)
			if tc.want != nil {
				assert.ErrorIs(t, err, tc.want)
			}
		})
	}
}

func TestGetBalancesByKeysDecodesSupportedCacheFormats(t *testing.T) {
	provider, client := newRecordingConnection(t)
	fixtures := balanceReadFixtures()
	keys := make([]string, len(fixtures)+1)
	client.mgetValues = make(map[string]any, len(keys))
	for i, fixture := range fixtures {
		keys[i] = fmt.Sprintf("tenant:balance:%d", i)
		client.mgetValues[keys[i]] = fixture.raw
	}
	client.mgetValues[keys[1]] = []byte(fixtures[1].raw)
	keys[len(fixtures)] = "tenant:missing"
	client.mgetValues[keys[len(fixtures)]] = nil

	got, err := (&RedisConsumerRepository{conn: provider}).GetBalancesByKeys(context.Background(), keys)
	require.NoError(t, err)
	for i, fixture := range fixtures {
		assert.Equal(t, fixture.want, got[keys[i]], fixture.name)
	}
	assert.Nil(t, got[keys[len(fixtures)]])
	assert.Equal(t, [][]string{keys}, client.mgetCalls, "fully-qualified keys must be passed through unchanged")
}

func TestGetBalancesByKeysPreservesBatchAndTransportBehavior(t *testing.T) {
	t.Run("empty input", func(t *testing.T) {
		got, err := (&RedisConsumerRepository{}).GetBalancesByKeys(context.Background(), nil)
		require.NoError(t, err)
		assert.Empty(t, got)
	})

	t.Run("client error", func(t *testing.T) {
		transportErr := errors.New("client unavailable")
		got, err := (&RedisConsumerRepository{conn: &testClientProvider{err: transportErr}}).
			GetBalancesByKeys(context.Background(), []string{"tenant:a"})
		assert.Nil(t, got)
		assert.ErrorIs(t, err, transportErr)
	})

	t.Run("mget error", func(t *testing.T) {
		transportErr := errors.New("mget unavailable")
		provider, client := newRecordingConnection(t)
		client.mgetErr = transportErr
		got, err := (&RedisConsumerRepository{conn: provider}).GetBalancesByKeys(context.Background(), []string{"tenant:a"})
		assert.Nil(t, got)
		assert.ErrorIs(t, err, transportErr)
	})

	for _, tc := range []struct {
		name      string
		value     any
		wantCause string
	}{
		{
			name:      "malformed authoritative field",
			value:     `{"SchemaVersion":2,"ID":"bad","id":"0198f06e-8c2b-7d3b-9dc9-7334b8f60d84"}`,
			wantCause: "invalid cached balance identity",
		},
		{
			name: "invalid semantic state",
			value: `{
				"SchemaVersion":2,
				"id":"0198f06e-8c2b-7d3b-9dc9-7334b8f60d84","alias":"@new","key":"settled",
				"accountId":"0198f06e-8c2b-7d3b-9dc9-7334b8f60d85","assetCode":"EUR","accountType":"wallet",
				"direction":"sideways","available":"300.375","onHold":"6.75","version":"12",
				"allowSending":true,"allowReceiving":true,"allowOverdraft":true,"overdraftLimitEnabled":true,
				"overdraftUsed":"12.125","overdraftLimit":"1000","balanceScope":"transactional"
			}`,
			wantCause: "invalid balance cache direction",
		},
		{
			name:      "unexpected redis value type",
			value:     int64(42),
			wantCause: "unexpected Redis value type int64",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			const key = "tenant:present-but-invalid"
			provider, client := newRecordingConnection(t)
			client.mgetValues = map[string]any{key: tc.value}

			got, err := (&RedisConsumerRepository{conn: provider}).GetBalancesByKeys(context.Background(), []string{key})
			require.Error(t, err)
			assert.Nil(t, got, "present invalid data must not be represented as an expired cache entry")
			assert.Contains(t, err.Error(), key)
			assert.Contains(t, err.Error(), tc.wantCause)
		})
	}

	t.Run("chunks large requests", func(t *testing.T) {
		provider, client := newRecordingConnection(t)
		keys := make([]string, maxRedisBatchSize+1)
		client.mgetValues = make(map[string]any, len(keys))
		for i := range keys {
			keys[i] = fmt.Sprintf("tenant:balance:%d", i)
			client.mgetValues[keys[i]] = newOnlyBalanceFixture
		}

		got, err := (&RedisConsumerRepository{conn: provider}).GetBalancesByKeys(context.Background(), keys)
		require.NoError(t, err)
		require.Len(t, got, len(keys))
		require.Len(t, client.mgetCalls, 2)
		assert.Equal(t, keys[:maxRedisBatchSize], client.mgetCalls[0])
		assert.Equal(t, keys[maxRedisBatchSize:], client.mgetCalls[1])
	})
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
				nil,
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

func TestDecodeBalanceRedisForReadRejectsQualifiedNewOnlyKey(t *testing.T) {
	t.Parallel()

	raw := []byte(`{
		"SchemaVersion":2,
		"id":"11111111-1111-4111-8111-111111111111",
		"accountId":"aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa",
		"accountType":"deposit",
		"assetCode":"USD",
		"alias":"@source",
		"key":"@source#default",
		"available":"100",
		"onHold":"0",
		"version":"1",
		"allowSending":true,
		"allowReceiving":true
	}`)

	_, err := decodeBalanceRedisForRead(raw)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "qualified key is not permitted in new-only cached balance")
}
