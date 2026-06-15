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

// These are permanent structural guards for the third-rail fee seam (P4-T27).
// Runtime integration tests cannot catch a future code reorder or a forked
// validate binding, so the seam's invariants are asserted directly over the
// source AST of executeCreateTransaction:
//
//	Gate 1 — single validate reassignment: exactly one `validate :=` binding
//	         and exactly one `validate =` reassignment; no second `validate :=`
//	         fork and no pre-fee snapshot of validate surviving the seam. This
//	         proves all downstream consumers read the post-fee validate by
//	         construction.
//	Gate 2 — seam precedes redis seed: both the applyFees call and the second
//	         ValidateSendSourceAndDistribute call appear positionally before the
//	         SendTransactionToRedisQueue seed.
//
// The two TestSeamGate*_Bites sub-tests feed deliberately-broken fixtures
// through the same analyzers to prove each gate actually fails — a gate that
// cannot bite is not a guard.

const seamFuncName = "executeCreateTransaction"

// seamMetrics captures the structural facts the gates assert over a single
// function declaration.
type seamMetrics struct {
	validateDefineCount int  // `validate :=` occurrences
	validateAssignCount int  // `validate =` (reassignment, non-define)
	applyFeesPos        int  // statement-list index of the applyFees call (-1 if absent)
	secondValidatePos   int  // statement-list index of the second validate ValidateSendSourceAndDistribute (-1)
	redisSeedPos        int  // statement-list index of SendTransactionToRedisQueue (-1)
	writeTransactionPos int  // statement-list index of WriteTransaction (-1)
	getSettingsCount    int  // `GetParsedLedgerSettings` call occurrences
	getSettingsPos      int  // statement-list index of GetParsedLedgerSettings (-1 if absent)
	preFeeSnapshot      bool // a non-validate var bound from validate before the seam (snapshot copy)
}

// analyzeSeamFunc walks the named function in src and extracts the structural
// metrics the gates rely on. The positions are top-level statement indices in
// the function body, which is sufficient for the strictly-sequential seam.
func analyzeSeamFunc(t *testing.T, src, funcName string) seamMetrics {
	t.Helper()

	fset := token.NewFileSet()

	file, err := parser.ParseFile(fset, "src.go", src, 0)
	if err != nil {
		t.Fatalf("parse source: %v", err)
	}

	var fn *ast.FuncDecl

	for _, decl := range file.Decls {
		if d, ok := decl.(*ast.FuncDecl); ok && d.Name.Name == funcName {
			fn = d
			break
		}
	}

	if fn == nil || fn.Body == nil {
		t.Fatalf("function %q not found or has no body", funcName)
	}

	m := seamMetrics{applyFeesPos: -1, secondValidatePos: -1, redisSeedPos: -1, writeTransactionPos: -1, getSettingsPos: -1}

	for i, stmt := range fn.Body.List {
		// validate :=  /  validate =  detection (top-level only — a fork or a
		// snapshot reuse would be a top-level statement in this function).
		if as, ok := stmt.(*ast.AssignStmt); ok {
			if assignsIdent(as, "validate") {
				if as.Tok == token.DEFINE {
					m.validateDefineCount++
				} else if as.Tok == token.ASSIGN {
					m.validateAssignCount++
				}
			}

			if as.Tok == token.DEFINE && bindsFromValidateSnapshot(as) {
				m.preFeeSnapshot = true
			}
		}

		if m.applyFeesPos == -1 && stmtCallsMethod(stmt, "applyFees") {
			m.applyFeesPos = i
		}

		if stmtCallsFunc(stmt, "ValidateSendSourceAndDistribute") {
			// The second validate is the one whose result is reassigned (= not :=).
			if as, ok := stmt.(*ast.AssignStmt); ok && as.Tok == token.ASSIGN && assignsIdent(as, "validate") {
				m.secondValidatePos = i
			}
		}

		if stmtCallsMethod(stmt, "GetParsedLedgerSettings") {
			m.getSettingsCount++

			if m.getSettingsPos == -1 {
				m.getSettingsPos = i
			}
		}

		if m.redisSeedPos == -1 && stmtCallsMethod(stmt, "SendTransactionToRedisQueue") {
			m.redisSeedPos = i
		}

		if m.writeTransactionPos == -1 && stmtCallsMethod(stmt, "WriteTransaction") {
			m.writeTransactionPos = i
		}
	}

	return m
}

// assignsIdent reports whether the assignment writes to a bare identifier with
// the given name on its LHS.
func assignsIdent(as *ast.AssignStmt, name string) bool {
	for _, lhs := range as.Lhs {
		if id, ok := lhs.(*ast.Ident); ok && id.Name == name {
			return true
		}
	}

	return false
}

