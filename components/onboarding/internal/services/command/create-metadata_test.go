package command

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/LerianStudio/midaz/components/onboarding/internal/adapters/mongodb"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
)

// \1 performs an operation
func TestCreateMetadata(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockMetadataRepo := mongodb.NewMockRepository(ctrl)

	uc := &UseCase{
		MetadataRepo: mockMetadataRepo,
	}

	ctx := context.Background()

	tests := []struct {
		name         string
		entityName   string
		entityID     string
		metadata     map[string]any
		mockSetup    func()
		expectedErr  error
		expectedMeta map[string]any
	}{
		{
			name:       "success - metadata created",
			entityName: "TestEntity",
			entityID:   "12345",
			metadata: map[string]any{
				"key1": "value1",
				"key2": "value2",
			},
			mockSetup: func() {
				meta := mongodb.Metadata{
					EntityID:   "12345",
					EntityName: "TestEntity",
					Data: map[string]any{
						"key1": "value1",
						"key2": "value2",
					},
					CreatedAt: time.Now(),
					UpdatedAt: time.Now(),
				}

				mockMetadataRepo.EXPECT().
					Create(gomock.Any(), "TestEntity", gomock.Any()).
					DoAndReturn(func(ctx context.Context, entityName string, metadata *mongodb.Metadata) error {
						assert.Equal(t, meta.EntityID, metadata.EntityID)
						assert.Equal(t, meta.EntityName, metadata.EntityName)
						assert.Equal(t, meta.Data, metadata.Data)
						return nil
					}).
					Times(1)
			},
			expectedErr: nil,
			expectedMeta: map[string]any{
				"key1": "value1",
				"key2": "value2",
			},
		},
		{
			name:       "failure - error creating metadata",
			entityName: "TestEntity",
			entityID:   "12345",
			metadata: map[string]any{
				"key1": "value1",
				"key2": "value2",
			},
			mockSetup: func() {
				mockMetadataRepo.EXPECT().
					Create(gomock.Any(), "TestEntity", gomock.Any()).
					Return(errors.New("failed to create metadata")).
					Times(1)
			},
			expectedErr:  errors.New("failed to create metadata"),
			expectedMeta: nil,
		},
		{
			name:         "no metadata provided",
			entityName:   "TestEntity",
			entityID:     "12345",
			metadata:     nil,
			mockSetup:    func() {},
			expectedErr:  nil,
			expectedMeta: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.mockSetup()

			result, err := uc.CreateMetadata(ctx, tt.entityName, tt.entityID, tt.metadata)

			if tt.expectedErr != nil {
				assert.Error(t, err)
				assert.Equal(t, tt.expectedErr.Error(), err.Error())
				assert.Nil(t, result)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expectedMeta, result)
			}
		})
	}
}
