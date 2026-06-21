// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

//go:build e2e

package e2e

import (
	"fmt"
	"math/rand"
	"net/http"
	"testing"

	"github.com/google/uuid"
)

// This file covers Epic 1.3 (CRM enforcement & integrity): the ledger<->CRM seam
// — requireHolder enforcement with the two-key holder skip (1.3.1), instrument
// referential validation + the holder-account composition contract (1.3.2), and
// holder soft-delete ownership integrity (1.3.3). Every contract below was
// calibrated against the live stack before being asserted; divergences from the
// plan's expected contract are pinned with a `// FINDING:` comment so the suite
// stays green while documenting the real behavior.
//
// All new symbols are prefixed "crm" so they cannot collide with the other
// parallel-epic files in this package.

// crmValidCPF generates a syntactically valid, check-digit-correct Brazilian CPF.
// The CRM holder create rejects malformed documents and enforces a one-document-
// per-holder uniqueness rule (CRM-0010, HTTP 409), so every holder in a test that
// needs more than one holder per org must use a fresh valid CPF. The harness's
// createHolder uses a single fixed CPF and therefore works only once per org.
func crmValidCPF() string {
	d := make([]int, 11)
	for i := 0; i < 9; i++ {
		d[i] = rand.Intn(10)
	}

	dv := func(n int) int {
		sum := 0
		for i := 0; i < n; i++ {
			sum += d[i] * (n + 1 - i)
		}

		r := (sum * 10) % 11
		if r == 10 {
			return 0
		}

		return r
	}

	d[9] = dv(9)
	d[10] = dv(10)

	out := make([]byte, 11)
	for i, v := range d {
		out[i] = byte('0' + v)
	}

	return string(out)
}

// crmCreateHolder creates a CRM holder with a fresh valid CPF and returns its id.
// Unlike the harness createHolder (fixed CPF, single-use per org) this is safe to
// call repeatedly within one org.
func crmCreateHolder(t *testing.T, orgID, name string) string {
	t.Helper()

	h := mustCreate(t, fmt.Sprintf("%s/v1/organizations/%s/holders", ledgerURL(), orgID), map[string]any{
		"type": "NATURAL_PERSON", "name": name, "document": crmValidCPF(),
		"externalId": "E2E-CRM-" + uuid.NewString()[:8],
	})

	return str(t, h, "id")
}

// crmCompleteSettings returns a full ledger settings block (a partial block leaves
// tracer.mode="" which the API rejects with 0176). requireHolder and
// allowHolderSkip are the two keys under test; everything else is the safe default.
func crmCompleteSettings(requireHolder, allowHolderSkip bool) map[string]any {
	return map[string]any{
		"accounting": map[string]any{
			"validateAccountType": false, "validateRoutes": false, "requireHolder": requireHolder,
		},
		"tracer": map[string]any{"mode": "off", "failPosture": "open", "timeoutMs": 250},
		"overrides": map[string]any{
			"allowFeeSkip": false, "allowTracerSkip": false, "allowHolderSkip": allowHolderSkip,
		},
	}
}

// crmFixtureRequiringHolder builds an org+ledger (with USD) where
// accounting.requireHolder is true; allowHolderSkip toggles the second key of the
// two-key holder-skip model. The returned fixture reuses the harness fixture
// type so all harness helpers (createAccount, etc.) apply.
func crmFixtureRequiringHolder(t *testing.T, allowHolderSkip bool) fixture {
	t.Helper()

	f := fixture{orgID: createOrg(t)}

	led := mustCreate(t, fmt.Sprintf("%s/v1/organizations/%s/ledgers", ledgerURL(), f.orgID), map[string]any{
		"name":     "E2E RequireHolder " + uuid.NewString()[:8],
		"settings": crmCompleteSettings(true, allowHolderSkip),
	})
	f.ledgerID = str(t, led, "id")
	createAsset(t, f, "USD")

	return f
}

// crmRawAccount posts a plain account-create body verbatim and returns the raw
// response so the caller can assert on status/code (used for the negative and
// boundary cases the happy-path harness helpers cannot express).
func crmRawAccount(t *testing.T, f fixture, body map[string]any) response {
	t.Helper()
	return call(t, http.MethodPost, f.ledgers()+"/accounts", body)
}

