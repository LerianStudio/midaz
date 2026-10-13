// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package tenantpool

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	tmpostgres "github.com/LerianStudio/lib-commons/v7/commons/tenant-manager/postgres"
	libObservability "github.com/LerianStudio/lib-observability/v4"
	obsconst "github.com/LerianStudio/lib-observability/v4/constants"
	libLog "github.com/LerianStudio/lib-observability/v4/log"
	"github.com/bxcodec/dbresolver/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	testTenantID = "tenant-a"
	testModule   = "onboarding"
)

var errResolve = errors.New("tenant manager unavailable")

// The tenant manager is the production Resolver.
var _ Resolver = (*tmpostgres.Manager)(nil)

// fakeDB is a dbresolver.DB double that records every method it receives, so
// tests can assert which pool a call reached. It never touches a database.
type fakeDB struct {
	mu    sync.Mutex
	calls []string
}

var _ dbresolver.DB = (*fakeDB)(nil)

func newFakeDB() *fakeDB { return &fakeDB{} }

func (f *fakeDB) record(method string) {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.calls = append(f.calls, method)
}

func (f *fakeDB) recorded() []string {
	f.mu.Lock()
	defer f.mu.Unlock()

	return append([]string(nil), f.calls...)
}

func (f *fakeDB) Begin() (dbresolver.Tx, error) {
	f.record("Begin")

	return nil, nil
}

func (f *fakeDB) BeginTx(_ context.Context, _ *sql.TxOptions) (dbresolver.Tx, error) {
	f.record("BeginTx")

	return nil, nil
}

func (f *fakeDB) Close() error {
	f.record("Close")

	return nil
}

func (f *fakeDB) Conn(_ context.Context) (dbresolver.Conn, error) {
	f.record("Conn")

	return nil, nil
}

func (f *fakeDB) Driver() driver.Driver {
	f.record("Driver")

	return nil
}

func (f *fakeDB) Exec(_ string, _ ...any) (sql.Result, error) {
	f.record("Exec")

	return nil, nil
}

func (f *fakeDB) ExecContext(_ context.Context, _ string, _ ...any) (sql.Result, error) {
	f.record("ExecContext")

	return nil, nil
}

func (f *fakeDB) Ping() error {
	f.record("Ping")

	return nil
}

func (f *fakeDB) PingContext(_ context.Context) error {
	f.record("PingContext")

	return nil
}

func (f *fakeDB) Prepare(_ string) (dbresolver.Stmt, error) {
	f.record("Prepare")

	return nil, nil
}

func (f *fakeDB) PrepareContext(_ context.Context, _ string) (dbresolver.Stmt, error) {
	f.record("PrepareContext")

	return nil, nil
}

func (f *fakeDB) Query(_ string, _ ...any) (*sql.Rows, error) {
	f.record("Query")

	return nil, nil
}

func (f *fakeDB) QueryContext(_ context.Context, _ string, _ ...any) (*sql.Rows, error) {
	f.record("QueryContext")

	return nil, nil
}

func (f *fakeDB) QueryRow(_ string, _ ...any) *sql.Row {
	f.record("QueryRow")

	return nil
}

func (f *fakeDB) QueryRowContext(_ context.Context, _ string, _ ...any) *sql.Row {
	f.record("QueryRowContext")

	return nil
}

func (f *fakeDB) SetConnMaxIdleTime(_ time.Duration) { f.record("SetConnMaxIdleTime") }

func (f *fakeDB) SetConnMaxLifetime(_ time.Duration) { f.record("SetConnMaxLifetime") }

func (f *fakeDB) SetMaxIdleConns(_ int) { f.record("SetMaxIdleConns") }

func (f *fakeDB) SetMaxOpenConns(_ int) { f.record("SetMaxOpenConns") }

func (f *fakeDB) PrimaryDBs() []*sql.DB {
	f.record("PrimaryDBs")

	return nil
}

func (f *fakeDB) ReplicaDBs() []*sql.DB {
	f.record("ReplicaDBs")

	return nil
}

func (f *fakeDB) Stats() sql.DBStats {
	f.record("Stats")

	return sql.DBStats{}
}

// fakeResolver stands in for the tenant manager. Its result can be swapped
// between calls, mimicking a manager that closed and rebuilt a tenant pool.
type fakeResolver struct {
	mu        sync.Mutex
	calls     atomic.Int32
	db        dbresolver.DB
	err       error
	tenantIDs []string
}

