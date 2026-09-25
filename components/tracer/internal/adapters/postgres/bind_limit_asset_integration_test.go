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
	"testing"
	"time"

	tmcore "github.com/LerianStudio/lib-commons/v7/commons/tenant-manager/core"
	pgdb "github.com/LerianStudio/midaz/v4/components/tracer/internal/adapters/postgres/db"
	"github.com/LerianStudio/midaz/v4/components/tracer/internal/services/command"
	"github.com/LerianStudio/midaz/v4/components/tracer/internal/testutil"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/clock"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/contextutil"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/model"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/tracercontract"
	"github.com/bxcodec/dbresolver/v2"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func assetBindingCommand(t *testing.T, db *sql.DB, singleTenant bool) (*command.BindLimitAssetCommand, *AuditEventRepository) {
	t.Helper()
	audit := NewAuditEventRepositoryWithConnection(&testutil.IntegrationDBAdapter{DB: db})
	beginner := pgdb.NewTxBeginnerAdapter(dbresolver.New(dbresolver.WithPrimaryDBs(db)))
	beginner.SetMultiTenantEnabled(!singleTenant)
	c, err := command.NewBindLimitAssetCommand(contextLimitRepository(t, 10), audit, beginner, clock.NewFixedClock(testutil.FixedTime()), command.LimitAssetBindingConfig{SingleTenant: singleTenant, MaxScopes: 10, Facts: tracercontract.Limits{MaxAccounts: 10, MaxEntries: 20, MaxTextBytes: 256, MaxIntegerDigits: 30, MaxFractionDigits: 20}})
	require.NoError(t, err)
	return c, audit
}

func assetBindingContext(ctx context.Context) context.Context {
	ctx = contextutil.WithIntegrationIdentity(ctx, contextutil.IntegrationIdentity{ID: "producer", AssetNamespace: "ledger"})
	return contextutil.WithPrincipal(ctx, contextutil.Principal{ID: "migration-agent", Type: "system"})
}

func assetBindingFacts(account uuid.UUID) []tracercontract.AccountAsset {
	return []tracercontract.AccountAsset{{AccountID: account, Asset: tracercontract.AssetRef{Namespace: "ledger", ID: "official-asset", Code: "USD"}}}
}

func TestIntegrationBindLimitAssetAuditsAndPreservesCapacity(t *testing.T) {
	db := completionDatabase(t)
	c, audit := assetBindingCommand(t, db, true)
	account := testutil.MustDeterministicUUID(83101)
	limit := contextLimitRow(t, db, 83102, account)
	_, err := db.ExecContext(t.Context(), "UPDATE limits SET status='DRAFT' WHERE id=$1", limit)
	require.NoError(t, err)
	scope := "acct:" + account.String()
	_, err = db.ExecContext(t.Context(), `INSERT INTO usage_counters (limit_id,scope_key,period_key,current_usage,reserved_usage) VALUES ($1,$2,'2026-09-24',7.125,10.125)`, limit, scope)
	require.NoError(t, err)
	ctx := assetBindingContext(t.Context())
	facts := assetBindingFacts(account)
	ref, err := c.Execute(ctx, limit, facts)
	require.NoError(t, err)
	require.Equal(t, facts[0].Asset, *ref)
	require.ErrorIs(t, func() error { _, err := c.Execute(ctx, limit, facts); return err }(), constant.ErrLimitAssetReferenceConflict)
	ids := completionEvents(t, db, limit)
	require.Len(t, ids, 1)
	event, err := audit.GetByID(ctx, ids[0])
	require.NoError(t, err)
	require.Equal(t, model.AuditEventLimitUpdated, event.EventType)
	require.Equal(t, model.ResourceTypeLimit, event.ResourceType)
	require.Equal(t, "migration-agent", event.Actor.ID)
	require.Equal(t, model.ActorTypeSystem, event.Actor.ActorType)
	require.Equal(t, "producer", event.Context["integrationId"])
	require.Equal(t, "asset_reference_binding", event.Context["operation"])
	raw, err := json.Marshal(event.Context["accountAssets"])
	require.NoError(t, err)
	var recorded []tracercontract.AccountAsset
	require.NoError(t, json.Unmarshal(raw, &recorded))
	require.Equal(t, facts, recorded)
	verified, err := audit.VerifyHashChain(ctx, ids[0])
	require.NoError(t, err)
	require.True(t, verified.IsValid)
	current, held := readCounterDecimal(t, db, limit, scope, "2026-09-24")
	require.Equal(t, "7.125", current.String())
	require.Equal(t, "10.125", held.String())
	var status string
	require.NoError(t, db.QueryRowContext(ctx, "SELECT status FROM limits WHERE id=$1", limit).Scan(&status))
	require.Equal(t, "DRAFT", status)
}

