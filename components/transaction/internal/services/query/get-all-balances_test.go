package query

import (
	"context"
	"errors"
	"github.com/LerianStudio/midaz/components/transaction/internal/adapters/postgres/balance"
	"github.com/LerianStudio/midaz/pkg/mmodel"
	"github.com/LerianStudio/midaz/pkg/net/http"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
	"testing"
	"time"
)

// TestGetAllBalances is responsible to test GetAllBalances with success and error
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
	mockCur := http.Pagination{
		Page: 0,
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
			Return(nil, http.Pagination{}, errors.New(errMsg)).
			Times(1)
		res, cur, err := uc.BalanceRepo.ListAllByAccountID(context.TODO(), organizationID, ledgerID, accountID, filter.ToCursorPagination())

		assert.EqualError(t, err, errMsg)
		assert.Nil(t, res)
		assert.Equal(t, cur, http.Pagination{})
	})

	t.Run("GetAllBalancesByAccountID_success", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockBalanceRepo := balance.NewMockRepository(ctrl)

		uc := &UseCase{
			BalanceRepo: mockBalanceRepo,
		}

		balances := []*mmodel.Balance{
			{
				ID:             "id-1",
				AccountID:      accountID.String(),
				OrganizationID: organizationID.String(),
				LedgerID:       ledgerID.String(),
				Alias:          "alias-1",
				AssetCode:      "BRL",
				Available:      decimal.NewFromInt(1000),
				OnHold:         decimal.NewFromInt(0),
				CreatedAt:      time.Now(),
				UpdatedAt:      time.Now(),
			},
		}

		mockBalanceRepo.
			EXPECT().
			ListAllByAccountID(gomock.Any(), organizationID, ledgerID, accountID, filter.ToCursorPagination()).
			Return(balances, mockCur, nil).
			Times(1)

		res, cur, err := uc.GetAllBalancesByAccountID(context.TODO(), organizationID, ledgerID, accountID, filter)

		assert.NoError(t, err)
		assert.Len(t, res, 1)
		assert.NotNil(t, cur)
	})
}

// TestGetAllBalancesByAlias is responsible to test GetAllBalancesByAlias with success and error
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
