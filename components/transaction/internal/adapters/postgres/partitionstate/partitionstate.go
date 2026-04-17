// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

// Package partitionstate exposes the partition cutover phase stored in the
// partition_migration_state control table (migration 000021). Repositories
// read the phase via Reader.Phase() to decide whether to dual-write into the
// partitioned shell tables.
//
// Reads are served from an in-memory value that expires after a configurable
// TTL (defaults to 30s) so the hot INSERT path does not hit the database on
// every call. If the DB read fails the cached value is returned with an error
// logged; stale values are strictly better than blocking writes on the state
// machine.
package partitionstate

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sync"
	"time"
)

// Phase represents the partition migration phase.
type Phase string

const (
	// PhaseLegacyOnly means only the legacy (non-partitioned) tables are written.
	PhaseLegacyOnly Phase = "legacy_only"
	// PhaseDualWrite means INSERTs must land in both legacy and partitioned tables.
	PhaseDualWrite Phase = "dual_write"
	// PhasePartitioned means the RENAME swap has happened; only the (now-partitioned)
	// table under the original name is written. Dual-write is no longer needed.
	PhasePartitioned Phase = "partitioned"
)

// DefaultTTL is the default cache lifetime for a phase lookup. Short enough
// that rollouts of a new phase reach every replica within ~1 minute, long
// enough to avoid turning the state table into a hot read path.
const DefaultTTL = 30 * time.Second

// ErrUnknownPhase is returned when the DB contains a phase value not
// recognized by this package.
var ErrUnknownPhase = errors.New("partitionstate: unknown phase")

// logger is the minimal logging surface Reader needs. Avoids coupling to a
// specific logger implementation and keeps the package testable with a stub.
type logger interface {
	Warnf(format string, args ...any)
}

// DB is the minimal database surface Reader needs.
type DB interface {
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

// Reader provides phase lookups with an in-memory TTL cache. It is safe for
// concurrent use.
type Reader struct {
	db     DB
	ttl    time.Duration
	logger logger
	now    func() time.Time

	mu      sync.RWMutex
	phase   Phase
	fetched time.Time
	ok      bool
}

// NewReader constructs a Reader. A zero ttl falls back to DefaultTTL.
// A nil logger is tolerated (warnings are silently dropped).
func NewReader(db DB, ttl time.Duration, log logger) *Reader {
	if ttl <= 0 {
		ttl = DefaultTTL
	}

	return &Reader{
		db:     db,
		ttl:    ttl,
		logger: log,
		now:    time.Now,
	}
}

// Phase returns the current partition phase. If the cached value is still
// fresh it is returned directly. Otherwise the database is consulted.
//
// On a database error a cached (stale) value is returned when available,
// alongside a wrapped error. When no cached value exists, PhaseLegacyOnly is
// returned together with the error so that a failure to read the control
// table does not break writes — it degrades gracefully to legacy-only
// behavior.
func (r *Reader) Phase(ctx context.Context) (Phase, error) {
	r.mu.RLock()

	if r.ok && r.now().Sub(r.fetched) < r.ttl {
		p := r.phase
		r.mu.RUnlock()

		return p, nil
	}

	r.mu.RUnlock()

	return r.refresh(ctx)
}

// Invalidate forces the next Phase call to hit the database. Useful in tests
// and after a known phase transition (e.g. immediately post-migration).
func (r *Reader) Invalidate() {
	r.mu.Lock()
	r.ok = false
	r.mu.Unlock()
}

func (r *Reader) refresh(ctx context.Context) (Phase, error) {
	var raw string

	err := r.db.QueryRowContext(ctx, `SELECT phase FROM partition_migration_state WHERE id = 1`).Scan(&raw)
	if err != nil {
		r.mu.RLock()
		cached, hadCache := r.phase, r.ok
		r.mu.RUnlock()

		if r.logger != nil {
			r.logger.Warnf("partitionstate: refresh failed, falling back (cached=%v hadCache=%t): %v", cached, hadCache, err)
		}

		if hadCache {
			return cached, fmt.Errorf("partitionstate: refresh failed: %w", err)
		}

		return PhaseLegacyOnly, fmt.Errorf("partitionstate: refresh failed (no cache): %w", err)
	}

	p, err := parse(raw)
	if err != nil {
		return PhaseLegacyOnly, err
	}

	r.mu.Lock()
	r.phase = p
	r.fetched = r.now()
	r.ok = true
	r.mu.Unlock()

	return p, nil
}

func parse(raw string) (Phase, error) {
	switch Phase(raw) {
	case PhaseLegacyOnly, PhaseDualWrite, PhasePartitioned:
		return Phase(raw), nil
	default:
		return PhaseLegacyOnly, fmt.Errorf("%w: %q", ErrUnknownPhase, raw)
	}
}

// StaticReader is a test/bootstrap helper that returns a fixed phase with no
// database access. It satisfies anything that consumes a *Reader-compatible
// interface in tests.
type StaticReader struct{ P Phase }

// Phase returns the static phase.
func (s StaticReader) Phase(_ context.Context) (Phase, error) { return s.P, nil }

// Invalidate is a no-op.
func (StaticReader) Invalidate() {}
