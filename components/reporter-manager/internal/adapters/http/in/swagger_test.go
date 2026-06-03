// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/LerianStudio/reporter/components/manager/api"
	"github.com/gofiber/fiber/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWithSwaggerEnvConfig(t *testing.T) {
	// NOTE: Cannot use t.Parallel() because this test modifies global api.SwaggerInfo state
	originalTitle := api.SwaggerInfo.Title
	originalDescription := api.SwaggerInfo.Description
	originalVersion := api.SwaggerInfo.Version
	originalHost := api.SwaggerInfo.Host
	originalBasePath := api.SwaggerInfo.BasePath
	originalSchemes := api.SwaggerInfo.Schemes

	defer func() {
		api.SwaggerInfo.Title = originalTitle
		api.SwaggerInfo.Description = originalDescription
		api.SwaggerInfo.Version = originalVersion
		api.SwaggerInfo.Host = originalHost
		api.SwaggerInfo.BasePath = originalBasePath
		api.SwaggerInfo.Schemes = originalSchemes
	}()

	tests := []struct {
		name           string
		envVars        map[string]string
		expectedTitle  string
		expectedDesc   string
		expectedVer    string
		expectedHost   string
		expectedBase   string
		expectedScheme []string
	}{
		{
			name:           "No environment variables set",
			envVars:        map[string]string{},
			expectedTitle:  originalTitle,
			expectedDesc:   originalDescription,
			expectedVer:    originalVersion,
			expectedHost:   originalHost,
			expectedBase:   originalBasePath,
			expectedScheme: originalSchemes,
		},
		{
			name: "All environment variables set",
			envVars: map[string]string{
				"SWAGGER_TITLE":       "Test API",
				"SWAGGER_DESCRIPTION": "Test Description",
				"SWAGGER_VERSION":     "2.0.0",
				"SWAGGER_HOST":        "localhost:8080",
				"SWAGGER_BASE_PATH":   "/api/v2",
				"SWAGGER_SCHEMES":     "https",
			},
			expectedTitle:  "Test API",
			expectedDesc:   "Test Description",
			expectedVer:    "2.0.0",
			expectedHost:   "localhost:8080",
			expectedBase:   "/api/v2",
			expectedScheme: []string{"https"},
		},
		{
			name: "Only title set",
			envVars: map[string]string{
				"SWAGGER_TITLE": "Custom Title",
			},
			expectedTitle:  "Custom Title",
			expectedDesc:   originalDescription,
			expectedVer:    originalVersion,
			expectedHost:   originalHost,
			expectedBase:   originalBasePath,
			expectedScheme: originalSchemes,
		},
		{
			name: "Invalid host format is ignored",
			envVars: map[string]string{
				"SWAGGER_HOST": "not a valid host format!!!",
			},
			expectedTitle:  originalTitle,
			expectedDesc:   originalDescription,
			expectedVer:    originalVersion,
			expectedHost:   originalHost,
			expectedBase:   originalBasePath,
			expectedScheme: originalSchemes,
		},
		{
			name: "Valid host with port",
			envVars: map[string]string{
				"SWAGGER_HOST": "api.example.com:443",
			},
			expectedTitle:  originalTitle,
			expectedDesc:   originalDescription,
			expectedVer:    originalVersion,
			expectedHost:   "api.example.com:443",
			expectedBase:   originalBasePath,
			expectedScheme: originalSchemes,
		},
		{
			name: "Multiple schemes",
			envVars: map[string]string{
				"SWAGGER_SCHEMES": "http",
			},
			expectedTitle:  originalTitle,
			expectedDesc:   originalDescription,
			expectedVer:    originalVersion,
			expectedHost:   originalHost,
			expectedBase:   originalBasePath,
			expectedScheme: []string{"http"},
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			api.SwaggerInfo.Title = originalTitle
			api.SwaggerInfo.Description = originalDescription
			api.SwaggerInfo.Version = originalVersion
			api.SwaggerInfo.Host = originalHost
			api.SwaggerInfo.BasePath = originalBasePath
			api.SwaggerInfo.Schemes = originalSchemes

			for key, value := range tt.envVars {
				t.Setenv(key, value)
			}

			app := fiber.New(fiber.Config{
				DisableStartupMessage: true,
			})

			app.Get("/swagger/*", WithSwaggerEnvConfig(), func(c *fiber.Ctx) error {
				return c.SendStatus(http.StatusOK)
			})

			req := httptest.NewRequest(http.MethodGet, "/swagger/index.html", nil)
			resp, err := app.Test(req)

			require.NoError(t, err)
			defer resp.Body.Close()

			assert.Equal(t, http.StatusOK, resp.StatusCode)
			assert.Equal(t, tt.expectedTitle, api.SwaggerInfo.Title)
			assert.Equal(t, tt.expectedDesc, api.SwaggerInfo.Description)
			assert.Equal(t, tt.expectedVer, api.SwaggerInfo.Version)
			assert.Equal(t, tt.expectedHost, api.SwaggerInfo.Host)
			assert.Equal(t, tt.expectedBase, api.SwaggerInfo.BasePath)

			if len(tt.envVars["SWAGGER_SCHEMES"]) > 0 {
				assert.Equal(t, tt.expectedScheme, api.SwaggerInfo.Schemes)
			}
		})
	}
}

