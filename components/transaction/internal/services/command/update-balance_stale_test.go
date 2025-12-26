package command

import (
	"context"
	"errors"
	"testing"

	libConstants "github.com/LerianStudio/lib-commons/v2/commons/constants"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/postgres/balance"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/redis"
	"github.com/LerianStudio/midaz/v3/pkg"
	"github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	pkgTransaction "github.com/LerianStudio/midaz/v3/pkg/transaction"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
)

func TestUpdateBalances_AllStale_RefreshesFromCache(t *testing.T) {
	t.Parallel()

	// When ALL balances are stale, UpdateBalances should refresh from cache
	// and persist the cached balances so BTO can continue safely.

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockRedis := redis.NewMockRedisRepository(ctrl)
	mockBalanceRepo := balance.NewMockRepository(ctrl)

	uc := &UseCase{
		RedisRepo:   mockRedis,
		BalanceRepo: mockBalanceRepo,
	}

	ctx := context.Background()
	orgID := uuid.New()
	ledgerID := uuid.New()

	// Create balances with version 1 (simulating message payload)
	balanceID1 := uuid.New().String()
	balanceID2 := uuid.New().String()

	// Validate must have From/To maps with proper TransactionType and Operation fields
	// The keys in From/To MUST match the Alias in balances exactly (see update-balance.go line 50)
	validate := pkgTransaction.Responses{
		Aliases: []string{"account1", "account2"},
		From: map[string]pkgTransaction.Amount{
			"account1": {
				Asset:           "USD",
				Value:           decimal.NewFromInt(100),
				Operation:       libConstants.DEBIT,
				TransactionType: libConstants.CREATED,
			},
		},
		To: map[string]pkgTransaction.Amount{
			"account2": {
				Asset:           "USD",
				Value:           decimal.NewFromInt(100),
				Operation:       libConstants.CREDIT,
				TransactionType: libConstants.CREATED,
			},
		},
	}

	balances := []*mmodel.Balance{
		{
			ID:        balanceID1,
			Alias:     "account1", // Must match key in validate.From
			AssetCode: "USD",
			Available: decimal.NewFromInt(500),
			OnHold:    decimal.Zero,
			Version:   1, // Message version
		},
		{
			ID:        balanceID2,
			Alias:     "account2", // Must match key in validate.To
			AssetCode: "USD",
			Available: decimal.NewFromInt(500),
			OnHold:    decimal.Zero,
			Version:   1, // Message version
		},
	}

	// Mock Redis returns version 5 (newer than message version 1) for both balances
	mockRedis.EXPECT().
		ListBalanceByKey(gomock.Any(), orgID, ledgerID, "account1").
		Return(&mmodel.Balance{ID: balanceID1, Alias: "account1", Available: decimal.NewFromInt(600), OnHold: decimal.Zero, Version: 5}, nil).
		AnyTimes()

	mockRedis.EXPECT().
		ListBalanceByKey(gomock.Any(), orgID, ledgerID, "account2").
		Return(&mmodel.Balance{ID: balanceID2, Alias: "account2", Available: decimal.NewFromInt(600), OnHold: decimal.Zero, Version: 5}, nil).
		AnyTimes()

	// Expect BalancesUpdate to be called with cached balances (version 5)
	mockBalanceRepo.EXPECT().
		BalancesUpdate(gomock.Any(), orgID, ledgerID, gomock.Any()).
		DoAndReturn(func(ctx context.Context, orgID, ledgerID uuid.UUID, balances []*mmodel.Balance) error {
			assert.Len(t, balances, 2, "Should update both balances from cache")
			assert.Equal(t, int64(5), balances[0].Version)
			assert.Equal(t, int64(5), balances[1].Version)
			return nil
		})

	err := uc.UpdateBalances(ctx, orgID, ledgerID, validate, balances)

	assert.NoError(t, err, "UpdateBalances should succeed after cache refresh when all balances are stale")
}

