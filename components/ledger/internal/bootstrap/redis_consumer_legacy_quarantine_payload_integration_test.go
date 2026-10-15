//go:build integration

// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"context"
	"encoding/json"
	"errors"
	"sync/atomic"
	"testing"

	tmcore "github.com/LerianStudio/lib-commons/v7/commons/tenant-manager/core"
	tmvalkey "github.com/LerianStudio/lib-commons/v7/commons/tenant-manager/valkey"
	libLog "github.com/LerianStudio/lib-observability/v4/log"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	txRedis "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/redis/transaction"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
)

// flakyRemovalRedisRepository fails the first quarantine removal, the shape of
// a Redis failure between the durable quarantine insert and the removal.
type flakyRemovalRedisRepository struct {
	*txRedis.RedisConsumerRepository
	failed *atomic.Bool
}

func (repo flakyRemovalRedisRepository) CompareAndDeleteBackupRecord(ctx context.Context, field, expected string) (int64, error) {
	if repo.failed.CompareAndSwap(false, true) {
		return txRedis.RecoveryAckMissing, errors.New("redis unavailable")
	}

	return repo.RedisConsumerRepository.CompareAndDeleteBackupRecord(ctx, field, expected)
}

// TestIntegrationLegacyBackupReplayQuarantineKeepsEveryDistinctPayloadOfAField
// quarantines a legacy record, then lets a different record land in the same
// field and reach the quarantine threshold too, as a commit written over a
// quarantined hold does. Both payloads must be durably quarantined before the
// field leaves the backup hash, in single- and multi-tenant mode.
func TestIntegrationLegacyBackupReplayQuarantineKeepsEveryDistinctPayloadOfAField(t *testing.T) {
	if testing.Short() {
		t.Skip("requires PostgreSQL, MongoDB, Valkey, and RabbitMQ")
	}

	for _, tenantID := range []string{"", "tenant-quarantine-payloads"} {
		name := "single tenant"
		if tenantID != "" {
			name = "multi tenant"
		}

		t.Run(name, func(t *testing.T) {
			infra, ctx, replay, field, transactionID := leaveUnwritableLegacyRecord(t, tenantID, nil)

			first := infra.legacyEntryAt(t, ctx, field)
			infra.consumeToQuarantineThreshold(ctx, replay)
			require.False(t, infra.redis.HExists(ctx, infra.backupHash(t, ctx), field).Val(), "the first payload leaves the hash once quarantined")

			var entry mmodel.TransactionRedisQueue
			require.NoError(t, json.Unmarshal([]byte(first), &entry))
			entry.HeaderID = uuid.NewString()
			second, err := json.Marshal(entry)
			require.NoError(t, err)
			require.NoError(t, infra.redis.HSet(ctx, infra.backupHash(t, ctx), field, second).Err())

			infra.consumeToQuarantineThreshold(ctx, replay)

			assert.ElementsMatch(t, []string{first, string(second)}, infra.quarantinedPayloads(t, transactionID),
				"every distinct payload of the field must be durably quarantined")
			assert.False(t, infra.redis.HExists(ctx, infra.backupHash(t, ctx), field).Val(), "the second payload leaves the hash once quarantined")
		})
	}
}

// TestIntegrationLegacyBackupReplayQuarantineOfTheSamePayloadIsIdempotent
// quarantines a legacy record whose removal fails after the durable insert:
// the next cycle quarantines the same payload again, which lands once, and then
// removes the record.
func TestIntegrationLegacyBackupReplayQuarantineOfTheSamePayloadIsIdempotent(t *testing.T) {
	if testing.Short() {
		t.Skip("requires PostgreSQL, MongoDB, Valkey, and RabbitMQ")
	}

	failed := &atomic.Bool{}
	infra, ctx, replay, field, transactionID := leaveUnwritableLegacyRecord(t, "", failed)
	payload := infra.legacyEntryAt(t, ctx, field)

	infra.consumeToQuarantineThreshold(ctx, replay)
	require.True(t, failed.Load(), "the first removal must fail")
	require.True(t, infra.redis.HExists(ctx, infra.backupHash(t, ctx), field).Val(), "a failed removal keeps the record")

	replay.Consume(ctx)

	assert.Equal(t, []string{payload}, infra.quarantinedPayloads(t, transactionID), "the same payload lands once")
	assert.False(t, infra.redis.HExists(ctx, infra.backupHash(t, ctx), field).Val())

	attempts, err := tmvalkey.GetKeyContext(ctx, txRedis.TransactionBackupAttemptsQueue)
	require.NoError(t, err)
	assert.Zero(t, infra.redis.HLen(ctx, attempts).Val())
}

