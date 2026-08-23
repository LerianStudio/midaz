// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	libCommons "github.com/LerianStudio/lib-commons/v6/commons"

	openapi "github.com/LerianStudio/lib-commons/v6/commons/net/http/openapi"
	libProblem "github.com/LerianStudio/lib-commons/v6/commons/net/http/problem"
	libLog "github.com/LerianStudio/lib-observability/v2/log"
	"github.com/gofiber/fiber/v3"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	ledgerMiddleware "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/http/in/middleware"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	pkgHTTP "github.com/LerianStudio/midaz/v4/pkg/net/http"
)

// buildHumaTransactionApp mounts the twelve transaction Huma operations on a /v1
// group, mirroring the production wiring in unified-server.go: problem.Install() runs
// before any huma.Register, WithRecover is the first middleware so a panic in a handler
// unwinds to a 500 attributed to the running subtest instead of killing the test process,
// the Huma API is built with openapi.New over a /v1 group, an auth-shim middleware stands
// in for auth.Authorize("midaz","transactions",verb) + tenant PostAuthMiddlewares, and
// http.ParseUUIDPathParameters("transaction") + RegisterTransactionRoutes attach the chain.
//
// MUST-NOT-PARALLELIZE (same rationale as buildHumaCountApp/buildHumaHolderApp):
// libProblem.Install() swaps the process-global huma.NewError hook and Huma validation
// uses process-global sync.Pools — concurrent builds/requests cross-contaminate. These
// tests are sub-second; keep them sequential.
// mountTransactionRoutes mounts the twelve transaction routes on group in the order
// production registers them, with ParseUUIDPathParameters as Fiber middleware ahead of
// each Huma terminal. Any extra middlewares run before parse. Shared with the
// integration harnesses so a route added to production is added in exactly one place
// here — Fiber v3 types its handler argument as any, so a mount that drifts from the
// registrar still compiles and only fails when the route is hit.
func mountTransactionRoutes(group fiber.Router, extra ...fiber.Handler) {
	parse := pkgHTTP.ParseUUIDPathParameters("transaction")
	base := "/organizations/:organization_id/ledgers/:ledger_id/transactions"

	chain := make([]any, 0, len(extra)+1)
	for _, mw := range extra {
		chain = append(chain, mw)
	}

	chain = append(chain, parse)

	post := func(path string) { group.Post(path, chain[0], chain[1:]...) }

	post(base + "/json")
	post(base + "/inflow")
	post(base + "/outflow")
	post(base + "/annotation")
	post(base + "/block")
	post(base + "/unblock")
	post(base + "/:transaction_id/commit")
	post(base + "/:transaction_id/cancel")
	post(base + "/:transaction_id/revert")
	group.Patch(base+"/:transaction_id", chain[0], chain[1:]...)
	group.Get(base+"/:transaction_id", chain[0], chain[1:]...)
	group.Get(base, chain[0], chain[1:]...)
}

func buildHumaTransactionApp(t *testing.T, handler *TransactionHandler, authOK bool) *fiber.App {
	t.Helper()

	f := fiber.New(fiber.Config{
		ErrorHandler: pkgHTTP.CanonicalFiberErrorHandler,
	})

	// Mirror production: ErrorEnvelope is registered AHEAD of WithRecover so a
	// recovered panic's 500 body is reshaped for its route version too. Registering
	// it after would leave the panic path on the /v2 envelope here while production
	// serves the /v1 one.
	f.Use(ledgerMiddleware.ErrorEnvelope())

	f.Use(pkgHTTP.WithRecover(pkgHTTP.WithRecoverLogger(&libLog.GoLogger{})))

	libProblem.Install()
	pkgHTTP.InstallHumaFrameworkErrors()

	apiV1 := f.Group("/v1")

	apiV1.Use(func(c fiber.Ctx) error {
		if !authOK {
			return pkgHTTP.Unauthorized(c, "0001", "Unauthorized", "auth required")
		}

		return c.Next()
	})

	hAPI := openapi.New(f, apiV1, openapi.Config{Title: "ledger-test", Version: "test", Servers: []string{"/v1"}})

	// Mirror unified-server.go: the transaction Out (transaction.Transaction, nesting
	// operation.Operation → operation.{Status,Balance,Amount}) collides on the bare
	// schema names "Status"/"Balance"/"Amount" with the mmodel/transaction types already
	// on the shared registry. InstallLedgerSchemaNamer qualifies the operation-package
	// types ("Operation" prefix) and MUST run after openapi.New and BEFORE any
	// huma.Register (the registry namer is captured lazily on first registration).
	pkgHTTP.InstallLedgerSchemaNamer(hAPI)

	// Mirror the production chain: ParseUUIDPathParameters runs as a Fiber middleware
	// (no terminal handler) before the Huma terminal on each transaction route.
	mountTransactionRoutes(apiV1)

	RegisterTransactionRoutes(hAPI, handler)

	return f
}

