package query

import (
	"context"
	"errors"
	"testing"

	"github.com/LerianStudio/midaz/common"
	"github.com/LerianStudio/midaz/common/mmodel"
	"github.com/LerianStudio/midaz/common/mpointers"
	mock "github.com/LerianStudio/midaz/components/ledger/internal/adapters/mock/portfolio/account"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
)

// TestGetAccountByIDSuccess is responsible to test GetAccountByID with success
func TestGetAccountByIDSuccess(t *testing.T) {
	organizationID := common.GenerateUUIDv7()
	ledgerID := common.GenerateUUIDv7()
	portfolioID := common.GenerateUUIDv7()
	id := common.GenerateUUIDv7()

	account := &mmodel.Account{
		ID:             id.String(),
		OrganizationID: organizationID.String(),
		LedgerID:       ledgerID.String(),
		PortfolioID:    mpointers.String(portfolioID.String()),
	}

	uc := UseCase{
		AccountRepo: mock.NewMockRepository(gomock.NewController(t)),
	}

	uc.AccountRepo.(*mock.MockRepository).
		EXPECT().
		Find(gomock.Any(), organizationID, ledgerID, &portfolioID, id).
		Return(account, nil).
		Times(1)
	res, err := uc.AccountRepo.Find(context.TODO(), organizationID, ledgerID, &portfolioID, id)

	assert.Equal(t, res, account)
	assert.Nil(t, err)
}

// TestGetAccountByIDWithoutPortfolioSuccess is responsible to test GetAccountByID without portfolio with success
func TestGetAccountByIDWithoutPortfolioSuccess(t *testing.T) {
	organizationID := common.GenerateUUIDv7()
	ledgerID := common.GenerateUUIDv7()
	id := common.GenerateUUIDv7()

	account := &mmodel.Account{
		ID:             id.String(),
		OrganizationID: organizationID.String(),
		LedgerID:       ledgerID.String(),
		PortfolioID:    nil,
	}

	uc := UseCase{
		AccountRepo: mock.NewMockRepository(gomock.NewController(t)),
	}

	uc.AccountRepo.(*mock.MockRepository).
		EXPECT().
		Find(gomock.Any(), organizationID, ledgerID, nil, id).
		Return(account, nil).
		Times(1)
	res, err := uc.AccountRepo.Find(context.TODO(), organizationID, ledgerID, nil, id)

	assert.Equal(t, res, account)
	assert.Nil(t, err)
}

// TestGetAccountByIDError is responsible to test GetAccountByID with error
func TestGetAccountByIDError(t *testing.T) {
	organizationID := common.GenerateUUIDv7()
	ledgerID := common.GenerateUUIDv7()
	portfolioID := common.GenerateUUIDv7()
	id := common.GenerateUUIDv7()

	errMSG := "errDatabaseItemNotFound"

	uc := UseCase{
		AccountRepo: mock.NewMockRepository(gomock.NewController(t)),
	}

	uc.AccountRepo.(*mock.MockRepository).
		EXPECT().
		Find(gomock.Any(), organizationID, ledgerID, &portfolioID, id).
		Return(nil, errors.New(errMSG)).
		Times(1)
	res, err := uc.AccountRepo.Find(context.TODO(), organizationID, ledgerID, &portfolioID, id)

	assert.NotEmpty(t, err)
	assert.Equal(t, err.Error(), errMSG)
	assert.Nil(t, res)
}
