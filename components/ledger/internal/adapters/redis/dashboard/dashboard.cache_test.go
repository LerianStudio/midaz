// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package dashboard_test

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	tmcore "github.com/LerianStudio/lib-commons/v7/commons/tenant-manager/core"
	"github.com/alicebob/miniredis/v2"
	"github.com/google/uuid"
	goredis "github.com/redis/go-redis/v9"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	postgresDashboard "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/dashboard"
	redisDashboard "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/redis/dashboard"
	"github.com/LerianStudio/midaz/v4/pkg/dashboard"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
)

var (
	orgA    = uuid.MustParse("11111111-1111-1111-1111-111111111111")
	ledgerA = uuid.MustParse("22222222-2222-2222-2222-222222222222")
	ledgerB = uuid.MustParse("33333333-3333-3333-3333-333333333333")

	testWindow = dashboard.Window{
		From:   time.Date(2026, 8, 21, 14, 31, 0, 0, time.UTC),
		To:     time.Date(2026, 9, 20, 14, 31, 0, 0, time.UTC),
		Period: "30d",
	}
)

// countingRepo is the inner repository. It records how many times it was asked
// to compute, which is the only way to tell a cache HIT from a cache MISS that
// happened to return the same value.
type countingRepo struct {
	mu sync.Mutex

	metricsCalls int
	volumeCalls  int
	assetsCalls  int

	// amount is echoed into every answer so a test can change it and prove a
	// second read came from the cache rather than from here.
	amount string
	err    error

	// gate, when non-nil, blocks every compute until it is closed. It is how
	// the singleflight test holds several callers inside one flight.
	gate chan struct{}
}

var _ postgresDashboard.Repository = (*countingRepo)(nil)

func (r *countingRepo) Metrics(_ context.Context, _, _ uuid.UUID, window dashboard.Window) (*mmodel.DashboardMetrics, error) {
	r.enter()

	r.mu.Lock()
	r.metricsCalls++
	amount := r.amount
	err := r.err
	r.mu.Unlock()

	if err != nil {
		return nil, err
	}

	return &mmodel.DashboardMetrics{
		Total:         7,
		ByStatus:      map[string]int64{"APPROVED": 7},
		VolumeByAsset: []mmodel.DashboardAssetVolume{{Asset: "BRL", Amount: decimal.RequireFromString(amount), Transactions: 7}},
		WindowStart:   window.From,
		WindowEnd:     window.To,
	}, nil
}

func (r *countingRepo) Volume(_ context.Context, _, _ uuid.UUID, window dashboard.Window) (*mmodel.DashboardVolume, error) {
	r.enter()

	r.mu.Lock()
	r.volumeCalls++
	amount := r.amount
	err := r.err
	r.mu.Unlock()

	if err != nil {
		return nil, err
	}

	return &mmodel.DashboardVolume{
		Points: []mmodel.DashboardVolumePoint{{
			Date:         "2026-09-20",
			Transactions: 3,
			ByAsset:      []mmodel.DashboardAssetVolume{{Asset: "BRL", Amount: decimal.RequireFromString(amount), Transactions: 3}},
		}},
		WindowStart: window.From,
		WindowEnd:   window.To,
	}, nil
}

func (r *countingRepo) Assets(_ context.Context, _, _ uuid.UUID) (*mmodel.DashboardAssets, error) {
	r.enter()

	r.mu.Lock()
	r.assetsCalls++
	amount := r.amount
	err := r.err
	r.mu.Unlock()

	if err != nil {
		return nil, err
	}

	return &mmodel.DashboardAssets{
		Assets: []mmodel.DashboardAssetPosition{{
			Asset: "BRL", Accounts: 2,
			Available: decimal.RequireFromString(amount),
			OnHold:    decimal.Zero,
		}},
	}, nil
}

func (r *countingRepo) enter() {
	r.mu.Lock()
	gate := r.gate
	r.mu.Unlock()

	if gate != nil {
		<-gate
	}
}

func (r *countingRepo) calls() (int, int, int) {
	r.mu.Lock()
	defer r.mu.Unlock()

	return r.metricsCalls, r.volumeCalls, r.assetsCalls
}

// staticProvider hands out one client, which is what a single-tenant
// deployment's provider does.
type staticProvider struct{ client goredis.UniversalClient }

func (p staticProvider) GetClient(context.Context) (goredis.UniversalClient, error) {
	return p.client, nil
}

// failingProvider is a provider that cannot reach Valkey at all.
type failingProvider struct{}

func (failingProvider) GetClient(context.Context) (goredis.UniversalClient, error) {
	return nil, errors.New("valkey unreachable")
}

