// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/LerianStudio/lib-auth/v3/auth/middleware"
	libCommons "github.com/LerianStudio/lib-commons/v6/commons"
	openapi "github.com/LerianStudio/lib-commons/v6/commons/net/http/openapi"
	libProblem "github.com/LerianStudio/lib-commons/v6/commons/net/http/problem"
	libObservability "github.com/LerianStudio/lib-observability/v2"
	libLog "github.com/LerianStudio/lib-observability/v2/log"
	"github.com/danielgtaylor/huma/v2"
	"github.com/gofiber/fiber/v3"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	pkgHTTP "github.com/LerianStudio/midaz/v4/pkg/net/http"
)

// buildHumaV2DirectApp mounts the v2 `direct` transaction op through the SAME
// production seam (RegisterTransactionV2RoutesToApp) the unified server uses, on a
// fresh Fiber app + its own /v2 Huma contract. It mirrors buildHumaTransactionApp:
// problem.Install() before any huma.Register, WithRecover as the first middleware so
// a nil-repo panic in the unwired funnel unwinds to a 500 (not a dropped connection),
// the ledger schema namer installed after openapi.New and before registration (the v2
// output embeds transaction.Transaction, which nests operation.{Status,Balance,Amount}
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

// directV2ConcretePath builds the concrete /v2 direct path for a random org+ledger so
// ParseUUIDPathParameters passes and dispatch reaches the terminal.
func directV2ConcretePath() string {
	return "/v2/organizations/" + uuid.New().String() + "/ledgers/" + uuid.New().String() + "/transactions/direct"
}

// postDirectV2 issues an authenticated POST to the v2 direct route with the given JSON body.
func postDirectV2(t *testing.T, app *fiber.App, body string) *http.Response {
	t.Helper()

	req := httptest.NewRequest(http.MethodPost, directV2ConcretePath(), strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req, fiber.TestConfig{Timeout: 0})
	require.NoError(t, err)

	return resp
}

// TestCreateTransactionDirectV2Huma_MalformedBody_400 proves the real handler decodes
// the flat v2 body through http.DecodeAndValidate (the SAME validator the v1 create
// ops run): malformed JSON is the canonical 400 RFC 9457 problem — never a native Huma
// 422 and never the 501 stub.
func TestCreateTransactionDirectV2Huma_MalformedBody_400(t *testing.T) {
	// NOT parallel: process-global huma state.
	app := buildHumaV2DirectApp(t, &TransactionHandler{})

	resp := postDirectV2(t, app, `{not-json`)
	defer func() { _ = resp.Body.Close() }()

	body, _ := io.ReadAll(resp.Body)
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode, "malformed v2 body stays canonical 400 — no native Huma 422, no 501 stub")
	assert.Contains(t, string(body), "status", "error body must be the RFC 9457 problem envelope")
}

// TestCreateTransactionDirectV2Huma_MalformedRouteUUID_400 pins the route-UUID hygiene
// contract: routeId is an optional *string on the flat v2 input, so a malformed route
// UUID MUST surface as a clean 400 at the decode boundary — never fall through to a deep
// 500 in the funnel.
func TestCreateTransactionDirectV2Huma_MalformedRouteUUID_400(t *testing.T) {
	// NOT parallel: process-global huma state.
	app := buildHumaV2DirectApp(t, &TransactionHandler{})

	resp := postDirectV2(t, app, `{"asset":"BRL","amount":"100","from":"@src","to":"@dst","routeId":"not-a-uuid"}`)
	defer func() { _ = resp.Body.Close() }()

	assert.Equal(t, http.StatusBadRequest, resp.StatusCode, "malformed routeId must be a clean 400, not a deep 500")
}

// TestCreateTransactionDirectV2Huma_MalformedOperationRouteUUID_400 mirrors the routeId
// hygiene for the per-leg operationRouteId.
func TestCreateTransactionDirectV2Huma_MalformedOperationRouteUUID_400(t *testing.T) {
	// NOT parallel: process-global huma state.
	app := buildHumaV2DirectApp(t, &TransactionHandler{})

	resp := postDirectV2(t, app, `{"asset":"BRL","amount":"100","from":"@src","to":"@dst","operationRouteId":"not-a-uuid"}`)
	defer func() { _ = resp.Body.Close() }()

	assert.Equal(t, http.StatusBadRequest, resp.StatusCode, "malformed operationRouteId must be a clean 400, not a deep 500")
}

