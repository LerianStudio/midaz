package query

import (
	"context"
	"errors"
	"testing"

	"github.com/LerianStudio/midaz/v3/components/onboarding/internal/adapters/postgres/organization"
	"github.com/LerianStudio/midaz/v3/components/onboarding/internal/services"
	"github.com/LerianStudio/midaz/v3/pkg"
	"github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
)

func TestCountOrganizations(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockOrganizationRepo := organization.NewMockRepository(ctrl)

	uc := &UseCase{
		OrganizationRepo: mockOrganizationRepo,
	}

	tests := []struct {
		name           string
		mockSetup      func()
		expectErr      bool
		expectedError  error
		expectedResult int64
	}{
		{
			name: "Success - Count organizations",
			mockSetup: func() {
				mockOrganizationRepo.EXPECT().
					Count(gomock.Any()).
					Return(int64(42), nil)
			},
			expectErr:      false,
			expectedResult: 42,
		},
		{
			name: "Error - No organizations found",
			mockSetup: func() {
				mockOrganizationRepo.EXPECT().
					Count(gomock.Any()).
					Return(int64(0), services.ErrDatabaseItemNotFound)
			},
			expectErr:     true,
			expectedError: pkg.ValidateBusinessError(constant.ErrNoOrganizationsFound, "Organization"),
		},
		{
			name: "Error - Database error",
			mockSetup: func() {
				mockOrganizationRepo.EXPECT().
					Count(gomock.Any()).
					Return(int64(0), errors.New("database error"))
			},
			expectErr:      true,
			expectedResult: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.mockSetup()

			result, err := uc.CountOrganizations(context.Background())

			if tt.expectErr {
				assert.Error(t, err)
				if tt.expectedError != nil {
					assert.Equal(t, tt.expectedError.Error(), err.Error())
				}
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expectedResult, result)
			}
		})
	}
}
