// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	libCommons "github.com/LerianStudio/lib-commons/v6/commons"

	openapi "github.com/LerianStudio/lib-commons/v6/commons/net/http/openapi"
	libProblem "github.com/LerianStudio/lib-commons/v6/commons/net/http/problem"
	"github.com/gofiber/fiber/v3"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	mongodb "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/mongodb/onboarding"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/organization"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/services"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/services/command"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/services/query"
	"github.com/LerianStudio/midaz/v4/pkg"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	pkgHTTP "github.com/LerianStudio/midaz/v4/pkg/net/http"
)

// buildHumaOrganizationApp mounts the six organization Huma operations on a /v1
// group, faithfully mirroring the production wiring in unified-server.go:
// problem.Install() runs before any huma.Register, the Huma API is built with
// openapi.New over a /v1 group, an auth-shim middleware stands in for auth.Authorize
// + tenant PostAuthMiddlewares (so the auth-preserved contract can be probed), and
// http.ParseUUIDPathParameters("organization") attaches the id-parse chain before the
// Huma terminals registered by RegisterOrganizationRoutes.
//
// Organization is a FIRST-LEVEL resource: the only UUID path param is {id}, and the
// list/create collection sits at /organizations directly (no org/ledger prefix).
//
// MUST-NOT-PARALLELIZE (same rationale as the asset exemplar's buildHumaAssetApp):
// libProblem.Install() swaps the process-global huma.NewError hook and Huma
// validation uses process-global sync.Pools — concurrent builds/requests
// cross-contaminate. -race does not catch the logical contamination. These tests are
// sub-second; keep them sequential.
//
// authOK=false makes the shim reject with the ledger's canonical 401 envelope
// (mirroring auth.Authorize failure) so the auth-preserved contract is testable
// without a live lib-auth server.
func buildHumaOrganizationApp(t *testing.T, handler *OrganizationHandler, authOK bool) *fiber.App {
	t.Helper()

	f := fiber.New(fiber.Config{
		ErrorHandler: pkgHTTP.CanonicalFiberErrorHandler,
	})

	// problem.Install must run before any huma.Register (runtime + spec-gen).
	libProblem.Install()
	pkgHTTP.InstallHumaFrameworkErrors()

	apiV1 := f.Group("/v1")

	// Auth shim: stands in for auth.Authorize("midaz","organizations",verb). A
	// rejected request (authOK=false) must never reach Huma — it returns the ledger 401.
	apiV1.Use(func(c fiber.Ctx) error {
		if !authOK {
			return pkgHTTP.Unauthorized(c, "0001", "Unauthorized", "auth required")
		}

		return c.Next()
	})

	hAPI := openapi.New(f, apiV1, openapi.Config{Title: "ledger-test", Version: "test", Servers: []string{"/v1"}})

	// Mirror the production chain: ParseUUIDPathParameters runs as a Fiber middleware
	// (no terminal handler) before the Huma terminal on the {id} routes. Registered
	// group-relative on apiV1 so Fiber prepends /v1 — matching the group-relative paths
	// RegisterOrganizationRoutes registers on the Huma API. The static metrics/count
	// route is registered BEFORE the :id route so it is not shadowed by the param.
	parse := pkgHTTP.ParseUUIDPathParameters("organization")
	passthrough := func(c fiber.Ctx) error { return c.Next() }
	apiV1.Post("/organizations", passthrough)
	apiV1.Get("/organizations", passthrough)
	apiV1.Head("/organizations/metrics/count", passthrough)
	apiV1.Get("/organizations/:id", parse)
	apiV1.Patch("/organizations/:id", parse)
	apiV1.Delete("/organizations/:id", parse)

	RegisterOrganizationRoutes(hAPI, handler, routeOpSuffixV1)

	return f
}

func TestCreateOrganization_Success(t *testing.T) {
	// NOT parallel: buildHumaOrganizationApp mutates process-global huma state.
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	orgRepo := organization.NewMockRepository(ctrl)
	metadataRepo := mongodb.NewMockRepository(ctrl)

	orgRepo.EXPECT().Create(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ any, org *mmodel.Organization) (*mmodel.Organization, error) {
			org.ID = uuid.Must(libCommons.GenerateUUIDv7()).String()
			org.CreatedAt = fixedTestTime
			org.UpdatedAt = fixedTestTime
			return org, nil
		}).Times(1)
	// The shared body pipeline (DecodeAndValidate -> parseMetadata) initializes
	// Metadata to a non-nil empty map when the body carries no "metadata" key, so
	// CreateOnboardingMetadata persists it.
	metadataRepo.EXPECT().Create(gomock.Any(), constant.EntityOrganization, gomock.Any()).Return(nil).Times(1)

	handler := &OrganizationHandler{Command: &command.UseCase{OrganizationRepo: orgRepo, OnboardingMetadataRepo: metadataRepo}}

	app := buildHumaOrganizationApp(t, handler, true)

	body, _ := json.Marshal(map[string]any{
		"legalName":     "Test Organization",
		"legalDocument": "12345678901234",
		"address":       map[string]any{"country": "US"},
		"status":        map[string]any{"code": "ACTIVE"},
	})
	req := httptest.NewRequest(http.MethodPost, "/v1/organizations", bytes.NewReader(body))
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
	assert.Equal(t, "Test Organization", got["legalName"])
	assert.Equal(t, "12345678901234", got["legalDocument"])
	assert.NotEmpty(t, got["id"])
}

