// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

//go:build e2e

package e2e

import (
	"net/http"
	"testing"
	"time"
)

// Epic 3.5 — async transaction processing. When RABBITMQ_TRANSACTION_ASYNC=true
// the create handler enqueues the transaction and returns 201 immediately
// (transaction_create.go:1429 -> http.Created); the in-process consumer
// goroutine (bootstrap/service.go:66) settles balances afterwards. These tests
// prove eventual consistency: the API accepts synchronously, balances converge
// asynchronously, and the converged total matches plain sync arithmetic.
//
// Replays are deduped at the API layer BEFORE enqueue
// (transaction_create.go:1037-1060: CreateOrCheckTransactionIdempotency returns
// the first result on replay), so a re-POST under the same idempotency key never
// produces a second settlement. The harness call() mints a fresh X-Request-Id
// per request, so each transfer here is a distinct enqueue, not a replay.

// asyncRequireAsync gates on the operator's intent flag E2E_ASYNC=1 rather than
// a behavioral probe. In sync mode a poll-with-timeout converges on the first
// read and proves nothing about async settlement; only the operator knows the
// stack was recreated with RABBITMQ_TRANSACTION_ASYNC=true, so we trust that
// signal instead of guessing from observed latency.
func asyncRequireAsync(t *testing.T) {
	t.Helper()

	if envOr("E2E_ASYNC", "") != "1" {
		t.Skipf("async settlement tests require an async-mode stack — set E2E_ASYNC=1 after recreating the ledger with RABBITMQ_TRANSACTION_ASYNC=true")
	}
}

// asyncSettleTimeout bounds how long a single balance is allowed to converge.
const asyncSettleTimeout = 10 * time.Second

// asyncPollInterval is the gap between balance reads while waiting to converge.
const asyncPollInterval = 50 * time.Millisecond

// asyncPollBalance polls the available balance for alias until it equals want or
// the timeout elapses. In async mode fund()/transfers return 201 before the
// consumer settles, so callers MUST poll for the expected amount before reading
// it as ground truth (a bare read races the consumer). On non-convergence it
// fails with the last observed value so a stuck consumer is obvious from the log.
func asyncPollBalance(t *testing.T, f fixture, alias string, want int64, timeout time.Duration) {
	t.Helper()

	deadline := time.Now().Add(timeout)

	var last int64
	for {
		last = atoiDecimal(t, availableBalance(t, f, alias))
		if last == want {
			return
		}

		if time.Now().After(deadline) {
			t.Fatalf("async balance for %q did not converge to %d within %s; last observed %d", alias, want, timeout, last)
		}

		time.Sleep(asyncPollInterval)
	}
}

// asyncTransferStatus extracts the nested status code from a created-transaction
// response. The response model serializes status as {"status": {"code": "..."}}
// (postgres/transaction/transaction.go:132,63), not a flat string field.
func asyncTransferStatus(t *testing.T, r response) string {
	t.Helper()

	st, ok := r.json["status"].(map[string]any)
	if !ok {
		t.Fatalf("transfer response missing 'status' object: %s", r.body)
	}

	return str(t, st, "code")
}

// TestAsyncTransferSettles proves a single transfer eventually settles: the POST
// returns 201 synchronously with a non-pending status, then both legs converge
// to their post-transfer balances via the consumer.
func TestAsyncTransferSettles(t *testing.T) {
	requireStack(t)
	asyncRequireAsync(t)

	f := newFixture(t, false)
	createAccount(t, f, "async-src-1")
	createAccount(t, f, "async-dst-1")

	// fund() asserts 201 but the inflow settles asynchronously; poll before
	// transferring so the source actually holds the funds.
	fund(t, f, "async-src-1", "1000")
	asyncPollBalance(t, f, "async-src-1", 1000, asyncSettleTimeout)

	r := call(t, http.MethodPost, f.ledgers()+"/transactions/json", transferBody("async-src-1", "async-dst-1", "100", nil))
	if r.status != http.StatusCreated {
		t.Fatalf("async transfer: want 201, got %d\nbody: %s", r.status, r.body)
	}

	// A non-pending transfer is admitted as CREATED or APPROVED
	// (pkg/constant/transaction.go:8-9); both are valid synchronous-admission
	// states for a fire-and-settle write.
	if code := asyncTransferStatus(t, r); code != "CREATED" && code != "APPROVED" {
		t.Fatalf("async transfer status: want CREATED or APPROVED, got %q\nbody: %s", code, r.body)
	}

	asyncPollBalance(t, f, "async-src-1", 900, asyncSettleTimeout)
	asyncPollBalance(t, f, "async-dst-1", 100, asyncSettleTimeout)
}

// TestAsyncSequentialOrdering issues two transfers in sequence on one source and
// asserts both apply cumulatively. Each POST is a distinct enqueue (fresh
// request id), so the consumer must apply both, not collapse them.
func TestAsyncSequentialOrdering(t *testing.T) {
	requireStack(t)
	asyncRequireAsync(t)

	f := newFixture(t, false)
	createAccount(t, f, "async-src-2")
	createAccount(t, f, "async-dst-2")

	fund(t, f, "async-src-2", "1000")
	asyncPollBalance(t, f, "async-src-2", 1000, asyncSettleTimeout)

	mustCreate(t, f.ledgers()+"/transactions/json", transferBody("async-src-2", "async-dst-2", "100", nil))
	mustCreate(t, f.ledgers()+"/transactions/json", transferBody("async-src-2", "async-dst-2", "50", nil))

	// Both must settle: 1000-100-50 on the source, 100+50 on the destination.
	asyncPollBalance(t, f, "async-src-2", 850, asyncSettleTimeout)
	asyncPollBalance(t, f, "async-dst-2", 150, asyncSettleTimeout)
}

// TestAsyncSyncEquivalence asserts the converged balances equal plain sync
// arithmetic — no operation lost (under-applied) and none doubled
// (over-applied). The idempotency dedup at the API layer
// (transaction_create.go:1037-1060) is what makes doubling impossible; this
// pins that the async path settles each enqueued transfer exactly once.
func TestAsyncSyncEquivalence(t *testing.T) {
	requireStack(t)
	asyncRequireAsync(t)

	f := newFixture(t, false)
	createAccount(t, f, "async-src-3")
	createAccount(t, f, "async-dst-3")

	const funded = int64(1000)
	const t1 = int64(300)
	const t2 = int64(125)

	fund(t, f, "async-src-3", "1000")
	asyncPollBalance(t, f, "async-src-3", funded, asyncSettleTimeout)

	mustCreate(t, f.ledgers()+"/transactions/json", transferBody("async-src-3", "async-dst-3", "300", nil))
	mustCreate(t, f.ledgers()+"/transactions/json", transferBody("async-src-3", "async-dst-3", "125", nil))

	wantSrc := funded - t1 - t2
	wantDst := t1 + t2

	asyncPollBalance(t, f, "async-src-3", wantSrc, asyncSettleTimeout)
	asyncPollBalance(t, f, "async-dst-3", wantDst, asyncSettleTimeout)

	// Conservation: the two legs sum back to the funded total (zero leakage).
	gotSrc := atoiDecimal(t, availableBalance(t, f, "async-src-3"))
	gotDst := atoiDecimal(t, availableBalance(t, f, "async-dst-3"))
	if gotSrc+gotDst != funded {
		t.Fatalf("async conservation violated: src %d + dst %d != funded %d", gotSrc, gotDst, funded)
	}
}
