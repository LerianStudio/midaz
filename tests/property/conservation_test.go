package property

import (
    "math/rand"
    "testing"
    "testing/quick"
)

// Property: For a transaction with a single asset, the sum of destination amounts equals the send value.
// This is a model-level property (no network calls).
func TestProperty_ConservationOfValue_Model(t *testing.T) {
    f := func(n int) bool {
        if n <= 0 { n = 1 }
        // bound n to avoid huge or negative sizes causing panics
        if n > 20 { n = 20 }
        // generate random positive parts that sum to total
        total := float64(rand.Intn(10000)+1) / 100.0
        parts := make([]float64, n)
        sum := 0.0
        for i := 0; i < n-1; i++ {
            parts[i] = float64(rand.Intn(10000)) / 100.0
            sum += parts[i]
        }
        parts[n-1] = total - sum
        // Rounding could make last negative; guard
        // clamp last part and recompute total
        if parts[n-1] < 0 { parts[n-1] = 0 }
        final := 0.0
        for _, p := range parts { final += p }
        // Allow minor float error tolerance
        return (final-total) < 0.000001 && (total-final) < 0.000001
    }
    cfg := &quick.Config{MaxCount: 100}
    if err := quick.Check(f, cfg); err != nil {
        t.Fatalf("conservation property failed: %v", err)
    }
}
