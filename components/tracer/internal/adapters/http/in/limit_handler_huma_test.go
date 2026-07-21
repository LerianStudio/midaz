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
	tmctx "github.com/LerianStudio/lib-commons/v5/commons/tenant-manager/core"
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/components/tracer/internal/services/command"
	"github.com/LerianStudio/midaz/v4/components/tracer/internal/testutil"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/model"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	pkgHTTP "github.com/LerianStudio/midaz/v4/pkg/net/http"
)

// tenantSpyLimitService is a LimitService stub that records the tenant ID it
// sees on its incoming context — the ctx-threading probe (see the rule test's
// tenantSpyService for the full chain rationale). A non-empty capturedTenant
// proves the tenant the middleware put on c.UserContext() reached the service
// through the Huma handler ctx with no bridge.
type tenantSpyLimitService struct {
	capturedTenant string

	createResult *model.Limit
	createErr    error
	getResult    *model.Limit
	getErr       error
	updateResult *model.Limit
	updateErr    error
	listResult   *model.ListLimitsResult
	listErr      error
	lifecycle    *model.Limit // returned by activate/deactivate/draft
	lifecycleErr error
	deleteErr    error
	usageResult  *model.UsageSnapshot
	usageErr     error
	// listFilter captures the filter the core built from the query, so tests can
	// assert imperative binding/defaults produced the same filter the Fiber path
	// would.
	listFilter *model.ListLimitsFilter
}

func (s *tenantSpyLimitService) CreateLimit(ctx context.Context, _ *command.CreateLimitInput) (*model.Limit, error) {
	s.capturedTenant = tmctx.GetTenantIDContext(ctx)
	return s.createResult, s.createErr
}

func (s *tenantSpyLimitService) GetLimit(ctx context.Context, _ uuid.UUID) (*model.Limit, error) {
	s.capturedTenant = tmctx.GetTenantIDContext(ctx)
	return s.getResult, s.getErr
}

func (s *tenantSpyLimitService) ListLimits(ctx context.Context, filter *model.ListLimitsFilter) (*model.ListLimitsResult, error) {
	s.capturedTenant = tmctx.GetTenantIDContext(ctx)
	s.listFilter = filter
	return s.listResult, s.listErr
}

func (s *tenantSpyLimitService) UpdateLimit(ctx context.Context, _ uuid.UUID, _ *command.UpdateLimitInput) (*model.Limit, error) {
	s.capturedTenant = tmctx.GetTenantIDContext(ctx)
	return s.updateResult, s.updateErr
}

func (s *tenantSpyLimitService) ActivateLimit(ctx context.Context, _ uuid.UUID) (*model.Limit, error) {
	s.capturedTenant = tmctx.GetTenantIDContext(ctx)
	return s.lifecycle, s.lifecycleErr
}

func (s *tenantSpyLimitService) DeactivateLimit(ctx context.Context, _ uuid.UUID) (*model.Limit, error) {
	s.capturedTenant = tmctx.GetTenantIDContext(ctx)
	return s.lifecycle, s.lifecycleErr
}

func (s *tenantSpyLimitService) DraftLimit(ctx context.Context, _ uuid.UUID) (*model.Limit, error) {
	s.capturedTenant = tmctx.GetTenantIDContext(ctx)
	return s.lifecycle, s.lifecycleErr
}

func (s *tenantSpyLimitService) DeleteLimit(ctx context.Context, _ uuid.UUID) error {
	s.capturedTenant = tmctx.GetTenantIDContext(ctx)
	return s.deleteErr
}

func (s *tenantSpyLimitService) GetLimitUsage(ctx context.Context, _ uuid.UUID) (*model.UsageSnapshot, error) {
	s.capturedTenant = tmctx.GetTenantIDContext(ctx)
	return s.usageResult, s.usageErr
}

