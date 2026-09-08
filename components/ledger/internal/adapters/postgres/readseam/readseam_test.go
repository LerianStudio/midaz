// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package readseam

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	libObservability "github.com/LerianStudio/lib-observability/v4"
	"github.com/LerianStudio/lib-observability/v4/metrics"
	"github.com/bxcodec/dbresolver/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/metric/noop"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"

	"github.com/LerianStudio/midaz/v4/components/ledger/pkg/readrouting"
	"github.com/LerianStudio/midaz/v4/pkg/utils"
)

// fakeTx is a minimal dbresolver.Tx double for the read-only transaction path.
// It records the commit call so tests can assert the release lifecycle and can be
// primed with commitErr to exercise release-error propagation. The read-only seam
// finalizes via Commit and never rolls back, so Rollback carries no bookkeeping.
type fakeTx struct {
	committed bool
	commitErr error
}

func (t *fakeTx) Commit() error {
	t.committed = true

	return t.commitErr
}

func (t *fakeTx) Rollback() error { return nil }

func (t *fakeTx) Exec(_ string, _ ...any) (sql.Result, error) { return nil, nil }

func (t *fakeTx) ExecContext(_ context.Context, _ string, _ ...any) (sql.Result, error) {
	return nil, nil
}

func (t *fakeTx) Prepare(_ string) (dbresolver.Stmt, error) { return nil, nil }

func (t *fakeTx) PrepareContext(_ context.Context, _ string) (dbresolver.Stmt, error) {
	return nil, nil
}

func (t *fakeTx) Query(_ string, _ ...any) (*sql.Rows, error) { return nil, nil }

func (t *fakeTx) QueryContext(_ context.Context, _ string, _ ...any) (*sql.Rows, error) {
	return nil, nil
}

func (t *fakeTx) QueryRow(_ string, _ ...any) *sql.Row { return nil }

func (t *fakeTx) QueryRowContext(_ context.Context, _ string, _ ...any) *sql.Row {
	return nil
}

func (t *fakeTx) Stmt(_ dbresolver.Stmt) dbresolver.Stmt { return nil }

func (t *fakeTx) StmtContext(_ context.Context, _ dbresolver.Stmt) dbresolver.Stmt { return nil }

// Ensure the fake satisfies the concrete dbresolver.Tx interface used in the seam.
var _ dbresolver.Tx = (*fakeTx)(nil)

// fakeReadConn is a ReadConn double for exercising AcquireReadFrom without a real
// dbresolver / database. It records whether BeginTx was invoked so tests can assert
// which branch the seam took.
type fakeReadConn struct {
	beginCalled bool
	beginOpts   *sql.TxOptions
	tx          dbresolver.Tx
	beginErr    error
}

func (c *fakeReadConn) QueryContext(_ context.Context, _ string, _ ...any) (*sql.Rows, error) {
	return nil, nil
}

func (c *fakeReadConn) QueryRowContext(_ context.Context, _ string, _ ...any) *sql.Row {
	return nil
}

func (c *fakeReadConn) BeginTx(_ context.Context, opts *sql.TxOptions) (dbresolver.Tx, error) {
	c.beginCalled = true
	c.beginOpts = opts

	if c.beginErr != nil {
		return nil, c.beginErr
	}

	return c.tx, nil
}

// Ensure the fake satisfies the seam's handle interface.
var _ ReadConn = (*fakeReadConn)(nil)

