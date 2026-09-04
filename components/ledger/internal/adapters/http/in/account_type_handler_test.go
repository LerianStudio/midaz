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

	libCommons "github.com/LerianStudio/lib-commons/v7/commons"

	libHTTP "github.com/LerianStudio/lib-commons/v7/commons/net/http"
	openapi "github.com/LerianStudio/lib-commons/v7/commons/net/http/openapi"
	libProblem "github.com/LerianStudio/lib-commons/v7/commons/net/http/problem"
	"github.com/gofiber/fiber/v3"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	ledgerMiddleware "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/http/in/middleware"
	mongodb "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/mongodb/onboarding"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/accounttype"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/services/command"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/services/query"
	"github.com/LerianStudio/midaz/v4/pkg"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	pkgHTTP "github.com/LerianStudio/midaz/v4/pkg/net/http"
)

// buildHumaAccountTypeApp mounts the five account-type Huma operations on a /v1
// group, mirroring the production wiring (RegisterAccountTypeRoutesToApp) and the
// asset exemplar (buildHumaAssetApp): problem.Install() runs before any
// huma.Register, the Huma API is built with openapi.New over a /v1 group, an
// auth-shim middleware stands in for auth.Authorize + tenant PostAuthMiddlewares (so
// the auth-preserved contract can be probed), and http.ParseUUIDPathParameters
// ("account_type") + RegisterAccountTypeRoutes attach the chain.
//
// MUST-NOT-PARALLELIZE (same rationale as buildHumaAssetApp): libProblem.Install()
// swaps the process-global huma.NewError hook and Huma validation uses process-global
// sync.Pools — concurrent builds/requests cross-contaminate. These tests are
// sub-second; keep them sequential.
//
// authOK=false makes the shim reject with the ledger's canonical 401 envelope
// (mirroring the midaz-appName auth.Authorize failure) so the auth-preserved
// contract is testable without a live lib-auth server.
func buildHumaAccountTypeApp(t *testing.T, handler *AccountTypeHandler, authOK bool) *fiber.App {
	t.Helper()

	f := fiber.New(fiber.Config{
		ErrorHandler: pkgHTTP.CanonicalFiberErrorHandler,
	})

	// problem.Install must run before any huma.Register (runtime + spec-gen).
	libProblem.Install()

	// Mirror production: the ledger registers ErrorEnvelope on the app root, so
	// /v1 serves the v3 envelope. Without it these assertions lock a shape no
	// deployed ledger returns.
	f.Use(ledgerMiddleware.ErrorEnvelope())

	apiV1 := f.Group("/v1")

	// Auth shim: stands in for protectedMidaz -> auth.Authorize("midaz",
	// "account-types", verb). A rejected request (authOK=false) must never reach
	// Huma — it returns the ledger 401.
	apiV1.Use(func(c fiber.Ctx) error {
		if !authOK {
			return pkgHTTP.Unauthorized(c, "0001", "Unauthorized", "auth required")
		}

		return c.Next()
	})

	hAPI := openapi.New(f, apiV1, openapi.Config{Title: "ledger-test", Version: "test", Servers: []string{"/v1"}})

	// Mirror the production chain: ParseUUIDPathParameters runs as a Fiber middleware
	// (no terminal handler) before the Huma terminal on each account-type route.
	// Registered group-relative on apiV1 so Fiber prepends /v1 — matching the
	// group-relative paths RegisterAccountTypeRoutes registers on the Huma API.
	parse := pkgHTTP.ParseUUIDPathParameters("account_type")
	base := "/organizations/:organization_id/ledgers/:ledger_id/account-types"
	apiV1.Post(base, parse)
	apiV1.Get(base, parse)
	apiV1.Get(base+"/:id", parse)
	apiV1.Patch(base+"/:id", parse)
	apiV1.Delete(base+"/:id", parse)

	RegisterAccountTypeRoutes(hAPI, handler, v1OpSuffix)

	return f
}

