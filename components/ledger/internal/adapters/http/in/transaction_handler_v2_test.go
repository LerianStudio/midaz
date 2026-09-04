// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/LerianStudio/lib-auth/v4/auth/middleware"
	libCommons "github.com/LerianStudio/lib-commons/v6/commons"
	openapi "github.com/LerianStudio/lib-commons/v6/commons/net/http/openapi"
	libProblem "github.com/LerianStudio/lib-commons/v6/commons/net/http/problem"
	libObservability "github.com/LerianStudio/lib-observability/v4"
	libLog "github.com/LerianStudio/lib-observability/v4/log"
	"github.com/danielgtaylor/huma/v2"
	"github.com/gofiber/fiber/v3"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/LerianStudio/midaz/v4/pkg"
	cn "github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mtransaction"
	pkgHTTP "github.com/LerianStudio/midaz/v4/pkg/net/http"
)

// v2ScopeOrgID and v2ScopeLedgerID are the organization and ledger every leg of a v2 test body
// names, and therefore the scope the request is posted against: a v2 create URL names neither.
const (
	v2ScopeOrgID    = "77777777-7777-7777-7777-777777777777"
	v2ScopeLedgerID = "88888888-8888-8888-8888-888888888888"
)

// v2ScopeJSON is that scope spelled for a JSON body, ready to splice into a leg alongside its
// alias.
const v2ScopeJSON = `"organizationId":"` + v2ScopeOrgID + `","ledgerId":"` + v2ScopeLedgerID + `"`

// buildHumaV2DirectApp mounts the v2 `direct` transaction op through the SAME
// production seam (RegisterTransactionV2RoutesToApp) the unified server uses, on a
// fresh Fiber app + its own /v2 Huma contract. It mirrors buildHumaTransactionApp:
// problem.Install() before any huma.Register, WithRecover as the first middleware so
// a nil-repo panic in the unwired funnel unwinds to a 500 (not a dropped connection),
// the ledger schema namer installed after openapi.New and before registration (the v2
// output body, TransactionV2, nests operation.Operation → operation.{Status,Balance,Amount}
// and clashes on the bare names without it), and auth disabled so requests reach the
// terminal. A bare handler is enough: these tests prove the transport boundary (decode,
// translate, route-UUID hygiene, error class) and funnel entry — the committed money
// path is the integration+parity test. Since the seam mounts EVERY v2 transaction op, the
// same app serves the lifecycle ops too; pass an optional logger to have it ride the Fiber
// request context (the humafiber adapter hands that context to the terminal) when a test
// needs to assert on what the exercised path logged.
//
// MUST-NOT-PARALLELIZE (same rationale as buildHumaTransactionApp): libProblem.Install()
// swaps the process-global huma.NewError hook and Huma validation uses process-global
// sync.Pools — concurrent builds/requests cross-contaminate.
func buildHumaV2DirectApp(t *testing.T, handler *TransactionHandler, logger ...libLog.Logger) *fiber.App {
	t.Helper()

	app := fiber.New(fiber.Config{
		ErrorHandler: pkgHTTP.CanonicalFiberErrorHandler,
	})

	app.Use(pkgHTTP.WithRecover(pkgHTTP.WithRecoverLogger(&libLog.GoLogger{})))

	if len(logger) > 0 {
		app.Use(func(c fiber.Ctx) error {
			c.SetContext(libObservability.ContextWithLogger(c.Context(), logger[0]))

			return c.Next()
		})
	}

	libProblem.Install()

	apiV2 := app.Group("/v2")

	humaAPI := openapi.New(app, apiV2, openapi.Config{Title: "ledger-test-v2", Version: "test", Servers: []string{"/v2"}})
	pkgHTTP.InstallLedgerSchemaNamer(humaAPI)

	RegisterTransactionV2RoutesToApp(apiV2, humaAPI, &middleware.AuthClient{Enabled: false}, handler, nil)

	return app
}

// directV2ConcretePath is the v2 direct path. It names no organization and no ledger: the
// scope a create is posted against travels in the request body.
const directV2ConcretePath = "/v2/transactions/direct"

// postDirectV2 issues an authenticated POST to the v2 direct route with the given JSON body.
func postDirectV2(t *testing.T, app *fiber.App, body string) *http.Response {
	t.Helper()

	req := httptest.NewRequest(http.MethodPost, directV2ConcretePath, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req, fiber.TestConfig{Timeout: 0})
	require.NoError(t, err)

	return resp
}

// TestCreateTransactionDirectV2_MalformedBody_400 proves the real handler decodes
// the flat v2 body through http.DecodeAndValidate (the SAME validator the v1 create
// ops run): malformed JSON is the canonical 400 RFC 9457 problem — never a native Huma
// 422 and never the 501 stub.
func TestCreateTransactionDirectV2_MalformedBody_400(t *testing.T) {
	// NOT parallel: process-global huma state.
	app := buildHumaV2DirectApp(t, &TransactionHandler{})

	resp := postDirectV2(t, app, `{not-json`)
	defer func() { _ = resp.Body.Close() }()

	body, _ := io.ReadAll(resp.Body)
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode, "malformed v2 body stays canonical 400 — no native Huma 422, no 501 stub")
	assert.Contains(t, string(body), "status", "error body must be the RFC 9457 problem envelope")
}

// TestCreateTransactionDirectV2_MalformedRouteUUID_400 pins the route-UUID hygiene
// contract: routeId is an optional *string on the flat v2 input, so a malformed route
// UUID MUST surface as a clean 400 at the decode boundary — never fall through to a deep
// 500 in the funnel.
func TestCreateTransactionDirectV2_MalformedRouteUUID_400(t *testing.T) {
	// NOT parallel: process-global huma state.
	app := buildHumaV2DirectApp(t, &TransactionHandler{})

	resp := postDirectV2(t, app, `{"asset":"BRL","amount":"100","debits":[{"alias":"@src",`+v2ScopeJSON+`,"amount":"100"}],"credits":[{"alias":"@dst",`+v2ScopeJSON+`,"amount":"100"}],"routeId":"not-a-uuid"}`)
	defer func() { _ = resp.Body.Close() }()

	assert.Equal(t, http.StatusBadRequest, resp.StatusCode, "malformed routeId must be a clean 400, not a deep 500")
}

