package query

import (
	"context"
	"errors"
	"testing"

	tx "github.com/LerianStudio/midaz/components/transaction/internal/domain/transaction"
	mock "github.com/LerianStudio/midaz/components/transaction/internal/gen/mock/transaction"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
)

// TestGetAllTransactions is responsible to test GetAllTransactions with success and error
func TestGetAllTransactions(t *testing.T) {
	organizationID := uuid.New()
	ledgerID := uuid.New()
	limit := 10
	page := 1

	t.Parallel()
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockTransactionRepo := mock.NewMockRepository(ctrl)

	uc := UseCase{
		TransactionRepo: mockTransactionRepo,
	}

	t.Run("Success", func(t *testing.T) {
		trans := []*tx.Transaction{{}}
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
