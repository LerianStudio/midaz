package query

import (
	"context"
	"errors"
	"testing"

	"go.uber.org/mock/gomock"

	"github.com/LerianStudio/midaz/components/onboarding/internal/adapters/mongodb"
	"github.com/LerianStudio/midaz/components/onboarding/internal/adapters/postgres/segment"
	"github.com/LerianStudio/midaz/components/onboarding/internal/services"
	"github.com/LerianStudio/midaz/pkg/mmodel"
	"github.com/LerianStudio/midaz/pkg/net/http"
	"github.com/google/uuid"

	"github.com/stretchr/testify/assert"
)

func TestGetAllSegments(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockSegmentRepo := segment.NewMockRepository(ctrl)
	mockMetadataRepo := mongodb.NewMockRepository(ctrl)

	uc := &UseCase{
		SegmentRepo:  mockSegmentRepo,
		MetadataRepo: mockMetadataRepo,
	}

	filter := http.QueryHeader{
		Limit: 10,
		Page:  1,
	}

	tests := []struct {
		name           string
		organizationID uuid.UUID
		ledgerID       uuid.UUID
		filter         http.QueryHeader
		mockSetup      func()
		expectErr      bool
		expectedResult []*mmodel.Segment
	}{
		{
			name:           "Success - Retrieve segments with metadata",
			organizationID: uuid.New(),
			ledgerID:       uuid.New(),
			filter:         filter,
			mockSetup: func() {
				validUUID := uuid.New()
				mockSegmentRepo.EXPECT().
					FindAll(gomock.Any(), gomock.Any(), gomock.Any(), filter.ToOffsetPagination()).
					Return([]*mmodel.Segment{
						{ID: validUUID.String(), Name: "Test Segment", Status: mmodel.Status{Code: "active"}},
					}, nil)
				mockMetadataRepo.EXPECT().
					FindList(gomock.Any(), gomock.Any(), gomock.Any()).
					Return([]*mongodb.Metadata{
						{EntityID: validUUID.String(), Data: map[string]any{"key": "value"}},
					}, nil)
			},
			expectErr: false,
			expectedResult: []*mmodel.Segment{
				{ID: "valid-uuid", Name: "Test Segment", Status: mmodel.Status{Code: "active"}, Metadata: map[string]any{"key": "value"}},
			},
		},
		{
			name:           "Error - No segments found",
			organizationID: uuid.New(),
			ledgerID:       uuid.New(),
			filter:         http.QueryHeader{Limit: 10, Page: 1},
			mockSetup: func() {
				mockSegmentRepo.EXPECT().
					FindAll(gomock.Any(), gomock.Any(), gomock.Any(), filter.ToOffsetPagination()).
					Return(nil, services.ErrDatabaseItemNotFound)
			},
			expectErr:      true,
			expectedResult: nil,
		},
		{
			name:           "Error - Failed to retrieve metadata",
			organizationID: uuid.New(),
			ledgerID:       uuid.New(),
			filter:         http.QueryHeader{Limit: 10, Page: 1},
			mockSetup: func() {
				validUUID := uuid.New()
				mockSegmentRepo.EXPECT().
					FindAll(gomock.Any(), gomock.Any(), gomock.Any(), filter.ToOffsetPagination()).
					Return([]*mmodel.Segment{
						{ID: validUUID.String(), Name: "Test Segment", Status: mmodel.Status{Code: "active"}},
					}, nil)
				mockMetadataRepo.EXPECT().
					FindList(gomock.Any(), gomock.Any(), gomock.Any()).
					Return(nil, errors.New("metadata retrieval error"))
			},
			expectErr:      true,
			expectedResult: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.mockSetup()

			ctx := context.Background()
			result, err := uc.GetAllSegments(ctx, tt.organizationID, tt.ledgerID, tt.filter)

			if tt.expectErr {
				assert.Error(t, err)
				assert.Nil(t, result)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, result)
			}
		})
	}
}