// TestCreateTransactionDirectV2_MalformedOperationRouteUUID_400 mirrors the routeId
// hygiene for the per-leg operationRouteId.
func TestCreateTransactionDirectV2_MalformedOperationRouteUUID_400(t *testing.T) {
	// NOT parallel: process-global huma state.
	app := buildHumaV2DirectApp(t, &TransactionHandler{})

	resp := postDirectV2(t, app, `{"asset":"BRL","amount":"100","debits":[{"alias":"@src",`+v2ScopeJSON+`,"amount":"100"}],"credits":[{"alias":"@dst",`+v2ScopeJSON+`,"amount":"100"}],"operationRouteId":"not-a-uuid"}`)
	defer func() { _ = resp.Body.Close() }()

	assert.Equal(t, http.StatusBadRequest, resp.StatusCode, "malformed operationRouteId must be a clean 400, not a deep 500")
}

// TestCreateTransactionDirectV2_NonPositiveAmount_422 proves the Translate business
// branch (non-positive amount) maps to a canonical 422.
func TestCreateTransactionDirectV2_NonPositiveAmount_422(t *testing.T) {
	// NOT parallel: process-global huma state.
	app := buildHumaV2DirectApp(t, &TransactionHandler{})

	resp := postDirectV2(t, app, `{"asset":"BRL","amount":"0","debits":[{"alias":"@src",`+v2ScopeJSON+`,"amount":"100"}],"credits":[{"alias":"@dst",`+v2ScopeJSON+`,"amount":"100"}]}`)
	defer func() { _ = resp.Body.Close() }()

	assert.Equal(t, http.StatusUnprocessableEntity, resp.StatusCode, "non-positive amount is a Translate business error → 422")
}

// TestCreateTransactionDirectV2_ValidBodyEntersFunnel proves the happy-path wiring
// up to the funnel: a fully valid flat body passes decode + Translate(false) and is
// handed to the SAME createTransaction funnel the v1 create ops use. With a bare handler
// the funnel's first repository call has no wired dependency, so WithRecover maps the
// resulting panic to a 500 — proving the request progressed PAST the transport/translate
// boundary into the funnel (never the 501 stub, never a decode/translate 4xx). The
// committed transaction result is asserted by the integration+parity test.
func TestCreateTransactionDirectV2_ValidBodyEntersFunnel(t *testing.T) {
	// NOT parallel: process-global huma state.
	app := buildHumaV2DirectApp(t, &TransactionHandler{})

	resp := postDirectV2(t, app, `{"description":"v2 direct","asset":"BRL","amount":"100","debits":[{"alias":"@src",`+v2ScopeJSON+`,"amount":"100"}],"credits":[{"alias":"@dst",`+v2ScopeJSON+`,"amount":"100"}]}`)
	defer func() { _ = resp.Body.Close() }()

	assert.Equal(t, http.StatusInternalServerError, resp.StatusCode,
		"valid body must clear the transport/translate boundary and enter the funnel (unwired repos → recovered 500; committed path is the integration+parity test)")
}

// TestCreateTransactionHoldV2_MalformedBody_400 proves the hold handler decodes the
// flat v2 body through the SAME http.DecodeAndValidate the direct handler runs: malformed
// JSON is the canonical 400 RFC 9457 problem, never a native Huma 422 nor a 501 stub.
func TestCreateTransactionHoldV2_MalformedBody_400(t *testing.T) {
	// NOT parallel: process-global huma state.
	app := buildHumaV2ActionApp(t, "hold", (&TransactionHandler{}).CreateTransactionHoldV2)

	resp := postActionV2(t, app, "hold", `{not-json`)
	defer func() { _ = resp.Body.Close() }()

	body, _ := io.ReadAll(resp.Body)
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode, "malformed v2 hold body stays canonical 400")
	assert.Contains(t, string(body), "status", "error body must be the RFC 9457 problem envelope")
}

// TestCreateTransactionHoldV2_ValidBodyEntersFunnel proves the hold happy-path wiring
// up to the funnel: a fully valid flat body passes decode + Translate(true) and is handed
// to the SAME createTransaction funnel. With a bare handler the funnel's first repository
// call has no wired dependency, so WithRecover maps the resulting panic to a 500 — proving
// the request progressed PAST the transport/translate boundary into the funnel.
func TestCreateTransactionHoldV2_ValidBodyEntersFunnel(t *testing.T) {
	// NOT parallel: process-global huma state.
	app := buildHumaV2ActionApp(t, "hold", (&TransactionHandler{}).CreateTransactionHoldV2)

	resp := postActionV2(t, app, "hold", `{"description":"v2 hold","asset":"BRL","amount":"100","debits":[{"alias":"@src",`+v2ScopeJSON+`,"amount":"100"}],"credits":[{"alias":"@dst",`+v2ScopeJSON+`,"amount":"100"}]}`)
	defer func() { _ = resp.Body.Close() }()

	assert.Equal(t, http.StatusInternalServerError, resp.StatusCode,
		"valid hold body must clear the transport/translate boundary and enter the funnel (unwired repos → recovered 500)")
}

