// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package validation

import (
	"fmt"
	"time"

	"tracer/pkg/constant"
)

// ParseRFC3339Timestamp parses a string as RFC3339 timestamp with helpful error messages.
// Returns zero time if value is empty (optional field behavior).
func ParseRFC3339Timestamp(value, fieldName string) (time.Time, error) {
	if value == "" {
		return time.Time{}, nil
	}

	t, err := time.Parse(constant.DateTimeFormat, value)
	if err != nil {
		return time.Time{}, &RFC3339ParseError{
			Field:   fieldName,
			Value:   value,
			Wrapped: err,
		}
	}

	return t, nil
}

// RFC3339ParseError provides detailed error information for invalid RFC3339 timestamps.
type RFC3339ParseError struct {
	Field   string
	Value   string
	Wrapped error
}

func (e *RFC3339ParseError) Error() string {
	return fmt.Sprintf(
		"%s must be in RFC3339 format with timezone (e.g., %s). Invalid value: %q",
		e.Field,
		constant.DateTimeFormatExample,
		e.Value,
	)
}

func (e *RFC3339ParseError) Unwrap() error {
	return e.Wrapped
}

// ValidateDateRange ensures startDate is before or equal to endDate.
// Returns nil if either date is zero (not provided).
func ValidateDateRange(startDate, endDate time.Time, startField, endField string) error {
	if startDate.IsZero() || endDate.IsZero() {
		return nil
	}

	if startDate.After(endDate) {
		return fmt.Errorf(
			"%s (%s) must not be after %s (%s)",
			startField,
			startDate.Format(constant.DateTimeFormat),
			endField,
			endDate.Format(constant.DateTimeFormat),
		)
	}

	return nil
}
