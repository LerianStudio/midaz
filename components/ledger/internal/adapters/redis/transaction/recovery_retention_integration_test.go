//go:build integration

// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package redis

import (
	"context"
	"encoding/json"
	"strconv"
	"testing"
	"time"

	tmcore "github.com/LerianStudio/lib-commons/v6/commons/tenant-manager/core"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/internal/cachepolicy"
	redistestutil "github.com/LerianStudio/midaz/v4/tests/utils/redis"
)

type retentionReceipt struct {
	FormatVersion     int                        `json:"formatVersion"`
	TenantID          string                     `json:"tenantId"`
	OrganizationID    string                     `json:"organizationId"`
	LedgerID          string                     `json:"ledgerId"`
	ExecutionID       string                     `json:"executionId"`
	IntentFingerprint string                     `json:"intentFingerprint"`
	Response          string                     `json:"response"`
	Protection        retentionReceiptProtection `json:"protection"`
}

type retentionReceiptProtection struct {
	FormatVersion         int              `json:"formatVersion"`
	RetentionSeconds      int64            `json:"retentionSeconds"`
	Transactions          []string         `json:"transactions"`
	RecoveryFields        []string         `json:"recoveryFields"`
	Acknowledged          map[string]bool  `json:"acknowledged"`
	TerminalCompletedAtMS map[string]int64 `json:"terminalCompletedAtMs"`
	CleanupAfterMS        int64            `json:"cleanupAfterMs"`
}

