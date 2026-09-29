// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package pkg

import (
	"math"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/stretchr/testify/assert"

	"github.com/LerianStudio/midaz/v4/pkg/utils"
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

func TestIsValidAssetCode(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		code     string
		expected bool
	}{
		// ISO 4217 codes remain valid asset codes.
		{"valid USD", "USD", true},
		{"valid BRL", "BRL", true},
		{"valid EUR", "EUR", true},

		// Non-ISO asset codes the ledger accepts.
		{"crypto BTC", "BTC", true},
		{"stablecoin USDT", "USDT", true},
		{"loyalty POINTS", "POINTS", true},
		{"formatted but not ISO 4217", "XYZ", true},
		{"single letter", "U", true},
		{"two letters", "US", true},
		{"exactly 100 letters", strings.Repeat("A", 100), true},

		// Invalid - length
		{"empty string", "", false},
		{"101 letters", strings.Repeat("A", 101), false},

		// Invalid - lowercase
		{"lowercase", "usd", false},
		{"mixed case lower first", "uSD", false},
		{"mixed case last", "USd", false},

		// Invalid - digits
		{"all numbers", "123", false},
		{"digit at end", "BR1", false},
		{"digit at start", "1SD", false},

		// Invalid - special characters
		{"with space", "US ", false},
		{"with hyphen", "US-D", false},
		{"with underscore", "US_D", false},
		{"with dollar sign", "US$", false},
		{"with period", "US.", false},

		// Edge cases
		{"whitespace only", "   ", false},
		{"tab character", "\t\t\t", false},
		{"newline", "US\n", false},

		// Unicode: any uppercase letter is valid, exactly as the ledger accepts.
		{"uppercase Latin-1 letter", "ÜSD", true},
		{"uppercase Cyrillic letter", "UЅD", true},
		{"100 multi-byte runes", strings.Repeat("Ü", 100), true},
		{"101 multi-byte runes", strings.Repeat("Ü", 101), false},
		{"lowercase non-ASCII letter", "ÜSß", false},
		{"titlecase letter", "Uǅ", false},
		{"emoji", "US😃", false},
		{"invalid UTF-8", "US\xff", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tt.expected, IsValidAssetCode(tt.code))
		})
	}
}

// TestIsValidAssetCode_LedgerParity locks IsValidAssetCode to the ledger's asset
// code rule: non-empty, at most MaxAssetCodeLength runes, and every rune accepted
// by utils.ValidateCode. A code the ledger creates must be one the tracer accepts.
func TestIsValidAssetCode_LedgerParity(t *testing.T) {
	t.Parallel()

	codes := []string{
		"", "USD", "usd", "Usd", "BTC", "USDT", "POINTS", "U", "ÜSD", "ÉURO", "ΔΣ", "ЖЁ",
		"ÜSß", "Uǅ", "US1", "US D", "US-D", "US_D", "US😃", "日本", "US\xff",
		strings.Repeat("A", MaxAssetCodeLength), strings.Repeat("A", MaxAssetCodeLength+1),
		strings.Repeat("Ü", MaxAssetCodeLength), strings.Repeat("Ü", MaxAssetCodeLength+1),
	}

	for _, code := range codes {
		ledgerAccepts := code != "" &&
			utf8.RuneCountInString(code) <= MaxAssetCodeLength &&
			utils.ValidateCode(code) == nil

		assert.Equal(t, ledgerAccepts, IsValidAssetCode(code), "asset code %q", code)
	}
}
