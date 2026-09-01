// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package validation

import (
	"errors"
	"strings"
	"testing"
	"time"
)

func TestValidateDateRange(t *testing.T) {
	tests := []struct {
		name        string
		startDate   time.Time
		endDate     time.Time
		startField  string
		endField    string
		wantErr     bool
		errContains string
	}{
		{
			name:       "valid range - start before end",
			startDate:  time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
			endDate:    time.Date(2026, 1, 31, 0, 0, 0, 0, time.UTC),
			startField: "start_date",
			endField:   "end_date",
			wantErr:    false,
		},
		{
			name:       "valid range - equal dates allowed",
			startDate:  time.Date(2026, 1, 15, 0, 0, 0, 0, time.UTC),
			endDate:    time.Date(2026, 1, 15, 0, 0, 0, 0, time.UTC),
			startField: "start_date",
			endField:   "end_date",
			wantErr:    false,
		},
		{
			name:        "invalid range - start after end",
			startDate:   time.Date(2026, 1, 31, 0, 0, 0, 0, time.UTC),
			endDate:     time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
			startField:  "start_date",
			endField:    "end_date",
			wantErr:     true,
			errContains: "must not be after",
		},
		{
			name:       "zero start date - skips validation",
			startDate:  time.Time{},
			endDate:    time.Date(2026, 1, 31, 0, 0, 0, 0, time.UTC),
			startField: "start_date",
			endField:   "end_date",
			wantErr:    false,
		},
		{
			name:       "zero end date - skips validation",
			startDate:  time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
			endDate:    time.Time{},
			startField: "start_date",
			endField:   "end_date",
			wantErr:    false,
		},
		{
			name:       "both zero dates - skips validation",
			startDate:  time.Time{},
			endDate:    time.Time{},
			startField: "start_date",
			endField:   "end_date",
			wantErr:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateDateRange(tt.startDate, tt.endDate, tt.startField, tt.endField)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateDateRange() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.errContains != "" && err != nil {
				if !strings.Contains(err.Error(), tt.errContains) {
					t.Errorf("ValidateDateRange() error = %q, want error containing %q", err.Error(), tt.errContains)
				}
			}
		})
	}
}

func TestParseRFC3339Timestamp(t *testing.T) {
	tests := []struct {
		name      string
		value     string
		fieldName string
		wantZero  bool
		wantErr   bool
	}{
		{
			name:      "valid RFC3339 with Z",
			value:     "2026-01-28T10:30:00Z",
			fieldName: "start_date",
			wantZero:  false,
			wantErr:   false,
		},
		{
			name:      "valid RFC3339 with offset",
			value:     "2026-01-28T10:30:00-03:00",
			fieldName: "start_date",
			wantZero:  false,
			wantErr:   false,
		},
		{
			name:      "empty string returns zero time",
			value:     "",
			fieldName: "start_date",
			wantZero:  true,
			wantErr:   false,
		},
		{
			name:      "date only - invalid",
			value:     "2026-01-28",
			fieldName: "start_date",
			wantZero:  false,
			wantErr:   true,
		},
		{
			name:      "missing timezone - invalid",
			value:     "2026-01-28T10:30:00",
			fieldName: "start_date",
			wantZero:  false,
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseRFC3339Timestamp(tt.value, tt.fieldName)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseRFC3339Timestamp() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantErr && err != nil {
				var parseErr *RFC3339ParseError
				if errors.As(err, &parseErr) {
					if parseErr.Field != tt.fieldName {
						t.Errorf("RFC3339ParseError.Field = %q, want %q", parseErr.Field, tt.fieldName)
					}
					if parseErr.Value != tt.value {
						t.Errorf("RFC3339ParseError.Value = %q, want %q", parseErr.Value, tt.value)
					}
					if parseErr.Wrapped == nil {
						t.Error("RFC3339ParseError.Wrapped should not be nil")
					}
				}
			}
			if tt.wantZero && !got.IsZero() {
				t.Errorf("ParseRFC3339Timestamp() = %v, want zero time", got)
			}
			if !tt.wantZero && !tt.wantErr && got.IsZero() {
				t.Errorf("ParseRFC3339Timestamp() returned zero time, want non-zero")
			}
		})
	}
}
