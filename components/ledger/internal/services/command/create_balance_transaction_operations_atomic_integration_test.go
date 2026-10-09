//go:build integration

// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vmihailenco/msgpack/v5"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"

	txmongodb "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/mongodb/transaction"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/operation"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/transaction"
	redis "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/redis/transaction"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	"github.com/LerianStudio/midaz/v4/pkg/mtransaction"
	"github.com/LerianStudio/midaz/v4/pkg/repository"
	mongotestutil "github.com/LerianStudio/midaz/v4/tests/utils/mongodb"
	pgtestutil "github.com/LerianStudio/midaz/v4/tests/utils/postgres"
)

var errInjectedOperationInsert = errors.New("injected operation insert failure")

var errInjectedOperationMetadata = errors.New("injected operation metadata failure")

// failingOperationInsertOnce fails the next operation insert, whichever entry
// point the writer uses, and then delegates to the real repository.
type failingOperationInsertOnce struct {
	operation.Repository

	mu   sync.Mutex
	fail bool
}

func (r *failingOperationInsertOnce) takeFailure() bool {
	r.mu.Lock()
	defer r.mu.Unlock()

	fail := r.fail
	r.fail = false

	return fail
}

func (r *failingOperationInsertOnce) Create(ctx context.Context, op *operation.Operation) (*operation.Operation, error) {
	if r.takeFailure() {
		return nil, errInjectedOperationInsert
	}

	return r.Repository.Create(ctx, op)
}

func (r *failingOperationInsertOnce) CreateBulkTx(ctx context.Context, tx repository.DBExecutor, ops []*operation.Operation) (*repository.BulkInsertResult, error) {
	if r.takeFailure() {
		return nil, errInjectedOperationInsert
	}

	return r.Repository.CreateBulkTx(ctx, tx, ops)
}

// failingOperationMetadataOnce fails the next operation metadata write and then
// delegates to the real repository.
type failingOperationMetadataOnce struct {
	txmongodb.Repository

	mu   sync.Mutex
	fail bool
}

func (r *failingOperationMetadataOnce) Create(ctx context.Context, collection string, metadata *txmongodb.Metadata) error {
	r.mu.Lock()
	fail := r.fail && collection == constant.EntityOperation
	if fail {
		r.fail = false
	}
	r.mu.Unlock()

	if fail {
		return errInjectedOperationMetadata
	}

	return r.Repository.Create(ctx, collection, metadata)
}

// acceptingCleanupRedis accepts the backup and write-behind cleanups the writer
// starts after a successful write.
type acceptingCleanupRedis struct {
	redis.RedisRepository
}

func (acceptingCleanupRedis) RemoveMessageFromQueue(context.Context, string) error { return nil }

func (acceptingCleanupRedis) ReadMessageFromQueue(context.Context, string) ([]byte, error) {
	return nil, errors.New("no backup entry")
}

func (acceptingCleanupRedis) Del(context.Context, string) error { return nil }

type legacyWriterEnv struct {
	pg          *sql.DB
	mongo       *mongo.Database
	transaction transaction.Repository
	operation   operation.Repository
	metadata    txmongodb.Repository
}

func setupLegacyWriterEnv(t *testing.T) *legacyWriterEnv {
	t.Helper()

	pgConfig := pgtestutil.DefaultContainerConfig()
	pgConfig.Image = "postgres:17"
	pg := pgtestutil.SetupContainerWithConfig(t, pgConfig)
	migrations := pgtestutil.FindMigrationsPath(t, "transaction")
	dsn := pgtestutil.BuildConnectionString(pg.Host, pg.Port, pg.Config)
	pgConnection := pgtestutil.CreatePostgresClient(t, dsn, dsn, pg.Config.DBName, migrations)

	mongoConfig := mongotestutil.DefaultContainerConfig()
	mongoConfig.Image = "mongo:8"
	mongoContainer := mongotestutil.SetupContainerWithConfig(t, mongoConfig)
	mongoConnection := mongotestutil.CreateConnection(t, mongoContainer.URI, mongoContainer.DBName)

	return &legacyWriterEnv{
		pg:          pg.DB,
		mongo:       mongoContainer.Database,
		transaction: transaction.NewTransactionPostgreSQLRepository(pgConnection, false),
		operation:   operation.NewOperationPostgreSQLRepository(pgConnection),
		metadata:    txmongodb.NewMetadataMongoDBRepository(mongoConnection),
	}
}

