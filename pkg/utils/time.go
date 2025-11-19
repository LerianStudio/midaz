package utils

import (
	"fmt"
	"time"
)

// IsValidDate checks if the provided date string is in the format "YYYY-MM-DD".
func IsValidDate(date string) bool {
	_, err := time.Parse("2006-01-02", date)
	return err == nil
}

// IsInitialDateBeforeFinalDate checks if the initial date is before or equal to the final date.
func IsInitialDateBeforeFinalDate(initial, final time.Time) bool {
	return !initial.After(final)
}

// IsDateRangeWithinMonthLimit checks if the date range is within the permitted range in months.
func IsDateRangeWithinMonthLimit(initial, final time.Time, limit int) bool {
	limitDate := initial.AddDate(0, limit, 0)
	return !final.After(limitDate)
}

// NormalizeDate normalizes a date adding or subtracting days without time to make it match the query requirements and string format.
func NormalizeDate(date time.Time, days *int) string {
	if days != nil {
		return date.AddDate(0, 0, *days).Format("2006-01-02")
	}

	return date.Format("2006-01-02")
}

// NormalizeDateTime normalizes a date adding or subtracting days with time.
// If the date already has a specific time (not at start/end of day), it preserves it.
// Otherwise, it normalizes to start (00:00:00) or end (23:59:59) of day.
func NormalizeDateTime(date time.Time, days *int, endOfDay bool) string {
	if days != nil {
		date = date.AddDate(0, 0, *days)
	}

	hour, min, sec := date.Hour(), date.Minute(), date.Second()
	isNormalized := (hour == 0 && min == 0 && sec == 0) || (hour == 23 && min == 59 && sec == 59)

	if isNormalized || (endOfDay && hour == 0 && min == 0 && sec == 0) {
		if endOfDay {
			date = time.Date(date.Year(), date.Month(), date.Day(), 23, 59, 59, 999999999, date.Location())
		} else {
			date = time.Date(date.Year(), date.Month(), date.Day(), 0, 0, 0, 0, date.Location())
		}
	}

	return date.Format("2006-01-02 15:04:05")
}

// ParseDateTime parses a date string that can be either:
// - Date only (YYYY-MM-DD): returns time with 00:00:00 for start, 23:59:59 for end
// - Date and time (ISO 8601 formats like YYYY-MM-DDTHH:MM:SS): returns exact time
// Returns the parsed time and a boolean indicating if time was specified
func ParseDateTime(dateStr string, isEndDate bool) (time.Time, bool, error) {
	formats := []string{
		time.RFC3339,
		"2006-01-02T15:04:05",
		"2006-01-02T15:04:05Z",
		"2006-01-02 15:04:05",
	}

	for _, format := range formats {
		if t, err := time.Parse(format, dateStr); err == nil {
			return t, true, nil
		}
	}

	if t, err := time.Parse("2006-01-02", dateStr); err == nil {
		if isEndDate {
			t = time.Date(t.Year(), t.Month(), t.Day(), 23, 59, 59, 999999999, t.Location())
		} else {
			t = time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, t.Location())
		}
		return t, false, nil
	}

	return time.Time{}, false, fmt.Errorf("invalid date format: %s", dateStr)
}