// bareTransactionHandler is a handler with no wired repos. It is enough to prove the
// transport boundary (path-param validation, body decode/validate, auth) rejects BEFORE
// any service call — the deep money-path behavior is covered by the mock-backed tests
// over the cores (transaction_test.go, transaction_state_handlers_test.go et al.).
func bareTransactionHandler() *TransactionHandler {
	return &TransactionHandler{}
}

// createOpPaths enumerates the four body-shaped CREATE ops so the shared assertions run
// over every create shell (all four route to the same createTransaction core).
var createOpPaths = []string{"json", "inflow", "outflow", "annotation"}

// humaTransactionURL builds a request path against the /v1 group the harness mounts.
// suffix is appended to ".../transactions" (e.g. "/json", "/"+id+"/commit", "?limit=5").
func humaTransactionURL(orgID, ledgerID uuid.UUID, suffix string) string {
	return "/v1/organizations/" + orgID.String() + "/ledgers/" + ledgerID.String() + "/transactions" + suffix
}

func TestCreateTransaction_BadUUID_Canonical400(t *testing.T) {
	// NOT parallel: buildHumaTransactionApp mutates process-global huma state.
	orgID := uuid.Must(libCommons.GenerateUUIDv7())

	for _, op := range createOpPaths {
		t.Run(op, func(t *testing.T) {
			handler := bareTransactionHandler()
			app := buildHumaTransactionApp(t, handler, true)

			// ledger id is not a UUID: ParseUUIDPathParameters rejects with the canonical
			// 0065 / 400 BEFORE the Huma terminal — no native Huma 422, no service call.
			url := "/v1/organizations/" + orgID.String() + "/ledgers/not-a-uuid/transactions/" + op
			req := httptest.NewRequest(http.MethodPost, url, strings.NewReader(`{"send":{}}`))
			req.Header.Set("Content-Type", "application/json")

			resp, err := app.Test(req, fiber.TestConfig{Timeout: 0})
			require.NoError(t, err)
			defer func() { _ = resp.Body.Close() }()

			assert.Equal(t, http.StatusBadRequest, resp.StatusCode, "bad path UUID stays canonical 400 — no native Huma 422")
		})
	}
}

func TestCreateTransaction_MalformedBody_Canonical400(t *testing.T) {
	// NOT parallel: process-global huma state.
	orgID := uuid.Must(libCommons.GenerateUUIDv7())
	ledgerID := uuid.Must(libCommons.GenerateUUIDv7())

	for _, op := range createOpPaths {
		t.Run(op, func(t *testing.T) {
			handler := bareTransactionHandler()
			app := buildHumaTransactionApp(t, handler, true)

			// Malformed JSON: http.DecodeAndValidate (the SAME validator the Fiber WithBody
			// decorator runs) rejects with the canonical 400 — NOT a native Huma 422 — and
			// the service is never reached.
			url := "/v1/organizations/" + orgID.String() + "/ledgers/" + ledgerID.String() + "/transactions/" + op
			req := httptest.NewRequest(http.MethodPost, url, strings.NewReader(`{not-json`))
			req.Header.Set("Content-Type", "application/json")

			resp, err := app.Test(req, fiber.TestConfig{Timeout: 0})
			require.NoError(t, err)
			defer func() { _ = resp.Body.Close() }()

			body, _ := io.ReadAll(resp.Body)
			assert.Equal(t, http.StatusBadRequest, resp.StatusCode, "malformed body stays canonical 400 — no native Huma 422")
			// /v1 serves the v3 envelope: the human text is "message" and there is
			// no "status" member, the HTTP status carrying that on its own.
			assert.Contains(t, string(body), `"message"`, "error body must be the v1 envelope")
			assert.NotContains(t, string(body), `"status"`, "the v1 envelope carries no status member")
		})
	}
}

