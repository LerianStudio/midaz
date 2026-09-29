// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	tmcore "github.com/LerianStudio/lib-commons/v7/commons/tenant-manager/core"
	libObservability "github.com/LerianStudio/lib-observability/v4"
	libOpentelemetry "github.com/LerianStudio/lib-observability/v4/tracing"
	"github.com/gofiber/fiber/v3"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"

	"github.com/LerianStudio/midaz/v4/components/tracer/internal/adapters/http/in/middleware"
	"github.com/LerianStudio/midaz/v4/components/tracer/internal/adapters/producerauth"
	"github.com/LerianStudio/midaz/v4/components/tracer/internal/adapters/seamtenant"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/contextutil"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/model"
	"github.com/LerianStudio/midaz/v4/pkg"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	pkgHTTP "github.com/LerianStudio/midaz/v4/pkg/net/http"
)

// reservationsResource and reservationsAction are the Access Manager tuple
// every reservation route authorizes as.
const (
	reservationsResource = "reservations"
	reservationsAction   = "post"
)

// ProducerAuthOption configures NewProducerAuthMiddleware.
type ProducerAuthOption func(*producerAuthOptions)

type producerAuthOptions struct {
	verificationDisabled bool
}

// WithProducerVerificationDisabled builds the chain for a deployment that
// runs without plugin auth under DEPLOYMENT_MODE=local: the chain carries no
// guard, so neither a token nor an API key is asked for, and every request is
// attributed to the ledger platform producer. The option is the only way to
// reach that attribution; a request that merely lacks a verified identity is
// never promoted to a producer.
func WithProducerVerificationDisabled() ProducerAuthOption {
	return func(o *producerAuthOptions) { o.verificationDisabled = true }
}

// NewProducerAuthMiddleware returns the authentication chain of the
// reservation routes, to mount before the reservation tenant middleware: the
// guard authorizes the bearer token against the Access Manager as
// tracer/reservations:post, then the authorized token must be an application
// token whose authorized party is a registered platform producer. The guard
// is part of the chain so the producer is never read from a token the Access
// Manager did not authorize. On success the request context carries the
// Producer and the matching contextutil.IntegrationIdentity, and the actor
// principal the guard published is marked as a system actor, because a
// producer is never a user. A nil registry, or a nil guard on a verified
// chain, fails every request closed.
//
// The guard answers a missing or refused token itself (401, 403, or 503 when
// the Access Manager cannot decide). An authorized token that is not an
// application token, or whose authorized party is not a platform producer, is
// 403 0043. Token content never reaches the response.
func NewProducerAuthMiddleware(guard *middleware.AuthGuard, reg *producerauth.Registry, opts ...ProducerAuthOption) []fiber.Handler {
	if reg == nil {
		return []fiber.Handler{producerAuthUnavailable}
	}

	var options producerAuthOptions

	for _, opt := range opts {
		if opt != nil {
			opt(&options)
		}
	}

	if options.verificationDisabled {
		return []fiber.Handler{attributeUnverifiedProducer}
	}

	if guard == nil {
		return []fiber.Handler{producerAuthUnavailable}
	}

	return guard.WithAuthorizedCaller(reservationsResource, reservationsAction, resolveTokenProducer(reg))
}

// NewTenantProducerAuthMiddleware returns the authentication chain of the
// reservation routes under multi-tenancy, to mount before
// reservationTokenTenantMiddleware. The guard authorizes the bearer token as
// tracer/reservations:post; the authorized token must then be an application
// token carrying the platform attributes the tenant-manager writes onto the
// per-tenant machine-to-machine application it provisions for a producer:
// "isInternal" set to "true", a "sourceService" that is a platform producer,
// and a valid "tenantId". No operator roster is consulted: only the
// tenant-manager can write those attributes. The token tenant is the tenant
// of the request; an X-Tenant-Id header, which the producer may send, is
// accepted only when it names the same tenant. On success the context carries
// the Producer and its contextutil.IntegrationIdentity, and the actor
// principal is marked as a system actor.
//
// The guard answers a missing or refused token itself (401, 403, or 503 when
// the Access Manager cannot decide). An authorized token that fails any claim
// check, or whose X-Tenant-Id differs from its tenant, is 403 0043. Token
// content never reaches the response. A nil guard fails every request closed.
func NewTenantProducerAuthMiddleware(guard *middleware.AuthGuard) []fiber.Handler {
	if guard == nil {
		return []fiber.Handler{producerAuthUnavailable}
	}

	return guard.WithAuthorizedCaller(reservationsResource, reservationsAction, resolveTenantTokenProducer)
}

// tokenTenantLocal is the fiber.Ctx local holding the canonical tenant of the
// producer token resolveTenantTokenProducer accepted.
type tokenTenantLocal struct{}

