package property

import (
    "math/rand"
    "testing"
    "testing/quick"

    "github.com/shopspring/decimal"
)

// Property: Starting from zero, applying only non-negative inflows and outflows
// whose amounts never exceed current balance, the balance never becomes negative.
// Uses production decimal.Decimal type and deterministic randomness.
func TestProperty_NonNegativeBalance_Model(t *testing.T) {
    f := func(seed int64, operations uint16) bool {
        // P2 Fix: All randomness derived from seed parameter (deterministic)
        rng := rand.New(rand.NewSource(seed))

        steps := int(operations)
        if steps <= 0 { steps = 1 }
        if steps > 1000 { steps = 1000 }

        // P1 Partial Fix: Use production decimal.Decimal for balance arithmetic
        bal := decimal.Zero

        for i := 0; i < steps; i++ {
            if rng.Intn(2) == 0 { // inflow
                inflowCents := rng.Intn(1000)
                inflow := decimal.NewFromInt(int64(inflowCents))
                bal = bal.Add(inflow)
            } else { // outflow
                if bal.IsZero() { continue }
                // Random outflow up to current balance
                maxOutflow := bal.IntPart()
                if maxOutflow > 1000 { maxOutflow = 1000 } // cap for performance
                outflowCents := rng.Int63n(maxOutflow + 1)
                outflow := decimal.NewFromInt(outflowCents)
                bal = bal.Sub(outflow)
            }

            // Property: balance must never be negative
            if bal.LessThan(decimal.Zero) {
                return false
            }
        }
        return true
    }

    cfg := &quick.Config{MaxCount: 200}
    if err := quick.Check(f, cfg); err != nil {
        t.Fatalf("non-negative property failed: %v", err)
    }
}
