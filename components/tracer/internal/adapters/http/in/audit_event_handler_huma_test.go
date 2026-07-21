// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
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
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/trace"

	"github.com/LerianStudio/midaz/v4/components/tracer/internal/testutil"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/model"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	pkgHTTP "github.com/LerianStudio/midaz/v4/pkg/net/http"
)

// tenantSpyAuditEventService is an AuditEventService stub that records the tenant
// ID it sees on its incoming context. It is the ctx-threading probe (see
// tenantSpyService in rule_handler_huma_test.go): a non-empty capturedTenant
// proves the tenant the middleware put on c.UserContext() reached the service
// through the Huma handler ctx with NO bridge. It also captures the filter the
// core built from the query, so tests can assert the imperative binding/defaults
// produced the same filter the Fiber QueryParser path would.
type tenantSpyAuditEventService struct {
	capturedTenant string

	listResult *model.ListAuditEventsResult
	listErr    error
	listFilter *model.AuditEventFilters

	getResult *model.AuditEvent
	getErr    error

	verifyResult *model.HashChainVerificationResult
	verifyErr    error
}

func (s *tenantSpyAuditEventService) ListAuditEvents(ctx context.Context, filters *model.AuditEventFilters) (*model.ListAuditEventsResult, error) {
	s.capturedTenant = tmctx.GetTenantIDContext(ctx)
	s.listFilter = filters

	return s.listResult, s.listErr
}

func (s *tenantSpyAuditEventService) GetAuditEvent(ctx context.Context, _ uuid.UUID) (*model.AuditEvent, error) {
	s.capturedTenant = tmctx.GetTenantIDContext(ctx)
	return s.getResult, s.getErr
}

func (s *tenantSpyAuditEventService) VerifyHashChain(ctx context.Context, _ uuid.UUID) (*model.HashChainVerificationResult, error) {
	s.capturedTenant = tmctx.GetTenantIDContext(ctx)
	return s.verifyResult, s.verifyErr
}

// buildHumaAuditEventApp mirrors buildHumaRuleApp (rule_handler_huma_test.go):
// problem.Install() before any Register, the Huma API built with openapi.New over
// the SAME /v1 group that carries a tenant-injecting middleware, and
// RegisterAuditEventRoutes registering the three audit-event ops.
//
// MUST-NOT-PARALLELIZE (identical rationale to buildHumaRuleApp): tests built
// through this helper CANNOT call t.Parallel(). libProblem.Install() swaps the
// process-global huma.NewError hook and Huma validation uses process-global
// sync.Pools; concurrent builds/requests cross-contaminate, surfacing a phantom
// native 422 with a nil code instead of the canonical envelope. -race does NOT
// catch this (the bug is logical, not a data race). Keep them sequential.
func buildHumaAuditEventApp(t *testing.T, svc AuditEventService, tenantID string) *fiber.App {
	t.Helper()

	f := fiber.New(fiber.Config{
		DisableStartupMessage: true,
		ErrorHandler:          pkgHTTP.CanonicalFiberErrorHandler,
	})

	// problem.Install must run before any huma.Register (runtime + spec-gen).
	libProblem.Install()

	api := f.Group("/v1")
	api.Use(func(c *fiber.Ctx) error {
		if tenantID != "" {
			c.SetUserContext(tmctx.ContextWithTenantID(c.UserContext(), tenantID))
		}
		return c.Next()
	})

	hAPI := openapi.New(f, api, openapi.Config{Title: "tracer-test", Version: "test", Servers: []string{"/v1"}})

	h := NewAuditEventHandler(svc)
	RegisterAuditEventRoutes(hAPI, h)

	return f
}

