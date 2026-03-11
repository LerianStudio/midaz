// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"testing"

	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
)

// routeActionKeyFuzz mirrors the routeActionKey struct defined in handleOperationRouteUpdates.
// This allows fuzz testing of the composite key diff logic without requiring mocked repositories.
type routeActionKeyFuzz struct {
	RouteID uuid.UUID
	Action  string
}

// computeRouteActionDiff replicates the diff logic from handleOperationRouteUpdates
// so it can be fuzz-tested in isolation. It returns OperationRouteActionInput entries to add and remove,
// preserving both the routeID and action.
func computeRouteActionDiff(
	existingRouteIDs []uuid.UUID,
	existingActions []string,
	newRouteIDs []uuid.UUID,
	newActions []string,
) (toAdd, toRemove []mmodel.OperationRouteActionInput) {
	existingKeys := make(map[routeActionKeyFuzz]bool)
	for i := 0; i < len(existingRouteIDs) && i < len(existingActions); i++ {
		existingKeys[routeActionKeyFuzz{RouteID: existingRouteIDs[i], Action: existingActions[i]}] = true
	}

	newKeys := make(map[routeActionKeyFuzz]bool)
	for i := 0; i < len(newRouteIDs) && i < len(newActions); i++ {
		newKeys[routeActionKeyFuzz{RouteID: newRouteIDs[i], Action: newActions[i]}] = true
	}

	// Find relationships to remove (exist currently but not in new list)
	for key := range existingKeys {
		if !newKeys[key] {
			toRemove = append(toRemove, mmodel.OperationRouteActionInput{
				OperationRouteID: key.RouteID,
				Action:           key.Action,
			})
		}
	}

	// Find relationships to add (in new list but don't exist currently)
	for key := range newKeys {
		if !existingKeys[key] {
			toAdd = append(toAdd, mmodel.OperationRouteActionInput{
				OperationRouteID: key.RouteID,
				Action:           key.Action,
			})
		}
	}

	return toAdd, toRemove
}

// FuzzRouteActionKeyDiff_ActionStrings fuzzes the diff logic with random action strings
// while keeping route IDs fixed. This tests that arbitrary action strings (empty, long,
// special characters, unicode, non-UTF8) do not cause panics in the map key comparison.
func FuzzRouteActionKeyDiff_ActionStrings(f *testing.F) {
	// Seed corpus: valid actions
	f.Add("direct", "direct")
	// Seed corpus: empty strings
	f.Add("", "")
	// Seed corpus: boundary - long action strings
	f.Add("aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", "direct")
	// Seed corpus: unicode characters
	f.Add("\xe6\x97\xa5\xe6\x9c\xac\xe8\xaa\x9e\xf0\x9f\x92\xb0", "revert")
	// Seed corpus: security payloads
	f.Add("<script>alert('xss')</script>", "'; DROP TABLE routes;--")
	// Seed corpus: mixed valid/invalid actions
	f.Add("hold", "cancel")
	// Seed corpus: null bytes and control characters
	f.Add("action\x00with\x00nulls", "\t\n\r")

	routeID1 := uuid.MustParse("01965ed9-7fa4-75b2-8872-fc9e8509ab0a")
	routeID2 := uuid.MustParse("01965ed9-7fa4-75b2-8872-fc9e8509ab0b")

	f.Fuzz(func(t *testing.T, existingAction, newAction string) {
		// Bound input to prevent excessive memory usage
		if len(existingAction) > 512 {
			existingAction = existingAction[:512]
		}

		if len(newAction) > 512 {
			newAction = newAction[:512]
		}

		existingRouteIDs := []uuid.UUID{routeID1, routeID2}
		existingActions := []string{existingAction, "direct"}
		newRouteIDs := []uuid.UUID{routeID1, routeID2}
		newActions := []string{newAction, "direct"}

		toAdd, toRemove := computeRouteActionDiff(existingRouteIDs, existingActions, newRouteIDs, newActions)

		// Invariant: if actions are the same, no changes should be detected
		if existingAction == newAction {
			assert.Empty(t, toAdd, "no additions expected when actions match")
			assert.Empty(t, toRemove, "no removals expected when actions match")
		}

		// Invariant: if actions differ for routeID1, both add and remove should include routeID1
		if existingAction != newAction {
			addIDs := make(map[uuid.UUID]bool)
			for _, entry := range toAdd {
				addIDs[entry.OperationRouteID] = true
			}

			removeIDs := make(map[uuid.UUID]bool)
			for _, entry := range toRemove {
				removeIDs[entry.OperationRouteID] = true
			}

			assert.True(t, addIDs[routeID1], "routeID1 should be in toAdd when action changes")
			assert.True(t, removeIDs[routeID1], "routeID1 should be in toRemove when action changes")
		}

		// Invariant: routeID2 always has "direct" in both sets, so never in add/remove
		for _, entry := range toAdd {
			assert.NotEqual(t, routeID2, entry.OperationRouteID, "routeID2 should not be in toAdd (action unchanged)")
		}

		for _, entry := range toRemove {
			assert.NotEqual(t, routeID2, entry.OperationRouteID, "routeID2 should not be in toRemove (action unchanged)")
		}
	})
}

