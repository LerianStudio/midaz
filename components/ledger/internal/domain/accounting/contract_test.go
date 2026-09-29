// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package accounting

import (
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"testing"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

func TestRequestContractRoundTrip(t *testing.T) {
	t.Parallel()

	transactionID := uuid.MustParse("1a2cf884-cf82-4520-9833-07d85c73bc14")
	exceptionID := uuid.MustParse("6e0ebc70-6039-4edf-b039-4bb5d85afafe")
	snapshot := contractSnapshot()
	request := Execution{
		OrganizationID: uuid.MustParse("139c4166-2139-4f17-b282-fba78e8c4c2a"),
		LedgerID:       uuid.MustParse("c029e784-535d-4554-aae3-65b6e713687f"),
		ExecutionID:    uuid.MustParse("95b4a433-59b3-4ac8-ad6f-e4be032ebca6"),
		Transactions: []Transaction{{
			OrganizationID: uuid.MustParse("139c4166-2139-4f17-b282-fba78e8c4c2a"),
			LedgerID:       uuid.MustParse("c029e784-535d-4554-aae3-65b6e713687f"),
			ID:             transactionID,
			AccountBlockException: &AccountBlockException{
				ExceptionID: exceptionID, Alias: snapshot.Alias,
				Amount:            decimal.RequireFromString("12345678901234567890.1234567890123456789"),
				PrimaryPostingRef: "source-debit",
			},
			Postings: []Posting{{
				Ref:             "source-debit",
				BalanceRef:      snapshot.BalanceRef,
				Type:            PostingDebit,
				Amount:          decimal.RequireFromString("12345678901234567890.1234567890123456789"),
				DrawPolicy:      DrawAllowed,
				OverdraftAmount: decimal.RequireFromString("0.0000000000000000001"),
			}},
		}},
		Balances: []BalanceSnapshot{snapshot},
	}
	request.Balances[0].OrganizationID = request.OrganizationID
	request.Balances[0].LedgerID = request.LedgerID

	// Typed Go JSON round trips do not define the adapter's Lua wire DTO.
	got := contractRoundTrip(t, request)
	if !reflect.DeepEqual(got, request) {
		t.Fatalf("request round trip changed identifiers, monetary values, versions or settings:\ngot: %#v\nwant: %#v", got, request)
	}
	identifiers := []uuid.UUID{
		got.OrganizationID, got.LedgerID, got.ExecutionID, got.Transactions[0].ID,
		got.Transactions[0].AccountBlockException.ExceptionID,
		got.Balances[0].ID, got.Balances[0].AccountID,
	}
	for i, identifier := range identifiers {
		if identifier == uuid.Nil {
			t.Errorf("identifier %d lost its UUID value", i)
		}
	}
}

func TestRequestContractOmitsAbsentAccountBlockException(t *testing.T) {
	t.Parallel()

	encoded, err := json.Marshal(Transaction{
		OrganizationID: uuid.MustParse("139c4166-2139-4f17-b282-fba78e8c4c2a"),
		LedgerID:       uuid.MustParse("c029e784-535d-4554-aae3-65b6e713687f"),
		ID:             uuid.MustParse("1a2cf884-cf82-4520-9833-07d85c73bc14"),
	})
	if err != nil {
		t.Fatalf("marshal transaction: %v", err)
	}
	if string(encoded) != `{"organizationId":"139c4166-2139-4f17-b282-fba78e8c4c2a","ledgerId":"c029e784-535d-4554-aae3-65b6e713687f","id":"1a2cf884-cf82-4520-9833-07d85c73bc14","rejectBlockedBalances":false,"balanceRequirements":null,"postings":null}` {
		t.Fatalf("absent account-block exception changed contract: %s", encoded)
	}
}

func TestResultContractPreservesZeroAmountDebtMovement(t *testing.T) {
	t.Parallel()

	snapshot := contractSnapshot()
	before := BalanceState{
		Available:     decimal.NewFromInt(0),
		OnHold:        decimal.RequireFromString("23.1234567890123456789"),
		OverdraftUsed: decimal.RequireFromString("1.0000000000000000001"),
		Version:       9007199254740993,
	}
	after := before
	after.OverdraftUsed = decimal.RequireFromString("0.0000000000000000001")
	after.Version++
	snapshot.Available = after.Available
	snapshot.OnHold = after.OnHold
	snapshot.OverdraftUsed = after.OverdraftUsed
	snapshot.Version = after.Version
	result := ExecutionResult{
		Movements: []Movement{{
			Ref:            "source-credit-primary-0",
			TransactionID:  uuid.MustParse("1a2cf884-cf82-4520-9833-07d85c73bc14"),
			PostingRef:     "source-credit",
			Role:           RolePrimary,
			BalanceRef:     snapshot.BalanceRef,
			Type:           PostingCredit,
			Amount:         decimal.NewFromInt(0),
			OverdraftDelta: decimal.NewFromInt(-1),
			Before:         before,
			After:          after,
		}},
		Final: []BalanceSnapshot{snapshot},
	}

	got := contractRoundTrip(t, result)
	if !reflect.DeepEqual(got, result) {
		t.Fatalf("result round trip changed movement or final snapshot:\ngot: %#v\nwant: %#v", got, result)
	}
	if !got.Movements[0].Amount.IsZero() || got.Movements[0].After.Version != before.Version+1 {
		t.Fatal("zero-amount debt movement must retain its version change")
	}
}

func TestContractVocabulary(t *testing.T) {
	t.Parallel()

	postings := map[PostingType]string{
		PostingDebit: "debit", PostingCredit: "credit", PostingReserve: "reserve",
		PostingUnreserve: "unreserve", PostingHold: "hold", PostingRelease: "release",
		PostingCollect: "collect", PostingRefund: "refund",
	}
	if len(postings) != 8 {
		t.Fatal("posting types must remain distinct")
	}
	for got, want := range postings {
		if string(got) != want {
			t.Errorf("posting type = %q, want %q", got, want)
		}
	}
	drawPolicies := map[DrawPolicy]string{
		DrawForbidden: "forbidden", DrawAllowed: "allowed", DrawRouteDenied: "route_denied",
	}
	if len(drawPolicies) != 3 {
		t.Fatal("draw policies must remain distinct")
	}
	for got, want := range drawPolicies {
		if string(got) != want {
			t.Errorf("draw policy = %q, want %q", got, want)
		}
	}
	if RolePrimary != "primary" || RoleOverdraftCompanion != "overdraft_companion" ||
		RoleFeeDebtDebit != "fee_debt_debit" || RoleFeeDebtCredit != "fee_debt_credit" ||
		RoleFeeDebtRefundCredit != "fee_debt_refund_credit" || RoleFeeDebtRefundDebit != "fee_debt_refund_debit" {
		t.Fatal("movement roles changed")
	}
	kinds := map[FeeDebtChangeKind]string{
		FeeDebtOpened: "opened", FeeDebtSettled: "settled", FeeDebtCanceled: "canceled", FeeDebtReopened: "reopened",
		FeeDebtRefunded: "refunded",
	}
	if len(kinds) != 5 {
		t.Fatal("fee debt change kinds must remain distinct")
	}
	for got, want := range kinds {
		if string(got) != want {
			t.Errorf("fee debt change kind = %q, want %q", got, want)
		}
	}
}

func TestFailureContract(t *testing.T) {
	t.Parallel()

	codes := map[string]string{
		FailureInsufficientFunds:            "insufficient_funds",
		FailureOverdraftLimitExceeded:       "overdraft_limit_exceeded",
		FailureOverdraftNotEligible:         "overdraft_not_eligible",
		FailureOverdraftCompanionMissing:    "overdraft_companion_missing",
		FailureBalanceDeleted:               "balance_deleted",
		FailureAccountBlocked:               "account_blocked",
		FailureOnHoldUnderflow:              "onhold_underflow",
		FailureBalanceMissing:               "balance_missing",
		FailureAssetMismatch:                "asset_mismatch",
		FailureSendingNotAllowed:            "sending_not_allowed",
		FailureReceivingNotAllowed:          "receiving_not_allowed",
		FailureExternalHoldNotAllowed:       "external_hold_not_allowed",
		FailureAccountBlockExceptionInvalid: "account_block_exception_invalid",
	}
	if len(codes) != 13 {
		t.Fatal("failure codes must remain distinct")
	}
	for code, want := range codes {
		t.Run(want, func(t *testing.T) {
			t.Parallel()

			failure := &Failure{Code: code, TransactionIndex: 0, PostingIndex: 1, BalanceRef: "@source#default"}
			if code != want || failure.Error() != want {
				t.Fatalf("failure code = %q, Error() = %q, want %q", code, failure.Error(), want)
			}
			var extracted *Failure
			wrapped := fmt.Errorf("accounting execution: %w", failure)
			if !errors.As(wrapped, &extracted) || extracted != failure || !errors.Is(wrapped, failure) {
				t.Fatal("typed failure must survive error wrapping")
			}
			if got := contractRoundTrip(t, *failure); got != *failure {
				t.Fatalf("failure round trip = %#v, want %#v", got, *failure)
			}
		})
	}
	preselection := Failure{Code: FailureBalanceMissing, TransactionIndex: -1, PostingIndex: -1}
	if got := contractRoundTrip(t, preselection); got != preselection {
		t.Fatalf("preselection failure indices changed: %#v", got)
	}
}

func TestFeeDebtRequestContractShape(t *testing.T) {
	t.Parallel()

	origin := uuid.MustParse("6e0ebc70-6039-4edf-b039-4bb5d85afafe")
	route := &FeeDebtRoute{ID: "0199f0a1-7c7e-7a4e-9a51-3d1b7c7f0a01", Code: "4.1.01", Description: "Fee revenue"}
	transaction := Transaction{
		OrganizationID: uuid.MustParse("139c4166-2139-4f17-b282-fba78e8c4c2a"),
		LedgerID:       uuid.MustParse("c029e784-535d-4554-aae3-65b6e713687f"),
		ID:             uuid.MustParse("1a2cf884-cf82-4520-9833-07d85c73bc14"),
		Postings: []Posting{
			{Ref: "fee-debit", BalanceRef: "@payer#default", Type: PostingDebit, Amount: decimal.NewFromInt(100), DrawPolicy: DrawForbidden, DeferShortfall: true, DebtRoute: route},
			{Ref: "fee-credit", BalanceRef: "@fees#default", Type: PostingCredit, Amount: decimal.NewFromInt(100), DrawPolicy: DrawForbidden, FundedByRef: "fee-debit"},
			{Ref: "fee-credit:collect", BalanceRef: "@payer#default", Type: PostingCollect, Amount: decimal.NewFromInt(200), DrawPolicy: DrawForbidden, Items: []string{origin.String() + ":fee-debit"}},
			{Ref: "fee-refund:0", BalanceRef: "@payer#default", Type: PostingRefund, Amount: decimal.NewFromInt(70), DrawPolicy: DrawForbidden, Refunds: []FeeDebtRefund{{
				DebtID: origin.String() + ":fee-debit", CreditRef: "@fees#default", Opened: decimal.NewFromInt(70), Seq: 3, ExpectedRefund: decimal.NewFromInt(30),
			}}},
		},
		FeeDebtRefs: []string{"@payer#default"},
		ReopenFeeDebts: []FeeDebtReopen{{
			DebtID: origin.String() + ":fee-debit", DebtorRef: "@payer#default", CreditRef: "@fees#default",
			Amount: decimal.RequireFromString("12.5"), Opened: decimal.NewFromInt(70), Seq: 7, CreditRoute: route,
		}},
	}

	want := `{"organizationId":"139c4166-2139-4f17-b282-fba78e8c4c2a","ledgerId":"c029e784-535d-4554-aae3-65b6e713687f","id":"1a2cf884-cf82-4520-9833-07d85c73bc14","rejectBlockedBalances":false,"balanceRequirements":null,"postings":[` +
		`{"ref":"fee-debit","balanceRef":"@payer#default","type":"debit","amount":"100","drawPolicy":"forbidden","overdraftAmount":"0","deferShortfall":true,` +
		`"debtRoute":{"id":"0199f0a1-7c7e-7a4e-9a51-3d1b7c7f0a01","code":"4.1.01","description":"Fee revenue"}},` +
		`{"ref":"fee-credit","balanceRef":"@fees#default","type":"credit","amount":"100","drawPolicy":"forbidden","overdraftAmount":"0","fundedByRef":"fee-debit"},` +
		`{"ref":"fee-credit:collect","balanceRef":"@payer#default","type":"collect","amount":"200","drawPolicy":"forbidden","overdraftAmount":"0","items":["6e0ebc70-6039-4edf-b039-4bb5d85afafe:fee-debit"]},` +
		`{"ref":"fee-refund:0","balanceRef":"@payer#default","type":"refund","amount":"70","drawPolicy":"forbidden","overdraftAmount":"0","refunds":[{"debtId":"6e0ebc70-6039-4edf-b039-4bb5d85afafe:fee-debit","creditRef":"@fees#default","opened":"70","seq":"3","expectedRefund":"30"}]}],` +
		`"feeDebtRefs":["@payer#default"],` +
		`"reopenFeeDebts":[{"debtId":"6e0ebc70-6039-4edf-b039-4bb5d85afafe:fee-debit","debtorRef":"@payer#default","creditRef":"@fees#default","amount":"12.5","opened":"70","seq":"7",` +
		`"creditRoute":{"id":"0199f0a1-7c7e-7a4e-9a51-3d1b7c7f0a01","code":"4.1.01","description":"Fee revenue"}}]}`
	assertContractJSON(t, transaction, want)

	plain := Posting{Ref: "debit", BalanceRef: "@payer#default", Type: PostingDebit, Amount: decimal.NewFromInt(1), DrawPolicy: DrawAllowed}
	assertContractJSON(t, plain, `{"ref":"debit","balanceRef":"@payer#default","type":"debit","amount":"1","drawPolicy":"allowed","overdraftAmount":"0"}`)
}

func TestFeeDebtResultContractShape(t *testing.T) {
	t.Parallel()

	change := FeeDebtChange{
		TransactionID: uuid.MustParse("1a2cf884-cf82-4520-9833-07d85c73bc14"), PostingRef: "fee-debit", Kind: FeeDebtOpened,
		DebtID: "1a2cf884-cf82-4520-9833-07d85c73bc14:fee-debit", DebtorRef: "@payer#default", CreditRef: "@fees#default",
		OriginTransactionID: uuid.MustParse("1a2cf884-cf82-4520-9833-07d85c73bc14"), Seq: 3, AssetCode: "BRL",
		Amount: decimal.RequireFromString("70.0000000000000000001"), Opened: decimal.RequireFromString("70.0000000000000000001"),
	}
	assertContractJSON(t, ExecutionResult{Movements: []Movement{}, Final: []BalanceSnapshot{}, FeeDebt: []FeeDebtChange{change}},
		`{"movements":[],"final":[],"feeDebt":[{"transactionId":"1a2cf884-cf82-4520-9833-07d85c73bc14","postingRef":"fee-debit","kind":"opened",`+
			`"debtId":"1a2cf884-cf82-4520-9833-07d85c73bc14:fee-debit","debtorRef":"@payer#default","creditRef":"@fees#default",`+
			`"originTransactionId":"1a2cf884-cf82-4520-9833-07d85c73bc14","seq":"3","assetCode":"BRL","amount":"70.0000000000000000001","opened":"70.0000000000000000001"}]}`)
	assertContractJSON(t, ExecutionResult{Movements: []Movement{}, Final: []BalanceSnapshot{}, FeeDebt: []FeeDebtChange{}}, `{"movements":[],"final":[]}`)

	assertContractJSON(t, FeeDebtItem{ID: change.DebtID, CreditRef: "@fees#default"},
		`{"id":"1a2cf884-cf82-4520-9833-07d85c73bc14:fee-debit","creditRef":"@fees#default"}`)

	route := &FeeDebtRoute{ID: "0199f0a1-7c7e-7a4e-9a51-3d1b7c7f0a01", Code: "", Description: ""}
	assertContractJSON(t, FeeDebtItem{ID: change.DebtID, CreditRef: "@fees#default", DebitRoute: route, CreditRoute: route},
		`{"id":"1a2cf884-cf82-4520-9833-07d85c73bc14:fee-debit","creditRef":"@fees#default",`+
			`"debitRoute":{"id":"0199f0a1-7c7e-7a4e-9a51-3d1b7c7f0a01","code":"","description":""},`+
			`"creditRoute":{"id":"0199f0a1-7c7e-7a4e-9a51-3d1b7c7f0a01","code":"","description":""}}`)
}

// assertContractJSON locks field names, order, count and encodings, then proves
// the golden decodes and re-encodes to itself.
func assertContractJSON[T any](t *testing.T, value T, want string) {
	t.Helper()

	encoded, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("marshal contract: %v", err)
	}
	if string(encoded) != want {
		t.Fatalf("contract JSON changed:\ngot:  %s\nwant: %s", encoded, want)
	}
	var decoded T
	if err := json.Unmarshal([]byte(want), &decoded); err != nil {
		t.Fatalf("unmarshal contract: %v", err)
	}
	if reencoded, err := json.Marshal(decoded); err != nil || string(reencoded) != want {
		t.Fatalf("contract round trip changed JSON: %s (%v)", reencoded, err)
	}
}

