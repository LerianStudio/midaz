//go:build integration

// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package asset

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/pkg"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	pgtestutil "github.com/LerianStudio/midaz/v4/tests/utils/postgres"
)

// fixedAssetTime is the single timestamp every asset built by these tests
// carries, keeping runs deterministic.
var fixedAssetTime = time.Date(2026, time.March, 4, 5, 6, 7, 0, time.UTC)

// newAssetEntity builds an asset ready for repo.Create.
func newAssetEntity(orgID, ledgerID uuid.UUID, name, code string) *mmodel.Asset {
	return &mmodel.Asset{
		OrganizationID: orgID.String(),
		LedgerID:       ledgerID.String(),
		Name:           name,
		Type:           "currency",
		Code:           code,
		Status:         mmodel.Status{Code: "ACTIVE"},
		CreatedAt:      fixedAssetTime,
		UpdatedAt:      fixedAssetTime,
	}
}

// createNamedAsset inserts an active asset with the given name and code.
func createNamedAsset(t *testing.T, container *pgtestutil.ContainerResult, orgID, ledgerID uuid.UUID, name, code string) uuid.UUID {
	t.Helper()

	params := pgtestutil.DefaultAssetParams()
	params.Name = name
	params.Code = code

	return pgtestutil.CreateTestAssetWithParams(t, container.DB, orgID, ledgerID, params)
}

// assertAssetNameOrCodeConflict asserts err is the 0003 business conflict
// rather than a leaked driver error.
func assertAssetNameOrCodeConflict(t *testing.T, err error) {
	t.Helper()

	var conflict pkg.EntityConflictError
	require.ErrorAs(t, err, &conflict, "a name or code collision must surface as a business conflict")
	assert.Equal(t, constant.ErrAssetNameOrCodeDuplicate.Error(), conflict.Code)
}

// countLiveAssets returns the number of active assets in the ledger.
func countLiveAssets(t *testing.T, container *pgtestutil.ContainerResult, ledgerID uuid.UUID) int {
	t.Helper()

	var count int
	require.NoError(t, container.DB.QueryRow(
		`SELECT COUNT(*) FROM asset WHERE ledger_id = $1 AND deleted_at IS NULL`, ledgerID,
	).Scan(&count))

	return count
}

// Scenario asset-nome-com-percentual-nao-gera-conflito-falso: % is a literal, so
// a name containing it must not match an unrelated asset.
func TestIntegration_AssetRepository_FindByNameOrCode_NameUniqueness_PercentIsLiteral(t *testing.T) {
	container := pgtestutil.SetupMigratedContainer(t, "onboarding")

	repo := createRepository(t, container)
	ctx := context.Background()
	orgID := pgtestutil.CreateTestOrganization(t, container.DB)
	ledgerID := pgtestutil.CreateTestLedger(t, container.DB, orgID)
	existingID := createNamedAsset(t, container, orgID, ledgerID, "Wildcard AB", "WCA")

	exists, err := repo.FindByNameOrCode(ctx, orgID, ledgerID, "Wildcard A%", "WCB")

	assert.False(t, exists)
	require.NoError(t, err, "a % in the requested name must not match an unrelated asset")

	created, err := repo.Create(ctx, newAssetEntity(orgID, ledgerID, "Wildcard A%", "WCB"))
	require.NoError(t, err)
	assert.NotEqual(t, existingID.String(), created.ID)
	assert.Equal(t, 2, countLiveAssets(t, container, ledgerID))
}

// Scenario asset-nome-com-underscore-nao-gera-conflito-falso: _ is a literal too.
func TestIntegration_AssetRepository_FindByNameOrCode_NameUniqueness_UnderscoreIsLiteral(t *testing.T) {
	container := pgtestutil.SetupMigratedContainer(t, "onboarding")

	repo := createRepository(t, container)
	ctx := context.Background()
	orgID := pgtestutil.CreateTestOrganization(t, container.DB)
	ledgerID := pgtestutil.CreateTestLedger(t, container.DB, orgID)
	createNamedAsset(t, container, orgID, ledgerID, "abc", "ABC")

	exists, err := repo.FindByNameOrCode(ctx, orgID, ledgerID, "a_c", "AXC")

	assert.False(t, exists)
	require.NoError(t, err, "an _ in the requested name must not match an unrelated asset")
}

