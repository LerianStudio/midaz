// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package redis

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// settingsUpdateStubClient is a test double for redis.UniversalClient that
// captures GET and SET calls for UpdateBalanceCacheSettings assertions. It
// supports per-key Get responses (including redis.Nil for cache misses) and
// records every SET so the test can inspect both the key and the marshalled
// BalanceRedis payload written back.
type settingsUpdateStubClient struct {
	redis.UniversalClient

	getResponses map[string]struct {
		val string
		err error
	}
	getCalls []string

	setCalls []recordedSetCall
	setErr   error
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

func (c *settingsUpdateStubClient) Set(ctx context.Context, key string, value any, expiration time.Duration) *redis.StatusCmd {
	c.setCalls = append(c.setCalls, recordedSetCall{Key: key, Value: value, TTL: expiration})

	cmd := redis.NewStatusCmd(ctx)
	if c.setErr != nil {
		cmd.SetErr(c.setErr)

		return cmd
	}

	cmd.SetVal("OK")

	return cmd
}

// TestUpdateBalanceCacheSettings_PreservesLiveTransactionalState is the HARD
// GATE for the settings-only rewrite: it pins the invariant that the repo
// method MUST NOT touch Available, OnHold, Version, or OverdraftUsed — those
// fields are owned by the atomic Lua script and may be ahead of PostgreSQL
// while sync is pending. Deleting the key or overwriting those fields would
// lose in-flight mutations and was the bug motivating this test's existence.
func TestUpdateBalanceCacheSettings_PreservesLiveTransactionalState(t *testing.T) {
	t.Parallel()

	organizationID := uuid.New()
	ledgerID := uuid.New()

	// Seed the cache with a balance that has live transactional state
	// "ahead" of PostgreSQL: higher Version, mutated Available, OverdraftUsed
	// incurred by a previous transaction. Any settings rewrite MUST keep all
	// of these fields intact.
	cached := mmodel.BalanceRedis{
		ID:                    "balance-id",
		Alias:                 "@alice",
		Key:                   "default",
		AccountID:             "account-id",
		AssetCode:             "USD",
		Available:             decimal.NewFromInt(7777),
		OnHold:                decimal.NewFromInt(123),
		Version:               42,
		AccountType:           "liability",
		AllowSending:          1,
		AllowReceiving:        1,
		Direction:             "credit",
		OverdraftUsed:         "250.50",
		AllowOverdraft:        0, // currently disabled — the update will enable it
		OverdraftLimitEnabled: 0,
		OverdraftLimit:        "0",
		BalanceScope:          mmodel.BalanceScopeTransactional,
	}

	cachedJSON, err := json.Marshal(&cached)
	require.NoError(t, err)

	// The repo builds "balance:{transactions}:<org>:<ledger>:@alice#default".
	expectedKey := "balance:{transactions}:" +
		organizationID.String() + ":" + ledgerID.String() + ":@alice#default"

	stub := &settingsUpdateStubClient{
		getResponses: map[string]struct {
			val string
			err error
		}{
			expectedKey: {val: string(cachedJSON)},
		},
	}

	rr := &RedisConsumerRepository{conn: &staticRedisProvider{client: stub}}

	// New settings: enable overdraft with a concrete limit and keep scope
	// transactional. This is the same payload shape the command layer
	// forwards verbatim from UpdateBalance.Settings.
	limit := "1000.00"
	newSettings := &mmodel.BalanceSettings{
		BalanceScope:          mmodel.BalanceScopeTransactional,
		AllowOverdraft:        true,
		OverdraftLimitEnabled: true,
		OverdraftLimit:        &limit,
	}

	err = rr.UpdateBalanceCacheSettings(context.Background(), organizationID, ledgerID, "@alice#default", newSettings)
	require.NoError(t, err)

	// Exactly one GET (read current) and one SET (write back) — no DEL.
	require.Len(t, stub.getCalls, 1, "must read the cache exactly once")
	require.Len(t, stub.setCalls, 1, "must write the cache back exactly once")
	assert.Equal(t, expectedKey, stub.getCalls[0])
	assert.Equal(t, expectedKey, stub.setCalls[0].Key)
	assert.Equal(t, balanceCacheSettingsTTL, stub.setCalls[0].TTL,
		"TTL must match the Lua script's 1-hour canonical value to avoid silent lifetime drift")

	// Decode the written payload and pin every invariant.
	raw, ok := stub.setCalls[0].Value.(string)
	require.True(t, ok, "SET value must be a string (JSON-encoded BalanceRedis)")

	var written mmodel.BalanceRedis

	require.NoError(t, json.Unmarshal([]byte(raw), &written))

	// Settings-derived fields MUST be updated from the new settings payload.
	assert.Equal(t, 1, written.AllowOverdraft,
		"AllowOverdraft must reflect the new settings")
	assert.Equal(t, 1, written.OverdraftLimitEnabled,
		"OverdraftLimitEnabled must reflect the new settings")
	assert.Equal(t, "1000.00", written.OverdraftLimit,
		"OverdraftLimit must be copied from the settings pointer")
	assert.Equal(t, mmodel.BalanceScopeTransactional, written.BalanceScope,
		"BalanceScope must reflect the new settings (or default)")

	// Live transactional state MUST be untouched — this is the whole point
	// of replacing Del with a settings-only rewrite.
	assert.True(t, written.Available.Equal(decimal.NewFromInt(7777)),
		"Available must be preserved verbatim")
	assert.True(t, written.OnHold.Equal(decimal.NewFromInt(123)),
		"OnHold must be preserved verbatim")
	assert.Equal(t, int64(42), written.Version,
		"Version must be preserved (losing it corrupts optimistic concurrency)")
	assert.Equal(t, "250.50", written.OverdraftUsed,
		"OverdraftUsed is transactional state and must not be reset by a settings update")

	// Identity + non-settings fields MUST also survive the rewrite so the
	// next Lua read sees a well-formed BalanceRedis payload.
	assert.Equal(t, "balance-id", written.ID)
	assert.Equal(t, "@alice", written.Alias)
	assert.Equal(t, "default", written.Key)
	assert.Equal(t, "account-id", written.AccountID)
	assert.Equal(t, "USD", written.AssetCode)
	assert.Equal(t, "liability", written.AccountType)
	assert.Equal(t, 1, written.AllowSending)
	assert.Equal(t, 1, written.AllowReceiving)
	assert.Equal(t, "credit", written.Direction)
}

// TestUpdateBalanceCacheSettings_CacheMissIsNoOp verifies that a missing
// Redis key is treated as a silent no-op: the next transaction's SETNX path
// will load the freshly-persisted settings from PostgreSQL, so there is
// nothing for this method to rewrite.
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
	assert.Empty(t, stub.setCalls, "no SET must fire when the key is absent")
}

