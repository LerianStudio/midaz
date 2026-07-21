// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
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

	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	pkgHTTP "github.com/LerianStudio/midaz/v4/pkg/net/http"
)

// buildHumaAuditApp mounts the protection-audit Huma operation on a /v1 group,
// faithfully mirroring the production wiring in unified-server.go: problem.Install()
// runs before any huma.Register, the Huma API is built with openapi.New over a /v1
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
		DisableStartupMessage: true,
		ErrorHandler:          pkgHTTP.CanonicalFiberErrorHandler,
	})

	libProblem.Install()

	apiV1 := f.Group("/v1")

	apiV1.Use(func(c *fiber.Ctx) error {
		if !authOK {
			return pkgHTTP.Unauthorized(c, "0001", "Unauthorized", "auth required")
		}

		return c.Next()
	})

	hAPI := openapi.New(f, apiV1, openapi.Config{Title: "ledger-test", Version: "test", Servers: []string{"/v1"}})

	apiV1.Get("/organizations/:organization_id/protection/audit", pkgHTTP.ParseUUIDPathParameters("organization"))

	RegisterAuditRoutes(hAPI, handler)

	return f
}

func TestHuma_GetAuditEvents_Success(t *testing.T) {
	// NOT parallel: buildHumaAuditApp mutates process-global huma state.
	orgID := uuid.New()
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

	req := httptest.NewRequest(http.MethodGet, "/v1/organizations/"+orgID.String()+"/protection/audit?limit=2&sort_order=desc", nil)

	resp, err := app.Test(req, -1)
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

	items := env["items"].([]any)
	require.Len(t, items, 1)
	item := items[0].(map[string]any)
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

func TestHuma_GetAuditEvents_UnsupportedOutcomeRejectedByCore(t *testing.T) {
	// NOT parallel: process-global huma state. An unsupported outcome filter must be
	// rejected by the core (canonical 400), NOT bound/accepted by Huma.
	orgID := uuid.New()

	stub := &auditServiceStub{}
	handler := &AuditHandler{Service: stub}
	app := buildHumaAuditApp(t, handler, true)

	req := httptest.NewRequest(http.MethodGet, "/v1/organizations/"+orgID.String()+"/protection/audit?outcome=conflict", nil)

	resp, err := app.Test(req, -1)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
	assert.Equal(t, 0, stub.calls, "service must not be called on a rejected filter")
}

func TestHuma_GetAuditEvents_AuthPreserved(t *testing.T) {
	// NOT parallel: process-global huma state.
	orgID := uuid.New()

	stub := &auditServiceStub{}
	handler := &AuditHandler{Service: stub}
	app := buildHumaAuditApp(t, handler, false)

	req := httptest.NewRequest(http.MethodGet, "/v1/organizations/"+orgID.String()+"/protection/audit", nil)

	resp, err := app.Test(req, -1)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
	assert.Equal(t, 0, stub.calls, "service must not be called when auth rejects")
}
