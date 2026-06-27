// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"context"
	"errors"
	"testing"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/http/in"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/transactionquarantine"
	redisTransaction "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/redis/transaction"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/services/command"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/mock/gomock"
)

// newQuarantineConsumer wires a RedisQueueConsumer with mocked Redis + quarantine
// repositories for poison-flow unit tests.
func newQuarantineConsumer(t *testing.T) (*RedisQueueConsumer, *redisTransaction.MockRedisRepository, *transactionquarantine.MockRepository) {
	t.Helper()

	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	mockRedis := redisTransaction.NewMockRedisRepository(ctrl)
	mockQuarantine := transactionquarantine.NewMockRepository(ctrl)

	handler := in.TransactionHandler{
		Command: &command.UseCase{TransactionRedisRepo: mockRedis},
	}

	consumer := NewRedisQueueConsumer(newTestLogger(), handler).
		WithQuarantineRepository(mockQuarantine)

	return consumer, mockRedis, mockQuarantine
}

// noopSpan returns a non-recording span suitable for tests that do not assert
// on span behavior.
func noopSpan() trace.Span {
	return trace.SpanFromContext(context.Background())
}

// TestQuarantine_ReachesThreshold_QuarantinesThenDeletes is case (a): a poison
// record that reaches attempts=QuarantineThreshold is persisted to the durable
// quarantine table, then (and only then) removed from Redis, and finally its
// attempts counter is cleared. Order is the invariant.
func TestQuarantine_ReachesThreshold_QuarantinesThenDeletes(t *testing.T) {
	t.Parallel()

	consumer, mockRedis, mockQuarantine := newQuarantineConsumer(t)

	ctx := context.Background()
	key := "transaction:{transactions}:" + uuid.NewString() + ":" + uuid.NewString() + ":" + uuid.NewString()
	payload := []byte(`{"raw":"poison"}`)

	// Reaching the threshold on this cycle.
	mockRedis.EXPECT().
		IncrementBackupAttempt(gomock.Any(), key).
		Return(int64(QuarantineThreshold), nil)

	// Insert must happen BEFORE the record is removed.
	gomock.InOrder(
		mockQuarantine.EXPECT().
			Insert(gomock.Any(), gomock.Any()).
			DoAndReturn(func(_ context.Context, rec *transactionquarantine.QuarantineRecord) error {
				assert.Equal(t, key, rec.RedisKey, "quarantine record must carry the redis key")
				assert.Equal(t, payload, rec.Payload, "quarantine record must carry the raw payload")
				assert.Equal(t, "nil_validate", rec.FailureReason)
				assert.Equal(t, QuarantineThreshold, rec.Attempts)

				return nil
			}),
		mockRedis.EXPECT().RemoveMessageFromQueue(gomock.Any(), key).Return(nil),
		mockRedis.EXPECT().ClearBackupAttempt(gomock.Any(), key).Return(nil),
	)

	consumer.quarantinePoisonRecord(ctx, noopSpan(), newTestLogger(), key, uuid.New(), uuid.New(), uuid.New(), payload, "nil_validate")
}

// TestQuarantine_InsertFailure_RecordNotDeleted is case (b): when the durable
// quarantine Insert fails, the record MUST NOT be removed from Redis and its
// attempts counter MUST NOT be cleared — it stays for the next cycle.
func TestQuarantine_InsertFailure_RecordNotDeleted(t *testing.T) {
	t.Parallel()

	consumer, mockRedis, mockQuarantine := newQuarantineConsumer(t)

	ctx := context.Background()
	key := "transaction:{transactions}:org:ledger:tx"
	payload := []byte(`{"raw":"poison"}`)

	mockRedis.EXPECT().
		IncrementBackupAttempt(gomock.Any(), key).
		Return(int64(QuarantineThreshold), nil)

	mockQuarantine.EXPECT().
		Insert(gomock.Any(), gomock.Any()).
		Return(errors.New("postgres down"))

	// THE INVARIANT: no RemoveMessageFromQueue and no ClearBackupAttempt when
	// the persist failed. gomock fails the test if either is called because no
	// expectation is registered for them.

	consumer.quarantinePoisonRecord(ctx, noopSpan(), newTestLogger(), key, uuid.New(), uuid.New(), uuid.New(), payload, "ledger_settings_fetch_failure")
}