func TestAcquireReadFrom(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		routeToPrimary bool
		primaryIntent  bool
		wantBeginTx    bool
	}{
		{
			name:           "flag off, intent absent -> direct read (no BeginTx)",
			routeToPrimary: false,
			primaryIntent:  false,
			wantBeginTx:    false,
		},
		{
			name:           "flag off, intent present -> direct read (no BeginTx)",
			routeToPrimary: false,
			primaryIntent:  true,
			wantBeginTx:    false,
		},
		{
			name:           "flag on, intent absent -> direct read (no BeginTx)",
			routeToPrimary: true,
			primaryIntent:  false,
			wantBeginTx:    false,
		},
		{
			name:           "flag on, intent present -> read-only tx opened (BeginTx)",
			routeToPrimary: true,
			primaryIntent:  true,
			wantBeginTx:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctx := context.Background()
			if tt.primaryIntent {
				ctx = readrouting.WithPrimaryRead(ctx)
			}

			ftx := &fakeTx{}
			conn := &fakeReadConn{tx: ftx}

			reader, release, _, err := AcquireReadFrom(ctx, conn, tt.routeToPrimary)
			require.NoError(t, err)
			require.NotNil(t, reader)
			require.NotNil(t, release)

			assert.Equal(t, tt.wantBeginTx, conn.beginCalled, "BeginTx branch mismatch")

			if tt.wantBeginTx {
				// read-only tx opened with ReadOnly option, reader IS the tx
				require.NotNil(t, conn.beginOpts)
				assert.True(t, conn.beginOpts.ReadOnly, "tx must be read-only")
				assert.Same(t, dbresolver.Tx(ftx), reader.(dbresolver.Tx),
					"reader must be the read-only tx")

				require.NoError(t, release())
				assert.True(t, ftx.committed, "read-only tx must be finalized (committed) on release")
			} else {
				// direct path: reader is the connection, release is a no-op
				assert.NoError(t, release())
				assert.False(t, ftx.committed)
			}
		})
	}
}

func TestAcquireReadFrom_FailClosedOnBeginTxError(t *testing.T) {
	t.Parallel()

	ctx := readrouting.WithPrimaryRead(context.Background())

	beginErr := errors.New("primary unavailable")
	conn := &fakeReadConn{beginErr: beginErr}

	ctx, recorder, endSpan := recordingReadSeamContext(ctx)

	reader, release, source, err := AcquireReadFrom(ctx, conn, true)
	endSpan()

	require.Error(t, err, "must fail closed when BeginTx fails")
	assert.ErrorIs(t, err, beginErr, "must propagate the BeginTx error")
	assert.Nil(t, reader, "must NOT return a replica/direct reader on fail-closed")
	assert.Nil(t, release, "must NOT return a release func on fail-closed")
	assert.Equal(t, ReadSource(""), source, "fail-closed must return the zero ReadSource")
	assert.True(t, conn.beginCalled)

	// The fail-closed path must NOT set a misleading db.read_source attribute.
	span := endedReadSeamSpan(t, recorder)
	assert.False(t, hasAttribute(span, "db.read_source"),
		"fail-closed must NOT emit a db.read_source attribute")
}

// TestAcquireReadFrom_FailsFastOnCancelledContext proves the primary-read path
// aborts before the BeginTx round-trip when the caller's context is already
// cancelled: it returns a context.Canceled error, no reader, no release, the
// zero ReadSource, and never touches the connection (BeginTx not called).
func TestAcquireReadFrom_FailsFastOnCancelledContext(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(readrouting.WithPrimaryRead(context.Background()))
	cancel()

	conn := &fakeReadConn{tx: &fakeTx{}}

	reader, release, source, err := AcquireReadFrom(ctx, conn, true)

	require.Error(t, err, "must fail fast when the context is already cancelled")
	assert.ErrorIs(t, err, context.Canceled, "must propagate the context cancellation")
	assert.Nil(t, reader, "must NOT return a reader on cancelled-context fail-fast")
	assert.Nil(t, release, "must NOT return a release func on cancelled-context fail-fast")
	assert.Equal(t, ReadSource(""), source, "cancelled-context fail-fast must return the zero ReadSource")
	assert.False(t, conn.beginCalled, "must NOT open a transaction when the context is already cancelled")
}

// recordingReadSeamContext starts a recording SDK span on the given context so
// AcquireReadFrom can attach db.read_source to trace.SpanFromContext(ctx), and
// returns the child context, the recorder, and the span's End func (which the
// test MUST call before inspecting recorded attributes).
func recordingReadSeamContext(ctx context.Context) (context.Context, *tracetest.SpanRecorder, func()) {
	recorder := tracetest.NewSpanRecorder()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(recorder))

	ctx, span := tp.Tracer("readseam-test").Start(ctx, "readseam.acquire")

	return ctx, recorder, func() { span.End() }
}

// endedReadSeamSpan returns the single recorded span, failing if none was captured.
func endedReadSeamSpan(t *testing.T, recorder *tracetest.SpanRecorder) sdktrace.ReadOnlySpan {
	t.Helper()

	ended := recorder.Ended()
	require.Len(t, ended, 1, "exactly one span must be recorded")

	return ended[0]
}

func hasAttribute(span sdktrace.ReadOnlySpan, key string) bool {
	for _, kv := range span.Attributes() {
		if string(kv.Key) == key {
			return true
		}
	}

	return false
}

