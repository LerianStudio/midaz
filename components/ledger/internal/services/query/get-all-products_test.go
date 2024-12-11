package query

import (
	"context"
	"errors"
	"github.com/LerianStudio/midaz/pkg/net/http"
	"go.uber.org/mock/gomock"
	"testing"
	"time"

	"github.com/LerianStudio/midaz/components/ledger/internal/adapters/postgres/product"
	"github.com/LerianStudio/midaz/pkg"
	"github.com/LerianStudio/midaz/pkg/mmodel"

	"github.com/stretchr/testify/assert"
)

// TestGetAllProductsError is responsible to test GetAllProducts with success and error
func TestGetAllProducts(t *testing.T) {
	organizationID := pkg.GenerateUUIDv7()
	ledgerID := pkg.GenerateUUIDv7()
	filter := http.QueryHeader{
		Limit:        10,
		Page:         1,
		SortOrder:    "asc",
		StartDate:    time.Now().AddDate(0, -1, 0),
		EndDate:      time.Now(),
		ToAssetCodes: []string{"BRL"},
	}

	t.Parallel()
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockProductRepo := product.NewMockRepository(ctrl)

	uc := UseCase{
		ProductRepo: mockProductRepo,
	}

	t.Run("Success", func(t *testing.T) {
		products := []*mmodel.Product{{}}
		mockProductRepo.
			EXPECT().
			FindAll(gomock.Any(), organizationID, ledgerID, filter.ToOffsetPagination()).
			Return(products, nil).
			Times(1)
		res, err := uc.ProductRepo.FindAll(context.TODO(), organizationID, ledgerID, filter.ToOffsetPagination())

		assert.NoError(t, err)
		assert.Len(t, res, 1)
	})

	t.Run("Error", func(t *testing.T) {
		errMsg := "errDatabaseItemNotFound"
		mockProductRepo.
			EXPECT().
			FindAll(gomock.Any(), organizationID, ledgerID, filter.ToOffsetPagination()).
			Return(nil, errors.New(errMsg)).
			Times(1)
		res, err := uc.ProductRepo.FindAll(context.TODO(), organizationID, ledgerID, filter.ToOffsetPagination())

		assert.EqualError(t, err, errMsg)
		assert.Nil(t, res)
	})
}
