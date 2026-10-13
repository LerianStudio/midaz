//go:build integration

// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"encoding/json"
	"sync"
	"testing"
	"time"

	libMongo "github.com/LerianStudio/lib-commons/v7/commons/mongo"
	libObservability "github.com/LerianStudio/lib-observability/v4"
	libLog "github.com/LerianStudio/lib-observability/v4/log"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	txmongodb "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/mongodb/transaction"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/operation"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/transaction"
	redis "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/redis/transaction"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	"github.com/LerianStudio/midaz/v4/pkg/mtransaction"
	"github.com/LerianStudio/midaz/v4/pkg/repository"
	"github.com/LerianStudio/midaz/v4/pkg/utils"
	mongotestutil "github.com/LerianStudio/midaz/v4/tests/utils/mongodb"
)

// bulkCleanupWait bounds how long a test waits for a cleanup goroutine that is expected to run.
const bulkCleanupWait = 5 * time.Second

// bulkCleanupAbsenceWindow is how long a test watches for a cleanup that must NOT run. The use
// case never starts the cleanup of a payload with unconfirmed metadata; the window guards against
// a regression that would start it, once a control payload's cleanup shows the batch reached the
// cleanup stage.
const bulkCleanupAbsenceWindow = 300 * time.Millisecond

// =============================================================================
// TEST INFRASTRUCTURE
// =============================================================================

// bulkCleanupRecorder records the backup and write-behind keys the cleanup goroutines remove
// and lets a test wait for, or rule out, the cleanup of a given transaction.
type bulkCleanupRecorder struct {
	orgID    uuid.UUID
	ledgerID uuid.UUID

	mu      sync.Mutex
	removed map[string]chan struct{}
	deleted map[string]chan struct{}
}

func newBulkCleanupRecorder(orgID, ledgerID uuid.UUID) *bulkCleanupRecorder {
	return &bulkCleanupRecorder{
		orgID:    orgID,
		ledgerID: ledgerID,
		removed:  make(map[string]chan struct{}),
		deleted:  make(map[string]chan struct{}),
	}
}

func (r *bulkCleanupRecorder) signal(set map[string]chan struct{}, key string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	ch := r.channel(set, key)

	select {
	case <-ch:
	default:
		close(ch)
	}
}

// channel returns the channel closed when key is cleaned up. Callers hold r.mu.
func (r *bulkCleanupRecorder) channel(set map[string]chan struct{}, key string) chan struct{} {
	ch, ok := set[key]
	if !ok {
		ch = make(chan struct{})
		set[key] = ch
	}

	return ch
}

func (r *bulkCleanupRecorder) channels(txID string) (backup, writeBehind chan struct{}) {
	r.mu.Lock()
	defer r.mu.Unlock()

	return r.channel(r.removed, utils.TransactionInternalKey(r.orgID, r.ledgerID, txID)),
		r.channel(r.deleted, utils.WriteBehindTransactionKey(r.orgID, r.ledgerID, txID))
}

// requireCleaned waits until both the backup and the write-behind entry of txID are removed.
func (r *bulkCleanupRecorder) requireCleaned(t *testing.T, txID string) {
	t.Helper()

	backup, writeBehind := r.channels(txID)

	for name, ch := range map[string]chan struct{}{"backup": backup, "write-behind": writeBehind} {
		select {
		case <-ch:
		case <-time.After(bulkCleanupWait):
			t.Fatalf("timed out waiting for %s cleanup of transaction %s", name, txID)
		}
	}
}

// requireNotCleaned asserts that neither entry of txID is removed within bulkCleanupAbsenceWindow.
func (r *bulkCleanupRecorder) requireNotCleaned(t *testing.T, txID string) {
	t.Helper()

	backup, writeBehind := r.channels(txID)
	deadline := time.After(bulkCleanupAbsenceWindow)

	select {
	case <-backup:
		t.Fatalf("backup of transaction %s must be retained", txID)
	case <-writeBehind:
		t.Fatalf("write-behind of transaction %s must be retained", txID)
	case <-deadline:
	}
}

