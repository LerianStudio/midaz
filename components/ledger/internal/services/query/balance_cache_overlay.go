// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package query

import (
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/shopspring/decimal"
)

// applyBalanceCacheOverlay overlays the Redis cache state onto a
// PostgreSQL-sourced balance. The cache is authoritative for runtime state
// because the Lua atomic script mutates Redis synchronously while the
// write-behind worker syncs PostgreSQL asynchronously, so a balance read
// immediately after a transaction commit may carry stale PG values until
// the worker catches up.
//
// Three field families are propagated:
//
//  1. Mutable runtime state: Available, OnHold, Version. Always overlaid
//     when the cache entry is present.
//  2. Direction. Always overlaid from a non-empty cache value. Empty cache
//     values fall back to PG, covering legacy entries that predate the
//     direction-aware balance model.
//  3. OverdraftUsed and Settings. The cache is the source of truth post-
//     transaction. OverdraftUsed always overlays; an unparseable value
//     defaults to zero, mirroring the Lua-side fallback. Settings are
//     synthesised from the cache only when the cache carries divergent
//     values; otherwise the PG-sourced Settings are preserved to avoid
//     stomping legitimate configuration with cache defaults.
//
// This helper centralises the overlay logic shared by GetAllBalancesByAccountID,
// GetAllBalances, and GetAllBalancesByAlias. Any future overlay caller MUST
// route through this function so the cache-to-response contract stays uniform.
func applyBalanceCacheOverlay(b *mmodel.Balance, c *mmodel.BalanceRedis) {
	if b == nil || c == nil {
		return
	}

	b.Available = c.Available
	b.OnHold = c.OnHold
	b.Version = c.Version

	if c.Direction != "" {
		b.Direction = c.Direction
	}

	// OverdraftUsed is stored as a decimal string in Redis. An unparseable
	// value defaults to zero rather than corrupting the domain model with
	// an arbitrary number, matching the Lua-side fallback applied by
	// getBalancesFromCache.
	overdraftUsed, err := decimal.NewFromString(c.OverdraftUsed)
	if err != nil {
		overdraftUsed = decimal.Zero
	}

	b.OverdraftUsed = overdraftUsed

	// Synthesise Settings from the cache only when at least one field
	// diverges from the defaults. This guards against overwriting legitimate
	// PG settings with empty defaults from legacy cache entries that predate
	// the overdraft feature. The divergence check mirrors the materialisation
	// guard in getBalancesFromCache so transaction flows and balance-query
	// flows observe the same overdraft configuration regardless of the read
	// path.
	if cacheHasDivergentSettings(c) {
		b.Settings = synthesiseSettingsFromCache(c)
	}
}

// cacheHasDivergentSettings reports whether the cache entry carries any
// non-default settings value worth overlaying onto the PG-sourced Balance.
// Empty / zero cache values are treated as "no information" rather than
// "settings disabled" — the PG row is the source of truth in that case.
func cacheHasDivergentSettings(c *mmodel.BalanceRedis) bool {
	if c.AllowOverdraft != 0 || c.OverdraftLimitEnabled != 0 {
		return true
	}

	if c.BalanceScope != "" && c.BalanceScope != mmodel.BalanceScopeTransactional {
		return true
	}

	if c.OverdraftLimit != "" && c.OverdraftLimit != "0" {
		return true
	}

	return false
}

// synthesiseSettingsFromCache constructs a BalanceSettings value from the
// fields carried on a Redis cache entry. The caller must have verified
// divergence via cacheHasDivergentSettings before invoking this function;
// otherwise the resulting Settings would carry pure defaults and clobber
// legitimate PG data.
func synthesiseSettingsFromCache(c *mmodel.BalanceRedis) *mmodel.BalanceSettings {
	s := &mmodel.BalanceSettings{
		BalanceScope:          c.BalanceScope,
		AllowOverdraft:        c.AllowOverdraft == 1,
		OverdraftLimitEnabled: c.OverdraftLimitEnabled == 1,
	}

	// Only expose OverdraftLimit when the limit is actively enforced.
	// BalanceSettings.Validate() requires OverdraftLimit to be nil whenever
	// OverdraftLimitEnabled is false.
	if c.OverdraftLimitEnabled == 1 && c.OverdraftLimit != "" {
		limit := c.OverdraftLimit
		s.OverdraftLimit = &limit
	}

	return s
}
