package command

import (
	"context"
	"errors"
	"go.uber.org/mock/gomock"
	"testing"
	"time"

	"github.com/LerianStudio/midaz/components/transaction/internal/adapters/postgres/operation"
	"github.com/LerianStudio/midaz/pkg"

	"github.com/stretchr/testify/assert"
)

// TestUpdateOperationSuccess is responsible to test UpdateOperationSuccess with success
func TestUpdateOperationSuccess(t *testing.T) {
	organizationID := pkg.GenerateUUIDv7()
	ledgerID := pkg.GenerateUUIDv7()
	transactionID := pkg.GenerateUUIDv7()
	operationID := pkg.GenerateUUIDv7()

	o := &operation.Operation{
		ID:             operationID.String(),
		LedgerID:       ledgerID.String(),
		OrganizationID: organizationID.String(),
		TransactionID:  transactionID.String(),
		UpdatedAt:      time.Now(),
	}

	uc := UseCase{
		OperationRepo: operation.NewMockRepository(gomock.NewController(t)),
	}

	uc.OperationRepo.(*operation.MockRepository).
		EXPECT().
		Update(gomock.Any(), organizationID, ledgerID, transactionID, operationID, o).
		Return(o, nil).
		Times(1)
	res, err := uc.OperationRepo.Update(context.TODO(), organizationID, ledgerID, transactionID, operationID, o)

	assert.Equal(t, o, res)
	assert.Nil(t, err)
}

// TestUpdateOperationError is responsible to test UpdateOperationError with error
func TestUpdateOperationError(t *testing.T) {
	errMSG := "errDatabaseItemNotFound"
	organizationID := pkg.GenerateUUIDv7()
	ledgerID := pkg.GenerateUUIDv7()
	transactionID := pkg.GenerateUUIDv7()
	operationID := pkg.GenerateUUIDv7()

	o := &operation.Operation{
		ID:             operationID.String(),
		LedgerID:       ledgerID.String(),
		OrganizationID: organizationID.String(),
		TransactionID:  transactionID.String(),
		UpdatedAt:      time.Now(),
	}

	uc := UseCase{
		OperationRepo: operation.NewMockRepository(gomock.NewController(t)),
	}

	uc.OperationRepo.(*operation.MockRepository).
		EXPECT().
		Update(gomock.Any(), organizationID, ledgerID, transactionID, operationID, o).
		Return(nil, errors.New(errMSG))
	res, err := uc.OperationRepo.Update(context.TODO(), organizationID, ledgerID, transactionID, operationID, o)

	assert.NotEmpty(t, err)
	assert.Equal(t, err.Error(), errMSG)
	assert.Nil(t, res)
}
