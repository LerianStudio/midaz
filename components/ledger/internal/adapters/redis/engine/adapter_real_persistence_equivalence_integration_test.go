//go:build integration

// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package engine

import (
	"context"
	"database/sql"
	"testing"
	"time"

	tmcore "github.com/LerianStudio/lib-commons/v6/commons/tenant-manager/core"
	libObservability "github.com/LerianStudio/lib-observability/v4"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	mongodb "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/mongodb/transaction"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/operation"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/recovery"
	postgresTransaction "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/transaction"
	txredis "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/redis/transaction"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/services/command"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	"github.com/LerianStudio/midaz/v4/pkg/mtransaction"
	mongotestutil "github.com/LerianStudio/midaz/v4/tests/utils/mongodb"
	pgtestutil "github.com/LerianStudio/midaz/v4/tests/utils/postgres"
)

type realPersistenceFixture struct {
	db        *sql.DB
	metadata  *mongodb.MetadataMongoDBRepository
	finalizer *command.BalanceEngineFinalizer
}

type realPersistenceSQLSnapshot struct {
	transaction  string
	operations   []string
	operationIDs []string
}

type realPersistenceMetadataSnapshot struct {
	entityID   string
	entityName string
	data       mongodb.JSON
	createdAt  time.Time
	updatedAt  time.Time
}

func newRealPersistenceFixture(t *testing.T) realPersistenceFixture {
	t.Helper()
	pgConfig := pgtestutil.DefaultContainerConfig()
	pgConfig.Image = "postgres:17"
	pg := pgtestutil.SetupContainerWithConfig(t, pgConfig)
	dsn := pgtestutil.BuildConnectionString(pg.Host, pg.Port, pg.Config)
	pgConnection := pgtestutil.CreatePostgresClient(t, dsn, dsn, pg.Config.DBName, pgtestutil.FindMigrationsPath(t, "transaction"))
	store := recovery.NewStore(
		postgresTransaction.NewTransactionPostgreSQLRepository(pgConnection),
		operation.NewOperationPostgreSQLRepository(pgConnection),
	)
	mongoConfig := mongotestutil.DefaultContainerConfig()
	mongoConfig.Image = "mongo:8"
	mongoContainer := mongotestutil.SetupContainerWithConfig(t, mongoConfig)
	mongoConnection := mongotestutil.CreateConnection(t, mongoContainer.URI, mongoContainer.DBName)
	metadata := mongodb.NewMetadataMongoDBRepository(mongoConnection)

	return realPersistenceFixture{
		db:        pg.DB,
		metadata:  metadata,
		finalizer: command.NewBalanceEngineFinalizer(store, metadata),
	}
}

func captureRealPersistenceSQL(t *testing.T, db *sql.DB, transactionID string) realPersistenceSQLSnapshot {
	t.Helper()
	ctx := context.Background()
	var snapshot realPersistenceSQLSnapshot
	require.NoError(t, db.QueryRowContext(ctx, `SELECT row_to_json(t)::text FROM transaction AS t WHERE id = $1`, transactionID).Scan(&snapshot.transaction))
	rows, err := db.QueryContext(ctx, `SELECT id, row_to_json(o)::text FROM operation AS o WHERE transaction_id = $1 ORDER BY id`, transactionID)
	require.NoError(t, err)
	defer rows.Close()
	for rows.Next() {
		var id, row string
		require.NoError(t, rows.Scan(&id, &row))
		snapshot.operationIDs = append(snapshot.operationIDs, id)
		snapshot.operations = append(snapshot.operations, row)
	}
	require.NoError(t, rows.Err())

	return snapshot
}

func assertRealPersistenceProjection(t *testing.T, db *sql.DB, record command.BalanceEnginePersistenceRecord) {
	t.Helper()
	for _, expected := range record.Transaction.Operations {
		var beforeVersion, afterVersion int64
		require.NoError(t, db.QueryRowContext(
			context.Background(),
			`SELECT balance_version_before, balance_version_after FROM operation WHERE id = $1 AND transaction_id = $2`,
			expected.ID, expected.TransactionID,
		).Scan(&beforeVersion, &afterVersion))
		require.Equal(t, *expected.Balance.Version, beforeVersion)
		require.Equal(t, *expected.BalanceAfter.Version, afterVersion)
	}
}

