// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

//go:build integration

package workers_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	libObservability "github.com/LerianStudio/lib-observability"
	libMetrics "github.com/LerianStudio/lib-observability/metrics"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"

	cel "github.com/LerianStudio/midaz/v3/components/tracer/internal/adapters/cel"
	"github.com/LerianStudio/midaz/v3/components/tracer/internal/adapters/postgres"
	"github.com/LerianStudio/midaz/v3/components/tracer/internal/services/cache"
	"github.com/LerianStudio/midaz/v3/components/tracer/internal/services/workers"
	"github.com/LerianStudio/midaz/v3/components/tracer/internal/testutil"
	"github.com/LerianStudio/midaz/v3/components/tracer/pkg/clock"
	"github.com/LerianStudio/midaz/v3/components/tracer/pkg/resilience"
)

// newIntegrationCircuitBreaker returns a permissive circuit breaker for integration tests.
func newIntegrationCircuitBreaker() *resilience.CircuitBreaker {
	cfg := resilience.CircuitBreakerConfig{
		Name:          "test_integration_polling",
		MaxRequests:   1,
		Interval:      0,
		Timeout:       1 * time.Second,
		FailureThresh: 5,
		FailureRatio:  0,
		MinRequests:   0,
	}

	return resilience.NewCircuitBreaker(cfg, testutil.NewMockLogger())
}

// celCompilerAdapter wraps cel.Adapter to satisfy workers.ExpressionCompiler interface.
// Mirrors the same adapter in bootstrap/config.go (unexported there).
type celCompilerAdapter struct {
	adapter *cel.Adapter
}

func (c *celCompilerAdapter) Compile(ctx context.Context, expression string) (any, error) {
	return c.adapter.Compile(ctx, expression)
}

// --- Test 1: Polling End-to-End (insert, update, delete) ---

func TestIntegration_Polling_EndToEnd_InsertUpdateDelete(t *testing.T) {
	setup := newTestIntegrationSetup(t)
	ctx := context.Background()

	// 1. Seed 3 initial active rules
	setup.seedActiveRules(t, 3)

	// 2. Create real components
	clk := clock.RealClock{}
	ruleCache := cache.NewRuleCache(clk)

	dbAdapter := &testutil.IntegrationDBAdapter{DB: setup.db}
	syncRepo := postgres.NewRuleSyncRepositoryWithConnection(dbAdapter)

	// Warm up cache
	allRules, err := syncRepo.GetAllActiveRules(ctx)
	require.NoError(t, err)
	require.Len(t, allRules, 3)

	cachedRules := make([]*cache.CachedRule, len(allRules))
	for i, r := range allRules {
		cachedRules[i] = &cache.CachedRule{Rule: r, Program: "compiled:" + r.Expression}
	}

	ruleCache.SetRules(context.Background(), cachedRules)
	ruleCache.MarkReady(context.Background())

	// 3. Create worker with fast polling
	logger := testutil.NewMockLogger()
	compiler := &noopCompiler{}

	syncCfg := workers.RuleSyncWorkerConfig{
		PollInterval:       100 * time.Millisecond,
		StalenessThreshold: 5 * time.Second,
		OverlapBuffer:      50 * time.Millisecond,
	}

	worker, err := workers.NewRuleSyncWorker(ruleCache, syncRepo, compiler, syncCfg, logger, newIntegrationCircuitBreaker(), clk, "")
	require.NoError(t, err)

	// 4. Start worker
	workerCtx, cancelWorker := context.WithCancel(ctx)
	defer cancelWorker()

	go func() {
		_ = worker.RunWithContext(workerCtx)
	}()

	// Wait for initial sync cycle to complete
	require.Eventually(t, func() bool {
		return ruleCache.Size(context.Background()) == 3
	}, 5*time.Second, 50*time.Millisecond, "cache should have 3 initial rules")

	// --- INSERT: Add 2 new rules ---
	for i := 10; i < 12; i++ {
		_, err := setup.db.Exec(
			`INSERT INTO rules (name, expression, action, status, scopes, activated_at)
			 VALUES ($1, $2, $3, $4, $5, NOW())`,
			fmt.Sprintf("new-rule-%d", i),
			"amount > 5000",
			"DENY",
			"ACTIVE",
			"[]",
		)
		require.NoError(t, err)
	}

	require.Eventually(t, func() bool {
		return ruleCache.Size(context.Background()) == 5
	}, 5*time.Second, 100*time.Millisecond, "cache should have 5 rules after INSERT")

	// --- UPDATE: Change expression of first inserted rule ---
	_, err = setup.db.Exec(
		`UPDATE rules SET expression = 'amount > 9999', updated_at = NOW()
		 WHERE name = 'new-rule-10'`)
	require.NoError(t, err)

	require.Eventually(t, func() bool {
		rules := ruleCache.GetActiveRules(context.Background(), nil)
		for _, r := range rules {
			if r.Rule.Name == "new-rule-10" && r.Rule.Expression == "amount > 9999" {
				return true
			}
		}

		return false
	}, 5*time.Second, 100*time.Millisecond, "cache should reflect updated expression")

	// --- DELETE: Deactivate a rule (set status=INACTIVE) ---
	_, err = setup.db.Exec(
		`UPDATE rules SET status = 'INACTIVE', deactivated_at = NOW(), updated_at = NOW()
		 WHERE name = 'new-rule-11'`)
	require.NoError(t, err)

	require.Eventually(t, func() bool {
		return ruleCache.Size(context.Background()) == 4
	}, 5*time.Second, 100*time.Millisecond, "cache should have 4 rules after deactivation")

	// Verify the deactivated rule is not in cache
	rules := ruleCache.GetActiveRules(context.Background(), nil)
	for _, r := range rules {
		assert.NotEqual(t, "new-rule-11", r.Rule.Name,
			"deactivated rule should not be in cache")
	}

	cancelWorker()
}

