// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

//go:build integration

package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"sync"
	"testing"

	"github.com/bxcodec/dbresolver/v2"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/components/tracer/internal/adapters/cel"
	pgdb "github.com/LerianStudio/midaz/v4/components/tracer/internal/adapters/postgres/db"
	"github.com/LerianStudio/midaz/v4/components/tracer/internal/services/command"
	"github.com/LerianStudio/midaz/v4/components/tracer/internal/services/query"
	"github.com/LerianStudio/midaz/v4/components/tracer/internal/testutil"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/clock"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/contextutil"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/model"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/tracercontract"
)

func bindingCommand(t *testing.T, db *sql.DB) (*command.BindContextPolicyCommand, *ContextPolicyRepository, *AuditEventRepository) {
	t.Helper()
	conn := &testutil.IntegrationDBAdapter{DB: db}
	repo, err := NewContextPolicyRepository(conn, 10)
	require.NoError(t, err)
	audit := NewAuditEventRepositoryWithConnection(conn)
	engine, err := cel.NewContextAdapter(cel.ContextAdapterConfig{
		Limits:    tracercontract.Limits{MaxAccounts: 10, MaxEntries: 20, MaxTextBytes: 256, MaxIntegerDigits: 128, MaxFractionDigits: 128},
		CostLimit: 100000, MaxExpressionBytes: 5000,
	})
	require.NoError(t, err)
	compiler, err := query.NewContextPolicyEvaluator(engine, query.ContextPolicyConfig{MaxRules: 10, TotalCost: 100000})
	require.NoError(t, err)
	c, err := command.NewBindContextPolicyCommand(repo, audit, pgdb.NewTxBeginnerAdapter(dbresolver.New(dbresolver.WithPrimaryDBs(db))), compiler, clock.NewFixedClock(testutil.FixedTime()))
	require.NoError(t, err)
	return c, repo, audit
}

func TestIntegrationBindContextPolicyConcurrencyAndAudit(t *testing.T) {
	db := testutil.SetupIntegrationDB(t)
	c, repo, audit := bindingCommand(t, db)
	ctx := contextutil.WithPrincipal(context.Background(), contextutil.Principal{ID: "binding-operator", Type: "user"})
	policy := storedContextPolicy()
	policy.ID = testutil.MustDeterministicUUID(65001)
	key := model.PolicyBindingKey{IntegrationID: "verified-producer", ContextID: "binding-concurrency"}
	for revision := int64(1); revision <= 3; revision++ {
		policy.Revision = revision
		require.NoError(t, inRealTx(t, db, func(tx *sql.Tx) error { return repo.PublishWithTx(ctx, tx, policy, "operator", testutil.FixedTime()) }))
	}
	original := model.PolicyRevision{ID: policy.ID, Revision: 1}
	first, err := c.Execute(ctx, key, original, nil)
	require.NoError(t, err)
	require.EqualValues(t, 1, first.Version)

	// Both writers saw version 1. Exactly one may update and append its event.
	type outcome struct {
		state *model.PolicyBindingState
		err   error
	}
	results := make(chan outcome, 2)
	start := make(chan struct{})
	var wg sync.WaitGroup
	for revision := int64(2); revision <= 3; revision++ {
		wg.Add(1)
		go func(revision int64) {
			defer wg.Done()
			<-start
			result, err := c.Execute(ctx, key, model.PolicyRevision{ID: policy.ID, Revision: revision}, &first.Version)
			results <- outcome{result, err}
		}(revision)
	}
	close(start)
	wg.Wait()
	close(results)
	var winner *model.PolicyBindingState
	conflicts := 0
	for result := range results {
		if errors.Is(result.err, constant.ErrContextPolicyConflict) {
			conflicts++
			require.Nil(t, result.state)
		} else {
			require.NoError(t, result.err)
			winner = result.state
		}
	}
	require.Equal(t, 1, conflicts)
	require.NotNil(t, winner)
	require.EqualValues(t, 2, winner.Version)
	active, err := repo.GetActive(ctx, key)
	require.NoError(t, err)
	require.Equal(t, winner.Policy.Revision, active.Revision)

	// Rebinding to A cannot make an old version-1 write valid again.
	returned, err := c.Execute(ctx, key, original, &winner.Version)
	require.NoError(t, err)
	require.EqualValues(t, 3, returned.Version)
	stale, err := c.Execute(ctx, key, winner.Policy, &first.Version)
	require.ErrorIs(t, err, constant.ErrContextPolicyConflict)
	require.Nil(t, stale)
	rows, err := db.QueryContext(ctx, "SELECT event_id FROM audit_events WHERE resource_id = $1 AND event_type = 'POLICY_BOUND' ORDER BY id", policy.ID.String())
	require.NoError(t, err)
	var ids []uuid.UUID
	for rows.Next() {
		var id uuid.UUID
		require.NoError(t, rows.Scan(&id))
		ids = append(ids, id)
	}
	require.NoError(t, rows.Err())
	require.NoError(t, rows.Close())
	require.Len(t, ids, 3)
	for i, id := range ids {
		event, err := audit.GetByID(ctx, id)
		require.NoError(t, err)
		require.Equal(t, "binding-operator", event.Actor.ID)
		require.Equal(t, model.AuditActionActivate, event.Action)
		encoded, err := json.Marshal(event.Context)
		require.NoError(t, err)
		var snapshot struct {
			Binding model.PolicyBindingKey
			Before  *model.PolicyBindingState
			After   model.PolicyBindingState
		}
		require.NoError(t, json.Unmarshal(encoded, &snapshot))
		require.Equal(t, key, snapshot.Binding)
		require.EqualValues(t, i+1, snapshot.After.Version)
		if i == 0 {
			require.Nil(t, snapshot.Before)
		} else {
			require.NotNil(t, snapshot.Before)
			require.EqualValues(t, i, snapshot.Before.Version)
			if i == 1 {
				require.Equal(t, original, snapshot.Before.Policy)
			} else {
				require.Equal(t, winner.Policy, snapshot.Before.Policy)
			}
		}
		verification, err := audit.VerifyHashChain(ctx, id)
		require.NoError(t, err)
		require.True(t, verification.IsValid)
	}
}

