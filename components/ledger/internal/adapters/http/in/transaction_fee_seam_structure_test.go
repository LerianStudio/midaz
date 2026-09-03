// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"go/ast"
	"os"
	"strings"
	"testing"
)

// Gate 4 — route-version fee policy: every createTransactionShell call in the /v1
// transport file passes command.RouteV1, and the /v2 funnel passes command.RouteV2. The policy is
// what keeps the fee engine (and its tenant fee-DB resolution) off the /v1 contract, and
// nothing at runtime would notice a new /v1 route wired with the wrong constant — it
// would simply start acquiring fee legs and 503s. Asserted over the source AST so a
// future route cannot silently opt /v1 back in.

// shellRouteVersionArgs returns the identifier passed as the route-version policy argument
// of every
// createTransactionShell call in src. The argument is the one immediately preceding the
// variadic idempotency hash source, so it is read by name rather than by index: a
// non-identifier or absent policy argument yields an empty entry and fails the gate.
func shellRouteVersionArgs(t *testing.T, src string) []string {
	t.Helper()

	return callRouteVersionArgs(t, src, "createTransactionShell")
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

func TestFeeSeamStructure_V1RoutesPassRouteV1(t *testing.T) {
	policies := shellRouteVersionArgs(t, readTransportSource(t, "transaction_handler.go", "createTransactionShell"))

	if len(policies) == 0 {
		t.Fatal("Gate 4: no createTransactionShell call found in transaction_handler.go")
	}

	for i, got := range policies {
		if got != "RouteV1" {
			t.Errorf("Gate 4: createTransactionShell call #%d in transaction_handler.go passes %q, want command.RouteV1 — the /v1 contract carries no fee engine", i, got)
		}
	}
}

func TestFeeSeamStructure_V2FunnelPassesRouteV2(t *testing.T) {
	policies := shellRouteVersionArgs(t, readTransportSource(t, "transaction_handler_v2.go", "createTransactionShell"))

	if len(policies) == 0 {
		t.Fatal("Gate 4: no createTransactionShell call found in transaction_handler_v2.go")
	}

	for i, got := range policies {
		if got != "RouteV2" {
			t.Errorf("Gate 4: createTransactionShell call #%d in transaction_handler_v2.go passes %q, want command.RouteV2", i, got)
		}
	}
}

func TestFeeSeamStructure_Gate4Bites(t *testing.T) {
	// A /v1 route wired with the wrong constant — or with none at all — must fail the
	// gate; a gate that cannot bite is not a guard.
	const wrongConstant = `package in

func (handler *TransactionHandler) CreateTransactionJSON(ctx context.Context, in *X) (*Y, error) {
	return handler.createTransactionShell(ctx, in.OrganizationID, in.LedgerID, t, s, in.IdempotencyKey, in.IdempotencyTTL, command.RouteV2)
}
`

	if got := shellRouteVersionArgs(t, wrongConstant); len(got) != 1 || got[0] != "RouteV2" {
		t.Fatalf("Gate 4 bite: analyzer must report the wrong constant, got %v", got)
	}

	const noPolicy = `package in

func (handler *TransactionHandler) CreateTransactionJSON(ctx context.Context, in *X) (*Y, error) {
	return handler.createTransactionShell(ctx, in.OrganizationID, in.LedgerID, t, s, in.IdempotencyKey, in.IdempotencyTTL)
}
`

	if got := shellRouteVersionArgs(t, noPolicy); len(got) != 1 || got[0] != "" {
		t.Fatalf("Gate 4 bite: analyzer must report an absent policy as empty, got %v", got)
	}
}
