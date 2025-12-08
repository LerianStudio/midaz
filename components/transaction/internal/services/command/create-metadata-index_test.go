package command

import (
	"context"
	"errors"
	"testing"

	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/mongodb"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/LerianStudio/midaz/v3/pkg/utils"
	"github.com/google/uuid"
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
	orgID := uuid.New()
	ledgerID := uuid.New()

	tests := []struct {
		name           string
		input          *mmodel.CreateMetadataIndexInput
		setupMocks     func()
		expectedErr    error
		validateResult func(t *testing.T, result *mmodel.MetadataIndex)
	}{
		{
			name: "success - create index with default sparse value",
			input: &mmodel.CreateMetadataIndexInput{
				EntityName:  "transaction",
				MetadataKey: "tier",
				Unique:      false,
				Sparse:      nil,
			},
			setupMocks: func() {
				mockMetadataRepo.EXPECT().
					CreateIndex(gomock.Any(), "transaction", &mongodb.MetadataIndex{
						EntityName:  "transaction",
						MetadataKey: "tier",
						Unique:      false,
						Sparse:      true,
					}).
					Return(&mongodb.MetadataIndex{
						EntityName:  "transaction",
						MetadataKey: "tier",
						Unique:      false,
						Sparse:      true,
					}, nil).
					Times(1)
			},
			expectedErr: nil,
			validateResult: func(t *testing.T, result *mmodel.MetadataIndex) {
				assert.Equal(t, "metadata.tier_1", result.IndexName)
				assert.Equal(t, "transaction", result.EntityName)
				assert.Equal(t, "tier", result.MetadataKey)
				assert.False(t, result.Unique)
				assert.True(t, result.Sparse)
				assert.False(t, result.CreatedAt.IsZero())
			},
		},
		{
			name: "success - create index with sparse explicitly set to true",
			input: &mmodel.CreateMetadataIndexInput{
				EntityName:  "operation",
				MetadataKey: "category",
				Unique:      true,
				Sparse:      utils.BoolPtr(true),
			},
			setupMocks: func() {
				mockMetadataRepo.EXPECT().
					CreateIndex(gomock.Any(), "operation", &mongodb.MetadataIndex{
						EntityName:  "operation",
						MetadataKey: "category",
						Unique:      true,
						Sparse:      true,
					}).
					Return(&mongodb.MetadataIndex{
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
			name: "success - create index with sparse explicitly set to false",
			input: &mmodel.CreateMetadataIndexInput{
				EntityName:  "transaction_route",
				MetadataKey: "priority",
				Unique:      false,
				Sparse:      utils.BoolPtr(false),
			},
			setupMocks: func() {
				mockMetadataRepo.EXPECT().
					CreateIndex(gomock.Any(), "transaction_route", &mongodb.MetadataIndex{
						EntityName:  "transaction_route",
						MetadataKey: "priority",
						Unique:      false,
						Sparse:      false,
					}).
					Return(&mongodb.MetadataIndex{
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
			name: "success - create unique index",
			input: &mmodel.CreateMetadataIndexInput{
				EntityName:  "operation_route",
				MetadataKey: "external_id",
				Unique:      true,
				Sparse:      utils.BoolPtr(true),
			},
			setupMocks: func() {
				mockMetadataRepo.EXPECT().
					CreateIndex(gomock.Any(), "operation_route", &mongodb.MetadataIndex{
						EntityName:  "operation_route",
						MetadataKey: "external_id",
						Unique:      true,
						Sparse:      true,
					}).
					Return(&mongodb.MetadataIndex{
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
			name: "failure - repository error",
			input: &mmodel.CreateMetadataIndexInput{
				EntityName:  "transaction",
				MetadataKey: "tier",
				Unique:      false,
				Sparse:      nil,
			},
			setupMocks: func() {
				mockMetadataRepo.EXPECT().
					CreateIndex(gomock.Any(), "transaction", gomock.Any()).
					Return(nil, errors.New("failed to create index")).
					Times(1)
			},
			expectedErr:    errors.New("failed to create index"),
			validateResult: nil,
		},
		{
			name: "failure - duplicate index error",
			input: &mmodel.CreateMetadataIndexInput{
				EntityName:  "transaction",
				MetadataKey: "existing_key",
				Unique:      false,
				Sparse:      nil,
			},
			setupMocks: func() {
				mockMetadataRepo.EXPECT().
					CreateIndex(gomock.Any(), "transaction", gomock.Any()).
					Return(nil, errors.New("index already exists")).
					Times(1)
			},
			expectedErr:    errors.New("index already exists"),
			validateResult: nil,
		},
		{
			name: "failure - database connection error",
			input: &mmodel.CreateMetadataIndexInput{
				EntityName:  "operation",
				MetadataKey: "field",
				Unique:      false,
				Sparse:      utils.BoolPtr(true),
			},
			setupMocks: func() {
				mockMetadataRepo.EXPECT().
					CreateIndex(gomock.Any(), "operation", gomock.Any()).
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

			result, err := uc.CreateMetadataIndex(ctx, orgID, ledgerID, tt.input)

			if tt.expectedErr != nil {
				assert.Error(t, err)
				assert.Equal(t, tt.expectedErr.Error(), err.Error())
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
	orgID := uuid.New()
	ledgerID := uuid.New()

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
				EntityName:  "transaction",
				MetadataKey: tc.metadataKey,
				Unique:      false,
				Sparse:      nil,
			}

			mockMetadataRepo.EXPECT().
				CreateIndex(gomock.Any(), "transaction", gomock.Any()).
				Return(&mongodb.MetadataIndex{
					EntityName:  "transaction",
					MetadataKey: tc.metadataKey,
					Unique:      false,
					Sparse:      true,
				}, nil).
				Times(1)

			result, err := uc.CreateMetadataIndex(ctx, orgID, ledgerID, input)

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
	orgID := uuid.New()
	ledgerID := uuid.New()

	input := &mmodel.CreateMetadataIndexInput{
		EntityName:  "transaction",
		MetadataKey: "test_key",
		Unique:      false,
		Sparse:      nil,
	}

	mockMetadataRepo.EXPECT().
		CreateIndex(gomock.Any(), "transaction", &mongodb.MetadataIndex{
			EntityName:  "transaction",
			MetadataKey: "test_key",
			Unique:      false,
			Sparse:      true,
		}).
		Return(&mongodb.MetadataIndex{
			EntityName:  "transaction",
			MetadataKey: "test_key",
			Unique:      false,
			Sparse:      true,
		}, nil).
		Times(1)

	result, err := uc.CreateMetadataIndex(ctx, orgID, ledgerID, input)

	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.True(t, result.Sparse)
}

// TestCreateMetadataIndexCreatedAtIsSet tests that CreatedAt is set on successful creation
func TestCreateMetadataIndexCreatedAtIsSet(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockMetadataRepo := mongodb.NewMockRepository(ctrl)

	uc := &UseCase{
		MetadataRepo: mockMetadataRepo,
	}

	ctx := context.Background()
	orgID := uuid.New()
	ledgerID := uuid.New()

	input := &mmodel.CreateMetadataIndexInput{
		EntityName:  "transaction",
		MetadataKey: "tier",
		Unique:      false,
		Sparse:      nil,
	}

	mockMetadataRepo.EXPECT().
		CreateIndex(gomock.Any(), "transaction", gomock.Any()).
		Return(&mongodb.MetadataIndex{
			EntityName:  "transaction",
			MetadataKey: "tier",
			Unique:      false,
			Sparse:      true,
		}, nil).
		Times(1)

	result, err := uc.CreateMetadataIndex(ctx, orgID, ledgerID, input)

	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.False(t, result.CreatedAt.IsZero())
}
