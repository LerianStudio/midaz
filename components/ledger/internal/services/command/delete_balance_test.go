// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"errors"
	"testing"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/balance"
	redis "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/redis/transaction"
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
			name      string
			available decimal.Decimal
			onHold    decimal.Decimal
		}{
			{"available only", decimal.NewFromInt(100), decimal.Zero},
			{"on-hold only", decimal.Zero, decimal.NewFromInt(50)},
		}

		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				uc, mockBalanceRepo, mockRedisRepo := setupDeleteBalanceUseCase(t)
				bal := &mmodel.Balance{
					ID:        balanceID.String(),
					Alias:     "alias",
					Key:       "key",
					Available: tc.available,
					OnHold:    tc.onHold,
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
		mockBalanceRepo.EXPECT().
			Delete(gomock.Any(), organizationID, ledgerID, balanceID).
			Return(nil)
		expectCacheEvict(mockRedisRepo, organizationID, ledgerID, zeroBalance)

		err := uc.DeleteBalance(ctx, organizationID, ledgerID, balanceID)

		assert.NoError(t, err)
	})
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
		del := mockBalanceRepo.EXPECT().
			Delete(gomock.Any(), organizationID, ledgerID, balanceID).
			Return(nil)
		evict := expectCacheEvict(mockRedisRepo, organizationID, ledgerID, bal)

		// plant-before-delete and evict-after-soft-delete.
		gomock.InOrder(plant, del, evict)

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
		mockBalanceRepo.EXPECT().
			Delete(gomock.Any(), organizationID, ledgerID, balanceID).
			Return(nil)
		mockRedisRepo.EXPECT().
			Del(gomock.Any(), balanceCacheKeyFor(organizationID, ledgerID, bal)).
			Return(errors.New("evict failed"))

		err := uc.DeleteBalance(ctx, organizationID, ledgerID, balanceID)
		assert.NoError(t, err)
	})
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
