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

	openapi "github.com/LerianStudio/lib-commons/v6/commons/net/http/openapi"
	libProblem "github.com/LerianStudio/lib-commons/v6/commons/net/http/problem"
	"github.com/gofiber/fiber/v3"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/mongodb/fees/pack"
	"github.com/LerianStudio/midaz/v4/components/ledger/pkg/feeshared/model"
	"github.com/LerianStudio/midaz/v4/pkg"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	pkgHTTP "github.com/LerianStudio/midaz/v4/pkg/net/http"
)

// feesAuthShim stands in for the auth.Authorize + tenant chain: 401 when authOK is
// false, passthrough otherwise. Mirrors the instrument harness's inline shim.
func feesAuthShim(authOK bool) fiber.Handler {
	return func(c fiber.Ctx) error {
		if !authOK {
			return pkgHTTP.Unauthorized(c, "0001", "Unauthorized", "auth required")
		}

		return c.Next()
	}
}

// The stubPackageService / stubFeeService fakes and the validCreatePackageInput
// helper live in fees_billing_handlers_test.go; these Huma tests reuse them.

// feePkgV2Base is the ledger-scoped fee-package path the v2 surface serves, in Fiber
// syntax. The org and ledger are substituted per test; create/estimate additionally
// require the body ledger to equal the path ledger (see fees_ledger_scope.go), so the
// tests that carry a body use validLedgerUUID() — the ledger validCreatePackageJSON /
// estimateBodyJSON stamp — as the path ledger.
const feePkgV2Base = "/v2/organizations/"

// buildHumaPackageApp mounts the five ledger-scoped package Huma operations on a /v2
// group, mirroring production (fees_v2_register.go/unified-server.go): problem.Install()
// before any huma.Register, the Huma API built with openapi.New over a /v2 group, an
// auth-shim standing in for auth.Authorize("plugin-fees","packages",verb) + tenant, and
// per-route ParseUUIDPathParameters("packages") + registerPackageV2Routes.
//
// MUST-NOT-PARALLELIZE (same rationale as buildHumaInstrumentApp): libProblem.Install()
// swaps the process-global huma.NewError hook and Huma validation uses process-global
// sync.Pools — concurrent builds/requests cross-contaminate.
func buildHumaPackageApp(t *testing.T, handler *PackageHandler, authOK bool) *fiber.App {
	t.Helper()

	f := fiber.New(fiber.Config{
		ErrorHandler: pkgHTTP.CanonicalFiberErrorHandler,
	})

	libProblem.Install()

	apiV2 := f.Group("/v2")

	apiV2.Use(feesAuthShim(authOK))

	parse := pkgHTTP.ParseUUIDPathParameters("packages")

	listPath := "/organizations/:organization_id/ledgers/:ledger_id/packages"
	idPath := listPath + "/:id"

	apiV2.Post(listPath, parse)
	apiV2.Get(listPath, parse)
	apiV2.Get(idPath, parse)
	apiV2.Patch(idPath, parse)
	apiV2.Delete(idPath, parse)

	hAPI := openapi.New(f, apiV2, openapi.Config{Title: "ledger-test", Version: "test", Servers: []string{"/v2"}})

	registerPackageV2Routes(hAPI, handler)

	return f
}

// buildHumaFeeEstimateApp mounts the single ledger-scoped estimate Huma operation.
func buildHumaFeeEstimateApp(t *testing.T, handler *FeeHandler, authOK bool) *fiber.App {
	t.Helper()

	f := fiber.New(fiber.Config{
		ErrorHandler: pkgHTTP.CanonicalFiberErrorHandler,
	})

	libProblem.Install()

	apiV2 := f.Group("/v2")

	apiV2.Use(feesAuthShim(authOK))
	apiV2.Post("/organizations/:organization_id/ledgers/:ledger_id/estimates", pkgHTTP.ParseUUIDPathParameters("estimates"))

	hAPI := openapi.New(f, apiV2, openapi.Config{Title: "ledger-test", Version: "test", Servers: []string{"/v2"}})

	registerFeeEstimateV2Routes(hAPI, handler)

	return f
}

func validLedgerUUID() string { return "00000000-0000-0000-0000-000000000009" }

