//go:build integration

// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package redis

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/LerianStudio/midaz/v3/pkg"
	"github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/LerianStudio/midaz/v3/pkg/shard"
	"github.com/LerianStudio/midaz/v3/pkg/utils"
	redistestutil "github.com/LerianStudio/midaz/v3/tests/utils/redis"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// =============================================================================
// SHARDED TEST INFRASTRUCTURE
// =============================================================================

// shardedTestInfra holds infrastructure for sharded Redis integration tests.
type shardedTestInfra struct {
	redisContainer *redistestutil.ContainerResult
	repo           *RedisConsumerRepository
	router         *shard.Router
}

// setupShardedInfra creates a Redis container with sharding enabled (8 shards).
func setupShardedInfra(t *testing.T) *shardedTestInfra {
	t.Helper()

	redisContainer := redistestutil.SetupContainer(t)
	conn := redistestutil.CreateConnection(t, redisContainer.Addr)
	router := shard.NewRouter(8)

	repo := &RedisConsumerRepository{
		conn:               conn,
		balanceSyncEnabled: true,
		shardingEnabled:    true,
		shardRouter:        router,
	}

	return &shardedTestInfra{
		redisContainer: redisContainer,
		repo:           repo,
		router:         router,
	}
}

// =============================================================================
// INTEGRATION TESTS — SINGLE-SHARD (FAST PATH)
// =============================================================================

// TestIntegration_Sharded_SingleShard_Debit tests the fast path where all operations
// land on the same shard (65% of traffic with external pre-split).
func TestIntegration_Sharded_SingleShard_Debit(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	infra := setupShardedInfra(t)

	ctx := context.Background()
	orgID := uuid.New()
	ledgerID := uuid.New()
	txID := uuid.New()

	// Pick an alias and compute its shard deterministically
	alias := "@alice"
	shardID := infra.router.Resolve(alias)

	ops := []mmodel.BalanceOperation{
		redistestutil.CreateShardedBalanceOperation(
			orgID, ledgerID, alias, "USD",
			constant.DEBIT, decimal.NewFromInt(100),
			decimal.NewFromInt(1000), "deposit", shardID,
		),
	}

	balances, err := infra.repo.ProcessBalanceAtomicOperation(
		ctx, orgID, ledgerID, txID, "ACTIVE", false, ops,
	)

	require.NoError(t, err, "single-shard debit should succeed")
	require.Len(t, balances, 1, "should return 1 balance")

	// The Lua returns pre-update state with alias set
	assert.Equal(t, alias, balances[0].Alias, "alias should be @alice")

	internalKey := utils.BalanceShardKey(shardID, orgID, ledgerID, alias+"#default")
	stored := readBalanceRedisByKey(t, ctx, infra, internalKey)
	assert.True(t, stored.Available.Equal(decimal.NewFromInt(900)), "debit should reduce available by 100")
	assert.True(t, stored.OnHold.Equal(decimal.Zero), "on-hold should stay zero for ACTIVE debit")
	assert.Equal(t, int64(2), stored.Version, "version should increment from 1 to 2")

	t.Logf("Single-shard debit: alias=%s shard=%d", alias, shardID)
}

// TestIntegration_Sharded_SingleShard_CreditDebit tests a debit+credit pair
// that both hash to the same shard (same-shard transaction).
func TestIntegration_Sharded_SingleShard_CreditDebit(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	infra := setupShardedInfra(t)

	ctx := context.Background()
	orgID := uuid.New()
	ledgerID := uuid.New()
	txID := uuid.New()

	// Find two aliases that hash to the same shard
	sender, receiver := findSameShardAliases(infra.router)
	shardID := infra.router.Resolve(sender)

	t.Logf("Same-shard aliases: sender=%s receiver=%s shard=%d", sender, receiver, shardID)

	ops := []mmodel.BalanceOperation{
		redistestutil.CreateShardedBalanceOperation(
			orgID, ledgerID, sender, "USD",
			constant.DEBIT, decimal.NewFromInt(200),
			decimal.NewFromInt(1000), "deposit", shardID,
		),
		redistestutil.CreateShardedBalanceOperation(
			orgID, ledgerID, receiver, "USD",
			constant.CREDIT, decimal.NewFromInt(200),
			decimal.NewFromInt(500), "deposit", shardID,
		),
	}

	balances, err := infra.repo.ProcessBalanceAtomicOperation(
		ctx, orgID, ledgerID, txID, "ACTIVE", false, ops,
	)

	require.NoError(t, err, "same-shard debit+credit should succeed")
	require.Len(t, balances, 2, "should return 2 balances")
}

