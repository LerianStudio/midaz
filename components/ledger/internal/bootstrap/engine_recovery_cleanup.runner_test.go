// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	tmcore "github.com/LerianStudio/lib-commons/v7/commons/tenant-manager/core"
	"github.com/LerianStudio/lib-commons/v7/commons/tenant-manager/tenantcache"
	libLog "github.com/LerianStudio/lib-observability/v4/log"
	"github.com/LerianStudio/lib-observability/v4/metrics"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/attribute"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"

	txRedis "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/redis/transaction"
	"github.com/LerianStudio/midaz/v4/pkg/utils"
)

type cleanupCall struct {
	tenantID string
	now      time.Time
	limit    int
}

type cleanupPage struct {
	result txRedis.RecoveryCleanupResult
	err    error
}

// cleanupLockStore stands in for the SetNX lock shared by every pod.
type cleanupLockStore struct {
	mu     sync.Mutex
	owners map[string]string
	err    error
}

func (store *cleanupLockStore) SetNX(_ context.Context, key, value string, ttl time.Duration) (bool, error) {
	store.mu.Lock()
	defer store.mu.Unlock()

	if store.err != nil {
		return false, store.err
	}

	if ttl != engineRecoveryCleanupLockTTL {
		return false, errors.New("unexpected lock ttl")
	}

	if _, held := store.owners[key]; held {
		return false, nil
	}

	if store.owners == nil {
		store.owners = map[string]string{}
	}

	store.owners[key] = value

	return true, nil
}

func (store *cleanupLockStore) DeleteIfValue(_ context.Context, key, value string) (bool, error) {
	store.mu.Lock()
	defer store.mu.Unlock()

	if store.owners[key] != value {
		return false, nil
	}

	delete(store.owners, key)

	return true, nil
}

func (store *cleanupLockStore) held() bool {
	store.mu.Lock()
	defer store.mu.Unlock()

	_, held := store.owners[utils.EngineRecoveryCleanupLockKey()]

	return held
}

// fakeCleanupRepository serves queued pages per tenant; an exhausted queue
// reports an empty schedule.
type fakeCleanupRepository struct {
	*cleanupLockStore
	mu        sync.Mutex
	pages     map[string][]cleanupPage
	repeat    map[string]cleanupPage
	calls     []cleanupCall
	onCleanup func(call cleanupCall)
	backlog   map[string]cleanupBacklog
	backlogs  []cleanupCall
}

type cleanupBacklog struct {
	result txRedis.RecoveryCleanupBacklog
	err    error
}

func newFakeCleanupRepository(locks *cleanupLockStore) *fakeCleanupRepository {
	if locks == nil {
		locks = &cleanupLockStore{}
	}

	return &fakeCleanupRepository{
		cleanupLockStore: locks, pages: map[string][]cleanupPage{}, repeat: map[string]cleanupPage{}, backlog: map[string]cleanupBacklog{},
	}
}

func (repo *fakeCleanupRepository) CleanupEngineRecovery(ctx context.Context, now time.Time, limit int) (txRedis.RecoveryCleanupResult, error) {
	call := cleanupCall{tenantID: tmcore.GetTenantIDContext(ctx), now: now, limit: limit}

	repo.mu.Lock()
	repo.calls = append(repo.calls, call)

	page, repeated := repo.repeat[call.tenantID]
	if queued := repo.pages[call.tenantID]; !repeated && len(queued) > 0 {
		page, repo.pages[call.tenantID] = queued[0], queued[1:]
	}

	onCleanup := repo.onCleanup
	repo.mu.Unlock()

	if onCleanup != nil {
		onCleanup(call)
	}

	return page.result, page.err
}

func (repo *fakeCleanupRepository) EngineRecoveryCleanupBacklog(ctx context.Context, now time.Time) (txRedis.RecoveryCleanupBacklog, error) {
	repo.mu.Lock()
	defer repo.mu.Unlock()

	tenantID := tmcore.GetTenantIDContext(ctx)
	repo.backlogs = append(repo.backlogs, cleanupCall{tenantID: tenantID, now: now})
	backlog := repo.backlog[tenantID]

	return backlog.result, backlog.err
}

func (repo *fakeCleanupRepository) recorded() []cleanupCall {
	repo.mu.Lock()
	defer repo.mu.Unlock()

	return append([]cleanupCall(nil), repo.calls...)
}

func (repo *fakeCleanupRepository) tenantsCalled() []string {
	tenants := []string{}
	for _, call := range repo.recorded() {
		tenants = append(tenants, call.tenantID)
	}

	return tenants
}

