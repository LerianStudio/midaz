// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"net/http"
	"testing"

	"github.com/LerianStudio/lib-auth/v4/auth/middleware"
	libOpentelemetry "github.com/LerianStudio/lib-observability/v4/tracing"
	"github.com/danielgtaylor/huma/v2"
	"github.com/gofiber/fiber/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	httpin "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/http/in"
)

// newSingleDocServer builds a unified server mounting a v1 op (organizations) and a
// v2 op (transactions/direct) through the production seams. Both prefixes carry at
// least one operation so the served document can be checked for BOTH path families.
// A zero-value handler is safe: registration stores handler funcs and never calls
// them here.
func newSingleDocServer(t *testing.T) *UnifiedServer {
	t.Helper()

	logger := newTestLogger()
	telemetry := &libOpentelemetry.Telemetry{}
	auth := &middleware.AuthClient{Enabled: false}

	v1Mount := func(group fiber.Router, api huma.API) {
		httpin.RegisterOrganizationRoutesToApp(group, api, auth, &httpin.OrganizationHandler{}, nil)
	}
	v2Mount := func(group fiber.Router, api huma.API) {
		httpin.RegisterTransactionV2RoutesToApp(group, api, auth, &httpin.TransactionHandler{}, nil)
	}

	server := NewUnifiedServer(":0", "test-version", logger, telemetry, nil, v1Mount, v2Mount)
	require.NotNil(t, server, "NewUnifiedServer should return a non-nil server")
	require.NotNil(t, server.app, "server should hold a Fiber app")

	return server
}

// TestNewUnifiedServer_SingleDocumentTopology asserts the single-document topology:
// the unified server serves ONE OpenAPI spec at the ROOT (/openapi.json) whose paths
// object carries BOTH the /v1 and /v2 operations, and whose servers block is exactly
// [{"url":"/"}]. One document with two path prefixes, not two documents.
func TestNewUnifiedServer_SingleDocumentTopology(t *testing.T) {
	// ServeSpec is gated on OPENAPI_DOCS_ENABLED; enable it so the root spec route is
	// mounted. t.Setenv precludes t.Parallel here.
	t.Setenv("OPENAPI_DOCS_ENABLED", "true")

	server := newSingleDocServer(t)

	doc := fetchOpenAPISpec(t, server.app, "/openapi.json")

	assert.Equal(t, "3.1.0", doc["openapi"], "the single spec should be OAS 3.1")
	assert.Equal(t, []string{"/"}, serverURLs(t, doc),
		"the single document advertises the root server only, so op paths carry the version prefix")

	info, _ := doc["info"].(map[string]any)
	require.NotNil(t, info, "the single spec should carry an info object")
	assert.Equal(t, "Midaz Ledger API", info["title"], "the single document keeps the v1 title")

	paths, ok := doc["paths"].(map[string]any)
	require.True(t, ok, "the single spec should carry a paths object")

	_, hasV1 := paths["/v1/organizations"]
	assert.Truef(t, hasV1, "the single document must carry the v1 op path /v1/organizations; paths=%v", keysOf(paths))

	_, hasV2 := paths["/v2/transactions/direct"]
	assert.Truef(t, hasV2, "the single document must carry the v2 op path /v2/transactions/direct; paths=%v", keysOf(paths))
}

// TestNewUnifiedServer_PerPrefixSpecRoutesNotServed asserts the per-prefix spec routes
// are gone: with docs enabled, GET /v1/openapi.json and GET /v2/openapi.json are NOT
// served (404). The one document lives at the root, not under either version prefix.
func TestNewUnifiedServer_PerPrefixSpecRoutesNotServed(t *testing.T) {
	// t.Setenv precludes t.Parallel here.
	t.Setenv("OPENAPI_DOCS_ENABLED", "true")

	server := newSingleDocServer(t)

	for _, path := range []string{"/v1/openapi.json", "/v2/openapi.json"} {
		t.Run(path, func(t *testing.T) {
			req, err := http.NewRequest(http.MethodGet, path, nil)
			require.NoError(t, err)

			resp, err := server.app.Test(req)
			require.NoError(t, err)

			defer func() { _ = resp.Body.Close() }()

			assert.Equalf(t, http.StatusNotFound, resp.StatusCode,
				"%s must not be served: the single document lives at the root", path)
		})
	}
}
