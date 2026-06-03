// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package constant

// Pongo template engine constants for arithmetic and formatting.
const (
	// PercentBase is the multiplier used to convert a ratio to a percentage.
	PercentBase = 100

	// DecimalPrecisionPercent is the number of decimal places for formatted percentage strings.
	DecimalPrecisionPercent = 2

	// RoundingPrecision is the multiplier/divisor used to round floating-point results
	// to 10 decimal places, avoiding floating-point artifacts (e.g., ...0000000001).
	RoundingPrecision = 1e10

	// SliceFormatParts is the expected number of parts when parsing a "start:end" slice format.
	SliceFormatParts = 2
)
