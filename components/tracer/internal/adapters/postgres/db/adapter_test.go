// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package db

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	tmcore "github.com/LerianStudio/lib-commons/v5/commons/tenant-manager/core"
	"github.com/bxcodec/dbresolver/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// stubTx is a minimal dbresolver.Tx implementation for testing.
type stubTx struct {
	dbresolver.Tx
}

// stubDB is a test double for dbresolver.DB that controls BeginTx behavior.
type stubDB struct {
	dbresolver.DB
	tx           dbresolver.Tx
	err          error
	receivedOpts *sql.TxOptions
	beginCalls   int
}

func (s *stubDB) BeginTx(_ context.Context, opts *sql.TxOptions) (dbresolver.Tx, error) {
	s.receivedOpts = opts
	s.beginCalls++

	return s.tx, s.err
}

func TestNewTxBeginnerAdapter_NilDB(t *testing.T) {
	adapter := NewTxBeginnerAdapter(nil)
	assert.Nil(t, adapter)
}

func TestNewTxBeginnerAdapter_ValidDB(t *testing.T) {
	adapter := NewTxBeginnerAdapter(&stubDB{})
	require.NotNil(t, adapter)
}

func TestTxBeginnerAdapter_BeginTx_NilAdapter(t *testing.T) {
	var adapter *TxBeginnerAdapter

	tx, err := adapter.BeginTx(context.Background(), nil)

	assert.Nil(t, tx)
	assert.ErrorIs(t, err, ErrNilConnection)
}

func TestTxBeginnerAdapter_BeginTx_NilDB(t *testing.T) {
	adapter := &TxBeginnerAdapter{db: nil}

	tx, err := adapter.BeginTx(context.Background(), nil)

	assert.Nil(t, tx)
	assert.ErrorIs(t, err, ErrNilConnection)
}

func TestTxBeginnerAdapter_BeginTx_ErrorPropagation(t *testing.T) {
	dbErr := errors.New("connection refused")
	adapter := NewTxBeginnerAdapter(&stubDB{err: dbErr})

	tx, err := adapter.BeginTx(context.Background(), nil)

	assert.Nil(t, tx)
	assert.ErrorIs(t, err, dbErr)
}

func TestTxBeginnerAdapter_BeginTx_Success(t *testing.T) {
	expectedTx := &stubTx{}
	adapter := NewTxBeginnerAdapter(&stubDB{tx: expectedTx})

	tx, err := adapter.BeginTx(context.Background(), nil)

	require.NoError(t, err)
	assert.Equal(t, expectedTx, tx)
}

func TestTxBeginnerAdapter_BeginTx_ForwardsOptions(t *testing.T) {
	stub := &stubDB{tx: &stubTx{}}
	adapter := NewTxBeginnerAdapter(stub)

	opts := &sql.TxOptions{Isolation: sql.LevelSerializable, ReadOnly: true}
	tx, err := adapter.BeginTx(context.Background(), opts)

	require.NoError(t, err)
	require.NotNil(t, tx)
	assert.Equal(t, opts, stub.receivedOpts)
}

// TestTxBeginnerAdapter_UsesTenantPoolWhenContextHasOne verifies that the
// adapter resolves the per-tenant PostgreSQL pool from the context when one is
// present (multi-tenant mode), NOT the static pool bound at boot. This is the
// critical isolation fix: ValidationService opens transactions via this
// adapter, and in MT mode those writes MUST land in the tenant's DB, not the
// default DB.
func TestTxBeginnerAdapter_UsesTenantPoolWhenContextHasOne(t *testing.T) {
	staticTx := &stubTx{}
	tenantTx := &stubTx{}

	staticDB := &stubDB{tx: staticTx}
	tenantDB := &stubDB{tx: tenantTx}

	adapter := NewTxBeginnerAdapter(staticDB)
	require.NotNil(t, adapter)

	// Inject the tenant pool into the context the way lib-commons v4
	// TenantMiddleware does on every request.
	ctx := tmcore.ContextWithPG(context.Background(), tenantDB)

	tx, err := adapter.BeginTx(ctx, nil)
	require.NoError(t, err)
	require.NotNil(t, tx)

	// Critical assertions: tenant pool was used, static pool was not touched.
	assert.Equal(t, tenantTx, tx, "BeginTx must return a Tx from the tenant pool")
	assert.Equal(t, 1, tenantDB.beginCalls, "tenant pool MUST receive exactly one BeginTx call")
	assert.Equal(t, 0, staticDB.beginCalls, "static pool MUST NOT be touched when ctx carries a tenant pool")
}

// TestTxBeginnerAdapter_FallsBackToDefaultWhenNoTenantInContext verifies that
// when no tenant pool is in the context (single-tenant mode, or any call-site
// that hasn't been through TenantMiddleware), the adapter uses the static pool
// it was constructed with. This preserves single-tenant behavior.
func TestTxBeginnerAdapter_FallsBackToDefaultWhenNoTenantInContext(t *testing.T) {
	staticTx := &stubTx{}
	staticDB := &stubDB{tx: staticTx}

	adapter := NewTxBeginnerAdapter(staticDB)
	require.NotNil(t, adapter)

	// Plain context — no tmcore.ContextWithPG call. This is the single-tenant
	// code path and also any background goroutine that hasn't threaded a
	// request context.
	tx, err := adapter.BeginTx(context.Background(), nil)
	require.NoError(t, err)
	require.NotNil(t, tx)

	assert.Equal(t, staticTx, tx, "BeginTx must return a Tx from the static pool")
	assert.Equal(t, 1, staticDB.beginCalls, "static pool MUST receive the BeginTx call in single-tenant mode")
}

