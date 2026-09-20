// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

// Package dashboard holds the Valkey read-through cache for the ledger's
// dashboard reads. It caches ANSWERS. The balance and transaction caches under
// internal/adapters/redis/{balancecache,transaction} serve the money-write hot
// path and are unrelated to this.
package dashboard

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	tmvalkey "github.com/LerianStudio/lib-commons/v7/commons/tenant-manager/valkey"
	libLog "github.com/LerianStudio/lib-observability/v4/log"
	"github.com/google/uuid"
	goredis "github.com/redis/go-redis/v9"
	"golang.org/x/sync/singleflight"

	postgresDashboard "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/dashboard"
	"github.com/LerianStudio/midaz/v4/pkg/dashboard"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
)

// Compile-time interface check: the cache is substitutable for the postgres
// repository, which is what lets bootstrap wire one or the other without the
// query service knowing which it got.
var _ postgresDashboard.Repository = (*DashboardCache)(nil)

// ClientProvider resolves the Valkey client for the CURRENT request.
//
// The cache holds a provider rather than a client because in multi-tenant mode
// each tenant may have its own Valkey: a client captured once at boot is the
// STATIC one, so every tenant's answers would be written to, and read from, one
// server regardless of which one their deployment owns. The tenant prefix on the
// key would keep those entries from colliding, but they would still be sitting
// in the wrong place — the same reason every other redis adapter in this
// component resolves per request rather than holding a client.
type ClientProvider interface {
	GetClient(ctx context.Context) (goredis.UniversalClient, error)
}

const (
	// DefaultTTL is how long a computed answer is served before it is
	// recomputed. It equals dashboard.Granularity: the window is truncated to
	// the minute, so a shorter TTL would recompute an answer that cannot have
	// changed, and a longer one would keep serving a window that has already
	// rolled forward.
	DefaultTTL = time.Minute

	// flightTimeout bounds a coalesced computation. The flight is deliberately
	// detached from the request that started it (see getOrCompute), so it needs
	// a deadline of its own — otherwise a stalled database would leave an orphan
	// flight holding the key forever and every later caller would join it
	// instead of retrying.
	flightTimeout = 30 * time.Second

	// cachePrefix namespaces every dashboard entry. tmvalkey.GetKeyContext adds
	// the tenant segment on top, so the full key reads
	// tenant:{tenantID}:midaz:dashboard:{endpoint}:{org}:{ledger}:{window}.
	cachePrefix = "midaz:dashboard"

	metricsEndpoint = "metrics"
	volumeEndpoint  = "volume"
	assetsEndpoint  = "assets"
)

// DashboardCache decorates a dashboard repository with a Valkey read-through
// cache.
//
// It is nil-tolerant by construction: a deployment with no Valkey client
// degrades to a straight pass-through rather than refusing to serve. A Valkey
// error is never fatal either — a failed read computes, a failed write is
// logged and dropped. The cache exists to keep load off the database, so losing
// it costs database time, which is exactly the thing it was buying. A dashboard
// with no cache is slower, never wrong.
//
// There is no invalidation, and that is correct rather than missing: the window
// is truncated to the minute, so the entry for "last 30 days" changes NAME
// every minute. An entry is never stale for longer than its own TTL because it
// stops being the entry anyone asks for.
type DashboardCache struct {
	inner    postgresDashboard.Repository
	provider ClientProvider
	ttl      time.Duration
	logger   libLog.Logger

	// flights coalesces concurrent misses on the same key into one database
	// read, keyed by the fully qualified key so two tenants — or two ledgers —
	// never share a flight.
	//
	// It matters more here than in a typical cache because every key expires on
	// the same wall-clock minute: the entry changes name for everyone
	// simultaneously and every open dashboard misses in the same instant.
	// Without coalescing that is one full aggregation per viewer per minute,
	// which turns a cached dashboard into a recurring load spike on the money
	// database.
	flights singleflight.Group
}