func TestIntegrationRecoveryRetentionWaitsForEveryMember(t *testing.T) {
	if testing.Short() {
		t.Skip("requires Valkey")
	}

	container := redistestutil.SetupReusableContainer(t)
	tenant := "retention-" + uuid.NewString()
	ctx := tmcore.ContextWithTenantID(t.Context(), tenant)
	repo, err := NewConsumerRedis(&recoveryAckClient{client: container.Client})
	require.NoError(t, err)

	organizationID := uuid.MustParse("11111111-1111-4111-8111-111111111111")
	ledgerID := uuid.MustParse("22222222-2222-4222-8222-222222222222")
	executionID := uuid.MustParse("33333333-3333-4333-8333-333333333333")
	transactionIDs := []uuid.UUID{
		uuid.MustParse("44444444-4444-4444-8444-444444444444"),
		uuid.MustParse("55555555-5555-4555-8555-555555555555"),
	}
	fields := []string{
		transactionIDs[0].String() + ":" + executionID.String(),
		transactionIDs[1].String() + ":" + executionID.String(),
	}
	scope := organizationID.String() + ":" + ledgerID.String()
	queue, err := tenantKeyFromContextOrError(ctx, TransactionBackupQueue)
	require.NoError(t, err)
	engineQueue, err := tenantKeyFromContextOrError(ctx, cachepolicy.EngineRecoverQueue)
	require.NoError(t, err)
	attempts, err := tenantKeyFromContextOrError(ctx, TransactionBackupAttemptsQueue)
	require.NoError(t, err)
	receipts, err := tenantKeyFromContextOrError(ctx, "engine:"+cachepolicy.HashTag+":receipts:"+scope)
	require.NoError(t, err)
	guards, err := tenantKeyFromContextOrError(ctx, "engine:"+cachepolicy.HashTag+":guards:"+scope)
	require.NoError(t, err)
	protection, err := tenantKeyFromContextOrError(ctx, "engine:"+cachepolicy.HashTag+":protection:"+scope)
	require.NoError(t, err)
	cleanup, err := tenantKeyFromContextOrError(ctx, EngineRecoveryCleanupSchedule)
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, container.Client.Del(context.Background(), queue, engineQueue, attempts, receipts, guards, protection, cleanup).Err())
	})

	receipt := retentionReceipt{
		FormatVersion: 1, TenantID: tenant, OrganizationID: organizationID.String(), LedgerID: ledgerID.String(), ExecutionID: executionID.String(),
		IntentFingerprint: "fingerprint", Response: `{"protocolVersion":1,"movements":[],"final":[]}`,
		Protection: retentionReceiptProtection{
			FormatVersion: 1, RetentionSeconds: 604800,
			Transactions: []string{transactionIDs[0].String(), transactionIDs[1].String()}, RecoveryFields: fields,
			Acknowledged: map[string]bool{}, TerminalCompletedAtMS: map[string]int64{},
		},
	}
	rawReceipt, err := json.Marshal(receipt)
	require.NoError(t, err)
	for index, field := range fields {
		require.NoError(t, container.Client.HSet(ctx, queue, field, "recovery-"+string(rune('a'+index))).Err())
		require.NoError(t, container.Client.HSet(ctx, guards, transactionIDs[index].String(), "APPROVED").Err())
		coordinator := `{"formatVersion":1,"executions":{"` + executionID.String() + `":0}}`
		require.NoError(t, container.Client.HSet(ctx, protection, transactionIDs[index].String(), coordinator).Err())
	}
	require.NoError(t, container.Client.HSet(ctx, receipts, executionID.String(), rawReceipt).Err())

	completedAt := time.Date(2040, time.January, 2, 3, 4, 5, 600_000_000, time.UTC)
	status, err := repo.CompareAndDeleteRecoveryWithProtection(ctx, organizationID, ledgerID, fields[0], "recovery-a", true, completedAt)
	require.NoError(t, err)
	require.Equal(t, RecoveryAckDeleted, status)
	require.False(t, container.Client.HExists(ctx, queue, fields[0]).Val())
	require.True(t, container.Client.HExists(ctx, queue, fields[1]).Val())
	require.Equal(t, time.Duration(-1), container.Client.TTL(ctx, receipts).Val())
	for _, transactionID := range transactionIDs {
		require.True(t, container.Client.HExists(ctx, guards, transactionID.String()).Val())
	}

	status, err = repo.CompareAndDeleteRecoveryWithProtection(ctx, organizationID, ledgerID, fields[1], "recovery-b", true, completedAt)
	require.NoError(t, err)
	require.Equal(t, RecoveryAckDeleted, status)
	require.False(t, container.Client.HExists(ctx, queue, fields[1]).Val())

	wantDeadline := completedAt.Add(7 * 24 * time.Hour).UnixMilli()
	var completed retentionReceipt
	require.NoError(t, json.Unmarshal([]byte(container.Client.HGet(ctx, receipts, executionID.String()).Val()), &completed))
	require.Equal(t, wantDeadline, completed.Protection.CleanupAfterMS)
	require.Equal(t, time.Duration(-1), container.Client.TTL(ctx, receipts).Val())
	for _, transactionID := range transactionIDs {
		var coordinator struct {
			CleanupAfterMS int64 `json:"cleanupAfterMs"`
		}
		require.NoError(t, json.Unmarshal([]byte(container.Client.HGet(ctx, protection, transactionID.String()).Val()), &coordinator))
		require.Equal(t, wantDeadline, coordinator.CleanupAfterMS)
		require.True(t, container.Client.HExists(ctx, guards, transactionID.String()).Val())
	}
	require.Equal(t, float64(wantDeadline), container.Client.ZScore(ctx, cleanup, recoveryCleanupMember(organizationID, ledgerID, executionID)).Val())

	result, err := repo.CleanupEngineRecovery(ctx, completedAt.Add(7*24*time.Hour-time.Millisecond), 10)
	require.NoError(t, err)
	require.Zero(t, result.Scanned)
	require.True(t, container.Client.HExists(ctx, receipts, executionID.String()).Val())

	require.NoError(t, container.Client.HSet(ctx, engineQueue, fields[0], "independent-engine-recover").Err())
	result, err = repo.CleanupEngineRecovery(ctx, completedAt.Add(7*24*time.Hour), 10)
	require.ErrorContains(t, err, "recovery member still exists")
	require.True(t, container.Client.HExists(ctx, receipts, executionID.String()).Val())
	require.True(t, container.Client.HExists(ctx, guards, transactionIDs[0].String()).Val())
	require.NoError(t, container.Client.HDel(ctx, engineQueue, fields[0]).Err())

	result, err = repo.CleanupEngineRecovery(ctx, completedAt.Add(7*24*time.Hour), 10)
	require.NoError(t, err)
	require.Equal(t, RecoveryCleanupResult{Scanned: 1, Cleaned: 1}, result)
	require.False(t, container.Client.HExists(ctx, receipts, executionID.String()).Val())
	for _, transactionID := range transactionIDs {
		require.False(t, container.Client.HExists(ctx, guards, transactionID.String()).Val())
		require.False(t, container.Client.HExists(ctx, protection, transactionID.String()).Val())
	}
	require.Zero(t, container.Client.ZCard(ctx, cleanup).Val())
}

