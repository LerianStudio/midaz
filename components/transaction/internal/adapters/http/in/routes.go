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
func NewRouter(lg libLog.Logger, tl *libOpentelemetry.Telemetry, auth *middleware.AuthClient, th *httpin.TransactionHandler, oh *httpin.OperationHandler, ah *httpin.AssetRateHandler, bh *httpin.BalanceHandler, orh *httpin.OperationRouteHandler, trh *httpin.TransactionRouteHandler) *fiber.App {
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
	httpin.RegisterTransactionRoutesToApp(f, auth, th, oh, ah, bh, orh, trh, nil)

	f.Get("/health", libHTTP.Ping)
	f.Get("/version", libHTTP.Version)

	f.Use(tlMid.EndTracingSpans)

	return f
}

// RegisterRoutesToApp registers transaction routes to an existing Fiber app.
// This delegates to the unified route registration in the ledger package.
func RegisterRoutesToApp(f fiber.Router, auth *middleware.AuthClient, th *httpin.TransactionHandler, oh *httpin.OperationHandler, ah *httpin.AssetRateHandler, bh *httpin.BalanceHandler, orh *httpin.OperationRouteHandler, trh *httpin.TransactionRouteHandler, routeOptions *http.ProtectedRouteOptions) {
	httpin.RegisterTransactionRoutesToApp(f, auth, th, oh, ah, bh, orh, trh, routeOptions)
}
