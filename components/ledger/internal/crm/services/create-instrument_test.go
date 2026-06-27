// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package services

import (
	"context"
	"testing"
	"time"

	libCommons "github.com/LerianStudio/lib-commons/v5/commons"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/crm/adapters/mongodb/holder"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/crm/adapters/mongodb/instrument"
	"github.com/LerianStudio/midaz/v4/pkg"
	cn "github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
)

// stubLedgerAccountReader is a hand-rolled LedgerAccountReader stub: the
// referential check is a hard dependency of CreateInstrument, so every test
// must inject one. The two booleans drive the not-found branches; the *Err
// fields drive the transient/infrastructure branches.
type stubLedgerAccountReader struct {
	ledgerExists  bool
	accountExists bool
	ledgerErr     error
	accountErr    error
	accountCount  int64
	accountCntErr error
}

func (s *stubLedgerAccountReader) LedgerExists(_ context.Context, _, _ uuid.UUID) (bool, error) {
	return s.ledgerExists, s.ledgerErr
}

func (s *stubLedgerAccountReader) AccountExists(_ context.Context, _, _, _ uuid.UUID) (bool, error) {
	return s.accountExists, s.accountErr
}

func (s *stubLedgerAccountReader) CountAccountsByHolder(_ context.Context, _, _ uuid.UUID) (int64, error) {
	return s.accountCount, s.accountCntErr
}

