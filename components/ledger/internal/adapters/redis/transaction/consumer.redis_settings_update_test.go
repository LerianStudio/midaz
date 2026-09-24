// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package redis

import (
	"testing"

	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	"github.com/stretchr/testify/assert"
)

// TestResolveBalanceSettingsArgs_FullSettings pins the happy path: every
// settings-derived ARGV value is resolved from a fully-populated
// BalanceSettings.
func TestResolveBalanceSettingsArgs_FullSettings(t *testing.T) {
	t.Parallel()

	limit := "1000.00"

	allowOverdraft, overdraftLimitEnabled, overdraftLimit, balanceScope := resolveBalanceSettingsArgs(&mmodel.BalanceSettings{
		BalanceScope:          mmodel.BalanceScopeInternal,
		AllowOverdraft:        true,
		OverdraftLimitEnabled: true,
		OverdraftLimit:        &limit,
	})

	assert.Equal(t, 1, allowOverdraft)
	assert.Equal(t, 1, overdraftLimitEnabled)
	assert.Equal(t, "1000.00", overdraftLimit)
	assert.Equal(t, mmodel.BalanceScopeInternal, balanceScope)
}

// TestResolveBalanceSettingsArgs_PartialSettings covers the two documented
// partial-payload cases: OverdraftLimit == nil collapses to the Lua-compatible
// "0" placeholder, and an empty BalanceScope defaults to transactional.
func TestResolveBalanceSettingsArgs_PartialSettings(t *testing.T) {
	t.Parallel()

	allowOverdraft, overdraftLimitEnabled, overdraftLimit, balanceScope := resolveBalanceSettingsArgs(&mmodel.BalanceSettings{
		AllowOverdraft:        true,
		OverdraftLimitEnabled: false,
		OverdraftLimit:        nil,
		BalanceScope:          "",
	})

	assert.Equal(t, 1, allowOverdraft)
	assert.Equal(t, 0, overdraftLimitEnabled)
	assert.Equal(t, "0", overdraftLimit,
		"a nil OverdraftLimit must collapse to the Lua-compatible placeholder")
	assert.Equal(t, mmodel.BalanceScopeTransactional, balanceScope,
		"an empty BalanceScope must default to transactional")
}

// TestResolveBalanceSettingsArgs_NilSettingsResetsToDefaults verifies that a
// nil settings payload resolves to the same zero-state
// buildBalanceAtomicOperationPlan uses for balances without Settings.
func TestResolveBalanceSettingsArgs_NilSettingsResetsToDefaults(t *testing.T) {
	t.Parallel()

	allowOverdraft, overdraftLimitEnabled, overdraftLimit, balanceScope := resolveBalanceSettingsArgs(nil)

	assert.Equal(t, 0, allowOverdraft)
	assert.Equal(t, 0, overdraftLimitEnabled)
	assert.Equal(t, "0", overdraftLimit)
	assert.Equal(t, mmodel.BalanceScopeTransactional, balanceScope)
}
