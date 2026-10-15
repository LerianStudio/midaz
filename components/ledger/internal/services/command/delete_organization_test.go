// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"errors"
	"testing"

	onbMongo "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/mongodb/onboarding"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/organization"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/services"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
)

func TestDeleteOrganizationByID(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockOrganizationRepo := organization.NewMockRepository(ctrl)

	mockMetadataRepo := onbMongo.NewMockRepository(ctrl)

	uc := &UseCase{
		OrganizationRepo:       mockOrganizationRepo,
		OnboardingMetadataRepo: mockMetadataRepo,
		metadataDeleteRetry:    fastMetadataDeleteRetryPolicy(),
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
				mockMetadataRepo.EXPECT().
					Delete(gomock.Any(), constant.EntityOrganization, organizationID.String()).
					Return(nil).
					Times(1)
			},
			expectedErr: nil,
		},
		{
			name: "success - metadata soft delete failure does not fail the delete",
			setupMocks: func() {
				mockOrganizationRepo.EXPECT().
					Delete(gomock.Any(), organizationID).
					Return(nil).
					Times(1)
				mockMetadataRepo.EXPECT().
					Delete(gomock.Any(), constant.EntityOrganization, organizationID.String()).
					Return(errors.New("mongo unavailable")).
					Times(3)
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
				mockMetadataRepo.EXPECT().
					Delete(gomock.Any(), constant.EntityOrganization, organizationID.String()).
					Times(0)
			},
			expectedErr: errors.New("The provided organization ID does not exist in our records. Please verify the organization ID and try again."),
		},
		{
			name: "failure - repository error",
			setupMocks: func() {
				mockOrganizationRepo.EXPECT().
					Delete(gomock.Any(), organizationID).
					Return(errors.New("failed to delete organization")).
					Times(1)
				mockMetadataRepo.EXPECT().
					Delete(gomock.Any(), constant.EntityOrganization, organizationID.String()).
					Times(0)
			},
			expectedErr: errors.New("failed to delete organization"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.setupMocks()

			err := uc.DeleteOrganizationByID(ctx, organizationID)

			if tt.expectedErr != nil {
				assert.Error(t, err)
				assert.Equal(t, tt.expectedErr.Error(), err.Error())
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
