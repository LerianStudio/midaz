// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package services

import (
	"context"
	"testing"

	libObservability "github.com/LerianStudio/lib-observability/v4"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/mongodb/fees/pack"
	mongoPack "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/mongodb/fees/pack"
	feeshared "github.com/LerianStudio/midaz/v4/components/ledger/pkg/feeshared"
	"github.com/LerianStudio/midaz/v4/components/ledger/pkg/feeshared/model"
	"github.com/LerianStudio/midaz/v4/pkg"
	"github.com/LerianStudio/midaz/v4/pkg/constant"

	transaction "github.com/LerianStudio/midaz/v4/pkg/mtransaction"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/codes"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.uber.org/mock/gomock"
)

func TestCreateFee(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockPackRepo := pack.NewMockRepository(ctrl)
	orgId := uuid.New()
	packID := uuid.New()
	ledgerID := uuid.New()
	enableFlag := true
	enableFlagFalse := false

	feeSvc := &UseCase{
		packageRepo: mockPackRepo,
	}

	from := transaction.FromTo{
		Amount: nil,
		Share: &transaction.Share{
			Percentage:             50,
			PercentageOfPercentage: 0,
		},
		Remaining:   "",
		Rate:        nil,
		Description: "",
		Metadata:    nil,
		IsFrom:      true,
	}

	from2 := transaction.FromTo{
		Amount: nil,
		Share: &transaction.Share{
			Percentage:             50,
			PercentageOfPercentage: 0,
		},
		Remaining:   "",
		Rate:        nil,
		Description: "",
		Metadata:    nil,
		IsFrom:      true,
	}

	to := transaction.FromTo{
		Amount: nil,
		Share: &transaction.Share{
			Percentage:             100,
			PercentageOfPercentage: 0,
		},
		Remaining:   "",
		Rate:        nil,
		Description: "",
		Metadata:    nil,
		IsFrom:      false,
	}

	sendValue := transaction.Send{
		Asset: "BRL",
		Value: decimal.NewFromInt(16100),
		Source: transaction.Source{
			Remaining: "",
			From:      append(make([]transaction.FromTo, 0), from, from2),
		},
		Distribute: transaction.Distribute{
			Remaining: "",
			To:        append(make([]transaction.FromTo, 0), to),
		},
	}

	transactionModel := transaction.Transaction{
		Pending:  false,
		Metadata: nil,
		Send:     sendValue,
	}

	createFeeInput := &model.FeeCalculate{
		SegmentID:   nil,
		LedgerID:    ledgerID,
		Transaction: transactionModel,
	}

	fees := make(map[string]model.Fee)
	calculationsIOF := make([]model.Calculation, 0)
	calculationsIOF = append(calculationsIOF, model.Calculation{
		Type:  "percentage",
		Value: "600",
	})

	fees["iof"] = model.Fee{
		FeeLabel: "Testes",
		CalculationModel: &model.CalculationModel{
			ApplicationRule: "percentual",
			Calculations:    calculationsIOF,
		},
		ReferenceAmount:  "originalAmount",
		Priority:         1,
		IsDeductibleFrom: &enableFlag,
		CreditAccount:    "@account",
	}

	calculationsAdmin := make([]model.Calculation, 0)
	calculationsAdmin = append(calculationsAdmin, model.Calculation{
		Type:  "percentage",
		Value: "600",
	})
	calculationsAdmin = append(calculationsAdmin, model.Calculation{
		Type:  "flat",
		Value: "1600",
	})
	fees["taxaAdmin"] = model.Fee{
		FeeLabel: "Testes",
		CalculationModel: &model.CalculationModel{
			ApplicationRule: "maxBetweenTypes",
			Calculations:    calculationsAdmin,
		},
		ReferenceAmount:  "afterFeesAmount",
		Priority:         2,
		IsDeductibleFrom: &enableFlagFalse,
		CreditAccount:    "@account",
	}
	packEntity := &mongoPack.Package{
		ID:             packID,
		FeeGroupLabel:  "teste group label",
		Description:    nil,
		SegmentID:      nil,
		LedgerID:       ledgerID,
		MinimumAmount:  decimal.NewFromInt(100),
		MaximumAmount:  decimal.NewFromInt(200),
		WaivedAccounts: &[]string{"acc01", "acc02"},
		Enable:         &enableFlag,
		Fees:           fees,
	}

	fromResponse1 := transaction.FromTo{
		Amount: &transaction.Amount{
			Asset:     "BRL",
			Value:     decimal.NewFromInt(8050),
			Operation: "",
		},
		Remaining:   "",
		Rate:        nil,
		Description: "",
		Metadata:    nil,
		IsFrom:      true,
	}

	fromResponse2 := transaction.FromTo{
		Amount: &transaction.Amount{
			Asset:     "BRL",
			Value:     decimal.NewFromInt(8050),
			Operation: "",
		},
		Remaining:   "",
		Rate:        nil,
		Description: "",
		Metadata:    nil,
		IsFrom:      true,
	}

	toResponse := transaction.FromTo{
		Amount: &transaction.Amount{
			Asset:     "BRL",
			Value:     decimal.NewFromInt(94),
			Operation: "",
		},
		Remaining:   "",
		Rate:        nil,
		Description: "",
		Metadata:    nil,
		IsFrom:      true,
	}

	toResponseFee := transaction.FromTo{
		Amount: &transaction.Amount{
			Asset:     "BRL",
			Value:     decimal.NewFromInt(16006),
			Operation: "",
		},
		Remaining:   "",
		Rate:        nil,
		Description: "",
		Metadata:    nil,
		IsFrom:      true,
	}

	responseFeeInput := &model.FeeCalculate{
		SegmentID: nil,
		LedgerID:  ledgerID,
		Transaction: transaction.Transaction{
			Pending:  false,
			Metadata: nil,
			Send: transaction.Send{
				Asset: "BRL",
				Value: decimal.NewFromInt(16100),
				Source: transaction.Source{
					Remaining: "",
					From:      append(make([]transaction.FromTo, 0), fromResponse1, fromResponse2),
				},
				Distribute: transaction.Distribute{
					Remaining: "",
					To:        append(make([]transaction.FromTo, 0), toResponse, toResponseFee),
				},
			},
		},
	}

	packList := append(make([]*mongoPack.Package, 0), packEntity)
	tests := []struct {
		name        string
		feeInput    *model.FeeCalculate
		orgId       uuid.UUID
		mockSetup   func()
		expectErr   bool
		errContains string
	}{
		{
			name:     "Success - Create a fee",
			feeInput: createFeeInput,
			orgId:    orgId,
			mockSetup: func() {
				mockPackRepo.EXPECT().
					FindByOrganizationIDAndLedgerID(gomock.Any(), gomock.Any(), gomock.Any()).
					Return(packList, nil)
			},
			expectErr: false,
		},
		{
			name:     "Error - Find package to create fee",
			feeInput: createFeeInput,
			orgId:    orgId,
			mockSetup: func() {
				mockPackRepo.EXPECT().
					FindByOrganizationIDAndLedgerID(gomock.Any(), gomock.Any(), gomock.Any()).
					Return(nil, constant.ErrEntityNotFound)
			},
			expectErr:   true,
			errContains: "0007",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			tt.mockSetup()

			ctx := context.Background()
			err := feeSvc.CalculateFee(ctx, createFeeInput, tt.orgId)

			if tt.expectErr {
				assert.Error(t, err)
				if tt.errContains != "" {
					assert.Contains(t, err.Error(), tt.errContains)
				}
			} else {
				assert.NoError(t, err)
				assert.True(t, createFeeInput.Transaction.Send.Value.Equal(responseFeeInput.Transaction.Send.Value), "expected Send.Value %s, got %s", responseFeeInput.Transaction.Send.Value, createFeeInput.Transaction.Send.Value)
				assert.NotEmpty(t, createFeeInput.Transaction.Send.Source.From, "From should not be empty after CalculateFee")
				assert.NotEmpty(t, createFeeInput.Transaction.Send.Distribute.To, "To should not be empty after CalculateFee")
			}
		})
	}
}

// TestCalculateFee_NoPackagesFound tests when no packages are found
func TestCalculateFee_NoPackagesFound(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockPackRepo := pack.NewMockRepository(ctrl)
	orgID := uuid.New()
	ledgerID := uuid.New()

	feeSvc := &UseCase{
		packageRepo: mockPackRepo,
	}

	feeInput := &model.FeeCalculate{
		SegmentID: nil,
		LedgerID:  ledgerID,
		Transaction: transaction.Transaction{
			Send: transaction.Send{
				Asset: "BRL",
				Value: decimal.NewFromInt(1000),
				Source: transaction.Source{
					From: []transaction.FromTo{{
						Amount: &transaction.Amount{Asset: "BRL", Value: decimal.NewFromInt(1000)},
					}},
				},
				Distribute: transaction.Distribute{
					To: []transaction.FromTo{{
						Amount: &transaction.Amount{Asset: "BRL", Value: decimal.NewFromInt(1000)},
					}},
				},
			},
		},
	}

	mockPackRepo.EXPECT().
		FindByOrganizationIDAndLedgerID(gomock.Any(), orgID, ledgerID).
		Return([]*pack.Package{}, nil)

	ctx := context.Background()
	err := feeSvc.CalculateFee(ctx, feeInput, orgID)
	assert.NoError(t, err)
}