func TestWithSwaggerEnvConfig_EmptyValues(t *testing.T) {
	// NOTE: Cannot use t.Parallel() because this test modifies global api.SwaggerInfo state and uses t.Setenv.
	originalTitle := api.SwaggerInfo.Title

	defer func() {
		api.SwaggerInfo.Title = originalTitle
	}()

	t.Setenv("SWAGGER_TITLE", "")

	app := fiber.New(fiber.Config{
		DisableStartupMessage: true,
	})

	app.Get("/swagger/*", WithSwaggerEnvConfig(), func(c *fiber.Ctx) error {
		return c.SendStatus(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/swagger/index.html", nil)
	resp, err := app.Test(req)

	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, originalTitle, api.SwaggerInfo.Title)
}

func TestWithSwaggerEnvConfig_DelimiterSettings(t *testing.T) {
	// NOTE: Cannot use t.Parallel() because this test modifies global api.SwaggerInfo state and uses t.Setenv.
	originalLeftDelim := api.SwaggerInfo.LeftDelim
	originalRightDelim := api.SwaggerInfo.RightDelim

	defer func() {
		api.SwaggerInfo.LeftDelim = originalLeftDelim
		api.SwaggerInfo.RightDelim = originalRightDelim
	}()

	t.Setenv("SWAGGER_LEFT_DELIM", "[[")
	t.Setenv("SWAGGER_RIGHT_DELIM", "]]")

	app := fiber.New(fiber.Config{
		DisableStartupMessage: true,
	})

	app.Get("/swagger/*", WithSwaggerEnvConfig(), func(c *fiber.Ctx) error {
		return c.SendStatus(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/swagger/index.html", nil)
	resp, err := app.Test(req)

	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, "[[", api.SwaggerInfo.LeftDelim)
	assert.Equal(t, "]]", api.SwaggerInfo.RightDelim)
}

func TestSwaggerSpec_SecurityDefinitions(t *testing.T) {
	t.Parallel()

	_, currentFile, _, ok := runtime.Caller(0)
	require.True(t, ok, "failed to get caller information")

	// Navigate from the test file to the swagger.json location:
	// test file:   components/manager/internal/adapters/http/in/swagger_test.go
	// swagger.json: components/manager/api/swagger.json
	swaggerPath := filepath.Join(filepath.Dir(currentFile), "..", "..", "..", "..", "api", "swagger.json")

	swaggerBytes, err := os.ReadFile(swaggerPath)
	require.NoError(t, err, "failed to read swagger.json at %s", swaggerPath)

	var spec map[string]any
	err = json.Unmarshal(swaggerBytes, &spec)
	require.NoError(t, err, "failed to parse swagger.json")

	tests := []struct {
		name      string
		assertion func(t *testing.T)
	}{
		{
			name: "securityDefinitions contains BearerAuth",
			assertion: func(t *testing.T) {
				t.Helper()

				secDefs, exists := spec["securityDefinitions"]
				require.True(t, exists, "swagger.json must contain securityDefinitions")

				secDefsMap, ok := secDefs.(map[string]any)
				require.True(t, ok, "securityDefinitions must be an object")

				bearerAuth, exists := secDefsMap["BearerAuth"]
				require.True(t, exists, "securityDefinitions must contain BearerAuth")

				bearerAuthMap, ok := bearerAuth.(map[string]any)
				require.True(t, ok, "BearerAuth must be an object")

				assert.Equal(t, "apiKey", bearerAuthMap["type"], "BearerAuth type must be apiKey")
				assert.Equal(t, "Authorization", bearerAuthMap["name"], "BearerAuth name must be Authorization")
				assert.Equal(t, "header", bearerAuthMap["in"], "BearerAuth in must be header")
			},
		},
		{
			name: "endpoints use security instead of Authorization parameter",
			assertion: func(t *testing.T) {
				t.Helper()

				paths, exists := spec["paths"]
				require.True(t, exists, "swagger.json must contain paths")

				pathsMap, ok := paths.(map[string]any)
				require.True(t, ok, "paths must be an object")

				for pathName, pathItem := range pathsMap {
					pathItemMap, ok := pathItem.(map[string]any)
					require.True(t, ok, "path item %s must be an object", pathName)

					for method, operation := range pathItemMap {
						operationMap, ok := operation.(map[string]any)
						if !ok {
							continue
						}

						// Check that no endpoint has Authorization as a header parameter
						if params, hasParams := operationMap["parameters"]; hasParams {
							paramsSlice, ok := params.([]any)
							if ok {
								for _, param := range paramsSlice {
									paramMap, ok := param.(map[string]any)
									if !ok {
										continue
									}

									if paramMap["name"] == "Authorization" && paramMap["in"] == "header" {
										t.Errorf(
											"%s %s still has @Param Authorization header; expected @Security BearerAuth instead",
											method, pathName,
										)
									}
								}
							}
						}

						// Check that the endpoint has a security section with BearerAuth
						security, hasSecurity := operationMap["security"]
						require.True(t, hasSecurity,
							"%s %s must have a security section with BearerAuth", method, pathName)

						securitySlice, ok := security.([]any)
						require.True(t, ok, "%s %s security must be an array", method, pathName)

						found := false
						for _, secItem := range securitySlice {
							secItemMap, ok := secItem.(map[string]any)
							if !ok {
								continue
							}

							if _, hasBearerAuth := secItemMap["BearerAuth"]; hasBearerAuth {
								found = true

								break
							}
						}

						assert.True(t, found,
							"%s %s security must reference BearerAuth", method, pathName)
					}
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			tt.assertion(t)
		})
	}
}
