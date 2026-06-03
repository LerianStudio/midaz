// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package workers

import (
	"context"
	"fmt"

	tmpostgres "github.com/LerianStudio/lib-commons/v5/commons/tenant-manager/postgres"
	"github.com/bxcodec/dbresolver/v2"
)

// WorkerPoolResolver returns a tenant-scoped Postgres pool for per-tenant
// background workers. It wraps the lib-commons tmpostgres.Manager with the
// narrow surface area workers need — one method, a single concern.
//
// Why the indirection: workers must resolve the pool PER-CYCLE, not once at
// spawn time. The pool Manager enforces an LRU eviction policy, so a pool
// reference captured at spawn time may dangle after an eviction+reconnect.
// Resolving each cycle also exercises the Manager's credential-rotation
// health check, keeping tenant workers in sync with Tenant Manager updates.
//
// In single-tenant mode this interface is unused — the worker's pool resolver
// is nil and the worker skips the ContextWithPG injection, falling back to
// the static pool that repositories reach via their own conn.GetDB(ctx).
type WorkerPoolResolver interface {
	// GetTenantDB returns the dbresolver.DB for the given tenantID. The
	// caller is expected to stash the result into the request/cycle context
	// via tmcore.ContextWithPG so downstream repositories' GetPGContext
	// lookups succeed.
	//
	// Returns a non-nil error if the pool cannot be resolved (tenant manager
	// unreachable, DSN invalid, credentials rotated, etc.). The caller MUST
	// treat this as a cycle-level failure and skip the cycle rather than
	// falling through to the static default pool — otherwise a tenant-scoped
	// cycle could silently land its work in the root database (the exact bug
	// this resolver exists to prevent).
	GetTenantDB(ctx context.Context, tenantID string) (dbresolver.DB, error)
}

// pgManagerPoolResolver adapts a *tmpostgres.Manager to the WorkerPoolResolver
// interface. The adapter is thin: GetConnection returns a *PostgresConnection
// whose GetDB() method returns the same dbresolver.DB that TenantMiddleware
// stashes on the request context for HTTP handlers. Background workers use
// the same pool resolution path, so a successfully resolved cycle runs with
// identical tenant-scoping semantics as a live request.
type pgManagerPoolResolver struct {
	mgr *tmpostgres.Manager
}

// NewPoolResolver wraps a tmpostgres.Manager as a WorkerPoolResolver.
// Returns an error if mgr is nil so bootstrap wiring can fail fast instead of
// producing a resolver that deadlocks on first use.
func NewPoolResolver(mgr *tmpostgres.Manager) (WorkerPoolResolver, error) {
	if mgr == nil {
		return nil, fmt.Errorf("pool resolver: pg manager is required")
	}

	return &pgManagerPoolResolver{mgr: mgr}, nil
}

// GetTenantDB delegates to tmpostgres.Manager.GetConnection and extracts the
// dbresolver.DB. Errors from either step propagate verbatim — workers handle
// them as "skip this cycle and try again".
func (p *pgManagerPoolResolver) GetTenantDB(ctx context.Context, tenantID string) (dbresolver.DB, error) {
	if p == nil || p.mgr == nil {
		return nil, fmt.Errorf("pool resolver: uninitialized manager")
	}

	conn, err := p.mgr.GetConnection(ctx, tenantID)
	if err != nil {
		return nil, fmt.Errorf("pool resolver: get connection for tenant %q: %w", tenantID, err)
	}

	db, err := conn.GetDB()
	if err != nil {
		return nil, fmt.Errorf("pool resolver: get db for tenant %q: %w", tenantID, err)
	}

	return db, nil
}