func TestIntegrationBindContextPolicyAuditRollback(t *testing.T) {
	db := testutil.SetupIntegrationDB(t)
	c, repo, _ := bindingCommand(t, db)
	ctx := contextutil.WithPrincipal(context.Background(), contextutil.Principal{ID: "binding-operator", Type: "user"})
	policy := storedContextPolicy()
	policy.ID = testutil.MustDeterministicUUID(65002)
	for revision := int64(1); revision <= 2; revision++ {
		policy.Revision = revision
		require.NoError(t, inRealTx(t, db, func(tx *sql.Tx) error { return repo.PublishWithTx(ctx, tx, policy, "operator", testutil.FixedTime()) }))
	}
	key := model.PolicyBindingKey{IntegrationID: "verified-producer", ContextID: "binding-rollback"}
	first, err := c.Execute(ctx, key, model.PolicyRevision{ID: policy.ID, Revision: 1}, nil)
	require.NoError(t, err)
	_, err = db.ExecContext(ctx, `CREATE FUNCTION reject_binding_audit_test() RETURNS trigger LANGUAGE plpgsql AS $$
 BEGIN
  IF NEW.event_type = 'POLICY_BOUND' THEN RAISE EXCEPTION 'injected binding audit failure'; END IF;
  RETURN NEW;
 END $$;
 CREATE TRIGGER reject_binding_audit_test BEFORE INSERT ON audit_events FOR EACH ROW EXECUTE FUNCTION reject_binding_audit_test()`)
	require.NoError(t, err)
	t.Cleanup(func() {
		_, err := db.ExecContext(context.Background(), "DROP TRIGGER reject_binding_audit_test ON audit_events; DROP FUNCTION reject_binding_audit_test()")
		require.NoError(t, err)
	})
	failed, err := c.Execute(ctx, key, model.PolicyRevision{ID: policy.ID, Revision: 2}, &first.Version)
	require.ErrorContains(t, err, "audit policy binding")
	require.Nil(t, failed)
	unchanged, err := repo.GetActive(ctx, key)
	require.NoError(t, err)
	require.EqualValues(t, 1, unchanged.Revision)
	require.EqualValues(t, 1, unchanged.BindingVersion)
	key.ContextID = "binding-failed-create"
	failed, err = c.Execute(ctx, key, first.Policy, nil)
	require.ErrorContains(t, err, "audit policy binding")
	require.Nil(t, failed)
	_, err = repo.GetActive(ctx, key)
	require.ErrorIs(t, err, constant.ErrContextPolicyUnavailable)
	var count int
	require.NoError(t, db.QueryRowContext(ctx, "SELECT count(*) FROM audit_events WHERE resource_id = $1 AND event_type = 'POLICY_BOUND'", policy.ID.String()).Scan(&count))
	require.Equal(t, 1, count)
}
