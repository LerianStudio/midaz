// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

//go:build integration

package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/components/tracer/internal/services/query"
	"github.com/LerianStudio/midaz/v4/components/tracer/internal/testutil"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/clock"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/model"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/tracercontract"
)

func contextLimitRepository(t *testing.T, maximum int) *ContextLimitRepository {
	t.Helper()
	repo, err := NewContextLimitRepository(ContextLimitRepositoryConfig{MaxAccounts: 10, MaxLimits: maximum, MaxScopes: 10, MaxScopeBytes: 4096, MaxTextBytes: 256})
	require.NoError(t, err)
	return repo
}

func contextLimitRow(t *testing.T, db *sql.DB, seed int64, account uuid.UUID) uuid.UUID {
	t.Helper()
	id := createTestLimitNamed(t, db, seed, account.String()+"-"+testutil.MustDeterministicUUID(seed).String())
	scopes, err := json.Marshal([]model.Scope{{AccountID: &account}})
	require.NoError(t, err)
	_, err = db.ExecContext(t.Context(), "UPDATE limits SET scopes=$2 WHERE id=$1", id, scopes)
	require.NoError(t, err)
	return id
}

func bindContextLimit(t *testing.T, db *sql.DB, repo *ContextLimitRepository, id uuid.UUID, asset tracercontract.AssetRef) {
	t.Helper()
	tx, err := db.BeginTx(t.Context(), nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = tx.Rollback() })
	require.NoError(t, repo.BindAssetWithTx(t.Context(), tx, id, asset))
	require.NoError(t, tx.Commit())
}

func contextLimitEligibilityReport(t *testing.T) string {
	t.Helper()

	data, err := os.ReadFile(filepath.Join("..", "..", "..", "..", "..", "scripts", "tracer", "context-limit-eligibility.sql"))
	require.NoError(t, err)

	return string(data)
}

func countContextLimitEligibilityFailures(t *testing.T, db *sql.DB) int {
	t.Helper()

	rows, err := db.QueryContext(t.Context(), contextLimitEligibilityReport(t))
	require.NoError(t, err)
	defer func() { require.NoError(t, rows.Close()) }()

	count := 0
	for rows.Next() {
		count++
	}
	require.NoError(t, rows.Err())

	return count
}

func TestIntegrationContextLimitEligibilityReport(t *testing.T) {
	db := completionDatabase(t)
	repo := contextLimitRepository(t, 10)
	account := testutil.MustDeterministicUUID(81901)
	eligible := contextLimitRow(t, db, 81902, account)
	bindContextLimit(t, db, repo, eligible, tracercontract.AssetRef{Namespace: "ledger", ID: "usd", Code: "USD"})
	require.Zero(t, countContextLimitEligibilityFailures(t, db))

	contextLimitRow(t, db, 81903, account)
	require.Equal(t, 1, countContextLimitEligibilityFailures(t, db))
}

