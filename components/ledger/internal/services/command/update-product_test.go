package command

import (
	"context"
	"errors"
	"go.uber.org/mock/gomock"
	"testing"
	"time"

	"github.com/LerianStudio/midaz/components/ledger/internal/adapters/postgres/product"
	"github.com/LerianStudio/midaz/pkg"
	"github.com/LerianStudio/midaz/pkg/mmodel"

	"github.com/stretchr/testify/assert"
)

// TestUpdateProductByIDSuccess is responsible to test UpdateProductByID with success
func TestUpdateProductByIDSuccess(t *testing.T) {
	id := pkg.GenerateUUIDv7()
	organizationID := pkg.GenerateUUIDv7()
	ledgerID := pkg.GenerateUUIDv7()
	p := &mmodel.Product{
		ID:             id.String(),
		OrganizationID: organizationID.String(),
		LedgerID:       ledgerID.String(),
		UpdatedAt:      time.Now(),
	}

	uc := UseCase{
		ProductRepo: product.NewMockRepository(gomock.NewController(t)),
	}

	uc.ProductRepo.(*product.MockRepository).
		EXPECT().
		Update(gomock.Any(), organizationID, ledgerID, id, p).
		Return(p, nil).
		Times(1)
	res, err := uc.ProductRepo.Update(context.TODO(), organizationID, ledgerID, id, p)

	assert.Equal(t, p, res)
	assert.Nil(t, err)
}

// TestUpdateProductByIDError is responsible to test UpdateProductByID with error
func TestUpdateProductByIDError(t *testing.T) {
	errMSG := "errDatabaseItemNotFound"
	id := pkg.GenerateUUIDv7()
	organizationID := pkg.GenerateUUIDv7()
	ledgerID := pkg.GenerateUUIDv7()
	p := &mmodel.Product{
		ID:             id.String(),
		OrganizationID: organizationID.String(),
		LedgerID:       ledgerID.String(),
		UpdatedAt:      time.Now(),
	}

	uc := UseCase{
		ProductRepo: product.NewMockRepository(gomock.NewController(t)),
	}

	uc.ProductRepo.(*product.MockRepository).
		EXPECT().
		Update(gomock.Any(), organizationID, ledgerID, id, p).
		Return(nil, errors.New(errMSG))
	res, err := uc.ProductRepo.Update(context.TODO(), organizationID, ledgerID, id, p)

	assert.NotEmpty(t, err)
	assert.Equal(t, err.Error(), errMSG)
	assert.Nil(t, res)
}
