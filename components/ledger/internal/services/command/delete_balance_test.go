// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/balance"
	redis "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/redis/transaction"
	"github.com/LerianStudio/midaz/v4/components/ledger/pkg/readrouting"
	midazpkg "github.com/LerianStudio/midaz/v4/pkg"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
)

func TestDeleteBalance(t *testing.T) {
	ctx := context.Background()
	organizationID := uuid.New()
	ledgerID := uuid.New()
	balanceID := uuid.New()

	t.Run("find balance error", func(t *testing.T) {
		uc, mockBalanceRepo, _ := setupDeleteBalanceUseCase(t)
		expectedErr := errors.New("database connection error")

		mockBalanceRepo.EXPECT().
			Find(gomock.Any(), organizationID, ledgerID, balanceID).
			Return(nil, expectedErr)

		err := uc.DeleteBalance(ctx, organizationID, ledgerID, balanceID)

		assert.Error(t, err)
		assert.Equal(t, expectedErr, err)
	})

	t.Run("balance with funds cannot be deleted", func(t *testing.T) {
		cases := []struct {
			name          string
			available     decimal.Decimal
			onHold        decimal.Decimal
			overdraftUsed decimal.Decimal
		}{
			{name: "available only", available: decimal.NewFromInt(100)},
			{name: "on-hold only", onHold: decimal.NewFromInt(50)},
			{name: "overdraft used", overdraftUsed: decimal.NewFromInt(25)},
		}

		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				uc, mockBalanceRepo, mockRedisRepo := setupDeleteBalanceUseCase(t)
				bal := &mmodel.Balance{
					ID:            balanceID.String(),
					Alias:         "alias",
					Key:           "key",
					Available:     tc.available,
					OnHold:        tc.onHold,
					OverdraftUsed: tc.overdraftUsed,
				}

				mockBalanceRepo.EXPECT().
					Find(gomock.Any(), organizationID, ledgerID, balanceID).
					Return(bal, nil)
				// Delete marker is planted before the funds guard; a rejected delete releases it.
				expectDeleteMarkerPlant(mockRedisRepo, organizationID, ledgerID, bal)
				expectDeleteMarkerRelease(mockRedisRepo, organizationID, ledgerID, bal)

				err := uc.DeleteBalance(ctx, organizationID, ledgerID, balanceID)

				var conflictErr midazpkg.EntityConflictError
				assert.True(t, errors.As(err, &conflictErr))
				assert.Equal(t, constant.ErrBalancesCantBeDeleted.Error(), conflictErr.Code)
			})
		}
	})

	t.Run("delete error", func(t *testing.T) {
		uc, mockBalanceRepo, mockRedisRepo := setupDeleteBalanceUseCase(t)
		zeroBalance := &mmodel.Balance{
			ID:        balanceID.String(),
			Alias:     "alias",
			Key:       "key",
			Available: decimal.Zero,
			OnHold:    decimal.Zero,
		}
		expectedErr := errors.New("delete failed")

		mockBalanceRepo.EXPECT().
			Find(gomock.Any(), organizationID, ledgerID, balanceID).
			Return(zeroBalance, nil)
		expectDeleteMarkerPlant(mockRedisRepo, organizationID, ledgerID, zeroBalance)
		mockRedisRepo.EXPECT().
			Get(gomock.Any(), balanceCacheKeyFor(organizationID, ledgerID, zeroBalance)).
			Return("", nil)
		mockBalanceRepo.EXPECT().
			Delete(gomock.Any(), organizationID, ledgerID, balanceID).
			Return(expectedErr)
		// A failed delete releases the delete marker and never evicts the cache key.
		expectDeleteMarkerRelease(mockRedisRepo, organizationID, ledgerID, zeroBalance)

		err := uc.DeleteBalance(ctx, organizationID, ledgerID, balanceID)

		assert.Error(t, err)
		assert.Equal(t, expectedErr, err)
	})

	t.Run("nil balance proceeds to delete", func(t *testing.T) {
		uc, mockBalanceRepo, _ := setupDeleteBalanceUseCase(t)

		mockBalanceRepo.EXPECT().
			Find(gomock.Any(), organizationID, ledgerID, balanceID).
			Return(nil, nil)
		mockBalanceRepo.EXPECT().
			Delete(gomock.Any(), organizationID, ledgerID, balanceID).
			Return(nil)

		err := uc.DeleteBalance(ctx, organizationID, ledgerID, balanceID)

		assert.NoError(t, err)
	})

	t.Run("deletes balance with zero funds", func(t *testing.T) {
		uc, mockBalanceRepo, mockRedisRepo := setupDeleteBalanceUseCase(t)
		zeroBalance := &mmodel.Balance{
			ID:        balanceID.String(),
			Alias:     "alias",
			Key:       "key",
			Available: decimal.Zero,
			OnHold:    decimal.Zero,
		}

		mockBalanceRepo.EXPECT().
			Find(gomock.Any(), organizationID, ledgerID, balanceID).
			Return(zeroBalance, nil)
		expectDeleteMarkerPlant(mockRedisRepo, organizationID, ledgerID, zeroBalance)
		mockRedisRepo.EXPECT().
			Get(gomock.Any(), balanceCacheKeyFor(organizationID, ledgerID, zeroBalance)).
			Return("", nil)
		mockBalanceRepo.EXPECT().
			Delete(gomock.Any(), organizationID, ledgerID, balanceID).
			Return(nil)
		expectCacheEvict(mockRedisRepo, organizationID, ledgerID, zeroBalance)

		err := uc.DeleteBalance(ctx, organizationID, ledgerID, balanceID)

		assert.NoError(t, err)
	})
}

