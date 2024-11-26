package command

import (
	"context"
	"errors"
	"testing"

	"github.com/LerianStudio/midaz/pkg"
	"github.com/LerianStudio/midaz/components/ledger/internal/adapters/postgres/product"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
)

// TestDeleteProductByIDSuccess is responsible to test DeleteProductByID with success
func TestDeleteProductByIDSuccess(t *testing.T) {
	id := pkg.GenerateUUIDv7()
	organizationID := pkg.GenerateUUIDv7()
	ledgerID := pkg.GenerateUUIDv7()

	uc := UseCase{
		ProductRepo: product.NewMockRepository(gomock.NewController(t)),
	}

	uc.ProductRepo.(*product.MockRepository).
		EXPECT().
		Delete(gomock.Any(), organizationID, ledgerID, id).
		Return(nil).
		Times(1)
	err := uc.ProductRepo.Delete(context.TODO(), organizationID, ledgerID, id)

	assert.Nil(t, err)
}

// TestDeleteProductByIDError is responsible to test DeleteProductByID with error
func TestDeleteProductByIDError(t *testing.T) {
	id := pkg.GenerateUUIDv7()
	organizationID := pkg.GenerateUUIDv7()
	ledgerID := pkg.GenerateUUIDv7()
	errMSG := "errDatabaseItemNotFound"

	uc := UseCase{
		ProductRepo: product.NewMockRepository(gomock.NewController(t)),
	}

	uc.ProductRepo.(*product.MockRepository).
		EXPECT().
		Delete(gomock.Any(), organizationID, ledgerID, id).
		Return(errors.New(errMSG)).
		Times(1)
	err := uc.ProductRepo.Delete(context.TODO(), organizationID, ledgerID, id)

	assert.NotEmpty(t, err)
	assert.Equal(t, err.Error(), errMSG)
}
