// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	openapi "github.com/LerianStudio/lib-commons/v6/commons/net/http/openapi"
	libProblem "github.com/LerianStudio/lib-commons/v6/commons/net/http/problem"
	"github.com/gofiber/fiber/v3"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/services/composition"
	"github.com/LerianStudio/midaz/v4/pkg"
	cn "github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	pkgHTTP "github.com/LerianStudio/midaz/v4/pkg/net/http"
)

// stubAccountCreator satisfies composition.AccountCreator for handler tests.
type stubAccountCreator struct {
	account *mmodel.Account
	err     error
}

func (s stubAccountCreator) CreateAccount(_ context.Context, _, _ uuid.UUID, _ *mmodel.CreateAccountInput, _ string) (*mmodel.Account, error) {
	return s.account, s.err
}

// stubInstrumentCreator satisfies composition.InstrumentCreator for handler tests.
type stubInstrumentCreator struct {
	instrument *mmodel.Instrument
	err        error
}

func (s stubInstrumentCreator) CreateInstrument(_ context.Context, _ string, _ uuid.UUID, _ *mmodel.CreateInstrumentInput) (*mmodel.Instrument, error) {
	return s.instrument, s.err
}

// buildHumaCompositionApp mounts the single composition Huma operation on a /v2
// group, faithfully mirroring the production wiring in unified-server.go:
// problem.Install() runs before any huma.Register, the Huma API is built with
// openapi.New over a /v2 group, an auth-shim middleware stands in for
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
		ErrorHandler: pkgHTTP.CanonicalFiberErrorHandler,
	})

	libProblem.Install()

	apiV2 := f.Group("/v2")

	apiV2.Use(func(c fiber.Ctx) error {
		if !authOK {
			return pkgHTTP.Unauthorized(c, "0001", "Unauthorized", "auth required")
		}

		return c.Next()
	})

	hAPI := openapi.New(f, apiV2, openapi.Config{Title: "ledger-test", Version: "test", Servers: []string{"/v2"}})

	// The :id path param is the holder; ParseUUIDPathParameters("holder") validates
	// it (mirrors composition_routes.go). Registered group-relative on apiV2.
	parse := pkgHTTP.ParseUUIDPathParameters("holder")
	apiV2.Post("/organizations/:organization_id/ledgers/:ledger_id/holders/:id/accounts", parse)

	RegisterCompositionRoutes(hAPI, handler, routeOpSuffixV2)

	return f
}

func compositionURL(orgID, ledgerID, holderID uuid.UUID) string {
	return "/v2/organizations/" + orgID.String() + "/ledgers/" + ledgerID.String() +
		"/holders/" + holderID.String() + "/accounts"
}

func validCompositionBody() []byte {
	body, _ := json.Marshal(map[string]any{
		"assetCode": "USD",
		"type":      "deposit",
	})

	return body
}

func TestCreateHolderAccount_Success(t *testing.T) {
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

	resp, err := app.Test(req, fiber.TestConfig{Timeout: 0})
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

func TestCreateHolderAccount_AuthPreserved(t *testing.T) {
	// NOT parallel: process-global huma state.
	orgID := uuid.New()
	ledgerID := uuid.New()
	holderID := uuid.New()

	// Service must never be reached: a rejected auth returns the ledger 401.
	handler := &CompositionHandler{Service: composition.NewService(stubAccountCreator{}, stubInstrumentCreator{})}

	app := buildHumaCompositionApp(t, handler, false)

	req := httptest.NewRequest(http.MethodPost, compositionURL(orgID, ledgerID, holderID), bytes.NewReader(validCompositionBody()))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req, fiber.TestConfig{Timeout: 0})
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode, "auth middleware must reject before Huma; no public route")
}

func TestCreateHolderAccount_ValidationError_Canonical400(t *testing.T) {
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

	resp, err := app.Test(req, fiber.TestConfig{Timeout: 0})
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

func TestCreateHolderAccount_MalformedBody_Canonical400(t *testing.T) {
	// NOT parallel: process-global huma state.
	orgID := uuid.New()
	ledgerID := uuid.New()
	holderID := uuid.New()

	handler := &CompositionHandler{Service: composition.NewService(stubAccountCreator{}, stubInstrumentCreator{})}

	app := buildHumaCompositionApp(t, handler, true)

	req := httptest.NewRequest(http.MethodPost, compositionURL(orgID, ledgerID, holderID), bytes.NewReader([]byte("{not valid json")))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req, fiber.TestConfig{Timeout: 0})
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

func TestCreateHolderAccount_BadUUID_Canonical400(t *testing.T) {
	// NOT parallel: process-global huma state.
	orgID := uuid.New()
	ledgerID := uuid.New()

	// Service must never be reached: ParseUUIDPathParameters rejects the bad holder
	// id with the canonical 0065 / 400 before Huma.
	handler := &CompositionHandler{Service: composition.NewService(stubAccountCreator{}, stubInstrumentCreator{})}

	app := buildHumaCompositionApp(t, handler, true)

	url := "/v2/organizations/" + orgID.String() + "/ledgers/" + ledgerID.String() + "/holders/not-a-uuid/accounts"
	req := httptest.NewRequest(http.MethodPost, url, bytes.NewReader(validCompositionBody()))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req, fiber.TestConfig{Timeout: 0})
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	respBody, _ := io.ReadAll(resp.Body)

	assert.Equal(t, http.StatusBadRequest, resp.StatusCode, "bad path UUID stays canonical 400 — no native Huma 422")

	var got map[string]any
	require.NoError(t, json.Unmarshal(respBody, &got), "body: %s", string(respBody))
	assert.Equal(t, cn.ErrInvalidPathParameter.Error(), got["code"])
}

