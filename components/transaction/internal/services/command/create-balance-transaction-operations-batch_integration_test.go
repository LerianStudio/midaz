//go:build integration

// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"database/sql"
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vmihailenco/msgpack/v5"
	"go.uber.org/mock/gomock"

	libPostgres "github.com/LerianStudio/lib-commons/v2/commons/postgres"
	libZap "github.com/LerianStudio/lib-commons/v2/commons/zap"

	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/postgres/balance"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/postgres/operation"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/postgres/transaction"
	redisadapter "github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/redis"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/redpanda"
	"github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	pkgTransaction "github.com/LerianStudio/midaz/v3/pkg/transaction"
	pgtestutil "github.com/LerianStudio/midaz/v3/tests/utils/postgres"
	redistestutil "github.com/LerianStudio/midaz/v3/tests/utils/redis"
)

func TestIntegration_CreateBalanceTransactionOperationsBatch_PersistsAllInSingleTx(t *testing.T) {
	pgContainer := pgtestutil.SetupContainer(t)
	redisContainer := redistestutil.SetupContainer(t)

	ctx := context.Background()

	txRepo, opRepo, balanceRepo := createCommandPostgresRepos(t, pgContainer)
	redisRepo := createCommandRedisRepo(t, redisContainer)

	uc := &UseCase{
		TransactionRepo: txRepo,
		OperationRepo:   opRepo,
		BalanceRepo:     balanceRepo,
		RedisRepo:       redisRepo,
		EventsEnabled:   false,
	}

	organizationID := uuid.New()
	ledgerID := uuid.New()

	balance1ID := pgtestutil.CreateTestBalance(t, pgContainer.DB, organizationID, ledgerID, uuid.New(), pgtestutil.BalanceParams{
		Alias:          "@batch-src-1",
		Key:            "default",
		AssetCode:      "USD",
		Available:      decimal.NewFromInt(1000),
		OnHold:         decimal.Zero,
		AccountType:    "deposit",
		AllowSending:   true,
		AllowReceiving: true,
	})
	balance2ID := pgtestutil.CreateTestBalance(t, pgContainer.DB, organizationID, ledgerID, uuid.New(), pgtestutil.BalanceParams{
		Alias:          "@batch-dst-1",
		Key:            "default",
		AssetCode:      "USD",
		Available:      decimal.NewFromInt(500),
		OnHold:         decimal.Zero,
		AccountType:    "deposit",
		AllowSending:   true,
		AllowReceiving: true,
	})
	balance3ID := pgtestutil.CreateTestBalance(t, pgContainer.DB, organizationID, ledgerID, uuid.New(), pgtestutil.BalanceParams{
		Alias:          "@batch-src-2",
		Key:            "default",
		AssetCode:      "USD",
		Available:      decimal.NewFromInt(700),
		OnHold:         decimal.Zero,
		AccountType:    "deposit",
		AllowSending:   true,
		AllowReceiving: true,
	})
	balance4ID := pgtestutil.CreateTestBalance(t, pgContainer.DB, organizationID, ledgerID, uuid.New(), pgtestutil.BalanceParams{
		Alias:          "@batch-dst-2",
		Key:            "default",
		AssetCode:      "USD",
		Available:      decimal.NewFromInt(300),
		OnHold:         decimal.Zero,
		AccountType:    "deposit",
		AllowSending:   true,
		AllowReceiving: true,
	})

	queueA := buildBatchQueueMessage(t, batchQueueInput{
		organizationID: organizationID,
		ledgerID:       ledgerID,
		transactionID:  uuid.New().String(),
		txAmount:       decimal.NewFromInt(100),
		fromAlias:      "0#@batch-src-1#default",
		toAlias:        "1#@batch-dst-1#default",
		fromBalanceID:  balance1ID,
		toBalanceID:    balance2ID,
		fromAccountID:  uuid.New().String(),
		toAccountID:    uuid.New().String(),
		fromAvailable:  decimal.NewFromInt(1000),
		toAvailable:    decimal.NewFromInt(500),
	})
	queueB := buildBatchQueueMessage(t, batchQueueInput{
		organizationID: organizationID,
		ledgerID:       ledgerID,
		transactionID:  uuid.New().String(),
		txAmount:       decimal.NewFromInt(50),
		fromAlias:      "0#@batch-src-2#default",
		toAlias:        "1#@batch-dst-2#default",
		fromBalanceID:  balance3ID,
		toBalanceID:    balance4ID,
		fromAccountID:  uuid.New().String(),
		toAccountID:    uuid.New().String(),
		fromAvailable:  decimal.NewFromInt(700),
		toAvailable:    decimal.NewFromInt(300),
	})

	err := uc.CreateBalanceTransactionOperationsBatch(ctx, []mmodel.Queue{queueA, queueB})
	require.NoError(t, err)

	txIDA := uuid.MustParse(queueA.QueueData[0].ID.String())
	txIDB := uuid.MustParse(queueB.QueueData[0].ID.String())

	_, err = txRepo.Find(ctx, organizationID, ledgerID, txIDA)
	assert.NoError(t, err)
	_, err = txRepo.Find(ctx, organizationID, ledgerID, txIDB)
	assert.NoError(t, err)

	assertTableCount(t, pgContainer, "transaction", 2)
	assertTableCount(t, pgContainer, "operation", 4)
	assertDoubleEntryInvariant(t, pgContainer)

	b1, err := balanceRepo.Find(ctx, organizationID, ledgerID, balance1ID)
	require.NoError(t, err)
	assert.Equal(t, int64(1), b1.Version)
	assert.True(t, b1.Available.Equal(decimal.NewFromInt(900)))

	b2, err := balanceRepo.Find(ctx, organizationID, ledgerID, balance2ID)
	require.NoError(t, err)
	assert.Equal(t, int64(1), b2.Version)
	assert.True(t, b2.Available.Equal(decimal.NewFromInt(600)))

	b3, err := balanceRepo.Find(ctx, organizationID, ledgerID, balance3ID)
	require.NoError(t, err)
	assert.Equal(t, int64(1), b3.Version)
	assert.True(t, b3.Available.Equal(decimal.NewFromInt(650)))

	b4, err := balanceRepo.Find(ctx, organizationID, ledgerID, balance4ID)
	require.NoError(t, err)
	assert.Equal(t, int64(1), b4.Version)
	assert.True(t, b4.Available.Equal(decimal.NewFromInt(350)))
}

