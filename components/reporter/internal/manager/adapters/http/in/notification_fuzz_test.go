//go:build fuzz

// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"math"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/valyala/fasthttp"
)

// fuzzNotificationApp is the shared Fiber app used to acquire a lightweight ctx
// per fuzz execution. It is built once (sync.OnceValue) so each execution costs
// only AcquireCtx/ReleaseCtx rather than a full fiber.New() plus an app.Test()
// HTTP round-trip — keeping execution cheap enough to avoid the Go -fuzztime
// boundary "context deadline exceeded" flake the round-trip provoked.
var fuzzNotificationApp = sync.OnceValue(func() *fiber.App {
	return fiber.New(fiber.Config{DisableStartupMessage: true})
})

// FuzzNotification_ParseLimit fuzz tests parseNotificationLimit by injecting
// random strings as the "limit" query parameter. The function must never panic
// and must always return either a valid limit in [1,100] or a non-nil error.
func FuzzNotification_ParseLimit(f *testing.F) {
	// Category 1: Valid inputs
	f.Add("10")
	f.Add("1")
	f.Add("100")

	// Category 2: Empty/boundary values
	f.Add("")
	f.Add("0")
	f.Add("-1")
	f.Add("101")
	f.Add(strings.Repeat("9", 50))

	// Category 3: Unicode
	f.Add("\u0661\u0662\u0663") // Arabic-Indic digits
	f.Add("\u00bd")             // vulgar fraction one half

	// Category 4: Invalid formats
	f.Add("abc")
	f.Add("10.5")
	f.Add("1e2")
	f.Add(" 10 ")
	f.Add("10abc")

	// Category 5: Security payloads
	f.Add("' OR 1=1 --")
	f.Add("<script>alert(1)</script>")
	f.Add("\x00\x01\x02")

	f.Fuzz(func(t *testing.T, raw string) {
		// Bound input to prevent resource exhaustion
		if len(raw) > 512 {
			raw = raw[:512]
		}

		// A panic in parseNotificationLimit is a crash — surface it as a failure
		// rather than letting it abort the fuzz worker silently.
		defer func() {
			if r := recover(); r != nil {
				t.Errorf("parseNotificationLimit panicked for input %q: %v", raw, r)
			}
		}()

		// parseNotificationLimit reads only the limit query parameter via c.Query,
		// so a directly-acquired ctx exercises the same path as an app.Test()
		// round-trip without the per-exec HTTP cost. URL-encode the raw value so
		// it survives intact as the query parameter.
		target := "/v1/deadlines/notifications?limit=" + url.QueryEscape(raw)

		fctx := &fasthttp.RequestCtx{}
		fctx.Request.SetRequestURI(target)

		c := fuzzNotificationApp().AcquireCtx(fctx)
		defer fuzzNotificationApp().ReleaseCtx(c)

		limit, parseErr := parseNotificationLimit(c)

		// Invariant 1: Must never return both a valid limit AND an error
		if parseErr != nil && limit != 0 {
			t.Errorf("returned non-zero limit %d with error: %v for input %q",
				limit, parseErr, raw)
		}

		// Invariant 2: On success, limit must be within [1, 100]
		if parseErr == nil {
			if limit < minNotificationLimit || limit > maxNotificationLimit {
				t.Errorf("limit %d outside valid range [%d, %d] for input %q",
					limit, minNotificationLimit, maxNotificationLimit, raw)
			}
		}
	})
}

