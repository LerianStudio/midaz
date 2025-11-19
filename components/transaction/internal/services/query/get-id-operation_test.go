package query

import (
	"context"
	"errors"
	"testing"

	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/postgres/operation"
	"github.com/LerianStudio/midaz/v3/pkg/utils"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
)

func TestGetOperationByID(t *testing.T) {
	ID := utils.GenerateUUIDv7()
	organizationID := utils.GenerateUUIDv7()
	ledgerID := utils.GenerateUUIDv7()
	transactionID := utils.GenerateUUIDv7()

	o := &operation.Operation{
		ID:             ID.String(),
		OrganizationID: organizationID.String(),
		LedgerID:       ledgerID.String(),
		TransactionID:  transactionID.String(),
	}

	uc := UseCase{
		OperationRepo: operation.NewMockRepository(gomock.NewController(t)),
	}

	uc.OperationRepo.(*operation.MockRepository).
		EXPECT().
		Find(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return(o, nil).
		Times(1)
	res, err := uc.OperationRepo.Find(context.TODO(), organizationID, ledgerID, transactionID, ID)

	assert.Equal(t, o, res)
	assert.Nil(t, err)
}

func TestGetOperationByIDError(t *testing.T) {
	errMSG := "err to get operation on database"
	ID := utils.GenerateUUIDv7()
	organizationID := utils.GenerateUUIDv7()
	ledgerID := utils.GenerateUUIDv7()
	transactionID := utils.GenerateUUIDv7()

	uc := UseCase{
		OperationRepo: operation.NewMockRepository(gomock.NewController(t)),
	}

	uc.OperationRepo.(*operation.MockRepository).
		EXPECT().
		Find(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return(nil, errors.New(errMSG)).
		Times(1)
	res, err := uc.OperationRepo.Find(context.TODO(), organizationID, ledgerID, transactionID, ID)

	assert.NotEmpty(t, err)
	assert.Equal(t, err.Error(), errMSG)
	assert.Nil(t, res)
}