func TestIntegrationContextLimitAssetPreservesHistory(t *testing.T) {
	db := completionDatabase(t)
	repo := contextLimitRepository(t, 10)
	account := testutil.MustDeterministicUUID(82001)
	limit := contextLimitRow(t, db, 82002, account)
	scope := "acct:" + account.String()
	reservation, transaction := testutil.MustDeterministicUUID(82003), testutil.MustDeterministicUUID(82004)
	_, err := db.ExecContext(t.Context(), `INSERT INTO usage_counters (limit_id,scope_key,period_key,current_usage,reserved_usage) VALUES ($1,$2,'2026-09-24',7.125,10.125)`, limit, scope)
	require.NoError(t, err)
	_, err = db.ExecContext(t.Context(), `INSERT INTO usage_reservations (id,limit_id,scope_key,period_key,amount,transaction_id,reservation_expires_at) VALUES ($1,$2,$3,'2026-09-24',10.125,$4,$5)`, reservation, limit, scope, transaction, testutil.FixedTime())
	require.NoError(t, err)
	// Empty association schema can roll back, preserving all financial rows.
	_, err = db.ExecContext(t.Context(), capacityMigration(t, "000032_limit_asset_references.down.sql"))
	require.NoError(t, err)
	_, err = db.ExecContext(t.Context(), capacityMigration(t, "000032_limit_asset_references.up.sql"))
	require.NoError(t, err)
	asset := tracercontract.AssetRef{Namespace: "ledger", ID: "opaque-asset", Code: "USD"}
	bindContextLimit(t, db, repo, limit, asset)
	current, held := readCounterDecimal(t, db, limit, scope, "2026-09-24")
	require.Equal(t, "7.125", current.String())
	require.Equal(t, "10.125", held.String())
	var amount, status string
	require.NoError(t, db.QueryRowContext(t.Context(), "SELECT amount::text,status FROM usage_reservations WHERE id=$1", reservation).Scan(&amount, &status))
	require.Equal(t, "10.125", amount)
	require.Equal(t, "RESERVED", status)
	for _, statement := range []string{
		"UPDATE limit_asset_references SET asset_id='other'", "DELETE FROM limit_asset_references", "TRUNCATE limit_asset_references",
		"UPDATE limits SET asset='EUR'", capacityMigration(t, "000032_limit_asset_references.down.sql"),
	} {
		_, err := db.ExecContext(t.Context(), statement)
		require.Error(t, err)
	}
	tx, err := db.BeginTx(t.Context(), nil)
	require.NoError(t, err)
	defer tx.Rollback()
	limits, err := repo.ListCandidatesWithTx(t.Context(), tx, "ledger", []uuid.UUID{account})
	require.NoError(t, err)
	require.Len(t, limits, 1)
	require.Equal(t, asset, limits[0].Asset)
	require.Equal(t, limit, limits[0].Definition.ID)
	blocked := false
	facts := tracercontract.Context{Accounts: []tracercontract.Account{{ID: account, Type: "checking", Status: "ACTIVE", Blocked: &blocked, Asset: asset}}, Entries: []tracercontract.Entry{{AccountID: account, Direction: tracercontract.Debit, Amount: "10.125", Asset: asset}}}
	resolver, err := query.NewContextReservationResolver(clock.NewFixedClock(testutil.FixedTime()), query.ContextReservationConfig{
		Facts: tracercontract.Limits{MaxAccounts: 10, MaxEntries: 20, MaxTextBytes: 256, MaxIntegerDigits: 30, MaxFractionDigits: 20}, MaxLimits: 10, MaxScopesPerLimit: 10, MaxReservations: 20,
	})
	require.NoError(t, err)
	plan, err := resolver.Execute(t.Context(), facts, "ledger", limits)
	require.NoError(t, err)
	require.Len(t, plan.Reservations, 1)
	require.Equal(t, limit, plan.Reservations[0].LimitID)
	require.Equal(t, scope, plan.Reservations[0].ScopeKey)
	require.Equal(t, "10.125", plan.Reservations[0].Amount.String())
}

