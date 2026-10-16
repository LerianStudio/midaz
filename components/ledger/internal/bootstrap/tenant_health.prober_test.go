// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	tmclient "github.com/LerianStudio/lib-commons/v7/commons/tenant-manager/client"
	libLog "github.com/LerianStudio/lib-observability/v4/log"
	"github.com/LerianStudio/lib-observability/v4/metrics"
	"github.com/bxcodec/dbresolver/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.opentelemetry.io/otel/attribute"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"

	"github.com/LerianStudio/midaz/v4/pkg/utils"
)

var proberTestTime = time.Date(2044, time.March, 3, 4, 5, 6, 0, time.UTC)

// proberLogEntry is one captured log call.
type proberLogEntry struct {
	level  int
	msg    string
	fields map[string]any
}

// proberCapturingLogger records every log call; probes log from several
// goroutines, so it is guarded by a mutex.
type proberCapturingLogger struct {
	libLog.Logger
	mu      sync.Mutex
	entries []proberLogEntry
}

func (logger *proberCapturingLogger) Log(_ context.Context, level int, msg string, fields ...any) {
	entry := proberLogEntry{level: level, msg: msg, fields: map[string]any{}}

	for _, field := range fields {
		if typed, ok := field.(libLog.Field); ok {
			entry.fields[typed.Key] = typed.Value
		}
	}

	logger.mu.Lock()
	logger.entries = append(logger.entries, entry)
	logger.mu.Unlock()
}

func (logger *proberCapturingLogger) withMessage(msg string) []proberLogEntry {
	logger.mu.Lock()
	defer logger.mu.Unlock()

	matched := []proberLogEntry{}

	for _, entry := range logger.entries {
		if entry.msg == msg {
			matched = append(matched, entry)
		}
	}

	return matched
}

// proberClock is a manual clock: it only moves when a test advances it.
type proberClock struct {
	mu  sync.Mutex
	now time.Time
}

func (clock *proberClock) Now() time.Time {
	clock.mu.Lock()
	defer clock.mu.Unlock()

	return clock.now
}

func (clock *proberClock) advance(step time.Duration) {
	clock.mu.Lock()
	clock.now = clock.now.Add(step)
	clock.mu.Unlock()
}

type proberTenantCache []string

func (tenants proberTenantCache) TenantIDs() []string { return append([]string(nil), tenants...) }

type proberTenantLister struct {
	tenants []*tmclient.TenantSummary
	err     error
}

func (lister *proberTenantLister) GetActiveTenantsByService(context.Context, string) ([]*tmclient.TenantSummary, error) {
	return lister.tenants, lister.err
}

func proberSummaries(ids ...string) []*tmclient.TenantSummary {
	summaries := make([]*tmclient.TenantSummary, 0, len(ids))
	for _, id := range ids {
		summaries = append(summaries, &tmclient.TenantSummary{ID: id})
	}

	return summaries
}

// proberTenantBehavior scripts what a resolver does for one tenant.
type proberTenantBehavior struct {
	err   error
	block bool
	panic bool
	// onCall runs on every resolution, before the scripted outcome.
	onCall func()
}

type proberDB struct{ dbresolver.DB }

// fakeProberPG resolves tenants to a placeholder handle unless scripted
// otherwise, and records each resolved tenant in order.
type fakeProberPG struct {
	mu        sync.Mutex
	behaviors map[string]proberTenantBehavior
	db        dbresolver.DB
	resolved  []string
	release   chan struct{}
}

func newFakeProberPG() *fakeProberPG {
	return &fakeProberPG{behaviors: map[string]proberTenantBehavior{}, release: make(chan struct{})}
}

func (resolver *fakeProberPG) script(tenantID string, behavior proberTenantBehavior) {
	resolver.mu.Lock()
	resolver.behaviors[tenantID] = behavior
	resolver.mu.Unlock()
}

