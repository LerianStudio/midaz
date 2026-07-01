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
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/mock/gomock"

	"github.com/LerianStudio/midaz/v4/components/tracer/internal/adapters/http/in/middleware"
	"github.com/LerianStudio/midaz/v4/components/tracer/internal/services/command"
	"github.com/LerianStudio/midaz/v4/components/tracer/internal/testutil"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/model"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	pkgHTTP "github.com/LerianStudio/midaz/v4/pkg/net/http"
)

// tenantSpyService is a RuleService stub that records the tenant ID it sees on
// its incoming context. It is the ctx-threading probe: the tenant middleware
// writes the tenant into c.UserContext(), the humafiber v2 adapter builds the
// Huma handler ctx from c.UserContext(), the handler passes that ctx to the
// service — so a non-empty capturedTenant proves the whole chain end to end
// without a ctx bridge.
type tenantSpyService struct {
	capturedTenant string
	createResult   *model.Rule
	createErr      error
	getResult      *model.Rule
	getErr         error
}

func (s *tenantSpyService) CreateRule(ctx context.Context, _ *command.CreateRuleInput) (*model.Rule, error) {
	s.capturedTenant = tmctx.GetTenantIDContext(ctx)
	return s.createResult, s.createErr
}

func (s *tenantSpyService) GetRule(ctx context.Context, _ uuid.UUID) (*model.Rule, error) {
	s.capturedTenant = tmctx.GetTenantIDContext(ctx)
	return s.getResult, s.getErr
}

func (s *tenantSpyService) UpdateRule(context.Context, uuid.UUID, *command.UpdateRuleInput) (*model.Rule, error) {
	return nil, nil
}
func (s *tenantSpyService) ListRules(context.Context, *model.ListRulesFilter) (*model.ListRulesResult, error) {
	return nil, nil
}
func (s *tenantSpyService) ActivateRule(context.Context, uuid.UUID) (*model.Rule, error) {
	return nil, nil
}
func (s *tenantSpyService) DeactivateRule(context.Context, uuid.UUID) (*model.Rule, error) {
	return nil, nil
}
func (s *tenantSpyService) DraftRule(context.Context, uuid.UUID) (*model.Rule, error) {
	return nil, nil
}
func (s *tenantSpyService) DeleteRule(context.Context, uuid.UUID) error { return nil }

// buildHumaRuleApp mounts the CreateRule/GetRule Huma routes on a /v1 group that
// carries a tenant-injecting middleware, faithfully mirroring the production
// wiring in routes.go: problem.Install() runs before any Register, the Huma API
// is built with openapi.New over the SAME /v1 group that holds the tenant MW,
// and RegisterRuleRoutes registers the two reference ops. The injected tenant
// stands in for tmmiddleware (which needs a live Tenant Manager); it uses the
// identical c.SetUserContext(tmctx.ContextWithTenantID(...)) mechanism the real
// middleware uses at tenant.go:184/225.
func buildHumaRuleApp(t *testing.T, svc RuleService, tenantID string) *fiber.App {
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

	h := NewHandler(svc)
	RegisterRuleRoutes(hAPI, h)

	return f
}

func TestHuma_CreateRule_Success(t *testing.T) {
	t.Parallel()

	rule := &model.Rule{
		ID:         testutil.MustDeterministicUUID(1),
		Name:       "Test Rule",
		Expression: "amount > 1000",
		Action:     model.DecisionDeny,
		Status:     model.RuleStatusDraft,
		CreatedAt:  testutil.FixedTime(),
		UpdatedAt:  testutil.FixedTime(),
	}
	svc := &tenantSpyService{createResult: rule}

	app := buildHumaRuleApp(t, svc, "tenant-alpha")

	body, err := json.Marshal(map[string]any{
		"name":       "Test Rule",
		"expression": "amount > 1000",
		"action":     "DENY",
	})
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, "/v1/rules", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req, -1)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	respBody, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	assert.Equal(t, http.StatusCreated, resp.StatusCode, "CreateRule must return 201 through Huma")

	var got map[string]any
	require.NoError(t, json.Unmarshal(respBody, &got), "body must be JSON: %s", string(respBody))
	assert.Equal(t, "Test Rule", got["name"])
	assert.Equal(t, "DENY", got["action"])
	assert.Equal(t, "DRAFT", got["status"])
	// ruleId is the model.Rule json tag for ID — proves body is model.Rule verbatim.
	assert.Equal(t, testutil.MustDeterministicUUID(1).String(), got["ruleId"])

	// ctxThreadingProof: the tenant the MW put on c.UserContext() reached the
	// service through the Huma handler ctx — no bridge.
	assert.Equal(t, "tenant-alpha", svc.capturedTenant,
		"tenant from c.UserContext() must reach the service via the Huma handler ctx")
}

