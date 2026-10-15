//go:build integration

// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"context"
	"strings"
	"testing"

	tmcore "github.com/LerianStudio/lib-commons/v7/commons/tenant-manager/core"
	tmvalkey "github.com/LerianStudio/lib-commons/v7/commons/tenant-manager/valkey"
	libLog "github.com/LerianStudio/lib-observability/v4/log"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	txRedis "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/redis/transaction"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
)

// TestIntegrationLegacyBackupReplayQuarantineRemovesTheTenantRecord replays a
// multi-tenant legacy record the replay can never write until it reaches the
// quarantine threshold. Its field in the tenant's backup hash is already
// tenant-scoped; once the record is durably quarantined that same field must
// leave the hash and its attempt counter must be cleared, so later cycles find
// nothing left to quarantine again.
func TestIntegrationLegacyBackupReplayQuarantineRemovesTheTenantRecord(t *testing.T) {
	if testing.Short() {
		t.Skip("requires PostgreSQL, MongoDB, Valkey, and RabbitMQ")
	}

	t.Setenv("ALLOW_INSECURE_TLS", "true")
	t.Setenv("AUDIT_LOG_ENABLED", "false")
	t.Setenv("RABBITMQ_TRANSACTION_ASYNC", "true")

	const tenantID = "tenant-quarantine"

	infra := setupEngineWriteBehindHTTPIntegration(t)
	infra.command.RabbitMQRepo = &recordingBalanceOperationProducer{}
	ctx := tmcore.ContextWithTenantID(context.Background(), tenantID)

	restoreTransactionRepo := infra.failTransactionWrites()
	transactionID := infra.leaveAnnotationInLegacyBackup(t, ctx, infra.newHTTPApp(tenantID))
	restoreTransactionRepo()

	field := infra.rewriteLegacyBackupEntry(t, ctx, func(entry *mmodel.TransactionRedisQueue) {
		entry.Validate = nil
	})
	require.True(t, strings.HasPrefix(field, "tenant:"+tenantID+":"), "the entry field must be tenant-scoped")

	consumer := NewRedisQueueConsumer(&libLog.GoLogger{}, infra.command, infra.query).WithQuarantineRepository(infra.quarantine)
	consumer.multiTenantEnabled = true
	consumer.tenantMongo = staticTenantMongo{database: infra.mongo}
	replay := consumer.newLegacyBackupConsumer()

	queue, err := tmvalkey.GetKeyContext(ctx, txRedis.TransactionBackupQueue)
	require.NoError(t, err)
	attempts, err := tmvalkey.GetKeyContext(ctx, txRedis.TransactionBackupAttemptsQueue)
	require.NoError(t, err)

	for cycle := 1; cycle < QuarantineThreshold; cycle++ {
		replay.Consume(ctx)
		require.True(t, infra.redis.HExists(ctx, queue, field).Val(), "below the threshold the record stays for the next cycle")
		require.EqualValues(t, 1, infra.redis.HLen(ctx, attempts).Val(), "each cycle counts one attempt for the record")
	}

	replay.Consume(ctx)

	var quarantined int
	require.NoError(t, infra.db.QueryRow(
		`SELECT COUNT(*) FROM transaction_backup_quarantine WHERE redis_key LIKE $1 AND transaction_id = $2 AND failure_reason = 'nil_validate'`,
		"legacy_backup:"+field+":%", transactionID,
	).Scan(&quarantined))
	require.Equal(t, 1, quarantined, "the record must be durably quarantined")

	assert.False(t, infra.redis.HExists(ctx, queue, field).Val(), "a quarantined record must leave the tenant's backup hash")
	assert.Zero(t, infra.redis.HLen(ctx, queue).Val(), "the removal must not leave or create another field")
	assert.Zero(t, infra.redis.HLen(ctx, attempts).Val(), "quarantine must clear the attempt counter")

	stats := replay.Consume(ctx)
	assert.Zero(t, stats.messageCount, "nothing is left to replay after quarantine")
}

// TestIntegrationLegacyBackupReplayQuarantineRemovesAnEmptyRecord leaves an
// empty value in a trusted backup field. The replay cannot parse it and
// quarantines it at the threshold; the empty payload must then be removed by
// value like any other, instead of staying in the hash and being quarantined
// again every cycle.
func TestIntegrationLegacyBackupReplayQuarantineRemovesAnEmptyRecord(t *testing.T) {
	if testing.Short() {
		t.Skip("requires PostgreSQL, MongoDB, Valkey, and RabbitMQ")
	}

	t.Setenv("ALLOW_INSECURE_TLS", "true")
	t.Setenv("AUDIT_LOG_ENABLED", "false")
	t.Setenv("RABBITMQ_TRANSACTION_ASYNC", "true")

	infra := setupEngineWriteBehindHTTPIntegration(t)
	infra.command.RabbitMQRepo = &recordingBalanceOperationProducer{}
	ctx := context.Background()

	restoreTransactionRepo := infra.failTransactionWrites()
	transactionID := infra.leaveAnnotationInLegacyBackup(t, ctx, infra.newHTTPApp(""))
	restoreTransactionRepo()

	field, _ := infra.requireSingleLegacyBackupEntry(t, ctx)
	queue, err := tmvalkey.GetKeyContext(ctx, txRedis.TransactionBackupQueue)
	require.NoError(t, err)
	require.NoError(t, infra.redis.HSet(ctx, queue, field, "").Err())

	replay := NewRedisQueueConsumer(&libLog.GoLogger{}, infra.command, infra.query).
		WithQuarantineRepository(infra.quarantine).
		newLegacyBackupConsumer()

	for cycle := 0; cycle < QuarantineThreshold; cycle++ {
		replay.Consume(ctx)
	}

	var (
		quarantined int
		payload     []byte
	)

	require.NoError(t, infra.db.QueryRow(
		`SELECT COUNT(*) FROM transaction_backup_quarantine WHERE transaction_id = $1 AND failure_reason = 'unmarshal_failure'`, transactionID,
	).Scan(&quarantined))
	require.Equal(t, 1, quarantined, "the empty record must be durably quarantined")
	require.NoError(t, infra.db.QueryRow(
		`SELECT payload FROM transaction_backup_quarantine WHERE transaction_id = $1`, transactionID,
	).Scan(&payload))
	assert.Empty(t, payload)

	assert.False(t, infra.redis.HExists(ctx, queue, field).Val(), "the quarantined empty record must leave the backup hash")

	attempts, err := tmvalkey.GetKeyContext(ctx, txRedis.TransactionBackupAttemptsQueue)
	require.NoError(t, err)
	assert.Zero(t, infra.redis.HLen(ctx, attempts).Val(), "quarantine must clear the attempt counter")
}