// --- Test 2: Overlap Buffer — no duplication ---

func TestIntegration_Polling_OverlapBuffer_NoDuplication(t *testing.T) {
	setup := newTestIntegrationSetup(t)
	ctx := context.Background()

	// 1. Seed 5 rules
	setup.seedActiveRules(t, 5)

	// 2. Create components with a LARGE overlap buffer
	// This ensures rules are re-fetched on every cycle
	clk := clock.RealClock{}
	ruleCache := cache.NewRuleCache(clk)

	dbAdapter := &testutil.IntegrationDBAdapter{DB: setup.db}
	syncRepo := postgres.NewRuleSyncRepositoryWithConnection(dbAdapter)

	// Warm up
	allRules, err := syncRepo.GetAllActiveRules(ctx)
	require.NoError(t, err)

	cachedRules := make([]*cache.CachedRule, len(allRules))
	for i, r := range allRules {
		cachedRules[i] = &cache.CachedRule{Rule: r, Program: "compiled:" + r.Expression}
	}

	ruleCache.SetRules(context.Background(), cachedRules)
	ruleCache.MarkReady(context.Background())

	// Large overlap buffer (1 hour) guarantees all rules are re-fetched every cycle
	syncCfg := workers.RuleSyncWorkerConfig{
		PollInterval:       100 * time.Millisecond,
		StalenessThreshold: 5 * time.Second,
		OverlapBuffer:      1 * time.Hour,
	}

	logger := testutil.NewMockLogger()
	compiler := &noopCompiler{}

	worker, err := workers.NewRuleSyncWorker(ruleCache, syncRepo, compiler, syncCfg, logger, newIntegrationCircuitBreaker(), clk, "")
	require.NoError(t, err)

	// 3. Run worker for several cycles
	workerCtx, cancelWorker := context.WithCancel(ctx)
	defer cancelWorker()

	go func() {
		_ = worker.RunWithContext(workerCtx)
	}()

	// Wait for at least 5 poll cycles (500ms at 100ms interval)
	time.Sleep(600 * time.Millisecond)

	// 4. Assert: cache still has exactly 5 rules — no duplicates
	assert.Equal(t, 5, ruleCache.Size(context.Background()),
		"cache should have exactly 5 rules after multiple overlap re-fetches")

	// Verify uniqueness by ID
	rules := ruleCache.GetActiveRules(context.Background(), nil)
	assert.Len(t, rules, 5, "GetActiveRules should return exactly 5 rules")

	idSet := make(map[string]bool)
	for _, r := range rules {
		id := r.Rule.ID.String()
		assert.False(t, idSet[id], "duplicate rule ID found: %s", id)
		idSet[id] = true
	}

	cancelWorker()
}