// FuzzNotification_ComputeSeverity fuzz tests ComputeNotificationSeverity with
// random integer inputs. The function must always return one of the three valid
// severity strings and must never panic.
func FuzzNotification_ComputeSeverity(f *testing.F) {
	// Category 1: Valid inputs (typical days-until-due values)
	f.Add(0)
	f.Add(3)
	f.Add(7)
	f.Add(30)

	// Category 2: Boundary values
	f.Add(-1)
	f.Add(1)
	f.Add(8)
	f.Add(warningThresholdDays)

	// Category 3: Extreme values
	f.Add(math.MinInt64)
	f.Add(math.MaxInt64)
	f.Add(math.MinInt32)
	f.Add(math.MaxInt32)

	// Category 4: Overdue values
	f.Add(-100)
	f.Add(-365)
	f.Add(-1000)

	// Category 5: Far future values
	f.Add(365)
	f.Add(3650)
	f.Add(100000)

	f.Fuzz(func(t *testing.T, daysUntilDue int) {
		severity := ComputeNotificationSeverity(daysUntilDue)

		// Invariant: result must be one of the three valid severity strings
		switch severity {
		case overdueSeverity, warningSeverity, infoSeverity:
			// valid
		default:
			t.Errorf("ComputeNotificationSeverity(%d) = %q, want one of %q/%q/%q",
				daysUntilDue, severity, overdueSeverity, warningSeverity, infoSeverity)
		}
	})
}

// FuzzNotification_ComputeDaysUntilDue fuzz tests ComputeDaysUntilDue with
// random Unix timestamps for dueDate and now. The function must never panic and
// must return a deterministic result for the same truncated-day inputs.
func FuzzNotification_ComputeDaysUntilDue(f *testing.F) {
	now := time.Now().UTC()

	// Category 1: Valid inputs (typical due dates)
	f.Add(now.Add(24*time.Hour).Unix(), now.Unix())   // tomorrow
	f.Add(now.Add(7*24*time.Hour).Unix(), now.Unix()) // 1 week
	f.Add(now.Add(30*24*time.Hour).Unix(), now.Unix())

	// Category 2: Boundary values
	f.Add(now.Unix(), now.Unix())                    // same day
	f.Add(now.Add(-24*time.Hour).Unix(), now.Unix()) // yesterday
	f.Add(now.Add(1*time.Hour).Unix(), now.Unix())   // same day, hours differ

	// Category 3: Extreme dates
	f.Add(int64(0), now.Unix())             // Unix epoch
	f.Add(int64(math.MaxInt32), now.Unix()) // year 2038
	f.Add(int64(-62135596800), now.Unix())  // year 0001

	// Category 4: Same timestamps
	f.Add(int64(1000000000), int64(1000000000))
	f.Add(int64(86400), int64(86400))

	// Category 5: Reversed (now > due = overdue)
	f.Add(now.Add(-365*24*time.Hour).Unix(), now.Unix())  // 1 year overdue
	f.Add(now.Add(-3650*24*time.Hour).Unix(), now.Unix()) // 10 years overdue

	f.Fuzz(func(t *testing.T, dueSec, nowSec int64) {
		// Bound timestamps to prevent time.Unix from producing extreme values
		// that could cause overflow in Sub() or Hours()
		const maxSec = 253402300799 // 9999-12-31T23:59:59Z
		const minSec = -62135596800 // 0001-01-01T00:00:00Z

		dueSec = clampInt64(dueSec, minSec, maxSec)
		nowSec = clampInt64(nowSec, minSec, maxSec)

		dueDate := time.Unix(dueSec, 0).UTC()
		nowTime := time.Unix(nowSec, 0).UTC()

		// Must not panic
		result := ComputeDaysUntilDue(dueDate, nowTime)

		// Invariant: same-day inputs must return 0
		dueTrunc := dueDate.Truncate(24 * time.Hour)
		nowTrunc := nowTime.Truncate(24 * time.Hour)

		if dueTrunc.Equal(nowTrunc) && result != 0 {
			t.Errorf("ComputeDaysUntilDue(%v, %v) = %d, want 0 for same day",
				dueDate, nowTime, result)
		}

		// Invariant: sign must match direction
		if dueTrunc.After(nowTrunc) && result <= 0 {
			t.Errorf("ComputeDaysUntilDue(%v, %v) = %d, want positive for future due date",
				dueDate, nowTime, result)
		}

		if dueTrunc.Before(nowTrunc) && result >= 0 {
			t.Errorf("ComputeDaysUntilDue(%v, %v) = %d, want negative for past due date",
				dueDate, nowTime, result)
		}
	})
}

// clampInt64 bounds v within [lo, hi].
func clampInt64(v, lo, hi int64) int64 {
	if v < lo {
		return lo
	}

	if v > hi {
		return hi
	}

	return v
}
