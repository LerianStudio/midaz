// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"net/http"

	libAuth "github.com/LerianStudio/lib-auth/v5/auth/middleware"
	libHTTP "github.com/LerianStudio/lib-commons/v7/commons/net/http"
	libObservability "github.com/LerianStudio/lib-observability/v4"
	libOpentelemetry "github.com/LerianStudio/lib-observability/v4/tracing"
	"github.com/gofiber/fiber/v3"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"

	"github.com/LerianStudio/midaz/v4/components/tracer/internal/adapters/producerauth"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/contextutil"
	"github.com/LerianStudio/midaz/v4/pkg"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	pkgHTTP "github.com/LerianStudio/midaz/v4/pkg/net/http"
)

// ProducerAuthOption configures NewProducerAuthMiddleware.
type ProducerAuthOption func(*producerAuthOptions)

type producerAuthOptions struct {
	verificationDisabled bool
}

// WithProducerVerificationDisabled builds the chain for a deployment that
// explicitly disabled producer token verification (DEPLOYMENT_MODE=local): no
// token is read and every request is attributed to the ledger platform
// producer. The option is the only way to reach that attribution; a request
// that merely lacks a verified identity is never promoted to a producer.
func WithProducerVerificationDisabled() ProducerAuthOption {
	return func(o *producerAuthOptions) { o.verificationDisabled = true }
}

// NewProducerAuthMiddleware authenticates a platform producer on the
// reservation routes: lib-auth verifies the M2M access token locally, then the
// token's authorized party is resolved against the platform producer registry.
// On success the request context carries the Producer and the matching
// contextutil.IntegrationIdentity. Mount it before the reservation tenant
// middleware. A nil authenticator or registry fails every request closed.
//
// Rejections are problem documents: a missing token is 401 0041, a token that
// does not verify is 401 0042, and an authentic token that is not an
// application token, or whose authorized party is not a platform producer, is
// 403 0043. Token content never reaches the response.
func NewProducerAuthMiddleware(m2m *libAuth.M2MAuthenticator, reg *producerauth.Registry, opts ...ProducerAuthOption) []fiber.Handler {
	if m2m == nil || reg == nil {
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

	return []fiber.Handler{problemOnM2MRejection(m2m.RequireM2M()), resolveTokenProducer(reg)}
}

// m2mAdmittedKey marks, in fiber Locals, a request that lib-auth passed on to
// the producer resolver.
type m2mAdmittedKey struct{}

// problemOnM2MRejection runs lib-auth's token check and, when it answered the
// request itself instead of passing it on, replaces its plain-text 401/403
// with the tracer's problem document. The check's success is observed by the
// next handler in the chain marking the request as admitted.
func problemOnM2MRejection(requireM2M fiber.Handler) fiber.Handler {
	return func(c fiber.Ctx) error {
		if err := requireM2M(c); err != nil {
			return err
		}

		if admitted, _ := c.Locals(m2mAdmittedKey{}).(bool); admitted {
			return nil
		}

		return writeM2MRejection(c)
	}
}

func writeM2MRejection(c fiber.Ctx) error {
	_, tracer, _, _ := libObservability.NewTrackingFromContext(c.Context()) //nolint:dogsled // only tracer is needed from tracking context

	_, span := tracer.Start(c.Context(), "middleware.reservations.authenticate_producer")
	defer span.End()

	rejection := constant.ErrInvalidToken

	switch {
	case c.Response().StatusCode() == http.StatusForbidden:
		rejection = constant.ErrInsufficientPrivileges
	case libHTTP.ExtractTokenFromHeader(c) == "":
		rejection = constant.ErrTokenMissing
	}

	libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Reservation producer token rejected", rejection)

	c.Response().ResetBody()

	return pkgHTTP.WithError(c, pkg.ValidateBusinessError(rejection, ""))
}

func resolveTokenProducer(reg *producerauth.Registry) fiber.Handler {
	return func(c fiber.Ctx) error {
		c.Locals(m2mAdmittedKey{}, true)

		base := c.Context()

		_, tracer, _, _ := libObservability.NewTrackingFromContext(base)

		_, span := tracer.Start(base, "middleware.reservations.authenticate_producer")
		defer span.End()

		var (
			producer producerauth.Producer
			mapped   bool
		)

		if identity, ok := libAuth.M2MIdentityFromContext(base); ok {
			producer, mapped = reg.ByClientID(identity.ClientID)
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
