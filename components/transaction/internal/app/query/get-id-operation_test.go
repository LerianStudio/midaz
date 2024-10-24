package query

import (
	"context"
	"errors"
	"github.com/LerianStudio/midaz/common"
	"testing"

	o "github.com/LerianStudio/midaz/components/transaction/internal/domain/operation"
	mock "github.com/LerianStudio/midaz/components/transaction/internal/gen/mock/operation"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
)

func TestGetOperationByID(t *testing.T) {
	ID := common.GenerateUUIDv7()
	organizationID := common.GenerateUUIDv7()
	ledgerID := common.GenerateUUIDv7()
	transactionID := common.GenerateUUIDv7()

	operation := &o.Operation{
		ID:             ID.String(),
		OrganizationID: organizationID.String(),
		LedgerID:       ledgerID.String(),
		TransactionID:  transactionID.String(),
	}

	uc := UseCase{
		OperationRepo: mock.NewMockRepository(gomock.NewController(t)),
	}

	uc.OperationRepo.(*mock.MockRepository).
		EXPECT().
		Find(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return(operation, nil).
		Times(1)
	res, err := uc.OperationRepo.Find(context.TODO(), organizationID, ledgerID, transactionID, ID)

	assert.Equal(t, operation, res)
	assert.Nil(t, err)
}

func TestGetOperationByIDError(t *testing.T) {
	errMSG := "err to get operation on database"
	ID := common.GenerateUUIDv7()
	organizationID := common.GenerateUUIDv7()
	ledgerID := common.GenerateUUIDv7()
	transactionID := common.GenerateUUIDv7()

	uc := UseCase{
		OperationRepo: mock.NewMockRepository(gomock.NewController(t)),
	}

	uc.OperationRepo.(*mock.MockRepository).
		EXPECT().
		Find(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return(nil, errors.New(errMSG)).
		Times(1)
	res, err := uc.OperationRepo.Find(context.TODO(), organizationID, ledgerID, transactionID, ID)

	assert.NotEmpty(t, err)
	assert.Equal(t, err.Error(), errMSG)
	assert.Nil(t, res)
}