// TestCreateTransactionDirectV2Huma_Ambiguous_422 proves a Translate business error
// (from == to) maps to the canonical 422 RFC 9457 problem (span stays green) — the
// handler decodes, translates, and surfaces the business error without reaching the
// funnel.
func TestCreateTransactionDirectV2Huma_Ambiguous_422(t *testing.T) {
	// NOT parallel: process-global huma state.
	app := buildHumaV2DirectApp(t, &TransactionHandler{})

	resp := postDirectV2(t, app, `{"asset":"BRL","amount":"100","from":"@same","to":"@same"}`)
	defer func() { _ = resp.Body.Close() }()

	assert.Equal(t, http.StatusUnprocessableEntity, resp.StatusCode, "source == destination is a Translate business error → 422")
}

// TestCreateTransactionDirectV2Huma_NonPositiveAmount_422 proves the second Translate
// business branch (non-positive amount) also maps to a canonical 422.
func TestCreateTransactionDirectV2Huma_NonPositiveAmount_422(t *testing.T) {
	// NOT parallel: process-global huma state.
	app := buildHumaV2DirectApp(t, &TransactionHandler{})

	resp := postDirectV2(t, app, `{"asset":"BRL","amount":"0","from":"@src","to":"@dst"}`)
	defer func() { _ = resp.Body.Close() }()

	assert.Equal(t, http.StatusUnprocessableEntity, resp.StatusCode, "non-positive amount is a Translate business error → 422")
}

// TestCreateTransactionDirectV2Huma_ValidBodyEntersFunnel proves the happy-path wiring
// up to the funnel: a fully valid flat body passes decode + Translate(false) and is
// handed to the SAME createTransaction funnel the v1 create ops use. With a bare handler
// the funnel's first repository call has no wired dependency, so WithRecover maps the
// resulting panic to a 500 — proving the request progressed PAST the transport/translate
// boundary into the funnel (never the 501 stub, never a decode/translate 4xx). The
// committed transaction result is asserted by the integration+parity test.
func TestCreateTransactionDirectV2Huma_ValidBodyEntersFunnel(t *testing.T) {
	// NOT parallel: process-global huma state.
	app := buildHumaV2DirectApp(t, &TransactionHandler{})

	resp := postDirectV2(t, app, `{"description":"v2 direct","asset":"BRL","amount":"100","from":"@src","to":"@dst"}`)
	defer func() { _ = resp.Body.Close() }()

	assert.Equal(t, http.StatusInternalServerError, resp.StatusCode,
		"valid body must clear the transport/translate boundary and enter the funnel (unwired repos → recovered 500; committed path is the integration+parity test)")
}

// buildHumaV2HoldApp mounts the v2 `hold` transaction op alone on a fresh Fiber app + its
// own /v2 Huma contract, mirroring buildHumaV2DirectApp. Wiring the single terminal — with
// the SAME Fiber auth/tenant/ParseUUIDPathParameters chain and SkipValidateBody Huma op the
// production route carries — keeps a failure attributable to the hold handler rather than to
// any sibling op. Same MUST-NOT-PARALLELIZE rationale as buildHumaV2DirectApp:
// libProblem.Install() and Huma validation use process-global state.
func buildHumaV2HoldApp(t *testing.T, handler *TransactionHandler) *fiber.App {
	t.Helper()

	app := fiber.New(fiber.Config{
		ErrorHandler: pkgHTTP.CanonicalFiberErrorHandler,
	})

	app.Use(pkgHTTP.WithRecover(pkgHTTP.WithRecoverLogger(&libLog.GoLogger{})))

	libProblem.Install()

	apiV2 := app.Group("/v2")

	humaAPI := openapi.New(app, apiV2, openapi.Config{Title: "ledger-test-v2-hold", Version: "test", Servers: []string{"/v2"}})
	pkgHTTP.InstallLedgerSchemaNamer(humaAPI)

	const holdMiddlewarePath = "/organizations/:organization_id/ledgers/:ledger_id/transactions/hold"

	parse := pkgHTTP.ParseUUIDPathParameters("transaction")
	routePost(apiV2, holdMiddlewarePath, protectedMidaz(&middleware.AuthClient{Enabled: false}, "transactions", "post", nil, parse))

	huma.Register(humaAPI, huma.Operation{
		OperationID:      "createTransactionHoldV2",
		Method:           http.MethodPost,
		Path:             "/organizations/{organization_id}/ledgers/{ledger_id}/transactions/hold",
		Summary:          "Create a Transaction using the v2 hold model",
		Tags:             []string{"Transactions"},
		Security:         secTransactionBearer,
		SkipValidateBody: true,
		DefaultStatus:    http.StatusCreated,
	}, handler.CreateTransactionHoldV2Huma)

	return app
}

