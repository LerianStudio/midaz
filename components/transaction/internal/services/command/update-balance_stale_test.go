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
	// Called twice per balance: once in filterStaleBalances, once in refreshBalancesFromCache
	mockRedis.EXPECT().
		ListBalanceByKey(gomock.Any(), orgID, ledgerID, "account1").
		Return(&mmodel.Balance{ID: balanceID1, Alias: "account1", Available: decimal.NewFromInt(600), OnHold: decimal.Zero, Version: 5}, nil).
		Times(2)

	mockRedis.EXPECT().
		ListBalanceByKey(gomock.Any(), orgID, ledgerID, "account2").
		Return(&mmodel.Balance{ID: balanceID2, Alias: "account2", Available: decimal.NewFromInt(600), OnHold: decimal.Zero, Version: 5}, nil).
		Times(2)

	// Expect BalancesUpdate to be called with cached balances + transaction amounts applied.
	// Version should be 6 (cached version 5 + 1 from OperateBalances).
	// Available amounts should reflect the transaction:
	// - account1: 600 (cached) - 100 (debit) = 500
	// - account2: 600 (cached) + 100 (credit) = 700
	mockBalanceRepo.EXPECT().
		BalancesUpdate(gomock.Any(), orgID, ledgerID, gomock.Any()).
		DoAndReturn(func(ctx context.Context, orgID, ledgerID uuid.UUID, balances []*mmodel.Balance) error {
			assert.Len(t, balances, 2, "Should update both balances from cache")
			// Version is incremented by OperateBalances: cache version (5) + 1 = 6
			assert.Equal(t, int64(6), balances[0].Version, "Version should be cache version + 1")
			assert.Equal(t, int64(6), balances[1].Version, "Version should be cache version + 1")
			// Verify transaction amounts were applied to cached balances
			assert.Equal(t, decimal.NewFromInt(500), balances[0].Available, "account1 should have 600 - 100 = 500")
			assert.Equal(t, decimal.NewFromInt(700), balances[1].Available, "account2 should have 600 + 100 = 700")
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
	// Called twice per balance: once in filterStaleBalances, once in refreshBalancesFromCache
	mockRedis.EXPECT().
		ListBalanceByKey(gomock.Any(), orgID, ledgerID, "account1").
		Return(&mmodel.Balance{ID: balanceID1, Alias: "account1", Available: decimal.NewFromInt(600), OnHold: decimal.Zero, Version: 5}, nil).
		Times(2)

	mockRedis.EXPECT().
		ListBalanceByKey(gomock.Any(), orgID, ledgerID, "account2").
		Return(&mmodel.Balance{ID: balanceID2, Alias: "account2", Available: decimal.NewFromInt(700), OnHold: decimal.Zero, Version: 1}, nil).
		Times(2)

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

// TestUpdateBalances_EmptyBalances_Panics verifies that when balancesToUpdate is empty,
// the system panics rather than silently succeeding. This prevents data loss scenarios
// where transactions are created but balances are never persisted.
func TestUpdateBalances_EmptyBalances_Panics(t *testing.T) {
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

	// Empty validate and balances to trigger the assertion
	validate := pkgTransaction.Responses{
		From: map[string]pkgTransaction.Amount{},
		To:   map[string]pkgTransaction.Amount{},
	}

	// Empty balances slice - this should trigger the assertion
	balances := []*mmodel.Balance{}

	// Verify panic occurs with expected assertion
	assert.Panics(t, func() {
		_ = uc.UpdateBalances(ctx, orgID, ledgerID, validate, balances)
	}, "UpdateBalances should panic when balancesToUpdate is empty")
}

// TestRefreshBalancesFromCache_MissingAmount_UsesWarnFallback verifies that when a balance alias
// is not found in the fromTo map, the system logs a warning and uses cached values with
// incremented version (fallback path in refreshBalancesFromCache).
//
// This tests the internal refreshBalancesFromCache function directly because the fallback
// path cannot be reached through UpdateBalances (calculateNewBalances would fail first).
func TestRefreshBalancesFromCache_MissingAmount_UsesWarnFallback(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockRedis := redis.NewMockRedisRepository(ctrl)

	uc := &UseCase{
		RedisRepo: mockRedis,
	}

	ctx := context.Background()
	orgID := uuid.New()
	ledgerID := uuid.New()

	balanceID1 := uuid.New().String()
	balanceID2 := uuid.New().String()

	// Only account1 has an entry in fromTo - account2 will trigger the fallback path
	fromTo := map[string]pkgTransaction.Amount{
		"account1": {
			Asset:           "USD",
			Value:           decimal.NewFromInt(100),
			Operation:       libConstants.DEBIT,
			TransactionType: libConstants.CREATED,
		},
		// account2 is intentionally missing to trigger fallback
	}

	// Balances to refresh - account2 has no entry in fromTo
	balances := []*mmodel.Balance{
		{
			ID:        balanceID1,
			Alias:     "account1",
			AssetCode: "USD",
			Available: decimal.NewFromInt(500),
			OnHold:    decimal.Zero,
			Version:   1,
		},
		{
			ID:        balanceID2,
			Alias:     "account2", // NOT in fromTo - triggers fallback
			AssetCode: "USD",
			Available: decimal.NewFromInt(500),
			OnHold:    decimal.Zero,
			Version:   1,
		},
	}

	// Mock Redis returns cached balances
	mockRedis.EXPECT().
		ListBalanceByKey(gomock.Any(), orgID, ledgerID, "account1").
		Return(&mmodel.Balance{ID: balanceID1, Alias: "account1", Available: decimal.NewFromInt(600), OnHold: decimal.Zero, Version: 5}, nil).
		Times(1)

	mockRedis.EXPECT().
		ListBalanceByKey(gomock.Any(), orgID, ledgerID, "account2").
		Return(&mmodel.Balance{ID: balanceID2, Alias: "account2", Available: decimal.NewFromInt(700), OnHold: decimal.NewFromInt(50), Version: 5}, nil).
		Times(1)

	// Create a no-op logger for testing
	logger := &MockLogger{}

	// Call refreshBalancesFromCache directly
	refreshed, err := uc.refreshBalancesFromCache(ctx, orgID, ledgerID, balances, fromTo, logger)

	// Verify no error
	assert.NoError(t, err, "refreshBalancesFromCache should succeed with fallback path")
	assert.Len(t, refreshed, 2, "Should return both balances")

	// Find balances by ID
	var bal1, bal2 *mmodel.Balance
	for _, b := range refreshed {
		if b.ID == balanceID1 {
			bal1 = b
		} else if b.ID == balanceID2 {
			bal2 = b
		}
	}

	// account1: transaction was applied via OperateBalances
	assert.NotNil(t, bal1, "account1 should be in result")
	assert.Equal(t, int64(6), bal1.Version, "account1 version should be 6 (from OperateBalances)")
	assert.Equal(t, decimal.NewFromInt(500), bal1.Available, "account1 should have 600 - 100 = 500")

	// account2: fallback path - cached values with manually incremented version
	assert.NotNil(t, bal2, "account2 should be in result")
	assert.Equal(t, int64(6), bal2.Version, "account2 version should be 6 (cached 5 + 1 manual increment)")
	assert.Equal(t, decimal.NewFromInt(700), bal2.Available, "account2 should keep cached value 700")
	assert.Equal(t, decimal.NewFromInt(50), bal2.OnHold, "account2 should keep cached OnHold 50")
}
