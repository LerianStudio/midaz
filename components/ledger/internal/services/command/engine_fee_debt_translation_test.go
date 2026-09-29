// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"encoding/json"
	"testing"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/domain/accounting"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
)

const (
	feeDebtOriginO = "0193a1b2-0000-7000-8000-000000000001"
	feeDebtOriginX = "0193a1b2-0000-7000-8000-000000000002"
	feeDebtOriginY = "0193a1b2-0000-7000-8000-000000000003"
)

func TestTranslateFeeDebtDeferralPairsBothLegsOnV2Direct(t *testing.T) {
	t.Parallel()

	input := feeDebtFreeCases()["direct_deferrable_v1"]
	input.FeeDebtEligible = true

	transaction, projection, err := TranslateEngineTransaction(input)
	require.NoError(t, err)

	debit, credit := feeDebtPosting(t, transaction, "from:1:debit"), feeDebtPosting(t, transaction, "to:1:credit")
	assert.True(t, debit.DeferShortfall)
	assert.Equal(t, "from:1:debit", credit.FundedByRef)
	assert.False(t, feeDebtPosting(t, transaction, "from:0:debit").DeferShortfall)
	assert.Empty(t, feeDebtPosting(t, transaction, "to:0:credit").FundedByRef)
	assert.Equal(t, []string{"@payer#default"}, transaction.FeeDebtRefs)

	for _, ref := range []string{"from:1:debit", "to:1:credit"} {
		primary := feeDebtContext(t, projection, ref, accounting.RolePrimary, 0)
		assert.Equal(t, "0:0#@payer#default", primary.Metadata[constant.MetadataKeyFeeDeferPair], ref)
		assert.Equal(t, OperationRecordStandard, primary.CompatibilityPath, ref)
	}
}

func TestTranslateFeeDebtCollectFollowsEligibleCredits(t *testing.T) {
	t.Parallel()

	seeds := map[string][]accounting.FeeDebtItem{"@payee#default": {
		{ID: feeDebtOriginO + ":from:1:debit", CreditRef: "@fees#default"},
		{ID: feeDebtOriginX + ":from:1:debit", CreditRef: "@other-fees#default"},
	}}

	for _, tc := range []struct {
		name, base string
		v2         bool
		collect    bool
	}{
		{name: "v2 direct", base: "direct", v2: true, collect: true},
		{name: "v2 commit", base: "commit", v2: true, collect: true},
		{name: "v2 revert", base: "revert", v2: true, collect: true},
		{name: "v1 direct", base: "direct"},
		{name: "v1 revert", base: "revert"},
		{name: "v2 hold", base: "hold_deferrable", v2: true},
		{name: "v2 cancel", base: "cancel", v2: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			input := feeDebtFreeCases()[tc.base]
			input.FeeDebtEligible, input.FeeDebtSeeds = tc.v2, seeds
			input.Balances = append(input.Balances, feeDebtBalance("@other-fees"))

			transaction, projection, err := TranslateEngineTransaction(input)
			require.NoError(t, err)

			if !tc.collect {
				for _, posting := range transaction.Postings {
					assert.NotEqual(t, accounting.PostingCollect, posting.Type)
				}

				assert.Empty(t, transaction.FeeDebtRefs)

				return
			}

			refs := feeDebtPostingRefs(transaction)
			collectAt := indexOf(refs, "to:0:credit:collect")
			require.Positive(t, collectAt, "refs %v", refs)
			assert.Equal(t, "to:0:credit", refs[collectAt-1])

			credit, collect := transaction.Postings[collectAt-1], transaction.Postings[collectAt]
			assert.Equal(t, accounting.Posting{
				Ref: "to:0:credit:collect", BalanceRef: "@payee#default", Type: accounting.PostingCollect, Amount: credit.Amount,
				DrawPolicy: accounting.DrawForbidden, OverdraftAmount: decimal.Zero,
				Items: []string{feeDebtOriginO + ":from:1:debit", feeDebtOriginX + ":from:1:debit"},
			}, collect)
			assert.Equal(t, []string{"@payee#default"}, transaction.FeeDebtRefs)

			debit := feeDebtContext(t, projection, collect.Ref, accounting.RoleFeeDebtDebit, 0)
			assert.Equal(t, "@payee#default", debit.BalanceRef)
			assert.Equal(t, constant.FEE_SETTLEMENT, debit.RowType)
			assert.Empty(t, debit.OriginRef)
			assert.Equal(t, "@fees#default", feeDebtContext(t, projection, collect.Ref, accounting.RoleFeeDebtCredit, 0).BalanceRef)
			assert.Equal(t, "@other-fees#default", feeDebtContext(t, projection, collect.Ref, accounting.RoleFeeDebtCredit, 1).BalanceRef)
		})
	}
}

