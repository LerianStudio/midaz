package command

import (
	"context"
	"errors"
	"testing"

	libConstants "github.com/LerianStudio/lib-commons/v2/commons/constants"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/postgres/balance"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/redis"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/services/testsupport"
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
	// and persist the cached balances AS-IS (no re-applying transaction amounts).
	// The Lua script already applied this transaction's effects to Redis during HTTP request.

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

	// Expect BalancesUpdate to be called with cached balances AS-IS (no amount re-application).
	// Version should be 5 (cached version, not incremented - Option A: DB aligns with cache).
	// Available amounts should be exactly what's in cache (Lua already applied tx effects).
	mockBalanceRepo.EXPECT().
		BalancesUpdate(gomock.Any(), orgID, ledgerID, gomock.Any()).
		DoAndReturn(func(ctx context.Context, orgID, ledgerID uuid.UUID, balances []*mmodel.Balance) error {
			assert.Len(t, balances, 2, "Should update both balances from cache")
			// Version equals cache version (Option A: DB aligns with cache)
			assert.Equal(t, int64(5), balances[0].Version, "Version should equal cache version")
			assert.Equal(t, int64(5), balances[1].Version, "Version should equal cache version")
			// Available amounts are exactly what's in cache (no re-application)
			assert.Equal(t, decimal.NewFromInt(600), balances[0].Available, "account1 should have cached value 600")
			assert.Equal(t, decimal.NewFromInt(600), balances[1].Available, "account2 should have cached value 600")
			return nil
		})

	err := uc.UpdateBalances(ctx, orgID, ledgerID, uuid.New().String(), validate, balances)

	assert.NoError(t, err, "UpdateBalances should succeed after cache refresh when all balances are stale")
}

func TestUpdateBalances_PartialStale_SucceedsWithFreshBalances(t *testing.T) {
	t.Parallel()

	// When SOME balances are stale, UpdateBalances should refresh ALL balances from cache
	// and persist cached snapshots AS-IS (no re-applying transaction amounts).

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

	// Mock: account1 is stale (cache Version 5 > calculated Version 2), account2 is fresh (cache Version 1)
	// Called twice per balance: once in filterStaleBalances, once in refreshBalancesFromCache
	mockRedis.EXPECT().
		ListBalanceByKey(gomock.Any(), orgID, ledgerID, "account1").
		Return(&mmodel.Balance{ID: balanceID1, Alias: "account1", Available: decimal.NewFromInt(600), OnHold: decimal.Zero, Version: 5}, nil).
		Times(2)

	mockRedis.EXPECT().
		ListBalanceByKey(gomock.Any(), orgID, ledgerID, "account2").
		Return(&mmodel.Balance{ID: balanceID2, Alias: "account2", Available: decimal.NewFromInt(700), OnHold: decimal.Zero, Version: 1}, nil).
		Times(2)

	// Mock: BalancesUpdate should be called with cached snapshots (versions match cache)
	mockBalanceRepo.EXPECT().
		BalancesUpdate(gomock.Any(), orgID, ledgerID, gomock.Any()).
		DoAndReturn(func(ctx context.Context, orgID, ledgerID uuid.UUID, balances []*mmodel.Balance) error {
			assert.Len(t, balances, 2, "Should update both balances from cache")
			assert.Equal(t, balanceID1, balances[0].ID, "Should include account1")
			assert.Equal(t, balanceID2, balances[1].ID, "Should include account2")
			// Versions should equal cache versions (Option A: DB aligns with cache)
			assert.Equal(t, int64(5), balances[0].Version, "account1 version should equal cache version 5")
			assert.Equal(t, int64(1), balances[1].Version, "account2 version should equal cache version 1")
			// Available amounts should be exactly what's in cache
			assert.Equal(t, decimal.NewFromInt(600), balances[0].Available, "account1 should have cached value 600")
			assert.Equal(t, decimal.NewFromInt(700), balances[1].Available, "account2 should have cached value 700")
			return nil
		})

	// Execute
	err := uc.UpdateBalances(ctx, orgID, ledgerID, uuid.New().String(), validate, balances)

	// Assert: No error (partial stale triggers cache refresh for all)
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
		// First call: filterStaleBalances - cache has newer version (stale detected)
		mockRedis.EXPECT().
			ListBalanceByKey(gomock.Any(), orgID, ledgerID, "account1").
			Return(&mmodel.Balance{ID: balanceID, Alias: "account1", Available: decimal.NewFromInt(600), OnHold: decimal.Zero, Version: 5}, nil),
		// Second call: refreshBalancesFromCache - Redis fails
		mockRedis.EXPECT().
			ListBalanceByKey(gomock.Any(), orgID, ledgerID, "account1").
			Return(nil, errors.New("redis down")),
		// Third call: refreshBalancesFromDatabase fallback - DB also fails
		mockBalanceRepo.EXPECT().
			ListByAliasesWithKeys(gomock.Any(), orgID, ledgerID, []string{"account1"}).
			Return(nil, errors.New("database down")),
	)

	err := uc.UpdateBalances(ctx, orgID, ledgerID, uuid.New().String(), validate, balances)

	var failedPreconditionErr pkg.FailedPreconditionError
	assert.True(t, errors.As(err, &failedPreconditionErr),
		"Error should be FailedPreconditionError, got: %T", err)
	assert.Equal(t, constant.ErrStaleBalanceUpdateSkipped.Error(), failedPreconditionErr.Code,
		"Error code should match ErrStaleBalanceUpdateSkipped")
}

