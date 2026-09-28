// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package redis

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mtransaction"
	"github.com/LerianStudio/midaz/v4/pkg/utils"
)

// These tests cover the adapter's half of the consumption contract: turning a
// resolved binding into the wire payload. WHICH balances a grant authorizes is
// decided by mtransaction.ResolveAccountBlockExceptionBinding and covered in that
// package; what matters here is that the payload names exactly those balances,
// namespaced identically to the per-operation ARGV groups, and that the no-grant
// path costs nothing.

// evalBinding resolves a binding for a debit on alias#default, optionally with the
// system-derived overdraft companion the enrichment appends when the debit
// overdraws. Built through the production resolver so the adapter is fed the same
// value the pipeline feeds it.
func evalBinding(t *testing.T, orgID, ledgerID uuid.UUID, alias string, withCompanion bool) *mtransaction.AccountBlockExceptionBinding {
	t.Helper()

	leg := func(balanceKey, amount string) mtransaction.AccountBlockExceptionLeg {
		aliasKey := alias + "#" + balanceKey

		return mtransaction.AccountBlockExceptionLeg{
			Alias:      alias,
			BalanceKey: balanceKey,
			// The companion inherits the primary's positional index prefix, which
			// is what marks it as derived from that leg.
			EntryKey:    "0#" + aliasKey,
			Direction:   constant.DirectionDebit,
			Amount:      amount,
			InternalKey: utils.BalanceInternalKey(orgID, ledgerID, aliasKey),
		}
	}

	legs := []mtransaction.AccountBlockExceptionLeg{leg(constant.DefaultBalanceKey, "150.5")}
	if withCompanion {
		legs = append(legs, leg(constant.OverdraftBalanceKey, "40"))
	}

	binding, err := mtransaction.ResolveAccountBlockExceptionBinding(
		&mtransaction.AccountBlockExceptionGrant{ID: uuid.New(), Alias: alias, Amount: "150.5"},
		legs,
	)
	require.NoError(t, err)
	require.NotNil(t, binding)

	return binding
}

// TestResolveAccountBlockExceptionEval_NamespacesTheBoundBalances locks the wire
// payload for a plain single-balance debit.
func TestResolveAccountBlockExceptionEval_NamespacesTheBoundBalances(t *testing.T) {
	t.Parallel()

	orgID, ledgerID := uuid.New(), uuid.New()
	binding := evalBinding(t, orgID, ledgerID, "@source", false)

	eval, err := resolveAccountBlockExceptionEval(context.Background(), orgID, ledgerID, binding)
	require.NoError(t, err)
	require.NotNil(t, eval)

	assert.Equal(t, utils.AccountBlockExceptionInternalKey(orgID, ledgerID, binding.ID), eval.key,
		"KEYS[4] must be the exception's own internal key")
	assert.Equal(t, "@source", eval.alias)
	assert.Equal(t, "150.5", eval.amount,
		"the expected amount is the primary debited leg's, carried through from the binding")
	assert.Equal(t,
		[]string{utils.BalanceInternalKey(orgID, ledgerID, "@source#"+constant.DefaultBalanceKey)},
		eval.balanceKeys,
		"single-tenant context leaves the balance keys unprefixed, matching the per-operation ARGV")
}

// TestResolveAccountBlockExceptionEval_CarriesTheOverdraftCompanion is the
// adapter-side half of the overdraft regression: the bypass list must name BOTH
// balances of the one logical debit, or the block guard rejects on the companion
// and a valid grant is unusable on every overdrawing transaction.
func TestResolveAccountBlockExceptionEval_CarriesTheOverdraftCompanion(t *testing.T) {
	t.Parallel()

	orgID, ledgerID := uuid.New(), uuid.New()
	binding := evalBinding(t, orgID, ledgerID, "@source", true)

	eval, err := resolveAccountBlockExceptionEval(context.Background(), orgID, ledgerID, binding)
	require.NoError(t, err)
	require.NotNil(t, eval)

	assert.ElementsMatch(t, []string{
		utils.BalanceInternalKey(orgID, ledgerID, "@source#"+constant.DefaultBalanceKey),
		utils.BalanceInternalKey(orgID, ledgerID, "@source#"+constant.OverdraftBalanceKey),
	}, eval.balanceKeys)

	assert.Equal(t, "150.5", eval.amount,
		"the companion's own portion must never become the amount the grant is matched against")
	assert.Equal(t, 2, eval.bypassedCount())
}

