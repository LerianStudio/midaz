// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"errors"

	tmcore "github.com/LerianStudio/lib-commons/v7/commons/tenant-manager/core"
	libObservability "github.com/LerianStudio/lib-observability/v4"
	libLog "github.com/LerianStudio/lib-observability/v4/log"
	libOpentelemetry "github.com/LerianStudio/lib-observability/v4/tracing"
	"github.com/gofiber/fiber/v3"

	"github.com/LerianStudio/midaz/v4/components/tracer/internal/adapters/seamtenant"
	"github.com/LerianStudio/midaz/v4/components/tracer/internal/services/workers"
	"github.com/LerianStudio/midaz/v4/pkg"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	pkgHTTP "github.com/LerianStudio/midaz/v4/pkg/net/http"
)

// reservationTenantMiddleware resolves the per-tenant PostgreSQL pool for the
// reservation routes from the TRUSTED X-Tenant-Id header the ledger forwards,
// and binds it into the request context BEFORE the reservation handler runs.
//
// The header is trusted because the reservation surface is reachable only over
// the mTLS/mesh-verified connection (the ledger is a verified service). This is
// a DEDICATED middleware applied ONLY to the reservation routes — the shared
// JWT-claim tenant middleware on the rest of the tracer's user routes is left
// untouched, and no header-trust path is opened on any route reachable without
// the verified peer.
//
// Under multi-tenant mode a missing/empty/invalid trusted tenant id fails clean
// (HTTP 422 via the typed business-error path) and NEVER resolves a
// default/wrong pool. In single-tenant (no-op) mode the resolver passes through.
//
// Once the tenant resolves, ensurer starts that tenant's workers — the same
// lazy spawn the rest of /v1 gets — so a tenant whose first traffic is a
// reservation still gets its rule cache loaded. A reached tenant cap answers 503
// with Retry-After; any other ensure failure is logged and the request proceeds.
// A nil ensurer skips the step.
func reservationTenantMiddleware(resolver *seamtenant.Resolver, ensurer WorkerEnsurer) fiber.Handler {
	return func(c fiber.Ctx) error {
		if !resolver.Active() {
			return c.Next()
		}

		ctx := c.Context()

		logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

		ctx, span := tracer.Start(ctx, "middleware.reservations.resolve_tenant")
		defer span.End()

		resolvedCtx, err := resolver.Resolve(ctx, c.Get(seamtenant.HeaderName))
		if err != nil {
			if errors.Is(err, constant.ErrReservationTenantRequired) {
				libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Missing trusted tenant id on reservation surface", err)

				return pkgHTTP.WithError(c, pkg.ValidateBusinessError(constant.ErrReservationTenantRequired, ""))
			}

			libOpentelemetry.HandleSpanError(span, "Failed to resolve tenant pool for reservation", err)

			// Technical resolution failure: WithError falls through to a generic
			// 500 (ValidateInternalError), so no internal detail leaks to the
			// client. The span already carries the underlying cause.
			return pkgHTTP.WithError(c, err)
		}

		c.SetContext(resolvedCtx)

		tenantID := tmcore.GetTenantIDContext(resolvedCtx)
		if ensurer == nil || tenantID == "" {
			return c.Next()
		}

		if err := ensurer.EnsureWorkers(resolvedCtx, tenantID); err != nil {
			if errors.Is(err, workers.ErrTenantCapReached) {
				logger.Log(ctx, libLog.LevelWarn, "Tenant worker cap reached; responding 503 so client backs off",
					libLog.String("operation", "middleware.reservations.lazy_spawn_workers"),
					libLog.String("tenant_id", tenantID),
					libLog.Err(err))

				c.Set("Retry-After", tenantCapRetryAfterHeader())

				return writeTenantCapReached(c)
			}

			logger.Log(ctx, libLog.LevelWarn, "Failed to ensure workers for tenant; reservation proceeds but background sync may be unavailable",
				libLog.String("operation", "middleware.reservations.lazy_spawn_workers"),
				libLog.String("tenant_id", tenantID),
				libLog.Err(err))
		}

		return c.Next()
	}
}
