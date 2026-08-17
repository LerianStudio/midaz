// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	stdhttp "net/http"

	"github.com/LerianStudio/lib-auth/v2/auth/middleware"
	pkgHTTP "github.com/LerianStudio/midaz/v3/pkg/net/http"
	pkgStreaming "github.com/LerianStudio/midaz/v3/pkg/streaming"
	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/adaptor"
)

// RegisterStreamingManifestRoute mounts the control-plane streaming manifest
// endpoint (GET /v1/streaming/manifest) behind the standard midaz protected
// chain, guarded by product "midaz", resource "streaming-manifest", action
// "get". The manifest is static component metadata, so it is NOT gated on
// STREAMING_ENABLED.
//
// manifestHandler is a stdlib net/http handler produced by
// pkgStreaming.NewManifestHTTPHandler and wrapped for Fiber here. A nil
// handler means the bootstrap build failed; the route is left unmounted so a
// degraded manifest build never breaks startup or serves a nil handler.
func RegisterStreamingManifestRoute(router fiber.Router, auth *middleware.AuthClient, manifestHandler stdhttp.Handler, routeOptions *pkgHTTP.ProtectedRouteOptions) {
	if manifestHandler == nil {
		return
	}

	router.Get(
		pkgStreaming.ManifestRoutePath,
		protectedMidaz(auth, "streaming-manifest", "get", routeOptions, adaptor.HTTPHandler(manifestHandler))...,
	)
}
