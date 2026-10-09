// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

// Package tenantpool provides a dbresolver.DB that re-resolves a tenant's
// PostgreSQL pool from the tenant manager on every context-bearing call.
//
// In multi-tenant mode lib-commons pins the tenant's concrete pool into the
// request context once, at request entry. The tenant manager may close that
// pool mid-request (configuration change, failed health check, suspension)
// without draining, which would leave the remaining repository calls of the
// request on a closed pool. The adapter returned by New replaces that pin: it
// asks the tenant manager for the current pool per operation and falls back to
// the pinned pool when resolution fails, so a resolution failure yields exactly
// the outcome the operation would have had before.
package tenantpool

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"time"

	libObservability "github.com/LerianStudio/lib-observability/v4"
	obsconst "github.com/LerianStudio/lib-observability/v4/constants"
	libLog "github.com/LerianStudio/lib-observability/v4/log"
	"github.com/bxcodec/dbresolver/v2"
)

// poolSwapLogMessage is the constant message of the warn emitted when an
// operation observes a pool different from the one pinned at request entry.
const poolSwapLogMessage = "Tenant PostgreSQL pool changed during request; using the current pool"

// logFieldModule is the log field carrying the PostgreSQL module name.
const logFieldModule = "module"

// Resolver resolves a tenant's current PostgreSQL pool. Satisfied by
// *tmpostgres.Manager.
type Resolver interface {
	GetDB(ctx context.Context, tenantID string) (dbresolver.DB, error)
}

// lazyDB is a dbresolver.DB that delegates each context-bearing call to the
// tenant's current pool. The pool is shared and owned by the tenant manager,
// so the adapter never closes or reconfigures it.
type lazyDB struct {
	resolver Resolver
	tenantID string
	module   string
	pinned   dbresolver.DB
}

// Compile-time guarantee that lazyDB keeps up with the dbresolver.DB contract.
var _ dbresolver.DB = (*lazyDB)(nil)

// New returns a dbresolver.DB that resolves tenantID's pool for module through
// resolver on every context-bearing call, falling back to pinned (the pool
// lib-commons stored in the request context) when resolution fails.
func New(resolver Resolver, tenantID, module string, pinned dbresolver.DB) dbresolver.DB {
	return &lazyDB{
		resolver: resolver,
		tenantID: tenantID,
		module:   module,
		pinned:   pinned,
	}
}

// current returns the pool a context-bearing call must run on. A done context
// returns its error together with the pinned pool, so callers without an error
// channel (QueryRowContext) still have a pool whose deferred error surfaces on
// Scan. When the resolved pool differs from the pinned one, a single warn is
// logged for the operation.
func (d *lazyDB) current(ctx context.Context) (dbresolver.DB, error) {
	if err := ctx.Err(); err != nil {
		return d.pinned, err
	}

	db := d.resolve(ctx)

	// Interface identity: the tenant manager mints a new pointer-backed
	// dbresolver.DB per connection, so inequality means the pool was swapped.
	if db != d.pinned {
		libObservability.NewLoggerFromContext(ctx).Log(ctx, libLog.LevelWarn, poolSwapLogMessage,
			libLog.String(obsconst.AttrKeyTenantID, d.tenantID),
			libLog.String(logFieldModule, d.module))
	}

	return db, nil
}

// resolve asks the tenant manager for the current pool and falls back to the
// pinned pool on any resolution failure. It never logs.
func (d *lazyDB) resolve(ctx context.Context) dbresolver.DB {
	if d.resolver == nil {
		return d.pinned
	}

	db, err := d.resolver.GetDB(ctx, d.tenantID)
	if err != nil || db == nil {
		return d.pinned
	}

	return db
}

// BeginTx opens the transaction on the current pool. The returned Tx is bound
// to a connection already taken from that pool, so a later pool close does not
// split it across pools.
func (d *lazyDB) BeginTx(ctx context.Context, opts *sql.TxOptions) (dbresolver.Tx, error) {
	db, err := d.current(ctx)
	if err != nil {
		return nil, err
	}

	return db.BeginTx(ctx, opts)
}