// newCache stands up miniredis and a cache over inner. The returned context
// carries a tenant id, because the key builder resolves the tenant BEFORE it
// reads or writes anything.
func newCache(t *testing.T, inner postgresDashboard.Repository) (*redisDashboard.DashboardCache, *miniredis.Miniredis, context.Context) {
	t.Helper()

	server := miniredis.RunT(t)
	client := goredis.NewClient(&goredis.Options{Addr: server.Addr()})

	t.Cleanup(func() { _ = client.Close() })

	cache := redisDashboard.NewDashboardCache(inner, staticProvider{client}, 0, nil)
	ctx := tmcore.ContextWithTenantID(context.Background(), "tenant-one")

	return cache, server, ctx
}

// =============================================================================
// get-or-compute
// =============================================================================

func TestDashboardCache_ComputesOnMissAndServesOnHit(t *testing.T) {
	inner := &countingRepo{amount: "100.00"}
	cache, _, ctx := newCache(t, inner)

	first, err := cache.Metrics(ctx, orgA, ledgerA, testWindow)
	require.NoError(t, err)
	assert.True(t, decimal.RequireFromString("100.00").Equal(first.VolumeByAsset[0].Amount))

	// Change what the inner repository would answer. A second read that still
	// reports 100.00 can only have come from the cache.
	inner.mu.Lock()
	inner.amount = "999.99"
	inner.mu.Unlock()

	second, err := cache.Metrics(ctx, orgA, ledgerA, testWindow)
	require.NoError(t, err)
	assert.True(t, decimal.RequireFromString("100.00").Equal(second.VolumeByAsset[0].Amount),
		"the second read must be served from cache, got %s", second.VolumeByAsset[0].Amount)

	metrics, _, _ := inner.calls()
	assert.Equal(t, 1, metrics, "one compute for two reads")
}

func TestDashboardCache_AllThreeEndpointsCacheIndependently(t *testing.T) {
	inner := &countingRepo{amount: "100.00"}
	cache, server, ctx := newCache(t, inner)

	for i := 0; i < 2; i++ {
		_, err := cache.Metrics(ctx, orgA, ledgerA, testWindow)
		require.NoError(t, err)

		_, err = cache.Volume(ctx, orgA, ledgerA, testWindow)
		require.NoError(t, err)

		_, err = cache.Assets(ctx, orgA, ledgerA)
		require.NoError(t, err)
	}

	metrics, volume, assets := inner.calls()
	assert.Equal(t, 1, metrics)
	assert.Equal(t, 1, volume)
	assert.Equal(t, 1, assets)

	assert.Len(t, server.Keys(), 3, "three endpoints, three entries, never one shared")
}

func TestDashboardCache_ExpiresAtTTL(t *testing.T) {
	inner := &countingRepo{amount: "100.00"}
	cache, server, ctx := newCache(t, inner)

	_, err := cache.Metrics(ctx, orgA, ledgerA, testWindow)
	require.NoError(t, err)

	for _, key := range server.Keys() {
		assert.Equal(t, redisDashboard.DefaultTTL, server.TTL(key),
			"the entry lives exactly one window granularity")
	}

	server.FastForward(redisDashboard.DefaultTTL + time.Second)

	_, err = cache.Metrics(ctx, orgA, ledgerA, testWindow)
	require.NoError(t, err)

	metrics, _, _ := inner.calls()
	assert.Equal(t, 2, metrics, "an expired entry recomputes")
}

func TestDashboardCache_HonoursExplicitTTL(t *testing.T) {
	inner := &countingRepo{amount: "100.00"}
	server := miniredis.RunT(t)
	client := goredis.NewClient(&goredis.Options{Addr: server.Addr()})

	t.Cleanup(func() { _ = client.Close() })

	cache := redisDashboard.NewDashboardCache(inner, staticProvider{client}, 5*time.Second, nil)
	ctx := tmcore.ContextWithTenantID(context.Background(), "tenant-one")

	_, err := cache.Metrics(ctx, orgA, ledgerA, testWindow)
	require.NoError(t, err)

	for _, key := range server.Keys() {
		assert.Equal(t, 5*time.Second, server.TTL(key))
	}
}

// =============================================================================
// key shape — the isolation the ledger needs and the tracer did not
// =============================================================================