func TestIntegration_CreateBalanceTransactionOperationsBatch_RollsBackOnOperationInsertError(t *testing.T) {
	pgContainer := pgtestutil.SetupContainer(t)
	redisContainer := redistestutil.SetupContainer(t)

	ctx := context.Background()

	txRepo, opRepo, balanceRepo := createCommandPostgresRepos(t, pgContainer)
	redisRepo := createCommandRedisRepo(t, redisContainer)

	uc := &UseCase{
		TransactionRepo: txRepo,
		OperationRepo:   opRepo,
		BalanceRepo:     balanceRepo,
		RedisRepo:       redisRepo,
		EventsEnabled:   false,
	}

	organizationID := uuid.New()
	ledgerID := uuid.New()

	balanceSrcID := pgtestutil.CreateTestBalance(t, pgContainer.DB, organizationID, ledgerID, uuid.New(), pgtestutil.BalanceParams{
		Alias:          "@rollback-src",
		Key:            "default",
		AssetCode:      "USD",
		Available:      decimal.NewFromInt(1000),
		OnHold:         decimal.Zero,
		AccountType:    "deposit",
		AllowSending:   true,
		AllowReceiving: true,
	})
	balanceDstID := pgtestutil.CreateTestBalance(t, pgContainer.DB, organizationID, ledgerID, uuid.New(), pgtestutil.BalanceParams{
		Alias:          "@rollback-dst",
		Key:            "default",
		AssetCode:      "USD",
		Available:      decimal.NewFromInt(500),
		OnHold:         decimal.Zero,
		AccountType:    "deposit",
		AllowSending:   true,
		AllowReceiving: true,
	})

	queue := buildBatchQueueMessage(t, batchQueueInput{
		organizationID: organizationID,
		ledgerID:       ledgerID,
		transactionID:  uuid.New().String(),
		txAmount:       decimal.NewFromInt(100),
		fromAlias:      "0#@rollback-src#default",
		toAlias:        "1#@rollback-dst#default",
		fromBalanceID:  balanceSrcID,
		toBalanceID:    balanceDstID,
		fromAccountID:  uuid.New().String(),
		toAccountID:    uuid.New().String(),
		fromAvailable:  decimal.NewFromInt(1000),
		toAvailable:    decimal.NewFromInt(500),
	})

	var payload transaction.TransactionProcessingPayload
	err := msgpack.Unmarshal(queue.QueueData[0].Value, &payload)
	require.NoError(t, err)
	require.Len(t, payload.Transaction.Operations, 2)
	payload.Transaction.Operations[1].BalanceID = "not-a-uuid"
	queue.QueueData[0].Value = mustMsgpackMarshal(t, payload)

	err = uc.CreateBalanceTransactionOperationsBatch(ctx, []mmodel.Queue{queue})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid balance_id", "error should indicate invalid BalanceID")

	// The per-item fallback path (CreateBalanceTransactionOperationsAsync) processes
	// steps sequentially without a shared SQL transaction:
	//   1. UpdateBalances   -> committed independently
	//   2. CreateTransaction -> committed independently
	//   3. CreateOperations  -> fails validation (invalid BalanceID)
	// Therefore: balance and transaction are committed, but no operations exist.
	assertTableCount(t, pgContainer, "transaction", 1)
	assertTableCount(t, pgContainer, "operation", 0)

	bSrc, findErr := balanceRepo.Find(ctx, organizationID, ledgerID, balanceSrcID)
	require.NoError(t, findErr)
	assert.Equal(t, int64(1), bSrc.Version, "balance version incremented by non-atomic per-item path")

	bDst, findErr := balanceRepo.Find(ctx, organizationID, ledgerID, balanceDstID)
	require.NoError(t, findErr)
	assert.Equal(t, int64(1), bDst.Version, "balance version incremented by non-atomic per-item path")
}

