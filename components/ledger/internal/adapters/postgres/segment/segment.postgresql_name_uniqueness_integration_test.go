//go:build integration

// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package segment

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

// fixedSegmentTime is the single timestamp every segment built by these tests
// carries, keeping runs deterministic.
var fixedSegmentTime = time.Date(2026, time.March, 4, 5, 6, 7, 0, time.UTC)

// newSegmentEntity builds a segment ready for repo.Create.
func newSegmentEntity(orgID, ledgerID uuid.UUID, name string) *mmodel.Segment {
	return &mmodel.Segment{
		OrganizationID: orgID.String(),
		LedgerID:       ledgerID.String(),
		Name:           name,
		Status:         mmodel.Status{Code: "ACTIVE"},
		CreatedAt:      fixedSegmentTime,
		UpdatedAt:      fixedSegmentTime,
	}
}

// createNamedSegment inserts an active segment with the given name.
func createNamedSegment(t *testing.T, container *pgtestutil.ContainerResult, orgID, ledgerID uuid.UUID, name string) uuid.UUID {
	t.Helper()

	params := pgtestutil.DefaultSegmentParams()
	params.Name = name

	return pgtestutil.CreateTestSegmentWithParams(t, container.DB, orgID, ledgerID, params)
}

// assertSegmentNameConflict asserts err is the 0015 business conflict rather
// than a leaked driver error.
func assertSegmentNameConflict(t *testing.T, err error) {
	t.Helper()

	var conflict pkg.EntityConflictError
	require.ErrorAs(t, err, &conflict, "a name collision must surface as a business conflict")
	assert.Equal(t, constant.ErrDuplicateSegmentName.Error(), conflict.Code)
}

// Scenario segment-nome-com-percentual-nao-gera-conflito-falso: % is a literal,
// so a name containing it must not match an unrelated segment.
func TestIntegration_SegmentRepository_ExistsByName_NameUniqueness_PercentIsLiteral(t *testing.T) {
	container := pgtestutil.SetupMigratedContainer(t, "onboarding")

	repo := createRepository(t, container)
	ctx := context.Background()
	orgID := pgtestutil.CreateTestOrganization(t, container.DB)
	ledgerID := pgtestutil.CreateTestLedger(t, container.DB, orgID)
	existingID := createNamedSegment(t, container, orgID, ledgerID, "E2E Wild AB")

	exists, err := repo.ExistsByName(ctx, orgID, ledgerID, "E2E Wild A%")

	assert.False(t, exists)
	require.NoError(t, err, "a % in the requested name must not match an unrelated segment")

	created, err := repo.Create(ctx, newSegmentEntity(orgID, ledgerID, "E2E Wild A%"))
	require.NoError(t, err)
	assert.Equal(t, "E2E Wild A%", created.Name)
	assert.NotEqual(t, existingID.String(), created.ID)
}

// Scenario segment-nome-com-underscore-nao-gera-conflito-falso: _ is a literal too.
func TestIntegration_SegmentRepository_ExistsByName_NameUniqueness_UnderscoreIsLiteral(t *testing.T) {
	container := pgtestutil.SetupMigratedContainer(t, "onboarding")

	repo := createRepository(t, container)
	ctx := context.Background()
	orgID := pgtestutil.CreateTestOrganization(t, container.DB)
	ledgerID := pgtestutil.CreateTestLedger(t, container.DB, orgID)
	createNamedSegment(t, container, orgID, ledgerID, "abc")

	exists, err := repo.ExistsByName(ctx, orgID, ledgerID, "a_c")

	assert.False(t, exists)
	require.NoError(t, err, "an _ in the requested name must not match an unrelated segment")
}

// Scenario segment-nome-percentual-isolado-nao-sonda-o-namespace: a bare % must
// not answer whether any segment exists in the ledger.
func TestIntegration_SegmentRepository_ExistsByName_NameUniqueness_BarePercentDoesNotProbe(t *testing.T) {
	container := pgtestutil.SetupMigratedContainer(t, "onboarding")

	repo := createRepository(t, container)
	ctx := context.Background()
	orgID := pgtestutil.CreateTestOrganization(t, container.DB)
	ledgerID := pgtestutil.CreateTestLedger(t, container.DB, orgID)
	createNamedSegment(t, container, orgID, ledgerID, "Retail")
	createNamedSegment(t, container, orgID, ledgerID, "Wholesale")

	exists, err := repo.ExistsByName(ctx, orgID, ledgerID, "%")

	assert.False(t, exists)
	require.NoError(t, err, "a bare % must not match every segment in the ledger")
}

// Scenario segment-conflito-real-continua-detectado.
func TestIntegration_SegmentRepository_ExistsByName_NameUniqueness_RealConflictStillDetected(t *testing.T) {
	container := pgtestutil.SetupMigratedContainer(t, "onboarding")

	repo := createRepository(t, container)
	ctx := context.Background()
	orgID := pgtestutil.CreateTestOrganization(t, container.DB)
	ledgerID := pgtestutil.CreateTestLedger(t, container.DB, orgID)
	createNamedSegment(t, container, orgID, ledgerID, "Retail")

	exists, err := repo.ExistsByName(ctx, orgID, ledgerID, "Retail")

	assert.True(t, exists)
	assertSegmentNameConflict(t, err)
}