func TestDeleteBalance_InitialReadUsesPrimaryIntent(t *testing.T) {
	organizationID := uuid.New()
	ledgerID := uuid.New()
	balanceID := uuid.New()

	uc, mockBalanceRepo, _ := setupDeleteBalanceUseCase(t)
	var observedCtx context.Context

	mockBalanceRepo.EXPECT().
		Find(gomock.Any(), organizationID, ledgerID, balanceID).
		DoAndReturn(func(ctx context.Context, _, _, _ uuid.UUID) (*mmodel.Balance, error) {
			observedCtx = ctx

			return nil, nil
		})
	mockBalanceRepo.EXPECT().
		Delete(gomock.Any(), organizationID, ledgerID, balanceID).
		Return(nil)

	err := uc.DeleteBalance(context.Background(), organizationID, ledgerID, balanceID)

	assert.NoError(t, err)
	assert.True(t, readrouting.IsPrimaryRead(observedCtx),
		"the initial balance read must carry the primary-read intent")
}

// TestDeleteBalanceBlockThenEvict locks the block-then-evict ordering for the single-balance
// delete: the delete marker is planted BEFORE the funds guard (so it fires even when the guard
// rejects), the cache is evicted AFTER the soft delete commits, the delete marker is released
// ONLY when the delete fails, and a failed eviction never fails an already-committed delete.
func TestDeleteBalanceBlockThenEvict(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	organizationID := uuid.New()
	ledgerID := uuid.New()
	balanceID := uuid.New()

	newZeroBalance := func() *mmodel.Balance {
		return &mmodel.Balance{
			ID:        balanceID.String(),
			Alias:     "alias",
			Key:       "key",
			Available: decimal.Zero,
			OnHold:    decimal.Zero,
		}
	}

	t.Run("plants delete marker before delete and evicts after delete", func(t *testing.T) {
		t.Parallel()

		uc, mockBalanceRepo, mockRedisRepo := setupDeleteBalanceUseCase(t)
		bal := newZeroBalance()

		mockBalanceRepo.EXPECT().
			Find(gomock.Any(), organizationID, ledgerID, balanceID).
			Return(bal, nil)
		plant := expectDeleteMarkerPlant(mockRedisRepo, organizationID, ledgerID, bal)
		get := mockRedisRepo.EXPECT().
			Get(gomock.Any(), balanceCacheKeyFor(organizationID, ledgerID, bal)).
			Return("", nil)
		del := mockBalanceRepo.EXPECT().
			Delete(gomock.Any(), organizationID, ledgerID, balanceID).
			Return(nil)
		evict := expectCacheEvict(mockRedisRepo, organizationID, ledgerID, bal)

		// plant-before-delete and evict-after-soft-delete.
		gomock.InOrder(plant, get, del, evict)

		err := uc.DeleteBalance(ctx, organizationID, ledgerID, balanceID)
		assert.NoError(t, err)
		// On success the delete marker release must NOT run: no Del of the delete marker key is set up,
		// so the strict controller fails if the release closure fires.
	})

	t.Run("funds guard rejection releases delete marker and skips evict", func(t *testing.T) {
		t.Parallel()

		uc, mockBalanceRepo, mockRedisRepo := setupDeleteBalanceUseCase(t)
		bal := &mmodel.Balance{
			ID:        balanceID.String(),
			Alias:     "alias",
			Key:       "key",
			Available: decimal.Zero,
			OnHold:    decimal.NewFromInt(5),
		}

		mockBalanceRepo.EXPECT().
			Find(gomock.Any(), organizationID, ledgerID, balanceID).
			Return(bal, nil)
		// SetNX firing on the reject path proves the delete marker is planted BEFORE the funds guard.
		expectDeleteMarkerPlant(mockRedisRepo, organizationID, ledgerID, bal)
		mockBalanceRepo.EXPECT().
			Delete(gomock.Any(), organizationID, ledgerID, balanceID).
			Times(0)
		expectDeleteMarkerRelease(mockRedisRepo, organizationID, ledgerID, bal)

		err := uc.DeleteBalance(ctx, organizationID, ledgerID, balanceID)

		var conflictErr midazpkg.EntityConflictError
		assert.Error(t, err)
		assert.True(t, errors.As(err, &conflictErr))
		assert.Equal(t, constant.ErrBalancesCantBeDeleted.Error(), conflictErr.Code)
	})

	t.Run("delete error releases delete marker and skips evict", func(t *testing.T) {
		t.Parallel()

		uc, mockBalanceRepo, mockRedisRepo := setupDeleteBalanceUseCase(t)
		bal := newZeroBalance()
		expectedErr := errors.New("delete failed")

		mockBalanceRepo.EXPECT().
			Find(gomock.Any(), organizationID, ledgerID, balanceID).
			Return(bal, nil)
		expectDeleteMarkerPlant(mockRedisRepo, organizationID, ledgerID, bal)
		mockRedisRepo.EXPECT().
			Get(gomock.Any(), balanceCacheKeyFor(organizationID, ledgerID, bal)).
			Return("", nil)
		mockBalanceRepo.EXPECT().
			Delete(gomock.Any(), organizationID, ledgerID, balanceID).
			Return(expectedErr)
		// Release Dels the delete marker key; the cache key is never evicted on the error path.
		expectDeleteMarkerRelease(mockRedisRepo, organizationID, ledgerID, bal)

		err := uc.DeleteBalance(ctx, organizationID, ledgerID, balanceID)
		assert.ErrorIs(t, err, expectedErr)
	})

	t.Run("evict del failure on success is non-fatal", func(t *testing.T) {
		t.Parallel()

		uc, mockBalanceRepo, mockRedisRepo := setupDeleteBalanceUseCase(t)
		bal := newZeroBalance()

		mockBalanceRepo.EXPECT().
			Find(gomock.Any(), organizationID, ledgerID, balanceID).
			Return(bal, nil)
		expectDeleteMarkerPlant(mockRedisRepo, organizationID, ledgerID, bal)
		mockRedisRepo.EXPECT().
			Get(gomock.Any(), balanceCacheKeyFor(organizationID, ledgerID, bal)).
			Return("", nil)
		mockBalanceRepo.EXPECT().
			Delete(gomock.Any(), organizationID, ledgerID, balanceID).
			Return(nil)
		mockRedisRepo.EXPECT().
			Del(gomock.Any(), balanceCacheKeyFor(organizationID, ledgerID, bal)).
			Return(errors.New("evict failed"))

		err := uc.DeleteBalance(ctx, organizationID, ledgerID, balanceID)
		assert.NoError(t, err)
	})

	t.Run("redis funds reject deletion and release marker", func(t *testing.T) {
		cases := []struct {
			name          string
			available     decimal.Decimal
			onHold        decimal.Decimal
			overdraftUsed string
		}{
			{name: "available", available: decimal.NewFromInt(50)},
			{name: "negative available", available: decimal.NewFromInt(-1)},
			{name: "on hold", onHold: decimal.NewFromInt(5)},
			{name: "negative on hold", onHold: decimal.NewFromInt(-2)},
			{name: "overdraft used", overdraftUsed: "25"},
		}

		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				uc, mockBalanceRepo, mockRedisRepo := setupDeleteBalanceUseCase(t)
				overdraftUsed := tc.overdraftUsed
				if overdraftUsed == "" {
					overdraftUsed = "0"
				}

				bal := &mmodel.Balance{
					ID:        balanceID.String(),
					Alias:     "alias",
					Key:       "key",
					Available: decimal.Zero,
					OnHold:    decimal.Zero,
				}

				mockBalanceRepo.EXPECT().
					Find(gomock.Any(), organizationID, ledgerID, balanceID).
					Return(bal, nil)
				plant := expectDeleteMarkerPlant(mockRedisRepo, organizationID, ledgerID, bal)
				get := mockRedisRepo.EXPECT().
					Get(gomock.Any(), balanceCacheKeyFor(organizationID, ledgerID, bal)).
					Return(balanceRedisSnapshotWithOverdraft(tc.available, tc.onHold, overdraftUsed), nil)
				release := expectDeleteMarkerRelease(mockRedisRepo, organizationID, ledgerID, bal)
				gomock.InOrder(plant, get, release)
				mockBalanceRepo.EXPECT().
					Delete(gomock.Any(), organizationID, ledgerID, balanceID).
					Times(0)

				err := uc.DeleteBalance(ctx, organizationID, ledgerID, balanceID)

				var conflictErr midazpkg.EntityConflictError
				assert.Error(t, err)
				assert.True(t, errors.As(err, &conflictErr))
				assert.Equal(t, constant.ErrBalancesCantBeDeleted.Error(), conflictErr.Code)
			})
		}
	})

	t.Run("invalid cached overdraft fails closed and releases marker", func(t *testing.T) {
		uc, mockBalanceRepo, mockRedisRepo := setupDeleteBalanceUseCase(t)
		bal := &mmodel.Balance{ID: balanceID.String(), Alias: "alias", Key: "key"}

		mockBalanceRepo.EXPECT().
			Find(gomock.Any(), organizationID, ledgerID, balanceID).
			Return(bal, nil)
		expectDeleteMarkerPlant(mockRedisRepo, organizationID, ledgerID, bal)
		mockRedisRepo.EXPECT().
			Get(gomock.Any(), balanceCacheKeyFor(organizationID, ledgerID, bal)).
			Return(balanceRedisSnapshotWithOverdraft(decimal.Zero, decimal.Zero, "not-a-decimal"), nil)
		expectDeleteMarkerRelease(mockRedisRepo, organizationID, ledgerID, bal)
		mockBalanceRepo.EXPECT().
			Delete(gomock.Any(), organizationID, ledgerID, balanceID).
			Times(0)

		err := uc.DeleteBalance(ctx, organizationID, ledgerID, balanceID)

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to parse overdraft used")
	})

	t.Run("redis cache zero snapshot permits deletion", func(t *testing.T) {
		uc, mockBalanceRepo, mockRedisRepo := setupDeleteBalanceUseCase(t)
		bal := &mmodel.Balance{
			ID:        balanceID.String(),
			Alias:     "alias",
			Key:       "key",
			Available: decimal.Zero,
			OnHold:    decimal.Zero,
		}

		mockBalanceRepo.EXPECT().
			Find(gomock.Any(), organizationID, ledgerID, balanceID).
			Return(bal, nil)
		expectDeleteMarkerPlant(mockRedisRepo, organizationID, ledgerID, bal)
		mockRedisRepo.EXPECT().
			Get(gomock.Any(), balanceCacheKeyFor(organizationID, ledgerID, bal)).
			Return(balanceRedisSnapshot(decimal.Zero, decimal.Zero), nil)
		mockBalanceRepo.EXPECT().
			Delete(gomock.Any(), organizationID, ledgerID, balanceID).
			Return(nil)
		expectCacheEvict(mockRedisRepo, organizationID, ledgerID, bal)

		assert.NoError(t, uc.DeleteBalance(ctx, organizationID, ledgerID, balanceID))
	})

	t.Run("redis get error fails closed and releases marker", func(t *testing.T) {
		uc, mockBalanceRepo, mockRedisRepo := setupDeleteBalanceUseCase(t)
		bal := &mmodel.Balance{ID: balanceID.String(), Alias: "alias", Key: "key"}
		expectedErr := errors.New("redis unavailable")

		mockBalanceRepo.EXPECT().
			Find(gomock.Any(), organizationID, ledgerID, balanceID).
			Return(bal, nil)
		expectDeleteMarkerPlant(mockRedisRepo, organizationID, ledgerID, bal)
		mockRedisRepo.EXPECT().
			Get(gomock.Any(), balanceCacheKeyFor(organizationID, ledgerID, bal)).
			Return("", expectedErr)
		expectDeleteMarkerRelease(mockRedisRepo, organizationID, ledgerID, bal)
		mockBalanceRepo.EXPECT().
			Delete(gomock.Any(), organizationID, ledgerID, balanceID).
			Times(0)

		err := uc.DeleteBalance(ctx, organizationID, ledgerID, balanceID)
		assert.ErrorIs(t, err, expectedErr)
	})

	t.Run("invalid redis json fails closed and releases marker", func(t *testing.T) {
		uc, mockBalanceRepo, mockRedisRepo := setupDeleteBalanceUseCase(t)
		bal := &mmodel.Balance{ID: balanceID.String(), Alias: "alias", Key: "key"}

		mockBalanceRepo.EXPECT().
			Find(gomock.Any(), organizationID, ledgerID, balanceID).
			Return(bal, nil)
		expectDeleteMarkerPlant(mockRedisRepo, organizationID, ledgerID, bal)
		mockRedisRepo.EXPECT().
			Get(gomock.Any(), balanceCacheKeyFor(organizationID, ledgerID, bal)).
			Return("not-json", nil)
		expectDeleteMarkerRelease(mockRedisRepo, organizationID, ledgerID, bal)
		mockBalanceRepo.EXPECT().
			Delete(gomock.Any(), organizationID, ledgerID, balanceID).
			Times(0)

		err := uc.DeleteBalance(ctx, organizationID, ledgerID, balanceID)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to decode balance cache value")
	})

	t.Run("already-owned marker fails before cache read", func(t *testing.T) {
		uc, mockBalanceRepo, mockRedisRepo := setupDeleteBalanceUseCase(t)
		bal := &mmodel.Balance{ID: balanceID.String(), Alias: "alias", Key: "key"}

		mockBalanceRepo.EXPECT().
			Find(gomock.Any(), organizationID, ledgerID, balanceID).
			Return(bal, nil)
		mockRedisRepo.EXPECT().
			SetNX(gomock.Any(), deleteMarkerKeyFor(organizationID, ledgerID, bal), gomock.Any(), time.Duration(balanceDeleteMarkerTTLSeconds)).
			Return(false, nil)
		mockRedisRepo.EXPECT().
			Get(gomock.Any(), gomock.Any()).
			Times(0)
		mockRedisRepo.EXPECT().
			DeleteIfValue(gomock.Any(), deleteMarkerKeyFor(organizationID, ledgerID, bal), gomock.Any()).
			Times(0)
		mockBalanceRepo.EXPECT().
			Delete(gomock.Any(), organizationID, ledgerID, balanceID).
			Times(0)

		err := uc.DeleteBalance(ctx, organizationID, ledgerID, balanceID)
		var conflictErr midazpkg.EntityConflictError
		assert.Error(t, err)
		assert.True(t, errors.As(err, &conflictErr))
		assert.Equal(t, constant.ErrBalancesCantBeDeleted.Error(), conflictErr.Code)
	})

	t.Run("marker redis error fails before cache read", func(t *testing.T) {
		uc, mockBalanceRepo, mockRedisRepo := setupDeleteBalanceUseCase(t)
		bal := &mmodel.Balance{ID: balanceID.String(), Alias: "alias", Key: "key"}
		expectedErr := errors.New("marker redis unavailable")

		mockBalanceRepo.EXPECT().
			Find(gomock.Any(), organizationID, ledgerID, balanceID).
			Return(bal, nil)
		mockRedisRepo.EXPECT().
			SetNX(gomock.Any(), deleteMarkerKeyFor(organizationID, ledgerID, bal), gomock.Any(), time.Duration(balanceDeleteMarkerTTLSeconds)).
			Return(false, expectedErr)
		mockRedisRepo.EXPECT().
			Get(gomock.Any(), gomock.Any()).
			Times(0)
		mockBalanceRepo.EXPECT().
			Delete(gomock.Any(), organizationID, ledgerID, balanceID).
			Times(0)

		err := uc.DeleteBalance(ctx, organizationID, ledgerID, balanceID)
		assert.ErrorIs(t, err, expectedErr)
	})
}