// newBulkMetadataIntegrationUseCase wires CreateBulkTransactionOperationsAsync with the given
// metadata repository, gomock PostgreSQL repositories that report insertedTxIDs as the only
// fresh inserts, and a gomock Redis repository whose cleanups are recorded.
func newBulkMetadataIntegrationUseCase(
	t *testing.T,
	metadataRepo txmongodb.Repository,
	attempted int,
	insertedTxIDs []string,
	recorder *bulkCleanupRecorder,
) *UseCase {
	t.Helper()

	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	transactionRepo := transaction.NewMockRepository(ctrl)
	operationRepo := operation.NewMockRepository(ctrl)
	redisRepo := redis.NewMockRedisRepository(ctrl)

	dbTx := &mockDBTransaction{}
	transactionRepo.EXPECT().BeginTx(gomock.Any()).Return(dbTx, nil).Times(1)
	transactionRepo.EXPECT().
		CreateBulkTx(gomock.Any(), dbTx, gomock.Any()).
		Return(&repository.BulkInsertResult{
			Attempted:   int64(attempted),
			Inserted:    int64(len(insertedTxIDs)),
			Ignored:     int64(attempted - len(insertedTxIDs)),
			InsertedIDs: insertedTxIDs,
		}, nil).
		Times(1)
	operationRepo.EXPECT().
		CreateBulkTx(gomock.Any(), dbTx, gomock.Any()).
		Return(&repository.BulkInsertResult{Attempted: int64(attempted)}, nil).
		Times(1)

	rawQueue, err := json.Marshal(mmodel.TransactionRedisQueue{TransactionStatus: constant.CREATED})
	require.NoError(t, err)

	redisRepo.EXPECT().ReadMessageFromQueue(gomock.Any(), gomock.Any()).Return(rawQueue, nil).AnyTimes()
	redisRepo.EXPECT().
		RemoveMessageFromQueue(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, key string) error {
			recorder.signal(recorder.removed, key)

			return nil
		}).
		AnyTimes()
	redisRepo.EXPECT().
		Del(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, key string) error {
			recorder.signal(recorder.deleted, key)

			return nil
		}).
		AnyTimes()

	return &UseCase{
		TransactionRepo:         transactionRepo,
		OperationRepo:           operationRepo,
		TransactionMetadataRepo: metadataRepo,
		TransactionRedisRepo:    redisRepo,
	}
}

// bulkMetadataPayload builds an APPROVED payload with transaction metadata and two operations
// carrying their own metadata. It returns the payload and the operation IDs.
func bulkMetadataPayload(orgID, ledgerID uuid.UUID, txID string) (transaction.TransactionProcessingPayload, []string) {
	opIDs := []string{uuid.NewString(), uuid.NewString()}

	return transaction.TransactionProcessingPayload{
		Transaction: &transaction.Transaction{
			ID:             txID,
			OrganizationID: orgID.String(),
			LedgerID:       ledgerID.String(),
			Status:         transaction.Status{Code: constant.APPROVED},
			Metadata:       map[string]any{"reference": "tx-" + txID},
			Operations: []*operation.Operation{
				{ID: opIDs[0], TransactionID: txID, Metadata: map[string]any{"leg": "debit"}},
				{ID: opIDs[1], TransactionID: txID, Metadata: map[string]any{"leg": "credit"}},
			},
		},
		Validate: &mtransaction.Responses{Aliases: []string{"alias"}},
		Version:  "v2",
	}, opIDs
}

// =============================================================================
// BULK ASYNC: METADATA PERSISTENCE
// =============================================================================

