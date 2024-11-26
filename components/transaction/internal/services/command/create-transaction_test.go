package command

import (
	"context"
	"errors"
	"go.uber.org/mock/gomock"
	"testing"

	"github.com/LerianStudio/midaz/components/transaction/internal/adapters/postgres/transaction"
	"github.com/LerianStudio/midaz/pkg"

	"github.com/stretchr/testify/assert"
)

// TestCreateTransactionSuccess is responsible to test CreateTransaction with success
func TestCreateTransactionSuccess(t *testing.T) {
	tran := &transaction.Transaction{
		ID:             pkg.GenerateUUIDv7().String(),
		OrganizationID: pkg.GenerateUUIDv7().String(),
		LedgerID:       pkg.GenerateUUIDv7().String(),
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

	ID := pkg.GenerateUUIDv7().String()
	tran := &transaction.Transaction{
		ID:                  ID,
		OrganizationID:      pkg.GenerateUUIDv7().String(),
		LedgerID:            pkg.GenerateUUIDv7().String(),
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
