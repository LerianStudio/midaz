package pkg

import (
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

// NormalizeDate normalizes a date adding or subtracting days to make it match the query requirements and string format.
func NormalizeDate(date time.Time, days *int) string {
	if days != nil {
		return date.AddDate(0, 0, *days).Format("2006-01-02")
	}

	return date.Format("2006-01-02")
}