func attributeValue(span sdktrace.ReadOnlySpan, key string) (string, bool) {
	for _, kv := range span.Attributes() {
		if string(kv.Key) == key {
			return kv.Value.AsString(), true
		}
	}

	return "", false
}

// TestReadSourceObservability locks the read-source signal: AcquireReadFrom
// returns ReadSourcePrimary only when routeToPrimary AND the primary-read intent
// is present, else ReadSourceReplica; and it records db.read_source on the
// caller's span matching the returned source in BOTH success branches.
func TestReadSourceObservability(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		routeToPrimary bool
		primaryIntent  bool
		wantSource     ReadSource
	}{
		{
			name:           "flag off, intent absent -> replica",
			routeToPrimary: false,
			primaryIntent:  false,
			wantSource:     ReadSourceReplica,
		},
		{
			name:           "flag off, intent present -> replica (baseline)",
			routeToPrimary: false,
			primaryIntent:  true,
			wantSource:     ReadSourceReplica,
		},
		{
			name:           "flag on, intent absent -> replica",
			routeToPrimary: true,
			primaryIntent:  false,
			wantSource:     ReadSourceReplica,
		},
		{
			name:           "flag on, intent present -> primary",
			routeToPrimary: true,
			primaryIntent:  true,
			wantSource:     ReadSourcePrimary,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctx := context.Background()
			if tt.primaryIntent {
				ctx = readrouting.WithPrimaryRead(ctx)
			}

			ctx, recorder, endSpan := recordingReadSeamContext(ctx)

			conn := &fakeReadConn{tx: &fakeTx{}}

			_, release, source, err := AcquireReadFrom(ctx, conn, tt.routeToPrimary)
			require.NoError(t, err)
			require.NotNil(t, release)
			require.NoError(t, release())
			endSpan()

			assert.Equal(t, tt.wantSource, source, "returned ReadSource mismatch")

			span := endedReadSeamSpan(t, recorder)
			got, ok := attributeValue(span, "db.read_source")
			require.True(t, ok, "db.read_source attribute must be set in both success branches")
			assert.Equal(t, string(tt.wantSource), got, "db.read_source attribute must match returned source")
		})
	}
}

// newReaderFactory builds a real MetricsFactory backed by a manual reader so the
// test can read back the db_read_source_total points emitted by recordReadSource.
func newReaderFactory(t *testing.T) (*sdkmetric.ManualReader, *metrics.MetricsFactory) {
	t.Helper()

	reader := sdkmetric.NewManualReader()
	mp := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))

	t.Cleanup(func() { _ = mp.Shutdown(context.Background()) })

	factory, err := metrics.NewMetricsFactory(mp.Meter("readseam-metrics-test"), nil)
	require.NoError(t, err)

	return reader, factory
}

// collectReadSourceCounters returns a map of source-label -> value for the
// db_read_source_total counter.
func collectReadSourceCounters(t *testing.T, reader *sdkmetric.ManualReader) map[string]int64 {
	t.Helper()

	var rm metricdata.ResourceMetrics

	require.NoError(t, reader.Collect(context.Background(), &rm))

	totals := make(map[string]int64)

	for _, sm := range rm.ScopeMetrics {
		for _, m := range sm.Metrics {
			if m.Name != utils.DBReadSourceTotal.Name {
				continue
			}

			sum, ok := m.Data.(metricdata.Sum[int64])
			require.True(t, ok, "data type must be Sum[int64], got %T", m.Data)

			for _, dp := range sum.DataPoints {
				source, _ := dp.Attributes.Value("source")
				totals[source.AsString()] = dp.Value
			}
		}
	}

	return totals
}

