// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package services

import (
	"context"
	"testing"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/mongodb/fees/pack"
	feeshared "github.com/LerianStudio/midaz/v4/components/ledger/pkg/feeshared"
	"github.com/LerianStudio/midaz/v4/components/ledger/pkg/feeshared/model"

	transaction "github.com/LerianStudio/midaz/v4/pkg/mtransaction"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
)

// segScopingFeeInput builds a single-source JSON transfer of value 1000 from
// srcAlias for the single-package scoping tests.
func segScopingFeeInput(ledgerID uuid.UUID, srcAlias string) *model.FeeCalculate {
	return &model.FeeCalculate{
		SegmentID: nil,
		LedgerID:  ledgerID,
		Transaction: transaction.Transaction{
			Send: transaction.Send{
				Asset: "BRL",
				Value: decimal.NewFromInt(1000),
				Source: transaction.Source{
					From: []transaction.FromTo{{
						AccountAlias: srcAlias,
						Amount:       &transaction.Amount{Asset: "BRL", Value: decimal.NewFromInt(1000)},
					}},
				},
				Distribute: transaction.Distribute{
					To: []transaction.FromTo{{
						AccountAlias: "@dst",
						Amount:       &transaction.Amount{Asset: "BRL", Value: decimal.NewFromInt(1000)},
					}},
				},
			},
		},
	}
}

// segScopingFlatPackage builds an enabled flat-100 package with optional segment
// scope. A nil segmentID leaves the package unscoped.
func segScopingFlatPackage(packID uuid.UUID, segmentID *uuid.UUID) *pack.Package {
	enableFlag := false

	return &pack.Package{
		ID:            packID,
		SegmentID:     segmentID,
		MinimumAmount: decimal.NewFromInt(100),
		MaximumAmount: decimal.NewFromInt(2000),
		Fees: map[string]model.Fee{
			"test": {
				FeeLabel: "TestFee",
				CalculationModel: &model.CalculationModel{
					ApplicationRule: "flatFee",
					Calculations:    []model.Calculation{{Type: "flat", Value: "100"}},
				},
				ReferenceAmount:  "originalAmount",
				Priority:         1,
				IsDeductibleFrom: &enableFlag,
				CreditAccount:    "@fee_account",
			},
		},
		WaivedAccounts: &[]string{},
	}
}

// TestCalculateFee_SinglePackage_UnscopedStillApplied is the regression guard for
// fix B: a single UNSCOPED package (nil segment, nil route) must still be selected
// and applied exactly as before, even though the single-package path now runs
// through FindPackageToCalculateFee. The resolver reports the source as
// unsegmented, which leaves cf.SegmentID nil — the unscoped package survives.
func TestCalculateFee_SinglePackage_UnscopedStillApplied(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockPackRepo := pack.NewMockRepository(ctrl)
	mockResolver := feeshared.NewMockMidazResolver(ctrl)
	orgID := uuid.New()
	ledgerID := uuid.New()
	packID := uuid.New()

	feeSvc := &UseCase{packageRepo: mockPackRepo, resolver: mockResolver}

	feeInput := segScopingFeeInput(ledgerID, "@src")

	mockPackRepo.EXPECT().
		FindByOrganizationIDAndLedgerID(gomock.Any(), orgID, ledgerID).
		Return([]*pack.Package{segScopingFlatPackage(packID, nil)}, nil)

	// Source resolves to an unsegmented account.
	mockResolver.EXPECT().
		GetAccountByAlias(gomock.Any(), orgID, ledgerID, "@src").
		Return(&feeshared.Account{ID: "acc", Alias: "@src", SegmentID: nil}, nil)

	ctx := context.Background()
	err := feeSvc.CalculateFee(ctx, feeInput, orgID)
	assert.NoError(t, err)

	// Flat 100 applied: value moved above the original 1000.
	assert.Greater(t, feeInput.Transaction.Send.Value.IntPart(), int64(1000), "unscoped single package must still apply")
	assert.Equal(t, packID.String(), feeInput.Transaction.Metadata["packageAppliedID"])
}

