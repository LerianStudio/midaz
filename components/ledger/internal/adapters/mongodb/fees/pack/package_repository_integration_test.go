//go:build integration

// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package pack

import (
	"context"
	"strings"
	"testing"
	"time"

	feesmongo "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/mongodb/fees"
	feeconstant "github.com/LerianStudio/midaz/v4/components/ledger/pkg/feeshared/constant"
	"github.com/LerianStudio/midaz/v4/components/ledger/pkg/feeshared/model"
	feehttp "github.com/LerianStudio/midaz/v4/components/ledger/pkg/feeshared/nethttp"
	"github.com/LerianStudio/midaz/v4/pkg"
	mongotestutil "github.com/LerianStudio/midaz/v4/tests/utils/mongodb"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
)

// ============================================================================
// Test Helpers
// ============================================================================

// newPackConnection builds a fees MongoConnection backed by the testcontainer's
// already-connected client (injected via DB so GetDB bypasses lazy dialing).
func newPackConnection(t *testing.T, container *mongotestutil.ContainerResult) *feesmongo.MongoConnection {
	t.Helper()

	return &feesmongo.MongoConnection{
		ConnectionStringSource: container.URI,
		Database:               container.DBName,
		MaxPoolSize:            1,
		DB:                     container.Client,
	}
}

// newPackRepository constructs the repository against the real container and
// ensures indexes, mirroring the production constructor's EnsureIndexes call.
// White-box construction is required because the struct fields are unexported
// and the production constructor needs a live logger plus its own dial.
func newPackRepository(t *testing.T, container *mongotestutil.ContainerResult) *PackageMongoDBRepository {
	t.Helper()

	conn := newPackConnection(t, container)

	require.NoError(t, EnsureIndexes(context.Background(), conn),
		"EnsureIndexes must succeed during repository setup")

	return &PackageMongoDBRepository{connection: conn, Database: conn.Database}
}

// packCollection returns the raw package collection so tests can assert
// persisted state independently of the repository read path.
func packCollection(container *mongotestutil.ContainerResult) *mongo.Collection {
	return container.Client.
		Database(strings.ToLower(container.DBName)).
		Collection(strings.ToLower(feeconstant.PackageCollection))
}

// newTestFee returns a single valid fee for fixtures.
func newTestFee(label, creditAccount, value string) model.Fee {
	return model.Fee{
		FeeLabel: label,
		CalculationModel: &model.CalculationModel{
			ApplicationRule: model.Percentual,
			Calculations: []model.Calculation{
				{Type: model.Percentage, Value: value},
			},
		},
		ReferenceAmount:  feeconstant.ReferenceAmountOriginalAmount,
		Priority:         1,
		IsDeductibleFrom: boolPtr(true),
		CreditAccount:    creditAccount,
	}
}

// newTestPackage builds a fully populated Package owned by org/ledger.
// fixedTime keeps the timestamps deterministic (house rule: no time.Now in tests).
func newTestPackage(ledgerID uuid.UUID) *Package {
	fixedTime := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	route := "route-debit"

	return &Package{
		ID:               uuid.New(),
		FeeGroupLabel:    "Standard Fee Package",
		Description:      stringPtr("Standard administrative fees"),
		LedgerID:         ledgerID,
		TransactionRoute: &route,
		MinimumAmount:    decimal.RequireFromString("100"),
		MaximumAmount:    decimal.RequireFromString("2000"),
		WaivedAccounts:   &[]string{"acc001", "acc002"},
		Fees: map[string]model.Fee{
			"adminFee": newTestFee("Taxa Administrativa", "conta_receita_taxas_adm", "1.50"),
		},
		Enable:    boolPtr(true),
		CreatedAt: fixedTime,
		UpdatedAt: fixedTime,
	}
}

// ============================================================================
// Create Tests
// ============================================================================

func TestIntegration_PackRepo_Create_PersistsAllFields(t *testing.T) {
	container := mongotestutil.SetupContainer(t)
	repo := newPackRepository(t, container)
	ctx := context.Background()

	orgID := uuid.New()
	pkgEntity := newTestPackage(uuid.New())

	result, err := repo.Create(ctx, pkgEntity, orgID)
	require.NoError(t, err)
	require.NotNil(t, result)

	assert.Equal(t, pkgEntity.ID, result.ID)
	assert.Equal(t, pkgEntity.FeeGroupLabel, result.FeeGroupLabel)
	assert.True(t, result.MinimumAmount.Equal(decimal.RequireFromString("100")),
		"minimum amount must round-trip exactly via bsondecimal")
	assert.True(t, result.MaximumAmount.Equal(decimal.RequireFromString("2000")))
	require.Contains(t, result.Fees, "adminFee")
	require.NotNil(t, result.Fees["adminFee"].CalculationModel)
	require.Len(t, result.Fees["adminFee"].CalculationModel.Calculations, 1)
	assert.Equal(t, "1.5", result.Fees["adminFee"].CalculationModel.Calculations[0].Value,
		"calculation value must round-trip via bsondecimal canonical form")

	// Verify exactly one document landed in storage and its org tag is set
	// (organizationID is supplied at Create time, not on the entity).
	count, err := packCollection(container).CountDocuments(ctx,
		bson.M{"_id": pkgEntity.ID, "organization_id": orgID})
	require.NoError(t, err)
	assert.Equal(t, int64(1), count, "Create must persist exactly one org-tagged document")
}

