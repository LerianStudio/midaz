// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package model

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/pkg/constant"
)

func mustNewTimeOfDay(s string) TimeOfDay {
	tod, err := NewTimeOfDay(s)
	if err != nil {
		panic(fmt.Sprintf("mustNewTimeOfDay(%q): %v", s, err))
	}

	return tod
}

// TestNewTimeOfDay tests TimeOfDay value object creation and parsing.
func TestNewTimeOfDay(t *testing.T) {
	tests := []struct {
		name         string
		input        string
		expectedHour int
		expectedMin  int
		expectError  bool
		errorIs      error
	}{
		{
			name:         "parses valid time HH:MM",
			input:        "09:30",
			expectedHour: 9,
			expectedMin:  30,
			expectError:  false,
		},
		{
			name:         "parses midnight",
			input:        "00:00",
			expectedHour: 0,
			expectedMin:  0,
			expectError:  false,
		},
		{
			name:         "parses end of day",
			input:        "23:59",
			expectedHour: 23,
			expectedMin:  59,
			expectError:  false,
		},
		{
			name:         "parses noon",
			input:        "12:00",
			expectedHour: 12,
			expectedMin:  0,
			expectError:  false,
		},
		{
			name:        "rejects invalid format without colon",
			input:       "0930",
			expectError: true,
			errorIs:     constant.ErrTimeOfDayInvalidFormat,
		},
		{
			name:        "rejects invalid format with seconds",
			input:       "09:30:00",
			expectError: true,
			errorIs:     constant.ErrTimeOfDayInvalidFormat,
		},
		{
			name:        "rejects invalid hour (24)",
			input:       "24:00",
			expectError: true,
			errorIs:     constant.ErrTimeOfDayInvalidFormat,
		},
		{
			name:        "rejects invalid hour (negative)",
			input:       "-1:00",
			expectError: true,
			errorIs:     constant.ErrTimeOfDayInvalidFormat,
		},
		{
			name:        "rejects invalid minute (60)",
			input:       "09:60",
			expectError: true,
			errorIs:     constant.ErrTimeOfDayInvalidFormat,
		},
		{
			name:        "rejects empty string",
			input:       "",
			expectError: true,
			errorIs:     constant.ErrTimeOfDayInvalidFormat,
		},
		{
			name:        "rejects whitespace only",
			input:       "   ",
			expectError: true,
			errorIs:     constant.ErrTimeOfDayInvalidFormat,
		},
		{
			name:        "rejects non-numeric hour",
			input:       "ab:30",
			expectError: true,
			errorIs:     constant.ErrTimeOfDayInvalidFormat,
		},
		{
			name:        "rejects non-numeric minute",
			input:       "09:xy",
			expectError: true,
			errorIs:     constant.ErrTimeOfDayInvalidFormat,
		},
		{
			// "9:30" is accepted; String() returns "09:30" which satisfies the DB constraint.
			name:         "accepts single-digit hour",
			input:        "9:30",
			expectedHour: 9,
			expectedMin:  30,
			expectError:  false,
		},
		{
			name:        "rejects single-digit minute",
			input:       "09:5",
			expectError: true,
			errorIs:     constant.ErrTimeOfDayInvalidFormat,
		},
		{
			name:        "rejects both single-digit",
			input:       "9:5",
			expectError: true,
			errorIs:     constant.ErrTimeOfDayInvalidFormat,
		},
		{
			name:         "trims whitespace",
			input:        "  09:30  ",
			expectedHour: 9,
			expectedMin:  30,
			expectError:  false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tod, err := NewTimeOfDay(tc.input)

			if tc.expectError {
				require.Error(t, err)
				assert.ErrorIs(t, err, tc.errorIs)
				assert.Equal(t, TimeOfDay{}, tod)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tc.expectedHour, tod.Hour())
				assert.Equal(t, tc.expectedMin, tod.Minute())
			}
		})
	}
}

