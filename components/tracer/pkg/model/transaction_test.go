// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package model

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestTransactionType_IsValid(t *testing.T) {
	tests := []struct {
		name     string
		txType   TransactionType
		expected bool
	}{
		{
			name:     "Success - CARD is valid",
			txType:   TransactionTypeCard,
			expected: true,
		},
		{
			name:     "Success - WIRE is valid",
			txType:   TransactionTypeWire,
			expected: true,
		},
		{
			name:     "Success - PIX is valid",
			txType:   TransactionTypePix,
			expected: true,
		},
		{
			name:     "Success - CRYPTO is valid",
			txType:   TransactionTypeCrypto,
			expected: true,
		},
		{
			name:     "Error - empty string is invalid",
			txType:   TransactionType(""),
			expected: false,
		},
		{
			name:     "Error - lowercase card is invalid",
			txType:   TransactionType("card"),
			expected: false,
		},
		{
			name:     "Error - random string is invalid",
			txType:   TransactionType("INVALID"),
			expected: false,
		},
		{
			name:     "Error - partial match is invalid",
			txType:   TransactionType("CAR"),
			expected: false,
		},
		{
			name:     "Error - ACH is invalid (not supported)",
			txType:   TransactionType("ACH"),
			expected: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := tc.txType.IsValid()
			assert.Equal(t, tc.expected, result)
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