// buildHumaLimitApp mirrors buildHumaRuleApp for the limit ops: problem.Install
// before any Register, the Huma API built with openapi.New over the SAME /v1
// group that carries the tenant middleware, and RegisterLimitRoutes registering
// all nine limit ops.
//
// MUST-NOT-PARALLELIZE (same reason as buildHumaRuleApp): libProblem.Install()
// swaps the process-global huma.NewError hook and Huma validation uses
// process-global sync.Pools, so tests that build a huma.API through this
// function CANNOT call t.Parallel().
func buildHumaLimitApp(t *testing.T, svc LimitService, tenantID string) *fiber.App {
	t.Helper()

	f := fiber.New(fiber.Config{
		DisableStartupMessage: true,
		ErrorHandler:          pkgHTTP.CanonicalFiberErrorHandler,
	})

	libProblem.Install()

	api := f.Group("/v1")
	api.Use(func(c *fiber.Ctx) error {
		if tenantID != "" {
			c.SetUserContext(tmctx.ContextWithTenantID(c.UserContext(), tenantID))
		}
		return c.Next()
	})

	hAPI := openapi.New(f, api, openapi.Config{Title: "tracer-test", Version: "test", Servers: []string{"/v1"}})

	h := NewLimitHandler(svc)
	RegisterLimitRoutes(hAPI, h)

	return f
}

func validLimit(id uuid.UUID) *model.Limit {
	return &model.Limit{
		ID:        id,
		Name:      "Daily Cap",
		LimitType: model.LimitTypeDaily,
		MaxAmount: decimal.RequireFromString("1000.00"),
		Currency:  "USD",
		Status:    model.LimitStatusDraft,
		CreatedAt: testutil.FixedTime(),
		UpdatedAt: testutil.FixedTime(),
	}
}

func validCreateLimitBody() []byte {
	body, _ := json.Marshal(map[string]any{
		"name":      "Daily Cap",
		"limitType": "DAILY",
		"maxAmount": "1000.00",
		"currency":  "USD",
		"scopes":    []map[string]any{{"accountId": testutil.MustDeterministicUUID(99).String()}},
	})
	return body
}

func TestHuma_CreateLimit_Success(t *testing.T) {
	id := testutil.MustDeterministicUUID(1)
	svc := &tenantSpyLimitService{createResult: validLimit(id)}
	app := buildHumaLimitApp(t, svc, "tenant-alpha")

	req := httptest.NewRequest(http.MethodPost, "/v1/limits", bytes.NewReader(validCreateLimitBody()))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req, -1)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	respBody, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	assert.Equal(t, http.StatusCreated, resp.StatusCode, "CreateLimit must return 201 through Huma")
	assert.NotContains(t, string(respBody), "$schema", "SchemaLinkTransformer must be zeroed — no $schema in body")
	assert.NotContains(t, string(respBody), "$ref")

	var got map[string]any
	require.NoError(t, json.Unmarshal(respBody, &got), "body must be JSON: %s", string(respBody))
	assert.Equal(t, "Daily Cap", got["name"])
	assert.Equal(t, "DAILY", got["limitType"])
	assert.Equal(t, "DRAFT", got["status"])
	assert.Equal(t, id.String(), got["limitId"], "limitId is model.Limit json tag for ID — body is model.Limit verbatim")

	assert.Equal(t, "tenant-alpha", svc.capturedTenant,
		"tenant from c.UserContext() must reach the service via the Huma handler ctx")
}

func TestHuma_CreateLimit_ValidationError(t *testing.T) {
	svc := &tenantSpyLimitService{}
	app := buildHumaLimitApp(t, svc, "tenant-gamma")

	// Missing required "name" -> imperative Validate() -> canonical 400, not native 422.
	body, _ := json.Marshal(map[string]any{
		"limitType": "DAILY",
		"maxAmount": "1000.00",
		"currency":  "USD",
		"scopes":    []map[string]any{{"accountId": testutil.MustDeterministicUUID(99).String()}},
	})

	req := httptest.NewRequest(http.MethodPost, "/v1/limits", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req, -1)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	respBody, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	assert.Equal(t, http.StatusBadRequest, resp.StatusCode, "imperative validation stays 400 — no new native Huma 422")
	assert.Equal(t, "application/problem+json", resp.Header.Get("Content-Type"))

	var got map[string]any
	require.NoError(t, json.Unmarshal(respBody, &got), "body must be JSON: %s", string(respBody))
	assert.Equal(t, float64(http.StatusBadRequest), got["status"], "RFC 9457 status field")
	assert.NotEmpty(t, got["code"], "canonical registry code present")
	assert.Empty(t, svc.capturedTenant, "service must not be reached on a validation error")
}