func TestCreateAlias(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockHolderRepo := holder.NewMockRepository(ctrl)
	mockAliasRepo := instrument.NewMockRepository(ctrl)

	holderID := uuid.Must(libCommons.GenerateUUIDv7())
	id := uuid.Must(libCommons.GenerateUUIDv7())
	accountID := uuid.Must(libCommons.GenerateUUIDv7()).String()
	ledgerID := uuid.Must(libCommons.GenerateUUIDv7()).String()
	holderDocument := "90217469051"
	participantDoc := "12345678912345"

	uc := &UseCase{
		HolderRepo:     mockHolderRepo,
		InstrumentRepo: mockAliasRepo,
	}

	// Default reader: both references resolve, so the pre-existing success and
	// holder/related-party cases exercise their original paths unchanged.
	bothExist := &stubLedgerAccountReader{ledgerExists: true, accountExists: true}

	testCases := []struct {
		name           string
		holderID       uuid.UUID
		input          *mmodel.CreateInstrumentInput
		reader         *stubLedgerAccountReader
		mockSetup      func()
		expectedErr    error
		expectedResult *mmodel.Instrument
	}{
		{
			name:     "Success with required fields provided",
			holderID: holderID,
			input: &mmodel.CreateInstrumentInput{
				LedgerID:  ledgerID,
				AccountID: accountID,
			},
			reader: bothExist,
			mockSetup: func() {
				mockHolderRepo.EXPECT().
					Find(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
					Return(&mmodel.Holder{
						ID:       &holderID,
						Document: &holderDocument,
					}, nil)

				mockAliasRepo.EXPECT().
					Create(gomock.Any(), gomock.Any(), gomock.Any()).
					Return(&mmodel.Instrument{
						ID:        &id,
						Document:  &holderDocument,
						AccountID: &accountID,
						LedgerID:  &ledgerID,
					}, nil)
			},
			expectedErr: nil,
			expectedResult: &mmodel.Instrument{
				ID:        &id,
				Document:  &holderDocument,
				AccountID: &accountID,
				LedgerID:  &ledgerID,
			},
		},
		{
			name:     "Success with RegulatoryFields",
			holderID: holderID,
			input: &mmodel.CreateInstrumentInput{
				LedgerID:  ledgerID,
				AccountID: accountID,
				RegulatoryFields: &mmodel.RegulatoryFields{
					ParticipantDocument: &participantDoc,
				},
			},
			reader: bothExist,
			mockSetup: func() {
				mockHolderRepo.EXPECT().
					Find(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
					Return(&mmodel.Holder{
						ID:       &holderID,
						Document: &holderDocument,
					}, nil)

				mockAliasRepo.EXPECT().
					Create(gomock.Any(), gomock.Any(), gomock.Any()).
					Return(&mmodel.Instrument{
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
			expectedResult: &mmodel.Instrument{
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
			input: &mmodel.CreateInstrumentInput{
				LedgerID:  ledgerID,
				AccountID: accountID,
			},
			reader: bothExist,
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
			input: &mmodel.CreateInstrumentInput{
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
			reader: bothExist,
			mockSetup: func() {
				mockHolderRepo.EXPECT().
					Find(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
					Return(&mmodel.Holder{
						ID:       &holderID,
						Document: &holderDocument,
					}, nil)

				mockAliasRepo.EXPECT().
					Create(gomock.Any(), gomock.Any(), gomock.Any()).
					Return(&mmodel.Instrument{
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
			expectedResult: &mmodel.Instrument{
				ID:        &id,
				Document:  &holderDocument,
				AccountID: &accountID,
				LedgerID:  &ledgerID,
			},
		},
		{
			name:     "Error when related party document is empty",
			holderID: holderID,
			input: &mmodel.CreateInstrumentInput{
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
			input: &mmodel.CreateInstrumentInput{
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
		{
			name:     "Error when ledger reference does not exist (422, no Mongo write)",
			holderID: holderID,
			input: &mmodel.CreateInstrumentInput{
				LedgerID:  ledgerID,
				AccountID: accountID,
			},
			reader: &stubLedgerAccountReader{ledgerExists: false, accountExists: true},
			mockSetup: func() {
				mockHolderRepo.EXPECT().
					Find(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
					Return(&mmodel.Holder{
						ID:       &holderID,
						Document: &holderDocument,
					}, nil)
				// No mockAliasRepo.Create expectation: the create must NOT run.
			},
			expectedErr:    cn.ErrInstrumentLedgerReferenceNotFound,
			expectedResult: nil,
		},
		{
			name:     "Error when account reference does not exist (422, no Mongo write)",
			holderID: holderID,
			input: &mmodel.CreateInstrumentInput{
				LedgerID:  ledgerID,
				AccountID: accountID,
			},
			reader: &stubLedgerAccountReader{ledgerExists: true, accountExists: false},
			mockSetup: func() {
				mockHolderRepo.EXPECT().
					Find(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
					Return(&mmodel.Holder{
						ID:       &holderID,
						Document: &holderDocument,
					}, nil)
				// No mockAliasRepo.Create expectation: the create must NOT run.
			},
			expectedErr:    cn.ErrInstrumentAccountReferenceNotFound,
			expectedResult: nil,
		},
		{
			name:     "Error when ledger id body field is a malformed UUID (no Mongo write)",
			holderID: holderID,
			input: &mmodel.CreateInstrumentInput{
				LedgerID:  "not-a-uuid",
				AccountID: accountID,
			},
			reader: bothExist,
			mockSetup: func() {
				mockHolderRepo.EXPECT().
					Find(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
					Return(&mmodel.Holder{
						ID:       &holderID,
						Document: &holderDocument,
					}, nil)
				// No mockAliasRepo.Create expectation: the create must NOT run.
			},
			expectedErr:    cn.ErrInvalidPathParameter,
			expectedResult: nil,
		},
		{
			name:     "Error when account id body field is a malformed UUID (no Mongo write)",
			holderID: holderID,
			input: &mmodel.CreateInstrumentInput{
				LedgerID:  ledgerID,
				AccountID: "not-a-uuid",
			},
			reader: bothExist,
			mockSetup: func() {
				mockHolderRepo.EXPECT().
					Find(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
					Return(&mmodel.Holder{
						ID:       &holderID,
						Document: &holderDocument,
					}, nil)
				// No mockAliasRepo.Create expectation: the create must NOT run.
			},
			expectedErr:    cn.ErrInvalidPathParameter,
			expectedResult: nil,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			testCase.mockSetup()
			uc.LedgerAccounts = testCase.reader

			ctx := context.Background()
			result, err := uc.CreateInstrument(ctx, uuid.New().String(), testCase.holderID, testCase.input)

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
					} else if unprocessableErr, ok := err.(pkg.UnprocessableOperationError); ok {
						assert.Equal(t, testCase.expectedErr.Error(), unprocessableErr.Code)
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
