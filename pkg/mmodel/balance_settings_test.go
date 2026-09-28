// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package mmodel

import (
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestBalanceSettings_Defaults verifies the default BalanceSettings produced by
// NewDefaultBalanceSettings returns safe defaults:
//   - BalanceScope = "transactional"
//   - AllowOverdraft = false
//   - OverdraftLimitEnabled = false
//   - OverdraftLimit = nil
func TestBalanceSettings_Defaults(t *testing.T) {
	t.Parallel()

	got := NewDefaultBalanceSettings()

	require.NotNil(t, got, "NewDefaultBalanceSettings must return non-nil value")
	assert.Equal(t, "transactional", got.BalanceScope, "default BalanceScope must be 'transactional'")
	assert.False(t, got.AllowOverdraft, "default AllowOverdraft must be false")
	assert.False(t, got.OverdraftLimitEnabled, "default OverdraftLimitEnabled must be false")
	assert.Nil(t, got.OverdraftLimit, "default OverdraftLimit must be nil")
}

func TestBalanceSettings_Normalize(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input string
		want  string
	}{
		{name: "scientific notation", input: "1E+3", want: "1000"},
		{name: "explicit positive sign", input: "+1000", want: "1000"},
		{name: "trailing zeros", input: "1000.00", want: "1000"},
		{name: "canonical fraction", input: "0.5", want: "0.5"},
		{name: "leading zeros", input: "001000.00", want: "1000"},
		{name: "leading decimal point", input: ".5", want: "0.5"},
		{name: "trailing decimal point", input: "1000.", want: "1000"},
		{name: "small exponent", input: "5e-8", want: "0.00000005"},
		{name: "large precise amount", input: "12345678901234567890.1234567890", want: "12345678901234567890.123456789"},
		{name: "invalid preserved", input: "not-a-number", want: "not-a-number"},
		{name: "empty preserved", input: "", want: ""},
		{name: "whitespace preserved", input: " 1000 ", want: " 1000 "},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			limit := tt.input
			settings := &BalanceSettings{
				BalanceScope:          BalanceScopeInternal,
				AllowOverdraft:        true,
				OverdraftLimitEnabled: true,
				OverdraftLimit:        &limit,
			}

			settings.Normalize()

			require.NotNil(t, settings.OverdraftLimit)
			assert.Equal(t, tt.want, *settings.OverdraftLimit)
			assert.Equal(t, tt.input, limit, "normalization must not mutate a shared string")
			assert.Equal(t, BalanceScopeInternal, settings.BalanceScope)
			assert.True(t, settings.AllowOverdraft)
			assert.True(t, settings.OverdraftLimitEnabled)

			settings.Normalize()
			assert.Equal(t, tt.want, *settings.OverdraftLimit, "normalization must be idempotent")
		})
	}
}

func TestBalanceSettings_Normalize_Nil(t *testing.T) {
	t.Parallel()

	var settings *BalanceSettings
	settings.Normalize()
	assert.Nil(t, settings)

	settings = NewDefaultBalanceSettings()
	settings.Normalize()
	assert.Equal(t, NewDefaultBalanceSettings(), settings)
}

func TestBalanceSettings_Normalize_PreservesValidation(t *testing.T) {
	t.Parallel()

	for _, raw := range []string{"", "invalid", " 1000 ", "0", "-100"} {
		t.Run(raw, func(t *testing.T) {
			t.Parallel()

			limit := raw
			settings := &BalanceSettings{OverdraftLimitEnabled: true, OverdraftLimit: &limit}
			before := settings.Validate()
			require.Error(t, before)

			settings.Normalize()

			require.EqualError(t, settings.Validate(), before.Error())
		})
	}

	t.Run("disabled limit remains invalid", func(t *testing.T) {
		t.Parallel()

		limit := "1E+3"
		settings := &BalanceSettings{OverdraftLimit: &limit}
		before := settings.Validate()
		require.Error(t, before)

		settings.Normalize()

		require.EqualError(t, settings.Validate(), before.Error())
	})
}

// TestBalanceSettings_Validate_ValidCombinations covers the 4 valid combinations
// from the balance settings contract.
func TestBalanceSettings_Validate_ValidCombinations(t *testing.T) {
	t.Parallel()

	limit := "1000.00"

	tests := []struct {
		name     string
		settings BalanceSettings
	}{
		{
			name: "transactional, no overdraft allowed (default)",
			settings: BalanceSettings{
				BalanceScope:          "transactional",
				AllowOverdraft:        false,
				OverdraftLimitEnabled: false,
			},
		},
		{
			name: "transactional, overdraft allowed without limit (unlimited)",
			settings: BalanceSettings{
				BalanceScope:          "transactional",
				AllowOverdraft:        true,
				OverdraftLimitEnabled: false,
			},
		},
		{
			name: "transactional, overdraft allowed with limit",
			settings: BalanceSettings{
				BalanceScope:          "transactional",
				AllowOverdraft:        true,
				OverdraftLimitEnabled: true,
				OverdraftLimit:        &limit,
			},
		},
		{
			name: "internal scope with default overdraft flags",
			settings: BalanceSettings{
				BalanceScope:          "internal",
				AllowOverdraft:        false,
				OverdraftLimitEnabled: false,
			},
		},
		{
			name: "empty scope is accepted and defaults to transactional",
			settings: BalanceSettings{
				BalanceScope:          "",
				AllowOverdraft:        false,
				OverdraftLimitEnabled: false,
			},
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := tt.settings.Validate()

			require.NoError(t, err, "valid settings must not return error")
		})
	}
}

