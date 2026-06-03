// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

//go:build integration

package in

// This file is the shared harness for the P4 third-rail fee proof suite. The
// proof classes themselves live in transaction_fee_proof_integration_test.go
// (T16), transaction_fee_revert_integration_test.go (T14), and
// transaction_fee_async_integration_test.go (T25). The harness wires a
// fee-enabled TransactionHandler against real Postgres + Mongo + Redis (and, for
// the async file, RabbitMQ) by reusing the production composition: the same
// command/query/fees use cases the unified ledger bootstrap builds at
// config.go:798 (transactionHandler := &TransactionHandler{Command, Query,
// FeeApplier: fees.useCase}).

import (
	"context"
	"database/sql"
	"testing"

	libLog "github.com/LerianStudio/lib-observability/log"

	mongoonb "github.com/LerianStudio/midaz/v3/components/ledger/internal/adapters/mongodb/onboarding"
	mongotxn "github.com/LerianStudio/midaz/v3/components/ledger/internal/adapters/mongodb/transaction"
	"github.com/LerianStudio/midaz/v3/components/ledger/internal/adapters/postgres/account"
	"github.com/LerianStudio/midaz/v3/components/ledger/internal/adapters/postgres/asset"
	"github.com/LerianStudio/midaz/v3/components/ledger/internal/adapters/postgres/balance"
	"github.com/LerianStudio/midaz/v3/components/ledger/internal/adapters/postgres/ledger"
	"github.com/LerianStudio/midaz/v3/components/ledger/internal/adapters/postgres/operation"
	"github.com/LerianStudio/midaz/v3/components/ledger/internal/adapters/postgres/organization"
	"github.com/LerianStudio/midaz/v3/components/ledger/internal/adapters/postgres/portfolio"
	"github.com/LerianStudio/midaz/v3/components/ledger/internal/adapters/postgres/segment"
	"github.com/LerianStudio/midaz/v3/components/ledger/internal/adapters/postgres/transaction"
	redis "github.com/LerianStudio/midaz/v3/components/ledger/internal/adapters/redis/transaction"
	"github.com/LerianStudio/midaz/v3/components/ledger/internal/services/command"
	"github.com/LerianStudio/midaz/v3/components/ledger/internal/services/query"

	feesmongo "github.com/LerianStudio/midaz/v3/components/ledger/internal/adapters/mongodb/fees"
	"github.com/LerianStudio/midaz/v3/components/ledger/internal/adapters/mongodb/fees/pack"
	feesservices "github.com/LerianStudio/midaz/v3/components/ledger/internal/services/fees"

	libPostgres "github.com/LerianStudio/lib-commons/v5/commons/postgres"
	mongotestutil "github.com/LerianStudio/midaz/v3/tests/utils/mongodb"
	postgrestestutil "github.com/LerianStudio/midaz/v3/tests/utils/postgres"
	redistestutil "github.com/LerianStudio/midaz/v3/tests/utils/redis"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

// feeHarness holds a fully-wired fee-enabled transaction stack backed by real
// containers, with the org/ledger seeded into the onboarding schema so the fee
// engine's in-process query-layer resolver works exactly as in production.
type feeHarness struct {
	pgContainer    *postgrestestutil.ContainerResult
	mongoContainer *mongotestutil.ContainerResult
	redisContainer *redistestutil.ContainerResult

	pgConn      *libPostgres.Client
	db          *sql.DB
	redisRepo   redis.RedisRepository
	metaRepo    mongotxn.Repository
	packageRepo pack.Repository

	commandUC *command.UseCase
	queryUC   *query.UseCase
	feeUC     *feesservices.UseCase
	handler   *TransactionHandler

	orgID    uuid.UUID
	ledgerID uuid.UUID
}

// setupFeeHarness builds the fee-enabled stack. It mirrors the production
// composition root: a single command.UseCase + query.UseCase share the
// onboarding + transaction Postgres repos, the transaction Mongo metadata repo,
// and the Redis repo; the fee use case is built from the fee Mongo package repo
// and an in-process MidazResolver over the same query.UseCase, and injected as
// the handler's FeeApplier — the seam exercised by executeCreateTransaction.
//
// RabbitMQ is intentionally absent: the default sync path persists inline, which
// is what every proof class except the T25 async file needs. The async file
// builds its own RabbitMQ-backed variant.
func setupFeeHarness(t *testing.T) *feeHarness {
	t.Helper()

	t.Setenv("RABBITMQ_TRANSACTION_EVENTS_ENABLED", "false")
	t.Setenv("AUDIT_LOG_ENABLED", "false")
	t.Setenv("RABBITMQ_TRANSACTION_ASYNC", "false")

	h := &feeHarness{}

	h.pgContainer = postgrestestutil.SetupContainer(t)
	h.mongoContainer = mongotestutil.SetupContainer(t)
	h.redisContainer = redistestutil.SetupContainer(t)
	h.db = h.pgContainer.DB

	// Transaction schema via golang-migrate (owns schema_migrations).
	migrationsPath := postgrestestutil.FindMigrationsPath(t, "transaction")
	connStr := postgrestestutil.BuildConnectionString(h.pgContainer.Host, h.pgContainer.Port, h.pgContainer.Config)
	h.pgConn = postgrestestutil.CreatePostgresClient(t, connStr, connStr, h.pgContainer.Config.DBName, migrationsPath)

	// Onboarding schema applied directly (disjoint tables; IF NOT EXISTS).
	postgrestestutil.ApplyOnboardingSchema(t, h.db)

	mongoTxnConn := mongotestutil.CreateConnection(t, h.mongoContainer.URI, "test_db")
	redisConn := redistestutil.CreateConnection(t, h.redisContainer.Addr)

	// Transaction-domain repos.
	transactionRepo := transaction.NewTransactionPostgreSQLRepository(h.pgConn)
	operationRepo := operation.NewOperationPostgreSQLRepository(h.pgConn)
	balanceRepo := balance.NewBalancePostgreSQLRepository(h.pgConn)
	h.metaRepo = mongotxn.NewMetadataMongoDBRepository(mongoTxnConn)

	redisRepo, err := redis.NewConsumerRedis(redisConn)
	require.NoError(t, err, "redis repo")
	h.redisRepo = redisRepo

	// Onboarding-domain repos (needed by the fee resolver's account/segment reads
	// and by GetParsedLedgerSettings on the create funnel).
	orgRepo := organization.NewOrganizationPostgreSQLRepository(h.pgConn)
	ledgerRepo := ledger.NewLedgerPostgreSQLRepository(h.pgConn)
	assetRepo := asset.NewAssetPostgreSQLRepository(h.pgConn)
	accountRepo := account.NewAccountPostgreSQLRepository(h.pgConn)
	portfolioRepo := portfolio.NewPortfolioPostgreSQLRepository(h.pgConn)
	segmentRepo := segment.NewSegmentPostgreSQLRepository(h.pgConn)
	onbMetaRepo := mongoonb.NewMetadataMongoDBRepository(mongoTxnConn)

	h.queryUC = &query.UseCase{
		OrganizationRepo:        orgRepo,
		LedgerRepo:              ledgerRepo,
		AssetRepo:               assetRepo,
		AccountRepo:             accountRepo,
		PortfolioRepo:           portfolioRepo,
		SegmentRepo:             segmentRepo,
		OnboardingMetadataRepo:  onbMetaRepo,
		TransactionRepo:         transactionRepo,
		OperationRepo:           operationRepo,
		BalanceRepo:             balanceRepo,
		TransactionMetadataRepo: h.metaRepo,
		TransactionRedisRepo:    redisRepo,
	}
	h.commandUC = &command.UseCase{
		OrganizationRepo:        orgRepo,
		LedgerRepo:              ledgerRepo,
		AssetRepo:               assetRepo,
		AccountRepo:             accountRepo,
		PortfolioRepo:           portfolioRepo,
		SegmentRepo:             segmentRepo,
		OnboardingMetadataRepo:  onbMetaRepo,
		TransactionRepo:         transactionRepo,
		OperationRepo:           operationRepo,
		BalanceRepo:             balanceRepo,
		TransactionMetadataRepo: h.metaRepo,
		TransactionRedisRepo:    redisRepo,
	}

	// Fee Mongo: inject the already-connected container client so the repo's
	// GetDB + EnsureIndexes run against real Mongo without re-dialing.
	logger := &libLog.GoLogger{}
	feeConn := &feesmongo.MongoConnection{
		ConnectionStringSource: h.mongoContainer.URI,
		Database:               "test_db",
		MaxPoolSize:            1,
		DB:                     h.mongoContainer.Client,
	}
	packageRepo, err := pack.NewPackageMongoDBRepository(feeConn, logger)
	require.NoError(t, err, "fee package repo")
	h.packageRepo = packageRepo

	resolver, err := feesservices.NewQueryResolver(h.queryUC)
	require.NoError(t, err, "fee resolver")
	h.feeUC, err = feesservices.NewUseCase(packageRepo, resolver, "USD")
	require.NoError(t, err, "fee use case")

	h.handler = &TransactionHandler{Query: h.queryUC, Command: h.commandUC, FeeApplier: h.feeUC}

	// Seed a real organization + ledger so GetParsedLedgerSettings succeeds and
	// the fee resolver resolves accounts against a real ledger.
	h.orgID = postgrestestutil.CreateTestOrganization(t, h.db)
	h.ledgerID = postgrestestutil.CreateTestLedger(t, h.db, h.orgID)

	return h
}

// dropFeePrecisionTable is a no-op assertion that the ISO-4217 precision table
// (asset_precision) does not exist in this schema, proving proof class 3 runs
// "with the precision table deleted" (P4-T11). The fee engine emits unrounded
// legs and reconciles residuals onto the max account; no precision table is
// consulted. We assert its absence so a future reintroduction is caught.
func (h *feeHarness) assertNoPrecisionTable(t *testing.T) {
	t.Helper()

	var exists bool
	err := h.db.QueryRow(`SELECT EXISTS (SELECT 1 FROM information_schema.tables WHERE table_name = 'asset_precision')`).Scan(&exists)
	require.NoError(t, err, "query for asset_precision table")
	require.False(t, exists, "the ISO-4217 asset_precision table must NOT exist (P4-T11 deleted it); residual reconciliation alone holds the balance")
}

// ctx returns a background context for fixture seeding outside the request path.
func (h *feeHarness) ctx() context.Context { return context.Background() }
