// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"net/http"
	"testing"

	libCommons "github.com/LerianStudio/lib-commons/v7/commons"
	libConstants "github.com/LerianStudio/lib-commons/v7/commons/constants"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	cn "github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/utils"
)

// A v2 create names its organization and ledger on every leg of the body, and the endpoint it is
// posted to names neither. These tests pin WHICH pair the funnel is entered with, and they read it
// off the idempotency slot key — utils.IdempotencyInternalKey(organization, ledger, key) — because
// that key is the first observable value the funnel builds from the resolved scope. The probe is
// the same TransactionRedisRepo.SetNX touch the sibling idempotency tests use.
//
// Not parallel: buildHumaV2ActionApp mutates process-global huma state (see its header).

// v2BodyScopeOrg and v2BodyScopeLedger are the parsed form of the scope every shared v2 test body
// names, which is the pair a request carrying those bodies must be funnelled under.
var (
	v2BodyScopeOrg    = uuid.MustParse(v2ScopeOrgID)
	v2BodyScopeLedger = uuid.MustParse(v2ScopeLedgerID)
)

// v2ForeignScopeJSON spells a DIFFERENT organization and ledger for a body, so two requests can
// differ in nothing but the scope they name.
const (
	v2ForeignOrgID     = "99999999-9999-9999-9999-999999999999"
	v2ForeignLedgerID  = "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa"
	v2ForeignScopeJSON = `"organizationId":"` + v2ForeignOrgID + `","ledgerId":"` + v2ForeignLedgerID + `"`
)

// v2ForeignScopeBody is v2DirectBody moved wholesale into the foreign scope: same asset, same
// amount, same aliases, different organization and ledger.
const v2ForeignScopeBody = `{"description":"v2 direct","asset":"BRL","amount":"100",` +
	`"debits":[{"alias":"@src",` + v2ForeignScopeJSON + `,"amount":"100"}],` +
	`"credits":[{"alias":"@dst",` + v2ForeignScopeJSON + `,"amount":"100"}]}`

// captureV2CreateSlotKey posts body to the named v2 create action and returns the idempotency slot
// key the funnel computed. idempotencyKey is sent as X-Idempotency when non-empty.
func captureV2CreateSlotKey(t *testing.T, action, body, idempotencyKey string) string {
	t.Helper()

	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	var gotKey string

	handler := captureSetNXKey(t, ctrl, &gotKey, "{}")
	app := buildHumaV2ActionApp(t, action, v2CreateOpFor(t, handler, action))

	resp := postActionV2WithIdempotency(t, app, action, body, idempotencyKey)
	defer func() { _ = resp.Body.Close() }()

	require.Equal(t, http.StatusCreated, resp.StatusCode,
		"the request must reach the idempotency claim; a losing claim with a cached value replays to 201")

	return gotKey
}

// v2CreateOpFor resolves the terminal of a named create action off the shared action table, so a
// test naming an action cannot silently probe a different one.
func v2CreateOpFor(t *testing.T, handler *TransactionHandler, action string) humaV2CreateOp {
	t.Helper()

	for _, tc := range v2CreateActionCases() {
		if tc.route == action {
			return tc.op(handler)
		}
	}

	t.Fatalf("no v2 create action named %q", action)

	return nil
}

// TestCreateTransactionV2_FunnelIsScopedByTheBody proves every v2 create action enters the funnel
// under the organization and ledger the BODY names. The slot key is asserted whole — the exact
// utils.IdempotencyInternalKey the funnel must have built — so neither half of the pair can be
// wrong, dropped, or swapped without turning this red.
func TestCreateTransactionV2_FunnelIsScopedByTheBody(t *testing.T) {
	for _, tc := range v2CreateActionCases() {
		t.Run(tc.name, func(t *testing.T) {
			// NOT parallel: process-global huma state.
			gotKey := captureV2CreateSlotKey(t, tc.route, v2DirectBody, "")

			wantHashSource := v2DirectBody
			if tc.wantDisc != "" {
				wantHashSource = tc.wantDisc + "\x00" + v2DirectBody
			}

			wantKey := utils.IdempotencyInternalKey(v2BodyScopeOrg, v2BodyScopeLedger,
				libCommons.HashSHA256(wantHashSource))

			assert.Equal(t, wantKey, gotKey,
				"the %s action must enter the funnel under the organization and ledger its body names", tc.name)
		})
	}
}

