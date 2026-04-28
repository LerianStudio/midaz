// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"context"
	"testing"
	"time"

	"github.com/LerianStudio/midaz/v3/components/ledger/internal/adapters/postgres/operation"
	"github.com/LerianStudio/midaz/v3/components/ledger/internal/adapters/postgres/transaction"
	"github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/LerianStudio/midaz/v3/pkg/mtransaction"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestBuildStandardOp_ReturnsOperation verifies that buildStandardOp returns a valid operation.
func TestBuildStandardOp_ReturnsOperation(t *testing.T) {
	handler := &TransactionHandler{}
	now := time.Now()

	blc := &mmodel.Balance{
		ID:             "balance-1",
		OrganizationID: "org-1",
		LedgerID:       "ledger-1",
		AccountID:      "account-1",
		Alias:          "@sender",
		Key:            "default",
		Available:      decimal.NewFromInt(1000),
		OnHold:         decimal.Zero,
		Version:        1,
	}

	ft := mtransaction.FromTo{
		AccountAlias: "@sender",
		IsFrom:       true,
	}

	amt := mtransaction.Amount{
		Asset:     "BRL",
		Value:     decimal.NewFromInt(100),
		Operation: "DEBIT",
		Direction: "debit",
	}

	bat := mtransaction.Balance{
		Available: decimal.NewFromInt(900),
		OnHold:    decimal.Zero,
		Version:   2,
	}

	tran := transaction.Transaction{
		ID: "txn-1",
	}

	transactionInput := mtransaction.Transaction{
		Description: "test txn",
		Send:        mtransaction.Send{Asset: "BRL"},
	}

	op, err := handler.buildStandardOp(
		blc, ft, amt, bat, tran, transactionInput, now, false,
	)

	require.NoError(t, err)
	require.NotNil(t, op)
	assert.Equal(t, "txn-1", op.TransactionID)
	assert.Equal(t, "balance-1", op.BalanceID)
}

func TestBuildStandardOp_OverdraftCompanionUsesOverdraftType(t *testing.T) {
	handler := &TransactionHandler{}
	now := time.Now()

	blc := &mmodel.Balance{
		ID:             "balance-overdraft",
		OrganizationID: "org-1",
		LedgerID:       "ledger-1",
		AccountID:      "account-1",
		Alias:          "@sender",
		Key:            constant.OverdraftBalanceKey,
		Available:      decimal.Zero,
		OnHold:         decimal.Zero,
		Version:        1,
	}
	amt := mtransaction.Amount{
		Asset:     "BRL",
		Value:     decimal.NewFromInt(50),
		Operation: constant.DEBIT,
		Direction: constant.DirectionDebit,
	}

	op, err := handler.buildStandardOp(
		blc,
		mtransaction.FromTo{AccountAlias: "@sender", BalanceKey: constant.OverdraftBalanceKey},
		amt,
		mtransaction.Balance{Available: decimal.NewFromInt(50), OnHold: decimal.Zero, Version: 2},
		transaction.Transaction{ID: "txn-1"},
		mtransaction.Transaction{Description: "overdraft companion", Send: mtransaction.Send{Asset: "BRL"}},
		now,
		false,
	)

	require.NoError(t, err)
	assert.Equal(t, constant.OVERDRAFT, op.Type)
	assert.Equal(t, constant.DirectionDebit, op.Direction,
		"direction still carries draw/repayment semantics for overdraft rows")
}

// TestBuildDoubleEntryPendingOps_ReturnsTwoOperations verifies that buildDoubleEntryPendingOps
// returns exactly 2 operations.
func TestBuildDoubleEntryPendingOps_ReturnsTwoOperations(t *testing.T) {
	handler := &TransactionHandler{}
	ctx := context.Background()
	now := time.Now()

	blc := &mmodel.Balance{
		ID:             "balance-1",
		OrganizationID: "org-1",
		LedgerID:       "ledger-1",
		AccountID:      "account-1",
		Alias:          "@sender",
		Key:            "default",
		Available:      decimal.NewFromInt(1000),
		OnHold:         decimal.Zero,
		Version:        1,
	}

	ft := mtransaction.FromTo{
		AccountAlias: "@sender",
		IsFrom:       true,
	}

	amt := mtransaction.Amount{
		Asset:     "BRL",
		Value:     decimal.NewFromInt(100),
		Operation: "DEBIT",
		Direction: "debit",
	}

	bat := mtransaction.Balance{}

	tran := transaction.Transaction{
		ID: "txn-1",
	}

	transactionInput := mtransaction.Transaction{
		Description: "test txn",
		Send:        mtransaction.Send{Asset: "BRL"},
	}

	ops, err := handler.buildDoubleEntryPendingOps(
		ctx, blc, ft, amt, bat, tran, transactionInput, now, false,
	)

	require.NoError(t, err)
	require.Len(t, ops, 2, "should return exactly 2 operations")
	assert.Equal(t, "txn-1", ops[0].TransactionID)
	assert.Equal(t, "txn-1", ops[1].TransactionID)
}