// TestIntegration_Sharded_SingleShard_InsufficientFunds verifies that a debit
// exceeding available balance correctly returns ErrInsufficientFunds.
func TestIntegration_Sharded_SingleShard_InsufficientFunds(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	infra := setupShardedInfra(t)

	ctx := context.Background()
	orgID := uuid.New()
	ledgerID := uuid.New()
	txID := uuid.New()

	alias := "@broke"
	shardID := infra.router.Resolve(alias)

	ops := []mmodel.BalanceOperation{
		redistestutil.CreateShardedBalanceOperation(
			orgID, ledgerID, alias, "USD",
			constant.DEBIT, decimal.NewFromInt(9999), // exceeds 1000 available
			decimal.NewFromInt(1000), "deposit", shardID,
		),
	}

	_, err := infra.repo.ProcessBalanceAtomicOperation(
		ctx, orgID, ledgerID, txID, "ACTIVE", false, ops,
	)

	require.Error(t, err, "should fail with insufficient funds")
	redistestutil.AssertInsufficientFundsError(t, err)
}

func TestIntegration_Sharded_ConcurrentDebits_NoOverspend(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	infra := setupShardedInfra(t)

	ctx := context.Background()
	orgID := uuid.New()
	ledgerID := uuid.New()

	alias := "@concurrent"
	shardID := infra.router.Resolve(alias)

	const (
		workers       = 20
		debitAmount   = 100
		startBalance  = 1000
		expectedWin   = 10
		expectedFails = workers - expectedWin
	)

	var wg sync.WaitGroup
	var successCount int32
	var insufficientCount int32

	errCh := make(chan error, workers)

	for i := 0; i < workers; i++ {
		wg.Add(1)

		go func() {
			defer wg.Done()

			ops := []mmodel.BalanceOperation{
				redistestutil.CreateShardedBalanceOperation(
					orgID, ledgerID, alias, "USD",
					constant.DEBIT, decimal.NewFromInt(debitAmount),
					decimal.NewFromInt(startBalance), "deposit", shardID,
				),
			}

			_, err := infra.repo.ProcessBalanceAtomicOperation(
				ctx, orgID, ledgerID, uuid.New(), "ACTIVE", false, ops,
			)
			if err == nil {
				atomic.AddInt32(&successCount, 1)

				return
			}

			var unprocessableErr pkg.UnprocessableOperationError
			if errors.As(err, &unprocessableErr) && unprocessableErr.Code == constant.ErrInsufficientFunds.Error() {
				atomic.AddInt32(&insufficientCount, 1)

				return
			}

			errCh <- err
		}()
	}

	wg.Wait()
	close(errCh)

	for err := range errCh {
		require.NoError(t, err)
	}

	assert.Equal(t, int32(expectedWin), successCount, "exactly 10 debits should succeed")
	assert.Equal(t, int32(expectedFails), insufficientCount, "remaining debits should fail with insufficient funds")

	internalKey := utils.BalanceShardKey(shardID, orgID, ledgerID, alias+"#default")
	stored := readBalanceRedisByKey(t, ctx, infra, internalKey)

	assert.True(t, stored.Available.Equal(decimal.Zero), "final available must never go below zero")
	assert.Equal(t, int64(11), stored.Version, "version should increment only for successful debits")
}

