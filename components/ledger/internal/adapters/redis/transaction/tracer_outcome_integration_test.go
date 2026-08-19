//go:build integration

// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package redis

import (
	"context"
	"encoding/json"
	"net"
	"strings"
	"testing"
	"time"

	tmcore "github.com/LerianStudio/lib-commons/v6/commons/tenant-manager/core"
	"github.com/google/uuid"
	goredis "github.com/redis/go-redis/v9"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	"github.com/LerianStudio/midaz/v4/pkg/utils"
	"github.com/LerianStudio/midaz/v4/tests/utils/chaos"
	redistestutil "github.com/LerianStudio/midaz/v4/tests/utils/redis"
)

var tracerOutcomeIntegrationFuture = time.Unix(4102444800, 0).UTC()

func TestIntegration_TracerOutcomeDiscoveryIsAtomicOnRedisCluster(t *testing.T) {
	t.Setenv("ALLOW_INSECURE_TLS", "true")
	ctx := context.Background()
	cluster := startSingleNodeRedisCluster(t, ctx)
	connection := redistestutil.CreateConnection(t, cluster.Addr)
	repo, err := NewConsumerRedis(connection)
	require.NoError(t, err)

	const tenantID = "tenant-cluster-discovery"
	tenantCtx := tmcore.ContextWithTenantID(ctx, tenantID)
	organizationID, ledgerID, transactionID := uuid.New(), uuid.New(), uuid.New()
	outcomeKey := utils.TransactionTracerOutcomeKey(organizationID, ledgerID, transactionID)
	tenantOutcomeKey, err := tenantKeyFromContextOrError(tenantCtx, outcomeKey)
	require.NoError(t, err)
	registrySlot, err := cluster.Client.Do(ctx, "CLUSTER", "KEYSLOT", utils.TracerOutcomeTenantRegistry).Int()
	require.NoError(t, err)
	outcomeSlot, err := cluster.Client.Do(ctx, "CLUSTER", "KEYSLOT", tenantOutcomeKey).Int()
	require.NoError(t, err)
	require.Equal(t, outcomeSlot, registrySlot,
		"tenant discovery and its durable outcome must share one Redis Cluster failure domain")

	operations := []mmodel.BalanceOperation{redistestutil.CreateBalanceOperationWithAvailable(
		organizationID, ledgerID, "@cluster-discovery", "USD", constant.DEBIT,
		decimal.NewFromInt(1), decimal.NewFromInt(100), "deposit",
	)}
	queue := mmodel.TransactionRedisQueue{
		TransactionID: transactionID, OrganizationID: organizationID, LedgerID: ledgerID,
		AttemptOwner: "cluster-owner", ExpectedOutcome: mmodel.TransactionOutcomeCommitted,
		TransactionStatus: constant.APPROVED,
	}
	operations = bindFinalEconomicPlan(t, &queue, operations, constant.APPROVED, false)
	seed, err := json.Marshal(queue)
	require.NoError(t, err)
	require.NoError(t, repo.AddMessageToQueue(tenantCtx,
		utils.TransactionInternalKey(organizationID, ledgerID, transactionID.String()), seed))

	preparedAt := time.Unix(1700000000, 0).UTC()
	_, err = repo.PrepareTracerOutcome(tenantCtx, organizationID, ledgerID, transactionID,
		queue.AttemptOwner, utils.TransactionTracerOutcomeID(transactionID), queue.ExpectedEconomicPlan,
		preparedAt, preparedAt.Add(time.Minute))
	require.NoError(t, err)
	registrations, err := repo.ListTracerOutcomeTenants(ctx)
	require.NoError(t, err)
	require.Equal(t, []TracerOutcomeTenantRegistration{{TenantID: tenantID, Generation: 1}}, registrations,
		"PREPARED must make tenant discovery durable in the same atomic command")
	require.NoError(t, NewFinancialRedisDurabilityGuard(connection).FinancialDurability(ctx))

	// Kill Valkey without a graceful shutdown. A new Ledger process must recover
	// the outcome and its only discovery pointer together from the same AOF.
	chaosOrchestrator := chaos.NewOrchestrator(t)
	t.Cleanup(func() { require.NoError(t, chaosOrchestrator.Close()) })
	containerID := cluster.Container.GetContainerID()
	require.NoError(t, chaosOrchestrator.KillContainer(ctx, containerID, "SIGKILL"))
	require.Eventually(t, func() bool {
		running, runningErr := chaosOrchestrator.IsContainerRunning(ctx, containerID)
		return runningErr == nil && !running
	}, 5*time.Second, 50*time.Millisecond)
	require.NoError(t, cluster.Container.Start(ctx))
	host, err := cluster.Container.Host(ctx)
	require.NoError(t, err)
	port, err := cluster.Container.MappedPort(ctx, "6379/tcp")
	require.NoError(t, err)
	restartedAddr := net.JoinHostPort(host, port.Port())
	restartedClient := goredis.NewClient(&goredis.Options{Addr: restartedAddr})
	t.Cleanup(func() { require.NoError(t, restartedClient.Close()) })
	require.Eventually(t, func() bool {
		if pingErr := restartedClient.Ping(ctx).Err(); pingErr != nil {
			return false
		}
		info, infoErr := restartedClient.ClusterInfo(ctx).Result()
		return infoErr == nil && strings.Contains(info, "cluster_state:ok")
	}, 30*time.Second, 100*time.Millisecond)

	restartedConnection := redistestutil.CreateConnection(t, restartedAddr)
	restartedRepo, err := NewConsumerRedis(restartedConnection)
	require.NoError(t, err)
	recovered, err := restartedRepo.ReadTracerOutcome(tenantCtx, organizationID, ledgerID, transactionID)
	require.NoError(t, err)
	require.NotNil(t, recovered)
	require.Equal(t, mmodel.TracerOutcomePrepared, recovered.State)
	recoveredRegistrations, err := restartedRepo.ListTracerOutcomeTenants(ctx)
	require.NoError(t, err)
	require.Equal(t, registrations, recoveredRegistrations,
		"an abrupt restart must never recover an undiscoverable tenant outcome")
}

