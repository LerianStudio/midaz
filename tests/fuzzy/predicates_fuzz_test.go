package fuzzy

import (
	"strings"
	"testing"

	"github.com/LerianStudio/midaz/v3/pkg/assert"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

// FuzzValidUUID tests the ValidUUID predicate against uuid.Parse for consistency.
// Run with: go test -v ./tests/fuzzy -fuzz=FuzzValidUUID -run=^$ -fuzztime=30s
func FuzzValidUUID(f *testing.F) {
	// Valid UUIDs
	f.Add("00000000-0000-0000-0000-000000000000") // Nil UUID
	f.Add("00000000-0000-0000-0000-000000000001") // Valid
	f.Add("ffffffff-ffff-ffff-ffff-ffffffffffff") // All F
	f.Add("a1b2c3d4-e5f6-7890-abcd-ef1234567890") // Mixed case

	// Invalid UUIDs
	f.Add("not-a-uuid")
	f.Add("")
	f.Add(strings.Repeat("0", 36))                 // No hyphens
	f.Add("00000000-0000-0000-0000-00000000000")   // Too short (35 chars)
	f.Add("00000000-0000-0000-0000-0000000000001") // Too long (37 chars)
	f.Add("00000000_0000_0000_0000_000000000001")  // Wrong separator
	f.Add("g0000000-0000-0000-0000-000000000001")  // Invalid hex char
	f.Add("00000000-0000-0000-0000-000000000001\n") // Trailing newline
	f.Add(" 00000000-0000-0000-0000-000000000001") // Leading space

	f.Fuzz(func(t *testing.T, s string) {
		result := assert.ValidUUID(s)

		// Cross-check with uuid.Parse
		_, err := uuid.Parse(s)
		expected := err == nil

		if result != expected {
			t.Errorf("ValidUUID(%q) = %v, but uuid.Parse returned err=%v", s, result, err)
		}
	})
}

// FuzzInRange tests the InRange predicate with diverse int64 values.
// Run with: go test -v ./tests/fuzzy -fuzz=FuzzInRange -run=^$ -fuzztime=30s
func FuzzInRange(f *testing.F) {
	// Normal cases
	f.Add(int64(5), int64(0), int64(10))  // In range
	f.Add(int64(0), int64(0), int64(10))  // At min boundary
	f.Add(int64(10), int64(0), int64(10)) // At max boundary
	f.Add(int64(-1), int64(0), int64(10)) // Below min
	f.Add(int64(11), int64(0), int64(10)) // Above max

	// Edge cases
	f.Add(int64(5), int64(10), int64(0))          // Inverted range
	f.Add(int64(0), int64(0), int64(0))           // Single value range
	f.Add(int64(1), int64(0), int64(0))           // Single value range, out of range
	f.Add(int64(1<<62), int64(0), int64(1<<63-1)) // Large positive values
	f.Add(int64(-1<<62), int64(-1<<63), int64(0)) // Large negative values
	f.Add(int64(0), int64(-1<<63), int64(1<<63-1)) // Full int64 range

	f.Fuzz(func(t *testing.T, n, minVal, maxVal int64) {
		result := assert.InRange(n, minVal, maxVal)

		// Manual verification
		var expected bool
		if minVal <= maxVal {
			expected = n >= minVal && n <= maxVal
		} else {
			// Inverted range should return false per predicate documentation
			expected = false
		}

		if result != expected {
			t.Errorf("InRange(%d, %d, %d) = %v, want %v", n, minVal, maxVal, result, expected)
		}
	})
}

// FuzzValidScale tests the ValidScale predicate with diverse scale values.
// Run with: go test -v ./tests/fuzzy -fuzz=FuzzValidScale -run=^$ -fuzztime=30s
func FuzzValidScale(f *testing.F) {
	// Boundary values
	f.Add(0)  // Min valid
	f.Add(18) // Max valid
	f.Add(-1) // Just below min
	f.Add(19) // Just above max

	// Edge cases
	f.Add(1)
	f.Add(9)
	f.Add(17)
	f.Add(100)
	f.Add(-100)
	f.Add(1 << 30)  // Large positive
	f.Add(-1 << 30) // Large negative

	f.Fuzz(func(t *testing.T, scale int) {
		result := assert.ValidScale(scale)
		expected := scale >= 0 && scale <= 18

		if result != expected {
			t.Errorf("ValidScale(%d) = %v, want %v", scale, result, expected)
		}
	})
}

// FuzzValidAmount tests the ValidAmount predicate with diverse decimal values.
// Run with: go test -v ./tests/fuzzy -fuzz=FuzzValidAmount -run=^$ -fuzztime=30s
func FuzzValidAmount(f *testing.F) {
	// Normal values
	f.Add("100", int32(0)) // 100, exp=0
	f.Add("1", int32(2))   // 100 (shifted), exp=2
	f.Add("1", int32(-2))  // 0.01, exp=-2

	// Boundary exponents
	f.Add("1", int32(-18)) // Min valid exponent
	f.Add("1", int32(18))  // Max valid exponent
	f.Add("1", int32(-19)) // Below min exponent (invalid)
	f.Add("1", int32(19))  // Above max exponent (invalid)

	// Edge cases
	f.Add("0", int32(0))                  // Zero
	f.Add("-100", int32(0))               // Negative
	f.Add("999999999999999999", int32(0)) // Large coefficient
	f.Add("1", int32(-30))                // Very small (invalid exp)
	f.Add("1", int32(30))                 // Very large (invalid exp)

	f.Fuzz(func(t *testing.T, valueStr string, shift int32) {
		d, err := decimal.NewFromString(valueStr)
		if err != nil {
			return // Invalid decimal string - skip
		}

		// Apply shift to test different exponents
		// Shift changes the exponent: positive shift multiplies by 10^shift
		d = d.Shift(shift)

		result := assert.ValidAmount(d)

		// Verify: exponent outside [-18, 18] should return false
		exp := d.Exponent()
		expectedValid := exp >= -18 && exp <= 18

		if result != expectedValid {
			t.Errorf("ValidAmount(%s) with exp=%d: got %v, want %v",
				d.String(), exp, result, expectedValid)
		}
	})
}
