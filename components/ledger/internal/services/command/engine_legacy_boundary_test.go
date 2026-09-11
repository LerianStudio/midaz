// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEnginePathDoesNotCallLegacyLiveBalanceMutationOrProjectionHelpers(t *testing.T) {
	optInFiles, err := filepath.Glob("engine_*.go")
	require.NoError(t, err)

	optInFiles = append(
		optInFiles,
		"create_transaction_engine.go",
		"transition_pending_engine.go",
	)
	legacyCalls := map[string]struct{}{
		"OperateBalances":                         {},
		"ValidateFromToOperation":                 {},
		"DetectOverdraftSplit":                    {},
		"CalculateOverdraftSplit":                 {},
		"CalculateRefundSplit":                    {},
		"enrichOverdraftOperations":               {},
		"resolveRouteCodesFromCache":              {},
		"effectiveOperationAmount":                {},
		"BuildOperations":                         {},
		"ProcessBalanceOperations":                {},
		"CreateBalanceTransactionOperations":      {},
		"CreateBalanceTransactionOperationsAsync": {},
	}

	for _, name := range optInFiles {
		if strings.HasSuffix(name, "_test.go") {
			continue
		}

		calls := sourceCalls(t, name)
		for call := range legacyCalls {
			assert.NotContains(t, calls, call, "%s must not call the legacy %s helper", name, call)
		}
	}
}

func TestLegacyLiveBalanceMutationAndProjectionHelpersRemainUsedByFallback(t *testing.T) {
	calls := make(map[string]struct{})
	for _, name := range []string{
		"build_transaction_operations.go",
		"create_transaction_v1.go",
		"create_transaction_v2.go",
		"revert_transaction.go",
		"transition_pending_steps.go",
		"transaction_overdraft_enrichment.go",
		"update_balance.go",
	} {
		for call := range sourceCalls(t, name) {
			calls[call] = struct{}{}
		}
	}

	for _, call := range []string{
		"OperateBalances",
		"ValidateFromToOperation",
		"DetectOverdraftSplit",
		"enrichOverdraftOperations",
		"resolveRouteCodesFromCache",
		"effectiveOperationAmount",
		"BuildOperations",
		"ProcessBalanceOperations",
	} {
		assert.Contains(t, calls, call, "fallback must retain its %s helper reference", call)
	}
}

func sourceCalls(t *testing.T, name string) map[string]struct{} {
	t.Helper()

	file, err := parser.ParseFile(token.NewFileSet(), name, nil, 0)
	require.NoError(t, err)

	calls := make(map[string]struct{})
	ast.Inspect(file, func(node ast.Node) bool {
		call, ok := node.(*ast.CallExpr)
		if !ok {
			return true
		}

		switch function := call.Fun.(type) {
		case *ast.Ident:
			calls[function.Name] = struct{}{}
		case *ast.SelectorExpr:
			calls[function.Sel.Name] = struct{}{}
		}

		return true
	})

	return calls
}