func TestIntegration_BulkMetadataPersistence_WritesTransactionAndOperationMetadata(t *testing.T) {
	env := setupMetadataPersistenceEnv(t)
	orgID, ledgerID := uuid.New(), uuid.New()
	recorder := newBulkCleanupRecorder(orgID, ledgerID)

	txID := uuid.NewString()
	payload, opIDs := bulkMetadataPayload(orgID, ledgerID, txID)

	uc := newBulkMetadataIntegrationUseCase(t, env.uc.TransactionMetadataRepo, 1, []string{txID}, recorder)

	result, err := uc.CreateBulkTransactionOperationsAsync(context.Background(), []transaction.TransactionProcessingPayload{payload})

	require.NoError(t, err)
	assert.Empty(t, result.MetadataFailedTransactionIDs)

	assert.Equal(t, map[string]any{"reference": "tx-" + txID}, env.read(t, constant.EntityTransaction, txID).Data)
	assert.Equal(t, map[string]any{"leg": "debit"}, env.read(t, constant.EntityOperation, opIDs[0]).Data)
	assert.Equal(t, map[string]any{"leg": "credit"}, env.read(t, constant.EntityOperation, opIDs[1]).Data)

	recorder.requireCleaned(t, txID)
}

func TestIntegration_BulkMetadataPersistence_RedeliveryRepairsDuplicateTransaction(t *testing.T) {
	env := setupMetadataPersistenceEnv(t)
	orgID, ledgerID := uuid.New(), uuid.New()
	recorder := newBulkCleanupRecorder(orgID, ledgerID)

	txID := uuid.NewString()
	payload, opIDs := bulkMetadataPayload(orgID, ledgerID, txID)

	require.Equal(t, int64(0), env.count(t, constant.EntityTransaction, txID), "metadata must be absent before redelivery")

	// The transaction is already in PostgreSQL: the insert reports it as a duplicate.
	uc := newBulkMetadataIntegrationUseCase(t, env.uc.TransactionMetadataRepo, 1, nil, recorder)

	result, err := uc.CreateBulkTransactionOperationsAsync(context.Background(), []transaction.TransactionProcessingPayload{payload})

	require.NoError(t, err)
	assert.Empty(t, result.InsertedTransactionIDs)
	assert.Empty(t, result.MetadataFailedTransactionIDs)

	assert.Equal(t, map[string]any{"reference": "tx-" + txID}, env.read(t, constant.EntityTransaction, txID).Data)
	assert.Equal(t, map[string]any{"leg": "debit"}, env.read(t, constant.EntityOperation, opIDs[0]).Data)
	assert.Equal(t, map[string]any{"leg": "credit"}, env.read(t, constant.EntityOperation, opIDs[1]).Data)

	recorder.requireCleaned(t, txID)
}

func TestIntegration_BulkMetadataPersistence_RedeliveryKeepsExistingMetadata(t *testing.T) {
	env := setupMetadataPersistenceEnv(t)
	orgID, ledgerID := uuid.New(), uuid.New()
	recorder := newBulkCleanupRecorder(orgID, ledgerID)

	txID := uuid.NewString()
	payload, opIDs := bulkMetadataPayload(orgID, ledgerID, txID)

	env.seed(t, constant.EntityTransaction, txID, map[string]any{"reference": "patched"})
	env.seed(t, constant.EntityOperation, opIDs[0], map[string]any{"leg": "patched"})

	uc := newBulkMetadataIntegrationUseCase(t, env.uc.TransactionMetadataRepo, 1, nil, recorder)

	result, err := uc.CreateBulkTransactionOperationsAsync(context.Background(), []transaction.TransactionProcessingPayload{payload})

	require.NoError(t, err)
	assert.Empty(t, result.MetadataFailedTransactionIDs)

	txDoc := env.read(t, constant.EntityTransaction, txID)
	assert.Equal(t, map[string]any{"reference": "patched"}, txDoc.Data, "existing transaction metadata must not be overwritten")
	assert.True(t, txDoc.UpdatedAt.Equal(seededMetadataTime), "existing transaction metadata must not be touched")

	opDoc := env.read(t, constant.EntityOperation, opIDs[0])
	assert.Equal(t, map[string]any{"leg": "patched"}, opDoc.Data, "existing operation metadata must not be overwritten")
	assert.True(t, opDoc.UpdatedAt.Equal(seededMetadataTime), "existing operation metadata must not be touched")

	assert.Equal(t, map[string]any{"leg": "credit"}, env.read(t, constant.EntityOperation, opIDs[1]).Data,
		"absent operation metadata is still written")
	assert.Equal(t, int64(1), env.count(t, constant.EntityTransaction, txID))

	recorder.requireCleaned(t, txID)
}

