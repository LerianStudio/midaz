// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

//go:build integration

package in

import (
	"context"
	"encoding/json"
	"os"
	"testing"
	"time"

	libLog "github.com/LerianStudio/lib-observability/log"
	libOpentelemetry "github.com/LerianStudio/lib-observability/tracing"
	libRabbitmq "github.com/LerianStudio/lib-commons/v5/commons/rabbitmq"

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
	"github.com/LerianStudio/midaz/v3/components/ledger/internal/adapters/rabbitmq"
	redis "github.com/LerianStudio/midaz/v3/components/ledger/internal/adapters/redis/transaction"
	"github.com/LerianStudio/midaz/v3/components/ledger/internal/services/command"
	"github.com/LerianStudio/midaz/v3/components/ledger/internal/services/query"

	feesmongo "github.com/LerianStudio/midaz/v3/components/ledger/internal/adapters/mongodb/fees"
	"github.com/LerianStudio/midaz/v3/components/ledger/internal/adapters/mongodb/fees/pack"
	feesservices "github.com/LerianStudio/midaz/v3/components/ledger/internal/services/fees"

	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/LerianStudio/midaz/v3/pkg/mtransaction"
	postgrestestutil "github.com/LerianStudio/midaz/v3/tests/utils/postgres"
	rabbitmqtestutil "github.com/LerianStudio/midaz/v3/tests/utils/rabbitmq"

	mongotestutil "github.com/LerianStudio/midaz/v3/tests/utils/mongodb"
	redistestutil "github.com/LerianStudio/midaz/v3/tests/utils/redis"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vmihailenco/msgpack/v5"
)

// asyncFeeConsumer mirrors bootstrap.MultiQueueConsumer.handlerBTOQueue but is
// test-local to avoid an import cycle. It drives the same
// CreateBalanceTransactionOperationsAsync the production worker runs.
type asyncFeeConsumer struct {
	routes  *rabbitmq.ConsumerRoutes
	useCase *command.UseCase
}

func (c *asyncFeeConsumer) handle(ctx context.Context, body []byte) error {
	var message mmodel.Queue
	if err := msgpack.Unmarshal(body, &message); err != nil {
		return err
	}
	return c.useCase.CreateBalanceTransactionOperationsAsync(ctx, message)
}

