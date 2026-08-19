// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

//go:build integration

package in

import (
	"context"
	nethttp "net/http"
	"sync"
	"testing"

	"github.com/gofiber/fiber/v3"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	cn "github.com/LerianStudio/midaz/v4/pkg/constant"
	postgrestestutil "github.com/LerianStudio/midaz/v4/tests/utils/postgres"
)

// This file is the commit/cancel CONCURRENCY proof for the v2 pending-transaction lifecycle. It
// is the twin of the concurrent-revert single-winner test in the sibling
// transaction_v2_lifecycle_integration_test.go (§19) and reuses that file's container/app
// fixtures verbatim — setupTestInfra, seedTransfer, buildHumaV2DirectApp, postV2Create,
// decodeTxResponse, drainBalanceSync, requireDecimalEqual, countTransactionsInLedger, the
// v2CommitURL/v2CancelURL builders, and the revertRaceResult capture struct — rather than
// standing up a second stack.
//
// Concurrent commit/cancel of the SAME pending transaction is deterministic: exactly ONE request
// wins (201, APPROVED for commit / CANCELED for cancel) and applies the balance effect EXACTLY
// ONCE; every other concurrent request is rejected 409 ErrPendingTransactionLocked (0486). The
// single-contention 0486 mapping and the not-PENDING status backstop (0099) are covered by unit +
// integration tests elsewhere and are NOT re-proved here. What IS proved here is the real
// concurrent race: single winner + balance effect applied EXACTLY ONCE.
//
// NOT PARALLEL: the app builders call libProblem.Install() (process-global huma.NewError hook) and
// Huma validation uses process-global sync.Pools; concurrent BUILDS cross-contaminate. Every test
// here stays sequential (no t.Parallel()). Only the REQUESTS race — the app is built once, before
// the barrier; fiber's App.Test serves each call on its own connection.

// concurrentLifecycleRacers is the number of goroutines that commit (or cancel) the single pending
// transaction at once. Small enough to stay fast, large enough that at least one racer loses the
// SetNX rather than the status check.
const concurrentLifecycleRacers = 8

// concurrentHoldV2Body is a 500 USD v2 `hold` body: from a source funded with 1000 it opens a
// PENDING transfer that reserves 500 (source available 500 / on-hold 500). Its amount is the whole
// point — the exactly-once balance assertions below key off the held 500 moving (commit) or being
// released (cancel) precisely once.
const concurrentHoldV2Body = `{"description":"concurrent pending lifecycle hold subject","asset":"USD","amount":"500","debits":[{"alias":"@src",` + v2ScopeJSON + `,"amount":"500"}],"credits":[{"alias":"@dst",` + v2ScopeJSON + `,"amount":"500"}]}`

// racePendingLifecycleOp fires concurrentLifecycleRacers bodiless POSTs at url from a common
// closed-channel barrier so every racer enters commitOrCancelTransaction together, and returns each
// racer's raw outcome. It reuses fireRevert, the sibling file's goroutine-safe bodiless-POST racer:
// commit and cancel are bodiless POSTs whose response is either a transaction (201) or an RFC 9457
// problem (4xx) — the identical envelope shape fireRevert decodes. Failures are captured as data
// because require/assert call t.FailNow, which is illegal outside the test goroutine.
//
// The two-barrier handshake guarantees genuine simultaneity: every racer signals ready.Done() and
// then parks on <-start, and the caller only close(start)s after ready.Wait() confirms all racers
// have reached the release point. Without the readiness barrier close(start) could fire before a
// racer parked, letting requests serialize and the single-winner proof pass vacuously.
func racePendingLifecycleOp(ctx context.Context, t *testing.T, app *fiber.App, url string) []revertRaceResult {
	t.Helper()

	require.NoError(t, ctx.Err(), "context must be live before launching the lifecycle racers")

	results := make([]revertRaceResult, concurrentLifecycleRacers)
	start := make(chan struct{})

	var (
		wg    sync.WaitGroup
		ready sync.WaitGroup
	)

	ready.Add(len(results))

	for i := range results {
		wg.Add(1)

		go func(slot int) {
			defer wg.Done()

			ready.Done()
			<-start

			results[slot] = fireRevert(app, url)
		}(i)
	}

	ready.Wait()
	close(start)
	wg.Wait()

	return results
}

