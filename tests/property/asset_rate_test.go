package property

import (
	"math"
	"testing"
	"testing/quick"

	"github.com/shopspring/decimal"
)

// Property: Rate conversion with scale preserves value semantics
// Actual rate = rate / 10^scale
func TestProperty_AssetRateScaleSemantics(t *testing.T) {
	f := func(rate int64, scale uint8) bool {
		// Constrain values
		if rate == 0 {
			return true // Skip zero rate
		}
		if rate < 0 {
			rate = -rate
		}

		s := int(scale) % 10 // 0-9 scale

		// Calculate actual rate: rate / 10^scale
		rateDecimal := decimal.NewFromInt(rate)
		divisor := decimal.NewFromInt(1)
		for i := 0; i < s; i++ {
			divisor = divisor.Mul(decimal.NewFromInt(10))
		}
		actualRate := rateDecimal.Div(divisor)

		// Property: actualRate * 10^scale should equal original rate
		reconstructed := actualRate.Mul(divisor)

		if !reconstructed.Equal(rateDecimal) {
			t.Logf("Scale semantics violated: rate=%d scale=%d actual=%s reconstructed=%s",
				rate, s, actualRate.String(), reconstructed.String())
			return false
		}

		return true
	}

	cfg := &quick.Config{MaxCount: 1000}
	if err := quick.Check(f, cfg); err != nil {
		t.Fatalf("Asset rate scale semantics property failed: %v", err)
	}
}

// Property: Converting amount with rate and back with inverse rate preserves value (within tolerance)
func TestProperty_AssetRateInverseRoundtrip(t *testing.T) {
	f := func(amount, rateNum, rateDenom int64) bool {
		// Skip invalid cases
		if rateNum == 0 || rateDenom == 0 {
			return true
		}

		// Constrain to reasonable values
		if amount < 0 {
			amount = -amount
		}
		if rateNum < 0 {
			rateNum = -rateNum
		}
		if rateDenom < 0 {
			rateDenom = -rateDenom
		}

		// Skip extreme rates
		ratio := float64(rateNum) / float64(rateDenom)
		if ratio > 1000 || ratio < 0.001 {
			return true
		}

		amountDec := decimal.NewFromInt(amount)
		rate := decimal.NewFromInt(rateNum).Div(decimal.NewFromInt(rateDenom))
		inverseRate := decimal.NewFromInt(rateDenom).Div(decimal.NewFromInt(rateNum))

		// Forward conversion: amount * rate
		converted := amountDec.Mul(rate)

		// Reverse conversion: converted * inverseRate
		roundtrip := converted.Mul(inverseRate)

		// Property: roundtrip should be close to original
		diff := roundtrip.Sub(amountDec).Abs()
		tolerance := amountDec.Abs().Mul(decimal.NewFromFloat(0.0001)) // 0.01% tolerance
		if tolerance.LessThan(decimal.NewFromFloat(0.0001)) {
			tolerance = decimal.NewFromFloat(0.0001)
		}

		if diff.GreaterThan(tolerance) {
			t.Logf("Inverse roundtrip exceeded tolerance: amount=%d rate=%d/%d diff=%s tolerance=%s",
				amount, rateNum, rateDenom, diff.String(), tolerance.String())
			return false
		}

		return true
	}

	cfg := &quick.Config{MaxCount: 1000}
	if err := quick.Check(f, cfg); err != nil {
		t.Fatalf("Asset rate inverse roundtrip property failed: %v", err)
	}
}

// Property: Rate conversion is associative when amounts are multiplied
// (a * rate1) * rate2 == a * (rate1 * rate2)
func TestProperty_AssetRateAssociative(t *testing.T) {
	f := func(amount, rate1Num, rate1Denom, rate2Num, rate2Denom int64) bool {
		// Skip invalid cases
		if rate1Denom == 0 || rate2Denom == 0 {
			return true
		}

		// Constrain values
		if amount < 0 {
			amount = -amount
		}
		if rate1Num == 0 || rate2Num == 0 {
			return true
		}

		// Avoid extreme values that could cause overflow
		if math.Abs(float64(rate1Num)/float64(rate1Denom)) > 100 ||
			math.Abs(float64(rate2Num)/float64(rate2Denom)) > 100 {
			return true
		}

		amountDec := decimal.NewFromInt(amount)
		rate1 := decimal.NewFromInt(rate1Num).Div(decimal.NewFromInt(rate1Denom))
		rate2 := decimal.NewFromInt(rate2Num).Div(decimal.NewFromInt(rate2Denom))

		// (a * rate1) * rate2
		leftSide := amountDec.Mul(rate1).Mul(rate2)

		// a * (rate1 * rate2)
		combinedRate := rate1.Mul(rate2)
		rightSide := amountDec.Mul(combinedRate)

		// Property: both should be equal
		diff := leftSide.Sub(rightSide).Abs()
		tolerance := decimal.NewFromFloat(0.0001)

		if diff.GreaterThan(tolerance) {
			t.Logf("Associativity violated: left=%s right=%s diff=%s",
				leftSide.String(), rightSide.String(), diff.String())
			return false
		}

		return true
	}

	cfg := &quick.Config{MaxCount: 1000}
	if err := quick.Check(f, cfg); err != nil {
		t.Fatalf("Asset rate associativity property failed: %v", err)
	}
}

// Property: Rate of 1 is identity (amount * 1 == amount)
func TestProperty_AssetRateIdentity(t *testing.T) {
	f := func(amount int64) bool {
		amountDec := decimal.NewFromInt(amount)
		identityRate := decimal.NewFromInt(1)

		result := amountDec.Mul(identityRate)

		if !result.Equal(amountDec) {
			t.Logf("Identity rate violated: amount=%s result=%s", amountDec.String(), result.String())
			return false
		}

		return true
	}

	cfg := &quick.Config{MaxCount: 1000}
	if err := quick.Check(f, cfg); err != nil {
		t.Fatalf("Asset rate identity property failed: %v", err)
	}
}

// Property: Conversion preserves sign
func TestProperty_AssetRateSignPreservation(t *testing.T) {
	f := func(amount, rateNum, rateDenom int64) bool {
		if rateDenom == 0 || rateNum == 0 {
			return true
		}

		// Make rate always positive for this test
		if rateNum < 0 {
			rateNum = -rateNum
		}
		if rateDenom < 0 {
			rateDenom = -rateDenom
		}

		amountDec := decimal.NewFromInt(amount)
		rate := decimal.NewFromInt(rateNum).Div(decimal.NewFromInt(rateDenom))
		result := amountDec.Mul(rate)

		// Property: sign of result should match sign of amount (since rate is positive)
		if amount > 0 && !result.IsPositive() {
			t.Logf("Sign not preserved (positive): amount=%d rate=%s result=%s",
				amount, rate.String(), result.String())
			return false
		}
		if amount < 0 && !result.IsNegative() {
			t.Logf("Sign not preserved (negative): amount=%d rate=%s result=%s",
				amount, rate.String(), result.String())
			return false
		}
		if amount == 0 && !result.IsZero() {
			t.Logf("Zero not preserved: amount=%d rate=%s result=%s",
				amount, rate.String(), result.String())
			return false
		}

		return true
	}

	cfg := &quick.Config{MaxCount: 1000}
	if err := quick.Check(f, cfg); err != nil {
		t.Fatalf("Asset rate sign preservation property failed: %v", err)
	}
}
