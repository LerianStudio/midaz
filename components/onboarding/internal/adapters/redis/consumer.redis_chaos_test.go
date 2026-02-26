//go:build integration

// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

// Package redis provides chaos tests for the Redis consumer adapter in onboarding.
//
// Chaos tests exercise fault-tolerance of Redis namespacing operations when
// the underlying Valkey/Redis connection experiences failures. They are a
// subset of integration tests and run only when CHAOS=1 is set.
//
// Run with:
//
//	CHAOS=1 go test -tags integration -v -run TestIntegration_Chaos_RedisNamespacing ./components/onboarding/internal/adapters/redis/...
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

// onboardingChaosInfra holds all resources for a network-chaos test scenario
// in the onboarding component. It wraps chaos.Infrastructure (Toxiproxy +
// Docker network) and a RedisConsumerRepository connected through the Toxiproxy
// proxy so faults can be injected/removed without restarting any container.
type onboardingChaosInfra struct {
	redisContainer *redistestutil.ContainerResult
	chaosInfra     *chaos.Infrastructure
	proxyRepo      *RedisConsumerRepository
	proxyConn      *libRedis.RedisConnection
	proxy          *chaos.Proxy
}

// setupOnboardingRedisChaosNetworkInfra creates the full chaos test infrastructure
// for the onboarding Redis adapter:
//  1. Creates a Docker network + Toxiproxy container via chaos.Infrastructure
//  2. Starts a Valkey container on the host network
//  3. Registers the Redis container with Infrastructure to get UpstreamAddr
//  4. Creates a Toxiproxy proxy that forwards through to Redis
//  5. Builds a RedisConsumerRepository connected through the proxy address
//
// All cleanup (proxy deletion, container termination, network removal) is
// registered with t.Cleanup() automatically.
func setupOnboardingRedisChaosNetworkInfra(t *testing.T) *onboardingChaosInfra {
	t.Helper()

	// 1. Create chaos infrastructure (Docker network + Toxiproxy).
	chaosInfra := chaos.NewInfrastructure(t)

	// 2. Start Valkey container on the host network.
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
		conn: proxyConn,
	}

	return &onboardingChaosInfra{
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
//  5. Recovery — Set succeeds again; namespaced key is present in Redis
func TestIntegration_Chaos_RedisNamespacing_ConnectionLossOnSet(t *testing.T) {
	if os.Getenv("CHAOS") != "1" {
		t.Skip("Set CHAOS=1 to run chaos tests")
	}

	if testing.Short() {
		t.Skip("Skipping chaos test in short mode")
	}

	infra := setupOnboardingRedisChaosNetworkInfra(t)

	tenantID := "chaos-set-tenant-" + uuid.New().String()
	ctx := tmcore.SetTenantIDInContext(context.Background(), tenantID)
	key := "chaos-set-key:" + uuid.New().String()
	value := "chaos-set-value-" + uuid.New().String()
	ttl := 5 * time.Minute

	// --- Phase 1: Normal ---
	// Verify Set works through the proxy before any fault is injected.
	t.Log("Phase 1 (Normal): verifying Set succeeds through proxy")

	normalCtx, normalCancel := context.WithTimeout(ctx, 10*time.Second)
	defer normalCancel()

	err := infra.proxyRepo.Set(normalCtx, key, value, ttl)
	require.NoError(t, err, "Phase 1: Set should succeed through proxy before fault injection")

	t.Log("Phase 1 PASS: Set succeeded through Toxiproxy")

	// --- Phase 2: Inject ---
	// Disable the Toxiproxy proxy to simulate a full connection loss.
	t.Log("Phase 2 (Inject): disabling Toxiproxy proxy to simulate connection loss")

	err = infra.proxy.Disconnect()
	require.NoError(t, err, "Phase 2: Toxiproxy Disconnect should not fail")

	t.Log("Phase 2: Toxiproxy proxy disabled — Redis is now unreachable")

	// --- Phase 3: Verify ---
	// Set must return an error. The implementation must not panic.
	t.Log("Phase 3 (Verify): Set must return error, not panic, when connection is lost")

	faultCtx, faultCancel := context.WithTimeout(ctx, 5*time.Second)
	defer faultCancel()

	var setFaultErr error

	require.NotPanics(t, func() {
		setFaultErr = infra.proxyRepo.Set(faultCtx, key+":fault", value, ttl)
	}, "Phase 3: Set must not panic on connection loss")

	assert.Error(t, setFaultErr,
		"Phase 3: Set must return an error when Redis connection is dropped")

	t.Logf("Phase 3 PASS: received expected error: %v", setFaultErr)

	// --- Phase 4: Restore ---
	// Re-enable the proxy to restore connectivity.
	t.Log("Phase 4 (Restore): re-enabling Toxiproxy proxy")

	err = infra.proxy.Reconnect()
	require.NoError(t, err, "Phase 4: Toxiproxy Reconnect should not fail")

	t.Log("Phase 4: Toxiproxy proxy re-enabled — Redis is reachable again")

	// --- Phase 5: Recovery ---
	// After the proxy is restored, Set must succeed and the key must be stored
	// under the tenant-prefixed path.
	t.Log("Phase 5 (Recovery): verifying Set succeeds and key is namespaced after proxy restoration")

	recoveryKey := "chaos-set-recovery:" + uuid.New().String()
	recoveryValue := "chaos-recovery-value-" + uuid.New().String()
	expectedNamespacedKey := "tenant:" + tenantID + ":" + recoveryKey

	chaos.AssertRecoveryWithin(t, func() error {
		recCtx, recCancel := context.WithTimeout(ctx, 5*time.Second)
		defer recCancel()

		return infra.proxyRepo.Set(recCtx, recoveryKey, recoveryValue, ttl)
	}, 15*time.Second, "Phase 5: Set should succeed after proxy is restored")

	// Confirm the recovered key is stored under the tenant namespace in raw Redis.
	rawCtx, rawCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer rawCancel()

	rawVal, rawErr := infra.redisContainer.Client.Get(rawCtx, expectedNamespacedKey).Result()
	require.NoError(t, rawErr,
		"Phase 5: namespaced recovery key must exist in Redis after restore")
	assert.Equal(t, recoveryValue, rawVal,
		"Phase 5: value stored under namespaced key must match after recovery")

	t.Log("CS-1 PASS: Set with tenant context returns error on connection loss, recovers with correct namespace")
}

// =============================================================================
// CS-2: FULL CYCLE — RECOVERY AFTER RECONNECT
// =============================================================================

// TestIntegration_Chaos_RedisNamespacing_RecoveryAfterReconnect verifies the
// full fault-and-recovery cycle for both Set and Get: after Redis recovers from
// an outage the namespaced operations continue to work correctly, producing the
// same key/value semantics as before the fault.
//
// 5-Phase structure:
//  1. Normal   — Set then Get succeed; raw Redis confirms key is prefixed
//  2. Inject   — Toxiproxy proxy disabled (Redis unreachable)
//  3. Verify   — Both Set and Get return errors; no panics
//  4. Restore  — Toxiproxy proxy re-enabled
//  5. Recovery — New Set then Get succeed; raw Redis confirms namespace is intact
func TestIntegration_Chaos_RedisNamespacing_RecoveryAfterReconnect(t *testing.T) {
	if os.Getenv("CHAOS") != "1" {
		t.Skip("Set CHAOS=1 to run chaos tests")
	}

	if testing.Short() {
		t.Skip("Skipping chaos test in short mode")
	}

	infra := setupOnboardingRedisChaosNetworkInfra(t)

	tenantID := "chaos-recovery-tenant-" + uuid.New().String()
	ctx := tmcore.SetTenantIDInContext(context.Background(), tenantID)
	key := "chaos-recovery-key:" + uuid.New().String()
	value := "chaos-recovery-value-" + uuid.New().String()
	ttl := 5 * time.Minute

	expectedNamespacedKey := "tenant:" + tenantID + ":" + key

	// --- Phase 1: Normal ---
	// Set and Get work; the raw Redis client confirms the key is namespaced.
	t.Log("Phase 1 (Normal): verifying Set/Get work and key is namespaced in Redis")

	setCtx, setCancel := context.WithTimeout(ctx, 10*time.Second)
	defer setCancel()

	err := infra.proxyRepo.Set(setCtx, key, value, ttl)
	require.NoError(t, err, "Phase 1: Set should succeed through proxy")

	getCtx, getCancel := context.WithTimeout(ctx, 5*time.Second)
	defer getCancel()

	retrieved, err := infra.proxyRepo.Get(getCtx, key)
	require.NoError(t, err, "Phase 1: Get should succeed through proxy")
	assert.Equal(t, value, retrieved, "Phase 1: retrieved value must equal stored value")

	// Use the raw client (direct connection, not proxied) to confirm the key is
	// stored under the tenant namespace.
	rawCtx, rawCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer rawCancel()

	rawStored, rawErr := infra.redisContainer.Client.Get(rawCtx, expectedNamespacedKey).Result()
	require.NoError(t, rawErr, "Phase 1: namespaced key should exist in raw Redis")
	assert.Equal(t, value, rawStored, "Phase 1: value stored under namespaced key must match")

	// Confirm the bare (un-namespaced) key does NOT exist.
	bareCtx, bareCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer bareCancel()

	bareVal, bareErr := infra.redisContainer.Client.Get(bareCtx, key).Result()
	assert.Error(t, bareErr, "Phase 1: bare key must NOT be set in Redis")
	assert.Empty(t, bareVal, "Phase 1: bare key must have no value")

	t.Log("Phase 1 PASS: Set/Get work correctly; key is namespaced in raw Redis")

	// --- Phase 2: Inject ---
	// Drop the connection via Toxiproxy.
	t.Log("Phase 2 (Inject): disabling Toxiproxy proxy to simulate Redis outage")

	err = infra.proxy.Disconnect()
	require.NoError(t, err, "Phase 2: Toxiproxy Disconnect should not fail")

	t.Log("Phase 2: Toxiproxy proxy disabled — Redis is unreachable")

	// --- Phase 3: Verify ---
	// Both Set and Get must return errors without panicking.
	t.Log("Phase 3 (Verify): Set and Get must return errors, not panic, during outage")

	outageKey := "chaos-recovery-outage:" + uuid.New().String()

	faultCtx, faultCancel := context.WithTimeout(ctx, 5*time.Second)
	defer faultCancel()

	var setOutageErr error

	require.NotPanics(t, func() {
		setOutageErr = infra.proxyRepo.Set(faultCtx, outageKey, value, ttl)
	}, "Phase 3: Set must not panic during outage")

	faultGetCtx, faultGetCancel := context.WithTimeout(ctx, 5*time.Second)
	defer faultGetCancel()

	var getOutageErr error

	require.NotPanics(t, func() {
		_, getOutageErr = infra.proxyRepo.Get(faultGetCtx, key)
	}, "Phase 3: Get must not panic during outage")

	assert.Error(t, setOutageErr,
		"Phase 3: Set must return an error when Redis is unavailable")
	assert.Error(t, getOutageErr,
		"Phase 3: Get must return an error when Redis is unavailable")

	t.Logf("Phase 3 PASS: Set error=%v  Get error=%v", setOutageErr, getOutageErr)

	// --- Phase 4: Restore ---
	// Re-enable the proxy.
	t.Log("Phase 4 (Restore): re-enabling Toxiproxy proxy")

	err = infra.proxy.Reconnect()
	require.NoError(t, err, "Phase 4: Toxiproxy Reconnect should not fail")

	t.Log("Phase 4: Toxiproxy proxy re-enabled — Redis is reachable again")

	// --- Phase 5: Recovery ---
	// A new Set/Get cycle with a different key must succeed and the stored key
	// must again carry the tenant namespace — confirming the namespacing logic
	// was not corrupted by the fault/recovery cycle.
	t.Log("Phase 5 (Recovery): verifying Set/Get succeed with correct namespacing after recovery")

	recoveryKey := "chaos-recovery-post:" + uuid.New().String()
	recoveryValue := "chaos-recovery-post-value-" + uuid.New().String()
	expectedRecoveryNamespacedKey := "tenant:" + tenantID + ":" + recoveryKey

	chaos.AssertRecoveryWithin(t, func() error {
		recCtx, recCancel := context.WithTimeout(ctx, 5*time.Second)
		defer recCancel()

		return infra.proxyRepo.Set(recCtx, recoveryKey, recoveryValue, ttl)
	}, 15*time.Second, "Phase 5: Set should succeed after proxy is restored")

	// Get via repository must return the correct value.
	var recoveredVal string

	chaos.AssertRecoveryWithin(t, func() error {
		recGetCtx, recGetCancel := context.WithTimeout(ctx, 5*time.Second)
		defer recGetCancel()

		var err error
		recoveredVal, err = infra.proxyRepo.Get(recGetCtx, recoveryKey)

		return err
	}, 10*time.Second, "Phase 5: Get should succeed after proxy is restored")

	assert.Equal(t, recoveryValue, recoveredVal,
		"Phase 5: recovered value must equal the value written after reconnect")

	// Inspect raw Redis to confirm the namespace is still applied correctly.
	rawRecCtx, rawRecCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer rawRecCancel()

	rawRecovery, rawRecoveryErr := infra.redisContainer.Client.Get(
		rawRecCtx, expectedRecoveryNamespacedKey,
	).Result()
	require.NoError(t, rawRecoveryErr,
		"Phase 5: namespaced key should exist in raw Redis after recovery")
	assert.Equal(t, recoveryValue, rawRecovery,
		"Phase 5: value stored under namespaced key after recovery must match")

	// Cross-check: the bare key must still not exist after recovery.
	barePosCtx, barePostCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer barePostCancel()

	barePost, barePostErr := infra.redisContainer.Client.Get(barePosCtx, recoveryKey).Result()
	assert.Error(t, barePostErr, "Phase 5: bare key must NOT exist in Redis after recovery")
	assert.Empty(t, barePost, "Phase 5: bare key must have no value after recovery")

	t.Log("CS-2 PASS: namespaced Set/Get work correctly after Redis recovers from outage")
}
