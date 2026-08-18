// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package utils

import (
	"testing"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mtransaction"
)

func TestLegacyTransactionIdempotencyHash_NormalizesDefaultBalanceKeys(t *testing.T) {
	t.Parallel()

	debit := mtransaction.Amount{Asset: "USD", Value: decimal.NewFromInt(10), Operation: constant.DEBIT}
	credit := mtransaction.Amount{Asset: "USD", Value: decimal.NewFromInt(10), Operation: constant.CREDIT}
	withoutKeys := mtransaction.Transaction{Send: mtransaction.Send{
		Asset:      "USD",
		Value:      decimal.NewFromInt(10),
		Source:     mtransaction.Source{From: []mtransaction.FromTo{{AccountAlias: "@source", Amount: &debit}}},
		Distribute: mtransaction.Distribute{To: []mtransaction.FromTo{{AccountAlias: "@destination", Amount: &credit}}},
	}}
	withKeys := withoutKeys
	withKeys.Send.Source.From = append([]mtransaction.FromTo(nil), withoutKeys.Send.Source.From...)
	withKeys.Send.Distribute.To = append([]mtransaction.FromTo(nil), withoutKeys.Send.Distribute.To...)
	withKeys.Send.Source.From[0].BalanceKey = constant.DefaultBalanceKey
	withKeys.Send.Distribute.To[0].BalanceKey = constant.DefaultBalanceKey

	first, err := LegacyTransactionIdempotencyHash(withoutKeys)
	require.NoError(t, err)
	second, err := LegacyTransactionIdempotencyHash(withKeys)
	require.NoError(t, err)
	assert.Equal(t, first, second)
}

func TestLegacyTransactionIdempotencyHash_QueueSanitizationPreservesPhaseZeroH1(t *testing.T) {
	t.Parallel()

	debit := mtransaction.Amount{Asset: "USD", Value: decimal.NewFromInt(10), Operation: constant.DEBIT}
	credit := mtransaction.Amount{Asset: "USD", Value: decimal.NewFromInt(10), Operation: constant.CREDIT}
	original := mtransaction.Transaction{Send: mtransaction.Send{
		Asset:      "USD",
		Value:      decimal.NewFromInt(10),
		Source:     mtransaction.Source{From: []mtransaction.FromTo{{AccountAlias: "@source", Amount: &debit}}},
		Distribute: mtransaction.Distribute{To: []mtransaction.FromTo{{AccountAlias: "@destination", Amount: &credit}}},
	}}
	want, err := LegacyTransactionIdempotencyHash(original)
	require.NoError(t, err)

	queued := original
	queued.Send.Source.From = append([]mtransaction.FromTo(nil), original.Send.Source.From...)
	queued.Send.Distribute.To = append([]mtransaction.FromTo(nil), original.Send.Distribute.To...)
	mtransaction.ApplyDefaultBalanceKeys(queued.Send.Source.From)
	mtransaction.ApplyDefaultBalanceKeys(queued.Send.Distribute.To)
	mtransaction.MutateConcatAliases(queued.Send.Source.From)
	mtransaction.MutateConcatAliases(queued.Send.Distribute.To)
	SanitizeAccountAliases(&queued)

	got, err := LegacyTransactionIdempotencyHash(queued)
	require.NoError(t, err)
	assert.Equal(t, want, got,
		"the backup snapshot must reproduce the exact old H1 after internal alias concatenation")
}
