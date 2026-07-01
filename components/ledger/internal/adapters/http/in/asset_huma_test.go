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
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/account"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/asset"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/balance"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/services/command"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/services/query"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	pkgHTTP "github.com/LerianStudio/midaz/v4/pkg/net/http"
)

// buildHumaAssetApp mounts the six asset Huma operations on a /v1 group, faithfully
// mirroring the production wiring in unified-server.go: problem.Install() runs
// before any huma.Register, the Huma API is built with openapi.New over a /v1
// group, an auth-shim middleware stands in for auth.Authorize + tenant
// PostAuthMiddlewares (so the auth-preserved contract can be probed), and
// http.ParseUUIDPathParameters("asset") + RegisterAssetRoutes attach the chain.
//
// MUST-NOT-PARALLELIZE (same rationale as the tracer's buildHumaRuleApp):
// libProblem.Install() swaps the process-global huma.NewError hook and Huma
// validation uses process-global sync.Pools — concurrent builds/requests
// cross-contaminate. -race does not catch the logical contamination. These
// tests are sub-second; keep them sequential.
//
// authOK=false makes the shim reject with the ledger's canonical 401 envelope
// (mirroring auth.Authorize failure) so the auth-preserved contract is testable
// without a live lib-auth server.
func buildHumaAssetApp(t *testing.T, handler *AssetHandler, orgID, ledgerID uuid.UUID, authOK bool) *fiber.App {
	t.Helper()

	f := fiber.New(fiber.Config{
		DisableStartupMessage: true,
		ErrorHandler:          pkgHTTP.CanonicalFiberErrorHandler,
	})

	// problem.Install must run before any huma.Register (runtime + spec-gen).
	libProblem.Install()

	apiV1 := f.Group("/v1")

	// Auth shim: stands in for auth.Authorize("midaz","assets",verb). A rejected
	// request (authOK=false) must never reach Huma — it returns the ledger 401.
	apiV1.Use(func(c *fiber.Ctx) error {
		if !authOK {
			return pkgHTTP.Unauthorized(c, "0001", "Unauthorized", "auth required")
		}

		return c.Next()
	})

	hAPI := openapi.New(f, apiV1, openapi.Config{Title: "ledger-test", Version: "test", Servers: []string{"/v1"}})

	// Mirror the production chain: ParseUUIDPathParameters runs as a Fiber
	// middleware (no terminal handler) before the Huma terminal on each asset
	// route. Registered group-relative on apiV1 so Fiber prepends /v1 — matching
	// the group-relative paths RegisterAssetRoutes registers on the Huma API.
	parse := pkgHTTP.ParseUUIDPathParameters("asset")
	base := "/organizations/:organization_id/ledgers/:ledger_id/assets"
	apiV1.Post(base, parse)
	apiV1.Patch(base+"/:id", parse)
	apiV1.Get(base, parse)
	apiV1.Get(base+"/:id", parse)
	apiV1.Delete(base+"/:id", parse)
	apiV1.Head(base+"/metrics/count", parse)

	RegisterAssetRoutes(hAPI, handler)

	return f
}

