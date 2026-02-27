// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"fmt"

	libLog "github.com/LerianStudio/lib-commons/v3/commons/log"
	libPostgres "github.com/LerianStudio/lib-commons/v3/commons/postgres"
	tmpostgres "github.com/LerianStudio/lib-commons/v3/commons/tenant-manager/postgres"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/postgres/assetrate"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/postgres/balance"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/postgres/operation"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/postgres/operationroute"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/postgres/transaction"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/postgres/transactionroute"
	"github.com/LerianStudio/midaz/v3/pkg/utils"
)

// postgresComponents holds PostgreSQL-related components initialized during bootstrap.
type postgresComponents struct {
	connection           *libPostgres.PostgresConnection
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
	logger.Info("Initializing multi-tenant PostgreSQL for transaction")

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
		connection:           conn,
		pgManager:            pgMgr,
		transactionRepo:      transaction.NewTransactionPostgreSQLRepository(conn),
		operationRepo:        operation.NewOperationPostgreSQLRepository(conn),
		assetRateRepo:        assetrate.NewAssetRatePostgreSQLRepository(conn),
		balanceRepo:          balance.NewBalancePostgreSQLRepository(conn),
		operationRouteRepo:   operationroute.NewOperationRoutePostgreSQLRepository(conn),
		transactionRouteRepo: transactionroute.NewTransactionRoutePostgreSQLRepository(conn),
	}, nil
}

// initSingleTenantPostgres initializes PostgreSQL in single-tenant mode.
func initSingleTenantPostgres(cfg *Config, logger libLog.Logger) (*postgresComponents, error) {
	conn, err := postgresConnector(cfg, logger)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to PostgreSQL (single-tenant): %w", err)
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
func defaultPostgresConnector(cfg *Config, logger libLog.Logger) (*libPostgres.PostgresConnection, error) {
	conn := buildPostgresConnection(cfg, logger)

	if err := conn.Connect(); err != nil {
		return nil, err
	}

	return conn, nil
}

// buildPostgresConnection creates a PostgresConnection from Config using prefixed env var fallback.
func buildPostgresConnection(cfg *Config, logger libLog.Logger) *libPostgres.PostgresConnection {
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

	return &libPostgres.PostgresConnection{
		ConnectionStringPrimary: postgreSourcePrimary,
		ConnectionStringReplica: postgreSourceReplica,
		PrimaryDBName:           dbName,
		ReplicaDBName:           dbReplicaName,
		Component:               ApplicationName,
		Logger:                  logger,
		MaxOpenConnections:      maxOpenConns,
		MaxIdleConnections:      maxIdleConns,
	}
}
