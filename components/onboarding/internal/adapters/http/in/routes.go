// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"github.com/LerianStudio/lib-auth/v2/auth/middleware"
	libLog "github.com/LerianStudio/lib-commons/v4/commons/log"
	libHTTP "github.com/LerianStudio/lib-commons/v4/commons/net/http"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v4/commons/opentelemetry"
	httpin "github.com/LerianStudio/midaz/v3/components/ledger/adapters/http/in"
	"github.com/LerianStudio/midaz/v3/pkg/net/http"
	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
)

// NewRouter register NewRouter routes to the Server.
func NewRouter(lg libLog.Logger, tl *libOpentelemetry.Telemetry, auth *middleware.AuthClient, ah *httpin.AccountHandler, ph *httpin.PortfolioHandler, lh *httpin.LedgerHandler, ih *httpin.AssetHandler, oh *httpin.OrganizationHandler, sh *httpin.SegmentHandler, ath *httpin.AccountTypeHandler) *fiber.App {
	// standalone mode - create Fiber app with standard error handling
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
	httpin.RegisterOnboardingRoutesToApp(f, auth, ah, ph, lh, ih, oh, sh, ath, nil)

	f.Get("/health", libHTTP.Ping)
	f.Get("/version", libHTTP.Version)

	f.Use(tlMid.EndTracingSpans)

	return f
}

// RegisterRoutesToApp registers onboarding routes to an existing Fiber app.
// This delegates to the unified route registration in the ledger package.
func RegisterRoutesToApp(f fiber.Router, auth *middleware.AuthClient, ah *httpin.AccountHandler, ph *httpin.PortfolioHandler, lh *httpin.LedgerHandler, ih *httpin.AssetHandler, oh *httpin.OrganizationHandler, sh *httpin.SegmentHandler, ath *httpin.AccountTypeHandler, routeOptions *http.ProtectedRouteOptions) {
	httpin.RegisterOnboardingRoutesToApp(f, auth, ah, ph, lh, ih, oh, sh, ath, routeOptions)
}