type cleanupLogEntry struct {
	level  int
	msg    string
	fields map[string]any
}

type cleanupCapturingLogger struct {
	libLog.Logger
	mu      sync.Mutex
	entries []cleanupLogEntry
}

func (logger *cleanupCapturingLogger) Log(_ context.Context, level int, msg string, fields ...any) {
	entry := cleanupLogEntry{level: level, msg: msg, fields: map[string]any{}}

	for _, field := range fields {
		if typed, ok := field.(libLog.Field); ok {
			entry.fields[typed.Key] = typed.Value
		}
	}

	logger.mu.Lock()
	logger.entries = append(logger.entries, entry)
	logger.mu.Unlock()
}

func (logger *cleanupCapturingLogger) at(level int) []cleanupLogEntry {
	logger.mu.Lock()
	defer logger.mu.Unlock()

	matched := []cleanupLogEntry{}
	for _, entry := range logger.entries {
		if entry.level == level {
			matched = append(matched, entry)
		}
	}

	return matched
}

type staticTenants []string

func (tenants staticTenants) TenantIDs() []string { return append([]string(nil), tenants...) }

// steppingClock returns its current time and advances only when told to, so
// the pass budget is driven by the cleanup calls a test makes.
type steppingClock struct {
	mu  sync.Mutex
	now time.Time
}

func (clock *steppingClock) Now() time.Time {
	clock.mu.Lock()
	defer clock.mu.Unlock()

	return clock.now
}

func (clock *steppingClock) advance(step time.Duration) {
	clock.mu.Lock()
	clock.now = clock.now.Add(step)
	clock.mu.Unlock()
}

var cleanupTestTime = time.Date(2043, time.March, 4, 5, 6, 7, 0, time.UTC)

func newTestCleanupRunner(repo *fakeCleanupRepository, logger libLog.Logger) *EngineRecoveryCleanupRunner {
	if logger == nil {
		logger = recoveryQuietLogger{}
	}

	return NewEngineRecoveryCleanupRunner(logger, repo).WithClock(func() time.Time { return cleanupTestTime })
}

func fullPage() cleanupPage {
	return cleanupPage{result: txRedis.RecoveryCleanupResult{Scanned: engineRecoveryCleanupPageSize, Cleaned: engineRecoveryCleanupPageSize}}
}

func TestEngineRecoveryCleanupRunnerPassesAtStartupAndOnEveryTick(t *testing.T) {
	repo := newFakeCleanupRepository(nil)
	passes := make(chan cleanupCall, 8)
	repo.onCleanup = func(call cleanupCall) { passes <- call }

	ticks := make(chan time.Time)
	runner := newTestCleanupRunner(repo, nil)
	runner.newTicker = func(interval time.Duration) (<-chan time.Time, func()) {
		require.Equal(t, engineRecoveryCleanupInterval, interval)
		return ticks, func() {}
	}

	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan struct{})

	go func() {
		defer close(done)
		runner.run(ctx)
	}()

	awaitPass := func(label string) cleanupCall {
		select {
		case call := <-passes:
			return call
		case <-time.After(5 * time.Second):
			t.Fatalf("%s: no cleanup pass", label)
			return cleanupCall{}
		}
	}

	startup := awaitPass("startup")
	require.Equal(t, cleanupTestTime, startup.now)
	require.Equal(t, engineRecoveryCleanupPageSize, startup.limit)

	ticks <- cleanupTestTime
	awaitPass("first tick")

	ticks <- cleanupTestTime
	awaitPass("second tick")

	cancel()
	<-done
	require.Len(t, repo.recorded(), 3, "one empty page per pass")
	require.False(t, repo.held(), "every pass releases the lock")
}

func TestEngineRecoveryCleanupRunnerDrainsUntilAPageIsShort(t *testing.T) {
	repo := newFakeCleanupRepository(nil)
	repo.pages[""] = []cleanupPage{
		fullPage(), fullPage(),
		{result: txRedis.RecoveryCleanupResult{Scanned: 50, Cleaned: 48, Stale: 2}},
		fullPage(),
	}
	logger := &cleanupCapturingLogger{}

	newTestCleanupRunner(repo, logger).pass(t.Context())

	require.Len(t, repo.recorded(), 3, "a short page means the schedule is drained up to now")

	summaries := logger.at(libLog.LevelDebug)
	require.NotEmpty(t, summaries)
	summary := summaries[len(summaries)-1]
	require.Equal(t, "Engine recovery cleanup pass finished", summary.msg)
	require.EqualValues(t, 250, summary.fields["scanned_count"])
	require.EqualValues(t, 248, summary.fields["cleaned_count"])
	require.Equal(t, true, summary.fields["drained"])
}