func TestIntegration_BulkMetadataPersistence_MongoUnavailableRetainsBackup(t *testing.T) {
	// A dedicated container: stopping the package's reusable one would break other tests.
	container := mongotestutil.SetupContainer(t)

	// The helper's own client is disconnected first with a short deadline: once the server is
	// stopped, an unbounded disconnect waits for the driver's default server selection timeout.
	t.Cleanup(func() {
		disconnectCtx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()

		_ = container.Client.Disconnect(disconnectCtx)
	})

	conn, err := libMongo.NewClient(context.Background(), libMongo.Config{
		URI:                    container.URI,
		Database:               container.DBName,
		ServerSelectionTimeout: 500 * time.Millisecond,
	})
	require.NoError(t, err)

	t.Cleanup(func() {
		if closeErr := conn.Close(context.Background()); closeErr != nil {
			t.Logf("failed to close mongo connection: %v", closeErr)
		}
	})

	metadataRepo := txmongodb.NewMetadataMongoDBRepository(conn)

	stopTimeout := 5 * time.Second
	require.NoError(t, container.Container.Stop(context.Background(), &stopTimeout))

	orgID, ledgerID := uuid.New(), uuid.New()
	recorder := newBulkCleanupRecorder(orgID, ledgerID)

	failedTxID := uuid.NewString()
	failedPayload, _ := bulkMetadataPayload(orgID, ledgerID, failedTxID)

	// A control payload without metadata has nothing to persist, so its cleanup runs; waiting for
	// it proves the batch reached the cleanup stage, where the failed payload's cleanup is skipped.
	cleanTxID := uuid.NewString()
	cleanPayload, _ := bulkMetadataPayload(orgID, ledgerID, cleanTxID)
	cleanPayload.Transaction.Metadata = nil

	for _, op := range cleanPayload.Transaction.Operations {
		op.Metadata = nil
	}

	uc := newBulkMetadataIntegrationUseCase(t, metadataRepo, 2, []string{failedTxID, cleanTxID}, recorder)

	logger := &capturingLogger{}
	ctx := libObservability.ContextWithLogger(context.Background(), logger)

	started := time.Now()
	result, err := uc.CreateBulkTransactionOperationsAsync(ctx, []transaction.TransactionProcessingPayload{failedPayload, cleanPayload})

	require.NoError(t, err, "the bulk use case keeps its error contract when metadata fails")
	assert.Less(t, time.Since(started), 10*time.Second, "server selection must fail fast")
	assert.Equal(t, map[string]struct{}{failedTxID: {}}, result.MetadataFailedTransactionIDs)
	assert.False(t, result.FallbackUsed)

	// The two operation entries share a collection and go through CreateBulk, whose failure is
	// classified as infrastructure and skips the per-entry fallback.
	infraLines := 0

	for _, line := range logger.atLevelOrMoreSevere(libLog.LevelError) {
		if line.Msg == "Bulk metadata insert failed with infrastructure error, no fallback" {
			infraLines++
		}
	}

	assert.Equal(t, 1, infraLines, "the operation bulk insert failure must be infrastructure-class")

	recorder.requireCleaned(t, cleanTxID)
	recorder.requireNotCleaned(t, failedTxID)
}
