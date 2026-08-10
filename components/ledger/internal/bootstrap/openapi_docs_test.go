// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	openapi "github.com/LerianStudio/lib-commons/v6/commons/net/http/openapi"
	"github.com/danielgtaylor/huma/v2"
	"github.com/gofiber/fiber/v3"
	"github.com/stretchr/testify/require"
)

// fixtureUnifiedSpec is the seed unified document the filter and route tests derive
// their per-version views from. Alongside the versioned happy-path pair (/v1/foo
// deprecated, /v2/foo) it carries three boundary fixtures:
//   - "/health": unversioned, so it must fall out of BOTH views.
//   - "/v2beta/foo": a sibling prefix that shares the "/v2" text but not the "/v2/"
//     boundary, so it must fall out of the /v2 view (guards the prefix+"/" check).
//   - "/v2/notags": an operation with no "tags" key, so the suffix strip must leave
//     it untouched and never panic.
func fixtureUnifiedSpec(t *testing.T) []byte {
	t.Helper()

	return []byte(`{
      "openapi":"3.1.0",
      "x-tagGroups":[{"name":"V1 (deprecated)","tags":["Foo (v1)"]},{"name":"V2","tags":["Foo (v2)"]}],
      "tags":[{"name":"Foo (v1)"},{"name":"Foo (v2)"}],
      "paths":{
        "/v1/foo":{"post":{"operationId":"createFoo","deprecated":true,"tags":["Foo (v1)"]},"parameters":[]},
        "/v2/foo":{"post":{"operationId":"createFooV2","tags":["Foo (v2)"]}},
        "/v2/notags":{"post":{"operationId":"noTagsV2"}},
        "/v2beta/foo":{"post":{"operationId":"betaFoo","tags":["Beta (v2)"]}},
        "/health":{"get":{"operationId":"health"}}
      }
    }`)
}

// TestFilterSpecByVersion folds the per-prefix filter behaviour into one table: each
// case pins the kept path, the paths that must drop out, the stripped operation tag,
// and — for the v1 view — that the per-operation deprecated flag survives. The
// boundary fixtures (/health, /v2beta/foo, /v2/notags) ride along per case.
func TestFilterSpecByVersion(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		prefix         string
		keptPath       string
		wantTags       []any
		droppedPaths   []string
		deprecatedKept bool
		// noTagsPath is a path whose operation carries no "tags" key; it must survive
		// the filter untouched. Empty means the case does not check it.
		noTagsPath string
	}{
		{
			name:         "v2 view",
			prefix:       "/v2",
			keptPath:     "/v2/foo",
			wantTags:     []any{"Foo"},
			droppedPaths: []string{"/v1/foo", "/health", "/v2beta/foo"},
			noTagsPath:   "/v2/notags",
		},
		{
			name:           "v1 view preserves deprecated",
			prefix:         "/v1",
			keptPath:       "/v1/foo",
			wantTags:       []any{"Foo"},
			droppedPaths:   []string{"/v2/foo", "/health", "/v2beta/foo", "/v2/notags"},
			deprecatedKept: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			src := fixtureUnifiedSpec(t)
			out, err := filterSpecByVersion(src, tt.prefix)
			require.NoError(t, err)
			require.JSONEq(t, string(fixtureUnifiedSpec(t)), string(src), "input must not be mutated")

			var doc map[string]any
			require.NoError(t, json.Unmarshal(out, &doc))

			require.NotContains(t, doc, "x-tagGroups", "single-version view drops the group extension")
			require.NotContains(t, doc, "tags", "single-version view drops the root tag list")

			paths := doc["paths"].(map[string]any)
			require.Contains(t, paths, tt.keptPath)

			for _, dropped := range tt.droppedPaths {
				require.NotContainsf(t, paths, dropped, "%s must not survive the %s view", dropped, tt.prefix)
			}

			op := paths[tt.keptPath].(map[string]any)["post"].(map[string]any)
			require.Equal(t, tt.wantTags, op["tags"], "version suffix must be stripped")

			if tt.deprecatedKept {
				require.Equal(t, true, op["deprecated"], "deprecated flag must be preserved on the v1 view")
			}

			if tt.noTagsPath != "" {
				require.Contains(t, paths, tt.noTagsPath, "a tagless operation must survive the filter")
				noTagsOp := paths[tt.noTagsPath].(map[string]any)["post"].(map[string]any)
				require.NotContains(t, noTagsOp, "tags", "a tagless operation must stay tagless")
			}
		})
	}
}

