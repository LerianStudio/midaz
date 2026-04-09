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
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	pkgTransaction "github.com/LerianStudio/midaz/v3/pkg/transaction"
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

	ft := pkgTransaction.FromTo{
		AccountAlias: "@sender",
		IsFrom:       true,
	}

	amt := pkgTransaction.Amount{
		Asset:     "BRL",
		Value:     decimal.NewFromInt(100),
		Operation: "DEBIT",
		Direction: "debit",
	}

	bat := pkgTransaction.Balance{
		Available: decimal.NewFromInt(900),
		OnHold:    decimal.Zero,
		Version:   2,
	}

	tran := transaction.Transaction{
		ID: "txn-1",
	}

	transactionInput := pkgTransaction.Transaction{
		Description: "test txn",
		Send:        pkgTransaction.Send{Asset: "BRL"},
	}

	op, err := handler.buildStandardOp(
		blc, ft, amt, bat, tran, transactionInput, now, false,
	)

	require.NoError(t, err)
	require.NotNil(t, op)
	assert.Equal(t, "txn-1", op.TransactionID)
	assert.Equal(t, "balance-1", op.BalanceID)
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

	ft := pkgTransaction.FromTo{
		AccountAlias: "@sender",
		IsFrom:       true,
	}

	amt := pkgTransaction.Amount{
		Asset:     "BRL",
		Value:     decimal.NewFromInt(100),
		Operation: "DEBIT",
		Direction: "debit",
	}

	bat := pkgTransaction.Balance{}

	tran := transaction.Transaction{
		ID: "txn-1",
	}

	transactionInput := pkgTransaction.Transaction{
		Description: "test txn",
		Send:        pkgTransaction.Send{Asset: "BRL"},
	}

	ops, err := handler.buildDoubleEntryPendingOps(
		ctx, blc, ft, amt, bat, tran, transactionInput, now, false,
	)

	require.NoError(t, err)
	require.Len(t, ops, 2, "should return exactly 2 operations")
	assert.Equal(t, "txn-1", ops[0].TransactionID)
	assert.Equal(t, "txn-1", ops[1].TransactionID)
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

	ft := pkgTransaction.FromTo{
		AccountAlias: "@sender",
		IsFrom:       true,
	}

	amt := pkgTransaction.Amount{
		Asset:     "BRL",
		Value:     decimal.NewFromInt(100),
		Operation: "RELEASE",
		Direction: "debit",
	}

	bat := pkgTransaction.Balance{}

	tran := transaction.Transaction{
		ID: "txn-1",
	}

	transactionInput := pkgTransaction.Transaction{
		Description: "test txn",
		Send:        pkgTransaction.Send{Asset: "BRL"},
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
	case "direct":
		rc.AccountingEntries.Direct = entry
	case "hold":
		rc.AccountingEntries.Hold = entry
	case "commit":
		rc.AccountingEntries.Commit = entry
	case "cancel":
		rc.AccountingEntries.Cancel = entry
	case "revert":
		rc.AccountingEntries.Revert = entry
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

// TestStatusToAction verifies the mapping from transaction status to accounting action.
func TestStatusToAction(t *testing.T) {
	assert.Equal(t, "direct", pkgTransaction.StatusToAction("CREATED"))
	assert.Equal(t, "hold", pkgTransaction.StatusToAction("PENDING"))
	assert.Equal(t, "commit", pkgTransaction.StatusToAction("APPROVED"))
	assert.Equal(t, "cancel", pkgTransaction.StatusToAction("CANCELED"))
	assert.Equal(t, "direct", pkgTransaction.StatusToAction("NOTED"))
	assert.Equal(t, "direct", pkgTransaction.StatusToAction(""))
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
