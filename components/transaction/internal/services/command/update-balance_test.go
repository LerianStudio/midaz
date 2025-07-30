package command

import (
	"context"
	"errors"
	libCommons "github.com/LerianStudio/lib-commons/commons"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/postgres/balance"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
	"testing"
)

// TestUpdateBalanceSuccess is responsible to test UpdateBalanceSuccess with success
func TestUpdateBalanceSuccess(t *testing.T) {
	organizationID := libCommons.GenerateUUIDv7()
	ledgerID := libCommons.GenerateUUIDv7()
	balanceID := libCommons.GenerateUUIDv7()

	allowSending := false

	balanceUpdate := mmodel.UpdateBalance{
		AllowSending:   &allowSending,
		AllowReceiving: nil,
	}

	uc := UseCase{
		BalanceRepo: balance.NewMockRepository(gomock.NewController(t)),
	}

	uc.BalanceRepo.(*balance.MockRepository).
		EXPECT().
		Update(gomock.Any(), organizationID, ledgerID, balanceID, balanceUpdate).
		Return(nil).
		Times(1)
	err := uc.BalanceRepo.Update(context.TODO(), organizationID, ledgerID, balanceID, balanceUpdate)

	assert.Nil(t, err)
}

// TestUpdateBalanceError is responsible to test UpdateBalanceError with error
func TestUpdateBalanceError(t *testing.T) {
	errMSG := "errDatabaseItemNotFound"
	organizationID := libCommons.GenerateUUIDv7()
	ledgerID := libCommons.GenerateUUIDv7()
	balanceID := libCommons.GenerateUUIDv7()

	allowSending := true
	allowReceiving := false

	balanceUpdate := mmodel.UpdateBalance{
		AllowSending:   &allowSending,
		AllowReceiving: &allowReceiving,
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