func TestIntegration_Sharded_ListBalanceByKey_ExternalPreSplitUsesBalanceKeyShard(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	infra := setupShardedInfra(t)

	ctx := context.Background()
	orgID := uuid.New()
	ledgerID := uuid.New()

	alias := "@external/USD"
	balanceKey := "shard_3"
	lookupKey := alias + "#" + balanceKey
	targetShard := infra.router.ResolveBalance(alias, balanceKey)

	internalKey := utils.BalanceShardKey(targetShard, orgID, ledgerID, lookupKey)

	seed := mmodel.BalanceRedis{
		ID:             uuid.NewString(),
		Alias:          alias,
		AccountID:      uuid.NewString(),
		AssetCode:      "USD",
		Available:      decimal.NewFromInt(12345),
		OnHold:         decimal.Zero,
		Version:        7,
		AccountType:    "external",
		AllowSending:   1,
		AllowReceiving: 1,
		Key:            balanceKey,
		Scale:          2,
	}

	payload, err := json.Marshal(seed)
	require.NoError(t, err)
	require.NoError(t, infra.redisContainer.Client.Set(ctx, internalKey, payload, time.Hour).Err())

	balance, err := infra.repo.ListBalanceByKey(ctx, orgID, ledgerID, lookupKey)
	require.NoError(t, err)
	require.NotNil(t, balance)

	assert.Equal(t, alias, balance.Alias)
	assert.Equal(t, balanceKey, balance.Key)
	assert.True(t, balance.Available.Equal(decimal.RequireFromString("123.45")))
}

// =============================================================================
// INTEGRATION TESTS — CROSS-SHARD
// =============================================================================

// TestIntegration_Sharded_CrossShard_DebitCredit tests the cross-shard path
// where debit and credit land on different shards.
func TestIntegration_Sharded_CrossShard_DebitCredit(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	infra := setupShardedInfra(t)

	ctx := context.Background()
	orgID := uuid.New()
	ledgerID := uuid.New()
	txID := uuid.New()

	// Find two aliases that hash to DIFFERENT shards
	sender, receiver := findCrossShardAliases(infra.router)
	senderShard := infra.router.Resolve(sender)
	receiverShard := infra.router.Resolve(receiver)

	t.Logf("Cross-shard: sender=%s(shard=%d) receiver=%s(shard=%d)", sender, senderShard, receiver, receiverShard)

	ops := []mmodel.BalanceOperation{
		redistestutil.CreateShardedBalanceOperation(
			orgID, ledgerID, sender, "USD",
			constant.DEBIT, decimal.NewFromInt(300),
			decimal.NewFromInt(1000), "deposit", senderShard,
		),
		redistestutil.CreateShardedBalanceOperation(
			orgID, ledgerID, receiver, "USD",
			constant.CREDIT, decimal.NewFromInt(300),
			decimal.NewFromInt(500), "deposit", receiverShard,
		),
	}

	balances, err := infra.repo.ProcessBalanceAtomicOperation(
		ctx, orgID, ledgerID, txID, "ACTIVE", false, ops,
	)

	require.NoError(t, err, "cross-shard debit+credit should succeed")
	require.Len(t, balances, 2, "should return 2 balances")

	// Verify both aliases are present
	aliases := map[string]bool{}
	for _, b := range balances {
		aliases[b.Alias] = true
	}

	assert.True(t, aliases[sender], "sender alias should be in results")
	assert.True(t, aliases[receiver], "receiver alias should be in results")

	senderKey := utils.BalanceShardKey(senderShard, orgID, ledgerID, sender+"#default")
	receiverKey := utils.BalanceShardKey(receiverShard, orgID, ledgerID, receiver+"#default")

	senderState := readBalanceRedisByKey(t, ctx, infra, senderKey)
	receiverState := readBalanceRedisByKey(t, ctx, infra, receiverKey)

	assert.True(t, senderState.Available.Equal(decimal.NewFromInt(700)), "sender available must be debited by 300")
	assert.True(t, receiverState.Available.Equal(decimal.NewFromInt(800)), "receiver available must be credited by 300")
	assert.True(t, senderState.OnHold.Equal(decimal.Zero), "sender on-hold remains zero on ACTIVE flow")
	assert.True(t, receiverState.OnHold.Equal(decimal.Zero), "receiver on-hold remains zero on ACTIVE flow")
	assert.Equal(t, int64(2), senderState.Version, "sender version should increment")
	assert.Equal(t, int64(2), receiverState.Version, "receiver version should increment")
}

