// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"

	tmcore "github.com/LerianStudio/lib-commons/v7/commons/tenant-manager/core"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	mongodb "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/mongodb/transaction"
	postgresOperation "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/operation"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/domain/accounting"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	"github.com/LerianStudio/midaz/v4/pkg/mtransaction"
)

const (
	feeDebtTransaction     = "12121212-1212-4121-8121-121212121212"
	feeDebtSettledFirst    = "13131313-1313-4131-8131-131313131313:from:1"
	feeDebtSettledSecond   = "14141414-1414-4141-8141-141414141414:from:0"
	feeDebtCollectPosting  = "to:0:collect"
	feeDebtOpeningsGolden  = `[{"debtId":"12121212-1212-4121-8121-121212121212:from:1","debtorRef":"@payer#default","creditRef":"@fees#default","opened":"50","seq":"4",` + `"debitRoute":{"id":"fee-from","code":"FD","description":"fee debit"},"creditRoute":{"id":"fee-to","code":"FC","description":"fee credit"}}]`
	feeDebtSettledGolden   = `[{"debtId":"13131313-1313-4131-8131-131313131313:from:1","debtorRef":"@dest#default","creditRef":"@fees#default","amount":"25","opened":"25","seq":"2",` + `"debitRoute":{"id":"old-from","code":"OD","description":"old debit"},"creditRoute":{"id":"old-to","code":"OC","description":"old credit"}},` + `{"debtId":"14141414-1414-4141-8141-141414141414:from:0","debtorRef":"@dest#default","creditRef":"@fees#default","amount":"5","opened":"30","seq":"3"}]`
	feeDebtAppliedAtMicros = 1_789_999_999_123_456
)

type feeDebtRecorderStub struct {
	calls   *[]string
	records []FeeDebtRecord
	err     error
	settled map[string]decimal.Decimal
	asked   []string
}

func (recorder *feeDebtRecorderStub) Settled(_ context.Context, _, _ uuid.UUID, debtIDs []string) (map[string]decimal.Decimal, error) {
	recorder.asked = append(recorder.asked, debtIDs...)

	return recorder.settled, recorder.err
}

func (recorder *feeDebtRecorderStub) Apply(_ context.Context, record FeeDebtRecord) error {
	*recorder.calls = append(*recorder.calls, "record")
	recorder.records = append(recorder.records, record)

	return recorder.err
}

func (*feeDebtRecorderStub) HasOpenCreditor(context.Context, uuid.UUID, uuid.UUID, []string) (bool, error) {
	return false, nil
}

func feeDebtRowBalance(alias string, index int) OperationBalanceContext {
	return OperationBalanceContext{
		ID: fmt.Sprintf("a1a1a1a1-0000-4000-8000-%012d", index), AccountID: fmt.Sprintf("b2b2b2b2-0000-4000-8000-%012d", index),
		OrganizationID: "33333333-3333-4333-8333-333333333333", LedgerID: "44444444-4444-4444-8444-444444444444",
		Alias: alias, Key: "default", AssetCode: "USD", AccountType: "deposit", Direction: constant.DirectionCredit,
	}
}

// feeDebtSpec builds a context the way translation does: a primary is anchored on its own
// origin, a collect or refund context has none and carries no metadata.
func feeDebtSpec(tx uuid.UUID, postingRef, role string, ordinal uint32, balance OperationBalanceContext, rowType, direction string, requested int64, metadata map[string]any) OperationRecordSpec {
	side, origin := OperationSpecSideTo, ""
	if direction == constant.DirectionDebit {
		side = OperationSpecSideFrom
	}

	if role == accounting.RolePrimary {
		origin = postingRef
	}

	return OperationRecordSpec{
		TransactionID: tx, PostingRef: postingRef, BalanceRef: balance.Alias + "#" + balance.Key, Role: role, Ordinal: ordinal, OriginRef: origin,
		Side: side, RowType: rowType, Direction: direction, Metadata: metadata, Balance: balance,
		RequestedAmount: decimal.NewFromInt(requested), CompatibilityPath: OperationRecordStandard,
	}
}

// movementRef is the ref the engine records: <txId>:<len(postingRef)>:<postingRef>:<role>:<ordinal>.
func movementRef(tx uuid.UUID, postingRef, role string, ordinal uint32) string {
	return fmt.Sprintf("%s:%d:%s:%s:%d", tx, len(postingRef), postingRef, role, ordinal)
}

func feeDebtMovement(tx uuid.UUID, postingRef, role string, ordinal uint32, balanceRef string, postingType accounting.PostingType, amount, before, after, version int64) accounting.Movement {
	return accounting.Movement{
		Ref: movementRef(tx, postingRef, role, ordinal), TransactionID: tx,
		PostingRef: postingRef, Role: role, BalanceRef: balanceRef, Type: postingType, Amount: decimal.NewFromInt(amount),
		Before: accounting.BalanceState{Available: decimal.NewFromInt(before), Version: version},
		After:  accounting.BalanceState{Available: decimal.NewFromInt(after), Version: version + 1},
	}
}

// routedSpec books spec under route, as translation does for a fee-debt movement.
func routedSpec(spec OperationRecordSpec, route *accounting.FeeDebtRoute) OperationRecordSpec {
	id := route.ID
	spec.RouteID, spec.RouteCode, spec.RouteDescription = &id, route.Code, route.Description

	return spec
}

func withRoutes(change accounting.FeeDebtChange, debit, credit *accounting.FeeDebtRoute) accounting.FeeDebtChange {
	change.DebitRoute, change.CreditRoute = debit, credit

	return change
}

func feeDebtChange(tx uuid.UUID, postingRef string, kind accounting.FeeDebtChangeKind, debtID, debtorRef string, seq, amount, opened int64) accounting.FeeDebtChange {
	return accounting.FeeDebtChange{
		TransactionID: tx, PostingRef: postingRef, Kind: kind, DebtID: debtID, DebtorRef: debtorRef, CreditRef: "@fees#default",
		OriginTransactionID: uuid.MustParse(debtID[:36]), Seq: seq, AssetCode: "USD", Amount: decimal.NewFromInt(amount), Opened: decimal.NewFromInt(opened),
	}
}