// holdV2ConcretePath builds the concrete /v2 hold path for a random org+ledger so
// ParseUUIDPathParameters passes and dispatch reaches the terminal.
func holdV2ConcretePath() string {
	return "/v2/organizations/" + uuid.New().String() + "/ledgers/" + uuid.New().String() + "/transactions/hold"
}

// postHoldV2 issues an authenticated POST to the v2 hold route with the given JSON body.
func postHoldV2(t *testing.T, app *fiber.App, body string) *http.Response {
	t.Helper()

	req := httptest.NewRequest(http.MethodPost, holdV2ConcretePath(), strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req, fiber.TestConfig{Timeout: 0})
	require.NoError(t, err)

	return resp
}

// TestCreateTransactionHoldV2Huma_MalformedBody_400 proves the hold handler decodes the
// flat v2 body through the SAME http.DecodeAndValidate the direct handler runs: malformed
// JSON is the canonical 400 RFC 9457 problem, never a native Huma 422 nor a 501 stub.
func TestCreateTransactionHoldV2Huma_MalformedBody_400(t *testing.T) {
	// NOT parallel: process-global huma state.
	app := buildHumaV2HoldApp(t, &TransactionHandler{})

	resp := postHoldV2(t, app, `{not-json`)
	defer func() { _ = resp.Body.Close() }()

	body, _ := io.ReadAll(resp.Body)
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode, "malformed v2 hold body stays canonical 400")
	assert.Contains(t, string(body), "status", "error body must be the RFC 9457 problem envelope")
}

// TestCreateTransactionHoldV2Huma_Ambiguous_422 proves a Translate business error
// (from == to) on the hold action maps to the canonical 422 RFC 9457 problem (span stays
// green) — the shared helper decodes, translates with pending=true, and surfaces the
// business error before reaching the funnel.
func TestCreateTransactionHoldV2Huma_Ambiguous_422(t *testing.T) {
	// NOT parallel: process-global huma state.
	app := buildHumaV2HoldApp(t, &TransactionHandler{})

	resp := postHoldV2(t, app, `{"asset":"BRL","amount":"100","from":"@same","to":"@same"}`)
	defer func() { _ = resp.Body.Close() }()

	assert.Equal(t, http.StatusUnprocessableEntity, resp.StatusCode, "source == destination is a Translate business error → 422")
}

// TestCreateTransactionHoldV2Huma_ValidBodyEntersFunnel proves the hold happy-path wiring
// up to the funnel: a fully valid flat body passes decode + Translate(true) and is handed
// to the SAME createTransaction funnel. With a bare handler the funnel's first repository
// call has no wired dependency, so WithRecover maps the resulting panic to a 500 — proving
// the request progressed PAST the transport/translate boundary into the funnel.
func TestCreateTransactionHoldV2Huma_ValidBodyEntersFunnel(t *testing.T) {
	// NOT parallel: process-global huma state.
	app := buildHumaV2HoldApp(t, &TransactionHandler{})

	resp := postHoldV2(t, app, `{"description":"v2 hold","asset":"BRL","amount":"100","from":"@src","to":"@dst"}`)
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
	app := buildHumaV2HoldApp(t, handler)

	resp := postHoldV2(t, app, v2DirectBody)
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

	out, err := handler.createTransactionV2(ctx, uuid.New().String(), uuid.New().String(), []byte(v2DirectBody), "", "", false, "")

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
	tx, err := decodeAndBuildV2Transaction([]byte(v2DirectBody), false, "BLOCK")
	require.NoError(t, err)

	assert.Equal(t, "BLOCK", tx.OperationTypeOverride,
		"a non-empty override must be stamped onto the transaction handed to the funnel")
	assert.False(t, tx.Pending,
		"the block action is non-pending; Translate must carry pending=false through to the funnel")
}

