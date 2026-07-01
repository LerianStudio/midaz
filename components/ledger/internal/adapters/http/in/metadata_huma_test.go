// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	openapi "github.com/LerianStudio/lib-commons/v5/commons/net/http/openapi"
	libProblem "github.com/LerianStudio/lib-commons/v5/commons/net/http/problem"
	"github.com/gofiber/fiber/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/LerianStudio/midaz/v4/pkg"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mbootstrap"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	pkgHTTP "github.com/LerianStudio/midaz/v4/pkg/net/http"
)

// buildHumaMetadataApp mounts the three metadata-index Huma operations on a /v1
// group, mirroring the production wiring: problem.Install() runs before any
// huma.Register, the Huma API is built with openapi.New over a /v1 group, and an
// auth shim stands in for auth.Authorize("midaz","settings",verb) + tenant
// PostAuthMiddlewares. Metadata carries NO UUID path params, so there is NO
// ParseUUIDPathParameters in the chain (unlike the asset exemplar).
//
// MUST-NOT-PARALLELIZE (same rationale as buildHumaAssetApp): libProblem.Install()
// swaps the process-global huma.NewError hook and Huma validation uses
// process-global sync.Pools; concurrent builds cross-contaminate. Keep sequential.
//
// authOK=false makes the shim reject with the ledger's canonical 401 envelope so
// the auth-preserved contract is testable without a live lib-auth server.
func buildHumaMetadataApp(t *testing.T, handler *MetadataIndexHandler, authOK bool) *fiber.App {
	t.Helper()

	f := fiber.New(fiber.Config{
		DisableStartupMessage: true,
		ErrorHandler:          pkgHTTP.CanonicalFiberErrorHandler,
	})

	libProblem.Install()

	apiV1 := f.Group("/v1")

	// Auth shim: stands in for auth.Authorize("midaz","settings",verb). A rejected
	// request (authOK=false) must never reach Huma — it returns the ledger 401.
	apiV1.Use(func(c *fiber.Ctx) error {
		if !authOK {
			return pkgHTTP.Unauthorized(c, "0001", "Unauthorized", "auth required")
		}

		return c.Next()
	})

	hAPI := openapi.New(f, apiV1, openapi.Config{Title: "ledger-test", Version: "test", Servers: []string{"/v1"}})

	RegisterMetadataIndexRoutes(hAPI, handler)

	return f
}

func newMetadataHandlerWithMocks(ctrl *gomock.Controller) (*MetadataIndexHandler, *mbootstrap.MockMetadataIndexRepository, *mbootstrap.MockMetadataIndexRepository) {
	onboarding := mbootstrap.NewMockMetadataIndexRepository(ctrl)
	transaction := mbootstrap.NewMockMetadataIndexRepository(ctrl)

	// MongoManagers are left nil: with no tenant id on the context, contextForEntity
	// short-circuits and returns the incoming ctx (single-tenant path), matching the
	// legacy Fiber unit tests.
	return &MetadataIndexHandler{
		OnboardingMetadataRepo:  onboarding,
		TransactionMetadataRepo: transaction,
	}, onboarding, transaction
}

func TestHuma_CreateMetadataIndex_Success(t *testing.T) {
	// NOT parallel: buildHumaMetadataApp mutates process-global huma state.
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	handler, _, transaction := newMetadataHandlerWithMocks(ctrl)

	transaction.EXPECT().
		CreateIndex(gomock.Any(), "transaction", gomock.Any()).
		DoAndReturn(func(_ context.Context, collection string, input *mmodel.CreateMetadataIndexInput) (*mmodel.MetadataIndex, error) {
			return &mmodel.MetadataIndex{
				IndexName:   "metadata.tier_1",
				EntityName:  collection,
				MetadataKey: input.MetadataKey,
				Unique:      input.Unique,
				Sparse:      true,
			}, nil
		}).Times(1)

	app := buildHumaMetadataApp(t, handler, true)

	body, _ := json.Marshal(map[string]any{"metadataKey": "tier", "unique": false})
	req := httptest.NewRequest(http.MethodPost, "/v1/settings/metadata-indexes/entities/transaction", bytes.NewReader(body))
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
	// Field-identical to the Fiber http.Created body: entityName reset to the
	// path's entity_name (not the collection), key/index preserved.
	assert.Equal(t, "transaction", got["entityName"])
	assert.Equal(t, "tier", got["metadataKey"])
	assert.Equal(t, "metadata.tier_1", got["indexName"])
}

