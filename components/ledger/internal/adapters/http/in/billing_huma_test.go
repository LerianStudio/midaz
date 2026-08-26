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

	libCommons "github.com/LerianStudio/lib-commons/v6/commons"

	openapi "github.com/LerianStudio/lib-commons/v6/commons/net/http/openapi"
	libProblem "github.com/LerianStudio/lib-commons/v6/commons/net/http/problem"
	"github.com/gofiber/fiber/v3"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/components/ledger/pkg/feeshared/model"
	"github.com/LerianStudio/midaz/v4/pkg"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	pkgHTTP "github.com/LerianStudio/midaz/v4/pkg/net/http"
)

// The stubBillingPackageService / stubBillingCalculateService fakes live in
// fees_billing_handlers_test.go; these Huma tests reuse them (mirroring how the fee
// Huma tests reuse stubPackageService/stubFeeService).

// buildHumaBillingPackageApp mounts the five ledger-scoped billing-package Huma
// operations on a /v2 group, mirroring production
// (billing_package_routes.go/unified-server.go): problem.Install() before any huma.Register,
// the Huma API built with openapi.New over a /v2 group, an auth-shim standing in for
// auth.Authorize("midaz","billing-packages",verb) + tenant, and per-route
// ParseUUIDPathParameters("billing-packages") + RegisterBillingPackageRoutes.
//
// MUST-NOT-PARALLELIZE (same rationale as buildHumaPackageApp): libProblem.Install()
// swaps the process-global huma.NewError hook and Huma validation uses process-global
// sync.Pools — concurrent builds/requests cross-contaminate.
func buildHumaBillingPackageApp(t *testing.T, handler *BillingPackageHandler, authOK bool) *fiber.App {
	t.Helper()

	f := fiber.New(fiber.Config{
		ErrorHandler: pkgHTTP.CanonicalFiberErrorHandler,
	})

	libProblem.Install()

	apiV2 := f.Group("/v2")

	apiV2.Use(feesAuthShim(authOK))

	parse := pkgHTTP.ParseUUIDPathParameters("billing-packages")

	listPath := "/organizations/:organization_id/ledgers/:ledger_id/billing-packages"
	idPath := listPath + "/:id"

	apiV2.Post(listPath, parse)
	apiV2.Get(listPath, parse)
	apiV2.Get(idPath, parse)
	apiV2.Patch(idPath, parse)
	apiV2.Delete(idPath, parse)

	hAPI := openapi.New(f, apiV2, openapi.Config{Title: "ledger-test", Version: "test", Servers: []string{"/v2"}})

	RegisterBillingPackageRoutes(hAPI, handler, v2OpSuffix)

	return f
}

// buildHumaBillingCalculateApp mounts the single ledger-scoped billing-calculate Huma
// operation.
func buildHumaBillingCalculateApp(t *testing.T, handler *BillingCalculateHandler, authOK bool) *fiber.App {
	t.Helper()

	f := fiber.New(fiber.Config{
		ErrorHandler: pkgHTTP.CanonicalFiberErrorHandler,
	})

	libProblem.Install()

	apiV2 := f.Group("/v2")

	apiV2.Use(feesAuthShim(authOK))
	apiV2.Post("/organizations/:organization_id/ledgers/:ledger_id/billing/calculate", pkgHTTP.ParseUUIDPathParameters("billing-calculate"))

	hAPI := openapi.New(f, apiV2, openapi.Config{Title: "ledger-test", Version: "test", Servers: []string{"/v2"}})

	RegisterBillingCalculateRoutes(hAPI, handler, v2OpSuffix)

	return f
}

// billingPkgV2URL builds the ledger-scoped billing-package collection URL for a given
// organization and ledger.
func billingPkgV2URL(orgID, ledgerID string) string {
	return "/v2/organizations/" + orgID + "/ledgers/" + ledgerID + "/billing-packages"
}

// billingCalcV2URL builds the ledger-scoped billing-calculate URL.
func billingCalcV2URL(orgID, ledgerID string) string {
	return "/v2/organizations/" + orgID + "/ledgers/" + ledgerID + "/billing/calculate"
}

