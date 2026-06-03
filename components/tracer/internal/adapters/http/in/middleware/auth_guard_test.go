// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package middleware

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	authMiddleware "github.com/LerianStudio/lib-auth/v2/auth/middleware"
	libLog "github.com/LerianStudio/lib-observability/log"

	"github.com/gofiber/fiber/v2"
	jwt "github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"tracer/pkg/contextutil"
)

// newTestAuthGuard creates an AuthGuard with a real AuthClient.
// When pluginAuthEnabled=true, a fake server address is provided so that
// the AuthClient actually enforces auth (instead of pass-through on empty address).
func newTestAuthGuard(t *testing.T, cfg AuthGuardConfig, fakeAuthServerURL string) *AuthGuard {
	t.Helper()

	address := ""
	if cfg.PluginAuthEnabled && fakeAuthServerURL != "" {
		address = fakeAuthServerURL
	}

	// Auth client logger is the lib-commons/v5 surface (see lib-auth v2.7.0).
	// Tests don't assert on auth-client log output, so a v5 NopLogger keeps
	// the test stdout clean while preserving the *log.Logger pointer shape
	// NewAuthClient expects.
	authLogger := libLog.NewNop()
	authClient := authMiddleware.NewAuthClient(address, cfg.PluginAuthEnabled, &authLogger)

	return NewAuthGuard(cfg, authClient)
}

// newFakeAuthServer creates a test HTTP server that simulates the auth service.
// Returns 403 to any request, ensuring unauthenticated calls are rejected.
func newFakeAuthServer(t *testing.T) *httptest.Server {
	t.Helper()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))

	t.Cleanup(server.Close)

	return server
}

// newTestApp creates a Fiber app with a single GET /test route protected by the given handler.
func newTestApp(authHandler fiber.Handler) *fiber.App {
	app := fiber.New()
	app.Get("/test", authHandler, func(c *fiber.Ctx) error {
		return c.SendString("success")
	})

	return app
}

// isPluginAuthResponse checks if the response body is plain text "Missing Token"
// (returned by lib-auth plugin auth when no Bearer token is present).
func isPluginAuthResponse(t *testing.T, body []byte) bool {
	t.Helper()

	return string(body) == "Missing Token"
}

// isAPIKeyAuthResponse checks if the response body is JSON with code "Unauthenticated"
// (returned by our API key middleware when no X-API-Key header is present).
func isAPIKeyAuthResponse(t *testing.T, body []byte) bool {
	t.Helper()

	var errResp errorResponse

	if err := json.Unmarshal(body, &errResp); err != nil {
		return false
	}

	return errResp.Code == "Unauthenticated"
}

func TestNewAuthGuard(t *testing.T) {
	t.Parallel()

	cfg := AuthGuardConfig{
		APIKey:            "test-key",
		APIKeyEnabled:     true,
		PluginAuthEnabled: false,
		AppName:           "tracer",
	}

	guard := newTestAuthGuard(t, cfg, "")

	assert.NotNil(t, guard)
	assert.NotNil(t, guard.apiKeyAuth)
	assert.NotNil(t, guard.authClient)
	assert.Equal(t, cfg, guard.cfg)
}

func TestNewAuthGuard_ReturnsNilWhenPluginEnabledAndClientNil(t *testing.T) {
	t.Parallel()

	cfg := AuthGuardConfig{
		APIKey:            "test-key",
		APIKeyEnabled:     true,
		PluginAuthEnabled: true,
		AppName:           "tracer",
	}

	guard := NewAuthGuard(cfg, nil)

	assert.Nil(t, guard, "Expected nil guard when PluginAuthEnabled=true and authClient is nil")
}

