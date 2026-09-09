// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package mmodel

import (
	"encoding/json"
	"testing"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestBalanceRedis_BlockedRoundTrip locks the cache contract for the
// account-block flag: a fresh blob carries Blocked, a legacy blob without the
// field decodes as not blocked (rollout compatibility with live caches), and
// the Lua CamelCase casing decodes into the Go struct.
func TestBalanceRedis_BlockedRoundTrip(t *testing.T) {
	t.Parallel()

	t.Run("blocked field round-trips through JSON", func(t *testing.T) {
		t.Parallel()

		in := BalanceRedis{
			ID:             "b-1",
			AccountID:      "a-1",
			Available:      decimal.NewFromInt(100),
			OnHold:         decimal.Zero,
			Version:        1,
			AllowSending:   1,
			AllowReceiving: 1,
			Blocked:        1,
		}

		raw, err := json.Marshal(in)
		require.NoError(t, err)

		var out BalanceRedis
		require.NoError(t, json.Unmarshal(raw, &out))

		assert.Equal(t, 1, out.Blocked)
	})

	t.Run("legacy blob without the field decodes as not blocked", func(t *testing.T) {
		t.Parallel()

		// Lua CamelCase casing, predating the Blocked field.
		legacy := `{"ID":"b-1","AccountID":"a-1","Available":"250","OnHold":"0",` +
			`"Version":2,"AccountType":"deposit","AllowSending":1,"AllowReceiving":1,` +
			`"AssetCode":"USD","Key":"default"}`

		var out BalanceRedis
		require.NoError(t, json.Unmarshal([]byte(legacy), &out))

		assert.Equal(t, 0, out.Blocked, "absent field must decode as 0 (not blocked)")
	})

	t.Run("lua CamelCase blob carrying Blocked decodes", func(t *testing.T) {
		t.Parallel()

		blob := `{"ID":"b-1","AccountID":"a-1","Available":"250","OnHold":"0",` +
			`"Version":2,"AccountType":"deposit","AllowSending":1,"AllowReceiving":1,` +
			`"AssetCode":"USD","Key":"default","Blocked":1}`

		var out BalanceRedis
		require.NoError(t, json.Unmarshal([]byte(blob), &out))

		assert.Equal(t, 1, out.Blocked)
	})
}

// TestBalance_ToTransactionBalance_CopiesBlocked guards the propagation of
// the account-block flag from the domain balance into the validation model.
func TestBalance_ToTransactionBalance_CopiesBlocked(t *testing.T) {
	t.Parallel()

	b := &Balance{
		ID:        "b-1",
		AccountID: "a-1",
		Alias:     "@alice",
		Key:       "default",
		Blocked:   true,
	}

	txBal, err := b.ToTransactionBalance()
	require.NoError(t, err)

	assert.True(t, txBal.Blocked)
}

// TestBalance_BlockedExcludedFromJSON locks the API-contract decision: the
// blocked flag is cache/flow-internal state and must never surface on balance
// JSON payloads — the account resource is the public surface for it.
func TestBalance_BlockedExcludedFromJSON(t *testing.T) {
	t.Parallel()

	b := &Balance{ID: "b-1", Blocked: true}

	raw, err := json.Marshal(b)
	require.NoError(t, err)

	var doc map[string]any
	require.NoError(t, json.Unmarshal(raw, &doc))

	_, hasLower := doc["blocked"]
	_, hasUpper := doc["Blocked"]
	assert.False(t, hasLower, "balance JSON must not expose a blocked field")
	assert.False(t, hasUpper, "balance JSON must not expose a Blocked field")
}
