// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

//go:build integration

package postgres

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/components/tracer/internal/testutil"
	"github.com/LerianStudio/midaz/v4/components/tracer/migrations"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/model"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
)

func contextPolicyDatabase(t *testing.T, suffix string) *sql.DB {
	t.Helper()
	admin := testutil.SetupIntegrationDB(t)
	digest := sha256.Sum256([]byte(t.Name() + suffix))
	schema := fmt.Sprintf("policy_test_%x", digest[:8])
	_, err := admin.ExecContext(context.Background(), "CREATE SCHEMA "+schema)
	require.NoError(t, err)
	t.Cleanup(func() {
		_, err := admin.ExecContext(context.Background(), "DROP SCHEMA "+schema+" CASCADE")
		require.NoError(t, err)
	})
	dsn, err := url.Parse(testutil.GetTestDSN())
	require.NoError(t, err)
	args := dsn.Query()
	args.Set("search_path", schema)
	dsn.RawQuery = args.Encode()
	db, err := sql.Open("pgx", dsn.String())
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, db.Close()) })
	dir := t.TempDir()
	require.NoError(t, migrations.WriteTo(dir))
	up, err := os.ReadFile(filepath.Join(dir, "000025_context_policies.up.sql"))
	require.NoError(t, err)
	_, err = db.ExecContext(context.Background(), string(up))
	require.NoError(t, err)
	return db
}

func storedContextPolicy() model.ContextPolicy {
	return model.ContextPolicy{
		ID: testutil.MustDeterministicUUID(60001), Revision: 1, DefaultDecision: model.DecisionDeny,
		Rules: []model.ContextPolicyRule{
			{ID: testutil.MustDeterministicUUID(60003), Revision: 1, Expression: "true", Action: model.DecisionReview},
			{ID: testutil.MustDeterministicUUID(60002), Revision: 1, Expression: "false", Action: model.DecisionDeny},
		},
	}
}

func TestIntegrationContextPolicyBindingAndImmutability(t *testing.T) {
	db := contextPolicyDatabase(t, "a")
	repo, err := NewContextPolicyRepository(&testutil.IntegrationDBAdapter{DB: db}, 10)
	require.NoError(t, err)
	ctx := context.Background()
	key := model.PolicyBindingKey{IntegrationID: "verified-producer", ContextID: "context-1"}
	policy := storedContextPolicy()
	ref := model.PolicyRevision{ID: policy.ID, Revision: policy.Revision}
	version := int64(1)
	require.NoError(t, inRealTx(t, db, func(tx *sql.Tx) error {
		if err := repo.PublishWithTx(ctx, tx, policy, "operator", testutil.FixedTime()); err != nil {
			return err
		}
		return repo.BindWithTx(ctx, tx, key, ref, nil, "operator", testutil.FixedTime())
	}))
	loaded, err := repo.GetActive(ctx, key)
	require.NoError(t, err)
	require.Equal(t, policy.ID, loaded.ID)
	require.EqualValues(t, 1, loaded.Revision)
	require.Equal(t, model.DecisionDeny, loaded.DefaultDecision)
	require.Len(t, loaded.Rules, 2)
	require.Equal(t, policy.Rules[1], loaded.Rules[0])
	for _, other := range []model.PolicyBindingKey{{IntegrationID: key.IntegrationID, ContextID: "unknown"}, {IntegrationID: "other-producer", ContextID: key.ContextID}} {
		_, err := repo.GetActive(ctx, other)
		require.ErrorIs(t, err, constant.ErrContextPolicyUnavailable)
	}
	next := policy
	next.Revision = 2
	next.DefaultDecision = model.DecisionAllow
	require.NoError(t, inRealTx(t, db, func(tx *sql.Tx) error {
		if err := repo.PublishWithTx(ctx, tx, next, "operator", testutil.FixedTime()); err != nil {
			return err
		}
		return repo.BindWithTx(ctx, tx, key, model.PolicyRevision{ID: next.ID, Revision: 2}, &version, "operator", testutil.FixedTime())
	}))
	current, err := repo.GetActive(ctx, key)
	require.NoError(t, err)
	require.EqualValues(t, 2, current.Revision)
	require.EqualValues(t, 2, current.BindingVersion)
	require.NoError(t, inRealTx(t, db, func(tx *sql.Tx) error {
		return repo.BindWithTx(ctx, tx, key, ref, &current.BindingVersion, "operator", testutil.FixedTime())
	}))
	require.ErrorIs(t, inRealTx(t, db, func(tx *sql.Tx) error {
		return repo.BindWithTx(ctx, tx, key, ref, &version, "operator", testutil.FixedTime())
	}), constant.ErrContextPolicyConflict, "reject stale writers even after rebinding to the original policy")
	require.EqualValues(t, 1, loaded.Revision, "a previously selected revision stays frozen")
	require.ErrorIs(t, inRealTx(t, db, func(tx *sql.Tx) error {
		return repo.BindWithTx(ctx, tx, key, ref, nil, "operator", testutil.FixedTime())
	}), constant.ErrContextPolicyConflict)
	for _, statement := range []string{
		"UPDATE evaluation_policy_revisions SET default_decision = 'ALLOW'",
		"DELETE FROM evaluation_policy_revisions",
		"UPDATE evaluation_rule_revisions SET expression = 'false'",
		"DELETE FROM evaluation_policy_rules",
	} {
		_, err := db.ExecContext(ctx, statement)
		require.Error(t, err, "published revisions are immutable")
	}
	small, err := NewContextPolicyRepository(&testutil.IntegrationDBAdapter{DB: db}, 1)
	require.NoError(t, err)
	_, err = small.GetActive(ctx, key)
	require.Error(t, err, "never truncate a persisted policy")
	otherDB := contextPolicyDatabase(t, "b")
	otherRepo, err := NewContextPolicyRepository(&testutil.IntegrationDBAdapter{DB: otherDB}, 10)
	require.NoError(t, err)
	_, err = otherRepo.GetActive(ctx, key)
	require.ErrorIs(t, err, constant.ErrContextPolicyUnavailable)
}

