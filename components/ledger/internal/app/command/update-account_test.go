package command

import (
	"context"
	"errors"
	"github.com/LerianStudio/midaz/common"
	"testing"
	"time"

	a "github.com/LerianStudio/midaz/components/ledger/internal/domain/portfolio/account"
	mock "github.com/LerianStudio/midaz/components/ledger/internal/gen/mock/account"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
)

// TestUpdateAccountByIDSuccess is responsible to test UpdateAccountByID with success
func TestUpdateAccountByIDSuccess(t *testing.T) {
	organizationID := common.GenerateUUIDv7()
	ledgerID := common.GenerateUUIDv7()
	portfolioID := common.GenerateUUIDv7()
	id := common.GenerateUUIDv7()
	account := &a.Account{
		ID:        id.String(),
		UpdatedAt: time.Now(),
	}

	uc := UseCase{
		AccountRepo: mock.NewMockRepository(gomock.NewController(t)),
	}

	uc.AccountRepo.(*mock.MockRepository).
		EXPECT().
		Update(gomock.Any(), organizationID, ledgerID, portfolioID, id, account).
		Return(account, nil).
		Times(1)
	res, err := uc.AccountRepo.Update(context.TODO(), organizationID, ledgerID, portfolioID, id, account)

	assert.Equal(t, account, res)
	assert.Nil(t, err)
}

// TestUpdateAccountByIDError is responsible to test UpdateAccountByID with error
func TestUpdateAccountByIDError(t *testing.T) {
	errMSG := "errDatabaseItemNotFound"
	organizationID := common.GenerateUUIDv7()
	ledgerID := common.GenerateUUIDv7()
	portfolioID := common.GenerateUUIDv7()
	id := common.GenerateUUIDv7()
	account := &a.Account{
		ID:        id.String(),
		UpdatedAt: time.Now(),
	}

	uc := UseCase{
		AccountRepo: mock.NewMockRepository(gomock.NewController(t)),
	}

	uc.AccountRepo.(*mock.MockRepository).
		EXPECT().
		Update(gomock.Any(), organizationID, ledgerID, portfolioID, id, account).
		Return(nil, errors.New(errMSG))
	res, err := uc.AccountRepo.Update(context.TODO(), organizationID, ledgerID, portfolioID, id, account)

	assert.NotEmpty(t, err)
	assert.Equal(t, err.Error(), errMSG)
	assert.Nil(t, res)
}
