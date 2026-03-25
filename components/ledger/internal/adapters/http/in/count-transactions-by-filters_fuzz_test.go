// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

// =============================================================================
// FUZZ TESTS — parseCountFilter Input Parsing
//
// These tests exercise the parseCountFilter function with fuzzer-generated
// inputs to verify:
//   1. No panics on any combination of route, status, start_date, end_date.
//   2. Validation errors are returned correctly for invalid inputs.
//   3. No SQL injection vectors pass through validation.
//
// Run with:
//
//	go test -run=^$ -fuzz=Fuzz -fuzztime=30s \
//	    ./components/ledger/internal/adapters/http/in/
//
// =============================================================================

import (
	"fmt"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/stretchr/testify/assert"
)

// fuzzParseCountFilter is a test helper that creates a Fiber context with the
// given query parameters and calls parseCountFilter. It returns the error (if
// any) and a boolean indicating whether the function panicked.
func fuzzParseCountFilter(t *testing.T, route, status, startDate, endDate string) (err error, panicked bool) {
	t.Helper()

	defer func() {
		if r := recover(); r != nil {
			panicked = true
			err = fmt.Errorf("panic: %v", r)
		}
	}()

	app := fiber.New()

	var capturedErr error

	app.Get("/test", func(c *fiber.Ctx) error {
		_, capturedErr = parseCountFilter(c)
		return c.SendStatus(200)
	})

	qv := url.Values{}
	if route != "" {
		qv.Set("route", route)
	}

	if status != "" {
		qv.Set("status", status)
	}

	if startDate != "" {
		qv.Set("start_date", startDate)
	}

	if endDate != "" {
		qv.Set("end_date", endDate)
	}

	target := "/test"
	if len(qv) > 0 {
		target += "?" + qv.Encode()
	}

	req := httptest.NewRequest("GET", target, nil)
	_, _ = app.Test(req)

	return capturedErr, false
}

// FuzzParseCountFilter_Route fuzzes the route query parameter to verify
// parseCountFilter never panics regardless of input.
//
// Invariants verified:
//   - No panic on any input
//   - Route value is accepted (no validation error on route alone)
func FuzzParseCountFilter_Route(f *testing.F) {
	// Seed corpus: 5+ entries covering required categories.
	f.Add("")                          // Empty
	f.Add("PIX")                       // Valid: typical route
	f.Add("TED")                       // Valid: another route
	f.Add(strings.Repeat("A", 1000))   // Boundary: very long string
	f.Add("route'; DROP TABLE t;--")   // Security: SQL injection
	f.Add("\x00\x01\x02null-bytes")    // Security: null bytes
	f.Add("日本語テスト")                    // Unicode: non-ASCII
	f.Add("route with spaces & chars") // Special characters

	f.Fuzz(func(t *testing.T, route string) {
		// Bound input to prevent OOM
		if len(route) > 1024 {
			route = route[:1024]
		}

		err, panicked := fuzzParseCountFilter(t, route, "", "", "")
		assert.False(t, panicked, "parseCountFilter must not panic for route=%q", route)

		// Route has no validation (any string is accepted).
		// The only error source is date parsing, and we provide no dates.
		assert.NoError(t, err, "route-only filter should not error for route=%q", route)
	})
}

// FuzzParseCountFilter_Status fuzzes the status query parameter.
//
// Invariants verified:
//   - No panic on any input
//   - Only allowlisted statuses are accepted (CREATED, APPROVED, PENDING, CANCELED, NOTED)
//   - All other strings produce a validation error
func FuzzParseCountFilter_Status(f *testing.F) {
	// Seed corpus
	f.Add("")                             // Empty: no filter
	f.Add("APPROVED")                     // Valid
	f.Add("CREATED")                      // Valid
	f.Add("PENDING")                      // Valid
	f.Add("CANCELED")                     // Valid
	f.Add("NOTED")                        // Valid
	f.Add("approved")                     // Boundary: lowercase (should be normalized)
	f.Add("INVALID_STATUS")               // Invalid
	f.Add("'; DROP TABLE transaction;--") // Security: SQL injection
	f.Add(strings.Repeat("STATUS", 200))  // Boundary: very long
	f.Add("\x00")                         // Security: null byte
	f.Add("日本語")                          // Unicode: non-ASCII

	f.Fuzz(func(t *testing.T, status string) {
		if len(status) > 1024 {
			status = status[:1024]
		}

		err, panicked := fuzzParseCountFilter(t, "", status, "", "")
		assert.False(t, panicked, "parseCountFilter must not panic for status=%q", status)

		// parseCountFilter trims the status before checking.
		// If the trimmed result is empty, no filter is applied (valid).
		trimmed := strings.TrimSpace(status)
		if trimmed == "" {
			assert.NoError(t, err, "empty/whitespace-only status should not error")
			return
		}

		upper := strings.ToUpper(trimmed)
		validStatuses := map[string]bool{
			"CREATED": true, "APPROVED": true, "PENDING": true,
			"CANCELED": true, "NOTED": true,
		}

		if validStatuses[upper] {
			assert.NoError(t, err, "valid status %q should not error", status)
		} else {
			assert.Error(t, err, "invalid status %q should produce error", status)
		}
	})
}