func (resolver *fakeProberPG) GetDB(_ context.Context, tenantID string) (dbresolver.DB, error) {
	resolver.mu.Lock()
	resolver.resolved = append(resolver.resolved, tenantID)
	behavior := resolver.behaviors[tenantID]
	db := resolver.db
	resolver.mu.Unlock()

	if behavior.onCall != nil {
		behavior.onCall()
	}

	switch {
	case behavior.panic:
		panic("tenant pool exploded")
	case behavior.block:
		// Ignores its context on purpose: only the probe timeout can end the wait.
		<-resolver.release
		return nil, errors.New("released")
	case behavior.err != nil:
		return nil, behavior.err
	}

	if db == nil {
		return proberDB{}, nil
	}

	return db, nil
}

func (resolver *fakeProberPG) calls() []string {
	resolver.mu.Lock()
	defer resolver.mu.Unlock()

	return append([]string(nil), resolver.resolved...)
}

// fakeProberMongo hands out a placeholder client; the ping seam decides the outcome.
type fakeProberMongo struct {
	mu       sync.Mutex
	failing  map[string]error
	resolved []string
}

func (resolver *fakeProberMongo) fail(tenantID string, err error) {
	resolver.mu.Lock()
	resolver.failing[tenantID] = err
	resolver.mu.Unlock()
}

func (resolver *fakeProberMongo) calls() []string {
	resolver.mu.Lock()
	defer resolver.mu.Unlock()

	return append([]string(nil), resolver.resolved...)
}

func (resolver *fakeProberMongo) GetConnection(_ context.Context, tenantID string) (*mongo.Client, error) {
	resolver.mu.Lock()
	defer resolver.mu.Unlock()

	resolver.resolved = append(resolver.resolved, tenantID)

	if err := resolver.failing[tenantID]; err != nil {
		return nil, err
	}

	return &mongo.Client{}, nil
}

func newProberMetricsReader(t *testing.T) (*sdkmetric.ManualReader, *metrics.MetricsFactory) {
	t.Helper()

	reader := sdkmetric.NewManualReader()
	provider := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	t.Cleanup(func() { require.NoError(t, provider.Shutdown(context.Background())) })

	factory, err := metrics.NewMetricsFactory(provider.Meter("tenant-health-prober-test"), nil)
	require.NoError(t, err)

	return reader, factory
}

// proberMetricPoints returns each recorded point of the named instrument, keyed
// by its attributes rendered as "key=value,..." in attribute order.
func proberMetricPoints(t *testing.T, reader *sdkmetric.ManualReader, name string) map[string]int64 {
	t.Helper()

	var collected metricdata.ResourceMetrics
	require.NoError(t, reader.Collect(context.Background(), &collected))

	points := map[string]int64{}

	for _, scope := range collected.ScopeMetrics {
		for _, metric := range scope.Metrics {
			if metric.Name != name {
				continue
			}

			switch data := metric.Data.(type) {
			case metricdata.Gauge[int64]:
				for _, point := range data.DataPoints {
					points[renderProberAttributes(point.Attributes)] = point.Value
				}
			case metricdata.Sum[int64]:
				for _, point := range data.DataPoints {
					points[renderProberAttributes(point.Attributes)] = point.Value
				}
			default:
				t.Fatalf("%s: unexpected data type %T", name, metric.Data)
			}
		}
	}

	return points
}

func renderProberAttributes(set attribute.Set) string {
	rendered := ""

	for _, kv := range set.ToSlice() {
		if rendered != "" {
			rendered += ","
		}

		rendered += string(kv.Key) + "=" + kv.Value.Emit()
	}

	return rendered
}

func dependencyKey(tenantID, dependency string) string {
	return "dependency=" + dependency + ",tenant_id=" + tenantID
}