func TestAuthGuard_Protect(t *testing.T) {
	t.Parallel()

	fakeServer := newFakeAuthServer(t)

	tests := []struct {
		name           string
		cfg            AuthGuardConfig
		apiKey         string
		setHeader      bool
		expectedStatus int
		expectedBody   string
	}{
		{
			name: "API key mode - valid key returns 200",
			cfg: AuthGuardConfig{
				APIKey:            "valid-key",
				APIKeyEnabled:     true,
				PluginAuthEnabled: false,
				AppName:           "tracer",
			},
			apiKey:         "valid-key",
			setHeader:      true,
			expectedStatus: http.StatusOK,
			expectedBody:   "success",
		},
		{
			name: "API key mode - missing key returns 401",
			cfg: AuthGuardConfig{
				APIKey:            "valid-key",
				APIKeyEnabled:     true,
				PluginAuthEnabled: false,
				AppName:           "tracer",
			},
			setHeader:      false,
			expectedStatus: http.StatusUnauthorized,
		},
		{
			name: "API key mode - invalid key returns 401",
			cfg: AuthGuardConfig{
				APIKey:            "valid-key",
				APIKeyEnabled:     true,
				PluginAuthEnabled: false,
				AppName:           "tracer",
			},
			apiKey:         "wrong-key",
			setHeader:      true,
			expectedStatus: http.StatusUnauthorized,
		},
		{
			name: "Plugin auth mode - missing Bearer token returns 401",
			cfg: AuthGuardConfig{
				APIKey:            "valid-key",
				APIKeyEnabled:     true,
				PluginAuthEnabled: true,
				AppName:           "tracer",
			},
			setHeader:      false,
			expectedStatus: http.StatusUnauthorized,
			expectedBody:   "Missing Token",
		},
		{
			name: "All auth disabled - passes through without key",
			cfg: AuthGuardConfig{
				APIKey:            "valid-key",
				APIKeyEnabled:     false,
				PluginAuthEnabled: false,
				AppName:           "tracer",
			},
			setHeader:      false,
			expectedStatus: http.StatusOK,
			expectedBody:   "success",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			serverURL := ""
			if tt.cfg.PluginAuthEnabled {
				serverURL = fakeServer.URL
			}

			guard := newTestAuthGuard(t, tt.cfg, serverURL)
			app := newTestApp(guard.Protect("rules", "get"))

			req := httptest.NewRequest(http.MethodGet, "/test", nil)
			if tt.setHeader {
				req.Header.Set(HeaderAPIKey, tt.apiKey)
			}

			resp, err := app.Test(req, -1)
			require.NoError(t, err)
			defer resp.Body.Close()

			assert.Equal(t, tt.expectedStatus, resp.StatusCode)

			if tt.expectedBody != "" {
				body, err := io.ReadAll(resp.Body)
				require.NoError(t, err)
				assert.Contains(t, string(body), tt.expectedBody)
			}
		})
	}
}

