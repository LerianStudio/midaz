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

	"github.com/LerianStudio/midaz/v4/components/ledger/pkg/feeshared/model"
	"github.com/LerianStudio/midaz/v4/pkg"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	pkgHTTP "github.com/LerianStudio/midaz/v4/pkg/net/http"
)

// The stubBillingPackageService / stubBillingCalculateService fakes live in
// fees_billing_handlers_test.go; these Huma tests reuse them (mirroring how the fee
// Huma tests reuse stubPackageService/stubFeeService).

// buildHumaBillingPackageApp mounts the five billing-package Huma operations on a /v1
// group, mirroring production (fees_routes.go/unified-server.go): problem.Install()
// before any huma.Register, the Huma API built with openapi.New over a /v1 group, an
// auth-shim standing in for auth.Authorize("plugin-fees","billing-packages",verb) +
// tenant, and per-route ParseUUIDPathParameters("billing-packages") +
// RegisterBillingPackageRoutes.
//
// MUST-NOT-PARALLELIZE (same rationale as buildHumaPackageApp): libProblem.Install()
// swaps the process-global huma.NewError hook and Huma validation uses process-global
// sync.Pools — concurrent builds/requests cross-contaminate.
func buildHumaBillingPackageApp(t *testing.T, handler *BillingPackageHandler, authOK bool) *fiber.App {
	t.Helper()

	f := fiber.New(fiber.Config{
		DisableStartupMessage: true,
		ErrorHandler:          pkgHTTP.CanonicalFiberErrorHandler,
	})

	libProblem.Install()

	apiV1 := f.Group("/v1")

	apiV1.Use(feesAuthShim(authOK))

	parse := pkgHTTP.ParseUUIDPathParameters("billing-packages")

	listPath := "/organizations/:organization_id/billing-packages"
	idPath := listPath + "/:id"

	apiV1.Post(listPath, parse)
	apiV1.Get(listPath, parse)
	apiV1.Get(idPath, parse)
	apiV1.Patch(idPath, parse)
	apiV1.Delete(idPath, parse)

	hAPI := openapi.New(f, apiV1, openapi.Config{Title: "ledger-test", Version: "test", Servers: []string{"/v1"}})

	RegisterBillingPackageRoutes(hAPI, handler)

	return f
}

// buildHumaBillingCalculateApp mounts the single billing-calculate Huma operation.
func buildHumaBillingCalculateApp(t *testing.T, handler *BillingCalculateHandler, authOK bool) *fiber.App {
	t.Helper()

	f := fiber.New(fiber.Config{
		DisableStartupMessage: true,
		ErrorHandler:          pkgHTTP.CanonicalFiberErrorHandler,
	})

	libProblem.Install()

	apiV1 := f.Group("/v1")

	apiV1.Use(feesAuthShim(authOK))
	apiV1.Post("/organizations/:organization_id/billing/calculate", pkgHTTP.ParseUUIDPathParameters("billing-calculate"))

	hAPI := openapi.New(f, apiV1, openapi.Config{Title: "ledger-test", Version: "test", Servers: []string{"/v1"}})

	RegisterBillingCalculateRoutes(hAPI, handler)

	return f
}

// validBillingPackageJSON is a decode-valid create-billing-package body: label + type
// + ledgerId are the fields the create path stamps/forwards. DecodeValidateBody runs
// ValidateStruct (no struct tags on BillingPackage → no-op) + unknown-field check;
// business Validate() runs in the service layer (stubbed), so this clears decode.
func validBillingPackageJSON() string {
	return `{"label":"Monthly Volume","type":"volume","ledgerId":"` + validLedgerUUID() + `"}`
}

func TestHuma_CreateBillingPackage_Success(t *testing.T) {
	orgID := uuid.New()

	stub := &stubBillingPackageService{
		createResult: &model.BillingPackage{ID: uuid.NewString(), Label: "Monthly Volume", Type: "volume"},
	}
	handler := &BillingPackageHandler{Service: stub}

	app := buildHumaBillingPackageApp(t, handler, true)

	req := httptest.NewRequest(http.MethodPost, "/v1/organizations/"+orgID.String()+"/billing-packages", bytes.NewBufferString(validBillingPackageJSON()))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req, -1)
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