func TestIntegration_TracerOutcomeTenantRetirementCannotRaceNewPrepare(t *testing.T) {
	t.Setenv("ALLOW_INSECURE_TLS", "true")
	infra := setupFinancialRedisIntegrationInfra(t)
	const tenantID = "tenant-generation-cas"
	ctx := tmcore.ContextWithTenantID(context.Background(), tenantID)
	organizationID, ledgerID := uuid.New(), uuid.New()
	preparedAt := time.Unix(1700000000, 0).UTC()
	prepare := func(transactionID uuid.UUID, address string) (string, uuid.UUID) {
		t.Helper()
		owner := uuid.NewString()
		operations := []mmodel.BalanceOperation{redistestutil.CreateBalanceOperationWithAvailable(
			organizationID, ledgerID, address, "USD", constant.DEBIT,
			decimal.NewFromInt(1), decimal.NewFromInt(100), "deposit",
		)}
		queue := mmodel.TransactionRedisQueue{
			TransactionID: transactionID, OrganizationID: organizationID, LedgerID: ledgerID,
			AttemptOwner: owner, ExpectedOutcome: mmodel.TransactionOutcomeCommitted,
			TransactionStatus: constant.APPROVED,
		}
		operations = bindFinalEconomicPlan(t, &queue, operations, constant.APPROVED, false)
		seed, marshalErr := json.Marshal(queue)
		require.NoError(t, marshalErr)
		require.NoError(t, infra.repo.AddMessageToQueue(ctx,
			utils.TransactionInternalKey(organizationID, ledgerID, transactionID.String()), seed))
		outcomeID := utils.TransactionTracerOutcomeID(transactionID)
		_, prepareErr := infra.repo.PrepareTracerOutcome(ctx, organizationID, ledgerID, transactionID,
			owner, outcomeID, queue.ExpectedEconomicPlan, preparedAt, preparedAt.Add(time.Minute))
		require.NoError(t, prepareErr)
		return utils.TransactionTracerOutcomeKey(organizationID, ledgerID, transactionID), outcomeID
	}

	firstOutcomeKey, firstOutcomeID := prepare(uuid.New(), "@tenant-generation-first")
	registrations, err := infra.repo.ListTracerOutcomeTenants(context.Background())
	require.NoError(t, err)
	require.Equal(t, []TracerOutcomeTenantRegistration{{TenantID: tenantID, Generation: 1}}, registrations)
	hasBacklog, err := infra.repo.TracerOutcomeTenantHasBacklog(ctx)
	require.NoError(t, err)
	require.True(t, hasBacklog)
	delivered, err := infra.repo.MarkTracerOutcomeDelivered(ctx, firstOutcomeKey, firstOutcomeID,
		mmodel.TracerOutcomePrepared, preparedAt.Add(time.Minute), time.Hour)
	require.NoError(t, err)
	require.True(t, delivered)
	hasBacklog, err = infra.repo.TracerOutcomeTenantHasBacklog(ctx)
	require.NoError(t, err)
	require.False(t, hasBacklog)

	// Simulate a producer entering after the worker observed generation 1 but
	// before it executes retirement. Prepare must atomically recreate backlog
	// and advance the generation before the stale retirement can run.
	secondOutcomeKey, secondOutcomeID := prepare(uuid.New(), "@tenant-generation-second")
	retired, err := infra.repo.RetireTracerOutcomeTenant(ctx, tenantID, 1)
	require.NoError(t, err)
	require.False(t, retired)
	delivered, err = infra.repo.MarkTracerOutcomeDelivered(ctx, secondOutcomeKey, secondOutcomeID,
		mmodel.TracerOutcomePrepared, preparedAt.Add(2*time.Minute), time.Hour)
	require.NoError(t, err)
	require.True(t, delivered)
	retired, err = infra.repo.RetireTracerOutcomeTenant(ctx, tenantID, 1)
	require.NoError(t, err)
	require.False(t, retired, "a stale observed generation cannot remove a newer registration")
	registrations, err = infra.repo.ListTracerOutcomeTenants(context.Background())
	require.NoError(t, err)
	require.Equal(t, []TracerOutcomeTenantRegistration{{TenantID: tenantID, Generation: 2}}, registrations)

	hasBacklog, err = infra.repo.TracerOutcomeTenantHasBacklog(ctx)
	require.NoError(t, err)
	require.False(t, hasBacklog)
	retired, err = infra.repo.RetireTracerOutcomeTenant(ctx, tenantID, 2)
	require.NoError(t, err)
	require.True(t, retired)
}