// validBillingPackageJSON is a decode-valid create-billing-package body: label + type
// + ledgerId are the fields the create path stamps/forwards. DecodeValidateBody runs
// ValidateStruct (no struct tags on BillingPackage → no-op) + unknown-field check;
// business Validate() runs in the service layer (stubbed), so this clears decode. The
// ledger is validLedgerUUID(), so the ledger-scoped create guard admits it when the
// path names the same ledger.
func validBillingPackageJSON() string {
	return `{"label":"Monthly Volume","type":"volume","ledgerId":"` + validLedgerUUID() + `"}`
}

func TestCreateBillingPackage_Success(t *testing.T) {
	orgID := uuid.Must(libCommons.GenerateUUIDv7())

	stub := &stubBillingPackageService{
		createResult: &model.BillingPackage{ID: uuid.NewString(), Label: "Monthly Volume", Type: "volume"},
	}
	handler := &BillingPackageHandler{Service: stub}

	app := buildHumaBillingPackageApp(t, handler, true)

	req := httptest.NewRequest(http.MethodPost, billingPkgV2URL(orgID.String(), validLedgerUUID()), bytes.NewBufferString(validBillingPackageJSON()))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req, fiber.TestConfig{Timeout: 0})
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	respBody, _ := io.ReadAll(resp.Body)
	assert.Equal(t, http.StatusCreated, resp.StatusCode, "body: %s", string(respBody))
	assert.True(t, stub.createCalled, "service.CreateBillingPackage must be invoked")
	assert.Equal(t, orgID.String(), stub.gotCreate.OrganizationID, "handler must stamp path org onto the payload")
	assert.NotContains(t, string(respBody), "$schema")

	var got map[string]any
	require.NoError(t, json.Unmarshal(respBody, &got), "body: %s", string(respBody))
	assert.Equal(t, "Monthly Volume", got["label"])
}

func TestCreateBillingPackage_AuthPreserved(t *testing.T) {
	orgID := uuid.Must(libCommons.GenerateUUIDv7())

	handler := &BillingPackageHandler{Service: &stubBillingPackageService{}}
	app := buildHumaBillingPackageApp(t, handler, false)

	req := httptest.NewRequest(http.MethodPost, billingPkgV2URL(orgID.String(), validLedgerUUID()), bytes.NewBufferString(`{}`))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req, fiber.TestConfig{Timeout: 0})
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode, "auth middleware must reject before Huma")
}

func TestCreateBillingPackage_MalformedBody_Canonical400(t *testing.T) {
	orgID := uuid.Must(libCommons.GenerateUUIDv7())

	handler := &BillingPackageHandler{Service: &stubBillingPackageService{}}
	app := buildHumaBillingPackageApp(t, handler, true)

	req := httptest.NewRequest(http.MethodPost, billingPkgV2URL(orgID.String(), validLedgerUUID()), bytes.NewReader([]byte("{not valid json")))
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

func TestGetBillingPackageByID_Success(t *testing.T) {
	orgID := uuid.Must(libCommons.GenerateUUIDv7())
	bpID := uuid.Must(libCommons.GenerateUUIDv7())

	stub := &stubBillingPackageService{getByIDResult: &model.BillingPackage{ID: bpID.String(), Label: "Standard"}}
	handler := &BillingPackageHandler{Service: stub}

	app := buildHumaBillingPackageApp(t, handler, true)

	req := httptest.NewRequest(http.MethodGet, billingPkgV2URL(orgID.String(), validLedgerUUID())+"/"+bpID.String(), nil)
	resp, err := app.Test(req, fiber.TestConfig{Timeout: 0})
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	respBody, _ := io.ReadAll(resp.Body)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, bpID, stub.gotGetByIDID)
	assert.Equal(t, orgID, stub.gotGetByIDOrg)

	var got map[string]any
	require.NoError(t, json.Unmarshal(respBody, &got), "body: %s", string(respBody))
	assert.Equal(t, bpID.String(), got["id"])
}

func TestGetBillingPackageByID_BadUUID_Canonical400(t *testing.T) {
	orgID := uuid.Must(libCommons.GenerateUUIDv7())

	handler := &BillingPackageHandler{Service: &stubBillingPackageService{}}
	app := buildHumaBillingPackageApp(t, handler, true)

	req := httptest.NewRequest(http.MethodGet, billingPkgV2URL(orgID.String(), validLedgerUUID())+"/not-a-uuid", nil)
	resp, err := app.Test(req, fiber.TestConfig{Timeout: 0})
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	respBody, _ := io.ReadAll(resp.Body)
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode, "bad path UUID stays canonical 400 — no native 422")

	var got map[string]any
	require.NoError(t, json.Unmarshal(respBody, &got), "body: %s", string(respBody))
	assert.Equal(t, constant.ErrInvalidPathParameter.Error(), got["code"])
}

