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

	libHTTP "github.com/LerianStudio/lib-commons/v5/commons/net/http"
	openapi "github.com/LerianStudio/lib-commons/v5/commons/net/http/openapi"
	libProblem "github.com/LerianStudio/lib-commons/v5/commons/net/http/problem"
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	txmongodb "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/mongodb/transaction"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/assetrate"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/services/command"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/services/query"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	pkgHTTP "github.com/LerianStudio/midaz/v4/pkg/net/http"
)

// buildHumaAssetRateApp mounts the three asset-rate Huma operations on a /v1 group,
// faithfully mirroring the production wiring in unified-server.go and the asset
// exemplar (buildHumaAssetApp): problem.Install() runs before any huma.Register, the
// Huma API is built with openapi.New over a /v1 group, an auth-shim middleware stands
// in for protectedMidaz (auth.Authorize("midaz","asset-rates",verb) + tenant
// PostAuthMiddlewares), and ParseUUIDPathParameters("asset-rate") + RegisterAssetRateRoutes
// attach the chain. asset-rate is MONEY-adjacent (exchange rates), so the tests probe
// byte-identical responses and canonical (non-422) error codes.
//
// MUST-NOT-PARALLELIZE (same rationale as buildHumaAssetApp): libProblem.Install()
// swaps the process-global huma.NewError hook and Huma validation uses process-global
// sync.Pools — concurrent builds/requests cross-contaminate. These tests are
// sub-second; keep them sequential.
//
// authOK=false makes the shim reject with the ledger's canonical 401 envelope so the
// auth-preserved contract is testable without a live lib-auth server.
func buildHumaAssetRateApp(t *testing.T, handler *AssetRateHandler, authOK bool) *fiber.App {
	t.Helper()

	f := fiber.New(fiber.Config{
		DisableStartupMessage: true,
		ErrorHandler:          pkgHTTP.CanonicalFiberErrorHandler,
	})

	// problem.Install must run before any huma.Register (runtime + spec-gen).
	libProblem.Install()

	apiV1 := f.Group("/v1")

	// Auth shim: stands in for protectedMidaz("midaz","asset-rates",verb). A rejected
	// request (authOK=false) must never reach Huma — it returns the ledger 401.
	apiV1.Use(func(c *fiber.Ctx) error {
		if !authOK {
			return pkgHTTP.Unauthorized(c, "0001", "Unauthorized", "auth required")
		}

		return c.Next()
	})

	hAPI := openapi.New(f, apiV1, openapi.Config{Title: "ledger-test", Version: "test", Servers: []string{"/v1"}})

	// Mirror the production chain: ParseUUIDPathParameters runs as a Fiber middleware
	// (no terminal handler) before the Huma terminal on each asset-rate route.
	// Registered group-relative on apiV1 so Fiber prepends /v1 — matching the
	// group-relative paths RegisterAssetRateRoutes registers on the Huma API. Note the
	// entity-name is "asset-rate" (singular) exactly as in the pre-migration routes.go.
	parse := pkgHTTP.ParseUUIDPathParameters("asset-rate")
	base := "/organizations/:organization_id/ledgers/:ledger_id/asset-rates"
	apiV1.Put(base, parse)
	apiV1.Get(base+"/:external_id", parse)
	apiV1.Get(base+"/from/:asset_code", parse)

	RegisterAssetRateRoutes(hAPI, handler)

	return f
}

