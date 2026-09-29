// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package engine

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/domain/accounting"
	"github.com/LerianStudio/midaz/v4/pkg/utils"
)

func TestPrepareExecutionAppendsFeeDebtKeysLast(t *testing.T) {
	t.Parallel()

	input, limits, resolved := validWireExecution()
	addWireBalance(&input, &resolved)
	resolved.Accounts = testResolvedAccountKeys("tenant:fixture:", input.Execution, testAdmissionToken)
	expandWireTransactions(&input, 2)
	input.Execution.Transactions[0].FeeDebtRefs = []string{"@source#default"}
	input.Execution.Transactions[1].FeeDebtRefs = []string{"@secondary#default", "@source#default"}
	input.Execution.Transactions[1].ReopenFeeDebts = []accounting.FeeDebtReopen{{
		DebtID: input.Execution.Transactions[0].ID.String() + ":fee", DebtorRef: "@source#default", CreditRef: "@secondary#default",
		Amount: decimal.RequireFromString("12.50"), Opened: decimal.NewFromInt(70), Seq: 3,
	}}
	input.Execution.Transactions[1].Postings = append(input.Execution.Transactions[1].Postings, accounting.Posting{
		Ref: "refund", BalanceRef: "@source#default", Type: accounting.PostingRefund, Amount: decimal.NewFromInt(70),
		DrawPolicy: accounting.DrawForbidden, OverdraftAmount: decimal.Zero,
		Refunds: []accounting.FeeDebtRefund{{DebtID: "parent:fee", CreditRef: "@secondary#default", Opened: decimal.NewFromInt(70), Seq: 9, ExpectedRefund: decimal.NewFromInt(25)}},
	})
	resolved.FeeDebts = make(map[string]string)
	require.NoError(t, resolveFeeDebtKeys(context.Background(), input.Execution, &resolved))
	source := utils.FeeDebtInternalKey(input.Execution.OrganizationID, input.Execution.LedgerID, "@source#default")
	secondary := utils.FeeDebtInternalKey(input.Execution.OrganizationID, input.Execution.LedgerID, "@secondary#default")

	prepared, err := prepareExecution(context.Background(), input, limits, resolved)
	require.NoError(t, err)
	require.Equal(t, []string{source, secondary}, prepared.Keys[len(prepared.Keys)-2:])

	var wire struct {
		FeeDebts     []wireFeeDebt `json:"feeDebts"`
		Transactions []struct {
			ReopenFeeDebts []wireFeeDebtReopen `json:"reopenFeeDebts"`
			Postings       []wirePosting       `json:"postings"`
		} `json:"transactions"`
	}
	require.NoError(t, json.Unmarshal(prepared.Payload, &wire))
	scope := input.Execution.OrganizationID.String()
	ledger := input.Execution.LedgerID.String()
	require.Equal(t, []wireFeeDebt{
		{OrganizationID: scope, LedgerID: ledger, BalanceRef: "@source#default", KeyIndex: len(prepared.Keys) - 1},
		{OrganizationID: scope, LedgerID: ledger, BalanceRef: "@secondary#default", KeyIndex: len(prepared.Keys)},
	}, wire.FeeDebts)
	require.Equal(t, []wireFeeDebtReopen{{
		DebtID: input.Execution.Transactions[0].ID.String() + ":fee", DebtorRef: "@source#default", CreditRef: "@secondary#default",
		Amount: "12.5", Opened: "70", Seq: "3",
	}}, wire.Transactions[1].ReopenFeeDebts)
	require.Equal(t, []wireFeeDebtRefund{{DebtID: "parent:fee", CreditRef: "@secondary#default", Opened: "70", Seq: "9", ExpectedRefund: "25"}}, wire.Transactions[1].Postings[1].Refunds)
	require.Nil(t, wire.Transactions[0].ReopenFeeDebts)

	resolved.FeeDebts = map[string]string{scopedBalanceRef(input.Execution.OrganizationID, input.Execution.LedgerID, "@source#default"): source}
	_, err = prepareExecution(context.Background(), input, limits, resolved)
	require.ErrorContains(t, err, "fee-debt inventory")
}