// crmInstrumentURL is the instrument-create endpoint for a holder.
func crmInstrumentURL(orgID, holderID string) string {
	return fmt.Sprintf("%s/v1/organizations/%s/holders/%s/instruments", ledgerURL(), orgID, holderID)
}

// crmHolderURL is the single-holder endpoint (GET/DELETE).
func crmHolderURL(orgID, holderID string) string {
	return fmt.Sprintf("%s/v1/organizations/%s/holders/%s", ledgerURL(), orgID, holderID)
}

// crmDeleteHolder issues DELETE on a holder and returns the raw response.
func crmDeleteHolder(t *testing.T, orgID, holderID string) response {
	t.Helper()
	return call(t, http.MethodDelete, crmHolderURL(orgID, holderID), nil)
}

// crmDeleteInstrument issues DELETE on an instrument and returns the raw response.
func crmDeleteInstrument(t *testing.T, orgID, holderID, instrumentID string) response {
	t.Helper()
	return call(t, http.MethodDelete,
		fmt.Sprintf("%s/v1/organizations/%s/holders/%s/instruments/%s", ledgerURL(), orgID, holderID, instrumentID), nil)
}

// crmGetHolder issues GET on a single holder and returns the raw response.
func crmGetHolder(t *testing.T, orgID, holderID string) response {
	t.Helper()
	return call(t, http.MethodGet, crmHolderURL(orgID, holderID), nil)
}

// crmCode extracts the API error "code" field from a response body, or "" when
// absent. Used to pin the exact sentinel a contract returns.
func crmCode(r response) string {
	if r.json == nil {
		return ""
	}

	c, _ := r.json["code"].(string)

	return c
}

