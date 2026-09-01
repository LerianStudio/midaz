// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package pkg

import (
	"math"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSafeIntToInt32(t *testing.T) {
	tests := []struct {
		name        string
		input       int
		expected    int32
		expectError bool
	}{
		{
			name:        "zero value",
			input:       0,
			expected:    0,
			expectError: false,
		},
		{
			name:        "positive value within range",
			input:       42,
			expected:    42,
			expectError: false,
		},
		{
			name:        "negative value within range",
			input:       -100,
			expected:    -100,
			expectError: false,
		},
		{
			name:        "max int32 value",
			input:       math.MaxInt32,
			expected:    math.MaxInt32,
			expectError: false,
		},
		{
			name:        "min int32 value",
			input:       math.MinInt32,
			expected:    math.MinInt32,
			expectError: false,
		},
		{
			name:        "overflow - exceeds max int32",
			input:       math.MaxInt32 + 1,
			expected:    0,
			expectError: true,
		},
		{
			name:        "overflow - below min int32",
			input:       math.MinInt32 - 1,
			expected:    0,
			expectError: true,
		},
		{
			name:        "large positive overflow",
			input:       math.MaxInt64,
			expected:    0,
			expectError: true,
		},
		{
			name:        "large negative overflow",
			input:       math.MinInt64,
			expected:    0,
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := SafeIntToInt32(tt.input)

			if tt.expectError {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), "integer overflow")
				assert.Equal(t, int32(0), result)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expected, result)
			}
		})
	}
}

func TestIsValidCurrency(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		currency string
		expected bool
	}{
		// Valid ISO 4217 currencies
		{"valid USD", "USD", true},
		{"valid BRL", "BRL", true},
		{"valid EUR", "EUR", true},
		{"valid GBP", "GBP", true},
		{"valid JPY", "JPY", true},

		// Invalid - not real ISO 4217 codes
		{"formatted but not ISO 4217", "XYZ", false},
		{"formatted but not ISO 4217 AAA", "AAA", false},

		// Invalid - wrong length
		{"empty string", "", false},
		{"single char", "U", false},
		{"two chars", "US", false},
		{"four chars", "USDD", false},
		{"five chars", "USDDD", false},

		// Invalid - lowercase (ISO 4217 requires uppercase)
		{"lowercase", "usd", false},
		{"mixed case lower first", "uSD", false},
		{"mixed case middle", "UsD", false},
		{"mixed case last", "USd", false},

		// Invalid - numbers
		{"all numbers", "123", false},
		{"numbers mixed", "US1", false},
		{"numbers at start", "1SD", false},

		// Invalid - special characters
		{"with space", "US ", false},
		{"with hyphen", "US-", false},
		{"with underscore", "US_", false},
		{"with dollar sign", "US$", false},
		{"with period", "US.", false},

		// Edge cases
		{"whitespace only", "   ", false},
		{"tab character", "\t\t\t", false},
		{"newline", "US\n", false},

		// Unicode edge cases
		{"non-ASCII char", "ÜSD", false},
		{"emoji", "US😃", false},
		{"cyrillic lookalike", "UЅD", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			result := IsValidCurrency(tt.currency)

			assert.Equal(t, tt.expected, result)
		})
	}
}
