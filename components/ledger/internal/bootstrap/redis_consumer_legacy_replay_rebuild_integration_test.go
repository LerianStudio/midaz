//go:build integration

// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	tmcore "github.com/LerianStudio/lib-commons/v7/commons/tenant-manager/core"
	tmvalkey "github.com/LerianStudio/lib-commons/v7/commons/tenant-manager/valkey"
	libLog "github.com/LerianStudio/lib-observability/v4/log"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	transactionMongo "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/mongodb/transaction"
	txRedis "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/redis/transaction"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	"github.com/LerianStudio/midaz/v4/pkg/mtransaction"
)

// unavailableMetadataRepository fails every metadata write, the shape of a
// MongoDB outage after the replay has committed the transaction to PostgreSQL.
type unavailableMetadataRepository struct {
	transactionMongo.Repository
}

func (unavailableMetadataRepository) Create(context.Context, string, *transactionMongo.Metadata) error {
	return errors.New("metadata database unavailable")
}

// lostCleanupRedisRepository drops the backup entry removal that follows a
// successful write, the shape of an HDEL lost to a Redis failure or a crash,
// and reports each dropped removal on dropped.
type lostCleanupRedisRepository struct {
	*txRedis.RedisConsumerRepository
	dropped chan string
}

func (repo lostCleanupRedisRepository) RemoveMessageFromQueue(_ context.Context, key string) error {
	repo.dropped <- key

	return nil
}

// replacementRedisRepository counts the backup record replacements the replay
// attempts and, when failure is set, fails them as an unreachable Redis would.
type replacementRedisRepository struct {
	*txRedis.RedisConsumerRepository
	failure  error
	attempts *atomic.Int32
}

func (repo replacementRedisRepository) ReplaceMessageInQueueIfValue(ctx context.Context, field, expected, replacement string) (txRedis.BackupRecordReplacement, error) {
	repo.attempts.Add(1)

	if repo.failure != nil {
		return txRedis.BackupRecordMissing, repo.failure
	}

	return repo.RedisConsumerRepository.ReplaceMessageInQueueIfValue(ctx, field, expected, replacement)
}

// rebuiltReplayCase is a legacy entry shape whose operations the replay must
// rebuild from its balance snapshots.
type rebuiltReplayCase struct {
	name    string
	status  string
	pending bool
}

var rebuiltReplayCases = []rebuiltReplayCase{
	{name: "commit of a pending transaction", status: constant.APPROVED, pending: true},
	{name: "direct transaction", status: constant.CREATED},
}

// TestIntegrationLegacyBackupReplayRetryAfterMetadataFailureKeepsRebuiltOperations
// replays a legacy entry without materialized operations whose first replay
// commits to PostgreSQL and then fails on the metadata write. The rebuilt
// operations must be kept in the entry, so the retry writes the same operation
// IDs instead of rebuilding new ones next to the committed legs.
func TestIntegrationLegacyBackupReplayRetryAfterMetadataFailureKeepsRebuiltOperations(t *testing.T) {
	if testing.Short() {
		t.Skip("requires PostgreSQL, MongoDB, Valkey, and RabbitMQ")
	}

	for _, tc := range rebuiltReplayCases {
		t.Run(tc.name, func(t *testing.T) {
			infra := setupRebuiltReplayIntegration(t)
			ctx := context.Background()
			field, transactionID := infra.leaveRebuildableLegacyEntry(t, ctx, tc)

			replay := NewRedisQueueConsumer(&libLog.GoLogger{}, infra.command, infra.query).
				WithQuarantineRepository(infra.quarantine).
				newLegacyBackupConsumer()

			metadata := infra.command.TransactionMetadataRepo
			infra.command.TransactionMetadataRepo = unavailableMetadataRepository{Repository: metadata}
			replay.Consume(ctx)
			infra.command.TransactionMetadataRepo = metadata

			require.EqualValues(t, 2, infra.operationCount(t, transactionID), "the first replay commits both legs")
			infra.requireEntryCarriesPersistedOperations(t, ctx, field, transactionID)
			infra.requireNoBackupAttempts(t, ctx)

			replay.Consume(ctx)
			infra.waitForEmptyLegacyBackupQueue(t, ctx)

			require.EqualValues(t, 2, infra.operationCount(t, transactionID), "a retry after a post-commit failure must not duplicate the legs")
			assert.Zero(t, infra.duplicatedLegCount(t, transactionID))
		})
	}
}

