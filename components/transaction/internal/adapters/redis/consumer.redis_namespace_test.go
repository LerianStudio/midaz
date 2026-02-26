// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package redis

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	libCommons "github.com/LerianStudio/lib-commons/v3/commons"
	libRedis "github.com/LerianStudio/lib-commons/v3/commons/redis"
	tmcore "github.com/LerianStudio/lib-commons/v3/commons/tenant-manager/core"
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

func newRecordingConnection(t *testing.T) (*libRedis.RedisConnection, *recordingRedisClient) {
	t.Helper()

	client := &recordingRedisClient{t: t}

	return &libRedis.RedisConnection{
		Client:    client,
		Connected: true,
	}, client
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

func newScriptCapturingConnection(t *testing.T) (*libRedis.RedisConnection, *scriptCapturingRedisClient) {
	t.Helper()

	client := &scriptCapturingRedisClient{}
	client.recordingRedisClient.t = t

	return &libRedis.RedisConnection{
		Client:    client,
		Connected: true,
	}, client
}

// =============================================================================
// UNIT TESTS — Redis Key Namespacing (T-001)
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
			repo := &RedisConsumerRepository{conn: conn, balanceSyncEnabled: false}

			ctx := context.Background()
			if tc.tenantID != "" {
				ctx = tmcore.SetTenantIDInContext(ctx, tc.tenantID)
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
			repo := &RedisConsumerRepository{conn: conn, balanceSyncEnabled: false}

			ctx := context.Background()
			if tc.tenantID != "" {
				ctx = tmcore.SetTenantIDInContext(ctx, tc.tenantID)
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
			repo := &RedisConsumerRepository{conn: conn, balanceSyncEnabled: false}

			ctx := context.Background()
			if tc.tenantID != "" {
				ctx = tmcore.SetTenantIDInContext(ctx, tc.tenantID)
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
// AC-4: ListBalanceByKey namespaces the internally-built key.
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
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			conn, recorder := newRecordingConnection(t)
			// Configure the Get stub to return valid BalanceRedis JSON.
			recorder.getReturnVal = string(balanceRedisJSON)

			repo := &RedisConsumerRepository{conn: conn, balanceSyncEnabled: false}

			ctx := context.Background()
			if tc.tenantID != "" {
				ctx = tmcore.SetTenantIDInContext(ctx, tc.tenantID)
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
// AC-6: ProcessBalanceAtomicOperation namespaces KEYS for Lua and balance internal keys in ARGV.
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
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			conn, scripter := newScriptCapturingConnection(t)
			repo := &RedisConsumerRepository{conn: conn, balanceSyncEnabled: true}

			ctx := context.Background()
			if tc.tenantID != "" {
				ctx = tmcore.SetTenantIDInContext(ctx, tc.tenantID)
			}

			balanceOp := mmodel.BalanceOperation{
				Balance: &mmodel.Balance{
					ID:             libCommons.GenerateUUIDv7().String(),
					OrganizationID: organizationID.String(),
					LedgerID:       ledgerID.String(),
					AccountID:      libCommons.GenerateUUIDv7().String(),
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
// AC-7: GetBalanceSyncKeys namespaces schedule key and lock prefix.
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
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			conn, scripter := newScriptCapturingConnection(t)
			repo := &RedisConsumerRepository{conn: conn, balanceSyncEnabled: true}

			ctx := context.Background()
			if tc.tenantID != "" {
				ctx = tmcore.SetTenantIDInContext(ctx, tc.tenantID)
			}

			// The script result will be empty []any — that is the expected fallback from the stub.
			_, _ = repo.GetBalanceSyncKeys(ctx, 10)

			require.NotEmpty(t, scripter.evalCalls,
				"GetBalanceSyncKeys should have invoked Eval (via script.Run NOSCRIPT fallback)")

			luaKeys := scripter.capturedScriptKeys()
			require.Len(t, luaKeys, 1, "get_balances_near_expiration.lua receives exactly 1 KEY (schedule key)")
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
// AC-7: RemoveBalanceSyncKey namespaces schedule key and lock prefix.
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
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			conn, scripter := newScriptCapturingConnection(t)
			repo := &RedisConsumerRepository{conn: conn, balanceSyncEnabled: true}

			ctx := context.Background()
			if tc.tenantID != "" {
				ctx = tmcore.SetTenantIDInContext(ctx, tc.tenantID)
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
// before the namespacing change (AC-8 and AC-9).
func TestKeyNamespacing_BackwardsCompatible_NoTenantInContext(t *testing.T) {
	t.Parallel()

	// Use context.Background() — no tenantID injected — simulates single-tenant mode.
	ctx := context.Background()

	t.Run("simple_key_methods_unchanged", func(t *testing.T) {
		t.Parallel()

		conn, recorder := newRecordingConnection(t)
		repo := &RedisConsumerRepository{conn: conn, balanceSyncEnabled: false}

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
		repo := &RedisConsumerRepository{conn: conn, balanceSyncEnabled: false}

		originalKeys := []string{"key:a", "key:b"}
		_, _ = repo.MGet(ctx, originalKeys)

		require.Len(t, recorder.mgetCalls, 1)
		assert.Equal(t, originalKeys, recorder.mgetCalls[0],
			"MGet: keys must be unchanged without tenant")
	})

	t.Run("queue_operations_keys_unchanged", func(t *testing.T) {
		t.Parallel()

		conn, recorder := newRecordingConnection(t)
		repo := &RedisConsumerRepository{conn: conn, balanceSyncEnabled: false}

		msgKey := "tx:orig-key"
		_ = repo.AddMessageToQueue(ctx, msgKey, []byte("data"))

		require.Len(t, recorder.hsetCalls, 1)
		assert.Equal(t, TransactionBackupQueue, recorder.hsetCalls[0].Key,
			"AddMessageToQueue: queue key must be unchanged without tenant")
		assert.Equal(t, msgKey, recorder.hsetCalls[0].Values[0],
			"AddMessageToQueue: message field key must be unchanged without tenant")
	})
}