// ExecContext runs the statement on the current pool.
func (d *lazyDB) ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error) {
	db, err := d.current(ctx)
	if err != nil {
		return nil, err
	}

	return db.ExecContext(ctx, query, args...)
}

// QueryContext runs the query on the current pool.
func (d *lazyDB) QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error) {
	db, err := d.current(ctx)
	if err != nil {
		return nil, err
	}

	return db.QueryContext(ctx, query, args...)
}

// QueryRowContext runs the query on the current pool. It has no error channel:
// on a done context it delegates to the pinned pool, whose Row defers the
// context error to Scan exactly as before.
func (d *lazyDB) QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row {
	db, _ := d.current(ctx)

	return db.QueryRowContext(ctx, query, args...)
}

// PrepareContext prepares the statement on the current pool.
func (d *lazyDB) PrepareContext(ctx context.Context, query string) (dbresolver.Stmt, error) {
	db, err := d.current(ctx)
	if err != nil {
		return nil, err
	}

	return db.PrepareContext(ctx, query)
}

// PingContext pings the current pool.
func (d *lazyDB) PingContext(ctx context.Context) error {
	db, err := d.current(ctx)
	if err != nil {
		return err
	}

	return db.PingContext(ctx)
}

// Conn takes a dedicated connection from the current pool.
func (d *lazyDB) Conn(ctx context.Context) (dbresolver.Conn, error) {
	db, err := d.current(ctx)
	if err != nil {
		return nil, err
	}

	return db.Conn(ctx)
}

// Begin delegates to the pinned pool: context-less calls carry no request
// context to resolve with, and no repository uses them.
func (d *lazyDB) Begin() (dbresolver.Tx, error) {
	return d.pinned.Begin()
}

// Exec delegates to the pinned pool (see Begin).
func (d *lazyDB) Exec(query string, args ...any) (sql.Result, error) {
	return d.pinned.Exec(query, args...)
}

// Query delegates to the pinned pool (see Begin).
func (d *lazyDB) Query(query string, args ...any) (*sql.Rows, error) {
	return d.pinned.Query(query, args...)
}

// QueryRow delegates to the pinned pool (see Begin).
func (d *lazyDB) QueryRow(query string, args ...any) *sql.Row {
	return d.pinned.QueryRow(query, args...)
}

// Prepare delegates to the pinned pool (see Begin).
func (d *lazyDB) Prepare(query string) (dbresolver.Stmt, error) {
	return d.pinned.Prepare(query)
}

// Ping delegates to the pinned pool (see Begin).
func (d *lazyDB) Ping() error {
	return d.pinned.Ping()
}

// Close is a no-op: the pool is shared and owned by the tenant manager.
func (d *lazyDB) Close() error {
	return nil
}

// SetConnMaxIdleTime is a no-op: the tenant manager configures the pool.
func (d *lazyDB) SetConnMaxIdleTime(_ time.Duration) {}

// SetConnMaxLifetime is a no-op: the tenant manager configures the pool.
func (d *lazyDB) SetConnMaxLifetime(_ time.Duration) {}

// SetMaxIdleConns is a no-op: the tenant manager configures the pool.
func (d *lazyDB) SetMaxIdleConns(_ int) {}

// SetMaxOpenConns is a no-op: the tenant manager configures the pool.
func (d *lazyDB) SetMaxOpenConns(_ int) {}

// Driver reports the current pool's driver. Diagnostic read: no swap warn.
func (d *lazyDB) Driver() driver.Driver {
	return d.resolve(context.Background()).Driver()
}

// Stats reports the current pool's statistics. Diagnostic read: no swap warn.
func (d *lazyDB) Stats() sql.DBStats {
	return d.resolve(context.Background()).Stats()
}

// PrimaryDBs reports the current pool's primaries. Diagnostic read: no swap warn.
func (d *lazyDB) PrimaryDBs() []*sql.DB {
	return d.resolve(context.Background()).PrimaryDBs()
}

// ReplicaDBs reports the current pool's replicas. Diagnostic read: no swap warn.
func (d *lazyDB) ReplicaDBs() []*sql.DB {
	return d.resolve(context.Background()).ReplicaDBs()
}
