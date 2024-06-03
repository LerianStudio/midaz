package query

import (
	"context"
	"errors"
	"testing"

	p "github.com/LerianStudio/midaz/components/ledger/internal/domain/portfolio/product"
	mock "github.com/LerianStudio/midaz/components/ledger/internal/gen/mock/product"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
)

// TestGetAllProductsError is responsible to test GetAllProducts with success and error
func TestGetAllProducts(t *testing.T) {
	organizationID := uuid.New()
	ledgerID := uuid.New()
	limit := 10
	page := 1

	t.Parallel()
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockProductRepo := mock.NewMockRepository(ctrl)

	uc := UseCase{
		ProductRepo: mockProductRepo,
	}

	t.Run("Success", func(t *testing.T) {
		products := []*p.Product{{}}
		mockProductRepo.
			EXPECT().
			FindAll(gomock.Any(), organizationID, ledgerID, limit, page).
			Return(products, nil).
			Times(1)
		res, err := uc.ProductRepo.FindAll(context.TODO(), organizationID, ledgerID, limit, page)

		assert.NoError(t, err)
		assert.Len(t, res, 1)
	})

	t.Run("Error", func(t *testing.T) {
		errMsg := "errDatabaseItemNotFound"
		mockProductRepo.
			EXPECT().
			FindAll(gomock.Any(), organizationID, ledgerID, limit, page).
			Return(nil, errors.New(errMsg)).
			Times(1)
		res, err := uc.ProductRepo.FindAll(context.TODO(), organizationID, ledgerID, limit, page)

		assert.EqualError(t, err, errMsg)
		assert.Nil(t, res)
	})
}