// --- Test 3: Real CEL Compilation ---

func TestIntegration_Polling_RealCELCompilation(t *testing.T) {
	setup := newTestIntegrationSetup(t)
	ctx := context.Background()

	// 1. Seed rules with real CEL expressions
	expressions := []struct {
		name string
		expr string
	}{
		{"amount-check", "amount > 1000"},
		{"type-check", `transactionType == "PIX"`},
		{"combined-check", `amount > 500 && transactionType == "WIRE"`},
	}

	for _, e := range expressions {
		_, err := setup.db.Exec(
			`INSERT INTO rules (name, expression, action, status, scopes, activated_at)
			 VALUES ($1, $2, $3, $4, $5, NOW())`,
			e.name, e.expr, "DENY", "ACTIVE", "[]",
		)
		require.NoError(t, err)
	}

	// 2. Create real CEL compiler
	celAdapter, err := cel.NewAdapter(cel.AdapterConfig{CostLimit: 10000}, logger(t))
	require.NoError(t, err)

	realCompiler := &celCompilerAdapter{adapter: celAdapter}

	// 3. Create cache and warm up with real compilation
	clk := clock.RealClock{}
	ruleCache := cache.NewRuleCache(clk)

	dbAdapter := &testutil.IntegrationDBAdapter{DB: setup.db}
	syncRepo := postgres.NewRuleSyncRepositoryWithConnection(dbAdapter)

	allRules, err := syncRepo.GetAllActiveRules(ctx)
	require.NoError(t, err)
	require.Len(t, allRules, 3)

	// Warm up with real CEL compilation
	cachedRules := make([]*cache.CachedRule, len(allRules))
	for i, r := range allRules {
		program, compileErr := realCompiler.Compile(ctx, r.Expression)
		require.NoError(t, compileErr, "warm-up compile should succeed for: %s", r.Expression)

		cachedRules[i] = &cache.CachedRule{Rule: r, Program: program}
	}

	ruleCache.SetRules(context.Background(), cachedRules)
	ruleCache.MarkReady(context.Background())

	// 4. Insert a new rule and start worker to detect it
	_, err = setup.db.Exec(
		`INSERT INTO rules (name, expression, action, status, scopes, activated_at)
		 VALUES ($1, $2, $3, $4, $5, NOW())`,
		"new-cel-rule", `amount >= 100 && currency == "BRL"`, "DENY", "ACTIVE", "[]",
	)
	require.NoError(t, err)

	syncCfg := workers.RuleSyncWorkerConfig{
		PollInterval:       100 * time.Millisecond,
		StalenessThreshold: 5 * time.Second,
		OverlapBuffer:      50 * time.Millisecond,
	}

	worker, err := workers.NewRuleSyncWorker(ruleCache, syncRepo, realCompiler, syncCfg, logger(t), newIntegrationCircuitBreaker(), clk, "")
	require.NoError(t, err)

	workerCtx, cancelWorker := context.WithCancel(ctx)
	defer cancelWorker()

	go func() {
		_ = worker.RunWithContext(workerCtx)
	}()

	// Wait for new rule to appear in cache
	require.Eventually(t, func() bool {
		return ruleCache.Size(context.Background()) == 4
	}, 5*time.Second, 100*time.Millisecond, "cache should detect the new rule")

	cancelWorker()

	// 5. Verify all cached programs are real compiled programs (not nil, not strings)
	rules := ruleCache.GetActiveRules(context.Background(), nil)
	require.Len(t, rules, 4)

	for _, r := range rules {
		require.NotNil(t, r.Program,
			"rule %q should have a compiled program, got nil", r.Rule.Name)

		// Real CEL compilation returns *cel.CompiledProgram, not a string
		_, isString := r.Program.(string)
		assert.False(t, isString,
			"rule %q program should be *cel.CompiledProgram, not string", r.Rule.Name)

		_, isCompiledProgram := r.Program.(*cel.CompiledProgram)
		assert.True(t, isCompiledProgram,
			"rule %q program should be *cel.CompiledProgram, got %T", r.Rule.Name, r.Program)
	}
}