// assertExactlyOneLifecycleWinner asserts EXACTLY one racer succeeded (201) and every other racer
// was rejected with 409 ErrPendingTransactionLocked (0486), then returns the winner's transaction
// id as a parsed uuid.UUID. Unlike revert, commit/cancel take no idempotency slot: there is no 201
// replay outcome, and the pending-transaction lock is held for its full TTL rather than released on
// success, so a losing racer can only ever be 0486 — never the 0099 not-pending backstop, and never
// a server error. All assertions run on the test goroutine, after the WaitGroup drained.
func assertExactlyOneLifecycleWinner(t *testing.T, results []revertRaceResult) uuid.UUID {
	t.Helper()

	var (
		winners    []revertRaceResult
		loserCodes []string
	)

	for i, res := range results {
		require.NoErrorf(t, res.transportErr, "racer %d failed at the transport layer", i)

		if res.status == nethttp.StatusCreated {
			winners = append(winners, res)

			continue
		}

		assert.Equalf(t, nethttp.StatusConflict, res.status,
			"racer %d: a losing commit/cancel must be 409 Conflict, not %d; body: %s", i, res.status, res.body)
		assert.Equalf(t, cn.ErrPendingTransactionLocked.Error(), res.problemCode,
			"racer %d: a losing commit/cancel may only be rejected for losing the pending-transaction lock (0486); body: %s", i, res.body)

		loserCodes = append(loserCodes, res.problemCode)
	}

	t.Logf("concurrent pending lifecycle op: %d/%d succeeded, loser problem codes %v",
		len(winners), len(results), loserCodes)

	require.Lenf(t, winners, 1,
		"exactly ONE concurrent commit/cancel of a single pending transaction may succeed; %d did, which means the pending-transaction mutex did NOT serialize the state transition and the balance effect was applied more than once",
		len(winners))

	require.NotEmpty(t, winners[0].txID, "the winning commit/cancel must return the transaction id")

	winnerID, err := uuid.Parse(winners[0].txID)
	require.NoErrorf(t, err, "the winning commit/cancel transaction id %q must be a valid UUID", winners[0].txID)

	return winnerID
}

// =============================================================================
// CONCURRENT COMMIT OF ONE PENDING TRANSACTION — EXACTLY ONE WINNER (money path): N racers commit
// the SAME pending transaction from a common start barrier. The per-transaction SetNX mutex serializes
// them, so exactly one wins (201, APPROVED) and every other gets 409/0486 BEFORE any balance work. The
// held 500 releases from the source and credits the destination EXACTLY ONCE — asserted by exact
// equality, so a second application (destination available reads 1000, source on-hold goes non-zero)
// fails the test deterministically.
// =============================================================================

