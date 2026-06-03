// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package fee

import (
	"strings"
	"testing"
	"testing/quick"
)

// TestProperty_AssetPrecision_NonNegative verifies that getAssetPrecision
// returns a non-negative value for ALL possible string inputs.
// Invariant: precision >= 0, always.
func TestProperty_AssetPrecision_NonNegative(t *testing.T) {
	t.Parallel()

	property := func(asset string) bool {
		return getAssetPrecision(asset) >= 0
	}

	if err := quick.Check(property, &quick.Config{MaxCount: 1000}); err != nil {
		t.Fatalf("non-negative property violated: %v", err)
	}
}

// TestProperty_AssetPrecision_BoundedByMaxPrecision verifies that
// getAssetPrecision never returns a value exceeding the maximum known
// precision (18, from ETH). This guards against map corruption or
// future additions that could break callers expecting bounded output.
// Invariant: precision <= 18, always.
func TestProperty_AssetPrecision_BoundedByMaxPrecision(t *testing.T) {
	t.Parallel()

	const maxPrecision int32 = 18 // ETH has the highest precision in the map

	property := func(asset string) bool {
		return getAssetPrecision(asset) <= maxPrecision
	}

	if err := quick.Check(property, &quick.Config{MaxCount: 1000}); err != nil {
		t.Fatalf("bounded precision property violated: %v", err)
	}
}

// TestProperty_AssetPrecision_Deterministic verifies that calling
// getAssetPrecision with the same input always returns the same output.
// This is critical for financial calculations where repeated lookups
// must produce identical results.
// Invariant: f(x) == f(x), always.
func TestProperty_AssetPrecision_Deterministic(t *testing.T) {
	t.Parallel()

	property := func(asset string) bool {
		first := getAssetPrecision(asset)
		second := getAssetPrecision(asset)

		return first == second
	}

	if err := quick.Check(property, &quick.Config{MaxCount: 1000}); err != nil {
		t.Fatalf("deterministic property violated: %v", err)
	}
}

// TestProperty_AssetPrecision_KnownAssetsStable verifies that every
// asset registered in the assetPrecision map returns its documented
// precision value. This guards against accidental map mutations.
// Invariant: for all known assets, getAssetPrecision(asset) == assetPrecision[asset].
func TestProperty_AssetPrecision_KnownAssetsStable(t *testing.T) {
	t.Parallel()

	// Snapshot expected values at test time
	knownAssets := make(map[string]int32, len(assetPrecision))
	for k, v := range assetPrecision {
		knownAssets[k] = v
	}

	// Use quick.Check to select random known assets repeatedly
	keys := make([]string, 0, len(knownAssets))
	for k := range knownAssets {
		keys = append(keys, k)
	}

	property := func(index uint8) bool {
		if len(keys) == 0 {
			return true
		}

		key := keys[int(index)%len(keys)]
		expected := knownAssets[key]

		return getAssetPrecision(key) == expected
	}

	if err := quick.Check(property, &quick.Config{MaxCount: 1000}); err != nil {
		t.Fatalf("known assets stability property violated: %v", err)
	}
}

// TestProperty_AssetPrecision_DefaultConsistency verifies that any input
// NOT present in the assetPrecision map returns exactly defaultPrecision (2).
// This uses quick.Check to generate arbitrary strings, most of which will
// not be valid asset codes, confirming the fallback behavior is universal.
// Invariant: for all unknown assets, getAssetPrecision(asset) == defaultPrecision.
func TestProperty_AssetPrecision_DefaultConsistency(t *testing.T) {
	t.Parallel()

	property := func(asset string) bool {
		if _, isKnown := assetPrecision[strings.ToUpper(asset)]; isKnown {
			// Skip known assets; this property is about unknowns only
			return true
		}

		return getAssetPrecision(asset) == defaultPrecision
	}

	if err := quick.Check(property, &quick.Config{MaxCount: 1000}); err != nil {
		t.Fatalf("default consistency property violated: %v", err)
	}
}
