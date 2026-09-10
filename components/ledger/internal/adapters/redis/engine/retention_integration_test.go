//go:build integration

// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package engine

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	txredis "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/redis/transaction"
)

func TestIntegrationDelayedFinalizationKeepsReplayProtectionThroughFullWindow(t *testing.T) {
	client, _, _ := newAdapterValkey(t)
	input, limits := richAdapterExecution(t)
	input.RetentionSeconds = maximumRetentionSeconds
	provider := &integrationClientProvider{client: client}
	adapter, err := NewAdapter(provider, limits)
	require.NoError(t, err)

	ctx := context.Background()
	first, err := adapter.Execute(ctx, input)
	require.NoError(t, err)
	resolved, err := resolveAdapterKeys(ctx, input.Execution)
	require.NoError(t, err)
	t.Cleanup(func() {
		keys := []string{resolved.Schedule, resolved.Recovery, resolved.Receipts, resolved.Guards, resolved.Protection, txredis.EngineRecoveryCleanupSchedule}
		for _, pair := range resolved.Balances {
			keys = append(keys, pair.Balance, pair.Deleted)
		}
		require.NoError(t, client.Del(context.Background(), keys...).Err())
	})

	transactionID := input.Execution.Transactions[0].ID
	executionID := input.Execution.ExecutionID
	recoveryField := transactionID.String() + ":" + executionID.String()
	recoveryRaw, err := client.HGet(ctx, resolved.Recovery, recoveryField).Result()
	require.NoError(t, err)
	require.Equal(t, time.Duration(-1), client.TTL(ctx, resolved.Receipts).Val())
	require.Equal(t, time.Duration(-1), client.TTL(ctx, resolved.Guards).Val())

	// Durable finalization is deliberately more than seven days after EVAL. The
	// deadline begins here, not at the engine mutation timestamp.
	completedAt := time.Date(2040, time.February, 10, 12, 30, 0, 123_000_000, time.UTC)
	recoveryRepo, err := txredis.NewConsumerRedis(provider)
	require.NoError(t, err)
	status, err := recoveryRepo.CompareAndDeleteRecoveryWithProtection(
		ctx, input.Execution.OrganizationID, input.Execution.LedgerID,
		recoveryField, recoveryRaw, true, completedAt,
	)
	require.NoError(t, err)
	require.Equal(t, txredis.RecoveryAckDeleted, status)

	receiptRaw, err := client.HGet(ctx, resolved.Receipts, executionID.String()).Bytes()
	require.NoError(t, err)
	var receipt struct {
		Protection struct {
			CleanupAfterMS int64 `json:"cleanupAfterMs"`
		} `json:"protection"`
	}
	require.NoError(t, json.Unmarshal(receiptRaw, &receipt))
	require.Equal(t, completedAt.Add(7*24*time.Hour).UnixMilli(), receipt.Protection.CleanupAfterMS)

	cleanup, err := recoveryRepo.CleanupEngineRecovery(ctx, completedAt.Add(7*24*time.Hour-time.Millisecond), 10)
	require.NoError(t, err)
	require.Zero(t, cleanup.Scanned)

	// The full retry window remains non-destructive through its last millisecond.
	replayed, err := adapter.Execute(ctx, input)
	require.NoError(t, err)
	require.Equal(t, first, replayed)
	require.True(t, client.HExists(ctx, resolved.Receipts, executionID.String()).Val())
	require.True(t, client.HExists(ctx, resolved.Guards, transactionID.String()).Val())

	cleanup, err = recoveryRepo.CleanupEngineRecovery(ctx, completedAt.Add(7*24*time.Hour), 10)
	require.NoError(t, err)
	require.Equal(t, txredis.RecoveryCleanupResult{Scanned: 1, Cleaned: 1}, cleanup)
	require.False(t, client.HExists(ctx, resolved.Receipts, executionID.String()).Val())
	require.False(t, client.HExists(ctx, resolved.Guards, transactionID.String()).Val())
	require.False(t, client.HExists(ctx, resolved.Protection, transactionID.String()).Val())
}
