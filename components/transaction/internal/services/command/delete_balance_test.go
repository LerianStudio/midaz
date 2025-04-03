package command

import (
	"context"
	"errors"
	"testing"

	libCommons "github.com/LerianStudio/lib-commons/commons"
	"github.com/LerianStudio/midaz/components/transaction/internal/adapters/postgres/balance"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
)

// TestDeleteBalanceSuccess is responsible to test DeleteBalanceSuccess with success
func TestDeleteBalanceSuccess(t *testing.T) {
	organizationID := libCommons.GenerateUUIDv7()
	ledgerID := libCommons.GenerateUUIDv7()
	balanceID := libCommons.GenerateUUIDv7()

	uc := UseCase{
		BalanceRepo: balance.NewMockRepository(gomock.NewController(t)),
	}

	uc.BalanceRepo.(*balance.MockRepository).
		EXPECT().
		Delete(gomock.Any(), organizationID, ledgerID, balanceID).
		Return(nil).
		Times(1)
	err := uc.BalanceRepo.Delete(context.TODO(), organizationID, ledgerID, balanceID)

	assert.Nil(t, err)
}

// TestDeleteBalanceError is responsible to test DeleteBalanceError with error
func TestDeleteBalanceError(t *testing.T) {
	errMSG := "errDatabaseItemNotFound"
	organizationID := libCommons.GenerateUUIDv7()
	ledgerID := libCommons.GenerateUUIDv7()
	balanceID := libCommons.GenerateUUIDv7()

	uc := UseCase{
		BalanceRepo: balance.NewMockRepository(gomock.NewController(t)),
	}

	uc.BalanceRepo.(*balance.MockRepository).
		EXPECT().
		Delete(gomock.Any(), organizationID, ledgerID, balanceID).
		Return(errors.New(errMSG))
	err := uc.BalanceRepo.Delete(context.TODO(), organizationID, ledgerID, balanceID)

	assert.NotEmpty(t, err)
	assert.Equal(t, err.Error(), errMSG)
}
