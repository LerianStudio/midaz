// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/LerianStudio/midaz/v3/components/onboarding/internal/adapters/mongodb"
)

var errMetaCreation = errors.New("failed to create metadata")

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
					Return(errMetaCreation).
					Times(1)
			},
			expectedErr:  errMetaCreation,
			expectedMeta: nil,
		},
		{
			name:         "no metadata provided",
			entityName:   "TestEntity",
			entityID:     "12345",
			metadata:     nil,
			mockSetup:    func() {},
			expectedErr:  nil,
			expectedMeta: map[string]any{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.mockSetup()

			result, err := uc.CreateMetadata(ctx, tt.entityName, tt.entityID, tt.metadata)

			if tt.expectedErr != nil {
				require.Error(t, err)
				require.ErrorContains(t, err, tt.expectedErr.Error())
				assert.Nil(t, result)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.expectedMeta, result)
			}
		})
	}
}
