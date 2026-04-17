// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"context"
	nethttp "net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/lib-auth/v2/auth/middleware"
	libLog "github.com/LerianStudio/lib-commons/v2/commons/log"
	libOpenTelemetry "github.com/LerianStudio/lib-commons/v2/commons/opentelemetry"

	"github.com/LerianStudio/midaz/v3/components/crm/api"
	"github.com/LerianStudio/midaz/v3/components/crm/internal/services"
)

// buildTelemetry returns a disabled Telemetry instance suitable for unit tests.
// EnableTelemetry=false short-circuits the OTLP exporter setup so no collector
// is needed.
func buildTelemetry(t *testing.T) *libOpenTelemetry.Telemetry {
	t.Helper()

	logger := &libLog.GoLogger{Level: libLog.InfoLevel}

	tl, err := libOpenTelemetry.InitializeTelemetryWithError(&libOpenTelemetry.TelemetryConfig{
		LibraryName:     "test",
		ServiceName:     "crm-test",
		EnableTelemetry: false,
		Logger:          logger,
	})
	require.NoError(t, err)

	return tl
}

// TestNewRouter_RegistersAllRoutes constructs the Fiber app via NewRouter and
// inspects the registered route stack. This proves:
//   - NewRouter builds without panicking
//   - Every declared endpoint is registered on the expected method/path
//   - Middleware chain (ErrorCodeTransformer, recover, telemetry, CORS, logging)
//     does not interfere with route registration
//
// Auth is configured with enabled=false so the Authorize middleware is a no-op
// and no network call is made.
func TestNewRouter_RegistersAllRoutes(t *testing.T) {
	t.Parallel()

	var logger libLog.Logger = &libLog.GoLogger{Level: libLog.InfoLevel}

	tl := buildTelemetry(t)

	auth := middleware.NewAuthClient("", false, &logger)

	uc := &services.UseCase{}
	holderHandler := &HolderHandler{Service: uc}
	aliasHandler := &AliasHandler{Service: uc}

	app := NewRouter(logger, tl, auth, holderHandler, aliasHandler)

	require.NotNil(t, app)

	type routeSpec struct {
		method string
		path   string
	}

	expected := []routeSpec{
		{fiber.MethodPost, "/v1/holders"},
		{fiber.MethodGet, "/v1/holders/:id"},
		{fiber.MethodPatch, "/v1/holders/:id"},
		{fiber.MethodDelete, "/v1/holders/:id"},
		{fiber.MethodGet, "/v1/holders"},
		{fiber.MethodGet, "/v1/aliases"},
		{fiber.MethodPost, "/v1/holders/:holder_id/aliases"},
		{fiber.MethodGet, "/v1/holders/:holder_id/aliases/:id"},
		{fiber.MethodPatch, "/v1/holders/:holder_id/aliases/:id"},
		{fiber.MethodDelete, "/v1/holders/:holder_id/aliases/:id"},
		{fiber.MethodDelete, "/v1/holders/:holder_id/aliases/:alias_id/related-parties/:related_party_id"},
		{fiber.MethodGet, "/health"},
		{fiber.MethodGet, "/version"},
	}

	got := make(map[string]map[string]bool) // method -> path -> registered?

	for _, stack := range app.Stack() {
		for _, r := range stack {
			if got[r.Method] == nil {
				got[r.Method] = make(map[string]bool)
			}

			got[r.Method][r.Path] = true
		}
	}

	for _, e := range expected {
		assert.Truef(t, got[e.method][e.path], "route %s %s should be registered", e.method, e.path)
	}
}

// TestNewRouter_HealthEndpointResponds exercises one of the registered routes
// end-to-end. We pick /health because it is provided by libCommons and avoids
// any dependency on handler-level logic covered by other tests. This also
// ensures the middleware chain executes without error for a real request.
func TestNewRouter_HealthEndpointResponds(t *testing.T) {
	t.Parallel()

	var logger libLog.Logger = &libLog.GoLogger{Level: libLog.InfoLevel}

	tl := buildTelemetry(t)

	auth := middleware.NewAuthClient("", false, &logger)

	uc := &services.UseCase{}
	app := NewRouter(logger, tl, auth, &HolderHandler{Service: uc}, &AliasHandler{Service: uc})

	req := httptest.NewRequestWithContext(context.Background(), nethttp.MethodGet, "/health", nil)
	resp, err := app.Test(req)

	require.NoError(t, err)

	defer resp.Body.Close()

	// Ping returns 200 OK when the service is up.
	assert.Equal(t, nethttp.StatusOK, resp.StatusCode)
}