// ============================================================================
// FindByID Tests
// ============================================================================

func TestIntegration_PackRepo_FindByID(t *testing.T) {
	container := mongotestutil.SetupContainer(t)
	repo := newPackRepository(t, container)
	ctx := context.Background()

	orgID := uuid.New()
	pkgEntity := newTestPackage(uuid.New())
	_, err := repo.Create(ctx, pkgEntity, orgID)
	require.NoError(t, err)

	result, err := repo.FindByID(ctx, pkgEntity.ID, orgID)
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, pkgEntity.ID, result.ID)
	assert.Equal(t, pkgEntity.FeeGroupLabel, result.FeeGroupLabel)
	assert.Equal(t, pkgEntity.LedgerID, result.LedgerID)
}

func TestIntegration_PackRepo_FindByID_NotFound(t *testing.T) {
	container := mongotestutil.SetupContainer(t)
	repo := newPackRepository(t, container)

	result, err := repo.FindByID(context.Background(), uuid.New(), uuid.New())
	require.Error(t, err)
	assert.Nil(t, result)
	assert.ErrorIs(t, err, mongo.ErrNoDocuments)
}

func TestIntegration_PackRepo_FindByID_WrongOrgIsolation(t *testing.T) {
	container := mongotestutil.SetupContainer(t)
	repo := newPackRepository(t, container)
	ctx := context.Background()

	pkgEntity := newTestPackage(uuid.New())
	_, err := repo.Create(ctx, pkgEntity, uuid.New())
	require.NoError(t, err)

	result, err := repo.FindByID(ctx, pkgEntity.ID, uuid.New())
	require.Error(t, err, "different org must not see the package")
	assert.Nil(t, result)
	assert.ErrorIs(t, err, mongo.ErrNoDocuments)
}

func TestIntegration_PackRepo_FindByID_ExcludesSoftDeleted(t *testing.T) {
	container := mongotestutil.SetupContainer(t)
	repo := newPackRepository(t, container)
	ctx := context.Background()

	orgID := uuid.New()
	pkgEntity := newTestPackage(uuid.New())
	_, err := repo.Create(ctx, pkgEntity, orgID)
	require.NoError(t, err)

	require.NoError(t, repo.SoftDelete(ctx, pkgEntity.ID, orgID))

	result, err := repo.FindByID(ctx, pkgEntity.ID, orgID)
	require.Error(t, err, "soft-deleted package must not be returned")
	assert.Nil(t, result)
	assert.ErrorIs(t, err, mongo.ErrNoDocuments)
}

// ============================================================================
// FindList Tests
// ============================================================================

func TestIntegration_PackRepo_FindList_FiltersAndPaginates(t *testing.T) {
	container := mongotestutil.SetupContainer(t)
	repo := newPackRepository(t, container)
	ctx := context.Background()

	orgID := uuid.New()
	ledgerID := uuid.New()
	otherLedger := uuid.New()

	for i := 0; i < 3; i++ {
		_, err := repo.Create(ctx, newTestPackage(ledgerID), orgID)
		require.NoError(t, err)
	}

	// One on a different ledger -> excluded by the ledger filter.
	_, err := repo.Create(ctx, newTestPackage(otherLedger), orgID)
	require.NoError(t, err)

	all, err := repo.FindList(ctx, feehttp.QueryHeader{
		OrganizationID: orgID,
		LedgerID:       ledgerID,
		Limit:          10,
		Page:           1,
	})
	require.NoError(t, err)
	assert.Len(t, all, 3, "ledger filter must scope the list")
	for _, p := range all {
		assert.Equal(t, ledgerID, p.LedgerID)
	}

	// Pagination over the 3 ledger-scoped docs: page1 limit2 -> 2, page2 -> 1.
	page1, err := repo.FindList(ctx, feehttp.QueryHeader{
		OrganizationID: orgID, LedgerID: ledgerID, Limit: 2, Page: 1,
	})
	require.NoError(t, err)
	assert.Len(t, page1, 2)

	page2, err := repo.FindList(ctx, feehttp.QueryHeader{
		OrganizationID: orgID, LedgerID: ledgerID, Limit: 2, Page: 2,
	})
	require.NoError(t, err)
	assert.Len(t, page2, 1)

	seen := map[uuid.UUID]bool{}
	for _, p := range append(page1, page2...) {
		assert.False(t, seen[p.ID], "pagination must not return duplicates")
		seen[p.ID] = true
	}
	assert.Len(t, seen, 3)
}

