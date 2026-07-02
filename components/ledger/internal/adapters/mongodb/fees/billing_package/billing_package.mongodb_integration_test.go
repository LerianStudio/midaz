//go:build integration

// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package billing_package_test

import (
	"context"
	"strings"
	"testing"

	feesmongo "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/mongodb/fees"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/mongodb/fees/billing_package"
	feeconstant "github.com/LerianStudio/midaz/v4/components/ledger/pkg/feeshared/constant"
	"github.com/LerianStudio/midaz/v4/components/ledger/pkg/feeshared/model"
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

// newConnection builds a fees MongoConnection backed by the testcontainer's
// already-connected client (injected via DB so GetDB bypasses lazy dialing).
func newConnection(t *testing.T, container *mongotestutil.ContainerResult) *feesmongo.MongoConnection {
	t.Helper()

	return &feesmongo.MongoConnection{
		ConnectionStringSource: container.URI,
		Database:               container.DBName,
		MaxPoolSize:            1,
		DB:                     container.Client,
	}
}

// newRepository builds the repository and ensures indexes against the real
// container, mirroring the production constructor's EnsureIndexes call.
func newRepository(t *testing.T, container *mongotestutil.ContainerResult) *billing_package.BillingPackageMongoDBRepository {
	t.Helper()

	conn := newConnection(t, container)

	require.NoError(t, billing_package.EnsureIndexes(context.Background(), conn),
		"EnsureIndexes must succeed during repository setup")

	return billing_package.NewBillingPackageMongoDBRepositoryFromConnection(conn)
}

// collection returns the raw billing_package collection on the test database so
// tests can assert persisted state directly, independent of the repository read
// path.
func collection(container *mongotestutil.ContainerResult) *mongo.Collection {
	return container.Client.
		Database(strings.ToLower(container.DBName)).
		Collection(strings.ToLower(feeconstant.BillingPackageCollection))
}

func ptr[T any](v T) *T { return &v }

// newVolumePackage returns a fully-populated volume billing package owned by the
// given org/ledger. Every optional branch in FromEntity/ToEntity (tiers,
// discount tiers, event filter) is exercised so the round-trip is meaningful.
func newVolumePackage(orgID, ledgerID string) *model.BillingPackage {
	maxQty := int64(999)

	return &model.BillingPackage{
		ID:                 uuid.New().String(),
		OrganizationID:     orgID,
		LedgerID:           ledgerID,
		Label:              "Monthly Volume Billing",
		Description:        ptr("Charges per completed transaction route"),
		Type:               model.BillingPackageTypeVolume,
		Enable:             ptr(true),
		EventFilter:        &model.EventFilter{TransactionRoute: "route-vol", Status: "APPROVED"},
		PricingModel:       ptr("tiered"),
		AssetCode:          ptr("BRL"),
		DebitAccountAlias:  ptr("account_fees_debit"),
		CreditAccountAlias: ptr("account_fees_credit"),
		Tiers: []model.PricingTier{
			{MinQuantity: 0, MaxQuantity: &maxQty, UnitPrice: decimal.RequireFromString("1.50")},
			{MinQuantity: 1000, UnitPrice: decimal.RequireFromString("0.75")},
		},
		DiscountTiers: []model.DiscountTier{
			{MinQuantity: 5000, DiscountPercentage: decimal.RequireFromString("10.00")},
		},
		CreatedAt: "2026-01-01T00:00:00Z",
		UpdatedAt: "2026-01-01T00:00:00Z",
	}
}

// newMaintenancePackage returns a maintenance billing package, exercising the
// maintenance-only fields (fee amount, account target).
func newMaintenancePackage(orgID, ledgerID, route string) *model.BillingPackage {
	segID := uuid.New().String()
	amt := decimal.RequireFromString("50.00")

	return &model.BillingPackage{
		ID:             uuid.New().String(),
		OrganizationID: orgID,
		LedgerID:       ledgerID,
		Label:          "Account Maintenance",
		Type:           model.BillingPackageTypeMaintenance,
		Enable:         ptr(true),
		EventFilter:    &model.EventFilter{TransactionRoute: route, Status: "APPROVED"},
		FeeAmount:      &amt,
		AccountTarget:  &model.AccountTarget{SegmentID: ptr(uuid.MustParse(segID))},
		CreatedAt:      "2026-01-01T00:00:00Z",
		UpdatedAt:      "2026-01-01T00:00:00Z",
	}
}

// ============================================================================
// Create Tests
// ============================================================================

func TestIntegration_BillingPackageRepo_Create_PersistsAllFields(t *testing.T) {
	container := mongotestutil.SetupContainer(t)
	repo := newRepository(t, container)
	ctx := context.Background()

	orgID := uuid.New().String()
	bp := newVolumePackage(orgID, uuid.New().String())

	result, err := repo.Create(ctx, bp)
	require.NoError(t, err)
	require.NotNil(t, result)

	// Returned entity round-trips through FromEntity -> ToEntity.
	assert.Equal(t, bp.ID, result.ID)
	assert.Equal(t, bp.Label, result.Label)
	assert.Equal(t, model.BillingPackageTypeVolume, result.Type)
	require.NotNil(t, result.Enable)
	assert.True(t, *result.Enable)
	require.NotNil(t, result.EventFilter)
	assert.Equal(t, "route-vol", result.EventFilter.TransactionRoute)
	require.Len(t, result.Tiers, 2)
	assert.True(t, result.Tiers[0].UnitPrice.Equal(decimal.RequireFromString("1.50")),
		"first tier unit price must round-trip exactly")
	require.NotNil(t, result.Tiers[0].MaxQuantity)
	assert.Equal(t, int64(999), *result.Tiers[0].MaxQuantity)
	require.Len(t, result.DiscountTiers, 1)
	assert.True(t, result.DiscountTiers[0].DiscountPercentage.Equal(decimal.RequireFromString("10.00")))

	// Verify exactly one document landed in storage.
	count, err := collection(container).CountDocuments(ctx, bson.M{"_id": bp.ID})
	require.NoError(t, err)
	assert.Equal(t, int64(1), count, "Create must persist exactly one document")
}

func TestIntegration_BillingPackageRepo_Create_Nil(t *testing.T) {
	container := mongotestutil.SetupContainer(t)
	repo := newRepository(t, container)

	result, err := repo.Create(context.Background(), nil)

	require.Error(t, err, "nil billing package must be rejected")
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "cannot be nil")
}