// TestHuma_CreateTransactionHoldV2_IdempotencyKeyedByDiscriminatedRawV2Body proves the hold
// surface keys idempotency off the raw v2 body AS SUBMITTED, but folds the HOLD action
// discriminator into the hash source (the endpoint, not the body, carries the action). It
// probes the SAME first-repo touch as the direct idempotency lock (TransactionRedisRepo.SetNX,
// whose internalKey embeds the hash source when no X-Idempotency header is sent): the captured
// key must embed hash("HOLD\x00"+body) and must NOT embed the bare-body hash the direct action
// uses — the observable guarantee that direct and hold never cross-dedup. The losing claim
// replays a cached canonical value to 201 — proving hold reaches the idempotency claim.
func TestHuma_CreateTransactionHoldV2_IdempotencyKeyedByDiscriminatedRawV2Body(t *testing.T) {
	// NOT parallel: process-global huma state.
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	var gotKey string

	handler := captureSetNXKey(t, ctrl, &gotKey, "{}")
	app := buildHumaV2ActionApp(t, "hold", handler.CreateTransactionHoldV2)

	resp := postActionV2(t, app, "hold", v2DirectBody)
	defer func() { _ = resp.Body.Close() }()

	assert.Contains(t, gotKey, libCommons.HashSHA256("HOLD\x00"+v2DirectBody),
		"v2 hold idempotency must be keyed by the HOLD-discriminated raw v2 body; got internalKey=%q", gotKey)
	assert.NotContains(t, gotKey, libCommons.HashSHA256(v2DirectBody),
		"v2 hold must NOT key off the bare body — that is the direct action's identity, and reusing it cross-dedups direct↔hold")
	assert.Equal(t, http.StatusCreated, resp.StatusCode, "a losing hold claim with a cached canonical value replays → 201")
}

// TestCreateTransactionV2_CancelledContext proves the shared helper guards the request
// context before any decode/translate work: a cancelled context short-circuits to an
// RFC 9457 problem, so the funnel is never entered.
func TestCreateTransactionV2_CancelledContext(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	handler := &TransactionHandler{}

	out, err := handler.createTransactionV2(ctx, []byte(v2DirectBody), "", "", false, "")

	require.Error(t, err, "a cancelled context must short-circuit before the funnel")
	assert.Nil(t, out, "no output envelope on the cancelled-context guard")
}

// TestCreateTransactionV2_StampsOperationTypeOverride proves the v2 helper stamps a
// non-empty Operation.Type override onto the transaction it hands to the funnel and carries
// the caller's (non-)pending intent through Translate. It asserts on
// decodeAndBuildV2Transaction — the EXACT mtransaction.Transaction createTransactionV2 passes
// into createTransactionShell — so the assertion goes red if the override-stamping line is
// removed (it is a real check, not a status/idempotency tautology). The persisted
// Operation.Type effect of this override is exercised end-to-end by the block/unblock
// integration test.
func TestCreateTransactionV2_StampsOperationTypeOverride(t *testing.T) {
	t.Parallel()

	// block action identity: (pending=false, override="BLOCK").
	tx, _, err := decodeAndBuildV2Transaction([]byte(v2DirectBody), false, "BLOCK")
	require.NoError(t, err)

	assert.Equal(t, "BLOCK", tx.OperationTypeOverride,
		"a non-empty override must be stamped onto the transaction handed to the funnel")
	assert.False(t, tx.Pending,
		"the block action is non-pending; Translate must carry pending=false through to the funnel")
}

// buildHumaV2ActionApp mounts a single named v2 create action on a fresh Fiber app + its own
// /v2 Huma contract. Wiring one terminal at a time — with the SAME Fiber auth/tenant chain and
// SkipValidateBody Huma op the production route carries — keeps a failure attributable to the
// handler under test rather than to any sibling op. Same MUST-NOT-PARALLELIZE rationale as
// buildHumaV2DirectApp: libProblem.Install() and Huma validation use process-global state.
func buildHumaV2ActionApp(t *testing.T, action string, op func(context.Context, *CreateTransactionInputV2) (*CreateTransactionOutputV2, error)) *fiber.App {
	t.Helper()

	app := fiber.New(fiber.Config{
		ErrorHandler: pkgHTTP.CanonicalFiberErrorHandler,
	})

	app.Use(pkgHTTP.WithRecover(pkgHTTP.WithRecoverLogger(&libLog.GoLogger{})))

	libProblem.Install()

	apiV2 := app.Group("/v2")

	humaAPI := openapi.New(app, apiV2, openapi.Config{Title: "ledger-test-v2-" + action, Version: "test", Servers: []string{"/v2"}})
	pkgHTTP.InstallLedgerSchemaNamer(humaAPI)

	actionPath := "/transactions/" + action

	routePost(apiV2, actionPath, protectedMidaz(&middleware.AuthClient{Enabled: false}, "transactions", "post", nil))

	huma.Register(humaAPI, huma.Operation{
		OperationID:      "createTransaction" + strings.ToUpper(action[:1]) + action[1:] + "V2",
		Method:           http.MethodPost,
		Path:             actionPath,
		Summary:          "Create a Transaction using the v2 " + action + " model",
		Tags:             []string{"Transactions"},
		Security:         secTransactionBearer,
		SkipValidateBody: true,
		DefaultStatus:    http.StatusCreated,
	}, op)

	return app
}

// postActionV2 issues an authenticated POST to a v2 create action route. The route names no
// organization and no ledger; the scope comes from the body the caller passes.
func postActionV2(t *testing.T, app *fiber.App, action, body string) *http.Response {
	t.Helper()

	return postActionV2WithIdempotency(t, app, action, body, "")
}

// postActionV2WithIdempotency is postActionV2 with an X-Idempotency header, sent only when
// idempotencyKey is non-empty so the no-key path stays reachable.
func postActionV2WithIdempotency(t *testing.T, app *fiber.App, action, body, idempotencyKey string) *http.Response {
	t.Helper()

	req := httptest.NewRequest(http.MethodPost, "/v2/transactions/"+action, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	if idempotencyKey != "" {
		req.Header.Set("X-Idempotency", idempotencyKey)
	}

	resp, err := app.Test(req, fiber.TestConfig{Timeout: 0})
	require.NoError(t, err)

	return resp
}

// readAllForTest reads a response body into a string for assertions that inspect it.
func readAllForTest(t *testing.T, resp *http.Response) string {
	t.Helper()

	raw, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	return string(raw)
}

// TestCreateTransactionBlockV2_MalformedBody_400 proves the block handler decodes the flat
// v2 body through the SAME http.DecodeAndValidate the direct/hold handlers run: malformed JSON
// is the canonical 400 RFC 9457 problem, never a native Huma 422 nor a 501 stub.
func TestCreateTransactionBlockV2_MalformedBody_400(t *testing.T) {
	// NOT parallel: process-global huma state.
	app := buildHumaV2ActionApp(t, "block", (&TransactionHandler{}).CreateTransactionBlockV2)

	resp := postActionV2(t, app, "block", `{not-json`)
	defer func() { _ = resp.Body.Close() }()

	body, _ := io.ReadAll(resp.Body)
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode, "malformed v2 block body stays canonical 400")
	assert.Contains(t, string(body), "status", "error body must be the RFC 9457 problem envelope")
}