func TestCreateTransaction_AuthPreserved(t *testing.T) {
	// NOT parallel: process-global huma state.
	orgID := uuid.Must(libCommons.GenerateUUIDv7())
	ledgerID := uuid.Must(libCommons.GenerateUUIDv7())

	for _, op := range createOpPaths {
		t.Run(op, func(t *testing.T) {
			handler := bareTransactionHandler()
			app := buildHumaTransactionApp(t, handler, false)

			url := "/v1/organizations/" + orgID.String() + "/ledgers/" + ledgerID.String() + "/transactions/" + op
			req := httptest.NewRequest(http.MethodPost, url, strings.NewReader(`{"send":{}}`))
			req.Header.Set("Content-Type", "application/json")

			resp, err := app.Test(req, fiber.TestConfig{Timeout: 0})
			require.NoError(t, err)
			defer func() { _ = resp.Body.Close() }()

			assert.Equal(t, http.StatusUnauthorized, resp.StatusCode, "auth middleware must reject before Huma; no public route")
		})
	}
}

func TestCreateTransaction_EmptyBody_Canonical400(t *testing.T) {
	// NOT parallel: process-global huma state.
	orgID := uuid.Must(libCommons.GenerateUUIDv7())
	ledgerID := uuid.Must(libCommons.GenerateUUIDv7())

	for _, op := range createOpPaths {
		t.Run(op, func(t *testing.T) {
			handler := bareTransactionHandler()
			app := buildHumaTransactionApp(t, handler, true)

			// A zero-length body trips Huma's request-body precondition before
			// SkipValidateBody, so DecodeAndValidate never runs.
			// InstallHumaFrameworkErrors maps it onto the canonical 0094 the
			// malformed-body case above gets. Proves the mapping is transport-wide,
			// not organization-specific. Service never reached.
			url := "/v1/organizations/" + orgID.String() + "/ledgers/" + ledgerID.String() + "/transactions/" + op
			req := httptest.NewRequest(http.MethodPost, url, strings.NewReader(""))
			req.Header.Set("Content-Type", "application/json")

			resp, err := app.Test(req, fiber.TestConfig{Timeout: 0})
			require.NoError(t, err)
			defer func() { _ = resp.Body.Close() }()

			respBody, _ := io.ReadAll(resp.Body)

			assert.Equal(t, http.StatusBadRequest, resp.StatusCode, "empty body stays 400 — no 500, no native 422")

			var got map[string]any
			require.NoError(t, json.Unmarshal(respBody, &got), "body: %s", string(respBody))
			assert.Equal(t, constant.ErrInvalidRequestBody.Error(), got["code"], "empty-body code is 0094, not absent")
			assert.Equal(t, "Unmarshalling error", got["title"])
		})
	}
}

// stateOps enumerates the three id-only state ops (commit/cancel/revert) + patch for the
// shared bad-UUID / auth assertions.
var stateOpPaths = []string{"commit", "cancel", "revert"}

