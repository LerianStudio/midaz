// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

//go:build integration

package workers_test

import (
	"context"
	"database/sql"
	"fmt"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"

	"github.com/LerianStudio/midaz/v3/components/tracer/internal/adapters/postgres"
	"github.com/LerianStudio/midaz/v3/components/tracer/internal/services/cache"
	"github.com/LerianStudio/midaz/v3/components/tracer/internal/services/workers"
	"github.com/LerianStudio/midaz/v3/components/tracer/internal/testutil"
	"github.com/LerianStudio/midaz/v3/components/tracer/pkg/clock"
	"github.com/LerianStudio/midaz/v3/components/tracer/pkg/resilience"
)

// testIntegrationSetup holds shared state for integration tests.
type testIntegrationSetup struct {
	container  *tcpostgres.PostgresContainer
	db         *sql.DB
	connStr    string
	terminated bool // set when the test terminates the container manually
}

// newTestIntegrationSetup starts a PostgreSQL testcontainer and runs the schema migration
// needed for the rules table.
func newTestIntegrationSetup(t *testing.T) *testIntegrationSetup {
	t.Helper()

	ctx := context.Background()

	container, err := tcpostgres.Run(ctx,
		"postgres:16-alpine",
		tcpostgres.WithDatabase("tracer_test"),
		tcpostgres.WithUsername("tracer"),
		tcpostgres.WithPassword("tracer"),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2).
				WithStartupTimeout(60*time.Second),
		),
	)
	require.NoError(t, err, "failed to start postgres container")

	connStr, err := container.ConnectionString(ctx, "sslmode=disable")
	require.NoError(t, err)

	db, err := sql.Open("pgx", connStr)
	require.NoError(t, err)
	require.NoError(t, db.Ping())

	// Create minimal rules table schema for integration tests
	_, err = db.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS rules (
			id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			name TEXT NOT NULL,
			description TEXT,
			expression TEXT NOT NULL DEFAULT '',
			action TEXT NOT NULL DEFAULT 'DENY',
			scopes JSONB NOT NULL DEFAULT '[]',
			status TEXT NOT NULL DEFAULT 'DRAFT',
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			activated_at TIMESTAMPTZ,
			deactivated_at TIMESTAMPTZ,
			deleted_at TIMESTAMPTZ
		)
	`)
	require.NoError(t, err, "failed to create rules table")

	setup := &testIntegrationSetup{
		container: container,
		db:        db,
		connStr:   connStr,
	}

	t.Cleanup(func() {
		db.Close()

		if setup.terminated {
			return
		}

		cleanupCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		if termErr := container.Terminate(cleanupCtx); termErr != nil {
			t.Logf("failed to terminate container: %v", termErr)
		}
	})

	return setup
}

// seedActiveRules inserts active rules into the database.
func (s *testIntegrationSetup) seedActiveRules(t *testing.T, count int) {
	t.Helper()

	for i := 0; i < count; i++ {
		_, err := s.db.Exec(
			`INSERT INTO rules (name, expression, action, status, scopes, activated_at)
			 VALUES ($1, $2, $3, $4, $5, NOW())`,
			fmt.Sprintf("test-rule-%d", i),
			"amount > 1000",
			"DENY",
			"ACTIVE",
			"[]",
		)
		require.NoError(t, err, "failed to seed rule %d", i)
	}
}

// Integration test -- Kill DB -> circuit trips -> stale cache served

func TestIntegration_CircuitBreaker_DBFailure(t *testing.T) {
	setup := newTestIntegrationSetup(t)
	ctx := context.Background()

	// 1. Seed 5 active rules
	setup.seedActiveRules(t, 5)

	// 2. Create real cache and warm up (use RealClock so the worker ticker fires)
	clk := clock.RealClock{}
	ruleCache := cache.NewRuleCache(clk)

	// Create real sync repo using DB adapter
	dbAdapter := &testutil.IntegrationDBAdapter{DB: setup.db}
	syncRepo := postgres.NewRuleSyncRepositoryWithConnection(dbAdapter)

	// Warm up cache manually
	allRules, err := syncRepo.GetAllActiveRules(ctx)
	require.NoError(t, err)
	require.Len(t, allRules, 5)

	cachedRules := make([]*cache.CachedRule, len(allRules))
	for i, r := range allRules {
		cachedRules[i] = &cache.CachedRule{Rule: r, Program: "compiled:" + r.Expression}
	}

	ruleCache.SetRules(ctx, cachedRules)
	ruleCache.MarkReady(ctx)

	// 3. Create sync worker with real circuit breaker (fast thresholds)
	logger := testutil.NewMockLogger()

	// Use a no-op compiler since we don't need real CEL in this test
	compiler := &noopCompiler{}

	syncCfg := workers.RuleSyncWorkerConfig{
		PollInterval:       100 * time.Millisecond,
		StalenessThreshold: 500 * time.Millisecond,
		OverlapBuffer:      50 * time.Millisecond,
	}

	cbCfg := resilience.CircuitBreakerConfig{
		Name:          "test_integration",
		MaxRequests:   1,
		Interval:      0,
		Timeout:       500 * time.Millisecond, // fast for tests
		FailureThresh: 3,
		FailureRatio:  0,
		MinRequests:   0,
	}
	cb := resilience.NewCircuitBreaker(cbCfg, logger)

	worker, err := workers.NewRuleSyncWorker(ruleCache, syncRepo, compiler, syncCfg, logger, cb, clk, "")
	require.NoError(t, err)

	// 4. Verify initial sync works (circuit closed)
	workerCtx, cancelWorker := context.WithCancel(ctx)
	defer cancelWorker()

	go func() {
		_ = worker.RunWithContext(workerCtx)
	}()

	// Wait for a couple of successful sync cycles (condition-based, not time-based)
	require.Eventually(t, func() bool {
		return !cb.IsOpen()
	}, 5*time.Second, 50*time.Millisecond, "circuit should be closed with healthy DB")

	// 5. Stop PostgreSQL container (simulate DB failure)
	terminateCtx, cancelTerminate := context.WithTimeout(ctx, 10*time.Second)
	defer cancelTerminate()

	err = setup.container.Terminate(terminateCtx)
	require.NoError(t, err, "failed to stop postgres container")

	setup.terminated = true

	// 6. Wait for circuit to open (3 failures at 100ms interval = ~300ms + margin)
	require.Eventually(t, func() bool {
		return cb.IsOpen()
	}, 5*time.Second, 100*time.Millisecond, "circuit should open after DB becomes unavailable")

	// 7. Verify cache still has original rules (stale but serving)
	cachedResult := ruleCache.GetActiveRules(ctx, nil)
	assert.Len(t, cachedResult, 5, "cache should still serve original 5 rules despite DB failure")
	assert.True(t, ruleCache.IsReady(ctx), "cache should still report ready (DEGRADED, not NOT_READY)")

	cancelWorker()
}

// Integration test -- DEGRADED health -> recovery cycle

func TestIntegration_CircuitBreaker_HealthRecovery(t *testing.T) {
	setup := newTestIntegrationSetup(t)
	ctx := context.Background()

	// 1. Seed 3 active rules
	setup.seedActiveRules(t, 3)

	// 2. Create real cache and warm up
	clk := &testutil.MockClock{FixedTime: time.Now().UTC()}
	ruleCache := cache.NewRuleCache(clk)

	dbAdapter := &testutil.IntegrationDBAdapter{DB: setup.db}
	syncRepo := postgres.NewRuleSyncRepositoryWithConnection(dbAdapter)

	allRules, err := syncRepo.GetAllActiveRules(ctx)
	require.NoError(t, err)
	require.Len(t, allRules, 3)

	cachedRules := make([]*cache.CachedRule, len(allRules))
	for i, r := range allRules {
		cachedRules[i] = &cache.CachedRule{Rule: r, Program: "compiled:" + r.Expression}
	}

	ruleCache.SetRules(ctx, cachedRules)
	ruleCache.MarkReady(ctx)

	// 3. Verify READY state
	assert.True(t, ruleCache.IsReady(ctx))
	assert.Equal(t, time.Duration(0), ruleCache.Staleness(ctx))

	// 4. Advance clock past staleness threshold to simulate what happens when DB is down
	baseTime := clk.FixedTime
	clk.FixedTime = baseTime.Add(60 * time.Second)

	// 5. Verify DEGRADED state: ready but stale
	assert.True(t, ruleCache.IsReady(ctx), "cache should still be ready when stale (DEGRADED)")
	assert.Equal(t, 60*time.Second, ruleCache.Staleness(ctx), "cache should be stale")

	// 6. Simulate recovery: sync touches cache
	ruleCache.ApplyChanges(ctx, nil, nil) // touch lastSyncTime at advanced clock

	// 7. Verify recovery: staleness resets
	assert.Equal(t, time.Duration(0), ruleCache.Staleness(ctx),
		"staleness should reset after successful sync (recovery)")
	assert.True(t, ruleCache.IsReady(ctx), "cache should report ready after recovery")
}

// noopCompiler satisfies workers.ExpressionCompiler for integration tests.
type noopCompiler struct{}

func (c *noopCompiler) Compile(_ context.Context, expression string) (any, error) {
	return "compiled:" + expression, nil
}