func TestIntegrationContextPolicyRollbackAndConcurrentBinding(t *testing.T) {
	db := contextPolicyDatabase(t, "a")
	repo, err := NewContextPolicyRepository(&testutil.IntegrationDBAdapter{DB: db}, 10)
	require.NoError(t, err)
	ctx := context.Background()
	policy := storedContextPolicy()
	key := model.PolicyBindingKey{IntegrationID: "producer", ContextID: "context"}
	ref := model.PolicyRevision{ID: policy.ID, Revision: 1}
	version := int64(1)
	require.NoError(t, inRealTx(t, db, func(tx *sql.Tx) error {
		if err := repo.PublishWithTx(ctx, tx, policy, "actor", testutil.FixedTime()); err != nil {
			return err
		}
		return repo.BindWithTx(ctx, tx, key, ref, nil, "actor", testutil.FixedTime())
	}))
	for _, revision := range []int64{2, 3} {
		policy.Revision = revision
		require.NoError(t, inRealTx(t, db, func(tx *sql.Tx) error { return repo.PublishWithTx(ctx, tx, policy, "actor", testutil.FixedTime()) }))
	}
	start := make(chan struct{})
	results := make(chan error, 2)
	var workers sync.WaitGroup
	for _, revision := range []int64{2, 3} {
		workers.Go(func() {
			<-start
			tx, err := db.BeginTx(ctx, nil)
			if err != nil {
				results <- err
				return
			}
			defer tx.Rollback() //nolint:errcheck // Commit/operation error is reported below.
			err = repo.BindWithTx(ctx, tx, key, model.PolicyRevision{ID: policy.ID, Revision: revision}, &version, "actor", testutil.FixedTime())
			if err == nil {
				err = tx.Commit()
			}
			results <- err
		})
	}
	close(start)
	workers.Wait()
	close(results)
	winners := 0
	for err := range results {
		if err == nil {
			winners++
		} else {
			require.ErrorIs(t, err, constant.ErrContextPolicyConflict)
		}
	}
	require.Equal(t, 1, winners)
	policy.Revision = 4
	policy.Rules[0].Expression = "changed without changing rule revision"
	err = inRealTx(t, db, func(tx *sql.Tx) error { return repo.PublishWithTx(ctx, tx, policy, "actor", testutil.FixedTime()) })
	require.ErrorIs(t, err, constant.ErrContextPolicyConflict)
	var count int
	require.NoError(t, db.QueryRowContext(ctx, "SELECT count(*) FROM evaluation_policy_revisions WHERE policy_revision = 4").Scan(&count))
	require.Zero(t, count, "conflicting publication rolls back completely")
}

