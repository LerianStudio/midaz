// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package fee

import (
	"strings"
	"testing"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/mongodb/fees/pack"
	"github.com/LerianStudio/midaz/v4/components/ledger/pkg/feeshared/model"
	"github.com/LerianStudio/midaz/v4/pkg"
	"github.com/LerianStudio/midaz/v4/pkg/constant"

	libZap "github.com/LerianStudio/lib-observability/v4/zap"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	transaction "github.com/LerianStudio/midaz/v4/pkg/mtransaction"
)

const bridgeAlias = "@external/BRL"

// nonPayerCase is one CalculateFee call over a send whose legs are given as
// alias/amount pairs, with the listed positions marked as non-payers.
type nonPayerCase struct {
	from      []transaction.FromTo
	to        []transaction.FromTo
	fees      []model.Fee
	waived    []string
	nonPayers []model.NonPayerLeg
}

func leg(alias string, value int64) transaction.FromTo {
	return transaction.FromTo{
		AccountAlias: alias,
		Amount:       &transaction.Amount{Asset: "BRL", Value: decimal.NewFromInt(value)},
	}
}

func asSource(legs ...transaction.FromTo) []transaction.FromTo {
	for i := range legs {
		legs[i].IsFrom = true
	}

	return legs
}

// run builds the Responses maps under the same keys the transaction pipeline
// uses (AmountMapKeys) and calls CalculateFee.
func (c nonPayerCase) run(t *testing.T) (*model.FeeCalculate, *transaction.Responses, error) {
	t.Helper()

	logger, _ := libZap.New(libZap.Config{Environment: libZap.EnvironmentLocal, OTelLibraryName: "test"})

	sendValue := decimal.Zero
	for _, l := range c.from {
		sendValue = sendValue.Add(l.Amount.Value)
	}

	fees := make(map[string]model.Fee, len(c.fees))
	for i, f := range c.fees {
		fees["fee_"+string(rune('a'+i))] = f
	}

	waived := append([]string{}, c.waived...)
	p := &pack.Package{ID: uuid.New(), Fees: fees, WaivedAccounts: &waived}

	feeCalc := &model.FeeCalculate{
		Transaction: transaction.Transaction{
			Send: transaction.Send{
				Asset:      "BRL",
				Value:      sendValue,
				Source:     transaction.Source{From: c.from},
				Distribute: transaction.Distribute{To: c.to},
			},
		},
		NonPayerLegs: c.nonPayers,
	}

	resp := &transaction.Responses{From: amountsByKey(c.from), To: amountsByKey(c.to)}

	err := CalculateFee(logger, feeCalc, p, resp, nil)

	return feeCalc, resp, err
}

func amountsByKey(legs []transaction.FromTo) map[string]transaction.Amount {
	keys := transaction.AmountMapKeys(legs)

	amounts := make(map[string]transaction.Amount, len(legs))
	for i, l := range legs {
		amounts[keys[i]] = transaction.Amount{Asset: l.Amount.Asset, Value: l.Amount.Value}
	}

	return amounts
}

func flatFeeWithPriority(value string, deductible bool, priority int) model.Fee {
	f := flatFee(value, deductible)
	f.Priority = priority

	return f
}

// splitFeeLegs returns the non-fee legs and the fee legs of one rebuilt side.
func splitFeeLegs(legs []transaction.FromTo) (principal, fees []transaction.FromTo) {
	for _, l := range legs {
		if l.Metadata[constant.MetadataKeyFeeLeg] == constant.MetadataValueFeeLeg {
			fees = append(fees, l)

			continue
		}

		principal = append(principal, l)
	}

	return principal, fees
}

func requireLeg(t *testing.T, l transaction.FromTo, alias string, value string) {
	t.Helper()

	require.Equal(t, alias, l.AccountAlias)
	require.Truef(t, decimal.RequireFromString(value).Equal(l.Amount.Value),
		"leg %s: got %s, want %s", alias, l.Amount.Value.String(), value)
}

func requireExemptionReason(t *testing.T, f *model.FeeCalculate, reason string) {
	t.Helper()

	exemption := decodedFeeExemption(t, f.Transaction.Metadata)
	require.Equal(t, true, exemption["exempt"])
	require.Equal(t, reason, exemption["reason"])
	require.Equal(t, exemptionMessages[reason], exemption["message"])
	require.NotEmpty(t, exemption["message"])
}

func requireNoExemption(t *testing.T, f *model.FeeCalculate) {
	t.Helper()

	_, has := f.Transaction.Metadata["feeExemption"]
	require.False(t, has, "feeExemption must not be set when the fee is charged")
}

