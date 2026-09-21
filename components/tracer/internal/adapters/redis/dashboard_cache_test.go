// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package redis_test

import (
	"context"
	"errors"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	tmcore "github.com/LerianStudio/lib-commons/v7/commons/tenant-manager/core"
	miniredis "github.com/alicebob/miniredis/v2"
	goredis "github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	tracerRedis "github.com/LerianStudio/midaz/v4/components/tracer/internal/adapters/redis"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/model"
)

// countingRepo is a DashboardRepository that records how many times each read
// reached it. The whole point of the cache is that this number stops growing,
// so counting calls is the assertion, not an implementation detail.
type countingRepo struct {
	metrics    atomic.Int64
	volume     atomic.Int64
	fraudTypes atomic.Int64
	topRules   atomic.Int64

	processed int64
	err       error

	// block, when non-nil, holds every read inside the repository until it is
	// closed. It is how the single-flight test gets N callers to overlap
	// without a sleep deciding whether the test passes.
	block chan struct{}

	// computeCtx records a cancellation observed INSIDE a read, if any.
	computeCtx atomic.Value
}

func (r *countingRepo) TopRules(ctx context.Context, w model.DashboardWindow) (*model.DashboardTopRules, error) {
	r.topRules.Add(1)

	if err := r.wait(ctx); err != nil {
		return nil, err
	}

	if r.err != nil {
		return nil, r.err
	}

	return &model.DashboardTopRules{
		Rules:       []model.TopRule{{Name: "high-value-wire", Matches: r.processed, Executions: r.processed}},
		WindowStart: w.From,
		WindowEnd:   w.To,
	}, nil
}

// wait blocks the caller until block is closed, if a test armed one. It
// honours ctx, because a fake that ignores cancellation makes every test about
// cancellation pass whatever the code does — the first version of this helper
// took no context and the flight-detachment test below passed against an
// implementation that had no detachment in it.
//
// A cancellation seen here is RECORDED as well as returned. That is the direct
// observation of the defect the detachment prevents: the shared computation
// being cancelled underneath the callers still waiting on it. Asserting only
// on what the stayer received would leave the test one scheduling race away
// from passing against broken code, which is how it passed 30/30 once already.
func (r *countingRepo) wait(ctx context.Context) error {
	if r.block == nil {
		return nil
	}

	select {
	case <-r.block:
		return nil
	case <-ctx.Done():
		r.computeCtx.Store(&ctxErr{err: ctx.Err()})

		return ctx.Err()
	}
}

// ctxErr boxes an error so it can live in an atomic.Value.
type ctxErr struct{ err error }

// computeCtxErr reports the cancellation the shared computation observed, or
// nil when it ran to completion on a live context.
func (r *countingRepo) computeCtxErr() error {
	if boxed, ok := r.computeCtx.Load().(*ctxErr); ok {
		return boxed.err
	}

	return nil
}

func (r *countingRepo) Metrics(ctx context.Context, w model.DashboardWindow) (*model.DashboardMetrics, error) {
	r.metrics.Add(1)

	if err := r.wait(ctx); err != nil {
		return nil, err
	}

	if r.err != nil {
		return nil, r.err
	}

	return &model.DashboardMetrics{
		TransactionsProcessed: r.processed,
		WindowStart:           w.From,
		WindowEnd:             w.To,
		AmountSavedByAsset:    []model.AssetAmount{},
	}, nil
}

func (r *countingRepo) Volume(ctx context.Context, w model.DashboardWindow) (*model.DashboardVolume, error) {
	r.volume.Add(1)

	if err := r.wait(ctx); err != nil {
		return nil, err
	}

	if r.err != nil {
		return nil, r.err
	}

	return &model.DashboardVolume{
		Points:      []model.VolumePoint{{Date: "2026-09-20", Volume: r.processed}},
		WindowStart: w.From,
		WindowEnd:   w.To,
	}, nil
}

