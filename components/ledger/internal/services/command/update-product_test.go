package command

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/LerianStudio/midaz/common"
	"github.com/LerianStudio/midaz/common/mmodel"
	mock "github.com/LerianStudio/midaz/components/ledger/internal/adapters/mock/portfolio/product"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
)

// TestUpdateProductByIDSuccess is responsible to test UpdateProductByID with success
func TestUpdateProductByIDSuccess(t *testing.T) {
	id := common.GenerateUUIDv7()
	organizationID := common.GenerateUUIDv7()
	ledgerID := common.GenerateUUIDv7()
	product := &mmodel.Product{
		ID:             id.String(),
		OrganizationID: organizationID.String(),
		LedgerID:       ledgerID.String(),
		UpdatedAt:      time.Now(),
	}

	uc := UseCase{
		ProductRepo: mock.NewMockRepository(gomock.NewController(t)),
	}

	uc.ProductRepo.(*mock.MockRepository).
		EXPECT().
		Update(gomock.Any(), organizationID, ledgerID, id, product).
		Return(product, nil).
		Times(1)
	res, err := uc.ProductRepo.Update(context.TODO(), organizationID, ledgerID, id, product)

	assert.Equal(t, product, res)
	assert.Nil(t, err)
}

// TestUpdateProductByIDError is responsible to test UpdateProductByID with error
func TestUpdateProductByIDError(t *testing.T) {
	errMSG := "errDatabaseItemNotFound"
	id := common.GenerateUUIDv7()
	organizationID := common.GenerateUUIDv7()
	ledgerID := common.GenerateUUIDv7()
	product := &mmodel.Product{
		ID:             id.String(),
		OrganizationID: organizationID.String(),
		LedgerID:       ledgerID.String(),
		UpdatedAt:      time.Now(),
	}

	uc := UseCase{
		ProductRepo: mock.NewMockRepository(gomock.NewController(t)),
	}

	uc.ProductRepo.(*mock.MockRepository).
		EXPECT().
		Update(gomock.Any(), organizationID, ledgerID, id, product).
		Return(nil, errors.New(errMSG))
	res, err := uc.ProductRepo.Update(context.TODO(), organizationID, ledgerID, id, product)

	assert.NotEmpty(t, err)
	assert.Equal(t, err.Error(), errMSG)
	assert.Nil(t, res)
}
