// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package fee

import (
	"strings"
	"testing"

	feeconstant "github.com/LerianStudio/midaz/v4/components/ledger/pkg/feeshared/constant"
	"github.com/LerianStudio/midaz/v4/components/ledger/pkg/feeshared/model"

	libZap "github.com/LerianStudio/lib-observability/v4/zap"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/pkg/constant"
	transaction "github.com/LerianStudio/midaz/v4/pkg/mtransaction"
)

// FuzzConservation_LegSumEqualsFeeTotal_TableDeleted is the P4-T11 property
// gate: across randomized account splits, fee percentages, asset codes, and
// deductibility, the distributed fee legs MUST sum EXACTLY to the unrounded fee
// total under decimal.Equal (ZERO tolerance) — proven WITH THE ISO-4217
// PRECISION TABLE DELETED. The invariant holds solely because
// applyFeeCorrection reconciles the division residual onto the max account's
// leg; it depends on no asset scale and no precision lookup. The asset codes
// fed in are now just transaction asset labels, not precision-table keys.
func FuzzConservation_LegSumEqualsFeeTotal_TableDeleted(f *testing.F) {
	// Seed corpus: (sendValue, numAccounts, pctTimes100, assetIdx, deductible).
	// pctTimes100 lets the fuzzer explore fractional percentages (e.g. 333 -> 3.33%).
	f.Add(int64(1000), uint8(3), uint16(1000), uint8(0), false) // JPY, 1/3 split, 10%
	f.Add(int64(1000), uint8(7), uint16(333), uint8(2), false)  // KWD, 1/7 split, 3.33%
	f.Add(int64(1000), uint8(3), uint16(1000), uint8(0), true)  // JPY deductible
	f.Add(int64(99999), uint8(5), uint16(715), uint8(4), false) // ETH, 5-way, 7.15%
	f.Add(int64(10), uint8(2), uint16(3333), uint8(3), true)    // BTC deductible, 33.33%

	assets := []string{"JPY", "BRL", "KWD", "BTC", "ETH"}

	logger, _ := libZap.New(libZap.Config{Environment: libZap.EnvironmentLocal, OTelLibraryName: "fuzz"})

	f.Fuzz(func(t *testing.T, sendRaw int64, nAccounts uint8, pctTimes100 uint16, assetIdx uint8, deductible bool) {
		// Constrain inputs to a sane, in-domain range.
		if sendRaw <= 0 {
			sendRaw = 1
		}

		n := int(nAccounts)%7 + 1 // 1..7 accounts on the paying side

		pct := decimal.NewFromInt(int64(pctTimes100 % 10001)).Div(decimal.NewFromInt(100)) // 0..100%
		if pct.IsZero() {
			pct = decimal.NewFromInt(1)
		}

		asset := assets[int(assetIdx)%len(assets)]
		sendValue := decimal.NewFromInt(sendRaw)

		fee := model.Fee{
			FeeLabel:         "FuzzFee",
			Priority:         1,
			ReferenceAmount:  feeconstant.ReferenceAmountOriginalAmount,
			IsDeductibleFrom: boolPtr(deductible),
			CreditAccount:    "@fee_credit",
			CalculationModel: &model.CalculationModel{
				ApplicationRule: feeconstant.AppRulePercentual,
				Calculations:    []model.Calculation{{Type: feeconstant.FeeTypePercentage, Value: pct.String()}},
			},
		}

		var cf conservationFixture
		if deductible {
			cf = conservationFixture{
				asset:      asset,
				sendValue:  sendValue,
				fromValues: []decimal.Decimal{sendValue},
				toValues:   evenSplit(sendValue, n),
				fee:        fee,
			}
		} else {
			cf = conservationFixture{
				asset:      asset,
				sendValue:  sendValue,
				fromValues: evenSplit(sendValue, n),
				toValues:   []decimal.Decimal{sendValue},
				fee:        fee,
			}
		}

		feeCalc, p, resp := cf.build()

		err := CalculateFee(logger, feeCalc, p, resp, nil)
		require.NoError(t, err)

		want := expectedFeeTotal(t, fee, sendValue, asset)

		// Conservation: distributed legs sum EXACTLY to the (unrounded) fee total.
		var got decimal.Decimal
		if deductible {
			got = sumFeeLegs(resp.To, true)
		} else {
			got = sumFeeLegs(resp.From, false)
		}

		require.Truef(t, got.Equal(want),
			"conservation broken WITH TABLE DELETED: sum(legs)=%s want feeTotal=%s (asset=%s n=%d pct=%s deductible=%v)",
			got.String(), want.String(), asset, n, pct.String(), deductible)
	})
}

