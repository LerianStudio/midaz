// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package hash

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// =============================================================================
// HashUUIDToInt32 Utility for Advisory Locks Tests
// =============================================================================
// These tests define the behavior for HashUUIDToInt32 which is used to generate
// PostgreSQL advisory lock keys from request_id UUIDs (DD-8).
// =============================================================================

// TestHashUUIDToInt32_Deterministic verifies that the same UUID always produces
// the same int32 hash value. This is critical for advisory locks - concurrent
// requests with the same request_id MUST acquire the same lock.
func TestHashUUIDToInt32_Deterministic(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		uuid uuid.UUID
	}{
		{
			name: "nil UUID",
			uuid: uuid.Nil,
		},
		{
			name: "random UUID 1",
			uuid: uuid.MustParse("550e8400-e29b-41d4-a716-446655440000"),
		},
		{
			name: "random UUID 2",
			uuid: uuid.MustParse("6ba7b810-9dad-11d1-80b4-00c04fd430c8"),
		},
		{
			name: "max UUID",
			uuid: uuid.MustParse("ffffffff-ffff-ffff-ffff-ffffffffffff"),
		},
		{
			name: "sequential UUID pattern",
			uuid: uuid.MustParse("00000000-0000-0000-0000-000000000001"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// Call HashUUIDToInt32 multiple times with the same input
			// All results should be identical
			result1 := HashUUIDToInt32(tt.uuid)
			result2 := HashUUIDToInt32(tt.uuid)
			result3 := HashUUIDToInt32(tt.uuid)

			assert.Equal(t, result1, result2, "Hash should be deterministic (call 1 vs 2)")
			assert.Equal(t, result2, result3, "Hash should be deterministic (call 2 vs 3)")

			// Additional validation: result should be in valid int32 range
			// (Go's int32 is already bounded, but explicit check for documentation)
			require.True(t, result1 >= -2147483648 && result1 <= 2147483647,
				"Result should be valid int32")
		})
	}
}

// TestHashUUIDToInt32_DifferentInputs verifies that different UUIDs produce
// different hash values (with high probability). This ensures that advisory
// locks for different request_ids don't collide.
func TestHashUUIDToInt32_DifferentInputs(t *testing.T) {
	t.Parallel()

	// Generate a set of unique UUIDs
	uuids := []uuid.UUID{
		uuid.MustParse("00000000-0000-0000-0000-000000000001"),
		uuid.MustParse("00000000-0000-0000-0000-000000000002"),
		uuid.MustParse("550e8400-e29b-41d4-a716-446655440000"),
		uuid.MustParse("6ba7b810-9dad-11d1-80b4-00c04fd430c8"),
		uuid.MustParse("f47ac10b-58cc-4372-a567-0e02b2c3d479"),
		uuid.MustParse("aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee"),
		uuid.MustParse("11111111-2222-3333-4444-555555555555"),
		uuid.MustParse("deadbeef-cafe-babe-dead-beefcafebabe"),
	}

	// Hash all UUIDs
	hashes := make(map[int32]uuid.UUID)

	for _, id := range uuids {
		hash := HashUUIDToInt32(id)

		// Check for collision
		if existing, found := hashes[hash]; found {
			// In real-world, FNV-1a should not collide for these test cases
			// If it does, the test will fail and we need to investigate
			t.Errorf("Hash collision detected: UUID %s and UUID %s both hash to %d",
				existing.String(), id.String(), hash)
		}

		hashes[hash] = id
	}

	// Verify we got unique hashes for all inputs
	assert.Len(t, hashes, len(uuids), "All UUIDs should produce unique hashes (no collisions)")
}

// TestHashUUIDToInt32_BoundaryValues tests edge cases and boundary values
// to ensure the hash function handles them correctly.
func TestHashUUIDToInt32_BoundaryValues(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		uuid uuid.UUID
	}{
		{
			name: "nil UUID (all zeros)",
			uuid: uuid.Nil,
		},
		{
			name: "max UUID (all ones)",
			uuid: uuid.MustParse("ffffffff-ffff-ffff-ffff-ffffffffffff"),
		},
		{
			name: "alternating bits pattern 1",
			uuid: uuid.MustParse("aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa"),
		},
		{
			name: "alternating bits pattern 2",
			uuid: uuid.MustParse("55555555-5555-5555-5555-555555555555"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// Should not panic for any valid UUID
			result := HashUUIDToInt32(tt.uuid)

			// Result should be deterministic
			assert.Equal(t, result, HashUUIDToInt32(tt.uuid),
				"Hash should be deterministic for boundary values")
		})
	}
}

// TestHashUUIDToInt32_Distribution tests that the hash function produces
// a reasonable distribution of values across the int32 range.
// This is important for advisory lock performance - we don't want all locks
// to cluster in a small range.
func TestHashUUIDToInt32_Distribution(t *testing.T) {
	t.Parallel()

	// Generate many random UUIDs and check distribution
	const sampleSize = 1000
	positiveCount := 0
	negativeCount := 0

	for i := 0; i < sampleSize; i++ {
		id := uuid.New()
		hash := HashUUIDToInt32(id)

		if hash >= 0 {
			positiveCount++
		} else {
			negativeCount++
		}
	}

	// FNV-1a should produce a roughly even distribution across positive/negative
	// Allow for some variance (40%-60% split is reasonable)
	minExpected := sampleSize * 40 / 100
	maxExpected := sampleSize * 60 / 100

	assert.GreaterOrEqual(t, positiveCount, minExpected,
		"Distribution should have at least 40%% positive values")
	assert.LessOrEqual(t, positiveCount, maxExpected,
		"Distribution should have at most 60%% positive values")
	assert.GreaterOrEqual(t, negativeCount, minExpected,
		"Distribution should have at least 40%% negative values")
	assert.LessOrEqual(t, negativeCount, maxExpected,
		"Distribution should have at most 60%% negative values")
}