// TestWithSwaggerEnvConfig_NoEnvVarsKeepsDefaults verifies the middleware
// leaves the Swagger config untouched when environment variables are unset.
// Because the middleware mutates package-level api.SwaggerInfocrm (Swagger
// generator output), we cannot run this test in parallel with the env-var
// variants below.
func TestWithSwaggerEnvConfig_NoEnvVarsKeepsDefaults(t *testing.T) {
	// Snapshot and restore SwaggerInfocrm so other tests see its original
	// state regardless of run order.
	saved := *api.SwaggerInfocrm

	t.Cleanup(func() { *api.SwaggerInfocrm = saved })

	// Unset all env vars the middleware reads so we drive the "nothing to do"
	// branch.
	for _, k := range swaggerEnvKeys() {
		if err := os.Unsetenv(k); err != nil {
			t.Fatalf("unsetenv %s: %v", k, err)
		}
	}

	app := fiber.New()
	app.Get("/swagger/*", WithSwaggerEnvConfig(), func(c *fiber.Ctx) error {
		return c.SendString("ok")
	})

	req := httptest.NewRequestWithContext(context.Background(), nethttp.MethodGet, "/swagger/index.html", nil)
	resp, err := app.Test(req)

	require.NoError(t, err)

	defer resp.Body.Close()

	assert.Equal(t, nethttp.StatusOK, resp.StatusCode)

	// With no env vars set, defaults from the saved snapshot must be preserved.
	assert.Equal(t, saved.Title, api.SwaggerInfocrm.Title)
	assert.Equal(t, saved.Description, api.SwaggerInfocrm.Description)
}

// TestWithSwaggerEnvConfig_OverridesFromEnv verifies that each documented env
// var maps onto its corresponding SwaggerInfocrm field. SWAGGER_HOST is
// exercised with a valid host so the ValidateServerAddress check passes. We
// also hit the schemes branch by setting SWAGGER_SCHEMES.
func TestWithSwaggerEnvConfig_OverridesFromEnv(t *testing.T) {
	saved := *api.SwaggerInfocrm

	t.Cleanup(func() { *api.SwaggerInfocrm = saved })

	t.Setenv("SWAGGER_TITLE", "CRM API Test")
	t.Setenv("SWAGGER_DESCRIPTION", "Testing description")
	t.Setenv("SWAGGER_VERSION", "9.9.9")
	// ValidateServerAddress rejects bare hostnames without a port. Use a
	// host:port pair that should pass validation.
	t.Setenv("SWAGGER_HOST", "api.example.com:8080")
	t.Setenv("SWAGGER_BASE_PATH", "/v1")
	t.Setenv("SWAGGER_LEFT_DELIM", "<<")
	t.Setenv("SWAGGER_RIGHT_DELIM", ">>")
	t.Setenv("SWAGGER_SCHEMES", "https")

	app := fiber.New()
	app.Get("/swagger/*", WithSwaggerEnvConfig(), func(c *fiber.Ctx) error {
		return c.SendString("ok")
	})

	req := httptest.NewRequestWithContext(context.Background(), nethttp.MethodGet, "/swagger/index.html", nil)
	resp, err := app.Test(req)

	require.NoError(t, err)

	defer resp.Body.Close()

	assert.Equal(t, nethttp.StatusOK, resp.StatusCode)

	assert.Equal(t, "CRM API Test", api.SwaggerInfocrm.Title)
	assert.Equal(t, "Testing description", api.SwaggerInfocrm.Description)
	assert.Equal(t, "9.9.9", api.SwaggerInfocrm.Version)
	assert.Equal(t, "/v1", api.SwaggerInfocrm.BasePath)
	assert.Equal(t, "<<", api.SwaggerInfocrm.LeftDelim)
	assert.Equal(t, ">>", api.SwaggerInfocrm.RightDelim)
	assert.Equal(t, []string{"https"}, api.SwaggerInfocrm.Schemes)
}

// TestWithSwaggerEnvConfig_SkipsInvalidHost covers the SWAGGER_HOST validation
// branch: when the value fails ValidateServerAddress, the middleware MUST
// leave the existing Host untouched rather than write garbage into the spec.
func TestWithSwaggerEnvConfig_SkipsInvalidHost(t *testing.T) {
	saved := *api.SwaggerInfocrm

	t.Cleanup(func() { *api.SwaggerInfocrm = saved })

	api.SwaggerInfocrm.Host = "preserved-host:1234"

	// A bare hostname has no port separator and is rejected by
	// commons.ValidateServerAddress. The middleware must skip it.
	t.Setenv("SWAGGER_HOST", "not a valid host")

	app := fiber.New()
	app.Get("/swagger/*", WithSwaggerEnvConfig(), func(c *fiber.Ctx) error {
		return c.SendString("ok")
	})

	req := httptest.NewRequestWithContext(context.Background(), nethttp.MethodGet, "/swagger/index.html", nil)
	resp, err := app.Test(req)

	require.NoError(t, err)

	defer resp.Body.Close()

	assert.Equal(t, "preserved-host:1234", api.SwaggerInfocrm.Host, "invalid SWAGGER_HOST must not overwrite existing host")
}

// swaggerEnvKeys returns the list of env var names consulted by
// WithSwaggerEnvConfig, used to clean state between tests.
func swaggerEnvKeys() []string {
	return []string{
		"SWAGGER_TITLE",
		"SWAGGER_DESCRIPTION",
		"SWAGGER_VERSION",
		"SWAGGER_HOST",
		"SWAGGER_BASE_PATH",
		"SWAGGER_LEFT_DELIM",
		"SWAGGER_RIGHT_DELIM",
		"SWAGGER_SCHEMES",
	}
}
