// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	tmcore "github.com/LerianStudio/lib-commons/v7/commons/tenant-manager/core"
	libLog "github.com/LerianStudio/lib-observability/v4/log"
	"github.com/bxcodec/dbresolver/v2"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/tracerobligation"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
)

// tracerDrainRequired distinguishes an explicitly configured shared profile
// being disabled from the default state and from the legacy REST integration.
// Operators must retain at least one shared identity setting until drainage is
// complete; the rollout procedure forbids removing both settings first.
func tracerDrainRequired(cfg *Config) bool {
	return cfg != nil && (strings.TrimSpace(cfg.TracerIntegrationID) != "" || strings.TrimSpace(cfg.TracerAssetNamespace) != "")
}

// Disabling a previously configured shared runtime requires proof of drainage.
// This startup check does not replace stopping old writers during a coordinated
// rollout or draining suspended tenants before their catalog removal. A readable
// participating tenant remains fail-closed when inspection fails.
func verifyTracerDrain(ctx context.Context, cfg *Config, deps contextTracerDependencies) error {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	if !cfg.MultiTenantEnabled {
		return verifySingleTenantTracerDrain(ctx, deps)
	}

	return verifyMultiTenantTracerDrain(ctx, deps)
}

func verifySingleTenantTracerDrain(ctx context.Context, deps contextTracerDependencies) error {
	if deps.transaction == nil {
		return constant.ErrTracerContractUnavailable
	}

	database, err := deps.transaction.Resolver(ctx)
	if err != nil {
		return fmt.Errorf("resolve tracer drain primary: %w", err)
	}

	return requireTracerDrained(ctx, database)
}

func verifyMultiTenantTracerDrain(ctx context.Context, deps contextTracerDependencies) error {
	if deps.catalog == nil || deps.resolver == nil || deps.service == "" {
		return constant.ErrTracerContractUnavailable
	}

	tenants, err := deps.catalog.GetActiveTenantsByService(ctx, deps.service)
	if err != nil {
		return fmt.Errorf("discover tracer drain tenants: %w", err)
	}

	seen := make(map[string]bool)
	skipped := 0

	for _, tenant := range tenants {
		if err := ctx.Err(); err != nil {
			return err
		}

		if tenant == nil || !eligibleTracerRecoveryTenant(tenant.ID, tenant.Status) {
			skipped++
			continue
		}

		if seen[tenant.ID] {
			continue
		}

		seen[tenant.ID] = true
		tenantCtx := tmcore.ContextWithTenantID(ctx, tenant.ID)

		database, err := deps.resolver.GetDB(tenantCtx, tenant.ID)
		if err != nil {
			if errors.Is(err, tmcore.ErrServiceNotConfigured) || errors.Is(err, tmcore.ErrTenantNotFound) {
				skipped++
				continue
			}

			return fmt.Errorf("resolve tracer drain tenant: %w", err)
		}

		if err := requireTracerDrained(tenantCtx, database); err != nil {
			return err
		}
	}

	if skipped > 0 && deps.logger != nil {
		deps.logger.Log(ctx, libLog.LevelWarn, "Tracer drain skipped tenants without eligible transaction storage", libLog.Int("count", skipped))
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
