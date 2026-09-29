// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package engine

import (
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"testing"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"

	core "github.com/LerianStudio/midaz/v4/components/ledger/internal/domain/accounting"
)

type feeDebtStep struct {
	posting, role string
	ordinal       int
	balance       string
	kind          core.PostingType
	amount        int64
}

var feeDebtOrigin = uuid.MustParse("66666666-6666-4666-8666-666666666666")

// feeDebtRequest is the adapter fixture with @fees beside @source.
func feeDebtRequest(t *testing.T, postings ...core.Posting) core.Execution {
	t.Helper()

	request, _ := adapterResultFixture(t)
	fees := request.Balances[0]
	fees.ID, fees.AccountID = uuid.MustParse("77777777-7777-4777-8777-777777777777"), uuid.MustParse("88888888-8888-4888-8888-888888888888")
	fees.BalanceRef, fees.Alias = "@fees#default", "@fees"
	request.Balances = append(request.Balances, fees)
	request.Transactions[0].Postings = postings

	return request
}

// feeDebtResponse simulates the engine's movements over the request's seeds, so
// every state is continuous and every final matches its last movement.
func feeDebtResponse(t *testing.T, request core.Execution, steps []feeDebtStep, changes []resultFeeDebtChange) []byte {
	t.Helper()

	balances, order := make(map[string]core.BalanceSnapshot), []string{}
	envelope := resultEnvelope{ProtocolVersion: 1, Movements: []json.RawMessage{}, Final: []json.RawMessage{}}
	transactionID := request.Transactions[0].ID

	for _, step := range steps {
		balance, seen := balances[step.balance]
		if !seen {
			for _, seed := range request.Balances {
				if seed.BalanceRef == step.balance {
					balance = seed
				}
			}
			order = append(order, step.balance)
		}
		state := func() json.RawMessage {
			return adapterJSON(t, resultState{balance.Available.String(), "0", "0", strconv.FormatInt(balance.Version, 10)})
		}
		before, amount := state(), decimal.NewFromInt(step.amount)
		if step.kind == core.PostingDebit {
			amount = amount.Neg()
		}
		balance.Available, balance.Version = balance.Available.Add(amount), balance.Version+1
		envelope.Movements = append(envelope.Movements, adapterJSON(t, resultMovement{
			Ref:           fmt.Sprintf("%s:%d:%s:%s:%d", transactionID, len(step.posting), step.posting, step.role, step.ordinal),
			TransactionID: transactionID.String(), PostingRef: step.posting, Role: step.role, BalanceRef: step.balance,
			Type: step.kind, Amount: strconv.FormatInt(step.amount, 10), OverdraftDelta: "0", Before: before, After: state(),
		}))
		balances[step.balance] = balance
	}

	for _, ref := range order {
		snapshot, err := prepareSnapshot(balances[ref], 1<<20)
		require.NoError(t, err)
		envelope.Final = append(envelope.Final, adapterJSON(t, resultBalance{ref, snapshot}))
	}

	for _, change := range changes {
		change.TransactionID = transactionID.String()
		envelope.FeeDebt = append(envelope.FeeDebt, adapterJSON(t, change))
	}

	return adapterJSON(t, envelope)
}

func feeDebtChange(kind core.FeeDebtChangeKind, posting, suffix, amount, opened string, seq int) resultFeeDebtChange {
	return resultFeeDebtChange{
		PostingRef: posting, Kind: kind, DebtID: feeDebtOrigin.String() + ":" + suffix, DebtorRef: "@source#default",
		CreditRef: "@fees#default", OriginTransactionID: feeDebtOrigin.String(), Seq: strconv.Itoa(seq),
		AssetCode: "BRL", Amount: amount, Opened: opened,
	}
}