// deferredFeeFixture is one direct transaction of 100: the payer (50 available) pays a 30
// principal and 20 of a 70 deferrable fee, opening a 50 debt, while the credited destination
// settles 25 and 5 of two older debts; the first seeded item was already gone, so the
// settlements land on ordinals 1 and 2, the first under the routes its debt stored.
func deferredFeeFixture(t testing.TB) (TransactionCompletionPlan, accounting.ExecutionResult) {
	t.Helper()
	payload, _ := recoveryContractFixture(t)
	tx := uuid.MustParse(feeDebtTransaction)
	payload.TransactionID = tx
	payload.TransactionInput.Send.Value = decimal.NewFromInt(100)
	payload.TransactionInput.Metadata = map[string]any{"purpose": "fee debt", "packageAppliedID": "package-1"}
	payer, dest, fees := feeDebtRowBalance("@payer", 1), feeDebtRowBalance("@dest", 2), feeDebtRowBalance("@fees", 3)
	pair := map[string]any{constant.MetadataKeyFeeDeferPair: "pair-0"}
	feeLeg := map[string]any{constant.MetadataKeyFeeLeg: "true", constant.MetadataKeyFeeDeferPair: "pair-0"}
	oldFrom := &accounting.FeeDebtRoute{ID: "old-from", Code: "OD", Description: "old debit"}
	oldTo := &accounting.FeeDebtRoute{ID: "old-to", Code: "OC", Description: "old credit"}
	payload.OperationSpecs = []OperationRecordSpec{
		feeDebtSpec(tx, "from:0", accounting.RolePrimary, 0, payer, constant.DEBIT, constant.DirectionDebit, 30, nil),
		feeDebtSpec(tx, "from:1", accounting.RolePrimary, 0, payer, constant.DEBIT, constant.DirectionDebit, 70, pair),
		feeDebtSpec(tx, "to:0", accounting.RolePrimary, 0, dest, constant.CREDIT, constant.DirectionCredit, 30, nil),
		feeDebtSpec(tx, feeDebtCollectPosting, accounting.RoleFeeDebtDebit, 0, dest, constant.FEE_SETTLEMENT, constant.DirectionDebit, 30, nil),
		feeDebtSpec(tx, feeDebtCollectPosting, accounting.RoleFeeDebtCredit, 0, fees, constant.FEE_SETTLEMENT, constant.DirectionCredit, 30, nil),
		routedSpec(feeDebtSpec(tx, feeDebtCollectPosting, accounting.RoleFeeDebtDebit, 1, dest, constant.FEE_SETTLEMENT, constant.DirectionDebit, 30, nil), oldFrom),
		routedSpec(feeDebtSpec(tx, feeDebtCollectPosting, accounting.RoleFeeDebtCredit, 1, fees, constant.FEE_SETTLEMENT, constant.DirectionCredit, 30, nil), oldTo),
		feeDebtSpec(tx, feeDebtCollectPosting, accounting.RoleFeeDebtDebit, 2, dest, constant.FEE_SETTLEMENT, constant.DirectionDebit, 30, nil),
		feeDebtSpec(tx, feeDebtCollectPosting, accounting.RoleFeeDebtCredit, 2, fees, constant.FEE_SETTLEMENT, constant.DirectionCredit, 30, nil),
		feeDebtSpec(tx, "to:1", accounting.RolePrimary, 0, fees, constant.CREDIT, constant.DirectionCredit, 70, feeLeg),
	}
	result := accounting.ExecutionResult{AppliedAtUnixMicro: feeDebtAppliedAtMicros, Movements: []accounting.Movement{
		feeDebtMovement(tx, "from:0", accounting.RolePrimary, 0, "@payer#default", accounting.PostingDebit, 30, 50, 20, 0),
		feeDebtMovement(tx, "from:1", accounting.RolePrimary, 0, "@payer#default", accounting.PostingDebit, 20, 20, 0, 1),
		feeDebtMovement(tx, "to:0", accounting.RolePrimary, 0, "@dest#default", accounting.PostingCredit, 30, 0, 30, 0),
		feeDebtMovement(tx, feeDebtCollectPosting, accounting.RoleFeeDebtDebit, 1, "@dest#default", accounting.PostingDebit, 25, 30, 5, 1),
		feeDebtMovement(tx, feeDebtCollectPosting, accounting.RoleFeeDebtCredit, 1, "@fees#default", accounting.PostingCredit, 25, 0, 25, 0),
		feeDebtMovement(tx, feeDebtCollectPosting, accounting.RoleFeeDebtDebit, 2, "@dest#default", accounting.PostingDebit, 5, 5, 0, 2),
		feeDebtMovement(tx, feeDebtCollectPosting, accounting.RoleFeeDebtCredit, 2, "@fees#default", accounting.PostingCredit, 5, 25, 30, 1),
		feeDebtMovement(tx, "to:1", accounting.RolePrimary, 0, "@fees#default", accounting.PostingCredit, 20, 30, 50, 2),
	}, FeeDebt: []accounting.FeeDebtChange{
		withRoutes(feeDebtChange(tx, feeDebtCollectPosting, accounting.FeeDebtSettled, feeDebtSettledFirst, "@dest#default", 2, 25, 25), oldFrom, oldTo),
		feeDebtChange(tx, feeDebtCollectPosting, accounting.FeeDebtSettled, feeDebtSettledSecond, "@dest#default", 3, 5, 30),
		withRoutes(feeDebtChange(tx, "from:1", accounting.FeeDebtOpened, feeDebtTransaction+":from:1", "@payer#default", 4, 50, 50),
			&accounting.FeeDebtRoute{ID: "fee-from", Code: "FD", Description: "fee debit"}, &accounting.FeeDebtRoute{ID: "fee-to", Code: "FC", Description: "fee credit"}),
	}}
	result.Final = recoveryContractFinal(payload, result.Movements)

	return payload, result
}

func withoutFeeDebtMovements(payload TransactionCompletionPlan, result accounting.ExecutionResult, postingRefs ...string) accounting.ExecutionResult {
	result.Movements = slices.DeleteFunc(slices.Clone(result.Movements), func(movement accounting.Movement) bool {
		return slices.Contains(postingRefs, movement.PostingRef)
	})
	result.Final = recoveryContractFinal(payload, result.Movements)

	return result
}

func feeDebtRowRoute(row *postgresOperation.Operation) string {
	if row.RouteID == nil {
		return ""
	}

	code := ""
	if row.RouteCode != nil {
		code = *row.RouteCode
	}

	return *row.RouteID + " " + code
}

