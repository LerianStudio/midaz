// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	openapi "github.com/LerianStudio/lib-commons/v5/commons/net/http/openapi"
	libProblem "github.com/LerianStudio/lib-commons/v5/commons/net/http/problem"
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/services/composition"
	"github.com/LerianStudio/midaz/v4/pkg"
	cn "github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	pkgHTTP "github.com/LerianStudio/midaz/v4/pkg/net/http"
)

// buildHumaCompositionApp mounts the single composition Huma operation on a /v1
// group, faithfully mirroring the production wiring in unified-server.go:
// problem.Install() runs before any huma.Register, the Huma API is built with
// openapi.New over a /v1 group, an auth-shim middleware stands in for
// auth.Authorize("midaz","accounts","post") + tenant PostAuthMiddlewares, and
// http.ParseUUIDPathParameters("holder") + RegisterCompositionRoutes attach the
// chain. See asset_huma_test.go's buildHumaAssetApp for the full rationale.
//
// MUST-NOT-PARALLELIZE: libProblem.Install() swaps the process-global
// huma.NewError hook and Huma validation uses process-global sync.Pools —
// concurrent builds/requests cross-contaminate. These tests are sub-second; keep
// them sequential.
func buildHumaCompositionApp(t *testing.T, handler *CompositionHandler, authOK bool) *fiber.App {
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

	// The :id path param is the holder; ParseUUIDPathParameters("holder") validates
	// it (mirrors composition_routes.go). Registered group-relative on apiV1.
	parse := pkgHTTP.ParseUUIDPathParameters("holder")
	apiV1.Post("/organizations/:organization_id/ledgers/:ledger_id/holders/:id/accounts", parse)

	RegisterCompositionRoutes(hAPI, handler)

	return f
}

func compositionURL(orgID, ledgerID, holderID uuid.UUID) string {
	return "/v1/organizations/" + orgID.String() + "/ledgers/" + ledgerID.String() +
		"/holders/" + holderID.String() + "/accounts"
}

func validCompositionBody() []byte {
	body, _ := json.Marshal(map[string]any{
		"assetCode": "USD",
		"type":      "deposit",
	})

	return body
}

func TestHuma_CreateHolderAccount_Success(t *testing.T) {
	// NOT parallel: buildHumaCompositionApp mutates process-global huma state.
	orgID := uuid.New()
	ledgerID := uuid.New()
	holderID := uuid.New()

	createdAccount := &mmodel.Account{ID: uuid.New().String(), AssetCode: "USD", Type: "deposit"}

	handler := &CompositionHandler{Service: composition.NewService(
		stubAccountCreator{account: createdAccount},
		stubInstrumentCreator{},
	)}

	app := buildHumaCompositionApp(t, handler, true)

	req := httptest.NewRequest(http.MethodPost, compositionURL(orgID, ledgerID, holderID), bytes.NewReader(validCompositionBody()))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req, -1)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	respBody, _ := io.ReadAll(resp.Body)

	assert.Equal(t, http.StatusCreated, resp.StatusCode)
	assert.NotContains(t, string(respBody), "$schema", "SchemaLinkTransformer must be zeroed")
	assert.NotContains(t, string(respBody), "$ref")

	var got map[string]any
	require.NoError(t, json.Unmarshal(respBody, &got), "body: %s", string(respBody))
	require.Contains(t, got, "account")
	acc, ok := got["account"].(map[string]any)
	require.True(t, ok, "account object present")
	assert.Equal(t, createdAccount.ID, acc["id"])
}

func TestHuma_CreateHolderAccount_AuthPreserved(t *testing.T) {
	// NOT parallel: process-global huma state.
	orgID := uuid.New()
	ledgerID := uuid.New()
	holderID := uuid.New()

	// Service must never be reached: a rejected auth returns the ledger 401.
	handler := &CompositionHandler{Service: composition.NewService(stubAccountCreator{}, stubInstrumentCreator{})}

	app := buildHumaCompositionApp(t, handler, false)

	req := httptest.NewRequest(http.MethodPost, compositionURL(orgID, ledgerID, holderID), bytes.NewReader(validCompositionBody()))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req, -1)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode, "auth middleware must reject before Huma; no public route")
}

