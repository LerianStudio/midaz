package mmodel

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestIsValidLinkType(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected bool
	}{
		{
			name:     "valid PRIMARY_HOLDER",
			input:    "PRIMARY_HOLDER",
			expected: true,
		},
		{
			name:     "valid LEGAL_REPRESENTATIVE",
			input:    "LEGAL_REPRESENTATIVE",
			expected: true,
		},
		{
			name:     "valid RESPONSIBLE_PARTY",
			input:    "RESPONSIBLE_PARTY",
			expected: true,
		},
		{
			name:     "invalid empty string",
			input:    "",
			expected: false,
		},
		{
			name:     "invalid lowercase primary_holder",
			input:    "primary_holder",
			expected: false,
		},
		{
			name:     "invalid random string",
			input:    "INVALID_TYPE",
			expected: false,
		},
		{
			name:     "invalid partial match",
			input:    "PRIMARY",
			expected: false,
		},
		{
			name:     "invalid with extra characters",
			input:    "PRIMARY_HOLDER_EXTRA",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsValidLinkType(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestGetValidLinkTypes(t *testing.T) {
	validTypes := GetValidLinkTypes()

	// Should return exactly 3 valid types
	assert.Len(t, validTypes, 3)

	// All returned types should be valid
	expectedTypes := map[string]bool{
		"PRIMARY_HOLDER":       false,
		"LEGAL_REPRESENTATIVE": false,
		"RESPONSIBLE_PARTY":    false,
	}

	for _, linkType := range validTypes {
		_, exists := expectedTypes[linkType]
		assert.True(t, exists, "unexpected link type: %s", linkType)
		expectedTypes[linkType] = true
	}

	// Verify all expected types were found
	for linkType, found := range expectedTypes {
		assert.True(t, found, "missing expected link type: %s", linkType)
	}
}

func TestLinkTypeConstants(t *testing.T) {
	// Verify constants have expected values
	assert.Equal(t, LinkType("PRIMARY_HOLDER"), LinkTypePrimaryHolder)
	assert.Equal(t, LinkType("LEGAL_REPRESENTATIVE"), LinkTypeLegalRepresentative)
	assert.Equal(t, LinkType("RESPONSIBLE_PARTY"), LinkTypeResponsibleParty)
}

func TestValidLinkTypeMapping(t *testing.T) {
	// Verify mapping contains all valid types
	assert.True(t, ValidLinkTypeMapping[LinkTypePrimaryHolder])
	assert.True(t, ValidLinkTypeMapping[LinkTypeLegalRepresentative])
	assert.True(t, ValidLinkTypeMapping[LinkTypeResponsibleParty])

	// Verify mapping returns false for invalid types
	assert.False(t, ValidLinkTypeMapping[LinkType("INVALID")])
	assert.False(t, ValidLinkTypeMapping[LinkType("")])
}
