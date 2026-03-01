// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package transaction

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

// ErrInvalidDateFormat is returned when a date string does not match any supported format.
var ErrInvalidDateFormat = errors.New("invalid date format")

// TransactionDate is a custom time type that supports multiple ISO 8601 formats including milliseconds.
type TransactionDate time.Time

var transactionDateFormats = []string{
	time.RFC3339Nano,
	time.RFC3339,
	"2006-01-02T15:04:05.000Z",
	"2006-01-02T15:04:05Z",
	"2006-01-02T15:04:05",
	"2006-01-02",
}

// UnmarshalJSON parses a JSON-encoded date string, supporting multiple ISO 8601 formats.
func (td *TransactionDate) UnmarshalJSON(data []byte) error {
	str := strings.Trim(string(data), `"`)

	if str == "null" || str == "" {
		*td = TransactionDate{}

		return nil
	}

	for _, format := range transactionDateFormats {
		if t, err := time.Parse(format, str); err == nil {
			if t.IsZero() {
				*td = TransactionDate{}

				return nil
			}

			*td = TransactionDate(t)

			return nil
		}
	}

	return fmt.Errorf("%w: %s", ErrInvalidDateFormat, str)
}

// MarshalJSON encodes the date as a JSON string in RFC3339 format, or "null" for zero values.
func (td TransactionDate) MarshalJSON() ([]byte, error) {
	if td.IsZero() {
		return []byte("null"), nil
	}

	t := time.Time(td)

	if t.Nanosecond() != 0 {
		return json.Marshal(t.Format("2006-01-02T15:04:05.000Z07:00"))
	}

	return json.Marshal(t.Format(time.RFC3339))
}

// Time converts the TransactionDate to a standard time.Time value.
func (td TransactionDate) Time() time.Time {
	return time.Time(td)
}

// IsZero reports whether the date represents the zero time instant.
func (td TransactionDate) IsZero() bool {
	return time.Time(td).IsZero()
}

// After reports whether the date is after the given time t.
func (td TransactionDate) After(t time.Time) bool {
	return time.Time(td).After(t)
}
