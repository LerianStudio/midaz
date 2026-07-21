// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package services

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	libCommons "github.com/LerianStudio/lib-commons/v5/commons"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/crm/adapters/mongodb/instrument"
	cn "github.com/LerianStudio/midaz/v4/pkg/constant"
	pkgStreaming "github.com/LerianStudio/midaz/v4/pkg/streaming"
	"github.com/LerianStudio/midaz/v4/pkg/streaming/events"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

func TestDeleteRelatedPartyByID(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	mockInstrumentRepo := instrument.NewMockRepository(ctrl)

	uc := &UseCase{
		InstrumentRepo: mockInstrumentRepo,
	}

	organizationID := uuid.Must(libCommons.GenerateUUIDv7()).String()
	holderID := uuid.Must(libCommons.GenerateUUIDv7())
	instrumentID := uuid.Must(libCommons.GenerateUUIDv7())
	relatedPartyID := uuid.Must(libCommons.GenerateUUIDv7())

	errRepoGeneric := errors.New("connection refused")

	testCases := []struct {
		name           string
		organizationID string
		holderID       uuid.UUID
		instrumentID   uuid.UUID
		relatedPartyID uuid.UUID
		mockSetup      func()
		expectedError  error
		errContains    string
	}{
		{
			name:           "success_deleting_related_party",
			organizationID: organizationID,
			holderID:       holderID,
			instrumentID:   instrumentID,
			relatedPartyID: relatedPartyID,
			mockSetup: func() {
				mockInstrumentRepo.EXPECT().
					DeleteRelatedParty(gomock.Any(), organizationID, holderID, instrumentID, relatedPartyID).
					Return(nil)
			},
			expectedError: nil,
		},
		{
			name:           "error_instrument_not_found",
			organizationID: organizationID,
			holderID:       holderID,
			instrumentID:   instrumentID,
			relatedPartyID: relatedPartyID,
			mockSetup: func() {
				mockInstrumentRepo.EXPECT().
					DeleteRelatedParty(gomock.Any(), organizationID, holderID, instrumentID, relatedPartyID).
					Return(cn.ErrInstrumentNotFound)
			},
			expectedError: cn.ErrInstrumentNotFound,
			errContains:   "CRM-0008",
		},
		{
			name:           "error_related_party_not_found",
			organizationID: organizationID,
			holderID:       holderID,
			instrumentID:   instrumentID,
			relatedPartyID: relatedPartyID,
			mockSetup: func() {
				mockInstrumentRepo.EXPECT().
					DeleteRelatedParty(gomock.Any(), organizationID, holderID, instrumentID, relatedPartyID).
					Return(cn.ErrRelatedPartyNotFound)
			},
			expectedError: cn.ErrRelatedPartyNotFound,
			errContains:   "CRM-0024",
		},
		{
			name:           "error_repository_timeout",
			organizationID: organizationID,
			holderID:       holderID,
			instrumentID:   instrumentID,
			relatedPartyID: relatedPartyID,
			mockSetup: func() {
				mockInstrumentRepo.EXPECT().
					DeleteRelatedParty(gomock.Any(), organizationID, holderID, instrumentID, relatedPartyID).
					Return(context.DeadlineExceeded)
			},
			expectedError: context.DeadlineExceeded,
			errContains:   "deadline exceeded",
		},
		{
			name:           "error_repository_generic",
			organizationID: organizationID,
			holderID:       holderID,
			instrumentID:   instrumentID,
			relatedPartyID: relatedPartyID,
			mockSetup: func() {
				mockInstrumentRepo.EXPECT().
					DeleteRelatedParty(gomock.Any(), organizationID, holderID, instrumentID, relatedPartyID).
					Return(errRepoGeneric)
			},
			expectedError: errRepoGeneric,
			errContains:   "connection refused",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			tc.mockSetup()

			ctx := context.Background()
			err := uc.DeleteRelatedPartyByID(ctx, tc.organizationID, tc.holderID, tc.instrumentID, tc.relatedPartyID)

			if tc.expectedError != nil {
				assert.Error(t, err)
				assert.ErrorIs(t, err, tc.expectedError)
				assert.Contains(t, err.Error(), tc.errContains)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestDeleteRelatedPartyByID_EmitsInstrumentRelatedPartyDeleted(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	mockInstrumentRepo := instrument.NewMockRepository(ctrl)

	organizationID := uuid.Must(libCommons.GenerateUUIDv7()).String()
	holderID := uuid.Must(libCommons.GenerateUUIDv7())
	instrumentID := uuid.Must(libCommons.GenerateUUIDv7())
	relatedPartyID := uuid.Must(libCommons.GenerateUUIDv7())

	emitter := pkgStreaming.NewMockEmitter()

	uc := &UseCase{
		InstrumentRepo: mockInstrumentRepo,
		Streaming:      emitter,
	}

	mockInstrumentRepo.EXPECT().
		DeleteRelatedParty(gomock.Any(), organizationID, holderID, instrumentID, relatedPartyID).
		Return(nil)

	ctx := context.Background()
	err := uc.DeleteRelatedPartyByID(ctx, organizationID, holderID, instrumentID, relatedPartyID)

	require.NoError(t, err)

	emitted := emitter.Events()
	require.Len(t, emitted, 1)
	assert.Equal(t, events.InstrumentRelatedPartyDeletedDefinition.Key(), emitted[0].DefinitionKey)

	// Subject is the INSTRUMENT ID (the aggregate), NOT the related-party ID.
	assert.Equal(t, instrumentID.String(), emitted[0].Subject)
	assert.NotEqual(t, relatedPartyID.String(), emitted[0].Subject)

	var payload struct {
		InstrumentID   string `json:"instrumentId"`
		HolderID       string `json:"holderId"`
		OrganizationID string `json:"organizationId"`
		RelatedPartyID string `json:"relatedPartyId"`
	}
	require.NoError(t, json.Unmarshal(emitted[0].Payload, &payload))
	assert.Equal(t, instrumentID.String(), payload.InstrumentID)
	assert.Equal(t, holderID.String(), payload.HolderID)
	assert.Equal(t, organizationID, payload.OrganizationID)
	assert.Equal(t, relatedPartyID.String(), payload.RelatedPartyID)
	pkgStreaming.AssertEventEmitted(t, emitter, "instrument", "related-party-deleted")
}

func TestDeleteRelatedPartyByID_NilEmitterSucceeds(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	mockInstrumentRepo := instrument.NewMockRepository(ctrl)

	organizationID := uuid.Must(libCommons.GenerateUUIDv7()).String()
	holderID := uuid.Must(libCommons.GenerateUUIDv7())
	instrumentID := uuid.Must(libCommons.GenerateUUIDv7())
	relatedPartyID := uuid.Must(libCommons.GenerateUUIDv7())

	uc := &UseCase{
		InstrumentRepo: mockInstrumentRepo,
		Streaming:      nil,
	}

	mockInstrumentRepo.EXPECT().
		DeleteRelatedParty(gomock.Any(), organizationID, holderID, instrumentID, relatedPartyID).
		Return(nil)

	ctx := context.Background()
	err := uc.DeleteRelatedPartyByID(ctx, organizationID, holderID, instrumentID, relatedPartyID)

	require.NoError(t, err)
}

func TestDeleteRelatedPartyByID_EmitFailureDoesNotFailRequest(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	mockInstrumentRepo := instrument.NewMockRepository(ctrl)

	organizationID := uuid.Must(libCommons.GenerateUUIDv7()).String()
	holderID := uuid.Must(libCommons.GenerateUUIDv7())
	instrumentID := uuid.Must(libCommons.GenerateUUIDv7())
	relatedPartyID := uuid.Must(libCommons.GenerateUUIDv7())

	emitter := pkgStreaming.NewMockEmitter()
	emitter.SetError(errors.New("broker unavailable"))

	uc := &UseCase{
		InstrumentRepo: mockInstrumentRepo,
		Streaming:      emitter,
	}

	mockInstrumentRepo.EXPECT().
		DeleteRelatedParty(gomock.Any(), organizationID, holderID, instrumentID, relatedPartyID).
		Return(nil)

	ctx := context.Background()
	err := uc.DeleteRelatedPartyByID(ctx, organizationID, holderID, instrumentID, relatedPartyID)

	require.NoError(t, err)
	assert.Empty(t, emitter.Events())
}