func TestHuma_CreateLimit_MalformedJSON(t *testing.T) {
	svc := &tenantSpyLimitService{}
	app := buildHumaLimitApp(t, svc, "tenant-zeta")

	req := httptest.NewRequest(http.MethodPost, "/v1/limits", bytes.NewReader([]byte("{not json")))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req, -1)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	respBody, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	assert.Equal(t, http.StatusBadRequest, resp.StatusCode, "malformed JSON must be the canonical 400 — no native Huma body validation")
	assert.Equal(t, "application/problem+json", resp.Header.Get("Content-Type"))

	var got map[string]any
	require.NoError(t, json.Unmarshal(respBody, &got), "body must be JSON: %s", string(respBody))
	assert.Equal(t, "0094", got["code"], "code must be ErrInvalidRequestBody, identical to the Fiber path")
	assert.Empty(t, svc.capturedTenant, "service must not be reached on malformed JSON")
}

func TestHuma_GetLimit_Success(t *testing.T) {
	id := testutil.MustDeterministicUUID(7)
	limit := validLimit(id)
	limit.Status = model.LimitStatusActive
	svc := &tenantSpyLimitService{getResult: limit}
	app := buildHumaLimitApp(t, svc, "tenant-beta")

	req := httptest.NewRequest(http.MethodGet, "/v1/limits/"+id.String(), nil)
	resp, err := app.Test(req, -1)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	respBody, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	assert.Equal(t, http.StatusOK, resp.StatusCode, "GetLimit must return 200 through Huma")
	assert.NotContains(t, string(respBody), "$schema")

	var got map[string]any
	require.NoError(t, json.Unmarshal(respBody, &got), "body must be JSON: %s", string(respBody))
	assert.Equal(t, id.String(), got["limitId"])
	assert.Equal(t, "tenant-beta", svc.capturedTenant)
}

func TestHuma_GetLimit_BadUUID(t *testing.T) {
	svc := &tenantSpyLimitService{}
	app := buildHumaLimitApp(t, svc, "tenant-epsilon")

	req := httptest.NewRequest(http.MethodGet, "/v1/limits/not-a-uuid", nil)
	resp, err := app.Test(req, -1)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	respBody, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	assert.Equal(t, http.StatusBadRequest, resp.StatusCode, "malformed path UUID must be the canonical 400 — no native Huma 422")
	assert.Equal(t, "application/problem+json", resp.Header.Get("Content-Type"))

	var got map[string]any
	require.NoError(t, json.Unmarshal(respBody, &got), "body must be JSON: %s", string(respBody))
	assert.Equal(t, "0065", got["code"], "code must be ErrInvalidPathParameter, identical to the Fiber path")
	assert.Equal(t, "Limit", got["entityType"], "canonical entityType from ErrInvalidPathParameter")
	assert.Empty(t, svc.capturedTenant, "service must not be reached on a bad path param")
}

func TestHuma_UpdateLimit_Success(t *testing.T) {
	id := testutil.MustDeterministicUUID(11)
	limit := validLimit(id)
	limit.Name = "Updated Cap"
	svc := &tenantSpyLimitService{updateResult: limit}
	app := buildHumaLimitApp(t, svc, "tenant-alpha")

	body, _ := json.Marshal(map[string]any{"name": "Updated Cap"})
	req := httptest.NewRequest(http.MethodPatch, "/v1/limits/"+id.String(), bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req, -1)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	respBody, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	assert.Equal(t, http.StatusOK, resp.StatusCode, "UpdateLimit must return 200 through Huma")
	assert.NotContains(t, string(respBody), "$schema")

	var got map[string]any
	require.NoError(t, json.Unmarshal(respBody, &got), "body must be JSON: %s", string(respBody))
	assert.Equal(t, "Updated Cap", got["name"])
	assert.Equal(t, id.String(), got["limitId"])
	assert.Equal(t, "tenant-alpha", svc.capturedTenant)
}

