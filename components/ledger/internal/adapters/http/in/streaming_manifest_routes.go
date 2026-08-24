// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	nethttp "net/http"

	"github.com/LerianStudio/lib-auth/v3/auth/middleware"
	"github.com/gofiber/fiber/v3"
	"github.com/gofiber/fiber/v3/middleware/adaptor"

	"github.com/LerianStudio/midaz/v4/pkg/net/http"
	pkgStreaming "github.com/LerianStudio/midaz/v4/pkg/streaming"
)

// RegisterStreamingManifestRouteToApp mounts the lib-streaming manifest endpoint
// (GET pkgStreaming.ManifestRoutePath) on the app root, guarded by the midaz
// authz tuple ("streaming-manifest", "get"). handler is the stdlib net/http
// manifest handler produced by lib-streaming (catalog-only), adapted onto Fiber
// via adaptor.HTTPHandler; it enforces its own GET/HEAD method allowlist and
// hardening headers. The route carries no Huma OAS contract, so the contract-spec
// gate carves this path out.
//
// A nil handler means the composition root could not build the manifest and
// degraded to skipping it: the route is then NOT mounted, so the path returns
// 404 rather than a broken terminal.
func RegisterStreamingManifestRouteToApp(app fiber.Router, auth *middleware.AuthClient, routeOptions *http.ProtectedRouteOptions, handler nethttp.Handler) {
	if handler == nil {
		return
	}

	routeGet(app, pkgStreaming.ManifestRoutePath,
		protectedMidaz(auth, "streaming-manifest", "get", routeOptions, adaptor.HTTPHandler(handler)))
}