func feeDebtRowViews(rows []*postgresOperation.Operation) []string {
	views := make([]string, 0, len(rows))
	for _, row := range rows {
		views = append(views, fmt.Sprintf("%s %s %s %s->%s", row.Type, row.AccountAlias, row.Amount.Value, row.Balance.Available, row.BalanceAfter.Available))
	}

	return views
}

func TestFeeDebtWriteSetRowsAmountAndMetadata(t *testing.T) {
	payload, result := deferredFeeFixture(t)

	writeSet, err := BuildTransactionWriteSet(payload, result)
	require.NoError(t, err)

	rows := writeSet.Transaction.Operations
	assert.Equal(t, []string{
		"DEBIT @payer 30 50->20", "DEBIT @payer 20 20->0", "CREDIT @dest 30 0->30",
		"FEE_SETTLEMENT @dest 25 30->5", "FEE_SETTLEMENT @fees 25 0->25", "FEE_SETTLEMENT @dest 5 5->0", "FEE_SETTLEMENT @fees 5 25->30",
		"CREDIT @fees 20 30->50",
	}, feeDebtRowViews(rows), "a partly paid fee writes its paid part and a collect writes a debit and a credit per settled item")

	for index, ordinal := range []uint32{1, 2} {
		for side, role := range []string{accounting.RoleFeeDebtDebit, accounting.RoleFeeDebtCredit} {
			id, err := DeterministicOperationID(payload.ExecutionID, payload.TransactionID, feeDebtCollectPosting, role, ordinal)
			require.NoError(t, err)
			assert.Equal(t, id.String(), rows[3+2*index+side].ID, "a settlement row is keyed by the ordinal its movement ref carries")
		}
	}

	routes := make([]string, 0, 4)
	for _, row := range rows[3:7] {
		assert.Empty(t, row.Metadata, "fee-debt rows carry no metadata")
		routes = append(routes, feeDebtRowRoute(row))
	}

	assert.Equal(t, []string{"old-from OD", "old-to OC", "", ""}, routes, "each settlement books under the routes its debt stored")

	assert.True(t, writeSet.Transaction.Amount.Equal(decimal.NewFromInt(50)), "the amount is what executed: 100 minus the 50 opened")
	assert.Equal(t, map[string]any{
		"purpose": "fee debt", "packageAppliedID": "package-1",
		constant.MetadataKeyFeeDebtOpenings: feeDebtOpeningsGolden, constant.MetadataKeyFeeDebtSettlements: feeDebtSettledGolden,
	}, writeSet.Transaction.Metadata)
	assert.Len(t, payload.TransactionInput.Metadata, 2, "the frozen plan metadata is not mutated")
}

func TestFeeDebtCollectionNeedsItsEmptySource(t *testing.T) {
	payload, result := deferredFeeFixture(t)
	payload.TransactionInput.Metadata[constant.MetadataKeyFeeDebtCollection] = "true"
	payload.TransactionInput.Send.Source.From = []mtransaction.FromTo{{AccountAlias: "@payer#default"}}
	payload.Validate = &mtransaction.Responses{Destinations: []string{"@dest#default", "@idle#default"}}

	writeSet, err := BuildTransactionWriteSet(payload, result)
	require.NoError(t, err)
	assert.Equal(t, "50", writeSet.Transaction.Amount.String(), "the mark alone keeps a client transaction's executed amount")
	assert.Equal(t, []string{"@dest", "@idle"}, writeSet.Transaction.Destination)

	payload.TransactionInput.Send.Source.From = nil
	writeSet, err = BuildTransactionWriteSet(payload, result)
	require.NoError(t, err)
	assert.Equal(t, "30", writeSet.Transaction.Amount.String(), "a collection's amount is what it settled")
	assert.Equal(t, []string{"@fees"}, writeSet.Transaction.Destination, "only fee accounts a settlement credited")
}

func TestFeeDebtCompletionRelaxesOnlyTheOpenedPair(t *testing.T) {
	payload, result := deferredFeeFixture(t)
	unpaid := withoutFeeDebtMovements(payload, result, "from:1", "to:1")
	unpaid.FeeDebt = slices.Clone(unpaid.FeeDebt)
	unpaid.FeeDebt[2].Amount, unpaid.FeeDebt[2].Opened = decimal.NewFromInt(70), decimal.NewFromInt(70)

	writeSet, err := BuildTransactionWriteSet(payload, unpaid)
	require.NoError(t, err, "an unpaid deferrable fee has no movement on its debit or its paired credit")
	assert.Len(t, writeSet.Transaction.Operations, 6)
	assert.True(t, writeSet.Transaction.Amount.Equal(decimal.NewFromInt(30)))

	inherited := unpaid
	inherited.FeeDebt = unpaid.FeeDebt[:2]
	strictPrincipal := withoutFeeDebtMovements(payload, result, "from:0")
	foreignChange := result
	foreignChange.FeeDebt = slices.Clone(result.FeeDebt)
	foreignChange.FeeDebt[0].TransactionID = uuid.MustParse("15151515-1515-4151-8151-151515151515")
	paddedOrdinal := result
	paddedOrdinal.Movements = slices.Clone(result.Movements)
	paddedOrdinal.Movements[4].Ref = paddedOrdinal.Movements[4].Ref[:len(paddedOrdinal.Movements[4].Ref)-1] + "01"
	unknownOrdinal := result
	unknownOrdinal.Movements = slices.Clone(result.Movements)
	unknownOrdinal.Movements[4].Ref = unknownOrdinal.Movements[4].Ref[:len(unknownOrdinal.Movements[4].Ref)-1] + "7"

	missing, correlation := "primary operation spec has no movement", "invalid per-transaction movement correlation"
	for _, refused := range []struct {
		name, reason string
		result       accounting.ExecutionResult
	}{
		{"token without an opened change", missing, inherited},
		{"primary outside the opened pair", missing, strictPrincipal},
		{"change of another transaction", "fee-debt change belongs to another transaction", foreignChange},
		{"non-canonical ordinal", correlation, paddedOrdinal},
		{"ordinal without a context", correlation, unknownOrdinal},
	} {
		t.Run(refused.name, func(t *testing.T) {
			_, err := BuildTransactionWriteSet(payload, refused.result)
			require.ErrorIs(t, err, ErrInvalidTransactionCompletionRecord)
			assert.ErrorContains(t, err, refused.reason)
		})
	}
}

