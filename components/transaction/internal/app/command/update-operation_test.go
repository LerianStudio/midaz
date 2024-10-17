package command

import (
	"context"
	"errors"
	"testing"
	"time"

	o "github.com/LerianStudio/midaz/components/transaction/internal/domain/operation"
	mock "github.com/LerianStudio/midaz/components/transaction/internal/gen/mock/operation"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
)

// TestUpdateOperationSuccess is responsible to test UpdateOperationSuccess with success
func TestUpdateOperationSuccess(t *testing.T) {
	organizationID := uuid.New()
	ledgerID := uuid.New()
	transactionID := uuid.New()
	operationID := uuid.New()

	operation := &o.Operation{
		ID:             operationID.String(),
		LedgerID:       ledgerID.String(),
		OrganizationID: organizationID.String(),
		TransactionID:  transactionID.String(),
		UpdatedAt:      time.Now(),
	}

	uc := UseCase{
		OperationRepo: mock.NewMockRepository(gomock.NewController(t)),
	}

	uc.OperationRepo.(*mock.MockRepository).
		EXPECT().
		Update(gomock.Any(), organizationID, ledgerID, transactionID, operationID, operation).
		Return(operation, nil).
		Times(1)
	res, err := uc.OperationRepo.Update(context.TODO(), organizationID, ledgerID, transactionID, operationID, operation)

	assert.Equal(t, operation, res)
	assert.Nil(t, err)
}

// TestUpdateOperationError is responsible to test UpdateOperationError with error
func TestUpdateOperationError(t *testing.T) {
	errMSG := "errDatabaseItemNotFound"
	organizationID := uuid.New()
	ledgerID := uuid.New()
	transactionID := uuid.New()
	operationID := uuid.New()

	operation := &o.Operation{
		ID:             operationID.String(),
		LedgerID:       ledgerID.String(),
		OrganizationID: organizationID.String(),
		TransactionID:  transactionID.String(),
		UpdatedAt:      time.Now(),
	}

	uc := UseCase{
		OperationRepo: mock.NewMockRepository(gomock.NewController(t)),
	}

	uc.OperationRepo.(*mock.MockRepository).
		EXPECT().
		Update(gomock.Any(), organizationID, ledgerID, transactionID, operationID, operation).
		Return(nil, errors.New(errMSG))
	res, err := uc.OperationRepo.Update(context.TODO(), organizationID, ledgerID, transactionID, operationID, operation)

	assert.NotEmpty(t, err)
	assert.Equal(t, err.Error(), errMSG)
	assert.Nil(t, res)
}
