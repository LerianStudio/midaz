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
	// stageBalancesPos is the statement index of the balance staging step (-1),
	// which is what the resolve has to precede: staging seeds the backup queue,
	// and unwinding after it needs the other compensation.
	stageBalancesPos int
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

	m := grantCompensationMetrics{resolvePos: -1, stageBalancesPos: -1}

	for i, stmt := range fn.Body.List {
		if m.resolvePos == -1 && stmtCallsMethod(stmt, grantResolverName) {
			m.resolvePos = i
		}

		if m.stageBalancesPos == -1 && stmtCallsMethod(stmt, "stageBalances") {
			m.stageBalancesPos = i
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
// read fails, and that the read happens before the balance staging that would
// otherwise leave a backup seed behind too.
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
			require.NotEqualf(t, -1, m.stageBalancesPos, "%s must call stageBalances", tt.funcName)

			assert.Lessf(t, m.resolvePos, m.stageBalancesPos,
				"%s must read the grant BEFORE staging balances, so a failed read has only the "+
					"idempotency claim to unwind and no backup seed", tt.funcName)

			assert.Truef(t, m.compensates,
				"%s must release the idempotency claim when the grant read fails, or the caller "+
					"can never retry the same body", tt.funcName)
			assert.Truef(t, m.returns,
				"%s must RETURN on a failed grant read — falling through would post a transaction "+
					"the grant never authorized", tt.funcName)
		})
	}
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
	ctx, err = uc.stageBalances()
	return nil
}
`

	m := analyzeGrantCompensation(t, leaky, "CreateTransactionV2")

	require.NotEqual(t, -1, m.resolvePos, "the fixture's resolve call must be found")
	assert.False(t, m.compensates, "the gate failed to bite: a missing claim release went undetected")
	assert.False(t, m.returns, "the gate failed to bite: a fall-through branch went undetected")
}