func TestIntegration_CreateBalanceTransactionOperationsBatch_EmitsPostingFailedAfterRetryExhaustion(t *testing.T) {
	pgContainer := pgtestutil.SetupContainer(t)
	redisContainer := redistestutil.SetupContainer(t)

	ctx := context.Background()

	realTxRepo, realOpRepo, realBalanceRepo := createCommandPostgresRepos(t, pgContainer)
	redisRepo := createCommandRedisRepo(t, redisContainer)

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	brokerRepo := redpanda.NewMockProducerRepository(ctrl)
	actions := setupIntegrationDecisionEventCapture(t, brokerRepo, 1, "test-decision-events")

	retryAttempts := 0
	txRepo := &integrationRetryableBatchTxRepository{
		TransactionPostgreSQLRepository: realTxRepo,
		createBatchWithTxFn: func(_ context.Context, _ sqlExecQueryTx, _ []*transaction.Transaction) error {
			retryAttempts++
			return &pgconn.PgError{Code: "40P01", Message: "integration injected deadlock"}
		},
	}
	opRepo := &integrationBatchOperationRepository{OperationPostgreSQLRepository: realOpRepo}
	balanceRepo := &integrationBatchBalanceRepository{BalancePostgreSQLRepository: realBalanceRepo}

	uc := &UseCase{
		TransactionRepo:               txRepo,
		OperationRepo:                 opRepo,
		BalanceRepo:                   balanceRepo,
		RedisRepo:                     redisRepo,
		BrokerRepo:                    brokerRepo,
		EventsEnabled:                 true,
		EventsTopic:                   "test-decision-events",
		DecisionLifecycleSyncForTests: true,
	}

	organizationID := uuid.New()
	ledgerID := uuid.New()

	balanceSrcID := pgtestutil.CreateTestBalance(t, pgContainer.DB, organizationID, ledgerID, uuid.New(), pgtestutil.BalanceParams{
		Alias:          "@retry-src",
		Key:            "default",
		AssetCode:      "USD",
		Available:      decimal.NewFromInt(1000),
		OnHold:         decimal.Zero,
		AccountType:    "deposit",
		AllowSending:   true,
		AllowReceiving: true,
	})
	balanceDstID := pgtestutil.CreateTestBalance(t, pgContainer.DB, organizationID, ledgerID, uuid.New(), pgtestutil.BalanceParams{
		Alias:          "@retry-dst",
		Key:            "default",
		AssetCode:      "USD",
		Available:      decimal.NewFromInt(500),
		OnHold:         decimal.Zero,
		AccountType:    "deposit",
		AllowSending:   true,
		AllowReceiving: true,
	})

	queue := buildBatchQueueMessage(t, batchQueueInput{
		organizationID: organizationID,
		ledgerID:       ledgerID,
		transactionID:  uuid.New().String(),
		txAmount:       decimal.NewFromInt(100),
		fromAlias:      "0#@retry-src#default",
		toAlias:        "1#@retry-dst#default",
		fromBalanceID:  balanceSrcID,
		toBalanceID:    balanceDstID,
		fromAccountID:  uuid.New().String(),
		toAccountID:    uuid.New().String(),
		fromAvailable:  decimal.NewFromInt(1000),
		toAvailable:    decimal.NewFromInt(500),
	})

	err := uc.CreateBalanceTransactionOperationsBatch(ctx, []mmodel.Queue{queue})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "integration injected deadlock")
	assert.Equal(t, batchPersistMaxRetries, retryAttempts)

	observed := collectIntegrationDecisionActions(t, actions, 1)
	assert.Equal(t, []string{string(pkgTransaction.DecisionLifecycleActionPostingFailed)}, observed)

	assertTableCount(t, pgContainer, "transaction", 0)
	assertTableCount(t, pgContainer, "operation", 0)

	bSrc, findErr := balanceRepo.Find(ctx, organizationID, ledgerID, balanceSrcID)
	require.NoError(t, findErr)
	assert.True(t, bSrc.Available.Equal(decimal.NewFromInt(1000)))
	assert.Equal(t, int64(0), bSrc.Version)

	bDst, findErr := balanceRepo.Find(ctx, organizationID, ledgerID, balanceDstID)
	require.NoError(t, findErr)
	assert.True(t, bDst.Available.Equal(decimal.NewFromInt(500)))
	assert.Equal(t, int64(0), bDst.Version)
}