func TestCreateOrganization_AuthPreserved(t *testing.T) {
	// NOT parallel: process-global huma state.
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	// No repo expectations: a rejected auth must never reach the service.
	handler := &OrganizationHandler{Command: &command.UseCase{OrganizationRepo: organization.NewMockRepository(ctrl)}}

	app := buildHumaOrganizationApp(t, handler, false)

	body, _ := json.Marshal(map[string]any{"legalName": "Test", "legalDocument": "123"})
	req := httptest.NewRequest(http.MethodPost, "/v1/organizations", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req, fiber.TestConfig{Timeout: 0})
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode, "auth middleware must reject before Huma; no public route")
}

func TestCreateOrganization_ValidationError_Canonical400(t *testing.T) {
	// NOT parallel: process-global huma state.
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	// Missing required "legalName" -> imperative ValidateStruct -> canonical 400,
	// service never reached.
	handler := &OrganizationHandler{Command: &command.UseCase{OrganizationRepo: organization.NewMockRepository(ctrl)}}

	app := buildHumaOrganizationApp(t, handler, true)

	body, _ := json.Marshal(map[string]any{"legalDocument": "12345678901234"})
	req := httptest.NewRequest(http.MethodPost, "/v1/organizations", bytes.NewReader(body))
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

func TestCreateOrganization_MalformedBody_Canonical400(t *testing.T) {
	// NOT parallel: process-global huma state.
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	// Malformed JSON -> DecodeAndValidate returns a pkg.ResponseError (0094), which
	// HumaProblem projects to problem+json at 400 (NOT the 500 fallback and NOT a
	// native Huma 422). Service never reached.
	handler := &OrganizationHandler{Command: &command.UseCase{OrganizationRepo: organization.NewMockRepository(ctrl)}}

	app := buildHumaOrganizationApp(t, handler, true)

	req := httptest.NewRequest(http.MethodPost, "/v1/organizations", bytes.NewReader([]byte("{not valid json")))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req, fiber.TestConfig{Timeout: 0})
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	respBody, _ := io.ReadAll(resp.Body)

	assert.Equal(t, http.StatusBadRequest, resp.StatusCode, "malformed body stays 400 — no 500, no native 422")
	assert.Equal(t, "application/problem+json", resp.Header.Get("Content-Type"))

	var got map[string]any
	require.NoError(t, json.Unmarshal(respBody, &got), "body: %s", string(respBody))
	assert.Equal(t, constant.ErrInvalidRequestBody.Error(), got["code"], "malformed-body code preserved (0094)")
	assert.Equal(t, float64(http.StatusBadRequest), got["status"])
}

func TestCreateOrganization_EmptyBody_Canonical400(t *testing.T) {
	// NOT parallel: process-global huma state.
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	// A zero-length body is rejected by Huma's request-body precondition BEFORE
	// SkipValidateBody is honoured, so DecodeAndValidate never runs and the
	// rejection carries no business code of its own. InstallHumaFrameworkErrors
	// maps it onto the same 0094 envelope the malformed-body case above gets.
	// Service never reached.
	//
	// This test is the canary for the humaEmptyBodyMessage string coupling: it
	// drives a real Fiber+Huma request, so a Huma upgrade that reworded the
	// precondition message fails here rather than silently in production.
	handler := &OrganizationHandler{Command: &command.UseCase{OrganizationRepo: organization.NewMockRepository(ctrl)}}

	app := buildHumaOrganizationApp(t, handler, true)

	req := httptest.NewRequest(http.MethodPost, "/v1/organizations", bytes.NewReader([]byte("")))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req, fiber.TestConfig{Timeout: 0})
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	respBody, _ := io.ReadAll(resp.Body)

	assert.Equal(t, http.StatusBadRequest, resp.StatusCode, "empty body stays 400 — no 500, no native 422")
	assert.Equal(t, "application/problem+json", resp.Header.Get("Content-Type"))

	var got map[string]any
	require.NoError(t, json.Unmarshal(respBody, &got), "body: %s", string(respBody))
	assert.Equal(t, constant.ErrInvalidRequestBody.Error(), got["code"], "empty-body code is 0094, not absent")
	assert.Equal(t, "Unmarshalling error", got["title"])
	assert.Equal(t, float64(http.StatusBadRequest), got["status"])
}

func TestGetOrganizationByID_Success(t *testing.T) {
	// NOT parallel: process-global huma state.
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	orgID := uuid.Must(libCommons.GenerateUUIDv7())

	orgRepo := organization.NewMockRepository(ctrl)
	metadataRepo := mongodb.NewMockRepository(ctrl)

	orgRepo.EXPECT().Find(gomock.Any(), orgID).
		Return(&mmodel.Organization{ID: orgID.String(), LegalName: "USD Corp", LegalDocument: "999"}, nil).Times(1)
	metadataRepo.EXPECT().FindByEntity(gomock.Any(), constant.EntityOrganization, orgID.String()).Return(nil, nil).Times(1)

	handler := &OrganizationHandler{Query: &query.UseCase{OrganizationRepo: orgRepo, OnboardingMetadataRepo: metadataRepo}}

	app := buildHumaOrganizationApp(t, handler, true)

	req := httptest.NewRequest(http.MethodGet, "/v1/organizations/"+orgID.String(), nil)
	resp, err := app.Test(req, fiber.TestConfig{Timeout: 0})
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	respBody, _ := io.ReadAll(resp.Body)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.NotContains(t, string(respBody), "$schema")

	var got map[string]any
	require.NoError(t, json.Unmarshal(respBody, &got), "body: %s", string(respBody))
	assert.Equal(t, "USD Corp", got["legalName"])
	assert.Equal(t, orgID.String(), got["id"])
}

