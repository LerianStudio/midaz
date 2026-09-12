//go:build integration

// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package engine

import (
	"context"
	"encoding/json"
	"math"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/redis/balancecache"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/domain/accounting"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/services/command"
	"github.com/LerianStudio/midaz/v4/internal/cachepolicy"
	redistestutil "github.com/LerianStudio/midaz/v4/tests/utils/redis"
)

var integrationEngineLua = accountingScriptSource

type integrationState struct {
	Available     string `json:"available"`
	OnHold        string `json:"onHold"`
	OverdraftUsed string `json:"overdraftUsed"`
	Version       string `json:"version"`
}

type integrationMovement struct {
	Ref            string           `json:"ref"`
	TransactionID  string           `json:"transactionId"`
	PostingRef     string           `json:"postingRef"`
	Role           string           `json:"role"`
	BalanceRef     string           `json:"balanceRef"`
	Type           string           `json:"type"`
	Amount         string           `json:"amount"`
	OverdraftDelta string           `json:"overdraftDelta"`
	Before         integrationState `json:"before"`
	After          integrationState `json:"after"`
}

type integrationFinal struct {
	wireBalanceSnapshot
	BalanceRef string `json:"balanceRef"`
}

type integrationResult struct {
	ProtocolVersion int                   `json:"protocolVersion"`
	Movements       []integrationMovement `json:"movements"`
	Final           []integrationFinal    `json:"final"`
}

type integrationFixture struct {
	input    command.EngineExecution
	limits   Limits
	resolved resolvedExecutionKeys
	client   redis.UniversalClient
}

func newIntegrationFixture(t *testing.T, client redis.UniversalClient) *integrationFixture {
	t.Helper()

	input, limits, resolved := validWireExecution()
	input.Execution.Balances[0].Available = decimal.NewFromInt(100)
	input.Execution.Balances[0].Version = 0
	input.Execution.Balances[0].AllowOverdraft = true
	input.Execution.Transactions[0].Postings[0].Amount = decimal.NewFromInt(30)
	input.CompletionPlans[0].Payload = json.RawMessage(`{"opaque":true,"version":9007199254740993}`)
	prefix := "test:" + strings.ReplaceAll(t.Name(), "/", ":") + ":"
	replace := func(key string) string { return strings.Replace(key, "tenant:fixture:", prefix, 1) }
	resolved.Schedule, resolved.Recovery = replace(resolved.Schedule), replace(resolved.Recovery)
	resolved.Receipts, resolved.Guards, resolved.Protection = replace(resolved.Receipts), replace(resolved.Guards), replace(resolved.Protection)
	for ref, pair := range resolved.Balances {
		resolved.Balances[ref] = resolvedBalanceKeys{
			Balance: replace(pair.Balance), Deleted: replace(pair.Deleted), LegacyDeleted: replace(pair.LegacyDeleted),
		}
	}
	fixture := &integrationFixture{input: input, limits: limits, resolved: resolved, client: client}
	t.Cleanup(func() {
		keys := []string{resolved.Schedule, resolved.Recovery, resolved.Receipts, resolved.Guards, resolved.Protection}
		for _, pair := range fixture.resolved.Balances {
			keys = append(keys, pair.Balance, pair.Deleted, pair.LegacyDeleted)
		}
		require.NoError(t, client.Del(context.Background(), keys...).Err())
	})
	return fixture
}

func (f *integrationFixture) addCompanion(available string) {
	balance := f.input.Execution.Balances[0]
	balance.ID = uuid.MustParse("abaedbdc-371a-46f9-945a-a95617d8b032")
	balance.BalanceRef, balance.Key = "@source#overdraft", "overdraft"
	balance.Direction, balance.BalanceScope = "debit", "internal"
	balance.Available, balance.OnHold, balance.OverdraftUsed = decimal.RequireFromString(available), decimal.Zero, decimal.Zero
	balance.AllowOverdraft = false
	f.input.Execution.Balances = append(f.input.Execution.Balances, balance)
	key := strings.Replace(f.resolved.Balances["@source#default"].Balance, "#default", "#overdraft", 1)
	f.resolved.Balances[balance.BalanceRef] = testResolvedBalanceKeys(key)
}

func (f *integrationFixture) prepared(t *testing.T) *preparedExecution {
	t.Helper()
	prepared, err := prepareExecution(context.Background(), f.input, f.limits, f.resolved)
	require.NoError(t, err)
	return prepared
}

func (f *integrationFixture) runRaw(t *testing.T, payload string) (string, error) {
	t.Helper()
	prepared := f.prepared(t)
	return f.client.Eval(context.Background(), integrationEngineLua, prepared.Keys, payload,
		strconv.Itoa(f.limits.MaxRequestBytes), strconv.Itoa(f.limits.MaxPreparedBytes)).Text()
}

func (f *integrationFixture) run(t *testing.T) (string, error) {
	t.Helper()
	return f.runRaw(t, string(f.prepared(t).Payload))
}

func (f *integrationFixture) seed(t *testing.T, index int, snapshot accounting.BalanceSnapshot) {
	t.Helper()
	encoded, err := balancecache.Encode(snapshot, balancecache.FormatDual)
	require.NoError(t, err)
	key := f.resolved.Balances[f.input.Execution.Balances[index].BalanceRef].Balance
	require.NoError(t, f.client.Set(context.Background(), key, encoded, time.Hour).Err())
}

type integrationStoredKey struct {
	Exists bool
	Dump   string
	Expiry int64
}

func (f *integrationFixture) capture(t *testing.T) map[string]integrationStoredKey {
	t.Helper()
	values := make(map[string]integrationStoredKey)
	for _, key := range f.prepared(t).Keys {
		dump, err := f.client.Dump(context.Background(), key).Result()
		exists := err == nil
		if !exists {
			require.ErrorIs(t, err, redis.Nil)
		}
		expiry, err := f.client.Do(context.Background(), "PEXPIRETIME", key).Int64()
		require.NoError(t, err)
		values[key] = integrationStoredKey{Exists: exists, Dump: dump, Expiry: expiry}
	}
	return values
}

func decodeIntegrationResult(t *testing.T, raw string) integrationResult {
	t.Helper()
	var result integrationResult
	require.NoError(t, json.Unmarshal([]byte(raw), &result))
	require.Equal(t, 1, result.ProtocolVersion)
	require.NotNil(t, result.Movements)
	require.NotNil(t, result.Final)
	return result
}

func finalState(final integrationFinal) integrationState {
	return integrationState{Available: final.Available, OnHold: final.OnHold, OverdraftUsed: final.OverdraftUsed, Version: final.Version}
}