func (r *countingRepo) FraudTypes(ctx context.Context, w model.DashboardWindow) (*model.DashboardFraudTypes, error) {
	r.fraudTypes.Add(1)

	if err := r.wait(ctx); err != nil {
		return nil, err
	}

	if r.err != nil {
		return nil, r.err
	}

	return &model.DashboardFraudTypes{
		Types:        []model.FraudTypeSlice{{Type: "CARD", Count: r.processed}},
		TotalFlagged: r.processed,
		WindowStart:  w.From,
		WindowEnd:    w.To,
	}, nil
}

// newCache wires a cache over miniredis and returns both so a test can drive
// the clock forward and inspect the stored keys.
func newCache(t *testing.T, inner *countingRepo, ttl time.Duration) (*tracerRedis.DashboardCache, *miniredis.Miniredis) {
	t.Helper()

	server := miniredis.RunT(t)
	client := goredis.NewClient(&goredis.Options{Addr: server.Addr()})
	t.Cleanup(func() { _ = client.Close() })

	return tracerRedis.NewDashboardCache(inner, client, ttl, nil), server
}

func testWindow(t *testing.T, period string) model.DashboardWindow {
	t.Helper()

	window, err := model.NewDashboardWindow(period, "", "",
		time.Date(2026, 9, 20, 14, 30, 12, 0, time.UTC))
	require.NoError(t, err)

	return window
}

func TestDashboardCache_ComputesOnceThenServesFromCache(t *testing.T) {
	t.Parallel()

	repo := &countingRepo{processed: 4711}
	cache, _ := newCache(t, repo, time.Minute)
	ctx := context.Background()
	window := testWindow(t, "30d")

	first, err := cache.Metrics(ctx, window)
	require.NoError(t, err)

	second, err := cache.Metrics(ctx, window)
	require.NoError(t, err)

	assert.EqualValues(t, 1, repo.metrics.Load(), "the second read must not reach the database")
	assert.Equal(t, first.TransactionsProcessed, second.TransactionsProcessed)
	assert.Equal(t, first.WindowStart.UTC(), second.WindowStart.UTC())
}

// Every endpoint caches, and each one caches separately: a metrics entry must
// never be served as a volume answer.
func TestDashboardCache_EndpointsAreSeparateEntries(t *testing.T) {
	t.Parallel()

	repo := &countingRepo{processed: 9}
	cache, server := newCache(t, repo, time.Minute)
	ctx := context.Background()
	window := testWindow(t, "7d")

	for range 2 {
		_, err := cache.Metrics(ctx, window)
		require.NoError(t, err)

		_, err = cache.Volume(ctx, window)
		require.NoError(t, err)

		_, err = cache.FraudTypes(ctx, window)
		require.NoError(t, err)
	}

	assert.EqualValues(t, 1, repo.metrics.Load())
	assert.EqualValues(t, 1, repo.volume.Load())
	assert.EqualValues(t, 1, repo.fraudTypes.Load())
	assert.Len(t, server.Keys(), 3, "one key per endpoint, not one shared key")
}

func TestDashboardCache_WindowsAreSeparateEntries(t *testing.T) {
	t.Parallel()

	repo := &countingRepo{processed: 3}
	cache, _ := newCache(t, repo, time.Minute)
	ctx := context.Background()

	for _, period := range []string{"7d", "30d", "90d"} {
		_, err := cache.Metrics(ctx, testWindow(t, period))
		require.NoError(t, err)
	}

	assert.EqualValues(t, 3, repo.metrics.Load(), "each window is its own question")
}

func TestDashboardCache_EntryExpiresAtTTL(t *testing.T) {
	t.Parallel()

	repo := &countingRepo{processed: 1}
	cache, server := newCache(t, repo, time.Minute)
	ctx := context.Background()
	window := testWindow(t, "30d")

	_, err := cache.Metrics(ctx, window)
	require.NoError(t, err)

	server.FastForward(59 * time.Second)

	_, err = cache.Metrics(ctx, window)
	require.NoError(t, err)
	assert.EqualValues(t, 1, repo.metrics.Load(), "still inside the TTL")

	server.FastForward(2 * time.Second)

	_, err = cache.Metrics(ctx, window)
	require.NoError(t, err)
	assert.EqualValues(t, 2, repo.metrics.Load(), "past the TTL the figures are recomputed")
}