func TestBuildOperations_DoubleEntryPendingOverdraftUsesLuaAfterState(t *testing.T) {
	handler := &TransactionHandler{}
	ctx := context.Background()
	now := time.Now()

	before := &mmodel.Balance{
		ID:             "balance-default",
		OrganizationID: "org-1",
		LedgerID:       "ledger-1",
		AccountID:      "account-1",
		Alias:          "0#@sender#default",
		Key:            constant.DefaultBalanceKey,
		Available:      decimal.NewFromInt(50),
		OnHold:         decimal.Zero,
		Version:        1,
		Direction:      constant.DirectionCredit,
		OverdraftUsed:  decimal.Zero,
	}
	after := &mmodel.Balance{
		ID:             before.ID,
		OrganizationID: before.OrganizationID,
		LedgerID:       before.LedgerID,
		AccountID:      before.AccountID,
		Alias:          before.Alias,
		Key:            before.Key,
		Available:      decimal.Zero,
		OnHold:         decimal.NewFromInt(100),
		Version:        3,
		Direction:      before.Direction,
		OverdraftUsed:  decimal.NewFromInt(50),
	}

	amount := mtransaction.Amount{
		Asset:                  "BRL",
		Value:                  decimal.NewFromInt(100),
		Operation:              constant.ONHOLD,
		TransactionType:        constant.PENDING,
		RouteValidationEnabled: true,
	}
	fromTo := []mtransaction.FromTo{{
		AccountAlias: before.Alias,
		BalanceKey:   constant.DefaultBalanceKey,
		Amount:       &amount,
		IsFrom:       true,
	}}
	validate := &mtransaction.Responses{
		From: map[string]mtransaction.Amount{
			before.Alias: amount,
		},
		Sources: []string{"@sender#default"},
		Aliases: []string{"@sender#default"},
	}
	tran := transaction.Transaction{
		ID:             "txn-1",
		OrganizationID: before.OrganizationID,
		LedgerID:       before.LedgerID,
	}
	input := mtransaction.Transaction{
		Pending:     true,
		Description: "pending overdraft",
		Send:        mtransaction.Send{Asset: "BRL"},
	}

	ops, _, err := handler.BuildOperations(ctx, []*mmodel.Balance{before}, []*mmodel.Balance{after}, fromTo,
		input, tran, validate, now, false, true, nil, constant.ActionHold)
	require.NoError(t, err)
	require.Len(t, ops, 2)

	debit := ops[0]
	require.Equal(t, constant.DEBIT, debit.Type)
	assert.True(t, debit.Amount.Value.Equal(decimal.NewFromInt(50)),
		"DEBIT amount must be clipped to the Lua-authoritative available movement")
	assert.True(t, debit.BalanceAfter.Available.Equal(decimal.Zero),
		"DEBIT after available must be floored at zero, not locally recomputed as -50")
	assert.True(t, debit.BalanceAfter.OverdraftUsed.Equal(decimal.NewFromInt(50)))
	assert.Equal(t, "0", debit.Snapshot.OverdraftUsedBefore)
	assert.Equal(t, "50", debit.Snapshot.OverdraftUsedAfter)

	onHold := ops[1]
	require.Equal(t, constant.ONHOLD, onHold.Type)
	assert.True(t, onHold.BalanceAfter.Available.Equal(decimal.Zero))
	assert.True(t, onHold.BalanceAfter.OnHold.Equal(decimal.NewFromInt(100)))
	assert.True(t, onHold.BalanceAfter.OverdraftUsed.Equal(decimal.NewFromInt(50)))
}

