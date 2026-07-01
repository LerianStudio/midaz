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
	"github.com/danielgtaylor/huma/v2"
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

	// lifecycle / update / list / delete results shared by the 2b ops.
	updateResult *model.Rule
	updateErr    error
	listResult   *model.ListRulesResult
	listErr      error
	lifecycle    *model.Rule // returned by activate/deactivate/draft
	lifecycleErr error
	deleteErr    error
	// listFilter captures the filter the core built from the query, so tests
	// can assert imperative binding/defaults produced the same filter the Fiber
	// path would.
	listFilter *model.ListRulesFilter
}

func (s *tenantSpyService) CreateRule(ctx context.Context, _ *command.CreateRuleInput) (*model.Rule, error) {
	s.capturedTenant = tmctx.GetTenantIDContext(ctx)
	return s.createResult, s.createErr
}

func (s *tenantSpyService) GetRule(ctx context.Context, _ uuid.UUID) (*model.Rule, error) {
	s.capturedTenant = tmctx.GetTenantIDContext(ctx)
	return s.getResult, s.getErr
}

func (s *tenantSpyService) UpdateRule(ctx context.Context, _ uuid.UUID, _ *command.UpdateRuleInput) (*model.Rule, error) {
	s.capturedTenant = tmctx.GetTenantIDContext(ctx)
	return s.updateResult, s.updateErr
}

func (s *tenantSpyService) ListRules(ctx context.Context, filter *model.ListRulesFilter) (*model.ListRulesResult, error) {
	s.capturedTenant = tmctx.GetTenantIDContext(ctx)
	s.listFilter = filter
	return s.listResult, s.listErr
}

func (s *tenantSpyService) ActivateRule(ctx context.Context, _ uuid.UUID) (*model.Rule, error) {
	s.capturedTenant = tmctx.GetTenantIDContext(ctx)
	return s.lifecycle, s.lifecycleErr
}

func (s *tenantSpyService) DeactivateRule(ctx context.Context, _ uuid.UUID) (*model.Rule, error) {
	s.capturedTenant = tmctx.GetTenantIDContext(ctx)
	return s.lifecycle, s.lifecycleErr
}

func (s *tenantSpyService) DraftRule(ctx context.Context, _ uuid.UUID) (*model.Rule, error) {
	s.capturedTenant = tmctx.GetTenantIDContext(ctx)
	return s.lifecycle, s.lifecycleErr
}

func (s *tenantSpyService) DeleteRule(ctx context.Context, _ uuid.UUID) error {
	s.capturedTenant = tmctx.GetTenantIDContext(ctx)
	return s.deleteErr
}

// buildHumaRuleApp mounts the CreateRule/GetRule Huma routes on a /v1 group that
// carries a tenant-injecting middleware, faithfully mirroring the production
// wiring in routes.go: problem.Install() runs before any Register, the Huma API
// is built with openapi.New over the SAME /v1 group that holds the tenant MW,
// and RegisterRuleRoutes registers all eight rule ops. The injected tenant
// stands in for tmmiddleware (which needs a live Tenant Manager); it uses the
// identical c.SetUserContext(tmctx.ContextWithTenantID(...)) mechanism the real
// middleware uses at tenant.go:184/225.
//
// MUST-NOT-PARALLELIZE (also applies to every 2b copy of this helper): tests
// that build a huma.API through this function CANNOT call t.Parallel(). Two
// process-global mutations make concurrent builds/requests cross-contaminate:
//   - libProblem.Install() swaps the process-global huma.NewError (the hook
//     Huma uses to render errors). While one API installs it, a concurrent
//     request on another API can render through the wrong hook.
//   - Huma validation uses process-global sync.Pools (huma/v2 huma.go
//     validatePool/bufPool). Concurrent requests share the pooled
//     *ValidateResult; a bad interleaving surfaces a phantom native 422 with a
//     nil code instead of the canonical 400/0065 envelope.
//
// -race does NOT catch this (Get/Put are individually race-safe; the bug is
// logical, not a data race). lib-commons marks its own Install-touching test
// non-parallel for the same reason (commons/net/http/openapi/openapi_test.go).
// Parallelizing buys nothing here — these tests are sub-second — and reopens the
// contamination window across all 28 fan-out copies. Keep them sequential.
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
	// NOT parallel: buildHumaRuleApp mutates process-global huma state
	// (libProblem.Install + Huma validation pools). See buildHumaRuleApp.
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

	// No Huma JSON-Schema hyperlink fields leak into the success body: openapi.New
	// zeroes the SchemaLinkTransformer, so the body is the model.Rule verbatim.
	// Asserted on the raw bytes because a leaked "$schema" would decode into the
	// map too — this is the executable form of the pattern's strongest claim,
	// carried into all 28 fan-out copies.
	assert.NotContains(t, string(respBody), "$schema", "SchemaLinkTransformer must be zeroed — no $schema in body")
	assert.NotContains(t, string(respBody), "$ref")
	assert.NotContains(t, string(respBody), "$id")

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
	// NOT parallel: buildHumaRuleApp mutates process-global huma state
	// (libProblem.Install + Huma validation pools). See buildHumaRuleApp.
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

	// No Huma JSON-Schema hyperlink fields leak into the success body: openapi.New
	// zeroes the SchemaLinkTransformer, so the body is the model.Rule verbatim.
	assert.NotContains(t, string(respBody), "$schema", "SchemaLinkTransformer must be zeroed — no $schema in body")
	assert.NotContains(t, string(respBody), "$ref")
	assert.NotContains(t, string(respBody), "$id")

	var got map[string]any
	require.NoError(t, json.Unmarshal(respBody, &got), "body must be JSON: %s", string(respBody))
	assert.Equal(t, "Fetched Rule", got["name"])
	assert.Equal(t, id.String(), got["ruleId"])

	assert.Equal(t, "tenant-beta", svc.capturedTenant,
		"tenant from c.UserContext() must reach the service via the Huma handler ctx")
}

