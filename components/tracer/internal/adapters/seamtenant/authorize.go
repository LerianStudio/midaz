// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package seamtenant

import (
	"context"
	"errors"

	libObservability "github.com/LerianStudio/lib-observability/v4"
	libOpentelemetry "github.com/LerianStudio/lib-observability/v4/tracing"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"

	"github.com/LerianStudio/midaz/v4/components/tracer/internal/adapters/producerauth"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
)

// AuthorizeTenant is the transport-neutral tenant step of the reservation
// seam. It requires the producer bound by the identity step, authorizes the
// requested tenant for that producer's service, and binds the tenant's
// PostgreSQL pool into the returned context. Transports map the returned error
// onto their wire format and nothing else.
//
// The returned error is exactly one of these sentinels, compared with
// errors.Is or ==, and never carries its cause, which is recorded on the span:
//
//   - constant.ErrReservationTenantRequired: missing or malformed tenant id.
//   - constant.ErrInsufficientPrivileges: the tenant is not associated with the
//     producer's service, or its Tracer pool is denied.
//   - context.Canceled: the caller went away.
//   - context.DeadlineExceeded: the caller's deadline passed.
//   - constant.ErrTenantServiceUnavailable: the tenant-manager or the tenant
//     pool could not answer, an availability failure.
//   - constant.ErrContextPolicyUnavailable: a deployment defect — no producer,
//     an authorizer and resolver that disagree on multi-tenancy, or an
//     unconfigured lookup.
//
// In single-tenant mode the tenant id is ignored and ctx is returned
// unchanged. A tenant is never authorized without its pool or pooled without
// authorization.
func AuthorizeTenant(ctx context.Context, authz *producerauth.TenantAuthorizer, resolver *Resolver, tenantID string) (context.Context, error) {
	_, tracer, _, _ := libObservability.NewTrackingFromContext(ctx) //nolint:dogsled // only tracer is needed from tracking context

	spanCtx, span := tracer.Start(ctx, "middleware.reservations.authorize_tenant")
	defer span.End()

	producer, ok := producerauth.ProducerFromContext(ctx)
	if !ok {
		libOpentelemetry.HandleSpanError(span, "Reservation producer missing before tenant authorization", constant.ErrContextPolicyUnavailable)

		return ctx, constant.ErrContextPolicyUnavailable
	}

	if resolver.Active() != authz.Active() {
		libOpentelemetry.HandleSpanError(span, "Reservation tenant authorizer and pool resolver disagree on multi-tenancy", constant.ErrContextPolicyUnavailable)

		return ctx, constant.ErrContextPolicyUnavailable
	}

	if !authz.Active() {
		return ctx, nil
	}

	span.SetAttributes(attribute.String("app.request.tenant_id", tenantID))

	if err := authz.Authorize(spanCtx, tenantID, producer); err != nil {
		return ctx, classifyTenantError(span, err)
	}

	db, err := resolver.resolvePool(spanCtx, tenantID)
	if err != nil {
		return ctx, classifyTenantError(span, err)
	}

	return bindTenant(ctx, tenantID, db), nil
}

// classifyTenantError reduces err to its sentinel and records it on span: the
// caller-side outcomes as business events, the availability and deployment
// failures as span errors.
func classifyTenantError(span trace.Span, err error) error {
	switch {
	case errors.Is(err, constant.ErrReservationTenantRequired):
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Missing trusted tenant id on reservation surface", err)

		return constant.ErrReservationTenantRequired
	case errors.Is(err, constant.ErrInsufficientPrivileges):
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Reservation tenant rejected for producer", err)

		return constant.ErrInsufficientPrivileges
	case errors.Is(err, context.Canceled):
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Reservation tenant authorization canceled", err)

		return context.Canceled
	case errors.Is(err, context.DeadlineExceeded):
		libOpentelemetry.HandleSpanError(span, "Reservation tenant authorization timed out", err)

		return context.DeadlineExceeded
	case errors.Is(err, constant.ErrTenantServiceUnavailable):
		libOpentelemetry.HandleSpanError(span, "Reservation tenant could not be resolved", err)

		return constant.ErrTenantServiceUnavailable
	default:
		libOpentelemetry.HandleSpanError(span, "Failed to authorize reservation tenant", err)

		return constant.ErrContextPolicyUnavailable
	}
}