func TestHuma_UpdateLimit_BadUUID(t *testing.T) {
	svc := &tenantSpyLimitService{}
	app := buildHumaLimitApp(t, svc, "tenant-gamma")

	body, _ := json.Marshal(map[string]any{"name": "x"})
	req := httptest.NewRequest(http.MethodPatch, "/v1/limits/not-a-uuid", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req, -1)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	respBody, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
	var got map[string]any
	require.NoError(t, json.Unmarshal(respBody, &got), "body must be JSON: %s", string(respBody))
	assert.Equal(t, "0065", got["code"])
	assert.Empty(t, svc.capturedTenant, "service must not be reached on a bad path param")
}

func TestHuma_UpdateLimit_EmptyBody(t *testing.T) {
	svc := &tenantSpyLimitService{}
	app := buildHumaLimitApp(t, svc, "tenant-gamma")

	id := testutil.MustDeterministicUUID(12)
	req := httptest.NewRequest(http.MethodPatch, "/v1/limits/"+id.String(), bytes.NewReader([]byte("{}")))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req, -1)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	respBody, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	assert.Equal(t, http.StatusBadRequest, resp.StatusCode, "empty update must be the canonical 400 — no native Huma 422")
	var got map[string]any
	require.NoError(t, json.Unmarshal(respBody, &got), "body must be JSON: %s", string(respBody))
	assert.Equal(t, constant.ErrNothingToUpdate.Error(), got["code"])
	assert.Empty(t, svc.capturedTenant, "service must not be reached when there is nothing to update")
}

func TestHuma_UpdateLimit_MalformedJSON(t *testing.T) {
	svc := &tenantSpyLimitService{}
	app := buildHumaLimitApp(t, svc, "tenant-zeta")

	id := testutil.MustDeterministicUUID(13)
	req := httptest.NewRequest(http.MethodPatch, "/v1/limits/"+id.String(), bytes.NewReader([]byte("{not json")))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req, -1)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	respBody, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
	var got map[string]any
	require.NoError(t, json.Unmarshal(respBody, &got), "body must be JSON: %s", string(respBody))
	assert.Equal(t, "0094", got["code"], "code must be ErrInvalidRequestBody, identical to the Fiber path")
	assert.Empty(t, svc.capturedTenant, "service must not be reached on malformed JSON")
}