func TestFeeDebtRevertWritesRefundRowsAndStripsInheritedKeys(t *testing.T) {
	payload, _ := recoveryContractFixture(t)
	tx, parent := uuid.MustParse(feeDebtTransaction), uuid.MustParse("16161616-1616-4161-8161-161616161616")
	payload.TransactionID, payload.ParentTransactionID, payload.Action = tx, &parent, constant.ActionRevert
	payload.TransactionInput.Metadata = map[string]any{
		"purpose": "revert", constant.MetadataKeyFeeDebtOpenings: feeDebtOpeningsGolden, constant.MetadataKeyFeeDebtSettlements: feeDebtSettledGolden,
	}
	payer, dest, fees := feeDebtRowBalance("@payer", 1), feeDebtRowBalance("@dest", 2), feeDebtRowBalance("@fees", 3)
	refund, settled, open := "fee-refund:0", parent.String()+":from:1", parent.String()+":from:2"
	payload.OperationSpecs = []OperationRecordSpec{
		feeDebtSpec(tx, "from:0", accounting.RolePrimary, 0, dest, constant.DEBIT, constant.DirectionDebit, 30, nil),
		feeDebtSpec(tx, "to:0", accounting.RolePrimary, 0, payer, constant.CREDIT, constant.DirectionCredit, 30, map[string]any{constant.MetadataKeyFeeDeferPair: "inherited"}),
		feeDebtSpec(tx, refund, accounting.RoleFeeDebtRefundCredit, 0, payer, constant.FEE_REFUND, constant.DirectionCredit, 70, nil),
		feeDebtSpec(tx, refund, accounting.RoleFeeDebtRefundDebit, 0, fees, constant.FEE_REFUND, constant.DirectionDebit, 70, nil),
		feeDebtSpec(tx, refund, accounting.RoleFeeDebtRefundCredit, 1, payer, constant.FEE_REFUND, constant.DirectionCredit, 50, nil),
		feeDebtSpec(tx, refund, accounting.RoleFeeDebtRefundDebit, 1, fees, constant.FEE_REFUND, constant.DirectionDebit, 50, nil),
	}
	result := accounting.ExecutionResult{Movements: []accounting.Movement{
		feeDebtMovement(tx, "from:0", accounting.RolePrimary, 0, "@dest#default", accounting.PostingDebit, 30, 30, 0, 0),
		feeDebtMovement(tx, "to:0", accounting.RolePrimary, 0, "@payer#default", accounting.PostingCredit, 30, 0, 30, 0),
		feeDebtMovement(tx, refund, accounting.RoleFeeDebtRefundCredit, 0, "@payer#default", accounting.PostingCredit, 70, 30, 100, 1),
		feeDebtMovement(tx, refund, accounting.RoleFeeDebtRefundDebit, 0, "@fees#default", accounting.PostingDebit, 70, 100, 30, 0),
	}, FeeDebt: []accounting.FeeDebtChange{
		feeDebtChange(tx, "", accounting.FeeDebtCanceled, open, "@payer#default", 5, 50, 50),
		feeDebtChange(tx, refund, accounting.FeeDebtRefunded, settled, "@payer#default", 4, 70, 70),
	}}
	result.Final = recoveryContractFinal(payload, result.Movements)

	writeSet, err := BuildTransactionWriteSet(payload, result)
	require.NoError(t, err)

	assert.Equal(t, []string{"DEBIT @dest 30 30->0", "CREDIT @payer 30 0->30", "FEE_REFUND @payer 70 30->100", "FEE_REFUND @fees 70 100->30"},
		feeDebtRowViews(writeSet.Transaction.Operations), "a refund entry that refunds 0 has no movement and writes no row")
	assert.Equal(t, map[string]any{"purpose": "revert"}, writeSet.Transaction.Metadata, "a revert keeps none of the keys it inherited")
	assert.True(t, writeSet.Transaction.Amount.Equal(decimal.NewFromInt(30)))

	_, own, err := completedTransactionFeeDebt(payload, []accounting.FeeDebtChange{
		feeDebtChange(tx, feeDebtCollectPosting, accounting.FeeDebtSettled, feeDebtSettledFirst, "@dest#default", 2, 25, 25),
	})
	require.NoError(t, err)
	assert.Equal(t, map[string]any{
		"purpose": "revert", constant.MetadataKeyFeeDebtSettlements: `[{"debtId":"` + feeDebtSettledFirst + `","debtorRef":"@dest#default","creditRef":"@fees#default","amount":"25","opened":"25","seq":"2"}]`,
	}, own, "a revert writes only what its own result settled")
}