func TestCreateAccountType_Success(t *testing.T) {
	// NOT parallel: buildHumaAccountTypeApp mutates process-global huma state.
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	orgID := uuid.Must(libCommons.GenerateUUIDv7())
	ledgerID := uuid.Must(libCommons.GenerateUUIDv7())

	accountTypeRepo := accounttype.NewMockRepository(ctrl)
	metadataRepo := mongodb.NewMockRepository(ctrl)

	accountTypeRepo.EXPECT().Create(gomock.Any(), orgID, ledgerID, gomock.Any()).
		DoAndReturn(func(_ any, _, _ uuid.UUID, at *mmodel.AccountType) (*mmodel.AccountType, error) {
			at.ID = uuid.Must(libCommons.GenerateUUIDv7())
			at.CreatedAt = fixedTestTime
			at.UpdatedAt = fixedTestTime
			return at, nil
		}).Times(1)
	// The shared body pipeline (DecodeAndValidate -> parseMetadata) initializes
	// Metadata to a non-nil empty map when the body carries no "metadata" key, so
	// CreateOnboardingMetadata persists it.
	metadataRepo.EXPECT().Create(gomock.Any(), constant.EntityAccountType, gomock.Any()).Return(nil).Times(1)

	handler := &AccountTypeHandler{Command: &command.UseCase{
		AccountTypeRepo:        accountTypeRepo,
		OnboardingMetadataRepo: metadataRepo,
	}}

	app := buildHumaAccountTypeApp(t, handler, true)

	body, _ := json.Marshal(map[string]any{"name": "Test Account Type", "keyValue": "test_account_type"})
	req := httptest.NewRequest(http.MethodPost, "/v1/organizations/"+orgID.String()+"/ledgers/"+ledgerID.String()+"/account-types", bytes.NewReader(body))
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
	assert.Equal(t, "Test Account Type", got["name"])
	assert.Equal(t, "test_account_type", got["keyValue"])
	assert.Equal(t, orgID.String(), got["organizationId"])
	assert.Equal(t, ledgerID.String(), got["ledgerId"])
}

func TestCreateAccountType_AuthPreserved(t *testing.T) {
	// NOT parallel: process-global huma state.
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	orgID := uuid.Must(libCommons.GenerateUUIDv7())
	ledgerID := uuid.Must(libCommons.GenerateUUIDv7())

	// No repo expectations: a rejected auth must never reach the service.
	handler := &AccountTypeHandler{Command: &command.UseCase{
		AccountTypeRepo:        accounttype.NewMockRepository(ctrl),
		OnboardingMetadataRepo: mongodb.NewMockRepository(ctrl),
	}}

	app := buildHumaAccountTypeApp(t, handler, false)

	body, _ := json.Marshal(map[string]any{"name": "Test Account Type", "keyValue": "test_account_type"})
	req := httptest.NewRequest(http.MethodPost, "/v1/organizations/"+orgID.String()+"/ledgers/"+ledgerID.String()+"/account-types", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req, fiber.TestConfig{Timeout: 0})
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode, "auth middleware must reject before Huma; no public route")
}

func TestCreateAccountType_ValidationError_Canonical400(t *testing.T) {
	// NOT parallel: process-global huma state.
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	orgID := uuid.Must(libCommons.GenerateUUIDv7())
	ledgerID := uuid.Must(libCommons.GenerateUUIDv7())

	// Missing required "name"/"keyValue" -> imperative ValidateStruct -> canonical
	// 400, service never reached.
	handler := &AccountTypeHandler{Command: &command.UseCase{
		AccountTypeRepo:        accounttype.NewMockRepository(ctrl),
		OnboardingMetadataRepo: mongodb.NewMockRepository(ctrl),
	}}

	app := buildHumaAccountTypeApp(t, handler, true)

	body, _ := json.Marshal(map[string]any{})
	req := httptest.NewRequest(http.MethodPost, "/v1/organizations/"+orgID.String()+"/ledgers/"+ledgerID.String()+"/account-types", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req, fiber.TestConfig{Timeout: 0})
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	respBody, _ := io.ReadAll(resp.Body)

	assert.Equal(t, http.StatusBadRequest, resp.StatusCode, "imperative validation stays 400 — no native Huma 422")
	assert.Equal(t, "application/json", resp.Header.Get("Content-Type"))

	var got map[string]any
	require.NoError(t, json.Unmarshal(respBody, &got), "body: %s", string(respBody))
	assert.NotEmpty(t, got["code"], "canonical code present")
	assert.NotContains(t, got, "status", "the v1 envelope carries no status member")
}