func (r *fakeResolver) GetDB(_ context.Context, tenantID string) (dbresolver.DB, error) {
	r.calls.Add(1)

	r.mu.Lock()
	defer r.mu.Unlock()

	r.tenantIDs = append(r.tenantIDs, tenantID)

	return r.db, r.err
}

func (r *fakeResolver) set(db dbresolver.DB, err error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.db = db
	r.err = err
}

// recordingLogger captures what the adapter logged. It satisfies
// libLog.Universal, which is all ContextWithLogger needs.
type recordingLogger struct {
	mu      sync.Mutex
	entries []loggedEntry
}

type loggedEntry struct {
	level  int
	msg    string
	fields map[string]any
}

func (l *recordingLogger) Log(_ context.Context, level int, msg string, fields ...any) {
	entry := loggedEntry{level: level, msg: msg, fields: make(map[string]any, len(fields))}

	for _, f := range fields {
		if field, ok := f.(libLog.Field); ok {
			entry.fields[field.Key] = field.Value
		}
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	l.entries = append(l.entries, entry)
}

func (l *recordingLogger) swapWarnings() []loggedEntry {
	l.mu.Lock()
	defer l.mu.Unlock()

	var out []loggedEntry

	for _, e := range l.entries {
		if e.msg == poolSwapLogMessage {
			out = append(out, e)
		}
	}

	return out
}

func newLoggedContext() (context.Context, *recordingLogger) {
	logger := &recordingLogger{}

	return libObservability.ContextWithLogger(context.Background(), logger), logger
}

func TestLazyDB_Current(t *testing.T) {
	t.Parallel()

	pinned := newFakeDB()
	swapped := newFakeDB()

	tests := []struct {
		name         string
		resolvedDB   dbresolver.DB
		resolveErr   error
		cancelCtx    bool
		wantDB       dbresolver.DB
		wantErr      error
		wantWarnings int
		wantResolves int32
	}{
		{
			name:         "same pool as pinned logs no warning (Nenhum aviso sem troca)",
			resolvedDB:   pinned,
			wantDB:       pinned,
			wantResolves: 1,
		},
		{
			name:         "different pool logs exactly one warning (Aviso emitido na troca)",
			resolvedDB:   swapped,
			wantDB:       swapped,
			wantWarnings: 1,
			wantResolves: 1,
		},
		{
			name:         "resolution error falls back to pinned without warning",
			resolveErr:   errResolve,
			wantDB:       pinned,
			wantResolves: 1,
		},
		{
			name:         "nil pool without error falls back to pinned without warning",
			wantDB:       pinned,
			wantResolves: 1,
		},
		{
			name:       "cancelled context returns the context error without resolving",
			resolvedDB: swapped,
			cancelCtx:  true,
			wantDB:     pinned,
			wantErr:    context.Canceled,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			resolver := &fakeResolver{db: tt.resolvedDB, err: tt.resolveErr}
			adapter, ok := New(resolver, testTenantID, testModule, pinned).(*lazyDB)
			require.True(t, ok)

			ctx, logger := newLoggedContext()

			if tt.cancelCtx {
				var cancel context.CancelFunc
				ctx, cancel = context.WithCancel(ctx)
				cancel()
			}

			db, err := adapter.current(ctx)

			if tt.wantErr != nil {
				require.ErrorIs(t, err, tt.wantErr)
			} else {
				require.NoError(t, err)
			}

			assert.Same(t, tt.wantDB, db)
			assert.Equal(t, tt.wantResolves, resolver.calls.Load())

			warnings := logger.swapWarnings()
			require.Len(t, warnings, tt.wantWarnings)

			for _, w := range warnings {
				assert.Equal(t, libLog.LevelWarn, w.level)
				assert.Equal(t, map[string]any{
					obsconst.AttrKeyTenantID: testTenantID,
					logFieldModule:           testModule,
				}, w.fields, "warning must carry only tenant and module")
			}
		})
	}
}

func TestLazyDB_ResolvesWithRequestTenant(t *testing.T) {
	t.Parallel()

	resolver := &fakeResolver{db: newFakeDB()}
	adapter := New(resolver, testTenantID, testModule, newFakeDB())

	ctx, _ := newLoggedContext()
	_, err := adapter.ExecContext(ctx, "SELECT 1")
	require.NoError(t, err)

	resolver.mu.Lock()
	defer resolver.mu.Unlock()

	assert.Equal(t, []string{testTenantID}, resolver.tenantIDs)
}