func TestHuma_CreateAsset_Success(t *testing.T) {
	// NOT parallel: buildHumaAssetApp mutates process-global huma state.
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	orgID := uuid.New()
	ledgerID := uuid.New()

	assetRepo := asset.NewMockRepository(ctrl)
	metadataRepo := mongodb.NewMockRepository(ctrl)
	accountRepo := account.NewMockRepository(ctrl)
	balanceRepo := balance.NewMockRepository(ctrl)

	assetRepo.EXPECT().FindByNameOrCode(gomock.Any(), orgID, ledgerID, "Test Asset", "TST").Return(false, nil).Times(1)
	assetRepo.EXPECT().Create(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ any, a *mmodel.Asset) (*mmodel.Asset, error) {
			a.ID = uuid.New().String()
			a.CreatedAt = time.Now()
			a.UpdatedAt = time.Now()
			return a, nil
		}).Times(1)
	// The shared body pipeline (DecodeAndValidate -> parseMetadata) initializes
	// Metadata to a non-nil empty map when the body carries no "metadata" key, so
	// CreateOnboardingMetadata persists it — faithful to the production Fiber
	// WithBody path (the legacy unit test bypassed WithBody, so it never saw this).
	metadataRepo.EXPECT().Create(gomock.Any(), constant.EntityAsset, gomock.Any()).Return(nil).Times(1)
	accountRepo.EXPECT().ListAccountsByAlias(gomock.Any(), orgID, ledgerID, []string{"@external/TST"}).Return([]*mmodel.Account{}, nil).Times(1)
	accountRepo.EXPECT().Create(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ any, acc *mmodel.Account) (*mmodel.Account, error) { return acc, nil }).Times(1)
	balanceRepo.EXPECT().ExistsByAccountIDAndKey(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(false, nil).Times(1)
	balanceRepo.EXPECT().Create(gomock.Any(), gomock.Any()).Return(nil, nil).Times(1)

	handler := &AssetHandler{Command: &command.UseCase{
		AssetRepo:              assetRepo,
		OnboardingMetadataRepo: metadataRepo,
		AccountRepo:            accountRepo,
		BalanceRepo:            balanceRepo,
	}}

	app := buildHumaAssetApp(t, handler, orgID, ledgerID, true)

	body, _ := json.Marshal(map[string]any{"name": "Test Asset", "type": "commodity", "code": "TST"})
	req := httptest.NewRequest(http.MethodPost, "/v1/organizations/"+orgID.String()+"/ledgers/"+ledgerID.String()+"/assets", bytes.NewReader(body))
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
	assert.Equal(t, "Test Asset", got["name"])
	assert.Equal(t, "TST", got["code"])
	assert.Equal(t, orgID.String(), got["organizationId"])
	assert.Equal(t, ledgerID.String(), got["ledgerId"])
}

func TestHuma_CreateAsset_AuthPreserved(t *testing.T) {
	// NOT parallel: process-global huma state.
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	orgID := uuid.New()
	ledgerID := uuid.New()

	// No repo expectations: a rejected auth must never reach the service.
	handler := &AssetHandler{Command: &command.UseCase{
		AssetRepo:              asset.NewMockRepository(ctrl),
		OnboardingMetadataRepo: mongodb.NewMockRepository(ctrl),
		AccountRepo:            account.NewMockRepository(ctrl),
		BalanceRepo:            balance.NewMockRepository(ctrl),
	}}

	app := buildHumaAssetApp(t, handler, orgID, ledgerID, false)

	body, _ := json.Marshal(map[string]any{"name": "Test Asset", "type": "commodity", "code": "TST"})
	req := httptest.NewRequest(http.MethodPost, "/v1/organizations/"+orgID.String()+"/ledgers/"+ledgerID.String()+"/assets", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req, -1)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode, "auth middleware must reject before Huma; no public route")
}