func TestIntegration_TransactionV2Commit_ConcurrentSingleWinner(t *testing.T) {
	// NOT parallel: process-global huma state (see file header). The requests inside race; the
	// test itself does not run alongside other tests.
	t.Setenv("ALLOW_INSECURE_TLS", "true")

	infra := setupTestInfra(t)
	t.Setenv("RABBITMQ_TRANSACTION_ASYNC", "false")

	ctx := context.Background()

	srcID, dstID := seedTransfer(t, infra.pgContainer.DB, infra.orgID, infra.ledgerID, "@src", "@dst", 1000)

	v2App := buildHumaV2DirectApp(t, infra.handler)

	// Open the PENDING transfer of 500 and settle its balance-sync schedule before racing.
	holdResp := decodeTxResponse(t, postV2Create(t, v2App, "hold", infra.orgID, infra.ledgerID, concurrentHoldV2Body, ""), nethttp.StatusCreated)
	rawPendingID, ok := holdResp["id"].(string)
	require.True(t, ok, "the hold response must carry a string transaction id")
	pendingID, err := uuid.Parse(rawPendingID)
	require.NoErrorf(t, err, "the hold response transaction id %q must be a valid UUID", rawPendingID)
	require.Equal(t, cn.PENDING, postgrestestutil.GetTransactionStatus(t, infra.pgContainer.DB, pendingID), "the hold subject should open PENDING")
	drainBalanceSync(t, ctx, infra.handler.Command, infra.redisRepo, infra.orgID, infra.ledgerID)

	// The pending hold reserved 500: source available 500 / on-hold 500, destination untouched.
	requireDecimalEqual(t, decimal.NewFromInt(500), postgrestestutil.GetBalanceAvailable(t, infra.pgContainer.DB, srcID), "source available after the pending hold")
	requireDecimalEqual(t, decimal.NewFromInt(500), postgrestestutil.GetBalanceOnHold(t, infra.pgContainer.DB, srcID), "source on-hold after the pending hold")
	requireDecimalEqual(t, decimal.Zero, postgrestestutil.GetBalanceAvailable(t, infra.pgContainer.DB, dstID), "destination available after the pending hold")
	requireDecimalEqual(t, decimal.Zero, postgrestestutil.GetBalanceOnHold(t, infra.pgContainer.DB, dstID), "destination on-hold after the pending hold")

	// Race: every goroutine parks on the same closed-channel barrier so they enter the commit core
	// together rather than in spawn order.
	results := racePendingLifecycleOp(ctx, t, v2App, v2CommitURL(infra.orgID, infra.ledgerID, pendingID))

	winnerID := assertExactlyOneLifecycleWinner(t, results)
	assert.Equal(t, pendingID, winnerID, "the winning commit must be the raced pending transaction, not some other id")

	drainBalanceSync(t, ctx, infra.handler.Command, infra.redisRepo, infra.orgID, infra.ledgerID)

	// APPROVED once, and the ledger still holds exactly the one transaction — commit transitions the
	// pending transaction, it does not create a new one.
	assert.Equal(t, cn.APPROVED, postgrestestutil.GetTransactionStatus(t, infra.pgContainer.DB, pendingID), "the raced transaction must settle APPROVED")
	assert.Equal(t, 1, countTransactionsInLedger(t, infra.pgContainer.DB, infra.ledgerID), "a concurrent commit must not persist a new transaction")

	// The commit effect applied EXACTLY ONCE: the held 500 released and moved to the destination.
	// Exact equality, never a range — a double commit would over-credit the destination available to
	// 1000 (asserted ==500) and/or leave the source on-hold non-zero instead of released (asserted
	// ==0), and either reads as a hard failure here.
	requireDecimalEqual(t, decimal.NewFromInt(500), postgrestestutil.GetBalanceAvailable(t, infra.pgContainer.DB, srcID), "source available exactly 500 after commit (a double commit would read below 500)")
	requireDecimalEqual(t, decimal.Zero, postgrestestutil.GetBalanceOnHold(t, infra.pgContainer.DB, srcID), "source on-hold released exactly once after commit")
	requireDecimalEqual(t, decimal.NewFromInt(500), postgrestestutil.GetBalanceAvailable(t, infra.pgContainer.DB, dstID), "destination credited exactly 500 after commit (a double commit would read 1000)")
	requireDecimalEqual(t, decimal.Zero, postgrestestutil.GetBalanceOnHold(t, infra.pgContainer.DB, dstID), "destination on-hold after commit")
}

// =============================================================================
// CONCURRENT CANCEL OF ONE PENDING TRANSACTION — EXACTLY ONE WINNER (money path): N racers cancel
// the SAME pending transaction from a common start barrier. Exactly one wins (201, CANCELED) and every
// other gets 409/0486. The reservation is released back to available EXACTLY ONCE — the source returns
// to its full 1000 and the destination is never credited — asserted by exact equality, so an over-release
// (source at 1500) fails deterministically.
// =============================================================================

