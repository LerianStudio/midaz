// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/components/tracer/internal/adapters/http/in/middleware"
)

// TestRoutes_ServeSpec_GatedByOpenAPIDocsEnabled proves the native Huma OpenAPI 3.1
// spec + docs surface (openapi.ServeSpec) is mounted under /v1 ONLY when
// RouteConfig.OpenAPIDocsEnabled is true, and absent (404) when false.
func TestRoutes_ServeSpec_GatedByOpenAPIDocsEnabled(t *testing.T) {
	guardCfg := middleware.AuthGuardConfig{
		APIKey:        "test-secret-key-32-characters-long",
		APIKeyEnabled: true,
		AppName:       "tracer",
	}

	cases := []struct {
		name               string
		openAPIDocsEnabled bool
		wantStatus         int
	}{
		{"enabled mounts spec+docs", true, http.StatusOK},
		{"disabled omits spec+docs", false, http.StatusNotFound},
	}

	// The native Huma spec routes ServeSpec mounts under the "/v1" prefix.
	specPaths := []string{"/v1/openapi.yaml", "/v1/openapi.json", "/v1/docs"}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			deps := newTestRouterDeps(t, guardCfg)
			deps.openAPIDocsEnabled = tc.openAPIDocsEnabled
			app := deps.build()

			for _, p := range specPaths {
				req := httptest.NewRequest(http.MethodGet, p, nil)
				resp, err := app.Test(req, fiber.TestConfig{Timeout: 0})
				require.NoError(t, err)
				require.NoError(t, resp.Body.Close())

				assert.Equalf(t, tc.wantStatus, resp.StatusCode,
					"OpenAPIDocsEnabled=%v: GET %s expected %d, got %d",
					tc.openAPIDocsEnabled, p, tc.wantStatus, resp.StatusCode)
			}
		})
	}
}
