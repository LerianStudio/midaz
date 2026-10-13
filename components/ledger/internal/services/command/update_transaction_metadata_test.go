// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"errors"
	"maps"
	"testing"

	mongodb "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/mongodb/transaction"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

func TestUpdateTransactionMetadata(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockMetadataRepo := mongodb.NewMockRepository(ctrl)

	uc := &UseCase{
		TransactionMetadataRepo: mockMetadataRepo,
	}

	ctx := context.Background()
	entityName := "TestEntity"
	entityID := "123456"

	tests := []struct {
		name             string
		inputMetadata    map[string]any
		setupMocks       func()
		expectedErr      error
		expectedMetadata map[string]any
	}{
		{
			name: "success - metadata updated with new data",
			inputMetadata: map[string]any{
				"key1": "value1",
				"key2": "value2",
			},
			setupMocks: func() {
				mockMetadataRepo.EXPECT().
					FindByEntity(gomock.Any(), entityName, entityID).
					Return(nil, nil).
					Times(1)

				mockMetadataRepo.EXPECT().
					Update(gomock.Any(), entityName, entityID, gomock.Any()).
					Return(nil).
					Times(1)
			},
			expectedErr: nil,
			expectedMetadata: map[string]any{
				"key1": "value1",
				"key2": "value2",
			},
		},
		{
			name: "success - metadata updated with merged data",
			inputMetadata: map[string]any{
				"key2": "new_value2",
				"key3": "value3",
			},
			setupMocks: func() {
				mockMetadataRepo.EXPECT().
					FindByEntity(gomock.Any(), entityName, entityID).
					Return(&mongodb.Metadata{
						Data: map[string]any{
							"key1": "value1",
							"key2": "value2",
						},
					}, nil).
					Times(1)

				mockMetadataRepo.EXPECT().
					Update(gomock.Any(), entityName, entityID, gomock.Any()).
					DoAndReturn(func(_ context.Context, _, _ string, updatedMetadata map[string]any) error {
						expectedMerged := map[string]any{
							"key1": "value1",
							"key2": "new_value2",
							"key3": "value3",
						}
						assert.Equal(t, expectedMerged, updatedMetadata)
						return nil
					}).
					Times(1)
			},
			expectedErr: nil,
			expectedMetadata: map[string]any{
				"key1": "value1",
				"key2": "new_value2",
				"key3": "value3",
			},
		},
		{
			name:          "failure - error retrieving existing metadata",
			inputMetadata: map[string]any{"key1": "value1"},
			setupMocks: func() {
				mockMetadataRepo.EXPECT().
					FindByEntity(gomock.Any(), entityName, entityID).
					Return(nil, errors.New("failed to retrieve metadata")).
					Times(1)
			},
			expectedErr:      errors.New("failed to retrieve metadata"),
			expectedMetadata: nil,
		},
		{
			name: "failure - error updating metadata",
			inputMetadata: map[string]any{
				"key1": "value1",
			},
			setupMocks: func() {
				mockMetadataRepo.EXPECT().
					FindByEntity(gomock.Any(), entityName, entityID).
					Return(nil, nil).
					Times(1)

				mockMetadataRepo.EXPECT().
					Update(gomock.Any(), entityName, entityID, gomock.Any()).
					Return(errors.New("failed to update metadata")).
					Times(1)
			},
			expectedErr:      errors.New("failed to update metadata"),
			expectedMetadata: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.setupMocks()

			result, err := uc.UpdateTransactionMetadata(ctx, entityName, entityID, tt.inputMetadata)

			if tt.expectedErr != nil {
				assert.Error(t, err)
				assert.Equal(t, tt.expectedErr.Error(), err.Error())
				assert.Nil(t, result)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, result)
				assert.Equal(t, tt.expectedMetadata, result)
			}
		})
	}
}

