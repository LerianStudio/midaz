// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"testing"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	"github.com/LerianStudio/midaz/v4/pkg/mtransaction"
	"github.com/LerianStudio/midaz/v4/pkg/tracercontract"
)

func TestTracerPreparedEntriesRetainFeesAndMultiplicity(t *testing.T) {
	payer := uuid.MustParse("35279c72-498a-4fd5-b5b7-1bd4bd44e338")
	input := mtransaction.Transaction{Send: mtransaction.Send{Asset: "BTC", Source: mtransaction.Source{From: []mtransaction.FromTo{{AccountAlias: "0#@payer#default"}, {AccountAlias: "1#@payer#default"}}}, Distribute: mtransaction.Distribute{To: []mtransaction.FromTo{{AccountAlias: "0#@external/BTC#default"}}}}}
	validated := &mtransaction.Responses{Pending: true, From: map[string]mtransaction.Amount{"0#@payer#default": {Value: decimal.RequireFromString("10.000000000000000001")}, "1#@payer#default": {Value: decimal.RequireFromString("0.125")}}, To: map[string]mtransaction.Amount{"0#@external/BTC#default": {Value: decimal.RequireFromString("10.125000000000000001")}}}
	balances := []*mmodel.Balance{{Alias: "@payer", Key: "default", AccountID: payer.String(), AssetCode: "BTC", AccountType: "deposit"}, {Alias: "@external/BTC", Key: "default", AccountID: uuid.MustParse("7e871c7b-24e9-4e3d-a4c2-957180a71e10").String(), AssetCode: "BTC", AccountType: constant.ExternalAccountType}, {Alias: "@payer", Key: constant.OverdraftBalanceKey, AccountID: payer.String(), AssetCode: "BTC", AccountType: "deposit"}}
	entries, err := tracerPreparedEntries(t.Context(), input, validated, balances, 10)
	require.NoError(t, err)
	require.Len(t, entries, 3, "fee legs remain entries; auxiliary overdraft balances do not")
	require.Equal(t, payer, entries[0].AccountID)
	require.Equal(t, payer, entries[1].AccountID)
	require.Equal(t, "10.000000000000000001", entries[0].Amount.String())
	require.Equal(t, "0.125", entries[1].Amount.String())
	require.Equal(t, tracercontract.Debit, entries[0].Direction)
	require.Equal(t, tracercontract.Debit, entries[1].Direction)
	require.Equal(t, tracercontract.Credit, entries[2].Direction)
	require.True(t, entries[2].External)
	require.Equal(t, uuid.Nil, entries[2].AccountID)
	// A hold still sends destination facts even though its engine execution only
	// moves the source into on-hold funds.
	require.Equal(t, "BTC", entries[2].AssetCode)
	_, err = tracerPreparedEntries(t.Context(), input, validated, balances, 2)
	require.Error(t, err)
	delete(validated.From, "1#@payer#default")
	_, err = tracerPreparedEntries(t.Context(), input, validated, balances, 10)
	require.Error(t, err, "never produce a partial context when a prepared leg is missing")
}