func TestHuma_ListAuditEvents_Success(t *testing.T) {
	// NOT parallel: buildHumaAuditEventApp mutates process-global huma state.
	eventID := testutil.MustDeterministicUUID(1)
	svc := &tenantSpyAuditEventService{listResult: &model.ListAuditEventsResult{
		AuditEvents: []*model.AuditEvent{
			{
				EventID:      eventID,
				EventType:    model.AuditEventRuleCreated,
				Action:       model.AuditActionCreate,
				Result:       model.AuditResultSuccess,
				ResourceID:   testutil.MustDeterministicUUID(2).String(),
				ResourceType: model.ResourceTypeRule,
				CreatedAt:    testutil.FixedTime(),
				Actor:        model.Actor{ActorType: model.ActorTypeUser, ID: "user-1", Name: "User 1"},
			},
		},
		HasMore:    true,
		NextCursor: "next123",
	}}
	app := buildHumaAuditEventApp(t, svc, "tenant-alpha")

	req := httptest.NewRequest(http.MethodGet, "/v1/audit-events?event_type=RULE_CREATED&limit=25&sort_by=event_type&sort_order=asc", nil)
	resp, err := app.Test(req, -1)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	respBody, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	assert.Equal(t, http.StatusOK, resp.StatusCode, "ListAuditEvents must return 200 through Huma")
	assert.NotContains(t, string(respBody), "$schema", "SchemaLinkTransformer must be zeroed — no $schema in body")

	var got ListAuditEventsResponse
	require.NoError(t, json.Unmarshal(respBody, &got), "body must be ListAuditEventsResponse: %s", string(respBody))
	require.Len(t, got.AuditEvents, 1)
	assert.Equal(t, model.AuditEventRuleCreated, got.AuditEvents[0].EventType)
	assert.Equal(t, "next123", got.NextCursor)
	assert.True(t, got.HasMore)

	assert.Equal(t, "tenant-alpha", svc.capturedTenant,
		"tenant from c.UserContext() must reach the service via the Huma handler ctx")

	// Imperative binding + typed-pointer conversion produced the same filter the
	// Fiber QueryParser path would.
	require.NotNil(t, svc.listFilter)
	assert.Equal(t, 25, svc.listFilter.Limit)
	require.NotNil(t, svc.listFilter.EventType)
	assert.Equal(t, model.AuditEventRuleCreated, *svc.listFilter.EventType)
	assert.Equal(t, "event_type", svc.listFilter.SortBy)
	assert.Equal(t, "ASC", svc.listFilter.SortOrder, "sort_order normalized to uppercase by SetDefaults")
}

func TestHuma_ListAuditEvents_Defaults(t *testing.T) {
	svc := &tenantSpyAuditEventService{listResult: &model.ListAuditEventsResult{AuditEvents: nil}}
	app := buildHumaAuditEventApp(t, svc, "tenant-alpha")

	req := httptest.NewRequest(http.MethodGet, "/v1/audit-events", nil)
	resp, err := app.Test(req, -1)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	respBody, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	// auditEvents must serialize as [] not null.
	var got map[string]any
	require.NoError(t, json.Unmarshal(respBody, &got), "body JSON: %s", string(respBody))
	assert.NotNil(t, got["auditEvents"], "auditEvents must be [] not null")

	require.NotNil(t, svc.listFilter)
	assert.Equal(t, DefaultAuditEventLimit, svc.listFilter.Limit, "default limit applied by SetDefaults")
	assert.Equal(t, "created_at", svc.listFilter.SortBy, "default sort field applied by SetDefaults")
	assert.Equal(t, "DESC", svc.listFilter.SortOrder, "default sort order applied by SetDefaults")
}