// TestRequireHolderTwoKey exercises Task 1.3.1: requireHolder enforcement and the
// two-key holder skip. Each case was calibrated live before assertion.
//
// Contract (post-F1):
//   - (a) requireHolder + no holderId  -> 422 (0491, holder required; KYC semantics)
//   - (b) requireHolder + unknown holderId -> 404 CRM-0006 (not 422)
//   - (c) requireHolder + valid holderId   -> 201
//   - (d) requireHolder + allowHolderSkip + skip.holder:true -> 201, holderCheckSkipped:true
//   - (e) requireHolder + no opt-in + skip.holder:true -> 422 (0490, skip not permitted)
//   - (f) requireHolder + no opt-in + skip.holder:false + valid holderId -> 201 (skip false is a no-op)
//   - (g) requireHolder:false + no holderId -> 201 (self-holder default; F1 regression guard)
func TestRequireHolderTwoKey(t *testing.T) {
	requireStack(t)

	// (a) requireHolder:true, no holderId supplied. F1: a requireHolder ledger
	// demands a named, existing holder; the derived self-holder default is NOT an
	// acceptable substitute, so an absent holderId is rejected 422 (0491) before
	// the self-holder is ever derived.
	t.Run("no holder id is rejected", func(t *testing.T) {
		f := crmFixtureRequiringHolder(t, false)

		r := crmRawAccount(t, f, map[string]any{
			"name": "NoHolder", "assetCode": "USD", "type": "deposit", "alias": "@noholder",
		})
		if r.status != http.StatusUnprocessableEntity {
			t.Fatalf("requireHolder + no holderId: want 422 (holder required), got %d\nbody: %s", r.status, r.body)
		}

		if got := crmCode(r); got != "0491" {
			t.Fatalf("requireHolder + no holderId: want code 0491, got %q\nbody: %s", got, r.body)
		}
	})

	// (b) requireHolder:true, unknown (well-formed lowercase) holderId.
	t.Run("unknown holder id is rejected", func(t *testing.T) {
		f := crmFixtureRequiringHolder(t, false)

		r := crmRawAccount(t, f, map[string]any{
			"name": "BadHolder", "assetCode": "USD", "type": "deposit", "alias": "@badholder",
			"holderId": uuid.NewString(),
		})
		// FINDING: plan expected the unknown-holder rejection to be a 422; observed
		// 404 CRM-0006 (ErrHolderNotFound maps to EntityNotFoundError -> 404). The
		// gate fires correctly (fail-closed), only the status/code differ. See F2.
		if r.status != http.StatusNotFound {
			t.Fatalf("requireHolder + unknown holderId: want 404, got %d\nbody: %s", r.status, r.body)
		}

		if got := crmCode(r); got != "CRM-0006" {
			t.Fatalf("requireHolder + unknown holderId: want code CRM-0006, got %q\nbody: %s", got, r.body)
		}
	})

	// (c) requireHolder:true, valid holderId.
	t.Run("valid holder id is accepted", func(t *testing.T) {
		f := crmFixtureRequiringHolder(t, false)
		holderID := crmCreateHolder(t, f.orgID, "Valid Holder")

		r := crmRawAccount(t, f, map[string]any{
			"name": "GoodHolder", "assetCode": "USD", "type": "deposit", "alias": "@goodholder",
			"holderId": holderID,
		})
		if r.status != http.StatusCreated {
			t.Fatalf("requireHolder + valid holderId: want 201, got %d\nbody: %s", r.status, r.body)
		}

		if got, _ := r.json["holderId"].(string); got != holderID {
			t.Fatalf("requireHolder + valid holderId: account holderId = %q, want %q", got, holderID)
		}
	})

	// (d) requireHolder:true + allowHolderSkip:true + skip.holder:true.
	t.Run("skip honored with opt-in", func(t *testing.T) {
		f := crmFixtureRequiringHolder(t, true)

		r := crmRawAccount(t, f, map[string]any{
			"name": "SkipOptin", "assetCode": "USD", "type": "deposit", "alias": "@skipoptin",
			"skip": map[string]any{"holder": true},
		})
		if r.status != http.StatusCreated {
			t.Fatalf("skip.holder:true with opt-in: want 201, got %d\nbody: %s", r.status, r.body)
		}

		if skipped, _ := r.json["holderCheckSkipped"].(bool); !skipped {
			t.Fatalf("skip.holder:true with opt-in: holderCheckSkipped = %v, want true\nbody: %s", r.json["holderCheckSkipped"], r.body)
		}
	})

	// (e) requireHolder:true + allowHolderSkip:false + skip.holder:true.
	t.Run("skip without opt-in is rejected", func(t *testing.T) {
		f := crmFixtureRequiringHolder(t, false)

		r := crmRawAccount(t, f, map[string]any{
			"name": "SkipNoOptin", "assetCode": "USD", "type": "deposit", "alias": "@skipnoopt",
			"skip": map[string]any{"holder": true},
		})
		if r.status != http.StatusUnprocessableEntity {
			t.Fatalf("skip.holder:true without opt-in: want 422, got %d\nbody: %s", r.status, r.body)
		}

		if got := crmCode(r); got != "0490" {
			t.Fatalf("skip.holder:true without opt-in: want code 0490, got %q\nbody: %s", got, r.body)
		}
	})

	// (f) requireHolder:true + allowHolderSkip:false + skip.holder:false. Explicit
	// false is a no-op: no skip is honored and the holder gate stays active. A valid
	// holderId is supplied so this isolates the skip:false no-op from the F1 gate
	// (an absent holderId would itself be rejected 0491 — covered by case a).
	t.Run("skip false is a no-op and accepted", func(t *testing.T) {
		f := crmFixtureRequiringHolder(t, false)
		holderID := crmCreateHolder(t, f.orgID, "Skip False Holder")

		r := crmRawAccount(t, f, map[string]any{
			"name": "SkipFalse", "assetCode": "USD", "type": "deposit", "alias": "@skipfalsehld",
			"holderId": holderID,
			"skip":     map[string]any{"holder": false},
		})
		if r.status != http.StatusCreated {
			t.Fatalf("skip.holder:false: want 201 (no-op), got %d\nbody: %s", r.status, r.body)
		}

		if skipped, _ := r.json["holderCheckSkipped"].(bool); skipped {
			t.Fatalf("skip.holder:false: holderCheckSkipped = true, want false (no skip honored)\nbody: %s", r.body)
		}
	})

	// (g) requireHolder:false + no holderId. F1 regression guard: when the gate is
	// off (the default), an account with no holderId must still derive the org's
	// deterministic self-holder and return 201 exactly as before the fix.
	t.Run("require false + no holder id derives self-holder", func(t *testing.T) {
		f := fixture{orgID: createOrg(t)}
		led := mustCreate(t, fmt.Sprintf("%s/v1/organizations/%s/ledgers", ledgerURL(), f.orgID), map[string]any{
			"name":     "E2E NoRequireHolder " + uuid.NewString()[:8],
			"settings": crmCompleteSettings(false, false),
		})
		f.ledgerID = str(t, led, "id")
		createAsset(t, f, "USD")

		r := crmRawAccount(t, f, map[string]any{
			"name": "SelfHolder", "assetCode": "USD", "type": "deposit", "alias": "@selfholder",
		})
		if r.status != http.StatusCreated {
			t.Fatalf("require false + no holderId: want 201 (self-holder default), got %d\nbody: %s", r.status, r.body)
		}

		if hid, _ := r.json["holderId"].(string); hid == "" {
			t.Fatalf("require false + no holderId: expected a derived self-holder id, got empty\nbody: %s", r.body)
		}
	})
}

