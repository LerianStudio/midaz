// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

//go:build unit

package redis_test

import (
	"context"
	"testing"

	tmValkey "github.com/LerianStudio/lib-commons/v5/commons/tenant-manager/valkey"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestWorkerRedis_KeyPrefixing_NoTenantContext verifies that GetKeyContext
// returns the key unchanged when no tenant ID is set in context (single-tenant mode).
func TestWorkerRedis_KeyPrefixing_NoTenantContext(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	originalKey := "some:cache:key"

	result, err := tmValkey.GetKeyContext(ctx, originalKey)
	require.NoError(t, err)

	assert.Equal(t, originalKey, result,
		"GetKeyContext must return the key unchanged when no tenant is in context")
}

// TestWorkerRedis_KeyPrefixing_WithTenant verifies that GetKey adds a tenant prefix
// when a tenant ID is provided (multi-tenant mode).
func TestWorkerRedis_KeyPrefixing_WithTenant(t *testing.T) {
	t.Parallel()

	tenantID := "org_01ABC"
	key := "session:data"

	result, err := tmValkey.GetKey(tenantID, key)
	require.NoError(t, err)

	expected := "tenant:" + tenantID + ":" + key
	assert.Equal(t, expected, result,
		"GetKey must prefix key with tenant:{tenantID}: in multi-tenant mode")
}

// TestWorkerRedis_KeyPrefixing_NilContext verifies that GetKeyContext
// handles a nil context gracefully (returns key unchanged).
func TestWorkerRedis_KeyPrefixing_NilContext(t *testing.T) {
	t.Parallel()

	originalKey := "some:key"
	result, err := tmValkey.GetKeyContext(nil, originalKey)
	require.NoError(t, err)

	assert.Equal(t, originalKey, result,
		"GetKeyContext must return key unchanged for nil context")
}
