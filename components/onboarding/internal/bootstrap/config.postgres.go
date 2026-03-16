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
	"github.com/LerianStudio/midaz/v3/components/onboarding/internal/adapters/postgres/account"
	"github.com/LerianStudio/midaz/v3/components/onboarding/internal/adapters/postgres/accounttype"
	"github.com/LerianStudio/midaz/v3/components/onboarding/internal/adapters/postgres/asset"
	"github.com/LerianStudio/midaz/v3/components/onboarding/internal/adapters/postgres/ledger"
	"github.com/LerianStudio/midaz/v3/components/onboarding/internal/adapters/postgres/organization"
	"github.com/LerianStudio/midaz/v3/components/onboarding/internal/adapters/postgres/portfolio"
	"github.com/LerianStudio/midaz/v3/components/onboarding/internal/adapters/postgres/segment"
)

// postgresComponents holds PostgreSQL-related components initialized during bootstrap.
type postgresComponents struct {
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

// initPostgres initializes PostgreSQL components.
// Dispatches to single-tenant or multi-tenant initialization based on Options.
func initPostgres(opts *Options, cfg *Config, logger libLog.Logger) (*postgresComponents, error) {
	if opts != nil && opts.MultiTenantEnabled {
		return initMultiTenantPostgres(opts, cfg, logger)
	}

	return initSingleTenantPostgres(cfg, logger)
}

// initMultiTenantPostgres initializes PostgreSQL in multi-tenant mode.
func initMultiTenantPostgres(opts *Options, cfg *Config, logger libLog.Logger) (*postgresComponents, error) {
	logger.Log(context.Background(), libLog.LevelInfo, "Initializing multi-tenant PostgreSQL for onboarding")

	if opts.TenantClient == nil {
		return nil, fmt.Errorf("TenantClient is required for multi-tenant PostgreSQL initialization")
	}

	pgMgr := tmpostgres.NewManager(
		opts.TenantClient,
		opts.TenantServiceName,
		tmpostgres.WithModule(ApplicationName),
		tmpostgres.WithLogger(logger),
	)

	// Build and connect. In multi-tenant mode, this connection serves as
	// fallback/placeholder; actual per-tenant connections are resolved by middleware.
	conn, err := postgresConnector(cfg, logger)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to PostgreSQL (multi-tenant): %w", err)
	}

	return &postgresComponents{
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

// initSingleTenantPostgres initializes PostgreSQL in single-tenant mode.
func initSingleTenantPostgres(cfg *Config, logger libLog.Logger) (*postgresComponents, error) {
	conn, err := postgresConnector(cfg, logger)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to PostgreSQL (single-tenant): %w", err)
	}

	// Run migrations on startup (single-tenant only; multi-tenant handles migrations via tenant-manager).
	if err := postgresMigrator(cfg, logger); err != nil {
		return nil, fmt.Errorf("failed to run PostgreSQL migrations: %w", err)
	}

	return &postgresComponents{
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

// postgresConnector builds and connects a PostgresConnection. Package-level variable
// to allow test injection of pre-connected connections without a live database.
var postgresConnector = defaultPostgresConnector

// defaultPostgresConnector builds a PostgresConnection from Config and establishes the connection.
func defaultPostgresConnector(cfg *Config, logger libLog.Logger) (*libPostgres.Client, error) {
	conn, err := buildPostgresConnection(cfg, logger)
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

// buildPostgresConnection creates a PostgresConnection from Config using prefixed env var fallback.
func buildPostgresConnection(cfg *Config, logger libLog.Logger) (*libPostgres.Client, error) {
	dbHost := envFallback(cfg.PrefixedPrimaryDBHost, cfg.PrimaryDBHost)
	dbUser := envFallback(cfg.PrefixedPrimaryDBUser, cfg.PrimaryDBUser)
	dbPassword := envFallback(cfg.PrefixedPrimaryDBPassword, cfg.PrimaryDBPassword)
	dbName := envFallback(cfg.PrefixedPrimaryDBName, cfg.PrimaryDBName)
	dbPort := envFallback(cfg.PrefixedPrimaryDBPort, cfg.PrimaryDBPort)
	dbSSLMode := envFallback(cfg.PrefixedPrimaryDBSSLMode, cfg.PrimaryDBSSLMode)

	dbReplicaHost := envFallback(cfg.PrefixedReplicaDBHost, cfg.ReplicaDBHost)
	dbReplicaUser := envFallback(cfg.PrefixedReplicaDBUser, cfg.ReplicaDBUser)
	dbReplicaPassword := envFallback(cfg.PrefixedReplicaDBPassword, cfg.ReplicaDBPassword)
	dbReplicaName := envFallback(cfg.PrefixedReplicaDBName, cfg.ReplicaDBName)
	dbReplicaPort := envFallback(cfg.PrefixedReplicaDBPort, cfg.ReplicaDBPort)
	dbReplicaSSLMode := envFallback(cfg.PrefixedReplicaDBSSLMode, cfg.ReplicaDBSSLMode)

	maxOpenConns := envFallbackInt(cfg.PrefixedMaxOpenConnections, cfg.MaxOpenConnections)
	maxIdleConns := envFallbackInt(cfg.PrefixedMaxIdleConnections, cfg.MaxIdleConnections)

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

// postgresMigrator runs database migrations. Package-level variable to allow
// test injection without requiring a live database.
var postgresMigrator = defaultPostgresMigrator

// defaultPostgresMigrator executes database migrations using the v4 explicit Migrator API.
func defaultPostgresMigrator(cfg *Config, logger libLog.Logger) error {
	dbHost := envFallback(cfg.PrefixedPrimaryDBHost, cfg.PrimaryDBHost)
	dbUser := envFallback(cfg.PrefixedPrimaryDBUser, cfg.PrimaryDBUser)
	dbPassword := envFallback(cfg.PrefixedPrimaryDBPassword, cfg.PrimaryDBPassword)
	dbName := envFallback(cfg.PrefixedPrimaryDBName, cfg.PrimaryDBName)
	dbPort := envFallback(cfg.PrefixedPrimaryDBPort, cfg.PrimaryDBPort)
	dbSSLMode := envFallback(cfg.PrefixedPrimaryDBSSLMode, cfg.PrimaryDBSSLMode)

	primaryDSN := fmt.Sprintf("host=%s user=%s password=%s dbname=%s port=%s sslmode=%s",
		dbHost, dbUser, dbPassword, dbName, dbPort, dbSSLMode)

	migrator, err := libPostgres.NewMigrator(libPostgres.MigrationConfig{
		PrimaryDSN:   primaryDSN,
		DatabaseName: dbName,
		Component:    ApplicationName,
		Logger:       logger,
	})
	if err != nil {
		return fmt.Errorf("failed to create migrator: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	return migrator.Up(ctx)
}
