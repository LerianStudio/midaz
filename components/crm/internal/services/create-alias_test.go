package services

import (
	"context"
	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
	"github.com/LerianStudio/midaz/v3/components/crm/internal/adapters/mongodb/alias"
	"github.com/LerianStudio/midaz/v3/components/crm/internal/adapters/mongodb/holder"
	cn "github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"testing"
)

func TestCreateAlias(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockHolderRepo := holder.NewMockRepository(ctrl)
	mockAliasRepo := alias.NewMockRepository(ctrl)

	holderID := libCommons.GenerateUUIDv7()
	id := libCommons.GenerateUUIDv7()
	accountID := libCommons.GenerateUUIDv7().String()
	ledgerID := libCommons.GenerateUUIDv7().String()
	holderDocument := "90217469051"

	uc := &UseCase{
		HolderRepo: mockHolderRepo,
		AliasRepo:  mockAliasRepo,
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
			name:     "Success with required fields provided",
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
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			testCase.mockSetup()

			ctx := context.Background()
			result, err := uc.CreateAlias(ctx, uuid.New().String(), holderID, testCase.input)

			if testCase.expectedErr != nil {
				assert.Error(t, err)
				assert.Nil(t, result)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, testCase.expectedResult, result)
			}
		})
	}
}
