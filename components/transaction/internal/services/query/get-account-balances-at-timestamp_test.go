package query

import (
	"context"
	"errors"
	"testing"
	"time"

	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/postgres/balance"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/postgres/operation"
	"github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

func TestGetAccountBalancesAtTimestamp(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name            string
		timestampOffset time.Duration
		setupMocks      func(balanceRepo *balance.MockRepository, orgID, ledgerID, accountID uuid.UUID, timestamp time.Time) []*mmodel.Balance
		expectError     bool
		errorContains   string
		validateResults func(t *testing.T, results []*mmodel.Balance, mockBalances []*mmodel.Balance)
	}{
		{
			name:            "Success_returns_balances_at_timestamp",
			timestampOffset: -time.Hour,
			setupMocks: func(balanceRepo *balance.MockRepository, orgID, ledgerID, accountID uuid.UUID, timestamp time.Time) []*mmodel.Balance {
				balanceID := libCommons.GenerateUUIDv7()
				balanceCreatedAt := time.Now().Add(-24 * time.Hour)
				updatedAt := timestamp.Add(-30 * time.Minute)

				balances := []*mmodel.Balance{
					{
						ID:             balanceID.String(),
						OrganizationID: orgID.String(),
						LedgerID:       ledgerID.String(),
						AccountID:      accountID.String(),
						Alias:          "@user1",
						Key:            "default",
						AssetCode:      "USD",
						AccountType:    "deposit",
						Available:      decimal.NewFromInt(5000),
						OnHold:         decimal.NewFromInt(500),
						Version:        10,
						CreatedAt:      balanceCreatedAt,
						UpdatedAt:      updatedAt,
					},
				}

				balanceRepo.EXPECT().
					ListByAccountIDAtTimestamp(gomock.Any(), orgID, ledgerID, accountID, timestamp).
					Return(balances, nil)

				return balances
			},
			expectError: false,
			validateResults: func(t *testing.T, results []*mmodel.Balance, mockBalances []*mmodel.Balance) {
				require.Len(t, results, 1)
				result := results[0]
				expected := mockBalances[0]

				assert.Equal(t, expected.ID, result.ID)
				assert.Equal(t, expected.Available, result.Available)
				assert.Equal(t, expected.OnHold, result.OnHold)
				assert.Equal(t, expected.Version, result.Version)
				assert.Equal(t, expected.CreatedAt, result.CreatedAt)
				assert.Equal(t, expected.UpdatedAt, result.UpdatedAt)
			},
		},
		{
			name:            "Future_timestamp_returns_error",
			timestampOffset: time.Hour, // Future
			setupMocks: func(balanceRepo *balance.MockRepository, orgID, ledgerID, accountID uuid.UUID, timestamp time.Time) []*mmodel.Balance {
				// No mock expectations - validation fails before repo call
				return nil
			},
			expectError:   true,
			errorContains: constant.ErrInvalidTimestamp.Error(),
			validateResults: func(t *testing.T, results []*mmodel.Balance, mockBalances []*mmodel.Balance) {
				assert.Nil(t, results)
			},
		},
		{
			name:            "No_balances_at_timestamp_returns_empty_slice",
			timestampOffset: -24 * time.Hour,
			setupMocks: func(balanceRepo *balance.MockRepository, orgID, ledgerID, accountID uuid.UUID, timestamp time.Time) []*mmodel.Balance {
				balanceRepo.EXPECT().
					ListByAccountIDAtTimestamp(gomock.Any(), orgID, ledgerID, accountID, timestamp).
					Return([]*mmodel.Balance{}, nil)

				return []*mmodel.Balance{}
			},
			expectError: false,
			validateResults: func(t *testing.T, results []*mmodel.Balance, mockBalances []*mmodel.Balance) {
				assert.Empty(t, results)
			},
		},
		{
			name:            "Balance_repo_error_returns_error",
			timestampOffset: -time.Hour,
			setupMocks: func(balanceRepo *balance.MockRepository, orgID, ledgerID, accountID uuid.UUID, timestamp time.Time) []*mmodel.Balance {
				balanceRepo.EXPECT().
					ListByAccountIDAtTimestamp(gomock.Any(), orgID, ledgerID, accountID, timestamp).
					Return(nil, errors.New("database error"))

				return nil
			},
			expectError: true,
			validateResults: func(t *testing.T, results []*mmodel.Balance, mockBalances []*mmodel.Balance) {
				assert.Nil(t, results)
			},
		},
		{
			name:            "Multiple_balances_with_different_values",
			timestampOffset: -time.Hour,
			setupMocks: func(balanceRepo *balance.MockRepository, orgID, ledgerID, accountID uuid.UUID, timestamp time.Time) []*mmodel.Balance {
				balanceID1 := libCommons.GenerateUUIDv7()
				balanceID2 := libCommons.GenerateUUIDv7()
				balance1CreatedAt := time.Now().Add(-48 * time.Hour)
				balance2CreatedAt := time.Now().Add(-24 * time.Hour)

				balances := []*mmodel.Balance{
					{
						ID:             balanceID1.String(),
						OrganizationID: orgID.String(),
						LedgerID:       ledgerID.String(),
						AccountID:      accountID.String(),
						Key:            "default",
						AssetCode:      "USD",
						AccountType:    "deposit",
						Available:      decimal.NewFromInt(5000),
						OnHold:         decimal.NewFromInt(500),
						Version:        10,
						CreatedAt:      balance1CreatedAt,
						UpdatedAt:      timestamp.Add(-30 * time.Minute),
					},
					{
						ID:             balanceID2.String(),
						OrganizationID: orgID.String(),
						LedgerID:       ledgerID.String(),
						AccountID:      accountID.String(),
						Key:            "savings",
						AssetCode:      "USD",
						AccountType:    "deposit",
						Available:      decimal.Zero,
						OnHold:         decimal.Zero,
						Version:        0,
						CreatedAt:      balance2CreatedAt,
						UpdatedAt:      balance2CreatedAt,
					},
				}

				balanceRepo.EXPECT().
					ListByAccountIDAtTimestamp(gomock.Any(), orgID, ledgerID, accountID, timestamp).
					Return(balances, nil)

				return balances
			},
			expectError: false,
			validateResults: func(t *testing.T, results []*mmodel.Balance, mockBalances []*mmodel.Balance) {
				require.Len(t, results, 2)

				// Find balances by key for deterministic validation
				var defaultBalance, savingsBalance *mmodel.Balance
				for _, r := range results {
					switch r.Key {
					case "default":
						defaultBalance = r
					case "savings":
						savingsBalance = r
					}
				}

				require.NotNil(t, defaultBalance, "default balance should exist")
				assert.Equal(t, decimal.NewFromInt(5000), defaultBalance.Available)
				assert.Equal(t, int64(10), defaultBalance.Version)

				require.NotNil(t, savingsBalance, "savings balance should exist")
				assert.True(t, savingsBalance.Available.Equal(decimal.Zero))
				assert.Equal(t, int64(0), savingsBalance.Version)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			t.Cleanup(ctrl.Finish)

			orgID := libCommons.GenerateUUIDv7()
			ledgerID := libCommons.GenerateUUIDv7()
			accountID := libCommons.GenerateUUIDv7()
			timestamp := time.Now().Add(tt.timestampOffset)

			balanceRepo := balance.NewMockRepository(ctrl)
			operationRepo := operation.NewMockRepository(ctrl)

			mockBalances := tt.setupMocks(balanceRepo, orgID, ledgerID, accountID, timestamp)

			uc := UseCase{BalanceRepo: balanceRepo, OperationRepo: operationRepo}

			results, err := uc.GetAccountBalancesAtTimestamp(context.Background(), orgID, ledgerID, accountID, timestamp)

			if tt.expectError {
				require.Error(t, err)
				if tt.errorContains != "" {
					assert.Contains(t, err.Error(), tt.errorContains)
				}
			} else {
				require.NoError(t, err)
			}

			tt.validateResults(t, results, mockBalances)
		})
	}
}
