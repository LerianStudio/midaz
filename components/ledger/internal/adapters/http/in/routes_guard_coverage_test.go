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
	"strconv"
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

// constStrings resolves every string-valued constant declared anywhere in file,
// folding "+" concatenation so a path assembled from parts — idPath = listPath +
// "/:organization_id" — is checked as the path it actually becomes. Without the
// folding the check would only ever see paths written as one literal, which is the
// minority: most route files build theirs from a base constant.
func constStrings(file *ast.File) map[string]string {
	exprs := make(map[string]ast.Expr)

	ast.Inspect(file, func(node ast.Node) bool {
		spec, isValue := node.(*ast.ValueSpec)
		if !isValue {
			return true
		}

		for i, name := range spec.Names {
			if i < len(spec.Values) {
				exprs[name.Name] = spec.Values[i]
			}
		}

		return true
	})

	resolved := make(map[string]string, len(exprs))

	var resolve func(expr ast.Expr, depth int) (string, bool)

	resolve = func(expr ast.Expr, depth int) (string, bool) {
		if depth > 16 {
			return "", false
		}

		switch e := expr.(type) {
		case *ast.BasicLit:
			if e.Kind != token.STRING {
				return "", false
			}

			value, err := strconv.Unquote(e.Value)

			return value, err == nil
		case *ast.Ident:
			target, known := exprs[e.Name]
			if !known {
				return "", false
			}

			return resolve(target, depth+1)
		case *ast.BinaryExpr:
			if e.Op != token.ADD {
				return "", false
			}

			left, leftOK := resolve(e.X, depth+1)
			right, rightOK := resolve(e.Y, depth+1)

			return left + right, leftOK && rightOK
		default:
			return "", false
		}
	}

	for name, expr := range exprs {
		if value, ok := resolve(expr, 0); ok {
			resolved[name] = value
		}
	}

	return resolved
}

// TestRouteGuardCoverage_OrganizationSegmentAlwaysNamesTheOrganization is the gate
// against a route re-spelling the organization parameter something the derivation
// cannot recognise.
//
// A parameter sitting directly under /organizations IS the organization, on every
// surface. When it is spelled anything but organization_id the derivation reads it as
// nothing, the route sends no identifier, and a partner confined to one organization
// is refused on the organization it owns. That is what the by-id organization routes
// did until they were renamed: they alone spelled it ":id".
//
// The check stays narrow on purpose. It says nothing about parameters deeper in a
// path — an account's ":id" three segments down is that account's own identifier, and
// reading it as the organization is the exact confusion whole-segment comparison
// exists to prevent.
func TestRouteGuardCoverage_OrganizationSegmentAlwaysNamesTheOrganization(t *testing.T) {
	t.Parallel()

	fset := token.NewFileSet()

	pkgs, err := parser.ParseDir(fset, ".", func(info fs.FileInfo) bool {
		return !strings.HasSuffix(info.Name(), "_test.go")
	}, parser.SkipObjectResolution)
	require.NoError(t, err)

	pkg, ok := pkgs["in"]
	require.True(t, ok, "package in must parse from its own directory")

	var checked int

	for name, file := range pkg.Files {
		for constName, value := range constStrings(file) {
			// Both spellings: Fiber mounts ":param", Huma publishes "{param}".
			for _, prefix := range []string{"/organizations/:", "/organizations/{"} {
				if !strings.HasPrefix(value, prefix) {
					continue
				}

				param := strings.SplitN(strings.TrimPrefix(value, prefix), "/", 2)[0]
				param = strings.TrimSuffix(param, "}")

				checked++

				assert.Equalf(t, pathParamOrganizationID, param,
					"%s: %s = %q names the organization parameter %q — the derivation only recognises %q, so this route would send no organization at all",
					name, constName, value, param, pathParamOrganizationID)
			}
		}
	}

	assert.Positive(t, checked, "the package must declare at least one organization-scoped path")
}
