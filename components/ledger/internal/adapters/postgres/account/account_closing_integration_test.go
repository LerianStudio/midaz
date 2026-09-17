//go:build integration

// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package account

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	libCommons "github.com/LerianStudio/lib-commons/v7/commons"
	libPointers "github.com/LerianStudio/lib-commons/v7/commons/pointers"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	"github.com/LerianStudio/midaz/v4/pkg/net/http"
	pgtestutil "github.com/LerianStudio/midaz/v4/tests/utils/postgres"
)

// closedAtMigration is the migration that introduces account.closed_at. The
// tests below drive it directly so the "existing accounts stay open" claim is
// proven against the migration's own SQL rather than a hand-written ALTER.
const closedAtMigration = "000022_add_account_closed_at_column"

// applyOnboardingMigrationFile executes one onboarding migration file (up or
// down) against the given database.
func applyOnboardingMigrationFile(t *testing.T, db *sql.DB, name string) {
	t.Helper()

	path := filepath.Join(pgtestutil.FindMigrationsPath(t, "onboarding"), name)

	content, err := os.ReadFile(path)
	require.NoErrorf(t, err, "failed to read onboarding migration %s", name)

	_, err = db.Exec(string(content))
	require.NoErrorf(t, err, "failed to apply onboarding migration %s", name)
}

// insertAccountRow writes an account with the minimal column set, i.e. without
// naming closed_at. It stands for an account that predates the closing feature.
// The alias is passed in because uuid v7 ids minted in the same millisecond
// share a prefix, so a derived alias would collide.
func insertAccountRow(t *testing.T, db *sql.DB, orgID, ledgerID, accountID uuid.UUID, alias string, createdAt time.Time) {
	t.Helper()

	_, err := db.Exec(`
		INSERT INTO account (id, name, asset_code, organization_id, ledger_id, status, alias, type, blocked, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $10)
	`, accountID, "Closing Test Account", "USD", orgID, ledgerID, "ACTIVE", alias, "deposit", false, createdAt)
	require.NoError(t, err, "failed to insert test account")
}

// readClosedAt reads the raw closed_at column of one account.
func readClosedAt(t *testing.T, db *sql.DB, accountID uuid.UUID) sql.NullTime {
	t.Helper()

	var closedAt sql.NullTime

	err := db.QueryRow(`SELECT closed_at FROM account WHERE id = $1`, accountID).Scan(&closedAt)
	require.NoError(t, err, "failed to read closed_at")

	return closedAt
}

// TestIntegration_AccountClosingMigrationLeavesExistingAccountsOpen rewinds the
// account table to the shape it had before the closing migration, writes an
// account there, then re-applies the migration. The pre-existing row must come
// back with closed_at NULL: the column carries no default, so no historical
// account is retroactively reported as closed.
func TestIntegration_AccountClosingMigrationLeavesExistingAccountsOpen(t *testing.T) {
	container := pgtestutil.SetupMigratedContainer(t, "onboarding")

	applyOnboardingMigrationFile(t, container.DB, closedAtMigration+".down.sql")

	var columnCount int

	err := container.DB.QueryRow(`
		SELECT count(*) FROM information_schema.columns
		WHERE table_name = 'account' AND column_name = 'closed_at'
	`).Scan(&columnCount)
	require.NoError(t, err, "failed to inspect account columns")
	require.Zero(t, columnCount, "the down migration must remove closed_at")

	orgID := pgtestutil.CreateTestOrganization(t, container.DB)
	ledgerID := pgtestutil.CreateTestLedger(t, container.DB, orgID)

	legacyAccountID := uuid.Must(libCommons.GenerateUUIDv7())
	insertAccountRow(t, container.DB, orgID, ledgerID, legacyAccountID, "@closing-legacy", time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC))

	applyOnboardingMigrationFile(t, container.DB, closedAtMigration+".up.sql")

	var dataType, isNullable, columnDefault sql.NullString

	err = container.DB.QueryRow(`
		SELECT data_type, is_nullable, column_default FROM information_schema.columns
		WHERE table_name = 'account' AND column_name = 'closed_at'
	`).Scan(&dataType, &isNullable, &columnDefault)
	require.NoError(t, err, "the up migration must add closed_at")

	assert.Equal(t, "timestamp with time zone", dataType.String, "closed_at must be TIMESTAMPTZ")
	assert.Equal(t, "YES", isNullable.String, "closed_at must be nullable")
	assert.False(t, columnDefault.Valid, "closed_at must carry no default")

	assert.False(t, readClosedAt(t, container.DB, legacyAccountID).Valid, "an account that predates the migration must stay open")

	// A fresh account written after the migration is open as well.
	newAccountID := uuid.Must(libCommons.GenerateUUIDv7())
	insertAccountRow(t, container.DB, orgID, ledgerID, newAccountID, "@closing-fresh", time.Date(2026, 2, 3, 4, 5, 6, 0, time.UTC))

	assert.False(t, readClosedAt(t, container.DB, newAccountID).Valid, "a newly created account must be open")
}

