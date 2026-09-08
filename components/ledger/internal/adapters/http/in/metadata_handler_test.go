// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	openapi "github.com/LerianStudio/lib-commons/v7/commons/net/http/openapi"
	libProblem "github.com/LerianStudio/lib-commons/v7/commons/net/http/problem"
	tmcore "github.com/LerianStudio/lib-commons/v7/commons/tenant-manager/core"
	"github.com/gofiber/fiber/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	ledgerMiddleware "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/http/in/middleware"
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
		ErrorHandler: pkgHTTP.CanonicalFiberErrorHandler,
	})

	libProblem.Install()

	// Mirror production: the ledger registers ErrorEnvelope on the app root, so
	// /v1 serves the v3 envelope. Without it these assertions lock a shape no
	// deployed ledger returns.
	f.Use(ledgerMiddleware.ErrorEnvelope())

	apiV1 := f.Group("/v1")

	// Auth shim: stands in for auth.Authorize("midaz","settings",verb). A rejected
	// request (authOK=false) must never reach Huma — it returns the ledger 401.
	apiV1.Use(func(c fiber.Ctx) error {
		if !authOK {
			return pkgHTTP.Unauthorized(c, "0001", "Unauthorized", "auth required")
		}

		return c.Next()
	})

	hAPI := openapi.New(f, apiV1, openapi.Config{Title: "ledger-test", Version: "test", Servers: []string{"/v1"}})

	RegisterMetadataIndexRoutes(hAPI, handler, v1OpSuffix)

	return f
}

func newMetadataHandlerWithMocks(ctrl *gomock.Controller) (*MetadataIndexHandler, *mbootstrap.MockMetadataIndexRepository, *mbootstrap.MockMetadataIndexRepository) {
	onboarding := mbootstrap.NewMockMetadataIndexRepository(ctrl)
	transaction := mbootstrap.NewMockMetadataIndexRepository(ctrl)

	// MongoManagers are left nil: with no tenant id on the context, contextForEntity
	// short-circuits and returns the incoming ctx — the single-tenant path.
	return &MetadataIndexHandler{
		OnboardingMetadataRepo:  onboarding,
		TransactionMetadataRepo: transaction,
	}, onboarding, transaction
}

