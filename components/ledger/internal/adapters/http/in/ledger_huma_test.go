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
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/ledger"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/services/command"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/services/query"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	pkgHTTP "github.com/LerianStudio/midaz/v4/pkg/net/http"
)

// buildHumaLedgerApp mounts the eight ledger Huma operations on a /v1 group,
// faithfully mirroring the production wiring the deferred RegisterLedgerRoutesToApp
// will install: problem.Install() runs before any huma.Register, the Huma API is
// built with openapi.New over a /v1 group, an auth-shim middleware stands in for
// auth.Authorize("midaz","ledgers",verb) + tenant PostAuthMiddlewares (so the
// auth-preserved contract can be probed), and ParseUUIDPathParameters("ledger") +
// RegisterLedgerRoutes attach the chain.
//
// MUST-NOT-PARALLELIZE (same rationale as the asset exemplar's buildHumaAssetApp):
// libProblem.Install() swaps the process-global huma.NewError hook and Huma
// validation uses process-global sync.Pools — concurrent builds/requests
// cross-contaminate. -race does not catch the logical contamination. These tests
// are sub-second; keep them sequential.
//
// authOK=false makes the shim reject with the ledger's canonical 401 envelope
// (mirroring auth.Authorize failure) so the auth-preserved contract is testable
// without a live lib-auth server.
func buildHumaLedgerApp(t *testing.T, handler *LedgerHandler, authOK bool) *fiber.App {
	t.Helper()

	f := fiber.New(fiber.Config{
		DisableStartupMessage: true,
		ErrorHandler:          pkgHTTP.CanonicalFiberErrorHandler,
	})

	// problem.Install must run before any huma.Register (runtime + spec-gen).
	libProblem.Install()

	apiV1 := f.Group("/v1")

	// Auth shim: stands in for auth.Authorize("midaz","ledgers",verb). A rejected
	// request (authOK=false) must never reach Huma — it returns the ledger 401.
	apiV1.Use(func(c *fiber.Ctx) error {
		if !authOK {
			return pkgHTTP.Unauthorized(c, "0001", "Unauthorized", "auth required")
		}

		return c.Next()
	})

	hAPI := openapi.New(f, apiV1, openapi.Config{Title: "ledger-test", Version: "test", Servers: []string{"/v1"}})

	// Mirror the production chain: ParseUUIDPathParameters runs as a Fiber
	// middleware (no terminal handler) before the Huma terminal on each ledger
	// route. Registered group-relative on apiV1 so Fiber prepends /v1.
	parse := pkgHTTP.ParseUUIDPathParameters("ledger")
	list := "/organizations/:organization_id/ledgers"
	id := list + "/:ledger_id"
	apiV1.Post(list, parse)
	apiV1.Get(list, parse)
	apiV1.Get(id, parse)
	apiV1.Patch(id, parse)
	apiV1.Delete(id, parse)
	apiV1.Head(list+"/metrics/count", parse)
	apiV1.Get(id+"/settings", parse)
	apiV1.Patch(id+"/settings", parse)

	RegisterLedgerRoutes(hAPI, handler)

	return f
}

func TestHuma_CreateLedger_Success(t *testing.T) {
	// NOT parallel: buildHumaLedgerApp mutates process-global huma state.
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	orgID := uuid.New()

	ledgerRepo := ledger.NewMockRepository(ctrl)
	metadataRepo := mongodb.NewMockRepository(ctrl)

	ledgerRepo.EXPECT().FindByName(gomock.Any(), orgID, "Test Ledger").Return(true, nil).Times(1)
	ledgerRepo.EXPECT().Create(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ any, l *mmodel.Ledger) (*mmodel.Ledger, error) {
			l.ID = uuid.New().String()
			l.CreatedAt = time.Now()
			l.UpdatedAt = time.Now()
			return l, nil
		}).Times(1)
	metadataRepo.EXPECT().Create(gomock.Any(), constant.EntityLedger, gomock.Any()).Return(nil).Times(1)

	handler := &LedgerHandler{Command: &command.UseCase{
		LedgerRepo:             ledgerRepo,
		OnboardingMetadataRepo: metadataRepo,
	}}

	app := buildHumaLedgerApp(t, handler, true)

	body, _ := json.Marshal(map[string]any{"name": "Test Ledger"})
	req := httptest.NewRequest(http.MethodPost, "/v1/organizations/"+orgID.String()+"/ledgers", bytes.NewReader(body))
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
	assert.Equal(t, "Test Ledger", got["name"])
	assert.Equal(t, orgID.String(), got["organizationId"])
}