func TestIntegrationContextLimitSelectionIsComplete(t *testing.T) {
	db := completionDatabase(t)
	repo := contextLimitRepository(t, 10)
	account, other := testutil.MustDeterministicUUID(82101), testutil.MustDeterministicUUID(82102)
	bound := contextLimitRow(t, db, 82103, account)
	missing := contextLimitRow(t, db, 82104, account)
	foreign := contextLimitRow(t, db, 82105, account)
	contextLimitRow(t, db, 82106, other)
	asset := tracercontract.AssetRef{Namespace: "ledger", ID: "usd-a", Code: "USD"}
	bindContextLimit(t, db, repo, bound, asset)
	asset.Namespace = "other"
	bindContextLimit(t, db, repo, foreign, asset)
	tx, err := db.BeginTx(t.Context(), nil)
	require.NoError(t, err)
	limits, err := repo.ListCandidatesWithTx(t.Context(), tx, "ledger", []uuid.UUID{account})
	require.NoError(t, err)
	require.Len(t, limits, 2)
	found := map[uuid.UUID]tracercontract.AssetRef{}
	for _, limit := range limits {
		found[limit.Definition.ID] = limit.Asset
	}
	require.Equal(t, "usd-a", found[bound].ID)
	require.Equal(t, tracercontract.AssetRef{}, found[missing])
	require.NoError(t, tx.Rollback())
	// No limit is dropped to satisfy a result cap.
	tx, err = db.BeginTx(t.Context(), nil)
	require.NoError(t, err)
	limits, err = contextLimitRepository(t, 1).ListCandidatesWithTx(t.Context(), tx, "ledger", []uuid.UUID{account})
	require.ErrorIs(t, err, constant.ErrContextLimitsUnavailable)
	require.Nil(t, limits)
	require.NoError(t, tx.Rollback())
	// Unknown ownership of a broad scope must reach validation, even if its code
	// differs from the incoming asset; code-only filtering would omit it.
	broad := createTestLimitNamed(t, db, 82107, "broad")
	_, err = db.ExecContext(t.Context(), "UPDATE limits SET asset='EUR',scopes='[{\"segmentId\":\"11111111-1111-1111-1111-111111111111\"}]' WHERE id=$1", broad)
	require.NoError(t, err)
	tx, err = db.BeginTx(t.Context(), nil)
	require.NoError(t, err)
	defer tx.Rollback()
	limits, err = repo.ListCandidatesWithTx(t.Context(), tx, "ledger", []uuid.UUID{account})
	require.NoError(t, err)
	require.Len(t, limits, 3)
}

func TestIntegrationContextLimitRejectsCorruptScopes(t *testing.T) {
	for _, raw := range []string{`{}`, `null`, `[{"accountId":"bad"}]`, `[{"accountId":"11111111-1111-1111-1111-111111111111","futureDimension":"hidden"}]`} {
		t.Run(raw, func(t *testing.T) {
			db := completionDatabase(t)
			repo := contextLimitRepository(t, 10)
			account := uuid.MustParse("11111111-1111-1111-1111-111111111111")
			limit := contextLimitRow(t, db, 82201, account)
			_, err := db.ExecContext(t.Context(), "UPDATE limits SET scopes=$2::jsonb WHERE id=$1", limit, raw)
			require.NoError(t, err)
			tx, err := db.BeginTx(t.Context(), nil)
			require.NoError(t, err)
			defer tx.Rollback()
			limits, err := repo.ListCandidatesWithTx(t.Context(), tx, "ledger", []uuid.UUID{account})
			require.ErrorIs(t, err, constant.ErrContextLimitsUnavailable)
			require.Nil(t, limits)
		})
	}
}

