package query

import (
	"context"
	"errors"
	"testing"

	"go.uber.org/mock/gomock"

	"github.com/LerianStudio/midaz/components/ledger/internal/adapters/mongodb"
	"github.com/LerianStudio/midaz/components/ledger/internal/adapters/postgres/portfolio"
	"github.com/LerianStudio/midaz/pkg/mmodel"
	"github.com/LerianStudio/midaz/pkg/net/http"
	"github.com/google/uuid"

	"github.com/stretchr/testify/assert"
)

func TestGetAllMetadataPortfolios(t *testing.T) {
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
		filter         http.QueryHeader
		mockSetup      func()
		expectErr      bool
		expectedResult []*mmodel.Portfolio
	}{
		{
			name:           "Success - Retrieve portfolios with metadata",
			organizationID: uuid.New(),
			ledgerID:       uuid.New(),
			mockSetup: func() {
				validUUID := uuid.New()
				mockMetadataRepo.EXPECT().
					FindList(gomock.Any(), gomock.Any(), gomock.Any()).
					Return([]*mongodb.Metadata{
						{EntityID: validUUID.String(), Data: map[string]any{"key": "value"}},
					}, nil)
				mockPortfolioRepo.EXPECT().
					ListByIDs(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Eq([]uuid.UUID{validUUID})).
					Return([]*mmodel.Portfolio{
						{ID: validUUID.String(), Name: "Test Portfolio", Status: mmodel.Status{Code: "active"}},
					}, nil)
			},
			expectErr: false,
			expectedResult: []*mmodel.Portfolio{
				{ID: "valid-uuid", Name: "Test Portfolio", Status: mmodel.Status{Code: "active"}, Metadata: map[string]any{"key": "value"}},
			},
		},
		{
			name:           "Error - No metadata found",
			organizationID: uuid.New(),
			ledgerID:       uuid.New(),
			mockSetup: func() {
				mockMetadataRepo.EXPECT().
					FindList(gomock.Any(), gomock.Any(), gomock.Any()).
					Return(nil, errors.New("error no metadata found"))
			},
			expectErr:      true,
			expectedResult: nil,
		},
		{
			name:           "Error - Failed to retrieve portfolios",
			organizationID: uuid.New(),
			ledgerID:       uuid.New(),
			mockSetup: func() {
				validUUID := uuid.New()
				mockMetadataRepo.EXPECT().
					FindList(gomock.Any(), gomock.Any(), gomock.Any()).
					Return([]*mongodb.Metadata{
						{EntityID: validUUID.String(), Data: map[string]any{"key": "value"}},
					}, nil)
				mockPortfolioRepo.EXPECT().
					ListByIDs(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Eq([]uuid.UUID{validUUID})).
					Return(nil, errors.New("database error"))
			},
			expectErr:      true,
			expectedResult: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.mockSetup()

			ctx := context.Background()
			result, err := uc.GetAllMetadataPortfolios(ctx, tt.organizationID, tt.ledgerID, tt.filter)

			if tt.expectErr {
				assert.Error(t, err)
				assert.Nil(t, result)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, result)
			}
		})
	}
}
