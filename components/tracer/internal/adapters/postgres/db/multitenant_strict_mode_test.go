// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package db

import (
	"context"
	"testing"

	tmcore "github.com/LerianStudio/lib-commons/v5/commons/tenant-manager/core"
	"github.com/bxcodec/dbresolver/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// tenantStubDB is a dbresolver.DB used only to verify the selector short-
// circuit. Only the DB interface surface (ExecContext, QueryContext,
// QueryRowContext) is realized when repositories use it; the adapter's
// selector returns it without invoking any method.
type tenantStubDB struct {
	dbresolver.DB
}

// H11 + M1: The PostgresConnectionAdapter is the single place where tenant
// pool resolution lives. It must:
//   - honor tmcore.GetPGContext(ctx) when a tenant pool is in the context
//   - refuse to fall back to the root pool in strict MT mode
//   - fall back transparently in single-tenant mode
//
// Previously each repository duplicated this logic in a local getDB helper.
// Consolidating into the adapter eliminates drift risk and keeps the
// repositories tenant-unaware.

// TestPostgresConnectionAdapter_GetDB_UsesTenantPoolFromContext verifies that
// when the request context carries a tenant-scoped pool via tmcore.ContextWithPG,
// GetDB returns that pool verbatim. This is the multi-tenant request path set
// up by TenantMiddleware.
func TestPostgresConnectionAdapter_GetDB_UsesTenantPoolFromContext(t *testing.T) {
	t.Parallel()

	tenantDB := &tenantStubDB{}

	// Construct the adapter manually: we don't need a real libPostgres.Client,
	// only the selector logic. The context-short-circuit runs BEFORE p.conn is
	// dereferenced on the happy path, so a nil conn is acceptable here.
	adapter := &PostgresConnectionAdapter{}

	ctx := tmcore.ContextWithPG(context.Background(), tenantDB)

	db, err := adapter.GetDB(ctx)
	require.NoError(t, err, "adapter must return the tenant pool without touching conn")
	assert.Equal(t, DB(tenantDB), db, "tenant pool must be returned verbatim")
}

// TestPostgresConnectionAdapter_GetDB_StrictMTRefusesRootFallback verifies
// that the adapter returns ErrNoTenantInContext when strict MT mode is on and
// no tenant pool is in the context. This replaces per-repo enforcement that
// lived in getDB helpers before M1.
//
// The adapter short-circuits before consulting p.conn, so a nil backing
// client is fine here — the test proves that the strict-MT branch fires
// ahead of the nil-conn fallback.
func TestPostgresConnectionAdapter_GetDB_StrictMTRefusesRootFallback(t *testing.T) {
	t.Parallel()

	adapter := &PostgresConnectionAdapter{}
	adapter.SetMultiTenantEnabled(true)

	db, err := adapter.GetDB(context.Background())
	require.Error(t, err)
	assert.Nil(t, db)
	assert.ErrorIs(t, err, ErrNoTenantInContext,
		"strict MT mode must surface ErrNoTenantInContext so operators can trace the cause")
	assert.Contains(t, err.Error(), "multi-tenant",
		"error message must mention multi-tenant so the cause is obvious in logs")
}

// TestPostgresConnectionAdapter_GetDB_StrictMTTenantPoolPresent verifies that
// strict MT mode does NOT break the happy path: when the context carries a
// tenant pool, the adapter returns it without error.
func TestPostgresConnectionAdapter_GetDB_StrictMTTenantPoolPresent(t *testing.T) {
	t.Parallel()

	tenantDB := &tenantStubDB{}

	adapter := &PostgresConnectionAdapter{}
	adapter.SetMultiTenantEnabled(true)

	ctx := tmcore.ContextWithPG(context.Background(), tenantDB)

	db, err := adapter.GetDB(ctx)
	require.NoError(t, err, "strict MT with tenant pool present must succeed")
	assert.Equal(t, DB(tenantDB), db)
}

// TestPostgresConnectionAdapter_GetDB_NilAdapter verifies defensive nil-checks.
func TestPostgresConnectionAdapter_GetDB_NilAdapter(t *testing.T) {
	t.Parallel()

	var adapter *PostgresConnectionAdapter

	db, err := adapter.GetDB(context.Background())
	assert.Nil(t, db)
	assert.ErrorIs(t, err, ErrNilConnection)
}

// TestPostgresConnectionAdapter_GetDB_NilConn verifies that GetDB returns
// ErrNilConnection when the adapter was constructed with a nil underlying
// client AND the context carries no tenant pool. This is the misconfigured-
// bootstrap failure mode.
func TestPostgresConnectionAdapter_GetDB_NilConn(t *testing.T) {
	t.Parallel()

	adapter := &PostgresConnectionAdapter{conn: nil}

	db, err := adapter.GetDB(context.Background())
	assert.Nil(t, db)
	assert.ErrorIs(t, err, ErrNilConnection)
}

// TestPostgresConnectionAdapter_SetMultiTenantEnabled_NilSafe verifies that
// the setter is a no-op on a nil adapter (defensive; the setter runs exactly
// once at boot and the adapter is nil only in misconfigured code paths).
func TestPostgresConnectionAdapter_SetMultiTenantEnabled_NilSafe(t *testing.T) {
	t.Parallel()

	var adapter *PostgresConnectionAdapter

	// Must not panic.
	adapter.SetMultiTenantEnabled(true)
}

// TestPostgresConnectionAdapter_StrictMTTogglePersists verifies that a
// subsequent toggle to false re-enables fallback semantics. The setter is
// atomic-backed; this test documents the invariant that runtime toggling is
// safe.
func TestPostgresConnectionAdapter_StrictMTTogglePersists(t *testing.T) {
	t.Parallel()

	adapter := &PostgresConnectionAdapter{}

	adapter.SetMultiTenantEnabled(true)
	assert.True(t, adapter.multiTenantEnabled.Load())

	adapter.SetMultiTenantEnabled(false)
	assert.False(t, adapter.multiTenantEnabled.Load())
}