func TestIntegrationBoundLimitRejectsUnattestedScopeChange(t *testing.T) {
	db := completionDatabase(t)
	binder, _ := assetBindingCommand(t, db, true)
	account := testutil.MustDeterministicUUID(83901)
	id := contextLimitRow(t, db, 83902, account)
	_, err := binder.Execute(assetBindingContext(t.Context()), id, assetBindingFacts(account))
	require.NoError(t, err)
	repo := NewLimitRepositoryWithConnection(&testutil.IntegrationDBAdapter{DB: db})
	limit, err := repo.GetByID(t.Context(), id)
	require.NoError(t, err)
	other := testutil.MustDeterministicUUID(83903)
	limit.Scopes = []model.Scope{{AccountID: &other}}
	require.ErrorIs(t, repo.UpdateWithTx(t.Context(), db, limit), constant.ErrLimitAssetReferenceConflict)
	stored, err := repo.GetByID(t.Context(), id)
	require.NoError(t, err)
	require.Equal(t, account, *stored.Scopes[0].AccountID)
	stored.Name = "same-attested-accounts"
	require.NoError(t, repo.UpdateWithTx(t.Context(), db, stored))
}

func TestIntegrationBindLimitAssetAuditFailureRollsBack(t *testing.T) {
	for _, suppress := range []bool{false, true} {
		t.Run(map[bool]string{false: "SQL failure", true: "suppressed row"}[suppress], func(t *testing.T) {
			db := completionDatabase(t)
			c, _ := assetBindingCommand(t, db, true)
			account := testutil.MustDeterministicUUID(83201)
			limit := contextLimitRow(t, db, 83202, account)
			body := "RAISE EXCEPTION 'injected failure';"
			if suppress {
				body = "RETURN NULL;"
			}
			_, err := db.ExecContext(t.Context(), "CREATE FUNCTION fail_asset_audit() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN "+body+" END $$; CREATE TRIGGER fail_asset_audit BEFORE INSERT ON audit_events FOR EACH ROW EXECUTE FUNCTION fail_asset_audit()")
			require.NoError(t, err)
			result, err := c.Execute(assetBindingContext(t.Context()), limit, assetBindingFacts(account))
			require.Error(t, err)
			require.Nil(t, result)
			var count int
			require.NoError(t, db.QueryRowContext(t.Context(), "SELECT count(*) FROM limit_asset_references").Scan(&count))
			require.Zero(t, count)
			require.Empty(t, completionEvents(t, db, limit))
			_, err = db.ExecContext(t.Context(), "DROP TRIGGER fail_asset_audit ON audit_events")
			require.NoError(t, err)
			result, err = c.Execute(assetBindingContext(t.Context()), limit, assetBindingFacts(account))
			require.NoError(t, err)
			require.NotNil(t, result)
			require.Len(t, completionEvents(t, db, limit), 1)
		})
	}
}

func TestIntegrationBindLimitAssetConcurrentCommands(t *testing.T) {
	db := completionDatabase(t)
	c, _ := assetBindingCommand(t, db, true)
	account := testutil.MustDeterministicUUID(83301)
	limit := contextLimitRow(t, db, 83302, account)
	ctx, cancel := context.WithTimeout(assetBindingContext(t.Context()), 5*time.Second)
	defer cancel()
	start := make(chan struct{})
	results := make(chan error, 2)
	for range 2 {
		go func() { <-start; _, err := c.Execute(ctx, limit, assetBindingFacts(account)); results <- err }()
	}
	close(start)
	successes, conflicts := 0, 0
	for range 2 {
		err := <-results
		if err == nil {
			successes++
		} else if errors.Is(err, constant.ErrLimitAssetReferenceConflict) {
			conflicts++
		} else {
			require.NoError(t, err)
		}
	}
	require.Equal(t, 1, successes)
	require.Equal(t, 1, conflicts)
	require.Len(t, completionEvents(t, db, limit), 1)
}

func TestIntegrationBindLimitAssetTenantPoolsAndScopeMismatch(t *testing.T) {
	a, b := completionDatabase(t, "a"), completionDatabase(t, "b")
	account := testutil.MustDeterministicUUID(83401)
	limit := contextLimitRow(t, a, 83402, account)
	contextLimitRow(t, b, 83402, account)
	c, _ := assetBindingCommand(t, a, false)
	ctx := assetBindingContext(t.Context())
	_, err := c.Execute(ctx, limit, assetBindingFacts(account))
	require.ErrorIs(t, err, constant.ErrReservationTenantRequired)
	ctx = tmcore.ContextWithTenantID(ctx, "tenant-b")
	_, err = c.Execute(ctx, limit, assetBindingFacts(account))
	require.ErrorIs(t, err, pgdb.ErrNoTenantInContext)
	ctx = tmcore.ContextWithPG(ctx, dbresolver.New(dbresolver.WithPrimaryDBs(b)))
	wrong := assetBindingFacts(testutil.MustDeterministicUUID(83403))
	_, err = c.Execute(ctx, limit, wrong)
	require.ErrorIs(t, err, constant.ErrContextLimitsUnavailable)
	require.Empty(t, completionEvents(t, b, limit))
	_, err = c.Execute(ctx, limit, assetBindingFacts(account))
	require.NoError(t, err)
	require.Empty(t, completionEvents(t, a, limit))
	require.Len(t, completionEvents(t, b, limit), 1)
	var count int
	require.NoError(t, a.QueryRowContext(t.Context(), "SELECT count(*) FROM limit_asset_references").Scan(&count))
	require.Zero(t, count)
}
