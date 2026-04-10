// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"testing"

	redis "github.com/LerianStudio/midaz/v3/components/ledger/internal/adapters/redis/transaction"
	"github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	pkgTransaction "github.com/LerianStudio/midaz/v3/pkg/transaction"
	"github.com/LerianStudio/midaz/v3/pkg/utils"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

func TestProcessBalanceOperations(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	ctx := context.Background()

	mockRedisRepo := redis.NewMockRedisRepository(ctrl)

	organizationID := uuid.MustParse("ad0032e5-ccf5-45f4-a3b2-12045e71b38a")
	ledgerID := uuid.MustParse("5d8ac48a-af68-4544-9bf8-80c3cc0715f4")
	uc := UseCase{
		TransactionRedisRepo: mockRedisRepo,
	}

	t.Run("lock balances successfully", func(t *testing.T) {
		balanceID1 := uuid.MustParse("c7d0fa07-3e11-4105-a0fc-6fa46834ce66")
		accountID1 := uuid.MustParse("bad0ddef-d697-4a4e-840d-1f5380de4607")

		fromAmount := pkgTransaction.Amount{
			Asset:     "USD",
			Value:     decimal.NewFromFloat(50),
			Operation: constant.DEBIT,
		}

		validate := &pkgTransaction.Responses{
			Aliases: []string{"alias1#default"},
			From: map[string]pkgTransaction.Amount{
				"0#alias1#default": fromAmount,
			},
		}

		balances := []*mmodel.Balance{
			{
				ID:             balanceID1.String(),
				AccountID:      accountID1.String(),
				OrganizationID: organizationID.String(),
				LedgerID:       ledgerID.String(),
				Alias:          "alias1",
				Key:            "default",
				Available:      decimal.NewFromFloat(100),
				OnHold:         decimal.NewFromFloat(0),
				Version:        1,
				AccountType:    "deposit",
				AllowSending:   true,
				AllowReceiving: true,
				AssetCode:      "USD",
			},
		}

		balanceOps := []mmodel.BalanceOperation{
			{
				Balance:     balances[0],
				Alias:       "0#alias1#default",
				Amount:      fromAmount,
				InternalKey: utils.BalanceInternalKey(organizationID, ledgerID, "alias1#default"),
			},
		}

		mockRedisRepo.EXPECT().
			ProcessBalanceAtomicOperation(
				gomock.Any(),
				organizationID,
				ledgerID,
				gomock.Any(), // transactionID
				constant.CREATED,
				false,
				gomock.Any(), // balance operations
			).
			Return(&mmodel.BalanceAtomicResult{Before: balances, After: balances}, nil)

		transactionID := uuid.New()
		result, err := uc.ProcessBalanceOperations(ctx, organizationID, ledgerID, transactionID, nil, validate, balanceOps, constant.CREATED)

		assert.NoError(t, err)
		assert.Len(t, result.Before, 1)
	})
}