func TestCreateMetadataIndex_Success(t *testing.T) {
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

	resp, err := app.Test(req, fiber.TestConfig{Timeout: 0})
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

func TestCreateMetadataIndex_TenantCaptured(t *testing.T) {
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

	f := fiber.New(fiber.Config{ErrorHandler: pkgHTTP.CanonicalFiberErrorHandler})
	libProblem.Install()
	// Mirror production: the ledger registers ErrorEnvelope on the app root, so
	// /v1 serves the v3 envelope. Without it these assertions lock a shape no
	// deployed ledger returns.
	f.Use(ledgerMiddleware.ErrorEnvelope())

	apiV1 := f.Group("/v1")
	// Shim marks the user context, standing in for the tenant middleware.
	apiV1.Use(func(c fiber.Ctx) error {
		c.SetContext(context.WithValue(c.Context(), ctxKey{}, "tenant-marker"))
		return c.Next()
	})
	hAPI := openapi.New(f, apiV1, openapi.Config{Title: "ledger-test", Version: "test", Servers: []string{"/v1"}})
	RegisterMetadataIndexRoutes(hAPI, handler, v1OpSuffix)

	body, _ := json.Marshal(map[string]any{"metadataKey": "tier"})
	req := httptest.NewRequest(http.MethodPost, "/v1/settings/metadata-indexes/entities/transaction", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	resp, err := f.Test(req, fiber.TestConfig{Timeout: 0})
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	require.Equal(t, http.StatusCreated, resp.StatusCode)
	require.NotNil(t, captured, "repo must receive the request context")
	assert.Equal(t, "tenant-marker", captured.Value(ctxKey{}), "tenant-populated ctx must reach the repo unchanged")
}

func TestCreateMetadataIndex_AuthPreserved(t *testing.T) {
	// NOT parallel: process-global huma state.
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	// No repo expectations: a rejected auth must never reach the service.
	handler, _, _ := newMetadataHandlerWithMocks(ctrl)

	app := buildHumaMetadataApp(t, handler, false)

	body, _ := json.Marshal(map[string]any{"metadataKey": "tier"})
	req := httptest.NewRequest(http.MethodPost, "/v1/settings/metadata-indexes/entities/transaction", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req, fiber.TestConfig{Timeout: 0})
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode, "auth middleware must reject before Huma; no public route")
}

func TestCreateMetadataIndex_InvalidEntity_Canonical400(t *testing.T) {
	// NOT parallel: process-global huma state.
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	// Invalid entity -> canonical 400 (ErrInvalidEntityName), service never reached.
	handler, _, _ := newMetadataHandlerWithMocks(ctrl)

	app := buildHumaMetadataApp(t, handler, true)

	body, _ := json.Marshal(map[string]any{"metadataKey": "tier"})
	req := httptest.NewRequest(http.MethodPost, "/v1/settings/metadata-indexes/entities/not_an_entity", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req, fiber.TestConfig{Timeout: 0})
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	respBody, _ := io.ReadAll(resp.Body)

	assert.Equal(t, http.StatusBadRequest, resp.StatusCode, "invalid entity stays canonical 400 — no native Huma 422")
	assert.Equal(t, "application/json", resp.Header.Get("Content-Type"))

	var got map[string]any
	require.NoError(t, json.Unmarshal(respBody, &got), "body: %s", string(respBody))
	assert.Equal(t, constant.ErrInvalidEntityName.Error(), got["code"])
	assert.NotContains(t, got, "status", "the v1 envelope carries no status member")
}

func TestCreateMetadataIndex_ValidationError_Canonical400(t *testing.T) {
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

func TestCreateMetadataIndex_MalformedBody_Canonical400(t *testing.T) {
	// NOT parallel: process-global huma state.
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	handler, _, _ := newMetadataHandlerWithMocks(ctrl)

	app := buildHumaMetadataApp(t, handler, true)

	req := httptest.NewRequest(http.MethodPost, "/v1/settings/metadata-indexes/entities/transaction", bytes.NewReader([]byte("{not valid json")))
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

func TestGetAllMetadataIndexes_FilteredSuccess(t *testing.T) {
	// NOT parallel: process-global huma state.
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	handler, _, transaction := newMetadataHandlerWithMocks(ctrl)

	transaction.EXPECT().
		FindAllIndexes(gomock.Any(), "transaction").
		Return([]*mmodel.MetadataIndex{{IndexName: "metadata.tier_1", MetadataKey: "tier"}}, nil).Times(1)

	app := buildHumaMetadataApp(t, handler, true)

	req := httptest.NewRequest(http.MethodGet, "/v1/settings/metadata-indexes?entity_name=transaction", nil)
	resp, err := app.Test(req, fiber.TestConfig{Timeout: 0})
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

func TestGetAllMetadataIndexes_InvalidEntity_Canonical400(t *testing.T) {
	// NOT parallel: process-global huma state.
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	handler, _, _ := newMetadataHandlerWithMocks(ctrl)

	app := buildHumaMetadataApp(t, handler, true)

	req := httptest.NewRequest(http.MethodGet, "/v1/settings/metadata-indexes?entity_name=not_an_entity", nil)
	resp, err := app.Test(req, fiber.TestConfig{Timeout: 0})
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	respBody, _ := io.ReadAll(resp.Body)
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode, "invalid entity filter stays canonical 400 — no native Huma 422")

	var got map[string]any
	require.NoError(t, json.Unmarshal(respBody, &got), "body: %s", string(respBody))
	assert.Equal(t, constant.ErrInvalidEntityName.Error(), got["code"])
}

func TestDeleteMetadataIndex_204Empty(t *testing.T) {
	// NOT parallel: process-global huma state.
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	handler, _, transaction := newMetadataHandlerWithMocks(ctrl)

	transaction.EXPECT().
		DeleteIndex(gomock.Any(), "transaction", "metadata.tier_1").
		Return(nil).Times(1)

	app := buildHumaMetadataApp(t, handler, true)

	req := httptest.NewRequest(http.MethodDelete, "/v1/settings/metadata-indexes/entities/transaction/key/tier", nil)
	resp, err := app.Test(req, fiber.TestConfig{Timeout: 0})
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	respBody, _ := io.ReadAll(resp.Body)
	assert.Equal(t, http.StatusNoContent, resp.StatusCode)
	assert.Empty(t, respBody, "DELETE 204 must have an empty body")
}

func TestDeleteMetadataIndex_NotFound_CanonicalMapped(t *testing.T) {
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
	resp, err := app.Test(req, fiber.TestConfig{Timeout: 0})
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	respBody, _ := io.ReadAll(resp.Body)
	assert.Equal(t, http.StatusNotFound, resp.StatusCode, "not-found maps to 404 — no native Huma 422")
	assert.Equal(t, "application/json", resp.Header.Get("Content-Type"))

	var got map[string]any
	require.NoError(t, json.Unmarshal(respBody, &got), "body: %s", string(respBody))
	assert.Equal(t, constant.ErrMetadataIndexNotFound.Error(), got["code"])
	assert.NotContains(t, got, "status", "the v1 envelope carries no status member")
}

// buildHumaMetadataAppWithTenant is buildHumaMetadataApp plus a shim that stamps a
// tenant id onto the request context, standing in for the tenant PostAuthMiddlewares.
// With the handler's mongo managers left nil, the cores' tenant resolution fails and
// the canonical 500 is projected — the multi-tenant misconfiguration contract.
//
// MUST-NOT-PARALLELIZE for the same reason as buildHumaMetadataApp.
func buildHumaMetadataAppWithTenant(t *testing.T, handler *MetadataIndexHandler, tenantID string) *fiber.App {
	t.Helper()

	f := fiber.New(fiber.Config{ErrorHandler: pkgHTTP.CanonicalFiberErrorHandler})

	libProblem.Install()

	// Mirror production: the ledger registers ErrorEnvelope on the app root, so
	// /v1 serves the v3 envelope. Without it these assertions lock a shape no
	// deployed ledger returns.
	f.Use(ledgerMiddleware.ErrorEnvelope())

	apiV1 := f.Group("/v1")
	apiV1.Use(func(c fiber.Ctx) error {
		c.SetContext(tmcore.ContextWithTenantID(c.Context(), tenantID))

		return c.Next()
	})

	hAPI := openapi.New(f, apiV1, openapi.Config{Title: "ledger-test", Version: "test", Servers: []string{"/v1"}})

	RegisterMetadataIndexRoutes(hAPI, handler, v1OpSuffix)

	return f
}

func TestCreateMetadataIndex_OnboardingEntity_Success(t *testing.T) {
	// NOT parallel: process-global huma state.
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	// An onboarding entity must route to the onboarding repo and its collection —
	// the other half of getRepoAndCollection's dispatch.
	handler, onboarding, _ := newMetadataHandlerWithMocks(ctrl)

	onboarding.EXPECT().
		CreateIndex(gomock.Any(), "account", gomock.Any()).
		DoAndReturn(func(_ context.Context, collection string, input *mmodel.CreateMetadataIndexInput) (*mmodel.MetadataIndex, error) {
			return &mmodel.MetadataIndex{
				IndexName:   "metadata.category_1",
				EntityName:  collection,
				MetadataKey: input.MetadataKey,
				Unique:      input.Unique,
				Sparse:      true,
			}, nil
		}).Times(1)

	app := buildHumaMetadataApp(t, handler, true)

	body, _ := json.Marshal(map[string]any{"metadataKey": "category", "unique": true})
	req := httptest.NewRequest(http.MethodPost, "/v1/settings/metadata-indexes/entities/account", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req, fiber.TestConfig{Timeout: 0})
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	respBody, _ := io.ReadAll(resp.Body)
	assert.Equal(t, http.StatusCreated, resp.StatusCode)

	var got map[string]any
	require.NoError(t, json.Unmarshal(respBody, &got), "body: %s", string(respBody))
	assert.Equal(t, "account", got["entityName"])
	assert.Equal(t, "category", got["metadataKey"])
	assert.Equal(t, true, got["unique"])
}

func TestCreateMetadataIndex_NilRepo_Canonical500(t *testing.T) {
	// NOT parallel: process-global huma state.
	// Valid entity, but no repository wired: an infrastructure fault, so a sanitized
	// 500 — never a business 4xx.
	handler := &MetadataIndexHandler{}

	app := buildHumaMetadataApp(t, handler, true)

	body, _ := json.Marshal(map[string]any{"metadataKey": "tier"})
	req := httptest.NewRequest(http.MethodPost, "/v1/settings/metadata-indexes/entities/transaction", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req, fiber.TestConfig{Timeout: 0})
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	respBody, _ := io.ReadAll(resp.Body)
	assert.Equal(t, http.StatusInternalServerError, resp.StatusCode)
	assert.Equal(t, "application/json", resp.Header.Get("Content-Type"))

	var got map[string]any
	require.NoError(t, json.Unmarshal(respBody, &got), "body: %s", string(respBody))
	assert.NotEmpty(t, got["code"])
	assert.NotContains(t, got, "status", "the v1 envelope carries no status member")
	assert.NotContains(t, string(respBody), "repository not configured", "internal detail must not leak")
}

func TestCreateMetadataIndex_RepoFailure_Canonical500(t *testing.T) {
	// NOT parallel: process-global huma state.
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	handler, _, transaction := newMetadataHandlerWithMocks(ctrl)

	transaction.EXPECT().
		CreateIndex(gomock.Any(), "transaction", gomock.Any()).
		Return(nil, errors.New("index already exists")).Times(1)

	app := buildHumaMetadataApp(t, handler, true)

	body, _ := json.Marshal(map[string]any{"metadataKey": "tier"})
	req := httptest.NewRequest(http.MethodPost, "/v1/settings/metadata-indexes/entities/transaction", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req, fiber.TestConfig{Timeout: 0})
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	respBody, _ := io.ReadAll(resp.Body)
	assert.Equal(t, http.StatusInternalServerError, resp.StatusCode)

	var got map[string]any
	require.NoError(t, json.Unmarshal(respBody, &got), "body: %s", string(respBody))
	assert.NotContains(t, got, "status", "the v1 envelope carries no status member")
	assert.NotContains(t, string(respBody), "index already exists", "repo detail must not leak")
}

func TestCreateMetadataIndex_MetadataKeyTooLong_Canonical400(t *testing.T) {
	// NOT parallel: process-global huma state.
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	// Metadata is flat with a key max of 100 chars; the imperative validator rejects
	// an over-long key at 400 before the service is reached.
	handler, _, _ := newMetadataHandlerWithMocks(ctrl)

	app := buildHumaMetadataApp(t, handler, true)

	body, _ := json.Marshal(map[string]any{"metadataKey": strings.Repeat("k", 101)})
	req := httptest.NewRequest(http.MethodPost, "/v1/settings/metadata-indexes/entities/transaction", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req, fiber.TestConfig{Timeout: 0})
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	respBody, _ := io.ReadAll(resp.Body)
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode, "over-long key stays canonical 400 — no native Huma 422")

	var got map[string]any
	require.NoError(t, json.Unmarshal(respBody, &got), "body: %s", string(respBody))
	assert.NotEmpty(t, got["code"])
	assert.NotContains(t, got, "status", "the v1 envelope carries no status member")
}

func TestCreateMetadataIndex_EmptyEntityName_Canonical400(t *testing.T) {
	t.Parallel()

	// No route can serve an empty entity_name, so the guard is exercised at the
	// terminal: an empty path param yields the canonical path-parameter 400.
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	handler, _, _ := newMetadataHandlerWithMocks(ctrl)

	out, err := handler.CreateMetadataIndex(context.Background(), &CreateMetadataIndexRequest{
		EntityName: "",
		RawBody:    []byte(`{"metadataKey":"tier"}`),
	})

	assert.Nil(t, out)
	assertMetadataProblem(t, err, http.StatusBadRequest, constant.ErrInvalidPathParameter.Error())
}

func TestGetAllMetadataIndexes_NoFilter_AggregatesEveryEntity(t *testing.T) {
	// NOT parallel: process-global huma state.
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	handler, onboarding, transaction := newMetadataHandlerWithMocks(ctrl)

	// Unfiltered: both repos are swept once per collection, so AnyTimes.
	onboarding.EXPECT().
		FindAllIndexes(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, collection string) ([]*mmodel.MetadataIndex, error) {
			if collection == "account" {
				return []*mmodel.MetadataIndex{{IndexName: "metadata.category_1", MetadataKey: "category"}}, nil
			}

			return []*mmodel.MetadataIndex{}, nil
		}).AnyTimes()

	transaction.EXPECT().
		FindAllIndexes(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, collection string) ([]*mmodel.MetadataIndex, error) {
			if collection == "transaction" {
				return []*mmodel.MetadataIndex{{IndexName: "metadata.tier_1", MetadataKey: "tier"}}, nil
			}

			return []*mmodel.MetadataIndex{}, nil
		}).AnyTimes()

	app := buildHumaMetadataApp(t, handler, true)

	req := httptest.NewRequest(http.MethodGet, "/v1/settings/metadata-indexes", nil)
	resp, err := app.Test(req, fiber.TestConfig{Timeout: 0})
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	respBody, _ := io.ReadAll(resp.Body)
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var got []map[string]any
	require.NoError(t, json.Unmarshal(respBody, &got), "body: %s", string(respBody))
	require.GreaterOrEqual(t, len(got), 2)

	var foundAccount, foundTransaction bool

	for _, idx := range got {
		assert.NotEmpty(t, idx["entityName"], "entity name set on every aggregated index")

		if idx["entityName"] == "account" && idx["metadataKey"] == "category" {
			foundAccount = true
		}

		if idx["entityName"] == "transaction" && idx["metadataKey"] == "tier" {
			foundTransaction = true
		}
	}

	assert.True(t, foundAccount, "onboarding index present")
	assert.True(t, foundTransaction, "transaction index present")
}

