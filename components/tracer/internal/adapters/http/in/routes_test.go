// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	authMiddleware "github.com/LerianStudio/lib-auth/v2/auth/middleware"

	libLog "github.com/LerianStudio/lib-observability/log"
	libOtel "github.com/LerianStudio/lib-observability/tracing"
	"github.com/gofiber/fiber/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/LerianStudio/midaz/v4/components/tracer/internal/adapters/http/in/middleware"
	"github.com/LerianStudio/midaz/v4/components/tracer/internal/adapters/http/in/mocks"
	"github.com/LerianStudio/midaz/v4/components/tracer/internal/testutil"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/clock"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/model"
)

func TestMain(m *testing.M) {
	// Skip telemetry middleware that causes data races in lib-commons ContextWithLogger.
	// The race occurs when multiple goroutines call it concurrently (as happens in Fiber's app.Test).
	os.Setenv("SKIP_LIB_COMMONS_TELEMETRY", "true")
	os.Exit(m.Run())
}

// errorResponse represents the standard error response format from libHTTP.
type errorResponse struct {
	Code   string `json:"code"`
	Title  string `json:"title"`
	Detail string `json:"detail"`
}

// testRouterDeps holds dependencies for creating test routers.
// Extracted to allow tests to configure mock expectations before router creation.
type testRouterDeps struct {
	RuleService                  *MockRuleService
	LimitService                 *MockLimitService
	ValidationService            *mocks.MockValidationService
	ReservationService           *mocks.MockReservationService
	TransactionValidationService *mocks.MockTransactionValidationService
	AuditEventService            *MockAuditEventService
	guardCfg                     middleware.AuthGuardConfig
	swaggerEnabled               bool
	t                            *testing.T
}

// newTestRouterDeps creates test dependencies without any mock expectations.
// Tests should configure expectations on the returned mocks before calling build().
func newTestRouterDeps(t *testing.T, guardCfg middleware.AuthGuardConfig) *testRouterDeps {
	ctrl := gomock.NewController(t)

	return &testRouterDeps{
		RuleService:                  NewMockRuleService(ctrl),
		LimitService:                 NewMockLimitService(ctrl),
		ValidationService:            mocks.NewMockValidationService(ctrl),
		ReservationService:           mocks.NewMockReservationService(ctrl),
		TransactionValidationService: mocks.NewMockTransactionValidationService(ctrl),
		AuditEventService:            NewMockAuditEventService(ctrl),
		guardCfg:                     guardCfg,
		t:                            t,
	}
}

// build creates the Fiber app with the configured dependencies.
func (d *testRouterDeps) build() *fiber.App {
	mockLogger := testutil.NewMockLogger()
	telemetry := &libOtel.Telemetry{
		TelemetryConfig: libOtel.TelemetryConfig{
			ServiceName:     "tracer-test",
			EnableTelemetry: false,
			Logger:          mockLogger,
		},
	}

	// lib-auth v2.7.0 takes a lib-commons/v5 *log.Logger. Tests don't assert
	// on auth-client log output — a v5 NopLogger keeps stdout clean.
	authLogger := libLog.NewNop()
	authClient := authMiddleware.NewAuthClient("", d.guardCfg.PluginAuthEnabled, &authLogger)
	guard := middleware.NewAuthGuard(d.guardCfg, authClient)

	routeCfg := &RouteConfig{SwaggerEnabled: d.swaggerEnabled}

	clk := clock.New()

	// Avoid the typed-nil interface trap: a nil *MockReservationService stored in
	// the ReservationService interface field would be non-nil and the route guard
	// would mount the routes. Convert an explicit nil mock to a true interface nil.
	var reservationService ReservationService
	if d.ReservationService != nil {
		reservationService = d.ReservationService
	}

	app, err := NewRoutes(RoutesDeps{
		Logger:                       mockLogger,
		Telemetry:                    telemetry,
		HealthChecker:                &HealthChecker{},
		Cfg:                          routeCfg,
		RuleService:                  d.RuleService,
		LimitService:                 d.LimitService,
		ValidationService:            d.ValidationService,
		ReservationService:           reservationService,
		TransactionValidationService: d.TransactionValidationService,
		AuditEventService:            d.AuditEventService,
		Guard:                        guard,
		Clock:                        clk,
	})
	require.NoError(d.t, err)
	return app
}

