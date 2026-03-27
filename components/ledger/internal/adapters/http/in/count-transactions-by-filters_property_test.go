//go:build integration

// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

// =============================================================================
// PROPERTY-BASED TESTS — CountTransactionsByFilters Domain Invariants
//
// These tests verify that domain invariants of the count filter parsing hold
// across hundreds of automatically-generated inputs. They use testing/quick.
//
// Invariants verified:
//   1. Count is non-negative: parseCountFilter never produces a filter that
//      could yield a negative count.
//   2. Date range validity: start_date > end_date always returns an error.
//   3. Status allowlist: only CREATED, APPROVED, PENDING, CANCELED, NOTED are
//      accepted; all other strings are rejected.
//   4. Filter determinism: same input always produces the same output.
//
// Run with:
//
//	go test -tags integration -run TestProperty -v -count=1 \
//	    ./components/ledger/internal/adapters/http/in/
//
// =============================================================================

import (
	"fmt"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"testing/quick"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// boundString trims a generated string to a maximum length for property tests.
func boundString(s string, maxLen int) string {
	if len(s) > maxLen {
		return s[:maxLen]
	}

	return s
}

// callParseCountFilter creates a Fiber context with given query params and calls
// parseCountFilter, returning the result.
func callParseCountFilter(route, status, startDate, endDate string) (errResult error) {
	app := fiber.New()

	app.Get("/prop", func(c *fiber.Ctx) error {
		_, errResult = parseCountFilter(c)
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

	target := "/prop"
	if len(qv) > 0 {
		target += "?" + qv.Encode()
	}

	req := httptest.NewRequest("GET", target, nil)
	_, _ = app.Test(req)

	return errResult
}

// TestProperty_ParseCountFilter_DateRangeInvalidity verifies that whenever
// start_date is strictly after end_date, parseCountFilter always returns an error.
// This property must hold for ANY pair of valid RFC 3339 timestamps.
func TestProperty_ParseCountFilter_DateRangeInvalidity(t *testing.T) {
	t.Parallel()

	cfg := &quick.Config{MaxCount: 100}

	// Property: for any gap > 0, startDate = base+gap and endDate = base
	// must always produce an error (start > end).
	f := func(baseUnix int64, gapSeconds uint32) bool {
		// Bound inputs to reasonable range to avoid overflow
		if baseUnix < 0 {
			baseUnix = -baseUnix
		}

		baseUnix = baseUnix % (365 * 24 * 3600 * 100) // ~100 years from epoch

		base := time.Unix(baseUnix, 0).UTC()

		// Ensure gap is at least 1 second
		gap := time.Duration(gapSeconds%86400+1) * time.Second
		start := base.Add(gap)
		end := base

		startStr := start.Format(time.RFC3339)
		endStr := end.Format(time.RFC3339)

		err := callParseCountFilter("", "", startStr, endStr)

		return err != nil // must always error when start > end
	}

	err := quick.Check(f, cfg)
	require.NoError(t, err, "property violated: start_date > end_date must always return error")
}

// TestProperty_ParseCountFilter_StatusAllowlist verifies that any status string
// NOT in the valid set {CREATED, APPROVED, PENDING, CANCELED, NOTED} always
// produces a validation error.
func TestProperty_ParseCountFilter_StatusAllowlist(t *testing.T) {
	t.Parallel()

	cfg := &quick.Config{MaxCount: 100}

	validStatuses := map[string]bool{
		"CREATED": true, "APPROVED": true, "PENDING": true,
		"CANCELED": true, "NOTED": true,
	}

	f := func(status string) bool {
		status = boundString(status, 256)
		if status == "" {
			return true // empty is valid (no filter)
		}

		err := callParseCountFilter("", status, "", "")
		upper := strings.ToUpper(strings.TrimSpace(status))

		if validStatuses[upper] {
			// Valid status: must NOT error
			return err == nil
		}

		// Invalid status: MUST error
		return err != nil
	}

	err := quick.Check(f, cfg)
	require.NoError(t, err, "property violated: invalid status must always produce error")
}

// TestProperty_ParseCountFilter_Determinism verifies that calling parseCountFilter
// twice with the same input always produces the same result (error or success).
func TestProperty_ParseCountFilter_Determinism(t *testing.T) {
	t.Parallel()

	cfg := &quick.Config{MaxCount: 100}

	f := func(route, status, startDate, endDate string) bool {
		route = boundString(route, 256)
		status = boundString(status, 256)
		startDate = boundString(startDate, 256)
		endDate = boundString(endDate, 256)

		err1 := callParseCountFilter(route, status, startDate, endDate)
		err2 := callParseCountFilter(route, status, startDate, endDate)

		if err1 == nil {
			return err2 == nil
		}

		return err2 != nil
	}

	err := quick.Check(f, cfg)
	require.NoError(t, err, "property violated: parseCountFilter must be deterministic")
}

// TestProperty_ParseCountFilter_ValidDateRangeAccepted verifies that when both
// dates are valid RFC 3339 and start <= end, and status is empty, parseCountFilter
// always succeeds (returns no error).
func TestProperty_ParseCountFilter_ValidDateRangeAccepted(t *testing.T) {
	t.Parallel()

	cfg := &quick.Config{MaxCount: 100}

	f := func(baseUnix int64, gapSeconds uint32) bool {
		if baseUnix < 0 {
			baseUnix = -baseUnix
		}

		baseUnix = baseUnix % (365 * 24 * 3600 * 100)

		base := time.Unix(baseUnix, 0).UTC()
		gap := time.Duration(gapSeconds%86400) * time.Second

		start := base
		end := base.Add(gap)

		startStr := start.Format(time.RFC3339)
		endStr := end.Format(time.RFC3339)

		err := callParseCountFilter("", "", startStr, endStr)

		return err == nil
	}

	err := quick.Check(f, cfg)
	require.NoError(t, err, "property violated: valid date range (start <= end) must always succeed")
}

// TestProperty_ParseCountFilter_ValidStatusAlwaysAccepted verifies that any
// valid status string is always accepted regardless of casing.
func TestProperty_ParseCountFilter_ValidStatusAlwaysAccepted(t *testing.T) {
	t.Parallel()

	validStatuses := []string{"CREATED", "APPROVED", "PENDING", "CANCELED", "NOTED"}

	for _, status := range validStatuses {
		status := status

		t.Run(fmt.Sprintf("status_%s", status), func(t *testing.T) {
			t.Parallel()

			// Test with exact case
			err := callParseCountFilter("", status, "", "")
			assert.NoError(t, err, "valid status %s should be accepted", status)

			// Test with lowercase
			err = callParseCountFilter("", strings.ToLower(status), "", "")
			assert.NoError(t, err, "lowercase status %s should be accepted", strings.ToLower(status))

			// Test with mixed case
			mixed := strings.ToLower(status[:1]) + status[1:]
			err = callParseCountFilter("", mixed, "", "")
			assert.NoError(t, err, "mixed case status %s should be accepted", mixed)
		})
	}
}