// TestBuildDoubleEntryCanceledOps_ReturnsTwoOperations verifies that buildDoubleEntryCanceledOps
// returns exactly 2 operations.
func TestBuildDoubleEntryCanceledOps_ReturnsTwoOperations(t *testing.T) {
	handler := &TransactionHandler{}
	ctx := context.Background()
	now := time.Now()

	blc := &mmodel.Balance{
		ID:             "balance-1",
		OrganizationID: "org-1",
		LedgerID:       "ledger-1",
		AccountID:      "account-1",
		Alias:          "@sender",
		Key:            "default",
		Available:      decimal.NewFromInt(1000),
		OnHold:         decimal.NewFromInt(500),
		Version:        1,
	}

	ft := mtransaction.FromTo{
		AccountAlias: "@sender",
		IsFrom:       true,
	}

	amt := mtransaction.Amount{
		Asset:     "BRL",
		Value:     decimal.NewFromInt(100),
		Operation: "RELEASE",
		Direction: "debit",
	}

	bat := mtransaction.Balance{}

	tran := transaction.Transaction{
		ID: "txn-1",
	}

	transactionInput := mtransaction.Transaction{
		Description: "test txn",
		Send:        mtransaction.Send{Asset: "BRL"},
	}

	ops, err := handler.buildDoubleEntryCanceledOps(
		ctx, blc, ft, amt, bat, tran, transactionInput, now, false,
	)

	require.NoError(t, err)
	require.Len(t, ops, 2, "should return exactly 2 operations")
	assert.Equal(t, "txn-1", ops[0].TransactionID)
	assert.Equal(t, "txn-1", ops[1].TransactionID)
}

// helper to build a cache with accounting entries for the given action.
func buildCacheWithEntries(action, routeID, routeType, description, debitCode, creditCode string) *mmodel.TransactionRouteCache {
	rc := mmodel.OperationRouteCache{
		Description:       description,
		AccountingEntries: &mmodel.AccountingEntries{},
	}

	entry := &mmodel.AccountingEntry{
		Debit:  &mmodel.AccountingRubric{Code: debitCode, Description: "Debit desc"},
		Credit: &mmodel.AccountingRubric{Code: creditCode, Description: "Credit desc"},
	}

	switch action {
	case constant.ActionDirect:
		rc.AccountingEntries.Direct = entry
	case constant.ActionHold:
		rc.AccountingEntries.Hold = entry
	case constant.ActionCommit:
		rc.AccountingEntries.Commit = entry
	case constant.ActionCancel:
		rc.AccountingEntries.Cancel = entry
	case constant.ActionRevert:
		rc.AccountingEntries.Revert = entry
	case constant.ActionOverdraft:
		rc.AccountingEntries.Overdraft = entry
	}

	actionCache := mmodel.ActionRouteCache{
		Source:        map[string]mmodel.OperationRouteCache{},
		Destination:   map[string]mmodel.OperationRouteCache{},
		Bidirectional: map[string]mmodel.OperationRouteCache{},
	}

	switch routeType {
	case "source":
		actionCache.Source[routeID] = rc
	case "destination":
		actionCache.Destination[routeID] = rc
	case "bidirectional":
		actionCache.Bidirectional[routeID] = rc
	}

	return &mmodel.TransactionRouteCache{
		Actions: map[string]mmodel.ActionRouteCache{
			action: actionCache,
		},
	}
}

// TestResolveRouteCodesFromCache_NilCache verifies that a nil cache is handled gracefully.
func TestResolveRouteCodesFromCache_NilCache(t *testing.T) {
	routeID := "route-uuid-1"
	ops := []*operation.Operation{
		{ID: "op-1", RouteID: &routeID},
	}

	resolveRouteCodesFromCache(ops, nil, "direct")

	assert.Nil(t, ops[0].RouteCode, "RouteCode should remain nil when cache is nil")
}

// TestResolveRouteCodesFromCache_NoRouteID verifies that operations without a RouteID are skipped.
func TestResolveRouteCodesFromCache_NoRouteID(t *testing.T) {
	cache := buildCacheWithEntries("direct", "route-uuid-1", "source", "Route desc", "1001", "2001")

	ops := []*operation.Operation{
		{ID: "op-1", RouteID: nil},
	}

	resolveRouteCodesFromCache(ops, cache, "direct")

	assert.Nil(t, ops[0].RouteCode, "RouteCode should remain nil when RouteID is nil")
}

// TestResolveRouteCodesFromCache_SourceRoute verifies that RouteCode is resolved from
// the accounting entry's debit rubric code for a debit source operation.
func TestResolveRouteCodesFromCache_SourceRoute(t *testing.T) {
	routeID := "route-uuid-1"
	cache := buildCacheWithEntries("direct", routeID, "source", "Route description", "1001", "2001")

	ops := []*operation.Operation{
		{ID: "op-1", RouteID: &routeID, Direction: "debit"},
	}

	resolveRouteCodesFromCache(ops, cache, "direct")

	require.NotNil(t, ops[0].RouteCode, "RouteCode should be populated from accounting entry debit rubric code")
	assert.Equal(t, "1001", *ops[0].RouteCode)
	require.NotNil(t, ops[0].RouteDescription, "RouteDescription should be populated from accounting rubric description")
	assert.Equal(t, "Debit desc", *ops[0].RouteDescription)
}

