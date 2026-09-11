// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"errors"
	"fmt"
	"reflect"
	"testing"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/domain/accounting"
)

type testEngineTechnicalError struct {
	code          string
	indeterminate bool
	cause         error
}

func (e testEngineTechnicalError) Error() string { return e.cause.Error() }
func (e testEngineTechnicalError) Unwrap() error { return e.cause }
func (e testEngineTechnicalError) EngineFailureCode() string {
	return e.code
}

func (e testEngineTechnicalError) OutcomeIndeterminate() bool {
	return e.indeterminate
}

func TestMapEngineError(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		code       string
		drawPolicy accounting.DrawPolicy
		wantCode   string
	}{
		{name: "insufficient funds", code: "insufficient_funds", wantCode: "0018"},
		{name: "overdraft limit", code: "overdraft_limit_exceeded", wantCode: "0167"},
		{name: "route denied", code: "overdraft_not_eligible", drawPolicy: accounting.DrawRouteDenied, wantCode: "0492"},
		{name: "draw forbidden", code: "overdraft_not_eligible", drawPolicy: accounting.DrawForbidden, wantCode: "0018"},
		{name: "balance deleted", code: "balance_deleted", wantCode: "0019"},
		{name: "balance missing", code: "balance_missing", wantCode: "0139"},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			request := accounting.Execution{Transactions: []accounting.Transaction{{Postings: []accounting.Posting{{
				BalanceRef: "balance-1",
				DrawPolicy: tt.drawPolicy,
			}}}}}
			failure := &accounting.Failure{Code: tt.code, TransactionIndex: 0, PostingIndex: 0, BalanceRef: "balance-1"}

			got := MapEngineError(request, failure)
			if code := errorCode(got); code != tt.wantCode {
				t.Fatalf("error code = %q, want %q (error: %v)", code, tt.wantCode, got)
			}
		})
	}
}

func TestMapEngineError_TechnicalPrecedence(t *testing.T) {
	t.Parallel()

	cause := &accounting.Failure{Code: "insufficient_funds", TransactionIndex: 0, PostingIndex: 0, BalanceRef: "balance-1"}
	technical := testEngineTechnicalError{code: "unknown_fingerprint", cause: cause}
	got := MapEngineError(accounting.Execution{}, technical)

	if !errors.Is(got, cause) {
		t.Fatalf("technical cause was not preserved: %v", got)
	}
	if code := errorCode(got); code != "" {
		t.Fatalf("technical error unexpectedly mapped to code %q", code)
	}
}

func TestMapEngineError_ExecutionGuardConflict(t *testing.T) {
	t.Parallel()

	cause := errors.New("guard conflict")
	confirmed := testEngineTechnicalError{code: "execution_guard_conflict", cause: cause}
	if got := errorCode(MapEngineError(accounting.Execution{}, confirmed)); got != "0486" {
		t.Fatalf("confirmed guard conflict code = %q, want 0486", got)
	}

	indeterminate := testEngineTechnicalError{code: "execution_guard_conflict", indeterminate: true, cause: cause}
	if got := MapEngineError(accounting.Execution{}, indeterminate); !errors.Is(got, cause) {
		t.Fatalf("indeterminate guard conflict lost cause: %v", got)
	}
}

