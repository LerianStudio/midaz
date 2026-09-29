// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"encoding/json"
	"strings"
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
	from, to := "fee-from", "fee-to"
	input.TransactionInput.Send.Source.From[1].RouteID, input.TransactionInput.Send.Distribute.To[1].RouteID = &from, &to
	input.RouteCache = feeDebtRouteCache(constant.ActionDirect)

	transaction, projection, err := TranslateEngineTransaction(input)
	require.NoError(t, err)

	debit, credit := feeDebtPosting(t, transaction, "from:1:debit"), feeDebtPosting(t, transaction, "to:1:credit")
	assert.Equal(t, &accounting.FeeDebtRoute{ID: from, Code: "D-" + constant.ActionDirect, Description: "debit " + constant.ActionDirect}, debit.DebtRoute)
	assert.Equal(t, &accounting.FeeDebtRoute{ID: to, Code: "C-" + constant.ActionDirect, Description: "credit " + constant.ActionDirect}, credit.DebtRoute)
	assert.Nil(t, feeDebtPosting(t, transaction, "from:0:debit").DebtRoute, "only a deferrable fee leg names a debt route")
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

	routeO := &accounting.FeeDebtRoute{ID: "from-o", Code: "D-O", Description: "debit o"}
	seeds := map[string][]accounting.FeeDebtItem{"@payee#default": {
		{ID: feeDebtOriginO + ":from:1:debit", CreditRef: "@fees#default", DebitRoute: routeO, CreditRoute: &accounting.FeeDebtRoute{ID: "to-o", Code: "C-O"}},
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

			for ordinal, creditRef := range []string{"@fees#default", "@other-fees#default"} {
				debit := feeDebtContext(t, projection, collect.Ref, accounting.RoleFeeDebtDebit, uint32(ordinal))
				assert.Equal(t, "@payee#default", debit.BalanceRef)
				assert.Equal(t, constant.FEE_SETTLEMENT, debit.RowType)
				assert.Empty(t, debit.OriginRef)
				assert.Equal(t, creditRef, feeDebtContext(t, projection, collect.Ref, accounting.RoleFeeDebtCredit, uint32(ordinal)).BalanceRef)
			}

			debit, settled := feeDebtContext(t, projection, collect.Ref, accounting.RoleFeeDebtDebit, 0), feeDebtContext(t, projection, collect.Ref, accounting.RoleFeeDebtCredit, 0)
			assert.Equal(t, []string{"from-o", "D-O", "debit o"}, []string{*debit.RouteID, debit.RouteCode, debit.RouteDescription})
			assert.Equal(t, []string{"to-o", "C-O", ""}, []string{*settled.RouteID, settled.RouteCode, settled.RouteDescription})
			assert.Nil(t, feeDebtContext(t, projection, collect.Ref, accounting.RoleFeeDebtDebit, 1).RouteID, "a debt without a stored route books none")
		})
	}
}

func TestTranslateFeeDebtCollectStopsAtTheFirstUnpooledCreditor(t *testing.T) {
	t.Parallel()

	pooled := accounting.FeeDebtItem{ID: feeDebtOriginO + ":from:1:debit", CreditRef: "@fees#default"}
	missing := accounting.FeeDebtItem{ID: feeDebtOriginX + ":from:1:debit", CreditRef: "@deleted-fees#default"}
	after := accounting.FeeDebtItem{ID: feeDebtOriginY + ":from:1:debit", CreditRef: "@fees#default"}

	translate := func(seed ...accounting.FeeDebtItem) (accounting.Transaction, []OperationRecordSpec) {
		input := feeDebtFreeCases()["direct"]
		input.FeeDebtEligible = true
		input.FeeDebtSeeds = map[string][]accounting.FeeDebtItem{"@payee#default": seed}

		transaction, projection, err := TranslateEngineTransaction(input)
		require.NoError(t, err)

		return transaction, projection
	}

	transaction, projection := translate(pooled, missing, after)
	assert.Equal(t, []string{pooled.ID}, feeDebtPosting(t, transaction, "to:0:credit:collect").Items)
	assert.Equal(t, "@fees#default", feeDebtContext(t, projection, "to:0:credit:collect", accounting.RoleFeeDebtCredit, 0).BalanceRef)
	assert.Equal(t, []string{"@payee#default"}, transaction.FeeDebtRefs)

	transaction, _ = translate(missing, pooled)
	assert.NotContains(t, feeDebtPostingRefs(transaction), "to:0:credit:collect")
	assert.Empty(t, transaction.FeeDebtRefs)
}

