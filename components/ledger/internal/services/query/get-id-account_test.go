package query

import (
	"context"
	"errors"
	"go.uber.org/mock/gomock"
	"testing"

	"github.com/LerianStudio/midaz/components/ledger/internal/adapters/postgres/account"
	"github.com/LerianStudio/midaz/pkg"
	"github.com/LerianStudio/midaz/pkg/mmodel"
	"github.com/LerianStudio/midaz/pkg/mpointers"

	"github.com/stretchr/testify/assert"
)

// TestGetAccountByIDSuccess is responsible to test GetAccountByID with success
func TestGetAccountByIDSuccess(t *testing.T) {
	organizationID := pkg.GenerateUUIDv7()
	ledgerID := pkg.GenerateUUIDv7()
	portfolioID := pkg.GenerateUUIDv7()
	id := pkg.GenerateUUIDv7()

	a := &mmodel.Account{
		ID:             id.String(),
		OrganizationID: organizationID.String(),
		LedgerID:       ledgerID.String(),
		PortfolioID:    mpointers.String(portfolioID.String()),
	}

	uc := UseCase{
		AccountRepo: account.NewMockRepository(gomock.NewController(t)),
	}

	uc.AccountRepo.(*account.MockRepository).
		EXPECT().
		Find(gomock.Any(), organizationID, ledgerID, &portfolioID, id).
		Return(a, nil).
		Times(1)
	res, err := uc.AccountRepo.Find(context.TODO(), organizationID, ledgerID, &portfolioID, id)

	assert.Equal(t, res, a)
	assert.Nil(t, err)
}

// TestGetAccountByIDWithoutPortfolioSuccess is responsible to test GetAccountByID without portfolio with success
func TestGetAccountByIDWithoutPortfolioSuccess(t *testing.T) {
	organizationID := pkg.GenerateUUIDv7()
	ledgerID := pkg.GenerateUUIDv7()
	id := pkg.GenerateUUIDv7()

	a := &mmodel.Account{
		ID:             id.String(),
		OrganizationID: organizationID.String(),
		LedgerID:       ledgerID.String(),
		PortfolioID:    nil,
	}

	uc := UseCase{
		AccountRepo: account.NewMockRepository(gomock.NewController(t)),
	}

	uc.AccountRepo.(*account.MockRepository).
		EXPECT().
		Find(gomock.Any(), organizationID, ledgerID, nil, id).
		Return(a, nil).
		Times(1)
	res, err := uc.AccountRepo.Find(context.TODO(), organizationID, ledgerID, nil, id)

	assert.Equal(t, res, a)
	assert.Nil(t, err)
}

// TestGetAccountByIDError is responsible to test GetAccountByID with error
func TestGetAccountByIDError(t *testing.T) {
	organizationID := pkg.GenerateUUIDv7()
	ledgerID := pkg.GenerateUUIDv7()
	portfolioID := pkg.GenerateUUIDv7()
	id := pkg.GenerateUUIDv7()

	errMSG := "errDatabaseItemNotFound"

	uc := UseCase{
		AccountRepo: account.NewMockRepository(gomock.NewController(t)),
	}

	uc.AccountRepo.(*account.MockRepository).
		EXPECT().
		Find(gomock.Any(), organizationID, ledgerID, &portfolioID, id).
		Return(nil, errors.New(errMSG)).
		Times(1)
	res, err := uc.AccountRepo.Find(context.TODO(), organizationID, ledgerID, &portfolioID, id)

	assert.NotEmpty(t, err)
	assert.Equal(t, err.Error(), errMSG)
	assert.Nil(t, res)
}
