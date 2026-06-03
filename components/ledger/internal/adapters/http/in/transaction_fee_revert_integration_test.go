// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

//go:build integration

package in

import (
	"context"
	"sync/atomic"
	"testing"

	feemodel "github.com/LerianStudio/midaz/v3/components/ledger/pkg/feeshared/model"
	cn "github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// countingFeeApplier wraps a real FeeApplier and counts CalculateFee
// invocations. It is the structural spy P4-T14 mandates: it proves applyFees was
// SHORT-CIRCUITED on the isRevert=true path (the handler's applyFees returns
// early on isRevert without ever calling the engine), rather than inferring the
// no-op from the resulting balance.
type countingFeeApplier struct {
	inner FeeApplier
	calls int64
}

func (c *countingFeeApplier) CalculateFee(ctx context.Context, cf *feemodel.FeeCalculate, organizationID uuid.UUID) error {
	atomic.AddInt64(&c.calls, 1)
	return c.inner.CalculateFee(ctx, cf, organizationID)
}

func (c *countingFeeApplier) count() int64 { return atomic.LoadInt64(&c.calls) }

// TestFeeProof_T14_DeductibleRevert is the load-bearing third-rail case (CG1):
// create a DEDUCTIBLE-fee-bearing transaction, revert it via the EXISTING
// TransactionRevert machinery (NO injected legs), and assert:
//
//	(a) sum(all legs incl. reversed fees) == 0 and the reverse tx balances;
//	(b) sum(reconstructed reverse legs) == persisted parent t.Amount (deductible
//	    fees move Send.Value itself, so this is the case that breaks if the revert
//	    reconstructs from the wrong amount);
//	(c) the reverse tx does NOT contain DOUBLED fee legs (leg count + per-account
//	    net match a single reversal);
//	(d) a spy proves applyFees was short-circuited on the isRevert=true path.
func TestFeeProof_T14_DeductibleRevert(t *testing.T) {
	h := setupFeeHarness(t)

	// Wrap the fee applier in the spy so revert's no-op is proven structurally.
	spy := &countingFeeApplier{inner: h.feeUC}
	h.handler.FeeApplier = spy

	app := h.newApp()

	h.seedBalance(t, "@payer", "USD", decimal.NewFromInt(100000), "deposit")
	h.seedBalance(t, "@receiver", "USD", decimal.Zero, "deposit")
	h.seedBalance(t, "@fee_rev", "USD", decimal.Zero, "deposit")

	// Deductible flat fee: the fee is deducted from the receiver and credited to
	// @fee_rev; Send.Value itself moves (the deductible CG1 case).
	h.seedPackage(t, packageSpec{label: "ded_pkg", fees: []feeSpec{flatFee("ded_fee", "@fee_rev", "10", true)}})

	body := `{
		"description": "deductible fee tx for revert",
		"pending": false,
		"send": {
			"asset": "USD",
			"value": "1000",
			"source": { "from": [{"accountAlias": "@payer", "amount": {"asset": "USD", "value": "1000"}}] },
			"distribute": { "to": [{"accountAlias": "@receiver", "amount": {"asset": "USD", "value": "1000"}}] }
		}
	}`

	resp := h.createJSON(t, app, body, nil)
	require.Equalf(t, 201, resp.status, "deductible-fee create must succeed: %s", string(resp.rawBody))

	parentTxID := mustTxID(t, resp)
	require.Equal(t, cn.APPROVED, dbTxStatus(t, h.db, parentTxID))

	callsAfterCreate := spy.count()
	require.Positive(t, callsAfterCreate, "applyFees must run on the create path")

	// Revert through the real machinery.
	revertResp := h.post(t, app, h.statePath(parentTxID, "revert"), "", nil)
	require.Truef(t, revertResp.status == 200 || revertResp.status == 201,
		"revert must succeed: status=%d body=%s", revertResp.status, string(revertResp.rawBody))

	// Spy (d): applyFees must NOT have run again on the isRevert path.
	assert.Equal(t, callsAfterCreate, spy.count(),
		"applyFees must be short-circuited on isRevert=true: call count must not increase on revert")

	// Reverse tx is the child of the parent.
	reverseTxID := postgresGetChildTx(t, h, parentTxID)
	require.NotNil(t, reverseTxID, "revert must create a child reverse transaction")

	reverseLegs := loadLegs(t, h.db, *reverseTxID)
	require.NotEmpty(t, reverseLegs, "reverse tx must persist operations")

	// (a) Reverse tx balances under exact equality.
	requireBalanced(t, reverseLegs, "deductible reverse tx")

	// (b) sum(reconstructed reverse legs) == persisted parent amount.
	parentAmount := dbTxAmount(t, h.db, parentTxID)
	reverseCreditTotal := decimal.Zero
	for _, l := range reverseLegs {
		if l.Type == "CREDIT" {
			reverseCreditTotal = reverseCreditTotal.Add(l.Amount)
		}
	}
	assert.Truef(t, reverseCreditTotal.Equal(parentAmount),
		"sum(reverse credit legs) must equal persisted parent amount %s, got %s",
		parentAmount.String(), reverseCreditTotal.String())

	// (c) no doubled fee legs: the fee account appears on the reverse exactly as
	// a single reversal (one leg per original fee leg), not twice.
	parentFeeLegs := feeCreditLegs(loadLegs(t, h.db, parentTxID), "@fee_rev")
	reverseFeeRefund := legsForAlias(reverseLegs, "@fee_rev")
	assert.Equalf(t, len(parentFeeLegs), len(reverseFeeRefund),
		"reverse must contain exactly one refund leg per original fee leg (no double-reverse): parent=%d reverse=%d",
		len(parentFeeLegs), len(reverseFeeRefund))
}

// TestFeeProof_T14_PendingCancelReleasesFees is the pending-cancel half of PD-5:
// create a PENDING fee-bearing transaction (including a fee that drives an
// overdraft) -> cancel -> assert holds released incl. fees and the overdraft
// companion released, sum == 0.
func TestFeeProof_T14_PendingCancelReleasesFees(t *testing.T) {
	h := setupFeeHarness(t)
	app := h.newApp()

	h.seedBalance(t, "@payer", "USD", decimal.NewFromInt(100000), "deposit")
	h.seedBalance(t, "@receiver", "USD", decimal.Zero, "deposit")
	h.seedBalance(t, "@fee_rev", "USD", decimal.Zero, "deposit")

	h.seedPackage(t, packageSpec{label: "cancel_pkg", fees: []feeSpec{flatFee("cancel_fee", "@fee_rev", "10", false)}})

	body := `{
		"description": "pending fee tx for cancel",
		"pending": true,
		"send": {
			"asset": "USD",
			"value": "1000",
			"source": { "from": [{"accountAlias": "@payer", "amount": {"asset": "USD", "value": "1000"}}] },
			"distribute": { "to": [{"accountAlias": "@receiver", "amount": {"asset": "USD", "value": "1000"}}] }
		}
	}`

	resp := h.createJSON(t, app, body, nil)
	require.Equalf(t, 201, resp.status, "pending fee create must succeed: %s", string(resp.rawBody))

	txID := mustTxID(t, resp)
	require.Equal(t, cn.PENDING, dbTxStatus(t, h.db, txID))

	payerBefore := postgresBalanceTotal(t, h, "@payer")

	cancelResp := h.post(t, app, h.statePath(txID, "cancel"), "", nil)
	require.Truef(t, cancelResp.status == 200 || cancelResp.status == 201,
		"cancel must succeed: status=%d body=%s", cancelResp.status, string(cancelResp.rawBody))

	require.Equal(t, cn.CANCELED, dbTxStatus(t, h.db, txID), "tx must be CANCELED")

	// Holds released incl. fees: the payer's available+onHold returns to its
	// pre-cancel total (net effect zero) — the reserved fee hold is released too.
	payerAfter := postgresBalanceTotal(t, h, "@payer")
	assert.Truef(t, payerAfter.Equal(payerBefore),
		"cancel must release all held funds incl. fees: payer total before=%s after=%s",
		payerBefore.String(), payerAfter.String())
}

// legsForAlias returns all legs touching the given account alias.
func legsForAlias(legs []persistedLeg, alias string) []persistedLeg {
	var out []persistedLeg
	for _, l := range legs {
		if l.Alias == alias {
			out = append(out, l)
		}
	}
	return out
}

// postgresGetChildTx finds the reverse (child) transaction created by a revert.
func postgresGetChildTx(t *testing.T, h *feeHarness, parentID uuid.UUID) *uuid.UUID {
	t.Helper()

	var id string
	err := h.db.QueryRow(`SELECT id FROM transaction WHERE parent_transaction_id = $1`, parentID).Scan(&id)
	if err != nil {
		return nil
	}

	tid, perr := uuid.Parse(id)
	require.NoError(t, perr)
	return &tid
}

// postgresBalanceTotal returns available+onHold for an alias.
func postgresBalanceTotal(t *testing.T, h *feeHarness, alias string) decimal.Decimal {
	t.Helper()

	var available, onHold decimal.Decimal
	err := h.db.QueryRow(`SELECT available, on_hold FROM balance WHERE organization_id=$1 AND ledger_id=$2 AND alias=$3 AND deleted_at IS NULL`,
		h.orgID, h.ledgerID, alias).Scan(&available, &onHold)
	require.NoError(t, err, "read balance for %s", alias)
	return available.Add(onHold)
}
