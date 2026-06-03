// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

//go:build unit

package redis_test

import (
	"context"
	"testing"

	tmCore "github.com/LerianStudio/lib-commons/v5/commons/tenant-manager/core"
	tmValkey "github.com/LerianStudio/lib-commons/v5/commons/tenant-manager/valkey"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestWorkerRedis_TenantContext_KeyIsPrefixed verifies that when a tenant ID is present
// in context, GetKeyContext returns a key prefixed with the tenant namespace.
// The worker redis repository uses this function on every Set/Get/Del/SetNX call to enforce tenant
// isolation at the key level without requiring separate Redis instances per tenant.
func TestWorkerRedis_TenantContext_KeyIsPrefixed(t *testing.T) {
	t.Parallel()

	tenantID := "tenant123"
	baseKey := "report:status"

	ctx := tmCore.ContextWithTenantID(context.Background(), tenantID)
	result, err := tmValkey.GetKeyContext(ctx, baseKey)
	require.NoError(t, err)

	// The prefix format enforced by lib-commons is "tenant:{tenantID}:{key}".
	expected, err := tmValkey.GetKey(tenantID, baseKey)
	require.NoError(t, err)
	assert.Equal(t, expected, result,
		"GetKeyContext must prefix the key with the tenant namespace when tenant is in context")
	assert.Contains(t, result, tenantID,
		"the resulting key must contain the tenant ID")
	assert.Contains(t, result, baseKey,
		"the resulting key must still contain the original key")
}

// TestWorkerRedis_NoTenantContext_KeyIsUnchanged verifies that when no tenant ID is set
// in context, GetKeyContext returns the key exactly as provided (backward compat).
// This is the single-tenant path: no prefix is added, no panic occurs.
func TestWorkerRedis_NoTenantContext_KeyIsUnchanged(t *testing.T) {
	t.Parallel()

	baseKey := "report:status"
	ctx := context.Background() // no tenant set

	result, err := tmValkey.GetKeyContext(ctx, baseKey)
	require.NoError(t, err)

	assert.Equal(t, baseKey, result,
		"GetKeyContext must return the key unchanged when no tenant is in context (single-tenant mode)")
}

// TestWorkerRedis_DifferentTenants_ProduceDifferentKeys verifies that two tenants with
// the same logical key produce different Redis keys, ensuring tenant isolation.
func TestWorkerRedis_DifferentTenants_ProduceDifferentKeys(t *testing.T) {
	t.Parallel()

	baseKey := "lock:reconcile:abc123"

	ctxA := tmCore.ContextWithTenantID(context.Background(), "tenant-alpha")
	ctxB := tmCore.ContextWithTenantID(context.Background(), "tenant-beta")

	keyA, err := tmValkey.GetKeyContext(ctxA, baseKey)
	require.NoError(t, err)
	keyB, err := tmValkey.GetKeyContext(ctxB, baseKey)
	require.NoError(t, err)

	assert.NotEqual(t, keyA, keyB,
		"the same logical key for different tenants must produce different Redis keys; "+
			"tenant-alpha and tenant-beta must not share cache entries")
}

// TestWorkerRedis_SameTenant_ProducesSameKey verifies that the same tenant ID always
// produces the same key prefix, making Redis lookups deterministic across requests.
func TestWorkerRedis_SameTenant_ProducesSameKey(t *testing.T) {
	t.Parallel()

	tenantID := "deterministic-tenant"
	baseKey := "session:token"

	ctx1 := tmCore.ContextWithTenantID(context.Background(), tenantID)
	ctx2 := tmCore.ContextWithTenantID(context.Background(), tenantID)

	key1, err := tmValkey.GetKeyContext(ctx1, baseKey)
	require.NoError(t, err)
	key2, err := tmValkey.GetKeyContext(ctx2, baseKey)
	require.NoError(t, err)

	assert.Equal(t, key1, key2,
		"the same tenant ID must always produce the same prefixed key for deterministic cache lookups")
}

// TestWorkerRedis_InvalidTenantID_ReturnsError verifies that an invalid tenant ID
// in context causes GetKeyContext to return an error.
func TestWorkerRedis_InvalidTenantID_ReturnsError(t *testing.T) {
	t.Parallel()

	ctx := tmCore.ContextWithTenantID(context.Background(), "tenant:bad")
	key, err := tmValkey.GetKeyContext(ctx, "report:status")
	require.Error(t, err)
	assert.Empty(t, key)
}
