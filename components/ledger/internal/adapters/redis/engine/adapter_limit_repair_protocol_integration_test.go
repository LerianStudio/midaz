//go:build integration

// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package engine

import (
	"bytes"
	"context"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/redis/balancecache"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/services/command"
)

func TestIntegration_AdapterExecute_BoundsAccountingAttemptsDuringCASContention(t *testing.T) {
	if testing.Short() {
		t.Skip("requires Valkey")
	}

	ctx := context.Background()
	inspector, address, password := newAdapterValkey(t)
	input, limits := richAdapterExecution(t)
	keys, cacheKey, raw := seedProtocolNoncanonicalLimit(t, ctx, inspector, input)
	require.NoError(t, inspector.ScriptFlush(ctx).Err())

	proxy := newAccountingProxy(t, address, false)
	shared := newProtocolProxyClient(t, proxy, password)
	var mutations atomic.Int32
	hookErrors := make(chan error, 2)
	proxy.setReplyHook(func(command string, _ []byte) {
		if command != "MGET" {
			return
		}

		mutation := mutations.Add(1)
		current, err := inspector.Get(ctx, cacheKey).Bytes()
		if err != nil {
			hookErrors <- err
			return
		}

		old := []byte(`"OverdraftLimit":"1E+3"`)
		if mutation == 2 {
			old = []byte(`"OverdraftLimit":"1000.00"`)
		}
		replacement := []byte(`"OverdraftLimit":"1000.00"`)
		if mutation == 2 {
			replacement = []byte(`"OverdraftLimit":"01000"`)
		}
		if bytes.Count(current, old) != 1 {
			hookErrors <- fmt.Errorf("concurrent fixture missing authoritative token on mutation %d", mutation)
			return
		}

		changed := bytes.Replace(current, old, replacement, 1)
		if err := inspector.Do(ctx, "SET", cacheKey, changed, "KEEPTTL").Err(); err != nil {
			hookErrors <- err
		}
	})

	adapter, err := newAdapterWithLimits(&integrationClientProvider{client: shared}, limits)
	require.NoError(t, err)
	result, err := adapter.Execute(ctx, input)
	require.Nil(t, result)
	assertAdapterTechnical(t, err, "normalization_required", false)
	require.Equal(t, int32(2), mutations.Load())
	close(hookErrors)
	for hookErr := range hookErrors {
		require.NoError(t, hookErr)
	}

	persisted, err := inspector.Get(ctx, cacheKey).Bytes()
	require.NoError(t, err)
	want := bytes.Replace(raw, []byte(`"OverdraftLimit":"1E+3"`), []byte(`"OverdraftLimit":"01000"`), 1)
	require.Equal(t, want, persisted, "conditional repair must not overwrite concurrent limit changes")
	require.Equal(t, 5, proxy.count("EVALSHA"), "three accounting attempts and two CAS attempts are the fixed ceiling")
	require.Equal(t, 2, proxy.count("EVAL"), "each script may use confirmed NOSCRIPT fallback only once")
	require.Equal(t, 2, proxy.count("MGET"))

	state := captureAdapterState(t, inspector, keys)
	for _, key := range []string{keys.Schedule, keys.Recovery, keys.Receipts, keys.Guards} {
		require.Equal(t, []any{"", int64(-2)}, state[key], "contention exhaustion must not write accounting sidecars")
	}
}

func TestIntegration_AdapterExecute_DoesNotReplayAccountingAfterRepairTransportAmbiguity(t *testing.T) {
	if testing.Short() {
		t.Skip("requires Valkey")
	}

	ctx := context.Background()
	inspector, address, password := newAdapterValkey(t)
	input, limits := richAdapterExecution(t)
	keys, cacheKey, _ := seedProtocolNoncanonicalLimit(t, ctx, inspector, input)
	require.NoError(t, inspector.ScriptFlush(ctx).Err())
	before := captureAdapterState(t, inspector, keys)

	proxy := newAccountingProxy(t, address, true)
	shared := newProtocolProxyClient(t, proxy, password)
	adapter, err := newAdapterWithLimits(&integrationClientProvider{client: shared}, limits)
	require.NoError(t, err)
	result, err := adapter.Execute(ctx, input)
	require.Nil(t, result)
	assertAdapterTechnical(t, err, "normalization_repair_failed", true)

	require.Equal(t, 2, proxy.count("EVALSHA"), "one accounting attempt and one repair attempt are allowed")
	require.Equal(t, 2, proxy.count("EVAL"), "only confirmed NOSCRIPT may fall back")
	require.Equal(t, 1, proxy.count("MGET"))
	persisted, err := inspector.Get(ctx, cacheKey).Bytes()
	require.NoError(t, err)
	require.Contains(t, string(persisted), `"OverdraftLimit":"1000"`, "the dropped response may hide a committed repair")
	after := captureAdapterState(t, inspector, keys)
	require.Equal(t, before[cacheKey].([]any)[1], after[cacheKey].([]any)[1], "repair must preserve expiry even when its reply is lost")
	assertProtocolSidecarsAbsent(t, after, keys)
}

