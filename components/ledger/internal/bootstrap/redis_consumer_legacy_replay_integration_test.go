//go:build integration

// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"runtime"
	"strings"
	"testing"
	"time"

	tmcore "github.com/LerianStudio/lib-commons/v7/commons/tenant-manager/core"
	tmvalkey "github.com/LerianStudio/lib-commons/v7/commons/tenant-manager/valkey"
	libLog "github.com/LerianStudio/lib-observability/v4/log"
	"github.com/gofiber/fiber/v3"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/v2/mongo"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"

	transactionMongo "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/mongodb/transaction"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/transaction"
	txRedis "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/redis/transaction"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	"github.com/LerianStudio/midaz/v4/pkg/mtransaction"
	"github.com/LerianStudio/midaz/v4/pkg/repository"
	"github.com/LerianStudio/midaz/v4/pkg/utils"
)

// unavailableTransactionRepository refuses to open the database transaction
// every transaction write runs in, the shape of a PostgreSQL outage at the
// moment an annotation is written.
type unavailableTransactionRepository struct {
	transaction.Repository
}

func (unavailableTransactionRepository) BeginTx(context.Context) (repository.DBTransaction, error) {
	return nil, errors.New("transaction database unavailable")
}

// staticTenantMongo resolves every tenant to one database, or fails with err.
type staticTenantMongo struct {
	database *mongo.Database
	err      error
}

func (resolver staticTenantMongo) GetDatabaseForTenant(context.Context, string) (*mongo.Database, error) {
	return resolver.database, resolver.err
}

// TestIntegrationLegacyBackupReplayPersistsAnnotationWithoutPublishing leaves an
// annotation in the legacy backup queue after its database write fails, then
// replays it: a replay that still cannot write keeps the entry, and the next one
// writes the transaction directly and removes the entry, without publishing.
func TestIntegrationLegacyBackupReplayPersistsAnnotationWithoutPublishing(t *testing.T) {
	if testing.Short() {
		t.Skip("requires PostgreSQL, MongoDB, Valkey, and RabbitMQ")
	}

	t.Setenv("ALLOW_INSECURE_TLS", "true")
	t.Setenv("AUDIT_LOG_ENABLED", "false")
	t.Setenv("RABBITMQ_TRANSACTION_ASYNC", "true")

	infra := setupEngineWriteBehindHTTPIntegration(t)
	producer := &recordingBalanceOperationProducer{}
	infra.command.RabbitMQRepo = producer
	app := infra.newHTTPApp("")
	ctx := context.Background()

	restoreTransactionRepo := infra.failTransactionWrites()
	transactionID := infra.leaveAnnotationInLegacyBackup(t, ctx, app)

	// Until the replay writes it, the annotation lives only in its backup and
	// write-behind entries, and must still read and refuse transitions as a
	// persisted one.
	infra.requireAnnotationServed(t, app, transactionID)
	infra.requireAnnotationTransitionsRefused(t, app, transactionID)
	infra.requireProjection(t, ctx, transactionID, 0, 0, 0)
	assert.Zero(t, len(producer.messages), "a failed annotation write must not publish")

	replay := NewRedisQueueConsumer(&libLog.GoLogger{}, infra.command, infra.query).
		WithQuarantineRepository(infra.quarantine).
		newLegacyBackupConsumer()

	stats := replay.Consume(ctx)
	require.Equal(t, 1, stats.messageCount)
	require.Zero(t, stats.tooYoung)
	infra.requireProjection(t, ctx, transactionID, 0, 0, 0)
	infra.requireSingleLegacyBackupEntry(t, ctx)
	infra.requireNoBackupAttempts(t, ctx)
	assert.Zero(t, len(producer.messages), "a replay that cannot write must not publish")

	restoreTransactionRepo()

	stats = replay.Consume(ctx)
	require.Equal(t, 1, stats.messageCount)
	infra.requireProjection(t, ctx, transactionID, 1, 2, 1)
	infra.requireNotedRow(t, transactionID)
	assert.Zero(t, len(producer.messages), "the replay must write the annotation, not queue it")

	infra.waitForEmptyLegacyBackupQueue(t, ctx)
}

