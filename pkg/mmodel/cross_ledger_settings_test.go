// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package mmodel

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/pkg/constant"
)

func TestCrossLedgerSettings_DefaultParseAndRoundTrip(t *testing.T) {
	t.Parallel()

	defaults := DefaultLedgerSettings()
	assert.False(t, defaults.CrossLedger.Enabled)
	assert.True(t, LedgerSettingsIsDefault(&defaults))

	legacy := ParseLedgerSettings(map[string]any{
		"accounting": map[string]any{"validateRoutes": true},
	})
	assert.True(t, legacy.Accounting.ValidateRoutes)
	assert.False(t, legacy.CrossLedger.Enabled, "missing crossLedger must preserve the safe default")

	enabled := DefaultLedgerSettings()
	enabled.CrossLedger.Enabled = true
	roundTripped := ParseLedgerSettings(LedgerSettingsToMap(enabled))
	assert.Equal(t, enabled, roundTripped)
	assert.False(t, LedgerSettingsIsDefault(&enabled))
}

func TestCrossLedgerSettings_ValidationAndDeepMerge(t *testing.T) {
	t.Parallel()

	err := ValidateSettings(map[string]any{
		"crossLedger": map[string]any{"enabled": "yes"},
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), constant.ErrInvalidSettingsFieldType.Error())

	err = ValidateSettings(map[string]any{
		"crossLedger": map[string]any{"unknown": true},
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), constant.ErrUnknownSettingsField.Error())

	merged := DeepMergeSettings(
		map[string]any{
			"accounting":  map[string]any{"validateRoutes": true},
			"crossLedger": map[string]any{"enabled": false},
		},
		map[string]any{"crossLedger": map[string]any{"enabled": true}},
	)
	assert.Equal(t, map[string]any{
		"accounting":  map[string]any{"validateRoutes": true},
		"crossLedger": map[string]any{"enabled": true},
	}, merged)
}

func TestCrossLedgerSettingsInput_ToSparseMapPreservesExplicitFalse(t *testing.T) {
	t.Parallel()

	assert.Equal(
		t,
		map[string]any{"crossLedger": map[string]any{"enabled": false}},
		(&LedgerSettingsInput{CrossLedger: &CrossLedgerSettingsInput{Enabled: boolPtr(false)}}).ToSparseMap(),
	)
	assert.Equal(t, map[string]any{"crossLedger": map[string]any{}}, (&LedgerSettingsInput{CrossLedger: &CrossLedgerSettingsInput{}}).ToSparseMap())
}

func boolPtr(value bool) *bool {
	return &value
}