// Scenario segment-conflito-ignora-caixa.
func TestIntegration_SegmentRepository_ExistsByName_NameUniqueness_ConflictIgnoresCase(t *testing.T) {
	container := pgtestutil.SetupMigratedContainer(t, "onboarding")

	repo := createRepository(t, container)
	ctx := context.Background()
	orgID := pgtestutil.CreateTestOrganization(t, container.DB)
	ledgerID := pgtestutil.CreateTestLedger(t, container.DB, orgID)
	createNamedSegment(t, container, orgID, ledgerID, "Retail")

	exists, err := repo.ExistsByName(ctx, orgID, ledgerID, "retail")

	assert.True(t, exists)
	assertSegmentNameConflict(t, err)
}

// Scenario segment-nome-soft-deletado-e-reutilizavel.
func TestIntegration_SegmentRepository_ExistsByName_NameUniqueness_SoftDeletedNameIsReusable(t *testing.T) {
	container := pgtestutil.SetupMigratedContainer(t, "onboarding")

	repo := createRepository(t, container)
	ctx := context.Background()
	orgID := pgtestutil.CreateTestOrganization(t, container.DB)
	ledgerID := pgtestutil.CreateTestLedger(t, container.DB, orgID)

	deletedAt := fixedSegmentTime.Add(time.Hour)
	params := pgtestutil.DefaultSegmentParams()
	params.Name = "Legacy"
	params.DeletedAt = &deletedAt
	pgtestutil.CreateTestSegmentWithParams(t, container.DB, orgID, ledgerID, params)

	exists, err := repo.ExistsByName(ctx, orgID, ledgerID, "Legacy")
	assert.False(t, exists)
	require.NoError(t, err)

	created, err := repo.Create(ctx, newSegmentEntity(orgID, ledgerID, "Legacy"))
	require.NoError(t, err, "a soft-deleted segment must release its name")
	assert.Equal(t, "Legacy", created.Name)
}

// Scenario segment-mesmo-nome-em-outro-ledger-nao-conflita: uniqueness is scoped
// to one ledger.
func TestIntegration_SegmentRepository_ExistsByName_NameUniqueness_OtherLedgerDoesNotConflict(t *testing.T) {
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

	createNamedSegment(t, container, orgID, l1ID, "Retail")

	exists, err := repo.ExistsByName(ctx, orgID, l2ID, "Retail")
	assert.False(t, exists)
	require.NoError(t, err, "a segment in another ledger must not conflict")

	created, err := repo.Create(ctx, newSegmentEntity(orgID, l2ID, "Retail"))
	require.NoError(t, err)
	assert.Equal(t, "Retail", created.Name)
}

// Scenario segment-rename-so-de-caixa-nao-colide-consigo: the segment's own row
// is excluded, so a case-only rename is not a conflict.
func TestIntegration_SegmentRepository_ExistsByNameExcludingID_NameUniqueness_SelfIsExcluded(t *testing.T) {
	container := pgtestutil.SetupMigratedContainer(t, "onboarding")

	repo := createRepository(t, container)
	ctx := context.Background()
	orgID := pgtestutil.CreateTestOrganization(t, container.DB)
	ledgerID := pgtestutil.CreateTestLedger(t, container.DB, orgID)
	selfID := createNamedSegment(t, container, orgID, ledgerID, "Retail")

	exists, err := repo.ExistsByNameExcludingID(ctx, orgID, ledgerID, "RETAIL", selfID)

	assert.False(t, exists)
	assert.NoError(t, err)
}

// Scenario segment-rename-so-de-caixa-colide-com-outro: another active segment
// that already carries the target name (in any case) is still a conflict.
func TestIntegration_SegmentRepository_ExistsByNameExcludingID_NameUniqueness_OtherSegmentStillConflicts(t *testing.T) {
	container := pgtestutil.SetupMigratedContainer(t, "onboarding")

	repo := createRepository(t, container)
	ctx := context.Background()
	orgID := pgtestutil.CreateTestOrganization(t, container.DB)
	ledgerID := pgtestutil.CreateTestLedger(t, container.DB, orgID)
	selfID := createNamedSegment(t, container, orgID, ledgerID, "Retail")
	createNamedSegment(t, container, orgID, ledgerID, "RETAIL")

	exists, err := repo.ExistsByNameExcludingID(ctx, orgID, ledgerID, "RETAIL", selfID)

	assert.True(t, exists)
	assertSegmentNameConflict(t, err)
}

// Scenario segment-rename-nao-colide-com-soft-deletado.
func TestIntegration_SegmentRepository_ExistsByNameExcludingID_NameUniqueness_SoftDeletedIsIgnored(t *testing.T) {
	container := pgtestutil.SetupMigratedContainer(t, "onboarding")

	repo := createRepository(t, container)
	ctx := context.Background()
	orgID := pgtestutil.CreateTestOrganization(t, container.DB)
	ledgerID := pgtestutil.CreateTestLedger(t, container.DB, orgID)
	selfID := createNamedSegment(t, container, orgID, ledgerID, "Retail")
	otherID := createNamedSegment(t, container, orgID, ledgerID, "Wholesale")
	require.NoError(t, repo.Delete(ctx, orgID, ledgerID, otherID))

	exists, err := repo.ExistsByNameExcludingID(ctx, orgID, ledgerID, "wholesale", selfID)

	assert.False(t, exists)
	assert.NoError(t, err)
}
