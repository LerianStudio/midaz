// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

// Package fee provides utilities for calculating transaction fees based on various rules and package configurations.
package fee

import "strings"

// Asset precision based on ISO 4217 (fiat) and common crypto standards.
var assetPrecision = map[string]int32{
	// Fiat currencies (ISO 4217)
	"BRL": 2, "USD": 2, "EUR": 2, "GBP": 2, "CAD": 2, "AUD": 2,
	"JPY": 0, "KRW": 0, "CLP": 0, "VND": 0, "ISK": 0, "UGX": 0,
	"KWD": 3, "BHD": 3, "OMR": 3,
	// Cryptocurrencies
	"BTC":  8,
	"ETH":  18,
	"USDT": 6,
	"USDC": 6,
}

const defaultPrecision int32 = 2

// GetAssetPrecision returns the decimal precision for a given asset.
// The asset code is case-insensitive. Returns defaultPrecision (2) for unknown or empty assets.
func GetAssetPrecision(asset string) int32 {
	return getAssetPrecision(asset)
}

// getAssetPrecision returns the decimal precision for a given asset.
// The asset code is case-insensitive. Returns defaultPrecision (2) for unknown or empty assets.
func getAssetPrecision(asset string) int32 {
	asset = strings.ToUpper(asset)

	if asset == "" {
		return defaultPrecision
	}

	if precision, ok := assetPrecision[asset]; ok {
		return precision
	}

	return defaultPrecision
}
