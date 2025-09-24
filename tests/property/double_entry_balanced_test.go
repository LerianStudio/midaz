package property

import (
    "math/rand"
    "testing"
    h "github.com/LerianStudio/midaz/v3/tests/helpers"
)

// Property: In a model of a double-entry transaction, the sum of debits equals the sum of credits,
// and both equal the transaction value. We simulate both sides (from/to) using the same total.
func TestProperty_DoubleEntryBalanced_Model(t *testing.T) {
    f := func(parts int) bool {
        if parts <= 0 { parts = 1 }
        if parts > 12 { parts = 12 }
        // choose a total between 0.01 and 1000.00 with 2 decimal precision
        cents := rand.Intn(100000-1) + 1 // [1, 99999]
        total := float64(cents) / 100.0

        // create random weights for debit and credit legs independently
        mkParts := func(n int, total float64) []float64 {
            w := make([]float64, n)
            sum := 0.0
            for i := 0; i < n; i++ { v := rand.Float64(); if v == 0 { v = 0.5 } ; w[i] = v; sum += v }
            out := make([]float64, n)
            assigned := 0.0
            for i := 0; i < n-1; i++ { p := total * (w[i]/sum); out[i] = p; assigned += p }
            out[n-1] = total - assigned
            return out
        }
        debits := mkParts(parts, total)
        credits := mkParts(parts, total)

        sum := func(xs []float64) float64 { s := 0.0; for _, v := range xs { s += v }; return s }
        const eps = 1e-6
        sDeb := sum(debits)
        sCred := sum(credits)
        if d := sDeb-total; d < -eps || d > eps { return false }
        if d := sCred-total; d < -eps || d > eps { return false }
        // and symmetric equality between legs
        if d := sDeb-sCred; d < -eps || d > eps { return false }
        return true
    }
    h.CheckProp(t, f)
}

