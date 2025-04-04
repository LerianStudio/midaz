package utils_test

import (
	"testing"

	"github.com/LerianStudio/midaz/sdks/go-sdk/pkg/utils"
	"github.com/stretchr/testify/assert"
)

func TestFormatAmount(t *testing.T) {
	testCases := []struct {
		name     string
		amount   int64
		scale    int
		expected string
	}{
		{
			name:     "Positive integer",
			amount:   100,
			scale:    0,
			expected: "100",
		},
		{
			name:     "Positive decimal",
			amount:   12345,
			scale:    2,
			expected: "123.45",
		},
		{
			name:     "Negative decimal",
			amount:   -5075,
			scale:    2,
			expected: "-50.75",
		},
		{
			name:     "Zero amount",
			amount:   0,
			scale:    2,
			expected: "0.00",
		},
		{
			name:     "Small decimal",
			amount:   5,
			scale:    2,
			expected: "0.05",
		},
		{
			name:     "Very small decimal",
			amount:   1,
			scale:    5,
			expected: "0.00001",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := utils.FormatAmount(tc.amount, tc.scale)
			assert.Equal(t, tc.expected, result)
		})
	}
}
