// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package workers

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"tracer/pkg/model"
)

func TestClassifyChanges_AllNew(t *testing.T) {
	t.Parallel()

	cached := buildCachedMap(t) // empty cache
	r1 := newSyncTestActiveRule(1)
	r2 := newSyncTestActiveRule(2)

	cs := ClassifyChanges(cached, []*model.Rule{r1, r2})

	assert.Len(t, cs.New, 2)
	assert.Empty(t, cs.Updated)
	assert.Empty(t, cs.Deleted)
}

func TestClassifyChanges_AllUpdated(t *testing.T) {
	t.Parallel()

	r1 := newSyncTestActiveRule(1)
	r2 := newSyncTestActiveRule(2)
	cached := buildCachedMap(t,
		newSyncTestCachedRule(r1),
		newSyncTestCachedRule(r2),
	)

	// Fetch same rules but with newer UpdatedAt
	r1Updated := newSyncTestActiveRule(1)
	r1Updated.UpdatedAt = r1.UpdatedAt.Add(5 * time.Second)
	r2Updated := newSyncTestActiveRule(2)
	r2Updated.UpdatedAt = r2.UpdatedAt.Add(5 * time.Second)

	cs := ClassifyChanges(cached, []*model.Rule{r1Updated, r2Updated})

	assert.Empty(t, cs.New)
	assert.Len(t, cs.Updated, 2)
	assert.Empty(t, cs.Deleted)
}

func TestClassifyChanges_AllDeleted(t *testing.T) {
	t.Parallel()

	r1 := newSyncTestActiveRule(1)
	r2 := newSyncTestActiveRule(2)
	cached := buildCachedMap(t,
		newSyncTestCachedRule(r1),
		newSyncTestCachedRule(r2),
	)

	// Fetch same rules but now INACTIVE
	r1Deactivated := newSyncTestRule(1, model.RuleStatusInactive)
	r2Deactivated := newSyncTestRule(2, model.RuleStatusInactive)

	cs := ClassifyChanges(cached, []*model.Rule{r1Deactivated, r2Deactivated})

	assert.Empty(t, cs.New)
	assert.Empty(t, cs.Updated)
	assert.Len(t, cs.Deleted, 2)
	assert.Contains(t, cs.Deleted, r1.ID)
	assert.Contains(t, cs.Deleted, r2.ID)
}

func TestClassifyChanges_Mixed(t *testing.T) {
	t.Parallel()

	// Existing rules in cache
	existingRule := newSyncTestActiveRule(1)
	toBeDeletedRule := newSyncTestActiveRule(2)
	cached := buildCachedMap(t,
		newSyncTestCachedRule(existingRule),
		newSyncTestCachedRule(toBeDeletedRule),
	)

	// Fetched: one updated, one deleted, one new
	updatedRule := newSyncTestActiveRule(1)
	updatedRule.UpdatedAt = existingRule.UpdatedAt.Add(5 * time.Second)
	deletedRule := newSyncTestRule(2, model.RuleStatusDeleted)
	newRule := newSyncTestActiveRule(3)

	cs := ClassifyChanges(cached, []*model.Rule{updatedRule, deletedRule, newRule})

	assert.Len(t, cs.New, 1)
	assert.Equal(t, newRule.ID, cs.New[0].ID)
	assert.Len(t, cs.Updated, 1)
	assert.Equal(t, updatedRule.ID, cs.Updated[0].ID)
	assert.Len(t, cs.Deleted, 1)
	assert.Contains(t, cs.Deleted, toBeDeletedRule.ID)
}

func TestClassifyChanges_NoChanges(t *testing.T) {
	t.Parallel()

	r1 := newSyncTestActiveRule(1)
	cached := buildCachedMap(t, newSyncTestCachedRule(r1))

	// Fetch same rule with same UpdatedAt (overlap buffer idempotency)
	cs := ClassifyChanges(cached, []*model.Rule{r1})

	assertChangeSetEmpty(t, cs)
}

func TestClassifyChanges_DeactivatedRule(t *testing.T) {
	t.Parallel()

	// Rule exists in cache as active
	activeRule := newSyncTestActiveRule(1)
	cached := buildCachedMap(t, newSyncTestCachedRule(activeRule))

	// Same rule fetched as INACTIVE (deactivated)
	deactivatedRule := newSyncTestRule(1, model.RuleStatusInactive)
	deactivatedRule.UpdatedAt = activeRule.UpdatedAt.Add(1 * time.Second)

	cs := ClassifyChanges(cached, []*model.Rule{deactivatedRule})

	assert.Empty(t, cs.New)
	assert.Empty(t, cs.Updated)
	require.Len(t, cs.Deleted, 1)
	assert.Equal(t, activeRule.ID, cs.Deleted[0])
}

func TestClassifyChanges_EmptyFetch(t *testing.T) {
	t.Parallel()

	r1 := newSyncTestActiveRule(1)
	cached := buildCachedMap(t, newSyncTestCachedRule(r1))

	cs := ClassifyChanges(cached, []*model.Rule{})

	assertChangeSetEmpty(t, cs)
}

func TestClassifyChanges_DraftedRuleRemovedFromCache(t *testing.T) {
	t.Parallel()

	// Rule exists in cache as active, fetched as DRAFT (reverted to draft for editing)
	activeRule := newSyncTestActiveRule(1)
	cached := buildCachedMap(t, newSyncTestCachedRule(activeRule))

	draftedRule := newSyncTestRule(1, model.RuleStatusDraft)
	draftedRule.UpdatedAt = activeRule.UpdatedAt.Add(1 * time.Second)

	cs := ClassifyChanges(cached, []*model.Rule{draftedRule})

	assert.Empty(t, cs.New)
	assert.Empty(t, cs.Updated)
	require.Len(t, cs.Deleted, 1)
	assert.Equal(t, activeRule.ID, cs.Deleted[0])
}

func TestClassifyChanges_DeactivatedNotInCache(t *testing.T) {
	t.Parallel()

	// Rule not in cache, fetched as DELETED
	cached := buildCachedMap(t)
	deletedRule := newSyncTestRule(1, model.RuleStatusDeleted)

	cs := ClassifyChanges(cached, []*model.Rule{deletedRule})

	// Deactivated rule not in cache should be ignored (no action needed)
	assertChangeSetEmpty(t, cs)
}