func resolveTenantTokenProducer(c fiber.Ctx, caller middleware.TokenCaller, ok bool) error {
	base := c.Context()

	_, tracer, _, _ := libObservability.NewTrackingFromContext(base) //nolint:dogsled // only tracer is needed from tracking context

	_, span := tracer.Start(base, "middleware.reservations.authenticate_producer")
	defer span.End()

	producer, tenantID, refusal := tenantTokenProducer(caller, ok, c.Get(seamtenant.HeaderName))
	if refusal != "" {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, refusal, constant.ErrInsufficientPrivileges)

		return pkgHTTP.WithError(c, pkg.ValidateBusinessError(constant.ErrInsufficientPrivileges, ""))
	}

	span.SetAttributes(attribute.String("app.request.tenant_id", tenantID))
	c.Locals(tokenTenantLocal{}, tenantID)

	return nextWithProducer(c, span, producer)
}

// tenantTokenProducer applies the multi-tenant claim checks to an authorized
// caller and returns its producer and canonical tenant, or the reason it is
// refused. requested is the X-Tenant-Id header, empty when absent.
func tenantTokenProducer(caller middleware.TokenCaller, ok bool, requested string) (producerauth.Producer, string, string) {
	switch {
	case !ok || caller.Type != middleware.TokenTypeApplication:
		return producerauth.Producer{}, "", "Reservation caller is not an application token"
	case !caller.Internal:
		return producerauth.Producer{}, "", "Reservation token is not a platform application"
	case !producerauth.InRoster(caller.SourceService):
		return producerauth.Producer{}, "", "Reservation token names no platform producer"
	}

	tenantID, err := tmcore.CanonicalTenantID(caller.TenantID)
	if err != nil {
		return producerauth.Producer{}, "", "Reservation token carries no valid tenant"
	}

	if requested != "" {
		canonical, err := tmcore.CanonicalTenantID(requested)
		if err != nil || canonical != tenantID {
			return producerauth.Producer{}, "", "Requested tenant differs from the token tenant"
		}
	}

	return producerauth.Producer{Service: caller.SourceService, Via: producerauth.ViaToken}, tenantID, ""
}

func resolveTokenProducer(reg *producerauth.Registry) middleware.CallerResolver {
	return func(c fiber.Ctx, caller middleware.TokenCaller, ok bool) error {
		base := c.Context()

		_, tracer, _, _ := libObservability.NewTrackingFromContext(base)

		_, span := tracer.Start(base, "middleware.reservations.authenticate_producer")
		defer span.End()

		var (
			producer producerauth.Producer
			mapped   bool
		)

		if ok && caller.Type == middleware.TokenTypeApplication {
			producer, mapped = reg.ByClientID(caller.ClientID)
		}

		if !mapped {
			libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Reservation producer rejected", constant.ErrInsufficientPrivileges)

			return pkgHTTP.WithError(c, pkg.ValidateBusinessError(constant.ErrInsufficientPrivileges, ""))
		}

		return nextWithProducer(c, span, producer)
	}
}

// attributeUnverifiedProducer serves a chain built with
// WithProducerVerificationDisabled.
func attributeUnverifiedProducer(c fiber.Ctx) error {
	base := c.Context()

	_, tracer, _, _ := libObservability.NewTrackingFromContext(base) //nolint:dogsled // only tracer is needed from tracking context

	_, span := tracer.Start(base, "middleware.reservations.authenticate_producer")
	defer span.End()

	span.SetAttributes(attribute.Bool("app.auth.producer_verification_disabled", true))

	return nextWithProducer(c, span, producerauth.Producer{Service: producerauth.ServiceLedger, Via: producerauth.ViaToken})
}

func nextWithProducer(c fiber.Ctx, span trace.Span, producer producerauth.Producer) error {
	span.SetAttributes(
		attribute.String("app.auth.producer_service", producer.Service),
		attribute.String("app.auth.producer_via", producer.Via),
	)

	ctx := producerauth.WithProducer(c.Context(), producer)
	ctx = contextutil.WithIntegrationIdentity(ctx, contextutil.IntegrationIdentity{ID: producer.Service})

	if principal, ok := contextutil.GetPrincipal(ctx); ok {
		principal.Type = string(model.ActorTypeSystem)
		ctx = contextutil.WithPrincipal(ctx, principal)
	}

	c.SetContext(ctx)

	return c.Next()
}

func producerAuthUnavailable(c fiber.Ctx) error {
	_, tracer, _, _ := libObservability.NewTrackingFromContext(c.Context()) //nolint:dogsled // only tracer is needed from tracking context

	_, span := tracer.Start(c.Context(), "middleware.reservations.authenticate_producer")
	defer span.End()

	libOpentelemetry.HandleSpanError(span, "Reservation producer authentication is not configured", constant.ErrContextPolicyUnavailable)

	return pkgHTTP.WithError(c, pkg.ValidateBusinessError(constant.ErrContextPolicyUnavailable, ""))
}