func TestGetAllBillingPackages_Success(t *testing.T) {
	orgID := uuid.Must(libCommons.GenerateUUIDv7())
	ledgerID := uuid.Must(libCommons.GenerateUUIDv7())

	stub := &stubBillingPackageService{
		getAllResult: []*model.BillingPackage{{ID: uuid.NewString()}},
		getAllTotal:  1,
	}
	handler := &BillingPackageHandler{Service: stub}

	app := buildHumaBillingPackageApp(t, handler, true)

	// The ledger-scoped listing pins to the path ledger; the ledgerId query is refused
	// (see TestFeesV2_ListsRefuseTheLedgerQueryParameter), so it is absent here.
	req := httptest.NewRequest(http.MethodGet, billingPkgV2URL(orgID.String(), ledgerID.String())+"?limit=5&page=2&type=volume", nil)
	resp, err := app.Test(req, fiber.TestConfig{Timeout: 0})
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	respBody, _ := io.ReadAll(resp.Body)
	assert.Equal(t, http.StatusOK, resp.StatusCode, "body: %s", string(respBody))
	assert.Equal(t, orgID, stub.gotGetAllOrg)
	assert.Equal(t, 5, stub.gotGetAllLimit, "query binder must feed the parsed limit")
	assert.Equal(t, 2, stub.gotGetAllPage)
	assert.Equal(t, "volume", stub.gotGetAllType)
	require.NotNil(t, stub.gotGetAllLedger, "the ledger-scoped listing must be pinned to the path ledger")
	assert.Equal(t, ledgerID, *stub.gotGetAllLedger, "the listing must scope to the ledger the path named")

	var got map[string]any
	require.NoError(t, json.Unmarshal(respBody, &got), "body: %s", string(respBody))
	assert.EqualValues(t, 5, got["limit"])
	assert.EqualValues(t, 2, got["page"])
	assert.EqualValues(t, 1, got["total"])
}

func TestGetAllBillingPackages_BadLimit_Canonical400(t *testing.T) {
	orgID := uuid.Must(libCommons.GenerateUUIDv7())

	handler := &BillingPackageHandler{Service: &stubBillingPackageService{}}
	app := buildHumaBillingPackageApp(t, handler, true)

	req := httptest.NewRequest(http.MethodGet, billingPkgV2URL(orgID.String(), validLedgerUUID())+"?limit=abc", nil)
	resp, err := app.Test(req, fiber.TestConfig{Timeout: 0})
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	respBody, _ := io.ReadAll(resp.Body)
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode, "bad query stays canonical 400 — no native 422")

	var got map[string]any
	require.NoError(t, json.Unmarshal(respBody, &got), "body: %s", string(respBody))
	assert.Equal(t, constant.ErrInvalidQueryParameter.Error(), got["code"])
}

func TestUpdateBillingPackage_Success(t *testing.T) {
	orgID := uuid.Must(libCommons.GenerateUUIDv7())
	bpID := uuid.Must(libCommons.GenerateUUIDv7())

	stub := &stubBillingPackageService{updateResult: &model.BillingPackage{ID: bpID.String(), Label: "Updated"}}
	handler := &BillingPackageHandler{Service: stub}

	app := buildHumaBillingPackageApp(t, handler, true)

	body := `{"label":"Updated"}`
	req := httptest.NewRequest(http.MethodPatch, billingPkgV2URL(orgID.String(), validLedgerUUID())+"/"+bpID.String(), bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req, fiber.TestConfig{Timeout: 0})
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	respBody, _ := io.ReadAll(resp.Body)
	assert.Equal(t, http.StatusOK, resp.StatusCode, "body: %s", string(respBody))
	assert.True(t, stub.updateCalled, "service.UpdateBillingPackage must be invoked")
	assert.Equal(t, bpID, stub.gotUpdateID)
	assert.Contains(t, stub.gotUpdateUpdates, "label", "ToMap must feed the merge-patch update set")

	var got map[string]any
	require.NoError(t, json.Unmarshal(respBody, &got), "body: %s", string(respBody))
	assert.Equal(t, bpID.String(), got["id"])
}

