// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	libCommons "github.com/LerianStudio/lib-commons/v7/commons"
	"github.com/google/uuid"
	goredis "github.com/redis/go-redis/v9"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/balance"
	redis "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/redis/transaction"
	"github.com/LerianStudio/midaz/v4/components/ledger/pkg/readrouting"
	midazpkg "github.com/LerianStudio/midaz/v4/pkg"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
)

// expectDeleteMarkerPlant expects the current and legacy marker writes that precede the funds guard.
func expectDeleteMarkerPlant(m *redis.MockRedisRepository, org, ledger uuid.UUID, b *mmodel.Balance) *gomock.Call {
	current := m.EXPECT().
		SetNX(gomock.Any(), deleteMarkerKeyFor(org, ledger, b), gomock.Any(), time.Duration(balanceDeleteMarkerTTLSeconds)).
		Return(true, nil)
	m.EXPECT().
		SetNX(gomock.Any(), legacyDeleteMarkerKeyFor(org, ledger, b), gomock.Any(), time.Duration(balanceDeleteMarkerTTLSeconds)).
		Return(true, nil)
	return current
}

// expectCacheEvict expects the cache-key Del that follows a committed soft-delete.
func expectCacheEvict(m *redis.MockRedisRepository, org, ledger uuid.UUID, b *mmodel.Balance) *gomock.Call {
	del := m.EXPECT().
		Del(gomock.Any(), balanceCacheKeyFor(org, ledger, b)).
		Return(nil)
	expectMarkerShorten(m, org, ledger, b)
	return del
}

func expectMarkerShorten(m *redis.MockRedisRepository, org, ledger uuid.UUID, b *mmodel.Balance) {
	m.EXPECT().
		ExpireIfValue(gomock.Any(), deleteMarkerKeyFor(org, ledger, b), gomock.Any(), time.Duration(balanceDeleteMarkerShortTTLSeconds)).
		Return(true, nil)
	m.EXPECT().
		ExpireIfValue(gomock.Any(), legacyDeleteMarkerKeyFor(org, ledger, b), gomock.Any(), time.Duration(balanceDeleteMarkerShortTTLSeconds)).
		Return(true, nil)
}

// expectDeleteMarkerRelease expects the ownership-checked marker release that runs only when the delete fails.
func expectDeleteMarkerRelease(m *redis.MockRedisRepository, org, ledger uuid.UUID, b *mmodel.Balance) *gomock.Call {
	current := m.EXPECT().
		DeleteIfValue(gomock.Any(), deleteMarkerKeyFor(org, ledger, b), gomock.Any()).
		Return(true, nil)
	m.EXPECT().
		DeleteIfValue(gomock.Any(), legacyDeleteMarkerKeyFor(org, ledger, b), gomock.Any()).
		Return(true, nil)
	return current
}