func TestIntegration_TransactionV2Cancel_ConcurrentSingleWinner(t *testing.T) {
	// NOT parallel: process-global huma state (see file header). The requests inside race; the
	// test itself does not run alongside other tests.
	t.Setenv("ALLOW_INSECURE_TLS", "true")

	infra := setupTestInfra(t)
	t.Setenv("RABBITMQ_TRANSACTION_ASYNC", "false")

	ctx := context.Background()

	srcID, dstID := seedTransfer(t, infra.pgContainer.DB, infra.orgID, infra.ledgerID, "@src", "@dst", 1000)

	v2App := buildHumaV2DirectApp(t, infra.handler)

	// Open the PENDING transfer of 500 and settle its balance-sync schedule before racing.
	holdResp := decodeTxResponse(t, postV2Create(t, v2App, "hold", infra.orgID, infra.ledgerID, concurrentHoldV2Body, ""), nethttp.StatusCreated)
	rawPendingID, ok := holdResp["id"].(string)
	require.True(t, ok, "the hold response must carry a string transaction id")
	pendingID, err := uuid.Parse(rawPendingID)
	require.NoErrorf(t, err, "the hold response transaction id %q must be a valid UUID", rawPendingID)
	require.Equal(t, cn.PENDING, postgrestestutil.GetTransactionStatus(t, infra.pgContainer.DB, pendingID), "the hold subject should open PENDING")
	drainBalanceSync(t, ctx, infra.handler.Command, infra.redisRepo, infra.orgID, infra.ledgerID)

	// The pending hold reserved 500: source available 500 / on-hold 500, destination untouched.
	requireDecimalEqual(t, decimal.NewFromInt(500), postgrestestutil.GetBalanceAvailable(t, infra.pgContainer.DB, srcID), "source available after the pending hold")
	requireDecimalEqual(t, decimal.NewFromInt(500), postgrestestutil.GetBalanceOnHold(t, infra.pgContainer.DB, srcID), "source on-hold after the pending hold")
	requireDecimalEqual(t, decimal.Zero, postgrestestutil.GetBalanceAvailable(t, infra.pgContainer.DB, dstID), "destination available after the pending hold")
	requireDecimalEqual(t, decimal.Zero, postgrestestutil.GetBalanceOnHold(t, infra.pgContainer.DB, dstID), "destination on-hold after the pending hold")

	// Race: every goroutine parks on the same closed-channel barrier so they enter the cancel core
	// together rather than in spawn order.
	results := racePendingLifecycleOp(ctx, t, v2App, v2CancelURL(infra.orgID, infra.ledgerID, pendingID))

	winnerID := assertExactlyOneLifecycleWinner(t, results)
	assert.Equal(t, pendingID, winnerID, "the winning cancel must be the raced pending transaction, not some other id")

	drainBalanceSync(t, ctx, infra.handler.Command, infra.redisRepo, infra.orgID, infra.ledgerID)

	// CANCELED once, and the ledger still holds exactly the one transaction — cancel transitions the
	// pending transaction, it does not create a new one.
	assert.Equal(t, cn.CANCELED, postgrestestutil.GetTransactionStatus(t, infra.pgContainer.DB, pendingID), "the raced transaction must settle CANCELED")
	assert.Equal(t, 1, countTransactionsInLedger(t, infra.pgContainer.DB, infra.ledgerID), "a concurrent cancel must not persist a new transaction")

	// The hold was released EXACTLY ONCE: the source is restored to its full 1000 and the destination
	// is never credited. Exact equality, never a range — an over-release would read the source at 1500,
	// which reads as a hard failure here.
	requireDecimalEqual(t, decimal.NewFromInt(1000), postgrestestutil.GetBalanceAvailable(t, infra.pgContainer.DB, srcID), "source available restored to exactly 1000 after cancel (a double release would read 1500)")
	requireDecimalEqual(t, decimal.Zero, postgrestestutil.GetBalanceOnHold(t, infra.pgContainer.DB, srcID), "source on-hold released exactly once after cancel")
	requireDecimalEqual(t, decimal.Zero, postgrestestutil.GetBalanceAvailable(t, infra.pgContainer.DB, dstID), "destination never credited on cancel")
	requireDecimalEqual(t, decimal.Zero, postgrestestutil.GetBalanceOnHold(t, infra.pgContainer.DB, dstID), "destination on-hold after cancel")
}