func TestEngineRecoveryCleanupRunnerStopsStartingPagesWhenBudgetIsSpent(t *testing.T) {
	clock := &steppingClock{now: cleanupTestTime}
	repo := newFakeCleanupRepository(nil)
	repo.repeat[""] = fullPage()
	repo.onCleanup = func(cleanupCall) { clock.advance(3 * time.Second) }

	newTestCleanupRunner(repo, nil).WithClock(clock.Now).pass(t.Context())

	calls := repo.recorded()
	require.Len(t, calls, 2, "the page that crosses the 5s budget finishes; no page starts after it")
	require.Equal(t, cleanupTestTime, calls[0].now)
	require.Equal(t, cleanupTestTime.Add(3*time.Second), calls[1].now, "each page is cleaned at its own instant")
}

func TestEngineRecoveryCleanupRunnerUsesTheWholeBudgetWithoutStarvingOtherTenants(t *testing.T) {
	clock := &steppingClock{now: cleanupTestTime}
	repo := newFakeCleanupRepository(nil)
	repo.repeat["large"] = fullPage()
	repo.onCleanup = func(cleanupCall) { clock.advance(100 * time.Millisecond) }

	runner := newTestCleanupRunner(repo, nil).WithClock(clock.Now).WithTenants(staticTenants{"small", "large"})
	runner.rotation = 0 // sorted order: "large" first

	runner.pass(t.Context())

	tenants := repo.tenantsCalled()
	require.Len(t, tenants, 50, "100ms pages fill the 5s budget")
	require.Equal(t, "small", tenants[engineRecoveryCleanupPagesPerRound],
		"the small tenant takes its turn right after the large tenant's first round")
	require.NotContains(t, tenants[engineRecoveryCleanupPagesPerRound+1:], "small", "a drained tenant leaves the pass")
}

func TestEngineRecoveryCleanupRunnerIsolatesTenantFailuresAndRotatesTheFirstTenant(t *testing.T) {
	repo := newFakeCleanupRepository(nil)
	repo.pages["tenant-a"] = []cleanupPage{fullPage()}
	repo.pages["tenant-b"] = []cleanupPage{{err: errors.New("connection reset by peer")}}
	logger := &cleanupCapturingLogger{}
	runner := newTestCleanupRunner(repo, logger).WithTenants(staticTenants{"tenant-c", "tenant-a", "tenant-b"})

	runner.pass(t.Context())

	require.Equal(t, []string{"tenant-a", "tenant-a", "tenant-b", "tenant-c"}, repo.tenantsCalled(),
		"a failing tenant is skipped for the pass; the others drain")

	warnings := logger.at(libLog.LevelWarn)
	require.Len(t, warnings, 1)
	require.Equal(t, "tenant-b", warnings[0].fields["tenant_id"])

	repo.calls = nil
	runner.pass(t.Context())
	require.Equal(t, []string{"tenant-b", "tenant-c", "tenant-a"}, repo.tenantsCalled(), "the next pass starts one tenant later")
}

func TestEngineRecoveryCleanupRunnerLogsRejectedProofsOncePerTenantPass(t *testing.T) {
	first, second := errors.New("cleanup coordinator differs"), errors.New("cleanup evidence differs")
	repo := newFakeCleanupRepository(nil)
	repo.pages[""] = []cleanupPage{
		{result: txRedis.RecoveryCleanupResult{Scanned: 100, Cleaned: 97, Failed: 3, FirstFailure: first}},
		{result: txRedis.RecoveryCleanupResult{Scanned: 20, Cleaned: 18, Failed: 2, FirstFailure: second}},
	}
	logger := &cleanupCapturingLogger{}

	newTestCleanupRunner(repo, logger).pass(t.Context())

	failures := logger.at(libLog.LevelError)
	require.Len(t, failures, 1)
	require.Equal(t, "Engine recovery cleanup rejected execution proofs", failures[0].msg)
	require.EqualValues(t, 5, failures[0].fields["failed_count"])
	require.Equal(t, first, failures[0].fields["error"])
	require.NotContains(t, failures[0].fields, "tenant_id", "single-tenant logs carry no tenant label")
}

