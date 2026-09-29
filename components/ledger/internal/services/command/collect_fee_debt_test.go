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

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/domain/accounting"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
)

func TestComposeFeeDebtCollection(t *testing.T) {
	o := accounting.FeeDebtItem{ID: feeDebtOriginO + ":from:1:debit", CreditRef: "@fees#default", Remaining: decimal.NewFromInt(50)}
	x := accounting.FeeDebtItem{ID: feeDebtOriginX + ":from:1:debit", CreditRef: "@other-fees#default", Remaining: decimal.NewFromInt(80)}
	y := accounting.FeeDebtItem{ID: feeDebtOriginY + ":from:1:debit", CreditRef: "@fees#default", Remaining: decimal.NewFromInt(40)}
	pool := EngineSnapshotPool{
		Balances:     []*mmodel.Balance{feeDebtBalance("@payer"), feeDebtBalance("@fees"), feeDebtBalance("@other-fees")},
		FeeDebtSeeds: map[string][]accounting.FeeDebtItem{"@payer#default": {o, x, y}},
	}
	ceiling := decimal.NewFromInt(500)
	run := &createTransactionRun{}

	prepared, owed, err := composeFeeDebtCollection(CollectFeeDebtInput{
		OrganizationID: uuid.New(), LedgerID: uuid.New(), BalanceRef: "@payer#default", MaxAmount: &ceiling,
	}, pool, run)
	require.NoError(t, err)
	require.True(t, owed)

	assert.Equal(t, []accounting.Posting{{
		Ref: "collect", BalanceRef: "@payer#default", Type: accounting.PostingCollect, Amount: decimal.NewFromInt(170),
		DrawPolicy: accounting.DrawForbidden, OverdraftAmount: decimal.Zero, Items: []string{o.ID, x.ID, y.ID},
	}}, prepared.transaction.Postings, "it names every seeded debt, capped at what the balance owes")
	assert.Equal(t, []string{"@payer#default"}, prepared.transaction.FeeDebtRefs)
	assert.True(t, prepared.transaction.RejectBlockedBalances)
	assert.Len(t, prepared.projection, 6)
	assert.Equal(t, map[string]any{constant.MetadataKeyFeeDebtCollection: "true"}, run.input.Metadata)
	assert.Empty(t, run.input.Send.Source.From, "the empty source is what marks the transaction a collection")
	assert.Equal(t, []string{"USD", "170"}, []string{run.input.Send.Asset, run.input.Send.Value.String()})
	assert.Equal(t, []string{"@payer#default"}, run.validate.Sources)
	assert.Equal(t, []string{constant.ActionDirect, constant.CREATED}, []string{run.action, run.status})
}
