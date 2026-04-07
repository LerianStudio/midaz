// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"math/rand"
	"reflect"
	"testing"
	"testing/quick"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// generateUUIDs produces a slice of 0..maxLen unique UUIDs
// using the provided random source.
func generateUUIDs(rng *rand.Rand, maxLen int) []uuid.UUID {
	n := rng.Intn(maxLen + 1)
	seen := make(map[uuid.UUID]bool)

	var ids []uuid.UUID

	for i := 0; i < n; i++ {
		var id uuid.UUID

		for j := range id {
			id[j] = byte(rng.Intn(256))
		}

		if seen[id] {
			continue
		}

		seen[id] = true

		ids = append(ids, id)
	}

	return ids
}

// diffPropertyInput is a quick.Generator-compatible type that holds two sets of
// route IDs (existing and new) for property-based testing.
type diffPropertyInput struct {
	Existing []uuid.UUID
	New      []uuid.UUID
}

// Generate implements quick.Generator for diffPropertyInput.
func (diffPropertyInput) Generate(rng *rand.Rand, size int) reflect.Value {
	maxLen := size
	if maxLen > 20 {
		maxLen = 20
	}

	input := diffPropertyInput{
		Existing: generateUUIDs(rng, maxLen),
		New:      generateUUIDs(rng, maxLen),
	}

	return reflect.ValueOf(input)
}

// toUUIDSet converts a slice of UUIDs to a set for set-theoretic assertions.
func toUUIDSet(ids []uuid.UUID) map[uuid.UUID]bool {
	s := make(map[uuid.UUID]bool, len(ids))
	for _, id := range ids {
		s[id] = true
	}

	return s
}

// TestProperty_RouteIDDiff_DiffSymmetry verifies that toAdd contains only items
// present in new but NOT in existing, and toRemove contains only items in existing
// but NOT in new.
func TestProperty_RouteIDDiff_DiffSymmetry(t *testing.T) {
	config := &quick.Config{MaxCount: 1000}

	property := func(input diffPropertyInput) bool {
		toAdd, toRemove := computeRouteIDDiff(input.Existing, input.New)

		existingSet := toUUIDSet(input.Existing)
		newSet := toUUIDSet(input.New)

		// Every toAdd entry must come from a key in new but NOT in existing
		for _, entry := range toAdd {
			if !newSet[entry] || existingSet[entry] {
				return false
			}
		}

		// Every toRemove entry must come from a key in existing but NOT in new
		for _, entry := range toRemove {
			if !existingSet[entry] || newSet[entry] {
				return false
			}
		}

		return true
	}

	err := quick.Check(property, config)
	require.NoError(t, err)
}

// TestProperty_RouteIDDiff_NoOverlap verifies that no ID appears in
// both toAdd and toRemove simultaneously.
func TestProperty_RouteIDDiff_NoOverlap(t *testing.T) {
	config := &quick.Config{MaxCount: 1000}

	property := func(input diffPropertyInput) bool {
		toAdd, toRemove := computeRouteIDDiff(input.Existing, input.New)

		addIDs := make(map[uuid.UUID]bool)
		for _, entry := range toAdd {
			addIDs[entry] = true
		}

		for _, entry := range toRemove {
			if addIDs[entry] {
				return false
			}
		}

		return true
	}

	err := quick.Check(property, config)
	require.NoError(t, err)
}

// TestProperty_RouteIDDiff_Identity verifies that when existing == new (same IDs),
// toAdd and toRemove are both empty.
func TestProperty_RouteIDDiff_Identity(t *testing.T) {
	config := &quick.Config{MaxCount: 1000}

	property := func(input diffPropertyInput) bool {
		// Use only the Existing set for both sides
		toAdd, toRemove := computeRouteIDDiff(input.Existing, input.Existing)

		return len(toAdd) == 0 && len(toRemove) == 0
	}

	err := quick.Check(property, config)
	require.NoError(t, err)
}

// TestProperty_RouteIDDiff_Completeness verifies the set-size balance invariant:
// |existingSet| - |toRemove| + |toAdd| == |newSet|
func TestProperty_RouteIDDiff_Completeness(t *testing.T) {
	config := &quick.Config{MaxCount: 1000}

	property := func(input diffPropertyInput) bool {
		toAdd, toRemove := computeRouteIDDiff(input.Existing, input.New)

		existingSet := toUUIDSet(input.Existing)
		newSet := toUUIDSet(input.New)

		return len(existingSet)-len(toRemove)+len(toAdd) == len(newSet)
	}

	err := quick.Check(property, config)
	require.NoError(t, err)
}

// TestProperty_RouteIDDiff_AllPairsExplicit verifies that when existing and new have
// different IDs, both toAdd and toRemove contain the correct entries.
func TestProperty_RouteIDDiff_AllPairsExplicit(t *testing.T) {
	routeID := uuid.MustParse("01965ed9-7fa4-75b2-8872-fc9e8509ab0a")
	otherID := uuid.MustParse("01965ed9-7fa4-75b2-8872-fc9e8509ab0b")

	toAdd, toRemove := computeRouteIDDiff(
		[]uuid.UUID{routeID},
		[]uuid.UUID{otherID},
	)

	assert.Len(t, toAdd, 1, "expected 1 addition")
	assert.Len(t, toRemove, 1, "expected 1 removal")
	assert.Equal(t, otherID, toAdd[0], "toAdd should contain the new ID")
	assert.Equal(t, routeID, toRemove[0], "toRemove should contain the old ID")
}