func TestCreateAccountType_MalformedBody_Canonical400(t *testing.T) {
	// NOT parallel: process-global huma state.
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	orgID := uuid.Must(libCommons.GenerateUUIDv7())
	ledgerID := uuid.Must(libCommons.GenerateUUIDv7())

	// Malformed JSON -> DecodeAndValidate returns a pkg.ResponseError (0094) at 400
	// (NOT the 500 fallback and NOT a native Huma 422). This is a /v1 route, so it
	// reaches the client as the legacy application/json envelope. Service never reached.
	handler := &AccountTypeHandler{Command: &command.UseCase{
		AccountTypeRepo:        accounttype.NewMockRepository(ctrl),
		OnboardingMetadataRepo: mongodb.NewMockRepository(ctrl),
	}}

	app := buildHumaAccountTypeApp(t, handler, true)

	req := httptest.NewRequest(http.MethodPost, "/v1/organizations/"+orgID.String()+"/ledgers/"+ledgerID.String()+"/account-types", bytes.NewReader([]byte("{not valid json")))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req, fiber.TestConfig{Timeout: 0})
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	respBody, _ := io.ReadAll(resp.Body)

	assert.Equal(t, http.StatusBadRequest, resp.StatusCode, "malformed body stays 400 — no 500, no native 422")
	assert.Equal(t, "application/json", resp.Header.Get("Content-Type"))

	var got map[string]any
	require.NoError(t, json.Unmarshal(respBody, &got), "body: %s", string(respBody))
	assert.Equal(t, constant.ErrInvalidRequestBody.Error(), got["code"], "malformed-body code preserved (0094)")
	assert.NotContains(t, got, "status", "the v1 envelope carries no status member")
}

func TestGetAccountTypeByID_Success(t *testing.T) {
	// NOT parallel: process-global huma state.
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	orgID := uuid.Must(libCommons.GenerateUUIDv7())
	ledgerID := uuid.Must(libCommons.GenerateUUIDv7())
	accountTypeID := uuid.Must(libCommons.GenerateUUIDv7())

	accountTypeRepo := accounttype.NewMockRepository(ctrl)
	metadataRepo := mongodb.NewMockRepository(ctrl)

	accountTypeRepo.EXPECT().FindByID(gomock.Any(), orgID, ledgerID, accountTypeID).
		Return(&mmodel.AccountType{ID: accountTypeID, Name: "Current Assets", KeyValue: "current_assets"}, nil).Times(1)
	metadataRepo.EXPECT().FindByEntity(gomock.Any(), constant.EntityAccountType, accountTypeID.String()).Return(nil, nil).Times(1)

	handler := &AccountTypeHandler{Query: &query.UseCase{AccountTypeRepo: accountTypeRepo, OnboardingMetadataRepo: metadataRepo}}

	app := buildHumaAccountTypeApp(t, handler, true)

	req := httptest.NewRequest(http.MethodGet, "/v1/organizations/"+orgID.String()+"/ledgers/"+ledgerID.String()+"/account-types/"+accountTypeID.String(), nil)
	resp, err := app.Test(req, fiber.TestConfig{Timeout: 0})
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	respBody, _ := io.ReadAll(resp.Body)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.NotContains(t, string(respBody), "$schema")

	var got map[string]any
	require.NoError(t, json.Unmarshal(respBody, &got), "body: %s", string(respBody))
	assert.Equal(t, "Current Assets", got["name"])
	assert.Equal(t, accountTypeID.String(), got["id"])
}

