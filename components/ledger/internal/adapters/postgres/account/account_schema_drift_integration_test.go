//go:build integration

// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package account

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	libCommons "github.com/LerianStudio/lib-commons/v6/commons"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/pkg"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	"github.com/LerianStudio/midaz/v4/pkg/net/http"
	pgtestutil "github.com/LerianStudio/midaz/v4/tests/utils/postgres"
)

// driftToPreHolderSchema rewinds the account table to the shape it had before
// migrations 000017 and 000019, by applying those migrations' own down files.
// Reusing the repository's own SQL keeps the fixture honest: it cannot drift from
// what the up migrations add.
func driftToPreHolderSchema(t *testing.T, db *sql.DB) {
	t.Helper()

	dir := pgtestutil.FindMigrationsPath(t, "onboarding")

	// Reverse order, as golang-migrate would roll them back.
	for _, name := range []string{
		"000021_create_idx_account_org_holder.down.sql",
		"000019_add_holder_skip_audit_to_account.down.sql",
		"000018_create_idx_account_holder.down.sql",
		"000017_add_account_holder_id_column.down.sql",
	} {
		content, err := os.ReadFile(filepath.Join(dir, name))
		require.NoErrorf(t, err, "failed to read %s", name)

		// Index drops are CONCURRENTLY, which cannot run inside the implicit
		// transaction of a multi-statement Exec; the column drop removes them anyway.
		if strings.Contains(strings.ToUpper(string(content)), "CONCURRENTLY") {
			continue
		}

		_, err = db.Exec(string(content))
		require.NoErrorf(t, err, "failed to apply %s", name)
	}

	requireHolderColumnsAbsent(t, db)
}

func requireHolderColumnsAbsent(t *testing.T, db *sql.DB) {
	t.Helper()

	for _, column := range []string{"holder_id", "holder_check_skipped"} {
		var exists bool
		err := db.QueryRow(`
			SELECT EXISTS (
				SELECT 1 FROM information_schema.columns
				WHERE table_name = 'account' AND column_name = $1
			)`, column).Scan(&exists)
		require.NoError(t, err)
		require.Falsef(t, exists, "fixture is not drifted: account.%s still exists", column)
	}
}

// driftedLedger returns a repository and ledger scope on a database whose account
// table predates the holder columns.
func driftedLedger(t *testing.T) (*AccountPostgreSQLRepository, *sql.DB, uuid.UUID, uuid.UUID) {
	t.Helper()

	container := pgtestutil.SetupMigratedContainer(t, "onboarding")
	driftToPreHolderSchema(t, container.DB)

	repo := createRepository(t, container)
	orgID := pgtestutil.CreateTestOrganization(t, container.DB)
	ledgerID := pgtestutil.CreateTestLedger(t, container.DB, orgID)
	pgtestutil.CreateTestAsset(t, container.DB, orgID, ledgerID, "USD")

	return repo, container.DB, orgID, ledgerID
}

// driftTestTime is a fixed timestamp: these tests assert schema tolerance, so the
// create/update instants carry no meaning and must not vary between runs.
var driftTestTime = time.Date(2026, 8, 27, 12, 0, 0, 0, time.UTC)

func newDriftAccount(orgID, ledgerID uuid.UUID) *mmodel.Account {
	now := driftTestTime
	alias := fmt.Sprintf("@drift-%s", uuid.Must(libCommons.GenerateUUIDv7()).String()[:8])
	blocked := false

	return &mmodel.Account{
		ID:             uuid.Must(libCommons.GenerateUUIDv7()).String(),
		Name:           "Drift Account",
		AssetCode:      "USD",
		OrganizationID: orgID.String(),
		LedgerID:       ledgerID.String(),
		Type:           "deposit",
		Alias:          &alias,
		Status:         mmodel.Status{Code: "ACTIVE"},
		Blocked:        &blocked,
		CreatedAt:      now,
		UpdatedAt:      now,
	}
}

// The /v1 contract links no holder, so a create on it must not depend on columns
// the applied migrations have not created.
func TestIntegration_SchemaDrift_V1CreateSucceeds(t *testing.T) {
	repo, _, orgID, ledgerID := driftedLedger(t)

	created, err := repo.Create(t.Context(), newDriftAccount(orgID, ledgerID))

	require.NoError(t, err, "a /v1 create must not require migration 000017/000019")
	require.NotNil(t, created)
	assert.Nil(t, created.HolderID)
	assert.False(t, created.HolderCheckSkipped)
}

