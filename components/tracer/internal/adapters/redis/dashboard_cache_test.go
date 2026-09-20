// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package redis_test

import (
	"context"
	"errors"
	"strings"
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
}

func (r *countingRepo) TopRules(_ context.Context, w model.DashboardWindow) (*model.DashboardTopRules, error) {
	r.topRules.Add(1)

	if r.err != nil {
		return nil, r.err
	}

	return &model.DashboardTopRules{
		Rules:       []model.TopRule{{Name: "high-value-wire", Matches: r.processed, Executions: r.processed}},
		WindowStart: w.From,
		WindowEnd:   w.To,
	}, nil
}

func (r *countingRepo) Metrics(_ context.Context, w model.DashboardWindow) (*model.DashboardMetrics, error) {
	r.metrics.Add(1)

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

func (r *countingRepo) Volume(_ context.Context, w model.DashboardWindow) (*model.DashboardVolume, error) {
	r.volume.Add(1)

	if r.err != nil {
		return nil, r.err
	}

	return &model.DashboardVolume{
		Points:      []model.VolumePoint{{Date: "2026-09-20", Volume: r.processed}},
		WindowStart: w.From,
		WindowEnd:   w.To,
	}, nil
}

func (r *countingRepo) FraudTypes(_ context.Context, w model.DashboardWindow) (*model.DashboardFraudTypes, error) {
	r.fraudTypes.Add(1)

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
