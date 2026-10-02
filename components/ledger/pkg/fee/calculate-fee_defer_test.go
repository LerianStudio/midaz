// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package fee

import (
	"testing"

	libZap "github.com/LerianStudio/lib-observability/v4/zap"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/mongodb/fees/pack"
	feeconstant "github.com/LerianStudio/midaz/v4/components/ledger/pkg/feeshared/constant"
	"github.com/LerianStudio/midaz/v4/components/ledger/pkg/feeshared/model"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	transaction "github.com/LerianStudio/midaz/v4/pkg/mtransaction"
)

func TestCalculateFee_DeferrableFeeMarksBothLegs(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name       string
		deductible bool
		deferrable bool
		wantToken  string
	}{
		{name: "deferrable non-deductible", deferrable: true, wantToken: "0:@from_account"},
		{name: "non-deferrable", wantToken: ""},
		{name: "deferrable deductible", deductible: true, deferrable: true, wantToken: ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			from, to := deferFeeLegs(t, tc.deductible, tc.deferrable)

			tokens := map[string]any{}
			for _, leg := range append(from, to...) {
				if token, ok := leg.Metadata[constant.MetadataKeyFeeDeferPair]; ok {
					tokens[leg.AccountAlias] = token
				}
			}

			if tc.wantToken == "" {
				require.Empty(t, tokens)
				return
			}

			require.Equal(t, map[string]any{"@from_account": tc.wantToken, "@fee_account": tc.wantToken}, tokens)
		})
	}
}

func deferFeeLegs(t *testing.T, deductible, deferrable bool) ([]transaction.FromTo, []transaction.FromTo) {
	t.Helper()

	logger, err := libZap.New(libZap.Config{Environment: libZap.EnvironmentLocal, OTelLibraryName: "test"})
	require.NoError(t, err)

	value := decimal.NewFromInt(1000)
	calc := &model.FeeCalculate{Transaction: transaction.Transaction{Send: transaction.Send{
		Asset: "BRL", Value: value,
		Source:     transaction.Source{From: []transaction.FromTo{{AccountAlias: "@from_account", Amount: &transaction.Amount{Asset: "BRL", Value: value}}}},
		Distribute: transaction.Distribute{To: []transaction.FromTo{{AccountAlias: "@to_account", Amount: &transaction.Amount{Asset: "BRL", Value: value}}}},
	}}}
	fee := model.Fee{
		FeeLabel: "deferrable",
		CalculationModel: &model.CalculationModel{
			ApplicationRule: feeconstant.AppRuleFlatFee,
			Calculations:    []model.Calculation{{Type: feeconstant.FeeTypeFlat, Value: "10"}},
		},
		ReferenceAmount: "originalAmount", Priority: 1, IsDeductibleFrom: &deductible,
		CreditAccount: "@fee_account", Deferrable: &deferrable,
	}
	resp := &transaction.Responses{
		From: map[string]transaction.Amount{"@from_account": {Asset: "BRL", Value: value}},
		To:   map[string]transaction.Amount{"@to_account": {Asset: "BRL", Value: value}},
	}

	require.NoError(t, CalculateFee(logger, calc, &pack.Package{ID: uuid.New(), Fees: map[string]model.Fee{"f": fee}, WaivedAccounts: &[]string{}}, resp, nil))

	return calc.Transaction.Send.Source.From, calc.Transaction.Send.Distribute.To
}