// TestHuma_ListAuditEvents_InvalidEnumParams pins the typed-pointer enum query
// params: a bad value must be bound into the typed pointer, reach the imperative
// Validate(), and produce the canonical 400 — NOT a native Huma 422 from an
// `enum`/`validate` struct tag. event_type=INVALID hits the go-playground
// auditeventtype validator whose formatValidationError default arm is
// ErrMissingFieldsInRequest (0009); transaction_type=INVALID is 0009 too. These
// codes are empirically anchored to the pre-Huma Fiber handler.
func TestHuma_ListAuditEvents_InvalidEnumParams(t *testing.T) {
	for _, tc := range []struct {
		name, query, code string
	}{
		{"invalid event_type", "event_type=INVALID", "0009"},
		{"invalid action", "action=INVALID", "0009"},
		{"invalid result", "result=INVALID", "0009"},
		{"invalid resource_type", "resource_type=INVALID", "0009"},
		{"invalid actor_type", "actor_type=INVALID", "0009"},
		{"invalid transaction_type", "transaction_type=INVALID", "0009"},
		{"invalid sort_by", "sort_by=priority", "0332"},     // ErrInvalidSortColumn
		{"invalid sort_order", "sort_order=RANDOM", "0081"}, // ErrInvalidSortOrder
	} {
		t.Run(tc.name, func(t *testing.T) {
			svc := &tenantSpyAuditEventService{}
			app := buildHumaAuditEventApp(t, svc, "tenant-gamma")

			req := httptest.NewRequest(http.MethodGet, "/v1/audit-events?"+tc.query, nil)
			resp, err := app.Test(req, -1)
			require.NoError(t, err)
			defer func() { _ = resp.Body.Close() }()

			respBody, err := io.ReadAll(resp.Body)
			require.NoError(t, err)

			assert.Equal(t, http.StatusBadRequest, resp.StatusCode, "invalid enum query must be the canonical 400 — no native Huma 422")
			assert.Equal(t, "application/problem+json", resp.Header.Get("Content-Type"))

			var got map[string]any
			require.NoError(t, json.Unmarshal(respBody, &got), "body JSON: %s", string(respBody))
			assert.Equal(t, tc.code, got["code"], "canonical code identical to the Fiber path")
			assert.Empty(t, svc.capturedTenant, "service must not be reached on a bad query param")
		})
	}
}

// TestHuma_ListAuditEvents_InvalidUUIDParam pins the uuid-shaped query params
// (resource_id/account_id/segment_id/portfolio_id/matched_rule_id): a malformed
// UUID reaches the imperative validator's `uuid` rule and produces the canonical
// 400/0009 (formatValidationError default arm) — NOT a native Huma 422 from a
// format:"uuid" tag.
func TestHuma_ListAuditEvents_InvalidUUIDParam(t *testing.T) {
	for _, tc := range []struct {
		name, query string
	}{
		{"bad resource_id", "resource_id=not-a-uuid"},
		{"bad account_id", "account_id=not-a-uuid"},
		{"bad segment_id", "segment_id=not-a-uuid"},
		{"bad portfolio_id", "portfolio_id=not-a-uuid"},
		{"bad matched_rule_id", "matched_rule_id=not-a-uuid"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			svc := &tenantSpyAuditEventService{}
			app := buildHumaAuditEventApp(t, svc, "tenant-gamma")

			req := httptest.NewRequest(http.MethodGet, "/v1/audit-events?"+tc.query, nil)
			resp, err := app.Test(req, -1)
			require.NoError(t, err)
			defer func() { _ = resp.Body.Close() }()

			respBody, err := io.ReadAll(resp.Body)
			require.NoError(t, err)

			assert.Equal(t, http.StatusBadRequest, resp.StatusCode, "malformed UUID query must be the canonical 400 — no native Huma 422")
			assert.Equal(t, "application/problem+json", resp.Header.Get("Content-Type"))

			var got map[string]any
			require.NoError(t, json.Unmarshal(respBody, &got), "body JSON: %s", string(respBody))
			assert.Equal(t, "0009", got["code"], "canonical code identical to the Fiber path")
			assert.Empty(t, svc.capturedTenant, "service must not be reached on a bad query param")
		})
	}
}

// TestHuma_ListAuditEvents_InvalidDate pins the date-format query contract:
// start_date=invalid reaches the imperative Validate() date parse and produces
// the canonical 400/0077 (ErrInvalidDateFormat) — identical to the Fiber path.
func TestHuma_ListAuditEvents_InvalidDate(t *testing.T) {
	svc := &tenantSpyAuditEventService{}
	app := buildHumaAuditEventApp(t, svc, "tenant-gamma")

	req := httptest.NewRequest(http.MethodGet, "/v1/audit-events?start_date=invalid-date", nil)
	resp, err := app.Test(req, -1)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	respBody, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
	assert.Equal(t, "application/problem+json", resp.Header.Get("Content-Type"))

	var got map[string]any
	require.NoError(t, json.Unmarshal(respBody, &got), "body JSON: %s", string(respBody))
	assert.Equal(t, "0077", got["code"], "code must be ErrInvalidDateFormat, identical to the Fiber path")
	assert.Empty(t, svc.capturedTenant, "service must not be reached on a bad date param")
}