func TestHuma_CreatePackage_Success(t *testing.T) {
	orgID := uuid.New()
	packID := uuid.New()

	stub := &stubPackageService{createResult: &pack.Package{ID: packID, FeeGroupLabel: "Standard"}}
	handler := &PackageHandler{Service: stub}

	app := buildHumaPackageApp(t, handler, true)

	// Minimal validator-valid create: min<=max, one non-deductible flatFee fee at
	// priority 2 (avoids the priority-1 originalAmount rule), valid ledger. This
	// exercises the REAL fee-package body validator the Huma shell delegates to
	// (the WithBodyTracing landmine), not a pre-built payload injection. The body
	// ledger equals the path ledger, so the ledger-scoped guard admits it.
	body := validCreatePackageJSON()
	req := httptest.NewRequest(http.MethodPost, feePkgV2Base+orgID.String()+"/ledgers/"+validLedgerUUID()+"/packages", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req, fiber.TestConfig{Timeout: 0})
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	respBody, _ := io.ReadAll(resp.Body)
	assert.Equal(t, http.StatusCreated, resp.StatusCode, "body: %s", string(respBody))
	assert.True(t, stub.createCalled, "service.CreatePackage must be invoked")
	assert.NotContains(t, string(respBody), "$schema")

	var got map[string]any
	require.NoError(t, json.Unmarshal(respBody, &got), "body: %s", string(respBody))
	assert.Equal(t, packID.String(), got["id"])
}

func TestHuma_CreatePackage_AuthPreserved(t *testing.T) {
	orgID := uuid.New()

	handler := &PackageHandler{Service: &stubPackageService{}}
	app := buildHumaPackageApp(t, handler, false)

	req := httptest.NewRequest(http.MethodPost, feePkgV2Base+orgID.String()+"/ledgers/"+validLedgerUUID()+"/packages", bytes.NewBufferString(`{}`))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req, fiber.TestConfig{Timeout: 0})
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode, "auth middleware must reject before Huma")
}

func TestHuma_CreatePackage_MalformedBody_Canonical400(t *testing.T) {
	orgID := uuid.New()

	handler := &PackageHandler{Service: &stubPackageService{}}
	app := buildHumaPackageApp(t, handler, true)

	req := httptest.NewRequest(http.MethodPost, feePkgV2Base+orgID.String()+"/ledgers/"+validLedgerUUID()+"/packages", bytes.NewReader([]byte("{not valid json")))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req, fiber.TestConfig{Timeout: 0})
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	respBody, _ := io.ReadAll(resp.Body)
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode, "malformed body stays 400 — no native 422")
	assert.Equal(t, "application/problem+json", resp.Header.Get("Content-Type"))

	var got map[string]any
	require.NoError(t, json.Unmarshal(respBody, &got), "body: %s", string(respBody))
	assert.Equal(t, constant.ErrInvalidRequestBody.Error(), got["code"], "malformed-body code preserved (0094)")
}

func TestHuma_GetPackageByID_Success(t *testing.T) {
	orgID := uuid.New()
	packID := uuid.New()

	stub := &stubPackageService{getByIDResult: &pack.Package{ID: packID, FeeGroupLabel: "Standard"}}
	handler := &PackageHandler{Service: stub}

	app := buildHumaPackageApp(t, handler, true)

	req := httptest.NewRequest(http.MethodGet, feePkgV2Base+orgID.String()+"/ledgers/"+validLedgerUUID()+"/packages/"+packID.String(), nil)
	resp, err := app.Test(req, fiber.TestConfig{Timeout: 0})
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	respBody, _ := io.ReadAll(resp.Body)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, packID, stub.gotGetByIDID)

	var got map[string]any
	require.NoError(t, json.Unmarshal(respBody, &got), "body: %s", string(respBody))
	assert.Equal(t, packID.String(), got["id"])
}

func TestHuma_GetPackageByID_BadUUID_Canonical400(t *testing.T) {
	orgID := uuid.New()

	handler := &PackageHandler{Service: &stubPackageService{}}
	app := buildHumaPackageApp(t, handler, true)

	req := httptest.NewRequest(http.MethodGet, feePkgV2Base+orgID.String()+"/ledgers/"+validLedgerUUID()+"/packages/not-a-uuid", nil)
	resp, err := app.Test(req, fiber.TestConfig{Timeout: 0})
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	respBody, _ := io.ReadAll(resp.Body)
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode, "bad path UUID stays canonical 400 — no native 422")

	var got map[string]any
	require.NoError(t, json.Unmarshal(respBody, &got), "body: %s", string(respBody))
	assert.Equal(t, constant.ErrInvalidPathParameter.Error(), got["code"])
}

