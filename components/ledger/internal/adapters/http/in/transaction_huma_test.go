// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	openapi "github.com/LerianStudio/lib-commons/v5/commons/net/http/openapi"
	libProblem "github.com/LerianStudio/lib-commons/v5/commons/net/http/problem"
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	pkgHTTP "github.com/LerianStudio/midaz/v4/pkg/net/http"
)

// buildHumaTransactionApp mounts the ten migrated transaction Huma operations on a /v1
// group, mirroring the production wiring in unified-server.go: problem.Install() runs
// before any huma.Register, the Huma API is built with openapi.New over a /v1 group, an
// auth-shim middleware stands in for auth.Authorize("midaz","transactions",verb) + tenant
// PostAuthMiddlewares, and http.ParseUUIDPathParameters("transaction") +
// RegisterTransactionRoutes attach the chain. POST /transactions/dsl is deliberately NOT
// mounted (SUNSET 2026-08-01, stays pure Fiber).
//
// MUST-NOT-PARALLELIZE (same rationale as buildHumaCountApp/buildHumaHolderApp):
// libProblem.Install() swaps the process-global huma.NewError hook and Huma validation
// uses process-global sync.Pools — concurrent builds/requests cross-contaminate. These
// tests are sub-second; keep them sequential.
func buildHumaTransactionApp(t *testing.T, handler *TransactionHandler, authOK bool) *fiber.App {
	t.Helper()

	f := fiber.New(fiber.Config{
		DisableStartupMessage: true,
		ErrorHandler:          pkgHTTP.CanonicalFiberErrorHandler,
	})

	libProblem.Install()

	apiV1 := f.Group("/v1")

	apiV1.Use(func(c *fiber.Ctx) error {
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
	parse := pkgHTTP.ParseUUIDPathParameters("transaction")
	base := "/organizations/:organization_id/ledgers/:ledger_id/transactions"
	apiV1.Post(base+"/json", parse)
	apiV1.Post(base+"/inflow", parse)
	apiV1.Post(base+"/outflow", parse)
	apiV1.Post(base+"/annotation", parse)
	apiV1.Post(base+"/:transaction_id/commit", parse)
	apiV1.Post(base+"/:transaction_id/cancel", parse)
	apiV1.Post(base+"/:transaction_id/revert", parse)
	apiV1.Patch(base+"/:transaction_id", parse)
	apiV1.Get(base+"/:transaction_id", parse)
	apiV1.Get(base, parse)

	RegisterTransactionRoutes(hAPI, handler)

	return f
}

// bareTransactionHandler is a handler with no wired repos. It is enough to prove the
// transport boundary (path-param validation, body decode/validate, auth) rejects BEFORE
// any service call — the deep money-path behavior is covered by the existing Fiber-level
// tests over the untouched core (transaction_state_handlers_test.go et al.).
func bareTransactionHandler() *TransactionHandler {
	return &TransactionHandler{}
}

// createOpPaths enumerates the four migrated CREATE ops so the shared assertions run over
// every create shell (all four route to the same createTransaction core).
var createOpPaths = []string{"json", "inflow", "outflow", "annotation"}

func TestHuma_CreateTransaction_BadUUID_Canonical400(t *testing.T) {
	// NOT parallel: buildHumaTransactionApp mutates process-global huma state.
	orgID := uuid.New()

	for _, op := range createOpPaths {
		t.Run(op, func(t *testing.T) {
			handler := bareTransactionHandler()
			app := buildHumaTransactionApp(t, handler, true)

			// ledger id is not a UUID: ParseUUIDPathParameters rejects with the canonical
			// 0065 / 400 BEFORE the Huma terminal — no native Huma 422, no service call.
			url := "/v1/organizations/" + orgID.String() + "/ledgers/not-a-uuid/transactions/" + op
			req := httptest.NewRequest(http.MethodPost, url, strings.NewReader(`{"send":{}}`))
			req.Header.Set("Content-Type", "application/json")

			resp, err := app.Test(req, -1)
			require.NoError(t, err)
			defer func() { _ = resp.Body.Close() }()

			assert.Equal(t, http.StatusBadRequest, resp.StatusCode, "bad path UUID stays canonical 400 — no native Huma 422")
		})
	}
}

func TestHuma_CreateTransaction_MalformedBody_Canonical400(t *testing.T) {
	// NOT parallel: process-global huma state.
	orgID := uuid.New()
	ledgerID := uuid.New()

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

			resp, err := app.Test(req, -1)
			require.NoError(t, err)
			defer func() { _ = resp.Body.Close() }()

			body, _ := io.ReadAll(resp.Body)
			assert.Equal(t, http.StatusBadRequest, resp.StatusCode, "malformed body stays canonical 400 — no native Huma 422")
			// RFC 9457 problem+json shape from the shared HumaProblem path.
			assert.Contains(t, string(body), "status", "error body must be the RFC 9457 problem envelope")
		})
	}
}

func TestHuma_CreateTransaction_AuthPreserved(t *testing.T) {
	// NOT parallel: process-global huma state.
	orgID := uuid.New()
	ledgerID := uuid.New()

	for _, op := range createOpPaths {
		t.Run(op, func(t *testing.T) {
			handler := bareTransactionHandler()
			app := buildHumaTransactionApp(t, handler, false)

			url := "/v1/organizations/" + orgID.String() + "/ledgers/" + ledgerID.String() + "/transactions/" + op
			req := httptest.NewRequest(http.MethodPost, url, strings.NewReader(`{"send":{}}`))
			req.Header.Set("Content-Type", "application/json")

			resp, err := app.Test(req, -1)
			require.NoError(t, err)
			defer func() { _ = resp.Body.Close() }()

			assert.Equal(t, http.StatusUnauthorized, resp.StatusCode, "auth middleware must reject before Huma; no public route")
		})
	}
}