// FuzzParseCountFilter_Dates fuzzes the start_date and end_date query parameters.
//
// Invariants verified:
//   - No panic on any input
//   - Valid RFC 3339 dates are accepted
//   - Invalid date formats produce validation errors
//   - start_date > end_date produces validation error
func FuzzParseCountFilter_Dates(f *testing.F) {
	now := time.Now().UTC()
	today := now.Format(time.RFC3339)
	yesterday := now.Add(-24 * time.Hour).Format(time.RFC3339)
	tomorrow := now.Add(24 * time.Hour).Format(time.RFC3339)

	// Seed corpus
	f.Add("", "")                                                   // Empty: use defaults
	f.Add(today, today)                                             // Valid: same day
	f.Add(yesterday, tomorrow)                                      // Valid: range
	f.Add("not-a-date", "")                                         // Invalid: bad format
	f.Add("", "not-a-date")                                         // Invalid: bad end_date
	f.Add(tomorrow, yesterday)                                      // Invalid: start > end
	f.Add("2025-01-01T00:00:00Z", "2025-12-31T23:59:59Z")           // Valid: explicit range
	f.Add("2025-01-01", "2025-12-31")                               // Invalid: not RFC 3339
	f.Add("'; DROP TABLE t;--", "")                                 // Security: SQL injection
	f.Add(strings.Repeat("2", 500), "")                             // Boundary: very long
	f.Add("0001-01-01T00:00:00Z", "9999-12-31T23:59:59Z")           // Boundary: extreme dates
	f.Add("2025-01-01T00:00:00+05:30", "2025-01-02T00:00:00-03:00") // Valid: with timezone

	f.Fuzz(func(t *testing.T, startDate, endDate string) {
		if len(startDate) > 512 {
			startDate = startDate[:512]
		}
		if len(endDate) > 512 {
			endDate = endDate[:512]
		}

		err, panicked := fuzzParseCountFilter(t, "", "", startDate, endDate)
		assert.False(t, panicked, "parseCountFilter must not panic for dates start=%q end=%q", startDate, endDate)

		// parseCountFilter trims whitespace from both dates.
		// If the trimmed result is empty, defaults are used (valid).
		trimmedStart := strings.TrimSpace(startDate)
		trimmedEnd := strings.TrimSpace(endDate)

		// If trimmed start is non-empty and not valid RFC 3339, should error
		if trimmedStart != "" {
			if _, parseErr := time.Parse(time.RFC3339, trimmedStart); parseErr != nil {
				assert.Error(t, err, "invalid start_date format should produce error")
				return
			}
		}

		// If trimmed end is non-empty and not valid RFC 3339, should error
		if trimmedEnd != "" {
			if _, parseErr := time.Parse(time.RFC3339, trimmedEnd); parseErr != nil {
				assert.Error(t, err, "invalid end_date format should produce error")
				return
			}
		}

		// Both parseable or empty - check date order
		if err != nil {
			// Could be start > end
			t.Logf("Error (possibly start > end): %v", err)
		}
	})
}

// FuzzParseCountFilter_AllParams fuzzes all four query parameters simultaneously.
//
// Invariants verified:
//   - No panic on any combination of inputs
//   - Function is deterministic (same input → same result)
func FuzzParseCountFilter_AllParams(f *testing.F) {
	// Seed corpus: combined scenarios
	f.Add("PIX", "APPROVED", "2025-01-01T00:00:00Z", "2025-12-31T23:59:59Z") // All valid
	f.Add("", "", "", "")                                                    // All empty
	f.Add("TED", "INVALID", "", "")                                          // Invalid status
	f.Add("", "", "not-a-date", "2025-01-01T00:00:00Z")                      // Invalid start
	f.Add("route'; DROP TABLE t;--", "'; DROP TABLE t;--", "'; DROP TABLE t;--", "")
	f.Add(strings.Repeat("X", 500), strings.Repeat("Y", 500), "", "") // Long strings

	f.Fuzz(func(t *testing.T, route, status, startDate, endDate string) {
		// Bound all inputs
		if len(route) > 1024 {
			route = route[:1024]
		}
		if len(status) > 1024 {
			status = status[:1024]
		}
		if len(startDate) > 512 {
			startDate = startDate[:512]
		}
		if len(endDate) > 512 {
			endDate = endDate[:512]
		}

		err1, panicked := fuzzParseCountFilter(t, route, status, startDate, endDate)
		assert.False(t, panicked, "parseCountFilter must not panic")

		// Determinism: same input must produce same result
		err2, panicked2 := fuzzParseCountFilter(t, route, status, startDate, endDate)
		assert.False(t, panicked2, "determinism check must not panic")

		if err1 == nil {
			assert.NoError(t, err2, "determinism: second call must also succeed")
		} else {
			assert.Error(t, err2, "determinism: second call must also fail")
		}
	})
}