// TestTxBeginnerAdapter_TenantPoolErrorPropagates verifies error handling for
// the tenant-pool branch. A failure to begin a tenant transaction must surface
// to the caller so ValidationService can abort cleanly rather than silently
// falling back to the default pool (which would corrupt isolation).
func TestTxBeginnerAdapter_TenantPoolErrorPropagates(t *testing.T) {
	tenantErr := errors.New("tenant pool: connection refused")
	staticDB := &stubDB{tx: &stubTx{}}
	tenantDB := &stubDB{err: tenantErr}

	adapter := NewTxBeginnerAdapter(staticDB)
	ctx := tmcore.ContextWithPG(context.Background(), tenantDB)

	tx, err := adapter.BeginTx(ctx, nil)

	assert.Nil(t, tx)
	assert.ErrorIs(t, err, tenantErr)
	assert.Equal(t, 0, staticDB.beginCalls, "adapter MUST NOT fall back to static pool on tenant pool error")
}

// C5: TxBeginnerAdapter must mirror PostgresConnectionAdapter's strict-MT
// semantics. In strict MT mode a missing ContextWithPG MUST fail closed
// instead of silently opening a tx on the root pool. These tests cover the
// three scenarios called out in the Gate 4 fix: strict MT without tenant ctx
// (must error), strict MT with tenant ctx (happy path), and strict MT
// disabled (legacy fallback preserved).

// TestTxBeginnerAdapter_StrictMTRefusesRootFallback verifies that strict MT
// mode surfaces ErrNoTenantInContext when no tenant pool is in the context.
// The static (root) pool MUST NOT be touched — fail closed semantics mirror
// PostgresConnectionAdapter.GetDB.
func TestTxBeginnerAdapter_StrictMTRefusesRootFallback(t *testing.T) {
	staticDB := &stubDB{tx: &stubTx{}}

	adapter := NewTxBeginnerAdapter(staticDB)
	require.NotNil(t, adapter)
	adapter.SetMultiTenantEnabled(true)

	tx, err := adapter.BeginTx(context.Background(), nil)

	assert.Nil(t, tx)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrNoTenantInContext,
		"strict MT mode must surface ErrNoTenantInContext so operators can trace the cause")
	assert.Equal(t, 0, staticDB.beginCalls,
		"static pool MUST NOT be touched in strict MT mode without tenant context")
}

// TestTxBeginnerAdapter_StrictMTTenantPoolPresent verifies that strict MT mode
// does NOT break the happy path: when the context carries a tenant pool, the
// adapter resolves it and opens the tx on the tenant pool.
func TestTxBeginnerAdapter_StrictMTTenantPoolPresent(t *testing.T) {
	staticDB := &stubDB{tx: &stubTx{}}
	tenantTx := &stubTx{}
	tenantDB := &stubDB{tx: tenantTx}

	adapter := NewTxBeginnerAdapter(staticDB)
	require.NotNil(t, adapter)
	adapter.SetMultiTenantEnabled(true)

	ctx := tmcore.ContextWithPG(context.Background(), tenantDB)

	tx, err := adapter.BeginTx(ctx, nil)
	require.NoError(t, err, "strict MT with tenant pool present must succeed")
	require.NotNil(t, tx)

	assert.Equal(t, tenantTx, tx, "BeginTx must return a Tx from the tenant pool")
	assert.Equal(t, 1, tenantDB.beginCalls, "tenant pool MUST receive exactly one BeginTx call")
	assert.Equal(t, 0, staticDB.beginCalls, "static pool MUST NOT be touched when tenant ctx present")
}

// TestTxBeginnerAdapter_StrictMTDisabledFallsBackToRoot verifies that when MT
// is disabled (single-tenant mode) and no tenant pool is in the context, the
// adapter still falls back to the static pool. This preserves the existing
// single-tenant behavior — the strict-MT branch only fires when the operator
// has explicitly enabled it.
func TestTxBeginnerAdapter_StrictMTDisabledFallsBackToRoot(t *testing.T) {
	staticTx := &stubTx{}
	staticDB := &stubDB{tx: staticTx}

	adapter := NewTxBeginnerAdapter(staticDB)
	require.NotNil(t, adapter)
	// Explicit: single-tenant mode (default, but we set it so the test reads
	// the same as the strict-MT case above).
	adapter.SetMultiTenantEnabled(false)

	tx, err := adapter.BeginTx(context.Background(), nil)
	require.NoError(t, err, "single-tenant mode must allow root fallback")
	require.NotNil(t, tx)

	assert.Equal(t, staticTx, tx)
	assert.Equal(t, 1, staticDB.beginCalls,
		"static pool MUST receive the BeginTx call in single-tenant mode")
}

// TestTxBeginnerAdapter_SetMultiTenantEnabled_NilSafe verifies that the setter
// is a no-op on a nil adapter (defensive; the setter runs exactly once at boot
// and the adapter is nil only in misconfigured code paths).
func TestTxBeginnerAdapter_SetMultiTenantEnabled_NilSafe(t *testing.T) {
	var adapter *TxBeginnerAdapter

	// Must not panic.
	adapter.SetMultiTenantEnabled(true)
}