func (env *legacyWriterEnv) useCase(operationRepo operation.Repository, metadataRepo txmongodb.Repository) *UseCase {
	return &UseCase{
		TransactionRepo:         env.transaction,
		OperationRepo:           operationRepo,
		TransactionMetadataRepo: metadataRepo,
		TransactionRedisRepo:    acceptingCleanupRedis{},
	}
}

func (env *legacyWriterEnv) rowCounts(t *testing.T, transactionID string) (transactions, operations int) {
	t.Helper()

	require.NoError(t, env.pg.QueryRowContext(context.Background(),
		"SELECT count(*) FROM transaction WHERE id = $1", transactionID).Scan(&transactions))
	require.NoError(t, env.pg.QueryRowContext(context.Background(),
		"SELECT count(*) FROM operation WHERE transaction_id = $1", transactionID).Scan(&operations))

	return transactions, operations
}

func (env *legacyWriterEnv) status(t *testing.T, transactionID string) string {
	t.Helper()

	var status string

	require.NoError(t, env.pg.QueryRowContext(context.Background(),
		"SELECT status FROM transaction WHERE id = $1", transactionID).Scan(&status))

	return status
}

func (env *legacyWriterEnv) metadataCount(t *testing.T, entity, id string) int64 {
	t.Helper()

	count, err := env.mongo.Collection(strings.ToLower(entity)).CountDocuments(context.Background(), bson.M{"entity_id": id})
	require.NoError(t, err)

	return count
}

func legacyWriterOperation(tran *transaction.Transaction, alias, direction, opType string, amount decimal.Decimal) *operation.Operation {
	zero := decimal.Zero
	versionBefore, versionAfter := int64(1), int64(2)

	return &operation.Operation{
		ID:              uuid.NewString(),
		TransactionID:   tran.ID,
		Description:     "legacy writer operation",
		Type:            opType,
		AssetCode:       "BRL",
		ChartOfAccounts: "1000",
		Amount:          operation.Amount{Value: &amount},
		Balance:         operation.Balance{Available: &zero, OnHold: &zero, Version: &versionBefore},
		BalanceAfter:    operation.Balance{Available: &zero, OnHold: &zero, Version: &versionAfter},
		Status:          operation.Status{Code: constant.APPROVED},
		AccountID:       uuid.NewString(),
		AccountAlias:    alias,
		BalanceKey:      constant.DefaultBalanceKey,
		BalanceID:       uuid.NewString(),
		OrganizationID:  tran.OrganizationID,
		LedgerID:        tran.LedgerID,
		Direction:       direction,
		Metadata:        map[string]any{"leg": alias},
		CreatedAt:       tran.CreatedAt,
		UpdatedAt:       tran.CreatedAt,
	}
}

func legacyWriterTransaction(status string) *transaction.Transaction {
	amount := decimal.NewFromInt(100)
	createdAt := time.Date(2026, 10, 8, 12, 0, 0, 0, time.UTC)
	description := status

	return &transaction.Transaction{
		ID:             uuid.NewString(),
		OrganizationID: uuid.NewString(),
		LedgerID:       uuid.NewString(),
		Description:    "legacy writer transaction",
		Status:         transaction.Status{Code: status, Description: &description},
		Amount:         &amount,
		AssetCode:      "BRL",
		CreatedAt:      createdAt,
		UpdatedAt:      createdAt,
		Metadata:       map[string]any{"purpose": "legacy writer"},
	}
}

func legacyWriterQueue(t *testing.T, payload transaction.TransactionProcessingPayload) mmodel.Queue {
	t.Helper()

	encoded, err := msgpack.Marshal(payload)
	require.NoError(t, err)

	return mmodel.Queue{
		OrganizationID: uuid.MustParse(payload.Transaction.OrganizationID),
		LedgerID:       uuid.MustParse(payload.Transaction.LedgerID),
		QueueData:      []mmodel.QueueData{{ID: uuid.New(), Value: encoded}},
	}
}

func notedWriterPayload() (transaction.TransactionProcessingPayload, []string) {
	tran := legacyWriterTransaction(constant.NOTED)
	amount := decimal.NewFromInt(100)
	tran.Operations = []*operation.Operation{
		legacyWriterOperation(tran, "@payer", "debit", constant.DEBIT, amount),
		legacyWriterOperation(tran, "@payee", "credit", constant.CREDIT, amount),
	}

	ids := []string{tran.Operations[0].ID, tran.Operations[1].ID}

	return transaction.TransactionProcessingPayload{
		Transaction: tran,
		Validate:    &mtransaction.Responses{},
		Input:       &mtransaction.Transaction{},
		Version:     "v2",
	}, ids
}

