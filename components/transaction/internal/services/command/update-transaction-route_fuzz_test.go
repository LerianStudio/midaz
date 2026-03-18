// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
)

// computeRouteIDDiff replicates the diff logic from handleOperationRouteUpdates
// so it can be fuzz-tested in isolation. It returns route IDs to add and remove.
func computeRouteIDDiff(
	existingRouteIDs []uuid.UUID,
	newRouteIDs []uuid.UUID,
) (toAdd, toRemove []uuid.UUID) {
	existingSet := make(map[uuid.UUID]bool)
	for _, id := range existingRouteIDs {
		existingSet[id] = true
	}

	newSet := make(map[uuid.UUID]bool)
	for _, id := range newRouteIDs {
		newSet[id] = true
	}

	// Find relationships to remove (exist currently but not in new list)
	for id := range existingSet {
		if !newSet[id] {
			toRemove = append(toRemove, id)
		}
	}

	// Find relationships to add (in new list but don't exist currently)
	for id := range newSet {
		if !existingSet[id] {
			toAdd = append(toAdd, id)
		}
	}

	return toAdd, toRemove
}

// FuzzRouteIDDiff_Combinations fuzzes the diff logic with random UUID bytes
// to test all combinations of existing and new route IDs. This verifies that
// the map-based diff never panics regardless of input shape.
func FuzzRouteIDDiff_Combinations(f *testing.F) {
	id1Bytes := uuid.MustParse("01965ed9-7fa4-75b2-8872-fc9e8509ab0a")
	id2Bytes := uuid.MustParse("01965ed9-7fa4-75b2-8872-fc9e8509ab0b")
	zeroBytes := make([]byte, 16)

	// Seed corpus: matching entries (no diff)
	f.Add(id1Bytes[:], id1Bytes[:])
	// Seed corpus: different routes
	f.Add(id1Bytes[:], id2Bytes[:])
	// Seed corpus: nil/zero UUID
	f.Add(zeroBytes, zeroBytes)
	// Seed corpus: boundary - random-looking UUID bytes
	f.Add([]byte{0xff, 0xfe, 0xfd, 0xfc, 0xfb, 0xfa, 0xf9, 0xf8, 0xf7, 0xf6, 0xf5, 0xf4, 0xf3, 0xf2, 0xf1, 0xf0}, id1Bytes[:])

	f.Fuzz(func(t *testing.T, existingIDBytes []byte, newIDBytes []byte) {
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

		toAdd, toRemove := computeRouteIDDiff(
			[]uuid.UUID{existingID},
			[]uuid.UUID{newID},
		)

		if existingID == newID {
			// Same ID: no diff
			assert.Empty(t, toAdd, "same ID should produce no additions")
			assert.Empty(t, toRemove, "same ID should produce no removals")
		} else {
			// Different IDs: old removed, new added
			assert.Len(t, toAdd, 1, "different ID should produce 1 addition")
			assert.Len(t, toRemove, 1, "different ID should produce 1 removal")
		}
	})
}

// FuzzRouteIDDiff_EmptySlices fuzzes the diff logic where one or both sides have
// empty slices, ensuring the algorithm handles asymmetric inputs without panics.
func FuzzRouteIDDiff_EmptySlices(f *testing.F) {
	id1Bytes := uuid.MustParse("01965ed9-7fa4-75b2-8872-fc9e8509ab0a")

	// Seed corpus
	f.Add(false, false)
	f.Add(false, true)
	f.Add(true, false)
	f.Add(true, true)

	f.Fuzz(func(t *testing.T, hasExisting, hasNew bool) {
		var existingIDs []uuid.UUID
		var newIDs []uuid.UUID

		if hasExisting {
			existingIDs = []uuid.UUID{id1Bytes}
		}

		if hasNew {
			newIDs = []uuid.UUID{id1Bytes}
		}

		toAdd, toRemove := computeRouteIDDiff(existingIDs, newIDs)

		if hasExisting && hasNew {
			assert.Empty(t, toAdd)
			assert.Empty(t, toRemove)
		} else if hasExisting && !hasNew {
			assert.Empty(t, toAdd)
			assert.Len(t, toRemove, 1)
		} else if !hasExisting && hasNew {
			assert.Len(t, toAdd, 1)
			assert.Empty(t, toRemove)
		} else {
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
