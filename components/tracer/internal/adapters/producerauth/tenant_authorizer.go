// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package producerauth

import (
	"context"
	"errors"
	"fmt"

	tmcore "github.com/LerianStudio/lib-commons/v7/commons/tenant-manager/core"
	libObservability "github.com/LerianStudio/lib-observability/v4"
	libOpentelemetry "github.com/LerianStudio/lib-observability/v4/tracing"
	"go.opentelemetry.io/otel/attribute"

	"github.com/LerianStudio/midaz/v4/pkg/constant"
)

// TenantLookup reports whether tenantID holds an active association with
// service. It returns nil when it does, an error wrapping
// tmcore.ErrTenantNotFound or tmcore.ErrTenantServiceAccessDenied when the
// tenant-manager denies the association, and any other error when the
// tenant-manager could not answer.
type TenantLookup func(ctx context.Context, tenantID, service string) error

// TenantAuthorizer accepts a producer's requested tenant only when the
// tenant-manager confirms the tenant uses that producer's service.
type TenantAuthorizer struct {
	lookup    TenantLookup
	mtEnabled bool
}

// NewTenantAuthorizer builds an authorizer. With mtEnabled false every request
// is single-tenant and lookup is never called.
func NewTenantAuthorizer(lookup TenantLookup, mtEnabled bool) *TenantAuthorizer {
	return &TenantAuthorizer{lookup: lookup, mtEnabled: mtEnabled}
}

// Active reports whether the authorizer enforces a tenant.
func (a *TenantAuthorizer) Active() bool {
	return a != nil && a.mtEnabled
}

// Authorize applies, in order: single-tenant mode accepts without a lookup; a
// missing or malformed tenant id is constant.ErrReservationTenantRequired; an
// association the tenant-manager denies is constant.ErrInsufficientPrivileges;
// an unconfigured lookup is constant.ErrContextPolicyUnavailable, a
// deployment defect; a tenant-manager that cannot answer (network failure,
// 5xx, open circuit breaker, cancelled call) is
// constant.ErrTenantServiceUnavailable, an availability failure. Returned
// errors wrap the sentinel and the cause, so callers classify with errors.Is.
func (a *TenantAuthorizer) Authorize(ctx context.Context, tenantID string, producer Producer) error {
	if !a.Active() {
		return nil
	}

	if tenantID == "" || !tmcore.IsValidTenantID(tenantID) {
		return constant.ErrReservationTenantRequired
	}

	_, tracer, _, _ := libObservability.NewTrackingFromContext(ctx) //nolint:dogsled // only tracer is needed from tracking context

	ctx, span := tracer.Start(ctx, "producerauth.authorize_tenant")
	defer span.End()

	span.SetAttributes(
		attribute.String("app.request.tenant_id", tenantID),
		attribute.String("app.auth.producer_service", producer.Service),
	)

	if _, ok := platformRoster[producer.Service]; !ok {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Producer is not a platform producer", constant.ErrInsufficientPrivileges)

		return constant.ErrInsufficientPrivileges
	}

	if a.lookup == nil {
		libOpentelemetry.HandleSpanError(span, "Tenant association lookup is not configured", constant.ErrContextPolicyUnavailable)

		return constant.ErrContextPolicyUnavailable
	}

	err := a.lookup(ctx, tenantID, producer.Service)

	switch {
	case err == nil:
		return nil
	case errors.Is(err, tmcore.ErrTenantNotFound), errors.Is(err, tmcore.ErrTenantServiceAccessDenied):
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Tenant is not associated with the producer service", err)

		return fmt.Errorf("%w: %w", constant.ErrInsufficientPrivileges, err)
	case errors.Is(err, context.Canceled):
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Tenant association lookup canceled by the caller", err)

		return fmt.Errorf("%w: %w", constant.ErrTenantServiceUnavailable, err)
	default:
		libOpentelemetry.HandleSpanError(span, "Failed to look up tenant association", err)

		return fmt.Errorf("%w: %w", constant.ErrTenantServiceUnavailable, err)
	}
}
