// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package readyz

import (
	"context"
	"fmt"
	"sync/atomic"

	"github.com/LerianStudio/lib-observability/log"
)

// SelfProbeState gates the /health endpoint. The flag starts false; only after
// RunSelfProbe successfully verifies all configured dependencies at startup
// does the bootstrap call MarkHealthy() to flip it.
//
// Lifecycle contract:
//   - On startup before RunSelfProbe runs → IsHealthy()=false, /health 503.
//   - On RunSelfProbe success                → MarkHealthy() flips to true,
//     /health 200.
//   - On RunSelfProbe failure                → flag stays false, /health 503.
//     The pod stays running so the container can flush logs and K8s
//     livenessProbe can restart it cleanly. We deliberately DO NOT call
//     os.Exit from probe code (anti-pattern #7 in dev-readyz/SKILL.md).
//
// One-way: there is no Reset. A pod that boots unhealthy stays unhealthy
// for its lifetime; K8s replaces it.
//
// Wraps atomic.Bool so concurrent readers (every /health request) and the
// single bootstrap writer race-free.
type SelfProbeState struct {
	ok atomic.Bool
}

// MarkHealthy flips the flag to true. Idempotent — repeated calls are no-ops.
// Safe to call from any goroutine.
func (s *SelfProbeState) MarkHealthy() {
	s.ok.Store(true)
}

// IsHealthy reports whether MarkHealthy has been called. Safe to call from
// any goroutine; reads use atomic.Bool.Load so they never race with
// MarkHealthy.
func (s *SelfProbeState) IsHealthy() bool {
	return s.ok.Load()
}

// RunSelfProbe runs each Checker.Check sequentially under the supplied ctx
// and reports whether every dependency was reachable. It is the canonical
// startup probe wired into both Manager and Worker bootstrap.
//
// Aggregation rule (mirrors aggregation.go for /readyz):
//   - up | skipped | n/a → counted as healthy.
//   - down | degraded   → counted as unhealthy → returns error.
//
// For every dep, RunSelfProbe:
//
//  1. Calls checker.Check(ctx) once.
//  2. Emits selfprobe_result via metrics.EmitSelfProbeResult (1 healthy, 0
//     unhealthy). nil-Metrics is tolerated by EmitSelfProbeResult.
//  3. Logs a structured "self_probe_check" event with dep name, status, and
//     either latency_ms (on success) or error (on failure).
//
// At the boundaries it logs "startup_self_probe_started" (with dep_count) and
// either "startup_self_probe_passed" or "startup_self_probe_failed".
//
// On failure the returned error wraps the first failing dep's error so the
// caller can surface a single root cause; subsequent failures are still
// logged and metricized but do not contribute to the wrapped error chain.
//
// The caller is responsible for what happens next:
//   - Success: call state.MarkHealthy() so /health flips to 200.
//   - Failure: leave the state unhealthy and let K8s livenessProbe restart
//     the pod. DO NOT call os.Exit here — that breaks log collection.
//
// Sequential rather than parallel by design: the /readyz handler runs probes
// in parallel because it is on the request path; RunSelfProbe runs once at
// startup, where determinism (logs in a stable order) outweighs latency.
func RunSelfProbe(ctx context.Context, checkers []Checker, metrics *Metrics, logger log.Logger) error {
	logger.Log(ctx, log.LevelInfo, "startup_self_probe_started", log.Int("dep_count", len(checkers)))

	var firstFailure error

	for _, checker := range checkers {
		result := checker.Check(ctx)

		// up | skipped | n/a → healthy gauge value (1).
		// down | degraded   → unhealthy gauge value (0).
		up := result.Status == StatusUp || result.Status == StatusSkipped || result.Status == StatusNA

		metrics.EmitSelfProbeResult(ctx, checker.Name(), up)

		if up {
			logger.Log(ctx, log.LevelInfo, "self_probe_check",
				log.String("dep", checker.Name()),
				log.String("status", string(result.Status)),
				log.Int("latency_ms", int(result.LatencyMs)))

			continue
		}

		// Failure path: capture the first error verbatim so the wrapped
		// chain points at the root cause; later failures still get a
		// per-dep error log so operators see the full picture.
		depErr := fmt.Errorf("dep %q: status=%s error=%q",
			checker.Name(), result.Status, result.Error)
		if firstFailure == nil {
			firstFailure = depErr
		}

		logger.Log(ctx, log.LevelError, "self_probe_check",
			log.String("dep", checker.Name()),
			log.String("status", string(result.Status)),
			log.String("error", result.Error))
	}

	if firstFailure != nil {
		logger.Log(ctx, log.LevelError, "startup_self_probe_failed", log.Err(firstFailure))

		return fmt.Errorf("startup self-probe failed: %w", firstFailure)
	}

	logger.Log(ctx, log.LevelInfo, "startup_self_probe_passed")

	return nil
}
