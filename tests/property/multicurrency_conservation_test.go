package property

import (
    "math/rand"
    "testing"

    "github.com/shopspring/decimal"
    h "github.com/LerianStudio/midaz/v3/tests/helpers"
)

// round2 rounds a decimal to 2 fractional digits using bankers rounding.
func round2(d decimal.Decimal) decimal.Decimal { return d.Round(2) }

// Property: Multi-currency conservation with rounding bounds.
// Split a base amount into N legs that sum exactly to the base. Convert each leg to a target
// currency (multiply by FX rate) and round to cents, then convert back (divide by rate) and round.
// The round-trip difference is bounded by at most 0.03 per leg (two rounds of 0.01 plus remainder effects).
func TestProperty_MultiCurrency_ConservationWithBounds_Model(t *testing.T) {
    f := func(n int) bool {
        if n <= 0 { n = 1 }
        if n > 12 { n = 12 }

        // Base amount between 0.01 and 10,000.00
        cents := rand.Intn(1_000_000-1) + 1
        base := decimal.NewFromInt(int64(cents)).Div(decimal.NewFromInt(100))

        // Random FX rates in (0.01, 5.00]
        rates := make([]decimal.Decimal, n)
        w := make([]decimal.Decimal, n)
        wsum := decimal.Zero
        for i := 0; i < n; i++ {
            r := decimal.NewFromFloat(0.01 + rand.Float64()*4.99)
            rates[i] = r
            w[i] = decimal.NewFromFloat(rand.Float64() + 1e-6) // avoid zeros
            wsum = wsum.Add(w[i])
        }

        // Allocate leg amounts in base currency that sum exactly to base
        legsBase := make([]decimal.Decimal, n)
        assigned := decimal.Zero
        for i := 0; i < n-1; i++ {
            p := base.Mul(w[i].Div(wsum))
            p = round2(p) // amounts commonly rounded at allocation time
            legsBase[i] = p
            assigned = assigned.Add(p)
        }
        legsBase[n-1] = base.Sub(assigned) // remainder to ensure exact sum

        // Convert each leg to target currency, then back to base
        backSum := decimal.Zero
        for i := 0; i < n; i++ {
            target := round2(legsBase[i].Mul(rates[i]))
            back := round2(target.Div(rates[i]))
            backSum = backSum.Add(back)
        }

        // Allow bounded error due to double rounding and remainder distribution
        // Bound: <= 0.03 per leg
        diff := backSum.Sub(base).Abs()
        bound := decimal.NewFromFloat(0.03 * float64(n))
        return diff.LessThanOrEqual(bound)
    }
    h.CheckProp(t, f)
}