func TestTranslateFeeDebtCollectStopsAtTheFirstUnpooledCreditor(t *testing.T) {
	t.Parallel()

	pooled := accounting.FeeDebtItem{ID: feeDebtOriginO + ":from:1:debit", CreditRef: "@fees#default"}
	missing := accounting.FeeDebtItem{ID: feeDebtOriginX + ":from:1:debit", CreditRef: "@deleted-fees#default"}
	after := accounting.FeeDebtItem{ID: feeDebtOriginY + ":from:1:debit", CreditRef: "@fees#default"}

	for name, seed := range map[string][]accounting.FeeDebtItem{"truncated": {pooled, missing, after}, "empty": {missing, pooled}} {
		input := feeDebtFreeCases()["direct"]
		input.FeeDebtEligible = true
		input.FeeDebtSeeds = map[string][]accounting.FeeDebtItem{"@payee#default": seed}

		transaction, projection, err := TranslateEngineTransaction(input)
		require.NoError(t, err, name)

		if name == "empty" {
			assert.NotContains(t, feeDebtPostingRefs(transaction), "to:0:credit:collect")
			assert.Empty(t, transaction.FeeDebtRefs)

			continue
		}

		assert.Equal(t, []string{pooled.ID}, feeDebtPosting(t, transaction, "to:0:credit:collect").Items)
		assert.Equal(t, "@fees#default", feeDebtContext(t, projection, "to:0:credit:collect", accounting.RoleFeeDebtCredit, 0).BalanceRef)
		assert.Equal(t, []string{"@payee#default"}, transaction.FeeDebtRefs)
	}
}

