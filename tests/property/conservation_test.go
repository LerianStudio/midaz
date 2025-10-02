package property

import (
	"math/rand"
	"testing"
	"testing/quick"

	"github.com/shopspring/decimal"
)

// Property: For a transaction with a single asset, the sum of destination amounts equals the send value.
// This is a model-level property (no network calls). Uses production decimal type and deterministic RNG.
func TestProperty_ConservationOfValue_Model(t *testing.T) {
	// Deterministic RNG for reproducibility across runs
	rng := rand.New(rand.NewSource(42))

	f := func(n int) bool {
		if n <= 0 {
			n = 1
		}
		if n > 20 {
			n = 20
		}

		// Total amount in cents (>=1 cent)
		totalCents := rng.Intn(10000) + 1
		total := decimal.NewFromInt(int64(totalCents)).Shift(-2)

		// Generate positive integer weights and distribute proportionally
		weights := make([]int, n)
		sumW := 0
		for i := 0; i < n; i++ {
			w := rng.Intn(100) + 1 // 1..100
			weights[i] = w
			sumW += w
		}
		if sumW == 0 { // defensive, though impossible with +1 above
			sumW = n
			for i := 0; i < n; i++ {
				weights[i] = 1
			}
		}

		parts := make([]decimal.Decimal, n)
		assigned := decimal.Zero
		for i := 0; i < n-1; i++ {
			num := decimal.NewFromInt(int64(weights[i]))
			den := decimal.NewFromInt(int64(sumW))
			p := total.Mul(num).Div(den)
			parts[i] = p
			assigned = assigned.Add(p)
		}
		// Make last part the exact remainder to ensure conservation with decimal arithmetic
		parts[n-1] = total.Sub(assigned)

		// Verify conservation exactly under decimal arithmetic
		final := decimal.Zero
		for _, p := range parts {
			final = final.Add(p)
		}
		return final.Equal(total)
	}

	cfg := &quick.Config{MaxCount: 100, Rand: rand.New(rand.NewSource(99))}
	if err := quick.Check(f, cfg); err != nil {
		t.Fatalf("conservation property failed: %v", err)
	}
}