func TestIntegrationRecoveryRetentionPropagatesTerminalProofToPendingExecution(t *testing.T) {
	if testing.Short() {
		t.Skip("requires Valkey")
	}

	container := redistestutil.SetupReusableContainer(t)
	tenant := "retention-pending-" + uuid.NewString()
	ctx := tmcore.ContextWithTenantID(t.Context(), tenant)
	repo, err := NewConsumerRedis(&recoveryAckClient{client: container.Client})
	require.NoError(t, err)

	organizationID := uuid.MustParse("11111111-1111-4111-8111-111111111111")
	ledgerID := uuid.MustParse("22222222-2222-4222-8222-222222222222")
	transactionID := uuid.MustParse("44444444-4444-4444-8444-444444444444")
	pendingExecutionID := uuid.MustParse("33333333-3333-4333-8333-333333333333")
	terminalExecutionID := uuid.MustParse("66666666-6666-4666-8666-666666666666")
	pendingField := transactionID.String() + ":" + pendingExecutionID.String()
	terminalField := transactionID.String() + ":" + terminalExecutionID.String()
	scope := organizationID.String() + ":" + ledgerID.String()

	queue, err := tenantKeyFromContextOrError(ctx, TransactionBackupQueue)
	require.NoError(t, err)
	receipts, err := tenantKeyFromContextOrError(ctx, "engine:"+cachepolicy.HashTag+":receipts:"+scope)
	require.NoError(t, err)
	guards, err := tenantKeyFromContextOrError(ctx, "engine:"+cachepolicy.HashTag+":guards:"+scope)
	require.NoError(t, err)
	protection, err := tenantKeyFromContextOrError(ctx, "engine:"+cachepolicy.HashTag+":protection:"+scope)
	require.NoError(t, err)
	cleanup, err := tenantKeyFromContextOrError(ctx, EngineRecoveryCleanupSchedule)
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, container.Client.Del(context.Background(), queue, receipts, guards, protection, cleanup).Err())
	})

	pending := retentionReceipt{
		FormatVersion: 1, TenantID: tenant, OrganizationID: organizationID.String(), LedgerID: ledgerID.String(), ExecutionID: pendingExecutionID.String(),
		IntentFingerprint: "pending", Response: `{"protocolVersion":1,"movements":[],"final":[]}`,
		Protection: retentionReceiptProtection{
			FormatVersion: 1, RetentionSeconds: 300,
			Transactions: []string{transactionID.String()}, RecoveryFields: []string{pendingField},
			Acknowledged: map[string]bool{transactionID.String(): true}, TerminalCompletedAtMS: map[string]int64{},
		},
	}
	terminal := retentionReceipt{
		FormatVersion: 1, TenantID: tenant, OrganizationID: organizationID.String(), LedgerID: ledgerID.String(), ExecutionID: terminalExecutionID.String(),
		IntentFingerprint: "terminal", Response: `{"protocolVersion":1,"movements":[],"final":[]}`,
		Protection: retentionReceiptProtection{
			FormatVersion: 1, RetentionSeconds: 604800,
			Transactions: []string{transactionID.String()}, RecoveryFields: []string{terminalField},
			Acknowledged: map[string]bool{}, TerminalCompletedAtMS: map[string]int64{},
		},
	}
	pendingRaw, err := json.Marshal(pending)
	require.NoError(t, err)
	terminalRaw, err := json.Marshal(terminal)
	require.NoError(t, err)
	require.NoError(t, container.Client.HSet(
		ctx, receipts,
		pendingExecutionID.String(), pendingRaw,
		terminalExecutionID.String(), terminalRaw,
	).Err())
	require.NoError(t, container.Client.HSet(ctx, queue, terminalField, "terminal-recovery").Err())
	require.NoError(t, container.Client.HSet(ctx, guards, transactionID.String(), "APPROVED").Err())
	coordinator := `{"formatVersion":1,"executions":{"` + pendingExecutionID.String() + `":0,"` + terminalExecutionID.String() + `":0}}`
	require.NoError(t, container.Client.HSet(ctx, protection, transactionID.String(), coordinator).Err())

	completedAt := time.Date(2041, time.April, 5, 6, 7, 8, 900_000_000, time.UTC)
	status, err := repo.CompareAndDeleteRecoveryWithProtection(
		ctx, organizationID, ledgerID, terminalField, "terminal-recovery", true, completedAt,
	)
	require.NoError(t, err)
	require.Equal(t, RecoveryAckDeleted, status)

	var updatedPending, updatedTerminal retentionReceipt
	require.NoError(t, json.Unmarshal([]byte(container.Client.HGet(ctx, receipts, pendingExecutionID.String()).Val()), &updatedPending))
	require.NoError(t, json.Unmarshal([]byte(container.Client.HGet(ctx, receipts, terminalExecutionID.String()).Val()), &updatedTerminal))
	require.Equal(t, completedAt.UnixMilli(), updatedPending.Protection.TerminalCompletedAtMS[transactionID.String()])
	require.Equal(t, completedAt.Add(300*time.Second).UnixMilli(), updatedPending.Protection.CleanupAfterMS)
	require.Equal(t, completedAt.Add(7*24*time.Hour).UnixMilli(), updatedTerminal.Protection.CleanupAfterMS)

	var updatedCoordinator struct {
		Executions     map[string]int64 `json:"executions"`
		CleanupAfterMS int64            `json:"cleanupAfterMs"`
	}
	require.NoError(t, json.Unmarshal([]byte(container.Client.HGet(ctx, protection, transactionID.String()).Val()), &updatedCoordinator))
	require.Equal(t, updatedPending.Protection.CleanupAfterMS, updatedCoordinator.Executions[pendingExecutionID.String()])
	require.Equal(t, updatedTerminal.Protection.CleanupAfterMS, updatedCoordinator.Executions[terminalExecutionID.String()])
	require.Equal(t, updatedTerminal.Protection.CleanupAfterMS, updatedCoordinator.CleanupAfterMS)
	require.True(t, container.Client.HExists(ctx, guards, transactionID.String()).Val())

	result, err := repo.CleanupEngineRecovery(ctx, completedAt.Add(300*time.Second), 10)
	require.NoError(t, err)
	require.Equal(t, RecoveryCleanupResult{Scanned: 1, Cleaned: 1}, result)
	require.False(t, container.Client.HExists(ctx, receipts, pendingExecutionID.String()).Val())
	require.True(t, container.Client.HExists(ctx, receipts, terminalExecutionID.String()).Val())
	require.True(t, container.Client.HExists(ctx, guards, transactionID.String()).Val())
	var retainedCoordinator struct {
		Executions     map[string]int64 `json:"executions"`
		CleanupAfterMS int64            `json:"cleanupAfterMs"`
	}
	require.NoError(t, json.Unmarshal([]byte(container.Client.HGet(ctx, protection, transactionID.String()).Val()), &retainedCoordinator))
	require.Equal(t, map[string]int64{terminalExecutionID.String(): updatedTerminal.Protection.CleanupAfterMS}, retainedCoordinator.Executions)
	require.Equal(t, updatedTerminal.Protection.CleanupAfterMS, retainedCoordinator.CleanupAfterMS)

	result, err = repo.CleanupEngineRecovery(ctx, completedAt.Add(7*24*time.Hour), 10)
	require.NoError(t, err)
	require.Equal(t, RecoveryCleanupResult{Scanned: 1, Cleaned: 1}, result)
	require.False(t, container.Client.HExists(ctx, receipts, terminalExecutionID.String()).Val())
	require.False(t, container.Client.HExists(ctx, guards, transactionID.String()).Val())
	require.False(t, container.Client.HExists(ctx, protection, transactionID.String()).Val())
}

