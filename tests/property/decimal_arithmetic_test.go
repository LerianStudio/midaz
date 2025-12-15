package property

import (
	"math"
	"testing"
	"testing/quick"

	"github.com/shopspring/decimal"
)

// Property: Decimal addition is commutative: a + b == b + a
func TestProperty_DecimalAdditionCommutative(t *testing.T) {
	f := func(a, b int64) bool {
		da := decimal.NewFromInt(a)
		db := decimal.NewFromInt(b)

		sum1 := da.Add(db)
		sum2 := db.Add(da)

		return sum1.Equal(sum2)
	}

	if err := quick.Check(f, nil); err != nil {
		t.Fatalf("decimal addition commutativity failed: %v", err)
	}
}

// Property: Decimal addition is associative: (a + b) + c == a + (b + c)
func TestProperty_DecimalAdditionAssociative(t *testing.T) {
	f := func(a, b, c int64) bool {
		da := decimal.NewFromInt(a)
		db := decimal.NewFromInt(b)
		dc := decimal.NewFromInt(c)

		// (a + b) + c
		sum1 := da.Add(db).Add(dc)
		// a + (b + c)
		sum2 := da.Add(db.Add(dc))

		return sum1.Equal(sum2)
	}

	if err := quick.Check(f, nil); err != nil {
		t.Fatalf("decimal addition associativity failed: %v", err)
	}
}

// Property: Zero is identity for addition: a + 0 == a
func TestProperty_DecimalAdditionIdentity(t *testing.T) {
	f := func(a int64) bool {
		da := decimal.NewFromInt(a)
		zero := decimal.Zero

		return da.Add(zero).Equal(da)
	}

	if err := quick.Check(f, nil); err != nil {
		t.Fatalf("decimal addition identity failed: %v", err)
	}
}

// Property: Decimal subtraction inverse: a - a == 0
func TestProperty_DecimalSubtractionInverse(t *testing.T) {
	f := func(a int64) bool {
		da := decimal.NewFromInt(a)

		return da.Sub(da).Equal(decimal.Zero)
	}

	if err := quick.Check(f, nil); err != nil {
		t.Fatalf("decimal subtraction inverse failed: %v", err)
	}
}

// Property: Decimal multiplication is commutative: a * b == b * a
func TestProperty_DecimalMultiplicationCommutative(t *testing.T) {
	f := func(a, b int64) bool {
		da := decimal.NewFromInt(a)
		db := decimal.NewFromInt(b)

		prod1 := da.Mul(db)
		prod2 := db.Mul(da)

		return prod1.Equal(prod2)
	}

	if err := quick.Check(f, nil); err != nil {
		t.Fatalf("decimal multiplication commutativity failed: %v", err)
	}
}

// Property: One is identity for multiplication: a * 1 == a
func TestProperty_DecimalMultiplicationIdentity(t *testing.T) {
	f := func(a int64) bool {
		da := decimal.NewFromInt(a)
		one := decimal.NewFromInt(1)

		return da.Mul(one).Equal(da)
	}

	if err := quick.Check(f, nil); err != nil {
		t.Fatalf("decimal multiplication identity failed: %v", err)
	}
}

// Property: Zero absorbs multiplication: a * 0 == 0
func TestProperty_DecimalMultiplicationZero(t *testing.T) {
	f := func(a int64) bool {
		da := decimal.NewFromInt(a)

		return da.Mul(decimal.Zero).Equal(decimal.Zero)
	}

	if err := quick.Check(f, nil); err != nil {
		t.Fatalf("decimal multiplication zero failed: %v", err)
	}
}

// Property: Decimal division inverse: (a * b) / b == a (when b != 0)
func TestProperty_DecimalDivisionInverse(t *testing.T) {
	f := func(a, b int64) bool {
		if b == 0 {
			return true // Skip division by zero
		}

		da := decimal.NewFromInt(a)
		db := decimal.NewFromInt(b)

		// (a * b) / b should equal a
		product := da.Mul(db)
		quotient := product.Div(db)

		return quotient.Equal(da)
	}

	if err := quick.Check(f, nil); err != nil {
		t.Fatalf("decimal division inverse failed: %v", err)
	}
}