// TestHuma_ListAuditEvents_InvalidLimit pins the pagination limit query contract:
// an out-of-range limit reaches the imperative Validate() and produces the
// canonical 400/0080 (ErrPaginationLimitExceeded) — NOT a native Huma 422 from a
// min/max struct tag. Max audit limit is 1000.
func TestHuma_ListAuditEvents_InvalidLimit(t *testing.T) {
	svc := &tenantSpyAuditEventService{}
	app := buildHumaAuditEventApp(t, svc, "tenant-gamma")

	req := httptest.NewRequest(http.MethodGet, "/v1/audit-events?limit=1001", nil)
	resp, err := app.Test(req, -1)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	respBody, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	assert.Equal(t, http.StatusBadRequest, resp.StatusCode, "out-of-range limit must be the canonical 400 — no native Huma 422")

	var got map[string]any
	require.NoError(t, json.Unmarshal(respBody, &got), "body JSON: %s", string(respBody))
	assert.Equal(t, "0080", got["code"], "code must be ErrPaginationLimitExceeded, identical to the Fiber path")
	assert.Empty(t, svc.capturedTenant, "service must not be reached on a bad query param")
}

// TestHuma_ListAuditEvents_NonNumericLimit pins the harder half of the query
// contract: limit=abc cannot coerce to an int. Because the Huma In struct takes
// limit as a STRING (no typed coercion, no min/max tag), Huma never fires a
// native 422; the binder parse fails and listAuditEvents canonicalizes it to
// ErrInvalidQueryParameter (0082) — the same code the Fiber QueryParser-failure
// path produces.
func TestHuma_ListAuditEvents_NonNumericLimit(t *testing.T) {
	svc := &tenantSpyAuditEventService{}
	app := buildHumaAuditEventApp(t, svc, "tenant-gamma")

	req := httptest.NewRequest(http.MethodGet, "/v1/audit-events?limit=abc", nil)
	resp, err := app.Test(req, -1)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	respBody, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	assert.Equal(t, http.StatusBadRequest, resp.StatusCode, "non-numeric limit must be the canonical 400 — no native Huma 422")

	var got map[string]any
	require.NoError(t, json.Unmarshal(respBody, &got), "body JSON: %s", string(respBody))
	assert.Equal(t, "0082", got["code"], "code must be ErrInvalidQueryParameter, identical to the Fiber QueryParser-failure path")
	assert.Empty(t, svc.capturedTenant, "service must not be reached on an unparseable query param")
}

