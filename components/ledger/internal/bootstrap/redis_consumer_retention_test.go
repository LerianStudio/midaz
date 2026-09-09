// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	transaction "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/redis/transaction"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/services/command"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
)

type retentionFinalizer struct{}

func (retentionFinalizer) Finalize(context.Context, *command.BalanceEngineRecoveryEnvelope) error {
	return nil
}

func (retentionFinalizer) FinalizeWithOutcome(context.Context, *command.BalanceEngineRecoveryEnvelope) (command.BalanceEngineFinalizationResult, error) {
	return command.BalanceEngineFinalizationResult{Outcome: command.BalanceEngineRecoveryOutcome{TransactionStatus: constant.APPROVED}}, nil
}

type retentionQueue struct {
	transaction.RedisRepository
	completedAt  time.Time
	cleanupAt    time.Time
	cleanupLimit int
	cleanupCalls int
}

func (queue *retentionQueue) ReadAllMessagesFromQueue(context.Context) (map[string]string, error) {
	return map[string]string{}, nil
}

func (queue *retentionQueue) CleanupEngineRecovery(_ context.Context, now time.Time, limit int) (transaction.RecoveryCleanupResult, error) {
	queue.cleanupCalls++
	queue.cleanupAt = now
	queue.cleanupLimit = limit
	return transaction.RecoveryCleanupResult{}, nil
}

func (queue *retentionQueue) CompareAndDeleteRecovery(context.Context, string, string) (int64, error) {
	return transaction.RecoveryAckDeleted, nil
}

func (queue *retentionQueue) CompareAndDeleteRecoveryWithProtection(
	_ context.Context,
	_, _ uuid.UUID,
	_, _ string,
	_ bool,
	completedAt time.Time,
) (int64, error) {
	queue.completedAt = completedAt
	return transaction.RecoveryAckDeleted, nil
}

func TestRecoveryCompletionUsesInjectedClockAfterDurableOutcome(t *testing.T) {
	fixed := time.Date(2042, time.March, 4, 5, 6, 7, 8, time.UTC)
	queue := &retentionQueue{}
	consumer := (&RedisQueueConsumer{Logger: recoveryQuietLogger{}, queue: queue}).
		WithBalanceEngineFinalizer(retentionFinalizer{}).
		WithRecoveryClock(func() time.Time { return fixed })
	envelope := &command.BalanceEngineRecoveryEnvelope{
		OrganizationID: uuid.MustParse("11111111-1111-4111-8111-111111111111"),
		LedgerID:       uuid.MustParse("22222222-2222-4222-8222-222222222222"),
	}

	err := consumer.finalizeRecoveryRecord(t.Context(), "field", "raw", envelope)
	require.NoError(t, err)
	require.Equal(t, fixed, queue.completedAt)
}

func TestRecoveryCleanupUsesInjectedClockWhenBackupQueueIsEmpty(t *testing.T) {
	fixed := time.Date(2042, time.April, 5, 6, 7, 8, 9, time.UTC)
	queue := &retentionQueue{}
	consumer := (&RedisQueueConsumer{Logger: recoveryQuietLogger{}, queue: queue}).
		WithRecoveryClock(func() time.Time { return fixed })

	consumer.readMessagesAndProcess(t.Context())

	require.Equal(t, 1, queue.cleanupCalls)
	require.Equal(t, fixed, queue.cleanupAt)
	require.Equal(t, 100, queue.cleanupLimit)
}
