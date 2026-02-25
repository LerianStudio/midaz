// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package mmodel

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestAccountingSettings_StructExists(t *testing.T) {
	settings := AccountingSettings{
		ValidateAccountType: true,
		ValidateRoutes:      true,
	}

	assert.True(t, settings.ValidateAccountType)
	assert.True(t, settings.ValidateRoutes)
}

func TestDefaultAccountingSettings(t *testing.T) {
	settings := DefaultAccountingSettings()

	assert.False(t, settings.ValidateAccountType, "ValidateAccountType must default to false")
	assert.False(t, settings.ValidateRoutes, "ValidateRoutes must default to false")
}

func TestParseAccountingSettings(t *testing.T) {
	tests := []struct {
		name     string
		input    map[string]any
		expected AccountingSettings
	}{
		{
			name:     "nil map returns defaults",
			input:    nil,
			expected: DefaultAccountingSettings(),
		},
		{
			name:     "empty map returns defaults",
			input:    map[string]any{},
			expected: DefaultAccountingSettings(),
		},
		{
			name: "missing accounting key returns defaults",
			input: map[string]any{
				"other": "value",
			},
			expected: DefaultAccountingSettings(),
		},
		{
			name: "accounting not a map returns defaults",
			input: map[string]any{
				"accounting": "not a map",
			},
			expected: DefaultAccountingSettings(),
		},
		{
			name: "both flags true",
			input: map[string]any{
				"accounting": map[string]any{
					"validateAccountType": true,
					"validateRoutes":      true,
				},
			},
			expected: AccountingSettings{
				ValidateAccountType: true,
				ValidateRoutes:      true,
			},
		},
		{
			name: "only validateAccountType true",
			input: map[string]any{
				"accounting": map[string]any{
					"validateAccountType": true,
				},
			},
			expected: AccountingSettings{
				ValidateAccountType: true,
				ValidateRoutes:      false,
			},
		},
		{
			name: "only validateRoutes true",
			input: map[string]any{
				"accounting": map[string]any{
					"validateRoutes": true,
				},
			},
			expected: AccountingSettings{
				ValidateAccountType: false,
				ValidateRoutes:      true,
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
			expected: AccountingSettings{
				ValidateAccountType: false,
				ValidateRoutes:      true,
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
			expected: AccountingSettings{
				ValidateAccountType: true,
				ValidateRoutes:      true,
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
			expected: AccountingSettings{
				ValidateAccountType: false,
				ValidateRoutes:      false,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ParseAccountingSettings(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}