func TestIntegration_TracerOutcomePrepareFailureLeavesDiscoverableFalsePositive(t *testing.T) {
	t.Setenv("ALLOW_INSECURE_TLS", "true")
	infra := setupFinancialRedisIntegrationInfra(t)
	const tenantID = "tenant-failed-prepare"
	ctx := tmcore.ContextWithTenantID(context.Background(), tenantID)
	organizationID, ledgerID, transactionID := uuid.New(), uuid.New(), uuid.New()
	operations := []mmodel.BalanceOperation{redistestutil.CreateBalanceOperationWithAvailable(
		organizationID, ledgerID, "@failed-prepare", "USD", constant.DEBIT,
		decimal.NewFromInt(1), decimal.NewFromInt(100), "deposit",
	)}
	queue := mmodel.TransactionRedisQueue{
		TransactionID: transactionID, OrganizationID: organizationID, LedgerID: ledgerID,
		AttemptOwner: "failed-prepare-owner", ExpectedOutcome: mmodel.TransactionOutcomeCommitted,
		TransactionStatus: constant.APPROVED,
	}
	operations = bindFinalEconomicPlan(t, &queue, operations, constant.APPROVED, false)
	seed, err := json.Marshal(queue)
	require.NoError(t, err)
	require.NoError(t, infra.repo.AddMessageToQueue(ctx,
		utils.TransactionInternalKey(organizationID, ledgerID, transactionID.String()), seed))

	// Redis Lua does not roll back commands that ran before a runtime error. A
	// corrupt schedule therefore exercises the only safe partial result: tenant
	// discovery and active evidence survive even though PREPARED does not.
	scheduleKey, err := tenantKeyFromContextOrError(ctx, utils.TracerOutcomeScheduleKey)
	require.NoError(t, err)
	rds, err := infra.repo.conn.GetClient(ctx)
	require.NoError(t, err)
	require.NoError(t, rds.Set(ctx, scheduleKey, "wrong-type", 0).Err())
	preparedAt := time.Unix(1700000000, 0).UTC()
	_, err = infra.repo.PrepareTracerOutcome(ctx, organizationID, ledgerID, transactionID,
		queue.AttemptOwner, utils.TransactionTracerOutcomeID(transactionID), queue.ExpectedEconomicPlan,
		preparedAt, preparedAt.Add(time.Minute))
	require.ErrorContains(t, err, "WRONGTYPE")
	record, err := infra.repo.ReadTracerOutcome(ctx, organizationID, ledgerID, transactionID)
	require.NoError(t, err)
	require.Nil(t, record)
	registrations, err := infra.repo.ListTracerOutcomeTenants(context.Background())
	require.NoError(t, err)
	require.Equal(t, []TracerOutcomeTenantRegistration{{TenantID: tenantID, Generation: 1}}, registrations)
	hasBacklog, err := infra.repo.TracerOutcomeTenantHasBacklog(ctx)
	require.NoError(t, err)
	require.True(t, hasBacklog, "a partial script failure may strand work, never hide it")
}

