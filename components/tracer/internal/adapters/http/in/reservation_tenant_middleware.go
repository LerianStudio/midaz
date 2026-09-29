// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"context"
	"errors"

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
// without a resolved Producer is a wiring error and fails with 503.
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
		return tenantServiceUnavailableError()
	default:
		return pkg.ValidateBusinessError(constant.ErrContextPolicyUnavailable, "")
	}
}

// tenantServiceUnavailableError is the 503 for a tenant-manager or tenant pool
// that could not answer. The shared business-error registry carries no entry
// for this code, so the envelope is built here.
func tenantServiceUnavailableError() error {
	return pkg.ServiceUnavailableError{
		EntityType: constant.EntityReservation,
		Code:       constant.ErrTenantServiceUnavailable.Error(),
		Title:      "Tenant Service Unavailable",
		Message:    "The tenant could not be resolved right now. Please retry.",
	}
}
