// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package readyz

import (
	"encoding/json"
	"net/http"
)

// NewNetHTTPHandler returns a net/http handler that implements the canonical
// /readyz contract. It is the bare-stdlib counterpart to NewHandler (Fiber)
// and shares the same runAll fan-out and aggregation rule, so a service that
// uses both transports produces identical /readyz semantics.
//
// The Worker component uses this variant because it runs a minimal HTTP
// server alongside its RabbitMQ consumer and does not pull in Fiber.
//
// Parameters mirror NewHandler exactly, including the metrics emitter.
// See NewHandler for documentation on the draining short-circuit, the
// no-caching guarantee, and the nil-metrics tolerance.
func NewNetHTTPHandler(checkers []Checker, drainState *DrainState, version, deploymentMode string, metrics *Metrics) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		if drainState != nil && drainState.IsDraining() {
			writeJSON(w, http.StatusServiceUnavailable, Response{
				Status: "draining",
				Reason: drainReason,
				// Always include an empty (but non-nil) Checks map so
				// dashboards keying on `checks` keep working during
				// drain. Same rationale as the Fiber handler — keeps
				// the wire format consistent across transports.
				Checks:         map[string]DependencyCheck{},
				Version:        version,
				DeploymentMode: deploymentMode,
			})

			return
		}

		ctx := r.Context()
		results := runAll(ctx, checkers)
		emitCheckResults(ctx, metrics, results)
		topStatus, code := Aggregate(results)

		writeJSON(w, code, Response{
			Status:         topStatus,
			Checks:         results,
			Version:        version,
			DeploymentMode: deploymentMode,
		})
	}
}

// writeJSON serializes a Response and writes it to w with the given status
// code. Encoding errors should never occur for the Response type (no
// non-JSON-encodable fields), but if they do they are silently dropped:
// the response status has already been set and there is no caller to
// surface the error to.
func writeJSON(w http.ResponseWriter, code int, body Response) {
	w.WriteHeader(code)

	if err := json.NewEncoder(w).Encode(body); err != nil {
		// Best-effort: write a schema-compliant Response payload. A
		// well-formed Response should not fail to encode, so the only
		// realistic cause here is a closed connection — in which case
		// Write() will also fail and there's nothing else we can do. The
		// payload mirrors the canonical Response shape (status + checks +
		// version + deployment_mode) so consumers parsing /readyz output
		// don't see an unexpected schema variant during this rare failure.
		_, _ = w.Write([]byte(`{"status":"unhealthy","checks":{},"version":"","deployment_mode":""}`))
	}
}