// TestUpdateBalanceCacheSettings_GetErrorIsPropagated verifies that a Redis
// connectivity failure on the read path bubbles up so the command layer can
// decide whether to swallow (best-effort) or escalate. The repo itself must
// not silently swallow infrastructure errors — that decision belongs to the
// caller.
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

	require.ErrorIs(t, err, boom, "transport errors on GET must propagate unchanged")
	assert.Empty(t, stub.setCalls, "no SET must fire when the GET failed")
}

// TestUpdateBalanceCacheSettings_NilSettingsResetsToDefaults verifies that
// passing nil Settings collapses the cached entry to the Lua-compatible
// zero-state used by buildBalanceAtomicOperationPlan for balances without
// Settings. This keeps the cache and the plan-builder in lock-step on the
// "no settings" interpretation.
func TestUpdateBalanceCacheSettings_NilSettingsResetsToDefaults(t *testing.T) {
	t.Parallel()

	organizationID := uuid.New()
	ledgerID := uuid.New()

	// Start with overdraft enabled so we can observe the reset taking effect.
	cached := mmodel.BalanceRedis{
		ID:                    "balance-id",
		Alias:                 "@alice",
		Key:                   "default",
		Available:             decimal.NewFromInt(100),
		OnHold:                decimal.Zero,
		Version:               1,
		AllowOverdraft:        1,
		OverdraftLimitEnabled: 1,
		OverdraftLimit:        "500.00",
		BalanceScope:          mmodel.BalanceScopeInternal,
		OverdraftUsed:         "42.00",
	}

	cachedJSON, err := json.Marshal(&cached)
	require.NoError(t, err)

	expectedKey := "balance:{transactions}:" +
		organizationID.String() + ":" + ledgerID.String() + ":@alice#default"

	stub := &settingsUpdateStubClient{
		getResponses: map[string]struct {
			val string
			err error
		}{
			expectedKey: {val: string(cachedJSON)},
		},
	}

	rr := &RedisConsumerRepository{conn: &staticRedisProvider{client: stub}}

	err = rr.UpdateBalanceCacheSettings(context.Background(), organizationID, ledgerID, "@alice#default", nil)
	require.NoError(t, err)

	require.Len(t, stub.setCalls, 1)

	raw, ok := stub.setCalls[0].Value.(string)
	require.True(t, ok)

	var written mmodel.BalanceRedis

	require.NoError(t, json.Unmarshal([]byte(raw), &written))

	assert.Equal(t, 0, written.AllowOverdraft)
	assert.Equal(t, 0, written.OverdraftLimitEnabled)
	assert.Equal(t, "0", written.OverdraftLimit)
	assert.Equal(t, mmodel.BalanceScopeTransactional, written.BalanceScope)

	// Transactional state still preserved even when settings are reset.
	assert.Equal(t, "42.00", written.OverdraftUsed,
		"OverdraftUsed is not a settings field and must survive a nil-settings reset")
	assert.Equal(t, int64(1), written.Version)
}