func TestHuma_GetRule_Success(t *testing.T) {
	t.Parallel()

	id := testutil.MustDeterministicUUID(7)
	rule := &model.Rule{
		ID:         id,
		Name:       "Fetched Rule",
		Expression: "amount > 500",
		Action:     model.DecisionReview,
		Status:     model.RuleStatusActive,
		CreatedAt:  testutil.FixedTime(),
		UpdatedAt:  testutil.FixedTime(),
	}
	svc := &tenantSpyService{getResult: rule}

	app := buildHumaRuleApp(t, svc, "tenant-beta")

	req := httptest.NewRequest(http.MethodGet, "/v1/rules/"+id.String(), nil)

	resp, err := app.Test(req, -1)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	respBody, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	assert.Equal(t, http.StatusOK, resp.StatusCode, "GetRule must return 200 through Huma")

	var got map[string]any
	require.NoError(t, json.Unmarshal(respBody, &got), "body must be JSON: %s", string(respBody))
	assert.Equal(t, "Fetched Rule", got["name"])
	assert.Equal(t, id.String(), got["ruleId"])

	assert.Equal(t, "tenant-beta", svc.capturedTenant,
		"tenant from c.UserContext() must reach the service via the Huma handler ctx")
}

// TestHuma_CreateRule_ValidationError asserts the RFC 9457 error contract is
// preserved byte-for-byte when the imperative CreateRuleInput.Validate() fails:
// same code, same 400 status, same problem+json shape (type/title/detail/code),
// NOT a native Huma 422. The service must never be reached.
func TestHuma_CreateRule_ValidationError(t *testing.T) {
	t.Parallel()

	svc := &tenantSpyService{}
	app := buildHumaRuleApp(t, svc, "tenant-gamma")

	// Missing required "name" -> imperative Validate() -> ErrRuleNameRequired
	// (0353) / 400. This is the EXACT code the pre-Huma Fiber handler produced;
	// the migration must not introduce a new native Huma 400/422 for the body.
	body, err := json.Marshal(map[string]any{
		"expression": "amount > 1000",
		"action":     "DENY",
	})
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, "/v1/rules", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req, -1)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	respBody, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	assert.Equal(t, http.StatusBadRequest, resp.StatusCode,
		"imperative validation stays 400 — no new native Huma 422")
	assert.Equal(t, "application/problem+json", resp.Header.Get("Content-Type"),
		"error must carry the RFC 9457 media type")

	var got map[string]any
	require.NoError(t, json.Unmarshal(respBody, &got), "body must be JSON: %s", string(respBody))
	assert.Equal(t, "0353", got["code"], "code must be the canonical registry code, byte-identical to the Fiber path")
	assert.Equal(t, float64(http.StatusBadRequest), got["status"], "RFC 9457 status field")
	assert.Equal(t, libProblem.BaseURI+"/0353", got["type"], "RFC 9457 type is BaseURI/code")
	assert.NotEmpty(t, got["title"], "RFC 9457 title present")

	assert.Empty(t, svc.capturedTenant, "service must not be reached on a validation error")
}

// TestHuma_GetRule_BadUUID pins the malformed-path-param contract: a non-UUID
// {id} must reach the handler's imperative uuid.Parse and produce the canonical
// 400 / code 0065 (ErrInvalidPathParameter) / entityType Rule — NOT a native
// Huma 422 fired before the handler. Path params can't be SkipValidate'd, so a
// `format:"uuid"` struct tag would let Huma reject the id first and diverge from
// the Fiber envelope. Reference pattern for all 28 by-id handlers in the 2b
// fan-out: NO format/struct-tag validation on path params.
func TestHuma_GetRule_BadUUID(t *testing.T) {
	t.Parallel()

	svc := &tenantSpyService{}
	app := buildHumaRuleApp(t, svc, "tenant-epsilon")

	req := httptest.NewRequest(http.MethodGet, "/v1/rules/not-a-uuid", nil)
	resp, err := app.Test(req, -1)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	respBody, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	assert.Equal(t, http.StatusBadRequest, resp.StatusCode,
		"malformed path UUID must be the canonical 400 — no native Huma 422")
	assert.Equal(t, "application/problem+json", resp.Header.Get("Content-Type"),
		"error must carry the RFC 9457 media type")

	var got map[string]any
	require.NoError(t, json.Unmarshal(respBody, &got), "body must be JSON: %s", string(respBody))
	assert.Equal(t, "0065", got["code"], "code must be ErrInvalidPathParameter, byte-identical to the Fiber path")
	assert.Equal(t, float64(http.StatusBadRequest), got["status"], "RFC 9457 status field")
	assert.Equal(t, libProblem.BaseURI+"/0065", got["type"], "RFC 9457 type is BaseURI/code")
	assert.Equal(t, "Rule", got["entityType"], "canonical entityType from ErrInvalidPathParameter")

	assert.Empty(t, svc.capturedTenant, "service must not be reached on a bad path param")
}

