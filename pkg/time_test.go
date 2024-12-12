package pkg

import (
	"testing"
	"time"
)

func TestIsValidDate(t *testing.T) {
	tests := []struct {
		date     string
		expected bool
	}{
		{"2023-01-01", true},
		{"2023-13-01", false},
		{"invalid-date", false},
	}

	for _, test := range tests {
		result := IsValidDate(test.date)
		if result != test.expected {
			t.Errorf("IsValidDate(%s) = %v; want %v", test.date, result, test.expected)
		}
	}
}

func TestIsInitialDateBeforeFinalDate(t *testing.T) {
	initial := time.Date(2023, 1, 1, 0, 0, 0, 0, time.UTC)
	final := time.Date(2023, 12, 31, 0, 0, 0, 0, time.UTC)

	if !IsInitialDateBeforeFinalDate(initial, final) {
		t.Errorf("IsInitialDateBeforeFinalDate(%v, %v) = false; want true", initial, final)
	}

	if IsInitialDateBeforeFinalDate(final, initial) {
		t.Errorf("IsInitialDateBeforeFinalDate(%v, %v) = true; want false", final, initial)
	}
}

func TestIsDateRangeWithinMonthLimit(t *testing.T) {
	initial := time.Date(2023, 1, 1, 0, 0, 0, 0, time.UTC)
	final := time.Date(2023, 3, 1, 0, 0, 0, 0, time.UTC)
	limit := 2

	if !IsDateRangeWithinMonthLimit(initial, final, limit) {
		t.Errorf("IsDateRangeWithinMonthLimit(%v, %v, %d) = false; want true", initial, final, limit)
	}

	final = time.Date(2023, 4, 1, 0, 0, 0, 0, time.UTC)
	if IsDateRangeWithinMonthLimit(initial, final, limit) {
		t.Errorf("IsDateRangeWithinMonthLimit(%v, %v, %d) = true; want false", initial, final, limit)
	}
}

func TestNormalizeDate(t *testing.T) {
	date := time.Date(2023, 1, 1, 0, 0, 0, 0, time.UTC)
	days := 5
	expected := "2023-01-06"

	result := NormalizeDate(date, &days)
	if result != expected {
		t.Errorf("NormalizeDate(%v, %d) = %s; want %s", date, days, result, expected)
	}

	expected = "2023-01-01"
	result = NormalizeDate(date, nil)
	if result != expected {
		t.Errorf("NormalizeDate(%v, nil) = %s; want %s", date, result, expected)
	}
}
