package property

import (
	"math/rand"
	"testing"
	"testing/quick"

	"github.com/shopspring/decimal"
)

// Property: Starting from zero, applying only non-negative inflows and outflows
// whose amounts never exceed current balance, the balance never becomes negative.
// Uses production decimal type and deterministic RNG.
func TestProperty_NonNegativeBalance_Model(t *testing.T) {
	rng := rand.New(rand.NewSource(4242))
	f := func(steps int) bool {
		if steps <= 0 {
			steps = 1
		}
		if steps > 1000 {
			steps = 1000
		}
		bal := decimal.Zero
		for i := 0; i < steps; i++ {
			if rng.Intn(2) == 0 { // inflow
				// 0..999 cents
				cents := rng.Intn(1000)
				bal = bal.Add(decimal.NewFromInt(int64(cents)).Shift(-2))
			} else { // outflow
				if bal.IsZero() {
					continue
				}
				// choose outflow in [0, bal] with cents granularity
				// draw k in [0, current cents]
				curCents := bal.Shift(2).Truncate(0)
				max := int(curCents.IntPart())
				outCents := 0
				if max > 0 {
					outCents = rng.Intn(max + 1)
				}
				out := decimal.NewFromInt(int64(outCents)).Shift(-2)
				bal = bal.Sub(out)
			}
			if bal.IsNegative() {
				return false
			}
		}
		return true
	}
	cfg := &quick.Config{MaxCount: 200, Rand: rand.New(rand.NewSource(77))}
	if err := quick.Check(f, cfg); err != nil {
		t.Fatalf("non-negative property failed: %v", err)
	}
}
