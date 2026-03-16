// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"context"
	"testing"
	"time"

	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/postgres/transaction"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	pkgTransaction "github.com/LerianStudio/midaz/v3/pkg/transaction"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestBuildStandardOp_RouteCode verifies that buildStandardOp correctly sets
// the RouteCode field on the returned operation.
func TestBuildStandardOp_RouteCode(t *testing.T) {
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

	tests := []struct {
		name              string
		routeCode         *string
		expectedRouteCode *string
	}{
		{
			name:              "non-nil routeCode is set on operation",
			routeCode:         strPtr("EXT-001"),
			expectedRouteCode: strPtr("EXT-001"),
		},
		{
			name:              "nil routeCode leaves RouteCode nil",
			routeCode:         nil,
			expectedRouteCode: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			op := handler.buildStandardOp(
				blc, ft, amt, bat, tran, transactionInput, now, false, tt.routeCode,
			)

			require.NotNil(t, op)

			if tt.expectedRouteCode == nil {
				assert.Nil(t, op.RouteCode, "RouteCode should be nil")
			} else {
				require.NotNil(t, op.RouteCode, "RouteCode should not be nil")
				assert.Equal(t, *tt.expectedRouteCode, *op.RouteCode)
			}
		})
	}
}

// TestBuildDoubleEntryPendingOps_RouteCode verifies that buildDoubleEntryPendingOps
// sets RouteCode on BOTH returned operations.
func TestBuildDoubleEntryPendingOps_RouteCode(t *testing.T) {
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

	tests := []struct {
		name              string
		routeCode         *string
		expectedRouteCode *string
	}{
		{
			name:              "non-nil routeCode is set on both ops",
			routeCode:         strPtr("ROUTE-001"),
			expectedRouteCode: strPtr("ROUTE-001"),
		},
		{
			name:              "nil routeCode leaves RouteCode nil on both ops",
			routeCode:         nil,
			expectedRouteCode: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ops := handler.buildDoubleEntryPendingOps(
				ctx, blc, ft, amt, bat, tran, transactionInput, now, false, tt.routeCode,
			)

			require.Len(t, ops, 2, "should return exactly 2 operations")

			for i, op := range ops {
				if tt.expectedRouteCode == nil {
					assert.Nil(t, op.RouteCode, "op[%d] RouteCode should be nil", i)
				} else {
					require.NotNil(t, op.RouteCode, "op[%d] RouteCode should not be nil", i)
					assert.Equal(t, *tt.expectedRouteCode, *op.RouteCode, "op[%d] RouteCode mismatch", i)
				}
			}
		})
	}
}

// TestBuildDoubleEntryCanceledOps_RouteCode verifies that buildDoubleEntryCanceledOps
// sets RouteCode on BOTH returned operations.
func TestBuildDoubleEntryCanceledOps_RouteCode(t *testing.T) {
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

	tests := []struct {
		name              string
		routeCode         *string
		expectedRouteCode *string
	}{
		{
			name:              "non-nil routeCode is set on both ops",
			routeCode:         strPtr("CANCEL-001"),
			expectedRouteCode: strPtr("CANCEL-001"),
		},
		{
			name:              "nil routeCode leaves RouteCode nil on both ops",
			routeCode:         nil,
			expectedRouteCode: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ops := handler.buildDoubleEntryCanceledOps(
				ctx, blc, ft, amt, bat, tran, transactionInput, now, false, tt.routeCode,
			)

			require.Len(t, ops, 2, "should return exactly 2 operations")

			for i, op := range ops {
				if tt.expectedRouteCode == nil {
					assert.Nil(t, op.RouteCode, "op[%d] RouteCode should be nil", i)
				} else {
					require.NotNil(t, op.RouteCode, "op[%d] RouteCode should not be nil", i)
					assert.Equal(t, *tt.expectedRouteCode, *op.RouteCode, "op[%d] RouteCode mismatch", i)
				}
			}
		})
	}
}

// strPtr returns a pointer to the given string.
func strPtr(s string) *string {
	return &s
}
