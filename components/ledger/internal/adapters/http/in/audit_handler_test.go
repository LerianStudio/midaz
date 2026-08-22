// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	libCommons "github.com/LerianStudio/lib-commons/v6/commons"

	libHTTP "github.com/LerianStudio/lib-commons/v6/commons/net/http"
	openapi "github.com/LerianStudio/lib-commons/v6/commons/net/http/openapi"
	libProblem "github.com/LerianStudio/lib-commons/v6/commons/net/http/problem"
	"github.com/gofiber/fiber/v3"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/crm/adapters/mongodb/audit"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	pkgHTTP "github.com/LerianStudio/midaz/v4/pkg/net/http"
)

// buildHumaAuditApp mounts the protection-audit Huma operation on a /v2 group,
// faithfully mirroring the production wiring in unified-server.go: problem.Install()
// runs before any huma.Register, the Huma API is built with openapi.New over a /v2
// group, an auth-shim middleware stands in for auth.Authorize("midaz","protection",
// "get") + tenant PostAuthMiddlewares, and http.ParseUUIDPathParameters("organization")
// + RegisterAuditRoutes attach the chain.
//
// MUST-NOT-PARALLELIZE (same rationale as the asset exemplar's buildHumaAssetApp):
// libProblem.Install() swaps the process-global huma.NewError hook and Huma
// validation uses process-global sync.Pools — concurrent builds/requests
// cross-contaminate. These tests are sub-second; keep them sequential.
func buildHumaAuditApp(t *testing.T, handler *AuditHandler, authOK bool) *fiber.App {
	t.Helper()

	f := fiber.New(fiber.Config{
		ErrorHandler: pkgHTTP.CanonicalFiberErrorHandler,
	})

	libProblem.Install()

	apiV2 := f.Group("/v2")

	apiV2.Use(func(c fiber.Ctx) error {
		if !authOK {
			return pkgHTTP.Unauthorized(c, "0001", "Unauthorized", "auth required")
		}

		return c.Next()
	})

	hAPI := openapi.New(f, apiV2, openapi.Config{Title: "ledger-test", Version: "test", Servers: []string{"/v2"}})

	apiV2.Get("/organizations/:organization_id/protection/audit", pkgHTTP.ParseUUIDPathParameters("organization"))

	RegisterAuditRoutes(hAPI, handler, crmOpSuffixV2)

	return f
}

func TestGetAuditEvents_Success(t *testing.T) {
	// NOT parallel: buildHumaAuditApp mutates process-global huma state.
	orgID := uuid.Must(libCommons.GenerateUUIDv7())
	ts := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	eventID := uuid.MustParse("11111111-2222-3333-4444-555555555555")

	stub := &auditServiceStub{
		events: []*mmodel.ProtectionAuditEvent{
			{
				ID:             eventID,
				TenantID:       "tenant-secret",
				OrganizationID: orgID.String(),
				EventType:      mmodel.AuditEventTypeProvisioning,
				Action:         mmodel.AuditActionProvision,
				Outcome:        mmodel.AuditOutcomeSuccess,
				ActorID:        "admin@example.com",
				Reason:         "initial setup",
				Timestamp:      ts,
				RequestID:      "req-123",
				Details: &mmodel.AuditDetails{
					PreviousStatus:    "PENDING",
					NewStatus:         "ACTIVE",
					ProviderReference: "vault://secret/ref",
				},
			},
		},
		pagination: libHTTP.CursorPagination{Next: "next-token", Prev: "prev-token"},
	}

	handler := &AuditHandler{Service: stub}
	app := buildHumaAuditApp(t, handler, true)

	req := httptest.NewRequest(http.MethodGet, "/v2/organizations/"+orgID.String()+"/protection/audit?limit=2&sort_order=desc", nil)

	resp, err := app.Test(req, fiber.TestConfig{Timeout: 0})
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	respBody, _ := io.ReadAll(resp.Body)

	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.NotContains(t, string(respBody), "$schema", "SchemaLinkTransformer must be zeroed")

	var env map[string]any
	require.NoError(t, json.Unmarshal(respBody, &env), "body: %s", string(respBody))

	assert.Equal(t, orgID.String(), env["organization_id"])
	assert.EqualValues(t, 2, env["limit"])
	assert.Equal(t, "next-token", env["next_cursor"])
	assert.Equal(t, "prev-token", env["prev_cursor"])

	items, ok := env["items"].([]any)
	require.True(t, ok, "items should be an array, body: %s", string(respBody))
	require.Len(t, items, 1)

	item, ok := items[0].(map[string]any)
	require.True(t, ok, "item should be an object, body: %s", string(respBody))
	assert.Equal(t, "provision", item["action"])
	assert.Equal(t, "PENDING", item["from_status"])
	assert.Equal(t, "ACTIVE", item["to_status"])

	// Query binding + core wiring: the stub saw the org id and the parsed query.
	assert.Equal(t, 1, stub.calls)
	assert.Equal(t, orgID.String(), stub.gotOrgID)
	assert.Equal(t, 2, stub.gotQuery.Limit)
	assert.Equal(t, "desc", stub.gotQuery.SortOrder)

	// Internal-only fields MUST be excluded.
	assert.NotContains(t, string(respBody), "tenant-secret")
	assert.NotContains(t, string(respBody), "vault://secret/ref")
}