// TestUpdateTransactionMetadata_Branches locks the three input branches of UpdateTransactionMetadata: nil
// (explicit null) clears an existing document and never creates one, an empty
// map (absent key or {}) reads the existing document without writing, and a
// non-empty map merges and upserts.
func TestUpdateTransactionMetadata_Branches(t *testing.T) {
	t.Parallel()

	const (
		entityName = "TestEntity"
		entityID   = "123456"
	)

	existingDoc := func() *mongodb.Metadata {
		return &mongodb.Metadata{
			EntityID:   entityID,
			EntityName: entityName,
			Data:       map[string]any{"k": "v"},
		}
	}

	tests := []struct {
		name             string
		inputMetadata    map[string]any
		existing         *mongodb.Metadata
		wantUpdate       bool
		wantUpdateData   map[string]any
		expectedMetadata map[string]any
	}{
		{
			name:             "nil input without document writes nothing",
			inputMetadata:    nil,
			existing:         nil,
			wantUpdate:       false,
			expectedMetadata: nil,
		},
		{
			name:             "nil input with document clears it",
			inputMetadata:    nil,
			existing:         existingDoc(),
			wantUpdate:       true,
			wantUpdateData:   map[string]any{},
			expectedMetadata: map[string]any{},
		},
		{
			name:             "empty input with document returns existing data without writing",
			inputMetadata:    map[string]any{},
			existing:         existingDoc(),
			wantUpdate:       false,
			expectedMetadata: map[string]any{"k": "v"},
		},
		{
			name:             "empty input without document writes nothing",
			inputMetadata:    map[string]any{},
			existing:         nil,
			wantUpdate:       false,
			expectedMetadata: nil,
		},
		{
			name:             "non-empty input without document creates it",
			inputMetadata:    map[string]any{"k": "v"},
			existing:         nil,
			wantUpdate:       true,
			wantUpdateData:   map[string]any{"k": "v"},
			expectedMetadata: map[string]any{"k": "v"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			t.Cleanup(ctrl.Finish)

			mockMetadataRepo := mongodb.NewMockRepository(ctrl)

			uc := &UseCase{
				TransactionMetadataRepo: mockMetadataRepo,
			}

			mockMetadataRepo.EXPECT().
				FindByEntity(gomock.Any(), entityName, entityID).
				Return(tt.existing, nil).
				Times(1)

			if tt.wantUpdate {
				mockMetadataRepo.EXPECT().
					Update(gomock.Any(), entityName, entityID, tt.wantUpdateData).
					Return(nil).
					Times(1)
			} else {
				mockMetadataRepo.EXPECT().
					Update(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
					Times(0)
			}

			result, err := uc.UpdateTransactionMetadata(context.Background(), entityName, entityID, tt.inputMetadata)

			assert.NoError(t, err)
			assert.Equal(t, tt.expectedMetadata, result)
		})
	}
}

func TestUpdateTransactionMetadataKeepsReservedKeysOfTheFreshRead(t *testing.T) {
	stored := func(data mongodb.JSON) *mongodb.Metadata { return &mongodb.Metadata{Data: maps.Clone(data)} }
	pending := mongodb.JSON{"purpose": "client", "feeApplied": "true"}
	settled := mongodb.JSON{"purpose": "client", "feeApplied": "true", constant.MetadataKeyFeeDebtSettlements: "[]"}
	absent := map[string]any{constant.MetadataKeyFeeDebtOpenings: nil, constant.MetadataKeyFeeDebtSettlements: nil}
	written := map[string]any{constant.MetadataKeyFeeDebtOpenings: nil, constant.MetadataKeyFeeDebtSettlements: "[]"}

	for _, scenario := range []struct {
		name, entity string
		sent, want   map[string]any
		expect       func(repo *mongodb.MockRepositoryMockRecorder, want map[string]any)
	}{
		{
			name: "clearing keeps only reserved keys", entity: constant.EntityTransaction,
			want: map[string]any{"feeApplied": "true"},
			expect: func(repo *mongodb.MockRepositoryMockRecorder, want map[string]any) {
				repo.FindByEntity(gomock.Any(), constant.EntityTransaction, "id").Return(stored(pending), nil)
				repo.UpdateIfUnchanged(gomock.Any(), constant.EntityTransaction, "id", "", want, absent).Return(true, nil)
			},
		},
		{
			name: "a concurrent fee-debt write makes it merge again", entity: constant.EntityOperation,
			sent: map[string]any{"purpose": "edited", "gone": nil},
			want: map[string]any{"purpose": "edited", "feeApplied": "true", constant.MetadataKeyFeeDebtSettlements: "[]"},
			expect: func(repo *mongodb.MockRepositoryMockRecorder, want map[string]any) {
				gomock.InOrder(
					repo.FindByEntity(gomock.Any(), constant.EntityOperation, "id").Return(stored(pending), nil),
					repo.UpdateIfUnchanged(gomock.Any(), constant.EntityOperation, "id", "", gomock.Any(), absent).Return(false, nil),
					repo.FindByEntity(gomock.Any(), constant.EntityOperation, "id").Return(stored(settled), nil),
					repo.UpdateIfUnchanged(gomock.Any(), constant.EntityOperation, "id", "", want, written).Return(true, nil),
				)
			},
		},
		{
			name: "a missing document is created before the merge", entity: constant.EntityTransaction,
			sent: map[string]any{"purpose": "first"}, want: map[string]any{"purpose": "first"},
			expect: func(repo *mongodb.MockRepositoryMockRecorder, want map[string]any) {
				gomock.InOrder(
					repo.FindByEntity(gomock.Any(), constant.EntityTransaction, "id").Return(nil, nil),
					repo.Create(gomock.Any(), constant.EntityTransaction, gomock.Any()).Return(nil),
					repo.FindByEntity(gomock.Any(), constant.EntityTransaction, "id").Return(stored(mongodb.JSON{}), nil),
					repo.UpdateIfUnchanged(gomock.Any(), constant.EntityTransaction, "id", "", want, absent).Return(true, nil),
				)
			},
		},
		{
			name: "a null-valued key deletes that key and keeps the reserved keys", entity: constant.EntityTransaction,
			sent: map[string]any{"purpose": nil}, want: map[string]any{"feeApplied": "true"},
			expect: func(repo *mongodb.MockRepositoryMockRecorder, want map[string]any) {
				repo.FindByEntity(gomock.Any(), constant.EntityTransaction, "id").Return(stored(pending), nil)
				repo.UpdateIfUnchanged(gomock.Any(), constant.EntityTransaction, "id", "", want, absent).Return(true, nil)
			},
		},
		{
			name: "nil input without a document writes nothing", entity: constant.EntityTransaction,
			sent: nil, want: nil,
			expect: func(repo *mongodb.MockRepositoryMockRecorder, _ map[string]any) {
				repo.FindByEntity(gomock.Any(), constant.EntityTransaction, "id").Return(nil, nil)
			},
		},
		{
			name: "empty input with a document returns it unchanged without writing", entity: constant.EntityOperation,
			sent: map[string]any{}, want: map[string]any(pending),
			expect: func(repo *mongodb.MockRepositoryMockRecorder, _ map[string]any) {
				repo.FindByEntity(gomock.Any(), constant.EntityOperation, "id").Return(stored(pending), nil)
			},
		},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			repo := mongodb.NewMockRepository(gomock.NewController(t))
			scenario.expect(repo.EXPECT(), scenario.want)

			updated, err := (&UseCase{TransactionMetadataRepo: repo}).UpdateTransactionMetadata(context.Background(), scenario.entity, "id", scenario.sent)
			require.NoError(t, err)
			assert.Equal(t, scenario.want, updated)
		})
	}
}