func TestGetOrganizationByID_NotFound_Canonical404(t *testing.T) {
	// NOT parallel: process-global huma state.
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	orgID := uuid.Must(libCommons.GenerateUUIDv7())

	orgRepo := organization.NewMockRepository(ctrl)
	orgRepo.EXPECT().Find(gomock.Any(), orgID).Return(nil, services.ErrDatabaseItemNotFound).Times(1)

	handler := &OrganizationHandler{Query: &query.UseCase{OrganizationRepo: orgRepo}}

	app := buildHumaOrganizationApp(t, handler, true)

	req := httptest.NewRequest(http.MethodGet, "/v1/organizations/"+orgID.String(), nil)
	resp, err := app.Test(req, fiber.TestConfig{Timeout: 0})
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	respBody, _ := io.ReadAll(resp.Body)
	assert.Equal(t, http.StatusNotFound, resp.StatusCode, "not-found stays canonical 404 — no native Huma 422")
	assert.Equal(t, "application/problem+json", resp.Header.Get("Content-Type"))

	var got map[string]any
	require.NoError(t, json.Unmarshal(respBody, &got), "body: %s", string(respBody))
	assert.Equal(t, constant.ErrOrganizationIDNotFound.Error(), got["code"])
	assert.Equal(t, float64(http.StatusNotFound), got["status"])
}

func TestGetOrganizationByID_BadUUID_Canonical400(t *testing.T) {
	// NOT parallel: process-global huma state.
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	// Service must never be reached: ParseUUIDPathParameters rejects the bad id with
	// the canonical 0065 / 400 before Huma.
	handler := &OrganizationHandler{Query: &query.UseCase{OrganizationRepo: organization.NewMockRepository(ctrl)}}

	app := buildHumaOrganizationApp(t, handler, true)

	req := httptest.NewRequest(http.MethodGet, "/v1/organizations/not-a-uuid", nil)
	resp, err := app.Test(req, fiber.TestConfig{Timeout: 0})
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	respBody, _ := io.ReadAll(resp.Body)
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode, "bad path UUID stays canonical 400 — no native Huma 422")

	var got map[string]any
	require.NoError(t, json.Unmarshal(respBody, &got), "body: %s", string(respBody))
	assert.Equal(t, constant.ErrInvalidPathParameter.Error(), got["code"])
}

func TestGetAllOrganizations_Success(t *testing.T) {
	// NOT parallel: process-global huma state.
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	orgRepo := organization.NewMockRepository(ctrl)
	orgRepo.EXPECT().FindAll(gomock.Any(), gomock.Any()).Return([]*mmodel.Organization{}, nil).Times(1)

	handler := &OrganizationHandler{Query: &query.UseCase{OrganizationRepo: orgRepo}}

	app := buildHumaOrganizationApp(t, handler, true)

	req := httptest.NewRequest(http.MethodGet, "/v1/organizations?limit=10&page=1", nil)
	resp, err := app.Test(req, fiber.TestConfig{Timeout: 0})
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	respBody, _ := io.ReadAll(resp.Body)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.NotContains(t, string(respBody), "$schema")

	var got map[string]any
	require.NoError(t, json.Unmarshal(respBody, &got), "body: %s", string(respBody))
	assert.Contains(t, got, "items")
	assert.EqualValues(t, 10, got["limit"])
}

func TestGetAllOrganizations_BadQuery_Canonical400(t *testing.T) {
	// NOT parallel: process-global huma state.
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	// Service must never be reached: ValidateParameters rejects limit=abc with the
	// canonical 400 (ErrInvalidQueryParameter), NOT a native Huma 422.
	handler := &OrganizationHandler{Query: &query.UseCase{OrganizationRepo: organization.NewMockRepository(ctrl)}}

	app := buildHumaOrganizationApp(t, handler, true)

	req := httptest.NewRequest(http.MethodGet, "/v1/organizations?limit=abc", nil)
	resp, err := app.Test(req, fiber.TestConfig{Timeout: 0})
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	respBody, _ := io.ReadAll(resp.Body)
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode, "bad query stays canonical 400 — no native Huma 422")

	var got map[string]any
	require.NoError(t, json.Unmarshal(respBody, &got), "body: %s", string(respBody))
	assert.Equal(t, constant.ErrInvalidQueryParameter.Error(), got["code"])
}

func TestGetAllOrganizations_InvalidStatus_Canonical400(t *testing.T) {
	// NOT parallel: process-global huma state.
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	// The organization-specific status guard rejects an out-of-allowlist status with
	// the canonical 400 before any repo call — service never reached.
	handler := &OrganizationHandler{Query: &query.UseCase{OrganizationRepo: organization.NewMockRepository(ctrl)}}

	app := buildHumaOrganizationApp(t, handler, true)

	req := httptest.NewRequest(http.MethodGet, "/v1/organizations?status=BOGUS", nil)
	resp, err := app.Test(req, fiber.TestConfig{Timeout: 0})
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	respBody, _ := io.ReadAll(resp.Body)
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode, "invalid status stays canonical 400")

	var got map[string]any
	require.NoError(t, json.Unmarshal(respBody, &got), "body: %s", string(respBody))
	assert.Equal(t, constant.ErrInvalidQueryParameter.Error(), got["code"])
}