// Tenant isolation is the one the money depends on: two tenants asking the same
// question in the same window must never share an entry.
func TestDashboardCache_TenantsDoNotShareEntries(t *testing.T) {
	t.Parallel()

	repo := &countingRepo{processed: 42}
	cache, server := newCache(t, repo, time.Minute)
	window := testWindow(t, "30d")

	alpha := tmcore.ContextWithTenantID(context.Background(), "tenant-alpha")
	beta := tmcore.ContextWithTenantID(context.Background(), "tenant-beta")

	_, err := cache.Metrics(alpha, window)
	require.NoError(t, err)

	_, err = cache.Metrics(beta, window)
	require.NoError(t, err)

	assert.EqualValues(t, 2, repo.metrics.Load(), "tenant-beta must not read tenant-alpha's entry")

	keys := server.Keys()
	require.Len(t, keys, 2)

	for _, key := range keys {
		assert.Contains(t, key, "tracer:dashboard:metrics")
	}

	assert.Condition(t, func() bool {
		var alphaSeen, betaSeen bool
		for _, key := range keys {
			alphaSeen = alphaSeen || strings.Contains(key, "tenant-alpha")
			betaSeen = betaSeen || strings.Contains(key, "tenant-beta")
		}

		return alphaSeen && betaSeen
	}, "each key must name its own tenant: %v", keys)
}

// Re-reading as the same tenant hits; the isolation above must not be bought by
// making every read a miss.
func TestDashboardCache_SameTenantHits(t *testing.T) {
	t.Parallel()

	repo := &countingRepo{processed: 42}
	cache, _ := newCache(t, repo, time.Minute)
	ctx := tmcore.ContextWithTenantID(context.Background(), "tenant-alpha")
	window := testWindow(t, "30d")

	for range 3 {
		_, err := cache.Metrics(ctx, window)
		require.NoError(t, err)
	}

	assert.EqualValues(t, 1, repo.metrics.Load())
}

// A nil client is single-tenant mode, where Tracer builds no Valkey connection.
// The dashboard must still answer.
func TestDashboardCache_NilClientPassesThrough(t *testing.T) {
	t.Parallel()

	repo := &countingRepo{processed: 7}
	cache := tracerRedis.NewDashboardCache(repo, nil, time.Minute, nil)
	ctx := context.Background()
	window := testWindow(t, "30d")

	for range 2 {
		metrics, err := cache.Metrics(ctx, window)
		require.NoError(t, err)
		assert.EqualValues(t, 7, metrics.TransactionsProcessed)
	}

	assert.EqualValues(t, 2, repo.metrics.Load(), "no cache means every read computes")
}

// A dead Valkey costs database time, never an error: the cache exists to keep
// load off the database, so losing it degrades to the pre-cache behaviour.
func TestDashboardCache_ValkeyOutageStillAnswers(t *testing.T) {
	t.Parallel()

	repo := &countingRepo{processed: 5}
	cache, server := newCache(t, repo, time.Minute)
	ctx := context.Background()
	window := testWindow(t, "30d")

	server.Close()

	metrics, err := cache.Metrics(ctx, window)
	require.NoError(t, err, "a Valkey outage must not fail the dashboard")
	assert.EqualValues(t, 5, metrics.TransactionsProcessed)
}

// A repository error is NOT swallowed, and nothing is stored for it: caching a
// failure would serve it for the whole TTL.
func TestDashboardCache_RepositoryErrorIsNotCached(t *testing.T) {
	t.Parallel()

	wantErr := errors.New("database unavailable")
	repo := &countingRepo{err: wantErr}
	cache, server := newCache(t, repo, time.Minute)
	ctx := context.Background()
	window := testWindow(t, "30d")

	_, err := cache.Metrics(ctx, window)
	require.ErrorIs(t, err, wantErr)

	assert.Empty(t, server.Keys(), "a failure must leave no entry behind")

	_, err = cache.Metrics(ctx, window)
	require.ErrorIs(t, err, wantErr)
	assert.EqualValues(t, 2, repo.metrics.Load(), "the next caller retries rather than reading a cached error")
}

