// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

//go:build e2e

package e2e

import (
	"net/http"
	"testing"

	"github.com/google/uuid"
)

// fee_tracer_test.go covers Epic 2.4 of the cross-service test plan: the
// highest-risk cross-seam, FEE x TRACER amount interaction. The fee engine runs
// in transaction_create.go (applyFees, ~line 1130) BEFORE the reserve anchor
// (~line 1281), and the reserve observes transactionInput.Send.Value AFTER the
// fee engine has mutated it. The asymmetry under test:
//
//   - ADDITIVE (non-deductible) fee: the engine moves Send.Value UPWARD
//     (value + fee) so the recipient still receives the full transfer while the
//     sender pays more. The tracer therefore reserves against the POST-FEE
//     amount. A platform fee can thus push a transfer that was UNDER a
//     per-transaction limit OVER it.
//   - DEDUCTIBLE fee: Send.Value is UNCHANGED (the fee is netted out of the
//     recipient's leg, sender still debits the original value). The tracer
//     reserves against the ORIGINAL amount.
//
// All package-private names here carry the "ftrx" prefix so this file does not
// collide with the sibling Phase-2 "trx"/"feematrix" names in package e2e.
//
// These tests are tracer-WIRED: they SKIP cleanly via requireTracerWired when
// the ledger does not forward reserves to the tracer (the dev stack ships with
// TRACER_BASE_URL empty). The supervisor runs them live against a wired ledger.

// ftrxFundAbove funds an alias with headroom well above any limit/fee so the
// ONLY gate that can reject the transfer is the tracer reserve, never balance.
func ftrxFundAbove(t *testing.T, f fixture, alias string) {
	t.Helper()
	fund(t, f, alias, "100000")
}

// TestFeeTracer_AdditiveFeeCrossesLimit (Epic 2.4.A) pins that an ADDITIVE fee
// inflates the amount the tracer reserves against, so a transfer that is BELOW
// a per-transaction limit on its face value is DENIED once the post-fee value
// exceeds the cap.
//
// Setup: enforce(fail-open) ledger; source-scoped PER_TRANSACTION limit of 100;
// an enabled ADDITIVE flat-10 fee package.
//
//	Negative: transfer 95 -> post-fee Send.Value = 95 + 10 = 105 > 100 -> DENIED.
//	Positive: transfer 85 -> post-fee Send.Value = 85 + 10 = 95  <= 100 -> SUCCEEDS.
//
// The positive control rules out a limit/seeding artifact: a value whose
// post-fee total stays under the cap must pass, proving the denial is the fee
// pushing the reserved amount over, not the limit being mis-seeded.
func TestFeeTracer_AdditiveFeeCrossesLimit(t *testing.T) {
	requireTracerWired(t)

	f := newEnforceFixture(t, "open")

	src := createAccount(t, f, "ftrx-add-src-"+uuid.NewString()[:8])
	dst := createAccount(t, f, "ftrx-add-dst-"+uuid.NewString()[:8])
	feeAcct := createAccount(t, f, "ftrx-add-fee-"+uuid.NewString()[:8])

	ftrxFundAbove(t, f, src)

	// Cap the SOURCE account at 100 per transaction. The reserve sends
	// account.accountId = first source account, so a source-scoped limit matches
	// a plain JSON transfer (a transactionType scope would not).
	seedLimitRule(t, f, "100", map[string]any{"accountId": accountIDByAlias(t, f, src)})

	// Enabled ADDITIVE flat-10 fee crediting feeAcct (IsDeductibleFrom omitted =
	// false). Default min 0 / max 100000000 keeps both transfers fee-eligible.
	feematrixCreatePackage(t, f, feematrixPackageOpts{},
		feematrixFeeSpec{Key: "admin", Rule: "flatFee", CalcType: "flat", Value: "10", Priority: 1, CreditAccount: feeAcct})

	// Negative: 95 + 10 = 105 > 100 -> reserve DENIES before commit.
	deny := call(t, http.MethodPost, f.ledgers()+"/transactions/json", transferBody(src, dst, "95", nil))
	if deny.status >= 200 && deny.status < 300 {
		t.Fatalf("additive fee should push post-fee value 105 over the limit 100 (reserve must DENY), got 2xx %d\nbody: %s", deny.status, deny.body)
	}
	if deny.status != http.StatusUnprocessableEntity {
		t.Fatalf("additive over-limit denial: want 422 (0177 ErrTransactionReservationDenied), got %d\nbody: %s", deny.status, deny.body)
	}

	// Positive control: 85 + 10 = 95 <= 100 -> reserve PERMITS, transfer commits.
	ok := call(t, http.MethodPost, f.ledgers()+"/transactions/json", transferBody(src, dst, "85", nil))
	if ok.status != http.StatusCreated {
		t.Fatalf("post-fee value 95 is within the limit 100: want 201, got %d\nbody: %s", ok.status, ok.body)
	}
}

