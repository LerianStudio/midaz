package services

import (
	"context"
	"errors"
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
			errorCode:   cn.ErrInvalidLinkType,
		},
		{
			name:        "Success with whitespace-padded PRIMARY_HOLDER",
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
					var validationErr pkg.ValidationError
					var conflictErr pkg.EntityConflictError

					switch {
					case errors.As(err, &validationErr):
						assert.Equal(t, testCase.errorCode.Error(), validationErr.Code)
					case errors.As(err, &conflictErr):
						assert.Equal(t, testCase.errorCode.Error(), conflictErr.Code)
					default:
						assert.ErrorIs(t, err, testCase.errorCode)
					}
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