// leaveUnwritableLegacyRecord leaves one legacy record the replay can never write (no
// Validate) in the backup hash, scoped to tenantID when it is not empty, and
// returns a replay wired for that scope. A non-nil failed makes the first
// quarantine removal fail.
func leaveUnwritableLegacyRecord(t *testing.T, tenantID string, failed *atomic.Bool) (*engineWriteBehindHTTPIntegration, context.Context, *LegacyBackupConsumer, string, uuid.UUID) {
	t.Helper()

	t.Setenv("ALLOW_INSECURE_TLS", "true")
	t.Setenv("AUDIT_LOG_ENABLED", "false")
	t.Setenv("RABBITMQ_TRANSACTION_ASYNC", "true")

	infra := setupEngineWriteBehindHTTPIntegration(t)
	infra.command.RabbitMQRepo = &recordingBalanceOperationProducer{}

	ctx := context.Background()
	if tenantID != "" {
		ctx = tmcore.ContextWithTenantID(ctx, tenantID)
	}

	restoreTransactionRepo := infra.failTransactionWrites()
	transactionID := infra.leaveAnnotationInLegacyBackup(t, ctx, infra.newHTTPApp(tenantID))
	restoreTransactionRepo()

	field := infra.rewriteLegacyBackupEntry(t, ctx, func(entry *mmodel.TransactionRedisQueue) {
		entry.Validate = nil
	})

	consumer := NewRedisQueueConsumer(&libLog.GoLogger{}, infra.command, infra.query).WithQuarantineRepository(infra.quarantine)
	if tenantID != "" {
		consumer.multiTenantEnabled = true
		consumer.tenantMongo = staticTenantMongo{database: infra.mongo}
	}

	if failed != nil {
		consumer.queue = flakyRemovalRedisRepository{RedisConsumerRepository: infra.redisRepo, failed: failed}
	}

	return infra, ctx, consumer.newLegacyBackupConsumer(), field, transactionID
}

func (infra *engineWriteBehindHTTPIntegration) consumeToQuarantineThreshold(ctx context.Context, replay *LegacyBackupConsumer) {
	for cycle := 0; cycle < QuarantineThreshold; cycle++ {
		replay.Consume(ctx)
	}
}

func (infra *engineWriteBehindHTTPIntegration) backupHash(tb testing.TB, ctx context.Context) string {
	tb.Helper()

	hash, err := tmvalkey.GetKeyContext(ctx, txRedis.TransactionBackupQueue)
	require.NoError(tb, err)

	return hash
}

func (infra *engineWriteBehindHTTPIntegration) legacyEntryAt(tb testing.TB, ctx context.Context, field string) string {
	tb.Helper()

	raw, err := infra.redis.HGet(ctx, infra.backupHash(tb, ctx), field).Result()
	require.NoError(tb, err)

	return raw
}

func (infra *engineWriteBehindHTTPIntegration) quarantinedPayloads(tb testing.TB, transactionID uuid.UUID) []string {
	tb.Helper()

	rows, err := infra.db.Query(`SELECT payload FROM transaction_backup_quarantine WHERE transaction_id = $1`, transactionID)
	require.NoError(tb, err)

	defer rows.Close()

	payloads := []string{}

	for rows.Next() {
		var payload []byte
		require.NoError(tb, rows.Scan(&payload))
		payloads = append(payloads, string(payload))
	}

	require.NoError(tb, rows.Err())

	return payloads
}