func TestIntegrationEnginePostingAlgebra(t *testing.T) {
	if testing.Short() {
		t.Skip("requires Valkey")
	}
	container := redistestutil.SetupReusableContainer(t)
	s := func(a, h, u, v string) integrationState { return integrationState{a, h, u, v} }
	tests := []struct {
		name, direction, accountType, available, onHold, debt, amount, override, companion string
		posting                                                                            accounting.PostingType
		want                                                                               integrationState
		wantCompanion, wantAmount, wantDelta, failure                                      string
	}{
		{name: "debit credit direction", posting: accounting.PostingDebit, available: "100", amount: "30", want: s("70", "0", "0", "1"), wantAmount: "30"},
		{name: "credit credit direction", posting: accounting.PostingCredit, available: "100", amount: "30", want: s("130", "0", "0", "1"), wantAmount: "30"},
		{name: "debit debit direction", direction: "debit", posting: accounting.PostingDebit, available: "100", amount: "30", want: s("130", "0", "0", "1"), wantAmount: "30"},
		{name: "credit debit direction", direction: "debit", posting: accounting.PostingCredit, available: "100", amount: "30", want: s("70", "0", "0", "1"), wantAmount: "30"},
		{name: "debit empty direction", direction: "empty", posting: accounting.PostingDebit, available: "100", amount: "30", want: s("70", "0", "0", "1"), wantAmount: "30"},
		{name: "credit empty direction", direction: "empty", posting: accounting.PostingCredit, available: "100", amount: "30", want: s("130", "0", "0", "1"), wantAmount: "30"},
		{name: "external negative", accountType: "external", posting: accounting.PostingDebit, available: "0", amount: "30", want: s("-30", "0", "0", "1"), wantAmount: "30"},
		{name: "external hold negative", accountType: "external", posting: accounting.PostingHold, available: "0", amount: "30", want: s("-30", "30", "0", "1"), wantAmount: "30"},
		{name: "reserve does not debit again", posting: accounting.PostingReserve, available: "40", amount: "60", want: s("40", "60", "0", "1"), wantAmount: "60"},
		{name: "unreserve", posting: accounting.PostingUnreserve, available: "40", onHold: "60", amount: "60", want: s("40", "0", "0", "1"), wantAmount: "60"},
		{name: "hold", posting: accounting.PostingHold, available: "100", amount: "60", want: s("40", "60", "0", "1"), wantAmount: "60"},
		{name: "hold debit direction", direction: "debit", posting: accounting.PostingHold, available: "100", amount: "60", want: s("160", "60", "0", "1"), wantAmount: "60"},
		{name: "release preserves debt without cap", posting: accounting.PostingRelease, available: "0", onHold: "50", debt: "50", amount: "50", want: s("50", "0", "50", "1"), wantAmount: "50"},
		{name: "release legacy cap", posting: accounting.PostingRelease, available: "0", onHold: "50", debt: "50", amount: "50", override: "20", companion: "50", want: s("30", "0", "30", "1"), wantCompanion: "30", wantAmount: "30", wantDelta: "-20"},
		{name: "release debit direction", direction: "debit", posting: accounting.PostingRelease, available: "100", onHold: "50", amount: "50", want: s("50", "0", "0", "1"), wantAmount: "50"},
		{name: "draw zero primary amount", posting: accounting.PostingDebit, available: "0", amount: "50", companion: "0", want: s("0", "0", "50", "1"), wantCompanion: "50", wantAmount: "0", wantDelta: "50"},
		{name: "partial repay", posting: accounting.PostingCredit, available: "0", debt: "50", amount: "20", companion: "50", want: s("0", "0", "30", "1"), wantCompanion: "30", wantAmount: "0", wantDelta: "-20"},
		{name: "full repay zero primary amount", posting: accounting.PostingCredit, available: "0", debt: "50", amount: "50", companion: "50", want: s("0", "0", "0", "1"), wantCompanion: "0", wantAmount: "0", wantDelta: "-50"},
		{name: "repay remainder", posting: accounting.PostingCredit, available: "0", debt: "50", amount: "70", companion: "50", want: s("20", "0", "0", "1"), wantCompanion: "0", wantAmount: "20", wantDelta: "-50"},
		{name: "credit legacy cap", posting: accounting.PostingCredit, available: "0", debt: "50", amount: "30", override: "10", companion: "50", want: s("20", "0", "40", "1"), wantCompanion: "40", wantAmount: "20", wantDelta: "-10"},
		{name: "hold refuses debt", posting: accounting.PostingHold, available: "100", amount: "101", failure: "insufficient_funds"},
		{name: "missing companion", posting: accounting.PostingDebit, available: "0", amount: "50", failure: "overdraft_companion_missing"},
		{name: "onhold underflow", posting: accounting.PostingUnreserve, available: "40", onHold: "49", amount: "50", failure: "onhold_underflow"},
		{name: "debit direction floor", direction: "debit", posting: accounting.PostingCredit, available: "40", amount: "50", failure: "insufficient_funds"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newIntegrationFixture(t, container.Client)
			balance := &f.input.Execution.Balances[0]
			balance.Available = decimal.RequireFromString(tt.available)
			if tt.onHold != "" {
				balance.OnHold = decimal.RequireFromString(tt.onHold)
			}
			if tt.debt != "" {
				balance.OverdraftUsed = decimal.RequireFromString(tt.debt)
			}
			if tt.direction != "" {
				balance.Direction = tt.direction
			}
			if tt.direction == "empty" {
				balance.Direction = ""
			}
			if tt.accountType != "" {
				balance.AccountType = tt.accountType
			}
			posting := &f.input.Execution.Transactions[0].Postings[0]
			posting.Type, posting.Amount = tt.posting, decimal.RequireFromString(tt.amount)
			if tt.override != "" {
				posting.OverdraftAmount = decimal.RequireFromString(tt.override)
			}
			if tt.companion != "" {
				f.addCompanion(tt.companion)
			}
			before := f.capture(t)
			raw, err := f.run(t)
			if tt.failure != "" {
				require.ErrorContains(t, err, `"code":"`+tt.failure+`"`)
				require.Equal(t, before, f.capture(t), "refusal must not create seeds or mutate any key")
				return
			}
			require.NoError(t, err)
			result := decodeIntegrationResult(t, raw)
			decodedResult, err := DecodeResult([]byte(raw), f.input.Execution)
			require.NoError(t, err)
			require.Len(t, decodedResult.Movements, len(result.Movements))
			require.Len(t, decodedResult.Final, len(result.Final))
			committed := f.capture(t)
			replayed, err := f.run(t)
			require.NoError(t, err)
			require.Equal(t, raw, replayed)
			require.Equal(t, committed, f.capture(t))
			require.Equal(t, tt.want, finalState(result.Final[0]))
			require.Equal(t, tt.wantAmount, result.Movements[0].Amount)
			delta := tt.wantDelta
			if delta == "" {
				delta = "0"
			}
			require.Equal(t, delta, result.Movements[0].OverdraftDelta)
			require.Equal(t, "primary", result.Movements[0].Role)
			if tt.companion != "" {
				require.Len(t, result.Movements, 2)
				require.Len(t, result.Final, 2)
				require.Equal(t, "overdraft_companion", result.Movements[1].Role)
				require.Equal(t, result.Movements[0].PostingRef, result.Movements[1].PostingRef)
				require.Equal(t, tt.wantCompanion, result.Final[1].Available)
			} else {
				require.Len(t, result.Movements, 1)
				require.Len(t, result.Final, 1)
			}
			members, err := container.Client.ZRange(context.Background(), f.resolved.Schedule, 0, -1).Result()
			require.NoError(t, err)
			require.Len(t, members, len(result.Final))
			for _, final := range result.Final {
				key := f.resolved.Balances[final.BalanceRef].Balance
				require.Contains(t, members, key)
				cached, err := container.Client.Get(context.Background(), key).Bytes()
				require.NoError(t, err)
				decoded, err := balancecache.Decode(cached)
				require.NoError(t, err)
				require.Equal(t, final.Available, decoded.Available.String())
				require.Equal(t, final.OnHold, decoded.OnHold.String())
				require.Equal(t, final.OverdraftUsed, decoded.OverdraftUsed.String())
				require.Equal(t, final.Version, strconv.FormatInt(decoded.Version, 10))
			}
		})
	}
}