func TestIntegrationRecoveryRetentionDoesNotRetrofitLegacyReceipt(t *testing.T) {
	if testing.Short() {
		t.Skip("requires Valkey")
	}

	container := redistestutil.SetupReusableContainer(t)
	tenant := "retention-legacy-" + uuid.NewString()
	ctx := tmcore.ContextWithTenantID(t.Context(), tenant)
	repo, err := NewConsumerRedis(&recoveryAckClient{client: container.Client})
	require.NoError(t, err)

	organizationID := uuid.MustParse("11111111-1111-4111-8111-111111111111")
	ledgerID := uuid.MustParse("22222222-2222-4222-8222-222222222222")
	executionID := uuid.MustParse("33333333-3333-4333-8333-333333333333")
	transactionID := uuid.MustParse("44444444-4444-4444-8444-444444444444")
	field := transactionID.String() + ":" + executionID.String()
	scope := organizationID.String() + ":" + ledgerID.String()

	queue, err := tenantKeyFromContextOrError(ctx, TransactionBackupQueue)
	require.NoError(t, err)
	receipts, err := tenantKeyFromContextOrError(ctx, "engine:"+cachepolicy.HashTag+":receipts:"+scope)
	require.NoError(t, err)
	guards, err := tenantKeyFromContextOrError(ctx, "engine:"+cachepolicy.HashTag+":guards:"+scope)
	require.NoError(t, err)
	protection, err := tenantKeyFromContextOrError(ctx, "engine:"+cachepolicy.HashTag+":protection:"+scope)
	require.NoError(t, err)
	cleanup, err := tenantKeyFromContextOrError(ctx, EngineRecoveryCleanupSchedule)
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, container.Client.Del(context.Background(), queue, receipts, guards, protection, cleanup).Err())
	})

	legacyReceipt := `{"formatVersion":1,"executionId":"` + executionID.String() + `","response":"{}"}`
	require.NoError(t, container.Client.HSet(ctx, queue, field, "legacy-recovery").Err())
	require.NoError(t, container.Client.HSet(ctx, receipts, executionID.String(), legacyReceipt).Err())
	require.NoError(t, container.Client.HSet(ctx, guards, transactionID.String(), "APPROVED").Err())

	status, err := repo.CompareAndDeleteRecoveryWithProtection(
		ctx, organizationID, ledgerID, field, "legacy-recovery", true,
		time.Date(2041, time.May, 6, 7, 8, 9, 0, time.UTC),
	)
	require.NoError(t, err)
	require.Equal(t, RecoveryAckDeleted, status)
	require.False(t, container.Client.HExists(ctx, queue, field).Val())
	require.True(t, container.Client.HExists(ctx, receipts, executionID.String()).Val())
	require.True(t, container.Client.HExists(ctx, guards, transactionID.String()).Val())
	require.False(t, container.Client.Exists(ctx, protection).Val() > 0)
	result, err := repo.CleanupEngineRecovery(ctx, time.Date(2100, time.January, 1, 0, 0, 0, 0, time.UTC), 10)
	require.NoError(t, err)
	require.Zero(t, result.Scanned)
	require.Zero(t, container.Client.ZCard(ctx, cleanup).Val())
}