// TestIntegration_Sharded_CrossShard_DebitFails_CreditNotExecuted tests that
// when a debit fails (insufficient funds), the credit on another shard is never
// executed (debit-first-credit-second protocol).
func TestIntegration_Sharded_CrossShard_DebitFails_CreditNotExecuted(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	infra := setupShardedInfra(t)

	ctx := context.Background()
	orgID := uuid.New()
	ledgerID := uuid.New()
	txID := uuid.New()

	sender, receiver := findCrossShardAliases(infra.router)
	senderShard := infra.router.Resolve(sender)
	receiverShard := infra.router.Resolve(receiver)

	ops := []mmodel.BalanceOperation{
		redistestutil.CreateShardedBalanceOperation(
			orgID, ledgerID, sender, "USD",
			constant.DEBIT, decimal.NewFromInt(99999), // WAY more than 1000 available
			decimal.NewFromInt(1000), "deposit", senderShard,
		),
		redistestutil.CreateShardedBalanceOperation(
			orgID, ledgerID, receiver, "USD",
			constant.CREDIT, decimal.NewFromInt(99999),
			decimal.NewFromInt(500), "deposit", receiverShard,
		),
	}

	_, err := infra.repo.ProcessBalanceAtomicOperation(
		ctx, orgID, ledgerID, txID, "ACTIVE", false, ops,
	)

	require.Error(t, err, "should fail because debit exceeds balance")
	redistestutil.AssertInsufficientFundsError(t, err)

	// Verify the receiver's balance was never written to Redis
	// (credit shard should not have been touched)
	receiverKey := utils.BalanceShardKey(receiverShard, orgID, ledgerID, receiver+"#default")
	exists, err := infra.redisContainer.Client.Exists(ctx, receiverKey).Result()
	require.NoError(t, err)
	assert.Zero(t, exists, "receiver balance should NOT exist in Redis (credit never executed)")
}

// TestIntegration_Sharded_CrossShard_Compensation tests that when two debits
// succeed but a subsequent credit-shard fails, the debits are compensated.
func TestIntegration_Sharded_CrossShard_Compensation(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	infra := setupShardedInfra(t)

	ctx := context.Background()
	orgID := uuid.New()
	ledgerID := uuid.New()

	// First: establish a sender balance in Redis with a setup credit.
	sender, receiver := findCrossShardAliases(infra.router)
	senderShard := infra.router.Resolve(sender)
	receiverShard := infra.router.Resolve(receiver)

	setupTxID := uuid.New()
	setupOps := []mmodel.BalanceOperation{
		redistestutil.CreateShardedBalanceOperation(
			orgID, ledgerID, sender, "USD",
			constant.CREDIT, decimal.NewFromInt(1000),
			decimal.Zero, "deposit", senderShard,
		),
	}

	_, err := infra.repo.ProcessBalanceAtomicOperation(
		ctx, orgID, ledgerID, setupTxID, "ACTIVE", false, setupOps,
	)
	require.NoError(t, err, "setup credit should succeed")

	// Read pre-state (after setup) for strong compensation assertions.
	senderKey := utils.BalanceShardKey(senderShard, orgID, ledgerID, sender+"#default")
	val, err := infra.redisContainer.Client.Get(ctx, senderKey).Result()
	require.NoError(t, err, "sender balance should exist after setup")
	require.NotEmpty(t, val, "sender balance should have data")

	var before mmodel.BalanceRedis
	require.NoError(t, json.Unmarshal([]byte(val), &before))

	// Trigger cross-shard compensation: debit succeeds on sender shard, then credit
	// fails on receiver shard due to external-account positive-balance guard.
	compTxID := uuid.New()
	debitAmount := decimal.NewFromInt(250)
	crossOps := []mmodel.BalanceOperation{
		redistestutil.CreateShardedBalanceOperation(
			orgID, ledgerID, sender, "USD",
			constant.DEBIT, debitAmount,
			before.Available, "deposit", senderShard,
		),
		func() mmodel.BalanceOperation {
			op := redistestutil.CreateShardedBalanceOperation(
				orgID, ledgerID, receiver, "USD",
				constant.CREDIT, debitAmount,
				decimal.Zero, "external", receiverShard,
			)
			op.Balance.AccountType = "external"
			return op
		}(),
	}

	_, err = infra.repo.ProcessBalanceAtomicOperation(
		ctx, orgID, ledgerID, compTxID, "ACTIVE", false, crossOps,
	)
	require.Error(t, err, "credit phase should fail and trigger compensation")
	redistestutil.AssertInsufficientFundsError(t, err)

	val, err = infra.redisContainer.Client.Get(ctx, senderKey).Result()
	require.NoError(t, err, "sender balance should still exist after compensation")

	var after mmodel.BalanceRedis
	require.NoError(t, json.Unmarshal([]byte(val), &after))

	assert.True(t, after.Available.Equal(before.Available), "sender available should be fully restored after compensation")
	assert.True(t, after.OnHold.Equal(before.OnHold), "sender on-hold should be fully restored after compensation")
	assert.GreaterOrEqual(t, after.Version, before.Version, "version should be monotonic after compensation")
}