// TestIntegrationLegacyBackupReplayResolvesTenantMongo replays a legacy
// annotation in the context the multi-tenant cycle builds, which carries the
// tenant but no Mongo database, against a metadata repository that requires the
// tenant database: the replay must resolve it before writing.
func TestIntegrationLegacyBackupReplayResolvesTenantMongo(t *testing.T) {
	if testing.Short() {
		t.Skip("requires PostgreSQL, MongoDB, Valkey, and RabbitMQ")
	}

	t.Setenv("ALLOW_INSECURE_TLS", "true")
	t.Setenv("AUDIT_LOG_ENABLED", "false")
	t.Setenv("RABBITMQ_TRANSACTION_ASYNC", "true")

	const tenantID = "tenant-replay"

	for _, testCase := range []struct {
		name     string
		resolver staticTenantMongo
		written  bool
	}{
		{name: "writes with the resolved tenant database", written: true},
		{name: "keeps the entry when the tenant database cannot be resolved", resolver: staticTenantMongo{err: errors.New("tenant mongo unavailable")}},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			infra := setupEngineWriteBehindHTTPIntegration(t)
			producer := &recordingBalanceOperationProducer{}
			infra.command.RabbitMQRepo = producer
			app := infra.newHTTPApp(tenantID)
			ctx := tmcore.ContextWithTenantID(context.Background(), tenantID)

			restoreTransactionRepo := infra.failTransactionWrites()
			transactionID := infra.leaveAnnotationInLegacyBackup(t, ctx, app)
			restoreTransactionRepo()

			infra.command.TransactionMetadataRepo = transactionMongo.NewMetadataMongoDBRepository(nil, true)
			resolver := testCase.resolver
			if testCase.written {
				resolver.database = infra.mongo
			}

			consumer := NewRedisQueueConsumer(&libLog.GoLogger{}, infra.command, infra.query).
				WithQuarantineRepository(infra.quarantine)
			consumer.multiTenantEnabled = true
			consumer.tenantMongo = resolver

			stats := consumer.newLegacyBackupConsumer().Consume(ctx)
			require.Equal(t, 1, stats.messageCount)
			assert.Zero(t, len(producer.messages), "the replay must not publish")

			if !testCase.written {
				infra.requireProjection(t, ctx, transactionID, 0, 0, 0)
				infra.requireSingleLegacyBackupEntry(t, ctx)
				infra.requireNoBackupAttempts(t, ctx)

				return
			}

			infra.requireProjection(t, ctx, transactionID, 1, 2, 1)
			infra.requireNotedRow(t, transactionID)
			infra.waitForEmptyLegacyBackupQueue(t, ctx)
		})
	}
}

// TestIntegrationLegacyBackupReplayQuarantinesARecordItCanNeverWrite replays a
// legacy annotation whose own content PostgreSQL refuses (a NUL byte in the
// description, SQLSTATE 22021): each cycle counts toward quarantine, and at the
// threshold the record lands in the durable quarantine table and leaves the
// backup queue instead of failing every cycle forever.
func TestIntegrationLegacyBackupReplayQuarantinesARecordItCanNeverWrite(t *testing.T) {
	if testing.Short() {
		t.Skip("requires PostgreSQL, MongoDB, Valkey, and RabbitMQ")
	}

	t.Setenv("ALLOW_INSECURE_TLS", "true")
	t.Setenv("AUDIT_LOG_ENABLED", "false")
	t.Setenv("RABBITMQ_TRANSACTION_ASYNC", "true")

	infra := setupEngineWriteBehindHTTPIntegration(t)
	producer := &recordingBalanceOperationProducer{}
	infra.command.RabbitMQRepo = producer
	app := infra.newHTTPApp("")
	ctx := context.Background()

	restoreTransactionRepo := infra.failTransactionWrites()
	transactionID := infra.leaveAnnotationInLegacyBackup(t, ctx, app)
	restoreTransactionRepo()

	field := infra.rewriteLegacyBackupEntry(t, ctx, func(entry *mmodel.TransactionRedisQueue) {
		entry.TransactionInput.Description = "annotation\x00"
	})

	replay := NewRedisQueueConsumer(&libLog.GoLogger{}, infra.command, infra.query).
		WithQuarantineRepository(infra.quarantine).
		newLegacyBackupConsumer()

	infra.requireQuarantinedAtThreshold(t, ctx, replay, field, transactionID, "deterministic_write_failure")
	assert.Zero(t, len(producer.messages), "the replay must not publish")
}

