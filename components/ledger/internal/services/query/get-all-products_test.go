package query

import (
	"context"
	"errors"
	"testing"

	"go.uber.org/mock/gomock"

	"github.com/LerianStudio/midaz/components/ledger/internal/adapters/mongodb"
	"github.com/LerianStudio/midaz/components/ledger/internal/adapters/postgres/product"
	"github.com/LerianStudio/midaz/components/ledger/internal/services"
	"github.com/LerianStudio/midaz/pkg/mmodel"
	"github.com/LerianStudio/midaz/pkg/net/http"
	"github.com/google/uuid"

	"github.com/stretchr/testify/assert"
)

func TestGetAllProducts(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockProductRepo := product.NewMockRepository(ctrl)
	mockMetadataRepo := mongodb.NewMockRepository(ctrl)

	uc := &UseCase{
		ProductRepo:  mockProductRepo,
		MetadataRepo: mockMetadataRepo,
	}

	tests := []struct {
		name           string
		organizationID uuid.UUID
		ledgerID       uuid.UUID
		filter         http.QueryHeader
		mockSetup      func()
		expectErr      bool
		expectedResult []*mmodel.Product
	}{
		{
			name:           "Success - Retrieve products with metadata",
			organizationID: uuid.New(),
			ledgerID:       uuid.New(),
			filter:         http.QueryHeader{Limit: 10, Page: 1},
			mockSetup: func() {
				validUUID := uuid.New()
				mockProductRepo.EXPECT().
					FindAll(gomock.Any(), gomock.Any(), gomock.Any(), 10, 1).
					Return([]*mmodel.Product{
						{ID: validUUID.String(), Name: "Test Product", Status: mmodel.Status{Code: "active"}},
					}, nil)
				mockMetadataRepo.EXPECT().
					FindList(gomock.Any(), gomock.Any(), gomock.Any()).
					Return([]*mongodb.Metadata{
						{EntityID: validUUID.String(), Data: map[string]any{"key": "value"}},
					}, nil)
			},
			expectErr: false,
			expectedResult: []*mmodel.Product{
				{ID: "valid-uuid", Name: "Test Product", Status: mmodel.Status{Code: "active"}, Metadata: map[string]any{"key": "value"}},
			},
		},
		{
			name:           "Error - No products found",
			organizationID: uuid.New(),
			ledgerID:       uuid.New(),
			filter:         http.QueryHeader{Limit: 10, Page: 1},
			mockSetup: func() {
				mockProductRepo.EXPECT().
					FindAll(gomock.Any(), gomock.Any(), gomock.Any(), 10, 1).
					Return(nil, services.ErrDatabaseItemNotFound)
			},
			expectErr:      true,
			expectedResult: nil,
		},
		{
			name:           "Error - Failed to retrieve metadata",
			organizationID: uuid.New(),
			ledgerID:       uuid.New(),
			filter:         http.QueryHeader{Limit: 10, Page: 1},
			mockSetup: func() {
				validUUID := uuid.New()
				mockProductRepo.EXPECT().
					FindAll(gomock.Any(), gomock.Any(), gomock.Any(), 10, 1).
					Return([]*mmodel.Product{
						{ID: validUUID.String(), Name: "Test Product", Status: mmodel.Status{Code: "active"}},
					}, nil)
				mockMetadataRepo.EXPECT().
					FindList(gomock.Any(), gomock.Any(), gomock.Any()).
					Return(nil, errors.New("metadata retrieval error"))
			},
			expectErr:      true,
			expectedResult: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.mockSetup()

			ctx := context.Background()
			result, err := uc.GetAllProducts(ctx, tt.organizationID, tt.ledgerID, tt.filter)

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
