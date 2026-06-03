// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/LerianStudio/midaz/v3/components/tracer/internal/services/cache"
	cachemocks "github.com/LerianStudio/midaz/v3/components/tracer/internal/services/cache/mocks"
	"github.com/LerianStudio/midaz/v3/components/tracer/internal/testutil"
	"github.com/LerianStudio/midaz/v3/components/tracer/pkg/clock"
	"github.com/LerianStudio/midaz/v3/components/tracer/pkg/model"
)

// TestConditionalWarmUpCache_SingleTenantCallsWarmUp verifies that the
// normal (non-MT) path still performs a full cache warmup and the
// RuleSyncRepository is invoked.
func TestConditionalWarmUpCache_SingleTenantCallsWarmUp(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)

	syncRepo := cachemocks.NewMockRuleSyncRepository(ctrl)
	// Single-tenant: GetAllActiveRules MUST be called — if not, warmup was
	// silently skipped and the cache starts empty.
	syncRepo.EXPECT().
		GetAllActiveRules(gomock.Any()).
		Return([]*model.Rule{}, nil).
		Times(1)

	compiler := cachemocks.NewMockExpressionCompiler(ctrl)

	clk := clock.RealClock{}
	ruleCache := cache.NewRuleCache(clk)
	logger := testutil.NewMockLogger()

	cfg := &Config{MultiTenantEnabled: false}

	err := conditionalWarmUpCache(context.Background(), cfg, ruleCache, syncRepo, compiler, logger, clk)
	require.NoError(t, err)

	assert.True(t, ruleCache.IsReady(context.Background()),
		"single-tenant warmup must mark the cache as ready")
}

// TestConditionalWarmUpCache_MultiTenantSkipsWarmUp verifies H15: in MT
// mode the RuleSyncRepository is NOT called at bootstrap. Per-tenant
// workers populate each tenant's bucket on first cycle; loading the root
// DB into the "" bucket would be wasted work and misleading.
//
// The mock sets no expectations for GetAllActiveRules — gomock makes any
// unexpected call fail the test, giving us a strict assertion that warmup
// was bypassed.
func TestConditionalWarmUpCache_MultiTenantSkipsWarmUp(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)

	syncRepo := cachemocks.NewMockRuleSyncRepository(ctrl)
	// Deliberately NO expectations. Any call fails the test.

	compiler := cachemocks.NewMockExpressionCompiler(ctrl)

	clk := clock.RealClock{}
	ruleCache := cache.NewRuleCache(clk)
	logger := testutil.NewMockLogger()

	cfg := &Config{MultiTenantEnabled: true}

	err := conditionalWarmUpCache(context.Background(), cfg, ruleCache, syncRepo, compiler, logger, clk)
	require.NoError(t, err)
}

// TestConditionalWarmUpCache_MultiTenantEvictsEmptyBucket verifies the
// defensive eviction: even if some earlier code path in MT mode seeded the
// "" bucket, conditionalWarmUpCache clears it so no tenant-less state
// lingers in the cache after bootstrap.
func TestConditionalWarmUpCache_MultiTenantEvictsEmptyBucket(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)

	syncRepo := cachemocks.NewMockRuleSyncRepository(ctrl)
	compiler := cachemocks.NewMockExpressionCompiler(ctrl)

	clk := clock.RealClock{}
	ruleCache := cache.NewRuleCache(clk)

	// Seed the "" bucket as if an earlier misbehaving path had populated it.
	// UpsertRule with bare context.Background() writes to the empty bucket.
	ruleCache.UpsertRule(context.Background(), newWarmUpTestRule(), "compiled")
	require.NotEmpty(t, ruleCache.GetActiveRules(context.Background(), nil),
		"pre-condition: empty bucket should have been seeded")

	logger := testutil.NewMockLogger()
	cfg := &Config{MultiTenantEnabled: true}

	err := conditionalWarmUpCache(context.Background(), cfg, ruleCache, syncRepo, compiler, logger, clk)
	require.NoError(t, err)

	assert.Empty(t, ruleCache.GetActiveRules(context.Background(), nil),
		"MT bootstrap must defensively evict the empty-tenant bucket")
}

// newWarmUpTestRule builds a minimal *model.Rule suitable for cache upsert
// assertions. Deterministic — no time.Now or uuid.New.
func newWarmUpTestRule() *model.Rule {
	r, err := model.NewRule(
		"warmup-test",
		"true",
		model.DecisionAllow,
		nil, // scopes
		nil, // description
		testutil.FixedTime(),
	)
	if err != nil {
		panic(err)
	}

	return r
}
