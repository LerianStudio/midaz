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

	"github.com/LerianStudio/midaz/v3/components/onboarding/internal/adapters/postgres/organization"
	"github.com/LerianStudio/midaz/v3/components/onboarding/internal/services"
)

var (
	errDeleteOrganization = errors.New("failed to delete organization")
	errOrgNotFound        = errors.New("The provided organization ID does not exist in our records. Please verify the organization ID and try again.") //nolint:revive,staticcheck // business error message
)

func TestDeleteOrganizationByID(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockOrganizationRepo := organization.NewMockRepository(ctrl)

	uc := &UseCase{
		OrganizationRepo: mockOrganizationRepo,
	}

	ctx := context.Background()
	organizationID := uuid.New()

	tests := []struct {
		name        string
		setupMocks  func()
		expectedErr error
	}{
		{
			name: "success - organization deleted",
			setupMocks: func() {
				mockOrganizationRepo.EXPECT().
					Delete(gomock.Any(), organizationID).
					Return(nil).
					Times(1)
			},
			expectedErr: nil,
		},
		{
			name: "failure - organization not found",
			setupMocks: func() {
				mockOrganizationRepo.EXPECT().
					Delete(gomock.Any(), organizationID).
					Return(services.ErrDatabaseItemNotFound).
					Times(1)
			},
			expectedErr: errOrgNotFound,
		},
		{
			name: "failure - repository error",
			setupMocks: func() {
				mockOrganizationRepo.EXPECT().
					Delete(gomock.Any(), organizationID).
					Return(errDeleteOrganization).
					Times(1)
			},
			expectedErr: errDeleteOrganization,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.setupMocks()

			err := uc.DeleteOrganizationByID(ctx, organizationID)

			if tt.expectedErr != nil {
				require.Error(t, err)
				require.ErrorContains(t, err, tt.expectedErr.Error())
			} else {
				require.NoError(t, err)
			}
		})
	}
}