func TestHuma_CreateLedger_AuthPreserved(t *testing.T) {
	// NOT parallel: process-global huma state.
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	orgID := uuid.New()

	// No repo expectations: a rejected auth must never reach the service.
	handler := &LedgerHandler{Command: &command.UseCase{
		LedgerRepo:             ledger.NewMockRepository(ctrl),
		OnboardingMetadataRepo: mongodb.NewMockRepository(ctrl),
	}}

	app := buildHumaLedgerApp(t, handler, false)

	body, _ := json.Marshal(map[string]any{"name": "Test Ledger"})
	req := httptest.NewRequest(http.MethodPost, "/v1/organizations/"+orgID.String()+"/ledgers", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req, -1)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode, "auth middleware must reject before Huma; no public route")
}

func TestHuma_CreateLedger_ValidationError_Canonical400(t *testing.T) {
	// NOT parallel: process-global huma state.
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	orgID := uuid.New()

	// Missing required "name" -> imperative ValidateStruct -> canonical 400, service never reached.
	handler := &LedgerHandler{Command: &command.UseCase{
		LedgerRepo:             ledger.NewMockRepository(ctrl),
		OnboardingMetadataRepo: mongodb.NewMockRepository(ctrl),
	}}

	app := buildHumaLedgerApp(t, handler, true)

	body, _ := json.Marshal(map[string]any{"status": map[string]any{"code": "ACTIVE"}})
	req := httptest.NewRequest(http.MethodPost, "/v1/organizations/"+orgID.String()+"/ledgers", bytes.NewReader(body))
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

func TestHuma_CreateLedger_MalformedBody_Canonical400(t *testing.T) {
	// NOT parallel: process-global huma state.
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	orgID := uuid.New()

	handler := &LedgerHandler{Command: &command.UseCase{
		LedgerRepo:             ledger.NewMockRepository(ctrl),
		OnboardingMetadataRepo: mongodb.NewMockRepository(ctrl),
	}}

	app := buildHumaLedgerApp(t, handler, true)

	req := httptest.NewRequest(http.MethodPost, "/v1/organizations/"+orgID.String()+"/ledgers", bytes.NewReader([]byte("{not valid json")))
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

func TestHuma_GetLedgerByID_Success(t *testing.T) {
	// NOT parallel: process-global huma state.
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	orgID := uuid.New()
	ledgerID := uuid.New()

	ledgerRepo := ledger.NewMockRepository(ctrl)
	metadataRepo := mongodb.NewMockRepository(ctrl)

	ledgerRepo.EXPECT().Find(gomock.Any(), orgID, ledgerID).
		Return(&mmodel.Ledger{ID: ledgerID.String(), Name: "Main", OrganizationID: orgID.String()}, nil).Times(1)
	metadataRepo.EXPECT().FindByEntity(gomock.Any(), constant.EntityLedger, ledgerID.String()).Return(nil, nil).Times(1)

	handler := &LedgerHandler{Query: &query.UseCase{LedgerRepo: ledgerRepo, OnboardingMetadataRepo: metadataRepo}}

	app := buildHumaLedgerApp(t, handler, true)

	req := httptest.NewRequest(http.MethodGet, "/v1/organizations/"+orgID.String()+"/ledgers/"+ledgerID.String(), nil)
	resp, err := app.Test(req, -1)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	respBody, _ := io.ReadAll(resp.Body)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.NotContains(t, string(respBody), "$schema")

	var got map[string]any
	require.NoError(t, json.Unmarshal(respBody, &got), "body: %s", string(respBody))
	assert.Equal(t, "Main", got["name"])
	assert.Equal(t, ledgerID.String(), got["id"])
}

func TestHuma_GetLedgerByID_BadUUID_Canonical400(t *testing.T) {
	// NOT parallel: process-global huma state.
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	orgID := uuid.New()

	// Service must never be reached: ParseUUIDPathParameters rejects the bad id
	// with the canonical 0065 / 400 before Huma.
	handler := &LedgerHandler{Query: &query.UseCase{
		LedgerRepo:             ledger.NewMockRepository(ctrl),
		OnboardingMetadataRepo: mongodb.NewMockRepository(ctrl),
	}}

	app := buildHumaLedgerApp(t, handler, true)

	req := httptest.NewRequest(http.MethodGet, "/v1/organizations/"+orgID.String()+"/ledgers/not-a-uuid", nil)
	resp, err := app.Test(req, -1)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	respBody, _ := io.ReadAll(resp.Body)
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode, "bad path UUID stays canonical 400 — no native Huma 422")

	var got map[string]any
	require.NoError(t, json.Unmarshal(respBody, &got), "body: %s", string(respBody))
	assert.Equal(t, constant.ErrInvalidPathParameter.Error(), got["code"])
}

func TestHuma_GetAllLedgers_Success(t *testing.T) {
	// NOT parallel: process-global huma state.
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	orgID := uuid.New()

	ledgerRepo := ledger.NewMockRepository(ctrl)
	ledgerRepo.EXPECT().FindAll(gomock.Any(), orgID, gomock.Any()).Return([]*mmodel.Ledger{}, nil).Times(1)

	handler := &LedgerHandler{Query: &query.UseCase{LedgerRepo: ledgerRepo}}

	app := buildHumaLedgerApp(t, handler, true)

	req := httptest.NewRequest(http.MethodGet, "/v1/organizations/"+orgID.String()+"/ledgers?limit=10&page=1", nil)
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

func TestHuma_GetAllLedgers_BadQuery_Canonical400(t *testing.T) {
	// NOT parallel: process-global huma state.
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	orgID := uuid.New()

	// Service must never be reached: ValidateParameters rejects limit=abc with
	// the canonical 400, NOT a native Huma 422.
	handler := &LedgerHandler{Query: &query.UseCase{LedgerRepo: ledger.NewMockRepository(ctrl)}}

	app := buildHumaLedgerApp(t, handler, true)

	req := httptest.NewRequest(http.MethodGet, "/v1/organizations/"+orgID.String()+"/ledgers?limit=abc", nil)
	resp, err := app.Test(req, -1)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	respBody, _ := io.ReadAll(resp.Body)
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode, "bad query stays canonical 400 — no native Huma 422")

	var got map[string]any
	require.NoError(t, json.Unmarshal(respBody, &got), "body: %s", string(respBody))
	assert.Equal(t, constant.ErrInvalidQueryParameter.Error(), got["code"])
}

func TestHuma_GetAllLedgers_InvalidStatus_Canonical400(t *testing.T) {
	// NOT parallel: process-global huma state.
	// Ledger's list path has a bespoke status allowlist (ACTIVE/INACTIVE). An
	// out-of-allowlist status must yield the canonical 400 (0082), not reach the
	// service, and not become a native Huma 422.
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	orgID := uuid.New()

	handler := &LedgerHandler{Query: &query.UseCase{LedgerRepo: ledger.NewMockRepository(ctrl)}}

	app := buildHumaLedgerApp(t, handler, true)

	req := httptest.NewRequest(http.MethodGet, "/v1/organizations/"+orgID.String()+"/ledgers?status=BOGUS", nil)
	resp, err := app.Test(req, -1)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	respBody, _ := io.ReadAll(resp.Body)
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode, "invalid ledger status stays canonical 400")

	var got map[string]any
	require.NoError(t, json.Unmarshal(respBody, &got), "body: %s", string(respBody))
	assert.Equal(t, constant.ErrInvalidQueryParameter.Error(), got["code"])
}