// TestIntegration_AccountClosingMigrationIsIdempotent re-applies the up
// migration over an already-migrated schema and over a row that carries a
// closing instant. IF NOT EXISTS must make the re-run a no-op that preserves
// the recorded instant.
func TestIntegration_AccountClosingMigrationIsIdempotent(t *testing.T) {
	container := pgtestutil.SetupMigratedContainer(t, "onboarding")

	orgID := pgtestutil.CreateTestOrganization(t, container.DB)
	ledgerID := pgtestutil.CreateTestLedger(t, container.DB, orgID)

	accountID := uuid.Must(libCommons.GenerateUUIDv7())
	insertAccountRow(t, container.DB, orgID, ledgerID, accountID, "@closing-idempotent", time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC))

	closedAt := time.Date(2026, 3, 4, 5, 6, 7, 0, time.UTC)

	_, err := container.DB.Exec(`UPDATE account SET closed_at = $1 WHERE id = $2`, closedAt, accountID)
	require.NoError(t, err, "failed to record a closing instant")

	applyOnboardingMigrationFile(t, container.DB, closedAtMigration+".up.sql")

	got := readClosedAt(t, container.DB, accountID)
	require.True(t, got.Valid, "re-running the migration must preserve the closing instant")
	assert.True(t, closedAt.Equal(got.Time), "the closing instant must be unchanged")
}

// closingReadFixture builds an org, a ledger and two accounts — one open, one
// closed at a fixed instant — and returns the repository plus their ids. Every
// read assertion below shares it, so the same pair is observed through every
// query shape.
type closingReadFixture struct {
	repo       *AccountPostgreSQLRepository
	db         *sql.DB
	orgID      uuid.UUID
	ledgerID   uuid.UUID
	openID     uuid.UUID
	closedID   uuid.UUID
	openAlias  string
	closeAlias string
	closedAt   time.Time
}

func newClosingReadFixture(t *testing.T) closingReadFixture {
	t.Helper()

	container := pgtestutil.SetupMigratedContainer(t, "onboarding")
	repo := createRepository(t, container)

	orgID := pgtestutil.CreateTestOrganization(t, container.DB)
	ledgerID := pgtestutil.CreateTestLedger(t, container.DB, orgID)

	createdAt := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	closedAt := time.Date(2026, 3, 4, 5, 6, 7, 0, time.UTC)

	openID := uuid.Must(libCommons.GenerateUUIDv7())
	closedID := uuid.Must(libCommons.GenerateUUIDv7())

	openAlias, closeAlias := "@closing-open", "@closing-closed"

	insertAccountRow(t, container.DB, orgID, ledgerID, openID, openAlias, createdAt)
	insertAccountRow(t, container.DB, orgID, ledgerID, closedID, closeAlias, createdAt)

	_, err := container.DB.Exec(`UPDATE account SET closed_at = $1 WHERE id = $2`, closedAt, closedID)
	require.NoError(t, err, "failed to record a closing instant")

	return closingReadFixture{
		repo:       repo,
		db:         container.DB,
		orgID:      orgID,
		ledgerID:   ledgerID,
		openID:     openID,
		closedID:   closedID,
		openAlias:  openAlias,
		closeAlias: closeAlias,
		closedAt:   closedAt,
	}
}