// TestUpdateBalances_StaleRefreshFails_DBFallbackSucceeds verifies that when cache refresh fails
// but DB fallback succeeds, the balance update completes successfully.
func TestUpdateBalances_StaleRefreshFails_DBFallbackSucceeds(t *testing.T) {
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

	// Use proper format: balance.Alias is "0#@account1#default" (index#@alias#key)
	// - The key in validate.From MUST match balance.Alias exactly for fromTo map lookup
	// - SplitAliasWithKey extracts "@account1#default" for Redis/DB lookup
	balanceAlias := "0#@account1#default"
	lookupKey := "@account1#default" // What SplitAliasWithKey returns

	validate := pkgTransaction.Responses{
		Aliases: []string{balanceAlias},
		From: map[string]pkgTransaction.Amount{
			balanceAlias: { // Must match balance.Alias exactly
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
			Alias:     balanceAlias, // Full format with index prefix
			AssetCode: "USD",
			Available: decimal.NewFromInt(500),
			OnHold:    decimal.Zero,
			Version:   1,
		},
	}

	gomock.InOrder(
		// First call: filterStaleBalances - cache has newer version (stale detected)
		mockRedis.EXPECT().
			ListBalanceByKey(gomock.Any(), orgID, ledgerID, lookupKey).
			Return(&mmodel.Balance{ID: balanceID, Alias: lookupKey, Available: decimal.NewFromInt(600), OnHold: decimal.Zero, Version: 5}, nil),
		// Second call: refreshBalancesFromCache - Redis fails
		mockRedis.EXPECT().
			ListBalanceByKey(gomock.Any(), orgID, ledgerID, lookupKey).
			Return(nil, errors.New("redis down")),
		// Third call: refreshBalancesFromDatabase fallback - DB succeeds
		// ListByAliasesWithKeys expects "@account1#default" format
		mockBalanceRepo.EXPECT().
			ListByAliasesWithKeys(gomock.Any(), orgID, ledgerID, []string{lookupKey}).
			Return([]*mmodel.Balance{
				{ID: balanceID, Alias: "@account1", Key: "default", Available: decimal.NewFromInt(600), OnHold: decimal.Zero, Version: 5},
			}, nil),
		// Fourth call: BalancesUpdate with DB values
		mockBalanceRepo.EXPECT().
			BalancesUpdate(gomock.Any(), orgID, ledgerID, gomock.Any()).
			Return(nil),
	)

	err := uc.UpdateBalances(ctx, orgID, ledgerID, uuid.New().String(), validate, balances)

	assert.NoError(t, err, "UpdateBalances should succeed when DB fallback works")
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
		_ = uc.UpdateBalances(ctx, orgID, ledgerID, uuid.New().String(), validate, balances)
	}, "UpdateBalances should panic when balancesToUpdate is empty")
}

