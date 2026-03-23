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
	"github.com/LerianStudio/midaz/v3/components/ledger/adapters/postgres/assetrate"
	"github.com/LerianStudio/midaz/v3/components/ledger/adapters/postgres/balance"
	"github.com/LerianStudio/midaz/v3/components/ledger/adapters/postgres/operation"
	"github.com/LerianStudio/midaz/v3/components/ledger/adapters/postgres/operationroute"
	"github.com/LerianStudio/midaz/v3/components/ledger/adapters/postgres/transaction"
	"github.com/LerianStudio/midaz/v3/components/ledger/adapters/postgres/transactionroute"
	"github.com/LerianStudio/midaz/v3/pkg/utils"
)

// postgresComponents holds PostgreSQL-related components initialized during bootstrap.
type postgresComponents struct {
	connection           *libPostgres.Client
	pgManager            *tmpostgres.Manager // nil in single-tenant mode; reserved for MultiPoolMiddleware wiring
	transactionRepo      *transaction.TransactionPostgreSQLRepository
	operationRepo        *operation.OperationPostgreSQLRepository
	assetRateRepo        *assetrate.AssetRatePostgreSQLRepository
	balanceRepo          *balance.BalancePostgreSQLRepository
	operationRouteRepo   *operationroute.OperationRoutePostgreSQLRepository
	transactionRouteRepo *transactionroute.TransactionRoutePostgreSQLRepository
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
	logger.Log(context.Background(), libLog.LevelInfo, "Initializing multi-tenant PostgreSQL for transaction")

	if opts.TenantClient == nil {
		return nil, fmt.Errorf("TenantClient is required for multi-tenant PostgreSQL initialization")
	}

	pgOpts := []tmpostgres.Option{
		tmpostgres.WithModule(ApplicationName),
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
	conn, err := postgresConnector(cfg, logger)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to PostgreSQL (multi-tenant): %w", err)
	}

	return &postgresComponents{
		connection:           conn,
		pgManager:            pgMgr,
		transactionRepo:      transaction.NewTransactionPostgreSQLRepository(conn, true),
		operationRepo:        operation.NewOperationPostgreSQLRepository(conn, true),
		assetRateRepo:        assetrate.NewAssetRatePostgreSQLRepository(conn, true),
		balanceRepo:          balance.NewBalancePostgreSQLRepository(conn, true),
		operationRouteRepo:   operationroute.NewOperationRoutePostgreSQLRepository(conn, true),
		transactionRouteRepo: transactionroute.NewTransactionRoutePostgreSQLRepository(conn, true),
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
		connection:           conn,
		transactionRepo:      transaction.NewTransactionPostgreSQLRepository(conn),
		operationRepo:        operation.NewOperationPostgreSQLRepository(conn),
		assetRateRepo:        assetrate.NewAssetRatePostgreSQLRepository(conn),
		balanceRepo:          balance.NewBalancePostgreSQLRepository(conn),
		operationRouteRepo:   operationroute.NewOperationRoutePostgreSQLRepository(conn),
		transactionRouteRepo: transactionroute.NewTransactionRoutePostgreSQLRepository(conn),
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
	dbHost := utils.EnvFallback(cfg.PrefixedPrimaryDBHost, cfg.PrimaryDBHost)
	dbUser := utils.EnvFallback(cfg.PrefixedPrimaryDBUser, cfg.PrimaryDBUser)
	dbPassword := utils.EnvFallback(cfg.PrefixedPrimaryDBPassword, cfg.PrimaryDBPassword)
	dbName := utils.EnvFallback(cfg.PrefixedPrimaryDBName, cfg.PrimaryDBName)
	dbPort := utils.EnvFallback(cfg.PrefixedPrimaryDBPort, cfg.PrimaryDBPort)
	dbSSLMode := utils.EnvFallback(cfg.PrefixedPrimaryDBSSLMode, cfg.PrimaryDBSSLMode)

	dbReplicaHost := utils.EnvFallback(cfg.PrefixedReplicaDBHost, cfg.ReplicaDBHost)
	dbReplicaUser := utils.EnvFallback(cfg.PrefixedReplicaDBUser, cfg.ReplicaDBUser)
	dbReplicaPassword := utils.EnvFallback(cfg.PrefixedReplicaDBPassword, cfg.ReplicaDBPassword)
	dbReplicaName := utils.EnvFallback(cfg.PrefixedReplicaDBName, cfg.ReplicaDBName)
	dbReplicaPort := utils.EnvFallback(cfg.PrefixedReplicaDBPort, cfg.ReplicaDBPort)
	dbReplicaSSLMode := utils.EnvFallback(cfg.PrefixedReplicaDBSSLMode, cfg.ReplicaDBSSLMode)

	maxOpenConns := utils.EnvFallbackInt(cfg.PrefixedMaxOpenConnections, cfg.MaxOpenConnections)
	maxIdleConns := utils.EnvFallbackInt(cfg.PrefixedMaxIdleConnections, cfg.MaxIdleConnections)

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
	dbHost := utils.EnvFallback(cfg.PrefixedPrimaryDBHost, cfg.PrimaryDBHost)
	dbUser := utils.EnvFallback(cfg.PrefixedPrimaryDBUser, cfg.PrimaryDBUser)
	dbPassword := utils.EnvFallback(cfg.PrefixedPrimaryDBPassword, cfg.PrimaryDBPassword)
	dbName := utils.EnvFallback(cfg.PrefixedPrimaryDBName, cfg.PrimaryDBName)
	dbPort := utils.EnvFallback(cfg.PrefixedPrimaryDBPort, cfg.PrimaryDBPort)
	dbSSLMode := utils.EnvFallback(cfg.PrefixedPrimaryDBSSLMode, cfg.PrimaryDBSSLMode)

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