// TestHuma_CreateRule_ValidationError asserts the RFC 9457 error contract is
// preserved field-for-field when the imperative CreateRuleInput.Validate()
// fails: same code, same 400 status, same problem+json shape
// (type/title/detail/code), NOT a native Huma 422. The service must never be
// reached.
func TestHuma_CreateRule_ValidationError(t *testing.T) {
	// NOT parallel: buildHumaRuleApp mutates process-global huma state
	// (libProblem.Install + Huma validation pools). See buildHumaRuleApp.
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
	assert.Equal(t, "0353", got["code"], "code must be the canonical registry code, identical to the Fiber path")
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
	// NOT parallel: buildHumaRuleApp mutates process-global huma state
	// (libProblem.Install + Huma validation pools). See buildHumaRuleApp.
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
	assert.Equal(t, "0065", got["code"], "code must be ErrInvalidPathParameter, identical to the Fiber path")
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
	// NOT parallel: buildHumaRuleApp mutates process-global huma state
	// (libProblem.Install + Huma validation pools). See buildHumaRuleApp.
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
	assert.Equal(t, "0094", got["code"], "code must be ErrInvalidRequestBody, identical to the Fiber path")
	assert.Equal(t, float64(http.StatusBadRequest), got["status"], "RFC 9457 status field")
	assert.Equal(t, libProblem.BaseURI+"/0094", got["type"], "RFC 9457 type is BaseURI/code")

	assert.Empty(t, svc.capturedTenant, "service must not be reached on malformed JSON")
}

