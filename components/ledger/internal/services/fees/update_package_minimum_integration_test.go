//go:build integration

// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package services

import (
	"context"
	"testing"
	"time"

	feesmongo "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/mongodb/fees"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/mongodb/fees/pack"
	"github.com/LerianStudio/midaz/v4/components/ledger/pkg/feeshared/model"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	mongotestutil "github.com/LerianStudio/midaz/v4/tests/utils/mongodb"

	libLog "github.com/LerianStudio/lib-observability/v4/log"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"
)

// newLivePackageUseCase wires the fee use case to a real MongoDB, so the update
// path below runs against stored documents rather than a mocked repository.
func newLivePackageUseCase(t *testing.T) (*UseCase, *pack.PackageMongoDBRepository) {
	t.Helper()

	container := mongotestutil.SetupContainer(t)

	conn := &feesmongo.MongoConnection{
		ConnectionStringSource: container.URI,
		Database:               container.DBName,
		Logger:                 &libLog.NopLogger{},
		MaxPoolSize:            1,
		DB:                     container.Client,
	}

	repo, err := pack.NewPackageMongoDBRepository(conn, &libLog.NopLogger{})
	require.NoError(t, err)

	return &UseCase{packageRepo: repo}, repo
}

// A package stored with a deductible flat fee of 25 keeps both its minimum and its
// fee when an operator tries to drop the minimum below that fee: the update is
// refused and the document is left exactly as it was.
func TestIntegration_UpdatePackage_LoweredMinimumLeavesTheStoredPackageUntouched(t *testing.T) {
	svc, repo := newLivePackageUseCase(t)
	ctx := context.Background()

	orgID := uuid.New()
	ledgerID := uuid.New()
	fixedTime := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	stored := &pack.Package{
		ID:             uuid.New(),
		FeeGroupLabel:  "Pacote Padrao",
		LedgerID:       ledgerID,
		MinimumAmount:  decimal.RequireFromString("100"),
		MaximumAmount:  decimal.RequireFromString("1000"),
		Fees:           storedDeductibleFlatFee("25"),
		Enable:         boolPtr(true),
		WaivedAccounts: &[]string{},
		CreatedAt:      fixedTime,
		UpdatedAt:      fixedTime,
	}

	_, errCreate := repo.Create(ctx, stored, orgID)
	require.NoError(t, errCreate)

	newMinimum := "1"
	err := svc.UpdatePackageByID(ctx, stored.ID, orgID, uuid.Nil, &model.UpdatePackageInput{MinAmount: &newMinimum})

	require.ErrorContains(t, err, constant.ErrCalculationValueFlatFee.Error())

	after, errFind := repo.FindByID(ctx, stored.ID, orgID, uuid.Nil)
	require.NoError(t, errFind)
	require.True(t, after.MinimumAmount.Equal(decimal.RequireFromString("100")),
		"the refused update must leave the stored minimum at 100, got %s", after.MinimumAmount)
	require.Equal(t, fixedTime.UTC(), after.UpdatedAt.UTC(),
		"the refused update must not stamp the document")
	require.Equal(t, "25", after.Fees["fee1"].CalculationModel.Calculations[0].Value,
		"the refused update must leave the stored fee alone")
}

// The same package accepts a minimum it can still charge the stored fee on.
func TestIntegration_UpdatePackage_MinimumAboveTheStoredFeeIsApplied(t *testing.T) {
	svc, repo := newLivePackageUseCase(t)
	ctx := context.Background()

	orgID := uuid.New()
	ledgerID := uuid.New()
	fixedTime := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	stored := &pack.Package{
		ID:             uuid.New(),
		FeeGroupLabel:  "Pacote Padrao",
		LedgerID:       ledgerID,
		MinimumAmount:  decimal.RequireFromString("100"),
		MaximumAmount:  decimal.RequireFromString("1000"),
		Fees:           storedDeductibleFlatFee("25"),
		Enable:         boolPtr(true),
		WaivedAccounts: &[]string{},
		CreatedAt:      fixedTime,
		UpdatedAt:      fixedTime,
	}

	_, errCreate := repo.Create(ctx, stored, orgID)
	require.NoError(t, errCreate)

	newMinimum := "30"
	require.NoError(t, svc.UpdatePackageByID(ctx, stored.ID, orgID, uuid.Nil, &model.UpdatePackageInput{MinAmount: &newMinimum}))

	after, errFind := repo.FindByID(ctx, stored.ID, orgID, uuid.Nil)
	require.NoError(t, errFind)
	require.True(t, after.MinimumAmount.Equal(decimal.RequireFromString("30")),
		"the accepted update must move the stored minimum to 30, got %s", after.MinimumAmount)
}
