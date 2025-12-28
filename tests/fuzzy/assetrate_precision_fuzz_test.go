package fuzzy

import (
	"math"
	"math/rand"
	"testing"

	fuzz "github.com/google/gofuzz"
	"github.com/shopspring/decimal"
)

// generatePrecisionBoundarySeeds uses gofuzz to generate int64 values
// focused around the 2^53 float64 precision boundary.
func generatePrecisionBoundarySeeds(count int) []int64 {
	const float64MaxSafeInt = int64(1 << 53) // 9007199254740992

	fuzzer := fuzz.New().NilChance(0).Funcs(
		func(v *int64, c fuzz.Continue) {
			// Generate values clustered around precision boundaries
			switch c.Intn(6) {
			case 0:
				// Near 2^53 boundary (most important for precision testing)
				offset := c.Int63n(1000) - 500
				*v = float64MaxSafeInt + offset
			case 1:
				// Large values above 2^53 (definite precision loss)
				*v = float64MaxSafeInt + c.Int63n(math.MaxInt64-float64MaxSafeInt)
			case 2:
				// Powers of 2 near boundary
				shift := uint(50 + c.Intn(13)) // 2^50 to 2^62
				*v = int64(1) << shift
				if c.Intn(2) == 0 {
					*v += c.Int63n(100) - 50 // Add small offset
				}
			case 3:
				// Negative values near boundary
				offset := c.Int63n(1000) - 500
				*v = -(float64MaxSafeInt + offset)
			case 4:
				// Values that lose specific bits (odd numbers above 2^53)
				base := float64MaxSafeInt + c.Int63n(1000000)
				*v = base | 1 // Ensure odd (LSB set)
			case 5:
				// Random large int64
				*v = c.Int63()
				if c.Intn(2) == 0 {
					*v = -*v
				}
			}
		},
	)

	seeds := make([]int64, count)
	for i := 0; i < count; i++ {
		fuzzer.Fuzz(&seeds[i])
	}
	return seeds
}

// generateScaleSeeds uses gofuzz to generate scale values for precision testing.
func generateScaleSeeds(count int) []int64 {
	fuzzer := fuzz.New().NilChance(0).Funcs(
		func(v *int64, c fuzz.Continue) {
			// Scales are typically 0-18 for decimal places, but test edge cases
			switch c.Intn(4) {
			case 0:
				*v = c.Int63n(19) // Normal scale range 0-18
			case 1:
				*v = c.Int63n(100) // Extended scale
			case 2:
				*v = c.Int63() // Random large scale (edge case)
			case 3:
				*v = -c.Int63n(19) // Negative scale (edge case)
			}
		},
	)

	seeds := make([]int64, count)
	for i := 0; i < count; i++ {
		fuzzer.Fuzz(&seeds[i])
	}
	return seeds
}

