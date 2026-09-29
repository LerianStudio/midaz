// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package producerauth

import (
	"context"
	"errors"
	"fmt"

	tmcore "github.com/LerianStudio/lib-commons/v7/commons/tenant-manager/core"

	"github.com/LerianStudio/midaz/v4/pkg/constant"
)

// ServiceTenancy reports whether the tenant a request runs under holds an
// active association with a platform producer's service, the per-tenant
// switch of the reservation seam under multi-tenancy.
type ServiceTenancy struct {
	lookup  TenantLookup
	service string
}

// NewServiceTenancy builds the tenancy of service over lookup.
func NewServiceTenancy(lookup TenantLookup, service string) *ServiceTenancy {
	return &ServiceTenancy{lookup: lookup, service: service}
}

// Applies reports whether the tenant bound in ctx is associated with the
// service. A denied association is false. A context without a tenant is
// true, so the stricter rule governs a request whose tenant is unknown. A
// caller that went away gets its context error; an unconfigured lookup or a
// tenant-manager that cannot answer is an error wrapping
// constant.ErrTenantServiceUnavailable, never a false.
func (t *ServiceTenancy) Applies(ctx context.Context) (bool, error) {
	tenantID := tmcore.GetTenantIDContext(ctx)
	if tenantID == "" {
		return true, nil
	}

	if t == nil || t.lookup == nil {
		return false, fmt.Errorf("%w: tenant association lookup is not configured", constant.ErrTenantServiceUnavailable)
	}

	err := t.lookup(ctx, tenantID, t.service)

	switch {
	case err == nil:
		return true, nil
	case errors.Is(err, tmcore.ErrTenantNotFound), errors.Is(err, tmcore.ErrTenantServiceAccessDenied):
		return false, nil
	case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
		return false, err
	default:
		return false, fmt.Errorf("%w: %w", constant.ErrTenantServiceUnavailable, err)
	}
}
