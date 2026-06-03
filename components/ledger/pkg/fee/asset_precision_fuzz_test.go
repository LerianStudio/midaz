// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

//go:build go1.18
// +build go1.18

package fee

import (
	"strings"
	"testing"
)

// FuzzGetAssetPrecision fuzzes the asset precision lookup with arbitrary string inputs
// to verify the function never panics and always returns a valid precision value.
func FuzzGetAssetPrecision(f *testing.F) {
	// Seed corpus: minimum 5 entries covering all required categories

	// 1. Known assets (valid inputs)
	f.Add("BRL") // Fiat, precision 2
	f.Add("BTC") // Crypto, precision 8
	f.Add("JPY") // Fiat, precision 0
	f.Add("KWD") // Fiat, precision 3
	f.Add("ETH") // Crypto, precision 18

	// 2. Empty/nil edge case
	f.Add("") // Empty string, should return defaultPrecision

	// 3. Boundary values
	f.Add(strings.Repeat("X", 512)) // Very long string

	// 4. Unicode / international characters
	f.Add("日本語") // Japanese characters
	f.Add("🎉💰🪙") // Emoji characters

	// 5. Security payloads / special characters
	f.Add("' OR 1=1 --")               // SQL injection attempt
	f.Add("<script>alert(1)</script>") // XSS attempt
	f.Add("BRL\x00USD")                // Null byte injection
	f.Add("../../../etc/passwd")       // Path traversal attempt

	// 6. Invalid/unknown assets
	f.Add("UNKNOWN") // Unknown asset code
	f.Add("brl")     // Lowercase variant (not in map)
	f.Add("BRL ")    // Trailing space
	f.Add(" BRL")    // Leading space

	f.Fuzz(func(t *testing.T, asset string) {
		// Bound input length to prevent resource exhaustion
		if len(asset) > 512 {
			asset = asset[:512]
		}

		// Call function under test - must never panic
		result := getAssetPrecision(asset)

		// Verify result is always a valid precision (non-negative)
		if result < 0 {
			t.Errorf("getAssetPrecision(%q) returned negative precision: %d", asset, result)
		}

		// Verify known assets return their expected precision
		// Normalize to uppercase before map lookup since getAssetPrecision is case-insensitive
		if expected, ok := assetPrecision[strings.ToUpper(asset)]; ok {
			if result != expected {
				t.Errorf("getAssetPrecision(%q) = %d, want %d", asset, result, expected)
			}
		} else {
			// Unknown or empty assets must return defaultPrecision
			if result != defaultPrecision {
				t.Errorf("getAssetPrecision(%q) = %d, want defaultPrecision %d", asset, result, defaultPrecision)
			}
		}
	})
}
