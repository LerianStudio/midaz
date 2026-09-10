//go:build integration

// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	tmcore "github.com/LerianStudio/lib-commons/v6/commons/tenant-manager/core"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"

	mongodb "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/mongodb/transaction"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/completion"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/operation"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/transaction"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/engine"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/services/command"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	"github.com/LerianStudio/midaz/v4/pkg/mtransaction"
	mongotestutil "github.com/LerianStudio/midaz/v4/tests/utils/mongodb"
	pgtestutil "github.com/LerianStudio/midaz/v4/tests/utils/postgres"
)

func completionError(_ command.TransactionCompletionResult, err error) error {
	return err
}

func finalizerIntegrationID(name string) uuid.UUID {
	return uuid.NewSHA1(uuid.NameSpaceOID, []byte("balance-finalizer:"+name))
}

func finalizerIntegrationEnvelope(t *testing.T, name, tenant string, companion, emptyCompanionMetadata, unrepresentable bool) *command.TransactionCompletionRecord {
	t.Helper()
	date := time.Date(2026, time.September, 4, 12, 0, 0, 0, time.UTC)
	organizationID := finalizerIntegrationID("organization")
	ledgerID := finalizerIntegrationID("ledger")
	transactionID := finalizerIntegrationID(name + ":transaction")
	executionID := finalizerIntegrationID(name + ":execution")
	accountID := finalizerIntegrationID("account")
	balanceID := finalizerIntegrationID("balance")
	amount := decimal.NewFromInt(30)
	before := engine.BalanceState{Available: decimal.NewFromInt(100), Version: 7}
	after := engine.BalanceState{Available: decimal.NewFromInt(70), Version: 8}
	postingType, rowType, side, direction := engine.PostingDebit, constant.DEBIT, command.OperationSpecSideFrom, constant.DirectionDebit
	if companion {
		amount = decimal.NewFromInt(50)
		before = engine.BalanceState{OverdraftUsed: amount, Version: 7}
		after = engine.BalanceState{Version: 8}
		postingType, rowType, side, direction = engine.PostingCredit, constant.CREDIT, command.OperationSpecSideTo, constant.DirectionCredit
	}

	balance := command.OperationBalanceContext(mmodel.Balance{
		ID: balanceID.String(), AccountID: accountID.String(), OrganizationID: organizationID.String(), LedgerID: ledgerID.String(),
		Alias: "@source", Key: constant.DefaultBalanceKey, AssetCode: "USD", AccountType: "deposit", Direction: constant.DirectionCredit,
		Available: before.Available, OnHold: before.OnHold, OverdraftUsed: before.OverdraftUsed, Version: before.Version,
	})
	projection := command.OperationRecordSpec{
		TransactionID: transactionID, PostingRef: "leg:0", BalanceRef: "@source#default", Role: engine.RolePrimary,
		Side: side, RowType: rowType, Direction: direction, Balance: balance, RequestedAmount: amount,
		CompatibilityPath: command.OperationRecordStandard, Metadata: map[string]any{"purpose": "primary"},
	}
	if unrepresentable {
		projection.Metadata["precision"] = json.Number("0.12345678901234567890123456789")
	}

	payload := command.TransactionCompletionPlan{
		FormatVersion: command.TransactionCompletionFormatVersion, TenantID: tenant, HeaderID: "finalization-integration",
		OrganizationID: organizationID, LedgerID: ledgerID, TransactionID: transactionID, ExecutionID: executionID,
		TTL: date, TransactionDate: date, TransactionStatus: constant.APPROVED, Action: constant.ActionDirect,
		TransactionCreatedAt: date, TransactionUpdatedAt: date.Add(time.Millisecond), OperationUpdatedAt: date.Add(2 * time.Millisecond),
		TransactionInput: mtransaction.Transaction{
			Description: "frozen transaction", Send: mtransaction.Send{Asset: "USD", Value: amount},
			Metadata: map[string]any{"sequence": json.Number("9007199254740993"), "fraction": json.Number("0.1"), "purpose": "frozen"},
		},
		Validate: &mtransaction.Responses{Sources: []string{"@source"}, Destinations: []string{"@destination"}}, OperationSpecs: []command.OperationRecordSpec{projection},
	}
	movementAmount := amount
	if companion {
		movementAmount = decimal.Zero
	}

	result := engine.Result{Movements: []engine.Movement{{
		Ref: "primary:0", TransactionID: transactionID, PostingRef: projection.PostingRef, BalanceRef: projection.BalanceRef, Role: engine.RolePrimary,
		Type: postingType, Amount: movementAmount, Before: before, After: after, OverdraftDelta: after.OverdraftUsed.Sub(before.OverdraftUsed),
	}}, Final: []engine.BalanceSnapshot{{
		BalanceRef: projection.BalanceRef, ID: balanceID, AccountID: accountID, Alias: "@source", Key: constant.DefaultBalanceKey,
		AssetCode: "USD", AccountType: "deposit", Direction: constant.DirectionCredit,
		Available: after.Available, OnHold: after.OnHold, OverdraftUsed: after.OverdraftUsed, Version: after.Version,
	}}}
	if companion {
		companionBalanceID := finalizerIntegrationID("overdraft balance")
		companionProjection := projection
		companionProjection.Role, companionProjection.RowType = engine.RoleOverdraftCompanion, constant.OVERDRAFT
		companionProjection.BalanceRef = "@source#overdraft"
		companionProjection.Metadata = nil
		if emptyCompanionMetadata {
			companionProjection.Metadata = map[string]any{}
		}

		companionProjection.Balance.ID, companionProjection.Balance.Key = companionBalanceID.String(), constant.OverdraftBalanceKey
		companionProjection.Balance.Direction = constant.DirectionDebit
		companionProjection.Balance.Available, companionProjection.Balance.OverdraftUsed, companionProjection.Balance.Version = amount, decimal.Zero, 9
		payload.OperationSpecs = append(payload.OperationSpecs, companionProjection)
		result.Movements = append(result.Movements, engine.Movement{
			Ref: "companion:0", TransactionID: transactionID, PostingRef: projection.PostingRef, BalanceRef: companionProjection.BalanceRef, Role: engine.RoleOverdraftCompanion,
			Type: engine.PostingCredit, Amount: amount, Before: engine.BalanceState{Available: amount, Version: 9}, After: engine.BalanceState{Version: 10},
		})
		result.Final = append(result.Final, engine.BalanceSnapshot{
			BalanceRef: companionProjection.BalanceRef, ID: companionBalanceID, AccountID: accountID, Alias: "@source", Key: constant.OverdraftBalanceKey,
			AssetCode: "USD", AccountType: "deposit", Direction: constant.DirectionDebit, Version: 10,
		})
	}

	return encodeFinalizerIntegrationEnvelope(t, payload, result)
}

