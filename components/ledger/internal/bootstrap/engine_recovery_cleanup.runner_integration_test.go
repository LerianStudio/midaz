//go:build integration

// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"context"
	"encoding/json"
	"strconv"
	"testing"
	"time"

	tmcore "github.com/LerianStudio/lib-commons/v7/commons/tenant-manager/core"
	tmvalkey "github.com/LerianStudio/lib-commons/v7/commons/tenant-manager/valkey"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"

	txRedis "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/redis/transaction"
	"github.com/LerianStudio/midaz/v4/internal/cachepolicy"
	"github.com/LerianStudio/midaz/v4/pkg/utils"
)

const cleanupRunnerRetention = 300

type cleanupRunnerTenant struct {
	ctx                      context.Context
	tenantID                 string
	organizationID, ledgerID uuid.UUID
	queue, receipts, guards  string
	protection, schedule     string
}

func newCleanupRunnerTenant(t *testing.T, tenantID string) *cleanupRunnerTenant {
	t.Helper()

	tenant := &cleanupRunnerTenant{
		ctx: tmcore.ContextWithTenantID(context.Background(), tenantID), tenantID: tenantID,
		organizationID: uuid.New(), ledgerID: uuid.New(),
	}
	scope := tenant.organizationID.String() + ":" + tenant.ledgerID.String()
	tenant.queue = tenant.key(t, txRedis.TransactionBackupQueue)
	tenant.receipts = tenant.key(t, "engine:"+cachepolicy.HashTag+":receipts:"+scope)
	tenant.guards = tenant.key(t, "engine:"+cachepolicy.HashTag+":guards:"+scope)
	tenant.protection = tenant.key(t, "engine:"+cachepolicy.HashTag+":protection:"+scope)
	tenant.schedule = tenant.key(t, txRedis.EngineRecoveryCleanupSchedule)

	return tenant
}

func (tenant *cleanupRunnerTenant) key(t *testing.T, name string) string {
	t.Helper()

	key, err := tmvalkey.GetKeyContext(tenant.ctx, name)
	require.NoError(t, err)

	return key
}

// acknowledge seeds one execution and completes it through the protected
// acknowledgement, which schedules its cleanup at completedAt + retention.
func (tenant *cleanupRunnerTenant) acknowledge(
	t *testing.T,
	client *redis.Client,
	repository *txRedis.RedisConsumerRepository,
	completedAt time.Time,
) (uuid.UUID, uuid.UUID) {
	t.Helper()

	executionID, transactionID := uuid.New(), uuid.New()
	field := transactionID.String() + ":" + executionID.String()
	receipt, err := json.Marshal(map[string]any{
		"formatVersion": 1, "tenantId": tenant.tenantID,
		"organizationId": tenant.organizationID.String(), "ledgerId": tenant.ledgerID.String(),
		"executionId": executionID.String(), "intentFingerprint": "runner", "response": "{}",
		"protection": map[string]any{
			"formatVersion": 1, "retentionSeconds": cleanupRunnerRetention,
			"transactions": []string{transactionID.String()}, "recoveryFields": []string{field},
			"acknowledged": map[string]bool{}, "terminalCompletedAtMs": map[string]int64{},
		},
	})
	require.NoError(t, err)

	_, err = client.Pipelined(tenant.ctx, func(pipe redis.Pipeliner) error {
		pipe.HSet(tenant.ctx, tenant.queue, field, "recovery")
		pipe.HSet(tenant.ctx, tenant.receipts, executionID.String(), receipt)
		pipe.HSet(tenant.ctx, tenant.guards, transactionID.String(), "APPROVED")
		pipe.HSet(tenant.ctx, tenant.protection, transactionID.String(), `{"formatVersion":1,"executions":{"`+executionID.String()+`":0}}`)

		return nil
	})
	require.NoError(t, err)

	status, err := repository.CompareAndDeleteRecoveryWithProtection(
		tenant.ctx, tenant.organizationID, tenant.ledgerID, field, "recovery", true, completedAt,
	)
	require.NoError(t, err)
	require.Equal(t, txRedis.RecoveryAckDeleted, status)

	return executionID, transactionID
}

func TestIntegrationEngineRecoveryCleanupRunnerDrainsEveryTenantBacklogInOnePass(t *testing.T) {
	if testing.Short() {
		t.Skip("requires Valkey")
	}

	client := recoveryEngineValkey(t)
	repository, err := txRedis.NewConsumerRedis(recoveryEngineClientProvider{client: client})
	require.NoError(t, err)

	completedAt := time.Date(2043, time.June, 7, 8, 9, 10, 0, time.UTC)
	now := completedAt.Add(cleanupRunnerRetention * time.Second)

	large, small := newCleanupRunnerTenant(t, "cleanup-runner-large"), newCleanupRunnerTenant(t, "cleanup-runner-small")
	for range 250 {
		large.acknowledge(t, client, repository, completedAt)
	}

	for range 30 {
		small.acknowledge(t, client, repository, completedAt)
	}

	// Completed one millisecond later, so it is not yet due at now.
	notDueExecution, notDueTransaction := small.acknowledge(t, client, repository, completedAt.Add(time.Millisecond))

	runner := initEngineRecoveryCleanupRunner(recoveryQuietLogger{}, repository, true, nil).
		WithTenants(staticTenants{large.tenantID, small.tenantID}).
		WithClock(func() time.Time { return now })

	// Another pod holds the lock: the tick is skipped and nothing is cleaned.
	require.NoError(t, client.Set(context.Background(), utils.EngineRecoveryCleanupLockKey(), "another-pod", time.Minute).Err())
	runner.pass(context.Background())
	require.EqualValues(t, 250, client.ZCount(context.Background(), large.schedule, "-inf", "+inf").Val())
	require.NoError(t, client.Del(context.Background(), utils.EngineRecoveryCleanupLockKey()).Err())

	runner.pass(context.Background())

	due := func(tenant *cleanupRunnerTenant) int64 {
		return client.ZCount(context.Background(), tenant.schedule, "-inf", strconv.FormatInt(now.UnixMilli(), 10)).Val()
	}
	require.Zero(t, due(large), "a single pass drains more than one page per tenant")
	require.Zero(t, due(small))
	require.Zero(t, client.HLen(context.Background(), large.receipts).Val())
	require.Zero(t, client.HLen(context.Background(), large.guards).Val())
	require.EqualValues(t, 1, client.HLen(context.Background(), small.receipts).Val(), "an execution that is not due keeps its receipt")
	require.True(t, client.HExists(context.Background(), small.receipts, notDueExecution.String()).Val())
	require.True(t, client.HExists(context.Background(), small.guards, notDueTransaction.String()).Val())
	require.EqualValues(t, 1, client.ZCard(context.Background(), small.schedule).Val())
	require.Zero(t, client.Exists(context.Background(), utils.EngineRecoveryCleanupLockKey()).Val(), "the pass releases its lock")
}