func TestIntegrationEngineCompositionAndLiveSettings(t *testing.T) {
	if testing.Short() {
		t.Skip("requires Valkey")
	}
	container := redistestutil.SetupReusableContainer(t)
	for _, amount := range []string{"60", "100", "101"} {
		t.Run("validated hold "+amount, func(t *testing.T) {
			f := newIntegrationFixture(t, container.Client)
			debit := f.input.Execution.Transactions[0].Postings[0]
			debit.Amount, debit.DrawPolicy = decimal.RequireFromString(amount), accounting.DrawForbidden
			reserve := debit
			reserve.Ref, reserve.Type = "reserve", accounting.PostingReserve
			f.input.Execution.Transactions[0].Postings = []accounting.Posting{debit, reserve}
			before := f.capture(t)
			raw, err := f.run(t)
			if amount == "101" {
				require.ErrorContains(t, err, `"code":"insufficient_funds"`)
				require.Equal(t, before, f.capture(t))
				return
			}
			require.NoError(t, err)
			result := decodeIntegrationResult(t, raw)
			require.Len(t, result.Movements, 2)
			available := "40"
			if amount == "100" {
				available = "0"
			}
			require.Equal(t, integrationState{available, amount, "0", "2"}, finalState(result.Final[0]))
			require.Equal(t, result.Movements[0].After, result.Movements[1].Before)
		})
	}
	t.Run("cancel restores held debt", func(t *testing.T) {
		f := newIntegrationFixture(t, container.Client)
		f.input.Execution.Balances[0].Available = decimal.Zero
		f.input.Execution.Balances[0].OnHold = decimal.NewFromInt(50)
		f.input.Execution.Balances[0].OverdraftUsed = decimal.NewFromInt(50)
		f.addCompanion("50")
		unreserve := f.input.Execution.Transactions[0].Postings[0]
		unreserve.Type, unreserve.Amount = accounting.PostingUnreserve, decimal.NewFromInt(50)
		credit := unreserve
		credit.Ref, credit.Type = "credit", accounting.PostingCredit
		f.input.Execution.Transactions[0].Postings = []accounting.Posting{unreserve, credit}
		raw, err := f.run(t)
		require.NoError(t, err)
		result := decodeIntegrationResult(t, raw)
		require.Len(t, result.Movements, 3)
		require.Equal(t, integrationState{"0", "0", "0", "2"}, finalState(result.Final[0]))
		require.Equal(t, "0", result.Movements[1].Amount)
	})
	for _, amount := range []int64{50, 51} {
		t.Run("live limit "+strconv.FormatInt(amount, 10), func(t *testing.T) {
			f := newIntegrationFixture(t, container.Client)
			f.input.Execution.Balances[0].Available = decimal.Zero
			f.input.Execution.Balances[0].AllowOverdraft = false
			f.input.Execution.Transactions[0].Postings[0].Amount = decimal.NewFromInt(amount)
			f.addCompanion("0")
			live := f.input.Execution.Balances[0]
			live.AllowOverdraft, live.OverdraftLimitEnabled = true, true
			live.OverdraftLimit = decimal.NewFromInt(50)
			f.seed(t, 0, live)
			before := f.capture(t)
			raw, err := f.run(t)
			if amount == 51 {
				require.ErrorContains(t, err, `"code":"overdraft_limit_exceeded"`)
				require.Equal(t, before, f.capture(t))
				return
			}
			require.NoError(t, err)
			require.Equal(t, "50", decodeIntegrationResult(t, raw).Final[0].OverdraftUsed)
		})
	}
}

func TestIntegrationEngineValidatesBalanceRequirementsAgainstLiveState(t *testing.T) {
	if testing.Short() {
		t.Skip("requires Valkey")
	}

	container := redistestutil.SetupReusableContainer(t)
	tests := []struct {
		name          string
		permission    accounting.BalancePermission
		forbid        bool
		rejectBlocked bool
		prepare       func(*accounting.BalanceSnapshot)
		live          func(*accounting.BalanceSnapshot)
		failure       string
	}{
		{
			name: "live sending permission", permission: accounting.BalancePermissionSend, failure: accounting.FailureSendingNotAllowed,
			live: func(balance *accounting.BalanceSnapshot) { balance.AllowSending = false },
		},
		{
			name: "live receiving permission", permission: accounting.BalancePermissionReceive, failure: accounting.FailureReceivingNotAllowed,
			live: func(balance *accounting.BalanceSnapshot) { balance.AllowReceiving = false },
		},
		{
			name: "live account block", permission: accounting.BalancePermissionSend, rejectBlocked: true, failure: accounting.FailureAccountBlocked,
			live: func(balance *accounting.BalanceSnapshot) { balance.Blocked = true },
		},
		{
			name: "transaction asset", permission: accounting.BalancePermissionSend, failure: accounting.FailureAssetMismatch,
			prepare: func(balance *accounting.BalanceSnapshot) { balance.AssetCode = "EUR" },
		},
		{
			name: "external pending source", permission: accounting.BalancePermissionSend, forbid: true, failure: accounting.FailureExternalHoldNotAllowed,
			prepare: func(balance *accounting.BalanceSnapshot) { balance.AccountType = "external" },
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fixture := newIntegrationFixture(t, container.Client)
			seed := &fixture.input.Execution.Balances[0]
			if test.prepare != nil {
				test.prepare(seed)
			}
			fixture.input.Execution.Transactions[0].BalanceRequirements = []accounting.BalanceRequirement{{
				BalanceRef: seed.BalanceRef, AssetCode: "USD", Permission: test.permission, ForbidExternal: test.forbid,
			}}
			fixture.input.Execution.Transactions[0].RejectBlockedBalances = test.rejectBlocked

			live := *seed
			if test.live != nil {
				test.live(&live)
			}
			fixture.seed(t, 0, live)
			before := fixture.capture(t)

			_, err := fixture.run(t)
			require.ErrorContains(t, err, `"code":"`+test.failure+`"`)
			require.Equal(t, before, fixture.capture(t), "eligibility refusal must not mutate any key")
		})
	}
}

