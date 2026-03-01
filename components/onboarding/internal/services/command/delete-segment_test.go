// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/LerianStudio/midaz/v3/components/onboarding/internal/adapters/postgres/segment"
	"github.com/LerianStudio/midaz/v3/components/onboarding/internal/services"
)

var (
	errDeleteSegment = errors.New("failed to delete segment")
	errSegNotFound   = errors.New("The provided segment ID does not exist in our records. Please verify the segment ID and try again.") //nolint:revive,staticcheck // business error message
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
			expectedErr: errSegNotFound,
		},
		{
			name: "failure - repository error",
			setupMocks: func() {
				mockSegmentRepo.EXPECT().
					Delete(gomock.Any(), organizationID, ledgerID, segmentID).
					Return(errDeleteSegment).
					Times(1)
			},
			expectedErr: errDeleteSegment,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.setupMocks()

			err := uc.DeleteSegmentByID(ctx, organizationID, ledgerID, segmentID)

			if tt.expectedErr != nil {
				require.Error(t, err)
				require.ErrorContains(t, err, tt.expectedErr.Error())
			} else {
				require.NoError(t, err)
			}
		})
	}
}