// TestCalculateFee_SinglePackage_Success tests successful calculation with single package
func TestCalculateFee_SinglePackage_Success(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockPackRepo := pack.NewMockRepository(ctrl)
	orgID := uuid.New()
	ledgerID := uuid.New()
	packID := uuid.New()
	enableFlag := false

	feeSvc := &UseCase{
		packageRepo: mockPackRepo,
	}

	from := transaction.FromTo{
		Amount: &transaction.Amount{
			Asset: "BRL",
			Value: decimal.NewFromInt(1000),
		},
	}

	to := transaction.FromTo{
		Amount: &transaction.Amount{
			Asset: "BRL",
			Value: decimal.NewFromInt(1000),
		},
	}

	feeInput := &model.FeeCalculate{
		SegmentID: nil,
		LedgerID:  ledgerID,
		Transaction: transaction.Transaction{
			Send: transaction.Send{
				Asset: "BRL",
				Value: decimal.NewFromInt(1000),
				Source: transaction.Source{
					From: []transaction.FromTo{from},
				},
				Distribute: transaction.Distribute{
					To: []transaction.FromTo{to},
				},
			},
		},
	}

	fee := model.Fee{
		FeeLabel: "TestFee",
		CalculationModel: &model.CalculationModel{
			ApplicationRule: "flatFee",
			Calculations: []model.Calculation{{
				Type:  "flat",
				Value: "100",
			}},
		},
		ReferenceAmount:  "originalAmount",
		Priority:         1,
		IsDeductibleFrom: &enableFlag,
		CreditAccount:    "@fee_account",
	}

	packEntity := &pack.Package{
		ID:             packID,
		MinimumAmount:  decimal.NewFromInt(100),
		MaximumAmount:  decimal.NewFromInt(2000),
		Fees:           map[string]model.Fee{"test": fee},
		WaivedAccounts: &[]string{},
	}

	mockPackRepo.EXPECT().
		FindByOrganizationIDAndLedgerID(gomock.Any(), orgID, ledgerID).
		Return([]*pack.Package{packEntity}, nil)

	ctx := context.Background()
	err := feeSvc.CalculateFee(ctx, feeInput, orgID)
	assert.NoError(t, err)
	assert.Greater(t, feeInput.Transaction.Send.Value.IntPart(), int64(1000))
}

// TestCalculateFee_SinglePackage_CalculateFeeError tests error when calculating fee in single package
func TestCalculateFee_SinglePackage_CalculateFeeError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockPackRepo := pack.NewMockRepository(ctrl)
	orgID := uuid.New()
	ledgerID := uuid.New()
	packID := uuid.New()

	feeSvc := &UseCase{
		packageRepo: mockPackRepo,
	}

	from := transaction.FromTo{
		Amount: &transaction.Amount{
			Asset: "BRL",
			Value: decimal.NewFromInt(1000),
		},
	}

	to := transaction.FromTo{
		Amount: &transaction.Amount{
			Asset: "BRL",
			Value: decimal.NewFromInt(1000),
		},
	}

	feeInput := &model.FeeCalculate{
		SegmentID: nil,
		LedgerID:  ledgerID,
		Transaction: transaction.Transaction{
			Send: transaction.Send{
				Asset: "BRL",
				Value: decimal.NewFromInt(1000),
				Source: transaction.Source{
					From: []transaction.FromTo{from},
				},
				Distribute: transaction.Distribute{
					To: []transaction.FromTo{to},
				},
			},
		},
	}

	fee := model.Fee{
		FeeLabel: "InvalidFee",
		CalculationModel: &model.CalculationModel{
			ApplicationRule: "invalidRule",
			Calculations: []model.Calculation{{
				Type:  "flat",
				Value: "100",
			}},
		},
		ReferenceAmount:  "originalAmount",
		Priority:         1,
		IsDeductibleFrom: func() *bool { b := false; return &b }(),
		CreditAccount:    "@fee_account",
	}

	packEntity := &pack.Package{
		ID:             packID,
		MinimumAmount:  decimal.NewFromInt(100),
		MaximumAmount:  decimal.NewFromInt(2000),
		Fees:           map[string]model.Fee{"test": fee},
		WaivedAccounts: &[]string{},
	}

	mockPackRepo.EXPECT().
		FindByOrganizationIDAndLedgerID(gomock.Any(), orgID, ledgerID).
		Return([]*pack.Package{packEntity}, nil)

	ctx := context.Background()
	err := feeSvc.CalculateFee(ctx, feeInput, orgID)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "0206")
}

// TestCalculateFee_SinglePackage_WithMetadataUpdate tests metadata update when From/To change
func TestCalculateFee_SinglePackage_WithMetadataUpdate(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockPackRepo := pack.NewMockRepository(ctrl)
	orgID := uuid.New()
	ledgerID := uuid.New()
	packID := uuid.New()
	enableFlag := false

	feeSvc := &UseCase{
		packageRepo: mockPackRepo,
	}

	from := transaction.FromTo{
		Amount: &transaction.Amount{
			Asset: "BRL",
			Value: decimal.NewFromInt(1000),
		},
	}

	to := transaction.FromTo{
		Amount: &transaction.Amount{
			Asset: "BRL",
			Value: decimal.NewFromInt(1000),
		},
	}

	feeInput := &model.FeeCalculate{
		SegmentID: nil,
		LedgerID:  ledgerID,
		Transaction: transaction.Transaction{
			Metadata: nil,
			Send: transaction.Send{
				Asset: "BRL",
				Value: decimal.NewFromInt(1000),
				Source: transaction.Source{
					From: []transaction.FromTo{from},
				},
				Distribute: transaction.Distribute{
					To: []transaction.FromTo{to},
				},
			},
		},
	}

	fee := model.Fee{
		FeeLabel: "TestFee",
		CalculationModel: &model.CalculationModel{
			ApplicationRule: "flatFee",
			Calculations: []model.Calculation{{
				Type:  "flat",
				Value: "100",
			}},
		},
		ReferenceAmount:  "originalAmount",
		Priority:         1,
		IsDeductibleFrom: &enableFlag,
		CreditAccount:    "@fee_account",
	}

	packEntity := &pack.Package{
		ID:             packID,
		MinimumAmount:  decimal.NewFromInt(100),
		MaximumAmount:  decimal.NewFromInt(2000),
		Fees:           map[string]model.Fee{"test": fee},
		WaivedAccounts: &[]string{},
	}

	mockPackRepo.EXPECT().
		FindByOrganizationIDAndLedgerID(gomock.Any(), orgID, ledgerID).
		Return([]*pack.Package{packEntity}, nil)

	ctx := context.Background()
	err := feeSvc.CalculateFee(ctx, feeInput, orgID)
	assert.NoError(t, err)
	assert.NotNil(t, feeInput.Transaction.Metadata)
	assert.Equal(t, packID.String(), feeInput.Transaction.Metadata["packageAppliedID"])
}

// TestCalculateFee_MultiplePackages_Success tests successful calculation with multiple packages
func TestCalculateFee_MultiplePackages_Success(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockPackRepo := pack.NewMockRepository(ctrl)
	orgID := uuid.New()
	ledgerID := uuid.New()
	packID1 := uuid.New()
	packID2 := uuid.New()
	enableFlag := false

	feeSvc := &UseCase{
		packageRepo: mockPackRepo,
	}

	route := "debitoted"
	from := transaction.FromTo{
		Amount: &transaction.Amount{
			Asset: "BRL",
			Value: decimal.NewFromInt(500),
		},
	}

	to := transaction.FromTo{
		Amount: &transaction.Amount{
			Asset: "BRL",
			Value: decimal.NewFromInt(500),
		},
	}

	feeInput := &model.FeeCalculate{
		SegmentID: nil,
		LedgerID:  ledgerID,
		Transaction: transaction.Transaction{
			Route: route,
			Send: transaction.Send{
				Asset: "BRL",
				Value: decimal.NewFromInt(500),
				Source: transaction.Source{
					From: []transaction.FromTo{from},
				},
				Distribute: transaction.Distribute{
					To: []transaction.FromTo{to},
				},
			},
		},
	}

	fee := model.Fee{
		FeeLabel: "TestFee",
		CalculationModel: &model.CalculationModel{
			ApplicationRule: "flatFee",
			Calculations: []model.Calculation{{
				Type:  "flat",
				Value: "50",
			}},
		},
		ReferenceAmount:  "originalAmount",
		Priority:         1,
		IsDeductibleFrom: &enableFlag,
		CreditAccount:    "@fee_account",
	}

	packEntity1 := &pack.Package{
		ID:               packID1,
		TransactionRoute: &route,
		MinimumAmount:    decimal.NewFromInt(100),
		MaximumAmount:    decimal.NewFromInt(1000),
		Fees:             map[string]model.Fee{"test": fee},
		WaivedAccounts:   &[]string{},
	}

	packEntity2 := &pack.Package{
		ID:               packID2,
		TransactionRoute: &route,
		MinimumAmount:    decimal.NewFromInt(2000),
		MaximumAmount:    decimal.NewFromInt(5000),
		Fees:             map[string]model.Fee{"test": fee},
		WaivedAccounts:   &[]string{},
	}

	mockPackRepo.EXPECT().
		FindByOrganizationIDAndLedgerID(gomock.Any(), orgID, ledgerID).
		Return([]*pack.Package{packEntity1, packEntity2}, nil)

	ctx := context.Background()
	err := feeSvc.CalculateFee(ctx, feeInput, orgID)
	assert.NoError(t, err)
	assert.Greater(t, feeInput.Transaction.Send.Value.IntPart(), int64(500))
}