func TestTranslateFeeDebtRevertRefundsAndReopens(t *testing.T) {
	t.Parallel()

	for _, v2 := range []bool{false, true} {
		input := feeDebtFreeCases()["revert"]
		input.FeeDebtEligible = v2
		input.TransactionInput.FeeDebtRevertedOrigins = []string{feeDebtOriginY}
		input.TransactionInput.FeeDebtExpectedRefunds = map[string]decimal.Decimal{feeDebtOriginO + ":from:1:debit": decimal.NewFromInt(25)}
		input.RouteCache = feeDebtRouteCache(constant.ActionRevert)
		input.TransactionInput.Metadata = feeDebtRevertMetadata(t,
			[]FeeDebtOpening{
				{
					DebtID: feeDebtOriginO + ":from:1:debit", DebtorRef: "@payer#default", CreditRef: "@fees#default", Opened: decimal.NewFromInt(70), Seq: 3,
					DebitRoute: &accounting.FeeDebtRoute{ID: "fee-from", RevertCode: "C-stored"}, CreditRoute: &accounting.FeeDebtRoute{ID: "fee-to", RevertCode: "D-stored"},
				},
				{DebtID: feeDebtOriginO + ":from:2:debit", DebtorRef: "@debtor#default", CreditRef: "@fees#default", Opened: decimal.NewFromInt(10), Seq: 1},
				{DebtID: feeDebtOriginO + ":from:3:debit", DebtorRef: "@payer#default", CreditRef: "@other-fees#default", Opened: decimal.NewFromInt(30), Seq: 4},
			},
			[]FeeDebtSettlement{
				{DebtID: feeDebtOriginX + ":from:1:debit", DebtorRef: "@debtor#default", CreditRef: "@fees#default", Amount: decimal.NewFromInt(5), Opened: decimal.NewFromInt(20), Seq: 7, DebitRoute: &accounting.FeeDebtRoute{ID: "x-from"}, CreditRoute: &accounting.FeeDebtRoute{ID: "x-to"}},
				{DebtID: feeDebtOriginY + ":from:1:debit", DebtorRef: "@debtor#default", CreditRef: "@fees#default", Amount: decimal.NewFromInt(4), Opened: decimal.NewFromInt(9), Seq: 2},
				{DebtID: feeDebtOriginX + ":from:1:debit", DebtorRef: "@debtor#default", CreditRef: "@fees#default", Amount: decimal.NewFromInt(7), Opened: decimal.NewFromInt(20), Seq: 7, DebitRoute: &accounting.FeeDebtRoute{ID: "x-from"}, CreditRoute: &accounting.FeeDebtRoute{ID: "x-to"}},
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
				{DebtID: feeDebtOriginO + ":from:1:debit", CreditRef: "@fees#default", Opened: decimal.NewFromInt(70), Seq: 3, ExpectedRefund: decimal.NewFromInt(25)},
				{DebtID: feeDebtOriginO + ":from:3:debit", CreditRef: "@other-fees#default", Opened: decimal.NewFromInt(30), Seq: 4},
			},
		}, feeDebtPosting(t, transaction, "fee-refund:0"))
		assert.Equal(t, "@debtor#default", feeDebtPosting(t, transaction, "fee-refund:1").BalanceRef)

		for ordinal, opened := range []int64{70, 30} {
			for _, role := range []string{accounting.RoleFeeDebtRefundCredit, accounting.RoleOverdraftCompanion, accounting.RoleFeeDebtRefundDebit} {
				leg := feeDebtContext(t, projection, "fee-refund:0", role, uint32(ordinal))
				assert.Equal(t, constant.FEE_REFUND, leg.RowType, "%s %d", role, ordinal)
				assert.True(t, leg.RequestedAmount.Equal(decimal.NewFromInt(opened)), "%s %d", role, ordinal)
			}
		}

		refundCredit := feeDebtContext(t, projection, "fee-refund:0", accounting.RoleFeeDebtRefundCredit, 0)
		companion := feeDebtContext(t, projection, "fee-refund:0", accounting.RoleOverdraftCompanion, 0)
		refundDebit := feeDebtContext(t, projection, "fee-refund:0", accounting.RoleFeeDebtRefundDebit, 0)
		assert.Equal(t, []string{"fee-from", "C-stored"}, []string{*refundCredit.RouteID, refundCredit.RouteCode}, "a refund books to the rubric its debt stored")
		assert.Equal(t, []string{"@payer#overdraft", constant.DirectionCredit, "fee-from", "C-" + constant.ActionOverdraft}, []string{companion.BalanceRef, companion.Direction, *companion.RouteID, companion.RouteCode})
		assert.Equal(t, []string{"@fees#default", "fee-to", "D-stored"}, []string{refundDebit.BalanceRef, *refundDebit.RouteID, refundDebit.RouteCode})
		assert.Nil(t, feeDebtContext(t, projection, "fee-refund:0", accounting.RoleFeeDebtRefundCredit, 1).RouteID, "an opening without a route refunds without one")
		assert.Equal(t, "@other-fees#default", feeDebtContext(t, projection, "fee-refund:0", accounting.RoleFeeDebtRefundDebit, 1).BalanceRef)

		for _, spec := range projection {
			assert.False(t, spec.PostingRef == "fee-refund:1" && spec.Role == accounting.RoleOverdraftCompanion, "a debtor without an overdraft balance has no companion")
		}

		assert.Equal(t, []accounting.FeeDebtReopen{
			{DebtID: feeDebtOriginO + ":from:9:debit", DebtorRef: "@payee#default", CreditRef: "@fees#default", Amount: decimal.NewFromInt(1), Opened: decimal.NewFromInt(3), Seq: 5},
			{
				DebtID: feeDebtOriginX + ":from:1:debit", DebtorRef: "@debtor#default", CreditRef: "@fees#default", Amount: decimal.NewFromInt(12), Opened: decimal.NewFromInt(20), Seq: 7,
				DebitRoute: &accounting.FeeDebtRoute{ID: "x-from"}, CreditRoute: &accounting.FeeDebtRoute{ID: "x-to"},
			},
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

func TestTranslateFeeDebtRevertRefusesADeletedBalance(t *testing.T) {
	t.Parallel()

	opened, one := decimal.NewFromInt(2), decimal.NewFromInt(1)
	opening := func(debtor, creditor string) []FeeDebtOpening {
		return []FeeDebtOpening{{DebtID: feeDebtOriginO + ":from:1:debit", DebtorRef: debtor, CreditRef: creditor, Opened: opened, Seq: 1}}
	}
	settled := []FeeDebtSettlement{{DebtID: feeDebtOriginX + ":from:1:debit", DebtorRef: "@gone#default", CreditRef: "@fees#default", Amount: one, Opened: opened, Seq: 1}}

	for name, metadata := range map[string]map[string]any{
		"refund debtor":   feeDebtRevertMetadata(t, opening("@gone#default", "@fees#default"), nil),
		"refund creditor": feeDebtRevertMetadata(t, opening("@payer#default", "@gone#default"), nil),
		"reopen debtor":   feeDebtRevertMetadata(t, nil, settled),
	} {
		input := feeDebtFreeCases()["revert"]
		input.TransactionInput.Metadata = metadata

		_, _, err := TranslateEngineTransaction(input)
		assert.Equal(t, "0019", errorCode(err), "%s: %v", name, err)
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

// feeDebtRouteCache resolves fee-from and fee-to under action and under overdraft,
// coding each rubric by its direction and action.
func feeDebtRouteCache(action string) *mmodel.TransactionRouteCache {
	rubric := func(direction, action string) *mmodel.AccountingRubric {
		return &mmodel.AccountingRubric{Code: strings.ToUpper(direction[:1]) + "-" + action, Description: direction + " " + action}
	}
	entry := func(action string) *mmodel.AccountingEntry {
		return &mmodel.AccountingEntry{Debit: rubric(constant.DirectionDebit, action), Credit: rubric(constant.DirectionCredit, action)}
	}
	routes := map[string]mmodel.OperationRouteCache{
		"fee-from": {AccountingEntries: &mmodel.AccountingEntries{Direct: entry(constant.ActionDirect), Revert: entry(constant.ActionRevert), Overdraft: entry(constant.ActionOverdraft)}},
	}
	routes["fee-to"] = routes["fee-from"]

	return &mmodel.TransactionRouteCache{Actions: map[string]mmodel.ActionRouteCache{
		action: {Bidirectional: routes}, constant.ActionOverdraft: {Bidirectional: routes},
	}}
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

	assert.Equal(t, map[string]int{accounting.RoleFeeDebtDebit: 2, accounting.RoleFeeDebtCredit: 2}, collected)
	assert.Contains(t, balanceSnapshotRefs(result.Final), "@other-fees#default")
}

func balanceSnapshotRefs(snapshots []accounting.BalanceSnapshot) []string {
	refs := make([]string, 0, len(snapshots))
	for _, snapshot := range snapshots {
		refs = append(refs, snapshot.BalanceRef)
	}

	return refs
}
