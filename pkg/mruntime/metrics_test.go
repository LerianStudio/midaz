package mruntime

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestSanitizeLabel tests the sanitizeLabel function.
func TestSanitizeLabel(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "empty string",
			input:    "",
			expected: "",
		},
		{
			name:     "short string",
			input:    "component",
			expected: "component",
		},
		{
			name:     "exactly max length",
			input:    strings.Repeat("a", maxLabelLength),
			expected: strings.Repeat("a", maxLabelLength),
		},
		{
			name:     "exceeds max length",
			input:    strings.Repeat("b", maxLabelLength+10),
			expected: strings.Repeat("b", maxLabelLength),
		},
		{
			name:     "much longer than max",
			input:    strings.Repeat("c", 200),
			expected: strings.Repeat("c", maxLabelLength),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := sanitizeLabel(tt.input)
			assert.Equal(t, tt.expected, result)
			assert.LessOrEqual(t, len(result), maxLabelLength)
		})
	}
}
