// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"context"
	"errors"

	tmcore "github.com/LerianStudio/lib-commons/v7/commons/tenant-manager/core"
	libObservability "github.com/LerianStudio/lib-observability/v4"
	libOpentelemetry "github.com/LerianStudio/lib-observability/v4/tracing"
	"github.com/gofiber/fiber/v3"

	"github.com/LerianStudio/midaz/v4/components/tracer/internal/adapters/producerauth"
	"github.com/LerianStudio/midaz/v4/components/tracer/internal/adapters/seamtenant"
	"github.com/LerianStudio/midaz/v4/pkg"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	pkgHTTP "github.com/LerianStudio/midaz/v4/pkg/net/http"
)

// reservationTenantMiddleware accepts the X-Tenant-Id a platform producer asks
// for only after the tenant-manager confirms the tenant uses that producer's
// service, and then binds the tenant's PostgreSQL pool into the request
// context. It must follow the producer authentication middleware: a request
// without a resolved Producer is a wiring error and fails with 503. The
// routes mount it on the single-tenant chain; multi-tenant HTTP takes the
// tenant from the token through reservationTokenTenantMiddleware instead.
//
// Outcomes: a missing or malformed tenant under multi-tenancy is 400 0487; a
// tenant the tenant-manager does not associate with the producer, or whose
// Tracer pool it denies, is 403 0043; a tenant-manager or pool that cannot
// answer is 503 0161, an availability failure the caller's fail posture
// governs; a request the caller cancelled or whose deadline passed is 503 0330
// or 504 0422. Single-tenant mode ignores the header. A missing producer, and
// an authorizer and resolver that disagree on multi-tenancy, are deployment
// defects that fail closed with 503 0537, so a tenant is never authorized
// without its pool or pooled without authorization.
func reservationTenantMiddleware(authz *producerauth.TenantAuthorizer, resolver *seamtenant.Resolver) fiber.Handler {
	return func(c fiber.Ctx) error {
		resolvedCtx, err := seamtenant.AuthorizeTenant(c.Context(), authz, resolver, c.Get(seamtenant.HeaderName))
		if err != nil {
			return pkgHTTP.WithError(c, reservationTenantError(err))
		}

		c.SetContext(resolvedCtx)

		return c.Next()
	}
}

// tenantBoundLocal is the fiber.Ctx local through which the step after the
// tenant binder tells it the chain went past it, so an error returned from
// further down is never mistaken for a binder refusal.
type tenantBoundLocal struct{}

// reservationTokenTenantMiddleware returns the tenant steps of the
// multi-tenant reservation routes, to mount after
// NewTenantProducerAuthMiddleware: the tenant-manager must list the token
// tenant as active for the producer's service, then bind — the lib-commons
// tenant middleware's WithTenantDB, built with WithRefusalsToErrorHandler, as
// every other tenant route resolves its pool — binds the tenant pool, and the
// last step requires the bound tenant to be the token tenant the producer
// check accepted.
//
// Outcomes: a tenant the tenant-manager does not associate with the producer,
// or whose Tracer pool it denies, is 403 0043; a tenant-manager or pool that
// cannot answer is 503 0161; a request the caller cancelled or whose deadline
// passed is 503 0330 or 504 0422. A missing producer or token tenant, an
// inactive authorizer, and a bound tenant other than the token tenant are
// deployment defects that fail closed with 503 0537.
func reservationTokenTenantMiddleware(authz *producerauth.TenantAuthorizer, bind fiber.Handler) []fiber.Handler {
	if bind == nil {
		return []fiber.Handler{producerAuthUnavailable}
	}

	return []fiber.Handler{
		func(c fiber.Ctx) error {
			tenantID, _ := c.Locals(tokenTenantLocal{}).(string)
			if tenantID == "" {
				return reservationTenantDefect(c, "Reservation token tenant missing before tenant authorization")
			}

			if err := seamtenant.AuthorizeAssociation(c.Context(), authz, tenantID); err != nil {
				return pkgHTTP.WithError(c, reservationTenantError(err))
			}

			return c.Next()
		},
		func(c fiber.Ctx) error {
			passed := new(bool)
			c.Locals(tenantBoundLocal{}, passed)

			err := bind(c)
			if err == nil || *passed {
				return err
			}

			return pkgHTTP.WithError(c, reservationTenantError(seamtenant.ClassifyTenantDBRefusal(c.Context(), err)))
		},
		confirmTokenTenant,
	}
}

// confirmTokenTenant runs after the tenant binder and requires it to have
// bound the token tenant and a pool.
func confirmTokenTenant(c fiber.Ctx) error {
	if passed, ok := c.Locals(tenantBoundLocal{}).(*bool); ok {
		*passed = true
	}

	ctx := c.Context()
	tenantID, _ := c.Locals(tokenTenantLocal{}).(string)

	if tenantID != "" && tmcore.GetTenantIDContext(ctx) == tenantID && tmcore.GetPGContext(ctx) != nil {
		return c.Next()
	}

	return reservationTenantDefect(c, "Reservation tenant binding differs from the token tenant")
}

// reservationTenantDefect fails a request closed with 503 0537 for a wiring
// defect of the multi-tenant reservation chain, recording message on a span.
func reservationTenantDefect(c fiber.Ctx, message string) error {
	ctx := c.Context()

	_, tracer, _, _ := libObservability.NewTrackingFromContext(ctx) //nolint:dogsled // only tracer is needed from tracking context

	_, span := tracer.Start(ctx, "middleware.reservations.bind_tenant")
	defer span.End()

	libOpentelemetry.HandleSpanError(span, message, constant.ErrContextPolicyUnavailable)

	return pkgHTTP.WithError(c, pkg.ValidateBusinessError(constant.ErrContextPolicyUnavailable, ""))
}

// reservationTenantError maps a seamtenant.AuthorizeTenant sentinel onto its
// HTTP envelope.
func reservationTenantError(err error) error {
	switch {
	case errors.Is(err, constant.ErrReservationTenantRequired):
		return pkg.ValidateBusinessError(constant.ErrReservationTenantRequired, "")
	case errors.Is(err, constant.ErrInsufficientPrivileges):
		return pkg.ValidateBusinessError(constant.ErrInsufficientPrivileges, "")
	case errors.Is(err, context.Canceled):
		return pkg.ValidateBusinessError(constant.ErrContextCancelled, constant.EntityReservation)
	case errors.Is(err, context.DeadlineExceeded):
		return pkg.ValidateBusinessError(constant.ErrValidationTimeout, constant.EntityReservation)
	case errors.Is(err, constant.ErrTenantServiceUnavailable):
		return tenantServiceUnavailableError(constant.EntityReservation)
	default:
		return pkg.ValidateBusinessError(constant.ErrContextPolicyUnavailable, "")
	}
}

// tenantServiceUnavailableError is the 503 for a tenant-manager or tenant pool
// that could not answer, reported against entityType. The shared
// business-error registry carries no entry for this code, so the envelope is
// built here.
func tenantServiceUnavailableError(entityType string) error {
	return pkg.ServiceUnavailableError{
		EntityType: entityType,
		Code:       constant.ErrTenantServiceUnavailable.Error(),
		Title:      "Tenant Service Unavailable",
		Message:    "The tenant could not be resolved right now. Please retry.",
	}
}