func TestIntegrationEngineUsesLivePermissionInsteadOfSeedPermission(t *testing.T) {
	if testing.Short() {
		t.Skip("requires Valkey")
	}

	container := redistestutil.SetupReusableContainer(t)
	fixture := newIntegrationFixture(t, container.Client)
	seed := &fixture.input.Execution.Balances[0]
	seed.AllowSending = false
	fixture.input.Execution.Transactions[0].BalanceRequirements = []accounting.BalanceRequirement{{
		BalanceRef: seed.BalanceRef, AssetCode: seed.AssetCode, Permission: accounting.BalancePermissionSend,
	}}
	live := *seed
	live.AllowSending = true
	live.Version = 9
	fixture.seed(t, 0, live)

	raw, err := fixture.run(t)
	require.NoError(t, err)
	result := decodeIntegrationResult(t, raw)
	require.Equal(t, "9", result.Movements[0].Before.Version)
	require.Equal(t, "10", result.Movements[0].After.Version)
}

func TestIntegrationEngineAtomicRefusals(t *testing.T) {
	if testing.Short() {
		t.Skip("requires Valkey")
	}
	container := redistestutil.SetupReusableContainer(t)
	for _, kind := range []string{"late posting", "schedule type", "recover type", "guard type", "receipt type", "protection type", "guard conflict", "orphan recovery", "prepared budget", "version overflow", "deleted balance"} {
		t.Run(kind, func(t *testing.T) {
			f := newIntegrationFixture(t, container.Client)
			f.seed(t, 0, f.input.Execution.Balances[0])
			want := "MIDAZ_ENGINE_TECH_V1 "
			switch kind {
			case "late posting":
				late := f.input.Execution.Transactions[0].Postings[0]
				late.Ref, late.Type, late.Amount = "late", accounting.PostingHold, decimal.NewFromInt(100)
				f.input.Execution.Transactions[0].Postings = append(f.input.Execution.Transactions[0].Postings, late)
				require.NoError(t, container.Client.ZAdd(context.Background(), f.resolved.Schedule, redis.Z{Score: 17, Member: "unrelated-balance"}).Err())
				for _, hash := range []string{f.resolved.Recovery, f.resolved.Receipts, f.resolved.Guards} {
					require.NoError(t, container.Client.HSet(context.Background(), hash, "unrelated", "preserve").Err())
				}
				want = `"code":"insufficient_funds"`
			case "schedule type":
				require.NoError(t, container.Client.Set(context.Background(), f.resolved.Schedule, "wrong", time.Hour).Err())
			case "recover type":
				require.NoError(t, container.Client.Set(context.Background(), f.resolved.Recovery, "wrong", time.Hour).Err())
			case "guard type":
				require.NoError(t, container.Client.Set(context.Background(), f.resolved.Guards, "wrong", time.Hour).Err())
			case "receipt type":
				require.NoError(t, container.Client.Set(context.Background(), f.resolved.Receipts, "wrong", time.Hour).Err())
			case "protection type":
				require.NoError(t, container.Client.Set(context.Background(), f.resolved.Protection, "wrong", time.Hour).Err())
			case "guard conflict":
				require.NoError(t, container.Client.HSet(context.Background(), f.resolved.Guards, f.input.Guards[0].TransactionID.String(), "other").Err())
			case "orphan recovery":
				field := f.input.Execution.Transactions[0].ID.String() + ":" + f.input.Execution.ExecutionID.String()
				require.NoError(t, container.Client.HSet(context.Background(), f.resolved.Recovery, field, "{}").Err())
				want = `"code":"execution_outcome_unknown"`
			case "prepared budget":
				f.limits.MaxPreparedBytes = 1
			case "version overflow":
				f.input.Execution.Balances[0].Version = math.MaxInt64
				f.seed(t, 0, f.input.Execution.Balances[0])
				want = `"code":"version_overflow"`
			case "deleted balance":
				require.NoError(t, container.Client.Set(context.Background(), f.resolved.Balances["@source#default"].Deleted, "1", time.Hour).Err())
				want = `"code":"balance_deleted"`
			}
			before := f.capture(t)
			_, err := f.run(t)
			require.ErrorContains(t, err, want)
			require.Equal(t, before, f.capture(t), "all values and absolute expirations must survive refusal")
		})
	}
}

func TestIntegrationEngineRejectsInvalidCompletionPlanWithoutWrites(t *testing.T) {
	if testing.Short() {
		t.Skip("requires Valkey")
	}
	container := redistestutil.SetupReusableContainer(t)
	for _, tt := range []struct {
		name    string
		payload string
	}{
		{name: "malformed_json", payload: "{"},
		{name: "scalar_json", payload: "7"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			f := newIntegrationFixture(t, container.Client)
			f.addCompanion("0")
			for i := range f.input.Execution.Balances {
				f.seed(t, i, f.input.Execution.Balances[i])
			}
			ctx := context.Background()
			require.NoError(t, container.Client.ZAdd(
				ctx, f.resolved.Schedule,
				redis.Z{Score: 17, Member: f.resolved.Balances["@source#default"].Balance},
				redis.Z{Score: 23, Member: f.resolved.Balances["@source#overdraft"].Balance},
			).Err())
			for _, key := range []string{f.resolved.Recovery, f.resolved.Receipts, f.resolved.Guards} {
				require.NoError(t, container.Client.HSet(ctx, key, "unrelated", "preserve").Err())
			}
			for _, key := range []string{
				f.resolved.Schedule, f.resolved.Recovery, f.resolved.Receipts, f.resolved.Guards,
				f.resolved.Balances["@source#default"].Balance,
				f.resolved.Balances["@source#overdraft"].Balance,
			} {
				require.True(t, container.Client.Expire(ctx, key, 45*time.Minute).Val())
			}

			raw := string(f.prepared(t).Payload)
			valid, err := json.Marshal(string(f.input.CompletionPlans[0].Payload))
			require.NoError(t, err)
			invalid, err := json.Marshal(tt.payload)
			require.NoError(t, err)
			mutated := strings.Replace(raw, `"completionPlan":`+string(valid), `"completionPlan":`+string(invalid), 1)
			require.NotEqual(t, raw, mutated)

			before := f.capture(t)
			_, err = f.runRaw(t, mutated)
			require.ErrorContains(t, err, "MIDAZ_ENGINE_TECH_V1 ")
			require.Equal(t, before, f.capture(t), "invalid completion plan must preserve every value and absolute expiration")
		})
	}
}

