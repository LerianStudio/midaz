// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package testutil

import (
	"errors"
	"fmt"

	"github.com/google/uuid"
)

// Sentinel errors for DeterministicUUID and DeterministicUUIDs validation.
var (
	ErrInvalidCount    = errors.New("count must be > 0")
	ErrInvalidBase     = errors.New("startBase must be >= 0")
	ErrRangeExceedsMax = errors.New("range exceeds maximum base value")
	ErrBaseOutOfRange  = errors.New("base out of valid range")
)

// Ptr returns a pointer to any value. Generic helper for tests.
func Ptr[T any](v T) *T {
	return &v
}

// UUIDPtr returns a pointer to the given UUID.
// Wrapper around Ptr for discoverability.
func UUIDPtr(u uuid.UUID) *uuid.UUID {
	return Ptr(u)
}

// StringPtr returns a pointer to the given string.
// Wrapper around Ptr for discoverability.
func StringPtr(s string) *string {
	return Ptr(s)
}

// Int64Ptr returns a pointer to the given int64.
// Wrapper around Ptr for discoverability.
func Int64Ptr(i int64) *int64 {
	return Ptr(i)
}

// maxDeterministicBase is the maximum supported base value for DeterministicUUID.
// Using int64 to avoid overflow on 32-bit platforms.
const maxDeterministicBase int64 = 999_999_999_999

// DeterministicUUID generates a deterministic UUID based on a base integer.
// Useful for reproducible test results.
// Supports base values from 0 to 999,999,999,999.
// Returns ErrBaseOutOfRange if base is out of valid range.
func DeterministicUUID(base int64) (uuid.UUID, error) {
	if base < 0 || base > maxDeterministicBase {
		return uuid.Nil, fmt.Errorf("%w: must be between 0 and %d, got %d", ErrBaseOutOfRange, maxDeterministicBase, base)
	}

	return uuid.MustParse(fmt.Sprintf("00000000-0000-0000-0000-%012d", base)), nil
}

// MustDeterministicUUID is like DeterministicUUID but panics on error.
// Useful for test setup where errors indicate programmer mistakes.
func MustDeterministicUUID(base int64) uuid.UUID {
	id, err := DeterministicUUID(base)
	if err != nil {
		panic(fmt.Sprintf("MustDeterministicUUID: %v", err))
	}

	return id
}

// DeterministicUUIDs generates a slice of deterministic UUIDs starting from a base integer.
// Each UUID in the slice increments the base by 1.
// Returns an error if count <= 0, startBase < 0, or the range exceeds maxDeterministicBase.
func DeterministicUUIDs(count int, startBase int64) ([]uuid.UUID, error) {
	if count <= 0 {
		return nil, fmt.Errorf("%w: got %d", ErrInvalidCount, count)
	}

	if startBase < 0 {
		return nil, fmt.Errorf("%w: got %d", ErrInvalidBase, startBase)
	}

	// Check range without risking overflow: startBase + count - 1 <= max
	// Rearranged to: startBase <= max - count + 1 (safe since count > 0 and max is large)
	if startBase > maxDeterministicBase-int64(count)+1 {
		lastBase := startBase + int64(count) - 1
		return nil, fmt.Errorf("%w: range [%d, %d] exceeds max %d", ErrRangeExceedsMax, startBase, lastBase, maxDeterministicBase)
	}

	ids := make([]uuid.UUID, count)
	for i := range ids {
		id, err := DeterministicUUID(startBase + int64(i))
		if err != nil {
			return nil, err
		}

		ids[i] = id
	}

	return ids, nil
}

// MustDeterministicUUIDs is like DeterministicUUIDs but panics on error.
// Useful for test setup where errors indicate programmer mistakes.
func MustDeterministicUUIDs(count int, startBase int64) []uuid.UUID {
	ids, err := DeterministicUUIDs(count, startBase)
	if err != nil {
		panic(fmt.Sprintf("MustDeterministicUUIDs: %v", err))
	}

	return ids
}