// assertClosingPair checks that a read returned both accounts and that only the
// closed one carries an instant, and that the instant is the persisted one.
func (f closingReadFixture) assertClosingPair(t *testing.T, accounts []*mmodel.Account) {
	t.Helper()

	byID := make(map[string]*mmodel.Account, len(accounts))
	for _, acc := range accounts {
		byID[acc.ID] = acc
	}

	open, ok := byID[f.openID.String()]
	require.True(t, ok, "the open account must be returned")
	assert.Nil(t, open.ClosedAt, "an open account must report no closing instant")

	closed, ok := byID[f.closedID.String()]
	require.True(t, ok, "the closed account must be returned")
	require.NotNil(t, closed.ClosedAt, "a closed account must report its closing instant")
	assert.True(t, f.closedAt.Equal(*closed.ClosedAt), "the reported instant must be the persisted one")
}

// TestIntegration_AccountClosingReadsReportClosedAt walks every account read
// shape — by id, by alias, the ledger listing and the two batch lookups — under
// both holder policies. closedAt is version-independent, so the /v1 projection
// must report it exactly like the /v2 one while still withholding holderId.
func TestIntegration_AccountClosingReadsReportClosedAt(t *testing.T) {
	f := newClosingReadFixture(t)
	ctx := t.Context()

	policies := map[string]mmodel.HolderPolicy{
		"v1": mmodel.HolderOffV1,
		"v2": mmodel.HolderOnV2,
	}

	for name, policy := range policies {
		t.Run("find_by_id_"+name, func(t *testing.T) {
			open, err := f.repo.Find(ctx, f.orgID, f.ledgerID, nil, f.openID, policy)
			require.NoError(t, err)

			closed, err := f.repo.Find(ctx, f.orgID, f.ledgerID, nil, f.closedID, policy)
			require.NoError(t, err)

			f.assertClosingPair(t, []*mmodel.Account{open, closed})
		})

		t.Run("find_with_deleted_"+name, func(t *testing.T) {
			open, err := f.repo.FindWithDeleted(ctx, f.orgID, f.ledgerID, nil, f.openID, policy)
			require.NoError(t, err)

			closed, err := f.repo.FindWithDeleted(ctx, f.orgID, f.ledgerID, nil, f.closedID, policy)
			require.NoError(t, err)

			f.assertClosingPair(t, []*mmodel.Account{open, closed})
		})

		t.Run("find_alias_"+name, func(t *testing.T) {
			open, err := f.repo.FindAlias(ctx, f.orgID, f.ledgerID, nil, f.openAlias, policy)
			require.NoError(t, err)

			closed, err := f.repo.FindAlias(ctx, f.orgID, f.ledgerID, nil, f.closeAlias, policy)
			require.NoError(t, err)

			f.assertClosingPair(t, []*mmodel.Account{open, closed})
		})

		t.Run("find_all_"+name, func(t *testing.T) {
			accounts, err := f.repo.FindAll(ctx, f.orgID, f.ledgerID, nil, nil, listAllHeader(), policy)
			require.NoError(t, err)

			f.assertClosingPair(t, accounts)
		})

		t.Run("list_by_ids_"+name, func(t *testing.T) {
			accounts, err := f.repo.ListByIDs(ctx, f.orgID, f.ledgerID, nil, nil, []uuid.UUID{f.openID, f.closedID}, policy)
			require.NoError(t, err)

			f.assertClosingPair(t, accounts)
		})
	}

	t.Run("list_accounts_by_ids", func(t *testing.T) {
		accounts, err := f.repo.ListAccountsByIDs(ctx, f.orgID, f.ledgerID, []uuid.UUID{f.openID, f.closedID})
		require.NoError(t, err)

		f.assertClosingPair(t, accounts)
	})

	t.Run("list_accounts_by_alias", func(t *testing.T) {
		accounts, err := f.repo.ListAccountsByAlias(ctx, f.orgID, f.ledgerID, []string{f.openAlias, f.closeAlias})
		require.NoError(t, err)

		f.assertClosingPair(t, accounts)
	})
}

