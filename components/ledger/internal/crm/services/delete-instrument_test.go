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
	pkgStreaming "github.com/LerianStudio/midaz/v4/pkg/streaming"
	"github.com/LerianStudio/midaz/v4/pkg/streaming/events"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

func TestDeleteInstrumentByID(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockInstrumentRepo := instrument.NewMockRepository(ctrl)

	uc := &UseCase{
		InstrumentRepo: mockInstrumentRepo,
	}

	id := uuid.Must(libCommons.GenerateUUIDv7())
	holderID := uuid.Must(libCommons.GenerateUUIDv7())

	testCases := []struct {
		name          string
		holderID      uuid.UUID
		id            uuid.UUID
		mockSetup     func()
		expectedError error
	}{
		{
			name:     "Success deleting instrument by ID",
			holderID: holderID,
			id:       id,
			mockSetup: func() {
				mockInstrumentRepo.EXPECT().
					Delete(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), false).
					Return(nil)
			},
			expectedError: nil,
		},
		{
			name:     "Error when repository fails to delete instrument",
			holderID: holderID,
			id:       id,
			mockSetup: func() {
				mockInstrumentRepo.EXPECT().
					Delete(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), false).
					Return(errors.New("database error"))
			},
			expectedError: errors.New("database error"),
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			testCase.mockSetup()

			ctx := context.Background()
			err := uc.DeleteInstrumentByID(ctx, uuid.Must(libCommons.GenerateUUIDv7()).String(), testCase.holderID, testCase.id, false)

			if testCase.expectedError != nil {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestDeleteInstrumentByID_EmitsInstrumentDeleted(t *testing.T) {
	subCases := []struct {
		name                 string
		hardDelete           bool
		expectedDeletionType string
	}{
		{name: "soft delete", hardDelete: false, expectedDeletionType: "soft"},
		{name: "hard delete", hardDelete: true, expectedDeletionType: "hard"},
	}

	for _, sc := range subCases {
		t.Run(sc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockInstrumentRepo := instrument.NewMockRepository(ctrl)

			holderID := uuid.Must(libCommons.GenerateUUIDv7())
			instrumentID := uuid.Must(libCommons.GenerateUUIDv7())
			orgID := uuid.Must(libCommons.GenerateUUIDv7()).String()

			emitter := pkgStreaming.NewMockEmitter()

			uc := &UseCase{
				InstrumentRepo: mockInstrumentRepo,
				Streaming:      emitter,
			}

			mockInstrumentRepo.EXPECT().
				Delete(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), sc.hardDelete).
				Return(nil)

			ctx := context.Background()
			err := uc.DeleteInstrumentByID(ctx, orgID, holderID, instrumentID, sc.hardDelete)

			require.NoError(t, err)

			emitted := emitter.Events()
			require.Len(t, emitted, 1)
			assert.Equal(t, events.InstrumentDeletedDefinition.Key(), emitted[0].DefinitionKey)
			assert.Equal(t, instrumentID.String(), emitted[0].Subject)

			var payload struct {
				ID           string `json:"id"`
				HolderID     string `json:"holderId"`
				DeletionType string `json:"deletionType"`
			}
			require.NoError(t, json.Unmarshal(emitted[0].Payload, &payload))
			assert.Equal(t, sc.expectedDeletionType, payload.DeletionType)
			assert.Equal(t, instrumentID.String(), payload.ID)
			assert.Equal(t, holderID.String(), payload.HolderID)
			pkgStreaming.AssertEventEmitted(t, emitter, "instrument", "deleted")
		})
	}
}

func TestDeleteInstrumentByID_NilEmitterSucceeds(t *testing.T) {
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
		Delete(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), false).
		Return(nil)

	ctx := context.Background()
	err := uc.DeleteInstrumentByID(ctx, uuid.Must(libCommons.GenerateUUIDv7()).String(), holderID, instrumentID, false)

	require.NoError(t, err)
}

func TestDeleteInstrumentByID_EmitFailureDoesNotFailRequest(t *testing.T) {
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
		Delete(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), false).
		Return(nil)

	ctx := context.Background()
	err := uc.DeleteInstrumentByID(ctx, uuid.Must(libCommons.GenerateUUIDv7()).String(), holderID, instrumentID, false)

	require.NoError(t, err)
	assert.Empty(t, emitter.Events())
}