func TestHuma_DeleteLedger_204Empty(t *testing.T) {
	// NOT parallel: process-global huma state.
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	orgID := uuid.New()
	ledgerID := uuid.New()

	ledgerRepo := ledger.NewMockRepository(ctrl)
	ledgerRepo.EXPECT().Delete(gomock.Any(), orgID, ledgerID).Return(nil).Times(1)

	handler := &LedgerHandler{Command: &command.UseCase{LedgerRepo: ledgerRepo}}

	app := buildHumaLedgerApp(t, handler, true)

	req := httptest.NewRequest(http.MethodDelete, "/v1/organizations/"+orgID.String()+"/ledgers/"+ledgerID.String(), nil)
	resp, err := app.Test(req, -1)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	respBody, _ := io.ReadAll(resp.Body)
	assert.Equal(t, http.StatusNoContent, resp.StatusCode)
	assert.Empty(t, respBody, "DELETE 204 must have an empty body")
}

func TestHuma_CountLedgers_204WithHeader(t *testing.T) {
	// NOT parallel: process-global huma state.
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	orgID := uuid.New()

	ledgerRepo := ledger.NewMockRepository(ctrl)
	ledgerRepo.EXPECT().Count(gomock.Any(), orgID).Return(int64(7), nil).Times(1)

	handler := &LedgerHandler{Query: &query.UseCase{LedgerRepo: ledgerRepo}}

	app := buildHumaLedgerApp(t, handler, true)

	req := httptest.NewRequest(http.MethodHead, "/v1/organizations/"+orgID.String()+"/ledgers/metrics/count", nil)
	resp, err := app.Test(req, -1)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	respBody, _ := io.ReadAll(resp.Body)
	assert.Equal(t, http.StatusNoContent, resp.StatusCode)
	assert.Equal(t, "7", resp.Header.Get(constant.XTotalCount), "X-Total-Count header must carry the count")
	assert.Empty(t, respBody, "HEAD count must have an empty body")
	assert.Equal(t, "0", resp.Header.Get("Content-Length"), "HEAD 204 must set Content-Length: 0 (parity with the Fiber NoContent path)")
}