func TestCalculateFee_NonPayer_NonDeductibleBridgeOnlySourceSkips(t *testing.T) {
	t.Parallel()

	feeCalc, resp, err := nonPayerCase{
		from:      asSource(leg(bridgeAlias, 100)),
		to:        []transaction.FromTo{leg("acc-b", 100)},
		fees:      []model.Fee{flatFee("2", false)},
		nonPayers: []model.NonPayerLeg{{IsFrom: true, Index: 0}},
	}.run(t)
	require.NoError(t, err)

	require.True(t, decimal.NewFromInt(100).Equal(feeCalc.Transaction.Send.Value), "send value must not grow")
	require.Len(t, feeCalc.Transaction.Send.Source.From, 1)
	requireLeg(t, feeCalc.Transaction.Send.Source.From[0], bridgeAlias, "100")
	require.Len(t, feeCalc.Transaction.Send.Distribute.To, 1)
	requireLeg(t, feeCalc.Transaction.Send.Distribute.To[0], "acc-b", "100")
	require.Len(t, resp.From, 1)
	require.Len(t, resp.To, 1)
	requireExemptionReason(t, feeCalc, "cross_ledger_bridge")
}

func TestCalculateFee_NonPayer_DeductibleBridgeOnlySourceStillDeductsFromReceiver(t *testing.T) {
	t.Parallel()

	feeCalc, _, err := nonPayerCase{
		from:      asSource(leg(bridgeAlias, 100)),
		to:        []transaction.FromTo{leg("acc-b", 100)},
		fees:      []model.Fee{flatFee("2", true)},
		nonPayers: []model.NonPayerLeg{{IsFrom: true, Index: 0}},
	}.run(t)
	require.NoError(t, err)

	require.True(t, decimal.NewFromInt(100).Equal(feeCalc.Transaction.Send.Value))
	require.Len(t, feeCalc.Transaction.Send.Source.From, 1)
	requireLeg(t, feeCalc.Transaction.Send.Source.From[0], bridgeAlias, "100")

	principal, fees := splitFeeLegs(feeCalc.Transaction.Send.Distribute.To)
	require.Len(t, principal, 1)
	requireLeg(t, principal[0], "acc-b", "98")
	require.Len(t, fees, 1)
	requireLeg(t, fees[0], "@fee_credit", "2")
	requireNoExemption(t, feeCalc)
}

func TestCalculateFee_NonPayer_DeductibleBridgeOnlyDestinationSkips(t *testing.T) {
	t.Parallel()

	feeCalc, resp, err := nonPayerCase{
		from:      asSource(leg("acc-a", 100)),
		to:        []transaction.FromTo{leg(bridgeAlias, 100)},
		fees:      []model.Fee{flatFee("2", true)},
		nonPayers: []model.NonPayerLeg{{IsFrom: false, Index: 0}},
	}.run(t)
	require.NoError(t, err)

	require.Len(t, feeCalc.Transaction.Send.Source.From, 1)
	requireLeg(t, feeCalc.Transaction.Send.Source.From[0], "acc-a", "100")
	require.Len(t, feeCalc.Transaction.Send.Distribute.To, 1)
	requireLeg(t, feeCalc.Transaction.Send.Distribute.To[0], bridgeAlias, "100")
	require.Len(t, resp.To, 1)
	requireExemptionReason(t, feeCalc, "cross_ledger_bridge")
}

func TestCalculateFee_NonPayer_NonDeductibleOriginPayerStillPays(t *testing.T) {
	t.Parallel()

	feeCalc, _, err := nonPayerCase{
		from:      asSource(leg("acc-a", 100)),
		to:        []transaction.FromTo{leg(bridgeAlias, 100)},
		fees:      []model.Fee{flatFee("2", false)},
		nonPayers: []model.NonPayerLeg{{IsFrom: false, Index: 0}},
	}.run(t)
	require.NoError(t, err)

	require.True(t, decimal.NewFromInt(102).Equal(feeCalc.Transaction.Send.Value))

	principal, fees := splitFeeLegs(feeCalc.Transaction.Send.Source.From)
	require.Len(t, principal, 1)
	requireLeg(t, principal[0], "acc-a", "100")
	require.Len(t, fees, 1)
	requireLeg(t, fees[0], "acc-a", "2")

	toPrincipal, toFees := splitFeeLegs(feeCalc.Transaction.Send.Distribute.To)
	require.Len(t, toPrincipal, 1)
	requireLeg(t, toPrincipal[0], bridgeAlias, "100")
	require.Len(t, toFees, 1)
	requireLeg(t, toFees[0], "@fee_credit", "2")
	requireNoExemption(t, feeCalc)
}

