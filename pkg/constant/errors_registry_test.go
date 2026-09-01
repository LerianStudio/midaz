// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package constant

import (
	"go/ast"
	"go/parser"
	"go/token"
	"regexp"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestErrorRegistryUniqueness is the E14 registry lock. It parses the declared
// error sentinels in errors.go via go/parser and asserts the invariants that
// keep the unified numeric registry sound after the FEE-/TRC-/TPL-/REP- fork
// consolidation:
//
//  1. Every errors.New code string is unique across the whole package.
//  2. No code string still carries a retired family prefix.
//  3. Numeric codes are zero-padded 4-digit (CRM-xxxx codes are a separate,
//     intentionally-namespaced string family and are exempted from the numeric
//     shape rule but still covered by uniqueness).
//
// A duplicated code (e.g. allocating "0179" twice) fails invariant 1; a stray
// "FEE-0005" literal fails invariant 2; a non-padded "179" fails invariant 3.
func TestErrorRegistryUniqueness(t *testing.T) {
	codes := parseErrorCodes(t, "errors.go")
	require.NotEmpty(t, codes, "expected to parse error sentinels from errors.go")

	prefixRe := regexp.MustCompile(`^(FEE|TRC|TPL|REP)-`)
	numericRe := regexp.MustCompile(`^\d{4}$`)
	crmRe := regexp.MustCompile(`^CRM-\d{4}$`)

	seen := make(map[string]string, len(codes))

	for _, c := range codes {
		// (a) uniqueness
		if prev, dup := seen[c.code]; dup {
			t.Errorf("duplicate error code %q declared by both %s and %s", c.code, prev, c.ident)
		}

		seen[c.code] = c.ident

		// (b) no retired family prefix
		assert.False(t, prefixRe.MatchString(c.code),
			"sentinel %s still carries a retired family prefix: %q", c.ident, c.code)

		// (c) numeric codes are zero-padded 4-digit (CRM- string family exempted)
		if crmRe.MatchString(c.code) {
			continue
		}

		assert.True(t, numericRe.MatchString(c.code),
			"sentinel %s code %q must be a zero-padded 4-digit numeric code", c.ident, c.code)

		// reject leading-zero-stripped or overflowing widths the regex would miss
		if numericRe.MatchString(c.code) {
			n, err := strconv.Atoi(c.code)
			require.NoError(t, err)
			assert.Equal(t, c.code, padCode(n),
				"sentinel %s code %q is not canonically zero-padded", c.ident, c.code)
		}
	}
}

type sentinel struct {
	ident string
	code  string
}

// parseErrorCodes walks the AST of the given source file and returns every
// `ErrX = errors.New("...")` declaration as an (identifier, code) pair.
func parseErrorCodes(t *testing.T, filename string) []sentinel {
	t.Helper()

	fset := token.NewFileSet()

	file, err := parser.ParseFile(fset, filename, nil, 0)
	require.NoError(t, err, "parsing %s", filename)

	var out []sentinel

	for _, decl := range file.Decls {
		gen, ok := decl.(*ast.GenDecl)
		if !ok || gen.Tok != token.VAR {
			continue
		}

		for _, spec := range gen.Specs {
			vs, ok := spec.(*ast.ValueSpec)
			if !ok {
				continue
			}

			for i, name := range vs.Names {
				if i >= len(vs.Values) {
					continue
				}

				code, ok := errorsNewLiteral(vs.Values[i])
				if !ok {
					continue
				}

				out = append(out, sentinel{ident: name.Name, code: code})
			}
		}
	}

	return out
}

// errorsNewLiteral returns the string literal passed to errors.New(...) for the
// given expression, or ("", false) if the expression is not such a call.
func errorsNewLiteral(expr ast.Expr) (string, bool) {
	call, ok := expr.(*ast.CallExpr)
	if !ok || len(call.Args) != 1 {
		return "", false
	}

	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok || sel.Sel.Name != "New" {
		return "", false
	}

	pkgIdent, ok := sel.X.(*ast.Ident)
	if !ok || pkgIdent.Name != "errors" {
		return "", false
	}

	lit, ok := call.Args[0].(*ast.BasicLit)
	if !ok || lit.Kind != token.STRING {
		return "", false
	}

	val, err := strconv.Unquote(lit.Value)
	if err != nil {
		return "", false
	}

	return val, true
}

func padCode(n int) string {
	s := strconv.Itoa(n)
	for len(s) < 4 {
		s = "0" + s
	}

	return s
}