func TestHuma_CreateBillingPackage_AuthPreserved(t *testing.T) {
	orgID := uuid.New()

	handler := &BillingPackageHandler{Service: &stubBillingPackageService{}}
	app := buildHumaBillingPackageApp(t, handler, false)

	req := httptest.NewRequest(http.MethodPost, "/v1/organizations/"+orgID.String()+"/billing-packages", bytes.NewBufferString(`{}`))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req, -1)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode, "auth middleware must reject before Huma")
}

func TestHuma_CreateBillingPackage_MalformedBody_Canonical400(t *testing.T) {
	orgID := uuid.New()

	handler := &BillingPackageHandler{Service: &stubBillingPackageService{}}
	app := buildHumaBillingPackageApp(t, handler, true)

	req := httptest.NewRequest(http.MethodPost, "/v1/organizations/"+orgID.String()+"/billing-packages", bytes.NewReader([]byte("{not valid json")))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req, -1)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	respBody, _ := io.ReadAll(resp.Body)
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode, "malformed body stays 400 — no native 422")
	assert.Equal(t, "application/problem+json", resp.Header.Get("Content-Type"))

	var got map[string]any
	require.NoError(t, json.Unmarshal(respBody, &got), "body: %s", string(respBody))
	assert.Equal(t, constant.ErrInvalidRequestBody.Error(), got["code"], "malformed-body code preserved (0094)")
}

func TestHuma_GetBillingPackageByID_Success(t *testing.T) {
	orgID := uuid.New()
	bpID := uuid.New()

	stub := &stubBillingPackageService{getByIDResult: &model.BillingPackage{ID: bpID.String(), Label: "Standard"}}
	handler := &BillingPackageHandler{Service: stub}

	app := buildHumaBillingPackageApp(t, handler, true)

	req := httptest.NewRequest(http.MethodGet, "/v1/organizations/"+orgID.String()+"/billing-packages/"+bpID.String(), nil)
	resp, err := app.Test(req, -1)
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

func TestHuma_GetBillingPackageByID_BadUUID_Canonical400(t *testing.T) {
	orgID := uuid.New()

	handler := &BillingPackageHandler{Service: &stubBillingPackageService{}}
	app := buildHumaBillingPackageApp(t, handler, true)

	req := httptest.NewRequest(http.MethodGet, "/v1/organizations/"+orgID.String()+"/billing-packages/not-a-uuid", nil)
	resp, err := app.Test(req, -1)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	respBody, _ := io.ReadAll(resp.Body)
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode, "bad path UUID stays canonical 400 — no native 422")

	var got map[string]any
	require.NoError(t, json.Unmarshal(respBody, &got), "body: %s", string(respBody))
	assert.Equal(t, constant.ErrInvalidPathParameter.Error(), got["code"])
}

func TestHuma_GetAllBillingPackages_Success(t *testing.T) {
	orgID := uuid.New()
	ledgerID := uuid.New()

	stub := &stubBillingPackageService{
		getAllResult: []*model.BillingPackage{{ID: uuid.NewString()}},
		getAllTotal:  1,
	}
	handler := &BillingPackageHandler{Service: stub}

	app := buildHumaBillingPackageApp(t, handler, true)

	req := httptest.NewRequest(http.MethodGet, "/v1/organizations/"+orgID.String()+"/billing-packages?limit=5&page=2&type=volume&ledgerId="+ledgerID.String(), nil)
	resp, err := app.Test(req, -1)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	respBody, _ := io.ReadAll(resp.Body)
	assert.Equal(t, http.StatusOK, resp.StatusCode, "body: %s", string(respBody))
	assert.Equal(t, orgID, stub.gotGetAllOrg)
	assert.Equal(t, 5, stub.gotGetAllLimit, "query binder must feed the parsed limit")
	assert.Equal(t, 2, stub.gotGetAllPage)
	assert.Equal(t, "volume", stub.gotGetAllType)
	require.NotNil(t, stub.gotGetAllLedger)
	assert.Equal(t, ledgerID, *stub.gotGetAllLedger)

	var got map[string]any
	require.NoError(t, json.Unmarshal(respBody, &got), "body: %s", string(respBody))
	assert.EqualValues(t, 5, got["limit"])
	assert.EqualValues(t, 2, got["page"])
	assert.EqualValues(t, 1, got["total"])
}