func encodeFinalizerIntegrationEnvelope(t *testing.T, payload command.TransactionCompletionPlan, result engine.Result) *command.TransactionCompletionRecord {
	t.Helper()
	intents := make([]command.OperationRecordIntent, 0, len(payload.OperationSpecs))
	refs := make([]string, 0, len(payload.OperationSpecs))
	for _, row := range payload.OperationSpecs {
		intents = append(intents, row.Intent())
		if row.Role == engine.RolePrimary {
			refs = append(refs, row.PostingRef)
		}
	}

	fingerprint, err := command.ComputeBalanceEngineIntentFingerprint(command.BalanceEngineIntent{
		TenantID: payload.TenantID, OrganizationID: payload.OrganizationID, LedgerID: payload.LedgerID, ExecutionID: payload.ExecutionID,
		Transactions: []command.BalanceEngineTransactionIntent{{
			TransactionID: payload.TransactionID, Action: payload.Action, TransactionStatus: payload.TransactionStatus,
			ParentTransactionID: payload.ParentTransactionID, FeesSkipped: payload.FeesSkipped, TracerSkipped: payload.TracerSkipped,
			TransactionDate: payload.TransactionDate, TransactionCreatedAt: payload.TransactionCreatedAt, TransactionUpdatedAt: payload.TransactionUpdatedAt, OperationUpdatedAt: payload.OperationUpdatedAt,
			Input: payload.TransactionInput, PostingRefs: refs, OperationSpecs: intents,
		}},
	})
	require.NoError(t, err)
	payload.IntentFingerprint = fingerprint
	raw, err := command.EncodeTransactionCompletionPlan(payload)
	require.NoError(t, err)
	envelope := command.TransactionCompletionRecord{
		FormatVersion: command.TransactionCompletionFormatVersion, TenantID: payload.TenantID, OrganizationID: payload.OrganizationID,
		LedgerID: payload.LedgerID, TransactionID: payload.TransactionID, ExecutionID: payload.ExecutionID, IntentFingerprint: fingerprint, Payload: string(raw), Result: result,
	}
	encoded, err := command.EncodeTransactionCompletionRecord(envelope)
	require.NoError(t, err)
	decoded, err := command.DecodeTransactionCompletionRecord(encoded)
	require.NoError(t, err)

	return decoded
}