// listAllHeader is the query header a full ledger listing uses: one page wide
// enough for the fixture, ordered deterministically.
func listAllHeader() http.QueryHeader {
	return http.QueryHeader{Limit: 100, Page: 1, SortOrder: "desc"}
}

// TestIntegration_AccountClosingUpdatePreservesClosedAt runs an allowed PATCH
// against a closed account and reads it back. The update must follow its normal
// contract while leaving the closing instant untouched: closed_at is outside the
// generic SET list, so no PATCH — including one whose payload carries merge-patch
// nulls — can rewrite or clear it, and none reopens the account.
func TestIntegration_AccountClosingUpdatePreservesClosedAt(t *testing.T) {
	f := newClosingReadFixture(t)
	ctx := t.Context()

	updates := []struct {
		name  string
		input *mmodel.Account
	}{
		{
			name:  "rename",
			input: &mmodel.Account{Name: "Renamed Account"},
		},
		{
			name:  "unblock",
			input: &mmodel.Account{Blocked: libPointers.Bool(false)},
		},
		{
			name:  "clear a nullable field",
			input: &mmodel.Account{Name: "Renamed Again", NullFields: []string{"segmentId", "closedAt"}},
		},
		{
			name:  "carry the closing instant in the entity",
			input: &mmodel.Account{Name: "Renamed Once More", ClosedAt: &f.closedAt},
		},
	}

	for _, tc := range updates {
		t.Run(tc.name, func(t *testing.T) {
			_, err := f.repo.Update(ctx, f.orgID, f.ledgerID, nil, f.closedID, tc.input)
			require.NoError(t, err, "an allowed PATCH must follow its normal contract on a closed account")

			stored := readClosedAt(t, f.db, f.closedID)
			require.True(t, stored.Valid, "the PATCH must not reopen the account")
			assert.True(t, f.closedAt.Equal(stored.Time), "the PATCH must not move the closing instant")

			reread, err := f.repo.Find(ctx, f.orgID, f.ledgerID, nil, f.closedID, mmodel.HolderOnV2)
			require.NoError(t, err)
			require.NotNil(t, reread.ClosedAt)
			assert.True(t, f.closedAt.Equal(*reread.ClosedAt), "the re-read after PATCH must report the same instant")
		})
	}

	// An open account is unaffected in the other direction: a PATCH does not
	// close it.
	_, err := f.repo.Update(ctx, f.orgID, f.ledgerID, nil, f.openID, &mmodel.Account{Name: "Still Open", ClosedAt: &f.closedAt})
	require.NoError(t, err)
	assert.False(t, readClosedAt(t, f.db, f.openID).Valid, "a PATCH must never close an open account")
}