func TestIntegrationEngineRequiresAllSharedKeys(t *testing.T) {
	if testing.Short() {
		t.Skip("requires Valkey")
	}

	container := redistestutil.SetupReusableContainer(t)
	f := newIntegrationFixture(t, container.Client)
	prepared := f.prepared(t)
	before := f.capture(t)

	_, err := f.client.Eval(
		context.Background(), integrationEngineLua, prepared.Keys[:4], prepared.Payload,
		strconv.Itoa(f.limits.MaxRequestBytes), strconv.Itoa(f.limits.MaxPreparedBytes),
	).Result()
	require.ErrorContains(t, err, `"code":"invalid_protocol"`)
	require.Equal(t, before, f.capture(t), "an incomplete shared-key inventory must not mutate Redis")
}

func TestIntegrationEngineReplayAndInt64(t *testing.T) {
	if testing.Short() {
		t.Skip("requires Valkey")
	}
	container := redistestutil.SetupReusableContainer(t)
	f := newIntegrationFixture(t, container.Client)
	f.input.Execution.Balances[0].Version = 9007199254740993
	f.seed(t, 0, f.input.Execution.Balances[0])
	raw, err := f.run(t)
	require.NoError(t, err)
	result := decodeIntegrationResult(t, raw)
	require.Equal(t, "9007199254740994", result.Final[0].Version)
	cache, err := container.Client.Get(context.Background(), f.resolved.Balances["@source#default"].Balance).Result()
	require.NoError(t, err)
	require.Contains(t, cache, `"Version":9007199254740994`)
	require.Contains(t, cache, `"version":"9007199254740994"`)
	field := f.input.Execution.Transactions[0].ID.String() + ":" + f.input.Execution.ExecutionID.String()
	recoverRecord, err := container.Client.HGet(context.Background(), f.resolved.Recovery, field).Result()
	require.NoError(t, err)
	require.Contains(t, recoverRecord, `"version":9007199254740993`)
	require.Contains(t, recoverRecord, `"version":9007199254740994`)
	var saved struct {
		Payload string
		Result  accounting.ExecutionResult
	}
	require.NoError(t, json.Unmarshal([]byte(recoverRecord), &saved))
	require.Equal(t, int64(9007199254740994), saved.Result.Final[0].Version)
	require.Equal(t, string(f.input.CompletionPlans[0].Payload), saved.Payload)
	before := f.capture(t)
	replay, err := f.run(t)
	require.NoError(t, err)
	require.Equal(t, raw, replay)
	require.Equal(t, before, f.capture(t), "replay must not refresh TTLs or schedule scores")
	f.input.IntentFingerprint = strings.Repeat("b", 64)
	_, err = f.run(t)
	require.ErrorContains(t, err, `"code":"execution_fingerprint_conflict"`)
	require.Equal(t, before, f.capture(t))

	// An older writer advances only the legacy representation. Its odd int64
	// version and balance must win over the preserved, stale lowerCamel fields.
	cache = strings.Replace(cache, `"Available":"70"`, `"Available":"69"`, 1)
	cache = strings.Replace(cache, `"Version":9007199254740994`, `"Version":9007199254740995`, 1)
	require.NoError(t, container.Client.Set(context.Background(), f.resolved.Balances["@source#default"].Balance, cache, time.Hour).Err())
	f.input.Execution.ExecutionID = uuid.MustParse("04079f3b-b8e1-43fa-b947-7fbc13a87b76")
	f.input.Execution.Balances[0].Version = 9007199254740995
	f.input.Guards[0].ExpectedToken, f.input.Guards[0].NextToken = f.input.Guards[0].NextToken, "next-transition"
	raw, err = f.run(t)
	require.NoError(t, err)
	result = decodeIntegrationResult(t, raw)
	require.Equal(t, integrationState{"69", "0", "0", "9007199254740995"}, result.Movements[0].Before)
	require.Equal(t, integrationState{"39", "0", "0", "9007199254740996"}, finalState(result.Final[0]))
}

func TestIntegrationEngineWritesOnlyEngineRecoverHash(t *testing.T) {
	if testing.Short() {
		t.Skip("requires Valkey")
	}

	container := redistestutil.SetupReusableContainer(t)
	f := newIntegrationFixture(t, container.Client)
	legacyQueue := strings.Replace(f.resolved.Recovery, cachepolicy.EngineRecoverQueue, "backup_queue:"+cachepolicy.HashTag, 1)
	require.NotEqual(t, f.resolved.Recovery, legacyQueue)
	t.Cleanup(func() { require.NoError(t, container.Client.Del(context.Background(), legacyQueue).Err()) })
	require.NoError(t, container.Client.HSet(context.Background(), legacyQueue, "legacy-field", "legacy-payload").Err())

	_, err := f.run(t)
	require.NoError(t, err)
	field := f.input.Execution.Transactions[0].ID.String() + ":" + f.input.Execution.ExecutionID.String()
	require.True(t, container.Client.HExists(context.Background(), f.resolved.Recovery, field).Val())
	require.False(t, container.Client.HExists(context.Background(), legacyQueue, field).Val())
	require.Equal(t, map[string]string{"legacy-field": "legacy-payload"}, container.Client.HGetAll(context.Background(), legacyQueue).Val())
}

func TestIntegrationEngineDrawPolicyPrecedence(t *testing.T) {
	if testing.Short() {
		t.Skip("requires Valkey")
	}
	container := redistestutil.SetupReusableContainer(t)
	for _, tt := range []struct {
		name    string
		allowed bool
		policy  accounting.DrawPolicy
		failure string
	}{
		{"route denied eligible account", true, accounting.DrawRouteDenied, "overdraft_not_eligible"},
		{"route denied ineligible account", false, accounting.DrawRouteDenied, "insufficient_funds"},
		{"forbidden eligible account", true, accounting.DrawForbidden, "insufficient_funds"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			f := newIntegrationFixture(t, container.Client)
			f.input.Execution.Balances[0].Available = decimal.Zero
			f.input.Execution.Balances[0].AllowOverdraft = tt.allowed
			f.input.Execution.Transactions[0].Postings[0].DrawPolicy = tt.policy
			before := f.capture(t)
			_, err := f.run(t)
			require.ErrorContains(t, err, `"code":"`+tt.failure+`"`)
			require.Equal(t, before, f.capture(t))
		})
	}
}

