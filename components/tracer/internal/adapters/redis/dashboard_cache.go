// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

// Package redis holds the Valkey-backed query-result cache for the dashboard
// reads. It is a cache of ANSWERS, not of rules: the in-memory rule cache in
// internal/services/cache serves the validation hot path and is unrelated.
package redis

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/LerianStudio/lib-commons/v7/commons/tenant-manager/valkey"
	libLog "github.com/LerianStudio/lib-observability/v4/log"
	goredis "github.com/redis/go-redis/v9"

	"github.com/LerianStudio/midaz/v4/components/tracer/internal/services/query"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/model"
)

// Compile-time interface implementation check: the cache is substitutable for
// the postgres repository, which is what lets bootstrap wire one or the other
// without the query service knowing which it got.
var _ query.DashboardRepository = (*DashboardCache)(nil)

const (
	// DefaultTTL is how long a computed dashboard answer is served before it is
	// recomputed. It equals model.DashboardWindowGranularity: the window is
	// truncated to the minute, so a shorter TTL would recompute an answer that
	// cannot have changed, and a longer one would keep serving a window that
	// has already rolled forward.
	DefaultTTL = time.Minute

	// cachePrefix namespaces every dashboard entry. The tenant segment is
	// added on top of this by valkey.GetKeyContext, so the full key reads
	// tenant:{tenantID}:tracer:dashboard:{endpoint}:{window}.
	cachePrefix = "tracer:dashboard"

	metricsEndpoint    = "metrics"
	volumeEndpoint     = "volume"
	fraudTypesEndpoint = "fraud-types"
)

// DashboardCache decorates a DashboardRepository with a Valkey read-through
// cache.
//
// It is nil-tolerant by construction. Tracer's only Valkey client is the
// tenant-manager Pub/Sub client, which exists in multi-tenant mode and not in
// single-tenant mode; rather than open a second connection, this decorator
// takes that client and degrades to a straight pass-through when there is
// none. A dashboard with no cache is slower, never wrong.
//
// A Valkey error is never fatal either: a failed read computes, and a failed
// write is logged and dropped. The cache exists to keep load off the database,
// so losing it costs database time, which is exactly the thing it was buying.
type DashboardCache struct {
	inner  query.DashboardRepository
	client goredis.UniversalClient
	ttl    time.Duration
	logger libLog.Logger
}

// NewDashboardCache wraps inner with a Valkey read-through cache. A nil client
// yields a decorator that only ever passes through. ttl <= 0 means DefaultTTL.
func NewDashboardCache(inner query.DashboardRepository, client goredis.UniversalClient, ttl time.Duration, logger libLog.Logger) *DashboardCache {
	if ttl <= 0 {
		ttl = DefaultTTL
	}

	return &DashboardCache{inner: inner, client: client, ttl: ttl, logger: logger}
}

// Metrics serves the headline panel from cache when it is there, and computes
// and stores it when it is not.
func (c *DashboardCache) Metrics(ctx context.Context, window model.DashboardWindow) (*model.DashboardMetrics, error) {
	return getOrCompute(ctx, c, metricsEndpoint, window, c.inner.Metrics)
}

// Volume serves the volume series from cache when it is there.
func (c *DashboardCache) Volume(ctx context.Context, window model.DashboardWindow) (*model.DashboardVolume, error) {
	return getOrCompute(ctx, c, volumeEndpoint, window, c.inner.Volume)
}

// FraudTypes serves the flagged breakdown from cache when it is there.
func (c *DashboardCache) FraudTypes(ctx context.Context, window model.DashboardWindow) (*model.DashboardFraudTypes, error) {
	return getOrCompute(ctx, c, fraudTypesEndpoint, window, c.inner.FraudTypes)
}

// getOrCompute is the whole cache. It is a package-level generic rather than
// three methods because the three endpoints differ only in the value type and
// the function that computes it; writing it once means a tenant-isolation or
// TTL bug can only exist in one place.
//
// Ordering is deliberate: the key is built (and therefore the tenant resolved)
// BEFORE anything is read or written, so a context carrying no tenant produces
// an unprefixed key it shares with no one rather than silently reading another
// tenant's entry.
func getOrCompute[T any](
	ctx context.Context,
	cache *DashboardCache,
	endpoint string,
	window model.DashboardWindow,
	compute func(context.Context, model.DashboardWindow) (*T, error),
) (*T, error) {
	if cache == nil || cache.client == nil {
		return compute(ctx, window)
	}

	key, err := cache.key(ctx, endpoint, window)
	if err != nil {
		// A key that cannot be built is a tenant id this process must not
		// guess around: compute, serve, cache nothing.
		cache.warn(ctx, "dashboard cache: cannot build key", err)

		return compute(ctx, window)
	}

	if hit, ok := readEntry(ctx, cache, key, new(T)); ok {
		cache.debug(ctx, "dashboard cache hit", key, endpoint)

		return hit, nil
	}

	cache.debug(ctx, "dashboard cache miss", key, endpoint)

	value, err := compute(ctx, window)
	if err != nil {
		return nil, err
	}

	cache.write(ctx, key, value)

	return value, nil
}

// key renders the tenant-prefixed cache key for one endpoint and window.
func (c *DashboardCache) key(ctx context.Context, endpoint string, window model.DashboardWindow) (string, error) {
	raw := cachePrefix + ":" + endpoint + ":" + window.CacheKey()

	key, err := valkey.GetKeyContext(ctx, raw)
	if err != nil {
		return "", fmt.Errorf("build dashboard cache key: %w", err)
	}

	return key, nil
}

// readEntry returns the decoded entry and true on a hit. A miss, a Valkey
// error and an entry that no longer decodes are all reported the same way — as
// a miss — because the caller's only correct response to each of them is to
// compute. It is a function rather than a method because Go methods cannot
// carry type parameters.
func readEntry[T any](ctx context.Context, cache *DashboardCache, key string, into *T) (*T, bool) {
	data, err := cache.client.Get(ctx, key).Bytes()
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
func (c *DashboardCache) write(ctx context.Context, key string, value any) {
	data, err := json.Marshal(value)
	if err != nil {
		c.warn(ctx, "dashboard cache: value did not encode", err)

		return
	}

	if err := c.client.Set(ctx, key, data, c.ttl).Err(); err != nil {
		c.warn(ctx, "dashboard cache: write failed", err)
	}
}

func (c *DashboardCache) warn(ctx context.Context, message string, err error) {
	if c.logger == nil {
		return
	}

	c.logger.Log(ctx, libLog.LevelWarn, message, libLog.Err(err))
}

func (c *DashboardCache) debug(ctx context.Context, message, key, endpoint string) {
	if c.logger == nil {
		return
	}

	c.logger.Log(ctx, libLog.LevelDebug, message,
		libLog.String("cache.key", key),
		libLog.String("dashboard.endpoint", endpoint))
}