func TestDecodeResult_PairsFeeDebtMovementsWithChanges(t *testing.T) {
	collect := core.Posting{
		Ref: "c", BalanceRef: "@source#default", Type: core.PostingCollect, Amount: decimal.NewFromInt(100),
		Items: []string{feeDebtOrigin.String() + ":a", feeDebtOrigin.String() + ":b"},
	}
	debit := core.Posting{Ref: "fd", BalanceRef: "@source#default", Type: core.PostingDebit, Amount: decimal.NewFromInt(100), DeferShortfall: true}
	credit := core.Posting{Ref: "fc", BalanceRef: "@fees#default", Type: core.PostingCredit, Amount: decimal.NewFromInt(100), FundedByRef: "fd"}
	refund := core.Posting{Ref: "r", BalanceRef: "@source#default", Type: core.PostingRefund, Amount: decimal.NewFromInt(90), Refunds: []core.FeeDebtRefund{
		{DebtID: feeDebtOrigin.String() + ":a", CreditRef: "@fees#default", Opened: decimal.NewFromInt(70), Seq: 1, ExpectedRefund: decimal.NewFromInt(30)},
		{DebtID: feeDebtOrigin.String() + ":b", CreditRef: "@fees#default", Opened: decimal.NewFromInt(20), Seq: 3, ExpectedRefund: decimal.NewFromInt(20)},
	}}

	settle := func() ([]feeDebtStep, []resultFeeDebtChange) {
		return []feeDebtStep{
				{"c", core.RoleFeeDebtDebit, 0, "@source#default", core.PostingDebit, 60},
				{"c", core.RoleFeeDebtCredit, 0, "@fees#default", core.PostingCredit, 50},
				{"c", core.RoleFeeDebtCredit, 1, "@fees#default", core.PostingCredit, 10},
			}, []resultFeeDebtChange{
				feeDebtChange(core.FeeDebtSettled, "c", "a", "50", "50", 1),
				feeDebtChange(core.FeeDebtSettled, "c", "b", "10", "70", 2),
			}
	}
	defer1 := func() ([]feeDebtStep, []resultFeeDebtChange) {
		return []feeDebtStep{
				{"fd", core.RolePrimary, 0, "@source#default", core.PostingDebit, 30},
				{"fc", core.RolePrimary, 0, "@fees#default", core.PostingCredit, 30},
			}, []resultFeeDebtChange{
				feeDebtChange(core.FeeDebtOpened, "fd", "fd", "70", "70", 1),
			}
	}
	reimburse := func() ([]feeDebtStep, []resultFeeDebtChange) {
		return []feeDebtStep{
				{"r", core.RoleFeeDebtRefundCredit, 0, "@source#default", core.PostingCredit, 50},
				{"r", core.RoleFeeDebtRefundDebit, 0, "@fees#default", core.PostingDebit, 30},
				{"r", core.RoleFeeDebtRefundDebit, 1, "@fees#default", core.PostingDebit, 20},
			}, []resultFeeDebtChange{
				feeDebtChange(core.FeeDebtCanceled, "", "a", "40", "70", 1),
				feeDebtChange(core.FeeDebtRefunded, "r", "a", "30", "70", 1),
				feeDebtChange(core.FeeDebtRefunded, "r", "b", "20", "20", 3),
			}
	}

	tests := []struct {
		name     string
		postings []core.Posting
		scenario func() ([]feeDebtStep, []resultFeeDebtChange)
		mutate   func(steps *[]feeDebtStep, changes *[]resultFeeDebtChange)
		valid    bool
	}{
		{name: "a collect", postings: []core.Posting{collect}, scenario: settle, valid: true},
		{name: "a deferral", postings: []core.Posting{debit, credit}, scenario: defer1, valid: true},
		{name: "a refund", postings: []core.Posting{refund}, scenario: reimburse, valid: true},
		{
			name: "changes without movements", postings: []core.Posting{collect}, scenario: settle,
			mutate: func(s *[]feeDebtStep, c *[]resultFeeDebtChange) { *s = nil },
		},
		{
			name: "a settlement without its change", postings: []core.Posting{collect}, scenario: settle,
			mutate: func(s *[]feeDebtStep, c *[]resultFeeDebtChange) { *c = (*c)[:1] },
		},
		{
			name: "a settlement of another amount", postings: []core.Posting{collect}, scenario: settle,
			mutate: func(s *[]feeDebtStep, c *[]resultFeeDebtChange) { (*c)[1].Amount = "9" },
		},
		{
			name: "a settlement naming another item", postings: []core.Posting{collect}, scenario: settle,
			mutate: func(s *[]feeDebtStep, c *[]resultFeeDebtChange) { (*c)[0].DebtID = (*c)[1].DebtID },
		},
		{
			name: "an ordinal outside the items", postings: []core.Posting{collect}, scenario: settle,
			mutate: func(s *[]feeDebtStep, c *[]resultFeeDebtChange) { (*s)[2].ordinal = 2 },
		},
		{
			name: "a debtor total other than its settlements", postings: []core.Posting{collect}, scenario: settle,
			mutate: func(s *[]feeDebtStep, c *[]resultFeeDebtChange) { (*s)[0].amount = 61 },
		},
		{
			name: "an unknown change kind", postings: []core.Posting{collect}, scenario: settle,
			mutate: func(s *[]feeDebtStep, c *[]resultFeeDebtChange) { (*c)[0].Kind = "forgiven" },
		},
		{
			name: "an amount above opened", postings: []core.Posting{collect}, scenario: settle,
			mutate: func(s *[]feeDebtStep, c *[]resultFeeDebtChange) { (*c)[1].Opened = "9" },
		},
		{
			name: "a deferral that does not add up", postings: []core.Posting{debit, credit}, scenario: defer1,
			mutate: func(s *[]feeDebtStep, c *[]resultFeeDebtChange) { (*c)[0].Amount = "69" },
		},
		{
			name: "an opening on another fee account", postings: []core.Posting{debit, credit}, scenario: defer1,
			mutate: func(s *[]feeDebtStep, c *[]resultFeeDebtChange) { (*c)[0].CreditRef = "@other#default" },
		},
		{
			name: "a cancellation with a posting", postings: []core.Posting{refund}, scenario: reimburse,
			mutate: func(s *[]feeDebtStep, c *[]resultFeeDebtChange) { (*c)[0].PostingRef = "r" },
		},
		{
			name: "a refund other than its expected refund", postings: []core.Posting{refund}, scenario: reimburse,
			mutate: func(s *[]feeDebtStep, c *[]resultFeeDebtChange) {
				(*s)[0].amount, (*s)[1].amount, (*c)[1].Amount = 90, 70, "70"
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			request := feeDebtRequest(t, test.postings...)
			steps, changes := test.scenario()
			if test.mutate != nil {
				test.mutate(&steps, &changes)
			}

			result, err := DecodeResult(feeDebtResponse(t, request, steps, changes), request)
			if !test.valid {
				require.Error(t, err)

				return
			}
			require.NoError(t, err)
			require.Len(t, result.FeeDebt, len(changes))
			for i, change := range result.FeeDebt {
				require.Equal(t, request.Transactions[0].ID, change.TransactionID)
				require.Equal(t, changes[i].Opened, change.Opened.String(), "every change carries opened")
			}
		})
	}
}