func TestIntegrationEnginePreservesCacheExtensions(t *testing.T) {
	if testing.Short() {
		t.Skip("requires Valkey")
	}
	container := redistestutil.SetupReusableContainer(t)
	f := newIntegrationFixture(t, container.Client)
	f.seed(t, 0, f.input.Execution.Balances[0])
	key := f.resolved.Balances["@source#default"].Balance
	cached, err := container.Client.Get(context.Background(), key).Result()
	require.NoError(t, err)
	cached = strings.TrimSuffix(cached, "}") + `,"futureCounter":9223372036854775807,"futureNote":"opaque"}`
	require.NoError(t, container.Client.Set(context.Background(), key, cached, time.Hour).Err())
	_, err = f.run(t)
	require.NoError(t, err)
	updated, err := container.Client.Get(context.Background(), key).Bytes()
	require.NoError(t, err)
	var fields map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(updated, &fields))
	require.Equal(t, json.RawMessage(`9223372036854775807`), fields["futureCounter"])
	require.Equal(t, json.RawMessage(`"opaque"`), fields["futureNote"])
}

func TestIntegrationEngineCachedMoneyMatchesCodec(t *testing.T) {
	if testing.Short() {
		t.Skip("requires Valkey")
	}
	container := redistestutil.SetupReusableContainer(t)
	for _, format := range []balancecache.Format{balancecache.FormatDual, balancecache.FormatNewOnly} {
		for _, field := range []string{"Available", "OnHold", "OverdraftUsed"} {
			for _, value := range []string{"-0", "00", "01", "1.0", "0.00", "1E+1"} {
				name := strconv.Itoa(int(format)) + "/" + field + "/" + value
				t.Run(name, func(t *testing.T) {
					f := newIntegrationFixture(t, container.Client)
					late := f.input.Execution.Balances[0]
					late.ID = uuid.MustParse("646b4233-cf0e-4a6f-a9ed-e74863240866")
					late.AccountID = uuid.MustParse("e6e902c8-c7f3-471d-9853-68cf75e99a91")
					late.Alias, late.BalanceRef = "@late", "@late#default"
					f.input.Execution.Balances = append(f.input.Execution.Balances, late)
					key := strings.Replace(f.resolved.Balances["@source#default"].Balance, "@source#default", late.BalanceRef, 1)
					f.resolved.Balances[late.BalanceRef] = testResolvedBalanceKeys(key)
					primary := &f.input.Execution.Transactions[0].Postings[0]
					primary.Type, primary.Amount = accounting.PostingCredit, decimal.NewFromInt(1)
					f.input.Execution.Transactions[0].Postings = append(f.input.Execution.Transactions[0].Postings,
						accounting.Posting{Ref: "late", BalanceRef: late.BalanceRef, Type: accounting.PostingReserve, Amount: decimal.NewFromInt(1), DrawPolicy: accounting.DrawForbidden})
					encoded, err := balancecache.Encode(late, format)
					require.NoError(t, err)
					var fields map[string]json.RawMessage
					require.NoError(t, json.Unmarshal(encoded, &fields))
					name := field
					if format == balancecache.FormatNewOnly {
						name = strings.ToLower(field[:1]) + field[1:]
					}
					fields[name], err = json.Marshal(value)
					require.NoError(t, err)
					encoded, err = json.Marshal(fields)
					require.NoError(t, err)
					_, err = balancecache.Decode(encoded)
					require.Error(t, err, "codec policy is the cache reader contract")
					require.NoError(t, container.Client.Set(context.Background(), key, encoded, time.Hour).Err())
					before := f.capture(t)
					_, err = f.run(t)
					require.ErrorContains(t, err, `MIDAZ_ENGINE_TECH_V1 `)
					require.ErrorContains(t, err, `"code":"invalid_balance"`)
					require.Equal(t, before, f.capture(t), "late malformed warm money must not seed or mutate earlier balances, sidecars or expiry")
				})
			}
		}
	}
}

func TestIntegrationEngineCachedMoneyLegacyAuthority(t *testing.T) {
	if testing.Short() {
		t.Skip("requires Valkey")
	}
	container := redistestutil.SetupReusableContainer(t)
	f := newIntegrationFixture(t, container.Client)
	encoded, err := balancecache.Encode(f.input.Execution.Balances[0], balancecache.FormatDual)
	require.NoError(t, err)
	var fields map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(encoded, &fields))
	fields["available"], fields["onHold"], fields["overdraftUsed"] = json.RawMessage(`"1.0"`), json.RawMessage(`"-0"`), json.RawMessage(`"01"`)
	encoded, err = json.Marshal(fields)
	require.NoError(t, err)
	snapshot, err := balancecache.Decode(encoded)
	require.NoError(t, err)
	require.True(t, snapshot.Available.Equal(decimal.NewFromInt(100)))
	key := f.resolved.Balances["@source#default"].Balance
	require.NoError(t, container.Client.Set(context.Background(), key, encoded, time.Hour).Err())
	raw, err := f.run(t)
	require.NoError(t, err)
	result := decodeIntegrationResult(t, raw)
	require.Equal(t, "100", result.Movements[0].Before.Available)
	require.Equal(t, "70", result.Final[0].Available)
}

func TestIntegrationEngineNoncanonicalLimitStillRequiresRepair(t *testing.T) {
	if testing.Short() {
		t.Skip("requires Valkey")
	}
	container := redistestutil.SetupReusableContainer(t)
	for _, format := range []balancecache.Format{balancecache.FormatDual, balancecache.FormatNewOnly} {
		for _, limit := range []string{"1E+3", "1000.00", "-0"} {
			t.Run(strconv.Itoa(int(format))+"/"+limit, func(t *testing.T) {
				f := newIntegrationFixture(t, container.Client)
				encoded, err := balancecache.Encode(f.input.Execution.Balances[0], format)
				require.NoError(t, err)
				var fields map[string]json.RawMessage
				require.NoError(t, json.Unmarshal(encoded, &fields))
				field := "OverdraftLimit"
				if format == balancecache.FormatNewOnly {
					field = "overdraftLimit"
				}
				fields[field], err = json.Marshal(limit)
				require.NoError(t, err)
				encoded, err = json.Marshal(fields)
				require.NoError(t, err)
				_, err = balancecache.Decode(encoded)
				var normalization *balancecache.NoncanonicalLimitError
				require.ErrorAs(t, err, &normalization)
				key := f.resolved.Balances["@source#default"].Balance
				require.NoError(t, container.Client.Set(context.Background(), key, encoded, time.Hour).Err())
				before := f.capture(t)
				_, err = f.run(t)
				require.ErrorContains(t, err, "BALANCE_LIMIT_NORMALIZATION_REQUIRED:")
				require.ErrorContains(t, err, key)
				require.Equal(t, before, f.capture(t))
			})
		}
	}
}

