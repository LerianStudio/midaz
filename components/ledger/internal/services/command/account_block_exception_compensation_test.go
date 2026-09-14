// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"go/ast"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The grant read can fail — the identifier is unknown, expired, already consumed,
// or the cache is down — and it sits AFTER the compensating claims each create-side
// pipeline has already taken. If that branch returns without unwinding them, the
// idempotency slot stays claimed and the caller cannot retry: the same body is
// answered from an empty slot forever, and the operator's grant looks broken.
//
// Whether the branch unwinds is a CALL-SITE fact — no runtime test of the resolver
// can see it — so it is asserted over the live source AST, the same way the
// tracer-skip and fee gates are.

// grantCompensationMetrics captures what one pipeline does on the grant-read
// failure branch.
type grantCompensationMetrics struct {
	// resolvePos is the statement index of the resolver call (-1 if absent).
	resolvePos int
	// stageBalancesPos is the statement index of an optional legacy balance staging
	// step (-1 when the pipeline is engine-only). When present, the resolve has to
	// precede it: staging seeds the backup queue, and unwinding after it needs the
	// other compensation.
	stageBalancesPos int
	// enginePos is the statement index of the engine call. The grant must be
	// resolved before accounting execution begins.
	enginePos int
	// compensates is true when the branch guarding the resolve releases the
	// idempotency claim.
	compensates bool
	// returns is true when that branch returns rather than falling through.
	returns bool
}

// analyzeGrantCompensation walks the named pipeline and reads off the grant
// resolve statement plus the `if err != nil` that immediately follows it.
func analyzeGrantCompensation(t *testing.T, src, funcName string) grantCompensationMetrics {
	t.Helper()

	fn := findFuncDecl(t, src, funcName)

	m := grantCompensationMetrics{resolvePos: -1, stageBalancesPos: -1, enginePos: -1}

	for i, stmt := range fn.Body.List {
		if m.resolvePos == -1 && stmtCallsMethod(stmt, grantResolverName) {
			m.resolvePos = i
		}

		if m.stageBalancesPos == -1 && stmtCallsMethod(stmt, "stageBalances") {
			m.stageBalancesPos = i
		}
		if m.enginePos == -1 && stmtCallsMethod(stmt, "createTransactionWithEngine") {
			m.enginePos = i
		}

		if m.resolvePos != -1 && i == m.resolvePos+1 {
			if ifStmt, ok := stmt.(*ast.IfStmt); ok {
				m.compensates = blockCallsMethod(ifStmt.Body, "rollbackCreateClaim")
				m.returns = blockEndsInReturn(ifStmt.Body)
			}
		}
	}

	return m
}

// TestAccountBlockExceptionGrant_CreateSideFailureCompensates asserts both
// create-side pipelines release the idempotency claim and return when the grant
// read fails, and that the read happens before accounting execution. A pipeline
// that still has a nonmonetary legacy path must also resolve before balance staging.
func TestAccountBlockExceptionGrant_CreateSideFailureCompensates(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct {
		name     string
		path     string
		funcName string
	}{
		{name: "create", path: "create_transaction_v2.go", funcName: "CreateTransactionV2"},
		{name: "revert", path: "revert_transaction.go", funcName: "createRevertV2"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			src := readTransportSource(t, tt.path, "func (uc *UseCase) "+tt.funcName)

			m := analyzeGrantCompensation(t, src, tt.funcName)

			require.NotEqualf(t, -1, m.resolvePos, "%s must call %s", tt.funcName, grantResolverName)
			require.NotEqualf(t, -1, m.enginePos, "%s must call createTransactionWithEngine", tt.funcName)

			if m.stageBalancesPos != -1 {
				assert.Lessf(t, m.resolvePos, m.stageBalancesPos,
					"%s must read the grant BEFORE staging balances, so a failed read has only the "+
						"idempotency claim to unwind and no backup seed", tt.funcName)
			}
			assert.Lessf(t, m.resolvePos, m.enginePos,
				"%s must resolve the grant before engine execution", tt.funcName)

			assert.Truef(t, m.compensates,
				"%s must release the idempotency claim when the grant read fails, or the caller "+
					"can never retry the same body", tt.funcName)
			assert.Truef(t, m.returns,
				"%s must RETURN on a failed grant read — falling through would post a transaction "+
					"the grant never authorized", tt.funcName)
		})
	}
}

func TestAccountBlockExceptionGrant_PendingFailureUnlocksBeforeEngine(t *testing.T) {
	t.Parallel()

	src := readTransportSource(t, "commit_transaction.go", "func (uc *UseCase) transitionPendingV2")
	fn := findFuncDecl(t, src, "transitionPendingV2")
	resolvePos, enginePos := -1, -1
	unlocks, returns := false, false
	for i, stmt := range fn.Body.List {
		if resolvePos == -1 && stmtCallsMethod(stmt, grantResolverName) {
			resolvePos = i
			continue
		}
		if resolvePos != -1 && i == resolvePos+1 {
			if ifStmt, ok := stmt.(*ast.IfStmt); ok {
				unlocks = blockCallsFunction(ifStmt.Body, "unlock")
				returns = blockEndsInReturn(ifStmt.Body)
			}
		}
		if enginePos == -1 && stmtCallsMethod(stmt, "transitionPendingWithEngine") {
			enginePos = i
		}
	}

	require.NotEqual(t, -1, resolvePos)
	require.NotEqual(t, -1, enginePos)
	assert.Less(t, resolvePos, enginePos)
	assert.True(t, unlocks)
	assert.True(t, returns)
}

func blockCallsFunction(block *ast.BlockStmt, name string) bool {
	found := false
	ast.Inspect(block, func(node ast.Node) bool {
		call, ok := node.(*ast.CallExpr)
		if !ok {
			return true
		}
		function, ok := call.Fun.(*ast.Ident)
		if ok && function.Name == name {
			found = true
		}
		return true
	})

	return found
}

// TestAccountBlockExceptionGrant_CreateSideCompensationGateBites proves the
// analyzer above actually bites. A gate that cannot fail is not a guard.
func TestAccountBlockExceptionGrant_CreateSideCompensationGateBites(t *testing.T) {
	t.Parallel()

	leaky := `package command
func (uc *UseCase) CreateTransactionV2() error {
	run.accountBlockExceptionGrant, err = uc.resolveAccountBlockExceptionGrant()
	if err != nil {
		// BUG: neither releases the claim nor returns
		_ = err
		}
		if uc.Engine != nil {
			return uc.createTransactionWithEngine()
		}
		ctx, err = uc.stageBalances()
	return nil
}
`

	m := analyzeGrantCompensation(t, leaky, "CreateTransactionV2")

	require.NotEqual(t, -1, m.resolvePos, "the fixture's resolve call must be found")
	assert.False(t, m.compensates, "the gate failed to bite: a missing claim release went undetected")
	assert.False(t, m.returns, "the gate failed to bite: a fall-through branch went undetected")
}