// TestCalculateFee_MultiplePackages_CalculateFeeError tests error when calculating fee in multiple packages
func TestCalculateFee_MultiplePackages_CalculateFeeError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockPackRepo := pack.NewMockRepository(ctrl)
	orgID := uuid.New()
	ledgerID := uuid.New()
	packID1 := uuid.New()

	feeSvc := &UseCase{
		packageRepo: mockPackRepo,
	}

	route := "debitoted"
	from := transaction.FromTo{
		Amount: &transaction.Amount{
			Asset: "BRL",
			Value: decimal.NewFromInt(500),
		},
	}

	to := transaction.FromTo{
		Amount: &transaction.Amount{
			Asset: "BRL",
			Value: decimal.NewFromInt(500),
		},
	}

	feeInput := &model.FeeCalculate{
		SegmentID: nil,
		LedgerID:  ledgerID,
		Transaction: transaction.Transaction{
			Route: route,
			Send: transaction.Send{
				Asset: "BRL",
				Value: decimal.NewFromInt(500),
				Source: transaction.Source{
					From: []transaction.FromTo{from},
				},
				Distribute: transaction.Distribute{
					To: []transaction.FromTo{to},
				},
			},
		},
	}

	fee := model.Fee{
		FeeLabel: "InvalidFee",
		CalculationModel: &model.CalculationModel{
			ApplicationRule: "invalidRule",
			Calculations: []model.Calculation{{
				Type:  "flat",
				Value: "50",
			}},
		},
		ReferenceAmount:  "originalAmount",
		Priority:         1,
		IsDeductibleFrom: func() *bool { b := false; return &b }(),
		CreditAccount:    "@fee_account",
	}

	packEntity1 := &pack.Package{
		ID:               packID1,
		TransactionRoute: &route,
		MinimumAmount:    decimal.NewFromInt(100),
		MaximumAmount:    decimal.NewFromInt(1000),
		Fees:             map[string]model.Fee{"test": fee},
		WaivedAccounts:   &[]string{},
	}

	mockPackRepo.EXPECT().
		FindByOrganizationIDAndLedgerID(gomock.Any(), orgID, ledgerID).
		Return([]*pack.Package{packEntity1}, nil)

	ctx := context.Background()
	err := feeSvc.CalculateFee(ctx, feeInput, orgID)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "0206")
}

// TestCalculateFee_SinglePackage_ValueAtMinimum tests value at minimum limit
func TestCalculateFee_SinglePackage_ValueAtMinimum(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockPackRepo := pack.NewMockRepository(ctrl)
	orgID := uuid.New()
	ledgerID := uuid.New()
	packID := uuid.New()
	enableFlag := false

	feeSvc := &UseCase{
		packageRepo: mockPackRepo,
	}

	from := transaction.FromTo{
		Amount: &transaction.Amount{
			Asset: "BRL",
			Value: decimal.NewFromInt(100),
		},
	}

	to := transaction.FromTo{
		Amount: &transaction.Amount{
			Asset: "BRL",
			Value: decimal.NewFromInt(100),
		},
	}

	feeInput := &model.FeeCalculate{
		SegmentID: nil,
		LedgerID:  ledgerID,
		Transaction: transaction.Transaction{
			Send: transaction.Send{
				Asset: "BRL",
				Value: decimal.NewFromInt(100),
				Source: transaction.Source{
					From: []transaction.FromTo{from},
				},
				Distribute: transaction.Distribute{
					To: []transaction.FromTo{to},
				},
			},
		},
	}

	fee := model.Fee{
		FeeLabel: "TestFee",
		CalculationModel: &model.CalculationModel{
			ApplicationRule: "flatFee",
			Calculations: []model.Calculation{{
				Type:  "flat",
				Value: "10",
			}},
		},
		ReferenceAmount:  "originalAmount",
		Priority:         1,
		IsDeductibleFrom: &enableFlag,
		CreditAccount:    "@fee_account",
	}

	packEntity := &pack.Package{
		ID:             packID,
		MinimumAmount:  decimal.NewFromInt(100),
		MaximumAmount:  decimal.NewFromInt(2000),
		Fees:           map[string]model.Fee{"test": fee},
		WaivedAccounts: &[]string{},
	}

	mockPackRepo.EXPECT().
		FindByOrganizationIDAndLedgerID(gomock.Any(), orgID, ledgerID).
		Return([]*pack.Package{packEntity}, nil)

	ctx := context.Background()
	err := feeSvc.CalculateFee(ctx, feeInput, orgID)
	assert.NoError(t, err)
}

// TestCalculateFee_SinglePackage_ValueAtMaximum tests value at maximum limit
func TestCalculateFee_SinglePackage_ValueAtMaximum(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockPackRepo := pack.NewMockRepository(ctrl)
	orgID := uuid.New()
	ledgerID := uuid.New()
	packID := uuid.New()
	enableFlag := false

	feeSvc := &UseCase{
		packageRepo: mockPackRepo,
	}

	from := transaction.FromTo{
		Amount: &transaction.Amount{
			Asset: "BRL",
			Value: decimal.NewFromInt(2000),
		},
	}

	to := transaction.FromTo{
		Amount: &transaction.Amount{
			Asset: "BRL",
			Value: decimal.NewFromInt(2000),
		},
	}

	feeInput := &model.FeeCalculate{
		SegmentID: nil,
		LedgerID:  ledgerID,
		Transaction: transaction.Transaction{
			Send: transaction.Send{
				Asset: "BRL",
				Value: decimal.NewFromInt(2000),
				Source: transaction.Source{
					From: []transaction.FromTo{from},
				},
				Distribute: transaction.Distribute{
					To: []transaction.FromTo{to},
				},
			},
		},
	}

	fee := model.Fee{
		FeeLabel: "TestFee",
		CalculationModel: &model.CalculationModel{
			ApplicationRule: "flatFee",
			Calculations: []model.Calculation{{
				Type:  "flat",
				Value: "10",
			}},
		},
		ReferenceAmount:  "originalAmount",
		Priority:         1,
		IsDeductibleFrom: &enableFlag,
		CreditAccount:    "@fee_account",
	}

	packEntity := &pack.Package{
		ID:             packID,
		MinimumAmount:  decimal.NewFromInt(100),
		MaximumAmount:  decimal.NewFromInt(2000),
		Fees:           map[string]model.Fee{"test": fee},
		WaivedAccounts: &[]string{},
	}

	mockPackRepo.EXPECT().
		FindByOrganizationIDAndLedgerID(gomock.Any(), orgID, ledgerID).
		Return([]*pack.Package{packEntity}, nil)

	ctx := context.Background()
	err := feeSvc.CalculateFee(ctx, feeInput, orgID)
	assert.NoError(t, err)
}

