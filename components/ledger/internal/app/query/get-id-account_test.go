package query

import (
	"context"
	"errors"
	"github.com/LerianStudio/midaz/common"
	"testing"

	a "github.com/LerianStudio/midaz/components/ledger/internal/domain/portfolio/account"
	mock "github.com/LerianStudio/midaz/components/ledger/internal/gen/mock/account"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
)

// TestGetAccountByIDSuccess is responsible to test GetAccountByID with success
func TestGetAccountByIDSuccess(t *testing.T) {
	organizationID := common.GenerateUUIDv7()
	ledgerID := common.GenerateUUIDv7()
	portfolioID := common.GenerateUUIDv7()
	id := common.GenerateUUIDv7()

	account := &a.Account{
		ID:             id.String(),
		OrganizationID: organizationID.String(),
		LedgerID:       ledgerID.String(),
		PortfolioID:    portfolioID.String(),
	}

	uc := UseCase{
		AccountRepo: mock.NewMockRepository(gomock.NewController(t)),
	}

	uc.AccountRepo.(*mock.MockRepository).
		EXPECT().
		Find(gomock.Any(), organizationID, ledgerID, portfolioID, id).
		Return(account, nil).
		Times(1)
	res, err := uc.AccountRepo.Find(context.TODO(), organizationID, ledgerID, portfolioID, id)

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
		Find(gomock.Any(), organizationID, ledgerID, portfolioID, id).
		Return(nil, errors.New(errMSG)).
		Times(1)
	res, err := uc.AccountRepo.Find(context.TODO(), organizationID, ledgerID, portfolioID, id)

	assert.NotEmpty(t, err)
	assert.Equal(t, err.Error(), errMSG)
	assert.Nil(t, res)
}