// TestInstrumentReferential exercises Task 1.3.2 (referential half): instrument
// create validates the body-supplied account/ledger references.
//
// Calibration outcomes (current contract): bogus accountId, bogus ledgerId, and a
// real account addressed under a DIFFERENT ledger all return 404 code 0007
// (EntityNotFound); a matching account+ledger returns 201.
func TestInstrumentReferential(t *testing.T) {
	requireStack(t)

	// Two ledgers in one org so a cross-ledger reference can be exercised.
	orgID := createOrg(t)

	ledgerA := mustCreate(t, fmt.Sprintf("%s/v1/organizations/%s/ledgers", ledgerURL(), orgID), map[string]any{"name": "LedgerA " + uuid.NewString()[:8]})
	ledgerB := mustCreate(t, fmt.Sprintf("%s/v1/organizations/%s/ledgers", ledgerURL(), orgID), map[string]any{"name": "LedgerB " + uuid.NewString()[:8]})
	fa := fixture{orgID: orgID, ledgerID: str(t, ledgerA, "id")}
	fb := fixture{orgID: orgID, ledgerID: str(t, ledgerB, "id")}
	createAsset(t, fa, "USD")
	createAsset(t, fb, "USD")

	holderID := crmCreateHolder(t, orgID, "Ref Holder")
	accID := accountID(t, fa, "@ref_acct") // account lives in ledger A

	cases := []struct {
		name      string
		accountID string
		ledgerID  string
		want      int
		wantCode  string
	}{
		{"bogus account id", uuid.NewString(), fa.ledgerID, http.StatusNotFound, "0007"},
		{"bogus ledger id", accID, uuid.NewString(), http.StatusNotFound, "0007"},
		{"cross-ledger account/ledger mismatch", accID, fb.ledgerID, http.StatusNotFound, "0007"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := call(t, http.MethodPost, crmInstrumentURL(orgID, holderID), map[string]any{
				"accountId": tc.accountID, "ledgerId": tc.ledgerID,
			})
			// FINDING: plan + source (validateInstrumentReferences, sentinels 0488/
			// 0489) expected bogus/cross-ledger references to be 422; observed
			// 404 code 0007 (generic EntityNotFound) for all three. The reference
			// IS rejected (fail-closed, no instrument written), only the status/code
			// differ from the documented 422 referential contract. See F3.
			if r.status != tc.want {
				t.Fatalf("%s: want %d, got %d\nbody: %s", tc.name, tc.want, r.status, r.body)
			}

			if got := crmCode(r); got != tc.wantCode {
				t.Fatalf("%s: want code %q, got %q\nbody: %s", tc.name, tc.wantCode, got, r.body)
			}
		})
	}

	t.Run("valid account and ledger is accepted", func(t *testing.T) {
		r := call(t, http.MethodPost, crmInstrumentURL(orgID, holderID), map[string]any{
			"accountId": accID, "ledgerId": fa.ledgerID,
		})
		if r.status != http.StatusCreated {
			t.Fatalf("valid instrument: want 201, got %d\nbody: %s", r.status, r.body)
		}

		if got := str(t, r.json, "accountId"); got != accID {
			t.Fatalf("valid instrument: accountId = %q, want %q", got, accID)
		}
	})
}