func TestIntegration_FinancialRedisDurabilityGuardAcceptsDocumentedRPO(t *testing.T) {
	t.Setenv("ALLOW_INSECURE_TLS", "true")
	cfg := redistestutil.FinancialContainerConfig()
	cfg.AppendFsync = "everysec"
	container := redistestutil.SetupContainerWithConfig(t, cfg)
	connection := redistestutil.CreateConnection(t, container.Addr)
	guard := NewFinancialRedisDurabilityGuard(connection)

	require.Eventually(t, func() bool {
		return guard.FinancialDurability(context.Background()) == nil
	}, 5*time.Second, 50*time.Millisecond)

	require.NoError(t, container.Client.ConfigSet(context.Background(), "appendonly", "no").Err())
	require.ErrorContains(t, guard.FinancialDurability(context.Background()), "appendonly must be enabled")
}

func TestIntegration_TracerOutcomeMovesWithBalanceAndFencesStaleRecovery(t *testing.T) {
	t.Setenv("ALLOW_INSECURE_TLS", "true")
	infra := setupFinancialRedisIntegrationInfra(t)
	const tenantID = "tenant-outcome-restart"
	ctx := tmcore.ContextWithTenantID(context.Background(), tenantID)
	organizationID, ledgerID, transactionID := uuid.New(), uuid.New(), uuid.New()
	owner := uuid.NewString()
	outcomeID := utils.TransactionTracerOutcomeID(transactionID)
	amount := decimal.RequireFromString("0.000000000000000001")
	available := decimal.RequireFromString("1000.123456789012345678")

	operations := []mmodel.BalanceOperation{redistestutil.CreateBalanceOperationWithAvailable(
		organizationID, ledgerID, "@tracer-outcome", "USD", constant.DEBIT, amount, available, "deposit",
	)}
	queue := mmodel.TransactionRedisQueue{
		TransactionID: transactionID, OrganizationID: organizationID, LedgerID: ledgerID,
		AttemptOwner: owner, ExpectedOutcome: mmodel.TransactionOutcomeCommitted,
		TransactionStatus: constant.APPROVED,
	}
	operations = bindFinalEconomicPlan(t, &queue, operations, constant.APPROVED, false)
	seed, err := json.Marshal(queue)
	require.NoError(t, err)
	require.NoError(t, infra.repo.AddMessageToQueue(ctx,
		utils.TransactionInternalKey(organizationID, ledgerID, transactionID.String()), seed))
	preparedAt := time.Unix(1700000000, 0).UTC()
	prepared, err := infra.repo.PrepareTracerOutcome(ctx, organizationID, ledgerID, transactionID,
		owner, outcomeID, queue.ExpectedEconomicPlan, preparedAt, preparedAt.Add(time.Minute))
	require.NoError(t, err)
	require.Equal(t, mmodel.TracerOutcomePrepared, prepared.State)

	restartedRepo := &RedisConsumerRepository{conn: infra.repo.conn}
	registrations, err := restartedRepo.ListTracerOutcomeTenants(context.Background())
	require.NoError(t, err)
	require.Contains(t, registrations, TracerOutcomeTenantRegistration{TenantID: tenantID, Generation: 1},
		"a fresh process must discover backlog without a process-local tenant cache")
	hasBacklog, err := restartedRepo.TracerOutcomeTenantHasBacklog(ctx)
	require.NoError(t, err)
	require.True(t, hasBacklog)

	attempt := mmodel.BalanceExecutionAttempt{
		ExecutionKey: utils.TransactionBalanceExecutionKey(organizationID, ledgerID, transactionID),
		OutcomeKey:   utils.TransactionBalanceOutcomeKey(organizationID, ledgerID, transactionID),
		Owner:        owner, Outcome: mmodel.TransactionOutcomeCommitted, Identity: transactionID,
		TracerOutcomeID: outcomeID, TracerOutcomeState: mmodel.TracerOutcomeCommitted,
	}
	first, err := infra.repo.ProcessOutcomeBalanceAtomicOperation(ctx, organizationID, ledgerID, transactionID,
		constant.APPROVED, false, operations, attempt)
	require.NoError(t, err)
	require.Len(t, first.After, 1)
	assert.True(t, available.Sub(amount).Equal(first.After[0].Available), "Lua must preserve the exact decimal movement")

	durable, err := infra.repo.ReadTracerOutcome(ctx, organizationID, ledgerID, transactionID)
	require.NoError(t, err)
	require.Equal(t, mmodel.TracerOutcomeCommitted, durable.State)
	require.NotNil(t, durable.EconomicOutcome)
	assert.Equal(t, queue.ExpectedEconomicPlan.Digest, durable.EconomicOutcome.EconomicPlanDigest)
	assert.True(t, durable.EconomicOutcome.After[0].Available.Equal(available.Sub(amount)))

	due, err := infra.repo.ListDueTracerOutcomes(ctx, tracerOutcomeIntegrationFuture, 10)
	require.NoError(t, err)
	assert.Contains(t, due, utils.TransactionTracerOutcomeKey(organizationID, ledgerID, transactionID))

	replay, err := infra.repo.ProcessOutcomeBalanceAtomicOperation(ctx, organizationID, ledgerID, transactionID,
		constant.APPROVED, false, operations, attempt)
	require.NoError(t, err)
	require.Equal(t, first, replay)
	staleAttempt := attempt
	staleAttempt.Owner = uuid.NewString()
	_, err = infra.repo.ProcessOutcomeBalanceAtomicOperation(ctx, organizationID, ledgerID, transactionID,
		constant.APPROVED, false, operations, staleAttempt)
	require.ErrorContains(t, err, "STALE_EXECUTOR")

	_, err = infra.repo.AbortPreparedTracerOutcome(ctx, organizationID, ledgerID, transactionID,
		owner, outcomeID, tracerOutcomeIntegrationFuture)
	require.ErrorContains(t, err, "STALE_EXECUTOR")

	balanceRaw, err := infra.repo.Get(ctx, operations[0].InternalKey)
	require.NoError(t, err)
	assert.Contains(t, balanceRaw, available.Sub(amount).String(), "replay and stale recovery must not move funds twice")

	retryAt := tracerOutcomeIntegrationFuture
	require.NoError(t, infra.repo.RescheduleTracerOutcome(ctx,
		utils.TransactionTracerOutcomeKey(organizationID, ledgerID, transactionID), outcomeID,
		mmodel.TracerOutcomeCommitted, "ack lost", retryAt.Add(-time.Minute), retryAt))
	due, err = infra.repo.ListDueTracerOutcomes(ctx, retryAt.Add(-time.Second), 10)
	require.NoError(t, err)
	assert.NotContains(t, due, utils.TransactionTracerOutcomeKey(organizationID, ledgerID, transactionID))
	delivered, err := infra.repo.MarkTracerOutcomeDelivered(ctx,
		utils.TransactionTracerOutcomeKey(organizationID, ledgerID, transactionID), outcomeID,
		mmodel.TracerOutcomeCommitted, retryAt, time.Hour)
	require.NoError(t, err)
	require.True(t, delivered)
	hasBacklog, err = infra.repo.TracerOutcomeTenantHasBacklog(ctx)
	require.NoError(t, err)
	require.False(t, hasBacklog)
	retired, err := infra.repo.RetireTracerOutcomeTenant(ctx, tenantID, 1)
	require.NoError(t, err)
	require.True(t, retired)
	durable, err = infra.repo.ReadTracerOutcome(ctx, organizationID, ledgerID, transactionID)
	require.NoError(t, err)
	assert.Equal(t, mmodel.TracerOutcomeDelivered, durable.State)
	replay, err = infra.repo.ProcessOutcomeBalanceAtomicOperation(ctx, organizationID, ledgerID, transactionID,
		constant.APPROVED, false, operations, attempt)
	require.NoError(t, err, "a lost Lua response remains replayable after the dispatcher has already acknowledged delivery")
	require.Equal(t, first, replay)
}