// createTestRouter creates a test router with the given AuthGuardConfig.
// For auth/route protection tests that don't exercise service handlers.
// No mock expectations are set - requests that reach handlers will fail with gomock errors,
// which helps catch accidental handler invocations during refactors.
func createTestRouter(t *testing.T, guardCfg middleware.AuthGuardConfig) *fiber.App {
	deps := newTestRouterDeps(t, guardCfg)
	return deps.build()
}

func TestRoutes_PublicEndpoints_NoAuthRequired(t *testing.T) {
	// Note: SKIP_LIB_COMMONS_TELEMETRY=true is set in TestMain to skip telemetry middleware that causes data races.
	guardCfg := middleware.AuthGuardConfig{
		APIKey:        "test-secret-key-32-characters-long",
		APIKeyEnabled: true,
		AppName:       "tracer",
	}
	app := createTestRouter(t, guardCfg)

	testCases := []struct {
		name             string
		path             string
		acceptableStatus []int
	}{
		{"health", "/health", []int{http.StatusOK}},
		{"readyz", "/readyz", []int{http.StatusOK, http.StatusServiceUnavailable}},
		{"version", "/version", []int{http.StatusOK}},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, tc.path, nil)
			// Note: NO X-API-Key header

			resp, err := app.Test(req, -1)
			require.NoError(t, err)

			// Public endpoints should NOT require authentication (should NOT return 401)
			assert.NotEqual(t, http.StatusUnauthorized, resp.StatusCode,
				"Public endpoint %s should NOT return 401 Unauthorized", tc.path)

			// Verify status is one of the acceptable statuses
			assert.Contains(t, tc.acceptableStatus, resp.StatusCode,
				"Public endpoint %s returned unexpected status %d", tc.path, resp.StatusCode)

			require.NoError(t, resp.Body.Close())
		})
	}
}

func TestRoutes_ProtectedEndpoints_RequireAuth(t *testing.T) {
	tests := []struct {
		name           string
		method         string
		path           string
		expectedStatus int
		expectedCode   string
	}{
		{
			name:           "POST /v1/validations returns 401 without API key",
			method:         http.MethodPost,
			path:           "/v1/validations",
			expectedStatus: http.StatusUnauthorized,
			expectedCode:   "Unauthenticated",
		},
		{
			name:           "GET /v1/rules returns 401 without API key",
			method:         http.MethodGet,
			path:           "/v1/rules",
			expectedStatus: http.StatusUnauthorized,
			expectedCode:   "Unauthenticated",
		},
		{
			name:           "GET /v1/limits returns 401 without API key",
			method:         http.MethodGet,
			path:           "/v1/limits",
			expectedStatus: http.StatusUnauthorized,
			expectedCode:   "Unauthenticated",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create router per subtest for proper isolation
			guardCfg := middleware.AuthGuardConfig{
				APIKey:        "test-secret-key-32-characters-long",
				APIKeyEnabled: true,
				AppName:       "tracer",
			}
			app := createTestRouter(t, guardCfg)

			req := httptest.NewRequest(tt.method, tt.path, nil)
			// Note: NO X-API-Key header

			resp, err := app.Test(req, -1)
			require.NoError(t, err)
			defer resp.Body.Close()

			// Protected endpoints should require authentication
			assert.Equal(t, tt.expectedStatus, resp.StatusCode,
				"Protected endpoint %s should require API key", tt.path)

			if tt.expectedStatus == http.StatusUnauthorized {
				body, err := io.ReadAll(resp.Body)
				require.NoError(t, err)

				var errResp errorResponse
				err = json.Unmarshal(body, &errResp)
				require.NoError(t, err, "Expected JSON error response, got: %s", string(body))
				assert.Equal(t, tt.expectedCode, errResp.Code,
					"Error code should be 'Unauthenticated'")
			}
		})
	}
}