func TestGetAccountTypeByID_BadUUID_Canonical400(t *testing.T) {
	// NOT parallel: process-global huma state.
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	orgID := uuid.Must(libCommons.GenerateUUIDv7())
	ledgerID := uuid.Must(libCommons.GenerateUUIDv7())

	// Service must never be reached: ParseUUIDPathParameters rejects the bad id with
	// the canonical 0065 / 400 before Huma.
	handler := &AccountTypeHandler{Query: &query.UseCase{
		AccountTypeRepo:        accounttype.NewMockRepository(ctrl),
		OnboardingMetadataRepo: mongodb.NewMockRepository(ctrl),
	}}

	app := buildHumaAccountTypeApp(t, handler, true)

	req := httptest.NewRequest(http.MethodGet, "/v1/organizations/"+orgID.String()+"/ledgers/"+ledgerID.String()+"/account-types/not-a-uuid", nil)
	resp, err := app.Test(req, fiber.TestConfig{Timeout: 0})
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	respBody, _ := io.ReadAll(resp.Body)
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode, "bad path UUID stays canonical 400 — no native Huma 422")

	var got map[string]any
	require.NoError(t, json.Unmarshal(respBody, &got), "body: %s", string(respBody))
	assert.Equal(t, constant.ErrInvalidPathParameter.Error(), got["code"])
}

func TestGetAllAccountTypes_Success(t *testing.T) {
	// NOT parallel: process-global huma state.
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	orgID := uuid.Must(libCommons.GenerateUUIDv7())
	ledgerID := uuid.Must(libCommons.GenerateUUIDv7())

	accountTypeRepo := accounttype.NewMockRepository(ctrl)
	metadataRepo := mongodb.NewMockRepository(ctrl)
	accountTypeRepo.EXPECT().FindAll(gomock.Any(), orgID, ledgerID, gomock.Any()).
		Return([]*mmodel.AccountType{}, libHTTP.CursorPagination{}, nil).Times(1)
	// A non-nil (even empty) result triggers the metadata hydration path (FindList).
	metadataRepo.EXPECT().FindList(gomock.Any(), constant.EntityAccountType, gomock.Any()).Return(nil, nil).Times(1)

	handler := &AccountTypeHandler{Query: &query.UseCase{AccountTypeRepo: accountTypeRepo, OnboardingMetadataRepo: metadataRepo}}

	app := buildHumaAccountTypeApp(t, handler, true)

	req := httptest.NewRequest(http.MethodGet, "/v1/organizations/"+orgID.String()+"/ledgers/"+ledgerID.String()+"/account-types?limit=10", nil)
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

func TestGetAllAccountTypes_BadQuery_Canonical400(t *testing.T) {
	// NOT parallel: process-global huma state.
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	orgID := uuid.Must(libCommons.GenerateUUIDv7())
	ledgerID := uuid.Must(libCommons.GenerateUUIDv7())

	// Service must never be reached: ValidateParameters rejects limit=abc with the
	// canonical 400 (ErrInvalidQueryParameter), NOT a native Huma 422.
	handler := &AccountTypeHandler{Query: &query.UseCase{AccountTypeRepo: accounttype.NewMockRepository(ctrl)}}

	app := buildHumaAccountTypeApp(t, handler, true)

	req := httptest.NewRequest(http.MethodGet, "/v1/organizations/"+orgID.String()+"/ledgers/"+ledgerID.String()+"/account-types?limit=abc", nil)
	resp, err := app.Test(req, fiber.TestConfig{Timeout: 0})
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	respBody, _ := io.ReadAll(resp.Body)
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode, "bad query stays canonical 400 — no native Huma 422")

	var got map[string]any
	require.NoError(t, json.Unmarshal(respBody, &got), "body: %s", string(respBody))
	assert.Equal(t, constant.ErrInvalidQueryParameter.Error(), got["code"])
}

