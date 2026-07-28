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

	"github.com/LerianStudio/lib-auth/v3/auth/middleware"
	openapi "github.com/LerianStudio/lib-commons/v6/commons/net/http/openapi"
	libProblem "github.com/LerianStudio/lib-commons/v6/commons/net/http/problem"
	libLog "github.com/LerianStudio/lib-observability/v2/log"
	"github.com/gofiber/fiber/v3"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

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
// path is the Task 1.3.3 integration+parity test.
//
// MUST-NOT-PARALLELIZE (same rationale as buildHumaTransactionApp): libProblem.Install()
// swaps the process-global huma.NewError hook and Huma validation uses process-global
// sync.Pools — concurrent builds/requests cross-contaminate.
func buildHumaV2DirectApp(t *testing.T, handler *TransactionHandler) *fiber.App {
	t.Helper()

	app := fiber.New(fiber.Config{
		ErrorHandler: pkgHTTP.CanonicalFiberErrorHandler,
	})

	app.Use(pkgHTTP.WithRecover(pkgHTTP.WithRecoverLogger(&libLog.GoLogger{})))

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
// committed transaction result is asserted by the Task 1.3.3 integration+parity test.
func TestCreateTransactionDirectV2Huma_ValidBodyEntersFunnel(t *testing.T) {
	// NOT parallel: process-global huma state.
	app := buildHumaV2DirectApp(t, &TransactionHandler{})

	resp := postDirectV2(t, app, `{"description":"v2 direct","asset":"BRL","amount":"100","from":"@src","to":"@dst"}`)
	defer func() { _ = resp.Body.Close() }()

	assert.Equal(t, http.StatusInternalServerError, resp.StatusCode,
		"valid body must clear the transport/translate boundary and enter the funnel (unwired repos → recovered 500; committed path is Task 1.3.3)")
}