func TestDeleteOrganization_204Empty(t *testing.T) {
	// NOT parallel: process-global huma state.
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	// Ensure the production-environment guard is not triggered.
	t.Setenv("ENV_NAME", "development")

	orgID := uuid.Must(libCommons.GenerateUUIDv7())

	orgRepo := organization.NewMockRepository(ctrl)
	orgRepo.EXPECT().Delete(gomock.Any(), orgID).Return(nil).Times(1)

	handler := &OrganizationHandler{Command: &command.UseCase{OrganizationRepo: orgRepo}}

	app := buildHumaOrganizationApp(t, handler, true)

	req := httptest.NewRequest(http.MethodDelete, "/v1/organizations/"+orgID.String(), nil)
	resp, err := app.Test(req, fiber.TestConfig{Timeout: 0})
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	respBody, _ := io.ReadAll(resp.Body)
	assert.Equal(t, http.StatusNoContent, resp.StatusCode)
	assert.Empty(t, respBody, "DELETE 204 must have an empty body")
}

func TestDeleteOrganization_ProductionForbidden(t *testing.T) {
	// NOT parallel: process-global huma state.
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	// The production guard rejects DELETE with the canonical ErrActionNotPermitted
	// before any repo call — service never reached.
	t.Setenv("ENV_NAME", "production")

	orgID := uuid.Must(libCommons.GenerateUUIDv7())

	// No repo expectations: the guard fires before Command.DeleteOrganizationByID.
	handler := &OrganizationHandler{Command: &command.UseCase{OrganizationRepo: organization.NewMockRepository(ctrl)}}

	app := buildHumaOrganizationApp(t, handler, true)

	req := httptest.NewRequest(http.MethodDelete, "/v1/organizations/"+orgID.String(), nil)
	resp, err := app.Test(req, fiber.TestConfig{Timeout: 0})
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	respBody, _ := io.ReadAll(resp.Body)

	var got map[string]any
	require.NoError(t, json.Unmarshal(respBody, &got), "body: %s", string(respBody))
	assert.Equal(t, constant.ErrActionNotPermitted.Error(), got["code"], "production DELETE guard rejects before any repo call")
}

func TestCountOrganizations_204WithHeader(t *testing.T) {
	// NOT parallel: process-global huma state.
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	orgRepo := organization.NewMockRepository(ctrl)
	orgRepo.EXPECT().Count(gomock.Any()).Return(int64(42), nil).Times(1)

	handler := &OrganizationHandler{Query: &query.UseCase{OrganizationRepo: orgRepo}}

	app := buildHumaOrganizationApp(t, handler, true)

	req := httptest.NewRequest(http.MethodHead, "/v1/organizations/metrics/count", nil)
	resp, err := app.Test(req, fiber.TestConfig{Timeout: 0})
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	respBody, _ := io.ReadAll(resp.Body)
	assert.Equal(t, http.StatusNoContent, resp.StatusCode)
	assert.Equal(t, "42", resp.Header.Get(constant.XTotalCount), "X-Total-Count header must carry the count")
	assert.Empty(t, respBody, "HEAD count must have an empty body")
	assert.Equal(t, "0", resp.Header.Get("Content-Length"), "HEAD 204 must set Content-Length: 0 (parity with the Fiber NoContent path)")
}

func TestCreateOrganization_RepositoryError_500(t *testing.T) {
	// NOT parallel: process-global huma state.
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	orgRepo := organization.NewMockRepository(ctrl)
	orgRepo.EXPECT().Create(gomock.Any(), gomock.Any()).
		Return(nil, pkg.InternalServerError{Code: "0046", Title: "Internal Server Error", Message: "Database connection failed"}).Times(1)

	handler := &OrganizationHandler{Command: &command.UseCase{OrganizationRepo: orgRepo}}

	app := buildHumaOrganizationApp(t, handler, true)

	body, _ := json.Marshal(map[string]any{
		"legalName":     "Test Organization",
		"legalDocument": "12345678901234",
		"address":       map[string]any{"country": "US"},
		"status":        map[string]any{"code": "ACTIVE"},
	})
	req := httptest.NewRequest(http.MethodPost, "/v1/organizations", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req, fiber.TestConfig{Timeout: 0})
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	respBody, _ := io.ReadAll(resp.Body)
	assert.Equal(t, http.StatusInternalServerError, resp.StatusCode)
	assert.Equal(t, "application/problem+json", resp.Header.Get("Content-Type"))

	var got map[string]any
	require.NoError(t, json.Unmarshal(respBody, &got), "body: %s", string(respBody))
	assert.Equal(t, "0046", got["code"])
	assert.Contains(t, got, "detail")
}

