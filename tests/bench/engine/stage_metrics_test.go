//go:build bench

// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package engine

import (
	"context"
	"fmt"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/LerianStudio/lib-commons/v7/commons/errgroup"
	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// sqlStagesEnv opts a cell into per-session SQL stage attribution; unset installs no tracer.
const sqlStagesEnv = "BENCH_SQL_STAGES"

// sqlStage is one label of the fixed statement vocabulary; stageOther takes every text outside it.
type sqlStage int

const (
	stageBegin sqlStage = iota
	stageRenew
	stageClaim
	stageTouch
	stageLockBalances
	stageLockControls
	stageInsertTransactions
	stageInsertOperations
	stageUpdateBalances
	stageUpdateControls
	stageOutbox
	stageFinalize
	stageCommit
	stageRollback
	stageOther
	stageCount
)

var stageNames = [stageCount]string{
	"begin", "renew fence", "claim", "touch", "lock balances", "lock controls", "insert transactions", "insert operations",
	"update balances", "update controls", "outbox insert", "finalize", "commit", "rollback", "other",
}

// stageTexts are the exact texts the writer sends: its fixed statements and pgx's for a READ COMMITTED transaction.
var stageTexts = map[string]sqlStage{
	"begin isolation level read committed": stageBegin, renewFenceSQL: stageRenew, claimSQL: stageClaim, touchSQL: stageTouch,
	lockBalancesSQL: stageLockBalances, lockControlsSQL: stageLockControls, insertTransactionsSQL: stageInsertTransactions,
	insertOperationsSQL: stageInsertOperations, updateBalancesSQL: stageUpdateBalances, updateControlsSQL: stageUpdateControls,
	finalizeSQL: stageFinalize, "commit": stageCommit, "rollback": stageRollback,
}

// outboxInsertPrefix starts the lib-commons outbox repository's INSERT, whose VALUES list grows with its rows.
const outboxInsertPrefix = `INSERT INTO "outbox_events" (`

// stageOf labels a statement text; the text itself is never kept.
func stageOf(text string) sqlStage {
	if s, ok := stageTexts[text]; ok {
		return s
	}

	if strings.HasPrefix(text, outboxInsertPrefix) {
		return stageOutbox
	}

	return stageOther
}

// sqlStages attributes the SQL of a cell's writer sessions to the vocabulary over one window on the batchers' clock.
// One lock orders the window's edges and every query's end by the clock read under it, so each query is classified
// once against the edges that preceded its end.
type sqlStages struct {
	now      func() time.Time
	mu       sync.Mutex
	from, to time.Time // zero until opened, then closed
	stages   [stageCount]stageTotal
	crossing int64
	sessions int // counted at each session's first query
}

// stageTracer is one session's pgx QueryTracer; ran, guarded by its set's lock, counts the session once.
type stageTracer struct {
	set *sqlStages
	ran bool
}

var _ pgx.QueryTracer = (*stageTracer)(nil)

// stageTotal aggregates one stage's queries wholly inside the window: the mean is Sum over N, derived when reported.
type stageTotal struct {
	Stage  string        `json:"stage"`
	N      int64         `json:"n"`
	Errors int64         `json:"errors"`
	Sum    time.Duration `json:"sumNS"`
	Max    time.Duration `json:"maxNS"`
}

// sqlStageReport is the window's attribution, read after the writer joined. Durations are pgx's elapsed statements,
// network, server, waits and decoding together; sums add work across parallel sessions, never wall-clock shares.
type sqlStageReport struct {
	Sessions int          `json:"sessions"`   // sessions seen running SQL, warm-up included
	From     time.Time    `json:"windowFrom"` // after S1, before the measured phase
	To       time.Time    `json:"windowTo"`   // after its caller drain, before S2
	Stages   []stageTotal `json:"stages"`     // stages observed, in vocabulary order
	Crossing int64        `json:"crossingExcluded"`
}

// stageStartKey carries a query's stage and start from TraceQueryStart to TraceQueryEnd.
type stageStartKey struct{}

type stageStart struct {
	stage sqlStage
	at    time.Time
}

// newSQLStages reads BENCH_SQL_STAGES: unset returns nil, which installs nothing; 1 returns a set on now; any other
// value is an error.
func newSQLStages(getenv func(string) string, now func() time.Time) (*sqlStages, error) {
	switch v := getenv(sqlStagesEnv); v {
	case "":
		return nil, nil
	case "1":
		return &sqlStages{now: now}, nil
	default:
		return nil, fmt.Errorf("%s=%q: want 1 or unset", sqlStagesEnv, v)
	}
}

// sqlStagesFor reads the opt-in before any database work and, when set, traces every writer session env opens.
func sqlStagesFor(t *testing.T, env *benchEnv, now func() time.Time) *sqlStages {
	t.Helper()

	s, err := newSQLStages(os.Getenv, now)
	require.NoError(t, err, "BENCH_SQL_STAGES, before any database work")

	if s != nil {
		env.sessionTracer = s.session // the cell's writer sessions only
	}

	return s
}

// session returns a new session's own tracer.
func (s *sqlStages) session() pgx.QueryTracer { return &stageTracer{set: s} }

// open sets the window's first edge; on nil it does nothing.
func (s *sqlStages) open() {
	if s != nil {
		s.mu.Lock()
		defer s.mu.Unlock()

		s.from = s.now()
	}
}

// close sets the window's last edge; on nil it does nothing.
func (s *sqlStages) close() {
	if s != nil {
		s.mu.Lock()
		defer s.mu.Unlock()

		s.to = s.now()
	}
}

func (t *stageTracer) TraceQueryStart(ctx context.Context, _ *pgx.Conn, data pgx.TraceQueryStartData) context.Context {
	stage, s := stageOf(data.SQL), t.set

	s.mu.Lock()

	if !t.ran {
		t.ran, s.sessions = true, s.sessions+1
	}

	at := s.now()

	s.mu.Unlock()

	return context.WithValue(ctx, stageStartKey{}, stageStart{stage, at})
}

// TraceQueryEnd counts a query wholly inside the window under its stage, one crossing an edge only as crossing, and
// one wholly before or after not at all.
func (t *stageTracer) TraceQueryEnd(ctx context.Context, _ *pgx.Conn, data pgx.TraceQueryEndData) {
	start, ok := ctx.Value(stageStartKey{}).(stageStart)
	if !ok {
		return // pgx always passes the context TraceQueryStart returned
	}

	s := t.set

	s.mu.Lock()
	defer s.mu.Unlock()

	end := s.now()

	switch {
	case s.from.IsZero() || (!s.to.IsZero() && !start.at.Before(s.to)):
	case start.at.Before(s.from) || (!s.to.IsZero() && end.After(s.to)):
		s.crossing++
	default:
		st, d := &s.stages[start.stage], end.Sub(start.at)
		st.N, st.Sum, st.Max = st.N+1, st.Sum+d, max(st.Max, d)

		if data.Err != nil {
			st.Errors++
		}
	}
}

// report returns and logs the window's attribution; nil without the opt-in or a window that never opened, after a
// failed warm-up or S1. Call it after the writer joined. The log is the only output of a cell that fails.
func (s *sqlStages) report(t *testing.T) *sqlStageReport {
	t.Helper()

	if s == nil {
		return nil
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if s.from.IsZero() {
		return nil
	}

	r := &sqlStageReport{Sessions: s.sessions, From: s.from, To: s.to, Crossing: s.crossing}

	for i, st := range s.stages {
		if st.N > 0 {
			st.Stage = stageNames[i]
			r.Stages = append(r.Stages, st)
		}
	}

	t.Logf("sql stages inside S1..S2, caller drain included, edge-crossing queries excluded, tracer cost unmeasured: %s", r)

	return r
}

func (r *sqlStageReport) String() string {
	var b strings.Builder

	fmt.Fprintf(&b, "window %s over %d sessions, summed work, not wall-clock", r.To.Sub(r.From), r.Sessions)

	for _, s := range r.Stages {
		fmt.Fprintf(&b, "; %s n=%d mean=%s max=%s errors=%d", s.Stage, s.N, s.Sum/time.Duration(s.N), s.Max, s.Errors)
	}

	fmt.Fprintf(&b, "; crossing an edge, excluded=%d", r.Crossing)

	return b.String()
}

// Unset installs nothing and an inert set; 1 opts in, reporting nothing until its window opens; any other value fails,
// before database work.
func TestSQLStagesOptIn(t *testing.T) {
	clock := newTestClock(scheduleStart)

	off, err := newSQLStages(envOf(), clock.now)
	require.NoError(t, err)
	assert.Nil(t, off)

	off.open()
	off.close()
	assert.Nil(t, off.report(t), "no fabricated report without the opt-in")

	on, err := newSQLStages(envOf(sqlStagesEnv, "1"), clock.now)
	require.NoError(t, err)
	require.NotNil(t, on)

	tr := on.session() // warm-up SQL, then a failed warm-up or S1: the window never opens
	tr.TraceQueryEnd(tr.TraceQueryStart(t.Context(), nil, pgx.TraceQueryStartData{SQL: claimSQL}), nil, pgx.TraceQueryEndData{})
	assert.Nil(t, on.report(t), "no fabricated report for a window never opened")

	for _, v := range []string{"0", "true", "yes", "2", " 1", "1 "} {
		_, err := newSQLStages(envOf(sqlStagesEnv, v), clock.now)
		assert.ErrorContains(t, err, sqlStagesEnv, "%q", v)
	}
}

// Every fixed writer text has its own label; the library's outbox INSERT matches by prefix whatever its row count;
// anything else is other.
func TestSQLStageLabels(t *testing.T) {
	outbox := `INSERT INTO "outbox_events" (id, event_type, aggregate_id, payload, status, attempts, published_at, last_error, ` +
		`created_at, updated_at) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10), ($11, $12, $13, $14, $15, $16, $17, $18, $19, $20) RETURNING id`

	for text, want := range map[string]string{
		"begin isolation level read committed": "begin", renewFenceSQL: "renew fence", claimSQL: "claim", touchSQL: "touch",
		lockBalancesSQL: "lock balances", lockControlsSQL: "lock controls", insertTransactionsSQL: "insert transactions",
		insertOperationsSQL: "insert operations", updateBalancesSQL: "update balances", updateControlsSQL: "update controls",
		outbox: "outbox insert", finalizeSQL: "finalize", "commit": "commit", "rollback": "rollback",
		`SET LOCAL search_path TO "public"`: "other", acquireFenceSQL: "other", reloadBalancesSQL: "other", "begin": "other",
	} {
		assert.Equal(t, want, stageNames[stageOf(text)], text)
	}
}

// The opted-in env gives every session the ownerless writer opens a tracer of its own, the unused template's included.
// Sessions dial lazily, so nothing here reaches a database. Whether a tracer reaches the dialed connection is left to a
// live run's sessions count.
func TestSQLStagesSessions(t *testing.T) {
	noLeaks(t)
	t.Setenv(sqlStagesEnv, "1")

	clock := newTestClock(scheduleStart)
	primary, err := pgx.ParseConfig("postgres://bench@127.0.0.1:1/bench_transaction")
	require.NoError(t, err)

	env := &benchEnv{primary: primary}
	require.NotNil(t, sqlStagesFor(t, env, clock.now))

	made, session := 0, env.sessionTracer
	env.sessionTracer = func() pgx.QueryTracer { made++; return session() }

	template, err := newBatcher(namedSession(t, env, "stages-0", nil), nil, nil, clock.now)
	require.NoError(t, err)

	w := newTestOwnerless(t.Context(), t, env, template, "stages", 3, 1)
	require.NoError(t, w.Close(t.Context()))

	assert.Equal(t, 4, made, "the template and three workers")
	assert.Nil(t, primary.Tracer, "the shared config stays untraced")
}

// Through the QueryTracer interface, in pgx's call order: a query wholly inside the window counts under its label,
// failures included, one crossing an edge only as crossing, one before or after not at all. Sessions keep their own
// starts, and one that never runs SQL is not counted.
func TestSQLStagesWindow(t *testing.T) {
	clock := newTestClock(scheduleStart)
	set, err := newSQLStages(envOf(sqlStagesEnv, "1"), clock.now)
	require.NoError(t, err)

	a, b, _ := set.session(), set.session(), set.session() // the third never runs SQL, as the ownerless template

	query := func(tr pgx.QueryTracer, text string) func(error) {
		ctx := tr.TraceQueryStart(t.Context(), nil, pgx.TraceQueryStartData{SQL: text})
		return func(err error) { tr.TraceQueryEnd(ctx, nil, pgx.TraceQueryEndData{Err: err}) }
	}

	query(a, claimSQL)(nil) // before the window
	early := query(a, lockBalancesSQL)

	clock.advance(time.Millisecond)
	set.open()

	from := clock.now()
	clock.advance(time.Millisecond)
	early(nil) // started before the window opened

	claimA := query(a, claimSQL)

	clock.advance(3 * time.Millisecond)

	claimB := query(b, claimSQL)

	clock.advance(2 * time.Millisecond)
	claimA(nil) // the longer query finishes first: 5 ms
	clock.advance(time.Millisecond)
	claimB(nil) // overlapped a's on its own session: 3 ms

	lock := query(b, lockBalancesSQL)

	clock.advance(2 * time.Millisecond)
	lock(fmt.Errorf("deadlock"))

	late := query(a, "commit")

	clock.advance(time.Millisecond)
	set.close()

	to := clock.now()
	clock.advance(time.Millisecond)
	late(nil) // ended after the window closed

	query(b, "rollback")(nil) // after the window

	got := set.report(t)
	assert.Equal(t, &sqlStageReport{
		Sessions: 2, From: from, To: to, Crossing: 2,
		Stages: []stageTotal{
			{Stage: "claim", N: 2, Sum: 8 * time.Millisecond, Max: 5 * time.Millisecond},
			{Stage: "lock balances", N: 1, Errors: 1, Sum: 2 * time.Millisecond, Max: 2 * time.Millisecond},
		},
	}, got)
	assert.Contains(t, got.String(), "claim n=2 mean=4ms max=5ms errors=0")
}

// Sessions on their own goroutines share the set race-free: each counted once, every query once.
func TestSQLStagesConcurrentSessions(t *testing.T) {
	clock := newTestClock(scheduleStart)
	set, err := newSQLStages(envOf(sqlStagesEnv, "1"), clock.now)
	require.NoError(t, err)

	set.open()

	var g errgroup.Group

	for range 4 {
		tr := set.session()

		g.Go(func() error {
			for range 25 {
				ctx := tr.TraceQueryStart(t.Context(), nil, pgx.TraceQueryStartData{SQL: claimSQL})

				clock.advance(time.Millisecond)
				tr.TraceQueryEnd(ctx, nil, pgx.TraceQueryEndData{})
			}

			return nil
		})
	}

	require.NoError(t, g.Wait())
	set.close()

	got := set.report(t)
	require.NotNil(t, got)
	assert.Equal(t, 4, got.Sessions)
	assert.Zero(t, got.Crossing)
	require.Len(t, got.Stages, 1)
	assert.Equal(t, int64(100), got.Stages[0].N)
}
