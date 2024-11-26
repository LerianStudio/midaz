package query

import (
	"context"
	"errors"
	"testing"

	"github.com/LerianStudio/midaz/common"
	"github.com/LerianStudio/midaz/components/transaction/internal/adapters/postgres/transaction"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
)

// TestGetAllTransactions is responsible to test GetAllTransactions with success and error
func TestGetAllTransactions(t *testing.T) {
	organizationID := common.GenerateUUIDv7()
	ledgerID := common.GenerateUUIDv7()
	limit := 10
	page := 1

	t.Parallel()
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockTransactionRepo := transaction.NewMockRepository(ctrl)

	uc := UseCase{
		TransactionRepo: mockTransactionRepo,
	}

	t.Run("Success", func(t *testing.T) {
		trans := []*transaction.Transaction{{}}
		mockTransactionRepo.
			EXPECT().
			FindAll(gomock.Any(), organizationID, ledgerID, limit, page).
			Return(trans, nil).
			Times(1)
		res, err := uc.TransactionRepo.FindAll(context.TODO(), organizationID, ledgerID, limit, page)

		assert.NoError(t, err)
		assert.Len(t, res, 1)
	})

	t.Run("Error", func(t *testing.T) {
		errMsg := "errDatabaseItemNotFound"
		mockTransactionRepo.
			EXPECT().
			FindAll(gomock.Any(), organizationID, ledgerID, limit, page).
			Return(nil, errors.New(errMsg)).
			Times(1)
		res, err := uc.TransactionRepo.FindAll(context.TODO(), organizationID, ledgerID, limit, page)

		assert.EqualError(t, err, errMsg)
		assert.Nil(t, res)
	})
}