func balanceRedisSnapshot(available, onHold decimal.Decimal) string {
	return balanceRedisSnapshotWithOverdraft(available, onHold, "0")
}

func balanceRedisSnapshotWithOverdraft(available, onHold decimal.Decimal, overdraftUsed string) string {
	data, _ := json.Marshal(mmodel.BalanceRedis{
		Available:     available,
		OnHold:        onHold,
		OverdraftUsed: overdraftUsed,
	})

	return string(data)
}

func setupDeleteBalanceUseCase(t *testing.T) (*UseCase, *balance.MockRepository, *redis.MockRedisRepository) {
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

// TestDeleteBalanceLegacyEmptyCachedOverdraftPermitsDeletion locks the single-delete half of the
// shared cached-OverdraftUsed rule: the pre-overdraft snapshot shape carries no debt, so it must
// not fail closed the way an unreadable value does.
func TestDeleteBalanceLegacyEmptyCachedOverdraftPermitsDeletion(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	organizationID := uuid.New()
	ledgerID := uuid.New()
	balanceID := uuid.New()

	uc, mockBalanceRepo, mockRedisRepo := setupDeleteBalanceUseCase(t)
	bal := &mmodel.Balance{
		ID:        balanceID.String(),
		Alias:     "alias",
		Key:       "key",
		Available: decimal.Zero,
		OnHold:    decimal.Zero,
	}

	mockBalanceRepo.EXPECT().
		Find(gomock.Any(), organizationID, ledgerID, balanceID).
		Return(bal, nil)
	expectDeleteMarkerPlant(mockRedisRepo, organizationID, ledgerID, bal)
	mockRedisRepo.EXPECT().
		Get(gomock.Any(), balanceCacheKeyFor(organizationID, ledgerID, bal)).
		Return(balanceRedisSnapshotWithOverdraft(decimal.Zero, decimal.Zero, ""), nil)
	mockBalanceRepo.EXPECT().
		Delete(gomock.Any(), organizationID, ledgerID, balanceID).
		Return(nil)
	expectCacheEvict(mockRedisRepo, organizationID, ledgerID, bal)

	assert.NoError(t, uc.DeleteBalance(ctx, organizationID, ledgerID, balanceID))
}