func TestHuma_GetAllBillingPackages_BadLimit_Canonical400(t *testing.T) {
	orgID := uuid.New()

	handler := &BillingPackageHandler{Service: &stubBillingPackageService{}}
	app := buildHumaBillingPackageApp(t, handler, true)

	req := httptest.NewRequest(http.MethodGet, "/v1/organizations/"+orgID.String()+"/billing-packages?limit=abc", nil)
	resp, err := app.Test(req, -1)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	respBody, _ := io.ReadAll(resp.Body)
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode, "bad query stays canonical 400 — no native 422")

	var got map[string]any
	require.NoError(t, json.Unmarshal(respBody, &got), "body: %s", string(respBody))
	assert.Equal(t, constant.ErrInvalidQueryParameter.Error(), got["code"])
}

func TestHuma_GetAllBillingPackages_BadLedgerID_Canonical400(t *testing.T) {
	orgID := uuid.New()

	handler := &BillingPackageHandler{Service: &stubBillingPackageService{}}
	app := buildHumaBillingPackageApp(t, handler, true)

	req := httptest.NewRequest(http.MethodGet, "/v1/organizations/"+orgID.String()+"/billing-packages?ledgerId=not-a-uuid", nil)
	resp, err := app.Test(req, -1)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	respBody, _ := io.ReadAll(resp.Body)
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode, "bad ledgerId stays canonical 400")

	var got map[string]any
	require.NoError(t, json.Unmarshal(respBody, &got), "body: %s", string(respBody))
	assert.Equal(t, constant.ErrInvalidQueryParameter.Error(), got["code"])
}

func TestHuma_UpdateBillingPackage_Success(t *testing.T) {
	orgID := uuid.New()
	bpID := uuid.New()

	stub := &stubBillingPackageService{updateResult: &model.BillingPackage{ID: bpID.String(), Label: "Updated"}}
	handler := &BillingPackageHandler{Service: stub}

	app := buildHumaBillingPackageApp(t, handler, true)

	body := `{"label":"Updated"}`
	req := httptest.NewRequest(http.MethodPatch, "/v1/organizations/"+orgID.String()+"/billing-packages/"+bpID.String(), bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req, -1)
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

func TestHuma_UpdateBillingPackage_Empty_NothingToUpdate(t *testing.T) {
	orgID := uuid.New()
	bpID := uuid.New()

	stub := &stubBillingPackageService{}
	handler := &BillingPackageHandler{Service: stub}

	app := buildHumaBillingPackageApp(t, handler, true)

	req := httptest.NewRequest(http.MethodPatch, "/v1/organizations/"+orgID.String()+"/billing-packages/"+bpID.String(), bytes.NewBufferString(`{}`))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req, -1)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	respBody, _ := io.ReadAll(resp.Body)
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode, "empty merge-patch → nothing-to-update (0183 is a 400), body: %s", string(respBody))
	assert.False(t, stub.updateCalled, "service must not be called on an empty update set")

	var got map[string]any
	require.NoError(t, json.Unmarshal(respBody, &got), "body: %s", string(respBody))
	assert.Equal(t, constant.ErrNothingToUpdate.Error(), got["code"])
}

func TestHuma_DeleteBillingPackage_204Empty(t *testing.T) {
	orgID := uuid.New()
	bpID := uuid.New()

	stub := &stubBillingPackageService{}
	handler := &BillingPackageHandler{Service: stub}

	app := buildHumaBillingPackageApp(t, handler, true)

	req := httptest.NewRequest(http.MethodDelete, "/v1/organizations/"+orgID.String()+"/billing-packages/"+bpID.String(), nil)
	resp, err := app.Test(req, -1)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	respBody, _ := io.ReadAll(resp.Body)
	assert.Equal(t, http.StatusNoContent, resp.StatusCode)
	assert.Empty(t, respBody, "DELETE 204 must have an empty body")
	assert.True(t, stub.deleteCalled)
	assert.Equal(t, bpID, stub.gotDeleteID)
}

func validBillingCalculateJSON(ledgerID string) string {
	return `{"ledgerId":"` + ledgerID + `","period":"2026-01","type":"volume"}`
}