func TestGetAllMetadataIndexes_NoFilter_PartialRepoFailureStill200(t *testing.T) {
	// NOT parallel: process-global huma state.
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	handler, onboarding, transaction := newMetadataHandlerWithMocks(ctrl)

	// A per-collection failure is logged and skipped, not propagated: the sweep
	// still returns the collections that answered.
	onboarding.EXPECT().
		FindAllIndexes(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, collection string) ([]*mmodel.MetadataIndex, error) {
			switch collection {
			case "organization":
				return nil, errors.New("connection refused")
			case "ledger":
				return []*mmodel.MetadataIndex{{IndexName: "metadata.env_1", MetadataKey: "env"}}, nil
			default:
				return []*mmodel.MetadataIndex{}, nil
			}
		}).AnyTimes()

	transaction.EXPECT().
		FindAllIndexes(gomock.Any(), gomock.Any()).
		Return(nil, errors.New("connection refused")).AnyTimes()

	app := buildHumaMetadataApp(t, handler, true)

	req := httptest.NewRequest(http.MethodGet, "/v1/settings/metadata-indexes", nil)
	resp, err := app.Test(req, fiber.TestConfig{Timeout: 0})
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	respBody, _ := io.ReadAll(resp.Body)
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var got []map[string]any
	require.NoError(t, json.Unmarshal(respBody, &got), "body: %s", string(respBody))

	var foundLedger bool

	for _, idx := range got {
		if idx["entityName"] == "ledger" && idx["metadataKey"] == "env" {
			foundLedger = true
		}
	}

	assert.True(t, foundLedger, "surviving collection still reported despite sibling failures")
}