// ============================================================================
// FindByID Tests
// ============================================================================

func TestIntegration_BillingPackageRepo_FindByID(t *testing.T) {
	container := mongotestutil.SetupContainer(t)
	repo := newRepository(t, container)
	ctx := context.Background()

	orgID := uuid.New().String()
	bp := newVolumePackage(orgID, uuid.New().String())
	_, err := repo.Create(ctx, bp)
	require.NoError(t, err)

	result, err := repo.FindByID(ctx, bp.ID, orgID)
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, bp.ID, result.ID)
	assert.Equal(t, orgID, result.OrganizationID)
	assert.Equal(t, bp.Label, result.Label)
}

func TestIntegration_BillingPackageRepo_FindByID_NotFound(t *testing.T) {
	container := mongotestutil.SetupContainer(t)
	repo := newRepository(t, container)

	result, err := repo.FindByID(context.Background(), uuid.New().String(), uuid.New().String())

	require.Error(t, err)
	assert.Nil(t, result)
	assert.ErrorIs(t, err, mongo.ErrNoDocuments, "missing document must surface ErrNoDocuments")
}

func TestIntegration_BillingPackageRepo_FindByID_WrongOrgIsolation(t *testing.T) {
	container := mongotestutil.SetupContainer(t)
	repo := newRepository(t, container)
	ctx := context.Background()

	bp := newVolumePackage(uuid.New().String(), uuid.New().String())
	_, err := repo.Create(ctx, bp)
	require.NoError(t, err)

	// Same id, different org must not leak across organizations.
	result, err := repo.FindByID(ctx, bp.ID, uuid.New().String())
	require.Error(t, err)
	assert.Nil(t, result)
	assert.ErrorIs(t, err, mongo.ErrNoDocuments)
}

