//go:build fuzz

// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/valyala/fasthttp"
)

// fuzzMetricsApp is the shared Fiber app used to acquire a lightweight ctx per
// fuzz execution. It is built once (sync.OnceValue) so each execution costs only
// AcquireCtx/ReleaseCtx rather than a full fiber.New() plus an app.Test() HTTP
// round-trip. That round-trip's per-exec latency provoked the Go -fuzztime
// boundary "context deadline exceeded" flake; keeping execution cheap removes it.
var fuzzMetricsApp = sync.OnceValue(func() *fiber.App {
	return fiber.New(fiber.Config{DisableStartupMessage: true})
})

// safeParseErrorPeriodDaysTest creates a Fiber app that exposes a test endpoint
// reading the errorPeriodDays query parameter via parseErrorPeriodDays. It
// returns any panic message. If no panic occurs, returns "".
// This helper isolates parseErrorPeriodDays panic behavior so fuzz tests can
// report panics as t.Error instead of crashing the fuzz process.
func safeParseErrorPeriodDaysTest(queryValue string) (panicMsg string, statusCode int, body string, testErr error) {
	defer func() {
		if r := recover(); r != nil {
			panicMsg = fmt.Sprintf("%v", r)
		}
	}()

	// parseErrorPeriodDays reads only the errorPeriodDays query parameter via
	// c.Query, so a directly-acquired Fiber ctx exercises the same path as a full
	// app.Test() round-trip. The status code mirrors the handler the round-trip
	// used (400 on error, 200 otherwise) so the caller's oracle is unchanged.
	fctx := &fasthttp.RequestCtx{}
	fctx.Request.SetRequestURI("/test?errorPeriodDays=" + url.QueryEscape(queryValue))

	c := fuzzMetricsApp().AcquireCtx(fctx)
	defer fuzzMetricsApp().ReleaseCtx(c)

	if _, err := parseErrorPeriodDays(c); err != nil {
		return "", http.StatusBadRequest, "", nil
	}

	return "", http.StatusOK, "", nil
}

// FuzzParseErrorPeriodDays_Value fuzz tests the parseErrorPeriodDays function
// with random string values for the errorPeriodDays query parameter. The
// function must never panic regardless of the input string content. It should
// return either a valid integer in range [1, 365] or an error.
func FuzzParseErrorPeriodDays_Value(f *testing.F) {
	// Seed corpus: 5+ categories per Ring fuzz standards

	// Category 1: Valid inputs
	f.Add("7")
	f.Add("1")
	f.Add("365")
	f.Add("30")
	f.Add("180")

	// Category 2: Empty/boundary values
	f.Add("")
	f.Add("0")
	f.Add("366")
	f.Add("-1")
	f.Add(strings.Repeat("9", 100))

	// Category 3: Unicode characters
	f.Add("\u65e5\u672c\u8a9e")
	f.Add("\U0001f389")
	f.Add("\u0000")

	// Category 4: Invalid formats
	f.Add("abc")
	f.Add("7.5")
	f.Add("1e10")
	f.Add("  7  ")
	f.Add("+7")
	f.Add("0x1F")
	f.Add("9223372036854775807")

	// Category 5: Security payloads
	f.Add("<script>alert(1)</script>")
	f.Add("' OR 1=1 --")
	f.Add("7; DROP TABLE metrics;")
	f.Add("%00%01%02")
	f.Add("../../../etc/passwd")

	f.Fuzz(func(t *testing.T, input string) {
		// Bound input length to prevent resource exhaustion
		if len(input) > 512 {
			input = input[:512]
		}

		panicMsg, statusCode, _, err := safeParseErrorPeriodDaysTest(input)

		// A panic in parseErrorPeriodDays is a crash -- report as test failure
		if panicMsg != "" {
			t.Errorf("parseErrorPeriodDays panicked for input=%q: %s",
				input, panicMsg)
			return
		}

		if err != nil {
			// Request construction errors are acceptable for fuzzed inputs
			return
		}

		// Invariant: response status must be 200 (valid) or 400 (invalid input)
		if statusCode != http.StatusOK && statusCode != http.StatusBadRequest {
			t.Errorf("unexpected HTTP status %d for input=%q; expected 200 or 400",
				statusCode, input)
		}
	})
}

// FuzzParseErrorPeriodDays_EmptyParam fuzz tests the parseErrorPeriodDays
// function when the query parameter key itself is varied. This exercises the
// code path where the parameter is absent (empty string from c.Query).
func FuzzParseErrorPeriodDays_EmptyParam(f *testing.F) {
	// Seed corpus: 5+ categories per Ring fuzz standards

	// Category 1: Valid query strings (parameter absent)
	f.Add("")
	f.Add("otherParam=42")

	// Category 2: Empty/boundary values
	f.Add("errorPeriodDays=")
	f.Add("errorPeriodDays")

	// Category 3: Unicode
	f.Add("errorPeriodDays=\u65e5\u672c\u8a9e")

	// Category 4: Invalid formats (duplicate keys)
	f.Add("errorPeriodDays=7&errorPeriodDays=30")
	f.Add("errorPeriodDays=abc&errorPeriodDays=7")

	// Category 5: Security payloads
	f.Add("errorPeriodDays=<script>")
	f.Add("errorPeriodDays=' OR 1=1 --")

	f.Fuzz(func(t *testing.T, queryString string) {
		// Bound input length to prevent resource exhaustion
		if len(queryString) > 1024 {
			queryString = queryString[:1024]
		}

		defer func() {
			if r := recover(); r != nil {
				t.Errorf("parseErrorPeriodDays panicked for queryString=%q: %v",
					queryString, r)
			}
		}()

		// Vary the full query string so the parameter may be absent, duplicated,
		// or malformed. parseErrorPeriodDays reads only via c.Query, so a directly
		// acquired ctx exercises that path without an app.Test() round-trip.
		fctx := &fasthttp.RequestCtx{}
		fctx.Request.SetRequestURI("/test?" + queryString)

		c := fuzzMetricsApp().AcquireCtx(fctx)
		defer fuzzMetricsApp().ReleaseCtx(c)

		// Invariant: must never panic (a server fault) for any query string; the
		// returned value/error itself is exercised by FuzzParseErrorPeriodDays_Value.
		_, _ = parseErrorPeriodDays(c)
	})
}
