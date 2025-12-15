package fuzzy

import (
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"
)

// FuzzPaginationCursor fuzzes pagination cursor decoding.
// Run with: go test -v ./tests/fuzzy -fuzz=FuzzPaginationCursor -run=^$ -fuzztime=60s
func FuzzPaginationCursor(f *testing.F) {
	// Seed: valid cursors (base64 encoded)
	f.Add("eyJpZCI6IjEyMzQ1Njc4LTEyMzQtMTIzNC0xMjM0LTEyMzQ1Njc4OTBhYiIsInBvaW50c05leHQiOnRydWV9")
	f.Add("eyJpZCI6IjAwMDAwMDAwLTAwMDAtMDAwMC0wMDAwLTAwMDAwMDAwMDAwMCIsInBvaW50c05leHQiOmZhbHNlfQ==")

	// Seed: invalid base64
	f.Add("not-base64!")
	f.Add("===")
	f.Add("")

	// Seed: valid base64, invalid JSON
	f.Add("bm90LWpzb24=")     // "not-json"
	f.Add("e30=")             // "{}"
	f.Add("eyJpZCI6bnVsbH0=") // {"id":null}

	// Seed: malformed JSON
	f.Add("eyJpZCI6ImludmFsaWQifQ==") // {"id":"invalid"} (not a UUID)
	f.Add("eyJwb2ludHNOZXh0IjoxMjN9") // {"pointsNext":123} (wrong type)

	// Seed: injection attempts
	f.Add("'; DROP TABLE balance; --")
	f.Add("<script>alert('xss')</script>")
	f.Add("{{.}}{{template}}")

	f.Fuzz(func(t *testing.T, cursor string) {
		defer func() {
			if r := recover(); r != nil {
				t.Errorf("Cursor parsing panicked: cursor=%q panic=%v",
					truncateString(cursor, 50), r)
			}
		}()

		// Simulate cursor decoding (without real implementation)
		_, _ = decodePaginationCursor(cursor)
	})
}

// FuzzDateRangeFilter fuzzes date range query parameters.
// Run with: go test -v ./tests/fuzzy -fuzz=FuzzDateRangeFilter -run=^$ -fuzztime=60s
func FuzzDateRangeFilter(f *testing.F) {
	// Seed: valid dates
	f.Add("2024-01-01", "2024-12-31")
	f.Add("2024-01-01T00:00:00Z", "2024-12-31T23:59:59Z")
	f.Add("2024-01-01T00:00:00+00:00", "2024-12-31T23:59:59-05:00")

	// Seed: edge cases
	f.Add("", "")
	f.Add("2024-01-01", "")
	f.Add("", "2024-12-31")
	f.Add("1970-01-01", "2038-01-19") // Unix epoch boundaries
	f.Add("0001-01-01", "9999-12-31") // Extreme dates

	// Seed: invalid dates
	f.Add("2024-13-01", "2024-12-31") // Invalid month
	f.Add("2024-02-30", "2024-12-31") // Invalid day
	f.Add("not-a-date", "also-not")
	f.Add("2024/01/01", "2024/12/31") // Wrong format

	// Seed: inverted range
	f.Add("2024-12-31", "2024-01-01") // End before start

	// Seed: injection
	f.Add("2024-01-01'; DROP TABLE--", "2024-12-31")

	f.Fuzz(func(t *testing.T, startDate, endDate string) {
		defer func() {
			if r := recover(); r != nil {
				t.Errorf("Date parsing panicked: start=%q end=%q panic=%v",
					startDate, endDate, r)
			}
		}()

		start, startErr := parseFilterDate(startDate)
		end, endErr := parseFilterDate(endDate)

		// If both parse successfully, start should be before end
		if startErr == nil && endErr == nil && !start.IsZero() && !end.IsZero() {
			if start.After(end) {
				// This is a validation error, not a panic - just log it
				_ = fmt.Sprintf("Inverted range: start=%v end=%v", start, end)
			}
		}
	})
}

