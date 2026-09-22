//go:build integration

// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package ledger

import (
	"context"
	"sync"
	"testing"
	"time"

	libCommons "github.com/LerianStudio/lib-commons/v7/commons"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/pkg"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	pgtestutil "github.com/LerianStudio/midaz/v4/tests/utils/postgres"
)

// fixedLedgerTime is the single timestamp every ledger built by these tests
// carries, keeping runs deterministic.
var fixedLedgerTime = time.Date(2026, time.March, 4, 5, 6, 7, 0, time.UTC)

// newLedgerEntity builds a ledger ready for repo.Create.
func newLedgerEntity(orgID uuid.UUID, name string) *mmodel.Ledger {
	return &mmodel.Ledger{
		ID:             uuid.Must(libCommons.GenerateUUIDv7()).String(),
		OrganizationID: orgID.String(),
		Name:           name,
		Status:         mmodel.Status{Code: "ACTIVE"},
		CreatedAt:      fixedLedgerTime,
		UpdatedAt:      fixedLedgerTime,
	}
}

// assertLedgerNameConflict asserts err is the 0002 business conflict rather than
// a leaked driver error.
func assertLedgerNameConflict(t *testing.T, err error) {
	t.Helper()

	var conflict pkg.EntityConflictError
	require.ErrorAs(t, err, &conflict, "a name collision must surface as a business conflict")
	assert.Equal(t, constant.ErrLedgerNameConflict.Error(), conflict.Code)
}

// Scenario nome-com-percentual-nao-gera-conflito-falso: % is a literal, so a
// name containing it must not match an unrelated ledger.
func TestIntegration_LedgerRepository_FindByName_PercentIsLiteral(t *testing.T) {
	container := pgtestutil.SetupMigratedContainer(t, "onboarding")

	repo := createRepository(t, container)
	ctx := context.Background()
	orgID := pgtestutil.CreateTestOrganization(t, container.DB)

	params := pgtestutil.DefaultLedgerParams()
	params.Name = "PctABCTest"
	pgtestutil.CreateTestLedgerWithParams(t, container.DB, orgID, params)

	exists, err := repo.FindByName(ctx, orgID, "Pct%Test")

	assert.False(t, exists)
	require.NoError(t, err, "a % in the requested name must not match an unrelated ledger")

	created, err := repo.Create(ctx, newLedgerEntity(orgID, "Pct%Test"))
	require.NoError(t, err)
	assert.Equal(t, "Pct%Test", created.Name)
}

// Scenario nome-com-underscore-nao-gera-conflito-falso: _ is a literal too.
func TestIntegration_LedgerRepository_FindByName_UnderscoreIsLiteral(t *testing.T) {
	container := pgtestutil.SetupMigratedContainer(t, "onboarding")

	repo := createRepository(t, container)
	ctx := context.Background()
	orgID := pgtestutil.CreateTestOrganization(t, container.DB)

	params := pgtestutil.DefaultLedgerParams()
	params.Name = "AXB"
	pgtestutil.CreateTestLedgerWithParams(t, container.DB, orgID, params)

	exists, err := repo.FindByName(ctx, orgID, "A_B")

	assert.False(t, exists)
	require.NoError(t, err, "an _ in the requested name must not match an unrelated ledger")

	created, err := repo.Create(ctx, newLedgerEntity(orgID, "A_B"))
	require.NoError(t, err)
	assert.Equal(t, "A_B", created.Name)
}

// Scenario conflito-real-continua-detectado-no-create: strict equality is still
// case-insensitive, and other organizations are out of scope.
func TestIntegration_LedgerRepository_FindByName_RealConflictStillDetected(t *testing.T) {
	container := pgtestutil.SetupMigratedContainer(t, "onboarding")

	repo := createRepository(t, container)
	ctx := context.Background()
	orgID := pgtestutil.CreateTestOrganization(t, container.DB)

	params := pgtestutil.DefaultLedgerParams()
	params.Name = "Alpha"
	pgtestutil.CreateTestLedgerWithParams(t, container.DB, orgID, params)

	exists, err := repo.FindByName(ctx, orgID, "alpha")

	assert.True(t, exists)
	assertLedgerNameConflict(t, err)

	otherOrgID := pgtestutil.CreateTestOrganization(t, container.DB)

	exists, err = repo.FindByName(ctx, otherOrgID, "Alpha")
	assert.False(t, exists)
	require.NoError(t, err, "uniqueness is scoped to one organization")
}

// Scenario nome-de-ledger-soft-deletado-e-reutilizavel.
func TestIntegration_LedgerRepository_FindByName_SoftDeletedNameIsReusable(t *testing.T) {
	container := pgtestutil.SetupMigratedContainer(t, "onboarding")

	repo := createRepository(t, container)
	ctx := context.Background()
	orgID := pgtestutil.CreateTestOrganization(t, container.DB)

	deletedAt := fixedLedgerTime.Add(time.Hour)
	params := pgtestutil.DefaultLedgerParams()
	params.Name = "Old"
	params.DeletedAt = &deletedAt
	pgtestutil.CreateTestLedgerWithParams(t, container.DB, orgID, params)

	exists, err := repo.FindByName(ctx, orgID, "Old")
	assert.False(t, exists)
	require.NoError(t, err)

	created, err := repo.Create(ctx, newLedgerEntity(orgID, "Old"))
	require.NoError(t, err, "a soft-deleted ledger must release its name")
	assert.Equal(t, "Old", created.Name)
}