// TestQuarantine_BelowThreshold_NoQuarantineNoDelete verifies that below the
// threshold the record is left in Redis: attempts are incremented but neither
// Insert nor any delete is called.
func TestQuarantine_BelowThreshold_NoQuarantineNoDelete(t *testing.T) {
	t.Parallel()

	consumer, mockRedis, _ := newQuarantineConsumer(t)

	ctx := context.Background()
	key := "transaction:{transactions}:org:ledger:tx"

	mockRedis.EXPECT().
		IncrementBackupAttempt(gomock.Any(), key).
		Return(int64(QuarantineThreshold-1), nil)

	// No Insert, no RemoveMessageFromQueue, no ClearBackupAttempt expected.

	consumer.quarantinePoisonRecord(ctx, noopSpan(), newTestLogger(), key, uuid.New(), uuid.New(), uuid.New(), []byte(`{}`), "unmarshal_failure")
}

// TestQuarantine_SuccessPath_ClearsAttempts is case (c): a previously-failing
// record that successfully replays has its attempts counter cleared (via the
// consumer's clearBackupAttempt helper exercised at the end of processMessage).
func TestQuarantine_SuccessPath_ClearsAttempts(t *testing.T) {
	t.Parallel()

	consumer, mockRedis, _ := newQuarantineConsumer(t)

	ctx := context.Background()
	key := "transaction:{transactions}:org:ledger:tx"

	mockRedis.EXPECT().ClearBackupAttempt(gomock.Any(), key).Return(nil)

	consumer.clearBackupAttempt(ctx, newTestLogger(), key)
}

// TestQuarantine_NoRepoConfigured_LeavesRecord verifies the fail-safe: with no
// quarantine repository wired, a poison record is left untouched in Redis
// (no increment, no delete) so the invariant cannot be violated by misconfig.
func TestQuarantine_NoRepoConfigured_LeavesRecord(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	mockRedis := redisTransaction.NewMockRedisRepository(ctrl)
	handler := in.TransactionHandler{Command: &command.UseCase{TransactionRedisRepo: mockRedis}}
	consumer := NewRedisQueueConsumer(newTestLogger(), handler) // no WithQuarantineRepository

	// No Redis calls at all when quarantine repo is nil.
	consumer.quarantinePoisonRecord(context.Background(), noopSpan(), newTestLogger(),
		"transaction:{transactions}:org:ledger:tx", uuid.New(), uuid.New(), uuid.New(), []byte(`{}`), "nil_validate")
}

// TestParsePoisonKeyIDs covers the unmarshal-failure key parser: the org/ledger/
// tx UUIDs are extracted positionally from the backup-queue field key.
func TestParsePoisonKeyIDs(t *testing.T) {
	t.Parallel()

	org := uuid.New()
	ledger := uuid.New()
	tx := uuid.New()

	t.Run("valid transaction key", func(t *testing.T) {
		t.Parallel()

		key := "transaction:{transactions}:" + org.String() + ":" + ledger.String() + ":" + tx.String()

		gotOrg, gotLedger, gotTx, ok := parsePoisonKeyIDs(key)
		require.True(t, ok)
		assert.Equal(t, org, gotOrg)
		assert.Equal(t, ledger, gotLedger)
		assert.Equal(t, tx, gotTx)
	})

	t.Run("key with fewer than three UUIDs", func(t *testing.T) {
		t.Parallel()

		key := "transaction:{transactions}:" + org.String() + ":" + ledger.String()

		_, _, _, ok := parsePoisonKeyIDs(key)
		assert.False(t, ok, "must report failure when fewer than three UUIDs are present")
	})

	t.Run("garbage key", func(t *testing.T) {
		t.Parallel()

		_, _, _, ok := parsePoisonKeyIDs("not-a-key")
		assert.False(t, ok)
	})
}

// silence unused import if SyncKey ever drops out of the redis mock surface.
var _ = redisTransaction.SyncKey{}