// TestHuma_CreateRule_MalformedJSON pins the SkipValidateBody + RawBody contract:
// a syntactically broken JSON body must reach the handler's imperative
// json.Unmarshal and produce the canonical 400 / code 0094 (ErrInvalidRequestBody),
// NOT a native Huma 400/422 fired by body-schema validation. This is the entire
// justification for SkipValidateBody:true + RawBody; without a test any tag/config
// change re-enabling native body validation regresses silently across 28 handlers.
func TestHuma_CreateRule_MalformedJSON(t *testing.T) {
	t.Parallel()

	svc := &tenantSpyService{}
	app := buildHumaRuleApp(t, svc, "tenant-zeta")

	req := httptest.NewRequest(http.MethodPost, "/v1/rules", bytes.NewReader([]byte("{not json")))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req, -1)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	respBody, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	assert.Equal(t, http.StatusBadRequest, resp.StatusCode,
		"malformed JSON must be the canonical 400 — no native Huma body validation")
	assert.Equal(t, "application/problem+json", resp.Header.Get("Content-Type"),
		"error must carry the RFC 9457 media type")

	var got map[string]any
	require.NoError(t, json.Unmarshal(respBody, &got), "body must be JSON: %s", string(respBody))
	assert.Equal(t, "0094", got["code"], "code must be ErrInvalidRequestBody, byte-identical to the Fiber path")
	assert.Equal(t, float64(http.StatusBadRequest), got["status"], "RFC 9457 status field")
	assert.Equal(t, libProblem.BaseURI+"/0094", got["type"], "RFC 9457 type is BaseURI/code")

	assert.Empty(t, svc.capturedTenant, "service must not be reached on malformed JSON")
}

// TestHuma_ErrorBodyMatchesFiberEnvelope pins byte-identity of the error body:
// the same domain error rendered through the legacy Fiber http.WithError path
// and through the migrated Huma handler must produce the identical JSON body.
func TestHuma_ErrorBodyMatchesFiberEnvelope(t *testing.T) {
	t.Parallel()

	// Drive the migrated Huma handler with a not-found from the service.
	svc := &tenantSpyService{getErr: constant.ErrRuleNotFound}
	app := buildHumaRuleApp(t, svc, "tenant-delta")

	id := testutil.MustDeterministicUUID(3)
	req := httptest.NewRequest(http.MethodGet, "/v1/rules/"+id.String(), nil)
	resp, err := app.Test(req, -1)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	humaBody, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	// Reference: the same error through the frozen Fiber envelope.
	ref := fiber.New(fiber.Config{ErrorHandler: pkgHTTP.CanonicalFiberErrorHandler})
	ref.Get("/probe", func(c *fiber.Ctx) error {
		return handleServiceError(c, trace.SpanFromContext(c.UserContext()), constant.ErrRuleNotFound)
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
	assert.Equal(t, refJSON, humaJSON, "Huma error body must be byte-identical to the Fiber envelope")
}

// TestHuma_MigratedRoutes_AuthStillEnforced proves, through the REAL NewRoutes
// wiring, that the auth guard remains a Fiber middleware in front of the
// migrated Huma routes: unauthenticated POST /v1/rules and GET /v1/rules/{id}
// still return 401 (guard runs, short-circuits before the Huma handler), while
// a valid API key lets the request reach the Huma handler (mock is invoked).
// This is the end-to-end proof that mounting Huma on the /v1 group did not
// bypass the guard chain.
func TestHuma_MigratedRoutes_AuthStillEnforced(t *testing.T) {
	guardCfg := middleware.AuthGuardConfig{
		APIKey:        testAPIKey,
		APIKeyEnabled: true,
		AppName:       "tracer",
	}

	t.Run("unauthenticated migrated routes -> 401", func(t *testing.T) {
		for _, tc := range []struct {
			method, path string
		}{
			{http.MethodPost, "/v1/rules"},
			{http.MethodGet, "/v1/rules/" + testutil.MustDeterministicUUID(1).String()},
		} {
			app := createTestRouter(t, guardCfg)
			req := httptest.NewRequest(tc.method, tc.path, nil)
			resp, err := app.Test(req, -1)
			require.NoError(t, err)
			require.NoError(t, resp.Body.Close())

			assert.Equal(t, http.StatusUnauthorized, resp.StatusCode,
				"migrated Huma route %s %s must be guarded by the Fiber auth middleware", tc.method, tc.path)
		}
	})

	t.Run("authenticated GET reaches the Huma handler", func(t *testing.T) {
		id := testutil.MustDeterministicUUID(9)
		deps := newTestRouterDeps(t, guardCfg)
		deps.RuleService.EXPECT().
			GetRule(gomock.Any(), id).
			Return(&model.Rule{ID: id, Name: "Reached", Action: model.DecisionDeny, Status: model.RuleStatusActive}, nil).
			Times(1)

		app := deps.build()
		req := httptest.NewRequest(http.MethodGet, "/v1/rules/"+id.String(), nil)
		req.Header.Set("X-API-Key", testAPIKey)

		resp, err := app.Test(req, -1)
		require.NoError(t, err)
		defer func() { _ = resp.Body.Close() }()

		assert.Equal(t, http.StatusOK, resp.StatusCode,
			"authenticated request must clear the guard and reach the migrated Huma handler")
	})
}
