package command

import (
	"context"
	"errors"
	"testing"

	a "github.com/LerianStudio/midaz/components/ledger/internal/domain/portfolio/account"
	mock "github.com/LerianStudio/midaz/components/ledger/internal/gen/mock/account"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
)

// TestCreateAccountSuccess is responsible to test CreateAccount with success
func TestCreateAccountSuccess(t *testing.T) {
	account := &a.Account{
		ID:             uuid.New().String(),
		OrganizationID: uuid.New().String(),
		LedgerID:       uuid.New().String(),
		PortfolioID:    uuid.New().String(),
	}

	uc := UseCase{
		AccountRepo: mock.NewMockRepository(gomock.NewController(t)),
	}

	uc.AccountRepo.(*mock.MockRepository).
		EXPECT().
		Create(gomock.Any(), account).
		Return(account, nil).
		Times(1)
	res, err := uc.AccountRepo.Create(context.TODO(), account)

	assert.Equal(t, account, res)
	assert.Nil(t, err)
}

// TestCreateAccountError is responsible to test CreateAccount with error
func TestCreateAccountError(t *testing.T) {
	errMSG := "err to create account on database"
	account := &a.Account{
		ID:             uuid.New().String(),
		OrganizationID: uuid.New().String(),
		LedgerID:       uuid.New().String(),
		PortfolioID:    uuid.New().String(),
	}

	uc := UseCase{
		AccountRepo: mock.NewMockRepository(gomock.NewController(t)),
	}

	uc.AccountRepo.(*mock.MockRepository).
		EXPECT().
		Create(gomock.Any(), account).
		Return(nil, errors.New(errMSG)).
		Times(1)
	res, err := uc.AccountRepo.Create(context.TODO(), account)

	assert.NotEmpty(t, err)
	assert.Equal(t, err.Error(), errMSG)
	assert.Nil(t, res)
}
