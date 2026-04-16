// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package dbpool

import (
	"fmt"
	"strings"
)

// DefaultBudgetRatio is the fraction of PostgreSQL max_connections that
// the aggregate Midaz pool budget is allowed to consume. The remaining
// headroom (20% by default) covers ad-hoc DBA sessions, PgBouncer's own
// server-side pool, and replication slots.
const DefaultBudgetRatio = 0.8

// ValidatePoolBudget returns ErrPoolBudgetExceeded when the sum of
// (pool.MaxConns * pool.ExpectedInstances) across all pools would exceed
// maxConnections * ratio.
//
// Passing ratio <= 0 substitutes DefaultBudgetRatio. Pools with
// non-positive MaxConns or ExpectedInstances are skipped (they
// contribute 0 to the budget).
//
// The error message lists each pool's contribution so operators can
// identify which service to downscale.
func ValidatePoolBudget(maxConnections int, ratio float64, pools []PoolBudget) error {
	if maxConnections <= 0 {
		// Cannot validate without knowing the server ceiling. Callers
		// that care about fail-closed behaviour should surface this as
		// a configuration error; returning nil here preserves backward
		// compatibility for test environments that don't set the env var.
		return nil
	}

	if ratio <= 0 {
		ratio = DefaultBudgetRatio
	}

	budget := float64(maxConnections) * ratio

	var total int64

	for _, p := range pools {
		if p.MaxConns <= 0 || p.ExpectedInstances <= 0 {
			continue
		}

		total += int64(p.MaxConns) * int64(p.ExpectedInstances)
	}

	if float64(total) <= budget {
		return nil
	}

	return fmt.Errorf(
		"%w: total=%d budget=%.0f (max_connections=%d ratio=%.2f) pools=%s",
		ErrPoolBudgetExceeded,
		total,
		budget,
		maxConnections,
		ratio,
		describePools(pools),
	)
}

func describePools(pools []PoolBudget) string {
	if len(pools) == 0 {
		return "[]"
	}

	var b strings.Builder

	b.WriteByte('[')

	for i, p := range pools {
		if i > 0 {
			b.WriteString(", ")
		}

		fmt.Fprintf(&b, "%s(max=%d x %d)", p.Name, p.MaxConns, p.ExpectedInstances)
	}

	b.WriteByte(']')

	return b.String()
}