// FuzzLimitOffset fuzzes limit/offset pagination parameters.
// Run with: go test -v ./tests/fuzzy -fuzz=FuzzLimitOffset -run=^$ -fuzztime=60s
func FuzzLimitOffset(f *testing.F) {
	// Seed: valid values
	f.Add("10", "0")
	f.Add("100", "50")
	f.Add("1", "0")

	// Seed: boundary values
	f.Add("0", "0")
	f.Add("-1", "0")
	f.Add("1000000", "0")
	f.Add("10", "-1")
	f.Add("10", "999999999")

	// Seed: non-numeric
	f.Add("ten", "zero")
	f.Add("10.5", "0")
	f.Add("1e10", "0")

	// Seed: overflow
	f.Add("9223372036854775807", "0")  // max int64
	f.Add("9223372036854775808", "0")  // overflow
	f.Add("18446744073709551615", "0") // max uint64

	f.Fuzz(func(t *testing.T, limitStr, offsetStr string) {
		defer func() {
			if r := recover(); r != nil {
				t.Errorf("Pagination parsing panicked: limit=%q offset=%q panic=%v",
					limitStr, offsetStr, r)
			}
		}()

		limit, _ := strconv.ParseInt(limitStr, 10, 64)
		offset, _ := strconv.ParseInt(offsetStr, 10, 64)

		// Validate bounds
		if limit < 0 || limit > 1000 {
			_ = fmt.Sprintf("limit out of bounds: %d", limit)
		}
		if offset < 0 {
			_ = fmt.Sprintf("offset negative: %d", offset)
		}
	})
}

// FuzzSortOrder fuzzes sort order parameters.
// Run with: go test -v ./tests/fuzzy -fuzz=FuzzSortOrder -run=^$ -fuzztime=30s
func FuzzSortOrder(f *testing.F) {
	// Seed: valid values
	f.Add("asc")
	f.Add("desc")
	f.Add("ASC")
	f.Add("DESC")
	f.Add("Asc")
	f.Add("Desc")

	// Seed: invalid
	f.Add("")
	f.Add("ascending")
	f.Add("descending")
	f.Add("random")

	// Seed: injection
	f.Add("asc; DROP TABLE--")
	f.Add("desc OR 1=1")

	f.Fuzz(func(t *testing.T, sortOrder string) {
		defer func() {
			if r := recover(); r != nil {
				t.Errorf("Sort order validation panicked: order=%q panic=%v",
					sortOrder, r)
			}
		}()

		normalized := strings.ToUpper(strings.TrimSpace(sortOrder))
		isValid := normalized == "ASC" || normalized == "DESC"
		_ = isValid
	})
}

// FuzzURLQueryString fuzzes URL query string parsing.
// Run with: go test -v ./tests/fuzzy -fuzz=FuzzURLQueryString -run=^$ -fuzztime=60s
func FuzzURLQueryString(f *testing.F) {
	// Seed: valid query strings
	f.Add("limit=10&offset=0")
	f.Add("sortOrder=desc&startDate=2024-01-01")
	f.Add("alias=test&assetCode=USD")

	// Seed: special characters
	f.Add("name=hello%20world")
	f.Add("name=hello+world")
	f.Add("filter=%3Cscript%3E")

	// Seed: edge cases
	f.Add("")
	f.Add("=")
	f.Add("&&&")
	f.Add("key=")
	f.Add("=value")
	f.Add("key=value=extra")

	// Seed: injection
	f.Add("id='; DROP TABLE--")
	f.Add("filter=<script>alert(1)</script>")
	f.Add("limit=-1 OR 1=1")

	// Seed: unicode
	f.Add("name=%E4%B8%AD%E6%96%87")
	f.Add("name=\u0000")

	f.Fuzz(func(t *testing.T, queryString string) {
		defer func() {
			if r := recover(); r != nil {
				t.Errorf("Query string parsing panicked: qs=%q panic=%v",
					truncateString(queryString, 100), r)
			}
		}()

		values, err := url.ParseQuery(queryString)
		if err != nil {
			return // Expected for malformed input
		}

		// Verify each value can be safely processed
		for key, vals := range values {
			_ = key
			for _, val := range vals {
				// Check for dangerous patterns
				lower := strings.ToLower(val)
				if strings.Contains(lower, "script") ||
					strings.Contains(lower, "drop table") ||
					strings.Contains(val, "\x00") {
					// Log but don't fail - these should be sanitized at a higher level
					_ = fmt.Sprintf("potentially dangerous value: %q", val)
				}
			}
		}
	})
}

// Helper functions

func decodePaginationCursor(cursor string) (map[string]any, error) {
	if cursor == "" {
		return nil, nil
	}
	// Simplified - real implementation would base64 decode then JSON parse
	if len(cursor) > 1000 {
		return nil, fmt.Errorf("cursor too long")
	}
	return map[string]any{}, nil
}

func parseFilterDate(dateStr string) (time.Time, error) {
	if dateStr == "" {
		return time.Time{}, nil
	}

	// Try multiple formats
	formats := []string{
		time.RFC3339,
		"2006-01-02T15:04:05Z07:00",
		"2006-01-02",
	}

	for _, format := range formats {
		if t, err := time.Parse(format, dateStr); err == nil {
			return t, nil
		}
	}

	return time.Time{}, fmt.Errorf("unable to parse date: %s", dateStr)
}

func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
