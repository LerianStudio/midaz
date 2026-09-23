// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

//go:build integration

package postgres

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/bxcodec/dbresolver/v2"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/components/tracer/internal/adapters/cel"
	pgdb "github.com/LerianStudio/midaz/v4/components/tracer/internal/adapters/postgres/db"
	"github.com/LerianStudio/midaz/v4/components/tracer/internal/services/command"
	"github.com/LerianStudio/midaz/v4/components/tracer/internal/services/query"
	"github.com/LerianStudio/midaz/v4/components/tracer/internal/testutil"
	"github.com/LerianStudio/midaz/v4/components/tracer/migrations"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/clock"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/contextutil"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/model"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/tracercontract"
)

func TestIntegrationPublishContextPolicyAuditedTransaction(t *testing.T) {
	db := testutil.SetupIntegrationDB(t)
	conn := &testutil.IntegrationDBAdapter{DB: db}
	repo, err := NewContextPolicyRepository(conn, 10)
	require.NoError(t, err)
	audit := NewAuditEventRepositoryWithConnection(conn)
	beginner := pgdb.NewTxBeginnerAdapter(dbresolver.New(dbresolver.WithPrimaryDBs(db)))
	engine, err := cel.NewContextAdapter(cel.ContextAdapterConfig{
		Limits:    tracercontract.Limits{MaxAccounts: 10, MaxEntries: 20, MaxTextBytes: 256, MaxIntegerDigits: 128, MaxFractionDigits: 128},
		CostLimit: 100000, MaxExpressionBytes: 5000,
	})
	require.NoError(t, err)
	compiler, err := query.NewContextPolicyEvaluator(engine, query.ContextPolicyConfig{MaxRules: 10, TotalCost: 100000})
	require.NoError(t, err)
	publisher, err := command.NewPublishContextPolicyCommand(repo, audit, beginner, compiler, clock.NewFixedClock(testutil.FixedTime()))
	require.NoError(t, err)
	ctx := contextutil.WithPrincipal(context.Background(), contextutil.Principal{ID: "policy-operator", Type: "user"})
	policy := storedContextPolicy()
	policy.ID = testutil.MustDeterministicUUID(63001)

	// Strict mode must fail before reaching the default database without a tenant.
	beginner.SetMultiTenantEnabled(true)
	require.ErrorIs(t, publisher.Execute(ctx, policy), pgdb.ErrNoTenantInContext)
	beginner.SetMultiTenantEnabled(false)
	require.NoError(t, publisher.Execute(ctx, policy))
	require.ErrorIs(t, publisher.Execute(ctx, policy), constant.ErrContextPolicyConflict)

	var eventID uuid.UUID
	var count int
	require.NoError(t, db.QueryRowContext(ctx, "SELECT count(*) FROM audit_events WHERE resource_id = $1 AND event_type = 'POLICY_PUBLISHED'", policy.ID.String()).Scan(&count))
	require.Equal(t, 1, count, "a duplicate publication does not duplicate its audit record")
	require.NoError(t, db.QueryRowContext(ctx, "SELECT event_id FROM audit_events WHERE resource_id = $1 AND event_type = 'POLICY_PUBLISHED'", policy.ID.String()).Scan(&eventID))
	event, err := audit.GetByID(ctx, eventID)
	require.NoError(t, err)
	require.Equal(t, "policy-operator", event.Actor.ID)
	require.Equal(t, model.ActorTypeUser, event.Actor.ActorType)
	require.Equal(t, testutil.FixedTime(), event.CreatedAt.UTC())
	encoded, err := json.Marshal(event.Context["policy"])
	require.NoError(t, err)
	var snapshot model.ContextPolicy
	require.NoError(t, json.Unmarshal(encoded, &snapshot))
	require.Equal(t, policy, snapshot)
	verified, err := audit.VerifyHashChain(ctx, eventID)
	require.NoError(t, err)
	require.True(t, verified.IsValid)
	require.NoError(t, db.QueryRowContext(ctx, "SELECT count(*) FROM evaluation_policy_bindings WHERE policy_id = $1", policy.ID).Scan(&count))
	require.Zero(t, count, "publication alone never activates a policy")

	// A real database trigger failure must roll back the revision AND new rules.
	_, err = db.ExecContext(ctx, `CREATE FUNCTION reject_policy_audit_test() RETURNS trigger LANGUAGE plpgsql AS $$
 BEGIN
  IF NEW.event_type = 'POLICY_PUBLISHED' THEN RAISE EXCEPTION 'injected policy audit failure'; END IF;
  RETURN NEW;
 END $$;
 CREATE TRIGGER reject_policy_audit_test BEFORE INSERT ON audit_events FOR EACH ROW EXECUTE FUNCTION reject_policy_audit_test()`)
	require.NoError(t, err)
	t.Cleanup(func() {
		_, err := db.ExecContext(context.Background(), "DROP TRIGGER reject_policy_audit_test ON audit_events; DROP FUNCTION reject_policy_audit_test()")
		require.NoError(t, err)
	})
	failed := storedContextPolicy()
	failed.ID = testutil.MustDeterministicUUID(63002)
	failed.Rules[0].ID = testutil.MustDeterministicUUID(63003)
	failed.Rules[1].ID = testutil.MustDeterministicUUID(63004)
	require.ErrorContains(t, publisher.Execute(ctx, failed), "audit policy publication")
	require.NoError(t, db.QueryRowContext(ctx, "SELECT count(*) FROM evaluation_policy_revisions WHERE policy_id = $1", failed.ID).Scan(&count))
	require.Zero(t, count)
	require.NoError(t, db.QueryRowContext(ctx, "SELECT count(*) FROM evaluation_rule_revisions WHERE rule_id IN ($1, $2)", failed.Rules[0].ID, failed.Rules[1].ID).Scan(&count))
	require.Zero(t, count)
	require.NoError(t, db.QueryRowContext(ctx, "SELECT count(*) FROM audit_events WHERE resource_id = $1", failed.ID.String()).Scan(&count))
	require.Zero(t, count)

	// Rolling back the enum migration retains every immutable event and its hash.
	dir := t.TempDir()
	require.NoError(t, migrations.WriteTo(dir))
	down, err := os.ReadFile(filepath.Join(dir, "000026_policy_audit_enums.down.sql"))
	require.NoError(t, err)
	_, err = db.ExecContext(ctx, string(down))
	require.NoError(t, err)
	verified, err = audit.VerifyHashChain(ctx, eventID)
	require.NoError(t, err)
	require.True(t, verified.IsValid)
}