// bindsFromValidateSnapshot reports whether a := assignment copies the bare
// `validate` identifier into another variable (a pre-fee snapshot), e.g.
// `preFeeValidate := validate`.
func bindsFromValidateSnapshot(as *ast.AssignStmt) bool {
	for _, rhs := range as.Rhs {
		if id, ok := rhs.(*ast.Ident); ok && id.Name == "validate" {
			// Exclude the no-op `validate := validate` (not produced by gofmt,
			// but guard anyway): a snapshot binds a DIFFERENT lhs name.
			if !assignsIdent(as, "validate") {
				return true
			}
		}
	}

	return false
}

// stmtCallsFunc reports whether the statement contains a call to a plain
// function with the given name (e.g. ValidateSendSourceAndDistribute, which may
// be package-qualified as mtransaction.ValidateSendSourceAndDistribute).
func stmtCallsFunc(stmt ast.Stmt, name string) bool {
	found := false

	ast.Inspect(stmt, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}

		switch f := call.Fun.(type) {
		case *ast.Ident:
			if f.Name == name {
				found = true
			}
		case *ast.SelectorExpr:
			if f.Sel.Name == name {
				found = true
			}
		}

		return true
	})

	return found
}

// stmtCallsMethod reports whether the statement contains a selector call whose
// method name matches (e.g. handler.applyFees(...), handler.Command.X(...)).
func stmtCallsMethod(stmt ast.Stmt, method string) bool {
	found := false

	ast.Inspect(stmt, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}

		if sel, ok := call.Fun.(*ast.SelectorExpr); ok && sel.Sel.Name == method {
			found = true
		}

		return true
	})

	return found
}

func TestFeeSeamStructure_SingleValidateReassignment(t *testing.T) {
	src := readSeamSource(t)

	m := analyzeSeamFunc(t, src, seamFuncName)

	if m.validateDefineCount != 1 {
		t.Errorf("Gate 1: expected exactly one `validate :=` binding, got %d", m.validateDefineCount)
	}

	if m.validateAssignCount != 1 {
		t.Errorf("Gate 1: expected exactly one `validate =` reassignment (the second validate), got %d", m.validateAssignCount)
	}

	if m.preFeeSnapshot {
		t.Error("Gate 1: a pre-fee snapshot of validate was detected; no copy may survive the applyFees seam")
	}
}

func TestFeeSeamStructure_SeamPrecedesRedisSeed(t *testing.T) {
	src := readSeamSource(t)

	m := analyzeSeamFunc(t, src, seamFuncName)

	if m.applyFeesPos == -1 {
		t.Fatal("Gate 2: applyFees call not found in executeCreateTransaction")
	}

	if m.secondValidatePos == -1 {
		t.Fatal("Gate 2: second (reassigning) ValidateSendSourceAndDistribute call not found")
	}

	if m.redisSeedPos == -1 {
		t.Fatal("Gate 2: SendTransactionToRedisQueue seed not found")
	}

	if m.applyFeesPos >= m.redisSeedPos {
		t.Errorf("Gate 2: applyFees (pos %d) must precede SendTransactionToRedisQueue (pos %d)", m.applyFeesPos, m.redisSeedPos)
	}

	if m.secondValidatePos >= m.redisSeedPos {
		t.Errorf("Gate 2: second validate (pos %d) must precede SendTransactionToRedisQueue (pos %d)", m.secondValidatePos, m.redisSeedPos)
	}

	if m.applyFeesPos >= m.secondValidatePos {
		t.Errorf("Gate 2: applyFees (pos %d) must precede the second validate (pos %d)", m.applyFeesPos, m.secondValidatePos)
	}
}

// TestFeeSeamStructure_SingleSettingsReadBeforeSeam asserts the ledger settings
// are read exactly once and that read precedes the fee seam — so the per-call
// skip resolution rides the single cached read and an honored fee skip can
// short-circuit applyFees before its package lookup, with no extra I/O.
func TestFeeSeamStructure_SingleSettingsReadBeforeSeam(t *testing.T) {
	src := readSeamSource(t)

	m := analyzeSeamFunc(t, src, seamFuncName)

	if m.getSettingsCount != 1 {
		t.Errorf("expected exactly one GetParsedLedgerSettings call, got %d", m.getSettingsCount)
	}

	if m.getSettingsPos == -1 {
		t.Fatal("GetParsedLedgerSettings call not found in executeCreateTransaction")
	}

	if m.applyFeesPos == -1 {
		t.Fatal("applyFees call not found in executeCreateTransaction")
	}

	if m.getSettingsPos >= m.applyFeesPos {
		t.Errorf("settings read (pos %d) must precede applyFees (pos %d) so the fee skip resolves off the cached read",
			m.getSettingsPos, m.applyFeesPos)
	}
}