// TestResolveRouteCodesFromCache_DestinationRoute verifies resolution from destination routes
// using the credit rubric code.
func TestResolveRouteCodesFromCache_DestinationRoute(t *testing.T) {
	routeID := "route-uuid-2"
	cache := buildCacheWithEntries("direct", routeID, "destination", "Dest route", "1001", "2001")

	ops := []*operation.Operation{
		{ID: "op-1", RouteID: &routeID, Direction: "credit"},
	}

	resolveRouteCodesFromCache(ops, cache, "direct")

	require.NotNil(t, ops[0].RouteCode, "RouteCode should be populated from accounting entry credit rubric code")
	assert.Equal(t, "2001", *ops[0].RouteCode)
}

// TestResolveRouteCodesFromCache_BidirectionalRoute verifies resolution from bidirectional routes.
func TestResolveRouteCodesFromCache_BidirectionalRoute(t *testing.T) {
	routeID := "route-uuid-3"
	cache := buildCacheWithEntries("hold", routeID, "bidirectional", "Bidir route", "3001", "4001")

	ops := []*operation.Operation{
		{ID: "op-1", RouteID: &routeID, Direction: "debit"},
	}

	resolveRouteCodesFromCache(ops, cache, "hold")

	require.NotNil(t, ops[0].RouteCode, "RouteCode should be populated from bidirectional route's accounting entry")
	assert.Equal(t, "3001", *ops[0].RouteCode)
}

// TestResolveRouteCodesFromCache_MultipleOperations verifies that multiple operations
// are resolved independently from different route types with correct rubric codes.
func TestResolveRouteCodesFromCache_MultipleOperations(t *testing.T) {
	srcRouteID := "route-src"
	dstRouteID := "route-dst"
	unknownRouteID := "route-unknown"

	cache := &mmodel.TransactionRouteCache{
		Actions: map[string]mmodel.ActionRouteCache{
			"direct": {
				Source: map[string]mmodel.OperationRouteCache{
					"route-src": {
						Description: "Source route",
						AccountingEntries: &mmodel.AccountingEntries{
							Direct: &mmodel.AccountingEntry{
								Debit:  &mmodel.AccountingRubric{Code: "SRC-DEBIT", Description: "Source debit"},
								Credit: &mmodel.AccountingRubric{Code: "SRC-CREDIT", Description: "Source credit"},
							},
						},
					},
				},
				Destination: map[string]mmodel.OperationRouteCache{
					"route-dst": {
						Description: "Dest route",
						AccountingEntries: &mmodel.AccountingEntries{
							Direct: &mmodel.AccountingEntry{
								Debit:  &mmodel.AccountingRubric{Code: "DST-DEBIT", Description: "Dest debit"},
								Credit: &mmodel.AccountingRubric{Code: "DST-CREDIT", Description: "Dest credit"},
							},
						},
					},
				},
				Bidirectional: map[string]mmodel.OperationRouteCache{},
			},
		},
	}

	ops := []*operation.Operation{
		{ID: "op-1", RouteID: &srcRouteID, Direction: "debit"},
		{ID: "op-2", RouteID: &dstRouteID, Direction: "credit"},
		{ID: "op-3", RouteID: &unknownRouteID, Direction: "debit"},
		{ID: "op-4", RouteID: nil},
	}

	resolveRouteCodesFromCache(ops, cache, "direct")

	require.NotNil(t, ops[0].RouteCode)
	assert.Equal(t, "SRC-DEBIT", *ops[0].RouteCode)

	require.NotNil(t, ops[1].RouteCode)
	assert.Equal(t, "DST-CREDIT", *ops[1].RouteCode)

	assert.Nil(t, ops[2].RouteCode, "unknown route ID should leave RouteCode nil")
	assert.Nil(t, ops[3].RouteCode, "nil route ID should leave RouteCode nil")
}

