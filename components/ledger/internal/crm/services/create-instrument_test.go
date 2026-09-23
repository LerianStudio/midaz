// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package services

import (
	"context"
	"errors"
	"testing"
	"time"

	libCommons "github.com/LerianStudio/lib-commons/v7/commons"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/codes"
	"go.uber.org/mock/gomock"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/crm/adapters/mongodb/holder"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/crm/adapters/mongodb/instrument"
	"github.com/LerianStudio/midaz/v4/pkg"
	cn "github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	pkgStreaming "github.com/LerianStudio/midaz/v4/pkg/streaming"
	"github.com/LerianStudio/midaz/v4/pkg/streaming/events"
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

func TestCreateInstrument(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockHolderRepo := holder.NewMockRepository(ctrl)
	mockInstrumentRepo := instrument.NewMockRepository(ctrl)

	holderID := uuid.Must(libCommons.GenerateUUIDv7())
	id := uuid.Must(libCommons.GenerateUUIDv7())
	accountID := uuid.Must(libCommons.GenerateUUIDv7()).String()
	ledgerID := uuid.Must(libCommons.GenerateUUIDv7()).String()
	holderDocument := "90217469051"
	participantDoc := "12345678912345"

	uc := &UseCase{
		HolderRepo:     mockHolderRepo,
		InstrumentRepo: mockInstrumentRepo,
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

				mockInstrumentRepo.EXPECT().
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

				mockInstrumentRepo.EXPECT().
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
			name:     "Error when holder not found for instrument creation",
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

				mockInstrumentRepo.EXPECT().
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
				// No mockInstrumentRepo.Create expectation: the create must NOT run.
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
				// No mockInstrumentRepo.Create expectation: the create must NOT run.
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
				// No mockInstrumentRepo.Create expectation: the create must NOT run.
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
				// No mockInstrumentRepo.Create expectation: the create must NOT run.
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

func TestCreateInstrument_EmitsInstrumentCreated(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockHolderRepo := holder.NewMockRepository(ctrl)
	mockInstrumentRepo := instrument.NewMockRepository(ctrl)

	holderID := uuid.Must(libCommons.GenerateUUIDv7())
	instrumentID := uuid.Must(libCommons.GenerateUUIDv7())
	accountID := uuid.Must(libCommons.GenerateUUIDv7()).String()
	ledgerID := uuid.Must(libCommons.GenerateUUIDv7()).String()
	holderDocument := "90217469051"
	orgID := uuid.Must(libCommons.GenerateUUIDv7()).String()

	emitter := pkgStreaming.NewMockEmitter()

	uc := &UseCase{
		HolderRepo:     mockHolderRepo,
		InstrumentRepo: mockInstrumentRepo,
		LedgerAccounts: &stubLedgerAccountReader{ledgerExists: true, accountExists: true},
		Streaming:      emitter,
	}

	mockHolderRepo.EXPECT().
		Find(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return(&mmodel.Holder{ID: &holderID, Document: &holderDocument}, nil)

	mockInstrumentRepo.EXPECT().
		Create(gomock.Any(), gomock.Any(), gomock.Any()).
		Return(&mmodel.Instrument{ID: &instrumentID, HolderID: &holderID, Document: &holderDocument, AccountID: &accountID, LedgerID: &ledgerID}, nil)

	ctx := context.Background()
	result, err := uc.CreateInstrument(ctx, orgID, holderID, &mmodel.CreateInstrumentInput{LedgerID: ledgerID, AccountID: accountID})

	require.NoError(t, err)
	require.NotNil(t, result)

	emitted := emitter.Events()
	require.Len(t, emitted, 1)
	assert.Equal(t, events.InstrumentCreatedDefinition.Key(), emitted[0].DefinitionKey)
	assert.Equal(t, instrumentID.String(), emitted[0].Subject)
	pkgStreaming.AssertEventEmitted(t, emitter, "instrument", "created")
}

func TestCreateInstrument_NilEmitterSucceeds(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockHolderRepo := holder.NewMockRepository(ctrl)
	mockInstrumentRepo := instrument.NewMockRepository(ctrl)

	holderID := uuid.Must(libCommons.GenerateUUIDv7())
	instrumentID := uuid.Must(libCommons.GenerateUUIDv7())
	accountID := uuid.Must(libCommons.GenerateUUIDv7()).String()
	ledgerID := uuid.Must(libCommons.GenerateUUIDv7()).String()
	holderDocument := "90217469051"

	uc := &UseCase{
		HolderRepo:     mockHolderRepo,
		InstrumentRepo: mockInstrumentRepo,
		LedgerAccounts: &stubLedgerAccountReader{ledgerExists: true, accountExists: true},
		Streaming:      nil,
	}

	mockHolderRepo.EXPECT().
		Find(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return(&mmodel.Holder{ID: &holderID, Document: &holderDocument}, nil)

	mockInstrumentRepo.EXPECT().
		Create(gomock.Any(), gomock.Any(), gomock.Any()).
		Return(&mmodel.Instrument{ID: &instrumentID, HolderID: &holderID, Document: &holderDocument, AccountID: &accountID, LedgerID: &ledgerID}, nil)

	ctx := context.Background()
	result, err := uc.CreateInstrument(ctx, uuid.Must(libCommons.GenerateUUIDv7()).String(), holderID, &mmodel.CreateInstrumentInput{LedgerID: ledgerID, AccountID: accountID})

	require.NoError(t, err)
	require.NotNil(t, result)
}

func TestCreateInstrument_EmitFailureDoesNotFailRequest(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockHolderRepo := holder.NewMockRepository(ctrl)
	mockInstrumentRepo := instrument.NewMockRepository(ctrl)

	holderID := uuid.Must(libCommons.GenerateUUIDv7())
	instrumentID := uuid.Must(libCommons.GenerateUUIDv7())
	accountID := uuid.Must(libCommons.GenerateUUIDv7()).String()
	ledgerID := uuid.Must(libCommons.GenerateUUIDv7()).String()
	holderDocument := "90217469051"

	emitter := pkgStreaming.NewMockEmitter()
	emitter.SetError(errors.New("broker unavailable"))

	uc := &UseCase{
		HolderRepo:     mockHolderRepo,
		InstrumentRepo: mockInstrumentRepo,
		LedgerAccounts: &stubLedgerAccountReader{ledgerExists: true, accountExists: true},
		Streaming:      emitter,
	}

	mockHolderRepo.EXPECT().
		Find(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return(&mmodel.Holder{ID: &holderID, Document: &holderDocument}, nil)

	mockInstrumentRepo.EXPECT().
		Create(gomock.Any(), gomock.Any(), gomock.Any()).
		Return(&mmodel.Instrument{ID: &instrumentID, HolderID: &holderID, Document: &holderDocument, AccountID: &accountID, LedgerID: &ledgerID}, nil)

	ctx := context.Background()
	result, err := uc.CreateInstrument(ctx, uuid.Must(libCommons.GenerateUUIDv7()).String(), holderID, &mmodel.CreateInstrumentInput{LedgerID: ledgerID, AccountID: accountID})

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Empty(t, emitter.Events())
}

func TestCreateInstrument_AccountType(t *testing.T) {
	holderID := uuid.Must(libCommons.GenerateUUIDv7())
	accountID := uuid.Must(libCommons.GenerateUUIDv7()).String()
	ledgerID := uuid.Must(libCommons.GenerateUUIDv7()).String()
	holderDocument := "90217469051"
	participantDoc := "12345678912345"

	strPtr := func(s string) *string { return &s }

	testCases := []struct {
		name                string
		regulatoryFields    *mmodel.RegulatoryFields
		expectRepoCall      bool
		expectedAccountType *string
		expectedErr         error
	}{
		{
			name:                "lowercase value with spaces reaches the repository normalized",
			regulatoryFields:    &mmodel.RegulatoryFields{AccountType: strPtr(" savings ")},
			expectRepoCall:      true,
			expectedAccountType: strPtr("SAVINGS"),
		},
		{
			name:                "account type alongside participant document",
			regulatoryFields:    &mmodel.RegulatoryFields{ParticipantDocument: &participantDoc, AccountType: strPtr("PAYMENT")},
			expectRepoCall:      true,
			expectedAccountType: strPtr("PAYMENT"),
		},
		{
			name:                "regulatory fields without account type keep a nil pointer",
			regulatoryFields:    &mmodel.RegulatoryFields{ParticipantDocument: &participantDoc},
			expectRepoCall:      true,
			expectedAccountType: nil,
		},
		{
			name:                "empty account type is treated as absent",
			regulatoryFields:    &mmodel.RegulatoryFields{ParticipantDocument: &participantDoc, AccountType: strPtr("")},
			expectRepoCall:      true,
			expectedAccountType: nil,
		},
		{
			name:             "value outside the enum is rejected without a repository call",
			regulatoryFields: &mmodel.RegulatoryFields{AccountType: strPtr("CHECKING")},
			expectRepoCall:   false,
			expectedErr:      cn.ErrInvalidInstrumentAccountType,
		},
		{
			name:             "numeric bacen code is rejected without a repository call",
			regulatoryFields: &mmodel.RegulatoryFields{AccountType: strPtr("1")},
			expectRepoCall:   false,
			expectedErr:      cn.ErrInvalidInstrumentAccountType,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockHolderRepo := holder.NewMockRepository(ctrl)
			mockInstrumentRepo := instrument.NewMockRepository(ctrl)

			uc := &UseCase{
				HolderRepo:     mockHolderRepo,
				InstrumentRepo: mockInstrumentRepo,
				LedgerAccounts: &stubLedgerAccountReader{ledgerExists: true, accountExists: true},
			}

			var persisted *mmodel.Instrument

			// With no expectation registered, gomock fails the test on any
			// unexpected repository call, so the rejection cases prove no write.
			if tc.expectRepoCall {
				mockHolderRepo.EXPECT().
					Find(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
					Return(&mmodel.Holder{ID: &holderID, Document: &holderDocument}, nil)

				mockInstrumentRepo.EXPECT().
					Create(gomock.Any(), gomock.Any(), gomock.Any()).
					DoAndReturn(func(_ context.Context, _ string, i *mmodel.Instrument) (*mmodel.Instrument, error) {
						persisted = i

						return i, nil
					})
			}

			input := &mmodel.CreateInstrumentInput{
				LedgerID:         ledgerID,
				AccountID:        accountID,
				RegulatoryFields: tc.regulatoryFields,
			}

			result, err := uc.CreateInstrument(context.Background(), uuid.Must(libCommons.GenerateUUIDv7()).String(), holderID, input)

			if tc.expectedErr != nil {
				require.Error(t, err)
				assert.Nil(t, result)

				var validationErr pkg.ValidationError
				require.True(t, errors.As(err, &validationErr), "rejection must be a ValidationError, got %T", err)
				assert.Equal(t, tc.expectedErr.Error(), validationErr.Code)
				assert.Equal(t, cn.EntityInstrument, validationErr.EntityType)

				return
			}

			require.NoError(t, err)
			require.NotNil(t, persisted)
			require.NotNil(t, persisted.RegulatoryFields)
			assert.Equal(t, tc.expectedAccountType, persisted.RegulatoryFields.AccountType)
			assert.Equal(t, tc.regulatoryFields.ParticipantDocument, persisted.RegulatoryFields.ParticipantDocument)

			if tc.regulatoryFields.AccountType != nil && persisted.RegulatoryFields.AccountType != nil {
				assert.NotSame(t, tc.regulatoryFields.AccountType, persisted.RegulatoryFields.AccountType,
					"the entity must carry the normalized pointer, not the raw input pointer")
			}
		})
	}
}

func TestCreateInstrument_InvalidAccountTypeIsBusinessSpanEvent(t *testing.T) {
	ctx, recorder := recordingContext()

	uc := &UseCase{}
	invalid := "CHECKING"

	_, err := uc.CreateInstrument(ctx, uuid.Must(libCommons.GenerateUUIDv7()).String(), uuid.Must(libCommons.GenerateUUIDv7()), &mmodel.CreateInstrumentInput{
		LedgerID:         uuid.Must(libCommons.GenerateUUIDv7()).String(),
		AccountID:        uuid.Must(libCommons.GenerateUUIDv7()).String(),
		RegulatoryFields: &mmodel.RegulatoryFields{AccountType: &invalid},
	})
	require.Error(t, err)

	span := findSpan(t, recorder, "service.create_instrument")

	assert.Equal(t, codes.Unset, span.Status().Code,
		"an invalid account type is a business failure and must leave the span status UNSET")
	assert.True(t, hasEvent(span, "Failed to validate instrument account type"),
		"the rejection must be recorded as a business event on the use-case span")
}
