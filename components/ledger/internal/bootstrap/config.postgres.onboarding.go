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
	"github.com/LerianStudio/midaz/v3/components/ledger/adapters/postgres/account"
	"github.com/LerianStudio/midaz/v3/components/ledger/adapters/postgres/accounttype"
	"github.com/LerianStudio/midaz/v3/components/ledger/adapters/postgres/asset"
	"github.com/LerianStudio/midaz/v3/components/ledger/adapters/postgres/ledger"
	"github.com/LerianStudio/midaz/v3/components/ledger/adapters/postgres/organization"
	"github.com/LerianStudio/midaz/v3/components/ledger/adapters/postgres/portfolio"
	"github.com/LerianStudio/midaz/v3/components/ledger/adapters/postgres/segment"
	"github.com/LerianStudio/midaz/v3/pkg/utils"
)

// onboardingPostgresComponents holds PostgreSQL-related components for the onboarding domain.
type onboardingPostgresComponents struct {
	connection       *libPostgres.Client
	pgManager        *tmpostgres.Manager // nil in single-tenant mode; reserved for MultiPoolMiddleware wiring
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
		tmpostgres.WithModule("onboarding"),
		tmpostgres.WithLogger(logger),
	}

	if cfg.MultiTenantSettingsCheckIntervalSec > 0 {
		pgOpts = append(pgOpts, tmpostgres.WithSettingsCheckInterval(time.Duration(cfg.MultiTenantSettingsCheckIntervalSec)*time.Second))
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
// using prefixed env var fallback (DB_ONBOARDING_* with DB_* fallback).
func buildOnboardingPostgresConnection(cfg *Config, logger libLog.Logger) (*libPostgres.Client, error) {
	dbHost := utils.EnvFallback(cfg.OnbPrefixedPrimaryDBHost, cfg.PrimaryDBHost)
	dbUser := utils.EnvFallback(cfg.OnbPrefixedPrimaryDBUser, cfg.PrimaryDBUser)
	dbPassword := utils.EnvFallback(cfg.OnbPrefixedPrimaryDBPassword, cfg.PrimaryDBPassword)
	dbName := utils.EnvFallback(cfg.OnbPrefixedPrimaryDBName, cfg.PrimaryDBName)
	dbPort := utils.EnvFallback(cfg.OnbPrefixedPrimaryDBPort, cfg.PrimaryDBPort)
	dbSSLMode := utils.EnvFallback(cfg.OnbPrefixedPrimaryDBSSLMode, cfg.PrimaryDBSSLMode)

	dbReplicaHost := utils.EnvFallback(cfg.OnbPrefixedReplicaDBHost, cfg.ReplicaDBHost)
	dbReplicaUser := utils.EnvFallback(cfg.OnbPrefixedReplicaDBUser, cfg.ReplicaDBUser)
	dbReplicaPassword := utils.EnvFallback(cfg.OnbPrefixedReplicaDBPassword, cfg.ReplicaDBPassword)
	dbReplicaName := utils.EnvFallback(cfg.OnbPrefixedReplicaDBName, cfg.ReplicaDBName)
	dbReplicaPort := utils.EnvFallback(cfg.OnbPrefixedReplicaDBPort, cfg.ReplicaDBPort)
	dbReplicaSSLMode := utils.EnvFallback(cfg.OnbPrefixedReplicaDBSSLMode, cfg.ReplicaDBSSLMode)

	maxOpenConns := utils.EnvFallbackInt(cfg.OnbPrefixedMaxOpenConnections, cfg.MaxOpenConnections)
	maxIdleConns := utils.EnvFallbackInt(cfg.OnbPrefixedMaxIdleConnections, cfg.MaxIdleConnections)

	postgreSourcePrimary := fmt.Sprintf("host=%s user=%s password=%s dbname=%s port=%s sslmode=%s",
		dbHost, dbUser, dbPassword, dbName, dbPort, dbSSLMode)

	postgreSourceReplica := fmt.Sprintf("host=%s user=%s password=%s dbname=%s port=%s sslmode=%s",
		dbReplicaHost, dbReplicaUser, dbReplicaPassword, dbReplicaName, dbReplicaPort, dbReplicaSSLMode)

	conn, err := libPostgres.New(libPostgres.Config{
		PrimaryDSN:         postgreSourcePrimary,
		ReplicaDSN:         postgreSourceReplica,
		Logger:             logger,
		MaxOpenConnections: maxOpenConns,
		MaxIdleConnections: maxIdleConns,
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
	dbHost := utils.EnvFallback(cfg.OnbPrefixedPrimaryDBHost, cfg.PrimaryDBHost)
	dbUser := utils.EnvFallback(cfg.OnbPrefixedPrimaryDBUser, cfg.PrimaryDBUser)
	dbPassword := utils.EnvFallback(cfg.OnbPrefixedPrimaryDBPassword, cfg.PrimaryDBPassword)
	dbName := utils.EnvFallback(cfg.OnbPrefixedPrimaryDBName, cfg.PrimaryDBName)
	dbPort := utils.EnvFallback(cfg.OnbPrefixedPrimaryDBPort, cfg.PrimaryDBPort)
	dbSSLMode := utils.EnvFallback(cfg.OnbPrefixedPrimaryDBSSLMode, cfg.PrimaryDBSSLMode)

	primaryDSN := fmt.Sprintf("host=%s user=%s password=%s dbname=%s port=%s sslmode=%s",
		dbHost, dbUser, dbPassword, dbName, dbPort, dbSSLMode)

	migrator, err := libPostgres.NewMigrator(libPostgres.MigrationConfig{
		PrimaryDSN:   primaryDSN,
		DatabaseName: dbName,
		Component:    "onboarding",
		Logger:       logger,
	})
	if err != nil {
		return fmt.Errorf("failed to create migrator: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	return migrator.Up(ctx)
}
