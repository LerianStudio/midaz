// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"encoding/json"
	"slices"
	"testing"

	constant "github.com/LerianStudio/lib-commons/v7/commons/constants"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/operation"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/transaction"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/domain/accounting"
	pkgConstant "github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	"github.com/LerianStudio/midaz/v4/pkg/mtransaction"
)

// feeDebtPreparationReader serves a fixed balance catalog and fee-debt seeds,
// recording every seed read and every explicit balance load.
type feeDebtPreparationReader struct {
	enginePreparationReader
	seeds   map[string][]accounting.FeeDebtItem
	debtors *[][]string
}

func (reader feeDebtPreparationReader) GetFeeDebtSeeds(_ context.Context, _, _ uuid.UUID, refs []string) (map[string][]accounting.FeeDebtItem, error) {
	*reader.debtors = append(*reader.debtors, refs)

	return reader.seeds, nil
}

type feeDebtPreparationRun struct {
	prepared enginePreparedTransaction
	debtors  [][]string
	loaded   []string
}

func prepareFeeDebtFixture(t *testing.T, input enginePreparationInput, catalog []*mmodel.Balance, seeds map[string][]accounting.FeeDebtItem) feeDebtPreparationRun {
	t.Helper()

	var run feeDebtPreparationRun

	reads := 0
	reader := feeDebtPreparationReader{seeds: seeds, debtors: &run.debtors, enginePreparationReader: enginePreparationReader{
		load: func(_ context.Context, _, _ uuid.UUID, aliases []string) ([]*mmodel.Balance, error) {
			if reads++; reads == 1 {
				run.loaded = aliases
			}

			var found []*mmodel.Balance

			for _, balance := range catalog {
				if slices.Contains(aliases, mtransaction.AliasKey(balance.Alias, balance.Key)) {
					found = append(found, balance)
				}
			}

			return found, nil
		},
		routes: func(context.Context, uuid.UUID, uuid.UUID, []mmodel.BalanceOperation, *mtransaction.Responses, string) (*mmodel.TransactionRouteCache, error) {
			return nil, nil
		},
	}}

	prepared, err := (&UseCase{TransactionReader: reader}).prepareEngineTransaction(context.Background(), input)
	require.NoError(t, err)

	run.prepared = prepared

	return run
}

func feeDebtPreparationCatalog(t *testing.T) (enginePreparationInput, []*mmodel.Balance) {
	t.Helper()

	input, balances := enginePreparationFixture(t)
	fees := translationBalance(input.organizationID, input.ledgerID, "88888888-8888-4888-8888-888888888888", "@fees", "default")

	return input, append(balances, fees)
}

func TestPrepareFeeDebtCollectLoadsTheSeededCreditors(t *testing.T) {
	t.Parallel()

	seeds := map[string][]accounting.FeeDebtItem{"@target#default": {{ID: feeDebtOriginO + ":from:1:debit", CreditRef: "@fees#default"}}}

	input, catalog := feeDebtPreparationCatalog(t)
	input.translation.FeeDebtEligible = true

	run := prepareFeeDebtFixture(t, input, catalog, seeds)

	assert.Equal(t, [][]string{{"@target#default"}}, run.debtors, "one seed read names every credited balance")
	assert.ElementsMatch(t, []string{"@source#default", "@target#default", "@fees#default"}, run.loaded)
	assert.Len(t, run.prepared.pool.ExplicitBalances, 2, "a seeded creditor is pooled, never an explicit leg")
	assert.Contains(t, balanceSnapshotRefs(run.prepared.pool.Snapshots), "@fees#default")
	assert.Equal(t, "to:0:credit:collect", feeDebtPosting(t, run.prepared.transaction, "to:0:credit:collect").Ref)
	assert.Equal(t, "@fees#default", feeDebtContext(t, run.prepared.projection, "to:0:credit:collect", accounting.RoleFeeDebtCredit, 0).BalanceRef)
}

func TestPrepareFeeDebtV1IgnoresSeeds(t *testing.T) {
	t.Parallel()

	seeds := map[string][]accounting.FeeDebtItem{"@target#default": {{ID: feeDebtOriginO + ":from:1:debit", CreditRef: "@fees#default"}}}

	input, catalog := feeDebtPreparationCatalog(t)
	run := prepareFeeDebtFixture(t, input, catalog, seeds)

	assert.Empty(t, run.debtors)
	assert.ElementsMatch(t, []string{"@source#default", "@target#default"}, run.loaded)
	assert.Equal(t, []string{"from:0:debit", "from:1:debit", "to:0:credit"}, feeDebtPostingRefs(run.prepared.transaction))
	assert.Empty(t, run.prepared.transaction.FeeDebtRefs)
}