// TestHuma_UpdateLimit_ImmutableField pins the raw-body map-probe: a body
// carrying limitType or currency must be rejected with ErrLimitImmutableField
// (0380) BEFORE BodyParser, identical to the Fiber path. The probe reads the
// RawBody the shell passes; the service must never be reached.
func TestHuma_UpdateLimit_ImmutableField(t *testing.T) {
	id := testutil.MustDeterministicUUID(14)

	for _, tc := range []struct {
		name string
		body map[string]any
	}{
		{"limitType present", map[string]any{"limitType": "MONTHLY"}},
		{"currency present", map[string]any{"currency": "EUR"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			svc := &tenantSpyLimitService{}
			app := buildHumaLimitApp(t, svc, "tenant-gamma")

			body, _ := json.Marshal(tc.body)
			req := httptest.NewRequest(http.MethodPatch, "/v1/limits/"+id.String(), bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")

			resp, err := app.Test(req, -1)
			require.NoError(t, err)
			defer func() { _ = resp.Body.Close() }()

			respBody, err := io.ReadAll(resp.Body)
			require.NoError(t, err)

			var got map[string]any
			require.NoError(t, json.Unmarshal(respBody, &got), "body must be JSON: %s", string(respBody))
			assert.Equal(t, "0380", got["code"], "immutable field must yield ErrLimitImmutableField, identical to Fiber")
			assert.Empty(t, svc.capturedTenant, "service must not be reached on an immutable-field request")
		})
	}
}

func TestHuma_ListLimits_Success(t *testing.T) {
	limit := validLimit(testutil.MustDeterministicUUID(20))
	limit.Status = model.LimitStatusActive
	svc := &tenantSpyLimitService{listResult: &model.ListLimitsResult{
		Limits:     []model.Limit{*limit},
		NextCursor: "next123",
		HasMore:    true,
	}}
	app := buildHumaLimitApp(t, svc, "tenant-alpha")

	req := httptest.NewRequest(http.MethodGet, "/v1/limits?limit=25&status=ACTIVE&sort_by=name&sort_order=asc", nil)
	resp, err := app.Test(req, -1)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	respBody, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	assert.Equal(t, http.StatusOK, resp.StatusCode, "ListLimits must return 200 through Huma")
	assert.NotContains(t, string(respBody), "$schema")

	var got ListLimitsResponse
	require.NoError(t, json.Unmarshal(respBody, &got), "body must be ListLimitsResponse: %s", string(respBody))
	require.Len(t, got.Limits, 1)
	assert.Equal(t, "next123", got.NextCursor)
	assert.True(t, got.HasMore)

	assert.Equal(t, "tenant-alpha", svc.capturedTenant)
	require.NotNil(t, svc.listFilter)
	assert.Equal(t, 25, svc.listFilter.Limit)
	require.NotNil(t, svc.listFilter.Status)
	assert.Equal(t, model.LimitStatusActive, *svc.listFilter.Status)
	assert.Equal(t, "name", svc.listFilter.SortBy)
	assert.Equal(t, "ASC", svc.listFilter.SortOrder, "sort_order normalized to uppercase by SetDefaults")
}

func TestHuma_ListLimits_Defaults(t *testing.T) {
	svc := &tenantSpyLimitService{listResult: &model.ListLimitsResult{Limits: nil}}
	app := buildHumaLimitApp(t, svc, "tenant-alpha")

	req := httptest.NewRequest(http.MethodGet, "/v1/limits", nil)
	resp, err := app.Test(req, -1)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	respBody, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var got map[string]any
	require.NoError(t, json.Unmarshal(respBody, &got), "body JSON: %s", string(respBody))
	assert.NotNil(t, got["limits"], "limits must be [] not null")

	require.NotNil(t, svc.listFilter)
	assert.Equal(t, "created_at", svc.listFilter.SortBy, "default sort field applied by SetDefaults")
	assert.Equal(t, "DESC", svc.listFilter.SortOrder, "default sort order applied by SetDefaults")
}

// TestHuma_ListLimits_InvalidQuery pins the query-param contract: invalid values
// must reach the core's imperative Validate() and produce the canonical 400 —
// NOT a native Huma 422. Query fields carry NO validation tags.
func TestHuma_ListLimits_InvalidQuery(t *testing.T) {
	for _, tc := range []struct {
		name, query, code string
	}{
		{"limit out of range", "limit=101", "0080"},          // ErrPaginationLimitExceeded
		{"limit non-numeric", "limit=abc", "0082"},           // ErrInvalidQueryParameter (bind failure)
		{"invalid status", "status=INVALID", "0082"},         // ErrInvalidQueryParameter
		{"invalid sort_by", "sort_by=priority", "0332"},      // ErrInvalidSortColumn
		{"invalid sort_order", "sort_order=RANDOM", "0081"},  // ErrInvalidSortOrder
		{"invalid limit_type", "limit_type=INVALID", "0082"}, // ErrInvalidQueryParameter
	} {
		t.Run(tc.name, func(t *testing.T) {
			svc := &tenantSpyLimitService{}
			app := buildHumaLimitApp(t, svc, "tenant-gamma")

			req := httptest.NewRequest(http.MethodGet, "/v1/limits?"+tc.query, nil)
			resp, err := app.Test(req, -1)
			require.NoError(t, err)
			defer func() { _ = resp.Body.Close() }()

			respBody, err := io.ReadAll(resp.Body)
			require.NoError(t, err)

			assert.Equal(t, http.StatusBadRequest, resp.StatusCode, "invalid query must be the canonical 400 — no native Huma 422")
			assert.Equal(t, "application/problem+json", resp.Header.Get("Content-Type"))

			var got map[string]any
			require.NoError(t, json.Unmarshal(respBody, &got), "body JSON: %s", string(respBody))
			assert.Equal(t, tc.code, got["code"], "canonical code identical to the Fiber path for %q", tc.query)
			assert.Empty(t, svc.capturedTenant, "service must not be reached on a bad query param")
		})
	}
}

// TestHuma_ListLimits_PresentButEmptyQueryParity anchors present-but-empty
// parity to the REAL ListLimitsInput field types (which differ from the rule
// path): only Limit is a pointer (*int), so ?limit= binds &0 and Validate
// rejects it (0331). Status/SortBy/LimitType/Cursor are PLAIN strings, so a
// present-but-empty value binds "" — identical to absent, a no-op filter (200) —
// exactly as Fiber's QueryParser yields for a non-pointer string. The Huma
// binder must reproduce that: it must NOT invent a pointer where Fiber has a
// scalar. Huma drops value=="" before the handler, so the Resolver-backed binder
// reads the raw query to preserve the ?limit=&0 distinction.
func TestHuma_ListLimits_PresentButEmptyQueryParity(t *testing.T) {
	rejectCases := []struct{ name, query, code string }{
		// ?limit= -> Fiber Limit=&0 (gorilla decodes "" as 0) -> *limit<1 -> 0331.
		{"empty limit", "?limit=", "0331"},
	}
	for _, tc := range rejectCases {
		t.Run(tc.name, func(t *testing.T) {
			svc := &tenantSpyLimitService{}
			app := buildHumaLimitApp(t, svc, "tenant-alpha")

			req := httptest.NewRequest(http.MethodGet, "/v1/limits"+tc.query, nil)
			resp, err := app.Test(req, -1)
			require.NoError(t, err)
			defer func() { _ = resp.Body.Close() }()

			respBody, err := io.ReadAll(resp.Body)
			require.NoError(t, err)

			assert.Equal(t, http.StatusBadRequest, resp.StatusCode,
				"present-but-empty %q must be the canonical 400, matching Fiber", tc.query)
			var got map[string]any
			require.NoError(t, json.Unmarshal(respBody, &got), "body JSON: %s", string(respBody))
			assert.Equal(t, tc.code, got["code"], "code must be identical to the Fiber path for %q", tc.query)
			assert.Empty(t, svc.capturedTenant, "service must not be reached on a rejected query param")
		})
	}

	// Present-but-empty scope/name values (all *string) and plain-string Status
	// are a no-op filter: 200, service reached. For Status the "" binds to the
	// plain string field, which Validate() skips (`if i.Status != ""`) — identical
	// to Fiber's non-pointer bind, and the reason ?status= is NOT a 400 here (it IS
	// on the rule path, where Status is a pointer).
	passCases := []struct{ name, query string }{
		{"empty name", "?name="},
		{"empty account_id", "?account_id="},
		{"empty sub_type", "?sub_type="},
		{"empty status", "?status="},
	}
	for _, tc := range passCases {
		t.Run(tc.name, func(t *testing.T) {
			svc := &tenantSpyLimitService{listResult: &model.ListLimitsResult{Limits: nil}}
			app := buildHumaLimitApp(t, svc, "tenant-alpha")

			req := httptest.NewRequest(http.MethodGet, "/v1/limits"+tc.query, nil)
			resp, err := app.Test(req, -1)
			require.NoError(t, err)
			defer func() { _ = resp.Body.Close() }()

			_, err = io.ReadAll(resp.Body)
			require.NoError(t, err)

			assert.Equal(t, http.StatusOK, resp.StatusCode,
				"present-but-empty %q is a no-op filter, must stay 200 as in Fiber", tc.query)
			require.NotNil(t, svc.listFilter, "service must be reached for %q", tc.query)
			assert.Nil(t, svc.listFilter.ScopeFilter, "empty scope value must build no scope filter for %q", tc.query)
		})
	}
}

// TestHuma_ListLimits_RepeatedKeyParity: Fiber's QueryParser is LAST-wins;
// ?status=ACTIVE&status=garbage binds "garbage". The binder must resolve the
// SAME last value — url.Values.Get (first) would flip the status/code.
func TestHuma_ListLimits_RepeatedKeyParity(t *testing.T) {
	rejectCases := []struct{ name, query, code string }{
		{"status last invalid", "?status=ACTIVE&status=garbage", "0082"},
		{"limit last non-numeric", "?limit=25&limit=abc", "0082"},
		{"sort_by last invalid", "?sort_by=name&sort_by=priority", "0332"},
	}
	for _, tc := range rejectCases {
		t.Run(tc.name, func(t *testing.T) {
			svc := &tenantSpyLimitService{listResult: &model.ListLimitsResult{Limits: nil}}
			app := buildHumaLimitApp(t, svc, "tenant-alpha")

			req := httptest.NewRequest(http.MethodGet, "/v1/limits"+tc.query, nil)
			resp, err := app.Test(req, -1)
			require.NoError(t, err)
			defer func() { _ = resp.Body.Close() }()

			respBody, err := io.ReadAll(resp.Body)
			require.NoError(t, err)

			assert.Equal(t, http.StatusBadRequest, resp.StatusCode,
				"repeated key %q must resolve to Fiber's LAST value and 400", tc.query)
			var got map[string]any
			require.NoError(t, json.Unmarshal(respBody, &got), "body JSON: %s", string(respBody))
			assert.Equal(t, tc.code, got["code"], "code must match Fiber's last-wins value for %q", tc.query)
			assert.Empty(t, svc.capturedTenant, "service must not be reached on a rejected query param")
		})
	}

	// Repeated keys whose LAST value passes Validate: 200, filter from the LAST value.
	passCases := []struct {
		name       string
		query      string
		wantLimit  int
		wantStatus model.LimitStatus
	}{
		{"limit last in-range", "?limit=101&limit=25", 25, ""},
		{"status empty then valid", "?status=&status=ACTIVE", 10, model.LimitStatusActive},
		// Plain-string Status: last "" clears the filter (no-op), 200, Status nil.
		{"status valid then empty", "?status=ACTIVE&status=", 10, ""},
	}
	for _, tc := range passCases {
		t.Run(tc.name, func(t *testing.T) {
			svc := &tenantSpyLimitService{listResult: &model.ListLimitsResult{Limits: nil}}
			app := buildHumaLimitApp(t, svc, "tenant-alpha")

			req := httptest.NewRequest(http.MethodGet, "/v1/limits"+tc.query, nil)
			resp, err := app.Test(req, -1)
			require.NoError(t, err)
			defer func() { _ = resp.Body.Close() }()

			_, err = io.ReadAll(resp.Body)
			require.NoError(t, err)

			assert.Equal(t, http.StatusOK, resp.StatusCode, "repeated key %q must resolve to Fiber's LAST value and 200", tc.query)
			require.NotNil(t, svc.listFilter, "service must be reached for %q", tc.query)
			assert.Equal(t, tc.wantLimit, svc.listFilter.Limit, "limit must be the LAST value for %q", tc.query)
			if tc.wantStatus == "" {
				assert.Nil(t, svc.listFilter.Status, "status must be nil for %q", tc.query)
			} else {
				require.NotNil(t, svc.listFilter.Status, "status must be set for %q", tc.query)
				assert.Equal(t, tc.wantStatus, *svc.listFilter.Status, "status must be the LAST value for %q", tc.query)
			}
		})
	}
}

func TestHuma_ActivateLimit_Success(t *testing.T) {
	id := testutil.MustDeterministicUUID(30)
	limit := validLimit(id)
	limit.Status = model.LimitStatusActive
	svc := &tenantSpyLimitService{lifecycle: limit}
	app := buildHumaLimitApp(t, svc, "tenant-alpha")

	req := httptest.NewRequest(http.MethodPost, "/v1/limits/"+id.String()+"/activate", nil)
	resp, err := app.Test(req, -1)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	respBody, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	assert.Equal(t, http.StatusOK, resp.StatusCode, "ActivateLimit must return 200 through Huma")
	assert.NotContains(t, string(respBody), "$schema")

	var got map[string]any
	require.NoError(t, json.Unmarshal(respBody, &got), "body JSON: %s", string(respBody))
	assert.Equal(t, id.String(), got["limitId"])
	assert.Equal(t, "ACTIVE", got["status"])
	assert.Equal(t, "tenant-alpha", svc.capturedTenant)
}

func TestHuma_DeactivateLimit_Success(t *testing.T) {
	id := testutil.MustDeterministicUUID(31)
	limit := validLimit(id)
	limit.Status = model.LimitStatusInactive
	svc := &tenantSpyLimitService{lifecycle: limit}
	app := buildHumaLimitApp(t, svc, "tenant-alpha")

	req := httptest.NewRequest(http.MethodPost, "/v1/limits/"+id.String()+"/deactivate", nil)
	resp, err := app.Test(req, -1)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	respBody, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	assert.Equal(t, http.StatusOK, resp.StatusCode, "DeactivateLimit must return 200 through Huma")
	var got map[string]any
	require.NoError(t, json.Unmarshal(respBody, &got), "body JSON: %s", string(respBody))
	assert.Equal(t, "INACTIVE", got["status"])
	assert.Equal(t, "tenant-alpha", svc.capturedTenant)
}

func TestHuma_DraftLimit_Success(t *testing.T) {
	id := testutil.MustDeterministicUUID(32)
	limit := validLimit(id)
	limit.Status = model.LimitStatusDraft
	svc := &tenantSpyLimitService{lifecycle: limit}
	app := buildHumaLimitApp(t, svc, "tenant-alpha")

	req := httptest.NewRequest(http.MethodPost, "/v1/limits/"+id.String()+"/draft", nil)
	resp, err := app.Test(req, -1)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	respBody, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	assert.Equal(t, http.StatusOK, resp.StatusCode, "DraftLimit must return 200 through Huma")
	var got map[string]any
	require.NoError(t, json.Unmarshal(respBody, &got), "body JSON: %s", string(respBody))
	assert.Equal(t, "DRAFT", got["status"])
	assert.Equal(t, "tenant-alpha", svc.capturedTenant)
}

func TestHuma_ActivateLimit_BadUUID(t *testing.T) {
	svc := &tenantSpyLimitService{}
	app := buildHumaLimitApp(t, svc, "tenant-gamma")

	req := httptest.NewRequest(http.MethodPost, "/v1/limits/not-a-uuid/activate", nil)
	resp, err := app.Test(req, -1)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	respBody, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
	var got map[string]any
	require.NoError(t, json.Unmarshal(respBody, &got), "body JSON: %s", string(respBody))
	assert.Equal(t, "0065", got["code"])
	assert.Empty(t, svc.capturedTenant, "service must not be reached on a bad path param")
}

func TestHuma_DeleteLimit_Success204NoBody(t *testing.T) {
	id := testutil.MustDeterministicUUID(40)
	svc := &tenantSpyLimitService{} // deleteErr nil -> success
	app := buildHumaLimitApp(t, svc, "tenant-alpha")

	req := httptest.NewRequest(http.MethodDelete, "/v1/limits/"+id.String(), nil)
	resp, err := app.Test(req, -1)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	respBody, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	assert.Equal(t, http.StatusNoContent, resp.StatusCode, "DeleteLimit must return 204 through Huma")
	assert.Empty(t, respBody, "204 must carry no body; got %q", string(respBody))
	assert.Equal(t, "tenant-alpha", svc.capturedTenant)
}

func TestHuma_DeleteLimit_BadUUID(t *testing.T) {
	svc := &tenantSpyLimitService{}
	app := buildHumaLimitApp(t, svc, "tenant-gamma")

	req := httptest.NewRequest(http.MethodDelete, "/v1/limits/not-a-uuid", nil)
	resp, err := app.Test(req, -1)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	respBody, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
	var got map[string]any
	require.NoError(t, json.Unmarshal(respBody, &got), "body JSON: %s", string(respBody))
	assert.Equal(t, "0065", got["code"])
	assert.Empty(t, svc.capturedTenant, "service must not be reached on a bad path param")
}

func TestHuma_GetLimitUsage_Success(t *testing.T) {
	id := testutil.MustDeterministicUUID(50)
	svc := &tenantSpyLimitService{usageResult: &model.UsageSnapshot{
		LimitID:            id,
		CurrentUsage:       decimal.RequireFromString("500.00"),
		LimitAmount:        decimal.RequireFromString("1000.00"),
		UtilizationPercent: 50.0,
	}}
	app := buildHumaLimitApp(t, svc, "tenant-alpha")

	req := httptest.NewRequest(http.MethodGet, "/v1/limits/"+id.String()+"/usage", nil)
	resp, err := app.Test(req, -1)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	respBody, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	assert.Equal(t, http.StatusOK, resp.StatusCode, "GetLimitUsage must return 200 through Huma")
	assert.NotContains(t, string(respBody), "$schema")

	var got map[string]any
	require.NoError(t, json.Unmarshal(respBody, &got), "body JSON: %s", string(respBody))
	assert.Equal(t, id.String(), got["limitId"])
	assert.Equal(t, 50.0, got["utilizationPercent"])
	assert.Equal(t, "tenant-alpha", svc.capturedTenant)
}