func TestRoutes_ProtectedEndpoints_ValidKey(t *testing.T) {
	validAPIKey := "test-secret-key-32-characters-long"

	tests := []struct {
		name      string
		method    string
		path      string
		needsMock string // "rules", "limits", or "" for no mock needed
	}{
		{
			name:      "POST /v1/validations accessible with valid API key",
			method:    http.MethodPost,
			path:      "/v1/validations",
			needsMock: "",
		},
		{
			name:      "GET /v1/rules accessible with valid API key",
			method:    http.MethodGet,
			path:      "/v1/rules",
			needsMock: "rules",
		},
		{
			name:      "GET /v1/limits accessible with valid API key",
			method:    http.MethodGet,
			path:      "/v1/limits",
			needsMock: "limits",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			guardCfg := middleware.AuthGuardConfig{
				APIKey:        validAPIKey,
				APIKeyEnabled: true,
				AppName:       "tracer",
			}
			deps := newTestRouterDeps(t, guardCfg)

			// Set expectations only for endpoints that actually hit handlers
			switch tt.needsMock {
			case "rules":
				deps.RuleService.EXPECT().ListRules(gomock.Any(), gomock.Any()).
					Return(&model.ListRulesResult{Rules: []model.Rule{}}, nil).Times(1)
			case "limits":
				deps.LimitService.EXPECT().ListLimits(gomock.Any(), gomock.Any()).
					Return(&model.ListLimitsResult{Limits: []model.Limit{}}, nil).Times(1)
			}

			app := deps.build()

			req := httptest.NewRequest(tt.method, tt.path, nil)
			req.Header.Set("X-API-Key", validAPIKey)

			resp, err := app.Test(req, -1)
			require.NoError(t, err)
			defer resp.Body.Close()

			// With valid API key, should NOT get 401 Unauthorized
			// The request might get 404 if no handler exists, but NOT 401
			assert.NotEqual(t, http.StatusUnauthorized, resp.StatusCode,
				"Protected endpoint %s with valid API key should not return 401", tt.path)
		})
	}
}

func TestRoutes_ProtectedEndpoints_AuthDisabled(t *testing.T) {
	tests := []struct {
		name      string
		method    string
		path      string
		needsMock string
	}{
		{
			name:      "GET /v1/rules accessible when auth disabled",
			method:    http.MethodGet,
			path:      "/v1/rules",
			needsMock: "rules",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			guardCfg := middleware.AuthGuardConfig{
				APIKey:        "some-key",
				APIKeyEnabled: false, // Auth disabled
				AppName:       "tracer",
			}
			deps := newTestRouterDeps(t, guardCfg)

			switch tt.needsMock {
			case "rules":
				deps.RuleService.EXPECT().ListRules(gomock.Any(), gomock.Any()).
					Return(&model.ListRulesResult{Rules: []model.Rule{}}, nil).Times(1)
			}

			app := deps.build()

			req := httptest.NewRequest(tt.method, tt.path, nil)
			// Note: NO X-API-Key header

			resp, err := app.Test(req, -1)
			require.NoError(t, err)
			defer resp.Body.Close()

			// With auth disabled, should NOT get 401 Unauthorized
			assert.NotEqual(t, http.StatusUnauthorized, resp.StatusCode,
				"With auth disabled, endpoint %s should not return 401", tt.path)
		})
	}
}

// TestRoutes_ReservationEndpoints_Mounted asserts the three two-phase
// reservation routes are mounted under the "reservations" guard. A guarded route
// answers 401 without an API key; an unmounted route answers 404. This is the
// route-table presence proof for F3-T08 — it distinguishes "mounted and
// protected" from "missing".
func TestRoutes_ReservationEndpoints_Mounted(t *testing.T) {
	reservationID := testutil.MustDeterministicUUID(1)

	tests := []struct {
		name   string
		method string
		path   string
	}{
		{"POST /v1/reservations", http.MethodPost, "/v1/reservations"},
		{"POST /v1/reservations/:id/confirm", http.MethodPost, "/v1/reservations/" + reservationID.String() + "/confirm"},
		{"POST /v1/reservations/:id/release", http.MethodPost, "/v1/reservations/" + reservationID.String() + "/release"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			guardCfg := middleware.AuthGuardConfig{
				APIKey:        "test-secret-key-32-characters-long",
				APIKeyEnabled: true,
				AppName:       "tracer",
			}
			app := createTestRouter(t, guardCfg)

			req := httptest.NewRequest(tt.method, tt.path, nil)
			// No X-API-Key header: a mounted+guarded route must reply 401, not 404.

			resp, err := app.Test(req, -1)
			require.NoError(t, err)
			defer resp.Body.Close()

			assert.Equal(t, http.StatusUnauthorized, resp.StatusCode,
				"reservation route %s should be mounted and require auth (401), not missing (404)", tt.path)
		})
	}
}