func TestIntegrationEngineRejectsUncorrelatedReceipt(t *testing.T) {
	if testing.Short() {
		t.Skip("requires Valkey")
	}
	container := redistestutil.SetupReusableContainer(t)
	for _, kind := range []string{"transaction", "posting", "type", "balance identity", "role", "movement reference"} {
		t.Run(kind, func(t *testing.T) {
			f := newIntegrationFixture(t, container.Client)
			raw, err := f.run(t)
			require.NoError(t, err)
			response := decodeIntegrationResult(t, raw)
			switch kind {
			case "transaction":
				response.Movements[0].TransactionID = "1935edb9-c953-4f87-bea4-c98f57dff8b4"
			case "posting":
				response.Movements[0].PostingRef = "unrelated-posting"
			case "type":
				response.Movements[0].Type = "credit"
			case "balance identity":
				response.Final[0].AccountID = "1935edb9-c953-4f87-bea4-c98f57dff8b4"
			case "role":
				response.Movements[0].Role = "overdraft_companion"
			case "movement reference":
				response.Movements[0].Ref = "unrelated-movement"
			}
			encoded, err := json.Marshal(response)
			require.NoError(t, err)
			field := f.input.Execution.ExecutionID.String()
			receipt, err := container.Client.HGet(context.Background(), f.resolved.Receipts, field).Bytes()
			require.NoError(t, err)
			var envelope map[string]json.RawMessage
			require.NoError(t, json.Unmarshal(receipt, &envelope))
			envelope["response"], err = json.Marshal(string(encoded))
			require.NoError(t, err)
			receipt, err = json.Marshal(envelope)
			require.NoError(t, err)
			require.NoError(t, container.Client.HSet(context.Background(), f.resolved.Receipts, field, receipt).Err())
			before := f.capture(t)
			_, err = f.run(t)
			require.ErrorContains(t, err, `MIDAZ_ENGINE_TECH_V1 `)
			require.ErrorContains(t, err, `"code":"invalid_receipt"`)
			require.Equal(t, before, f.capture(t))
		})
	}
}

func TestIntegrationEngineMultipleTransactions(t *testing.T) {
	if testing.Short() {
		t.Skip("requires Valkey")
	}
	container := redistestutil.SetupReusableContainer(t)
	f := newIntegrationFixture(t, container.Client)
	f.input.Execution.Transactions[0].Postings[0].Amount = decimal.NewFromInt(10)
	for _, id := range []string{"1935edb9-c953-4f87-bea4-c98f57dff8b4", "770c910c-6539-471d-9e3c-e8f6346b3d76"} {
		transaction := f.input.Execution.Transactions[0]
		transaction.ID = uuid.MustParse(id)
		f.input.Execution.Transactions = append(f.input.Execution.Transactions, transaction)
		f.input.Guards = append(f.input.Guards, command.ExecutionGuard{TransactionID: transaction.ID, NextToken: "committed"})
		f.input.CompletionPlans = append(f.input.CompletionPlans, command.CompletionPlanRecord{TransactionID: transaction.ID, Payload: json.RawMessage(`{"opaque":true}`)})
	}
	raw, err := f.run(t)
	require.NoError(t, err)
	result := decodeIntegrationResult(t, raw)
	require.Len(t, result.Movements, 3)
	require.Len(t, result.Final, 1)
	require.Equal(t, integrationState{"70", "0", "0", "3"}, finalState(result.Final[0]))
	for i, wantAvailable := range []string{"90", "80", "70"} {
		field := f.input.Execution.Transactions[i].ID.String() + ":" + f.input.Execution.ExecutionID.String()
		recoverRecord, err := container.Client.HGet(context.Background(), f.resolved.Recovery, field).Bytes()
		require.NoError(t, err)
		var envelope struct {
			TransactionID string                     `json:"transactionId"`
			Result        accounting.ExecutionResult `json:"result"`
		}
		require.NoError(t, json.Unmarshal(recoverRecord, &envelope))
		require.Equal(t, f.input.Execution.Transactions[i].ID.String(), envelope.TransactionID)
		require.Len(t, envelope.Result.Final, 1)
		require.Len(t, envelope.Result.Movements, 1)
		require.Equal(t, wantAvailable, envelope.Result.Final[0].Available.String())
		require.Equal(t, int64(i+1), envelope.Result.Final[0].Version)
	}
}

func TestIntegrationEngineThirdTransactionRefusalPreservesAllState(t *testing.T) {
	if testing.Short() {
		t.Skip("requires Valkey")
	}
	container := redistestutil.SetupReusableContainer(t)
	f := newIntegrationFixture(t, container.Client)
	f.addCompanion("0")
	f.input.Execution.Transactions[0].Postings[0].Amount = decimal.NewFromInt(30)
	originalPostingRef := f.input.Execution.Transactions[0].Postings[0].Ref
	originalDrawPolicy := f.input.Execution.Transactions[0].Postings[0].DrawPolicy
	for i, id := range []string{"1935edb9-c953-4f87-bea4-c98f57dff8b4", "770c910c-6539-471d-9e3c-e8f6346b3d76"} {
		transaction := f.input.Execution.Transactions[0]
		transaction.Postings = append([]accounting.Posting(nil), transaction.Postings...)
		transaction.ID = uuid.MustParse(id)
		transaction.Postings[0].Ref = "posting-" + strconv.Itoa(i+2)
		if i == 1 {
			transaction.Postings[0].Amount = decimal.NewFromInt(50)
			transaction.Postings[0].DrawPolicy = accounting.DrawForbidden
		}
		f.input.Execution.Transactions = append(f.input.Execution.Transactions, transaction)
		f.input.Guards = append(f.input.Guards, command.ExecutionGuard{TransactionID: transaction.ID, NextToken: "committed"})
		f.input.CompletionPlans = append(f.input.CompletionPlans, command.CompletionPlanRecord{TransactionID: transaction.ID, Payload: json.RawMessage(`{"opaque":true}`)})
	}
	require.Equal(t, originalPostingRef, f.input.Execution.Transactions[0].Postings[0].Ref)
	require.Equal(t, "posting-2", f.input.Execution.Transactions[1].Postings[0].Ref)
	require.Equal(t, "posting-3", f.input.Execution.Transactions[2].Postings[0].Ref)
	require.True(t, f.input.Execution.Transactions[0].Postings[0].Amount.Equal(decimal.NewFromInt(30)))
	require.True(t, f.input.Execution.Transactions[1].Postings[0].Amount.Equal(decimal.NewFromInt(30)))
	require.True(t, f.input.Execution.Transactions[2].Postings[0].Amount.Equal(decimal.NewFromInt(50)))
	require.Equal(t, originalDrawPolicy, f.input.Execution.Transactions[0].Postings[0].DrawPolicy)
	require.Equal(t, originalDrawPolicy, f.input.Execution.Transactions[1].Postings[0].DrawPolicy)
	require.Equal(t, accounting.DrawForbidden, f.input.Execution.Transactions[2].Postings[0].DrawPolicy)
	for i := range f.input.Execution.Balances {
		f.seed(t, i, f.input.Execution.Balances[i])
	}
	ctx := context.Background()
	require.NoError(t, container.Client.ZAdd(
		ctx, f.resolved.Schedule,
		redis.Z{Score: 17, Member: f.resolved.Balances["@source#default"].Balance},
		redis.Z{Score: 23, Member: f.resolved.Balances["@source#overdraft"].Balance},
	).Err())
	for _, key := range []string{f.resolved.Recovery, f.resolved.Receipts, f.resolved.Guards} {
		require.NoError(t, container.Client.HSet(ctx, key, "unrelated", "preserve").Err())
	}
	for _, key := range []string{
		f.resolved.Schedule, f.resolved.Recovery, f.resolved.Receipts, f.resolved.Guards,
		f.resolved.Balances["@source#default"].Balance,
		f.resolved.Balances["@source#overdraft"].Balance,
	} {
		require.True(t, container.Client.Expire(ctx, key, 45*time.Minute).Val())
	}

	before := f.capture(t)
	_, err := f.run(t)
	require.ErrorContains(t, err, `"code":"insufficient_funds"`)
	require.ErrorContains(t, err, `"transactionIndex":2`)
	require.Equal(t, before, f.capture(t), "third-transaction refusal must preserve every value and absolute expiration")
}