func TestIntegration_AdapterExecute_MissingBalanceAfterNormalizationIsNotRecreated(t *testing.T) {
	if testing.Short() {
		t.Skip("requires Valkey")
	}

	ctx := context.Background()
	inspector, address, password := newAdapterValkey(t)
	input, limits := richAdapterExecution(t)
	keys, cacheKey, _ := seedProtocolNoncanonicalLimit(t, ctx, inspector, input)
	require.NoError(t, inspector.ScriptFlush(ctx).Err())

	proxy := newAccountingProxy(t, address, false)
	shared := newProtocolProxyClient(t, proxy, password)
	var deleted atomic.Bool
	deleteErrors := make(chan error, 1)
	proxy.setReplyHook(func(command string, reply []byte) {
		if command != "EVAL" && command != "EVALSHA" || !bytes.Contains(reply, []byte("BALANCE_LIMIT_NORMALIZATION_REQUIRED:")) || !deleted.CompareAndSwap(false, true) {
			return
		}

		if err := inspector.Del(ctx, cacheKey).Err(); err != nil {
			deleteErrors <- err
		}
	})

	adapter, err := newAdapterWithLimits(&integrationClientProvider{client: shared}, limits)
	require.NoError(t, err)
	result, err := adapter.Execute(ctx, input)
	require.Nil(t, result)
	assertAdapterTechnical(t, err, "normalization_balance_missing", false)
	require.True(t, deleted.Load(), "fixture must delete after the confirmed normalization response")
	close(deleteErrors)
	for deleteErr := range deleteErrors {
		require.NoError(t, deleteErr)
	}

	exists, err := inspector.Exists(ctx, cacheKey).Result()
	require.NoError(t, err)
	require.Zero(t, exists, "repair must never recreate a concurrently missing balance")
	require.Equal(t, 1, proxy.count("EVALSHA"), "missing repair input must stop after one accounting attempt")
	require.Equal(t, 1, proxy.count("EVAL"))
	require.Equal(t, 1, proxy.count("MGET"))
	assertProtocolSidecarsAbsent(t, captureAdapterState(t, inspector, keys), keys)
}

func seedProtocolNoncanonicalLimit(t *testing.T, ctx context.Context, inspector *redis.Client, input command.EngineExecution) (resolvedExecutionKeys, string, []byte) {
	t.Helper()
	hot := input.Execution.Balances[0]
	hot.Available = decimal.NewFromInt(120)
	hot.OverdraftLimitEnabled = true
	hot.OverdraftLimit = decimal.NewFromInt(1000)
	raw, err := balancecache.Encode(hot, balancecache.FormatDual)
	require.NoError(t, err)
	require.Equal(t, 1, bytes.Count(raw, []byte(`"OverdraftLimit":"1000"`)))
	raw = bytes.Replace(raw, []byte(`"OverdraftLimit":"1000"`), []byte(`"OverdraftLimit":"1E+3"`), 1)
	keys, err := resolveAdapterKeys(ctx, input.Execution)
	require.NoError(t, err)
	cacheKey := keys.Balances[hot.BalanceRef].Balance
	require.NoError(t, inspector.Set(ctx, cacheKey, raw, time.Hour).Err())
	return keys, cacheKey, raw
}

func newProtocolProxyClient(t *testing.T, proxy *accountingProxy, password string) *redis.Client {
	t.Helper()
	shared := redis.NewClient(&redis.Options{
		Addr: proxy.listener.Addr().String(), Username: "default", Password: password, DB: 2, Protocol: 2,
		MaxRetries: 3, TLSConfig: proxy.clientTLS,
	})
	t.Cleanup(func() { require.NoError(t, shared.Close()) })
	return shared
}

func assertProtocolSidecarsAbsent(t *testing.T, state map[string]any, keys resolvedExecutionKeys) {
	t.Helper()
	for _, key := range []string{keys.Schedule, keys.Recovery, keys.Receipts, keys.Guards} {
		require.Equal(t, []any{"", int64(-2)}, state[key], "repair protocol must not write accounting sidecars")
	}
}
