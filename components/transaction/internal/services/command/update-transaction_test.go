package command

import (
	"context"
	"errors"
	"go.uber.org/mock/gomock"
	"testing"
	"time"

	"github.com/LerianStudio/midaz/components/transaction/internal/adapters/postgres/transaction"
	"github.com/LerianStudio/midaz/pkg"

	"github.com/stretchr/testify/assert"
)

// TestUpdateTransactionSuccess is responsible to test UpdateTransactionSuccess with success
func TestUpdateTransactionSuccess(t *testing.T) {
	organizationID := pkg.GenerateUUIDv7()
	ledgerID := pkg.GenerateUUIDv7()
	transactionID := pkg.GenerateUUIDv7()

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
	organizationID := pkg.GenerateUUIDv7()
	ledgerID := pkg.GenerateUUIDv7()
	transactionID := pkg.GenerateUUIDv7()

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
