package command

import (
	"context"
	"errors"
	"testing"

	mock "github.com/LerianStudio/midaz/components/ledger/internal/gen/mock/product"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
)

// TestDeleteProductByIDSuccess is responsible to test DeleteProductByID with success
func TestDeleteProductByIDSuccess(t *testing.T) {
	id := uuid.New()
	organizationID := uuid.New()
	ledgerID := uuid.New()

	uc := UseCase{
		ProductRepo: mock.NewMockRepository(gomock.NewController(t)),
	}

	uc.ProductRepo.(*mock.MockRepository).
		EXPECT().
		Delete(gomock.Any(), organizationID, ledgerID, id).
		Return(nil).
		Times(1)
	err := uc.ProductRepo.Delete(context.TODO(), organizationID, ledgerID, id)

	assert.Nil(t, err)
}

// TestDeleteProductByIDError is responsible to test DeleteProductByID with error
func TestDeleteProductByIDError(t *testing.T) {
	id := uuid.New()
	organizationID := uuid.New()
	ledgerID := uuid.New()
	errMSG := "errDatabaseItemNotFound"

	uc := UseCase{
		ProductRepo: mock.NewMockRepository(gomock.NewController(t)),
	}

	uc.ProductRepo.(*mock.MockRepository).
		EXPECT().
		Delete(gomock.Any(), organizationID, ledgerID, id).
		Return(errors.New(errMSG)).
		Times(1)
	err := uc.ProductRepo.Delete(context.TODO(), organizationID, ledgerID, id)

	assert.NotEmpty(t, err)
	assert.Equal(t, err.Error(), errMSG)
}
