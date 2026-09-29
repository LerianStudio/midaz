// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package transaction

import (
	"testing"

	constant "github.com/LerianStudio/lib-commons/v7/commons/constants"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/operation"
	pkgConstant "github.com/LerianStudio/midaz/v4/pkg/constant"
)

func TestTransactionRevert_FoldsFeeSettlements(t *testing.T) {
	t.Parallel()

	paid := func(debtor, creditor string, value int64) []*operation.Operation {
		return []*operation.Operation{
			feeDebtRow(pkgConstant.FEE_SETTLEMENT, debtor, pkgConstant.DirectionDebit, value),
			feeDebtRow(pkgConstant.FEE_SETTLEMENT, creditor, pkgConstant.DirectionCredit, value),
		}
	}
	credit := func(alias string, value int64) *operation.Operation {
		return feeDebtRow(constant.CREDIT, alias, "", value)
	}
	routed := func(route string, rows ...*operation.Operation) []*operation.Operation {
		for _, row := range rows {
			row.RouteID = ptr(route)
		}

		return rows
	}

	for _, tc := range []struct {
		name    string
		credits []*operation.Operation
		rows    []*operation.Operation
		kept    map[FeeSettlementGroup]decimal.Decimal
		froms   map[string]int64
	}{
		{
			name:    "a full settlement folds the debtor leg to zero and takes back from the creditor",
			credits: []*operation.Operation{credit("@debtor", 20)},
			rows:    paid("@debtor", "@fees", 20),
			froms:   map[string]int64{"@fees": 20},
		},
		{
			name:    "a partial settlement splits the take-back",
			credits: []*operation.Operation{credit("@debtor", 20)},
			rows:    paid("@debtor", "@fees", 5),
			froms:   map[string]int64{"@debtor": 15, "@fees": 5},
		},
		{
			name:    "a creditor that is also a destination merges into its source",
			credits: []*operation.Operation{credit("@debtor", 15), credit("@fees", 5)},
			rows:    paid("@debtor", "@fees", 5),
			froms:   map[string]int64{"@debtor": 10, "@fees": 10},
		},
		{
			name:    "a kept settlement stays where it was paid",
			credits: []*operation.Operation{credit("@debtor", 20)},
			rows:    paid("@debtor", "@fees", 20),
			kept:    map[FeeSettlementGroup]decimal.Decimal{{Ref: "@fees#default"}: decimal.NewFromInt(12), {Ref: "@debtor#default"}: decimal.NewFromInt(-12)},
			froms:   map[string]int64{"@debtor": 12, "@fees": 8},
		},
		{
			name:    "each fee route takes back under its own route",
			credits: []*operation.Operation{credit("@debtor", 20)},
			rows: append(append(routed("fee-a", paid("@debtor", "@fees", 5)...), routed("fee-b", paid("@debtor", "@fees", 3)...)...),
				routed("fee-a", paid("@debtor", "@fees", 2)...)...),
			froms: map[string]int64{"@debtor": 10, "@fees|fee-a": 7, "@fees|fee-b": 3},
		},
		{
			name:    "a creditor that is also a destination under another route keeps both legs",
			credits: append([]*operation.Operation{credit("@debtor", 15)}, routed("leg", credit("@fees", 5))...),
			rows:    routed("fee-a", paid("@debtor", "@fees", 5)...),
			froms:   map[string]int64{"@debtor": 10, "@fees|leg": 5, "@fees|fee-a": 5},
		},
		{
			name:    "a kept settlement stays under its route",
			credits: []*operation.Operation{credit("@debtor", 20)},
			rows:    append(routed("fee-a", paid("@debtor", "@fees", 5)...), routed("fee-b", paid("@debtor", "@fees", 3)...)...),
			kept: map[FeeSettlementGroup]decimal.Decimal{
				{Ref: "@fees#default", RouteID: "fee-b"}: decimal.NewFromInt(3), {Ref: "@debtor#default", RouteID: "fee-b"}: decimal.NewFromInt(-3),
			},
			froms: map[string]int64{"@debtor": 15, "@fees|fee-a": 5},
		},
		{
			name:    "refund rows are never reversed",
			credits: []*operation.Operation{credit("@debtor", 20)},
			rows: []*operation.Operation{
				feeDebtRow(pkgConstant.FEE_REFUND, "@debtor", pkgConstant.DirectionCredit, 3),
				feeDebtRow(pkgConstant.FEE_REFUND, "@fees", pkgConstant.DirectionDebit, 3),
			},
			froms: map[string]int64{"@debtor": 20},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			amount := decimal.NewFromInt(20)
			operations := append([]*operation.Operation{feeDebtRow(constant.DEBIT, "@payer", "", 20)}, tc.credits...)
			txn := Transaction{AssetCode: "BRL", Amount: &amount, Operations: append(operations, tc.rows...)}

			reverted := txn.TransactionRevert(tc.kept)

			froms, total := make(map[string]int64), decimal.Zero
			for _, from := range reverted.Send.Source.From {
				key := from.AccountAlias
				if from.RouteID != nil {
					key += "|" + *from.RouteID
				}

				froms[key] = from.Amount.Value.IntPart()
				total = total.Add(from.Amount.Value)
			}

			assert.Equal(t, tc.froms, froms)
			assert.Len(t, reverted.Send.Source.From, len(tc.froms), "a source folded to zero is dropped")
			assert.True(t, total.Equal(amount), "sources %s, amount %s", total, amount)
			require.Len(t, reverted.Send.Distribute.To, 1)
			assert.True(t, reverted.Send.Distribute.To[0].Amount.Value.Equal(amount))
		})
	}
}

func feeDebtRow(kind, alias, direction string, value int64) *operation.Operation {
	return &operation.Operation{
		Type: kind, Direction: direction, AccountAlias: alias, BalanceKey: pkgConstant.DefaultBalanceKey,
		AssetCode: "BRL", Amount: operation.Amount{Value: ptr(decimal.NewFromInt(value))},
	}
}