func TestUpdateOrganization_Success(t *testing.T) {
	// NOT parallel: process-global huma state.
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	orgID := uuid.Must(libCommons.GenerateUUIDv7())

	orgRepo := organization.NewMockRepository(ctrl)
	metadataRepo := mongodb.NewMockRepository(ctrl)

	orgRepo.EXPECT().Update(gomock.Any(), orgID, gomock.Any()).
		Return(&mmodel.Organization{
			ID:        orgID.String(),
			LegalName: "Updated Organization Name",
			Status:    mmodel.Status{Code: "ACTIVE"},
		}, nil).Times(1)
	// The metadata upsert and the re-read both reach the metadata repo.
	metadataRepo.EXPECT().Update(gomock.Any(), constant.EntityOrganization, orgID.String(), gomock.Any()).Return(nil).AnyTimes()
	metadataRepo.EXPECT().FindByEntity(gomock.Any(), constant.EntityOrganization, orgID.String()).Return(nil, nil).AnyTimes()

	handler := &OrganizationHandler{
		Command: &command.UseCase{OrganizationRepo: orgRepo, OnboardingMetadataRepo: metadataRepo},
		Query:   &query.UseCase{OrganizationRepo: orgRepo, OnboardingMetadataRepo: metadataRepo},
	}

	app := buildHumaOrganizationApp(t, handler, true)

	body, _ := json.Marshal(map[string]any{"legalName": "Updated Organization Name"})
	req := httptest.NewRequest(http.MethodPatch, "/v1/organizations/"+orgID.String(), bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req, fiber.TestConfig{Timeout: 0})
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	respBody, _ := io.ReadAll(resp.Body)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.NotContains(t, string(respBody), "$schema")

	var got map[string]any
	require.NoError(t, json.Unmarshal(respBody, &got), "body: %s", string(respBody))
	assert.Equal(t, orgID.String(), got["id"])
	assert.Equal(t, "Updated Organization Name", got["legalName"])
}

func TestUpdateOrganization_NotFound_Canonical404(t *testing.T) {
	// NOT parallel: process-global huma state.
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	orgID := uuid.Must(libCommons.GenerateUUIDv7())

	orgRepo := organization.NewMockRepository(ctrl)
	orgRepo.EXPECT().Update(gomock.Any(), orgID, gomock.Any()).Return(nil, services.ErrDatabaseItemNotFound).Times(1)

	handler := &OrganizationHandler{Command: &command.UseCase{OrganizationRepo: orgRepo}}

	app := buildHumaOrganizationApp(t, handler, true)

	body, _ := json.Marshal(map[string]any{"legalName": "Updated Name"})
	req := httptest.NewRequest(http.MethodPatch, "/v1/organizations/"+orgID.String(), bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req, fiber.TestConfig{Timeout: 0})
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	respBody, _ := io.ReadAll(resp.Body)
	assert.Equal(t, http.StatusNotFound, resp.StatusCode, "not-found stays canonical 404 — no native Huma 422")
	assert.Equal(t, "application/problem+json", resp.Header.Get("Content-Type"))

	var got map[string]any
	require.NoError(t, json.Unmarshal(respBody, &got), "body: %s", string(respBody))
	assert.Equal(t, constant.ErrOrganizationIDNotFound.Error(), got["code"])
	assert.Equal(t, float64(http.StatusNotFound), got["status"])
}

func TestUpdateOrganization_MalformedBody_Canonical400(t *testing.T) {
	// NOT parallel: process-global huma state.
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	// Malformed JSON -> DecodeAndValidate returns 0094 at 400; service never reached.
	handler := &OrganizationHandler{Command: &command.UseCase{OrganizationRepo: organization.NewMockRepository(ctrl)}}

	app := buildHumaOrganizationApp(t, handler, true)

	req := httptest.NewRequest(http.MethodPatch, "/v1/organizations/"+uuid.Must(libCommons.GenerateUUIDv7()).String(), bytes.NewReader([]byte("{not valid json")))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req, fiber.TestConfig{Timeout: 0})
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	respBody, _ := io.ReadAll(resp.Body)
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode, "malformed body stays 400 — no 500, no native 422")

	var got map[string]any
	require.NoError(t, json.Unmarshal(respBody, &got), "body: %s", string(respBody))
	assert.Equal(t, constant.ErrInvalidRequestBody.Error(), got["code"])
}

func TestGetAllOrganizations_MetadataFilter_Success(t *testing.T) {
	// NOT parallel: process-global huma state.
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	org1ID := uuid.Must(libCommons.GenerateUUIDv7()).String()
	org2ID := uuid.Must(libCommons.GenerateUUIDv7()).String()

	orgRepo := organization.NewMockRepository(ctrl)
	metadataRepo := mongodb.NewMockRepository(ctrl)

	// A metadata query routes the list through FindList + FindAll (the by-EntityIDs
	// re-read), NOT the plain FindAll branch.
	metadataRepo.EXPECT().FindList(gomock.Any(), constant.EntityOrganization, gomock.Any()).
		Return([]*mongodb.Metadata{
			{EntityID: org1ID, Data: map[string]any{"tier": "premium"}},
			{EntityID: org2ID, Data: map[string]any{"tier": "premium"}},
		}, nil).Times(1)
	orgRepo.EXPECT().FindAll(gomock.Any(), gomock.Any()).
		Return([]*mmodel.Organization{
			{ID: org1ID, LegalName: "Premium Org One", Status: mmodel.Status{Code: "ACTIVE"}},
			{ID: org2ID, LegalName: "Premium Org Two", Status: mmodel.Status{Code: "ACTIVE"}},
		}, nil).Times(1)

	handler := &OrganizationHandler{Query: &query.UseCase{OrganizationRepo: orgRepo, OnboardingMetadataRepo: metadataRepo}}

	app := buildHumaOrganizationApp(t, handler, true)

	req := httptest.NewRequest(http.MethodGet, "/v1/organizations?metadata.tier=premium", nil)
	resp, err := app.Test(req, fiber.TestConfig{Timeout: 0})
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	respBody, _ := io.ReadAll(resp.Body)
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var got map[string]any
	require.NoError(t, json.Unmarshal(respBody, &got), "body: %s", string(respBody))

	items, ok := got["items"].([]any)
	require.True(t, ok, "items should be an array")
	assert.Len(t, items, 2)
}

