package utils

import (
	"testing"
	"time"

	"github.com/LerianStudio/midaz/v3/pkg/pointers"
	"github.com/stretchr/testify/assert"
)

func TestParseDateTime(t *testing.T) {
	tests := []struct {
		name        string
		dateStr     string
		isEndDate   bool
		wantTime    string
		wantHasTime bool
		wantErr     bool
	}{
		{
			name:        "date only start date",
			dateStr:     "2023-10-25",
			isEndDate:   false,
			wantTime:    "2023-10-25 00:00:00",
			wantHasTime: false,
			wantErr:     false,
		},
		{
			name:        "date only end date",
			dateStr:     "2023-10-25",
			isEndDate:   true,
			wantTime:    "2023-10-25 23:59:59",
			wantHasTime: false,
			wantErr:     false,
		},
		{
			name:        "RFC3339 format",
			dateStr:     "2023-10-25T13:30:45Z",
			isEndDate:   false,
			wantTime:    "2023-10-25 13:30:45",
			wantHasTime: true,
			wantErr:     false,
		},
		{
			name:        "ISO format without timezone",
			dateStr:     "2023-10-25T13:30:45",
			isEndDate:   false,
			wantTime:    "2023-10-25 13:30:45",
			wantHasTime: true,
			wantErr:     false,
		},
		{
			name:        "space separated format",
			dateStr:     "2023-10-25 13:30:45",
			isEndDate:   false,
			wantTime:    "2023-10-25 13:30:45",
			wantHasTime: true,
			wantErr:     false,
		},
		{
			name:        "invalid format",
			dateStr:     "invalid-date",
			isEndDate:   false,
			wantTime:    "",
			wantHasTime: false,
			wantErr:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, hasTime, err := ParseDateTime(tt.dateStr, tt.isEndDate)

			if tt.wantErr {
				assert.Error(t, err)
				return
			}

			assert.NoError(t, err)
			assert.Equal(t, tt.wantHasTime, hasTime)

			expectedTime, _ := time.Parse("2006-01-02 15:04:05", tt.wantTime)
			assert.True(t, got.Equal(expectedTime),
				"expected %s, got %s", tt.wantTime, got.Format("2006-01-02 15:04:05"))
		})
	}
}

func TestNormalizeDateTime(t *testing.T) {
	tests := []struct {
		name     string
		date     time.Time
		days     *int
		endOfDay bool
		want     string
	}{
		{
			name:     "start date normalized (00:00:00)",
			date:     time.Date(2023, 10, 25, 0, 0, 0, 0, time.UTC),
			days:     nil,
			endOfDay: false,
			want:     "2023-10-25 00:00:00",
		},
		{
			name:     "start date with specific time",
			date:     time.Date(2023, 10, 25, 13, 30, 45, 0, time.UTC),
			days:     nil,
			endOfDay: false,
			want:     "2023-10-25 13:30:45",
		},
		{
			name:     "end date normalized (23:59:59)",
			date:     time.Date(2023, 10, 25, 23, 59, 59, 0, time.UTC),
			days:     nil,
			endOfDay: true,
			want:     "2023-10-25 23:59:59",
		},
		{
			name:     "end date with specific time",
			date:     time.Date(2023, 10, 25, 13, 30, 45, 0, time.UTC),
			days:     nil,
			endOfDay: true,
			want:     "2023-10-25 13:30:45",
		},
		{
			name:     "end date at midnight (should normalize to end of day)",
			date:     time.Date(2023, 10, 25, 0, 0, 0, 0, time.UTC),
			days:     nil,
			endOfDay: true,
			want:     "2023-10-25 23:59:59",
		},
		{
			name:     "with days offset",
			date:     time.Date(2023, 10, 25, 13, 30, 45, 0, time.UTC),
			days:     pointers.Int(2),
			endOfDay: false,
			want:     "2023-10-27 13:30:45",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := NormalizeDateTime(tt.date, tt.days, tt.endOfDay)
			assert.Equal(t, tt.want, got)
		})
	}
}