func TestCreateHolderAccount_BusinessError_Preserved(t *testing.T) {
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

	resp, err := app.Test(req, fiber.TestConfig{Timeout: 0})
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	respBody, _ := io.ReadAll(resp.Body)

	assert.Equal(t, "application/problem+json", resp.Header.Get("Content-Type"))

	var got map[string]any
	require.NoError(t, json.Unmarshal(respBody, &got), "body: %s", string(respBody))
	assert.Equal(t, cn.ErrAssetCodeNotFound.Error(), got["code"], "business error code preserved across Huma")
}

func TestCreateHolderAccount_WithInstrument_201(t *testing.T) {
	// NOT parallel: process-global huma state.
	orgID := uuid.New()
	ledgerID := uuid.New()
	holderID := uuid.New()

	createdAccount := &mmodel.Account{ID: uuid.New().String(), AssetCode: "USD", Type: "deposit"}
	instrumentID := uuid.New()

	handler := &CompositionHandler{Service: composition.NewService(
		stubAccountCreator{account: createdAccount},
		stubInstrumentCreator{instrument: &mmodel.Instrument{ID: &instrumentID}},
	)}

	app := buildHumaCompositionApp(t, handler, true)

	body, _ := json.Marshal(map[string]any{
		"assetCode":      "USD",
		"type":           "deposit",
		"bankingDetails": map[string]any{},
	})

	req := httptest.NewRequest(http.MethodPost, compositionURL(orgID, ledgerID, holderID), bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req, fiber.TestConfig{Timeout: 0})
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	respBody, _ := io.ReadAll(resp.Body)

	assert.Equal(t, http.StatusCreated, resp.StatusCode)

	var got mmodel.HolderAccountResponse
	require.NoError(t, json.Unmarshal(respBody, &got), "body: %s", string(respBody))
	require.NotNil(t, got.Account)
	require.NotNil(t, got.Instrument, "instrument created alongside the account")
	assert.Nil(t, got.InstrumentError)
}

// TestCreateHolderAccount_PartialFailure_201 locks the partial-failure
// contract: the account is committed, the instrument write fails, and the service
// returns a nil error, so the terminal renders 201 carrying the typed failure block.
func TestCreateHolderAccount_PartialFailure_201(t *testing.T) {
	// NOT parallel: process-global huma state.
	orgID := uuid.New()
	ledgerID := uuid.New()
	holderID := uuid.New()

	createdAccount := &mmodel.Account{ID: uuid.New().String(), AssetCode: "USD", Type: "deposit"}

	handler := &CompositionHandler{Service: composition.NewService(
		stubAccountCreator{account: createdAccount},
		stubInstrumentCreator{err: pkg.ValidateBusinessError(cn.ErrEntityNotFound, "Holder")},
	)}

	app := buildHumaCompositionApp(t, handler, true)

	body, _ := json.Marshal(map[string]any{
		"assetCode":      "USD",
		"type":           "deposit",
		"bankingDetails": map[string]any{},
	})

	req := httptest.NewRequest(http.MethodPost, compositionURL(orgID, ledgerID, holderID), bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req, fiber.TestConfig{Timeout: 0})
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	respBody, _ := io.ReadAll(resp.Body)

	assert.Equal(t, http.StatusCreated, resp.StatusCode, "a failed instrument does not roll back the committed account")

	var got mmodel.HolderAccountResponse
	require.NoError(t, json.Unmarshal(respBody, &got), "body: %s", string(respBody))
	require.NotNil(t, got.Account, "account remains persisted on instrument failure")
	assert.Nil(t, got.Instrument)
	require.NotNil(t, got.InstrumentError, "typed failure block surfaced")
	assert.Equal(t, "FAILED", got.InstrumentError.Status)
	assert.Equal(t, cn.ErrEntityNotFound.Error(), got.InstrumentError.Reason)
}

// TestCreateHolderAccount_BadPathUUID_Direct drives the terminal's defensive
// org/ledger/holder guards, which the wired ParseUUIDPathParameters middleware makes
// unreachable through the app.
func TestCreateHolderAccount_BadPathUUID_Direct(t *testing.T) {
	t.Parallel()

	handler := &CompositionHandler{Service: composition.NewService(stubAccountCreator{}, stubInstrumentCreator{})}

	tests := []struct {
		name string
		in   *CreateHolderAccountRequest
	}{
		{
			name: "bad organization_id",
			in:   &CreateHolderAccountRequest{OrganizationID: "not-a-uuid", LedgerID: uuid.New().String(), ID: uuid.New().String()},
		},
		{
			name: "bad holder id",
			in:   &CreateHolderAccountRequest{OrganizationID: uuid.New().String(), LedgerID: uuid.New().String(), ID: "not-a-uuid"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			_, err := handler.CreateHolderAccount(context.Background(), tt.in)

			var detail *pkgHTTP.Detail
			require.ErrorAs(t, err, &detail, "terminal must return the canonical problem detail")
			assert.Equal(t, http.StatusBadRequest, detail.Status)
			assert.Equal(t, cn.ErrInvalidPathParameter.Error(), detail.Code)
		})
	}
}
