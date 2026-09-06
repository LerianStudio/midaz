//go:build integration

// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package engine

import (
	"bytes"
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/redis/balancecache"
)

func TestIntegration_AdapterExecute_RepairsNoncanonicalHotCacheLimit(t *testing.T) {
	if testing.Short() {
		t.Skip("requires Valkey")
	}

	ctx := context.Background()
	inspector, _, _ := newAdapterValkey(t)
	input, limits := richAdapterExecution(t)
	input.Request.Balances[0].Available = decimal.NewFromInt(100)

	hot := input.Request.Balances[0]
	hot.Available = decimal.NewFromInt(120)
	hot.OverdraftLimitEnabled = true
	hot.OverdraftLimit = decimal.NewFromInt(1000)
	encoded, err := balancecache.Encode(hot, balancecache.FormatDual)
	require.NoError(t, err)
	var fields map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(encoded, &fields))
	fields["OverdraftLimit"] = json.RawMessage(`"1E+3"`)
	encoded, err = json.Marshal(fields)
	require.NoError(t, err)

	keys, err := resolveAdapterKeys(ctx, input.Request)
	require.NoError(t, err)
	cacheKey := keys.Balances[input.Request.Balances[0].BalanceRef].Balance
	require.NoError(t, inspector.Set(ctx, cacheKey, encoded, time.Hour).Err())
	before := captureAdapterState(t, inspector, keys)

	adapter, err := NewAdapter(&integrationClientProvider{client: inspector}, limits)
	require.NoError(t, err)
	result, err := adapter.Execute(ctx, input)
	require.NoError(t, err)
	require.Len(t, result.Final, 1)
	require.True(t, result.Final[0].Available.Equal(decimal.NewFromInt(90)), "execution must use the hot cached balance")
	require.True(t, result.Final[0].OverdraftLimit.Equal(decimal.NewFromInt(1000)))

	persisted, err := inspector.Get(ctx, cacheKey).Bytes()
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal(persisted, &fields))
	require.JSONEq(t, `"1000"`, string(fields["OverdraftLimit"]))
	require.JSONEq(t, `"1000"`, string(fields["overdraftLimit"]))
	after := captureAdapterState(t, inspector, keys)
	require.Greater(t, after[cacheKey].([]any)[1].(int64), before[cacheKey].([]any)[1].(int64), "successful accounting must refresh the balance expiry")
}

func TestIntegration_AdapterExecute_RepairThenRefusalPreservesHotBalance(t *testing.T) {
	if testing.Short() {
		t.Skip("requires Valkey")
	}

	ctx := context.Background()
	inspector, _, _ := newAdapterValkey(t)
	tests := []struct {
		name       string
		format     balancecache.Format
		find       []byte
		replace    []byte
		wantRepair []byte
	}{
		{
			name:       "dual uses uppercase authority and preserves divergent lower shadow",
			format:     balancecache.FormatDual,
			find:       []byte(`"OverdraftLimit":"1000"`),
			replace:    []byte(`"OverdraftLimit":"1E+3"`),
			wantRepair: []byte(`"OverdraftLimit":"1000"`),
		},
		{
			name:       "new only uses lowercase authority",
			format:     balancecache.FormatNewOnly,
			find:       []byte(`"overdraftLimit":"1000"`),
			replace:    []byte(`"overdraftLimit":"1000.00"`),
			wantRepair: []byte(`"overdraftLimit":"1000"`),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			input, limits := richAdapterExecution(t)
			hot := input.Request.Balances[0]
			hot.Available = decimal.NewFromInt(20)
			hot.OverdraftLimitEnabled = true
			hot.OverdraftLimit = decimal.NewFromInt(1000)
			encoded, err := balancecache.Encode(hot, tt.format)
			require.NoError(t, err)
			encoded = replaceLimitTestBytes(t, encoded, tt.find, tt.replace)
			if tt.format == balancecache.FormatDual {
				encoded = replaceLimitTestBytes(t, encoded, []byte(`"overdraftLimit":"1000"`), []byte(`"overdraftLimit":"777"`))
			}
			encoded = append([]byte(`{"opaqueNumber":1E+3,`), encoded[1:]...)
			want := replaceLimitTestBytes(t, encoded, tt.replace, tt.wantRepair)

			keys, err := resolveAdapterKeys(ctx, input.Request)
			require.NoError(t, err)
			cacheKey := keys.Balances[hot.BalanceRef].Balance
			require.NoError(t, inspector.Set(ctx, cacheKey, encoded, time.Hour).Err())
			before := captureAdapterState(t, inspector, keys)

			adapter, err := NewAdapter(&integrationClientProvider{client: inspector}, limits)
			require.NoError(t, err)
			result, err := adapter.Execute(ctx, input)
			require.Nil(t, result)
			require.ErrorContains(t, err, "insufficient_funds")

			persisted, err := inspector.Get(ctx, cacheKey).Bytes()
			require.NoError(t, err)
			require.Equal(t, want, persisted, "refusal may only leave the authoritative limit normalized")
			after := captureAdapterState(t, inspector, keys)
			require.Equal(t, before[cacheKey].([]any)[1], after[cacheKey].([]any)[1], "repair must preserve the exact expiry")
			for key, beforeValue := range before {
				if key != cacheKey {
					require.Equal(t, beforeValue, after[key], "repair followed by refusal must not write accounting sidecars")
				}
			}
		})
	}
}

func TestIntegration_AdapterExecute_InvalidNoncanonicalLimitDoesNotMutate(t *testing.T) {
	if testing.Short() {
		t.Skip("requires Valkey")
	}

	ctx := context.Background()
	inspector, _, _ := newAdapterValkey(t)
	for _, invalid := range []string{`"not-a-decimal"`, `"-1.0"`} {
		t.Run(invalid, func(t *testing.T) {
			input, limits := richAdapterExecution(t)
			hot := input.Request.Balances[0]
			hot.OverdraftLimitEnabled = true
			hot.OverdraftLimit = decimal.NewFromInt(1000)
			encoded, err := balancecache.Encode(hot, balancecache.FormatDual)
			require.NoError(t, err)
			encoded = replaceLimitTestBytes(t, encoded, []byte(`"OverdraftLimit":"1000"`), []byte(`"OverdraftLimit":`+invalid))

			keys, err := resolveAdapterKeys(ctx, input.Request)
			require.NoError(t, err)
			cacheKey := keys.Balances[hot.BalanceRef].Balance
			require.NoError(t, inspector.Set(ctx, cacheKey, encoded, time.Hour).Err())
			before := captureAdapterState(t, inspector, keys)

			adapter, err := NewAdapter(&integrationClientProvider{client: inspector}, limits)
			require.NoError(t, err)
			result, err := adapter.Execute(ctx, input)
			require.Nil(t, result)
			assertAdapterTechnical(t, err, "normalization_invalid_balance", false)
			require.Equal(t, before, captureAdapterState(t, inspector, keys), "invalid authoritative data must not be repaired or write accounting state")
		})
	}
}

func replaceLimitTestBytes(t *testing.T, raw, old, replacement []byte) []byte {
	t.Helper()
	require.Equal(t, 1, bytes.Count(raw, old), "test fixture must contain exactly one target token")
	return bytes.Replace(raw, old, replacement, 1)
}