func TestStateTransaction_EmptyBodyStaysBodiless(t *testing.T) {
	// NOT parallel: process-global huma state.
	orgID := uuid.Must(libCommons.GenerateUUIDv7())
	ledgerID := uuid.Must(libCommons.GenerateUUIDv7())
	txID := uuid.Must(libCommons.GenerateUUIDv7())

	for _, op := range stateOpPaths {
		t.Run(op, func(t *testing.T) {
			handler := bareTransactionHandler()
			app := buildHumaTransactionApp(t, handler, true)

			// commit/cancel/revert carry no RawBody, so op.RequestBody stays nil and
			// Huma's empty-body precondition never fires. They must NOT be dragged into
			// the 0094 mapping — a bodiless mutation sending no body is legitimate.
			url := "/v1/organizations/" + orgID.String() + "/ledgers/" + ledgerID.String() + "/transactions/" + txID.String() + "/" + op
			req := httptest.NewRequest(http.MethodPost, url, nil)

			resp, err := app.Test(req, fiber.TestConfig{Timeout: 0})
			require.NoError(t, err)
			defer func() { _ = resp.Body.Close() }()

			respBody, _ := io.ReadAll(resp.Body)

			var got map[string]any
			require.NoError(t, json.Unmarshal(respBody, &got), "body must be JSON: %s", string(respBody))

			// Pin the exact downstream answer rather than merely "not 400": the bare
			// handler has no repos, so it unwinds through WithRecover to the canonical
			// 500/0046. That response is reachable ONLY from inside the handler — every
			// transport-boundary rejection (precondition 400, auth 401, routing 404)
			// short-circuits before it. So this is positive proof the request was NOT
			// intercepted by the empty-body precondition.
			assert.Equal(t, http.StatusInternalServerError, resp.StatusCode,
				"no body is valid for %s: the request must reach the handler, not be rejected at the transport", op)
			assert.Equal(t, constant.ErrInternalServer.Error(), got["code"],
				"bodiless state op reaches the handler and fails there on the bare wiring")
			assert.NotEqual(t, constant.ErrInvalidRequestBody.Error(), got["code"],
				"bodiless state op must never be rejected as a malformed body: %s", string(respBody))
		})
	}
}

func TestStateTransaction_BadUUID_Canonical400(t *testing.T) {
	// NOT parallel: process-global huma state.
	orgID := uuid.Must(libCommons.GenerateUUIDv7())
	ledgerID := uuid.Must(libCommons.GenerateUUIDv7())

	for _, op := range stateOpPaths {
		t.Run(op, func(t *testing.T) {
			handler := bareTransactionHandler()
			app := buildHumaTransactionApp(t, handler, true)

			// transaction_id is not a UUID: ParseUUIDPathParameters rejects with 0065 / 400
			// before the Huma terminal.
			url := "/v1/organizations/" + orgID.String() + "/ledgers/" + ledgerID.String() + "/transactions/not-a-uuid/" + op
			req := httptest.NewRequest(http.MethodPost, url, nil)

			resp, err := app.Test(req, fiber.TestConfig{Timeout: 0})
			require.NoError(t, err)
			defer func() { _ = resp.Body.Close() }()

			assert.Equal(t, http.StatusBadRequest, resp.StatusCode, "bad transaction_id stays canonical 400")
		})
	}
}

func TestStateTransaction_AuthPreserved(t *testing.T) {
	// NOT parallel: process-global huma state.
	orgID := uuid.Must(libCommons.GenerateUUIDv7())
	ledgerID := uuid.Must(libCommons.GenerateUUIDv7())
	txID := uuid.Must(libCommons.GenerateUUIDv7())

	for _, op := range stateOpPaths {
		t.Run(op, func(t *testing.T) {
			handler := bareTransactionHandler()
			app := buildHumaTransactionApp(t, handler, false)

			url := "/v1/organizations/" + orgID.String() + "/ledgers/" + ledgerID.String() + "/transactions/" + txID.String() + "/" + op
			req := httptest.NewRequest(http.MethodPost, url, nil)

			resp, err := app.Test(req, fiber.TestConfig{Timeout: 0})
			require.NoError(t, err)
			defer func() { _ = resp.Body.Close() }()

			assert.Equal(t, http.StatusUnauthorized, resp.StatusCode, "auth middleware must reject before Huma")
		})
	}
}

