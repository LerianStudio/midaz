// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package services

import (
	"context"
	"testing"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/mongodb/fees/pack"
	feeconstant "github.com/LerianStudio/midaz/v4/components/ledger/pkg/feeshared/constant"
	"github.com/LerianStudio/midaz/v4/components/ledger/pkg/feeshared/model"
	"github.com/LerianStudio/midaz/v4/pkg"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	pkgStreaming "github.com/LerianStudio/midaz/v4/pkg/streaming"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.uber.org/mock/gomock"
)

// The by-ID fee-package use cases take the ledger the caller is acting within.
// These tests pin both halves of that contract: a named ledger reaches the
// repository unchanged, and a package the named ledger does not own is reported
// absent — never as a forbidden or otherwise existing resource.

func TestGetPackageByID_PassesCallerLedgerToRepository(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockPackRepo := pack.NewMockRepository(ctrl)

	packID := uuid.New()
	orgID := uuid.New()
	ledgerID := uuid.New()

	mockPackRepo.EXPECT().
		FindByID(gomock.Any(), gomock.Eq(packID), gomock.Eq(orgID), gomock.Eq(ledgerID)).
		Return(&pack.Package{ID: packID, LedgerID: ledgerID}, nil)

	svc := &UseCase{packageRepo: mockPackRepo}

	got, err := svc.GetPackageByID(context.Background(), packID, orgID, ledgerID)
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, packID, got.ID)
}

func TestGetPackageByID_CrossLedger_IsNotFound(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockPackRepo := pack.NewMockRepository(ctrl)

	packID := uuid.New()
	orgID := uuid.New()
	ledgerB := uuid.New()

	// The repository answers a cross-ledger by-ID read exactly as it answers a
	// nonexistent id.
	mockPackRepo.EXPECT().
		FindByID(gomock.Any(), gomock.Eq(packID), gomock.Eq(orgID), gomock.Eq(ledgerB)).
		Return(nil, mongo.ErrNoDocuments)

	svc := &UseCase{packageRepo: mockPackRepo}

	got, err := svc.GetPackageByID(context.Background(), packID, orgID, ledgerB)
	require.Error(t, err)
	assert.Nil(t, got)

	assertPackageAbsent(t, err, constant.EntityPackage)
}

func TestUpdatePackageByID_CrossLedger_IsNotFoundAndDoesNotWrite(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockPackRepo := pack.NewMockRepository(ctrl)
	mockEmitter := pkgStreaming.NewMockEmitter()

	packID := uuid.New()
	orgID := uuid.New()
	ledgerA := uuid.New()
	ledgerB := uuid.New()

	mockPackRepo.EXPECT().
		FindFeesAndAmountDataByPackageID(gomock.Any(), gomock.Eq(orgID), gomock.Eq(packID)).
		Return(&model.AmountData{
			MinAmount: decimal.NewFromInt(100),
			MaxAmount: decimal.NewFromInt(1000),
			Fees:      map[string]model.Fee{},
			LedgerID:  ledgerA,
		}, nil)

	// No Update call is registered: a write reaching the repository at all would
	// fail this test on the unexpected-call check.

	svc := &UseCase{packageRepo: mockPackRepo, Streaming: mockEmitter}

	err := svc.UpdatePackageByID(context.Background(), packID, orgID, ledgerB,
		&model.UpdatePackageInput{FeeGroupLabel: "Renamed Through Ledger B"})
	require.Error(t, err)

	assertPackageAbsent(t, err, "")
	assert.Empty(t, mockEmitter.Events(), "a rejected cross-ledger update must not emit an updated event")
}

// TestUpdatePackageByID_CrossLedgerIsIndistinguishableFromMissingID compares the two
// 404 answers of the ledger-scoped PATCH byte for byte.
//
// They share a status, a code, a title and a detail, so a difference anywhere else in
// the envelope is still a difference a caller can read: it turns the endpoint into an
// oracle for which package ids exist under a ledger the caller cannot reach. Asserting
// the errors are equal — rather than that each is "a not found" — is what makes the
// two answers actually the same answer.
func TestUpdatePackageByID_CrossLedgerIsIndistinguishableFromMissingID(t *testing.T) {
	t.Parallel()

	packID := uuid.New()
	orgID := uuid.New()
	ledgerA := uuid.New()
	ledgerB := uuid.New()

	update := func(t *testing.T, amountData *model.AmountData, findErr error) error {
		t.Helper()

		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockPackRepo := pack.NewMockRepository(ctrl)
		mockPackRepo.EXPECT().
			FindFeesAndAmountDataByPackageID(gomock.Any(), gomock.Eq(orgID), gomock.Eq(packID)).
			Return(amountData, findErr)

		svc := &UseCase{packageRepo: mockPackRepo, Streaming: pkgStreaming.NewMockEmitter()}

		err := svc.UpdatePackageByID(context.Background(), packID, orgID, ledgerB,
			&model.UpdatePackageInput{FeeGroupLabel: "Renamed"})
		require.Error(t, err)

		return err
	}

	// The id exists on another ledger of the same organization.
	crossLedger := update(t, &model.AmountData{
		MinAmount: decimal.NewFromInt(100),
		MaxAmount: decimal.NewFromInt(1000),
		Fees:      map[string]model.Fee{},
		LedgerID:  ledgerA,
	}, nil)

	// The id exists nowhere. The lookup that resolves the owning ledger carries no
	// ledger clause, so this is the answer it produces for an unknown id.
	missing := update(t, nil, pkg.ValidateBusinessError(constant.ErrEntityNotFound, "", "Package"))

	assert.Equal(t, missing, crossLedger,
		"MONEY-PATH: a cross-ledger denial must be the same answer as a missing record, not merely the same status")
}