// TestFeeProof_T25_AsyncFeeInclusive is the P4-T25 behavioral async gate: with
// RABBITMQ_TRANSACTION_ASYNC=true, a fee-bearing transaction queued via
// Redis/RabbitMQ persists operations that include the fee legs and balances
// (sum == 0); and the crash-recovery backup seed reconstructs to the
// FEE-INCLUSIVE transaction (not the pre-fee payload).
func TestFeeProof_T25_AsyncFeeInclusive(t *testing.T) {
	t.Setenv("AUDIT_LOG_ENABLED", "false")
	t.Setenv("RABBITMQ_TRANSACTION_EVENTS_ENABLED", "false")
	t.Setenv("RABBITMQ_TRANSACTION_ASYNC", "true")
	t.Setenv("RABBITMQ_TRANSACTION_BALANCE_OPERATION_EXCHANGE", "test.fee.exchange")
	t.Setenv("RABBITMQ_TRANSACTION_BALANCE_OPERATION_KEY", "test.fee.key")
	t.Setenv("RABBITMQ_TRANSACTION_BALANCE_OPERATION_QUEUE", "test.fee.queue")

	pgContainer := postgrestestutil.SetupContainer(t)
	mongoContainer := mongotestutil.SetupContainer(t)
	redisContainer := redistestutil.SetupContainer(t)
	rabbitContainer := rabbitmqtestutil.SetupContainer(t)

	rabbitHealthURL := "http://" + rabbitContainer.Host + ":" + rabbitContainer.MgmtPort
	t.Setenv("RABBITMQ_HEALTH_CHECK_URL", rabbitHealthURL)

	rabbitmqtestutil.SetupExchange(t, rabbitContainer.Channel, "test.fee.exchange", "direct")
	rabbitmqtestutil.SetupQueue(t, rabbitContainer.Channel, "test.fee.queue", "test.fee.exchange", "test.fee.key")

	migrationsPath := postgrestestutil.FindMigrationsPath(t, "transaction")
	connStr := postgrestestutil.BuildConnectionString(pgContainer.Host, pgContainer.Port, pgContainer.Config)
	pgConn := postgrestestutil.CreatePostgresClient(t, connStr, connStr, pgContainer.Config.DBName, migrationsPath)
	postgrestestutil.ApplyOnboardingSchema(t, pgContainer.DB)

	mongoConn := mongotestutil.CreateConnection(t, mongoContainer.URI, "test_db")
	redisConn := redistestutil.CreateConnection(t, redisContainer.Addr)
	logger := &libLog.GoLogger{Level: libLog.LevelInfo}

	transactionRepo := transaction.NewTransactionPostgreSQLRepository(pgConn)
	operationRepo := operation.NewOperationPostgreSQLRepository(pgConn)
	balanceRepo := balance.NewBalancePostgreSQLRepository(pgConn)
	metaRepo := mongotxn.NewMetadataMongoDBRepository(mongoConn)
	onbMetaRepo := mongoonb.NewMetadataMongoDBRepository(mongoConn)
	redisRepo, err := redis.NewConsumerRedis(redisConn)
	require.NoError(t, err)

	orgRepo := organization.NewOrganizationPostgreSQLRepository(pgConn)
	ledgerRepo := ledger.NewLedgerPostgreSQLRepository(pgConn)
	assetRepo := asset.NewAssetPostgreSQLRepository(pgConn)
	accountRepo := account.NewAccountPostgreSQLRepository(pgConn)
	portfolioRepo := portfolio.NewPortfolioPostgreSQLRepository(pgConn)
	segmentRepo := segment.NewSegmentPostgreSQLRepository(pgConn)

	rabbitConn := &libRabbitmq.RabbitMQConnection{
		ConnectionStringSource: rabbitContainer.URI,
		HealthCheckURL:         rabbitHealthURL,
		Host:                   rabbitContainer.Host,
		Port:                   rabbitContainer.AMQPPort,
		User:                   rabbitmqtestutil.DefaultUser,
		Pass:                   rabbitmqtestutil.DefaultPassword,
		Logger:                 logger,
	}
	producerRepo, err := rabbitmq.NewProducerRabbitMQ(rabbitConn)
	require.NoError(t, err)

	queryUC := &query.UseCase{
		OrganizationRepo: orgRepo, LedgerRepo: ledgerRepo, AssetRepo: assetRepo,
		AccountRepo: accountRepo, PortfolioRepo: portfolioRepo, SegmentRepo: segmentRepo,
		OnboardingMetadataRepo: onbMetaRepo,
		TransactionRepo:        transactionRepo, OperationRepo: operationRepo, BalanceRepo: balanceRepo,
		TransactionMetadataRepo: metaRepo, TransactionRedisRepo: redisRepo,
	}
	commandUC := &command.UseCase{
		OrganizationRepo: orgRepo, LedgerRepo: ledgerRepo, AssetRepo: assetRepo,
		AccountRepo: accountRepo, PortfolioRepo: portfolioRepo, SegmentRepo: segmentRepo,
		OnboardingMetadataRepo: onbMetaRepo,
		TransactionRepo:        transactionRepo, OperationRepo: operationRepo, BalanceRepo: balanceRepo,
		TransactionMetadataRepo: metaRepo, TransactionRedisRepo: redisRepo, RabbitMQRepo: producerRepo,
	}

	feeConn := &feesmongo.MongoConnection{ConnectionStringSource: mongoContainer.URI, Database: "test_db", MaxPoolSize: 1, DB: mongoContainer.Client}
	packageRepo, err := pack.NewPackageMongoDBRepository(feeConn, logger)
	require.NoError(t, err)
	resolver, err := feesservices.NewQueryResolver(queryUC)
	require.NoError(t, err)
	feeUC, err := feesservices.NewUseCase(packageRepo, resolver, "USD")
	require.NoError(t, err)

	h := &feeHarness{
		pgContainer: pgContainer, mongoContainer: mongoContainer, redisContainer: redisContainer,
		pgConn: pgConn, db: pgContainer.DB, redisRepo: redisRepo, metaRepo: metaRepo, packageRepo: packageRepo,
		commandUC: commandUC, queryUC: queryUC, feeUC: feeUC,
		handler: &TransactionHandler{Query: queryUC, Command: commandUC, FeeApplier: feeUC},
	}
	h.orgID = postgrestestutil.CreateTestOrganization(t, h.db)
	h.ledgerID = postgrestestutil.CreateTestLedger(t, h.db, h.orgID)

	app := h.newApp()

	// Consumer wiring.
	telemetry, err := libOpentelemetry.NewTelemetry(libOpentelemetry.TelemetryConfig{
		LibraryName: "test", ServiceName: "fee-async-test", ServiceVersion: "test", EnableTelemetry: false, Logger: logger,
	})
	require.NoError(t, err)

	consumerConn := &libRabbitmq.RabbitMQConnection{
		ConnectionStringSource: rabbitContainer.URI, HealthCheckURL: rabbitHealthURL,
		Host: rabbitContainer.Host, Port: rabbitContainer.AMQPPort,
		User: rabbitmqtestutil.DefaultUser, Pass: rabbitmqtestutil.DefaultPassword, Logger: logger,
	}
	t.Cleanup(func() {
		if consumerConn.Channel != nil {
			_ = consumerConn.Channel.Close()
		}
		if consumerConn.Connection != nil {
			_ = consumerConn.Connection.Close()
		}
		time.Sleep(500 * time.Millisecond)
	})

	routes := rabbitmq.NewConsumerRoutes(consumerConn, 1, 1, logger, telemetry)
	consumer := &asyncFeeConsumer{routes: routes, useCase: commandUC}
	routes.Register(os.Getenv("RABBITMQ_TRANSACTION_BALANCE_OPERATION_QUEUE"), consumer.handle)

	started := make(chan struct{})
	go func() { close(started); _ = routes.RunConsumers() }()
	<-started
	time.Sleep(500 * time.Millisecond)

	// Seed balances + a non-deductible flat fee package.
	h.seedBalance(t, "@payer", "USD", decimal.NewFromInt(100000), "deposit")
	h.seedBalance(t, "@receiver", "USD", decimal.Zero, "deposit")
	h.seedBalance(t, "@fee_rev", "USD", decimal.Zero, "deposit")
	h.seedPackage(t, packageSpec{label: "async_pkg", fees: []feeSpec{flatFee("async_fee", "@fee_rev", "10", false)}})

	body := `{
		"description": "async fee tx",
		"pending": false,
		"send": {
			"asset": "USD",
			"value": "1000",
			"source": { "from": [{"accountAlias": "@payer", "amount": {"asset": "USD", "value": "1000"}}] },
			"distribute": { "to": [{"accountAlias": "@receiver", "amount": {"asset": "USD", "value": "1000"}}] }
		}
	}`

	resp := h.createJSON(t, app, body, nil)
	require.Equalf(t, 201, resp.status, "async fee create must succeed: %s", string(resp.rawBody))

	txID := mustTxID(t, resp)

	// Crash-recovery proof: the backup seed reconstructs to the FEE-INCLUSIVE
	// transaction. SendTransactionToRedisQueue (funnel L1128) seeds AFTER
	// applyFees (L1084), so TransactionInput must carry the fee legs.
	assertBackupFeeInclusive(t, h, txID)

	// Behavioral proof: the async worker persists fee legs and balances.
	require.True(t, waitForTxStatus(t, h, txID, "APPROVED", 15*time.Second),
		"async worker must persist the transaction to APPROVED")

	legs := loadLegs(t, h.db, txID)
	require.NotEmpty(t, legs, "async path must persist operations")
	requireBalanced(t, legs, "async fee tx")

	feeLegs := feeCreditLegs(legs, "@fee_rev")
	require.NotEmpty(t, feeLegs, "async persisted operations must include the fee legs")
	assert.Truef(t, sumAmounts(feeLegs).Equal(decimal.NewFromInt(10)),
		"async fee legs must total exactly 10, got %s", sumAmounts(feeLegs).String())
}