func TestIntegration_PackRepo_FindList_FilterByEnable(t *testing.T) {
	container := mongotestutil.SetupContainer(t)
	repo := newPackRepository(t, container)
	ctx := context.Background()

	orgID := uuid.New()
	ledgerID := uuid.New()

	enabled := newTestPackage(ledgerID)
	_, err := repo.Create(ctx, enabled, orgID)
	require.NoError(t, err)

	disabled := newTestPackage(ledgerID)
	disabled.Enable = boolPtr(false)
	_, err = repo.Create(ctx, disabled, orgID)
	require.NoError(t, err)

	results, err := repo.FindList(ctx, feehttp.QueryHeader{
		OrganizationID: orgID,
		LedgerID:       ledgerID,
		Enable:         boolPtr(true),
		Limit:          10,
		Page:           1,
	})
	require.NoError(t, err)
	require.Len(t, results, 1, "enable filter must exclude disabled packages")
	assert.Equal(t, enabled.ID, results[0].ID)
}

func TestIntegration_PackRepo_FindList_ExcludesSoftDeleted(t *testing.T) {
	container := mongotestutil.SetupContainer(t)
	repo := newPackRepository(t, container)
	ctx := context.Background()

	orgID := uuid.New()
	ledgerID := uuid.New()

	kept := newTestPackage(ledgerID)
	deleted := newTestPackage(ledgerID)
	_, err := repo.Create(ctx, kept, orgID)
	require.NoError(t, err)
	_, err = repo.Create(ctx, deleted, orgID)
	require.NoError(t, err)

	require.NoError(t, repo.SoftDelete(ctx, deleted.ID, orgID))

	results, err := repo.FindList(ctx, feehttp.QueryHeader{
		OrganizationID: orgID, LedgerID: ledgerID, Limit: 10, Page: 1,
	})
	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.Equal(t, kept.ID, results[0].ID)
}

// ============================================================================
// FindByOrganizationIDAndLedgerID / FindFeesAndAmountDataByPackageID Tests
// ============================================================================

func TestIntegration_PackRepo_FindByOrganizationIDAndLedgerID(t *testing.T) {
	container := mongotestutil.SetupContainer(t)
	repo := newPackRepository(t, container)
	ctx := context.Background()

	orgID := uuid.New()
	ledgerID := uuid.New()

	enabled1 := newTestPackage(ledgerID)
	enabled2 := newTestPackage(ledgerID)
	_, err := repo.Create(ctx, enabled1, orgID)
	require.NoError(t, err)
	_, err = repo.Create(ctx, enabled2, orgID)
	require.NoError(t, err)

	// Disabled -> excluded (method filters enable=true).
	disabled := newTestPackage(ledgerID)
	disabled.Enable = boolPtr(false)
	_, err = repo.Create(ctx, disabled, orgID)
	require.NoError(t, err)

	// Different ledger -> excluded.
	_, err = repo.Create(ctx, newTestPackage(uuid.New()), orgID)
	require.NoError(t, err)

	results, err := repo.FindByOrganizationIDAndLedgerID(ctx, orgID, ledgerID)
	require.NoError(t, err)
	require.Len(t, results, 2, "only enabled packages on the ledger are returned")

	ids := map[uuid.UUID]bool{results[0].ID: true, results[1].ID: true}
	assert.True(t, ids[enabled1.ID])
	assert.True(t, ids[enabled2.ID])
}

func TestIntegration_PackRepo_FindFeesAndAmountDataByPackageID(t *testing.T) {
	container := mongotestutil.SetupContainer(t)
	repo := newPackRepository(t, container)
	ctx := context.Background()

	orgID := uuid.New()
	ledgerID := uuid.New()
	pkgEntity := newTestPackage(ledgerID)
	_, err := repo.Create(ctx, pkgEntity, orgID)
	require.NoError(t, err)

	data, err := repo.FindFeesAndAmountDataByPackageID(ctx, orgID, pkgEntity.ID)
	require.NoError(t, err)
	require.NotNil(t, data)
	assert.Equal(t, ledgerID, data.LedgerID)
	assert.True(t, data.MinAmount.Equal(decimal.RequireFromString("100")))
	assert.True(t, data.MaxAmount.Equal(decimal.RequireFromString("2000")))
	require.Contains(t, data.Fees, "adminFee")
	assert.Equal(t, "Taxa Administrativa", data.Fees["adminFee"].FeeLabel)
}

