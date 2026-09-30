//go:build integration

// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package engine

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"
	"unicode"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/redis/balancecache"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/domain/accounting"
	redistestutil "github.com/LerianStudio/midaz/v4/tests/utils/redis"
)

// legacyIndexedAliasBlob encodes a balance in the dual cache shape and returns
// its raw fields, so a test can reshape it into what an older release wrote
// without going through the cache codec's read policy.
func legacyIndexedAliasBlob(t *testing.T, balance accounting.BalanceSnapshot) map[string]json.RawMessage {
	t.Helper()

	encoded, err := balancecache.Encode(balance, balancecache.FormatDual)
	require.NoError(t, err)

	var fields map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(encoded, &fields))

	return fields
}

// pureLegacyIndexedAliasBlob keeps only the uppercase fields, as a pre-4.1 writer
// stores them: no lowercase mirror and no SchemaVersion.
func pureLegacyIndexedAliasBlob(t *testing.T, balance accounting.BalanceSnapshot, alias string) map[string]json.RawMessage {
	t.Helper()

	fields := legacyIndexedAliasBlob(t, balance)
	delete(fields, "SchemaVersion")

	for name := range fields {
		if unicode.IsLower(rune(name[0])) {
			delete(fields, name)
		}
	}

	fields["Alias"] = rawJSON(t, alias)

	return fields
}

func rawJSON(t *testing.T, value any) json.RawMessage {
	t.Helper()

	raw, err := json.Marshal(value)
	require.NoError(t, err)

	return raw
}

func storeLegacyIndexedAliasBlob(t *testing.T, f *integrationFixture, fields map[string]json.RawMessage) string {
	t.Helper()

	encoded, err := json.Marshal(fields)
	require.NoError(t, err)

	key := f.resolved.Balances["@source#default"].Balance
	require.NoError(t, f.client.Set(context.Background(), key, encoded, time.Hour).Err())

	return key
}

func storedLegacyIndexedAliasBlob(t *testing.T, f *integrationFixture, key string) map[string]any {
	t.Helper()

	stored, err := f.client.Get(context.Background(), key).Bytes()
	require.NoError(t, err)

	decoder := json.NewDecoder(strings.NewReader(string(stored)))
	decoder.UseNumber()

	var fields map[string]any
	require.NoError(t, decoder.Decode(&fields))

	return fields
}

func TestIntegrationEngineLegacyIndexedAliasApplies(t *testing.T) {
	if testing.Short() {
		t.Skip("requires Valkey")
	}

	container := redistestutil.SetupReusableContainer(t)

	for _, alias := range []string{"0#@source#default", "12#@source#default"} {
		t.Run("pure legacy "+alias, func(t *testing.T) {
			f := newIntegrationFixture(t, container.Client)
			fields := pureLegacyIndexedAliasBlob(t, f.input.Execution.Balances[0], alias)
			fields["Version"] = json.RawMessage(`5`)
			key := storeLegacyIndexedAliasBlob(t, f, fields)

			raw, err := f.run(t)
			require.NoError(t, err)

			result := decodeIntegrationResult(t, raw)
			require.Equal(t, "100", result.Movements[0].Before.Available)
			require.Equal(t, "5", result.Movements[0].Before.Version)
			require.Equal(t, "6", result.Movements[0].After.Version)
			require.Equal(t, "70", result.Final[0].Available)

			stored := storedLegacyIndexedAliasBlob(t, f, key)
			require.Equal(t, "@source", stored["Alias"], "the next engine write heals the uppercase alias")
			require.Equal(t, "@source", stored["alias"])
		})
	}

	t.Run("mixed blob reads the uppercase fields", func(t *testing.T) {
		f := newIntegrationFixture(t, container.Client)
		fields := legacyIndexedAliasBlob(t, f.input.Execution.Balances[0])
		fields["Alias"] = rawJSON(t, "0#@source#default")
		fields["Available"], fields["Version"] = rawJSON(t, "100"), json.RawMessage(`7`)
		fields["available"], fields["version"] = rawJSON(t, "1"), rawJSON(t, "1")
		key := storeLegacyIndexedAliasBlob(t, f, fields)

		raw, err := f.run(t)
		require.NoError(t, err)

		result := decodeIntegrationResult(t, raw)
		require.Equal(t, "100", result.Movements[0].Before.Available)
		require.Equal(t, "7", result.Movements[0].Before.Version)
		require.Equal(t, "8", result.Movements[0].After.Version)
		require.Equal(t, "70", result.Final[0].Available)

		stored := storedLegacyIndexedAliasBlob(t, f, key)
		require.Equal(t, json.Number("8"), stored["Version"])
		require.Equal(t, "8", stored["version"])
		require.Equal(t, "@source", stored["Alias"])
		require.Equal(t, "@source", stored["alias"])
	})
}