// TestCalculateFee_MultiplePackages_ValueAtMinimum tests multiple packages with value at minimum
func TestCalculateFee_MultiplePackages_ValueAtMinimum(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockPackRepo := pack.NewMockRepository(ctrl)
	orgID := uuid.New()
	ledgerID := uuid.New()
	packID1 := uuid.New()
	packID2 := uuid.New()

	feeSvc := &UseCase{
		packageRepo: mockPackRepo,
	}

	route := "debitoted"
	from := transaction.FromTo{
		Amount: &transaction.Amount{
			Asset: "BRL",
			Value: decimal.NewFromInt(100),
		},
	}

	to := transaction.FromTo{
		Amount: &transaction.Amount{
			Asset: "BRL",
			Value: decimal.NewFromInt(100),
		},
	}

	feeInput := &model.FeeCalculate{
		SegmentID: nil,
		LedgerID:  ledgerID,
		Transaction: transaction.Transaction{
			Route: route,
			Send: transaction.Send{
				Asset: "BRL",
				Value: decimal.NewFromInt(100),
				Source: transaction.Source{
					From: []transaction.FromTo{from},
				},
				Distribute: transaction.Distribute{
					To: []transaction.FromTo{to},
				},
			},
		},
	}

	packEntity1 := &pack.Package{
		ID:               packID1,
		TransactionRoute: &route,
		MinimumAmount:    decimal.NewFromInt(100),
		MaximumAmount:    decimal.NewFromInt(1000),
		Fees:             map[string]model.Fee{},
		WaivedAccounts:   &[]string{},
	}

	packEntity2 := &pack.Package{
		ID:               packID2,
		TransactionRoute: &route,
		MinimumAmount:    decimal.NewFromInt(2000),
		MaximumAmount:    decimal.NewFromInt(5000),
		Fees:             map[string]model.Fee{},
		WaivedAccounts:   &[]string{},
	}

	mockPackRepo.EXPECT().
		FindByOrganizationIDAndLedgerID(gomock.Any(), orgID, ledgerID).
		Return([]*pack.Package{packEntity1, packEntity2}, nil)

	ctx := context.Background()
	err := feeSvc.CalculateFee(ctx, feeInput, orgID)
	assert.NoError(t, err)
}

// TestCalculateFee_MultiplePackages_ValueAtMaximum tests multiple packages with value at maximum
func TestCalculateFee_MultiplePackages_ValueAtMaximum(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockPackRepo := pack.NewMockRepository(ctrl)
	orgID := uuid.New()
	ledgerID := uuid.New()
	packID1 := uuid.New()
	packID2 := uuid.New()

	feeSvc := &UseCase{
		packageRepo: mockPackRepo,
	}

	route := "debitoted"
	from := transaction.FromTo{
		Amount: &transaction.Amount{
			Asset: "BRL",
			Value: decimal.NewFromInt(1000),
		},
	}

	to := transaction.FromTo{
		Amount: &transaction.Amount{
			Asset: "BRL",
			Value: decimal.NewFromInt(1000),
		},
	}

	feeInput := &model.FeeCalculate{
		SegmentID: nil,
		LedgerID:  ledgerID,
		Transaction: transaction.Transaction{
			Route: route,
			Send: transaction.Send{
				Asset: "BRL",
				Value: decimal.NewFromInt(1000),
				Source: transaction.Source{
					From: []transaction.FromTo{from},
				},
				Distribute: transaction.Distribute{
					To: []transaction.FromTo{to},
				},
			},
		},
	}

	packEntity1 := &pack.Package{
		ID:               packID1,
		TransactionRoute: &route,
		MinimumAmount:    decimal.NewFromInt(100),
		MaximumAmount:    decimal.NewFromInt(1000),
		Fees:             map[string]model.Fee{},
		WaivedAccounts:   &[]string{},
	}

	packEntity2 := &pack.Package{
		ID:               packID2,
		TransactionRoute: &route,
		MinimumAmount:    decimal.NewFromInt(2000),
		MaximumAmount:    decimal.NewFromInt(5000),
		Fees:             map[string]model.Fee{},
		WaivedAccounts:   &[]string{},
	}

	mockPackRepo.EXPECT().
		FindByOrganizationIDAndLedgerID(gomock.Any(), orgID, ledgerID).
		Return([]*pack.Package{packEntity1, packEntity2}, nil)

	ctx := context.Background()
	err := feeSvc.CalculateFee(ctx, feeInput, orgID)
	assert.NoError(t, err)
}

// TestCalculateFee_MultiplePackages_WithSegmentID tests multiple packages with segmentID
func TestCalculateFee_MultiplePackages_WithSegmentID(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockPackRepo := pack.NewMockRepository(ctrl)
	orgID := uuid.New()
	ledgerID := uuid.New()
	packID1 := uuid.New()
	packID2 := uuid.New()
	segmentID1 := uuid.New()
	segmentID2 := uuid.New()
	enableFlag := false

	feeSvc := &UseCase{
		packageRepo: mockPackRepo,
	}

	route := "debitoted"
	from := transaction.FromTo{
		Amount: &transaction.Amount{
			Asset: "BRL",
			Value: decimal.NewFromInt(500),
		},
	}

	to := transaction.FromTo{
		Amount: &transaction.Amount{
			Asset: "BRL",
			Value: decimal.NewFromInt(500),
		},
	}

	feeInput := &model.FeeCalculate{
		SegmentID: &segmentID1,
		LedgerID:  ledgerID,
		Transaction: transaction.Transaction{
			Route: route,
			Send: transaction.Send{
				Asset: "BRL",
				Value: decimal.NewFromInt(500),
				Source: transaction.Source{
					From: []transaction.FromTo{from},
				},
				Distribute: transaction.Distribute{
					To: []transaction.FromTo{to},
				},
			},
		},
	}

	fee := model.Fee{
		FeeLabel: "TestFee",
		CalculationModel: &model.CalculationModel{
			ApplicationRule: "flatFee",
			Calculations: []model.Calculation{{
				Type:  "flat",
				Value: "50",
			}},
		},
		ReferenceAmount:  "originalAmount",
		Priority:         1,
		IsDeductibleFrom: &enableFlag,
		CreditAccount:    "@fee_account",
	}

	packEntity1 := &pack.Package{
		ID:               packID1,
		TransactionRoute: &route,
		SegmentID:        &segmentID1,
		MinimumAmount:    decimal.NewFromInt(100),
		MaximumAmount:    decimal.NewFromInt(1000),
		Fees:             map[string]model.Fee{"test": fee},
		WaivedAccounts:   &[]string{},
	}

	packEntity2 := &pack.Package{
		ID:               packID2,
		TransactionRoute: &route,
		SegmentID:        &segmentID2,
		MinimumAmount:    decimal.NewFromInt(100),
		MaximumAmount:    decimal.NewFromInt(1000),
		Fees:             map[string]model.Fee{"test": fee},
		WaivedAccounts:   &[]string{},
	}

	mockPackRepo.EXPECT().
		FindByOrganizationIDAndLedgerID(gomock.Any(), orgID, ledgerID).
		Return([]*pack.Package{packEntity1, packEntity2}, nil)

	ctx := context.Background()
	err := feeSvc.CalculateFee(ctx, feeInput, orgID)
	assert.NoError(t, err)
	assert.Greater(t, feeInput.Transaction.Send.Value.IntPart(), int64(500))
}

// TestCalculateFee_MultiplePackages_WithMetadataUpdate tests multiple packages with metadata update
func TestCalculateFee_MultiplePackages_WithMetadataUpdate(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockPackRepo := pack.NewMockRepository(ctrl)
	orgID := uuid.New()
	ledgerID := uuid.New()
	packID1 := uuid.New()
	packID2 := uuid.New()
	enableFlag := false

	feeSvc := &UseCase{
		packageRepo: mockPackRepo,
	}

	route := "debitoted"
	from := transaction.FromTo{
		Amount: &transaction.Amount{
			Asset: "BRL",
			Value: decimal.NewFromInt(500),
		},
	}

	to := transaction.FromTo{
		Amount: &transaction.Amount{
			Asset: "BRL",
			Value: decimal.NewFromInt(500),
		},
	}

	feeInput := &model.FeeCalculate{
		SegmentID: nil,
		LedgerID:  ledgerID,
		Transaction: transaction.Transaction{
			Route:    route,
			Metadata: nil,
			Send: transaction.Send{
				Asset: "BRL",
				Value: decimal.NewFromInt(500),
				Source: transaction.Source{
					From: []transaction.FromTo{from},
				},
				Distribute: transaction.Distribute{
					To: []transaction.FromTo{to},
				},
			},
		},
	}

	fee := model.Fee{
		FeeLabel: "TestFee",
		CalculationModel: &model.CalculationModel{
			ApplicationRule: "flatFee",
			Calculations: []model.Calculation{{
				Type:  "flat",
				Value: "50",
			}},
		},
		ReferenceAmount:  "originalAmount",
		Priority:         1,
		IsDeductibleFrom: &enableFlag,
		CreditAccount:    "@fee_account",
	}

	packEntity1 := &pack.Package{
		ID:               packID1,
		TransactionRoute: &route,
		MinimumAmount:    decimal.NewFromInt(100),
		MaximumAmount:    decimal.NewFromInt(1000),
		Fees:             map[string]model.Fee{"test": fee},
		WaivedAccounts:   &[]string{},
	}

	packEntity2 := &pack.Package{
		ID:               packID2,
		TransactionRoute: &route,
		MinimumAmount:    decimal.NewFromInt(2000),
		MaximumAmount:    decimal.NewFromInt(5000),
		Fees:             map[string]model.Fee{"test": fee},
		WaivedAccounts:   &[]string{},
	}

	mockPackRepo.EXPECT().
		FindByOrganizationIDAndLedgerID(gomock.Any(), orgID, ledgerID).
		Return([]*pack.Package{packEntity1, packEntity2}, nil)

	ctx := context.Background()
	err := feeSvc.CalculateFee(ctx, feeInput, orgID)
	assert.NoError(t, err)
	assert.NotNil(t, feeInput.Transaction.Metadata)
	assert.Equal(t, packID1.String(), feeInput.Transaction.Metadata["packageAppliedID"])
}

