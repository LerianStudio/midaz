// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"testing"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/attribute"
	"go.uber.org/mock/gomock"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/balance"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/testutil/spantest"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	"github.com/LerianStudio/midaz/v4/pkg/mtransaction"
)

// Scenario: Lote de saldos registra contagem no pai.
func TestUpdateBalancesSpans_BatchRecordsCountOnParent(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)

	organizationID := uuid.New()
	ledgerID := uuid.New()

	balancesBefore := []*mmodel.Balance{
		{ID: "bal-1", Alias: "@alice", Available: decimal.NewFromInt(1000), OnHold: decimal.Zero, Version: 1, AccountType: "deposit"},
		{ID: "bal-2", Alias: "@bob", Available: decimal.NewFromInt(50), OnHold: decimal.Zero, Version: 4, AccountType: "deposit"},
	}

	validate := mtransaction.Responses{
		From: map[string]mtransaction.Amount{
			"@alice": {Value: decimal.NewFromInt(100), Operation: "DEBIT", TransactionType: "CREATED"},
		},
		To: map[string]mtransaction.Amount{
			"@bob": {Value: decimal.NewFromInt(100), Operation: "CREDIT", TransactionType: "CREATED"},
		},
	}

	mockBalanceRepo := balance.NewMockRepository(ctrl)
	mockBalanceRepo.EXPECT().
		BalancesUpdate(gomock.Any(), organizationID, ledgerID, gomock.Len(len(balancesBefore))).
		Return(nil).
		Times(1)

	uc := UseCase{BalanceRepo: mockBalanceRepo}

	ctx, recorder := spantest.RecordingContext()

	// nil balancesAfter takes the legacy fallback path that used to open one span per balance.
	err := uc.UpdateBalances(ctx, organizationID, ledgerID, validate, balancesBefore, nil)
	require.NoError(t, err)

	spantest.AssertNoSpan(t, recorder, "command.update_balances_new.balance")

	span := spantest.FindSpan(t, recorder, "command.update_balances_new")
	assert.Contains(t, span.Attributes(), attribute.Int("app.request.balances_count", len(balancesBefore)))
}

// Scenario: Lote de saldos registra contagem no pai (primary path, Lua after-state).
func TestUpdateBalancesSpans_PrimaryPathRecordsCountOnParent(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)

	organizationID := uuid.New()
	ledgerID := uuid.New()

	balancesBefore := []*mmodel.Balance{
		{ID: "bal-1", Alias: "@alice", Available: decimal.NewFromInt(1000), OnHold: decimal.Zero, Version: 1},
		{ID: "bal-2", Alias: "@bob", Available: decimal.NewFromInt(500), OnHold: decimal.Zero, Version: 3},
	}
	balancesAfter := []*mmodel.Balance{
		{Alias: "@alice", Available: decimal.NewFromInt(900), OnHold: decimal.Zero, Version: 2},
		{Alias: "@bob", Available: decimal.NewFromInt(600), OnHold: decimal.Zero, Version: 4},
	}

	mockBalanceRepo := balance.NewMockRepository(ctrl)
	mockBalanceRepo.EXPECT().
		BalancesUpdate(gomock.Any(), organizationID, ledgerID, gomock.Len(len(balancesBefore))).
		Return(nil).
		Times(1)

	uc := UseCase{BalanceRepo: mockBalanceRepo}

	ctx, recorder := spantest.RecordingContext()

	// Non-empty balancesAfter takes the primary path, which uses Lua's after-state directly.
	err := uc.UpdateBalances(ctx, organizationID, ledgerID, mtransaction.Responses{}, balancesBefore, balancesAfter)
	require.NoError(t, err)

	spantest.AssertNoSpan(t, recorder, "command.update_balances_new.balance")

	span := spantest.FindSpan(t, recorder, "command.update_balances_new")
	assert.Contains(t, span.Attributes(), attribute.Int("app.request.balances_count", len(balancesBefore)))
}