func TestGetAllMetadataIndexes_NilRepoForFilteredEntity_Canonical500(t *testing.T) {
	// NOT parallel: process-global huma state.
	handler := &MetadataIndexHandler{}

	app := buildHumaMetadataApp(t, handler, true)

	req := httptest.NewRequest(http.MethodGet, "/v1/settings/metadata-indexes?entity_name=transaction", nil)
	resp, err := app.Test(req, fiber.TestConfig{Timeout: 0})
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	respBody, _ := io.ReadAll(resp.Body)
	assert.Equal(t, http.StatusInternalServerError, resp.StatusCode)
	assert.Equal(t, "application/json", resp.Header.Get("Content-Type"))

	var got map[string]any
	require.NoError(t, json.Unmarshal(respBody, &got), "body: %s", string(respBody))
	assert.NotContains(t, got, "status", "the v1 envelope carries no status member")
}

func TestGetAllMetadataIndexes_FilteredRepoFailure_Canonical500(t *testing.T) {
	// NOT parallel: process-global huma state.
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	handler, _, transaction := newMetadataHandlerWithMocks(ctrl)

	// Filtered reads propagate the repo failure (unlike the unfiltered sweep).
	transaction.EXPECT().
		FindAllIndexes(gomock.Any(), "operation").
		Return(nil, errors.New("database error")).Times(1)

	app := buildHumaMetadataApp(t, handler, true)

	req := httptest.NewRequest(http.MethodGet, "/v1/settings/metadata-indexes?entity_name=operation", nil)
	resp, err := app.Test(req, fiber.TestConfig{Timeout: 0})
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	respBody, _ := io.ReadAll(resp.Body)
	assert.Equal(t, http.StatusInternalServerError, resp.StatusCode)
	assert.NotContains(t, string(respBody), "database error", "repo detail must not leak")
}