// TestCalculateFee_ValidationError tests error in transaction validation
func TestCalculateFee_ValidationError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockPackRepo := pack.NewMockRepository(ctrl)
	orgID := uuid.New()
	ledgerID := uuid.New()
	packID := uuid.New()

	feeSvc := &UseCase{
		packageRepo: mockPackRepo,
	}

	feeInput := &model.FeeCalculate{
		SegmentID: nil,
		LedgerID:  ledgerID,
		Transaction: transaction.Transaction{
			Send: transaction.Send{
				Asset: "BRL",
				Value: decimal.NewFromInt(1000),
				Source: transaction.Source{
					From: []transaction.FromTo{},
				},
				Distribute: transaction.Distribute{
					To: []transaction.FromTo{},
				},
			},
		},
	}

	packEntity := &pack.Package{
		ID:            packID,
		MinimumAmount: decimal.NewFromInt(100),
		MaximumAmount: decimal.NewFromInt(2000),
		Fees:          map[string]model.Fee{},
	}

	mockPackRepo.EXPECT().
		FindByOrganizationIDAndLedgerID(gomock.Any(), orgID, ledgerID).
		Return([]*pack.Package{packEntity}, nil)

	ctx := context.Background()
	err := feeSvc.CalculateFee(ctx, feeInput, orgID)
	assert.Error(t, err)
}

// TestCalculateFee_SinglePackage_ValueOutOfRange tests single package with value out of range
func TestCalculateFee_SinglePackage_ValueOutOfRange(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockPackRepo := pack.NewMockRepository(ctrl)
	orgID := uuid.New()
	ledgerID := uuid.New()
	packID := uuid.New()

	feeSvc := &UseCase{
		packageRepo: mockPackRepo,
	}

	feeInput := &model.FeeCalculate{
		SegmentID: nil,
		LedgerID:  ledgerID,
		Transaction: transaction.Transaction{
			Send: transaction.Send{
				Asset: "BRL",
				Value: decimal.NewFromInt(50),
				Source: transaction.Source{
					From: []transaction.FromTo{{
						Amount: &transaction.Amount{Asset: "BRL", Value: decimal.NewFromInt(50)},
					}},
				},
				Distribute: transaction.Distribute{
					To: []transaction.FromTo{{
						Amount: &transaction.Amount{Asset: "BRL", Value: decimal.NewFromInt(50)},
					}},
				},
			},
		},
	}

	packEntity := &pack.Package{
		ID:            packID,
		MinimumAmount: decimal.NewFromInt(100),
		MaximumAmount: decimal.NewFromInt(2000),
		Fees:          map[string]model.Fee{},
	}

	mockPackRepo.EXPECT().
		FindByOrganizationIDAndLedgerID(gomock.Any(), orgID, ledgerID).
		Return([]*pack.Package{packEntity}, nil)

	ctx := context.Background()
	err := feeSvc.CalculateFee(ctx, feeInput, orgID)
	assert.NoError(t, err)
}

// TestCalculateFee_SinglePackage_ValueAboveMax tests single package with value above maximum
func TestCalculateFee_SinglePackage_ValueAboveMax(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockPackRepo := pack.NewMockRepository(ctrl)
	orgID := uuid.New()
	ledgerID := uuid.New()
	packID := uuid.New()

	feeSvc := &UseCase{
		packageRepo: mockPackRepo,
	}

	feeInput := &model.FeeCalculate{
		SegmentID: nil,
		LedgerID:  ledgerID,
		Transaction: transaction.Transaction{
			Send: transaction.Send{
				Asset: "BRL",
				Value: decimal.NewFromInt(3000),
				Source: transaction.Source{
					From: []transaction.FromTo{{
						Amount: &transaction.Amount{Asset: "BRL", Value: decimal.NewFromInt(3000)},
					}},
				},
				Distribute: transaction.Distribute{
					To: []transaction.FromTo{{
						Amount: &transaction.Amount{Asset: "BRL", Value: decimal.NewFromInt(3000)},
					}},
				},
			},
		},
	}

	packEntity := &pack.Package{
		ID:            packID,
		MinimumAmount: decimal.NewFromInt(100),
		MaximumAmount: decimal.NewFromInt(2000),
		Fees:          map[string]model.Fee{},
	}

	mockPackRepo.EXPECT().
		FindByOrganizationIDAndLedgerID(gomock.Any(), orgID, ledgerID).
		Return([]*pack.Package{packEntity}, nil)

	ctx := context.Background()
	err := feeSvc.CalculateFee(ctx, feeInput, orgID)
	assert.NoError(t, err)
}

// TestCalculateFee_MultiplePackages tests calculation with multiple packages
func TestCalculateFee_MultiplePackages(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockPackRepo := pack.NewMockRepository(ctrl)
	orgID := uuid.New()
	ledgerID := uuid.New()
	packID1 := uuid.New()
	packID2 := uuid.New()

	feeSvc := &UseCase{
		packageRepo: mockPackRepo,
	}

	route := "debitoted"
	feeInput := &model.FeeCalculate{
		SegmentID: nil,
		LedgerID:  ledgerID,
		Transaction: transaction.Transaction{
			Route: route,
			Send: transaction.Send{
				Asset: "BRL",
				Value: decimal.NewFromInt(500),
				Source: transaction.Source{
					From: []transaction.FromTo{{
						Amount: &transaction.Amount{Asset: "BRL", Value: decimal.NewFromInt(500)},
					}},
				},
				Distribute: transaction.Distribute{
					To: []transaction.FromTo{{
						Amount: &transaction.Amount{Asset: "BRL", Value: decimal.NewFromInt(500)},
					}},
				},
			},
		},
	}

	packEntity1 := &pack.Package{
		ID:               packID1,
		TransactionRoute: &route,
		MinimumAmount:    decimal.NewFromInt(100),
		MaximumAmount:    decimal.NewFromInt(1000),
		Fees:             map[string]model.Fee{},
	}

	packEntity2 := &pack.Package{
		ID:               packID2,
		TransactionRoute: &route,
		MinimumAmount:    decimal.NewFromInt(2000),
		MaximumAmount:    decimal.NewFromInt(5000),
		Fees:             map[string]model.Fee{},
	}

	mockPackRepo.EXPECT().
		FindByOrganizationIDAndLedgerID(gomock.Any(), orgID, ledgerID).
		Return([]*pack.Package{packEntity1, packEntity2}, nil)

	ctx := context.Background()
	err := feeSvc.CalculateFee(ctx, feeInput, orgID)
	assert.NoError(t, err)
}

// TestCalculateFee_MultiplePackages_NoPackageFound tests multiple packages but none found after filter
func TestCalculateFee_MultiplePackages_NoPackageFound(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockPackRepo := pack.NewMockRepository(ctrl)
	orgID := uuid.New()
	ledgerID := uuid.New()
	packID1 := uuid.New()
	packID2 := uuid.New()

	feeSvc := &UseCase{
		packageRepo: mockPackRepo,
	}

	route1 := "debitoted"
	route2 := "creditfrom"
	feeInput := &model.FeeCalculate{
		SegmentID: nil,
		LedgerID:  ledgerID,
		Transaction: transaction.Transaction{
			Route: route1,
			Send: transaction.Send{
				Asset: "BRL",
				Value: decimal.NewFromInt(500),
				Source: transaction.Source{
					From: []transaction.FromTo{{
						Amount: &transaction.Amount{Asset: "BRL", Value: decimal.NewFromInt(500)},
					}},
				},
				Distribute: transaction.Distribute{
					To: []transaction.FromTo{{
						Amount: &transaction.Amount{Asset: "BRL", Value: decimal.NewFromInt(500)},
					}},
				},
			},
		},
	}

	packEntity1 := &pack.Package{
		ID:               packID1,
		TransactionRoute: &route2,
		MinimumAmount:    decimal.NewFromInt(100),
		MaximumAmount:    decimal.NewFromInt(1000),
		Fees:             map[string]model.Fee{},
	}

	packEntity2 := &pack.Package{
		ID:               packID2,
		TransactionRoute: &route2,
		MinimumAmount:    decimal.NewFromInt(2000),
		MaximumAmount:    decimal.NewFromInt(5000),
		Fees:             map[string]model.Fee{},
	}

	mockPackRepo.EXPECT().
		FindByOrganizationIDAndLedgerID(gomock.Any(), orgID, ledgerID).
		Return([]*pack.Package{packEntity1, packEntity2}, nil)

	ctx := context.Background()
	err := feeSvc.CalculateFee(ctx, feeInput, orgID)
	assert.NoError(t, err)
}

