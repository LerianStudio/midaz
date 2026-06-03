// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package cache_test

import (
	"context"
	"testing"
	"time"

	tmcore "github.com/LerianStudio/lib-commons/v5/commons/tenant-manager/core"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v3/components/tracer/internal/services/cache"
	"github.com/LerianStudio/midaz/v3/components/tracer/internal/testutil"
	"github.com/LerianStudio/midaz/v3/components/tracer/pkg/clock"
)

// TestRuleCache_ContextIsolation verifies that rules inserted under one
// tenant's context are not visible to another tenant.
func TestRuleCache_ContextIsolation(t *testing.T) {
	t.Parallel()

	ctxA := tmcore.ContextWithTenantID(context.Background(), "tenant-a")
	ctxB := tmcore.ContextWithTenantID(context.Background(), "tenant-b")

	ruleA := newTestCachedRule(newTestRule(1))
	ruleB1 := newTestCachedRule(newTestRule(2))
	ruleB2 := newTestCachedRule(newTestRule(3))

	c := cache.NewRuleCache(clock.New())
	c.SetRules(ctxA, []*cache.CachedRule{ruleA})
	c.SetRules(ctxB, []*cache.CachedRule{ruleB1, ruleB2})

	rulesA := c.GetActiveRules(ctxA, nil)
	require.Len(t, rulesA, 1, "tenant-a context should see exactly one rule")
	assert.Equal(t, ruleA.Rule.ID, rulesA[0].Rule.ID, "tenant-a rule must match")

	rulesB := c.GetActiveRules(ctxB, nil)
	require.Len(t, rulesB, 2, "tenant-b context should see exactly two rules")

	// Collect tenant-b IDs to make the assertion order-independent
	bIDs := map[string]bool{
		rulesB[0].Rule.ID.String(): true,
		rulesB[1].Rule.ID.String(): true,
	}
	assert.True(t, bIDs[ruleB1.Rule.ID.String()], "tenant-b must see ruleB1")
	assert.True(t, bIDs[ruleB2.Rule.ID.String()], "tenant-b must see ruleB2")
	assert.False(t, bIDs[ruleA.Rule.ID.String()], "tenant-b must NOT see tenant-a's rule")

	assert.Equal(t, 1, c.Size(ctxA), "tenant-a size should reflect its own bucket")
	assert.Equal(t, 2, c.Size(ctxB), "tenant-b size should reflect its own bucket")
}

// TestRuleCache_EmptyContext_BehavesAsSingleTenant verifies that when no
// tenant is attached to ctx (tmcore returns ""), all operations work on the
// canonical "" bucket. This is the single-tenant backward-compat contract.
func TestRuleCache_EmptyContext_BehavesAsSingleTenant(t *testing.T) {
	t.Parallel()

	ctx := context.Background() // no tenant attached → tenantID == ""
	rule := newTestCachedRule(newTestRule(1))

	c := cache.NewRuleCache(clock.New())
	c.SetRules(ctx, []*cache.CachedRule{rule})
	c.MarkReady(ctx)

	rules := c.GetActiveRules(ctx, nil)
	require.Len(t, rules, 1, "empty-context SetRules + GetActiveRules should roundtrip")
	assert.Equal(t, rule.Rule.ID, rules[0].Rule.ID)

	assert.True(t, c.IsReady(ctx), "empty-context MarkReady must set the default bucket ready")
	assert.Equal(t, 1, c.Size(ctx), "empty-context Size must report the default bucket")
}

// TestRuleCache_EvictTenant verifies that EvictTenant removes all state for
// one tenant without affecting any other tenant.
func TestRuleCache_EvictTenant(t *testing.T) {
	t.Parallel()

	ctxA := tmcore.ContextWithTenantID(context.Background(), "tenant-a")
	ctxB := tmcore.ContextWithTenantID(context.Background(), "tenant-b")

	ruleA := newTestCachedRule(newTestRule(1))
	ruleB := newTestCachedRule(newTestRule(2))

	c := cache.NewRuleCache(clock.New())
	c.SetRules(ctxA, []*cache.CachedRule{ruleA})
	c.MarkReady(ctxA)
	c.SetRules(ctxB, []*cache.CachedRule{ruleB})
	c.MarkReady(ctxB)

	require.Equal(t, 1, c.Size(ctxA), "precondition: tenant-a has one rule")
	require.Equal(t, 1, c.Size(ctxB), "precondition: tenant-b has one rule")

	c.EvictTenant("tenant-a")

	assert.Equal(t, 0, c.Size(ctxA), "tenant-a size should be 0 after eviction")
	assert.False(t, c.IsReady(ctxA), "tenant-a ready flag should be reset after eviction")
	assert.True(t, c.LastSyncTime(ctxA).IsZero(), "tenant-a lastSyncTime should be zero after eviction")

	assert.Equal(t, 1, c.Size(ctxB), "tenant-b size must be unaffected by tenant-a eviction")
	assert.True(t, c.IsReady(ctxB), "tenant-b ready flag must be unaffected by tenant-a eviction")
	rulesB := c.GetActiveRules(ctxB, nil)
	require.Len(t, rulesB, 1, "tenant-b must still see its rule after tenant-a eviction")
	assert.Equal(t, ruleB.Rule.ID, rulesB[0].Rule.ID)
}

// TestRuleCache_LastSyncTime_PerTenant verifies that each tenant's
// lastSyncTime is tracked independently.
func TestRuleCache_LastSyncTime_PerTenant(t *testing.T) {
	t.Parallel()

	baseTime := testutil.FixedTime()
	clk := &testutil.MockClock{FixedTime: baseTime}
	c := cache.NewRuleCache(clk)

	ctxA := tmcore.ContextWithTenantID(context.Background(), "tenant-a")
	ctxB := tmcore.ContextWithTenantID(context.Background(), "tenant-b")

	// Set A first, advance the clock, then set B
	c.SetRules(ctxA, []*cache.CachedRule{newTestCachedRule(newTestRule(1))})
	syncedA := clk.Now()

	clk.SetTime(baseTime.Add(30 * time.Second))

	c.SetRules(ctxB, []*cache.CachedRule{newTestCachedRule(newTestRule(2))})
	syncedB := clk.Now()

	assert.Equal(t, syncedA, c.LastSyncTime(ctxA), "tenant-a lastSyncTime must reflect its own sync")
	assert.Equal(t, syncedB, c.LastSyncTime(ctxB), "tenant-b lastSyncTime must reflect its own sync")
	assert.NotEqual(t, c.LastSyncTime(ctxA), c.LastSyncTime(ctxB),
		"different tenants must not share lastSyncTime")
}

// TestRuleCache_IsReady_PerTenant verifies that MarkReady in one tenant's
// context does not mark the cache ready for another tenant.
func TestRuleCache_IsReady_PerTenant(t *testing.T) {
	t.Parallel()

	ctxA := tmcore.ContextWithTenantID(context.Background(), "tenant-a")
	ctxB := tmcore.ContextWithTenantID(context.Background(), "tenant-b")

	c := cache.NewRuleCache(clock.New())
	c.MarkReady(ctxA)

	assert.True(t, c.IsReady(ctxA), "tenant-a should be ready after MarkReady in ctxA")
	assert.False(t, c.IsReady(ctxB), "tenant-b must NOT be ready when only ctxA called MarkReady")
}
