// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"strconv"
	"strings"
	"testing"
)

// transactionSpanNames locks the span the transactional pipelines open, one row per
// function. It exists so the next rename is a deliberate table edit instead of an
// incidental string change: a span name is a dashboard/query key, and changing one
// here requires coordinating with docs/dashboards/v4 (and any saved Grafana query)
// before it lands.
var transactionSpanNames = []struct {
	file  string
	fn    string
	names []string
}{
	{"create_transaction_v1.go", "CreateTransactionV1", []string{"command.create_transaction_v1"}},
	{"create_transaction_v2.go", "CreateTransactionV2", []string{"command.create_transaction_v2"}},
	{"revert_transaction.go", "RevertTransactionV1", []string{"command.revert_transaction_v1"}},
	{"revert_transaction.go", "RevertTransactionV2", []string{"command.revert_transaction_v2"}},
	{"commit_transaction.go", "CommitTransactionV1", []string{"command.commit_transaction_v1"}},
	{"commit_transaction.go", "CancelTransactionV1", []string{"command.cancel_transaction_v1"}},
	{"commit_transaction.go", "CommitTransactionV2", []string{"command.commit_transaction_v2"}},
	{"commit_transaction.go", "CancelTransactionV2", []string{"command.cancel_transaction_v2"}},
	{"commit_transaction.go", "transitionPendingV1", []string{"command.transition_pending_transaction"}},
	{"commit_transaction.go", "transitionPendingV2", []string{"command.transition_pending_transaction"}},
	{"transition_pending_steps.go", "commitPendingBalances", []string{"command.transition_pending_transaction.pre_seed_backup"}},
	{"transition_pending_steps.go", "finalizePendingTransition", []string{"command.transition_pending_transaction.send_to_redis_queue"}},
	{"build_transaction_operations.go", "BuildOperations", []string{"command.build_transaction_operations"}},
	{"build_transaction_operations.go", "buildDoubleEntryPendingOps", []string{"command.build_double_entry_pending_ops"}},
	{"build_transaction_operations.go", "buildDoubleEntryCanceledOps", []string{"command.build_double_entry_canceled_ops"}},
}

// spanNamesIn returns, in source order, the literal string passed as the span-name
// argument to every tracer.Start call inside the named function.
func spanNamesIn(t *testing.T, src, funcName string) []string {
	t.Helper()

	fn := findFuncDecl(t, src, funcName)

	var names []string

	ast.Inspect(fn, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}

		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok || sel.Sel.Name != "Start" {
			return true
		}

		if ident, ok := sel.X.(*ast.Ident); !ok || ident.Name != "tracer" {
			return true
		}

		if len(call.Args) < 2 {
			return true
		}

		lit, ok := call.Args[1].(*ast.BasicLit)
		if !ok || lit.Kind != token.STRING {
			return true
		}

		unquoted, err := strconv.Unquote(lit.Value)
		if err != nil {
			t.Fatalf("unquote span literal %s in %s: %v", lit.Value, funcName, err)
		}

		names = append(names, unquoted)

		return true
	})

	return names
}

// TestTransactionSpanNames_LockedTable pins the exact span each transactional
// pipeline function opens. A diff here is the signal that a rename needs a dashboard
// and saved-query check before it merges.
func TestTransactionSpanNames_LockedTable(t *testing.T) {
	for _, tc := range transactionSpanNames {
		t.Run(tc.fn, func(t *testing.T) {
			src := readTransportSource(t, tc.file, "func (uc *UseCase) "+tc.fn)
			got := spanNamesIn(t, src, tc.fn)

			if len(got) != len(tc.names) {
				t.Fatalf("%s opens spans %v, want %v", tc.fn, got, tc.names)
			}

			for i, want := range tc.names {
				if got[i] != want {
					t.Errorf("%s span[%d] = %q, want %q — renaming a span requires coordinating with dashboards (docs/dashboards/v4) first", tc.fn, i, got[i], want)
				}
			}
		})
	}
}

// handlerPrefixedSpanNames returns every span-name string literal passed to a
// tracer.Start call in src that begins with the handler. prefix. The context
// argument is deliberately unconstrained: a span opened on a derived context
// (readCtx, spanCtx) carries the same dashboard key as one opened on ctx.
func handlerPrefixedSpanNames(t *testing.T, filename, src string) []string {
	t.Helper()

	fset := token.NewFileSet()

	file, err := parser.ParseFile(fset, filename, src, 0)
	if err != nil {
		t.Fatalf("parse %s: %v", filename, err)
	}

	var names []string

	ast.Inspect(file, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}

		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok || sel.Sel.Name != "Start" {
			return true
		}

		if ident, ok := sel.X.(*ast.Ident); !ok || ident.Name != "tracer" {
			return true
		}

		if len(call.Args) < 2 {
			return true
		}

		lit, ok := call.Args[1].(*ast.BasicLit)
		if !ok || lit.Kind != token.STRING {
			return true
		}

		unquoted, err := strconv.Unquote(lit.Value)
		if err != nil {
			t.Fatalf("unquote span literal %s in %s: %v", lit.Value, filename, err)
		}

		if strings.HasPrefix(unquoted, "handler.") {
			names = append(names, unquoted)
		}

		return true
	})

	return names
}

// TestTransactionPackage_NeverOpensHandlerPrefixedSpans is the negative gate: no
// source file in this package may open a span under the handler.* prefix, which
// belongs to the HTTP adapter, not to services/command. Its failure message is the
// same warning as the locked table above: coordinate with dashboards before renaming.
func TestTransactionPackage_NeverOpensHandlerPrefixedSpans(t *testing.T) {
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("read command package directory: %v", err)
	}

	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}

		data, err := os.ReadFile(name)
		if err != nil {
			t.Fatalf("read %s: %v", name, err)
		}

		for _, span := range handlerPrefixedSpanNames(t, name, string(data)) {
			t.Errorf("%s opens a span under the handler. prefix (%q) — the command package convention is command.<snake_op>; renaming a span requires coordinating with dashboards (docs/dashboards/v4) before it lands", name, span)
		}
	}
}

// TestHandlerPrefixedSpanNames_IgnoresContextArgument proves the gate bites on the
// call shape a substring match missed: only the span name decides, never which
// context variable the call receives.
func TestHandlerPrefixedSpanNames_IgnoresContextArgument(t *testing.T) {
	tests := []struct {
		name string
		src  string
		want []string
	}{
		{
			name: "unmarked ctx",
			src:  "package p\n\nfunc f() { tracer.Start(ctx, \"handler.create\") }\n",
			want: []string{"handler.create"},
		},
		{
			name: "derived ctx",
			src:  "package p\n\nfunc f() { tracer.Start(readCtx, \"handler.create\") }\n",
			want: []string{"handler.create"},
		},
		{
			name: "command prefix is allowed",
			src:  "package p\n\nfunc f() { tracer.Start(readCtx, \"command.create_transaction_v2\") }\n",
			want: nil,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := handlerPrefixedSpanNames(t, "snippet.go", tc.src)

			if len(got) != len(tc.want) {
				t.Fatalf("handlerPrefixedSpanNames() = %v, want %v", got, tc.want)
			}

			for i, want := range tc.want {
				if got[i] != want {
					t.Errorf("span[%d] = %q, want %q", i, got[i], want)
				}
			}
		})
	}
}
