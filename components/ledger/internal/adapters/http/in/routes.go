// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"github.com/LerianStudio/lib-auth/v2/auth/middleware"
	libLog "github.com/LerianStudio/lib-commons/v4/commons/log"
	libHTTP "github.com/LerianStudio/lib-commons/v4/commons/net/http"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v4/commons/opentelemetry"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/LerianStudio/midaz/v3/pkg/net/http"
	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
)

const midazName = "midaz"

// NewRouter registers routes for the ledger component HTTP server.
func NewRouter(lg libLog.Logger, tl *libOpentelemetry.Telemetry, auth *middleware.AuthClient, mdi *MetadataIndexHandler) *fiber.App {
	f := fiber.New(fiber.Config{
		DisableStartupMessage: true,
		ErrorHandler: func(ctx *fiber.Ctx, err error) error {
			return libHTTP.FiberErrorHandler(ctx, err)
		},
	})

	tlMid := libHTTP.NewTelemetryMiddleware(tl)

	f.Use(tlMid.WithTelemetry(tl))
	f.Use(cors.New())
	f.Use(libHTTP.WithHTTPLogging(libHTTP.WithCustomLogger(lg)))
	f.Use(http.BridgeLibAuthHTTPContext())

	// Register metadata index routes
	RegisterRoutesToApp(f, auth, mdi, nil)

	// Health
	f.Get("/health", libHTTP.Ping)

	// Version
	f.Get("/version", libHTTP.Version)

	f.Use(tlMid.EndTracingSpans)

	return f
}

// RegisterRoutesToApp registers ledger routes (metadata indexes) to an existing Fiber app.
// This is used by the unified ledger server to consolidate all routes in a single port.
func RegisterRoutesToApp(f fiber.Router, auth *middleware.AuthClient, mdi *MetadataIndexHandler, routeOptions *http.ProtectedRouteOptions) {
	// Metadata Indexes
	f.Post("/v1/settings/metadata-indexes/entities/:entity_name",
		http.ProtectedRouteChain(
			auth.Authorize(midazName, "settings", "post"),
			routeOptions,
			http.WithBody(new(mmodel.CreateMetadataIndexInput), mdi.CreateMetadataIndex),
		)...)

	f.Get("/v1/settings/metadata-indexes",
		http.ProtectedRouteChain(
			auth.Authorize(midazName, "settings", "get"),
			routeOptions,
			mdi.GetAllMetadataIndexes,
		)...)

	f.Delete("/v1/settings/metadata-indexes/entities/:entity_name/key/:index_key",
		http.ProtectedRouteChain(
			auth.Authorize(midazName, "settings", "delete"),
			routeOptions,
			mdi.DeleteMetadataIndex,
		)...)
}

// CreateRouteRegistrar returns a function that registers ledger routes to an existing Fiber app.
// This is used by the unified ledger server to consolidate all routes in a single port.
func CreateRouteRegistrar(auth *middleware.AuthClient, mdi *MetadataIndexHandler, routeOptions *http.ProtectedRouteOptions) func(fiber.Router) {
	return func(fiberRouter fiber.Router) {
		RegisterRoutesToApp(fiberRouter, auth, mdi, routeOptions)
	}
}