func TestUpdateBalances_PartialStale_SucceedsWithFreshBalances(t *testing.T) {
	t.Parallel()

	// When SOME balances are stale, UpdateBalances should refresh from cache
	// to ensure the database reflects the latest state and returns success.

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockRedis := redis.NewMockRedisRepository(ctrl)
	mockBalanceRepo := balance.NewMockRepository(ctrl)

	uc := &UseCase{
		RedisRepo:   mockRedis,
		BalanceRepo: mockBalanceRepo,
	}

	ctx := context.Background()
	orgID := uuid.New()
	ledgerID := uuid.New()

	balanceID1 := uuid.New().String()
	balanceID2 := uuid.New().String()

	// Validate must have From/To maps with proper TransactionType and Operation fields
	// The keys in From/To MUST match the Alias in balances exactly (see update-balance.go line 50)
	validate := pkgTransaction.Responses{
		Aliases: []string{"account1", "account2"},
		From: map[string]pkgTransaction.Amount{
			"account1": {
				Asset:           "USD",
				Value:           decimal.NewFromInt(100),
				Operation:       libConstants.DEBIT,
				TransactionType: libConstants.CREATED,
			},
		},
		To: map[string]pkgTransaction.Amount{
			"account2": {
				Asset:           "USD",
				Value:           decimal.NewFromInt(100),
				Operation:       libConstants.CREDIT,
				TransactionType: libConstants.CREATED,
			},
		},
	}

	balances := []*mmodel.Balance{
		{
			ID:        balanceID1,
			Alias:     "account1", // Must match key in validate.From
			AssetCode: "USD",
			Available: decimal.NewFromInt(500),
			OnHold:    decimal.Zero,
			Version:   1, // Message version
		},
		{
			ID:        balanceID2,
			Alias:     "account2", // Must match key in validate.To
			AssetCode: "USD",
			Available: decimal.NewFromInt(500),
			OnHold:    decimal.Zero,
			Version:   1, // Message version
		},
	}

	// Mock: account1 is stale (Version 5 > 2), account2 is fresh (Version 1 < 2)
	mockRedis.EXPECT().
		ListBalanceByKey(gomock.Any(), orgID, ledgerID, "account1").
		Return(&mmodel.Balance{ID: balanceID1, Alias: "account1", Available: decimal.NewFromInt(600), OnHold: decimal.Zero, Version: 5}, nil).
		AnyTimes()

	mockRedis.EXPECT().
		ListBalanceByKey(gomock.Any(), orgID, ledgerID, "account2").
		Return(&mmodel.Balance{ID: balanceID2, Alias: "account2", Available: decimal.NewFromInt(700), OnHold: decimal.Zero, Version: 1}, nil).
		AnyTimes()

	// Mock: BalancesUpdate should be called with refreshed balances (both accounts)
	mockBalanceRepo.EXPECT().
		BalancesUpdate(gomock.Any(), orgID, ledgerID, gomock.Any()).
		DoAndReturn(func(ctx context.Context, orgID, ledgerID uuid.UUID, balances []*mmodel.Balance) error {
			assert.Len(t, balances, 2, "Should update both balances from cache")
			assert.Equal(t, balanceID1, balances[0].ID, "Should include account1 (stale)")
			assert.Equal(t, balanceID2, balances[1].ID, "Should include account2 (fresh)")
			return nil
		})

	// Execute
	err := uc.UpdateBalances(ctx, orgID, ledgerID, validate, balances)

	// Assert: No error (partial stale is acceptable)
	assert.NoError(t, err, "UpdateBalances should succeed after cache refresh with partial stale balances")
}

func TestUpdateBalances_StaleRefreshFails_ReturnsError(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockRedis := redis.NewMockRedisRepository(ctrl)
	mockBalanceRepo := balance.NewMockRepository(ctrl)

	uc := &UseCase{
		RedisRepo:   mockRedis,
		BalanceRepo: mockBalanceRepo,
	}

	ctx := context.Background()
	orgID := uuid.New()
	ledgerID := uuid.New()

	balanceID := uuid.New().String()

	validate := pkgTransaction.Responses{
		Aliases: []string{"account1"},
		From: map[string]pkgTransaction.Amount{
			"account1": {
				Asset:           "USD",
				Value:           decimal.NewFromInt(100),
				Operation:       libConstants.DEBIT,
				TransactionType: libConstants.CREATED,
			},
		},
	}

	balances := []*mmodel.Balance{
		{
			ID:        balanceID,
			Alias:     "account1",
			AssetCode: "USD",
			Available: decimal.NewFromInt(500),
			OnHold:    decimal.Zero,
			Version:   1,
		},
	}

	gomock.InOrder(
		mockRedis.EXPECT().
			ListBalanceByKey(gomock.Any(), orgID, ledgerID, "account1").
			Return(&mmodel.Balance{ID: balanceID, Alias: "account1", Available: decimal.NewFromInt(600), OnHold: decimal.Zero, Version: 5}, nil),
		mockRedis.EXPECT().
			ListBalanceByKey(gomock.Any(), orgID, ledgerID, "account1").
			Return(nil, errors.New("redis down")),
	)

	err := uc.UpdateBalances(ctx, orgID, ledgerID, validate, balances)

	var failedPreconditionErr pkg.FailedPreconditionError
	assert.True(t, errors.As(err, &failedPreconditionErr),
		"Error should be FailedPreconditionError, got: %T", err)
	assert.Equal(t, constant.ErrStaleBalanceUpdateSkipped.Error(), failedPreconditionErr.Code,
		"Error code should match ErrStaleBalanceUpdateSkipped")
}
