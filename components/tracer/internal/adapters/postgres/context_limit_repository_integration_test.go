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
	repo, err := NewContextLimitRepository(ContextLimitRepositoryConfig{MaxAccounts: 10, MaxLimits: maximum, MaxScopes: 10, MaxScopeBytes: 4096})
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

func setContextLimitAsset(t *testing.T, db *sql.DB, id uuid.UUID, asset string) {
	t.Helper()
	_, err := db.ExecContext(t.Context(), "UPDATE limits SET asset=$2 WHERE id=$1", id, asset)
	require.NoError(t, err)
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
	account := testutil.MustDeterministicUUID(81901)
	contextLimitRow(t, db, 81902, account)
	other := contextLimitRow(t, db, 81903, account)
	setContextLimitAsset(t, db, other, "LERIANPOINTS")
	require.Zero(t, countContextLimitEligibilityFailures(t, db))

	broad := contextLimitRow(t, db, 81904, account)
	_, err := db.ExecContext(t.Context(), "UPDATE limits SET scopes='[]' WHERE id=$1", broad)
	require.NoError(t, err)
	require.Equal(t, 1, countContextLimitEligibilityFailures(t, db))
}

func TestIntegrationContextLimitCandidateCarriesStoredCode(t *testing.T) {
	db := completionDatabase(t)
	repo := contextLimitRepository(t, 10)
	account := testutil.MustDeterministicUUID(82001)
	limit := contextLimitRow(t, db, 82002, account)
	scope := "acct:" + account.String()
	_, err := db.ExecContext(t.Context(), `INSERT INTO usage_counters (limit_id,scope_key,period_key,current_usage,reserved_usage) VALUES ($1,$2,'2026-09-24',7.125,10.125)`, limit, scope)
	require.NoError(t, err)
	tx, err := db.BeginTx(t.Context(), nil)
	require.NoError(t, err)
	defer tx.Rollback()
	limits, err := repo.ListCandidatesWithTx(t.Context(), tx, []string{"USD"}, []uuid.UUID{account})
	require.NoError(t, err)
	require.Len(t, limits, 1)
	require.Equal(t, "USD", limits[0].Definition.Asset)
	require.Equal(t, limit, limits[0].Definition.ID)
	current, held := readCounterDecimal(t, db, limit, scope, "2026-09-24")
	require.Equal(t, "7.125", current.String())
	require.Equal(t, "10.125", held.String())
	blocked := false
	facts := tracercontract.Context{Accounts: []tracercontract.Account{{ID: account, Type: "checking", Status: "ACTIVE", Blocked: &blocked, Asset: "USD"}}, Entries: []tracercontract.Entry{{AccountID: account, Direction: tracercontract.Debit, Amount: "10.125", Asset: "USD"}}}
	resolver, err := query.NewContextReservationResolver(clock.NewFixedClock(testutil.FixedTime()), query.ContextReservationConfig{
		Facts: tracercontract.Limits{MaxAccounts: 10, MaxEntries: 20, MaxTextBytes: 256, MaxIntegerDigits: 30, MaxFractionDigits: 20}, MaxLimits: 10, MaxScopesPerLimit: 10, MaxReservations: 20,
	})
	require.NoError(t, err)
	plan, err := resolver.Execute(t.Context(), facts, limits)
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
	first := contextLimitRow(t, db, 82103, account)
	second := contextLimitRow(t, db, 82104, account)
	otherCode := contextLimitRow(t, db, 82105, account)
	setContextLimitAsset(t, db, otherCode, "EUR")
	contextLimitRow(t, db, 82106, other)
	tx, err := db.BeginTx(t.Context(), nil)
	require.NoError(t, err)
	limits, err := repo.ListCandidatesWithTx(t.Context(), tx, []string{"USD"}, []uuid.UUID{account})
	require.NoError(t, err)
	found := map[uuid.UUID]string{}
	for _, limit := range limits {
		found[limit.Definition.ID] = limit.Definition.Asset
	}
	require.Equal(t, map[uuid.UUID]string{first: "USD", second: "USD"}, found)
	require.NoError(t, tx.Rollback())
	// Each requested code selects its own limits; codes are never equated.
	tx, err = db.BeginTx(t.Context(), nil)
	require.NoError(t, err)
	limits, err = repo.ListCandidatesWithTx(t.Context(), tx, []string{"EUR"}, []uuid.UUID{account})
	require.NoError(t, err)
	require.Len(t, limits, 1)
	require.Equal(t, otherCode, limits[0].Definition.ID)
	require.Equal(t, "EUR", limits[0].Definition.Asset)
	limits, err = repo.ListCandidatesWithTx(t.Context(), tx, []string{"XBT"}, []uuid.UUID{account})
	require.NoError(t, err)
	require.Empty(t, limits)
	require.NoError(t, tx.Rollback())
	// No limit is dropped to satisfy a result cap.
	tx, err = db.BeginTx(t.Context(), nil)
	require.NoError(t, err)
	limits, err = contextLimitRepository(t, 1).ListCandidatesWithTx(t.Context(), tx, []string{"USD"}, []uuid.UUID{account})
	require.ErrorIs(t, err, constant.ErrContextLimitsUnavailable)
	require.Nil(t, limits)
	require.NoError(t, tx.Rollback())
	// A broad scope of the requested code must reach validation; one of another
	// code does not apply.
	broad := createTestLimitNamed(t, db, 82107, "broad")
	_, err = db.ExecContext(t.Context(), "UPDATE limits SET scopes='[{\"segmentId\":\"11111111-1111-1111-1111-111111111111\"}]' WHERE id=$1", broad)
	require.NoError(t, err)
	foreignBroad := createTestLimitNamed(t, db, 82108, "foreign-broad")
	_, err = db.ExecContext(t.Context(), "UPDATE limits SET asset='EUR',scopes='[]' WHERE id=$1", foreignBroad)
	require.NoError(t, err)
	tx, err = db.BeginTx(t.Context(), nil)
	require.NoError(t, err)
	defer tx.Rollback()
	limits, err = repo.ListCandidatesWithTx(t.Context(), tx, []string{"USD"}, []uuid.UUID{account})
	require.NoError(t, err)
	require.Len(t, limits, 3)
	limits, err = repo.ListCandidatesWithTx(t.Context(), tx, []string{"EUR", "USD"}, []uuid.UUID{account, other})
	require.NoError(t, err)
	require.Len(t, limits, 6)
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
			limits, err := repo.ListCandidatesWithTx(t.Context(), tx, []string{"USD"}, []uuid.UUID{account})
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
		limit := contextLimitRow(t, db, 82302+int64(i), account)
		asset := []string{"USD", "LERIANPOINTS"}[i]
		setContextLimitAsset(t, db, limit, asset)
		tx, err := db.BeginTx(t.Context(), nil)
		require.NoError(t, err)
		limits, err := repo.ListCandidatesWithTx(t.Context(), tx, []string{"USD", "LERIANPOINTS"}, []uuid.UUID{account, testutil.MustDeterministicUUID(82309)})
		require.NoError(t, err)
		require.Len(t, limits, 1)
		require.Equal(t, limit, limits[0].Definition.ID)
		require.Equal(t, asset, limits[0].Definition.Asset)
		require.NoError(t, tx.Rollback())
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	result, err := repo.ListCandidatesWithTx(ctx, nil, []string{"USD"}, []uuid.UUID{account})
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
	_, err = repo.ListCandidatesWithTx(t.Context(), read, []string{"USD"}, []uuid.UUID{account})
	require.NoError(t, err)
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
	limits, err := repo.ListCandidatesWithTx(t.Context(), tx, []string{"USD"}, []uuid.UUID{account})
	require.NoError(t, err)
	require.Len(t, limits, 1)
	require.NoError(t, tx.Rollback())
	for _, tc := range []struct {
		name   string
		config ContextLimitRepositoryConfig
	}{
		{"scope bytes", ContextLimitRepositoryConfig{MaxAccounts: 10, MaxLimits: 10, MaxScopes: 10, MaxScopeBytes: 10}},
		{"scope count", ContextLimitRepositoryConfig{MaxAccounts: 10, MaxLimits: 10, MaxScopes: 1, MaxScopeBytes: 4096}},
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
			limits, err := bounded.ListCandidatesWithTx(t.Context(), tx, []string{"USD"}, []uuid.UUID{account})
			require.ErrorIs(t, err, constant.ErrContextLimitsUnavailable)
			require.Nil(t, limits)
		})
	}
}

func TestIntegrationContextLimitCodeBounds(t *testing.T) {
	db := completionDatabase(t)
	account := testutil.MustDeterministicUUID(82801)
	limit := contextLimitRow(t, db, 82802, account)
	long := "ABCDEFGHIJKLMNOPQRSTUVWXYZABCDEFGHIJKLMNOPQRSTUVWXYZABCDEFGHIJKLMNOPQRSTUVWXYZABCDEFGHIJKLMNOPQRSTUV"
	require.Len(t, long, 100)
	setContextLimitAsset(t, db, limit, long)
	repo := contextLimitRepository(t, 10)
	tx, err := db.BeginTx(t.Context(), nil)
	require.NoError(t, err)
	defer tx.Rollback()
	limits, err := repo.ListCandidatesWithTx(t.Context(), tx, []string{long}, []uuid.UUID{account})
	require.NoError(t, err)
	require.Len(t, limits, 1)
	require.Equal(t, long, limits[0].Definition.Asset)
	limits, err = repo.ListCandidatesWithTx(t.Context(), tx, []string{long + "A"}, []uuid.UUID{account})
	require.ErrorIs(t, err, constant.ErrInvalidRequestBody)
	require.Nil(t, limits)
}
