package mmodel

import (
	"encoding/json"
	"time"
)

// Date is a custom date type that accepts both "2025-01-23" (date only)
// and "2025-01-23T00:00:00Z" (RFC3339) formats during JSON unmarshaling.
// It outputs date-only format ("2025-01-23") during marshaling.
//
// swagger:model Date
// @Description Date in YYYY-MM-DD format (e.g., "2025-06-15") or null
// @type string
// @format date
// @example "2025-06-15"
type Date struct {
	time.Time
}

// dateFormats lists the formats we accept, in order of preference
var dateFormats = []string{
	time.RFC3339,          // "2006-01-02T15:04:05Z07:00"
	"2006-01-02T15:04:05", // RFC3339 without timezone
	"2006-01-02",          // Date only
}

// UnmarshalJSON implements json.Unmarshaler with flexible date parsing.
// Accepts both "2025-01-23" and "2025-01-23T00:00:00Z" formats.
func (d *Date) UnmarshalJSON(data []byte) error {
	var s string
	if err := json.Unmarshal(data, &s); err != nil {
		return err
	}

	if s == "" {
		d.Time = time.Time{}
		return nil
	}

	var parseErr error

	for _, format := range dateFormats {
		t, err := time.Parse(format, s)
		if err == nil {
			// Normalize to UTC midnight to ensure consistency with date-only MarshalJSON
			d.Time = time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)
			return nil
		}

		parseErr = err
	}

	return parseErr
}

// MarshalJSON implements json.Marshaler, outputting date-only format ("2006-01-02").
func (d Date) MarshalJSON() ([]byte, error) {
	if d.IsZero() {
		return json.Marshal(nil)
	}

	return json.Marshal(d.Format("2006-01-02"))
}

// Before reports whether d is before u (date-only comparison).
func (d Date) Before(u Date) bool {
	dy, dm, dd := d.Date()
	uy, um, ud := u.Date()

	if dy != uy {
		return dy < uy
	}

	if dm != um {
		return dm < um
	}

	return dd < ud
}

// After reports whether d is after u (date-only comparison).
func (d Date) After(u Date) bool {
	return u.Before(d)
}