func TestHuma_CreateHolderAccount_ValidationError_Canonical400(t *testing.T) {
	// NOT parallel: process-global huma state.
	orgID := uuid.New()
	ledgerID := uuid.New()
	holderID := uuid.New()

	// Missing required assetCode/type -> imperative ValidateStruct -> canonical 400,
	// service never reached.
	handler := &CompositionHandler{Service: composition.NewService(stubAccountCreator{}, stubInstrumentCreator{})}

	app := buildHumaCompositionApp(t, handler, true)

	body, _ := json.Marshal(map[string]any{"name": "no asset code"})
	req := httptest.NewRequest(http.MethodPost, compositionURL(orgID, ledgerID, holderID), bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req, -1)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	respBody, _ := io.ReadAll(resp.Body)

	assert.Equal(t, http.StatusBadRequest, resp.StatusCode, "imperative validation stays 400 — no native Huma 422")
	assert.Equal(t, "application/problem+json", resp.Header.Get("Content-Type"))

	var got map[string]any
	require.NoError(t, json.Unmarshal(respBody, &got), "body: %s", string(respBody))
	assert.NotEmpty(t, got["code"], "canonical code present")
	assert.Equal(t, float64(http.StatusBadRequest), got["status"])
}

func TestHuma_CreateHolderAccount_MalformedBody_Canonical400(t *testing.T) {
	// NOT parallel: process-global huma state.
	orgID := uuid.New()
	ledgerID := uuid.New()
	holderID := uuid.New()

	handler := &CompositionHandler{Service: composition.NewService(stubAccountCreator{}, stubInstrumentCreator{})}

	app := buildHumaCompositionApp(t, handler, true)

	req := httptest.NewRequest(http.MethodPost, compositionURL(orgID, ledgerID, holderID), bytes.NewReader([]byte("{not valid json")))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req, -1)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	respBody, _ := io.ReadAll(resp.Body)

	assert.Equal(t, http.StatusBadRequest, resp.StatusCode, "malformed body stays 400 — no 500, no native 422")
	assert.Equal(t, "application/problem+json", resp.Header.Get("Content-Type"))

	var got map[string]any
	require.NoError(t, json.Unmarshal(respBody, &got), "body: %s", string(respBody))
	assert.Equal(t, cn.ErrInvalidRequestBody.Error(), got["code"], "malformed-body code preserved (0094)")
	assert.Equal(t, float64(http.StatusBadRequest), got["status"])
}

func TestHuma_CreateHolderAccount_BadUUID_Canonical400(t *testing.T) {
	// NOT parallel: process-global huma state.
	orgID := uuid.New()
	ledgerID := uuid.New()

	// Service must never be reached: ParseUUIDPathParameters rejects the bad holder
	// id with the canonical 0065 / 400 before Huma.
	handler := &CompositionHandler{Service: composition.NewService(stubAccountCreator{}, stubInstrumentCreator{})}

	app := buildHumaCompositionApp(t, handler, true)

	url := "/v1/organizations/" + orgID.String() + "/ledgers/" + ledgerID.String() + "/holders/not-a-uuid/accounts"
	req := httptest.NewRequest(http.MethodPost, url, bytes.NewReader(validCompositionBody()))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req, -1)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	respBody, _ := io.ReadAll(resp.Body)

	assert.Equal(t, http.StatusBadRequest, resp.StatusCode, "bad path UUID stays canonical 400 — no native Huma 422")

	var got map[string]any
	require.NoError(t, json.Unmarshal(respBody, &got), "body: %s", string(respBody))
	assert.Equal(t, cn.ErrInvalidPathParameter.Error(), got["code"])
}

func TestHuma_CreateHolderAccount_BusinessError_Preserved(t *testing.T) {
	// NOT parallel: process-global huma state. The account-create fails with a
	// business error; HumaProblem must project the canonical envelope verbatim.
	orgID := uuid.New()
	ledgerID := uuid.New()
	holderID := uuid.New()

	bizErr := pkg.ValidateBusinessError(cn.ErrAssetCodeNotFound, "Account")

	handler := &CompositionHandler{Service: composition.NewService(
		stubAccountCreator{err: bizErr},
		stubInstrumentCreator{},
	)}

	app := buildHumaCompositionApp(t, handler, true)

	req := httptest.NewRequest(http.MethodPost, compositionURL(orgID, ledgerID, holderID), bytes.NewReader(validCompositionBody()))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req, -1)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	respBody, _ := io.ReadAll(resp.Body)

	assert.Equal(t, "application/problem+json", resp.Header.Get("Content-Type"))

	var got map[string]any
	require.NoError(t, json.Unmarshal(respBody, &got), "body: %s", string(respBody))
	assert.Equal(t, cn.ErrAssetCodeNotFound.Error(), got["code"], "business error code preserved across Huma")
}
