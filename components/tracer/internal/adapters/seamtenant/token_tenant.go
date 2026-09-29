// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package seamtenant

import (
	"context"
	"errors"

	tmcore "github.com/LerianStudio/lib-commons/v7/commons/tenant-manager/core"
	libObservability "github.com/LerianStudio/lib-observability/v4"
	libOpentelemetry "github.com/LerianStudio/lib-observability/v4/tracing"
	"go.opentelemetry.io/otel/attribute"

	"github.com/LerianStudio/midaz/v4/components/tracer/internal/adapters/producerauth"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
)

// AuthorizeAssociation is the association step of AuthorizeTenant for a
// tenant the transport has already established, as the HTTP seam does under
// multi-tenancy from the authorized token: it requires the producer bound by
// the identity step and has authz confirm the tenant is active for that
// producer's service. It binds no pool; the caller resolves one only after a
// nil return. It returns the AuthorizeTenant sentinels, and
// constant.ErrContextPolicyUnavailable for an inactive authorizer, because a
// tenant established from the token is never single-tenant.
func AuthorizeAssociation(ctx context.Context, authz *producerauth.TenantAuthorizer, tenantID string) error {
	_, tracer, _, _ := libObservability.NewTrackingFromContext(ctx) //nolint:dogsled // only tracer is needed from tracking context

	spanCtx, span := tracer.Start(ctx, "middleware.reservations.authorize_tenant")
	defer span.End()

	producer, ok := producerauth.ProducerFromContext(ctx)
	if !ok {
		libOpentelemetry.HandleSpanError(span, "Reservation producer missing before tenant authorization", constant.ErrContextPolicyUnavailable)

		return constant.ErrContextPolicyUnavailable
	}

	if !authz.Active() {
		libOpentelemetry.HandleSpanError(span, "Reservation tenant authorizer is not active under multi-tenancy", constant.ErrContextPolicyUnavailable)

		return constant.ErrContextPolicyUnavailable
	}

	span.SetAttributes(attribute.String("app.request.tenant_id", tenantID))

	if err := authz.Authorize(spanCtx, tenantID, producer); err != nil {
		return classifyTenantError(span, err)
	}

	return nil
}

// ClassifyTenantDBRefusal reduces a refusal of the lib-commons tenant
// middleware to the AuthorizeTenant sentinels. ctx is the request context,
// consulted so a caller that went away is reported as such even when the
// refusal does not wrap its context error. A token or tenant claim the
// middleware could not use is constant.ErrInsufficientPrivileges, as is a
// tenant the tenant-manager denies; any other refusal is
// constant.ErrTenantServiceUnavailable. The middleware has already recorded
// the refusal on its own span.
func ClassifyTenantDBRefusal(ctx context.Context, err error) error {
	switch {
	case errors.Is(err, context.Canceled) || errors.Is(ctx.Err(), context.Canceled):
		return context.Canceled
	case errors.Is(err, context.DeadlineExceeded) || errors.Is(ctx.Err(), context.DeadlineExceeded):
		return context.DeadlineExceeded
	case errors.Is(err, tmcore.ErrAuthorizationTokenRequired), errors.Is(err, tmcore.ErrInvalidAuthorizationToken),
		errors.Is(err, tmcore.ErrInvalidTenantClaims), errors.Is(err, tmcore.ErrMissingTenantIDClaim):
		return constant.ErrInsufficientPrivileges
	case errors.Is(classifyPoolError(err), constant.ErrInsufficientPrivileges):
		return constant.ErrInsufficientPrivileges
	default:
		return constant.ErrTenantServiceUnavailable
	}
}
