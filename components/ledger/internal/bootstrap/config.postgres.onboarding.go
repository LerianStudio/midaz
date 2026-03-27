// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"context"
	"fmt"
	"time"

	libLog "github.com/LerianStudio/lib-commons/v4/commons/log"
	libPostgres "github.com/LerianStudio/lib-commons/v4/commons/postgres"
	tmpostgres "github.com/LerianStudio/lib-commons/v4/commons/tenant-manager/postgres"
	"github.com/LerianStudio/midaz/v3/components/ledger/internal/adapters/postgres/account"
	"github.com/LerianStudio/midaz/v3/components/ledger/internal/adapters/postgres/accounttype"
	"github.com/LerianStudio/midaz/v3/components/ledger/internal/adapters/postgres/asset"
	"github.com/LerianStudio/midaz/v3/components/ledger/internal/adapters/postgres/ledger"
	"github.com/LerianStudio/midaz/v3/components/ledger/internal/adapters/postgres/organization"
	"github.com/LerianStudio/midaz/v3/components/ledger/internal/adapters/postgres/portfolio"
	"github.com/LerianStudio/midaz/v3/components/ledger/internal/adapters/postgres/segment"
	"github.com/LerianStudio/midaz/v3/pkg/constant"
)

// onboardingPostgresComponents holds PostgreSQL-related components for the onboarding domain.
type onboardingPostgresComponents struct {
	connection       *libPostgres.Client
	pgManager        *tmpostgres.Manager // nil in single-tenant mode; used by TenantMiddleware
	organizationRepo *organization.OrganizationPostgreSQLRepository
	ledgerRepo       *ledger.LedgerPostgreSQLRepository
	accountRepo      *account.AccountPostgreSQLRepository
	assetRepo        *asset.AssetPostgreSQLRepository
	portfolioRepo    *portfolio.PortfolioPostgreSQLRepository
	segmentRepo      *segment.SegmentPostgreSQLRepository
	accountTypeRepo  *accounttype.AccountTypePostgreSQLRepository
}

// initOnboardingPostgres initializes PostgreSQL components for the onboarding domain.
// Dispatches to single-tenant or multi-tenant initialization based on Options.
func initOnboardingPostgres(opts *Options, cfg *Config, logger libLog.Logger) (*onboardingPostgresComponents, error) {
	if opts != nil && opts.MultiTenantEnabled {
		return initOnboardingMultiTenantPostgres(opts, cfg, logger)
	}

	return initOnboardingSingleTenantPostgres(cfg, logger)
}

// initOnboardingMultiTenantPostgres initializes PostgreSQL in multi-tenant mode for onboarding.
func initOnboardingMultiTenantPostgres(opts *Options, cfg *Config, logger libLog.Logger) (*onboardingPostgresComponents, error) {
	logger.Log(context.Background(), libLog.LevelInfo, "Initializing multi-tenant PostgreSQL for onboarding")

	if opts.TenantClient == nil {
		return nil, fmt.Errorf("TenantClient is required for multi-tenant PostgreSQL initialization")
	}

	pgOpts := []tmpostgres.Option{
		tmpostgres.WithModule(constant.ModuleOnboarding),
		tmpostgres.WithLogger(logger),
	}

	if cfg.MultiTenantConnectionsCheckIntervalSec > 0 {
		pgOpts = append(pgOpts, tmpostgres.WithConnectionsCheckInterval(time.Duration(cfg.MultiTenantConnectionsCheckIntervalSec)*time.Second))
	}

	pgMgr := tmpostgres.NewManager(
		opts.TenantClient,
		opts.TenantServiceName,
		pgOpts...,
	)

	// Build and connect. In multi-tenant mode, this connection serves as
	// fallback/placeholder; actual per-tenant connections are resolved by middleware.
	conn, err := onboardingPostgresConnector(cfg, logger)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to PostgreSQL (multi-tenant): %w", err)
	}

	return &onboardingPostgresComponents{
		connection:       conn,
		pgManager:        pgMgr,
		organizationRepo: organization.NewOrganizationPostgreSQLRepository(conn, true),
		ledgerRepo:       ledger.NewLedgerPostgreSQLRepository(conn, true),
		accountRepo:      account.NewAccountPostgreSQLRepository(conn, true),
		assetRepo:        asset.NewAssetPostgreSQLRepository(conn, true),
		portfolioRepo:    portfolio.NewPortfolioPostgreSQLRepository(conn, true),
		segmentRepo:      segment.NewSegmentPostgreSQLRepository(conn, true),
		accountTypeRepo:  accounttype.NewAccountTypePostgreSQLRepository(conn, true),
	}, nil
}