func TestGetAuditEvents_UnsupportedOutcomeRejectedByCore(t *testing.T) {
	// NOT parallel: process-global huma state. An unsupported outcome filter must be
	// rejected by the core (canonical 400), NOT bound/accepted by Huma.
	orgID := uuid.Must(libCommons.GenerateUUIDv7())

	stub := &auditServiceStub{}
	handler := &AuditHandler{Service: stub}
	app := buildHumaAuditApp(t, handler, true)

	req := httptest.NewRequest(http.MethodGet, "/v2/organizations/"+orgID.String()+"/protection/audit?outcome=conflict", nil)

	resp, err := app.Test(req, fiber.TestConfig{Timeout: 0})
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
	assert.Equal(t, 0, stub.calls, "service must not be called on a rejected filter")
}

func TestGetAuditEvents_AuthPreserved(t *testing.T) {
	// NOT parallel: process-global huma state.
	orgID := uuid.Must(libCommons.GenerateUUIDv7())

	stub := &auditServiceStub{}
	handler := &AuditHandler{Service: stub}
	app := buildHumaAuditApp(t, handler, false)

	req := httptest.NewRequest(http.MethodGet, "/v2/organizations/"+orgID.String()+"/protection/audit", nil)

	resp, err := app.Test(req, fiber.TestConfig{Timeout: 0})
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
	assert.Equal(t, 0, stub.calls, "service must not be called when auth rejects")
}