// TestRefreshBalancesFromCache_UsesCachedValuesAsIs verifies that refreshBalancesFromCache
// returns cached values AS-IS without applying any transaction amounts.
// Lua script already applied transaction effects to the cache during HTTP request.
func TestRefreshBalancesFromCache_UsesCachedValuesAsIs(t *testing.T) {
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

	// Balances to refresh
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
			Alias:     "account2",
			AssetCode: "USD",
			Available: decimal.NewFromInt(500),
			OnHold:    decimal.Zero,
			Version:   1,
		},
	}

	// Mock Redis returns cached balances (these already include tx effects from Lua)
	mockRedis.EXPECT().
		ListBalanceByKey(gomock.Any(), orgID, ledgerID, "account1").
		Return(&mmodel.Balance{ID: balanceID1, Alias: "account1", Available: decimal.NewFromInt(600), OnHold: decimal.Zero, Version: 5}, nil).
		Times(1)

	mockRedis.EXPECT().
		ListBalanceByKey(gomock.Any(), orgID, ledgerID, "account2").
		Return(&mmodel.Balance{ID: balanceID2, Alias: "account2", Available: decimal.NewFromInt(700), OnHold: decimal.NewFromInt(50), Version: 5}, nil).
		Times(1)

	// Create a no-op logger for testing
	logger := &testsupport.MockLogger{}

	// Call refreshBalancesFromCache directly
	refreshed, err := uc.refreshBalancesFromCache(ctx, orgID, ledgerID, uuid.New().String(), balances, logger)

	// Verify no error
	assert.NoError(t, err, "refreshBalancesFromCache should succeed")
	assert.Len(t, refreshed, 2, "Should return both balances")

	// Find balances by ID
	var bal1, bal2 *mmodel.Balance
	for _, b := range refreshed {
		switch b.ID {
		case balanceID1:
			bal1 = b
		case balanceID2:
			bal2 = b
		}
	}

	// account1: cached values AS-IS (no amount applied, version equals cache version)
	assert.NotNil(t, bal1, "account1 should be in result")
	assert.Equal(t, int64(5), bal1.Version, "account1 version should equal cache version 5")
	assert.Equal(t, decimal.NewFromInt(600), bal1.Available, "account1 should have cached value 600 (no debit applied)")

	// account2: cached values AS-IS (even though not in fromTo)
	assert.NotNil(t, bal2, "account2 should be in result")
	assert.Equal(t, int64(5), bal2.Version, "account2 version should equal cache version 5")
	assert.Equal(t, decimal.NewFromInt(700), bal2.Available, "account2 should have cached value 700")
	assert.Equal(t, decimal.NewFromInt(50), bal2.OnHold, "account2 should have cached OnHold 50")
}

func TestRefreshBalancesFromCache_CacheVersionLowerThanMessage_LogsWarning(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockRedis := redis.NewMockRedisRepository(ctrl)

	uc := &UseCase{RedisRepo: mockRedis}

	ctx := context.Background()
	orgID := uuid.New()
	ledgerID := uuid.New()

	balanceID := uuid.New().String()

	balances := []*mmodel.Balance{
		{
			ID:        balanceID,
			Alias:     "account1",
			AssetCode: "USD",
			Available: decimal.NewFromInt(500),
			OnHold:    decimal.Zero,
			Version:   10, // Message/newBalances version
		},
	}

	mockRedis.EXPECT().
		ListBalanceByKey(gomock.Any(), orgID, ledgerID, "account1").
		Return(&mmodel.Balance{ID: balanceID, Alias: "account1", Available: decimal.NewFromInt(600), OnHold: decimal.Zero, Version: 5}, nil).
		Times(1)

	logger := &testsupport.MockLogger{}

	refreshed, err := uc.refreshBalancesFromCache(ctx, orgID, ledgerID, uuid.New().String(), balances, logger)
	assert.NoError(t, err)
	assert.Len(t, refreshed, 1)
	assert.Equal(t, int64(5), refreshed[0].Version)

	assert.NotEmpty(t, logger.WarnfMessages, "expected a warning when cache version is lower than message version")
	assert.Contains(t, logger.WarnfMessages[0], "cache version")
}

