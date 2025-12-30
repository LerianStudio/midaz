package command

import (
	"context"
	"errors"
	"testing"

	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/mongodb"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
)

func TestDeleteMetadataIndex(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockMetadataRepo := mongodb.NewMockRepository(ctrl)

	uc := &UseCase{
		MetadataRepo: mockMetadataRepo,
	}

	ctx := context.Background()

	tests := []struct {
		name        string
		entityName  string
		indexName   string
		setupMocks  func()
		expectedErr bool
		errContains string
	}{
		{
			name:       "success - delete metadata index",
			entityName: "transaction",
			indexName:  "metadata.tier_1",
			setupMocks: func() {
				mockMetadataRepo.EXPECT().
					DeleteIndex(gomock.Any(), "transaction", "metadata.tier_1").
					Return(nil).
					Times(1)
			},
			expectedErr: false,
		},
		{
			name:       "success - delete metadata index from operation entity",
			entityName: "operation",
			indexName:  "metadata.category_1",
			setupMocks: func() {
				mockMetadataRepo.EXPECT().
					DeleteIndex(gomock.Any(), "operation", "metadata.category_1").
					Return(nil).
					Times(1)
			},
			expectedErr: false,
		},
		{
			name:       "success - delete metadata index from operation_route entity",
			entityName: "operation_route",
			indexName:  "metadata.external_id_1",
			setupMocks: func() {
				mockMetadataRepo.EXPECT().
					DeleteIndex(gomock.Any(), "operation_route", "metadata.external_id_1").
					Return(nil).
					Times(1)
			},
			expectedErr: false,
		},
		{
			name:       "success - delete metadata index from transaction_route entity",
			entityName: "transaction_route",
			indexName:  "metadata.priority_1",
			setupMocks: func() {
				mockMetadataRepo.EXPECT().
					DeleteIndex(gomock.Any(), "transaction_route", "metadata.priority_1").
					Return(nil).
					Times(1)
			},
			expectedErr: false,
		},
		{
			name:        "failure - index name without metadata prefix",
			entityName:  "transaction",
			indexName:   "tier_1",
			setupMocks:  func() {},
			expectedErr: true,
			errContains: "0137",
		},
		{
			name:        "failure - index name with wrong prefix",
			entityName:  "transaction",
			indexName:   "custom.tier_1",
			setupMocks:  func() {},
			expectedErr: true,
			errContains: "0137",
		},
		{
			name:        "failure - empty index name",
			entityName:  "transaction",
			indexName:   "",
			setupMocks:  func() {},
			expectedErr: true,
			errContains: "0137",
		},
		{
			name:       "failure - repository error",
			entityName: "transaction",
			indexName:  "metadata.tier_1",
			setupMocks: func() {
				mockMetadataRepo.EXPECT().
					DeleteIndex(gomock.Any(), "transaction", "metadata.tier_1").
					Return(errors.New("failed to delete index")).
					Times(1)
			},
			expectedErr: true,
			errContains: "failed to delete index",
		},
		{
			name:       "failure - index not found",
			entityName: "transaction",
			indexName:  "metadata.nonexistent_1",
			setupMocks: func() {
				mockMetadataRepo.EXPECT().
					DeleteIndex(gomock.Any(), "transaction", "metadata.nonexistent_1").
					Return(errors.New("0130")).
					Times(1)
			},
			expectedErr: true,
			errContains: "0130",
		},
		{
			name:       "failure - database connection error",
			entityName: "operation",
			indexName:  "metadata.field_1",
			setupMocks: func() {
				mockMetadataRepo.EXPECT().
					DeleteIndex(gomock.Any(), "operation", "metadata.field_1").
					Return(errors.New("database connection error")).
					Times(1)
			},
			expectedErr: true,
			errContains: "database connection error",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.setupMocks()

			err := uc.DeleteMetadataIndex(ctx, tt.entityName, tt.indexName)

			if tt.expectedErr {
				assert.Error(t, err)
				if tt.errContains != "" {
					assert.Contains(t, err.Error(), tt.errContains)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// TestDeleteMetadataIndexValidatesPrefix tests that only indexes with "metadata." prefix can be deleted
func TestDeleteMetadataIndexValidatesPrefix(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockMetadataRepo := mongodb.NewMockRepository(ctrl)

	uc := &UseCase{
		MetadataRepo: mockMetadataRepo,
	}

	ctx := context.Background()

	invalidIndexNames := []string{
		"tier_1",
		"_id_",
		"entity_id_1",
		"created_at_1",
		"custom.field_1",
		"meta.field_1",
		"METADATA.field_1",
		"Metadata.field_1",
	}

	for _, indexName := range invalidIndexNames {
		t.Run(indexName, func(t *testing.T) {
			err := uc.DeleteMetadataIndex(ctx, "transaction", indexName)

			assert.Error(t, err)
			assert.Contains(t, err.Error(), "0137")
		})
	}
}

// TestDeleteMetadataIndexValidPrefixes tests that indexes with valid "metadata." prefix are accepted
func TestDeleteMetadataIndexValidPrefixes(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockMetadataRepo := mongodb.NewMockRepository(ctrl)

	uc := &UseCase{
		MetadataRepo: mockMetadataRepo,
	}

	ctx := context.Background()

	validIndexNames := []string{
		"metadata.tier_1",
		"metadata.customer_id_1",
		"metadata.externalReference_1",
		"metadata.level1.level2_1",
		"metadata.field_with_underscore_1",
	}

	for _, indexName := range validIndexNames {
		t.Run(indexName, func(t *testing.T) {
			mockMetadataRepo.EXPECT().
				DeleteIndex(gomock.Any(), "transaction", indexName).
				Return(nil).
				Times(1)

			err := uc.DeleteMetadataIndex(ctx, "transaction", indexName)

			assert.NoError(t, err)
		})
	}
}

// TestDeleteMetadataIndexAllEntities tests deletion across all valid entity types
func TestDeleteMetadataIndexAllEntities(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockMetadataRepo := mongodb.NewMockRepository(ctrl)

	uc := &UseCase{
		MetadataRepo: mockMetadataRepo,
	}

	ctx := context.Background()

	entities := []string{
		"transaction",
		"operation",
		"operation_route",
		"transaction_route",
	}

	for _, entity := range entities {
		t.Run(entity, func(t *testing.T) {
			indexName := "metadata.test_field_1"

			mockMetadataRepo.EXPECT().
				DeleteIndex(gomock.Any(), entity, indexName).
				Return(nil).
				Times(1)

			err := uc.DeleteMetadataIndex(ctx, entity, indexName)

			assert.NoError(t, err)
		})
	}
}
