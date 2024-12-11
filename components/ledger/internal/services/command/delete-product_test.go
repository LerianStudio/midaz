package command

import (
	"context"
	"errors"
	"testing"

	"go.uber.org/mock/gomock"

	"github.com/LerianStudio/midaz/components/ledger/internal/adapters/postgres/product"
	"github.com/LerianStudio/midaz/components/ledger/internal/services"
	"github.com/google/uuid"

	"github.com/stretchr/testify/assert"
)

func TestDeleteProductByID(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockProductRepo := product.NewMockRepository(ctrl)

	uc := &UseCase{
		ProductRepo: mockProductRepo,
	}

	ctx := context.Background()
	organizationID := uuid.New()
	ledgerID := uuid.New()
	productID := uuid.New()

	tests := []struct {
		name        string
		setupMocks  func()
		expectedErr error
	}{
		{
			name: "success - product deleted",
			setupMocks: func() {
				mockProductRepo.EXPECT().
					Delete(gomock.Any(), organizationID, ledgerID, productID).
					Return(nil).
					Times(1)
			},
			expectedErr: nil,
		},
		{
			name: "failure - product not found",
			setupMocks: func() {
				mockProductRepo.EXPECT().
					Delete(gomock.Any(), organizationID, ledgerID, productID).
					Return(services.ErrDatabaseItemNotFound).
					Times(1)
			},
			expectedErr: errors.New("The provided product ID does not exist in our records. Please verify the product ID and try again."),
		},
		{
			name: "failure - repository error",
			setupMocks: func() {
				mockProductRepo.EXPECT().
					Delete(gomock.Any(), organizationID, ledgerID, productID).
					Return(errors.New("failed to delete product")).
					Times(1)
			},
			expectedErr: errors.New("failed to delete product"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.setupMocks()

			err := uc.DeleteProductByID(ctx, organizationID, ledgerID, productID)

			if tt.expectedErr != nil {
				assert.Error(t, err)
				assert.Equal(t, tt.expectedErr.Error(), err.Error())
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