// TestCalculateFee_SinglePackage_SegmentScoped_Matches proves fix A+B+C together
// on the single-package path: a sole segment-scoped package is applied when the
// resolved source segment matches the package's segment.
func TestCalculateFee_SinglePackage_SegmentScoped_Matches(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockPackRepo := pack.NewMockRepository(ctrl)
	mockResolver := feeshared.NewMockMidazResolver(ctrl)
	orgID := uuid.New()
	ledgerID := uuid.New()
	packID := uuid.New()
	segA := uuid.New()

	feeSvc := &UseCase{packageRepo: mockPackRepo, resolver: mockResolver}

	feeInput := segScopingFeeInput(ledgerID, "@seg_src")

	mockPackRepo.EXPECT().
		FindByOrganizationIDAndLedgerID(gomock.Any(), orgID, ledgerID).
		Return([]*pack.Package{segScopingFlatPackage(packID, &segA)}, nil)

	mockResolver.EXPECT().
		GetAccountByAlias(gomock.Any(), orgID, ledgerID, "@seg_src").
		Return(&feeshared.Account{ID: "acc", Alias: "@seg_src", SegmentID: &segA}, nil)

	ctx := context.Background()
	err := feeSvc.CalculateFee(ctx, feeInput, orgID)
	assert.NoError(t, err)

	assert.Greater(t, feeInput.Transaction.Send.Value.IntPart(), int64(1000), "segment-scoped package must apply when source is in the segment")
	assert.Equal(t, packID.String(), feeInput.Transaction.Metadata["packageAppliedID"])
}

// TestCalculateFee_SinglePackage_SegmentScoped_DroppedForUnsegmentedSource
// proves the core defect is fixed: a sole segment-scoped package is NOT applied
// to a source whose resolved segment is nil. Previously the single-package fast
// path skipped scope filtering and charged the fee regardless.
func TestCalculateFee_SinglePackage_SegmentScoped_DroppedForUnsegmentedSource(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockPackRepo := pack.NewMockRepository(ctrl)
	mockResolver := feeshared.NewMockMidazResolver(ctrl)
	orgID := uuid.New()
	ledgerID := uuid.New()
	packID := uuid.New()
	segA := uuid.New()

	feeSvc := &UseCase{packageRepo: mockPackRepo, resolver: mockResolver}

	feeInput := segScopingFeeInput(ledgerID, "@plain_src")

	mockPackRepo.EXPECT().
		FindByOrganizationIDAndLedgerID(gomock.Any(), orgID, ledgerID).
		Return([]*pack.Package{segScopingFlatPackage(packID, &segA)}, nil)

	// Unsegmented source: must NOT match the segment-A package.
	mockResolver.EXPECT().
		GetAccountByAlias(gomock.Any(), orgID, ledgerID, "@plain_src").
		Return(&feeshared.Account{ID: "acc", Alias: "@plain_src", SegmentID: nil}, nil)

	ctx := context.Background()
	err := feeSvc.CalculateFee(ctx, feeInput, orgID)
	assert.NoError(t, err)

	// No fee applied: value unchanged, no package metadata.
	assert.True(t, feeInput.Transaction.Send.Value.Equal(decimal.NewFromInt(1000)), "segment-scoped package must NOT apply to an unsegmented source; value = %s", feeInput.Transaction.Send.Value)
	assert.Nil(t, feeInput.Transaction.Metadata["packageAppliedID"])
}

// TestCalculateFee_SinglePackage_SegmentScoped_DroppedForDifferentSegment proves
// a sole segment-scoped package is NOT applied to a source in a DIFFERENT segment.
func TestCalculateFee_SinglePackage_SegmentScoped_DroppedForDifferentSegment(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockPackRepo := pack.NewMockRepository(ctrl)
	mockResolver := feeshared.NewMockMidazResolver(ctrl)
	orgID := uuid.New()
	ledgerID := uuid.New()
	packID := uuid.New()
	segA := uuid.New()
	segB := uuid.New()

	feeSvc := &UseCase{packageRepo: mockPackRepo, resolver: mockResolver}

	feeInput := segScopingFeeInput(ledgerID, "@seg_b_src")

	mockPackRepo.EXPECT().
		FindByOrganizationIDAndLedgerID(gomock.Any(), orgID, ledgerID).
		Return([]*pack.Package{segScopingFlatPackage(packID, &segA)}, nil)

	mockResolver.EXPECT().
		GetAccountByAlias(gomock.Any(), orgID, ledgerID, "@seg_b_src").
		Return(&feeshared.Account{ID: "acc", Alias: "@seg_b_src", SegmentID: &segB}, nil)

	ctx := context.Background()
	err := feeSvc.CalculateFee(ctx, feeInput, orgID)
	assert.NoError(t, err)

	assert.True(t, feeInput.Transaction.Send.Value.Equal(decimal.NewFromInt(1000)), "segment-A package must NOT apply to a segment-B source; value = %s", feeInput.Transaction.Send.Value)
	assert.Nil(t, feeInput.Transaction.Metadata["packageAppliedID"])
}
