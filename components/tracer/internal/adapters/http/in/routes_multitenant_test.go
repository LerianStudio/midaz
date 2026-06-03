// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	authMiddleware "github.com/LerianStudio/lib-auth/v2/auth/middleware"
	libLog "github.com/LerianStudio/lib-observability/log"
	libOtel "github.com/LerianStudio/lib-observability/tracing"
	"github.com/gofiber/fiber/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"tracer/internal/adapters/http/in/middleware"
	"tracer/internal/adapters/http/in/mocks"
	"tracer/internal/testutil"
	"tracer/pkg/clock"
	"tracer/pkg/model"
)

// testAPIKey is shared between the router builder and tests that need to send
// an authenticated request through the full /v1 pipeline.
const testAPIKey = "test-secret-key-32-characters-long"

// recordingEnsurer is a test double for WorkerEnsurer. It records every
// tenantID it is called with so tests can assert whether the lazy-spawn hook
// fired (or did not).
type recordingEnsurer struct {
	mu    sync.Mutex
	calls []string
}

func (r *recordingEnsurer) EnsureWorkers(_ context.Context, tenantID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.calls = append(r.calls, tenantID)

	return nil
}

func (r *recordingEnsurer) callCount() int {
	r.mu.Lock()
	defer r.mu.Unlock()

	return len(r.calls)
}

// multiTenantTestRouter bundles the Fiber app with the service mocks so tests
// can wire per-endpoint expectations (e.g. stubbing out ListRules on the rule
// service when the request needs to clear the auth guard and reach a handler).
type multiTenantTestRouter struct {
	app         *fiber.App
	ruleService *MockRuleService
}

// buildMultiTenantTestRouter creates a Fiber app with the given multi-tenant
// flags wired into NewRoutes. pgManager is always nil in these tests because
// constructing a real tmpostgres.Manager would require a live Tenant Manager
// HTTP server. The tenant middleware registration is gated on both
// multiTenantEnabled AND pgManager != nil, so passing nil disables the
// middleware — which is exactly what each test wants to verify.
func buildMultiTenantTestRouter(t *testing.T, multiTenantEnabled bool, ensurer WorkerEnsurer) *multiTenantTestRouter {
	t.Helper()

	ctrl := gomock.NewController(t)

	ruleService := NewMockRuleService(ctrl)
	limitService := NewMockLimitService(ctrl)
	validationService := mocks.NewMockValidationService(ctrl)
	transactionValidationService := mocks.NewMockTransactionValidationService(ctrl)
	auditEventService := NewMockAuditEventService(ctrl)

	mockLogger := testutil.NewMockLogger()
	telemetry := &libOtel.Telemetry{
		TelemetryConfig: libOtel.TelemetryConfig{
			ServiceName:     "tracer-test",
			EnableTelemetry: false,
			Logger:          mockLogger,
		},
	}

	// For multi-tenant tests we still use API key auth for simplicity: the
	// tenant middleware lives at /v1 level alongside the AuthGuard, so an
	// authenticated request that reaches /v1/* exercises the tenant path too.
	guardCfg := middleware.AuthGuardConfig{
		APIKey:        testAPIKey,
		APIKeyEnabled: true,
		AppName:       "tracer",
	}

	// lib-auth v2.7.0 takes a lib-commons/v5 *log.Logger. Tests don't assert
	// on auth-client log output — a v5 NopLogger keeps stdout clean.
	authLogger := libLog.NewNop()
	authClient := authMiddleware.NewAuthClient("", guardCfg.PluginAuthEnabled, &authLogger)
	guard := middleware.NewAuthGuard(guardCfg, authClient)

	routeCfg := &RouteConfig{}
	clk := clock.New()

	app, err := NewRoutes(RoutesDeps{
		Logger:                       mockLogger,
		Telemetry:                    telemetry,
		HealthChecker:                &HealthChecker{},
		Cfg:                          routeCfg,
		RuleService:                  ruleService,
		LimitService:                 limitService,
		ValidationService:            validationService,
		TransactionValidationService: transactionValidationService,
		AuditEventService:            auditEventService,
		Guard:                        guard,
		Clock:                        clk,
		MultiTenantEnabled:           multiTenantEnabled,
		PgManager:                    nil, // real tmpostgres.Manager needs a live TM server
		Supervisor:                   ensurer,
	})
	require.NoError(t, err)

	return &multiTenantTestRouter{app: app, ruleService: ruleService}
}

// TestRoutes_SingleTenant_NoTenantMiddleware verifies that when the multi-tenant
// flag is false, neither the tenant middleware nor the lazy-worker hook runs.
// Public endpoints must still serve 200 without any Authorization header.
func TestRoutes_SingleTenant_NoTenantMiddleware(t *testing.T) {
	t.Parallel()

	ensurer := &recordingEnsurer{}
	router := buildMultiTenantTestRouter(t, false /*multiTenantEnabled*/, ensurer)

	publicPaths := []struct {
		name             string
		path             string
		acceptableStatus []int
	}{
		{"health", "/health", []int{http.StatusOK}},
		{"readyz", "/readyz", []int{http.StatusOK, http.StatusServiceUnavailable}},
		{"version", "/version", []int{http.StatusOK}},
	}

	for _, tc := range publicPaths {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, tc.path, nil)
			resp, err := router.app.Test(req, -1)
			require.NoError(t, err)
			require.NoError(t, resp.Body.Close())

			assert.Contains(t, tc.acceptableStatus, resp.StatusCode,
				"single-tenant public endpoint %s should not require tenant context", tc.path)
		})
	}

	assert.Equal(t, 0, ensurer.callCount(),
		"EnsureWorkers must not be called in single-tenant mode")
}