func TestGetAllMetadataIndexes_InvalidQueryParameter_Canonical400(t *testing.T) {
	// NOT parallel: process-global huma state.
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	handler, _, _ := newMetadataHandlerWithMocks(ctrl)

	app := buildHumaMetadataApp(t, handler, true)

	req := httptest.NewRequest(http.MethodGet, "/v1/settings/metadata-indexes?limit=not-a-number", nil)
	resp, err := app.Test(req, fiber.TestConfig{Timeout: 0})
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	respBody, _ := io.ReadAll(resp.Body)
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode, "query binding failure stays canonical 400")

	var got map[string]any
	require.NoError(t, json.Unmarshal(respBody, &got), "body: %s", string(respBody))
	assert.NotEmpty(t, got["code"])
}

func TestDeleteMetadataIndex_OnboardingEntity_204(t *testing.T) {
	// NOT parallel: process-global huma state.
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	handler, onboarding, _ := newMetadataHandlerWithMocks(ctrl)

	onboarding.EXPECT().
		DeleteIndex(gomock.Any(), "account", "metadata.category_1").
		Return(nil).Times(1)

	app := buildHumaMetadataApp(t, handler, true)

	req := httptest.NewRequest(http.MethodDelete, "/v1/settings/metadata-indexes/entities/account/key/category", nil)
	resp, err := app.Test(req, fiber.TestConfig{Timeout: 0})
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	respBody, _ := io.ReadAll(resp.Body)
	assert.Equal(t, http.StatusNoContent, resp.StatusCode)
	assert.Empty(t, respBody)
}