// TestGetAuditEvents_QueryContract exercises the full query-binding and
// validation contract of the getAuditEvents core through the Huma transport:
// defaults, per-bound date semantics, filter forwarding, rejection cases and the
// service error branches.
func TestGetAuditEvents_QueryContract(t *testing.T) {
	// NOT parallel: buildHumaAuditApp mutates process-global huma state.
	orgID := uuid.Must(libCommons.GenerateUUIDv7())
	ts := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)

	tests := []struct {
		name           string
		query          string
		stub           *auditServiceStub
		expectedStatus int
		expectedCode   string
		validateBody   func(t *testing.T, body []byte)
		validateCall   func(t *testing.T, stub *auditServiceStub)
	}{
		{
			name:  "absent limit and sort_order default to 20 and desc",
			query: "",
			stub: &auditServiceStub{
				events:     []*mmodel.ProtectionAuditEvent{},
				pagination: libHTTP.CursorPagination{},
			},
			expectedStatus: http.StatusOK,
			validateBody: func(t *testing.T, body []byte) {
				var env map[string]any
				require.NoError(t, json.Unmarshal(body, &env))
				assert.EqualValues(t, 20, env["limit"], "absent limit defaults to 20")
			},
			validateCall: func(t *testing.T, stub *auditServiceStub) {
				assert.Equal(t, 20, stub.gotQuery.Limit, "absent limit forwarded to query as 20")
				assert.Equal(t, "desc", stub.gotQuery.SortOrder, "absent sort_order forwarded to query as desc")
			},
		},
		{
			name:  "date-only (yyyy-mm-dd) bounds are accepted and forwarded",
			query: "?start_date=2026-01-01&end_date=2026-02-01",
			stub: &auditServiceStub{
				events:     []*mmodel.ProtectionAuditEvent{},
				pagination: libHTTP.CursorPagination{},
			},
			expectedStatus: http.StatusOK,
			validateCall: func(t *testing.T, stub *auditServiceStub) {
				assert.Equal(t, time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), stub.gotQuery.StartTime.UTC())
				assert.Equal(t, 2026, stub.gotQuery.EndTime.Year())
				assert.Equal(t, time.February, stub.gotQuery.EndTime.Month())
				assert.Equal(t, 1, stub.gotQuery.EndTime.Day())
				assert.Equal(t, 23, stub.gotQuery.EndTime.Hour(), "date-only end bound normalizes to end-of-day")
			},
		},
		{
			name:  "single-sided start_date alone is accepted with unbounded end",
			query: "?start_date=2026-01-01",
			stub: &auditServiceStub{
				events:     []*mmodel.ProtectionAuditEvent{},
				pagination: libHTTP.CursorPagination{},
			},
			expectedStatus: http.StatusOK,
			validateCall: func(t *testing.T, stub *auditServiceStub) {
				assert.Equal(t, 1, stub.calls, "service must be called for a single-sided start bound")
				assert.Equal(t, time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), stub.gotQuery.StartTime.UTC())
				assert.True(t, stub.gotQuery.EndTime.IsZero(), "end bound stays zero/unbounded when only start_date is set")
			},
		},
		{
			name:  "single-sided end_date alone is accepted with unbounded start",
			query: "?end_date=2026-02-01",
			stub: &auditServiceStub{
				events:     []*mmodel.ProtectionAuditEvent{},
				pagination: libHTTP.CursorPagination{},
			},
			expectedStatus: http.StatusOK,
			validateCall: func(t *testing.T, stub *auditServiceStub) {
				assert.Equal(t, 1, stub.calls, "service must be called for a single-sided end bound")
				assert.True(t, stub.gotQuery.StartTime.IsZero(), "start bound stays zero/unbounded when only end_date is set")
				assert.Equal(t, 23, stub.gotQuery.EndTime.Hour(), "date-only end bound normalizes to end-of-day")
			},
		},
		{
			name:  "absent date bounds stay zero/unbounded",
			query: "",
			stub: &auditServiceStub{
				events:     []*mmodel.ProtectionAuditEvent{},
				pagination: libHTTP.CursorPagination{},
			},
			expectedStatus: http.StatusOK,
			validateCall: func(t *testing.T, stub *auditServiceStub) {
				assert.True(t, stub.gotQuery.StartTime.IsZero(), "absent start_date stays zero/unbounded")
				assert.True(t, stub.gotQuery.EndTime.IsZero(), "absent end_date stays zero/unbounded")
			},
		},
		{
			name:  "filters action/actor/outcome are forwarded to query",
			query: "?action=provision&actor=admin&outcome=success",
			stub: &auditServiceStub{
				events:     []*mmodel.ProtectionAuditEvent{},
				pagination: libHTTP.CursorPagination{},
			},
			expectedStatus: http.StatusOK,
			validateCall: func(t *testing.T, stub *auditServiceStub) {
				assert.Equal(t, "provision", stub.gotQuery.Action)
				assert.Equal(t, "admin", stub.gotQuery.Actor)
				assert.Equal(t, "success", stub.gotQuery.Outcome)
			},
		},
		{
			name:  "RFC3339 date range is parsed and forwarded",
			query: "?start_date=2026-01-01T00:00:00Z&end_date=2026-02-01T00:00:00Z",
			stub: &auditServiceStub{
				events:     []*mmodel.ProtectionAuditEvent{},
				pagination: libHTTP.CursorPagination{},
			},
			expectedStatus: http.StatusOK,
			validateCall: func(t *testing.T, stub *auditServiceStub) {
				assert.Equal(t, time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), stub.gotQuery.StartTime.UTC())
				assert.Equal(t, time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC), stub.gotQuery.EndTime.UTC())
			},
		},
		{
			name:           "invalid outcome returns 400 before service",
			query:          "?outcome=bogus",
			stub:           &auditServiceStub{},
			expectedStatus: http.StatusBadRequest,
			expectedCode:   constant.ErrInvalidQueryParameter.Error(),
			validateCall: func(t *testing.T, stub *auditServiceStub) {
				assert.Equal(t, 0, stub.calls, "service must not be called for invalid outcome")
			},
		},
		{
			name:           "unsupported outcome not_found returns 400 before service",
			query:          "?outcome=not_found",
			stub:           &auditServiceStub{},
			expectedStatus: http.StatusBadRequest,
			expectedCode:   constant.ErrInvalidQueryParameter.Error(),
			validateCall: func(t *testing.T, stub *auditServiceStub) {
				assert.Equal(t, 0, stub.calls)
			},
		},
		{
			name:           "unparseable start_date returns 400 before service",
			query:          "?start_date=not-a-date",
			stub:           &auditServiceStub{},
			expectedStatus: http.StatusBadRequest,
			expectedCode:   constant.ErrInvalidQueryParameter.Error(),
			validateCall: func(t *testing.T, stub *auditServiceStub) {
				assert.Equal(t, 0, stub.calls)
			},
		},
		{
			name:           "unparseable end_date returns 400 before service",
			query:          "?end_date=not-a-date",
			stub:           &auditServiceStub{},
			expectedStatus: http.StatusBadRequest,
			expectedCode:   constant.ErrInvalidQueryParameter.Error(),
			validateCall: func(t *testing.T, stub *auditServiceStub) {
				assert.Equal(t, 0, stub.calls)
			},
		},
		{
			name:           "inverted date range returns 400 before service",
			query:          "?start_date=2026-02-01&end_date=2026-01-01",
			stub:           &auditServiceStub{},
			expectedStatus: http.StatusBadRequest,
			expectedCode:   constant.ErrInvalidQueryParameter.Error(),
			validateCall: func(t *testing.T, stub *auditServiceStub) {
				assert.Equal(t, 0, stub.calls, "service must not be called for an inverted range")
			},
		},
		{
			name:           "malformed cursor rejected by ValidateParameters returns 400",
			query:          "?cursor=not-a-valid-token",
			stub:           &auditServiceStub{},
			expectedStatus: http.StatusBadRequest,
			validateCall: func(t *testing.T, stub *auditServiceStub) {
				assert.Equal(t, 0, stub.calls, "service must not be called when ValidateParameters rejects the cursor")
			},
		},
		{
			name:           "generic service error maps to 500",
			query:          "",
			stub:           &auditServiceStub{err: errors.New("mongo unavailable")},
			expectedStatus: http.StatusInternalServerError,
			validateCall: func(t *testing.T, stub *auditServiceStub) {
				assert.Equal(t, 1, stub.calls)
			},
		},
		{
			// A well-formed cursor passes ValidateParameters; the service then rejects
			// it with libHTTP.ErrInvalidCursor, exercising the core's cursor branch.
			name:           "invalid cursor from service maps to 400",
			query:          "?cursor=" + validCursorToken(t),
			stub:           &auditServiceStub{err: libHTTP.ErrInvalidCursor},
			expectedStatus: http.StatusBadRequest,
			expectedCode:   constant.ErrInvalidQueryParameter.Error(),
			validateCall: func(t *testing.T, stub *auditServiceStub) {
				assert.Equal(t, 1, stub.calls)
			},
		},
		{
			name:  "empty result returns items array and no cursors",
			query: "",
			stub: &auditServiceStub{
				events:     []*mmodel.ProtectionAuditEvent{},
				pagination: libHTTP.CursorPagination{},
			},
			expectedStatus: http.StatusOK,
			validateBody: func(t *testing.T, body []byte) {
				var env map[string]any
				require.NoError(t, json.Unmarshal(body, &env))

				items, ok := env["items"].([]any)
				require.True(t, ok, "items should be a (possibly empty) array, not null")
				assert.Empty(t, items)

				_, hasNext := env["next_cursor"]
				_, hasPrev := env["prev_cursor"]
				assert.False(t, hasNext, "next_cursor should be omitted when empty")
				assert.False(t, hasPrev, "prev_cursor should be omitted when empty")
			},
		},
		{
			name:  "nil details yields empty status strings",
			query: "",
			stub: &auditServiceStub{
				events: []*mmodel.ProtectionAuditEvent{
					{
						ID:             uuid.Must(libCommons.GenerateUUIDv7()),
						OrganizationID: orgID.String(),
						Action:         mmodel.AuditActionProvision,
						Outcome:        mmodel.AuditOutcomeSuccess,
						ActorID:        "svc",
						Timestamp:      ts,
						RequestID:      "req-9",
						Details:        nil,
					},
				},
				pagination: libHTTP.CursorPagination{},
			},
			expectedStatus: http.StatusOK,
			validateBody: func(t *testing.T, body []byte) {
				var env map[string]any
				require.NoError(t, json.Unmarshal(body, &env))

				items, ok := env["items"].([]any)
				require.True(t, ok, "items should be an array, body: %s", string(body))
				require.Len(t, items, 1)

				item, ok := items[0].(map[string]any)
				require.True(t, ok, "item should be an object, body: %s", string(body))
				assert.Equal(t, "", item["from_status"])
				assert.Equal(t, "", item["to_status"])
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler := &AuditHandler{Service: tt.stub}
			app := buildHumaAuditApp(t, handler, true)

			req := httptest.NewRequest(http.MethodGet,
				"/v2/organizations/"+orgID.String()+"/protection/audit"+tt.query, nil)

			resp, err := app.Test(req, fiber.TestConfig{Timeout: 0})
			require.NoError(t, err)
			defer func() { _ = resp.Body.Close() }()

			body, err := io.ReadAll(resp.Body)
			require.NoError(t, err)

			assert.Equal(t, tt.expectedStatus, resp.StatusCode, "body: %s", string(body))

			if tt.expectedCode != "" {
				var got map[string]any
				require.NoError(t, json.Unmarshal(body, &got), "body: %s", string(body))
				assert.Equal(t, tt.expectedCode, got["code"])
			}

			if tt.validateBody != nil {
				tt.validateBody(t, body)
			}

			if tt.validateCall != nil {
				tt.validateCall(t, tt.stub)
			}
		})
	}
}

