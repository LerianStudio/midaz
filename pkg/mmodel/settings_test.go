// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package mmodel

import (
	"testing"

	"github.com/stretchr/testify/assert"
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