func TestIntegrationContextLimitTenantTransactions(t *testing.T) {
	a, b := completionDatabase(t, "a"), completionDatabase(t, "b")
	account := testutil.MustDeterministicUUID(82301)
	repo := contextLimitRepository(t, 10)
	for i, db := range []*sql.DB{a, b} {
		limit := contextLimitRow(t, db, 82302, account)
		asset := tracercontract.AssetRef{Namespace: "ledger", ID: []string{"asset-a", "asset-b"}[i], Code: "USD"}
		bindContextLimit(t, db, repo, limit, asset)
		tx, err := db.BeginTx(t.Context(), nil)
		require.NoError(t, err)
		limits, err := repo.ListCandidatesWithTx(t.Context(), tx, "ledger", []uuid.UUID{account})
		require.NoError(t, err)
		require.Len(t, limits, 1)
		require.Equal(t, asset, limits[0].Asset)
		require.NoError(t, tx.Rollback())
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	result, err := repo.ListCandidatesWithTx(ctx, nil, "ledger", []uuid.UUID{account})
	require.ErrorIs(t, err, context.Canceled)
	require.Nil(t, result)
}

func TestIntegrationContextLimitReadLocksSnapshot(t *testing.T) {
	db := completionDatabase(t)
	repo := contextLimitRepository(t, 10)
	account := testutil.MustDeterministicUUID(82401)
	limit := contextLimitRow(t, db, 82402, account)
	read, err := db.BeginTx(t.Context(), nil)
	require.NoError(t, err)
	defer read.Rollback()
	_, err = repo.ListCandidatesWithTx(t.Context(), read, "ledger", []uuid.UUID{account})
	require.NoError(t, err)
	// Rollback cannot race an in-flight admission snapshot or wait indefinitely.
	_, err = db.ExecContext(t.Context(), capacityMigration(t, "000032_limit_asset_references.down.sql"))
	var locked *pgconn.PgError
	require.ErrorAs(t, err, &locked)
	require.Equal(t, "55P03", locked.Code)
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	write, err := db.BeginTx(ctx, nil)
	require.NoError(t, err)
	defer write.Rollback()
	var pid int
	require.NoError(t, write.QueryRowContext(ctx, "SELECT pg_backend_pid()").Scan(&pid))
	done := make(chan error, 1)
	go func() {
		_, err := write.ExecContext(ctx, "UPDATE limits SET max_amount=17 WHERE id=$1", limit)
		done <- err
	}()
	require.Eventually(t, func() bool {
		var blocked bool
		err := db.QueryRowContext(ctx, "SELECT cardinality(pg_blocking_pids($1))>0", pid).Scan(&blocked)
		return err == nil && blocked
	}, 2*time.Second, 10*time.Millisecond)
	require.NoError(t, read.Commit())
	require.NoError(t, <-done)
	require.NoError(t, write.Commit())
}

func TestIntegrationContextLimitAssociationConflictsAndRollback(t *testing.T) {
	db := completionDatabase(t)
	repo := contextLimitRepository(t, 10)
	account := testutil.MustDeterministicUUID(82601)
	limit := contextLimitRow(t, db, 82602, account)
	asset := tracercontract.AssetRef{Namespace: "ledger", ID: "asset", Code: "USD"}
	tx, err := db.BeginTx(t.Context(), nil)
	require.NoError(t, err)
	require.NoError(t, repo.BindAssetWithTx(t.Context(), tx, limit, asset))
	require.NoError(t, tx.Rollback())
	var count int
	require.NoError(t, db.QueryRowContext(t.Context(), "SELECT count(*) FROM limit_asset_references").Scan(&count))
	require.Zero(t, count)
	for _, tc := range []struct {
		id   uuid.UUID
		code string
	}{{limit, "EUR"}, {testutil.MustDeterministicUUID(82603), "USD"}} {
		tx, err := db.BeginTx(t.Context(), nil)
		require.NoError(t, err)
		wrong := asset
		wrong.Code = tc.code
		require.ErrorIs(t, repo.BindAssetWithTx(t.Context(), tx, tc.id, wrong), constant.ErrContextLimitsUnavailable)
		require.NoError(t, tx.Rollback())
	}
	winner, err := db.BeginTx(t.Context(), nil)
	require.NoError(t, err)
	defer winner.Rollback()
	require.NoError(t, repo.BindAssetWithTx(t.Context(), winner, limit, asset))
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	loser, err := db.BeginTx(ctx, nil)
	require.NoError(t, err)
	defer loser.Rollback()
	var pid int
	require.NoError(t, loser.QueryRowContext(ctx, "SELECT pg_backend_pid()").Scan(&pid))
	done := make(chan error, 1)
	other := asset
	other.ID = "different"
	go func() { done <- repo.BindAssetWithTx(ctx, loser, limit, other) }()
	require.Eventually(t, func() bool {
		var blocked bool
		err := db.QueryRowContext(ctx, "SELECT cardinality(pg_blocking_pids($1))>0", pid).Scan(&blocked)
		return err == nil && blocked
	}, 2*time.Second, 10*time.Millisecond)
	require.NoError(t, winner.Commit())
	require.ErrorIs(t, <-done, constant.ErrLimitAssetReferenceConflict)
	require.NoError(t, loser.Rollback())
	require.NoError(t, db.QueryRowContext(t.Context(), "SELECT count(*) FROM limit_asset_references").Scan(&count))
	require.Equal(t, 1, count)
	tx, err = db.BeginTx(t.Context(), nil)
	require.NoError(t, err)
	defer tx.Rollback()
	require.ErrorIs(t, repo.BindAssetWithTx(t.Context(), tx, limit, asset), constant.ErrLimitAssetReferenceConflict)
}

func TestIntegrationContextLimitSnapshotBoundsAndAccountEncoding(t *testing.T) {
	db := completionDatabase(t)
	account := uuid.MustParse("aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee")
	limit := contextLimitRow(t, db, 82701, account)
	// PostgreSQL-valid UUID spellings must not silently bypass candidate lookup.
	_, err := db.ExecContext(t.Context(), `UPDATE limits SET scopes='[{"accountId":"AAAAAAAA-BBBB-CCCC-DDDD-EEEEEEEEEEEE"}]' WHERE id=$1`, limit)
	require.NoError(t, err)
	repo := contextLimitRepository(t, 10)
	tx, err := db.BeginTx(t.Context(), nil)
	require.NoError(t, err)
	limits, err := repo.ListCandidatesWithTx(t.Context(), tx, "ledger", []uuid.UUID{account})
	require.NoError(t, err)
	require.Len(t, limits, 1)
	require.NoError(t, tx.Rollback())
	for _, tc := range []struct {
		name   string
		config ContextLimitRepositoryConfig
	}{
		{"scope bytes", ContextLimitRepositoryConfig{MaxAccounts: 10, MaxLimits: 10, MaxScopes: 10, MaxScopeBytes: 10, MaxTextBytes: 256}},
		{"scope count", ContextLimitRepositoryConfig{MaxAccounts: 10, MaxLimits: 10, MaxScopes: 1, MaxScopeBytes: 4096, MaxTextBytes: 256}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if tc.name == "scope count" {
				_, err := db.ExecContext(t.Context(), "UPDATE limits SET scopes=scopes || scopes WHERE id=$1", limit)
				require.NoError(t, err)
			}
			bounded, err := NewContextLimitRepository(tc.config)
			require.NoError(t, err)
			tx, err := db.BeginTx(t.Context(), nil)
			require.NoError(t, err)
			defer tx.Rollback()
			limits, err := bounded.ListCandidatesWithTx(t.Context(), tx, "ledger", []uuid.UUID{account})
			require.ErrorIs(t, err, constant.ErrContextLimitsUnavailable)
			require.Nil(t, limits)
		})
	}
}

func TestIntegrationContextLimitStoredReferenceBounds(t *testing.T) {
	db := completionDatabase(t)
	account := testutil.MustDeterministicUUID(82801)
	limit := contextLimitRow(t, db, 82802, account)
	repo := contextLimitRepository(t, 10)
	asset := tracercontract.AssetRef{Namespace: "ledger", ID: "opaque-asset-longer-than-ten", Code: "USD"}
	bindContextLimit(t, db, repo, limit, asset)
	bounded, err := NewContextLimitRepository(ContextLimitRepositoryConfig{MaxAccounts: 10, MaxLimits: 10, MaxScopes: 10, MaxScopeBytes: 4096, MaxTextBytes: 10})
	require.NoError(t, err)
	tx, err := db.BeginTx(t.Context(), nil)
	require.NoError(t, err)
	defer tx.Rollback()
	limits, err := bounded.ListCandidatesWithTx(t.Context(), tx, "ledger", []uuid.UUID{account})
	require.ErrorIs(t, err, constant.ErrContextLimitsUnavailable)
	require.Nil(t, limits)
}