func TestUpdateBillingPackage_Empty_NothingToUpdate(t *testing.T) {
	orgID := uuid.Must(libCommons.GenerateUUIDv7())
	bpID := uuid.Must(libCommons.GenerateUUIDv7())

	stub := &stubBillingPackageService{}
	handler := &BillingPackageHandler{Service: stub}

	app := buildHumaBillingPackageApp(t, handler, true)

	req := httptest.NewRequest(http.MethodPatch, billingPkgV2URL(orgID.String(), validLedgerUUID())+"/"+bpID.String(), bytes.NewBufferString(`{}`))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req, fiber.TestConfig{Timeout: 0})
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	respBody, _ := io.ReadAll(resp.Body)
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode, "empty merge-patch → nothing-to-update (0183 is a 400), body: %s", string(respBody))
	assert.False(t, stub.updateCalled, "service must not be called on an empty update set")

	var got map[string]any
	require.NoError(t, json.Unmarshal(respBody, &got), "body: %s", string(respBody))
	assert.Equal(t, constant.ErrNothingToUpdate.Error(), got["code"])
}

func TestDeleteBillingPackage_204Empty(t *testing.T) {
	orgID := uuid.Must(libCommons.GenerateUUIDv7())
	bpID := uuid.Must(libCommons.GenerateUUIDv7())

	stub := &stubBillingPackageService{}
	handler := &BillingPackageHandler{Service: stub}

	app := buildHumaBillingPackageApp(t, handler, true)

	req := httptest.NewRequest(http.MethodDelete, billingPkgV2URL(orgID.String(), validLedgerUUID())+"/"+bpID.String(), nil)
	resp, err := app.Test(req, fiber.TestConfig{Timeout: 0})
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	respBody, _ := io.ReadAll(resp.Body)
	assert.Equal(t, http.StatusNoContent, resp.StatusCode)
	assert.Empty(t, respBody, "DELETE 204 must have an empty body")
	assert.True(t, stub.deleteCalled)
	assert.Equal(t, bpID, stub.gotDeleteID)
}

