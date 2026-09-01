// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package redis

import (
	"context"
	"errors"
	"testing"

	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// settingsUpdateStubClient is a test double for redis.UniversalClient that
// captures GET calls for UpdateBalanceCacheSettings assertions. It supports
// per-key Get responses (including redis.Nil for cache misses). The CAS
// write path (EVAL) is exercised against a real Redis in the integration
// suite (consumer_settings_cas_stress_integration_test.go and the CAS-direct
// tests alongside it) since a stub cannot reproduce Lua semantics; this stub
// only needs to cover the pre-EVAL GET short-circuits (cache miss, transport
// error) that UpdateBalanceCacheSettings resolves before ever building a CAS
// call.
type settingsUpdateStubClient struct {
	redis.UniversalClient

	getResponses map[string]struct {
		val string
		err error
	}
	getCalls []string
}

func (c *settingsUpdateStubClient) Get(ctx context.Context, key string) *redis.StringCmd {
	c.getCalls = append(c.getCalls, key)

	cmd := redis.NewStringCmd(ctx)

	if resp, ok := c.getResponses[key]; ok {
		if resp.err != nil {
			cmd.SetErr(resp.err)

			return cmd
		}

		cmd.SetVal(resp.val)

		return cmd
	}

	// Default to cache miss when no response is pre-configured. This makes
	// the stub safe to use even if a test forgets to seed a key.
	cmd.SetErr(redis.Nil)

	return cmd
}

// TestUpdateBalanceCacheSettings_CacheMissIsNoOp verifies that a missing
// Redis key is treated as a silent no-op: the next transaction's SETNX path
// will load the freshly-persisted settings from PostgreSQL, so there is
// nothing for this method to rewrite. No CAS attempt is made.
func TestUpdateBalanceCacheSettings_CacheMissIsNoOp(t *testing.T) {
	t.Parallel()

	organizationID := uuid.New()
	ledgerID := uuid.New()

	// No pre-configured Get response → the stub returns redis.Nil by default.
	stub := &settingsUpdateStubClient{}

	rr := &RedisConsumerRepository{conn: &staticRedisProvider{client: stub}}

	err := rr.UpdateBalanceCacheSettings(context.Background(), organizationID, ledgerID, "@alice#default",
		&mmodel.BalanceSettings{AllowOverdraft: true})

	require.NoError(t, err, "cache miss must be a silent no-op")
	require.Len(t, stub.getCalls, 1, "the single GET that discovered the miss is expected")
}

// TestUpdateBalanceCacheSettings_GetErrorIsPropagated verifies that a Redis
// connectivity failure on the read path bubbles up immediately (not retried)
// so the command layer can decide whether to swallow (best-effort) or
// escalate. Only a CAS conflict (-1) triggers a retry; a transport error is
// a technical failure the repo does not swallow internally.
func TestUpdateBalanceCacheSettings_GetErrorIsPropagated(t *testing.T) {
	t.Parallel()

	organizationID := uuid.New()
	ledgerID := uuid.New()

	expectedKey := "balance:{transactions}:" +
		organizationID.String() + ":" + ledgerID.String() + ":@alice#default"

	boom := errors.New("redis connection reset")

	stub := &settingsUpdateStubClient{
		getResponses: map[string]struct {
			val string
			err error
		}{
			expectedKey: {err: boom},
		},
	}

	rr := &RedisConsumerRepository{conn: &staticRedisProvider{client: stub}}

	err := rr.UpdateBalanceCacheSettings(context.Background(), organizationID, ledgerID, "@alice#default",
		&mmodel.BalanceSettings{AllowOverdraft: true})

	require.ErrorIs(t, err, boom, "transport errors on GET must propagate unchanged, not be retried")
	assert.Len(t, stub.getCalls, 1, "a transport error must not trigger a retry")
}

// TestApplySettingsToCachedBalance_FullSettings pins the happy path: every
// settings-derived field is overwritten from a fully-populated
// BalanceSettings, and live transactional state is left untouched.
func TestApplySettingsToCachedBalance_FullSettings(t *testing.T) {
	t.Parallel()

	limit := "1000.00"
	cached := map[string]any{
		"ID":            "balance-id",
		"Alias":         "@alice",
		"Available":     "7777",
		"OnHold":        "123",
		"Version":       float64(42),
		"OverdraftUsed": "250.50",
	}

	applySettingsToCachedBalance(cached, &mmodel.BalanceSettings{
		BalanceScope:          mmodel.BalanceScopeTransactional,
		AllowOverdraft:        true,
		OverdraftLimitEnabled: true,
		OverdraftLimit:        &limit,
	})

	assert.Equal(t, 1, cached["AllowOverdraft"])
	assert.Equal(t, 1, cached["OverdraftLimitEnabled"])
	assert.Equal(t, "1000.00", cached["OverdraftLimit"])
	assert.Equal(t, mmodel.BalanceScopeTransactional, cached["BalanceScope"])

	// Live transactional state and identity fields are untouched.
	assert.Equal(t, "balance-id", cached["ID"])
	assert.Equal(t, "@alice", cached["Alias"])
	assert.Equal(t, "7777", cached["Available"])
	assert.Equal(t, "123", cached["OnHold"])
	assert.Equal(t, float64(42), cached["Version"])
	assert.Equal(t, "250.50", cached["OverdraftUsed"])
}

// TestApplySettingsToCachedBalance_PartialSettings covers the two documented
// partial-payload cases: OverdraftLimit == nil collapses to the Lua-compatible
// "0" placeholder, and an empty BalanceScope defaults to transactional.
func TestApplySettingsToCachedBalance_PartialSettings(t *testing.T) {
	t.Parallel()

	cached := map[string]any{"Available": "100"}

	applySettingsToCachedBalance(cached, &mmodel.BalanceSettings{
		AllowOverdraft:        true,
		OverdraftLimitEnabled: false,
		OverdraftLimit:        nil,
		BalanceScope:          "",
	})

	assert.Equal(t, 1, cached["AllowOverdraft"])
	assert.Equal(t, 0, cached["OverdraftLimitEnabled"])
	assert.Equal(t, "0", cached["OverdraftLimit"],
		"a nil OverdraftLimit must collapse to the Lua-compatible placeholder")
	assert.Equal(t, mmodel.BalanceScopeTransactional, cached["BalanceScope"],
		"an empty BalanceScope must default to transactional")
}

// TestApplySettingsToCachedBalance_NilSettingsResetsToDefaults verifies that
// passing nil Settings collapses the cached entry to the Lua-compatible
// zero-state used by buildBalanceAtomicOperationPlan for balances without
// Settings, while leaving non-settings fields untouched.
func TestApplySettingsToCachedBalance_NilSettingsResetsToDefaults(t *testing.T) {
	t.Parallel()

	cached := map[string]any{
		"AllowOverdraft":        1,
		"OverdraftLimitEnabled": 1,
		"OverdraftLimit":        "500.00",
		"BalanceScope":          mmodel.BalanceScopeInternal,
		"OverdraftUsed":         "42.00",
		"Version":               float64(1),
	}

	applySettingsToCachedBalance(cached, nil)

	assert.Equal(t, 0, cached["AllowOverdraft"])
	assert.Equal(t, 0, cached["OverdraftLimitEnabled"])
	assert.Equal(t, "0", cached["OverdraftLimit"])
	assert.Equal(t, mmodel.BalanceScopeTransactional, cached["BalanceScope"])

	// Non-settings fields are not part of the reset.
	assert.Equal(t, "42.00", cached["OverdraftUsed"])
	assert.Equal(t, float64(1), cached["Version"])
}

// TestApplySettingsToCachedBalance_DedupesLegacyCasing covers the cleanup of
// legacy camelCase keys left behind by pre-fix writers: applying a settings
// mutation over a document carrying both the legacy camelCase and Lua-native
// CamelCase spellings must converge to exactly one casing per field, so Lua
// never sees a stale duplicate alongside the fresh CamelCase write.
func TestApplySettingsToCachedBalance_DedupesLegacyCasing(t *testing.T) {
	t.Parallel()

	limit := "500.00"
	cached := map[string]any{
		"ID":        "balance-id",
		"Direction": "credit",
		// Legacy camelCase keys from a pre-fix Go writer that must be dropped.
		"allowOverdraft":        0,
		"overdraftLimitEnabled": 0,
		"overdraftLimit":        "0",
		"balanceScope":          "transactional",
	}

	applySettingsToCachedBalance(cached, &mmodel.BalanceSettings{
		BalanceScope:          mmodel.BalanceScopeTransactional,
		AllowOverdraft:        true,
		OverdraftLimitEnabled: true,
		OverdraftLimit:        &limit,
	})

	// Lua-native CamelCase keys must be present with the new values.
	assert.Equal(t, 1, cached["AllowOverdraft"])
	assert.Equal(t, 1, cached["OverdraftLimitEnabled"])
	assert.Equal(t, "500.00", cached["OverdraftLimit"])
	assert.Equal(t, mmodel.BalanceScopeTransactional, cached["BalanceScope"])

	// Legacy camelCase keys must be purged.
	for _, legacyKey := range []string{"allowOverdraft", "overdraftLimitEnabled", "overdraftLimit", "balanceScope"} {
		_, present := cached[legacyKey]
		assert.False(t, present, "legacy camelCase key %q must be removed from the cache document", legacyKey)
	}

	// Untouched identity fields survive.
	assert.Equal(t, "balance-id", cached["ID"])
	assert.Equal(t, "credit", cached["Direction"])
}
