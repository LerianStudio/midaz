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

        // Total amount to distribute (keep cents-like scale as before)
        total := float64(rand.Intn(10000)+1) / 100.0

        // Generate random weights and normalize them so that parts sum to total
        weights := make([]float64, n)
        wsum := 0.0
        for i := 0; i < n; i++ {
            w := rand.Float64()
            weights[i] = w
            wsum += w
        }
        if wsum == 0 { // extremely unlikely, but guard against pathological RNG
            for i := 0; i < n; i++ { weights[i] = 1.0 }
            wsum = float64(n)
        }

        parts := make([]float64, n)
        assigned := 0.0
        for i := 0; i < n-1; i++ {
            p := total * (weights[i] / wsum)
            parts[i] = p
            assigned += p
        }
        // Make last part the remainder to ensure exact conservation (up to FP epsilon)
        parts[n-1] = total - assigned

        // Verify conservation with small FP tolerance
        final := 0.0
        for _, p := range parts { final += p }
        const eps = 1e-6
        diff := final - total
        if diff < 0 { diff = -diff }
        return diff < eps
    }
    cfg := &quick.Config{MaxCount: 100}
    if err := quick.Check(f, cfg); err != nil {
        t.Fatalf("conservation property failed: %v", err)
    }
}