// TestResolveRouteCodesFromCache_EmptyRouteID verifies that an empty string RouteID is skipped.
func TestResolveRouteCodesFromCache_EmptyRouteID(t *testing.T) {
	emptyRouteID := ""
	cache := buildCacheWithEntries("direct", "", "source", "Route desc", "1001", "2001")

	ops := []*operation.Operation{
		{ID: "op-1", RouteID: &emptyRouteID},
	}

	resolveRouteCodesFromCache(ops, cache, "direct")

	assert.Nil(t, ops[0].RouteCode, "RouteCode should remain nil for empty RouteID")
}

// TestResolveRouteCodesFromCache_NoAccountingEntries verifies that when the cache has no
// AccountingEntries, both RouteCode and RouteDescription remain nil since description
// now comes from the accounting rubric rather than the route-level description.
func TestResolveRouteCodesFromCache_NoAccountingEntries(t *testing.T) {
	routeID := "route-uuid-1"
	cache := &mmodel.TransactionRouteCache{
		Actions: map[string]mmodel.ActionRouteCache{
			"direct": {
				Source: map[string]mmodel.OperationRouteCache{
					"route-uuid-1": {
						Description:       "Route without entries",
						AccountingEntries: nil,
					},
				},
				Destination:   map[string]mmodel.OperationRouteCache{},
				Bidirectional: map[string]mmodel.OperationRouteCache{},
			},
		},
	}

	ops := []*operation.Operation{
		{ID: "op-1", RouteID: &routeID, Direction: "debit"},
	}

	resolveRouteCodesFromCache(ops, cache, "direct")

	assert.Nil(t, ops[0].RouteCode, "RouteCode should remain nil when no AccountingEntries exist")
	assert.Nil(t, ops[0].RouteDescription, "RouteDescription should remain nil when no accounting rubric is resolved")
}

// TestResolveRouteCodesFromCache_HoldAction verifies resolution for the hold action.
func TestResolveRouteCodesFromCache_HoldAction(t *testing.T) {
	routeID := "route-uuid-1"
	cache := buildCacheWithEntries("hold", routeID, "source", "Hold route", "HOLD-DEBIT", "HOLD-CREDIT")

	ops := []*operation.Operation{
		{ID: "op-1", RouteID: &routeID, Direction: "credit"},
	}

	resolveRouteCodesFromCache(ops, cache, "hold")

	require.NotNil(t, ops[0].RouteCode, "RouteCode should be populated for hold action")
	assert.Equal(t, "HOLD-CREDIT", *ops[0].RouteCode)
}

// TestResolveRouteCodesFromCache_CommitAction verifies resolution for the commit action.
func TestResolveRouteCodesFromCache_CommitAction(t *testing.T) {
	routeID := "route-uuid-1"
	cache := buildCacheWithEntries("commit", routeID, "destination", "Commit route", "COMMIT-DEBIT", "COMMIT-CREDIT")

	ops := []*operation.Operation{
		{ID: "op-1", RouteID: &routeID, Direction: "debit"},
	}

	resolveRouteCodesFromCache(ops, cache, "commit")

	require.NotNil(t, ops[0].RouteCode, "RouteCode should be populated for commit action")
	assert.Equal(t, "COMMIT-DEBIT", *ops[0].RouteCode)
}

// TestResolveRouteCodesFromCache_OverdraftAction verifies resolution for the
// overdraft accounting-entry action. Overdraft is a supplementary scenario
// that must resolve its Debit/Credit rubrics through the same cache lookup
// path as the legacy actions (direct/hold/commit/cancel/revert).
func TestResolveRouteCodesFromCache_OverdraftAction(t *testing.T) {
	routeID := "route-uuid-1"
	cache := buildCacheWithEntries(constant.ActionOverdraft, routeID, "bidirectional", "Overdraft route", "OVERDRAFT-DEBIT", "OVERDRAFT-CREDIT")

	ops := []*operation.Operation{
		{ID: "op-1", RouteID: &routeID, Direction: "debit"},
	}

	resolveRouteCodesFromCache(ops, cache, constant.ActionOverdraft)

	require.NotNil(t, ops[0].RouteCode, "RouteCode should be populated for overdraft action")
	assert.Equal(t, "OVERDRAFT-DEBIT", *ops[0].RouteCode)
	require.NotNil(t, ops[0].RouteDescription, "RouteDescription should be populated for overdraft action")
	assert.Equal(t, "Debit desc", *ops[0].RouteDescription)
}