// A stored entry that no longer decodes (an older shape left over across a
// deploy) is a miss, not a 500.
func TestDashboardCache_UndecodableEntryIsAMiss(t *testing.T) {
	t.Parallel()

	repo := &countingRepo{processed: 11}
	cache, server := newCache(t, repo, time.Minute)
	ctx := context.Background()
	window := testWindow(t, "30d")

	_, err := cache.Metrics(ctx, window)
	require.NoError(t, err)

	keys := server.Keys()
	require.Len(t, keys, 1)
	require.NoError(t, server.Set(keys[0], "}{ not json"))

	metrics, err := cache.Metrics(ctx, window)
	require.NoError(t, err)
	assert.EqualValues(t, 11, metrics.TransactionsProcessed)
	assert.EqualValues(t, 2, repo.metrics.Load())
}

// A zero TTL means "use the default", never "never expire".
func TestDashboardCache_ZeroTTLUsesDefault(t *testing.T) {
	t.Parallel()

	repo := &countingRepo{processed: 1}
	cache, server := newCache(t, repo, 0)
	ctx := context.Background()
	window := testWindow(t, "30d")

	_, err := cache.Metrics(ctx, window)
	require.NoError(t, err)

	keys := server.Keys()
	require.Len(t, keys, 1)
	assert.Equal(t, tracerRedis.DefaultTTL, server.TTL(keys[0]))
}

// getCounter counts completed Valkey reads, so the coalescing test below can
// wait for a fact instead of for a duration.
type getCounter struct{ gets atomic.Int64 }

func (h *getCounter) DialHook(next goredis.DialHook) goredis.DialHook { return next }

func (h *getCounter) ProcessPipelineHook(next goredis.ProcessPipelineHook) goredis.ProcessPipelineHook {
	return next
}

func (h *getCounter) ProcessHook(next goredis.ProcessHook) goredis.ProcessHook {
	return func(ctx context.Context, cmd goredis.Cmder) error {
		err := next(ctx, cmd)

		if cmd.Name() == "get" {
			h.gets.Add(1)
		}

		return err
	}
}

// countReads attaches a getCounter to client and returns a func reporting how
// many Valkey reads have completed. Both concurrency tests below need to wait
// on "every caller has finished its cache read", which is a fact rather than a
// duration.
func countReads(client *goredis.Client) func() int64 {
	hook := &getCounter{}
	client.AddHook(hook)

	return hook.gets.Load
}

// TestDashboardCacheCoalescesConcurrentMisses pins the behaviour that makes
// the cache protect the database rather than merely accelerate it.
//
// Every tenant's key rotates on the same wall-clock minute, because the window
// is truncated to the minute. So the moment an entry expires, every dashboard
// panel open anywhere misses at once, and without coalescing each of those
// misses runs its own aggregation over the window. On the /top-rules panel
// that is 146 ms of database work per concurrent viewer, every minute, on a
// query that reads ~92 MB of heap — the exact load on the database the cache
// was introduced to avoid.
//
// Getting this deterministic takes two barriers, and the second one is the
// subtle one. The first caller is let in ALONE and held inside the repository,
// so the flight is registered before anyone else arrives. But a latecomer is
// not safely on the flight the moment it is scheduled: it first reads Valkey,
// and releasing the flight during that read lets it arrive to find the key
// already gone and start a second computation. So the test also waits until
// every caller's Valkey read has COMPLETED before releasing the repository.
// Waiting only for the goroutines to start made this fail about one run in ten
// under a full-suite load, reporting 2.
func TestDashboardCacheCoalescesConcurrentMisses(t *testing.T) {
	const callers = 24

	server := miniredis.RunT(t)
	client := goredis.NewClient(&goredis.Options{Addr: server.Addr()})
	reads := countReads(client)

	t.Cleanup(func() { _ = client.Close() })

	repo := &countingRepo{processed: 42, block: make(chan struct{})}
	cache := tracerRedis.NewDashboardCache(repo, client, time.Minute, nil)
	ctx := tmcore.ContextWithTenantID(context.Background(), "tenant-coalesce")
	window := testWindow(t, "30d")

	var (
		wg      sync.WaitGroup
		failure atomic.Value
	)

	call := func() {
		defer wg.Done()

		if _, err := cache.TopRules(ctx, window); err != nil {
			failure.Store(err)
		}
	}

	// Barrier 1: caller 1 opens the flight and blocks inside the repository.
	// singleflight registers the key before invoking the function, so once the
	// repository has been entered the flight is joinable.
	wg.Add(1)

	go call()

	require.Eventually(t, func() bool { return repo.topRules.Load() == 1 }, 10*time.Second, time.Millisecond,
		"the first caller never reached the repository")

	wg.Add(callers - 1)

	for range callers - 1 {
		go call()
	}

	// Barrier 2: every caller has finished its Valkey read and has nothing left
	// to do but join the flight, which is a mutex acquisition rather than a
	// round trip.
	require.Eventually(t, func() bool { return reads() == callers }, 10*time.Second, time.Millisecond,
		"not every caller reached the cache read")

	close(repo.block)
	wg.Wait()

	if err, ok := failure.Load().(error); ok {
		require.NoError(t, err)
	}

	assert.Equal(t, int64(1), repo.topRules.Load(),
		"%d callers arriving during an open flight must reach the database once, not %d times",
		callers, repo.topRules.Load())
}