func TestIntegration_PackRepo_FindFeesAndAmountDataByPackageID_NotFound(t *testing.T) {
	container := mongotestutil.SetupContainer(t)
	repo := newPackRepository(t, container)

	data, err := repo.FindFeesAndAmountDataByPackageID(context.Background(), uuid.New(), uuid.New())
	require.Error(t, err)
	assert.Nil(t, data)
	var notFound pkg.EntityNotFoundError
	require.ErrorAs(t, err, &notFound, "missing package must map to EntityNotFoundError")
	assert.Equal(t, "0007", notFound.Code)
}

// ============================================================================
// Update Tests
// ============================================================================

func TestIntegration_PackRepo_Update_PersistsChange(t *testing.T) {
	container := mongotestutil.SetupContainer(t)
	repo := newPackRepository(t, container)
	ctx := context.Background()

	orgID := uuid.New()
	pkgEntity := newTestPackage(uuid.New())
	_, err := repo.Create(ctx, pkgEntity, orgID)
	require.NoError(t, err)

	update := &bson.M{"$set": bson.M{"fee_group_label": "Updated Package Label"}}
	require.NoError(t, repo.Update(ctx, pkgEntity.ID, orgID, update))

	got, err := repo.FindByID(ctx, pkgEntity.ID, orgID)
	require.NoError(t, err)
	assert.Equal(t, "Updated Package Label", got.FeeGroupLabel, "label change must be persisted")
}

func TestIntegration_PackRepo_Update_DisablesWhenFeesEmptied(t *testing.T) {
	container := mongotestutil.SetupContainer(t)
	repo := newPackRepository(t, container)
	ctx := context.Background()

	orgID := uuid.New()
	pkgEntity := newTestPackage(uuid.New())
	_, err := repo.Create(ctx, pkgEntity, orgID)
	require.NoError(t, err)

	// Emptying the fees map must trigger the auto-disable side effect in Update.
	update := &bson.M{"$set": bson.M{"fees": bson.M{}}}
	require.NoError(t, repo.Update(ctx, pkgEntity.ID, orgID, update))

	got, err := repo.FindByID(ctx, pkgEntity.ID, orgID)
	require.NoError(t, err)
	require.NotNil(t, got.Enable)
	assert.False(t, *got.Enable, "package with no fees must be auto-disabled by Update")
}

func TestIntegration_PackRepo_Update_NotFound(t *testing.T) {
	container := mongotestutil.SetupContainer(t)
	repo := newPackRepository(t, container)

	update := &bson.M{"$set": bson.M{"fee_group_label": "x"}}
	err := repo.Update(context.Background(), uuid.New(), uuid.New(), update)

	require.Error(t, err)
	var notFound pkg.EntityNotFoundError
	require.ErrorAs(t, err, &notFound)
	assert.Equal(t, "0007", notFound.Code)
}

// ============================================================================
// SoftDelete Tests
// ============================================================================

func TestIntegration_PackRepo_SoftDelete_SetsDeletedAt(t *testing.T) {
	container := mongotestutil.SetupContainer(t)
	repo := newPackRepository(t, container)
	ctx := context.Background()

	orgID := uuid.New()
	pkgEntity := newTestPackage(uuid.New())
	_, err := repo.Create(ctx, pkgEntity, orgID)
	require.NoError(t, err)

	require.NoError(t, repo.SoftDelete(ctx, pkgEntity.ID, orgID))

	var raw bson.M
	err = packCollection(container).FindOne(ctx, bson.M{"_id": pkgEntity.ID}).Decode(&raw)
	require.NoError(t, err, "soft delete must keep the document")
	assert.NotNil(t, raw["deleted_at"], "deleted_at must be set on soft delete")
}

func TestIntegration_PackRepo_SoftDelete_NotFound(t *testing.T) {
	container := mongotestutil.SetupContainer(t)
	repo := newPackRepository(t, container)

	err := repo.SoftDelete(context.Background(), uuid.New(), uuid.New())
	require.Error(t, err)
	var notFound pkg.EntityNotFoundError
	require.ErrorAs(t, err, &notFound)
	assert.Equal(t, "0007", notFound.Code)
}

func TestIntegration_PackRepo_SoftDelete_Idempotency(t *testing.T) {
	container := mongotestutil.SetupContainer(t)
	repo := newPackRepository(t, container)
	ctx := context.Background()

	orgID := uuid.New()
	pkgEntity := newTestPackage(uuid.New())
	_, err := repo.Create(ctx, pkgEntity, orgID)
	require.NoError(t, err)

	require.NoError(t, repo.SoftDelete(ctx, pkgEntity.ID, orgID))

	err = repo.SoftDelete(ctx, pkgEntity.ID, orgID)
	require.Error(t, err, "deleting an already-deleted package must report not found")
	var notFound pkg.EntityNotFoundError
	require.ErrorAs(t, err, &notFound)
}
