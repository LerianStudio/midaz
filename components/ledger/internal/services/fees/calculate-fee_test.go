// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package services

import (
	"context"
	"testing"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/mongodb/fees/pack"
	mongoPack "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/mongodb/fees/pack"
	"github.com/LerianStudio/midaz/v4/components/ledger/pkg/feeshared/model"
	"github.com/LerianStudio/midaz/v4/pkg/constant"

	transaction "github.com/LerianStudio/midaz/v4/pkg/mtransaction"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
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

func TestCalculateFee_RemainingSourceIsResolvedBeforeFeeDistribution(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockPackRepo := pack.NewMockRepository(ctrl)
	orgID := uuid.New()
	ledgerID := uuid.New()
	packID := uuid.New()
	deductible := false

	feeSvc := &UseCase{packageRepo: mockPackRepo}
	routeID := uuid.New().String()
	rateID := uuid.New().String()
	explicitMetadata := map[string]any{"leg": "explicit"}
	remainingMetadata := map[string]any{"leg": "remaining"}
	destinationMetadata := map[string]any{"leg": "destination"}
	feeDebitRoute := "fee-debit-route"
	feeCreditRoute := "fee-credit-route"
	feeInput := &model.FeeCalculate{
		LedgerID: ledgerID,
		Transaction: transaction.Transaction{
			Send: transaction.Send{
				Asset: "USD",
				Value: decimal.NewFromInt(100),
				Source: transaction.Source{From: []transaction.FromTo{
					{
						AccountAlias:    "@payer",
						BalanceKey:      "reserved",
						Amount:          &transaction.Amount{Asset: "USD", Value: decimal.NewFromInt(60)},
						Rate:            &transaction.Rate{From: "USD", To: "USDe", Value: decimal.NewFromInt(1), ExternalID: rateID},
						Description:     "explicit leg",
						ChartOfAccounts: "asset.explicit",
						Metadata:        explicitMetadata,
						IsFrom:          true,
						Route:           "legacy-explicit",
						RouteID:         &routeID,
					},
					{
						AccountAlias:    "@payer",
						BalanceKey:      "available",
						Remaining:       "remaining",
						Description:     "remaining leg",
						ChartOfAccounts: "asset.remaining",
						Metadata:        remainingMetadata,
						IsFrom:          true,
						Route:           "legacy-remaining",
					},
				}},
				Distribute: transaction.Distribute{To: []transaction.FromTo{{
					AccountAlias:    "@destination",
					BalanceKey:      "settlement",
					Share:           &transaction.Share{Percentage: 100},
					Description:     "destination leg",
					ChartOfAccounts: "liability.destination",
					Metadata:        destinationMetadata,
					Route:           "legacy-destination",
					RouteID:         &routeID,
				}}},
			},
		},
	}

	feePackage := &pack.Package{
		ID:            packID,
		MinimumAmount: decimal.NewFromInt(1),
		MaximumAmount: decimal.NewFromInt(1000),
		Fees: map[string]model.Fee{
			"flat": {
				CalculationModel: &model.CalculationModel{
					ApplicationRule: "flatFee",
					Calculations:    []model.Calculation{{Type: "flat", Value: "10"}},
				},
				ReferenceAmount:  "originalAmount",
				Priority:         1,
				IsDeductibleFrom: &deductible,
				CreditAccount:    "@fee_account",
				RouteFrom:        &feeDebitRoute,
				RouteTo:          &feeCreditRoute,
			},
		},
		WaivedAccounts: &[]string{},
	}

	mockPackRepo.EXPECT().
		FindByOrganizationIDAndLedgerID(gomock.Any(), orgID, ledgerID).
		Return([]*pack.Package{feePackage}, nil)

	err := feeSvc.CalculateFee(context.Background(), feeInput, orgID)
	assert.NoError(t, err)
	assert.Equal(t, decimal.NewFromInt(110), feeInput.Transaction.Send.Value)
	assert.Len(t, feeInput.Transaction.Send.Source.From, 4, "both original legs and both proportional fee legs must survive")

	explicit := feeInput.Transaction.Send.Source.From[0]
	assert.Equal(t, "@payer", explicit.AccountAlias)
	assert.Equal(t, "reserved", explicit.BalanceKey)
	assert.Equal(t, "explicit leg", explicit.Description)
	assert.Equal(t, "asset.explicit", explicit.ChartOfAccounts)
	assert.Equal(t, explicitMetadata, explicit.Metadata)
	assert.True(t, explicit.IsFrom)
	assert.Equal(t, "legacy-explicit", explicit.Route)
	assert.Equal(t, &routeID, explicit.RouteID)
	assert.Equal(t, &transaction.Rate{From: "USD", To: "USDe", Value: decimal.NewFromInt(1), ExternalID: rateID}, explicit.Rate)
	assert.Nil(t, explicit.Share)
	assert.Empty(t, explicit.Remaining)
	if assert.NotNil(t, explicit.Amount) {
		assert.Equal(t, decimal.NewFromInt(60), explicit.Amount.Value)
	}

	remaining := feeInput.Transaction.Send.Source.From[1]
	assert.Equal(t, "@payer", remaining.AccountAlias)
	assert.Equal(t, "available", remaining.BalanceKey)
	assert.Equal(t, "remaining leg", remaining.Description)
	assert.Equal(t, "asset.remaining", remaining.ChartOfAccounts)
	assert.Equal(t, remainingMetadata, remaining.Metadata)
	assert.True(t, remaining.IsFrom)
	assert.Equal(t, "legacy-remaining", remaining.Route)
	assert.Nil(t, remaining.RouteID)
	assert.Nil(t, remaining.Share)
	assert.Empty(t, remaining.Remaining)
	if assert.NotNil(t, remaining.Amount) {
		assert.Equal(t, decimal.NewFromInt(40), remaining.Amount.Value)
	}

	assert.Len(t, feeInput.Transaction.Send.Distribute.To, 3, "the destination plus both proportional fee credits must survive")
	destination := feeInput.Transaction.Send.Distribute.To[0]
	assert.Equal(t, "@destination", destination.AccountAlias)
	assert.Equal(t, "settlement", destination.BalanceKey)
	assert.Equal(t, "destination leg", destination.Description)
	assert.Equal(t, "liability.destination", destination.ChartOfAccounts)
	assert.Equal(t, destinationMetadata, destination.Metadata)
	assert.Equal(t, "legacy-destination", destination.Route)
	assert.Equal(t, &routeID, destination.RouteID)
	assert.Nil(t, destination.Share)
	assert.Empty(t, destination.Remaining)
	if assert.NotNil(t, destination.Amount) {
		assert.True(t, decimal.NewFromInt(100).Equal(destination.Amount.Value))
	}

	for _, feeLeg := range feeInput.Transaction.Send.Source.From[2:] {
		assert.Equal(t, "@payer", feeLeg.AccountAlias)
		assert.Equal(t, feeDebitRoute, feeLeg.Route)
		assert.Empty(t, feeLeg.BalanceKey, "new fee legs retain the default-balance semantics")
	}

	for _, feeLeg := range feeInput.Transaction.Send.Distribute.To[1:] {
		assert.Equal(t, "@fee_account", feeLeg.AccountAlias)
		assert.Equal(t, feeCreditRoute, feeLeg.Route)
		assert.Equal(t, "@payer", feeLeg.Metadata["source"])
	}

	var sourceTotal, destinationTotal decimal.Decimal
	for _, leg := range feeInput.Transaction.Send.Source.From {
		assert.NotNil(t, leg.Amount, "fee normalization must preserve every resolved source leg")
		if leg.Amount != nil {
			assert.True(t, leg.Amount.Value.IsPositive(), "fee normalization must not emit a zero source leg")
			sourceTotal = sourceTotal.Add(leg.Amount.Value)
		}
	}

	for _, leg := range feeInput.Transaction.Send.Distribute.To {
		assert.NotNil(t, leg.Amount, "fee normalization must preserve every resolved destination leg")
		if leg.Amount != nil {
			assert.True(t, leg.Amount.Value.IsPositive(), "fee normalization must not emit a zero destination leg")
			destinationTotal = destinationTotal.Add(leg.Amount.Value)
		}
	}

	assert.True(t, decimal.NewFromInt(110).Equal(sourceTotal), "source legs must sum to the fee-inclusive amount: %s", sourceTotal)
	assert.True(t, decimal.NewFromInt(110).Equal(destinationTotal), "destination legs must sum to the fee-inclusive amount: %s", destinationTotal)
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