// TestRoutes_MultiTenant_PublicEndpointsBypass verifies that even with
// multi-tenant mode on, the public endpoints remain reachable without a JWT.
// These paths are declared on the root Fiber instance (not /v1), so the
// tenant middleware should not intercept them.
func TestRoutes_MultiTenant_PublicEndpointsBypass(t *testing.T) {
	t.Parallel()

	ensurer := &recordingEnsurer{}
	router := buildMultiTenantTestRouter(t, true /*multiTenantEnabled*/, ensurer)

	publicPaths := []struct {
		name             string
		path             string
		acceptableStatus []int
	}{
		{"health", "/health", []int{http.StatusOK}},
		{"readyz", "/readyz", []int{http.StatusOK, http.StatusServiceUnavailable}},
		{"version", "/version", []int{http.StatusOK}},
	}

	for _, tc := range publicPaths {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, tc.path, nil)
			// Explicitly NO Authorization or X-API-Key headers.
			resp, err := router.app.Test(req, -1)
			require.NoError(t, err)
			require.NoError(t, resp.Body.Close())

			// Must not reject with 401 or 403 — these endpoints are public by design.
			assert.NotEqual(t, http.StatusUnauthorized, resp.StatusCode,
				"public endpoint %s must bypass tenant middleware", tc.path)
			assert.NotEqual(t, http.StatusForbidden, resp.StatusCode,
				"public endpoint %s must bypass tenant middleware", tc.path)
			assert.Contains(t, tc.acceptableStatus, resp.StatusCode,
				"public endpoint %s returned unexpected status %d", tc.path, resp.StatusCode)
		})
	}

	assert.Equal(t, 0, ensurer.callCount(),
		"EnsureWorkers must not run for public paths even with multi-tenant enabled")
}

// TestRoutes_MultiTenant_ProtectedRequiresAuth verifies that protected /v1 routes
// still reject unauthenticated requests in multi-tenant mode. Regardless of the
// tenant middleware, the AuthGuard runs first for /v1 and returns 401.
func TestRoutes_MultiTenant_ProtectedRequiresAuth(t *testing.T) {
	t.Parallel()

	ensurer := &recordingEnsurer{}
	router := buildMultiTenantTestRouter(t, true /*multiTenantEnabled*/, ensurer)

	req := httptest.NewRequest(http.MethodPost, "/v1/validations", nil)
	// Deliberately no X-API-Key or Authorization header.
	resp, err := router.app.Test(req, -1)
	require.NoError(t, err)
	require.NoError(t, resp.Body.Close())

	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode,
		"protected endpoint must reject unauthenticated request in multi-tenant mode")

	// The AuthGuard short-circuits before the tenant middleware has a chance
	// to run (and pgManager is nil so the middleware is disabled anyway), so
	// EnsureWorkers must NOT have been called.
	assert.Equal(t, 0, ensurer.callCount(),
		"EnsureWorkers must not fire on unauthenticated requests")
}

// TestRoutes_MultiTenant_MiddlewareDisabledWhenPgManagerNil verifies the
// actual safety-rail invariant: with multiTenantEnabled=true AND pgManager=nil,
// the tenant middleware block is skipped entirely, so the lazy-spawn hook
// that sits inside that block does not run — even for requests that clear
// authentication and reach a handler.
//
// Test shape:
//   - send a request with a VALID API key so auth passes,
//   - target a handler we can stub (GET /v1/rules → ListRules),
//   - assert 200 (or whatever the stub returns), and
//   - assert ensurer.callCount() == 0 to prove the MW was not registered.
//
// Contrast with TestRoutes_MultiTenant_ProtectedRequiresAuth: that test
// exercises the UNauthenticated case where AuthGuard short-circuits at 401.
// This test exercises the AUTHENTICATED case where the request passes auth,
// which is the only way to observe whether the tenant MW was wired at all.
func TestRoutes_MultiTenant_MiddlewareDisabledWhenPgManagerNil(t *testing.T) {
	t.Parallel()

	ensurer := &recordingEnsurer{}
	router := buildMultiTenantTestRouter(t, true /*multiTenantEnabled*/, ensurer)

	// Stub ListRules so the handler returns 200 instead of hitting an
	// unexpected-call assertion from gomock. The filter shape is whatever the
	// handler builds from a bare GET — gomock.Any keeps the test tolerant of
	// default-filter drift.
	router.ruleService.EXPECT().
		ListRules(gomock.Any(), gomock.Any()).
		Return(&model.ListRulesResult{Rules: []model.Rule{}}, nil).
		Times(1)

	req := httptest.NewRequest(http.MethodGet, "/v1/rules", nil)
	req.Header.Set("X-API-Key", testAPIKey)
	resp, err := router.app.Test(req, -1)
	require.NoError(t, err)
	require.NoError(t, resp.Body.Close())

	assert.Equal(t, http.StatusOK, resp.StatusCode,
		"authenticated request must reach the handler when tenant MW is disabled")
	assert.Equal(t, 0, ensurer.callCount(),
		"EnsureWorkers must not be called when pgManager is nil — the entire "+
			"tenant middleware block (including the lazy-spawn hook) is skipped")
}