// =============================================================================
// INTEGRATION TESTS — BACKUP QUEUE (SHARD-AWARE)
// =============================================================================

// TestIntegration_Sharded_BackupQueue_ShardIsolation tests that shard-aware backup
// queues correctly route messages to per-shard hashes.
func TestIntegration_Sharded_BackupQueue_ShardIsolation(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	infra := setupShardedInfra(t)

	ctx := context.Background()

	// Add messages to different shard queues
	expectedByKey := make(map[string]string)
	for shardID := 0; shardID < 3; shardID++ {
		key := fmt.Sprintf("transaction:{shard_%d}:org:ledger:tx-%d", shardID, shardID)
		msg := fmt.Sprintf(`{"shard":%d}`, shardID)
		expectedByKey[key] = msg

		err := infra.repo.AddMessageToQueue(ctx, key, []byte(msg))
		require.NoError(t, err, "should add message to shard %d queue", shardID)
	}

	// ReadAllMessagesFromQueue should scan all shard queues
	allMsgs, err := infra.repo.ReadAllMessagesFromQueue(ctx)
	require.NoError(t, err, "should read all shard queues")
	assert.GreaterOrEqual(t, len(allMsgs), len(expectedByKey), "should have all expected messages across shards")

	// Verify each message is in the correct shard queue with expected payload.
	for key, expectedPayload := range expectedByKey {
		value, exists := allMsgs[key]
		assert.True(t, exists, "message for key %s should exist", key)
		assert.JSONEq(t, expectedPayload, value, "message payload should match for %s", key)
	}

	// Cleanup
	for key := range expectedByKey {
		err := infra.repo.RemoveMessageFromQueue(ctx, key)
		require.NoError(t, err, "should remove message %s", key)
	}

	// Verify cleanup
	afterCleanup, err := infra.repo.ReadAllMessagesFromQueue(ctx)
	require.NoError(t, err)

	for key := range expectedByKey {
		_, exists := afterCleanup[key]
		assert.False(t, exists, "message %s should be removed", key)
	}
}

// TestIntegration_Sharded_BackupQueue_LegacyFallback tests that messages with
// non-shard keys fall back to the legacy backup queue.
func TestIntegration_Sharded_BackupQueue_LegacyFallback(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	infra := setupShardedInfra(t)

	ctx := context.Background()

	// Legacy key without {shard_N} tag
	legacyKey := "transaction:{transactions}:org:ledger:legacy-tx"
	msg := []byte(`{"legacy":true}`)

	err := infra.repo.AddMessageToQueue(ctx, legacyKey, msg)
	require.NoError(t, err, "should add legacy message")

	// Should be routed to legacy TransactionBackupQueue
	data, err := infra.repo.ReadMessageFromQueue(ctx, legacyKey)
	require.NoError(t, err, "should read legacy message")
	assert.Equal(t, msg, data, "message content should match")

	// Cleanup
	err = infra.repo.RemoveMessageFromQueue(ctx, legacyKey)
	require.NoError(t, err, "should remove legacy message")
}

// =============================================================================
// INTEGRATION TESTS — BALANCE SYNC SCHEDULE (SHARD-AWARE)
// =============================================================================

// TestIntegration_Sharded_BalanceSyncKeys tests that GetBalanceSyncKeys polls
// all shard schedules when sharding is enabled.
func TestIntegration_Sharded_BalanceSyncKeys(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	infra := setupShardedInfra(t)

	ctx := context.Background()
	orgID := uuid.New()
	ledgerID := uuid.New()

	alias := "@sync-test"
	shardID := infra.router.Resolve(alias)
	targetKey := utils.BalanceShardKey(shardID, orgID, ledgerID, alias+"#default")
	addDueBalanceSyncEntry(t, ctx, infra, shardID, targetKey)

	keys, err := infra.repo.GetBalanceSyncKeys(ctx, 100)
	require.NoError(t, err, "should get balance sync keys")
	require.NotEmpty(t, keys, "at least one due key should be returned")
	assert.Contains(t, keys, targetKey, "scheduled key should be returned as due")
}

