// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

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
	participantDoc := "12345678912345"

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
			name:     "Success with RegulatoryFields",
			holderID: holderID,
			input: &mmodel.CreateAliasInput{
				LedgerID:  ledgerID,
				AccountID: accountID,
				RegulatoryFields: &mmodel.RegulatoryFields{
					ParticipantDocument: &participantDoc,
				},
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
						RegulatoryFields: &mmodel.RegulatoryFields{
							ParticipantDocument: &participantDoc,
						},
					}, nil)
			},
			expectedErr: nil,
			expectedResult: &mmodel.Alias{
				ID:        &id,
				Document:  &holderDocument,
				AccountID: &accountID,
				LedgerID:  &ledgerID,
				RegulatoryFields: &mmodel.RegulatoryFields{
					ParticipantDocument: &participantDoc,
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
			name:     "Success with RelatedParties",
			holderID: holderID,
			input: &mmodel.CreateAliasInput{
				LedgerID:  ledgerID,
				AccountID: accountID,
				RelatedParties: []*mmodel.RelatedParty{
					{
						Document:  "12345678900",
						Name:      "John Smith",
						Role:      "PRIMARY_HOLDER",
						StartDate: mmodel.Date{Time: time.Now()},
					},
				},
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
						RelatedParties: []*mmodel.RelatedParty{
							{
								Document:  "12345678900",
								Name:      "John Smith",
								Role:      "PRIMARY_HOLDER",
								StartDate: mmodel.Date{Time: time.Now()},
							},
						},
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
			name:     "Error when related party document is empty",
			holderID: holderID,
			input: &mmodel.CreateAliasInput{
				LedgerID:  ledgerID,
				AccountID: accountID,
				RelatedParties: []*mmodel.RelatedParty{
					{
						Document:  "",
						Name:      "Jane Doe",
						Role:      "PRIMARY_HOLDER",
						StartDate: mmodel.Date{Time: time.Now()},
					},
				},
			},
			mockSetup:      func() {},
			expectedErr:    cn.ErrRelatedPartyDocumentRequired,
			expectedResult: nil,
		},
		{
			name:     "Error when related party role is invalid",
			holderID: holderID,
			input: &mmodel.CreateAliasInput{
				LedgerID:  ledgerID,
				AccountID: accountID,
				RelatedParties: []*mmodel.RelatedParty{
					{
						Document:  "12345678900",
						Name:      "Jane Doe",
						Role:      "INVALID_ROLE",
						StartDate: mmodel.Date{Time: time.Now()},
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

			ctx := context.Background()
			result, err := uc.CreateAlias(ctx, uuid.New().String(), testCase.holderID, testCase.input)

			if testCase.expectedErr != nil {
				assert.Error(t, err)
				assert.Nil(t, result)
				if testCase.expectedErr != nil {
					if validationErr, ok := err.(pkg.ValidationError); ok {
						assert.Equal(t, testCase.expectedErr.Error(), validationErr.Code)
					} else if conflictErr, ok := err.(pkg.EntityConflictError); ok {
						assert.Equal(t, testCase.expectedErr.Error(), conflictErr.Code)
					} else if notFoundErr, ok := err.(pkg.EntityNotFoundError); ok {
						assert.Equal(t, testCase.expectedErr.Error(), notFoundErr.Code)
					} else {
						assert.Equal(t, testCase.expectedErr, err)
					}
				}
			} else {
				assert.NoError(t, err)
				if testCase.expectedResult != nil {
					assert.NotNil(t, result)
					assert.Equal(t, testCase.expectedResult.ID, result.ID)
					assert.Equal(t, testCase.expectedResult.AccountID, result.AccountID)
					assert.Equal(t, testCase.expectedResult.LedgerID, result.LedgerID)
				}
			}
		})
	}
}