func captureRealPersistenceMetadata(t *testing.T, ctx context.Context, repo *mongodb.MetadataMongoDBRepository, transactionID string, operationIDs []string) map[string]realPersistenceMetadataSnapshot {
	t.Helper()
	entities := append([]string{transactionID}, operationIDs...)
	snapshot := make(map[string]realPersistenceMetadataSnapshot, len(entities))
	for index, id := range entities {
		entity := constant.EntityOperation
		if index == 0 {
			entity = constant.EntityTransaction
		}
		actual, err := repo.FindByEntity(ctx, entity, id)
		require.NoError(t, err)
		require.NotNil(t, actual, "missing %s metadata for %s", entity, id)
		snapshot[entity+":"+id] = realPersistenceMetadataSnapshot{
			entityID: actual.EntityID, entityName: actual.EntityName, data: actual.Data,
			createdAt: actual.CreatedAt, updatedAt: actual.UpdatedAt,
		}
	}

	return snapshot
}

func clearRealPersistenceRecord(t *testing.T, ctx context.Context, fixture realPersistenceFixture, snapshot realPersistenceSQLSnapshot, transactionID string) {
	t.Helper()
	for _, id := range snapshot.operationIDs {
		_, err := fixture.db.ExecContext(ctx, `DELETE FROM operation WHERE id = $1`, id)
		require.NoError(t, err)
		require.NoError(t, fixture.metadata.Delete(ctx, constant.EntityOperation, id))
	}
	_, err := fixture.db.ExecContext(ctx, `DELETE FROM transaction WHERE id = $1`, transactionID)
	require.NoError(t, err)
	require.NoError(t, fixture.metadata.Delete(ctx, constant.EntityTransaction, transactionID))
}

func resetRealPersistenceToPending(t *testing.T, ctx context.Context, fixture realPersistenceFixture, pending *postgresTransaction.Transaction, terminalOperationIDs []string) {
	t.Helper()
	for _, id := range terminalOperationIDs {
		_, err := fixture.db.ExecContext(ctx, `DELETE FROM operation WHERE id = $1`, id)
		require.NoError(t, err)
		require.NoError(t, fixture.metadata.Delete(ctx, constant.EntityOperation, id))
	}
	_, err := fixture.db.ExecContext(ctx, `UPDATE transaction SET status = $1, status_description = $2, updated_at = $3 WHERE id = $4`,
		pending.Status.Code, pending.Status.Description, pending.UpdatedAt, pending.ID)
	require.NoError(t, err)
}

func realPersistenceRecoveryEnvelope(t *testing.T, ctx context.Context, client *redis.Client, keys resolvedExecutionKeys, execution command.EngineExecution) ([]byte, *command.BalanceEngineRecoveryEnvelope) {
	t.Helper()
	field := execution.Request.Transactions[0].ID.String() + ":" + execution.Request.ExecutionID.String()
	raw, err := client.HGet(ctx, keys.Recovery, field).Bytes()
	require.NoError(t, err)
	envelope, err := command.DecodeBalanceEngineRecoveryEnvelope(raw)
	require.NoError(t, err)

	return raw, envelope
}

