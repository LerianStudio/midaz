package query

import (
	"context"
	"errors"
	"github.com/LerianStudio/midaz/pkg/net/http"
	"go.uber.org/mock/gomock"
	"testing"
	"time"

	"github.com/LerianStudio/midaz/components/transaction/internal/adapters/postgres/transaction"
	"github.com/LerianStudio/midaz/pkg"

	"github.com/stretchr/testify/assert"
)

// TestGetAllTransactions is responsible to test GetAllTransactions with success and error
func TestGetAllTransactions(t *testing.T) {
	organizationID := pkg.GenerateUUIDv7()
	ledgerID := pkg.GenerateUUIDv7()
	filter := http.QueryHeader{
		Limit:        10,
		Page:         1,
		SortOrder:    "asc",
		StartDate:    time.Now().AddDate(0, -1, 0),
		EndDate:      time.Now(),
		ToAssetCodes: []string{"BRL"},
	}

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
			FindAll(gomock.Any(), organizationID, ledgerID, filter.ToPagination()).
			Return(trans, nil).
			Times(1)
		res, err := uc.TransactionRepo.FindAll(context.TODO(), organizationID, ledgerID, filter.ToPagination())

		assert.NoError(t, err)
		assert.Len(t, res, 1)
	})

	t.Run("Error", func(t *testing.T) {
		errMsg := "errDatabaseItemNotFound"
		mockTransactionRepo.
			EXPECT().
			FindAll(gomock.Any(), organizationID, ledgerID, filter.ToPagination()).
			Return(nil, errors.New(errMsg)).
			Times(1)
		res, err := uc.TransactionRepo.FindAll(context.TODO(), organizationID, ledgerID, filter.ToPagination())

		assert.EqualError(t, err, errMsg)
		assert.Nil(t, res)
	})
}