// TestCreateTransactionBlockV2_ValidBodyEntersFunnel proves the block happy-path wiring up
// to the funnel: a fully valid flat body passes decode + Translate(false) and is handed to the
// SAME createTransaction funnel. With a bare handler the funnel's first repository call has no
// wired dependency, so WithRecover maps the resulting panic to a 500 — proving the request
// progressed PAST the transport/translate boundary into the funnel.
func TestCreateTransactionBlockV2_ValidBodyEntersFunnel(t *testing.T) {
	// NOT parallel: process-global huma state.
	app := buildHumaV2ActionApp(t, "block", (&TransactionHandler{}).CreateTransactionBlockV2)

	resp := postActionV2(t, app, "block", `{"description":"v2 block","asset":"BRL","amount":"100","debits":[{"alias":"@src",`+v2ScopeJSON+`,"amount":"100"}],"credits":[{"alias":"@dst",`+v2ScopeJSON+`,"amount":"100"}]}`)
	defer func() { _ = resp.Body.Close() }()

	assert.Equal(t, http.StatusInternalServerError, resp.StatusCode,
		"valid block body must clear the transport/translate boundary and enter the funnel (unwired repos → recovered 500)")
}

// TestHuma_CreateTransactionBlockV2_IdempotencyKeyedByBlockDiscriminatedRawV2Body proves the block
// handler routes with the BLOCK operation-type override: it probes the SAME first-repo touch as the
// direct/hold idempotency locks (TransactionRedisRepo.SetNX, whose internalKey embeds the hash
// source when no X-Idempotency header is sent). The captured key must embed hash("BLOCK\x00"+body)
// and must NOT embed the bare-body hash the direct action uses — the observable guarantee that the
// handler passed constant.BLOCK through to the shared helper and that block never cross-dedups direct.
func TestHuma_CreateTransactionBlockV2_IdempotencyKeyedByBlockDiscriminatedRawV2Body(t *testing.T) {
	// NOT parallel: process-global huma state.
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	var gotKey string

	handler := captureSetNXKey(t, ctrl, &gotKey, "{}")
	app := buildHumaV2ActionApp(t, "block", handler.CreateTransactionBlockV2)

	resp := postActionV2(t, app, "block", v2DirectBody)
	defer func() { _ = resp.Body.Close() }()

	assert.Contains(t, gotKey, libCommons.HashSHA256("BLOCK\x00"+v2DirectBody),
		"v2 block idempotency must be keyed by the BLOCK-discriminated raw v2 body; got internalKey=%q", gotKey)
	assert.NotContains(t, gotKey, libCommons.HashSHA256(v2DirectBody),
		"v2 block must NOT key off the bare body — that is the direct action's identity")
	assert.Equal(t, http.StatusCreated, resp.StatusCode, "a losing block claim with a cached canonical value replays → 201")
}

// TestCreateTransactionUnblockV2_MalformedBody_400 mirrors the block malformed-body contract
// for the unblock action.
func TestCreateTransactionUnblockV2_MalformedBody_400(t *testing.T) {
	// NOT parallel: process-global huma state.
	app := buildHumaV2ActionApp(t, "unblock", (&TransactionHandler{}).CreateTransactionUnblockV2)

	resp := postActionV2(t, app, "unblock", `{not-json`)
	defer func() { _ = resp.Body.Close() }()

	body, _ := io.ReadAll(resp.Body)
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode, "malformed v2 unblock body stays canonical 400")
	assert.Contains(t, string(body), "status", "error body must be the RFC 9457 problem envelope")
}

// TestCreateTransactionUnblockV2_ValidBodyEntersFunnel mirrors the block funnel-entry contract
// for the unblock action.
func TestCreateTransactionUnblockV2_ValidBodyEntersFunnel(t *testing.T) {
	// NOT parallel: process-global huma state.
	app := buildHumaV2ActionApp(t, "unblock", (&TransactionHandler{}).CreateTransactionUnblockV2)

	resp := postActionV2(t, app, "unblock", `{"description":"v2 unblock","asset":"BRL","amount":"100","debits":[{"alias":"@src",`+v2ScopeJSON+`,"amount":"100"}],"credits":[{"alias":"@dst",`+v2ScopeJSON+`,"amount":"100"}]}`)
	defer func() { _ = resp.Body.Close() }()

	assert.Equal(t, http.StatusInternalServerError, resp.StatusCode,
		"valid unblock body must clear the transport/translate boundary and enter the funnel (unwired repos → recovered 500)")
}