func TestHuma_CreateOrUpdateAssetRate_Success(t *testing.T) {
	// NOT parallel: buildHumaAssetRateApp mutates process-global huma state.
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	orgID := uuid.New()
	ledgerID := uuid.New()

	assetRateRepo := assetrate.NewMockRepository(ctrl)
	metadataRepo := txmongodb.NewMockRepository(ctrl)

	// New-record path: no existing pair -> Create. The shared body pipeline
	// (DecodeAndValidate -> parseMetadata) initializes Metadata to a non-nil empty
	// map when the body carries no "metadata" key, so the service persists it via
	// TransactionMetadataRepo.Create — faithful to the production Fiber WithBody path.
	assetRateRepo.EXPECT().FindByCurrencyPair(gomock.Any(), orgID, ledgerID, "USD", "BRL").Return(nil, nil).Times(1)
	assetRateRepo.EXPECT().Create(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ any, ar *assetrate.AssetRate) (*assetrate.AssetRate, error) {
			ar.CreatedAt = time.Now()
			ar.UpdatedAt = time.Now()
			return ar, nil
		}).Times(1)
	metadataRepo.EXPECT().Create(gomock.Any(), constant.EntityAssetRate, gomock.Any()).Return(nil).Times(1)

	handler := &AssetRateHandler{Command: &command.UseCase{AssetRateRepo: assetRateRepo, TransactionMetadataRepo: metadataRepo}}

	app := buildHumaAssetRateApp(t, handler, true)

	// ttl is dereferenced unconditionally by the service, so it must be present.
	body, _ := json.Marshal(map[string]any{"from": "USD", "to": "BRL", "rate": 100, "scale": 2, "ttl": 3600})
	req := httptest.NewRequest(http.MethodPut, "/v1/organizations/"+orgID.String()+"/ledgers/"+ledgerID.String()+"/asset-rates", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req, -1)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	respBody, _ := io.ReadAll(resp.Body)

	assert.Equal(t, http.StatusCreated, resp.StatusCode, "upsert returns 201 (parity with the Fiber http.Created path)")
	assert.NotContains(t, string(respBody), "$schema", "SchemaLinkTransformer must be zeroed")
	assert.NotContains(t, string(respBody), "$ref")

	var got map[string]any
	require.NoError(t, json.Unmarshal(respBody, &got), "body: %s", string(respBody))
	assert.Equal(t, "USD", got["from"])
	assert.Equal(t, "BRL", got["to"])
	assert.EqualValues(t, 100, got["rate"])
	// Tenant captured from the path into the persisted+echoed entity.
	assert.Equal(t, orgID.String(), got["organizationId"])
	assert.Equal(t, ledgerID.String(), got["ledgerId"])
}

func TestHuma_CreateOrUpdateAssetRate_AuthPreserved(t *testing.T) {
	// NOT parallel: process-global huma state.
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	orgID := uuid.New()
	ledgerID := uuid.New()

	// No repo expectations: a rejected auth must never reach the service.
	handler := &AssetRateHandler{Command: &command.UseCase{AssetRateRepo: assetrate.NewMockRepository(ctrl)}}

	app := buildHumaAssetRateApp(t, handler, false)

	body, _ := json.Marshal(map[string]any{"from": "USD", "to": "BRL", "rate": 100, "ttl": 3600})
	req := httptest.NewRequest(http.MethodPut, "/v1/organizations/"+orgID.String()+"/ledgers/"+ledgerID.String()+"/asset-rates", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req, -1)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode, "auth middleware must reject before Huma; no public route")
}

