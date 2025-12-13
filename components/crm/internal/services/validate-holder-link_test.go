package services

import (
	"context"
	"errors"
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

				var validationErr pkg.ValidationError
				var conflictErr pkg.EntityConflictError
				var notFoundErr pkg.EntityNotFoundError

				switch {
				case errors.As(err, &validationErr):
					assert.Equal(t, testCase.expectedError.Error(), validationErr.Code)
				case errors.As(err, &conflictErr):
					assert.Equal(t, testCase.expectedError.Error(), conflictErr.Code)
				case errors.As(err, &notFoundErr):
					assert.Equal(t, testCase.expectedError.Error(), notFoundErr.Code)
				default:
					assert.ErrorIs(t, err, testCase.expectedError)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
