// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

//go:build integration

package cache_test

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"tracer/internal/adapters/postgres"
	"tracer/internal/services/cache"
	"tracer/internal/services/cache/mocks"
	"tracer/internal/testutil"
	"tracer/pkg/clock"

	"go.uber.org/mock/gomock"
)

// seedActiveRule inserts a rule directly into the database and returns its ID.
func seedActiveRule(t *testing.T, db *sql.DB, name, expression string) uuid.UUID {
	t.Helper()

	id := uuid.New()
	now := time.Now().UTC()

	_, err := db.Exec(`
		INSERT INTO rules (id, name, description, expression, action, scopes, status, created_at, updated_at, activated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
	`, id, name, "integration test rule", expression, "DENY", "[]", "ACTIVE", now, now, now)
	require.NoError(t, err, "Failed to seed rule %s", name)

	return id
}

// cleanupRules removes all rules created by integration tests.
func cleanupRules(t *testing.T, db *sql.DB, ids ...uuid.UUID) {
	t.Helper()

	for _, id := range ids {
		_, err := db.Exec("DELETE FROM rules WHERE id = $1", id)
		if err != nil {
			t.Logf("Cleanup: failed to delete rule %s: %v", id, err)
		}
	}
}

func TestIntegration_WarmUp_FromSeededPostgreSQL(t *testing.T) {
	testutil.SetupTestTracing(t)

	db := testutil.SetupIntegrationDB(t)
	// ctx is declared at the top of the test for consistency with
	// rule_cache_multitenant_test.go — one place to tweak (e.g. add a timeout)
	// when future assertions need it.
	ctx := context.Background()

	// Seed two active rules
	id1 := seedActiveRule(t, db, "rule-integ-1", "transaction.amount > 1000")
	id2 := seedActiveRule(t, db, "rule-integ-2", "transaction.amount > 5000")

	t.Cleanup(func() {
		cleanupRules(t, db, id1, id2)
	})

	// Create real RuleSyncRepository backed by integration DB
	adapter := &testutil.IntegrationDBAdapter{DB: db}
	repo := postgres.NewRuleSyncRepositoryWithConnection(adapter)

	// Use mock compiler — we are testing the DB-to-cache path, not CEL compilation
	ctrl := gomock.NewController(t)
	mockCompiler := mocks.NewMockExpressionCompiler(ctrl)
	mockCompiler.EXPECT().Compile(gomock.Any(), gomock.Any()).Return("compiled", nil).MinTimes(2)

	logger := testutil.NewMockLogger()
	clk := clock.New()
	ruleCache := cache.NewRuleCache(clk)

	count, duration, err := cache.WarmUp(ctx, ruleCache, repo, mockCompiler, logger, clk)

	require.NoError(t, err, "WarmUp should succeed with seeded rules")
	assert.GreaterOrEqual(t, count, 2, "should have loaded at least 2 rules")
	assert.Greater(t, duration, time.Duration(0), "duration should be positive")
	assert.True(t, ruleCache.IsReady(ctx), "cache should be marked ready")
	assert.GreaterOrEqual(t, ruleCache.Size(ctx), 2, "cache should contain at least 2 rules")
}

func TestIntegration_RuleSyncRepository_GetAllActiveRules(t *testing.T) {
	testutil.SetupTestTracing(t)

	db := testutil.SetupIntegrationDB(t)
	ctx := context.Background()

	// Seed rules with different statuses
	activeID := seedActiveRule(t, db, "rule-sync-active", "true")

	inactiveID := uuid.New()
	now := time.Now().UTC()

	_, err := db.Exec(`
		INSERT INTO rules (id, name, description, expression, action, scopes, status, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
	`, inactiveID, "rule-sync-inactive", "inactive rule", "false", "DENY", "[]", "INACTIVE", now, now)
	require.NoError(t, err)

	t.Cleanup(func() {
		cleanupRules(t, db, activeID, inactiveID)
	})

	adapter := &testutil.IntegrationDBAdapter{DB: db}
	repo := postgres.NewRuleSyncRepositoryWithConnection(adapter)

	rules, err := repo.GetAllActiveRules(ctx)

	require.NoError(t, err, "GetAllActiveRules should succeed")

	// Verify only ACTIVE rules are returned
	for _, rule := range rules {
		assert.Equal(t, "ACTIVE", string(rule.Status), "all returned rules should be ACTIVE")
	}

	// Verify our active rule is in the results
	found := false

	for _, rule := range rules {
		if rule.ID == activeID {
			found = true

			break
		}
	}

	assert.True(t, found, "seeded active rule should be in results")
}

func TestIntegration_RuleSyncRepository_GetRulesUpdatedSince(t *testing.T) {
	testutil.SetupTestTracing(t)

	db := testutil.SetupIntegrationDB(t)
	ctx := context.Background()

	// Record time before seeding
	beforeSeed := time.Now().UTC().Add(-1 * time.Second)

	// Seed a rule
	id := seedActiveRule(t, db, "rule-sync-updated", "true")

	t.Cleanup(func() {
		cleanupRules(t, db, id)
	})

	adapter := &testutil.IntegrationDBAdapter{DB: db}
	repo := postgres.NewRuleSyncRepositoryWithConnection(adapter)

	rules, err := repo.GetRulesUpdatedSince(ctx, beforeSeed)

	require.NoError(t, err, "GetRulesUpdatedSince should succeed")
	assert.GreaterOrEqual(t, len(rules), 1, "should find at least 1 rule updated since beforeSeed")

	found := false

	for _, rule := range rules {
		if rule.ID == id {
			found = true

			break
		}
	}

	assert.True(t, found, "seeded rule should appear in updated-since results")
}

func TestIntegration_Migration_IndexExists(t *testing.T) {
	testutil.SetupTestTracing(t)

	db := testutil.SetupIntegrationDB(t)

	// Check if the index exists (migrations should have already run via test setup)
	var indexExists bool

	err := db.QueryRow(`
		SELECT EXISTS (
			SELECT 1 FROM pg_indexes
			WHERE indexname = 'idx_rules_updated_at'
		)
	`).Scan(&indexExists)
	require.NoError(t, err, "should be able to query pg_indexes")
	assert.True(t, indexExists, "idx_rules_updated_at should exist after migration 000005")
}