func TestGetAllOrganizations_MetadataFilter_NotFound_Canonical404(t *testing.T) {
	// NOT parallel: process-global huma state.
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	metadataRepo := mongodb.NewMockRepository(ctrl)
	metadataRepo.EXPECT().FindList(gomock.Any(), constant.EntityOrganization, gomock.Any()).Return(nil, nil).Times(1)

	handler := &OrganizationHandler{Query: &query.UseCase{
		OrganizationRepo:       organization.NewMockRepository(ctrl),
		OnboardingMetadataRepo: metadataRepo,
	}}

	app := buildHumaOrganizationApp(t, handler, true)

	req := httptest.NewRequest(http.MethodGet, "/v1/organizations?metadata.tier=nonexistent", nil)
	resp, err := app.Test(req, fiber.TestConfig{Timeout: 0})
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	respBody, _ := io.ReadAll(resp.Body)
	assert.Equal(t, http.StatusNotFound, resp.StatusCode)

	var got map[string]any
	require.NoError(t, json.Unmarshal(respBody, &got), "body: %s", string(respBody))
	assert.Equal(t, constant.ErrNoOrganizationsFound.Error(), got["code"])
}

func TestGetAllOrganizations_MetadataWithNameFilter_Canonical400(t *testing.T) {
	// NOT parallel: process-global huma state.
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	// metadata and the name filters are mutually exclusive: the guard rejects with
	// the canonical 400 before any repo call.
	handler := &OrganizationHandler{Query: &query.UseCase{
		OrganizationRepo:       organization.NewMockRepository(ctrl),
		OnboardingMetadataRepo: mongodb.NewMockRepository(ctrl),
	}}

	app := buildHumaOrganizationApp(t, handler, true)

	req := httptest.NewRequest(http.MethodGet, "/v1/organizations?metadata.tier=premium&legal_name=Acme", nil)
	resp, err := app.Test(req, fiber.TestConfig{Timeout: 0})
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	respBody, _ := io.ReadAll(resp.Body)
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)

	var got map[string]any
	require.NoError(t, json.Unmarshal(respBody, &got), "body: %s", string(respBody))
	assert.Equal(t, constant.ErrInvalidQueryParameter.Error(), got["code"])
}

func TestGetAllOrganizations_RepositoryError_500(t *testing.T) {
	// NOT parallel: process-global huma state.
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	orgRepo := organization.NewMockRepository(ctrl)
	orgRepo.EXPECT().FindAll(gomock.Any(), gomock.Any()).
		Return(nil, pkg.InternalServerError{Code: "0046", Title: "Internal Server Error", Message: "Database connection failed"}).Times(1)

	handler := &OrganizationHandler{Query: &query.UseCase{OrganizationRepo: orgRepo}}

	app := buildHumaOrganizationApp(t, handler, true)

	req := httptest.NewRequest(http.MethodGet, "/v1/organizations", nil)
	resp, err := app.Test(req, fiber.TestConfig{Timeout: 0})
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	respBody, _ := io.ReadAll(resp.Body)
	assert.Equal(t, http.StatusInternalServerError, resp.StatusCode)

	var got map[string]any
	require.NoError(t, json.Unmarshal(respBody, &got), "body: %s", string(respBody))
	assert.Equal(t, "0046", got["code"])
	assert.Contains(t, got, "detail")
}

func TestDeleteOrganization_RepositoryError_500(t *testing.T) {
	// NOT parallel: process-global huma state.
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	t.Setenv("ENV_NAME", "development")

	orgID := uuid.Must(libCommons.GenerateUUIDv7())

	orgRepo := organization.NewMockRepository(ctrl)
	orgRepo.EXPECT().Delete(gomock.Any(), orgID).
		Return(pkg.InternalServerError{Code: "0046", Title: "Internal Server Error", Message: "Database connection failed"}).Times(1)

	handler := &OrganizationHandler{Command: &command.UseCase{OrganizationRepo: orgRepo}}

	app := buildHumaOrganizationApp(t, handler, true)

	req := httptest.NewRequest(http.MethodDelete, "/v1/organizations/"+orgID.String(), nil)
	resp, err := app.Test(req, fiber.TestConfig{Timeout: 0})
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	respBody, _ := io.ReadAll(resp.Body)
	assert.Equal(t, http.StatusInternalServerError, resp.StatusCode)

	var got map[string]any
	require.NoError(t, json.Unmarshal(respBody, &got), "body: %s", string(respBody))
	assert.Equal(t, "0046", got["code"])
	assert.Contains(t, got, "detail")
}

func TestCountOrganizations_RepositoryError_500(t *testing.T) {
	// NOT parallel: process-global huma state.
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	orgRepo := organization.NewMockRepository(ctrl)
	orgRepo.EXPECT().Count(gomock.Any()).
		Return(int64(0), pkg.InternalServerError{Code: "0046", Title: "Internal Server Error", Message: "Database connection failed"}).Times(1)

	handler := &OrganizationHandler{Query: &query.UseCase{OrganizationRepo: orgRepo}}

	app := buildHumaOrganizationApp(t, handler, true)

	req := httptest.NewRequest(http.MethodHead, "/v1/organizations/metrics/count", nil)
	resp, err := app.Test(req, fiber.TestConfig{Timeout: 0})
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	assert.Equal(t, http.StatusInternalServerError, resp.StatusCode)
	assert.Equal(t, "application/problem+json", resp.Header.Get("Content-Type"))
}

