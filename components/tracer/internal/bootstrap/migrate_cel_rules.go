// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"context"
	"fmt"

	libCommons "github.com/LerianStudio/lib-commons/v6/commons"
	libLog "github.com/LerianStudio/lib-observability/v2/log"
	libZap "github.com/LerianStudio/lib-observability/v2/zap"

	"github.com/LerianStudio/midaz/v4/components/tracer/internal/adapters/cel"
	"github.com/LerianStudio/midaz/v4/components/tracer/internal/adapters/postgres"
	pgdb "github.com/LerianStudio/midaz/v4/components/tracer/internal/adapters/postgres/db"
	"github.com/LerianStudio/midaz/v4/components/tracer/internal/migration/celrules"
)

// CELRuleMigration bundles the fully-wired stored-rule migration and the
// resources that must be released when the one-shot job exits.
type CELRuleMigration struct {
	Logger   libLog.Logger
	Migrator *celrules.Migrator

	close func() error
}

// Close releases the migration's resources (the Postgres connection pool).
func (m *CELRuleMigration) Close() error {
	if m == nil || m.close == nil {
		return nil
	}

	return m.close()
}

// celCompileChecker adapts *cel.Environment to celrules.ExpressionCompiler.
// The recompile-all gate only needs the compile outcome, so the AST is
// discarded.
type celCompileChecker struct {
	env *cel.Environment
}

func (c celCompileChecker) Compile(expression string) error {
	_, err := c.env.Compile(expression)
	return err
}

// InitCELRuleMigration wires the stored-rule migration from the same
// configuration, Postgres pool, and CEL environment the tracer service uses at
// runtime — it does NOT reuse InitServers, so no HTTP/gRPC listener or worker is
// started. The caller MUST call Close on the returned value.
//
// The migration is single-tenant scoped: it resolves the connection through the
// static pool and has no per-tenant fan-out. It therefore refuses to run under
// MULTI_TENANT_ENABLED=true — the guard below fails loud before any resource is
// opened. Per-tenant execution is a documented follow-up.
func InitCELRuleMigration(ctx context.Context) (*CELRuleMigration, error) {
	cfg := &Config{}
	if err := libCommons.SetConfigFromEnvVars(cfg); err != nil {
		return nil, fmt.Errorf("failed to load config: %w", err)
	}

	ApplyMultiTenantDefaults(cfg)

	// Fail loud BEFORE opening the logger, the connection pool, or any
	// transaction: the job has no per-tenant fan-out, so running it in
	// multi-tenant mode would half-migrate the deployment.
	if cfg.MultiTenantEnabled {
		return nil, celrules.ErrMultiTenantUnsupported
	}

	zapEnv := libZap.Environment(cfg.OtelDeploymentEnv)
	if zapEnv == "" {
		zapEnv = libZap.EnvironmentDevelopment
	}

	zapLogger, err := libZap.New(libZap.Config{
		Environment:     zapEnv,
		Level:           cfg.LogLevel,
		OTelLibraryName: "tracer-migrate-cel-rules",
	})
	if err != nil {
		return nil, fmt.Errorf("failed to initialize logger: %w", err)
	}

	var logger libLog.Logger = zapLogger

	// Enforce the SaaS TLS posture before opening the Postgres connection —
	// the same gate the runtime bootstrap applies (a no-op in local/dev).
	if err := ValidateSaaSTLS(cfg); err != nil {
		return nil, fmt.Errorf("TLS enforcement: %w", err)
	}

	postgresConn, err := initPostgresConnection(ctx, cfg, logger)
	if err != nil {
		return nil, err
	}

	initSuccess := false

	defer func() {
		if !initSuccess {
			_ = postgresConn.Close()
		}
	}()

	pgConn := pgdb.NewPostgresConnectionAdapter(postgresConn)
	pgConn.SetMultiTenantEnabled(cfg.MultiTenantEnabled)

	ruleRepo := postgres.NewRepositoryWithConnection(pgConn)

	txBeginner, err := initTxBeginner(ctx, postgresConn, cfg.MultiTenantEnabled)
	if err != nil {
		return nil, err
	}

	celEnv, err := cel.NewEnvironment()
	if err != nil {
		return nil, fmt.Errorf("failed to create CEL environment: %w", err)
	}

	migrator, err := celrules.NewMigrator(txBeginner, ruleRepo, celCompileChecker{env: celEnv}, cel.RewriteCurrencyToAsset, cfg.MultiTenantEnabled)
	if err != nil {
		return nil, fmt.Errorf("failed to construct CEL rule migrator: %w", err)
	}

	initSuccess = true

	return &CELRuleMigration{
		Logger:   logger,
		Migrator: migrator,
		close:    postgresConn.Close,
	}, nil
}