func TestTranslateFeeDebtRevertRefundsAndReopens(t *testing.T) {
	t.Parallel()

	for _, v2 := range []bool{false, true} {
		input := feeDebtFreeCases()["revert"]
		input.FeeDebtEligible = v2
		input.TransactionInput.FeeDebtRevertedOrigins = []string{feeDebtOriginY}
		input.TransactionInput.Metadata = feeDebtRevertMetadata(t,
			[]FeeDebtOpening{
				{DebtID: feeDebtOriginO + ":from:1:debit", DebtorRef: "@payer#default", CreditRef: "@fees#default", Opened: decimal.NewFromInt(70), Seq: 3},
				{DebtID: feeDebtOriginO + ":from:2:debit", DebtorRef: "@debtor#default", CreditRef: "@fees#default", Opened: decimal.NewFromInt(10), Seq: 1},
				{DebtID: feeDebtOriginO + ":from:3:debit", DebtorRef: "@payer#default", CreditRef: "@other-fees#default", Opened: decimal.NewFromInt(30), Seq: 4},
			},
			[]FeeDebtSettlement{
				{DebtID: feeDebtOriginX + ":from:1:debit", DebtorRef: "@debtor#default", CreditRef: "@fees#default", Amount: decimal.NewFromInt(5), Opened: decimal.NewFromInt(20), Seq: 7},
				{DebtID: feeDebtOriginY + ":from:1:debit", DebtorRef: "@debtor#default", CreditRef: "@fees#default", Amount: decimal.NewFromInt(4), Opened: decimal.NewFromInt(9), Seq: 2},
				{DebtID: feeDebtOriginX + ":from:1:debit", DebtorRef: "@debtor#default", CreditRef: "@fees#default", Amount: decimal.NewFromInt(7), Opened: decimal.NewFromInt(20), Seq: 7},
				{DebtID: feeDebtOriginO + ":from:9:debit", DebtorRef: "@payee#default", CreditRef: "@fees#default", Amount: decimal.NewFromInt(1), Opened: decimal.NewFromInt(3), Seq: 5},
			})
		input.Balances = append(input.Balances, feeDebtBalance("@other-fees"), feeDebtBalance("@debtor"))

		transaction, projection, err := TranslateEngineTransaction(input)
		require.NoError(t, err)

		refs := feeDebtPostingRefs(transaction)
		assert.Equal(t, []string{"fee-refund:0", "fee-refund:1"}, refs[len(refs)-2:], "v2=%v", v2)

		assert.Equal(t, accounting.Posting{
			Ref: "fee-refund:0", BalanceRef: "@payer#default", Type: accounting.PostingRefund, Amount: decimal.NewFromInt(100),
			DrawPolicy: accounting.DrawForbidden, OverdraftAmount: decimal.Zero,
			Refunds: []accounting.FeeDebtRefund{
				{DebtID: feeDebtOriginO + ":from:1:debit", CreditRef: "@fees#default", Opened: decimal.NewFromInt(70), Seq: 3},
				{DebtID: feeDebtOriginO + ":from:3:debit", CreditRef: "@other-fees#default", Opened: decimal.NewFromInt(30), Seq: 4},
			},
		}, feeDebtPosting(t, transaction, "fee-refund:0"))
		assert.Equal(t, "@debtor#default", feeDebtPosting(t, transaction, "fee-refund:1").BalanceRef)

		refundCredit := feeDebtContext(t, projection, "fee-refund:0", accounting.RoleFeeDebtRefundCredit, 0)
		assert.Equal(t, constant.FEE_REFUND, refundCredit.RowType)
		assert.True(t, refundCredit.RequestedAmount.Equal(decimal.NewFromInt(100)))
		assert.Equal(t, "@other-fees#default", feeDebtContext(t, projection, "fee-refund:0", accounting.RoleFeeDebtRefundDebit, 1).BalanceRef)

		assert.Equal(t, []accounting.FeeDebtReopen{
			{DebtID: feeDebtOriginO + ":from:9:debit", DebtorRef: "@payee#default", CreditRef: "@fees#default", Amount: decimal.NewFromInt(1), Opened: decimal.NewFromInt(3), Seq: 5},
			{DebtID: feeDebtOriginX + ":from:1:debit", DebtorRef: "@debtor#default", CreditRef: "@fees#default", Amount: decimal.NewFromInt(12), Opened: decimal.NewFromInt(20), Seq: 7},
		}, transaction.ReopenFeeDebts, "v2=%v", v2)
		assert.Equal(t, []string{"@payer#default", "@debtor#default", "@payee#default"}, transaction.FeeDebtRefs, "v2=%v", v2)
	}
}

func TestTranslateFeeDebtRevertRefusesMalformedMetadata(t *testing.T) {
	t.Parallel()

	for _, value := range []any{[]any{"not", "a", "string"}, "{not json"} {
		input := feeDebtFreeCases()["revert"]
		input.TransactionInput.Metadata = map[string]any{constant.MetadataKeyFeeDebtSettlements: value}

		_, _, err := TranslateEngineTransaction(input)
		require.ErrorIs(t, err, ErrInvalidEngineTranslation, "%v", value)
	}
}

