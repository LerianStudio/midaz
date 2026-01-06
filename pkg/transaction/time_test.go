package transaction

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestTransactionDate_UnmarshalJSON(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		input       string
		expectError bool
		isZero      bool
	}{
		{
			name:        "RFC3339Nano format",
			input:       `"2024-01-15T10:30:45.123456789Z"`,
			expectError: false,
			isZero:      false,
		},
		{
			name:        "RFC3339 format",
			input:       `"2024-01-15T10:30:45Z"`,
			expectError: false,
			isZero:      false,
		},
		{
			name:        "ISO 8601 with milliseconds",
			input:       `"2024-01-15T10:30:45.000Z"`,
			expectError: false,
			isZero:      false,
		},
		{
			name:        "ISO 8601 without milliseconds",
			input:       `"2024-01-15T10:30:45Z"`,
			expectError: false,
			isZero:      false,
		},
		{
			name:        "ISO 8601 without timezone",
			input:       `"2024-01-15T10:30:45"`,
			expectError: false,
			isZero:      false,
		},
		{
			name:        "Date only format",
			input:       `"2024-01-15"`,
			expectError: false,
			isZero:      false,
		},
		{
			name:        "null value",
			input:       `"null"`,
			expectError: false,
			isZero:      true,
		},
		{
			name:        "empty string",
			input:       `""`,
			expectError: false,
			isZero:      true,
		},
		{
			name:        "invalid date format",
			input:       `"15-01-2024"`,
			expectError: true,
			isZero:      false,
		},
		{
			name:        "invalid format - text",
			input:       `"invalid-date"`,
			expectError: true,
			isZero:      false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var td TransactionDate
			err := json.Unmarshal([]byte(tt.input), &td)

			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.isZero, td.IsZero())
			}
		})
	}
}

func TestTransactionDate_MarshalJSON(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		date     TransactionDate
		expected string
	}{
		{
			name:     "zero value returns null",
			date:     TransactionDate{},
			expected: "null",
		},
		{
			name:     "valid date returns RFC3339 format",
			date:     TransactionDate(time.Date(2024, 1, 15, 10, 30, 45, 0, time.UTC)),
			expected: `"2024-01-15T10:30:45Z"`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			result, err := json.Marshal(tt.date)
			assert.NoError(t, err)
			assert.Equal(t, tt.expected, string(result))
		})
	}
}

func TestTransactionDate_Time(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		date     TransactionDate
		expected time.Time
	}{
		{
			name:     "zero value",
			date:     TransactionDate{},
			expected: time.Time{},
		},
		{
			name:     "valid date",
			date:     TransactionDate(time.Date(2024, 1, 15, 10, 30, 45, 0, time.UTC)),
			expected: time.Date(2024, 1, 15, 10, 30, 45, 0, time.UTC),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			result := tt.date.Time()
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestTransactionDate_IsZero(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		date TransactionDate
		want bool
	}{
		{
			name: "zero value",
			date: TransactionDate{},
			want: true,
		},
		{
			name: "non-zero value",
			date: TransactionDate(time.Date(2024, 1, 15, 10, 30, 45, 0, time.UTC)),
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := tt.date.IsZero()
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestTransactionDate_After(t *testing.T) {
	t.Parallel()

	baseTime := time.Date(2024, 1, 15, 10, 30, 45, 0, time.UTC)

	tests := []struct {
		name string
		date TransactionDate
		t    time.Time
		want bool
	}{
		{
			name: "date after reference time",
			date: TransactionDate(time.Date(2024, 1, 16, 10, 30, 45, 0, time.UTC)),
			t:    baseTime,
			want: true,
		},
		{
			name: "date before reference time",
			date: TransactionDate(time.Date(2024, 1, 14, 10, 30, 45, 0, time.UTC)),
			t:    baseTime,
			want: false,
		},
		{
			name: "date equal to reference time",
			date: TransactionDate(baseTime),
			t:    baseTime,
			want: false,
		},
		{
			name: "zero date is not after reference time",
			date: TransactionDate{},
			t:    baseTime,
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := tt.date.After(tt.t)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestTransactionDate_UnmarshalJSON_ZeroTime(t *testing.T) {
	t.Parallel()

	input := `"0001-01-01T00:00:00Z"`
	var td TransactionDate
	err := json.Unmarshal([]byte(input), &td)

	assert.NoError(t, err)
	assert.True(t, td.IsZero())
}

func TestTransactionDate_RoundTrip(t *testing.T) {
	t.Parallel()

	originalTime := time.Date(2024, 6, 20, 14, 30, 0, 0, time.UTC)
	original := TransactionDate(originalTime)

	marshaled, err := json.Marshal(original)
	assert.NoError(t, err)

	var unmarshaled TransactionDate
	err = json.Unmarshal(marshaled, &unmarshaled)
	assert.NoError(t, err)

	assert.Equal(t, original.Time().Unix(), unmarshaled.Time().Unix())
}