func TestCalculateFee_NonPayer_NonDeductibleMixedSourcesChargeOnlyClient(t *testing.T) {
	t.Parallel()

	feeCalc, _, err := nonPayerCase{
		from:      asSource(leg("acc-b1", 30), leg(bridgeAlias, 70)),
		to:        []transaction.FromTo{leg("acc-b2", 100)},
		fees:      []model.Fee{pctFee("2", false)},
		nonPayers: []model.NonPayerLeg{{IsFrom: true, Index: 1}},
	}.run(t)
	require.NoError(t, err)

	require.True(t, decimal.NewFromInt(102).Equal(feeCalc.Transaction.Send.Value))

	principal, fees := splitFeeLegs(feeCalc.Transaction.Send.Source.From)
	require.Len(t, principal, 2)
	requireLeg(t, principal[0], "acc-b1", "30")
	requireLeg(t, principal[1], bridgeAlias, "70")
	require.Len(t, fees, 1, "only the client source pays")
	requireLeg(t, fees[0], "acc-b1", "2")

	toPrincipal, toFees := splitFeeLegs(feeCalc.Transaction.Send.Distribute.To)
	require.Len(t, toPrincipal, 1)
	requireLeg(t, toPrincipal[0], "acc-b2", "100")
	require.Len(t, toFees, 1)
	requireLeg(t, toFees[0], "@fee_credit", "2")
	requireNoExemption(t, feeCalc)
}

func TestCalculateFee_NonPayer_DeductibleMixedDestinationsDeductOnlyClient(t *testing.T) {
	t.Parallel()

	t.Run("fee below the client receiver's amount", func(t *testing.T) {
		t.Parallel()

		feeCalc, _, err := nonPayerCase{
			from:      asSource(leg("acc-a1", 100)),
			to:        []transaction.FromTo{leg("acc-a2", 30), leg(bridgeAlias, 70)},
			fees:      []model.Fee{flatFee("3", true)},
			nonPayers: []model.NonPayerLeg{{IsFrom: false, Index: 1}},
		}.run(t)
		require.NoError(t, err)

		principal, fees := splitFeeLegs(feeCalc.Transaction.Send.Distribute.To)
		require.Len(t, principal, 2)
		requireLeg(t, principal[0], "acc-a2", "27")
		requireLeg(t, principal[1], bridgeAlias, "70")
		require.Len(t, fees, 1)
		requireLeg(t, fees[0], "@fee_credit", "3")
		requireNoExemption(t, feeCalc)
	})

	t.Run("fee reaching the client receiver's amount is refused", func(t *testing.T) {
		t.Parallel()

		_, resp, err := nonPayerCase{
			from:      asSource(leg("acc-a1", 100)),
			to:        []transaction.FromTo{leg("acc-a2", 30), leg(bridgeAlias, 70)},
			fees:      []model.Fee{flatFee("30", true)},
			nonPayers: []model.NonPayerLeg{{IsFrom: false, Index: 1}},
		}.run(t)
		require.Error(t, err)
		require.True(t, strings.HasPrefix(err.Error(), constant.ErrDeductibleFeeExceedsAmount.Error()),
			"want 0233, got %v", err)
		require.True(t, decimal.NewFromInt(30).Equal(resp.To["acc-a2"].Value))
		require.True(t, decimal.NewFromInt(70).Equal(resp.To[bridgeAlias].Value))
	})
}

func TestCalculateFee_NonPayer_SameAliasClientLegStillPays(t *testing.T) {
	t.Parallel()

	from := asSource(leg(bridgeAlias, 40), leg(bridgeAlias, 60))
	keys := transaction.AmountMapKeys(from)

	feeCalc, resp, err := nonPayerCase{
		from:      from,
		to:        []transaction.FromTo{leg("acc", 100)},
		fees:      []model.Fee{flatFee("2", false)},
		nonPayers: []model.NonPayerLeg{{IsFrom: true, Index: 1}},
	}.run(t)
	require.NoError(t, err)

	clientDebit := keys[0] + "->fee0->"
	require.Contains(t, resp.From, clientDebit)
	require.True(t, decimal.NewFromInt(2).Equal(resp.From[clientDebit].Value),
		"the client leg pays the whole fee, got %s", resp.From[clientDebit].Value.String())

	for key := range resp.From {
		require.False(t, strings.HasPrefix(key, keys[1]+"->"), "the non-payer leg must not pay: %s", key)
	}

	require.True(t, decimal.NewFromInt(60).Equal(resp.From[keys[1]].Value))
	require.True(t, decimal.NewFromInt(102).Equal(feeCalc.Transaction.Send.Value))
	requireNoExemption(t, feeCalc)
}

