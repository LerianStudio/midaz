// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	libCommons "github.com/LerianStudio/lib-commons/v5/commons"
	"github.com/LerianStudio/midaz/v3/components/ledger/internal/adapters/postgres/balance"
	redis "github.com/LerianStudio/midaz/v3/components/ledger/internal/adapters/redis/transaction"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/LerianStudio/midaz/v3/pkg/mtransaction"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

// TestUpdateBalance tests the Update method with no Redis cached value
func TestUpdateBalance(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	organizationID := uuid.Must(libCommons.GenerateUUIDv7())
	ledgerID := uuid.Must(libCommons.GenerateUUIDv7())
	balanceID := uuid.Must(libCommons.GenerateUUIDv7())

	allowSending := false

	balanceUpdate := mmodel.UpdateBalance{
		AllowSending:   &allowSending,
		AllowReceiving: nil,
	}

	expectedBalance := &mmodel.Balance{
		ID:             balanceID.String(),
		OrganizationID: organizationID.String(),
		LedgerID:       ledgerID.String(),
		Alias:          "@test",
		Key:            "default",
		AllowSending:   false,
		AllowReceiving: true,
	}

	mockBalanceRepo := balance.NewMockRepository(ctrl)
	mockRedisRepo := redis.NewMockRedisRepository(ctrl)

	// Find is always called (scope guard requires the current balance).
	mockBalanceRepo.EXPECT().
		Find(gomock.Any(), organizationID, ledgerID, balanceID).
		Return(expectedBalance, nil).
		Times(1)

	mockBalanceRepo.EXPECT().
		Update(gomock.Any(), organizationID, ledgerID, balanceID, balanceUpdate).
		Return(expectedBalance, nil).
		Times(1)

	// Redis returns empty (no cached value)
	mockRedisRepo.EXPECT().
		Get(gomock.Any(), gomock.Any()).
		Return("", nil).
		Times(1)

	// No Settings in the update payload → the cache settings rewrite MUST NOT
	// fire. AllowSending/AllowReceiving mutations don't touch the overdraft
	// settings contract guarded by UpdateBalanceCacheSettings, so the cached
	// balance JSON (including its live transactional state) is left alone.
	mockRedisRepo.EXPECT().
		UpdateBalanceCacheSettings(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Times(0)

	uc := UseCase{
		BalanceRepo:          mockBalanceRepo,
		TransactionRedisRepo: mockRedisRepo,
	}

	result, err := uc.Update(context.TODO(), organizationID, ledgerID, balanceID, balanceUpdate)

	assert.Nil(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, expectedBalance.ID, result.ID)
	assert.False(t, result.AllowSending)
}

// TestUpdateBalance_RepoError tests the Update method when repository returns error
func TestUpdateBalance_RepoError(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	errMSG := "errDatabaseItemNotFound"
	organizationID := uuid.Must(libCommons.GenerateUUIDv7())
	ledgerID := uuid.Must(libCommons.GenerateUUIDv7())
	balanceID := uuid.Must(libCommons.GenerateUUIDv7())

	allowSending := true
	allowReceiving := false

	balanceUpdate := mmodel.UpdateBalance{
		AllowSending:   &allowSending,
		AllowReceiving: &allowReceiving,
	}

	mockBalanceRepo := balance.NewMockRepository(ctrl)

	// Find is always called (scope guard requires the current balance).
	normalBalance := &mmodel.Balance{
		ID:    balanceID.String(),
		Alias: "@test",
		Key:   "default",
	}
	mockBalanceRepo.EXPECT().
		Find(gomock.Any(), organizationID, ledgerID, balanceID).
		Return(normalBalance, nil).
		Times(1)

	mockBalanceRepo.EXPECT().
		Update(gomock.Any(), organizationID, ledgerID, balanceID, balanceUpdate).
		Return(nil, errors.New(errMSG))
	// Redis is NOT called when Update fails

	uc := UseCase{
		BalanceRepo: mockBalanceRepo,
	}

	result, err := uc.Update(context.TODO(), organizationID, ledgerID, balanceID, balanceUpdate)

	assert.Nil(t, result)
	assert.NotEmpty(t, err)
	assert.Equal(t, err.Error(), errMSG)
}

// TestUpdateBalance_RedisOverlay verifies that when Redis has cached balance values,
// they are overlayed onto the balance returned from the repository.
func TestUpdateBalance_RedisOverlay(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	organizationID := uuid.Must(libCommons.GenerateUUIDv7())
	ledgerID := uuid.Must(libCommons.GenerateUUIDv7())
	balanceID := uuid.Must(libCommons.GenerateUUIDv7())

	allowSending := false

	balanceUpdate := mmodel.UpdateBalance{
		AllowSending: &allowSending,
	}

	// Repository returns balance with initial values
	repoBalance := &mmodel.Balance{
		ID:             balanceID.String(),
		OrganizationID: organizationID.String(),
		LedgerID:       ledgerID.String(),
		Alias:          "@user1",
		Key:            "default",
		AssetCode:      "USD",
		Available:      decimal.NewFromInt(100),
		OnHold:         decimal.NewFromInt(10),
		Version:        1,
		AllowSending:   false,
		AllowReceiving: true,
	}

	// Redis has fresher values that should be overlayed
	cachedBalance := mmodel.BalanceRedis{
		Available: decimal.NewFromInt(500),
		OnHold:    decimal.NewFromInt(50),
		Version:   5,
	}
	cachedJSON, err := json.Marshal(cachedBalance)
	require.NoError(t, err)

	mockBalanceRepo := balance.NewMockRepository(ctrl)
	mockRedisRepo := redis.NewMockRedisRepository(ctrl)

	// Find is always called (scope guard requires the current balance).
	mockBalanceRepo.EXPECT().
		Find(gomock.Any(), organizationID, ledgerID, balanceID).
		Return(repoBalance, nil).
		Times(1)

	mockBalanceRepo.EXPECT().
		Update(gomock.Any(), organizationID, ledgerID, balanceID, balanceUpdate).
		Return(repoBalance, nil).
		Times(1)

	mockRedisRepo.EXPECT().
		Get(gomock.Any(), gomock.Any()).
		Return(string(cachedJSON), nil).
		Times(1)

	// No Settings in the update → the cache rewrite MUST NOT run, so the
	// in-flight transactional snapshot held in Redis (Available=500, OnHold=50,
	// Version=5) is neither overwritten nor deleted. This is the whole point
	// of replacing the prior Del: preserving live state across settings-free
	// updates.
	mockRedisRepo.EXPECT().
		UpdateBalanceCacheSettings(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Times(0)

	uc := UseCase{
		BalanceRepo:          mockBalanceRepo,
		TransactionRedisRepo: mockRedisRepo,
	}

	result, err := uc.Update(context.TODO(), organizationID, ledgerID, balanceID, balanceUpdate)

	require.NoError(t, err)
	require.NotNil(t, result)

	// Redis values should be overlayed
	assert.True(t, result.Available.Equal(decimal.NewFromInt(500)), "Available should be overlayed from Redis")
	assert.True(t, result.OnHold.Equal(decimal.NewFromInt(50)), "OnHold should be overlayed from Redis")
	assert.Equal(t, int64(5), result.Version, "Version should be overlayed from Redis")

	// Other fields should remain unchanged from repository
	assert.Equal(t, balanceID.String(), result.ID)
	assert.Equal(t, "@user1", result.Alias)
	assert.Equal(t, "USD", result.AssetCode)
	assert.False(t, result.AllowSending)
	assert.True(t, result.AllowReceiving)
}

// TestUpdateBalance_CacheSettingsUpdate_UsesCompositeAliasKey pins the exact
// composite `alias#key` that the command layer passes to the Redis repo when
// rewriting settings in the cached balance. If the composition rule drifts
// (e.g. a refactor separates alias and key into distinct arguments or
// re-orders them), this test fails loudly so the Lua-mutated balance key
// and the command-side cache key stay in lock-step.
//
// The repo method is responsible for composing BalanceInternalKey from the
// composite cacheKey + (org, ledger); this test pins the command-side
// contract with the repo, not the full prefixed Redis key.
func TestUpdateBalance_CacheSettingsUpdate_UsesCompositeAliasKey(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	organizationID := uuid.Must(libCommons.GenerateUUIDv7())
	ledgerID := uuid.Must(libCommons.GenerateUUIDv7())
	balanceID := uuid.Must(libCommons.GenerateUUIDv7())

	allowOverdraft := true
	newSettings := &mmodel.BalanceSettings{
		AllowOverdraft: allowOverdraft,
	}
	balanceUpdate := mmodel.UpdateBalance{Settings: newSettings}

	expectedBalance := &mmodel.Balance{
		ID:             balanceID.String(),
		OrganizationID: organizationID.String(),
		LedgerID:       ledgerID.String(),
		Alias:          "@user1",
		Key:            "default",
		AllowSending:   true,
		AllowReceiving: true,
	}

	// The command layer MUST pass the composite "alias#key" form so the repo
	// can rebuild BalanceInternalKey consistently with the Lua script's
	// SET NX key. Any other separator or ordering would desynchronize the
	// write and the transactional read.
	expectedCompositeKey := "@user1#default"

	mockBalanceRepo := balance.NewMockRepository(ctrl)
	mockRedisRepo := redis.NewMockRedisRepository(ctrl)

	// Settings path triggers Find + ensureOverdraftBalance; stub them as no-ops
	// that simulate a balance which already has overdraft enabled (avoids the
	// auto-create side-effect). Find returns a balance whose settings match the
	// update — enforceOverdraftTransition treats this as a no-op transition.
	existing := &mmodel.Balance{
		ID:             balanceID.String(),
		OrganizationID: organizationID.String(),
		LedgerID:       ledgerID.String(),
		Alias:          "@user1",
		Key:            "default",
		Settings: &mmodel.BalanceSettings{
			AllowOverdraft: true,
		},
	}

	mockBalanceRepo.EXPECT().
		Find(gomock.Any(), organizationID, ledgerID, balanceID).
		Return(existing, nil).
		Times(1)

	mockBalanceRepo.EXPECT().
		Update(gomock.Any(), organizationID, ledgerID, balanceID, balanceUpdate).
		Return(expectedBalance, nil).
		Times(1)

	// The Get overlay still uses the same prefixed key; keep it tolerant.
	mockRedisRepo.EXPECT().
		Get(gomock.Any(), gomock.Any()).
		Return("", nil).
		Times(1)

	// HARD GATE: the cacheKey arg MUST be the composite "alias#key".
	// The new settings payload MUST be forwarded verbatim so the repo can
	// apply an in-place rewrite without losing live transactional state.
	mockRedisRepo.EXPECT().
		UpdateBalanceCacheSettings(gomock.Any(), organizationID, ledgerID, expectedCompositeKey, newSettings).
		Return(nil).
		Times(1)

	uc := UseCase{
		BalanceRepo:          mockBalanceRepo,
		TransactionRedisRepo: mockRedisRepo,
	}

	result, err := uc.Update(context.TODO(), organizationID, ledgerID, balanceID, balanceUpdate)

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, expectedBalance.ID, result.ID)
}

// TestUpdateBalance_CacheSettingsUpdate_FailureIsBestEffort verifies that a
// Redis failure during the settings-only cache rewrite does not propagate to
// the caller: the PostgreSQL write has already succeeded and is durable, so
// the update must still return the updated balance. A subsequent
// transaction's cache miss (or the sync worker) will reconcile.
//
// A Settings payload is used to guarantee the cache-rewrite path actually
// executes (non-settings updates skip the rewrite by design).
func TestUpdateBalance_CacheSettingsUpdate_FailureIsBestEffort(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	organizationID := uuid.Must(libCommons.GenerateUUIDv7())
	ledgerID := uuid.Must(libCommons.GenerateUUIDv7())
	balanceID := uuid.Must(libCommons.GenerateUUIDv7())

	allowOverdraft := true
	balanceUpdate := mmodel.UpdateBalance{
		Settings: &mmodel.BalanceSettings{
			AllowOverdraft: allowOverdraft,
		},
	}

	expectedBalance := &mmodel.Balance{
		ID:             balanceID.String(),
		OrganizationID: organizationID.String(),
		LedgerID:       ledgerID.String(),
		Alias:          "@user1",
		Key:            "default",
	}

	// Find returns a balance whose settings already match the update — the
	// enforceOverdraftTransition check treats this as a no-op transition so
	// the test stays focused on the cache-rewrite failure path.
	existing := &mmodel.Balance{
		ID:             balanceID.String(),
		OrganizationID: organizationID.String(),
		LedgerID:       ledgerID.String(),
		Alias:          "@user1",
		Key:            "default",
		Settings: &mmodel.BalanceSettings{
			AllowOverdraft: true,
		},
	}

	mockBalanceRepo := balance.NewMockRepository(ctrl)
	mockRedisRepo := redis.NewMockRedisRepository(ctrl)

	mockBalanceRepo.EXPECT().
		Find(gomock.Any(), organizationID, ledgerID, balanceID).
		Return(existing, nil).
		Times(1)

	mockBalanceRepo.EXPECT().
		Update(gomock.Any(), organizationID, ledgerID, balanceID, balanceUpdate).
		Return(expectedBalance, nil).
		Times(1)

	mockRedisRepo.EXPECT().
		Get(gomock.Any(), gomock.Any()).
		Return("", nil).
		Times(1)

	// Redis is down / network partition. The use case must swallow the error
	// and return the updated balance anyway — the PG write is the source of
	// truth and cannot be rolled back at this point.
	mockRedisRepo.EXPECT().
		UpdateBalanceCacheSettings(gomock.Any(), organizationID, ledgerID, gomock.Any(), gomock.Any()).
		Return(errors.New("redis connection refused")).
		Times(1)

	uc := UseCase{
		BalanceRepo:          mockBalanceRepo,
		TransactionRedisRepo: mockRedisRepo,
	}

	result, err := uc.Update(context.TODO(), organizationID, ledgerID, balanceID, balanceUpdate)

	require.NoError(t, err, "Cache rewrite failure must not propagate — PG write is durable")
	require.NotNil(t, result)
	assert.Equal(t, expectedBalance.ID, result.ID)
}

func TestUpdateBalances_PrimaryPath_UsesAfterDirectly(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	organizationID := uuid.Must(libCommons.GenerateUUIDv7())
	ledgerID := uuid.Must(libCommons.GenerateUUIDv7())

	balancesBefore := []*mmodel.Balance{
		{
			ID:        "bal-1",
			Alias:     "@alice",
			Available: decimal.NewFromInt(1000),
			OnHold:    decimal.NewFromInt(0),
			Version:   1,
		},
		{
			ID:        "bal-2",
			Alias:     "@bob",
			Available: decimal.NewFromInt(500),
			OnHold:    decimal.NewFromInt(0),
			Version:   3,
		},
	}

	balancesAfter := []*mmodel.Balance{
		{
			Alias:     "@alice",
			Available: decimal.NewFromInt(900),
			OnHold:    decimal.NewFromInt(0),
			Version:   2,
		},
		{
			Alias:     "@bob",
			Available: decimal.NewFromInt(600),
			OnHold:    decimal.NewFromInt(0),
			Version:   4,
		},
	}

	mockBalanceRepo := balance.NewMockRepository(ctrl)

	mockBalanceRepo.EXPECT().
		BalancesUpdate(gomock.Any(), organizationID, ledgerID, gomock.Any()).
		DoAndReturn(func(_ context.Context, _, _ uuid.UUID, balances []*mmodel.Balance) error {
			require.Len(t, balances, 2)

			// Verify ID comes from BEFORE, values from AFTER
			assert.Equal(t, "bal-1", balances[0].ID)
			assert.Equal(t, "@alice", balances[0].Alias)
			assert.True(t, balances[0].Available.Equal(decimal.NewFromInt(900)))
			assert.Equal(t, int64(2), balances[0].Version)

			assert.Equal(t, "bal-2", balances[1].ID)
			assert.Equal(t, "@bob", balances[1].Alias)
			assert.True(t, balances[1].Available.Equal(decimal.NewFromInt(600)))
			assert.Equal(t, int64(4), balances[1].Version)

			return nil
		}).Times(1)

	uc := UseCase{
		BalanceRepo: mockBalanceRepo,
	}

	validate := mtransaction.Responses{}

	err := uc.UpdateBalances(context.TODO(), organizationID, ledgerID, validate, balancesBefore, balancesAfter)

	assert.NoError(t, err)
}

func TestUpdateBalances_FallbackPath_NilAfter(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	organizationID := uuid.Must(libCommons.GenerateUUIDv7())
	ledgerID := uuid.Must(libCommons.GenerateUUIDv7())

	balancesBefore := []*mmodel.Balance{
		{
			ID:          "bal-1",
			Alias:       "@alice",
			Available:   decimal.NewFromInt(1000),
			OnHold:      decimal.NewFromInt(0),
			Version:     1,
			AccountType: "deposit",
		},
	}

	mockBalanceRepo := balance.NewMockRepository(ctrl)

	mockBalanceRepo.EXPECT().
		BalancesUpdate(gomock.Any(), organizationID, ledgerID, gomock.Any()).
		DoAndReturn(func(_ context.Context, _, _ uuid.UUID, balances []*mmodel.Balance) error {
			require.Len(t, balances, 1)

			// Fallback: OperateBalances recalculates, version is BEFORE+1
			assert.Equal(t, "bal-1", balances[0].ID)
			assert.True(t, balances[0].Available.Equal(decimal.NewFromInt(900)))
			assert.Equal(t, int64(2), balances[0].Version)

			return nil
		}).Times(1)

	uc := UseCase{
		BalanceRepo: mockBalanceRepo,
	}

	validate := mtransaction.Responses{
		From: map[string]mtransaction.Amount{
			"@alice": {
				Value:           decimal.NewFromInt(100),
				Operation:       "DEBIT",
				TransactionType: "CREATED",
			},
		},
	}

	// nil balancesAfter triggers fallback
	err := uc.UpdateBalances(context.TODO(), organizationID, ledgerID, validate, balancesBefore, nil)

	assert.NoError(t, err)
}

func TestUpdateBalances_PrimaryPath_FailsOnMissingAlias(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	organizationID := uuid.Must(libCommons.GenerateUUIDv7())
	ledgerID := uuid.Must(libCommons.GenerateUUIDv7())

	balancesBefore := []*mmodel.Balance{
		{ID: "bal-1", Alias: "@alice"},
		{ID: "bal-2", Alias: "@bob"},
	}

	// Only @alice has AFTER state; incomplete payload must fail closed.
	balancesAfter := []*mmodel.Balance{
		{Alias: "@alice", Available: decimal.NewFromInt(900), OnHold: decimal.NewFromInt(0), Version: 2},
	}

	mockBalanceRepo := balance.NewMockRepository(ctrl)
	mockBalanceRepo.EXPECT().BalancesUpdate(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Times(0)

	uc := UseCase{BalanceRepo: mockBalanceRepo}

	err := uc.UpdateBalances(context.TODO(), organizationID, ledgerID, mtransaction.Responses{}, balancesBefore, balancesAfter)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "missing AFTER state for alias @bob")
}

func TestUpdateBalances_BalancesUpdateError(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	organizationID := uuid.Must(libCommons.GenerateUUIDv7())
	ledgerID := uuid.Must(libCommons.GenerateUUIDv7())

	balancesBefore := []*mmodel.Balance{
		{ID: "bal-1", Alias: "@alice"},
	}
	balancesAfter := []*mmodel.Balance{
		{Alias: "@alice", Available: decimal.NewFromInt(900), Version: 2},
	}

	mockBalanceRepo := balance.NewMockRepository(ctrl)

	mockBalanceRepo.EXPECT().
		BalancesUpdate(gomock.Any(), organizationID, ledgerID, gomock.Any()).
		Return(errors.New("database connection error")).
		Times(1)

	uc := UseCase{BalanceRepo: mockBalanceRepo}

	err := uc.UpdateBalances(context.TODO(), organizationID, ledgerID, mtransaction.Responses{}, balancesBefore, balancesAfter)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "database connection error")
}
