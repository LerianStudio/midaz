package services

import (
	"context"
	"testing"
	"time"

	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	"github.com/LerianStudio/midaz/v3/components/crm/internal/adapters/mongodb/alias"
	"github.com/LerianStudio/midaz/v3/components/crm/internal/adapters/mongodb/holder"
	"github.com/LerianStudio/midaz/v3/pkg"
	cn "github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
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
	participantDoc := "12345678912345"

	uc := &UseCase{
		HolderRepo: mockHolderRepo,
		AliasRepo:  mockAliasRepo,
	}

	testCases := []struct {
		name           string
		id             uuid.UUID
		holderID       uuid.UUID
		input          *mmodel.UpdateAliasInput
		mockSetup      func()
		expectedErr    error
		expectedResult *mmodel.Alias
	}{
		{
			name:     "Success with single field provided",
			id:       id,
			holderID: holderID,
			input: &mmodel.UpdateAliasInput{
				BankingDetails: &mmodel.BankingDetails{
					Branch: &branch,
				},
			},
			mockSetup: func() {
				mockAliasRepo.EXPECT().
					Update(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
					Return(&mmodel.Alias{
						ID:        &id,
						Document:  &holderDocument,
						LedgerID:  &ledgerID,
						HolderID:  &holderID,
						AccountID: &accountID,
						BankingDetails: &mmodel.BankingDetails{
							Branch: &branch,
						},
					}, nil)
			},
			expectedErr: nil,
			expectedResult: &mmodel.Alias{
				ID:        &id,
				Document:  &holderDocument,
				LedgerID:  &ledgerID,
				HolderID:  &holderID,
				AccountID: &accountID,
				BankingDetails: &mmodel.BankingDetails{
					Branch: &branch,
				},
			},
		},
		{
			name:     "Success with RegulatoryFields and RelatedParties",
			id:       id,
			holderID: holderID,
			input: &mmodel.UpdateAliasInput{
				RegulatoryFields: &mmodel.RegulatoryFields{
					ParticipantDocument: &participantDoc,
				},
				RelatedParties: []*mmodel.RelatedParty{
					{
						Document:  "12345678900",
						Name:      "Maria de Jesus",
						Role:      "PRIMARY_HOLDER",
						StartDate: time.Now(),
					},
				},
			},
			mockSetup: func() {
				mockAliasRepo.EXPECT().
					Update(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
					Return(&mmodel.Alias{
						ID:        &id,
						Document:  &holderDocument,
						LedgerID:  &ledgerID,
						HolderID:  &holderID,
						AccountID: &accountID,
						RegulatoryFields: &mmodel.RegulatoryFields{
							ParticipantDocument: &participantDoc,
						},
					}, nil)
			},
			expectedErr: nil,
			expectedResult: &mmodel.Alias{
				ID:        &id,
				Document:  &holderDocument,
				LedgerID:  &ledgerID,
				HolderID:  &holderID,
				AccountID: &accountID,
				RegulatoryFields: &mmodel.RegulatoryFields{
					ParticipantDocument: &participantDoc,
				},
			},
		},
		{
			name:     "Error when alias not found by ID",
			id:       id,
			holderID: holderID,
			input: &mmodel.UpdateAliasInput{
				BankingDetails: &mmodel.BankingDetails{
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
		{
			name:     "Error when invalid RelatedParty role",
			id:       id,
			holderID: holderID,
			input: &mmodel.UpdateAliasInput{
				RelatedParties: []*mmodel.RelatedParty{
					{
						Document:  "12345678900",
						Name:      "Maria de Jesus",
						Role:      "INVALID_ROLE",
						StartDate: time.Now(),
					},
				},
			},
			mockSetup:      func() {},
			expectedErr:    cn.ErrInvalidRelatedPartyRole,
			expectedResult: nil,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			testCase.mockSetup()

			fieldsToRemove := []string{"field1", "field2"}

			ctx := context.Background()
			result, err := uc.UpdateAliasByID(ctx, uuid.New().String(), holderID, id, testCase.input, fieldsToRemove)

			if testCase.expectedErr != nil {
				assert.Error(t, err)
				assert.Nil(t, result)
				if validationErr, ok := err.(pkg.ValidationError); ok {
					assert.Equal(t, testCase.expectedErr.Error(), validationErr.Code)
				}
			} else {
				assert.NoError(t, err)
				assert.Equal(t, testCase.expectedResult.AccountID, result.AccountID)
				assert.Equal(t, testCase.expectedResult.HolderID, result.HolderID)
				assert.Equal(t, testCase.expectedResult.Document, result.Document)
				assert.Equal(t, testCase.expectedResult.LedgerID, result.LedgerID)
			}
		})
	}
}