// TestHuma_CreateTransactionUnblockV2_IdempotencyKeyedByUnblockDiscriminatedRawV2Body proves the
// unblock handler routes with the UNBLOCK operation-type override, mirroring the block idempotency
// proof: the captured SetNX key must embed hash("UNBLOCK\x00"+body) and not the bare-body hash.
func TestHuma_CreateTransactionUnblockV2_IdempotencyKeyedByUnblockDiscriminatedRawV2Body(t *testing.T) {
	// NOT parallel: process-global huma state.
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	var gotKey string

	handler := captureSetNXKey(t, ctrl, &gotKey, "{}")
	app := buildHumaV2ActionApp(t, "unblock", handler.CreateTransactionUnblockV2)

	resp := postActionV2(t, app, "unblock", v2DirectBody)
	defer func() { _ = resp.Body.Close() }()

	assert.Contains(t, gotKey, libCommons.HashSHA256("UNBLOCK\x00"+v2DirectBody),
		"v2 unblock idempotency must be keyed by the UNBLOCK-discriminated raw v2 body; got internalKey=%q", gotKey)
	assert.NotContains(t, gotKey, libCommons.HashSHA256(v2DirectBody),
		"v2 unblock must NOT key off the bare body — that is the direct action's identity")
	assert.Equal(t, http.StatusCreated, resp.StatusCode, "a losing unblock claim with a cached canonical value replays → 201")
}

// TestDecodeAndBuildV2Transaction_BlockUnblockStampOverrideAndForceNonPending locks the canonical
// Transaction shape the block/unblock handlers hand to the funnel — the EXACT
// mtransaction.Transaction createTransactionV2 passes into createTransactionShell for the
// (pending=false, override) action identity each handler wires. It asserts the override is stamped
// AND Translate carried pending=false (parity with v1 block/unblock which force Pending=false).
func TestDecodeAndBuildV2Transaction_BlockUnblockStampOverrideAndForceNonPending(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		override string
	}{
		{name: "block", override: "BLOCK"},
		{name: "unblock", override: "UNBLOCK"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			tx, _, err := decodeAndBuildV2Transaction([]byte(v2DirectBody), false, tc.override)
			require.NoError(t, err)

			assert.Equal(t, tc.override, tx.OperationTypeOverride,
				"the %s action must stamp its override onto the transaction handed to the funnel", tc.name)
			assert.False(t, tx.Pending,
				"the %s action is non-pending; Translate must carry pending=false through to the funnel", tc.name)
		})
	}
}

// TestV2IdempotencyHashSource_DiscriminatesActions locks the no-key idempotency mapping: the
// v2 action is carried by the endpoint, so each action must fold a distinct identity into the
// hash source. Direct MUST stay byte-identical to the bare body (its established contract);
// every other action prefixes its discriminator + NUL. This is the observable guarantee that
// byte-identical bodies posted to different actions never share an idempotency slot.
//
// The mapping is a function of the ACTION alone, never of the body shape: the single-leg and
// multi-leg bodies are two byte sequences fed to the same source, so the table runs over both
// and expects the identical per-action treatment.
func TestV2IdempotencyHashSource_DiscriminatesActions(t *testing.T) {
	t.Parallel()

	bodies := []struct {
		name string
		body string
	}{
		{name: "single-leg body", body: v2DirectBody},
		{name: "advanced body", body: v2AdvancedBody},
	}

	actions := []struct {
		name     string
		pending  bool
		override string
		wantDisc string
	}{
		{name: "direct stays bare body", pending: false, override: "", wantDisc: ""},
		{name: "hold", pending: true, override: "", wantDisc: "HOLD"},
		{name: "block", pending: false, override: "BLOCK", wantDisc: "BLOCK"},
		{name: "unblock", pending: false, override: "UNBLOCK", wantDisc: "UNBLOCK"},
	}

	for _, bc := range bodies {
		t.Run(bc.name, func(t *testing.T) {
			t.Parallel()

			raw := []byte(bc.body)

			for _, tc := range actions {
				t.Run(tc.name, func(t *testing.T) {
					t.Parallel()

					wantHashSrc := bc.body
					if tc.wantDisc != "" {
						wantHashSrc = tc.wantDisc + "\x00" + bc.body
					}

					assert.Equal(t, tc.wantDisc, idempotencyActionDiscriminator(tc.pending, tc.override),
						"action discriminator mapping")
					assert.Equal(t, wantHashSrc, v2IdempotencyHashSource(raw, tc.pending, tc.override),
						"idempotency hash source")
				})
			}

			// Direct is byte-identical to the bare body, whatever the body spells: this exact
			// invariant keeps the existing direct idempotency tests green unchanged.
			assert.Equal(t, bc.body, v2IdempotencyHashSource(raw, false, ""),
				"direct's hash source MUST remain exactly the bare body")

			// No two actions collide on the same body.
			seen := map[string]string{}
			for _, tc := range actions {
				src := v2IdempotencyHashSource(raw, tc.pending, tc.override)
				if prev, dup := seen[src]; dup {
					t.Fatalf("actions %q and %q share an idempotency hash source", prev, tc.name)
				}

				seen[src] = tc.name
			}
		})
	}
}

// v2AdvancedBody is the leg-array spelling of a 100 BRL transaction: two explicit-amount
// debit legs and two 50% share credit legs, so one body exercises both per-leg value
// expressions on both sides. It is the counterpart of v2DirectBody, which spells the same
// total with one leg per side. Two legs per side is what makes it a valid probe for a
// per-leg claim — with one leg, "expanded per entry" and "collapsed onto one leg" are
// indistinguishable.
const v2AdvancedBody = `{"description":"v2 advanced","asset":"BRL","amount":"100",` +
	`"debits":[{"alias":"@srcA",` + v2ScopeJSON + `,"amount":"60"},{"alias":"@srcB",` + v2ScopeJSON + `,"amount":"40"}],` +
	`"credits":[{"alias":"@dstA",` + v2ScopeJSON + `,"share":{"percentage":50}},{"alias":"@dstB",` + v2ScopeJSON + `,"share":{"percentage":50}}]}`

// humaV2CreateOp is the shape every v2 create terminal shares. All four actions carry the
// same request envelope and the same success envelope; only the identity they pass to
// createTransactionV2 differs.
type humaV2CreateOp = func(context.Context, *CreateTransactionInputV2) (*CreateTransactionOutputV2, error)

// v2CreateActionCase describes one v2 create action by everything that distinguishes it: the
// route suffix, the terminal, the (pending, override) identity the terminal passes to the
// shared helper, the transaction status that identity opens, and the idempotency
// discriminator it folds into the hash source.
type v2CreateActionCase struct {
	name       string
	route      string
	op         func(*TransactionHandler) humaV2CreateOp
	pending    bool
	override   string
	wantStatus string
	wantDisc   string
}