func TestDashboardCache_KeyCarriesTenantOrgLedgerEndpointAndWindow(t *testing.T) {
	inner := &countingRepo{amount: "100.00"}
	cache, server, ctx := newCache(t, inner)

	_, err := cache.Metrics(ctx, orgA, ledgerA, testWindow)
	require.NoError(t, err)

	keys := server.Keys()
	require.Len(t, keys, 1)

	key := keys[0]

	assert.Contains(t, key, "tenant-one", "the tenant prefix must be present")
	assert.Contains(t, key, "midaz:dashboard")
	assert.Contains(t, key, "metrics")
	assert.Contains(t, key, orgA.String(), "the organization must be in the key")
	assert.Contains(t, key, ledgerA.String(), "the ledger must be in the key")
	assert.Contains(t, key, testWindow.CacheKey(), "the window must be in the key")
}

// TestDashboardCache_TwoLedgersInOneTenantNeverShareAnEntry is the failure this
// key shape exists to prevent. A tenant-only key — which is all the tracer
// needs, because its window IS the whole query — would serve one ledger's money
// figures to another ledger's dashboard inside the same organization.
func TestDashboardCache_TwoLedgersInOneTenantNeverShareAnEntry(t *testing.T) {
	inner := &countingRepo{amount: "100.00"}
	cache, server, ctx := newCache(t, inner)

	_, err := cache.Metrics(ctx, orgA, ledgerA, testWindow)
	require.NoError(t, err)

	inner.mu.Lock()
	inner.amount = "777.00"
	inner.mu.Unlock()

	other, err := cache.Metrics(ctx, orgA, ledgerB, testWindow)
	require.NoError(t, err)

	assert.True(t, decimal.RequireFromString("777.00").Equal(other.VolumeByAsset[0].Amount),
		"ledger B must compute its own answer, got %s", other.VolumeByAsset[0].Amount)

	metrics, _, _ := inner.calls()
	assert.Equal(t, 2, metrics, "two ledgers are two computations")
	assert.Len(t, server.Keys(), 2, "two ledgers are two entries")
}

func TestDashboardCache_TwoTenantsNeverShareAnEntry(t *testing.T) {
	inner := &countingRepo{amount: "100.00"}
	cache, server, _ := newCache(t, inner)

	one := tmcore.ContextWithTenantID(context.Background(), "tenant-one")
	two := tmcore.ContextWithTenantID(context.Background(), "tenant-two")

	_, err := cache.Metrics(one, orgA, ledgerA, testWindow)
	require.NoError(t, err)

	_, err = cache.Metrics(two, orgA, ledgerA, testWindow)
	require.NoError(t, err)

	metrics, _, _ := inner.calls()
	assert.Equal(t, 2, metrics)
	assert.Len(t, server.Keys(), 2)
}

func TestDashboardCache_DifferentWindowsAreDifferentEntries(t *testing.T) {
	inner := &countingRepo{amount: "100.00"}
	cache, server, ctx := newCache(t, inner)

	sevenDay := dashboard.Window{From: testWindow.To.Add(-7 * 24 * time.Hour), To: testWindow.To, Period: "7d"}

	_, err := cache.Metrics(ctx, orgA, ledgerA, testWindow)
	require.NoError(t, err)

	_, err = cache.Metrics(ctx, orgA, ledgerA, sevenDay)
	require.NoError(t, err)

	metrics, _, _ := inner.calls()
	assert.Equal(t, 2, metrics)
	assert.Len(t, server.Keys(), 2)
}

// TestDashboardCache_AssetsKeyCarriesNoWindow: /assets ignores the window, so
// its entry must not be keyed by one — a window in the key would mean a
// position recomputed for every distinct period a viewer happened to select,
// answering the same question several times over.
func TestDashboardCache_AssetsKeyCarriesNoWindow(t *testing.T) {
	inner := &countingRepo{amount: "100.00"}
	cache, server, ctx := newCache(t, inner)

	_, err := cache.Assets(ctx, orgA, ledgerA)
	require.NoError(t, err)

	keys := server.Keys()
	require.Len(t, keys, 1)

	assert.NotContains(t, keys[0], testWindow.CacheKey())
	assert.NotContains(t, keys[0], "p=", "no period fragment belongs in the assets key")
	assert.Contains(t, keys[0], ledgerA.String())
}

// =============================================================================
// degradation — a dashboard with no cache is slower, never wrong
// =============================================================================

func TestDashboardCache_NilProviderPassesThrough(t *testing.T) {
	inner := &countingRepo{amount: "100.00"}
	cache := redisDashboard.NewDashboardCache(inner, nil, 0, nil)
	ctx := tmcore.ContextWithTenantID(context.Background(), "tenant-one")

	for i := 0; i < 3; i++ {
		result, err := cache.Metrics(ctx, orgA, ledgerA, testWindow)
		require.NoError(t, err)
		assert.Equal(t, int64(7), result.Total)
	}

	metrics, _, _ := inner.calls()
	assert.Equal(t, 3, metrics, "with no cache every read computes")
}

