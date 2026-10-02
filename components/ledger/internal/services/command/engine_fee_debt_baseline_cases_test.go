// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	"github.com/LerianStudio/midaz/v4/pkg/mtransaction"
)

// feeDebtFreeCases are executions that owe nothing: no seed, no fee-debt
// metadata, and a deferrable pair only where the route or action ignores it.
// testdata/fee_debt_free_translation.golden.json records their translation as
// produced before fee debts existed.
func feeDebtFreeCases() map[string]EngineTranslationInput {
	organizationID := uuid.MustParse("11111111-1111-4111-8111-111111111111")
	ledgerID := uuid.MustParse("22222222-2222-4222-8222-222222222222")
	transactionID := uuid.MustParse("33333333-3333-4333-8333-333333333333")
	pair := map[string]any{constant.MetadataKeyFeeLeg: constant.MetadataValueFeeLeg, constant.MetadataKeyFeeDeferPair: "0:0#@payer#default"}

	build := func(action, status string, deferPair bool) EngineTranslationInput {
		ten, one := decimal.NewFromInt(10), decimal.NewFromInt(1)
		amount := func(value decimal.Decimal) mtransaction.Amount {
			return mtransaction.Amount{Asset: "USD", Value: value, TransactionType: status}
		}

		feeLeg := func(alias string) mtransaction.FromTo {
			leg := mtransaction.FromTo{AccountAlias: alias, BalanceKey: "default", Metadata: map[string]any{constant.MetadataKeyFeeLeg: constant.MetadataValueFeeLeg}}
			if deferPair {
				leg.Metadata = pair
			}

			return leg
		}

		payerFee, feeCredit := feeLeg("1#@payer#default"), feeLeg("1#@fees#default")
		payerFee.IsFrom = true

		return EngineTranslationInput{
			TransactionID: transactionID, Action: action, TransactionStatus: status,
			TransactionInput: mtransaction.Transaction{Description: "baseline", Metadata: map[string]any{"origin": "client"}, Send: mtransaction.Send{
				Asset: "USD",
				Source: mtransaction.Source{From: []mtransaction.FromTo{
					{AccountAlias: "0#@payer#default", BalanceKey: "default", IsFrom: true, Description: "main"},
					payerFee,
				}},
				Distribute: mtransaction.Distribute{To: []mtransaction.FromTo{
					{AccountAlias: "0#@payee#default", BalanceKey: "default"},
					feeCredit,
				}},
			}},
			Validate: &mtransaction.Responses{
				From: map[string]mtransaction.Amount{"0#@payer#default": amount(ten), "1#@payer#default": amount(one)},
				To:   map[string]mtransaction.Amount{"0#@payee#default": amount(ten), "1#@fees#default": amount(one)},
			},
			Balances: feeDebtFreeBalances(organizationID, ledgerID),
		}
	}

	return map[string]EngineTranslationInput{
		"direct":                build(constant.ActionDirect, constant.CREATED, false),
		"direct_deferrable_v1":  build(constant.ActionDirect, constant.CREATED, true),
		"hold_deferrable":       build(constant.ActionHold, constant.PENDING, true),
		"commit":                build(constant.ActionCommit, constant.APPROVED, false),
		"cancel":                build(constant.ActionCancel, constant.CANCELED, false),
		"revert":                build(constant.ActionRevert, constant.CREATED, false),
		"revert_inherited_pair": build(constant.ActionRevert, constant.CREATED, true),
	}
}

func feeDebtFreeBalances(organizationID, ledgerID uuid.UUID) []*mmodel.Balance {
	payer := translationBalance(organizationID, ledgerID, "44444444-4444-4444-8444-444444444444", "@payer", "default")
	companion := translationBalance(organizationID, ledgerID, "45444444-4444-4444-8444-444444444444", "@payer", constant.OverdraftBalanceKey)
	companion.AccountID = payer.AccountID

	return []*mmodel.Balance{
		payer, companion,
		translationBalance(organizationID, ledgerID, "55555555-5555-4555-8555-555555555555", "@payee", "default"),
		translationBalance(organizationID, ledgerID, "66666666-6666-4666-8666-666666666666", "@fees", "default"),
	}
}