type failOperationMetadataOnce struct {
	*mongodb.MetadataMongoDBRepository
	failure error
}

func (repo *failOperationMetadataOnce) Create(ctx context.Context, collection string, metadata *mongodb.Metadata) error {
	if collection == constant.EntityOperation && repo.failure != nil {
		failure := repo.failure
		repo.failure = nil

		return failure
	}

	return repo.MetadataMongoDBRepository.Create(ctx, collection, metadata)
}

func assertFinalizerSQLCounts(t *testing.T, db *sql.DB, transactionID uuid.UUID, transactions, operations int) {
	t.Helper()
	var count int
	require.NoError(t, db.QueryRowContext(context.Background(), "SELECT count(*) FROM transaction WHERE id = $1", transactionID.String()).Scan(&count))
	assert.Equal(t, transactions, count)
	require.NoError(t, db.QueryRowContext(context.Background(), "SELECT count(*) FROM operation WHERE transaction_id = $1", transactionID.String()).Scan(&count))
	assert.Equal(t, operations, count)
}

func finalizerOperationIDs(t *testing.T, envelope *command.TransactionCompletionRecord) []string {
	t.Helper()
	payload, err := command.DecodeTransactionCompletionPlan([]byte(envelope.Payload))
	require.NoError(t, err)
	rows, err := command.BuildOperationRecordsFromMovements(*payload, envelope.Result)
	require.NoError(t, err)
	ids := make([]string, 0, len(rows))
	for _, row := range rows {
		ids = append(ids, row.ID)
	}

	return ids
}

func assertFinalizerMetadataCount(t *testing.T, db *mongo.Database, entity, id string, expected int64) {
	t.Helper()
	count, err := db.Collection(strings.ToLower(entity)).CountDocuments(context.Background(), bson.M{"entity_id": id, "entity_name": entity})
	require.NoError(t, err)
	assert.Equal(t, expected, count)
}

func assertFinalizerLifecycleDates(t *testing.T, db *sql.DB, envelope *command.TransactionCompletionRecord, createdAt, updatedAt, operationUpdatedAt time.Time) {
	t.Helper()
	var actualCreated, actualUpdated time.Time
	require.NoError(t, db.QueryRowContext(context.Background(), "SELECT created_at, updated_at FROM transaction WHERE id = $1", envelope.TransactionID.String()).Scan(&actualCreated, &actualUpdated))
	assert.True(t, createdAt.Equal(actualCreated), "original transaction creation time must survive lifecycle transitions")
	assert.True(t, updatedAt.Equal(actualUpdated), "transaction update time must come from the frozen transaction")
	payload, err := command.DecodeTransactionCompletionPlan([]byte(envelope.Payload))
	require.NoError(t, err)
	for _, id := range finalizerOperationIDs(t, envelope) {
		require.NoError(t, db.QueryRowContext(context.Background(), "SELECT created_at, updated_at FROM operation WHERE id = $1", id).Scan(&actualCreated, &actualUpdated))
		assert.True(t, payload.TransactionDate.Equal(actualCreated), "operation creation time must come from the action")
		assert.True(t, operationUpdatedAt.Equal(actualUpdated), "operation update time must be frozen independently")
	}
}

