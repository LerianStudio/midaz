// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"errors"
	"testing"

	mongodb "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/mongodb/onboarding"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
)

func TestUpdateOnboardingMetadata(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockMetadataRepo := mongodb.NewMockRepository(ctrl)

	uc := &UseCase{
		OnboardingMetadataRepo: mockMetadataRepo,
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

			result, err := uc.UpdateOnboardingMetadata(ctx, entityName, entityID, tt.inputMetadata)

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

// TestUpdateOnboardingMetadata_Branches locks the three input branches of UpdateOnboardingMetadata: nil
// (explicit null) clears an existing document and never creates one, an empty
// map (absent key or {}) reads the existing document without writing, and a
// non-empty map merges and upserts.
func TestUpdateOnboardingMetadata_Branches(t *testing.T) {
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
		{
			name:             "null-valued key with document deletes that key and merges the rest",
			inputMetadata:    map[string]any{"k": nil, "n": "1"},
			existing:         existingDoc(),
			wantUpdate:       true,
			wantUpdateData:   map[string]any{"n": "1"},
			expectedMetadata: map[string]any{"n": "1"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			t.Cleanup(ctrl.Finish)

			mockMetadataRepo := mongodb.NewMockRepository(ctrl)

			uc := &UseCase{
				OnboardingMetadataRepo: mockMetadataRepo,
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

			result, err := uc.UpdateOnboardingMetadata(context.Background(), entityName, entityID, tt.inputMetadata)

			assert.NoError(t, err)
			assert.Equal(t, tt.expectedMetadata, result)
		})
	}
}