func TestDeleteMetadataIndex_InvalidEntity_Canonical400(t *testing.T) {
	// NOT parallel: process-global huma state.
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	handler, _, _ := newMetadataHandlerWithMocks(ctrl)

	app := buildHumaMetadataApp(t, handler, true)

	req := httptest.NewRequest(http.MethodDelete, "/v1/settings/metadata-indexes/entities/not_an_entity/key/tier", nil)
	resp, err := app.Test(req, fiber.TestConfig{Timeout: 0})
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	respBody, _ := io.ReadAll(resp.Body)
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode, "invalid entity stays canonical 400 — no native Huma 422")

	var got map[string]any
	require.NoError(t, json.Unmarshal(respBody, &got), "body: %s", string(respBody))
	assert.Equal(t, constant.ErrInvalidEntityName.Error(), got["code"])
}

func TestDeleteMetadataIndex_NilRepo_Canonical500(t *testing.T) {
	// NOT parallel: process-global huma state.
	handler := &MetadataIndexHandler{}

	app := buildHumaMetadataApp(t, handler, true)

	req := httptest.NewRequest(http.MethodDelete, "/v1/settings/metadata-indexes/entities/transaction/key/tier", nil)
	resp, err := app.Test(req, fiber.TestConfig{Timeout: 0})
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	respBody, _ := io.ReadAll(resp.Body)
	assert.Equal(t, http.StatusInternalServerError, resp.StatusCode)

	var got map[string]any
	require.NoError(t, json.Unmarshal(respBody, &got), "body: %s", string(respBody))
	assert.NotContains(t, got, "status", "the v1 envelope carries no status member")
}

func TestDeleteMetadataIndex_RepoFailure_Canonical500(t *testing.T) {
	// NOT parallel: process-global huma state.
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	handler, _, transaction := newMetadataHandlerWithMocks(ctrl)

	transaction.EXPECT().
		DeleteIndex(gomock.Any(), "transaction", "metadata.tier_1").
		Return(errors.New("index not found")).Times(1)

	app := buildHumaMetadataApp(t, handler, true)

	req := httptest.NewRequest(http.MethodDelete, "/v1/settings/metadata-indexes/entities/transaction/key/tier", nil)
	resp, err := app.Test(req, fiber.TestConfig{Timeout: 0})
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	respBody, _ := io.ReadAll(resp.Body)
	assert.Equal(t, http.StatusInternalServerError, resp.StatusCode, "an unmapped repo error is a 500, not a 404")
	assert.NotContains(t, string(respBody), "index not found", "repo detail must not leak")
}

