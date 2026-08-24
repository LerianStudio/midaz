// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"go/ast"
	"go/parser"
	"go/token"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// parseFeesRoutesFile parses fees_routes.go into an AST. Tests run with the
// package directory as the working directory, so the source file is reachable by
// its base name.
func parseFeesRoutesFile(t *testing.T) *ast.File {
	t.Helper()

	fset := token.NewFileSet()

	file, err := parser.ParseFile(fset, "fees_routes.go", nil, parser.ParseComments)
	require.NoError(t, err, "fees_routes.go must parse")

	return file
}

// TestProtectedFeesAuthorizesUnderMidazSlug pins the authz application slug of every
// fee and billing route to the ledger core's own "midaz" slug (midazName), the single
// literal shared with protectedMidaz. Fee/billing is a product embedded in the ledger
// V4 binary, and BOLA "one identity, one slug" in the declaration receiver
// (plugin-identity :4001) forbids a route serving a second slug behind the same
// identity. The invariant has no runtime seam without a full auth server, so it is
// pinned at the definition site: protectedFees must call auth.Authorize with midazName.
func TestProtectedFeesAuthorizesUnderMidazSlug(t *testing.T) {
	t.Parallel()

	file := parseFeesRoutesFile(t)

	var (
		found     bool
		firstArg  string
		inspected bool
	)

	ast.Inspect(file, func(n ast.Node) bool {
		fn, ok := n.(*ast.FuncDecl)
		if !ok || fn.Name.Name != "protectedFees" {
			return true
		}

		found = true

		ast.Inspect(fn.Body, func(inner ast.Node) bool {
			call, ok := inner.(*ast.CallExpr)
			if !ok {
				return true
			}

			sel, ok := call.Fun.(*ast.SelectorExpr)
			if !ok || sel.Sel.Name != "Authorize" || len(call.Args) == 0 {
				return true
			}

			inspected = true

			if ident, ok := call.Args[0].(*ast.Ident); ok {
				firstArg = ident.Name
			}

			return false
		})

		return false
	})

	require.True(t, found, "protectedFees must exist in fees_routes.go")
	require.True(t, inspected, "protectedFees must call auth.Authorize")
	assert.Equal(t, "midazName", firstArg,
		"protectedFees must authorize under the shared midazName slug, not a fees-specific one")
}

// TestFeesApplicationNameConstRemoved asserts the fees-specific slug constant is gone,
// leaving midazName (declared in routes.go) as the single source of the "midaz" literal
// for the package. A reintroduced fees const would resurrect the retired two-slug model.
func TestFeesApplicationNameConstRemoved(t *testing.T) {
	t.Parallel()

	file := parseFeesRoutesFile(t)

	for _, decl := range file.Decls {
		gen, ok := decl.(*ast.GenDecl)
		if !ok || gen.Tok != token.CONST {
			continue
		}

		for _, spec := range gen.Specs {
			vs, ok := spec.(*ast.ValueSpec)
			if !ok {
				continue
			}

			for _, name := range vs.Names {
				assert.NotEqual(t, "feesApplicationName", name.Name,
					"the fees-specific slug constant must be removed; use midazName")
			}
		}
	}
}

// TestFeesRoutesHasNoRetiredSlug asserts the retired fee-plugin slug appears nowhere in
// fees_routes.go — neither as a string literal nor inside a comment. The forbidden token
// is assembled from fragments so the source of this guard does not itself carry the very
// literal a directory-wide grep is meant to find zero of.
func TestFeesRoutesHasNoRetiredSlug(t *testing.T) {
	t.Parallel()

	retiredSlug := "plugin" + "-" + "fees"

	file := parseFeesRoutesFile(t)

	ast.Inspect(file, func(n ast.Node) bool {
		if lit, ok := n.(*ast.BasicLit); ok && lit.Kind == token.STRING {
			assert.NotContains(t, lit.Value, retiredSlug,
				"no string literal in fees_routes.go may contain the retired fee-plugin slug")
		}

		return true
	})

	for _, group := range file.Comments {
		assert.NotContains(t, group.Text(), retiredSlug,
			"no comment in fees_routes.go may reference the retired fee-plugin slug")
	}
}
