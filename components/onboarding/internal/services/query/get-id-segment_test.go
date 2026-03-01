// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package query

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/LerianStudio/midaz/v3/components/onboarding/internal/adapters/mongodb"
	"github.com/LerianStudio/midaz/v3/components/onboarding/internal/adapters/postgres/segment"
	"github.com/LerianStudio/midaz/v3/components/onboarding/internal/services"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
)

func TestGetSegmentByID(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockSegmentRepo := segment.NewMockRepository(ctrl)
	mockMetadataRepo := mongodb.NewMockRepository(ctrl)

	uc := &UseCase{
		SegmentRepo:  mockSegmentRepo,
		MetadataRepo: mockMetadataRepo,
	}

	tests := []struct {
		name           string
		organizationID uuid.UUID
		ledgerID       uuid.UUID
		segmentID      uuid.UUID
		mockSetup      func()
		expectErr      bool
		expectedResult *mmodel.Segment
	}{
		{
			name:           "Success - Retrieve segment with metadata",
			organizationID: uuid.New(),
			ledgerID:       uuid.New(),
			segmentID:      uuid.New(),
			mockSetup: func() {
				segmentID := uuid.New()
				mockSegmentRepo.EXPECT().
					Find(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
					Return(&mmodel.Segment{ID: segmentID.String(), Name: "Test Segment", Status: mmodel.Status{Code: "active"}}, nil)
				mockMetadataRepo.EXPECT().
					FindByEntity(gomock.Any(), gomock.Any(), gomock.Any()).
					Return(&mongodb.Metadata{Data: map[string]any{"key": "value"}}, nil)
			},
			expectErr: false,
			expectedResult: &mmodel.Segment{
				ID:       "valid-uuid",
				Name:     "Test Segment",
				Status:   mmodel.Status{Code: "active"},
				Metadata: map[string]any{"key": "value"},
			},
		},
		{
			name:           "Error - Segment not found",
			organizationID: uuid.New(),
			ledgerID:       uuid.New(),
			segmentID:      uuid.New(),
			mockSetup: func() {
				mockSegmentRepo.EXPECT().
					Find(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
					Return(nil, services.ErrDatabaseItemNotFound)
			},
			expectErr:      true,
			expectedResult: nil,
		},
		{
			name:           "Error - Failed to retrieve metadata",
			organizationID: uuid.New(),
			ledgerID:       uuid.New(),
			segmentID:      uuid.New(),
			mockSetup: func() {
				segmentID := uuid.New()
				mockSegmentRepo.EXPECT().
					Find(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
					Return(&mmodel.Segment{ID: segmentID.String(), Name: "Test Segment", Status: mmodel.Status{Code: "active"}}, nil)
				mockMetadataRepo.EXPECT().
					FindByEntity(gomock.Any(), gomock.Any(), gomock.Any()).
					Return(nil, errMetadataRetrievalError)
			},
			expectErr:      true,
			expectedResult: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.mockSetup()

			ctx := context.Background()
			result, err := uc.GetSegmentByID(ctx, tt.organizationID, tt.ledgerID, tt.segmentID)

			if tt.expectErr {
				require.Error(t, err)
				assert.Nil(t, result)
			} else {
				require.NoError(t, err)
				assert.NotNil(t, result)
			}
		})
	}
}