// Property: Balance cannot go negative for certain account types
// This is a business rule property, not a mathematical one
func TestProperty_BalanceNonNegative(t *testing.T) {
	f := func(initial int64, operations []int64) bool {
		// Simulate balance with non-negative constraint
		if initial < 0 {
			initial = 0
		}

		balance := decimal.NewFromInt(initial)

		for _, op := range operations {
			opDec := decimal.NewFromInt(op)

			if op < 0 {
				// Debit - check if allowed
				newBalance := balance.Add(opDec) // op is negative, so this subtracts
				if newBalance.IsNegative() {
					// Should be rejected - balance would go negative
					continue
				}
				balance = newBalance
			} else {
				// Credit - always allowed
				balance = balance.Add(opDec)
			}
		}

		// Final balance should never be negative
		return !balance.IsNegative()
	}

	cfg := &quick.Config{MaxCount: 1000}
	if err := quick.Check(f, cfg); err != nil {
		t.Fatalf("balance non-negative property failed: %v", err)
	}
}

// Property: Share calculation totals 100% (or specified total)
func TestProperty_ShareCalculationTotal(t *testing.T) {
	f := func(shares []uint8) bool {
		if len(shares) == 0 {
			return true
		}

		// Convert to decimals and normalize to 100%
		total := decimal.Zero
		shareDecimals := make([]decimal.Decimal, len(shares))

		for i, s := range shares {
			shareDecimals[i] = decimal.NewFromInt(int64(s))
			total = total.Add(shareDecimals[i])
		}

		if total.IsZero() {
			return true // All zero shares
		}

		// Calculate percentages
		hundred := decimal.NewFromInt(100)
		calculatedTotal := decimal.Zero

		for _, s := range shareDecimals {
			percentage := s.Div(total).Mul(hundred)
			calculatedTotal = calculatedTotal.Add(percentage)
		}

		// Total should be 100 (within rounding tolerance)
		diff := calculatedTotal.Sub(hundred).Abs()
		tolerance := decimal.NewFromFloat(0.01)

		return diff.LessThanOrEqual(tolerance)
	}

	cfg := &quick.Config{MaxCount: 1000}
	if err := quick.Check(f, cfg); err != nil {
		t.Fatalf("share calculation total property failed: %v", err)
	}
}

// Property: Exchange rate conversion roundtrip preserves value (within tolerance)
func TestProperty_ExchangeRateRoundtrip(t *testing.T) {
	f := func(amount int64, rateNum, rateDenom int64) bool {
		if rateDenom == 0 || rateNum == 0 {
			return true // Skip invalid rates
		}

		// Ensure reasonable rate bounds
		if math.Abs(float64(rateNum)/float64(rateDenom)) > 1000 || math.Abs(float64(rateNum)/float64(rateDenom)) < 0.001 {
			return true // Skip extreme rates
		}

		original := decimal.NewFromInt(amount)
		rate := decimal.NewFromInt(rateNum).Div(decimal.NewFromInt(rateDenom))
		inverseRate := decimal.NewFromInt(rateDenom).Div(decimal.NewFromInt(rateNum))

		// Convert: original * rate * inverseRate should ≈ original
		converted := original.Mul(rate)
		roundtrip := converted.Mul(inverseRate)

		// Check within tolerance (small error due to division)
		diff := roundtrip.Sub(original).Abs()
		tolerance := original.Abs().Mul(decimal.NewFromFloat(0.0001)) // 0.01% tolerance
		if tolerance.IsZero() {
			tolerance = decimal.NewFromFloat(0.0001)
		}

		return diff.LessThanOrEqual(tolerance)
	}

	cfg := &quick.Config{MaxCount: 1000}
	if err := quick.Check(f, cfg); err != nil {
		t.Fatalf("exchange rate roundtrip property failed: %v", err)
	}
}
