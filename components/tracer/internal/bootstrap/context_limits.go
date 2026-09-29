// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"context"
	"errors"
	"fmt"
	"strconv"

	"github.com/LerianStudio/midaz/v4/components/tracer/internal/adapters/producerauth"
	"github.com/LerianStudio/midaz/v4/components/tracer/internal/services/command"
	"github.com/LerianStudio/midaz/v4/pkg/tracercontract"
)

// initContextLimitDefinitionPolicy returns the account-only definition policy
// limits must satisfy while the reservation surface is enabled, and nil (any
// scope accepted) on a validations-only Tracer, where no reservation reads
// limits by account. Under multi-tenancy the surface is enabled per tenant,
// so the policy is scoped to tenancy: it applies to a tenant only while that
// tenant takes part in the reservation seam.
func initContextLimitDefinitionPolicy(cfg *Config, tenancy command.ReservationTenancy) (*command.ContextLimitDefinitionPolicy, error) {
	if !reservationSurfaceEnabled(cfg) {
		return nil, nil
	}

	if cfg.MultiTenantEnabled && tenancy == nil {
		return nil, fmt.Errorf("multi-tenant limit administration requires the tenant reservation tenancy")
	}

	facts, err := loadContextFactBounds(cfg)
	if err != nil {
		return nil, err
	}

	policy, err := command.NewContextLimitDefinitionPolicy(facts, cfg.ContextLimitMaxScopes, cfg.ContextLimitMaxScopeBytes)
	if err != nil || !cfg.MultiTenantEnabled {
		return policy, err
	}

	return policy.ScopedTo(tenancy), nil
}

// errTenantAssociationsUnbound refuses a multi-tenant boot whose limit
// commands could not be bound to the tenant association set.
var errTenantAssociationsUnbound = errors.New("multi-tenant limit administration requires the tenant association set")

// deferredTenantLookup is the tenant association lookup the limit commands
// are built with before the multi-tenant components exist. bootstrap binds it
// once, before any listener starts; unbound, every lookup is an availability
// failure, never a denial.
type deferredTenantLookup struct {
	lookup producerauth.TenantLookup
}

// Lookup satisfies producerauth.TenantLookup.
func (d *deferredTenantLookup) Lookup(ctx context.Context, tenantID, service string) error {
	if d == nil || d.lookup == nil {
		return fmt.Errorf("%w for %s: tenant association lookup is not bound", errActiveTenantsUnavailable, service)
	}

	return d.lookup(ctx, tenantID, service)
}

// bind installs the multi-tenant association lookup. It must run before the
// listeners start. A nil receiver is single-tenant mode and binds nothing; a
// multi-tenant receiver without the association set refuses boot, because the
// limit commands would otherwise answer every tenant with an outage.
func (d *deferredTenantLookup) bind(mtComponents *componentsMT) error {
	if d == nil {
		return nil
	}

	if mtComponents == nil || mtComponents.tenantAssociations == nil {
		return errTenantAssociationsUnbound
	}

	d.lookup = mtComponents.tenantAssociations.Lookup

	return nil
}

func loadContextFactBounds(cfg *Config) (tracercontract.Limits, error) {
	fraction, err := strconv.Atoi(cfg.ContextMaxFractionDigits)
	if err != nil || fraction < tracercontract.MinimumResourceProfileFractionDigits {
		return tracercontract.Limits{}, fmt.Errorf("CONTEXT_MAX_FRACTION_DIGITS must be at least %d", tracercontract.MinimumResourceProfileFractionDigits)
	}

	bounds := tracercontract.Limits{MaxAccounts: cfg.ContextMaxAccounts, MaxEntries: cfg.ContextMaxEntries, MaxTextBytes: cfg.ContextMaxTextBytes, MaxIntegerDigits: cfg.ContextMaxIntegerDigits, MaxFractionDigits: fraction}
	if err := bounds.Validate(); err != nil {
		return tracercontract.Limits{}, fmt.Errorf("invalid CONTEXT resource bounds: %w", err)
	}

	return bounds, nil
}
