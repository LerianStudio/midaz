// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package cache

import (
	"context"
	"math"
	"sync"
	"time"

	tmcore "github.com/LerianStudio/lib-commons/v5/commons/tenant-manager/core"
	"github.com/google/uuid"

	"github.com/LerianStudio/midaz/v3/components/tracer/pkg/clock"
	"github.com/LerianStudio/midaz/v3/components/tracer/pkg/model"
)

// RuleCache provides thread-safe in-memory storage for compiled rules.
// Read operations use RLock for maximum concurrency.
// Write operations use full Lock.
//
// In multi-tenant mode, state is partitioned per tenant using the tenant ID
// extracted from the request context via tmcore.GetTenantIDContext. In
// single-tenant mode the tenant ID is the empty string, so all state lives in
// the "" bucket — behaviour is identical to the pre-multi-tenant cache.
type RuleCache struct {
	mu           sync.RWMutex
	rules        map[string]map[uuid.UUID]*CachedRule // outer key: tenantID ("" in single-tenant)
	ready        map[string]bool
	lastSyncTime map[string]time.Time
	clock        clock.Clock
}

// NewRuleCache creates a new empty rule cache.
func NewRuleCache(clk clock.Clock) *RuleCache {
	if clk == nil {
		clk = clock.New()
	}

	return &RuleCache{
		rules:        make(map[string]map[uuid.UUID]*CachedRule),
		ready:        make(map[string]bool),
		lastSyncTime: make(map[string]time.Time),
		clock:        clk,
	}
}

// GetActiveRules returns cached rules matching the given scope for the tenant
// resolved from ctx. If txScope is nil, returns all rules for the tenant
// (bypasses scope filtering). Otherwise, returns rules whose scopes match
// txScope via model.RuleScopesMatch. Returns deep copies to prevent concurrent
// mutation.
func (c *RuleCache) GetActiveRules(ctx context.Context, txScope *model.Scope) []*CachedRule {
	tenantID := getTenantID(ctx)

	c.mu.RLock()
	defer c.mu.RUnlock()

	rules := c.rules[tenantID]
	result := make([]*CachedRule, 0, len(rules))

	for _, cached := range rules {
		if cached == nil || cached.Rule == nil {
			continue
		}

		// Scope filtering: ruleScope.Matches(txScope) — direction is critical
		if txScope == nil || model.RuleScopesMatch(cached.Rule.Scopes, txScope) {
			copied := deepCopy(cached)
			if copied == nil {
				continue // defense-in-depth: deepCopy returns nil only for nil input (guarded above)
			}

			result = append(result, copied)
		}
	}

	return result
}

// SetRules replaces all rules in the cache (full reload) for the tenant
// resolved from ctx.
func (c *RuleCache) SetRules(ctx context.Context, rules []*CachedRule) {
	tenantID := getTenantID(ctx)

	c.mu.Lock()
	defer c.mu.Unlock()

	newRules := make(map[uuid.UUID]*CachedRule, len(rules))
	for _, r := range rules {
		if r == nil || r.Rule == nil {
			continue
		}

		newRules[r.Rule.ID] = r
	}

	if c.rules == nil {
		c.rules = make(map[string]map[uuid.UUID]*CachedRule)
	}

	c.rules[tenantID] = newRules

	if c.lastSyncTime == nil {
		c.lastSyncTime = make(map[string]time.Time)
	}

	c.lastSyncTime[tenantID] = c.clock.Now()
}

// ApplyChanges applies a delta (upserts + removals) for the tenant resolved
// from ctx.
func (c *RuleCache) ApplyChanges(ctx context.Context, upserts []*CachedRule, removeIDs []uuid.UUID) {
	tenantID := getTenantID(ctx)

	c.mu.Lock()
	defer c.mu.Unlock()

	if c.rules == nil {
		c.rules = make(map[string]map[uuid.UUID]*CachedRule)
	}

	rules, ok := c.rules[tenantID]
	if !ok {
		rules = make(map[uuid.UUID]*CachedRule)
		c.rules[tenantID] = rules
	}

	for _, r := range upserts {
		if r == nil || r.Rule == nil {
			continue
		}

		rules[r.Rule.ID] = r
	}

	for _, id := range removeIDs {
		delete(rules, id)
	}

	if c.lastSyncTime == nil {
		c.lastSyncTime = make(map[string]time.Time)
	}

	c.lastSyncTime[tenantID] = c.clock.Now()
}

// UpsertRule adds or replaces a single rule in the cache with its compiled
// program for the tenant resolved from ctx. Convenience wrapper around
// ApplyChanges for single-rule updates.
func (c *RuleCache) UpsertRule(ctx context.Context, rule *model.Rule, program any) {
	c.ApplyChanges(ctx, []*CachedRule{{Rule: rule, Program: program}}, nil)
}

