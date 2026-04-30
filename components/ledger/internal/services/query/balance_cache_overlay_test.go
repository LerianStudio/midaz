// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package query

import (
	"testing"

	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestApplyBalanceCacheOverlay_PropagatesAvailableOnHoldVersion validates the
// baseline overlay (Available, OnHold, Version).
func TestApplyBalanceCacheOverlay_PropagatesAvailableOnHoldVersion(t *testing.T) {
	t.Parallel()

	b := &mmodel.Balance{
		Available: decimal.NewFromInt(10),
		OnHold:    decimal.NewFromInt(2),
		Version:   1,
	}
	c := &mmodel.BalanceRedis{
		Available: decimal.NewFromInt(500),
		OnHold:    decimal.NewFromInt(50),
		Version:   42,
	}

	applyBalanceCacheOverlay(b, c)

	assert.True(t, b.Available.Equal(decimal.NewFromInt(500)))
	assert.True(t, b.OnHold.Equal(decimal.NewFromInt(50)))
	assert.Equal(t, int64(42), b.Version)
}

// TestApplyBalanceCacheOverlay_PropagatesOverdraftUsed verifies the cache value
// for OverdraftUsed wins over the (potentially stale) PG value.
func TestApplyBalanceCacheOverlay_PropagatesOverdraftUsed(t *testing.T) {
	t.Parallel()

	b := &mmodel.Balance{
		OverdraftUsed: decimal.Zero, // PG says "no overdraft used"
	}
	c := &mmodel.BalanceRedis{
		OverdraftUsed: "50.00", // cache says "50 in overdraft"
	}

	applyBalanceCacheOverlay(b, c)

	assert.True(t, b.OverdraftUsed.Equal(decimal.NewFromInt(50)),
		"cache OverdraftUsed must overlay stale PG value")
}

// TestApplyBalanceCacheOverlay_OverdraftUsed_UnparseableFallsBackToZero
// matches the Lua/getBalancesFromCache fallback contract.
func TestApplyBalanceCacheOverlay_OverdraftUsed_UnparseableFallsBackToZero(t *testing.T) {
	t.Parallel()

	b := &mmodel.Balance{
		OverdraftUsed: decimal.NewFromInt(99), // PG value to be replaced
	}
	c := &mmodel.BalanceRedis{
		OverdraftUsed: "not-a-number",
	}

	applyBalanceCacheOverlay(b, c)

	assert.True(t, b.OverdraftUsed.Equal(decimal.Zero),
		"unparseable cache OverdraftUsed must fall back to zero, matching Lua")
}

// TestApplyBalanceCacheOverlay_OverdraftUsed_EmptyStringFallsBackToZero
// confirms legacy cache entries that predate the overdraft fields are
// handled cleanly.
func TestApplyBalanceCacheOverlay_OverdraftUsed_EmptyStringFallsBackToZero(t *testing.T) {
	t.Parallel()

	b := &mmodel.Balance{
		OverdraftUsed: decimal.NewFromInt(7),
	}
	c := &mmodel.BalanceRedis{
		OverdraftUsed: "",
	}

	applyBalanceCacheOverlay(b, c)

	assert.True(t, b.OverdraftUsed.Equal(decimal.Zero))
}

// TestApplyBalanceCacheOverlay_PropagatesDirectionWhenNonEmpty checks the
// direction-overlay rule: cache wins when populated.
func TestApplyBalanceCacheOverlay_PropagatesDirectionWhenNonEmpty(t *testing.T) {
	t.Parallel()

	b := &mmodel.Balance{Direction: ""}
	c := &mmodel.BalanceRedis{Direction: "debit"}

	applyBalanceCacheOverlay(b, c)

	assert.Equal(t, "debit", b.Direction)
}

// TestApplyBalanceCacheOverlay_PreservesDirectionWhenCacheEmpty covers legacy
// cache entries that predate the direction-aware balance model.
func TestApplyBalanceCacheOverlay_PreservesDirectionWhenCacheEmpty(t *testing.T) {
	t.Parallel()

	b := &mmodel.Balance{Direction: "credit"}
	c := &mmodel.BalanceRedis{Direction: ""}

	applyBalanceCacheOverlay(b, c)

	assert.Equal(t, "credit", b.Direction,
		"empty cache Direction must not stomp PG value")
}

// TestApplyBalanceCacheOverlay_PropagatesSettings_AllowOverdraftEnabled
// covers the most common divergence: overdraft is enabled and the cache
// carries the active settings.
func TestApplyBalanceCacheOverlay_PropagatesSettings_AllowOverdraftEnabled(t *testing.T) {
	t.Parallel()

	b := &mmodel.Balance{Settings: nil}
	c := &mmodel.BalanceRedis{
		AllowOverdraft:        1,
		OverdraftLimitEnabled: 1,
		OverdraftLimit:        "500",
		BalanceScope:          mmodel.BalanceScopeTransactional,
	}

	applyBalanceCacheOverlay(b, c)

	require.NotNil(t, b.Settings)
	assert.True(t, b.Settings.AllowOverdraft)
	assert.True(t, b.Settings.OverdraftLimitEnabled)
	require.NotNil(t, b.Settings.OverdraftLimit)
	assert.Equal(t, "500", *b.Settings.OverdraftLimit)
	assert.Equal(t, mmodel.BalanceScopeTransactional, b.Settings.BalanceScope)
}

// TestApplyBalanceCacheOverlay_PropagatesSettings_InternalScope confirms the
// internal-scope marker on the auto-created overdraft companion is overlaid.
func TestApplyBalanceCacheOverlay_PropagatesSettings_InternalScope(t *testing.T) {
	t.Parallel()

	b := &mmodel.Balance{Settings: nil}
	c := &mmodel.BalanceRedis{
		BalanceScope: mmodel.BalanceScopeInternal,
	}

	applyBalanceCacheOverlay(b, c)

	require.NotNil(t, b.Settings)
	assert.Equal(t, mmodel.BalanceScopeInternal, b.Settings.BalanceScope)
	assert.False(t, b.Settings.AllowOverdraft)
}

// TestApplyBalanceCacheOverlay_PropagatesSettings_UnlimitedOverdraft asserts
// that an unlimited-overdraft cache entry (limit disabled) carries no
// OverdraftLimit pointer through to the synthesised Settings, matching the
// validation contract that requires nil OverdraftLimit when the flag is off.
func TestApplyBalanceCacheOverlay_PropagatesSettings_UnlimitedOverdraft(t *testing.T) {
	t.Parallel()

	b := &mmodel.Balance{Settings: nil}
	c := &mmodel.BalanceRedis{
		AllowOverdraft:        1,
		OverdraftLimitEnabled: 0,
		OverdraftLimit:        "", // no concrete limit
	}

	applyBalanceCacheOverlay(b, c)

	require.NotNil(t, b.Settings)
	assert.True(t, b.Settings.AllowOverdraft)
	assert.False(t, b.Settings.OverdraftLimitEnabled)
	assert.Nil(t, b.Settings.OverdraftLimit,
		"OverdraftLimit must remain nil when OverdraftLimitEnabled is false")
}

// TestApplyBalanceCacheOverlay_NilSafety_PreservesPGSettingsWhenCacheIsDefault
// covers the nil-safety contract: a cache entry with no divergent settings
// must NOT clobber a populated PG Settings value.
func TestApplyBalanceCacheOverlay_NilSafety_PreservesPGSettingsWhenCacheIsDefault(t *testing.T) {
	t.Parallel()

	limit := "1000"

	pgSettings := &mmodel.BalanceSettings{
		BalanceScope:          mmodel.BalanceScopeTransactional,
		AllowOverdraft:        true,
		OverdraftLimitEnabled: true,
		OverdraftLimit:        &limit,
	}

	b := &mmodel.Balance{Settings: pgSettings}
	c := &mmodel.BalanceRedis{
		// Pure defaults — cache carries no divergent settings.
		AllowOverdraft:        0,
		OverdraftLimitEnabled: 0,
		OverdraftLimit:        "",
		BalanceScope:          "",
	}

	applyBalanceCacheOverlay(b, c)

	require.NotNil(t, b.Settings, "PG Settings must not be nilled out by an empty cache entry")
	assert.True(t, b.Settings.AllowOverdraft)
	assert.True(t, b.Settings.OverdraftLimitEnabled)
	require.NotNil(t, b.Settings.OverdraftLimit)
	assert.Equal(t, "1000", *b.Settings.OverdraftLimit)
}

// TestApplyBalanceCacheOverlay_NilSafety_TransactionalScopeAlone covers the
// borderline case: a cache entry that only carries BalanceScope="transactional"
// (the default) is treated as "no divergence" so PG wins.
func TestApplyBalanceCacheOverlay_NilSafety_TransactionalScopeAlone(t *testing.T) {
	t.Parallel()

	pgSettings := &mmodel.BalanceSettings{
		AllowOverdraft: true,
		BalanceScope:   mmodel.BalanceScopeTransactional,
	}

	b := &mmodel.Balance{Settings: pgSettings}
	c := &mmodel.BalanceRedis{
		BalanceScope: mmodel.BalanceScopeTransactional,
	}

	applyBalanceCacheOverlay(b, c)

	require.NotNil(t, b.Settings)
	assert.True(t, b.Settings.AllowOverdraft,
		"transactional-scope cache entry must not stomp PG AllowOverdraft=true")
}

// TestApplyBalanceCacheOverlay_NilSafety_OverdraftLimitZero matches the
// reference helper getBalancesFromCache: "0" is treated as no information.
func TestApplyBalanceCacheOverlay_NilSafety_OverdraftLimitZero(t *testing.T) {
	t.Parallel()

	pgLimit := "750"
	pgSettings := &mmodel.BalanceSettings{
		AllowOverdraft:        true,
		OverdraftLimitEnabled: true,
		OverdraftLimit:        &pgLimit,
	}

	b := &mmodel.Balance{Settings: pgSettings}
	c := &mmodel.BalanceRedis{
		OverdraftLimit: "0", // sentinel for "no limit info" in cache
	}

	applyBalanceCacheOverlay(b, c)

	require.NotNil(t, b.Settings.OverdraftLimit)
	assert.Equal(t, "750", *b.Settings.OverdraftLimit)
}

// TestApplyBalanceCacheOverlay_NilGuards confirms the helper is safe to call
// with nil arguments.
func TestApplyBalanceCacheOverlay_NilGuards(t *testing.T) {
	t.Parallel()

	t.Run("nil balance is no-op", func(t *testing.T) {
		t.Parallel()
		assert.NotPanics(t, func() {
			applyBalanceCacheOverlay(nil, &mmodel.BalanceRedis{})
		})
	})

	t.Run("nil cache is no-op", func(t *testing.T) {
		t.Parallel()
		b := &mmodel.Balance{Available: decimal.NewFromInt(10)}
		applyBalanceCacheOverlay(b, nil)
		assert.True(t, b.Available.Equal(decimal.NewFromInt(10)))
	})
}

// TestApplyBalanceCacheOverlay_FullOverdraftEntry_EndToEnd asserts that a
// fully-populated cache entry (Lua-CamelCase shape, all fields present) round-
// trips every field onto the target Balance, with the Settings struct exactly
// reflecting the divergent values.
func TestApplyBalanceCacheOverlay_FullOverdraftEntry_EndToEnd(t *testing.T) {
	t.Parallel()

	b := &mmodel.Balance{
		Available:     decimal.NewFromInt(0),
		OnHold:        decimal.NewFromInt(0),
		Version:       1,
		Direction:     "",
		OverdraftUsed: decimal.Zero,
		Settings:      nil,
	}
	c := &mmodel.BalanceRedis{
		Available:             decimal.NewFromInt(0),
		OnHold:                decimal.NewFromInt(0),
		Version:               7,
		Direction:             "credit",
		OverdraftUsed:         "130",
		AllowOverdraft:        1,
		OverdraftLimitEnabled: 1,
		OverdraftLimit:        "500",
		BalanceScope:          mmodel.BalanceScopeTransactional,
	}

	applyBalanceCacheOverlay(b, c)

	assert.Equal(t, int64(7), b.Version)
	assert.Equal(t, "credit", b.Direction)
	assert.True(t, b.OverdraftUsed.Equal(decimal.NewFromInt(130)))
	require.NotNil(t, b.Settings)
	assert.True(t, b.Settings.AllowOverdraft)
	assert.True(t, b.Settings.OverdraftLimitEnabled)
	require.NotNil(t, b.Settings.OverdraftLimit)
	assert.Equal(t, "500", *b.Settings.OverdraftLimit)
}