func TestHuma_GetAllPackages_Success(t *testing.T) {
	orgID := uuid.New()

	stub := &stubPackageService{getAllResult: []*pack.Package{{ID: uuid.New()}}}
	handler := &PackageHandler{Service: stub}

	app := buildHumaPackageApp(t, handler, true)

	req := httptest.NewRequest(http.MethodGet, feePkgV2Base+orgID.String()+"/ledgers/"+validLedgerUUID()+"/packages?limit=5&page=2", nil)
	resp, err := app.Test(req, fiber.TestConfig{Timeout: 0})
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	respBody, _ := io.ReadAll(resp.Body)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, orgID, stub.gotGetAllOrg)
	assert.Equal(t, 5, stub.gotGetAllFilter.Limit, "query binder must feed the fee ValidateParameters result")
	assert.Equal(t, 2, stub.gotGetAllFilter.Page)

	var got map[string]any
	require.NoError(t, json.Unmarshal(respBody, &got), "body: %s", string(respBody))
	assert.EqualValues(t, 5, got["limit"])
	assert.EqualValues(t, 2, got["page"])
	assert.EqualValues(t, 1, got["total"])
}

func TestHuma_GetAllPackages_BadQuery_Canonical400(t *testing.T) {
	orgID := uuid.New()

	handler := &PackageHandler{Service: &stubPackageService{}}
	app := buildHumaPackageApp(t, handler, true)

	req := httptest.NewRequest(http.MethodGet, feePkgV2Base+orgID.String()+"/ledgers/"+validLedgerUUID()+"/packages?limit=abc", nil)
	resp, err := app.Test(req, fiber.TestConfig{Timeout: 0})
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	respBody, _ := io.ReadAll(resp.Body)
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode, "bad query stays canonical 400 — no native 422")

	var got map[string]any
	require.NoError(t, json.Unmarshal(respBody, &got), "body: %s", string(respBody))
	assert.Equal(t, constant.ErrInvalidQueryParameter.Error(), got["code"])
}

func TestHuma_UpdatePackage_Success(t *testing.T) {
	orgID := uuid.New()
	packID := uuid.New()

	// Update re-reads via GetPackageByID after a successful update; both are stubbed.
	stub := &stubPackageService{getByIDResult: &pack.Package{ID: packID, FeeGroupLabel: "Updated"}}
	handler := &PackageHandler{Service: stub}

	app := buildHumaPackageApp(t, handler, true)

	body := `{"description":"new desc"}`
	req := httptest.NewRequest(http.MethodPatch, feePkgV2Base+orgID.String()+"/ledgers/"+validLedgerUUID()+"/packages/"+packID.String(), bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req, fiber.TestConfig{Timeout: 0})
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	respBody, _ := io.ReadAll(resp.Body)
	assert.Equal(t, http.StatusOK, resp.StatusCode, "body: %s", string(respBody))
	assert.True(t, stub.updateCalled, "service.UpdatePackageByID must be invoked")

	var got map[string]any
	require.NoError(t, json.Unmarshal(respBody, &got), "body: %s", string(respBody))
	assert.Equal(t, packID.String(), got["id"])
}

func TestHuma_DeletePackage_204Empty(t *testing.T) {
	orgID := uuid.New()
	packID := uuid.New()

	stub := &stubPackageService{}
	handler := &PackageHandler{Service: stub}

	app := buildHumaPackageApp(t, handler, true)

	req := httptest.NewRequest(http.MethodDelete, feePkgV2Base+orgID.String()+"/ledgers/"+validLedgerUUID()+"/packages/"+packID.String(), nil)
	resp, err := app.Test(req, fiber.TestConfig{Timeout: 0})
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	respBody, _ := io.ReadAll(resp.Body)
	assert.Equal(t, http.StatusNoContent, resp.StatusCode)
	assert.Empty(t, respBody, "DELETE 204 must have an empty body")
	assert.True(t, stub.deleteCalled)
	assert.Equal(t, packID, stub.gotDeleteID)
}

func TestHuma_EstimateFee_Success(t *testing.T) {
	orgID := uuid.New()

	// A result whose transaction carries packageAppliedID triggers the "success"
	// message branch (non-nil FeesApplied).
	result := &model.FeeEstimateResult{Transaction: model.FeeAdjustedTransaction{Metadata: map[string]any{"packageAppliedID": "abc"}}}

	stub := &stubFeeService{result: result}
	handler := &FeeHandler{Service: stub}

	app := buildHumaFeeEstimateApp(t, handler, true)

	body := estimateBodyJSON()
	req := httptest.NewRequest(http.MethodPost, feePkgV2Base+orgID.String()+"/ledgers/"+validLedgerUUID()+"/estimates", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req, fiber.TestConfig{Timeout: 0})
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	respBody, _ := io.ReadAll(resp.Body)
	assert.Equal(t, http.StatusOK, resp.StatusCode, "body: %s", string(respBody))
	assert.True(t, stub.called, "service.EstimateFeeCalculation must be invoked")
	assert.Equal(t, orgID, stub.gotOrg)

	var got map[string]any
	require.NoError(t, json.Unmarshal(respBody, &got), "body: %s", string(respBody))
	assert.Equal(t, "Successfully estimated fee.", got["message"])
	assert.NotNil(t, got["feesApplied"])
}