// --- Test 4: Metrics Emission with Real OTel SDK ---

func TestIntegration_Polling_MetricsEmission(t *testing.T) {
	setup := newTestIntegrationSetup(t)

	// 1. Set up in-memory OTel metric reader
	reader := sdkmetric.NewManualReader()
	provider := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))

	defer func() {
		_ = provider.Shutdown(context.Background())
	}()

	meter := provider.Meter("tracer-integration-test")
	mockLogger := testutil.NewMockLogger()

	// Create real MetricsFactory backed by in-memory reader
	metricsFactory, err := libMetrics.NewMetricsFactory(meter, mockLogger)
	require.NoError(t, err, "NewMetricsFactory should not fail")

	// Build context with metrics factory and tracer
	traceCleanup := setupIntegrationTracer(t)
	defer traceCleanup()

	ctx := context.Background()
	ctx = libObservability.ContextWithMetricFactory(ctx, metricsFactory)

	// 2. Seed rules and create worker
	setup.seedActiveRules(t, 3)

	clk := clock.RealClock{}
	ruleCache := cache.NewRuleCache(clk)

	dbAdapter := &testutil.IntegrationDBAdapter{DB: setup.db}
	syncRepo := postgres.NewRuleSyncRepositoryWithConnection(dbAdapter)

	allRules, err := syncRepo.GetAllActiveRules(ctx)
	require.NoError(t, err)

	cachedRules := make([]*cache.CachedRule, len(allRules))
	for i, r := range allRules {
		cachedRules[i] = &cache.CachedRule{Rule: r, Program: "compiled:" + r.Expression}
	}

	ruleCache.SetRules(context.Background(), cachedRules)
	ruleCache.MarkReady(context.Background())

	syncCfg := workers.RuleSyncWorkerConfig{
		PollInterval:       100 * time.Millisecond,
		StalenessThreshold: 5 * time.Second,
		OverlapBuffer:      50 * time.Millisecond,
	}

	compiler := &noopCompiler{}
	worker, err := workers.NewRuleSyncWorker(ruleCache, syncRepo, compiler, syncCfg, mockLogger, newIntegrationCircuitBreaker(), clk, "")
	require.NoError(t, err)

	// 3. Run worker with enriched context (carries metrics factory)
	workerCtx, cancelWorker := context.WithCancel(ctx)
	defer cancelWorker()

	go func() {
		_ = worker.RunWithContext(workerCtx)
	}()

	// Wait for at least 3 poll cycles
	time.Sleep(400 * time.Millisecond)
	cancelWorker()

	// 4. Collect and assert metrics
	var rm metricdata.ResourceMetrics
	err = reader.Collect(context.Background(), &rm)
	require.NoError(t, err)

	// Find our metrics in the collected data
	metricsByName := flattenMetrics(rm)

	// polls_total counter should have been incremented (success polls)
	pollsMetric, found := metricsByName["tracer_cache_sync_polls_total"]
	assert.True(t, found, "tracer_cache_sync_polls_total metric should be present")

	if found {
		assert.Greater(t, sumCounterDataPoints(pollsMetric), int64(0),
			"polls_total should have recorded at least one poll")
	}

	// duration histogram should have recordings
	durationMetric, found := metricsByName["tracer_cache_sync_duration_milliseconds"]
	assert.True(t, found, "tracer_cache_sync_duration_milliseconds metric should be present")

	if found {
		assert.Greater(t, countHistogramDataPoints(durationMetric), 0,
			"duration histogram should have at least one recording")
	}

	// cache size gauge should be set
	cacheSizeMetric, found := metricsByName["tracer_cache_sync_rule_cache_size"]
	assert.True(t, found, "tracer_cache_sync_rule_cache_size metric should be present")

	if found {
		lastGaugeValue := lastGaugeDataPoint(cacheSizeMetric)
		assert.Equal(t, int64(3), lastGaugeValue,
			"cache size gauge should reflect 3 rules")
	}

	// staleness gauge should be 0 (successful polls)
	stalenessMetric, found := metricsByName["tracer_cache_sync_staleness_seconds"]
	assert.True(t, found, "tracer_cache_sync_staleness_seconds metric should be present")

	if found {
		lastVal := lastGaugeDataPoint(stalenessMetric)
		assert.Equal(t, int64(0), lastVal,
			"staleness should be 0 after successful sync")
	}
}

