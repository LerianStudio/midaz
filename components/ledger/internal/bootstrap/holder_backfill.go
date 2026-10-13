// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"context"
	"errors"
	"fmt"
	"strings"

	libCommons "github.com/LerianStudio/lib-commons/v7/commons"
	tmclient "github.com/LerianStudio/lib-commons/v7/commons/tenant-manager/client"
	tmcore "github.com/LerianStudio/lib-commons/v7/commons/tenant-manager/core"
	libLog "github.com/LerianStudio/lib-observability/v4/log"
	libOpentelemetry "github.com/LerianStudio/lib-observability/v4/tracing"
	libZap "github.com/LerianStudio/lib-observability/v4/zap"
	"go.mongodb.org/mongo-driver/v2/mongo"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/services/backfill"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
)

// activeTenantLister enumerates the active tenants for a service. It is the seam
// over *tmclient.Client used by the multi-tenant loop, so the loop can be tested
// without a live Tenant Manager HTTP client.
type activeTenantLister interface {
	GetActiveTenantsByService(ctx context.Context, service string) ([]*tmclient.TenantSummary, error)
}

// HolderBackfillRunner runs the cross-store self-holder backfill, then the metadata dedupe,
// against the ambient connections in single-tenant mode or once per active tenant otherwise.
type HolderBackfillRunner struct {
	logger             libLog.Logger
	multiTenantEnabled bool
	tenantServiceName  string

	runner *backfill.HolderBackfiller

	onbPG  *onboardingPostgresComponents
	crm    *crmComponents
	onbMgo *onboardingMongoComponents
	txnMgo *transactionMongoComponents

	tenantClient activeTenantLister

	// runForTenantFn is the per-tenant step the multi-tenant loop invokes. It is a
	// field (defaulting to runForTenant) only so the loop's orchestration — one
	// invocation per tenant and abort-on-first-failure — can be unit-tested without
	// real per-tenant PG + Mongo managers.
	runForTenantFn func(ctx context.Context, tenantID string) error
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

	telemetry, err := libOpentelemetry.NewTelemetry(telemetryConfig(cfg, logger))
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

	crm, err := initCRM(opts, cfg, nil, logger)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize CRM: %w", err)
	}

	onbMgo, err := initOnboardingMongo(opts, cfg, logger)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize onboarding MongoDB: %w", err)
	}

	txnMgo, err := initTransactionMongo(opts, cfg, logger)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize transaction MongoDB: %w", err)
	}

	runner := backfill.NewHolderBackfiller(onbPG.organizationRepo, crm.holderHandler.Service)

	r := &HolderBackfillRunner{
		logger:             logger,
		multiTenantEnabled: cfg.MultiTenantEnabled,
		tenantServiceName:  opts.TenantServiceName,
		runner:             runner,
		onbPG:              onbPG,
		crm:                crm,
		onbMgo:             onbMgo,
		txnMgo:             txnMgo,
	}

	r.runForTenantFn = r.runForTenant

	if cfg.MultiTenantEnabled {
		r.tenantClient = tenantClient
	}

	return r, nil
}

// Run executes the backfill. A failure on one tenant aborts the run, which is idempotent; a
// raced metadata collection does not, and the run ends with one error naming them all.
func (r *HolderBackfillRunner) Run(ctx context.Context) error {
	if !r.multiTenantEnabled {
		pgDB, err := r.onbPG.connection.Resolver(ctx)
		if err != nil {
			return fmt.Errorf("failed to resolve onboarding PG connection: %w", err)
		}

		ctx = tmcore.ContextWithPG(ctx, pgDB, constant.ModuleOnboarding)

		result, err := r.runner.RunTenant(ctx)
		if err != nil {
			return err
		}

		r.logger.Log(ctx, libLog.LevelInfo, "Holder backfill completed",
			libLog.Int("orgs_processed", result.OrgsProcessed),
			libLog.Int("holders_provisioned", result.HoldersProvisioned),
			libLog.Any("accounts_materialised", result.AccountsMaterialised))

		onbDB, err := r.onbMgo.connection.Database(ctx)
		if err != nil {
			return fmt.Errorf("failed to resolve onboarding Mongo database: %w", err)
		}

		txnDB, err := r.txnMgo.connection.Database(ctx)
		if err != nil {
			return fmt.Errorf("failed to resolve transaction Mongo database: %w", err)
		}

		return r.dedupeMetadata(ctx, onbDB, txnDB)
	}

	tenants, err := r.tenantClient.GetActiveTenantsByService(ctx, r.tenantServiceName)
	if err != nil {
		return fmt.Errorf("failed to list active tenants: %w", err)
	}

	var raced error

	for i, tenant := range tenants {
		if tenant == nil {
			r.logger.Log(ctx, libLog.LevelWarn, "Skipping nil tenant entry in active tenants list",
				libLog.Int("index", i))

			continue
		}

		switch err := r.runForTenantFn(ctx, tenant.ID); {
		case errors.Is(err, backfill.ErrMetadataRaced):
			raced = errors.Join(raced, fmt.Errorf("tenant %s: %w", tenant.ID, err))
		case err != nil:
			return fmt.Errorf("backfill failed for tenant %s: %w", tenant.ID, err)
		}
	}

	r.logger.Log(ctx, libLog.LevelInfo, "Holder backfill completed for all tenants",
		libLog.Int("tenants_processed", len(tenants)))

	return raced
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

	onbDB, err := r.onbMgo.mongoManager.GetDatabaseForTenant(tenantCtx, tenantID)
	if err != nil {
		return fmt.Errorf("failed to get onboarding Mongo database: %w", err)
	}

	txnDB, err := r.txnMgo.mongoManager.GetDatabaseForTenant(tenantCtx, tenantID)
	if err != nil {
		return fmt.Errorf("failed to get transaction Mongo database: %w", err)
	}

	return r.dedupeMetadata(tenantCtx, onbDB, txnDB)
}

// dedupeMetadata folds duplicate metadata documents in both modules' databases and makes
// entity_id unique on every metadata collection; raced collections are joined, not fatal.
func (r *HolderBackfillRunner) dedupeMetadata(ctx context.Context, onboarding, transaction *mongo.Database) error {
	var raced error

	for _, pass := range []struct {
		db       *mongo.Database
		entities []string
	}{{onboarding, onboardingMetadataEntities}, {transaction, transactionMetadataEntities}} {
		folded, err := backfill.DedupeMetadata(ctx, pass.db, pass.entities)
		if err != nil && !errors.Is(err, backfill.ErrMetadataRaced) {
			return err
		}

		raced = errors.Join(raced, err)

		r.logger.Log(ctx, libLog.LevelInfo, "Metadata dedupe completed",
			libLog.String("database", pass.db.Name()), libLog.Int("entities_folded", folded))
	}

	return raced
}