func TestUpdateAccountType_Success(t *testing.T) {
	// NOT parallel: process-global huma state.
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	orgID := uuid.Must(libCommons.GenerateUUIDv7())
	ledgerID := uuid.Must(libCommons.GenerateUUIDv7())
	accountTypeID := uuid.Must(libCommons.GenerateUUIDv7())

	accountTypeRepo := accounttype.NewMockRepository(ctrl)
	metadataRepo := mongodb.NewMockRepository(ctrl)

	accountTypeRepo.EXPECT().Update(gomock.Any(), orgID, ledgerID, accountTypeID, gomock.Any()).
		Return(&mmodel.AccountType{ID: accountTypeID, Name: "Renamed", KeyValue: "current_assets"}, nil).Times(1)
	// The shared body pipeline initializes Metadata to a non-nil empty map, so
	// UpdateOnboardingMetadata takes the FindByEntity + Update path.
	metadataRepo.EXPECT().FindByEntity(gomock.Any(), constant.EntityAccountType, accountTypeID.String()).Return(nil, nil).Times(1)
	metadataRepo.EXPECT().Update(gomock.Any(), constant.EntityAccountType, accountTypeID.String(), gomock.Any()).Return(nil).Times(1)

	handler := &AccountTypeHandler{Command: &command.UseCase{
		AccountTypeRepo:        accountTypeRepo,
		OnboardingMetadataRepo: metadataRepo,
	}}

	app := buildHumaAccountTypeApp(t, handler, true)

	body, _ := json.Marshal(map[string]any{"name": "Renamed"})
	req := httptest.NewRequest(http.MethodPatch, "/v1/organizations/"+orgID.String()+"/ledgers/"+ledgerID.String()+"/account-types/"+accountTypeID.String(), bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req, fiber.TestConfig{Timeout: 0})
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	respBody, _ := io.ReadAll(resp.Body)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.NotContains(t, string(respBody), "$schema")

	var got map[string]any
	require.NoError(t, json.Unmarshal(respBody, &got), "body: %s", string(respBody))
	assert.Equal(t, "Renamed", got["name"])
	assert.Equal(t, accountTypeID.String(), got["id"])
}

func TestDeleteAccountType_204Empty(t *testing.T) {
	// NOT parallel: process-global huma state.
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	orgID := uuid.Must(libCommons.GenerateUUIDv7())
	ledgerID := uuid.Must(libCommons.GenerateUUIDv7())
	accountTypeID := uuid.Must(libCommons.GenerateUUIDv7())

	accountTypeRepo := accounttype.NewMockRepository(ctrl)
	accountTypeRepo.EXPECT().Delete(gomock.Any(), orgID, ledgerID, accountTypeID).Return(nil).Times(1)

	handler := &AccountTypeHandler{Command: &command.UseCase{AccountTypeRepo: accountTypeRepo}}

	app := buildHumaAccountTypeApp(t, handler, true)

	req := httptest.NewRequest(http.MethodDelete, "/v1/organizations/"+orgID.String()+"/ledgers/"+ledgerID.String()+"/account-types/"+accountTypeID.String(), nil)
	resp, err := app.Test(req, fiber.TestConfig{Timeout: 0})
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	respBody, _ := io.ReadAll(resp.Body)
	assert.Equal(t, http.StatusNoContent, resp.StatusCode)
	assert.Empty(t, respBody, "DELETE 204 must have an empty body")
}

