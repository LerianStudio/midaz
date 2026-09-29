//go:build integration

// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package engine

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/domain/accounting"
	redistestutil "github.com/LerianStudio/midaz/v4/tests/utils/redis"
)

var (
	revertParent = uuid.MustParse("0a4b6d8e-4444-4a1a-8a1a-000000000001")
	revertOther  = uuid.MustParse("0a4b6d8e-4444-4a1a-8a1a-000000000002")
)

// newRefundFixture reverts P, which charged @source two fees it could not fully
// pay: 70 to @fees (debt seq 1, 30 of it settled since) and 20 to @fees2 (debt
// seq 3, settled in full).
func newRefundFixture(t *testing.T, client redis.UniversalClient, fees, fees2 int64) *integrationFixture {
	t.Helper()

	f := newIntegrationFixture(t, client)
	f.addPoolBalance("@fees", fees, nil)
	f.addPoolBalance("@fees2", fees2, nil)
	f.declareFeeDebts(t, "@source#default")
	f.revert(revertParent)
	refund := feePosting("fee-refund:0", "@source#default", accounting.PostingRefund, "90")
	refund.Refunds = []accounting.FeeDebtRefund{
		{DebtID: revertParent.String() + ":fee-debit", CreditRef: "@fees#default", Opened: decimal.NewFromInt(70), Seq: 1, ExpectedRefund: decimal.NewFromInt(30)},
		{DebtID: revertParent.String() + ":fee-debit-2", CreditRef: "@fees2#default", Opened: decimal.NewFromInt(20), Seq: 3, ExpectedRefund: decimal.NewFromInt(20)},
	}
	f.input.Execution.Transactions[0].Postings = []accounting.Posting{
		feePosting("rev-fees", "@fees#default", accounting.PostingDebit, "30"),
		feePosting("rev-source", "@source#default", accounting.PostingCredit, "30"),
		refund,
	}

	return f
}

