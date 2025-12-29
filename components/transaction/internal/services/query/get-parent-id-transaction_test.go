package query

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/postgres/transaction"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
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

func TestGetParentByTransactionID_NilOrganizationID_Panics(t *testing.T) {
	uc := &UseCase{}

	defer func() {
		r := recover()
		assert.NotNil(t, r, "expected panic on nil organizationID")
		panicMsg := fmt.Sprintf("%v", r)
		assert.True(t, strings.Contains(panicMsg, "organizationID must not be nil UUID"),
			"panic message should mention organizationID, got: %s", panicMsg)
	}()

	ctx := context.Background()
	_, _ = uc.GetParentByTransactionID(ctx, uuid.Nil, uuid.New(), uuid.New())
}

func TestGetParentByTransactionID_NilLedgerID_Panics(t *testing.T) {
	uc := &UseCase{}

	defer func() {
		r := recover()
		assert.NotNil(t, r, "expected panic on nil ledgerID")
		panicMsg := fmt.Sprintf("%v", r)
		assert.True(t, strings.Contains(panicMsg, "ledgerID must not be nil UUID"),
			"panic message should mention ledgerID, got: %s", panicMsg)
	}()

	ctx := context.Background()
	_, _ = uc.GetParentByTransactionID(ctx, uuid.New(), uuid.Nil, uuid.New())
}

func TestGetParentByTransactionID_NilParentID_Panics(t *testing.T) {
	uc := &UseCase{}

	defer func() {
		r := recover()
		assert.NotNil(t, r, "expected panic on nil parentID")
		panicMsg := fmt.Sprintf("%v", r)
		assert.True(t, strings.Contains(panicMsg, "parentID must not be nil UUID"),
			"panic message should mention parentID, got: %s", panicMsg)
	}()

	ctx := context.Background()
	_, _ = uc.GetParentByTransactionID(ctx, uuid.New(), uuid.New(), uuid.Nil)
}