func TestCreateAccountType_DuplicateKeyValue_409(t *testing.T) {
	// NOT parallel: process-global huma state.
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	orgID := uuid.Must(libCommons.GenerateUUIDv7())
	ledgerID := uuid.Must(libCommons.GenerateUUIDv7())

	accountTypeRepo := accounttype.NewMockRepository(ctrl)
	// The command rejects the duplicate key value, so metadata is never written.
	accountTypeRepo.EXPECT().Create(gomock.Any(), orgID, ledgerID, gomock.Any()).
		Return(nil, pkg.ValidateBusinessError(constant.ErrDuplicateAccountTypeKeyValue, constant.EntityAccountType)).Times(1)

	handler := &AccountTypeHandler{Command: &command.UseCase{
		AccountTypeRepo:        accountTypeRepo,
		OnboardingMetadataRepo: mongodb.NewMockRepository(ctrl),
	}}

	app := buildHumaAccountTypeApp(t, handler, true)

	body, _ := json.Marshal(map[string]any{"name": "Existing Account Type", "keyValue": "existing_key"})
	req := httptest.NewRequest(http.MethodPost, "/v1/organizations/"+orgID.String()+"/ledgers/"+ledgerID.String()+"/account-types", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req, fiber.TestConfig{Timeout: 0})
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	respBody, _ := io.ReadAll(resp.Body)
	assert.Equal(t, http.StatusConflict, resp.StatusCode)

	var got map[string]any
	require.NoError(t, json.Unmarshal(respBody, &got), "body: %s", string(respBody))
	assert.Equal(t, constant.ErrDuplicateAccountTypeKeyValue.Error(), got["code"])
}

func TestGetAccountTypeByID_NotFound_404(t *testing.T) {
	// NOT parallel: process-global huma state.
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	orgID := uuid.Must(libCommons.GenerateUUIDv7())
	ledgerID := uuid.Must(libCommons.GenerateUUIDv7())
	accountTypeID := uuid.Must(libCommons.GenerateUUIDv7())

	accountTypeRepo := accounttype.NewMockRepository(ctrl)
	accountTypeRepo.EXPECT().FindByID(gomock.Any(), orgID, ledgerID, accountTypeID).
		Return(nil, pkg.ValidateBusinessError(constant.ErrAccountTypeNotFound, constant.EntityAccountType)).Times(1)

	handler := &AccountTypeHandler{Query: &query.UseCase{
		AccountTypeRepo:        accountTypeRepo,
		OnboardingMetadataRepo: mongodb.NewMockRepository(ctrl),
	}}

	app := buildHumaAccountTypeApp(t, handler, true)

	req := httptest.NewRequest(http.MethodGet, "/v1/organizations/"+orgID.String()+"/ledgers/"+ledgerID.String()+"/account-types/"+accountTypeID.String(), nil)
	resp, err := app.Test(req, fiber.TestConfig{Timeout: 0})
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	respBody, _ := io.ReadAll(resp.Body)
	assert.Equal(t, http.StatusNotFound, resp.StatusCode)

	var got map[string]any
	require.NoError(t, json.Unmarshal(respBody, &got), "body: %s", string(respBody))
	assert.Equal(t, constant.ErrAccountTypeNotFound.Error(), got["code"])
}

