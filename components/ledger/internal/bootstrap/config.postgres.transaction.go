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
	"github.com/LerianStudio/midaz/v3/components/ledger/internal/adapters/postgres/assetrate"
	"github.com/LerianStudio/midaz/v3/components/ledger/internal/adapters/postgres/balance"
	"github.com/LerianStudio/midaz/v3/components/ledger/internal/adapters/postgres/operation"
	"github.com/LerianStudio/midaz/v3/components/ledger/internal/adapters/postgres/operationroute"
	"github.com/LerianStudio/midaz/v3/components/ledger/internal/adapters/postgres/transaction"
	"github.com/LerianStudio/midaz/v3/components/ledger/internal/adapters/postgres/transactionroute"
)

// transactionPostgresComponents holds PostgreSQL-related components for the transaction domain.
type transactionPostgresComponents struct {
	connection           *libPostgres.Client
	pgManager            *tmpostgres.Manager // nil in single-tenant mode; reserved for MultiPoolMiddleware wiring
	transactionRepo      *transaction.TransactionPostgreSQLRepository
	operationRepo        *operation.OperationPostgreSQLRepository
	assetRateRepo        *assetrate.AssetRatePostgreSQLRepository
	balanceRepo          *balance.BalancePostgreSQLRepository
	operationRouteRepo   *operationroute.OperationRoutePostgreSQLRepository
	transactionRouteRepo *transactionroute.TransactionRoutePostgreSQLRepository
}

// initTransactionPostgres initializes PostgreSQL components for the transaction domain.
// Dispatches to single-tenant or multi-tenant initialization based on Options.
func initTransactionPostgres(opts *Options, cfg *Config, logger libLog.Logger) (*transactionPostgresComponents, error) {
	if opts != nil && opts.MultiTenantEnabled {
		return initTransactionMultiTenantPostgres(opts, cfg, logger)
	}

	return initTransactionSingleTenantPostgres(cfg, logger)
}

// initTransactionMultiTenantPostgres initializes PostgreSQL in multi-tenant mode for transaction.
func initTransactionMultiTenantPostgres(opts *Options, cfg *Config, logger libLog.Logger) (*transactionPostgresComponents, error) {
	logger.Log(context.Background(), libLog.LevelInfo, "Initializing multi-tenant PostgreSQL for transaction")

	if opts.TenantClient == nil {
		return nil, fmt.Errorf("TenantClient is required for multi-tenant PostgreSQL initialization")
	}

	pgOpts := []tmpostgres.Option{
		tmpostgres.WithModule("transaction"),
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
	conn, err := transactionPostgresConnector(cfg, logger)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to PostgreSQL (multi-tenant): %w", err)
	}

	return &transactionPostgresComponents{
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

// initTransactionSingleTenantPostgres initializes PostgreSQL in single-tenant mode for transaction.
func initTransactionSingleTenantPostgres(cfg *Config, logger libLog.Logger) (*transactionPostgresComponents, error) {
	conn, err := transactionPostgresConnector(cfg, logger)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to PostgreSQL (single-tenant): %w", err)
	}

	// Run migrations on startup (single-tenant only; multi-tenant handles migrations via tenant-manager).
	if err := transactionPostgresMigrator(cfg, logger); err != nil {
		return nil, fmt.Errorf("failed to run PostgreSQL migrations: %w", err)
	}

	return &transactionPostgresComponents{
		connection:           conn,
		transactionRepo:      transaction.NewTransactionPostgreSQLRepository(conn),
		operationRepo:        operation.NewOperationPostgreSQLRepository(conn),
		assetRateRepo:        assetrate.NewAssetRatePostgreSQLRepository(conn),
		balanceRepo:          balance.NewBalancePostgreSQLRepository(conn),
		operationRouteRepo:   operationroute.NewOperationRoutePostgreSQLRepository(conn),
		transactionRouteRepo: transactionroute.NewTransactionRoutePostgreSQLRepository(conn),
	}, nil
}

// transactionPostgresConnector builds and connects a PostgresConnection for transaction.
// Package-level variable to allow test injection of pre-connected connections without a live database.
var transactionPostgresConnector = defaultTransactionPostgresConnector

// defaultTransactionPostgresConnector builds a PostgresConnection from Config and establishes the connection.
func defaultTransactionPostgresConnector(cfg *Config, logger libLog.Logger) (*libPostgres.Client, error) {
	conn, err := buildTransactionPostgresConnection(cfg, logger)
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

// buildTransactionPostgresConnection creates a PostgresConnection for the transaction domain
// using DB_TRANSACTION_* env vars.
func buildTransactionPostgresConnection(cfg *Config, logger libLog.Logger) (*libPostgres.Client, error) {
	postgreSourcePrimary := fmt.Sprintf("host=%s user=%s password=%s dbname=%s port=%s sslmode=%s",
		cfg.TxnPrefixedPrimaryDBHost, cfg.TxnPrefixedPrimaryDBUser, cfg.TxnPrefixedPrimaryDBPassword,
		cfg.TxnPrefixedPrimaryDBName, cfg.TxnPrefixedPrimaryDBPort, cfg.TxnPrefixedPrimaryDBSSLMode)

	postgreSourceReplica := fmt.Sprintf("host=%s user=%s password=%s dbname=%s port=%s sslmode=%s",
		cfg.TxnPrefixedReplicaDBHost, cfg.TxnPrefixedReplicaDBUser, cfg.TxnPrefixedReplicaDBPassword,
		cfg.TxnPrefixedReplicaDBName, cfg.TxnPrefixedReplicaDBPort, cfg.TxnPrefixedReplicaDBSSLMode)

	conn, err := libPostgres.New(libPostgres.Config{
		PrimaryDSN:         postgreSourcePrimary,
		ReplicaDSN:         postgreSourceReplica,
		Logger:             logger,
		MaxOpenConnections: cfg.TxnPrefixedMaxOpenConnections,
		MaxIdleConnections: cfg.TxnPrefixedMaxIdleConnections,
	})
	if err != nil {
		return nil, err
	}

	return conn, nil
}

// transactionPostgresMigrator runs database migrations for transaction. Package-level variable
// to allow test injection without requiring a live database.
var transactionPostgresMigrator = defaultTransactionPostgresMigrator

// defaultTransactionPostgresMigrator executes database migrations for transaction.
func defaultTransactionPostgresMigrator(cfg *Config, logger libLog.Logger) error {
	primaryDSN := fmt.Sprintf("host=%s user=%s password=%s dbname=%s port=%s sslmode=%s",
		cfg.TxnPrefixedPrimaryDBHost, cfg.TxnPrefixedPrimaryDBUser, cfg.TxnPrefixedPrimaryDBPassword,
		cfg.TxnPrefixedPrimaryDBName, cfg.TxnPrefixedPrimaryDBPort, cfg.TxnPrefixedPrimaryDBSSLMode)

	migrator, err := libPostgres.NewMigrator(libPostgres.MigrationConfig{
		PrimaryDSN:     primaryDSN,
		DatabaseName:   cfg.TxnPrefixedPrimaryDBName,
		Component:      "ledger",
		MigrationsPath: "components/ledger/migrations/transaction",
		Logger:         logger,
	})
	if err != nil {
		return fmt.Errorf("failed to create migrator: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	return migrator.Up(ctx)
}
