package property

import (
    "math/rand"
    "testing"
    "testing/quick"
)

// Property: Starting from zero, applying only non-negative inflows and outflows
// whose amounts never exceed current balance, the balance never becomes negative.
func TestProperty_NonNegativeBalance_Model(t *testing.T) {
    f := func(steps int) bool {
        if steps <= 0 { steps = 1 }
        bal := 0
        for i := 0; i < steps; i++ {
            if rand.Intn(2) == 0 { // inflow
                bal += rand.Intn(1000)
            } else { // outflow
                if bal == 0 { continue }
                out := rand.Intn(bal+1)
                bal -= out
            }
            if bal < 0 { return false }
        }
        return true
    }
    cfg := &quick.Config{MaxCount: 200}
    if err := quick.Check(f, cfg); err != nil {
        t.Fatalf("non-negative property failed: %v", err)
    }
}