func TestHuma_CreateMetadataIndex_TenantCaptured(t *testing.T) {
	// NOT parallel: process-global huma state.
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	handler, _, transaction := newMetadataHandlerWithMocks(ctrl)

	// The core threads the request context (populated by the tenant middleware
	// chain) straight into the repo call. Assert that ctx is the one that reached
	// the handler — the fiber UserContext carries a marker we set via a shim MW.
	type ctxKey struct{}

	var captured context.Context

	transaction.EXPECT().
		CreateIndex(gomock.Any(), "transaction", gomock.Any()).
		DoAndReturn(func(ctx context.Context, _ string, _ *mmodel.CreateMetadataIndexInput) (*mmodel.MetadataIndex, error) {
			captured = ctx
			return &mmodel.MetadataIndex{IndexName: "metadata.tier_1", EntityName: "transaction", MetadataKey: "tier"}, nil
		}).Times(1)

	f := fiber.New(fiber.Config{DisableStartupMessage: true, ErrorHandler: pkgHTTP.CanonicalFiberErrorHandler})
	libProblem.Install()
	apiV1 := f.Group("/v1")
	// Shim marks the user context, standing in for the tenant middleware.
	apiV1.Use(func(c *fiber.Ctx) error {
		c.SetUserContext(context.WithValue(c.UserContext(), ctxKey{}, "tenant-marker"))
		return c.Next()
	})
	hAPI := openapi.New(f, apiV1, openapi.Config{Title: "ledger-test", Version: "test", Servers: []string{"/v1"}})
	RegisterMetadataIndexRoutes(hAPI, handler)

	body, _ := json.Marshal(map[string]any{"metadataKey": "tier"})
	req := httptest.NewRequest(http.MethodPost, "/v1/settings/metadata-indexes/entities/transaction", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	resp, err := f.Test(req, -1)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	require.Equal(t, http.StatusCreated, resp.StatusCode)
	require.NotNil(t, captured, "repo must receive the request context")
	assert.Equal(t, "tenant-marker", captured.Value(ctxKey{}), "tenant-populated ctx must reach the repo unchanged")
}

func TestHuma_CreateMetadataIndex_AuthPreserved(t *testing.T) {
	// NOT parallel: process-global huma state.
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	// No repo expectations: a rejected auth must never reach the service.
	handler, _, _ := newMetadataHandlerWithMocks(ctrl)

	app := buildHumaMetadataApp(t, handler, false)

	body, _ := json.Marshal(map[string]any{"metadataKey": "tier"})
	req := httptest.NewRequest(http.MethodPost, "/v1/settings/metadata-indexes/entities/transaction", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req, -1)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode, "auth middleware must reject before Huma; no public route")
}

func TestHuma_CreateMetadataIndex_InvalidEntity_Canonical400(t *testing.T) {
	// NOT parallel: process-global huma state.
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	// Invalid entity -> canonical 400 (ErrInvalidEntityName), service never reached.
	handler, _, _ := newMetadataHandlerWithMocks(ctrl)

	app := buildHumaMetadataApp(t, handler, true)

	body, _ := json.Marshal(map[string]any{"metadataKey": "tier"})
	req := httptest.NewRequest(http.MethodPost, "/v1/settings/metadata-indexes/entities/not_an_entity", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req, -1)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	respBody, _ := io.ReadAll(resp.Body)

	assert.Equal(t, http.StatusBadRequest, resp.StatusCode, "invalid entity stays canonical 400 — no native Huma 422")
	assert.Equal(t, "application/problem+json", resp.Header.Get("Content-Type"))

	var got map[string]any
	require.NoError(t, json.Unmarshal(respBody, &got), "body: %s", string(respBody))
	assert.Equal(t, constant.ErrInvalidEntityName.Error(), got["code"])
	assert.Equal(t, float64(http.StatusBadRequest), got["status"])
}

