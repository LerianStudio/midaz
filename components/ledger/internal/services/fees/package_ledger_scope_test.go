// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package services

import (
	"context"
	"testing"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/mongodb/fees/pack"
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

	assertPackageAbsent(t, err)
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

	assertPackageAbsent(t, err)
	assert.Empty(t, mockEmitter.Events(), "a rejected cross-ledger update must not emit an updated event")
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

	notFound := pkg.ValidateBusinessError(constant.ErrEntityNotFound, constant.EntityPackage)

	mockPackRepo.EXPECT().
		FindByID(gomock.Any(), gomock.Eq(packID), gomock.Eq(orgID), gomock.Eq(ledgerB)).
		Return(nil, mongo.ErrNoDocuments)
	mockPackRepo.EXPECT().
		SoftDelete(gomock.Any(), gomock.Eq(packID), gomock.Eq(orgID), gomock.Eq(ledgerB)).
		Return(notFound)

	svc := &UseCase{packageRepo: mockPackRepo, Streaming: mockEmitter}

	err := svc.DeletePackageByID(context.Background(), packID, orgID, ledgerB)
	require.Error(t, err)

	assertPackageAbsent(t, err)
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
		LedgerID:  uuid.New(),
	}, orgID)
	require.Error(t, err)

	assertPackageAbsent(t, err)
}

// assertPackageAbsent asserts an error reports the package as absent — the same
// answer a nonexistent id produces — and reveals nothing about another ledger.
func assertPackageAbsent(t *testing.T, err error) {
	t.Helper()

	var notFound pkg.EntityNotFoundError
	require.ErrorAs(t, err, &notFound, "a cross-ledger by-ID access must map to not found, never to forbidden")
	assert.Equal(t, "0007", notFound.Code)

	assert.NotContains(t, err.Error(), "ledger")
	assert.NotContains(t, err.Error(), "Ledger")
}
