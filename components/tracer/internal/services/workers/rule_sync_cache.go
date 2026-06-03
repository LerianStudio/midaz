// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package workers

//go:generate mockgen -source=rule_sync_cache.go -destination=mocks/rule_sync_cache_mock.go -package=mocks

import (
	"context"
	"time"

	"github.com/google/uuid"

	"github.com/LerianStudio/midaz/v3/components/tracer/internal/services/cache"
	"github.com/LerianStudio/midaz/v3/components/tracer/pkg/model"
)

// RuleSyncCache defines the cache operations needed by the sync worker.
// Combines read (for ClassifyChanges) and write (for applying deltas).
// Satisfied by *cache.RuleCache.
type RuleSyncCache interface {
	// GetActiveRules returns cached rules matching the given scope.
	// Pass nil to get all cached rules (used to build ClassifyChanges map).
	GetActiveRules(ctx context.Context, txScope *model.Scope) []*cache.CachedRule

	// ApplyChanges applies a delta: upserts rules and removes by ID.
	ApplyChanges(ctx context.Context, upserts []*cache.CachedRule, removeIDs []uuid.UUID)

	// LastSyncTime returns when the cache was last successfully updated.
	// Used to initialize the worker's lastSync from the warm-up timestamp.
	LastSyncTime(ctx context.Context) time.Time

	// Size returns the number of rules currently in the cache.
	// Used for observability metrics (cache size gauge).
	Size(ctx context.Context) int

	// MarkReady signals that the cache bucket for the tenant resolved from ctx
	// has been populated at least once. The sync worker calls this after every
	// successful ApplyChanges so per-tenant cache buckets become readable as
	// soon as the first sync cycle completes — otherwise the cache adapter's
	// IsReady gate (ErrRuleCacheNotReady / TRC-0281) never opens for freshly
	// spawned tenants. Idempotent — calling on an already-ready bucket is a
	// no-op.
	MarkReady(ctx context.Context)
}
