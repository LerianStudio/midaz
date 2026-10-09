// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package model

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestTransactionType_Valid_TableCases(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		txType TransactionType
		want   bool
	}{
		{name: "PIX example constant", txType: TransactionTypePix, want: true},
		{name: "free-form BOLETO", txType: TransactionType("BOLETO"), want: true},
		{name: "digits, dash and underscore", txType: TransactionType("TED_01-X"), want: true},
		{name: "exactly 50 characters", txType: TransactionType(strings.Repeat("A", 50)), want: true},
		{name: "lowercase is not canonical", txType: TransactionType("pix"), want: false},
		{name: "padded is not canonical", txType: TransactionType(" PIX "), want: false},
		{name: "51 characters", txType: TransactionType(strings.Repeat("A", 51)), want: false},
		{name: "inner whitespace", txType: TransactionType("A B"), want: false},
		{name: "punctuation", txType: TransactionType("BAD VALUE!"), want: false},
		{name: "empty", txType: TransactionType(""), want: false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tc.want, tc.txType.Valid())
		})
	}
}

func TestNewTransactionType_TableCases(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		raw    string
		want   TransactionType
		wantOK bool
	}{
		{name: "canonical PIX", raw: "PIX", want: TransactionTypePix, wantOK: true},
		{name: "canonical BOLETO", raw: "BOLETO", want: TransactionType("BOLETO"), wantOK: true},
		{name: "lowercase is upper-cased", raw: "pix", want: TransactionTypePix, wantOK: true},
		{name: "padding is trimmed", raw: " boleto ", want: TransactionType("BOLETO"), wantOK: true},
		{name: "51 characters", raw: strings.Repeat("a", 51), wantOK: false},
		{name: "inner whitespace", raw: "a b", wantOK: false},
		{name: "empty", raw: "", wantOK: false},
		{name: "whitespace only", raw: "   ", wantOK: false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got, ok := NewTransactionType(tc.raw)
			assert.Equal(t, tc.wantOK, ok)

			if tc.wantOK {
				assert.Equal(t, tc.want, got)
				assert.True(t, got.Valid())
			}
		})
	}
}

func TestTransactionTypeConstants(t *testing.T) {
	t.Run("Success - constants have expected values", func(t *testing.T) {
		assert.Equal(t, TransactionType("CARD"), TransactionTypeCard)
		assert.Equal(t, TransactionType("WIRE"), TransactionTypeWire)
		assert.Equal(t, TransactionType("PIX"), TransactionTypePix)
		assert.Equal(t, TransactionType("CRYPTO"), TransactionTypeCrypto)
	})
}

func TestTransactionType_String(t *testing.T) {
	tests := []struct {
		name     string
		txType   TransactionType
		expected string
	}{
		{
			name:     "CARD returns CARD string",
			txType:   TransactionTypeCard,
			expected: "CARD",
		},
		{
			name:     "WIRE returns WIRE string",
			txType:   TransactionTypeWire,
			expected: "WIRE",
		},
		{
			name:     "PIX returns PIX string",
			txType:   TransactionTypePix,
			expected: "PIX",
		},
		{
			name:     "CRYPTO returns CRYPTO string",
			txType:   TransactionTypeCrypto,
			expected: "CRYPTO",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.expected, tc.txType.String())
		})
	}
}
