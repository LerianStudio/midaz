package property

import (
	"math/rand"
	"testing"
	"testing/quick"

	"github.com/shopspring/decimal"
)

// Property: Sum of percentage shares in a distribution cannot exceed 100%
func TestProperty_ShareSumNotExceed100_Model(t *testing.T) {
	f := func(seed int64, shareCount uint8) bool {
		rng := rand.New(rand.NewSource(seed))

		// Limit share count to reasonable number
		count := int(shareCount % 10)
		if count == 0 {
			count = 1
		}

		// Generate random shares that should sum to <= 100
		shares := make([]decimal.Decimal, count)
		remaining := decimal.NewFromInt(100)

		for i := 0; i < count-1; i++ {
			// Each share is a random portion of remaining
			maxShare := remaining.Div(decimal.NewFromInt(int64(count - i)))
			sharePercent := rng.Float64() * maxShare.InexactFloat64()
			shares[i] = decimal.NewFromFloat(sharePercent).Round(2)
			remaining = remaining.Sub(shares[i])
		}
		// Last share gets the remainder
		shares[count-1] = remaining

		// Calculate total
		total := decimal.Zero
		for _, s := range shares {
			total = total.Add(s)
		}

		// Property: total should be <= 100 and >= 0
		hundred := decimal.NewFromInt(100)
		if total.GreaterThan(hundred) {
			t.Logf("Shares exceed 100%%: total=%s", total)
			return false
		}

		if total.LessThan(decimal.Zero) {
			t.Logf("Shares are negative: total=%s", total)
			return false
		}

		return true
	}

	cfg := &quick.Config{MaxCount: 500}
	if err := quick.Check(f, cfg); err != nil {
		t.Fatalf("Share sum not exceed 100 property failed: %v", err)
	}
}

// Property: When distributing with :remaining, all value is accounted for
func TestProperty_RemainderDistribution_Model(t *testing.T) {
	f := func(total int64, fixedCount uint8) bool {
		// Constrain inputs
		if total <= 0 {
			total = 100
		}
		if total > 1_000_000 {
			total = 1_000_000
		}

		count := int(fixedCount%5) + 1 // 1-5 recipients

		totalDec := decimal.NewFromInt(total)
		fixedAmounts := make([]decimal.Decimal, count-1)
		fixedSum := decimal.Zero

		// Generate fixed amounts that don't exceed total
		for i := 0; i < count-1; i++ {
			maxFixed := totalDec.Sub(fixedSum).Div(decimal.NewFromInt(int64(count - i)))
			fixedAmounts[i] = maxFixed.Mul(decimal.NewFromFloat(0.5)).Round(2)
			fixedSum = fixedSum.Add(fixedAmounts[i])
		}

		// Remainder should get the rest
		remainder := totalDec.Sub(fixedSum)

		// Property: fixed + remainder == total
		distributedTotal := fixedSum.Add(remainder)
		if !distributedTotal.Equal(totalDec) {
			t.Logf("Distribution mismatch: fixed=%s remainder=%s total=%s expected=%s",
				fixedSum, remainder, distributedTotal, totalDec)
			return false
		}

		// Property: remainder should be non-negative
		if remainder.LessThan(decimal.Zero) {
			t.Logf("Negative remainder: %s", remainder)
			return false
		}

		return true
	}

	cfg := &quick.Config{MaxCount: 500}
	if err := quick.Check(f, cfg); err != nil {
		t.Fatalf("Remainder distribution property failed: %v", err)
	}
}

// Property: Percentage-based distribution preserves total value
func TestProperty_PercentageDistributionPreservesTotal_Model(t *testing.T) {
	f := func(total int64, p1, p2, p3 uint8) bool {
		// Constrain to reasonable financial values
		if total <= 0 {
			total = 1000
		}
		if total > 1_000_000_000 {
			total = 1_000_000_000
		}

		totalDec := decimal.NewFromInt(total)

		// Convert to percentages (0-100 range)
		pct1 := decimal.NewFromInt(int64(p1 % 101))
		pct2 := decimal.NewFromInt(int64(p2 % 101))
		pct3 := decimal.NewFromInt(int64(p3 % 101))

		pctSum := pct1.Add(pct2).Add(pct3)
		if pctSum.IsZero() {
			return true // Skip zero case
		}

		// Normalize to 100%
		hundred := decimal.NewFromInt(100)
		pct1 = pct1.Div(pctSum).Mul(hundred)
		pct2 = pct2.Div(pctSum).Mul(hundred)
		pct3 = pct3.Div(pctSum).Mul(hundred)

		// Calculate amounts
		amt1 := totalDec.Mul(pct1).Div(hundred).Round(2)
		amt2 := totalDec.Mul(pct2).Div(hundred).Round(2)
		amt3 := totalDec.Mul(pct3).Div(hundred).Round(2)

		distributed := amt1.Add(amt2).Add(amt3)

		// Property: distributed should be very close to total (allowing for rounding)
		diff := distributed.Sub(totalDec).Abs()
		tolerance := decimal.NewFromFloat(0.03) // 3 cents tolerance for rounding

		if diff.GreaterThan(tolerance) {
			t.Logf("Distribution error too large: total=%s distributed=%s diff=%s",
				totalDec, distributed, diff)
			return false
		}

		return true
	}

	cfg := &quick.Config{MaxCount: 500}
	if err := quick.Check(f, cfg); err != nil {
		t.Fatalf("Percentage distribution preserves total property failed: %v", err)
	}
}
