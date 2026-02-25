// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package query

import (
	"context"
	"errors"
	"testing"
	"time"

	libCommons "github.com/LerianStudio/lib-commons/v3/commons"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/postgres/balance"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/postgres/operation"
	"github.com/LerianStudio/midaz/v3/pkg"
	"github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

func TestGetBalanceAtTimestamp(t *testing.T) {
	t.Parallel()

	t.Run("success_returns_balance_with_created_at_from_current_balance", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		orgID := libCommons.GenerateUUIDv7()
		ledgerID := libCommons.GenerateUUIDv7()
		accountID := libCommons.GenerateUUIDv7()
		balanceID := libCommons.GenerateUUIDv7()
		timestamp := time.Now().Add(-time.Hour)
		balanceCreatedAt := time.Now().Add(-24 * time.Hour)
		operationCreatedAt := timestamp.Add(-30 * time.Minute)

		available := decimal.NewFromInt(5000)
		onHold := decimal.NewFromInt(500)
		version := int64(10)

		currentBalance := &mmodel.Balance{
			ID:             balanceID.String(),
			OrganizationID: orgID.String(),
			LedgerID:       ledgerID.String(),
			AccountID:      accountID.String(),
			Alias:          "@user1",
			Key:            "default",
			AssetCode:      "USD",
			AccountType:    "deposit",
			CreatedAt:      balanceCreatedAt,
		}

		lastOperation := &operation.Operation{
			ID:         libCommons.GenerateUUIDv7().String(),
			AccountID:  accountID.String(),
			BalanceKey: "default",
			AssetCode:  "USD",
			BalanceAfter: operation.Balance{
				Available: &available,
				OnHold:    &onHold,
				Version:   &version,
			},
			CreatedAt: operationCreatedAt,
		}

		balanceRepo := balance.NewMockRepository(ctrl)
		operationRepo := operation.NewMockRepository(ctrl)

		uc := UseCase{BalanceRepo: balanceRepo, OperationRepo: operationRepo}

		balanceRepo.EXPECT().
			Find(gomock.Any(), orgID, ledgerID, balanceID).
			Return(currentBalance, nil)

		operationRepo.EXPECT().
			FindLastOperationBeforeTimestamp(gomock.Any(), orgID, ledgerID, balanceID, timestamp).
			Return(lastOperation, nil)

		result, err := uc.GetBalanceAtTimestamp(context.Background(), orgID, ledgerID, balanceID, timestamp)

		require.NoError(t, err)
		assert.NotNil(t, result)
		assert.Equal(t, balanceID.String(), result.ID)
		assert.Equal(t, available, result.Available)
		assert.Equal(t, onHold, result.OnHold)
		assert.Equal(t, version, result.Version)
		// Key assertion: CreatedAt should come from currentBalance, not operation
		assert.Equal(t, balanceCreatedAt, result.CreatedAt, "CreatedAt should match currentBalance.CreatedAt")
		// UpdatedAt should be the operation timestamp
		assert.Equal(t, operationCreatedAt, result.UpdatedAt, "UpdatedAt should match operation.CreatedAt")
	})

	t.Run("future_timestamp_returns_error", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		orgID := libCommons.GenerateUUIDv7()
		ledgerID := libCommons.GenerateUUIDv7()
		balanceID := libCommons.GenerateUUIDv7()
		futureTimestamp := time.Now().Add(time.Hour)

		balanceRepo := balance.NewMockRepository(ctrl)
		operationRepo := operation.NewMockRepository(ctrl)

		uc := UseCase{BalanceRepo: balanceRepo, OperationRepo: operationRepo}

		result, err := uc.GetBalanceAtTimestamp(context.Background(), orgID, ledgerID, balanceID, futureTimestamp)

		assert.Error(t, err)
		assert.Nil(t, result)
		assert.Contains(t, err.Error(), constant.ErrInvalidTimestamp.Error())
	})

	t.Run("balance_not_found_returns_error", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		orgID := libCommons.GenerateUUIDv7()
		ledgerID := libCommons.GenerateUUIDv7()
		balanceID := libCommons.GenerateUUIDv7()
		timestamp := time.Now().Add(-time.Hour)

		balanceRepo := balance.NewMockRepository(ctrl)
		operationRepo := operation.NewMockRepository(ctrl)

		uc := UseCase{BalanceRepo: balanceRepo, OperationRepo: operationRepo}

		// Repository returns (nil, ErrEntityNotFound) when balance doesn't exist
		notFoundErr := pkg.ValidateBusinessError(constant.ErrEntityNotFound, "Balance")
		balanceRepo.EXPECT().
			Find(gomock.Any(), orgID, ledgerID, balanceID).
			Return(nil, notFoundErr)

		result, err := uc.GetBalanceAtTimestamp(context.Background(), orgID, ledgerID, balanceID, timestamp)

		assert.Error(t, err)
		assert.Nil(t, result)
		assert.Contains(t, err.Error(), "entity")
	})

	t.Run("repo_errors", func(t *testing.T) {
		t.Parallel()

		tests := []struct {
			name              string
			balanceRepoErr    error
			operationRepoErr  error
			setupOperationExp bool
		}{
			{
				name:              "balance_repo_error",
				balanceRepoErr:    errors.New("balance database error"),
				operationRepoErr:  nil,
				setupOperationExp: false,
			},
			{
				name:              "operation_repo_error",
				balanceRepoErr:    nil,
				operationRepoErr:  errors.New("operation database error"),
				setupOperationExp: true,
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()

				ctrl := gomock.NewController(t)
				defer ctrl.Finish()

				orgID := libCommons.GenerateUUIDv7()
				ledgerID := libCommons.GenerateUUIDv7()
				balanceID := libCommons.GenerateUUIDv7()
				timestamp := time.Now().Add(-time.Hour)

				currentBalance := &mmodel.Balance{
					ID:             balanceID.String(),
					OrganizationID: orgID.String(),
					LedgerID:       ledgerID.String(),
				}

				balanceRepo := balance.NewMockRepository(ctrl)
				operationRepo := operation.NewMockRepository(ctrl)

				uc := UseCase{BalanceRepo: balanceRepo, OperationRepo: operationRepo}

				if tt.balanceRepoErr != nil {
					balanceRepo.EXPECT().
						Find(gomock.Any(), orgID, ledgerID, balanceID).
						Return(nil, tt.balanceRepoErr)
				} else {
					balanceRepo.EXPECT().
						Find(gomock.Any(), orgID, ledgerID, balanceID).
						Return(currentBalance, nil)
				}

				if tt.setupOperationExp {
					operationRepo.EXPECT().
						FindLastOperationBeforeTimestamp(gomock.Any(), orgID, ledgerID, balanceID, timestamp).
						Return(nil, tt.operationRepoErr)
				}

				result, err := uc.GetBalanceAtTimestamp(context.Background(), orgID, ledgerID, balanceID, timestamp)

				assert.Error(t, err)
				assert.Nil(t, result)
			})
		}
	})

	t.Run("no_operation_before_timestamp_and_balance_exists_returns_zero_balance", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		orgID := libCommons.GenerateUUIDv7()
		ledgerID := libCommons.GenerateUUIDv7()
		accountID := libCommons.GenerateUUIDv7()
		balanceID := libCommons.GenerateUUIDv7()
		timestamp := time.Now().Add(-time.Hour)
		// Balance was created before the timestamp
		balanceCreatedAt := time.Now().Add(-24 * time.Hour)

		currentBalance := &mmodel.Balance{
			ID:             balanceID.String(),
			OrganizationID: orgID.String(),
			LedgerID:       ledgerID.String(),
			AccountID:      accountID.String(),
			Alias:          "@user1",
			Key:            "default",
			AssetCode:      "USD",
			AccountType:    "deposit",
			CreatedAt:      balanceCreatedAt,
		}

		balanceRepo := balance.NewMockRepository(ctrl)
		operationRepo := operation.NewMockRepository(ctrl)

		uc := UseCase{BalanceRepo: balanceRepo, OperationRepo: operationRepo}

		balanceRepo.EXPECT().
			Find(gomock.Any(), orgID, ledgerID, balanceID).
			Return(currentBalance, nil)

		operationRepo.EXPECT().
			FindLastOperationBeforeTimestamp(gomock.Any(), orgID, ledgerID, balanceID, timestamp).
			Return(nil, nil)

		result, err := uc.GetBalanceAtTimestamp(context.Background(), orgID, ledgerID, balanceID, timestamp)

		require.NoError(t, err)
		assert.NotNil(t, result)
		assert.Equal(t, balanceID.String(), result.ID)
		assert.True(t, result.Available.Equal(decimal.Zero), "Available should be zero for balance without operations")
		assert.True(t, result.OnHold.Equal(decimal.Zero), "OnHold should be zero for balance without operations")
		assert.Equal(t, int64(0), result.Version, "Version should be zero for balance without operations")
		assert.Equal(t, balanceCreatedAt, result.CreatedAt)
		assert.Equal(t, balanceCreatedAt, result.UpdatedAt, "UpdatedAt should equal CreatedAt for initial state")
		assert.Equal(t, "@user1", result.Alias)
		assert.Equal(t, "default", result.Key)
		assert.Equal(t, "USD", result.AssetCode)
	})

	t.Run("no_operation_and_balance_created_after_timestamp_returns_error", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		orgID := libCommons.GenerateUUIDv7()
		ledgerID := libCommons.GenerateUUIDv7()
		balanceID := libCommons.GenerateUUIDv7()
		timestamp := time.Now().Add(-24 * time.Hour)
		// Balance was created AFTER the timestamp
		balanceCreatedAt := time.Now().Add(-time.Hour)

		currentBalance := &mmodel.Balance{
			ID:             balanceID.String(),
			OrganizationID: orgID.String(),
			LedgerID:       ledgerID.String(),
			CreatedAt:      balanceCreatedAt,
		}

		balanceRepo := balance.NewMockRepository(ctrl)
		operationRepo := operation.NewMockRepository(ctrl)

		uc := UseCase{BalanceRepo: balanceRepo, OperationRepo: operationRepo}

		balanceRepo.EXPECT().
			Find(gomock.Any(), orgID, ledgerID, balanceID).
			Return(currentBalance, nil)

		operationRepo.EXPECT().
			FindLastOperationBeforeTimestamp(gomock.Any(), orgID, ledgerID, balanceID, timestamp).
			Return(nil, nil)

		result, err := uc.GetBalanceAtTimestamp(context.Background(), orgID, ledgerID, balanceID, timestamp)

		assert.Error(t, err)
		assert.Nil(t, result)
		// Error message contains info about no balance data at timestamp
		assert.Contains(t, err.Error(), "balance data")
	})

	t.Run("nil_balance_amounts_default_to_zero", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		orgID := libCommons.GenerateUUIDv7()
		ledgerID := libCommons.GenerateUUIDv7()
		accountID := libCommons.GenerateUUIDv7()
		balanceID := libCommons.GenerateUUIDv7()
		timestamp := time.Now().Add(-time.Hour)
		balanceCreatedAt := time.Now().Add(-24 * time.Hour)
		operationCreatedAt := timestamp.Add(-30 * time.Minute)

		currentBalance := &mmodel.Balance{
			ID:             balanceID.String(),
			OrganizationID: orgID.String(),
			LedgerID:       ledgerID.String(),
			AccountID:      accountID.String(),
			Alias:          "@user1",
			Key:            "default",
			AssetCode:      "USD",
			AccountType:    "deposit",
			CreatedAt:      balanceCreatedAt,
		}

		// Operation with nil BalanceAfter fields
		lastOperation := &operation.Operation{
			ID:           libCommons.GenerateUUIDv7().String(),
			AccountID:    accountID.String(),
			BalanceKey:   "default",
			AssetCode:    "USD",
			BalanceAfter: operation.Balance{}, // All nil
			CreatedAt:    operationCreatedAt,
		}

		balanceRepo := balance.NewMockRepository(ctrl)
		operationRepo := operation.NewMockRepository(ctrl)

		uc := UseCase{BalanceRepo: balanceRepo, OperationRepo: operationRepo}

		balanceRepo.EXPECT().
			Find(gomock.Any(), orgID, ledgerID, balanceID).
			Return(currentBalance, nil)

		operationRepo.EXPECT().
			FindLastOperationBeforeTimestamp(gomock.Any(), orgID, ledgerID, balanceID, timestamp).
			Return(lastOperation, nil)

		result, err := uc.GetBalanceAtTimestamp(context.Background(), orgID, ledgerID, balanceID, timestamp)

		require.NoError(t, err)
		assert.NotNil(t, result)
		assert.True(t, result.Available.Equal(decimal.Zero), "Available should default to zero")
		assert.True(t, result.OnHold.Equal(decimal.Zero), "OnHold should default to zero")
		assert.Equal(t, int64(0), result.Version, "Version should default to zero")
		assert.Equal(t, balanceCreatedAt, result.CreatedAt)
	})
}
