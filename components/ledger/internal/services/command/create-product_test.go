package command

import (
	"context"
	"errors"
	"testing"

	"github.com/LerianStudio/midaz/common"
	"github.com/LerianStudio/midaz/common/mmodel"
	mock "github.com/LerianStudio/midaz/components/ledger/internal/adapters/mock/portfolio/product"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
)

// TestCreateProductSuccess is responsible to test CreateProduct with success
func TestCreateProductSuccess(t *testing.T) {
	product := &mmodel.Product{
		ID:             common.GenerateUUIDv7().String(),
		OrganizationID: common.GenerateUUIDv7().String(),
		LedgerID:       common.GenerateUUIDv7().String(),
	}

	uc := UseCase{
		ProductRepo: mock.NewMockRepository(gomock.NewController(t)),
	}

	uc.ProductRepo.(*mock.MockRepository).
		EXPECT().
		Create(gomock.Any(), product).
		Return(product, nil).
		Times(1)
	res, err := uc.ProductRepo.Create(context.TODO(), product)

	assert.Equal(t, product, res)
	assert.Nil(t, err)
}

// TestCreateProductError is responsible to test CreateProduct with error
func TestCreateProductError(t *testing.T) {
	errMSG := "err to create product on database"
	product := &mmodel.Product{
		ID:             common.GenerateUUIDv7().String(),
		OrganizationID: common.GenerateUUIDv7().String(),
		LedgerID:       common.GenerateUUIDv7().String(),
	}

	uc := UseCase{
		ProductRepo: mock.NewMockRepository(gomock.NewController(t)),
	}

	uc.ProductRepo.(*mock.MockRepository).
		EXPECT().
		Create(gomock.Any(), product).
		Return(nil, errors.New(errMSG)).
		Times(1)
	res, err := uc.ProductRepo.Create(context.TODO(), product)

	assert.NotEmpty(t, err)
	assert.Equal(t, err.Error(), errMSG)
	assert.Nil(t, res)
}