// contextMethods invokes each context-bearing dbresolver.DB method once and
// returns its error (nil for QueryRowContext, which has no error channel).
var contextMethods = []struct {
	name string
	call func(ctx context.Context, db dbresolver.DB) error
}{
	{"BeginTx", func(ctx context.Context, db dbresolver.DB) error {
		_, err := db.BeginTx(ctx, nil)

		return err
	}},
	{"ExecContext", func(ctx context.Context, db dbresolver.DB) error {
		_, err := db.ExecContext(ctx, "SELECT 1")

		return err
	}},
	{"QueryContext", func(ctx context.Context, db dbresolver.DB) error {
		rows, err := db.QueryContext(ctx, "SELECT 1")
		if rows != nil {
			_ = rows.Close()
		}

		return err
	}},
	{"QueryRowContext", func(ctx context.Context, db dbresolver.DB) error {
		_ = db.QueryRowContext(ctx, "SELECT 1")

		return nil
	}},
	{"PrepareContext", func(ctx context.Context, db dbresolver.DB) error {
		_, err := db.PrepareContext(ctx, "SELECT 1")

		return err
	}},
	{"PingContext", func(ctx context.Context, db dbresolver.DB) error { return db.PingContext(ctx) }},
	{"Conn", func(ctx context.Context, db dbresolver.DB) error {
		_, err := db.Conn(ctx)

		return err
	}},
}

func TestLazyDB_ContextMethods_ReachCurrentPoolAfterSwap(t *testing.T) {
	t.Parallel()

	for _, m := range contextMethods {
		t.Run(m.name, func(t *testing.T) {
			t.Parallel()

			pinned := newFakeDB()
			swapped := newFakeDB()
			resolver := &fakeResolver{db: pinned}
			adapter := New(resolver, testTenantID, testModule, pinned)

			ctx, logger := newLoggedContext()

			require.NoError(t, m.call(ctx, adapter))
			assert.Equal(t, []string{m.name}, pinned.recorded(), "before the swap the call reaches the pinned pool")
			assert.Empty(t, logger.swapWarnings())

			resolver.set(swapped, nil)

			require.NoError(t, m.call(ctx, adapter))
			assert.Equal(t, []string{m.name}, swapped.recorded(), "after the swap the call reaches the current pool")
			assert.Equal(t, []string{m.name}, pinned.recorded(), "the pinned pool is not reached after the swap")
			assert.Len(t, logger.swapWarnings(), 1, "one warning per operation that observes the swap")
		})
	}
}

func TestLazyDB_ContextMethods_FallBackToPinnedOnResolveError(t *testing.T) {
	t.Parallel()

	for _, m := range contextMethods {
		t.Run(m.name, func(t *testing.T) {
			t.Parallel()

			pinned := newFakeDB()
			resolver := &fakeResolver{err: errResolve}
			adapter := New(resolver, testTenantID, testModule, pinned)

			ctx, logger := newLoggedContext()

			require.NoError(t, m.call(ctx, adapter))
			assert.Equal(t, []string{m.name}, pinned.recorded())
			assert.Equal(t, int32(1), resolver.calls.Load())
			assert.Empty(t, logger.swapWarnings())
		})
	}
}

func TestLazyDB_ContextMethods_CancelledContext(t *testing.T) {
	t.Parallel()

	for _, m := range contextMethods {
		t.Run(m.name, func(t *testing.T) {
			t.Parallel()

			pinned := newFakeDB()
			swapped := newFakeDB()
			resolver := &fakeResolver{db: swapped}
			adapter := New(resolver, testTenantID, testModule, pinned)

			ctx, logger := newLoggedContext()
			ctx, cancel := context.WithCancel(ctx)
			cancel()

			err := m.call(ctx, adapter)

			assert.Zero(t, resolver.calls.Load(), "a done context is not resolved")
			assert.Empty(t, swapped.recorded())
			assert.Empty(t, logger.swapWarnings())

			if m.name == "QueryRowContext" {
				// No error channel: the pinned pool's Row defers the context
				// error to Scan, as it did before the adapter existed.
				assert.Equal(t, []string{m.name}, pinned.recorded())

				return
			}

			require.ErrorIs(t, err, context.Canceled)
			assert.Empty(t, pinned.recorded())
		})
	}
}

