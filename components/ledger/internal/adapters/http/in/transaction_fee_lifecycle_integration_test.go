// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

//go:build integration

package in

import (
	"testing"
	"time"

	cn "github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestFeeProof_T15_IdempotencyReplay is the P4-T15 idempotency E2E: the same
// idempotency key + same body twice returns an identical fee-inclusive response,
// with IdempotencyReplayed=true on the second request. The fee is applied once;
// the replay returns the first fee-inclusive transaction.
func TestFeeProof_T15_IdempotencyReplay(t *testing.T) {
	h := setupFeeHarness(t)
	app := h.newApp()

	h.seedBalance(t, "@payer", "USD", decimal.NewFromInt(100000), "deposit")
	h.seedBalance(t, "@receiver", "USD", decimal.Zero, "deposit")
	h.seedBalance(t, "@fee_rev", "USD", decimal.Zero, "deposit")

	h.seedPackage(t, packageSpec{label: "idem_pkg", fees: []feeSpec{flatFee("idem_fee", "@fee_rev", "10", false)}})

	body := `{
		"description": "idempotent fee tx",
		"pending": false,
		"send": {
			"asset": "USD",
			"value": "1000",
			"source": { "from": [{"accountAlias": "@payer", "amount": {"asset": "USD", "value": "1000"}}] },
			"distribute": { "to": [{"accountAlias": "@receiver", "amount": {"asset": "USD", "value": "1000"}}] }
		}
	}`

	key := "fee-idem-" + uuid.New().String()
	headers := map[string]string{"X-Idempotency": key, "X-TTL": "60"}

	first := h.createJSON(t, app, body, headers)
	require.Equalf(t, 201, first.status, "first create must succeed: %s", string(first.rawBody))
	assert.Equal(t, "false", first.replayed, "first request must not be a replay")

	// Allow the async idempotency persistence to land.
	time.Sleep(300 * time.Millisecond)

	second := h.createJSON(t, app, body, headers)
	require.Equalf(t, 201, second.status, "replay must succeed: %s", string(second.rawBody))
	assert.Equal(t, "true", second.replayed, "second identical request must set IdempotencyReplayed=true")

	assert.Equal(t, first.body["id"], second.body["id"], "replay must return the SAME fee-inclusive transaction id")

	// The fee was applied exactly once: only one transaction's worth of fee legs
	// exists on the (single) persisted transaction.
	txID := mustTxID(t, first)
	feeLegs := feeCreditLegs(loadLegs(t, h.db, txID), "@fee_rev")
	require.NotEmpty(t, feeLegs, "the original (replayed) transaction must carry fee legs")
	assert.Truef(t, sumAmounts(feeLegs).Equal(decimal.NewFromInt(10)),
		"fee applied exactly once: total fee legs must equal 10, got %s", sumAmounts(feeLegs).String())
}

// TestFeeProof_T13_CommitParity is the P4-T13 no-double-charge assertion: a
// PENDING fee-bearing transaction committed must SETTLE the already-reserved fee
// exactly once — the commit state handler must NOT re-invoke applyFees, so the
// payer is charged amount+fee a single time and @fee_rev receives the single
// configured fee, never doubled.
//
// The invariant is asserted against the persisted operation legs — the suite's
// canonical model (transaction_integration_test.go proves the pending->commit
// lifecycle via operation rows + balance state, never via a signed-sum over the
// raw row UNION). A committed pending transaction persists BOTH the pending
// ON_HOLD reservation rows AND the commit-phase settlement rows; the two are
// distinct phases of one money movement, so a naive signed-sum over their union
// double-counts the payer outflow (it nets to -1010, not 0) even though money is
// conserved. The real double-entry invariant is over the SETTLEMENT legs alone:
// the commit-phase DEBIT/CREDIT rows net to zero, the intra-account ON_HOLD
// reservation rows are excluded (a reservation is Available->OnHold on a single
// account, not an inter-account transfer).
func TestFeeProof_T13_CommitParity(t *testing.T) {
	h := setupFeeHarness(t)

	// Spy the fee applier so commit's no-op is proven STRUCTURALLY: the commit
	// path has no applyFees call, so the engine invocation count must not change.
	spy := &countingFeeApplier{inner: h.feeUC}
	h.handler.FeeApplier = spy

	app := h.newApp()

	h.seedBalance(t, "@payer", "USD", decimal.NewFromInt(100000), "deposit")
	h.seedBalance(t, "@receiver", "USD", decimal.Zero, "deposit")
	h.seedBalance(t, "@fee_rev", "USD", decimal.Zero, "deposit")

	const (
		sendValue = 1000
		feeValue  = 10
	)
	h.seedPackage(t, packageSpec{label: "commit_pkg", fees: []feeSpec{flatFee("commit_fee", "@fee_rev", "10", false)}})

	body := `{
		"description": "pending fee tx for commit",
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

	// At pending, the fee is RESERVED with the principal: the payer holds
	// amount+fee. The fee CREDIT to @fee_rev is a destination leg that defers to
	// commit, so there is no @fee_rev settlement leg yet. The reservation rows
	// (ON_HOLD@payer) total amount+fee — that is the single charge being held.
	pendingLegs := loadLegs(t, h.db, txID)
	pendingHoldTotal := sumAmounts(reservationLegs(pendingLegs))
	require.Truef(t, pendingHoldTotal.Equal(decimal.NewFromInt(sendValue+feeValue)),
		"pending must reserve amount+fee exactly once on the payer: want %d, got %s",
		sendValue+feeValue, pendingHoldTotal.String())
	require.Empty(t, feeCreditLegs(pendingLegs, "@fee_rev"),
		"the fee CREDIT to @fee_rev is a destination leg deferred to commit; none must exist at pending")

	callsAfterPending := spy.count()
	require.Positive(t, callsAfterPending, "applyFees must run on the pending-create path")

	commitResp := h.post(t, app, h.statePath(txID, "commit"), "", nil)
	require.Equalf(t, 201, commitResp.status, "commit must succeed: %s", string(commitResp.rawBody))
	require.Equal(t, cn.APPROVED, dbTxStatus(t, h.db, txID))

	// (1) applyFees NOT re-invoked on commit: the engine call count is unchanged
	// across the commit. This is the structural no-double-charge guard.
	assert.Equalf(t, callsAfterPending, spy.count(),
		"commit must NOT re-invoke applyFees: engine call count must not increase (pending=%d, after-commit=%d)",
		callsAfterPending, spy.count())

	committedLegs := loadLegs(t, h.db, txID)
	settlement := settlementLegs(committedLegs)

	// (2) SETTLEMENT BALANCES: the commit-phase legs (ON_HOLD reservation rows
	// excluded) net to exactly zero — the payer's settlement DEBIT total equals
	// the receiver + fee-revenue CREDIT total. Exact decimal equality.
	require.NotEmpty(t, settlement, "commit must persist settlement legs")
	settlementNet := signedSum(settlement)
	assert.Truef(t, settlementNet.Equal(decimal.Zero),
		"commit settlement legs must net to exactly zero (ON_HOLD reservation excluded), got %s", settlementNet.String())

	payerSettleDebit := sumAmounts(debitLegsForAlias(settlement, "@payer"))
	receiverCredit := sumAmounts(feeCreditLegs(settlement, "@receiver"))
	committedFeeTotal := sumAmounts(feeCreditLegs(settlement, "@fee_rev"))
	assert.Truef(t, payerSettleDebit.Equal(receiverCredit.Add(committedFeeTotal)),
		"settlement must balance: payer DEBIT %s must equal receiver CREDIT %s + fee CREDIT %s",
		payerSettleDebit.String(), receiverCredit.String(), committedFeeTotal.String())

	// (3) NO DOUBLE CHARGE — fee side: @fee_rev receives the SINGLE configured fee
	// (10), never doubled (20). A commit that re-applied fees would credit 20 here.
	assert.Truef(t, committedFeeTotal.Equal(decimal.NewFromInt(feeValue)),
		"committed @fee_rev CREDIT total must equal the single configured fee %d (not doubled), got %s",
		feeValue, committedFeeTotal.String())

	// (3) NO DOUBLE CHARGE — payer side: the payer is settled for amount+fee
	// exactly once, and that settlement DEBIT equals the amount+fee that was held
	// at pending. A double charge would settle 2020 (or hold 1010 then settle 2020).
	assert.Truef(t, payerSettleDebit.Equal(decimal.NewFromInt(sendValue+feeValue)),
		"payer settlement DEBIT must charge amount+fee exactly once: want %d, got %s",
		sendValue+feeValue, payerSettleDebit.String())
	assert.Truef(t, payerSettleDebit.Equal(pendingHoldTotal),
		"the committed payer charge %s must equal the amount+fee reserved at pending %s — settle the hold, do not re-charge",
		payerSettleDebit.String(), pendingHoldTotal.String())

	// Receiver is credited the principal exactly (the fee did not erode the
	// non-deductible transfer).
	assert.Truef(t, receiverCredit.Equal(decimal.NewFromInt(sendValue)),
		"receiver must be credited the full principal %d, got %s", sendValue, receiverCredit.String())
}

// reservationLegs returns the intra-account ON_HOLD reservation rows — funds
// moved Available->OnHold on a single account at pending creation. They are NOT
// inter-account transfers and are excluded from the settlement balance.
func reservationLegs(legs []persistedLeg) []persistedLeg {
	var out []persistedLeg
	for _, l := range legs {
		if l.Type == "ON_HOLD" {
			out = append(out, l)
		}
	}
	return out
}

// settlementLegs returns the commit-phase inter-account transfer rows
// (DEBIT/CREDIT), excluding the pending ON_HOLD reservation rows. A committed
// pending transaction's settlement legs net to zero under double-entry.
func settlementLegs(legs []persistedLeg) []persistedLeg {
	var out []persistedLeg
	for _, l := range legs {
		if l.Type == "DEBIT" || l.Type == "CREDIT" {
			out = append(out, l)
		}
	}
	return out
}

// debitLegsForAlias returns the DEBIT legs that debit the given account.
func debitLegsForAlias(legs []persistedLeg, alias string) []persistedLeg {
	var out []persistedLeg
	for _, l := range legs {
		if l.Alias == alias && l.Type == "DEBIT" {
			out = append(out, l)
		}
	}
	return out
}
