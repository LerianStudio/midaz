package property

import (
    "math/rand"
    "testing"
    "github.com/shopspring/decimal"
    h "github.com/LerianStudio/midaz/v3/tests/helpers"
)

// Property: Cross-rate consistency. For rates A->B and B->C, the implied A->C should be close to A->B * B->C.
// With rounding to cents at each conversion, bound the error per leg.
func TestProperty_Rate_CrossConsistency_Model(t *testing.T) {
    f := func() bool {
        // random base amount between 0.01 and 1000.00
        cents := rand.Intn(100000-1) + 1
        base := decimal.NewFromInt(int64(cents)).Div(decimal.NewFromInt(100))
        ab := decimal.NewFromFloat(0.01 + 4.99*rand.Float64())
        bc := decimal.NewFromFloat(0.01 + 4.99*rand.Float64())
        acImplied := ab.Mul(bc)

        round2 := func(d decimal.Decimal) decimal.Decimal { return d.Round(2) }

        // Convert A->B->C and A->C directly, both with rounding each step
        bAmt := round2(base.Mul(ab))
        cAmtViaB := round2(bAmt.Mul(bc))
        cAmtDirect := round2(base.Mul(acImplied))

        diff := cAmtViaB.Sub(cAmtDirect).Abs()
        // Bound: at most two rounding steps of 0.01
        return diff.LessThanOrEqual(decimal.NewFromFloat(0.02))
    }
    h.CheckProp(t, f)
}