func TestIntegrationRecoveryCleanupPreservesNonterminalExecution(t *testing.T) {
	if testing.Short() {
		t.Skip("requires Valkey")
	}

	container := redistestutil.SetupReusableContainer(t)
	tenant := "retention-nonterminal-" + uuid.NewString()
	ctx := tmcore.ContextWithTenantID(t.Context(), tenant)
	repo, err := NewConsumerRedis(&recoveryAckClient{client: container.Client})
	require.NoError(t, err)

	organizationID := uuid.MustParse("11111111-1111-4111-8111-111111111111")
	ledgerID := uuid.MustParse("22222222-2222-4222-8222-222222222222")
	executionID := uuid.MustParse("33333333-3333-4333-8333-333333333333")
	transactionID := uuid.MustParse("44444444-4444-4444-8444-444444444444")
	field := transactionID.String() + ":" + executionID.String()
	scope := organizationID.String() + ":" + ledgerID.String()
	queue, err := tenantKeyFromContextOrError(ctx, TransactionBackupQueue)
	require.NoError(t, err)
	receipts, err := tenantKeyFromContextOrError(ctx, "engine:"+cachepolicy.HashTag+":receipts:"+scope)
	require.NoError(t, err)
	guards, err := tenantKeyFromContextOrError(ctx, "engine:"+cachepolicy.HashTag+":guards:"+scope)
	require.NoError(t, err)
	protection, err := tenantKeyFromContextOrError(ctx, "engine:"+cachepolicy.HashTag+":protection:"+scope)
	require.NoError(t, err)
	cleanup, err := tenantKeyFromContextOrError(ctx, EngineRecoveryCleanupSchedule)
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, container.Client.Del(context.Background(), queue, receipts, guards, protection, cleanup).Err())
	})

	receipt := retentionReceipt{
		FormatVersion: 1, TenantID: tenant, OrganizationID: organizationID.String(), LedgerID: ledgerID.String(), ExecutionID: executionID.String(),
		IntentFingerprint: "pending", Response: `{"protocolVersion":1,"movements":[],"final":[]}`,
		Protection: retentionReceiptProtection{
			FormatVersion: 1, RetentionSeconds: 300,
			Transactions: []string{transactionID.String()}, RecoveryFields: []string{field},
			Acknowledged: map[string]bool{}, TerminalCompletedAtMS: map[string]int64{},
		},
	}
	rawReceipt, err := json.Marshal(receipt)
	require.NoError(t, err)
	require.NoError(t, container.Client.HSet(ctx, queue, field, "pending-recovery").Err())
	require.NoError(t, container.Client.HSet(ctx, receipts, executionID.String(), rawReceipt).Err())
	require.NoError(t, container.Client.HSet(ctx, guards, transactionID.String(), "PENDING").Err())
	require.NoError(t, container.Client.HSet(ctx, protection, transactionID.String(), `{"formatVersion":1,"executions":{"`+executionID.String()+`":0}}`).Err())

	completedAt := time.Date(2041, time.June, 7, 8, 9, 10, 0, time.UTC)
	status, err := repo.CompareAndDeleteRecoveryWithProtection(ctx, organizationID, ledgerID, field, "pending-recovery", false, completedAt)
	require.NoError(t, err)
	require.Equal(t, RecoveryAckDeleted, status)
	result, err := repo.CleanupEngineRecovery(ctx, completedAt.Add(24*time.Hour), 10)
	require.NoError(t, err)
	require.Zero(t, result.Scanned)
	require.True(t, container.Client.HExists(ctx, receipts, executionID.String()).Val())
	require.True(t, container.Client.HExists(ctx, guards, transactionID.String()).Val())
	require.True(t, container.Client.HExists(ctx, protection, transactionID.String()).Val())
	require.Zero(t, container.Client.ZCard(ctx, cleanup).Val())
}

