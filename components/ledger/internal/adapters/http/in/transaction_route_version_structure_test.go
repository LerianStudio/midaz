// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"go/ast"
	"go/parser"
	"go/token"
	"testing"
)

// Route-version structural gates on the transport side. The fee seam's Gate 4
// (transaction_fee_seam_structure_test.go) proves every create shell binds the use case
// that matches its version; Gate 6 below covers what that gate cannot see.
//
// Gate 6 — COVERAGE. commit, cancel and revert reach the tracer through their own use
// cases, not through the create shell, so Gate 4 is blind to them. The method name IS the
// route version, so binding a /v1 route to a /v2 use case is the only way a /v1 client
// could start acquiring a reservation confirm it never asked for. Nothing at runtime
// would notice, so it is asserted over the source AST.
//
// The command-side counterpart — /v1 never NAMES a reservation seam, /v2 names both in
// the one order the contract allows — lives beside the pipelines themselves, in the
// command package.

// stateUseCaseCallees are the command entry points the state shells may bind. Only these
// names are reported by stateUseCaseCalls, so an unrelated call cannot satisfy a gate.
var stateUseCaseCallees = map[string]bool{
	"CommitTransactionV1": true,
	"CommitTransactionV2": true,
	"CancelTransactionV1": true,
	"CancelTransactionV2": true,
	"RevertTransactionV1": true,
	"RevertTransactionV2": true,
}

// stateUseCaseCalls returns the names of the versioned state use cases called in src, in
// source order.
func stateUseCaseCalls(t *testing.T, src string) []string {
	t.Helper()

	fset := token.NewFileSet()

	file, err := parser.ParseFile(fset, "src.go", src, 0)
	if err != nil {
		t.Fatalf("parse source: %v", err)
	}

	var callees []string

	ast.Inspect(file, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}

		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok {
			return true
		}

		if stateUseCaseCallees[sel.Sel.Name] {
			callees = append(callees, sel.Sel.Name)
		}

		return true
	})

	return callees
}

func TestRouteVersionStructure_V1StateShellsBindV1UseCases(t *testing.T) {
	src := readTransportSource(t, "transaction_handler_v1.go", "CommitTransactionV1")

	callees := stateUseCaseCalls(t, src)
	if len(callees) != 3 {
		t.Fatalf("Gate 6: expected the commit, cancel and revert shells to bind exactly three state use cases in transaction_handler_v1.go, got %v", callees)
	}

	want := map[string]bool{"CommitTransactionV1": true, "CancelTransactionV1": true, "RevertTransactionV1": true}

	for i, got := range callees {
		if !want[got] {
			t.Errorf("Gate 6: state call #%d in transaction_handler_v1.go binds %q — the /v1 contract carries no tracer, so every /v1 shell must bind a V1 use case", i, got)
		}
	}
}

func TestRouteVersionStructure_V2StateShellsBindV2UseCases(t *testing.T) {
	src := readTransportSource(t, "transaction_handler_v2.go", "CommitTransactionV2")

	callees := stateUseCaseCalls(t, src)
	if len(callees) != 3 {
		t.Fatalf("Gate 6: expected the commit, cancel and revert shells to bind exactly three state use cases in transaction_handler_v2.go, got %v", callees)
	}

	want := map[string]bool{"CommitTransactionV2": true, "CancelTransactionV2": true, "RevertTransactionV2": true}

	for i, got := range callees {
		if !want[got] {
			t.Errorf("Gate 6: state call #%d in transaction_handler_v2.go binds %q, want the matching V2 use case", i, got)
		}
	}
}

func TestRouteVersionStructure_GatesBite(t *testing.T) {
	// A shell binding the wrong version must be reported by name, not swallowed.
	const wrongVersion = `package in

func (handler *TransactionHandler) CommitTransaction(ctx context.Context) error {
	return handler.Command.CommitTransactionV2(ctx, command.PendingTransitionInput{})
}
`

	if got := stateUseCaseCalls(t, wrongVersion); len(got) != 1 || got[0] != "CommitTransactionV2" {
		t.Fatalf("Gate 6 bite: analyzer must report the wrong version, got %v", got)
	}

	// A shell that binds no state use case at all must report nothing, so a gate that
	// expects three bindings fails loudly instead of passing vacuously.
	const noBinding = `package in

func (handler *TransactionHandler) CommitTransaction(ctx context.Context) error {
	return handler.Command.SomethingElse(ctx)
}
`

	if got := stateUseCaseCalls(t, noBinding); len(got) != 0 {
		t.Fatalf("Gate 6 bite: analyzer must report an absent binding as empty, got %v", got)
	}

	// The correct shape must satisfy the gate, or the gate is unsatisfiable.
	const correct = `package in

func (handler *TransactionHandler) CommitTransaction(ctx context.Context) error {
	return handler.Command.CommitTransactionV1(ctx, command.PendingTransitionInput{})
}

func (handler *TransactionHandler) CancelTransaction(ctx context.Context) error {
	return handler.Command.CancelTransactionV1(ctx, command.PendingTransitionInput{})
}

func (handler *TransactionHandler) RevertTransaction(ctx context.Context) error {
	return handler.Command.RevertTransactionV1(ctx, command.RevertTransactionInput{})
}
`

	got := stateUseCaseCalls(t, correct)
	if len(got) != 3 || got[0] != "CommitTransactionV1" || got[1] != "CancelTransactionV1" || got[2] != "RevertTransactionV1" {
		t.Fatalf("Gate 6 bite: fixture sanity — the correct shape must report all three bindings in order, got %v", got)
	}
}