// FuzzRouteActionKeyDiff_Combinations fuzzes the diff logic with random UUID bytes and action
// strings to test all combinations of existing and new route-action tuples. This verifies that
// the map-based diff never panics regardless of input shape.
func FuzzRouteActionKeyDiff_Combinations(f *testing.F) {
	// Helper to create deterministic UUID bytes for seeds
	zeroBytes := make([]byte, 16)
	id1Bytes := uuid.MustParse("01965ed9-7fa4-75b2-8872-fc9e8509ab0a")
	id2Bytes := uuid.MustParse("01965ed9-7fa4-75b2-8872-fc9e8509ab0b")

	// Seed corpus: matching entries (no diff)
	f.Add(id1Bytes[:], "direct", id1Bytes[:], "direct")
	// Seed corpus: different actions for same route
	f.Add(id1Bytes[:], "direct", id1Bytes[:], "hold")
	// Seed corpus: different routes, same action
	f.Add(id1Bytes[:], "direct", id2Bytes[:], "direct")
	// Seed corpus: nil/zero UUID
	f.Add(zeroBytes, "", zeroBytes, "")
	// Seed corpus: unicode action with random-looking UUID bytes
	f.Add([]byte{0xff, 0xfe, 0xfd, 0xfc, 0xfb, 0xfa, 0xf9, 0xf8, 0xf7, 0xf6, 0xf5, 0xf4, 0xf3, 0xf2, 0xf1, 0xf0}, "\xe6\x97\xa5\xe6\x9c\xac\xe8\xaa\x9e", id1Bytes[:], "commit")
	// Seed corpus: boundary - empty action with valid UUID
	f.Add(id2Bytes[:], "", id1Bytes[:], "revert")
	// Seed corpus: security payloads
	f.Add(id1Bytes[:], "'; DROP TABLE--", id2Bytes[:], "<script>alert(1)</script>")

	f.Fuzz(func(t *testing.T, existingIDBytes []byte, existingAction string, newIDBytes []byte, newAction string) {
		// Bound inputs
		if len(existingAction) > 512 {
			existingAction = existingAction[:512]
		}

		if len(newAction) > 512 {
			newAction = newAction[:512]
		}

		// Pad or truncate UUID bytes to exactly 16 bytes
		existingUUIDBytes := padToUUID(existingIDBytes)
		newUUIDBytes := padToUUID(newIDBytes)

		existingID, err := uuid.FromBytes(existingUUIDBytes)
		if err != nil {
			t.Skip("invalid UUID bytes for existing")
		}

		newID, err := uuid.FromBytes(newUUIDBytes)
		if err != nil {
			t.Skip("invalid UUID bytes for new")
		}

		toAdd, toRemove := computeRouteActionDiff(
			[]uuid.UUID{existingID},
			[]string{existingAction},
			[]uuid.UUID{newID},
			[]string{newAction},
		)

		existingKey := routeActionKeyFuzz{RouteID: existingID, Action: existingAction}
		newKey := routeActionKeyFuzz{RouteID: newID, Action: newAction}

		if existingKey == newKey {
			// Same composite key: no diff
			assert.Empty(t, toAdd, "same composite key should produce no additions")
			assert.Empty(t, toRemove, "same composite key should produce no removals")
		} else {
			// Different composite keys: old removed, new added
			assert.Len(t, toAdd, 1, "different composite key should produce 1 addition")
			assert.Len(t, toRemove, 1, "different composite key should produce 1 removal")
		}
	})
}

// FuzzRouteActionKeyDiff_EmptySlices fuzzes the diff logic where one or both sides have
// empty slices, ensuring the algorithm handles asymmetric inputs without panics.
func FuzzRouteActionKeyDiff_EmptySlices(f *testing.F) {
	id1Bytes := uuid.MustParse("01965ed9-7fa4-75b2-8872-fc9e8509ab0a")

	// Seed corpus: both empty
	f.Add(false, false, "direct")
	// Seed corpus: existing empty, new populated
	f.Add(false, true, "direct")
	// Seed corpus: existing populated, new empty
	f.Add(true, false, "hold")
	// Seed corpus: both populated
	f.Add(true, true, "commit")
	// Seed corpus: unicode action
	f.Add(true, false, "\xe6\x97\xa5\xe6\x9c\xac\xe8\xaa\x9e")
	// Seed corpus: empty action string
	f.Add(false, true, "")
	// Seed corpus: security payload
	f.Add(true, true, "'; DROP TABLE--")

	f.Fuzz(func(t *testing.T, hasExisting, hasNew bool, action string) {
		if len(action) > 512 {
			action = action[:512]
		}

		var existingIDs []uuid.UUID
		var existingActions []string
		var newIDs []uuid.UUID
		var newActions []string

		if hasExisting {
			existingIDs = []uuid.UUID{id1Bytes}
			existingActions = []string{action}
		}

		if hasNew {
			newIDs = []uuid.UUID{id1Bytes}
			newActions = []string{action}
		}

		toAdd, toRemove := computeRouteActionDiff(existingIDs, existingActions, newIDs, newActions)

		if hasExisting && hasNew {
			// Same route+action in both: no changes
			assert.Empty(t, toAdd)
			assert.Empty(t, toRemove)
		} else if hasExisting && !hasNew {
			// Only in existing: should be removed
			assert.Empty(t, toAdd)
			assert.Len(t, toRemove, 1)
		} else if !hasExisting && hasNew {
			// Only in new: should be added
			assert.Len(t, toAdd, 1)
			assert.Empty(t, toRemove)
		} else {
			// Both empty: no changes
			assert.Empty(t, toAdd)
			assert.Empty(t, toRemove)
		}
	})
}

// padToUUID pads or truncates a byte slice to exactly 16 bytes for UUID construction.
func padToUUID(b []byte) []byte {
	result := make([]byte, 16)

	copy(result, b)

	return result
}
