// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package cache

import (
	"context"
	"fmt"
	"time"

	libLog "github.com/LerianStudio/lib-observability/log"

	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/clock"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
)

// WarmUp loads all active rules from the database, compiles their CEL expressions,
// populates the cache, and marks it as ready.
// MUST complete successfully before the instance reports READY.
// Fail-fast: any rule compilation error aborts the entire warm-up.
// Uses the provided clock for timing (enables deterministic tests).
func WarmUp(ctx context.Context, c *RuleCache, repo RuleSyncRepository, compiler ExpressionCompiler, logger libLog.Logger, clk clock.Clock) (int, time.Duration, error) {
	if clk == nil {
		clk = clock.New()
	}

	start := clk.Now()

	if c == nil {
		return 0, 0, ErrNilCache
	}

	if repo == nil {
		return 0, 0, ErrNilRepository
	}

	if compiler == nil {
		return 0, 0, ErrNilCompiler
	}

	if logger == nil {
		return 0, 0, ErrNilLogger
	}

	logger.With(
		libLog.String("operation", "cache.warmup"),
	).Log(ctx, libLog.LevelInfo, "Starting rule cache warm-up")

	rules, err := repo.GetAllActiveRules(ctx)
	if err != nil {
		return 0, clk.Now().Sub(start), fmt.Errorf("%w: %w", constant.ErrRuleCacheWarmUpFailed, err)
	}

	cachedRules := make([]*CachedRule, 0, len(rules))

	for _, rule := range rules {
		if err := ctx.Err(); err != nil {
			return 0, clk.Now().Sub(start), fmt.Errorf("%w: %w", constant.ErrRuleCacheWarmUpFailed, err)
		}

		if rule == nil {
			logger.With(
				libLog.String("operation", "cache.warmup"),
			).Log(ctx, libLog.LevelWarn, "Skipping nil rule from repository")

			continue
		}

		program, compileErr := compiler.Compile(ctx, rule.Expression)
		if compileErr != nil {
			logger.With(
				libLog.String("operation", "cache.warmup"),
				libLog.String("rule.id", rule.ID.String()),
				libLog.String("error.message", compileErr.Error()),
			).Log(ctx, libLog.LevelError, "Failed to compile rule expression — aborting warmup")

			return 0, clk.Now().Sub(start), fmt.Errorf("%w: rule %s failed to compile: %w",
				constant.ErrRuleCacheWarmUpFailed, rule.ID.String(), compileErr)
		}

		cachedRules = append(cachedRules, &CachedRule{
			Rule:    rule,
			Program: program,
		})
	}

	c.SetRules(ctx, cachedRules)
	c.MarkReady(ctx)

	duration := clk.Now().Sub(start)

	logger.With(
		libLog.String("operation", "cache.warmup"),
		libLog.Int("rules.total", len(rules)),
		libLog.Int("rules.cached", len(cachedRules)),
		libLog.Int("rules.skipped", len(rules)-len(cachedRules)),
		libLog.Any("duration_ms", duration.Milliseconds()),
	).Log(ctx, libLog.LevelInfo, "Rule cache warm-up completed")

	return len(cachedRules), duration, nil
}