// TestCalculateFee_MultiplePackages_FilterError tests error when filtering package
func TestCalculateFee_MultiplePackages_FilterError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockPackRepo := pack.NewMockRepository(ctrl)
	orgID := uuid.New()
	ledgerID := uuid.New()
	packID1 := uuid.New()
	packID2 := uuid.New()

	feeSvc := &UseCase{
		packageRepo: mockPackRepo,
	}

	route := "debitoted"
	feeInput := &model.FeeCalculate{
		SegmentID: nil,
		LedgerID:  ledgerID,
		Transaction: transaction.Transaction{
			Route: route,
			Send: transaction.Send{
				Asset: "BRL",
				Value: decimal.NewFromInt(500),
				Source: transaction.Source{
					From: []transaction.FromTo{{
						Amount: &transaction.Amount{Asset: "BRL", Value: decimal.NewFromInt(500)},
					}},
				},
				Distribute: transaction.Distribute{
					To: []transaction.FromTo{{
						Amount: &transaction.Amount{Asset: "BRL", Value: decimal.NewFromInt(500)},
					}},
				},
			},
		},
	}

	packEntity1 := &pack.Package{
		ID:               packID1,
		TransactionRoute: &route,
		MinimumAmount:    decimal.NewFromInt(100),
		MaximumAmount:    decimal.NewFromInt(1000),
		Fees:             map[string]model.Fee{},
	}

	packEntity2 := &pack.Package{
		ID:               packID2,
		TransactionRoute: &route,
		MinimumAmount:    decimal.NewFromInt(100),
		MaximumAmount:    decimal.NewFromInt(1000),
		Fees:             map[string]model.Fee{},
	}

	mockPackRepo.EXPECT().
		FindByOrganizationIDAndLedgerID(gomock.Any(), orgID, ledgerID).
		Return([]*pack.Package{packEntity1, packEntity2}, nil)

	ctx := context.Background()
	err := feeSvc.CalculateFee(ctx, feeInput, orgID)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), constant.ErrFilterPackage.Error())
}

// TestCalculateFee_MultiplePackages_ValueOutOfRange tests multiple packages with value out of range
func TestCalculateFee_MultiplePackages_ValueOutOfRange(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockPackRepo := pack.NewMockRepository(ctrl)
	orgID := uuid.New()
	ledgerID := uuid.New()
	packID1 := uuid.New()

	feeSvc := &UseCase{
		packageRepo: mockPackRepo,
	}

	route := "debitoted"
	feeInput := &model.FeeCalculate{
		SegmentID: nil,
		LedgerID:  ledgerID,
		Transaction: transaction.Transaction{
			Route: route,
			Send: transaction.Send{
				Asset: "BRL",
				Value: decimal.NewFromInt(50),
				Source: transaction.Source{
					From: []transaction.FromTo{{
						Amount: &transaction.Amount{Asset: "BRL", Value: decimal.NewFromInt(50)},
					}},
				},
				Distribute: transaction.Distribute{
					To: []transaction.FromTo{{
						Amount: &transaction.Amount{Asset: "BRL", Value: decimal.NewFromInt(50)},
					}},
				},
			},
		},
	}

	packEntity1 := &pack.Package{
		ID:               packID1,
		TransactionRoute: &route,
		MinimumAmount:    decimal.NewFromInt(100),
		MaximumAmount:    decimal.NewFromInt(1000),
		Fees:             map[string]model.Fee{},
	}

	mockPackRepo.EXPECT().
		FindByOrganizationIDAndLedgerID(gomock.Any(), orgID, ledgerID).
		Return([]*pack.Package{packEntity1}, nil)

	ctx := context.Background()
	err := feeSvc.CalculateFee(ctx, feeInput, orgID)
	assert.NoError(t, err)
}

// TestCalculateFee_TechnicalError_MalformedSegmentWaiver drives a NON-business
// (technical) error out of feeUtils.CalculateFee so recordSpanError takes its
// HandleSpanError arm — the technical/5xx side of the T5 span classification.
// A package whose waivedAccounts carries "segment:<not-a-uuid>" fails segment
// waiver resolution with a bare error that pkg.IsBusinessError must NOT accept
// as business: were that predicate ever inverted, every technical fee failure
// would be reported on a green span. Both CalculateFee tails are exercised, the
// single-package one and the multi-package one.
func TestCalculateFee_TechnicalError_MalformedSegmentWaiver(t *testing.T) {
	t.Parallel()

	const malformedWaiver = "segment:not-a-uuid"

	isDeductible := false

	validFee := model.Fee{
		FeeLabel: "TestFee",
		CalculationModel: &model.CalculationModel{
			ApplicationRule: "flatFee",
			Calculations: []model.Calculation{{
				Type:  "flat",
				Value: "100",
			}},
		},
		ReferenceAmount:  "originalAmount",
		Priority:         1,
		IsDeductibleFrom: &isDeductible,
		CreditAccount:    "@fee_account",
	}

	// newPackage builds an otherwise valid flat-fee package, so the only error
	// the calculation can produce comes from the waivedAccounts entries.
	newPackage := func(minAmount, maxAmount int64, waived []string) *pack.Package {
		return &pack.Package{
			ID:             uuid.New(),
			MinimumAmount:  decimal.NewFromInt(minAmount),
			MaximumAmount:  decimal.NewFromInt(maxAmount),
			Fees:           map[string]model.Fee{"test": validFee},
			WaivedAccounts: &waived,
		}
	}

	tests := []struct {
		name     string
		packages []*pack.Package
	}{
		{
			name:     "single package tail",
			packages: []*pack.Package{newPackage(100, 2000, []string{malformedWaiver})},
		},
		{
			// The second package is out of the amount range, so the filter
			// selects the malformed one while len(packages) > 1 still routes
			// through calculateFeeForMultiplePackages.
			name: "multiple packages tail",
			packages: []*pack.Package{
				newPackage(100, 2000, []string{malformedWaiver}),
				newPackage(5000, 9000, []string{}),
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockPackRepo := pack.NewMockRepository(ctrl)
			orgID := uuid.New()
			ledgerID := uuid.New()

			feeSvc := &UseCase{
				packageRepo: mockPackRepo,
			}

			feeInput := &model.FeeCalculate{
				SegmentID: nil,
				LedgerID:  ledgerID,
				Transaction: transaction.Transaction{
					Send: transaction.Send{
						Asset: "BRL",
						Value: decimal.NewFromInt(1000),
						Source: transaction.Source{
							From: []transaction.FromTo{{
								Amount: &transaction.Amount{Asset: "BRL", Value: decimal.NewFromInt(1000)},
							}},
						},
						Distribute: transaction.Distribute{
							To: []transaction.FromTo{{
								Amount: &transaction.Amount{Asset: "BRL", Value: decimal.NewFromInt(1000)},
							}},
						},
					},
				},
			}

			mockPackRepo.EXPECT().
				FindByOrganizationIDAndLedgerID(gomock.Any(), orgID, ledgerID).
				Return(tt.packages, nil)

			// A real SDK tracer injected through the lib-observability context
			// seam, so the span CalculateFee opens is recorded and its final
			// status can be read back instead of only the branch predicate.
			recorder := tracetest.NewSpanRecorder()
			provider := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(recorder))

			ctx := libObservability.ContextWithTracer(context.Background(), provider.Tracer("fees_test"))
			err := feeSvc.CalculateFee(ctx, feeInput, orgID)

			assert.Error(t, err)
			assert.ErrorContains(t, err, malformedWaiver)
			assert.False(t, pkg.IsBusinessError(err),
				"malformed segment waiver must stay technical so recordSpanError flips the span red")
			assert.True(t, feeInput.Transaction.Send.Value.Equal(decimal.NewFromInt(1000)),
				"a failed calculation must leave the send value untouched")

			var recorded sdktrace.ReadOnlySpan

			for _, s := range recorder.Ended() {
				if s.Name() == "service.calculate_fee" {
					recorded = s

					break
				}
			}

			require.NotNil(t, recorded, "the injected tracer must receive the service.calculate_fee span")
			assert.Equal(t, codes.Error, recorded.Status().Code,
				"a technical fee failure must leave the span status Error")
		})
	}
}

// routedFeeInput builds the transfer the create seam produces for a routed
// payment: the canonical route identifier carries the route UUID and the
// deprecated route string stays empty, which is the shape every payment that
// can be charged a fee actually travels in.
func routedFeeInput(ledgerID uuid.UUID, routeID string) *model.FeeCalculate {
	feeInput := segScopingFeeInput(ledgerID, "@src")
	feeInput.Transaction.RouteID = &routeID

	return feeInput
}

// routeScopedFlatPackage builds the flat-100 package a client restricted to one
// transaction route, over the amount band the routed transfer of 1000 falls in.
func routeScopedFlatPackage(packID uuid.UUID, routeID string) *pack.Package {
	packEntity := segScopingFlatPackage(packID, nil)
	packEntity.TransactionRoute = &routeID

	return packEntity
}