// TestDashboardCacheFlightSurvivesTheCallerThatOpenedIt pins the half of
// coalescing that is easy to get wrong.
//
// Sharing one computation between callers means sharing its fate. If the
// computation runs on anything the first caller owns, that caller closing its
// browser tab kills the database read every other viewer is blocked on, and a
// dozen dashboards error for a request that was doing fine. Coalescing would
// then have introduced exactly the outage it exists to prevent.
//
// **The timing here is the test.** An earlier version cancelled the winner and
// released the repository in the same instant, which left both arms of the
// fake's select ready together; the release always won the race and the test
// passed 30 times out of 30 against code that had the defect. So the winner
// hangs up and the repository stays SHUT for a further beat, which is the only
// arrangement under which a cancellation reaching the computation can be seen.
func TestDashboardCacheFlightSurvivesTheCallerThatOpenedIt(t *testing.T) {
	// How long the database stays shut after the winner hangs up. Long enough
	// that a cancellation delivered to the computation is observed and recorded
	// before the answer can be produced.
	const beat = 300 * time.Millisecond

	server := miniredis.RunT(t)
	client := goredis.NewClient(&goredis.Options{Addr: server.Addr()})
	reads := countReads(client)

	t.Cleanup(func() { _ = client.Close() })

	repo := &countingRepo{processed: 7, block: make(chan struct{})}
	cache := tracerRedis.NewDashboardCache(repo, client, time.Minute, nil)
	window := testWindow(t, "30d")

	base := tmcore.ContextWithTenantID(context.Background(), "tenant-detached")
	leaverCtx, leave := context.WithCancel(base)

	var (
		wg           sync.WaitGroup
		leaverErr    error
		stayerAnswer *model.DashboardTopRules
		stayerErr    error
	)

	// The leaver opens the flight and blocks inside the repository.
	wg.Add(1)

	go func() {
		defer wg.Done()

		_, leaverErr = cache.TopRules(leaverCtx, window)
	}()

	require.Eventually(t, func() bool { return repo.topRules.Load() == 1 }, 10*time.Second, time.Millisecond,
		"the leaver never reached the repository")

	// The stayer joins the open flight.
	wg.Add(1)

	go func() {
		defer wg.Done()

		stayerAnswer, stayerErr = cache.TopRules(base, window)
	}()

	require.Eventually(t, func() bool { return reads() >= 2 }, 10*time.Second, time.Millisecond,
		"the stayer never reached the cache read")

	// The leaver hangs up, and the database stays shut afterwards. A
	// cancellation that reaches the computation has this whole beat to land.
	leave()
	time.Sleep(beat)
	close(repo.block)
	wg.Wait()

	require.ErrorIs(t, leaverErr, context.Canceled, "the caller that hung up gets its own cancellation")
	require.NoError(t, repo.computeCtxErr(), "the shared computation must not observe a cancelled context")
	require.NoError(t, stayerErr, "a caller whose context is alive must not inherit another caller's cancellation")
	require.NotNil(t, stayerAnswer)
	assert.Equal(t, int64(7), stayerAnswer.Rules[0].Matches)
}
