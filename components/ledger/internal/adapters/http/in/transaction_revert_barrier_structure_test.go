// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestRevertBarrierAcquisitionOrder is a permanent money-path guard. The
// bridge deliberately uses three independent operations because its Redis
// barriers have different Cluster hash tags. The PostgreSQL claim must exist
// first, then the legacy Redis barrier is acquired, then executeCreateTransaction
// acquires the origin Redis barrier, and only then may the balance Lua run.
func TestRevertBarrierAcquisitionOrder(t *testing.T) {
	t.Parallel()

	stateSource, err := os.ReadFile("transaction_state_handlers.go")
	require.NoError(t, err)
	createSource, err := os.ReadFile("transaction_create.go")
	require.NoError(t, err)

	stateCalls := callsInFunction(t, stateSource, "revertTransaction")
	require.Len(t, stateCalls["WithPrimaryRead"], 1)
	require.NotEmpty(t, stateCalls["GetParentByTransactionID"])
	require.NotEmpty(t, stateCalls["GetTransactionWithOperationsByID"])
	assert.Less(t, stateCalls["WithPrimaryRead"][0], stateCalls["GetParentByTransactionID"][0],
		"replay eligibility must be marked primary before its first query")
	assert.Less(t, stateCalls["WithPrimaryRead"][0], stateCalls["GetTransactionWithOperationsByID"][0],
		"revert eligibility must be marked primary before loading the origin")

	claimPositions := stateCalls["ClaimRevert"]
	require.Len(t, claimPositions, 2, "legacy adoption and fresh claim must remain explicit")
	require.Len(t, stateCalls["acquireLegacyRevertBarrier"], 1)
	require.Len(t, stateCalls["createRevertTransaction"], 1)
	assert.Less(t, claimPositions[1], stateCalls["acquireLegacyRevertBarrier"][0],
		"fresh PostgreSQL claim must precede the legacy Redis barrier")
	assert.Less(t, stateCalls["acquireLegacyRevertBarrier"][0], stateCalls["createRevertTransaction"][0],
		"bridge must own the legacy barrier before entering the origin-scoped create path")

	createCalls := callsInFunction(t, createSource, "executeCreateTransaction")
	require.Len(t, createCalls["CreateOrCheckTransactionIdempotency"], 1)
	require.Len(t, createCalls["ProcessBalanceOperations"], 1)
	assert.Less(t, createCalls["CreateOrCheckTransactionIdempotency"][0], createCalls["ProcessBalanceOperations"][0],
		"origin Redis barrier must be acquired before balance mutation")
}

func TestRevertAmbiguousOutcomeCannotReachAnyFenceRelease(t *testing.T) {
	t.Parallel()

	createSource, err := os.ReadFile("transaction_create.go")
	require.NoError(t, err)
	claimSource, err := os.ReadFile("transaction_revert_claim.go")
	require.NoError(t, err)

	createGuards := callsGuardedBy(t, createSource, "executeCreateTransaction", "mayReleaseRevertFences")
	require.Len(t, createGuards, 2)
	assert.Contains(t, createGuards[0], "deleteIdempotencyKey",
		"the origin Redis fence may only be deleted through the common proof gate")
	assert.Contains(t, createGuards[0], "RemoveTransactionFromRedisQueue",
		"the atomic balance outcome may only be removed through the common proof gate")
	assert.Contains(t, createGuards[1], "releaseReservations")

	claimGuards := callsGuardedBy(t, claimSource, "failRevertClaim", "mayReleaseRevertFences")
	require.Len(t, claimGuards, 1)
	assert.Contains(t, claimGuards[0], "releaseFreshRevertClaim",
		"the PostgreSQL claim and legacy Redis fence may only be released through the same proof gate")
}

func callsInFunction(t *testing.T, source []byte, function string) map[string][]token.Pos {
	t.Helper()

	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "source.go", source, 0)
	require.NoError(t, err)

	var body *ast.BlockStmt
	for _, declaration := range file.Decls {
		fn, ok := declaration.(*ast.FuncDecl)
		if ok && fn.Name.Name == function {
			body = fn.Body
			break
		}
	}
	require.NotNil(t, body, "function %s not found", function)

	calls := make(map[string][]token.Pos)
	ast.Inspect(body, func(node ast.Node) bool {
		call, ok := node.(*ast.CallExpr)
		if !ok {
			return true
		}

		switch fn := call.Fun.(type) {
		case *ast.Ident:
			calls[fn.Name] = append(calls[fn.Name], call.Pos())
		case *ast.SelectorExpr:
			calls[fn.Sel.Name] = append(calls[fn.Sel.Name], call.Pos())
		}

		return true
	})

	return calls
}

func callsGuardedBy(t *testing.T, source []byte, function, guard string) []map[string]struct{} {
	t.Helper()

	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "source.go", source, 0)
	require.NoError(t, err)

	var body *ast.BlockStmt
	for _, declaration := range file.Decls {
		fn, ok := declaration.(*ast.FuncDecl)
		if ok && fn.Name.Name == function {
			body = fn.Body
			break
		}
	}
	require.NotNil(t, body, "function %s not found", function)

	var guarded []map[string]struct{}
	ast.Inspect(body, func(node ast.Node) bool {
		ifStatement, ok := node.(*ast.IfStmt)
		if !ok || !nodeCalls(ifStatement.Cond, guard) {
			return true
		}

		bodyCalls := make(map[string]struct{})
		ast.Inspect(ifStatement.Body, func(child ast.Node) bool {
			call, ok := child.(*ast.CallExpr)
			if !ok {
				return true
			}
			switch fn := call.Fun.(type) {
			case *ast.Ident:
				bodyCalls[fn.Name] = struct{}{}
			case *ast.SelectorExpr:
				bodyCalls[fn.Sel.Name] = struct{}{}
			}
			return true
		})
		guarded = append(guarded, bodyCalls)

		return true
	})

	return guarded
}

func nodeCalls(node ast.Node, name string) bool {
	found := false
	ast.Inspect(node, func(child ast.Node) bool {
		call, ok := child.(*ast.CallExpr)
		if !ok {
			return true
		}
		switch fn := call.Fun.(type) {
		case *ast.Ident:
			found = found || fn.Name == name
		case *ast.SelectorExpr:
			found = found || fn.Sel.Name == name
		}
		return !found
	})

	return found
}