func TestClassifyAccountingError_CorrelatesFeeDebtRefusals(t *testing.T) {
	refund := core.Posting{
		Ref: "r", BalanceRef: "@source#default", Type: core.PostingRefund, Amount: decimal.NewFromInt(70),
		Refunds: []core.FeeDebtRefund{{DebtID: feeDebtOrigin.String() + ":a", CreditRef: "@fees#default", Opened: decimal.NewFromInt(70), Seq: 1}},
	}
	request := feeDebtRequest(t, refund)
	request.Transactions[0].ReopenFeeDebts = []core.FeeDebtReopen{{DebtID: feeDebtOrigin.String() + ":b", DebtorRef: "@source#default", CreditRef: "@fees#default"}}

	for _, failure := range []core.Failure{
		{Code: core.FailureInsufficientFunds, PostingIndex: 0, BalanceRef: "@fees#default"},
		{Code: core.FailureBalanceDeleted, PostingIndex: -1, BalanceRef: "@source#default"},
	} {
		err := classifyAccountingError(accountingReply("MIDAZ_ENGINE_V1 "+string(adapterJSON(t, failure))), request, nil)
		var refusal *core.Failure
		require.ErrorAs(t, err, &refusal)
		require.Equal(t, failure, *refusal)
	}

	unrelated := core.Failure{Code: core.FailureInsufficientFunds, PostingIndex: -1, BalanceRef: "@source#default"}
	err := classifyAccountingError(accountingReply("MIDAZ_ENGINE_V1 "+string(adapterJSON(t, unrelated))), request, nil)
	var refusal *core.Failure
	require.False(t, errors.As(err, &refusal), "a reopen correlates only a touch refusal")
}