func TestDashboardCache_NilProviderPassesThroughOnEveryEndpoint(t *testing.T) {
	inner := &countingRepo{amount: "100.00"}
	cache := redisDashboard.NewDashboardCache(inner, nil, 0, nil)
	ctx := context.Background()

	_, err := cache.Volume(ctx, orgA, ledgerA, testWindow)
	require.NoError(t, err)

	_, err = cache.Assets(ctx, orgA, ledgerA)
	require.NoError(t, err)

	_, volume, assets := inner.calls()
	assert.Equal(t, 1, volume)
	assert.Equal(t, 1, assets)
}

// TestDashboardCache_ValkeyDownStillAnswers: a Valkey that cannot be reached
// costs database time, which is exactly the thing the cache was buying. It
// never costs the caller an error.
func TestDashboardCache_ValkeyDownStillAnswers(t *testing.T) {
	inner := &countingRepo{amount: "100.00"}
	server := miniredis.RunT(t)
	client := goredis.NewClient(&goredis.Options{Addr: server.Addr()})

	t.Cleanup(func() { _ = client.Close() })

	cache := redisDashboard.NewDashboardCache(inner, staticProvider{client}, 0, nil)
	ctx := tmcore.ContextWithTenantID(context.Background(), "tenant-one")

	server.Close()

	result, err := cache.Metrics(ctx, orgA, ledgerA, testWindow)
	require.NoError(t, err, "a dead cache must not fail the read")
	assert.Equal(t, int64(7), result.Total)

	metrics, _, _ := inner.calls()
	assert.Equal(t, 1, metrics)
}

// TestDashboardCache_UndecodableEntryRecomputes: a stored entry that no longer
// decodes — an older shape left behind by a deploy — is a miss, not a 500.
func TestDashboardCache_UndecodableEntryRecomputes(t *testing.T) {
	inner := &countingRepo{amount: "100.00"}
	cache, server, ctx := newCache(t, inner)

	_, err := cache.Metrics(ctx, orgA, ledgerA, testWindow)
	require.NoError(t, err)

	keys := server.Keys()
	require.Len(t, keys, 1)
	require.NoError(t, server.Set(keys[0], "}{ not json"))

	result, err := cache.Metrics(ctx, orgA, ledgerA, testWindow)
	require.NoError(t, err)
	assert.Equal(t, int64(7), result.Total)

	metrics, _, _ := inner.calls()
	assert.Equal(t, 2, metrics)
}

// TestDashboardCache_ComputeErrorIsReturnedAndNotCached.
func TestDashboardCache_ComputeErrorIsReturnedAndNotCached(t *testing.T) {
	boom := errors.New("database is unhappy")
	inner := &countingRepo{amount: "100.00", err: boom}
	cache, server, ctx := newCache(t, inner)

	_, err := cache.Metrics(ctx, orgA, ledgerA, testWindow)
	require.ErrorIs(t, err, boom)

	assert.Empty(t, server.Keys(), "a failed computation stores nothing")
}

// TestDashboardCache_MissingTenantStillAnswers: a context with no tenant id
// cannot produce a prefixed key. The read computes and caches nothing rather
// than guessing a prefix it might share with somebody.
func TestDashboardCache_MissingTenantStillAnswers(t *testing.T) {
	inner := &countingRepo{amount: "100.00"}
	cache, server, _ := newCache(t, inner)

	result, err := cache.Metrics(context.Background(), orgA, ledgerA, testWindow)
	require.NoError(t, err)
	assert.Equal(t, int64(7), result.Total)

	for _, key := range server.Keys() {
		assert.NotContains(t, key, "tenant-one", "no other tenant's prefix may be borrowed")
	}
}

// =============================================================================
// coalescing
// =============================================================================

