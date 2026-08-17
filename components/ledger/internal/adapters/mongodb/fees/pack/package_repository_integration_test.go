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

func TestIntegration_PackRepo_CreateAndFind_RoundTripsFeeOperationRouteIDs(t *testing.T) {
	container := mongotestutil.SetupContainer(t)
	repo := newPackRepository(t, container)
	ctx := context.Background()

	orgID := uuid.New()
	pkgEntity := newTestPackage(uuid.New())
	legacyFrom := uuid.NewString()
	legacyTo := uuid.NewString()
	operationFrom := uuid.NewString()
	operationTo := uuid.NewString()
	fee := pkgEntity.Fees["adminFee"]
	fee.RouteFrom = &legacyFrom
	fee.RouteTo = &legacyTo
	fee.OperationRouteFromID = &operationFrom
	fee.OperationRouteToID = &operationTo
	pkgEntity.Fees["adminFee"] = fee

	created, err := repo.Create(ctx, pkgEntity, orgID)
	require.NoError(t, err)
	require.NotNil(t, created)

	found, err := repo.FindByID(ctx, pkgEntity.ID, orgID, uuid.Nil)
	require.NoError(t, err)
	roundTrip := found.Fees["adminFee"]
	assert.Equal(t, legacyFrom, *roundTrip.RouteFrom)
	assert.Equal(t, legacyTo, *roundTrip.RouteTo)
	assert.Equal(t, operationFrom, *roundTrip.OperationRouteFromID)
	assert.Equal(t, operationTo, *roundTrip.OperationRouteToID)

	var stored struct {
		Fees map[string]Fee `bson:"fees"`
	}
	require.NoError(t, packCollection(container).FindOne(ctx, bson.M{"_id": pkgEntity.ID}).Decode(&stored))
	assert.Equal(t, operationFrom, *stored.Fees["adminFee"].OperationRouteFromID)
	assert.Equal(t, operationTo, *stored.Fees["adminFee"].OperationRouteToID)
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

	result, err := repo.FindByID(ctx, pkgEntity.ID, orgID, uuid.Nil)
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, pkgEntity.ID, result.ID)
	assert.Equal(t, pkgEntity.FeeGroupLabel, result.FeeGroupLabel)
	assert.Equal(t, pkgEntity.LedgerID, result.LedgerID)
}

func TestIntegration_PackRepo_FindByID_NotFound(t *testing.T) {
	container := mongotestutil.SetupContainer(t)
	repo := newPackRepository(t, container)

	result, err := repo.FindByID(context.Background(), uuid.New(), uuid.New(), uuid.Nil)
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

	result, err := repo.FindByID(ctx, pkgEntity.ID, uuid.New(), uuid.Nil)
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

	require.NoError(t, repo.SoftDelete(ctx, pkgEntity.ID, orgID, uuid.Nil))

	result, err := repo.FindByID(ctx, pkgEntity.ID, orgID, uuid.Nil)
	require.Error(t, err, "soft-deleted package must not be returned")
	assert.Nil(t, result)
	assert.ErrorIs(t, err, mongo.ErrNoDocuments)
}

func TestIntegration_PackRepo_FindByID_WrongLedgerIsolation(t *testing.T) {
	container := mongotestutil.SetupContainer(t)
	repo := newPackRepository(t, container)
	ctx := context.Background()

	orgID := uuid.New()
	ledgerA := uuid.New()
	ledgerB := uuid.New()

	pkgEntity := newTestPackage(ledgerA)
	_, err := repo.Create(ctx, pkgEntity, orgID)
	require.NoError(t, err)

	result, err := repo.FindByID(ctx, pkgEntity.ID, orgID, ledgerB)
	require.Error(t, err, "a package on ledger A must not be readable through ledger B")
	assert.Nil(t, result)
	assert.ErrorIs(t, err, mongo.ErrNoDocuments,
		"the cross-ledger read must report the same absence a nonexistent id produces")

	// Positive controls: without them the assertion above would also hold for a
	// filter that matches nothing at all.
	owned, err := repo.FindByID(ctx, pkgEntity.ID, orgID, ledgerA)
	require.NoError(t, err, "the owning ledger must still read its own package")
	require.NotNil(t, owned)
	assert.Equal(t, pkgEntity.ID, owned.ID)

	orgScoped, err := repo.FindByID(ctx, pkgEntity.ID, orgID, uuid.Nil)
	require.NoError(t, err, "organization scope must keep reaching the package on any ledger")
	require.NotNil(t, orgScoped)
	assert.Equal(t, pkgEntity.ID, orgScoped.ID)
}