func TestHuma_CreateAsset_ValidationError_Canonical400(t *testing.T) {
	// NOT parallel: process-global huma state.
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	orgID := uuid.New()
	ledgerID := uuid.New()

	// Missing required "name" -> imperative ValidateStruct -> canonical 400, service never reached.
	handler := &AssetHandler{Command: &command.UseCase{
		AssetRepo:              asset.NewMockRepository(ctrl),
		OnboardingMetadataRepo: mongodb.NewMockRepository(ctrl),
		AccountRepo:            account.NewMockRepository(ctrl),
		BalanceRepo:            balance.NewMockRepository(ctrl),
	}}

	app := buildHumaAssetApp(t, handler, orgID, ledgerID, true)

	body, _ := json.Marshal(map[string]any{"type": "commodity", "code": "TST"})
	req := httptest.NewRequest(http.MethodPost, "/v1/organizations/"+orgID.String()+"/ledgers/"+ledgerID.String()+"/assets", bytes.NewReader(body))
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

func TestHuma_CreateAsset_MalformedBody_Canonical400(t *testing.T) {
	// NOT parallel: process-global huma state.
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	orgID := uuid.New()
	ledgerID := uuid.New()

	// Malformed JSON -> DecodeAndValidate returns a pkg.ResponseError (0094). The
	// Fiber path writes it flat at 400; HumaProblem must project it to problem+json
	// at 400 (NOT the 500 fallback and NOT a native Huma 422). Service never reached.
	handler := &AssetHandler{Command: &command.UseCase{
		AssetRepo:              asset.NewMockRepository(ctrl),
		OnboardingMetadataRepo: mongodb.NewMockRepository(ctrl),
		AccountRepo:            account.NewMockRepository(ctrl),
		BalanceRepo:            balance.NewMockRepository(ctrl),
	}}

	app := buildHumaAssetApp(t, handler, orgID, ledgerID, true)

	req := httptest.NewRequest(http.MethodPost, "/v1/organizations/"+orgID.String()+"/ledgers/"+ledgerID.String()+"/assets", bytes.NewReader([]byte("{not valid json")))
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

func TestHuma_GetAssetByID_Success(t *testing.T) {
	// NOT parallel: process-global huma state.
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	orgID := uuid.New()
	ledgerID := uuid.New()
	assetID := uuid.New()

	assetRepo := asset.NewMockRepository(ctrl)
	metadataRepo := mongodb.NewMockRepository(ctrl)

	assetRepo.EXPECT().Find(gomock.Any(), orgID, ledgerID, assetID).
		Return(&mmodel.Asset{ID: assetID.String(), Name: "USD", Code: "USD", Type: "currency"}, nil).Times(1)
	metadataRepo.EXPECT().FindByEntity(gomock.Any(), constant.EntityAsset, assetID.String()).Return(nil, nil).Times(1)

	handler := &AssetHandler{Query: &query.UseCase{AssetRepo: assetRepo, OnboardingMetadataRepo: metadataRepo}}

	app := buildHumaAssetApp(t, handler, orgID, ledgerID, true)

	req := httptest.NewRequest(http.MethodGet, "/v1/organizations/"+orgID.String()+"/ledgers/"+ledgerID.String()+"/assets/"+assetID.String(), nil)
	resp, err := app.Test(req, -1)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	respBody, _ := io.ReadAll(resp.Body)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.NotContains(t, string(respBody), "$schema")

	var got map[string]any
	require.NoError(t, json.Unmarshal(respBody, &got), "body: %s", string(respBody))
	assert.Equal(t, "USD", got["name"])
	assert.Equal(t, assetID.String(), got["id"])
}

func TestHuma_GetAssetByID_BadUUID_Canonical400(t *testing.T) {
	// NOT parallel: process-global huma state.
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	orgID := uuid.New()
	ledgerID := uuid.New()

	// Service must never be reached: ParseUUIDPathParameters rejects the bad id
	// with the canonical 0065 / 400 before Huma.
	handler := &AssetHandler{Query: &query.UseCase{
		AssetRepo:              asset.NewMockRepository(ctrl),
		OnboardingMetadataRepo: mongodb.NewMockRepository(ctrl),
	}}

	app := buildHumaAssetApp(t, handler, orgID, ledgerID, true)

	req := httptest.NewRequest(http.MethodGet, "/v1/organizations/"+orgID.String()+"/ledgers/"+ledgerID.String()+"/assets/not-a-uuid", nil)
	resp, err := app.Test(req, -1)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	respBody, _ := io.ReadAll(resp.Body)
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode, "bad path UUID stays canonical 400 — no native Huma 422")

	var got map[string]any
	require.NoError(t, json.Unmarshal(respBody, &got), "body: %s", string(respBody))
	assert.Equal(t, constant.ErrInvalidPathParameter.Error(), got["code"])
}

func TestHuma_GetAllAssets_Success(t *testing.T) {
	// NOT parallel: process-global huma state.
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	orgID := uuid.New()
	ledgerID := uuid.New()

	assetRepo := asset.NewMockRepository(ctrl)
	assetRepo.EXPECT().FindAll(gomock.Any(), orgID, ledgerID, gomock.Any()).Return([]*mmodel.Asset{}, nil).Times(1)

	handler := &AssetHandler{Query: &query.UseCase{AssetRepo: assetRepo}}

	app := buildHumaAssetApp(t, handler, orgID, ledgerID, true)

	req := httptest.NewRequest(http.MethodGet, "/v1/organizations/"+orgID.String()+"/ledgers/"+ledgerID.String()+"/assets?limit=10&page=1", nil)
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

func TestHuma_GetAllAssets_BadQuery_Canonical400(t *testing.T) {
	// NOT parallel: process-global huma state.
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	orgID := uuid.New()
	ledgerID := uuid.New()

	// Service must never be reached: ValidateParameters rejects limit=abc with
	// the canonical 400 (ErrInvalidQueryParameter), NOT a native Huma 422.
	handler := &AssetHandler{Query: &query.UseCase{AssetRepo: asset.NewMockRepository(ctrl)}}

	app := buildHumaAssetApp(t, handler, orgID, ledgerID, true)

	req := httptest.NewRequest(http.MethodGet, "/v1/organizations/"+orgID.String()+"/ledgers/"+ledgerID.String()+"/assets?limit=abc", nil)
	resp, err := app.Test(req, -1)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	respBody, _ := io.ReadAll(resp.Body)
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode, "bad query stays canonical 400 — no native Huma 422")

	var got map[string]any
	require.NoError(t, json.Unmarshal(respBody, &got), "body: %s", string(respBody))
	assert.Equal(t, constant.ErrInvalidQueryParameter.Error(), got["code"])
}

func TestHuma_DeleteAsset_204Empty(t *testing.T) {
	// NOT parallel: process-global huma state.
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	orgID := uuid.New()
	ledgerID := uuid.New()
	assetID := uuid.New()

	assetRepo := asset.NewMockRepository(ctrl)
	accountRepo := account.NewMockRepository(ctrl)

	// Delete flow: Find the asset (for its Code), look up its external account
	// (none -> no account Delete), then delete the asset.
	assetRepo.EXPECT().Find(gomock.Any(), orgID, ledgerID, assetID).
		Return(&mmodel.Asset{ID: assetID.String(), Code: "TST"}, nil).Times(1)
	accountRepo.EXPECT().ListAccountsByAlias(gomock.Any(), orgID, ledgerID, gomock.Any()).Return([]*mmodel.Account{}, nil).Times(1)
	assetRepo.EXPECT().Delete(gomock.Any(), orgID, ledgerID, assetID).Return(nil).Times(1)

	handler := &AssetHandler{Command: &command.UseCase{
		AssetRepo:   assetRepo,
		AccountRepo: accountRepo,
	}}

	app := buildHumaAssetApp(t, handler, orgID, ledgerID, true)

	req := httptest.NewRequest(http.MethodDelete, "/v1/organizations/"+orgID.String()+"/ledgers/"+ledgerID.String()+"/assets/"+assetID.String(), nil)
	resp, err := app.Test(req, -1)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	respBody, _ := io.ReadAll(resp.Body)
	assert.Equal(t, http.StatusNoContent, resp.StatusCode)
	assert.Empty(t, respBody, "DELETE 204 must have an empty body")
}

func TestHuma_CountAssets_204WithHeader(t *testing.T) {
	// NOT parallel: process-global huma state.
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	orgID := uuid.New()
	ledgerID := uuid.New()

	assetRepo := asset.NewMockRepository(ctrl)
	assetRepo.EXPECT().Count(gomock.Any(), orgID, ledgerID).Return(int64(42), nil).Times(1)

	handler := &AssetHandler{Query: &query.UseCase{AssetRepo: assetRepo}}

	app := buildHumaAssetApp(t, handler, orgID, ledgerID, true)

	req := httptest.NewRequest(http.MethodHead, "/v1/organizations/"+orgID.String()+"/ledgers/"+ledgerID.String()+"/assets/metrics/count", nil)
	resp, err := app.Test(req, -1)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	respBody, _ := io.ReadAll(resp.Body)
	assert.Equal(t, http.StatusNoContent, resp.StatusCode)
	assert.Equal(t, "42", resp.Header.Get(constant.XTotalCount), "X-Total-Count header must carry the count")
	assert.Empty(t, respBody, "HEAD count must have an empty body")

	_ = libProblem.BaseURI
}