// NewDashboardCache wraps inner with a Valkey read-through cache. A nil
// provider yields a decorator that only ever passes through. ttl <= 0 means
// DefaultTTL.
func NewDashboardCache(inner postgresDashboard.Repository, provider ClientProvider, ttl time.Duration, logger libLog.Logger) *DashboardCache {
	if ttl <= 0 {
		ttl = DefaultTTL
	}

	return &DashboardCache{inner: inner, provider: provider, ttl: ttl, logger: logger}
}

// Metrics serves the headline panel from cache when it is there, and computes
// and stores it when it is not.
func (c *DashboardCache) Metrics(ctx context.Context, organizationID, ledgerID uuid.UUID, window dashboard.Window) (*mmodel.DashboardMetrics, error) {
	return getOrCompute(ctx, c, c.windowKey(metricsEndpoint, organizationID, ledgerID, window),
		func(ctx context.Context) (*mmodel.DashboardMetrics, error) {
			return c.inner.Metrics(ctx, organizationID, ledgerID, window)
		})
}

// Volume serves the per-day series from cache when it is there.
func (c *DashboardCache) Volume(ctx context.Context, organizationID, ledgerID uuid.UUID, window dashboard.Window) (*mmodel.DashboardVolume, error) {
	return getOrCompute(ctx, c, c.windowKey(volumeEndpoint, organizationID, ledgerID, window),
		func(ctx context.Context) (*mmodel.DashboardVolume, error) {
			return c.inner.Volume(ctx, organizationID, ledgerID, window)
		})
}

// Assets serves the current position from cache when it is there.
//
// Its key carries NO window, because the read carries no window: keying it by
// one would recompute the same position for every distinct period a viewer
// happened to select, answering one question several times over. It is the
// cheapest of the three reads and still cached, because N viewers coalescing
// onto one answer is the point rather than the cost of any single read.
func (c *DashboardCache) Assets(ctx context.Context, organizationID, ledgerID uuid.UUID) (*mmodel.DashboardAssets, error) {
	return getOrCompute(ctx, c, c.scopeKey(assetsEndpoint, organizationID, ledgerID),
		func(ctx context.Context) (*mmodel.DashboardAssets, error) {
			return c.inner.Assets(ctx, organizationID, ledgerID)
		})
}

// getOrCompute is the whole cache. It is a package-level generic rather than
// three methods because the three endpoints differ only in the value type, the
// key and the function that computes it; writing it once means a tenant- or
// ledger-isolation bug can only exist in one place.
//
// Ordering is deliberate: the key is built — and therefore the tenant resolved
// — BEFORE anything is read or written, so a context carrying no tenant
// produces no key at all rather than silently reading somebody else's entry.
func getOrCompute[T any](
	ctx context.Context,
	cache *DashboardCache,
	rawKey string,
	compute func(context.Context) (*T, error),
) (*T, error) {
	if cache == nil || cache.provider == nil {
		return compute(ctx)
	}

	client, err := cache.provider.GetClient(ctx)
	if err != nil || client == nil {
		// Valkey is unreachable right now. That costs database time, which is
		// exactly what the cache was buying; it never costs the caller an answer.
		cache.warn(ctx, "dashboard cache: no client", err)

		return compute(ctx)
	}

	key, err := tmvalkey.GetKeyContext(ctx, rawKey)
	if err != nil {
		// A key that cannot be built names a tenant this process must not guess
		// around: compute, serve, cache nothing.
		cache.warn(ctx, "dashboard cache: cannot build key", err)

		return compute(ctx)
	}

	if hit, ok := readEntry(ctx, cache, client, key, new(T)); ok {
		cache.debug(ctx, "dashboard cache hit", key)

		return hit, nil
	}

	cache.debug(ctx, "dashboard cache miss", key)

	// One flight per key: the losers of the race wait for the winner's answer
	// instead of running the same aggregation again.
	//
	// The flight runs on a context DETACHED from the request that opened it.
	// Without that, the first caller to hang up — a viewer closing a tab —
	// cancels the query every other viewer is waiting on, and coalescing would
	// have introduced the very failure it was added to prevent. WithoutCancel
	// keeps the tenant id and the trace that the key and the logs are built
	// from, and drops only the cancellation.
	//
	// The context is built INSIDE the closure, and that placement is the fix
	// rather than a style choice. Built in this frame instead, its deferred
	// cancel would belong to whichever caller happened to open the flight: that
	// caller returns the moment it hangs up, its cancel fires, and the detached
	// context dies under every other viewer still waiting — putting back by hand
	// exactly the cancellation WithoutCancel had severed.
	result := cache.flights.DoChan(key, func() (any, error) {
		flightCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), flightTimeout)
		defer cancel()

		computed, err := compute(flightCtx)
		if err != nil {
			return nil, err
		}

		cache.write(flightCtx, client, key, computed)

		return computed, nil
	})

	select {
	case <-ctx.Done():
		// This caller gave up. The flight continues for whoever else is on it.
		return nil, ctx.Err()
	case answer := <-result:
		if answer.Err != nil {
			return nil, answer.Err
		}

		// The type assertion cannot fail: the only producer of this key's value
		// is the closure above, which returns *T.
		return answer.Val.(*T), nil
	}
}