// TestUpdateBalances_CacheRefreshThenDBAlreadyUpdated_ReturnsSuccess verifies that when
// cache refresh is used and BalancesUpdate returns ErrNoBalancesUpdated (DB already has
// a newer version), UpdateBalances treats it as success (idempotent).
func TestUpdateBalances_CacheRefreshThenDBAlreadyUpdated_ReturnsSuccess(t *testing.T) {
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
			Version:   1, // Message version
		},
	}

	// Mock Redis returns version 5 (stale message triggers cache refresh)
	mockRedis.EXPECT().
		ListBalanceByKey(gomock.Any(), orgID, ledgerID, "account1").
		Return(&mmodel.Balance{ID: balanceID, Alias: "account1", Available: decimal.NewFromInt(600), OnHold: decimal.Zero, Version: 5}, nil).
		Times(2) // filterStaleBalances + refreshBalancesFromCache

	// BalancesUpdate returns ErrNoBalancesUpdated (DB already at version >= 5)
	mockBalanceRepo.EXPECT().
		BalancesUpdate(gomock.Any(), orgID, ledgerID, gomock.Any()).
		Return(pkg.ValidateInternalError(balance.ErrNoBalancesUpdated, "Balance"))

	err := uc.UpdateBalances(ctx, orgID, ledgerID, uuid.New().String(), validate, balances)

	// Should succeed (idempotent - DB already up-to-date)
	assert.NoError(t, err, "UpdateBalances should return nil when DB already has newer version after cache refresh")
}

// TestUpdateBalances_RetryRefreshThenDBAlreadyUpdated_ReturnsSuccess verifies that when
// the initial BalancesUpdate fails with ErrNoBalancesUpdated (without cache refresh),
// a retry with cache refresh is attempted, and if that also returns ErrNoBalancesUpdated,
// UpdateBalances treats it as success (idempotent).
func TestUpdateBalances_RetryRefreshThenDBAlreadyUpdated_ReturnsSuccess(t *testing.T) {
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
			Version:   1, // Message version
		},
	}

	// First call: filterStaleBalances - cache version equals calculated version (not stale)
	// This means usedCache=false for initial BalancesUpdate
	mockRedis.EXPECT().
		ListBalanceByKey(gomock.Any(), orgID, ledgerID, "account1").
		Return(&mmodel.Balance{ID: balanceID, Alias: "account1", Available: decimal.NewFromInt(400), OnHold: decimal.Zero, Version: 2}, nil).
		Times(1)

	// Second call: retryBalanceUpdateWithCacheRefresh - refreshBalancesFromCache
	mockRedis.EXPECT().
		ListBalanceByKey(gomock.Any(), orgID, ledgerID, "account1").
		Return(&mmodel.Balance{ID: balanceID, Alias: "account1", Available: decimal.NewFromInt(600), OnHold: decimal.Zero, Version: 5}, nil).
		Times(1)

	gomock.InOrder(
		// First BalancesUpdate fails with ErrNoBalancesUpdated (version conflict)
		mockBalanceRepo.EXPECT().
			BalancesUpdate(gomock.Any(), orgID, ledgerID, gomock.Any()).
			Return(pkg.ValidateInternalError(balance.ErrNoBalancesUpdated, "Balance")),
		// Retry BalancesUpdate also returns ErrNoBalancesUpdated (DB already newer)
		mockBalanceRepo.EXPECT().
			BalancesUpdate(gomock.Any(), orgID, ledgerID, gomock.Any()).
			Return(pkg.ValidateInternalError(balance.ErrNoBalancesUpdated, "Balance")),
	)

	err := uc.UpdateBalances(ctx, orgID, ledgerID, uuid.New().String(), validate, balances)

	// Should succeed (idempotent - DB already up-to-date after retry)
	assert.NoError(t, err, "UpdateBalances should return nil when DB already has newer version after retry refresh")
}