func TestIntegrationLegacyWriterPersistsTransactionAndOperationsAtomically(t *testing.T) {
	env := setupLegacyWriterEnv(t)
	ctx := context.Background()

	t.Run("a failed operation insert leaves no transaction row and the retry writes everything", func(t *testing.T) {
		payload, operationIDs := notedWriterPayload()
		transactionID := payload.Transaction.ID
		uc := env.useCase(&failingOperationInsertOnce{Repository: env.operation, fail: true}, env.metadata)

		err := uc.CreateBalanceTransactionOperationsAsync(ctx, legacyWriterQueue(t, payload))
		require.ErrorIs(t, err, errInjectedOperationInsert)

		transactions, operations := env.rowCounts(t, transactionID)
		assert.Equal(t, 0, transactions, "a failed write must not leave the transaction row behind")
		assert.Equal(t, 0, operations)

		require.NoError(t, uc.CreateBalanceTransactionOperationsAsync(ctx, legacyWriterQueue(t, payload)))

		transactions, operations = env.rowCounts(t, transactionID)
		assert.Equal(t, 1, transactions)
		assert.Equal(t, 2, operations)
		assert.Equal(t, int64(1), env.metadataCount(t, constant.EntityTransaction, transactionID))

		for _, id := range operationIDs {
			assert.Equal(t, int64(1), env.metadataCount(t, constant.EntityOperation, id), "operation %s metadata", id)
		}

		require.NoError(t, uc.CreateBalanceTransactionOperationsAsync(ctx, legacyWriterQueue(t, payload)),
			"repeating a completed write is an idempotent no-op")

		transactions, operations = env.rowCounts(t, transactionID)
		assert.Equal(t, 1, transactions)
		assert.Equal(t, 2, operations)
	})

	t.Run("the retry writes the metadata of an operation that is already persisted", func(t *testing.T) {
		payload, operationIDs := notedWriterPayload()
		transactionID := payload.Transaction.ID
		uc := env.useCase(env.operation, &failingOperationMetadataOnce{Repository: env.metadata, fail: true})

		err := uc.CreateBalanceTransactionOperationsAsync(ctx, legacyWriterQueue(t, payload))
		require.ErrorIs(t, err, errInjectedOperationMetadata)

		require.NoError(t, uc.CreateBalanceTransactionOperationsAsync(ctx, legacyWriterQueue(t, payload)))

		transactions, operations := env.rowCounts(t, transactionID)
		assert.Equal(t, 1, transactions)
		assert.Equal(t, 2, operations)
		assert.Equal(t, int64(1), env.metadataCount(t, constant.EntityTransaction, transactionID))

		for _, id := range operationIDs {
			assert.Equal(t, int64(1), env.metadataCount(t, constant.EntityOperation, id), "operation %s metadata", id)
		}
	})

	t.Run("a failed operation insert leaves the pending transaction untransitioned", func(t *testing.T) {
		pending := legacyWriterTransaction(constant.PENDING)
		pending.Body = mtransaction.Transaction{Send: mtransaction.Send{Asset: "BRL", Value: decimal.NewFromInt(100)}}
		_, err := env.transaction.Create(ctx, pending)
		require.NoError(t, err)

		commit := legacyWriterTransaction(constant.APPROVED)
		commit.ID = pending.ID
		commit.OrganizationID = pending.OrganizationID
		commit.LedgerID = pending.LedgerID
		commit.Metadata = nil
		commit.Operations = []*operation.Operation{
			legacyWriterOperation(commit, "@payer", "debit", constant.RELEASE, decimal.NewFromInt(100)),
		}

		payload := transaction.TransactionProcessingPayload{
			Transaction: commit,
			Validate:    &mtransaction.Responses{Pending: true},
			Input:       &mtransaction.Transaction{},
			Version:     "v2",
		}
		uc := env.useCase(&failingOperationInsertOnce{Repository: env.operation, fail: true}, env.metadata)

		err = uc.CreateBalanceTransactionOperationsAsync(ctx, legacyWriterQueue(t, payload))
		require.ErrorIs(t, err, errInjectedOperationInsert)

		assert.Equal(t, constant.PENDING, env.status(t, pending.ID), "the transition must roll back with the operations")

		_, operations := env.rowCounts(t, pending.ID)
		assert.Equal(t, 0, operations)

		require.NoError(t, uc.CreateBalanceTransactionOperationsAsync(ctx, legacyWriterQueue(t, payload)))

		assert.Equal(t, constant.APPROVED, env.status(t, pending.ID))

		_, operations = env.rowCounts(t, pending.ID)
		assert.Equal(t, 1, operations)
	})
}
