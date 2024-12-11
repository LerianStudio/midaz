package query

import (
	"context"
	"errors"
	"github.com/LerianStudio/midaz/pkg/net/http"
	"go.uber.org/mock/gomock"
	"testing"
	"time"

	"github.com/LerianStudio/midaz/components/ledger/internal/adapters/postgres/account"
	"github.com/LerianStudio/midaz/pkg"
	"github.com/LerianStudio/midaz/pkg/mmodel"

	"github.com/stretchr/testify/assert"
)

// TestGetAllAccounts is responsible to test GetAllAccounts with success and error
func TestGetAllAccounts(t *testing.T) {
	organizationID := pkg.GenerateUUIDv7()
	ledgerID := pkg.GenerateUUIDv7()
	portfolioID := pkg.GenerateUUIDv7()
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
	mockAccountRepo := account.NewMockRepository(ctrl)

	uc := UseCase{
		AccountRepo: mockAccountRepo,
	}

	t.Run("Success", func(t *testing.T) {
		accounts := []*mmodel.Account{{}}
		mockAccountRepo.
			EXPECT().
			FindAll(gomock.Any(), organizationID, ledgerID, &portfolioID, filter.ToOffsetPagination()).
			Return(accounts, nil).
			Times(1)
		res, err := uc.AccountRepo.FindAll(context.TODO(), organizationID, ledgerID, &portfolioID, filter.ToOffsetPagination())

		assert.NoError(t, err)
		assert.Len(t, res, 1)
	})

	t.Run("Error", func(t *testing.T) {
		errMsg := "errDatabaseItemNotFound"
		mockAccountRepo.
			EXPECT().
			FindAll(gomock.Any(), organizationID, ledgerID, &portfolioID, filter.ToOffsetPagination()).
			Return(nil, errors.New(errMsg)).
			Times(1)
		res, err := uc.AccountRepo.FindAll(context.TODO(), organizationID, ledgerID, &portfolioID, filter.ToOffsetPagination())

		assert.EqualError(t, err, errMsg)
		assert.Nil(t, res)
	})
}

// TestGetAllAccountsWithoutPortfolio is responsible to test GetAllAccounts without portfolio with success and error
func TestGetAllAccountsWithoutPortfolio(t *testing.T) {
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
	mockAccountRepo := account.NewMockRepository(ctrl)

	uc := UseCase{
		AccountRepo: mockAccountRepo,
	}

	t.Run("Success", func(t *testing.T) {
		accounts := []*mmodel.Account{{}}
		mockAccountRepo.
			EXPECT().
			FindAll(gomock.Any(), organizationID, ledgerID, nil, filter.ToOffsetPagination()).
			Return(accounts, nil).
			Times(1)
		res, err := uc.AccountRepo.FindAll(context.TODO(), organizationID, ledgerID, nil, filter.ToOffsetPagination())

		assert.NoError(t, err)
		assert.Len(t, res, 1)
	})

	t.Run("Error", func(t *testing.T) {
		errMsg := "errDatabaseItemNotFound"
		mockAccountRepo.
			EXPECT().
			FindAll(gomock.Any(), organizationID, ledgerID, nil, filter.ToOffsetPagination()).
			Return(nil, errors.New(errMsg)).
			Times(1)
		res, err := uc.AccountRepo.FindAll(context.TODO(), organizationID, ledgerID, nil, filter.ToOffsetPagination())

		assert.EqualError(t, err, errMsg)
		assert.Nil(t, res)
	})
}