// Scenario asset-nome-com-barra-invertida-e-literal: \ is a literal, so the
// same name is a real conflict.
func TestIntegration_AssetRepository_FindByNameOrCode_NameUniqueness_BackslashIsLiteral(t *testing.T) {
	container := pgtestutil.SetupMigratedContainer(t, "onboarding")

	repo := createRepository(t, container)
	ctx := context.Background()
	orgID := pgtestutil.CreateTestOrganization(t, container.DB)
	ledgerID := pgtestutil.CreateTestLedger(t, container.DB, orgID)
	createNamedAsset(t, container, orgID, ledgerID, `Gold\Bar`, "GLD")

	exists, err := repo.FindByNameOrCode(ctx, orgID, ledgerID, `Gold\Bar`, "GL2")

	assert.True(t, exists)
	assertAssetNameOrCodeConflict(t, err)
	assert.Equal(t, 1, countLiveAssets(t, container, ledgerID))
}

// Scenario asset-nome-percentual-isolado-nao-sonda-o-namespace: a bare % must
// not answer whether any asset exists in the ledger.
func TestIntegration_AssetRepository_FindByNameOrCode_NameUniqueness_BarePercentDoesNotProbe(t *testing.T) {
	container := pgtestutil.SetupMigratedContainer(t, "onboarding")

	repo := createRepository(t, container)
	ctx := context.Background()
	orgID := pgtestutil.CreateTestOrganization(t, container.DB)
	ledgerID := pgtestutil.CreateTestLedger(t, container.DB, orgID)
	createNamedAsset(t, container, orgID, ledgerID, "US Dollar", "USD")
	createNamedAsset(t, container, orgID, ledgerID, "Euro", "EUR")

	exists, err := repo.FindByNameOrCode(ctx, orgID, ledgerID, "%", "PCT")

	assert.False(t, exists)
	require.NoError(t, err, "a bare % must not match every asset in the ledger")
}

// Scenario asset-conflito-real-de-nome-continua-detectado.
func TestIntegration_AssetRepository_FindByNameOrCode_NameUniqueness_RealNameConflictStillDetected(t *testing.T) {
	container := pgtestutil.SetupMigratedContainer(t, "onboarding")

	repo := createRepository(t, container)
	ctx := context.Background()
	orgID := pgtestutil.CreateTestOrganization(t, container.DB)
	ledgerID := pgtestutil.CreateTestLedger(t, container.DB, orgID)
	createNamedAsset(t, container, orgID, ledgerID, "US Dollar", "USD")

	exists, err := repo.FindByNameOrCode(ctx, orgID, ledgerID, "US Dollar", "USX")

	assert.True(t, exists)
	assertAssetNameOrCodeConflict(t, err)
}

// Scenario asset-conflito-de-nome-ignora-caixa.
func TestIntegration_AssetRepository_FindByNameOrCode_NameUniqueness_NameConflictIgnoresCase(t *testing.T) {
	container := pgtestutil.SetupMigratedContainer(t, "onboarding")

	repo := createRepository(t, container)
	ctx := context.Background()
	orgID := pgtestutil.CreateTestOrganization(t, container.DB)
	ledgerID := pgtestutil.CreateTestLedger(t, container.DB, orgID)
	createNamedAsset(t, container, orgID, ledgerID, "US Dollar", "USD")

	exists, err := repo.FindByNameOrCode(ctx, orgID, ledgerID, "us dollar", "USX")

	assert.True(t, exists)
	assertAssetNameOrCodeConflict(t, err)
}

// Scenario asset-conflito-de-codigo-com-nome-distinto.
func TestIntegration_AssetRepository_FindByNameOrCode_NameUniqueness_CodeConflictWithDistinctName(t *testing.T) {
	container := pgtestutil.SetupMigratedContainer(t, "onboarding")

	repo := createRepository(t, container)
	ctx := context.Background()
	orgID := pgtestutil.CreateTestOrganization(t, container.DB)
	ledgerID := pgtestutil.CreateTestLedger(t, container.DB, orgID)
	createNamedAsset(t, container, orgID, ledgerID, "US Dollar", "USD")

	exists, err := repo.FindByNameOrCode(ctx, orgID, ledgerID, "Dólar Americano", "USD")

	assert.True(t, exists)
	assertAssetNameOrCodeConflict(t, err)
}