func TestIntegration_TransactionV2CommitAndCancel_ConcurrentOppositesHaveOneEconomicWinner(t *testing.T) {
	t.Setenv("ALLOW_INSECURE_TLS", "true")
	t.Setenv("RABBITMQ_TRANSACTION_ASYNC", "false")

	infra := setupTestInfra(t)
	ctx := context.Background()
	srcID, dstID := seedTransfer(t, infra.pgContainer.DB, infra.orgID, infra.ledgerID, "@src", "@dst", 1000)
	v2App := buildHumaV2DirectApp(t, infra.handler)
	hold := decodeTxResponse(t, postV2Create(t, v2App, "hold", infra.orgID, infra.ledgerID,
		concurrentHoldV2Body, ""), nethttp.StatusCreated)
	pendingID := uuid.MustParse(hold["id"].(string))
	drainBalanceSync(t, ctx, infra.handler.Command, infra.redisRepo, infra.orgID, infra.ledgerID)

	start := make(chan struct{})
	results := make(chan revertRaceResult, 2)
	var ready sync.WaitGroup
	ready.Add(2)
	for _, endpoint := range []string{
		v2CommitURL(infra.orgID, infra.ledgerID, pendingID),
		v2CancelURL(infra.orgID, infra.ledgerID, pendingID),
	} {
		go func(url string) {
			ready.Done()
			<-start
			results <- fireRevert(v2App, url)
		}(endpoint)
	}
	ready.Wait()
	close(start)
	first, second := <-results, <-results

	winners := 0
	for _, result := range []revertRaceResult{first, second} {
		require.NoError(t, result.transportErr)
		if result.status == nethttp.StatusCreated {
			winners++
			continue
		}
		assert.Equal(t, nethttp.StatusConflict, result.status)
		assert.Equal(t, cn.ErrPendingTransactionLocked.Error(), result.problemCode)
	}
	require.Equal(t, 1, winners, "commit and cancel may produce exactly one terminal economic outcome")
	drainBalanceSync(t, ctx, infra.handler.Command, infra.redisRepo, infra.orgID, infra.ledgerID)

	status := postgrestestutil.GetTransactionStatus(t, infra.pgContainer.DB, pendingID)
	assert.Equal(t, 1, countTransactionsInLedger(t, infra.pgContainer.DB, infra.ledgerID))
	switch status {
	case cn.APPROVED:
		requireDecimalEqual(t, decimal.NewFromInt(500), postgrestestutil.GetBalanceAvailable(t, infra.pgContainer.DB, srcID),
			"commit winner moves the held amount once")
		requireDecimalEqual(t, decimal.Zero, postgrestestutil.GetBalanceOnHold(t, infra.pgContainer.DB, srcID),
			"commit winner releases the hold once")
		requireDecimalEqual(t, decimal.NewFromInt(500), postgrestestutil.GetBalanceAvailable(t, infra.pgContainer.DB, dstID),
			"commit winner credits the destination once")
	case cn.CANCELED:
		requireDecimalEqual(t, decimal.NewFromInt(1000), postgrestestutil.GetBalanceAvailable(t, infra.pgContainer.DB, srcID),
			"cancel winner restores the held amount once")
		requireDecimalEqual(t, decimal.Zero, postgrestestutil.GetBalanceOnHold(t, infra.pgContainer.DB, srcID),
			"cancel winner releases the hold once")
		requireDecimalEqual(t, decimal.Zero, postgrestestutil.GetBalanceAvailable(t, infra.pgContainer.DB, dstID),
			"cancel winner never credits the destination")
	default:
		require.Failf(t, "missing terminal winner", "status=%s", status)
	}
}
