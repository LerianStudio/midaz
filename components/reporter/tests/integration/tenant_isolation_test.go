//go:build integration

// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

// Package integration holds integration tests for the reporter service.
package integration

import (
	"context"
	"os"
	"testing"

	tmcore "github.com/LerianStudio/lib-commons/v5/commons/tenant-manager/core"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestTenantIsolation_MongoDB_TwoTenants verifies that two tenants operating against
// the same reporter service work with independent contexts. Tenant A's context must
// not contain tenant B's identity and vice versa.
//
// This test runs in integration mode only. When INTEGRATION_TEST is not set it
// is skipped to allow it to live in the test suite without blocking CI unit runs.
//
// When extended with a real MongoDB testcontainer (testInfra), this test becomes the
// canonical gate-8 isolation proof: create data under tenant A, confirm tenant B
// cannot see it via the repository layer.
func TestTenantIsolation_MongoDB_TwoTenants(t *testing.T) {
	// Skip when not explicitly running integration tests.
	if os.Getenv("INTEGRATION_TEST") == "" {
		t.Skip("skipping: INTEGRATION_TEST not set")
	}

	// Create two separate tenant contexts.
	ctx1 := tmcore.ContextWithTenantID(context.Background(), "tenant-A")
	ctx2 := tmcore.ContextWithTenantID(context.Background(), "tenant-B")

	// Contexts must be distinct — a shallow copy or context key collision would cause
	// both tenants to share state, breaking isolation.
	assert.NotEqual(t, ctx1, ctx2, "tenant contexts must be distinct")

	// Each context must carry only its own tenant ID.
	tenantA := tmcore.GetTenantIDContext(ctx1)
	tenantB := tmcore.GetTenantIDContext(ctx2)

	require.Equal(t, "tenant-A", tenantA,
		"context 1 must carry tenant-A identity")
	require.Equal(t, "tenant-B", tenantB,
		"context 2 must carry tenant-B identity")
	assert.NotEqual(t, tenantA, tenantB,
		"tenant A and tenant B must be isolated — different identities, different databases")
}

// TestTenantIsolation_ContextLeak verifies that overwriting a context value for one
// tenant does not affect a previously derived context for another tenant.
// This guards against a future context key refactoring that reuses the same key type.
func TestTenantIsolation_ContextLeak(t *testing.T) {
	if os.Getenv("INTEGRATION_TEST") == "" {
		t.Skip("skipping: INTEGRATION_TEST not set")
	}

	baseCtx := context.Background()

	// Sequentially set different tenant IDs in derived contexts.
	ctxFirst := tmcore.ContextWithTenantID(baseCtx, "first-tenant")
	ctxSecond := tmcore.ContextWithTenantID(baseCtx, "second-tenant")

	firstID := tmcore.GetTenantIDContext(ctxFirst)
	secondID := tmcore.GetTenantIDContext(ctxSecond)

	assert.Equal(t, "first-tenant", firstID,
		"first context must still carry first-tenant after second context was created")
	assert.Equal(t, "second-tenant", secondID,
		"second context must carry second-tenant")
	assert.NotEqual(t, firstID, secondID,
		"contexts derived from the same base context must not share tenant identity")
}

// TestTenantIsolation_NoTenantContext_IsEmpty verifies that a context without any
// tenant ID set returns an empty string, which triggers the single-tenant fallback
// in all repository implementations.
func TestTenantIsolation_NoTenantContext_IsEmpty(t *testing.T) {
	if os.Getenv("INTEGRATION_TEST") == "" {
		t.Skip("skipping: INTEGRATION_TEST not set")
	}

	ctx := context.Background()
	tenantID := tmcore.GetTenantIDContext(ctx)

	assert.Empty(t, tenantID,
		"a plain context.Background() must return empty tenant ID, triggering single-tenant fallback")
}