// TestResolveAccountBlockExceptionEval_NoBindingIsNoEval keeps the no-grant path
// allocation-free and nil-safe.
func TestResolveAccountBlockExceptionEval_NoBindingIsNoEval(t *testing.T) {
	t.Parallel()

	eval, err := resolveAccountBlockExceptionEval(context.Background(), uuid.New(), uuid.New(), nil)

	require.NoError(t, err)
	assert.Nil(t, eval)
	assert.Zero(t, eval.bypassedCount(), "the nil receiver must answer without a guard at the call site")
}

// TestAccountBlockExceptionEval_HeaderLayout locks the ARGV header on both sides
// of the presented/absent split.
//
// The five fixed slots are ALWAYS occupied — the third carries the bypass count,
// which is what lets the script derive its own stride without branching on
// whether a grant exists, and the fourth and fifth carry the idempotency marker
// key of this execution and the marker key of the opposite terminal status,
// which the script's replay and cross-transition gates read before any grant
// concern. A header that shrank when no grant was presented would shift every
// balance operation and silently mis-parse the whole batch.
func TestAccountBlockExceptionEval_HeaderLayout(t *testing.T) {
	t.Parallel()

	const (
		markerKey         = "transaction_apply_marker:{transactions}:o:l:t:APPROVED"
		oppositeMarkerKey = "transaction_apply_marker:{transactions}:o:l:t:CANCELED"
	)

	t.Run("no grant occupies the fixed slots and declares a zero count", func(t *testing.T) {
		t.Parallel()

		var absent *accountBlockExceptionEval

		require.Equal(t, luaArgsHeaderFixedSize, absent.headerWidth())

		args := make([]any, absent.headerWidth())
		absent.writeHeader(args, markerKey, oppositeMarkerKey)

		assert.Equal(t, []any{"", "", "0", markerKey, oppositeMarkerKey}, args)
	})

	t.Run("a grant declares its count and lists its keys", func(t *testing.T) {
		t.Parallel()

		eval := &accountBlockExceptionEval{
			key:    "account_block_exception:{transactions}:o:l:e",
			alias:  "@source",
			amount: "150.5",
			balanceKeys: []string{
				"balance:{transactions}:o:l:@source#default",
				"balance:{transactions}:o:l:@source#overdraft",
			},
		}

		require.Equal(t, luaArgsHeaderFixedSize+2, eval.headerWidth())

		args := make([]any, eval.headerWidth())
		eval.writeHeader(args, markerKey, oppositeMarkerKey)

		assert.Equal(t, []any{
			"@source",
			"150.5",
			"2",
			markerKey,
			oppositeMarkerKey,
			"balance:{transactions}:o:l:@source#default",
			"balance:{transactions}:o:l:@source#overdraft",
		}, args, "the header must be alias, amount, count, marker key, opposite marker key, then one slot per bypassed key")
	})
}

// TestAccountBlockExceptionEval_WriteHeaderLeavesTheBatchAlone proves the header
// is written into the slots the plan builder RESERVED and touches nothing after
// them — which is what makes writing in place (instead of prepending to a fresh
// slice) safe on the hot path.
func TestAccountBlockExceptionEval_WriteHeaderLeavesTheBatchAlone(t *testing.T) {
	t.Parallel()

	eval := &accountBlockExceptionEval{
		alias:       "@source",
		amount:      "100",
		balanceKeys: []string{"balance:{transactions}:o:l:@source#default"},
	}

	args := make([]any, eval.headerWidth(), eval.headerWidth()+2)
	args = append(args, "first-operation-slot", "second-operation-slot")

	eval.writeHeader(args,
		"transaction_apply_marker:{transactions}:o:l:t:APPROVED",
		"transaction_apply_marker:{transactions}:o:l:t:CANCELED")

	assert.Equal(t, "first-operation-slot", args[eval.headerWidth()])
	assert.Equal(t, "second-operation-slot", args[eval.headerWidth()+1])
}
