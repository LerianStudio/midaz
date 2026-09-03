// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"strings"
	"testing"
)

// Gate 4 — route-version create binding: every /v1 create shell delegates to
// command.CreateTransactionV1 and the /v2 funnel to command.CreateTransactionV2. The
// method name IS the route version: CreateTransactionV1 names neither the fee engine
// nor the tracer reservation (the command-side negative gate asserts that), so binding
// a /v1 route to the /v2 use case is the only way a /v1 client could start acquiring
// fee legs, a tenant fee-DB resolution failure or a reservation rejection. Nothing at
// runtime would notice, so it is asserted over the source AST.

// createUseCaseCallees returns the names of the command create entry points called in
// src, in source order. Only the two versioned create methods are reported, so an
// unrelated call cannot satisfy the gate.
func createUseCaseCallees(t *testing.T, src string) []string {
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

		if sel.Sel.Name == "CreateTransactionV1" || sel.Sel.Name == "CreateTransactionV2" {
			callees = append(callees, sel.Sel.Name)
		}

		return true
	})

	return callees
}

// routeVersionArgName picks the route-version policy argument out of a call's
// argument list and returns its constant name (without the package qualifier), or ""
// when it is absent or is not one of the policy constants. The policy is recognised by
// value, so a reordering that moved it elsewhere in the list still reports it rather
// than silently passing.
func routeVersionArgName(args []ast.Expr) string {
	for _, arg := range args {
		sel, ok := arg.(*ast.SelectorExpr)
		if !ok {
			continue
		}

		pkg, ok := sel.X.(*ast.Ident)
		if !ok || pkg.Name != "command" {
			continue
		}

		if sel.Sel.Name == "RouteV1" || sel.Sel.Name == "RouteV2" {
			return sel.Sel.Name
		}
	}

	return ""
}

// readTransportSource reads a source file for a structural gate. mustContain is the
// sentinel the gate depends on being present: without it a renamed or moved seam would
// make the gate pass over a file it no longer describes.
func readTransportSource(t *testing.T, path, mustContain string) string {
	t.Helper()

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}

	src := string(data)
	if !strings.Contains(src, mustContain) {
		t.Fatalf("%s contains no %s — the gate is pointed at the wrong file", path, mustContain)
	}

	return src
}

func TestFeeSeamStructure_V1RoutesBindCreateTransactionV1(t *testing.T) {
	callees := createUseCaseCallees(t, readTransportSource(t, "transaction_handler.go", "createTransactionShellV1"))

	if len(callees) == 0 {
		t.Fatal("Gate 4: no command create call found in transaction_handler.go")
	}

	for i, got := range callees {
		if got != "CreateTransactionV1" {
			t.Errorf("Gate 4: create call #%d in transaction_handler.go binds %q, want CreateTransactionV1 — the /v1 contract carries neither the fee engine nor the tracer", i, got)
		}
	}
}

func TestFeeSeamStructure_V2FunnelBindsCreateTransactionV2(t *testing.T) {
	callees := createUseCaseCallees(t, readTransportSource(t, "transaction_handler_v2.go", "createTransactionV2"))

	if len(callees) == 0 {
		t.Fatal("Gate 4: no command create call found in transaction_handler_v2.go")
	}

	for i, got := range callees {
		if got != "CreateTransactionV2" {
			t.Errorf("Gate 4: create call #%d in transaction_handler_v2.go binds %q, want CreateTransactionV2", i, got)
		}
	}
}

func TestFeeSeamStructure_Gate4Bites(t *testing.T) {
	// A /v1 route bound to the /v2 use case — or to none at all — must fail the gate;
	// a gate that cannot bite is not a guard.
	const wrongVersion = `package in

func (handler *TransactionHandler) CreateTransactionJSON(ctx context.Context, in *X) (*Y, error) {
	return handler.Command.CreateTransactionV2(ctx, command.CreateTransactionV2Input{})
}
`

	if got := createUseCaseCallees(t, wrongVersion); len(got) != 1 || got[0] != "CreateTransactionV2" {
		t.Fatalf("Gate 4 bite: analyzer must report the wrong version, got %v", got)
	}

	const noCreate = `package in

func (handler *TransactionHandler) CreateTransactionJSON(ctx context.Context, in *X) (*Y, error) {
	return handler.Command.SomethingElse(ctx)
}
`

	if got := createUseCaseCallees(t, noCreate); len(got) != 0 {
		t.Fatalf("Gate 4 bite: analyzer must report no create binding as empty, got %v", got)
	}
}
