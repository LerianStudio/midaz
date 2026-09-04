//go:build integration

// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package redis

import (
	"context"
	_ "embed"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

//go:embed scripts/normalize_balance_limit.lua
var balanceLimitNormalizationTestLua string

func TestIntegration_BalanceLimitNormalization_PreservesUnrelatedBytesAndTTL(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	infra := setupRedisIntegrationInfra(t)
	client := infra.redisContainer.Client
	ctx := context.Background()
	key := "{transactions}:normalization:preserve"
	raw := `{"Available":"120","OnHold":"37.00","OverdraftUsed":"0.3","OverdraftLimit":"1e+2","Version":9007199254740993,"AllowOverdraft":true,"AccountType":"liability","Direction":"debit","Unknown":{"OverdraftLimit":"8e+1","empty":[],"numbers":[-0,1e9999,1.00,0.001,-1e-9999,1E+3],"text":"escaped \\\" OverdraftLimit"},"Optional":null}`

	for _, expiration := range []time.Duration{time.Minute, 0} {
		t.Run(expiration.String(), func(t *testing.T) {
			require.NoError(t, client.Set(ctx, key, raw, expiration).Err())
			before, err := client.Do(ctx, "PEXPIRETIME", key).Int64()
			require.NoError(t, err)

			status, err := client.Eval(ctx, balanceLimitNormalizationTestLua, []string{key}, "1e+2", "100").Int64()
			require.NoError(t, err)
			require.EqualValues(t, 1, status)

			stored, err := client.Get(ctx, key).Result()
			require.NoError(t, err)
			require.Equal(t, strings.Replace(raw, `"OverdraftLimit":"1e+2"`, `"OverdraftLimit":"100"`, 1), stored)
			after, err := client.Do(ctx, "PEXPIRETIME", key).Int64()
			require.NoError(t, err)
			require.Equal(t, before, after, "the original absolute expiry must not change")
		})
	}
}

func TestIntegration_BalanceLimitNormalization_ConditionalUpdate(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	infra := setupRedisIntegrationInfra(t)
	client := infra.redisContainer.Client
	ctx := context.Background()
	key := "{transactions}:normalization:conditional"

	t.Run("missing cache does not create a balance", func(t *testing.T) {
		require.NoError(t, client.Del(ctx, key).Err())
		status, err := client.Eval(ctx, balanceLimitNormalizationTestLua, []string{key}, "1e+2", "100").Int64()
		require.NoError(t, err)
		require.Zero(t, status)
		exists, err := client.Exists(ctx, key).Result()
		require.NoError(t, err)
		require.Zero(t, exists)
	})

	t.Run("concurrent settings update wins", func(t *testing.T) {
		raw := `{"Available":"120","Version":42,"OverdraftLimit":"250","AllowOverdraft":false}`
		require.NoError(t, client.Set(ctx, key, raw, time.Minute).Err())
		expires, err := client.Do(ctx, "PEXPIRETIME", key).Int64()
		require.NoError(t, err)

		status, err := client.Eval(ctx, balanceLimitNormalizationTestLua, []string{key}, "1e+2", "100").Int64()
		require.NoError(t, err)
		require.EqualValues(t, 2, status)
		stored, err := client.Get(ctx, key).Result()
		require.NoError(t, err)
		require.Equal(t, raw, stored)
		actualExpires, err := client.Do(ctx, "PEXPIRETIME", key).Int64()
		require.NoError(t, err)
		require.Equal(t, expires, actualExpires)
	})
}

func TestIntegration_BalanceLimitNormalization_RejectsInvalidInputWithoutMutation(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	infra := setupRedisIntegrationInfra(t)
	client := infra.redisContainer.Client
	ctx := context.Background()
	key := "{transactions}:normalization:invalid"
	tests := []struct {
		name        string
		raw         string
		replacement string
	}{
		{name: "malformed JSON", raw: `{"OverdraftLimit":`, replacement: "100"},
		{name: "NaN extension", raw: `{"OverdraftLimit":"1e+2","Extension":NaN}`, replacement: "100"},
		{name: "infinity extension", raw: `{"OverdraftLimit":"1e+2","Extension":Infinity}`, replacement: "100"},
		{name: "hexadecimal extension", raw: `{"OverdraftLimit":"1e+2","Extension":0x10}`, replacement: "100"},
		{name: "leading zero extension", raw: `{"OverdraftLimit":"1e+2","Extension":01}`, replacement: "100"},
		{name: "raw control character", raw: "{\"OverdraftLimit\":\"1e+2\",\"Extension\":\"line\nbreak\"}", replacement: "100"},
		{name: "array", raw: `[{"OverdraftLimit":"1e+2"}]`, replacement: "100"},
		{name: "null", raw: `null`, replacement: "100"},
		{name: "scalar", raw: `"1e+2"`, replacement: "100"},
		{name: "missing field", raw: `{"Available":"120"}`, replacement: "100"},
		{name: "number field", raw: `{"OverdraftLimit":100}`, replacement: "100"},
		{name: "null field", raw: `{"OverdraftLimit":null}`, replacement: "100"},
		{name: "boolean field", raw: `{"OverdraftLimit":true}`, replacement: "100"},
		{name: "array field", raw: `{"OverdraftLimit":[]}`, replacement: "100"},
		{name: "object field", raw: `{"OverdraftLimit":{}}`, replacement: "100"},
		{name: "duplicate field", raw: `{"OverdraftLimit":"1e+2","OverdraftLimit":"1e+2"}`, replacement: "100"},
		{name: "escaped duplicate", raw: `{"OverdraftLimit":"1e+2","Overdraft\u004cimit":"1e+2"}`, replacement: "100"},
		{name: "duplicate with different type", raw: `{"OverdraftLimit":false,"OverdraftLimit":"1e+2"}`, replacement: "100"},
	}
	for _, replacement := range []string{"", "+100", "1e2", "01", ".1", "1.", "1.0", "0.10", "-0", "--1", "NaN", "Infinity", " 1", "1 ", "1\n", "1.2.3"} {
		tests = append(tests, struct {
			name        string
			raw         string
			replacement string
		}{name: "replacement " + replacement, raw: `{"OverdraftLimit":"1e+2"}`, replacement: replacement})
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.NoError(t, client.Set(ctx, key, tt.raw, time.Minute).Err())
			expires, err := client.Do(ctx, "PEXPIRETIME", key).Int64()
			require.NoError(t, err)

			err = client.Eval(ctx, balanceLimitNormalizationTestLua, []string{key}, "1e+2", tt.replacement).Err()
			require.EqualError(t, err, "ERR BALANCE_LIMIT_INVALID")
			stored, err := client.Get(ctx, key).Result()
			require.NoError(t, err)
			require.Equal(t, tt.raw, stored)
			actualExpires, err := client.Do(ctx, "PEXPIRETIME", key).Int64()
			require.NoError(t, err)
			require.Equal(t, expires, actualExpires)
		})
	}
}

func TestIntegration_BalanceLimitNormalization_DecimalAndJSONTokenBoundaries(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	infra := setupRedisIntegrationInfra(t)
	client := infra.redisContainer.Client
	ctx := context.Background()
	key := "{transactions}:normalization:tokens"
	for _, canonical := range []string{"0", "100", "0.01", "-100", "-0.01", "999999999999999999999999999999999999.123456789"} {
		t.Run(canonical, func(t *testing.T) {
			raw := ` { "Nested" : [ {"OverdraftLimit":"nested"} ], "Overdraft\u004cimit" : "1\u0065+2", "tail":"brace } bracket ] quote \" slash \\" } `
			require.NoError(t, client.Set(ctx, key, raw, 0).Err())

			status, err := client.Eval(ctx, balanceLimitNormalizationTestLua, []string{key}, "1e+2", canonical).Int64()
			require.NoError(t, err)
			require.EqualValues(t, 1, status)
			stored, err := client.Get(ctx, key).Result()
			require.NoError(t, err)
			require.Equal(t, strings.Replace(raw, `"1\u0065+2"`, `"`+canonical+`"`, 1), stored)
		})
	}
}
