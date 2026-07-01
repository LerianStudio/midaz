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
	"time"

	openapi "github.com/LerianStudio/lib-commons/v5/commons/net/http/openapi"
	libProblem "github.com/LerianStudio/lib-commons/v5/commons/net/http/problem"
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	mongodb "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/mongodb/onboarding"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/organization"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/services"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/services/command"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/services/query"
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
		DisableStartupMessage: true,
		ErrorHandler:          pkgHTTP.CanonicalFiberErrorHandler,
	})

	// problem.Install must run before any huma.Register (runtime + spec-gen).
	libProblem.Install()

	apiV1 := f.Group("/v1")

	// Auth shim: stands in for auth.Authorize("midaz","organizations",verb). A
	// rejected request (authOK=false) must never reach Huma — it returns the ledger 401.
	apiV1.Use(func(c *fiber.Ctx) error {
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
	apiV1.Post("/organizations", parse)
	apiV1.Get("/organizations", parse)
	apiV1.Head("/organizations/metrics/count", parse)
	apiV1.Get("/organizations/:id", parse)
	apiV1.Patch("/organizations/:id", parse)
	apiV1.Delete("/organizations/:id", parse)

	RegisterOrganizationRoutes(hAPI, handler)

	return f
}

func TestHuma_CreateOrganization_Success(t *testing.T) {
	// NOT parallel: buildHumaOrganizationApp mutates process-global huma state.
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	orgRepo := organization.NewMockRepository(ctrl)
	metadataRepo := mongodb.NewMockRepository(ctrl)

	orgRepo.EXPECT().Create(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ any, org *mmodel.Organization) (*mmodel.Organization, error) {
			org.ID = uuid.New().String()
			org.CreatedAt = time.Now()
			org.UpdatedAt = time.Now()
			return org, nil
		}).Times(1)
	// The shared body pipeline (DecodeAndValidate -> parseMetadata) initializes
	// Metadata to a non-nil empty map when the body carries no "metadata" key, so
	// CreateOnboardingMetadata persists it — faithful to the production Fiber WithBody
	// path (the legacy unit test bypassed WithBody, so it never saw this).
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

	resp, err := app.Test(req, -1)
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

func TestHuma_CreateOrganization_AuthPreserved(t *testing.T) {
	// NOT parallel: process-global huma state.
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	// No repo expectations: a rejected auth must never reach the service.
	handler := &OrganizationHandler{Command: &command.UseCase{OrganizationRepo: organization.NewMockRepository(ctrl)}}

	app := buildHumaOrganizationApp(t, handler, false)

	body, _ := json.Marshal(map[string]any{"legalName": "Test", "legalDocument": "123"})
	req := httptest.NewRequest(http.MethodPost, "/v1/organizations", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req, -1)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode, "auth middleware must reject before Huma; no public route")
}

