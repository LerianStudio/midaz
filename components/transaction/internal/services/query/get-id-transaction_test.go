package query

import (
	"context"
	"errors"
	"testing"

	"github.com/LerianStudio/midaz/common"
	"github.com/LerianStudio/midaz/components/transaction/internal/adapters/database/postgres/transaction"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
)

func TestGetTransactionByID(t *testing.T) {
	ID := common.GenerateUUIDv7()
	organizationID := common.GenerateUUIDv7()
	ledgerID := common.GenerateUUIDv7()

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
	ID := common.GenerateUUIDv7()
	organizationID := common.GenerateUUIDv7()
	ledgerID := common.GenerateUUIDv7()

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