// FindByNameExcludingID must ignore the row being renamed — otherwise a ledger
// would conflict with itself — while still seeing every other live row.
func TestIntegration_LedgerRepository_FindByNameExcludingID_IgnoresOwnRow(t *testing.T) {
	container := pgtestutil.SetupMigratedContainer(t, "onboarding")

	repo := createRepository(t, container)
	ctx := context.Background()
	orgID := pgtestutil.CreateTestOrganization(t, container.DB)

	alphaParams := pgtestutil.DefaultLedgerParams()
	alphaParams.Name = "Alpha"
	alphaID := pgtestutil.CreateTestLedgerWithParams(t, container.DB, orgID, alphaParams)

	betaParams := pgtestutil.DefaultLedgerParams()
	betaParams.Name = "Beta"
	betaID := pgtestutil.CreateTestLedgerWithParams(t, container.DB, orgID, betaParams)

	// Its own name, in its own case and in another, is not a conflict.
	exists, err := repo.FindByNameExcludingID(ctx, orgID, "Alpha", alphaID)
	assert.False(t, exists)
	require.NoError(t, err)

	exists, err = repo.FindByNameExcludingID(ctx, orgID, "alpha", alphaID)
	assert.False(t, exists)
	require.NoError(t, err)

	// Another ledger's name is a conflict, case-insensitively.
	exists, err = repo.FindByNameExcludingID(ctx, orgID, "alpha", betaID)
	assert.True(t, exists)
	assertLedgerNameConflict(t, err)
}

// The application check is a courtesy; the index is the guarantee. An insert
// that bypasses the check must still surface 0002, never a driver error.
func TestIntegration_LedgerRepository_Create_DuplicateNameMapsToConflict(t *testing.T) {
	container := pgtestutil.SetupMigratedContainer(t, "onboarding")

	repo := createRepository(t, container)
	ctx := context.Background()
	orgID := pgtestutil.CreateTestOrganization(t, container.DB)

	_, err := repo.Create(ctx, newLedgerEntity(orgID, "Alpha"))
	require.NoError(t, err)

	_, err = repo.Create(ctx, newLedgerEntity(orgID, "alpha"))
	require.Error(t, err)
	assertLedgerNameConflict(t, err)
}

// The same guarantee on the rename path.
func TestIntegration_LedgerRepository_Update_DuplicateNameMapsToConflict(t *testing.T) {
	container := pgtestutil.SetupMigratedContainer(t, "onboarding")

	repo := createRepository(t, container)
	ctx := context.Background()
	orgID := pgtestutil.CreateTestOrganization(t, container.DB)

	alphaParams := pgtestutil.DefaultLedgerParams()
	alphaParams.Name = "Alpha"
	pgtestutil.CreateTestLedgerWithParams(t, container.DB, orgID, alphaParams)

	betaParams := pgtestutil.DefaultLedgerParams()
	betaParams.Name = "Beta"
	betaID := pgtestutil.CreateTestLedgerWithParams(t, container.DB, orgID, betaParams)

	_, err := repo.Update(ctx, orgID, betaID, &mmodel.Ledger{Name: "alpha"})
	require.Error(t, err)
	assertLedgerNameConflict(t, err)

	var persistedName string
	require.NoError(t, container.DB.QueryRowContext(ctx, `SELECT name FROM ledger WHERE id = $1`, betaID).Scan(&persistedName))
	assert.Equal(t, "Beta", persistedName, "a rejected rename must persist nothing")

	// Re-sending its own name is not a conflict with itself.
	updated, err := repo.Update(ctx, orgID, betaID, &mmodel.Ledger{Name: "Beta"})
	require.NoError(t, err)
	assert.Equal(t, "Beta", updated.Name)
}

// Scenario creates-concorrentes-apenas-um-vence: the index is the only
// serialization point, so concurrent inserts of one name settle at exactly one
// winner, every loser carrying 0002.
func TestIntegration_LedgerRepository_Create_ConcurrentSameNameHasOneWinner(t *testing.T) {
	container := pgtestutil.SetupMigratedContainer(t, "onboarding")

	repo := createRepository(t, container)
	ctx := context.Background()
	orgID := pgtestutil.CreateTestOrganization(t, container.DB)

	const writers = 8

	var (
		wg    sync.WaitGroup
		start = make(chan struct{})
		errs  = make([]error, writers)
	)

	wg.Add(writers)

	for i := 0; i < writers; i++ {
		go func(index int) {
			defer wg.Done()

			<-start

			_, errs[index] = repo.Create(ctx, newLedgerEntity(orgID, "Race"))
		}(i)
	}

	close(start)
	wg.Wait()

	successes := 0

	for _, err := range errs {
		if err == nil {
			successes++

			continue
		}

		assertLedgerNameConflict(t, err)
	}

	assert.Equal(t, 1, successes, "exactly one concurrent create must win")

	var liveRows int
	require.NoError(t, container.DB.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM ledger WHERE organization_id = $1 AND LOWER(name) = 'race' AND deleted_at IS NULL`,
		orgID,
	).Scan(&liveRows))
	assert.Equal(t, 1, liveRows, "no duplicate may persist")
}
