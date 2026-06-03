// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package fee

import (
	"strings"
	"testing"

	"github.com/LerianStudio/midaz/v3/components/ledger/internal/adapters/mongodb/fees/pack"
	"github.com/LerianStudio/midaz/v3/components/ledger/pkg/feeshared/constant"
	"github.com/LerianStudio/midaz/v3/components/ledger/pkg/feeshared/model"

	libZap "github.com/LerianStudio/lib-observability/zap"
	transaction "github.com/LerianStudio/midaz/v3/pkg/mtransaction"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// P4-T24 — fee legs MUST be denominated in the transaction's Send.Asset, never
// the global default currency. The ledger validator aggregates per-asset and
// requires sum == 0 under exact decimal.Equal; a fee leg in any asset other
// than Send.Asset would trip ErrTransactionValueMismatch or silently create a
// multi-asset imbalance.

// assertAllFeeLegsUseAsset fails the test if any fee leg (key contains "fee")
// in resp.From or resp.To carries an asset other than wantAsset.
func assertAllFeeLegsUseAsset(t *testing.T, resp *transaction.Responses, wantAsset string) {
	t.Helper()

	checked := 0

	for _, side := range []map[string]transaction.Amount{resp.From, resp.To} {
		for key, amt := range side {
			if !strings.Contains(key, "fee") {
				continue
			}

			checked++

			assert.Equalf(t, wantAsset, amt.Asset,
				"fee leg %q denominated in %q, want Send.Asset %q", key, amt.Asset, wantAsset)
		}
	}

	require.Positivef(t, checked, "no fee legs were emitted to assert denomination on")
}

func denominationFixture(asset string, deductible bool) (*model.FeeCalculate, *pack.Package, *transaction.Responses) {
	send := transaction.Send{
		Asset: asset,
		Value: decimal.NewFromInt(1000),
		Source: transaction.Source{From: []transaction.FromTo{
			{AccountAlias: "@from_a", Amount: &transaction.Amount{Asset: asset, Value: decimal.NewFromInt(1000)}},
		}},
		Distribute: transaction.Distribute{To: []transaction.FromTo{
			{AccountAlias: "@to_a", Amount: &transaction.Amount{Asset: asset, Value: decimal.NewFromInt(1000)}},
		}},
	}

	feeCalc := &model.FeeCalculate{Transaction: transaction.Transaction{Send: send}}

	p := &pack.Package{
		MinimumAmount: decimal.Zero,
		MaximumAmount: decimal.NewFromInt(1000000),
		Fees: map[string]model.Fee{"fee": {
			FeeLabel:         "PctFee",
			Priority:         1,
			ReferenceAmount:  constant.ReferenceAmountOriginalAmount,
			IsDeductibleFrom: boolPtr(deductible),
			CreditAccount:    "@fee_credit",
			CalculationModel: &model.CalculationModel{
				ApplicationRule: constant.AppRulePercentual,
				Calculations:    []model.Calculation{{Type: constant.FeeTypePercentage, Value: "2.5"}},
			},
		}},
		WaivedAccounts: &[]string{},
	}

	resp := &transaction.Responses{
		From: map[string]transaction.Amount{"@from_a": {Asset: asset, Value: decimal.NewFromInt(1000)}},
		To:   map[string]transaction.Amount{"@to_a": {Asset: asset, Value: decimal.NewFromInt(1000)}},
	}

	return feeCalc, p, resp
}

// TestCalculateFee_LegsDenominatedInSendAsset_NotDefaultCurrency proves a USD
// transaction produces USD-denominated fee legs even when the configured
// default currency is BRL — non-deductible and deductible paths.
func TestCalculateFee_LegsDenominatedInSendAsset_NotDefaultCurrency(t *testing.T) {
	t.Parallel()

	logger, _ := libZap.New(libZap.Config{Environment: libZap.EnvironmentLocal, OTelLibraryName: "test"})

	for _, deductible := range []bool{false, true} {
		deductible := deductible

		name := "non_deductible"
		if deductible {
			name = "deductible"
		}

		t.Run(name, func(t *testing.T) {
			t.Parallel()

			feeCalc, p, resp := denominationFixture("USD", deductible)

			// defaultCurrency deliberately differs from Send.Asset (USD).
			err := CalculateFee(logger, feeCalc, p, resp, DefaultCurrencyBRL, nil)
			require.NoError(t, err)

			assertAllFeeLegsUseAsset(t, resp, "USD")

			// The mutated Send and the rebuilt From/To legs must also be USD.
			assert.Equal(t, "USD", feeCalc.Transaction.Send.Asset)

			for _, ft := range feeCalc.Transaction.Send.Source.From {
				if ft.Amount != nil {
					assert.Equalf(t, "USD", ft.Amount.Asset, "From leg %s denominated in %s", ft.AccountAlias, ft.Amount.Asset)
				}
			}

			for _, ft := range feeCalc.Transaction.Send.Distribute.To {
				if ft.Amount != nil {
					assert.Equalf(t, "USD", ft.Amount.Asset, "To leg %s denominated in %s", ft.AccountAlias, ft.Amount.Asset)
				}
			}
		})
	}
}

// TestCalculateFee_EmptySendAssetFallsBackToDefault documents the only path
// where the default currency denominates a leg: when the transaction omits a
// Send.Asset entirely. This is the value-only fallback, not a cross-asset emit.
func TestCalculateFee_EmptySendAssetFallsBackToDefault(t *testing.T) {
	t.Parallel()

	logger, _ := libZap.New(libZap.Config{Environment: libZap.EnvironmentLocal, OTelLibraryName: "test"})

	feeCalc, p, resp := denominationFixture("", false)

	err := CalculateFee(logger, feeCalc, p, resp, DefaultCurrencyBRL, nil)
	require.NoError(t, err)

	// With no Send.Asset, legs fall back to the default currency — they are all
	// in ONE asset (BRL), so the transaction still balances single-asset.
	assertAllFeeLegsUseAsset(t, resp, DefaultCurrencyBRL)
}