func TestIntegration_TracerOutcomePendingHeldTransitionsOnlyOnCancelLua(t *testing.T) {
	t.Setenv("ALLOW_INSECURE_TLS", "true")
	infra := setupFinancialRedisIntegrationInfra(t)
	ctx := context.Background()
	organizationID, ledgerID, transactionID := uuid.New(), uuid.New(), uuid.New()
	outcomeID := utils.TransactionTracerOutcomeID(transactionID)
	owner := uuid.NewString()

	operations := []mmodel.BalanceOperation{redistestutil.CreateBalanceOperationWithAvailable(
		organizationID, ledgerID, "@pending-outcome", "USD", constant.ONHOLD,
		decimal.NewFromInt(100), decimal.NewFromInt(1000), "deposit",
	)}
	pendingQueue := mmodel.TransactionRedisQueue{
		TransactionID: transactionID, OrganizationID: organizationID, LedgerID: ledgerID,
		AttemptOwner: owner, ExpectedOutcome: mmodel.TransactionOutcomeCommitted,
		TransactionStatus: constant.PENDING,
	}
	operations = bindFinalEconomicPlan(t, &pendingQueue, operations, constant.PENDING, true)
	seed, err := json.Marshal(pendingQueue)
	require.NoError(t, err)
	transactionKey := utils.TransactionInternalKey(organizationID, ledgerID, transactionID.String())
	require.NoError(t, infra.repo.AddMessageToQueue(ctx, transactionKey, seed))
	preparedAt := time.Unix(1700000000, 0).UTC()
	_, err = infra.repo.PrepareTracerOutcome(ctx, organizationID, ledgerID, transactionID,
		owner, outcomeID, pendingQueue.ExpectedEconomicPlan, preparedAt, preparedAt.Add(time.Minute))
	require.NoError(t, err)
	pendingAttempt := mmodel.BalanceExecutionAttempt{
		ExecutionKey: utils.TransactionPendingBalanceExecutionKey(organizationID, ledgerID, transactionID),
		OutcomeKey:   utils.TransactionPendingBalanceOutcomeKey(organizationID, ledgerID, transactionID),
		Owner:        owner, Outcome: mmodel.TransactionOutcomeCommitted, Identity: transactionID,
		Action:          constant.ActionHold,
		TracerOutcomeID: outcomeID, TracerOutcomeState: mmodel.TracerOutcomePendingHeld,
	}
	pendingResult, err := infra.repo.ProcessOutcomeBalanceAtomicOperation(ctx, organizationID, ledgerID, transactionID,
		constant.PENDING, true, operations, pendingAttempt)
	require.NoError(t, err)
	require.NotNil(t, pendingResult)
	pendingRecord, err := infra.repo.ReadTracerOutcome(ctx, organizationID, ledgerID, transactionID)
	require.NoError(t, err)
	assert.Equal(t, mmodel.TracerOutcomePendingHeld, pendingRecord.State)
	hasBacklog, err := infra.repo.TracerOutcomeTenantHasBacklog(ctx)
	require.NoError(t, err)
	assert.True(t, hasBacklog, "PENDING_HELD stays in the active index while it is intentionally absent from delivery")
	due, err := infra.repo.ListDueTracerOutcomes(ctx, tracerOutcomeIntegrationFuture, 10)
	require.NoError(t, err)
	assert.NotContains(t, due, utils.TransactionTracerOutcomeKey(organizationID, ledgerID, transactionID),
		"PENDING_HELD must never enter recovery or delivery")
	_, err = infra.repo.AbortPreparedTracerOutcome(ctx, organizationID, ledgerID, transactionID,
		owner, outcomeID, tracerOutcomeIntegrationFuture)
	require.ErrorContains(t, err, "STALE_EXECUTOR")

	// Simulate the normal pending persistence handoff: the economic transition
	// receipt is finalized, while the dedicated tracer outbox remains held.
	require.NoError(t, infra.repo.Del(ctx, pendingAttempt.OutcomeKey))
	cancelOwner := uuid.NewString()
	cancelExecutionKey := utils.TransactionBalanceExecutionKey(organizationID, ledgerID, transactionID)
	cancelOutcomeKey := utils.TransactionBalanceOutcomeKey(organizationID, ledgerID, transactionID)
	acquired, err := infra.repo.AcquireOwnedKey(ctx, cancelExecutionKey, cancelOwner, 300)
	require.NoError(t, err)
	require.True(t, acquired)
	for index := range operations {
		operations[index].ExpectedEconomicPlan = nil
	}
	cancelQueue := mmodel.TransactionRedisQueue{
		TransactionID: transactionID, OrganizationID: organizationID, LedgerID: ledgerID,
		AttemptOwner: cancelOwner, ExpectedOutcome: mmodel.TransactionOutcomeAborted,
		TransactionStatus: constant.CANCELED,
	}
	operations = bindFinalEconomicPlan(t, &cancelQueue, operations, constant.CANCELED, false)
	seed, err = json.Marshal(cancelQueue)
	require.NoError(t, err)
	require.NoError(t, infra.repo.AddMessageToQueue(ctx, transactionKey, seed))
	cancelAttempt := mmodel.BalanceExecutionAttempt{
		ExecutionKey: cancelExecutionKey, OutcomeKey: cancelOutcomeKey,
		Owner: cancelOwner, Outcome: mmodel.TransactionOutcomeAborted, Identity: transactionID,
		TracerOutcomeID: outcomeID, TracerOutcomeState: mmodel.TracerOutcomeAborted,
	}
	_, err = infra.repo.ProcessOutcomeBalanceAtomicOperation(ctx, organizationID, ledgerID, transactionID,
		constant.CANCELED, false, operations, cancelAttempt)
	require.NoError(t, err)
	canceledRecord, err := infra.repo.ReadTracerOutcome(ctx, organizationID, ledgerID, transactionID)
	require.NoError(t, err)
	assert.Equal(t, mmodel.TracerOutcomeAborted, canceledRecord.State)
	assert.Equal(t, cancelQueue.ExpectedEconomicPlan.Digest, canceledRecord.EconomicPlanDigest,
		"the terminal lifecycle plan replaces the pending hold plan")
	due, err = infra.repo.ListDueTracerOutcomes(ctx, tracerOutcomeIntegrationFuture, 10)
	require.NoError(t, err)
	assert.Contains(t, due, utils.TransactionTracerOutcomeKey(organizationID, ledgerID, transactionID))
}