// TestHuma_Property_Organization_FieldLengths asserts the create path never answers
// 5xx for any legalName/legalDocument length: over-long values must be rejected by
// the imperative validator as a canonical 4xx, never crash the transport.
func TestProperty_Organization_FieldLengths(t *testing.T) {
	// NOT parallel: process-global huma state.
	randString := func(n int) string {
		if n == 0 {
			return ""
		}

		letters := []rune("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789 _-")
		b := make([]rune, n)

		for i := range b {
			b[i] = letters[i%len(letters)]
		}

		return string(b)
	}

	testCases := []struct {
		nameLen int
		docLen  int
	}{
		{0, 0},
		{1, 1},
		{10, 10},
		{50, 20},
		{100, 50},
		{200, 100},
		{256, 128},
		{400, 200},
	}

	for _, tc := range testCases {
		t.Run(fmt.Sprintf("nameLen=%d_docLen=%d", tc.nameLen, tc.docLen), func(t *testing.T) {
			ctrl := gomock.NewController(t)
			t.Cleanup(ctrl.Finish)

			orgRepo := organization.NewMockRepository(ctrl)
			metadataRepo := mongodb.NewMockRepository(ctrl)

			orgRepo.EXPECT().Create(gomock.Any(), gomock.Any()).
				DoAndReturn(func(_ any, org *mmodel.Organization) (*mmodel.Organization, error) {
					org.ID = uuid.Must(libCommons.GenerateUUIDv7()).String()
					return org, nil
				}).AnyTimes()
			metadataRepo.EXPECT().Create(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).AnyTimes()

			handler := &OrganizationHandler{Command: &command.UseCase{OrganizationRepo: orgRepo, OnboardingMetadataRepo: metadataRepo}}

			app := buildHumaOrganizationApp(t, handler, true)

			body, _ := json.Marshal(map[string]any{
				"legalName":     randString(tc.nameLen),
				"legalDocument": randString(tc.docLen),
				"address":       map[string]any{"country": "US"},
				"status":        map[string]any{"code": "ACTIVE"},
			})
			req := httptest.NewRequest(http.MethodPost, "/v1/organizations", bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")

			resp, err := app.Test(req, fiber.TestConfig{Timeout: 0})
			require.NoError(t, err)
			defer func() { _ = resp.Body.Close() }()

			respBody, _ := io.ReadAll(resp.Body)
			assert.Less(t, resp.StatusCode, http.StatusInternalServerError,
				"nameLen=%d docLen=%d must not be a 5xx: body=%s", tc.nameLen, tc.docLen, string(respBody))
		})
	}
}

// TestHuma_Property_Headers_InvalidFormats asserts the list path never answers 5xx
// for odd X-Request-Id header shapes.
func TestProperty_Headers_InvalidFormats(t *testing.T) {
	// NOT parallel: process-global huma state.
	tests := []struct {
		name    string
		headers map[string]string
	}{
		{"empty X-Request-Id", map[string]string{"X-Request-Id": ""}},
		{"very long X-Request-Id", map[string]string{"X-Request-Id": strings.Repeat("a", 1024)}},
		{"special chars in header", map[string]string{"X-Request-Id": "test-123_abc.def"}},
		{"UUID format", map[string]string{"X-Request-Id": "550e8400-e29b-41d4-a716-446655440000"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			t.Cleanup(ctrl.Finish)

			orgRepo := organization.NewMockRepository(ctrl)
			orgRepo.EXPECT().FindAll(gomock.Any(), gomock.Any()).Return([]*mmodel.Organization{}, nil).AnyTimes()

			handler := &OrganizationHandler{Query: &query.UseCase{OrganizationRepo: orgRepo}}

			app := buildHumaOrganizationApp(t, handler, true)

			req := httptest.NewRequest(http.MethodGet, "/v1/organizations", nil)
			for k, v := range tt.headers {
				req.Header.Set(k, v)
			}

			resp, err := app.Test(req, fiber.TestConfig{Timeout: 0})
			require.NoError(t, err)
			defer func() { _ = resp.Body.Close() }()

			assert.Less(t, resp.StatusCode, http.StatusInternalServerError, "headers %v must not be a 5xx", tt.headers)
		})
	}
}

// TestHuma_Property_ContentType_Variations asserts the create path never answers 5xx
// for any Content-Type spelling, including a wrong or missing one.
func TestProperty_ContentType_Variations(t *testing.T) {
	// NOT parallel: process-global huma state.
	contentTypes := []string{
		"application/json",
		"application/json; charset=utf-8",
		"APPLICATION/JSON",
		"application/json; charset=UTF-8",
		"text/plain",
		"",
	}

	for _, ct := range contentTypes {
		t.Run("content-type="+ct, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			t.Cleanup(ctrl.Finish)

			orgRepo := organization.NewMockRepository(ctrl)
			metadataRepo := mongodb.NewMockRepository(ctrl)

			orgRepo.EXPECT().Create(gomock.Any(), gomock.Any()).
				DoAndReturn(func(_ any, org *mmodel.Organization) (*mmodel.Organization, error) {
					org.ID = uuid.Must(libCommons.GenerateUUIDv7()).String()
					return org, nil
				}).AnyTimes()
			metadataRepo.EXPECT().Create(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).AnyTimes()

			handler := &OrganizationHandler{Command: &command.UseCase{OrganizationRepo: orgRepo, OnboardingMetadataRepo: metadataRepo}}

			app := buildHumaOrganizationApp(t, handler, true)

			body, _ := json.Marshal(map[string]any{
				"legalName":     "Test Org",
				"legalDocument": "12345678901234",
				"address":       map[string]any{"country": "US"},
				"status":        map[string]any{"code": "ACTIVE"},
			})
			req := httptest.NewRequest(http.MethodPost, "/v1/organizations", bytes.NewReader(body))

			if ct != "" {
				req.Header.Set("Content-Type", ct)
			}

			resp, err := app.Test(req, fiber.TestConfig{Timeout: 0})
			require.NoError(t, err)
			defer func() { _ = resp.Body.Close() }()

			assert.Less(t, resp.StatusCode, http.StatusInternalServerError, "Content-Type=%q must not be a 5xx", ct)
		})
	}
}