// v2CreateActionCases enumerates the four v2 create actions. Every test that must hold for
// ALL of them iterates this table, so an action added to the surface without being covered
// here is a visible omission rather than a silent gap.
func v2CreateActionCases() []v2CreateActionCase {
	return []v2CreateActionCase{
		{
			name:       "direct",
			route:      "direct",
			op:         func(h *TransactionHandler) humaV2CreateOp { return h.CreateTransactionDirectV2 },
			pending:    false,
			override:   "",
			wantStatus: cn.CREATED,
			wantDisc:   "",
		},
		{
			name:       "hold",
			route:      "hold",
			op:         func(h *TransactionHandler) humaV2CreateOp { return h.CreateTransactionHoldV2 },
			pending:    true,
			override:   "",
			wantStatus: cn.PENDING,
			wantDisc:   "HOLD",
		},
		{
			name:       "block",
			route:      "block",
			op:         func(h *TransactionHandler) humaV2CreateOp { return h.CreateTransactionBlockV2 },
			pending:    false,
			override:   cn.BLOCK,
			wantStatus: cn.CREATED,
			wantDisc:   cn.BLOCK,
		},
		{
			name:       "unblock",
			route:      "unblock",
			op:         func(h *TransactionHandler) humaV2CreateOp { return h.CreateTransactionUnblockV2 },
			pending:    false,
			override:   cn.UNBLOCK,
			wantStatus: cn.CREATED,
			wantDisc:   cn.UNBLOCK,
		},
	}
}

// TestDecodeAndBuildV2Transaction_AdvancedFormAcrossActions proves every v2 create action
// accepts the leg-array spelling and expands it into one canonical leg per entry, carrying
// the action's own identity onto the result: hold opens PENDING while the other three open
// CREATED, and block/unblock stamp their Operation.Type override. It asserts on
// decodeAndBuildV2Transaction — the EXACT mtransaction.Transaction createTransactionV2 hands
// to createTransactionShell — so the leg expansion and the stamped identity are checked on
// the value the funnel receives. The per-OPERATION effect of the override across N legs is
// asserted by the advanced integration test.
func TestDecodeAndBuildV2Transaction_AdvancedFormAcrossActions(t *testing.T) {
	t.Parallel()

	for _, tc := range v2CreateActionCases() {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			tx, _, err := decodeAndBuildV2Transaction([]byte(v2AdvancedBody), tc.pending, tc.override)
			require.NoError(t, err, "the %s action must accept the leg-array spelling", tc.name)

			// One canonical leg per array entry, in submission order, each carrying the value
			// expression its entry spelled.
			from := tx.Send.Source.From
			to := tx.Send.Distribute.To

			require.Len(t, from, 2, "the %s action must expand both source legs", tc.name)
			require.Len(t, to, 2, "the %s action must expand both destination legs", tc.name)

			assert.Equal(t, "@srcA", from[0].AccountAlias)
			require.NotNil(t, from[0].Amount, "the explicit-amount source leg must carry an amount")
			assert.True(t, decimal.NewFromInt(60).Equal(from[0].Amount.Value), "source leg amount")
			assert.Equal(t, "@srcB", from[1].AccountAlias)
			require.NotNil(t, from[1].Amount, "the second explicit-amount source leg must carry an amount")
			assert.True(t, decimal.NewFromInt(40).Equal(from[1].Amount.Value), "second source leg amount")

			assert.Equal(t, "@dstA", to[0].AccountAlias)
			require.NotNil(t, to[0].Share, "the share destination leg must carry a share")
			assert.Equal(t, int64(50), to[0].Share.Percentage, "destination leg share percentage")
			assert.Equal(t, "@dstB", to[1].AccountAlias)
			require.NotNil(t, to[1].Share, "the second share destination leg must carry a share")
			assert.Equal(t, int64(50), to[1].Share.Percentage, "second destination leg share percentage")

			// The action identity rides on the advanced body exactly as it does on the
			// single-leg one: pending drives the opening status, the override is stamped
			// verbatim.
			assert.Equal(t, tc.pending, tx.Pending, "the %s action must carry its pending intent through Translate", tc.name)
			assert.Equal(t, tc.wantStatus, tx.InitialStatus(), "the %s action must open the transaction as %s", tc.name, tc.wantStatus)
			assert.Equal(t, tc.override, tx.OperationTypeOverride, "the %s action must stamp its Operation.Type override", tc.name)
		})
	}
}

// v2PerLegDescriptionBody spells a v2 body whose transaction description differs from every
// leg's, and whose last credit leg names none at all. The three values are distinct so a seam
// that collapsed them onto one — or onto the transaction's — cannot pass.
const v2PerLegDescriptionBody = `{"description":"v2 transaction note","asset":"BRL","amount":"100",` +
	`"debits":[{"alias":"@srcA",` + v2ScopeJSON + `,"amount":"60","description":"srcA leg note"},` +
	`{"alias":"@srcB",` + v2ScopeJSON + `,"amount":"40","description":"srcB leg note"}],` +
	`"credits":[{"alias":"@dstA",` + v2ScopeJSON + `,"amount":"70","description":"dstA leg note"},` +
	`{"alias":"@dstB",` + v2ScopeJSON + `,"amount":"30"}]}`