func feeDebtRevertMetadata(t *testing.T, openings []FeeDebtOpening, settlements []FeeDebtSettlement) map[string]any {
	t.Helper()

	encodedOpenings, err := json.Marshal(openings)
	require.NoError(t, err)

	encodedSettlements, err := json.Marshal(settlements)
	require.NoError(t, err)

	return map[string]any{
		constant.MetadataKeyFeeDebtOpenings:    string(encodedOpenings),
		constant.MetadataKeyFeeDebtSettlements: string(encodedSettlements),
	}
}

func feeDebtBalance(alias string) *mmodel.Balance {
	organizationID := uuid.MustParse("11111111-1111-4111-8111-111111111111")
	ledgerID := uuid.MustParse("22222222-2222-4222-8222-222222222222")

	return translationBalance(organizationID, ledgerID, uuid.NewSHA1(ledgerID, []byte(alias)).String(), alias, "default")
}

func feeDebtPostingRefs(transaction accounting.Transaction) []string {
	refs := make([]string, 0, len(transaction.Postings))
	for _, posting := range transaction.Postings {
		refs = append(refs, posting.Ref)
	}

	return refs
}

func feeDebtPosting(t *testing.T, transaction accounting.Transaction, ref string) accounting.Posting {
	t.Helper()

	for _, posting := range transaction.Postings {
		if posting.Ref == ref {
			return posting
		}
	}

	t.Fatalf("no posting %s in %v", ref, feeDebtPostingRefs(transaction))

	return accounting.Posting{}
}

func feeDebtContext(t *testing.T, projection []OperationRecordSpec, postingRef, role string, ordinal uint32) OperationRecordSpec {
	t.Helper()

	for _, spec := range projection {
		if spec.PostingRef == postingRef && spec.Role == role && spec.Ordinal == ordinal {
			return spec
		}
	}

	t.Fatalf("no %s context %d for %s", role, ordinal, postingRef)

	return OperationRecordSpec{}
}

func indexOf(values []string, value string) int {
	for index, candidate := range values {
		if candidate == value {
			return index
		}
	}

	return -1
}

func TestAtomicBatchBudgetCountsEveryFeeDebtMovement(t *testing.T) {
	t.Parallel()

	input := feeDebtFreeCases()["direct"]
	input.FeeDebtEligible = true
	input.Balances = append(input.Balances, feeDebtBalance("@other-fees"))
	input.FeeDebtSeeds = map[string][]accounting.FeeDebtItem{"@payee#default": {
		{ID: feeDebtOriginO + ":from:1:debit", CreditRef: "@fees#default"},
		{ID: feeDebtOriginX + ":from:1:debit", CreditRef: "@other-fees#default"},
	}}

	translated, projection, err := TranslateEngineTransaction(input)
	require.NoError(t, err)

	snapshots := make([]accounting.BalanceSnapshot, 0, len(input.Balances))
	for _, balance := range input.Balances {
		snapshot, err := balanceToEngineSnapshot(uuid.MustParse(balance.OrganizationID), uuid.MustParse(balance.LedgerID), balance)
		require.NoError(t, err)

		snapshots = append(snapshots, snapshot)
	}

	result := atomicTransactionBatchBudgetResult(atomicTransactionBatchItemRun{
		transactionID: input.TransactionID,
		prepared:      enginePreparedTransaction{pool: EngineSnapshotPool{Snapshots: snapshots}, transaction: translated, projection: projection},
	})

	assert.Len(t, result.Movements, len(projection), "one measured movement per context")

	collected := make(map[string]int)
	for _, movement := range result.Movements {
		if movement.PostingRef == "to:0:credit:collect" {
			collected[movement.Role]++
		}
	}

	assert.Equal(t, map[string]int{accounting.RoleFeeDebtDebit: 1, accounting.RoleFeeDebtCredit: 2}, collected)
	assert.Contains(t, balanceSnapshotRefs(result.Final), "@other-fees#default")
}

func balanceSnapshotRefs(snapshots []accounting.BalanceSnapshot) []string {
	refs := make([]string, 0, len(snapshots))
	for _, snapshot := range snapshots {
		refs = append(refs, snapshot.BalanceRef)
	}

	return refs
}
