package command

import (
	"context"
	"errors"
	"github.com/LerianStudio/midaz/common"
	"testing"

	mock "github.com/LerianStudio/midaz/components/ledger/internal/gen/mock/account"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
)

// TestDeleteAccountByIDSuccess is responsible to test DeleteAccountByID with success
func TestDeleteAccountByIDSuccess(t *testing.T) {
	organizationID := common.GenerateUUIDv7()
	ledgerID := common.GenerateUUIDv7()
	portfolioID := common.GenerateUUIDv7()
	id := common.GenerateUUIDv7()
	uc := UseCase{
		AccountRepo: mock.NewMockRepository(gomock.NewController(t)),
	}

	uc.AccountRepo.(*mock.MockRepository).
		EXPECT().
		Delete(gomock.Any(), organizationID, ledgerID, &portfolioID, id).
		Return(nil).
		Times(1)
	err := uc.AccountRepo.Delete(context.TODO(), organizationID, ledgerID, &portfolioID, id)

	assert.Nil(t, err)
}

// TestDeleteAccountByIDWithoutPortfolioSuccess is responsible to test DeleteAccountByID without portfolio with success
func TestDeleteAccountByIDWithoutPortfolioSuccess(t *testing.T) {
	organizationID := common.GenerateUUIDv7()
	ledgerID := common.GenerateUUIDv7()
	id := common.GenerateUUIDv7()
	uc := UseCase{
		AccountRepo: mock.NewMockRepository(gomock.NewController(t)),
	}

	uc.AccountRepo.(*mock.MockRepository).
		EXPECT().
		Delete(gomock.Any(), organizationID, ledgerID, nil, id).
		Return(nil).
		Times(1)
	err := uc.AccountRepo.Delete(context.TODO(), organizationID, ledgerID, nil, id)

	assert.Nil(t, err)
}

// TestDeleteAccountByIDError is responsible to test DeleteAccountByID with error
func TestDeleteAccountByIDError(t *testing.T) {
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
		Delete(gomock.Any(), organizationID, ledgerID, &portfolioID, id).
		Return(errors.New(errMSG)).
		Times(1)
	err := uc.AccountRepo.Delete(context.TODO(), organizationID, ledgerID, &portfolioID, id)

	assert.NotEmpty(t, err)
	assert.Equal(t, err.Error(), errMSG)
}