func TestIntegrationEngineLegacyIndexedAliasKeepsIdentityGuard(t *testing.T) {
	if testing.Short() {
		t.Skip("requires Valkey")
	}

	container := redistestutil.SetupReusableContainer(t)

	for name, alias := range map[string]string{
		"other alias":      "0#@other#default",
		"other key":        "0#@source#overdraft",
		"extra part":       "0#@source#default#x",
		"non-digit prefix": "a#@source#default",
		"empty prefix":     "#@source#default",
	} {
		t.Run(name, func(t *testing.T) {
			f := newIntegrationFixture(t, container.Client)
			storeLegacyIndexedAliasBlob(t, f, pureLegacyIndexedAliasBlob(t, f.input.Execution.Balances[0], alias))
			before := f.capture(t)

			_, err := f.run(t)
			require.ErrorContains(t, err, `"code":"balance_identity_mismatch"`)
			require.Equal(t, before, f.capture(t), "a rejected identity must not mutate any key")
		})
	}
}

// A pre-4.1 blob can carry both the indexed alias and a noncanonical overdraft
// limit. The engine must accept its identity to reach the limit signal, and the
// Go repair must accept it to rewrite the limit, before the execution applies.
func TestIntegration_AdapterExecute_RepairsLimitOfLegacyIndexedAliasBalance(t *testing.T) {
	if testing.Short() {
		t.Skip("requires Valkey")
	}

	ctx := context.Background()
	inspector, _, _ := newAdapterValkey(t)
	input, limits := richAdapterExecution(t)

	hot := input.Execution.Balances[0]
	hot.Available = decimal.NewFromInt(120)
	hot.OverdraftLimitEnabled = true
	hot.OverdraftLimit = decimal.NewFromInt(1000)
	fields := pureLegacyIndexedAliasBlob(t, hot, "0#"+hot.Alias+"#"+hot.Key)
	fields["OverdraftLimit"] = rawJSON(t, "1000.00")
	encoded, err := json.Marshal(fields)
	require.NoError(t, err)

	keys, err := resolveAdapterKeys(ctx, input.Execution)
	require.NoError(t, err)

	cacheKey := keys.Balances[hot.BalanceRef].Balance
	require.NoError(t, inspector.Set(ctx, cacheKey, encoded, time.Hour).Err())

	adapter, err := newAdapterWithLimits(&integrationClientProvider{client: inspector}, limits)
	require.NoError(t, err)
	result, err := adapter.Execute(ctx, input)
	require.NoError(t, err)
	require.Len(t, result.Final, 1)
	require.True(t, result.Final[0].Available.Equal(decimal.NewFromInt(90)), "execution must use the hot cached balance")
	require.True(t, result.Final[0].OverdraftLimit.Equal(decimal.NewFromInt(1000)))

	persisted, err := inspector.Get(ctx, cacheKey).Bytes()
	require.NoError(t, err)

	var stored map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(persisted, &stored))
	require.JSONEq(t, `"1000"`, string(stored["OverdraftLimit"]))
	require.JSONEq(t, `"1000"`, string(stored["overdraftLimit"]))
	require.JSONEq(t, string(rawJSON(t, hot.Alias)), string(stored["Alias"]))
	require.JSONEq(t, string(rawJSON(t, hot.Alias)), string(stored["alias"]))
}