// TestTimeOfDay_String tests TimeOfDay serialization to string.
func TestTimeOfDay_String(t *testing.T) {
	tests := []struct {
		name     string
		expected string
	}{
		{
			name:     "formats single digit hour and minute with padding",
			expected: "09:05",
		},
		{
			name:     "formats midnight",
			expected: "00:00",
		},
		{
			name:     "formats noon",
			expected: "12:00",
		},
		{
			name:     "formats end of day",
			expected: "23:59",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Create TimeOfDay using mustNewTimeOfDay helper
			tod := mustNewTimeOfDay(tc.expected)
			assert.Equal(t, tc.expected, tod.String())
		})
	}
}

// TestTimeOfDay_Compare tests TimeOfDay comparison methods.
func TestTimeOfDay_Compare(t *testing.T) {
	tests := []struct {
		name           string
		time1          string
		time2          string
		expectedBefore bool
		expectedAfter  bool
		expectedEqual  bool
	}{
		{
			name:           "09:00 is before 10:00",
			time1:          "09:00",
			time2:          "10:00",
			expectedBefore: true,
			expectedAfter:  false,
			expectedEqual:  false,
		},
		{
			name:           "10:00 is after 09:00",
			time1:          "10:00",
			time2:          "09:00",
			expectedBefore: false,
			expectedAfter:  true,
			expectedEqual:  false,
		},
		{
			name:           "same times are equal",
			time1:          "12:30",
			time2:          "12:30",
			expectedBefore: false,
			expectedAfter:  false,
			expectedEqual:  true,
		},
		{
			name:           "minute difference - 09:30 is before 09:45",
			time1:          "09:30",
			time2:          "09:45",
			expectedBefore: true,
			expectedAfter:  false,
			expectedEqual:  false,
		},
		{
			name:           "midnight is before noon",
			time1:          "00:00",
			time2:          "12:00",
			expectedBefore: true,
			expectedAfter:  false,
			expectedEqual:  false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tod1 := mustNewTimeOfDay(tc.time1)
			tod2 := mustNewTimeOfDay(tc.time2)

			assert.Equal(t, tc.expectedBefore, tod1.Before(tod2), "Before() mismatch")
			assert.Equal(t, tc.expectedAfter, tod1.After(tod2), "After() mismatch")
			assert.Equal(t, tc.expectedEqual, tod1.Equal(tod2), "Equal() mismatch")
		})
	}
}

// TestTimeOfDay_BeforeAfter_InvalidReturnsFalse tests that invalid TimeOfDay never reports Before/After true.
func TestTimeOfDay_BeforeAfter_InvalidReturnsFalse(t *testing.T) {
	var invalid TimeOfDay // zero-value, isValid=false
	valid := mustNewTimeOfDay("12:00")

	assert.False(t, invalid.Before(valid), "invalid.Before(valid) should be false")
	assert.False(t, invalid.After(valid), "invalid.After(valid) should be false")
	assert.False(t, valid.Before(invalid), "valid.Before(invalid) should be false")
	assert.False(t, valid.After(invalid), "valid.After(invalid) should be false")
	assert.False(t, invalid.Before(invalid), "invalid.Before(invalid) should be false")
	assert.False(t, invalid.After(invalid), "invalid.After(invalid) should be false")
}

// TestTimeOfDay_MinutesSinceMidnight tests conversion to minutes.
func TestTimeOfDay_MinutesSinceMidnight(t *testing.T) {
	tests := []struct {
		name            string
		timeStr         string
		expectedMinutes int
	}{
		{
			name:            "midnight is 0 minutes",
			timeStr:         "00:00",
			expectedMinutes: 0,
		},
		{
			name:            "01:00 is 60 minutes",
			timeStr:         "01:00",
			expectedMinutes: 60,
		},
		{
			name:            "12:00 is 720 minutes",
			timeStr:         "12:00",
			expectedMinutes: 720,
		},
		{
			name:            "23:59 is 1439 minutes",
			timeStr:         "23:59",
			expectedMinutes: 1439,
		},
		{
			name:            "09:30 is 570 minutes",
			timeStr:         "09:30",
			expectedMinutes: 570,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tod := mustNewTimeOfDay(tc.timeStr)
			assert.Equal(t, tc.expectedMinutes, tod.MinutesSinceMidnight())
		})
	}
}

