// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"testing"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mtransaction"
)

func TestDecomposeCrossLedgerTransaction_ClosesEveryLedgerAndPreservesOrder(t *testing.T) {
	t.Parallel()

	organizationID := uuid.MustParse("01994f13-29b7-7000-8000-000000000101")
	ledgerA := atomicTransactionBatchLedgerRef{organizationID: organizationID, ledgerID: uuid.MustParse("01994f13-29b7-7000-8000-000000000102")}
	ledgerB := atomicTransactionBatchLedgerRef{organizationID: organizationID, ledgerID: uuid.MustParse("01994f13-29b7-7000-8000-000000000103")}
	ledgerC := atomicTransactionBatchLedgerRef{organizationID: organizationID, ledgerID: uuid.MustParse("01994f13-29b7-7000-8000-000000000104")}

	tests := []struct {
		name        string
		transaction mtransaction.Transaction
		scopes      crossLedgerTransactionScopes
		want        []decomposedCrossLedgerPart
	}{
		{
			name: "one debit ledger to one credit ledger",
			transaction: crossLedgerTestTransaction("100",
				[]mtransaction.FromTo{crossLedgerAmountLeg("@debit", "100", true)},
				[]mtransaction.FromTo{crossLedgerAmountLeg("@credit", "100", false)}),
			scopes: crossLedgerTransactionScopes{from: []atomicTransactionBatchLedgerRef{ledgerA}, to: []atomicTransactionBatchLedgerRef{ledgerB}},
			want: []decomposedCrossLedgerPart{
				crossLedgerExpectedPart(ledgerA, "100", []string{"@debit"}, []string{"@external/BRL"}, []string{"100"}, []string{"100"}),
				crossLedgerExpectedPart(ledgerB, "100", []string{"@external/BRL"}, []string{"@credit"}, []string{"100"}, []string{"100"}),
			},
		},
		{
			name: "many debit ledgers keep first-seen origin order",
			transaction: crossLedgerTestTransaction("100",
				[]mtransaction.FromTo{crossLedgerAmountLeg("@a", "40", true), crossLedgerAmountLeg("@b", "60", true)},
				[]mtransaction.FromTo{crossLedgerAmountLeg("@c", "100", false)}),
			scopes: crossLedgerTransactionScopes{from: []atomicTransactionBatchLedgerRef{ledgerB, ledgerA}, to: []atomicTransactionBatchLedgerRef{ledgerC}},
			want: []decomposedCrossLedgerPart{
				crossLedgerExpectedPart(ledgerB, "40", []string{"@a"}, []string{"@external/BRL"}, []string{"40"}, []string{"40"}),
				crossLedgerExpectedPart(ledgerA, "60", []string{"@b"}, []string{"@external/BRL"}, []string{"60"}, []string{"60"}),
				crossLedgerExpectedPart(ledgerC, "100", []string{"@external/BRL"}, []string{"@c"}, []string{"100"}, []string{"100"}),
			},
		},
		{
			name: "one debit ledger to many credit ledgers",
			transaction: crossLedgerTestTransaction("100",
				[]mtransaction.FromTo{crossLedgerAmountLeg("@a", "100", true)},
				[]mtransaction.FromTo{crossLedgerAmountLeg("@b", "25", false), crossLedgerAmountLeg("@c", "75", false)}),
			scopes: crossLedgerTransactionScopes{from: []atomicTransactionBatchLedgerRef{ledgerA}, to: []atomicTransactionBatchLedgerRef{ledgerB, ledgerC}},
			want: []decomposedCrossLedgerPart{
				crossLedgerExpectedPart(ledgerA, "100", []string{"@a"}, []string{"@external/BRL"}, []string{"100"}, []string{"100"}),
				crossLedgerExpectedPart(ledgerB, "25", []string{"@external/BRL"}, []string{"@b"}, []string{"25"}, []string{"25"}),
				crossLedgerExpectedPart(ledgerC, "75", []string{"@external/BRL"}, []string{"@c"}, []string{"75"}, []string{"75"}),
			},
		},
		{
			name: "ledger on both sides crosses only the net difference",
			transaction: crossLedgerTestTransaction("100",
				[]mtransaction.FromTo{crossLedgerAmountLeg("@a-debit", "100", true)},
				[]mtransaction.FromTo{crossLedgerAmountLeg("@a-credit", "30", false), crossLedgerAmountLeg("@b-credit", "70", false)}),
			scopes: crossLedgerTransactionScopes{from: []atomicTransactionBatchLedgerRef{ledgerA}, to: []atomicTransactionBatchLedgerRef{ledgerA, ledgerB}},
			want: []decomposedCrossLedgerPart{
				crossLedgerExpectedPart(ledgerA, "100", []string{"@a-debit"}, []string{"@a-credit", "@external/BRL"}, []string{"100"}, []string{"30", "70"}),
				crossLedgerExpectedPart(ledgerB, "70", []string{"@external/BRL"}, []string{"@b-credit"}, []string{"70"}, []string{"70"}),
			},
		},
		{
			name: "share and remaining are frozen before decomposition",
			transaction: crossLedgerTestTransaction("100",
				[]mtransaction.FromTo{
					{AccountAlias: "@share", Share: &mtransaction.Share{Percentage: 25}, IsFrom: true},
					{AccountAlias: "@remaining", Remaining: "remaining", IsFrom: true},
				},
				[]mtransaction.FromTo{crossLedgerAmountLeg("@credit", "100", false)}),
			scopes: crossLedgerTransactionScopes{from: []atomicTransactionBatchLedgerRef{ledgerA, ledgerB}, to: []atomicTransactionBatchLedgerRef{ledgerC}},
			want: []decomposedCrossLedgerPart{
				crossLedgerExpectedPart(ledgerA, "25", []string{"@share"}, []string{"@external/BRL"}, []string{"25"}, []string{"25"}),
				crossLedgerExpectedPart(ledgerB, "75", []string{"@remaining"}, []string{"@external/BRL"}, []string{"75"}, []string{"75"}),
				crossLedgerExpectedPart(ledgerC, "100", []string{"@external/BRL"}, []string{"@credit"}, []string{"100"}, []string{"100"}),
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := decomposeCrossLedgerTransaction(tt.transaction, tt.scopes)
			require.NoError(t, err)
			require.Len(t, got, len(tt.want))

			for i := range tt.want {
				assert.Equal(t, tt.want[i].ledgerRef, got[i].ledgerRef)
				assert.Equal(t, tt.want[i].transaction.Description, got[i].transaction.Description)
				assert.Equal(t, tt.want[i].transaction.Metadata, got[i].transaction.Metadata)
				assert.Equal(t, tt.want[i].transaction.Send.Value.String(), got[i].transaction.Send.Value.String())
				assertCrossLedgerLegs(t, tt.want[i].transaction.Send.Source.From, got[i].transaction.Send.Source.From)
				assertCrossLedgerLegs(t, tt.want[i].transaction.Send.Distribute.To, got[i].transaction.Send.Distribute.To)
			}
		})
	}
}