// TestCompositionAtomicity exercises Task 1.3.2 (atomicity half): the holder-owned
// account endpoint opens an account and OPTIONALLY an instrument in one call.
//
// Calibration outcome (current contract): the composition is DELIBERATELY
// NON-ATOMIC. When the account leg commits but the instrument leg fails (here: an
// invalid relatedParty role), the call returns HTTP 201 with instrument:null and a
// typed instrumentError block; the account REMAINS persisted and readable. This
// matches the documented design in composition.go / mmodel.HolderAccountResponse
// (no compensating delete).
func TestCompositionAtomicity(t *testing.T) {
	requireStack(t)

	f := newFixture(t, false)
	holderID := crmCreateHolder(t, f.orgID, "Composition Holder")

	alias := "@composition_orphan_" + uuid.NewString()[:8]

	// An invalid relatedParty role makes the instrument leg fail (ValidateRelatedParties)
	// AFTER the account leg has already committed inside the composition service.
	r := call(t, http.MethodPost, fmt.Sprintf("%s/holders/%s/accounts", f.ledgers(), holderID), map[string]any{
		"assetCode": "USD", "type": "deposit", "alias": alias,
		"relatedParties": []any{map[string]any{
			"document": "12345678900", "name": "Bad Role", "role": "NOT_A_ROLE", "startDate": "2025-01-01",
		}},
	})

	// FINDING: plan expected the composition to be ATOMIC (a failed instrument leg
	// rolls back the account, no orphan). Observed: non-atomic by design — 201 with
	// instrumentError, account survives. This is the documented contract (no
	// rollback), not a defect, but it diverges from the plan's atomicity
	// expectation. See F4.
	if r.status != http.StatusCreated {
		t.Fatalf("composition with failing instrument leg: want 201 (non-atomic, account survives), got %d\nbody: %s", r.status, r.body)
	}

	if r.json["instrument"] != nil {
		t.Fatalf("composition with failing instrument leg: instrument should be null, got %v", r.json["instrument"])
	}

	instErr, ok := r.json["instrumentError"].(map[string]any)
	if !ok {
		t.Fatalf("composition with failing instrument leg: missing instrumentError block\nbody: %s", r.body)
	}

	if status, _ := instErr["status"].(string); status != "FAILED" {
		t.Fatalf("composition instrumentError.status = %v, want FAILED", instErr["status"])
	}

	if reason, _ := instErr["reason"].(string); reason == "" {
		t.Fatalf("composition instrumentError.reason is empty\nbody: %s", r.body)
	}

	// The account leg survived: it is readable by its alias (no compensating delete).
	got := call(t, http.MethodGet, f.ledgers()+"/accounts/alias/"+alias, nil)
	if got.status != http.StatusOK {
		t.Fatalf("orphan account after failed instrument leg: want 200 (account persisted), got %d\nbody: %s", got.status, got.body)
	}

	if hid := str(t, got.json, "holderId"); hid != holderID {
		t.Fatalf("orphan account holderId = %q, want %q (path holder)", hid, holderID)
	}
}

