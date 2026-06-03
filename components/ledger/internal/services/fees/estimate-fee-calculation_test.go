// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package services

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/LerianStudio/midaz/v3/components/ledger/internal/adapters/mongodb/fees/pack"
	"github.com/LerianStudio/midaz/v3/components/ledger/pkg/feeshared"
	"github.com/LerianStudio/midaz/v3/components/ledger/pkg/feeshared/constant"
	"github.com/LerianStudio/midaz/v3/components/ledger/pkg/feeshared/model"

	transaction "github.com/LerianStudio/midaz/v3/pkg/mtransaction"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.uber.org/mock/gomock"
)

func TestCreateFeeEstimate(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockPackRepo := pack.NewMockRepository(ctrl)

	feeSvc := &UseCase{
		packageRepo: mockPackRepo,
	}

	orgID := uuid.New()
	packID := uuid.New()
	ledgerID := uuid.New()
	enableFlag := true
	enableFlagFalse := false

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
		Value: decimal.NewFromInt(100),
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

	createFeeInput := &model.FeeEstimate{
		PackageID:   packID,
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
	packEntity := &pack.Package{
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
		LedgerID: ledgerID,
		Transaction: transaction.Transaction{
			Pending:  false,
			Metadata: nil,
			Send: transaction.Send{
				Asset: "BRL",
				Value: decimal.NewFromInt(1700),
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

	tests := []struct {
		name             string
		feeEstimateInput *model.FeeEstimate
		orgID            uuid.UUID
		mockSetup        func()
		expectErr        bool
		errContains      string
		expectResul      *model.FeeCalculate
	}{
		{
			name:             "Success - Generate a fee estimate",
			feeEstimateInput: createFeeInput,
			orgID:            orgID,
			mockSetup: func() {
				mockPackRepo.EXPECT().
					FindByID(gomock.Any(), gomock.Any(), gomock.Any()).
					Return(packEntity, nil)
			},
			expectErr:   false,
			expectResul: responseFeeInput,
		},
		{
			name:             "Error - Find package to create fee",
			feeEstimateInput: createFeeInput,
			orgID:            orgID,
			mockSetup: func() {
				mockPackRepo.EXPECT().
					FindByID(gomock.Any(), gomock.Any(), gomock.Any()).
					Return(nil, constant.ErrEntityNotFound)
			},
			expectErr:   true,
			errContains: "FEE-0012",
			expectResul: nil,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			tt.mockSetup()

			ctx := context.Background()
			result, err := feeSvc.EstimateFeeCalculation(ctx, createFeeInput, tt.orgID)

			if tt.expectErr {
				assert.Error(t, err)
				assert.Nil(t, result)
				if tt.errContains != "" {
					assert.Contains(t, err.Error(), tt.errContains)
				}
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, result)
				assert.True(t, result.Transaction.Send.Value.Equal(responseFeeInput.Transaction.Send.Value), "expected Send.Value %s, got %s", responseFeeInput.Transaction.Send.Value, result.Transaction.Send.Value)
				assert.Equal(t, responseFeeInput.LedgerID, result.LedgerID)

				// Deterministic value assertions on the mutated result.
				// Fee "iof": 600% of 100 = 600 (deductible, priority 1)
				// Fee "taxaAdmin": max(600% of afterFees, flat 1600) = 1600 (non-deductible, priority 2)
				// Note: From/To order is non-deterministic (comes from map iteration),
				// so we collect all amounts and verify expected values exist.
				assert.NotEmpty(t, result.Transaction.Send.Source.From, "From should not be empty")
				assert.NotEmpty(t, result.Transaction.Send.Distribute.To, "To should not be empty")

				// Verify expected fee values exist in From (non-deductible fee adds to From)
				fromValues := make([]string, 0)
				for _, f := range result.Transaction.Send.Source.From {
					if f.Amount != nil {
						fromValues = append(fromValues, f.Amount.Value.String())
					}
				}
				assert.Contains(t, fromValues, "1600", "From should contain non-deductible fee 1600, got %v", fromValues)
				assert.Contains(t, fromValues, "50", "From should contain account share 50, got %v", fromValues)

				// Verify expected fee values exist in To (deductible fee + non-deductible fee)
				toValues := make([]string, 0)
				for _, to := range result.Transaction.Send.Distribute.To {
					if to.Amount != nil {
						toValues = append(toValues, to.Amount.Value.String())
					}
				}
				assert.Contains(t, toValues, "600", "To should contain deductible fee 600, got %v", toValues)
				assert.Contains(t, toValues, "1600", "To should contain non-deductible fee 1600, got %v", toValues)
			}
		})
	}
}

// TestEstimateFeeCalculation_PackageNotFound_MongoErrNoDocuments tests when package is not found (mongo.ErrNoDocuments)
func TestEstimateFeeCalculation_PackageNotFound_MongoErrNoDocuments(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockPackRepo := pack.NewMockRepository(ctrl)
	orgID := uuid.New()
	packID := uuid.New()
	ledgerID := uuid.New()

	feeSvc := &UseCase{
		packageRepo: mockPackRepo,
	}

	feeEstimate := &model.FeeEstimate{
		PackageID: packID,
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
		FindByID(gomock.Any(), packID, orgID).
		Return(nil, mongo.ErrNoDocuments)

	ctx := context.Background()
	result, err := feeSvc.EstimateFeeCalculation(ctx, feeEstimate, orgID)
	assert.Error(t, err)
	assert.Nil(t, result)
	// Verify if the error contains "FEE-0012" or the entity not found message
	assert.True(t,
		errors.Is(err, constant.ErrEntityNotFound) ||
			errors.As(err, &pkg.EntityNotFoundError{}) ||
			strings.Contains(err.Error(), "FEE-0012") ||
			strings.Contains(err.Error(), "No Package entity was found"))
}

// TestEstimateFeeCalculation_PackageNotFound_OtherError tests when another error occurs while searching for package
func TestEstimateFeeCalculation_PackageNotFound_OtherError(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockPackRepo := pack.NewMockRepository(ctrl)
	orgID := uuid.New()
	packID := uuid.New()
	ledgerID := uuid.New()

	feeSvc := &UseCase{
		packageRepo: mockPackRepo,
	}

	feeEstimate := &model.FeeEstimate{
		PackageID: packID,
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

	otherError := errors.New("database connection error")
	mockPackRepo.EXPECT().
		FindByID(gomock.Any(), packID, orgID).
		Return(nil, otherError)

	ctx := context.Background()
	result, err := feeSvc.EstimateFeeCalculation(ctx, feeEstimate, orgID)
	assert.Error(t, err)
	assert.Nil(t, result)
	assert.Equal(t, otherError, err)
}

// TestEstimateFeeCalculation_ValidationError tests error in transaction validation
func TestEstimateFeeCalculation_ValidationError(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockPackRepo := pack.NewMockRepository(ctrl)
	orgID := uuid.New()
	packID := uuid.New()
	ledgerID := uuid.New()

	feeSvc := &UseCase{
		packageRepo: mockPackRepo,
	}

	feeEstimate := &model.FeeEstimate{
		PackageID: packID,
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
		FindByID(gomock.Any(), packID, orgID).
		Return(packEntity, nil)

	ctx := context.Background()
	result, err := feeSvc.EstimateFeeCalculation(ctx, feeEstimate, orgID)
	assert.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), constant.ErrMissingFieldsInRequest.Error())
}

// TestEstimateFeeCalculation_ValueBelowMinimum tests when value is below minimum
func TestEstimateFeeCalculation_ValueBelowMinimum(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockPackRepo := pack.NewMockRepository(ctrl)
	orgID := uuid.New()
	packID := uuid.New()
	ledgerID := uuid.New()

	feeSvc := &UseCase{
		packageRepo: mockPackRepo,
	}

	feeEstimate := &model.FeeEstimate{
		PackageID: packID,
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
		FindByID(gomock.Any(), packID, orgID).
		Return(packEntity, nil)

	ctx := context.Background()
	result, err := feeSvc.EstimateFeeCalculation(ctx, feeEstimate, orgID)
	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, feeEstimate.LedgerID, result.LedgerID)
	assert.Nil(t, result.Transaction.Metadata)
}

// TestEstimateFeeCalculation_ValueAboveMaximum tests when value is above maximum
func TestEstimateFeeCalculation_ValueAboveMaximum(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockPackRepo := pack.NewMockRepository(ctrl)
	orgID := uuid.New()
	packID := uuid.New()
	ledgerID := uuid.New()

	feeSvc := &UseCase{
		packageRepo: mockPackRepo,
	}

	feeEstimate := &model.FeeEstimate{
		PackageID: packID,
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
		FindByID(gomock.Any(), packID, orgID).
		Return(packEntity, nil)

	ctx := context.Background()
	result, err := feeSvc.EstimateFeeCalculation(ctx, feeEstimate, orgID)
	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, feeEstimate.LedgerID, result.LedgerID)
	assert.Nil(t, result.Transaction.Metadata)
}

// TestEstimateFeeCalculation_CalculateFeeError tests error when calculating fee
func TestEstimateFeeCalculation_CalculateFeeError(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockPackRepo := pack.NewMockRepository(ctrl)
	orgID := uuid.New()
	packID := uuid.New()
	ledgerID := uuid.New()

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

	feeEstimate := &model.FeeEstimate{
		PackageID: packID,
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
		FindByID(gomock.Any(), packID, orgID).
		Return(packEntity, nil)

	ctx := context.Background()
	result, err := feeSvc.EstimateFeeCalculation(ctx, feeEstimate, orgID)
	assert.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "FEE-0044")
}

// TestEstimateFeeCalculation_NoFeeApplied tests when fee is not applied (package without fees)
func TestEstimateFeeCalculation_NoFeeApplied(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockPackRepo := pack.NewMockRepository(ctrl)
	orgID := uuid.New()
	packID := uuid.New()
	ledgerID := uuid.New()

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

	feeEstimate := &model.FeeEstimate{
		PackageID: packID,
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

	// Package without fees - when there are no fees, the From/To arrays don't change
	packEntity := &pack.Package{
		ID:             packID,
		MinimumAmount:  decimal.NewFromInt(100),
		MaximumAmount:  decimal.NewFromInt(2000),
		Fees:           map[string]model.Fee{},
		WaivedAccounts: &[]string{},
	}

	mockPackRepo.EXPECT().
		FindByID(gomock.Any(), packID, orgID).
		Return(packEntity, nil)

	ctx := context.Background()
	result, err := feeSvc.EstimateFeeCalculation(ctx, feeEstimate, orgID)
	assert.NoError(t, err)
	assert.NotNil(t, result)
	// When there are no fees, the From/To arrays don't change, so metadata is not created
	assert.Nil(t, result.Transaction.Metadata)
}

// TestEstimateFeeCalculation_Success_WithMetadata tests success with fee applied and metadata created
func TestEstimateFeeCalculation_Success_WithMetadata(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockPackRepo := pack.NewMockRepository(ctrl)
	orgID := uuid.New()
	packID := uuid.New()
	ledgerID := uuid.New()
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

	feeEstimate := &model.FeeEstimate{
		PackageID: packID,
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
		FindByID(gomock.Any(), packID, orgID).
		Return(packEntity, nil)

	ctx := context.Background()
	result, err := feeSvc.EstimateFeeCalculation(ctx, feeEstimate, orgID)
	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.NotNil(t, result.Transaction.Metadata)
	assert.Equal(t, packID.String(), result.Transaction.Metadata["packageAppliedID"])
}

// TestEstimateFeeCalculation_Success_WithExistingMetadata tests success with existing metadata
func TestEstimateFeeCalculation_Success_WithExistingMetadata(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockPackRepo := pack.NewMockRepository(ctrl)
	orgID := uuid.New()
	packID := uuid.New()
	ledgerID := uuid.New()
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

	feeEstimate := &model.FeeEstimate{
		PackageID: packID,
		LedgerID:  ledgerID,
		Transaction: transaction.Transaction{
			Metadata: map[string]any{
				"existingKey": "existingValue",
			},
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
		FindByID(gomock.Any(), packID, orgID).
		Return(packEntity, nil)

	ctx := context.Background()
	result, err := feeSvc.EstimateFeeCalculation(ctx, feeEstimate, orgID)
	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.NotNil(t, result.Transaction.Metadata)
	assert.Equal(t, packID.String(), result.Transaction.Metadata["packageAppliedID"])
	assert.Equal(t, "existingValue", result.Transaction.Metadata["existingKey"])
}

// TestEstimateFeeCalculation_ValueAtMinimum tests value at minimum limit
func TestEstimateFeeCalculation_ValueAtMinimum(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockPackRepo := pack.NewMockRepository(ctrl)
	orgID := uuid.New()
	packID := uuid.New()
	ledgerID := uuid.New()
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

	feeEstimate := &model.FeeEstimate{
		PackageID: packID,
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
		FindByID(gomock.Any(), packID, orgID).
		Return(packEntity, nil)

	ctx := context.Background()
	result, err := feeSvc.EstimateFeeCalculation(ctx, feeEstimate, orgID)
	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.NotNil(t, result.Transaction.Metadata)
	assert.Equal(t, packID.String(), result.Transaction.Metadata["packageAppliedID"])
}

// TestEstimateFeeCalculation_ValueAtMaximum tests value at maximum limit
func TestEstimateFeeCalculation_ValueAtMaximum(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockPackRepo := pack.NewMockRepository(ctrl)
	orgID := uuid.New()
	packID := uuid.New()
	ledgerID := uuid.New()
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

	feeEstimate := &model.FeeEstimate{
		PackageID: packID,
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
		FindByID(gomock.Any(), packID, orgID).
		Return(packEntity, nil)

	ctx := context.Background()
	result, err := feeSvc.EstimateFeeCalculation(ctx, feeEstimate, orgID)
	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.NotNil(t, result.Transaction.Metadata)
	assert.Equal(t, packID.String(), result.Transaction.Metadata["packageAppliedID"])
}