func TestPrepareFeeDebtFreeV2MatchesV1(t *testing.T) {
	t.Parallel()

	for action, status := range map[string]string{
		pkgConstant.ActionDirect: pkgConstant.CREATED, pkgConstant.ActionCommit: pkgConstant.APPROVED, pkgConstant.ActionRevert: pkgConstant.CREATED,
	} {
		t.Run(action, func(t *testing.T) {
			t.Parallel()

			prepare := func(v2 bool) ([]byte, []string) {
				input, catalog := feeDebtPreparationCatalog(t)
				input.translation.Action, input.translation.TransactionStatus, input.translation.FeeDebtEligible = action, status, v2

				run := prepareFeeDebtFixture(t, input, catalog, nil)

				encoded, err := json.Marshal(map[string]any{
					"pool": run.prepared.pool, "transaction": run.prepared.transaction, "projection": run.prepared.projection,
				})
				require.NoError(t, err)

				return encoded, run.loaded
			}

			v1, v1Loaded := prepare(false)
			v2, v2Loaded := prepare(true)

			assert.JSONEq(t, string(v1), string(v2), "an execution with no debt composes today's request")
			assert.Equal(t, v1Loaded, v2Loaded)
		})
	}
}

func TestPrepareFeeDebtV1RevertRefundsWithoutCollect(t *testing.T) {
	t.Parallel()

	seeds := map[string][]accounting.FeeDebtItem{"@source#default": {{ID: feeDebtOriginX + ":from:1:debit", CreditRef: "@fees#default"}}}

	input, catalog := feeDebtPreparationCatalog(t)
	input.translation.Action = pkgConstant.ActionRevert
	input.translation.TransactionInput.Metadata = feeDebtRevertMetadata(t, []FeeDebtOpening{{
		DebtID: feeDebtOriginO + ":from:1:debit", DebtorRef: "@target#default", CreditRef: "@fees#default", Opened: decimal.NewFromInt(2), Seq: 1,
	}}, nil)

	run := prepareFeeDebtFixture(t, input, catalog, seeds)

	assert.Empty(t, run.debtors, "a v1 revert collects nothing")
	assert.ElementsMatch(t, []string{"@source#default", "@target#default", "@fees#default"}, run.loaded)
	assert.Equal(t, []string{"from:0:debit", "from:1:debit", "to:0:credit", "fee-refund:0"}, feeDebtPostingRefs(run.prepared.transaction))
	assert.Equal(t, "@target#default", feeDebtPosting(t, run.prepared.transaction, "fee-refund:0").BalanceRef)
	assert.Equal(t, []string{"@target#default"}, run.prepared.transaction.FeeDebtRefs)
	assert.Empty(t, run.prepared.transaction.ReopenFeeDebts)
}

func TestPrepareFeeDebtRevertTouchesAFoldedDebtor(t *testing.T) {
	t.Parallel()

	input, catalog := feeDebtPreparationCatalog(t)
	settled := FeeDebtSettlement{
		DebtID: feeDebtOriginO + ":from:1:debit", DebtorRef: "@target#default", CreditRef: "@fees#default",
		Amount: decimal.NewFromInt(20), Opened: decimal.NewFromInt(20), Seq: 1,
	}

	row := func(kind, alias, direction string) *operation.Operation {
		value := decimal.NewFromInt(20)

		return &operation.Operation{
			Type: kind, Direction: direction, AccountAlias: alias, BalanceKey: pkgConstant.DefaultBalanceKey,
			AssetCode: "USD", Amount: operation.Amount{Value: &value},
		}
	}

	amount := decimal.NewFromInt(20)
	credited := transaction.Transaction{
		AssetCode: "USD", Amount: &amount, Metadata: feeDebtRevertMetadata(t, nil, []FeeDebtSettlement{settled}),
		Operations: []*operation.Operation{
			row(constant.DEBIT, "@source", pkgConstant.DirectionDebit), row(constant.CREDIT, "@target", pkgConstant.DirectionCredit),
			row(pkgConstant.FEE_SETTLEMENT, "@target", pkgConstant.DirectionDebit), row(pkgConstant.FEE_SETTLEMENT, "@fees", pkgConstant.DirectionCredit),
		},
	}

	reversal := credited.TransactionRevert(nil)
	normalizeTransactionSendLegs(&reversal)

	validate, err := mtransaction.ValidateSendSourceAndDistribute(context.Background(), reversal, pkgConstant.CREATED)
	require.NoError(t, err)

	input.translation.Action, input.translation.FeeDebtEligible = pkgConstant.ActionRevert, true
	input.translation.TransactionInput, input.translation.Validate = reversal, validate

	run := prepareFeeDebtFixture(t, input, catalog, nil)

	for _, posting := range run.prepared.transaction.Postings {
		assert.NotEqual(t, "@target#default", posting.BalanceRef, "the folded debtor moves nothing")
	}

	assert.Contains(t, balanceSnapshotRefs(run.prepared.pool.Snapshots), "@target#default", "the reopen debtor is touched")
	assert.Equal(t, []accounting.FeeDebtReopen{{
		DebtID: settled.DebtID, DebtorRef: settled.DebtorRef, CreditRef: settled.CreditRef, Amount: settled.Amount, Opened: settled.Opened, Seq: settled.Seq,
	}}, run.prepared.transaction.ReopenFeeDebts)
	assert.Contains(t, run.prepared.transaction.FeeDebtRefs, "@target#default")
	assert.Equal(t, [][]string{{"@source#default"}}, run.debtors)
}