func TestAuthGuard_With(t *testing.T) {
	t.Parallel()

	fakeServer := newFakeAuthServer(t)

	tests := []struct {
		name           string
		cfg            AuthGuardConfig
		apiKeyParam    bool
		apiKey         string
		setHeader      bool
		expectedStatus int
		expectedBody   string
	}{
		{
			name: "apiKey=false, plugin disabled - valid API key returns 200",
			cfg: AuthGuardConfig{
				APIKey:            "valid-key",
				APIKeyEnabled:     true,
				PluginAuthEnabled: false,
				AppName:           "tracer",
			},
			apiKeyParam:    false,
			apiKey:         "valid-key",
			setHeader:      true,
			expectedStatus: http.StatusOK,
			expectedBody:   "success",
		},
		{
			name: "apiKey=false, plugin disabled - missing key returns 401",
			cfg: AuthGuardConfig{
				APIKey:            "valid-key",
				APIKeyEnabled:     true,
				PluginAuthEnabled: false,
				AppName:           "tracer",
			},
			apiKeyParam:    false,
			setHeader:      false,
			expectedStatus: http.StatusUnauthorized,
		},
		{
			name: "apiKey=false, plugin enabled - uses plugin auth (Missing Token)",
			cfg: AuthGuardConfig{
				APIKey:            "valid-key",
				APIKeyEnabled:     true,
				PluginAuthEnabled: true,
				AppName:           "tracer",
			},
			apiKeyParam:    false,
			setHeader:      false,
			expectedStatus: http.StatusUnauthorized,
			expectedBody:   "Missing Token",
		},
		{
			name: "apiKey=true, plugin enabled - bypasses plugin auth, valid API key returns 200",
			cfg: AuthGuardConfig{
				APIKey:            "valid-key",
				APIKeyEnabled:     true,
				PluginAuthEnabled: true,
				AppName:           "tracer",
			},
			apiKeyParam:    true,
			apiKey:         "valid-key",
			setHeader:      true,
			expectedStatus: http.StatusOK,
			expectedBody:   "success",
		},
		{
			name: "apiKey=true, plugin enabled - bypasses plugin auth, missing key returns API key 401",
			cfg: AuthGuardConfig{
				APIKey:            "valid-key",
				APIKeyEnabled:     true,
				PluginAuthEnabled: true,
				AppName:           "tracer",
			},
			apiKeyParam:    true,
			setHeader:      false,
			expectedStatus: http.StatusUnauthorized,
		},
		{
			name: "apiKey=true, all auth disabled - passes through",
			cfg: AuthGuardConfig{
				APIKey:            "valid-key",
				APIKeyEnabled:     false,
				PluginAuthEnabled: false,
				AppName:           "tracer",
			},
			apiKeyParam:    true,
			setHeader:      false,
			expectedStatus: http.StatusOK,
			expectedBody:   "success",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			serverURL := ""
			if tt.cfg.PluginAuthEnabled {
				serverURL = fakeServer.URL
			}

			guard := newTestAuthGuard(t, tt.cfg, serverURL)
			app := newTestApp(guard.With("validations", "post", tt.apiKeyParam))

			req := httptest.NewRequest(http.MethodGet, "/test", nil)
			if tt.setHeader {
				req.Header.Set(HeaderAPIKey, tt.apiKey)
			}

			resp, err := app.Test(req, -1)
			require.NoError(t, err)
			defer resp.Body.Close()

			assert.Equal(t, tt.expectedStatus, resp.StatusCode)

			if tt.expectedBody != "" {
				body, err := io.ReadAll(resp.Body)
				require.NoError(t, err)
				assert.Contains(t, string(body), tt.expectedBody)
			}
		})
	}
}

// TestAuthGuard_PluginAuthPriority verifies that plugin auth takes priority over API key.
// When PluginAuthEnabled=true, Protect() should return plugin auth middleware.
// Sending only an API key (no Bearer token) should trigger plugin auth's "Missing Token",
// proving that plugin auth was chosen instead of API key auth.
func TestAuthGuard_PluginAuthPriority(t *testing.T) {
	t.Parallel()

	fakeServer := newFakeAuthServer(t)

	guard := newTestAuthGuard(t, AuthGuardConfig{
		APIKey:            "valid-key",
		APIKeyEnabled:     true,
		PluginAuthEnabled: true,
		AppName:           "tracer",
	}, fakeServer.URL)

	app := newTestApp(guard.Protect("rules", "get"))

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set(HeaderAPIKey, "valid-key") // Valid API key, but no Bearer token

	resp, err := app.Test(req, -1)
	require.NoError(t, err)
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	// Plugin auth should be enforced: "Missing Token" instead of API key auth passing
	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
	assert.True(t, isPluginAuthResponse(t, body),
		"Expected plugin auth response 'Missing Token', got: %s", string(body))
}

// ---------------------------------------------------------------------------
// Principal extraction tests — verify the auth middleware stamps a
// contextutil.Principal carrying the JWT identity onto the Fiber UserContext
// when plugin auth accepts the request.
// ---------------------------------------------------------------------------

// newAuthorizedMockServer returns a fake Access Manager that authorizes every
// non-health request. Mirrors the deny-all server above but produces
// {Authorized: true} so the principal extraction flow can complete.
func newAuthorizedMockServer(t *testing.T) *httptest.Server {
	t.Helper()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/health" {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("healthy"))

			return
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(authMiddleware.AuthResponse{Authorized: true})
	}))

	t.Cleanup(server.Close)

	return server
}

// principalCaptureApp wraps the auth handler with a probe that returns the
// Principal stored in c.UserContext() as JSON so tests can assert on it.
func principalCaptureApp(authHandler fiber.Handler) *fiber.App {
	app := fiber.New()
	app.Get("/test", authHandler, func(c *fiber.Ctx) error {
		p, ok := contextutil.GetPrincipal(c.UserContext())

		return c.JSON(fiber.Map{
			"hasPrincipal": ok,
			"type":         p.Type,
			"id":           p.ID,
			"name":         p.Name,
		})
	})

	return app
}