func TestHuma_GetLedgerSettings_Success(t *testing.T) {
	// NOT parallel: process-global huma state.
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	orgID := uuid.New()
	ledgerID := uuid.New()

	ledgerRepo := ledger.NewMockRepository(ctrl)
	// OnboardingRedisRepo nil -> cache skipped -> GetSettings hits the repo.
	ledgerRepo.EXPECT().GetSettings(gomock.Any(), orgID, ledgerID).
		Return(map[string]any{"accounting": map[string]any{"validateRoutes": true}}, nil).Times(1)

	handler := &LedgerHandler{Query: &query.UseCase{LedgerRepo: ledgerRepo}}

	app := buildHumaLedgerApp(t, handler, true)

	req := httptest.NewRequest(http.MethodGet, "/v1/organizations/"+orgID.String()+"/ledgers/"+ledgerID.String()+"/settings", nil)
	resp, err := app.Test(req, -1)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	respBody, _ := io.ReadAll(resp.Body)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.NotContains(t, string(respBody), "$schema")

	var got map[string]any
	require.NoError(t, json.Unmarshal(respBody, &got), "body: %s", string(respBody))
	assert.Contains(t, got, "accounting")
}

// TestHuma_UpdateLedgerSettings_Success exercises the merge-patch happy path: a
// known allowlisted field is validated (ValidateSettings), deep-merged atomically,
// and the parsed settings returned at 200.
func TestHuma_UpdateLedgerSettings_Success(t *testing.T) {
	// NOT parallel: process-global huma state.
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	orgID := uuid.New()
	ledgerID := uuid.New()

	ledgerRepo := ledger.NewMockRepository(ctrl)
	ledgerRepo.EXPECT().UpdateSettingsAtomic(gomock.Any(), orgID, ledgerID, gomock.Any()).
		DoAndReturn(func(_ any, _, _ uuid.UUID, mergeFn func(map[string]any) (map[string]any, error)) (map[string]any, error) {
			return mergeFn(map[string]any{})
		}).Times(1)

	handler := &LedgerHandler{Command: &command.UseCase{LedgerRepo: ledgerRepo}}

	app := buildHumaLedgerApp(t, handler, true)

	body, _ := json.Marshal(map[string]any{"accounting": map[string]any{"validateRoutes": true}})
	req := httptest.NewRequest(http.MethodPatch, "/v1/organizations/"+orgID.String()+"/ledgers/"+ledgerID.String()+"/settings", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req, -1)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	respBody, _ := io.ReadAll(resp.Body)
	assert.Equal(t, http.StatusOK, resp.StatusCode, "body: %s", string(respBody))
	assert.NotContains(t, string(respBody), "$schema")

	var got map[string]any
	require.NoError(t, json.Unmarshal(respBody, &got), "body: %s", string(respBody))
	assert.Contains(t, got, "accounting")
}