func TestProcessBalanceOperations_DoubleEntrySplitting(t *testing.T) {
	t.Parallel()

	organizationID := uuid.MustParse("ad0032e5-ccf5-45f4-a3b2-12045e71b38a")
	ledgerID := uuid.MustParse("5d8ac48a-af68-4544-9bf8-80c3cc0715f4")

	tests := []struct {
		name             string
		fromAmount       pkgTransaction.Amount
		transactionType  string
		expectedOpsCount int
		expectedOp1      string
		expectedOp2      string
		// buildOps returns the inline balance operations for this test case.
		buildOps func(balance *mmodel.Balance) []mmodel.BalanceOperation
	}{
		{
			name: "CANCELED with RouteValidationEnabled produces 2 ops (RELEASE + CREDIT)",
			fromAmount: pkgTransaction.Amount{
				Asset:                  "USD",
				Value:                  decimal.NewFromFloat(50),
				Operation:              constant.RELEASE,
				TransactionType:        constant.CANCELED,
				RouteValidationEnabled: true,
			},
			transactionType:  constant.CANCELED,
			expectedOpsCount: 2,
			expectedOp1:      constant.RELEASE,
			expectedOp2:      constant.CREDIT,
			buildOps: func(balance *mmodel.Balance) []mmodel.BalanceOperation {
				amt := pkgTransaction.Amount{
					Asset:                  "USD",
					Value:                  decimal.NewFromFloat(50),
					Operation:              constant.RELEASE,
					TransactionType:        constant.CANCELED,
					RouteValidationEnabled: true,
				}
				op1, op2 := pkgTransaction.SplitDoubleEntryOps(amt)
				ik := utils.BalanceInternalKey(organizationID, ledgerID, "alias1#default")

				return []mmodel.BalanceOperation{
					{Balance: balance, Alias: "0#alias1#default", Amount: op1, InternalKey: ik},
					{Balance: balance, Alias: "0#alias1#default", Amount: op2, InternalKey: ik},
				}
			},
		},
		{
			name: "CANCELED without route flag produces 1 op (RELEASE)",
			fromAmount: pkgTransaction.Amount{
				Asset:           "USD",
				Value:           decimal.NewFromFloat(50),
				Operation:       constant.RELEASE,
				TransactionType: constant.CANCELED,
			},
			transactionType:  constant.CANCELED,
			expectedOpsCount: 1,
			expectedOp1:      constant.RELEASE,
			buildOps: func(balance *mmodel.Balance) []mmodel.BalanceOperation {
				return []mmodel.BalanceOperation{
					{
						Balance: balance,
						Alias:   "0#alias1#default",
						Amount: pkgTransaction.Amount{
							Asset:           "USD",
							Value:           decimal.NewFromFloat(50),
							Operation:       constant.RELEASE,
							TransactionType: constant.CANCELED,
						},
						InternalKey: utils.BalanceInternalKey(organizationID, ledgerID, "alias1#default"),
					},
				}
			},
		},
		{
			name: "PENDING with RouteValidationEnabled produces 2 ops (DEBIT + ONHOLD)",
			fromAmount: pkgTransaction.Amount{
				Asset:                  "USD",
				Value:                  decimal.NewFromFloat(50),
				Operation:              constant.ONHOLD,
				TransactionType:        constant.PENDING,
				RouteValidationEnabled: true,
			},
			transactionType:  constant.PENDING,
			expectedOpsCount: 2,
			expectedOp1:      constant.DEBIT,
			expectedOp2:      constant.ONHOLD,
			buildOps: func(balance *mmodel.Balance) []mmodel.BalanceOperation {
				amt := pkgTransaction.Amount{
					Asset:                  "USD",
					Value:                  decimal.NewFromFloat(50),
					Operation:              constant.ONHOLD,
					TransactionType:        constant.PENDING,
					RouteValidationEnabled: true,
				}
				op1, op2 := pkgTransaction.SplitDoubleEntryOps(amt)
				ik := utils.BalanceInternalKey(organizationID, ledgerID, "alias1#default")

				return []mmodel.BalanceOperation{
					{Balance: balance, Alias: "0#alias1#default", Amount: op1, InternalKey: ik},
					{Balance: balance, Alias: "0#alias1#default", Amount: op2, InternalKey: ik},
				}
			},
		},
		{
			name: "PENDING without route flag produces 1 op (ONHOLD)",
			fromAmount: pkgTransaction.Amount{
				Asset:           "USD",
				Value:           decimal.NewFromFloat(50),
				Operation:       constant.ONHOLD,
				TransactionType: constant.PENDING,
			},
			transactionType:  constant.PENDING,
			expectedOpsCount: 1,
			expectedOp1:      constant.ONHOLD,
			buildOps: func(balance *mmodel.Balance) []mmodel.BalanceOperation {
				return []mmodel.BalanceOperation{
					{
						Balance: balance,
						Alias:   "0#alias1#default",
						Amount: pkgTransaction.Amount{
							Asset:           "USD",
							Value:           decimal.NewFromFloat(50),
							Operation:       constant.ONHOLD,
							TransactionType: constant.PENDING,
						},
						InternalKey: utils.BalanceInternalKey(organizationID, ledgerID, "alias1#default"),
					},
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			ctx := context.Background()
			mockRedisRepo := redis.NewMockRedisRepository(ctrl)

			uc := UseCase{
				TransactionRedisRepo: mockRedisRepo,
			}

			balanceID := uuid.New().String()
			accountID := uuid.New().String()

			validate := &pkgTransaction.Responses{
				Aliases: []string{"alias1#default"},
				From: map[string]pkgTransaction.Amount{
					"0#alias1#default": tt.fromAmount,
				},
			}

			balances := []*mmodel.Balance{
				{
					ID:             balanceID,
					AccountID:      accountID,
					OrganizationID: organizationID.String(),
					LedgerID:       ledgerID.String(),
					Alias:          "alias1",
					Key:            "default",
					Available:      decimal.NewFromFloat(100),
					OnHold:         decimal.NewFromFloat(50),
					Version:        1,
					AccountType:    "deposit",
					AllowSending:   true,
					AllowReceiving: true,
					AssetCode:      "USD",
				},
			}

			balanceOps := tt.buildOps(balances[0])

			var capturedOps []mmodel.BalanceOperation

			mockRedisRepo.EXPECT().
				ProcessBalanceAtomicOperation(
					gomock.Any(),
					organizationID,
					ledgerID,
					gomock.Any(),
					tt.transactionType,
					false,
					gomock.Any(),
				).
				DoAndReturn(func(_ context.Context, _, _ uuid.UUID, _ uuid.UUID, _ string, _ bool, ops []mmodel.BalanceOperation) (*mmodel.BalanceAtomicResult, error) {
					capturedOps = ops
					return &mmodel.BalanceAtomicResult{Before: balances, After: balances}, nil
				})

			transactionID := uuid.New()
			result, err := uc.ProcessBalanceOperations(ctx, organizationID, ledgerID, transactionID, nil, validate, balanceOps, tt.transactionType)

			require.NoError(t, err)
			require.NotNil(t, result)
			require.Len(t, capturedOps, tt.expectedOpsCount,
				"expected %d balance operations, got %d", tt.expectedOpsCount, len(capturedOps))

			assert.Equal(t, tt.expectedOp1, capturedOps[0].Amount.Operation,
				"first operation should be %s", tt.expectedOp1)

			if tt.expectedOpsCount == 2 {
				assert.Equal(t, tt.expectedOp2, capturedOps[1].Amount.Operation,
					"second operation should be %s", tt.expectedOp2)

				// Both operations reference the same balance alias
				assert.Equal(t, capturedOps[0].Alias, capturedOps[1].Alias,
					"both operations should reference the same alias")
			}
		})
	}
}

func TestProcessBalanceOperations_DoubleEntry_SeenDeduplication(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	ctx := context.Background()

	organizationID := uuid.MustParse("ad0032e5-ccf5-45f4-a3b2-12045e71b38a")
	ledgerID := uuid.MustParse("5d8ac48a-af68-4544-9bf8-80c3cc0715f4")

	mockRedisRepo := redis.NewMockRedisRepository(ctrl)

	uc := UseCase{
		TransactionRedisRepo: mockRedisRepo,
	}

	balanceID := uuid.New().String()
	accountID := uuid.New().String()

	// Use a transactionInput to trigger the "seen" deduplication path.
	// Asset must match the balance's AssetCode to pass validateFromBalances.
	transactionInput := &pkgTransaction.Transaction{
		Pending: true,
		Send:    pkgTransaction.Send{Asset: "USD"},
	}

	validate := &pkgTransaction.Responses{
		Aliases: []string{"alias1#default"},
		Asset:   "USD",
		Pending: true,
		From: map[string]pkgTransaction.Amount{
			"0#alias1#default": {
				Asset:                  "USD",
				Value:                  decimal.NewFromFloat(50),
				Operation:              constant.ONHOLD,
				TransactionType:        constant.PENDING,
				RouteValidationEnabled: true,
			},
		},
	}

	balances := []*mmodel.Balance{
		{
			ID:             balanceID,
			AccountID:      accountID,
			OrganizationID: organizationID.String(),
			LedgerID:       ledgerID.String(),
			Alias:          "alias1",
			Key:            "default",
			Available:      decimal.NewFromFloat(100),
			OnHold:         decimal.NewFromFloat(0),
			Version:        1,
			AccountType:    "deposit",
			AllowSending:   true,
			AllowReceiving: true,
			AssetCode:      "USD",
		},
	}

	// Construct balance operations inline: PENDING+RouteValidationEnabled splits into DEBIT+ONHOLD
	pendingAmt := validate.From["0#alias1#default"]
	op1, op2 := pkgTransaction.SplitDoubleEntryOps(pendingAmt)
	internalKey := utils.BalanceInternalKey(organizationID, ledgerID, "alias1#default")

	balanceOps := []mmodel.BalanceOperation{
		{Balance: balances[0], Alias: "0#alias1#default", Amount: op1, InternalKey: internalKey},
		{Balance: balances[0], Alias: "0#alias1#default", Amount: op2, InternalKey: internalKey},
	}

	var capturedOps []mmodel.BalanceOperation

	mockRedisRepo.EXPECT().
		ProcessBalanceAtomicOperation(
			gomock.Any(),
			organizationID,
			ledgerID,
			gomock.Any(),
			constant.PENDING,
			true, // validate.Pending
			gomock.Any(),
		).
		DoAndReturn(func(_ context.Context, _, _ uuid.UUID, _ uuid.UUID, _ string, _ bool, ops []mmodel.BalanceOperation) (*mmodel.BalanceAtomicResult, error) {
			capturedOps = ops
			return &mmodel.BalanceAtomicResult{Before: balances, After: balances}, nil
		})

	transactionID := uuid.New()
	result, err := uc.ProcessBalanceOperations(ctx, organizationID, ledgerID, transactionID, transactionInput, validate, balanceOps, constant.PENDING)

	require.NoError(t, err)
	require.NotNil(t, result)

	// Double-entry splitting produces 2 operations (DEBIT + ONHOLD) for the same alias
	require.Len(t, capturedOps, 2, "expected 2 balance operations for double-entry PENDING")

	// Both share the same alias -- the "seen" deduplication ensures only one
	// txBalance entry is created despite two operations for the same alias.
	assert.Equal(t, capturedOps[0].Alias, capturedOps[1].Alias,
		"both operations should reference the same alias")
}
