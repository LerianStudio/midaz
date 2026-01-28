package query

import (
	"context"
	"errors"
	"testing"
	"time"

	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	libHTTP "github.com/LerianStudio/lib-commons/v2/commons/net/http"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/postgres/balance"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/postgres/operation"
	"github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/LerianStudio/midaz/v3/pkg/net/http"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

func TestGetAccountBalancesAtTimestamp(t *testing.T) {
	t.Parallel()

	t.Run("Success returns balances with correct CreatedAt from currentBalance", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		orgID := libCommons.GenerateUUIDv7()
		ledgerID := libCommons.GenerateUUIDv7()
		accountID := libCommons.GenerateUUIDv7()
		balanceID := libCommons.GenerateUUIDv7()
		timestamp := time.Now().Add(-time.Hour)
		balanceCreatedAt := time.Now().Add(-24 * time.Hour)
		operationCreatedAt := timestamp.Add(-30 * time.Minute)

		available := decimal.NewFromInt(5000)
		onHold := decimal.NewFromInt(500)
		version := int64(10)

		operations := []*operation.Operation{
			{
				ID:         libCommons.GenerateUUIDv7().String(),
				AccountID:  accountID.String(),
				BalanceID:  balanceID.String(),
				BalanceKey: "default",
				AssetCode:  "USD",
				BalanceAfter: operation.Balance{
					Available: &available,
					OnHold:    &onHold,
					Version:   &version,
				},
				CreatedAt: operationCreatedAt,
			},
		}

		currentBalances := []*mmodel.Balance{
			{
				ID:             balanceID.String(),
				OrganizationID: orgID.String(),
				LedgerID:       ledgerID.String(),
				AccountID:      accountID.String(),
				Alias:          "@user1",
				Key:            "default",
				AssetCode:      "USD",
				AccountType:    "deposit",
				CreatedAt:      balanceCreatedAt,
			},
		}

		balanceRepo := balance.NewMockRepository(ctrl)
		operationRepo := operation.NewMockRepository(ctrl)

		uc := UseCase{BalanceRepo: balanceRepo, OperationRepo: operationRepo}

		filter := http.Pagination{Limit: 10}
		cursor := libHTTP.CursorPagination{Next: ""}

		operationRepo.EXPECT().
			FindLastOperationsForAccountBeforeTimestamp(gomock.Any(), orgID, ledgerID, accountID, timestamp, filter).
			Return(operations, cursor, nil)

		balanceRepo.EXPECT().
			ListByAccountID(gomock.Any(), orgID, ledgerID, accountID).
			Return(currentBalances, nil)

		results, resultCursor, err := uc.GetAccountBalancesAtTimestamp(context.Background(), orgID, ledgerID, accountID, timestamp, filter)

		require.NoError(t, err)
		require.Len(t, results, 1)
		assert.Equal(t, cursor, resultCursor)

		result := results[0]
		assert.Equal(t, balanceID.String(), result.ID)
		assert.Equal(t, available, result.Available)
		assert.Equal(t, onHold, result.OnHold)
		assert.Equal(t, version, result.Version)
		// Key assertion: CreatedAt should come from currentBalance, not operation
		assert.Equal(t, balanceCreatedAt, result.CreatedAt, "CreatedAt should match currentBalance.CreatedAt")
		// UpdatedAt should be the operation timestamp
		assert.Equal(t, operationCreatedAt, result.UpdatedAt, "UpdatedAt should match operation.CreatedAt")
	})

	t.Run("Future timestamp returns error", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		orgID := libCommons.GenerateUUIDv7()
		ledgerID := libCommons.GenerateUUIDv7()
		accountID := libCommons.GenerateUUIDv7()
		futureTimestamp := time.Now().Add(time.Hour)

		balanceRepo := balance.NewMockRepository(ctrl)
		operationRepo := operation.NewMockRepository(ctrl)

		uc := UseCase{BalanceRepo: balanceRepo, OperationRepo: operationRepo}

		filter := http.Pagination{Limit: 10}

		results, _, err := uc.GetAccountBalancesAtTimestamp(context.Background(), orgID, ledgerID, accountID, futureTimestamp, filter)

		assert.Error(t, err)
		assert.Nil(t, results)
		assert.Contains(t, err.Error(), constant.ErrInvalidTimestamp.Error())
	})

	t.Run("No operations but balance created before timestamp returns zero balance", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		orgID := libCommons.GenerateUUIDv7()
		ledgerID := libCommons.GenerateUUIDv7()
		accountID := libCommons.GenerateUUIDv7()
		balanceID := libCommons.GenerateUUIDv7()
		timestamp := time.Now().Add(-time.Hour)
		// Balance was created before the timestamp
		balanceCreatedAt := time.Now().Add(-24 * time.Hour)

		balanceRepo := balance.NewMockRepository(ctrl)
		operationRepo := operation.NewMockRepository(ctrl)

		uc := UseCase{BalanceRepo: balanceRepo, OperationRepo: operationRepo}

		filter := http.Pagination{Limit: 10}
		cursor := libHTTP.CursorPagination{}

		// Return balances that exist but have no operations
		allBalances := []*mmodel.Balance{
			{
				ID:             balanceID.String(),
				OrganizationID: orgID.String(),
				LedgerID:       ledgerID.String(),
				AccountID:      accountID.String(),
				Alias:          "@user1",
				Key:            "default",
				AssetCode:      "USD",
				AccountType:    "deposit",
				CreatedAt:      balanceCreatedAt,
			},
		}

		operationRepo.EXPECT().
			FindLastOperationsForAccountBeforeTimestamp(gomock.Any(), orgID, ledgerID, accountID, timestamp, filter).
			Return([]*operation.Operation{}, cursor, nil)

		balanceRepo.EXPECT().
			ListByAccountID(gomock.Any(), orgID, ledgerID, accountID).
			Return(allBalances, nil)

		results, _, err := uc.GetAccountBalancesAtTimestamp(context.Background(), orgID, ledgerID, accountID, timestamp, filter)

		require.NoError(t, err)
		require.Len(t, results, 1)

		result := results[0]
		assert.Equal(t, balanceID.String(), result.ID)
		assert.True(t, result.Available.Equal(decimal.Zero), "Available should be zero for balance without operations")
		assert.True(t, result.OnHold.Equal(decimal.Zero), "OnHold should be zero for balance without operations")
		assert.Equal(t, int64(0), result.Version, "Version should be zero for balance without operations")
		assert.Equal(t, balanceCreatedAt, result.CreatedAt)
		assert.Equal(t, balanceCreatedAt, result.UpdatedAt, "UpdatedAt should equal CreatedAt for initial state")
	})

	t.Run("No operations and no balances at timestamp returns error", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		orgID := libCommons.GenerateUUIDv7()
		ledgerID := libCommons.GenerateUUIDv7()
		accountID := libCommons.GenerateUUIDv7()
		timestamp := time.Now().Add(-24 * time.Hour)

		balanceRepo := balance.NewMockRepository(ctrl)
		operationRepo := operation.NewMockRepository(ctrl)

		uc := UseCase{BalanceRepo: balanceRepo, OperationRepo: operationRepo}

		filter := http.Pagination{Limit: 10}
		cursor := libHTTP.CursorPagination{}

		operationRepo.EXPECT().
			FindLastOperationsForAccountBeforeTimestamp(gomock.Any(), orgID, ledgerID, accountID, timestamp, filter).
			Return([]*operation.Operation{}, cursor, nil)

		// Return empty list - no balances exist
		balanceRepo.EXPECT().
			ListByAccountID(gomock.Any(), orgID, ledgerID, accountID).
			Return([]*mmodel.Balance{}, nil)

		results, _, err := uc.GetAccountBalancesAtTimestamp(context.Background(), orgID, ledgerID, accountID, timestamp, filter)

		assert.Error(t, err)
		assert.Nil(t, results)
		// Error message contains info about no balance data at timestamp
		assert.Contains(t, err.Error(), "balance data")
	})

	t.Run("Balance created after timestamp is excluded", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		orgID := libCommons.GenerateUUIDv7()
		ledgerID := libCommons.GenerateUUIDv7()
		accountID := libCommons.GenerateUUIDv7()
		balanceID := libCommons.GenerateUUIDv7()
		timestamp := time.Now().Add(-24 * time.Hour)
		// Balance was created AFTER the timestamp
		balanceCreatedAt := time.Now().Add(-time.Hour)

		balanceRepo := balance.NewMockRepository(ctrl)
		operationRepo := operation.NewMockRepository(ctrl)

		uc := UseCase{BalanceRepo: balanceRepo, OperationRepo: operationRepo}

		filter := http.Pagination{Limit: 10}
		cursor := libHTTP.CursorPagination{}

		// Balance exists but was created after timestamp
		allBalances := []*mmodel.Balance{
			{
				ID:             balanceID.String(),
				OrganizationID: orgID.String(),
				LedgerID:       ledgerID.String(),
				AccountID:      accountID.String(),
				CreatedAt:      balanceCreatedAt,
			},
		}

		operationRepo.EXPECT().
			FindLastOperationsForAccountBeforeTimestamp(gomock.Any(), orgID, ledgerID, accountID, timestamp, filter).
			Return([]*operation.Operation{}, cursor, nil)

		balanceRepo.EXPECT().
			ListByAccountID(gomock.Any(), orgID, ledgerID, accountID).
			Return(allBalances, nil)

		results, _, err := uc.GetAccountBalancesAtTimestamp(context.Background(), orgID, ledgerID, accountID, timestamp, filter)

		assert.Error(t, err)
		assert.Nil(t, results)
		// Error message contains info about no balance data at timestamp
		assert.Contains(t, err.Error(), "balance data")
	})

	t.Run("Mixed balances with and without operations", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		orgID := libCommons.GenerateUUIDv7()
		ledgerID := libCommons.GenerateUUIDv7()
		accountID := libCommons.GenerateUUIDv7()
		balanceID1 := libCommons.GenerateUUIDv7()
		balanceID2 := libCommons.GenerateUUIDv7()
		timestamp := time.Now().Add(-time.Hour)
		balance1CreatedAt := time.Now().Add(-48 * time.Hour)
		balance2CreatedAt := time.Now().Add(-24 * time.Hour)
		operationCreatedAt := timestamp.Add(-30 * time.Minute)

		available := decimal.NewFromInt(5000)

		// Only balance1 has operations
		operations := []*operation.Operation{
			{
				ID:         libCommons.GenerateUUIDv7().String(),
				AccountID:  accountID.String(),
				BalanceID:  balanceID1.String(),
				BalanceKey: "default",
				AssetCode:  "USD",
				BalanceAfter: operation.Balance{
					Available: &available,
				},
				CreatedAt: operationCreatedAt,
			},
		}

		// Both balances exist
		allBalances := []*mmodel.Balance{
			{
				ID:             balanceID1.String(),
				OrganizationID: orgID.String(),
				LedgerID:       ledgerID.String(),
				AccountID:      accountID.String(),
				Alias:          "@user1",
				Key:            "default",
				AssetCode:      "USD",
				AccountType:    "deposit",
				CreatedAt:      balance1CreatedAt,
			},
			{
				ID:             balanceID2.String(),
				OrganizationID: orgID.String(),
				LedgerID:       ledgerID.String(),
				AccountID:      accountID.String(),
				Alias:          "@user1",
				Key:            "savings",
				AssetCode:      "USD",
				AccountType:    "deposit",
				CreatedAt:      balance2CreatedAt,
			},
		}

		balanceRepo := balance.NewMockRepository(ctrl)
		operationRepo := operation.NewMockRepository(ctrl)

		uc := UseCase{BalanceRepo: balanceRepo, OperationRepo: operationRepo}

		filter := http.Pagination{Limit: 10}
		cursor := libHTTP.CursorPagination{}

		operationRepo.EXPECT().
			FindLastOperationsForAccountBeforeTimestamp(gomock.Any(), orgID, ledgerID, accountID, timestamp, filter).
			Return(operations, cursor, nil)

		balanceRepo.EXPECT().
			ListByAccountID(gomock.Any(), orgID, ledgerID, accountID).
			Return(allBalances, nil)

		results, _, err := uc.GetAccountBalancesAtTimestamp(context.Background(), orgID, ledgerID, accountID, timestamp, filter)

		require.NoError(t, err)
		require.Len(t, results, 2)

		// Verify balance1 has operation data
		var balance1, balance2 *mmodel.Balance
		for _, r := range results {
			if r.ID == balanceID1.String() {
				balance1 = r
			} else if r.ID == balanceID2.String() {
				balance2 = r
			}
		}

		require.NotNil(t, balance1)
		assert.Equal(t, available, balance1.Available, "Balance1 should have operation data")
		assert.Equal(t, operationCreatedAt, balance1.UpdatedAt, "Balance1 UpdatedAt should be operation time")

		require.NotNil(t, balance2)
		assert.True(t, balance2.Available.Equal(decimal.Zero), "Balance2 should have zero available")
		assert.Equal(t, balance2CreatedAt, balance2.UpdatedAt, "Balance2 UpdatedAt should be CreatedAt")
	})

	t.Run("Operation repo error returns error", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		orgID := libCommons.GenerateUUIDv7()
		ledgerID := libCommons.GenerateUUIDv7()
		accountID := libCommons.GenerateUUIDv7()
		timestamp := time.Now().Add(-time.Hour)

		balanceRepo := balance.NewMockRepository(ctrl)
		operationRepo := operation.NewMockRepository(ctrl)

		uc := UseCase{BalanceRepo: balanceRepo, OperationRepo: operationRepo}

		filter := http.Pagination{Limit: 10}

		operationRepo.EXPECT().
			FindLastOperationsForAccountBeforeTimestamp(gomock.Any(), orgID, ledgerID, accountID, timestamp, filter).
			Return(nil, libHTTP.CursorPagination{}, errors.New("database error"))

		results, _, err := uc.GetAccountBalancesAtTimestamp(context.Background(), orgID, ledgerID, accountID, timestamp, filter)

		assert.Error(t, err)
		assert.Nil(t, results)
	})

	t.Run("Balance repo error returns error", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		orgID := libCommons.GenerateUUIDv7()
		ledgerID := libCommons.GenerateUUIDv7()
		accountID := libCommons.GenerateUUIDv7()
		balanceID := libCommons.GenerateUUIDv7()
		timestamp := time.Now().Add(-time.Hour)

		available := decimal.NewFromInt(5000)

		operations := []*operation.Operation{
			{
				ID:        libCommons.GenerateUUIDv7().String(),
				AccountID: accountID.String(),
				BalanceID: balanceID.String(),
				BalanceAfter: operation.Balance{
					Available: &available,
				},
			},
		}

		balanceRepo := balance.NewMockRepository(ctrl)
		operationRepo := operation.NewMockRepository(ctrl)

		uc := UseCase{BalanceRepo: balanceRepo, OperationRepo: operationRepo}

		filter := http.Pagination{Limit: 10}
		cursor := libHTTP.CursorPagination{}

		operationRepo.EXPECT().
			FindLastOperationsForAccountBeforeTimestamp(gomock.Any(), orgID, ledgerID, accountID, timestamp, filter).
			Return(operations, cursor, nil)

		balanceRepo.EXPECT().
			ListByAccountID(gomock.Any(), orgID, ledgerID, accountID).
			Return(nil, errors.New("database error"))

		results, _, err := uc.GetAccountBalancesAtTimestamp(context.Background(), orgID, ledgerID, accountID, timestamp, filter)

		assert.Error(t, err)
		assert.Nil(t, results)
	})

	t.Run("Deleted balance uses operation data for ID", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		orgID := libCommons.GenerateUUIDv7()
		ledgerID := libCommons.GenerateUUIDv7()
		accountID := libCommons.GenerateUUIDv7()
		balanceID := libCommons.GenerateUUIDv7()
		timestamp := time.Now().Add(-time.Hour)
		operationCreatedAt := timestamp.Add(-30 * time.Minute)

		available := decimal.NewFromInt(5000)
		onHold := decimal.NewFromInt(500)
		version := int64(10)

		operations := []*operation.Operation{
			{
				ID:         libCommons.GenerateUUIDv7().String(),
				AccountID:  accountID.String(),
				BalanceID:  balanceID.String(),
				BalanceKey: "default",
				AssetCode:  "USD",
				BalanceAfter: operation.Balance{
					Available: &available,
					OnHold:    &onHold,
					Version:   &version,
				},
				CreatedAt: operationCreatedAt,
			},
		}

		// Return empty list - balance was deleted
		currentBalances := []*mmodel.Balance{}

		balanceRepo := balance.NewMockRepository(ctrl)
		operationRepo := operation.NewMockRepository(ctrl)

		uc := UseCase{BalanceRepo: balanceRepo, OperationRepo: operationRepo}

		filter := http.Pagination{Limit: 10}
		cursor := libHTTP.CursorPagination{}

		operationRepo.EXPECT().
			FindLastOperationsForAccountBeforeTimestamp(gomock.Any(), orgID, ledgerID, accountID, timestamp, filter).
			Return(operations, cursor, nil)

		balanceRepo.EXPECT().
			ListByAccountID(gomock.Any(), orgID, ledgerID, accountID).
			Return(currentBalances, nil)

		results, _, err := uc.GetAccountBalancesAtTimestamp(context.Background(), orgID, ledgerID, accountID, timestamp, filter)

		require.NoError(t, err)
		require.Len(t, results, 1)

		result := results[0]
		assert.Equal(t, balanceID.String(), result.ID, "ID should come from operation when balance is deleted")
		assert.Equal(t, available, result.Available)
	})

	t.Run("Multiple balances with different CreatedAt values", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		orgID := libCommons.GenerateUUIDv7()
		ledgerID := libCommons.GenerateUUIDv7()
		accountID := libCommons.GenerateUUIDv7()
		balanceID1 := libCommons.GenerateUUIDv7()
		balanceID2 := libCommons.GenerateUUIDv7()
		timestamp := time.Now().Add(-time.Hour)
		balance1CreatedAt := time.Now().Add(-48 * time.Hour)
		balance2CreatedAt := time.Now().Add(-24 * time.Hour)
		operationCreatedAt := timestamp.Add(-30 * time.Minute)

		available1 := decimal.NewFromInt(5000)
		available2 := decimal.NewFromInt(3000)

		operations := []*operation.Operation{
			{
				ID:         libCommons.GenerateUUIDv7().String(),
				AccountID:  accountID.String(),
				BalanceID:  balanceID1.String(),
				BalanceKey: "default",
				AssetCode:  "USD",
				BalanceAfter: operation.Balance{
					Available: &available1,
				},
				CreatedAt: operationCreatedAt,
			},
			{
				ID:         libCommons.GenerateUUIDv7().String(),
				AccountID:  accountID.String(),
				BalanceID:  balanceID2.String(),
				BalanceKey: "savings",
				AssetCode:  "USD",
				BalanceAfter: operation.Balance{
					Available: &available2,
				},
				CreatedAt: operationCreatedAt.Add(-time.Minute),
			},
		}

		currentBalances := []*mmodel.Balance{
			{
				ID:             balanceID1.String(),
				OrganizationID: orgID.String(),
				LedgerID:       ledgerID.String(),
				AccountID:      accountID.String(),
				Key:            "default",
				AssetCode:      "USD",
				CreatedAt:      balance1CreatedAt,
			},
			{
				ID:             balanceID2.String(),
				OrganizationID: orgID.String(),
				LedgerID:       ledgerID.String(),
				AccountID:      accountID.String(),
				Key:            "savings",
				AssetCode:      "USD",
				CreatedAt:      balance2CreatedAt,
			},
		}

		balanceRepo := balance.NewMockRepository(ctrl)
		operationRepo := operation.NewMockRepository(ctrl)

		uc := UseCase{BalanceRepo: balanceRepo, OperationRepo: operationRepo}

		filter := http.Pagination{Limit: 10}
		cursor := libHTTP.CursorPagination{}

		operationRepo.EXPECT().
			FindLastOperationsForAccountBeforeTimestamp(gomock.Any(), orgID, ledgerID, accountID, timestamp, filter).
			Return(operations, cursor, nil)

		balanceRepo.EXPECT().
			ListByAccountID(gomock.Any(), orgID, ledgerID, accountID).
			Return(currentBalances, nil)

		results, _, err := uc.GetAccountBalancesAtTimestamp(context.Background(), orgID, ledgerID, accountID, timestamp, filter)

		require.NoError(t, err)
		require.Len(t, results, 2)

		// Each balance should have its own CreatedAt
		for _, result := range results {
			if result.ID == balanceID1.String() {
				assert.Equal(t, balance1CreatedAt, result.CreatedAt, "Balance 1 CreatedAt should match")
			} else if result.ID == balanceID2.String() {
				assert.Equal(t, balance2CreatedAt, result.CreatedAt, "Balance 2 CreatedAt should match")
			}
		}
	})
}