// TestPrincipal_FromJWT_PreferredUsername verifies that on a successful
// authorized request, the middleware stores a Principal of type "user" with
// id=sub and name=preferred_username.
func TestPrincipal_FromJWT_PreferredUsername(t *testing.T) {
	t.Parallel()

	mock := newAuthorizedMockServer(t)

	guard := newTestAuthGuard(t, AuthGuardConfig{
		PluginAuthEnabled: true,
		AppName:           "tracer",
	}, mock.URL)
	require.NotNil(t, guard)

	app := principalCaptureApp(guard.Protect("rules", "manage"))

	token := makeJWT(t, jwt.MapClaims{
		"sub":                "user-sub-123",
		"preferred_username": "alice",
		"email":              "alice@example.com",
	})

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := app.Test(req, -1)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var got map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&got))

	assert.Equal(t, true, got["hasPrincipal"])
	assert.Equal(t, "user", got["type"])
	assert.Equal(t, "user-sub-123", got["id"])
	assert.Equal(t, "alice", got["name"], "preferred_username should be used as name")
}

// TestPrincipal_FromJWT_EmailFallback verifies that when preferred_username
// is absent, the middleware falls back to email as Principal.Name.
func TestPrincipal_FromJWT_EmailFallback(t *testing.T) {
	t.Parallel()

	mock := newAuthorizedMockServer(t)

	guard := newTestAuthGuard(t, AuthGuardConfig{
		PluginAuthEnabled: true,
		AppName:           "tracer",
	}, mock.URL)

	app := principalCaptureApp(guard.Protect("rules", "manage"))

	token := makeJWT(t, jwt.MapClaims{
		"sub":   "user-sub-456",
		"email": "bob@example.com",
	})

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := app.Test(req, -1)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var got map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&got))

	assert.Equal(t, "user", got["type"])
	assert.Equal(t, "user-sub-456", got["id"])
	assert.Equal(t, "bob@example.com", got["name"], "email should fall back as name")
}

// TestPrincipal_FromJWT_MissingSub_Returns401 verifies the strict-sub policy:
// a token without `sub` is rejected with 401 UNAUTHORIZED_MISSING_SUB BEFORE
// reaching lib-auth, so no Principal can ever be observed and no audit row
// would be silently attributed to the system actor.
func TestPrincipal_FromJWT_MissingSub_Returns401(t *testing.T) {
	t.Parallel()

	mock := newAuthorizedMockServer(t)

	guard := newTestAuthGuard(t, AuthGuardConfig{
		PluginAuthEnabled: true,
		AppName:           "tracer",
	}, mock.URL)

	app := principalCaptureApp(guard.Protect("rules", "manage"))

	token := makeJWT(t, jwt.MapClaims{
		"preferred_username": "claire",
	})

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := app.Test(req, -1)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	require.Equal(t, http.StatusUnauthorized, resp.StatusCode)

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	var errResp errorResponse
	require.NoError(t, json.Unmarshal(body, &errResp))
	assert.Equal(t, CodeUnauthorizedMissingSub, errResp.Code,
		"expected UNAUTHORIZED_MISSING_SUB to distinguish strict-sub rejection from other 401s")
}

// TestPrincipal_NoBearerHeader_DelegatesToLibAuth verifies that absent Bearer
// header does NOT short-circuit — lib-auth handles the "missing token" 401
// as before. Backward compatible: no behavior change for unauthenticated
// requests.
func TestPrincipal_NoBearerHeader_DelegatesToLibAuth(t *testing.T) {
	t.Parallel()

	mock := newAuthorizedMockServer(t)

	guard := newTestAuthGuard(t, AuthGuardConfig{
		PluginAuthEnabled: true,
		AppName:           "tracer",
	}, mock.URL)

	app := newTestApp(guard.Protect("rules", "manage"))

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	resp, err := app.Test(req, -1)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
	assert.True(t, isPluginAuthResponse(t, body),
		"no Bearer header must yield lib-auth's 'Missing Token' (not strict-sub 401)")
}

