package command

import (
	"context"
	"errors"
	"github.com/LerianStudio/midaz/common"
	"testing"

	tran "github.com/LerianStudio/midaz/components/transaction/internal/domain/transaction"
	mock "github.com/LerianStudio/midaz/components/transaction/internal/gen/mock/transaction"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
)

// TestCreateTransactionSuccess is responsible to test CreateTransaction with success
func TestCreateTransactionSuccess(t *testing.T) {
	Transaction := &tran.Transaction{
		ID:             common.GenerateUUIDv7().String(),
		OrganizationID: common.GenerateUUIDv7().String(),
		LedgerID:       common.GenerateUUIDv7().String(),
	}

	uc := UseCase{
		TransactionRepo: mock.NewMockRepository(gomock.NewController(t)),
	}

	uc.TransactionRepo.(*mock.MockRepository).
		EXPECT().
		Create(gomock.Any(), Transaction).
		Return(Transaction, nil).
		Times(1)
	res, err := uc.TransactionRepo.Create(context.TODO(), Transaction)

	assert.Equal(t, Transaction, res)
	assert.Nil(t, err)
}

// TestCreateTransactionError is responsible to test CreateTransaction with error
func TestCreateTransactionError(t *testing.T) {
	errMSG := "err to create Transaction on database"

	ID := common.GenerateUUIDv7().String()
	Transaction := &tran.Transaction{
		ID:                  ID,
		OrganizationID:      common.GenerateUUIDv7().String(),
		LedgerID:            common.GenerateUUIDv7().String(),
		ParentTransactionID: &ID,
	}

	uc := UseCase{
		TransactionRepo: mock.NewMockRepository(gomock.NewController(t)),
	}

	uc.TransactionRepo.(*mock.MockRepository).
		EXPECT().
		Create(gomock.Any(), Transaction).
		Return(nil, errors.New(errMSG)).
		Times(1)
	res, err := uc.TransactionRepo.Create(context.TODO(), Transaction)

	assert.NotEmpty(t, err)
	assert.Equal(t, err.Error(), errMSG)
	assert.Nil(t, res)
}
