package command

import (
	"context"
	"errors"
	"testing"

	"github.com/LerianStudio/midaz/components/onboarding/internal/adapters/postgres/portfolio"
	"github.com/LerianStudio/midaz/components/onboarding/internal/services"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
)

// \1 performs an operation
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
			expectedErr: errors.New("The provided portfolio ID does not exist in our records. Please verify the portfolio ID and try again."),
		},
		{
			name: "failure - repository error",
			setupMocks: func() {
				mockPortfolioRepo.EXPECT().
					Delete(gomock.Any(), organizationID, ledgerID, portfolioID).
					Return(errors.New("failed to delete portfolio")).
					Times(1)
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