func TestDeleteAllBalancesByAccountID(t *testing.T) {
	ctx := context.Background()
	organizationID := uuid.New()
	ledgerID := uuid.New()
	accountID := uuid.New()
	requestID := uuid.Must(libCommons.GenerateUUIDv7())

	t.Run("list balances error", func(t *testing.T) {
		uc, mockBalanceRepo, _ := setupDeleteAllBalancesUseCase(t)
		expectedErr := errors.New("list balances error")

		mockBalanceRepo.EXPECT().
			ListByAccountID(gomock.Any(), organizationID, ledgerID, accountID).
			Return(nil, expectedErr)

		err := uc.DeleteAllBalancesByAccountID(ctx, organizationID, ledgerID, accountID, requestID.String())
		assert.ErrorIs(t, err, expectedErr)
	})

	t.Run("no balances returns nil", func(t *testing.T) {
		uc, mockBalanceRepo, _ := setupDeleteAllBalancesUseCase(t)

		mockBalanceRepo.EXPECT().
			ListByAccountID(gomock.Any(), organizationID, ledgerID, accountID).
			Return([]*mmodel.Balance{}, nil)

		err := uc.DeleteAllBalancesByAccountID(ctx, organizationID, ledgerID, accountID, requestID.String())
		assert.NoError(t, err)
	})

	t.Run("already-owned marker aborts before cache guard", func(t *testing.T) {
		uc, mockBalanceRepo, mockRedisRepo := setupDeleteAllBalancesUseCase(t)
		balanceItem := newTestBalance(decimal.Zero, decimal.Zero)
		markerKey := deleteMarkerKeyFor(organizationID, ledgerID, balanceItem)

		mockBalanceRepo.EXPECT().
			ListByAccountID(gomock.Any(), organizationID, ledgerID, accountID).
			Return([]*mmodel.Balance{balanceItem}, nil)
		mockRedisRepo.EXPECT().
			SetNX(gomock.Any(), markerKey, gomock.Any(), time.Duration(balanceDeleteMarkerTTLSeconds)).
			Return(false, nil)
		mockRedisRepo.EXPECT().
			DeleteIfValue(gomock.Any(), markerKey, gomock.Any()).
			Times(0)
		mockRedisRepo.EXPECT().
			ListBalanceByKey(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
			Times(0)
		mockBalanceRepo.EXPECT().
			UpdateAllByAccountID(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
			Times(0)
		mockBalanceRepo.EXPECT().
			DeleteAllByIDs(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
			Times(0)

		err := uc.DeleteAllBalancesByAccountID(ctx, organizationID, ledgerID, accountID, requestID.String())

		var conflictErr midazpkg.EntityConflictError
		assert.Error(t, err)
		assert.True(t, errors.As(err, &conflictErr))
		assert.Equal(t, constant.ErrBalancesCantBeDeleted.Error(), conflictErr.Code)
	})

	t.Run("marker redis error aborts before cache guard", func(t *testing.T) {
		uc, mockBalanceRepo, mockRedisRepo := setupDeleteAllBalancesUseCase(t)
		balanceItem := newTestBalance(decimal.Zero, decimal.Zero)
		markerKey := deleteMarkerKeyFor(organizationID, ledgerID, balanceItem)
		expectedErr := errors.New("marker redis unavailable")

		mockBalanceRepo.EXPECT().
			ListByAccountID(gomock.Any(), organizationID, ledgerID, accountID).
			Return([]*mmodel.Balance{balanceItem}, nil)
		mockRedisRepo.EXPECT().
			SetNX(gomock.Any(), markerKey, gomock.Any(), time.Duration(balanceDeleteMarkerTTLSeconds)).
			Return(false, expectedErr)
		mockRedisRepo.EXPECT().
			DeleteIfValue(gomock.Any(), markerKey, gomock.Any()).
			Times(0)
		mockRedisRepo.EXPECT().
			ListBalanceByKey(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
			Times(0)
		mockBalanceRepo.EXPECT().
			UpdateAllByAccountID(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
			Times(0)
		mockBalanceRepo.EXPECT().
			DeleteAllByIDs(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
			Times(0)

		err := uc.DeleteAllBalancesByAccountID(ctx, organizationID, ledgerID, accountID, requestID.String())
		assert.ErrorIs(t, err, expectedErr)
	})

	t.Run("partial marker acquisition rolls back only owned marker", func(t *testing.T) {
		uc, mockBalanceRepo, mockRedisRepo := setupDeleteAllBalancesUseCase(t)
		first := newTestBalanceWithIdentity(decimal.Zero, decimal.Zero, "first", "default")
		second := newTestBalanceWithIdentity(decimal.Zero, decimal.Zero, "second", "default")
		firstMarker := deleteMarkerKeyFor(organizationID, ledgerID, first)
		secondMarker := deleteMarkerKeyFor(organizationID, ledgerID, second)
		expectedErr := errors.New("marker redis unavailable")

		mockBalanceRepo.EXPECT().
			ListByAccountID(gomock.Any(), organizationID, ledgerID, accountID).
			Return([]*mmodel.Balance{first, second}, nil)
		firstSet := mockRedisRepo.EXPECT().
			SetNX(gomock.Any(), firstMarker, gomock.Any(), time.Duration(balanceDeleteMarkerTTLSeconds)).
			Return(true, nil)
		firstLegacySet := mockRedisRepo.EXPECT().
			SetNX(gomock.Any(), legacyDeleteMarkerKeyFor(organizationID, ledgerID, first), gomock.Any(), time.Duration(balanceDeleteMarkerTTLSeconds)).
			Return(true, nil)
		secondSet := mockRedisRepo.EXPECT().
			SetNX(gomock.Any(), secondMarker, gomock.Any(), time.Duration(balanceDeleteMarkerTTLSeconds)).
			Return(false, expectedErr)
		rollback := mockRedisRepo.EXPECT().
			DeleteIfValue(gomock.Any(), firstMarker, gomock.Any()).
			Return(true, nil)
		legacyRollback := mockRedisRepo.EXPECT().
			DeleteIfValue(gomock.Any(), legacyDeleteMarkerKeyFor(organizationID, ledgerID, first), gomock.Any()).
			Return(true, nil)
		gomock.InOrder(firstSet, firstLegacySet, secondSet, rollback, legacyRollback)
		mockRedisRepo.EXPECT().
			DeleteIfValue(gomock.Any(), secondMarker, gomock.Any()).
			Times(0)
		mockRedisRepo.EXPECT().
			ListBalanceByKey(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
			Times(0)
		mockBalanceRepo.EXPECT().
			UpdateAllByAccountID(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
			Times(0)
		mockBalanceRepo.EXPECT().
			DeleteAllByIDs(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
			Times(0)

		err := uc.DeleteAllBalancesByAccountID(ctx, organizationID, ledgerID, accountID, requestID.String())
		assert.ErrorIs(t, err, expectedErr)
	})

	t.Run("redis lookup error", func(t *testing.T) {
		uc, mockBalanceRepo, mockRedisRepo := setupDeleteAllBalancesUseCase(t)
		expectedErr := errors.New("redis error")
		balanceItem := newTestBalance(decimal.NewFromInt(1), decimal.Zero)

		mockBalanceRepo.EXPECT().
			ListByAccountID(gomock.Any(), organizationID, ledgerID, accountID).
			Return([]*mmodel.Balance{balanceItem}, nil)
		expectDeleteMarkerPlant(mockRedisRepo, organizationID, ledgerID, balanceItem)
		mockRedisRepo.EXPECT().
			ListBalanceByKey(gomock.Any(), organizationID, ledgerID, balanceRedisKey(balanceItem)).
			Return(nil, expectedErr)
		expectDeleteMarkerRelease(mockRedisRepo, organizationID, ledgerID, balanceItem)

		err := uc.DeleteAllBalancesByAccountID(ctx, organizationID, ledgerID, accountID, requestID.String())
		assert.ErrorIs(t, err, expectedErr)
	})

	t.Run("redis zero-funds balance present proceeds to deletion", func(t *testing.T) {
		uc, mockBalanceRepo, mockRedisRepo := setupDeleteAllBalancesUseCase(t)
		balanceItem := newTestBalance(decimal.Zero, decimal.Zero)

		mockBalanceRepo.EXPECT().
			ListByAccountID(gomock.Any(), organizationID, ledgerID, accountID).
			Return([]*mmodel.Balance{balanceItem}, nil)
		expectDeleteMarkerPlant(mockRedisRepo, organizationID, ledgerID, balanceItem)
		mockRedisRepo.EXPECT().
			ListBalanceByKey(gomock.Any(), organizationID, ledgerID, balanceRedisKey(balanceItem)).
			Return(&mmodel.Balance{}, nil)
		mockBalanceRepo.EXPECT().
			UpdateAllByAccountID(gomock.Any(), organizationID, ledgerID, accountID, gomock.Any()).
			Return(nil)
		mockBalanceRepo.EXPECT().
			DeleteAllByIDs(gomock.Any(), organizationID, ledgerID, gomock.Any()).
			Return(nil)
		expectCacheEvict(mockRedisRepo, organizationID, ledgerID, balanceItem)

		err := uc.DeleteAllBalancesByAccountID(ctx, organizationID, ledgerID, accountID, requestID.String())
		assert.NoError(t, err)
	})

	t.Run("balances with funds remaining prevent deletion", func(t *testing.T) {
		uc, mockBalanceRepo, mockRedisRepo := setupDeleteAllBalancesUseCase(t)
		balanceItem := newTestBalance(decimal.NewFromInt(10), decimal.Zero)

		mockBalanceRepo.EXPECT().
			ListByAccountID(gomock.Any(), organizationID, ledgerID, accountID).
			Return([]*mmodel.Balance{balanceItem}, nil)
		expectDeleteMarkerPlant(mockRedisRepo, organizationID, ledgerID, balanceItem)
		mockRedisRepo.EXPECT().
			ListBalanceByKey(gomock.Any(), organizationID, ledgerID, balanceRedisKey(balanceItem)).
			Return(nil, nil)
		expectDeleteMarkerRelease(mockRedisRepo, organizationID, ledgerID, balanceItem)

		err := uc.DeleteAllBalancesByAccountID(ctx, organizationID, ledgerID, accountID, requestID.String())

		var conflictErr midazpkg.EntityConflictError
		assert.Error(t, err)
		assert.True(t, errors.As(err, &conflictErr))
		assert.Equal(t, constant.ErrBalancesCantBeDeleted.Error(), conflictErr.Code)
	})

	t.Run("overdraft debt prevents deletion", func(t *testing.T) {
		uc, mockBalanceRepo, mockRedisRepo := setupDeleteAllBalancesUseCase(t)
		balanceItem := newTestBalanceWithOverdraft(decimal.Zero, decimal.Zero, decimal.NewFromInt(25))

		mockBalanceRepo.EXPECT().
			ListByAccountID(gomock.Any(), organizationID, ledgerID, accountID).
			Return([]*mmodel.Balance{balanceItem}, nil)
		expectDeleteMarkerPlant(mockRedisRepo, organizationID, ledgerID, balanceItem)
		mockRedisRepo.EXPECT().
			ListBalanceByKey(gomock.Any(), organizationID, ledgerID, balanceRedisKey(balanceItem)).
			Return(nil, nil)
		expectDeleteMarkerRelease(mockRedisRepo, organizationID, ledgerID, balanceItem)

		err := uc.DeleteAllBalancesByAccountID(ctx, organizationID, ledgerID, accountID, requestID.String())

		var conflictErr midazpkg.EntityConflictError
		assert.Error(t, err)
		assert.True(t, errors.As(err, &conflictErr))
		assert.Equal(t, constant.ErrBalancesCantBeDeleted.Error(), conflictErr.Code)
	})

	t.Run("toggle balance transfers error", func(t *testing.T) {
		uc, mockBalanceRepo, mockRedisRepo := setupDeleteAllBalancesUseCase(t)
		balanceItem := newTestBalance(decimal.Zero, decimal.Zero)
		expectedErr := errors.New("update permissions error")

		mockBalanceRepo.EXPECT().
			ListByAccountID(gomock.Any(), organizationID, ledgerID, accountID).
			Return([]*mmodel.Balance{balanceItem}, nil)
		expectDeleteMarkerPlant(mockRedisRepo, organizationID, ledgerID, balanceItem)
		mockRedisRepo.EXPECT().
			ListBalanceByKey(gomock.Any(), organizationID, ledgerID, balanceRedisKey(balanceItem)).
			Return(nil, nil)

		firstCall := mockBalanceRepo.EXPECT().
			UpdateAllByAccountID(gomock.Any(), organizationID, ledgerID, accountID, gomock.Any()).
			DoAndReturn(func(_ context.Context, _, _, _ uuid.UUID, update mmodel.UpdateBalance) error {
				assert.NotNil(t, update.AllowReceiving)
				assert.NotNil(t, update.AllowSending)
				assert.False(t, *update.AllowReceiving)
				assert.False(t, *update.AllowSending)
				return expectedErr
			})
		secondCall := mockBalanceRepo.EXPECT().
			UpdateAllByAccountID(gomock.Any(), organizationID, ledgerID, accountID, gomock.Any()).
			DoAndReturn(func(_ context.Context, _, _, _ uuid.UUID, update mmodel.UpdateBalance) error {
				assert.NotNil(t, update.AllowReceiving)
				assert.NotNil(t, update.AllowSending)
				assert.True(t, *update.AllowReceiving)
				assert.True(t, *update.AllowSending)
				return nil
			})
		gomock.InOrder(firstCall, secondCall)
		expectDeleteMarkerRelease(mockRedisRepo, organizationID, ledgerID, balanceItem)

		err := uc.DeleteAllBalancesByAccountID(ctx, organizationID, ledgerID, accountID, requestID.String())
		assert.ErrorIs(t, err, expectedErr)
	})

	t.Run("delete balances error rolls back transfers", func(t *testing.T) {
		uc, mockBalanceRepo, mockRedisRepo := setupDeleteAllBalancesUseCase(t)
		balanceItem := newTestBalance(decimal.Zero, decimal.Zero)
		expectedErr := errors.New("delete balances error")

		mockBalanceRepo.EXPECT().
			ListByAccountID(gomock.Any(), organizationID, ledgerID, accountID).
			Return([]*mmodel.Balance{balanceItem}, nil)
		expectDeleteMarkerPlant(mockRedisRepo, organizationID, ledgerID, balanceItem)
		mockRedisRepo.EXPECT().
			ListBalanceByKey(gomock.Any(), organizationID, ledgerID, balanceRedisKey(balanceItem)).
			Return(nil, nil)

		mockBalanceRepo.EXPECT().
			UpdateAllByAccountID(gomock.Any(), organizationID, ledgerID, accountID, gomock.Any()).
			DoAndReturn(func(_ context.Context, _, _, _ uuid.UUID, update mmodel.UpdateBalance) error {
				assert.False(t, *update.AllowReceiving)
				assert.False(t, *update.AllowSending)
				return nil
			})
		mockBalanceRepo.EXPECT().
			DeleteAllByIDs(gomock.Any(), organizationID, ledgerID, gomock.Any()).
			Return(expectedErr)
		mockBalanceRepo.EXPECT().
			UpdateAllByAccountID(gomock.Any(), organizationID, ledgerID, accountID, gomock.Any()).
			DoAndReturn(func(_ context.Context, _, _, _ uuid.UUID, update mmodel.UpdateBalance) error {
				assert.True(t, *update.AllowReceiving)
				assert.True(t, *update.AllowSending)
				return nil
			})
		expectDeleteMarkerRelease(mockRedisRepo, organizationID, ledgerID, balanceItem)

		err := uc.DeleteAllBalancesByAccountID(ctx, organizationID, ledgerID, accountID, requestID.String())
		assert.ErrorIs(t, err, expectedErr)
	})

	t.Run("successfully deletes balances", func(t *testing.T) {
		uc, mockBalanceRepo, mockRedisRepo := setupDeleteAllBalancesUseCase(t)
		balanceItem := newTestBalance(decimal.Zero, decimal.Zero)
		expectedID := uuid.MustParse(balanceItem.ID)

		mockBalanceRepo.EXPECT().
			ListByAccountID(gomock.Any(), organizationID, ledgerID, accountID).
			Return([]*mmodel.Balance{balanceItem}, nil)
		expectDeleteMarkerPlant(mockRedisRepo, organizationID, ledgerID, balanceItem)
		mockRedisRepo.EXPECT().
			ListBalanceByKey(gomock.Any(), organizationID, ledgerID, balanceRedisKey(balanceItem)).
			Return(nil, nil)
		mockBalanceRepo.EXPECT().
			UpdateAllByAccountID(gomock.Any(), organizationID, ledgerID, accountID, gomock.Any()).
			DoAndReturn(func(_ context.Context, _, _, _ uuid.UUID, update mmodel.UpdateBalance) error {
				assert.False(t, *update.AllowReceiving)
				assert.False(t, *update.AllowSending)
				return nil
			})
		mockBalanceRepo.EXPECT().
			DeleteAllByIDs(gomock.Any(), organizationID, ledgerID, gomock.Any()).
			DoAndReturn(func(_ context.Context, _, _ uuid.UUID, ids []uuid.UUID) error {
				assert.Len(t, ids, 1)
				assert.Equal(t, expectedID, ids[0])
				return nil
			})
		expectCacheEvict(mockRedisRepo, organizationID, ledgerID, balanceItem)

		err := uc.DeleteAllBalancesByAccountID(ctx, organizationID, ledgerID, accountID, requestID.String())
		assert.NoError(t, err)
	})
}

func TestDeleteAllBalancesByAccountID_InitialReadUsesPrimaryIntent(t *testing.T) {
	organizationID := uuid.New()
	ledgerID := uuid.New()
	accountID := uuid.New()
	requestID := uuid.Must(libCommons.GenerateUUIDv7())

	uc, mockBalanceRepo, _ := setupDeleteAllBalancesUseCase(t)
	var observedCtx context.Context

	mockBalanceRepo.EXPECT().
		ListByAccountID(gomock.Any(), organizationID, ledgerID, accountID).
		DoAndReturn(func(ctx context.Context, _, _, _ uuid.UUID) ([]*mmodel.Balance, error) {
			observedCtx = ctx

			return nil, nil
		})

	err := uc.DeleteAllBalancesByAccountID(context.Background(), organizationID, ledgerID, accountID, requestID.String())

	assert.NoError(t, err)
	assert.True(t, readrouting.IsPrimaryRead(observedCtx),
		"the initial balance read must carry the primary-read intent")
}

// TestDeleteAllBalancesByAccountIDBlockThenEvict locks the block-then-evict ordering: the
// delete marker is planted BEFORE the funds guard reads, the cache is evicted AFTER the soft
// delete commits, the delete marker is released ONLY when the delete fails, and a failed eviction
// never fails an already-committed delete.
func TestDeleteAllBalancesByAccountIDBlockThenEvict(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	organizationID := uuid.New()
	ledgerID := uuid.New()
	accountID := uuid.New()
	requestID := uuid.Must(libCommons.GenerateUUIDv7())

	t.Run("plants delete marker before guard and evicts after delete", func(t *testing.T) {
		t.Parallel()

		uc, mockBalanceRepo, mockRedisRepo := setupDeleteAllBalancesUseCase(t)
		balanceItem := newTestBalance(decimal.Zero, decimal.Zero)

		mockBalanceRepo.EXPECT().
			ListByAccountID(gomock.Any(), organizationID, ledgerID, accountID).
			Return([]*mmodel.Balance{balanceItem}, nil)

		plant := mockRedisRepo.EXPECT().
			SetNX(gomock.Any(), deleteMarkerKeyFor(organizationID, ledgerID, balanceItem), gomock.Any(), time.Duration(balanceDeleteMarkerTTLSeconds)).
			Return(true, nil)
		legacyPlant := mockRedisRepo.EXPECT().
			SetNX(gomock.Any(), legacyDeleteMarkerKeyFor(organizationID, ledgerID, balanceItem), gomock.Any(), time.Duration(balanceDeleteMarkerTTLSeconds)).
			Return(true, nil)
		guard := mockRedisRepo.EXPECT().
			ListBalanceByKey(gomock.Any(), organizationID, ledgerID, balanceRedisKey(balanceItem)).
			Return(&mmodel.Balance{}, nil)
		mockBalanceRepo.EXPECT().
			UpdateAllByAccountID(gomock.Any(), organizationID, ledgerID, accountID, gomock.Any()).
			Return(nil)
		del := mockBalanceRepo.EXPECT().
			DeleteAllByIDs(gomock.Any(), organizationID, ledgerID, gomock.Any()).
			Return(nil)
		evict := mockRedisRepo.EXPECT().
			Del(gomock.Any(), balanceCacheKeyFor(organizationID, ledgerID, balanceItem)).
			Return(nil)
		expectMarkerShorten(mockRedisRepo, organizationID, ledgerID, balanceItem)

		// delete marker-before-guard and evict-after-soft-delete.
		gomock.InOrder(plant, legacyPlant, guard)
		gomock.InOrder(del, evict)

		err := uc.DeleteAllBalancesByAccountID(ctx, organizationID, ledgerID, accountID, requestID.String())
		assert.NoError(t, err)
		// On success the delete marker release must NOT run: no Del of the delete marker key is set up,
		// so the strict controller would fail if the release closure fired.
	})

	t.Run("delete error releases delete marker and skips evict", func(t *testing.T) {
		t.Parallel()

		uc, mockBalanceRepo, mockRedisRepo := setupDeleteAllBalancesUseCase(t)
		balanceItem := newTestBalance(decimal.Zero, decimal.Zero)
		expectedErr := errors.New("delete balances error")

		mockBalanceRepo.EXPECT().
			ListByAccountID(gomock.Any(), organizationID, ledgerID, accountID).
			Return([]*mmodel.Balance{balanceItem}, nil)
		expectDeleteMarkerPlant(mockRedisRepo, organizationID, ledgerID, balanceItem)
		mockRedisRepo.EXPECT().
			ListBalanceByKey(gomock.Any(), organizationID, ledgerID, balanceRedisKey(balanceItem)).
			Return(&mmodel.Balance{}, nil)
		// Flip to false before delete, then rollback to true after delete fails.
		mockBalanceRepo.EXPECT().
			UpdateAllByAccountID(gomock.Any(), organizationID, ledgerID, accountID, gomock.Any()).
			Return(nil).
			Times(2)
		mockBalanceRepo.EXPECT().
			DeleteAllByIDs(gomock.Any(), organizationID, ledgerID, gomock.Any()).
			Return(expectedErr)
		// Release Dels the delete marker key; the cache key is never evicted on the error path.
		expectDeleteMarkerRelease(mockRedisRepo, organizationID, ledgerID, balanceItem)

		err := uc.DeleteAllBalancesByAccountID(ctx, organizationID, ledgerID, accountID, requestID.String())
		assert.ErrorIs(t, err, expectedErr)
	})

	t.Run("evict del failure on success is non-fatal", func(t *testing.T) {
		t.Parallel()

		uc, mockBalanceRepo, mockRedisRepo := setupDeleteAllBalancesUseCase(t)
		balanceItem := newTestBalance(decimal.Zero, decimal.Zero)

		mockBalanceRepo.EXPECT().
			ListByAccountID(gomock.Any(), organizationID, ledgerID, accountID).
			Return([]*mmodel.Balance{balanceItem}, nil)
		expectDeleteMarkerPlant(mockRedisRepo, organizationID, ledgerID, balanceItem)
		mockRedisRepo.EXPECT().
			ListBalanceByKey(gomock.Any(), organizationID, ledgerID, balanceRedisKey(balanceItem)).
			Return(&mmodel.Balance{}, nil)
		mockBalanceRepo.EXPECT().
			UpdateAllByAccountID(gomock.Any(), organizationID, ledgerID, accountID, gomock.Any()).
			Return(nil)
		mockBalanceRepo.EXPECT().
			DeleteAllByIDs(gomock.Any(), organizationID, ledgerID, gomock.Any()).
			Return(nil)
		mockRedisRepo.EXPECT().
			Del(gomock.Any(), balanceCacheKeyFor(organizationID, ledgerID, balanceItem)).
			Return(errors.New("evict failed"))

		err := uc.DeleteAllBalancesByAccountID(ctx, organizationID, ledgerID, accountID, requestID.String())
		assert.NoError(t, err)
	})
}

// TestDeleteAllBalancesByAccountIDCacheMissFundsGuard covers the funds guard for both a
// Redis cache hit and a cache miss: a cached balance blocks deletion only when it still
// holds funds, and a balance absent from Redis (TTL expired) must still be checked against
// the authoritative Postgres row so accounts holding funds are never soft-deleted.
func TestDeleteAllBalancesByAccountIDCacheMissFundsGuard(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	organizationID := uuid.New()
	ledgerID := uuid.New()
	accountID := uuid.New()
	requestID := uuid.Must(libCommons.GenerateUUIDv7())

	tests := []struct {
		name          string
		balance       *mmodel.Balance
		cacheBalance  *mmodel.Balance
		cacheErr      error
		expectProceed bool
	}{
		{
			name:          "on_hold funds with cache miss prevents deletion",
			balance:       newTestBalance(decimal.Zero, decimal.NewFromInt(5)),
			cacheBalance:  nil,
			cacheErr:      goredis.Nil,
			expectProceed: false,
		},
		{
			name:          "available funds with cache miss prevents deletion",
			balance:       newTestBalance(decimal.NewFromInt(7), decimal.Zero),
			cacheBalance:  nil,
			cacheErr:      goredis.Nil,
			expectProceed: false,
		},
		{
			name:          "cached zero-funds balance proceeds to deletion",
			balance:       newTestBalance(decimal.Zero, decimal.Zero),
			cacheBalance:  &mmodel.Balance{},
			cacheErr:      nil,
			expectProceed: true,
		},
		{
			name:          "zero funds with cache miss proceeds to deletion",
			balance:       newTestBalance(decimal.Zero, decimal.Zero),
			cacheBalance:  nil,
			cacheErr:      goredis.Nil,
			expectProceed: true,
		},
		{
			name:          "cached available funds prevents deletion",
			balance:       newTestBalance(decimal.Zero, decimal.Zero),
			cacheBalance:  newTestBalance(decimal.NewFromInt(7), decimal.Zero),
			cacheErr:      nil,
			expectProceed: false,
		},
		{
			name:          "cached on_hold funds prevents deletion",
			balance:       newTestBalance(decimal.Zero, decimal.Zero),
			cacheBalance:  newTestBalance(decimal.Zero, decimal.NewFromInt(9)),
			cacheErr:      nil,
			expectProceed: false,
		},
		{
			name:          "cached overdraft debt prevents deletion",
			balance:       newTestBalance(decimal.Zero, decimal.Zero),
			cacheBalance:  newTestBalanceWithOverdraft(decimal.Zero, decimal.Zero, decimal.NewFromInt(25)),
			cacheErr:      nil,
			expectProceed: false,
		},
		{
			name:          "cached zero-funds but postgres funds prevents deletion",
			balance:       newTestBalance(decimal.NewFromInt(3), decimal.Zero),
			cacheBalance:  &mmodel.Balance{},
			cacheErr:      nil,
			expectProceed: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			uc, mockBalanceRepo, mockRedisRepo := setupDeleteAllBalancesUseCase(t)

			mockBalanceRepo.EXPECT().
				ListByAccountID(gomock.Any(), organizationID, ledgerID, accountID).
				Return([]*mmodel.Balance{tt.balance}, nil)
			expectDeleteMarkerPlant(mockRedisRepo, organizationID, ledgerID, tt.balance)
			mockRedisRepo.EXPECT().
				ListBalanceByKey(gomock.Any(), organizationID, ledgerID, balanceRedisKey(tt.balance)).
				Return(tt.cacheBalance, tt.cacheErr)

			if tt.expectProceed {
				mockBalanceRepo.EXPECT().
					UpdateAllByAccountID(gomock.Any(), organizationID, ledgerID, accountID, gomock.Any()).
					Return(nil)
				mockBalanceRepo.EXPECT().
					DeleteAllByIDs(gomock.Any(), organizationID, ledgerID, gomock.Any()).
					Return(nil)
				expectCacheEvict(mockRedisRepo, organizationID, ledgerID, tt.balance)
			} else {
				mockBalanceRepo.EXPECT().
					DeleteAllByIDs(gomock.Any(), organizationID, ledgerID, gomock.Any()).
					Times(0)
				expectDeleteMarkerRelease(mockRedisRepo, organizationID, ledgerID, tt.balance)
			}

			err := uc.DeleteAllBalancesByAccountID(ctx, organizationID, ledgerID, accountID, requestID.String())

			if tt.expectProceed {
				assert.NoError(t, err)
				return
			}

			var conflictErr midazpkg.EntityConflictError
			assert.Error(t, err)
			assert.True(t, errors.As(err, &conflictErr))
			assert.Equal(t, constant.ErrBalancesCantBeDeleted.Error(), conflictErr.Code)
		})
	}

	// Guards the loop across MULTIPLE balances: the first balance has zero funds on a cache
	// miss (loop proceeds), the second holds on_hold funds on a cache miss (loop rejects).
	// Deletion must be refused and DeleteAllByIDs must never run.
	t.Run("multiple balances reject when a later balance still holds funds", func(t *testing.T) {
		t.Parallel()

		uc, mockBalanceRepo, mockRedisRepo := setupDeleteAllBalancesUseCase(t)

		zeroFundsBalance := newTestBalanceWithIdentity(decimal.Zero, decimal.Zero, "zero", "default")
		onHoldFundsBalance := newTestBalanceWithIdentity(decimal.Zero, decimal.NewFromInt(5), "on-hold", "default")

		mockBalanceRepo.EXPECT().
			ListByAccountID(gomock.Any(), organizationID, ledgerID, accountID).
			Return([]*mmodel.Balance{zeroFundsBalance, onHoldFundsBalance}, nil)

		// Both delete markers are planted up front, before the guard loop runs.
		expectDeleteMarkerPlant(mockRedisRepo, organizationID, ledgerID, zeroFundsBalance)
		expectDeleteMarkerPlant(mockRedisRepo, organizationID, ledgerID, onHoldFundsBalance)

		firstLookup := mockRedisRepo.EXPECT().
			ListBalanceByKey(gomock.Any(), organizationID, ledgerID, balanceRedisKey(zeroFundsBalance)).
			Return(nil, goredis.Nil)
		secondLookup := mockRedisRepo.EXPECT().
			ListBalanceByKey(gomock.Any(), organizationID, ledgerID, balanceRedisKey(onHoldFundsBalance)).
			Return(nil, goredis.Nil)
		gomock.InOrder(firstLookup, secondLookup)

		mockBalanceRepo.EXPECT().
			DeleteAllByIDs(gomock.Any(), organizationID, ledgerID, gomock.Any()).
			Times(0)

		// On rejection both planted delete markers are released.
		expectDeleteMarkerRelease(mockRedisRepo, organizationID, ledgerID, zeroFundsBalance)
		expectDeleteMarkerRelease(mockRedisRepo, organizationID, ledgerID, onHoldFundsBalance)

		err := uc.DeleteAllBalancesByAccountID(ctx, organizationID, ledgerID, accountID, requestID.String())

		var conflictErr midazpkg.EntityConflictError
		assert.Error(t, err)
		assert.True(t, errors.As(err, &conflictErr))
		assert.Equal(t, constant.ErrBalancesCantBeDeleted.Error(), conflictErr.Code)
	})
}

func setupDeleteAllBalancesUseCase(t *testing.T) (*UseCase, *balance.MockRepository, *redis.MockRedisRepository) {
	t.Helper()

	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	mockBalanceRepo := balance.NewMockRepository(ctrl)
	mockRedisRepo := redis.NewMockRedisRepository(ctrl)

	return &UseCase{
		BalanceRepo:          mockBalanceRepo,
		TransactionRedisRepo: mockRedisRepo,
	}, mockBalanceRepo, mockRedisRepo
}

func newTestBalance(available, onHold decimal.Decimal) *mmodel.Balance {
	return &mmodel.Balance{
		ID:        uuid.New().String(),
		Alias:     "alias",
		Key:       "key",
		Available: available,
		OnHold:    onHold,
	}
}

func newTestBalanceWithOverdraft(available, onHold, overdraftUsed decimal.Decimal) *mmodel.Balance {
	balance := newTestBalance(available, onHold)
	balance.OverdraftUsed = overdraftUsed

	return balance
}

func newTestBalanceWithIdentity(available, onHold decimal.Decimal, alias, key string) *mmodel.Balance {
	balance := newTestBalance(available, onHold)
	balance.Alias = alias
	balance.Key = key

	return balance
}

func balanceRedisKey(b *mmodel.Balance) string {
	return fmt.Sprintf("%s#%s", b.Alias, b.Key)
}

func TestToggleBalanceTransfers(t *testing.T) {
	ctx := context.Background()
	organizationID := uuid.New()
	ledgerID := uuid.New()
	accountID := uuid.New()

	t.Run("successfully toggles transfers", func(t *testing.T) {
		uc, mockBalanceRepo, _ := setupDeleteAllBalancesUseCase(t)

		mockBalanceRepo.EXPECT().
			UpdateAllByAccountID(gomock.Any(), organizationID, ledgerID, accountID, gomock.Any()).
			DoAndReturn(func(_ context.Context, _, _, _ uuid.UUID, update mmodel.UpdateBalance) error {
				assert.NotNil(t, update.AllowReceiving)
				assert.NotNil(t, update.AllowSending)
				assert.True(t, *update.AllowReceiving)
				assert.True(t, *update.AllowSending)
				return nil
			})

		err := uc.toggleBalanceTransfers(ctx, organizationID, ledgerID, accountID, true)
		assert.NoError(t, err)
	})

	t.Run("error triggers rollback with opposite permissions", func(t *testing.T) {
		uc, mockBalanceRepo, _ := setupDeleteAllBalancesUseCase(t)
		expectedErr := errors.New("update permissions error")

		firstCall := mockBalanceRepo.EXPECT().
			UpdateAllByAccountID(gomock.Any(), organizationID, ledgerID, accountID, gomock.Any()).
			DoAndReturn(func(_ context.Context, _, _, _ uuid.UUID, update mmodel.UpdateBalance) error {
				assert.NotNil(t, update.AllowReceiving)
				assert.NotNil(t, update.AllowSending)
				assert.False(t, *update.AllowReceiving)
				assert.False(t, *update.AllowSending)
				return expectedErr
			})
		secondCall := mockBalanceRepo.EXPECT().
			UpdateAllByAccountID(gomock.Any(), organizationID, ledgerID, accountID, gomock.Any()).
			DoAndReturn(func(_ context.Context, _, _, _ uuid.UUID, update mmodel.UpdateBalance) error {
				assert.NotNil(t, update.AllowReceiving)
				assert.NotNil(t, update.AllowSending)
				assert.True(t, *update.AllowReceiving)
				assert.True(t, *update.AllowSending)
				return nil
			})
		gomock.InOrder(firstCall, secondCall)

		err := uc.toggleBalanceTransfers(ctx, organizationID, ledgerID, accountID, false)
		assert.ErrorIs(t, err, expectedErr)
	})
}

func TestUpdateBalanceTransferPermissions(t *testing.T) {
	ctx := context.Background()
	organizationID := uuid.New()
	ledgerID := uuid.New()
	accountID := uuid.New()

	t.Run("successfully updates permissions", func(t *testing.T) {
		uc, mockBalanceRepo, _ := setupDeleteAllBalancesUseCase(t)
		allow := boolPtr(true)

		mockBalanceRepo.EXPECT().
			UpdateAllByAccountID(gomock.Any(), organizationID, ledgerID, accountID, gomock.Any()).
			DoAndReturn(func(_ context.Context, _, _, _ uuid.UUID, update mmodel.UpdateBalance) error {
				assert.Equal(t, allow, update.AllowReceiving)
				assert.Equal(t, allow, update.AllowSending)
				return nil
			})

		err := uc.updateBalanceTransferPermissions(ctx, organizationID, ledgerID, accountID, allow)
		assert.NoError(t, err)
	})

	t.Run("returns error from repository", func(t *testing.T) {
		uc, mockBalanceRepo, _ := setupDeleteAllBalancesUseCase(t)
		allow := boolPtr(false)
		expectedErr := errors.New("update permissions error")

		mockBalanceRepo.EXPECT().
			UpdateAllByAccountID(gomock.Any(), organizationID, ledgerID, accountID, gomock.Any()).
			Return(expectedErr)

		err := uc.updateBalanceTransferPermissions(ctx, organizationID, ledgerID, accountID, allow)
		assert.ErrorIs(t, err, expectedErr)
	})
}

func boolPtr(v bool) *bool {
	return &v
}

// TestDeleteAllBalancesByAccountIDShortensOnlyEvictedMarkers locks the per-balance alignment
// between an eviction and its marker lease: the balance whose cache key survived a failed Del
// keeps its long marker, while the balance that was evicted has both of its markers shortened.
func TestDeleteAllBalancesByAccountIDShortensOnlyEvictedMarkers(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	organizationID := uuid.New()
	ledgerID := uuid.New()
	accountID := uuid.New()
	requestID := uuid.Must(libCommons.GenerateUUIDv7())

	uc, mockBalanceRepo, mockRedisRepo := setupDeleteAllBalancesUseCase(t)

	first := newTestBalanceWithIdentity(decimal.Zero, decimal.Zero, "evict-fails", "default")
	second := newTestBalanceWithIdentity(decimal.Zero, decimal.Zero, "evict-succeeds", "default")

	mockBalanceRepo.EXPECT().
		ListByAccountID(gomock.Any(), organizationID, ledgerID, accountID).
		Return([]*mmodel.Balance{first, second}, nil)
	expectDeleteMarkerPlant(mockRedisRepo, organizationID, ledgerID, first)
	expectDeleteMarkerPlant(mockRedisRepo, organizationID, ledgerID, second)
	mockRedisRepo.EXPECT().
		ListBalanceByKey(gomock.Any(), organizationID, ledgerID, balanceRedisKey(first)).
		Return(nil, goredis.Nil)
	mockRedisRepo.EXPECT().
		ListBalanceByKey(gomock.Any(), organizationID, ledgerID, balanceRedisKey(second)).
		Return(nil, goredis.Nil)
	mockBalanceRepo.EXPECT().
		UpdateAllByAccountID(gomock.Any(), organizationID, ledgerID, accountID, gomock.Any()).
		Return(nil)
	mockBalanceRepo.EXPECT().
		DeleteAllByIDs(gomock.Any(), organizationID, ledgerID, gomock.Any()).
		Return(nil)

	mockRedisRepo.EXPECT().
		Del(gomock.Any(), balanceCacheKeyFor(organizationID, ledgerID, first)).
		Return(errors.New("evict failed"))
	mockRedisRepo.EXPECT().
		Del(gomock.Any(), balanceCacheKeyFor(organizationID, ledgerID, second)).
		Return(nil)

	// A surviving cache snapshot must keep its long marker, so the failed eviction shortens nothing.
	mockRedisRepo.EXPECT().
		ExpireIfValue(gomock.Any(), deleteMarkerKeyFor(organizationID, ledgerID, first), gomock.Any(), gomock.Any()).
		Times(0)
	mockRedisRepo.EXPECT().
		ExpireIfValue(gomock.Any(), legacyDeleteMarkerKeyFor(organizationID, ledgerID, first), gomock.Any(), gomock.Any()).
		Times(0)
	expectMarkerShorten(mockRedisRepo, organizationID, ledgerID, second)

	err := uc.DeleteAllBalancesByAccountID(ctx, organizationID, ledgerID, accountID, requestID.String())
	assert.NoError(t, err)
}

// TestDeleteAllBalancesByAccountIDMalformedCachedOverdraftFailsClosed locks the cascade half of
// the shared rule: an unreadable cached OverdraftUsed is reported by ListBalanceByKey and must
// abort the delete with the markers released, never be flattened to zero debt.
func TestDeleteAllBalancesByAccountIDMalformedCachedOverdraftFailsClosed(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	organizationID := uuid.New()
	ledgerID := uuid.New()
	accountID := uuid.New()
	requestID := uuid.Must(libCommons.GenerateUUIDv7())

	uc, mockBalanceRepo, mockRedisRepo := setupDeleteAllBalancesUseCase(t)
	balanceItem := newTestBalance(decimal.Zero, decimal.Zero)
	expectedErr := errors.New("failed to parse overdraft used from balance cache: can't convert not-a-decimal to decimal")

	mockBalanceRepo.EXPECT().
		ListByAccountID(gomock.Any(), organizationID, ledgerID, accountID).
		Return([]*mmodel.Balance{balanceItem}, nil)
	expectDeleteMarkerPlant(mockRedisRepo, organizationID, ledgerID, balanceItem)
	mockRedisRepo.EXPECT().
		ListBalanceByKey(gomock.Any(), organizationID, ledgerID, balanceRedisKey(balanceItem)).
		Return(nil, expectedErr)
	expectDeleteMarkerRelease(mockRedisRepo, organizationID, ledgerID, balanceItem)
	mockBalanceRepo.EXPECT().
		UpdateAllByAccountID(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Times(0)
	mockBalanceRepo.EXPECT().
		DeleteAllByIDs(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Times(0)

	err := uc.DeleteAllBalancesByAccountID(ctx, organizationID, ledgerID, accountID, requestID.String())
	assert.ErrorIs(t, err, expectedErr)
}