// TestDashboardCache_CoalescesConcurrentMisses: every entry in this family
// expires on the same wall-clock minute, because the window is truncated to
// one. Without coalescing, N open dashboards are N simultaneous aggregations
// in the same instant, every minute, forever.
func TestDashboardCache_CoalescesConcurrentMisses(t *testing.T) {
	gate := make(chan struct{})
	inner := &countingRepo{amount: "100.00", gate: gate}
	cache, _, ctx := newCache(t, inner)

	const viewers = 12

	var (
		wg      sync.WaitGroup
		results = make([]*mmodel.DashboardMetrics, viewers)
		errs    = make([]error, viewers)
	)

	wg.Add(viewers)

	for i := 0; i < viewers; i++ {
		go func(i int) {
			defer wg.Done()

			results[i], errs[i] = cache.Metrics(ctx, orgA, ledgerA, testWindow)
		}(i)
	}

	// Let every goroutine reach the flight before any compute may finish.
	assert.Eventually(t, func() bool {
		return len(inner.gate) == 0
	}, time.Second, 5*time.Millisecond)

	time.Sleep(50 * time.Millisecond)
	close(gate)
	wg.Wait()

	for i := 0; i < viewers; i++ {
		require.NoError(t, errs[i])
		require.NotNil(t, results[i])
		assert.Equal(t, int64(7), results[i].Total)
	}

	metrics, _, _ := inner.calls()
	assert.Equal(t, 1, metrics, "twelve viewers, one aggregation")
}

// TestDashboardCache_OneCallerHangingUpDoesNotCancelTheOthers. The flight runs
// on a context detached from the request that opened it, so the first viewer to
// close a tab cannot cancel the query every other viewer is waiting on.
func TestDashboardCache_OneCallerHangingUpDoesNotCancelTheOthers(t *testing.T) {
	gate := make(chan struct{})
	inner := &countingRepo{amount: "100.00", gate: gate}
	cache, _, base := newCache(t, inner)

	leaver, cancel := context.WithCancel(base)

	var (
		wg        sync.WaitGroup
		stayerErr error
		stayer    *mmodel.DashboardMetrics
		leaverErr error
	)

	wg.Add(2)

	go func() {
		defer wg.Done()

		_, leaverErr = cache.Metrics(leaver, orgA, ledgerA, testWindow)
	}()

	go func() {
		defer wg.Done()

		stayer, stayerErr = cache.Metrics(base, orgA, ledgerA, testWindow)
	}()

	time.Sleep(50 * time.Millisecond)
	cancel()
	time.Sleep(20 * time.Millisecond)
	close(gate)
	wg.Wait()

	require.NoError(t, stayerErr, "the viewer who stayed must still get an answer")
	require.NotNil(t, stayer)
	assert.Equal(t, int64(7), stayer.Total)

	if leaverErr != nil {
		assert.True(t, errors.Is(leaverErr, context.Canceled))
	}
}

// TestDashboardCache_KeyIsStableAcrossReads guards the one thing a typo in the
// key builder would silently break: two identical reads must produce the same
// key, or the cache is a write-only store with a 100% miss rate.
func TestDashboardCache_KeyIsStableAcrossReads(t *testing.T) {
	inner := &countingRepo{amount: "100.00"}
	cache, server, ctx := newCache(t, inner)

	for i := 0; i < 5; i++ {
		_, err := cache.Metrics(ctx, orgA, ledgerA, testWindow)
		require.NoError(t, err)
	}

	assert.Len(t, server.Keys(), 1)

	metrics, _, _ := inner.calls()
	assert.Equal(t, 1, metrics)
}

// TestDashboardCache_EndpointSegmentsAreDistinct: the three endpoints must not
// collide on a shared prefix, which a naive key built by concatenation without
// a separator would allow.
func TestDashboardCache_EndpointSegmentsAreDistinct(t *testing.T) {
	inner := &countingRepo{amount: "100.00"}
	cache, server, ctx := newCache(t, inner)

	_, err := cache.Metrics(ctx, orgA, ledgerA, testWindow)
	require.NoError(t, err)

	_, err = cache.Volume(ctx, orgA, ledgerA, testWindow)
	require.NoError(t, err)

	keys := server.Keys()
	require.Len(t, keys, 2)

	assert.NotEqual(t, keys[0], keys[1])

	for _, key := range keys {
		assert.True(t, strings.Contains(key, ":metrics:") || strings.Contains(key, ":volume:"),
			"each endpoint owns a delimited segment, got %q", key)
	}
}

// TestDashboardCache_ProviderFailureStillAnswers: a provider that cannot hand
// out a client at all — Valkey down, or a tenant whose cache is not reachable —
// degrades to computing rather than failing the read.
func TestDashboardCache_ProviderFailureStillAnswers(t *testing.T) {
	inner := &countingRepo{amount: "100.00"}
	cache := redisDashboard.NewDashboardCache(inner, failingProvider{}, 0, nil)
	ctx := tmcore.ContextWithTenantID(context.Background(), "tenant-one")

	result, err := cache.Metrics(ctx, orgA, ledgerA, testWindow)
	require.NoError(t, err)
	assert.Equal(t, int64(7), result.Total)

	metrics, _, _ := inner.calls()
	assert.Equal(t, 1, metrics)
}
