// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package mgrpc

import (
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
)

// Sentinel errors for shard range parsing and validation.
var (
	// ErrEmptyShardRanges is returned when an empty shard ranges string is provided.
	ErrEmptyShardRanges = errors.New("empty shard ranges string")

	// ErrInvalidShardRangeFormat is returned when a shard range string does not match "start-end" format.
	ErrInvalidShardRangeFormat = errors.New("invalid shard range format")

	// ErrInvalidShardRangeBounds is returned when a shard range has invalid start/end values.
	ErrInvalidShardRangeBounds = errors.New("invalid shard range bounds")

	// ErrShardRangesOverlap is returned when shard ranges overlap.
	ErrShardRangesOverlap = errors.New("shard ranges overlap")

	// ErrShardRangeOutOfBounds is returned when a shard range exceeds the shard count.
	ErrShardRangeOutOfBounds = errors.New("shard range out of bounds")
)

// expectedShardRangeParts is the number of parts expected when splitting a "start-end" range string.
const expectedShardRangeParts = 2

// ShardRange represents a contiguous range of shard IDs [Start, End] (inclusive).
type ShardRange struct {
	Start int
	End   int
}

// ParseShardRange parses a single "start-end" shard range string.
// It returns an error if the format is invalid or if start > end or start < 0.
func ParseShardRange(raw string) (ShardRange, error) {
	parts := strings.Split(strings.TrimSpace(raw), "-")
	if len(parts) != expectedShardRangeParts {
		return ShardRange{}, fmt.Errorf("%w: %q expected start-end", ErrInvalidShardRangeFormat, raw)
	}

	start, err := strconv.Atoi(strings.TrimSpace(parts[0]))
	if err != nil {
		return ShardRange{}, fmt.Errorf("invalid shard range start %q: %w", parts[0], err)
	}

	end, err := strconv.Atoi(strings.TrimSpace(parts[1]))
	if err != nil {
		return ShardRange{}, fmt.Errorf("invalid shard range end %q: %w", parts[1], err)
	}

	if start < 0 || end < start {
		return ShardRange{}, fmt.Errorf("%w: %q start=%d end=%d", ErrInvalidShardRangeBounds, raw, start, end)
	}

	return ShardRange{Start: start, End: end}, nil
}

// ParseShardRangeCSV parses a comma-separated list of "start-end" shard range
// strings. It validates that all ranges are well-formed, sorts them by start,
// and checks for overlaps. Returns an error if the input is empty or invalid.
func ParseShardRangeCSV(raw string) ([]ShardRange, error) {
	parts := splitCSV(raw)
	if len(parts) == 0 {
		return nil, ErrEmptyShardRanges
	}

	ranges := make([]ShardRange, 0, len(parts))
	for _, part := range parts {
		sr, err := ParseShardRange(part)
		if err != nil {
			return nil, err
		}

		ranges = append(ranges, sr)
	}

	sort.Slice(ranges, func(i, j int) bool {
		if ranges[i].Start == ranges[j].Start {
			return ranges[i].End < ranges[j].End
		}

		return ranges[i].Start < ranges[j].Start
	})

	for i := 1; i < len(ranges); i++ {
		prev := ranges[i-1]
		cur := ranges[i]

		if cur.Start <= prev.End {
			return nil, fmt.Errorf(
				"%w: between %d-%d and %d-%d",
				ErrShardRangesOverlap, prev.Start, prev.End, cur.Start, cur.End,
			)
		}
	}

	return ranges, nil
}

// ValidateShardRangeBounds checks that a ShardRange is within the bounds of the
// given shard count (i.e., end < shardCount).
func ValidateShardRangeBounds(sr ShardRange, shardCount int) error {
	if sr.End >= shardCount {
		return fmt.Errorf(
			"%w: range %d-%d exceeds shard count %d",
			ErrShardRangeOutOfBounds, sr.Start, sr.End, shardCount,
		)
	}

	return nil
}

// splitCSV splits a comma-separated string into trimmed, non-empty parts.
func splitCSV(raw string) []string {
	if strings.TrimSpace(raw) == "" {
		return nil
	}

	parts := strings.Split(raw, ",")

	out := make([]string, 0, len(parts))
	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		if trimmed == "" {
			continue
		}

		out = append(out, trimmed)
	}

	return out
}