func TestIntegrationRecoveryCleanupDropsOnlyStaleScheduleMembers(t *testing.T) {
	if testing.Short() {
		t.Skip("requires Valkey")
	}

	container := redistestutil.SetupReusableContainer(t)
	tenant := "retention-stale-" + uuid.NewString()
	ctx := tmcore.ContextWithTenantID(t.Context(), tenant)
	repo, err := NewConsumerRedis(&recoveryAckClient{client: container.Client})
	require.NoError(t, err)

	organizationID := uuid.MustParse("11111111-1111-4111-8111-111111111111")
	ledgerID := uuid.MustParse("22222222-2222-4222-8222-222222222222")
	executionID := uuid.MustParse("33333333-3333-4333-8333-333333333333")
	transactionID := uuid.MustParse("44444444-4444-4444-8444-444444444444")
	scope := organizationID.String() + ":" + ledgerID.String()
	cleanup, err := tenantKeyFromContextOrError(ctx, EngineRecoveryCleanupSchedule)
	require.NoError(t, err)
	guards, err := tenantKeyFromContextOrError(ctx, "engine:"+cachepolicy.HashTag+":guards:"+scope)
	require.NoError(t, err)
	protection, err := tenantKeyFromContextOrError(ctx, "engine:"+cachepolicy.HashTag+":protection:"+scope)
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, container.Client.Del(context.Background(), cleanup, guards, protection).Err())
	})

	due := time.Date(2041, time.July, 8, 9, 10, 11, 0, time.UTC)
	member := recoveryCleanupMember(organizationID, ledgerID, executionID)
	require.NoError(t, container.Client.ZAdd(
		ctx, cleanup,
		redis.Z{Score: float64(due.UnixMilli()), Member: member},
		redis.Z{Score: float64(due.UnixMilli()), Member: "malformed"},
	).Err())
	require.NoError(t, container.Client.HSet(ctx, guards, transactionID.String(), "APPROVED").Err())
	require.NoError(t, container.Client.HSet(ctx, protection, transactionID.String(), `{"formatVersion":1,"executions":{"newer":0}}`).Err())

	result, err := repo.CleanupEngineRecovery(ctx, due, 10)
	require.NoError(t, err)
	require.Equal(t, RecoveryCleanupResult{Scanned: 2, Stale: 2}, result)
	require.Zero(t, container.Client.ZCard(ctx, cleanup).Val())
	require.Equal(t, "APPROVED", container.Client.HGet(ctx, guards, transactionID.String()).Val())
	require.True(t, container.Client.HExists(ctx, protection, transactionID.String()).Val())
}