// --- Helpers ---

// setupIntegrationTracer configures a test tracer provider.
// Mirrors setupTestTracer from usage_cleanup_worker_test.go (internal package).
func setupIntegrationTracer(t *testing.T) func() {
	t.Helper()

	exporter := tracetest.NewInMemoryExporter()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSyncer(exporter))
	otel.SetTracerProvider(tp)

	return func() {
		if err := tp.Shutdown(context.Background()); err != nil {
			t.Logf("tp.Shutdown error: %v", err)
		}
	}
}

// logger creates a test-compatible logger using the mock logger.
// Used for CEL adapter which requires a non-nil logger.
func logger(t *testing.T) *testutil.MockLogger {
	t.Helper()

	return testutil.NewMockLogger()
}

// flattenMetrics extracts all metrics from ResourceMetrics into a name→Metrics map.
func flattenMetrics(rm metricdata.ResourceMetrics) map[string]metricdata.Metrics {
	result := make(map[string]metricdata.Metrics)

	for _, sm := range rm.ScopeMetrics {
		for _, m := range sm.Metrics {
			result[m.Name] = m
		}
	}

	return result
}

// sumCounterDataPoints sums all data points in a counter metric.
func sumCounterDataPoints(m metricdata.Metrics) int64 {
	switch data := m.Data.(type) {
	case metricdata.Sum[int64]:
		var total int64
		for _, dp := range data.DataPoints {
			total += dp.Value
		}

		return total
	case metricdata.Sum[float64]:
		var total float64
		for _, dp := range data.DataPoints {
			total += dp.Value
		}

		return int64(total)
	}

	return 0
}

// countHistogramDataPoints returns the number of data points in a histogram.
func countHistogramDataPoints(m metricdata.Metrics) int {
	if data, ok := m.Data.(metricdata.Histogram[int64]); ok {
		return len(data.DataPoints)
	}

	if data, ok := m.Data.(metricdata.Histogram[float64]); ok {
		return len(data.DataPoints)
	}

	return 0
}

// lastGaugeDataPoint returns the last recorded value of a gauge metric.
func lastGaugeDataPoint(m metricdata.Metrics) int64 {
	switch data := m.Data.(type) {
	case metricdata.Gauge[int64]:
		if len(data.DataPoints) > 0 {
			return data.DataPoints[len(data.DataPoints)-1].Value
		}
	case metricdata.Gauge[float64]:
		if len(data.DataPoints) > 0 {
			return int64(data.DataPoints[len(data.DataPoints)-1].Value)
		}
	}

	return -1
}
