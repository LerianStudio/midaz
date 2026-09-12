// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package redis

import (
	"testing"

	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestResolveBalanceSettingsArgs_FullSettings pins the happy path: every
// settings-derived ARGV value is resolved from a fully-populated
// BalanceSettings.
func TestResolveBalanceSettingsArgs_FullSettings(t *testing.T) {
	t.Parallel()

	limit := "1000.00"

	allowOverdraft, overdraftLimitEnabled, overdraftLimit, balanceScope, err := resolveBalanceSettingsArgs(&mmodel.BalanceSettings{
		BalanceScope:          mmodel.BalanceScopeInternal,
		AllowOverdraft:        true,
		OverdraftLimitEnabled: true,
		OverdraftLimit:        &limit,
	})
	require.NoError(t, err)

	assert.Equal(t, 1, allowOverdraft)
	assert.Equal(t, 1, overdraftLimitEnabled)
	assert.Equal(t, "1000", overdraftLimit)
	assert.Equal(t, mmodel.BalanceScopeInternal, balanceScope)
}

// TestResolveBalanceSettingsArgs_PartialSettings covers the two documented
// partial-payload cases: OverdraftLimit == nil collapses to the Lua-compatible
// "0" placeholder, and an empty BalanceScope defaults to transactional.
func TestResolveBalanceSettingsArgs_PartialSettings(t *testing.T) {
	t.Parallel()

	allowOverdraft, overdraftLimitEnabled, overdraftLimit, balanceScope, err := resolveBalanceSettingsArgs(&mmodel.BalanceSettings{
		AllowOverdraft:        true,
		OverdraftLimitEnabled: false,
		OverdraftLimit:        nil,
		BalanceScope:          "",
	})
	require.NoError(t, err)

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

	allowOverdraft, overdraftLimitEnabled, overdraftLimit, balanceScope, err := resolveBalanceSettingsArgs(nil)
	require.NoError(t, err)

	assert.Equal(t, 0, allowOverdraft)
	assert.Equal(t, 0, overdraftLimitEnabled)
	assert.Equal(t, "0", overdraftLimit)
	assert.Equal(t, mmodel.BalanceScopeTransactional, balanceScope)
}

func TestResolveBalanceSettingsArgs_CanonicalLimit(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		name  string
		input string
		want  string
	}{
		{name: "scientific", input: "1e3", want: "1000"},
		{name: "fractional scientific", input: "1.2500e-2", want: "0.0125"},
		{name: "canonical", input: "12.5", want: "12.5"},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			limit := test.input
			_, _, got, _, err := resolveBalanceSettingsArgs(&mmodel.BalanceSettings{
				AllowOverdraft:        true,
				OverdraftLimitEnabled: true,
				OverdraftLimit:        &limit,
			})
			require.NoError(t, err)
			assert.Equal(t, test.want, got)
			assert.Equal(t, test.input, limit, "serialization must not mutate the caller's settings")
		})
	}
}

func TestResolveBalanceSettingsArgs_InvalidLimit(t *testing.T) {
	t.Parallel()

	for _, input := range []string{"", "not-a-number", "1e"} {
		t.Run(input, func(t *testing.T) {
			limit := input
			_, _, _, _, err := resolveBalanceSettingsArgs(&mmodel.BalanceSettings{
				AllowOverdraft:        true,
				OverdraftLimitEnabled: true,
				OverdraftLimit:        &limit,
			})
			require.Error(t, err)
		})
	}
}
