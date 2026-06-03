// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package db

import (
	"context"
	"database/sql"
	"errors"
	"sync/atomic"

	libPostgres "github.com/LerianStudio/lib-commons/v5/commons/postgres"
	tmcore "github.com/LerianStudio/lib-commons/v5/commons/tenant-manager/core"
	"github.com/bxcodec/dbresolver/v2"
)

// ErrNilConnection is returned when attempting to use a nil database connection.
var ErrNilConnection = errors.New("database connection is nil")

// ErrNoTenantInContext is returned when the connection adapter is in strict
// multi-tenant mode (SetMultiTenantEnabled(true)) and the request context does
// not carry a tenant-scoped *sql.DB via tmcore.ContextWithPG. This surfaces
// the bug explicitly instead of silently reaching into the root pool, which
// would reopen CRITICAL-A style cross-tenant data leaks (H11 + M1).
var ErrNoTenantInContext = errors.New("postgres connection: no tenant pool in context (multi-tenant mode requires ContextWithPG)")

// PostgresConnectionAdapter adapts *libPostgres.Client to Connection interface.
// This allows repositories to use a common interface for database connections,
// enabling easier testing with mocks while maintaining compatibility with production connections.
//
// The adapter is tenant-aware: in multi-tenant mode (SetMultiTenantEnabled(true))
// it resolves a per-tenant pool from tmcore.GetPGContext(ctx) and refuses to
// fall back to the static root pool when one is missing. In single-tenant mode
// the fallback is permitted and transparent.
type PostgresConnectionAdapter struct {
	conn *libPostgres.Client
	// multiTenantEnabled is read atomically so the boot-time setter is safe to
	// race with per-request GetDB reads (SetMultiTenantEnabled runs exactly
	// once during InitServers, then the adapter is treated as immutable).
	multiTenantEnabled atomic.Bool
}

// NewPostgresConnectionAdapter creates a new adapter for a postgres Client.
// Returns nil if conn is nil. Callers should check for nil before use.
func NewPostgresConnectionAdapter(conn *libPostgres.Client) *PostgresConnectionAdapter {
	if conn == nil {
		return nil
	}

	return &PostgresConnectionAdapter{conn: conn}
}

// SetMultiTenantEnabled toggles strict MT mode for this adapter. When true,
// GetDB refuses to fall back to the root pool when the request context does
// not carry a tenant-scoped *sql.DB — any caller that reaches the adapter
// without going through TenantMiddleware receives an explicit error instead
// of a silent cross-tenant read/write (H11 + M1).
//
// This setter is called exactly once at boot from InitServers; the adapter is
// then treated as immutable for the lifetime of the process.
func (p *PostgresConnectionAdapter) SetMultiTenantEnabled(enabled bool) {
	if p == nil {
		return
	}

	p.multiTenantEnabled.Store(enabled)
}

// GetDB returns the underlying database connection using the provided context.
//
// Pool resolution order:
//  1. If the context carries a tenant-scoped pool (tmcore.GetPGContext), use
//     it. This is the multi-tenant request path set up by TenantMiddleware.
//  2. Otherwise, if strict MT mode is enabled, return ErrNoTenantInContext —
//     the caller bypassed the tenant middleware in multi-tenant mode, which
//     would otherwise silently leak across tenants (H11).
//  3. Otherwise fall back to the static root pool bound at construction time.
//     This is the single-tenant code path and any background caller that has
//     not threaded a request context through.
//
// The context is propagated to the connection resolver, enabling deadline,
// cancellation, and trace correlation through the connection lifecycle.
// Returns ErrNilConnection if the adapter was created with a nil connection.
func (p *PostgresConnectionAdapter) GetDB(ctx context.Context) (DB, error) {
	if p == nil {
		return nil, ErrNilConnection
	}

	// Tenant-pool short-circuit runs BEFORE the nil-conn check: in multi-tenant
	// mode the static root client may even be nil (the supervisor + middleware
	// own all DB resolution per-tenant). Returning the context-scoped pool
	// here keeps single-tenant and multi-tenant paths uniform.
	if db := tmcore.GetPGContext(ctx); db != nil {
		return db, nil
	}

	if p.multiTenantEnabled.Load() {
		return nil, ErrNoTenantInContext
	}

	if p.conn == nil {
		return nil, ErrNilConnection
	}

	return p.conn.Resolver(ctx)
}