// TestHuma_UpdateLedgerSettings_UnknownField_Canonical400 is the LANDMINE guard:
// the merge-patch allowlist (ValidateSettings) rejects an unknown field with the
// canonical 0147 at HTTP 400 — projected to problem+json, NEVER a native Huma 422
// and NEVER the 500 fallback. The atomic write must never be reached.
func TestHuma_UpdateLedgerSettings_UnknownField_Canonical400(t *testing.T) {
	// NOT parallel: process-global huma state.
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	orgID := uuid.New()
	ledgerID := uuid.New()

	// No UpdateSettingsAtomic expectation: validation rejects before the write.
	handler := &LedgerHandler{Command: &command.UseCase{LedgerRepo: ledger.NewMockRepository(ctrl)}}

	app := buildHumaLedgerApp(t, handler, true)

	body, _ := json.Marshal(map[string]any{"bogusField": true})
	req := httptest.NewRequest(http.MethodPatch, "/v1/organizations/"+orgID.String()+"/ledgers/"+ledgerID.String()+"/settings", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req, -1)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	respBody, _ := io.ReadAll(resp.Body)
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode, "unknown settings field stays canonical 400 — no 500, no native 422")
	assert.Equal(t, "application/problem+json", resp.Header.Get("Content-Type"))

	var got map[string]any
	require.NoError(t, json.Unmarshal(respBody, &got), "body: %s", string(respBody))
	assert.Equal(t, constant.ErrUnknownSettingsField.Error(), got["code"], "unknown-field code preserved (0147)")
	assert.Equal(t, float64(http.StatusBadRequest), got["status"])
}

// TestHuma_UpdateLedgerSettings_InvalidType_Canonical400 guards the sibling
// allowlist rejection: a known field with a wrong-typed value yields 0148 at 400.
func TestHuma_UpdateLedgerSettings_InvalidType_Canonical400(t *testing.T) {
	// NOT parallel: process-global huma state.
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	orgID := uuid.New()
	ledgerID := uuid.New()

	handler := &LedgerHandler{Command: &command.UseCase{LedgerRepo: ledger.NewMockRepository(ctrl)}}

	app := buildHumaLedgerApp(t, handler, true)

	// accounting.validateRoutes must be a bool; a string trips 0148.
	body, _ := json.Marshal(map[string]any{"accounting": map[string]any{"validateRoutes": "yes"}})
	req := httptest.NewRequest(http.MethodPatch, "/v1/organizations/"+orgID.String()+"/ledgers/"+ledgerID.String()+"/settings", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req, -1)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	respBody, _ := io.ReadAll(resp.Body)
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode, "invalid settings type stays canonical 400")

	var got map[string]any
	require.NoError(t, json.Unmarshal(respBody, &got), "body: %s", string(respBody))
	assert.Equal(t, constant.ErrInvalidSettingsFieldType.Error(), got["code"], "invalid-type code preserved (0148)")
	assert.Equal(t, float64(http.StatusBadRequest), got["status"])
}

func TestHuma_UpdateLedgerSettings_AuthPreserved(t *testing.T) {
	// NOT parallel: process-global huma state.
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	orgID := uuid.New()
	ledgerID := uuid.New()

	handler := &LedgerHandler{Command: &command.UseCase{LedgerRepo: ledger.NewMockRepository(ctrl)}}

	app := buildHumaLedgerApp(t, handler, false)

	body, _ := json.Marshal(map[string]any{"accounting": map[string]any{"validateRoutes": true}})
	req := httptest.NewRequest(http.MethodPatch, "/v1/organizations/"+orgID.String()+"/ledgers/"+ledgerID.String()+"/settings", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req, -1)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode, "auth middleware must reject before Huma; settings is not public")

	_ = libProblem.BaseURI
}
