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

	libLog "github.com/LerianStudio/lib-observability/v4/log"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/transaction"
	txRedis "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/redis/transaction"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
)

// unavailableTransactionRepository fails every transaction insert, the shape of
// a PostgreSQL outage at the moment an annotation is written.
type unavailableTransactionRepository struct {
	transaction.Repository
}

func (unavailableTransactionRepository) Create(context.Context, *transaction.Transaction) (*transaction.Transaction, error) {
	return nil, errors.New("transaction database unavailable")
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

	transactionRepo := infra.command.TransactionRepo
	infra.command.TransactionRepo = unavailableTransactionRepository{Repository: transactionRepo}

	aliases := infra.seedTransfer(t, "replay-"+strings.ReplaceAll(uuid.NewString(), "-", "")[:8])
	created := infra.postAnnotationCreate(t, app, aliases)
	require.Equalf(t, http.StatusInternalServerError, created.status, "a failed annotation write must not answer 201: %s", created.body)
	assert.Equal(t, constant.ErrMessageBrokerUnavailable.Error(), created.decoded["code"])

	field, entry := infra.requireSingleLegacyBackupEntry(t, ctx)
	require.Equal(t, constant.NOTED, entry.TransactionStatus)
	transactionID := entry.TransactionID

	// The cycle replays only entries older than MessageTimeOfLife.
	entry.TTL = time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC)
	aged, err := json.Marshal(entry)
	require.NoError(t, err)
	require.NoError(t, infra.redisRepo.AddMessageToQueue(ctx, field, aged))

	replay := NewRedisQueueConsumer(&libLog.GoLogger{}, infra.command, infra.query).newLegacyBackupConsumer()

	stats := replay.Consume(ctx)
	require.Equal(t, 1, stats.messageCount)
	require.Zero(t, stats.tooYoung)
	infra.requireProjection(t, ctx, transactionID, 0, 0, 0)
	infra.requireSingleLegacyBackupEntry(t, ctx)
	assert.Zero(t, len(producer.messages), "a replay that cannot write must not publish")

	infra.command.TransactionRepo = transactionRepo

	stats = replay.Consume(ctx)
	require.Equal(t, 1, stats.messageCount)
	infra.requireProjection(t, ctx, transactionID, 1, 2, 1)

	var projected string
	require.NoError(t, infra.db.QueryRow(`SELECT status FROM transaction WHERE id = $1`, transactionID).Scan(&projected))
	assert.Equal(t, constant.NOTED, projected)
	assert.Zero(t, len(producer.messages), "the replay must write the annotation, not queue it")

	infra.waitForEmptyLegacyBackupQueue(t, ctx)
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