// TestBalanceSettings_Validate_InvalidCombinations covers every rejection path
// described in the balance settings contract.
func TestBalanceSettings_Validate_InvalidCombinations(t *testing.T) {
	t.Parallel()

	zero := "0"
	negative := "-100.00"
	empty := ""
	notANumber := "not-a-number"
	valid := "500.00"

	tests := []struct {
		name            string
		settings        BalanceSettings
		wantErrContains string
	}{
		{
			name: "overdraftLimitEnabled true without overdraftLimit",
			settings: BalanceSettings{
				BalanceScope:          "transactional",
				AllowOverdraft:        true,
				OverdraftLimitEnabled: true,
				OverdraftLimit:        nil,
			},
			wantErrContains: "overdraftLimit is required",
		},
		{
			name: "overdraftLimitEnabled true with empty overdraftLimit",
			settings: BalanceSettings{
				BalanceScope:          "transactional",
				AllowOverdraft:        true,
				OverdraftLimitEnabled: true,
				OverdraftLimit:        &empty,
			},
			wantErrContains: "non-empty decimal string",
		},
		{
			name: "overdraftLimitEnabled true with zero overdraftLimit",
			settings: BalanceSettings{
				BalanceScope:          "transactional",
				AllowOverdraft:        true,
				OverdraftLimitEnabled: true,
				OverdraftLimit:        &zero,
			},
			wantErrContains: "strictly greater than zero",
		},
		{
			name: "overdraftLimitEnabled true with negative overdraftLimit",
			settings: BalanceSettings{
				BalanceScope:          "transactional",
				AllowOverdraft:        true,
				OverdraftLimitEnabled: true,
				OverdraftLimit:        &negative,
			},
			wantErrContains: "strictly greater than zero",
		},
		{
			name: "overdraftLimitEnabled true with non-numeric overdraftLimit",
			settings: BalanceSettings{
				BalanceScope:          "transactional",
				AllowOverdraft:        true,
				OverdraftLimitEnabled: true,
				OverdraftLimit:        &notANumber,
			},
			wantErrContains: "not a valid decimal",
		},
		{
			name: "overdraftLimitEnabled false with overdraftLimit present (ambiguous)",
			settings: BalanceSettings{
				BalanceScope:          "transactional",
				AllowOverdraft:        true,
				OverdraftLimitEnabled: false,
				OverdraftLimit:        &valid,
			},
			wantErrContains: "must be absent",
		},
		{
			name: "balanceScope is not one of allowed values",
			settings: BalanceSettings{
				BalanceScope:          "external",
				AllowOverdraft:        false,
				OverdraftLimitEnabled: false,
			},
			wantErrContains: "invalid balanceScope",
		},
		{
			name: "balanceScope has random casing / typo",
			settings: BalanceSettings{
				BalanceScope:          "TRANSACTIONAL",
				AllowOverdraft:        false,
				OverdraftLimitEnabled: false,
			},
			wantErrContains: "invalid balanceScope",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := tt.settings.Validate()

			require.Error(t, err, "invalid settings must return error for: %s", tt.name)
			assert.Contains(t, err.Error(), tt.wantErrContains,
				"error message must contain %q for case: %s", tt.wantErrContains, tt.name)
		})
	}
}

// TestDeepCopySettings_ReflectionGuard uses reflection to detect new pointer
// fields added to BalanceSettings in the future. If a developer adds a new
// *string or *SomeStruct field to BalanceSettings but forgets to deep-copy it
// in deepCopySettings, this test fails — forcing the copy to be updated.
func TestDeepCopySettings_ReflectionGuard(t *testing.T) {
	t.Parallel()

	// Seed every pointer field with a non-nil value so the reflection
	// check can distinguish shallow copy (same address) from deep copy
	// (different address, same value).
	limit := "500.00"
	src := &BalanceSettings{
		BalanceScope:          "transactional",
		AllowOverdraft:        true,
		OverdraftLimitEnabled: true,
		OverdraftLimit:        &limit,
	}

	cp := deepCopySettings(src)
	require.NotNil(t, cp)

	srcVal := reflect.ValueOf(src).Elem()
	cpVal := reflect.ValueOf(cp).Elem()

	for i := 0; i < srcVal.NumField(); i++ {
		field := srcVal.Type().Field(i)
		srcField := srcVal.Field(i)
		cpField := cpVal.Field(i)

		if srcField.Kind() != reflect.Ptr {
			continue
		}

		if srcField.IsNil() {
			t.Errorf("reflection guard: field %q is nil on the seed object — "+
				"add a non-nil seed value so the deep-copy check can verify "+
				"that deepCopySettings copies it", field.Name)
			continue
		}

		require.False(t, cpField.IsNil(),
			"field %q was non-nil on source but nil on copy — deepCopySettings must copy it", field.Name)
		assert.NotEqual(t, srcField.Pointer(), cpField.Pointer(),
			"field %q has the same pointer address on source and copy — "+
				"deepCopySettings must allocate a new value, not share the pointer", field.Name)
		assert.Equal(t, srcField.Elem().Interface(), cpField.Elem().Interface(),
			"field %q has different values on source and copy — deep copy must preserve the value", field.Name)
	}
}
