// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTransactionBackupDeletionRequiresAtomicProof(t *testing.T) {
	t.Parallel()

	repositoryRoot := filepath.Clean(filepath.Join("..", "..", "..", "..", "..", ".."))
	ledgerRoot := filepath.Join(repositoryRoot, "components", "ledger")
	err := filepath.WalkDir(ledgerRoot, func(path string, entry os.DirEntry, walkErr error) error {
		require.NoError(t, walkErr)
		if entry.IsDir() || filepath.Ext(path) != ".go" || strings.HasSuffix(path, "_test.go") ||
			strings.HasSuffix(path, "_mock.go") {
			return nil
		}

		source, readErr := os.ReadFile(path)
		require.NoError(t, readErr)
		assert.NotContains(t, string(source), "RemoveMessageFromQueue(",
			"transaction backups must never have a key-only delete path: %s", path)

		return nil
	})
	require.NoError(t, err)

	consumerPath := filepath.Join(ledgerRoot, "internal", "adapters", "redis", "transaction", "consumer.redis.go")
	consumerSource, err := os.ReadFile(consumerPath)
	require.NoError(t, err)
	clearAttemptCalls := callsInFunction(t, consumerSource, "ClearBackupAttempt")
	require.Len(t, clearAttemptCalls["HDel"], 1,
		"the only direct Go HDEL is the non-economic retry counter cleanup")
	assert.Equal(t, 1, strings.Count(string(consumerSource), ".HDel("),
		"backup envelopes must be deleted only by compared Lua scripts")

	scriptDirectory := filepath.Join(ledgerRoot, "internal", "adapters", "redis", "transaction", "scripts")
	allowed := map[string][]string{
		"finalize_legacy_transaction_persistence.lua": {
			"parent_transaction_id", "operations", "balancesAfter",
			"economic_effect_digest", "TRANSACTION_PERSISTENCE_TOMBSTONE_MISSING",
			`redis.call("SET", KEYS[3]`, "HDEL",
		},
		"finalize_transaction_persistence.lua": {
			"attempt_owner", "expected_outcome", "balancesAfter",
			"economic_effect_digest", "TRANSACTION_PERSISTENCE_TOMBSTONE_MISSING",
			`redis.call("SET", KEYS[4]`, "HDEL",
		},
		"remove_transaction_backup_if_status.lua": {"attempt_owner", "expected_outcome", "balancesAfter", "HDEL"},
		"remove_transaction_backup_if_value.lua":  {"raw ~= ARGV[1]", "HDEL"},
		"release_pre_movement_revert.lua":         {"KEYS[6]", "ARGV[1]", "attempt_owner", "HDEL"},
	}
	entries, err := os.ReadDir(scriptDirectory)
	require.NoError(t, err)
	seen := make(map[string]bool, len(allowed))
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".lua" {
			continue
		}
		source, readErr := os.ReadFile(filepath.Join(scriptDirectory, entry.Name()))
		require.NoError(t, readErr)
		if !strings.Contains(string(source), "HDEL") {
			continue
		}

		proofs, ok := allowed[entry.Name()]
		require.True(t, ok, "new transaction-backup HDEL requires an explicit atomic proof classification: %s", entry.Name())
		seen[entry.Name()] = true
		for _, proof := range proofs {
			assert.Contains(t, string(source), proof, "%s must retain proof %q", entry.Name(), proof)
		}
	}
	for script := range allowed {
		assert.True(t, seen[script], "classified backup cleanup script %s must remain present", script)
	}
}

func TestTerminalEconomicProofUsesOpaqueDigestInsteadOfLuaNumbers(t *testing.T) {
	t.Parallel()

	scriptDirectory := filepath.Clean(filepath.Join("..", "..", "redis", "transaction", "scripts"))
	for _, name := range []string{
		"bind_transaction_economic_digest.lua",
		"bind_legacy_transaction_economic_digest.lua",
		"finalize_transaction_persistence.lua",
		"finalize_legacy_transaction_persistence.lua",
	} {
		source, err := os.ReadFile(filepath.Join(scriptDirectory, name))
		require.NoError(t, err)
		assert.Contains(t, string(source), "economic_effect_digest")
		assert.NotContains(t, string(source), "tonumber",
			"terminal money proof must never compare decimals through Lua doubles: %s", name)
	}
}