// TestIntegrationLegacyBackupReplayAfterLostCleanupKeepsRebuiltOperations
// replays a legacy entry without materialized operations whose removal is lost
// after a successful write. The entry left behind carries the operations the
// first replay wrote, so the next replay converges on the same rows.
func TestIntegrationLegacyBackupReplayAfterLostCleanupKeepsRebuiltOperations(t *testing.T) {
	if testing.Short() {
		t.Skip("requires PostgreSQL, MongoDB, Valkey, and RabbitMQ")
	}

	for _, tc := range rebuiltReplayCases {
		t.Run(tc.name, func(t *testing.T) {
			infra := setupRebuiltReplayIntegration(t)
			ctx := context.Background()
			field, transactionID := infra.leaveRebuildableLegacyEntry(t, ctx, tc)

			replay := NewRedisQueueConsumer(&libLog.GoLogger{}, infra.command, infra.query).
				WithQuarantineRepository(infra.quarantine).
				newLegacyBackupConsumer()

			lost := lostCleanupRedisRepository{RedisConsumerRepository: infra.redisRepo, dropped: make(chan string, 1)}
			infra.command.TransactionRedisRepo = lost
			replay.Consume(ctx)
			infra.awaitDroppedCleanup(t, lost)
			infra.command.TransactionRedisRepo = infra.redisRepo

			require.EqualValues(t, 2, infra.operationCount(t, transactionID), "the first replay writes both legs")
			infra.requireEntryCarriesPersistedOperations(t, ctx, field, transactionID)

			replay.Consume(ctx)
			infra.waitForEmptyLegacyBackupQueue(t, ctx)

			require.EqualValues(t, 2, infra.operationCount(t, transactionID), "a second replay of the same entry must not duplicate the legs")
			assert.Zero(t, infra.duplicatedLegCount(t, transactionID))
		})
	}
}

// TestIntegrationLegacyBackupReplayWritesNothingWhileRebuiltOperationsCannotBeStored
// replays a legacy entry without materialized operations while Redis refuses to
// store the rebuilt operations in it: nothing reaches PostgreSQL, the entry is
// kept exactly as read, and the outage does not count toward quarantine. Once
// Redis recovers, the next replay writes the transaction once.
func TestIntegrationLegacyBackupReplayWritesNothingWhileRebuiltOperationsCannotBeStored(t *testing.T) {
	if testing.Short() {
		t.Skip("requires PostgreSQL, MongoDB, Valkey, and RabbitMQ")
	}

	infra := setupRebuiltReplayIntegration(t)
	ctx := context.Background()
	field, transactionID := infra.leaveRebuildableLegacyEntry(t, ctx, rebuiltReplayCases[0])
	raw := infra.legacyEntryRaw(t, ctx, field)

	unavailable := replacementRedisRepository{
		RedisConsumerRepository: infra.redisRepo, failure: errors.New("redis unavailable"), attempts: &atomic.Int32{},
	}
	failing := NewRedisQueueConsumer(&libLog.GoLogger{}, infra.command, infra.query).WithQuarantineRepository(infra.quarantine)
	failing.queue = unavailable

	failing.newLegacyBackupConsumer().Consume(ctx)

	require.EqualValues(t, 1, unavailable.attempts.Load(), "the replay must try to store the rebuilt operations")
	infra.requireProjection(t, ctx, transactionID, 0, 0, 0)
	assert.Equal(t, raw, infra.legacyEntryRaw(t, ctx, field), "the entry must stay exactly as read")
	infra.requireNoBackupAttempts(t, ctx)

	NewRedisQueueConsumer(&libLog.GoLogger{}, infra.command, infra.query).
		WithQuarantineRepository(infra.quarantine).
		newLegacyBackupConsumer().
		Consume(ctx)
	infra.waitForEmptyLegacyBackupQueue(t, ctx)

	require.EqualValues(t, 2, infra.operationCount(t, transactionID))
	assert.Zero(t, infra.duplicatedLegCount(t, transactionID))
}

