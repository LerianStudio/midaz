// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"context"
	"encoding/json"
	"fmt"
	"html"
	"strings"

	libLog "github.com/LerianStudio/lib-observability/v2/log"
	"github.com/danielgtaylor/huma/v2"
	"github.com/gofiber/fiber/v3"
)

// filterSpecByVersion returns a copy of the marshaled OpenAPI document containing
// only the paths under prefix ("/v1" or "/v2"), cleaned for a single-version docs
// view: the document-level x-tagGroups and root tags are removed and the
// " (v1)"/" (v2)" suffix is stripped from each operation's tags, while
// per-operation deprecated flags are preserved. specJSON is not mutated.
func filterSpecByVersion(specJSON []byte, prefix string) ([]byte, error) {
	var doc map[string]any
	if err := json.Unmarshal(specJSON, &doc); err != nil {
		return nil, fmt.Errorf("unmarshal openapi spec: %w", err)
	}

	suffix := " (" + strings.TrimPrefix(prefix, "/") + ")"

	if paths, ok := doc["paths"].(map[string]any); ok {
		for key := range paths {
			if key != prefix && !strings.HasPrefix(key, prefix+"/") {
				delete(paths, key)

				continue
			}

			if item, ok := paths[key].(map[string]any); ok {
				stripVersionSuffixFromPathItem(item, suffix)
			}
		}
	}

	// The version selector makes the group extension and the root tag list
	// redundant in a single-version view; Scalar rebuilds the tag list from
	// the operations.
	delete(doc, "x-tagGroups")
	delete(doc, "tags")

	return json.Marshal(doc)
}

// stripVersionSuffixFromPathItem trims suffix from every operation tag within a
// single OpenAPI path-item map, leaving non-operation members (parameters, etc.)
// untouched.
func stripVersionSuffixFromPathItem(item map[string]any, suffix string) {
	for _, raw := range item {
		op, ok := raw.(map[string]any)
		if !ok {
			continue
		}

		tags, ok := op["tags"].([]any)
		if !ok {
			continue
		}

		for i, t := range tags {
			if s, ok := t.(string); ok {
				tags[i] = strings.TrimSuffix(s, suffix)
			}
		}
	}
}

// scalarDocsCSP relaxes the strict global CSP for the Scalar docs page so the
// Scalar bundle and its assets load from the jsdelivr CDN. It is copied verbatim
// from the unexported scalarCSP const in lib-commons
// (commons/net/http/openapi/openapi.go) so the docs CSP stays byte-identical to
// the policy the shared ServeSpec helper applies today; the lib-commons upstream
// follow-up removes this local copy.
const scalarDocsCSP = "default-src 'self'; script-src 'self' 'unsafe-inline' https://cdn.jsdelivr.net; style-src 'self' 'unsafe-inline' https://cdn.jsdelivr.net; img-src 'self' data: https://cdn.jsdelivr.net; font-src 'self' data: https://cdn.jsdelivr.net; connect-src 'self'; frame-ancestors 'none'"

// docsHTMLTemplate is a minimal, dependency-free docs page that renders the specs
// via Scalar loaded from a CDN <script>. %[1]s = HTML-escaped title, %[2]s = the
// Scalar configuration JSON (a trusted package constant). Scalar reads its config
// from the data-configuration attribute on the #api-reference script tag.
const docsHTMLTemplate = `<!doctype html>
<html>
  <head>
    <title>%[1]s</title>
    <meta charset="utf-8" />
    <meta name="viewport" content="width=device-width, initial-scale=1" />
  </head>
  <body>
    <script id="api-reference" data-configuration='%[2]s'></script>
    <script src="https://cdn.jsdelivr.net/npm/@scalar/api-reference"></script>
  </body>
</html>`

// scalarConfig declares the version selector: Scalar renders one dropdown entry
// per element of the sources array, and the entry marked default:true is shown
// first. V2 is the default source; V1 (deprecated) is available on demand. The
// sources/url/title/default/layout field names match the multi-document
// configuration of the @scalar/api-reference bundle the docs page loads, which is
// unpinned/latest (the CDN URL carries no version tag, mirroring lib-commons).
const scalarConfig = `{"sources":[` +
	`{"url":"/openapi.v2.json","title":"V2","default":true},` +
	`{"url":"/openapi.v1.json","title":"V1 (deprecated)"}` +
	`],"layout":"modern"}`

// registerVersionedDocRoutes registers, at the root, GET /openapi.json|.yaml
// (the unified document, unchanged), GET /openapi.v2.json and /openapi.v1.json (the
// per-version view specs), and GET /docs (the Scalar version-selector page). The
// served source URLs in scalarConfig are root-absolute, so the routes are mounted at
// the root to match. The docs route carries the relaxed CSP and nosniff header the
// shared helper applied.
func registerVersionedDocRoutes(app *fiber.App, unifiedJSON, unifiedYAML, v1JSON, v2JSON []byte, title string) {
	group := app.Group("/")

	serveBytes := func(contentType string, body []byte) fiber.Handler {
		return func(c fiber.Ctx) error {
			c.Set(fiber.HeaderContentType, contentType)

			return c.Send(body)
		}
	}

	group.Get("/openapi.json", serveBytes("application/json", unifiedJSON))
	group.Get("/openapi.yaml", serveBytes("application/yaml; charset=utf-8", unifiedYAML))
	group.Get("/openapi.v2.json", serveBytes("application/json", v2JSON))
	group.Get("/openapi.v1.json", serveBytes("application/json", v1JSON))

	page := fmt.Appendf(nil, docsHTMLTemplate, html.EscapeString(title), scalarConfig)

	group.Get("/docs", func(c fiber.Ctx) error {
		c.Set("Content-Security-Policy", scalarDocsCSP)
		c.Set("X-Content-Type-Options", "nosniff")
		c.Set(fiber.HeaderContentType, "text/html; charset=utf-8")

		return c.Send(page)
	})
}

// serveVersionedDocs snapshots the assembled unified spec (JSON + YAML), derives
// the v1-only and v2-only view specs via filterSpecByVersion, and registers the
// versioned docs routes at the root. The Huma spec is immutable after operation
// registration, so the bytes are snapshotted once here rather than per request.
// On any marshal/derive failure the routes are skipped and the failure is logged;
// it never panics.
func serveVersionedDocs(app *fiber.App, api huma.API, logger libLog.Logger, title string) {
	if app == nil || api == nil {
		return
	}

	if logger == nil {
		logger = libLog.NewNop()
	}

	ctx := context.Background()

	unifiedYAML, err := api.OpenAPI().YAML()
	if err != nil {
		logger.Log(ctx, libLog.LevelError, "failed to render unified openapi yaml", libLog.Err(err))

		return
	}

	unifiedJSON, err := json.Marshal(api.OpenAPI())
	if err != nil {
		logger.Log(ctx, libLog.LevelError, "failed to marshal unified openapi json", libLog.Err(err))

		return
	}

	v2JSON, err := filterSpecByVersion(unifiedJSON, "/v2")
	if err != nil {
		logger.Log(ctx, libLog.LevelError, "failed to derive v2 view spec", libLog.Err(err))

		return
	}

	v1JSON, err := filterSpecByVersion(unifiedJSON, "/v1")
	if err != nil {
		logger.Log(ctx, libLog.LevelError, "failed to derive v1 view spec", libLog.Err(err))

		return
	}

	registerVersionedDocRoutes(app, unifiedJSON, unifiedYAML, v1JSON, v2JSON, title)
}