func TestEveryPersistenceConsumerUsesOneTerminalHandoff(t *testing.T) {
	t.Parallel()

	commandDirectory := filepath.Clean(filepath.Join("..", "..", "..", "services", "command"))
	individual, err := os.ReadFile(filepath.Join(commandDirectory, "create_balance_transaction_operations_async.go"))
	require.NoError(t, err)
	bulk, err := os.ReadFile(filepath.Join(commandDirectory, "create_bulk_transaction_operations_async.go"))
	require.NoError(t, err)

	individualCalls := callsInFunction(t, individual, "CreateBalanceTransactionOperationsAsync")
	require.Len(t, individualCalls["FinalizeDurableTransactionPersistence"], 2,
		"normal persistence and terminal lost-ack replay must both use the same complete handoff")
	assert.Empty(t, individualCalls["FinalizeTransactionPersistence"],
		"the individual consumer cannot implement a partial terminal handoff")
	assert.Empty(t, individualCalls["CompleteRevertClaim"],
		"the individual consumer cannot complete a claim outside the shared handoff")

	bulkCalls := callsInFunction(t, bulk, "processMetadataAndEvents")
	require.Len(t, bulkCalls["FinalizeDurableTransactionPersistence"], 1)
	assert.Empty(t, bulkCalls["FinalizeTransactionPersistence"],
		"the bulk consumer cannot implement a partial terminal handoff")
	assert.Empty(t, bulkCalls["CompleteRevertClaim"],
		"the bulk consumer cannot complete a claim outside the shared handoff")
}

func TestOutcomeBackedPreflightCannotBeGatedByRedisGeneration(t *testing.T) {
	t.Parallel()

	commandDirectory := filepath.Clean(filepath.Join("..", "..", "..", "services", "command"))
	individual, err := os.ReadFile(filepath.Join(commandDirectory, "create_balance_transaction_operations_async.go"))
	require.NoError(t, err)
	bulk, err := os.ReadFile(filepath.Join(commandDirectory, "create_bulk_transaction_operations_async.go"))
	require.NoError(t, err)
	finalizer, err := os.ReadFile(filepath.Join(commandDirectory, "finalize_transaction_persistence.go"))
	require.NoError(t, err)

	require.Len(t, callsInFunction(t, individual, "CreateBalanceTransactionOperationsAsync")["preflightOutcomeBackedTransaction"], 1)
	require.Len(t, callsInFunction(t, bulk, "preflightDurableBulkPayloads")["preflightOutcomeBackedTransaction"], 1)
	require.Len(t, callsInFunction(t, finalizer, "FinalizeDurableTransactionPersistence")["preflightOutcomeBackedTransaction"], 1)
	assert.NotContains(t, string(individual), `if t.RedisGeneration != ""`,
		"outcome-backed individual persistence cannot skip economic proof when generation is absent")
	assert.NotContains(t, string(bulk), `if payload.RedisGeneration == ""`,
		"outcome-backed bulk persistence cannot classify economic proof from generation alone")
}

func TestHTTPAdoptionUsesTheSameTerminalHandoffOrder(t *testing.T) {
	t.Parallel()

	claimSource, err := os.ReadFile("transaction_revert_claim.go")
	require.NoError(t, err)

	calls := callsInFunction(t, claimSource, "finalizeDurableRevert")
	require.Len(t, calls["MarkRevertClaim"], 1)
	require.Len(t, calls["completeOriginRevertBarrier"], 1)
	require.Len(t, calls["finalizeDurableRevertPersistence"], 1)
	assert.Less(t, calls["MarkRevertClaim"][0], calls["completeOriginRevertBarrier"][0],
		"the durable claim must become terminal before any Redis replay is published")
	assert.Less(t, calls["completeOriginRevertBarrier"][0], calls["finalizeDurableRevertPersistence"][0],
		"outcome and backup cleanup require terminal claim plus published replay")
}