// TestResolveRouteCodesFromCache_OverdraftCreditAction verifies that a CREDIT
// operation on the overdraft balance resolves to Overdraft.Credit (not a
// separate "refund" action). After T-014 collapsed the refund slot into the
// overdraft slot, both DEBIT and CREDIT on the overdraft balance resolve
// through the single ActionOverdraft entry.
func TestResolveRouteCodesFromCache_OverdraftCreditAction(t *testing.T) {
	routeID := "route-uuid-1"
	cache := buildCacheWithEntries(constant.ActionOverdraft, routeID, "bidirectional", "Overdraft route", "OVERDRAFT-DEBIT", "OVERDRAFT-CREDIT")

	ops := []*operation.Operation{
		{ID: "op-1", RouteID: &routeID, Direction: "credit"},
	}

	resolveRouteCodesFromCache(ops, cache, constant.ActionOverdraft)

	require.NotNil(t, ops[0].RouteCode, "RouteCode should be populated for overdraft credit action")
	assert.Equal(t, "OVERDRAFT-CREDIT", *ops[0].RouteCode)
	require.NotNil(t, ops[0].RouteDescription, "RouteDescription should be populated for overdraft credit action")
	assert.Equal(t, "Credit desc", *ops[0].RouteDescription)
}

// TestResolveRouteCodesFromCache_CompanionUsesOverdraftAction pins the
// behaviour required for overdraft enrichment: when operations include
// companion entries on the `#overdraft` balance, those entries MUST resolve
// their RouteCode/RouteDescription through the OVERDRAFT accounting action
// even though the transaction-wide action is still `direct` (or
// `hold`/`commit`). The same RouteID is reused; only the action differs —
// exactly mirroring how hold/commit reuse the direct routeId but resolve
// different rubrics. Both DEBIT (deficit grows) and CREDIT (repayment)
// companions resolve via the single Overdraft entry.
//
// Failure mode guarded against: without the second pass, the companion
// op stays with RouteCode=nil because the `direct` rubric is the Direct
// AccountingEntry, not the Overdraft one.
func TestResolveRouteCodesFromCache_CompanionUsesOverdraftAction(t *testing.T) {
	routeID := "route-uuid-1"

	// Build a cache that contains entries for BOTH "direct" and "overdraft"
	// actions, as the real ToCache() emits whenever a route has both Direct
	// and Overdraft AccountingEntries.
	rc := mmodel.OperationRouteCache{
		Description: "Bidirectional route",
		AccountingEntries: &mmodel.AccountingEntries{
			Direct: &mmodel.AccountingEntry{
				Debit:  &mmodel.AccountingRubric{Code: "DIRECT-DEBIT", Description: "Direct debit desc"},
				Credit: &mmodel.AccountingRubric{Code: "DIRECT-CREDIT", Description: "Direct credit desc"},
			},
			Overdraft: &mmodel.AccountingEntry{
				Debit:  &mmodel.AccountingRubric{Code: "OVERDRAFT-DEBIT", Description: "Overdraft debit desc"},
				Credit: &mmodel.AccountingRubric{Code: "OVERDRAFT-CREDIT", Description: "Overdraft credit desc"},
			},
		},
	}

	directAction := mmodel.ActionRouteCache{
		Source:        map[string]mmodel.OperationRouteCache{},
		Destination:   map[string]mmodel.OperationRouteCache{},
		Bidirectional: map[string]mmodel.OperationRouteCache{routeID: rc},
	}
	overdraftAction := mmodel.ActionRouteCache{
		Source:        map[string]mmodel.OperationRouteCache{},
		Destination:   map[string]mmodel.OperationRouteCache{},
		Bidirectional: map[string]mmodel.OperationRouteCache{routeID: rc},
	}

	cache := &mmodel.TransactionRouteCache{
		Actions: map[string]mmodel.ActionRouteCache{
			constant.ActionDirect:    directAction,
			constant.ActionOverdraft: overdraftAction,
		},
	}

	primary := &operation.Operation{
		ID:         "primary-op",
		RouteID:    &routeID,
		Type:       "DEBIT",
		Direction:  "debit",
		BalanceKey: constant.DefaultBalanceKey,
	}
	companion := &operation.Operation{
		ID:         "companion-op",
		RouteID:    &routeID,
		Type:       constant.OVERDRAFT,
		Direction:  "debit",
		BalanceKey: constant.OverdraftBalanceKey,
	}

	ops := []*operation.Operation{primary, companion}

	resolveRouteCodesFromCache(ops, cache, constant.ActionDirect)

	// Primary resolves to DIRECT rubric.
	require.NotNil(t, primary.RouteCode, "primary op must resolve via direct action")
	assert.Equal(t, "DIRECT-DEBIT", *primary.RouteCode,
		"primary (BalanceKey=default) must resolve from Direct AccountingEntry")

	// Companion resolves to OVERDRAFT rubric despite the top-level action
	// being `direct`, because it lives on the overdraft balance and
	// represents the overdraft leg of the enrichment.
	require.NotNil(t, companion.RouteCode, "companion op must resolve via overdraft action")
	assert.Equal(t, "OVERDRAFT-DEBIT", *companion.RouteCode,
		"companion (BalanceKey=overdraft) must resolve from Overdraft AccountingEntry, not Direct")
}