// windowKey renders the un-prefixed key for a windowed endpoint.
//
// The organization and ledger segments are MANDATORY and are the difference
// between this cache and the tracer's, whose key needs only the tenant because
// its window IS the whole query. A ledger read is scoped by its path, so a
// tenant-only key would serve one ledger's money figures to another ledger's
// dashboard inside the same organization.
func (c *DashboardCache) windowKey(endpoint string, organizationID, ledgerID uuid.UUID, window dashboard.Window) string {
	return c.scopeKey(endpoint, organizationID, ledgerID) + ":" + window.CacheKey()
}

// scopeKey renders the un-prefixed key for an endpoint scoped to one ledger.
func (c *DashboardCache) scopeKey(endpoint string, organizationID, ledgerID uuid.UUID) string {
	return fmt.Sprintf("%s:%s:%s:%s", cachePrefix, endpoint, organizationID, ledgerID)
}

// readEntry returns the decoded entry and true on a hit. A miss, a Valkey error
// and an entry that no longer decodes are all reported the same way — as a miss
// — because the caller's only correct response to each of them is to compute.
// It is a function rather than a method because Go methods cannot carry type
// parameters.
func readEntry[T any](ctx context.Context, cache *DashboardCache, client goredis.UniversalClient, key string, into *T) (*T, bool) {
	data, err := client.Get(ctx, key).Bytes()
	if err != nil {
		if !errors.Is(err, goredis.Nil) {
			cache.warn(ctx, "dashboard cache: read failed", err)
		}

		return nil, false
	}

	if err := json.Unmarshal(data, into); err != nil {
		cache.warn(ctx, "dashboard cache: stored entry did not decode", err)

		return nil, false
	}

	return into, true
}

// write stores the computed answer under the TTL. Failures are logged, never
// returned: the caller already holds a correct answer.
func (c *DashboardCache) write(ctx context.Context, client goredis.UniversalClient, key string, value any) {
	data, err := json.Marshal(value)
	if err != nil {
		c.warn(ctx, "dashboard cache: value did not encode", err)

		return
	}

	if err := client.Set(ctx, key, data, c.ttl).Err(); err != nil {
		c.warn(ctx, "dashboard cache: write failed", err)
	}
}

func (c *DashboardCache) warn(ctx context.Context, message string, err error) {
	if c.logger == nil {
		return
	}

	c.logger.Log(ctx, libLog.LevelWarn, message, libLog.Err(err))
}

func (c *DashboardCache) debug(ctx context.Context, message, key string) {
	if c.logger == nil {
		return
	}

	c.logger.Log(ctx, libLog.LevelDebug, message, libLog.String("cache.key", key))
}
