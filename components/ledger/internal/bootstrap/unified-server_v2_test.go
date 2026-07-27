// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"encoding/json"
	"io"
	"net/http"
	"testing"

	libOpentelemetry "github.com/LerianStudio/lib-observability/v2/tracing"
	"github.com/danielgtaylor/huma/v2"
	"github.com/gofiber/fiber/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fetchOpenAPISpec drives the in-process Fiber app for the given spec path and
// decodes the JSON body into a generic document. It fails the test if the route
// is not served with 200 application/json.
func fetchOpenAPISpec(t *testing.T, app *fiber.App, specPath string) map[string]any {
	t.Helper()

	req, err := http.NewRequest(http.MethodGet, specPath, nil)
	require.NoError(t, err)

	resp, err := app.Test(req)
	require.NoError(t, err)

	defer func() {
		_ = resp.Body.Close()
	}()

	require.Equal(t, http.StatusOK, resp.StatusCode, "spec %q should be served", specPath)

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	var doc map[string]any
	require.NoError(t, json.Unmarshal(body, &doc), "spec %q should decode as JSON", specPath)

	return doc
}

// serverURLs extracts the advertised server URLs from a decoded OpenAPI doc.
func serverURLs(t *testing.T, doc map[string]any) []string {
	t.Helper()

	rawServers, ok := doc["servers"].([]any)
	require.True(t, ok, "spec should carry a servers array")

	urls := make([]string, 0, len(rawServers))

	for _, raw := range rawServers {
		server, ok := raw.(map[string]any)
		require.True(t, ok, "each server entry should be an object")

		url, ok := server["url"].(string)
		require.True(t, ok, "each server entry should carry a url string")

		urls = append(urls, url)
	}

	return urls
}

// TestNewUnifiedServer_V2ContractMountedIndependently asserts ADR-003: the
// unified server exposes a SECOND, independent OpenAPI contract instance under
// /v2 (OAS 3.1, servers ["/v2"]) alongside the untouched /v1 contract (servers
// ["/v1"]). Each instance owns its Info metadata, proving two independent Huma
// component registries rather than one shared document.
func TestNewUnifiedServer_V2ContractMountedIndependently(t *testing.T) {
	// ServeSpec is gated on LEDGER_HUMA_DOCS_ENABLED; enable it so both the /v1
	// and /v2 spec routes are mounted. t.Setenv precludes t.Parallel here.
	t.Setenv("LEDGER_HUMA_DOCS_ENABLED", "true")

	logger := newTestLogger()
	telemetry := &libOpentelemetry.Telemetry{}

	// Empty mounts: this task mounts the contract surfaces without registering
	// ops (v2 ops arrive in Task 1.1.2). A non-nil mount is what triggers each
	// version's Huma bootstrap block.
	emptyMount := func(_ fiber.Router, _ huma.API) {}

	server := NewUnifiedServer(":0", "test-version", logger, telemetry, nil, emptyMount, emptyMount)
	require.NotNil(t, server, "NewUnifiedServer should return a non-nil server")
	require.NotNil(t, server.app, "server should hold a Fiber app")

	// v2 contract: served, OAS 3.1, advertises /v2 only.
	v2doc := fetchOpenAPISpec(t, server.app, "/v2/openapi.json")
	assert.Equal(t, "3.1.0", v2doc["openapi"], "v2 spec should be OAS 3.1")
	assert.Equal(t, []string{"/v2"}, serverURLs(t, v2doc), "v2 spec should advertise the /v2 server only")

	// v1 contract: unchanged, OAS 3.1, advertises /v1 only. v2 mounting must not
	// leak into the v1 document.
	v1doc := fetchOpenAPISpec(t, server.app, "/v1/openapi.json")
	assert.Equal(t, "3.1.0", v1doc["openapi"], "v1 spec should remain OAS 3.1")
	assert.Equal(t, []string{"/v1"}, serverURLs(t, v1doc), "v1 spec should still advertise the /v1 server only")

	// Independent Info objects prove two distinct contract instances (separate
	// Huma component registries), not a single shared document.
	v1info, _ := v1doc["info"].(map[string]any)
	v2info, _ := v2doc["info"].(map[string]any)
	require.NotNil(t, v1info, "v1 spec should carry an info object")
	require.NotNil(t, v2info, "v2 spec should carry an info object")
	assert.Equal(t, "Midaz Ledger API", v1info["title"], "v1 title unchanged")
	assert.Equal(t, "Midaz Ledger API v2", v2info["title"], "v2 carries its own title")
}