// TestIntegrationLegacyBackupReplayStoresRebuiltOperationsInTheTenantField
// replays a multi-tenant legacy entry, whose field in the tenant's backup hash
// is already tenant-scoped, and fails the metadata write after the commit. The
// rebuilt operations must replace that same field rather than a second,
// doubly scoped one, so the retry reuses them and the entry then leaves the
// queue.
func TestIntegrationLegacyBackupReplayStoresRebuiltOperationsInTheTenantField(t *testing.T) {
	if testing.Short() {
		t.Skip("requires PostgreSQL, MongoDB, Valkey, and RabbitMQ")
	}

	const tenantID = "tenant-rebuilt-replay"

	infra := setupRebuiltReplayIntegration(t)
	ctx := tmcore.ContextWithTenantID(context.Background(), tenantID)
	field, transactionID := infra.leaveRebuildableLegacyEntry(t, ctx, rebuiltReplayCases[0])
	require.True(t, strings.HasPrefix(field, "tenant:"+tenantID+":"), "the entry field must be tenant-scoped")

	tenantMetadata := transactionMongo.NewMetadataMongoDBRepository(nil, true)
	consumer := NewRedisQueueConsumer(&libLog.GoLogger{}, infra.command, infra.query).WithQuarantineRepository(infra.quarantine)
	consumer.multiTenantEnabled = true
	consumer.tenantMongo = staticTenantMongo{database: infra.mongo}
	replay := consumer.newLegacyBackupConsumer()

	infra.command.TransactionMetadataRepo = unavailableMetadataRepository{Repository: tenantMetadata}
	replay.Consume(ctx)
	infra.command.TransactionMetadataRepo = tenantMetadata

	require.EqualValues(t, 2, infra.operationCount(t, transactionID), "the first replay commits both legs")
	infra.requireEntryCarriesPersistedOperations(t, ctx, field, transactionID)
	infra.requireNoBackupAttempts(t, ctx)

	replay.Consume(ctx)
	infra.waitForEmptyLegacyBackupQueue(t, ctx)

	require.EqualValues(t, 2, infra.operationCount(t, transactionID))
	assert.Zero(t, infra.duplicatedLegCount(t, transactionID))
}

// TestIntegrationLegacyBackupReplayQuarantinesTheRewrittenRecordWhenTheWriteCanNeverSucceed
// replays a legacy entry without materialized operations whose own content
// PostgreSQL refuses (a NUL byte in the description), one attempt short of the
// quarantine threshold. The cycle stores the rebuilt operations in the entry
// before the write, so the quarantine must hold that rewritten entry and remove
// it; quarantining the payload read at the start of the cycle would leave the
// rewritten entry behind.
func TestIntegrationLegacyBackupReplayQuarantinesTheRewrittenRecordWhenTheWriteCanNeverSucceed(t *testing.T) {
	if testing.Short() {
		t.Skip("requires PostgreSQL, MongoDB, Valkey, and RabbitMQ")
	}

	infra := setupRebuiltReplayIntegration(t)
	ctx := context.Background()
	field, transactionID := infra.leaveRebuildableLegacyEntry(t, ctx, rebuiltReplayCases[1])
	infra.rewriteLegacyBackupEntry(t, ctx, func(entry *mmodel.TransactionRedisQueue) {
		entry.TransactionInput.Description = "transfer\x00"
	})

	attempts, err := tmvalkey.GetKeyContext(ctx, txRedis.TransactionBackupAttemptsQueue)
	require.NoError(t, err)
	require.NoError(t, infra.redis.HSet(ctx, attempts, field, QuarantineThreshold-1).Err())

	NewRedisQueueConsumer(&libLog.GoLogger{}, infra.command, infra.query).
		WithQuarantineRepository(infra.quarantine).
		newLegacyBackupConsumer().
		Consume(ctx)

	var payload []byte
	require.NoError(t, infra.db.QueryRow(
		`SELECT payload FROM transaction_backup_quarantine WHERE transaction_id = $1`, transactionID,
	).Scan(&payload))

	var quarantined mmodel.TransactionRedisQueue
	require.NoError(t, json.Unmarshal(payload, &quarantined))
	assert.Len(t, quarantined.Operations, 2, "the quarantine must hold the entry rewritten with its rebuilt operations")

	reason, counted := infra.requireSingleQuarantineRow(t, transactionID, field, string(payload))
	assert.Equal(t, "deterministic_write_failure", reason)
	assert.Equal(t, QuarantineThreshold, counted)
	assert.False(t, infra.redis.HExists(ctx, infra.legacyBackupQueueKey(t, ctx), field).Val(), "the quarantined entry must leave the backup hash")
	assert.Zero(t, infra.operationCount(t, transactionID))
}