// TestHuma_ListAuditEvents_RepeatedKeyParity pins the repeated-query-key
// contract. Fiber's c.QueryParser (gorilla/schema over a scalar target) is
// LAST-wins: ?event_type=RULE_CREATED&event_type=garbage binds "garbage". The
// Huma binder MUST resolve the SAME last value; reading the first value
// (url.Values.Get) would flip the status/code. Each row's code is what the LAST
// value hits in the shared imperative Validate().
func TestHuma_ListAuditEvents_RepeatedKeyParity(t *testing.T) {
	rejectCases := []struct {
		name, query, code string
	}{
		// Last-wins "garbage" -> auditeventtype invalid -> 0009. First-wins "RULE_CREATED" would 200 (FLIP).
		{"event_type last invalid", "?event_type=RULE_CREATED&event_type=garbage", "0009"},
		// Last-wins "abc" -> int parse error -> 0082. First-wins "25" would 200 (FLIP).
		{"limit last non-numeric", "?limit=25&limit=abc", "0082"},
		// Last-wins "priority" -> not in sort whitelist -> 0332. First-wins "event_type" would 200 (FLIP).
		{"sort_by last invalid", "?sort_by=event_type&sort_by=priority", "0332"},
	}
	for _, tc := range rejectCases {
		t.Run(tc.name, func(t *testing.T) {
			svc := &tenantSpyAuditEventService{listResult: &model.ListAuditEventsResult{AuditEvents: nil}}
			app := buildHumaAuditEventApp(t, svc, "tenant-alpha")

			req := httptest.NewRequest(http.MethodGet, "/v1/audit-events"+tc.query, nil)
			resp, err := app.Test(req, -1)
			require.NoError(t, err)
			defer func() { _ = resp.Body.Close() }()

			respBody, err := io.ReadAll(resp.Body)
			require.NoError(t, err)

			assert.Equal(t, http.StatusBadRequest, resp.StatusCode,
				"repeated key %q must resolve to Fiber's LAST value and 400 — never the first-value 200 (a status flip)", tc.query)

			var got map[string]any
			require.NoError(t, json.Unmarshal(respBody, &got), "body JSON: %s", string(respBody))
			assert.Equal(t, tc.code, got["code"], "code must match Fiber's last-wins value for %q", tc.query)
			assert.Empty(t, svc.capturedTenant, "service must not be reached on a rejected query param")
		})
	}

	// Repeated key whose LAST value passes Validate: 200, filter built from the
	// LAST value. Guards the reverse flip.
	t.Run("event_type last valid", func(t *testing.T) {
		svc := &tenantSpyAuditEventService{listResult: &model.ListAuditEventsResult{AuditEvents: nil}}
		app := buildHumaAuditEventApp(t, svc, "tenant-alpha")

		req := httptest.NewRequest(http.MethodGet, "/v1/audit-events?event_type=garbage&event_type=RULE_CREATED", nil)
		resp, err := app.Test(req, -1)
		require.NoError(t, err)
		defer func() { _ = resp.Body.Close() }()

		_, err = io.ReadAll(resp.Body)
		require.NoError(t, err)

		assert.Equal(t, http.StatusOK, resp.StatusCode, "repeated key must resolve to Fiber's LAST value and 200")
		require.NotNil(t, svc.listFilter)
		require.NotNil(t, svc.listFilter.EventType)
		assert.Equal(t, model.AuditEventRuleCreated, *svc.listFilter.EventType, "event_type must be the LAST value")
	})
}

// TestHuma_ListAuditEvents_PresentButEmptyQueryParity is the money-path
// regression for the present-vs-absent distinction. Fiber's c.QueryParser binds
// a present-but-empty typed key to a NON-nil pointer (`?event_type=` -> &"") which
// downstream Validate() rejects (auditeventtype("") invalid -> 0009); an ABSENT
// key stays nil (no filter). Huma's request layer drops value=="" before the
// handler, so the Huma path MUST read the raw query (via the Resolver) to
// reproduce Fiber's distinction.
func TestHuma_ListAuditEvents_PresentButEmptyQueryParity(t *testing.T) {
	// Present-but-empty typed enum: Fiber binds &"" -> Validate rejects -> 0009.
	rejectCases := []struct {
		name, query, code string
	}{
		{"empty event_type", "?event_type=", "0009"},
		{"empty action", "?action=", "0009"},
		{"empty limit", "?limit=", "0331"}, // Fiber Limit=&0 -> *limit<1 -> 0331.
	}
	for _, tc := range rejectCases {
		t.Run(tc.name, func(t *testing.T) {
			svc := &tenantSpyAuditEventService{}
			app := buildHumaAuditEventApp(t, svc, "tenant-alpha")

			req := httptest.NewRequest(http.MethodGet, "/v1/audit-events"+tc.query, nil)
			resp, err := app.Test(req, -1)
			require.NoError(t, err)
			defer func() { _ = resp.Body.Close() }()

			respBody, err := io.ReadAll(resp.Body)
			require.NoError(t, err)

			assert.Equal(t, http.StatusBadRequest, resp.StatusCode,
				"present-but-empty %q must be the canonical 400, matching Fiber — never a silent 200 or native 422", tc.query)

			var got map[string]any
			require.NoError(t, json.Unmarshal(respBody, &got), "body JSON: %s", string(respBody))
			assert.Equal(t, tc.code, got["code"], "code must be identical to the Fiber path for %q", tc.query)
			assert.Empty(t, svc.capturedTenant, "service must not be reached on a rejected query param")
		})
	}

	// Present-but-empty untyped string (actor_id/resource_id): resource_id carries
	// a uuid validate rule but omitempty means "" is skipped; actor_id has no rule.
	// Both are 200, no filter applied — identical to Fiber and to an absent key.
	t.Run("empty actor_id passes", func(t *testing.T) {
		svc := &tenantSpyAuditEventService{listResult: &model.ListAuditEventsResult{AuditEvents: nil}}
		app := buildHumaAuditEventApp(t, svc, "tenant-alpha")

		req := httptest.NewRequest(http.MethodGet, "/v1/audit-events?actor_id=", nil)
		resp, err := app.Test(req, -1)
		require.NoError(t, err)
		defer func() { _ = resp.Body.Close() }()

		_, err = io.ReadAll(resp.Body)
		require.NoError(t, err)

		assert.Equal(t, http.StatusOK, resp.StatusCode, "present-but-empty actor_id is a no-op filter, must stay 200 as in Fiber")
		require.NotNil(t, svc.listFilter)
		require.NotNil(t, svc.listFilter.ActorID, "Fiber binds present-but-empty actor_id to &\"\"")
		assert.Equal(t, "", *svc.listFilter.ActorID)
	})
}