type proberFixture struct {
	prober *TenantHealthProber
	lease  *fakeReconcilerLease
	logger *proberCapturingLogger
	reader *sdkmetric.ManualReader
	onb    *fakeProberPG
	txn    *fakeProberPG
	mongo  *fakeProberMongo
	clock  *proberClock
}

func newProberFixture(t *testing.T, cfg TenantHealthProbeConfig) *proberFixture {
	t.Helper()

	reader, factory := newProberMetricsReader(t)

	fixture := &proberFixture{
		lease:  &fakeReconcilerLease{held: true},
		logger: &proberCapturingLogger{Logger: libLog.NewNop()},
		reader: reader,
		onb:    newFakeProberPG(),
		txn:    newFakeProberPG(),
		mongo:  &fakeProberMongo{failing: map[string]error{}},
		clock:  &proberClock{now: proberTestTime},
	}

	t.Cleanup(func() {
		close(fixture.onb.release)
		close(fixture.txn.release)
	})

	fixture.prober = NewTenantHealthProber(fixture.logger, fixture.lease, cfg).
		WithPostgres("postgres_onboarding", fixture.onb).
		WithPostgres("postgres_transaction", fixture.txn).
		WithMongo(fixture.mongo).
		WithMetricsFactory(factory)
	fixture.prober.clock = fixture.clock.Now
	fixture.prober.pingPG = func(context.Context, dbresolver.DB) error { return nil }
	fixture.prober.pingMongo = func(context.Context, *mongo.Client) error { return nil }

	return fixture
}

var allProbeDependencies = []string{"postgres_onboarding", "postgres_transaction", "mongo"}

// Scenario 1: every dependency of every tenant answers, so each pair reads 1 and
// nothing is counted as failed. The PostgreSQL probe is the real SELECT 1.
func TestTenantHealthProber_AllDependenciesUp(t *testing.T) {
	fixture := newProberFixture(t, TenantHealthProbeConfig{Timeout: time.Second})

	sqlDB, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherEqual))
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, sqlDB.Close()) })

	// One driver connection: sqlmock closes it once, however many probes ran concurrently.
	sqlDB.SetMaxOpenConns(1)
	mock.MatchExpectationsInOrder(false)

	for range 4 {
		mock.ExpectQuery("SELECT 1").WillReturnRows(sqlmock.NewRows([]string{"one"}).AddRow(1))
	}

	shared := dbresolver.New(dbresolver.WithPrimaryDBs(sqlDB))
	fixture.onb.db = shared
	fixture.txn.db = shared
	fixture.prober.pingPG = defaultTenantPGPing

	// The cache knows tenant C, but the catalog answered: the catalog list is authoritative.
	fixture.prober.WithTenantCache(proberTenantCache{"C"}).
		WithActiveTenantLister(&proberTenantLister{tenants: proberSummaries("A", "B")}, "ledger")

	fixture.prober.pass(t.Context())

	require.NoError(t, mock.ExpectationsWereMet())
	mock.ExpectClose()

	expected := map[string]int64{}
	for _, tenantID := range []string{"A", "B"} {
		for _, dependency := range allProbeDependencies {
			expected[dependencyKey(tenantID, dependency)] = 1
		}
	}

	assert.Equal(t, expected, proberMetricPoints(t, fixture.reader, utils.TenantDependencyUp.Name))
	assert.Empty(t, proberMetricPoints(t, fixture.reader, utils.TenantDependencyProbeFailures.Name))
	assert.Empty(t, proberMetricPoints(t, fixture.reader, utils.TenantHealthProbePassFailures.Name))
	assert.NotContains(t, fixture.onb.calls(), "C", "a cache-only tenant is not probed while the catalog answers")
	assert.Empty(t, fixture.logger.withMessage("tenant dependency probe failed"))
}

