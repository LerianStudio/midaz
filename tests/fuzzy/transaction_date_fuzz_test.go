package fuzzy

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/LerianStudio/midaz/v3/pkg/transaction"
)

// FuzzTransactionDateUnmarshalJSON fuzzes the TransactionDate JSON unmarshalling
// with various date formats and malformed inputs.
// Run with: go test -v ./tests/fuzzy -fuzz=FuzzTransactionDateUnmarshalJSON -run=^$ -fuzztime=60s
func FuzzTransactionDateUnmarshalJSON(f *testing.F) {
	// Seed: valid RFC3339Nano formats
	f.Add(`"2024-01-15T14:30:45.123456789Z"`)
	f.Add(`"2024-12-31T23:59:59.999999999Z"`)
	f.Add(`"2024-01-01T00:00:00.000000001Z"`)

	// Seed: valid RFC3339 formats
	f.Add(`"2024-01-15T14:30:45Z"`)
	f.Add(`"2024-01-15T14:30:45+05:30"`)
	f.Add(`"2024-01-15T14:30:45-08:00"`)

	// Seed: valid milliseconds format
	f.Add(`"2024-01-15T14:30:45.000Z"`)
	f.Add(`"2024-01-15T14:30:45.123Z"`)
	f.Add(`"2024-01-15T14:30:45.999Z"`)

	// Seed: valid ISO 8601 without timezone
	f.Add(`"2024-01-15T14:30:45"`)
	f.Add(`"2024-12-31T00:00:00"`)

	// Seed: valid date-only format
	f.Add(`"2024-01-15"`)
	f.Add(`"2024-12-31"`)
	f.Add(`"2000-01-01"`)
	f.Add(`"1970-01-01"`)

	// Seed: null and empty
	f.Add(`null`)
	f.Add(`""`)
	f.Add(`"null"`)

	// Seed: boundary dates
	f.Add(`"0001-01-01T00:00:00Z"`)
	f.Add(`"9999-12-31T23:59:59Z"`)
	f.Add(`"1970-01-01T00:00:00Z"`)

	// Seed: invalid month/day combinations
	f.Add(`"2024-02-30T00:00:00Z"`) // Feb 30 doesn't exist
	f.Add(`"2024-04-31T00:00:00Z"`) // Apr has 30 days
	f.Add(`"2024-13-01T00:00:00Z"`) // Month 13
	f.Add(`"2024-00-01T00:00:00Z"`) // Month 0
	f.Add(`"2024-01-32T00:00:00Z"`) // Day 32

	// Seed: leap year edge cases
	f.Add(`"2024-02-29T00:00:00Z"`) // Valid leap year
	f.Add(`"2023-02-29T00:00:00Z"`) // Invalid non-leap year
	f.Add(`"2000-02-29T00:00:00Z"`) // Valid century leap year
	f.Add(`"1900-02-29T00:00:00Z"`) // Invalid century non-leap year

	// Seed: malformed JSON
	f.Add(`2024-01-15`)             // Missing quotes
	f.Add(`"2024-01-15`)            // Missing end quote
	f.Add(`2024-01-15"`)            // Missing start quote
	f.Add(`{"date": "2024-01-15"}`) // Object instead of string

	// Seed: wrong separators
	f.Add(`"2024/01/15T14:30:45Z"`)
	f.Add(`"2024.01.15T14:30:45Z"`)
	f.Add(`"2024-01-15 14:30:45Z"`)
	f.Add(`"2024-01-15T14.30.45Z"`)

	// Seed: timezone edge cases
	f.Add(`"2024-01-15T14:30:45+14:00"`) // Max positive offset
	f.Add(`"2024-01-15T14:30:45-12:00"`) // Max negative offset
	f.Add(`"2024-01-15T14:30:45+00:00"`) // UTC explicit
	f.Add(`"2024-01-15T14:30:45+25:00"`) // Invalid offset

	// Seed: precision edge cases
	f.Add(`"2024-01-15T14:30:45.1Z"`)
	f.Add(`"2024-01-15T14:30:45.12Z"`)
	f.Add(`"2024-01-15T14:30:45.123Z"`)
	f.Add(`"2024-01-15T14:30:45.1234567890Z"`) // Too many digits

	// Seed: injection patterns
	f.Add(`"2024-01-15'; DROP TABLE--"`)
	f.Add(`"2024-01-15<script>alert(1)</script>"`)
	f.Add(`"2024-01-15${env.SECRET}"`)

	// Seed: unicode and control characters
	f.Add(`"2024-01-15T14:30:45Z` + "\x00" + `"`)
	f.Add(`"2024-01-15T14:30:45Z` + "\u200B" + `"`)
	f.Add(`"` + "\u202E" + `2024-01-15"`) // Right-to-left override

	// Seed: extremely long values
	f.Add(`"` + strings.Repeat("2024-01-15", 1000) + `"`)

	f.Fuzz(func(t *testing.T, jsonInput string) {
		// Should NEVER panic regardless of input
		defer func() {
			if r := recover(); r != nil {
				t.Errorf("TransactionDate.UnmarshalJSON panicked on input: %q panic=%v",
					truncateDateStr(jsonInput, 100), r)
			}
		}()

		var td transaction.TransactionDate
		err := json.Unmarshal([]byte(jsonInput), &td)

		// If parsing succeeded, verify the result is usable
		if err == nil {
			// Calling Time() should never panic
			_ = td.Time()

			// Calling IsZero() should never panic
			_ = td.IsZero()

			// Re-marshaling should not panic
			_, marshalErr := json.Marshal(td)
			if marshalErr != nil {
				t.Logf("Marshal failed after successful unmarshal: input=%q err=%v",
					truncateDateStr(jsonInput, 50), marshalErr)
			}
		}
	})
}

