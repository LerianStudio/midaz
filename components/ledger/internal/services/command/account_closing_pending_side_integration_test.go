//go:build integration

// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"testing"

	libCommons "github.com/LerianStudio/lib-commons/v7/commons"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/pkg/constant"
	pgtestutil "github.com/LerianStudio/midaz/v4/tests/utils/postgres"
)

// TestIntegrationAccountClosingIgnoresAPendingItOnlyReceives is AC-08b at the
// command boundary.
//
// A hold reserves funds on the source alone, and the posting plan of a hold
// projects nothing for the destination side — so a pending transaction that names
// an account only as destination leaves no operation row tying it to that account.
// The destination is therefore free to close while the pending is still alive; what
// answers for the inbound side is the closed-account refusal at commit, not an
// eligibility rule here.
//
// The fixture writes exactly what a hold persists: the PENDING transaction and its
// source-side leg, and nothing for the destination.
func TestIntegrationAccountClosingIgnoresAPendingItOnlyReceives(t *testing.T) {
	h := newAccountClosingHarness(t)
	ctx := context.Background()

	source := h.seedAccount(t, "@closing-pending-source", "deposit")
	destination := h.seedAccount(t, "@closing-pending-destination", "deposit")

	h.seedBalance(t, source, accountClosingBalanceSeed{alias: "@closing-pending-source", key: "default"})
	destinationBalance := h.seedBalance(t, destination, accountClosingBalanceSeed{alias: "@closing-pending-destination", key: "default"})

	transactionID := pgtestutil.CreateTestTransactionWithStatus(t, h.transactionDB, h.organizationID, h.ledgerID,
		constant.PENDING, decimal.NewFromInt(10), "USD")

	pgtestutil.CreateTestOperation(t, h.transactionDB, h.organizationID, h.ledgerID, pgtestutil.OperationParams{
		TransactionID: transactionID,
		Description:   "Account closing hold",
		Type:          constant.DEBIT,
		AccountID:     source,
		AccountAlias:  "@closing-pending-source",
		BalanceID:     uuid.Must(libCommons.GenerateUUIDv7()),
		AssetCode:     "USD",
		Amount:        decimal.NewFromInt(10),
	})

	var legs int

	require.NoError(t, h.transactionDB.QueryRow(
		`SELECT count(*) FROM operation WHERE transaction_id = $1 AND account_id = $2`, transactionID, destination).Scan(&legs))
	require.Zero(t, legs, "a hold projects no operation row for the account it only credits later")

	closedAt, err := h.close(ctx, destination)
	require.NoError(t, err, "an inbound pending is no impediment to closing the destination")

	stored := h.closedAt(t, destination)
	require.True(t, stored.Valid)
	require.True(t, closedAt.Equal(stored.Time))

	require.Equal(t, constant.PENDING, pgtestutil.GetTransactionStatus(t, h.transactionDB, transactionID),
		"the closing neither commits nor cancels the pending transaction it left alive")

	balances, err := h.uc.BalanceRepo.ListByAccountID(ctx, h.organizationID, h.ledgerID, destination)
	require.NoError(t, err)
	require.Len(t, balances, 1)
	require.Equal(t, int64(0), h.balanceRow(t, destinationBalance).version, "the closing moves no money on the destination")
}

// TestIntegrationAccountClosingRefusesTheSourceOfALivePending is AC-08 and the
// cross-check the design asks for: the live monetary evidence of the source reads
// zero — the hold is recorded on a balance the fixture leaves settled — and the
// durable operation record still ties the account to a PENDING transaction. The two
// sources of evidence disagree, and the closing fails closed on the durable one.
func TestIntegrationAccountClosingRefusesTheSourceOfALivePending(t *testing.T) {
	h := newAccountClosingHarness(t)
	ctx := context.Background()

	source := h.seedAccount(t, "@closing-pending-holder", "deposit")
	row := accountClosingBalanceSeed{alias: "@closing-pending-holder", key: "default"}
	balanceID := h.seedBalance(t, source, row)

	// Every monetary component reads zero, in the cache and in the row alike.
	cacheKey := h.cacheBalance(t, source, balanceID, row)

	transactionID := h.seedPendingTransaction(t, source, constant.DEBIT)

	_, err := h.close(ctx, source)
	requireClosingCode(t, err, constant.ErrAccountHasPendingTransactions)

	require.False(t, h.closedAt(t, source).Valid, "a pending the account still owes keeps it open")
	require.Equal(t, int64(1), h.exists(t, cacheKey), "the refusal evicts nothing")

	protection := h.protection(source)
	require.Equal(t, int64(0), h.exists(t, protection.closing, protection.closed, protection.ownership),
		"a known refusal gives back the protection of its own attempt")

	// Once the pending transaction is no longer pending the same account closes.
	pgtestutil.UpdateTransactionStatus(t, h.transactionDB, transactionID, constant.CANCELED)

	_, err = h.close(ctx, source)
	require.NoError(t, err, "regularizing the pending transaction is what unblocks the closing")
}
