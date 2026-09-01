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

// accountBlockedOp builds a single-balance operation carrying an explicit
// account-block state, so the ARGV assertions below read the value they set.
func accountBlockedOp(t *testing.T, blocked bool) mmodel.BalanceOperation {
	t.Helper()

	organizationID := uuid.Must(libCommons.GenerateUUIDv7())
	ledgerID := uuid.Must(libCommons.GenerateUUIDv7())

	return mmodel.BalanceOperation{
		Balance: &mmodel.Balance{
			ID:             uuid.Must(libCommons.GenerateUUIDv7()).String(),
			OrganizationID: organizationID.String(),
			LedgerID:       ledgerID.String(),
			AccountID:      uuid.Must(libCommons.GenerateUUIDv7()).String(),
			Alias:          "@blocked",
			Key:            "default",
			AssetCode:      "USD",
			Available:      decimal.NewFromInt(1000),
			OnHold:         decimal.Zero,
			Version:        1,
			AccountType:    "deposit",
			AllowSending:   true,
			AllowReceiving: true,
			AccountBlocked: blocked,
		},
		Alias: "@blocked",
		Amount: mtransaction.Amount{
			Asset:     "USD",
			Value:     decimal.NewFromInt(10),
			Operation: constant.DEBIT,
		},
		InternalKey: utils.BalanceInternalKey(organizationID, ledgerID, "default"),
	}
}

// TestBuildPlan_AccountBlockedIsLastARGV pins the new field to the END of the
// per-balance ARGV block. Every existing position is a contract with the Lua
// script's parsing loop, so the flag may only be appended — never inserted.
func TestBuildPlan_AccountBlockedIsLastARGV(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		blocked bool
		want    int
	}{
		{name: "blocked account is sent as 1", blocked: true, want: 1},
		{name: "unblocked account is sent as 0", blocked: false, want: 0},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			repo := &RedisConsumerRepository{conn: newFailOnCallConnection(t)}

			plan, err := repo.buildBalanceAtomicOperationPlan(
				t.Context(), constant.APPROVED, false, []mmodel.BalanceOperation{accountBlockedOp(t, tc.blocked)},
			)
			require.NoError(t, err)
			require.NotNil(t, plan)
			require.Len(t, plan.args, luaArgsPerOperation,
				"one balance must produce exactly one ARGV group")

			assert.Equal(t, tc.want, plan.args[luaArgsPerOperation-1],
				"ARGV[i+%d] balance.AccountBlocked must be the LAST entry of the group", luaArgsPerOperation-1)
		})
	}
}

// TestBuildPlan_AccountBlockedStrideIsAtomic guards the Go half of the stride
// contract. The Lua half is asserted separately against the embedded script.
func TestBuildPlan_AccountBlockedStrideIsAtomic(t *testing.T) {
	t.Parallel()

	assert.Equal(t, 25, luaArgsPerOperation,
		"luaArgsPerOperation must be 25: 17 base + 7 overdraft + 1 account-block field")

	repo := &RedisConsumerRepository{conn: newFailOnCallConnection(t)}

	plan, err := repo.buildBalanceAtomicOperationPlan(
		t.Context(), constant.APPROVED, false,
		[]mmodel.BalanceOperation{accountBlockedOp(t, false), accountBlockedOp(t, true)},
	)
	require.NoError(t, err)
	require.Len(t, plan.args, 2*luaArgsPerOperation,
		"two operations must produce two ARGV groups of the same stride")

	assert.Equal(t, 0, plan.args[luaArgsPerOperation-1], "1st balance AccountBlocked")
	assert.Equal(t, 1, plan.args[2*luaArgsPerOperation-1], "2nd balance AccountBlocked")
}

// TestLuaScript_StrideMatchesGoStride keeps the Go constant and the Lua
// groupSize in lock-step. A drift between the two silently misreads every field
// of every balance after the first in a batch, so it is asserted at unit level
// rather than only through a live Redis.
func TestLuaScript_StrideMatchesGoStride(t *testing.T) {
	t.Parallel()

	assert.Contains(t, balanceAtomicOperationLua, "local groupSize = 25",
		"the Lua groupSize must match luaArgsPerOperation")
	assert.Contains(t, balanceAtomicOperationLua, "AccountBlocked = tonumber(ARGV[i + 24])",
		"the Lua script must read the account-block flag from the last ARGV slot")
	assert.Contains(t, strings.ReplaceAll(balanceAtomicOperationLua, "\n", " "),
		"if balance.AccountBlocked == nil then",
		"the cache-hit backfill must default a legacy entry's AccountBlocked, never leave it nil")
}

// TestBalanceRedisToBalance_CopiesAccountBlocked locks the Postgres-map half of
// the conversion: like the allow flags, the block state is authoritative on the
// balance the caller loaded, not on the Lua payload.
func TestBalanceRedisToBalance_CopiesAccountBlocked(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		blocked bool
	}{
		{name: "blocked", blocked: true},
		{name: "unblocked", blocked: false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			mapBalances := map[string]*mmodel.Balance{
				"@blocked": {
					Alias:          "@blocked",
					Key:            "default",
					AssetCode:      "USD",
					AllowSending:   true,
					AllowReceiving: true,
					AccountBlocked: tc.blocked,
				},
			}

			got := balanceRedisToBalance(mmodel.BalanceRedis{
				ID:            uuid.Must(libCommons.GenerateUUIDv7()).String(),
				Alias:         "@blocked",
				Available:     decimal.NewFromInt(10),
				OnHold:        decimal.Zero,
				Version:       2,
				OverdraftUsed: "0",
			}, mapBalances)

			require.NotNil(t, got)
			assert.Equal(t, tc.blocked, got.AccountBlocked,
				"AccountBlocked must be carried through the Lua->domain conversion")
		})
	}
}
