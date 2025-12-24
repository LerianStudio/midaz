package command

import (
	"context"
	"errors"
	"testing"

	"github.com/LerianStudio/midaz/v3/components/onboarding/internal/adapters/postgres/organization"
	"github.com/LerianStudio/midaz/v3/components/onboarding/internal/services"
	"github.com/LerianStudio/midaz/v3/pkg"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
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
			expectedErr: errors.New("EntityNotFoundError"),
		},
		{
			name: "failure - repository error",
			setupMocks: func() {
				mockOrganizationRepo.EXPECT().
					Delete(gomock.Any(), organizationID).
					Return(errors.New("failed to delete organization")).
					Times(1)
			},
			expectedErr: errors.New("InternalServerError"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.setupMocks()

			err := uc.DeleteOrganizationByID(ctx, organizationID)

			if tt.expectedErr != nil {
				require.Error(t, err)
				switch tt.expectedErr.Error() {
				case "InternalServerError":
					var internalErr pkg.InternalServerError
					require.True(t, errors.As(err, &internalErr), "expected InternalServerError, got %T", err)
				case "EntityNotFoundError":
					var notFoundErr pkg.EntityNotFoundError
					require.True(t, errors.As(err, &notFoundErr), "expected EntityNotFoundError, got %T", err)
				default:
					assert.Contains(t, err.Error(), tt.expectedErr.Error())
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