func TestUpdatePackageByID_OwningLedger_PassesLedgerToRepository(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockPackRepo := pack.NewMockRepository(ctrl)

	packID := uuid.New()
	orgID := uuid.New()
	ledgerID := uuid.New()

	mockPackRepo.EXPECT().
		FindFeesAndAmountDataByPackageID(gomock.Any(), gomock.Eq(orgID), gomock.Eq(packID)).
		Return(&model.AmountData{
			MinAmount: decimal.NewFromInt(100),
			MaxAmount: decimal.NewFromInt(1000),
			Fees:      map[string]model.Fee{},
			LedgerID:  ledgerID,
		}, nil)

	mockPackRepo.EXPECT().
		Update(gomock.Any(), gomock.Eq(packID), gomock.Eq(orgID), gomock.Eq(ledgerID), gomock.Any()).
		Return(&pack.Package{ID: packID, LedgerID: ledgerID}, nil)

	svc := &UseCase{packageRepo: mockPackRepo}

	require.NoError(t, svc.UpdatePackageByID(context.Background(), packID, orgID, ledgerID,
		&model.UpdatePackageInput{FeeGroupLabel: "Renamed Through Its Own Ledger"}))
}

func TestDeletePackageByID_PassesCallerLedgerToRepository(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockPackRepo := pack.NewMockRepository(ctrl)

	packID := uuid.New()
	orgID := uuid.New()
	ledgerID := uuid.New()

	mockPackRepo.EXPECT().
		FindByID(gomock.Any(), gomock.Eq(packID), gomock.Eq(orgID), gomock.Eq(ledgerID)).
		Return(&pack.Package{ID: packID, LedgerID: ledgerID}, nil)
	mockPackRepo.EXPECT().
		SoftDelete(gomock.Any(), gomock.Eq(packID), gomock.Eq(orgID), gomock.Eq(ledgerID)).
		Return(nil)

	svc := &UseCase{packageRepo: mockPackRepo}

	require.NoError(t, svc.DeletePackageByID(context.Background(), packID, orgID, ledgerID))
}

func TestDeletePackageByID_CrossLedger_IsNotFoundAndDoesNotEmit(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockPackRepo := pack.NewMockRepository(ctrl)
	mockEmitter := pkgStreaming.NewMockEmitter()

	packID := uuid.New()
	orgID := uuid.New()
	ledgerB := uuid.New()

	// The repository's own not-found carries no entity type; the delete passes it
	// through untouched, so the fixture must be the shape the repository returns.
	notFound := pkg.ValidateBusinessError(constant.ErrEntityNotFound, "", feeconstant.PackageCollection)

	mockPackRepo.EXPECT().
		FindByID(gomock.Any(), gomock.Eq(packID), gomock.Eq(orgID), gomock.Eq(ledgerB)).
		Return(nil, mongo.ErrNoDocuments)
	mockPackRepo.EXPECT().
		SoftDelete(gomock.Any(), gomock.Eq(packID), gomock.Eq(orgID), gomock.Eq(ledgerB)).
		Return(notFound)

	svc := &UseCase{packageRepo: mockPackRepo, Streaming: mockEmitter}

	err := svc.DeletePackageByID(context.Background(), packID, orgID, ledgerB)
	require.Error(t, err)

	assertPackageAbsent(t, err, "")
	assert.Empty(t, mockEmitter.Events(), "a rejected cross-ledger delete must not emit a deleted event")
}

// TestEstimateFeeCalculation_ResolvesPackageOrganizationWide pins that the
// estimate keeps addressing the package by id alone: the ledger it carries
// selects the accounts the calculation reads, not which package answers.
func TestEstimateFeeCalculation_ResolvesPackageOrganizationWide(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockPackRepo := pack.NewMockRepository(ctrl)

	packID := uuid.New()
	orgID := uuid.New()

	mockPackRepo.EXPECT().
		FindByID(gomock.Any(), gomock.Eq(packID), gomock.Eq(orgID), gomock.Eq(uuid.Nil)).
		Return(nil, mongo.ErrNoDocuments)

	svc := &UseCase{packageRepo: mockPackRepo}

	_, err := svc.EstimateFeeCalculation(context.Background(), &model.FeeEstimate{
		PackageID: packID,
	}, orgID, uuid.New())
	require.Error(t, err)

	assertPackageAbsent(t, err, constant.EntityPackage)
}

// assertPackageAbsent asserts an error reports the package as absent — the same
// answer a nonexistent id produces — and reveals nothing about another ledger.
//
// wantEntityType is the entity type the operation's organization-wide miss carries.
// It is asserted because it is a rendered field of the error envelope: two 404s that
// agree on status, code and message but disagree on it are still distinguishable
// bytes, which is enough to enumerate which ids exist under another ledger.
func assertPackageAbsent(t *testing.T, err error, wantEntityType string) {
	t.Helper()

	var notFound pkg.EntityNotFoundError
	require.ErrorAs(t, err, &notFound, "a cross-ledger by-ID access must map to not found, never to forbidden")
	assert.Equal(t, "0007", notFound.Code)
	assert.Equal(t, wantEntityType, notFound.EntityType,
		"the cross-ledger answer must carry the same entity type the organization-wide miss carries")

	assert.NotContains(t, err.Error(), "ledger")
	assert.NotContains(t, err.Error(), "Ledger")
}
