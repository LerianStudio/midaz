// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package cache

import (
	"context"

	"github.com/LerianStudio/midaz/v3/components/tracer/pkg/constant"
	"github.com/LerianStudio/midaz/v3/components/tracer/pkg/model"
)

// CacheAdapter wraps RuleCache to satisfy query.ActiveRulesRepository.
// This enables the validation hot path to read from cache instead of PostgreSQL.
type CacheAdapter struct {
	cache *RuleCache
}

// NewCacheAdapter creates a new cache adapter.
// Returns ErrNilCache if cache is nil.
func NewCacheAdapter(cache *RuleCache) (*CacheAdapter, error) {
	if cache == nil {
		return nil, ErrNilCache
	}

	return &CacheAdapter{cache: cache}, nil
}

// GetActiveRules returns active rules from the cache, optionally filtered by scope.
// Returns constant.ErrRuleCacheNotReady if the cache has not been populated yet.
// Satisfies the query.ActiveRulesRepository interface.
func (a *CacheAdapter) GetActiveRules(ctx context.Context, txScope *model.Scope) ([]*model.Rule, error) {
	if !a.cache.IsReady(ctx) {
		return nil, constant.ErrRuleCacheNotReady
	}

	cachedRules := a.cache.GetActiveRules(ctx, txScope)
	rules := make([]*model.Rule, 0, len(cachedRules))

	for _, cr := range cachedRules {
		if cr == nil || cr.Rule == nil {
			continue // defense-in-depth
		}

		// Pass compiled program through to the evaluator via transient field,
		// avoiding recompilation on the hot evaluation path.
		cr.Rule.CompiledProgram = cr.Program

		rules = append(rules, cr.Rule)
	}

	return rules, nil
}