func TestIntegrationRecoveryCleanupRejectsForgedEarlyDeadlineWithoutWrites(t *testing.T) {
	if testing.Short() {
		t.Skip("requires Valkey")
	}

	container := redistestutil.SetupReusableContainer(t)
	tenant := "retention-forged-" + uuid.NewString()
	ctx := tmcore.ContextWithTenantID(t.Context(), tenant)
	repo, err := NewConsumerRedis(&recoveryAckClient{client: container.Client})
	require.NoError(t, err)

	organizationID := uuid.MustParse("11111111-1111-4111-8111-111111111111")
	ledgerID := uuid.MustParse("22222222-2222-4222-8222-222222222222")
	executionID := uuid.MustParse("33333333-3333-4333-8333-333333333333")
	transactionID := uuid.MustParse("44444444-4444-4444-8444-444444444444")
	field := transactionID.String() + ":" + executionID.String()
	scope := organizationID.String() + ":" + ledgerID.String()
	receipts, err := tenantKeyFromContextOrError(ctx, "engine:"+cachepolicy.HashTag+":receipts:"+scope)
	require.NoError(t, err)
	guards, err := tenantKeyFromContextOrError(ctx, "engine:"+cachepolicy.HashTag+":guards:"+scope)
	require.NoError(t, err)
	protection, err := tenantKeyFromContextOrError(ctx, "engine:"+cachepolicy.HashTag+":protection:"+scope)
	require.NoError(t, err)
	cleanup, err := tenantKeyFromContextOrError(ctx, EngineRecoveryCleanupSchedule)
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, container.Client.Del(context.Background(), receipts, guards, protection, cleanup).Err())
	})

	completedAt := time.Date(2041, time.August, 9, 10, 11, 12, 0, time.UTC)
	forgedDeadline := completedAt.Add(time.Second).UnixMilli()
	receipt := retentionReceipt{
		FormatVersion: 1, TenantID: tenant, OrganizationID: organizationID.String(), LedgerID: ledgerID.String(), ExecutionID: executionID.String(),
		IntentFingerprint: "forged", Response: `{"protocolVersion":1,"movements":[],"final":[]}`,
		Protection: retentionReceiptProtection{
			FormatVersion: 1, RetentionSeconds: 300,
			Transactions: []string{transactionID.String()}, RecoveryFields: []string{field},
			Acknowledged:          map[string]bool{transactionID.String(): true},
			TerminalCompletedAtMS: map[string]int64{transactionID.String(): completedAt.UnixMilli()},
			CleanupAfterMS:        forgedDeadline,
		},
	}
	rawReceipt, err := json.Marshal(receipt)
	require.NoError(t, err)
	deadlineText := strconv.FormatInt(forgedDeadline, 10)
	rawCoordinator := `{"formatVersion":1,"executions":{"` + executionID.String() + `":` + deadlineText + `},"cleanupAfterMs":` + deadlineText + `}`
	member := recoveryCleanupMember(organizationID, ledgerID, executionID)
	require.NoError(t, container.Client.HSet(ctx, receipts, executionID.String(), rawReceipt).Err())
	require.NoError(t, container.Client.HSet(ctx, guards, transactionID.String(), "APPROVED").Err())
	require.NoError(t, container.Client.HSet(ctx, protection, transactionID.String(), rawCoordinator).Err())
	require.NoError(t, container.Client.ZAdd(ctx, cleanup, redis.Z{Score: float64(forgedDeadline), Member: member}).Err())

	_, err = repo.CleanupEngineRecovery(ctx, time.UnixMilli(forgedDeadline), 10)
	require.ErrorContains(t, err, "cleanup receipt deadline differs from terminal proof")
	require.Equal(t, string(rawReceipt), container.Client.HGet(ctx, receipts, executionID.String()).Val())
	require.Equal(t, rawCoordinator, container.Client.HGet(ctx, protection, transactionID.String()).Val())
	require.Equal(t, "APPROVED", container.Client.HGet(ctx, guards, transactionID.String()).Val())
	require.Equal(t, float64(forgedDeadline), container.Client.ZScore(ctx, cleanup, member).Val())
}
