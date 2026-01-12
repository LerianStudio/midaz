package mmodel

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDate_UnmarshalJSON(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{
			name:    "RFC3339 format with timezone",
			input:   `"2025-01-23T15:04:05Z"`,
			wantErr: false,
		},
		{
			name:    "RFC3339 format with offset",
			input:   `"2025-01-23T15:04:05+03:00"`,
			wantErr: false,
		},
		{
			name:    "RFC3339 format without timezone",
			input:   `"2025-01-23T15:04:05"`,
			wantErr: false,
		},
		{
			name:    "Date only format",
			input:   `"2025-01-23"`,
			wantErr: false,
		},
		{
			name:    "Empty string",
			input:   `""`,
			wantErr: false,
		},
		{
			name:    "Invalid format",
			input:   `"not-a-date"`,
			wantErr: true,
		},
		{
			name:    "Null value",
			input:   `null`,
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var d Date
			err := json.Unmarshal([]byte(tt.input), &d)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestDate_UnmarshalJSON_DateOnly_ParsesCorrectly(t *testing.T) {
	input := `"2025-06-15"`
	var d Date
	err := json.Unmarshal([]byte(input), &d)
	require.NoError(t, err)

	assert.Equal(t, 2025, d.Year())
	assert.Equal(t, time.June, d.Month())
	assert.Equal(t, 15, d.Day())
}

func TestDate_MarshalJSON(t *testing.T) {
	d := Date{Time: time.Date(2025, 6, 15, 10, 30, 0, 0, time.UTC)}
	data, err := json.Marshal(d)
	require.NoError(t, err)

	// Should output date-only format (time component is omitted)
	assert.Equal(t, `"2025-06-15"`, string(data))
}

func TestDate_MarshalJSON_Zero(t *testing.T) {
	var d Date
	data, err := json.Marshal(d)
	require.NoError(t, err)

	assert.Equal(t, "null", string(data))
}

func TestDate_Before(t *testing.T) {
	d1 := Date{Time: time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)}
	d2 := Date{Time: time.Date(2025, 6, 1, 0, 0, 0, 0, time.UTC)}

	assert.True(t, d1.Before(d2))
	assert.False(t, d2.Before(d1))
}

func TestDate_After(t *testing.T) {
	d1 := Date{Time: time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)}
	d2 := Date{Time: time.Date(2025, 6, 1, 0, 0, 0, 0, time.UTC)}

	assert.True(t, d2.After(d1))
	assert.False(t, d1.After(d2))
}

func TestDate_IsZero(t *testing.T) {
	var d Date
	assert.True(t, d.IsZero())

	d = Date{Time: time.Now()}
	assert.False(t, d.IsZero())
}
