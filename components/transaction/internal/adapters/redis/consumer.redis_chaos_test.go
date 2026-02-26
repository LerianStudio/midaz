//go:build integration

// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

// Package redis provides chaos tests for the Redis consumer adapter.
//
// Chaos tests exercise fault-tolerance of Redis namespacing operations when
// the underlying Valkey/Redis connection experiences failures. They are a
// subset of integration tests and run only when CHAOS=1 is set.
//
// Run with:
//
//	CHAOS=1 go test -tags integration -v -run TestIntegration_Chaos_RedisNamespacing ./components/transaction/internal/adapters/redis/...
package redis

import (
	"context"
	"os"
	"testing"
	"time"

	libRedis "github.com/LerianStudio/lib-commons/v3/commons/redis"
	tmcore "github.com/LerianStudio/lib-commons/v3/commons/tenant-manager/core"
	libZap "github.com/LerianStudio/lib-commons/v3/commons/zap"
	"github.com/LerianStudio/midaz/v3/tests/utils/chaos"
	redistestutil "github.com/LerianStudio/midaz/v3/tests/utils/redis"
	"github.com/google/uuid"
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
	proxyConn      *libRedis.RedisConnection
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
	logger := libZap.InitializeLogger()
	proxyConn := &libRedis.RedisConnection{
		Address: []string{proxyAddr},
		Logger:  logger,
	}

	proxyRepo := &RedisConsumerRepository{
		conn:               proxyConn,
		balanceSyncEnabled: false,
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
// CS-1: CONNECTION LOSS DURING SET
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
	ctx := tmcore.SetTenantIDInContext(context.Background(), tenantID)
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

	t.Log("CS-1 PASS: Set with tenant context returns error on connection loss, recovers correctly")
}

// =============================================================================
// CS-2: HIGH LATENCY ON GET
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
	ctx := tmcore.SetTenantIDInContext(context.Background(), tenantID)
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

	t.Log("CS-2 PASS: Get with tenant context times out gracefully under high latency, recovers correctly")
}

// =============================================================================
// CS-3: CONNECTION LOSS DURING MGET
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
	ctx := tmcore.SetTenantIDInContext(context.Background(), tenantID)

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

	t.Log("CS-3 PASS: MGet with tenant context returns error on connection loss, recovers correctly")
}

// =============================================================================
// CS-4: CONNECTION LOSS DURING QUEUE OPS (AddMessageToQueue)
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
	ctx := tmcore.SetTenantIDInContext(context.Background(), tenantID)

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

	t.Log("CS-4 PASS: AddMessageToQueue returns error on connection loss, tenant prefix does not mask failure")
}

// =============================================================================
// CS-5: RECOVERY AFTER RECONNECT
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
	ctx := tmcore.SetTenantIDInContext(context.Background(), tenantID)
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

	t.Log("CS-5 PASS: namespaced Set/Get work correctly after Redis recovers from outage")
}
