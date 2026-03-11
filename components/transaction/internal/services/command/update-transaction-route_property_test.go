// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"math/rand"
	"reflect"
	"testing"
	"testing/quick"

	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// validActions mirrors the enum constraint from OperationRouteActionInput.Action.
var validActions = []string{"direct", "hold", "commit", "cancel", "revert"}

// routeActionPair holds a generated (routeID, action) pair for property tests.
type routeActionPair struct {
	RouteID uuid.UUID
	Action  string
}

// generateRouteActionPairs produces a slice of 0..maxLen unique (routeID, action) pairs
// using the provided random source. Uniqueness is enforced via a set to match map-key
// deduplication semantics in the production code.
func generateRouteActionPairs(rng *rand.Rand, maxLen int) []routeActionPair {
	n := rng.Intn(maxLen + 1)
	seen := make(map[routeActionKeyFuzz]bool)

	var pairs []routeActionPair

	for i := 0; i < n; i++ {
		var id uuid.UUID

		for j := range id {
			id[j] = byte(rng.Intn(256))
		}

		action := validActions[rng.Intn(len(validActions))]
		key := routeActionKeyFuzz{RouteID: id, Action: action}

		if seen[key] {
			continue
		}

		seen[key] = true

		pairs = append(pairs, routeActionPair{RouteID: id, Action: action})
	}

	return pairs
}

// splitPairs converts a slice of routeActionPair into parallel slices suitable
// for computeRouteActionDiff.
func splitPairs(pairs []routeActionPair) ([]uuid.UUID, []string) {
	ids := make([]uuid.UUID, len(pairs))
	actions := make([]string, len(pairs))

	for i, p := range pairs {
		ids[i] = p.RouteID
		actions[i] = p.Action
	}

	return ids, actions
}

// toKeySet converts a slice of routeActionPair to a set of routeActionKeyFuzz for
// set-theoretic assertions.
func toKeySet(pairs []routeActionPair) map[routeActionKeyFuzz]bool {
	s := make(map[routeActionKeyFuzz]bool, len(pairs))
	for _, p := range pairs {
		s[routeActionKeyFuzz{RouteID: p.RouteID, Action: p.Action}] = true
	}

	return s
}

// toResultKeySet converts toAdd/toRemove OperationRouteActionInput slices back into sets.
// It rebuilds composite keys by looking them up in the source set they came from.
func toResultKeySet(entries []mmodel.OperationRouteActionInput, sourceSet map[routeActionKeyFuzz]bool) map[routeActionKeyFuzz]bool {
	result := make(map[routeActionKeyFuzz]bool, len(entries))

	for _, entry := range entries {
		key := routeActionKeyFuzz{RouteID: entry.OperationRouteID, Action: entry.Action}
		if sourceSet[key] {
			result[key] = true
		}
	}

	return result
}

// diffPropertyInput is a quick.Generator-compatible type that holds two sets of
// route-action pairs (existing and new) for property-based testing.
type diffPropertyInput struct {
	Existing []routeActionPair
	New      []routeActionPair
}

// Generate implements quick.Generator for diffPropertyInput.
func (diffPropertyInput) Generate(rng *rand.Rand, size int) reflect.Value {
	maxLen := size
	if maxLen > 20 {
		maxLen = 20
	}

	input := diffPropertyInput{
		Existing: generateRouteActionPairs(rng, maxLen),
		New:      generateRouteActionPairs(rng, maxLen),
	}

	return reflect.ValueOf(input)
}

// TestProperty_RouteActionDiff_DiffSymmetry verifies that toAdd contains only items
// present in new but NOT in existing, and toRemove contains only items in existing
// but NOT in new.
func TestProperty_RouteActionDiff_DiffSymmetry(t *testing.T) {
	config := &quick.Config{MaxCount: 1000}

	property := func(input diffPropertyInput) bool {
		existingIDs, existingActions := splitPairs(input.Existing)
		newIDs, newActions := splitPairs(input.New)

		toAdd, toRemove := computeRouteActionDiff(existingIDs, existingActions, newIDs, newActions)

		existingSet := toKeySet(input.Existing)
		newSet := toKeySet(input.New)

		// Every toAdd entry must come from a key in new but NOT in existing
		for _, entry := range toAdd {
			key := routeActionKeyFuzz{RouteID: entry.OperationRouteID, Action: entry.Action}
			if !newSet[key] || existingSet[key] {
				return false
			}
		}

		// Every toRemove entry must come from a key in existing but NOT in new
		for _, entry := range toRemove {
			key := routeActionKeyFuzz{RouteID: entry.OperationRouteID, Action: entry.Action}
			if !existingSet[key] || newSet[key] {
				return false
			}
		}

		return true
	}

	err := quick.Check(property, config)
	require.NoError(t, err)
}