type integrationRetryableBatchTxRepository struct {
	*transaction.TransactionPostgreSQLRepository
	createBatchWithTxFn func(ctx context.Context, tx sqlExecQueryTx, transactions []*transaction.Transaction) error
}

func (r *integrationRetryableBatchTxRepository) BeginTx(ctx context.Context) (sqlBatchTx, error) {
	return r.TransactionPostgreSQLRepository.BeginTx(ctx)
}

func (r *integrationRetryableBatchTxRepository) CreateBatchWithTx(ctx context.Context, tx sqlExecQueryTx, transactions []*transaction.Transaction) error {
	if r.createBatchWithTxFn != nil {
		return r.createBatchWithTxFn(ctx, tx, transactions)
	}

	return r.TransactionPostgreSQLRepository.CreateBatchWithTx(ctx, tx, transactions)
}

type integrationBatchOperationRepository struct {
	*operation.OperationPostgreSQLRepository
}

func (r *integrationBatchOperationRepository) CreateBatchWithTx(
	ctx context.Context,
	tx interface {
		ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	},
	operations []*operation.Operation,
) error {
	return r.OperationPostgreSQLRepository.CreateBatchWithTx(ctx, tx, operations)
}

type integrationBatchBalanceRepository struct {
	*balance.BalancePostgreSQLRepository
}

func (r *integrationBatchBalanceRepository) BalancesUpdateWithTx(
	ctx context.Context,
	tx sqlExecQueryTx,
	organizationID, ledgerID uuid.UUID,
	balances []*mmodel.Balance,
) error {
	return r.BalancePostgreSQLRepository.BalancesUpdateWithTx(ctx, tx, organizationID, ledgerID, balances)
}

func setupIntegrationDecisionEventCapture(
	t *testing.T,
	mockBrokerRepo *redpanda.MockProducerRepository,
	expected int,
	topic string,
) <-chan string {
	t.Helper()

	actions := make(chan string, expected)

	mockBrokerRepo.EXPECT().
		ProducerDefault(gomock.Any(), topic, gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, _, _ string, payload []byte) (*string, error) {
			event := mmodel.Event{}
			require.NoError(t, json.Unmarshal(payload, &event))
			actions <- event.Action

			return nil, nil
		}).
		Times(expected)

	return actions
}

func collectIntegrationDecisionActions(t *testing.T, actions <-chan string, expected int) []string {
	t.Helper()

	collected := make([]string, 0, expected)
	timeout := time.After(2 * time.Second)

	for len(collected) < expected {
		select {
		case action := <-actions:
			collected = append(collected, action)
		case <-timeout:
			t.Fatalf("timed out waiting for %d decision lifecycle events, got %d", expected, len(collected))
		}
	}

	return collected
}

