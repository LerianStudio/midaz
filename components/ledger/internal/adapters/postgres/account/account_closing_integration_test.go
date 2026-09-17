//go:build integration

// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package account

import (
	"database/sql"
	"os"
	"path/filepath"
	"testing"
	"time"

	libCommons "github.com/LerianStudio/lib-commons/v7/commons"
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