// scalarSource / scalarConfigDoc mirror the fields the docs test reads back out of the
// Scalar.createApiReference config argument, so a regression that moved default:true onto
// V1 (or changed a source URL) fails the assertion.
type scalarSource struct {
	URL     string `json:"url"`
	Title   string `json:"title"`
	Default bool   `json:"default"`
}

type scalarConfigDoc struct {
	Sources []scalarSource `json:"sources"`
	Layout  string         `json:"layout"`
}

// parseScalarConfig extracts and unmarshals the JSON config passed to
// Scalar.createApiReference in the docs page. A json.Decoder reads exactly the first
// JSON value (the config object) and ignores the trailing `)` and markup — robust to a
// ')' appearing inside a string value such as the "V1 (deprecated)" title.
func parseScalarConfig(t *testing.T, page string) scalarConfigDoc {
	t.Helper()

	const marker = `Scalar.createApiReference('#app', `

	i := strings.Index(page, marker)
	require.GreaterOrEqual(t, i, 0, "docs page must call Scalar.createApiReference with the config")

	var cfg scalarConfigDoc
	dec := json.NewDecoder(strings.NewReader(page[i+len(marker):]))
	require.NoError(t, dec.Decode(&cfg))

	return cfg
}

// defaultSource returns the single source flagged default:true.
func defaultSource(t *testing.T, cfg scalarConfigDoc) scalarSource {
	t.Helper()

	var found []scalarSource

	for _, s := range cfg.Sources {
		if s.Default {
			found = append(found, s)
		}
	}

	require.Len(t, found, 1, "exactly one source must be the default")

	return found[0]
}

func TestRegisterVersionedDocRoutes(t *testing.T) {
	t.Parallel()

	app := fiber.New()
	unified := fixtureUnifiedSpec(t)
	v2, err := filterSpecByVersion(unified, "/v2")
	require.NoError(t, err)
	v1, err := filterSpecByVersion(unified, "/v1")
	require.NoError(t, err)

	registerVersionedDocRoutes(app, unified, []byte("openapi: 3.1.0\n"), v1, v2, "Midaz Ledger")

	// unified document served at the root, carrying BOTH version families.
	resp, err := app.Test(httptest.NewRequest(http.MethodGet, "/openapi.json", nil))
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	unifiedBody, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.Contains(t, string(unifiedBody), "/v1/foo")
	require.Contains(t, string(unifiedBody), "/v2/foo")

	// v2 view served, contains only /v2 (and not the /v2beta sibling).
	resp, err = app.Test(httptest.NewRequest(http.MethodGet, "/openapi.v2.json", nil))
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	v2Body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.Contains(t, string(v2Body), "/v2/foo")
	require.NotContains(t, string(v2Body), "/v1/foo")
	require.NotContains(t, string(v2Body), "/v2beta/foo")

	// v1 view served, contains only /v1.
	resp, err = app.Test(httptest.NewRequest(http.MethodGet, "/openapi.v1.json", nil))
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	v1Body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.Contains(t, string(v1Body), "/v1/foo")
	require.NotContains(t, string(v1Body), "/v2/foo")

	// per-prefix spec paths are NOT registered here (both versions 404).
	for _, path := range []string{"/v1/openapi.json", "/v2/openapi.json"} {
		resp, err = app.Test(httptest.NewRequest(http.MethodGet, path, nil))
		require.NoError(t, err)
		require.Equalf(t, http.StatusNotFound, resp.StatusCode, "%s must not be served at the root", path)
	}

	// docs page: security headers pinned, version selector config with V2 as default.
	resp, err = app.Test(httptest.NewRequest(http.MethodGet, "/docs", nil))
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.Equal(t, scalarDocsCSP, resp.Header.Get("Content-Security-Policy"), "docs CSP must be pinned byte-for-byte")
	require.Equal(t, "nosniff", resp.Header.Get("X-Content-Type-Options"))

	page, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	cfg := parseScalarConfig(t, string(page))
	require.Equal(t, "/openapi.v2.json", defaultSource(t, cfg).URL, "V2 must be the default docs source")

	urls := make([]string, 0, len(cfg.Sources))
	for _, s := range cfg.Sources {
		urls = append(urls, s.URL)
	}

	require.Contains(t, urls, "/openapi.v2.json")
	require.Contains(t, urls, "/openapi.v1.json")
}

