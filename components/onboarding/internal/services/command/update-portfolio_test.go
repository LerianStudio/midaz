package command

import (
	"context"
	"errors"
	"github.com/LerianStudio/midaz/v3/components/onboarding/internal/adapters/mongodb"
	"github.com/LerianStudio/midaz/v3/components/onboarding/internal/adapters/postgres/portfolio"
	"github.com/LerianStudio/midaz/v3/components/onboarding/internal/services"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
	"testing"
)

func TestUpdatePortfolioByID(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockPortfolioRepo := portfolio.NewMockRepository(ctrl)
	mockMetadataRepo := mongodb.NewMockRepository(ctrl)

	uc := &UseCase{
		PortfolioRepo: mockPortfolioRepo,
		MetadataRepo:  mockMetadataRepo,
	}

	tests := []struct {
		name           string
		organizationID uuid.UUID
		ledgerID       uuid.UUID
		portfolioID    uuid.UUID
		input          *mmodel.UpdatePortfolioInput
		mockSetup      func()
		expectErr      bool
	}{
		{
			name:           "Success - Portfolio updated with metadata",
			organizationID: uuid.New(),
			ledgerID:       uuid.New(),
			portfolioID:    uuid.New(),
			input: &mmodel.UpdatePortfolioInput{
				Name:     "Updated Portfolio",
				Status:   mmodel.Status{Code: "active"},
				Metadata: map[string]any{"key": "value"},
			},
			mockSetup: func() {
				mockPortfolioRepo.EXPECT().
					Update(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
					Return(&mmodel.Portfolio{ID: "123", Name: "Updated Portfolio", Status: mmodel.Status{Code: "active"}, Metadata: nil}, nil)
				mockMetadataRepo.EXPECT().
					FindByEntity(gomock.Any(), gomock.Any(), gomock.Any()).
					Return(&mongodb.Metadata{Data: map[string]any{"existing_key": "existing_value"}}, nil)
				mockMetadataRepo.EXPECT().
					Update(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
					Return(nil)
			},
			expectErr: false,
		},
		{
			name:           "Error - Portfolio not found",
			organizationID: uuid.New(),
			ledgerID:       uuid.New(),
			portfolioID:    uuid.New(),
			input: &mmodel.UpdatePortfolioInput{
				Name:     "Nonexistent Portfolio",
				Status:   mmodel.Status{Code: "inactive"},
				Metadata: nil,
			},
			mockSetup: func() {
				mockPortfolioRepo.EXPECT().
					Update(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
					Return(nil, services.ErrDatabaseItemNotFound)
			},
			expectErr: true,
		},
		{
			name:           "Error - Failed to update metadata",
			organizationID: uuid.New(),
			ledgerID:       uuid.New(),
			portfolioID:    uuid.New(),
			input: &mmodel.UpdatePortfolioInput{
				Name:     "Portfolio with Metadata Error",
				Status:   mmodel.Status{Code: "active"},
				Metadata: map[string]any{"key": "value"},
			},
			mockSetup: func() {
				mockPortfolioRepo.EXPECT().
					Update(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
					Return(&mmodel.Portfolio{ID: "123", Name: "Portfolio with Metadata Error", Status: mmodel.Status{Code: "active"}, Metadata: nil}, nil)
				mockMetadataRepo.EXPECT().
					FindByEntity(gomock.Any(), gomock.Any(), gomock.Any()).
					Return(&mongodb.Metadata{Data: map[string]any{"existing_key": "existing_value"}}, nil)
				mockMetadataRepo.EXPECT().
					Update(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
					Return(errors.New("metadata update error"))
			},
			expectErr: true,
		},
		{
			name:           "Error - Failure to update portfolio",
			organizationID: uuid.New(),
			ledgerID:       uuid.New(),
			portfolioID:    uuid.New(),
			input: &mmodel.UpdatePortfolioInput{
				Name:     "Update Failure Portfolio",
				Status:   mmodel.Status{Code: "inactive"},
				Metadata: nil,
			},
			mockSetup: func() {
				mockPortfolioRepo.EXPECT().
					Update(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
					Return(nil, errors.New("update error"))
			},
			expectErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.mockSetup()

			ctx := context.Background()
			result, err := uc.UpdatePortfolioByID(ctx, tt.organizationID, tt.ledgerID, tt.portfolioID, tt.input)

			if tt.expectErr {
				assert.Error(t, err)
				assert.Nil(t, result)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, result)
				assert.Equal(t, tt.input.Name, result.Name)
				assert.Equal(t, tt.input.Status, result.Status)
			}
		})
	}
}
