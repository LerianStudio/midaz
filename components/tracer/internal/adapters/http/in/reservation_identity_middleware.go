// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"errors"

	libObservability "github.com/LerianStudio/lib-observability/v4"
	libOpentelemetry "github.com/LerianStudio/lib-observability/v4/tracing"
	"github.com/gofiber/fiber/v3"

	"github.com/LerianStudio/midaz/v4/components/tracer/internal/adapters/seamidentity"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/contextutil"
	"github.com/LerianStudio/midaz/v4/pkg"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	pkgHTTP "github.com/LerianStudio/midaz/v4/pkg/net/http"
)

// NewReservationIdentityMiddleware uses the native TLS connection verified by
// the listener. Forwarded identity/certificate headers are never authentication.
// Mount on context reservation and official asset-association routes. Asset
// administration also requires independent JWT authorization; this middleware
// does not replace it. Reservation tenant resolution must follow this guard.
func NewReservationIdentityMiddleware(resolver *seamidentity.Resolver) fiber.Handler {
	return newPurposeIdentityMiddleware(resolver, seamidentity.PurposeReserve)
}

// NewLimitAssetIdentityMiddleware requires a separately granted admin purpose.
// Access Manager authorization remains mandatory after certificate verification.
func NewLimitAssetIdentityMiddleware(resolver *seamidentity.Resolver) fiber.Handler {
	return newPurposeIdentityMiddleware(resolver, seamidentity.PurposeAssetAdmin)
}

func newPurposeIdentityMiddleware(resolver *seamidentity.Resolver, purpose seamidentity.Purpose) fiber.Handler {
	return func(c fiber.Ctx) error {
		_, tracer, _, _ := libObservability.NewTrackingFromContext(c.Context())

		ctx, span := tracer.Start(c.Context(), "middleware.reservations.resolve_identity")
		defer span.End()

		identity, err := resolver.ResolveTLSFor(ctx, c.RequestCtx().TLSConnectionState(), purpose)
		if err != nil {
			if errors.Is(err, constant.ErrInsufficientPrivileges) {
				libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Reservation producer rejected", err)
			} else {
				libOpentelemetry.HandleSpanError(span, "Reservation identity resolution failed", err)
			}

			return pkgHTTP.WithError(c, pkg.ValidateBusinessError(err, constant.EntityContextPolicy))
		}

		c.SetContext(contextutil.WithIntegrationIdentity(ctx, identity))

		return c.Next()
	}
}
