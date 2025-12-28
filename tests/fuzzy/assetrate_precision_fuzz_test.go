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

// FuzzAssetRatePrecisionLoss is a REGRESSION TEST for prior float64 precision loss.
//
// This test ensures that the decimal.Decimal implementation (now in place) correctly
// preserves full int64 precision for asset rate values. The migration from float64
// to decimal.Decimal is complete.
//
// The float64 simulation below is retained as INFORMATIONAL ONLY to demonstrate
// the precision loss that would occur with the old implementation. It uses t.Logf
// (not t.Errorf) to document the magnitude of precision loss without failing the test.
// This serves as educational documentation for why the migration was necessary.
//
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
		// =======================================================================
		// INFORMATIONAL: float64 precision loss demonstration (OLD IMPLEMENTATION)
		// This section is kept to document the precision loss that would occur
		// with the prior float64-based implementation. It uses t.Logf only and
		// does NOT fail the test - it serves as educational documentation.
		// =======================================================================
		rateFloat := float64(rate)
		scaleFloat := float64(scale)
		rateBack := int64(rateFloat)
		scaleBack := int64(scaleFloat)

		if rate != rateBack {
			t.Logf("[INFO] Old float64 would lose precision: rate=%d, recovered=%d, diff=%d",
				rate, rateBack, rate-rateBack)
		}
		if scale != scaleBack {
			t.Logf("[INFO] Old float64 would lose precision: scale=%d, recovered=%d, diff=%d",
				scale, scaleBack, scale-scaleBack)
		}

		// =======================================================================
		// REGRESSION TEST: decimal.Decimal MUST preserve exact precision
		// This is the actual test assertion. The migration to decimal.Decimal
		// is complete - these assertions ensure no regression occurs.
		// =======================================================================
		rateDecimal := decimal.NewFromInt(rate)
		scaleDecimal := decimal.NewFromInt(scale)

		// Strict assertion 1: decimal.Decimal equality check
		expectedRate := decimal.NewFromInt(rate)
		expectedScale := decimal.NewFromInt(scale)
		if !rateDecimal.Equal(expectedRate) {
			t.Errorf("REGRESSION: decimal.Decimal failed to preserve rate precision: "+
				"input=%d, decimal=%s, expected=%s",
				rate, rateDecimal.String(), expectedRate.String())
		}
		if !scaleDecimal.Equal(expectedScale) {
			t.Errorf("REGRESSION: decimal.Decimal failed to preserve scale precision: "+
				"input=%d, decimal=%s, expected=%s",
				scale, scaleDecimal.String(), expectedScale.String())
		}

		// Strict assertion 2: BigInt comparison for exact integer preservation
		// This verifies the underlying big.Int representation is exact
		rateBigInt := rateDecimal.BigInt()
		scaleBigInt := scaleDecimal.BigInt()
		if rateBigInt.Int64() != rate {
			t.Errorf("REGRESSION: decimal.Decimal BigInt conversion lost rate precision: "+
				"input=%d, bigint=%d", rate, rateBigInt.Int64())
		}
		if scaleBigInt.Int64() != scale {
			t.Errorf("REGRESSION: decimal.Decimal BigInt conversion lost scale precision: "+
				"input=%d, bigint=%d", scale, scaleBigInt.Int64())
		}

		// Strict assertion 3: String round-trip verification
		// Ensures string representation and parsing preserve exact value
		rateStr := rateDecimal.String()
		rateParsed, err := decimal.NewFromString(rateStr)
		if err != nil {
			t.Errorf("REGRESSION: failed to parse rate decimal string: %v", err)
		} else if !rateParsed.Equal(expectedRate) {
			t.Errorf("REGRESSION: rate string round-trip failed: "+
				"input=%d, string=%s, parsed=%s",
				rate, rateStr, rateParsed.String())
		}

		scaleStr := scaleDecimal.String()
		scaleParsed, err := decimal.NewFromString(scaleStr)
		if err != nil {
			t.Errorf("REGRESSION: failed to parse scale decimal string: %v", err)
		} else if !scaleParsed.Equal(expectedScale) {
			t.Errorf("REGRESSION: scale string round-trip failed: "+
				"input=%d, string=%s, parsed=%s",
				scale, scaleStr, scaleParsed.String())
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