// TestDecodeAndBuildV2Transaction_CarriesPerLegDescriptions proves a v2 body carrying a
// `description` on its legs clears the decode boundary — a field the struct does not publish is
// answered with a 400, so acceptance is worth pinning — and that each leg's own value reaches the
// transaction the funnel is handed, keyed to the leg that spelled it.
//
// The leg that names none stays EMPTY here: the transaction-level description is substituted by
// the operation builders downstream, and filling it at this seam would make an inherited
// description indistinguishable from an authored one.
func TestDecodeAndBuildV2Transaction_CarriesPerLegDescriptions(t *testing.T) {
	t.Parallel()

	var probe mtransaction.CreateTransactionV2Input

	_, decodeErr := pkgHTTP.DecodeAndValidate([]byte(v2PerLegDescriptionBody), &probe)
	require.NoError(t, decodeErr, "a per-leg description must not be answered as an unknown field")

	tx, _, err := decodeAndBuildV2Transaction([]byte(v2PerLegDescriptionBody), false, "")
	require.NoError(t, err)

	assert.Equal(t, "v2 transaction note", tx.Description,
		"the transaction keeps its own description alongside the legs'")

	from := tx.Send.Source.From
	to := tx.Send.Distribute.To

	require.Len(t, from, 2)
	require.Len(t, to, 2)

	assert.Equal(t, "srcA leg note", from[0].Description)
	assert.Equal(t, "srcB leg note", from[1].Description)
	assert.Equal(t, "dstA leg note", to[0].Description)
	assert.Empty(t, to[1].Description,
		"a leg naming no description reaches the operation builders empty, which is what makes the fallback theirs to apply")
}

// TestDecodeV2Body_RemainingLegRejectionIsSpellingSensitive pins what the decode boundary
// ACTUALLY does with a `remaining` key on a v2 leg. The v2 leg publishes no such field, so
// the intended answer is the unknown-field rejection (0053) — and that is what the truthy
// spelling gets. The FALSY spelling does not: FindUnknownFields skips any key whose value is
// boolean false (a deliberate carve-out so the per-call `skip.*` flags can send their own
// default), so `"remaining": false` is silently accepted and the leg decodes as if the key
// were never sent.
//
// The money impact is nil — a swallowed `false` asks for nothing — but "a client that sends
// `remaining` gets the unknown-field rejection" is not true of both spellings, and this pin
// is what stops that claim from being restated as unconditional. It asserts on
// pkgHTTP.DecodeAndValidate, the exact boundary the v2 handlers call, so no funnel status
// stands in for the decode verdict.
func TestDecodeV2Body_RemainingLegRejectionIsSpellingSensitive(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		body       string
		wantReject bool
		wantCode   string
	}{
		{
			name:       "truthy remaining on a leg is an unknown field",
			body:       `{"asset":"USD","amount":"100","debits":[{"alias":"@srcA",` + v2ScopeJSON + `,"remaining":true}],"credits":[{"alias":"@dst",` + v2ScopeJSON + `,"amount":"100"}]}`,
			wantReject: true,
			wantCode:   cn.ErrUnexpectedFieldsInTheRequest.Error(),
		},
		{
			name:       "falsy remaining on a leg is swallowed by the boolean-false carve-out",
			body:       `{"asset":"USD","amount":"100","debits":[{"alias":"@srcA",` + v2ScopeJSON + `,"amount":"100","remaining":false}],"credits":[{"alias":"@dst",` + v2ScopeJSON + `,"amount":"100"}]}`,
			wantReject: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			payload := new(mtransaction.CreateTransactionV2Input)

			_, err := pkgHTTP.DecodeAndValidate([]byte(tc.body), payload)

			if !tc.wantReject {
				require.NoError(t, err, "the falsy spelling is accepted at the decode boundary")
				assert.Nil(t, payload.Debits[0].Share, "a swallowed remaining key leaves the leg's value expressions untouched")
				assert.Equal(t, "100", payload.Debits[0].Amount, "the leg decodes on its own amount, as if remaining were never sent")

				return
			}

			require.Error(t, err, "the truthy spelling must be rejected at the decode boundary")

			var unknownFields pkg.ValidationUnknownFieldsError
			require.ErrorAs(t, err, &unknownFields, "the rejection must be the unknown-field class, not a generic decode failure")
			assert.Equal(t, tc.wantCode, unknownFields.Code, "unknown-field rejections carry the canonical unexpected-fields code")
		})
	}
}

// externalUSDLegAlias is the alias every ledger's USD external account carries, spelled from
// the production prefix so a change to it surfaces here. The `/` is the point: the registered
// account-alias charset (`invalidaliascharacters`) excludes it, so any guard derived from that
// charset rejects this alias.
const externalUSDLegAlias = cn.DefaultExternalAccountAliasPrefix + "USD"

// TestDecodeV2Body_ExternalAccountAliasSurvivesTheLegPositions pins that the external-account
// alias reaches the canonical leg through both leg arrays. On a surface that publishes no
// inflow/outflow action, a single debit leg naming `@external/USD` is the canonical deposit call
// and its mirror on the credit side is the canonical withdrawal — so a rejection here 400s every
// deposit and every withdrawal in production.
//
// It goes through pkgHTTP.DecodeAndValidate rather than straight into Translate because the
// regression this guards is a TAG: `invalidaliascharacters` is registered and already applied
// that way to mmodel.Account.Alias and mmodel.Composition.Alias, while V2LegInput.Alias carries
// no such tag. Appending it is a one-token change, and Translate evaluates no struct tags — so a
// lock that calls Translate directly stays green through it. Only the decode boundary sees the
// tag layer.
func TestDecodeV2Body_ExternalAccountAliasSurvivesTheLegPositions(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		body     string
		wantFrom string
		wantTo   string
	}{
		{
			name: "a deposit names the external account in the debit leg",
			body: `{"asset":"USD","amount":"100",` +
				`"debits":[{"alias":"` + externalUSDLegAlias + `",` + v2ScopeJSON + `,"amount":"100"}],` +
				`"credits":[{"alias":"@alice",` + v2ScopeJSON + `,"amount":"100"}]}`,
			wantFrom: externalUSDLegAlias,
			wantTo:   "@alice",
		},
		{
			name: "a withdrawal names the external account in the credit leg",
			body: `{"asset":"USD","amount":"100",` +
				`"debits":[{"alias":"@alice",` + v2ScopeJSON + `,"amount":"100"}],` +
				`"credits":[{"alias":"` + externalUSDLegAlias + `",` + v2ScopeJSON + `,"amount":"100"}]}`,
			wantFrom: "@alice",
			wantTo:   externalUSDLegAlias,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			payload := new(mtransaction.CreateTransactionV2Input)

			_, err := pkgHTTP.DecodeAndValidate([]byte(tc.body), payload)
			require.NoError(t, err,
				"the external-account alias must clear the decode boundary on the leg positions — a charset tag on the leg alias would 400 every deposit")

			require.Len(t, payload.Debits, 1)
			require.Len(t, payload.Credits, 1)
			assert.Equal(t, tc.wantFrom, payload.Debits[0].Alias, "the decoded debit leg must carry the submitted alias verbatim")
			assert.Equal(t, tc.wantTo, payload.Credits[0].Alias, "the decoded credit leg must carry the submitted alias verbatim")

			tx, _, err := payload.Translate(false)
			require.NoError(t, err, "the leg-array spelling of the external account must translate")

			require.Len(t, tx.Send.Source.From, 1)
			require.Len(t, tx.Send.Distribute.To, 1)

			assert.Equal(t, tc.wantFrom, tx.Send.Source.From[0].AccountAlias,
				"the alias must reach the canonical debit leg unchanged — no rewrite, no truncation at the separator")
			assert.Equal(t, tc.wantTo, tx.Send.Distribute.To[0].AccountAlias,
				"the alias must reach the canonical credit leg unchanged")
		})
	}
}