// TestIntegration_AccountClosingConditionalUpdateRecordsTheInstant closes an
// open account and asserts the instant comes from the database and lands on the
// row, without disturbing any other column.
func TestIntegration_AccountClosingConditionalUpdateRecordsTheInstant(t *testing.T) {
	f := newClosingReadFixture(t)
	ctx := t.Context()

	before, err := f.repo.Find(ctx, f.orgID, f.ledgerID, nil, f.openID, mmodel.HolderOnV2)
	require.NoError(t, err)

	closedAt, err := f.repo.CloseAccount(ctx, f.orgID, f.ledgerID, f.openID)
	require.NoError(t, err)
	require.False(t, closedAt.IsZero(), "the close must return the instant the database generated")

	stored := readClosedAt(t, f.db, f.openID)
	require.True(t, stored.Valid)
	assert.True(t, closedAt.Equal(stored.Time), "the returned instant must be the persisted one")

	after, err := f.repo.Find(ctx, f.orgID, f.ledgerID, nil, f.openID, mmodel.HolderOnV2)
	require.NoError(t, err)
	require.NotNil(t, after.ClosedAt)
	assert.True(t, closedAt.Equal(*after.ClosedAt), "the read-back must report the same instant")

	assert.Equal(t, before.Name, after.Name, "closing must not touch the registry fields")
	assert.Equal(t, before.Status, after.Status)
	assert.Equal(t, before.Blocked, after.Blocked)
	assert.Equal(t, before.UpdatedAt, after.UpdatedAt, "closing writes closed_at alone")
}

// TestIntegration_AccountClosingConditionalUpdateIsSingleShot repeats the close
// over a confirmed closing. The second attempt must not apply, and the original
// instant must survive it.
func TestIntegration_AccountClosingConditionalUpdateIsSingleShot(t *testing.T) {
	f := newClosingReadFixture(t)
	ctx := t.Context()

	first, err := f.repo.CloseAccount(ctx, f.orgID, f.ledgerID, f.openID)
	require.NoError(t, err)

	_, err = f.repo.CloseAccount(ctx, f.orgID, f.ledgerID, f.openID)
	require.ErrorIs(t, err, ErrAccountCloseNotApplied, "a repeat must not apply")

	stored := readClosedAt(t, f.db, f.openID)
	require.True(t, stored.Valid)
	assert.True(t, first.Equal(stored.Time), "the repeat must not overwrite the original instant")
}

// TestIntegration_AccountClosingConditionalUpdateResolvesADispute runs several
// closings of the same account at once. Exactly one applies; every loser reports
// the not-applied sentinel and none of them moves the instant.
func TestIntegration_AccountClosingConditionalUpdateResolvesADispute(t *testing.T) {
	f := newClosingReadFixture(t)
	ctx := t.Context()

	const contenders = 8

	start := make(chan struct{})
	results := make(chan struct {
		closedAt time.Time
		err      error
	}, contenders)

	var wg sync.WaitGroup

	for i := 0; i < contenders; i++ {
		wg.Add(1)

		go func() {
			defer wg.Done()

			<-start

			closedAt, err := f.repo.CloseAccount(ctx, f.orgID, f.ledgerID, f.openID)
			results <- struct {
				closedAt time.Time
				err      error
			}{closedAt: closedAt, err: err}
		}()
	}

	close(start)
	wg.Wait()
	close(results)

	var (
		winners int
		winner  time.Time
	)

	for res := range results {
		if res.err == nil {
			winners++
			winner = res.closedAt

			continue
		}

		require.ErrorIs(t, res.err, ErrAccountCloseNotApplied, "a loser must report the not-applied sentinel")
	}

	require.Equal(t, 1, winners, "exactly one closing may apply")

	stored := readClosedAt(t, f.db, f.openID)
	require.True(t, stored.Valid)
	assert.True(t, winner.Equal(stored.Time), "the persisted instant must be the winner's")
}

