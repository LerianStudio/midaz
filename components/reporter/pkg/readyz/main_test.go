// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package readyz

import (
	"testing"

	"go.uber.org/goleak"
)

// TestMain wraps every test in this package with go.uber.org/goleak so any
// goroutine launched by the /readyz code paths (handler.runAll fan-out,
// metrics emission, self-probe, drain) is asserted to exit cleanly before
// the test binary terminates.
//
// Layer 9 of the canonical 9-test-layer matrix from dev-readyz/SKILL.md.
//
// If a leak surfaces, prefer fixing the actual leak in production code over
// adding ignore filters; only add an IgnoreTopFunction here when the leaked
// goroutine is owned by a third-party SDK (e.g. OTel SDK background
// goroutines that linger after the meter provider is created).
func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m,
		// OTel SDK manual reader / meter provider keeps internal background
		// goroutines alive for the lifetime of the test process. They are not
		// owned by /readyz code and never leak from individual tests.
		goleak.IgnoreTopFunction("go.opentelemetry.io/otel/sdk/metric.(*PeriodicReader).run"),
		goleak.IgnoreTopFunction("go.opentelemetry.io/otel/sdk/trace.(*batchSpanProcessor).processQueue"),

		// fasthttp (the engine under gofiber/fiber) starts a singleton
		// updateServerDate goroutine the first time a handler runs. Its
		// stack tops out at time.Sleep so we match anywhere in the stack to
		// catch the wrapping closure. The goroutine sleeps in a 1s loop for
		// the lifetime of the process and is not owned by /readyz code.
		// See valyala/fasthttp/header.go: updateServerDate.
		goleak.IgnoreAnyFunction("github.com/valyala/fasthttp.updateServerDate.func1"),
	)
}