// doBillingPkgRequest issues one request against the ledger-scoped billing-package
// app and returns the status, the decoded problem/resource body, and the raw bytes.
func doBillingPkgRequest(t *testing.T, handler *BillingPackageHandler, method, url, body string) (int, map[string]any, []byte) {
	t.Helper()

	app := buildHumaBillingPackageApp(t, handler, true)

	var req *http.Request
	if body == "" {
		req = httptest.NewRequest(method, url, nil)
	} else {
		req = httptest.NewRequest(method, url, bytes.NewBufferString(body))
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := app.Test(req, fiber.TestConfig{Timeout: 0})
	require.NoError(t, err)

	defer func() { _ = resp.Body.Close() }()

	raw, _ := io.ReadAll(resp.Body)

	var got map[string]any
	if len(raw) > 0 {
		require.NoError(t, json.Unmarshal(raw, &got), "body: %s", string(raw))
	}

	return resp.StatusCode, got, raw
}

func TestCreateBillingPackage_ServiceError_Mapped(t *testing.T) {
	orgID := uuid.Must(libCommons.GenerateUUIDv7())

	stub := &stubBillingPackageService{
		createErr: pkg.ValidateBusinessError(constant.ErrBillingRouteOverlap, constant.EntityBillingPackage),
	}
	handler := &BillingPackageHandler{Service: stub}

	status, got, raw := doBillingPkgRequest(t, handler, http.MethodPost,
		billingPkgV2URL(orgID.String(), validLedgerUUID()), validBillingPackageJSON())

	assert.Equal(t, http.StatusConflict, status, "body: %s", string(raw))
	assert.Equal(t, constant.ErrBillingRouteOverlap.Error(), got["code"])
	assert.True(t, stub.createCalled)
}

func TestCreateBillingPackage_NilResult_500(t *testing.T) {
	orgID := uuid.Must(libCommons.GenerateUUIDv7())

	// A service that answers (nil, nil) is a contract breach, not a business
	// outcome: the core turns it into an internal error rather than rendering a
	// null resource body with a 201.
	stub := &stubBillingPackageService{createResult: nil, createErr: nil}
	handler := &BillingPackageHandler{Service: stub}

	status, got, raw := doBillingPkgRequest(t, handler, http.MethodPost,
		billingPkgV2URL(orgID.String(), validLedgerUUID()), validBillingPackageJSON())

	assert.Equal(t, http.StatusInternalServerError, status, "body: %s", string(raw))
	assert.Equal(t, constant.ErrInternalServer.Error(), got["code"])
}

func TestGetAllBillingPackages_LimitAboveMax_Canonical400(t *testing.T) {
	orgID := uuid.Must(libCommons.GenerateUUIDv7())

	stub := &stubBillingPackageService{}
	handler := &BillingPackageHandler{Service: stub}

	status, got, raw := doBillingPkgRequest(t, handler, http.MethodGet,
		billingPkgV2URL(orgID.String(), validLedgerUUID())+"?limit=101", "")

	assert.Equal(t, http.StatusBadRequest, status, "body: %s", string(raw))
	assert.Equal(t, constant.ErrPaginationLimitExceeded.Error(), got["code"])
	assert.Equal(t, uuid.Nil, stub.gotGetAllOrg, "the limit ceiling must be enforced before the service call")
}

func TestGetAllBillingPackages_LimitBelowOne_Canonical400(t *testing.T) {
	orgID := uuid.Must(libCommons.GenerateUUIDv7())

	handler := &BillingPackageHandler{Service: &stubBillingPackageService{}}

	status, got, raw := doBillingPkgRequest(t, handler, http.MethodGet,
		billingPkgV2URL(orgID.String(), validLedgerUUID())+"?limit=0", "")

	assert.Equal(t, http.StatusBadRequest, status, "body: %s", string(raw))
	assert.Equal(t, constant.ErrInvalidQueryParameter.Error(), got["code"])
}

func TestGetAllBillingPackages_BadPage_Canonical400(t *testing.T) {
	orgID := uuid.Must(libCommons.GenerateUUIDv7())

	handler := &BillingPackageHandler{Service: &stubBillingPackageService{}}

	status, got, raw := doBillingPkgRequest(t, handler, http.MethodGet,
		billingPkgV2URL(orgID.String(), validLedgerUUID())+"?page=xyz", "")

	assert.Equal(t, http.StatusBadRequest, status, "body: %s", string(raw))
	assert.Equal(t, constant.ErrInvalidQueryParameter.Error(), got["code"])
}

func TestGetAllBillingPackages_ServiceError_Mapped(t *testing.T) {
	orgID := uuid.Must(libCommons.GenerateUUIDv7())

	stub := &stubBillingPackageService{
		getAllErr: pkg.ValidateBusinessError(constant.ErrBillingCalculationFailed, constant.EntityBillingPackage, "boom"),
	}
	handler := &BillingPackageHandler{Service: stub}

	status, got, raw := doBillingPkgRequest(t, handler, http.MethodGet,
		billingPkgV2URL(orgID.String(), validLedgerUUID()), "")

	assert.Equal(t, http.StatusInternalServerError, status, "body: %s", string(raw))
	assert.Equal(t, constant.ErrBillingCalculationFailed.Error(), got["code"])
}

func TestGetBillingPackageByID_NotFound_404(t *testing.T) {
	orgID := uuid.Must(libCommons.GenerateUUIDv7())
	bpID := uuid.Must(libCommons.GenerateUUIDv7())

	stub := &stubBillingPackageService{
		getByIDErr: pkg.ValidateBusinessError(constant.ErrBillingPackageNotFound, constant.EntityBillingPackage, bpID.String()),
	}
	handler := &BillingPackageHandler{Service: stub}

	status, got, raw := doBillingPkgRequest(t, handler, http.MethodGet,
		billingPkgV2URL(orgID.String(), validLedgerUUID())+"/"+bpID.String(), "")

	assert.Equal(t, http.StatusNotFound, status, "body: %s", string(raw))
	assert.Equal(t, constant.ErrBillingPackageNotFound.Error(), got["code"])
}

func TestUpdateBillingPackage_BlankLabel_Canonical400(t *testing.T) {
	orgID := uuid.Must(libCommons.GenerateUUIDv7())
	bpID := uuid.Must(libCommons.GenerateUUIDv7())

	stub := &stubBillingPackageService{}
	handler := &BillingPackageHandler{Service: stub}

	status, got, raw := doBillingPkgRequest(t, handler, http.MethodPatch,
		billingPkgV2URL(orgID.String(), validLedgerUUID())+"/"+bpID.String(), `{"label":"   "}`)

	assert.Equal(t, http.StatusBadRequest, status, "body: %s", string(raw))
	assert.Equal(t, constant.ErrMissingFieldsInRequest.Error(), got["code"])
	assert.False(t, stub.updateCalled, "merge-patch validation must short-circuit before the service")
}

func TestUpdateBillingPackage_NotFound_404(t *testing.T) {
	orgID := uuid.Must(libCommons.GenerateUUIDv7())
	bpID := uuid.Must(libCommons.GenerateUUIDv7())

	stub := &stubBillingPackageService{
		updateErr: pkg.ValidateBusinessError(constant.ErrBillingPackageNotFound, constant.EntityBillingPackage, bpID.String()),
	}
	handler := &BillingPackageHandler{Service: stub}

	status, got, raw := doBillingPkgRequest(t, handler, http.MethodPatch,
		billingPkgV2URL(orgID.String(), validLedgerUUID())+"/"+bpID.String(), `{"label":"X"}`)

	assert.Equal(t, http.StatusNotFound, status, "body: %s", string(raw))
	assert.Equal(t, constant.ErrBillingPackageNotFound.Error(), got["code"])
	assert.True(t, stub.updateCalled)
}

func TestDeleteBillingPackage_NotFound_404(t *testing.T) {
	orgID := uuid.Must(libCommons.GenerateUUIDv7())
	bpID := uuid.Must(libCommons.GenerateUUIDv7())

	stub := &stubBillingPackageService{
		deleteErr: pkg.ValidateBusinessError(constant.ErrBillingPackageNotFound, constant.EntityBillingPackage, bpID.String()),
	}
	handler := &BillingPackageHandler{Service: stub}

	status, got, raw := doBillingPkgRequest(t, handler, http.MethodDelete,
		billingPkgV2URL(orgID.String(), validLedgerUUID())+"/"+bpID.String(), "")

	assert.Equal(t, http.StatusNotFound, status, "body: %s", string(raw))
	assert.Equal(t, constant.ErrBillingPackageNotFound.Error(), got["code"])
	assert.True(t, stub.deleteCalled)
}

func validBillingCalculateJSON(ledgerID string) string {
	return `{"ledgerId":"` + ledgerID + `","period":"2026-01","type":"volume"}`
}

func TestCalculateBilling_Success(t *testing.T) {
	orgID := uuid.Must(libCommons.GenerateUUIDv7())
	ledgerID := uuid.Must(libCommons.GenerateUUIDv7())

	stub := &stubBillingCalculateService{
		result: &model.BillingCalculateResponse{Summary: model.BillingCalculateSummary{TotalResults: 3}},
	}
	handler := &BillingCalculateHandler{Service: stub}

	app := buildHumaBillingCalculateApp(t, handler, true)

	req := httptest.NewRequest(http.MethodPost, billingCalcV2URL(orgID.String(), ledgerID.String()), bytes.NewBufferString(validBillingCalculateJSON(ledgerID.String())))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req, fiber.TestConfig{Timeout: 0})
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	respBody, _ := io.ReadAll(resp.Body)
	assert.Equal(t, http.StatusOK, resp.StatusCode, "body: %s", string(respBody))
	assert.True(t, stub.called, "service.Calculate must be invoked")
	assert.Equal(t, orgID.String(), stub.got.OrganizationID, "handler must stamp path org onto the request")
	assert.NotContains(t, string(respBody), "$schema")

	var got map[string]any
	require.NoError(t, json.Unmarshal(respBody, &got), "body: %s", string(respBody))
	summary, ok := got["summary"].(map[string]any)
	require.True(t, ok, "response must carry the summary envelope, body: %s", string(respBody))
	assert.EqualValues(t, 3, summary["totalResults"])
}