// Scenario 2: one tenant's onboarding database fails; only that pair goes down.
func TestTenantHealthProber_OneDependencyDownIsIsolated(t *testing.T) {
	fixture := newProberFixture(t, TenantHealthProbeConfig{Timeout: time.Second})
	fixture.prober.WithTenantCache(proberTenantCache{"A", "B"})
	fixture.onb.script("B", proberTenantBehavior{err: errors.New("connection refused")})

	fixture.prober.pass(t.Context())

	up := proberMetricPoints(t, fixture.reader, utils.TenantDependencyUp.Name)
	assert.Equal(t, int64(0), up[dependencyKey("B", "postgres_onboarding")])

	for _, tenantID := range []string{"A", "B"} {
		for _, dependency := range allProbeDependencies {
			if tenantID == "B" && dependency == "postgres_onboarding" {
				continue
			}

			assert.Equal(t, int64(1), up[dependencyKey(tenantID, dependency)], "%s/%s", tenantID, dependency)
		}
	}

	assert.Equal(t, map[string]int64{dependencyKey("B", "postgres_onboarding"): 1},
		proberMetricPoints(t, fixture.reader, utils.TenantDependencyProbeFailures.Name))

	failures := fixture.logger.withMessage("tenant dependency probe failed")
	require.Len(t, failures, 1)
	assert.Equal(t, libLog.LevelError, failures[0].level)
	assert.Equal(t, "B", failures[0].fields["tenant_id"])
	assert.Equal(t, "postgres_onboarding", failures[0].fields["dependency"])
	assert.NotNil(t, failures[0].fields["error"])
}

// Scenario 3: a failure followed by a success logs the recovery exactly once,
// and the failure keeps being logged on every failing pass.
func TestTenantHealthProber_RecoveryIsLoggedOnce(t *testing.T) {
	fixture := newProberFixture(t, TenantHealthProbeConfig{Timeout: time.Second})
	fixture.prober.WithTenantCache(proberTenantCache{"A"})
	fixture.mongo.fail("A", errors.New("server selection timeout"))

	fixture.prober.pass(t.Context())
	fixture.prober.pass(t.Context())

	assert.Len(t, fixture.logger.withMessage("tenant dependency probe failed"), 2, "every failing pass logs")

	fixture.mongo.fail("A", nil)

	fixture.prober.pass(t.Context())
	fixture.prober.pass(t.Context())

	recovered := fixture.logger.withMessage("tenant dependency recovered")
	require.Len(t, recovered, 1)
	assert.Equal(t, libLog.LevelInfo, recovered[0].level)
	assert.Equal(t, "A", recovered[0].fields["tenant_id"])
	assert.Equal(t, "mongo", recovered[0].fields["dependency"])

	assert.Equal(t, int64(1), proberMetricPoints(t, fixture.reader, utils.TenantDependencyUp.Name)[dependencyKey("A", "mongo")])
	assert.Equal(t, int64(2), proberMetricPoints(t, fixture.reader, utils.TenantDependencyProbeFailures.Name)[dependencyKey("A", "mongo")])
}

// Scenario 4: a failed catalog listing falls back to the cached tenants.
func TestTenantHealthProber_ListerFailureFallsBackToCache(t *testing.T) {
	fixture := newProberFixture(t, TenantHealthProbeConfig{Timeout: time.Second})
	fixture.prober.WithTenantCache(proberTenantCache{"A", "B"}).
		WithActiveTenantLister(&proberTenantLister{err: errors.New("tenant manager unavailable")}, "ledger")

	fixture.prober.pass(t.Context())

	warns := fixture.logger.withMessage("tenant health probe could not list active tenants; probing cached tenants")
	require.Len(t, warns, 1)
	assert.Equal(t, libLog.LevelWarn, warns[0].level)
	assert.NotNil(t, warns[0].fields["error"])

	assert.Equal(t, map[string]int64{"reason=tenant_list": 1},
		proberMetricPoints(t, fixture.reader, utils.TenantHealthProbePassFailures.Name))
	assert.ElementsMatch(t, []string{"A", "B"}, fixture.onb.calls())
	assert.ElementsMatch(t, []string{"A", "B"}, fixture.txn.calls())
}

