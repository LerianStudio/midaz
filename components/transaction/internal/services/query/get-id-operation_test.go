package query

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/postgres/operation"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
)

func TestGetOperationByID(t *testing.T) {
	ID := libCommons.GenerateUUIDv7()
	organizationID := libCommons.GenerateUUIDv7()
	ledgerID := libCommons.GenerateUUIDv7()
	transactionID := libCommons.GenerateUUIDv7()

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
	ID := libCommons.GenerateUUIDv7()
	organizationID := libCommons.GenerateUUIDv7()
	ledgerID := libCommons.GenerateUUIDv7()
	transactionID := libCommons.GenerateUUIDv7()

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

// Precondition tests for GetOperationByID

func TestGetOperationByID_NilOrganizationID_Panics(t *testing.T) {
	uc := &UseCase{}

	defer func() {
		r := recover()
		assert.NotNil(t, r, "expected panic on nil organizationID")
		panicMsg := fmt.Sprintf("%v", r)
		assert.True(t, strings.Contains(panicMsg, "organizationID must not be nil UUID"),
			"panic message should mention organizationID, got: %s", panicMsg)
	}()

	ctx := context.Background()
	_, _ = uc.GetOperationByID(ctx, uuid.Nil, uuid.New(), uuid.New(), uuid.New())
}

func TestGetOperationByID_NilLedgerID_Panics(t *testing.T) {
	uc := &UseCase{}

	defer func() {
		r := recover()
		assert.NotNil(t, r, "expected panic on nil ledgerID")
		panicMsg := fmt.Sprintf("%v", r)
		assert.True(t, strings.Contains(panicMsg, "ledgerID must not be nil UUID"),
			"panic message should mention ledgerID, got: %s", panicMsg)
	}()

	ctx := context.Background()
	_, _ = uc.GetOperationByID(ctx, uuid.New(), uuid.Nil, uuid.New(), uuid.New())
}

func TestGetOperationByID_NilTransactionID_Panics(t *testing.T) {
	uc := &UseCase{}

	defer func() {
		r := recover()
		assert.NotNil(t, r, "expected panic on nil transactionID")
		panicMsg := fmt.Sprintf("%v", r)
		assert.True(t, strings.Contains(panicMsg, "transactionID must not be nil UUID"),
			"panic message should mention transactionID, got: %s", panicMsg)
	}()

	ctx := context.Background()
	_, _ = uc.GetOperationByID(ctx, uuid.New(), uuid.New(), uuid.Nil, uuid.New())
}

func TestGetOperationByID_NilOperationID_Panics(t *testing.T) {
	uc := &UseCase{}

	defer func() {
		r := recover()
		assert.NotNil(t, r, "expected panic on nil operationID")
		panicMsg := fmt.Sprintf("%v", r)
		assert.True(t, strings.Contains(panicMsg, "operationID must not be nil UUID"),
			"panic message should mention operationID, got: %s", panicMsg)
	}()

	ctx := context.Background()
	_, _ = uc.GetOperationByID(ctx, uuid.New(), uuid.New(), uuid.New(), uuid.Nil)
}