func TestIntegrationContextPolicyMigrationPreservesUsage(t *testing.T) {
	db := testutil.SetupIntegrationDB(t)
	ctx := context.Background()
	limitID := createTestLimitNamed(t, db, 60201, "policy-migration")
	t.Cleanup(func() { cleanupTestLimit(t, db, limitID) })
	reservation, err := model.NewReservation(limitID, testutil.MustDeterministicUUID(60202), "acct:policy-migration", "2026-09",
		decimal.RequireFromString("0.00000001"), testutil.FixedTime().Add(time.Hour), testutil.FixedTime())
	require.NoError(t, err)
	repo := newReservationRepoIntegration(db)
	require.NoError(t, inRealTx(t, db, func(tx *sql.Tx) error { return repo.ReserveWithTx(ctx, tx, reservation, decimal.NewFromInt(100)) }))
	dir := t.TempDir()
	require.NoError(t, migrations.WriteTo(dir))
	for _, direction := range []string{"down", "up"} {
		body, err := os.ReadFile(filepath.Join(dir, "000025_context_policies."+direction+".sql"))
		require.NoError(t, err)
		_, err = db.ExecContext(ctx, string(body))
		require.NoError(t, err)
		current, reserved := readCounterDecimal(t, db, limitID, reservation.ScopeKey, reservation.PeriodKey)
		require.True(t, current.IsZero())
		require.True(t, reserved.Equal(reservation.Amount), "migration must preserve fractional reserved usage")
		require.Equal(t, string(model.StatusReserved), readReservationStatus(t, db, reservation.ID))
	}
}

func TestIntegrationContextPolicyCompletenessAndEmptyDefault(t *testing.T) {
	db := contextPolicyDatabase(t, "a")
	repo, err := NewContextPolicyRepository(&testutil.IntegrationDBAdapter{DB: db}, 10)
	require.NoError(t, err)
	ctx := context.Background()
	policy := storedContextPolicy()
	policy.Rules = nil
	key := model.PolicyBindingKey{IntegrationID: "producer", ContextID: "empty-policy"}
	require.NoError(t, inRealTx(t, db, func(tx *sql.Tx) error {
		if err := repo.PublishWithTx(ctx, tx, policy, "actor", testutil.FixedTime()); err != nil {
			return err
		}
		return repo.BindWithTx(ctx, tx, key, model.PolicyRevision{ID: policy.ID, Revision: policy.Revision}, nil, "actor", testutil.FixedTime())
	}))
	loaded, err := repo.GetActive(ctx, key)
	require.NoError(t, err)
	require.Empty(t, loaded.Rules)
	require.Equal(t, model.DecisionDeny, loaded.DefaultDecision)
	_, err = db.ExecContext(ctx, "INSERT INTO evaluation_policy_revisions (policy_id, policy_revision, default_decision, rule_count, published_by, published_at) VALUES ($1, 2, 'ALLOW', 1, 'actor', $2)", policy.ID, testutil.FixedTime())
	require.Error(t, err, "a snapshot missing its declared rules cannot commit")
	rule := storedContextPolicy().Rules[0]
	_, err = db.ExecContext(ctx, "INSERT INTO evaluation_rule_revisions VALUES ($1, $2, $3, $4)", rule.ID, rule.Revision, rule.Expression, rule.Action)
	require.NoError(t, err)
	_, err = db.ExecContext(ctx, "INSERT INTO evaluation_policy_rules VALUES ($1, $2, $3, $4)", policy.ID, policy.Revision, rule.ID, rule.Revision)
	require.Error(t, err, "rules cannot be appended to an already published snapshot")
}