// TestCreateTransactionV2_IdempotencySlotsAreSeparatedByBodyScope proves the slot stays scoped:
// two requests that differ in NOTHING but the organization and ledger their body names must claim
// different slots. Moving the scope from the URL into the body is only safe while this holds — a
// shared slot would let one organization's retry replay another organization's transaction.
//
// It also proves the separation survives a caller-supplied idempotency key, which replaces the
// body hash in the slot key and would otherwise make the two requests indistinguishable.
func TestCreateTransactionV2_IdempotencySlotsAreSeparatedByBodyScope(t *testing.T) {
	tests := []struct {
		name           string
		idempotencyKey string
	}{
		{name: "no caller key: the slot key falls back to the body hash", idempotencyKey: ""},
		{name: "one caller key reused across scopes", idempotencyKey: "shared-caller-key"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// NOT parallel: process-global huma state.
			homeKey := captureV2CreateSlotKey(t, "direct", v2DirectBody, tt.idempotencyKey)
			foreignKey := captureV2CreateSlotKey(t, "direct", v2ForeignScopeBody, tt.idempotencyKey)

			assert.NotEqual(t, homeKey, foreignKey,
				"two organizations must never share one idempotency slot")
			assert.Contains(t, homeKey, v2ScopeOrgID, "the slot key must carry the organization the body named")
			assert.NotContains(t, homeKey, v2ForeignOrgID, "the slot key must not carry another organization")
			assert.Contains(t, foreignKey, v2ForeignOrgID, "the slot key must carry the organization the body named")
			assert.NotContains(t, foreignKey, v2ScopeOrgID, "the slot key must not carry another organization")
		})
	}
}

// TestCreateTransactionV2_IdenticalRetryClaimsOneSlot proves the other direction of the same
// contract: a byte-identical resubmission must land on the SAME slot, so it replays instead of
// posting a second transaction. Narrowing the slot — by folding anything request-specific into the
// key — would make every retry a new transaction on the money path.
func TestCreateTransactionV2_IdenticalRetryClaimsOneSlot(t *testing.T) {
	tests := []struct {
		name           string
		idempotencyKey string
	}{
		{name: "no caller key: the slot key falls back to the body hash", idempotencyKey: ""},
		{name: "caller key", idempotencyKey: "retry-caller-key"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// NOT parallel: process-global huma state.
			first := captureV2CreateSlotKey(t, "direct", v2DirectBody, tt.idempotencyKey)
			second := captureV2CreateSlotKey(t, "direct", v2DirectBody, tt.idempotencyKey)

			assert.Equal(t, first, second,
				"an identical resubmission must claim the same idempotency slot so it replays")
		})
	}
}

// TestCreateTransactionV2_RetryReplaysTheStoredTransaction proves the replay is observable end to
// end on the scope-free create path: a losing claim whose slot holds a canonical transaction
// answers 201 with that transaction and X-Idempotency-Replayed:true.
func TestCreateTransactionV2_RetryReplaysTheStoredTransaction(t *testing.T) {
	// NOT parallel: process-global huma state.
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	var gotKey string

	storedID := "11111111-1111-1111-1111-111111111111"
	handler := captureSetNXKey(t, ctrl, &gotKey, `{"id":"`+storedID+`"}`)
	app := buildHumaV2ActionApp(t, "direct", handler.CreateTransactionDirectV2)

	resp := postActionV2WithIdempotency(t, app, "direct", v2DirectBody, "replay-caller-key")
	defer func() { _ = resp.Body.Close() }()

	body := readAllForTest(t, resp)

	assert.Equal(t, http.StatusCreated, resp.StatusCode, "a replayed v2 create answers 201; body: %s", body)
	assert.Equal(t, "true", resp.Header.Get(libConstants.IdempotencyReplayed),
		"a losing claim with a stored transaction must be reported as a replay")
	assert.Contains(t, body, storedID, "the replay must return the stored transaction")
	assert.Equal(t,
		utils.IdempotencyInternalKey(v2BodyScopeOrg, v2BodyScopeLedger, "replay-caller-key"), gotKey,
		"the replayed slot must be the one scoped by the body")
}