func TestIntegration_Sharded_BalanceSyncKeys_RespectsGlobalLimit(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	infra := setupShardedInfra(t)

	ctx := context.Background()
	orgID := uuid.New()
	ledgerID := uuid.New()

	aliasA, aliasB := findCrossShardAliases(infra.router)
	shardA := infra.router.Resolve(aliasA)
	shardB := infra.router.Resolve(aliasB)

	require.NotEqual(t, shardA, shardB, "aliases must target different shards")

	expectedA := utils.BalanceShardKey(shardA, orgID, ledgerID, aliasA+"#default")
	expectedB := utils.BalanceShardKey(shardB, orgID, ledgerID, aliasB+"#default")

	addDueBalanceSyncEntry(t, ctx, infra, shardA, expectedA)
	addDueBalanceSyncEntry(t, ctx, infra, shardB, expectedB)

	keys, err := infra.repo.GetBalanceSyncKeys(ctx, 1)
	require.NoError(t, err)
	require.Len(t, keys, 1, "global limit must cap merged keys across shards")
	assert.Contains(t, []string{expectedA, expectedB}, keys[0], "returned key must come from a scheduled shard balance")
}

// =============================================================================
// INTEGRATION TESTS — PENDING TRANSACTIONS (SHARDED)
// =============================================================================

// TestIntegration_Sharded_PendingTransaction tests ON_HOLD → APPROVED flow
// through the sharded path.
func TestIntegration_Sharded_PendingTransaction(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	infra := setupShardedInfra(t)

	ctx := context.Background()
	orgID := uuid.New()
	ledgerID := uuid.New()
	txID := uuid.New()

	alias := "@pending-shard-test"
	shardID := infra.router.Resolve(alias)

	// Step 1: Hold funds (PENDING + isPending=true + ON_HOLD)
	holdOps := []mmodel.BalanceOperation{
		redistestutil.CreateShardedBalanceOperation(
			orgID, ledgerID, alias, "USD",
			"ON_HOLD", decimal.NewFromInt(200),
			decimal.NewFromInt(1000), "deposit", shardID,
		),
	}

	balances, err := infra.repo.ProcessBalanceAtomicOperation(
		ctx, orgID, ledgerID, txID, "PENDING", true, holdOps,
	)
	require.NoError(t, err, "hold operation should succeed")
	require.NotEmpty(t, balances, "should return balances")

	internalKey := utils.BalanceShardKey(shardID, orgID, ledgerID, alias+"#default")
	holdState := readBalanceRedisByKey(t, ctx, infra, internalKey)
	assert.True(t, holdState.Available.Equal(decimal.NewFromInt(800)), "hold should move amount out of available")
	assert.True(t, holdState.OnHold.Equal(decimal.NewFromInt(200)), "hold should increase on-hold")
	assert.Equal(t, int64(2), holdState.Version, "hold should increment version")

	t.Logf("Hold complete: %d balances updated on shard %d", len(balances), shardID)

	// Step 2: Approve (APPROVED + isPending=true + DEBIT)
	// After hold: available=800, onHold=200. Approve takes from onHold.
	approveOps := []mmodel.BalanceOperation{
		func() mmodel.BalanceOperation {
			op := redistestutil.CreateShardedBalanceOperation(
				orgID, ledgerID, alias, "USD",
				constant.DEBIT, decimal.NewFromInt(200),
				decimal.NewFromInt(800), "deposit", shardID,
			)
			// Set onHold to match the held amount
			op.Balance.OnHold = decimal.NewFromInt(200)
			return op
		}(),
	}

	balances, err = infra.repo.ProcessBalanceAtomicOperation(
		ctx, orgID, ledgerID, txID, "APPROVED", true, approveOps,
	)
	require.NoError(t, err, "approve operation should succeed")

	approvedState := readBalanceRedisByKey(t, ctx, infra, internalKey)
	assert.True(t, approvedState.Available.Equal(decimal.NewFromInt(800)), "approve should keep available after prior hold")
	assert.True(t, approvedState.OnHold.Equal(decimal.Zero), "approve should release held amount")
	assert.Equal(t, int64(3), approvedState.Version, "approve should increment version again")

	t.Log("Pending transaction flow on sharded path: PASSED")
}