// TestIntegrationLegacyBackupReplayQuarantinesARebuildWithoutOperations replays
// a legacy entry with neither materialized operations nor balance snapshots: an
// annotation seeded before its operations were built, or a 4.0.x create, whose
// rebuild yields no operation. No transaction row is written; each cycle counts
// toward quarantine, and at the threshold the entry moves to the quarantine
// table under its own reason.
func TestIntegrationLegacyBackupReplayQuarantinesARebuildWithoutOperations(t *testing.T) {
	if testing.Short() {
		t.Skip("requires PostgreSQL, MongoDB, Valkey, and RabbitMQ")
	}

	t.Setenv("ALLOW_INSECURE_TLS", "true")
	t.Setenv("AUDIT_LOG_ENABLED", "false")
	t.Setenv("RABBITMQ_TRANSACTION_ASYNC", "true")

	for _, status := range []string{constant.NOTED, constant.APPROVED} {
		t.Run(status, func(t *testing.T) {
			infra := setupEngineWriteBehindHTTPIntegration(t)
			producer := &recordingBalanceOperationProducer{}
			infra.command.RabbitMQRepo = producer
			app := infra.newHTTPApp("")
			ctx := context.Background()

			restoreTransactionRepo := infra.failTransactionWrites()
			transactionID := infra.leaveAnnotationInLegacyBackup(t, ctx, app)
			restoreTransactionRepo()

			field := infra.rewriteLegacyBackupEntry(t, ctx, func(entry *mmodel.TransactionRedisQueue) {
				entry.Operations = nil
				entry.Balances = nil
				entry.TransactionStatus = status
			})

			reader, factory := newBalanceSyncReaderFactory(t)
			replay := NewRedisQueueConsumer(&libLog.GoLogger{}, infra.command, infra.query).
				WithQuarantineRepository(infra.quarantine).
				WithMetricsFactory(factory).
				newLegacyBackupConsumer()

			infra.requireQuarantinedAtThreshold(t, ctx, replay, field, transactionID, "empty_rebuilt_operations")
			assert.Zero(t, counterTotal(t, reader, utils.RedisBackupReplayRecomputedBalancesAfterTotal.Name),
				"a rebuild that yields no operation must not count as a recomputed replay")
			assert.Zero(t, len(producer.messages), "the replay must not publish")
		})
	}
}

// TestIntegrationLegacyBackupReplayRebuildsOperationsFromBalanceSnapshots
// replays a legacy entry without materialized operations whose balance
// snapshots match its legs, the shape of a 4.0.x commit: the operations are
// rebuilt from the snapshots and written with the transaction, and the rebuild
// is counted as one whose after-balances were recomputed.
func TestIntegrationLegacyBackupReplayRebuildsOperationsFromBalanceSnapshots(t *testing.T) {
	if testing.Short() {
		t.Skip("requires PostgreSQL, MongoDB, Valkey, and RabbitMQ")
	}

	t.Setenv("ALLOW_INSECURE_TLS", "true")
	t.Setenv("AUDIT_LOG_ENABLED", "false")
	t.Setenv("RABBITMQ_TRANSACTION_ASYNC", "true")

	infra := setupEngineWriteBehindHTTPIntegration(t)
	producer := &recordingBalanceOperationProducer{}
	infra.command.RabbitMQRepo = producer
	app := infra.newHTTPApp("")
	ctx := context.Background()

	restoreTransactionRepo := infra.failTransactionWrites()
	transactionID := infra.leaveAnnotationInLegacyBackup(t, ctx, app)
	restoreTransactionRepo()

	infra.rewriteLegacyBackupEntry(t, ctx, func(entry *mmodel.TransactionRedisQueue) {
		entry.Operations = nil
		entry.Balances = infra.legBalanceSnapshots(t, entry.TransactionInput)
		entry.TransactionStatus = constant.APPROVED
		entry.Validate.Pending = true
	})

	reader, factory := newBalanceSyncReaderFactory(t)
	replay := NewRedisQueueConsumer(&libLog.GoLogger{}, infra.command, infra.query).
		WithQuarantineRepository(infra.quarantine).
		WithMetricsFactory(factory).
		newLegacyBackupConsumer()

	stats := replay.Consume(ctx)
	require.Equal(t, 1, stats.messageCount)
	infra.requireProjection(t, ctx, transactionID, 1, 2, 1)
	infra.requireNoBackupAttempts(t, ctx)
	assert.Equal(t, int64(1), counterTotal(t, reader, utils.RedisBackupReplayRecomputedBalancesAfterTotal.Name),
		"a rebuild from balance snapshots must stay observable")
	assert.Zero(t, len(producer.messages), "the replay must write the transaction, not queue it")

	infra.waitForEmptyLegacyBackupQueue(t, ctx)
}

