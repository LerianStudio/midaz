package command

import (
	"context"
	"errors"
	"github.com/LerianStudio/midaz/components/onboarding/internal/adapters/postgres/segment"
	"testing"

	"go.uber.org/mock/gomock"

	"github.com/LerianStudio/midaz/components/onboarding/internal/adapters/mongodb"
	"github.com/LerianStudio/midaz/components/onboarding/internal/services"
	"github.com/LerianStudio/midaz/pkg/mmodel"
	"github.com/google/uuid"

	"github.com/stretchr/testify/assert"
)

func TestUpdateSegmentByID(t *testing.T) {
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
		input          *mmodel.UpdateSegmentInput
		mockSetup      func()
		expectErr      bool
	}{
		{
			name:           "Success - Segment updated with metadata",
			organizationID: uuid.New(),
			ledgerID:       uuid.New(),
			segmentID:      uuid.New(),
			input: &mmodel.UpdateSegmentInput{
				Name:     "Updated Segment",
				Status:   mmodel.Status{Code: "active"},
				Metadata: map[string]any{"key": "value"},
			},
			mockSetup: func() {
				mockSegmentRepo.EXPECT().
					Update(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
					Return(&mmodel.Segment{ID: "123", Name: "Updated Segment", Status: mmodel.Status{Code: "active"}, Metadata: nil}, nil)
				mockMetadataRepo.EXPECT().
					FindByEntity(gomock.Any(), gomock.Any(), gomock.Any()).
					Return(&mongodb.Metadata{Data: map[string]any{"existing_key": "existing_value"}}, nil)
				mockMetadataRepo.EXPECT().
					Update(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
					Return(nil)
			},
			expectErr: false,
		},
		{
			name:           "Error - Segment not found",
			organizationID: uuid.New(),
			ledgerID:       uuid.New(),
			segmentID:      uuid.New(),
			input: &mmodel.UpdateSegmentInput{
				Name:     "Nonexistent Segment",
				Status:   mmodel.Status{Code: "inactive"},
				Metadata: nil,
			},
			mockSetup: func() {
				mockSegmentRepo.EXPECT().
					Update(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
					Return(nil, services.ErrDatabaseItemNotFound)
			},
			expectErr: true,
		},
		{
			name:           "Error - Failed to update metadata",
			organizationID: uuid.New(),
			ledgerID:       uuid.New(),
			segmentID:      uuid.New(),
			input: &mmodel.UpdateSegmentInput{
				Name:     "Segment with Metadata Error",
				Status:   mmodel.Status{Code: "active"},
				Metadata: map[string]any{"key": "value"},
			},
			mockSetup: func() {
				mockSegmentRepo.EXPECT().
					Update(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
					Return(&mmodel.Segment{ID: "123", Name: "Segment with Metadata Error", Status: mmodel.Status{Code: "active"}, Metadata: nil}, nil)
				mockMetadataRepo.EXPECT().
					FindByEntity(gomock.Any(), gomock.Any(), gomock.Any()).
					Return(&mongodb.Metadata{Data: map[string]any{"existing_key": "existing_value"}}, nil)
				mockMetadataRepo.EXPECT().
					Update(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
					Return(errors.New("metadata update error"))
			},
			expectErr: true,
		},
		{
			name:           "Error - Failure to update segment",
			organizationID: uuid.New(),
			ledgerID:       uuid.New(),
			segmentID:      uuid.New(),
			input: &mmodel.UpdateSegmentInput{
				Name:     "Update Failure Segment",
				Status:   mmodel.Status{Code: "inactive"},
				Metadata: nil,
			},
			mockSetup: func() {
				mockSegmentRepo.EXPECT().
					Update(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
					Return(nil, errors.New("update error"))
			},
			expectErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.mockSetup()

			ctx := context.Background()
			result, err := uc.UpdateSegmentByID(ctx, tt.organizationID, tt.ledgerID, tt.segmentID, tt.input)

			if tt.expectErr {
				assert.Error(t, err)
				assert.Nil(t, result)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, result)
				assert.Equal(t, tt.input.Name, result.Name)
				assert.Equal(t, tt.input.Status, result.Status)
			}
		})
	}
}