func TestIntegration_Sharded_PendingApproved_CrossShardSucceeds(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	infra := setupShardedInfra(t)

	ctx := context.Background()
	orgID := uuid.New()
	ledgerID := uuid.New()
	txID := uuid.New()

	sender, receiver := findCrossShardAliases(infra.router)
	senderShard := infra.router.Resolve(sender)
	receiverShard := infra.router.Resolve(receiver)

	ops := []mmodel.BalanceOperation{
		redistestutil.CreateShardedBalanceOperation(
			orgID, ledgerID, sender, "USD",
			constant.DEBIT, decimal.NewFromInt(100),
			decimal.NewFromInt(900), "deposit", senderShard,
		),
		redistestutil.CreateShardedBalanceOperation(
			orgID, ledgerID, receiver, "USD",
			constant.CREDIT, decimal.NewFromInt(100),
			decimal.NewFromInt(0), "deposit", receiverShard,
		),
	}

	_, err := infra.repo.ProcessBalanceAtomicOperation(
		ctx, orgID, ledgerID, txID, constant.APPROVED, true, ops,
	)
	require.NoError(t, err)

	senderKey := utils.BalanceShardKey(senderShard, orgID, ledgerID, sender+"#default")
	receiverKey := utils.BalanceShardKey(receiverShard, orgID, ledgerID, receiver+"#default")

	senderExists, err := infra.redisContainer.Client.Exists(ctx, senderKey).Result()
	require.NoError(t, err)
	receiverExists, err := infra.redisContainer.Client.Exists(ctx, receiverKey).Result()
	require.NoError(t, err)

	assert.EqualValues(t, 1, senderExists)
	assert.EqualValues(t, 1, receiverExists)

	senderState := readBalanceRedisByKey(t, ctx, infra, senderKey)
	receiverState := readBalanceRedisByKey(t, ctx, infra, receiverKey)
	senderTotal := senderState.Available.Add(senderState.OnHold)
	assert.True(t, senderTotal.Equal(decimal.NewFromInt(800)), "sender total (available+on-hold) should decrease by 100")
	assert.True(t, receiverState.Available.Equal(decimal.NewFromInt(100)), "receiver approved credit should increase available by 100")
	assert.True(t, receiverState.OnHold.Equal(decimal.Zero), "receiver on-hold should remain zero")
	assert.Equal(t, int64(2), senderState.Version, "sender version should increment")
	assert.Equal(t, int64(2), receiverState.Version, "receiver version should increment")
}

// =============================================================================
// INTEGRATION TESTS — EXTERNAL ACCOUNT ROUTING
// =============================================================================

// TestIntegration_Sharded_ExternalAccountDeterministicRouting tests that external
// accounts use deterministic shard routing and still execute correctly.
func TestIntegration_Sharded_ExternalAccountDeterministicRouting(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	infra := setupShardedInfra(t)

	ctx := context.Background()
	orgID := uuid.New()
	ledgerID := uuid.New()
	txID := uuid.New()

	// @alice is the regular account; @external/USD uses deterministic routing.
	regularAlias := "@alice"
	externalAlias := "@external/USD"

	shardMap := infra.router.ResolveTransaction([]string{regularAlias, externalAlias})
	aliceShard := shardMap[regularAlias]
	externalShard := shardMap[externalAlias]

	assert.Equal(t, infra.router.Resolve(externalAlias), externalShard, "external account should use deterministic alias shard")

	ops := []mmodel.BalanceOperation{
		redistestutil.CreateShardedBalanceOperation(
			orgID, ledgerID, regularAlias, "USD",
			constant.DEBIT, decimal.NewFromInt(100),
			decimal.NewFromInt(1000), "deposit", aliceShard,
		),
		func() mmodel.BalanceOperation {
			op := redistestutil.CreateShardedBalanceOperation(
				orgID, ledgerID, externalAlias, "USD",
				constant.CREDIT, decimal.NewFromInt(100),
				decimal.NewFromInt(-5000), "external", externalShard,
			)
			op.Balance.AccountType = "external"
			return op
		}(),
	}

	balances, err := infra.repo.ProcessBalanceAtomicOperation(
		ctx, orgID, ledgerID, txID, "ACTIVE", false, ops,
	)

	require.NoError(t, err, "debit regular + credit external should succeed")
	require.Len(t, balances, 2, "should return 2 balances")

	t.Logf("External routing: alice(shard=%d) external(shard=%d) — deterministic routing confirmed", aliceShard, externalShard)
}

