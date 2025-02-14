package command

import (
	"context"
	"errors"
	"testing"

	"github.com/LerianStudio/midaz/components/onboarding/internal/adapters/postgres/segment"
	"github.com/LerianStudio/midaz/components/onboarding/internal/services"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
)

func TestDeleteSegmentByID(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockSegmentRepo := segment.NewMockRepository(ctrl)

	uc := &UseCase{
		SegmentRepo: mockSegmentRepo,
	}

	ctx := context.Background()
	organizationID := uuid.New()
	ledgerID := uuid.New()
	segmentID := uuid.New()

	tests := []struct {
		name        string
		setupMocks  func()
		expectedErr error
	}{
		{
			name: "success - segment deleted",
			setupMocks: func() {
				mockSegmentRepo.EXPECT().
					Delete(gomock.Any(), organizationID, ledgerID, segmentID).
					Return(nil).
					Times(1)
			},
			expectedErr: nil,
		},
		{
			name: "failure - segment not found",
			setupMocks: func() {
				mockSegmentRepo.EXPECT().
					Delete(gomock.Any(), organizationID, ledgerID, segmentID).
					Return(services.ErrDatabaseItemNotFound).
					Times(1)
			},
			expectedErr: errors.New("The provided segment ID does not exist in our records. Please verify the segment ID and try again."),
		},
		{
			name: "failure - repository error",
			setupMocks: func() {
				mockSegmentRepo.EXPECT().
					Delete(gomock.Any(), organizationID, ledgerID, segmentID).
					Return(errors.New("failed to delete segment")).
					Times(1)
			},
			expectedErr: errors.New("failed to delete segment"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.setupMocks()

			err := uc.DeleteSegmentByID(ctx, organizationID, ledgerID, segmentID)

			if tt.expectedErr != nil {
				assert.Error(t, err)
				assert.Equal(t, tt.expectedErr.Error(), err.Error())
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
