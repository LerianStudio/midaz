// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"errors"
	"testing"
	"time"

	libCommons "github.com/LerianStudio/lib-commons/v4/commons"
	"github.com/LerianStudio/midaz/v3/components/ledger/internal/adapters/postgres/balance"
	redis "github.com/LerianStudio/midaz/v3/components/ledger/internal/adapters/redis/transaction"
	"github.com/LerianStudio/midaz/v3/pkg"
	"github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/LerianStudio/midaz/v3/pkg/mtransaction"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

// TestTransactionCreate_RejectsInternalBalance verifies that a transaction
// which targets a balance whose scope is "internal" is rejected with
// ErrDirectOperationOnInternalBalance (0168) BEFORE it is published to the
// Redis transaction queue.
//
// The scope guard protects system-managed balances (e.g. overdraft reserves)
// from client-initiated operations. Enforcement lives in the transaction
// command use case so that every entry point — HTTP, gRPC, DSL — benefits
// from the same guarantee.
func TestTransactionCreate_RejectsInternalBalance(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	orgID := uuid.Must(libCommons.GenerateUUIDv7())
	ledgerID := uuid.Must(libCommons.GenerateUUIDv7())
	transactionID := uuid.Must(libCommons.GenerateUUIDv7())

	internalBalance := &mmodel.Balance{
		ID:             uuid.New().String(),
		OrganizationID: orgID.String(),
		LedgerID:       ledgerID.String(),
		AccountID:      uuid.New().String(),
		Alias:          "@indebted",
		Key:            "overdraft",
		AssetCode:      "USD",
		Direction:      constant.DirectionDebit,
		Available:      decimal.NewFromInt(0),
		OnHold:         decimal.NewFromInt(0),
		Version:        1,
		Settings: &mmodel.BalanceSettings{
			BalanceScope: mmodel.BalanceScopeInternal,
		},
	}

	mockBalanceRepo := balance.NewMockRepository(ctrl)
	mockRedisRepo := redis.NewMockRedisRepository(ctrl)

	// The scope guard MUST run BEFORE any Redis queue publishing.
	mockRedisRepo.EXPECT().
		AddMessageToQueue(gomock.Any(), gomock.Any(), gomock.Any()).
		Times(0)

	uc := UseCase{
		BalanceRepo:          mockBalanceRepo,
		TransactionRedisRepo: mockRedisRepo,
	}

	// Transaction targeting the overdraft (internal) balance.
	validate := &mtransaction.Responses{
		From: map[string]mtransaction.Amount{
			"@indebted#overdraft": {
				Value:           decimal.NewFromInt(100),
				Operation:       "DEBIT",
				TransactionType: "CREATED",
			},
		},
	}

	input := mtransaction.Transaction{
		ChartOfAccountsGroupName: "test",
	}

	// When the resolved balance slice contains an internal-scope balance,
	// the use case MUST refuse to enqueue the transaction and surface the
	// 0168 error.
	err := uc.SendTransactionToRedisQueue(
		context.TODO(),
		orgID,
		ledgerID,
		transactionID,
		input,
		validate,
		constant.CREATED,
		"CREATED",
		time.Now(),
		[]*mmodel.Balance{internalBalance},
	)

	require.Error(t, err, "transactions against an internal balance MUST be rejected")

	var vErr pkg.UnprocessableOperationError
	require.True(t, errors.As(err, &vErr),
		"scope protection MUST surface an UnprocessableOperationError")
	assert.Equal(t, constant.ErrDirectOperationOnInternalBalance.Error(), vErr.Code,
		"error code MUST be 0168 (ErrDirectOperationOnInternalBalance)")
}

// TestBalanceDelete_RejectsInternalBalance verifies that attempting to delete
// a balance whose scope is "internal" is rejected with
// ErrDeletionOfInternalBalance (0169) BEFORE the repository Delete is called.
func TestBalanceDelete_RejectsInternalBalance(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	orgID := uuid.Must(libCommons.GenerateUUIDv7())
	ledgerID := uuid.Must(libCommons.GenerateUUIDv7())
	balanceID := uuid.Must(libCommons.GenerateUUIDv7())

	internalBalance := &mmodel.Balance{
		ID:             balanceID.String(),
		OrganizationID: orgID.String(),
		LedgerID:       ledgerID.String(),
		AccountID:      uuid.New().String(),
		Alias:          "@indebted",
		Key:            "overdraft",
		Available:      decimal.Zero,
		OnHold:         decimal.Zero,
		Direction:      constant.DirectionDebit,
		Settings: &mmodel.BalanceSettings{
			BalanceScope: mmodel.BalanceScopeInternal,
		},
	}

	mockBalanceRepo := balance.NewMockRepository(ctrl)

	mockBalanceRepo.EXPECT().
		Find(gomock.Any(), orgID, ledgerID, balanceID).
		Return(internalBalance, nil).
		Times(1)

	// Delete MUST NOT be invoked for internal-scope balances.
	mockBalanceRepo.EXPECT().
		Delete(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Times(0)

	uc := UseCase{
		BalanceRepo: mockBalanceRepo,
	}

	err := uc.DeleteBalance(context.TODO(), orgID, ledgerID, balanceID)

	require.Error(t, err, "deleting an internal-scope balance MUST be rejected")

	var vErr pkg.UnprocessableOperationError
	require.True(t, errors.As(err, &vErr),
		"scope protection MUST surface an UnprocessableOperationError")
	assert.Equal(t, constant.ErrDeletionOfInternalBalance.Error(), vErr.Code,
		"error code MUST be 0169 (ErrDeletionOfInternalBalance)")
}
