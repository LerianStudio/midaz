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
	"github.com/LerianStudio/midaz/v3/components/crm/internal/adapters/mongodb/alias"
	pkgStreaming "github.com/LerianStudio/midaz/v3/pkg/streaming"
	"github.com/LerianStudio/midaz/v3/pkg/streaming/events"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

func TestDeleteAliasByID(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockAliasRepo := alias.NewMockRepository(ctrl)

	uc := &UseCase{
		AliasRepo: mockAliasRepo,
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
			name:     "Success deleting alias by ID",
			holderID: holderID,
			id:       id,
			mockSetup: func() {
				mockAliasRepo.EXPECT().
					Delete(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), false).
					Return(nil)
			},
			expectedError: nil,
		},
		{
			name:     "Error when repository fails to delete alias",
			holderID: holderID,
			id:       id,
			mockSetup: func() {
				mockAliasRepo.EXPECT().
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
			err := uc.DeleteAliasByID(ctx, uuid.Must(libCommons.GenerateUUIDv7()).String(), testCase.holderID, testCase.id, false)

			if testCase.expectedError != nil {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestDeleteAliasByID_EmitsAliasDeleted(t *testing.T) {
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

			mockAliasRepo := alias.NewMockRepository(ctrl)

			holderID := uuid.Must(libCommons.GenerateUUIDv7())
			aliasID := uuid.Must(libCommons.GenerateUUIDv7())
			orgID := uuid.Must(libCommons.GenerateUUIDv7()).String()

			emitter := pkgStreaming.NewMockEmitter()

			uc := &UseCase{
				AliasRepo: mockAliasRepo,
				Streaming: emitter,
			}

			mockAliasRepo.EXPECT().
				Delete(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), sc.hardDelete).
				Return(nil)

			ctx := context.Background()
			err := uc.DeleteAliasByID(ctx, orgID, holderID, aliasID, sc.hardDelete)

			require.NoError(t, err)

			emitted := emitter.Events()
			require.Len(t, emitted, 1)
			assert.Equal(t, events.AliasDeletedDefinition.Key(), emitted[0].DefinitionKey)
			assert.Equal(t, aliasID.String(), emitted[0].Subject)

			var payload struct {
				ID           string `json:"id"`
				HolderID     string `json:"holderId"`
				DeletionType string `json:"deletionType"`
			}
			require.NoError(t, json.Unmarshal(emitted[0].Payload, &payload))
			assert.Equal(t, sc.expectedDeletionType, payload.DeletionType)
			assert.Equal(t, aliasID.String(), payload.ID)
			assert.Equal(t, holderID.String(), payload.HolderID)
			pkgStreaming.AssertEventEmitted(t, emitter, "alias", "deleted")
		})
	}
}

func TestDeleteAliasByID_NilEmitterSucceeds(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockAliasRepo := alias.NewMockRepository(ctrl)

	holderID := uuid.Must(libCommons.GenerateUUIDv7())
	aliasID := uuid.Must(libCommons.GenerateUUIDv7())

	uc := &UseCase{
		AliasRepo: mockAliasRepo,
		Streaming: nil,
	}

	mockAliasRepo.EXPECT().
		Delete(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), false).
		Return(nil)

	ctx := context.Background()
	err := uc.DeleteAliasByID(ctx, uuid.Must(libCommons.GenerateUUIDv7()).String(), holderID, aliasID, false)

	require.NoError(t, err)
}

func TestDeleteAliasByID_EmitFailureDoesNotFailRequest(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockAliasRepo := alias.NewMockRepository(ctrl)

	holderID := uuid.Must(libCommons.GenerateUUIDv7())
	aliasID := uuid.Must(libCommons.GenerateUUIDv7())

	emitter := pkgStreaming.NewMockEmitter()
	emitter.SetError(errors.New("broker unavailable"))

	uc := &UseCase{
		AliasRepo: mockAliasRepo,
		Streaming: emitter,
	}

	mockAliasRepo.EXPECT().
		Delete(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), false).
		Return(nil)

	ctx := context.Background()
	err := uc.DeleteAliasByID(ctx, uuid.Must(libCommons.GenerateUUIDv7()).String(), holderID, aliasID, false)

	require.NoError(t, err)
	assert.Empty(t, emitter.Events())
}
