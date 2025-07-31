package query

import (
	"context"
	"errors"
	libHTTP "github.com/LerianStudio/lib-commons/commons/net/http"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/postgres/balance"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/LerianStudio/midaz/v3/pkg/net/http"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
	"testing"
	"time"
)

func TestGetAllBalances(t *testing.T) {
	organizationID := uuid.New()
	ledgerID := uuid.New()
	accountID := uuid.New()

	filter := http.QueryHeader{
		Limit:        10,
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

func TestGetAllBalancesByAlias(t *testing.T) {
	organizationID := uuid.New()
	ledgerID := uuid.New()
	alias := "test-alias"

	t.Parallel()

	t.Run("GetAllBalancesByAlias_success", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockBalanceRepo := balance.NewMockRepository(ctrl)

		uc := &UseCase{
			BalanceRepo: mockBalanceRepo,
		}

		balances := []*mmodel.Balance{
			{
				ID:        "account-id-1",
				AccountID: "account-id-1",
				Alias:     alias,
				AssetCode: "BRL",
				Available: decimal.NewFromInt(1000),
				OnHold:    decimal.NewFromInt(0),
			},
		}

		mockBalanceRepo.
			EXPECT().
			ListByAliases(gomock.Any(), organizationID, ledgerID, []string{alias}).
			Return(balances, nil).
			Times(1)

		res, err := uc.GetAllBalancesByAlias(context.TODO(), organizationID, ledgerID, alias)

		assert.NoError(t, err)
		assert.Len(t, res, 1)
	})

	t.Run("GetAllBalancesByAlias_error", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockBalanceRepo := balance.NewMockRepository(ctrl)

		uc := &UseCase{
			BalanceRepo: mockBalanceRepo,
		}

		errMsg := "error getting balances"

		mockBalanceRepo.
			EXPECT().
			ListByAliases(gomock.Any(), organizationID, ledgerID, []string{alias}).
			Return(nil, errors.New(errMsg)).
			Times(1)

		res, err := uc.GetAllBalancesByAlias(context.TODO(), organizationID, ledgerID, alias)

		assert.Error(t, err)
		assert.Equal(t, errMsg, err.Error())
		assert.Nil(t, res)
	})
}