func TestHuma_CreateOrganization_ValidationError_Canonical400(t *testing.T) {
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

func TestHuma_CreateOrganization_MalformedBody_Canonical400(t *testing.T) {
	// NOT parallel: process-global huma state.
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	// Malformed JSON -> DecodeAndValidate returns a pkg.ResponseError (0094). The
	// Fiber path writes it flat at 400; HumaProblem must project it to problem+json at
	// 400 (NOT the 500 fallback and NOT a native Huma 422). Service never reached.
	handler := &OrganizationHandler{Command: &command.UseCase{OrganizationRepo: organization.NewMockRepository(ctrl)}}

	app := buildHumaOrganizationApp(t, handler, true)

	req := httptest.NewRequest(http.MethodPost, "/v1/organizations", bytes.NewReader([]byte("{not valid json")))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req, -1)
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

func TestHuma_GetOrganizationByID_Success(t *testing.T) {
	// NOT parallel: process-global huma state.
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	orgID := uuid.New()

	orgRepo := organization.NewMockRepository(ctrl)
	metadataRepo := mongodb.NewMockRepository(ctrl)

	orgRepo.EXPECT().Find(gomock.Any(), orgID).
		Return(&mmodel.Organization{ID: orgID.String(), LegalName: "USD Corp", LegalDocument: "999"}, nil).Times(1)
	metadataRepo.EXPECT().FindByEntity(gomock.Any(), constant.EntityOrganization, orgID.String()).Return(nil, nil).Times(1)

	handler := &OrganizationHandler{Query: &query.UseCase{OrganizationRepo: orgRepo, OnboardingMetadataRepo: metadataRepo}}

	app := buildHumaOrganizationApp(t, handler, true)

	req := httptest.NewRequest(http.MethodGet, "/v1/organizations/"+orgID.String(), nil)
	resp, err := app.Test(req, -1)
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

func TestHuma_GetOrganizationByID_NotFound_Canonical404(t *testing.T) {
	// NOT parallel: process-global huma state.
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	orgID := uuid.New()

	orgRepo := organization.NewMockRepository(ctrl)
	orgRepo.EXPECT().Find(gomock.Any(), orgID).Return(nil, services.ErrDatabaseItemNotFound).Times(1)

	handler := &OrganizationHandler{Query: &query.UseCase{OrganizationRepo: orgRepo}}

	app := buildHumaOrganizationApp(t, handler, true)

	req := httptest.NewRequest(http.MethodGet, "/v1/organizations/"+orgID.String(), nil)
	resp, err := app.Test(req, -1)
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

func TestHuma_GetOrganizationByID_BadUUID_Canonical400(t *testing.T) {
	// NOT parallel: process-global huma state.
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	// Service must never be reached: ParseUUIDPathParameters rejects the bad id with
	// the canonical 0065 / 400 before Huma.
	handler := &OrganizationHandler{Query: &query.UseCase{OrganizationRepo: organization.NewMockRepository(ctrl)}}

	app := buildHumaOrganizationApp(t, handler, true)

	req := httptest.NewRequest(http.MethodGet, "/v1/organizations/not-a-uuid", nil)
	resp, err := app.Test(req, -1)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	respBody, _ := io.ReadAll(resp.Body)
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode, "bad path UUID stays canonical 400 — no native Huma 422")

	var got map[string]any
	require.NoError(t, json.Unmarshal(respBody, &got), "body: %s", string(respBody))
	assert.Equal(t, constant.ErrInvalidPathParameter.Error(), got["code"])
}

func TestHuma_GetAllOrganizations_Success(t *testing.T) {
	// NOT parallel: process-global huma state.
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	orgRepo := organization.NewMockRepository(ctrl)
	orgRepo.EXPECT().FindAll(gomock.Any(), gomock.Any()).Return([]*mmodel.Organization{}, nil).Times(1)

	handler := &OrganizationHandler{Query: &query.UseCase{OrganizationRepo: orgRepo}}

	app := buildHumaOrganizationApp(t, handler, true)

	req := httptest.NewRequest(http.MethodGet, "/v1/organizations?limit=10&page=1", nil)
	resp, err := app.Test(req, -1)
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

func TestHuma_GetAllOrganizations_BadQuery_Canonical400(t *testing.T) {
	// NOT parallel: process-global huma state.
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	// Service must never be reached: ValidateParameters rejects limit=abc with the
	// canonical 400 (ErrInvalidQueryParameter), NOT a native Huma 422.
	handler := &OrganizationHandler{Query: &query.UseCase{OrganizationRepo: organization.NewMockRepository(ctrl)}}

	app := buildHumaOrganizationApp(t, handler, true)

	req := httptest.NewRequest(http.MethodGet, "/v1/organizations?limit=abc", nil)
	resp, err := app.Test(req, -1)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	respBody, _ := io.ReadAll(resp.Body)
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode, "bad query stays canonical 400 — no native Huma 422")

	var got map[string]any
	require.NoError(t, json.Unmarshal(respBody, &got), "body: %s", string(respBody))
	assert.Equal(t, constant.ErrInvalidQueryParameter.Error(), got["code"])
}

func TestHuma_GetAllOrganizations_InvalidStatus_Canonical400(t *testing.T) {
	// NOT parallel: process-global huma state.
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	// The organization-specific status guard rejects an out-of-allowlist status with
	// the canonical 400 before any repo call — service never reached.
	handler := &OrganizationHandler{Query: &query.UseCase{OrganizationRepo: organization.NewMockRepository(ctrl)}}

	app := buildHumaOrganizationApp(t, handler, true)

	req := httptest.NewRequest(http.MethodGet, "/v1/organizations?status=BOGUS", nil)
	resp, err := app.Test(req, -1)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	respBody, _ := io.ReadAll(resp.Body)
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode, "invalid status stays canonical 400")

	var got map[string]any
	require.NoError(t, json.Unmarshal(respBody, &got), "body: %s", string(respBody))
	assert.Equal(t, constant.ErrInvalidQueryParameter.Error(), got["code"])
}

func TestHuma_DeleteOrganization_204Empty(t *testing.T) {
	// NOT parallel: process-global huma state.
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	// Ensure the production-environment guard is not triggered.
	t.Setenv("ENV_NAME", "development")

	orgID := uuid.New()

	orgRepo := organization.NewMockRepository(ctrl)
	orgRepo.EXPECT().Delete(gomock.Any(), orgID).Return(nil).Times(1)

	handler := &OrganizationHandler{Command: &command.UseCase{OrganizationRepo: orgRepo}}

	app := buildHumaOrganizationApp(t, handler, true)

	req := httptest.NewRequest(http.MethodDelete, "/v1/organizations/"+orgID.String(), nil)
	resp, err := app.Test(req, -1)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	respBody, _ := io.ReadAll(resp.Body)
	assert.Equal(t, http.StatusNoContent, resp.StatusCode)
	assert.Empty(t, respBody, "DELETE 204 must have an empty body")
}

func TestHuma_DeleteOrganization_ProductionForbidden(t *testing.T) {
	// NOT parallel: process-global huma state.
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	// The production guard rejects DELETE with the canonical ErrActionNotPermitted
	// before any repo call — service never reached.
	t.Setenv("ENV_NAME", "production")

	orgID := uuid.New()

	// No repo expectations: the guard fires before Command.DeleteOrganizationByID.
	handler := &OrganizationHandler{Command: &command.UseCase{OrganizationRepo: organization.NewMockRepository(ctrl)}}

	app := buildHumaOrganizationApp(t, handler, true)

	req := httptest.NewRequest(http.MethodDelete, "/v1/organizations/"+orgID.String(), nil)
	resp, err := app.Test(req, -1)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	respBody, _ := io.ReadAll(resp.Body)

	var got map[string]any
	require.NoError(t, json.Unmarshal(respBody, &got), "body: %s", string(respBody))
	assert.Equal(t, constant.ErrActionNotPermitted.Error(), got["code"], "production DELETE guard preserved across transports")
}

func TestHuma_CountOrganizations_204WithHeader(t *testing.T) {
	// NOT parallel: process-global huma state.
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	orgRepo := organization.NewMockRepository(ctrl)
	orgRepo.EXPECT().Count(gomock.Any()).Return(int64(42), nil).Times(1)

	handler := &OrganizationHandler{Query: &query.UseCase{OrganizationRepo: orgRepo}}

	app := buildHumaOrganizationApp(t, handler, true)

	req := httptest.NewRequest(http.MethodHead, "/v1/organizations/metrics/count", nil)
	resp, err := app.Test(req, -1)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	respBody, _ := io.ReadAll(resp.Body)
	assert.Equal(t, http.StatusNoContent, resp.StatusCode)
	assert.Equal(t, "42", resp.Header.Get(constant.XTotalCount), "X-Total-Count header must carry the count")
	assert.Empty(t, respBody, "HEAD count must have an empty body")
	assert.Equal(t, "0", resp.Header.Get("Content-Length"), "HEAD 204 must set Content-Length: 0 (parity with the Fiber NoContent path)")
}