// requireQuarantinedAtThreshold runs the replay until the single legacy entry
// reaches the quarantine threshold. Each cycle below it must keep the entry and
// count one attempt; the cycle at the threshold must move the entry to the
// quarantine table under reason and clear its attempts. No cycle may write the
// transaction.
func (infra *engineWriteBehindHTTPIntegration) requireQuarantinedAtThreshold(
	tb testing.TB, ctx context.Context, replay *LegacyBackupConsumer, field string, transactionID uuid.UUID, reason string,
) {
	tb.Helper()
	t := tb

	attemptsKey, err := tmvalkey.GetKeyContext(ctx, txRedis.TransactionBackupAttemptsQueue)
	require.NoError(t, err)

	for cycle := int64(1); cycle < QuarantineThreshold; cycle++ {
		stats := replay.Consume(ctx)
		require.Equal(t, 1, stats.messageCount)
		infra.requireSingleLegacyBackupEntry(t, ctx)
		infra.requireProjection(t, ctx, transactionID, 0, 0, 0)

		attempts, err := infra.redis.HGet(ctx, attemptsKey, field).Int64()
		require.NoError(t, err, "a record the replay can never write must count toward quarantine")
		assert.Equal(t, cycle, attempts)
	}

	stats := replay.Consume(ctx)
	require.Equal(t, 1, stats.messageCount)

	var (
		quarantinedTransaction uuid.UUID
		failureReason          string
		quarantinedAttempts    int
	)

	require.NoError(t, infra.db.QueryRow(
		`SELECT transaction_id, failure_reason, attempts FROM transaction_backup_quarantine WHERE redis_key = $1`, field,
	).Scan(&quarantinedTransaction, &failureReason, &quarantinedAttempts))
	assert.Equal(t, transactionID, quarantinedTransaction)
	assert.Equal(t, reason, failureReason)
	assert.Equal(t, QuarantineThreshold, quarantinedAttempts)

	messages, err := readRecoveryMessages(ctx, infra.redisRepo, txRedis.RecoveryQueueSourceLegacyBackup)
	require.NoError(t, err)
	assert.Empty(t, messages, "a quarantined record must leave the backup queue")
	assert.Zero(t, infra.redis.HLen(ctx, attemptsKey).Val(), "quarantine must clear the attempts counter")
	infra.requireProjection(t, ctx, transactionID, 0, 0, 0)
}

// legBalanceSnapshots reads the persisted balance of every leg of input and
// returns it in the form a backup entry carries balance snapshots: one per leg,
// keyed by the leg's indexed alias so the replay matches it to that leg.
func (infra *engineWriteBehindHTTPIntegration) legBalanceSnapshots(tb testing.TB, input mtransaction.Transaction) []mmodel.BalanceRedis {
	tb.Helper()
	t := tb

	var snapshots []mmodel.BalanceRedis

	for _, legs := range [][]mtransaction.FromTo{input.Send.Source.From, input.Send.Distribute.To} {
		for index, leg := range legs {
			snapshot := mmodel.BalanceRedis{Alias: leg.ConcatAlias(index), Key: constant.DefaultBalanceKey, AllowSending: 1, AllowReceiving: 1}

			var available, onHold string

			require.NoError(t, infra.db.QueryRow(
				`SELECT id, account_id, asset_code, available::text, on_hold::text, version, account_type FROM balance
				 WHERE organization_id = $1 AND ledger_id = $2 AND alias = $3 AND key = $4 AND deleted_at IS NULL`,
				infra.organization, infra.ledger, leg.AccountAlias, constant.DefaultBalanceKey,
			).Scan(&snapshot.ID, &snapshot.AccountID, &snapshot.AssetCode, &available, &onHold, &snapshot.Version, &snapshot.AccountType))

			snapshot.Available = decimal.RequireFromString(available)
			snapshot.OnHold = decimal.RequireFromString(onHold)
			snapshots = append(snapshots, snapshot)
		}
	}

	require.Len(t, snapshots, 2, "the annotation must carry one source and one destination leg")

	return snapshots
}

// counterTotal sums the counter named name across all label sets.
func counterTotal(t *testing.T, reader *sdkmetric.ManualReader, name string) int64 {
	t.Helper()

	var rm metricdata.ResourceMetrics

	require.NoError(t, reader.Collect(context.Background(), &rm))

	var total int64

	for _, sm := range rm.ScopeMetrics {
		for _, m := range sm.Metrics {
			if m.Name != name {
				continue
			}

			sum, ok := m.Data.(metricdata.Sum[int64])
			require.Truef(t, ok, "%s data type must be Sum[int64], got %T", name, m.Data)

			for _, dp := range sum.DataPoints {
				total += dp.Value
			}
		}
	}

	return total
}

// failTransactionWrites makes every transaction write of the command use case
// fail until the returned function restores the real repository.
func (infra *engineWriteBehindHTTPIntegration) failTransactionWrites() func() {
	available := infra.command.TransactionRepo
	infra.command.TransactionRepo = unavailableTransactionRepository{Repository: available}

	return func() { infra.command.TransactionRepo = available }
}