func contractSnapshot() BalanceSnapshot {
	return BalanceSnapshot{
		BalanceRef:            "@source#default",
		ID:                    uuid.MustParse("fb1c1a90-ff37-41bd-a6eb-e3e1a8fd5454"),
		AccountID:             uuid.MustParse("8ea42cfc-6319-4d81-a7e2-3c509b5453c1"),
		AccountType:           "deposit",
		AssetCode:             "USD",
		Alias:                 "@source",
		Key:                   "default",
		Direction:             "credit",
		BalanceScope:          "default",
		Available:             decimal.RequireFromString("12345678901234567890.1234567890123456789"),
		OnHold:                decimal.RequireFromString("0.0000000000000000001"),
		OverdraftUsed:         decimal.RequireFromString("2.1234567890123456789"),
		OverdraftLimit:        decimal.RequireFromString("98765432109876543210.9876543210987654321"),
		Version:               9007199254740993,
		AllowSending:          true,
		AllowReceiving:        true,
		Blocked:               true,
		AllowOverdraft:        true,
		OverdraftLimitEnabled: true,
	}
}

func contractRoundTrip[T any](t *testing.T, input T) T {
	t.Helper()

	encoded, err := json.Marshal(input)
	if err != nil {
		t.Fatalf("marshal contract: %v", err)
	}
	var decoded T
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		t.Fatalf("unmarshal contract: %v", err)
	}

	return decoded
}
