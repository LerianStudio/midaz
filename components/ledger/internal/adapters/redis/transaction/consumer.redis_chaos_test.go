// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

//go:build integration

// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

// Package redis provides chaos tests for the Redis consumer adapter.
//
// Chaos tests exercise fault-tolerance of Redis operations when the underlying
// Valkey/Redis connection experiences failures. They cover both namespacing
// operations and balance atomic operations (double-entry PENDING). They are a
// subset of integration tests and run only when CHAOS=1 is set.
//
// Run with:
//
//	CHAOS=1 go test -tags integration -v -run TestIntegration_Chaos ./components/ledger/internal/adapters/redis/transaction/...
package redis

import (
	"context"
	"encoding/json"
	"os"
	"testing"
	"time"

	libConstants "github.com/LerianStudio/lib-commons/v4/commons/constants"
	libRedis "github.com/LerianStudio/lib-commons/v4/commons/redis"
	tmcore "github.com/LerianStudio/lib-commons/v4/commons/tenant-manager/core"
	"github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	pkgTransaction "github.com/LerianStudio/midaz/v3/pkg/transaction"
	"github.com/LerianStudio/midaz/v3/pkg/utils"
	"github.com/LerianStudio/midaz/v3/tests/utils/chaos"
	redistestutil "github.com/LerianStudio/midaz/v3/tests/utils/redis"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// =============================================================================
// CHAOS TEST INFRASTRUCTURE
// =============================================================================

// chaosNetworkTestInfra holds all resources for a network-chaos test scenario.
// It wraps chaos.Infrastructure (Toxiproxy + Docker network) and a Redis
// consumer repository connected through the Toxiproxy proxy so that faults
// can be injected/removed without restarting any container.
type chaosNetworkTestInfra struct {
	redisContainer *redistestutil.ContainerResult
	chaosInfra     *chaos.Infrastructure
	proxyRepo      *RedisConsumerRepository
	proxyConn      *libRedis.Client
	proxy          *chaos.Proxy
}

// setupRedisChaosNetworkInfra creates the full chaos test infrastructure:
//  1. Creates a Docker network + Toxiproxy container via chaos.Infrastructure
//  2. Starts a Valkey container on the host network
//  3. Registers the Redis container with Infrastructure to get UpstreamAddr
//  4. Creates a Toxiproxy proxy that forwards through to Redis
//  5. Builds a RedisConsumerRepository connected through the proxy address
//
// All cleanup (proxy deletion, container termination, network removal) is
// registered with t.Cleanup() automatically.
func setupRedisChaosNetworkInfra(t *testing.T) *chaosNetworkTestInfra {
	t.Helper()

	// 1. Create chaos infrastructure (Docker network + Toxiproxy)
	chaosInfra := chaos.NewInfrastructure(t)

	// 2. Start Valkey container on the host network
	redisContainer := redistestutil.SetupContainer(t)

	// 3. Register Redis container so Infrastructure can compute UpstreamAddr
	//    (host.docker.internal:<mapped-port>) for the Toxiproxy -> Redis path.
	_, err := chaosInfra.RegisterContainerWithPort("redis", redisContainer.Container, "6379/tcp")
	require.NoError(t, err, "failed to register Redis container with chaos infrastructure")

	// 4. Create a Toxiproxy proxy for Redis using port 8666 (pre-exposed on the
	//    Toxiproxy container by the chaos infrastructure).
	proxy, err := chaosInfra.CreateProxyFor("redis", "8666/tcp")
	require.NoError(t, err, "failed to create Toxiproxy proxy for Redis")

	// 5. Resolve the proxy listen address (host:mapped-port on the test host).
	containerInfo, ok := chaosInfra.GetContainer("redis")
	require.True(t, ok, "Redis container must be registered in chaos infrastructure")
	require.NotEmpty(t, containerInfo.ProxyListen, "proxy listen address must be non-empty")

	proxyAddr := containerInfo.ProxyListen

	// 6. Build a RedisConsumerRepository pointing at the proxy address.
	proxyConn := redistestutil.CreateConnection(t, proxyAddr)

	proxyRepo := &RedisConsumerRepository{
		conn: proxyConn,
	}

	return &chaosNetworkTestInfra{
		redisContainer: redisContainer,
		chaosInfra:     chaosInfra,
		proxyRepo:      proxyRepo,
		proxyConn:      proxyConn,
		proxy:          proxy,
	}
}

// =============================================================================
// CONNECTION LOSS DURING SET
// =============================================================================

// TestIntegration_Chaos_RedisNamespacing_ConnectionLossOnSet verifies that
// Set with a tenant-namespaced key returns a non-nil error (and does not panic)
// when the Redis connection is dropped via Toxiproxy.
//
// 5-Phase structure:
//  1. Normal   — Set succeeds through the proxy
//  2. Inject   — Toxiproxy proxy is disabled (connection loss)
//  3. Verify   — Set returns error; no panic
//  4. Restore  — Toxiproxy proxy is re-enabled
//  5. Recovery — Set succeeds again with the namespaced key
func TestIntegration_Chaos_RedisNamespacing_ConnectionLossOnSet(t *testing.T) {
	if os.Getenv("CHAOS") != "1" {
		t.Skip("Set CHAOS=1 to run chaos tests")
	}

	if testing.Short() {
		t.Skip("Skipping chaos test in short mode")
	}

	infra := setupRedisChaosNetworkInfra(t)

	tenantID := "chaos-tenant-" + uuid.New().String()
	ctx := tmcore.ContextWithTenantID(context.Background(), tenantID)
	key := "chaos-set-key:" + uuid.New().String()
	value := "chaos-set-value-" + uuid.New().String()

	// --- Phase 1: Normal ---
	// Verify Set works through the proxy before any fault is injected.
	t.Log("Phase 1 (Normal): verifying Set succeeds through proxy")

	err := infra.proxyRepo.Set(ctx, key, value, 60)
	require.NoError(t, err, "Phase 1: Set should succeed through proxy before fault injection")

	// --- Phase 2: Inject ---
	// Disable the Toxiproxy proxy to simulate a full connection loss.
	t.Log("Phase 2 (Inject): disabling Toxiproxy proxy to simulate connection loss")

	err = infra.proxy.Disconnect()
	require.NoError(t, err, "Phase 2: Toxiproxy Disconnect should not fail")

	// --- Phase 3: Verify ---
	// Set must return an error. The implementation must not panic.
	t.Log("Phase 3 (Verify): Set must return error, not panic, when connection is lost")

	var setChaosErr error

	require.NotPanics(t, func() {
		setChaosErr = infra.proxyRepo.Set(ctx, key+"_chaos", value, 60)
	}, "Phase 3: Set must not panic on connection loss")

	assert.Error(t, setChaosErr,
		"Phase 3: Set must return an error when Redis connection is dropped")

	t.Logf("Phase 3: received expected error: %v", setChaosErr)

	// --- Phase 4: Restore ---
	// Re-enable the proxy to restore connectivity.
	t.Log("Phase 4 (Restore): re-enabling Toxiproxy proxy")

	err = infra.proxy.Reconnect()
	require.NoError(t, err, "Phase 4: Toxiproxy Reconnect should not fail")

	// --- Phase 5: Recovery ---
	// After the proxy is restored, Set must succeed again.
	t.Log("Phase 5 (Recovery): verifying Set succeeds after proxy restoration")

	recoveryKey := "chaos-set-recovery:" + uuid.New().String()

	chaos.AssertRecoveryWithin(t, func() error {
		return infra.proxyRepo.Set(ctx, recoveryKey, value, 60)
	}, 10*time.Second, "Phase 5: Set should succeed after proxy is restored")

	t.Log("PASS: Set with tenant context returns error on connection loss, recovers correctly")
}

// =============================================================================
// HIGH LATENCY ON GET
// =============================================================================

// TestIntegration_Chaos_RedisNamespacing_HighLatencyOnGet verifies that Get
// with a tenant-namespaced key times out gracefully (returns error, no panic)
// when Toxiproxy injects 5 seconds of downstream latency and the caller uses
// a context with a 1-second deadline.
//
// 5-Phase structure:
//  1. Normal   — Get succeeds with no latency
//  2. Inject   — 5 000 ms latency toxic added to proxy
//  3. Verify   — Get with 1 s deadline returns context.DeadlineExceeded or
//     a connection-timeout error (not panic, not hang)
//  4. Restore  — Latency toxic removed
//  5. Recovery — Get succeeds within the original deadline
func TestIntegration_Chaos_RedisNamespacing_HighLatencyOnGet(t *testing.T) {
	if os.Getenv("CHAOS") != "1" {
		t.Skip("Set CHAOS=1 to run chaos tests")
	}

	if testing.Short() {
		t.Skip("Skipping chaos test in short mode")
	}

	infra := setupRedisChaosNetworkInfra(t)

	tenantID := "chaos-latency-tenant-" + uuid.New().String()
	ctx := tmcore.ContextWithTenantID(context.Background(), tenantID)
	key := "chaos-latency-key:" + uuid.New().String()
	value := "chaos-latency-value"

	// Pre-populate key so Get has something to retrieve.
	err := infra.proxyRepo.Set(ctx, key, value, 300)
	require.NoError(t, err, "pre-populate Set should succeed before latency injection")

	// --- Phase 1: Normal ---
	// Get returns the value quickly (no latency injected yet).
	t.Log("Phase 1 (Normal): verifying Get succeeds with no latency")

	normalCtx, normalCancel := context.WithTimeout(ctx, 3*time.Second)
	defer normalCancel()

	retrieved, err := infra.proxyRepo.Get(normalCtx, key)
	require.NoError(t, err, "Phase 1: Get should succeed before latency injection")
	assert.Equal(t, value, retrieved, "Phase 1: retrieved value must match stored value")

	// --- Phase 2: Inject ---
	// Add 5 000 ms downstream latency — any caller with a shorter deadline will time out.
	t.Log("Phase 2 (Inject): adding 5 000 ms downstream latency via Toxiproxy")

	err = infra.proxy.AddLatency(5000*time.Millisecond, 0)
	require.NoError(t, err, "Phase 2: AddLatency should not fail")

	// --- Phase 3: Verify ---
	// A Get call with a 1-second deadline must return an error before the 5-second
	// latency elapses. The call must neither panic nor block indefinitely.
	t.Log("Phase 3 (Verify): Get must return error within 1 s deadline under 5 s latency")

	highLatencyCtx, highLatencyCancel := context.WithTimeout(ctx, 1*time.Second)
	defer highLatencyCancel()

	var latencyErr error

	done := make(chan struct{})

	go func() {
		defer close(done)
		_, latencyErr = infra.proxyRepo.Get(highLatencyCtx, key)
	}()

	select {
	case <-done:
		// Good — call returned.
	case <-time.After(3 * time.Second):
		t.Fatal("Phase 3: Get hung for more than 3 s — should have returned an error within 1 s deadline")
	}

	assert.Error(t, latencyErr,
		"Phase 3: Get must return an error when context deadline expires before Redis responds")

	t.Logf("Phase 3: received expected error: %v", latencyErr)

	// --- Phase 4: Restore ---
	// Remove all toxics so the proxy forwards traffic without added latency.
	t.Log("Phase 4 (Restore): removing latency toxic")

	err = infra.proxy.RemoveAllToxics()
	require.NoError(t, err, "Phase 4: RemoveAllToxics should not fail")

	// --- Phase 5: Recovery ---
	// Get should now return the correct value within a normal deadline.
	t.Log("Phase 5 (Recovery): verifying Get succeeds after latency toxic removed")

	chaos.AssertRecoveryWithin(t, func() error {
		recoveryCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
		defer cancel()

		val, err := infra.proxyRepo.Get(recoveryCtx, key)
		if err != nil {
			return err
		}

		if val != value {
			return assert.AnError
		}

		return nil
	}, 15*time.Second, "Phase 5: Get should return correct value after latency is removed")

	t.Log("PASS: Get with tenant context times out gracefully under high latency, recovers correctly")
}

// =============================================================================
// CONNECTION LOSS DURING MGET
// =============================================================================

// TestIntegration_Chaos_RedisNamespacing_ConnectionLossOnMGet verifies that
// MGet with tenant-namespaced keys returns a non-nil error (and does not panic)
// when the Redis connection is dropped mid-operation via Toxiproxy.
//
// 5-Phase structure:
//  1. Normal   — MGet returns correct values through the proxy
//  2. Inject   — Toxiproxy proxy disabled (connection loss)
//  3. Verify   — MGet returns error; no panic; returns nil map (not partial data)
//  4. Restore  — Toxiproxy proxy re-enabled
//  5. Recovery — MGet returns correct values again
func TestIntegration_Chaos_RedisNamespacing_ConnectionLossOnMGet(t *testing.T) {
	if os.Getenv("CHAOS") != "1" {
		t.Skip("Set CHAOS=1 to run chaos tests")
	}

	if testing.Short() {
		t.Skip("Skipping chaos test in short mode")
	}

	infra := setupRedisChaosNetworkInfra(t)

	tenantID := "chaos-mget-tenant-" + uuid.New().String()
	ctx := tmcore.ContextWithTenantID(context.Background(), tenantID)

	// Pre-populate several keys.
	keys := []string{
		"chaos-mget-key1:" + uuid.New().String(),
		"chaos-mget-key2:" + uuid.New().String(),
		"chaos-mget-key3:" + uuid.New().String(),
	}
	values := map[string]string{
		keys[0]: "value-mget-1",
		keys[1]: "value-mget-2",
		keys[2]: "value-mget-3",
	}

	for k, v := range values {
		err := infra.proxyRepo.Set(ctx, k, v, 300)
		require.NoError(t, err, "pre-populate Set should succeed for key %s", k)
	}

	// --- Phase 1: Normal ---
	// MGet returns all values correctly through the proxy.
	t.Log("Phase 1 (Normal): verifying MGet returns all values through proxy")

	result, err := infra.proxyRepo.MGet(ctx, keys)
	require.NoError(t, err, "Phase 1: MGet should succeed before fault injection")
	assert.Len(t, result, len(keys), "Phase 1: MGet should return all requested keys")

	for _, k := range keys {
		assert.Equal(t, values[k], result[k], "Phase 1: value for key %s should match", k)
	}

	// --- Phase 2: Inject ---
	// Disable the proxy to simulate a complete connection loss.
	t.Log("Phase 2 (Inject): disabling Toxiproxy proxy to simulate connection loss")

	err = infra.proxy.Disconnect()
	require.NoError(t, err, "Phase 2: Toxiproxy Disconnect should not fail")

	// --- Phase 3: Verify ---
	// MGet must return an error. It must not panic and must not return partial data
	// as if the operation succeeded.
	t.Log("Phase 3 (Verify): MGet must return error, not panic, when connection is lost")

	var mgetChaosErr error

	var mgetChaosResult map[string]string

	require.NotPanics(t, func() {
		mgetChaosResult, mgetChaosErr = infra.proxyRepo.MGet(ctx, keys)
	}, "Phase 3: MGet must not panic on connection loss")

	assert.Error(t, mgetChaosErr,
		"Phase 3: MGet must return an error when Redis connection is dropped")
	assert.Nil(t, mgetChaosResult,
		"Phase 3: MGet must return nil map when the operation fails — tenant prefix must not mask the failure")

	t.Logf("Phase 3: received expected error: %v", mgetChaosErr)

	// --- Phase 4: Restore ---
	// Re-enable the proxy.
	t.Log("Phase 4 (Restore): re-enabling Toxiproxy proxy")

	err = infra.proxy.Reconnect()
	require.NoError(t, err, "Phase 4: Toxiproxy Reconnect should not fail")

	// --- Phase 5: Recovery ---
	// MGet must return all values again after recovery.
	t.Log("Phase 5 (Recovery): verifying MGet returns all values after proxy restoration")

	chaos.AssertRecoveryWithin(t, func() error {
		res, err := infra.proxyRepo.MGet(ctx, keys)
		if err != nil {
			return err
		}

		if len(res) != len(keys) {
			return assert.AnError
		}

		return nil
	}, 10*time.Second, "Phase 5: MGet should succeed after proxy is restored")

	t.Log("PASS: MGet with tenant context returns error on connection loss, recovers correctly")
}

// =============================================================================
// CONNECTION LOSS DURING QUEUE OPS (AddMessageToQueue)
// =============================================================================

// TestIntegration_Chaos_RedisNamespacing_ConnectionLossOnQueueOp verifies that
// AddMessageToQueue returns a non-nil error (and does not panic) when Redis is
// unavailable. The tenant namespace applied to the queue key must not silently
// absorb the failure.
//
// 5-Phase structure:
//  1. Normal   — AddMessageToQueue succeeds through the proxy
//  2. Inject   — Toxiproxy proxy disabled (connection loss)
//  3. Verify   — AddMessageToQueue returns error; no panic; error propagates
//  4. Restore  — Toxiproxy proxy re-enabled
//  5. Recovery — AddMessageToQueue succeeds again
func TestIntegration_Chaos_RedisNamespacing_ConnectionLossOnQueueOp(t *testing.T) {
	if os.Getenv("CHAOS") != "1" {
		t.Skip("Set CHAOS=1 to run chaos tests")
	}

	if testing.Short() {
		t.Skip("Skipping chaos test in short mode")
	}

	infra := setupRedisChaosNetworkInfra(t)

	tenantID := "chaos-queue-tenant-" + uuid.New().String()
	ctx := tmcore.ContextWithTenantID(context.Background(), tenantID)

	msgKey := "chaos-queue-msg:" + uuid.New().String()
	msgPayload := []byte(`{"chaos":"test","ts":"` + time.Now().Format(time.RFC3339) + `"}`)

	// --- Phase 1: Normal ---
	// AddMessageToQueue should succeed through the proxy before any fault.
	t.Log("Phase 1 (Normal): verifying AddMessageToQueue succeeds through proxy")

	err := infra.proxyRepo.AddMessageToQueue(ctx, msgKey, msgPayload)
	require.NoError(t, err, "Phase 1: AddMessageToQueue should succeed before fault injection")

	// --- Phase 2: Inject ---
	// Disable the proxy to simulate Redis being unreachable.
	t.Log("Phase 2 (Inject): disabling Toxiproxy proxy to simulate connection loss")

	err = infra.proxy.Disconnect()
	require.NoError(t, err, "Phase 2: Toxiproxy Disconnect should not fail")

	// --- Phase 3: Verify ---
	// The queue write must return an error. The tenant prefix must not mask the failure.
	t.Log("Phase 3 (Verify): AddMessageToQueue must return error, not panic, when connection is lost")

	chaosKey := "chaos-queue-msg-fault:" + uuid.New().String()

	var queueChaosErr error

	require.NotPanics(t, func() {
		queueChaosErr = infra.proxyRepo.AddMessageToQueue(ctx, chaosKey, msgPayload)
	}, "Phase 3: AddMessageToQueue must not panic on connection loss")

	assert.Error(t, queueChaosErr,
		"Phase 3: AddMessageToQueue must return an error when Redis is unavailable — "+
			"tenant prefix must not mask the failure")

	t.Logf("Phase 3: received expected error: %v", queueChaosErr)

	// --- Phase 4: Restore ---
	// Re-enable the proxy.
	t.Log("Phase 4 (Restore): re-enabling Toxiproxy proxy")

	err = infra.proxy.Reconnect()
	require.NoError(t, err, "Phase 4: Toxiproxy Reconnect should not fail")

	// --- Phase 5: Recovery ---
	// AddMessageToQueue must succeed again after recovery.
	t.Log("Phase 5 (Recovery): verifying AddMessageToQueue succeeds after proxy restoration")

	recoveryKey := "chaos-queue-msg-recovery:" + uuid.New().String()

	chaos.AssertRecoveryWithin(t, func() error {
		return infra.proxyRepo.AddMessageToQueue(ctx, recoveryKey, msgPayload)
	}, 10*time.Second, "Phase 5: AddMessageToQueue should succeed after proxy is restored")

	t.Log("PASS: AddMessageToQueue returns error on connection loss, tenant prefix does not mask failure")
}

// =============================================================================
// RECOVERY AFTER RECONNECT
// =============================================================================

// TestIntegration_Chaos_RedisNamespacing_RecoveryAfterReconnect verifies the
// full fault-and-recovery cycle for both Set and Get: after Redis recovers from
// an outage the namespaced operations continue to work correctly, producing the
// same key/value semantics as before the fault.
//
// 5-Phase structure:
//  1. Normal   — Set then Get succeed; namespaced value is confirmed
//  2. Inject   — Toxiproxy proxy disabled (Redis unreachable)
//  3. Verify   — Both Set and Get return errors; no panics
//  4. Restore  — Toxiproxy proxy re-enabled
//  5. Recovery — New Set then Get succeed; value is stored under the correct
//     tenant namespace (not the bare key)
func TestIntegration_Chaos_RedisNamespacing_RecoveryAfterReconnect(t *testing.T) {
	if os.Getenv("CHAOS") != "1" {
		t.Skip("Set CHAOS=1 to run chaos tests")
	}

	if testing.Short() {
		t.Skip("Skipping chaos test in short mode")
	}

	infra := setupRedisChaosNetworkInfra(t)

	tenantID := "chaos-recovery-tenant-" + uuid.New().String()
	ctx := tmcore.ContextWithTenantID(context.Background(), tenantID)
	key := "chaos-recovery-key:" + uuid.New().String()
	value := "chaos-recovery-value-" + uuid.New().String()

	expectedNamespacedKey := "tenant:" + tenantID + ":" + key

	// --- Phase 1: Normal ---
	// Set and Get work; the raw Redis client confirms the key is namespaced.
	t.Log("Phase 1 (Normal): verifying Set/Get work and key is namespaced in Redis")

	err := infra.proxyRepo.Set(ctx, key, value, 300)
	require.NoError(t, err, "Phase 1: Set should succeed through proxy")

	retrieved, err := infra.proxyRepo.Get(ctx, key)
	require.NoError(t, err, "Phase 1: Get should succeed through proxy")
	assert.Equal(t, value, retrieved, "Phase 1: retrieved value must equal stored value")

	// Use the raw client (direct connection, not proxied) to confirm the key is stored
	// under the tenant namespace.
	rawStored, rawErr := infra.redisContainer.Client.Get(context.Background(), expectedNamespacedKey).Result()
	require.NoError(t, rawErr, "Phase 1: namespaced key should exist in raw Redis")
	assert.Equal(t, value, rawStored, "Phase 1: value stored under namespaced key must match")

	// Confirm the bare (un-namespaced) key does NOT exist.
	bareVal, bareErr := infra.redisContainer.Client.Get(context.Background(), key).Result()
	assert.Error(t, bareErr, "Phase 1: bare key must NOT be set in Redis")
	assert.Empty(t, bareVal, "Phase 1: bare key must have no value")

	// --- Phase 2: Inject ---
	// Drop the connection via Toxiproxy.
	t.Log("Phase 2 (Inject): disabling Toxiproxy proxy to simulate Redis outage")

	err = infra.proxy.Disconnect()
	require.NoError(t, err, "Phase 2: Toxiproxy Disconnect should not fail")

	// --- Phase 3: Verify ---
	// Both Set and Get must return errors without panicking.
	t.Log("Phase 3 (Verify): Set and Get must return errors, not panic, during outage")

	outageKey := "chaos-recovery-outage:" + uuid.New().String()

	var setOutageErr, getOutageErr error

	require.NotPanics(t, func() {
		setOutageErr = infra.proxyRepo.Set(ctx, outageKey, value, 60)
	}, "Phase 3: Set must not panic during outage")

	require.NotPanics(t, func() {
		_, getOutageErr = infra.proxyRepo.Get(ctx, key)
	}, "Phase 3: Get must not panic during outage")

	assert.Error(t, setOutageErr,
		"Phase 3: Set must return an error when Redis is unavailable")
	assert.Error(t, getOutageErr,
		"Phase 3: Get must return an error when Redis is unavailable")

	t.Logf("Phase 3: Set error: %v", setOutageErr)
	t.Logf("Phase 3: Get error: %v", getOutageErr)

	// --- Phase 4: Restore ---
	// Re-enable the proxy.
	t.Log("Phase 4 (Restore): re-enabling Toxiproxy proxy")

	err = infra.proxy.Reconnect()
	require.NoError(t, err, "Phase 4: Toxiproxy Reconnect should not fail")

	// --- Phase 5: Recovery ---
	// A new Set/Get cycle with a different key must succeed, and the stored key
	// must again carry the tenant namespace — confirming the namespacing logic
	// was not corrupted by the fault/recovery cycle.
	t.Log("Phase 5 (Recovery): verifying Set/Get succeed with correct namespacing after recovery")

	recoveryKey := "chaos-recovery-post:" + uuid.New().String()
	recoveryValue := "chaos-recovery-post-value-" + uuid.New().String()
	expectedRecoveryNamespacedKey := "tenant:" + tenantID + ":" + recoveryKey

	chaos.AssertRecoveryWithin(t, func() error {
		return infra.proxyRepo.Set(ctx, recoveryKey, recoveryValue, 300)
	}, 10*time.Second, "Phase 5: Set should succeed after proxy is restored")

	// Get via repository — must return correct value.
	var recoveredVal string

	chaos.AssertRecoveryWithin(t, func() error {
		var err error
		recoveredVal, err = infra.proxyRepo.Get(ctx, recoveryKey)

		return err
	}, 5*time.Second, "Phase 5: Get should succeed after proxy is restored")

	assert.Equal(t, recoveryValue, recoveredVal,
		"Phase 5: recovered value must equal the value written after reconnect")

	// Inspect raw Redis to confirm the namespace is still applied correctly.
	rawRecovery, rawRecoveryErr := infra.redisContainer.Client.Get(
		context.Background(), expectedRecoveryNamespacedKey,
	).Result()
	require.NoError(t, rawRecoveryErr,
		"Phase 5: namespaced key should exist in raw Redis after recovery")
	assert.Equal(t, recoveryValue, rawRecovery,
		"Phase 5: value stored under namespaced key after recovery must match")

	t.Log("PASS: namespaced Set/Get work correctly after Redis recovers from outage")
}

// =============================================================================
// BALANCE ATOMIC OPERATION HELPERS
// =============================================================================

// buildApprovedBalanceOps creates a []mmodel.BalanceOperation slice for an
// APPROVED transaction. For the source side, the operation is DEBIT (OnHold--).
// For the destination side, the operation is CREDIT (Available++).
// When used with a PENDING-preconditioned balance, this simulates the
// "approve after pending" workflow.
func buildApprovedBalanceOps(
	orgID, ledgerID uuid.UUID,
	alias string,
	available, onHold decimal.Decimal,
	version int64,
	isFrom bool,
) []mmodel.BalanceOperation {
	balanceID := uuid.New().String()
	accountID := uuid.New().String()
	balanceKey := constant.DefaultBalanceKey

	balance := &mmodel.Balance{
		ID:             balanceID,
		AccountID:      accountID,
		Alias:          alias,
		Key:            balanceKey,
		AssetCode:      "BRL",
		Available:      available,
		OnHold:         onHold,
		Version:        version,
		AccountType:    "deposit",
		AllowSending:   true,
		AllowReceiving: true,
		OrganizationID: orgID.String(),
		LedgerID:       ledgerID.String(),
	}

	var operation string

	var direction string

	if isFrom {
		operation = libConstants.DEBIT
		direction = libConstants.DEBIT
	} else {
		operation = libConstants.CREDIT
		direction = libConstants.CREDIT
	}

	amount := pkgTransaction.Amount{
		Asset:                  "BRL",
		Value:                  decimal.NewFromInt(100),
		Operation:              operation,
		TransactionType:        constant.APPROVED,
		Direction:              direction,
		RouteValidationEnabled: false, // APPROVED does not use route flag for per-field atomicity
	}

	internalKey := utils.BalanceInternalKey(orgID, ledgerID, pkgTransaction.AliasKey(alias, balanceKey))

	return []mmodel.BalanceOperation{
		{
			Balance:     balance,
			Alias:       alias,
			Amount:      amount,
			InternalKey: internalKey,
		},
	}
}

// buildCanceledBalanceOps creates a []mmodel.BalanceOperation slice for a
// CANCELED transaction with routeValidationEnabled=true. For the source side:
//   - RELEASE operation: OnHold-- only (Available unchanged)
//   - CREDIT operation: Available++ only (OnHold unchanged)
//
// Each operation produces version+1, so together the source gets version+2.
func buildCanceledBalanceOps(
	orgID, ledgerID uuid.UUID,
	alias string,
	available, onHold decimal.Decimal,
	version int64,
	operation string,
	routeEnabled bool,
) []mmodel.BalanceOperation {
	balanceID := uuid.New().String()
	accountID := uuid.New().String()
	balanceKey := constant.DefaultBalanceKey

	balance := &mmodel.Balance{
		ID:             balanceID,
		AccountID:      accountID,
		Alias:          alias,
		Key:            balanceKey,
		AssetCode:      "BRL",
		Available:      available,
		OnHold:         onHold,
		Version:        version,
		AccountType:    "deposit",
		AllowSending:   true,
		AllowReceiving: true,
		OrganizationID: orgID.String(),
		LedgerID:       ledgerID.String(),
	}

	var direction string

	if operation == libConstants.RELEASE {
		direction = libConstants.DEBIT
	} else {
		direction = libConstants.CREDIT
	}

	amount := pkgTransaction.Amount{
		Asset:                  "BRL",
		Value:                  decimal.NewFromInt(100),
		Operation:              operation,
		TransactionType:        constant.CANCELED,
		Direction:              direction,
		RouteValidationEnabled: routeEnabled,
	}

	internalKey := utils.BalanceInternalKey(orgID, ledgerID, pkgTransaction.AliasKey(alias, balanceKey))

	return []mmodel.BalanceOperation{
		{
			Balance:     balance,
			Alias:       alias,
			Amount:      amount,
			InternalKey: internalKey,
		},
	}
}

// buildTestBalanceOps creates a []mmodel.BalanceOperation slice for a PENDING
// transaction with a single ONHOLD source entry. When routeEnabled is true, the
// Lua script increments version by 2 (double-entry behavior).
func buildTestBalanceOps(
	orgID, ledgerID uuid.UUID,
	alias string,
	available, onHold decimal.Decimal,
	version int64,
	routeEnabled bool,
) []mmodel.BalanceOperation {
	balanceID := uuid.New().String()
	accountID := uuid.New().String()
	balanceKey := constant.DefaultBalanceKey

	balance := &mmodel.Balance{
		ID:             balanceID,
		AccountID:      accountID,
		Alias:          alias,
		Key:            balanceKey,
		AssetCode:      "BRL",
		Available:      available,
		OnHold:         onHold,
		Version:        version,
		AccountType:    "deposit",
		AllowSending:   true,
		AllowReceiving: true,
		OrganizationID: orgID.String(),
		LedgerID:       ledgerID.String(),
	}

	amount := pkgTransaction.Amount{
		Asset:                  "BRL",
		Value:                  decimal.NewFromInt(100),
		Operation:              libConstants.ONHOLD,
		TransactionType:        constant.PENDING,
		Direction:              libConstants.CREDIT,
		RouteValidationEnabled: routeEnabled,
	}

	internalKey := utils.BalanceInternalKey(orgID, ledgerID, pkgTransaction.AliasKey(alias, balanceKey))

	return []mmodel.BalanceOperation{
		{
			Balance:     balance,
			Alias:       alias,
			Amount:      amount,
			InternalKey: internalKey,
		},
	}
}

// verifyRedisBalance reads a balance directly from Redis (bypassing the proxy)
// and asserts its fields match expected values.
func verifyRedisBalance(
	t *testing.T,
	infra *chaosNetworkTestInfra,
	balanceKey string,
	expectedAvailable decimal.Decimal,
	expectedOnHold decimal.Decimal,
	expectedVersion int64,
) {
	t.Helper()

	raw, err := infra.redisContainer.Client.Get(context.Background(), balanceKey).Result()
	require.NoError(t, err, "direct Redis GET should succeed for key %s", balanceKey)

	var balanceRedis mmodel.BalanceRedis

	err = json.Unmarshal([]byte(raw), &balanceRedis)
	require.NoError(t, err, "balance JSON should unmarshal correctly")

	assert.True(t, expectedAvailable.Equal(balanceRedis.Available),
		"available: expected %s, got %s", expectedAvailable.String(), balanceRedis.Available.String())
	assert.True(t, expectedOnHold.Equal(balanceRedis.OnHold),
		"onHold: expected %s, got %s", expectedOnHold.String(), balanceRedis.OnHold.String())
	assert.Equal(t, expectedVersion, balanceRedis.Version,
		"version: expected %d, got %d", expectedVersion, balanceRedis.Version)
}

// =============================================================================
// CONNECTION LOSS DURING LUA SCRIPT (DOUBLE-ENTRY PENDING)
// =============================================================================

// TestIntegration_Chaos_BalanceAtomic_ConnectionLossOnDoubleEntry verifies that
// ProcessBalanceAtomicOperation with routeValidationEnabled=true returns a
// non-nil error (and does not panic) when the Redis connection is dropped via
// Toxiproxy during the Lua script execution.
//
// 5-Phase structure:
//  1. Normal   -- ProcessBalanceAtomicOperation succeeds, version increments by 2
//  2. Inject   -- Toxiproxy proxy disabled (connection loss)
//  3. Verify   -- ProcessBalanceAtomicOperation returns error; no panic; no partial state
//  4. Restore  -- Toxiproxy proxy re-enabled
//  5. Recovery -- ProcessBalanceAtomicOperation succeeds again with correct version
func TestIntegration_Chaos_BalanceAtomic_ConnectionLossOnDoubleEntry(t *testing.T) {
	if os.Getenv("CHAOS") != "1" {
		t.Skip("Set CHAOS=1 to run chaos tests")
	}

	if testing.Short() {
		t.Skip("Skipping chaos test in short mode")
	}

	infra := setupRedisChaosNetworkInfra(t)

	orgID := uuid.New()
	ledgerID := uuid.New()
	txID := uuid.New()
	alias := "@source-" + uuid.New().String()[:8]
	initialAvailable := decimal.NewFromInt(1000)
	initialOnHold := decimal.NewFromInt(0)
	initialVersion := int64(1)

	balanceOps := buildTestBalanceOps(orgID, ledgerID, alias, initialAvailable, initialOnHold, initialVersion, true)

	ctx := context.Background()

	// --- Phase 1: Normal ---
	// Verify ProcessBalanceAtomicOperation works through the proxy with route
	// validation enabled. The Lua script should increment version by 2.
	t.Log("Phase 1 (Normal): verifying ProcessBalanceAtomicOperation succeeds with routeValidationEnabled=true")

	result, err := infra.proxyRepo.ProcessBalanceAtomicOperation(
		ctx, orgID, ledgerID, txID, constant.PENDING, true, balanceOps,
	)
	require.NoError(t, err, "Phase 1: ProcessBalanceAtomicOperation should succeed through proxy")
	require.NotNil(t, result, "Phase 1: result must not be nil")
	require.Len(t, result.After, 1, "Phase 1: should have 1 after-balance entry")

	// Verify version incremented by 2 (double-entry behavior)
	afterBalance := result.After[0]
	expectedVersionAfterPhase1 := initialVersion + 2
	assert.Equal(t, expectedVersionAfterPhase1, afterBalance.Version,
		"Phase 1: version should increment by 2 for ONHOLD+PENDING+routeEnabled")

	// Verify balance changes: Available decreased by 100, OnHold increased by 100
	expectedAvailableAfterPhase1 := initialAvailable.Sub(decimal.NewFromInt(100))
	expectedOnHoldAfterPhase1 := initialOnHold.Add(decimal.NewFromInt(100))
	assert.True(t, expectedAvailableAfterPhase1.Equal(afterBalance.Available),
		"Phase 1: available should be %s, got %s", expectedAvailableAfterPhase1.String(), afterBalance.Available.String())
	assert.True(t, expectedOnHoldAfterPhase1.Equal(afterBalance.OnHold),
		"Phase 1: onHold should be %s, got %s", expectedOnHoldAfterPhase1.String(), afterBalance.OnHold.String())

	t.Logf("Phase 1: version=%d, available=%s, onHold=%s",
		afterBalance.Version, afterBalance.Available.String(), afterBalance.OnHold.String())

	// --- Phase 2: Inject ---
	// Disable the Toxiproxy proxy to simulate a full connection loss.
	t.Log("Phase 2 (Inject): disabling Toxiproxy proxy to simulate connection loss")

	err = infra.proxy.Disconnect()
	require.NoError(t, err, "Phase 2: Toxiproxy Disconnect should not fail")

	// --- Phase 3: Verify ---
	// ProcessBalanceAtomicOperation must return an error. It must not panic.
	t.Log("Phase 3 (Verify): ProcessBalanceAtomicOperation must return error, not panic, when connection is lost")

	chaosTxID := uuid.New()
	chaosBalanceOps := buildTestBalanceOps(orgID, ledgerID, alias, expectedAvailableAfterPhase1, expectedOnHoldAfterPhase1, expectedVersionAfterPhase1, true)

	var chaosErr error
	var chaosResult *mmodel.BalanceAtomicResult

	require.NotPanics(t, func() {
		chaosResult, chaosErr = infra.proxyRepo.ProcessBalanceAtomicOperation(
			ctx, orgID, ledgerID, chaosTxID, constant.PENDING, true, chaosBalanceOps,
		)
	}, "Phase 3: ProcessBalanceAtomicOperation must not panic on connection loss")

	assert.Error(t, chaosErr,
		"Phase 3: ProcessBalanceAtomicOperation must return an error when Redis connection is dropped")
	assert.Nil(t, chaosResult,
		"Phase 3: result must be nil when the operation fails")

	t.Logf("Phase 3: received expected error: %v", chaosErr)

	// Verify balance in Redis is unchanged from Phase 1 (via direct client, bypassing proxy)
	balanceKey := pkgTransaction.AliasKey(alias, constant.DefaultBalanceKey)
	internalKey := utils.BalanceInternalKey(orgID, ledgerID, balanceKey)

	verifyRedisBalance(t, infra, internalKey, expectedAvailableAfterPhase1, expectedOnHoldAfterPhase1, expectedVersionAfterPhase1)
	t.Log("Phase 3: confirmed balance unchanged in Redis after connection loss")

	// --- Phase 4: Restore ---
	// Re-enable the proxy to restore connectivity.
	t.Log("Phase 4 (Restore): re-enabling Toxiproxy proxy")

	err = infra.proxy.Reconnect()
	require.NoError(t, err, "Phase 4: Toxiproxy Reconnect should not fail")

	// --- Phase 5: Recovery ---
	// After the proxy is restored, ProcessBalanceAtomicOperation must succeed
	// and produce correct version increment.
	t.Log("Phase 5 (Recovery): verifying ProcessBalanceAtomicOperation succeeds after proxy restoration")

	recoveryTxID := uuid.New()

	// Build ops with the current state from Phase 1 (since Phase 3 failed, state is unchanged)
	recoveryOps := buildTestBalanceOps(orgID, ledgerID, alias, expectedAvailableAfterPhase1, expectedOnHoldAfterPhase1, expectedVersionAfterPhase1, true)

	var recoveryResult *mmodel.BalanceAtomicResult

	chaos.AssertRecoveryWithin(t, func() error {
		var err error
		recoveryResult, err = infra.proxyRepo.ProcessBalanceAtomicOperation(
			ctx, orgID, ledgerID, recoveryTxID, constant.PENDING, true, recoveryOps,
		)

		return err
	}, 10*time.Second, "Phase 5: ProcessBalanceAtomicOperation should succeed after proxy is restored")

	require.NotNil(t, recoveryResult, "Phase 5: recovery result must not be nil")
	require.Len(t, recoveryResult.After, 1, "Phase 5: should have 1 after-balance entry")

	recoveryAfter := recoveryResult.After[0]
	expectedVersionAfterRecovery := expectedVersionAfterPhase1 + 2
	assert.Equal(t, expectedVersionAfterRecovery, recoveryAfter.Version,
		"Phase 5: version should be %d after recovery (incremented by 2)", expectedVersionAfterRecovery)

	t.Logf("Phase 5: recovery version=%d, available=%s, onHold=%s",
		recoveryAfter.Version, recoveryAfter.Available.String(), recoveryAfter.OnHold.String())

	t.Log("PASS: ProcessBalanceAtomicOperation returns error on connection loss, balance state preserved, recovers correctly with version+2")
}

// =============================================================================
// HIGH LATENCY DURING LUA SCRIPT (DOUBLE-ENTRY PENDING)
// =============================================================================

// TestIntegration_Chaos_BalanceAtomic_HighLatencyOnDoubleEntry verifies that
// ProcessBalanceAtomicOperation with routeValidationEnabled=true times out
// gracefully when Toxiproxy injects high latency and the caller uses a context
// with a short deadline. The Lua script is atomic, so a timeout before completion
// should leave Redis state unchanged.
//
// 5-Phase structure:
//  1. Normal   -- Operation succeeds with no latency, version increments by 2
//  2. Inject   -- 5000 ms latency toxic added to proxy
//  3. Verify   -- Operation with 1s deadline returns timeout error; balance unchanged
//  4. Restore  -- Latency toxic removed
//  5. Recovery -- Operation succeeds within normal deadline
func TestIntegration_Chaos_BalanceAtomic_HighLatencyOnDoubleEntry(t *testing.T) {
	if os.Getenv("CHAOS") != "1" {
		t.Skip("Set CHAOS=1 to run chaos tests")
	}

	if testing.Short() {
		t.Skip("Skipping chaos test in short mode")
	}

	infra := setupRedisChaosNetworkInfra(t)

	orgID := uuid.New()
	ledgerID := uuid.New()
	txID := uuid.New()
	alias := "@latency-src-" + uuid.New().String()[:8]
	initialAvailable := decimal.NewFromInt(5000)
	initialOnHold := decimal.NewFromInt(0)
	initialVersion := int64(1)

	balanceOps := buildTestBalanceOps(orgID, ledgerID, alias, initialAvailable, initialOnHold, initialVersion, true)

	ctx := context.Background()

	// --- Phase 1: Normal ---
	// Execute the operation successfully with no latency injected.
	t.Log("Phase 1 (Normal): verifying operation succeeds with no latency")

	result, err := infra.proxyRepo.ProcessBalanceAtomicOperation(
		ctx, orgID, ledgerID, txID, constant.PENDING, true, balanceOps,
	)
	require.NoError(t, err, "Phase 1: operation should succeed before latency injection")
	require.NotNil(t, result, "Phase 1: result must not be nil")
	require.Len(t, result.After, 1, "Phase 1: should have 1 after-balance entry")

	afterPhase1 := result.After[0]
	expectedVersionPhase1 := initialVersion + 2
	expectedAvailablePhase1 := initialAvailable.Sub(decimal.NewFromInt(100))
	expectedOnHoldPhase1 := initialOnHold.Add(decimal.NewFromInt(100))

	assert.Equal(t, expectedVersionPhase1, afterPhase1.Version,
		"Phase 1: version should be %d", expectedVersionPhase1)

	t.Logf("Phase 1: version=%d, available=%s, onHold=%s",
		afterPhase1.Version, afterPhase1.Available.String(), afterPhase1.OnHold.String())

	// --- Phase 2: Inject ---
	// Add 5000 ms downstream latency -- any caller with a shorter deadline will time out.
	t.Log("Phase 2 (Inject): adding 5000 ms downstream latency via Toxiproxy")

	err = infra.proxy.AddLatency(5000*time.Millisecond, 0)
	require.NoError(t, err, "Phase 2: AddLatency should not fail")

	// --- Phase 3: Verify ---
	// Call with a 1-second deadline. Must return an error before the 5-second latency
	// elapses. Must neither panic nor block indefinitely.
	t.Log("Phase 3 (Verify): operation must return error within 1s deadline under 5s latency")

	latencyTxID := uuid.New()
	latencyOps := buildTestBalanceOps(orgID, ledgerID, alias, expectedAvailablePhase1, expectedOnHoldPhase1, expectedVersionPhase1, true)

	highLatencyCtx, highLatencyCancel := context.WithTimeout(ctx, 1*time.Second)
	defer highLatencyCancel()

	var latencyErr error
	var latencyResult *mmodel.BalanceAtomicResult

	done := make(chan struct{})

	go func() {
		defer close(done)
		latencyResult, latencyErr = infra.proxyRepo.ProcessBalanceAtomicOperation(
			highLatencyCtx, orgID, ledgerID, latencyTxID, constant.PENDING, true, latencyOps,
		)
	}()

	select {
	case <-done:
		// Call returned within acceptable time.
	case <-time.After(3 * time.Second):
		t.Fatal("Phase 3: operation hung for more than 3s -- should have returned an error within 1s deadline")
	}

	assert.Error(t, latencyErr,
		"Phase 3: operation must return an error when context deadline expires before Redis responds")
	assert.Nil(t, latencyResult,
		"Phase 3: result must be nil when the operation times out")

	t.Logf("Phase 3: received expected error: %v", latencyErr)

	// Verify balance in Redis is unchanged from Phase 1.
	// Since the Lua script is atomic, if it didn't complete before the context was
	// cancelled, the balance should remain at Phase 1 state. If the script DID
	// complete (Redis processed it before timeout), the balance may have advanced --
	// either outcome is acceptable as long as the Go code returned an error.
	balanceKey := pkgTransaction.AliasKey(alias, constant.DefaultBalanceKey)
	internalKey := utils.BalanceInternalKey(orgID, ledgerID, balanceKey)

	raw, rawErr := infra.redisContainer.Client.Get(context.Background(), internalKey).Result()
	require.NoError(t, rawErr, "Phase 3: direct Redis GET should succeed")

	var storedBalance mmodel.BalanceRedis

	err = json.Unmarshal([]byte(raw), &storedBalance)
	require.NoError(t, err, "Phase 3: balance JSON should unmarshal")

	// The balance version must be either Phase 1 value (script didn't execute) or
	// Phase 1 + 2 (script completed but Go timed out reading the response).
	// No intermediate version should exist (no version gaps).
	validVersions := []int64{expectedVersionPhase1, expectedVersionPhase1 + 2}
	assert.Contains(t, validVersions, storedBalance.Version,
		"Phase 3: version must be %d (unchanged) or %d (script completed), got %d",
		expectedVersionPhase1, expectedVersionPhase1+2, storedBalance.Version)

	t.Logf("Phase 3: Redis balance version=%d (unchanged=%v)", storedBalance.Version, storedBalance.Version == expectedVersionPhase1)

	// --- Phase 4: Restore ---
	// Remove all toxics so the proxy forwards traffic without added latency.
	t.Log("Phase 4 (Restore): removing latency toxic")

	err = infra.proxy.RemoveAllToxics()
	require.NoError(t, err, "Phase 4: RemoveAllToxics should not fail")

	// --- Phase 5: Recovery ---
	// After latency is removed, operation should succeed within a normal deadline.
	t.Log("Phase 5 (Recovery): verifying operation succeeds after latency toxic removed")

	recoveryTxID := uuid.New()

	// Read current state from Redis directly to build correct recovery ops,
	// since Phase 3 may or may not have mutated the balance.
	currentRaw, currentErr := infra.redisContainer.Client.Get(context.Background(), internalKey).Result()
	require.NoError(t, currentErr, "Phase 5: should read current balance from Redis")

	var currentBalance mmodel.BalanceRedis

	err = json.Unmarshal([]byte(currentRaw), &currentBalance)
	require.NoError(t, err, "Phase 5: should unmarshal current balance")

	recoveryOps := buildTestBalanceOps(orgID, ledgerID, alias,
		currentBalance.Available, currentBalance.OnHold, currentBalance.Version, true)

	var recoveryResult *mmodel.BalanceAtomicResult

	chaos.AssertRecoveryWithin(t, func() error {
		var err error
		recoveryResult, err = infra.proxyRepo.ProcessBalanceAtomicOperation(
			context.Background(), orgID, ledgerID, recoveryTxID, constant.PENDING, true, recoveryOps,
		)

		return err
	}, 15*time.Second, "Phase 5: operation should succeed after latency is removed")

	require.NotNil(t, recoveryResult, "Phase 5: recovery result must not be nil")
	require.Len(t, recoveryResult.After, 1, "Phase 5: should have 1 after-balance entry")

	recoveryAfter := recoveryResult.After[0]
	expectedRecoveryVersion := currentBalance.Version + 2
	assert.Equal(t, expectedRecoveryVersion, recoveryAfter.Version,
		"Phase 5: version should be %d after recovery", expectedRecoveryVersion)

	t.Logf("Phase 5: recovery version=%d, available=%s, onHold=%s",
		recoveryAfter.Version, recoveryAfter.Available.String(), recoveryAfter.OnHold.String())

	t.Log("PASS: operation times out gracefully under high latency, balance state consistent, recovers with version+2")
}

// =============================================================================
// CONNECTION RESET DURING BALANCE OPERATION (DOUBLE-ENTRY PENDING)
// =============================================================================

// TestIntegration_Chaos_BalanceAtomic_ConnectionResetOnDoubleEntry verifies that
// ProcessBalanceAtomicOperation with routeValidationEnabled=true handles a
// connection reset (proxy disconnect/reconnect) gracefully. The test confirms
// that after recovery the balance state in Redis is consistent and no version
// gaps were introduced by the failed operation.
//
// 5-Phase structure:
//  1. Normal   -- Two operations succeed, building up balance state
//  2. Inject   -- Toxiproxy proxy disabled (simulating connection reset)
//  3. Verify   -- Operation returns error; balance in Redis unchanged from Phase 1
//  4. Restore  -- Toxiproxy proxy re-enabled
//  5. Recovery -- Operation succeeds; final balance state is consistent
func TestIntegration_Chaos_BalanceAtomic_ConnectionResetOnDoubleEntry(t *testing.T) {
	if os.Getenv("CHAOS") != "1" {
		t.Skip("Set CHAOS=1 to run chaos tests")
	}

	if testing.Short() {
		t.Skip("Skipping chaos test in short mode")
	}

	infra := setupRedisChaosNetworkInfra(t)

	orgID := uuid.New()
	ledgerID := uuid.New()
	alias := "@reset-src-" + uuid.New().String()[:8]
	initialAvailable := decimal.NewFromInt(3000)
	initialOnHold := decimal.NewFromInt(0)
	initialVersion := int64(1)

	ctx := context.Background()

	// --- Phase 1: Normal ---
	// Execute first operation to establish balance state.
	t.Log("Phase 1 (Normal): establishing initial balance state via successful operation")

	txID1 := uuid.New()
	ops1 := buildTestBalanceOps(orgID, ledgerID, alias, initialAvailable, initialOnHold, initialVersion, true)

	result1, err := infra.proxyRepo.ProcessBalanceAtomicOperation(
		ctx, orgID, ledgerID, txID1, constant.PENDING, true, ops1,
	)
	require.NoError(t, err, "Phase 1: first operation should succeed")
	require.NotNil(t, result1, "Phase 1: result must not be nil")
	require.Len(t, result1.After, 1, "Phase 1: should have 1 after-balance entry")

	phase1After := result1.After[0]
	phase1Version := initialVersion + 2 // route-enabled: version + 2
	phase1Available := initialAvailable.Sub(decimal.NewFromInt(100))
	phase1OnHold := initialOnHold.Add(decimal.NewFromInt(100))

	assert.Equal(t, phase1Version, phase1After.Version, "Phase 1: version should be %d", phase1Version)
	assert.True(t, phase1Available.Equal(phase1After.Available), "Phase 1: available should be %s", phase1Available.String())
	assert.True(t, phase1OnHold.Equal(phase1After.OnHold), "Phase 1: onHold should be %s", phase1OnHold.String())

	t.Logf("Phase 1: established state -- version=%d, available=%s, onHold=%s",
		phase1After.Version, phase1After.Available.String(), phase1After.OnHold.String())

	// Record balance key for direct verification
	balanceKey := pkgTransaction.AliasKey(alias, constant.DefaultBalanceKey)
	internalKey := utils.BalanceInternalKey(orgID, ledgerID, balanceKey)

	// --- Phase 2: Inject ---
	// Disable the proxy to simulate a connection reset.
	t.Log("Phase 2 (Inject): disabling Toxiproxy proxy to simulate connection reset")

	err = infra.proxy.Disconnect()
	require.NoError(t, err, "Phase 2: Toxiproxy Disconnect should not fail")

	// --- Phase 3: Verify ---
	// Operation must return an error. Balance in Redis must remain at Phase 1 state.
	t.Log("Phase 3 (Verify): operation must return error and balance must remain unchanged")

	txIDChaos := uuid.New()
	chaosOps := buildTestBalanceOps(orgID, ledgerID, alias, phase1Available, phase1OnHold, phase1Version, true)

	var resetErr error
	var resetResult *mmodel.BalanceAtomicResult

	require.NotPanics(t, func() {
		resetResult, resetErr = infra.proxyRepo.ProcessBalanceAtomicOperation(
			ctx, orgID, ledgerID, txIDChaos, constant.PENDING, true, chaosOps,
		)
	}, "Phase 3: operation must not panic during connection reset")

	assert.Error(t, resetErr,
		"Phase 3: operation must return an error when connection is reset")
	assert.Nil(t, resetResult,
		"Phase 3: result must be nil when the operation fails")

	t.Logf("Phase 3: received expected error: %v", resetErr)

	// Verify balance is unchanged via direct Redis client
	verifyRedisBalance(t, infra, internalKey, phase1Available, phase1OnHold, phase1Version)
	t.Log("Phase 3: confirmed balance unchanged in Redis after connection reset")

	// --- Phase 4: Restore ---
	// Re-enable the proxy to restore connectivity.
	t.Log("Phase 4 (Restore): re-enabling Toxiproxy proxy")

	err = infra.proxy.Reconnect()
	require.NoError(t, err, "Phase 4: Toxiproxy Reconnect should not fail")

	// --- Phase 5: Recovery ---
	// After recovery, a new operation must succeed with correct version chaining.
	// Since Phase 3 failed, the balance is at Phase 1 state, so the new operation
	// should produce version = phase1Version + 2.
	t.Log("Phase 5 (Recovery): verifying operation succeeds with correct version after recovery")

	recoveryTxID := uuid.New()
	recoveryOps := buildTestBalanceOps(orgID, ledgerID, alias, phase1Available, phase1OnHold, phase1Version, true)

	var recoveryResult *mmodel.BalanceAtomicResult

	chaos.AssertRecoveryWithin(t, func() error {
		var err error
		recoveryResult, err = infra.proxyRepo.ProcessBalanceAtomicOperation(
			ctx, orgID, ledgerID, recoveryTxID, constant.PENDING, true, recoveryOps,
		)

		return err
	}, 10*time.Second, "Phase 5: operation should succeed after proxy is restored")

	require.NotNil(t, recoveryResult, "Phase 5: recovery result must not be nil")
	require.Len(t, recoveryResult.After, 1, "Phase 5: should have 1 after-balance entry")

	recoveryAfter := recoveryResult.After[0]
	expectedFinalVersion := phase1Version + 2
	expectedFinalAvailable := phase1Available.Sub(decimal.NewFromInt(100))
	expectedFinalOnHold := phase1OnHold.Add(decimal.NewFromInt(100))

	assert.Equal(t, expectedFinalVersion, recoveryAfter.Version,
		"Phase 5: version should be %d (phase1 + 2)", expectedFinalVersion)
	assert.True(t, expectedFinalAvailable.Equal(recoveryAfter.Available),
		"Phase 5: available should be %s, got %s", expectedFinalAvailable.String(), recoveryAfter.Available.String())
	assert.True(t, expectedFinalOnHold.Equal(recoveryAfter.OnHold),
		"Phase 5: onHold should be %s, got %s", expectedFinalOnHold.String(), recoveryAfter.OnHold.String())

	// Final integrity check: verify Redis state matches the recovery result
	verifyRedisBalance(t, infra, internalKey, expectedFinalAvailable, expectedFinalOnHold, expectedFinalVersion)

	t.Logf("Phase 5: final state -- version=%d, available=%s, onHold=%s",
		recoveryAfter.Version, recoveryAfter.Available.String(), recoveryAfter.OnHold.String())

	t.Log("PASS: operation handles connection reset gracefully, balance state preserved, recovers with correct version chain")
}

// =============================================================================
// CONNECTION LOSS DURING APPROVED TRANSACTION (SOURCE: DEBIT OnHold--)
// =============================================================================

// TestIntegration_Chaos_BalanceAtomic_ConnectionLossOnApprovedSource verifies
// that ProcessBalanceAtomicOperation for an APPROVED source (DEBIT: OnHold--)
// returns a non-nil error and does not panic when the Redis connection is
// dropped. After recovery, the operation succeeds with correct version chaining.
//
// 5-Phase structure:
//  1. Normal   -- PENDING creates initial balance state, then APPROVED DEBIT succeeds
//  2. Inject   -- Toxiproxy proxy disabled (connection loss)
//  3. Verify   -- APPROVED DEBIT returns error; no panic; balance unchanged
//  4. Restore  -- Toxiproxy proxy re-enabled
//  5. Recovery -- APPROVED DEBIT succeeds with correct version
func TestIntegration_Chaos_BalanceAtomic_ConnectionLossOnApprovedSource(t *testing.T) {
	if os.Getenv("CHAOS") != "1" {
		t.Skip("Set CHAOS=1 to run chaos tests")
	}

	if testing.Short() {
		t.Skip("Skipping chaos test in short mode")
	}

	infra := setupRedisChaosNetworkInfra(t)

	orgID := uuid.New()
	ledgerID := uuid.New()
	alias := "@approved-src-" + uuid.New().String()[:8]
	initialAvailable := decimal.NewFromInt(2000)
	initialOnHold := decimal.NewFromInt(0)
	initialVersion := int64(1)

	ctx := context.Background()

	// Pre-condition: execute a PENDING operation to put funds on hold.
	// This establishes the balance state that an APPROVED transaction operates on.
	pendingTxID := uuid.New()
	pendingOps := buildTestBalanceOps(orgID, ledgerID, alias, initialAvailable, initialOnHold, initialVersion, true)

	pendingResult, err := infra.proxyRepo.ProcessBalanceAtomicOperation(
		ctx, orgID, ledgerID, pendingTxID, constant.PENDING, true, pendingOps,
	)
	require.NoError(t, err, "Pre-condition: PENDING operation should succeed")
	require.NotNil(t, pendingResult, "Pre-condition: result must not be nil")
	require.Len(t, pendingResult.After, 1, "Pre-condition: should have 1 after-balance entry")

	// After PENDING with route enabled: Available -= 100, OnHold += 100, Version += 2
	postPendingAvailable := initialAvailable.Sub(decimal.NewFromInt(100))
	postPendingOnHold := initialOnHold.Add(decimal.NewFromInt(100))
	postPendingVersion := initialVersion + 2

	assert.Equal(t, postPendingVersion, pendingResult.After[0].Version,
		"Pre-condition: version should be %d after PENDING", postPendingVersion)

	// --- Phase 1: Normal ---
	// Execute APPROVED DEBIT (source: OnHold--) to verify it works through the proxy.
	t.Log("Phase 1 (Normal): verifying APPROVED DEBIT succeeds through proxy")

	approvedTxID := uuid.New()
	approvedOps := buildApprovedBalanceOps(orgID, ledgerID, alias,
		postPendingAvailable, postPendingOnHold, postPendingVersion, true)

	result, err := infra.proxyRepo.ProcessBalanceAtomicOperation(
		ctx, orgID, ledgerID, approvedTxID, constant.APPROVED, true, approvedOps,
	)
	require.NoError(t, err, "Phase 1: APPROVED DEBIT should succeed through proxy")
	require.NotNil(t, result, "Phase 1: result must not be nil")
	require.Len(t, result.After, 1, "Phase 1: should have 1 after-balance entry")

	phase1After := result.After[0]
	phase1Version := postPendingVersion + 1 // APPROVED DEBIT: version + 1
	phase1Available := postPendingAvailable // Available unchanged by DEBIT
	phase1OnHold := postPendingOnHold.Sub(decimal.NewFromInt(100))

	assert.Equal(t, phase1Version, phase1After.Version,
		"Phase 1: version should be %d after APPROVED DEBIT", phase1Version)
	assert.True(t, phase1Available.Equal(phase1After.Available),
		"Phase 1: available should be %s", phase1Available.String())
	assert.True(t, phase1OnHold.Equal(phase1After.OnHold),
		"Phase 1: onHold should be %s, got %s", phase1OnHold.String(), phase1After.OnHold.String())

	t.Logf("Phase 1: version=%d, available=%s, onHold=%s",
		phase1After.Version, phase1After.Available.String(), phase1After.OnHold.String())

	balanceKey := pkgTransaction.AliasKey(alias, constant.DefaultBalanceKey)
	internalKey := utils.BalanceInternalKey(orgID, ledgerID, balanceKey)

	// --- Phase 2: Inject ---
	t.Log("Phase 2 (Inject): disabling Toxiproxy proxy to simulate connection loss")

	err = infra.proxy.Disconnect()
	require.NoError(t, err, "Phase 2: Toxiproxy Disconnect should not fail")

	// --- Phase 3: Verify ---
	t.Log("Phase 3 (Verify): APPROVED DEBIT must return error, not panic, when connection is lost")

	chaosTxID := uuid.New()
	chaosOps := buildApprovedBalanceOps(orgID, ledgerID, alias,
		phase1Available, phase1OnHold, phase1Version, true)

	var chaosErr error
	var chaosResult *mmodel.BalanceAtomicResult

	require.NotPanics(t, func() {
		chaosResult, chaosErr = infra.proxyRepo.ProcessBalanceAtomicOperation(
			ctx, orgID, ledgerID, chaosTxID, constant.APPROVED, true, chaosOps,
		)
	}, "Phase 3: APPROVED DEBIT must not panic on connection loss")

	assert.Error(t, chaosErr,
		"Phase 3: APPROVED DEBIT must return an error when Redis connection is dropped")
	assert.Nil(t, chaosResult,
		"Phase 3: result must be nil when the operation fails")

	t.Logf("Phase 3: received expected error: %v", chaosErr)

	// Verify balance unchanged via direct Redis client
	verifyRedisBalance(t, infra, internalKey, phase1Available, phase1OnHold, phase1Version)
	t.Log("Phase 3: confirmed balance unchanged in Redis after connection loss")

	// --- Phase 4: Restore ---
	t.Log("Phase 4 (Restore): re-enabling Toxiproxy proxy")

	err = infra.proxy.Reconnect()
	require.NoError(t, err, "Phase 4: Toxiproxy Reconnect should not fail")

	// --- Phase 5: Recovery ---
	t.Log("Phase 5 (Recovery): verifying APPROVED DEBIT succeeds after proxy restoration")

	recoveryTxID := uuid.New()
	recoveryOps := buildApprovedBalanceOps(orgID, ledgerID, alias,
		phase1Available, phase1OnHold, phase1Version, true)

	var recoveryResult *mmodel.BalanceAtomicResult

	chaos.AssertRecoveryWithin(t, func() error {
		var err error
		recoveryResult, err = infra.proxyRepo.ProcessBalanceAtomicOperation(
			ctx, orgID, ledgerID, recoveryTxID, constant.APPROVED, true, recoveryOps,
		)

		return err
	}, 10*time.Second, "Phase 5: APPROVED DEBIT should succeed after proxy is restored")

	require.NotNil(t, recoveryResult, "Phase 5: recovery result must not be nil")
	require.Len(t, recoveryResult.After, 1, "Phase 5: should have 1 after-balance entry")

	recoveryAfter := recoveryResult.After[0]
	expectedRecoveryVersion := phase1Version + 1

	assert.Equal(t, expectedRecoveryVersion, recoveryAfter.Version,
		"Phase 5: version should be %d after recovery", expectedRecoveryVersion)

	// Verify final state in Redis directly
	verifyRedisBalance(t, infra, internalKey,
		phase1Available,
		phase1OnHold.Sub(decimal.NewFromInt(100)),
		expectedRecoveryVersion)

	t.Logf("Phase 5: recovery version=%d, available=%s, onHold=%s",
		recoveryAfter.Version, recoveryAfter.Available.String(), recoveryAfter.OnHold.String())

	t.Log("PASS: APPROVED DEBIT returns error on connection loss, balance preserved, recovers correctly")
}

// =============================================================================
// HIGH LATENCY DURING APPROVED TRANSACTION (DESTINATION: CREDIT Available++)
// =============================================================================

// TestIntegration_Chaos_BalanceAtomic_HighLatencyOnApprovedDestination verifies
// that ProcessBalanceAtomicOperation for an APPROVED destination (CREDIT: Available++)
// times out gracefully when Toxiproxy injects high latency and the caller uses
// a context with a short deadline. Balance state must remain consistent.
//
// 5-Phase structure:
//  1. Normal   -- APPROVED CREDIT succeeds with no latency
//  2. Inject   -- 5000 ms latency toxic added to proxy
//  3. Verify   -- APPROVED CREDIT with 1s deadline returns timeout error; balance consistent
//  4. Restore  -- Latency toxic removed
//  5. Recovery -- APPROVED CREDIT succeeds within normal deadline
func TestIntegration_Chaos_BalanceAtomic_HighLatencyOnApprovedDestination(t *testing.T) {
	if os.Getenv("CHAOS") != "1" {
		t.Skip("Set CHAOS=1 to run chaos tests")
	}

	if testing.Short() {
		t.Skip("Skipping chaos test in short mode")
	}

	infra := setupRedisChaosNetworkInfra(t)

	orgID := uuid.New()
	ledgerID := uuid.New()
	alias := "@approved-dest-" + uuid.New().String()[:8]
	// Destination starts with lower available, will receive credits
	initialAvailable := decimal.NewFromInt(500)
	initialOnHold := decimal.NewFromInt(0)
	initialVersion := int64(1)

	ctx := context.Background()

	// --- Phase 1: Normal ---
	// Execute APPROVED CREDIT (destination: Available++) to verify it works.
	t.Log("Phase 1 (Normal): verifying APPROVED CREDIT succeeds with no latency")

	txID := uuid.New()
	ops := buildApprovedBalanceOps(orgID, ledgerID, alias,
		initialAvailable, initialOnHold, initialVersion, false)

	result, err := infra.proxyRepo.ProcessBalanceAtomicOperation(
		ctx, orgID, ledgerID, txID, constant.APPROVED, true, ops,
	)
	require.NoError(t, err, "Phase 1: APPROVED CREDIT should succeed")
	require.NotNil(t, result, "Phase 1: result must not be nil")
	require.Len(t, result.After, 1, "Phase 1: should have 1 after-balance entry")

	phase1After := result.After[0]
	phase1Version := initialVersion + 1
	phase1Available := initialAvailable.Add(decimal.NewFromInt(100))

	assert.Equal(t, phase1Version, phase1After.Version,
		"Phase 1: version should be %d", phase1Version)
	assert.True(t, phase1Available.Equal(phase1After.Available),
		"Phase 1: available should be %s", phase1Available.String())

	t.Logf("Phase 1: version=%d, available=%s, onHold=%s",
		phase1After.Version, phase1After.Available.String(), phase1After.OnHold.String())

	balanceKey := pkgTransaction.AliasKey(alias, constant.DefaultBalanceKey)
	internalKey := utils.BalanceInternalKey(orgID, ledgerID, balanceKey)

	// --- Phase 2: Inject ---
	t.Log("Phase 2 (Inject): adding 5000 ms downstream latency via Toxiproxy")

	err = infra.proxy.AddLatency(5000*time.Millisecond, 0)
	require.NoError(t, err, "Phase 2: AddLatency should not fail")

	// --- Phase 3: Verify ---
	t.Log("Phase 3 (Verify): APPROVED CREDIT must return error within 1s deadline under 5s latency")

	latencyTxID := uuid.New()
	latencyOps := buildApprovedBalanceOps(orgID, ledgerID, alias,
		phase1Available, initialOnHold, phase1Version, false)

	highLatencyCtx, highLatencyCancel := context.WithTimeout(ctx, 1*time.Second)
	defer highLatencyCancel()

	var latencyErr error
	var latencyResult *mmodel.BalanceAtomicResult

	done := make(chan struct{})

	go func() {
		defer close(done)
		latencyResult, latencyErr = infra.proxyRepo.ProcessBalanceAtomicOperation(
			highLatencyCtx, orgID, ledgerID, latencyTxID, constant.APPROVED, true, latencyOps,
		)
	}()

	select {
	case <-done:
		// Call returned within acceptable time.
	case <-time.After(3 * time.Second):
		t.Fatal("Phase 3: operation hung for more than 3s -- should have returned an error within 1s deadline")
	}

	assert.Error(t, latencyErr,
		"Phase 3: APPROVED CREDIT must return an error when context deadline expires")
	assert.Nil(t, latencyResult,
		"Phase 3: result must be nil when the operation times out")

	t.Logf("Phase 3: received expected error: %v", latencyErr)

	// Verify balance version is either unchanged or advanced (Lua is atomic).
	raw, rawErr := infra.redisContainer.Client.Get(context.Background(), internalKey).Result()
	require.NoError(t, rawErr, "Phase 3: direct Redis GET should succeed")

	var storedBalance mmodel.BalanceRedis

	err = json.Unmarshal([]byte(raw), &storedBalance)
	require.NoError(t, err, "Phase 3: balance JSON should unmarshal")

	validVersions := []int64{phase1Version, phase1Version + 1}
	assert.Contains(t, validVersions, storedBalance.Version,
		"Phase 3: version must be %d (unchanged) or %d (script completed), got %d",
		phase1Version, phase1Version+1, storedBalance.Version)

	t.Logf("Phase 3: Redis balance version=%d (unchanged=%v)",
		storedBalance.Version, storedBalance.Version == phase1Version)

	// --- Phase 4: Restore ---
	t.Log("Phase 4 (Restore): removing latency toxic")

	err = infra.proxy.RemoveAllToxics()
	require.NoError(t, err, "Phase 4: RemoveAllToxics should not fail")

	// --- Phase 5: Recovery ---
	t.Log("Phase 5 (Recovery): verifying APPROVED CREDIT succeeds after latency removed")

	recoveryTxID := uuid.New()

	// Read current state from Redis directly (latency may or may not have mutated)
	currentRaw, currentErr := infra.redisContainer.Client.Get(context.Background(), internalKey).Result()
	require.NoError(t, currentErr, "Phase 5: should read current balance from Redis")

	var currentBalance mmodel.BalanceRedis

	err = json.Unmarshal([]byte(currentRaw), &currentBalance)
	require.NoError(t, err, "Phase 5: should unmarshal current balance")

	recoveryOps := buildApprovedBalanceOps(orgID, ledgerID, alias,
		currentBalance.Available, currentBalance.OnHold, currentBalance.Version, false)

	var recoveryResult *mmodel.BalanceAtomicResult

	chaos.AssertRecoveryWithin(t, func() error {
		var err error
		recoveryResult, err = infra.proxyRepo.ProcessBalanceAtomicOperation(
			context.Background(), orgID, ledgerID, recoveryTxID, constant.APPROVED, true, recoveryOps,
		)

		return err
	}, 15*time.Second, "Phase 5: APPROVED CREDIT should succeed after latency is removed")

	require.NotNil(t, recoveryResult, "Phase 5: recovery result must not be nil")
	require.Len(t, recoveryResult.After, 1, "Phase 5: should have 1 after-balance entry")

	recoveryAfter := recoveryResult.After[0]
	expectedRecoveryVersion := currentBalance.Version + 1

	assert.Equal(t, expectedRecoveryVersion, recoveryAfter.Version,
		"Phase 5: version should be %d after recovery", expectedRecoveryVersion)

	t.Logf("Phase 5: recovery version=%d, available=%s, onHold=%s",
		recoveryAfter.Version, recoveryAfter.Available.String(), recoveryAfter.OnHold.String())

	t.Log("PASS: APPROVED CREDIT times out gracefully under high latency, balance consistent, recovers correctly")
}

// =============================================================================
// CONNECTION LOSS DURING CANCELED RELEASE (DOUBLE-ENTRY: OnHold-- only)
// =============================================================================

// TestIntegration_Chaos_BalanceAtomic_ConnectionLossOnCanceledRelease verifies
// that ProcessBalanceAtomicOperation for a CANCELED RELEASE with
// routeValidationEnabled=true (OnHold-- only, Available unchanged) returns an
// error and does not panic when the connection is dropped.
//
// 5-Phase structure:
//  1. Normal   -- PENDING establishes on-hold state, then RELEASE+CANCELED succeeds
//  2. Inject   -- Toxiproxy proxy disabled
//  3. Verify   -- RELEASE+CANCELED returns error; balance unchanged
//  4. Restore  -- Toxiproxy proxy re-enabled
//  5. Recovery -- RELEASE+CANCELED succeeds with correct version
func TestIntegration_Chaos_BalanceAtomic_ConnectionLossOnCanceledRelease(t *testing.T) {
	if os.Getenv("CHAOS") != "1" {
		t.Skip("Set CHAOS=1 to run chaos tests")
	}

	if testing.Short() {
		t.Skip("Skipping chaos test in short mode")
	}

	infra := setupRedisChaosNetworkInfra(t)

	orgID := uuid.New()
	ledgerID := uuid.New()
	alias := "@cancel-rel-" + uuid.New().String()[:8]
	initialAvailable := decimal.NewFromInt(3000)
	initialOnHold := decimal.NewFromInt(0)
	initialVersion := int64(1)

	ctx := context.Background()

	// Pre-condition: PENDING to create on-hold state (route enabled -> version +2)
	pendingTxID := uuid.New()
	pendingOps := buildTestBalanceOps(orgID, ledgerID, alias, initialAvailable, initialOnHold, initialVersion, true)

	pendingResult, err := infra.proxyRepo.ProcessBalanceAtomicOperation(
		ctx, orgID, ledgerID, pendingTxID, constant.PENDING, true, pendingOps,
	)
	require.NoError(t, err, "Pre-condition: PENDING should succeed")
	require.NotNil(t, pendingResult)

	postPendingAvailable := initialAvailable.Sub(decimal.NewFromInt(100))
	postPendingOnHold := initialOnHold.Add(decimal.NewFromInt(100))
	postPendingVersion := initialVersion + 2

	// --- Phase 1: Normal ---
	// RELEASE+CANCELED with routeValidationEnabled=true: OnHold-- only, version+1
	t.Log("Phase 1 (Normal): verifying RELEASE+CANCELED succeeds with routeValidationEnabled=true")

	cancelReleaseTxID := uuid.New()
	cancelReleaseOps := buildCanceledBalanceOps(orgID, ledgerID, alias,
		postPendingAvailable, postPendingOnHold, postPendingVersion,
		libConstants.RELEASE, true)

	result, err := infra.proxyRepo.ProcessBalanceAtomicOperation(
		ctx, orgID, ledgerID, cancelReleaseTxID, constant.CANCELED, true, cancelReleaseOps,
	)
	require.NoError(t, err, "Phase 1: RELEASE+CANCELED should succeed")
	require.NotNil(t, result)
	require.Len(t, result.After, 1)

	phase1After := result.After[0]
	phase1Version := postPendingVersion + 1
	phase1Available := postPendingAvailable                        // Available unchanged by RELEASE with route enabled
	phase1OnHold := postPendingOnHold.Sub(decimal.NewFromInt(100)) // OnHold decreased

	assert.Equal(t, phase1Version, phase1After.Version,
		"Phase 1: version should be %d", phase1Version)
	assert.True(t, phase1Available.Equal(phase1After.Available),
		"Phase 1: available should be unchanged at %s", phase1Available.String())
	assert.True(t, phase1OnHold.Equal(phase1After.OnHold),
		"Phase 1: onHold should be %s, got %s", phase1OnHold.String(), phase1After.OnHold.String())

	t.Logf("Phase 1: version=%d, available=%s, onHold=%s",
		phase1After.Version, phase1After.Available.String(), phase1After.OnHold.String())

	balanceKey := pkgTransaction.AliasKey(alias, constant.DefaultBalanceKey)
	internalKey := utils.BalanceInternalKey(orgID, ledgerID, balanceKey)

	// --- Phase 2: Inject ---
	t.Log("Phase 2 (Inject): disabling Toxiproxy proxy to simulate connection loss")

	err = infra.proxy.Disconnect()
	require.NoError(t, err, "Phase 2: Toxiproxy Disconnect should not fail")

	// --- Phase 3: Verify ---
	t.Log("Phase 3 (Verify): RELEASE+CANCELED must return error, not panic")

	chaosTxID := uuid.New()
	// Build a new PENDING first to have something to cancel (need on-hold balance)
	// For this test, we re-use the current state to attempt another RELEASE
	chaosOps := buildCanceledBalanceOps(orgID, ledgerID, alias,
		phase1Available, phase1OnHold, phase1Version,
		libConstants.RELEASE, true)

	var chaosErr error
	var chaosResult *mmodel.BalanceAtomicResult

	require.NotPanics(t, func() {
		chaosResult, chaosErr = infra.proxyRepo.ProcessBalanceAtomicOperation(
			ctx, orgID, ledgerID, chaosTxID, constant.CANCELED, true, chaosOps,
		)
	}, "Phase 3: RELEASE+CANCELED must not panic on connection loss")

	assert.Error(t, chaosErr,
		"Phase 3: RELEASE+CANCELED must return an error when Redis connection is dropped")
	assert.Nil(t, chaosResult,
		"Phase 3: result must be nil when the operation fails")

	t.Logf("Phase 3: received expected error: %v", chaosErr)

	// Verify balance unchanged
	verifyRedisBalance(t, infra, internalKey, phase1Available, phase1OnHold, phase1Version)
	t.Log("Phase 3: confirmed balance unchanged after connection loss")

	// --- Phase 4: Restore ---
	t.Log("Phase 4 (Restore): re-enabling Toxiproxy proxy")

	err = infra.proxy.Reconnect()
	require.NoError(t, err, "Phase 4: Toxiproxy Reconnect should not fail")

	// --- Phase 5: Recovery ---
	t.Log("Phase 5 (Recovery): verifying RELEASE+CANCELED succeeds after proxy restoration")

	recoveryTxID := uuid.New()
	recoveryOps := buildCanceledBalanceOps(orgID, ledgerID, alias,
		phase1Available, phase1OnHold, phase1Version,
		libConstants.RELEASE, true)

	var recoveryResult *mmodel.BalanceAtomicResult

	chaos.AssertRecoveryWithin(t, func() error {
		var err error
		recoveryResult, err = infra.proxyRepo.ProcessBalanceAtomicOperation(
			ctx, orgID, ledgerID, recoveryTxID, constant.CANCELED, true, recoveryOps,
		)

		return err
	}, 10*time.Second, "Phase 5: RELEASE+CANCELED should succeed after proxy is restored")

	require.NotNil(t, recoveryResult, "Phase 5: recovery result must not be nil")
	require.Len(t, recoveryResult.After, 1)

	recoveryAfter := recoveryResult.After[0]
	expectedRecoveryVersion := phase1Version + 1

	assert.Equal(t, expectedRecoveryVersion, recoveryAfter.Version,
		"Phase 5: version should be %d after recovery", expectedRecoveryVersion)

	verifyRedisBalance(t, infra, internalKey,
		phase1Available,
		phase1OnHold.Sub(decimal.NewFromInt(100)),
		expectedRecoveryVersion)

	t.Logf("Phase 5: recovery version=%d, available=%s, onHold=%s",
		recoveryAfter.Version, recoveryAfter.Available.String(), recoveryAfter.OnHold.String())

	t.Log("PASS: RELEASE+CANCELED returns error on connection loss, balance preserved, recovers correctly")
}

// =============================================================================
// HIGH LATENCY DURING CANCELED CREDIT (DOUBLE-ENTRY: Available++ only)
// =============================================================================

// TestIntegration_Chaos_BalanceAtomic_HighLatencyOnCanceledCredit verifies that
// ProcessBalanceAtomicOperation for a CANCELED CREDIT with
// routeValidationEnabled=true (Available++ only) times out gracefully under
// high latency. This is the second half of the CANCELED double-entry path.
//
// 5-Phase structure:
//  1. Normal   -- CREDIT+CANCELED with route enabled succeeds
//  2. Inject   -- 5000 ms latency toxic added
//  3. Verify   -- Operation with 1s deadline returns timeout; balance consistent
//  4. Restore  -- Latency removed
//  5. Recovery -- Operation succeeds
func TestIntegration_Chaos_BalanceAtomic_HighLatencyOnCanceledCredit(t *testing.T) {
	if os.Getenv("CHAOS") != "1" {
		t.Skip("Set CHAOS=1 to run chaos tests")
	}

	if testing.Short() {
		t.Skip("Skipping chaos test in short mode")
	}

	infra := setupRedisChaosNetworkInfra(t)

	orgID := uuid.New()
	ledgerID := uuid.New()
	alias := "@cancel-cred-" + uuid.New().String()[:8]
	// Balance state after a PENDING and a RELEASE: Available was decremented,
	// OnHold was incremented then decremented back. We simulate the state
	// as if RELEASE already happened.
	initialAvailable := decimal.NewFromInt(1900) // e.g., started at 2000, PENDING took 100
	initialOnHold := decimal.NewFromInt(0)       // RELEASE brought it back to 0
	initialVersion := int64(5)                   // Several operations already happened

	ctx := context.Background()

	// --- Phase 1: Normal ---
	// CREDIT+CANCELED with routeValidationEnabled=true: Available++ only, version+1
	t.Log("Phase 1 (Normal): verifying CREDIT+CANCELED succeeds with routeValidationEnabled=true")

	txID := uuid.New()
	ops := buildCanceledBalanceOps(orgID, ledgerID, alias,
		initialAvailable, initialOnHold, initialVersion,
		libConstants.CREDIT, true)

	result, err := infra.proxyRepo.ProcessBalanceAtomicOperation(
		ctx, orgID, ledgerID, txID, constant.CANCELED, true, ops,
	)
	require.NoError(t, err, "Phase 1: CREDIT+CANCELED should succeed")
	require.NotNil(t, result)
	require.Len(t, result.After, 1)

	phase1After := result.After[0]
	phase1Version := initialVersion + 1
	phase1Available := initialAvailable.Add(decimal.NewFromInt(100))

	assert.Equal(t, phase1Version, phase1After.Version,
		"Phase 1: version should be %d", phase1Version)
	assert.True(t, phase1Available.Equal(phase1After.Available),
		"Phase 1: available should be %s", phase1Available.String())
	assert.True(t, initialOnHold.Equal(phase1After.OnHold),
		"Phase 1: onHold should be unchanged at %s", initialOnHold.String())

	t.Logf("Phase 1: version=%d, available=%s, onHold=%s",
		phase1After.Version, phase1After.Available.String(), phase1After.OnHold.String())

	balanceKey := pkgTransaction.AliasKey(alias, constant.DefaultBalanceKey)
	internalKey := utils.BalanceInternalKey(orgID, ledgerID, balanceKey)

	// --- Phase 2: Inject ---
	t.Log("Phase 2 (Inject): adding 5000 ms downstream latency via Toxiproxy")

	err = infra.proxy.AddLatency(5000*time.Millisecond, 0)
	require.NoError(t, err, "Phase 2: AddLatency should not fail")

	// --- Phase 3: Verify ---
	t.Log("Phase 3 (Verify): CREDIT+CANCELED must return error within 1s deadline under 5s latency")

	latencyTxID := uuid.New()
	latencyOps := buildCanceledBalanceOps(orgID, ledgerID, alias,
		phase1Available, initialOnHold, phase1Version,
		libConstants.CREDIT, true)

	highLatencyCtx, highLatencyCancel := context.WithTimeout(ctx, 1*time.Second)
	defer highLatencyCancel()

	var latencyErr error
	var latencyResult *mmodel.BalanceAtomicResult

	done := make(chan struct{})

	go func() {
		defer close(done)
		latencyResult, latencyErr = infra.proxyRepo.ProcessBalanceAtomicOperation(
			highLatencyCtx, orgID, ledgerID, latencyTxID, constant.CANCELED, true, latencyOps,
		)
	}()

	select {
	case <-done:
		// Call returned.
	case <-time.After(3 * time.Second):
		t.Fatal("Phase 3: operation hung for more than 3s -- should have returned within 1s deadline")
	}

	assert.Error(t, latencyErr,
		"Phase 3: CREDIT+CANCELED must return error when context deadline expires")
	assert.Nil(t, latencyResult,
		"Phase 3: result must be nil when operation times out")

	t.Logf("Phase 3: received expected error: %v", latencyErr)

	// Verify balance consistency: version either unchanged or advanced
	raw, rawErr := infra.redisContainer.Client.Get(context.Background(), internalKey).Result()
	require.NoError(t, rawErr, "Phase 3: direct Redis GET should succeed")

	var storedBalance mmodel.BalanceRedis

	err = json.Unmarshal([]byte(raw), &storedBalance)
	require.NoError(t, err, "Phase 3: balance JSON should unmarshal")

	validVersions := []int64{phase1Version, phase1Version + 1}
	assert.Contains(t, validVersions, storedBalance.Version,
		"Phase 3: version must be %d or %d, got %d",
		phase1Version, phase1Version+1, storedBalance.Version)

	t.Logf("Phase 3: Redis balance version=%d (unchanged=%v)",
		storedBalance.Version, storedBalance.Version == phase1Version)

	// --- Phase 4: Restore ---
	t.Log("Phase 4 (Restore): removing latency toxic")

	err = infra.proxy.RemoveAllToxics()
	require.NoError(t, err, "Phase 4: RemoveAllToxics should not fail")

	// --- Phase 5: Recovery ---
	t.Log("Phase 5 (Recovery): verifying CREDIT+CANCELED succeeds after latency removed")

	recoveryTxID := uuid.New()

	// Read current state from Redis
	currentRaw, currentErr := infra.redisContainer.Client.Get(context.Background(), internalKey).Result()
	require.NoError(t, currentErr, "Phase 5: should read current balance")

	var currentBalance mmodel.BalanceRedis

	err = json.Unmarshal([]byte(currentRaw), &currentBalance)
	require.NoError(t, err, "Phase 5: should unmarshal current balance")

	recoveryOps := buildCanceledBalanceOps(orgID, ledgerID, alias,
		currentBalance.Available, currentBalance.OnHold, currentBalance.Version,
		libConstants.CREDIT, true)

	var recoveryResult *mmodel.BalanceAtomicResult

	chaos.AssertRecoveryWithin(t, func() error {
		var err error
		recoveryResult, err = infra.proxyRepo.ProcessBalanceAtomicOperation(
			context.Background(), orgID, ledgerID, recoveryTxID, constant.CANCELED, true, recoveryOps,
		)

		return err
	}, 15*time.Second, "Phase 5: CREDIT+CANCELED should succeed after latency removed")

	require.NotNil(t, recoveryResult, "Phase 5: recovery result must not be nil")
	require.Len(t, recoveryResult.After, 1)

	recoveryAfter := recoveryResult.After[0]
	expectedRecoveryVersion := currentBalance.Version + 1

	assert.Equal(t, expectedRecoveryVersion, recoveryAfter.Version,
		"Phase 5: version should be %d after recovery", expectedRecoveryVersion)

	t.Logf("Phase 5: recovery version=%d, available=%s, onHold=%s",
		recoveryAfter.Version, recoveryAfter.Available.String(), recoveryAfter.OnHold.String())

	t.Log("PASS: CREDIT+CANCELED times out gracefully under high latency, balance consistent, recovers correctly")
}

// =============================================================================
// CONNECTION TIMEOUT DURING APPROVED ATOMIC OPERATION
// =============================================================================

// TestIntegration_Chaos_BalanceAtomic_TimeoutOnApprovedOperation verifies that
// ProcessBalanceAtomicOperation for an APPROVED transaction returns a context
// deadline error when the Redis proxy is configured with extreme latency. The
// test uses a very short context timeout (500ms) to ensure timeout behavior is
// exercised at the Go level, not just at the TCP layer.
//
// 5-Phase structure:
//  1. Normal   -- APPROVED operation succeeds
//  2. Inject   -- 10000 ms latency added (simulating Redis stall)
//  3. Verify   -- Operation with 500ms deadline returns timeout error
//  4. Restore  -- Latency removed
//  5. Recovery -- Operation succeeds
func TestIntegration_Chaos_BalanceAtomic_TimeoutOnApprovedOperation(t *testing.T) {
	if os.Getenv("CHAOS") != "1" {
		t.Skip("Set CHAOS=1 to run chaos tests")
	}

	if testing.Short() {
		t.Skip("Skipping chaos test in short mode")
	}

	infra := setupRedisChaosNetworkInfra(t)

	orgID := uuid.New()
	ledgerID := uuid.New()
	alias := "@timeout-approved-" + uuid.New().String()[:8]
	initialAvailable := decimal.NewFromInt(1000)
	initialOnHold := decimal.NewFromInt(200)
	initialVersion := int64(3)

	ctx := context.Background()

	// --- Phase 1: Normal ---
	t.Log("Phase 1 (Normal): verifying APPROVED DEBIT succeeds before timeout injection")

	txID := uuid.New()
	ops := buildApprovedBalanceOps(orgID, ledgerID, alias,
		initialAvailable, initialOnHold, initialVersion, true)

	result, err := infra.proxyRepo.ProcessBalanceAtomicOperation(
		ctx, orgID, ledgerID, txID, constant.APPROVED, true, ops,
	)
	require.NoError(t, err, "Phase 1: APPROVED operation should succeed")
	require.NotNil(t, result)
	require.Len(t, result.After, 1)

	phase1After := result.After[0]
	phase1Version := initialVersion + 1
	phase1Available := initialAvailable
	phase1OnHold := initialOnHold.Sub(decimal.NewFromInt(100))

	assert.Equal(t, phase1Version, phase1After.Version,
		"Phase 1: version should be %d", phase1Version)

	t.Logf("Phase 1: version=%d, available=%s, onHold=%s",
		phase1After.Version, phase1After.Available.String(), phase1After.OnHold.String())

	balanceKey := pkgTransaction.AliasKey(alias, constant.DefaultBalanceKey)
	internalKey := utils.BalanceInternalKey(orgID, ledgerID, balanceKey)

	// --- Phase 2: Inject ---
	t.Log("Phase 2 (Inject): adding 10000 ms latency to simulate Redis stall")

	err = infra.proxy.AddLatency(10000*time.Millisecond, 0)
	require.NoError(t, err, "Phase 2: AddLatency should not fail")

	// --- Phase 3: Verify ---
	t.Log("Phase 3 (Verify): APPROVED operation must timeout with 500ms deadline under 10s latency")

	timeoutTxID := uuid.New()
	timeoutOps := buildApprovedBalanceOps(orgID, ledgerID, alias,
		phase1Available, phase1OnHold, phase1Version, true)

	timeoutCtx, timeoutCancel := context.WithTimeout(ctx, 500*time.Millisecond)
	defer timeoutCancel()

	var timeoutErr error
	var timeoutResult *mmodel.BalanceAtomicResult

	done := make(chan struct{})

	go func() {
		defer close(done)
		timeoutResult, timeoutErr = infra.proxyRepo.ProcessBalanceAtomicOperation(
			timeoutCtx, orgID, ledgerID, timeoutTxID, constant.APPROVED, true, timeoutOps,
		)
	}()

	select {
	case <-done:
		// Returned within expected bounds.
	case <-time.After(3 * time.Second):
		t.Fatal("Phase 3: operation hung for more than 3s -- 500ms deadline should have triggered")
	}

	assert.Error(t, timeoutErr,
		"Phase 3: APPROVED operation must return error on timeout")
	assert.Nil(t, timeoutResult,
		"Phase 3: result must be nil when operation times out")

	t.Logf("Phase 3: received expected error: %v", timeoutErr)

	// Verify balance: either unchanged or completed (Lua is atomic)
	raw, rawErr := infra.redisContainer.Client.Get(context.Background(), internalKey).Result()
	require.NoError(t, rawErr, "Phase 3: direct Redis GET should succeed")

	var storedBalance mmodel.BalanceRedis

	err = json.Unmarshal([]byte(raw), &storedBalance)
	require.NoError(t, err, "Phase 3: balance JSON should unmarshal")

	validVersions := []int64{phase1Version, phase1Version + 1}
	assert.Contains(t, validVersions, storedBalance.Version,
		"Phase 3: version must be %d or %d, got %d",
		phase1Version, phase1Version+1, storedBalance.Version)

	// --- Phase 4: Restore ---
	t.Log("Phase 4 (Restore): removing latency toxic")

	err = infra.proxy.RemoveAllToxics()
	require.NoError(t, err, "Phase 4: RemoveAllToxics should not fail")

	// --- Phase 5: Recovery ---
	t.Log("Phase 5 (Recovery): verifying operation succeeds after timeout recovery")

	recoveryTxID := uuid.New()

	currentRaw, currentErr := infra.redisContainer.Client.Get(context.Background(), internalKey).Result()
	require.NoError(t, currentErr, "Phase 5: should read current balance")

	var currentBalance mmodel.BalanceRedis

	err = json.Unmarshal([]byte(currentRaw), &currentBalance)
	require.NoError(t, err, "Phase 5: should unmarshal current balance")

	recoveryOps := buildApprovedBalanceOps(orgID, ledgerID, alias,
		currentBalance.Available, currentBalance.OnHold, currentBalance.Version, true)

	var recoveryResult *mmodel.BalanceAtomicResult

	chaos.AssertRecoveryWithin(t, func() error {
		var err error
		recoveryResult, err = infra.proxyRepo.ProcessBalanceAtomicOperation(
			context.Background(), orgID, ledgerID, recoveryTxID, constant.APPROVED, true, recoveryOps,
		)

		return err
	}, 15*time.Second, "Phase 5: operation should succeed after timeout recovery")

	require.NotNil(t, recoveryResult, "Phase 5: recovery result must not be nil")
	require.Len(t, recoveryResult.After, 1)

	recoveryAfter := recoveryResult.After[0]
	expectedRecoveryVersion := currentBalance.Version + 1

	assert.Equal(t, expectedRecoveryVersion, recoveryAfter.Version,
		"Phase 5: version should be %d after recovery", expectedRecoveryVersion)

	t.Logf("Phase 5: recovery version=%d, available=%s, onHold=%s",
		recoveryAfter.Version, recoveryAfter.Available.String(), recoveryAfter.OnHold.String())

	t.Log("PASS: APPROVED operation times out correctly under extreme latency, recovers with correct version")
}

// =============================================================================
// RECOVERY AFTER RECONNECT FOR CANCELED DOUBLE-ENTRY
// =============================================================================

// TestIntegration_Chaos_BalanceAtomic_RecoveryAfterReconnectCanceled verifies
// the full fault-and-recovery cycle for a CANCELED double-entry operation with
// routeValidationEnabled=true. The test confirms that after Redis recovers from
// an outage, CANCELED operations produce correct state transitions and no
// version gaps are introduced.
//
// 5-Phase structure:
//  1. Normal   -- PENDING then RELEASE+CANCELED then CREDIT+CANCELED succeed
//  2. Inject   -- Toxiproxy proxy disabled
//  3. Verify   -- Both RELEASE and CREDIT return errors; balance unchanged
//  4. Restore  -- Toxiproxy proxy re-enabled
//  5. Recovery -- Full CANCELED double-entry (RELEASE then CREDIT) succeeds
func TestIntegration_Chaos_BalanceAtomic_RecoveryAfterReconnectCanceled(t *testing.T) {
	if os.Getenv("CHAOS") != "1" {
		t.Skip("Set CHAOS=1 to run chaos tests")
	}

	if testing.Short() {
		t.Skip("Skipping chaos test in short mode")
	}

	infra := setupRedisChaosNetworkInfra(t)

	orgID := uuid.New()
	ledgerID := uuid.New()
	alias := "@recovery-cancel-" + uuid.New().String()[:8]
	initialAvailable := decimal.NewFromInt(5000)
	initialOnHold := decimal.NewFromInt(0)
	initialVersion := int64(1)

	ctx := context.Background()

	// Pre-condition: PENDING with route enabled to establish on-hold state
	pendingTxID := uuid.New()
	pendingOps := buildTestBalanceOps(orgID, ledgerID, alias, initialAvailable, initialOnHold, initialVersion, true)

	pendingResult, err := infra.proxyRepo.ProcessBalanceAtomicOperation(
		ctx, orgID, ledgerID, pendingTxID, constant.PENDING, true, pendingOps,
	)
	require.NoError(t, err, "Pre-condition: PENDING should succeed")
	require.NotNil(t, pendingResult)

	postPendingAvailable := initialAvailable.Sub(decimal.NewFromInt(100))
	postPendingOnHold := initialOnHold.Add(decimal.NewFromInt(100))
	postPendingVersion := initialVersion + 2

	// --- Phase 1: Normal ---
	// Execute RELEASE+CANCELED (route enabled: OnHold-- only) then
	// CREDIT+CANCELED (route enabled: Available++ only) to verify both work.
	t.Log("Phase 1 (Normal): verifying CANCELED double-entry succeeds through proxy")

	// Phase 1a: RELEASE+CANCELED
	releaseTxID := uuid.New()
	releaseOps := buildCanceledBalanceOps(orgID, ledgerID, alias,
		postPendingAvailable, postPendingOnHold, postPendingVersion,
		libConstants.RELEASE, true)

	releaseResult, err := infra.proxyRepo.ProcessBalanceAtomicOperation(
		ctx, orgID, ledgerID, releaseTxID, constant.CANCELED, true, releaseOps,
	)
	require.NoError(t, err, "Phase 1a: RELEASE+CANCELED should succeed")
	require.NotNil(t, releaseResult)
	require.Len(t, releaseResult.After, 1)

	afterRelease := releaseResult.After[0]
	postReleaseVersion := postPendingVersion + 1
	postReleaseAvailable := postPendingAvailable                        // Available unchanged
	postReleaseOnHold := postPendingOnHold.Sub(decimal.NewFromInt(100)) // OnHold decreased

	assert.Equal(t, postReleaseVersion, afterRelease.Version,
		"Phase 1a: version should be %d after RELEASE", postReleaseVersion)

	// Phase 1b: CREDIT+CANCELED
	creditTxID := uuid.New()
	creditOps := buildCanceledBalanceOps(orgID, ledgerID, alias,
		postReleaseAvailable, postReleaseOnHold, postReleaseVersion,
		libConstants.CREDIT, true)

	creditResult, err := infra.proxyRepo.ProcessBalanceAtomicOperation(
		ctx, orgID, ledgerID, creditTxID, constant.CANCELED, true, creditOps,
	)
	require.NoError(t, err, "Phase 1b: CREDIT+CANCELED should succeed")
	require.NotNil(t, creditResult)
	require.Len(t, creditResult.After, 1)

	afterCredit := creditResult.After[0]
	postCreditVersion := postReleaseVersion + 1
	postCreditAvailable := postReleaseAvailable.Add(decimal.NewFromInt(100)) // Available restored
	postCreditOnHold := postReleaseOnHold                                    // OnHold unchanged

	assert.Equal(t, postCreditVersion, afterCredit.Version,
		"Phase 1b: version should be %d after CREDIT", postCreditVersion)
	assert.True(t, postCreditAvailable.Equal(afterCredit.Available),
		"Phase 1b: available should be restored to %s", postCreditAvailable.String())

	t.Logf("Phase 1: final state -- version=%d, available=%s, onHold=%s",
		afterCredit.Version, afterCredit.Available.String(), afterCredit.OnHold.String())

	balanceKey := pkgTransaction.AliasKey(alias, constant.DefaultBalanceKey)
	internalKey := utils.BalanceInternalKey(orgID, ledgerID, balanceKey)

	// Now set up for a second PENDING+CANCEL cycle to test chaos during CANCEL
	pending2TxID := uuid.New()
	pending2Ops := buildTestBalanceOps(orgID, ledgerID, alias,
		postCreditAvailable, postCreditOnHold, postCreditVersion, true)

	pending2Result, err := infra.proxyRepo.ProcessBalanceAtomicOperation(
		ctx, orgID, ledgerID, pending2TxID, constant.PENDING, true, pending2Ops,
	)
	require.NoError(t, err, "Phase 1: second PENDING should succeed")
	require.NotNil(t, pending2Result)

	preChaosAvailable := postCreditAvailable.Sub(decimal.NewFromInt(100))
	preChaosOnHold := postCreditOnHold.Add(decimal.NewFromInt(100))
	preChaosVersion := postCreditVersion + 2

	// --- Phase 2: Inject ---
	t.Log("Phase 2 (Inject): disabling Toxiproxy proxy to simulate Redis outage")

	err = infra.proxy.Disconnect()
	require.NoError(t, err, "Phase 2: Toxiproxy Disconnect should not fail")

	// --- Phase 3: Verify ---
	t.Log("Phase 3 (Verify): CANCELED operations must return errors during outage")

	// Try RELEASE during outage
	chaosReleaseTxID := uuid.New()
	chaosReleaseOps := buildCanceledBalanceOps(orgID, ledgerID, alias,
		preChaosAvailable, preChaosOnHold, preChaosVersion,
		libConstants.RELEASE, true)

	var releaseOutageErr error

	require.NotPanics(t, func() {
		_, releaseOutageErr = infra.proxyRepo.ProcessBalanceAtomicOperation(
			ctx, orgID, ledgerID, chaosReleaseTxID, constant.CANCELED, true, chaosReleaseOps,
		)
	}, "Phase 3: RELEASE must not panic during outage")

	assert.Error(t, releaseOutageErr,
		"Phase 3: RELEASE must return error during outage")

	// Try CREDIT during outage
	chaosCreditTxID := uuid.New()
	chaosCreditOps := buildCanceledBalanceOps(orgID, ledgerID, alias,
		preChaosAvailable, preChaosOnHold, preChaosVersion,
		libConstants.CREDIT, true)

	var creditOutageErr error

	require.NotPanics(t, func() {
		_, creditOutageErr = infra.proxyRepo.ProcessBalanceAtomicOperation(
			ctx, orgID, ledgerID, chaosCreditTxID, constant.CANCELED, true, chaosCreditOps,
		)
	}, "Phase 3: CREDIT must not panic during outage")

	assert.Error(t, creditOutageErr,
		"Phase 3: CREDIT must return error during outage")

	t.Logf("Phase 3: RELEASE error: %v", releaseOutageErr)
	t.Logf("Phase 3: CREDIT error: %v", creditOutageErr)

	// Verify balance unchanged
	verifyRedisBalance(t, infra, internalKey, preChaosAvailable, preChaosOnHold, preChaosVersion)
	t.Log("Phase 3: confirmed balance unchanged in Redis after outage")

	// --- Phase 4: Restore ---
	t.Log("Phase 4 (Restore): re-enabling Toxiproxy proxy")

	err = infra.proxy.Reconnect()
	require.NoError(t, err, "Phase 4: Toxiproxy Reconnect should not fail")

	// --- Phase 5: Recovery ---
	t.Log("Phase 5 (Recovery): verifying full CANCELED double-entry succeeds after recovery")

	// RELEASE+CANCELED after recovery
	recoveryReleaseTxID := uuid.New()
	recoveryReleaseOps := buildCanceledBalanceOps(orgID, ledgerID, alias,
		preChaosAvailable, preChaosOnHold, preChaosVersion,
		libConstants.RELEASE, true)

	var recoveryReleaseResult *mmodel.BalanceAtomicResult

	chaos.AssertRecoveryWithin(t, func() error {
		var err error
		recoveryReleaseResult, err = infra.proxyRepo.ProcessBalanceAtomicOperation(
			ctx, orgID, ledgerID, recoveryReleaseTxID, constant.CANCELED, true, recoveryReleaseOps,
		)

		return err
	}, 10*time.Second, "Phase 5: RELEASE should succeed after recovery")

	require.NotNil(t, recoveryReleaseResult)
	require.Len(t, recoveryReleaseResult.After, 1)

	recoveryReleaseAfter := recoveryReleaseResult.After[0]
	postRecoveryReleaseVersion := preChaosVersion + 1
	postRecoveryReleaseAvailable := preChaosAvailable
	postRecoveryReleaseOnHold := preChaosOnHold.Sub(decimal.NewFromInt(100))

	assert.Equal(t, postRecoveryReleaseVersion, recoveryReleaseAfter.Version,
		"Phase 5: RELEASE version should be %d", postRecoveryReleaseVersion)

	// CREDIT+CANCELED after recovery
	recoveryCreditTxID := uuid.New()
	recoveryCreditOps := buildCanceledBalanceOps(orgID, ledgerID, alias,
		postRecoveryReleaseAvailable, postRecoveryReleaseOnHold, postRecoveryReleaseVersion,
		libConstants.CREDIT, true)

	var recoveryCreditResult *mmodel.BalanceAtomicResult

	chaos.AssertRecoveryWithin(t, func() error {
		var err error
		recoveryCreditResult, err = infra.proxyRepo.ProcessBalanceAtomicOperation(
			ctx, orgID, ledgerID, recoveryCreditTxID, constant.CANCELED, true, recoveryCreditOps,
		)

		return err
	}, 10*time.Second, "Phase 5: CREDIT should succeed after recovery")

	require.NotNil(t, recoveryCreditResult)
	require.Len(t, recoveryCreditResult.After, 1)

	recoveryCreditAfter := recoveryCreditResult.After[0]
	finalVersion := postRecoveryReleaseVersion + 1
	finalAvailable := postRecoveryReleaseAvailable.Add(decimal.NewFromInt(100))
	finalOnHold := postRecoveryReleaseOnHold

	assert.Equal(t, finalVersion, recoveryCreditAfter.Version,
		"Phase 5: final version should be %d", finalVersion)
	assert.True(t, finalAvailable.Equal(recoveryCreditAfter.Available),
		"Phase 5: final available should be %s", finalAvailable.String())
	assert.True(t, finalOnHold.Equal(recoveryCreditAfter.OnHold),
		"Phase 5: final onHold should be %s", finalOnHold.String())

	// Final integrity check
	verifyRedisBalance(t, infra, internalKey, finalAvailable, finalOnHold, finalVersion)

	t.Logf("Phase 5: final state -- version=%d, available=%s, onHold=%s",
		recoveryCreditAfter.Version, recoveryCreditAfter.Available.String(), recoveryCreditAfter.OnHold.String())

	t.Log("PASS: CANCELED double-entry (RELEASE+CREDIT) recovers correctly after Redis outage, version chain preserved")
}
