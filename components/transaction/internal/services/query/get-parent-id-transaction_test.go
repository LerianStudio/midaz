package query

import (
	"context"
	"errors"
	libCommons "github.com/LerianStudio/lib-commons/commons"
	"github.com/LerianStudio/midaz/components/transaction/internal/adapters/postgres/transaction"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
	"testing"
)

func TestGetParentByTransactionID(t *testing.T) {
	ID := libCommons.GenerateUUIDv7()
	organizationID := libCommons.GenerateUUIDv7()
	ledgerID := libCommons.GenerateUUIDv7()
	parentID := ID.String()

	tran := &transaction.Transaction{
		ParentTransactionID: &parentID,
		OrganizationID:      organizationID.String(),
		LedgerID:            ledgerID.String(),
	}

	uc := UseCase{
		TransactionRepo: transaction.NewMockRepository(gomock.NewController(t)),
	}

	uc.TransactionRepo.(*transaction.MockRepository).
		EXPECT().
		FindByParentID(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return(tran, nil).
		Times(1)
	res, err := uc.TransactionRepo.FindByParentID(context.TODO(), organizationID, ledgerID, ID)

	assert.Equal(t, tran, res)
	assert.Nil(t, err)
}

func TestGetParentByTransactionIDError(t *testing.T) {
	errMSG := "err to create account on database"
	ID := libCommons.GenerateUUIDv7()
	organizationID := libCommons.GenerateUUIDv7()
	ledgerID := libCommons.GenerateUUIDv7()

	uc := UseCase{
		TransactionRepo: transaction.NewMockRepository(gomock.NewController(t)),
	}

	uc.TransactionRepo.(*transaction.MockRepository).
		EXPECT().
		FindByParentID(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return(nil, errors.New(errMSG)).
		Times(1)
	res, err := uc.TransactionRepo.FindByParentID(context.TODO(), organizationID, ledgerID, ID)

	assert.NotEmpty(t, err)
	assert.Equal(t, err.Error(), errMSG)
	assert.Nil(t, res)
}
