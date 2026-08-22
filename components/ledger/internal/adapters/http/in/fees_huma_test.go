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
	"strconv"
	"testing"

	libCommons "github.com/LerianStudio/lib-commons/v6/commons"

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

// The stubPackageService and stubFeeService fakes live in
// fees_billing_handlers_test.go; these tests reuse them.

// feePkgV2Base is the request-URL prefix for the v2 fee surface. Each test completes it
// by hand, appending the org, "/ledgers/"+ledger, and the resource segment (packages /
// estimates). It is not a Fiber route template — those are the separate listPath/idPath
// literals in buildHumaPackageApp. Create/estimate additionally require the body ledger to
// equal the path ledger (see fees_ledger_scope.go), so the tests that carry a body use
// validLedgerUUID() — the ledger validCreatePackageJSON / estimateBodyJSON stamp — as the
// path ledger.
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
	pkgHTTP.InstallLedgerSchemaNamer(hAPI)

	registerFeeEstimateV2Routes(hAPI, handler)

	return f
}

func validLedgerUUID() string { return "00000000-0000-0000-0000-000000000009" }

func TestCreatePackage_Success(t *testing.T) {
	orgID := uuid.Must(libCommons.GenerateUUIDv7())
	packID := uuid.Must(libCommons.GenerateUUIDv7())

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

func TestCreatePackage_AuthPreserved(t *testing.T) {
	orgID := uuid.Must(libCommons.GenerateUUIDv7())

	handler := &PackageHandler{Service: &stubPackageService{}}
	app := buildHumaPackageApp(t, handler, false)

	req := httptest.NewRequest(http.MethodPost, feePkgV2Base+orgID.String()+"/ledgers/"+validLedgerUUID()+"/packages", bytes.NewBufferString(`{}`))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req, fiber.TestConfig{Timeout: 0})
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode, "auth middleware must reject before Huma")
}