// Scenario asset-existencia-por-codigo-sem-nome: an empty name skips the name
// leg instead of being compared as a name.
func TestIntegration_AssetRepository_FindByNameOrCode_NameUniqueness_CodeOnlyExistence(t *testing.T) {
	container := pgtestutil.SetupMigratedContainer(t, "onboarding")

	repo := createRepository(t, container)
	ctx := context.Background()
	orgID := pgtestutil.CreateTestOrganization(t, container.DB)
	ledgerID := pgtestutil.CreateTestLedger(t, container.DB, orgID)
	createNamedAsset(t, container, orgID, ledgerID, "US Dollar", "USD")

	exists, err := repo.FindByNameOrCode(ctx, orgID, ledgerID, "", "USD")

	assert.True(t, exists)
	assertAssetNameOrCodeConflict(t, err)

	exists, err = repo.FindByNameOrCode(ctx, orgID, ledgerID, "", "EUR")

	assert.False(t, exists)
	require.NoError(t, err, "an empty name must not match an asset by name")
}

// With both name and code empty no leg remains, so the check reports free
// rather than matching every asset in the ledger.
func TestIntegration_AssetRepository_FindByNameOrCode_NameUniqueness_BothEmptyIsFree(t *testing.T) {
	container := pgtestutil.SetupMigratedContainer(t, "onboarding")

	repo := createRepository(t, container)
	ctx := context.Background()
	orgID := pgtestutil.CreateTestOrganization(t, container.DB)
	ledgerID := pgtestutil.CreateTestLedger(t, container.DB, orgID)
	createNamedAsset(t, container, orgID, ledgerID, "US Dollar", "USD")

	exists, err := repo.FindByNameOrCode(ctx, orgID, ledgerID, "", "")

	assert.False(t, exists)
	require.NoError(t, err)
}

// Scenario asset-nome-soft-deletado-e-reutilizavel.
func TestIntegration_AssetRepository_FindByNameOrCode_NameUniqueness_SoftDeletedIsReusable(t *testing.T) {
	container := pgtestutil.SetupMigratedContainer(t, "onboarding")

	repo := createRepository(t, container)
	ctx := context.Background()
	orgID := pgtestutil.CreateTestOrganization(t, container.DB)
	ledgerID := pgtestutil.CreateTestLedger(t, container.DB, orgID)

	deletedAt := fixedAssetTime.Add(time.Hour)
	params := pgtestutil.DefaultAssetParams()
	params.Name = "Old Coin"
	params.Code = "OLD"
	params.DeletedAt = &deletedAt
	pgtestutil.CreateTestAssetWithParams(t, container.DB, orgID, ledgerID, params)

	exists, err := repo.FindByNameOrCode(ctx, orgID, ledgerID, "Old Coin", "OLD")
	assert.False(t, exists)
	require.NoError(t, err)

	created, err := repo.Create(ctx, newAssetEntity(orgID, ledgerID, "Old Coin", "OLD"))
	require.NoError(t, err, "a soft-deleted asset must release its name and code")
	assert.Equal(t, "Old Coin", created.Name)
}

// Scenario asset-mesmo-nome-em-outro-ledger-nao-conflita: uniqueness is scoped
// to one ledger.
func TestIntegration_AssetRepository_FindByNameOrCode_NameUniqueness_OtherLedgerDoesNotConflict(t *testing.T) {
	container := pgtestutil.SetupMigratedContainer(t, "onboarding")

	repo := createRepository(t, container)
	ctx := context.Background()
	orgID := pgtestutil.CreateTestOrganization(t, container.DB)

	l1Params := pgtestutil.DefaultLedgerParams()
	l1Params.Name = "L1"
	l1ID := pgtestutil.CreateTestLedgerWithParams(t, container.DB, orgID, l1Params)

	l2Params := pgtestutil.DefaultLedgerParams()
	l2Params.Name = "L2"
	l2ID := pgtestutil.CreateTestLedgerWithParams(t, container.DB, orgID, l2Params)

	createNamedAsset(t, container, orgID, l1ID, "US Dollar", "USD")

	exists, err := repo.FindByNameOrCode(ctx, orgID, l2ID, "US Dollar", "USD")
	assert.False(t, exists)
	require.NoError(t, err, "an asset in another ledger must not conflict")

	created, err := repo.Create(ctx, newAssetEntity(orgID, l2ID, "US Dollar", "USD"))
	require.NoError(t, err)
	assert.Equal(t, "USD", created.Code)
}

