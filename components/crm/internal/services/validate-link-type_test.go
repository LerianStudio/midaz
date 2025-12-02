package services

import (
	"context"
	"testing"

	"github.com/LerianStudio/midaz/v3/pkg"
	cn "github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/LerianStudio/midaz/v3/pkg/utils"
	"github.com/stretchr/testify/assert"
)

func TestValidateLinkType(t *testing.T) {
	uc := &UseCase{}

	testCases := []struct {
		name        string
		linkType    *string
		expectError bool
		errorCode   error
	}{
		{
			name:        "Success with PRIMARY_HOLDER",
			linkType:    utils.StringPtr(string(mmodel.LinkTypePrimaryHolder)),
			expectError: false,
		},
		{
			name:        "Success with LEGAL_REPRESENTATIVE",
			linkType:    utils.StringPtr(string(mmodel.LinkTypeLegalRepresentative)),
			expectError: false,
		},
		{
			name:        "Success with RESPONSIBLE_PARTY",
			linkType:    utils.StringPtr(string(mmodel.LinkTypeResponsibleParty)),
			expectError: false,
		},
		{
			name:        "Success with nil linkType (optional field)",
			linkType:    nil,
			expectError: false,
		},
		{
			name:        "Success with empty string (optional field)",
			linkType:    utils.StringPtr(""),
			expectError: false,
		},
		{
			name:        "Success with whitespace only (optional field)",
			linkType:    utils.StringPtr("   "),
			expectError: false,
		},
		{
			name:        "Error with invalid linkType value",
			linkType:    utils.StringPtr("INVALID_TYPE"),
			expectError: true,
			errorCode:   cn.ErrInvalidType,
		},
		{
			name:        "Error with empty string but valid when trimmed",
			linkType:    utils.StringPtr(" PRIMARY_HOLDER "),
			expectError: false,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			ctx := context.Background()
			err := uc.ValidateLinkType(ctx, testCase.linkType)

			if testCase.expectError {
				assert.Error(t, err)
				if testCase.errorCode != nil {
					if validationErr, ok := err.(pkg.ValidationError); ok {
						assert.Equal(t, testCase.errorCode.Error(), validationErr.Code)
					} else if conflictErr, ok := err.(pkg.EntityConflictError); ok {
						assert.Equal(t, testCase.errorCode.Error(), conflictErr.Code)
					} else {
						assert.Equal(t, testCase.errorCode, err)
					}
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