// buildHumaV2ActionApp mounts a single named v2 create action on a fresh Fiber app + its own
// /v2 Huma contract. Wiring one terminal at a time — with the SAME Fiber
// auth/tenant/ParseUUIDPathParameters chain and SkipValidateBody Huma op the production route
// carries — keeps a failure attributable to the handler under test rather than to any sibling
// op. Same MUST-NOT-PARALLELIZE rationale as buildHumaV2DirectApp: libProblem.Install() and
// Huma validation use process-global state.
func buildHumaV2ActionApp(t *testing.T, handler *TransactionHandler, action string, op func(context.Context, *CreateTransactionDirectV2InputHuma) (*CreateTransactionOutputHuma, error)) *fiber.App {
	t.Helper()

	app := fiber.New(fiber.Config{
		ErrorHandler: pkgHTTP.CanonicalFiberErrorHandler,
	})

	app.Use(pkgHTTP.WithRecover(pkgHTTP.WithRecoverLogger(&libLog.GoLogger{})))

	libProblem.Install()

	apiV2 := app.Group("/v2")

	humaAPI := openapi.New(app, apiV2, openapi.Config{Title: "ledger-test-v2-" + action, Version: "test", Servers: []string{"/v2"}})
	pkgHTTP.InstallLedgerSchemaNamer(humaAPI)

	middlewarePath := "/organizations/:organization_id/ledgers/:ledger_id/transactions/" + action

	parse := pkgHTTP.ParseUUIDPathParameters("transaction")
	routePost(apiV2, middlewarePath, protectedMidaz(&middleware.AuthClient{Enabled: false}, "transactions", "post", nil, parse))

	huma.Register(humaAPI, huma.Operation{
		OperationID:      "createTransaction" + strings.ToUpper(action[:1]) + action[1:] + "V2",
		Method:           http.MethodPost,
		Path:             "/organizations/{organization_id}/ledgers/{ledger_id}/transactions/" + action,
		Summary:          "Create a Transaction using the v2 " + action + " model",
		Tags:             []string{"Transactions"},
		Security:         secTransactionBearer,
		SkipValidateBody: true,
		DefaultStatus:    http.StatusCreated,
	}, op)

	return app
}

// postActionV2 issues an authenticated POST to a v2 action route for a random org+ledger so
// ParseUUIDPathParameters passes and dispatch reaches the terminal.
func postActionV2(t *testing.T, app *fiber.App, action, body string) *http.Response {
	t.Helper()

	path := "/v2/organizations/" + uuid.New().String() + "/ledgers/" + uuid.New().String() + "/transactions/" + action
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req, fiber.TestConfig{Timeout: 0})
	require.NoError(t, err)

	return resp
}

// TestCreateTransactionBlockV2Huma_MalformedBody_400 proves the block handler decodes the flat
// v2 body through the SAME http.DecodeAndValidate the direct/hold handlers run: malformed JSON
// is the canonical 400 RFC 9457 problem, never a native Huma 422 nor a 501 stub.
func TestCreateTransactionBlockV2Huma_MalformedBody_400(t *testing.T) {
	// NOT parallel: process-global huma state.
	app := buildHumaV2ActionApp(t, &TransactionHandler{}, "block", (&TransactionHandler{}).CreateTransactionBlockV2Huma)

	resp := postActionV2(t, app, "block", `{not-json`)
	defer func() { _ = resp.Body.Close() }()

	body, _ := io.ReadAll(resp.Body)
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode, "malformed v2 block body stays canonical 400")
	assert.Contains(t, string(body), "status", "error body must be the RFC 9457 problem envelope")
}

// TestCreateTransactionBlockV2Huma_Ambiguous_422 proves a Translate business error (from == to)
// on the block action maps to the canonical 422 RFC 9457 problem (span stays green) — the shared
// helper decodes, translates with pending=false, and surfaces the business error before the funnel.
func TestCreateTransactionBlockV2Huma_Ambiguous_422(t *testing.T) {
	// NOT parallel: process-global huma state.
	app := buildHumaV2ActionApp(t, &TransactionHandler{}, "block", (&TransactionHandler{}).CreateTransactionBlockV2Huma)

	resp := postActionV2(t, app, "block", `{"asset":"BRL","amount":"100","from":"@same","to":"@same"}`)
	defer func() { _ = resp.Body.Close() }()

	assert.Equal(t, http.StatusUnprocessableEntity, resp.StatusCode, "source == destination is a Translate business error → 422")
}

// TestCreateTransactionBlockV2Huma_ValidBodyEntersFunnel proves the block happy-path wiring up
// to the funnel: a fully valid flat body passes decode + Translate(false) and is handed to the
// SAME createTransaction funnel. With a bare handler the funnel's first repository call has no
// wired dependency, so WithRecover maps the resulting panic to a 500 — proving the request
// progressed PAST the transport/translate boundary into the funnel.
func TestCreateTransactionBlockV2Huma_ValidBodyEntersFunnel(t *testing.T) {
	// NOT parallel: process-global huma state.
	app := buildHumaV2ActionApp(t, &TransactionHandler{}, "block", (&TransactionHandler{}).CreateTransactionBlockV2Huma)

	resp := postActionV2(t, app, "block", `{"description":"v2 block","asset":"BRL","amount":"100","from":"@src","to":"@dst"}`)
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
	app := buildHumaV2ActionApp(t, handler, "block", handler.CreateTransactionBlockV2Huma)

	resp := postActionV2(t, app, "block", v2DirectBody)
	defer func() { _ = resp.Body.Close() }()

	assert.Contains(t, gotKey, libCommons.HashSHA256("BLOCK\x00"+v2DirectBody),
		"v2 block idempotency must be keyed by the BLOCK-discriminated raw v2 body; got internalKey=%q", gotKey)
	assert.NotContains(t, gotKey, libCommons.HashSHA256(v2DirectBody),
		"v2 block must NOT key off the bare body — that is the direct action's identity")
	assert.Equal(t, http.StatusCreated, resp.StatusCode, "a losing block claim with a cached canonical value replays → 201")
}

