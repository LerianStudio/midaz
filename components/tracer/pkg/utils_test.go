// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package pkg

import (
	"math"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSafeIntToInt32(t *testing.T) {
	tests := []struct {
		name        string
		input       int
		expected    int32
		expectError bool
	}{
		{
			name:        "zero value",
			input:       0,
			expected:    0,
			expectError: false,
		},
		{
			name:        "positive value within range",
			input:       42,
			expected:    42,
			expectError: false,
		},
		{
			name:        "negative value within range",
			input:       -100,
			expected:    -100,
			expectError: false,
		},
		{
			name:        "max int32 value",
			input:       math.MaxInt32,
			expected:    math.MaxInt32,
			expectError: false,
		},
		{
			name:        "min int32 value",
			input:       math.MinInt32,
			expected:    math.MinInt32,
			expectError: false,
		},
		{
			name:        "overflow - exceeds max int32",
			input:       math.MaxInt32 + 1,
			expected:    0,
			expectError: true,
		},
		{
			name:        "overflow - below min int32",
			input:       math.MinInt32 - 1,
			expected:    0,
			expectError: true,
		},
		{
			name:        "large positive overflow",
			input:       math.MaxInt64,
			expected:    0,
			expectError: true,
		},
		{
			name:        "large negative overflow",
			input:       math.MinInt64,
			expected:    0,
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := SafeIntToInt32(tt.input)

			if tt.expectError {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), "integer overflow")
				assert.Equal(t, int32(0), result)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expected, result)
			}
		})
	}
}
