package query

import (
	"context"
	"errors"
	"testing"
	"time"

	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/postgres/balance"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/postgres/operation"
	"github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

func TestGetAccountBalancesAtTimestamp(t *testing.T) {
	t.Parallel()

	t.Run("Success returns balances at timestamp", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		orgID := libCommons.GenerateUUIDv7()
		ledgerID := libCommons.GenerateUUIDv7()
		accountID := libCommons.GenerateUUIDv7()
		balanceID := libCommons.GenerateUUIDv7()
		timestamp := time.Now().Add(-time.Hour)
		balanceCreatedAt := time.Now().Add(-24 * time.Hour)
		updatedAt := timestamp.Add(-30 * time.Minute)

		// Balances returned by the optimized query
		balancesAtTimestamp := []*mmodel.Balance{
			{
				ID:             balanceID.String(),
				OrganizationID: orgID.String(),
				LedgerID:       ledgerID.String(),
				AccountID:      accountID.String(),
				Alias:          "@user1",
				Key:            "default",
				AssetCode:      "USD",
				AccountType:    "deposit",
				Available:      decimal.NewFromInt(5000),
				OnHold:         decimal.NewFromInt(500),
				Version:        10,
				CreatedAt:      balanceCreatedAt,
				UpdatedAt:      updatedAt,
			},
		}

		balanceRepo := balance.NewMockRepository(ctrl)
		operationRepo := operation.NewMockRepository(ctrl)

		uc := UseCase{BalanceRepo: balanceRepo, OperationRepo: operationRepo}

		balanceRepo.EXPECT().
			ListByAccountIDAtTimestamp(gomock.Any(), orgID, ledgerID, accountID, timestamp).
			Return(balancesAtTimestamp, nil)

		results, err := uc.GetAccountBalancesAtTimestamp(context.Background(), orgID, ledgerID, accountID, timestamp)

		require.NoError(t, err)
		require.Len(t, results, 1)

		result := results[0]
		assert.Equal(t, balanceID.String(), result.ID)
		assert.Equal(t, decimal.NewFromInt(5000), result.Available)
		assert.Equal(t, decimal.NewFromInt(500), result.OnHold)
		assert.Equal(t, int64(10), result.Version)
		assert.Equal(t, balanceCreatedAt, result.CreatedAt)
		assert.Equal(t, updatedAt, result.UpdatedAt)
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

		results, err := uc.GetAccountBalancesAtTimestamp(context.Background(), orgID, ledgerID, accountID, futureTimestamp)

		assert.Error(t, err)
		assert.Nil(t, results)
		assert.Contains(t, err.Error(), constant.ErrInvalidTimestamp.Error())
	})

	t.Run("No balances at timestamp returns empty slice", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		orgID := libCommons.GenerateUUIDv7()
		ledgerID := libCommons.GenerateUUIDv7()
		accountID := libCommons.GenerateUUIDv7()
		timestamp := time.Now().Add(-24 * time.Hour)

		balanceRepo := balance.NewMockRepository(ctrl)
		operationRepo := operation.NewMockRepository(ctrl)

		uc := UseCase{BalanceRepo: balanceRepo, OperationRepo: operationRepo}

		balanceRepo.EXPECT().
			ListByAccountIDAtTimestamp(gomock.Any(), orgID, ledgerID, accountID, timestamp).
			Return([]*mmodel.Balance{}, nil)

		results, err := uc.GetAccountBalancesAtTimestamp(context.Background(), orgID, ledgerID, accountID, timestamp)

		require.NoError(t, err)
		assert.Empty(t, results)
	})

	t.Run("Balance repo error returns error", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		orgID := libCommons.GenerateUUIDv7()
		ledgerID := libCommons.GenerateUUIDv7()
		accountID := libCommons.GenerateUUIDv7()
		timestamp := time.Now().Add(-time.Hour)

		balanceRepo := balance.NewMockRepository(ctrl)
		operationRepo := operation.NewMockRepository(ctrl)

		uc := UseCase{BalanceRepo: balanceRepo, OperationRepo: operationRepo}

		balanceRepo.EXPECT().
			ListByAccountIDAtTimestamp(gomock.Any(), orgID, ledgerID, accountID, timestamp).
			Return(nil, errors.New("database error"))

		results, err := uc.GetAccountBalancesAtTimestamp(context.Background(), orgID, ledgerID, accountID, timestamp)

		assert.Error(t, err)
		assert.Nil(t, results)
	})

	t.Run("Multiple balances with different values", func(t *testing.T) {
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

		// Balance 1 has operations, Balance 2 has zero values (no operations)
		balancesAtTimestamp := []*mmodel.Balance{
			{
				ID:             balanceID1.String(),
				OrganizationID: orgID.String(),
				LedgerID:       ledgerID.String(),
				AccountID:      accountID.String(),
				Key:            "default",
				AssetCode:      "USD",
				AccountType:    "deposit",
				Available:      decimal.NewFromInt(5000),
				OnHold:         decimal.NewFromInt(500),
				Version:        10,
				CreatedAt:      balance1CreatedAt,
				UpdatedAt:      timestamp.Add(-30 * time.Minute),
			},
			{
				ID:             balanceID2.String(),
				OrganizationID: orgID.String(),
				LedgerID:       ledgerID.String(),
				AccountID:      accountID.String(),
				Key:            "savings",
				AssetCode:      "USD",
				AccountType:    "deposit",
				Available:      decimal.Zero,
				OnHold:         decimal.Zero,
				Version:        0,
				CreatedAt:      balance2CreatedAt,
				UpdatedAt:      balance2CreatedAt, // No operations, so UpdatedAt = CreatedAt
			},
		}

		balanceRepo := balance.NewMockRepository(ctrl)
		operationRepo := operation.NewMockRepository(ctrl)

		uc := UseCase{BalanceRepo: balanceRepo, OperationRepo: operationRepo}

		balanceRepo.EXPECT().
			ListByAccountIDAtTimestamp(gomock.Any(), orgID, ledgerID, accountID, timestamp).
			Return(balancesAtTimestamp, nil)

		results, err := uc.GetAccountBalancesAtTimestamp(context.Background(), orgID, ledgerID, accountID, timestamp)

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
		assert.Equal(t, decimal.NewFromInt(5000), balance1.Available)
		assert.Equal(t, int64(10), balance1.Version)

		require.NotNil(t, balance2)
		assert.True(t, balance2.Available.Equal(decimal.Zero))
		assert.Equal(t, int64(0), balance2.Version)
		assert.Equal(t, balance2CreatedAt, balance2.UpdatedAt)
	})
}
