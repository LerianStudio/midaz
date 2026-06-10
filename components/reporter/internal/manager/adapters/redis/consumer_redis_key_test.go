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

// TestRedisKey_WithTenant_HasPrefix verifies that GetKeyContext prefixes a key
// with the tenant namespace when a tenant ID is present in context.
func TestRedisKey_WithTenant_HasPrefix(t *testing.T) {
	t.Parallel()

	ctx := tmCore.ContextWithTenantID(context.Background(), "acme-corp")
	key, err := tmValkey.GetKeyContext(ctx, "report:abc123")
	require.NoError(t, err)

	// lib-commons GetKey format: "tenant:{tenantID}:{key}"
	expected, err := tmValkey.GetKey("acme-corp", "report:abc123")
	require.NoError(t, err)
	assert.Equal(t, expected, key,
		"GetKeyContext must prefix the key with the tenant namespace")
}

// TestRedisKey_WithoutTenant_NoPrefix verifies that GetKeyContext returns the key
// unchanged when no tenant ID is in context, preserving backward compatibility.
func TestRedisKey_WithoutTenant_NoPrefix(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	key, err := tmValkey.GetKeyContext(ctx, "report:abc123")
	require.NoError(t, err)

	assert.Equal(t, "report:abc123", key,
		"GetKeyContext must return the key unchanged when no tenant is in context")
}

func TestRedisKey_WithInvalidTenant_ReturnsError(t *testing.T) {
	t.Parallel()

	ctx := tmCore.ContextWithTenantID(context.Background(), "tenant:bad")
	key, err := tmValkey.GetKeyContext(ctx, "report:abc123")
	require.Error(t, err)
	assert.Empty(t, key)
}