// TestHuma_Property_Headers_Duplicated asserts duplicated request headers never
// produce a 5xx on the create path.
func TestProperty_Headers_Duplicated(t *testing.T) {
	// NOT parallel: process-global huma state.
	tests := []struct {
		name       string
		addHeaders func(req *http.Request)
	}{
		{
			name: "duplicate Content-Type",
			addHeaders: func(req *http.Request) {
				req.Header.Add("Content-Type", "application/json")
				req.Header.Add("Content-Type", "application/json")
			},
		},
		{
			name: "duplicate X-Request-Id",
			addHeaders: func(req *http.Request) {
				req.Header.Set("Content-Type", "application/json")
				req.Header.Add("X-Request-Id", "req-123")
				req.Header.Add("X-Request-Id", "req-456")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			t.Cleanup(ctrl.Finish)

			orgRepo := organization.NewMockRepository(ctrl)
			metadataRepo := mongodb.NewMockRepository(ctrl)

			orgRepo.EXPECT().Create(gomock.Any(), gomock.Any()).
				DoAndReturn(func(_ any, org *mmodel.Organization) (*mmodel.Organization, error) {
					org.ID = uuid.Must(libCommons.GenerateUUIDv7()).String()
					return org, nil
				}).AnyTimes()
			metadataRepo.EXPECT().Create(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).AnyTimes()

			handler := &OrganizationHandler{Command: &command.UseCase{OrganizationRepo: orgRepo, OnboardingMetadataRepo: metadataRepo}}

			app := buildHumaOrganizationApp(t, handler, true)

			body := []byte(`{"legalName":"Test Org","legalDocument":"12345678901234","address":{"country":"US"}}`)
			req := httptest.NewRequest(http.MethodPost, "/v1/organizations", bytes.NewReader(body))
			tt.addHeaders(req)

			resp, err := app.Test(req, fiber.TestConfig{Timeout: 0})
			require.NoError(t, err)
			defer func() { _ = resp.Body.Close() }()

			assert.Less(t, resp.StatusCode, http.StatusInternalServerError, "%s must not be a 5xx", tt.name)
		})
	}
}

// FuzzHumaCreateOrganization_LegalName asserts no legalName input drives the create
// path into a 5xx, and that an accepted create always carries an id.
// Run with: go test -fuzz=FuzzHumaCreateOrganization_LegalName ./components/ledger/internal/adapters/http/in/
func FuzzHumaCreateOrganization_LegalName(f *testing.F) {
	f.Add("Acme, Inc.")
	f.Add("")
	f.Add("a")
	f.Add("Αθήνα")
	f.Add("日本語テスト")
	f.Add("Test\x00Name")
	f.Add("Test\nName")
	f.Add("<script>alert(1)</script>")

	f.Fuzz(func(t *testing.T, name string) {
		if len(name) > 512 {
			name = name[:512]
		}

		ctrl := gomock.NewController(t)
		t.Cleanup(ctrl.Finish)

		orgRepo := organization.NewMockRepository(ctrl)
		metadataRepo := mongodb.NewMockRepository(ctrl)

		orgRepo.EXPECT().Create(gomock.Any(), gomock.Any()).
			DoAndReturn(func(_ any, org *mmodel.Organization) (*mmodel.Organization, error) {
				org.ID = uuid.Must(libCommons.GenerateUUIDv7()).String()
				return org, nil
			}).AnyTimes()
		metadataRepo.EXPECT().Create(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).AnyTimes()

		handler := &OrganizationHandler{Command: &command.UseCase{OrganizationRepo: orgRepo, OnboardingMetadataRepo: metadataRepo}}

		app := buildHumaOrganizationApp(t, handler, true)

		body, _ := json.Marshal(map[string]any{
			"legalName":     name,
			"legalDocument": "12345678901234",
			"address":       map[string]any{"country": "US"},
			"status":        map[string]any{"code": "ACTIVE"},
		})
		req := httptest.NewRequest(http.MethodPost, "/v1/organizations", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")

		resp, err := app.Test(req, fiber.TestConfig{Timeout: 0})
		require.NoError(t, err)
		defer func() { _ = resp.Body.Close() }()

		respBody, _ := io.ReadAll(resp.Body)
		require.Less(t, resp.StatusCode, http.StatusInternalServerError,
			"legalName=%q must not be a 5xx: body=%s", name, string(respBody))

		if resp.StatusCode == http.StatusCreated {
			var got map[string]any
			require.NoError(t, json.Unmarshal(respBody, &got))
			assert.NotEmpty(t, got["id"], "accepted create must carry an id for legalName=%q", name)
		}
	})
}