// TestCreateTransactionUnblockV2Huma_MalformedBody_400 mirrors the block malformed-body contract
// for the unblock action.
func TestCreateTransactionUnblockV2Huma_MalformedBody_400(t *testing.T) {
	// NOT parallel: process-global huma state.
	app := buildHumaV2ActionApp(t, &TransactionHandler{}, "unblock", (&TransactionHandler{}).CreateTransactionUnblockV2Huma)

	resp := postActionV2(t, app, "unblock", `{not-json`)
	defer func() { _ = resp.Body.Close() }()

	body, _ := io.ReadAll(resp.Body)
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode, "malformed v2 unblock body stays canonical 400")
	assert.Contains(t, string(body), "status", "error body must be the RFC 9457 problem envelope")
}

// TestCreateTransactionUnblockV2Huma_Ambiguous_422 mirrors the block business-error contract for
// the unblock action.
func TestCreateTransactionUnblockV2Huma_Ambiguous_422(t *testing.T) {
	// NOT parallel: process-global huma state.
	app := buildHumaV2ActionApp(t, &TransactionHandler{}, "unblock", (&TransactionHandler{}).CreateTransactionUnblockV2Huma)

	resp := postActionV2(t, app, "unblock", `{"asset":"BRL","amount":"100","from":"@same","to":"@same"}`)
	defer func() { _ = resp.Body.Close() }()

	assert.Equal(t, http.StatusUnprocessableEntity, resp.StatusCode, "source == destination is a Translate business error → 422")
}

// TestCreateTransactionUnblockV2Huma_ValidBodyEntersFunnel mirrors the block funnel-entry contract
// for the unblock action.
func TestCreateTransactionUnblockV2Huma_ValidBodyEntersFunnel(t *testing.T) {
	// NOT parallel: process-global huma state.
	app := buildHumaV2ActionApp(t, &TransactionHandler{}, "unblock", (&TransactionHandler{}).CreateTransactionUnblockV2Huma)

	resp := postActionV2(t, app, "unblock", `{"description":"v2 unblock","asset":"BRL","amount":"100","from":"@src","to":"@dst"}`)
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
	app := buildHumaV2ActionApp(t, handler, "unblock", handler.CreateTransactionUnblockV2Huma)

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

			tx, err := decodeAndBuildV2Transaction([]byte(v2DirectBody), false, tc.override)
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
func TestV2IdempotencyHashSource_DiscriminatesActions(t *testing.T) {
	t.Parallel()

	body := []byte(v2DirectBody)

	tests := []struct {
		name        string
		pending     bool
		override    string
		wantDisc    string
		wantHashSrc string
	}{
		{name: "direct stays bare body", pending: false, override: "", wantDisc: "", wantHashSrc: v2DirectBody},
		{name: "hold", pending: true, override: "", wantDisc: "HOLD", wantHashSrc: "HOLD\x00" + v2DirectBody},
		{name: "block", pending: false, override: "BLOCK", wantDisc: "BLOCK", wantHashSrc: "BLOCK\x00" + v2DirectBody},
		{name: "unblock", pending: false, override: "UNBLOCK", wantDisc: "UNBLOCK", wantHashSrc: "UNBLOCK\x00" + v2DirectBody},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tc.wantDisc, idempotencyActionDiscriminator(tc.pending, tc.override),
				"action discriminator mapping")
			assert.Equal(t, tc.wantHashSrc, v2IdempotencyHashSource(body, tc.pending, tc.override),
				"idempotency hash source")
		})
	}

	// Direct is byte-identical to the bare body: this exact invariant keeps the existing direct
	// idempotency tests green unchanged.
	assert.Equal(t, v2DirectBody, v2IdempotencyHashSource(body, false, ""),
		"direct's hash source MUST remain exactly the bare body")

	// No two actions collide.
	seen := map[string]string{}
	for _, tc := range tests {
		src := v2IdempotencyHashSource(body, tc.pending, tc.override)
		if prev, dup := seen[src]; dup {
			t.Fatalf("actions %q and %q share an idempotency hash source", prev, tc.name)
		}

		seen[src] = tc.name
	}
}