func TestHuma_CreateMetadataIndex_ValidationError_Canonical400(t *testing.T) {
	// NOT parallel: process-global huma state.
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	// Missing required metadataKey -> imperative ValidateStruct -> canonical 400,
	// service never reached (no native Huma 422).
	handler, _, _ := newMetadataHandlerWithMocks(ctrl)

	app := buildHumaMetadataApp(t, handler, true)

	body, _ := json.Marshal(map[string]any{"unique": true})
	req := httptest.NewRequest(http.MethodPost, "/v1/settings/metadata-indexes/entities/transaction", bytes.NewReader(body))
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

func TestHuma_CreateMetadataIndex_MalformedBody_Canonical400(t *testing.T) {
	// NOT parallel: process-global huma state.
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	handler, _, _ := newMetadataHandlerWithMocks(ctrl)

	app := buildHumaMetadataApp(t, handler, true)

	req := httptest.NewRequest(http.MethodPost, "/v1/settings/metadata-indexes/entities/transaction", bytes.NewReader([]byte("{not valid json")))
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

func TestHuma_GetAllMetadataIndexes_FilteredSuccess(t *testing.T) {
	// NOT parallel: process-global huma state.
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	handler, _, transaction := newMetadataHandlerWithMocks(ctrl)

	transaction.EXPECT().
		FindAllIndexes(gomock.Any(), "transaction").
		Return([]*mmodel.MetadataIndex{{IndexName: "metadata.tier_1", MetadataKey: "tier"}}, nil).Times(1)

	app := buildHumaMetadataApp(t, handler, true)

	req := httptest.NewRequest(http.MethodGet, "/v1/settings/metadata-indexes?entity_name=transaction", nil)
	resp, err := app.Test(req, -1)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	respBody, _ := io.ReadAll(resp.Body)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.NotContains(t, string(respBody), "$schema")

	var got []map[string]any
	require.NoError(t, json.Unmarshal(respBody, &got), "body: %s", string(respBody))
	require.Len(t, got, 1)
	assert.Equal(t, "transaction", got[0]["entityName"], "entity name set on the response")
	assert.Equal(t, "tier", got[0]["metadataKey"])
}

func TestHuma_GetAllMetadataIndexes_InvalidEntity_Canonical400(t *testing.T) {
	// NOT parallel: process-global huma state.
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	handler, _, _ := newMetadataHandlerWithMocks(ctrl)

	app := buildHumaMetadataApp(t, handler, true)

	req := httptest.NewRequest(http.MethodGet, "/v1/settings/metadata-indexes?entity_name=not_an_entity", nil)
	resp, err := app.Test(req, -1)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	respBody, _ := io.ReadAll(resp.Body)
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode, "invalid entity filter stays canonical 400 — no native Huma 422")

	var got map[string]any
	require.NoError(t, json.Unmarshal(respBody, &got), "body: %s", string(respBody))
	assert.Equal(t, constant.ErrInvalidEntityName.Error(), got["code"])
}

func TestHuma_DeleteMetadataIndex_204Empty(t *testing.T) {
	// NOT parallel: process-global huma state.
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	handler, _, transaction := newMetadataHandlerWithMocks(ctrl)

	transaction.EXPECT().
		DeleteIndex(gomock.Any(), "transaction", "metadata.tier_1").
		Return(nil).Times(1)

	app := buildHumaMetadataApp(t, handler, true)

	req := httptest.NewRequest(http.MethodDelete, "/v1/settings/metadata-indexes/entities/transaction/key/tier", nil)
	resp, err := app.Test(req, -1)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	respBody, _ := io.ReadAll(resp.Body)
	assert.Equal(t, http.StatusNoContent, resp.StatusCode)
	assert.Empty(t, respBody, "DELETE 204 must have an empty body")
}

func TestHuma_DeleteMetadataIndex_NotFound_CanonicalMapped(t *testing.T) {
	// NOT parallel: process-global huma state.
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	handler, _, transaction := newMetadataHandlerWithMocks(ctrl)

	// Repo signals a not-found; HumaProblem must project the SAME status the Fiber
	// WithError path would (never a native 422), with the canonical code.
	notFound := pkg.EntityNotFoundError{
		Code:    constant.ErrMetadataIndexNotFound.Error(),
		Title:   "Metadata Index Not Found",
		Message: "metadata index not found",
	}
	transaction.EXPECT().
		DeleteIndex(gomock.Any(), "transaction", "metadata.ghost_1").
		Return(notFound).Times(1)

	app := buildHumaMetadataApp(t, handler, true)

	req := httptest.NewRequest(http.MethodDelete, "/v1/settings/metadata-indexes/entities/transaction/key/ghost", nil)
	resp, err := app.Test(req, -1)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	respBody, _ := io.ReadAll(resp.Body)
	assert.Equal(t, http.StatusNotFound, resp.StatusCode, "not-found maps to 404 — no native Huma 422")
	assert.Equal(t, "application/problem+json", resp.Header.Get("Content-Type"))

	var got map[string]any
	require.NoError(t, json.Unmarshal(respBody, &got), "body: %s", string(respBody))
	assert.Equal(t, constant.ErrMetadataIndexNotFound.Error(), got["code"])
	assert.Equal(t, float64(http.StatusNotFound), got["status"])
}
