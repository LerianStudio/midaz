// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"testing"

	"go.uber.org/goleak"
)

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(
		m,
		goleak.IgnoreCurrent(),
		goleak.IgnoreTopFunction("github.com/redis/go-redis/v9/maintnotifications.(*CircuitBreakerManager).cleanupLoop"),
		// The lib-commons telemetry middleware spawns a long-lived metrics
		// collector goroutine on first request. It is owned by global state
		// inside the middleware and has no exposed Stop() — tests that only
		// exercise Fiber routing cannot cleanly shut it down.
		goleak.IgnoreTopFunction("github.com/LerianStudio/lib-commons/v2/commons/net/http.(*TelemetryMiddleware).ensureMetricsCollector.func1.1"),
		// fasthttp starts a global server-date updater goroutine on the first
		// served request. It is intentionally unexported/long-lived and is
		// not a resource leak we can address from our code. The anonymous
		// function parks in time.Sleep, so match by AnyFunction on the
		// creating package function.
		goleak.IgnoreAnyFunction("github.com/valyala/fasthttp.updateServerDate.func1"),
		// fasthttp's workerPool spawns per-connection worker goroutines
		// whose cleanup is driven by a 10s idle timer. Tests that start a
		// Fiber app and shut it down via app.Shutdown() observe the pool's
		// drain, but the per-worker time.Sleep goroutines linger until
		// their next tick. This is a pool lifecycle detail, not a leak in
		// our code — ignore by AnyFunction since the closure is anonymous.
		goleak.IgnoreAnyFunction("github.com/valyala/fasthttp.(*workerPool).Start.func2"),
	)
}