// TestIntegrationLegacyBackupReplayOfMaterializedEntryIsIdempotent replays a
// legacy entry that already carries its operations twice: the second replay
// writes nothing new.
func TestIntegrationLegacyBackupReplayOfMaterializedEntryIsIdempotent(t *testing.T) {
	if testing.Short() {
		t.Skip("requires PostgreSQL, MongoDB, Valkey, and RabbitMQ")
	}

	infra := setupRebuiltReplayIntegration(t)
	ctx := context.Background()
	app := infra.newHTTPApp("")

	restoreTransactionRepo := infra.failTransactionWrites()
	transactionID := infra.leaveAnnotationInLegacyBackup(t, ctx, app)
	restoreTransactionRepo()

	field, entry := infra.requireSingleLegacyBackupEntry(t, ctx)
	require.NotEmpty(t, entry.Operations)
	raw := infra.legacyEntryRaw(t, ctx, field)

	counting := replacementRedisRepository{RedisConsumerRepository: infra.redisRepo, attempts: &atomic.Int32{}}
	consumer := NewRedisQueueConsumer(&libLog.GoLogger{}, infra.command, infra.query).WithQuarantineRepository(infra.quarantine)
	consumer.queue = counting
	replay := consumer.newLegacyBackupConsumer()

	replay.Consume(ctx)
	infra.waitForEmptyLegacyBackupQueue(t, ctx)
	infra.restoreLegacyEntry(t, ctx, field, raw)
	replay.Consume(ctx)
	infra.waitForEmptyLegacyBackupQueue(t, ctx)

	require.EqualValues(t, 2, infra.operationCount(t, transactionID))
	assert.Zero(t, infra.duplicatedLegCount(t, transactionID))
	assert.Zero(t, counting.attempts.Load(), "an entry with operations must be replayed as recorded, not rewritten")
}

func setupRebuiltReplayIntegration(t *testing.T) *engineWriteBehindHTTPIntegration {
	t.Helper()

	t.Setenv("ALLOW_INSECURE_TLS", "true")
	t.Setenv("AUDIT_LOG_ENABLED", "false")
	t.Setenv("RABBITMQ_TRANSACTION_ASYNC", "true")

	infra := setupEngineWriteBehindHTTPIntegration(t)
	infra.command.RabbitMQRepo = &recordingBalanceOperationProducer{}

	return infra
}

// leaveRebuildableLegacyEntry leaves one aged legacy entry in the backup queue
// without materialized operations but with a balance snapshot per leg, so the
// replay rebuilds its operations, and returns its field and transaction ID. A
// pending case carries the validation of that lifecycle stage.
func (infra *engineWriteBehindHTTPIntegration) leaveRebuildableLegacyEntry(tb testing.TB, ctx context.Context, tc rebuiltReplayCase) (string, uuid.UUID) {
	tb.Helper()
	t := tb

	app := infra.newHTTPApp(tmcore.GetTenantIDContext(ctx))

	restoreTransactionRepo := infra.failTransactionWrites()
	transactionID := infra.leaveAnnotationInLegacyBackup(t, ctx, app)
	restoreTransactionRepo()

	field := infra.rewriteLegacyBackupEntry(t, ctx, func(entry *mmodel.TransactionRedisQueue) {
		entry.Operations = nil
		entry.Balances = infra.legBalanceSnapshots(t, entry.TransactionInput)
		entry.TransactionStatus = tc.status

		if tc.pending {
			infra.validateAsLifecycleStage(t, ctx, entry, tc.status)
		}
	})

	return field, transactionID
}

// validateAsLifecycleStage gives entry the status, action and validation a
// pending transaction's stage carries, computed as the create path does: the
// annotation the harness starts from validates every leg as a plain movement.
func (infra *engineWriteBehindHTTPIntegration) validateAsLifecycleStage(tb testing.TB, ctx context.Context, entry *mmodel.TransactionRedisQueue, status string) {
	tb.Helper()

	entry.TransactionInput.Pending = true
	input := entry.TransactionInput
	input.Send.Source.From = slices.Clone(input.Send.Source.From)
	input.Send.Distribute.To = slices.Clone(input.Send.Distribute.To)

	for i := range input.Send.Source.From {
		input.Send.Source.From[i].IsFrom = true
	}

	mtransaction.ApplyDefaultBalanceKeys(input.Send.Source.From)
	mtransaction.ApplyDefaultBalanceKeys(input.Send.Distribute.To)
	mtransaction.MutateConcatAliases(input.Send.Source.From)
	mtransaction.MutateConcatAliases(input.Send.Distribute.To)

	validate, err := mtransaction.ValidateSendSourceAndDistribute(ctx, input, status)
	require.NoError(tb, err)

	validate.Pending = true
	entry.Validate = validate
	entry.TransactionStatus = status
	entry.Action = mtransaction.StatusToAction(status)
}

