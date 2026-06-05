// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"context"
	"fmt"
	"strings"

	libCommons "github.com/LerianStudio/lib-commons/v5/commons"
	tmclient "github.com/LerianStudio/lib-commons/v5/commons/tenant-manager/client"
	tmcore "github.com/LerianStudio/lib-commons/v5/commons/tenant-manager/core"
	libLog "github.com/LerianStudio/lib-observability/log"
	libOpentelemetry "github.com/LerianStudio/lib-observability/tracing"
	libZap "github.com/LerianStudio/lib-observability/zap"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/services/backfill"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
)

// HolderBackfillRunner is the composed entrypoint for the cross-store self-holder
// backfill. It owns the dependencies the runner needs and the multi-tenant
// iteration: single-tenant runs against the ambient connections; multi-tenant
// enumerates active tenants and injects per-tenant PG + Mongo connections before
// each pass, mirroring how the balance-sync worker scopes per-tenant work.
type HolderBackfillRunner struct {
	logger             libLog.Logger
	multiTenantEnabled bool
	tenantServiceName  string

	runner *backfill.HolderBackfiller

	onbPG *onboardingPostgresComponents
	crm   *crmComponents

	tenantClient *tmclient.Client
}

// InitHolderBackfill composes the backfill runner from environment configuration,
// reusing the binary's onboarding-PG and CRM initialisers. It does NOT start the
// HTTP server, RabbitMQ consumers, or any request-path infrastructure.
func InitHolderBackfill() (*HolderBackfillRunner, error) {
	cfg := &Config{}

	if err := libCommons.SetConfigFromEnvVars(cfg); err != nil {
		return nil, fmt.Errorf("failed to load config from environment variables: %w", err)
	}

	applyConfigDefaults(cfg)

	if cfg.MultiTenantEnabled && !cfg.AuthEnabled {
		return nil, fmt.Errorf("MULTI_TENANT_ENABLED=true requires PLUGIN_AUTH_ENABLED=true")
	}

	logger, err := libZap.New(libZap.Config{
		Environment:     resolveLoggerEnvironment(cfg.EnvName),
		Level:           cfg.LogLevel,
		OTelLibraryName: cfg.OtelLibraryName,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to initialize logger: %w", err)
	}

	telemetry, err := libOpentelemetry.NewTelemetry(libOpentelemetry.TelemetryConfig{
		LibraryName:               cfg.OtelLibraryName,
		ServiceName:               cfg.OtelServiceName,
		ServiceVersion:            cfg.OtelServiceVersion,
		DeploymentEnv:             cfg.OtelDeploymentEnv,
		CollectorExporterEndpoint: cfg.OtelColExporterEndpoint,
		EnableTelemetry:           cfg.EnableTelemetry,
		Logger:                    logger,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to initialize telemetry: %w", err)
	}

	if err := telemetry.ApplyGlobals(); err != nil {
		return nil, fmt.Errorf("failed to apply telemetry globals: %w", err)
	}

	tenantClient, tenantServiceName, err := initTenantClient(cfg, logger)
	if err != nil {
		return nil, err
	}

	opts := &Options{
		Logger:             logger,
		TenantClient:       tenantClient,
		TenantServiceName:  strings.TrimSpace(tenantServiceName),
		MultiTenantEnabled: cfg.MultiTenantEnabled,
	}

	onbPG, err := initOnboardingPostgres(opts, cfg, logger)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize onboarding postgres: %w", err)
	}

	crm, err := initCRM(opts, cfg, logger)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize CRM: %w", err)
	}

	runner := backfill.NewHolderBackfiller(onbPG.organizationRepo, crm.holderHandler.Service)

	r := &HolderBackfillRunner{
		logger:             logger,
		multiTenantEnabled: cfg.MultiTenantEnabled,
		tenantServiceName:  opts.TenantServiceName,
		runner:             runner,
		onbPG:              onbPG,
		crm:                crm,
	}

	if cfg.MultiTenantEnabled {
		r.tenantClient = tenantClient
	}

	return r, nil
}

// Run executes the backfill. In single-tenant mode it runs one pass against the
// ambient connections. In multi-tenant mode it enumerates active tenants and runs
// one pass per tenant against per-tenant PG + Mongo connections. A failure on one
// tenant aborts the whole run: leaving the rest unprocessed is safer than masking
// a fault, and the runner is idempotent so a re-run after the fix is a no-op.
func (r *HolderBackfillRunner) Run(ctx context.Context) error {
	if !r.multiTenantEnabled {
		result, err := r.runner.RunTenant(ctx)
		if err != nil {
			return err
		}

		r.logger.Log(ctx, libLog.LevelInfo, "Holder backfill completed",
			libLog.Int("orgs_processed", result.OrgsProcessed),
			libLog.Int("holders_provisioned", result.HoldersProvisioned),
			libLog.Any("accounts_materialised", result.AccountsMaterialised))

		return nil
	}

	tenants, err := r.tenantClient.GetActiveTenantsByService(ctx, r.tenantServiceName)
	if err != nil {
		return fmt.Errorf("failed to list active tenants: %w", err)
	}

	for _, tenant := range tenants {
		if err := r.runForTenant(ctx, tenant.ID); err != nil {
			return fmt.Errorf("backfill failed for tenant %s: %w", tenant.ID, err)
		}
	}

	r.logger.Log(ctx, libLog.LevelInfo, "Holder backfill completed for all tenants",
		libLog.Int("tenants_processed", len(tenants)))

	return nil
}

// runForTenant resolves the tenant's PG and Mongo connections and injects them
// into the context before running one backfill pass. PG goes under the onboarding
// module key (matching the repositories); Mongo goes under the generic key
// (matching the CRM holder repository's module-less getDatabase resolution).
func (r *HolderBackfillRunner) runForTenant(ctx context.Context, tenantID string) error {
	tenantCtx := tmcore.ContextWithTenantID(ctx, tenantID)

	conn, err := r.onbPG.pgManager.GetConnection(tenantCtx, tenantID)
	if err != nil {
		return fmt.Errorf("failed to get PG connection: %w", err)
	}

	pgDB, err := conn.GetDB()
	if err != nil {
		return fmt.Errorf("failed to get PG DB: %w", err)
	}

	tenantCtx = tmcore.ContextWithPG(tenantCtx, pgDB, constant.ModuleOnboarding)

	mongoDB, err := r.crm.mongoManager.GetDatabaseForTenant(tenantCtx, tenantID)
	if err != nil {
		return fmt.Errorf("failed to get Mongo database: %w", err)
	}

	tenantCtx = tmcore.ContextWithMB(tenantCtx, mongoDB)

	result, err := r.runner.RunTenant(tenantCtx)
	if err != nil {
		return err
	}

	r.logger.Log(tenantCtx, libLog.LevelInfo, "Holder backfill completed for tenant",
		libLog.String("tenant_id", tenantID),
		libLog.Int("orgs_processed", result.OrgsProcessed),
		libLog.Int("holders_provisioned", result.HoldersProvisioned),
		libLog.Any("accounts_materialised", result.AccountsMaterialised))

	return nil
}