// =============================================================================
// INTEGRATION TESTS — NOTED STATUS (SHARDED PATH)
// =============================================================================

// TestIntegration_Sharded_NotedStatus_SkipsLua verifies NOTED status early-returns
// even when sharding is enabled.
func TestIntegration_Sharded_NotedStatus_SkipsLua(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	infra := setupShardedInfra(t)

	ctx := context.Background()
	orgID := uuid.New()
	ledgerID := uuid.New()
	txID := uuid.New()

	alias := "@noted-shard"
	shardID := infra.router.Resolve(alias)

	ops := []mmodel.BalanceOperation{
		redistestutil.CreateShardedBalanceOperation(
			orgID, ledgerID, alias, "USD",
			constant.DEBIT, decimal.NewFromInt(100),
			decimal.NewFromInt(1000), "deposit", shardID,
		),
	}

	balances, err := infra.repo.ProcessBalanceAtomicOperation(
		ctx, orgID, ledgerID, txID, constant.NOTED, false, ops,
	)

	require.NoError(t, err, "NOTED should always succeed")
	require.Len(t, balances, 1, "should return 1 balance unchanged")
	assert.Equal(t, alias, balances[0].Alias)

	// Balance should be the same pointer (no processing occurred)
	assert.Same(t, ops[0].Balance, balances[0], "should be same pointer (early return)")
}

// =============================================================================
// HELPERS
// =============================================================================

// findSameShardAliases brute-forces two aliases that hash to the same shard.
func findSameShardAliases(r *shard.Router) (string, string) {
	first := "@user_0"
	firstShard := r.Resolve(first)

	for i := 1; i < 10000; i++ {
		candidate := fmt.Sprintf("@user_%d", i)
		if r.Resolve(candidate) == firstShard {
			return first, candidate
		}
	}

	// Fallback: with 8 shards, collision is guaranteed within ~8 attempts
	return "@user_0", "@user_1"
}

// findCrossShardAliases brute-forces two aliases that hash to DIFFERENT shards.
func findCrossShardAliases(r *shard.Router) (string, string) {
	first := "@sender_0"
	firstShard := r.Resolve(first)

	for i := 1; i < 10000; i++ {
		candidate := fmt.Sprintf("@receiver_%d", i)
		if r.Resolve(candidate) != firstShard {
			return first, candidate
		}
	}

	// Should never reach here with 8 shards
	return "@sender_0", "@receiver_0"
}

func readBalanceRedisByKey(t *testing.T, ctx context.Context, infra *shardedTestInfra, key string) mmodel.BalanceRedis {
	t.Helper()

	value, err := infra.redisContainer.Client.Get(ctx, key).Result()
	require.NoError(t, err, "redis key must exist: %s", key)
	require.NotEmpty(t, value, "redis value must not be empty: %s", key)

	var state mmodel.BalanceRedis
	require.NoError(t, json.Unmarshal([]byte(value), &state))

	return state
}

func addDueBalanceSyncEntry(
	t *testing.T,
	ctx context.Context,
	infra *shardedTestInfra,
	shardID int,
	member string,
) {
	t.Helper()

	now, err := infra.redisContainer.Client.Time(ctx).Result()
	require.NoError(t, err, "failed to read redis time")

	scheduleKey := utils.BalanceSyncScheduleShardKey(shardID)
	_, err = infra.redisContainer.Client.ZAdd(ctx, scheduleKey, redis.Z{
		Score:  float64(now.Unix() - 1),
		Member: member,
	}).Result()
	require.NoError(t, err, "failed to add due member to schedule")

	lockKey := utils.BalanceSyncLockShardPrefix(shardID) + member
	err = infra.redisContainer.Client.Del(ctx, lockKey).Err()
	require.NoError(t, err, "failed to clear pre-existing claim lock")
}
