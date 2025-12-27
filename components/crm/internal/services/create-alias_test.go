package services

import (
	"context"
	"errors"
	"testing"

	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	"github.com/LerianStudio/midaz/v3/components/crm/internal/adapters/mongodb/alias"
	"github.com/LerianStudio/midaz/v3/components/crm/internal/adapters/mongodb/holder"
	holderlink "github.com/LerianStudio/midaz/v3/components/crm/internal/adapters/mongodb/holder-link"
	"github.com/LerianStudio/midaz/v3/pkg"
	cn "github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/LerianStudio/midaz/v3/pkg/utils"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

func TestCreateAlias(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockHolderRepo := holder.NewMockRepository(ctrl)
	mockAliasRepo := alias.NewMockRepository(ctrl)
	mockHolderLinkRepo := holderlink.NewMockRepository(ctrl)

	holderID := libCommons.GenerateUUIDv7()
	id := libCommons.GenerateUUIDv7()
	accountID := libCommons.GenerateUUIDv7().String()
	ledgerID := libCommons.GenerateUUIDv7().String()
	holderLinkID := libCommons.GenerateUUIDv7()
	holderDocument := "90217469051"
	linkTypePrimaryHolder := string(mmodel.LinkTypePrimaryHolder)

	uc := &UseCase{
		HolderRepo:     mockHolderRepo,
		AliasRepo:      mockAliasRepo,
		HolderLinkRepo: mockHolderLinkRepo,
	}

	testCases := []struct {
		name           string
		holderID       uuid.UUID
		input          *mmodel.CreateAliasInput
		mockSetup      func()
		expectedErr    error
		expectedResult *mmodel.Alias
	}{
		{
			name:     "Success with required fields provided (no LinkType)",
			holderID: holderID,
			input: &mmodel.CreateAliasInput{
				LedgerID:  ledgerID,
				AccountID: accountID,
			},
			mockSetup: func() {
				mockHolderRepo.EXPECT().
					Find(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
					Return(&mmodel.Holder{
						ID:       &holderID,
						Document: &holderDocument,
					}, nil)

				mockAliasRepo.EXPECT().
					Create(gomock.Any(), gomock.Any(), gomock.Any()).
					Return(&mmodel.Alias{
						ID:        &id,
						Document:  &holderDocument,
						AccountID: &accountID,
						LedgerID:  &ledgerID,
					}, nil)
			},
			expectedErr: nil,
			expectedResult: &mmodel.Alias{
				ID:        &id,
				Document:  &holderDocument,
				AccountID: &accountID,
				LedgerID:  &ledgerID,
			},
		},
		{
			name:     "Success with LinkType provided",
			holderID: holderID,
			input: &mmodel.CreateAliasInput{
				LedgerID:  ledgerID,
				AccountID: accountID,
				LinkType:  &linkTypePrimaryHolder,
			},
			mockSetup: func() {
				mockHolderRepo.EXPECT().
					Find(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
					Return(&mmodel.Holder{
						ID:       &holderID,
						Document: &holderDocument,
					}, nil)

				mockAliasRepo.EXPECT().
					Create(gomock.Any(), gomock.Any(), gomock.Any()).
					Return(&mmodel.Alias{
						ID:        &id,
						Document:  &holderDocument,
						AccountID: &accountID,
						LedgerID:  &ledgerID,
					}, nil)

				mockHolderLinkRepo.EXPECT().
					FindByAliasIDAndLinkType(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), false).
					Return(nil, nil)

				mockHolderLinkRepo.EXPECT().
					Create(gomock.Any(), gomock.Any(), gomock.Any()).
					Return(&mmodel.HolderLink{
						ID:       &holderLinkID,
						HolderID: &holderID,
						AliasID:  &id,
						LinkType: &linkTypePrimaryHolder,
					}, nil)

				mockAliasRepo.EXPECT().
					Update(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
					Return(&mmodel.Alias{
						ID:        &id,
						Document:  &holderDocument,
						AccountID: &accountID,
						LedgerID:  &ledgerID,
					}, nil)

				mockHolderLinkRepo.EXPECT().
					FindByAliasID(gomock.Any(), gomock.Any(), gomock.Any(), false).
					Return([]*mmodel.HolderLink{
						{
							ID:       &holderLinkID,
							HolderID: &holderID,
							AliasID:  &id,
							LinkType: &linkTypePrimaryHolder,
						},
					}, nil)
			},
			expectedErr: nil,
			expectedResult: &mmodel.Alias{
				ID:        &id,
				Document:  &holderDocument,
				AccountID: &accountID,
				LedgerID:  &ledgerID,
				HolderLinks: []*mmodel.HolderLink{
					{
						ID:       &holderLinkID,
						HolderID: &holderID,
						AliasID:  &id,
						LinkType: &linkTypePrimaryHolder,
					},
				},
			},
		},
		{
			name:     "Error when holder not found for alias creation",
			holderID: uuid.New(),
			input: &mmodel.CreateAliasInput{
				LedgerID:  ledgerID,
				AccountID: accountID,
			},
			mockSetup: func() {
				mockHolderRepo.EXPECT().
					Find(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
					Return(nil, cn.ErrHolderNotFound)
			},
			expectedErr:    cn.ErrHolderNotFound,
			expectedResult: nil,
		},
		{
			name:     "Error when invalid LinkType provided",
			holderID: holderID,
			input: &mmodel.CreateAliasInput{
				LedgerID:  ledgerID,
				AccountID: accountID,
				LinkType:  utils.StringPtr("INVALID_TYPE"),
			},
			mockSetup:      func() {},
			expectedErr:    cn.ErrInvalidLinkType,
			expectedResult: nil,
		},
		{
			name:     "Error when PRIMARY_HOLDER already exists",
			holderID: holderID,
			input: &mmodel.CreateAliasInput{
				LedgerID:  ledgerID,
				AccountID: accountID,
				LinkType:  &linkTypePrimaryHolder,
			},
			mockSetup: func() {
				mockHolderRepo.EXPECT().
					Find(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
					Return(&mmodel.Holder{
						ID:       &holderID,
						Document: &holderDocument,
					}, nil)

				mockAliasRepo.EXPECT().
					Create(gomock.Any(), gomock.Any(), gomock.Any()).
					Return(&mmodel.Alias{
						ID:        &id,
						Document:  &holderDocument,
						AccountID: &accountID,
						LedgerID:  &ledgerID,
					}, nil)

				mockHolderLinkRepo.EXPECT().
					FindByAliasIDAndLinkType(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), false).
					Return(&mmodel.HolderLink{
						ID:       &holderLinkID,
						LinkType: &linkTypePrimaryHolder,
					}, nil)

				mockAliasRepo.EXPECT().
					Delete(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), true).
					Return(nil)
			},
			expectedErr:    cn.ErrPrimaryHolderAlreadyExists,
			expectedResult: nil,
		},
		{
			name:     "Success with nil LinkType (optional field)",
			holderID: holderID,
			input: &mmodel.CreateAliasInput{
				LedgerID:  ledgerID,
				AccountID: accountID,
				LinkType:  nil,
			},
			mockSetup: func() {
				mockHolderRepo.EXPECT().
					Find(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
					Return(&mmodel.Holder{
						ID:       &holderID,
						Document: &holderDocument,
					}, nil)

				mockAliasRepo.EXPECT().
					Create(gomock.Any(), gomock.Any(), gomock.Any()).
					Return(&mmodel.Alias{
						ID:        &id,
						Document:  &holderDocument,
						AccountID: &accountID,
						LedgerID:  &ledgerID,
					}, nil)
			},
			expectedErr: nil,
			expectedResult: &mmodel.Alias{
				ID:        &id,
				Document:  &holderDocument,
				AccountID: &accountID,
				LedgerID:  &ledgerID,
			},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			testCase.mockSetup()

			ctx := context.Background()
			result, err := uc.CreateAlias(ctx, uuid.New().String(), testCase.holderID, testCase.input)

			if testCase.expectedErr != nil {
				assert.Error(t, err)
				assert.Nil(t, result)

				var validationErr pkg.ValidationError
				var conflictErr pkg.EntityConflictError
				var notFoundErr pkg.EntityNotFoundError

				switch {
				case errors.As(err, &validationErr):
					assert.Equal(t, testCase.expectedErr.Error(), validationErr.Code)
				case errors.As(err, &conflictErr):
					assert.Equal(t, testCase.expectedErr.Error(), conflictErr.Code)
				case errors.As(err, &notFoundErr):
					assert.Equal(t, testCase.expectedErr.Error(), notFoundErr.Code)
				default:
					assert.ErrorIs(t, err, testCase.expectedErr)
				}
			} else {
				assert.NoError(t, err)
				if testCase.expectedResult != nil {
					assert.NotNil(t, result)
					assert.Equal(t, testCase.expectedResult.ID, result.ID)
					assert.Equal(t, testCase.expectedResult.AccountID, result.AccountID)
					assert.Equal(t, testCase.expectedResult.LedgerID, result.LedgerID)
					if testCase.expectedResult.HolderLinks != nil {
						assert.Equal(t, len(testCase.expectedResult.HolderLinks), len(result.HolderLinks))
						if len(testCase.expectedResult.HolderLinks) > 0 {
							assert.Equal(t, testCase.expectedResult.HolderLinks[0].ID, result.HolderLinks[0].ID)
							assert.Equal(t, testCase.expectedResult.HolderLinks[0].LinkType, result.HolderLinks[0].LinkType)
						}
					}
				}
			}
		})
	}
}

// TestCreateAliasWithHolderLink_NilCreatedAccount_Panics verifies that passing
// nil createdAccount causes a panic with context rather than a cryptic nil pointer error.
func TestCreateAliasWithHolderLink_NilCreatedAccount_Panics(t *testing.T) {
	uc := &UseCase{}

	ctx := context.Background()
	holderID := libCommons.GenerateUUIDv7()
	linkType := string(mmodel.LinkTypePrimaryHolder)

	require.Panics(t, func() {
		_, _ = uc.createAliasWithHolderLink(
			ctx,
			nil, // span
			nil, // logger
			"org-123",
			holderID,
			&mmodel.CreateAliasInput{LinkType: &linkType},
			&mmodel.Alias{}, // alias
			nil,             // createdAccount is nil - should panic
		)
	}, "createAliasWithHolderLink should panic when createdAccount is nil")
}

// TestCreateAliasWithHolderLink_NilCreatedAccountID_Panics verifies that passing
// createdAccount with nil ID causes a panic with context rather than a cryptic nil pointer error.
func TestCreateAliasWithHolderLink_NilCreatedAccountID_Panics(t *testing.T) {
	uc := &UseCase{}

	ctx := context.Background()
	holderID := libCommons.GenerateUUIDv7()
	linkType := string(mmodel.LinkTypePrimaryHolder)

	require.Panics(t, func() {
		_, _ = uc.createAliasWithHolderLink(
			ctx,
			nil, // span
			nil, // logger
			"org-123",
			holderID,
			&mmodel.CreateAliasInput{LinkType: &linkType},
			&mmodel.Alias{},          // alias
			&mmodel.Alias{ID: nil},   // createdAccount with nil ID - should panic
		)
	}, "createAliasWithHolderLink should panic when createdAccount.ID is nil")
}