func TestEngineRecoveryCleanupRunnerElectsOnePassPerTick(t *testing.T) {
	locks := &cleanupLockStore{}
	leaderRepo, followerRepo := newFakeCleanupRepository(locks), newFakeCleanupRepository(locks)
	leader, follower := newTestCleanupRunner(leaderRepo, nil), newTestCleanupRunner(followerRepo, nil)

	// The follower's tick fires while the leader is still cleaning.
	leaderRepo.onCleanup = func(cleanupCall) { follower.pass(t.Context()) }

	leader.pass(t.Context())

	require.Len(t, leaderRepo.recorded(), 1)
	require.Empty(t, followerRepo.recorded(), "the follower skips the tick the leader holds")
	require.False(t, locks.held())

	follower.pass(t.Context())
	require.Len(t, followerRepo.recorded(), 1, "the next tick is free once the leader releases the lock")
}

func TestEngineRecoveryCleanupRunnerSkipsThePassWhenTheLockIsUnavailable(t *testing.T) {
	repo := newFakeCleanupRepository(&cleanupLockStore{err: errors.New("READONLY You can't write against a read only replica")})
	logger := &cleanupCapturingLogger{}

	newTestCleanupRunner(repo, logger).pass(t.Context())

	require.Empty(t, repo.recorded())
	require.Len(t, logger.at(libLog.LevelWarn), 1)
}

func TestEngineRecoveryCleanupRunnerSurvivesAPanickingPass(t *testing.T) {
	repo := newFakeCleanupRepository(nil)
	repo.onCleanup = func(cleanupCall) { panic("unexpected schedule state") }
	logger := &cleanupCapturingLogger{}

	require.NotPanics(t, func() { newTestCleanupRunner(repo, logger).pass(t.Context()) })
	require.Len(t, logger.at(libLog.LevelError), 1)
	require.False(t, repo.held(), "a panicking pass still releases the lock")
}

func TestInitEngineRecoveryCleanupRunnerDrainsCachedTenantsWithoutPostgres(t *testing.T) {
	repository := &cleanupRedisRepository{fakeCleanupRepository: newFakeCleanupRepository(nil)}
	cache := tenantcache.NewTenantCache()
	cache.Set("tenant-2", &tmcore.TenantConfig{ID: "tenant-2"}, time.Hour)
	cache.Set("tenant-1", &tmcore.TenantConfig{ID: "tenant-1"}, time.Hour)

	runner := initEngineRecoveryCleanupRunner(recoveryQuietLogger{}, repository, true, cache, nil)
	require.NotNil(t, runner)
	runner.WithClock(func() time.Time { return cleanupTestTime }).pass(t.Context())

	require.Equal(t, []string{"tenant-1", "tenant-2"}, repository.tenantsCalled())
}

func TestInitEngineRecoveryCleanupRunnerFallsBackToSingleTenantWithoutCache(t *testing.T) {
	repository := &cleanupRedisRepository{fakeCleanupRepository: newFakeCleanupRepository(nil)}

	runner := initEngineRecoveryCleanupRunner(recoveryQuietLogger{}, repository, true, nil, nil)
	require.NotNil(t, runner)
	runner.WithClock(func() time.Time { return cleanupTestTime }).pass(t.Context())

	require.Equal(t, []string{""}, repository.tenantsCalled())
}

func TestInitEngineRecoveryCleanupRunnerNeedsCleanupSupport(t *testing.T) {
	require.Nil(t, initEngineRecoveryCleanupRunner(recoveryQuietLogger{}, &struct{ txRedis.RedisRepository }{}, false, nil, nil),
		"a repository that cannot elect a pass gets no runner")
}

// cleanupRedisRepository is a transaction Redis repository that supports engine
// recovery cleanup, as the production adapter does.
type cleanupRedisRepository struct {
	txRedis.RedisRepository
	*fakeCleanupRepository
}

func (repo *cleanupRedisRepository) SetNX(ctx context.Context, key, value string, ttl time.Duration) (bool, error) {
	return repo.fakeCleanupRepository.SetNX(ctx, key, value, ttl)
}

func (repo *cleanupRedisRepository) DeleteIfValue(ctx context.Context, key, value string) (bool, error) {
	return repo.fakeCleanupRepository.DeleteIfValue(ctx, key, value)
}

func newCleanupMetricsReader(t *testing.T) (*sdkmetric.ManualReader, *metrics.MetricsFactory) {
	t.Helper()

	reader := sdkmetric.NewManualReader()
	provider := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	t.Cleanup(func() { _ = provider.Shutdown(context.Background()) })

	factory, err := metrics.NewMetricsFactory(provider.Meter("engine-recovery-cleanup-test"), nil)
	require.NoError(t, err)

	return reader, factory
}

