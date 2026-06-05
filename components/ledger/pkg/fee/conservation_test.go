// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package fee

import (
	"strconv"
	"strings"
	"testing"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/mongodb/fees/pack"
	"github.com/LerianStudio/midaz/v4/components/ledger/pkg/feeshared/constant"
	"github.com/LerianStudio/midaz/v4/components/ledger/pkg/feeshared/model"

	libZap "github.com/LerianStudio/lib-observability/zap"
	transaction "github.com/LerianStudio/midaz/v4/pkg/mtransaction"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// This file freezes the FORWARD leg-sum conservation invariant of the fee
// distribution engine (P2a-T17, third-rail de-risking for the Phase 4 embed).
//
// Invariant under test:
//
//	sum(distributed fee legs) == fee total   (decimal.Equal, ZERO tolerance)
//
// The fee total is the UNROUNDED per-fee amount: the ISO-4217 precision table
// is deleted (P4-T11) and the engine emits legs at full decimal precision. The
// invariant is therefore proven INDEPENDENT of any precision rule — it holds
// solely because applyFeeCorrection reconciles the division residual onto the
// max account's leg. The distributed legs are the entries the engine appends to
// resp.From (non-deductible fees) or resp.To (deductible fees) whose synthetic
// key contains "fee". For non-deductible fees the engine also mirrors a
// credit-side "fee_source" leg into resp.To; double-entry requires the debit
// legs and the credit legs to each sum to the same total.
//
// The "asset precision" axis in the matrix below is now just a set of
// transaction assets (JPY/0, BRL/2, KWD/3, BTC/8, ETH/18) carried as plain asset
// codes — they are NOT precision-table keys anymore; the matrix proves
// conservation holds across them with the table gone.
//
// The matrix stresses applyFeeCorrection's residual reconciliation across asset,
// account count, fee shape, deductibility, and repeating-decimal proportions.
//
// boolPtr returns a pointer to b (model.Fee.IsDeductibleFrom is *bool).
func boolPtr(b bool) *bool { return &b }

// conservationFixture builds the inputs CalculateFee mutates: a FeeCalculate
// whose Send carries one From account and N To accounts, and a Responses whose
// From/To maps mirror them. fromValues / toValues are the per-account amounts
// (length = account count on each side). The package carries exactly one fee.
type conservationFixture struct {
	asset      string
	sendValue  decimal.Decimal
	fromValues []decimal.Decimal
	toValues   []decimal.Decimal
	fee        model.Fee
	routeFrom  *string
	routeTo    *string
}

func (cf conservationFixture) build() (*model.FeeCalculate, *pack.Package, *transaction.Responses) {
	fromLegs := make([]transaction.FromTo, 0, len(cf.fromValues))
	respFrom := make(map[string]transaction.Amount, len(cf.fromValues))

	for i, v := range cf.fromValues {
		alias := "@from_" + string(rune('a'+i))
		fromLegs = append(fromLegs, transaction.FromTo{
			AccountAlias: alias,
			Amount:       &transaction.Amount{Asset: cf.asset, Value: v},
		})
		respFrom[alias] = transaction.Amount{Asset: cf.asset, Value: v}
	}

	toLegs := make([]transaction.FromTo, 0, len(cf.toValues))
	respTo := make(map[string]transaction.Amount, len(cf.toValues))

	for i, v := range cf.toValues {
		alias := "@to_" + string(rune('a'+i))
		toLegs = append(toLegs, transaction.FromTo{
			AccountAlias: alias,
			Amount:       &transaction.Amount{Asset: cf.asset, Value: v},
		})
		respTo[alias] = transaction.Amount{Asset: cf.asset, Value: v}
	}

	feeWithRoutes := cf.fee
	feeWithRoutes.RouteFrom = cf.routeFrom
	feeWithRoutes.RouteTo = cf.routeTo

	feeCalc := &model.FeeCalculate{
		Transaction: transaction.Transaction{
			Send: transaction.Send{
				Asset:      cf.asset,
				Value:      cf.sendValue,
				Source:     transaction.Source{From: fromLegs},
				Distribute: transaction.Distribute{To: toLegs},
			},
		},
	}

	p := &pack.Package{
		MinimumAmount:  decimal.Zero,
		MaximumAmount:  cf.sendValue.Mul(decimal.NewFromInt(1000)),
		Fees:           map[string]model.Fee{"fee_under_test": feeWithRoutes},
		WaivedAccounts: &[]string{},
	}

	resp := &transaction.Responses{From: respFrom, To: respTo}

	return feeCalc, p, resp
}

// sumFeeLegs sums the values of every entry whose key contains "fee", excluding
// "fee_source" mirror legs unless includeSource is true. This isolates the
// debit-side distributed fee legs (feeKey->routeFrom) from the credit-side
// fee_source mirror.
func sumFeeLegs(m map[string]transaction.Amount, includeSource bool) decimal.Decimal {
	total := decimal.Zero

	for key, amt := range m {
		if !strings.Contains(key, "fee") {
			continue
		}

		isSource := strings.Contains(key, "fee_source")
		if isSource && !includeSource {
			continue
		}

		if !isSource && includeSource {
			continue
		}

		total = total.Add(amt.Value)
	}

	return total
}

// expectedFeeTotal re-derives the fee total the same way CalculateFee does:
// compute by application rule, UNROUNDED. The ISO-4217 precision table is
// deleted (P4-T11) and no per-leg or per-total asset-scale rounding is applied;
// the residual-to-max reconciliation in applyFeeCorrection holds sum(legs) ==
// this total exactly under decimal.Equal, independent of any precision rule.
// Single-fee packages only.
func expectedFeeTotal(t *testing.T, f model.Fee, sendValue decimal.Decimal, _ string) decimal.Decimal {
	t.Helper()

	switch f.CalculationModel.ApplicationRule {
	case constant.AppRuleFlatFee:
		v, err := decimal.NewFromString(f.CalculationModel.Calculations[0].Value)
		require.NoError(t, err)

		return v
	case constant.AppRulePercentual:
		pct, err := decimal.NewFromString(f.CalculationModel.Calculations[0].Value)
		require.NoError(t, err)

		return sendValue.Mul(pct.Div(decimal.NewFromInt(100)))
	case constant.AppRuleMaxBetweenTypes:
		var maxVal decimal.Decimal

		for _, c := range f.CalculationModel.Calculations {
			v, err := decimal.NewFromString(c.Value)
			require.NoError(t, err)

			if c.Type == constant.FeeTypePercentage {
				v = sendValue.Mul(v.Div(decimal.NewFromInt(100)))
			}

			if v.GreaterThan(maxVal) {
				maxVal = v
			}
		}

		return maxVal
	default:
		t.Fatalf("unsupported application rule %q", f.CalculationModel.ApplicationRule)

		return decimal.Zero
	}
}

// flatFee / pctFee / maxFee construct single-fee models with the given shape.
func flatFee(value string, deductible bool) model.Fee {
	return model.Fee{
		FeeLabel:         "FlatFee",
		Priority:         1,
		ReferenceAmount:  constant.ReferenceAmountOriginalAmount,
		IsDeductibleFrom: boolPtr(deductible),
		CreditAccount:    "@fee_credit",
		CalculationModel: &model.CalculationModel{
			ApplicationRule: constant.AppRuleFlatFee,
			Calculations:    []model.Calculation{{Type: constant.FeeTypeFlat, Value: value}},
		},
	}
}

func pctFee(value string, deductible bool) model.Fee {
	return model.Fee{
		FeeLabel:         "PctFee",
		Priority:         1,
		ReferenceAmount:  constant.ReferenceAmountOriginalAmount,
		IsDeductibleFrom: boolPtr(deductible),
		CreditAccount:    "@fee_credit",
		CalculationModel: &model.CalculationModel{
			ApplicationRule: constant.AppRulePercentual,
			Calculations:    []model.Calculation{{Type: constant.FeeTypePercentage, Value: value}},
		},
	}
}

func maxFee(flatValue, pctValue string, deductible bool) model.Fee {
	return model.Fee{
		FeeLabel:         "MaxFee",
		Priority:         1,
		ReferenceAmount:  constant.ReferenceAmountOriginalAmount,
		IsDeductibleFrom: boolPtr(deductible),
		CreditAccount:    "@fee_credit",
		CalculationModel: &model.CalculationModel{
			ApplicationRule: constant.AppRuleMaxBetweenTypes,
			Calculations: []model.Calculation{
				{Type: constant.FeeTypeFlat, Value: flatValue},
				{Type: constant.FeeTypePercentage, Value: pctValue},
			},
		},
	}
}

// evenSplit returns n copies of total/n (un-rounded decimal division), so the
// per-account amounts force the proportional distribution to recombine to the
// whole. For n=3 this yields repeating decimals (1/3), exercising the
// Ceil-max/Floor-others rounding path in calculateProportionalFees.
func evenSplit(total decimal.Decimal, n int) []decimal.Decimal {
	out := make([]decimal.Decimal, n)
	per := total.Div(decimal.NewFromInt(int64(n)))

	for i := range out {
		out[i] = per
	}

	return out
}

// assetByPrecision maps each precision under test to a representative asset code.
var assetByPrecision = map[int32]string{
	0:  "JPY",
	2:  "BRL",
	3:  "KWD",
	8:  "BTC",
	18: "ETH",
}

// conservationMatrix enumerates the stress matrix shared by the deductible and
// non-deductible conservation tests: asset precision {0,2,3,8,18} x account
// count {1,2,3,5,7} x fee shape {flat, percentual, maxBetween}.
type matrixCase struct {
	name      string
	asset     string
	precision int32
	accounts  int
	shapeName string
	fee       func(deductible bool) model.Fee
}

func conservationMatrix() []matrixCase {
	precisions := []int32{0, 2, 3, 8, 18}
	accountCounts := []int{1, 2, 3, 5, 7}

	shapes := []struct {
		name string
		fee  func(bool) model.Fee
	}{
		{"flat", func(d bool) model.Fee { return flatFee("100", d) }},
		{"percentual_10pct", func(d bool) model.Fee { return pctFee("10", d) }},
		{"percentual_3.33pct", func(d bool) model.Fee { return pctFee("3.33", d) }},
		{"maxBetween", func(d bool) model.Fee { return maxFee("50", "7.5", d) }},
	}

	out := make([]matrixCase, 0, len(precisions)*len(accountCounts)*len(shapes))

	for _, prec := range precisions {
		asset := assetByPrecision[prec]

		for _, n := range accountCounts {
			for _, sh := range shapes {
				out = append(out, matrixCase{
					name:      asset + "_p" + strconv.Itoa(int(prec)) + "_n" + strconv.Itoa(n) + "_" + sh.name,
					asset:     asset,
					precision: prec,
					accounts:  n,
					shapeName: sh.name,
					fee:       sh.fee,
				})
			}
		}
	}

	return out
}

// baseSendValue is the transaction Send value all conservation cases use; chosen
// so percentual and 1/3-split distributions produce non-trivial rounding
// residuals that stress applyFeeCorrection.
var baseSendValue = decimal.NewFromInt(1000)

// TestConservation_NonDeductible_LegSumEqualsFeeTotal is the FROZEN forward
// conservation gate for NON-DEDUCTIBLE fees (P2a-T17). For every (precision x
// account count x shape) it asserts, under decimal.Equal with ZERO tolerance:
//
//	sum(debit ->feeN-> legs in resp.From)            == rounded fee total
//	sum(credit ->fee_sourceN-> legs in resp.To)      == rounded fee total
//
// i.e. debit == credit == feeTotal (double-entry). This holds universally for
// non-deductible fees because applyFeeCorrection reconciles the rounding
// residual onto the max account's leg on BOTH the From-struct and To-struct
// maps. Phase 4 depends on this invariant.
func TestConservation_NonDeductible_LegSumEqualsFeeTotal(t *testing.T) {
	t.Parallel()

	logger, _ := libZap.New(libZap.Config{Environment: libZap.EnvironmentLocal, OTelLibraryName: "test"})

	for _, mc := range conservationMatrix() {
		mc := mc

		t.Run(mc.name, func(t *testing.T) {
			t.Parallel()

			f := mc.fee(false)

			cf := conservationFixture{
				asset:      mc.asset,
				sendValue:  baseSendValue,
				fromValues: evenSplit(baseSendValue, mc.accounts),
				toValues:   []decimal.Decimal{baseSendValue},
				fee:        f,
			}

			feeCalc, p, resp := cf.build()

			err := CalculateFee(logger, feeCalc, p, resp, mc.asset, nil)
			require.NoError(t, err, "CalculateFee must not error")

			want := expectedFeeTotal(t, f, baseSendValue, mc.asset)

			gotDebit := sumFeeLegs(resp.From, false)
			assert.Truef(t, gotDebit.Equal(want),
				"DEBIT leg conservation broken: sum(legs)=%s want feeTotal=%s (asset=%s prec=%d n=%d shape=%s)",
				gotDebit.String(), want.String(), mc.asset, mc.precision, mc.accounts, mc.shapeName)

			gotCredit := sumFeeLegs(resp.To, true)
			assert.Truef(t, gotCredit.Equal(want),
				"CREDIT (fee_source) leg conservation broken: sum=%s want feeTotal=%s (asset=%s prec=%d n=%d shape=%s)",
				gotCredit.String(), want.String(), mc.asset, mc.precision, mc.accounts, mc.shapeName)
		})
	}
}

// TestConservation_Deductible_LegSumEqualsFeeTotal is the FROZEN forward
// conservation gate for DEDUCTIBLE fees (P2a-T17). For every (precision x
// account count x shape) it asserts, under decimal.Equal with ZERO tolerance at
// the asset scale:
//
//	sum(fee_source legs in resp.To) == rounded fee total
//
// Deductible fees emit only the fee_source legs (the deduction credited to the
// fee account); the rounding residual is reconciled onto the max account's
// fee_source leg by applyFeeCorrection, with the same delta subtracted from the
// max account's reduced balance so the internal debit==credit identity holds.
//
// This was previously QUARANTINED: before the fix, applyFeeCorrection's residual
// reconciliation was structurally dead on the deductible path (its string-based
// key matching never matched the deductible leg shape and the To-struct map was
// nil), so the residual was silently dropped and 34/100 combos under- or
// over-distributed the fee. The fix wires the reconciliation to the deductible
// leg via structurally-captured keys (feeCorrectionTarget) and quantizes the
// residual to the asset scale. See distribute.go applyFeeCorrection.
func TestConservation_Deductible_LegSumEqualsFeeTotal(t *testing.T) {
	t.Parallel()

	logger, _ := libZap.New(libZap.Config{Environment: libZap.EnvironmentLocal, OTelLibraryName: "test"})

	for _, mc := range conservationMatrix() {
		mc := mc

		t.Run(mc.name, func(t *testing.T) {
			t.Parallel()

			f := mc.fee(true)

			cf := conservationFixture{
				asset:      mc.asset,
				sendValue:  baseSendValue,
				fromValues: []decimal.Decimal{baseSendValue},
				toValues:   evenSplit(baseSendValue, mc.accounts),
				fee:        f,
			}

			feeCalc, p, resp := cf.build()

			err := CalculateFee(logger, feeCalc, p, resp, mc.asset, nil)
			require.NoError(t, err, "CalculateFee must not error")

			want := expectedFeeTotal(t, f, baseSendValue, mc.asset)

			// CREDIT side: the fee_source legs credited to the fee account must
			// sum to the fee total.
			gotCredit := sumFeeLegs(resp.To, true)
			assert.Truef(t, gotCredit.Equal(want),
				"DEDUCTIBLE fee_source leg conservation broken: sum=%s want feeTotal=%s (asset=%s prec=%d n=%d shape=%s)",
				gotCredit.String(), want.String(), mc.asset, mc.precision, mc.accounts, mc.shapeName)

			// PAYER side (the other half of the double-entry identity): the total
			// deducted from the payer balances (original to-value minus the reduced
			// balance left in resp.To) must equal the fee total too. Without this,
			// the residual reconciliation could credit the fee account correctly
			// while debiting the wrong amount from a payer — the matrix would stay
			// green on the credit side alone. Asserting it across all N payers is
			// the end-to-end balance proof.
			deducted := decimal.Zero

			for i := range cf.toValues {
				payerKey := "@to_" + string(rune('a'+i))
				deducted = deducted.Add(cf.toValues[i].Sub(resp.To[payerKey].Value))
			}

			assert.Truef(t, deducted.Equal(want),
				"DEDUCTIBLE payer-balance identity broken: total deducted=%s want feeTotal=%s (asset=%s prec=%d n=%d shape=%s)",
				deducted.String(), want.String(), mc.asset, mc.precision, mc.accounts, mc.shapeName)
		})
	}
}

// TestConservation_DeltaDropEdgeCase adversarially probes the applyFeeCorrection
// residual-reconciliation path (distribute.go:350-384). The correction adds the
// rounding residual delta to the max account's leg ONLY IF both the From-side
// and To-struct-side max-account fee keys are found via string Contains/HasPrefix
// matching (distribute.go:360-375). This test constructs inputs designed to
// stress that matching: prefix-overlapping account aliases and route labels
// containing the substring "fee". If the delta is silently dropped, the legs
// will NOT sum to the fee total under decimal.Equal and the assertion fails,
// exposing the unbalancing as a third-rail finding.
func TestConservation_DeltaDropEdgeCase(t *testing.T) {
	t.Parallel()

	logger, _ := libZap.New(libZap.Config{Environment: libZap.EnvironmentLocal, OTelLibraryName: "test"})

	routeFee := "fee" // route label that itself contains "fee" — collision bait
	routeSource := "fee_source_route"
	routeDebit := "debito"
	routeCredit := "credito"

	cases := []struct {
		name       string
		asset      string
		deductible bool
		fromVals   []decimal.Decimal
		toVals     []decimal.Decimal
		fee        model.Fee
		routeFrom  *string
		routeTo    *string
	}{
		{
			// 1/3 split on a 0-precision asset (JPY): residual is a full unit
			// that MUST be reconciled onto the max account; route labels carry
			// "fee" to bait the key-matching loops.
			name:       "jpy_thirds_route_contains_fee",
			asset:      "JPY",
			deductible: false,
			fromVals:   evenSplit(decimal.NewFromInt(1000), 3),
			toVals:     []decimal.Decimal{decimal.NewFromInt(1000)},
			fee:        pctFee("10", false),
			routeFrom:  &routeFee,
			routeTo:    &routeSource,
		},
		{
			// 7-way split, 3-decimal asset (KWD), repeating proportions.
			name:       "kwd_seven_way_repeating",
			asset:      "KWD",
			deductible: false,
			fromVals:   evenSplit(decimal.NewFromInt(1000), 7),
			toVals:     []decimal.Decimal{decimal.NewFromInt(1000)},
			fee:        pctFee("3.33", false),
			routeFrom:  &routeFee,
			routeTo:    &routeFee,
		},
		{
			// Deductible variant: residual reconciliation on the To side.
			name:       "jpy_deductible_thirds",
			asset:      "JPY",
			deductible: true,
			fromVals:   []decimal.Decimal{decimal.NewFromInt(1000)},
			toVals:     evenSplit(decimal.NewFromInt(1000), 3),
			fee:        pctFee("10", true),
			routeFrom:  &routeFee,
			routeTo:    &routeFee,
		},
		{
			// Max account is NOT the lexically-first account: uneven splits so
			// findMaxAccount selects a specific (the largest) account for the
			// residual reconciliation. Benign route labels (the engine must
			// reconcile regardless of where the max account sits).
			name:       "btc_uneven_max_not_first",
			asset:      "BTC",
			deductible: false,
			fromVals: []decimal.Decimal{
				decimal.RequireFromString("1"),
				decimal.RequireFromString("2"),
				decimal.RequireFromString("7"),
			},
			toVals:    []decimal.Decimal{decimal.NewFromInt(10)},
			fee:       pctFee("33.333333", false),
			routeFrom: &routeDebit,
			routeTo:   &routeCredit,
		},
	}

	for _, tc := range cases {
		tc := tc

		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			cf := conservationFixture{
				asset:      tc.asset,
				sendValue:  decimal.NewFromInt(1000),
				fromValues: tc.fromVals,
				toValues:   tc.toVals,
				fee:        tc.fee,
				routeFrom:  tc.routeFrom,
				routeTo:    tc.routeTo,
			}
			if tc.asset == "BTC" {
				cf.sendValue = decimal.NewFromInt(10)
			}

			feeCalc, p, resp := cf.build()

			err := CalculateFee(logger, feeCalc, p, resp, tc.asset, nil)
			require.NoError(t, err)

			want := expectedFeeTotal(t, tc.fee, cf.sendValue, tc.asset)

			// Deductible fees emit only fee_source legs in resp.To; non-deductible
			// emit debit ->feeN-> legs in resp.From plus the fee_source mirror in
			// resp.To. The residual is reconciled onto the max account's leg; if
			// the delta is dropped (distribute.go:375 key-match failure), the sum
			// will not equal the fee total.
			var got decimal.Decimal
			if tc.deductible {
				got = sumFeeLegs(resp.To, true)
			} else {
				got = sumFeeLegs(resp.From, false)
			}

			assert.Truef(t, got.Equal(want),
				"DELTA-DROP: residual not reconciled — sum(legs)=%s want feeTotal=%s (case=%s). "+
					"If unequal, applyFeeCorrection dropped the delta (distribute.go:375 key-match failure).",
				got.String(), want.String(), tc.name)

			// For non-deductible, also assert the credit mirror balances.
			if !tc.deductible {
				gotCredit := sumFeeLegs(resp.To, true)
				assert.Truef(t, gotCredit.Equal(want),
					"DELTA-DROP (credit side): fee_source legs sum=%s want=%s (case=%s)",
					gotCredit.String(), want.String(), tc.name)
			}
		})
	}
}
