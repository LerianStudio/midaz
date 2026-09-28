// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"errors"
	"testing"
	"time"

	mongodb "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/mongodb/onboarding"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	pkgHTTP "github.com/LerianStudio/midaz/v4/pkg/net/http"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

func TestCreateMetadata(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockMetadataRepo := mongodb.NewMockRepository(ctrl)

	uc := &UseCase{
		OnboardingMetadataRepo: mockMetadataRepo,
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

			result, err := uc.CreateOnboardingMetadata(ctx, tt.entityName, tt.entityID, tt.metadata)

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

// TestCreateOnboardingMetadata_DecodedBody builds the metadata through the real
// HTTP decode path, so the input reaching the use case is exactly what a create
// handler passes: an absent key, an explicit null and an empty object all persist
// no document, and only a non-empty object is written.
func TestCreateOnboardingMetadata_DecodedBody(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		body         string
		wantCreate   bool
		expectedMeta map[string]any
	}{
		{
			name:         "absent metadata key writes no document",
			body:         `{"name":"Ledger"}`,
			wantCreate:   false,
			expectedMeta: nil,
		},
		{
			name:         "null metadata writes no document",
			body:         `{"name":"Ledger","metadata":null}`,
			wantCreate:   false,
			expectedMeta: nil,
		},
		{
			name:         "empty metadata object writes no document",
			body:         `{"name":"Ledger","metadata":{}}`,
			wantCreate:   false,
			expectedMeta: nil,
		},
		{
			name:         "non-empty metadata writes one document",
			body:         `{"name":"Ledger","metadata":{"k":"v"}}`,
			wantCreate:   true,
			expectedMeta: map[string]any{"k": "v"},
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

			payload := new(mmodel.CreateLedgerInput)

			_, err := pkgHTTP.DecodeAndValidate([]byte(tt.body), payload)
			require.NoError(t, err)

			if tt.wantCreate {
				mockMetadataRepo.EXPECT().
					Create(gomock.Any(), "Ledger", gomock.Any()).
					DoAndReturn(func(_ context.Context, _ string, meta *mongodb.Metadata) error {
						assert.Equal(t, "entity-id", meta.EntityID)
						assert.Equal(t, mongodb.JSON(tt.expectedMeta), meta.Data)

						return nil
					}).
					Times(1)
			} else {
				mockMetadataRepo.EXPECT().
					Create(gomock.Any(), gomock.Any(), gomock.Any()).
					Times(0)
			}

			result, err := uc.CreateOnboardingMetadata(context.Background(), "Ledger", "entity-id", payload.Metadata)

			require.NoError(t, err)
			assert.Equal(t, tt.expectedMeta, result)
		})
	}
}

// TestCreateOnboardingMetadata_WriteFailure pins how a failing metadata write
// interacts with the create: filled metadata propagates the repository error,
// while nil or empty metadata never reaches the repository and succeeds.
func TestCreateOnboardingMetadata_WriteFailure(t *testing.T) {
	t.Parallel()

	writeErr := errors.New("failed to create metadata")

	tests := []struct {
		name        string
		metadata    map[string]any
		wantCreate  bool
		expectedErr error
	}{
		{
			name:        "filled metadata propagates the write error",
			metadata:    map[string]any{"k": "v"},
			wantCreate:  true,
			expectedErr: writeErr,
		},
		{
			name:        "empty metadata does not attempt a write",
			metadata:    map[string]any{},
			wantCreate:  false,
			expectedErr: nil,
		},
		{
			name:        "nil metadata does not attempt a write",
			metadata:    nil,
			wantCreate:  false,
			expectedErr: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			t.Cleanup(ctrl.Finish)

			// Without a registered Create expectation, any write attempt fails the test.
			mockMetadataRepo := mongodb.NewMockRepository(ctrl)

			if tt.wantCreate {
				mockMetadataRepo.EXPECT().
					Create(gomock.Any(), "Ledger", gomock.Any()).
					Return(writeErr).
					Times(1)
			}

			uc := &UseCase{
				OnboardingMetadataRepo: mockMetadataRepo,
			}

			result, err := uc.CreateOnboardingMetadata(context.Background(), "Ledger", "entity-id", tt.metadata)

			if tt.expectedErr != nil {
				assert.Error(t, err)
				assert.Equal(t, tt.expectedErr.Error(), err.Error())
				assert.Nil(t, result)
			} else {
				assert.NoError(t, err)
				assert.Nil(t, result)
			}
		})
	}
}
