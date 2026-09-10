// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"errors"
	"fmt"
	"reflect"
	"testing"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/engine"
)

type testBalanceEngineTechnicalError struct {
	code          string
	indeterminate bool
	cause         error
}

func (e testBalanceEngineTechnicalError) Error() string { return e.cause.Error() }
func (e testBalanceEngineTechnicalError) Unwrap() error { return e.cause }
func (e testBalanceEngineTechnicalError) EngineFailureCode() string {
	return e.code
}

func (e testBalanceEngineTechnicalError) OutcomeIndeterminate() bool {
	return e.indeterminate
}

func TestMapBalanceEngineError(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		code       string
		drawPolicy engine.DrawPolicy
		wantCode   string
	}{
		{name: "insufficient funds", code: "insufficient_funds", wantCode: "0018"},
		{name: "overdraft limit", code: "overdraft_limit_exceeded", wantCode: "0167"},
		{name: "route denied", code: "overdraft_not_eligible", drawPolicy: engine.DrawRouteDenied, wantCode: "0492"},
		{name: "draw forbidden", code: "overdraft_not_eligible", drawPolicy: engine.DrawForbidden, wantCode: "0018"},
		{name: "balance deleted", code: "balance_deleted", wantCode: "0019"},
		{name: "balance missing", code: "balance_missing", wantCode: "0139"},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			request := engine.Request{Transactions: []engine.Transaction{{Postings: []engine.Posting{{
				BalanceRef: "balance-1",
				DrawPolicy: tt.drawPolicy,
			}}}}}
			failure := &engine.Failure{Code: tt.code, TransactionIndex: 0, PostingIndex: 0, BalanceRef: "balance-1"}

			got := MapBalanceEngineError(request, failure)
			if code := errorCode(got); code != tt.wantCode {
				t.Fatalf("error code = %q, want %q (error: %v)", code, tt.wantCode, got)
			}
		})
	}
}

func TestMapBalanceEngineError_TechnicalPrecedence(t *testing.T) {
	t.Parallel()

	cause := &engine.Failure{Code: "insufficient_funds", TransactionIndex: 0, PostingIndex: 0, BalanceRef: "balance-1"}
	technical := testBalanceEngineTechnicalError{code: "unknown_fingerprint", cause: cause}
	got := MapBalanceEngineError(engine.Request{}, technical)

	if !errors.Is(got, cause) {
		t.Fatalf("technical cause was not preserved: %v", got)
	}
	if code := errorCode(got); code != "" {
		t.Fatalf("technical error unexpectedly mapped to code %q", code)
	}
}

func TestMapBalanceEngineError_ExecutionGuardConflict(t *testing.T) {
	t.Parallel()

	cause := errors.New("guard conflict")
	confirmed := testBalanceEngineTechnicalError{code: "execution_guard_conflict", cause: cause}
	if got := errorCode(MapBalanceEngineError(engine.Request{}, confirmed)); got != "0486" {
		t.Fatalf("confirmed guard conflict code = %q, want 0486", got)
	}

	indeterminate := testBalanceEngineTechnicalError{code: "execution_guard_conflict", indeterminate: true, cause: cause}
	if got := MapBalanceEngineError(engine.Request{}, indeterminate); !errors.Is(got, cause) {
		t.Fatalf("indeterminate guard conflict lost cause: %v", got)
	}
}