// TestResolveRouteCodesFromCache_CompanionCreditUsesOverdraftAction verifies
// that repayment companions on the overdraft balance resolve their rubric via
// Overdraft.Credit (not Overdraft.Debit). Public type is always "overdraft";
// the Direction field carries debit/draw vs credit/repayment semantics.
func TestResolveRouteCodesFromCache_CompanionCreditUsesOverdraftAction(t *testing.T) {
	routeID := "route-uuid-1"

	rc := mmodel.OperationRouteCache{
		Description: "Bidirectional route",
		AccountingEntries: &mmodel.AccountingEntries{
			Direct: &mmodel.AccountingEntry{
				Debit:  &mmodel.AccountingRubric{Code: "DIRECT-DEBIT", Description: "Direct debit desc"},
				Credit: &mmodel.AccountingRubric{Code: "DIRECT-CREDIT", Description: "Direct credit desc"},
			},
			Overdraft: &mmodel.AccountingEntry{
				Debit:  &mmodel.AccountingRubric{Code: "OVERDRAFT-DEBIT", Description: "Overdraft debit desc"},
				Credit: &mmodel.AccountingRubric{Code: "OVERDRAFT-CREDIT", Description: "Overdraft credit desc"},
			},
		},
	}

	cache := &mmodel.TransactionRouteCache{
		Actions: map[string]mmodel.ActionRouteCache{
			constant.ActionDirect: {
				Source:        map[string]mmodel.OperationRouteCache{},
				Destination:   map[string]mmodel.OperationRouteCache{},
				Bidirectional: map[string]mmodel.OperationRouteCache{routeID: rc},
			},
			constant.ActionOverdraft: {
				Source:        map[string]mmodel.OperationRouteCache{},
				Destination:   map[string]mmodel.OperationRouteCache{},
				Bidirectional: map[string]mmodel.OperationRouteCache{routeID: rc},
			},
		},
	}

	// Direction is "credit" — set by the enrichment engine for repayment
	// companions (buildCompanionCreditOp), reflecting the operation semantics
	// rather than the balance direction. This allows the resolver to pick
	// Overdraft.Credit via the standard op.Direction path.
	companion := &operation.Operation{
		ID:         "companion-op",
		RouteID:    &routeID,
		Type:       constant.OVERDRAFT,
		Direction:  "credit",
		BalanceKey: constant.OverdraftBalanceKey,
	}
	ops := []*operation.Operation{companion}

	resolveRouteCodesFromCache(ops, cache, constant.ActionDirect)

	require.NotNil(t, companion.RouteCode, "companion CREDIT must resolve via overdraft action")
	assert.Equal(t, "OVERDRAFT-CREDIT", *companion.RouteCode,
		"companion CREDIT on overdraft balance must resolve from Overdraft.Credit, not Overdraft.Debit")
}

// TestStatusToAction verifies the mapping from transaction status to accounting action.
func TestStatusToAction(t *testing.T) {
	assert.Equal(t, "direct", mtransaction.StatusToAction("CREATED"))
	assert.Equal(t, "hold", mtransaction.StatusToAction("PENDING"))
	assert.Equal(t, "commit", mtransaction.StatusToAction("APPROVED"))
	assert.Equal(t, "cancel", mtransaction.StatusToAction("CANCELED"))
	assert.Equal(t, "direct", mtransaction.StatusToAction("NOTED"))
	assert.Equal(t, "direct", mtransaction.StatusToAction(""))
}

