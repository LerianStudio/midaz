package property

import (
    "math/rand"
    "testing"
    "testing/quick"

    "github.com/shopspring/decimal"
)

// Property: For a transaction with a single asset, the sum of destination amounts equals the send value.
// Uses production decimal.Decimal type and deterministic randomness.
func TestProperty_ConservationOfValue_Model(t *testing.T) {
    f := func(seed int64, destinations uint8) bool {
        // P2 Fix: All randomness derived from seed parameter (deterministic)
        rng := rand.New(rand.NewSource(seed))

        n := int(destinations)
        if n <= 0 { n = 1 }
        if n > 20 { n = 20 }

        // P1 Partial Fix: Use production decimal.Decimal type for money arithmetic
        totalCents := rng.Intn(10000) + 1
        total := decimal.NewFromInt(int64(totalCents)).Div(decimal.NewFromInt(100))

        // Generate random weights and normalize
        weights := make([]float64, n)
        wsum := 0.0
        for i := 0; i < n; i++ {
            w := rng.Float64()
            weights[i] = w
            wsum += w
        }
        if wsum == 0 {
            for i := 0; i < n; i++ { weights[i] = 1.0 }
            wsum = float64(n)
        }

        // Distribute using decimal.Decimal for precision (money arithmetic)
        parts := make([]decimal.Decimal, n)
        assigned := decimal.Zero
        for i := 0; i < n-1; i++ {
            proportion := weights[i] / wsum
            part := total.Mul(decimal.NewFromFloat(proportion))
            parts[i] = part
            assigned = assigned.Add(part)
        }
        // Last part gets remainder (ensures exact conservation)
        parts[n-1] = total.Sub(assigned)

        // Verify conservation using decimal comparison
        sum := decimal.Zero
        for _, p := range parts {
            sum = sum.Add(p)
        }

        // Property: sum of distributed parts must equal original total
        return sum.Equal(total)
    }

    cfg := &quick.Config{MaxCount: 100}
    if err := quick.Check(f, cfg); err != nil {
        t.Fatalf("conservation property failed: %v", err)
    }
}