func TestIntegration_BalanceEngineNormalAndRecoveryPersistenceAreEquivalent(t *testing.T) {
	t.Setenv("AUDIT_LOG_ENABLED", "false")
	fixture := newRealPersistenceFixture(t)

	t.Run("direct create", func(t *testing.T) {
		ctx := tmcore.ContextWithTenantID(context.Background(), "tenant-real-persistence-direct")
		ctx = libObservability.ContextWithHeaderID(ctx, "request-real-persistence-direct")
		client, _, _ := newAdapterValkey(t)
		hook := &integrationCommandHook{}
		client.AddHook(hook)
		organizationID := uuid.MustParse("c1111111-1111-4111-8111-111111111111")
		ledgerID := uuid.MustParse("c2222222-2222-4222-8222-222222222222")
		reader := &adapterCreateReader{balances: []*mmodel.Balance{
			adapterCreateBalance(organizationID, ledgerID, "c3333333-3333-4333-8333-333333333333", "c4444444-4444-4444-8444-444444444444", "@source", 100, 7),
			adapterCreateBalance(organizationID, ledgerID, "c5555555-5555-4555-8555-555555555555", "c6666666-6666-4666-8666-666666666666", "@target", 20, 3),
		}}
		ctrl := gomock.NewController(t)
		idempotency := txredis.NewMockRedisRepository(ctrl)
		stored := make(chan struct{})
		idempotency.EXPECT().SetNX(gomock.Any(), gomock.Any(), "", time.Minute).Return(true, nil)
		idempotency.EXPECT().Set(gomock.Any(), gomock.Any(), gomock.Any(), time.Minute).DoAndReturn(
			func(context.Context, string, string, time.Duration) error { close(stored); return nil },
		)
		realAdapter, err := NewAdapter(&integrationClientProvider{client: client}, guardBootstrapLimits())
		require.NoError(t, err)
		executor := &recordingCreateAdapter{delegate: realAdapter}
		uc := &command.UseCase{
			TransactionRedisRepo: idempotency, TransactionReader: reader,
			BalanceEngine: executor, BalanceEngineFinalizer: fixture.finalizer,
		}
		amount := decimal.NewFromInt(30)
		created, replayed, err := uc.CreateTransactionV2(ctx, command.CreateTransactionV2Input{
			OrganizationID: organizationID,
			LedgerID:       ledgerID,
			Transaction: mtransaction.Transaction{
				Description: "real direct persistence", Metadata: map[string]any{"path": "direct"},
				Send: mtransaction.Send{
					Asset: "USD", Value: amount,
					Source: mtransaction.Source{From: []mtransaction.FromTo{{
						AccountAlias: "@source", Amount: &mtransaction.Amount{Asset: "USD", Value: amount}, Metadata: map[string]any{"leg": "source"},
					}}},
					Distribute: mtransaction.Distribute{To: []mtransaction.FromTo{{
						AccountAlias: "@target", Amount: &mtransaction.Amount{Asset: "USD", Value: amount}, Metadata: map[string]any{"leg": "target"},
					}}},
				},
			},
			TransactionStatus: constant.CREATED,
			IdempotencyTTL:    time.Minute,
		})
		require.NoError(t, err)
		require.False(t, replayed)
		require.NotNil(t, created)
		require.Len(t, executor.inputs, 1)
		select {
		case <-stored:
		case <-time.After(time.Second):
			t.Fatal("durable create did not populate idempotency")
		}

		execution := executor.inputs[0]
		keys, err := resolveAdapterKeys(ctx, execution.Request)
		require.NoError(t, err)
		normalSQL := captureRealPersistenceSQL(t, fixture.db, created.ID)
		require.Len(t, normalSQL.operations, 2)
		normalMetadata := captureRealPersistenceMetadata(t, ctx, fixture.metadata, created.ID, normalSQL.operationIDs)
		normalState := captureAdapterState(t, client, keys)
		evalSHA, eval := hook.evalSHA.Load(), hook.eval.Load()
		recoveryRaw, envelope := realPersistenceRecoveryEnvelope(t, ctx, client, keys, execution)
		payload, err := command.DecodeBalanceEngineRecoveryPayload([]byte(envelope.Payload))
		require.NoError(t, err)
		record, err := command.ComposeBalanceEnginePersistenceRecord(*payload, envelope.Result)
		require.NoError(t, err)
		assertRealPersistenceProjection(t, fixture.db, record)

		clearRealPersistenceRecord(t, ctx, fixture, normalSQL, created.ID)
		outcome, err := fixture.finalizer.FinalizeWithOutcome(ctx, envelope)
		require.NoError(t, err)
		require.Equal(t, command.TransactionLifecyclePhaseCreated, outcome.Outcome.LifecyclePhase)
		require.Equal(t, normalSQL, captureRealPersistenceSQL(t, fixture.db, created.ID))
		require.Equal(t, normalMetadata, captureRealPersistenceMetadata(t, ctx, fixture.metadata, created.ID, normalSQL.operationIDs))
		assertRealPersistenceProjection(t, fixture.db, record)

		outcome, err = fixture.finalizer.FinalizeWithOutcome(ctx, envelope)
		require.NoError(t, err)
		require.Equal(t, command.TransactionLifecyclePhaseNoop, outcome.Outcome.LifecyclePhase)
		require.Equal(t, normalSQL, captureRealPersistenceSQL(t, fixture.db, created.ID))
		require.Equal(t, normalMetadata, captureRealPersistenceMetadata(t, ctx, fixture.metadata, created.ID, normalSQL.operationIDs))
		require.Equal(t, normalState, captureAdapterState(t, client, keys))
		require.Equal(t, evalSHA, hook.evalSHA.Load())
		require.Equal(t, eval, hook.eval.Load())
		retained, err := client.HGet(ctx, keys.Recovery, created.ID+":"+execution.Request.ExecutionID.String()).Bytes()
		require.NoError(t, err)
		require.Equal(t, recoveryRaw, retained, "the finalizer must not take recovery ACK ownership")
	})

	t.Run("pending commit", func(t *testing.T) {
		ctx := tmcore.ContextWithTenantID(context.Background(), "tenant-real-persistence-pending")
		ctx = libObservability.ContextWithHeaderID(ctx, "request-real-persistence-pending")
		client, _, _ := newAdapterValkey(t)
		hook := &integrationCommandHook{}
		client.AddHook(hook)
		organizationID := uuid.MustParse("d1111111-1111-4111-8111-111111111111")
		ledgerID := uuid.MustParse("d2222222-2222-4222-8222-222222222222")
		reader := &pendingLifecycleReader{balances: []*mmodel.Balance{
			adapterCreateBalance(organizationID, ledgerID, "d3333333-3333-4333-8333-333333333333", "d4444444-4444-4444-8444-444444444444", "@source", 100, 7),
			adapterCreateBalance(organizationID, ledgerID, "d5555555-5555-4555-8555-555555555555", "d6666666-6666-4666-8666-666666666666", "@target", 20, 3),
		}}
		ctrl := gomock.NewController(t)
		idempotency := txredis.NewMockRedisRepository(ctrl)
		stored := make(chan struct{})
		idempotency.EXPECT().SetNX(gomock.Any(), gomock.Any(), "", time.Minute).Return(true, nil)
		idempotency.EXPECT().Set(gomock.Any(), gomock.Any(), gomock.Any(), time.Minute).DoAndReturn(
			func(context.Context, string, string, time.Duration) error { close(stored); return nil },
		)
		idempotency.EXPECT().SetNX(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(true, nil)
		realAdapter, err := NewAdapter(&integrationClientProvider{client: client}, guardBootstrapLimits())
		require.NoError(t, err)
		executor := &pendingLifecycleAdapter{delegate: realAdapter}
		uc := &command.UseCase{
			TransactionRedisRepo: idempotency, TransactionReader: reader,
			BalanceEngine: executor, BalanceEngineFinalizer: fixture.finalizer,
		}
		amount := decimal.NewFromInt(30)
		pending, replayed, err := uc.CreateTransactionV2(ctx, command.CreateTransactionV2Input{
			OrganizationID: organizationID,
			LedgerID:       ledgerID,
			Transaction: mtransaction.Transaction{
				Description: "real pending persistence", Pending: true, Metadata: map[string]any{"path": "pending"},
				Send: mtransaction.Send{
					Asset: "USD", Value: amount,
					Source: mtransaction.Source{From: []mtransaction.FromTo{{
						AccountAlias: "@source", Amount: &mtransaction.Amount{Asset: "USD", Value: amount}, Metadata: map[string]any{"leg": "source"},
					}}},
					Distribute: mtransaction.Distribute{To: []mtransaction.FromTo{{
						AccountAlias: "@target", Amount: &mtransaction.Amount{Asset: "USD", Value: amount}, Metadata: map[string]any{"leg": "target"},
					}}},
				},
			},
			TransactionStatus: constant.PENDING,
			IdempotencyTTL:    time.Minute,
		})
		require.NoError(t, err)
		require.False(t, replayed)
		require.Equal(t, constant.PENDING, pending.Status.Code)
		select {
		case <-stored:
		case <-time.After(time.Second):
			t.Fatal("pending create did not populate idempotency")
		}

		reader.persisted = pending
		reader.balances[0].Available = decimal.NewFromInt(70)
		reader.balances[0].OnHold = decimal.NewFromInt(30)
		reader.balances[0].Version = 8
		committed, err := uc.CommitTransactionV2(ctx, command.PendingTransitionInput{
			OrganizationID: organizationID,
			LedgerID:       ledgerID,
			TransactionID:  uuid.MustParse(pending.ID),
		})
		require.NoError(t, err)
		require.Equal(t, constant.APPROVED, committed.Status.Code)
		require.Len(t, executor.executions, 2)

		terminalExecution := executor.executions[1]
		keys, err := resolveAdapterKeys(ctx, terminalExecution.Request)
		require.NoError(t, err)
		normalSQL := captureRealPersistenceSQL(t, fixture.db, committed.ID)
		require.Len(t, normalSQL.operations, 3)
		normalMetadata := captureRealPersistenceMetadata(t, ctx, fixture.metadata, committed.ID, normalSQL.operationIDs)
		normalState := captureAdapterState(t, client, keys)
		evalSHA, eval := hook.evalSHA.Load(), hook.eval.Load()
		recoveryRaw, envelope := realPersistenceRecoveryEnvelope(t, ctx, client, keys, terminalExecution)
		payload, err := command.DecodeBalanceEngineRecoveryPayload([]byte(envelope.Payload))
		require.NoError(t, err)
		terminalRecord, err := command.ComposeBalanceEnginePersistenceRecord(*payload, envelope.Result)
		require.NoError(t, err)
		assertRealPersistenceProjection(t, fixture.db, terminalRecord)
		terminalOperationIDs := make([]string, 0, len(terminalRecord.Transaction.Operations))
		for _, row := range terminalRecord.Transaction.Operations {
			terminalOperationIDs = append(terminalOperationIDs, row.ID)
		}

		resetRealPersistenceToPending(t, ctx, fixture, pending, terminalOperationIDs)
		outcome, err := fixture.finalizer.FinalizeWithOutcome(ctx, envelope)
		require.NoError(t, err)
		require.Equal(t, command.TransactionLifecyclePhaseUpdated, outcome.Outcome.LifecyclePhase)
		require.Equal(t, normalSQL, captureRealPersistenceSQL(t, fixture.db, committed.ID))
		require.Equal(t, normalMetadata, captureRealPersistenceMetadata(t, ctx, fixture.metadata, committed.ID, normalSQL.operationIDs))
		assertRealPersistenceProjection(t, fixture.db, terminalRecord)

		outcome, err = fixture.finalizer.FinalizeWithOutcome(ctx, envelope)
		require.NoError(t, err)
		require.Equal(t, command.TransactionLifecyclePhaseNoop, outcome.Outcome.LifecyclePhase)
		require.Equal(t, normalSQL, captureRealPersistenceSQL(t, fixture.db, committed.ID))
		require.Equal(t, normalMetadata, captureRealPersistenceMetadata(t, ctx, fixture.metadata, committed.ID, normalSQL.operationIDs))
		require.Equal(t, normalState, captureAdapterState(t, client, keys))
		require.Equal(t, evalSHA, hook.evalSHA.Load())
		require.Equal(t, eval, hook.eval.Load())
		retained, err := client.HGet(ctx, keys.Recovery, committed.ID+":"+terminalExecution.Request.ExecutionID.String()).Bytes()
		require.NoError(t, err)
		require.Equal(t, recoveryRaw, retained, "the finalizer must not take recovery ACK ownership")
	})
}
