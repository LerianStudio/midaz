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
	"go.uber.org/mock/gomock"

	mongodb "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/mongodb/onboarding"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/account"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/asset"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/balance"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/services/command"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/services/query"
	"github.com/LerianStudio/midaz/v4/pkg"
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
		ErrorHandler: pkgHTTP.CanonicalFiberErrorHandler,
	})

	// problem.Install must run before any huma.Register (runtime + spec-gen).
	libProblem.Install()

	apiV1 := f.Group("/v1")

	// Auth shim: stands in for auth.Authorize("midaz","assets",verb). A rejected
	// request (authOK=false) must never reach Huma — it returns the ledger 401.
	apiV1.Use(func(c fiber.Ctx) error {
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

	RegisterAssetRoutes(hAPI, handler, routeOpSuffixV1)

	return f
}

func TestCreateAsset_Success(t *testing.T) {
	// NOT parallel: buildHumaAssetApp mutates process-global huma state.
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	orgID := uuid.Must(libCommons.GenerateUUIDv7())
	ledgerID := uuid.Must(libCommons.GenerateUUIDv7())

	assetRepo := asset.NewMockRepository(ctrl)
	metadataRepo := mongodb.NewMockRepository(ctrl)
	accountRepo := account.NewMockRepository(ctrl)
	balanceRepo := balance.NewMockRepository(ctrl)

	assetRepo.EXPECT().FindByNameOrCode(gomock.Any(), orgID, ledgerID, "Test Asset", "TST").Return(false, nil).Times(1)
	assetRepo.EXPECT().Create(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ any, a *mmodel.Asset) (*mmodel.Asset, error) {
			a.ID = uuid.Must(libCommons.GenerateUUIDv7()).String()
			a.CreatedAt = fixedTestTime
			a.UpdatedAt = fixedTestTime
			return a, nil
		}).Times(1)
	// The shared body pipeline (DecodeAndValidate -> parseMetadata) initializes
	// Metadata to a non-nil empty map when the body carries no "metadata" key, so
	// CreateOnboardingMetadata persists it.
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

	resp, err := app.Test(req, fiber.TestConfig{Timeout: 0})
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

func TestCreateAsset_AuthPreserved(t *testing.T) {
	// NOT parallel: process-global huma state.
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	orgID := uuid.Must(libCommons.GenerateUUIDv7())
	ledgerID := uuid.Must(libCommons.GenerateUUIDv7())

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

	resp, err := app.Test(req, fiber.TestConfig{Timeout: 0})
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode, "auth middleware must reject before Huma; no public route")
}