// cleanupMetricPoints returns each recorded point of the named instrument, keyed
// by its attributes rendered as "key=value,..." in attribute order.
func cleanupMetricPoints(t *testing.T, reader *sdkmetric.ManualReader, name string) map[string]int64 {
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
					points[renderCleanupAttributes(point.Attributes)] = point.Value
				}
			case metricdata.Sum[int64]:
				for _, point := range data.DataPoints {
					points[renderCleanupAttributes(point.Attributes)] = point.Value
				}
			default:
				t.Fatalf("%s: unexpected data type %T", name, metric.Data)
			}
		}
	}

	return points
}

func renderCleanupAttributes(set attribute.Set) string {
	rendered := ""

	for _, kv := range set.ToSlice() {
		if rendered != "" {
			rendered += ","
		}

		rendered += string(kv.Key) + "=" + kv.Value.Emit()
	}

	return rendered
}

func TestEngineRecoveryCleanupRunnerReportsTheBacklogLeftAfterThePass(t *testing.T) {
	reader, factory := newCleanupMetricsReader(t)
	repo := newFakeCleanupRepository(nil)
	repo.backlog["tenant-a"] = cleanupBacklog{result: txRedis.RecoveryCleanupBacklog{
		Due: 7, OldestDueMs: cleanupTestTime.Add(-90 * time.Second).UnixMilli(),
	}}

	newTestCleanupRunner(repo, nil).WithTenants(staticTenants{"tenant-a", "tenant-b"}).WithMetricsFactory(factory).pass(t.Context())

	require.Equal(t, map[string]int64{"tenant_id=tenant-a": 7, "tenant_id=tenant-b": 0},
		cleanupMetricPoints(t, reader, utils.EngineRecoveryCleanupDue.Name))
	require.Equal(t, map[string]int64{"tenant_id=tenant-a": 90, "tenant_id=tenant-b": 0},
		cleanupMetricPoints(t, reader, utils.EngineRecoveryCleanupOldestOverdue.Name),
		"an empty schedule reports zero, which clears the alert")
	require.Equal(t, []cleanupCall{{tenantID: "tenant-a", now: cleanupTestTime}, {tenantID: "tenant-b", now: cleanupTestTime}}, repo.backlogs,
		"the backlog is read once per tenant, after the pass drained it")
}

func TestEngineRecoveryCleanupRunnerCountsEveryOutcome(t *testing.T) {
	reader, factory := newCleanupMetricsReader(t)
	repo := newFakeCleanupRepository(nil)
	repo.pages[""] = []cleanupPage{
		{result: txRedis.RecoveryCleanupResult{Scanned: 9, Cleaned: 5, Stale: 1, Rescheduled: 2, Failed: 1, FirstFailure: errors.New("cleanup evidence differs")}},
	}

	newTestCleanupRunner(repo, nil).WithMetricsFactory(factory).pass(t.Context())

	require.Equal(t, map[string]int64{
		"outcome=cleaned,tenant_id=":     5,
		"outcome=failed,tenant_id=":      1,
		"outcome=rescheduled,tenant_id=": 2,
		"outcome=stale,tenant_id=":       1,
	}, cleanupMetricPoints(t, reader, utils.EngineRecoveryCleanupEntries.Name))
}

func TestEngineRecoveryCleanupRunnerKeepsCleaningWhenMetricsAreUnavailable(t *testing.T) {
	t.Run("nil factory skips the backlog read", func(t *testing.T) {
		repo := newFakeCleanupRepository(nil)
		repo.pages[""] = []cleanupPage{fullPage()}

		require.NotPanics(t, func() { newTestCleanupRunner(repo, nil).pass(t.Context()) })
		require.Len(t, repo.recorded(), 2)
		require.Empty(t, repo.backlogs)
	})

	t.Run("a failed backlog read leaves the gauges unset", func(t *testing.T) {
		reader, factory := newCleanupMetricsReader(t)
		repo := newFakeCleanupRepository(nil)
		repo.backlog[""] = cleanupBacklog{err: errors.New("connection reset by peer")}
		logger := &cleanupCapturingLogger{}

		newTestCleanupRunner(repo, logger).WithMetricsFactory(factory).pass(t.Context())

		require.Len(t, repo.recorded(), 1)
		require.Empty(t, cleanupMetricPoints(t, reader, utils.EngineRecoveryCleanupDue.Name))
		require.Empty(t, logger.at(libLog.LevelWarn), "metrics are best-effort")
		require.Empty(t, logger.at(libLog.LevelError))
	})
}
