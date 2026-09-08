// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package fee

import (
	"maps"
	"strings"
	"testing"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/mongodb/fees/pack"
	feeconstant "github.com/LerianStudio/midaz/v4/components/ledger/pkg/feeshared/constant"
	"github.com/LerianStudio/midaz/v4/components/ledger/pkg/feeshared/model"

	libZap "github.com/LerianStudio/lib-observability/v4/zap"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/pkg"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	transaction "github.com/LerianStudio/midaz/v4/pkg/mtransaction"
)

// P4-T24 — fee legs MUST be denominated in the transaction's Send.Asset, the
// single source of truth. The ledger validator aggregates per-asset and
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
			ReferenceAmount:  feeconstant.ReferenceAmountOriginalAmount,
			IsDeductibleFrom: boolPtr(deductible),
			CreditAccount:    "@fee_credit",
			CalculationModel: &model.CalculationModel{
				ApplicationRule: feeconstant.AppRulePercentual,
				Calculations:    []model.Calculation{{Type: feeconstant.FeeTypePercentage, Value: "2.5"}},
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

// TestCalculateFee_LegsDenominatedInSendAsset_NotAmbientCurrency asserts every
// fee leg — plus the mutated Send and the rebuilt From/To legs — is denominated
// in the transaction's Send.Asset (P4-T24), on both the non-deductible and the
// deductible path. The fixture deliberately uses USD rather than BRL so that a
// leg picking up an ambient or hardcoded currency instead of Send.Asset fails
// the assertion rather than passing by coincidence.
func TestCalculateFee_LegsDenominatedInSendAsset_NotAmbientCurrency(t *testing.T) {
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

			err := CalculateFee(logger, feeCalc, p, resp, nil)
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

// TestCalculateFee_EmptySendAssetRejected records the decision that a Send.Asset
// that is empty or whitespace-only is a hard 0009 rejection naming send.asset:
// the engine refuses the calculation and leaves resp.From, resp.To and
// Send.Value untouched, so the caller must name the asset every fee leg is
// denominated in.
func TestCalculateFee_EmptySendAssetRejected(t *testing.T) {
	t.Parallel()

	logger, _ := libZap.New(libZap.Config{Environment: libZap.EnvironmentLocal, OTelLibraryName: "test"})

	tests := []struct {
		name  string
		asset string
	}{
		{name: "empty asset", asset: ""},
		{name: "blank asset", asset: "  "},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			feeCalc, p, resp := denominationFixture(tt.asset, false)

			fromBefore := maps.Clone(resp.From)
			toBefore := maps.Clone(resp.To)

			err := CalculateFee(logger, feeCalc, p, resp, nil)
			require.Error(t, err)

			var validationErr pkg.ValidationError
			require.ErrorAs(t, err, &validationErr)

			// Wire-code lock: 0009 is an external API surface, not an internal label.
			assert.Equal(t, "0009", validationErr.Code)
			assert.Equal(t, constant.EntityFeeCalculation, validationErr.EntityType)
			assert.Contains(t, validationErr.Message, "send.asset",
				"the rejection must name the missing field so the caller can fix the request")

			// The rejection short-circuits before any leg emission: no fee leg was
			// appended, the pre-existing legs are untouched, and Send.Value — the
			// money field CalculateFee mutates in place — still holds its fixture
			// value.
			assert.Equal(t, fromBefore, resp.From)
			assert.Equal(t, toBefore, resp.To)
			assert.Truef(t, feeCalc.Transaction.Send.Value.Equal(decimal.NewFromInt(1000)),
				"Send.Value mutated to %s, want the untouched fixture value 1000", feeCalc.Transaction.Send.Value)
		})
	}
}