type batchQueueInput struct {
	organizationID uuid.UUID
	ledgerID       uuid.UUID
	transactionID  string
	txAmount       decimal.Decimal
	fromAlias      string
	toAlias        string
	fromBalanceID  uuid.UUID
	toBalanceID    uuid.UUID
	fromAccountID  string
	toAccountID    string
	fromAvailable  decimal.Decimal
	toAvailable    decimal.Decimal
}

func buildBatchQueueMessage(t *testing.T, input batchQueueInput) mmodel.Queue {
	t.Helper()

	now := time.Now().Truncate(time.Microsecond)
	txAmount := input.txAmount
	operationAmount := input.txAmount

	versionBefore := int64(0)
	versionAfter := int64(1)
	fromAfter := input.fromAvailable.Sub(input.txAmount)
	toAfter := input.toAvailable.Add(input.txAmount)

	tran := &transaction.Transaction{
		ID:                       input.transactionID,
		Description:              "integration batch transaction",
		Status:                   transaction.Status{Code: constant.CREATED},
		Amount:                   &txAmount,
		AssetCode:                "USD",
		ChartOfAccountsGroupName: "default",
		LedgerID:                 input.ledgerID.String(),
		OrganizationID:           input.organizationID.String(),
		CreatedAt:                now,
		UpdatedAt:                now,
		Operations: []*operation.Operation{
			{
				ID:              uuid.New().String(),
				TransactionID:   input.transactionID,
				Description:     "debit",
				Type:            constant.DEBIT,
				AssetCode:       "USD",
				Amount:          operation.Amount{Value: &operationAmount},
				Balance:         operation.Balance{Available: &input.fromAvailable, OnHold: decimalPtr(decimal.Zero), Version: &versionBefore},
				BalanceAfter:    operation.Balance{Available: &fromAfter, OnHold: decimalPtr(decimal.Zero), Version: &versionAfter},
				Status:          operation.Status{Code: constant.APPROVED},
				AccountID:       input.fromAccountID,
				AccountAlias:    "@src",
				BalanceID:       input.fromBalanceID.String(),
				BalanceKey:      "default",
				OrganizationID:  input.organizationID.String(),
				LedgerID:        input.ledgerID.String(),
				BalanceAffected: true,
				CreatedAt:       now,
				UpdatedAt:       now,
			},
			{
				ID:              uuid.New().String(),
				TransactionID:   input.transactionID,
				Description:     "credit",
				Type:            constant.CREDIT,
				AssetCode:       "USD",
				Amount:          operation.Amount{Value: &operationAmount},
				Balance:         operation.Balance{Available: &input.toAvailable, OnHold: decimalPtr(decimal.Zero), Version: &versionBefore},
				BalanceAfter:    operation.Balance{Available: &toAfter, OnHold: decimalPtr(decimal.Zero), Version: &versionAfter},
				Status:          operation.Status{Code: constant.APPROVED},
				AccountID:       input.toAccountID,
				AccountAlias:    "@dst",
				BalanceID:       input.toBalanceID.String(),
				BalanceKey:      "default",
				OrganizationID:  input.organizationID.String(),
				LedgerID:        input.ledgerID.String(),
				BalanceAffected: true,
				CreatedAt:       now,
				UpdatedAt:       now,
			},
		},
	}

	validate := &pkgTransaction.Responses{
		Aliases: []string{input.fromAlias, input.toAlias},
		From: map[string]pkgTransaction.Amount{
			input.fromAlias: {
				Asset:           "USD",
				Value:           input.txAmount,
				Operation:       constant.DEBIT,
				TransactionType: constant.CREATED,
			},
		},
		To: map[string]pkgTransaction.Amount{
			input.toAlias: {
				Asset:           "USD",
				Value:           input.txAmount,
				Operation:       constant.CREDIT,
				TransactionType: constant.CREATED,
			},
		},
	}

	payload := transaction.TransactionProcessingPayload{
		Validate: validate,
		Balances: []*mmodel.Balance{
			{
				ID:             input.fromBalanceID.String(),
				OrganizationID: input.organizationID.String(),
				LedgerID:       input.ledgerID.String(),
				AccountID:      input.fromAccountID,
				Alias:          input.fromAlias,
				Key:            "default",
				AssetCode:      "USD",
				Available:      input.fromAvailable,
				OnHold:         decimal.Zero,
				Version:        0,
				AccountType:    "deposit",
				AllowSending:   true,
				AllowReceiving: true,
			},
			{
				ID:             input.toBalanceID.String(),
				OrganizationID: input.organizationID.String(),
				LedgerID:       input.ledgerID.String(),
				AccountID:      input.toAccountID,
				Alias:          input.toAlias,
				Key:            "default",
				AssetCode:      "USD",
				Available:      input.toAvailable,
				OnHold:         decimal.Zero,
				Version:        0,
				AccountType:    "deposit",
				AllowSending:   true,
				AllowReceiving: true,
			},
		},
		Transaction: tran,
		Input:       &pkgTransaction.Transaction{},
	}

	return mmodel.Queue{
		OrganizationID: input.organizationID,
		LedgerID:       input.ledgerID,
		QueueData: []mmodel.QueueData{
			{
				ID:    uuid.MustParse(input.transactionID),
				Value: mustMsgpackMarshal(t, payload),
			},
		},
	}
}