func TestHuma_CalculateBilling_Success(t *testing.T) {
	orgID := uuid.New()
	ledgerID := uuid.New()

	stub := &stubBillingCalculateService{
		result: &model.BillingCalculateResponse{Summary: model.BillingCalculateSummary{TotalResults: 3}},
	}
	handler := &BillingCalculateHandler{Service: stub}

	app := buildHumaBillingCalculateApp(t, handler, true)

	req := httptest.NewRequest(http.MethodPost, "/v1/organizations/"+orgID.String()+"/billing/calculate", bytes.NewBufferString(validBillingCalculateJSON(ledgerID.String())))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req, -1)
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

func TestHuma_CalculateBilling_AuthPreserved(t *testing.T) {
	orgID := uuid.New()

	handler := &BillingCalculateHandler{Service: &stubBillingCalculateService{}}
	app := buildHumaBillingCalculateApp(t, handler, false)

	req := httptest.NewRequest(http.MethodPost, "/v1/organizations/"+orgID.String()+"/billing/calculate", bytes.NewBufferString(`{}`))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req, -1)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode, "auth middleware must reject before Huma")
}

func TestHuma_CalculateBilling_MissingLedger_Canonical400(t *testing.T) {
	orgID := uuid.New()

	handler := &BillingCalculateHandler{Service: &stubBillingCalculateService{}}
	app := buildHumaBillingCalculateApp(t, handler, true)

	// ledgerId omitted → the fee body validator (WithBodyTracing/DecodeValidateBody,
	// which the shell preserves via decodeFeeBodyInSpan) rejects it on the
	// `validate:"required"` struct tag with ErrMissingFieldsInRequest (0009) BEFORE
	// the handler-level validateBillingCalculateRequest runs. This is byte-identical
	// to the Fiber path — a native Huma 422 must NOT appear.
	req := httptest.NewRequest(http.MethodPost, "/v1/organizations/"+orgID.String()+"/billing/calculate", bytes.NewBufferString(`{"period":"2026-01"}`))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req, -1)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	respBody, _ := io.ReadAll(resp.Body)
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode, "missing ledgerId stays canonical 400, body: %s", string(respBody))
	assert.Equal(t, "application/problem+json", resp.Header.Get("Content-Type"))

	var got map[string]any
	require.NoError(t, json.Unmarshal(respBody, &got), "body: %s", string(respBody))
	assert.Equal(t, constant.ErrMissingFieldsInRequest.Error(), got["code"])
}

func TestHuma_CalculateBilling_MalformedLedger_Canonical400(t *testing.T) {
	orgID := uuid.New()

	handler := &BillingCalculateHandler{Service: &stubBillingCalculateService{}}
	app := buildHumaBillingCalculateApp(t, handler, true)

	// ledgerId present but not a UUID → clears the `required` struct tag, so the
	// handler-level validateBillingCalculateRequest (in the shared calculateBilling
	// core) rejects it with ErrInvalidLedgerID (0203) BEFORE the service call.
	req := httptest.NewRequest(http.MethodPost, "/v1/organizations/"+orgID.String()+"/billing/calculate", bytes.NewBufferString(`{"ledgerId":"not-a-uuid","period":"2026-01"}`))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req, -1)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	respBody, _ := io.ReadAll(resp.Body)
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode, "malformed ledgerId stays canonical 400, body: %s", string(respBody))

	var got map[string]any
	require.NoError(t, json.Unmarshal(respBody, &got), "body: %s", string(respBody))
	assert.Equal(t, constant.ErrInvalidLedgerID.Error(), got["code"])
}

func TestHuma_CalculateBilling_ServiceError_Mapped(t *testing.T) {
	orgID := uuid.New()
	ledgerID := uuid.New()

	stub := &stubBillingCalculateService{err: pkg.ValidateBusinessError(constant.ErrInvalidPathParameter, "", "packageId")}
	handler := &BillingCalculateHandler{Service: stub}

	app := buildHumaBillingCalculateApp(t, handler, true)

	req := httptest.NewRequest(http.MethodPost, "/v1/organizations/"+orgID.String()+"/billing/calculate", bytes.NewBufferString(validBillingCalculateJSON(ledgerID.String())))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req, -1)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	respBody, _ := io.ReadAll(resp.Body)
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode, "body: %s", string(respBody))
	assert.Equal(t, "application/problem+json", resp.Header.Get("Content-Type"))

	var got map[string]any
	require.NoError(t, json.Unmarshal(respBody, &got), "body: %s", string(respBody))
	assert.Equal(t, constant.ErrInvalidPathParameter.Error(), got["code"])
}
