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
// (transaction_fee_seam_structure_test.go) proves every createTransactionShell call
// names its policy; Gate 6 below covers what that gate cannot see.
//
// Gate 6 — COVERAGE. commit, cancel and revert reach the tracer through their own paths,
// not through the create shell, so Gate 4 is blind to them. Commit and cancel are not
// split by version yet, so they still name a policy constant and a wrong one would
// silently start reaching the tracer from a /v1 route. Revert IS split, so its gate is
// the same as create's: the route must bind the use case that matches its version.
//
// Gate 5 (the route gate must be the FIRST statement of each by-transaction tracer seam)
// lives beside the seams themselves, in the command package.

// callRouteVersionArgs returns the route-version constant passed by every call to
// callee in src, in source order. An absent or unrecognised policy argument yields an
// empty entry, which fails the gates below.
func callRouteVersionArgs(t *testing.T, src, callee string) []string {
	t.Helper()

	fset := token.NewFileSet()

	file, err := parser.ParseFile(fset, "src.go", src, 0)
	if err != nil {
		t.Fatalf("parse source: %v", err)
	}

	var policies []string

	ast.Inspect(file, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}

		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok || sel.Sel.Name != callee {
			return true
		}

		policies = append(policies, routeVersionArgName(call.Args))

		return true
	})

	return policies
}

func TestRouteVersionStructure_V1StateRoutesPassRouteV1(t *testing.T) {
	src := readTransportSource(t, "transaction_handler.go", "commitTransaction")

	policies := callRouteVersionArgs(t, src, "commitTransaction")
	if len(policies) == 0 {
		t.Fatal("Gate 6: no commitTransaction call found in transaction_handler.go")
	}

	for i, got := range policies {
		if got != "RouteV1" {
			t.Errorf("Gate 6: commitTransaction call #%d in transaction_handler.go passes %q, want command.RouteV1 — the /v1 contract carries no tracer", i, got)
		}
	}

	reverts := revertUseCaseCallees(t, src)
	if len(reverts) == 0 {
		t.Fatal("Gate 6: no command revert call found in transaction_handler.go")
	}

	for i, got := range reverts {
		if got != "RevertTransactionV1" {
			t.Errorf("Gate 6: revert call #%d in transaction_handler.go binds %q, want RevertTransactionV1 — the /v1 contract carries no tracer", i, got)
		}
	}
}

func TestRouteVersionStructure_V2StateRoutesPassRouteV2(t *testing.T) {
	src := readTransportSource(t, "transaction_handler_v2.go", "commitTransaction")

	policies := callRouteVersionArgs(t, src, "commitTransaction")
	if len(policies) == 0 {
		t.Fatal("Gate 6: no commitTransaction call found in transaction_handler_v2.go")
	}

	for i, got := range policies {
		if got != "RouteV2" {
			t.Errorf("Gate 6: commitTransaction call #%d in transaction_handler_v2.go passes %q, want command.RouteV2", i, got)
		}
	}

	reverts := revertUseCaseCallees(t, src)
	if len(reverts) == 0 {
		t.Fatal("Gate 6: no command revert call found in transaction_handler_v2.go")
	}

	for i, got := range reverts {
		if got != "RevertTransactionV2" {
			t.Errorf("Gate 6: revert call #%d in transaction_handler_v2.go binds %q, want RevertTransactionV2", i, got)
		}
	}
}

// revertUseCaseCallees returns the names of the command revert entry points called in
// src, in source order. Only the two versioned revert methods are reported, so an
// unrelated call cannot satisfy the gate.
func revertUseCaseCallees(t *testing.T, src string) []string {
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

		if sel.Sel.Name == "RevertTransactionV1" || sel.Sel.Name == "RevertTransactionV2" {
			callees = append(callees, sel.Sel.Name)
		}

		return true
	})

	return callees
}

func TestRouteVersionStructure_GatesBite(t *testing.T) {
	// The revert half must report the wrong version.
	const wrongRevert = `package in

func (handler *TransactionHandler) RevertTransaction(ctx context.Context) error {
	return handler.Command.RevertTransactionV2(ctx, command.RevertTransactionInput{})
}
`

	if got := revertUseCaseCallees(t, wrongRevert); len(got) != 1 || got[0] != "RevertTransactionV2" {
		t.Fatalf("Gate 6 bite: analyzer must report the wrong revert version, got %v", got)
	}

	const noRevert = `package in

func (handler *TransactionHandler) RevertTransaction(ctx context.Context) error {
	return handler.Command.SomethingElse(ctx)
}
`

	if got := revertUseCaseCallees(t, noRevert); len(got) != 0 {
		t.Fatalf("Gate 6 bite: analyzer must report no revert binding as empty, got %v", got)
	}

	// The commit half must report the wrong constant, and an absent one as empty.
	const wrongConstant = `package in

func (handler *TransactionHandler) CommitTransaction(ctx context.Context) error {
	return handler.commitTransaction(ctx, orgID, ledgerID, txID, constant.APPROVED, command.RouteV2)
}
`

	if got := callRouteVersionArgs(t, wrongConstant, "commitTransaction"); len(got) != 1 || got[0] != "RouteV2" {
		t.Fatalf("Gate 6 bite: analyzer must report the wrong constant, got %v", got)
	}

	const noPolicy = `package in

func (handler *TransactionHandler) CommitTransaction(ctx context.Context) error {
	return handler.commitTransaction(ctx, orgID, ledgerID, txID, constant.APPROVED)
}
`

	if got := callRouteVersionArgs(t, noPolicy, "commitTransaction"); len(got) != 1 || got[0] != "" {
		t.Fatalf("Gate 6 bite: analyzer must report an absent policy as empty, got %v", got)
	}
}