// TestFeeTracer_DeductibleFeeDoesNotInflateReserve (Epic 2.4.B) pins that a
// DEDUCTIBLE fee leaves Send.Value at the original amount, so the tracer
// reserves against the ORIGINAL value, not value+fee.
//
// Setup: enforce(fail-open) ledger; source-scoped PER_TRANSACTION limit of 100;
// a DEDUCTIBLE flat-10 fee package (minimumAmount >= the flat value, else
// creation 400s with code 0208 — see fees_matrix_test.go calibration).
//
//	Transfer 100 -> Send.Value stays 100 (fee netted out of the recipient leg)
//	-> reserve sees 100, exactly == the cap.
//
// The ==-boundary outcome is the open question: a "maximum" PER_TRANSACTION
// limit conventionally ALLOWS the boundary value (deny only when strictly over),
// so 100 == 100 is pinned as PERMITTED here. If an ADDITIVE fee were in play the
// reserve would instead see 110 and deny — that asymmetry is the whole point of
// this case. The exact ==-outcome is listed in liveVerifyNeeded; if the tracer
// treats the cap as exclusive the supervisor flips this to expect a denial.
func TestFeeTracer_DeductibleFeeDoesNotInflateReserve(t *testing.T) {
	requireTracerWired(t)

	f := newEnforceFixture(t, "open")

	src := createAccount(t, f, "ftrx-ded-src-"+uuid.NewString()[:8])
	dst := createAccount(t, f, "ftrx-ded-dst-"+uuid.NewString()[:8])
	feeAcct := createAccount(t, f, "ftrx-ded-fee-"+uuid.NewString()[:8])

	ftrxFundAbove(t, f, src)

	seedLimitRule(t, f, "100", map[string]any{"accountId": accountIDByAlias(t, f, src)})

	// DEDUCTIBLE flat-10 fee; minimumAmount "10" satisfies the per-fee guard
	// (0208) that a deductible flat value must be <= the package minimum.
	feematrixCreatePackage(t, f, feematrixPackageOpts{MinAmount: "10"},
		feematrixFeeSpec{Key: "admin", Rule: "flatFee", CalcType: "flat", Value: "10", Priority: 1, IsDeductibleFrom: true, CreditAccount: feeAcct})

	// Transfer exactly the cap. Deductible => Send.Value stays 100 (not 110), so
	// the reserve sees 100 == cap. A maximum limit allows the boundary -> 201.
	r := call(t, http.MethodPost, f.ledgers()+"/transactions/json", transferBody(src, dst, "100", nil))
	if r.status != http.StatusCreated {
		t.Fatalf("deductible fee must NOT inflate the reserved amount: transfer 100 with a 100 cap should reserve against 100 (== cap, allowed) and return 201, got %d — if this is a denial the deductible fee wrongly inflated the reserve OR the cap is exclusive (see liveVerifyNeeded)\nbody: %s", r.status, r.body)
	}

	// The fee was applied (recipient netted): confirm a fee credit leg exists, so
	// this is genuinely the deductible path and not a silently fee-free transfer.
	if !feematrixFeeApplied(t, r.json, feeAcct) {
		t.Fatalf("expected a deductible fee credit leg for %s on the committed transfer\nbody: %s", feeAcct, r.body)
	}
}
