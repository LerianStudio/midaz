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

	"github.com/LerianStudio/midaz/v3/components/onboarding/internal/adapters/postgres/portfolio"
	"github.com/LerianStudio/midaz/v3/components/onboarding/internal/services"
	"github.com/LerianStudio/midaz/v3/pkg"
	"github.com/LerianStudio/midaz/v3/pkg/constant"
)

func TestCountPortfolios(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockPortfolioRepo := portfolio.NewMockRepository(ctrl)
	organizationID := uuid.New()
	ledgerID := uuid.New()

	uc := &UseCase{
		PortfolioRepo: mockPortfolioRepo,
	}

	tests := []struct {
		name           string
		mockSetup      func()
		expectErr      bool
		expectedError  error
		expectedResult int64
	}{
		{
			name: "Success - Count portfolios",
			mockSetup: func() {
				mockPortfolioRepo.EXPECT().
					Count(gomock.Any(), organizationID, ledgerID).
					Return(int64(15), nil)
			},
			expectErr:      false,
			expectedResult: 15,
		},
		{
			name: "Error - No portfolios found",
			mockSetup: func() {
				mockPortfolioRepo.EXPECT().
					Count(gomock.Any(), organizationID, ledgerID).
					Return(int64(0), services.ErrDatabaseItemNotFound)
			},
			expectErr:     true,
			expectedError: pkg.ValidateBusinessError(constant.ErrNoPortfoliosFound, "Portfolio"),
		},
		{
			name: "Error - Database error",
			mockSetup: func() {
				mockPortfolioRepo.EXPECT().
					Count(gomock.Any(), organizationID, ledgerID).
					Return(int64(0), errDatabaseError)
			},
			expectErr:      true,
			expectedResult: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.mockSetup()

			result, err := uc.CountPortfolios(context.Background(), organizationID, ledgerID)

			if tt.expectErr {
				require.Error(t, err)

				if tt.expectedError != nil {
					require.ErrorContains(t, err, tt.expectedError.Error())
				}
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.expectedResult, result)
			}
		})
	}
}
