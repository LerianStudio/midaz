package query

import (
	"context"
	"errors"
	libCommons "github.com/LerianStudio/lib-commons/commons"
	libHTTP "github.com/LerianStudio/lib-commons/commons/net/http"
	"github.com/LerianStudio/midaz/components/transaction/internal/adapters/postgres/balance"
	"github.com/LerianStudio/midaz/pkg/mmodel"
	"github.com/LerianStudio/midaz/pkg/net/http"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
	"testing"
	"time"
)

// TestGetAllBalances is responsible to test GetAllBalances with success and error
func TestGetAllBalances(t *testing.T) {
	organizationID := libCommons.GenerateUUIDv7()
	ledgerID := libCommons.GenerateUUIDv7()
	accountID := libCommons.GenerateUUIDv7()

	filter := http.QueryHeader{
		Limit:        10,
		Page:         1,
		SortOrder:    "asc",
		StartDate:    time.Now().AddDate(0, -1, 0),
		EndDate:      time.Now(),
		ToAssetCodes: []string{"BRL"},
	}
	mockCur := libHTTP.CursorPagination{
		Next: "next",
		Prev: "prev",
	}

	t.Parallel()
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockBalanceRepo := balance.NewMockRepository(ctrl)

	uc := UseCase{
		BalanceRepo: mockBalanceRepo,
	}

	t.Run("Success", func(t *testing.T) {
		trans := []*mmodel.Balance{{}}
		mockBalanceRepo.
			EXPECT().
			ListAllByAccountID(gomock.Any(), organizationID, ledgerID, accountID, filter.ToCursorPagination()).
			Return(trans, mockCur, nil).
			Times(1)
		res, cur, err := uc.BalanceRepo.ListAllByAccountID(context.TODO(), organizationID, ledgerID, accountID, filter.ToCursorPagination())

		assert.NoError(t, err)
		assert.Len(t, res, 1)
		assert.NotNil(t, cur)
	})

	t.Run("Error", func(t *testing.T) {
		errMsg := "errDatabaseItemNotFound"
		mockBalanceRepo.
			EXPECT().
			ListAllByAccountID(gomock.Any(), organizationID, ledgerID, accountID, filter.ToCursorPagination()).
			Return(nil, libHTTP.CursorPagination{}, errors.New(errMsg)).
			Times(1)
		res, cur, err := uc.BalanceRepo.ListAllByAccountID(context.TODO(), organizationID, ledgerID, accountID, filter.ToCursorPagination())

		assert.EqualError(t, err, errMsg)
		assert.Nil(t, res)
		assert.Equal(t, cur, libHTTP.CursorPagination{})
	})
}
