package command

import (
	"context"
	"errors"
	"testing"

	"github.com/LerianStudio/midaz/components/onboarding/internal/adapters/postgres/organization"
	"github.com/LerianStudio/midaz/components/onboarding/internal/services"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
)

// \1 performs an operation
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
			expectedErr: errors.New("The provided organization ID does not exist in our records. Please verify the organization ID and try again."),
		},
		{
			name: "failure - repository error",
			setupMocks: func() {
				mockOrganizationRepo.EXPECT().
					Delete(gomock.Any(), organizationID).
					Return(errors.New("failed to delete organization")).
					Times(1)
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
