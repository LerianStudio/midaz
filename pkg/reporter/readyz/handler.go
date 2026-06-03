// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package readyz

import (
	"context"
	"sync"
	"time"

	"github.com/gofiber/fiber/v2"
)

// drainReason is the reason text emitted when the service is draining.
const drainReason = "service is draining; refusing new traffic"

// NewHandler returns a Fiber handler that implements the canonical /readyz
// contract. It runs all configured checkers in parallel, applies the
// canonical aggregation rule, and short-circuits to a 503 "draining"
// response when DrainState.IsDraining() is true.
//
// The handler MUST NOT cache responses. Every call runs live probes.
//
// Parameters:
//   - checkers: the per-dependency probes; nil/empty produces a healthy
//     response with no checks block.
//   - drainState: the shared drain flag; nil is treated as "not draining"
//     so callers don't have to allocate a DrainState in tests.
//   - version: emitted in every response (typically OTEL_RESOURCE_SERVICE_VERSION).
//   - deploymentMode: emitted in every response (saas | byoc | local).
//   - metrics: the OTel emitter for readyz_check_duration_ms and
//     readyz_check_status. nil is tolerated — emit calls are no-ops on a
//     nil receiver, which is convenient for tests and partial-bootstrap
//     code paths. Production callers MUST pass a non-nil Metrics.
func NewHandler(checkers []Checker, drainState *DrainState, version, deploymentMode string, metrics *Metrics) fiber.Handler {
	return func(c *fiber.Ctx) error {
		if drainState != nil && drainState.IsDraining() {
			return c.Status(fiber.StatusServiceUnavailable).JSON(Response{
				Status: "draining",
				Reason: drainReason,
				// Always include an empty (but non-nil) Checks map so
				// dashboards keying on `checks` keep working during
				// drain. Without this, encoding/json omits the field
				// entirely (nil map + omitempty) and downstream parsers
				// see two different shapes for /readyz responses.
				Checks:         map[string]DependencyCheck{},
				Version:        version,
				DeploymentMode: deploymentMode,
			})
		}

		ctx := c.UserContext()
		results := runAll(ctx, checkers)
		emitCheckResults(ctx, metrics, results)
		topStatus, code := Aggregate(results)

		return c.Status(code).JSON(Response{
			Status:         topStatus,
			Checks:         results,
			Version:        version,
			DeploymentMode: deploymentMode,
		})
	}
}

// runAll fans out each Checker.Check call into its own goroutine, collects
// the results into a map keyed by checker name, and returns once all probes
// complete OR ctx is cancelled — whichever happens first. Each checker is
// responsible for applying its own per-probe timeout (see CheckTimeout*
// constants in checks.go); the request-level ctx is the upper bound for the
// fan-out as a whole.
//
// Defensive ctx-aware wait (test-reviewer H4): if a checker ignores ctx and
// hangs past the request budget, runAll returns the partial results instead
// of blocking on wg.Wait(). The misbehaving goroutine remains live until
// its Check call eventually returns; the buffered out channel ensures it
// never blocks on the receiver, so it cleans up on its own.
//
// In-flight checkers whose result has not been collected are synthesized as
// StatusDown with a "timed out" error so the response shape is stable —
// every declared dependency always has an entry in the Checks map.
//
// runAll is shared between the Fiber and net/http handlers so the parallel
// fan-out semantics are identical regardless of transport.
func runAll(ctx context.Context, checkers []Checker) map[string]DependencyCheck {
	if len(checkers) == 0 {
		return map[string]DependencyCheck{}
	}

	type result struct {
		name  string
		check DependencyCheck
	}

	out := make(chan result, len(checkers))

	var wg sync.WaitGroup

	for _, ck := range checkers {
		ck := ck

		wg.Add(1)

		go func() {
			defer wg.Done()

			out <- result{name: ck.Name(), check: ck.Check(ctx)}
		}()
	}

	// done fires when every spawned goroutine has finished. We DO NOT close
	// out here: a checker that ignores ctx may still be running, and any
	// future write would panic on a closed channel. The buffered channel
	// (len=len(checkers)) absorbs late writes safely.
	done := make(chan struct{})

	go func() {
		wg.Wait()
		close(done)
	}()

	collected := make(map[string]DependencyCheck, len(checkers))

	collect := func() {
		for {
			select {
			case r := <-out:
				collected[r.name] = r.check
			default:
				return
			}
		}
	}

	select {
	case <-done:
		// Happy path: every checker finished. Drain the buffered out chan.
		collect()
	case <-ctx.Done():
		// Defensive: ctx fired before all checkers returned. Drain whatever
		// is already in the buffer, then synthesize a timed-out entry for
		// every checker whose result is still missing so the response shape
		// is consistent. The in-flight goroutines remain live until their
		// Check call returns; they will write to the buffered out chan on
		// the way out, but we have already returned and that's fine — the
		// channel is buffered to len(checkers) so the writes never block.
		collect()
	}

	for _, ck := range checkers {
		if _, ok := collected[ck.Name()]; ok {
			continue
		}

		collected[ck.Name()] = DependencyCheck{
			Status: StatusDown,
			Error:  "checker timed out: did not complete within request budget",
		}
	}

	return collected
}

// emitCheckResults emits the duration histogram and the status counter for
// every per-dep check returned by runAll. Called synchronously after runAll
// completes (not in a background goroutine) so the emission happens on the
// request path and is visible to tests using a manual reader.
//
// Skipping emission entirely when metrics is nil is delegated to the nil
// receiver guard inside Metrics; this keeps the call site uncluttered.
//
// Precision contract (test-reviewer H2): when the checker populates the
// internal Latency field, that unrounded duration is forwarded to the
// histogram so sub-ms probes (cache hits, in-memory probes) record their
// actual fractional ms value rather than 0. Older checkers that leave
// Latency=0 fall back to LatencyMs, preserving backward compatibility at
// the cost of bottom-out precision (the lossy path).
func emitCheckResults(ctx context.Context, metrics *Metrics, results map[string]DependencyCheck) {
	for name, check := range results {
		latency := check.Latency
		if latency == 0 {
			latency = time.Duration(check.LatencyMs) * time.Millisecond
		}

		metrics.EmitCheckDuration(ctx, name, check.Status, latency)
		metrics.EmitCheckStatus(ctx, name, check.Status)
	}
}
