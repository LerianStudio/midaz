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

func TestUpdateAliasByID(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockHolderRepo := holder.NewMockRepository(ctrl)
	mockAliasRepo := alias.NewMockRepository(ctrl)

	holderID := libCommons.GenerateUUIDv7()
	id := libCommons.GenerateUUIDv7()
	accountID := libCommons.GenerateUUIDv7().String()
	ledgerID := libCommons.GenerateUUIDv7().String()
	holderDocument := "90217469051"
	branch := "0001"

	uc := &UseCase{
		HolderRepo: mockHolderRepo,
		AliasRepo:  mockAliasRepo,
	}

	testCases := []struct {
		name           string
		id             uuid.UUID
		holderID       uuid.UUID
		input          *model.UpdateAliasInput
		mockSetup      func()
		expectedErr    error
		expectedResult *model.Alias
	}{
		{
			name:     "Success with single field provided",
			id:       id,
			holderID: holderID,
			input: &model.UpdateAliasInput{
				BankingDetails: &model.BankingDetails{
					Branch: &branch,
				},
			},
			mockSetup: func() {
				mockAliasRepo.EXPECT().
					Update(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
					Return(&model.Alias{
						ID:        &id,
						Document:  &holderDocument,
						LedgerID:  &ledgerID,
						HolderID:  &holderID,
						AccountID: &accountID,
						BankingDetails: &model.BankingDetails{
							Branch: &branch,
						},
					}, nil)
			},
			expectedErr: nil,
			expectedResult: &model.Alias{
				ID:        &id,
				Document:  &holderDocument,
				LedgerID:  &ledgerID,
				HolderID:  &holderID,
				AccountID: &accountID,
				BankingDetails: &model.BankingDetails{
					Branch: &branch,
				},
			},
		},
		{
			name:     "Error when alias not found by ID",
			id:       id,
			holderID: holderID,
			input: &model.UpdateAliasInput{
				BankingDetails: &model.BankingDetails{
					Branch: &branch,
				},
			},
			mockSetup: func() {
				mockAliasRepo.EXPECT().
					Update(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
					Return(nil, cn.ErrAliasNotFound)
			},
			expectedErr:    cn.ErrAliasNotFound,
			expectedResult: nil,
		},
	}

	for _, testCase := range testCases {
		testCase.mockSetup()

		fieldsToRemove := []string{"field1", "field2"}

		ctx := context.Background()
		result, err := uc.UpdateAliasByID(ctx, uuid.New().String(), holderID, id, testCase.input, fieldsToRemove)

		if testCase.expectedErr != nil {
			assert.Error(t, err)
			assert.Nil(t, result)
		} else {
			assert.NoError(t, err)
			assert.Equal(t, testCase.expectedResult.AccountID, result.AccountID)
			assert.Equal(t, testCase.expectedResult.HolderID, result.HolderID)
			assert.Equal(t, testCase.expectedResult.Document, result.Document)
			assert.Equal(t, testCase.expectedResult.LedgerID, result.LedgerID)
			assert.Equal(t, testCase.expectedResult.AccountID, result.AccountID)
			assert.Equal(t, testCase.expectedResult.BankingDetails.BankID, result.BankingDetails.BankID)
		}
	}
}