// TestReadSource_EmitsCounter locks the metric half of the read-source signal: when a
// real MetricsFactory is present on the context, AcquireReadFrom emits
// db_read_source_total once per call with the correct source label, for BOTH the
// primary branch (flag on + intent) and the replica branch (flag off). The read
// itself must never be failed by the metric path.
func TestReadSource_EmitsCounter(t *testing.T) {
	t.Parallel()

	reader, factory := newReaderFactory(t)

	ctx := libObservability.ContextWithMetricFactory(context.Background(), factory)

	// Replica branch: flag off -> direct read, source=replica.
	replicaConn := &fakeReadConn{tx: &fakeTx{}}

	reader1, release1, source1, err := AcquireReadFrom(ctx, replicaConn, false)
	require.NoError(t, err, "metric emission must never fail the read")
	require.NotNil(t, reader1)
	require.NoError(t, release1())
	assert.Equal(t, ReadSourceReplica, source1)

	// Primary branch: flag on + intent -> read-only tx, source=primary.
	primaryCtx := readrouting.WithPrimaryRead(ctx)
	primaryConn := &fakeReadConn{tx: &fakeTx{}}

	reader2, release2, source2, err := AcquireReadFrom(primaryCtx, primaryConn, true)
	require.NoError(t, err, "metric emission must never fail the read")
	require.NotNil(t, reader2)
	require.NoError(t, release2())
	assert.Equal(t, ReadSourcePrimary, source2)
	require.True(t, primaryConn.beginCalled, "guard: primary branch must open the read-only tx")

	totals := collectReadSourceCounters(t, reader)

	assert.Equal(t, int64(1), totals[string(ReadSourceReplica)],
		"one replica read must be counted on db_read_source_total{source=replica}")
	assert.Equal(t, int64(1), totals[string(ReadSourcePrimary)],
		"one primary read must be counted on db_read_source_total{source=primary}")
}

// TestReadSource_NilFactory_SwallowsAndSucceeds exercises the factory==nil guard:
// when the context carries an explicit nil MetricsFactory, recordReadSource returns
// early without emitting, and the read still succeeds. This proves the metric path
// never fails a read even when no factory is available.
func TestReadSource_NilFactory_SwallowsAndSucceeds(t *testing.T) {
	t.Parallel()

	ctx := libObservability.ContextWithMetricFactory(context.Background(), nil)

	conn := &fakeReadConn{tx: &fakeTx{}}

	reader, release, source, err := AcquireReadFrom(ctx, conn, false)
	require.NoError(t, err)
	require.NotNil(t, reader)
	require.NotNil(t, release)
	require.NoError(t, release())
	assert.Equal(t, ReadSourceReplica, source)
}

// failingCounterMeter embeds a noop meter and forces Int64Counter to error so a
// factory built over it makes recordReadSource take its emit-error swallow branch.
type failingCounterMeter struct {
	metric.Meter
}

func (m failingCounterMeter) Int64Counter(string, ...metric.Int64CounterOption) (metric.Int64Counter, error) {
	return nil, errors.New("counter unavailable")
}

// TestReadSource_EmitError_SwallowsAndSucceeds exercises the best-effort contract:
// when the counter cannot be created, recordReadSource swallows the error (logs at
// Debug) and AcquireReadFrom still returns a valid reader with no error.
func TestReadSource_EmitError_SwallowsAndSucceeds(t *testing.T) {
	t.Parallel()

	factory, err := metrics.NewMetricsFactory(
		failingCounterMeter{Meter: noop.NewMeterProvider().Meter("readseam-fail-test")}, nil,
	)
	require.NoError(t, err)

	ctx := libObservability.ContextWithMetricFactory(context.Background(), factory)

	conn := &fakeReadConn{tx: &fakeTx{}}

	reader, release, source, err := AcquireReadFrom(ctx, conn, false)
	require.NoError(t, err, "a metric-emission error must never fail the read")
	require.NotNil(t, reader)
	require.NotNil(t, release)
	require.NoError(t, release())
	assert.Equal(t, ReadSourceReplica, source)
}

// TestAcquireReadFrom_PropagatesCommitError verifies the primary path's release func
// surfaces the read-only tx commit error to the caller.
func TestAcquireReadFrom_PropagatesCommitError(t *testing.T) {
	t.Parallel()

	ctx := readrouting.WithPrimaryRead(context.Background())

	commitErr := errors.New("commit failed")
	ftx := &fakeTx{commitErr: commitErr}
	conn := &fakeReadConn{tx: ftx}

	reader, release, source, err := AcquireReadFrom(ctx, conn, true)
	require.NoError(t, err)
	require.NotNil(t, reader)
	require.NotNil(t, release)
	assert.Equal(t, ReadSourcePrimary, source)

	relErr := release()
	require.Error(t, relErr, "release must surface the commit error")
	assert.ErrorIs(t, relErr, commitErr, "release must propagate the underlying commit error")
	assert.True(t, ftx.committed, "release must attempt commit on the read-only tx")
}