// routeAndSegmentScopedFlatPackage builds the flat-100 package a client
// restricted to one transaction route AND one segment, the most specific scope a
// package can carry.
func routeAndSegmentScopedFlatPackage(packID uuid.UUID, routeID string, segmentID *uuid.UUID) *pack.Package {
	packEntity := segScopingFlatPackage(packID, segmentID)
	packEntity.TransactionRoute = &routeID

	return packEntity
}

// outOfBandPackage moves a package out of the routed transfer amount band, so a
// second stored package forces the multi-package selection path without making
// the selection ambiguous.
func outOfBandPackage(packEntity *pack.Package) *pack.Package {
	packEntity.MinimumAmount = decimal.NewFromInt(3000)
	packEntity.MaximumAmount = decimal.NewFromInt(5000)

	return packEntity
}

// TestCalculateFee_RouteScoping proves at the fee service entry point, the seam
// every payment travels, which package a routed payment is charged and how much
// it is charged. Every fixture here is the same flat 100 over a transfer of
// 1000, so a charged case is pinned to the exact post-fee value of 1100 and to
// the id of the package the ledger recorded: a fee applied twice, a fee read as
// a percentage, or a fee landing on the wrong leg all fail here rather than
// passing a "the value grew" assertion.
//
// Three rows carry the payment shapes whose money must not move at all: a
// client running one package with no segment constraint, on a payment whose
// source resolves into a segment, is charged on the legacy unrouted payment and
// on the routed one alike, exactly as the ledger charges them today.
//
// Six more carry the segment rule and what it costs: a package carrying no
// segment constraint is charged in every segment rather than dropped, the
// package matching the most constraints wins, and packages matching the same
// number refuse the payment. Each row that moves money against origin/develop
// says what origin/develop charges on that shape.
//
// Mutants each row kills are named on the row.
func TestCalculateFee_RouteScoping(t *testing.T) {
	t.Parallel()

	const (
		originalValue = int64(1000)
		chargedValue  = int64(1100)
	)

	routeID := uuid.New().String()
	otherRouteID := uuid.New().String()
	sourceSegment := uuid.New()
	otherSegment := uuid.New()

	tests := []struct {
		name string
		// packages is what the ledger holds; one entry drives the sole-package
		// selection path and more than one drives the multi-package path.
		packages []*pack.Package
		// wantChargedIdx indexes packages with the one that must be charged, and
		// is negative when the payment must be charged nothing.
		wantChargedIdx int
		// wantErrCode is the business error code the payment must be refused
		// with, empty when the payment must succeed.
		wantErrCode string
		// segmentOfSource, when set, resolves the payment's source account into
		// that segment; nil leaves the payment unsegmented and needs no resolver.
		segmentOfSource *uuid.UUID
		// unroutedPayment builds the legacy payment that carries no route
		// identifier at all, the shape whose selection must stay exactly what
		// the ledger selects today.
		unroutedPayment bool
	}{
		{
			// The defect this repair closes, on the sole-package path: the
			// service handed the selector the deprecated route string, which the
			// create seam leaves empty, so the restricted package was never
			// selected and the payment was charged nothing.
			// Mutant: revert the route accessor to the deprecated string.
			name:           "a package restricted to this route is charged when it is the only one on the ledger",
			packages:       []*pack.Package{routeScopedFlatPackage(uuid.New(), routeID)},
			wantChargedIdx: 0,
		},
		{
			// The same repair on the multi-package path, a separate call into
			// the selector that carried the same defect. The second package sits
			// outside the amount band, so it forces that path without colliding.
			// Mutant: revert the route accessor to the deprecated string.
			name: "a package restricted to this route is charged when the ledger holds several",
			packages: []*pack.Package{
				routeScopedFlatPackage(uuid.New(), routeID),
				outOfBandPackage(routeScopedFlatPackage(uuid.New(), otherRouteID)),
			},
			wantChargedIdx: 0,
		},
		{
			// The regression guard: a client charging one flat package on
			// everything must go on being charged it once routed payments reach
			// the selector.
			// Mutant: restore the clause that kept an unrestricted package only
			// against an empty route.
			name:           "a package restricted to nothing is still charged on a routed payment",
			packages:       []*pack.Package{segScopingFlatPackage(uuid.New(), nil)},
			wantChargedIdx: 0,
		},
		{
			// The same regression guard on the multi-package path.
			// Mutant: restore the clause that kept an unrestricted package only
			// against an empty route.
			name: "a package restricted to nothing is still charged when the ledger holds several",
			packages: []*pack.Package{
				segScopingFlatPackage(uuid.New(), nil),
				outOfBandPackage(segScopingFlatPackage(uuid.New(), nil)),
			},
			wantChargedIdx: 0,
		},
		{
			// The collision rule, at the seam the money moves: the package a
			// client restricted to this route is charged, not the one they left
			// unrestricted.
			// Mutant: delete the specificity tiebreak.
			name: "the package restricted to this route is charged rather than the unrestricted one",
			packages: []*pack.Package{
				segScopingFlatPackage(uuid.New(), nil),
				routeScopedFlatPackage(uuid.New(), routeID),
			},
			wantChargedIdx: 1,
		},
		{
			// Two packages a client restricted to the same route are equally
			// specific, so no tiebreak separates them and the payment is refused
			// rather than charged an arbitrary one of the two. Before route
			// selection was repaired both were dropped and the payment posted
			// with no fee, so this refusal is newly reachable and is pinned as a
			// decision rather than left to be discovered in production.
			name: "two packages restricted to the same route refuse the payment",
			packages: []*pack.Package{
				routeScopedFlatPackage(uuid.New(), routeID),
				routeScopedFlatPackage(uuid.New(), routeID),
			},
			wantChargedIdx: -1,
			wantErrCode:    constant.ErrFilterPackage.Error(),
		},
		{
			// A package restricted to this route is charged only inside the
			// amount band its client configured, even when it is the only
			// package the ledger holds. The selector drops it and both
			// selection paths re-check the band on whatever comes back, so two
			// independent guards stand between that package and the money.
			name:           "a package restricted to this route is not charged outside its own amount band",
			packages:       []*pack.Package{outOfBandPackage(routeScopedFlatPackage(uuid.New(), routeID))},
			wantChargedIdx: -1,
		},
		{
			// The same band, on the other selection path. The second package is
			// scoped to another route, so the route filter leaves the
			// out-of-band one standing alone and the band filter then drops it,
			// with this path re-checking the band on whatever comes back.
			name: "a package restricted to this route is not charged outside its own amount band when the ledger holds several",
			packages: []*pack.Package{
				outOfBandPackage(routeScopedFlatPackage(uuid.New(), routeID)),
				routeScopedFlatPackage(uuid.New(), otherRouteID),
			},
			wantChargedIdx: -1,
		},
		{
			// The create contract accepts a blank route and stores it, so every
			// package a client saved without choosing a route carries one. They
			// applied to every payment before this repair and must go on doing
			// so after it.
			// Mutant: compare the stored route pointer instead of its value.
			name:           "a package saved with a blank route is charged on a routed payment",
			packages:       []*pack.Package{routeScopedFlatPackage(uuid.New(), "")},
			wantChargedIdx: 0,
		},
		{
			// The money the ledger moves today and this repair must not touch,
			// on the legacy payment that carries no route identifier: a client
			// running one package with no segment constraint is charged on a
			// payment whose source resolves into a segment. Measured on
			// origin/develop at ab7708be9: sendValue 1100, packageAppliedID
			// set, feeApplied true.
			name:            "a package restricted to nothing is charged on an unrouted payment whose source carries a segment",
			packages:        []*pack.Package{segScopingFlatPackage(uuid.New(), nil)},
			wantChargedIdx:  0,
			segmentOfSource: &sourceSegment,
			unroutedPayment: true,
		},
		{
			// The same client, the same package, on a payment carrying the
			// canonical route identifier. Also 1100 on origin/develop, and it
			// stays 1100 here: the package carries no segment constraint, so
			// it survives the segment filter on its own merit, the way a
			// package carrying no route constraint survives the route filter.
			// Nothing short-circuits it past a filter that was not run.
			name:            "a package restricted to nothing is charged on a routed payment whose source carries a segment",
			packages:        []*pack.Package{segScopingFlatPackage(uuid.New(), nil)},
			wantChargedIdx:  0,
			segmentOfSource: &sourceSegment,
		},
		{
			// What this repair adds on a segmented payment: the package a
			// client restricted to this route is now the survivor of the route
			// filter and is charged. origin/develop charges nothing here,
			// because the payment route never reached the filter and the
			// package was dropped by it.
			name:            "a package restricted to this route is charged on a payment whose source carries a segment",
			packages:        []*pack.Package{routeScopedFlatPackage(uuid.New(), routeID)},
			wantChargedIdx:  0,
			segmentOfSource: &sourceSegment,
		},
		{
			// The segment rule now matches the route rule: a package carrying
			// no segment constraint applies to any segment. Both packages
			// reach the specificity rule and the one a client restricted to
			// this route is charged. origin/develop charges the unrestricted
			// package here, sendValue 1100, because its route filter drops the
			// route-scoped one and leaves the unrestricted one standing alone.
			// This branch charged nothing here until the segment rule landed.
			name: "the package restricted to this route is charged on a segmented payment beside an unrestricted one",
			packages: []*pack.Package{
				segScopingFlatPackage(uuid.New(), nil),
				routeScopedFlatPackage(uuid.New(), routeID),
			},
			wantChargedIdx:  1,
			segmentOfSource: &sourceSegment,
		},
		{
			// The segment half of the specificity rule at the seam the money
			// moves: the package restricted to the payment segment matches one
			// more constraint than the package restricted to nothing.
			// Mutant: delete the specificity preference.
			name: "the package scoped to this segment is charged rather than the unrestricted one",
			packages: []*pack.Package{
				segScopingFlatPackage(uuid.New(), nil),
				segScopingFlatPackage(uuid.New(), &sourceSegment),
			},
			wantChargedIdx:  1,
			segmentOfSource: &sourceSegment,
		},
		{
			// A package restricted to another segment is dropped and the
			// unrestricted one is charged. origin/develop charges nothing on
			// this shape, because its segment filter drops the unrestricted
			// package too.
			// Mutant: restore the segment filter that kept only segment-scoped
			// packages.
			name: "the unrestricted package is charged when the segment-scoped one belongs to another segment",
			packages: []*pack.Package{
				segScopingFlatPackage(uuid.New(), nil),
				segScopingFlatPackage(uuid.New(), &otherSegment),
			},
			wantChargedIdx:  0,
			segmentOfSource: &sourceSegment,
		},
		{
			// Two packages restricted to nothing are as ambiguous on a
			// segmented payment as on an unsegmented one, so the payment is
			// refused rather than charged an arbitrary one of the two.
			// origin/develop charges nothing here instead: the refusal is a
			// behaviour change this rule brings.
			name: "two unrestricted packages refuse a segmented payment",
			packages: []*pack.Package{
				segScopingFlatPackage(uuid.New(), nil),
				segScopingFlatPackage(uuid.New(), nil),
			},
			wantChargedIdx:  -1,
			wantErrCode:     constant.ErrFilterPackage.Error(),
			segmentOfSource: &sourceSegment,
		},
		{
			// Specificity counts constraints matched, so the package
			// restricted to this route AND this segment beats the one
			// restricted to the route alone.
			// Mutant: delete the specificity preference.
			name: "the package restricted to this route and this segment is charged rather than the route-only one",
			packages: []*pack.Package{
				routeScopedFlatPackage(uuid.New(), routeID),
				routeAndSegmentScopedFlatPackage(uuid.New(), routeID, &sourceSegment),
			},
			wantChargedIdx:  1,
			segmentOfSource: &sourceSegment,
		},
		{
			// One constraint each and both matched: nothing separates them, so
			// the payment is refused rather than charged whichever of the two
			// storage returned first.
			name: "a route-scoped and a segment-scoped package refuse the payment",
			packages: []*pack.Package{
				routeScopedFlatPackage(uuid.New(), routeID),
				segScopingFlatPackage(uuid.New(), &sourceSegment),
			},
			wantChargedIdx:  -1,
			wantErrCode:     constant.ErrFilterPackage.Error(),
			segmentOfSource: &sourceSegment,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockPackRepo := pack.NewMockRepository(ctrl)
			orgID := uuid.New()
			ledgerID := uuid.New()

			feeSvc := &UseCase{packageRepo: mockPackRepo}

			if tc.segmentOfSource != nil {
				mockResolver := feeshared.NewMockMidazResolver(ctrl)
				mockResolver.EXPECT().
					GetAccountByAlias(gomock.Any(), orgID, ledgerID, "@src").
					Return(&feeshared.Account{ID: "acc", Alias: "@src", SegmentID: tc.segmentOfSource}, nil)

				feeSvc.resolver = mockResolver
			}

			feeInput := routedFeeInput(ledgerID, routeID)
			if tc.unroutedPayment {
				feeInput = segScopingFeeInput(ledgerID, "@src")
			}

			mockPackRepo.EXPECT().
				FindByOrganizationIDAndLedgerID(gomock.Any(), orgID, ledgerID).
				Return(tc.packages, nil)

			err := feeSvc.CalculateFee(context.Background(), feeInput, orgID)

			if tc.wantErrCode != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tc.wantErrCode)
			} else {
				require.NoError(t, err)
			}

			if tc.wantChargedIdx < 0 {
				assert.Equal(t, originalValue, feeInput.Transaction.Send.Value.IntPart(),
					"no package matches this payment, so its value must not move")
				assert.Nil(t, feeInput.Transaction.Metadata["packageAppliedID"])
				assert.Nil(t, feeInput.Transaction.Metadata["feeApplied"])

				return
			}

			assert.Equal(t, chargedValue, feeInput.Transaction.Send.Value.IntPart(),
				"the charged fee must be the flat 100 the selected package configures")
			assert.Equal(t, tc.packages[tc.wantChargedIdx].ID.String(), feeInput.Transaction.Metadata["packageAppliedID"],
				"the ledger must record the package it actually charged")
			assert.Equal(t, "true", feeInput.Transaction.Metadata["feeApplied"])
		})
	}
}

