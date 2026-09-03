// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package redis

import (
	"strings"
	"testing"

	libCommons "github.com/LerianStudio/lib-commons/v6/commons"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	"github.com/LerianStudio/midaz/v4/pkg/mtransaction"
	"github.com/LerianStudio/midaz/v4/pkg/utils"
)

// =============================================================================
// BLOCK GATE — ARGV INPUT CONTRACT (UNIT)
// =============================================================================
// The atomic script decides the block verdict itself, so what Go emits IS the
// gate's input contract. These tests pin the two halves that cannot drift
// apart: the position of the per-operation grant flag inside the ARGV group,
// and the fact that the Lua script reads the gate from exactly those slots.

// blockGateOp builds one balance operation carrying an explicit exception-grant
// state on its Amount carrier, which is where the enrichment layer publishes it.
func blockGateOp(t *testing.T, grantedExceptionID string, bypassGranted bool) mmodel.BalanceOperation {
	t.Helper()

	organizationID := uuid.Must(libCommons.GenerateUUIDv7())
	ledgerID := uuid.Must(libCommons.GenerateUUIDv7())

	return mmodel.BalanceOperation{
		Balance: &mmodel.Balance{
			ID:             uuid.Must(libCommons.GenerateUUIDv7()).String(),
			OrganizationID: organizationID.String(),
			LedgerID:       ledgerID.String(),
			AccountID:      uuid.Must(libCommons.GenerateUUIDv7()).String(),
			Alias:          "@gate",
			Key:            "default",
			AssetCode:      "USD",
			Available:      decimal.NewFromInt(1000),
			OnHold:         decimal.Zero,
			Version:        1,
			AccountType:    "deposit",
			AllowSending:   true,
			AllowReceiving: true,
		},
		Alias: "@gate",
		Amount: mtransaction.Amount{
			Asset:              "USD",
			Value:              decimal.NewFromInt(10),
			Operation:          constant.DEBIT,
			BlockBypassGranted: bypassGranted,
			GrantedExceptionID: grantedExceptionID,
		},
		InternalKey: utils.BalanceInternalKey(organizationID, ledgerID, "default"),
	}
}

// TestBuildPlan_ExceptionGrantIsLastARGV pins the grant flag to the END of the
// per-balance ARGV group. Every earlier index is a positional contract with the
// Lua parsing loop, so the gate input may only be appended.
func TestBuildPlan_ExceptionGrantIsLastARGV(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name               string
		grantedExceptionID string
		bypassGranted      bool
		want               int
	}{
		{name: "no grant at all is sent as 0", want: 0},
		{
			name:               "a granted exception id is sent as 1",
			grantedExceptionID: uuid.NewString(),
			want:               1,
		},
		{
			name:          "the legacy bypass flag alone is sent as 1",
			bypassGranted: true,
			want:          1,
		},
		{
			name:               "both carriers set is still 1",
			grantedExceptionID: uuid.NewString(),
			bypassGranted:      true,
			want:               1,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			repo := &RedisConsumerRepository{conn: newFailOnCallConnection(t)}

			plan, err := repo.buildBalanceAtomicOperationPlan(
				t.Context(), constant.APPROVED, false,
				[]mmodel.BalanceOperation{blockGateOp(t, tc.grantedExceptionID, tc.bypassGranted)},
			)
			require.NoError(t, err)
			require.Len(t, plan.args, luaArgsPerOperation,
				"one balance must produce exactly one ARGV group")

			assert.Equal(t, tc.want, plan.args[luaArgsPerOperation-1],
				"ARGV[i+%d] exception grant must be the LAST entry of the group", luaArgsPerOperation-1)
		})
	}
}

// TestBuildPlan_ExceptionGrantIsPerOperation proves the grant travels per
// operation rather than per transaction: a batch mixing a granted and an
// ungranted leg must emit different flags in the same call.
func TestBuildPlan_ExceptionGrantIsPerOperation(t *testing.T) {
	t.Parallel()

	repo := &RedisConsumerRepository{conn: newFailOnCallConnection(t)}

	plan, err := repo.buildBalanceAtomicOperationPlan(
		t.Context(), constant.APPROVED, false,
		[]mmodel.BalanceOperation{
			blockGateOp(t, "", false),
			blockGateOp(t, uuid.NewString(), false),
		},
	)
	require.NoError(t, err)
	require.Len(t, plan.args, 2*luaArgsPerOperation)

	assert.Equal(t, 0, plan.args[luaArgsPerOperation-1], "1st operation carries no grant")
	assert.Equal(t, 1, plan.args[2*luaArgsPerOperation-1], "2nd operation carries a grant")
}

// TestLuaScript_BlockGateReadsTheGoContract keeps the Lua half of the gate in
// lock-step with what Go emits. A drift here does not fail loudly — it silently
// reads the wrong slot and turns the gate into a coin flip.
func TestLuaScript_BlockGateReadsTheGoContract(t *testing.T) {
	t.Parallel()

	flat := strings.ReplaceAll(balanceAtomicOperationLua, "\n", " ")

	assert.Contains(t, balanceAtomicOperationLua, "local groupSize = 26",
		"the Lua groupSize must match luaArgsPerOperation")
	assert.Contains(t, flat, "ARGV[i + 25]",
		"the gate must read the per-operation grant from the last ARGV slot")
	assert.Contains(t, flat, "KEYS[4]",
		"the blocked-accounts SET key is supplied by Go as KEYS[4], never rebuilt in-script")
	assert.Contains(t, flat, `"SMISMEMBER"`,
		"the gate probes the SET with SMISMEMBER so the sentinel and the accounts cost one call")
	assert.Contains(t, flat, "NEEDS_HYDRATION",
		"a SET without the hydration sentinel must return NEEDS_HYDRATION")
	assert.Contains(t, flat, `"BLOCKED:"`,
		"a denial must name the blocked account")
}

// TestLuaScript_BlockGatePrecedesEveryMutation is the structural half of the
// two-pass guarantee: the gate must appear in the script BEFORE the first write
// command. Asserting on order rather than on behaviour catches the refactor
// that moves the gate down into the mutation loop, where a denial would leave
// earlier balances already written.
func TestLuaScript_BlockGatePrecedesEveryMutation(t *testing.T) {
	t.Parallel()

	// main() is where the pass structure lives; the helper functions above it
	// define writes that only run when main calls them.
	mainBody := balanceAtomicOperationLua[strings.Index(balanceAtomicOperationLua, "local function main()"):]

	gateAt := strings.Index(mainBody, "NEEDS_HYDRATION")
	require.GreaterOrEqual(t, gateAt, 0, "the gate must live inside main()")

	firstWriteAt := strings.Index(mainBody, `redis.call("SET"`)
	require.GreaterOrEqual(t, firstWriteAt, 0, "the mutation pass must still write balances")

	assert.Less(t, gateAt, firstWriteAt,
		"pass 1 (checks) must complete before pass 2 (mutations): no denial may follow a partial write")
}