// TestHuma_ErrorBodyMatchesFiberEnvelope pins field-identity of the error body:
// the same domain error rendered through the legacy Fiber http.WithError path
// and through the migrated Huma handler must DECODE to the identical JSON object.
// It compares the decoded maps, not the raw bytes: both share pkgHTTP.ProblemDetail
// so every field matches, but the raw bytes differ by Huma's encoder trailing-'\n'
// + HTML-escaping (invisible to any JSON parser). Byte-alignment is a non-goal.
func TestHuma_ErrorBodyMatchesFiberEnvelope(t *testing.T) {
	// NOT parallel: buildHumaRuleApp mutates process-global huma state
	// (libProblem.Install + Huma validation pools). See buildHumaRuleApp.
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

	// Reference: the same error through the frozen Fiber envelope. This mirrors
	// what every rule Fiber wrapper now does — render classifyServiceError's
	// canonical error through pkgHTTP.WithError.
	ref := fiber.New(fiber.Config{ErrorHandler: pkgHTTP.CanonicalFiberErrorHandler})
	ref.Get("/probe", func(c *fiber.Ctx) error {
		return pkgHTTP.WithError(c, classifyServiceError(trace.SpanFromContext(c.UserContext()), constant.ErrRuleNotFound))
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

// ---------------------------------------------------------------------------
// Phase 2b-1: the remaining six rule ops migrated to Huma. Each op mirrors the
// Create/Get reference tests: a success path (status + field-identical body +
// tenant threaded + no $schema leak) and a canonical-error path (code/status/
// type/problem+json, NEVER a native Huma 422, service not reached where the
// error precedes it). All NON-PARALLEL — see buildHumaRuleApp.
// ---------------------------------------------------------------------------

func TestHuma_UpdateRule_Success(t *testing.T) {
	id := testutil.MustDeterministicUUID(11)
	rule := &model.Rule{
		ID:         id,
		Name:       "Updated Rule",
		Expression: "amount > 2000",
		Action:     model.DecisionReview,
		Status:     model.RuleStatusDraft,
		CreatedAt:  testutil.FixedTime(),
		UpdatedAt:  testutil.FixedTime(),
	}
	svc := &tenantSpyService{updateResult: rule}
	app := buildHumaRuleApp(t, svc, "tenant-alpha")

	body, err := json.Marshal(map[string]any{"name": "Updated Rule"})
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPatch, "/v1/rules/"+id.String(), bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req, -1)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	respBody, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	assert.Equal(t, http.StatusOK, resp.StatusCode, "UpdateRule must return 200 through Huma")
	assert.NotContains(t, string(respBody), "$schema")

	var got map[string]any
	require.NoError(t, json.Unmarshal(respBody, &got), "body must be JSON: %s", string(respBody))
	assert.Equal(t, "Updated Rule", got["name"])
	assert.Equal(t, id.String(), got["ruleId"])
	assert.Equal(t, "tenant-alpha", svc.capturedTenant)
}

func TestHuma_UpdateRule_BadUUID(t *testing.T) {
	svc := &tenantSpyService{}
	app := buildHumaRuleApp(t, svc, "tenant-gamma")

	body, _ := json.Marshal(map[string]any{"name": "x"})
	req := httptest.NewRequest(http.MethodPatch, "/v1/rules/not-a-uuid", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req, -1)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	respBody, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	assert.Equal(t, http.StatusBadRequest, resp.StatusCode, "malformed path UUID must be the canonical 400 — no native Huma 422")
	assert.Equal(t, "application/problem+json", resp.Header.Get("Content-Type"))

	var got map[string]any
	require.NoError(t, json.Unmarshal(respBody, &got), "body must be JSON: %s", string(respBody))
	assert.Equal(t, "0065", got["code"])
	assert.Equal(t, libProblem.BaseURI+"/0065", got["type"])
	assert.Empty(t, svc.capturedTenant, "service must not be reached on a bad path param")
}

func TestHuma_UpdateRule_EmptyBody(t *testing.T) {
	svc := &tenantSpyService{}
	app := buildHumaRuleApp(t, svc, "tenant-gamma")

	id := testutil.MustDeterministicUUID(12)
	// A well-formed but empty patch body -> IsEmpty() -> ErrNothingToUpdate.
	req := httptest.NewRequest(http.MethodPatch, "/v1/rules/"+id.String(), bytes.NewReader([]byte("{}")))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req, -1)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	respBody, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	assert.Equal(t, http.StatusBadRequest, resp.StatusCode, "empty update must be the canonical 400 — no native Huma 422")
	assert.Equal(t, "application/problem+json", resp.Header.Get("Content-Type"))

	var got map[string]any
	require.NoError(t, json.Unmarshal(respBody, &got), "body must be JSON: %s", string(respBody))
	// ErrNothingToUpdate canonical code, identical to the Fiber path.
	assert.Equal(t, constant.ErrNothingToUpdate.Error(), got["code"])
	assert.Empty(t, svc.capturedTenant, "service must not be reached when there is nothing to update")
}

func TestHuma_UpdateRule_MalformedJSON(t *testing.T) {
	svc := &tenantSpyService{}
	app := buildHumaRuleApp(t, svc, "tenant-zeta")

	id := testutil.MustDeterministicUUID(13)
	req := httptest.NewRequest(http.MethodPatch, "/v1/rules/"+id.String(), bytes.NewReader([]byte("{not json")))
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

func TestHuma_ListRules_Success(t *testing.T) {
	rule := &model.Rule{
		ID:        testutil.MustDeterministicUUID(20),
		Name:      "Listed Rule",
		Action:    model.DecisionAllow,
		Status:    model.RuleStatusActive,
		CreatedAt: testutil.FixedTime(),
		UpdatedAt: testutil.FixedTime(),
	}
	svc := &tenantSpyService{listResult: &model.ListRulesResult{
		Rules:      []model.Rule{*rule},
		NextCursor: "next123",
		HasMore:    true,
	}}
	app := buildHumaRuleApp(t, svc, "tenant-alpha")

	req := httptest.NewRequest(http.MethodGet, "/v1/rules?limit=25&status=ACTIVE&sort_by=name&sort_order=asc", nil)
	resp, err := app.Test(req, -1)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	respBody, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	assert.Equal(t, http.StatusOK, resp.StatusCode, "ListRules must return 200 through Huma")
	assert.NotContains(t, string(respBody), "$schema")

	var got ListRulesResponse
	require.NoError(t, json.Unmarshal(respBody, &got), "body must be ListRulesResponse: %s", string(respBody))
	require.Len(t, got.Rules, 1)
	assert.Equal(t, "Listed Rule", got.Rules[0].Name)
	assert.Equal(t, "next123", got.NextCursor)
	assert.True(t, got.HasMore)

	assert.Equal(t, "tenant-alpha", svc.capturedTenant)
	// Imperative binding produced the same typed filter the Fiber QueryParser path would.
	require.NotNil(t, svc.listFilter)
	assert.Equal(t, 25, svc.listFilter.Limit)
	require.NotNil(t, svc.listFilter.Status)
	assert.Equal(t, model.RuleStatusActive, *svc.listFilter.Status)
	assert.Equal(t, "name", svc.listFilter.SortBy)
	assert.Equal(t, "ASC", svc.listFilter.SortOrder, "sort_order normalized to uppercase by SetDefaults")
}

func TestHuma_ListRules_Defaults(t *testing.T) {
	svc := &tenantSpyService{listResult: &model.ListRulesResult{Rules: nil}}
	app := buildHumaRuleApp(t, svc, "tenant-alpha")

	// No query params at all -> SetDefaults() must fill limit + sort.
	req := httptest.NewRequest(http.MethodGet, "/v1/rules", nil)
	resp, err := app.Test(req, -1)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	respBody, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	// Rules must serialize as [] not null.
	var got map[string]any
	require.NoError(t, json.Unmarshal(respBody, &got), "body JSON: %s", string(respBody))
	assert.NotNil(t, got["rules"], "rules must be [] not null")

	require.NotNil(t, svc.listFilter)
	assert.Equal(t, "created_at", svc.listFilter.SortBy, "default sort field applied by SetDefaults")
	assert.Equal(t, "DESC", svc.listFilter.SortOrder, "default sort order applied by SetDefaults")
}

// TestHuma_ListRules_InvalidLimit pins the query-param contract: an out-of-range
// limit must reach the core's imperative Validate() and produce the canonical
// 400 / code 0080 (ErrPaginationLimitExceeded) — NOT a native Huma 422 fired by
// a min/max struct tag. This is the query-param analogue of the format:"uuid"
// bug from Phase 2a: query fields carry NO validation tags; validation is
// imperative in the core.
func TestHuma_ListRules_InvalidLimit(t *testing.T) {
	svc := &tenantSpyService{}
	app := buildHumaRuleApp(t, svc, "tenant-gamma")

	req := httptest.NewRequest(http.MethodGet, "/v1/rules?limit=101", nil)
	resp, err := app.Test(req, -1)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	respBody, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	assert.Equal(t, http.StatusBadRequest, resp.StatusCode, "out-of-range limit must be the canonical 400 — no native Huma 422")
	assert.Equal(t, "application/problem+json", resp.Header.Get("Content-Type"))

	var got map[string]any
	require.NoError(t, json.Unmarshal(respBody, &got), "body must be JSON: %s", string(respBody))
	assert.Equal(t, "0080", got["code"], "code must be ErrPaginationLimitExceeded, identical to the Fiber path")
	assert.Equal(t, float64(http.StatusBadRequest), got["status"])
	assert.Equal(t, libProblem.BaseURI+"/0080", got["type"])
	assert.Empty(t, svc.capturedTenant, "service must not be reached on a bad query param")
}

// TestHuma_ListRules_NonNumericLimit pins the harder half of the query contract:
// limit=abc cannot coerce to an int. Because the Huma In struct takes limit as a
// STRING (no typed coercion, no min/max tag), Huma never fires a native 422; the
// core parses it imperatively and returns the canonical 400 / code 0082
// (ErrInvalidQueryParameter) — the same code the Fiber QueryParser-failure path
// produces.
func TestHuma_ListRules_NonNumericLimit(t *testing.T) {
	svc := &tenantSpyService{}
	app := buildHumaRuleApp(t, svc, "tenant-gamma")

	req := httptest.NewRequest(http.MethodGet, "/v1/rules?limit=abc", nil)
	resp, err := app.Test(req, -1)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	respBody, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	assert.Equal(t, http.StatusBadRequest, resp.StatusCode, "non-numeric limit must be the canonical 400 — no native Huma 422")
	assert.Equal(t, "application/problem+json", resp.Header.Get("Content-Type"))

	var got map[string]any
	require.NoError(t, json.Unmarshal(respBody, &got), "body must be JSON: %s", string(respBody))
	assert.Equal(t, "0082", got["code"], "code must be ErrInvalidQueryParameter, identical to the Fiber QueryParser-failure path")
	assert.Empty(t, svc.capturedTenant, "service must not be reached on an unparseable query param")
}

// TestHuma_ListRules_PresentButEmptyQueryParity is the money-path regression for
// the Phase 2b-1 review finding. Fiber's c.QueryParser binds a present-but-empty
// query key to a NON-nil pointer (`?status=` -> &"", `?limit=` -> &0), which
// downstream Validate() rejects; an ABSENT key stays nil (defaults / no filter).
// Huma's request layer drops value=="" before the handler, collapsing present-
// but-empty and absent to the same empty string — so the Huma path MUST read the
// raw query (via the Resolver) to reproduce Fiber's distinction. Each row below
// is empirically anchored to fiber v2.52.13's real QueryParser output against the
// real ListRulesInput; the expected code is what that value hits in the shared
// imperative Validate(). Without the Resolver-backed binder these all silently
// returned 200 — the exact format:"uuid"-class regression the third rail forbids.
func TestHuma_ListRules_PresentButEmptyQueryParity(t *testing.T) {
	// Validation-rejecting present-but-empty values: canonical 400, service NOT reached.
	rejectCases := []struct {
		name  string
		query string
		code  string // canonical Midaz code, identical to the Fiber path
	}{
		// ?status= -> Fiber Status=&"" -> RuleStatus("").IsValid()==false -> 0082.
		{"empty status", "?status=", "0082"},
		// ?action= -> Fiber Action=&"" -> Decision("").IsValid()==false -> 0082.
		{"empty action", "?action=", "0082"},
		// ?limit= -> Fiber Limit=&0 (gorilla decodes "" as 0) -> *limit<1 -> 0331.
		{"empty limit", "?limit=", "0331"},
	}
	for _, tc := range rejectCases {
		t.Run(tc.name, func(t *testing.T) {
			svc := &tenantSpyService{}
			app := buildHumaRuleApp(t, svc, "tenant-alpha")

			req := httptest.NewRequest(http.MethodGet, "/v1/rules"+tc.query, nil)
			resp, err := app.Test(req, -1)
			require.NoError(t, err)
			defer func() { _ = resp.Body.Close() }()

			respBody, err := io.ReadAll(resp.Body)
			require.NoError(t, err)

			assert.Equal(t, http.StatusBadRequest, resp.StatusCode,
				"present-but-empty %q must be the canonical 400, matching Fiber — never a silent 200 or native 422", tc.query)
			assert.Equal(t, "application/problem+json", resp.Header.Get("Content-Type"))

			var got map[string]any
			require.NoError(t, json.Unmarshal(respBody, &got), "body must be JSON: %s", string(respBody))
			assert.Equal(t, tc.code, got["code"], "code must be identical to the Fiber path for %q", tc.query)
			assert.Empty(t, svc.capturedTenant, "service must not be reached on a rejected query param")
		})
	}

	// Non-validating present-but-empty scope/name values: Fiber yields a non-nil
	// pointer to "" but every filter arm guards `*v != ""`, so the result is 200
	// with NO filter applied — identical to Fiber and to an absent key. The point
	// is parity: these must NOT 400 (the empty string is not a bad UUID/enum).
	passCases := []struct{ name, query string }{
		{"empty name", "?name="},
		{"empty account_id", "?account_id="},
		{"empty sub_type", "?sub_type="},
	}
	for _, tc := range passCases {
		t.Run(tc.name, func(t *testing.T) {
			svc := &tenantSpyService{listResult: &model.ListRulesResult{Rules: nil}}
			app := buildHumaRuleApp(t, svc, "tenant-alpha")

			req := httptest.NewRequest(http.MethodGet, "/v1/rules"+tc.query, nil)
			resp, err := app.Test(req, -1)
			require.NoError(t, err)
			defer func() { _ = resp.Body.Close() }()

			respBody, err := io.ReadAll(resp.Body)
			require.NoError(t, err)

			assert.Equal(t, http.StatusOK, resp.StatusCode,
				"present-but-empty %q is a no-op filter (guarded by *v != \"\"), must stay 200 as in Fiber", tc.query)
			require.NotNil(t, svc.listFilter, "service must be reached for %q", tc.query)
			// No scope filter and no name filter must be built from an empty value.
			assert.Nil(t, svc.listFilter.ScopeFilter, "empty scope value must build no scope filter for %q", tc.query)
			if tc.query == "?name=" {
				// Fiber binds Name=&""; the repo guards *Name != "" so no name filter
				// is applied. The pointer round-trips as &"" (parity with Fiber),
				// which the repo treats as no-filter.
				require.NotNil(t, svc.listFilter.Name, "Fiber binds present-but-empty name to &\"\"")
				assert.Equal(t, "", *svc.listFilter.Name)
			}
			_ = respBody
		})
	}
}

func TestHuma_ActivateRule_Success(t *testing.T) {
	id := testutil.MustDeterministicUUID(30)
	svc := &tenantSpyService{lifecycle: &model.Rule{ID: id, Name: "Active", Action: model.DecisionDeny, Status: model.RuleStatusActive}}
	app := buildHumaRuleApp(t, svc, "tenant-alpha")

	req := httptest.NewRequest(http.MethodPost, "/v1/rules/"+id.String()+"/activate", nil)
	resp, err := app.Test(req, -1)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	respBody, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	assert.Equal(t, http.StatusOK, resp.StatusCode, "ActivateRule must return 200 through Huma")
	assert.NotContains(t, string(respBody), "$schema")

	var got map[string]any
	require.NoError(t, json.Unmarshal(respBody, &got), "body JSON: %s", string(respBody))
	assert.Equal(t, id.String(), got["ruleId"])
	assert.Equal(t, "ACTIVE", got["status"])
	assert.Equal(t, "tenant-alpha", svc.capturedTenant)
}

func TestHuma_DeactivateRule_Success(t *testing.T) {
	id := testutil.MustDeterministicUUID(31)
	svc := &tenantSpyService{lifecycle: &model.Rule{ID: id, Name: "Inactive", Action: model.DecisionDeny, Status: model.RuleStatusInactive}}
	app := buildHumaRuleApp(t, svc, "tenant-alpha")

	req := httptest.NewRequest(http.MethodPost, "/v1/rules/"+id.String()+"/deactivate", nil)
	resp, err := app.Test(req, -1)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	respBody, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	assert.Equal(t, http.StatusOK, resp.StatusCode, "DeactivateRule must return 200 through Huma")

	var got map[string]any
	require.NoError(t, json.Unmarshal(respBody, &got), "body JSON: %s", string(respBody))
	assert.Equal(t, id.String(), got["ruleId"])
	assert.Equal(t, "INACTIVE", got["status"])
	assert.Equal(t, "tenant-alpha", svc.capturedTenant)
}

func TestHuma_DraftRule_Success(t *testing.T) {
	id := testutil.MustDeterministicUUID(32)
	svc := &tenantSpyService{lifecycle: &model.Rule{ID: id, Name: "Draft", Action: model.DecisionDeny, Status: model.RuleStatusDraft}}
	app := buildHumaRuleApp(t, svc, "tenant-alpha")

	req := httptest.NewRequest(http.MethodPost, "/v1/rules/"+id.String()+"/draft", nil)
	resp, err := app.Test(req, -1)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	respBody, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	assert.Equal(t, http.StatusOK, resp.StatusCode, "DraftRule must return 200 through Huma")

	var got map[string]any
	require.NoError(t, json.Unmarshal(respBody, &got), "body JSON: %s", string(respBody))
	assert.Equal(t, id.String(), got["ruleId"])
	assert.Equal(t, "DRAFT", got["status"])
	assert.Equal(t, "tenant-alpha", svc.capturedTenant)
}

func TestHuma_ActivateRule_BadUUID(t *testing.T) {
	svc := &tenantSpyService{}
	app := buildHumaRuleApp(t, svc, "tenant-gamma")

	req := httptest.NewRequest(http.MethodPost, "/v1/rules/not-a-uuid/activate", nil)
	resp, err := app.Test(req, -1)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	respBody, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	assert.Equal(t, http.StatusBadRequest, resp.StatusCode, "malformed path UUID must be the canonical 400 — no native Huma 422")
	assert.Equal(t, "application/problem+json", resp.Header.Get("Content-Type"))

	var got map[string]any
	require.NoError(t, json.Unmarshal(respBody, &got), "body JSON: %s", string(respBody))
	assert.Equal(t, "0065", got["code"])
	assert.Empty(t, svc.capturedTenant, "service must not be reached on a bad path param")
}

func TestHuma_DeleteRule_Success204NoBody(t *testing.T) {
	id := testutil.MustDeterministicUUID(40)
	svc := &tenantSpyService{} // deleteErr nil -> success
	app := buildHumaRuleApp(t, svc, "tenant-alpha")

	req := httptest.NewRequest(http.MethodDelete, "/v1/rules/"+id.String(), nil)
	resp, err := app.Test(req, -1)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	respBody, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	assert.Equal(t, http.StatusNoContent, resp.StatusCode, "DeleteRule must return 204 through Huma")
	// The body must be EMPTY — not "null", not "{}". Huma DefaultStatus:204 + an
	// Out struct with no Body field must emit a bodiless response.
	assert.Empty(t, respBody, "204 must carry no body; got %q", string(respBody))
	assert.Equal(t, "tenant-alpha", svc.capturedTenant)
}

func TestHuma_DeleteRule_BadUUID(t *testing.T) {
	svc := &tenantSpyService{}
	app := buildHumaRuleApp(t, svc, "tenant-gamma")

	req := httptest.NewRequest(http.MethodDelete, "/v1/rules/not-a-uuid", nil)
	resp, err := app.Test(req, -1)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	respBody, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	assert.Equal(t, http.StatusBadRequest, resp.StatusCode, "malformed path UUID must be the canonical 400 — no native Huma 422")
	assert.Equal(t, "application/problem+json", resp.Header.Get("Content-Type"))

	var got map[string]any
	require.NoError(t, json.Unmarshal(respBody, &got), "body JSON: %s", string(respBody))
	assert.Equal(t, "0065", got["code"])
	assert.Empty(t, svc.capturedTenant, "service must not be reached on a bad path param")
}

// TestHuma_ListRules_InvalidEnumParams pins the enum-shaped query params
// (status/sort_by/sort_order): a bad value must reach the imperative Validate()
// and produce the canonical 400 — NOT a native Huma 422 from an `enum` struct
// tag. These are the params most likely to be mis-tagged in a future edit, so
// each gets its own canonical-code assertion.
func TestHuma_ListRules_InvalidEnumParams(t *testing.T) {
	for _, tc := range []struct {
		name, query, code string
	}{
		{"invalid status", "status=INVALID", "0082"},        // ErrInvalidQueryParameter
		{"invalid sort_by", "sort_by=priority", "0332"},     // ErrInvalidSortColumn
		{"invalid sort_order", "sort_order=RANDOM", "0081"}, // ErrInvalidSortOrder
	} {
		t.Run(tc.name, func(t *testing.T) {
			svc := &tenantSpyService{}
			app := buildHumaRuleApp(t, svc, "tenant-gamma")

			req := httptest.NewRequest(http.MethodGet, "/v1/rules?"+tc.query, nil)
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

// TestHuma_ListRules_RepeatedKeyParity pins the repeated-query-key contract.
// Fiber's c.QueryParser (gorilla/schema over a scalar target) is LAST-wins:
// `?status=ACTIVE&status=garbage` binds "garbage". The Huma binder MUST resolve
// the SAME last value; reading the first value (url.Values.Get) flips the
// status/code for every affected field. Each row is empirically anchored to
// fiber v2.52.13's real QueryParser output against the real ListRulesInput; the
// expected code is what that LAST value hits in the shared imperative Validate().
// This is the money-path regression for the Phase 2b-1 review finding.
func TestHuma_ListRules_RepeatedKeyParity(t *testing.T) {
	// Repeated keys whose LAST value fails Validate: canonical 400, service NOT reached.
	rejectCases := []struct {
		name  string
		query string
		code  string // canonical Midaz code, identical to the Fiber last-wins path
	}{
		// Fiber last-wins "garbage" -> RuleStatus invalid -> 0082. First-wins "ACTIVE" would 200 (FLIP).
		{"status last invalid", "?status=ACTIVE&status=garbage", "0082"},
		// Fiber last-wins "abc" -> gorilla int decode error -> 0082. First-wins "25" would 200 (FLIP).
		{"limit last non-numeric", "?limit=25&limit=abc", "0082"},
		// Fiber last-wins "priority" -> not in sort whitelist -> 0332. First-wins "name" would 200 (FLIP).
		{"sort_by last invalid", "?sort_by=name&sort_by=priority", "0332"},
		// Last-wins subsumes present-but-empty: last "" -> RuleStatus("") invalid -> 0082.
		{"status last empty", "?status=ACTIVE&status=", "0082"},
	}
	for _, tc := range rejectCases {
		t.Run(tc.name, func(t *testing.T) {
			// Non-nil listResult so that if the (buggy first-wins) binder wrongly
			// lets the LAST-invalid value pass Validate, the request returns a
			// silent 200 — the flip this test catches — instead of panicking in
			// the service layer on a nil result.
			svc := &tenantSpyService{listResult: &model.ListRulesResult{Rules: nil}}
			app := buildHumaRuleApp(t, svc, "tenant-alpha")

			req := httptest.NewRequest(http.MethodGet, "/v1/rules"+tc.query, nil)
			resp, err := app.Test(req, -1)
			require.NoError(t, err)
			defer func() { _ = resp.Body.Close() }()

			respBody, err := io.ReadAll(resp.Body)
			require.NoError(t, err)

			assert.Equal(t, http.StatusBadRequest, resp.StatusCode,
				"repeated key %q must resolve to Fiber's LAST value and 400 — never the first-value 200 (a status flip)", tc.query)
			assert.Equal(t, "application/problem+json", resp.Header.Get("Content-Type"))

			var got map[string]any
			require.NoError(t, json.Unmarshal(respBody, &got), "body must be JSON: %s", string(respBody))
			assert.Equal(t, tc.code, got["code"], "code must match Fiber's last-wins value for %q", tc.query)
			assert.Empty(t, svc.capturedTenant, "service must not be reached on a rejected query param")
		})
	}

	// Repeated keys whose LAST value passes Validate: 200, filter built from the
	// LAST value. Guards the reverse flip (last-wins accepting where first-wins
	// would reject) and confirms last-wins subsumes the empty-then-valid case.
	passCases := []struct {
		name       string
		query      string
		wantLimit  int
		wantStatus model.RuleStatus // "" => Status expected nil
		wantSortBy string
	}{
		// Fiber last-wins "25" -> 200. First-wins "101" would 400/0080 (reverse FLIP).
		{"limit last in-range", "?limit=101&limit=25", 25, "", ""},
		// Last-wins subsumes present-but-empty: last "ACTIVE" after empty -> valid.
		{"status empty then valid", "?status=&status=ACTIVE", 10, model.RuleStatusActive, ""},
	}
	for _, tc := range passCases {
		t.Run(tc.name, func(t *testing.T) {
			svc := &tenantSpyService{listResult: &model.ListRulesResult{Rules: nil}}
			app := buildHumaRuleApp(t, svc, "tenant-alpha")

			req := httptest.NewRequest(http.MethodGet, "/v1/rules"+tc.query, nil)
			resp, err := app.Test(req, -1)
			require.NoError(t, err)
			defer func() { _ = resp.Body.Close() }()

			_, err = io.ReadAll(resp.Body)
			require.NoError(t, err)

			assert.Equal(t, http.StatusOK, resp.StatusCode,
				"repeated key %q must resolve to Fiber's LAST value and 200", tc.query)
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

// TestHuma_RuleRoutes_SecurityMetadata asserts every rule op advertises the
// bearer-OR-apikey security requirement in the generated OAS 3.1 spec. This is
// SPEC metadata only — runtime auth is unchanged (Fiber guard.With). It guards
// the reference handler's Security fields; the 2b fan-out reuses the identical
// secBearerOrAPIKey helper, and the shared check-docs.sh security-coverage gate
// asserts the per-op coverage across all 28 ops.
func TestHuma_RuleRoutes_SecurityMetadata(t *testing.T) {
	// NOT parallel: openapi.New over a shared Fiber app mutates process-global
	// Huma state (libProblem is installed by sibling tests). See buildHumaRuleApp.
	f := fiber.New(fiber.Config{DisableStartupMessage: true})
	api := f.Group("/v1")
	hAPI := openapi.New(f, api, openapi.Config{Title: "tracer-test", Version: "test", Servers: []string{"/v1"}})
	RegisterRuleRoutes(hAPI, NewHandler(&tenantSpyService{}))

	paths := hAPI.OpenAPI().Paths

	op := func(path, verb string) *huma.Operation {
		t.Helper()
		item := paths[path]
		require.NotNilf(t, item, "path %q missing from spec", path)
		switch verb {
		case http.MethodGet:
			return item.Get
		case http.MethodPost:
			return item.Post
		case http.MethodPatch:
			return item.Patch
		case http.MethodDelete:
			return item.Delete
		default:
			t.Fatalf("unsupported verb %q", verb)
			return nil
		}
	}

	cases := []struct {
		path, verb string
	}{
		{"/rules", http.MethodPost},
		{"/rules/{id}", http.MethodGet},
		{"/rules", http.MethodGet},
		{"/rules/{id}", http.MethodPatch},
		{"/rules/{id}", http.MethodDelete},
		{"/rules/{id}/activate", http.MethodPost},
		{"/rules/{id}/deactivate", http.MethodPost},
		{"/rules/{id}/draft", http.MethodPost},
	}
	for _, tc := range cases {
		o := op(tc.path, tc.verb)
		require.NotNilf(t, o, "%s %s missing from spec", tc.verb, tc.path)
		assert.Equalf(t, secBearerOrAPIKey, o.Security,
			"%s %s must advertise bearer-OR-apikey security", tc.verb, tc.path)
	}
}