// TestFeeDebtRefundRepaysTheDebtorOverdraft uses the engine's refund shapes: a debtor with 30
// of overdraft used takes the refund into the overdraft first, so its credit may be 0, and the
// overdraft companion mirrors the repayment; both write FEE_REFUND rows.
func TestFeeDebtRefundRepaysTheDebtorOverdraft(t *testing.T) {
	payload, _ := recoveryContractFixture(t)
	tx, parent := uuid.MustParse(feeDebtTransaction), uuid.MustParse("16161616-1616-4161-8161-161616161616")
	payload.TransactionID, payload.ParentTransactionID, payload.Action = tx, &parent, constant.ActionRevert
	payload.TransactionInput.Metadata = map[string]any{"purpose": "revert"}
	payer, fees, overdraft := feeDebtRowBalance("@payer", 1), feeDebtRowBalance("@fees", 3), feeDebtRowBalance("@payer", 4)
	overdraft.AccountID, overdraft.Key, overdraft.Direction = payer.AccountID, constant.OverdraftBalanceKey, constant.DirectionDebit
	refund, settled := "fee-refund:0", parent.String()+":from:1"

	for _, tc := range []struct {
		name                   string
		refund, credited, left int64
		rows                   []string
	}{
		{"smaller than the overdraft", 20, 0, 10, []string{"FEE_REFUND @payer 0 0->0", "FEE_REFUND @payer 20 30->10", "FEE_REFUND @fees 20 100->80"}},
		{"equal to the overdraft", 30, 0, 0, []string{"FEE_REFUND @payer 0 0->0", "FEE_REFUND @payer 30 30->0", "FEE_REFUND @fees 30 100->70"}},
		{"larger than the overdraft", 50, 20, 0, []string{"FEE_REFUND @payer 20 0->20", "FEE_REFUND @payer 30 30->0", "FEE_REFUND @fees 50 100->50"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repaid := 30 - tc.left
			payload.OperationSpecs = []OperationRecordSpec{
				feeDebtSpec(tx, refund, accounting.RoleFeeDebtRefundCredit, 0, payer, constant.FEE_REFUND, constant.DirectionCredit, tc.refund, nil),
				feeDebtSpec(tx, refund, accounting.RoleOverdraftCompanion, 0, overdraft, constant.FEE_REFUND, constant.DirectionCredit, tc.refund, nil),
				feeDebtSpec(tx, refund, accounting.RoleFeeDebtRefundDebit, 0, fees, constant.FEE_REFUND, constant.DirectionDebit, tc.refund, nil),
			}
			credit := feeDebtMovement(tx, refund, accounting.RoleFeeDebtRefundCredit, 0, "@payer#default", accounting.PostingCredit, tc.credited, 0, tc.credited, 1)
			credit.Before.OverdraftUsed, credit.After.OverdraftUsed, credit.OverdraftDelta = decimal.NewFromInt(30), decimal.NewFromInt(tc.left), decimal.NewFromInt(-repaid)
			companion := feeDebtMovement(tx, refund, accounting.RoleOverdraftCompanion, 0, "@payer#overdraft", accounting.PostingCredit, repaid, 30, tc.left, 0)
			result := accounting.ExecutionResult{Movements: []accounting.Movement{
				credit, companion,
				feeDebtMovement(tx, refund, accounting.RoleFeeDebtRefundDebit, 0, "@fees#default", accounting.PostingDebit, tc.refund, 100, 100-tc.refund, 0),
			}, FeeDebt: []accounting.FeeDebtChange{feeDebtChange(tx, refund, accounting.FeeDebtRefunded, settled, "@payer#default", 4, tc.refund, tc.refund)}}
			result.Final = recoveryContractFinal(payload, result.Movements)

			writeSet, err := BuildTransactionWriteSet(payload, result)
			require.NoError(t, err)
			assert.Equal(t, tc.rows, feeDebtRowViews(writeSet.Transaction.Operations))
			assert.Equal(t, mmodel.OperationSnapshot{OverdraftUsedBefore: "30", OverdraftUsedAfter: fmt.Sprint(tc.left)}, writeSet.Transaction.Operations[1].Snapshot,
				"the companion row carries the debtor's overdraft, like any companion")

			unpaired := result
			unpaired.Movements = []accounting.Movement{credit, result.Movements[2]}
			unpaired.Final = recoveryContractFinal(payload, unpaired.Movements)
			_, err = BuildTransactionWriteSet(payload, unpaired)
			require.ErrorIs(t, err, ErrInvalidTransactionCompletionRecord)
			assert.ErrorContains(t, err, "debt change has no matching companion movement")
		})
	}
}

// TestFeeDebtRefundCompanionKeepsItsEntryOrdinal refunds two debts of one debtor: the first
// refunds 0 and moves nothing, the second repays the debtor's 30 of overdraft, so its
// companion is entry 1's, booked under entry 1's routes, not the first companion counted.
func TestFeeDebtRefundCompanionKeepsItsEntryOrdinal(t *testing.T) {
	payload, _ := recoveryContractFixture(t)
	tx, parent := uuid.MustParse(feeDebtTransaction), uuid.MustParse("16161616-1616-4161-8161-161616161616")
	payload.TransactionID, payload.ParentTransactionID, payload.Action = tx, &parent, constant.ActionRevert
	payload.TransactionInput.Metadata = map[string]any{"purpose": "revert"}
	payer, fees, overdraft := feeDebtRowBalance("@payer", 1), feeDebtRowBalance("@fees", 3), feeDebtRowBalance("@payer", 4)
	overdraft.AccountID, overdraft.Key, overdraft.Direction = payer.AccountID, constant.OverdraftBalanceKey, constant.DirectionDebit
	refund, from, to := "fee-refund:0", &accounting.FeeDebtRoute{ID: "fee-from", Code: "RC"}, &accounting.FeeDebtRoute{ID: "fee-to", Code: "RD"}
	payload.OperationSpecs = []OperationRecordSpec{
		feeDebtSpec(tx, refund, accounting.RoleFeeDebtRefundCredit, 0, payer, constant.FEE_REFUND, constant.DirectionCredit, 10, nil),
		feeDebtSpec(tx, refund, accounting.RoleOverdraftCompanion, 0, overdraft, constant.FEE_REFUND, constant.DirectionCredit, 10, nil),
		feeDebtSpec(tx, refund, accounting.RoleFeeDebtRefundDebit, 0, fees, constant.FEE_REFUND, constant.DirectionDebit, 10, nil),
		routedSpec(feeDebtSpec(tx, refund, accounting.RoleFeeDebtRefundCredit, 1, payer, constant.FEE_REFUND, constant.DirectionCredit, 50, nil), from),
		routedSpec(feeDebtSpec(tx, refund, accounting.RoleOverdraftCompanion, 1, overdraft, constant.FEE_REFUND, constant.DirectionCredit, 50, nil), from),
		routedSpec(feeDebtSpec(tx, refund, accounting.RoleFeeDebtRefundDebit, 1, fees, constant.FEE_REFUND, constant.DirectionDebit, 50, nil), to),
	}
	credit := feeDebtMovement(tx, refund, accounting.RoleFeeDebtRefundCredit, 1, "@payer#default", accounting.PostingCredit, 20, 0, 20, 1)
	credit.Before.OverdraftUsed, credit.OverdraftDelta = decimal.NewFromInt(30), decimal.NewFromInt(-30)
	result := accounting.ExecutionResult{Movements: []accounting.Movement{
		credit,
		feeDebtMovement(tx, refund, accounting.RoleOverdraftCompanion, 1, "@payer#overdraft", accounting.PostingCredit, 30, 30, 0, 0),
		feeDebtMovement(tx, refund, accounting.RoleFeeDebtRefundDebit, 1, "@fees#default", accounting.PostingDebit, 50, 100, 50, 0),
	}}
	result.Final = recoveryContractFinal(payload, result.Movements)

	writeSet, err := BuildTransactionWriteSet(payload, result)
	require.NoError(t, err)

	rows := writeSet.Transaction.Operations
	assert.Equal(t, []string{"FEE_REFUND @payer 20 0->20", "FEE_REFUND @payer 30 30->0", "FEE_REFUND @fees 50 100->50"}, feeDebtRowViews(rows))
	assert.Equal(t, []string{"fee-from RC", "fee-from RC", "fee-to RD"}, []string{feeDebtRowRoute(rows[0]), feeDebtRowRoute(rows[1]), feeDebtRowRoute(rows[2])})
	assert.Equal(t, mmodel.OperationSnapshot{OverdraftUsedBefore: "30", OverdraftUsedAfter: "0"}, rows[1].Snapshot)

	id, err := DeterministicOperationID(payload.ExecutionID, payload.TransactionID, refund, accounting.RoleOverdraftCompanion, 1)
	require.NoError(t, err)
	assert.Equal(t, id.String(), rows[1].ID, "the companion row is keyed by the ordinal its movement ref carries")
}

