// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"context"
	"fmt"
	"time"

	tmcore "github.com/LerianStudio/lib-commons/v7/commons/tenant-manager/core"
	"github.com/bxcodec/dbresolver/v2"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/tracerobligation"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
)

// Disabling the runtime requires proof of drainage, even when its endpoint and
// identity settings have also been removed. This startup check does not replace
// stopping old writers during a coordinated rollout or draining suspended tenants
// before their catalog removal. Failure to inspect an active tenant fails boot.
func verifyTracerDrain(ctx context.Context, cfg *Config, deps contextTracerDependencies) error {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	if !cfg.MultiTenantEnabled {
		if deps.transaction == nil {
			return constant.ErrTracerContractUnavailable
		}

		database, err := deps.transaction.Resolver(ctx)
		if err != nil {
			return fmt.Errorf("resolve tracer drain primary: %w", err)
		}

		return requireTracerDrained(ctx, database)
	}

	if deps.catalog == nil || deps.resolver == nil || deps.service == "" {
		return constant.ErrTracerContractUnavailable
	}

	tenants, err := deps.catalog.GetActiveTenantsByService(ctx, deps.service)
	if err != nil {
		return fmt.Errorf("discover tracer drain tenants: %w", err)
	}

	seen := make(map[string]bool)

	for _, tenant := range tenants {
		if err := ctx.Err(); err != nil {
			return err
		}

		if tenant == nil || !tmcore.IsValidTenantID(tenant.ID) || tenant.Status != "active" {
			return constant.ErrTracerContractUnavailable
		}

		if seen[tenant.ID] {
			continue
		}

		seen[tenant.ID] = true
		tenantCtx := tmcore.ContextWithTenantID(ctx, tenant.ID)

		database, err := deps.resolver.GetDB(tenantCtx, tenant.ID)
		if err != nil {
			return fmt.Errorf("resolve tracer drain tenant: %w", err)
		}

		if err := requireTracerDrained(tenantCtx, database); err != nil {
			return err
		}
	}

	return nil
}

func requireTracerDrained(ctx context.Context, database dbresolver.DB) error {
	pending, err := tracerobligation.HasUndelivered(ctx, database)
	if err != nil {
		return err
	}

	if pending {
		return fmt.Errorf("undelivered Tracer obligations require TRACER_CONTEXT_ENABLED=true; drain before disabling recovery: %w", constant.ErrTracerContractUnavailable)
	}

	return nil
}
