// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package engine

import (
	"context"
	"encoding/json"
	"time"

	fetcher "github.com/LerianStudio/fetcher/pkg/engine"
)

// schemaCacheKeyPrefix namespaces schema-cache entries within the shared Redis
// keyspace so they never collide with the reconciler's lock keys.
const schemaCacheKeyPrefix = "reporter:engine:schema:"

// redisRepository is the narrow Redis surface the schema cache needs. It is
// satisfied by pkg/reporter/redis.RedisRepository (the same repository wired for
// the reconciler distributed lock). Declaring it here keeps this package
// unit-testable and free of a concrete Redis dependency.
type redisRepository interface {
	Get(ctx context.Context, key string) (string, error)
	Set(ctx context.Context, key, value string, ttl time.Duration) error
}

// schemaCache is the optional SchemaCache backed by Redis. It tenant-scopes
// every key by TenantContext.TenantID so a cached snapshot is never served
// across tenants. Redis faults degrade to a cache miss rather than failing the
// operation, so a cache outage falls back to fresh discovery instead of breaking
// extraction.
type schemaCache struct {
	redis redisRepository
	ttl   time.Duration
}

// Compile-time check that schemaCache satisfies the engine's optional
// SchemaCache port.
var _ fetcher.SchemaCache = (*schemaCache)(nil)

// NewSchemaCache builds a Redis-backed SchemaCache. A non-positive ttl means the
// entries do not expire (the host relies on key overwrite). The cache is
// explicitly optional: when Redis is not configured, the bootstrap passes no
// WithSchemaCache and schema is always discovered fresh.
func NewSchemaCache(redis redisRepository, ttl time.Duration) fetcher.SchemaCache {
	return &schemaCache{redis: redis, ttl: ttl}
}

// GetSchema returns the cached snapshot for the tenant+datasource pair. A Redis
// error or an absent/unparseable entry reports ok=false with a nil error so the
// engine falls back to fresh discovery instead of failing.
func (c *schemaCache) GetSchema(ctx context.Context, tenant fetcher.TenantContext, configName string) (fetcher.SchemaSnapshot, bool, error) {
	raw, err := c.redis.Get(ctx, schemaCacheKey(tenant.TenantID, configName))
	if err != nil || raw == "" {
		// A Redis fault degrades to a cache miss by design, so the engine falls
		// back to fresh discovery instead of failing the extraction.
		return fetcher.SchemaSnapshot{}, false, nil //nolint:nilerr // intentional: cache outage = miss
	}

	var snapshot fetcher.SchemaSnapshot
	if err := json.Unmarshal([]byte(raw), &snapshot); err != nil {
		// A corrupt cache entry is treated as a miss, not a hard failure.
		return fetcher.SchemaSnapshot{}, false, nil //nolint:nilerr // intentional: corrupt entry = miss
	}

	return snapshot, true, nil
}

// PutSchema stores the snapshot under the tenant-scoped key. A Redis write error
// is swallowed: the cache is best-effort, and a failed write must not fail the
// extraction that produced the snapshot.
func (c *schemaCache) PutSchema(ctx context.Context, tenant fetcher.TenantContext, snapshot fetcher.SchemaSnapshot) error {
	payload, err := json.Marshal(snapshot)
	if err != nil {
		// An unmarshalable snapshot is a best-effort cache write skipped silently;
		// it must never fail the extraction that produced the snapshot.
		return nil //nolint:nilerr // intentional: best-effort cache, never fail the caller
	}

	_ = c.redis.Set(ctx, schemaCacheKey(tenant.TenantID, snapshot.ConfigName), string(payload), c.ttl)

	return nil
}

// schemaCacheKey builds the tenant-scoped cache key. The tenant ID is the first
// path segment after the prefix so the keyspace is partitioned per tenant — no
// cross-tenant schema serving is structurally possible.
func schemaCacheKey(tenantID, configName string) string {
	return schemaCacheKeyPrefix + tenantID + ":" + configName
}