func TestHuma_GetAuditEvent_Success(t *testing.T) {
	id := testutil.MustDeterministicUUID(30)
	svc := &tenantSpyAuditEventService{getResult: &model.AuditEvent{
		EventID:      id,
		EventType:    model.AuditEventLimitActivated,
		Action:       model.AuditActionActivate,
		Result:       model.AuditResultSuccess,
		ResourceID:   testutil.MustDeterministicUUID(31).String(),
		ResourceType: model.ResourceTypeLimit,
		CreatedAt:    testutil.FixedTime(),
		Actor:        model.Actor{ActorType: model.ActorTypeSystem, ID: "system", Name: "System"},
	}}
	app := buildHumaAuditEventApp(t, svc, "tenant-beta")

	req := httptest.NewRequest(http.MethodGet, "/v1/audit-events/"+id.String(), nil)
	resp, err := app.Test(req, -1)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	respBody, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	assert.Equal(t, http.StatusOK, resp.StatusCode, "GetAuditEvent must return 200 through Huma")
	assert.NotContains(t, string(respBody), "$schema")

	var got map[string]any
	require.NoError(t, json.Unmarshal(respBody, &got), "body JSON: %s", string(respBody))
	assert.Equal(t, id.String(), got["eventId"], "body is model.AuditEvent verbatim (eventId json tag)")
	assert.Equal(t, "LIMIT_ACTIVATED", got["eventType"])
	assert.Equal(t, "tenant-beta", svc.capturedTenant)
}

func TestHuma_GetAuditEvent_BadUUID(t *testing.T) {
	svc := &tenantSpyAuditEventService{}
	app := buildHumaAuditEventApp(t, svc, "tenant-gamma")

	req := httptest.NewRequest(http.MethodGet, "/v1/audit-events/not-a-uuid", nil)
	resp, err := app.Test(req, -1)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	respBody, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	assert.Equal(t, http.StatusBadRequest, resp.StatusCode, "malformed path UUID must be the canonical 400 — no native Huma 422")
	assert.Equal(t, "application/problem+json", resp.Header.Get("Content-Type"))

	var got map[string]any
	require.NoError(t, json.Unmarshal(respBody, &got), "body JSON: %s", string(respBody))
	assert.Equal(t, "0065", got["code"], "code must be ErrInvalidPathParameter, identical to the Fiber path")
	assert.Equal(t, libProblem.BaseURI+"/0065", got["type"])
	assert.Equal(t, "AuditEvent", got["entityType"], "canonical entityType from ErrInvalidPathParameter")
	assert.Empty(t, svc.capturedTenant, "service must not be reached on a bad path param")
}

func TestHuma_GetAuditEvent_NotFound(t *testing.T) {
	id := testutil.MustDeterministicUUID(33)
	svc := &tenantSpyAuditEventService{getErr: constant.ErrAuditEventNotFound}
	app := buildHumaAuditEventApp(t, svc, "tenant-delta")

	req := httptest.NewRequest(http.MethodGet, "/v1/audit-events/"+id.String(), nil)
	resp, err := app.Test(req, -1)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	respBody, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	assert.Equal(t, http.StatusNotFound, resp.StatusCode)
	assert.Equal(t, "application/problem+json", resp.Header.Get("Content-Type"))

	var got map[string]any
	require.NoError(t, json.Unmarshal(respBody, &got), "body JSON: %s", string(respBody))
	assert.Equal(t, "0381", got["code"], "code must be ErrAuditEventNotFound, identical to the Fiber path")
}

