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
	"github.com/LerianStudio/midaz/v3/components/crm/internal/adapters/mongodb/holder"
	cn "github.com/LerianStudio/midaz/v3/pkg/constant"
	pkgStreaming "github.com/LerianStudio/midaz/v3/pkg/streaming"
	"github.com/LerianStudio/midaz/v3/pkg/streaming/events"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

func TestDeleteHolderByID(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockHolderRepo := holder.NewMockRepository(ctrl)
	mockAliasRepo := alias.NewMockRepository(ctrl)

	uc := &UseCase{
		HolderRepo: mockHolderRepo,
		AliasRepo:  mockAliasRepo,
	}

	holderID := uuid.Must(libCommons.GenerateUUIDv7())

	testCases := []struct {
		name        string
		holderID    uuid.UUID
		mockSetup   func()
		expectError bool
	}{
		{
			name:     "Success deleting holder by ID",
			holderID: holderID,
			mockSetup: func() {
				mockAliasRepo.EXPECT().
					Count(gomock.Any(), gomock.Any(), gomock.Any()).
					Return(int64(0), nil)
				mockHolderRepo.EXPECT().
					Delete(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
					Return(nil)
			},
			expectError: false,
		},
		{
			name:     "Error when holder not found by ID",
			holderID: holderID,
			mockSetup: func() {
				mockAliasRepo.EXPECT().
					Count(gomock.Any(), gomock.Any(), gomock.Any()).
					Return(int64(0), nil)
				mockHolderRepo.EXPECT().
					Delete(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
					Return(cn.ErrHolderNotFound)
			},
			expectError: true,
		},
		{
			name:     "Error when holder has linked accounts",
			holderID: holderID,
			mockSetup: func() {
				mockAliasRepo.EXPECT().
					Count(gomock.Any(), gomock.Any(), gomock.Any()).
					Return(int64(1), nil)
			},
			expectError: true,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			testCase.mockSetup()

			ctx := context.Background()
			err := uc.DeleteHolderByID(ctx, uuid.New().String(), holderID, false)

			if testCase.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestDeleteHolderByID_EmitsHolderDeleted(t *testing.T) {
	subCases := []struct {
		name                 string
		hardDelete           bool
		expectedDeletionType string
	}{
		{name: "soft delete", hardDelete: false, expectedDeletionType: "soft"},
		{name: "hard delete", hardDelete: true, expectedDeletionType: "hard"},
	}

	for _, sub := range subCases {
		t.Run(sub.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockHolderRepo := holder.NewMockRepository(ctrl)
			mockAliasRepo := alias.NewMockRepository(ctrl)

			emitter := pkgStreaming.NewMockEmitter()

			uc := &UseCase{
				HolderRepo: mockHolderRepo,
				AliasRepo:  mockAliasRepo,
				Streaming:  emitter,
			}

			holderID := uuid.Must(libCommons.GenerateUUIDv7())

			mockAliasRepo.EXPECT().
				Count(gomock.Any(), gomock.Any(), gomock.Any()).
				Return(int64(0), nil)
			mockHolderRepo.EXPECT().
				Delete(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
				Return(nil)

			ctx := context.Background()
			err := uc.DeleteHolderByID(ctx, uuid.New().String(), holderID, sub.hardDelete)

			require.NoError(t, err)

			emitted := emitter.Events()
			require.Len(t, emitted, 1)
			assert.Equal(t, events.HolderDeletedDefinition.Key(), emitted[0].DefinitionKey)
			assert.Equal(t, holderID.String(), emitted[0].Subject)

			var payload struct {
				DeletionType string `json:"deletionType"`
			}
			require.NoError(t, json.Unmarshal(emitted[0].Payload, &payload))
			assert.Equal(t, sub.expectedDeletionType, payload.DeletionType)

			pkgStreaming.AssertEventEmitted(t, emitter, "holder", "deleted")
		})
	}
}

func TestDeleteHolderByID_NilEmitterSucceeds(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockHolderRepo := holder.NewMockRepository(ctrl)
	mockAliasRepo := alias.NewMockRepository(ctrl)

	uc := &UseCase{
		HolderRepo: mockHolderRepo,
		AliasRepo:  mockAliasRepo,
		Streaming:  nil,
	}

	holderID := uuid.Must(libCommons.GenerateUUIDv7())

	mockAliasRepo.EXPECT().
		Count(gomock.Any(), gomock.Any(), gomock.Any()).
		Return(int64(0), nil)
	mockHolderRepo.EXPECT().
		Delete(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return(nil)

	ctx := context.Background()
	err := uc.DeleteHolderByID(ctx, uuid.New().String(), holderID, false)

	require.NoError(t, err)
}

func TestDeleteHolderByID_EmitFailureDoesNotFailRequest(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockHolderRepo := holder.NewMockRepository(ctrl)
	mockAliasRepo := alias.NewMockRepository(ctrl)

	emitter := pkgStreaming.NewMockEmitter()
	emitter.SetError(errors.New("broker unavailable"))

	uc := &UseCase{
		HolderRepo: mockHolderRepo,
		AliasRepo:  mockAliasRepo,
		Streaming:  emitter,
	}

	holderID := uuid.Must(libCommons.GenerateUUIDv7())

	mockAliasRepo.EXPECT().
		Count(gomock.Any(), gomock.Any(), gomock.Any()).
		Return(int64(0), nil)
	mockHolderRepo.EXPECT().
		Delete(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return(nil)

	ctx := context.Background()
	err := uc.DeleteHolderByID(ctx, uuid.New().String(), holderID, false)

	require.NoError(t, err)
}