func TestMapEngineError_NilAndUnmappedInputs(t *testing.T) {
	t.Parallel()

	if got := MapEngineError(accounting.Execution{}, nil); got != nil {
		t.Fatalf("nil error mapped to %v", got)
	}

	tests := []struct {
		name string
		err  error
		req  accounting.Execution
	}{
		{name: "non-engine error", err: errors.New("driver failed")},
		{name: "unknown code", err: &accounting.Failure{Code: "9999", BalanceRef: "balance-1"}, req: validEngineRequest()},
		{name: "transaction index", err: &accounting.Failure{Code: "insufficient_funds", TransactionIndex: 1, BalanceRef: "balance-1"}, req: validEngineRequest()},
		{name: "posting index", err: &accounting.Failure{Code: "insufficient_funds", PostingIndex: 1, BalanceRef: "balance-1"}, req: validEngineRequest()},
		{name: "empty reference", err: &accounting.Failure{Code: "insufficient_funds"}, req: validEngineRequest()},
		{name: "mismatched reference", err: &accounting.Failure{Code: "insufficient_funds", BalanceRef: "balance-2"}, req: validEngineRequest()},
		{name: "unknown requirement", err: &accounting.Failure{Code: accounting.FailureAssetMismatch, TransactionIndex: 0, PostingIndex: -1, BalanceRef: "balance-2"}, req: validRequirementEngineRequest(accounting.BalancePermissionSend, true)},
		{name: "wrong requirement permission", err: &accounting.Failure{Code: accounting.FailureReceivingNotAllowed, TransactionIndex: 0, PostingIndex: -1, BalanceRef: "balance-1"}, req: validRequirementEngineRequest(accounting.BalancePermissionSend, true)},
		{name: "unexpected allowed policy", err: &accounting.Failure{Code: "overdraft_not_eligible", BalanceRef: "balance-1"}, req: validEngineRequest()},
		{name: "companion missing", err: &accounting.Failure{Code: "overdraft_companion_missing", BalanceRef: "balance-1"}, req: validEngineRequest()},
		{name: "onhold underflow", err: &accounting.Failure{Code: "onhold_underflow", BalanceRef: "balance-1"}, req: validEngineRequest()},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := MapEngineError(tt.req, tt.err)
			if !errors.Is(got, tt.err) {
				t.Fatalf("original error was not preserved: %v", got)
			}
			if code := errorCode(got); code != "" {
				t.Fatalf("unmapped error unexpectedly has code %q", code)
			}
		})
	}
}

func TestMapEngineError_BalanceRequirements(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		name, code, want string
		permission       accounting.BalancePermission
		forbidExternal   bool
	}{
		{name: "asset", code: accounting.FailureAssetMismatch, want: "0034", permission: accounting.BalancePermissionSend},
		{name: "sending", code: accounting.FailureSendingNotAllowed, want: "0024", permission: accounting.BalancePermissionSend},
		{name: "receiving", code: accounting.FailureReceivingNotAllowed, want: "0024", permission: accounting.BalancePermissionReceive},
		{name: "external hold", code: accounting.FailureExternalHoldNotAllowed, want: "0098", permission: accounting.BalancePermissionSend, forbidExternal: true},
		{name: "deleted", code: accounting.FailureBalanceDeleted, want: "0019", permission: accounting.BalancePermissionSend},
	} {
		t.Run(test.name, func(t *testing.T) {
			request := validRequirementEngineRequest(test.permission, test.forbidExternal)
			failure := &accounting.Failure{Code: test.code, TransactionIndex: 0, PostingIndex: -1, BalanceRef: "balance-1"}
			if got := errorCode(MapEngineError(request, failure)); got != test.want {
				t.Fatalf("requirement failure code = %q, want %q", got, test.want)
			}
		})
	}
}

func validEngineRequest() accounting.Execution {
	return accounting.Execution{Transactions: []accounting.Transaction{{Postings: []accounting.Posting{{
		BalanceRef: "balance-1",
		DrawPolicy: accounting.DrawAllowed,
	}}}}}
}

func validRequirementEngineRequest(permission accounting.BalancePermission, forbidExternal bool) accounting.Execution {
	return accounting.Execution{
		Transactions: []accounting.Transaction{{BalanceRequirements: []accounting.BalanceRequirement{{
			BalanceRef: "balance-1", AssetCode: "USD", Permission: permission, ForbidExternal: forbidExternal,
		}}}},
		Balances: []accounting.BalanceSnapshot{{BalanceRef: "balance-1", Alias: "@source"}},
	}
}

func errorCode(err error) string {
	if err == nil {
		return ""
	}

	value := reflect.ValueOf(err)
	if value.Kind() == reflect.Pointer {
		value = value.Elem()
	}
	if !value.IsValid() || value.Kind() != reflect.Struct {
		return ""
	}
	field := value.FieldByName("Code")
	if !field.IsValid() || field.Kind() != reflect.String {
		return ""
	}
	return fmt.Sprint(field.Interface())
}
