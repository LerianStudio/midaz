// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package cache_test

import (
	"context"
	"errors"
	"testing"
	"time"

	libLog "github.com/LerianStudio/lib-observability/log"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/LerianStudio/midaz/v3/components/tracer/internal/services/cache"
	"github.com/LerianStudio/midaz/v3/components/tracer/internal/services/cache/mocks"
	"github.com/LerianStudio/midaz/v3/components/tracer/internal/testutil"
	"github.com/LerianStudio/midaz/v3/components/tracer/pkg/clock"
	"github.com/LerianStudio/midaz/v3/components/tracer/pkg/constant"
	"github.com/LerianStudio/midaz/v3/components/tracer/pkg/model"
)

func TestWarmUp_Success(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	mockRepo := mocks.NewMockRuleSyncRepository(ctrl)
	mockCompiler := mocks.NewMockExpressionCompiler(ctrl)
	logger := testutil.NewMockLogger()
	clk := clock.New()

	ctx := context.Background()
	rules := []*model.Rule{newTestRule(1), newTestRule(2)}

	mockRepo.EXPECT().GetAllActiveRules(ctx).Return(rules, nil)
	mockCompiler.EXPECT().Compile(ctx, rules[0].Expression).Return("prog1", nil)
	mockCompiler.EXPECT().Compile(ctx, rules[1].Expression).Return("prog2", nil)

	c := cache.NewRuleCache(clk)
	count, duration, err := cache.WarmUp(ctx, c, mockRepo, mockCompiler, logger, clk)

	require.NoError(t, err)
	assert.Equal(t, 2, count)
	assert.GreaterOrEqual(t, duration, time.Duration(0))
	assert.True(t, c.IsReady(ctx))
	assertCacheSize(t, c, 2)
}

func TestWarmUp_AbortsOnCompilationFailure(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	mockRepo := mocks.NewMockRuleSyncRepository(ctrl)
	mockCompiler := mocks.NewMockExpressionCompiler(ctrl)
	logger := testutil.NewMockLogger()
	clk := clock.New()

	ctx := context.Background()
	rules := []*model.Rule{newTestRule(1), newTestRule(2)}

	mockRepo.EXPECT().GetAllActiveRules(ctx).Return(rules, nil)
	mockCompiler.EXPECT().Compile(ctx, rules[0].Expression).Return("prog1", nil)
	mockCompiler.EXPECT().Compile(ctx, rules[1].Expression).Return(nil, errors.New("bad expression"))

	c := cache.NewRuleCache(clk)
	_, _, err := cache.WarmUp(ctx, c, mockRepo, mockCompiler, logger, clk)

	require.Error(t, err)
	assert.ErrorIs(t, err, constant.ErrRuleCacheWarmUpFailed)
	assert.Contains(t, err.Error(), rules[1].ID.String(), "error should reference the failing rule ID")
	assert.False(t, c.IsReady(ctx), "cache must not be marked ready on compilation failure")
}

func TestWarmUp_RepoError(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	mockRepo := mocks.NewMockRuleSyncRepository(ctrl)
	mockCompiler := mocks.NewMockExpressionCompiler(ctrl)
	logger := testutil.NewMockLogger()
	clk := clock.New()

	ctx := context.Background()
	mockRepo.EXPECT().GetAllActiveRules(ctx).Return(nil, errors.New("db down"))

	c := cache.NewRuleCache(clk)
	_, _, err := cache.WarmUp(ctx, c, mockRepo, mockCompiler, logger, clk)

	require.Error(t, err)
	assert.ErrorIs(t, err, constant.ErrRuleCacheWarmUpFailed)
}

