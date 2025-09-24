package property

import (
    "math/rand"
    "testing"
    h "github.com/LerianStudio/midaz/v3/tests/helpers"
)

// Property: Given an initial balance B0 and a multiset of deltas that never cause
// the balance to go negative when applied cumulatively, the final balance equals
// B0 + sum(deltas) independent of application order.
func TestProperty_OrderInvariance_ForSafeDeltas(t *testing.T) {
    f := func(n int) bool {
        if n <= 0 { n = 1 }
        if n > 64 { n = 64 }
        // initial balance between 0 and 10,000
        B0 := rand.Intn(10001)
        // create non-negative inflows and constrained outflows such that sum(out) <= B0
        inflows := make([]int, n)
        outflows := make([]int, n)
        for i := 0; i < n; i++ { inflows[i] = rand.Intn(200) }
        // cap total outflows to not exceed B0 to guarantee no-negative regardless of order
        remaining := B0
        for i := 0; i < n; i++ {
            // draw up to remaining/(n-i) to reduce chance of late negatives
            capPer := 0
            if n-i > 0 { capPer = remaining / (n - i) }
            if capPer < 0 { capPer = 0 }
            v := 0
            if capPer > 0 { v = rand.Intn(capPer+1) }
            outflows[i] = v
            remaining -= v
        }
        // build combined deltas: inflows positive, outflows negative
        deltas := make([]int, 0, 2*n)
        for _, v := range inflows { deltas = append(deltas, v) }
        for _, v := range outflows { deltas = append(deltas, -v) }

        // compute final ignoring order: B0 + sum(deltas)
        sum := 0
        for _, d := range deltas { sum += d }
        want := B0 + sum

        // random permutation application should never go negative and end at want
        // we retry a few random orders to strengthen property
        for tries := 0; tries < 5; tries++ {
            // shuffle
            for i := range deltas { j := rand.Intn(i+1); deltas[i], deltas[j] = deltas[j], deltas[i] }
            bal := B0
            for _, d := range deltas {
                bal += d
                if bal < 0 { return false }
            }
            if bal != want { return false }
        }
        return true
    }
    h.CheckProp(t, f)
}
