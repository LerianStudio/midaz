package command

import (
	"context"
	"errors"
	"testing"

	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/postgres/transaction"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
)

// TestCreateTransactionSuccess is responsible to test CreateTransaction with success
func TestCreateTransactionSuccess(t *testing.T) {
	tran := &transaction.Transaction{
		ID:             libCommons.GenerateUUIDv7().String(),
		OrganizationID: libCommons.GenerateUUIDv7().String(),
		LedgerID:       libCommons.GenerateUUIDv7().String(),
	}

	uc := UseCase{
		TransactionRepo: transaction.NewMockRepository(gomock.NewController(t)),
	}

	uc.TransactionRepo.(*transaction.MockRepository).
		EXPECT().
		Create(gomock.Any(), tran).
		Return(tran, nil).
		Times(1)
	res, err := uc.TransactionRepo.Create(context.TODO(), tran)

	assert.Equal(t, tran, res)
	assert.Nil(t, err)
}

// TestCreateTransactionError is responsible to test CreateTransaction with error
func TestCreateTransactionError(t *testing.T) {
	errMSG := "err to create tran on database"

	ID := libCommons.GenerateUUIDv7().String()
	tran := &transaction.Transaction{
		ID:                  ID,
		OrganizationID:      libCommons.GenerateUUIDv7().String(),
		LedgerID:            libCommons.GenerateUUIDv7().String(),
		ParentTransactionID: &ID,
	}

	uc := UseCase{
		TransactionRepo: transaction.NewMockRepository(gomock.NewController(t)),
	}

	uc.TransactionRepo.(*transaction.MockRepository).
		EXPECT().
		Create(gomock.Any(), tran).
		Return(nil, errors.New(errMSG)).
		Times(1)
	res, err := uc.TransactionRepo.Create(context.TODO(), tran)

	assert.NotEmpty(t, err)
	assert.Equal(t, err.Error(), errMSG)
	assert.Nil(t, res)
}