// initOnboardingSingleTenantPostgres initializes PostgreSQL in single-tenant mode for onboarding.
func initOnboardingSingleTenantPostgres(cfg *Config, logger libLog.Logger) (*onboardingPostgresComponents, error) {
	conn, err := onboardingPostgresConnector(cfg, logger)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to PostgreSQL (single-tenant): %w", err)
	}

	// Run migrations on startup (single-tenant only; multi-tenant handles migrations via tenant-manager).
	if err := onboardingPostgresMigrator(cfg, logger); err != nil {
		return nil, fmt.Errorf("failed to run PostgreSQL migrations: %w", err)
	}

	return &onboardingPostgresComponents{
		connection:       conn,
		organizationRepo: organization.NewOrganizationPostgreSQLRepository(conn),
		ledgerRepo:       ledger.NewLedgerPostgreSQLRepository(conn),
		accountRepo:      account.NewAccountPostgreSQLRepository(conn),
		assetRepo:        asset.NewAssetPostgreSQLRepository(conn),
		portfolioRepo:    portfolio.NewPortfolioPostgreSQLRepository(conn),
		segmentRepo:      segment.NewSegmentPostgreSQLRepository(conn),
		accountTypeRepo:  accounttype.NewAccountTypePostgreSQLRepository(conn),
	}, nil
}

// onboardingPostgresConnector builds and connects a PostgresConnection for onboarding.
// Package-level variable to allow test injection of pre-connected connections without a live database.
var onboardingPostgresConnector = defaultOnboardingPostgresConnector

// defaultOnboardingPostgresConnector builds a PostgresConnection from Config and establishes the connection.
func defaultOnboardingPostgresConnector(cfg *Config, logger libLog.Logger) (*libPostgres.Client, error) {
	conn, err := buildOnboardingPostgresConnection(cfg, logger)
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := conn.Connect(ctx); err != nil {
		return nil, err
	}

	return conn, nil
}

// buildOnboardingPostgresConnection creates a PostgresConnection for the onboarding domain
// using DB_ONBOARDING_* env vars.
func buildOnboardingPostgresConnection(cfg *Config, logger libLog.Logger) (*libPostgres.Client, error) {
	postgreSourcePrimary := fmt.Sprintf("host=%s user=%s password=%s dbname=%s port=%s sslmode=%s",
		cfg.OnbPrefixedPrimaryDBHost, cfg.OnbPrefixedPrimaryDBUser, cfg.OnbPrefixedPrimaryDBPassword,
		cfg.OnbPrefixedPrimaryDBName, cfg.OnbPrefixedPrimaryDBPort, cfg.OnbPrefixedPrimaryDBSSLMode)

	postgreSourceReplica := fmt.Sprintf("host=%s user=%s password=%s dbname=%s port=%s sslmode=%s",
		cfg.OnbPrefixedReplicaDBHost, cfg.OnbPrefixedReplicaDBUser, cfg.OnbPrefixedReplicaDBPassword,
		cfg.OnbPrefixedReplicaDBName, cfg.OnbPrefixedReplicaDBPort, cfg.OnbPrefixedReplicaDBSSLMode)

	conn, err := libPostgres.New(libPostgres.Config{
		PrimaryDSN:         postgreSourcePrimary,
		ReplicaDSN:         postgreSourceReplica,
		Logger:             logger,
		MaxOpenConnections: cfg.OnbPrefixedMaxOpenConnections,
		MaxIdleConnections: cfg.OnbPrefixedMaxIdleConnections,
	})
	if err != nil {
		return nil, err
	}

	return conn, nil
}

// onboardingPostgresMigrator runs database migrations for onboarding. Package-level variable
// to allow test injection without requiring a live database.
var onboardingPostgresMigrator = defaultOnboardingPostgresMigrator

// defaultOnboardingPostgresMigrator executes database migrations for onboarding.
func defaultOnboardingPostgresMigrator(cfg *Config, logger libLog.Logger) error {
	primaryDSN := fmt.Sprintf("host=%s user=%s password=%s dbname=%s port=%s sslmode=%s",
		cfg.OnbPrefixedPrimaryDBHost, cfg.OnbPrefixedPrimaryDBUser, cfg.OnbPrefixedPrimaryDBPassword,
		cfg.OnbPrefixedPrimaryDBName, cfg.OnbPrefixedPrimaryDBPort, cfg.OnbPrefixedPrimaryDBSSLMode)

	migrator, err := libPostgres.NewMigrator(libPostgres.MigrationConfig{
		PrimaryDSN:     primaryDSN,
		DatabaseName:   cfg.OnbPrefixedPrimaryDBName,
		Component:      "ledger",
		MigrationsPath: "components/ledger/migrations/onboarding",
		Logger:         logger,
	})
	if err != nil {
		return fmt.Errorf("failed to create migrator: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	return migrator.Up(ctx)
}