// TestRoutes_ReservationEndpoints_NotMountedWhenServiceNil asserts the reservation
// routes are absent (404) when the reservation service is not wired — the API is
// additive per the RoutesDeps zero-value contract.
func TestRoutes_ReservationEndpoints_NotMountedWhenServiceNil(t *testing.T) {
	guardCfg := middleware.AuthGuardConfig{
		APIKey:        "test-secret-key-32-characters-long",
		APIKeyEnabled: true,
		AppName:       "tracer",
	}
	deps := newTestRouterDeps(t, guardCfg)
	deps.ReservationService = nil // not wired

	app := deps.build()

	req := httptest.NewRequest(http.MethodPost, "/v1/reservations", nil)

	resp, err := app.Test(req, -1)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusNotFound, resp.StatusCode,
		"reservation route should not be mounted when the reservation service is nil")
}

// TestWriteTenantCapReached verifies that the 503 envelope emitted when the
// worker supervisor declines a tenant matches the canonical
// libCommons.Response{Code,Title,Message} shape — not the legacy
// fiber.Map{code, message} body that bypassed Title (H4/H6).
func TestWriteTenantCapReached(t *testing.T) {
	t.Parallel()

	app := fiber.New()
	app.Get("/__test/cap", func(c *fiber.Ctx) error {
		c.Set("Retry-After", tenantCapRetryAfterHeader())

		return writeTenantCapReached(c)
	})

	req := httptest.NewRequest(http.MethodGet, "/__test/cap", nil)
	resp, err := app.Test(req, -1)
	require.NoError(t, err)

	defer func() {
		require.NoError(t, resp.Body.Close())
	}()

	assert.Equal(t, http.StatusServiceUnavailable, resp.StatusCode,
		"tenant cap response must be HTTP 503")
	assert.Equal(t, tenantCapRetryAfterHeader(), resp.Header.Get("Retry-After"),
		"Retry-After header must be preserved")

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	var payload struct {
		Code    string `json:"code"`
		Title   string `json:"title"`
		Message string `json:"message"`
	}
	require.NoError(t, json.Unmarshal(body, &payload),
		"body must be valid JSON in the canonical envelope; got %s", string(body))

	// Code must be the registered canonical code, not a legacy ad-hoc string.
	assert.Equal(t, "0466", payload.Code,
		"code must be the registered ErrTenantCapReached sentinel")
	assert.NotEmpty(t, payload.Title,
		"title must be present so SDK consumers can parse the envelope")
	assert.Equal(t, "Tenant Capacity Reached", payload.Title)
	assert.Equal(t, "Tenant capacity reached; please retry shortly", payload.Message)
}

func TestGetCORSAllowedOrigins(t *testing.T) {
	tests := []struct {
		name       string
		configured string
		expected   string
	}{
		{
			name:       "Success - empty returns default (restrictive)",
			configured: "",
			expected:   "",
		},
		{
			name:       "Success - wildcard for development",
			configured: "*",
			expected:   "*",
		},
		{
			name:       "Success - single origin for production",
			configured: "https://app.example.com",
			expected:   "https://app.example.com",
		},
		{
			name:       "Success - multiple origins for production",
			configured: "https://app.example.com,https://admin.example.com",
			expected:   "https://app.example.com,https://admin.example.com",
		},
		// Edge cases - passed through as-is (CORS middleware handles validation)
		{
			name:       "Edge case - whitespace in origins passed through",
			configured: "https://app.example.com, https://admin.example.com",
			expected:   "https://app.example.com, https://admin.example.com",
		},
		{
			name:       "Edge case - trailing comma passed through",
			configured: "https://app.example.com,",
			expected:   "https://app.example.com,",
		},
		{
			name:       "Edge case - whitespace only passed through",
			configured: "   ",
			expected:   "   ",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := getCORSAllowedOrigins(tt.configured)
			assert.Equal(t, tt.expected, result)
		})
	}
}
