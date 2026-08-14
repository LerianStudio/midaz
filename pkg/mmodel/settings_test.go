// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package mmodel

import (
	"encoding/json"
	"errors"
	"testing"

	pkg "github.com/LerianStudio/midaz/v4/pkg"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLedgerSettings_StructExists(t *testing.T) {
	settings := LedgerSettings{
		Accounting: AccountingValidation{
			ValidateAccountType: true,
			ValidateRoutes:      true,
			RequireHolder:       true,
		},
	}

	assert.True(t, settings.Accounting.ValidateAccountType)
	assert.True(t, settings.Accounting.ValidateRoutes)
	assert.True(t, settings.Accounting.RequireHolder)
}

func TestDefaultLedgerSettings(t *testing.T) {
	settings := DefaultLedgerSettings()

	assert.False(t, settings.Accounting.ValidateAccountType, "ValidateAccountType must default to false")
	assert.False(t, settings.Accounting.ValidateRoutes, "ValidateRoutes must default to false")
	assert.False(t, settings.Accounting.RequireHolder, "RequireHolder must default to false")

	assert.Equal(t, "off", settings.Tracer.Mode, "Tracer.Mode must default to off")
	assert.Equal(t, "open", settings.Tracer.FailPosture, "Tracer.FailPosture must default to open")
	assert.Equal(t, 250, settings.Tracer.TimeoutMs, "Tracer.TimeoutMs must default to 250")
}

// TestLedgerSettings_Comparable enforces L1: LedgerSettings must stay ==-comparable
// (LedgerSettingsIsDefault relies on struct equality). This fails to compile if any
// field is changed to a non-comparable type such as a slice, map, or func.
func TestLedgerSettings_Comparable(t *testing.T) {
	a := DefaultLedgerSettings()
	b := DefaultLedgerSettings()

	assert.True(t, a == b, "default settings must be == equal")

	b.Tracer.Mode = "enforce"
	assert.False(t, a == b, "settings differing in Tracer.Mode must not be == equal")
}

func TestDefaultLedgerSettingsMap(t *testing.T) {
	settings := DefaultLedgerSettingsMap()

	assert.NotNil(t, settings)
	accounting, ok := settings["accounting"].(map[string]any)
	assert.True(t, ok, "accounting section must exist")
	assert.Equal(t, false, accounting["validateAccountType"])
	assert.Equal(t, false, accounting["validateRoutes"])
	assert.Equal(t, false, accounting["requireHolder"])

	tracer, ok := settings["tracer"].(map[string]any)
	assert.True(t, ok, "tracer section must exist")
	assert.Equal(t, "off", tracer["mode"])
	assert.Equal(t, "open", tracer["failPosture"])
	assert.Equal(t, 250, tracer["timeoutMs"])
}

// TestDefaultLedgerSettingsMap_SerializesIdenticallyForExistingLedgers asserts that the
// default map is deterministic and round-trips through JSON to a stable shape. Existing
// ledgers that never set tracer settings must resolve to these defaults, so the default
// map serialization is the contract their stored/absent settings are compared against.
func TestDefaultLedgerSettingsMap_SerializesIdenticallyForExistingLedgers(t *testing.T) {
	// The default map and the map produced from default typed settings must be identical.
	assert.Equal(t, DefaultLedgerSettingsMap(), LedgerSettingsToMap(DefaultLedgerSettings()),
		"DefaultLedgerSettingsMap must equal LedgerSettingsToMap(DefaultLedgerSettings())")

	// JSON serialization must be stable across repeated calls (deterministic keys).
	first, err := json.Marshal(DefaultLedgerSettingsMap())
	require.NoError(t, err)
	second, err := json.Marshal(DefaultLedgerSettingsMap())
	require.NoError(t, err)
	assert.JSONEq(t, string(first), string(second))

	// An existing ledger with no tracer group parses to the tracer defaults: behavior
	// is unchanged for settings written before the tracer group existed.
	legacy := ParseLedgerSettings(map[string]any{
		"accounting": map[string]any{
			"validateAccountType": true,
		},
	})
	assert.Equal(t, defaultTracerSettings, legacy.Tracer,
		"legacy settings without a tracer group must resolve to tracer defaults")
}

func TestParseLedgerSettings(t *testing.T) {
	tests := []struct {
		name     string
		input    map[string]any
		expected LedgerSettings
	}{
		{
			name:     "nil map returns defaults",
			input:    nil,
			expected: DefaultLedgerSettings(),
		},
		{
			name:     "empty map returns defaults",
			input:    map[string]any{},
			expected: DefaultLedgerSettings(),
		},
		{
			name: "missing accounting key returns defaults",
			input: map[string]any{
				"other": "value",
			},
			expected: DefaultLedgerSettings(),
		},
		{
			name: "accounting not a map returns defaults",
			input: map[string]any{
				"accounting": "not a map",
			},
			expected: DefaultLedgerSettings(),
		},
		{
			name: "all flags true",
			input: map[string]any{
				"accounting": map[string]any{
					"validateAccountType": true,
					"validateRoutes":      true,
					"requireHolder":       true,
				},
			},
			expected: LedgerSettings{
				Accounting: AccountingValidation{
					ValidateAccountType: true,
					ValidateRoutes:      true,
					RequireHolder:       true,
				},
				Tracer: defaultTracerSettings,
			},
		},
		{
			name: "only requireHolder true",
			input: map[string]any{
				"accounting": map[string]any{
					"requireHolder": true,
				},
			},
			expected: LedgerSettings{
				Accounting: AccountingValidation{
					ValidateAccountType: false,
					ValidateRoutes:      false,
					RequireHolder:       true,
				},
				Tracer: defaultTracerSettings,
			},
		},
		{
			name: "invalid type for requireHolder uses default",
			input: map[string]any{
				"accounting": map[string]any{
					"requireHolder": "not a bool",
				},
			},
			expected: LedgerSettings{
				Accounting: AccountingValidation{
					ValidateAccountType: false,
					ValidateRoutes:      false,
					RequireHolder:       false,
				},
				Tracer: defaultTracerSettings,
			},
		},
		{
			name: "only validateAccountType true",
			input: map[string]any{
				"accounting": map[string]any{
					"validateAccountType": true,
				},
			},
			expected: LedgerSettings{
				Accounting: AccountingValidation{
					ValidateAccountType: true,
					ValidateRoutes:      false,
				},
				Tracer: defaultTracerSettings,
			},
		},
		{
			name: "only validateRoutes true",
			input: map[string]any{
				"accounting": map[string]any{
					"validateRoutes": true,
				},
			},
			expected: LedgerSettings{
				Accounting: AccountingValidation{
					ValidateAccountType: false,
					ValidateRoutes:      true,
				},
				Tracer: defaultTracerSettings,
			},
		},
		{
			name: "invalid type for validateAccountType uses default",
			input: map[string]any{
				"accounting": map[string]any{
					"validateAccountType": "not a bool",
					"validateRoutes":      true,
				},
			},
			expected: LedgerSettings{
				Accounting: AccountingValidation{
					ValidateAccountType: false,
					ValidateRoutes:      true,
				},
				Tracer: defaultTracerSettings,
			},
		},
		{
			name: "extra fields are ignored",
			input: map[string]any{
				"accounting": map[string]any{
					"validateAccountType": true,
					"validateRoutes":      true,
					"unknownField":        "ignored",
				},
			},
			expected: LedgerSettings{
				Accounting: AccountingValidation{
					ValidateAccountType: true,
					ValidateRoutes:      true,
				},
				Tracer: defaultTracerSettings,
			},
		},
		{
			name: "explicit false values",
			input: map[string]any{
				"accounting": map[string]any{
					"validateAccountType": false,
					"validateRoutes":      false,
				},
			},
			expected: LedgerSettings{
				Accounting: AccountingValidation{
					ValidateAccountType: false,
					ValidateRoutes:      false,
				},
				Tracer: defaultTracerSettings,
			},
		},
		{
			name: "explicit tracer values are parsed",
			input: map[string]any{
				"tracer": map[string]any{
					"mode":        "enforce",
					"failPosture": "closed",
					"timeoutMs":   float64(500), // JSON numbers unmarshal to float64
				},
			},
			expected: LedgerSettings{
				Accounting: defaultAccountingValidation,
				Tracer: TracerSettings{
					Mode:        "enforce",
					FailPosture: "closed",
					TimeoutMs:   500,
				},
			},
		},
		{
			name: "partial tracer keeps other tracer defaults",
			input: map[string]any{
				"tracer": map[string]any{
					"mode": "advisory",
				},
			},
			expected: LedgerSettings{
				Accounting: defaultAccountingValidation,
				Tracer: TracerSettings{
					Mode:        "advisory",
					FailPosture: "open",
					TimeoutMs:   250,
				},
			},
		},
		{
			name: "tracer not a map returns tracer defaults",
			input: map[string]any{
				"tracer": "not a map",
			},
			expected: DefaultLedgerSettings(),
		},
		{
			name: "wrong type for tracer fields uses tracer defaults",
			input: map[string]any{
				"tracer": map[string]any{
					"mode":      123,
					"timeoutMs": "not a number",
				},
			},
			expected: DefaultLedgerSettings(),
		},
		{
			name: "tracer timeoutMs as int is parsed",
			input: map[string]any{
				"tracer": map[string]any{
					"timeoutMs": 750,
				},
			},
			expected: LedgerSettings{
				Accounting: defaultAccountingValidation,
				Tracer: TracerSettings{
					Mode:        "off",
					FailPosture: "open",
					TimeoutMs:   750,
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ParseLedgerSettings(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestValidateSettings(t *testing.T) {
	tests := []struct {
		name        string
		input       map[string]any
		wantErr     bool
		errContains string
		wantErrCode string // structured error code to assert, if non-empty
	}{
		{
			name:    "nil settings returns no error",
			input:   nil,
			wantErr: false,
		},
		{
			name:    "empty settings returns no error",
			input:   map[string]any{},
			wantErr: false,
		},
		{
			name: "valid complete settings",
			input: map[string]any{
				"accounting": map[string]any{
					"validateAccountType": true,
					"validateRoutes":      false,
					"requireHolder":       true,
				},
			},
			wantErr: false,
		},
		{
			name: "requireHolder wrong type returns error",
			input: map[string]any{
				"accounting": map[string]any{
					"requireHolder": "yes",
				},
			},
			wantErr:     true,
			errContains: "requireHolder",
		},
		{
			name: "valid partial settings",
			input: map[string]any{
				"accounting": map[string]any{
					"validateRoutes": true,
				},
			},
			wantErr: false,
		},
		{
			name: "null value at nested level is valid",
			input: map[string]any{
				"accounting": map[string]any{
					"validateRoutes": nil,
				},
			},
			wantErr: false,
		},
		{
			name: "null value at top level is valid",
			input: map[string]any{
				"accounting": nil,
			},
			wantErr: false,
		},
		{
			name: "unknown top-level key returns error",
			input: map[string]any{
				"unknownKey": true,
			},
			wantErr:     true,
			errContains: "unknownKey",
		},
		{
			name: "unknown nested key returns error",
			input: map[string]any{
				"accounting": map[string]any{
					"unknownField": true,
				},
			},
			wantErr:     true,
			errContains: "accounting.unknownField",
		},
		{
			name: "wrong type for boolean field returns error",
			input: map[string]any{
				"accounting": map[string]any{
					"validateRoutes": "yes",
				},
			},
			wantErr:     true,
			errContains: "validateRoutes",
		},
		{
			name: "wrong type at top level returns error",
			input: map[string]any{
				"accounting": "not a map",
			},
			wantErr:     true,
			errContains: "accounting",
		},
		{
			name: "number instead of boolean returns error",
			input: map[string]any{
				"accounting": map[string]any{
					"validateAccountType": 123,
				},
			},
			wantErr:     true,
			errContains: "validateAccountType",
		},
		{
			name: "root-level validateAccountType returns error with parent key in message",
			input: map[string]any{
				"validateAccountType": true,
			},
			wantErr:     true,
			errContains: "accounting",
			wantErrCode: "0149",
		},
		{
			name: "root-level validateRoutes returns error with field name in message",
			input: map[string]any{
				"validateRoutes": false,
			},
			wantErr:     true,
			errContains: "validateRoutes",
			wantErrCode: "0149",
		},
		{
			name: "mixed root-level and nested returns error for root-level",
			input: map[string]any{
				"validateAccountType": true,
				"accounting": map[string]any{
					"validateRoutes": true,
				},
			},
			wantErr:     true,
			errContains: "validateAccountType",
			wantErrCode: "0149",
		},
		{
			name: "multiple root-level fields returns error for first alphabetically",
			input: map[string]any{
				"validateAccountType": false,
				"validateRoutes":      true,
			},
			wantErr:     true,
			errContains: "validateAccountType", // Deterministic: alphabetically first field is reported
			wantErrCode: "0149",
		},
		{
			name: "valid tracer enums accepted",
			input: map[string]any{
				"tracer": map[string]any{
					"mode":        "enforce",
					"failPosture": "closed",
					"timeoutMs":   float64(500),
				},
			},
			wantErr: false,
		},
		{
			name: "all valid tracer mode values accepted",
			input: map[string]any{
				"tracer": map[string]any{
					"mode": "advisory",
				},
			},
			wantErr: false,
		},
		{
			name: "tracer mode typo rejected with field-value error",
			input: map[string]any{
				"tracer": map[string]any{
					"mode": "enfroce",
				},
			},
			wantErr:     true,
			errContains: "tracer.mode",
			wantErrCode: "0176",
		},
		{
			name: "tracer failPosture typo rejected with field-value error",
			input: map[string]any{
				"tracer": map[string]any{
					"failPosture": "closd",
				},
			},
			wantErr:     true,
			errContains: "tracer.failPosture",
			wantErrCode: "0176",
		},
		{
			name: "tracer mode wrong type rejected as type error not value error",
			input: map[string]any{
				"tracer": map[string]any{
					"mode": 123,
				},
			},
			wantErr:     true,
			errContains: "tracer.mode",
			wantErrCode: "0148", // type check fires before membership check
		},
		{
			name: "tracer timeoutMs wrong type rejected",
			input: map[string]any{
				"tracer": map[string]any{
					"timeoutMs": "fast",
				},
			},
			wantErr:     true,
			errContains: "tracer.timeoutMs",
			wantErrCode: "0148",
		},
		{
			name: "unknown tracer nested field rejected",
			input: map[string]any{
				"tracer": map[string]any{
					"unknownField": "x",
				},
			},
			wantErr:     true,
			errContains: "tracer.unknownField",
			wantErrCode: "0147",
		},
		{
			name: "null tracer value is valid",
			input: map[string]any{
				"tracer": nil,
			},
			wantErr: false,
		},
		{
			name: "null tracer mode is valid",
			input: map[string]any{
				"tracer": map[string]any{
					"mode": nil,
				},
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateSettings(tt.input)

			if tt.wantErr {
				require.Error(t, err)

				if tt.wantErrCode != "" {
					var vErr pkg.ValidationError
					require.True(t, errors.As(err, &vErr), "expected ValidationError type, got %T", err)
					assert.Equal(t, tt.wantErrCode, vErr.Code, "expected error code %q, got %q", tt.wantErrCode, vErr.Code)
				}

				assert.Contains(t, err.Error(), tt.errContains)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestDeepMergeSettings(t *testing.T) {
	tests := []struct {
		name     string
		existing map[string]any
		new      map[string]any
		expected map[string]any
	}{
		{
			name:     "nil existing with nil new returns empty map",
			existing: nil,
			new:      nil,
			expected: map[string]any{},
		},
		{
			name:     "nil existing with new settings returns new",
			existing: nil,
			new: map[string]any{
				"accounting": map[string]any{
					"validateRoutes": true,
				},
			},
			expected: map[string]any{
				"accounting": map[string]any{
					"validateRoutes": true,
				},
			},
		},
		{
			name: "existing with nil new returns existing",
			existing: map[string]any{
				"accounting": map[string]any{
					"validateAccountType": true,
				},
			},
			new: nil,
			expected: map[string]any{
				"accounting": map[string]any{
					"validateAccountType": true,
				},
			},
		},
		{
			name: "deep merge preserves unmodified nested values",
			existing: map[string]any{
				"accounting": map[string]any{
					"validateAccountType": true,
					"validateRoutes":      false,
				},
			},
			new: map[string]any{
				"accounting": map[string]any{
					"validateRoutes": true,
				},
			},
			expected: map[string]any{
				"accounting": map[string]any{
					"validateAccountType": true,
					"validateRoutes":      true,
				},
			},
		},
		{
			name: "new nested key adds to existing",
			existing: map[string]any{
				"accounting": map[string]any{
					"validateAccountType": true,
				},
			},
			new: map[string]any{
				"accounting": map[string]any{
					"validateRoutes": true,
				},
			},
			expected: map[string]any{
				"accounting": map[string]any{
					"validateAccountType": true,
					"validateRoutes":      true,
				},
			},
		},
		{
			name: "null value in new overwrites existing",
			existing: map[string]any{
				"accounting": map[string]any{
					"validateRoutes": true,
				},
			},
			new: map[string]any{
				"accounting": map[string]any{
					"validateRoutes": nil,
				},
			},
			expected: map[string]any{
				"accounting": map[string]any{
					"validateRoutes": nil,
				},
			},
		},
		{
			name: "null top-level value replaces existing",
			existing: map[string]any{
				"accounting": map[string]any{
					"validateRoutes": true,
				},
			},
			new: map[string]any{
				"accounting": nil,
			},
			expected: map[string]any{
				"accounting": nil,
			},
		},
		{
			name: "empty new map does not modify existing",
			existing: map[string]any{
				"accounting": map[string]any{
					"validateAccountType": true,
					"validateRoutes":      false,
				},
			},
			new: map[string]any{},
			expected: map[string]any{
				"accounting": map[string]any{
					"validateAccountType": true,
					"validateRoutes":      false,
				},
			},
		},
		{
			name:     "empty existing with empty new returns empty",
			existing: map[string]any{},
			new:      map[string]any{},
			expected: map[string]any{},
		},
		{
			name: "does not mutate original existing map",
			existing: map[string]any{
				"accounting": map[string]any{
					"validateAccountType": false,
				},
			},
			new: map[string]any{
				"accounting": map[string]any{
					"validateAccountType": true,
				},
			},
			expected: map[string]any{
				"accounting": map[string]any{
					"validateAccountType": true,
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Make a copy of existing to verify no mutation
			var existingCopy map[string]any
			if tt.existing != nil && tt.name == "does not mutate original existing map" {
				existingCopy = make(map[string]any)
				for k, v := range tt.existing {
					if m, ok := v.(map[string]any); ok {
						innerCopy := make(map[string]any)
						for ik, iv := range m {
							innerCopy[ik] = iv
						}
						existingCopy[k] = innerCopy
					} else {
						existingCopy[k] = v
					}
				}
			}

			result := DeepMergeSettings(tt.existing, tt.new)
			assert.Equal(t, tt.expected, result)

			// Verify original was not mutated for tests with non-nil existing
			if existingCopy != nil {
				assert.Equal(t, existingCopy, tt.existing, "original existing map must not be mutated")
			}
		})
	}
}

// TestSettingsDefaultOverridePolicyIsAllFalse asserts the override opt-ins default to false
// on both the typed defaults and their map form, so no control is skippable without an explicit opt-in.
func TestSettingsDefaultOverridePolicyIsAllFalse(t *testing.T) {
	settings := DefaultLedgerSettings()
	assert.False(t, settings.Overrides.AllowFeeSkip, "AllowFeeSkip must default to false")
	assert.False(t, settings.Overrides.AllowTracerSkip, "AllowTracerSkip must default to false")
	assert.False(t, settings.Overrides.AllowHolderSkip, "AllowHolderSkip must default to false")

	overrides, ok := DefaultLedgerSettingsMap()["overrides"].(map[string]any)
	require.True(t, ok, "overrides section must exist in default map")
	assert.Equal(t, false, overrides["allowFeeSkip"])
	assert.Equal(t, false, overrides["allowTracerSkip"])
	assert.Equal(t, false, overrides["allowHolderSkip"])
}

// TestSettingsParseOverridePolicy covers parsing of the overrides group: full object,
// each partial single-flag case (others stay false), empty/absent → all false, and
// wrong-type values falling back to the all-false default.
func TestSettingsParseOverridePolicy(t *testing.T) {
	tests := []struct {
		name     string
		input    map[string]any
		expected OverridePolicy
	}{
		{
			name:     "absent overrides group returns all false",
			input:    map[string]any{},
			expected: defaultOverridePolicy,
		},
		{
			name: "empty overrides group returns all false",
			input: map[string]any{
				"overrides": map[string]any{},
			},
			expected: defaultOverridePolicy,
		},
		{
			name: "all opt-ins true",
			input: map[string]any{
				"overrides": map[string]any{
					"allowFeeSkip":    true,
					"allowTracerSkip": true,
					"allowHolderSkip": true,
				},
			},
			expected: OverridePolicy{AllowFeeSkip: true, AllowTracerSkip: true, AllowHolderSkip: true},
		},
		{
			name: "only allowFeeSkip true leaves others false",
			input: map[string]any{
				"overrides": map[string]any{
					"allowFeeSkip": true,
				},
			},
			expected: OverridePolicy{AllowFeeSkip: true},
		},
		{
			name: "only allowTracerSkip true leaves others false",
			input: map[string]any{
				"overrides": map[string]any{
					"allowTracerSkip": true,
				},
			},
			expected: OverridePolicy{AllowTracerSkip: true},
		},
		{
			name: "only allowHolderSkip true leaves others false",
			input: map[string]any{
				"overrides": map[string]any{
					"allowHolderSkip": true,
				},
			},
			expected: OverridePolicy{AllowHolderSkip: true},
		},
		{
			name: "overrides not a map returns all false",
			input: map[string]any{
				"overrides": "not a map",
			},
			expected: defaultOverridePolicy,
		},
		{
			name: "wrong type for allowFeeSkip uses default",
			input: map[string]any{
				"overrides": map[string]any{
					"allowFeeSkip":    "not a bool",
					"allowTracerSkip": true,
				},
			},
			expected: OverridePolicy{AllowTracerSkip: true},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ParseLedgerSettings(tt.input)
			assert.Equal(t, tt.expected, result.Overrides)
		})
	}
}

// TestSettingsValidateOverridePolicy asserts the overrides group validates as a bool-only
// object: correct bools pass, an unknown nested key is rejected with ErrUnknownSettingsField,
// and a non-bool value is rejected as a type error.
func TestSettingsValidateOverridePolicy(t *testing.T) {
	tests := []struct {
		name        string
		input       map[string]any
		wantErr     bool
		errContains string
		wantErrCode string
	}{
		{
			name: "valid overrides accepted",
			input: map[string]any{
				"overrides": map[string]any{
					"allowFeeSkip":    true,
					"allowTracerSkip": false,
					"allowHolderSkip": true,
				},
			},
			wantErr: false,
		},
		{
			name: "partial overrides accepted",
			input: map[string]any{
				"overrides": map[string]any{
					"allowTracerSkip": true,
				},
			},
			wantErr: false,
		},
		{
			name: "null overrides value is valid",
			input: map[string]any{
				"overrides": nil,
			},
			wantErr: false,
		},
		{
			name: "unknown overrides nested key rejected",
			input: map[string]any{
				"overrides": map[string]any{
					"allowEverything": true,
				},
			},
			wantErr:     true,
			errContains: "overrides.allowEverything",
			wantErrCode: "0147",
		},
		{
			name: "non-bool override value rejected as type error",
			input: map[string]any{
				"overrides": map[string]any{
					"allowFeeSkip": "yes",
				},
			},
			wantErr:     true,
			errContains: "overrides.allowFeeSkip",
			wantErrCode: "0148",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateSettings(tt.input)

			if tt.wantErr {
				require.Error(t, err)

				if tt.wantErrCode != "" {
					var vErr pkg.ValidationError
					require.True(t, errors.As(err, &vErr), "expected ValidationError type, got %T", err)
					assert.Equal(t, tt.wantErrCode, vErr.Code, "expected error code %q, got %q", tt.wantErrCode, vErr.Code)
				}

				assert.Contains(t, err.Error(), tt.errContains)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

// TestSettingsOverridePolicyRoundTrip guards the create path (POST /ledgers runs through
// LedgerSettingsToMap): a typed LedgerSettings with overrides set must survive
// LedgerSettingsToMap -> ParseLedgerSettings without losing any opt-in. A drop here would
// silently discard CreateLedgerInput.Settings.Overrides.
func TestSettingsOverridePolicyRoundTrip(t *testing.T) {
	original := LedgerSettings{
		Accounting: defaultAccountingValidation,
		Tracer:     defaultTracerSettings,
		Overrides:  OverridePolicy{AllowFeeSkip: true},
	}

	roundTripped := ParseLedgerSettings(LedgerSettingsToMap(original))

	assert.Equal(t, original, roundTripped, "typed->map->typed round-trip must preserve overrides")
	assert.True(t, roundTripped.Overrides.AllowFeeSkip, "AllowFeeSkip must survive the round-trip")
	assert.False(t, roundTripped.Overrides.AllowTracerSkip, "unset AllowTracerSkip must stay false")
	assert.False(t, roundTripped.Overrides.AllowHolderSkip, "unset AllowHolderSkip must stay false")
}

// TestSettingsSchema_NoDuplicateNestedFieldNames validates that settingsSchema has no duplicate
// nested field names across different parent keys. If two parent keys define the same nested
// field name, knownNestedFieldNames would have nondeterministic behavior due to map iteration order.
// This test catches such issues at CI/CD time before deployment.
func TestSettingsSchema_NoDuplicateNestedFieldNames(t *testing.T) {
	// Track which parent key owns each field name
	fieldToParent := make(map[string]string)

	for parentKey, nestedFields := range settingsSchema {
		for fieldName := range nestedFields {
			if existingParent, exists := fieldToParent[fieldName]; exists {
				// Build suggestion safely, handling empty fieldName
				suggestion := parentKey
				if fieldName != "" {
					suggestion = parentKey + "." + fieldName
				}

				t.Fatalf(
					"settingsSchema has duplicate nested field name %q: defined in both %q and %q. "+
						"This causes nondeterministic behavior in knownNestedFieldNames. "+
						"Use unique field names or qualified names (e.g., %q instead of %q).",
					fieldName,
					existingParent,
					parentKey,
					suggestion,
					fieldName,
				)
			}

			fieldToParent[fieldName] = parentKey
		}
	}

	// Also verify knownNestedFieldNames was built correctly
	for fieldName, expectedParent := range fieldToParent {
		actualParent, exists := knownNestedFieldNames[fieldName]
		if !exists {
			t.Errorf("knownNestedFieldNames missing field %q (expected parent: %q)", fieldName, expectedParent)
		} else if actualParent != expectedParent {
			t.Errorf("knownNestedFieldNames[%q] = %q, expected %q", fieldName, actualParent, expectedParent)
		}
	}

	// Verify no extra fields in knownNestedFieldNames
	for fieldName := range knownNestedFieldNames {
		if _, exists := fieldToParent[fieldName]; !exists {
			t.Errorf("knownNestedFieldNames contains unexpected field %q", fieldName)
		}
	}
}

// FuzzValidateSettings verifies ValidateSettings never panics on arbitrary JSON input.
// It uses JSON strings as fuzz input since Go's fuzzing only supports primitive types.
// The function must either return nil (valid) or an error (invalid) - never panic.
func FuzzValidateSettings(f *testing.F) {
	// Seed corpus with known edge cases
	seeds := []string{
		`null`,
		`{}`,
		`{"accounting": {}}`,
		`{"accounting": {"validateRoutes": true}}`,
		`{"accounting": {"validateAccountType": true, "validateRoutes": false}}`,
		`{"accounting": null}`,
		`{"accounting": {"validateRoutes": null}}`,
		`{"unknown": true}`,
		`{"accounting": {"unknown": true}}`,
		`{"accounting": "not a map"}`,
		`{"accounting": {"validateRoutes": "yes"}}`,
		`{"accounting": {"validateRoutes": 123}}`,
		`{"accounting": {"validateRoutes": []}}`,
		`{"accounting": {"validateRoutes": {}}}`,
		`{"deeply": {"nested": {"structure": {"here": true}}}}`,
		`{"": {}}`,
		`{"accounting": {"": true}}`,
		`{"a": {"b": {"c": {"d": {"e": true}}}}}`,
		`[]`,
		`"string"`,
		`123`,
		`true`,
		`false`,
		// Root-level field cases (should be nested under parent key)
		`{"validateAccountType": true}`,
		`{"validateRoutes": false}`,
		`{"validateAccountType": true, "validateRoutes": false}`,
		`{"validateAccountType": true, "accounting": {"validateRoutes": true}}`,
	}

	for _, seed := range seeds {
		f.Add(seed)
	}

	f.Fuzz(func(t *testing.T, jsonInput string) {
		var settings map[string]any

		// Try to unmarshal as map - if it fails, that's fine (not a valid settings input)
		if err := json.Unmarshal([]byte(jsonInput), &settings); err != nil {
			// Not valid JSON or not a map - skip this input
			t.Skip("input is not valid JSON map")
		}

		// The function must not panic - it should return nil or error
		_ = ValidateSettings(settings)
	})
}

// FuzzDeepMergeSettings verifies DeepMergeSettings never panics and maintains immutability.
// Uses two JSON strings to represent existing and new settings maps.
func FuzzDeepMergeSettings(f *testing.F) {
	// Seed corpus with combinations of edge cases
	type seedPair struct {
		existing string
		new      string
	}

	seeds := []seedPair{
		{`null`, `null`},
		{`{}`, `{}`},
		{`{"accounting": {"validateRoutes": true}}`, `{"accounting": {"validateAccountType": true}}`},
		{`{"accounting": {"a": 1, "b": 2}}`, `{"accounting": {"b": 3, "c": 4}}`},
		{`{"accounting": null}`, `{"accounting": {"validateRoutes": true}}`},
		{`{"accounting": {"validateRoutes": true}}`, `{"accounting": null}`},
		{`{"a": {"x": 1}}`, `{"a": {"y": 2}}`},
		{`{"a": "string"}`, `{"a": {"nested": true}}`},
		{`{"a": {"nested": true}}`, `{"a": "string"}`},
		{`{}`, `{"new": {"field": true}}`},
		{`{"existing": {"field": false}}`, `{}`},
	}

	for _, seed := range seeds {
		f.Add(seed.existing, seed.new)
	}

	f.Fuzz(func(t *testing.T, existingJSON, newJSON string) {
		var existing, newSettings map[string]any

		// Parse existing - if not a map, use nil
		if err := json.Unmarshal([]byte(existingJSON), &existing); err != nil {
			existing = nil
		}

		// Parse new - if not a map, use nil
		if err := json.Unmarshal([]byte(newJSON), &newSettings); err != nil {
			newSettings = nil
		}

		// Deep copy existing for mutation check
		var existingCopy map[string]any
		if existing != nil {
			existingCopyBytes, _ := json.Marshal(existing)
			_ = json.Unmarshal(existingCopyBytes, &existingCopy)
		}

		// The function must not panic
		result := DeepMergeSettings(existing, newSettings)

		// Result must not be nil (contract: always returns a map)
		if result == nil {
			t.Error("DeepMergeSettings returned nil, expected non-nil map")
		}

		// Verify existing was not mutated (immutability contract)
		if existing != nil && existingCopy != nil {
			existingAfter, _ := json.Marshal(existing)
			existingBefore, _ := json.Marshal(existingCopy)
			if string(existingAfter) != string(existingBefore) {
				t.Error("DeepMergeSettings mutated the existing map")
			}
		}
	})
}

// FuzzParseLedgerSettings verifies ParseLedgerSettings never panics and always returns valid defaults.
func FuzzParseLedgerSettings(f *testing.F) {
	// Seed corpus with edge cases
	seeds := []string{
		`null`,
		`{}`,
		`{"accounting": {}}`,
		`{"accounting": {"validateRoutes": true}}`,
		`{"accounting": {"validateAccountType": true, "validateRoutes": false}}`,
		`{"accounting": null}`,
		`{"accounting": "not a map"}`,
		`{"accounting": {"validateRoutes": "not a bool"}}`,
		`{"accounting": {"validateRoutes": 123}}`,
		`{"accounting": {"validateRoutes": null}}`,
		`{"other": "value"}`,
		`{"accounting": [], "other": {}}`,
	}

	for _, seed := range seeds {
		f.Add(seed)
	}

	f.Fuzz(func(t *testing.T, jsonInput string) {
		var settings map[string]any

		// Try to unmarshal - if it fails, use nil
		if err := json.Unmarshal([]byte(jsonInput), &settings); err != nil {
			settings = nil
		}

		// The function must not panic
		result := ParseLedgerSettings(settings)

		// Result must be a valid LedgerSettings (the function always succeeds)
		// Just verify the fields are accessible without panic
		_ = result.Accounting.ValidateAccountType
		_ = result.Accounting.ValidateRoutes
	})
}