// Scenario 5: without a lister the cache is the only tenant source.
func TestTenantHealthProber_WithoutListerProbesCachedTenants(t *testing.T) {
	fixture := newProberFixture(t, TenantHealthProbeConfig{Timeout: time.Second})
	fixture.prober.WithTenantCache(proberTenantCache{"A", "B", "A"})

	fixture.prober.pass(t.Context())

	assert.ElementsMatch(t, []string{"A", "B"}, fixture.onb.calls(), "each tenant is probed once per pass")
	assert.ElementsMatch(t, []string{"A", "B"}, fixture.mongo.calls())
	assert.Empty(t, proberMetricPoints(t, fixture.reader, utils.TenantHealthProbePassFailures.Name))
}

// Scenario 6: a resolver that never returns is cut by the probe timeout; the
// dependency reads down and the pass goes on.
func TestTenantHealthProber_BlockingResolverTimesOut(t *testing.T) {
	timeout := 50 * time.Millisecond
	fixture := newProberFixture(t, TenantHealthProbeConfig{Timeout: timeout})
	fixture.prober.WithTenantCache(proberTenantCache{"A"})
	fixture.onb.script("A", proberTenantBehavior{block: true})

	done := make(chan struct{})

	go func() {
		fixture.prober.pass(t.Context())
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(2 * timeout * time.Duration(len(allProbeDependencies))):
		t.Fatal("the pass did not end within twice the probe timeout per dependency")
	}

	up := proberMetricPoints(t, fixture.reader, utils.TenantDependencyUp.Name)
	assert.Equal(t, int64(0), up[dependencyKey("A", "postgres_onboarding")])
	assert.Equal(t, int64(1), up[dependencyKey("A", "postgres_transaction")], "the pass continues after the timeout")
	assert.Equal(t, int64(1), up[dependencyKey("A", "mongo")])

	failures := fixture.logger.withMessage("tenant dependency probe failed")
	require.Len(t, failures, 1)
	assert.ErrorIs(t, failures[0].fields["error"].(error), context.DeadlineExceeded)
}

// Scenario 7: a panicking resolver is recovered and counted as down.
func TestTenantHealthProber_PanickingResolverIsRecovered(t *testing.T) {
	fixture := newProberFixture(t, TenantHealthProbeConfig{Timeout: time.Second})
	fixture.prober.WithTenantCache(proberTenantCache{"A", "B"})
	fixture.txn.script("A", proberTenantBehavior{panic: true})

	require.NotPanics(t, func() { fixture.prober.pass(t.Context()) })

	up := proberMetricPoints(t, fixture.reader, utils.TenantDependencyUp.Name)
	assert.Equal(t, int64(0), up[dependencyKey("A", "postgres_transaction")])
	assert.Equal(t, int64(1), up[dependencyKey("A", "mongo")], "the tenant's remaining probes still run")
	assert.Equal(t, int64(1), up[dependencyKey("B", "postgres_transaction")], "other tenants still run")
	assert.Equal(t, int64(1),
		proberMetricPoints(t, fixture.reader, utils.TenantDependencyProbeFailures.Name)[dependencyKey("A", "postgres_transaction")])
}

// Scenario 8: cancelling the context ends the loop, releases leadership once and
// stops the ticker.
func TestTenantHealthProber_StopsWithContext(t *testing.T) {
	fixture := newProberFixture(t, TenantHealthProbeConfig{Timeout: time.Second})

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	passes := 0
	fixture.prober.WithTenantCache(proberTenantCache{"A"})
	fixture.onb.script("A", proberTenantBehavior{onCall: func() {
		passes++
		if passes == 2 {
			cancel()
		}
	}})

	ticks := make(chan time.Time, 1)
	ticks <- proberTestTime

	stopped := 0
	fixture.prober.newTicker = func(time.Duration) (<-chan time.Time, func()) {
		return ticks, func() { stopped++ }
	}

	returned := make(chan struct{})

	go func() {
		fixture.prober.run(ctx)
		close(returned)
	}()

	select {
	case <-returned:
	case <-time.After(5 * time.Second):
		t.Fatal("run did not return after the context was cancelled")
	}

	assert.Equal(t, 2, passes, "one immediate pass and one per tick")
	assert.Equal(t, 1, fixture.lease.releases, "leadership is released once, on shutdown")
	assert.Equal(t, 1, stopped, "the ticker is stopped")
}

