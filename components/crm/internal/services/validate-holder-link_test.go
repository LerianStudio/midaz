package services

import (
	"context"
	"testing"

	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	holderlink "github.com/LerianStudio/midaz/v3/components/crm/internal/adapters/mongodb/holder-link"
	"github.com/LerianStudio/midaz/v3/pkg"
	cn "github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
)

func TestValidateHolderLinkConstraints(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockHolderLinkRepo := holderlink.NewMockRepository(ctrl)

	organizationID := libCommons.GenerateUUIDv7().String()
	aliasID := libCommons.GenerateUUIDv7()
	holderLinkID := libCommons.GenerateUUIDv7()
	linkTypePrimaryHolder := string(mmodel.LinkTypePrimaryHolder)
	linkTypeLegalRepresentative := string(mmodel.LinkTypeLegalRepresentative)

	uc := &UseCase{
		HolderLinkRepo: mockHolderLinkRepo,
	}

	testCases := []struct {
		name          string
		aliasID       uuid.UUID
		linkType      string
		mockSetup     func()
		expectError   bool
		expectedError error
	}{
		{
			name:     "Success when no existing holder link",
			aliasID:  aliasID,
			linkType: linkTypePrimaryHolder,
			mockSetup: func() {
				mockHolderLinkRepo.EXPECT().
					FindByAliasIDAndLinkType(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), false).
					Return(nil, nil)
			},
			expectError: false,
		},
		{
			name:     "Error when PRIMARY_HOLDER already exists",
			aliasID:  aliasID,
			linkType: linkTypePrimaryHolder,
			mockSetup: func() {
				mockHolderLinkRepo.EXPECT().
					FindByAliasIDAndLinkType(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), false).
					Return(&mmodel.HolderLink{
						ID:       &holderLinkID,
						AliasID:  &aliasID,
						LinkType: &linkTypePrimaryHolder,
					}, nil)
			},
			expectError:   true,
			expectedError: cn.ErrPrimaryHolderAlreadyExists,
		},
		{
			name:     "Error when duplicate link type exists",
			aliasID:  aliasID,
			linkType: linkTypeLegalRepresentative,
			mockSetup: func() {
				mockHolderLinkRepo.EXPECT().
					FindByAliasIDAndLinkType(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), false).
					Return(&mmodel.HolderLink{
						ID:       &holderLinkID,
						AliasID:  &aliasID,
						LinkType: &linkTypeLegalRepresentative,
					}, nil)
			},
			expectError:   true,
			expectedError: cn.ErrDuplicateHolderLink,
		},
		{
			name:     "Error when repository returns error",
			aliasID:  aliasID,
			linkType: linkTypePrimaryHolder,
			mockSetup: func() {
				mockHolderLinkRepo.EXPECT().
					FindByAliasIDAndLinkType(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), false).
					Return(nil, cn.ErrEntityNotFound)
			},
			expectError:   true,
			expectedError: cn.ErrEntityNotFound,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			testCase.mockSetup()

			ctx := context.Background()
			err := uc.ValidateHolderLinkConstraints(ctx, organizationID, testCase.aliasID, testCase.linkType)

			if testCase.expectError {
				assert.Error(t, err)
				if testCase.expectedError != nil {
					if validationErr, ok := err.(pkg.ValidationError); ok {
						assert.Equal(t, testCase.expectedError.Error(), validationErr.Code)
					} else if conflictErr, ok := err.(pkg.EntityConflictError); ok {
						assert.Equal(t, testCase.expectedError.Error(), conflictErr.Code)
					} else if notFoundErr, ok := err.(pkg.EntityNotFoundError); ok {
						assert.Equal(t, testCase.expectedError.Error(), notFoundErr.Code)
					} else {
						assert.Equal(t, testCase.expectedError, err)
					}
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
