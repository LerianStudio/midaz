// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package fee

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGetAssetPrecision(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name              string
		asset             string
		expectedPrecision int32
	}{
		// Known fiat currencies with 2 decimal places
		{
			name:              "BRL returns precision 2",
			asset:             "BRL",
			expectedPrecision: 2,
		},
		{
			name:              "USD returns precision 2",
			asset:             "USD",
			expectedPrecision: 2,
		},
		{
			name:              "EUR returns precision 2",
			asset:             "EUR",
			expectedPrecision: 2,
		},
		{
			name:              "GBP returns precision 2",
			asset:             "GBP",
			expectedPrecision: 2,
		},

		// Known fiat currencies with 0 decimal places
		{
			name:              "JPY returns precision 0",
			asset:             "JPY",
			expectedPrecision: 0,
		},

		// Known fiat currencies with 3 decimal places
		{
			name:              "KWD returns precision 3",
			asset:             "KWD",
			expectedPrecision: 3,
		},

		// Cryptocurrencies
		{
			name:              "BTC returns precision 8",
			asset:             "BTC",
			expectedPrecision: 8,
		},
		{
			name:              "ETH returns precision 18",
			asset:             "ETH",
			expectedPrecision: 18,
		},
		{
			name:              "USDT returns precision 6",
			asset:             "USDT",
			expectedPrecision: 6,
		},
		{
			name:              "USDC returns precision 6",
			asset:             "USDC",
			expectedPrecision: 6,
		},

		// Additional fiat currencies with 2 decimal places
		{
			name:              "CAD returns default precision 2",
			asset:             "CAD",
			expectedPrecision: 2,
		},
		{
			name:              "AUD returns default precision 2",
			asset:             "AUD",
			expectedPrecision: 2,
		},

		// Additional fiat currencies with 0 decimal places
		{
			name:              "KRW returns precision 0",
			asset:             "KRW",
			expectedPrecision: 0,
		},

		// Additional fiat currencies with 3 decimal places
		{
			name:              "BHD returns precision 3",
			asset:             "BHD",
			expectedPrecision: 3,
		},
		{
			name:              "OMR returns precision 3",
			asset:             "OMR",
			expectedPrecision: 3,
		},

		// Zero-decimal currencies
		{
			name:              "CLP returns precision 0",
			asset:             "CLP",
			expectedPrecision: 0,
		},
		{
			name:              "VND returns precision 0",
			asset:             "VND",
			expectedPrecision: 0,
		},

		// Case-insensitive lookups
		{
			name:              "lowercase brl returns precision 2",
			asset:             "brl",
			expectedPrecision: 2,
		},
		{
			name:              "lowercase btc returns precision 8",
			asset:             "btc",
			expectedPrecision: 8,
		},
		{
			name:              "lowercase jpy returns precision 0",
			asset:             "jpy",
			expectedPrecision: 0,
		},
		{
			name:              "mixed case Brl returns precision 2",
			asset:             "Brl",
			expectedPrecision: 2,
		},

		// Unknown and edge cases (should return default of 2)
		{
			name:              "unknown asset returns default precision 2",
			asset:             "XYZ",
			expectedPrecision: 2,
		},
		{
			name:              "empty string returns default precision 2",
			asset:             "",
			expectedPrecision: 2,
		},
	}

	for _, tt := range tests {
		tt := tt

		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			result := getAssetPrecision(tt.asset)

			assert.Equal(t, tt.expectedPrecision, result, "precision for asset %q", tt.asset)
		})
	}
}
