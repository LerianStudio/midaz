// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package mmodel

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLedgerSettings_StructExists(t *testing.T) {
	settings := LedgerSettings{
		Accounting: AccountingValidation{
			ValidateAccountType: true,
			ValidateRoutes:      true,
		},
	}

	assert.True(t, settings.Accounting.ValidateAccountType)
	assert.True(t, settings.Accounting.ValidateRoutes)
}

func TestDefaultLedgerSettings(t *testing.T) {
	settings := DefaultLedgerSettings()

	assert.False(t, settings.Accounting.ValidateAccountType, "ValidateAccountType must default to false")
	assert.False(t, settings.Accounting.ValidateRoutes, "ValidateRoutes must default to false")
}

func TestDefaultLedgerSettingsMap(t *testing.T) {
	settings := DefaultLedgerSettingsMap()

	assert.NotNil(t, settings)
	accounting, ok := settings["accounting"].(map[string]any)
	assert.True(t, ok, "accounting section must exist")
	assert.Equal(t, false, accounting["validateAccountType"])
	assert.Equal(t, false, accounting["validateRoutes"])
}

func TestMergeSettingsWithDefaults(t *testing.T) {
	tests := []struct {
		name     string
		input    map[string]any
		expected map[string]any
	}{
		{
			name:  "nil map returns defaults",
			input: nil,
			expected: map[string]any{
				"accounting": map[string]any{
					"validateAccountType": false,
					"validateRoutes":      false,
				},
			},
		},
		{
			name:  "empty map returns defaults",
			input: map[string]any{},
			expected: map[string]any{
				"accounting": map[string]any{
					"validateAccountType": false,
					"validateRoutes":      false,
				},
			},
		},
		{
			name: "partial accounting section merges with defaults",
			input: map[string]any{
				"accounting": map[string]any{
					"validateAccountType": true,
				},
			},
			expected: map[string]any{
				"accounting": map[string]any{
					"validateAccountType": true,
					"validateRoutes":      false,
				},
			},
		},
		{
			name: "complete settings preserved",
			input: map[string]any{
				"accounting": map[string]any{
					"validateAccountType": true,
					"validateRoutes":      true,
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
			name: "extra fields in accounting preserved",
			input: map[string]any{
				"accounting": map[string]any{
					"validateAccountType": true,
					"customField":         "customValue",
				},
			},
			expected: map[string]any{
				"accounting": map[string]any{
					"validateAccountType": true,
					"validateRoutes":      false,
					"customField":         "customValue",
				},
			},
		},
		{
			name: "extra top-level sections preserved",
			input: map[string]any{
				"accounting": map[string]any{
					"validateAccountType": true,
				},
				"customSection": map[string]any{
					"key": "value",
				},
			},
			expected: map[string]any{
				"accounting": map[string]any{
					"validateAccountType": true,
					"validateRoutes":      false,
				},
				"customSection": map[string]any{
					"key": "value",
				},
			},
		},
		{
			name: "non-map accounting value preserved as-is",
			input: map[string]any{
				"accounting": "stringValue",
			},
			expected: map[string]any{
				"accounting": "stringValue",
			},
		},
		{
			name: "missing accounting section filled from defaults",
			input: map[string]any{
				"otherSection": "value",
			},
			expected: map[string]any{
				"accounting": map[string]any{
					"validateAccountType": false,
					"validateRoutes":      false,
				},
				"otherSection": "value",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := MergeSettingsWithDefaults(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
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
			name: "both flags true",
			input: map[string]any{
				"accounting": map[string]any{
					"validateAccountType": true,
					"validateRoutes":      true,
				},
			},
			expected: LedgerSettings{
				Accounting: AccountingValidation{
					ValidateAccountType: true,
					ValidateRoutes:      true,
				},
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
				},
			},
			wantErr: false,
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
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateSettings(tt.input)

			if tt.wantErr {
				require.Error(t, err)
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

// FuzzMergeSettingsWithDefaults verifies MergeSettingsWithDefaults never panics.
func FuzzMergeSettingsWithDefaults(f *testing.F) {
	// Seed corpus with edge cases
	seeds := []string{
		`null`,
		`{}`,
		`{"accounting": {}}`,
		`{"accounting": {"validateRoutes": true}}`,
		`{"accounting": {"validateAccountType": true, "validateRoutes": false}}`,
		`{"accounting": null}`,
		`{"accounting": "string value"}`,
		`{"customSection": {"key": "value"}}`,
		`{"accounting": {"validateRoutes": true}, "extra": {}}`,
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
		result := MergeSettingsWithDefaults(settings)

		// Result must contain at least the defaults
		if result == nil {
			t.Error("MergeSettingsWithDefaults returned nil, expected non-nil map")
		}

		// Accounting section must always exist in result
		if _, exists := result["accounting"]; !exists {
			t.Error("MergeSettingsWithDefaults result missing 'accounting' section")
		}
	})
}
