// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package model

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/LerianStudio/midaz/v4/pkg/constant"
)

// TimeOfDay is a value object for validating and comparing daily time boundaries.
// Used for limit time windows to restrict when limits are evaluated during the day.
// Supports overnight windows (e.g., "20:00" to "06:00") and provides serialization for database storage.
type TimeOfDay struct {
	hour    int
	minute  int
	isValid bool // Distinguishes zero value from midnight
}

// NewTimeOfDay creates a TimeOfDay from a string in "HH:MM" format.
// Returns ErrTimeOfDayInvalidFormat if the format is invalid or values are out of range.
// Accepts single-digit hour (e.g., "9:30") since time.Parse handles it correctly.
// String() always returns zero-padded format ("09:30"), matching the DB constraint.
func NewTimeOfDay(s string) (TimeOfDay, error) {
	s = strings.TrimSpace(s)

	t, err := time.Parse("15:04", s)
	if err != nil {
		return TimeOfDay{}, constant.ErrTimeOfDayInvalidFormat
	}

	return TimeOfDay{hour: t.Hour(), minute: t.Minute(), isValid: true}, nil
}

// Hour returns the hour component (0-23).
func (t TimeOfDay) Hour() int {
	return t.hour
}

// Minute returns the minute component (0-59).
func (t TimeOfDay) Minute() int {
	return t.minute
}

// String returns the time in "HH:MM" format with zero-padding.
// Returns an empty string for invalid (zero-value) TimeOfDay.
func (t TimeOfDay) String() string {
	if !t.isValid {
		return ""
	}

	return fmt.Sprintf("%02d:%02d", t.hour, t.minute)
}

// MinutesSinceMidnight returns the total minutes since midnight.
// Useful for comparisons and database storage.
func (t TimeOfDay) MinutesSinceMidnight() int {
	return t.hour*60 + t.minute
}

// Before returns true if t is before other.
// Returns false if either value is invalid (zero-value).
func (t TimeOfDay) Before(other TimeOfDay) bool {
	if !t.isValid || !other.isValid {
		return false
	}

	return t.MinutesSinceMidnight() < other.MinutesSinceMidnight()
}

// After returns true if t is after other.
// Returns false if either value is invalid (zero-value).
func (t TimeOfDay) After(other TimeOfDay) bool {
	if !t.isValid || !other.isValid {
		return false
	}

	return t.MinutesSinceMidnight() > other.MinutesSinceMidnight()
}

// Equal returns true if t equals other.
func (t TimeOfDay) Equal(other TimeOfDay) bool {
	return t.MinutesSinceMidnight() == other.MinutesSinceMidnight() && t.isValid == other.isValid
}

// IsZero returns true if this is an uninitialized TimeOfDay.
// A midnight time (00:00) created via NewTimeOfDay is not zero.
func (t TimeOfDay) IsZero() bool {
	return !t.isValid
}

// MarshalJSON implements json.Marshaler.
// Returns the time as a JSON string in "HH:MM" format.
func (t TimeOfDay) MarshalJSON() ([]byte, error) {
	if !t.isValid {
		return []byte("null"), nil
	}

	return []byte(fmt.Sprintf(`"%s"`, t.String())), nil
}

// UnmarshalJSON implements json.Unmarshaler.
// Parses a JSON string in "HH:MM" format.
func (t *TimeOfDay) UnmarshalJSON(data []byte) error {
	if string(data) == "null" {
		*t = TimeOfDay{}
		return nil
	}

	var s string
	if err := json.Unmarshal(data, &s); err != nil {
		return constant.ErrTimeOfDayInvalidFormat
	}

	parsed, err := NewTimeOfDay(s)
	if err != nil {
		return err
	}

	*t = parsed

	return nil
}

// MarshalText implements encoding.TextMarshaler. It is not exercised by
// encoding/json (which prefers MarshalJSON), but its presence makes schema
// generators that key off encoding.TextMarshaler/TextUnmarshaler — such as the
// Huma OpenAPI generator — treat TimeOfDay as a plain "HH:MM" string rather than
// introspecting its unexported fields into a bogus object schema.
func (t TimeOfDay) MarshalText() ([]byte, error) {
	if !t.isValid {
		return []byte{}, nil
	}

	return []byte(t.String()), nil
}

// UnmarshalText implements encoding.TextUnmarshaler. Like MarshalText it is not
// used by encoding/json (UnmarshalJSON takes precedence); it exists so schema
// generators treat TimeOfDay as a string. An empty text is the zero value.
func (t *TimeOfDay) UnmarshalText(text []byte) error {
	if len(text) == 0 {
		*t = TimeOfDay{}
		return nil
	}

	parsed, err := NewTimeOfDay(string(text))
	if err != nil {
		return err
	}

	*t = parsed

	return nil
}
