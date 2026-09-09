// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestNewRoutes_WrapsTheAPIWithTheErrorReference pins the Tracer's production
// wiring of the per-occurrence error reference.
//
// The reference's own unit tests live in pkg/net/http and call the decorator
// themselves, so removing every production caller leaves them green — the
// customer-facing defect can return with the suite passing. The ledger's mount
// point is pinned behaviourally by
// TestAssembleHumaContract_WiresTheErrorReference, which reads a real error body
// off the wire.
//
// This one is structural, and deliberately so: the Tracer builds its Huma API
// inside NewRoutes, and this package's TestMain disables the telemetry
// middleware to avoid a data race in the shared logger. With no span on the
// request there is no trace to reference, so an on-the-wire assertion here would
// pass whether the decorator is installed or not — the weaker kind of test this
// exists to prevent. Reading the source instead makes the deletion of the line
// the failure, which is the failure mode being guarded.
//
// It also enforces the ordering the decorator depends on: huma.Register captures
// the API it is handed, so wrapping must happen before the first registration or
// it reaches no operation.
func TestNewRoutes_WrapsTheAPIWithTheErrorReference(t *testing.T) {
	t.Parallel()

	const (
		decorator = "WithProblemInstance"
		builder   = "New" // openapi.New, which returns the API to be wrapped
	)

	path := routesSourcePath(t)

	fset := token.NewFileSet()

	file, err := parser.ParseFile(fset, path, nil, parser.SkipObjectResolution)
	require.NoError(t, err, "parse routes.go")

	var (
		decoratorLine int
		builderLine   int
		registerLine  int
	)

	ast.Inspect(file, func(node ast.Node) bool {
		call, ok := node.(*ast.CallExpr)
		if !ok {
			return true
		}

		selector, ok := call.Fun.(*ast.SelectorExpr)
		if !ok {
			return true
		}

		line := fset.Position(call.Pos()).Line

		switch {
		case selector.Sel.Name == decorator && decoratorLine == 0:
			decoratorLine = line
		case selector.Sel.Name == builder && isIdent(selector.X, "openapi") && builderLine == 0:
			builderLine = line
		case selector.Sel.Name == "Register" && isIdent(selector.X, "huma") && registerLine == 0:
			registerLine = line
		}

		return true
	})

	require.NotZerof(t, builderLine,
		"routes.go must build a Huma API through openapi.New for this assertion to mean anything")

	require.NotZerof(t, decoratorLine,
		"routes.go must pass its Huma API through %s, or no Tracer error body carries a reference a customer can quote to support",
		decorator)

	require.Greaterf(t, decoratorLine, builderLine,
		"%s must wrap the API openapi.New returns (line %d), not something built before it",
		decorator, builderLine)

	if registerLine != 0 {
		require.Lessf(t, decoratorLine, registerLine,
			"%s must run BEFORE the first huma.Register (line %d): Register captures the API it is handed, so a later wrap reaches no operation",
			decorator, registerLine)
	}
}

// isIdent reports whether an expression is the named identifier, so a selector
// like openapi.New is matched on both halves rather than on the method name
// alone.
func isIdent(expr ast.Expr, name string) bool {
	ident, ok := expr.(*ast.Ident)

	return ok && ident.Name == name
}

// routesSourcePath locates routes.go relative to this test, which runs with its
// own package directory as the working directory.
func routesSourcePath(t *testing.T) string {
	t.Helper()

	dir, err := os.Getwd()
	require.NoError(t, err)

	path := filepath.Join(dir, "routes.go")

	_, err = os.Stat(path)
	require.NoError(t, err, "routes.go must sit beside this test")

	return path
}