func TestUpdateAccountType_NotFound_404(t *testing.T) {
	// NOT parallel: process-global huma state.
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	orgID := uuid.Must(libCommons.GenerateUUIDv7())
	ledgerID := uuid.Must(libCommons.GenerateUUIDv7())
	accountTypeID := uuid.Must(libCommons.GenerateUUIDv7())

	accountTypeRepo := accounttype.NewMockRepository(ctrl)
	// Update fails, so the metadata upsert that follows it never runs.
	accountTypeRepo.EXPECT().Update(gomock.Any(), orgID, ledgerID, accountTypeID, gomock.Any()).
		Return(nil, pkg.ValidateBusinessError(constant.ErrAccountTypeNotFound, constant.EntityAccountType)).Times(1)

	handler := &AccountTypeHandler{Command: &command.UseCase{
		AccountTypeRepo:        accountTypeRepo,
		OnboardingMetadataRepo: mongodb.NewMockRepository(ctrl),
	}}

	app := buildHumaAccountTypeApp(t, handler, true)

	body, _ := json.Marshal(map[string]any{"name": "Renamed"})
	req := httptest.NewRequest(http.MethodPatch, "/v1/organizations/"+orgID.String()+"/ledgers/"+ledgerID.String()+"/account-types/"+accountTypeID.String(), bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req, fiber.TestConfig{Timeout: 0})
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	respBody, _ := io.ReadAll(resp.Body)
	assert.Equal(t, http.StatusNotFound, resp.StatusCode)

	var got map[string]any
	require.NoError(t, json.Unmarshal(respBody, &got), "body: %s", string(respBody))
	assert.Equal(t, constant.ErrAccountTypeNotFound.Error(), got["code"])
}

func TestDeleteAccountType_NotFound_404(t *testing.T) {
	// NOT parallel: process-global huma state.
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	orgID := uuid.Must(libCommons.GenerateUUIDv7())
	ledgerID := uuid.Must(libCommons.GenerateUUIDv7())
	accountTypeID := uuid.Must(libCommons.GenerateUUIDv7())

	accountTypeRepo := accounttype.NewMockRepository(ctrl)
	accountTypeRepo.EXPECT().Delete(gomock.Any(), orgID, ledgerID, accountTypeID).
		Return(pkg.ValidateBusinessError(constant.ErrAccountTypeNotFound, constant.EntityAccountType)).Times(1)

	handler := &AccountTypeHandler{Command: &command.UseCase{AccountTypeRepo: accountTypeRepo}}

	app := buildHumaAccountTypeApp(t, handler, true)

	req := httptest.NewRequest(http.MethodDelete, "/v1/organizations/"+orgID.String()+"/ledgers/"+ledgerID.String()+"/account-types/"+accountTypeID.String(), nil)
	resp, err := app.Test(req, fiber.TestConfig{Timeout: 0})
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	respBody, _ := io.ReadAll(resp.Body)
	assert.Equal(t, http.StatusNotFound, resp.StatusCode)

	var got map[string]any
	require.NoError(t, json.Unmarshal(respBody, &got), "body: %s", string(respBody))
	assert.Equal(t, constant.ErrAccountTypeNotFound.Error(), got["code"])
}

func TestGetAllAccountTypes_MetadataFilter_Success(t *testing.T) {
	// NOT parallel: process-global huma state.
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	orgID := uuid.Must(libCommons.GenerateUUIDv7())
	ledgerID := uuid.Must(libCommons.GenerateUUIDv7())
	accountTypeID := uuid.Must(libCommons.GenerateUUIDv7())

	accountTypeRepo := accounttype.NewMockRepository(ctrl)
	metadataRepo := mongodb.NewMockRepository(ctrl)

	// A metadata filter routes the list through GetAllMetadataAccountType: FindList
	// resolves the matching entity IDs, then FindAll fetches those account types.
	metadataRepo.EXPECT().FindList(gomock.Any(), constant.EntityAccountType, gomock.Any()).
		Return([]*mongodb.Metadata{{EntityID: accountTypeID.String(), Data: map[string]any{"tier": "premium"}}}, nil).Times(1)
	accountTypeRepo.EXPECT().FindAll(gomock.Any(), orgID, ledgerID, gomock.Any()).
		Return([]*mmodel.AccountType{{
			ID:             accountTypeID,
			OrganizationID: orgID,
			LedgerID:       ledgerID,
			Name:           "Premium Account Type",
			KeyValue:       "premium_key",
		}}, libHTTP.CursorPagination{}, nil).Times(1)

	handler := &AccountTypeHandler{Query: &query.UseCase{AccountTypeRepo: accountTypeRepo, OnboardingMetadataRepo: metadataRepo}}

	app := buildHumaAccountTypeApp(t, handler, true)

	req := httptest.NewRequest(http.MethodGet, "/v1/organizations/"+orgID.String()+"/ledgers/"+ledgerID.String()+"/account-types?metadata.tier=premium", nil)
	resp, err := app.Test(req, fiber.TestConfig{Timeout: 0})
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	respBody, _ := io.ReadAll(resp.Body)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.NotContains(t, string(respBody), "$schema")

	var got map[string]any
	require.NoError(t, json.Unmarshal(respBody, &got), "body: %s", string(respBody))

	items, ok := got["items"].([]any)
	require.True(t, ok, "items should be an array; body: %s", string(respBody))
	require.Len(t, items, 1)

	first, ok := items[0].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, accountTypeID.String(), first["id"])
	assert.Equal(t, "Premium Account Type", first["name"])
}