func TestDecomposeCrossLedgerTransaction_RejectsMixedAssets(t *testing.T) {
	t.Parallel()

	organizationID := uuid.MustParse("01994f13-29b7-7000-8000-000000000111")
	ledgerA := atomicTransactionBatchLedgerRef{organizationID: organizationID, ledgerID: uuid.MustParse("01994f13-29b7-7000-8000-000000000112")}
	ledgerB := atomicTransactionBatchLedgerRef{organizationID: organizationID, ledgerID: uuid.MustParse("01994f13-29b7-7000-8000-000000000113")}
	tx := crossLedgerTestTransaction("100",
		[]mtransaction.FromTo{crossLedgerAmountLeg("@debit", "100", true)},
		[]mtransaction.FromTo{crossLedgerAmountLeg("@credit", "100", false)})
	tx.Send.Distribute.To[0].Amount.Asset = "USD"

	_, err := decomposeCrossLedgerTransaction(tx, crossLedgerTransactionScopes{
		from: []atomicTransactionBatchLedgerRef{ledgerA},
		to:   []atomicTransactionBatchLedgerRef{ledgerB},
	})

	require.Error(t, err)
	assert.ErrorIs(t, err, constant.ErrCrossLedgerAssetMismatch)
}

func crossLedgerTestTransaction(total string, from, to []mtransaction.FromTo) mtransaction.Transaction {
	return mtransaction.Transaction{
		Description: "cross-ledger transfer",
		Metadata:    map[string]any{"purpose": "settlement"},
		Send: mtransaction.Send{
			Asset:      "BRL",
			Value:      decimal.RequireFromString(total),
			Source:     mtransaction.Source{From: from},
			Distribute: mtransaction.Distribute{To: to},
		},
	}
}

func crossLedgerAmountLeg(alias, amount string, isFrom bool) mtransaction.FromTo {
	return mtransaction.FromTo{
		AccountAlias: alias,
		Amount:       &mtransaction.Amount{Asset: "BRL", Value: decimal.RequireFromString(amount)},
		IsFrom:       isFrom,
	}
}

func crossLedgerExpectedPart(ref atomicTransactionBatchLedgerRef, total string, fromAliases, toAliases, fromAmounts, toAmounts []string) decomposedCrossLedgerPart {
	from := make([]mtransaction.FromTo, len(fromAliases))
	for i := range fromAliases {
		from[i] = crossLedgerAmountLeg(fromAliases[i], fromAmounts[i], true)
	}
	to := make([]mtransaction.FromTo, len(toAliases))
	for i := range toAliases {
		to[i] = crossLedgerAmountLeg(toAliases[i], toAmounts[i], false)
	}

	return decomposedCrossLedgerPart{ledgerRef: ref, transaction: crossLedgerTestTransaction(total, from, to)}
}

func assertCrossLedgerLegs(t *testing.T, want, got []mtransaction.FromTo) {
	t.Helper()
	require.Len(t, got, len(want))
	for i := range want {
		assert.Equal(t, want[i].AccountAlias, got[i].AccountAlias)
		require.NotNil(t, got[i].Amount)
		assert.Equal(t, want[i].Amount.Asset, got[i].Amount.Asset)
		assert.True(t, want[i].Amount.Value.Equal(got[i].Amount.Value))
		assert.Nil(t, got[i].Share)
		assert.Empty(t, got[i].Remaining)
	}
}