func TestIntegrationFeeDebtRevertRefundsWhatWasSettled(t *testing.T) {
	if testing.Short() {
		t.Skip("requires Valkey")
	}

	container := redistestutil.SetupReusableContainer(t)
	open := feeDebt(revertParent, "fee-debit", "@fees#default", "40", "70", 1)
	other := feeDebt(revertOther, "fee-debit", "@fees#default", "20", "20", 2)

	t.Run("it cancels what is open and refunds opened minus canceled", func(t *testing.T) {
		f := newRefundFixture(t, container.Client, 100, 50)
		f.seedFeeDebts(t, "@source#default", 4, open, other)

		raw, result := f.execute(t)
		require.Equal(t, []string{
			"rev-fees primary:0 @fees#default debit 30",
			"rev-source primary:0 @source#default credit 30",
			"fee-refund:0 fee_debt_refund_credit:0 @source#default credit 30",
			"fee-refund:0 fee_debt_refund_debit:0 @fees#default debit 30",
			"fee-refund:0 fee_debt_refund_credit:1 @source#default credit 20",
			"fee-refund:0 fee_debt_refund_debit:1 @fees2#default debit 20",
		}, movementLines(result))
		require.Equal(t, map[string]string{"@fees#default": "40", "@source#default": "180", "@fees2#default": "30"}, finalAvailable(result))
		require.Equal(t, []string{
			`canceled "" ` + open.ID + ` @source#default->@fees#default 40/70 seq 1 USD`,
			`refunded "fee-refund:0" ` + open.ID + ` @source#default->@fees#default 30/70 seq 1 USD`,
			`refunded "fee-refund:0" ` + revertParent.String() + `:fee-debit-2 @source#default->@fees2#default 20/20 seq 3 USD`,
		}, changeLines(result))
		require.Equal(t, integrationFeeDebtList{V: 1, NextSeq: "4", Items: []integrationFeeDebt{other}}, f.storedFeeDebts(t, "@source#default"))

		f.requireReplay(t, raw)
	})

	t.Run("a debt canceled in full refunds nothing", func(t *testing.T) {
		f := newRefundFixture(t, container.Client, 100, 50)
		f.input.Execution.Transactions[0].Postings[2].Amount = decimal.NewFromInt(70)
		f.input.Execution.Transactions[0].Postings[2].Refunds = f.input.Execution.Transactions[0].Postings[2].Refunds[:1]
		f.input.Execution.Transactions[0].Postings[2].Refunds[0].ExpectedRefund = decimal.Zero
		unpaid := feeDebt(revertParent, "fee-debit", "@fees#default", "70", "70", 1)
		f.seedFeeDebts(t, "@source#default", 2, unpaid)

		_, result := f.execute(t)
		require.Len(t, result.Movements, 2)
		require.Equal(t, []string{`canceled "" ` + unpaid.ID + ` @source#default->@fees#default 70/70 seq 1 USD`}, changeLines(result))
		require.Empty(t, f.storedFeeDebts(t, "@source#default").Items)
	})

	t.Run("a refund the fee account cannot fund refuses the revert", func(t *testing.T) {
		f := newRefundFixture(t, container.Client, 100, 10)
		f.seedFeeDebts(t, "@source#default", 4, open, other)
		f.requireUnchanged(t, "insufficient_funds", `"postingIndex":2`, `"balanceRef":"@fees2#default"`)
	})

	t.Run("a lost list holds the revert", func(t *testing.T) {
		f := newRefundFixture(t, container.Client, 100, 50)
		f.requireUnchanged(t, "fee_debt_conflict")
	})

	t.Run("a list older than its refunds holds the revert", func(t *testing.T) {
		f := newRefundFixture(t, container.Client, 100, 50)
		f.seedFeeDebts(t, "@source#default", 3, open, other)
		f.requireUnchanged(t, "fee_debt_conflict")
	})

	// The list was lost and a later deferral recreated it past P's seq, so cancel
	// finds nothing and only the expected refund shows nothing was settled.
	t.Run("a recreated list holds a revert that would refund unsettled debt", func(t *testing.T) {
		f := newRefundFixture(t, container.Client, 100, 50)
		refund := &f.input.Execution.Transactions[0].Postings[2]
		refund.Amount, refund.Refunds = decimal.NewFromInt(70), refund.Refunds[:1]
		refund.Refunds[0].ExpectedRefund = decimal.Zero
		f.seedFeeDebts(t, "@source#default", 2, feeDebt(revertOther, "fee-debit", "@fees#default", "50", "50", 1))
		f.requireUnchanged(t, "fee_debt_conflict")
	})

	t.Run("a refund repays the debtor's overdraft first", func(t *testing.T) {
		f := newRefundFixture(t, container.Client, 100, 50)
		f.input.Execution.Balances[0].Available, f.input.Execution.Balances[0].OverdraftUsed = decimal.Zero, decimal.NewFromInt(80)
		f.addCompanion("80")
		f.seedFeeDebts(t, "@source#default", 4, open, other)

		raw, result := f.execute(t)
		require.Equal(t, []string{
			"rev-fees primary:0 @fees#default debit 30",
			"rev-source primary:0 @source#default credit 0",
			"rev-source overdraft_companion:0 @source#overdraft credit 30",
			"fee-refund:0 fee_debt_refund_credit:0 @source#default credit 0",
			"fee-refund:0 overdraft_companion:0 @source#overdraft credit 30",
			"fee-refund:0 fee_debt_refund_debit:0 @fees#default debit 30",
			"fee-refund:0 fee_debt_refund_credit:1 @source#default credit 0",
			"fee-refund:0 overdraft_companion:1 @source#overdraft credit 20",
			"fee-refund:0 fee_debt_refund_debit:1 @fees2#default debit 20",
		}, movementLines(result))
		require.Equal(t, map[string]string{"@fees#default": "40", "@source#default": "0", "@source#overdraft": "0", "@fees2#default": "30"}, finalAvailable(result))
		for _, balance := range result.Final {
			require.True(t, balance.OverdraftUsed.IsZero(), "%s keeps no overdraft", balance.BalanceRef)
		}

		f.requireReplay(t, raw)
	})
}