func TestMapBalanceEngineError_NilAndUnmappedInputs(t *testing.T) {
	t.Parallel()

	if got := MapBalanceEngineError(engine.Request{}, nil); got != nil {
		t.Fatalf("nil error mapped to %v", got)
	}

	tests := []struct {
		name string
		err  error
		req  engine.Request
	}{
		{name: "non-engine error", err: errors.New("driver failed")},
		{name: "unknown code", err: &engine.Failure{Code: "9999", BalanceRef: "balance-1"}, req: validEngineRequest()},
		{name: "transaction index", err: &engine.Failure{Code: "insufficient_funds", TransactionIndex: 1, BalanceRef: "balance-1"}, req: validEngineRequest()},
		{name: "posting index", err: &engine.Failure{Code: "insufficient_funds", PostingIndex: 1, BalanceRef: "balance-1"}, req: validEngineRequest()},
		{name: "empty reference", err: &engine.Failure{Code: "insufficient_funds"}, req: validEngineRequest()},
		{name: "mismatched reference", err: &engine.Failure{Code: "insufficient_funds", BalanceRef: "balance-2"}, req: validEngineRequest()},
		{name: "unknown requirement", err: &engine.Failure{Code: engine.FailureAssetMismatch, TransactionIndex: 0, PostingIndex: -1, BalanceRef: "balance-2"}, req: validRequirementEngineRequest(engine.BalancePermissionSend, true)},
		{name: "wrong requirement permission", err: &engine.Failure{Code: engine.FailureReceivingNotAllowed, TransactionIndex: 0, PostingIndex: -1, BalanceRef: "balance-1"}, req: validRequirementEngineRequest(engine.BalancePermissionSend, true)},
		{name: "unexpected allowed policy", err: &engine.Failure{Code: "overdraft_not_eligible", BalanceRef: "balance-1"}, req: validEngineRequest()},
		{name: "companion missing", err: &engine.Failure{Code: "overdraft_companion_missing", BalanceRef: "balance-1"}, req: validEngineRequest()},
		{name: "onhold underflow", err: &engine.Failure{Code: "onhold_underflow", BalanceRef: "balance-1"}, req: validEngineRequest()},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := MapBalanceEngineError(tt.req, tt.err)
			if !errors.Is(got, tt.err) {
				t.Fatalf("original error was not preserved: %v", got)
			}
			if code := errorCode(got); code != "" {
				t.Fatalf("unmapped error unexpectedly has code %q", code)
			}
		})
	}
}

func TestMapBalanceEngineError_BalanceRequirements(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		name, code, want string
		permission       engine.BalancePermission
		forbidExternal   bool
	}{
		{name: "asset", code: engine.FailureAssetMismatch, want: "0034", permission: engine.BalancePermissionSend},
		{name: "sending", code: engine.FailureSendingNotAllowed, want: "0024", permission: engine.BalancePermissionSend},
		{name: "receiving", code: engine.FailureReceivingNotAllowed, want: "0024", permission: engine.BalancePermissionReceive},
		{name: "external hold", code: engine.FailureExternalHoldNotAllowed, want: "0098", permission: engine.BalancePermissionSend, forbidExternal: true},
		{name: "deleted", code: engine.FailureBalanceDeleted, want: "0019", permission: engine.BalancePermissionSend},
	} {
		t.Run(test.name, func(t *testing.T) {
			request := validRequirementEngineRequest(test.permission, test.forbidExternal)
			failure := &engine.Failure{Code: test.code, TransactionIndex: 0, PostingIndex: -1, BalanceRef: "balance-1"}
			if got := errorCode(MapBalanceEngineError(request, failure)); got != test.want {
				t.Fatalf("requirement failure code = %q, want %q", got, test.want)
			}
		})
	}
}

func validEngineRequest() engine.Request {
	return engine.Request{Transactions: []engine.Transaction{{Postings: []engine.Posting{{
		BalanceRef: "balance-1",
		DrawPolicy: engine.DrawAllowed,
	}}}}}
}

func validRequirementEngineRequest(permission engine.BalancePermission, forbidExternal bool) engine.Request {
	return engine.Request{
		Transactions: []engine.Transaction{{BalanceRequirements: []engine.BalanceRequirement{{
			BalanceRef: "balance-1", AssetCode: "USD", Permission: permission, ForbidExternal: forbidExternal,
		}}}},
		Balances: []engine.BalanceSnapshot{{BalanceRef: "balance-1", Alias: "@source"}},
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