// TestResolveAccountingRubric verifies the resolveAccountingRubric helper function.
func TestResolveAccountingRubric(t *testing.T) {
	entries := &mmodel.AccountingEntries{
		Direct: &mmodel.AccountingEntry{
			Debit:  &mmodel.AccountingRubric{Code: "D-DIRECT", Description: "Direct debit"},
			Credit: &mmodel.AccountingRubric{Code: "C-DIRECT", Description: "Direct credit"},
		},
		Hold: &mmodel.AccountingEntry{
			Debit:  &mmodel.AccountingRubric{Code: "D-HOLD", Description: "Hold debit"},
			Credit: &mmodel.AccountingRubric{Code: "C-HOLD", Description: "Hold credit"},
		},
		Commit: &mmodel.AccountingEntry{
			Debit:  &mmodel.AccountingRubric{Code: "D-COMMIT", Description: "Commit debit"},
			Credit: &mmodel.AccountingRubric{Code: "C-COMMIT", Description: "Commit credit"},
		},
		Cancel: &mmodel.AccountingEntry{
			Debit:  &mmodel.AccountingRubric{Code: "D-CANCEL", Description: "Cancel debit"},
			Credit: &mmodel.AccountingRubric{Code: "C-CANCEL", Description: "Cancel credit"},
		},
		Revert: &mmodel.AccountingEntry{
			Debit:  &mmodel.AccountingRubric{Code: "D-REVERT", Description: "Revert debit"},
			Credit: &mmodel.AccountingRubric{Code: "C-REVERT", Description: "Revert credit"},
		},
		Overdraft: &mmodel.AccountingEntry{
			Debit:  &mmodel.AccountingRubric{Code: "D-OVERDRAFT", Description: "Overdraft debit"},
			Credit: &mmodel.AccountingRubric{Code: "C-OVERDRAFT", Description: "Overdraft credit"},
		},
	}

	tests := []struct {
		name      string
		action    string
		direction string
		wantCode  string
		wantNil   bool
	}{
		{"direct debit", "direct", "debit", "D-DIRECT", false},
		{"direct credit", "direct", "credit", "C-DIRECT", false},
		{"hold debit", "hold", "debit", "D-HOLD", false},
		{"hold credit", "hold", "credit", "C-HOLD", false},
		{"commit debit", "commit", "debit", "D-COMMIT", false},
		{"commit credit", "commit", "credit", "C-COMMIT", false},
		{"cancel debit", "cancel", "debit", "D-CANCEL", false},
		{"cancel credit", "cancel", "credit", "C-CANCEL", false},
		{"revert debit", "revert", "debit", "D-REVERT", false},
		{"revert credit", "revert", "credit", "C-REVERT", false},
		{"overdraft debit", "overdraft", "debit", "D-OVERDRAFT", false},
		{"overdraft credit", "overdraft", "credit", "C-OVERDRAFT", false},
		{"refund action returns nil (removed)", "refund", "debit", "", true},
		{"unknown action", "unknown", "debit", "", true},
		{"unknown direction", "direct", "unknown", "", true},
		{"nil entries", "direct", "debit", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var e *mmodel.AccountingEntries
			if tt.name != "nil entries" {
				e = entries
			}

			rubric := resolveAccountingRubric(e, tt.action, tt.direction)
			if tt.wantNil {
				assert.Nil(t, rubric)
			} else {
				require.NotNil(t, rubric)
				assert.Equal(t, tt.wantCode, rubric.Code)
			}
		})
	}
}

// TestResolveRouteCodesFromCache_ActionMissingEntry verifies that when the action exists
// in the cache but the AccountingEntries don't have that action's entry, RouteCode stays nil.
func TestResolveRouteCodesFromCache_ActionMissingEntry(t *testing.T) {
	routeID := "route-uuid-1"

	// Cache has route under "direct" action, but AccountingEntries only has Hold
	cache := &mmodel.TransactionRouteCache{
		Actions: map[string]mmodel.ActionRouteCache{
			"direct": {
				Source: map[string]mmodel.OperationRouteCache{
					"route-uuid-1": {
						Description: "Has entries but not for direct",
						AccountingEntries: &mmodel.AccountingEntries{
							Hold: &mmodel.AccountingEntry{
								Debit:  &mmodel.AccountingRubric{Code: "HOLD-D", Description: "Hold debit"},
								Credit: &mmodel.AccountingRubric{Code: "HOLD-C", Description: "Hold credit"},
							},
						},
					},
				},
				Destination:   map[string]mmodel.OperationRouteCache{},
				Bidirectional: map[string]mmodel.OperationRouteCache{},
			},
		},
	}

	ops := []*operation.Operation{
		{ID: "op-1", RouteID: &routeID, Direction: "debit"},
	}

	resolveRouteCodesFromCache(ops, cache, "direct")

	assert.Nil(t, ops[0].RouteCode, "RouteCode should remain nil when action entry is missing from AccountingEntries")
	assert.Nil(t, ops[0].RouteDescription, "RouteDescription should remain nil when no matching accounting rubric is resolved")
}