// TestProperty_RouteActionDiff_NoOverlap verifies that no composite key appears in
// both toAdd and toRemove simultaneously.
func TestProperty_RouteActionDiff_NoOverlap(t *testing.T) {
	config := &quick.Config{MaxCount: 1000}

	property := func(input diffPropertyInput) bool {
		existingIDs, existingActions := splitPairs(input.Existing)
		newIDs, newActions := splitPairs(input.New)

		toAdd, toRemove := computeRouteActionDiff(existingIDs, existingActions, newIDs, newActions)

		existingSet := toKeySet(input.Existing)
		newSet := toKeySet(input.New)

		addKeys := toResultKeySet(toAdd, newSet)
		removeKeys := toResultKeySet(toRemove, existingSet)

		// No composite key should exist in both sets
		for key := range addKeys {
			if removeKeys[key] {
				return false
			}
		}

		return true
	}

	err := quick.Check(property, config)
	require.NoError(t, err)
}

// TestProperty_RouteActionDiff_Identity verifies that when existing == new (same
// routeID+action pairs), toAdd and toRemove are both empty.
func TestProperty_RouteActionDiff_Identity(t *testing.T) {
	config := &quick.Config{MaxCount: 1000}

	property := func(input diffPropertyInput) bool {
		// Use only the Existing set for both sides
		ids, actions := splitPairs(input.Existing)

		toAdd, toRemove := computeRouteActionDiff(ids, actions, ids, actions)

		return len(toAdd) == 0 && len(toRemove) == 0
	}

	err := quick.Check(property, config)
	require.NoError(t, err)
}

// TestProperty_RouteActionDiff_Completeness verifies the set-size balance invariant:
// |existingKeys| - |toRemove| + |toAdd| == |newKeys|
// where keys are unique (routeID, action) composite keys.
func TestProperty_RouteActionDiff_Completeness(t *testing.T) {
	config := &quick.Config{MaxCount: 1000}

	property := func(input diffPropertyInput) bool {
		existingIDs, existingActions := splitPairs(input.Existing)
		newIDs, newActions := splitPairs(input.New)

		toAdd, toRemove := computeRouteActionDiff(existingIDs, existingActions, newIDs, newActions)

		existingSet := toKeySet(input.Existing)
		newSet := toKeySet(input.New)

		// Count unique keys (map deduplicates automatically)
		existingCount := len(existingSet)
		newCount := len(newSet)

		// Since toAdd and toRemove now carry the full composite key, count them directly
		removeKeys := toResultKeySet(toRemove, existingSet)
		addKeys := toResultKeySet(toAdd, newSet)

		return existingCount-len(removeKeys)+len(addKeys) == newCount
	}

	err := quick.Check(property, config)
	require.NoError(t, err)
}

// TestProperty_RouteActionDiff_ActionAwareness verifies that the same routeID with
// different actions is treated as distinct entries. When the same UUID appears with
// action "direct" in existing and action "hold" in new, both toAdd and toRemove must
// contain that UUID.
func TestProperty_RouteActionDiff_ActionAwareness(t *testing.T) {
	config := &quick.Config{MaxCount: 1000}

	property := func(input diffPropertyInput) bool {
		// Generate a single UUID and pick two different actions
		if len(input.Existing) == 0 {
			return true // vacuously true for empty input
		}

		routeID := input.Existing[0].RouteID

		// Pick two different actions deterministically from the pool
		actionA := validActions[len(input.Existing)%len(validActions)]
		actionB := validActions[(len(input.Existing)+1)%len(validActions)]

		if actionA == actionB {
			actionB = validActions[(len(input.Existing)+2)%len(validActions)]
		}

		existingIDs := []uuid.UUID{routeID}
		existingActions := []string{actionA}
		newIDs := []uuid.UUID{routeID}
		newActions := []string{actionB}

		toAdd, toRemove := computeRouteActionDiff(existingIDs, existingActions, newIDs, newActions)

		// Same routeID, different action: must appear in both add and remove
		return len(toAdd) == 1 && len(toRemove) == 1 &&
			toAdd[0].OperationRouteID == routeID && toRemove[0].OperationRouteID == routeID &&
			toAdd[0].Action == actionB && toRemove[0].Action == actionA
	}

	err := quick.Check(property, config)
	require.NoError(t, err)

	// Explicit sub-test: verify all action pair permutations for a fixed UUID
	t.Run("AllActionPairPermutations", func(t *testing.T) {
		routeID := uuid.MustParse("01965ed9-7fa4-75b2-8872-fc9e8509ab0a")

		for i, actionA := range validActions {
			for j, actionB := range validActions {
				if i == j {
					continue
				}

				toAdd, toRemove := computeRouteActionDiff(
					[]uuid.UUID{routeID}, []string{actionA},
					[]uuid.UUID{routeID}, []string{actionB},
				)

				assert.Len(t, toAdd, 1, "routeID with action %q->%q: expected 1 addition", actionA, actionB)
				assert.Len(t, toRemove, 1, "routeID with action %q->%q: expected 1 removal", actionA, actionB)
				assert.Equal(t, routeID, toAdd[0].OperationRouteID, "toAdd should contain the routeID")
				assert.Equal(t, actionB, toAdd[0].Action, "toAdd should carry the new action")
				assert.Equal(t, routeID, toRemove[0].OperationRouteID, "toRemove should contain the routeID")
				assert.Equal(t, actionA, toRemove[0].Action, "toRemove should carry the old action")
			}
		}
	})
}