// TestTimeOfDay_JSONSerialization tests JSON marshaling/unmarshaling.
func TestTimeOfDay_JSONSerialization(t *testing.T) {
	tests := []struct {
		name       string
		timeStr    string
		jsonExpect string
	}{
		{
			name:       "serializes to JSON string",
			timeStr:    "09:30",
			jsonExpect: `"09:30"`,
		},
		{
			name:       "serializes midnight",
			timeStr:    "00:00",
			jsonExpect: `"00:00"`,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tod := mustNewTimeOfDay(tc.timeStr)

			// Test MarshalJSON
			jsonBytes, err := tod.MarshalJSON()
			require.NoError(t, err)
			assert.Equal(t, tc.jsonExpect, string(jsonBytes))

			// Test UnmarshalJSON
			var parsed TimeOfDay
			err = parsed.UnmarshalJSON(jsonBytes)
			require.NoError(t, err)
			assert.True(t, tod.Equal(parsed))
		})
	}
}

// TestTimeOfDay_MarshalJSON_InvalidReturnsNull tests that zero-value TimeOfDay marshals to null.
func TestTimeOfDay_MarshalJSON_InvalidReturnsNull(t *testing.T) {
	var tod TimeOfDay // zero-value, isValid=false

	jsonBytes, err := tod.MarshalJSON()
	require.NoError(t, err)
	assert.Equal(t, "null", string(jsonBytes))
}

// TestTimeOfDay_UnmarshalJSON_NullRoundtrip tests that null JSON unmarshals to invalid TimeOfDay.
func TestTimeOfDay_UnmarshalJSON_NullRoundtrip(t *testing.T) {
	var tod TimeOfDay

	err := tod.UnmarshalJSON([]byte("null"))
	require.NoError(t, err)
	assert.True(t, tod.IsZero(), "null should unmarshal to invalid/zero TimeOfDay")
}

// TestTimeOfDay_UnmarshalJSON_Errors tests JSON unmarshaling error cases.
func TestTimeOfDay_UnmarshalJSON_Errors(t *testing.T) {
	tests := []struct {
		name    string
		json    string
		errorIs error
	}{
		{
			name:    "rejects invalid JSON",
			json:    `invalid`,
			errorIs: constant.ErrTimeOfDayInvalidFormat,
		},
		{
			name:    "rejects non-string JSON",
			json:    `123`,
			errorIs: constant.ErrTimeOfDayInvalidFormat,
		},
		{
			name:    "rejects invalid time format in JSON",
			json:    `"25:00"`,
			errorIs: constant.ErrTimeOfDayInvalidFormat,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var tod TimeOfDay
			err := tod.UnmarshalJSON([]byte(tc.json))
			require.Error(t, err)
			assert.ErrorIs(t, err, tc.errorIs)
		})
	}
}

// TestTimeOfDay_IsZero tests zero value detection.
func TestTimeOfDay_IsZero(t *testing.T) {
	tests := []struct {
		name     string
		timeStr  string
		expected bool
	}{
		{
			name:     "midnight is not zero (valid value)",
			timeStr:  "00:00",
			expected: false,
		},
		{
			name:     "noon is not zero",
			timeStr:  "12:00",
			expected: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tod := mustNewTimeOfDay(tc.timeStr)
			assert.Equal(t, tc.expected, tod.IsZero())
		})
	}

	t.Run("uninitialized TimeOfDay is zero", func(t *testing.T) {
		var tod TimeOfDay
		assert.True(t, tod.IsZero())
	})
}
