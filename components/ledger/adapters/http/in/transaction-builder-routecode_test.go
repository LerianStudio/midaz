// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"context"
	"testing"
	"time"

	"github.com/LerianStudio/midaz/v3/components/ledger/adapters/postgres/operation"
	"github.com/LerianStudio/midaz/v3/components/ledger/adapters/postgres/transaction"
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

// TestResolveRouteCodesFromCache_NilCache verifies that a nil cache is handled gracefully.
func TestResolveRouteCodesFromCache_NilCache(t *testing.T) {
	routeID := "route-uuid-1"
	ops := []*operation.Operation{
		{ID: "op-1", RouteID: &routeID},
	}

	resolveRouteCodesFromCache(ops, nil)

	assert.Nil(t, ops[0].RouteCode, "RouteCode should remain nil when cache is nil")
}

// TestResolveRouteCodesFromCache_NoRouteID verifies that operations without a RouteID are skipped.
func TestResolveRouteCodesFromCache_NoRouteID(t *testing.T) {
	cache := &mmodel.TransactionRouteCache{
		Actions: map[string]mmodel.ActionRouteCache{
			"direct": {
				Source: map[string]mmodel.OperationRouteCache{
					"route-uuid-1": {Code: "RT-SRC-001"},
				},
				Destination:   map[string]mmodel.OperationRouteCache{},
				Bidirectional: map[string]mmodel.OperationRouteCache{},
			},
		},
	}

	ops := []*operation.Operation{
		{ID: "op-1", RouteID: nil},
	}

	resolveRouteCodesFromCache(ops, cache)

	assert.Nil(t, ops[0].RouteCode, "RouteCode should remain nil when RouteID is nil")
}

// TestResolveRouteCodesFromCache_SourceRoute verifies that RouteCode is resolved from source routes.
func TestResolveRouteCodesFromCache_SourceRoute(t *testing.T) {
	routeID := "route-uuid-1"
	cache := &mmodel.TransactionRouteCache{
		Actions: map[string]mmodel.ActionRouteCache{
			"direct": {
				Source: map[string]mmodel.OperationRouteCache{
					"route-uuid-1": {Code: "RT-SRC-001"},
				},
				Destination:   map[string]mmodel.OperationRouteCache{},
				Bidirectional: map[string]mmodel.OperationRouteCache{},
			},
		},
	}

	ops := []*operation.Operation{
		{ID: "op-1", RouteID: &routeID},
	}

	resolveRouteCodesFromCache(ops, cache)

	require.NotNil(t, ops[0].RouteCode, "RouteCode should be populated from source route cache")
	assert.Equal(t, "RT-SRC-001", *ops[0].RouteCode)
}

// TestResolveRouteCodesFromCache_DestinationRoute verifies resolution from destination routes.
func TestResolveRouteCodesFromCache_DestinationRoute(t *testing.T) {
	routeID := "route-uuid-2"
	cache := &mmodel.TransactionRouteCache{
		Actions: map[string]mmodel.ActionRouteCache{
			"direct": {
				Source: map[string]mmodel.OperationRouteCache{},
				Destination: map[string]mmodel.OperationRouteCache{
					"route-uuid-2": {Code: "RT-DST-002"},
				},
				Bidirectional: map[string]mmodel.OperationRouteCache{},
			},
		},
	}

	ops := []*operation.Operation{
		{ID: "op-1", RouteID: &routeID},
	}

	resolveRouteCodesFromCache(ops, cache)

	require.NotNil(t, ops[0].RouteCode, "RouteCode should be populated from destination route cache")
	assert.Equal(t, "RT-DST-002", *ops[0].RouteCode)
}

// TestResolveRouteCodesFromCache_BidirectionalRoute verifies resolution from bidirectional routes.
func TestResolveRouteCodesFromCache_BidirectionalRoute(t *testing.T) {
	routeID := "route-uuid-3"
	cache := &mmodel.TransactionRouteCache{
		Actions: map[string]mmodel.ActionRouteCache{
			"hold": {
				Source:      map[string]mmodel.OperationRouteCache{},
				Destination: map[string]mmodel.OperationRouteCache{},
				Bidirectional: map[string]mmodel.OperationRouteCache{
					"route-uuid-3": {Code: "RT-BIDIR-003"},
				},
			},
		},
	}

	ops := []*operation.Operation{
		{ID: "op-1", RouteID: &routeID},
	}

	resolveRouteCodesFromCache(ops, cache)

	require.NotNil(t, ops[0].RouteCode, "RouteCode should be populated from bidirectional route cache")
	assert.Equal(t, "RT-BIDIR-003", *ops[0].RouteCode)
}

// TestResolveRouteCodesFromCache_MultipleOperations verifies that multiple operations
// are resolved independently from different route types.
func TestResolveRouteCodesFromCache_MultipleOperations(t *testing.T) {
	srcRouteID := "route-src"
	dstRouteID := "route-dst"
	unknownRouteID := "route-unknown"

	cache := &mmodel.TransactionRouteCache{
		Actions: map[string]mmodel.ActionRouteCache{
			"direct": {
				Source: map[string]mmodel.OperationRouteCache{
					"route-src": {Code: "SRC-CODE"},
				},
				Destination: map[string]mmodel.OperationRouteCache{
					"route-dst": {Code: "DST-CODE"},
				},
				Bidirectional: map[string]mmodel.OperationRouteCache{},
			},
		},
	}

	ops := []*operation.Operation{
		{ID: "op-1", RouteID: &srcRouteID},
		{ID: "op-2", RouteID: &dstRouteID},
		{ID: "op-3", RouteID: &unknownRouteID},
		{ID: "op-4", RouteID: nil},
	}

	resolveRouteCodesFromCache(ops, cache)

	require.NotNil(t, ops[0].RouteCode)
	assert.Equal(t, "SRC-CODE", *ops[0].RouteCode)

	require.NotNil(t, ops[1].RouteCode)
	assert.Equal(t, "DST-CODE", *ops[1].RouteCode)

	assert.Nil(t, ops[2].RouteCode, "unknown route ID should leave RouteCode nil")
	assert.Nil(t, ops[3].RouteCode, "nil route ID should leave RouteCode nil")
}

// TestResolveRouteCodesFromCache_EmptyRouteID verifies that an empty string RouteID is skipped.
func TestResolveRouteCodesFromCache_EmptyRouteID(t *testing.T) {
	emptyRouteID := ""
	cache := &mmodel.TransactionRouteCache{
		Actions: map[string]mmodel.ActionRouteCache{
			"direct": {
				Source: map[string]mmodel.OperationRouteCache{
					"": {Code: "SHOULD-NOT-MATCH"},
				},
				Destination:   map[string]mmodel.OperationRouteCache{},
				Bidirectional: map[string]mmodel.OperationRouteCache{},
			},
		},
	}

	ops := []*operation.Operation{
		{ID: "op-1", RouteID: &emptyRouteID},
	}

	resolveRouteCodesFromCache(ops, cache)

	assert.Nil(t, ops[0].RouteCode, "RouteCode should remain nil for empty RouteID")
}