func TestWarmUp_EmptyDatabase(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	mockRepo := mocks.NewMockRuleSyncRepository(ctrl)
	mockCompiler := mocks.NewMockExpressionCompiler(ctrl)
	logger := testutil.NewMockLogger()
	clk := clock.New()

	ctx := context.Background()
	mockRepo.EXPECT().GetAllActiveRules(ctx).Return([]*model.Rule{}, nil)

	c := cache.NewRuleCache(clk)
	count, _, err := cache.WarmUp(ctx, c, mockRepo, mockCompiler, logger, clk)

	require.NoError(t, err)
	assert.Equal(t, 0, count)
	assert.True(t, c.IsReady(ctx), "cache should be marked ready even with zero rules")
	assertCacheSize(t, c, 0)
}

func TestWarmUp_NilDependencies(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	mockRepo := mocks.NewMockRuleSyncRepository(ctrl)
	mockCompiler := mocks.NewMockExpressionCompiler(ctrl)
	logger := testutil.NewMockLogger()
	clk := clock.New()
	ctx := context.Background()
	c := cache.NewRuleCache(clk)

	tests := []struct {
		name        string
		cache       *cache.RuleCache
		repo        cache.RuleSyncRepository
		compiler    cache.ExpressionCompiler
		logger      libLog.Logger
		expectedErr error
	}{
		{"nil cache", nil, mockRepo, mockCompiler, logger, cache.ErrNilCache},
		{"nil repo", c, nil, mockCompiler, logger, cache.ErrNilRepository},
		{"nil compiler", c, mockRepo, nil, logger, cache.ErrNilCompiler},
		{"nil logger", c, mockRepo, mockCompiler, nil, cache.ErrNilLogger},
	}

	// NOTE: subtests are intentionally sequential (no t.Parallel()) because they share
	// mockRepo/mockCompiler/c instances. WarmUp nil checks short-circuit before touching them.
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, _, err := cache.WarmUp(ctx, tt.cache, tt.repo, tt.compiler, tt.logger, clk)
			require.ErrorIs(t, err, tt.expectedErr)
		})
	}
}

func TestWarmUp_ContextCancellation(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	mockRepo := mocks.NewMockRuleSyncRepository(ctrl)
	mockCompiler := mocks.NewMockExpressionCompiler(ctrl)
	logger := testutil.NewMockLogger()
	clk := clock.New()

	// Cancel context before WarmUp starts the compilation loop
	ctx, cancel := context.WithCancel(context.Background())

	// Return many rules so the loop has a chance to check ctx.Err()
	rules := make([]*model.Rule, 10)
	for i := range rules {
		rules[i] = newTestRule(int64(i + 1))
	}

	mockRepo.EXPECT().GetAllActiveRules(gomock.Any()).Return(rules, nil)
	// First compilation succeeds, then context is cancelled
	mockCompiler.EXPECT().Compile(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, _ string) (any, error) {
			cancel() // Cancel after first compile
			return "prog", nil
		}).Times(1)

	c := cache.NewRuleCache(clk)
	_, _, err := cache.WarmUp(ctx, c, mockRepo, mockCompiler, logger, clk)

	require.Error(t, err)
	assert.ErrorIs(t, err, constant.ErrRuleCacheWarmUpFailed)
}

func TestWarmUp_NilRulesFromRepo(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	mockRepo := mocks.NewMockRuleSyncRepository(ctrl)
	mockCompiler := mocks.NewMockExpressionCompiler(ctrl)
	logger := testutil.NewMockLogger()
	clk := clock.New()
	ctx := context.Background()

	validRule := newTestRule(1)
	// Repo returns a slice with a nil element mixed in
	mockRepo.EXPECT().GetAllActiveRules(ctx).Return([]*model.Rule{nil, validRule}, nil)
	mockCompiler.EXPECT().Compile(ctx, validRule.Expression).Return("prog1", nil)

	c := cache.NewRuleCache(clk)
	count, _, err := cache.WarmUp(ctx, c, mockRepo, mockCompiler, logger, clk)

	require.NoError(t, err)
	assert.Equal(t, 1, count, "should skip nil rules and cache only valid ones")
	assertCacheSize(t, c, 1)
}