func TestCalculateFee_NonPayer_WaivedReceiverKeepsPackageReason(t *testing.T) {
	t.Parallel()

	feeCalc, _, err := nonPayerCase{
		from:      asSource(leg(bridgeAlias, 100)),
		to:        []transaction.FromTo{leg("acc-b", 100)},
		fees:      []model.Fee{flatFee("2", true)},
		waived:    []string{"acc-b"},
		nonPayers: []model.NonPayerLeg{{IsFrom: true, Index: 0}},
	}.run(t)
	require.NoError(t, err)

	require.Len(t, feeCalc.Transaction.Send.Distribute.To, 1)
	requireLeg(t, feeCalc.Transaction.Send.Distribute.To[0], "acc-b", "100")
	requireExemptionReason(t, feeCalc, "all_destination_accounts_exempt")
}

func TestCalculateFee_NonPayer_BridgeSkipNeverCombinesWithPackageReason(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		fees []model.Fee
	}{
		{
			name: "bridge skip first",
			fees: []model.Fee{flatFeeWithPriority("2", false, 1), flatFeeWithPriority("3", true, 2)},
		},
		{
			name: "package waiver first",
			fees: []model.Fee{flatFeeWithPriority("3", true, 1), flatFeeWithPriority("2", false, 2)},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			feeCalc, _, err := nonPayerCase{
				from:      asSource(leg(bridgeAlias, 100)),
				to:        []transaction.FromTo{leg("acc-b", 100)},
				fees:      tt.fees,
				waived:    []string{"acc-b"},
				nonPayers: []model.NonPayerLeg{{IsFrom: true, Index: 0}},
			}.run(t)
			require.NoError(t, err)

			require.True(t, decimal.NewFromInt(100).Equal(feeCalc.Transaction.Send.Value))
			require.Len(t, feeCalc.Transaction.Send.Source.From, 1)
			require.Len(t, feeCalc.Transaction.Send.Distribute.To, 1)
			requireExemptionReason(t, feeCalc, "all_destination_accounts_exempt")
		})
	}
}

func TestCalculateFee_NonPayer_PositionOutsideTheSideIsTechnicalError(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		nonPayer model.NonPayerLeg
	}{
		{name: "destination index past the end", nonPayer: model.NonPayerLeg{IsFrom: false, Index: 1}},
		{name: "source index past the end", nonPayer: model.NonPayerLeg{IsFrom: true, Index: 5}},
		{name: "negative index", nonPayer: model.NonPayerLeg{IsFrom: true, Index: -1}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			feeCalc, resp, err := nonPayerCase{
				from:      asSource(leg("acc-a", 100)),
				to:        []transaction.FromTo{leg("acc-b", 100)},
				fees:      []model.Fee{flatFee("2", false)},
				nonPayers: []model.NonPayerLeg{tt.nonPayer},
			}.run(t)
			require.Error(t, err)
			require.False(t, pkg.IsBusinessError(err), "an invalid position is a technical error, got %v", err)

			require.Len(t, resp.From, 1)
			require.Len(t, resp.To, 1)
			require.True(t, decimal.NewFromInt(100).Equal(feeCalc.Transaction.Send.Value))
		})
	}
}

func TestSetFeeExemptionMetadata_BridgeReasonMerge(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		reasons []string
		want    string
	}{
		{name: "bridge alone", reasons: []string{"cross_ledger_bridge"}, want: "cross_ledger_bridge"},
		{name: "bridge twice", reasons: []string{"cross_ledger_bridge", "cross_ledger_bridge"}, want: "cross_ledger_bridge"},
		{name: "package reason replaces bridge", reasons: []string{"cross_ledger_bridge", "all_source_accounts_exempt"}, want: "all_source_accounts_exempt"},
		{name: "bridge never replaces package reason", reasons: []string{"all_destination_accounts_exempt", "cross_ledger_bridge"}, want: "all_destination_accounts_exempt"},
		{name: "bridge never replaces combined reason", reasons: []string{"all_accounts_exempt", "cross_ledger_bridge"}, want: "all_accounts_exempt"},
		{
			name:    "two package reasons still combine around a bridge skip",
			reasons: []string{"all_source_accounts_exempt", "cross_ledger_bridge", "all_destination_accounts_exempt"},
			want:    "all_accounts_exempt",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			f := &model.FeeCalculate{}
			for _, reason := range tt.reasons {
				setFeeExemptionMetadata(f, reason)
			}

			assert.Equal(t, map[string]any{
				"exempt":  true,
				"reason":  tt.want,
				"message": exemptionMessages[tt.want],
			}, decodedFeeExemption(t, f.Transaction.Metadata))
			assert.NotEmpty(t, exemptionMessages[tt.want])
		})
	}
}
