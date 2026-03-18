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
