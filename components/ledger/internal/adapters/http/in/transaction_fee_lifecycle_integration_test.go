// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

//go:build integration

package in

import (
	"strings"
	"testing"
	"time"

	libCommons "github.com/LerianStudio/lib-commons/v7/commons"
	redistransaction "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/redis/transaction"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/services/command"
	cn "github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mtransaction"
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
	app := h.newV2App()

	h.seedBalance(t, "@payer", "USD", decimal.NewFromInt(100000), "deposit")
	h.seedBalance(t, "@receiver", "USD", decimal.Zero, "deposit")
	h.seedBalance(t, "@fee_rev", "USD", decimal.Zero, "deposit")

	h.seedPackage(t, packageSpec{label: "idem_pkg", fees: []feeSpec{flatFee("idem_fee", "@fee_rev", "10", false)}})

	body := h.v2Body("idempotent fee tx", "USD", "1000",
		[]string{h.v2Leg("@payer", "1000")},
		[]string{h.v2Leg("@receiver", "1000")})

	key := "fee-idem-" + uuid.New().String()
	headers := map[string]string{"X-Idempotency": key, "X-TTL": "60"}

	first := h.createV2Direct(t, app, body, headers)
	require.Equalf(t, 201, first.status, "first create must succeed: %s", string(first.rawBody))
	assert.Equal(t, "false", first.replayed, "first request must not be a replay")

	// Allow the async idempotency persistence to land.
	time.Sleep(300 * time.Millisecond)

	second := h.createV2Direct(t, app, body, headers)
	require.Equalf(t, 201, second.status, "replay must succeed: %s", string(second.rawBody))
	assert.Equal(t, "true", second.replayed, "second identical request must set IdempotencyReplayed=true")

	assert.Equal(t, first.body["id"], second.body["id"], "replay must return the SAME fee-inclusive transaction id")

	// The fee was applied exactly once: only one transaction's worth of fee legs
	// exists on the (single) persisted transaction.
	txID := mustTxID(t, first)
	feeLegs := legsFor(loadLegs(t, h.db, txID), "@fee_rev", "CREDIT")
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
	h.handler.Command.FeeApplier = spy

	app := h.newV2App()

	h.seedBalance(t, "@payer", "USD", decimal.NewFromInt(100000), "deposit")
	h.seedBalance(t, "@receiver", "USD", decimal.Zero, "deposit")
	h.seedBalance(t, "@fee_rev", "USD", decimal.Zero, "deposit")

	const (
		sendValue = 1000
		feeValue  = 10
	)
	h.seedPackage(t, packageSpec{label: "commit_pkg", fees: []feeSpec{flatFee("commit_fee", "@fee_rev", "10", false)}})

	body := h.v2Body("pending fee tx for commit", "USD", "1000",
		[]string{h.v2Leg("@payer", "1000")},
		[]string{h.v2Leg("@receiver", "1000")})

	resp := h.createV2Hold(t, app, body, nil)
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
	require.Empty(t, legsFor(pendingLegs, "@fee_rev", "CREDIT"),
		"the fee CREDIT to @fee_rev is a destination leg deferred to commit; none must exist at pending")

	callsAfterPending := spy.count()
	require.Positive(t, callsAfterPending, "applyFees must run on the pending-create path")

	commitResp := h.post(t, app, h.v2StatePath(txID, "commit"), "", nil)
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

	payerSettleDebit := sumAmounts(legsFor(settlement, "@payer", "DEBIT"))
	receiverCredit := sumAmounts(legsFor(settlement, "@receiver", "CREDIT"))
	committedFeeTotal := sumAmounts(legsFor(settlement, "@fee_rev", "CREDIT"))
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

// TestFeeProof_AtomicBatchV2HoldReservesAndSettlesFee proves a hold item in a /v2
// atomic batch is charged like a single /v2 hold: the payer reserves principal+fee,
// the fee account is credited only when the pending transaction is committed.
func TestFeeProof_AtomicBatchV2HoldReservesAndSettlesFee(t *testing.T) {
	h := setupFeeHarness(t)
	h.enableAccountingEngine(t)
	engineRedis, ok := h.redisRepo.(*redistransaction.RedisConsumerRepository)
	require.True(t, ok, "Redis repository must expose the batch state machine and engine evidence")
	h.queryUC.EngineWriteBehindCodec = command.EngineWriteBehindEvidenceCodec{}
	h.commandUC.TransactionEvidenceResolver = testEngineEvidenceResolver{repository: engineRedis}
	h.commandUC.AtomicTransactionBatchIdempotencyRepo = engineRedis
	h.commandUC.UUIDv7Generator = libCommons.GenerateUUIDv7
	h.commandUC.Clock = func() time.Time { return time.Date(2026, time.September, 28, 12, 0, 0, 0, time.UTC) }
	h.handler.TransactionBatchMaxSize = 1
	app := h.newV2App()

	h.seedBalance(t, "@payer", "USD", decimal.NewFromInt(100000), "deposit")
	h.seedBalance(t, "@receiver", "USD", decimal.Zero, "deposit")
	h.seedBalance(t, "@fee_rev", "USD", decimal.Zero, "deposit")
	h.seedPackage(t, packageSpec{label: "batch_hold_pkg", fees: []feeSpec{flatFee("batch_hold_fee", "@fee_rev", "10", false)}})

	item := h.v2Body("batch hold with fee", "USD", "1000",
		[]string{h.v2Leg("@payer", "1000")},
		[]string{h.v2Leg("@receiver", "1000")})
	batch := `{"transactions":[{"action":"hold","order":1,` + strings.TrimPrefix(item, "{") + `]}`

	resp := h.post(t, app, h.v2CreatePath("batch"), batch, nil)
	require.Equalf(t, 201, resp.status, "batch hold with a fee must succeed: %s", string(resp.rawBody))

	transactions, ok := resp.body["transactions"].([]any)
	require.Truef(t, ok && len(transactions) == 1, "batch must return one transaction: %s", string(resp.rawBody))
	txID := mustTxID(t, txResponse{rawBody: resp.rawBody, body: transactions[0].(map[string]any)})
	require.Equal(t, cn.PENDING, dbTxStatus(t, h.db, txID))

	pendingLegs := loadLegs(t, h.db, txID)
	assert.Truef(t, sumAmounts(legsFor(pendingLegs, "@payer", "ON_HOLD")).Equal(decimal.NewFromInt(1010)),
		"the payer must reserve principal+fee: %+v", pendingLegs)
	assert.Empty(t, legsFor(pendingLegs, "@fee_rev", "CREDIT"), "the fee credit is deferred to commit")

	payer := func() (available, onHold decimal.Decimal) {
		balances, err := h.queryUC.GetBalances(h.ctx(), h.orgID, h.ledgerID, []string{mtransaction.AliasKey("@payer", "default")})
		require.NoError(t, err, "read live payer balance")
		require.Len(t, balances, 1)

		return balances[0].Available, balances[0].OnHold
	}

	available, onHold := payer()
	assert.Truef(t, available.Equal(decimal.NewFromInt(98990)), "payer available after hold: %s", available)
	assert.Truef(t, onHold.Equal(decimal.NewFromInt(1010)), "payer on hold after hold: %s", onHold)
	assertLiveBalance(t, h, "@fee_rev", "default", "0")
	assertLiveBalance(t, h, "@receiver", "default", "0")

	commit := h.post(t, app, h.v2StatePath(txID, "commit"), "", nil)
	require.Equalf(t, 201, commit.status, "commit must succeed: %s", string(commit.rawBody))
	require.Equal(t, cn.APPROVED, dbTxStatus(t, h.db, txID))

	available, onHold = payer()
	assert.Truef(t, available.Equal(decimal.NewFromInt(98990)), "payer available after commit: %s", available)
	assert.Truef(t, onHold.IsZero(), "payer on hold after commit: %s", onHold)
	assertLiveBalance(t, h, "@fee_rev", "default", "10")
	assertLiveBalance(t, h, "@receiver", "default", "1000")
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

// TestFeeProof_MetadataSelectorScoping drives package selection over HTTP: a
// package scoped to a metadata pair is charged on a v2 create whose metadata
// carries it, and the unscoped package is charged when the metadata does not.
func TestFeeProof_MetadataSelectorScoping(t *testing.T) {
	h := setupFeeHarness(t)
	h.enableAccountingEngine(t)
	app := h.newV2App()

	h.seedBalance(t, "@payer", "USD", decimal.NewFromInt(100000), "deposit")
	h.seedBalance(t, "@receiver", "USD", decimal.Zero, "deposit")
	h.seedBalance(t, "@fee_rev", "USD", decimal.Zero, "deposit")
	h.seedBalance(t, "@fee_ted", "USD", decimal.Zero, "deposit")

	h.seedPackage(t, packageSpec{label: "any_pkg", fees: []feeSpec{flatFee("any_fee", "@fee_rev", "10", false)}})
	h.seedPackage(t, packageSpec{
		label:            "ted_salario_pkg",
		metadataSelector: map[string]string{"fee_context": "ted_salario"},
		fees:             []feeSpec{flatFee("ted_fee", "@fee_ted", "25", false)},
	})

	body := h.v2Body("selector tx", "USD", "1000",
		[]string{h.v2Leg("@payer", "1000")},
		[]string{h.v2Leg("@receiver", "1000")})

	tagged := h.createV2Direct(t, app, h.v2WithMetadata(body, `{"fee_context":"ted_salario"}`), nil)
	require.Equalf(t, 201, tagged.status, "tagged create must succeed: %s", string(tagged.rawBody))
	assert.Len(t, legsFor(loadLegs(t, h.db, mustTxID(t, tagged)), "@fee_ted", ""), 1, "the scoped package's credit account must receive the fee")

	plain := h.createV2Direct(t, app, body, nil)
	require.Equalf(t, 201, plain.status, "plain create must succeed: %s", string(plain.rawBody))
	assert.Len(t, legsFor(loadLegs(t, h.db, mustTxID(t, plain)), "@fee_rev", ""), 1, "the unscoped package's credit account must receive the fee")
}
