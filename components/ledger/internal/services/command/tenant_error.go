// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"errors"
	"fmt"

	tmcore "github.com/LerianStudio/lib-commons/v6/commons/tenant-manager/core"
	libOpentelemetry "github.com/LerianStudio/lib-observability/v4/tracing"
	"go.opentelemetry.io/otel/trace"

	"github.com/LerianStudio/midaz/v4/pkg"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
)

// MapTenantError converts tenant-manager errors into Midaz-specific error types
// so that the caller's HumaProblem can map them to the correct HTTP status codes.
func MapTenantError(ctx context.Context, err error, tenantID string) error {
	var suspErr *tmcore.TenantSuspendedError
	if errors.As(err, &suspErr) {
		return pkg.ForbiddenError{
			Code:    constant.ErrTenantServiceSuspended.Error(),
			Title:   "Service Suspended",
			Message: fmt.Sprintf("service is %s for tenant %s", suspErr.Status, tenantID),
		}
	}

	if errors.Is(err, tmcore.ErrTenantNotFound) {
		return pkg.EntityNotFoundError{
			Code:    constant.ErrTenantNotFound.Error(),
			Title:   "Tenant Not Found",
			Message: fmt.Sprintf("tenant not found: %s", tenantID),
		}
	}

	if tmcore.IsTenantNotProvisionedError(err) {
		return pkg.UnprocessableOperationError{
			Code:    constant.ErrTenantNotProvisioned.Error(),
			Title:   "Tenant Not Provisioned",
			Message: "Database schema not initialized for this tenant. Contact your administrator.",
		}
	}

	// err is deliberately not interpolated: this is a 503, and the ledger publishes
	// >=500 message text to clients. The tenant ID stays because it is the caller's
	// own. The cause is recorded on the span so it is not lost — without this the
	// only record of WHY tenant resolution failed would be gone.
	libOpentelemetry.HandleSpanError(trace.SpanFromContext(ctx), "Failed to resolve tenant database", err)

	return pkg.ServiceUnavailableError{
		Code:    constant.ErrTenantServiceUnavailable.Error(),
		Title:   "Tenant Service Unavailable",
		Message: fmt.Sprintf("failed to resolve tenant %s", tenantID),
	}
}