// leaveAnnotationInLegacyBackup creates an annotation whose database write
// fails, so its only copy is the legacy backup entry, and ages that entry past
// MessageTimeOfLife so the next cycle replays it.
func (infra *engineWriteBehindHTTPIntegration) leaveAnnotationInLegacyBackup(tb testing.TB, ctx context.Context, app *fiber.App) uuid.UUID {
	tb.Helper()
	t := tb

	aliases := infra.seedTransfer(t, "replay-"+strings.ReplaceAll(uuid.NewString(), "-", "")[:8])
	created := infra.postAnnotationCreate(t, app, aliases)
	require.Equalf(t, http.StatusInternalServerError, created.status, "a failed annotation write must not answer 201: %s", created.body)
	assert.Equal(t, constant.ErrMessageBrokerUnavailable.Error(), created.decoded["code"])

	_, entry := infra.requireSingleLegacyBackupEntry(t, ctx)
	require.Equal(t, constant.NOTED, entry.TransactionStatus)

	infra.rewriteLegacyBackupEntry(t, ctx, func(entry *mmodel.TransactionRedisQueue) {
		entry.TTL = time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC)
	})

	return entry.TransactionID
}

// rewriteLegacyBackupEntry applies change to the single legacy backup entry and
// returns its field. The field read back is already tenant-scoped, so the entry
// is rewritten in place rather than through AddMessageToQueue, which scopes its
// key again.
func (infra *engineWriteBehindHTTPIntegration) rewriteLegacyBackupEntry(tb testing.TB, ctx context.Context, change func(*mmodel.TransactionRedisQueue)) string {
	tb.Helper()
	t := tb

	field, entry := infra.requireSingleLegacyBackupEntry(t, ctx)
	change(&entry)

	rewritten, err := json.Marshal(entry)
	require.NoError(t, err)
	queue, err := tmvalkey.GetKeyContext(ctx, txRedis.TransactionBackupQueue)
	require.NoError(t, err)
	require.NoError(t, infra.redis.HSet(ctx, queue, field, rewritten).Err())

	return field
}

func (infra *engineWriteBehindHTTPIntegration) requireNotedRow(tb testing.TB, transactionID uuid.UUID) {
	tb.Helper()
	t := tb

	var projected string
	require.NoError(t, infra.db.QueryRow(`SELECT status FROM transaction WHERE id = $1`, transactionID).Scan(&projected))
	assert.Equal(t, constant.NOTED, projected)
}

func (infra *engineWriteBehindHTTPIntegration) requireSingleLegacyBackupEntry(tb testing.TB, ctx context.Context) (string, mmodel.TransactionRedisQueue) {
	tb.Helper()
	t := tb
	messages, err := readRecoveryMessages(ctx, infra.redisRepo, txRedis.RecoveryQueueSourceLegacyBackup)
	require.NoError(t, err)
	require.Len(t, messages, 1, "the legacy backup queue must hold exactly the annotation entry")

	for field, raw := range messages {
		var entry mmodel.TransactionRedisQueue
		require.NoError(t, json.Unmarshal([]byte(raw), &entry))

		return field, entry
	}

	return "", mmodel.TransactionRedisQueue{}
}

// requireNoBackupAttempts asserts that no record counts toward quarantine: a
// write or tenant-resolution failure is infrastructure, not a poison record, and
// must not push a sound record out of the replay during a long outage.
func (infra *engineWriteBehindHTTPIntegration) requireNoBackupAttempts(tb testing.TB, ctx context.Context) {
	tb.Helper()
	t := tb

	attempts, err := tmvalkey.GetKeyContext(ctx, txRedis.TransactionBackupAttemptsQueue)
	require.NoError(t, err)
	assert.Zero(t, infra.redis.HLen(ctx, attempts).Val(), "a failed replay write must not count toward quarantine")
}

// waitForEmptyLegacyBackupQueue waits for the entry removal that follows a
// successful write; it runs off the caller's goroutine.
func (infra *engineWriteBehindHTTPIntegration) waitForEmptyLegacyBackupQueue(tb testing.TB, ctx context.Context) {
	tb.Helper()
	t := tb
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		messages, err := readRecoveryMessages(ctx, infra.redisRepo, txRedis.RecoveryQueueSourceLegacyBackup)
		require.NoError(t, err)
		if len(messages) == 0 {
			return
		}

		runtime.Gosched()
	}

	t.Fatal("the replayed annotation must leave the legacy backup queue")
}