func TestFeeDebtWriteSetLeavesNoDebtTransactionUnchanged(t *testing.T) {
	payload, result := recoveryContractFixture(t)
	payload.TransactionInput.Metadata = map[string]any{"purpose": "plain"}

	writeSet, err := BuildTransactionWriteSet(payload, result)
	require.NoError(t, err)

	assert.Equal(t, fmt.Sprintf("%p", payload.TransactionInput.Metadata), fmt.Sprintf("%p", writeSet.Transaction.Metadata), "no fee debt keeps the frozen map itself")
	assert.True(t, writeSet.Transaction.Amount.Equal(payload.TransactionInput.Send.Value))
}

func feeDebtCompletionService(t *testing.T, calls *[]string) (*TransactionCompletionService, *finalizationOutcomeStoreStub, *finalizationMetadataStub, *feeDebtRecorderStub) {
	t.Helper()
	store := &finalizationOutcomeStoreStub{
		outcome: TransactionPersistenceOutcome{TransactionStatus: constant.APPROVED, LifecyclePhase: TransactionLifecyclePhaseCreated},
		bulkOutcomes: []TransactionPersistenceOutcome{
			{TransactionStatus: constant.APPROVED, LifecyclePhase: TransactionLifecyclePhaseCreated},
			{TransactionStatus: constant.APPROVED, LifecyclePhase: TransactionLifecyclePhaseCreated},
		},
		calls: calls,
	}
	metadata := &finalizationMetadataStub{calls: calls, data: make(map[string]*mongodb.Metadata)}
	recorder := &feeDebtRecorderStub{calls: calls}
	service, err := NewTransactionCompletionServiceWithEvents(store, metadata, &finalizationEventPublisherStub{calls: calls})
	require.NoError(t, err)

	return service.WithFeeDebtRecorder(recorder), store, metadata, recorder
}

// assertRecordedAfterMetadataAndEvents proves the recorder ran once per fee-debt unit, after
// every event and every metadata confirmation.
func assertRecordedAfterMetadataAndEvents(t *testing.T, calls []string, records int) {
	t.Helper()
	lastFind, lastPublish, firstRecord, recorded := -1, -1, len(calls), 0
	for index, call := range calls {
		switch {
		case call == "record":
			recorded++
			firstRecord = min(firstRecord, index)
		case call == "publish":
			lastPublish = index
		case strings.HasPrefix(call, "find:"):
			lastFind = index
		}
	}

	assert.Equal(t, records, recorded)
	assert.Contains(t, calls, "publish")
	assert.Less(t, lastFind, firstRecord, "fee debts are recorded after metadata is confirmed")
	assert.Less(t, lastPublish, firstRecord, "fee debts are recorded after the events are published")
}

func TestFeeDebtCompletionRecordsAndReplaysIdentically(t *testing.T) {
	payload, result := deferredFeeFixture(t)
	envelope := recoveryContractEnvelope(t, payload, result)
	ctx := tmcore.ContextWithTenantID(context.Background(), payload.TenantID)
	calls := []string{}
	service, store, metadata, recorder := feeDebtCompletionService(t, &calls)

	require.NoError(t, completionError(service.Complete(ctx, &envelope)))
	assertRecordedAfterMetadataAndEvents(t, calls, 1)

	encoded, err := EncodeTransactionCompletionRecord(envelope)
	require.NoError(t, err)
	recovered, err := DecodeTransactionCompletionRecord(encoded)
	require.NoError(t, err)
	require.NoError(t, completionError(service.Complete(ctx, recovered)), "recovery replays the persisted record")

	require.Len(t, store.records, 2)
	first, err := json.Marshal(store.records[0])
	require.NoError(t, err)
	replay, err := json.Marshal(store.records[1])
	require.NoError(t, err)
	assert.JSONEq(t, string(first), string(replay), "recovery writes the same rows and metadata")

	require.Len(t, recorder.records, 2)
	record := recorder.records[0]
	assert.Equal(t, payload.OrganizationID, record.OrganizationID)
	assert.Equal(t, payload.LedgerID, record.LedgerID)
	assert.Equal(t, "package-1", record.FeePackageID)
	assert.Equal(t, int64(feeDebtAppliedAtMicros), record.AppliedAt.UnixMicro())
	firstRecord, err := json.Marshal(recorder.records[0])
	require.NoError(t, err)
	replayRecord, err := json.Marshal(recorder.records[1])
	require.NoError(t, err)
	assert.JSONEq(t, string(firstRecord), string(replayRecord), "recovery hands the recorder the same record")
	changes, err := json.Marshal(result.FeeDebt)
	require.NoError(t, err)
	recorded, err := json.Marshal(record.Changes)
	require.NoError(t, err)
	assert.JSONEq(t, string(changes), string(recorded))

	stored := metadata.data[constant.EntityTransaction+":"+feeDebtTransaction].Data
	assert.Equal(t, feeDebtOpeningsGolden, stored[constant.MetadataKeyFeeDebtOpenings])
	assert.Equal(t, feeDebtSettledGolden, stored[constant.MetadataKeyFeeDebtSettlements])
}

