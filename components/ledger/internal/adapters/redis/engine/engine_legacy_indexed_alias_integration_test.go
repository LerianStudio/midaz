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

	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/redis/balancecache"
	redistestutil "github.com/LerianStudio/midaz/v4/tests/utils/redis"
)

// legacyIndexedAliasBlob encodes the fixture's primary balance in the dual cache
// shape and returns its raw fields, so a test can reshape it into what an older
// release wrote without going through the cache codec's read policy.
func legacyIndexedAliasBlob(t *testing.T, f *integrationFixture) map[string]json.RawMessage {
	t.Helper()

	encoded, err := balancecache.Encode(f.input.Execution.Balances[0], balancecache.FormatDual)
	require.NoError(t, err)

	var fields map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(encoded, &fields))

	return fields
}

// pureLegacyIndexedAliasBlob keeps only the uppercase fields, as a pre-4.1 writer
// stores them: no lowercase mirror and no SchemaVersion.
func pureLegacyIndexedAliasBlob(t *testing.T, f *integrationFixture, alias string) map[string]json.RawMessage {
	t.Helper()

	fields := legacyIndexedAliasBlob(t, f)
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
			fields := pureLegacyIndexedAliasBlob(t, f, alias)
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
		fields := legacyIndexedAliasBlob(t, f)
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
			storeLegacyIndexedAliasBlob(t, f, pureLegacyIndexedAliasBlob(t, f, alias))
			before := f.capture(t)

			_, err := f.run(t)
			require.ErrorContains(t, err, `"code":"balance_identity_mismatch"`)
			require.Equal(t, before, f.capture(t), "a rejected identity must not mutate any key")
		})
	}
}
