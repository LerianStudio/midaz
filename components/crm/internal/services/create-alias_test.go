package services

import (
	"context"
	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
	"plugin-crm/v2/internal/adapters/mongodb/alias"
	"plugin-crm/v2/internal/adapters/mongodb/holder"
	cn "plugin-crm/v2/pkg/constant"
	"plugin-crm/v2/pkg/model"
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
		input          *model.CreateAliasInput
		mockSetup      func()
		expectedErr    error
		expectedResult *model.Alias
	}{
		{
			name:     "Success with required fields provided",
			holderID: holderID,
			input: &model.CreateAliasInput{
				LedgerID:  ledgerID,
				AccountID: accountID,
			},
			mockSetup: func() {
				mockHolderRepo.EXPECT().
					Find(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
					Return(&model.Holder{
						ID:       &holderID,
						Document: &holderDocument,
					}, nil)

				mockAliasRepo.EXPECT().
					Create(gomock.Any(), gomock.Any(), gomock.Any()).
					Return(&model.Alias{
						ID:        &id,
						Document:  &holderDocument,
						AccountID: &accountID,
						LedgerID:  &ledgerID,
					}, nil)
			},
			expectedErr: nil,
			expectedResult: &model.Alias{
				ID:        &id,
				Document:  &holderDocument,
				AccountID: &accountID,
				LedgerID:  &ledgerID,
			},
		},
		{
			name:     "Error when holder not found for alias creation",
			holderID: uuid.New(),
			input: &model.CreateAliasInput{
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