func TestIntegration_SchemaDrift_V1ReadsSucceed(t *testing.T) {
	repo, _, orgID, ledgerID := driftedLedger(t)
	ctx := t.Context()

	acc := newDriftAccount(orgID, ledgerID)
	_, err := repo.Create(ctx, acc)
	require.NoError(t, err)

	accountID := uuid.MustParse(acc.ID)

	found, err := repo.Find(ctx, orgID, ledgerID, nil, accountID, mmodel.HolderOffV1)
	require.NoError(t, err, "a /v1 get must not require the holder columns")
	assert.Nil(t, found.HolderID)
	assert.False(t, found.HolderCheckSkipped)

	listed, err := repo.FindAll(ctx, orgID, ledgerID, nil, nil, http.QueryHeader{
		Limit: 10, Page: 1, SortOrder: "desc",
	}, mmodel.HolderOffV1)
	require.NoError(t, err, "a /v1 list must not require the holder columns")
	require.Len(t, listed, 1)
	assert.Nil(t, listed[0].HolderID)

	byAlias, err := repo.FindAlias(ctx, orgID, ledgerID, nil, *acc.Alias, mmodel.HolderOffV1)
	require.NoError(t, err, "a /v1 get-by-alias must not require the holder columns")
	assert.Nil(t, byAlias.HolderID)
}

// The transaction path resolves accounts by alias and by id on both contracts and
// never reads a holder, so it must be immune to the drift as well.
func TestIntegration_SchemaDrift_TransactionAccountLookupsSucceed(t *testing.T) {
	repo, _, orgID, ledgerID := driftedLedger(t)
	ctx := t.Context()

	acc := newDriftAccount(orgID, ledgerID)
	_, err := repo.Create(ctx, acc)
	require.NoError(t, err)

	byAlias, err := repo.ListAccountsByAlias(ctx, orgID, ledgerID, []string{*acc.Alias})
	require.NoError(t, err, "transaction alias resolution must survive the drift")
	require.Len(t, byAlias, 1)

	byIDs, err := repo.ListAccountsByIDs(ctx, orgID, ledgerID, []uuid.UUID{uuid.MustParse(acc.ID)})
	require.NoError(t, err, "transaction id resolution must survive the drift")
	require.Len(t, byIDs, 1)
}

// /v2 cannot honor its contract without the columns, so it must fail with the
// retryable sentinel rather than an opaque 500.
func TestIntegration_SchemaDrift_V2CreateWithHolderReportsMigrationPending(t *testing.T) {
	repo, _, orgID, ledgerID := driftedLedger(t)

	acc := newDriftAccount(orgID, ledgerID)
	holderID := uuid.Must(libCommons.GenerateUUIDv7()).String()
	acc.HolderID = &holderID

	_, err := repo.Create(t.Context(), acc)

	require.Error(t, err)

	var unavailable pkg.ServiceUnavailableError
	require.ErrorAs(t, err, &unavailable)
	assert.Equal(t, constant.ErrSchemaMigrationPending.Error(), unavailable.Code)
}

func TestIntegration_SchemaDrift_V2ReadsReportMigrationPending(t *testing.T) {
	repo, _, orgID, ledgerID := driftedLedger(t)
	ctx := t.Context()

	acc := newDriftAccount(orgID, ledgerID)
	_, err := repo.Create(ctx, acc)
	require.NoError(t, err)

	accountID := uuid.MustParse(acc.ID)

	reads := map[string]func() error{
		"Find": func() error {
			_, err := repo.Find(ctx, orgID, ledgerID, nil, accountID, mmodel.HolderOnV2)
			return err
		},
		"FindAll": func() error {
			_, err := repo.FindAll(ctx, orgID, ledgerID, nil, nil, http.QueryHeader{
				Limit: 10, Page: 1, SortOrder: "desc",
			}, mmodel.HolderOnV2)
			return err
		},
		"FindAlias": func() error {
			_, err := repo.FindAlias(ctx, orgID, ledgerID, nil, *acc.Alias, mmodel.HolderOnV2)
			return err
		},
		"FindWithDeleted": func() error {
			_, err := repo.FindWithDeleted(ctx, orgID, ledgerID, nil, accountID, mmodel.HolderOnV2)
			return err
		},
	}

	for name, read := range reads {
		t.Run(name, func(t *testing.T) {
			err := read()
			require.Error(t, err, "a /v2 read must not silently answer with a null holder")

			// The read must carry the cause, not fall through as an opaque 500.
			var unavailable pkg.ServiceUnavailableError
			require.ErrorAs(t, err, &unavailable)
			assert.Equal(t, constant.ErrSchemaMigrationPending.Error(), unavailable.Code)
		})
	}
}