// TestCreateTransactionV2_ScopeMismatchInBodyIsRejected proves a body whose legs name two different
// pairs is still refused with the canonical scope-mismatch code. With the scope now deciding where
// the transaction posts, this rejection is what keeps a single request from being split across two
// ledgers.
func TestCreateTransactionV2_ScopeMismatchInBodyIsRejected(t *testing.T) {
	// NOT parallel: process-global huma state.
	app := buildHumaV2ActionApp(t, "direct", (&TransactionHandler{}).CreateTransactionDirectV2)

	mismatched := `{"asset":"BRL","amount":"100",` +
		`"debits":[{"alias":"@src",` + v2ScopeJSON + `,"amount":"100"}],` +
		`"credits":[{"alias":"@dst",` + v2ForeignScopeJSON + `,"amount":"100"}]}`

	resp := postActionV2(t, app, "direct", mismatched)
	defer func() { _ = resp.Body.Close() }()

	body := readAllForTest(t, resp)

	assert.Equal(t, http.StatusUnprocessableEntity, resp.StatusCode,
		"legs naming two different scopes must be refused; body: %s", body)
	assert.Contains(t, body, cn.ErrTransactionScopeMismatch.Error(),
		"the refusal must carry the canonical scope-mismatch code")
}

// TestCreateTransactionV2_MalformedBodyScopeIsRejected proves the UUID hygiene the URL segments
// used to get from ParseUUIDPathParameters is applied to the body fields that replaced them: a
// malformed organization or ledger is a clean 400 at the decode boundary, not a deep funnel error.
func TestCreateTransactionV2_MalformedBodyScopeIsRejected(t *testing.T) {
	tests := []struct {
		name string
		body string
	}{
		{
			name: "malformed organization on a debit leg",
			body: `{"asset":"BRL","amount":"100","debits":[{"alias":"@src","organizationId":"not-a-uuid","ledgerId":"` +
				v2ScopeLedgerID + `","amount":"100"}],"credits":[{"alias":"@dst",` + v2ScopeJSON + `,"amount":"100"}]}`,
		},
		{
			name: "malformed ledger on a debit leg",
			body: `{"asset":"BRL","amount":"100","debits":[{"alias":"@src","organizationId":"` + v2ScopeOrgID +
				`","ledgerId":"not-a-uuid","amount":"100"}],"credits":[{"alias":"@dst",` + v2ScopeJSON + `,"amount":"100"}]}`,
		},
		{
			name: "malformed organization on a credit leg",
			body: `{"asset":"BRL","amount":"100","debits":[{"alias":"@src",` + v2ScopeJSON + `,"amount":"100"}],` +
				`"credits":[{"alias":"@dst","organizationId":"not-a-uuid","ledgerId":"` + v2ScopeLedgerID + `","amount":"100"}]}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// NOT parallel: process-global huma state.
			app := buildHumaV2ActionApp(t, "direct", (&TransactionHandler{}).CreateTransactionDirectV2)

			resp := postActionV2(t, app, "direct", tt.body)
			defer func() { _ = resp.Body.Close() }()

			assert.Equal(t, http.StatusBadRequest, resp.StatusCode,
				"a malformed scope in the body must be a clean 400")
		})
	}
}

// TestDecodeAndBuildV2Transaction_ReturnsTheBodyScope pins the seam the create terminals read the
// scope off: the pair the body named comes back beside the canonical transaction, whichever
// spelling the body used.
func TestDecodeAndBuildV2Transaction_ReturnsTheBodyScope(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		body string
	}{
		{name: "single leg per side", body: v2DirectBody},
		{name: "several legs per side", body: v2AdvancedBody},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			_, scope, err := decodeAndBuildV2Transaction([]byte(tt.body), false, "")
			require.NoError(t, err)

			assert.Equal(t, v2ScopeOrgID, scope.OrganizationID, "the resolved scope must be the body's organization")
			assert.Equal(t, v2ScopeLedgerID, scope.LedgerID, "the resolved scope must be the body's ledger")
		})
	}
}