func TestIntegration_BillingPackageRepo_FindByID_ExcludesSoftDeleted(t *testing.T) {
	container := mongotestutil.SetupContainer(t)
	repo := newRepository(t, container)
	ctx := context.Background()

	orgID := uuid.New().String()
	bp := newVolumePackage(orgID, uuid.New().String())
	_, err := repo.Create(ctx, bp)
	require.NoError(t, err)

	require.NoError(t, repo.SoftDelete(ctx, bp.ID, orgID))

	result, err := repo.FindByID(ctx, bp.ID, orgID)
	require.Error(t, err, "soft-deleted package must not be returned by FindByID")
	assert.Nil(t, result)
	assert.ErrorIs(t, err, mongo.ErrNoDocuments)
}

// ============================================================================
// FindAll Tests
// ============================================================================

func TestIntegration_BillingPackageRepo_FindAll_FiltersAndPaginates(t *testing.T) {
	container := mongotestutil.SetupContainer(t)
	repo := newRepository(t, container)
	ctx := context.Background()

	orgID := uuid.New().String()
	ledgerID := uuid.New().String()
	otherLedger := uuid.New().String()

	// 3 volume packages on the target ledger.
	for i := 0; i < 3; i++ {
		_, err := repo.Create(ctx, newVolumePackage(orgID, ledgerID))
		require.NoError(t, err)
	}

	// 1 maintenance package on the target ledger (different type).
	_, err := repo.Create(ctx, newMaintenancePackage(orgID, ledgerID, "route-maint"))
	require.NoError(t, err)

	// 1 volume package on a DIFFERENT ledger (must be excluded by ledger filter).
	_, err = repo.Create(ctx, newVolumePackage(orgID, otherLedger))
	require.NoError(t, err)

	// Filter by ledger only: 3 volume + 1 maintenance = 4.
	all, total, err := repo.FindAll(ctx, orgID, ledgerID, "", 10, 1)
	require.NoError(t, err)
	assert.Equal(t, int64(4), total, "ledger filter must scope the count")
	assert.Len(t, all, 4)

	// Filter by ledger + type=volume: 3.
	volumes, total, err := repo.FindAll(ctx, orgID, ledgerID, model.BillingPackageTypeVolume, 10, 1)
	require.NoError(t, err)
	assert.Equal(t, int64(3), total)
	assert.Len(t, volumes, 3)
	for _, v := range volumes {
		assert.Equal(t, model.BillingPackageTypeVolume, v.Type)
	}

	// Pagination: limit 2 over the 4 ledger-scoped docs -> page 1 has 2, page 2 has 2.
	page1, total, err := repo.FindAll(ctx, orgID, ledgerID, "", 2, 1)
	require.NoError(t, err)
	assert.Equal(t, int64(4), total, "total must reflect the full filtered set, not the page")
	assert.Len(t, page1, 2)

	page2, _, err := repo.FindAll(ctx, orgID, ledgerID, "", 2, 2)
	require.NoError(t, err)
	assert.Len(t, page2, 2)

	// Pages must not overlap.
	seen := map[string]bool{}
	for _, p := range append(page1, page2...) {
		assert.False(t, seen[p.ID], "pagination must not return duplicates")
		seen[p.ID] = true
	}
	assert.Len(t, seen, 4)
}