// stateOps enumerates the three id-only state ops (commit/cancel/revert) + patch for the
// shared bad-UUID / auth assertions.
var stateOpPaths = []string{"commit", "cancel", "revert"}

func TestHuma_StateTransaction_BadUUID_Canonical400(t *testing.T) {
	// NOT parallel: process-global huma state.
	orgID := uuid.New()
	ledgerID := uuid.New()

	for _, op := range stateOpPaths {
		t.Run(op, func(t *testing.T) {
			handler := bareTransactionHandler()
			app := buildHumaTransactionApp(t, handler, true)

			// transaction_id is not a UUID: ParseUUIDPathParameters rejects with 0065 / 400
			// before the Huma terminal.
			url := "/v1/organizations/" + orgID.String() + "/ledgers/" + ledgerID.String() + "/transactions/not-a-uuid/" + op
			req := httptest.NewRequest(http.MethodPost, url, nil)

			resp, err := app.Test(req, -1)
			require.NoError(t, err)
			defer func() { _ = resp.Body.Close() }()

			assert.Equal(t, http.StatusBadRequest, resp.StatusCode, "bad transaction_id stays canonical 400")
		})
	}
}

func TestHuma_StateTransaction_AuthPreserved(t *testing.T) {
	// NOT parallel: process-global huma state.
	orgID := uuid.New()
	ledgerID := uuid.New()
	txID := uuid.New()

	for _, op := range stateOpPaths {
		t.Run(op, func(t *testing.T) {
			handler := bareTransactionHandler()
			app := buildHumaTransactionApp(t, handler, false)

			url := "/v1/organizations/" + orgID.String() + "/ledgers/" + ledgerID.String() + "/transactions/" + txID.String() + "/" + op
			req := httptest.NewRequest(http.MethodPost, url, nil)

			resp, err := app.Test(req, -1)
			require.NoError(t, err)
			defer func() { _ = resp.Body.Close() }()

			assert.Equal(t, http.StatusUnauthorized, resp.StatusCode, "auth middleware must reject before Huma")
		})
	}
}

func TestHuma_UpdateTransaction_BadUUID_Canonical400(t *testing.T) {
	// NOT parallel: process-global huma state.
	orgID := uuid.New()
	ledgerID := uuid.New()

	handler := bareTransactionHandler()
	app := buildHumaTransactionApp(t, handler, true)

	url := "/v1/organizations/" + orgID.String() + "/ledgers/" + ledgerID.String() + "/transactions/not-a-uuid"
	req := httptest.NewRequest(http.MethodPatch, url, strings.NewReader(`{"description":"x"}`))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req, -1)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	assert.Equal(t, http.StatusBadRequest, resp.StatusCode, "bad transaction_id stays canonical 400 on PATCH")
}

func TestHuma_UpdateTransaction_MalformedBody_Canonical400(t *testing.T) {
	// NOT parallel: process-global huma state.
	orgID := uuid.New()
	ledgerID := uuid.New()
	txID := uuid.New()

	handler := bareTransactionHandler()
	app := buildHumaTransactionApp(t, handler, true)

	url := "/v1/organizations/" + orgID.String() + "/ledgers/" + ledgerID.String() + "/transactions/" + txID.String()
	req := httptest.NewRequest(http.MethodPatch, url, strings.NewReader(`{not-json`))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req, -1)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	assert.Equal(t, http.StatusBadRequest, resp.StatusCode, "malformed PATCH body stays canonical 400 — no native Huma 422")
}

func TestHuma_GetTransaction_BadUUID_Canonical400(t *testing.T) {
	// NOT parallel: process-global huma state.
	orgID := uuid.New()
	ledgerID := uuid.New()

	handler := bareTransactionHandler()
	app := buildHumaTransactionApp(t, handler, true)

	url := "/v1/organizations/" + orgID.String() + "/ledgers/" + ledgerID.String() + "/transactions/not-a-uuid"
	req := httptest.NewRequest(http.MethodGet, url, nil)

	resp, err := app.Test(req, -1)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	assert.Equal(t, http.StatusBadRequest, resp.StatusCode, "bad transaction_id stays canonical 400 on GET-by-id")
}

func TestHuma_GetAllTransactions_AuthPreserved(t *testing.T) {
	// NOT parallel: process-global huma state.
	orgID := uuid.New()
	ledgerID := uuid.New()

	handler := bareTransactionHandler()
	app := buildHumaTransactionApp(t, handler, false)

	url := "/v1/organizations/" + orgID.String() + "/ledgers/" + ledgerID.String() + "/transactions"
	req := httptest.NewRequest(http.MethodGet, url, nil)

	resp, err := app.Test(req, -1)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode, "auth middleware must reject before Huma on list")
}

func TestHuma_GetAllTransactions_BadQueryParam_Canonical400(t *testing.T) {
	// NOT parallel: process-global huma state.
	orgID := uuid.New()
	ledgerID := uuid.New()

	handler := bareTransactionHandler()
	app := buildHumaTransactionApp(t, handler, true)

	// An out-of-range limit is rejected by http.ValidateParameters (the SAME binder the
	// Fiber path runs) with the canonical 400 — no native Huma 422 — before any service call.
	url := "/v1/organizations/" + orgID.String() + "/ledgers/" + ledgerID.String() + "/transactions?limit=not-a-number"
	req := httptest.NewRequest(http.MethodGet, url, nil)

	resp, err := app.Test(req, -1)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	assert.Equal(t, http.StatusBadRequest, resp.StatusCode, "bad query param stays canonical 400 — no native Huma 422")
}