// TestCalculateFee_DeprecatedRouteStringCarriesNoFeeScope pins that the
// deprecated route string a transaction body can still carry selects no fee
// package. Only the canonical route identifier scopes a package.
//
// The two fields can disagree. The create path that charges fees declares the
// canonical identifier and refuses an unknown field, so a posted payment never
// carries the string at all; the fee estimate embeds the whole transaction
// model, whose contract still publishes the deprecated string, so a caller can
// send one there. Reading whichever field happens to hold a value let the same
// client be quoted one fee and charged another, and let a payment carrying only
// the deprecated string be charged a package it was never scoped to.
//
// So the string is inert here: a payment carrying it alone is charged what an
// unrouted payment is charged, and a payment carrying both is scoped by the
// canonical identifier only.
func TestCalculateFee_DeprecatedRouteStringCarriesNoFeeScope(t *testing.T) {
	t.Parallel()

	const (
		originalValue = int64(1000)
		chargedValue  = int64(1100)
	)

	routeID := uuid.New().String()
	otherRouteID := uuid.New().String()

	tests := []struct {
		name string
		// deprecatedRoute is the legacy route string the payment carries.
		deprecatedRoute string
		// canonicalRouteID is the route identifier the payment carries; empty
		// leaves the payment carrying none.
		canonicalRouteID string
		// wantCharged says whether the package restricted to routeID is charged.
		wantCharged bool
	}{
		{
			// The defect: the deprecated string alone selected the package, so a
			// payment nobody routed was charged a route-scoped fee.
			name:            "a payment carrying only the deprecated route string is charged nothing",
			deprecatedRoute: routeID,
			wantCharged:     false,
		},
		{
			// The canonical identifier is the one that scopes, and it still does.
			name:             "a payment carrying the canonical route identifier is charged",
			canonicalRouteID: routeID,
			wantCharged:      true,
		},
		{
			// The two fields disagreeing: the canonical identifier names another
			// route, so the package restricted to this one is out of scope and the
			// deprecated string does not put it back in.
			name:             "a payment whose canonical identifier names another route is charged nothing",
			deprecatedRoute:  routeID,
			canonicalRouteID: otherRouteID,
			wantCharged:      false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockPackRepo := pack.NewMockRepository(ctrl)
			orgID := uuid.New()
			ledgerID := uuid.New()
			packID := uuid.New()

			feeSvc := &UseCase{packageRepo: mockPackRepo}

			feeInput := segScopingFeeInput(ledgerID, "@src")
			feeInput.Transaction.Route = tc.deprecatedRoute

			if tc.canonicalRouteID != "" {
				canonical := tc.canonicalRouteID
				feeInput.Transaction.RouteID = &canonical
			}

			mockPackRepo.EXPECT().
				FindByOrganizationIDAndLedgerID(gomock.Any(), orgID, ledgerID).
				Return([]*pack.Package{routeScopedFlatPackage(packID, routeID)}, nil)

			err := feeSvc.CalculateFee(context.Background(), feeInput, orgID)
			require.NoError(t, err)

			if !tc.wantCharged {
				assert.Equal(t, originalValue, feeInput.Transaction.Send.Value.IntPart(),
					"the deprecated route string must not put a route-scoped package in scope")
				assert.Nil(t, feeInput.Transaction.Metadata["packageAppliedID"])
				assert.Nil(t, feeInput.Transaction.Metadata["feeApplied"])

				return
			}

			assert.Equal(t, chargedValue, feeInput.Transaction.Send.Value.IntPart(),
				"the canonical route identifier must go on selecting the package it names")
			assert.Equal(t, packID.String(), feeInput.Transaction.Metadata["packageAppliedID"])
		})
	}
}
