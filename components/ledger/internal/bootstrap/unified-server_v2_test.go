// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

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

// TestNewUnifiedServer_SingleDocumentAtRootSpansBothVersionPrefixes asserts the
// consolidated topology that replaced the two-document layout: the unified server
// serves ONE OpenAPI document at the root (/openapi.json), OAS 3.1, whose servers
// block is exactly [{"url":"/"}] and whose info.title is "Midaz Ledger API". The
// version rides each operation's PATH, so the single paths object carries BOTH the
// /v1 and /v2 prefixes. The 404 negative is what makes "one document" falsifiable:
// without it, one document and two-documents-plus-a-root-document read identically.
func TestNewUnifiedServer_SingleDocumentAtRootSpansBothVersionPrefixes(t *testing.T) {
	// ServeSpec is gated on OPENAPI_DOCS_ENABLED; enable it so the root spec route is
	// mounted. t.Setenv precludes t.Parallel here.
	t.Setenv("OPENAPI_DOCS_ENABLED", "true")

	server := newSingleDocServer(t)

	doc := fetchOpenAPISpec(t, server.app, "/openapi.json")
	assert.Equal(t, "3.1.0", doc["openapi"], "the single spec should be OAS 3.1")
	assert.Equal(t, []string{"/"}, serverURLs(t, doc),
		"the single document advertises the root server only; the version rides the op path")

	info, _ := doc["info"].(map[string]any)
	require.NotNil(t, info, "the single spec should carry an info object")
	assert.Equal(t, "Midaz Ledger API", info["title"], "the single document carries one title")

	paths, ok := doc["paths"].(map[string]any)
	require.True(t, ok, "the single spec should carry a paths object")

	for _, want := range []string{"/v1/organizations", "/v2/transactions/direct"} {
		_, has := paths[want]
		assert.Truef(t, has, "the single document must carry the %q key; paths=%v", want, keysOf(paths))
	}

	// No per-version document exists: the prefixed spec routes 404. This inverts the
	// old "mounted independently" claim — the version is a path prefix, not a document.
	for _, path := range []string{"/v1/openapi.json", "/v2/openapi.json"} {
		req, err := http.NewRequest(http.MethodGet, path, nil)
		require.NoError(t, err)

		resp, err := server.app.Test(req)
		require.NoError(t, err)

		func() {
			defer func() { _ = resp.Body.Close() }()

			assert.Equalf(t, http.StatusNotFound, resp.StatusCode,
				"%s must 404: there is no per-version document, only the root one", path)
		}()
	}
}

// TestNewUnifiedServer_V2DirectOpDoesNotLeakIntoV1 asserts the version prefix lives
// IN the path key, not in a separate document: the single document carries the v2
// `direct` op under /v2/transactions/direct and NEVER under /v1/transactions/direct.
// The v1 prefix mounts a different op (organizations), so a v2 create is unreachable
// through a v1 key — path-key isolation, not registry isolation.
func TestNewUnifiedServer_V2DirectOpDoesNotLeakIntoV1(t *testing.T) {
	// ServeSpec is gated on OPENAPI_DOCS_ENABLED; enable it so the root spec route is
	// mounted. t.Setenv precludes t.Parallel here.
	t.Setenv("OPENAPI_DOCS_ENABLED", "true")

	server := newSingleDocServer(t)

	doc := fetchOpenAPISpec(t, server.app, "/openapi.json")
	paths, ok := doc["paths"].(map[string]any)
	require.True(t, ok, "the single spec should carry a paths object")

	_, hasV2 := paths[directOpV2Path]
	assert.Truef(t, hasV2, "the single document MUST carry the v2 direct op key %q; paths=%v", directOpV2Path, keysOf(paths))

	// Derive the v1 sibling from the v2 key so the two stay mirror images: a hardcoded
	// v1 literal would keep probing a never-registered key if directOpV2Path changed,
	// letting the negative assertion pass vacuously and miss a real cross-version leak.
	directOpV1Path := strings.Replace(directOpV2Path, "/v2/", "/v1/", 1)

	_, hasV1 := paths[directOpV1Path]
	assert.Falsef(t, hasV1, "the v2 direct op MUST NOT appear under the v1 key %q; paths=%v", directOpV1Path, keysOf(paths))
}