func TestCalculateBilling_AuthPreserved(t *testing.T) {
	orgID := uuid.Must(libCommons.GenerateUUIDv7())

	handler := &BillingCalculateHandler{Service: &stubBillingCalculateService{}}
	app := buildHumaBillingCalculateApp(t, handler, false)

	req := httptest.NewRequest(http.MethodPost, billingCalcV2URL(orgID.String(), validLedgerUUID()), bytes.NewBufferString(`{}`))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req, fiber.TestConfig{Timeout: 0})
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode, "auth middleware must reject before Huma")
}

func TestCalculateBilling_MissingLedger_Canonical400(t *testing.T) {
	orgID := uuid.Must(libCommons.GenerateUUIDv7())

	handler := &BillingCalculateHandler{Service: &stubBillingCalculateService{}}
	app := buildHumaBillingCalculateApp(t, handler, true)

	// ledgerId omitted → the fee body validator (WithBodyTracing/DecodeValidateBody,
	// which the shell preserves via decodeFeeBodyInSpan) rejects it on the
	// `validate:"required"` struct tag with ErrMissingFieldsInRequest (0009) BEFORE the
	// body-ledger-match guard and the handler-level validateBillingCalculateRequest run.
	// This is byte-identical to the Fiber path — a native Huma 422 must NOT appear.
	req := httptest.NewRequest(http.MethodPost, billingCalcV2URL(orgID.String(), validLedgerUUID()), bytes.NewBufferString(`{"period":"2026-01"}`))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req, fiber.TestConfig{Timeout: 0})
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	respBody, _ := io.ReadAll(resp.Body)
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode, "missing ledgerId stays canonical 400, body: %s", string(respBody))
	assert.Equal(t, "application/problem+json", resp.Header.Get("Content-Type"))

	var got map[string]any
	require.NoError(t, json.Unmarshal(respBody, &got), "body: %s", string(respBody))
	assert.Equal(t, constant.ErrMissingFieldsInRequest.Error(), got["code"])
}