func testFinalizerLifecycleTimestamps(t *testing.T, db *sql.DB, store command.TransactionWriteStore, metadata *mongodb.MetadataMongoDBRepository, action string) {
	t.Helper()
	ctx := context.Background()
	base := finalizerIntegrationEnvelope(t, t.Name(), "", false, false, false)
	holdPayload, err := command.DecodeTransactionCompletionPlan([]byte(base.Payload))
	require.NoError(t, err)
	t0 := holdPayload.TransactionCreatedAt
	holdPayload.Action, holdPayload.TransactionStatus = constant.ActionHold, constant.PENDING
	holdPayload.TransactionInput.Pending = true
	holdPayload.OperationSpecs[0].RowType = constant.ONHOLD
	holdResult := base.Result
	holdResult.Movements[0].Type = engine.PostingHold
	holdResult.Movements[0].After.OnHold = holdPayload.TransactionInput.Send.Value
	holdResult.Final[0].OnHold = holdPayload.TransactionInput.Send.Value
	holdEnvelope := encodeFinalizerIntegrationEnvelope(t, *holdPayload, holdResult)
	finalizer := command.NewTransactionCompletionService(store, metadata)
	require.NoError(t, completionError(finalizer.Complete(ctx, holdEnvelope)))
	assertFinalizerLifecycleDates(t, db, holdEnvelope, t0, holdPayload.TransactionUpdatedAt, holdPayload.OperationUpdatedAt)
	rootMetadata, err := metadata.FindByEntity(ctx, constant.EntityTransaction, holdEnvelope.TransactionID.String())
	require.NoError(t, err)
	require.NotNil(t, rootMetadata)

	terminalPayload, err := command.DecodeTransactionCompletionPlan([]byte(holdEnvelope.Payload))
	require.NoError(t, err)
	t1 := t0.Add(24 * time.Hour)
	terminalPayload.ExecutionID = finalizerIntegrationID(t.Name() + ":terminal execution")
	terminalPayload.Action = action
	terminalPayload.TransactionDate, terminalPayload.TransactionUpdatedAt, terminalPayload.OperationUpdatedAt = t1, t1.Add(time.Millisecond), t1.Add(2*time.Millisecond)
	terminalPayload.TransactionStatus = constant.APPROVED
	terminalPayload.OperationSpecs[0].PostingRef = "terminal:0"
	terminalPayload.OperationSpecs[0].RowType = constant.DEBIT
	terminalPayload.OperationSpecs[0].Balance.Available = holdResult.Movements[0].After.Available
	terminalPayload.OperationSpecs[0].Balance.OnHold = holdResult.Movements[0].After.OnHold
	terminalPayload.OperationSpecs[0].Balance.Version = holdResult.Movements[0].After.Version
	terminalType := engine.PostingUnreserve
	terminalAfter := engine.BalanceState{Available: decimal.NewFromInt(70), Version: 9}
	if action == constant.ActionCancel {
		terminalPayload.TransactionStatus = constant.CANCELED
		terminalPayload.OperationSpecs[0].RowType, terminalPayload.OperationSpecs[0].Direction = constant.RELEASE, constant.DirectionCredit
		terminalType, terminalAfter.Available = engine.PostingRelease, decimal.NewFromInt(100)
	}

	terminalFinal := holdResult.Final[0]
	terminalFinal.Available, terminalFinal.OnHold, terminalFinal.Version = terminalAfter.Available, terminalAfter.OnHold, terminalAfter.Version
	terminalResult := engine.Result{Movements: []engine.Movement{{
		Ref: "terminal:primary:0", TransactionID: holdEnvelope.TransactionID, PostingRef: "terminal:0", BalanceRef: "@source#default", Role: engine.RolePrimary,
		Type: terminalType, Amount: terminalPayload.TransactionInput.Send.Value, Before: holdResult.Movements[0].After, After: terminalAfter,
	}}, Final: []engine.BalanceSnapshot{terminalFinal}}
	terminalEnvelope := encodeFinalizerIntegrationEnvelope(t, *terminalPayload, terminalResult)
	failure := errors.New("operation metadata awaits recovery")
	flakyMetadata := &failOperationMetadataOnce{MetadataMongoDBRepository: metadata, failure: failure}
	recoveringFinalizer := command.NewTransactionCompletionService(store, flakyMetadata)
	require.ErrorIs(t, completionError(recoveringFinalizer.Complete(ctx, terminalEnvelope)), failure)
	assertFinalizerSQLCounts(t, db, terminalEnvelope.TransactionID, 1, 2)
	assertFinalizerLifecycleDates(t, db, terminalEnvelope, t0, terminalPayload.TransactionUpdatedAt, terminalPayload.OperationUpdatedAt)
	idsBefore := finalizerOperationIDs(t, terminalEnvelope)
	require.NoError(t, completionError(recoveringFinalizer.Complete(ctx, terminalEnvelope)))
	require.NoError(t, completionError(recoveringFinalizer.Complete(ctx, terminalEnvelope)))
	assert.Equal(t, idsBefore, finalizerOperationIDs(t, terminalEnvelope))
	assertFinalizerSQLCounts(t, db, terminalEnvelope.TransactionID, 1, 2)
	assertFinalizerLifecycleDates(t, db, terminalEnvelope, t0, terminalPayload.TransactionUpdatedAt, terminalPayload.OperationUpdatedAt)
	afterMetadata, err := metadata.FindByEntity(ctx, constant.EntityTransaction, terminalEnvelope.TransactionID.String())
	require.NoError(t, err)
	require.NotNil(t, afterMetadata)
	assert.Equal(t, rootMetadata.CreatedAt, afterMetadata.CreatedAt)
	assert.Equal(t, rootMetadata.UpdatedAt, afterMetadata.UpdatedAt)
	var persistedStatus string
	require.NoError(t, db.QueryRowContext(ctx, "SELECT status FROM transaction WHERE id = $1", terminalEnvelope.TransactionID.String()).Scan(&persistedStatus))
	assert.Equal(t, terminalPayload.TransactionStatus, persistedStatus)
}

