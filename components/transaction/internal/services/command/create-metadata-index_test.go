// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"errors"
	"testing"

	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/mongodb"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/LerianStudio/midaz/v3/pkg/utils"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
)

func TestCreateMetadataIndex(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockMetadataRepo := mongodb.NewMockRepository(ctrl)

	uc := &UseCase{
		MetadataRepo: mockMetadataRepo,
	}

	ctx := context.Background()

	tests := []struct {
		name           string
		entityName     string
		input          *mmodel.CreateMetadataIndexInput
		setupMocks     func()
		expectedErr    error
		validateError  func(t *testing.T, err error)
		validateResult func(t *testing.T, result *mmodel.MetadataIndex)
	}{
		{
			name:       "success - create index with default sparse value",
			entityName: "transaction",
			input: &mmodel.CreateMetadataIndexInput{
				MetadataKey: "tier",
				Unique:      false,
				Sparse:      nil,
			},
			setupMocks: func() {
				mockMetadataRepo.EXPECT().
					FindAllIndexes(gomock.Any(), "transaction").
					Return([]*mmodel.MetadataIndex{}, nil).
					Times(1)
				mockMetadataRepo.EXPECT().
					CreateIndex(gomock.Any(), "transaction", gomock.Any()).
					DoAndReturn(func(_ context.Context, _ string, _ *mmodel.CreateMetadataIndexInput) (*mmodel.MetadataIndex, error) {
						return &mmodel.MetadataIndex{
							IndexName:   "metadata.tier_1",
							EntityName:  "transaction",
							MetadataKey: "tier",
							Unique:      false,
							Sparse:      true,
						}, nil
					}).
					Times(1)
			},
			expectedErr: nil,
			validateResult: func(t *testing.T, result *mmodel.MetadataIndex) {
				assert.Equal(t, "metadata.tier_1", result.IndexName)
				assert.Equal(t, "transaction", result.EntityName)
				assert.Equal(t, "tier", result.MetadataKey)
				assert.False(t, result.Unique)
				assert.True(t, result.Sparse)
			},
		},
		{
			name:       "success - create index with sparse explicitly set to true",
			entityName: "operation",
			input: &mmodel.CreateMetadataIndexInput{
				MetadataKey: "category",
				Unique:      true,
				Sparse:      utils.BoolPtr(true),
			},
			setupMocks: func() {
				mockMetadataRepo.EXPECT().
					FindAllIndexes(gomock.Any(), "operation").
					Return([]*mmodel.MetadataIndex{}, nil).
					Times(1)
				mockMetadataRepo.EXPECT().
					CreateIndex(gomock.Any(), "operation", gomock.Any()).
					Return(&mmodel.MetadataIndex{
						IndexName:   "metadata.category_1",
						EntityName:  "operation",
						MetadataKey: "category",
						Unique:      true,
						Sparse:      true,
					}, nil).
					Times(1)
			},
			expectedErr: nil,
			validateResult: func(t *testing.T, result *mmodel.MetadataIndex) {
				assert.Equal(t, "metadata.category_1", result.IndexName)
				assert.Equal(t, "operation", result.EntityName)
				assert.Equal(t, "category", result.MetadataKey)
				assert.True(t, result.Unique)
				assert.True(t, result.Sparse)
			},
		},
		{
			name:       "success - create index with sparse explicitly set to false",
			entityName: "transaction_route",
			input: &mmodel.CreateMetadataIndexInput{
				MetadataKey: "priority",
				Unique:      false,
				Sparse:      utils.BoolPtr(false),
			},
			setupMocks: func() {
				mockMetadataRepo.EXPECT().
					FindAllIndexes(gomock.Any(), "transaction_route").
					Return([]*mmodel.MetadataIndex{}, nil).
					Times(1)
				mockMetadataRepo.EXPECT().
					CreateIndex(gomock.Any(), "transaction_route", gomock.Any()).
					Return(&mmodel.MetadataIndex{
						IndexName:   "metadata.priority_1",
						EntityName:  "transaction_route",
						MetadataKey: "priority",
						Unique:      false,
						Sparse:      false,
					}, nil).
					Times(1)
			},
			expectedErr: nil,
			validateResult: func(t *testing.T, result *mmodel.MetadataIndex) {
				assert.Equal(t, "metadata.priority_1", result.IndexName)
				assert.Equal(t, "transaction_route", result.EntityName)
				assert.Equal(t, "priority", result.MetadataKey)
				assert.False(t, result.Unique)
				assert.False(t, result.Sparse)
			},
		},
		{
			name:       "success - create unique index",
			entityName: "operation_route",
			input: &mmodel.CreateMetadataIndexInput{
				MetadataKey: "external_id",
				Unique:      true,
				Sparse:      utils.BoolPtr(true),
			},
			setupMocks: func() {
				mockMetadataRepo.EXPECT().
					FindAllIndexes(gomock.Any(), "operation_route").
					Return([]*mmodel.MetadataIndex{}, nil).
					Times(1)
				mockMetadataRepo.EXPECT().
					CreateIndex(gomock.Any(), "operation_route", gomock.Any()).
					Return(&mmodel.MetadataIndex{
						IndexName:   "metadata.external_id_1",
						EntityName:  "operation_route",
						MetadataKey: "external_id",
						Unique:      true,
						Sparse:      true,
					}, nil).
					Times(1)
			},
			expectedErr: nil,
			validateResult: func(t *testing.T, result *mmodel.MetadataIndex) {
				assert.Equal(t, "metadata.external_id_1", result.IndexName)
				assert.Equal(t, "operation_route", result.EntityName)
				assert.Equal(t, "external_id", result.MetadataKey)
				assert.True(t, result.Unique)
				assert.True(t, result.Sparse)
			},
		},
		{
			name:       "failure - repository error on create",
			entityName: "transaction",
			input: &mmodel.CreateMetadataIndexInput{
				MetadataKey: "tier",
				Unique:      false,
				Sparse:      nil,
			},
			setupMocks: func() {
				mockMetadataRepo.EXPECT().
					FindAllIndexes(gomock.Any(), "transaction").
					Return([]*mmodel.MetadataIndex{}, nil).
					Times(1)
				mockMetadataRepo.EXPECT().
					CreateIndex(gomock.Any(), "transaction", gomock.Any()).
					Return(nil, errors.New("failed to create index")).
					Times(1)
			},
			expectedErr:    errors.New("failed to create index"),
			validateResult: nil,
		},
		{
			name:       "failure - index already exists",
			entityName: "transaction",
			input: &mmodel.CreateMetadataIndexInput{
				MetadataKey: "existing_key",
				Unique:      false,
				Sparse:      nil,
			},
			setupMocks: func() {
				// FindAllIndexes returns MetadataKey WITHOUT the "metadata." prefix
				mockMetadataRepo.EXPECT().
					FindAllIndexes(gomock.Any(), "transaction").
					Return([]*mmodel.MetadataIndex{
						{MetadataKey: "existing_key", Unique: false, Sparse: true},
					}, nil).
					Times(1)
			},
			validateError: func(t *testing.T, err error) {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), "metadata index with the same key already exists")
			},
			validateResult: nil,
		},
		{
			name:       "failure - error checking existing indexes",
			entityName: "operation",
			input: &mmodel.CreateMetadataIndexInput{
				MetadataKey: "field",
				Unique:      false,
				Sparse:      utils.BoolPtr(true),
			},
			setupMocks: func() {
				mockMetadataRepo.EXPECT().
					FindAllIndexes(gomock.Any(), "operation").
					Return(nil, errors.New("database connection error")).
					Times(1)
			},
			expectedErr:    errors.New("database connection error"),
			validateResult: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.setupMocks()

			result, err := uc.CreateMetadataIndex(ctx, tt.entityName, tt.input)

			if tt.expectedErr != nil {
				assert.Error(t, err)
				assert.Equal(t, tt.expectedErr.Error(), err.Error())
				assert.Nil(t, result)
			} else if tt.validateError != nil {
				tt.validateError(t, err)
				assert.Nil(t, result)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, result)
				if tt.validateResult != nil {
					tt.validateResult(t, result)
				}
			}
		})
	}
}

