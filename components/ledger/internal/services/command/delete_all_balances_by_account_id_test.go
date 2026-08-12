// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"errors"
	"fmt"
	"testing"

	libCommons "github.com/LerianStudio/lib-commons/v5/commons"
	"github.com/LerianStudio/midaz/v3/components/ledger/internal/adapters/postgres/balance"
	redis "github.com/LerianStudio/midaz/v3/components/ledger/internal/adapters/redis/transaction"
	midazpkg "github.com/LerianStudio/midaz/v3/pkg"
	"github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/google/uuid"
	goredis "github.com/redis/go-redis/v9"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
)

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

	t.Run("redis lookup error", func(t *testing.T) {
		uc, mockBalanceRepo, mockRedisRepo := setupDeleteAllBalancesUseCase(t)
		expectedErr := errors.New("redis error")
		balanceItem := newTestBalance(decimal.NewFromInt(1), decimal.Zero)

		mockBalanceRepo.EXPECT().
			ListByAccountID(gomock.Any(), organizationID, ledgerID, accountID).
			Return([]*mmodel.Balance{balanceItem}, nil)
		mockRedisRepo.EXPECT().
			ListBalanceByKey(gomock.Any(), organizationID, ledgerID, balanceRedisKey(balanceItem)).
			Return(nil, expectedErr)

		err := uc.DeleteAllBalancesByAccountID(ctx, organizationID, ledgerID, accountID, requestID.String())
		assert.ErrorIs(t, err, expectedErr)
	})

	t.Run("redis zero-funds balance present proceeds to deletion", func(t *testing.T) {
		uc, mockBalanceRepo, mockRedisRepo := setupDeleteAllBalancesUseCase(t)
		balanceItem := newTestBalance(decimal.Zero, decimal.Zero)

		mockBalanceRepo.EXPECT().
			ListByAccountID(gomock.Any(), organizationID, ledgerID, accountID).
			Return([]*mmodel.Balance{balanceItem}, nil)
		mockRedisRepo.EXPECT().
			ListBalanceByKey(gomock.Any(), organizationID, ledgerID, balanceRedisKey(balanceItem)).
			Return(&mmodel.Balance{}, nil)
		mockBalanceRepo.EXPECT().
			UpdateAllByAccountID(gomock.Any(), organizationID, ledgerID, accountID, gomock.Any()).
			Return(nil)
		mockBalanceRepo.EXPECT().
			DeleteAllByIDs(gomock.Any(), organizationID, ledgerID, gomock.Any()).
			Return(nil)

		err := uc.DeleteAllBalancesByAccountID(ctx, organizationID, ledgerID, accountID, requestID.String())
		assert.NoError(t, err)
	})

	t.Run("balances with funds remaining prevent deletion", func(t *testing.T) {
		uc, mockBalanceRepo, mockRedisRepo := setupDeleteAllBalancesUseCase(t)
		balanceItem := newTestBalance(decimal.NewFromInt(10), decimal.Zero)

		mockBalanceRepo.EXPECT().
			ListByAccountID(gomock.Any(), organizationID, ledgerID, accountID).
			Return([]*mmodel.Balance{balanceItem}, nil)
		mockRedisRepo.EXPECT().
			ListBalanceByKey(gomock.Any(), organizationID, ledgerID, balanceRedisKey(balanceItem)).
			Return(nil, nil)

		err := uc.DeleteAllBalancesByAccountID(ctx, organizationID, ledgerID, accountID, requestID.String())

		var validationErr midazpkg.ValidationError
		assert.Error(t, err)
		assert.True(t, errors.As(err, &validationErr))
		assert.Equal(t, constant.ErrBalancesCantBeDeleted.Error(), validationErr.Code)
	})

	t.Run("toggle balance transfers error", func(t *testing.T) {
		uc, mockBalanceRepo, mockRedisRepo := setupDeleteAllBalancesUseCase(t)
		balanceItem := newTestBalance(decimal.Zero, decimal.Zero)
		expectedErr := errors.New("update permissions error")

		mockBalanceRepo.EXPECT().
			ListByAccountID(gomock.Any(), organizationID, ledgerID, accountID).
			Return([]*mmodel.Balance{balanceItem}, nil)
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
			} else {
				mockBalanceRepo.EXPECT().
					DeleteAllByIDs(gomock.Any(), organizationID, ledgerID, gomock.Any()).
					Times(0)
			}

			err := uc.DeleteAllBalancesByAccountID(ctx, organizationID, ledgerID, accountID, requestID.String())

			if tt.expectProceed {
				assert.NoError(t, err)
				return
			}

			var validationErr midazpkg.ValidationError
			assert.Error(t, err)
			assert.True(t, errors.As(err, &validationErr))
			assert.Equal(t, constant.ErrBalancesCantBeDeleted.Error(), validationErr.Code)
		})
	}

	// Guards the loop across MULTIPLE balances: the first balance has zero funds on a cache
	// miss (loop proceeds), the second holds on_hold funds on a cache miss (loop rejects).
	// Deletion must be refused and DeleteAllByIDs must never run.
	t.Run("multiple balances reject when a later balance still holds funds", func(t *testing.T) {
		t.Parallel()

		uc, mockBalanceRepo, mockRedisRepo := setupDeleteAllBalancesUseCase(t)

		zeroFundsBalance := newTestBalance(decimal.Zero, decimal.Zero)
		onHoldFundsBalance := newTestBalance(decimal.Zero, decimal.NewFromInt(5))

		mockBalanceRepo.EXPECT().
			ListByAccountID(gomock.Any(), organizationID, ledgerID, accountID).
			Return([]*mmodel.Balance{zeroFundsBalance, onHoldFundsBalance}, nil)

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

		err := uc.DeleteAllBalancesByAccountID(ctx, organizationID, ledgerID, accountID, requestID.String())

		var validationErr midazpkg.ValidationError
		assert.Error(t, err)
		assert.True(t, errors.As(err, &validationErr))
		assert.Equal(t, constant.ErrBalancesCantBeDeleted.Error(), validationErr.Code)
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