func TestCreateAsset_ValidationError_Canonical400(t *testing.T) {
	// NOT parallel: process-global huma state.
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	orgID := uuid.Must(libCommons.GenerateUUIDv7())
	ledgerID := uuid.Must(libCommons.GenerateUUIDv7())

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

func TestCreateAsset_MalformedBody_Canonical400(t *testing.T) {
	// NOT parallel: process-global huma state.
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	orgID := uuid.Must(libCommons.GenerateUUIDv7())
	ledgerID := uuid.Must(libCommons.GenerateUUIDv7())

	// Malformed JSON -> DecodeAndValidate returns a pkg.ResponseError (0094).
	// HumaProblem must project it to problem+json at 400 (NOT the 500 fallback and
	// NOT a native Huma 422). Service never reached.
	handler := &AssetHandler{Command: &command.UseCase{
		AssetRepo:              asset.NewMockRepository(ctrl),
		OnboardingMetadataRepo: mongodb.NewMockRepository(ctrl),
		AccountRepo:            account.NewMockRepository(ctrl),
		BalanceRepo:            balance.NewMockRepository(ctrl),
	}}

	app := buildHumaAssetApp(t, handler, orgID, ledgerID, true)

	req := httptest.NewRequest(http.MethodPost, "/v1/organizations/"+orgID.String()+"/ledgers/"+ledgerID.String()+"/assets", bytes.NewReader([]byte("{not valid json")))
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

func TestGetAssetByID_Success(t *testing.T) {
	// NOT parallel: process-global huma state.
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	orgID := uuid.Must(libCommons.GenerateUUIDv7())
	ledgerID := uuid.Must(libCommons.GenerateUUIDv7())
	assetID := uuid.Must(libCommons.GenerateUUIDv7())

	assetRepo := asset.NewMockRepository(ctrl)
	metadataRepo := mongodb.NewMockRepository(ctrl)

	assetRepo.EXPECT().Find(gomock.Any(), orgID, ledgerID, assetID).
		Return(&mmodel.Asset{ID: assetID.String(), Name: "USD", Code: "USD", Type: "currency"}, nil).Times(1)
	metadataRepo.EXPECT().FindByEntity(gomock.Any(), constant.EntityAsset, assetID.String()).Return(nil, nil).Times(1)

	handler := &AssetHandler{Query: &query.UseCase{AssetRepo: assetRepo, OnboardingMetadataRepo: metadataRepo}}

	app := buildHumaAssetApp(t, handler, orgID, ledgerID, true)

	req := httptest.NewRequest(http.MethodGet, "/v1/organizations/"+orgID.String()+"/ledgers/"+ledgerID.String()+"/assets/"+assetID.String(), nil)
	resp, err := app.Test(req, fiber.TestConfig{Timeout: 0})
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

func TestGetAssetByID_BadUUID_Canonical400(t *testing.T) {
	// NOT parallel: process-global huma state.
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	orgID := uuid.Must(libCommons.GenerateUUIDv7())
	ledgerID := uuid.Must(libCommons.GenerateUUIDv7())

	// Service must never be reached: ParseUUIDPathParameters rejects the bad id
	// with the canonical 0065 / 400 before Huma.
	handler := &AssetHandler{Query: &query.UseCase{
		AssetRepo:              asset.NewMockRepository(ctrl),
		OnboardingMetadataRepo: mongodb.NewMockRepository(ctrl),
	}}

	app := buildHumaAssetApp(t, handler, orgID, ledgerID, true)

	req := httptest.NewRequest(http.MethodGet, "/v1/organizations/"+orgID.String()+"/ledgers/"+ledgerID.String()+"/assets/not-a-uuid", nil)
	resp, err := app.Test(req, fiber.TestConfig{Timeout: 0})
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	respBody, _ := io.ReadAll(resp.Body)
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode, "bad path UUID stays canonical 400 — no native Huma 422")

	var got map[string]any
	require.NoError(t, json.Unmarshal(respBody, &got), "body: %s", string(respBody))
	assert.Equal(t, constant.ErrInvalidPathParameter.Error(), got["code"])
}

func TestGetAllAssets_Success(t *testing.T) {
	// NOT parallel: process-global huma state.
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	orgID := uuid.Must(libCommons.GenerateUUIDv7())
	ledgerID := uuid.Must(libCommons.GenerateUUIDv7())

	assetRepo := asset.NewMockRepository(ctrl)
	assetRepo.EXPECT().FindAll(gomock.Any(), orgID, ledgerID, gomock.Any()).Return([]*mmodel.Asset{}, nil).Times(1)

	handler := &AssetHandler{Query: &query.UseCase{AssetRepo: assetRepo}}

	app := buildHumaAssetApp(t, handler, orgID, ledgerID, true)

	req := httptest.NewRequest(http.MethodGet, "/v1/organizations/"+orgID.String()+"/ledgers/"+ledgerID.String()+"/assets?limit=10&page=1", nil)
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

func TestGetAllAssets_BadQuery_Canonical400(t *testing.T) {
	// NOT parallel: process-global huma state.
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	orgID := uuid.Must(libCommons.GenerateUUIDv7())
	ledgerID := uuid.Must(libCommons.GenerateUUIDv7())

	// Service must never be reached: ValidateParameters rejects limit=abc with
	// the canonical 400 (ErrInvalidQueryParameter), NOT a native Huma 422.
	handler := &AssetHandler{Query: &query.UseCase{AssetRepo: asset.NewMockRepository(ctrl)}}

	app := buildHumaAssetApp(t, handler, orgID, ledgerID, true)

	req := httptest.NewRequest(http.MethodGet, "/v1/organizations/"+orgID.String()+"/ledgers/"+ledgerID.String()+"/assets?limit=abc", nil)
	resp, err := app.Test(req, fiber.TestConfig{Timeout: 0})
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	respBody, _ := io.ReadAll(resp.Body)
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode, "bad query stays canonical 400 — no native Huma 422")

	var got map[string]any
	require.NoError(t, json.Unmarshal(respBody, &got), "body: %s", string(respBody))
	assert.Equal(t, constant.ErrInvalidQueryParameter.Error(), got["code"])
}

func TestDeleteAsset_204Empty(t *testing.T) {
	// NOT parallel: process-global huma state.
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	orgID := uuid.Must(libCommons.GenerateUUIDv7())
	ledgerID := uuid.Must(libCommons.GenerateUUIDv7())
	assetID := uuid.Must(libCommons.GenerateUUIDv7())

	assetRepo := asset.NewMockRepository(ctrl)
	accountRepo := account.NewMockRepository(ctrl)

	// Delete flow: Find the asset (for its Code), look up its external account
	// (none -> no account Delete), then delete the asset.
	assetRepo.EXPECT().Find(gomock.Any(), orgID, ledgerID, assetID).
		Return(&mmodel.Asset{ID: assetID.String(), Code: "TST"}, nil).Times(1)
	accountRepo.EXPECT().ListExternalAccountsByAssetCode(gomock.Any(), orgID, ledgerID, "TST").Return([]*mmodel.Account{}, nil).Times(1)
	assetRepo.EXPECT().Delete(gomock.Any(), orgID, ledgerID, assetID).Return(nil).Times(1)

	handler := &AssetHandler{Command: &command.UseCase{
		AssetRepo:   assetRepo,
		AccountRepo: accountRepo,
	}}

	app := buildHumaAssetApp(t, handler, orgID, ledgerID, true)

	req := httptest.NewRequest(http.MethodDelete, "/v1/organizations/"+orgID.String()+"/ledgers/"+ledgerID.String()+"/assets/"+assetID.String(), nil)
	resp, err := app.Test(req, fiber.TestConfig{Timeout: 0})
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	respBody, _ := io.ReadAll(resp.Body)
	assert.Equal(t, http.StatusNoContent, resp.StatusCode)
	assert.Empty(t, respBody, "DELETE 204 must have an empty body")
}

func TestCountAssets_204WithHeader(t *testing.T) {
	// NOT parallel: process-global huma state.
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	orgID := uuid.Must(libCommons.GenerateUUIDv7())
	ledgerID := uuid.Must(libCommons.GenerateUUIDv7())

	assetRepo := asset.NewMockRepository(ctrl)
	assetRepo.EXPECT().Count(gomock.Any(), orgID, ledgerID).Return(int64(42), nil).Times(1)

	handler := &AssetHandler{Query: &query.UseCase{AssetRepo: assetRepo}}

	app := buildHumaAssetApp(t, handler, orgID, ledgerID, true)

	req := httptest.NewRequest(http.MethodHead, "/v1/organizations/"+orgID.String()+"/ledgers/"+ledgerID.String()+"/assets/metrics/count", nil)
	resp, err := app.Test(req, fiber.TestConfig{Timeout: 0})
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	respBody, _ := io.ReadAll(resp.Body)
	assert.Equal(t, http.StatusNoContent, resp.StatusCode)
	assert.Equal(t, "42", resp.Header.Get(constant.XTotalCount), "X-Total-Count header must carry the count")
	assert.Empty(t, respBody, "HEAD count must have an empty body")
	assert.Equal(t, "0", resp.Header.Get("Content-Length"), "HEAD 204 must set Content-Length: 0 (parity with the Fiber NoContent path)")
}

//
// The six exported fiber.Ctx terminals on AssetHandler were deleted with the Huma
// migration; the branches their tests covered in the shared cores are exercised
// here through the live Huma transport instead.

func TestUpdateAsset_Success(t *testing.T) {
	// NOT parallel: process-global huma state.
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	orgID := uuid.Must(libCommons.GenerateUUIDv7())
	ledgerID := uuid.Must(libCommons.GenerateUUIDv7())
	assetID := uuid.Must(libCommons.GenerateUUIDv7())

	assetRepo := asset.NewMockRepository(ctrl)
	metadataRepo := mongodb.NewMockRepository(ctrl)

	assetRepo.EXPECT().Update(gomock.Any(), orgID, ledgerID, assetID, gomock.Any()).
		Return(&mmodel.Asset{
			ID:             assetID.String(),
			OrganizationID: orgID.String(),
			LedgerID:       ledgerID.String(),
			Name:           "Updated Asset Name",
			Code:           "TST",
			Type:           "commodity",
			Status:         mmodel.Status{Code: "ACTIVE"},
		}, nil).Times(1)
	metadataRepo.EXPECT().Update(gomock.Any(), constant.EntityAsset, assetID.String(), gomock.Any()).Return(nil).AnyTimes()
	metadataRepo.EXPECT().FindByEntity(gomock.Any(), constant.EntityAsset, assetID.String()).Return(nil, nil).AnyTimes()

	handler := &AssetHandler{Command: &command.UseCase{AssetRepo: assetRepo, OnboardingMetadataRepo: metadataRepo}}

	app := buildHumaAssetApp(t, handler, orgID, ledgerID, true)

	body, _ := json.Marshal(map[string]any{"name": "Updated Asset Name"})
	req := httptest.NewRequest(http.MethodPatch, "/v1/organizations/"+orgID.String()+"/ledgers/"+ledgerID.String()+"/assets/"+assetID.String(), bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req, fiber.TestConfig{Timeout: 0})
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	respBody, _ := io.ReadAll(resp.Body)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.NotContains(t, string(respBody), "$schema", "SchemaLinkTransformer must be zeroed")

	var got map[string]any
	require.NoError(t, json.Unmarshal(respBody, &got), "body: %s", string(respBody))
	assert.Equal(t, "Updated Asset Name", got["name"])
	assert.Equal(t, assetID.String(), got["id"])
}

func TestUpdateAsset_NotFound_Canonical404(t *testing.T) {
	// NOT parallel: process-global huma state. updateAsset's command-error branch.
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	orgID := uuid.Must(libCommons.GenerateUUIDv7())
	ledgerID := uuid.Must(libCommons.GenerateUUIDv7())
	assetID := uuid.Must(libCommons.GenerateUUIDv7())

	assetRepo := asset.NewMockRepository(ctrl)
	assetRepo.EXPECT().Update(gomock.Any(), orgID, ledgerID, assetID, gomock.Any()).
		Return(nil, pkg.ValidateBusinessError(constant.ErrAssetIDNotFound, constant.EntityAsset)).Times(1)

	handler := &AssetHandler{Command: &command.UseCase{AssetRepo: assetRepo}}

	app := buildHumaAssetApp(t, handler, orgID, ledgerID, true)

	body, _ := json.Marshal(map[string]any{"name": "Updated Asset Name"})
	req := httptest.NewRequest(http.MethodPatch, "/v1/organizations/"+orgID.String()+"/ledgers/"+ledgerID.String()+"/assets/"+assetID.String(), bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req, fiber.TestConfig{Timeout: 0})
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	respBody, _ := io.ReadAll(resp.Body)
	assert.Equal(t, http.StatusNotFound, resp.StatusCode)

	var got map[string]any
	require.NoError(t, json.Unmarshal(respBody, &got), "body: %s", string(respBody))
	assert.Equal(t, constant.ErrAssetIDNotFound.Error(), got["code"])
}

func TestCreateAsset_ServiceError_Canonical4xx(t *testing.T) {
	// NOT parallel: process-global huma state. createAsset's command-error branch: a
	// duplicate name/code is a canonical business error, not a 5xx.
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	orgID := uuid.Must(libCommons.GenerateUUIDv7())
	ledgerID := uuid.Must(libCommons.GenerateUUIDv7())

	assetRepo := asset.NewMockRepository(ctrl)
	assetRepo.EXPECT().FindByNameOrCode(gomock.Any(), orgID, ledgerID, "Test Asset", "TST").Return(false, nil).Times(1)
	assetRepo.EXPECT().Create(gomock.Any(), gomock.Any()).
		Return(nil, pkg.ValidateBusinessError(constant.ErrAssetNameOrCodeDuplicate, constant.EntityAsset)).Times(1)

	handler := &AssetHandler{Command: &command.UseCase{AssetRepo: assetRepo}}

	app := buildHumaAssetApp(t, handler, orgID, ledgerID, true)

	body, _ := json.Marshal(map[string]any{"name": "Test Asset", "type": "commodity", "code": "TST"})
	req := httptest.NewRequest(http.MethodPost, "/v1/organizations/"+orgID.String()+"/ledgers/"+ledgerID.String()+"/assets", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req, fiber.TestConfig{Timeout: 0})
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	respBody, _ := io.ReadAll(resp.Body)
	assert.GreaterOrEqual(t, resp.StatusCode, http.StatusBadRequest)
	assert.Less(t, resp.StatusCode, http.StatusInternalServerError, "a duplicate asset is a client error, never a 5xx")

	var got map[string]any
	require.NoError(t, json.Unmarshal(respBody, &got), "body: %s", string(respBody))
	assert.Contains(t, got, "code")
}

func TestGetAllAssets_MetadataFilter(t *testing.T) {
	// NOT parallel: process-global huma state. Exercises getAllAssets' metadata
	// branch (GetAllMetadataAssets), not the plain FindAll path.
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	orgID := uuid.Must(libCommons.GenerateUUIDv7())
	ledgerID := uuid.Must(libCommons.GenerateUUIDv7())
	asset1 := uuid.Must(libCommons.GenerateUUIDv7()).String()

	assetRepo := asset.NewMockRepository(ctrl)
	metadataRepo := mongodb.NewMockRepository(ctrl)

	metadataRepo.EXPECT().FindList(gomock.Any(), constant.EntityAsset, gomock.Any()).
		Return([]*mongodb.Metadata{{EntityID: asset1, Data: map[string]any{"tier": "premium"}}}, nil).Times(1)
	assetRepo.EXPECT().ListByIDs(gomock.Any(), orgID, ledgerID, gomock.Any()).
		Return([]*mmodel.Asset{{ID: asset1, Name: "Premium One", Code: "PRM", Type: "commodity"}}, nil).Times(1)

	handler := &AssetHandler{Query: &query.UseCase{AssetRepo: assetRepo, OnboardingMetadataRepo: metadataRepo}}

	app := buildHumaAssetApp(t, handler, orgID, ledgerID, true)

	req := httptest.NewRequest(http.MethodGet, "/v1/organizations/"+orgID.String()+"/ledgers/"+ledgerID.String()+"/assets?metadata.tier=premium", nil)
	resp, err := app.Test(req, fiber.TestConfig{Timeout: 0})
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	respBody, _ := io.ReadAll(resp.Body)
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var got map[string]any
	require.NoError(t, json.Unmarshal(respBody, &got), "body: %s", string(respBody))
	items, ok := got["items"].([]any)
	require.True(t, ok, "items should be an array")
	assert.Len(t, items, 1)
}

func TestGetAllAssets_MetadataFilter_NoMatch_Canonical404(t *testing.T) {
	// NOT parallel: process-global huma state. The metadata branch's error path:
	// no matching metadata is a canonical 404.
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	orgID := uuid.Must(libCommons.GenerateUUIDv7())
	ledgerID := uuid.Must(libCommons.GenerateUUIDv7())

	metadataRepo := mongodb.NewMockRepository(ctrl)
	metadataRepo.EXPECT().FindList(gomock.Any(), constant.EntityAsset, gomock.Any()).Return(nil, nil).Times(1)

	handler := &AssetHandler{Query: &query.UseCase{
		AssetRepo:              asset.NewMockRepository(ctrl),
		OnboardingMetadataRepo: metadataRepo,
	}}

	app := buildHumaAssetApp(t, handler, orgID, ledgerID, true)

	req := httptest.NewRequest(http.MethodGet, "/v1/organizations/"+orgID.String()+"/ledgers/"+ledgerID.String()+"/assets?metadata.tier=nonexistent", nil)
	resp, err := app.Test(req, fiber.TestConfig{Timeout: 0})
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	respBody, _ := io.ReadAll(resp.Body)
	assert.Equal(t, http.StatusNotFound, resp.StatusCode)

	var got map[string]any
	require.NoError(t, json.Unmarshal(respBody, &got), "body: %s", string(respBody))
	assert.Contains(t, got, "code")
}

func TestGetAllAssets_ServiceError_Canonical404(t *testing.T) {
	// NOT parallel: process-global huma state. getAllAssets' plain query-error branch.
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	orgID := uuid.Must(libCommons.GenerateUUIDv7())
	ledgerID := uuid.Must(libCommons.GenerateUUIDv7())

	assetRepo := asset.NewMockRepository(ctrl)
	assetRepo.EXPECT().FindAll(gomock.Any(), orgID, ledgerID, gomock.Any()).
		Return(nil, pkg.ValidateBusinessError(constant.ErrNoAssetsFound, constant.EntityAsset)).Times(1)

	handler := &AssetHandler{Query: &query.UseCase{AssetRepo: assetRepo}}

	app := buildHumaAssetApp(t, handler, orgID, ledgerID, true)

	req := httptest.NewRequest(http.MethodGet, "/v1/organizations/"+orgID.String()+"/ledgers/"+ledgerID.String()+"/assets", nil)
	resp, err := app.Test(req, fiber.TestConfig{Timeout: 0})
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	respBody, _ := io.ReadAll(resp.Body)
	assert.Equal(t, http.StatusNotFound, resp.StatusCode)

	var got map[string]any
	require.NoError(t, json.Unmarshal(respBody, &got), "body: %s", string(respBody))
	assert.Equal(t, constant.ErrNoAssetsFound.Error(), got["code"])
}

func TestGetAssetByID_ServiceError_Canonical404(t *testing.T) {
	// NOT parallel: process-global huma state. getAssetByID's query-error branch.
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	orgID := uuid.Must(libCommons.GenerateUUIDv7())
	ledgerID := uuid.Must(libCommons.GenerateUUIDv7())
	assetID := uuid.Must(libCommons.GenerateUUIDv7())

	assetRepo := asset.NewMockRepository(ctrl)
	assetRepo.EXPECT().Find(gomock.Any(), orgID, ledgerID, assetID).
		Return(nil, pkg.ValidateBusinessError(constant.ErrAssetIDNotFound, constant.EntityAsset)).Times(1)

	handler := &AssetHandler{Query: &query.UseCase{AssetRepo: assetRepo}}

	app := buildHumaAssetApp(t, handler, orgID, ledgerID, true)

	req := httptest.NewRequest(http.MethodGet, "/v1/organizations/"+orgID.String()+"/ledgers/"+ledgerID.String()+"/assets/"+assetID.String(), nil)
	resp, err := app.Test(req, fiber.TestConfig{Timeout: 0})
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	respBody, _ := io.ReadAll(resp.Body)
	assert.Equal(t, http.StatusNotFound, resp.StatusCode)

	var got map[string]any
	require.NoError(t, json.Unmarshal(respBody, &got), "body: %s", string(respBody))
	assert.Equal(t, constant.ErrAssetIDNotFound.Error(), got["code"])
}

func TestDeleteAsset_ServiceError_Canonical404(t *testing.T) {
	// NOT parallel: process-global huma state. deleteAsset's command-error branch.
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	orgID := uuid.Must(libCommons.GenerateUUIDv7())
	ledgerID := uuid.Must(libCommons.GenerateUUIDv7())
	assetID := uuid.Must(libCommons.GenerateUUIDv7())

	assetRepo := asset.NewMockRepository(ctrl)
	assetRepo.EXPECT().Find(gomock.Any(), orgID, ledgerID, assetID).
		Return(nil, pkg.ValidateBusinessError(constant.ErrAssetIDNotFound, constant.EntityAsset)).Times(1)

	handler := &AssetHandler{Command: &command.UseCase{AssetRepo: assetRepo}}

	app := buildHumaAssetApp(t, handler, orgID, ledgerID, true)

	req := httptest.NewRequest(http.MethodDelete, "/v1/organizations/"+orgID.String()+"/ledgers/"+ledgerID.String()+"/assets/"+assetID.String(), nil)
	resp, err := app.Test(req, fiber.TestConfig{Timeout: 0})
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	respBody, _ := io.ReadAll(resp.Body)
	assert.Equal(t, http.StatusNotFound, resp.StatusCode)

	var got map[string]any
	require.NoError(t, json.Unmarshal(respBody, &got), "body: %s", string(respBody))
	assert.Equal(t, constant.ErrAssetIDNotFound.Error(), got["code"])
}

func TestCountAssets_ServiceError(t *testing.T) {
	// NOT parallel: process-global huma state. countAssets' query-error branch: the
	// HEAD op must surface the canonical status with no body.
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	orgID := uuid.Must(libCommons.GenerateUUIDv7())
	ledgerID := uuid.Must(libCommons.GenerateUUIDv7())

	assetRepo := asset.NewMockRepository(ctrl)
	assetRepo.EXPECT().Count(gomock.Any(), orgID, ledgerID).
		Return(int64(0), pkg.ValidateBusinessError(constant.ErrNoAssetsFound, constant.EntityAsset)).Times(1)

	handler := &AssetHandler{Query: &query.UseCase{AssetRepo: assetRepo}}

	app := buildHumaAssetApp(t, handler, orgID, ledgerID, true)

	req := httptest.NewRequest(http.MethodHead, "/v1/organizations/"+orgID.String()+"/ledgers/"+ledgerID.String()+"/assets/metrics/count", nil)
	resp, err := app.Test(req, fiber.TestConfig{Timeout: 0})
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	assert.Equal(t, http.StatusNotFound, resp.StatusCode)
	assert.Empty(t, resp.Header.Get(constant.XTotalCount), "a failed count must not advertise a total")
}
