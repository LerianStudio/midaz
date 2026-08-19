// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package services

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/mongodb/fees/pack"
	feeconstant "github.com/LerianStudio/midaz/v4/components/ledger/pkg/feeshared/constant"
	"github.com/LerianStudio/midaz/v4/components/ledger/pkg/feeshared/model"
	transaction "github.com/LerianStudio/midaz/v4/pkg/mtransaction"
)

func TestZeroFeeIsNoOpForCreateAndEstimate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name            string
		applicationRule string
		calculationType string
	}{
		{name: "flat zero", applicationRule: feeconstant.AppRuleFlatFee, calculationType: feeconstant.FeeTypeFlat},
		{name: "percentage zero", applicationRule: feeconstant.AppRulePercentual, calculationType: feeconstant.FeeTypePercentage},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			organizationID := uuid.New()
			ledgerID := uuid.New()
			packageID := uuid.New()
			deductible := false
			feePackage := &pack.Package{
				ID:             packageID,
				MinimumAmount:  decimal.NewFromInt(1),
				MaximumAmount:  decimal.NewFromInt(1000),
				WaivedAccounts: &[]string{},
				Fees: map[string]model.Fee{"zero": {
					CalculationModel: &model.CalculationModel{
						ApplicationRule: tt.applicationRule,
						Calculations: []model.Calculation{{
							Type:  tt.calculationType,
							Value: "0",
						}},
					},
					ReferenceAmount:  "originalAmount",
					Priority:         1,
					IsDeductibleFrom: &deductible,
					CreditAccount:    "@fee-revenue",
				}},
			}

			newTransaction := func() transaction.Transaction {
				return transaction.Transaction{Send: transaction.Send{
					Asset: "USD",
					Value: decimal.NewFromInt(100),
					Source: transaction.Source{From: []transaction.FromTo{{
						AccountAlias: "@payer",
						Amount:       &transaction.Amount{Asset: "USD", Value: decimal.NewFromInt(100)},
						IsFrom:       true,
					}}},
					Distribute: transaction.Distribute{To: []transaction.FromTo{{
						AccountAlias: "@receiver",
						Amount:       &transaction.Amount{Asset: "USD", Value: decimal.NewFromInt(100)},
					}}},
				}}
			}

			t.Run("create", func(t *testing.T) {
				ctrl := gomock.NewController(t)
				repo := pack.NewMockRepository(ctrl)
				repo.EXPECT().
					FindByOrganizationIDAndLedgerID(gomock.Any(), organizationID, ledgerID).
					Return([]*pack.Package{feePackage}, nil)

				input := &model.FeeCalculate{LedgerID: ledgerID, Transaction: newTransaction()}
				err := (&UseCase{packageRepo: repo, defaultCurrency: "USD"}).CalculateFee(context.Background(), input, organizationID)
				require.NoError(t, err)
				assertZeroFeeNoOp(t, input.Transaction.Send, input.Transaction.Metadata)
			})

			t.Run("estimate", func(t *testing.T) {
				ctrl := gomock.NewController(t)
				repo := pack.NewMockRepository(ctrl)
				repo.EXPECT().
					FindByID(gomock.Any(), packageID, organizationID, uuid.Nil).
					Return(feePackage, nil)

				input := &model.FeeEstimate{PackageID: packageID, LedgerID: ledgerID, Transaction: newTransaction()}
				result, err := (&UseCase{packageRepo: repo, defaultCurrency: "USD"}).EstimateFeeCalculation(context.Background(), input, organizationID)
				require.NoError(t, err)
				require.NotNil(t, result)
				assertZeroFeeNoOp(t, result.Transaction.Send, result.Transaction.Metadata)
			})
		})
	}
}

func assertZeroFeeNoOp(t *testing.T, send transaction.Send, metadata map[string]any) {
	t.Helper()

	assert.True(t, decimal.NewFromInt(100).Equal(send.Value))
	require.Len(t, send.Source.From, 1)
	require.Len(t, send.Distribute.To, 1)
	assert.Equal(t, "@payer", send.Source.From[0].AccountAlias)
	assert.Equal(t, "@receiver", send.Distribute.To[0].AccountAlias)
	assert.NotContains(t, metadata, "feeApplied")
	assert.NotContains(t, metadata, "packageAppliedID")
}