// TestPrincipal_MalformedBearer_DelegatesToLibAuth verifies that corrupt
// tokens are passed through to lib-auth instead of being rejected here.
// lib-auth has a more specific error from the Access Manager.
func TestPrincipal_MalformedBearer_DelegatesToLibAuth(t *testing.T) {
	t.Parallel()

	denyServer := newFakeAuthServer(t)

	guard := newTestAuthGuard(t, AuthGuardConfig{
		PluginAuthEnabled: true,
		AppName:           "tracer",
	}, denyServer.URL)

	app := newTestApp(guard.Protect("rules", "manage"))

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("Authorization", "Bearer not.a.valid.jwt")
	resp, err := app.Test(req, -1)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	// Mandatory status assertion BEFORE inspecting the payload. Without this,
	// an unexpected 200 (bypass bug) or 500 (panic) would let the test pass
	// silently as long as the body isn't a JSON envelope with our strict-sub
	// code — the deny-all fake auth server still returns SOME 401 body.
	require.Equal(t, http.StatusUnauthorized, resp.StatusCode,
		"malformed Bearer must still produce 401 (rejected by lib-auth downstream)")

	// Should NOT be UNAUTHORIZED_MISSING_SUB — extraction returns (false, nil)
	// and lib-auth gets to reject the malformed token itself.
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	var errResp errorResponse
	if json.Unmarshal(body, &errResp) == nil {
		assert.NotEqual(t, CodeUnauthorizedMissingSub, errResp.Code,
			"malformed Bearer must NOT short-circuit with UNAUTHORIZED_MISSING_SUB")
	}
}

// TestAuthGuard_DualMode verifies the dual auth mode behavior:
// When PluginAuthEnabled=true, apiKey=true should use API key only,
// while apiKey=false should use plugin auth.
func TestAuthGuard_DualMode(t *testing.T) {
	t.Parallel()

	fakeServer := newFakeAuthServer(t)

	guard := newTestAuthGuard(t, AuthGuardConfig{
		APIKey:            "valid-key",
		APIKeyEnabled:     true,
		PluginAuthEnabled: true,
		AppName:           "tracer",
	}, fakeServer.URL)

	t.Run("validation endpoint with apiKey=true uses API key auth", func(t *testing.T) {
		t.Parallel()

		app := newTestApp(guard.With("validations", "post", true))

		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		req.Header.Set(HeaderAPIKey, "valid-key")

		resp, err := app.Test(req, -1)
		require.NoError(t, err)
		defer resp.Body.Close()

		body, err := io.ReadAll(resp.Body)
		require.NoError(t, err)

		assert.Equal(t, http.StatusOK, resp.StatusCode,
			"With apiKey=true + valid API key, should return 200")
		assert.Equal(t, "success", string(body))
	})

	t.Run("validation endpoint with apiKey=true rejects missing key", func(t *testing.T) {
		t.Parallel()

		app := newTestApp(guard.With("validations", "post", true))

		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		// No API key and no Bearer token

		resp, err := app.Test(req, -1)
		require.NoError(t, err)
		defer resp.Body.Close()

		body, err := io.ReadAll(resp.Body)
		require.NoError(t, err)

		// Should get API key 401 (not plugin auth's "Missing Token")
		assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
		assert.True(t, isAPIKeyAuthResponse(t, body),
			"Expected API key auth JSON response, got: %s", string(body))
	})

	t.Run("other endpoint with apiKey=false uses plugin auth", func(t *testing.T) {
		t.Parallel()

		app := newTestApp(guard.With("rules", "get", false))

		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		req.Header.Set(HeaderAPIKey, "valid-key") // Valid API key, but no Bearer token

		resp, err := app.Test(req, -1)
		require.NoError(t, err)
		defer resp.Body.Close()

		body, err := io.ReadAll(resp.Body)
		require.NoError(t, err)

		// Should get plugin auth response, NOT API key auth passing
		assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
		assert.True(t, isPluginAuthResponse(t, body),
			"Expected plugin auth response 'Missing Token', got: %s", string(body))
	})
}
