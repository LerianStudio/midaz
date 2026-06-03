// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package readyz

import "net/http"

// healthyStatuses is the closed set of per-dep statuses that count toward
// "healthy" at the top level. Anything outside this set yields a 503 — the
// aggregator fails closed so typos and future-but-unknown values surface
// the bug instead of being silently treated as healthy.
var healthyStatuses = map[Status]struct{}{
	StatusUp:      {},
	StatusSkipped: {},
	StatusNA:      {},
}

// Aggregate applies the canonical /readyz aggregation rule to a map of
// dependency check results and returns:
//
//   - the top-level status string ("healthy" or "unhealthy")
//   - the corresponding HTTP status code (200 or 503)
//
// The rule (see SKILL.md):
//
//	"Top-level status is 'healthy' if and only if every check has status in
//	{up, skipped, n/a}. ANY check whose status is NOT in that set yields
//	top-level 'unhealthy' and HTTP 503."
//
// Fail-closed semantics (Dispatch 2 LOW-2 promoted): the aggregator does
// NOT special-case {down, degraded} — it whitelists {up, skipped, n/a}
// and treats everything else as unhealthy. This way:
//   - typos like "OK" instead of "up" → 503 (visible bug)
//   - empty status string → 503 (visible bug)
//   - future hypothetical values like "degraded-recovering" → 503 until
//     the closed vocabulary is extended deliberately
//
// An empty map is considered healthy: a service with no declared
// dependencies has nothing that could be unhealthy.
//
// This function is pure (no I/O, no goroutines) and safe to call from any
// goroutine.
func Aggregate(checks map[string]DependencyCheck) (string, int) {
	for _, c := range checks {
		if _, ok := healthyStatuses[c.Status]; !ok {
			return "unhealthy", http.StatusServiceUnavailable
		}
	}

	return "healthy", http.StatusOK
}