func TestGetAllAccountTypes_MetadataFilter_NoMatch_404(t *testing.T) {
	// NOT parallel: process-global huma state.
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	orgID := uuid.Must(libCommons.GenerateUUIDv7())
	ledgerID := uuid.Must(libCommons.GenerateUUIDv7())

	metadataRepo := mongodb.NewMockRepository(ctrl)
	// No metadata matches, so FindAll is never reached.
	metadataRepo.EXPECT().FindList(gomock.Any(), constant.EntityAccountType, gomock.Any()).Return(nil, nil).Times(1)

	handler := &AccountTypeHandler{Query: &query.UseCase{
		AccountTypeRepo:        accounttype.NewMockRepository(ctrl),
		OnboardingMetadataRepo: metadataRepo,
	}}

	app := buildHumaAccountTypeApp(t, handler, true)

	req := httptest.NewRequest(http.MethodGet, "/v1/organizations/"+orgID.String()+"/ledgers/"+ledgerID.String()+"/account-types?metadata.tier=nonexistent", nil)
	resp, err := app.Test(req, fiber.TestConfig{Timeout: 0})
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	respBody, _ := io.ReadAll(resp.Body)
	assert.Equal(t, http.StatusNotFound, resp.StatusCode)

	var got map[string]any
	require.NoError(t, json.Unmarshal(respBody, &got), "body: %s", string(respBody))
	assert.Equal(t, constant.ErrNoAccountTypesFound.Error(), got["code"])
}

func TestGetAllAccountTypes_NotFound_404(t *testing.T) {
	// NOT parallel: process-global huma state.
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	orgID := uuid.Must(libCommons.GenerateUUIDv7())
	ledgerID := uuid.Must(libCommons.GenerateUUIDv7())

	accountTypeRepo := accounttype.NewMockRepository(ctrl)
	accountTypeRepo.EXPECT().FindAll(gomock.Any(), orgID, ledgerID, gomock.Any()).
		Return(nil, libHTTP.CursorPagination{}, pkg.ValidateBusinessError(constant.ErrNoAccountTypesFound, constant.EntityAccountType)).Times(1)

	handler := &AccountTypeHandler{Query: &query.UseCase{
		AccountTypeRepo:        accountTypeRepo,
		OnboardingMetadataRepo: mongodb.NewMockRepository(ctrl),
	}}

	app := buildHumaAccountTypeApp(t, handler, true)

	req := httptest.NewRequest(http.MethodGet, "/v1/organizations/"+orgID.String()+"/ledgers/"+ledgerID.String()+"/account-types", nil)
	resp, err := app.Test(req, fiber.TestConfig{Timeout: 0})
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	respBody, _ := io.ReadAll(resp.Body)
	assert.Equal(t, http.StatusNotFound, resp.StatusCode)

	var got map[string]any
	require.NoError(t, json.Unmarshal(respBody, &got), "body: %s", string(respBody))
	assert.Equal(t, constant.ErrNoAccountTypesFound.Error(), got["code"])
}