// FuzzTransactionDateMarshalJSON fuzzes the TransactionDate JSON marshalling
// with various time values to ensure consistent output.
// Run with: go test -v ./tests/fuzzy -fuzz=FuzzTransactionDateMarshalJSON -run=^$ -fuzztime=30s
func FuzzTransactionDateMarshalJSON(f *testing.F) {
	// Seed: epoch timestamps
	f.Add(int64(0))                    // Unix epoch
	f.Add(int64(1704067200))           // 2024-01-01 00:00:00 UTC
	f.Add(int64(1735689599))           // 2024-12-31 23:59:59 UTC
	f.Add(int64(-62135596800))         // 0001-01-01 00:00:00 UTC
	f.Add(int64(253402300799))         // 9999-12-31 23:59:59 UTC

	// Seed: boundary values
	f.Add(int64(-9223372036854775808)) // min int64
	f.Add(int64(9223372036854775807))  // max int64

	// Seed: additional timestamp values (varied seconds, nanoseconds handled by UnmarshalJSON test)
	f.Add(int64(1704067200) + 1)
	f.Add(int64(1704067200) + 999999999)

	f.Fuzz(func(t *testing.T, unixSeconds int64) {
		defer func() {
			if r := recover(); r != nil {
				t.Errorf("TransactionDate.MarshalJSON panicked on unix=%d panic=%v",
					unixSeconds, r)
			}
		}()

		// Create TransactionDate from time
		tm := time.Unix(unixSeconds, 0).UTC()
		td := transaction.TransactionDate(tm)

		// Marshal should not panic
		data, err := json.Marshal(td)

		// If marshaling succeeded, the output should be valid JSON
		if err == nil && len(data) > 0 {
			// Verify it's valid JSON string
			var checkStr string
			if jsonErr := json.Unmarshal(data, &checkStr); jsonErr != nil && string(data) != "null" {
				t.Errorf("MarshalJSON produced invalid JSON string: unix=%d output=%s err=%v",
					unixSeconds, string(data), jsonErr)
			}
		}
	})
}

// TODO(review): Consider rune-based truncation for multi-byte UTF-8
// truncateDateStr safely truncates a string for logging
func truncateDateStr(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