func TestHuma_EstimateFee_NoRules_EmptyMessage(t *testing.T) {
	orgID := uuid.New()

	// No packageAppliedID => "no rules found" branch, feesApplied nil.
	result := &model.FeeEstimateResult{Transaction: model.FeeAdjustedTransaction{Metadata: map[string]any{}}}

	stub := &stubFeeService{result: result}
	handler := &FeeHandler{Service: stub}

	app := buildHumaFeeEstimateApp(t, handler, true)

	body := estimateBodyJSON()
	req := httptest.NewRequest(http.MethodPost, feePkgV2Base+orgID.String()+"/ledgers/"+validLedgerUUID()+"/estimates", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req, fiber.TestConfig{Timeout: 0})
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	respBody, _ := io.ReadAll(resp.Body)
	assert.Equal(t, http.StatusOK, resp.StatusCode, "body: %s", string(respBody))

	var got map[string]any
	require.NoError(t, json.Unmarshal(respBody, &got), "body: %s", string(respBody))
	assert.Equal(t, "No fee or gratuity rules were found for the given parameters.", got["message"])
	assert.Nil(t, got["feesApplied"])
}

func TestHuma_EstimateFee_ServiceError_Mapped(t *testing.T) {
	orgID := uuid.New()

	stub := &stubFeeService{err: pkg.ValidateBusinessError(constant.ErrInvalidPathParameter, "", "packageId")}
	handler := &FeeHandler{Service: stub}

	app := buildHumaFeeEstimateApp(t, handler, true)

	body := estimateBodyJSON()
	req := httptest.NewRequest(http.MethodPost, feePkgV2Base+orgID.String()+"/ledgers/"+validLedgerUUID()+"/estimates", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req, fiber.TestConfig{Timeout: 0})
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	respBody, _ := io.ReadAll(resp.Body)
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode, "body: %s", string(respBody))
	assert.Equal(t, "application/problem+json", resp.Header.Get("Content-Type"))

	var got map[string]any
	require.NoError(t, json.Unmarshal(respBody, &got), "body: %s", string(respBody))
	assert.Equal(t, constant.ErrInvalidPathParameter.Error(), got["code"])
}

// estimateBodyJSON returns a FeeEstimate payload that satisfies the fee-package
// validator: packageId + ledgerId are required UUIDs and the embedded transaction's
// send (asset + value + source.from + distribute.to) is required. The fee engine is
// stubbed, so only decode+validate must pass. ledgerId is validLedgerUUID(), so the
// ledger-scoped estimate guard admits it when the path names the same ledger.
func estimateBodyJSON() string {
	return `{"packageId":"` + validLedgerUUID() + `","ledgerId":"` + validLedgerUUID() + `","transaction":{"send":` + validSendJSON() + `}}`
}

// validSendJSON is a minimal transaction send that clears the fee-package validator:
// each from/to entry carries exactly one of amount/share/remaining
// (singletransactiontype).
func validSendJSON() string {
	return `{"asset":"BRL","value":"100","source":{"from":[{"accountAlias":"@external/BRL","amount":{"asset":"BRL","value":"100"}}]},"distribute":{"to":[{"accountAlias":"@person1","amount":{"asset":"BRL","value":"100"}}]}}`
}

// validCreatePackageJSON is a validator-valid create-package body: one non-deductible
// flatFee fee at priority 2 with a single flat calculation (avoids the priority-1
// originalAmount rule and the deductible min-amount check), min<=max, valid ledger.
// ledgerId is validLedgerUUID(), so the ledger-scoped create guard admits it when the
// path names the same ledger.
func validCreatePackageJSON() string {
	return `{"feeGroupLabel":"Standard","ledgerId":"` + validLedgerUUID() + `","minimumAmount":"100.00","maximumAmount":"1000.00","enable":true,"fees":{"f1":{"feeLabel":"Admin","referenceAmount":"afterFeesAmount","priority":2,"isDeductibleFrom":false,"creditAccount":"conta_receita","calculationModel":{"applicationRule":"flatFee","calculations":[{"type":"flat","value":"50.00"}]}}}}`
}
