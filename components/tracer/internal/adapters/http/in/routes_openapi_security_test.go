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

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/components/tracer/internal/adapters/http/in/middleware"
)

// openAPISpec is a minimal view of the served OpenAPI 3.1 document — enough to
// assert the auth-scheme declarations, per-operation security requirements, and
// the RFC 9457 problem model reference. The served bytes are json.Marshal of the
// typed huma.OpenAPI (see openapi.ServeSpec), so parsing /v1/openapi.json is the
// same object the huma.API would hand back, minus a runtime accessor we don't need.
type openAPISpec struct {
	Paths map[string]map[string]openAPIOperation `json:"paths"`

	Components struct {
		SecuritySchemes map[string]openAPISecurityScheme `json:"securitySchemes"`
		Schemas         map[string]openAPISchema         `json:"schemas"`
	} `json:"components"`
}

type openAPIOperation struct {
	// Security is a list of requirement objects; multiple entries mean OR.
	Security  []map[string][]string      `json:"security"`
	Responses map[string]openAPIResponse `json:"responses"`
}

type openAPIResponse struct {
	Content map[string]struct {
		Schema struct {
			Ref string `json:"$ref"`
		} `json:"schema"`
	} `json:"content"`
}

type openAPISecurityScheme struct {
	Type   string `json:"type"`
	Scheme string `json:"scheme"`
	In     string `json:"in"`
	Name   string `json:"name"`
}

type openAPISchema struct {
	Properties map[string]json.RawMessage `json:"properties"`
}

// fetchTracerSpec builds the tracer routes with the spec surface enabled and
// returns the parsed OpenAPI document served at /v1/openapi.json. The served
// bytes are json.Marshal(humaAPI.OpenAPI()), so this is the spec-lock proxy for
// the huma.API without adding a runtime accessor.
func fetchTracerSpec(t *testing.T) openAPISpec {
	t.Helper()

	guardCfg := middleware.AuthGuardConfig{
		APIKey:        "test-secret-key-32-characters-long",
		APIKeyEnabled: true,
		AppName:       "tracer",
	}
	deps := newTestRouterDeps(t, guardCfg)
	deps.swaggerEnabled = true // gate ServeSpec on
	app := deps.build()

	req := httptest.NewRequest(http.MethodGet, "/v1/openapi.json", nil)
	resp, err := app.Test(req, -1)
	require.NoError(t, err)
	defer func() { require.NoError(t, resp.Body.Close()) }()
	require.Equal(t, http.StatusOK, resp.StatusCode, "spec must be served when SwaggerEnabled=true")

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	var spec openAPISpec
	require.NoError(t, json.Unmarshal(body, &spec), "spec must be valid JSON; got: %s", string(body))

	return spec
}

// TestSpecLock_SecuritySchemes asserts the two auth schemes referenced by the
// per-op Security metadata are declared on the shared Huma API: BearerAuth (from
// openapi.DeclareBearerAuth) and ApiKeyAuth (declared locally). Without these the
// per-op security:[{BearerAuth:{}}] / {ApiKeyAuth:{}} entries would dangle.
func TestSpecLock_SecuritySchemes(t *testing.T) {
	spec := fetchTracerSpec(t)

	bearer, ok := spec.Components.SecuritySchemes["BearerAuth"]
	require.True(t, ok, "BearerAuth scheme must be declared")
	assert.Equal(t, "http", bearer.Type, "BearerAuth.type")
	assert.Equal(t, "bearer", bearer.Scheme, "BearerAuth.scheme")

	apiKey, ok := spec.Components.SecuritySchemes["ApiKeyAuth"]
	require.True(t, ok, "ApiKeyAuth scheme must be declared")
	assert.Equal(t, "apiKey", apiKey.Type, "ApiKeyAuth.type")
	assert.Equal(t, "header", apiKey.In, "ApiKeyAuth.in")
	assert.Equal(t, "X-API-Key", apiKey.Name, "ApiKeyAuth.name")
}

// TestSpecLock_PerOpSecurity spot-checks the three security shapes that matter:
//   - a Bearer|ApiKey op (GET /rules): two requirement entries = OR.
//   - the POST /validations hot path: ApiKeyAuth-only (no bearer).
//   - the public endpoints (health/readyz/version): NOT present as Huma ops.
func TestSpecLock_PerOpSecurity(t *testing.T) {
	spec := fetchTracerSpec(t)

	// (a) GET /rules accepts EITHER bearer OR api key — two OR requirement entries.
	rulesGet, ok := op(spec, "/rules", http.MethodGet)
	require.True(t, ok, "GET /rules must be a registered Huma op")
	assert.ElementsMatch(t,
		[]map[string][]string{{"BearerAuth": {}}, {"ApiKeyAuth": {}}},
		rulesGet.Security,
		"GET /rules must advertise BearerAuth OR ApiKeyAuth")

	// (b) POST /validations is the API-key-only hot path — no bearer alternative.
	validationsPost, ok := op(spec, "/validations", http.MethodPost)
	require.True(t, ok, "POST /validations must be a registered Huma op")
	assert.Equal(t,
		[]map[string][]string{{"ApiKeyAuth": {}}},
		validationsPost.Security,
		"POST /validations must be ApiKeyAuth-only")

	// (c) The three public endpoints are Fiber-only (no Huma op). Their auth is
	// "none", and they must not surface as protected Huma operations in the spec.
	for _, p := range []string{"/health", "/readyz", "/version"} {
		_, present := spec.Paths[p]
		assert.Falsef(t, present, "public endpoint %s must NOT appear as a Huma op in the spec", p)
	}
}

// TestSpecLock_ErrorSchemaIsProblemDetail asserts every operation's error
// response references the RFC 9457 problem model (#/components/schemas/Detail)
// under application/problem+json, and that the Detail schema carries the RFC 9457
// members type/title/status/detail/code. This is the problem.Install() contract
// surfaced in the spec.
func TestSpecLock_ErrorSchemaIsProblemDetail(t *testing.T) {
	spec := fetchTracerSpec(t)

	detail, ok := spec.Components.Schemas["Detail"]
	require.True(t, ok, "problem model schema 'Detail' must be declared")
	for _, field := range []string{"type", "title", "status", "detail", "code"} {
		_, present := detail.Properties[field]
		assert.Truef(t, present, "Detail schema must carry RFC 9457 member %q", field)
	}

	// Every op's error response must $ref the Detail schema via problem+json.
	rulesGet, ok := op(spec, "/rules", http.MethodGet)
	require.True(t, ok)
	errResp, ok := rulesGet.Responses["default"]
	require.True(t, ok, "GET /rules must declare a default (error) response")
	problemJSON, ok := errResp.Content["application/problem+json"]
	require.True(t, ok, "error response must use application/problem+json")
	assert.Equal(t, "#/components/schemas/Detail", problemJSON.Schema.Ref,
		"error response must reference the RFC 9457 problem model")
}

// op returns the operation for a method+path from the parsed spec.
func op(spec openAPISpec, path, method string) (openAPIOperation, bool) {
	methods, ok := spec.Paths[path]
	if !ok {
		return openAPIOperation{}, false
	}

	o, ok := methods[lower(method)]

	return o, ok
}

// lower is a tiny helper so op() reads cleanly; OpenAPI method keys are lowercase.
func lower(method string) string {
	switch method {
	case http.MethodGet:
		return "get"
	case http.MethodPost:
		return "post"
	case http.MethodPatch:
		return "patch"
	case http.MethodDelete:
		return "delete"
	case http.MethodPut:
		return "put"
	default:
		return method
	}
}