// TestRevertBarrierAcquisitionOrder is a permanent money-path guard. The
// bridge deliberately uses independent operations because its legacy and
// transaction Redis barriers have different Cluster hash tags. The Redis
// economic attempt must exist before the durable claim becomes visible. The
// claim then precedes the legacy barrier; executeCreateTransaction acquires the
// origin barrier, and only then may balance Lua record the immutable outcome.
func TestRevertBarrierAcquisitionOrder(t *testing.T) {
	t.Parallel()

	stateSource, err := os.ReadFile("transaction_state_handlers.go")
	require.NoError(t, err)
	createSource, err := os.ReadFile("transaction_create.go")
	require.NoError(t, err)
	claimSource, err := os.ReadFile("transaction_revert_claim.go")
	require.NoError(t, err)

	stateCalls := callsInFunction(t, stateSource, "revertTransaction")
	require.Len(t, stateCalls["WithPrimaryRead"], 1)
	require.NotEmpty(t, stateCalls["GetParentByTransactionID"])
	require.NotEmpty(t, stateCalls["GetTransactionWithOperationsByID"])
	require.NotEmpty(t, stateCalls["GetOperationRouteByID"])
	assert.Less(t, stateCalls["WithPrimaryRead"][0], stateCalls["GetParentByTransactionID"][0],
		"replay eligibility must be marked primary before its first query")
	assert.Less(t, stateCalls["WithPrimaryRead"][0], stateCalls["GetTransactionWithOperationsByID"][0],
		"revert eligibility must be marked primary before loading the origin")
	assert.Less(t, stateCalls["WithPrimaryRead"][0], stateCalls["GetOperationRouteByID"][0],
		"route eligibility must be marked primary before validating bidirectionality")

	claimPositions := stateCalls["ClaimRevert"]
	require.Len(t, claimPositions, 2, "legacy adoption and fresh claim must remain explicit")
	barrierPositions := stateCalls["requireRevertRolloutBarrier"]
	require.Len(t, barrierPositions, 2,
		"target-empty and durable paths must recheck the primary rollout certificate immediately before money movement")
	require.Len(t, stateCalls["acquireLegacyRevertBarrier"], 1)
	require.Len(t, stateCalls["AcquireOwnedKey"], 1,
		"a fresh claim must atomically acquire its balance execution attempt and owner")
	require.Len(t, stateCalls["createRevertTransaction"], 2, "phase zero legacy and bridge/final paths must remain explicit")
	assert.Less(t, barrierPositions[0], stateCalls["createRevertTransaction"][0],
		"a target-empty request paused across initialization must abort before entering the legacy money path")
	assert.Less(t, stateCalls["AcquireOwnedKey"][0], claimPositions[1],
		"a visible PostgreSQL claim must already have the attempt that fences a stale winner")
	assert.Less(t, claimPositions[1], stateCalls["acquireLegacyRevertBarrier"][0],
		"fresh PostgreSQL claim must precede the legacy Redis barrier")
	assert.Less(t, stateCalls["acquireLegacyRevertBarrier"][0], stateCalls["createRevertTransaction"][1],
		"bridge must own the legacy barrier before entering the origin-scoped create path")
	assert.Less(t, barrierPositions[1], stateCalls["createRevertTransaction"][1],
		"the durable path must revalidate the rollout certificate after every barrier acquisition")

	createCalls := callsInFunction(t, createSource, "executeCreateTransaction")
	require.Len(t, createCalls["acquireOriginRevertBarrier"], 1)
	require.Len(t, createCalls["SendTransactionToRedisQueue"], 1)
	require.Len(t, createCalls["ProcessBalanceOperations"], 1)
	assert.Less(t, createCalls["acquireOriginRevertBarrier"][0], createCalls["ProcessBalanceOperations"][0],
		"origin Redis barrier must be acquired before balance mutation")
	assert.Less(t, createCalls["acquireOriginRevertBarrier"][0], createCalls["SendTransactionToRedisQueue"][0],
		"the origin owner companion must exist before the recoverable seed is written")
	originCalls := callsInFunction(t, claimSource, "acquireOriginRevertBarrier")
	require.Len(t, originCalls["AcquireOwnedKey"], 1,
		"the origin barrier and reserved-reverse owner must be one atomic same-slot acquisition")
}

func TestRevertRecoveryNeverBlindDeletesOriginFence(t *testing.T) {
	t.Parallel()

	claimSource, err := os.ReadFile("transaction_revert_claim.go")
	require.NoError(t, err)

	recoveryCalls := callsInFunction(t, claimSource, "recoverProvenPreMovementRevert")
	assert.Empty(t, recoveryCalls["Del"], "a stale RECOVERING owner must never blind-delete a successor's origin barrier")
	require.Len(t, recoveryCalls["releaseOwnedRevertOriginFence"], 1)
	require.Len(t, recoveryCalls["ReleaseProvenPreMovementRevert"], 1)
	assert.Less(t, recoveryCalls["ReleaseProvenPreMovementRevert"][0], recoveryCalls["releaseOwnedRevertOriginFence"][0],
		"recovery must confirm the reverse seed is removed before releasing the shared origin barrier")
	releaseCalls := callsInFunction(t, claimSource, "releaseOwnedRevertOriginFence")
	require.Len(t, releaseCalls["ReleaseOwnedKey"], 1)
	assert.Empty(t, releaseCalls["Del"], "origin cleanup must remain owner-checked")
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
	assert.Contains(t, createGuards[0], "removePreMovementTransactionBackup",
		"the atomic balance outcome may only be removed through the common proof gate")
	assert.Contains(t, createGuards[1], "releaseReservations")

	claimGuards := callsGuardedBy(t, claimSource, "failRevertClaim", "mayReleaseRevertFences")
	require.Len(t, claimGuards, 1)
	assert.Contains(t, claimGuards[0], "releaseFreshRevertClaim",
		"the PostgreSQL claim and legacy Redis fence may only be released through the same proof gate")

	failureCalls := callsInFunction(t, claimSource, "failRevertClaim")
	require.Len(t, failureCalls["RemoveMessageFromQueueIfStatus"], 1)
	require.Len(t, failureCalls["releaseOwnedRevertOriginFence"], 1)
	require.Len(t, failureCalls["ReleaseOwnedKey"], 1)
	require.Len(t, failureCalls["releaseFreshRevertClaim"], 1)
	assert.Less(t, failureCalls["RemoveMessageFromQueueIfStatus"][0], failureCalls["releaseOwnedRevertOriginFence"][0],
		"the reverse-scoped seed must be removed before shared fences")
	assert.Less(t, failureCalls["releaseOwnedRevertOriginFence"][0], failureCalls["ReleaseOwnedKey"][0],
		"origin ownership must be resolved before the execution lease is removed")
	assert.Less(t, failureCalls["ReleaseOwnedKey"][0], failureCalls["releaseFreshRevertClaim"][0],
		"PostgreSQL and the legacy fence must be released last")
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