func TestIntegration_PackRepo_Update_WrongLedgerIsolation(t *testing.T) {
	container := mongotestutil.SetupContainer(t)
	repo := newPackRepository(t, container)
	ctx := context.Background()

	orgID := uuid.New()
	ledgerA := uuid.New()
	ledgerB := uuid.New()

	pkgEntity := newTestPackage(ledgerA)
	_, err := repo.Create(ctx, pkgEntity, orgID)
	require.NoError(t, err)

	update := &bson.M{"$set": bson.M{"fee_group_label": "Written Through Ledger B"}}
	returned, errUpdate := repo.Update(ctx, pkgEntity.ID, orgID, ledgerB, update)
	require.Error(t, errUpdate, "a package on ledger A must not be writable through ledger B")
	assert.Nil(t, returned)

	var notFound pkg.EntityNotFoundError
	require.ErrorAs(t, errUpdate, &notFound)
	assert.Equal(t, "0007", notFound.Code)

	var raw bson.M
	require.NoError(t, packCollection(container).FindOne(ctx, bson.M{"_id": pkgEntity.ID}).Decode(&raw))
	assert.Equal(t, pkgEntity.FeeGroupLabel, raw["fee_group_label"],
		"the cross-ledger update must not have touched the stored document")

	// Positive control: the same update through the owning ledger lands.
	ownedUpdate := &bson.M{"$set": bson.M{"fee_group_label": "Written Through Ledger A"}}
	owned, errOwned := repo.Update(ctx, pkgEntity.ID, orgID, ledgerA, ownedUpdate)
	require.NoError(t, errOwned, "the owning ledger must still write its own package")
	require.NotNil(t, owned)
	assert.Equal(t, "Written Through Ledger A", owned.FeeGroupLabel)
}

func TestIntegration_PackRepo_SoftDelete_WrongLedgerIsolation(t *testing.T) {
	container := mongotestutil.SetupContainer(t)
	repo := newPackRepository(t, container)
	ctx := context.Background()

	orgID := uuid.New()
	ledgerA := uuid.New()
	ledgerB := uuid.New()

	pkgEntity := newTestPackage(ledgerA)
	_, err := repo.Create(ctx, pkgEntity, orgID)
	require.NoError(t, err)

	errDelete := repo.SoftDelete(ctx, pkgEntity.ID, orgID, ledgerB)
	require.Error(t, errDelete, "a package on ledger A must not be deletable through ledger B")

	var notFound pkg.EntityNotFoundError
	require.ErrorAs(t, errDelete, &notFound)
	assert.Equal(t, "0007", notFound.Code)

	var raw bson.M
	require.NoError(t, packCollection(container).FindOne(ctx, bson.M{"_id": pkgEntity.ID}).Decode(&raw))
	assert.Nil(t, raw["deleted_at"], "the cross-ledger delete must not have soft-deleted the document")

	// Positive control: the owning ledger still deletes.
	require.NoError(t, repo.SoftDelete(ctx, pkgEntity.ID, orgID, ledgerA),
		"the owning ledger must still delete its own package")

	require.NoError(t, packCollection(container).FindOne(ctx, bson.M{"_id": pkgEntity.ID}).Decode(&raw))
	assert.NotNil(t, raw["deleted_at"], "the owning ledger's delete must have soft-deleted the document")
}

