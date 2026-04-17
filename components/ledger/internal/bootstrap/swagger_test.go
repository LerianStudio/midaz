// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v3/components/ledger/api"
)

// installSwaggerMiddlewareRoute wires WithSwaggerEnvConfig in front of a
// trivial handler so tests can exercise the middleware in isolation and
// assert on its status-code contract.
func installSwaggerMiddlewareRoute(t *testing.T) *fiber.App {
	t.Helper()

	app := fiber.New()
	app.Get("/swagger/*", WithSwaggerEnvConfig(), func(c *fiber.Ctx) error {
		return c.SendString("ok")
	})

	return app
}

// TestWithSwaggerEnvConfig_DisabledReturns404 verifies that when
// SWAGGER_ENABLED is explicitly false the middleware short-circuits with
// 404 before reaching the downstream handler.
func TestWithSwaggerEnvConfig_DisabledReturns404(t *testing.T) {
	// No t.Parallel — we mutate env vars.
	t.Setenv("SWAGGER_ENABLED", "false")
	t.Setenv("SWAGGER_AUTH_TOKEN", "")

	app := installSwaggerMiddlewareRoute(t)

	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/swagger/index.html", http.NoBody)

	resp, err := app.Test(req)
	require.NoError(t, err)

	defer resp.Body.Close()

	assert.Equal(t, http.StatusNotFound, resp.StatusCode)
}

// TestWithSwaggerEnvConfig_EnabledNoTokenReturns200 verifies the allow-path
// when SWAGGER_ENABLED=true and no SWAGGER_AUTH_TOKEN is configured.
func TestWithSwaggerEnvConfig_EnabledNoTokenReturns200(t *testing.T) {
	t.Setenv("SWAGGER_ENABLED", "true")
	t.Setenv("SWAGGER_AUTH_TOKEN", "")

	app := installSwaggerMiddlewareRoute(t)

	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/swagger/index.html", http.NoBody)

	resp, err := app.Test(req)
	require.NoError(t, err)

	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)
}

// TestWithSwaggerEnvConfig_WrongTokenReturns401 verifies that a mismatched
// SWAGGER_AUTH_TOKEN is rejected with 401 before the handler runs.
func TestWithSwaggerEnvConfig_WrongTokenReturns401(t *testing.T) {
	t.Setenv("SWAGGER_ENABLED", "true")
	t.Setenv("SWAGGER_AUTH_TOKEN", "correct-token")

	app := installSwaggerMiddlewareRoute(t)

	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/swagger/index.html", http.NoBody)
	req.Header.Set("X-Swagger-Token", "wrong-token")

	resp, err := app.Test(req)
	require.NoError(t, err)

	defer resp.Body.Close()

	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
}

// TestWithSwaggerEnvConfig_CorrectTokenReturns200 verifies the happy path
// with a valid SWAGGER_AUTH_TOKEN.
func TestWithSwaggerEnvConfig_CorrectTokenReturns200(t *testing.T) {
	t.Setenv("SWAGGER_ENABLED", "true")
	t.Setenv("SWAGGER_AUTH_TOKEN", "correct-token")

	app := installSwaggerMiddlewareRoute(t)

	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/swagger/index.html", http.NoBody)
	req.Header.Set("X-Swagger-Token", "correct-token")

	resp, err := app.Test(req)
	require.NoError(t, err)

	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)
}

