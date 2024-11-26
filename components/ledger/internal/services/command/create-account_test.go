package command

import (
	"context"
	"errors"
	"testing"

	"github.com/LerianStudio/midaz/pkg"
	"github.com/LerianStudio/midaz/pkg/mmodel"
	"github.com/LerianStudio/midaz/pkg/mpointers"
	"github.com/LerianStudio/midaz/components/ledger/internal/adapters/postgres/account"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
)

// TestCreateAccountSuccess is responsible to test CreateAccount with success
func TestCreateAccountSuccess(t *testing.T) {
	a := &mmodel.Account{
		ID:             pkg.GenerateUUIDv7().String(),
		OrganizationID: pkg.GenerateUUIDv7().String(),
		LedgerID:       pkg.GenerateUUIDv7().String(),
		PortfolioID:    mpointers.String(pkg.GenerateUUIDv7().String()),
	}

	uc := UseCase{
		AccountRepo: account.NewMockRepository(gomock.NewController(t)),
	}

	uc.AccountRepo.(*account.MockRepository).
		EXPECT().
		Create(gomock.Any(), a).
		Return(a, nil).
		Times(1)
	res, err := uc.AccountRepo.Create(context.TODO(), a)

	assert.Equal(t, a, res)
	assert.Nil(t, err)
}

// TestCreateWithoutPortfolioAccountSuccess is responsible to test CreateAccountWithoutPortfolio with success
func TestCreateWithoutPortfolioAccountSuccess(t *testing.T) {
	a := &mmodel.Account{
		ID:             pkg.GenerateUUIDv7().String(),
		OrganizationID: pkg.GenerateUUIDv7().String(),
		LedgerID:       pkg.GenerateUUIDv7().String(),
	}

	uc := UseCase{
		AccountRepo: account.NewMockRepository(gomock.NewController(t)),
	}

	uc.AccountRepo.(*account.MockRepository).
		EXPECT().
		Create(gomock.Any(), a).
		Return(a, nil).
		Times(1)
	res, err := uc.AccountRepo.Create(context.TODO(), a)

	assert.Equal(t, a, res)
	assert.Nil(t, err)
}

// TestCreateAccountError is responsible to test CreateAccount with error
func TestCreateAccountError(t *testing.T) {
	errMSG := "err to create account on database"
	a := &mmodel.Account{
		ID:             pkg.GenerateUUIDv7().String(),
		OrganizationID: pkg.GenerateUUIDv7().String(),
		LedgerID:       pkg.GenerateUUIDv7().String(),
		PortfolioID:    mpointers.String(pkg.GenerateUUIDv7().String()),
	}

	uc := UseCase{
		AccountRepo: account.NewMockRepository(gomock.NewController(t)),
	}

	uc.AccountRepo.(*account.MockRepository).
		EXPECT().
		Create(gomock.Any(), a).
		Return(nil, errors.New(errMSG)).
		Times(1)
	res, err := uc.AccountRepo.Create(context.TODO(), a)

	assert.NotEmpty(t, err)
	assert.Equal(t, err.Error(), errMSG)
	assert.Nil(t, res)
}
