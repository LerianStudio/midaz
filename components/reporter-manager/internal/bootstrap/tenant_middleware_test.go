// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

//go:build unit

package bootstrap

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestInitTenantMiddleware_ReturnsNil_WhenDisabled(t *testing.T) {
	t.Parallel()
	// When MultiTenantEnabled=false, initTenantMiddleware must return nil
	// (no middleware registered, single-tenant passthrough)
	result, err := initTenantMiddlewareForTest(false, "", "")
	require.NoError(t, err)
	assert.Nil(t, result, "must return nil when multi-tenant is disabled")
}

func TestInitTenantMiddleware_ReturnsNil_WhenNoAddress(t *testing.T) {
	t.Parallel()
	result, err := initTenantMiddlewareForTest(true, "", "")
	require.NoError(t, err)
	assert.Nil(t, result, "must return nil when MultiTenantURL is empty even if enabled")
}

func TestInitTenantMiddleware_ReturnsNonNil_WhenEnabledWithAddress(t *testing.T) {
	t.Parallel()
	result, err := initTenantMiddlewareForTest(true, "development", "http://tenant-manager:8080")
	require.NoError(t, err)
	assert.NotNil(t, result, "must return a handler when multi-tenant is enabled with an address")
}

func TestInitTenantMiddleware_ReturnsError_WhenProductionUsesHTTP(t *testing.T) {
	t.Parallel()

	result, err := initTenantMiddlewareForTest(true, "production", "http://tenant-manager:8080")
	require.Error(t, err)
	assert.Nil(t, result)
}

// TestTenantBypassPaths_IncludesReadyz verifies that the canonical /readyz
// path is in the tenant bypass list so /readyz never carries a tenant JWT.
func TestTenantBypassPaths_IncludesReadyz(t *testing.T) {
	t.Parallel()

	found := false

	for _, p := range tenantBypassPaths {
		if p == "/readyz" {
			found = true
			break
		}
	}

	assert.True(t, found, "/readyz must appear in tenantBypassPaths")
}

// TestTenantBypassPaths_IncludesHealth verifies that /health remains in the
// tenant bypass list (regression guard).
func TestTenantBypassPaths_IncludesHealth(t *testing.T) {
	t.Parallel()

	found := false

	for _, p := range tenantBypassPaths {
		if p == "/health" {
			found = true
			break
		}
	}

	assert.True(t, found, "/health must appear in tenantBypassPaths")
}