// TestIntegration_AccountClosingConditionalUpdateRefusesOutOfScope asserts the
// condition covers the whole scope and the row's lifecycle: a wrong
// organization, a wrong ledger, an unknown id, a soft-deleted account and an
// already-closed one all match nothing, and none of them is distinguishable from
// the others by the result alone — the caller has to read authoritatively.
func TestIntegration_AccountClosingConditionalUpdateRefusesOutOfScope(t *testing.T) {
	f := newClosingReadFixture(t)
	ctx := t.Context()

	otherOrgID := pgtestutil.CreateTestOrganization(t, f.db)
	otherLedgerID := pgtestutil.CreateTestLedger(t, f.db, otherOrgID)

	deletedID := uuid.Must(libCommons.GenerateUUIDv7())
	insertAccountRow(t, f.db, f.orgID, f.ledgerID, deletedID, "@closing-deleted", time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC))

	_, err := f.db.Exec(`UPDATE account SET deleted_at = now() WHERE id = $1`, deletedID)
	require.NoError(t, err)

	tests := []struct {
		name      string
		orgID     uuid.UUID
		ledgerID  uuid.UUID
		accountID uuid.UUID
	}{
		{name: "unknown account", orgID: f.orgID, ledgerID: f.ledgerID, accountID: uuid.Must(libCommons.GenerateUUIDv7())},
		{name: "another organization", orgID: otherOrgID, ledgerID: f.ledgerID, accountID: f.openID},
		{name: "another ledger", orgID: f.orgID, ledgerID: otherLedgerID, accountID: f.openID},
		{name: "soft-deleted account", orgID: f.orgID, ledgerID: f.ledgerID, accountID: deletedID},
		{name: "already-closed account", orgID: f.orgID, ledgerID: f.ledgerID, accountID: f.closedID},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			_, err := f.repo.CloseAccount(ctx, tc.orgID, tc.ledgerID, tc.accountID)
			require.ErrorIs(t, err, ErrAccountCloseNotApplied)
		})
	}

	assert.False(t, readClosedAt(t, f.db, f.openID).Valid, "a refused close must leave the account open")
	assert.False(t, readClosedAt(t, f.db, deletedID).Valid, "a soft-deleted account must not acquire an instant")

	closed := readClosedAt(t, f.db, f.closedID)
	require.True(t, closed.Valid)
	assert.True(t, f.closedAt.Equal(closed.Time), "an already-closed account keeps its original instant")
}

// TestIntegration_AccountClosingConditionalUpdateReportsTechnicalFailure drives
// the close with a cancelled context, so the statement fails for a reason that
// is not "matched no row". The distinction is the whole point of the sentinel:
// a caller that cannot tell an unknown outcome from a refused one would treat a
// lost write as proof the account is still open.
func TestIntegration_AccountClosingConditionalUpdateReportsTechnicalFailure(t *testing.T) {
	f := newClosingReadFixture(t)

	cancelled, cancel := context.WithCancel(t.Context())
	cancel()

	closedAt, err := f.repo.CloseAccount(cancelled, f.orgID, f.ledgerID, f.openID)

	require.Error(t, err, "a cancelled close must report a failure")
	require.NotErrorIs(t, err, ErrAccountCloseNotApplied,
		"a technical failure must never be reported as a refused close: the outcome is unknown, not decided")
	assert.ErrorIs(t, err, context.Canceled, "the technical failure must propagate its own cause")

	assert.True(t, closedAt.IsZero(),
		"a failed close returns the zero time, which is not an instant a caller may persist or publish")

	// Read back on a live context: the account the failed attempt targeted must
	// still be open, and the fixture's already-closed account untouched.
	assert.False(t, readClosedAt(t, f.db, f.openID).Valid, "a failed close must leave the account open")

	stored := readClosedAt(t, f.db, f.closedID)
	require.True(t, stored.Valid)
	assert.True(t, f.closedAt.Equal(stored.Time), "a failed close must not disturb another account's instant")

	// The account is still closable once the caller retries on a live context,
	// so the failure left nothing behind that blocks the operation.
	retried, err := f.repo.CloseAccount(t.Context(), f.orgID, f.ledgerID, f.openID)
	require.NoError(t, err, "a retry after a technical failure must be able to apply")
	assert.False(t, retried.IsZero())
}