func TestHuma_VerifyHashChain_Success(t *testing.T) {
	id := testutil.MustDeterministicUUID(40)
	svc := &tenantSpyAuditEventService{verifyResult: &model.HashChainVerificationResult{
		IsValid:      true,
		TotalChecked: 100,
		Message:      "Hash chain is valid",
	}}
	app := buildHumaAuditEventApp(t, svc, "tenant-alpha")

	req := httptest.NewRequest(http.MethodGet, "/v1/audit-events/"+id.String()+"/verify", nil)
	resp, err := app.Test(req, -1)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	respBody, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	assert.Equal(t, http.StatusOK, resp.StatusCode, "VerifyHashChain must return 200 through Huma")
	assert.NotContains(t, string(respBody), "$schema")

	var got model.HashChainVerificationResult
	require.NoError(t, json.Unmarshal(respBody, &got), "body JSON: %s", string(respBody))
	assert.True(t, got.IsValid)
	assert.Equal(t, int64(100), got.TotalChecked)
	assert.Equal(t, "tenant-alpha", svc.capturedTenant)
}

func TestHuma_VerifyHashChain_BadUUID(t *testing.T) {
	svc := &tenantSpyAuditEventService{}
	app := buildHumaAuditEventApp(t, svc, "tenant-gamma")

	req := httptest.NewRequest(http.MethodGet, "/v1/audit-events/not-a-uuid/verify", nil)
	resp, err := app.Test(req, -1)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	respBody, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	assert.Equal(t, http.StatusBadRequest, resp.StatusCode, "malformed path UUID must be the canonical 400 — no native Huma 422")

	var got map[string]any
	require.NoError(t, json.Unmarshal(respBody, &got), "body JSON: %s", string(respBody))
	assert.Equal(t, "0065", got["code"])
	assert.Empty(t, svc.capturedTenant, "service must not be reached on a bad path param")
}

// TestHuma_AuditEventErrorBodyMatchesFiberEnvelope pins field-identity of the
// error body: the same domain error rendered through the legacy Fiber
// http.WithError path and through the migrated Huma handler must DECODE to the
// identical JSON object. Both share pkgHTTP.ProblemDetail so every field matches.
func TestHuma_AuditEventErrorBodyMatchesFiberEnvelope(t *testing.T) {
	svc := &tenantSpyAuditEventService{getErr: constant.ErrAuditEventNotFound}
	app := buildHumaAuditEventApp(t, svc, "tenant-delta")

	id := testutil.MustDeterministicUUID(3)
	req := httptest.NewRequest(http.MethodGet, "/v1/audit-events/"+id.String(), nil)
	resp, err := app.Test(req, -1)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	humaBody, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	// Reference: the same error through the frozen Fiber envelope.
	ref := fiber.New(fiber.Config{ErrorHandler: pkgHTTP.CanonicalFiberErrorHandler})
	ref.Get("/probe", func(c *fiber.Ctx) error {
		return pkgHTTP.WithError(c, classifyAuditEventError(trace.SpanFromContext(c.UserContext()), constant.ErrAuditEventNotFound))
	})
	refResp, err := ref.Test(httptest.NewRequest(http.MethodGet, "/probe", nil), -1)
	require.NoError(t, err)
	defer func() { _ = refResp.Body.Close() }()

	refBody, err := io.ReadAll(refResp.Body)
	require.NoError(t, err)

	assert.Equal(t, refResp.StatusCode, resp.StatusCode, "status must match the Fiber envelope")

	var humaJSON, refJSON map[string]any
	require.NoError(t, json.Unmarshal(humaBody, &humaJSON), "huma body JSON: %s", string(humaBody))
	require.NoError(t, json.Unmarshal(refBody, &refJSON), "ref body JSON: %s", string(refBody))
	assert.Equal(t, refJSON, humaJSON, "Huma error body must decode field-identical to the Fiber envelope")
}