// FuzzConservation_NonPayerLegNeverPays marks one random leg of the paying side
// as a non-payer. Whatever the split, the fee legs still sum exactly to the fee
// total, no fee leg is drawn from the non-payer, and its amount is untouched.
// When the non-payer is the only leg on the paying side, the fee is skipped.
func FuzzConservation_NonPayerLegNeverPays(f *testing.F) {
	// Seed corpus: (sendValue, numAccounts, pctTimes100, nonPayerIdx, deductible).
	f.Add(int64(1000), uint8(3), uint16(1000), uint8(0), false)
	f.Add(int64(1000), uint8(3), uint16(1000), uint8(2), true)
	f.Add(int64(1000), uint8(0), uint16(250), uint8(0), false)
	f.Add(int64(1000), uint8(0), uint16(250), uint8(0), true)
	f.Add(int64(99999), uint8(6), uint16(715), uint8(4), false)
	f.Add(int64(10), uint8(1), uint16(3333), uint8(1), true)

	logger, _ := libZap.New(libZap.Config{Environment: libZap.EnvironmentLocal, OTelLibraryName: "fuzz"})

	f.Fuzz(func(t *testing.T, sendRaw int64, nAccounts uint8, pctTimes100 uint16, nonPayerRaw uint8, deductible bool) {
		if sendRaw <= 0 {
			sendRaw = 1
		}

		n := int(nAccounts)%7 + 1
		nonPayerIdx := int(nonPayerRaw) % n

		pct := decimal.NewFromInt(int64(pctTimes100 % 10001)).Div(decimal.NewFromInt(100))
		if pct.IsZero() {
			pct = decimal.NewFromInt(1)
		}

		sendValue := decimal.NewFromInt(sendRaw)
		fee := pctFee(pct.String(), deductible)

		cf := conservationFixture{asset: "BRL", sendValue: sendValue, fee: fee}
		if deductible {
			cf.fromValues = []decimal.Decimal{sendValue}
			cf.toValues = evenSplit(sendValue, n)
		} else {
			cf.fromValues = evenSplit(sendValue, n)
			cf.toValues = []decimal.Decimal{sendValue}
		}

		feeCalc, p, resp := cf.build()
		feeCalc.NonPayerLegs = []model.NonPayerLeg{{IsFrom: !deductible, Index: nonPayerIdx}}

		payingLegs := feeCalc.Transaction.Send.Source.From
		paying := &resp.From

		if deductible {
			payingLegs = feeCalc.Transaction.Send.Distribute.To
			paying = &resp.To
		}

		nonPayerKey := transaction.AmountMapKeys(payingLegs)[nonPayerIdx]
		nonPayerAmount := (*paying)[nonPayerKey].Value

		err := CalculateFee(logger, feeCalc, p, resp, nil)
		if deductible && err != nil && strings.HasPrefix(err.Error(), constant.ErrDeductibleFeeExceedsAmount.Error()) {
			return
		}

		require.NoError(t, err)

		require.Truef(t, (*paying)[nonPayerKey].Value.Equal(nonPayerAmount),
			"non-payer amount changed: got %s want %s", (*paying)[nonPayerKey].Value.String(), nonPayerAmount.String())

		for key := range *paying {
			require.Falsef(t, strings.HasPrefix(key, nonPayerKey+"->"), "fee leg drawn from the non-payer: %s", key)
			require.Falsef(t, strings.HasSuffix(key, "->"+nonPayerKey+"->"), "fee leg drawn from the non-payer: %s", key)
		}

		var got decimal.Decimal
		if deductible {
			got = sumFeeLegs(resp.To, true)
		} else {
			got = sumFeeLegs(resp.From, false)
		}

		want := expectedFeeTotal(t, fee, sendValue, "BRL")
		if n == 1 {
			want = decimal.Zero
		}

		require.Truef(t, got.Equal(want),
			"conservation broken with a non-payer: sum(legs)=%s want=%s (n=%d nonPayer=%d pct=%s deductible=%v)",
			got.String(), want.String(), n, nonPayerIdx, pct.String(), deductible)
	})
}