// TestCreateMetadataIndexIndexNameFormat tests that the index name is formatted correctly
func TestCreateMetadataIndexIndexNameFormat(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockMetadataRepo := mongodb.NewMockRepository(ctrl)

	uc := &UseCase{
		MetadataRepo: mockMetadataRepo,
	}

	ctx := context.Background()

	testCases := []struct {
		metadataKey       string
		expectedIndexName string
	}{
		{"tier", "metadata.tier_1"},
		{"customer_id", "metadata.customer_id_1"},
		{"externalReference", "metadata.externalReference_1"},
		{"level1.level2", "metadata.level1.level2_1"},
	}

	for _, tc := range testCases {
		t.Run(tc.metadataKey, func(t *testing.T) {
			input := &mmodel.CreateMetadataIndexInput{
				MetadataKey: tc.metadataKey,
				Unique:      false,
				Sparse:      nil,
			}

			mockMetadataRepo.EXPECT().
				FindAllIndexes(gomock.Any(), "transaction").
				Return([]*mmodel.MetadataIndex{}, nil).
				Times(1)
			mockMetadataRepo.EXPECT().
				CreateIndex(gomock.Any(), "transaction", gomock.Any()).
				Return(&mmodel.MetadataIndex{
					IndexName:   tc.expectedIndexName,
					EntityName:  "transaction",
					MetadataKey: tc.metadataKey,
					Unique:      false,
					Sparse:      true,
				}, nil).
				Times(1)

			result, err := uc.CreateMetadataIndex(ctx, "transaction", input)

			assert.NoError(t, err)
			assert.NotNil(t, result)
			assert.Equal(t, tc.expectedIndexName, result.IndexName)
		})
	}
}

// TestCreateMetadataIndexSparseDefaultValue tests that sparse defaults to true when nil
func TestCreateMetadataIndexSparseDefaultValue(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockMetadataRepo := mongodb.NewMockRepository(ctrl)

	uc := &UseCase{
		MetadataRepo: mockMetadataRepo,
	}

	ctx := context.Background()

	input := &mmodel.CreateMetadataIndexInput{
		MetadataKey: "test_key",
		Unique:      false,
		Sparse:      nil,
	}

	mockMetadataRepo.EXPECT().
		FindAllIndexes(gomock.Any(), "transaction").
		Return([]*mmodel.MetadataIndex{}, nil).
		Times(1)
	mockMetadataRepo.EXPECT().
		CreateIndex(gomock.Any(), "transaction", gomock.Any()).
		Return(&mmodel.MetadataIndex{
			IndexName:   "metadata.test_key_1",
			EntityName:  "transaction",
			MetadataKey: "test_key",
			Unique:      false,
			Sparse:      true,
		}, nil).
		Times(1)

	result, err := uc.CreateMetadataIndex(ctx, "transaction", input)

	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.True(t, result.Sparse)
}
