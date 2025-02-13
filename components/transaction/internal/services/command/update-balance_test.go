package command

import (
	"context"
	"errors"
	"github.com/LerianStudio/midaz/components/transaction/internal/adapters/postgres/balance"
	"github.com/LerianStudio/midaz/pkg"
	"github.com/LerianStudio/midaz/pkg/mmodel"
	"go.uber.org/mock/gomock"
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestUpdateBalanceSuccess is responsible to test UpdateBalanceSuccess with success
func TestUpdateBalanceSuccess(t *testing.T) {
	organizationID := pkg.GenerateUUIDv7()
	ledgerID := pkg.GenerateUUIDv7()
	balanceID := pkg.GenerateUUIDv7()

	balanceUpdate := &mmodel.UpdateBalance{
		AllowSending:   true,
		AllowReceiving: false,
	}

	uc := UseCase{
		BalanceRepo: balance.NewMockRepository(gomock.NewController(t)),
	}

	uc.BalanceRepo.(*balance.MockRepository).
		EXPECT().
		Update(gomock.Any(), organizationID, ledgerID, balanceID, balanceUpdate).
		Return(balanceUpdate, nil).
		Times(1)
	err := uc.BalanceRepo.Update(context.TODO(), organizationID, ledgerID, balanceID, *balanceUpdate)

	assert.Nil(t, err)
}

// TestUpdateBalanceError is responsible to test UpdateBalanceError with error
func TestUpdateBalanceError(t *testing.T) {
	errMSG := "errDatabaseItemNotFound"
	organizationID := pkg.GenerateUUIDv7()
	ledgerID := pkg.GenerateUUIDv7()
	balanceID := pkg.GenerateUUIDv7()

	balanceUpdate := mmodel.UpdateBalance{
		AllowSending:   true,
		AllowReceiving: false,
	}

	uc := UseCase{
		BalanceRepo: balance.NewMockRepository(gomock.NewController(t)),
	}

	uc.BalanceRepo.(*balance.MockRepository).
		EXPECT().
		Update(gomock.Any(), organizationID, ledgerID, balanceID, balanceUpdate).
		Return(errors.New(errMSG))
	err := uc.BalanceRepo.Update(context.TODO(), organizationID, ledgerID, balanceID, balanceUpdate)

	assert.NotEmpty(t, err)
	assert.Equal(t, err.Error(), errMSG)
}