// TestFeeSeamStructure_SettingsGateBites proves the single-read gate fails when
// a second GetParsedLedgerSettings call sneaks back in (the duplicated read the
// hoist removed).
func TestFeeSeamStructure_SettingsGateBites(t *testing.T) {
	doubleRead := `package in
func executeCreateTransaction() {
	validate, err := ValidateSendSourceAndDistribute()
	ledgerSettings, err := handler.Query.GetParsedLedgerSettings()
	_ = handler.applyFees()
	validate, err = ValidateSendSourceAndDistribute()
	ledgerSettings, err = handler.Query.GetParsedLedgerSettings() // BUG: second read
	_ = handler.Command.SendTransactionToRedisQueue(validate)
	_ = handler.Command.WriteTransaction(validate)
}`

	m := analyzeSeamFunc(t, doubleRead, seamFuncName)
	if m.getSettingsCount == 1 {
		t.Error("settings gate failed to bite: a second GetParsedLedgerSettings call was not detected (expected != 1)")
	}

	if m.getSettingsCount != 2 {
		t.Errorf("settings gate fixture sanity: expected 2 GetParsedLedgerSettings calls in the fixture, got %d", m.getSettingsCount)
	}
}

// TestFeeSeamStructure_Gate1Bites proves Gate 1 fails on a forked binding and
// on a surviving pre-fee snapshot.
func TestFeeSeamStructure_Gate1Bites(t *testing.T) {
	forked := `package in
func executeCreateTransaction() {
	validate, err := ValidateSendSourceAndDistribute()
	_ = handler.applyFees()
	validate, err := ValidateSendSourceAndDistribute() // BUG: := fork, not =
	_ = handler.Command.SendTransactionToRedisQueue(validate)
	_ = handler.Command.WriteTransaction(validate)
}`

	m := analyzeSeamFunc(t, forked, seamFuncName)
	if m.validateAssignCount == 1 && m.validateDefineCount == 1 {
		t.Error("Gate 1 failed to bite: a forked `validate :=` was not detected (expected != 1 define or != 1 assign)")
	}

	if m.validateDefineCount != 2 {
		t.Errorf("Gate 1 fixture sanity: expected 2 `validate :=` in the forked fixture, got %d", m.validateDefineCount)
	}

	snapshot := `package in
func executeCreateTransaction() {
	validate, err := ValidateSendSourceAndDistribute()
	preFeeValidate := validate // BUG: pre-fee snapshot survives the seam
	_ = handler.applyFees()
	validate, err = ValidateSendSourceAndDistribute()
	_ = handler.Command.SendTransactionToRedisQueue(validate)
	_ = handler.Command.WriteTransaction(preFeeValidate)
}`

	ms := analyzeSeamFunc(t, snapshot, seamFuncName)
	if !ms.preFeeSnapshot {
		t.Error("Gate 1 failed to bite: a pre-fee snapshot `preFeeValidate := validate` was not detected")
	}
}

// TestFeeSeamStructure_Gate2Bites proves Gate 2 fails when the seam is moved
// after the redis seed.
func TestFeeSeamStructure_Gate2Bites(t *testing.T) {
	reordered := `package in
func executeCreateTransaction() {
	validate, err := ValidateSendSourceAndDistribute()
	_ = handler.Command.SendTransactionToRedisQueue(validate) // BUG: seed precedes the seam
	_ = handler.applyFees()
	validate, err = ValidateSendSourceAndDistribute()
	_ = handler.Command.WriteTransaction(validate)
}`

	m := analyzeSeamFunc(t, reordered, seamFuncName)

	if m.applyFeesPos == -1 || m.redisSeedPos == -1 || m.secondValidatePos == -1 {
		t.Fatalf("Gate 2 fixture sanity: missing positions applyFees=%d redis=%d secondValidate=%d",
			m.applyFeesPos, m.redisSeedPos, m.secondValidatePos)
	}

	if m.applyFeesPos < m.redisSeedPos && m.secondValidatePos < m.redisSeedPos {
		t.Error("Gate 2 failed to bite: the reordered fixture (seed before seam) was not detected as out-of-order")
	}
}

// readSeamSource reads transaction_create.go from disk so the gates run against
// the live source, not a snapshot, and fail the moment the seam is edited.
func readSeamSource(t *testing.T) string {
	t.Helper()

	const path = "transaction_create.go"

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}

	src := string(data)
	if !strings.Contains(src, "func (handler *TransactionHandler) "+seamFuncName) {
		t.Fatalf("%s does not contain %s — the gate is pointed at the wrong file", path, seamFuncName)
	}

	return src
}