// TxBeginnerAdapter adapts dbresolver.DB to our TxBeginner interface.
// This is necessary because dbresolver.DB.BeginTx returns dbresolver.Tx,
// while our TxBeginner interface expects Tx (our interface).
// Both interfaces are structurally compatible, but Go requires explicit adaptation.
//
// The adapter is context-aware: BeginTx prefers a per-tenant pool resolved from
// tmcore.GetPGContext(ctx) when one is present (multi-tenant mode), and falls
// back to the static pool bound at construction time otherwise (single-tenant
// mode). This guarantees that ValidationService's transactional writes
// (usage_counters, transaction_validations, audit_events) land in the correct
// tenant database even though the adapter itself is wired once at boot.
//
// In strict multi-tenant mode (SetMultiTenantEnabled(true)) BeginTx refuses to
// fall back to the root pool when the context carries no tenant — mirroring
// PostgresConnectionAdapter.GetDB so a code path that forgets ContextWithPG
// fails closed instead of silently opening a tx on the default pool (C5).
type TxBeginnerAdapter struct {
	db dbresolver.DB
	// multiTenantEnabled is read atomically so the boot-time setter is safe to
	// race with per-request BeginTx reads (SetMultiTenantEnabled runs exactly
	// once during InitServers, then the adapter is treated as immutable).
	multiTenantEnabled atomic.Bool
}

// NewTxBeginnerAdapter creates a new TxBeginnerAdapter.
// Returns nil if db is nil. Callers should check for nil before use.
func NewTxBeginnerAdapter(db dbresolver.DB) *TxBeginnerAdapter {
	if db == nil {
		return nil
	}

	return &TxBeginnerAdapter{db: db}
}

// SetMultiTenantEnabled toggles strict MT mode for this adapter. When true,
// BeginTx refuses to fall back to the root pool when the request context does
// not carry a tenant-scoped *sql.DB — any caller that reaches the adapter
// without going through TenantMiddleware receives an explicit error instead
// of a silent cross-tenant write. This mirrors PostgresConnectionAdapter and
// closes the C5 defense-in-depth gap flagged in the Gate 4 multi-tenant review.
//
// This setter is called exactly once at boot from InitServers; the adapter is
// then treated as immutable for the lifetime of the process.
func (t *TxBeginnerAdapter) SetMultiTenantEnabled(enabled bool) {
	if t == nil {
		return
	}

	t.multiTenantEnabled.Store(enabled)
}

// BeginTx starts a new database transaction.
//
// Pool resolution order:
//  1. If the context carries a tenant-scoped pool (tmcore.GetPGContext), use
//     it. This is the multi-tenant request path set up by TenantMiddleware.
//  2. Otherwise, if strict MT mode is enabled, return ErrNoTenantInContext —
//     the caller bypassed the tenant middleware in multi-tenant mode, which
//     would otherwise silently open a tx on the root pool (C5, mirrors H11
//     behavior in PostgresConnectionAdapter.GetDB).
//  3. Otherwise fall back to the static pool bound at construction time. This
//     is the single-tenant code path and also any background caller that has
//     not threaded a request context through.
//
// The returned Tx is a wrapper around dbresolver.Tx that satisfies our Tx
// interface.
func (t *TxBeginnerAdapter) BeginTx(ctx context.Context, opts *sql.TxOptions) (Tx, error) {
	if t == nil || t.db == nil {
		return nil, ErrNilConnection
	}

	db := t.db
	if tenantDB := tmcore.GetPGContext(ctx); tenantDB != nil {
		db = tenantDB
	} else if t.multiTenantEnabled.Load() {
		return nil, ErrNoTenantInContext
	}

	tx, err := db.BeginTx(ctx, opts)
	if err != nil {
		return nil, err
	}

	// dbresolver.Tx satisfies our Tx interface (structurally compatible)
	return tx, nil
}

// Compile-time interface satisfaction checks.
var _ TxBeginner = (*TxBeginnerAdapter)(nil)