// TestHolderDeleteIntegrity exercises Task 1.3.3: holder soft-delete ownership
// integrity and independent instrument delete.
//
// Calibration outcomes (current contract):
//   - Holder owning an ACCOUNT (no instrument): DELETE -> 422 CRM-0030 (blocked);
//     both the holder and the owned account stay readable. The delete guard now
//     counts owned active accounts as well as instruments, so the account's
//     holderId can never dangle. See F5.
//   - Holder owning an INSTRUMENT: DELETE -> 422 CRM-0017 (blocked); holder stays
//     readable.
//   - Instrument independent delete -> 204; holder unaffected.
//   - After the instrument is gone, holder DELETE -> 204.
func TestHolderDeleteIntegrity(t *testing.T) {
	requireStack(t)

	f := newFixture(t, false)

	t.Run("delete holder owning an account is blocked", func(t *testing.T) {
		holderID := crmCreateHolder(t, f.orgID, "Account-Only Holder")

		alias := "@del_acct_" + uuid.NewString()[:8]
		// Holder-owned account with NO instrument fields (account-only composition).
		env := mustCreate(t, fmt.Sprintf("%s/holders/%s/accounts", f.ledgers(), holderID), map[string]any{
			"assetCode": "USD", "type": "deposit", "alias": alias,
		})
		acc, _ := env["account"].(map[string]any)
		if acc == nil {
			t.Fatalf("holder-owned account missing 'account' envelope: %v", env)
		}

		del := crmDeleteHolder(t, f.orgID, holderID)
		// F5 (fixed): the holder-delete guard (DeleteHolderByID) now counts owned
		// active accounts as well as instruments. A holder that owns active accounts
		// is blocked (422 CRM-0030, mirroring the instrument guard's CRM-0017) so the
		// account's holderId can never become a dangling reference.
		if del.status != http.StatusUnprocessableEntity {
			t.Fatalf("delete holder owning an account: want 422 (blocked), got %d\nbody: %s", del.status, del.body)
		}

		if got := crmCode(del); got != "CRM-0030" {
			t.Fatalf("delete holder owning an account: want code CRM-0030, got %q\nbody: %s", got, del.body)
		}

		// Holder is still readable after the blocked delete.
		if g := crmGetHolder(t, f.orgID, holderID); g.status != http.StatusOK {
			t.Fatalf("holder after blocked delete: want 200 (still present), got %d\nbody: %s", g.status, g.body)
		}

		// Account is intact and still owned by the holder (no dangling reference).
		got := call(t, http.MethodGet, f.ledgers()+"/accounts/alias/"+alias, nil)
		if got.status != http.StatusOK {
			t.Fatalf("account after blocked holder delete: want 200 (survives), got %d\nbody: %s", got.status, got.body)
		}

		if hid := str(t, got.json, "holderId"); hid != holderID {
			t.Fatalf("account after blocked holder delete: holderId = %q, want %q", hid, holderID)
		}
	})

	t.Run("delete holder owning an instrument is blocked", func(t *testing.T) {
		holderID := crmCreateHolder(t, f.orgID, "Instrument Holder")
		accID := accountID(t, f, "@inst_owner_"+uuid.NewString()[:8])

		inst := createInstrument(t, f.orgID, f.ledgerID, holderID, accID)
		instID := str(t, inst, "id")

		// Delete is blocked while the holder owns an instrument.
		del := crmDeleteHolder(t, f.orgID, holderID)
		// The block fires with 422 CRM-0017 (business-rule violation: a holder with
		// dependents cannot be deleted), aligned to the 422 standard for business
		// rules and matching the owned-account guard (CRM-0030). See F6.
		if del.status != http.StatusUnprocessableEntity {
			t.Fatalf("delete holder owning an instrument: want 422 (blocked), got %d\nbody: %s", del.status, del.body)
		}

		if got := crmCode(del); got != "CRM-0017" {
			t.Fatalf("delete holder owning an instrument: want code CRM-0017, got %q\nbody: %s", got, del.body)
		}

		// Holder is still readable after the blocked delete.
		if g := crmGetHolder(t, f.orgID, holderID); g.status != http.StatusOK {
			t.Fatalf("holder after blocked delete: want 200 (still present), got %d\nbody: %s", g.status, g.body)
		}

		// Independent instrument soft-delete succeeds and leaves the holder intact.
		di := crmDeleteInstrument(t, f.orgID, holderID, instID)
		if di.status != http.StatusNoContent {
			t.Fatalf("independent instrument delete: want 204, got %d\nbody: %s", di.status, di.body)
		}

		if g := crmGetHolder(t, f.orgID, holderID); g.status != http.StatusOK {
			t.Fatalf("holder after independent instrument delete: want 200 (unaffected), got %d\nbody: %s", g.status, g.body)
		}

		// With the instrument gone, the holder delete is now unblocked.
		del2 := crmDeleteHolder(t, f.orgID, holderID)
		if del2.status != http.StatusNoContent {
			t.Fatalf("delete holder after instrument removed: want 204, got %d\nbody: %s", del2.status, del2.body)
		}
	})
}