func TestUpdateTransaction_BadUUID_Canonical400(t *testing.T) {
	// NOT parallel: process-global huma state.
	orgID := uuid.Must(libCommons.GenerateUUIDv7())
	ledgerID := uuid.Must(libCommons.GenerateUUIDv7())

	handler := bareTransactionHandler()
	app := buildHumaTransactionApp(t, handler, true)

	url := "/v1/organizations/" + orgID.String() + "/ledgers/" + ledgerID.String() + "/transactions/not-a-uuid"
	req := httptest.NewRequest(http.MethodPatch, url, strings.NewReader(`{"description":"x"}`))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req, fiber.TestConfig{Timeout: 0})
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	assert.Equal(t, http.StatusBadRequest, resp.StatusCode, "bad transaction_id stays canonical 400 on PATCH")
}

func TestUpdateTransaction_MalformedBody_Canonical400(t *testing.T) {
	// NOT parallel: process-global huma state.
	orgID := uuid.Must(libCommons.GenerateUUIDv7())
	ledgerID := uuid.Must(libCommons.GenerateUUIDv7())
	txID := uuid.Must(libCommons.GenerateUUIDv7())

	handler := bareTransactionHandler()
	app := buildHumaTransactionApp(t, handler, true)

	url := "/v1/organizations/" + orgID.String() + "/ledgers/" + ledgerID.String() + "/transactions/" + txID.String()
	req := httptest.NewRequest(http.MethodPatch, url, strings.NewReader(`{not-json`))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req, fiber.TestConfig{Timeout: 0})
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	assert.Equal(t, http.StatusBadRequest, resp.StatusCode, "malformed PATCH body stays canonical 400 — no native Huma 422")
}

func TestGetTransaction_BadUUID_Canonical400(t *testing.T) {
	// NOT parallel: process-global huma state.
	orgID := uuid.Must(libCommons.GenerateUUIDv7())
	ledgerID := uuid.Must(libCommons.GenerateUUIDv7())

	handler := bareTransactionHandler()
	app := buildHumaTransactionApp(t, handler, true)

	url := "/v1/organizations/" + orgID.String() + "/ledgers/" + ledgerID.String() + "/transactions/not-a-uuid"
	req := httptest.NewRequest(http.MethodGet, url, nil)

	resp, err := app.Test(req, fiber.TestConfig{Timeout: 0})
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	assert.Equal(t, http.StatusBadRequest, resp.StatusCode, "bad transaction_id stays canonical 400 on GET-by-id")
}

func TestGetAllTransactions_AuthPreserved(t *testing.T) {
	// NOT parallel: process-global huma state.
	orgID := uuid.Must(libCommons.GenerateUUIDv7())
	ledgerID := uuid.Must(libCommons.GenerateUUIDv7())

	handler := bareTransactionHandler()
	app := buildHumaTransactionApp(t, handler, false)

	url := "/v1/organizations/" + orgID.String() + "/ledgers/" + ledgerID.String() + "/transactions"
	req := httptest.NewRequest(http.MethodGet, url, nil)

	resp, err := app.Test(req, fiber.TestConfig{Timeout: 0})
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode, "auth middleware must reject before Huma on list")
}

func TestGetAllTransactions_BadQueryParam_Canonical400(t *testing.T) {
	// NOT parallel: process-global huma state.
	orgID := uuid.Must(libCommons.GenerateUUIDv7())
	ledgerID := uuid.Must(libCommons.GenerateUUIDv7())

	handler := bareTransactionHandler()
	app := buildHumaTransactionApp(t, handler, true)

	// An out-of-range limit is rejected by http.ValidateParameters with the canonical 400
	// — no native Huma 422 — before any service call.
	url := "/v1/organizations/" + orgID.String() + "/ledgers/" + ledgerID.String() + "/transactions?limit=not-a-number"
	req := httptest.NewRequest(http.MethodGet, url, nil)

	resp, err := app.Test(req, fiber.TestConfig{Timeout: 0})
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	assert.Equal(t, http.StatusBadRequest, resp.StatusCode, "bad query param stays canonical 400 — no native Huma 422")
}
