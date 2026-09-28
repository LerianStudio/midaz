// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// spanLogClassGateBaseline lists every "site:line" where HandleSpanBusinessErrorEvent and a
// LevelError log call currently coexist in the same BlockStmt — a contradiction under T5/T7,
// since a span kept green by the business helper is paired with a log line that claims an
// infrastructure failure requiring an operator. Card 4413 works through this list lote by lote;
// each lote removes the sites it fixes. THE LIST MUST REACH EMPTY, at which point it — and this
// baseline mechanism — should be deleted along with the coexistence check itself.
//
// Entries are pinned to develop@42c7b6968 (see the card-4413 plan, §2 and appendix). Do not add
// an entry here to silence a NEW violation: only pre-existing sites from the original 92-site
// inventory belong in this list.
var spanLogClassGateBaseline = []string{}

// TestSpanLogClassGate enforces T5/T7 by construction: no non-test file in this package may
// pair HandleSpanBusinessErrorEvent with a LevelError log call in the same BlockStmt, except
// the sites still tracked in spanLogClassGateBaseline. A new violation introduced outside the
// baseline fails; a baseline entry whose code no longer reproduces the pair (fixed but not
// removed from the list) also fails, so the baseline can only ever shrink truthfully.
func TestSpanLogClassGate(t *testing.T) {
	found := findSpanLogClassSites(t)

	assert.ElementsMatch(t, spanLogClassGateBaseline, found,
		"span/log class gate: found sites must match the baseline exactly; "+
			"remove fixed entries from spanLogClassGateBaseline and add no new ones")
}

// findSpanLogClassSites walks every non-test .go file in this package directory and returns
// "file:line" for each BlockStmt that directly contains both a call to
// HandleSpanBusinessErrorEvent and a Log call at LevelError — regardless of order or distance
// within the block, per T5. line is the position of the HandleSpanBusinessErrorEvent call.
func findSpanLogClassSites(t *testing.T) []string {
	t.Helper()

	files, err := filepath.Glob("*.go")
	require.NoError(t, err)

	fset := token.NewFileSet()

	var sites []string

	for _, name := range files {
		if strings.HasSuffix(name, "_test.go") {
			continue
		}

		file, err := parser.ParseFile(fset, name, nil, 0)
		require.NoError(t, err)

		ast.Inspect(file, func(n ast.Node) bool {
			block, ok := n.(*ast.BlockStmt)
			if !ok {
				return true
			}

			var spanLine int

			hasLogError := false

			for _, stmt := range block.List {
				if call, ok := directCall(stmt, "HandleSpanBusinessErrorEvent"); ok && spanLine == 0 {
					spanLine = fset.Position(call.Pos()).Line
				}

				if isLevelErrorLogCall(stmt) {
					hasLogError = true
				}
			}

			if spanLine != 0 && hasLogError {
				sites = append(sites, filepath.Base(name)+":"+strconv.Itoa(spanLine))
			}

			return true
		})
	}

	return sites
}

// directCall returns the call expression when stmt is exactly `pkg.selName(...)`.
func directCall(stmt ast.Stmt, selName string) (*ast.CallExpr, bool) {
	exprStmt, ok := stmt.(*ast.ExprStmt)
	if !ok {
		return nil, false
	}

	call, ok := exprStmt.X.(*ast.CallExpr)
	if !ok {
		return nil, false
	}

	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok || sel.Sel.Name != selName {
		return nil, false
	}

	return call, true
}

// isLevelErrorLogCall reports whether stmt is a direct `logger.Log(ctx, libLog.LevelError, ...)`
// call: a "Log" selector call whose second argument is a `LevelError` selector.
func isLevelErrorLogCall(stmt ast.Stmt) bool {
	call, ok := directCall(stmt, "Log")
	if !ok || len(call.Args) < 2 {
		return false
	}

	level, ok := call.Args[1].(*ast.SelectorExpr)

	return ok && level.Sel.Name == "LevelError"
}