func TestFeeDebtMetadataMatchesEvidenceLookupByteForByte(t *testing.T) {
	payload, result := deferredFeeFixture(t)
	envelope := recoveryContractEnvelope(t, payload, result)
	calls := []string{}
	service, store, metadata, _ := feeDebtCompletionService(t, &calls)

	views, err := BuildTransactionEvidenceViews(envelope)
	require.NoError(t, err)
	require.NoError(t, completionError(service.Complete(tmcore.ContextWithTenantID(context.Background(), payload.TenantID), &envelope)))

	stored := metadata.data[constant.EntityTransaction+":"+feeDebtTransaction].Data
	for _, key := range []string{constant.MetadataKeyFeeDebtOpenings, constant.MetadataKeyFeeDebtSettlements} {
		lookup, isText := views.Lookup.Metadata[key].(string)
		require.True(t, isText, key)
		mongo, isText := stored[key].(string)
		require.True(t, isText, key)
		assert.Equal(t, []byte(lookup), []byte(mongo), "%s: the evidence route and the Mongo route resolve the same bytes", key)
		assert.Equal(t, lookup, store.records[0].Transaction.Metadata[key], key)
	}
}

func TestFeeDebtCompletionBulkRecordsEachUnit(t *testing.T) {
	payload, result := deferredFeeFixture(t)
	feeDebt := recoveryContractEnvelope(t, payload, result)
	plainPayload, plainResult := recoveryContractFixture(t)
	plain := recoveryContractEnvelope(t, plainPayload, plainResult)
	calls := []string{}
	service, _, _, recorder := feeDebtCompletionService(t, &calls)

	_, err := service.CompleteBulk(tmcore.ContextWithTenantID(context.Background(), payload.TenantID), []*TransactionCompletionRecord{&plain, &feeDebt})
	require.NoError(t, err)

	assertRecordedAfterMetadataAndEvents(t, calls, 1)
	require.Len(t, recorder.records, 1, "a unit without fee debt is not recorded")
	assert.Len(t, recorder.records[0].Changes, 3)
}

func TestFeeDebtCompletionRecorderFailureFollowsPublishedEvents(t *testing.T) {
	payload, result := deferredFeeFixture(t)
	envelope := recoveryContractEnvelope(t, payload, result)
	calls := []string{}
	service, _, _, recorder := feeDebtCompletionService(t, &calls)
	recorder.err = errors.New("fees unavailable")

	require.ErrorIs(t, completionError(service.Complete(tmcore.ContextWithTenantID(context.Background(), payload.TenantID), &envelope)), recorder.err)
	assert.Equal(t, 1, slices.Index(calls, "publish"), "events leave once SQL commits, before the failing projection")
	assert.Equal(t, "record", calls[len(calls)-1])
}

func TestFeeDebtCompletionWithoutDebtOrRecorderIsUnchanged(t *testing.T) {
	ctx, envelope := finalizationFixture(t)
	calls := []string{}
	service, _, _, recorder := feeDebtCompletionService(t, &calls)

	require.NoError(t, completionError(service.Complete(ctx, envelope)))
	assert.Equal(t, []string{
		"sql-with-outcome", "publish", "create:" + constant.EntityTransaction, "find:" + constant.EntityTransaction,
		"create:" + constant.EntityOperation, "find:" + constant.EntityOperation,
	}, calls, "a transaction without fee debt neither records nor updates metadata")
	assert.Empty(t, recorder.records)

	payload, result := deferredFeeFixture(t)
	feeDebt := recoveryContractEnvelope(t, payload, result)
	plain, _, _, _ := finalizationDependencies()
	require.NoError(t, completionError(plain.Complete(ctx, &feeDebt)), "a nil recorder skips the projection")
}

func TestFeeDebtCommitMergesIntoPendingMetadata(t *testing.T) {
	payload, result := deferredFeeFixture(t)
	payload.Action = constant.ActionCommit
	envelope := recoveryContractEnvelope(t, payload, result)
	ctx := tmcore.ContextWithTenantID(context.Background(), payload.TenantID)
	service, _, metadata, calls := finalizationDependencies()
	key := constant.EntityTransaction + ":" + feeDebtTransaction
	metadata.data[key] = &mongodb.Metadata{EntityID: feeDebtTransaction, EntityName: constant.EntityTransaction, Data: mongodb.JSON{"purpose": "fee debt", "packageAppliedID": "package-1"}}

	require.NoError(t, completionError(service.Complete(ctx, &envelope)))
	require.NoError(t, completionError(service.Complete(ctx, &envelope)))

	assert.Equal(t, feeDebtCommittedMetadata(), metadata.data[key].Data, "the commit adds its keys and keeps every stored key")
	updates := 0
	for _, call := range *calls {
		if call == "update:"+constant.EntityTransaction {
			updates++
		}
	}
	assert.Equal(t, 1, updates, "a replay finds the keys already merged")
}

// TestFeeDebtCommitReplayAndRevertConfirmThePendingMetadata completes the commit of a pending
// twice and then a revert of that commit; each re-completes its dependencies first, so the
// pending must still confirm against the document its commit extended.
func TestFeeDebtCommitReplayAndRevertConfirmThePendingMetadata(t *testing.T) {
	commit, revert, resolver := feeDebtLifecycleFixture(t)
	service, _, metadata, _ := finalizationDependencies()
	ctx := tmcore.ContextWithTenantID(context.Background(), commit.Record.TenantID)

	for _, envelope := range []*TransactionWriteBehindEnvelope{&commit, &commit, &revert} {
		_, err := CompleteTransactionWriteBehind(ctx, envelope, resolver, service)
		require.NoError(t, err)
	}

	assert.Equal(t, feeDebtCommittedMetadata(), metadata.data[constant.EntityTransaction+":"+feeDebtTransaction].Data)
}