func TestHuma_CreateOrUpdateAssetRate_MalformedBody_Canonical400(t *testing.T) {
	// NOT parallel: process-global huma state.
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	orgID := uuid.New()
	ledgerID := uuid.New()

	// Malformed JSON -> DecodeAndValidate returns a pkg.ResponseError (0094). The
	// Fiber path writes it flat at 400; HumaProblem must project it to problem+json
	// at 400 (NOT the 500 fallback and NOT a native Huma 422). Service never reached.
	handler := &AssetRateHandler{Command: &command.UseCase{AssetRateRepo: assetrate.NewMockRepository(ctrl)}}

	app := buildHumaAssetRateApp(t, handler, true)

	req := httptest.NewRequest(http.MethodPut, "/v1/organizations/"+orgID.String()+"/ledgers/"+ledgerID.String()+"/asset-rates", bytes.NewReader([]byte("{not valid json")))
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

func TestHuma_GetAssetRateByExternalID_Success(t *testing.T) {
	// NOT parallel: process-global huma state.
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	orgID := uuid.New()
	ledgerID := uuid.New()
	externalID := uuid.New()
	rateID := uuid.New()

	assetRateRepo := assetrate.NewMockRepository(ctrl)
	metadataRepo := txmongodb.NewMockRepository(ctrl)
	// Return a fully-populated rate; the service then reads (empty) metadata by entity.
	assetRateRepo.EXPECT().FindByExternalID(gomock.Any(), orgID, ledgerID, externalID).
		Return(&assetrate.AssetRate{
			ID:             rateID.String(),
			OrganizationID: orgID.String(),
			LedgerID:       ledgerID.String(),
			ExternalID:     externalID.String(),
			From:           "USD",
			To:             "BRL",
			Rate:           100,
		}, nil).Times(1)
	metadataRepo.EXPECT().FindByEntity(gomock.Any(), constant.EntityAssetRate, rateID.String()).Return(nil, nil).Times(1)

	handler := &AssetRateHandler{Query: &query.UseCase{AssetRateRepo: assetRateRepo, TransactionMetadataRepo: metadataRepo}}

	app := buildHumaAssetRateApp(t, handler, true)

	req := httptest.NewRequest(http.MethodGet, "/v1/organizations/"+orgID.String()+"/ledgers/"+ledgerID.String()+"/asset-rates/"+externalID.String(), nil)
	resp, err := app.Test(req, -1)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	respBody, _ := io.ReadAll(resp.Body)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.NotContains(t, string(respBody), "$schema")

	var got map[string]any
	require.NoError(t, json.Unmarshal(respBody, &got), "body: %s", string(respBody))
	assert.Equal(t, rateID.String(), got["id"])
	assert.Equal(t, externalID.String(), got["externalId"])
	assert.Equal(t, "USD", got["from"])
	assert.Equal(t, "BRL", got["to"])
}

func TestHuma_GetAssetRateByExternalID_BadUUID_Canonical400(t *testing.T) {
	// NOT parallel: process-global huma state.
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	orgID := uuid.New()
	ledgerID := uuid.New()

	// Service must never be reached: ParseUUIDPathParameters rejects the bad
	// external_id with the canonical 0065 / 400 before Huma (external_id IS a
	// UUID path param).
	handler := &AssetRateHandler{Query: &query.UseCase{AssetRateRepo: assetrate.NewMockRepository(ctrl)}}

	app := buildHumaAssetRateApp(t, handler, true)

	req := httptest.NewRequest(http.MethodGet, "/v1/organizations/"+orgID.String()+"/ledgers/"+ledgerID.String()+"/asset-rates/not-a-uuid", nil)
	resp, err := app.Test(req, -1)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	respBody, _ := io.ReadAll(resp.Body)
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode, "bad path UUID stays canonical 400 — no native Huma 422")

	var got map[string]any
	require.NoError(t, json.Unmarshal(respBody, &got), "body: %s", string(respBody))
	assert.Equal(t, constant.ErrInvalidPathParameter.Error(), got["code"])
}

func TestHuma_GetAllAssetRatesByAssetCode_Success(t *testing.T) {
	// NOT parallel: process-global huma state.
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	orgID := uuid.New()
	ledgerID := uuid.New()

	assetRateRepo := assetrate.NewMockRepository(ctrl)
	// Return a nil slice + empty cursor: the service's `assetRates != nil` gate then
	// skips the metadata lookup, keeping the test focused on the transport contract.
	assetRateRepo.EXPECT().FindAllByAssetCodes(gomock.Any(), orgID, ledgerID, "USD", gomock.Any(), gomock.Any()).
		Return(nil, libHTTP.CursorPagination{}, nil).Times(1)

	handler := &AssetRateHandler{Query: &query.UseCase{AssetRateRepo: assetRateRepo}}

	app := buildHumaAssetRateApp(t, handler, true)

	req := httptest.NewRequest(http.MethodGet, "/v1/organizations/"+orgID.String()+"/ledgers/"+ledgerID.String()+"/asset-rates/from/USD?limit=10", nil)
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

func TestHuma_GetAllAssetRatesByAssetCode_BadQuery_Canonical400(t *testing.T) {
	// NOT parallel: process-global huma state.
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	orgID := uuid.New()
	ledgerID := uuid.New()

	// Service must never be reached: ValidateParameters rejects limit=abc with the
	// canonical 400 (ErrInvalidQueryParameter), NOT a native Huma 422.
	handler := &AssetRateHandler{Query: &query.UseCase{AssetRateRepo: assetrate.NewMockRepository(ctrl)}}

	app := buildHumaAssetRateApp(t, handler, true)

	req := httptest.NewRequest(http.MethodGet, "/v1/organizations/"+orgID.String()+"/ledgers/"+ledgerID.String()+"/asset-rates/from/USD?limit=abc", nil)
	resp, err := app.Test(req, -1)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	respBody, _ := io.ReadAll(resp.Body)
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode, "bad query stays canonical 400 — no native Huma 422")

	var got map[string]any
	require.NoError(t, json.Unmarshal(respBody, &got), "body: %s", string(respBody))
	assert.Equal(t, constant.ErrInvalidQueryParameter.Error(), got["code"])

	_ = libProblem.BaseURI
}