// RemoveRule removes a single rule from the cache by ID for the tenant
// resolved from ctx. Convenience wrapper around ApplyChanges for single-rule
// removal.
func (c *RuleCache) RemoveRule(ctx context.Context, id uuid.UUID) {
	c.ApplyChanges(ctx, nil, []uuid.UUID{id})
}

// MarkReady signals that the cache has been populated for the tenant resolved
// from ctx.
func (c *RuleCache) MarkReady(ctx context.Context) {
	tenantID := getTenantID(ctx)

	c.mu.Lock()
	defer c.mu.Unlock()

	if c.ready == nil {
		c.ready = make(map[string]bool)
	}

	c.ready[tenantID] = true
}

// IsReady returns true if the cache has been populated at least once for the
// tenant resolved from ctx.
func (c *RuleCache) IsReady(ctx context.Context) bool {
	tenantID := getTenantID(ctx)

	c.mu.RLock()
	defer c.mu.RUnlock()

	return c.ready[tenantID]
}

// Size returns the number of rules in the cache for the tenant resolved from
// ctx.
func (c *RuleCache) Size(ctx context.Context) int {
	tenantID := getTenantID(ctx)

	c.mu.RLock()
	defer c.mu.RUnlock()

	return len(c.rules[tenantID])
}

// LastSyncTime returns when the cache was last successfully updated for the
// tenant resolved from ctx.
func (c *RuleCache) LastSyncTime(ctx context.Context) time.Time {
	tenantID := getTenantID(ctx)

	c.mu.RLock()
	defer c.mu.RUnlock()

	return c.lastSyncTime[tenantID]
}

// Staleness returns how long since the last successful sync for the tenant
// resolved from ctx. Returns math.MaxInt64 if the cache was never synced (zero
// lastSyncTime).
func (c *RuleCache) Staleness(ctx context.Context) time.Duration {
	tenantID := getTenantID(ctx)

	c.mu.RLock()
	defer c.mu.RUnlock()

	ts := c.lastSyncTime[tenantID]
	if ts.IsZero() {
		return time.Duration(math.MaxInt64)
	}

	return c.clock.Now().Sub(ts)
}

// EvictTenant removes all cached state for a tenant. Called by the tenant
// event listener when a tenant is deleted.
func (c *RuleCache) EvictTenant(tenantID string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	delete(c.rules, tenantID)
	delete(c.ready, tenantID)
	delete(c.lastSyncTime, tenantID)
}

// getTenantID extracts the tenant identifier from ctx. In single-tenant mode
// (no tenant attached to the context) it returns the empty string, which is
// used as the canonical single-tenant bucket throughout the cache.
func getTenantID(ctx context.Context) string {
	if ctx == nil {
		return ""
	}

	return tmcore.GetTenantIDContext(ctx)
}

// deepCopy creates a deep copy of a CachedRule to prevent concurrent mutation.
// Copies ALL pointer fields: Rule-level (*string, *time.Time) and Scope-level (*uuid.UUID, *TransactionType, *string).
func deepCopy(src *CachedRule) *CachedRule {
	if src == nil || src.Rule == nil {
		return nil
	}

	ruleCopy := *src.Rule

	// Deep copy Rule pointer fields
	if src.Rule.Description != nil {
		desc := *src.Rule.Description
		ruleCopy.Description = &desc
	}

	if src.Rule.ActivatedAt != nil {
		t := *src.Rule.ActivatedAt
		ruleCopy.ActivatedAt = &t
	}

	if src.Rule.DeactivatedAt != nil {
		t := *src.Rule.DeactivatedAt
		ruleCopy.DeactivatedAt = &t
	}

	if src.Rule.DeletedAt != nil {
		t := *src.Rule.DeletedAt
		ruleCopy.DeletedAt = &t
	}

	// TODO: update this manual deep copy when model.Scope gains new pointer fields.
	// Deep copy scopes slice with all pointer fields.
	if src.Rule.Scopes != nil {
		scopesCopy := make([]model.Scope, len(src.Rule.Scopes))
		for i, s := range src.Rule.Scopes {
			scopesCopy[i] = s

			if s.SegmentID != nil {
				id := *s.SegmentID
				scopesCopy[i].SegmentID = &id
			}

			if s.PortfolioID != nil {
				id := *s.PortfolioID
				scopesCopy[i].PortfolioID = &id
			}

			if s.AccountID != nil {
				id := *s.AccountID
				scopesCopy[i].AccountID = &id
			}

			if s.MerchantID != nil {
				id := *s.MerchantID
				scopesCopy[i].MerchantID = &id
			}

			if s.TransactionType != nil {
				tt := *s.TransactionType
				scopesCopy[i].TransactionType = &tt
			}

			if s.SubType != nil {
				st := *s.SubType
				scopesCopy[i].SubType = &st
			}
		}

		ruleCopy.Scopes = scopesCopy
	}

	return &CachedRule{
		Rule: &ruleCopy,
		// Program is shared, not cloned: compiled CEL programs are immutable after compilation.
		Program: src.Program,
	}
}
