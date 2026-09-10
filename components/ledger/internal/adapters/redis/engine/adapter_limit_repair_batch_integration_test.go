//go:build integration

// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package engine

import (
	"bytes"
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/redis/balancecache"
	core "github.com/LerianStudio/midaz/v4/components/ledger/internal/domain/accounting"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/services/command"
)

func TestIntegration_AdapterExecute_LimitRepairBatchPrevalidatesAllBalances(t *testing.T) {
	ctx := context.Background()
	inspector, address, password := newAdapterValkey(t)
	input, limits := adapterLimitRepairBatchExecution(t)
	keys, err := resolveAdapterKeys(ctx, input.Execution)
	require.NoError(t, err)

	expiresAt := time.Date(2100, time.January, 2, 3, 4, 5, 0, time.UTC).UnixMilli()
	first := input.Execution.Balances[0]
	first.Available = decimal.NewFromInt(20)
	firstRaw := adapterNoncanonicalLimit(t, first, "1000.00")
	secondRaw := adapterNoncanonicalLimit(t, input.Execution.Balances[1], "2000.00")
	firstKey := keys.Balances[first.BalanceRef].Balance
	secondKey := keys.Balances[input.Execution.Balances[1].BalanceRef].Balance
	setAdapterBalanceAt(t, inspector, firstKey, firstRaw, expiresAt)
	setAdapterBalanceAt(t, inspector, secondKey, secondRaw, expiresAt)

	proxy := newAccountingProxy(t, address, false)
	hookResult := make(chan error, 1)
	proxy.setReplyHook(func(_ string, reply []byte) {
		if bytes.Contains(reply, []byte("BALANCE_LIMIT_NORMALIZATION_REQUIRED:")) {
			hookResult <- inspector.SetArgs(ctx, secondKey, "{", redis.SetArgs{KeepTTL: true}).Err()
		}
	})
	shared := redis.NewClient(&redis.Options{
		Addr: proxy.listener.Addr().String(), Username: "default", Password: password,
		DB: 2, Protocol: 2, TLSConfig: proxy.clientTLS, MaxRetries: 3,
	})
	t.Cleanup(func() { require.NoError(t, shared.Close()) })
	adapter, err := newAdapterWithLimits(&integrationClientProvider{client: shared}, limits)
	require.NoError(t, err)

	result, err := adapter.Execute(ctx, input)
	require.Nil(t, result)
	assertAdapterTechnical(t, err, "normalization_invalid_balance", false)
	select {
	case hookErr := <-hookResult:
		require.NoError(t, hookErr)
	default:
		t.Fatal("accounting did not report the complete noncanonical batch")
	}
	requireAdapterCachedValue(t, inspector, firstKey, firstRaw, expiresAt)
	requireAdapterCachedValue(t, inspector, secondKey, []byte("{"), expiresAt)
	requireAdapterRepairSidecarsAbsent(t, inspector, keys)
}

func TestIntegration_AdapterExecute_LimitRepairBatchPreservesFinancialStateOnRefusal(t *testing.T) {
	ctx := context.Background()
	inspector, _, _ := newAdapterValkey(t)
	input, limits := adapterLimitRepairBatchExecution(t)
	keys, err := resolveAdapterKeys(ctx, input.Execution)
	require.NoError(t, err)
	adapter, err := newAdapterWithLimits(&integrationClientProvider{client: inspector}, limits)
	require.NoError(t, err)

	expiresAt := time.Date(2100, time.February, 3, 4, 5, 6, 0, time.UTC).UnixMilli()
	first := input.Execution.Balances[0]
	first.Available = decimal.NewFromInt(20)
	firstRaw := adapterNoncanonicalLimit(t, first, "1000.00")
	secondRaw := adapterNoncanonicalLimit(t, input.Execution.Balances[1], "2000.00")
	firstKey := keys.Balances[first.BalanceRef].Balance
	secondKey := keys.Balances[input.Execution.Balances[1].BalanceRef].Balance
	setAdapterBalanceAt(t, inspector, firstKey, firstRaw, expiresAt)
	setAdapterBalanceAt(t, inspector, secondKey, secondRaw, expiresAt)

	result, err := adapter.Execute(ctx, input)
	require.Nil(t, result)
	var failure *core.Failure
	require.ErrorAs(t, err, &failure)
	require.Equal(t, core.FailureInsufficientFunds, failure.Code)
	requireAdapterCachedValue(t, inspector, firstKey, adapterCanonicalLimit(t, firstRaw, "1000.00", "1000"), expiresAt)
	secondExpected := adapterCanonicalLimit(t, secondRaw, "2000.00", "2000")
	lowerShadow := []byte(`"overdraftLimit":"1000"`)
	require.Equal(t, 1, bytes.Count(secondExpected, lowerShadow))
	secondExpected = bytes.Replace(secondExpected, lowerShadow, []byte(`"overdraftLimit":"2000"`), 1)
	requireAdapterCachedValue(t, inspector, secondKey, secondExpected, expiresAt)
	requireAdapterRepairSidecarsAbsent(t, inspector, keys)
}