// Parity: the /v1 contract must behave identically once the migrations land, so
// the mitigation is permanent rather than a transitional shim.
func TestIntegration_SchemaDrift_V1ParityAfterMigrationsApplied(t *testing.T) {
	container := pgtestutil.SetupMigratedContainer(t, "onboarding")
	repo := createRepository(t, container)
	ctx := t.Context()

	orgID := pgtestutil.CreateTestOrganization(t, container.DB)
	ledgerID := pgtestutil.CreateTestLedger(t, container.DB, orgID)
	pgtestutil.CreateTestAsset(t, container.DB, orgID, ledgerID, "USD")

	acc := newDriftAccount(orgID, ledgerID)
	created, err := repo.Create(ctx, acc)
	require.NoError(t, err)
	assert.Nil(t, created.HolderID)
	assert.False(t, created.HolderCheckSkipped)

	found, err := repo.Find(ctx, orgID, ledgerID, nil, uuid.MustParse(acc.ID), mmodel.HolderOffV1)
	require.NoError(t, err)
	assert.Nil(t, found.HolderID)
	assert.False(t, found.HolderCheckSkipped)

	// The persisted row carries exactly what the columns default to, which is what
	// the /v1 contract documents and what the drifted database produces.
	var (
		holderID           *string
		holderCheckSkipped bool
	)

	err = container.DB.QueryRow(
		`SELECT holder_id, holder_check_skipped FROM account WHERE id = $1`, acc.ID,
	).Scan(&holderID, &holderCheckSkipped)
	require.NoError(t, err)
	assert.Nil(t, holderID, "a /v1 create must persist holder_id NULL")
	assert.False(t, holderCheckSkipped, "a /v1 create must persist holder_check_skipped false")
}

// A /v2 update on a schema without the holder columns must fail at the lookup,
// BEFORE the row is mutated. The update statement itself names no holder column,
// so without the route policy on the lookup the mutation would land and the
// account.updated event would fire, and only the caller's re-read would 503 —
// a failure status over a write that actually happened.
func TestIntegration_SchemaDrift_V2UpdateFailsBeforeMutating(t *testing.T) {
	repo, db, orgID, ledgerID := driftedLedger(t)
	ctx := t.Context()

	acc := newDriftAccount(orgID, ledgerID)
	_, err := repo.Create(ctx, acc)
	require.NoError(t, err)

	accountID := uuid.MustParse(acc.ID)
	originalName := acc.Name

	// The lookup a /v2 update performs before mutating.
	_, err = repo.Find(ctx, orgID, ledgerID, nil, accountID, mmodel.HolderOnV2)

	require.Error(t, err, "a /v2 update must not get past the lookup on this schema")

	var unavailable pkg.ServiceUnavailableError
	require.ErrorAs(t, err, &unavailable)
	assert.Equal(t, constant.ErrSchemaMigrationPending.Error(), unavailable.Code)

	var persistedName string
	require.NoError(t, db.QueryRow(`SELECT name FROM account WHERE id = $1`, acc.ID).Scan(&persistedName))
	assert.Equal(t, originalName, persistedName, "the row must be untouched")
}

// The /v1 counterpart completes: its lookup names no holder column, and neither
// does the update statement.
func TestIntegration_SchemaDrift_V1UpdateSucceeds(t *testing.T) {
	repo, db, orgID, ledgerID := driftedLedger(t)
	ctx := t.Context()

	acc := newDriftAccount(orgID, ledgerID)
	_, err := repo.Create(ctx, acc)
	require.NoError(t, err)

	accountID := uuid.MustParse(acc.ID)

	found, err := repo.Find(ctx, orgID, ledgerID, nil, accountID, mmodel.HolderOffV1)
	require.NoError(t, err, "a /v1 update must get past the lookup")
	require.NotNil(t, found)

	renamed := "Drift Account Renamed"
	_, err = repo.Update(ctx, orgID, ledgerID, nil, accountID, &mmodel.Account{Name: renamed})
	require.NoError(t, err, "the update statement names no holder column")

	var persistedName string
	require.NoError(t, db.QueryRow(`SELECT name FROM account WHERE id = $1`, acc.ID).Scan(&persistedName))
	assert.Equal(t, renamed, persistedName)
}