func TestIntegrationTransactionCompletionServiceSQLAndMongo(t *testing.T) {
	pgConfig := pgtestutil.DefaultContainerConfig()
	pgConfig.Image = "postgres:17"
	pg := pgtestutil.SetupContainerWithConfig(t, pgConfig)
	migrations := pgtestutil.FindMigrationsPath(t, "transaction")
	dsn := pgtestutil.BuildConnectionString(pg.Host, pg.Port, pg.Config)
	pgConnection := pgtestutil.CreatePostgresClient(t, dsn, dsn, pg.Config.DBName, migrations)
	store := completion.NewStore(transaction.NewTransactionPostgreSQLRepository(pgConnection), operation.NewOperationPostgreSQLRepository(pgConnection))
	mongoConfig := mongotestutil.DefaultContainerConfig()
	mongoConfig.Image = "mongo:8"
	mongoContainer := mongotestutil.SetupContainerWithConfig(t, mongoConfig)
	mongoConnection := mongotestutil.CreateConnection(t, mongoContainer.URI, mongoContainer.DBName)
	metadata := mongodb.NewMetadataMongoDBRepository(mongoConnection)
	finalizer := command.NewTransactionCompletionService(store, metadata)
	ctx := context.Background()

	for _, action := range []string{constant.ActionCommit, constant.ActionCancel} {
		t.Run("hold then "+action+" preserves frozen timestamps", func(t *testing.T) {
			testFinalizerLifecycleTimestamps(t, pg.DB, store, metadata, action)
		})
	}

	t.Run("success and exact replay", func(t *testing.T) {
		envelope := finalizerIntegrationEnvelope(t, t.Name(), "", false, false, false)
		require.NoError(t, completionError(finalizer.Complete(ctx, envelope)))
		require.NoError(t, completionError(finalizer.Complete(ctx, envelope)))
		assertFinalizerSQLCounts(t, pg.DB, envelope.TransactionID, 1, 1)
		assertFinalizerMetadataCount(t, mongoContainer.Database, constant.EntityTransaction, envelope.TransactionID.String(), 1)
		for _, id := range finalizerOperationIDs(t, envelope) {
			assertFinalizerMetadataCount(t, mongoContainer.Database, constant.EntityOperation, id, 1)
		}

		actual, err := metadata.FindByEntity(ctx, constant.EntityTransaction, envelope.TransactionID.String())
		require.NoError(t, err)
		require.NotNil(t, actual)
		assert.Equal(t, int64(9007199254740993), actual.Data["sequence"])
		assert.Equal(t, float64(0.1), actual.Data["fraction"])
	})

	t.Run("metadata retry after SQL commit", func(t *testing.T) {
		envelope := finalizerIntegrationEnvelope(t, t.Name(), "", false, false, false)
		failure := errors.New("operation metadata write unavailable")
		flaky := &failOperationMetadataOnce{MetadataMongoDBRepository: metadata, failure: failure}
		retryFinalizer := command.NewTransactionCompletionService(store, flaky)
		require.ErrorIs(t, completionError(retryFinalizer.Complete(ctx, envelope)), failure)
		assertFinalizerSQLCounts(t, pg.DB, envelope.TransactionID, 1, 1)
		ids := finalizerOperationIDs(t, envelope)
		assertFinalizerMetadataCount(t, mongoContainer.Database, constant.EntityTransaction, envelope.TransactionID.String(), 1)
		assertFinalizerMetadataCount(t, mongoContainer.Database, constant.EntityOperation, ids[0], 0)
		require.NoError(t, completionError(retryFinalizer.Complete(ctx, envelope)))
		assertFinalizerSQLCounts(t, pg.DB, envelope.TransactionID, 1, 1)
		assertFinalizerMetadataCount(t, mongoContainer.Database, constant.EntityOperation, ids[0], 1)
	})

	t.Run("existing metadata is not overwritten", func(t *testing.T) {
		envelope := finalizerIntegrationEnvelope(t, t.Name(), "", false, false, false)
		date := time.Date(2026, time.September, 4, 12, 0, 0, 0, time.UTC)
		require.NoError(t, metadata.Create(ctx, constant.EntityTransaction, &mongodb.Metadata{
			EntityID: envelope.TransactionID.String(), EntityName: constant.EntityTransaction, Data: mongodb.JSON{"purpose": "authorized later edit"}, CreatedAt: date, UpdatedAt: date,
		}))
		require.ErrorIs(t, completionError(finalizer.Complete(ctx, envelope)), command.ErrBalanceEngineMetadataConflict)
		actual, err := metadata.FindByEntity(ctx, constant.EntityTransaction, envelope.TransactionID.String())
		require.NoError(t, err)
		require.NotNil(t, actual)
		assert.Equal(t, mongodb.JSON{"purpose": "authorized later edit"}, actual.Data)
		assertFinalizerSQLCounts(t, pg.DB, envelope.TransactionID, 1, 1)
	})

	for _, emptyMetadata := range []bool{false, true} {
		name := "companion nil metadata"
		if emptyMetadata {
			name = "companion explicit empty metadata"
		}

		t.Run(name, func(t *testing.T) {
			envelope := finalizerIntegrationEnvelope(t, t.Name(), "", true, emptyMetadata, false)
			require.NoError(t, completionError(finalizer.Complete(ctx, envelope)))
			require.NoError(t, completionError(finalizer.Complete(ctx, envelope)))
			assertFinalizerSQLCounts(t, pg.DB, envelope.TransactionID, 1, 2)
			ids := finalizerOperationIDs(t, envelope)
			assertFinalizerMetadataCount(t, mongoContainer.Database, constant.EntityOperation, ids[0], 1)
			expectedCompanionMetadata := int64(0)
			if emptyMetadata {
				expectedCompanionMetadata = 1
			}

			assertFinalizerMetadataCount(t, mongoContainer.Database, constant.EntityOperation, ids[1], expectedCompanionMetadata)
			var primaryAmount string
			require.NoError(t, pg.DB.QueryRowContext(ctx, "SELECT amount::text FROM operation WHERE id = $1", ids[0]).Scan(&primaryAmount))
			assert.True(t, decimal.RequireFromString(primaryAmount).IsZero())
		})
	}

	t.Run("unsupported late metadata does not write SQL", func(t *testing.T) {
		envelope := finalizerIntegrationEnvelope(t, t.Name(), "", false, false, true)
		require.ErrorIs(t, completionError(finalizer.Complete(ctx, envelope)), command.ErrBalanceEngineMetadataConflict)
		assertFinalizerSQLCounts(t, pg.DB, envelope.TransactionID, 0, 0)
		assertFinalizerMetadataCount(t, mongoContainer.Database, constant.EntityTransaction, envelope.TransactionID.String(), 0)
	})

	t.Run("trusted Mongo tenant context is required", func(t *testing.T) {
		tenantID := "finalizer-tenant"
		envelope := finalizerIntegrationEnvelope(t, t.Name(), tenantID, false, false, false)
		tenantMetadata := mongodb.NewMetadataMongoDBRepository(nil, true)
		tenantFinalizer := command.NewTransactionCompletionService(store, tenantMetadata)
		tenantCtx := tmcore.ContextWithTenantID(ctx, tenantID)
		require.ErrorContains(t, completionError(tenantFinalizer.Complete(tenantCtx, envelope)), "tenant mongo database missing from context")
		assertFinalizerSQLCounts(t, pg.DB, envelope.TransactionID, 1, 1)
		tenantDB := mongoContainer.Client.Database("finalizer_tenant")
		tenantCtx = tmcore.ContextWithMB(tenantCtx, tenantDB)
		tenantCtx = tmcore.ContextWithMB(tenantCtx, tenantDB, constant.ModuleTransaction)
		require.NoError(t, completionError(tenantFinalizer.Complete(tenantCtx, envelope)))
		assertFinalizerMetadataCount(t, tenantDB, constant.EntityTransaction, envelope.TransactionID.String(), 1)
		assertFinalizerMetadataCount(t, mongoContainer.Database, constant.EntityTransaction, envelope.TransactionID.String(), 0)
		otherCtx := tmcore.ContextWithTenantID(tenantCtx, "another-tenant")
		require.ErrorIs(t, completionError(tenantFinalizer.Complete(otherCtx, envelope)), command.ErrInvalidTransactionCompletionRecord)
	})
}