func TestDeleteMetadataIndex_EmptyPathParams_Canonical400(t *testing.T) {
	t.Parallel()

	// No route can serve an empty entity_name or index_key, so both guards are
	// exercised at the terminal.
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	handler, _, _ := newMetadataHandlerWithMocks(ctrl)

	out, err := handler.DeleteMetadataIndex(context.Background(), &DeleteMetadataIndexRequest{
		EntityName: "",
		IndexKey:   "tier",
	})
	assert.Nil(t, out)
	assertMetadataProblem(t, err, http.StatusBadRequest, constant.ErrInvalidPathParameter.Error())

	out, err = handler.DeleteMetadataIndex(context.Background(), &DeleteMetadataIndexRequest{
		EntityName: "transaction",
		IndexKey:   "",
	})
	assert.Nil(t, out)
	assertMetadataProblem(t, err, http.StatusBadRequest, constant.ErrInvalidPathParameter.Error())
}

// TestHuma_MetadataIndex_MultiTenantContextResolutionErrors covers the misconfigured
// multi-tenant deployment: the request carries a tenant id but no mongo manager is
// wired, so every operation fails tenant resolution with a sanitized 500 rather than
// silently serving the single-tenant database.
func TestMetadataIndex_MultiTenantContextResolutionErrors(t *testing.T) {
	// NOT parallel: process-global huma state.
	tests := []struct {
		name   string
		method string
		url    string
		body   []byte
	}{
		{
			name:   "create fails when tenant mongo manager is missing",
			method: http.MethodPost,
			url:    "/v1/settings/metadata-indexes/entities/transaction",
			body:   []byte(`{"metadataKey":"tier"}`),
		},
		{
			name:   "filtered list fails when tenant mongo manager is missing",
			method: http.MethodGet,
			url:    "/v1/settings/metadata-indexes?entity_name=transaction",
		},
		{
			name:   "unfiltered list fails when tenant mongo manager is missing",
			method: http.MethodGet,
			url:    "/v1/settings/metadata-indexes",
		},
		{
			name:   "delete fails when tenant mongo manager is missing",
			method: http.MethodDelete,
			url:    "/v1/settings/metadata-indexes/entities/transaction/key/tier",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			t.Cleanup(ctrl.Finish)

			// Repos are wired but the mongo managers are not: tenant resolution is
			// the failure point, so no repo call is ever expected.
			handler, _, _ := newMetadataHandlerWithMocks(ctrl)

			app := buildHumaMetadataAppWithTenant(t, handler, "tenant-1")

			req := httptest.NewRequest(tt.method, tt.url, bytes.NewReader(tt.body))
			req.Header.Set("Content-Type", "application/json")

			resp, err := app.Test(req, fiber.TestConfig{Timeout: 0})
			require.NoError(t, err)
			defer func() { _ = resp.Body.Close() }()

			respBody, _ := io.ReadAll(resp.Body)
			assert.Equal(t, http.StatusInternalServerError, resp.StatusCode)
			assert.Equal(t, "application/json", resp.Header.Get("Content-Type"))

			var got map[string]any
			require.NoError(t, json.Unmarshal(respBody, &got), "body: %s", string(respBody))
			assert.NotEmpty(t, got["code"])
			assert.NotContains(t, got, "status", "the v1 envelope carries no status member")
			assert.NotContains(t, string(respBody), "mongo manager", "internal detail must not leak")
		})
	}
}

// assertMetadataProblem asserts a terminal returned the canonical problem envelope
// with the given HTTP status and business code.
func assertMetadataProblem(t *testing.T, err error, wantStatus int, wantCode string) {
	t.Helper()

	require.Error(t, err)

	var detail *pkgHTTP.Detail
	require.ErrorAs(t, err, &detail, "terminal must return the canonical problem detail")
	assert.Equal(t, wantStatus, detail.Status)
	assert.Equal(t, wantCode, detail.Code)
}
