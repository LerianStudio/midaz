//go:build integration

// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

// Package redis provides integration tests for the Redis consumer adapter.
//
// Integration tests exercise real Redis/Valkey behaviour: they start a Valkey
// container via testcontainers and verify that namespace-prefixed keys are
// stored and retrieved correctly.
//
// Run with:
//
//	go test -tags integration -v -run TestIntegration_RedisNamespacing ./components/onboarding/internal/adapters/redis/...
package redis

import (
	"context"
	"testing"
	"time"

	libRedis "github.com/LerianStudio/lib-commons/v3/commons/redis"
	tmcore "github.com/LerianStudio/lib-commons/v3/commons/tenant-manager/core"
	redistestutil "github.com/LerianStudio/midaz/v3/tests/utils/redis"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// =============================================================================
// TEST INFRASTRUCTURE
// =============================================================================

// integrationTestInfra holds the resources needed for a Redis integration test.
type integrationTestInfra struct {
	redisContainer *redistestutil.ContainerResult
	repo           *RedisConsumerRepository
}

// setupOnboardingRedisIntegrationInfra starts a Valkey container and builds a
// RedisConsumerRepository connected to it. All cleanup (client close + container
// termination) is registered via t.Cleanup().
func setupOnboardingRedisIntegrationInfra(t *testing.T) *integrationTestInfra {
	t.Helper()

	// Start a Valkey container (valkey/valkey:8 image from DefaultContainerConfig).
	redisContainer := redistestutil.SetupContainer(t)

	// Build lib-commons RedisConnection from the mapped address.
	conn := redistestutil.CreateConnection(t, redisContainer.Addr)

	repo := &RedisConsumerRepository{
		conn: conn,
	}

	return &integrationTestInfra{
		redisContainer: redisContainer,
		repo:           repo,
	}
}

// =============================================================================
// IS-1: SET / GET WITH TENANT — PREFIX IS APPLIED
// =============================================================================

// TestIntegration_RedisNamespacing_SetGetWithTenant verifies that when a tenant
// ID is present in the context, Set stores the value under the tenant-prefixed
// key (tenant:<id>:<key>) in Redis and Get retrieves it correctly.
//
// The test uses the raw redis.Client to inspect actual stored keys so the
// assertion is independent of the repository's own Get path.
func TestIntegration_RedisNamespacing_SetGetWithTenant(t *testing.T) {
	infra := setupOnboardingRedisIntegrationInfra(t)

	tenantID := "int-tenant-" + uuid.New().String()
	ctx := tmcore.SetTenantIDInContext(context.Background(), tenantID)

	logicalKey := "session:token:" + uuid.New().String()
	value := "integration-value-" + uuid.New().String()
	ttl := 5 * time.Minute

	expectedRawKey := "tenant:" + tenantID + ":" + logicalKey

	// --- Set ---
	t.Logf("IS-1: Setting key=%q value=%q with tenant=%q", logicalKey, value, tenantID)

	setCtx, setCancel := context.WithTimeout(ctx, 10*time.Second)
	defer setCancel()

	err := infra.repo.Set(setCtx, logicalKey, value, ttl)
	require.NoError(t, err, "Set must succeed with a valid tenant context")

	// --- Verify raw key in Redis ---
	// Read directly via the raw client to confirm the stored key carries the
	// tenant prefix. This is independent of the repository's Get path.
	t.Logf("IS-1: Verifying raw key=%q is present in Redis", expectedRawKey)

	rawCtx, rawCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer rawCancel()

	rawVal, rawErr := infra.redisContainer.Client.Get(rawCtx, expectedRawKey).Result()
	require.NoError(t, rawErr,
		"IS-1: tenant-prefixed key %q must exist in Redis", expectedRawKey)
	assert.Equal(t, value, rawVal,
		"IS-1: value stored under prefixed key must match what was Set")

	// The bare (un-prefixed) key must NOT exist.
	bareCtx, bareCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer bareCancel()

	bareVal, bareErr := infra.redisContainer.Client.Get(bareCtx, logicalKey).Result()
	assert.Error(t, bareErr, "IS-1: bare key must NOT be stored in Redis when tenant is set")
	assert.Empty(t, bareVal, "IS-1: bare key must have no value")

	// --- Get via repository ---
	t.Log("IS-1: Retrieving value via repository Get")

	getCtx, getCancel := context.WithTimeout(ctx, 5*time.Second)
	defer getCancel()

	retrieved, getErr := infra.repo.Get(getCtx, logicalKey)
	require.NoError(t, getErr, "IS-1: Get must succeed after Set with tenant context")
	assert.Equal(t, value, retrieved,
		"IS-1: Get must return the value stored by Set")

	t.Log("IS-1 PASS: Set stores under tenant-prefixed key; Get retrieves the correct value")
}

// =============================================================================
// IS-2: SET / GET WITHOUT TENANT — NO PREFIX APPLIED
// =============================================================================

// TestIntegration_RedisNamespacing_SetGetWithoutTenant verifies that when no
// tenant ID is present in the context the logical key is used unchanged in Redis
// — i.e. no prefix is added and the key is backwards-compatible.
func TestIntegration_RedisNamespacing_SetGetWithoutTenant(t *testing.T) {
	infra := setupOnboardingRedisIntegrationInfra(t)

	// Plain context — no tenant ID.
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	logicalKey := "session:no-tenant:" + uuid.New().String()
	value := "no-tenant-value-" + uuid.New().String()
	ttl := 5 * time.Minute

	// --- Set ---
	t.Logf("IS-2: Setting key=%q without tenant context", logicalKey)

	err := infra.repo.Set(ctx, logicalKey, value, ttl)
	require.NoError(t, err, "IS-2: Set must succeed without a tenant context")

	// --- Verify raw key in Redis (bare key, no prefix) ---
	t.Logf("IS-2: Verifying bare key=%q is stored in Redis without any prefix", logicalKey)

	rawCtx, rawCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer rawCancel()

	rawVal, rawErr := infra.redisContainer.Client.Get(rawCtx, logicalKey).Result()
	require.NoError(t, rawErr,
		"IS-2: bare key %q must exist in Redis when no tenant is set", logicalKey)
	assert.Equal(t, value, rawVal,
		"IS-2: value stored under bare key must match what was Set")

	// --- Get via repository ---
	t.Log("IS-2: Retrieving value via repository Get")

	getCtx, getCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer getCancel()

	retrieved, getErr := infra.repo.Get(getCtx, logicalKey)
	require.NoError(t, getErr, "IS-2: Get must succeed after Set without tenant context")
	assert.Equal(t, value, retrieved,
		"IS-2: Get must return the value stored by Set when no tenant is present")

	t.Log("IS-2 PASS: Set/Get without tenant context use bare key (no prefix); backwards-compatible")
}

// =============================================================================
// IS-3: TWO TENANTS — NO COLLISION ON SAME LOGICAL KEY
// =============================================================================

// TestIntegration_RedisNamespacing_TwoTenantsNoCollision verifies that two
// different tenant IDs setting the same logical key are stored at completely
// different Redis keys, so reads by one tenant never return data written by
// the other tenant.
func TestIntegration_RedisNamespacing_TwoTenantsNoCollision(t *testing.T) {
	infra := setupOnboardingRedisIntegrationInfra(t)

	tenantA := "tenant-a-" + uuid.New().String()
	tenantB := "tenant-b-" + uuid.New().String()

	ctxA := tmcore.SetTenantIDInContext(context.Background(), tenantA)
	ctxB := tmcore.SetTenantIDInContext(context.Background(), tenantB)

	// Both tenants use the exact same logical key.
	sharedLogicalKey := "shared:session:token"

	valueA := "value-for-tenant-a-" + uuid.New().String()
	valueB := "value-for-tenant-b-" + uuid.New().String()

	ttl := 5 * time.Minute

	// --- Set for tenant A ---
	t.Logf("IS-3: Tenant A (%s) sets key=%q", tenantA, sharedLogicalKey)

	setCtxA, setCancelA := context.WithTimeout(ctxA, 10*time.Second)
	defer setCancelA()

	err := infra.repo.Set(setCtxA, sharedLogicalKey, valueA, ttl)
	require.NoError(t, err, "IS-3: Set for tenant A must succeed")

	// --- Set for tenant B ---
	t.Logf("IS-3: Tenant B (%s) sets key=%q", tenantB, sharedLogicalKey)

	setCtxB, setCancelB := context.WithTimeout(ctxB, 10*time.Second)
	defer setCancelB()

	err = infra.repo.Set(setCtxB, sharedLogicalKey, valueB, ttl)
	require.NoError(t, err, "IS-3: Set for tenant B must succeed")

	// --- Verify raw Redis keys are distinct ---
	rawKeyA := "tenant:" + tenantA + ":" + sharedLogicalKey
	rawKeyB := "tenant:" + tenantB + ":" + sharedLogicalKey

	t.Logf("IS-3: Verifying raw keys are distinct: %q vs %q", rawKeyA, rawKeyB)

	rawCtx, rawCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer rawCancel()

	rawA, rawErrA := infra.redisContainer.Client.Get(rawCtx, rawKeyA).Result()
	require.NoError(t, rawErrA, "IS-3: raw key for tenant A must exist")
	assert.Equal(t, valueA, rawA, "IS-3: raw key for tenant A must hold tenant A's value")

	rawB, rawErrB := infra.redisContainer.Client.Get(rawCtx, rawKeyB).Result()
	require.NoError(t, rawErrB, "IS-3: raw key for tenant B must exist")
	assert.Equal(t, valueB, rawB, "IS-3: raw key for tenant B must hold tenant B's value")

	// Confirm values are different (no collision).
	assert.NotEqual(t, rawA, rawB,
		"IS-3: tenant A and tenant B values must not collide")

	// --- Get via repository: each tenant sees only its own value ---
	t.Log("IS-3: Verifying each tenant retrieves its own value via repository Get")

	getCtxA, getCancelA := context.WithTimeout(ctxA, 5*time.Second)
	defer getCancelA()

	retrievedA, errA := infra.repo.Get(getCtxA, sharedLogicalKey)
	require.NoError(t, errA, "IS-3: Get for tenant A must succeed")
	assert.Equal(t, valueA, retrievedA,
		"IS-3: tenant A must retrieve its own value, not tenant B's")

	getCtxB, getCancelB := context.WithTimeout(ctxB, 5*time.Second)
	defer getCancelB()

	retrievedB, errB := infra.repo.Get(getCtxB, sharedLogicalKey)
	require.NoError(t, errB, "IS-3: Get for tenant B must succeed")
	assert.Equal(t, valueB, retrievedB,
		"IS-3: tenant B must retrieve its own value, not tenant A's")

	// Cross-check: tenant A's get must not return tenant B's value.
	assert.NotEqual(t, retrievedA, retrievedB,
		"IS-3: tenants must not see each other's values — isolation is broken")

	t.Log("IS-3 PASS: Two tenants sharing the same logical key have fully isolated Redis keys")
}

// =============================================================================
// IS-4: DEL WITH TENANT — ONLY PREFIXED KEY IS REMOVED
// =============================================================================

// TestIntegration_RedisNamespacing_DelWithTenant verifies that Del with a
// tenant context removes the tenant-prefixed key and does not affect any other
// key (e.g., another tenant's key at the same logical path).
func TestIntegration_RedisNamespacing_DelWithTenant(t *testing.T) {
	infra := setupOnboardingRedisIntegrationInfra(t)

	tenantA := "del-tenant-a-" + uuid.New().String()
	tenantB := "del-tenant-b-" + uuid.New().String()

	ctxA := tmcore.SetTenantIDInContext(context.Background(), tenantA)
	ctxB := tmcore.SetTenantIDInContext(context.Background(), tenantB)

	logicalKey := "profile:settings:" + uuid.New().String()
	valueA := "settings-a-" + uuid.New().String()
	valueB := "settings-b-" + uuid.New().String()
	ttl := 5 * time.Minute

	// Pre-populate both tenants.
	setCtxA, setCancelA := context.WithTimeout(ctxA, 10*time.Second)
	defer setCancelA()

	require.NoError(t, infra.repo.Set(setCtxA, logicalKey, valueA, ttl), "IS-4: Set for A")

	setCtxB, setCancelB := context.WithTimeout(ctxB, 10*time.Second)
	defer setCancelB()

	require.NoError(t, infra.repo.Set(setCtxB, logicalKey, valueB, ttl), "IS-4: Set for B")

	// Del only tenant A's key.
	t.Logf("IS-4: Deleting key=%q for tenant A only", logicalKey)

	delCtx, delCancel := context.WithTimeout(ctxA, 5*time.Second)
	defer delCancel()

	err := infra.repo.Del(delCtx, logicalKey)
	require.NoError(t, err, "IS-4: Del for tenant A must succeed")

	// Tenant A's key must be gone.
	rawCtx, rawCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer rawCancel()

	rawKeyA := "tenant:" + tenantA + ":" + logicalKey

	deletedVal, deletedErr := infra.redisContainer.Client.Get(rawCtx, rawKeyA).Result()
	assert.Error(t, deletedErr, "IS-4: tenant A's key must no longer exist after Del")
	assert.Empty(t, deletedVal)

	// Tenant B's key must be untouched.
	rawKeyB := "tenant:" + tenantB + ":" + logicalKey

	rawCtx2, rawCancel2 := context.WithTimeout(context.Background(), 5*time.Second)
	defer rawCancel2()

	survivingVal, survivingErr := infra.redisContainer.Client.Get(rawCtx2, rawKeyB).Result()
	require.NoError(t, survivingErr, "IS-4: tenant B's key must still exist after tenant A's Del")
	assert.Equal(t, valueB, survivingVal, "IS-4: tenant B's value must be unchanged")

	t.Log("IS-4 PASS: Del removes only the tenant-prefixed key; sibling tenants are unaffected")
}

// =============================================================================
// IS-5: TTL IS RESPECTED — KEY EXPIRES
// =============================================================================

// TestIntegration_RedisNamespacing_TTLIsRespected verifies that a key stored
// with a short TTL via Set actually expires in Redis, even when the key is
// tenant-namespaced.
func TestIntegration_RedisNamespacing_TTLIsRespected(t *testing.T) {
	infra := setupOnboardingRedisIntegrationInfra(t)

	tenantID := "ttl-tenant-" + uuid.New().String()
	ctx := tmcore.SetTenantIDInContext(context.Background(), tenantID)

	logicalKey := "ephemeral:key:" + uuid.New().String()
	value := "ephemeral-value"
	shortTTL := 1 * time.Second

	// Set with a 1-second TTL.
	setCtx, setCancel := context.WithTimeout(ctx, 10*time.Second)
	defer setCancel()

	err := infra.repo.Set(setCtx, logicalKey, value, shortTTL)
	require.NoError(t, err, "IS-5: Set with short TTL must succeed")

	// Immediately confirm the key exists.
	rawKey := "tenant:" + tenantID + ":" + logicalKey

	rawCtxBefore, rawCancelBefore := context.WithTimeout(context.Background(), 5*time.Second)
	defer rawCancelBefore()

	rawVal, rawErr := infra.redisContainer.Client.Get(rawCtxBefore, rawKey).Result()
	require.NoError(t, rawErr, "IS-5: key must exist immediately after Set")
	assert.Equal(t, value, rawVal, "IS-5: value must match before TTL expires")

	// Wait for the TTL to elapse (slightly longer to account for clock skew).
	time.Sleep(shortTTL + 500*time.Millisecond)

	// Key must now be gone.
	rawCtxAfter, rawCancelAfter := context.WithTimeout(context.Background(), 5*time.Second)
	defer rawCancelAfter()

	expiredVal, expiredErr := infra.redisContainer.Client.Get(rawCtxAfter, rawKey).Result()
	assert.Error(t, expiredErr, "IS-5: tenant-prefixed key must have expired after TTL")
	assert.Empty(t, expiredVal, "IS-5: expired key must have no value")

	// Get via repository must return cache-miss (empty string, no error).
	getCtx, getCancel := context.WithTimeout(ctx, 5*time.Second)
	defer getCancel()

	retrieved, getErr := infra.repo.Get(getCtx, logicalKey)
	require.NoError(t, getErr, "IS-5: Get on expired key must return no error (cache-miss)")
	assert.Empty(t, retrieved, "IS-5: Get on expired key must return empty string (cache-miss)")

	t.Log("IS-5 PASS: Tenant-prefixed key expires correctly after the specified TTL")
}

// =============================================================================
// HELPER: build a RedisConsumerRepository directly from a connection address
// (kept here to avoid duplicating logic in chaos tests)
// =============================================================================

// buildOnboardingRepo creates a RedisConsumerRepository connected to the given
// Redis address using a lib-commons RedisConnection.
func buildOnboardingRepo(t *testing.T, addr string) *RedisConsumerRepository {
	t.Helper()

	conn := redistestutil.CreateConnection(t, addr)

	return &RedisConsumerRepository{conn: conn}
}

// buildOnboardingRepoWithConn creates a RedisConsumerRepository from an existing
// lib-commons RedisConnection (used in chaos tests where the connection is
// pre-configured to route through Toxiproxy).
func buildOnboardingRepoWithConn(conn *libRedis.RedisConnection) *RedisConsumerRepository {
	return &RedisConsumerRepository{conn: conn}
}
