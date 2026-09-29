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

func TestUpdateTransactionMetadataWritesOnlyClientKeys(t *testing.T) {
	transactionStored := map[string]any{
		"purpose": "client", constant.MetadataKeyFeeDebtOpenings: `[{"debtId":"d"}]`, constant.MetadataKeyFeeDebtSettlements: `[]`,
		"feeApplied": "true", "packageAppliedID": "package-1",
	}
	operationStored := map[string]any{"note": "client", constant.MetadataKeyFeeLeg: "true", constant.MetadataKeyFeeDeferPair: "pair-0"}
	stored := map[string]any{"stored": "document"}

	for _, scenario := range []struct {
		name, entity   string
		stored, sent   map[string]any
		fields         map[string]any
		readsNoStorage bool
	}{
		{name: "cleared transaction", entity: constant.EntityTransaction, stored: transactionStored, fields: map[string]any{"purpose": nil}},
		{name: "cleared operation", entity: constant.EntityOperation, stored: operationStored, fields: map[string]any{"note": nil}},
		{name: "cleared transaction without document", entity: constant.EntityTransaction, fields: map[string]any{}},
		{
			name: "merged transaction", entity: constant.EntityTransaction, readsNoStorage: true,
			sent: map[string]any{"purpose": "edited", "gone": nil}, fields: map[string]any{"purpose": "edited", "gone": nil},
		},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			repo := mongodb.NewMockRepository(gomock.NewController(t))
			if !scenario.readsNoStorage {
				var document *mongodb.Metadata
				if scenario.stored != nil {
					document = &mongodb.Metadata{Data: maps.Clone(scenario.stored)}
				}

				repo.EXPECT().FindByEntity(gomock.Any(), scenario.entity, "id").Return(document, nil)
			}

			repo.EXPECT().UpdateFields(gomock.Any(), scenario.entity, "id", scenario.fields).Return(&mongodb.Metadata{Data: stored}, nil)

			updated, err := (&UseCase{TransactionMetadataRepo: repo}).UpdateTransactionMetadata(context.Background(), scenario.entity, "id", scenario.sent)
			require.NoError(t, err)
			assert.Equal(t, stored, updated)
		})
	}

	t.Run("cleared route replaces the document", func(t *testing.T) {
		repo := mongodb.NewMockRepository(gomock.NewController(t))
		repo.EXPECT().Update(gomock.Any(), constant.EntityTransactionRoute, "id", map[string]any{}).Return(nil)

		updated, err := (&UseCase{TransactionMetadataRepo: repo}).UpdateTransactionMetadata(context.Background(), constant.EntityTransactionRoute, "id", nil)
		require.NoError(t, err)
		assert.Empty(t, updated)
	})
}