func (infra *engineWriteBehindHTTPIntegration) legacyEntryRaw(tb testing.TB, ctx context.Context, field string) string {
	tb.Helper()

	queue, err := tmvalkey.GetKeyContext(ctx, txRedis.TransactionBackupQueue)
	require.NoError(tb, err)
	raw, err := infra.redis.HGet(ctx, queue, field).Result()
	require.NoError(tb, err)

	return raw
}

func (infra *engineWriteBehindHTTPIntegration) restoreLegacyEntry(tb testing.TB, ctx context.Context, field, raw string) {
	tb.Helper()

	queue, err := tmvalkey.GetKeyContext(ctx, txRedis.TransactionBackupQueue)
	require.NoError(tb, err)
	require.NoError(tb, infra.redis.HSet(ctx, queue, field, raw).Err())
}

// awaitDroppedCleanup waits until the write path has attempted, and lost, the
// entry removal it runs off the caller's goroutine. Receiving the signal also
// orders that goroutine's repository read before the caller restores it.
func (infra *engineWriteBehindHTTPIntegration) awaitDroppedCleanup(tb testing.TB, lost lostCleanupRedisRepository) {
	tb.Helper()

	select {
	case <-lost.dropped:
	case <-time.After(5 * time.Second):
		tb.Fatal("the write path must attempt to remove the replayed entry")
	}
}

// requireEntryCarriesPersistedOperations asserts the single legacy entry still
// sits at field, still selects the legacy replay path, and carries exactly the
// operation IDs PostgreSQL holds for the transaction.
func (infra *engineWriteBehindHTTPIntegration) requireEntryCarriesPersistedOperations(tb testing.TB, ctx context.Context, field string, transactionID uuid.UUID) {
	tb.Helper()
	t := tb

	entryField, entry := infra.requireSingleLegacyBackupEntry(t, ctx)
	assert.Equal(t, field, entryField, "the entry must stay at the field it was read from")

	var fields map[string]json.RawMessage
	require.NoError(t, json.Unmarshal([]byte(infra.legacyEntryRaw(t, ctx, field)), &fields))
	assert.NotContains(t, fields, "formatVersion", "the entry must keep selecting the legacy replay path")

	recorded := make([]string, 0, len(entry.Operations))
	for _, op := range entry.Operations {
		recorded = append(recorded, op.ID)
	}

	slices.Sort(recorded)
	assert.Equal(t, infra.operationIDs(t, transactionID), recorded, "the entry must carry the operation IDs the replay wrote")
}

func (infra *engineWriteBehindHTTPIntegration) operationIDs(tb testing.TB, transactionID uuid.UUID) []string {
	tb.Helper()

	rows, err := infra.db.Query(`SELECT id FROM operation WHERE transaction_id = $1 ORDER BY id`, transactionID)
	require.NoError(tb, err)

	defer rows.Close()

	ids := []string{}

	for rows.Next() {
		var id string
		require.NoError(tb, rows.Scan(&id))
		ids = append(ids, id)
	}

	require.NoError(tb, rows.Err())

	return ids
}

func (infra *engineWriteBehindHTTPIntegration) operationCount(tb testing.TB, transactionID uuid.UUID) int64 {
	tb.Helper()

	var count int64
	require.NoError(tb, infra.db.QueryRow(`SELECT COUNT(*) FROM operation WHERE transaction_id = $1`, transactionID).Scan(&count))

	return count
}

// duplicatedLegCount counts the legs persisted more than once: the same balance,
// type and starting version recorded under different operation IDs.
func (infra *engineWriteBehindHTTPIntegration) duplicatedLegCount(tb testing.TB, transactionID uuid.UUID) int64 {
	tb.Helper()

	var count int64
	require.NoError(tb, infra.db.QueryRow(`SELECT COUNT(*) FROM (
		SELECT 1 FROM operation WHERE transaction_id = $1
		GROUP BY balance_id, type, balance_version_before HAVING COUNT(*) > 1) duplicated`, transactionID).Scan(&count))

	return count
}