func TestLazyDB_ContextlessMethods_DelegateToPinned(t *testing.T) {
	t.Parallel()

	methods := []struct {
		name string
		call func(db dbresolver.DB) error
	}{
		{"Begin", func(db dbresolver.DB) error {
			_, err := db.Begin()

			return err
		}},
		{"Exec", func(db dbresolver.DB) error {
			_, err := db.Exec("SELECT 1")

			return err
		}},
		{"Query", func(db dbresolver.DB) error {
			rows, err := db.Query("SELECT 1")
			if rows != nil {
				_ = rows.Close()
			}

			return err
		}},
		{"QueryRow", func(db dbresolver.DB) error {
			_ = db.QueryRow("SELECT 1")

			return nil
		}},
		{"Prepare", func(db dbresolver.DB) error {
			_, err := db.Prepare("SELECT 1")

			return err
		}},
		{"Ping", func(db dbresolver.DB) error { return db.Ping() }},
	}

	for _, m := range methods {
		t.Run(m.name, func(t *testing.T) {
			t.Parallel()

			pinned := newFakeDB()
			swapped := newFakeDB()
			resolver := &fakeResolver{db: swapped}
			adapter := New(resolver, testTenantID, testModule, pinned)

			require.NoError(t, m.call(adapter))
			assert.Equal(t, []string{m.name}, pinned.recorded())
			assert.Empty(t, swapped.recorded())
			assert.Zero(t, resolver.calls.Load())
		})
	}
}

func TestLazyDB_CloseAndSetters_NeverReachUnderlyingPool(t *testing.T) {
	t.Parallel()

	pinned := newFakeDB()
	swapped := newFakeDB()
	resolver := &fakeResolver{db: swapped}
	adapter := New(resolver, testTenantID, testModule, pinned)

	require.NoError(t, adapter.Close())
	adapter.SetConnMaxIdleTime(time.Minute)
	adapter.SetConnMaxLifetime(time.Minute)
	adapter.SetMaxIdleConns(1)
	adapter.SetMaxOpenConns(1)

	assert.Empty(t, pinned.recorded())
	assert.Empty(t, swapped.recorded())
	assert.Zero(t, resolver.calls.Load())
}

func TestLazyDB_DiagnosticMethods(t *testing.T) {
	t.Parallel()

	methods := []struct {
		name string
		call func(db dbresolver.DB)
	}{
		{"Driver", func(db dbresolver.DB) { _ = db.Driver() }},
		{"Stats", func(db dbresolver.DB) { _ = db.Stats() }},
		{"PrimaryDBs", func(db dbresolver.DB) { _ = db.PrimaryDBs() }},
		{"ReplicaDBs", func(db dbresolver.DB) { _ = db.ReplicaDBs() }},
	}

	for _, m := range methods {
		t.Run(m.name+" reaches the current pool after a swap", func(t *testing.T) {
			t.Parallel()

			pinned := newFakeDB()
			swapped := newFakeDB()
			adapter := New(&fakeResolver{db: swapped}, testTenantID, testModule, pinned)

			m.call(adapter)

			assert.Equal(t, []string{m.name}, swapped.recorded())
			assert.Empty(t, pinned.recorded())
		})

		t.Run(m.name+" falls back to pinned on resolve error", func(t *testing.T) {
			t.Parallel()

			pinned := newFakeDB()
			adapter := New(&fakeResolver{err: errResolve}, testTenantID, testModule, pinned)

			m.call(adapter)

			assert.Equal(t, []string{m.name}, pinned.recorded())
		})
	}
}

func TestLazyDB_NilResolverFallsBackToPinned(t *testing.T) {
	t.Parallel()

	pinned := newFakeDB()
	adapter := New(nil, testTenantID, testModule, pinned)

	ctx, logger := newLoggedContext()
	_, err := adapter.ExecContext(ctx, "SELECT 1")
	require.NoError(t, err)

	assert.Equal(t, []string{"ExecContext"}, pinned.recorded())
	assert.Empty(t, logger.swapWarnings())
}

// TestLazyDB_ConcurrentUseAcrossSwap drives one adapter from many goroutines
// while the resolver swaps pools, so `go test -race` can flag shared state.
func TestLazyDB_ConcurrentUseAcrossSwap(t *testing.T) {
	t.Parallel()

	pinned := newFakeDB()
	swapped := newFakeDB()
	resolver := &fakeResolver{db: pinned}
	adapter := New(resolver, testTenantID, testModule, pinned)

	ctx, _ := newLoggedContext()

	const workers = 16

	var wg sync.WaitGroup

	wg.Add(workers + 1)

	go func() {
		defer wg.Done()

		resolver.set(swapped, nil)
	}()

	for i := 0; i < workers; i++ {
		go func() {
			defer wg.Done()

			_, _ = adapter.ExecContext(ctx, "SELECT 1")
			_ = adapter.Stats()
		}()
	}

	wg.Wait()

	assert.Len(t, append(pinned.recorded(), swapped.recorded()...), workers*2)
}