// TestV2CreateOps_OversizedBodyBehindAuthAnswers401 pins the ORDER of the two guards on the v2
// create chain: the body-size guard sits AFTER auth, so a tokenless caller is answered 401 and is
// never told what body size this endpoint accepts. That ordering is an information-disclosure
// property, and moving the guard ahead of auth — the natural "reject early" optimisation — is
// invisible to every other guard test, because they all build the app with auth DISABLED, and
// invisible to the auth test, which posts a nil body.
//
// The assertion is therefore two-sided: the status must be 401, AND the answer must carry no trace
// of the payload-too-large code. A 413 alone would already be the leak.
func TestV2CreateOps_OversizedBodyBehindAuthAnswers401(t *testing.T) {
	// NOT parallel: process-global huma state.
	// Address must be non-empty so Authorize enforces the token check (it is never dialed: a
	// missing token short-circuits with 401 first).
	app := registerV2TransactionRoutesForTest(&middleware.AuthClient{Enabled: true, Address: "http://auth.invalid"})

	// One byte past the declared ceiling, so the guard would fire if it ran at all.
	oversized := oversizedV2CreateBody(v2CreateMaxBodyBytes + 1)

	for _, rt := range v2Routes {
		if !rt.hasBody {
			continue
		}

		t.Run(rt.action, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, concreteV2Path(rt.fiberPath), strings.NewReader(oversized))
			req.Header.Set(fiber.HeaderContentType, fiber.MIMEApplicationJSON)

			resp, err := app.Test(req, fiber.TestConfig{Timeout: 0})
			require.NoError(t, err)

			defer func() { _ = resp.Body.Close() }()

			raw, err := io.ReadAll(resp.Body)
			require.NoError(t, err)

			assert.Equalf(t, http.StatusUnauthorized, resp.StatusCode,
				"an oversized TOKENLESS v2 %s request must be answered by auth, not by the body guard; got: %s", rt.action, raw)

			assert.NotContainsf(t, string(raw), cn.ErrPayloadTooLarge.Error(),
				"the v2 %s answer to an unauthenticated caller must not disclose the body-size rejection", rt.action)

			var problem map[string]any
			if json.Unmarshal(raw, &problem) == nil {
				assert.NotEqualf(t, cn.ErrPayloadTooLarge.Error(), problem["code"],
					"the v2 %s answer to an unauthenticated caller must not carry the payload-too-large code", rt.action)
			}
		})
	}
}

// TestHuma_CreateTransactionV2_AdvancedBodyKeepsPerActionIdempotencySource is the idempotency
// regression for the advanced form: an advanced body is just another byte sequence in the
// SAME hash source each action already used, so direct still hashes the bare body and every
// other action still hashes its discriminator + NUL + body. It probes the same first-repo
// touch as the single-leg idempotency tests (TransactionRedisRepo.SetNX, whose internalKey embeds
// the hash source when no X-Idempotency header is sent). A per-action cross-check asserts no
// action keys off another action's source.
func TestHuma_CreateTransactionV2_AdvancedBodyKeepsPerActionIdempotencySource(t *testing.T) {
	// NOT parallel: process-global huma state.
	for _, tc := range v2CreateActionCases() {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			t.Cleanup(ctrl.Finish)

			var gotKey string

			handler := captureSetNXKey(t, ctrl, &gotKey, "{}")
			app := buildHumaV2ActionApp(t, tc.route, tc.op(handler))

			resp := postActionV2(t, app, tc.route, v2AdvancedBody)
			defer func() { _ = resp.Body.Close() }()

			wantSource := v2AdvancedBody
			if tc.wantDisc != "" {
				wantSource = tc.wantDisc + "\x00" + v2AdvancedBody
			}

			assert.Contains(t, gotKey, libCommons.HashSHA256(wantSource),
				"the advanced %s body must key idempotency off the SAME source shape the single-leg body uses; got internalKey=%q", tc.name, gotKey)

			for _, other := range v2CreateActionCases() {
				if other.wantDisc == tc.wantDisc {
					continue
				}

				otherSource := v2AdvancedBody
				if other.wantDisc != "" {
					otherSource = other.wantDisc + "\x00" + v2AdvancedBody
				}

				assert.NotContains(t, gotKey, libCommons.HashSHA256(otherSource),
					"the advanced %s body must NOT key off the %s action's source", tc.name, other.name)
			}

			assert.Equal(t, http.StatusCreated, resp.StatusCode, "a losing %s claim with a cached canonical value replays → 201", tc.name)
		})
	}
}