func TestIntegration_BillingPackageRepo_FindAll_ExcludesSoftDeleted(t *testing.T) {
	container := mongotestutil.SetupContainer(t)
	repo := newRepository(t, container)
	ctx := context.Background()

	orgID := uuid.New().String()
	ledgerID := uuid.New().String()

	kept := newVolumePackage(orgID, ledgerID)
	deleted := newVolumePackage(orgID, ledgerID)
	_, err := repo.Create(ctx, kept)
	require.NoError(t, err)
	_, err = repo.Create(ctx, deleted)
	require.NoError(t, err)

	require.NoError(t, repo.SoftDelete(ctx, deleted.ID, orgID))

	all, total, err := repo.FindAll(ctx, orgID, ledgerID, "", 10, 1)
	require.NoError(t, err)
	assert.Equal(t, int64(1), total)
	require.Len(t, all, 1)
	assert.Equal(t, kept.ID, all[0].ID)
}

func TestIntegration_BillingPackageRepo_FindAll_Empty(t *testing.T) {
	container := mongotestutil.SetupContainer(t)
	repo := newRepository(t, container)

	all, total, err := repo.FindAll(context.Background(), uuid.New().String(), uuid.New().String(), "", 10, 1)
	require.NoError(t, err, "empty result must not error")
	assert.Equal(t, int64(0), total)
	assert.Empty(t, all)
}

// ============================================================================
// FindMatchingPackages / FindActiveByType Tests
// ============================================================================

func TestIntegration_BillingPackageRepo_FindMatchingPackages(t *testing.T) {
	container := mongotestutil.SetupContainer(t)
	repo := newRepository(t, container)
	ctx := context.Background()

	orgID := uuid.New().String()
	ledgerID := uuid.New().String()
	route := "route-match"

	// Enabled package on the target route -> must match.
	match := newMaintenancePackage(orgID, ledgerID, route)
	_, err := repo.Create(ctx, match)
	require.NoError(t, err)

	// Disabled package on the same route -> must NOT match.
	disabled := newMaintenancePackage(orgID, ledgerID, route)
	disabled.Enable = ptr(false)
	_, err = repo.Create(ctx, disabled)
	require.NoError(t, err)

	// Enabled package on a different route -> must NOT match.
	otherRoute := newMaintenancePackage(orgID, ledgerID, "route-other")
	_, err = repo.Create(ctx, otherRoute)
	require.NoError(t, err)

	results, err := repo.FindMatchingPackages(ctx, orgID, ledgerID, route)
	require.NoError(t, err)
	require.Len(t, results, 1, "only the enabled package on the matching route is returned")
	assert.Equal(t, match.ID, results[0].ID)
}

func TestIntegration_BillingPackageRepo_FindActiveByType(t *testing.T) {
	container := mongotestutil.SetupContainer(t)
	repo := newRepository(t, container)
	ctx := context.Background()

	orgID := uuid.New().String()
	ledgerID := uuid.New().String()

	// Two enabled volume packages.
	v1 := newVolumePackage(orgID, ledgerID)
	v2 := newVolumePackage(orgID, ledgerID)
	_, err := repo.Create(ctx, v1)
	require.NoError(t, err)
	_, err = repo.Create(ctx, v2)
	require.NoError(t, err)

	// One disabled volume package -> excluded.
	disabled := newVolumePackage(orgID, ledgerID)
	disabled.Enable = ptr(false)
	_, err = repo.Create(ctx, disabled)
	require.NoError(t, err)

	// One enabled maintenance package -> excluded by type.
	_, err = repo.Create(ctx, newMaintenancePackage(orgID, ledgerID, "route-x"))
	require.NoError(t, err)

	results, err := repo.FindActiveByType(ctx, orgID, ledgerID, model.BillingPackageTypeVolume)
	require.NoError(t, err)
	require.Len(t, results, 2, "only enabled packages of the requested type are returned")

	ids := map[string]bool{results[0].ID: true, results[1].ID: true}
	assert.True(t, ids[v1.ID])
	assert.True(t, ids[v2.ID])
}

// ============================================================================
// Update Tests
// ============================================================================

