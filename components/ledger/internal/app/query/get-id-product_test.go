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

// TestGetProductByIDSuccess is responsible to test GetProductByID with success
func TestGetProductByIDSuccess(t *testing.T) {
	id := uuid.New()
	organizationID := uuid.New()
	ledgerID := uuid.New()
	product := &p.Product{
		ID:             id.String(),
		LedgerID:       ledgerID.String(),
		OrganizationID: organizationID.String(),
	}

	uc := UseCase{
		ProductRepo: mock.NewMockRepository(gomock.NewController(t)),
	}

	uc.ProductRepo.(*mock.MockRepository).
		EXPECT().
		Find(gomock.Any(), organizationID, ledgerID, id).
		Return(product, nil).
		Times(1)
	res, err := uc.ProductRepo.Find(context.TODO(), organizationID, ledgerID, id)

	assert.Equal(t, res, product)
	assert.Nil(t, err)
}

// TestGetProductByIDError is responsible to test GetProductByID with error
func TestGetProductByIDError(t *testing.T) {
	id := uuid.New()
	organizationID := uuid.New()
	ledgerID := uuid.New()
	errMSG := "errDatabaseItemNotFound"

	uc := UseCase{
		ProductRepo: mock.NewMockRepository(gomock.NewController(t)),
	}

	uc.ProductRepo.(*mock.MockRepository).
		EXPECT().
		Find(gomock.Any(), organizationID, ledgerID, id).
		Return(nil, errors.New(errMSG)).
		Times(1)
	res, err := uc.ProductRepo.Find(context.TODO(), organizationID, ledgerID, id)

	assert.NotEmpty(t, err)
	assert.Equal(t, err.Error(), errMSG)
	assert.Nil(t, res)
}
