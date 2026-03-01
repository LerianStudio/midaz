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

	"github.com/LerianStudio/midaz/v3/components/onboarding/internal/adapters/postgres/portfolio"
	"github.com/LerianStudio/midaz/v3/components/onboarding/internal/services"
)

var (
	errDeletePortfolio = errors.New("failed to delete portfolio")
	errPortNotFound    = errors.New("The provided portfolio ID does not exist in our records. Please verify the portfolio ID and try again.") //nolint:revive,staticcheck // business error message
)

func TestDeletePortfolioByID(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockPortfolioRepo := portfolio.NewMockRepository(ctrl)

	uc := &UseCase{
		PortfolioRepo: mockPortfolioRepo,
	}

	ctx := context.Background()
	organizationID := uuid.New()
	ledgerID := uuid.New()
	portfolioID := uuid.New()

	tests := []struct {
		name        string
		setupMocks  func()
		expectedErr error
	}{
		{
			name: "success - portfolio deleted",
			setupMocks: func() {
				mockPortfolioRepo.EXPECT().
					Delete(gomock.Any(), organizationID, ledgerID, portfolioID).
					Return(nil).
					Times(1)
			},
			expectedErr: nil,
		},
		{
			name: "failure - portfolio not found",
			setupMocks: func() {
				mockPortfolioRepo.EXPECT().
					Delete(gomock.Any(), organizationID, ledgerID, portfolioID).
					Return(services.ErrDatabaseItemNotFound).
					Times(1)
			},
			expectedErr: errPortNotFound,
		},
		{
			name: "failure - repository error",
			setupMocks: func() {
				mockPortfolioRepo.EXPECT().
					Delete(gomock.Any(), organizationID, ledgerID, portfolioID).
					Return(errDeletePortfolio).
					Times(1)
			},
			expectedErr: errDeletePortfolio,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.setupMocks()

			err := uc.DeletePortfolioByID(ctx, organizationID, ledgerID, portfolioID)

			if tt.expectedErr != nil {
				require.Error(t, err)
				require.ErrorContains(t, err, tt.expectedErr.Error())
			} else {
				require.NoError(t, err)
			}
		})
	}
}