func TestIntegration_BillingPackageRepo_Update_PersistsChange(t *testing.T) {
	container := mongotestutil.SetupContainer(t)
	repo := newRepository(t, container)
	ctx := context.Background()

	orgID := uuid.New().String()
	bp := newVolumePackage(orgID, uuid.New().String())
	_, err := repo.Create(ctx, bp)
	require.NoError(t, err)

	update := &bson.M{"$set": bson.M{"label": "Updated Label", "enable": false}}
	updated, err := repo.Update(ctx, bp.ID, orgID, update)
	require.NoError(t, err)
	require.NotNil(t, updated, "Update must return the persisted entity")
	assert.Equal(t, "Updated Label", updated.Label, "returned entity reflects the label change")
	require.NotNil(t, updated.Enable)
	assert.False(t, *updated.Enable, "returned entity reflects the enable change")

	got, err := repo.FindByID(ctx, bp.ID, orgID)
	require.NoError(t, err)
	assert.Equal(t, "Updated Label", got.Label, "label change must be persisted")
	require.NotNil(t, got.Enable)
	assert.False(t, *got.Enable, "enable change must be persisted")
}

func TestIntegration_BillingPackageRepo_Update_NotFound(t *testing.T) {
	container := mongotestutil.SetupContainer(t)
	repo := newRepository(t, container)

	update := &bson.M{"$set": bson.M{"label": "x"}}
	got, err := repo.Update(context.Background(), uuid.New().String(), uuid.New().String(), update)

	require.Nil(t, got, "no entity is returned when the document is missing")
	require.Error(t, err, "updating a missing document must report not found")
	var notFound pkg.EntityNotFoundError
	require.ErrorAs(t, err, &notFound, "should map to EntityNotFoundError")
	assert.Equal(t, "0007", notFound.Code)
}

// ============================================================================
// SoftDelete Tests
// ============================================================================

func TestIntegration_BillingPackageRepo_SoftDelete_SetsDeletedAt(t *testing.T) {
	container := mongotestutil.SetupContainer(t)
	repo := newRepository(t, container)
	ctx := context.Background()

	orgID := uuid.New().String()
	bp := newVolumePackage(orgID, uuid.New().String())
	_, err := repo.Create(ctx, bp)
	require.NoError(t, err)

	require.NoError(t, repo.SoftDelete(ctx, bp.ID, orgID))

	// Document remains physically present but carries deleted_at.
	var raw bson.M
	err = collection(container).FindOne(ctx, bson.M{"_id": bp.ID}).Decode(&raw)
	require.NoError(t, err, "soft delete must keep the document")
	assert.NotNil(t, raw["deleted_at"], "deleted_at must be set on soft delete")
	assert.NotEmpty(t, raw["deleted_at"])
}

func TestIntegration_BillingPackageRepo_SoftDelete_NotFound(t *testing.T) {
	container := mongotestutil.SetupContainer(t)
	repo := newRepository(t, container)

	err := repo.SoftDelete(context.Background(), uuid.New().String(), uuid.New().String())

	require.Error(t, err)
	var notFound pkg.EntityNotFoundError
	require.ErrorAs(t, err, &notFound, "should map to EntityNotFoundError")
	assert.Equal(t, "0007", notFound.Code)
}

func TestIntegration_BillingPackageRepo_SoftDelete_Idempotency(t *testing.T) {
	container := mongotestutil.SetupContainer(t)
	repo := newRepository(t, container)
	ctx := context.Background()

	orgID := uuid.New().String()
	bp := newVolumePackage(orgID, uuid.New().String())
	_, err := repo.Create(ctx, bp)
	require.NoError(t, err)

	require.NoError(t, repo.SoftDelete(ctx, bp.ID, orgID))

	// Second delete must fail: the deleted_at filter excludes the already-deleted doc.
	err = repo.SoftDelete(ctx, bp.ID, orgID)
	require.Error(t, err, "deleting an already-deleted package must report not found")
	var notFound pkg.EntityNotFoundError
	require.ErrorAs(t, err, &notFound)
}
