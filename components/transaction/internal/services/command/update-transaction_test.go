package command

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/LerianStudio/midaz/common"
	"github.com/LerianStudio/midaz/components/transaction/internal/adapters/database/postgres/transaction"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
)

// TestUpdateTransactionSuccess is responsible to test UpdateTransactionSuccess with success
func TestUpdateTransactionSuccess(t *testing.T) {
	organizationID := common.GenerateUUIDv7()
	ledgerID := common.GenerateUUIDv7()
	transactionID := common.GenerateUUIDv7()

	trans := &transaction.Transaction{
		ID:             transactionID.String(),
		LedgerID:       ledgerID.String(),
		OrganizationID: organizationID.String(),
		UpdatedAt:      time.Now(),
	}

	uc := UseCase{
		TransactionRepo: transaction.NewMockRepository(gomock.NewController(t)),
	}

	uc.TransactionRepo.(*transaction.MockRepository).
		EXPECT().
		Update(gomock.Any(), organizationID, ledgerID, transactionID, trans).
		Return(trans, nil).
		Times(1)
	res, err := uc.TransactionRepo.Update(context.TODO(), organizationID, ledgerID, transactionID, trans)

	assert.Equal(t, trans, res)
	assert.Nil(t, err)
}

// TestUpdateTransactionError is responsible to test UpdateTransactionError with error
func TestUpdateTransactionError(t *testing.T) {
	errMSG := "errDatabaseItemNotFound"
	organizationID := common.GenerateUUIDv7()
	ledgerID := common.GenerateUUIDv7()
	transactionID := common.GenerateUUIDv7()

	trans := &transaction.Transaction{
		ID:             transactionID.String(),
		LedgerID:       ledgerID.String(),
		OrganizationID: organizationID.String(),
		UpdatedAt:      time.Now(),
	}

	uc := UseCase{
		TransactionRepo: transaction.NewMockRepository(gomock.NewController(t)),
	}

	uc.TransactionRepo.(*transaction.MockRepository).
		EXPECT().
		Update(gomock.Any(), organizationID, ledgerID, transactionID, trans).
		Return(nil, errors.New(errMSG))
	res, err := uc.TransactionRepo.Update(context.TODO(), organizationID, ledgerID, transactionID, trans)

	assert.NotEmpty(t, err)
	assert.Equal(t, err.Error(), errMSG)
	assert.Nil(t, res)
}