// Scenario 9: a nil metrics factory disables emission without panicking.
func TestTenantHealthProber_NilMetricsFactory(t *testing.T) {
	fixture := newProberFixture(t, TenantHealthProbeConfig{Timeout: time.Second})
	fixture.prober.WithMetricsFactory(nil).
		WithTenantCache(proberTenantCache{"A"}).
		WithActiveTenantLister(&proberTenantLister{err: errors.New("down")}, "ledger")
	fixture.onb.script("A", proberTenantBehavior{err: errors.New("connection refused")})

	require.NotPanics(t, func() { fixture.prober.pass(t.Context()) })

	assert.Empty(t, proberMetricPoints(t, fixture.reader, utils.TenantDependencyUp.Name))
	assert.Len(t, fixture.logger.withMessage("tenant dependency probe failed"), 1)
}

// Scenario 10: another pod leads, so this pod probes nothing and publishes no
// dependency outcome.
func TestTenantHealthProber_SkipsWithoutLease(t *testing.T) {
	fixture := newProberFixture(t, TenantHealthProbeConfig{Timeout: time.Second})
	fixture.lease.held = false
	fixture.prober.WithTenantCache(proberTenantCache{"A"})

	fixture.prober.pass(t.Context())

	assert.Equal(t, 1, fixture.lease.holds)
	assert.Empty(t, fixture.onb.calls())
	assert.Empty(t, fixture.mongo.calls())
	assert.Empty(t, proberMetricPoints(t, fixture.reader, utils.TenantDependencyUp.Name))
	assert.Empty(t, proberMetricPoints(t, fixture.reader, utils.TenantHealthProbePassFailures.Name),
		"another leader is not a failure")

	t.Run("a lease error is counted", func(t *testing.T) {
		fixture.lease.err = errors.New("redis unavailable")

		fixture.prober.pass(t.Context())

		assert.Equal(t, map[string]int64{"reason=lease": 1},
			proberMetricPoints(t, fixture.reader, utils.TenantHealthProbePassFailures.Name))
		assert.Empty(t, fixture.onb.calls())
	})
}

// The leader gauge reports, on every tick, whether this pod held the lease: a
// former leader's tenant_dependency_up series keep their last value, so
// dashboards and alerts select the current leader through this gauge.
func TestTenantHealthProber_LeaderGauge(t *testing.T) {
	leader := func(t *testing.T, fixture *proberFixture) map[string]int64 {
		t.Helper()

		return proberMetricPoints(t, fixture.reader, utils.TenantHealthProbeLeader.Name)
	}

	t.Run("held lease reads 1", func(t *testing.T) {
		fixture := newProberFixture(t, TenantHealthProbeConfig{Timeout: time.Second})

		fixture.prober.pass(t.Context())

		assert.Equal(t, map[string]int64{"": 1}, leader(t, fixture))
	})

	t.Run("lease held by another pod reads 0", func(t *testing.T) {
		fixture := newProberFixture(t, TenantHealthProbeConfig{Timeout: time.Second})
		fixture.lease.held = false

		fixture.prober.pass(t.Context())

		assert.Equal(t, map[string]int64{"": 0}, leader(t, fixture))
	})

	t.Run("lease error reads 0", func(t *testing.T) {
		fixture := newProberFixture(t, TenantHealthProbeConfig{Timeout: time.Second})
		fixture.lease.err = errors.New("redis unavailable")

		fixture.prober.pass(t.Context())

		assert.Equal(t, map[string]int64{"": 0}, leader(t, fixture))
	})

	t.Run("losing the lease moves the gauge from 1 to 0", func(t *testing.T) {
		fixture := newProberFixture(t, TenantHealthProbeConfig{Timeout: time.Second})

		fixture.prober.pass(t.Context())
		require.Equal(t, map[string]int64{"": 1}, leader(t, fixture))

		fixture.lease.held = false

		fixture.prober.pass(t.Context())

		assert.Equal(t, map[string]int64{"": 0}, leader(t, fixture))
	})

	t.Run("nil metrics factory emits nothing", func(t *testing.T) {
		fixture := newProberFixture(t, TenantHealthProbeConfig{Timeout: time.Second})
		fixture.prober.WithMetricsFactory(nil)

		require.NotPanics(t, func() { fixture.prober.pass(t.Context()) })

		assert.Empty(t, leader(t, fixture))
	})
}