// TestInitSwaggerFromEnv_AppliesOverrides calls initSwaggerFromEnv directly
// (bypassing the sync.Once guard inside the middleware, which is only
// triggered on the first allowed request in the whole process) to verify the
// env → api.SwaggerInfo mapping. We save and restore the global so parallel
// tests do not observe the mutation after we return.
func TestInitSwaggerFromEnv_AppliesOverrides(t *testing.T) {
	saved := *api.SwaggerInfo

	t.Cleanup(func() {
		*api.SwaggerInfo = saved
	})

	// Valid host (satisfies ValidateServerAddress), custom schemes, plus
	// populated string fields to exercise every branch in the loop.
	t.Setenv("SWAGGER_TITLE", "Test Title")
	t.Setenv("SWAGGER_DESCRIPTION", "Test Description")
	t.Setenv("SWAGGER_VERSION", "v9.9.9")
	t.Setenv("SWAGGER_HOST", "example.com:9090")
	t.Setenv("SWAGGER_BASE_PATH", "/test")
	t.Setenv("SWAGGER_LEFT_DELIM", "<<")
	t.Setenv("SWAGGER_RIGHT_DELIM", ">>")
	t.Setenv("SWAGGER_SCHEMES", "https, wss ,  ")

	initSwaggerFromEnv()

	assert.Equal(t, "Test Title", api.SwaggerInfo.Title)
	assert.Equal(t, "Test Description", api.SwaggerInfo.Description)
	assert.Equal(t, "v9.9.9", api.SwaggerInfo.Version)
	assert.Equal(t, "example.com:9090", api.SwaggerInfo.Host)
	assert.Equal(t, "/test", api.SwaggerInfo.BasePath)
	assert.Equal(t, "<<", api.SwaggerInfo.LeftDelim)
	assert.Equal(t, ">>", api.SwaggerInfo.RightDelim)
	// Whitespace-only tokens in the CSV must be dropped, keeping only
	// the non-empty schemes.
	assert.Equal(t, []string{"https", "wss"}, api.SwaggerInfo.Schemes)
}

// TestInitSwaggerFromEnv_InvalidHostIgnored verifies that an invalid
// SWAGGER_HOST value does not overwrite api.SwaggerInfo.Host. This is a
// documented defence-in-depth branch in initSwaggerFromEnv.
func TestInitSwaggerFromEnv_InvalidHostIgnored(t *testing.T) {
	saved := *api.SwaggerInfo

	t.Cleanup(func() {
		*api.SwaggerInfo = saved
	})

	originalHost := api.SwaggerInfo.Host

	t.Setenv("SWAGGER_HOST", "://not a valid host::")

	initSwaggerFromEnv()

	assert.Equal(t, originalHost, api.SwaggerInfo.Host)
}

// TestInitSwaggerFromEnv_EmptyValuesNoOp verifies that when no env overrides
// are set the function leaves api.SwaggerInfo untouched. This guards against
// accidental zeroing of the defaults.
func TestInitSwaggerFromEnv_EmptyValuesNoOp(t *testing.T) {
	saved := *api.SwaggerInfo

	t.Cleanup(func() {
		*api.SwaggerInfo = saved
	})

	t.Setenv("SWAGGER_TITLE", "")
	t.Setenv("SWAGGER_DESCRIPTION", "")
	t.Setenv("SWAGGER_VERSION", "")
	t.Setenv("SWAGGER_HOST", "")
	t.Setenv("SWAGGER_BASE_PATH", "")
	t.Setenv("SWAGGER_LEFT_DELIM", "")
	t.Setenv("SWAGGER_RIGHT_DELIM", "")
	t.Setenv("SWAGGER_SCHEMES", "")

	initSwaggerFromEnv()

	assert.Equal(t, saved.Title, api.SwaggerInfo.Title)
	assert.Equal(t, saved.Description, api.SwaggerInfo.Description)
	assert.Equal(t, saved.Version, api.SwaggerInfo.Version)
	assert.Equal(t, saved.Host, api.SwaggerInfo.Host)
	assert.Equal(t, saved.BasePath, api.SwaggerInfo.BasePath)
	assert.Equal(t, saved.LeftDelim, api.SwaggerInfo.LeftDelim)
	assert.Equal(t, saved.RightDelim, api.SwaggerInfo.RightDelim)
	assert.Equal(t, saved.Schemes, api.SwaggerInfo.Schemes)
}
