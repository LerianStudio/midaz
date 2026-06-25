// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http/httptest"
	"testing"
	"time"

	libHTTP "github.com/LerianStudio/lib-commons/v5/commons/net/http"
	"github.com/LerianStudio/midaz/v3/components/crm/internal/adapters/mongodb/audit"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// validCursorToken returns a well-formed opaque cursor token that passes
// http.ValidateParameters, so tests can reach the service layer.
func validCursorToken(t *testing.T) string {
	t.Helper()

	token, err := libHTTP.EncodeCursor(libHTTP.Cursor{ID: "some-id", Direction: libHTTP.CursorDirectionNext})
	require.NoError(t, err)

	return token
}

// newAuditTestApp wires a Fiber app that injects organization_id into locals
// (mirroring encryption_test.go) and registers the audit handler at a GET route.
func newAuditTestApp(handler *AuditHandler, orgID string) *fiber.App {
	app := fiber.New()
	app.Get("/v1/organizations/:organization_id/protection/audit",
		func(c *fiber.Ctx) error {
			c.Locals("organization_id", uuid.MustParse(orgID))
			return c.Next()
		},
		handler.GetAuditEvents,
	)
	return app
}

func TestAuditHandler_GetAuditEvents(t *testing.T) {
	orgID := uuid.New().String()
	ts := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	fixedEventID := uuid.MustParse("11111111-2222-3333-4444-555555555555")

	tests := []struct {
		name           string
		query          string
		fake           *auditServiceStub
		expectedStatus int
		validateBody   func(t *testing.T, body []byte)
		validateCall   func(t *testing.T, stub *auditServiceStub)
	}{
		{
			name:  "happy path returns envelope with lifted statuses and cursors",
			query: "?limit=2&sort_order=desc",
			fake: &auditServiceStub{
				events: []*mmodel.ProtectionAuditEvent{
					{
						ID:             fixedEventID,
						TenantID:       "tenant-secret",
						OrganizationID: orgID,
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
				pagination: libHTTP.CursorPagination{Next: "next-token", Prev: "prev-token"},
			},
			expectedStatus: 200,
			validateBody: func(t *testing.T, body []byte) {
				var env map[string]any
				require.NoError(t, json.Unmarshal(body, &env))

				assert.Equal(t, orgID, env["organization_id"])
				assert.EqualValues(t, 2, env["limit"])
				assert.Equal(t, "next-token", env["next_cursor"])
				assert.Equal(t, "prev-token", env["prev_cursor"])

				items, ok := env["items"].([]any)
				require.True(t, ok, "items should be an array")
				require.Len(t, items, 1)

				item := items[0].(map[string]any)
				assert.Equal(t, "provision", item["action"])
				assert.Equal(t, "admin@example.com", item["actor"])
				assert.Equal(t, "success", item["outcome"])
				assert.Equal(t, "initial setup", item["reason"])
				assert.Equal(t, "PENDING", item["from_status"])
				assert.Equal(t, "ACTIVE", item["to_status"])
				assert.Equal(t, "req-123", item["request_id"])
				assert.Equal(t, ts.Format(time.RFC3339), item["timestamp"])
				assert.Equal(t, "11111111-2222-3333-4444-555555555555", item["id"])

				// Internal-only fields MUST be excluded.
				raw := string(body)
				assert.NotContains(t, raw, "tenant-secret")
				assert.NotContains(t, raw, "vault://secret/ref")
				assert.NotContains(t, raw, "provisioning") // event_type
				assert.NotContains(t, raw, "actor_type")
				assert.NotContains(t, raw, "affected_key")
				assert.NotContains(t, raw, "provider_reference")
				assert.NotContains(t, raw, "error_code")
				assert.NotContains(t, raw, "event_type")
			},
			validateCall: func(t *testing.T, stub *auditServiceStub) {
				assert.Equal(t, 1, stub.calls)
				assert.Equal(t, orgID, stub.gotOrgID)
				assert.Equal(t, 2, stub.gotQuery.Limit)
				assert.Equal(t, "desc", stub.gotQuery.SortOrder)
			},
		},
		{
			name:  "absent limit and sort_order default to 20 and desc",
			query: "",
			fake: &auditServiceStub{
				events:     []*mmodel.ProtectionAuditEvent{},
				pagination: libHTTP.CursorPagination{},
			},
			expectedStatus: 200,
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
			fake: &auditServiceStub{
				events:     []*mmodel.ProtectionAuditEvent{},
				pagination: libHTTP.CursorPagination{},
			},
			expectedStatus: 200,
			validateCall: func(t *testing.T, stub *auditServiceStub) {
				// start_date normalizes to start-of-day, end_date to end-of-day.
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
			fake: &auditServiceStub{
				events:     []*mmodel.ProtectionAuditEvent{},
				pagination: libHTTP.CursorPagination{},
			},
			expectedStatus: 200,
			validateCall: func(t *testing.T, stub *auditServiceStub) {
				assert.Equal(t, 1, stub.calls, "service must be called for a single-sided start bound")
				assert.Equal(t, time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), stub.gotQuery.StartTime.UTC())
				assert.True(t, stub.gotQuery.EndTime.IsZero(), "end bound stays zero/unbounded when only start_date is set")
			},
		},
		{
			name:  "single-sided end_date alone is accepted with unbounded start",
			query: "?end_date=2026-02-01",
			fake: &auditServiceStub{
				events:     []*mmodel.ProtectionAuditEvent{},
				pagination: libHTTP.CursorPagination{},
			},
			expectedStatus: 200,
			validateCall: func(t *testing.T, stub *auditServiceStub) {
				assert.Equal(t, 1, stub.calls, "service must be called for a single-sided end bound")
				assert.True(t, stub.gotQuery.StartTime.IsZero(), "start bound stays zero/unbounded when only end_date is set")
				assert.Equal(t, 2026, stub.gotQuery.EndTime.Year())
				assert.Equal(t, time.February, stub.gotQuery.EndTime.Month())
				assert.Equal(t, 1, stub.gotQuery.EndTime.Day())
				assert.Equal(t, 23, stub.gotQuery.EndTime.Hour(), "date-only end bound normalizes to end-of-day")
			},
		},
		{
			name:  "absent date bounds stay zero/unbounded",
			query: "",
			fake: &auditServiceStub{
				events:     []*mmodel.ProtectionAuditEvent{},
				pagination: libHTTP.CursorPagination{},
			},
			expectedStatus: 200,
			validateCall: func(t *testing.T, stub *auditServiceStub) {
				assert.True(t, stub.gotQuery.StartTime.IsZero(), "absent start_date stays zero/unbounded")
				assert.True(t, stub.gotQuery.EndTime.IsZero(), "absent end_date stays zero/unbounded")
			},
		},
		{
			name:  "filters action/actor/outcome are forwarded to query",
			query: "?action=provision&actor=admin&outcome=success",
			fake: &auditServiceStub{
				events:     []*mmodel.ProtectionAuditEvent{},
				pagination: libHTTP.CursorPagination{},
			},
			expectedStatus: 200,
			validateCall: func(t *testing.T, stub *auditServiceStub) {
				assert.Equal(t, "provision", stub.gotQuery.Action)
				assert.Equal(t, "admin", stub.gotQuery.Actor)
				assert.Equal(t, "success", stub.gotQuery.Outcome)
			},
		},
		{
			name:  "valid date range is parsed and forwarded",
			query: "?start_date=2026-01-01T00:00:00Z&end_date=2026-02-01T00:00:00Z",
			fake: &auditServiceStub{
				events:     []*mmodel.ProtectionAuditEvent{},
				pagination: libHTTP.CursorPagination{},
			},
			expectedStatus: 200,
			validateCall: func(t *testing.T, stub *auditServiceStub) {
				assert.Equal(t, time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), stub.gotQuery.StartTime.UTC())
				assert.Equal(t, time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC), stub.gotQuery.EndTime.UTC())
			},
		},
		{
			name:           "invalid outcome returns 400 before service",
			query:          "?outcome=bogus",
			fake:           &auditServiceStub{},
			expectedStatus: 400,
			validateCall: func(t *testing.T, stub *auditServiceStub) {
				assert.Equal(t, 0, stub.calls, "service must not be called for invalid outcome")
			},
		},
		{
			name:           "deferred outcome conflict returns 400 before service",
			query:          "?outcome=conflict",
			fake:           &auditServiceStub{},
			expectedStatus: 400,
			validateCall: func(t *testing.T, stub *auditServiceStub) {
				assert.Equal(t, 0, stub.calls)
			},
		},
		{
			name:           "deferred outcome not_found returns 400 before service",
			query:          "?outcome=not_found",
			fake:           &auditServiceStub{},
			expectedStatus: 400,
			validateCall: func(t *testing.T, stub *auditServiceStub) {
				assert.Equal(t, 0, stub.calls)
			},
		},
		{
			name:           "unparseable start_date returns 400 before service",
			query:          "?start_date=not-a-date",
			fake:           &auditServiceStub{},
			expectedStatus: 400,
			validateCall: func(t *testing.T, stub *auditServiceStub) {
				assert.Equal(t, 0, stub.calls)
			},
		},
		{
			name:           "unparseable end_date returns 400 before service",
			query:          "?end_date=not-a-date",
			fake:           &auditServiceStub{},
			expectedStatus: 400,
			validateCall: func(t *testing.T, stub *auditServiceStub) {
				assert.Equal(t, 0, stub.calls)
			},
		},
		{
			name:           "malformed cursor rejected by ValidateParameters returns 400",
			query:          "?cursor=not-a-valid-token",
			fake:           &auditServiceStub{},
			expectedStatus: 400,
			validateCall: func(t *testing.T, stub *auditServiceStub) {
				assert.Equal(t, 0, stub.calls, "service must not be called when ValidateParameters rejects the cursor")
			},
		},
		{
			name:  "generic service error maps to 500",
			query: "",
			fake: &auditServiceStub{
				err: errors.New("mongo unavailable"),
			},
			expectedStatus: 500,
			validateCall: func(t *testing.T, stub *auditServiceStub) {
				assert.Equal(t, 1, stub.calls)
			},
		},
		{
			name: "invalid cursor from service maps to 400",
			// A well-formed cursor passes ValidateParameters; the service then
			// rejects it with libHTTP.ErrInvalidCursor, exercising the handler's
			// cursor error branch.
			query: "?cursor=" + validCursorToken(t),
			fake: &auditServiceStub{
				err: libHTTP.ErrInvalidCursor,
			},
			expectedStatus: 400,
			validateCall: func(t *testing.T, stub *auditServiceStub) {
				assert.Equal(t, 1, stub.calls)
			},
		},
		{
			name:  "empty result returns items array and no cursors",
			query: "",
			fake: &auditServiceStub{
				events:     []*mmodel.ProtectionAuditEvent{},
				pagination: libHTTP.CursorPagination{},
			},
			expectedStatus: 200,
			validateBody: func(t *testing.T, body []byte) {
				var env map[string]any
				require.NoError(t, json.Unmarshal(body, &env))

				items, ok := env["items"].([]any)
				require.True(t, ok, "items should be a (possibly empty) array, not null")
				assert.Len(t, items, 0)

				_, hasNext := env["next_cursor"]
				_, hasPrev := env["prev_cursor"]
				assert.False(t, hasNext, "next_cursor should be omitted when empty")
				assert.False(t, hasPrev, "prev_cursor should be omitted when empty")
			},
		},
		{
			name:  "nil details yields empty status strings",
			query: "",
			fake: &auditServiceStub{
				events: []*mmodel.ProtectionAuditEvent{
					{
						ID:             uuid.New(),
						OrganizationID: orgID,
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
			expectedStatus: 200,
			validateBody: func(t *testing.T, body []byte) {
				var env map[string]any
				require.NoError(t, json.Unmarshal(body, &env))

				items := env["items"].([]any)
				require.Len(t, items, 1)
				item := items[0].(map[string]any)
				assert.Equal(t, "", item["from_status"])
				assert.Equal(t, "", item["to_status"])
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler := &AuditHandler{Service: tt.fake}
			app := newAuditTestApp(handler, orgID)

			req := httptest.NewRequest(fiber.MethodGet,
				"/v1/organizations/"+orgID+"/protection/audit"+tt.query, nil)
			resp, err := app.Test(req)
			require.NoError(t, err)
			defer resp.Body.Close()

			assert.Equal(t, tt.expectedStatus, resp.StatusCode)

			if tt.validateBody != nil {
				body, err := io.ReadAll(resp.Body)
				require.NoError(t, err)
				tt.validateBody(t, body)
			}

			if tt.validateCall != nil {
				tt.validateCall(t, tt.fake)
			}
		})
	}
}

// auditServiceStub implements encryption.AuditQueryService.
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
