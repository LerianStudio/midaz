// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"go/ast"
	"go/parser"
	"go/printer"
	"go/token"
	"io/fs"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// routeMountHelpers are the functions that mount a guard chain on a Fiber router,
// mapped to the argument index at which each one takes the route's path.
var routeMountHelpers = map[string]int{
	"routePost":     1,
	"routeGet":      1,
	"routePatch":    1,
	"routePut":      1,
	"routeDelete":   1,
	"routeHead":     1,
	"registerRoute": 2,
}

// exprText renders an expression back to source so two path expressions can be
// compared as the programmer wrote them — idPath, base+"/metrics" — instead of only
// when both happen to be literals. Comparing the written expression is the point: the
// two must be the SAME expression, not two expressions that agree today.
func exprText(fset *token.FileSet, expr ast.Expr) string {
	var b strings.Builder

	if err := printer.Fprint(&b, fset, expr); err != nil {
		return "<unprintable>"
	}

	return b.String()
}

// isProtectedMidazCall reports whether expr is a protectedMidaz(...) call.
func isProtectedMidazCall(expr ast.Expr) (*ast.CallExpr, bool) {
	call, ok := expr.(*ast.CallExpr)
	if !ok {
		return nil, false
	}

	ident, ok := call.Fun.(*ast.Ident)
	if !ok || ident.Name != "protectedMidaz" {
		return nil, false
	}

	return call, true
}

// TestRouteGuardCoverage_EveryMountedChainDeclaresItsOwnPath is the gate against silent
// forgetting. It reads the package's own source and requires, for every route mounted
// with a protectedMidaz chain, that the path handed to protectedMidaz is the SAME
// expression as the path the route is mounted on.
//
// The compiler cannot do this. It demands the argument be PRESENT, never that it be
// RIGHT: a path copied from a neighbouring route compiles, answers 200, and authorizes
// against a different organization or ledger than the one the caller addressed. The
// count check on the second half is what catches a chain built outside a mount helper,
// where this comparison would never look.
func TestRouteGuardCoverage_EveryMountedChainDeclaresItsOwnPath(t *testing.T) {
	t.Parallel()

	fset := token.NewFileSet()

	pkgs, err := parser.ParseDir(fset, ".", func(info fs.FileInfo) bool {
		return !strings.HasSuffix(info.Name(), "_test.go")
	}, parser.SkipObjectResolution)
	require.NoError(t, err)

	pkg, ok := pkgs["in"]
	require.True(t, ok, "package in must parse from its own directory")

	var mounted, guarded int

	for _, file := range pkg.Files {
		ast.Inspect(file, func(node ast.Node) bool {
			call, isCall := node.(*ast.CallExpr)
			if !isCall {
				return true
			}

			if _, isGuard := isProtectedMidazCall(call); isGuard {
				guarded++

				return true
			}

			fn, isIdent := call.Fun.(*ast.Ident)
			if !isIdent {
				return true
			}

			pathArg, isMount := routeMountHelpers[fn.Name]
			if !isMount || len(call.Args) <= pathArg+1 {
				return true
			}

			guard, isGuarded := isProtectedMidazCall(call.Args[pathArg+1])
			if !isGuarded {
				return true
			}

			mounted++

			require.GreaterOrEqualf(t, len(guard.Args), 2,
				"%s: protectedMidaz must carry the route path", fset.Position(call.Pos()))

			routePath := exprText(fset, call.Args[pathArg])
			declaredPath := exprText(fset, guard.Args[1])

			assert.Equalf(t, routePath, declaredPath,
				"%s: %s mounts %s but its guard chain declares %s — the route would authorize against another instance",
				fset.Position(call.Pos()), fn.Name, routePath, declaredPath)

			return true
		})
	}

	assert.Positive(t, mounted, "the package must mount at least one guarded route")
	assert.Equalf(t, guarded, mounted,
		"every protectedMidaz chain must be mounted by a route helper, so this gate sees it: %d chains built, %d mounted",
		guarded, mounted)
}