// assertBackupFeeInclusive reads the Redis backup-queue seed for the transaction
// and asserts the seeded TransactionInput carries the fee legs (fee-inclusive),
// proving a crash-recovery replay reconstructs the post-fee payload.
func assertBackupFeeInclusive(t *testing.T, h *feeHarness, txID interface{ String() string }) {
	t.Helper()

	ctx := context.Background()
	msgs, err := h.redisRepo.ReadAllMessagesFromQueue(ctx)
	require.NoError(t, err, "read backup queue")

	var found bool
	for _, raw := range msgs {
		var q mmodel.TransactionRedisQueue
		if json.Unmarshal([]byte(raw), &q) != nil {
			continue
		}
		if q.TransactionID.String() != txID.String() {
			continue
		}
		found = true

		// The seeded transaction input must contain the fee credit account on the
		// distribute side — i.e. the post-fee, fee-inclusive payload.
		var hasFeeLeg bool
		for _, ft := range q.TransactionInput.Send.Distribute.To {
			if aliasContains(ft.AccountAlias, "@fee_rev") {
				hasFeeLeg = true
			}
		}
		assert.True(t, hasFeeLeg,
			"backup seed must reconstruct to the FEE-INCLUSIVE transaction (fee leg present in TransactionInput), not the pre-fee payload")
	}
	require.True(t, found, "backup seed for the transaction must exist in the Redis backup queue")
}

func aliasContains(alias, want string) bool {
	return len(alias) >= len(want) && (alias == want || containsSub(alias, want))
}

func containsSub(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

// waitForTxStatus polls the persisted transaction status until it matches or the
// timeout elapses.
func waitForTxStatus(t *testing.T, h *feeHarness, txID interface{ String() string }, want string, timeout time.Duration) bool {
	t.Helper()

	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		var status string
		err := h.db.QueryRow(`SELECT status FROM transaction WHERE id = $1`, txID.String()).Scan(&status)
		if err == nil && status == want {
			return true
		}
		time.Sleep(150 * time.Millisecond)
	}
	return false
}

var _ = mtransaction.Transaction{}