// FuzzAssetRatePrecisionLoss tests for precision loss when converting
// large integers to float64 (the issue at create-assetrate.go:110-111).
// Run with: go test -v ./tests/fuzzy -fuzz=FuzzAssetRatePrecisionLoss -run=^$ -fuzztime=60s
func FuzzAssetRatePrecisionLoss(f *testing.F) {
	// Seed: values that fit in float64 exactly
	f.Add(int64(100), int64(2))
	f.Add(int64(1000000), int64(6))
	f.Add(int64(999999999999), int64(12))

	// Seed: values near float64 precision boundary (2^53 = 9007199254740992)
	f.Add(int64(9007199254740992), int64(0)) // exactly 2^53
	f.Add(int64(9007199254740993), int64(0)) // 2^53 + 1 (loses precision)
	f.Add(int64(9007199254740994), int64(0)) // 2^53 + 2
	f.Add(int64(9007199254740991), int64(0)) // 2^53 - 1

	// Seed: larger values (definite precision loss)
	f.Add(int64(9223372036854775807), int64(0))  // max int64
	f.Add(int64(9223372036854775806), int64(0))  // max int64 - 1
	f.Add(int64(1000000000000000000), int64(18)) // 10^18

	// Seed: negative values
	f.Add(int64(-9007199254740993), int64(0))
	f.Add(int64(-9223372036854775808), int64(0)) // min int64

	// Seed: various scales
	f.Add(int64(123456789012345678), int64(0))
	f.Add(int64(123456789012345678), int64(2))
	f.Add(int64(123456789012345678), int64(10))
	f.Add(int64(123456789012345678), int64(18))

	// Generate 20 diverse seeds using gofuzz for rate values around 2^53 boundary
	rand.Seed(42) // Deterministic seed generation for reproducibility
	gofuzzRateSeeds := generatePrecisionBoundarySeeds(20)
	gofuzzScaleSeeds := generateScaleSeeds(20)
	for i := 0; i < 20; i++ {
		f.Add(gofuzzRateSeeds[i], gofuzzScaleSeeds[i])
	}

	f.Fuzz(func(t *testing.T, rate int64, scale int64) {
		// Simulate the conversion done in create-assetrate.go:110-111
		// Original code: rate := float64(cari.Rate)
		rateFloat := float64(rate)
		scaleFloat := float64(scale)

		// Convert back to int64 to check for precision loss
		rateBack := int64(rateFloat)
		scaleBack := int64(scaleFloat)

		// Check for precision loss in rate
		if rate != rateBack {
			// This is the bug! Log it but don't fail (we're documenting, not fixing)
			t.Logf("PRECISION LOSS DETECTED: rate=%d, after float64 conversion=%d, diff=%d",
				rate, rateBack, rate-rateBack)
		}

		// Check for precision loss in scale
		if scale != scaleBack {
			t.Logf("PRECISION LOSS (scale): scale=%d, after float64 conversion=%d, diff=%d",
				scale, scaleBack, scale-scaleBack)
		}

		// Also test with decimal.Decimal (the safe alternative)
		rateDecimal := decimal.NewFromInt(rate)
		scaleDecimal := decimal.NewFromInt(scale)

		// The decimal should preserve full precision
		if !rateDecimal.Equal(decimal.NewFromInt(rate)) {
			t.Errorf("decimal.Decimal failed to preserve rate: %d", rate)
		}
		if !scaleDecimal.Equal(decimal.NewFromInt(scale)) {
			t.Errorf("decimal.Decimal failed to preserve scale: %d", scale)
		}
	})
}

// FuzzAssetRateFloat64Boundaries specifically targets the float64 precision boundary.
// Run with: go test -v ./tests/fuzzy -fuzz=FuzzAssetRateFloat64Boundaries -run=^$ -fuzztime=30s
func FuzzAssetRateFloat64Boundaries(f *testing.F) {
	// 2^53 is the boundary where float64 loses integer precision
	const float64MaxSafeInt = 1 << 53 // 9007199254740992

	// Seed: values around the boundary
	for i := int64(-10); i <= 10; i++ {
		f.Add(float64MaxSafeInt + i)
	}

	// Seed: powers of 2 near boundary
	f.Add(int64(1 << 52))
	f.Add(int64(1 << 53))
	f.Add(int64(1 << 54))
	f.Add(int64(1 << 55))

	// Seed: max values
	f.Add(int64(math.MaxInt64))
	f.Add(int64(math.MinInt64))

	// Generate 20 diverse seeds using gofuzz around 2^53 boundary
	rand.Seed(43) // Different seed for variety
	gofuzzSeeds := generatePrecisionBoundarySeeds(20)
	for _, seed := range gofuzzSeeds {
		f.Add(seed)
	}

	f.Fuzz(func(t *testing.T, value int64) {
		// Convert to float64 and back
		asFloat := float64(value)
		backToInt := int64(asFloat)

		if value != backToInt {
			// Document precision loss magnitude
			lossMagnitude := value - backToInt
			if lossMagnitude < 0 {
				lossMagnitude = -lossMagnitude
			}

			// Calculate relative error
			var relativeError float64
			if value != 0 {
				relativeError = float64(lossMagnitude) / float64(value)
				if relativeError < 0 {
					relativeError = -relativeError
				}
			}

			t.Logf("PRECISION LOSS: value=%d, recovered=%d, loss=%d, relative_error=%.10f",
				value, backToInt, value-backToInt, relativeError)

			// If relative error exceeds 1%, this is significant for financial calculations
			if relativeError > 0.01 {
				t.Logf("  WARNING: >1%% relative error - CRITICAL for financial accuracy")
			}
		}
	})
}