func TestCreatePackage_MalformedBody_Canonical400(t *testing.T) {
	orgID := uuid.Must(libCommons.GenerateUUIDv7())

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

func TestGetPackageByID_Success(t *testing.T) {
	orgID := uuid.Must(libCommons.GenerateUUIDv7())
	packID := uuid.Must(libCommons.GenerateUUIDv7())

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

func TestGetPackageByID_BadUUID_Canonical400(t *testing.T) {
	orgID := uuid.Must(libCommons.GenerateUUIDv7())

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

func TestGetAllPackages_Success(t *testing.T) {
	orgID := uuid.Must(libCommons.GenerateUUIDv7())

	stub := &stubPackageService{getAllResult: []*pack.Package{{ID: uuid.Must(libCommons.GenerateUUIDv7())}}}
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

func TestGetAllPackages_BadQuery_Canonical400(t *testing.T) {
	orgID := uuid.Must(libCommons.GenerateUUIDv7())

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

func TestUpdatePackage_Success(t *testing.T) {
	orgID := uuid.Must(libCommons.GenerateUUIDv7())
	packID := uuid.Must(libCommons.GenerateUUIDv7())

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

func TestDeletePackage_204Empty(t *testing.T) {
	orgID := uuid.Must(libCommons.GenerateUUIDv7())
	packID := uuid.Must(libCommons.GenerateUUIDv7())

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
	orgID := uuid.Must(libCommons.GenerateUUIDv7())

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
	orgID := uuid.Must(libCommons.GenerateUUIDv7())

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
	orgID := uuid.Must(libCommons.GenerateUUIDv7())

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

// packageFeeJSON builds one validator-clearing fee entry: every required Fee field is
// present and the calculation model is a single flat calculation, so only the
// referenceAmount/priority pair under test decides whether the handler guards fire.
func packageFeeJSON(label, referenceAmount string, priority int) string {
	return `{"feeLabel":"` + label + `","referenceAmount":"` + referenceAmount +
		`","priority":` + strconv.Itoa(priority) +
		`,"isDeductibleFrom":false,"creditAccount":"conta_receita",` +
		`"calculationModel":{"applicationRule":"flatFee","calculations":[{"type":"flat","value":"50.00"}]}}`
}

// createPackageJSON assembles a create-package body around a fee map and an amount
// range, keeping every other required field at a validator-clearing value. extra is
// spliced in verbatim so a test can add an optional field such as segmentId.
func createPackageJSON(minAmount, maxAmount, fees, extra string) string {
	return `{"feeGroupLabel":"Standard","ledgerId":"` + validLedgerUUID() + `"` + extra +
		`,"minimumAmount":"` + minAmount + `","maximumAmount":"` + maxAmount +
		`","enable":true,"fees":` + fees + `}`
}

// postPackage sends a create-package request at the ledger the body names.
func postPackage(t *testing.T, app *fiber.App, orgID uuid.UUID, body string) *http.Response {
	t.Helper()

	req := httptest.NewRequest(http.MethodPost, feePkgV2Base+orgID.String()+"/ledgers/"+validLedgerUUID()+"/packages", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req, fiber.TestConfig{Timeout: 0})
	require.NoError(t, err)

	return resp
}

// patchPackage sends an update-package request for one package id.
func patchPackage(t *testing.T, app *fiber.App, orgID, packID uuid.UUID, body string) *http.Response {
	t.Helper()

	req := httptest.NewRequest(http.MethodPatch, feePkgV2Base+orgID.String()+"/ledgers/"+validLedgerUUID()+"/packages/"+packID.String(), bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req, fiber.TestConfig{Timeout: 0})
	require.NoError(t, err)

	return resp
}

// assertProblem drains a problem response and asserts the canonical error code.
func assertProblem(t *testing.T, resp *http.Response, wantStatus int, wantCode string) {
	t.Helper()

	respBody, _ := io.ReadAll(resp.Body)
	assert.Equal(t, wantStatus, resp.StatusCode, "body: %s", string(respBody))

	var got map[string]any
	require.NoError(t, json.Unmarshal(respBody, &got), "body: %s", string(respBody))
	assert.Equal(t, wantCode, got["code"])
}

func TestCreatePackage_SegmentIDParsedAndForwarded(t *testing.T) {
	orgID := uuid.Must(libCommons.GenerateUUIDv7())
	segmentID := uuid.Must(libCommons.GenerateUUIDv7())

	stub := &stubPackageService{createResult: &pack.Package{ID: uuid.Must(libCommons.GenerateUUIDv7())}}
	handler := &PackageHandler{Service: stub}

	body := createPackageJSON("100.00", "1000.00",
		`{"f1":`+packageFeeJSON("Admin", "afterFeesAmount", 2)+`}`,
		`,"segmentId":"`+segmentID.String()+`"`)

	resp := postPackage(t, buildHumaPackageApp(t, handler, true), orgID, body)
	defer func() { _ = resp.Body.Close() }()

	respBody, _ := io.ReadAll(resp.Body)
	assert.Equal(t, http.StatusCreated, resp.StatusCode, "body: %s", string(respBody))
	assert.Equal(t, segmentID, stub.gotCreateSeg, "segmentId must be parsed and forwarded")
}

func TestCreatePackage_MalformedSegmentID_Canonical400(t *testing.T) {
	orgID := uuid.Must(libCommons.GenerateUUIDv7())

	stub := &stubPackageService{}
	handler := &PackageHandler{Service: stub}

	body := createPackageJSON("100.00", "1000.00",
		`{"f1":`+packageFeeJSON("Admin", "afterFeesAmount", 2)+`}`,
		`,"segmentId":"not-a-uuid"`)

	resp := postPackage(t, buildHumaPackageApp(t, handler, true), orgID, body)
	defer func() { _ = resp.Body.Close() }()

	assertProblem(t, resp, http.StatusBadRequest, constant.ErrInvalidSegmentID.Error())
	assert.False(t, stub.createCalled, "a malformed segment id must short-circuit before the service")
}

func TestCreatePackage_MinGreaterThanMax_422(t *testing.T) {
	orgID := uuid.Must(libCommons.GenerateUUIDv7())

	stub := &stubPackageService{}
	handler := &PackageHandler{Service: stub}

	body := createPackageJSON("1000.00", "1.00",
		`{"f1":`+packageFeeJSON("Admin", "afterFeesAmount", 2)+`}`, "")

	resp := postPackage(t, buildHumaPackageApp(t, handler, true), orgID, body)
	defer func() { _ = resp.Body.Close() }()

	assertProblem(t, resp, http.StatusUnprocessableEntity, constant.ErrMinAmountGreaterThanMaxAmount.Error())
	assert.False(t, stub.createCalled)
}

func TestCreatePackage_PriorityOneWrongReference_Canonical400(t *testing.T) {
	orgID := uuid.Must(libCommons.GenerateUUIDv7())

	stub := &stubPackageService{}
	handler := &PackageHandler{Service: stub}

	body := createPackageJSON("100.00", "1000.00",
		`{"f1":`+packageFeeJSON("Admin", "afterFeesAmount", 1)+`}`, "")

	resp := postPackage(t, buildHumaPackageApp(t, handler, true), orgID, body)
	defer func() { _ = resp.Body.Close() }()

	assertProblem(t, resp, http.StatusBadRequest, constant.ErrPriorityOne.Error())
	assert.False(t, stub.createCalled, "fee validation must short-circuit before the service")
}

func TestCreatePackage_DuplicatePriorities_Canonical400(t *testing.T) {
	orgID := uuid.Must(libCommons.GenerateUUIDv7())

	stub := &stubPackageService{}
	handler := &PackageHandler{Service: stub}

	// Both fees clear ValidateFees (priority 2, non-deductible), so the request
	// reaches the handler's duplicate-priority guard.
	body := createPackageJSON("100.00", "1000.00",
		`{"a":`+packageFeeJSON("F1", "afterFeesAmount", 2)+`,"b":`+packageFeeJSON("F2", "afterFeesAmount", 2)+`}`, "")

	resp := postPackage(t, buildHumaPackageApp(t, handler, true), orgID, body)
	defer func() { _ = resp.Body.Close() }()

	assertProblem(t, resp, http.StatusBadRequest, constant.ErrPriorityInvalid.Error())
	assert.False(t, stub.createCalled)
}

func TestCreatePackage_ServiceError_Mapped(t *testing.T) {
	orgID := uuid.Must(libCommons.GenerateUUIDv7())

	stub := &stubPackageService{createErr: pkg.ValidateBusinessError(constant.ErrDuplicatePackage, constant.EntityPackage)}
	handler := &PackageHandler{Service: stub}

	resp := postPackage(t, buildHumaPackageApp(t, handler, true), orgID, validCreatePackageJSON())
	defer func() { _ = resp.Body.Close() }()

	assertProblem(t, resp, http.StatusConflict, constant.ErrDuplicatePackage.Error())
}

func TestGetAllPackages_ServiceError_Mapped(t *testing.T) {
	orgID := uuid.Must(libCommons.GenerateUUIDv7())

	stub := &stubPackageService{getAllErr: pkg.ValidateBusinessError(constant.ErrCalculateFee, constant.EntityFeeCalculation)}
	handler := &PackageHandler{Service: stub}

	app := buildHumaPackageApp(t, handler, true)

	req := httptest.NewRequest(http.MethodGet, feePkgV2Base+orgID.String()+"/ledgers/"+validLedgerUUID()+"/packages", nil)
	resp, err := app.Test(req, fiber.TestConfig{Timeout: 0})
	require.NoError(t, err)

	defer func() { _ = resp.Body.Close() }()

	assertProblem(t, resp, http.StatusInternalServerError, constant.ErrCalculateFee.Error())
}

func TestGetPackageByID_NotFound_404(t *testing.T) {
	orgID := uuid.Must(libCommons.GenerateUUIDv7())
	packID := uuid.Must(libCommons.GenerateUUIDv7())

	stub := &stubPackageService{getByIDErr: pkg.ValidateBusinessError(constant.ErrEntityNotFound, constant.EntityPackage)}
	handler := &PackageHandler{Service: stub}

	app := buildHumaPackageApp(t, handler, true)

	req := httptest.NewRequest(http.MethodGet, feePkgV2Base+orgID.String()+"/ledgers/"+validLedgerUUID()+"/packages/"+packID.String(), nil)
	resp, err := app.Test(req, fiber.TestConfig{Timeout: 0})
	require.NoError(t, err)

	defer func() { _ = resp.Body.Close() }()

	assertProblem(t, resp, http.StatusNotFound, constant.ErrEntityNotFound.Error())
}

func TestUpdatePackage_ValidFeeMapReachesService(t *testing.T) {
	orgID := uuid.Must(libCommons.GenerateUUIDv7())
	packID := uuid.Must(libCommons.GenerateUUIDv7())

	stub := &stubPackageService{getByIDResult: &pack.Package{ID: packID, FeeGroupLabel: "After"}}
	handler := &PackageHandler{Service: stub}

	body := `{"fees":{"a":{"feeLabel":"Admin","priority":2,"referenceAmount":"originalAmount"}}}`

	resp := patchPackage(t, buildHumaPackageApp(t, handler, true), orgID, packID, body)
	defer func() { _ = resp.Body.Close() }()

	respBody, _ := io.ReadAll(resp.Body)
	assert.Equal(t, http.StatusOK, resp.StatusCode, "body: %s", string(respBody))
	assert.True(t, stub.updateCalled, "a valid fee map must pass the fee guards and reach the service")
}

func TestUpdatePackage_PriorityOneWrongReference_Canonical400(t *testing.T) {
	orgID := uuid.Must(libCommons.GenerateUUIDv7())
	packID := uuid.Must(libCommons.GenerateUUIDv7())

	stub := &stubPackageService{}
	handler := &PackageHandler{Service: stub}

	body := `{"fees":{"a":{"feeLabel":"F1","priority":1,"referenceAmount":"afterFeesAmount"}}}`

	resp := patchPackage(t, buildHumaPackageApp(t, handler, true), orgID, packID, body)
	defer func() { _ = resp.Body.Close() }()

	assertProblem(t, resp, http.StatusBadRequest, constant.ErrPriorityOne.Error())
	assert.False(t, stub.updateCalled, "fee validation must short-circuit before the service")
}

func TestUpdatePackage_DuplicatePriorities_Canonical400(t *testing.T) {
	orgID := uuid.Must(libCommons.GenerateUUIDv7())
	packID := uuid.Must(libCommons.GenerateUUIDv7())

	stub := &stubPackageService{}
	handler := &PackageHandler{Service: stub}

	body := `{"fees":{` +
		`"a":{"feeLabel":"F1","priority":3,"referenceAmount":"originalAmount"},` +
		`"b":{"feeLabel":"F2","priority":3,"referenceAmount":"originalAmount"}}}`

	resp := patchPackage(t, buildHumaPackageApp(t, handler, true), orgID, packID, body)
	defer func() { _ = resp.Body.Close() }()

	assertProblem(t, resp, http.StatusBadRequest, constant.ErrPriorityInvalid.Error())
	assert.False(t, stub.updateCalled, "duplicate priorities must short-circuit before the service")
}

func TestUpdatePackage_MinGreaterThanMax_422(t *testing.T) {
	orgID := uuid.Must(libCommons.GenerateUUIDv7())
	packID := uuid.Must(libCommons.GenerateUUIDv7())

	stub := &stubPackageService{}
	handler := &PackageHandler{Service: stub}

	resp := patchPackage(t, buildHumaPackageApp(t, handler, true), orgID, packID, `{"minimumAmount":"100","maximumAmount":"1"}`)
	defer func() { _ = resp.Body.Close() }()

	assertProblem(t, resp, http.StatusUnprocessableEntity, constant.ErrMinAmountGreaterThanMaxAmount.Error())
	assert.False(t, stub.updateCalled)
}

func TestUpdatePackage_UpdateError_404(t *testing.T) {
	orgID := uuid.Must(libCommons.GenerateUUIDv7())
	packID := uuid.Must(libCommons.GenerateUUIDv7())

	stub := &stubPackageService{updateErr: pkg.ValidateBusinessError(constant.ErrEntityNotFound, constant.EntityPackage)}
	handler := &PackageHandler{Service: stub}

	resp := patchPackage(t, buildHumaPackageApp(t, handler, true), orgID, packID, `{"feeGroupLabel":"X"}`)
	defer func() { _ = resp.Body.Close() }()

	assertProblem(t, resp, http.StatusNotFound, constant.ErrEntityNotFound.Error())
	assert.True(t, stub.updateCalled)
}

func TestUpdatePackage_ReReadError_Mapped(t *testing.T) {
	orgID := uuid.Must(libCommons.GenerateUUIDv7())
	packID := uuid.Must(libCommons.GenerateUUIDv7())

	// The write succeeds and the re-read fails, so the response must carry the
	// re-read failure rather than a 200 built from a stale package.
	stub := &stubPackageService{getByIDErr: pkg.ValidateBusinessError(constant.ErrCalculateFee, constant.EntityFeeCalculation)}
	handler := &PackageHandler{Service: stub}

	resp := patchPackage(t, buildHumaPackageApp(t, handler, true), orgID, packID, `{"feeGroupLabel":"X"}`)
	defer func() { _ = resp.Body.Close() }()

	assertProblem(t, resp, http.StatusInternalServerError, constant.ErrCalculateFee.Error())
	assert.True(t, stub.updateCalled)
}

func TestDeletePackage_NotFound_404(t *testing.T) {
	orgID := uuid.Must(libCommons.GenerateUUIDv7())
	packID := uuid.Must(libCommons.GenerateUUIDv7())

	stub := &stubPackageService{deleteErr: pkg.ValidateBusinessError(constant.ErrEntityNotFound, constant.EntityPackage)}
	handler := &PackageHandler{Service: stub}

	app := buildHumaPackageApp(t, handler, true)

	req := httptest.NewRequest(http.MethodDelete, feePkgV2Base+orgID.String()+"/ledgers/"+validLedgerUUID()+"/packages/"+packID.String(), nil)
	resp, err := app.Test(req, fiber.TestConfig{Timeout: 0})
	require.NoError(t, err)

	defer func() { _ = resp.Body.Close() }()

	assertProblem(t, resp, http.StatusNotFound, constant.ErrEntityNotFound.Error())
}

// TestHuma_EstimateFee_NilResult_500 covers estimateFeeCalculation's nil-result
// guard: a service returning (nil, nil) must surface as a canonical internal
// error, not an empty 200.
func TestHuma_EstimateFee_NilResult_500(t *testing.T) {
	orgID := uuid.Must(libCommons.GenerateUUIDv7())

	stub := &stubFeeService{result: nil}
	handler := &FeeHandler{Service: stub}

	app := buildHumaFeeEstimateApp(t, handler, true)

	req := httptest.NewRequest(http.MethodPost, feePkgV2Base+orgID.String()+"/ledgers/"+validLedgerUUID()+"/estimates", bytes.NewBufferString(estimateBodyJSON()))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req, fiber.TestConfig{Timeout: 0})
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	respBody, _ := io.ReadAll(resp.Body)
	assert.Equal(t, http.StatusInternalServerError, resp.StatusCode, "body: %s", string(respBody))
}
