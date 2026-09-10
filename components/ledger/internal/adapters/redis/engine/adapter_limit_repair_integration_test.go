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
	input.Execution.Balances[0].Available = decimal.NewFromInt(100)

	hot := input.Execution.Balances[0]
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

	keys, err := resolveAdapterKeys(ctx, input.Execution)
	require.NoError(t, err)
	cacheKey := keys.Balances[input.Execution.Balances[0].BalanceRef].Balance
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

func TestIntegration_AdapterExecute_LimitRepairPromotesLegacyCache(t *testing.T) {
	if testing.Short() {
		t.Skip("requires Valkey")
	}

	ctx := context.Background()
	inspector, _, _ := newAdapterValkey(t)
	pairs := [][2]string{
		{"ID", "id"},
		{"AccountID", "accountId"},
		{"AccountType", "accountType"},
		{"AssetCode", "assetCode"},
		{"Alias", "alias"},
		{"Key", "key"},
		{"Direction", "direction"},
		{"BalanceScope", "balanceScope"},
		{"Available", "available"},
		{"OnHold", "onHold"},
		{"OverdraftUsed", "overdraftUsed"},
		{"Version", "version"},
		{"AllowSending", "allowSending"},
		{"AllowReceiving", "allowReceiving"},
		{"AllowOverdraft", "allowOverdraft"},
		{"OverdraftLimitEnabled", "overdraftLimitEnabled"},
		{"OverdraftLimit", "overdraftLimit"},
	}
	for _, name := range []string{"existing alias", "scoped alias fallback", "qualified legacy key"} {
		t.Run(name, func(t *testing.T) {
			input, limits := richAdapterExecution(t)
			input.Execution.Balances[0].Version = 9007199254740993
			hot := input.Execution.Balances[0]
			hot.Available = decimal.NewFromInt(20)
			hot.OverdraftLimitEnabled = true
			hot.OverdraftLimit = decimal.NewFromInt(1000)
			encoded, err := balancecache.Encode(hot, balancecache.FormatDual)
			require.NoError(t, err)
			var fields map[string]json.RawMessage
			require.NoError(t, json.Unmarshal(encoded, &fields))
			for _, pair := range pairs {
				delete(fields, pair[1])
			}
			delete(fields, "SchemaVersion")
			if name == "scoped alias fallback" {
				delete(fields, "Alias")
			}
			if name == "qualified legacy key" {
				fields["Key"], err = json.Marshal(hot.BalanceRef)
				require.NoError(t, err)
			}
			fields["OverdraftLimit"] = json.RawMessage(`"1E+3"`)
			fields["extension"] = json.RawMessage(`{"exact":9007199254740995}`)
			encoded, err = json.Marshal(fields)
			require.NoError(t, err)
			keys, err := resolveAdapterKeys(ctx, input.Execution)
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
			var repaired map[string]json.RawMessage
			require.NoError(t, json.Unmarshal(persisted, &repaired))
			for _, pair := range pairs {
				require.Contains(t, repaired, pair[0])
				require.Contains(t, repaired, pair[1])
			}
			require.Equal(t, `2`, string(repaired["SchemaVersion"]))
			expectedAlias, err := json.Marshal(hot.Alias)
			require.NoError(t, err)
			require.Equal(t, string(expectedAlias), string(repaired["Alias"]))
			require.Equal(t, string(expectedAlias), string(repaired["alias"]))
			if name == "qualified legacy key" {
				require.Equal(t, string(fields["Key"]), string(repaired["Key"]))
				require.Equal(t, `"default"`, string(repaired["key"]))
			}
			require.Equal(t, `9007199254740993`, string(repaired["Version"]))
			require.Equal(t, `"9007199254740993"`, string(repaired["version"]))
			require.Equal(t, `"20"`, string(repaired["Available"]))
			require.Equal(t, `"1000"`, string(repaired["OverdraftLimit"]))
			require.Equal(t, `"1000"`, string(repaired["overdraftLimit"]))
			require.Equal(t, `{"exact":9007199254740995}`, string(repaired["extension"]))
			after := captureAdapterState(t, inspector, keys)
			require.Equal(t, before[cacheKey].([]any)[1], after[cacheKey].([]any)[1])
			for key, state := range before {
				if key != cacheKey {
					require.Equal(t, state, after[key])
				}
			}
		})
	}
}

