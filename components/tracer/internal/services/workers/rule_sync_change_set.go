// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package workers

import (
	"github.com/google/uuid"

	"github.com/LerianStudio/midaz/v4/components/tracer/internal/services/cache"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/model"
)

// ChangeSet holds the result of classifying fetched rules against cached state.
// Used by runSyncCycle to determine what cache mutations are needed.
type ChangeSet struct {
	// New contains active rules not currently in the cache.
	New []*model.Rule
	// Updated contains active rules in the cache with a newer UpdatedAt timestamp.
	Updated []*model.Rule
	// Deleted contains IDs of rules to remove from cache (deactivated/deleted/drafted).
	Deleted []uuid.UUID
}

// IsEmpty returns true if the change set contains no changes.
func (cs ChangeSet) IsEmpty() bool {
	return len(cs.New) == 0 && len(cs.Updated) == 0 && len(cs.Deleted) == 0
}

// ClassifyChanges compares fetched rules against the current cache snapshot
// and classifies each rule as new, updated, or deleted.
// This is a pure function with no side effects — independently testable.
//
// Classification logic:
//   - Status != ACTIVE + in cache -> Deleted (deactivated/deleted)
//   - Status != ACTIVE + not in cache -> ignored
//   - Status == ACTIVE + not in cache -> New
//   - Status == ACTIVE + in cache + newer UpdatedAt -> Updated
//   - Status == ACTIVE + in cache + same/older UpdatedAt -> ignored (overlap buffer idempotency)
func ClassifyChanges(cached map[uuid.UUID]*cache.CachedRule, fetched []*model.Rule) ChangeSet {
	var cs ChangeSet

	for _, rule := range fetched {
		if rule == nil {
			continue
		}

		existing, exists := cached[rule.ID]

		// Non-active rule: classify as deleted if currently cached
		if rule.Status != model.RuleStatusActive {
			if exists {
				cs.Deleted = append(cs.Deleted, rule.ID)
			}

			continue
		}

		// Active rule: classify as new or updated
		if !exists {
			cs.New = append(cs.New, rule)
		} else if existing.Rule != nil && rule.UpdatedAt.After(existing.Rule.UpdatedAt) {
			cs.Updated = append(cs.Updated, rule)
		}
		// else: same or older timestamp -> no change (overlap buffer idempotency)
	}

	return cs
}