// Scenario asset-create-concorrente-barrado-pelos-indices: a name or code that
// slipped past the lookup (seeded directly) is rejected by the unique indexes
// and surfaces as the same 0003 conflict, not a raw driver error.
func TestIntegration_AssetRepository_Create_NameUniqueness_IndexRejectsDuplicate(t *testing.T) {
	tests := []struct {
		name  string
		asset func(orgID, ledgerID uuid.UUID) *mmodel.Asset
	}{
		{
			name: "name in another case",
			asset: func(orgID, ledgerID uuid.UUID) *mmodel.Asset {
				return newAssetEntity(orgID, ledgerID, "us dollar", "USX")
			},
		},
		{
			name: "same code",
			asset: func(orgID, ledgerID uuid.UUID) *mmodel.Asset {
				return newAssetEntity(orgID, ledgerID, "Dólar Americano", "USD")
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			container := pgtestutil.SetupMigratedContainer(t, "onboarding")

			repo := createRepository(t, container)
			ctx := context.Background()
			orgID := pgtestutil.CreateTestOrganization(t, container.DB)
			ledgerID := pgtestutil.CreateTestLedger(t, container.DB, orgID)
			createNamedAsset(t, container, orgID, ledgerID, "US Dollar", "USD")

			created, err := repo.Create(ctx, tc.asset(orgID, ledgerID))

			assert.Nil(t, created)
			assertAssetNameOrCodeConflict(t, err)
			assert.Equal(t, 1, countLiveAssets(t, container, ledgerID))
		})
	}
}

// Scenario asset-update-concorrente-barrado-pelo-indice: a rename into a name
// another active asset already holds is rejected by the unique index.
func TestIntegration_AssetRepository_Update_NameUniqueness_IndexRejectsRenameIntoExistingName(t *testing.T) {
	container := pgtestutil.SetupMigratedContainer(t, "onboarding")

	repo := createRepository(t, container)
	ctx := context.Background()
	orgID := pgtestutil.CreateTestOrganization(t, container.DB)
	ledgerID := pgtestutil.CreateTestLedger(t, container.DB, orgID)
	selfID := createNamedAsset(t, container, orgID, ledgerID, "Euro", "EUR")
	createNamedAsset(t, container, orgID, ledgerID, "US Dollar", "USD")

	updated, err := repo.Update(ctx, orgID, ledgerID, selfID, &mmodel.Asset{Name: "US DOLLAR"})

	assert.Nil(t, updated)
	assertAssetNameOrCodeConflict(t, err)
}

// Scenario asset-rename-so-de-caixa-passa-pelo-indice: the unique index
// excludes the row's own prior version, so a case-only rename succeeds.
func TestIntegration_AssetRepository_Update_NameUniqueness_IndexAllowsCaseOnlySelfRename(t *testing.T) {
	container := pgtestutil.SetupMigratedContainer(t, "onboarding")

	repo := createRepository(t, container)
	ctx := context.Background()
	orgID := pgtestutil.CreateTestOrganization(t, container.DB)
	ledgerID := pgtestutil.CreateTestLedger(t, container.DB, orgID)
	selfID := createNamedAsset(t, container, orgID, ledgerID, "US Dollar", "USD")

	updated, err := repo.Update(ctx, orgID, ledgerID, selfID, &mmodel.Asset{Name: "US DOLLAR"})

	require.NoError(t, err, "a case-only rename must not collide with the asset's own row")
	require.NotNil(t, updated)
	assert.Equal(t, "US DOLLAR", updated.Name)
}
