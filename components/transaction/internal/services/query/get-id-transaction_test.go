package query

import (
	"context"
	"errors"
	"go.uber.org/mock/gomock"
	"testing"

	"github.com/LerianStudio/midaz/components/transaction/internal/adapters/postgres/transaction"
	"github.com/LerianStudio/midaz/pkg"

	"github.com/stretchr/testify/assert"
)

func TestGetTransactionByID(t *testing.T) {
	ID := pkg.GenerateUUIDv7()
	organizationID := pkg.GenerateUUIDv7()
	ledgerID := pkg.GenerateUUIDv7()

	tran := &transaction.Transaction{
		ID:             ID.String(),
		OrganizationID: organizationID.String(),
		LedgerID:       ledgerID.String(),
	}

	uc := UseCase{
		TransactionRepo: transaction.NewMockRepository(gomock.NewController(t)),
	}

	uc.TransactionRepo.(*transaction.MockRepository).
		EXPECT().
		Find(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return(tran, nil).
		Times(1)
	res, err := uc.TransactionRepo.Find(context.TODO(), organizationID, ledgerID, ID)

	assert.Equal(t, tran, res)
	assert.Nil(t, err)
}

func TestGetTransactionByIDError(t *testing.T) {
	errMSG := "err to create account on database"
	ID := pkg.GenerateUUIDv7()
	organizationID := pkg.GenerateUUIDv7()
	ledgerID := pkg.GenerateUUIDv7()

	uc := UseCase{
		TransactionRepo: transaction.NewMockRepository(gomock.NewController(t)),
	}

	uc.TransactionRepo.(*transaction.MockRepository).
		EXPECT().
		Find(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return(nil, errors.New(errMSG)).
		Times(1)
	res, err := uc.TransactionRepo.Find(context.TODO(), organizationID, ledgerID, ID)

	assert.NotEmpty(t, err)
	assert.Equal(t, err.Error(), errMSG)
	assert.Nil(t, res)
}
