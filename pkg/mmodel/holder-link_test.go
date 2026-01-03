package mmodel

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestIsValidLinkType(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		linkTypeStr string
		want        bool
	}{
		{
			name:        "PRIMARY_HOLDER is valid",
			linkTypeStr: "PRIMARY_HOLDER",
			want:        true,
		},
		{
			name:        "LEGAL_REPRESENTATIVE is valid",
			linkTypeStr: "LEGAL_REPRESENTATIVE",
			want:        true,
		},
		{
			name:        "RESPONSIBLE_PARTY is valid",
			linkTypeStr: "RESPONSIBLE_PARTY",
			want:        true,
		},
		{
			name:        "empty string is invalid",
			linkTypeStr: "",
			want:        false,
		},
		{
			name:        "lowercase primary_holder is invalid",
			linkTypeStr: "primary_holder",
			want:        false,
		},
		{
			name:        "unknown type is invalid",
			linkTypeStr: "UNKNOWN_TYPE",
			want:        false,
		},
		{
			name:        "partial match is invalid",
			linkTypeStr: "PRIMARY",
			want:        false,
		},
		{
			name:        "whitespace is invalid",
			linkTypeStr: " PRIMARY_HOLDER ",
			want:        false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := IsValidLinkType(tt.linkTypeStr)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestGetValidLinkTypes(t *testing.T) {
	t.Parallel()
	validTypes := GetValidLinkTypes()

	assert.Len(t, validTypes, 3, "should return exactly 3 valid link types")

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

	for linkType, found := range expectedTypes {
		assert.True(t, found, "expected link type not found: %s", linkType)
	}
}

func TestLinkTypeConstants(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		linkType LinkType
		want     string
	}{
		{
			name:     "LinkTypePrimaryHolder constant value",
			linkType: LinkTypePrimaryHolder,
			want:     "PRIMARY_HOLDER",
		},
		{
			name:     "LinkTypeLegalRepresentative constant value",
			linkType: LinkTypeLegalRepresentative,
			want:     "LEGAL_REPRESENTATIVE",
		},
		{
			name:     "LinkTypeResponsibleParty constant value",
			linkType: LinkTypeResponsibleParty,
			want:     "RESPONSIBLE_PARTY",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tt.want, string(tt.linkType))
		})
	}
}

func TestValidLinkTypeMapping(t *testing.T) {
	t.Parallel()
	assert.True(t, ValidLinkTypeMapping[LinkTypePrimaryHolder], "PRIMARY_HOLDER should be valid")
	assert.True(t, ValidLinkTypeMapping[LinkTypeLegalRepresentative], "LEGAL_REPRESENTATIVE should be valid")
	assert.True(t, ValidLinkTypeMapping[LinkTypeResponsibleParty], "RESPONSIBLE_PARTY should be valid")

	assert.False(t, ValidLinkTypeMapping["INVALID"], "INVALID should not be in mapping")
	assert.False(t, ValidLinkTypeMapping[""], "empty string should not be in mapping")
}
