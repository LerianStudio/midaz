// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/LerianStudio/midaz/v3/pkg/net/http"

	"github.com/LerianStudio/lib-auth/v2/auth/middleware"
	libHTTP "github.com/LerianStudio/lib-commons/v5/commons/net/http"
	libLog "github.com/LerianStudio/lib-observability/log"
	libObsMiddleware "github.com/LerianStudio/lib-observability/middleware"
	libOpenTelemetry "github.com/LerianStudio/lib-observability/tracing"
	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
	fiberSwagger "github.com/swaggo/fiber-swagger"
)

const ApplicationName = "plugin-crm"

// ReadyzHandler is the interface for the readyz endpoint handler.
// This interface is defined here to avoid circular imports with the bootstrap package.
type ReadyzHandler interface {
	HandleReadyz(c *fiber.Ctx) error
}

func NewRouter(lg libLog.Logger, tl *libOpenTelemetry.Telemetry, auth *middleware.AuthClient, tenantMw fiber.Handler, readyzHandler ReadyzHandler, hh *HolderHandler, ah *InstrumentHandler, hah *HolderAccountsHandler) *fiber.App {
	f := fiber.New(fiber.Config{
		DisableStartupMessage: true,
		ErrorHandler: func(ctx *fiber.Ctx, err error) error {
			return libHTTP.FiberErrorHandler(ctx, err)
		},
	})
	tlMid := libObsMiddleware.NewTelemetryMiddleware(tl)

	f.Use(http.WithRecover(http.WithRecoverLogger(lg)))
	f.Use(tlMid.WithTelemetry(tl))
	f.Use(cors.New())
	f.Use(libObsMiddleware.WithHTTPLogging(libObsMiddleware.WithCustomLogger(lg)))
	// Public endpoints: registered BEFORE tenant middleware so they remain
	// accessible to Kubernetes probes, load balancer health checks, and
	// Swagger documentation without requiring a JWT or tenant context.
	f.Get("/health", libHTTP.Ping)
	f.Get("/version", libHTTP.Version)
	f.Get("/swagger", func(c *fiber.Ctx) error {
		return c.Redirect("/swagger/index.html", fiber.StatusMovedPermanently)
	})
	f.Get("/swagger/*", WithSwaggerEnvConfig(), fiberSwagger.FiberWrapHandler(
		fiberSwagger.InstanceName("crm"),
	))

	// Readyz endpoint: registered BEFORE auth/tenant middleware for K8s readiness probes.
	// K8s probes do not authenticate, so this endpoint MUST NOT require auth.
	if readyzHandler != nil {
		f.Get("/readyz", readyzHandler.HandleReadyz)
	}

	// Tenant middleware: registered only when multi-tenant mode is enabled.
	// When tenantMw is nil (single-tenant mode), this block is skipped entirely.
	if tenantMw != nil {
		f.Use(tenantMw)
	}

	// Standalone mode applies tenant middleware globally (above), so the routes
	// themselves carry no PostAuthMiddlewares.
	RegisterCRMRoutesToApp(f, auth, hh, ah, hah, nil)

	f.Use(tlMid.EndTracingSpans)

	return f
}

