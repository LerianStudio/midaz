// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"errors"
	"testing"

	onbMongo "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/mongodb/onboarding"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/portfolio"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/services"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
)

func TestDeletePortfolioByID(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockPortfolioRepo := portfolio.NewMockRepository(ctrl)

	mockMetadataRepo := onbMongo.NewMockRepository(ctrl)

	uc := &UseCase{
		PortfolioRepo:          mockPortfolioRepo,
		OnboardingMetadataRepo: mockMetadataRepo,
		metadataDeleteRetry:    fastMetadataDeleteRetryPolicy(),
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
				mockMetadataRepo.EXPECT().
					Delete(gomock.Any(), constant.EntityPortfolio, portfolioID.String()).
					Return(nil).
					Times(1)
			},
			expectedErr: nil,
		},
		{
			name: "success - metadata soft delete failure does not fail the delete",
			setupMocks: func() {
				mockPortfolioRepo.EXPECT().
					Delete(gomock.Any(), organizationID, ledgerID, portfolioID).
					Return(nil).
					Times(1)
				mockMetadataRepo.EXPECT().
					Delete(gomock.Any(), constant.EntityPortfolio, portfolioID.String()).
					Return(errors.New("mongo unavailable")).
					Times(3)
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
				mockMetadataRepo.EXPECT().
					Delete(gomock.Any(), constant.EntityPortfolio, portfolioID.String()).
					Times(0)
			},
			expectedErr: errors.New("The provided portfolio ID does not exist in our records. Please verify the portfolio ID and try again."),
		},
		{
			name: "failure - repository error",
			setupMocks: func() {
				mockPortfolioRepo.EXPECT().
					Delete(gomock.Any(), organizationID, ledgerID, portfolioID).
					Return(errors.New("failed to delete portfolio")).
					Times(1)
				mockMetadataRepo.EXPECT().
					Delete(gomock.Any(), constant.EntityPortfolio, portfolioID.String()).
					Times(0)
			},
			expectedErr: errors.New("failed to delete portfolio"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.setupMocks()

			err := uc.DeletePortfolioByID(ctx, organizationID, ledgerID, portfolioID)

			if tt.expectedErr != nil {
				assert.Error(t, err)
				assert.Equal(t, tt.expectedErr.Error(), err.Error())
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
