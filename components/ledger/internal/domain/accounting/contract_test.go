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
	snapshot := contractSnapshot()
	request := Execution{
		OrganizationID: uuid.MustParse("139c4166-2139-4f17-b282-fba78e8c4c2a"),
		LedgerID:       uuid.MustParse("c029e784-535d-4554-aae3-65b6e713687f"),
		ExecutionID:    uuid.MustParse("95b4a433-59b3-4ac8-ad6f-e4be032ebca6"),
		Transactions: []Transaction{{
			ID: transactionID,
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

	// Typed Go JSON round trips do not define the adapter's Lua wire DTO.
	got := contractRoundTrip(t, request)
	if !reflect.DeepEqual(got, request) {
		t.Fatalf("request round trip changed identifiers, monetary values, versions or settings:\ngot: %#v\nwant: %#v", got, request)
	}
	identifiers := []uuid.UUID{
		got.OrganizationID, got.LedgerID, got.ExecutionID, got.Transactions[0].ID,
		got.Balances[0].ID, got.Balances[0].AccountID,
	}
	for i, identifier := range identifiers {
		if identifier == uuid.Nil {
			t.Errorf("identifier %d lost its UUID value", i)
		}
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
	}
	if len(postings) != 6 {
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
	if RolePrimary != "primary" || RoleOverdraftCompanion != "overdraft_companion" {
		t.Fatal("movement roles changed")
	}
}

func TestFailureContract(t *testing.T) {
	t.Parallel()

	codes := map[string]string{
		FailureInsufficientFunds:         "insufficient_funds",
		FailureOverdraftLimitExceeded:    "overdraft_limit_exceeded",
		FailureOverdraftNotEligible:      "overdraft_not_eligible",
		FailureOverdraftCompanionMissing: "overdraft_companion_missing",
		FailureBalanceDeleted:            "balance_deleted",
		FailureAccountBlocked:            "account_blocked",
		FailureOnHoldUnderflow:           "onhold_underflow",
		FailureBalanceMissing:            "balance_missing",
		FailureAssetMismatch:             "asset_mismatch",
		FailureSendingNotAllowed:         "sending_not_allowed",
		FailureReceivingNotAllowed:       "receiving_not_allowed",
		FailureExternalHoldNotAllowed:    "external_hold_not_allowed",
	}
	if len(codes) != 12 {
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