// RegisterCRMRoutesToApp registers the CRM holder/alias routes on an existing
// Fiber router. It is used both by the standalone NewRouter (above, with
// routeOptions=nil because tenant middleware is mounted globally there) and by
// the unified ledger server (which passes a CRM-scoped routeOptions carrying a
// route-local tenant middleware so CRM's tenant Mongo never overwrites the
// onboarding/transaction tenant DB injected for ledger routes).
//
// The routes, paths, authz namespace (plugin-crm via ApplicationName),
// UUID-path validation and body binding are identical in both callers.
//
// hah may be nil (standalone CRM router has no ledger account-query backing);
// when nil the holder-accounts route is not mounted.
func RegisterCRMRoutesToApp(f fiber.Router, auth *middleware.AuthClient, hh *HolderHandler, ah *InstrumentHandler, hah *HolderAccountsHandler, routeOptions *http.ProtectedRouteOptions) {
	// Holders
	f.Post("/v1/holders", http.ProtectedRouteChain(auth.Authorize(ApplicationName, "holders", "post"), routeOptions, http.WithBody(new(mmodel.CreateHolderInput), hh.CreateHolder))...)
	f.Get("/v1/holders/:id", http.ProtectedRouteChain(auth.Authorize(ApplicationName, "holders", "get"), routeOptions, http.ParseUUIDPathParameters("holder"), hh.GetHolderByID)...)
	if hah != nil {
		f.Get("/v1/holders/:id/accounts", http.ProtectedRouteChain(auth.Authorize(ApplicationName, "holders", "get"), routeOptions, http.ParseUUIDPathParameters("holder"), hah.GetAccountsByHolder)...)
	}
	f.Patch("/v1/holders/:id", http.ProtectedRouteChain(auth.Authorize(ApplicationName, "holders", "patch"), routeOptions, http.ParseUUIDPathParameters("holder"), http.WithBody(new(mmodel.UpdateHolderInput), hh.UpdateHolder))...)
	f.Delete("/v1/holders/:id", http.ProtectedRouteChain(auth.Authorize(ApplicationName, "holders", "delete"), routeOptions, http.ParseUUIDPathParameters("holder"), hh.DeleteHolderByID)...)
	f.Get("/v1/holders", http.ProtectedRouteChain(auth.Authorize(ApplicationName, "holders", "get"), routeOptions, hh.GetAllHolders)...)

	// Aliases
	f.Get("/v1/aliases", http.ProtectedRouteChain(auth.Authorize(ApplicationName, "aliases", "get"), routeOptions, ah.GetAllInstruments)...)
	f.Post("/v1/holders/:holder_id/aliases", http.ProtectedRouteChain(auth.Authorize(ApplicationName, "aliases", "post"), routeOptions, http.ParseUUIDPathParameters("aliases"), http.WithBody(new(mmodel.CreateInstrumentInput), ah.CreateInstrument))...)
	f.Get("/v1/holders/:holder_id/aliases/:alias_id", http.ProtectedRouteChain(auth.Authorize(ApplicationName, "aliases", "get"), routeOptions, http.ParseUUIDPathParameters("aliases"), ah.GetInstrumentByID)...)
	f.Patch("/v1/holders/:holder_id/aliases/:alias_id", http.ProtectedRouteChain(auth.Authorize(ApplicationName, "aliases", "patch"), routeOptions, http.ParseUUIDPathParameters("aliases"), http.WithBody(new(mmodel.UpdateInstrumentInput), ah.UpdateInstrument))...)
	f.Delete("/v1/holders/:holder_id/aliases/:alias_id", http.ProtectedRouteChain(auth.Authorize(ApplicationName, "aliases", "delete"), routeOptions, http.ParseUUIDPathParameters("aliases"), ah.DeleteInstrumentByID)...)
	f.Delete("/v1/holders/:holder_id/aliases/:alias_id/related-parties/:related_party_id", http.ProtectedRouteChain(auth.Authorize(ApplicationName, "aliases", "delete"), routeOptions, http.ParseUUIDPathParameters("related-parties"), ah.DeleteRelatedParty)...)
}

// CreateCRMRouteRegistrar returns a registrar that mounts the CRM routes on the
// unified ledger server. The routeOptions carries the CRM-scoped tenant
// middleware (built in the ledger composition root) so it applies ONLY to CRM
// routes.
func CreateCRMRouteRegistrar(auth *middleware.AuthClient, hh *HolderHandler, ah *InstrumentHandler, hah *HolderAccountsHandler, routeOptions *http.ProtectedRouteOptions) func(fiber.Router) {
	return func(router fiber.Router) {
		RegisterCRMRoutesToApp(router, auth, hh, ah, hah, routeOptions)
	}
}