// newVersionedDocsAPI builds a minimal single-document Huma contract over a Fiber app
// the way the production seam does: one /v1 GET op and one /v2 GET op, each hung off a
// version-prefixed huma.NewGroup so the operation paths carry the version prefix. A
// zero-value handler is safe — registration stores the handler func and never calls it.
func newVersionedDocsAPI(t *testing.T) (*fiber.App, huma.API) {
	t.Helper()

	app := fiber.New()
	api := openapi.New(app, app, openapi.Config{
		Title:   "Midaz Ledger API",
		Version: "test",
		Servers: []string{"/"},
	})

	type fooOutput struct {
		Body struct {
			OK bool `json:"ok"`
		}
	}

	noop := func(_ context.Context, _ *struct{}) (*fooOutput, error) { return &fooOutput{}, nil }

	huma.Register(huma.NewGroup(api, "/v1"), huma.Operation{
		OperationID: "getFooV1",
		Method:      http.MethodGet,
		Path:        "/foo",
		Tags:        []string{"Foo"},
	}, noop)

	huma.Register(huma.NewGroup(api, "/v2"), huma.Operation{
		OperationID: "getFooV2",
		Method:      http.MethodGet,
		Path:        "/foo",
		Tags:        []string{"Foo"},
	}, noop)

	return app, api
}

// TestServeVersionedDocs exercises serveVersionedDocs — the production entry point —
// end to end over a real huma.API. It asserts the happy path (unified spec + both
// per-version views + docs page all served) and the nil-guard no-op.
//
// The marshal/derive degrade branches (YAML/JSON render or filterSpecByVersion
// failure) are impractical to trigger through a real huma.API: OpenAPI().YAML()/
// json.Marshal do not fail on a well-formed document and the assembled spec always
// unmarshals. They are left uncovered here rather than mocked; the nil-guard and the
// happy path are the reachable coverage of this entry point.
func TestServeVersionedDocs(t *testing.T) {
	t.Parallel()

	app, api := newVersionedDocsAPI(t)

	serveVersionedDocs(app, api, newTestLogger(), "Midaz Ledger")

	// unified spec: 200, both version families present.
	resp, err := app.Test(httptest.NewRequest(http.MethodGet, "/openapi.json", nil))
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	unified, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.Contains(t, string(unified), "/v1/foo")
	require.Contains(t, string(unified), "/v2/foo")

	// v2 view: 200, only /v2.
	resp, err = app.Test(httptest.NewRequest(http.MethodGet, "/openapi.v2.json", nil))
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	v2Body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.Contains(t, string(v2Body), "/v2/foo")
	require.NotContains(t, string(v2Body), "/v1/foo")

	// v1 view: 200, only /v1.
	resp, err = app.Test(httptest.NewRequest(http.MethodGet, "/openapi.v1.json", nil))
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	v1Body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.Contains(t, string(v1Body), "/v1/foo")
	require.NotContains(t, string(v1Body), "/v2/foo")

	// docs page served.
	resp, err = app.Test(httptest.NewRequest(http.MethodGet, "/docs", nil))
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode)

	// nil-guard: no routes, no panic.
	require.NotPanics(t, func() {
		serveVersionedDocs(nil, nil, newTestLogger(), "Midaz Ledger")
	})
}
