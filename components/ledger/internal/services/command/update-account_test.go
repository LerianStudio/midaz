package command

import (
	"context"
	"errors"
	"go.uber.org/mock/gomock"
	"testing"
	"time"

	"github.com/LerianStudio/midaz/components/ledger/internal/adapters/postgres/account"
	"github.com/LerianStudio/midaz/pkg"
	"github.com/LerianStudio/midaz/pkg/mmodel"

	"github.com/stretchr/testify/assert"
)

// TestUpdateAccountByIDSuccess is responsible to test UpdateAccountByID with success
func TestUpdateAccountByIDSuccess(t *testing.T) {
	organizationID := pkg.GenerateUUIDv7()
	ledgerID := pkg.GenerateUUIDv7()
	portfolioID := pkg.GenerateUUIDv7()
	id := pkg.GenerateUUIDv7()
	a := &mmodel.Account{
		ID:        id.String(),
		UpdatedAt: time.Now(),
	}

	uc := UseCase{
		AccountRepo: account.NewMockRepository(gomock.NewController(t)),
	}

	uc.AccountRepo.(*account.MockRepository).
		EXPECT().
		Update(gomock.Any(), organizationID, ledgerID, &portfolioID, id, a).
		Return(a, nil).
		Times(1)
	res, err := uc.AccountRepo.Update(context.TODO(), organizationID, ledgerID, &portfolioID, id, a)

	assert.Equal(t, a, res)
	assert.Nil(t, err)
}

// TestUpdateAccountByIDWithoutPortfolioSuccess is responsible to test UpdateAccountByIDWithoutPortfolio with success
func TestUpdateAccountByIDWithoutPortfolioSuccess(t *testing.T) {
	organizationID := pkg.GenerateUUIDv7()
	ledgerID := pkg.GenerateUUIDv7()
	id := pkg.GenerateUUIDv7()
	a := &mmodel.Account{
		ID:        id.String(),
		UpdatedAt: time.Now(),
	}

	uc := UseCase{
		AccountRepo: account.NewMockRepository(gomock.NewController(t)),
	}

	uc.AccountRepo.(*account.MockRepository).
		EXPECT().
		Update(gomock.Any(), organizationID, ledgerID, nil, id, a).
		Return(a, nil).
		Times(1)
	res, err := uc.AccountRepo.Update(context.TODO(), organizationID, ledgerID, nil, id, a)

	assert.Equal(t, a, res)
	assert.Nil(t, err)
}

// TestUpdateAccountByIDError is responsible to test UpdateAccountByID with error
func TestUpdateAccountByIDError(t *testing.T) {
	errMSG := "errDatabaseItemNotFound"
	organizationID := pkg.GenerateUUIDv7()
	ledgerID := pkg.GenerateUUIDv7()
	portfolioID := pkg.GenerateUUIDv7()
	id := pkg.GenerateUUIDv7()
	a := &mmodel.Account{
		ID:        id.String(),
		UpdatedAt: time.Now(),
	}

	uc := UseCase{
		AccountRepo: account.NewMockRepository(gomock.NewController(t)),
	}

	uc.AccountRepo.(*account.MockRepository).
		EXPECT().
		Update(gomock.Any(), organizationID, ledgerID, &portfolioID, id, a).
		Return(nil, errors.New(errMSG))
	res, err := uc.AccountRepo.Update(context.TODO(), organizationID, ledgerID, &portfolioID, id, a)

	assert.NotEmpty(t, err)
	assert.Equal(t, err.Error(), errMSG)
	assert.Nil(t, res)
}
