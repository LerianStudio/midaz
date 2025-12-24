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

func TestUpdateBalances_AllStale_ReturnsError(t *testing.T) {
	t.Parallel()

	// This test verifies the CRITICAL FIX: when ALL balances are stale,
	// UpdateBalances must return an error rather than silently succeeding.
	//
	// Previously this would return nil (silent success) - which was the BUG
	// causing 82-97% data loss in chaos tests.

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockRedis := redis.NewMockRedisRepository(ctrl)

	uc := &UseCase{
		RedisRepo: mockRedis,
		// BalanceRepo not needed for this test (won't be called)
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
	// This simulates the scenario where concurrent transactions have advanced the version
	mockRedis.EXPECT().
		ListBalanceByKey(gomock.Any(), orgID, ledgerID, "account1").
		Return(&mmodel.Balance{Version: 5}, nil)

	mockRedis.EXPECT().
		ListBalanceByKey(gomock.Any(), orgID, ledgerID, "account2").
		Return(&mmodel.Balance{Version: 5}, nil)

	// Execute: UpdateBalances should return error when all balances are stale
	err := uc.UpdateBalances(ctx, orgID, ledgerID, validate, balances)

	// Assert: Error is returned (not nil)
	assert.Error(t, err, "UpdateBalances should return error when all balances are stale")

	// Assert: Error is FailedPreconditionError with correct code
	var failedPreconditionErr pkg.FailedPreconditionError
	assert.True(t, errors.As(err, &failedPreconditionErr),
		"Error should be FailedPreconditionError, got: %T", err)
	assert.Equal(t, constant.ErrStaleBalanceUpdateSkipped.Error(), failedPreconditionErr.Code,
		"Error code should match ErrStaleBalanceUpdateSkipped")

	// Assert: Error message is descriptive
	assert.Contains(t, err.Error(), "All balance updates were skipped",
		"Error message should explain the issue")
}

func TestUpdateBalances_PartialStale_SucceedsWithFreshBalances(t *testing.T) {
	t.Parallel()

	// This test verifies that when SOME balances are stale,
	// UpdateBalances still processes the fresh ones and returns success.

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
		Return(&mmodel.Balance{Version: 5}, nil)

	mockRedis.EXPECT().
		ListBalanceByKey(gomock.Any(), orgID, ledgerID, "account2").
		Return(&mmodel.Balance{Version: 1}, nil)

	// Mock: BalancesUpdate should be called with only account2 (the fresh one)
	mockBalanceRepo.EXPECT().
		BalancesUpdate(gomock.Any(), orgID, ledgerID, gomock.Any()).
		DoAndReturn(func(ctx context.Context, orgID, ledgerID uuid.UUID, balances []*mmodel.Balance) error {
			// Verify only 1 balance is being updated (account2)
			assert.Len(t, balances, 1, "Should only update 1 fresh balance")
			assert.Equal(t, balanceID2, balances[0].ID, "Should update account2 (fresh)")
			return nil
		})

	// Execute
	err := uc.UpdateBalances(ctx, orgID, ledgerID, validate, balances)

	// Assert: No error (partial stale is acceptable)
	assert.NoError(t, err, "UpdateBalances should succeed with partial stale balances")
}