func TestIntegration_AdapterExecute_LimitRepairPreservesPersistentKey(t *testing.T) {
	ctx := context.Background()
	inspector, _, _ := newAdapterValkey(t)
	input, limits := richAdapterExecution(t)
	keys, err := resolveAdapterKeys(ctx, input.Execution)
	require.NoError(t, err)
	adapter, err := newAdapterWithLimits(&integrationClientProvider{client: inspector}, limits)
	require.NoError(t, err)

	live := input.Execution.Balances[0]
	live.Available = decimal.NewFromInt(20)
	raw := adapterNoncanonicalLimit(t, live, "1000.00")
	key := keys.Balances[live.BalanceRef].Balance
	require.NoError(t, inspector.Set(ctx, key, raw, 0).Err())

	result, err := adapter.Execute(ctx, input)
	require.Nil(t, result)
	var failure *core.Failure
	require.ErrorAs(t, err, &failure)
	require.Equal(t, core.FailureInsufficientFunds, failure.Code)
	requireAdapterCachedValue(t, inspector, key, adapterCanonicalLimit(t, raw, "1000.00", "1000"), -1)
	requireAdapterRepairSidecarsAbsent(t, inspector, keys)
}

func adapterLimitRepairBatchExecution(t *testing.T) (command.EngineExecution, Limits) {
	t.Helper()
	input, limits := richAdapterExecution(t)
	second := input.Execution.Balances[0]
	second.ID = uuid.NewSHA1(uuid.NameSpaceOID, []byte(t.Name()+":second-balance"))
	second.AccountID = uuid.NewSHA1(uuid.NameSpaceOID, []byte(t.Name()+":second-account"))
	second.Alias, second.BalanceRef = "@batch", "@batch#default"
	input.Execution.Balances = append(input.Execution.Balances, second)
	return input, limits
}

func adapterNoncanonicalLimit(t *testing.T, snapshot core.BalanceSnapshot, limit string) []byte {
	t.Helper()
	encoded, err := balancecache.Encode(snapshot, balancecache.FormatDual)
	require.NoError(t, err)
	needle := []byte(`"OverdraftLimit":"` + snapshot.OverdraftLimit.String() + `"`)
	require.Equal(t, 1, bytes.Count(encoded, needle))
	return bytes.Replace(encoded, needle, []byte(`"OverdraftLimit":"`+limit+`"`), 1)
}

func adapterCanonicalLimit(t *testing.T, raw []byte, from, to string) []byte {
	t.Helper()
	needle := []byte(`"OverdraftLimit":"` + from + `"`)
	require.Equal(t, 1, bytes.Count(raw, needle))
	return bytes.Replace(raw, needle, []byte(`"OverdraftLimit":"`+to+`"`), 1)
}

func setAdapterBalanceAt(t *testing.T, client *redis.Client, key string, raw []byte, expiresAt int64) {
	t.Helper()
	require.NoError(t, client.Set(context.Background(), key, raw, 0).Err())
	require.NoError(t, client.Do(context.Background(), "PEXPIREAT", key, expiresAt).Err())
}

func requireAdapterCachedValue(t *testing.T, client *redis.Client, key string, expected []byte, expectedExpiry int64) {
	t.Helper()
	actual, err := client.Get(context.Background(), key).Bytes()
	require.NoError(t, err)
	require.Equal(t, expected, actual)
	expiry, err := client.Do(context.Background(), "PEXPIRETIME", key).Int64()
	require.NoError(t, err)
	require.Equal(t, expectedExpiry, expiry)
}

func requireAdapterRepairSidecarsAbsent(t *testing.T, client *redis.Client, keys resolvedExecutionKeys) {
	t.Helper()
	inventory := []string{keys.Schedule, keys.Recovery, keys.Guards, keys.Receipts}
	for _, pair := range keys.Balances {
		inventory = append(inventory, pair.Deleted)
	}
	exists, err := client.Exists(context.Background(), inventory...).Result()
	require.NoError(t, err)
	require.Zero(t, exists)
}