func TestIntegrationEngineScheduleOverwritesOnlyChangedBalances(t *testing.T) {
	if testing.Short() {
		t.Skip("requires Valkey")
	}
	container := redistestutil.SetupReusableContainer(t)
	f := newIntegrationFixture(t, container.Client)
	f.addCompanion("0")
	for i := range f.input.Execution.Balances {
		f.seed(t, i, f.input.Execution.Balances[i])
	}
	changed := f.resolved.Balances["@source#default"].Balance
	untouched := f.resolved.Balances["@source#overdraft"].Balance
	ctx := context.Background()
	require.NoError(t, container.Client.ZAdd(
		ctx, f.resolved.Schedule,
		redis.Z{Score: 17, Member: changed},
		redis.Z{Score: 23, Member: untouched},
	).Err())

	_, err := f.run(t)
	require.NoError(t, err)
	changedScore, err := container.Client.ZScore(ctx, f.resolved.Schedule, changed).Result()
	require.NoError(t, err)
	untouchedScore, err := container.Client.ZScore(ctx, f.resolved.Schedule, untouched).Result()
	require.NoError(t, err)
	require.NotEqual(t, float64(17), changedScore, "changed balance must receive the current schedule score")
	require.Equal(t, float64(23), untouchedScore, "untouched balance must retain its existing schedule score")
}

func TestIntegrationEngineUnusedPoolDoesNotParticipate(t *testing.T) {
	if testing.Short() {
		t.Skip("requires Valkey")
	}
	container := redistestutil.SetupReusableContainer(t)
	f := newIntegrationFixture(t, container.Client)
	f.addCompanion("0")
	live := f.input.Execution.Balances[1]
	live.Version = 8
	f.seed(t, 1, live)
	pair := f.resolved.Balances["@source#overdraft"]
	require.NoError(t, container.Client.Set(context.Background(), pair.Deleted, "1", time.Hour).Err())
	before := f.capture(t)
	raw, err := f.run(t)
	require.NoError(t, err)
	result := decodeIntegrationResult(t, raw)
	require.Len(t, result.Final, 1)
	after := f.capture(t)
	require.Equal(t, before[pair.Balance], after[pair.Balance])
	require.Equal(t, before[pair.Deleted], after[pair.Deleted])
	members, err := container.Client.ZRange(context.Background(), f.resolved.Schedule, 0, -1).Result()
	require.NoError(t, err)
	require.Equal(t, []string{f.resolved.Balances["@source#default"].Balance}, members)
}

func TestIntegrationEngineRejectsBothDeletionMarkerNamespaces(t *testing.T) {
	if testing.Short() {
		t.Skip("requires Valkey")
	}

	container := redistestutil.SetupReusableContainer(t)
	for _, test := range []struct {
		name   string
		marker func(resolvedBalanceKeys) string
	}{
		{name: "current", marker: func(keys resolvedBalanceKeys) string { return keys.Deleted }},
		{name: "legacy", marker: func(keys resolvedBalanceKeys) string { return keys.LegacyDeleted }},
	} {
		t.Run(test.name, func(t *testing.T) {
			fixture := newIntegrationFixture(t, container.Client)
			fixture.input.Execution.Transactions[0].BalanceRequirements = []accounting.BalanceRequirement{{
				BalanceRef: "@source#default", AssetCode: "USD", Permission: accounting.BalancePermissionSend,
			}}
			marker := test.marker(fixture.resolved.Balances["@source#default"])
			require.NoError(t, container.Client.Set(context.Background(), marker, "1", time.Hour).Err())
			before := fixture.capture(t)

			_, err := fixture.run(t)
			require.ErrorContains(t, err, `"code":"balance_deleted"`)
			require.Equal(t, before, fixture.capture(t), "deletion refusal must not mutate any key")
		})
	}
}

func TestIntegrationEngineRejectsMalformedProtocol(t *testing.T) {
	if testing.Short() {
		t.Skip("requires Valkey")
	}
	container := redistestutil.SetupReusableContainer(t)
	for _, kind := range []string{"duplicate key", "trailing JSON", "array as object", "postings as object", "unknown posting", "unknown balance", "noncanonical amount", "malformed cache", "duplicate cached version"} {
		t.Run(kind, func(t *testing.T) {
			f := newIntegrationFixture(t, container.Client)
			raw := string(f.prepared(t).Payload)
			switch kind {
			case "duplicate key":
				raw = strings.Replace(raw, `"protocolVersion":1`, `"protocolVersion":1,"protocolVersion":1`, 1)
			case "trailing JSON":
				raw += `{}`
			case "array as object", "postings as object":
				var request map[string]any
				require.NoError(t, json.Unmarshal([]byte(raw), &request))
				if kind == "array as object" {
					request["balances"] = map[string]any{}
				} else {
					request["transactions"].([]any)[0].(map[string]any)["postings"] = map[string]any{}
				}
				encoded, err := json.Marshal(request)
				require.NoError(t, err)
				raw = string(encoded)
			case "unknown posting":
				raw = strings.Replace(raw, `"type":"debit"`, `"type":"unknown"`, 1)
			case "unknown balance":
				raw = strings.Replace(raw, `"balanceRef":"@source#default"`, `"balanceRef":"@missing#default"`, 1)
			case "noncanonical amount":
				raw = strings.Replace(raw, `"amount":"30"`, `"amount":"3e1"`, 1)
			case "malformed cache", "duplicate cached version":
				cache := `{"Version":01}`
				if kind == "duplicate cached version" {
					cache = `{"Version":1,"Version":1}`
				}
				require.NoError(t, container.Client.Set(context.Background(), f.resolved.Balances["@source#default"].Balance, cache, time.Hour).Err())
			}
			before := f.capture(t)
			_, err := f.runRaw(t, raw)
			require.ErrorContains(t, err, "MIDAZ_ENGINE_TECH_V1 ")
			require.Equal(t, before, f.capture(t))
		})
	}
}
