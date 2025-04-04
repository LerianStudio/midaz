// Package conversion provides utilities for converting between different data formats
// and creating human-readable representations of Midaz SDK models.
package conversion

import (
	"time"
)

// ConvertToISODate formats a time.Time as an ISO date string (YYYY-MM-DD).
//
// Example:
//
//	isoDate := conversion.ConvertToISODate(time.Now())
//	// Result: "2025-04-02"
func ConvertToISODate(t time.Time) string {
	return t.Format("2006-01-02")
}

// ConvertToISODateTime formats a time.Time as an ISO date-time string (YYYY-MM-DDThh:mm:ssZ).
//
// Example:
//
//	isoDateTime := conversion.ConvertToISODateTime(time.Now())
//	// Result: "2025-04-02T15:04:05Z"
func ConvertToISODateTime(t time.Time) string {
	return t.Format(time.RFC3339)
}