func TestIntegration_AdapterExecute_RepairThenRefusalPreservesHotBalance(t *testing.T) {
	if testing.Short() {
		t.Skip("requires Valkey")
	}

	ctx := context.Background()
	inspector, _, _ := newAdapterValkey(t)
	tests := []struct {
		name             string
		format           balancecache.Format
		find             []byte
		replace          []byte
		wantRepair       []byte
		lowerValue       []byte
		lowerKey         []byte
		lowerBeforeUpper bool
		nestedDecoy      bool
	}{
		{
			name:       "dual quoted numeric lower shadow",
			format:     balancecache.FormatDual,
			find:       []byte(`"OverdraftLimit":"1000"`),
			replace:    []byte(`"OverdraftLimit":"1E+3"`),
			wantRepair: []byte(`"OverdraftLimit":"1000"`),
			lowerValue: []byte(`"99"`),
		},
		{
			name:       "dual numeric lower shadow",
			format:     balancecache.FormatDual,
			find:       []byte(`"OverdraftLimit":"1000"`),
			replace:    []byte(`"OverdraftLimit":"1E+3"`),
			wantRepair: []byte(`"OverdraftLimit":"1000"`),
			lowerValue: []byte(`99`),
		},
		{
			name:       "dual boolean lower shadow",
			format:     balancecache.FormatDual,
			find:       []byte(`"OverdraftLimit":"1000"`),
			replace:    []byte(`"OverdraftLimit":"1E+3"`),
			wantRepair: []byte(`"OverdraftLimit":"1000"`),
			lowerValue: []byte(`true`),
		},
		{
			name:       "dual null lower shadow",
			format:     balancecache.FormatDual,
			find:       []byte(`"OverdraftLimit":"1000"`),
			replace:    []byte(`"OverdraftLimit":"1E+3"`),
			wantRepair: []byte(`"OverdraftLimit":"1000"`),
			lowerValue: []byte(`null`),
		},
		{
			name:       "dual object lower shadow",
			format:     balancecache.FormatDual,
			find:       []byte(`"OverdraftLimit":"1000"`),
			replace:    []byte(`"OverdraftLimit":"1E+3"`),
			wantRepair: []byte(`"OverdraftLimit":"1000"`),
			lowerValue: []byte(`{"nested":99}`),
		},
		{
			name:       "dual array lower shadow",
			format:     balancecache.FormatDual,
			find:       []byte(`"OverdraftLimit":"1000"`),
			replace:    []byte(`"OverdraftLimit":"1E+3"`),
			wantRepair: []byte(`"OverdraftLimit":"1000"`),
			lowerValue: []byte(`[99]`),
		},
		{
			name:             "dual lower field before uppercase field",
			format:           balancecache.FormatDual,
			find:             []byte(`"OverdraftLimit":"1000"`),
			replace:          []byte(`"OverdraftLimit":"1E+3"`),
			wantRepair:       []byte(`"OverdraftLimit":"1000"`),
			lowerValue:       []byte(`"99"`),
			lowerBeforeUpper: true,
		},
		{
			name:        "dual escaped lowercase key with nested decoy",
			format:      balancecache.FormatDual,
			find:        []byte(`"OverdraftLimit":"1000"`),
			replace:     []byte(`"OverdraftLimit":"1E+3"`),
			wantRepair:  []byte(`"OverdraftLimit":"1000"`),
			lowerValue:  []byte(`"99"`),
			lowerKey:    []byte(`"overdraft\u004cimit"`),
			nestedDecoy: true,
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
			hot := input.Execution.Balances[0]
			hot.Available = decimal.NewFromInt(20)
			hot.OverdraftLimitEnabled = true
			hot.OverdraftLimit = decimal.NewFromInt(1000)
			encoded, err := balancecache.Encode(hot, tt.format)
			require.NoError(t, err)
			encoded = replaceLimitTestBytes(t, encoded, tt.find, tt.replace)
			if tt.format == balancecache.FormatDual {
				lowerKey := []byte(`"overdraftLimit"`)
				if len(tt.lowerKey) > 0 {
					lowerKey = tt.lowerKey
				}
				lowerField := append(append(append([]byte{}, lowerKey...), ':'), tt.lowerValue...)
				encoded = replaceLimitTestBytes(t, encoded, []byte(`"overdraftLimit":"1000"`), lowerField)
				if tt.nestedDecoy {
					encoded = append([]byte(`{"nested":{"overdraftLimit":"777"},`), encoded[1:]...)
				}
				if tt.lowerBeforeUpper {
					upperField := []byte(`"OverdraftLimit":"1E+3"`)
					lowerStart := bytes.Index(encoded, lowerField)
					upperStart := bytes.Index(encoded, upperField)
					require.GreaterOrEqual(t, lowerStart, 0)
					require.GreaterOrEqual(t, upperStart, 0)
					require.Less(t, upperStart, lowerStart)
					betweenFields := encoded[upperStart+len(upperField) : lowerStart]
					trailingFields := encoded[lowerStart+len(lowerField):]
					encoded = append(append(append(append([]byte{}, encoded[:upperStart]...), lowerField...), betweenFields...), upperField...)
					encoded = append(encoded, trailingFields...)
				}
			}
			encoded = append([]byte(`{"opaqueNumber":1E+3,`), encoded[1:]...)
			want := replaceLimitTestBytes(t, encoded, tt.replace, tt.wantRepair)
			if tt.format == balancecache.FormatDual {
				lowerKey := []byte(`"overdraftLimit"`)
				if len(tt.lowerKey) > 0 {
					lowerKey = tt.lowerKey
				}
				lowerValue := tt.lowerValue
				if len(lowerValue) == 0 {
					lowerValue = []byte(`"99"`)
				}
				lowerField := append(append(append([]byte{}, lowerKey...), ':'), lowerValue...)
				canonicalLowerField := append(append(append([]byte{}, lowerKey...), ':'), []byte(`"1000"`)...)
				want = replaceLimitTestBytes(t, want, lowerField, canonicalLowerField)
			}

			keys, err := resolveAdapterKeys(ctx, input.Execution)
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
			var expectedFields, repairedFields map[string]json.RawMessage
			require.NoError(t, json.Unmarshal(want, &expectedFields))
			require.NoError(t, json.Unmarshal(persisted, &repairedFields))
			for name, value := range expectedFields {
				require.Equal(t, string(value), string(repairedFields[name]), "repair must preserve the exact value of %s", name)
			}
			extraFields := 0
			if tt.format == balancecache.FormatNewOnly {
				extraFields = 17
			}
			require.Len(t, repairedFields, len(expectedFields)+extraFields, "repair adds only missing dual fields")
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
			hot := input.Execution.Balances[0]
			hot.OverdraftLimitEnabled = true
			hot.OverdraftLimit = decimal.NewFromInt(1000)
			encoded, err := balancecache.Encode(hot, balancecache.FormatDual)
			require.NoError(t, err)
			encoded = replaceLimitTestBytes(t, encoded, []byte(`"OverdraftLimit":"1000"`), []byte(`"OverdraftLimit":`+invalid))

			keys, err := resolveAdapterKeys(ctx, input.Execution)
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
