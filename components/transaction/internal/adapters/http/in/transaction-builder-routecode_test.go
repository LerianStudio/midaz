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
		debitCode         *string
		creditCode        *string
		expectedRouteCode *string
	}{
		{
			name:              "non-nil codes are set on operation",
			debitCode:         strPtr("EXT-001"),
			creditCode:        strPtr("EXT-002"),
			expectedRouteCode: strPtr("EXT-001"),
		},
		{
			name:              "nil codes leaves RouteCode nil",
			debitCode:         nil,
			creditCode:        nil,
			expectedRouteCode: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			op := handler.buildStandardOp(
				blc, ft, amt, bat, tran, transactionInput, now, false, tt.debitCode, tt.creditCode,
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
		name       string
		debitCode  *string
		creditCode *string
	}{
		{
			name:       "non-nil codes are set per direction",
			debitCode:  strPtr("ROUTE-001"),
			creditCode: strPtr("ROUTE-002"),
		},
		{
			name:       "nil codes leaves RouteCode nil on both ops",
			debitCode:  nil,
			creditCode: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ops := handler.buildDoubleEntryPendingOps(
				ctx, blc, ft, amt, bat, tran, transactionInput, now, false, tt.debitCode, tt.creditCode,
			)

			require.Len(t, ops, 2, "should return exactly 2 operations")

			// op[0] is DEBIT → uses debitCode
			if tt.debitCode == nil {
				assert.Nil(t, ops[0].RouteCode, "DEBIT RouteCode should be nil")
			} else {
				require.NotNil(t, ops[0].RouteCode, "DEBIT RouteCode should not be nil")
				assert.Equal(t, *tt.debitCode, *ops[0].RouteCode, "DEBIT RouteCode mismatch")
			}

			// op[1] is ON_HOLD (credit) → uses creditCode
			if tt.creditCode == nil {
				assert.Nil(t, ops[1].RouteCode, "ON_HOLD RouteCode should be nil")
			} else {
				require.NotNil(t, ops[1].RouteCode, "ON_HOLD RouteCode should not be nil")
				assert.Equal(t, *tt.creditCode, *ops[1].RouteCode, "ON_HOLD RouteCode mismatch")
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
		name       string
		debitCode  *string
		creditCode *string
	}{
		{
			name:       "non-nil codes are set per direction",
			debitCode:  strPtr("CANCEL-001"),
			creditCode: strPtr("CANCEL-002"),
		},
		{
			name:       "nil codes leaves RouteCode nil on both ops",
			debitCode:  nil,
			creditCode: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ops := handler.buildDoubleEntryCanceledOps(
				ctx, blc, ft, amt, bat, tran, transactionInput, now, false, tt.debitCode, tt.creditCode,
			)

			require.Len(t, ops, 2, "should return exactly 2 operations")

			// op[0] is RELEASE (debit) → uses debitCode
			if tt.debitCode == nil {
				assert.Nil(t, ops[0].RouteCode, "RELEASE RouteCode should be nil")
			} else {
				require.NotNil(t, ops[0].RouteCode, "RELEASE RouteCode should not be nil")
				assert.Equal(t, *tt.debitCode, *ops[0].RouteCode, "RELEASE RouteCode mismatch")
			}

			// op[1] is CREDIT → uses creditCode
			if tt.creditCode == nil {
				assert.Nil(t, ops[1].RouteCode, "CREDIT RouteCode should be nil")
			} else {
				require.NotNil(t, ops[1].RouteCode, "CREDIT RouteCode should not be nil")
				assert.Equal(t, *tt.creditCode, *ops[1].RouteCode, "CREDIT RouteCode mismatch")
			}
		})
	}
}

// strPtr returns a pointer to the given string.
func strPtr(s string) *string {
	return &s
}
