// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	nethttp "net/http"
	"strings"

	"github.com/gofiber/fiber/v3"
	"github.com/gofiber/fiber/v3/middleware/adaptor"

	"github.com/LerianStudio/midaz/v4/components/tracer/internal/adapters/http/in/middleware"
	pkgStreaming "github.com/LerianStudio/midaz/v4/pkg/streaming"
)

// streamingManifestGroupPrefix is the /v1 group prefix carried by
// pkgStreaming.ManifestRoutePath ("/v1/streaming/manifest"). The manifest route
// mounts INSIDE the api := f.Group("/v1") group, so the group-relative path is
// the const with this prefix trimmed — the const stays the single source of
// truth for the externally-advertised path while the route lands under /v1.
const streamingManifestGroupPrefix = "/v1"

// RegisterStreamingManifestRoute mounts the catalog-only lib-streaming manifest
// endpoint (GET pkgStreaming.ManifestRoutePath) INSIDE the tracer's protected
// /v1 group, guarded by the AuthGuard authz tuple ("streaming-manifest",
// "get"). handler is the stdlib net/http manifest handler produced by
// lib-streaming (catalog-only), adapted onto Fiber via adaptor.HTTPHandler; it
// enforces its own GET/HEAD method allowlist and hardening headers. The route
// carries no Huma OAS contract, so the contract-spec gate carves this path out.
//
// api is the /v1 group router, so the route is registered at the group-relative
// path and never lands in the public probe block (/health, /readyz, /metrics,
// /version) that precedes the group.
//
// A nil handler means the composition root could not build the manifest and
// degraded to skipping it: the route is then NOT mounted, so the path returns
// 404 rather than a broken terminal. A nil guard is likewise a no-op — the route
// must never be mounted without its auth chain.
func RegisterStreamingManifestRoute(api fiber.Router, guard *middleware.AuthGuard, handler nethttp.Handler) {
	if handler == nil || guard == nil {
		return
	}

	relPath := strings.TrimPrefix(pkgStreaming.ManifestRoutePath, streamingManifestGroupPrefix)

	api.Get(relPath, guard.With("streaming-manifest", "get", false), adaptor.HTTPHandler(handler))
}