func decimalPtr(v decimal.Decimal) *decimal.Decimal {
	return &v
}

func createCommandPostgresRepos(
	t *testing.T,
	container *pgtestutil.ContainerResult,
) (*transaction.TransactionPostgreSQLRepository, *operation.OperationPostgreSQLRepository, *balance.BalancePostgreSQLRepository) {
	t.Helper()

	logger := libZap.InitializeLogger()
	migrationsPath := pgtestutil.FindMigrationsPath(t, "transaction")

	connStr := pgtestutil.BuildConnectionString(container.Host, container.Port, container.Config)
	conn := &libPostgres.PostgresConnection{
		ConnectionStringPrimary: connStr,
		ConnectionStringReplica: connStr,
		PrimaryDBName:           container.Config.DBName,
		ReplicaDBName:           container.Config.DBName,
		MigrationsPath:          migrationsPath,
		Logger:                  logger,
	}

	db, err := conn.GetDB()
	require.NoError(t, err)
	t.Cleanup(func() {
		if closeErr := db.Close(); closeErr != nil {
			t.Logf("failed to close postgres dbresolver connection: %v", closeErr)
		}
	})

	transactionRepo, err := transaction.NewTransactionPostgreSQLRepository(conn)
	require.NoError(t, err, "failed to create transaction repository")

	operationRepo, err := operation.NewOperationPostgreSQLRepository(conn)
	require.NoError(t, err, "failed to create operation repository")

	balanceRepo, err := balance.NewBalancePostgreSQLRepository(conn)
	require.NoError(t, err, "failed to create balance repository")

	return transactionRepo, operationRepo, balanceRepo
}

func createCommandRedisRepo(t *testing.T, container *redistestutil.ContainerResult) *redisadapter.RedisConsumerRepository {
	t.Helper()

	conn := redistestutil.CreateConnection(t, container.Addr)
	repo, err := redisadapter.NewConsumerRedis(conn, false, nil)
	require.NoError(t, err)

	return repo
}

func assertTableCount(t *testing.T, container *pgtestutil.ContainerResult, table string, expected int) {
	t.Helper()

	var query string
	switch table {
	case "transaction":
		query = "SELECT COUNT(*) FROM transaction"
	case "operation":
		query = "SELECT COUNT(*) FROM operation"
	case "balance":
		query = "SELECT COUNT(*) FROM balance"
	default:
		require.FailNowf(t, "invalid table assertion", "table %q is not allowed in assertTableCount", table)
	}

	var count int
	err := container.DB.QueryRow(query).Scan(&count)
	require.NoError(t, err)
	assert.Equal(t, expected, count)
}

func assertDoubleEntryInvariant(t *testing.T, container *pgtestutil.ContainerResult) {
	t.Helper()

	var debitTotalText string
	var creditTotalText string

	err := container.DB.QueryRow(`
		SELECT
			COALESCE(SUM(CASE WHEN type = 'DEBIT' THEN amount ELSE 0 END), 0)::text,
			COALESCE(SUM(CASE WHEN type = 'CREDIT' THEN amount ELSE 0 END), 0)::text
		FROM operation;
	`).Scan(&debitTotalText, &creditTotalText)
	require.NoError(t, err)

	debitTotal, err := decimal.NewFromString(debitTotalText)
	require.NoError(t, err)

	creditTotal, err := decimal.NewFromString(creditTotalText)
	require.NoError(t, err)

	assert.Truef(t, debitTotal.Equal(creditTotal), "double-entry invariant violated: debit=%s credit=%s", debitTotal, creditTotal)
}
