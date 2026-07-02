// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package services

import (
	"context"
	"errors"
	"testing"
	"time"

	libCommons "github.com/LerianStudio/lib-commons/v5/commons"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/crm/adapters/mongodb/holder"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/crm/adapters/mongodb/instrument"
	"github.com/LerianStudio/midaz/v4/pkg"
	cn "github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	pkgStreaming "github.com/LerianStudio/midaz/v4/pkg/streaming"
	"github.com/LerianStudio/midaz/v4/pkg/streaming/events"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

func TestUpdateInstrumentByID(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockHolderRepo := holder.NewMockRepository(ctrl)
	mockInstrumentRepo := instrument.NewMockRepository(ctrl)

	holderID := uuid.Must(libCommons.GenerateUUIDv7())
	id := uuid.Must(libCommons.GenerateUUIDv7())
	accountID := uuid.Must(libCommons.GenerateUUIDv7()).String()
	ledgerID := uuid.Must(libCommons.GenerateUUIDv7()).String()
	holderDocument := "90217469051"
	branch := "0001"
	participantDoc := "12345678912345"

	uc := &UseCase{
		HolderRepo:     mockHolderRepo,
		InstrumentRepo: mockInstrumentRepo,
	}

	testCases := []struct {
		name           string
		id             uuid.UUID
		holderID       uuid.UUID
		input          *mmodel.UpdateInstrumentInput
		mockSetup      func()
		expectedErr    error
		expectedResult *mmodel.Instrument
	}{
		{
			name:     "Success with single field provided",
			id:       id,
			holderID: holderID,
			input: &mmodel.UpdateInstrumentInput{
				BankingDetails: &mmodel.BankingDetails{
					Branch: &branch,
				},
			},
			mockSetup: func() {
				mockInstrumentRepo.EXPECT().
					Update(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
					Return(&mmodel.Instrument{
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
			expectedResult: &mmodel.Instrument{
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
			input: &mmodel.UpdateInstrumentInput{
				RegulatoryFields: &mmodel.RegulatoryFields{
					ParticipantDocument: &participantDoc,
				},
				RelatedParties: []*mmodel.RelatedParty{
					{
						Document:  "12345678900",
						Name:      "Maria de Jesus",
						Role:      "PRIMARY_HOLDER",
						StartDate: mmodel.Date{Time: time.Now()},
					},
				},
			},
			mockSetup: func() {
				mockInstrumentRepo.EXPECT().
					Find(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), false).
					Return(&mmodel.Instrument{
						ID:        &id,
						Document:  &holderDocument,
						LedgerID:  &ledgerID,
						HolderID:  &holderID,
						AccountID: &accountID,
					}, nil)

				mockInstrumentRepo.EXPECT().
					Update(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
					Return(&mmodel.Instrument{
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
			expectedResult: &mmodel.Instrument{
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
			name:     "Error when instrument not found by ID",
			id:       id,
			holderID: holderID,
			input: &mmodel.UpdateInstrumentInput{
				BankingDetails: &mmodel.BankingDetails{
					Branch: &branch,
				},
			},
			mockSetup: func() {
				mockInstrumentRepo.EXPECT().
					Update(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
					Return(nil, cn.ErrInstrumentNotFound)
			},
			expectedErr:    cn.ErrInstrumentNotFound,
			expectedResult: nil,
		},
		{
			name:     "Error when invalid RelatedParty role",
			id:       id,
			holderID: holderID,
			input: &mmodel.UpdateInstrumentInput{
				RelatedParties: []*mmodel.RelatedParty{
					{
						Document:  "12345678900",
						Name:      "Maria de Jesus",
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
			name:     "Error when fetch existing instrument for related parties fails",
			id:       id,
			holderID: holderID,
			input: &mmodel.UpdateInstrumentInput{
				RelatedParties: []*mmodel.RelatedParty{
					{
						Document:  "12345678900",
						Name:      "Maria de Jesus",
						Role:      "PRIMARY_HOLDER",
						StartDate: mmodel.Date{Time: time.Now()},
					},
				},
			},
			mockSetup: func() {
				mockInstrumentRepo.EXPECT().
					Find(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), false).
					Return(nil, errors.New("database error"))
			},
			expectedErr:    errors.New("database error"),
			expectedResult: nil,
		},
		{
			name:     "Success with RelatedParties appended to existing",
			id:       id,
			holderID: holderID,
			input: &mmodel.UpdateInstrumentInput{
				RelatedParties: []*mmodel.RelatedParty{
					{
						Document:  "12345678900",
						Name:      "Maria de Jesus",
						Role:      "PRIMARY_HOLDER",
						StartDate: mmodel.Date{Time: time.Now()},
					},
				},
			},
			mockSetup: func() {
				existingRPID := uuid.Must(libCommons.GenerateUUIDv7())
				mockInstrumentRepo.EXPECT().
					Find(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), false).
					Return(&mmodel.Instrument{
						ID:        &id,
						Document:  &holderDocument,
						LedgerID:  &ledgerID,
						HolderID:  &holderID,
						AccountID: &accountID,
						RelatedParties: []*mmodel.RelatedParty{
							{
								ID:        &existingRPID,
								Document:  "99988877766",
								Name:      "Existing Party",
								Role:      "LEGAL_REPRESENTATIVE",
								StartDate: mmodel.Date{Time: time.Now().Add(-24 * time.Hour)},
							},
						},
					}, nil)

				mockInstrumentRepo.EXPECT().
					Update(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
					Return(&mmodel.Instrument{
						ID:        &id,
						Document:  &holderDocument,
						LedgerID:  &ledgerID,
						HolderID:  &holderID,
						AccountID: &accountID,
					}, nil)
			},
			expectedErr: nil,
			expectedResult: &mmodel.Instrument{
				ID:        &id,
				Document:  &holderDocument,
				LedgerID:  &ledgerID,
				HolderID:  &holderID,
				AccountID: &accountID,
			},
		},
		{
			name:     "Error when closing date is before creation date",
			id:       id,
			holderID: holderID,
			input: &mmodel.UpdateInstrumentInput{
				BankingDetails: &mmodel.BankingDetails{
					Branch:      &branch,
					ClosingDate: &mmodel.Date{Time: time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)},
				},
			},
			mockSetup: func() {
				mockInstrumentRepo.EXPECT().
					Find(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), false).
					Return(&mmodel.Instrument{
						ID:        &id,
						Document:  &holderDocument,
						LedgerID:  &ledgerID,
						HolderID:  &holderID,
						AccountID: &accountID,
						CreatedAt: time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
					}, nil)
			},
			expectedErr:    cn.ErrInstrumentClosingDateBeforeCreation,
			expectedResult: nil,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			testCase.mockSetup()

			fieldsToRemove := []string{"field1", "field2"}

			ctx := context.Background()
			result, err := uc.UpdateInstrumentByID(ctx, uuid.New().String(), holderID, id, testCase.input, fieldsToRemove)

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

func TestUpdateInstrumentByID_EmitsInstrumentUpdated(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockInstrumentRepo := instrument.NewMockRepository(ctrl)

	holderID := uuid.Must(libCommons.GenerateUUIDv7())
	instrumentID := uuid.Must(libCommons.GenerateUUIDv7())
	orgID := uuid.Must(libCommons.GenerateUUIDv7()).String()
	updatedAt := time.Date(2026, 6, 30, 12, 0, 0, 0, time.UTC)

	emitter := pkgStreaming.NewMockEmitter()

	uc := &UseCase{
		InstrumentRepo: mockInstrumentRepo,
		Streaming:      emitter,
	}

	mockInstrumentRepo.EXPECT().
		Update(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return(&mmodel.Instrument{ID: &instrumentID, HolderID: &holderID, UpdatedAt: updatedAt}, nil)

	ctx := context.Background()
	result, err := uc.UpdateInstrumentByID(ctx, orgID, holderID, instrumentID, &mmodel.UpdateInstrumentInput{}, nil)

	require.NoError(t, err)
	require.NotNil(t, result)

	emitted := emitter.Events()
	require.Len(t, emitted, 1)
	assert.Equal(t, events.InstrumentUpdatedDefinition.Key(), emitted[0].DefinitionKey)
	assert.Equal(t, instrumentID.String(), emitted[0].Subject)
	assert.Equal(t, updatedAt, emitted[0].Timestamp)
	pkgStreaming.AssertEventEmitted(t, emitter, "instrument", "updated")
}

func TestUpdateInstrumentByID_NilEmitterSucceeds(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockInstrumentRepo := instrument.NewMockRepository(ctrl)

	holderID := uuid.Must(libCommons.GenerateUUIDv7())
	instrumentID := uuid.Must(libCommons.GenerateUUIDv7())

	uc := &UseCase{
		InstrumentRepo: mockInstrumentRepo,
		Streaming:      nil,
	}

	mockInstrumentRepo.EXPECT().
		Update(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return(&mmodel.Instrument{ID: &instrumentID, HolderID: &holderID}, nil)

	ctx := context.Background()
	result, err := uc.UpdateInstrumentByID(ctx, uuid.Must(libCommons.GenerateUUIDv7()).String(), holderID, instrumentID, &mmodel.UpdateInstrumentInput{}, nil)

	require.NoError(t, err)
	require.NotNil(t, result)
}

func TestUpdateInstrumentByID_EmitFailureDoesNotFailRequest(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockInstrumentRepo := instrument.NewMockRepository(ctrl)

	holderID := uuid.Must(libCommons.GenerateUUIDv7())
	instrumentID := uuid.Must(libCommons.GenerateUUIDv7())

	emitter := pkgStreaming.NewMockEmitter()
	emitter.SetError(errors.New("broker unavailable"))

	uc := &UseCase{
		InstrumentRepo: mockInstrumentRepo,
		Streaming:      emitter,
	}

	mockInstrumentRepo.EXPECT().
		Update(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return(&mmodel.Instrument{ID: &instrumentID, HolderID: &holderID}, nil)

	ctx := context.Background()
	result, err := uc.UpdateInstrumentByID(ctx, uuid.Must(libCommons.GenerateUUIDv7()).String(), holderID, instrumentID, &mmodel.UpdateInstrumentInput{}, nil)

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Empty(t, emitter.Events())
}