// TestGetAuditEvents_InternalFieldsExcluded pins the response projection: the
// envelope carries only the public audit fields, never the internal-only ones.
func TestGetAuditEvents_InternalFieldsExcluded(t *testing.T) {
	// NOT parallel: process-global huma state.
	orgID := uuid.Must(libCommons.GenerateUUIDv7())
	ts := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)

	stub := &auditServiceStub{
		events: []*mmodel.ProtectionAuditEvent{
			{
				ID:             uuid.MustParse("11111111-2222-3333-4444-555555555555"),
				TenantID:       "tenant-secret",
				OrganizationID: orgID.String(),
				EventType:      mmodel.AuditEventTypeProvisioning,
				Action:         mmodel.AuditActionProvision,
				Outcome:        mmodel.AuditOutcomeSuccess,
				ActorID:        "admin@example.com",
				ActorType:      "user",
				Reason:         "initial setup",
				Timestamp:      ts,
				RequestID:      "req-123",
				Details: &mmodel.AuditDetails{
					PreviousStatus:    "PENDING",
					NewStatus:         "ACTIVE",
					AffectedKeyIDs:    []uint32{1, 2, 3},
					ProviderReference: "vault://secret/ref",
					ErrorCode:         "NONE",
				},
			},
		},
		pagination: libHTTP.CursorPagination{},
	}

	handler := &AuditHandler{Service: stub}
	app := buildHumaAuditApp(t, handler, true)

	req := httptest.NewRequest(http.MethodGet, "/v2/organizations/"+orgID.String()+"/protection/audit", nil)

	resp, err := app.Test(req, fiber.TestConfig{Timeout: 0})
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	body, _ := io.ReadAll(resp.Body)
	raw := string(body)

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	for _, forbidden := range []string{
		"tenant-secret",
		"vault://secret/ref",
		"provisioning",
		"actor_type",
		"affected_key",
		"provider_reference",
		"error_code",
		"event_type",
	} {
		assert.NotContains(t, raw, forbidden, "internal-only field %q must not be rendered", forbidden)
	}

	var env map[string]any
	require.NoError(t, json.Unmarshal(body, &env), "body: %s", raw)

	items, ok := env["items"].([]any)
	require.True(t, ok, "items should be an array, body: %s", raw)
	require.Len(t, items, 1)

	item, ok := items[0].(map[string]any)
	require.True(t, ok, "item should be an object, body: %s", raw)
	assert.Equal(t, "11111111-2222-3333-4444-555555555555", item["id"])
	assert.Equal(t, "admin@example.com", item["actor"])
	assert.Equal(t, "success", item["outcome"])
	assert.Equal(t, "initial setup", item["reason"])
	assert.Equal(t, "req-123", item["request_id"])
	assert.Equal(t, ts.Format(time.RFC3339), item["timestamp"])
}

// validCursorToken returns a well-formed opaque cursor token that passes
// http.ValidateParameters, so tests can reach the service layer.
func validCursorToken(t *testing.T) string {
	t.Helper()

	token, err := libHTTP.EncodeCursor(libHTTP.Cursor{ID: "some-id", Direction: libHTTP.CursorDirectionNext})
	require.NoError(t, err)

	return token
}

// auditServiceStub implements encryption.AuditQueryService and records the call it
// received so tests can assert the query the core assembled.
type auditServiceStub struct {
	events     []*mmodel.ProtectionAuditEvent
	pagination libHTTP.CursorPagination
	err        error

	calls    int
	gotOrgID string
	gotQuery audit.AuditQuery
}

func (s *auditServiceStub) GetAuditEvents(_ context.Context, organizationID string, query audit.AuditQuery) ([]*mmodel.ProtectionAuditEvent, libHTTP.CursorPagination, error) {
	s.calls++
	s.gotOrgID = organizationID
	s.gotQuery = query

	return s.events, s.pagination, s.err
}