func TestCalculateBilling_MalformedLedger_Canonical400(t *testing.T) {
	orgID := uuid.Must(libCommons.GenerateUUIDv7())

	handler := &BillingCalculateHandler{Service: &stubBillingCalculateService{}}
	app := buildHumaBillingCalculateApp(t, handler, true)

	// ledgerId present but not a UUID → clears the `required` struct tag, so the
	// ledger-scoped body-match guard (requireBodyLedgerMatchesPath) rejects it with
	// ErrInvalidLedgerID (0203) when uuid.Parse fails, BEFORE the service call.
	req := httptest.NewRequest(http.MethodPost, billingCalcV2URL(orgID.String(), validLedgerUUID()), bytes.NewBufferString(`{"ledgerId":"not-a-uuid","period":"2026-01"}`))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req, fiber.TestConfig{Timeout: 0})
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	respBody, _ := io.ReadAll(resp.Body)
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode, "malformed ledgerId stays canonical 400, body: %s", string(respBody))

	var got map[string]any
	require.NoError(t, json.Unmarshal(respBody, &got), "body: %s", string(respBody))
	assert.Equal(t, constant.ErrInvalidLedgerID.Error(), got["code"])
}

func TestCalculateBilling_ServiceError_Mapped(t *testing.T) {
	orgID := uuid.Must(libCommons.GenerateUUIDv7())
	ledgerID := uuid.Must(libCommons.GenerateUUIDv7())

	stub := &stubBillingCalculateService{err: pkg.ValidateBusinessError(constant.ErrInvalidPathParameter, "", "packageId")}
	handler := &BillingCalculateHandler{Service: stub}

	app := buildHumaBillingCalculateApp(t, handler, true)

	req := httptest.NewRequest(http.MethodPost, billingCalcV2URL(orgID.String(), ledgerID.String()), bytes.NewBufferString(validBillingCalculateJSON(ledgerID.String())))
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

//
// The CalculateBilling fiber.Ctx terminal was deleted with the Huma migration. Its
// tests covered the request validators and the nil-result guard, which the Huma
// suite did not reach.

func TestValidateBillingPeriod(t *testing.T) {
	// Pure validator: the calendar branches are cheaper and clearer exercised
	// directly than through six HTTP round-trips.
	tests := []struct {
		name    string
		period  string
		wantErr bool
	}{
		{"empty is rejected", "", true},
		{"full date is accepted", "2026-03-14", false},
		{"month is accepted", "2026-03", false},
		{"valid ISO week is accepted", "2026-W07", false},
		{"nonexistent ISO week is rejected", "2026-W99", true},
		{"free text is rejected", "last-march", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateBillingPeriod(tt.period)
			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), constant.ErrInvalidBillingPeriod.Error())

				return
			}

			assert.NoError(t, err)
		})
	}
}