// feeDebtLifecycleFixture is the commit of a pending that settles and opens debt, carrying the
// pending as its predecessor, and a revert carrying that commit as its origin.
func feeDebtLifecycleFixture(t testing.TB) (commit, revert TransactionWriteBehindEnvelope, resolver *transactionEvidenceResolverStub) {
	t.Helper()
	tx := uuid.MustParse(feeDebtTransaction)
	pendingPayload, pendingResult := recoveryContractFixture(t)
	pendingPayload.TransactionID, pendingPayload.ExecutionID = tx, uuid.MustParse("15151515-1515-4151-8151-151515151515")
	pendingPayload.Action, pendingPayload.TransactionStatus = constant.ActionHold, constant.PENDING
	pendingPayload.TransactionInput.Metadata = map[string]any{"purpose": "fee debt", "packageAppliedID": "package-1"}
	pendingPayload.OperationSpecs[0].TransactionID, pendingResult.Movements[0].TransactionID = tx, tx
	pending := feeDebtWriteBehind(t, pendingPayload, pendingResult)

	commitPayload, commitResult := deferredFeeFixture(t)
	commitPayload.Action = constant.ActionCommit
	commit = feeDebtWriteBehind(t, commitPayload, commitResult, transactionEvidenceReference(TransactionDependencyPredecessor, pending))

	revertPayload, revertResult := recoveryContractFixture(t)
	revertPayload.ExecutionID = uuid.MustParse("16161616-1616-4161-8161-161616161616")
	revertPayload.Action, revertPayload.ParentTransactionID = constant.ActionRevert, &tx
	revert = feeDebtWriteBehind(t, revertPayload, revertResult, transactionEvidenceReference(TransactionDependencyOrigin, commit))

	resolver = &transactionEvidenceResolverStub{records: map[string]*TransactionWriteBehindEnvelope{
		transactionCompletionEvidenceIdentity(tx, pending.Record.ExecutionID): &pending,
		transactionCompletionEvidenceIdentity(tx, commit.Record.ExecutionID):  &commit,
	}}

	return commit, revert, resolver
}

// feeDebtCommittedMetadata is the transaction document once its commit settled and opened debt.
func feeDebtCommittedMetadata() mongodb.JSON {
	return mongodb.JSON{
		"purpose": "fee debt", "packageAppliedID": "package-1",
		constant.MetadataKeyFeeDebtOpenings: feeDebtOpeningsGolden, constant.MetadataKeyFeeDebtSettlements: feeDebtSettledGolden,
	}
}

func feeDebtWriteBehind(
	t testing.TB, payload TransactionCompletionPlan, result accounting.ExecutionResult, dependencies ...TransactionEvidenceReference,
) TransactionWriteBehindEnvelope {
	t.Helper()
	fingerprint, err := ComputeEngineIntentFingerprint(recoveryContractIntent(payload))
	require.NoError(t, err)
	payload.IntentFingerprint = fingerprint

	return TransactionWriteBehindEnvelope{
		FormatVersion: TransactionWriteBehindFormatVersion, ApplicationState: TransactionApplicationConfirmed,
		ReplayState: TransactionReplayReconstructible, DurabilityState: TransactionDurabilityPending,
		Record: recoveryContractEnvelope(t, payload, result), Dependencies: append([]TransactionEvidenceReference{}, dependencies...),
	}
}

func TestPartitionEngineResultRoutesFeeDebtChangesPerTransaction(t *testing.T) {
	prepared, result := repeatedBalancePartitionFixture(t, 2)
	first, second := prepared.Execution.Execution.Transactions[0].ID, prepared.Execution.Execution.Transactions[1].ID
	settled := func(tx uuid.UUID, seq int64) accounting.FeeDebtChange {
		return feeDebtChange(tx, "source:0", accounting.FeeDebtSettled, feeDebtSettledFirst, "@source#default", seq, 1, 1)
	}
	result.FeeDebt = []accounting.FeeDebtChange{settled(second, 1), settled(first, 2), settled(second, 3)}

	partitions, err := PartitionEngineResult(prepared, result)
	require.NoError(t, err)
	assert.Equal(t, []accounting.FeeDebtChange{settled(first, 2)}, partitions[0].FeeDebt)
	assert.Equal(t, []accounting.FeeDebtChange{settled(second, 1), settled(second, 3)}, partitions[1].FeeDebt, "changes keep execution order")

	result.FeeDebt = append(result.FeeDebt, settled(uuid.MustParse(feeDebtTransaction), 4))
	_, err = PartitionEngineResult(prepared, result)
	require.ErrorIs(t, err, ErrInvalidTransactionCompletionRecord)
}

func TestValidateCompletionPostingsAnchorsCollectAndRefundOnTheirDebtor(t *testing.T) {
	tx := uuid.MustParse(feeDebtTransaction)
	payer, fees := feeDebtRowBalance("@payer", 1), feeDebtRowBalance("@fees", 3)
	transaction := accounting.Transaction{Postings: []accounting.Posting{
		{Ref: "to:0", BalanceRef: "@payer#default", Type: accounting.PostingCredit},
		{Ref: feeDebtCollectPosting, BalanceRef: "@payer#default", Type: accounting.PostingCollect},
		{Ref: "fee-refund:0", BalanceRef: "@payer#default", Type: accounting.PostingRefund},
	}}
	primary := feeDebtSpec(tx, "to:0", accounting.RolePrimary, 0, payer, constant.CREDIT, constant.DirectionCredit, 10, nil)
	collect := feeDebtSpec(tx, feeDebtCollectPosting, accounting.RoleFeeDebtDebit, 0, payer, constant.FEE_SETTLEMENT, constant.DirectionDebit, 10, nil)
	credit := feeDebtSpec(tx, feeDebtCollectPosting, accounting.RoleFeeDebtCredit, 0, fees, constant.FEE_SETTLEMENT, constant.DirectionCredit, 10, nil)
	refund := feeDebtSpec(tx, "fee-refund:0", accounting.RoleFeeDebtRefundCredit, 0, payer, constant.FEE_REFUND, constant.DirectionCredit, 10, nil)

	require.NoError(t, validateCompletionPostings(transaction, []OperationRecordSpec{primary, collect, credit, refund}))

	misplaced := collect
	misplaced.BalanceRef, misplaced.Balance = "@fees#default", fees
	collectAsPrimary := collect
	collectAsPrimary.Role = accounting.RolePrimary

	for name, specs := range map[string][]OperationRecordSpec{
		"collect without its debit":        {primary, credit, refund},
		"collect debit on another balance": {primary, misplaced, credit, refund},
		"collect anchored as a primary":    {primary, collectAsPrimary, refund},
		"refund without its credit":        {primary, collect, credit},
	} {
		t.Run(name, func(t *testing.T) {
			require.ErrorIs(t, validateCompletionPostings(transaction, specs), ErrInvalidTransactionCompletionRecord)
		})
	}
}