// TestIntegration_PackRepo_ByID_OrganizationScopeUnchanged pins the pre-migration
// contract the organization-scoped callers depend on: passing uuid.Nil as the
// ledger reaches a package regardless of which ledger owns it, on all three
// by-ID operations, and still excludes soft-deleted documents.
func TestIntegration_PackRepo_ByID_OrganizationScopeUnchanged(t *testing.T) {
	container := mongotestutil.SetupContainer(t)
	repo := newPackRepository(t, container)
	ctx := context.Background()

	orgID := uuid.New()

	pkgEntity := newTestPackage(uuid.New())
	_, err := repo.Create(ctx, pkgEntity, orgID)
	require.NoError(t, err)

	found, err := repo.FindByID(ctx, pkgEntity.ID, orgID, uuid.Nil)
	require.NoError(t, err)
	require.NotNil(t, found)
	assert.Equal(t, pkgEntity.ID, found.ID)

	updated, err := repo.Update(ctx, pkgEntity.ID, orgID, uuid.Nil,
		&bson.M{"$set": bson.M{"fee_group_label": "Org Scoped Write"}})
	require.NoError(t, err)
	require.NotNil(t, updated)
	assert.Equal(t, "Org Scoped Write", updated.FeeGroupLabel)

	require.NoError(t, repo.SoftDelete(ctx, pkgEntity.ID, orgID, uuid.Nil))

	gone, err := repo.FindByID(ctx, pkgEntity.ID, orgID, uuid.Nil)
	require.Error(t, err, "organization scope must keep excluding soft-deleted documents")
	assert.Nil(t, gone)
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

// TestIntegration_PackRepo_FindList_LedgerFilterHoldsForLeadingZeroIdentifier pins the
// ledger filter against a real identifier whose leading four bytes are zero.
//
// A predicate written as UUID.ID() != 0 reads exactly those four bytes, so such a
// ledger is indistinguishable from "no ledger requested" and the clause is dropped —
// the listing then answers with every ledger of the organization. On the
// organization-scoped surface that silently ignores a caller's filter; on a
// ledger-scoped path it hands back packages the named ledger does not own. The
// identifier is fixed rather than generated so the case is exercised every run.
func TestIntegration_PackRepo_FindList_LedgerFilterHoldsForLeadingZeroIdentifier(t *testing.T) {
	container := mongotestutil.SetupContainer(t)
	repo := newPackRepository(t, container)
	ctx := context.Background()

	orgID := uuid.New()
	leadingZeroLedger := uuid.MustParse("00000000-1111-4111-8111-111111111111")
	otherLedger := uuid.New()

	require.Zero(t, leadingZeroLedger.ID(), "the fixture must be the identifier shape the old predicate misread")
	require.NotEqual(t, uuid.Nil, leadingZeroLedger, "and it must still be a real ledger")

	_, err := repo.Create(ctx, newTestPackage(leadingZeroLedger), orgID)
	require.NoError(t, err)

	_, err = repo.Create(ctx, newTestPackage(otherLedger), orgID)
	require.NoError(t, err)

	results, err := repo.FindList(ctx, feehttp.QueryHeader{
		OrganizationID: orgID,
		LedgerID:       leadingZeroLedger,
		Limit:          10,
		Page:           1,
	})
	require.NoError(t, err)
	require.Len(t, results, 1, "the ledger filter must still apply")
	assert.Equal(t, leadingZeroLedger, results[0].LedgerID)
}

// TestIntegration_PackRepo_FindList_SegmentFilterHoldsForLeadingZeroIdentifier is the
// ledger test above for the OTHER filter the same listing carries. Both are guarded by
// the same shape of predicate, so a fix applied to one and a regression test written
// for one leaves the other free to be reverted unnoticed. A dropped segment clause
// widens the listing to every segment of the ledger, and the package it then returns
// is the one that prices a transaction.
func TestIntegration_PackRepo_FindList_SegmentFilterHoldsForLeadingZeroIdentifier(t *testing.T) {
	container := mongotestutil.SetupContainer(t)
	repo := newPackRepository(t, container)
	ctx := context.Background()

	orgID := uuid.New()
	ledgerID := uuid.New()
	leadingZeroSegment := uuid.MustParse("00000000-2222-4222-8222-222222222222")
	otherSegment := uuid.New()

	require.Zero(t, leadingZeroSegment.ID(), "the fixture must be the identifier shape the old predicate misread")
	require.NotEqual(t, uuid.Nil, leadingZeroSegment, "and it must still be a real segment")

	inSegment := newTestPackage(ledgerID)
	inSegment.SegmentID = &leadingZeroSegment

	_, err := repo.Create(ctx, inSegment, orgID)
	require.NoError(t, err)

	elsewhere := newTestPackage(ledgerID)
	elsewhere.SegmentID = &otherSegment

	_, err = repo.Create(ctx, elsewhere, orgID)
	require.NoError(t, err)

	results, err := repo.FindList(ctx, feehttp.QueryHeader{
		OrganizationID: orgID,
		LedgerID:       ledgerID,
		SegmentID:      leadingZeroSegment,
		Limit:          10,
		Page:           1,
	})
	require.NoError(t, err)
	require.Len(t, results, 1, "the segment filter must still apply")
	require.NotNil(t, results[0].SegmentID)
	assert.Equal(t, leadingZeroSegment, *results[0].SegmentID)
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

	require.NoError(t, repo.SoftDelete(ctx, deleted.ID, orgID, uuid.Nil))

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
	returned, errUpdate := repo.Update(ctx, pkgEntity.ID, orgID, uuid.Nil, update)
	require.NoError(t, errUpdate)
	require.NotNil(t, returned, "Update must return the persisted entity")
	assert.Equal(t, "Updated Package Label", returned.FeeGroupLabel, "returned entity must reflect the change")

	got, err := repo.FindByID(ctx, pkgEntity.ID, orgID, uuid.Nil)
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
	returned, errUpdate := repo.Update(ctx, pkgEntity.ID, orgID, uuid.Nil, update)
	require.NoError(t, errUpdate)
	require.NotNil(t, returned, "Update must return the persisted entity")
	require.NotNil(t, returned.Enable)
	assert.False(t, *returned.Enable, "returned entity must reflect the auto-disable side effect")

	got, err := repo.FindByID(ctx, pkgEntity.ID, orgID, uuid.Nil)
	require.NoError(t, err)
	require.NotNil(t, got.Enable)
	assert.False(t, *got.Enable, "package with no fees must be auto-disabled by Update")
}

func TestIntegration_PackRepo_Update_NotFound(t *testing.T) {
	container := mongotestutil.SetupContainer(t)
	repo := newPackRepository(t, container)

	update := &bson.M{"$set": bson.M{"fee_group_label": "x"}}
	_, err := repo.Update(context.Background(), uuid.New(), uuid.New(), uuid.Nil, update)

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

	require.NoError(t, repo.SoftDelete(ctx, pkgEntity.ID, orgID, uuid.Nil))

	var raw bson.M
	err = packCollection(container).FindOne(ctx, bson.M{"_id": pkgEntity.ID}).Decode(&raw)
	require.NoError(t, err, "soft delete must keep the document")
	assert.NotNil(t, raw["deleted_at"], "deleted_at must be set on soft delete")
}

func TestIntegration_PackRepo_SoftDelete_NotFound(t *testing.T) {
	container := mongotestutil.SetupContainer(t)
	repo := newPackRepository(t, container)

	err := repo.SoftDelete(context.Background(), uuid.New(), uuid.New(), uuid.Nil)
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

	require.NoError(t, repo.SoftDelete(ctx, pkgEntity.ID, orgID, uuid.Nil))

	err = repo.SoftDelete(ctx, pkgEntity.ID, orgID, uuid.Nil)
	require.Error(t, err, "deleting an already-deleted package must report not found")
	var notFound pkg.EntityNotFoundError
	require.ErrorAs(t, err, &notFound)
}