// newReopenFixture reverts P, which settled 30 of debt seq 3 (gone since) and 15
// of debt seq 2, both owed by @source to @fees.
func newReopenFixture(t *testing.T, client redis.UniversalClient) *integrationFixture {
	t.Helper()

	f := newIntegrationFixture(t, client)
	f.addPoolBalance("@fees", 100, nil)
	f.declareFeeDebts(t, "@source#default")
	f.revert(revertParent)
	f.input.Execution.Transactions[0].Postings = []accounting.Posting{
		feePosting("rev-settlement", "@fees#default", accounting.PostingDebit, "45"),
		feePosting("rev-source", "@source#default", accounting.PostingCredit, "45"),
	}
	f.input.Execution.Transactions[0].ReopenFeeDebts = []accounting.FeeDebtReopen{
		{
			DebtID: revertOther.String() + ":fee-debit-3", DebtorRef: "@source#default", CreditRef: "@fees#default", Amount: decimal.NewFromInt(30), Opened: decimal.NewFromInt(30), Seq: 3,
			DebitRoute: debtRoute("g-from"), CreditRoute: debtRoute("g-to"),
		},
		{DebtID: revertOther.String() + ":fee-debit-2", DebtorRef: "@source#default", CreditRef: "@fees#default", Amount: decimal.NewFromInt(15), Opened: decimal.NewFromInt(20), Seq: 2},
	}

	return f
}

func TestIntegrationFeeDebtRevertReopensWhatWasSettled(t *testing.T) {
	if testing.Short() {
		t.Skip("requires Valkey")
	}

	container := redistestutil.SetupReusableContainer(t)
	second := feeDebt(revertOther, "fee-debit-2", "@fees#default", "5", "20", 2)
	fifth := feeDebt(revertOther, "fee-debit-5", "@fees#default", "10", "10", 5)

	t.Run("it restores debts in seq order", func(t *testing.T) {
		f := newReopenFixture(t, container.Client)
		f.seedFeeDebts(t, "@source#default", 6, second, fifth)

		raw, result := f.execute(t)
		require.Equal(t, []string{
			"rev-settlement primary:0 @fees#default debit 45",
			"rev-source primary:0 @source#default credit 45",
		}, movementLines(result))
		require.Equal(t, []string{
			`reopened "" ` + revertOther.String() + `:fee-debit-3 @source#default->@fees#default 30/30 seq 3 USD routes g-from|code-g-from|rubric g-from/g-to|code-g-to|rubric g-to`,
			`reopened "" ` + second.ID + ` @source#default->@fees#default 15/20 seq 2 USD`,
		}, changeLines(result))
		grown := second
		grown.Remaining = "20"
		require.Equal(t, integrationFeeDebtList{V: 1, NextSeq: "6", Items: []integrationFeeDebt{
			grown, routed(feeDebt(revertOther, "fee-debit-3", "@fees#default", "30", "30", 3), "g-from", "g-to"), fifth,
		}}, f.storedFeeDebts(t, "@source#default"))

		f.requireReplay(t, raw)
	})

	t.Run("a reopen disagreeing with its live debt is a conflict", func(t *testing.T) {
		f := newReopenFixture(t, container.Client)
		f.input.Execution.Transactions[0].ReopenFeeDebts[1].Opened = decimal.NewFromInt(25)
		f.seedFeeDebts(t, "@source#default", 6, second, fifth)
		f.requireUnchanged(t, "fee_debt_conflict")
	})

	t.Run("a gone debt newer than its list is a conflict", func(t *testing.T) {
		f := newReopenFixture(t, container.Client)
		f.seedFeeDebts(t, "@source#default", 3, second)
		f.requireUnchanged(t, "fee_debt_conflict")
	})

	// The debtor moves in no posting here, so only the reopen touches it.
	t.Run("a deleted debtor refuses the reopen", func(t *testing.T) {
		f := newReopenFixture(t, container.Client)
		f.addPoolBalance("@dest", 0, nil)
		f.input.Execution.Transactions[0].Postings[1] = feePosting("rev-dest", "@dest#default", accounting.PostingCredit, "45")
		f.seedFeeDebts(t, "@source#default", 6, second, fifth)
		require.NoError(t, f.client.Set(context.Background(), f.resolved.Balances["@source#default"].Deleted, "1", time.Hour).Err())
		f.requireUnchanged(t, "balance_deleted", `"postingIndex":-1`)
	})
}