// Scenario 11: newly taken leadership forgets the previous tenure's state, so a
// success is not reported as a recovery from a failure another leader saw.
func TestTenantHealthProber_FreshLeaseResetsState(t *testing.T) {
	fixture := newProberFixture(t, TenantHealthProbeConfig{Timeout: time.Second})
	fixture.prober.WithTenantCache(proberTenantCache{"A"})
	fixture.onb.script("A", proberTenantBehavior{err: errors.New("connection refused")})

	fixture.prober.pass(t.Context())
	require.Len(t, fixture.logger.withMessage("tenant dependency probe failed"), 1)

	fixture.onb.script("A", proberTenantBehavior{})
	fixture.lease.freshNext = true

	fixture.prober.pass(t.Context())

	assert.Empty(t, fixture.logger.withMessage("tenant dependency recovered"),
		"state from an earlier tenure is not trusted")
	assert.Equal(t, int64(1), proberMetricPoints(t, fixture.reader, utils.TenantDependencyUp.Name)[dependencyKey("A", "postgres_onboarding")])
}

// Scenario 12: the pass budget runs out; the tenants left over are counted,
// logged, and probed first on the next pass.
func TestTenantHealthProber_BudgetCarriesTenantsOver(t *testing.T) {
	fixture := newProberFixture(t, TenantHealthProbeConfig{Timeout: time.Second, PassBudget: time.Second})
	fixture.prober.parallelism = 1

	tenants := make([]string, 0, 10)
	for i := range 10 {
		tenantID := fmt.Sprintf("t%02d", i)
		tenants = append(tenants, tenantID)
		fixture.onb.script(tenantID, proberTenantBehavior{onCall: func() { fixture.clock.advance(400 * time.Millisecond) }})
	}

	fixture.prober.WithActiveTenantLister(&proberTenantLister{tenants: proberSummaries(tenants...)}, "ledger")

	fixture.prober.pass(t.Context())

	assert.Equal(t, []string{"t00", "t01", "t02"}, fixture.onb.calls(), "tenants start until the budget ends")

	warns := fixture.logger.withMessage("tenant health probe pass budget exhausted")
	require.Len(t, warns, 1)
	assert.Equal(t, libLog.LevelWarn, warns[0].level)
	assert.EqualValues(t, 3, warns[0].fields["probed"])
	assert.EqualValues(t, 7, warns[0].fields["remaining"])
	assert.Equal(t, map[string]int64{"reason=budget": 1},
		proberMetricPoints(t, fixture.reader, utils.TenantHealthProbePassFailures.Name))

	fixture.prober.pass(t.Context())

	calls := fixture.onb.calls()
	require.Len(t, calls, 6)
	assert.Equal(t, []string{"t03", "t04", "t05"}, calls[3:], "carried-over tenants go first")
}