func TestValidateBillingCalculateRequest(t *testing.T) {
	// Pure validator. Each case isolates one guard; the period cases live in
	// TestValidateBillingPeriod.
	ledgerID := uuid.Must(libCommons.GenerateUUIDv7()).String()
	orgID := uuid.Must(libCommons.GenerateUUIDv7()).String()

	base := func() *model.BillingCalculateRequest {
		return &model.BillingCalculateRequest{OrganizationID: orgID, LedgerID: ledgerID, Period: "2026-03"}
	}

	tests := []struct {
		name    string
		mutate  func(r *model.BillingCalculateRequest)
		wantErr string
	}{
		{"valid request passes", func(*model.BillingCalculateRequest) {}, ""},
		{"missing organization", func(r *model.BillingCalculateRequest) { r.OrganizationID = "" }, constant.ErrFeeInvalidHeaderParameter.Error()},
		{"missing ledger", func(r *model.BillingCalculateRequest) { r.LedgerID = "" }, constant.ErrInvalidLedgerID.Error()},
		{"malformed ledger", func(r *model.BillingCalculateRequest) { r.LedgerID = "not-a-uuid" }, constant.ErrInvalidLedgerID.Error()},
		{"unknown package type", func(r *model.BillingCalculateRequest) { r.Type = "subscription" }, constant.ErrInvalidBillingPackageType.Error()},
		{"volume type passes", func(r *model.BillingCalculateRequest) { r.Type = model.BillingPackageTypeVolume }, ""},
		{"maintenance type passes", func(r *model.BillingCalculateRequest) { r.Type = model.BillingPackageTypeMaintenance }, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := base()
			tt.mutate(req)

			err := validateBillingCalculateRequest(req)
			if tt.wantErr == "" {
				assert.NoError(t, err)

				return
			}

			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
		})
	}
}

func TestCalculateBilling_NilResult_500(t *testing.T) {
	// The service returning (nil, nil) must not surface as a 200 with an empty
	// body; calculateBilling turns it into a canonical internal error.
	orgID := uuid.Must(libCommons.GenerateUUIDv7())
	ledgerID := uuid.Must(libCommons.GenerateUUIDv7())

	stub := &stubBillingCalculateService{result: nil}
	handler := &BillingCalculateHandler{Service: stub}

	app := buildHumaBillingCalculateApp(t, handler, true)

	req := httptest.NewRequest(http.MethodPost, billingCalcV2URL(orgID.String(), ledgerID.String()), bytes.NewBufferString(validBillingCalculateJSON(ledgerID.String())))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req, fiber.TestConfig{Timeout: 0})
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	respBody, _ := io.ReadAll(resp.Body)
	assert.Equal(t, http.StatusInternalServerError, resp.StatusCode, "body: %s", string(respBody))
	assert.True(t, stub.called)
}

func TestCalculateBilling_InvalidPeriod_Canonical400(t *testing.T) {
	// The period guard runs inside calculateBilling, before the service call.
	orgID := uuid.Must(libCommons.GenerateUUIDv7())
	ledgerID := uuid.Must(libCommons.GenerateUUIDv7())

	stub := &stubBillingCalculateService{}
	handler := &BillingCalculateHandler{Service: stub}

	app := buildHumaBillingCalculateApp(t, handler, true)

	body := `{"ledgerId":"` + ledgerID.String() + `","period":"2026-W99"}`
	req := httptest.NewRequest(http.MethodPost, billingCalcV2URL(orgID.String(), ledgerID.String()), bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req, fiber.TestConfig{Timeout: 0})
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	respBody, _ := io.ReadAll(resp.Body)
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode, "body: %s", string(respBody))
	assert.False(t, stub.called, "an invalid period must never reach the service")

	var got map[string]any
	require.NoError(t, json.Unmarshal(respBody, &got), "body: %s", string(respBody))
	assert.Equal(t, constant.ErrInvalidBillingPeriod.Error(), got["code"])
}
