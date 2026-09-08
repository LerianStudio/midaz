// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package engine

import (
	"context"
	"encoding/json"
	"math"
	"reflect"
	"strconv"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/engine"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/services/command"
)

func TestPrepareExecutionDeterministicLosslessWire(t *testing.T) {
	t.Parallel()

	input, limits, resolved := validWireExecution()
	original, err := json.Marshal(input)
	require.NoError(t, err)
	first, err := prepareExecution(context.Background(), input, limits, resolved)
	require.NoError(t, err)
	second, err := prepareExecution(context.Background(), input, limits, resolved)
	require.NoError(t, err)
	require.Equal(t, first, second)
	after, err := json.Marshal(input)
	require.NoError(t, err)
	require.Equal(t, original, after, "preparation must not mutate domain input")

	var wire wireRequest
	require.NoError(t, json.Unmarshal(first.Payload, &wire))
	require.Equal(t, 1, wire.ProtocolVersion)
	require.Equal(t, resolved.TenantID, wire.TenantID)
	require.Equal(t, input.Request.OrganizationID.String(), wire.OrganizationID)
	require.Equal(t, input.Request.LedgerID.String(), wire.LedgerID)
	require.Equal(t, input.Request.ExecutionID.String(), wire.ExecutionID)
	require.Equal(t, input.IntentFingerprint, wire.IntentFingerprint)
	require.Equal(t, []string{
		resolved.Schedule, resolved.Recovery, resolved.Receipts, resolved.Guards,
		resolved.Balances["@source#default"].Balance, resolved.Balances["@source#default"].Deleted,
	}, first.Keys)
	require.Equal(t, 1, wire.ScheduleKeyIndex)
	require.Equal(t, 2, wire.RecoveryKeyIndex)
	require.Equal(t, 3, wire.ReceiptKeyIndex)
	require.Equal(t, 4, wire.GuardKeyIndex)
	require.Len(t, wire.Balances, 1)
	require.Equal(t, 5, wire.Balances[0].KeyIndex)
	require.Equal(t, 6, wire.Balances[0].DeleteKeyIndex)
	require.Equal(t, "9223372036854775807", wire.Balances[0].Snapshot.Version)
	require.Equal(t, "12345678901234567890.1234567890123456789", wire.Balances[0].Snapshot.Available)
	require.Equal(t, "0.0000000000000000001", wire.Transactions[0].Postings[0].Amount)
	require.Equal(t, "0", wire.Transactions[0].Postings[0].OverdraftAmount)
	require.Equal(t, string(input.Recovery[0].Payload), wire.Transactions[0].RecoveryPayload)
	require.Equal(t, input.Request.Transactions[0].ID.String()+":"+input.Request.ExecutionID.String(), wire.Transactions[0].RecoveryField)
	require.Equal(t, input.Request.Transactions[0].ID.String(), wire.Transactions[0].GuardField)
	require.Equal(t, input.Request.ExecutionID.String(), wire.ReceiptField)
	for _, key := range first.Keys {
		require.NotContains(t, string(first.Payload), key, "physical keys belong exclusively in KEYS")
	}
	input.Recovery[0].Payload[0] = '['
	input.Request.Transactions[0].Postings[0].Ref = "changed"
	require.Equal(t, first.Payload, second.Payload, "prepared bytes must not alias input storage")
}

func TestPrepareExecutionPreservesTransactionAndSnapshotOrder(t *testing.T) {
	t.Parallel()

	input, limits, resolved := validWireExecution()
	secondID := uuid.MustParse("1935edb9-c953-4f87-bea4-c98f57dff8b4")
	second := input.Request.Transactions[0]
	second.ID = secondID
	input.Request.Transactions = append(input.Request.Transactions, second)
	input.Guards = append([]command.ExecutionGuard{{TransactionID: secondID, NextToken: "next-two"}}, input.Guards...)
	input.Recovery = append([]command.RecoveryIntent{{TransactionID: secondID, Payload: json.RawMessage(`{"value":"two"}`)}}, input.Recovery...)
	prepared, err := prepareExecution(context.Background(), input, limits, resolved)
	require.NoError(t, err)
	var wire wireRequest
	require.NoError(t, json.Unmarshal(prepared.Payload, &wire))
	require.Equal(t, input.Request.Transactions[0].ID.String(), wire.Transactions[0].ID)
	require.Equal(t, secondID.String(), wire.Transactions[1].ID)
	require.Equal(t, "next-two", wire.Transactions[1].NextGuard)
	require.Equal(t, `{"value":"two"}`, wire.Transactions[1].RecoveryPayload)
	require.Len(t, wire.Balances, 1)
}

func TestPrepareExecutionRejectsInvalidInputs(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		mutate func(*command.EngineExecution, *Limits, *resolvedExecutionKeys)
	}{
		{"zero scope", func(x *command.EngineExecution, _ *Limits, _ *resolvedExecutionKeys) { x.Request.LedgerID = uuid.Nil }},
		{"zero execution", func(x *command.EngineExecution, _ *Limits, _ *resolvedExecutionKeys) {
			x.Request.ExecutionID = uuid.Nil
		}},
		{"empty fingerprint", func(x *command.EngineExecution, _ *Limits, _ *resolvedExecutionKeys) { x.IntentFingerprint = " " }},
		{"zero limits", func(_ *command.EngineExecution, l *Limits, _ *resolvedExecutionKeys) { l.MaxBalances = 0 }},
		{"request bytes", func(_ *command.EngineExecution, l *Limits, _ *resolvedExecutionKeys) { l.MaxRequestBytes = 600 }},
		{"recovery bytes", func(_ *command.EngineExecution, l *Limits, _ *resolvedExecutionKeys) { l.MaxRecoveryBytes = 1 }},
		{"missing guard", func(x *command.EngineExecution, _ *Limits, _ *resolvedExecutionKeys) { x.Guards = nil }},
		{"unrelated guard", func(x *command.EngineExecution, _ *Limits, _ *resolvedExecutionKeys) {
			x.Guards[0].TransactionID = x.Request.ExecutionID
		}},
		{"unchanged guard", func(x *command.EngineExecution, _ *Limits, _ *resolvedExecutionKeys) {
			x.Guards[0].ExpectedToken = x.Guards[0].NextToken
		}},
		{"empty next guard", func(x *command.EngineExecution, _ *Limits, _ *resolvedExecutionKeys) { x.Guards[0].NextToken = "" }},
		{"missing recovery", func(x *command.EngineExecution, _ *Limits, _ *resolvedExecutionKeys) { x.Recovery = nil }},
		{"unrelated recovery", func(x *command.EngineExecution, _ *Limits, _ *resolvedExecutionKeys) {
			x.Recovery[0].TransactionID = x.Request.ExecutionID
		}},
		{"invalid recovery JSON", func(x *command.EngineExecution, _ *Limits, _ *resolvedExecutionKeys) {
			x.Recovery[0].Payload = json.RawMessage(`{"broken":}`)
		}},
		{"recovery array", func(x *command.EngineExecution, _ *Limits, _ *resolvedExecutionKeys) {
			x.Recovery[0].Payload = json.RawMessage(`[]`)
		}},
		{"recovery null", func(x *command.EngineExecution, _ *Limits, _ *resolvedExecutionKeys) {
			x.Recovery[0].Payload = json.RawMessage(`null`)
		}},
		{"zero amount", func(x *command.EngineExecution, _ *Limits, _ *resolvedExecutionKeys) {
			x.Request.Transactions[0].Postings[0].Amount = decimal.Zero
		}},
		{"negative override", func(x *command.EngineExecution, _ *Limits, _ *resolvedExecutionKeys) {
			x.Request.Transactions[0].Postings[0].OverdraftAmount = decimal.NewFromInt(-1)
		}},
		{"huge positive exponent", func(x *command.EngineExecution, _ *Limits, _ *resolvedExecutionKeys) {
			x.Request.Transactions[0].Postings[0].Amount = decimal.New(1, math.MaxInt32)
		}},
		{"huge negative exponent", func(x *command.EngineExecution, _ *Limits, _ *resolvedExecutionKeys) {
			x.Request.Transactions[0].Postings[0].Amount = decimal.New(1, math.MinInt32)
		}},
		{"unknown posting", func(x *command.EngineExecution, _ *Limits, _ *resolvedExecutionKeys) {
			x.Request.Transactions[0].Postings[0].Type = "unknown"
		}},
		{"unknown policy", func(x *command.EngineExecution, _ *Limits, _ *resolvedExecutionKeys) {
			x.Request.Transactions[0].Postings[0].DrawPolicy = "unknown"
		}},
		{"empty posting reference", func(x *command.EngineExecution, _ *Limits, _ *resolvedExecutionKeys) {
			x.Request.Transactions[0].Postings[0].Ref = ""
		}},
		{"duplicate posting reference", func(x *command.EngineExecution, _ *Limits, _ *resolvedExecutionKeys) {
			x.Request.Transactions[0].Postings = append(x.Request.Transactions[0].Postings, x.Request.Transactions[0].Postings[0])
		}},
		{"unknown balance", func(x *command.EngineExecution, _ *Limits, _ *resolvedExecutionKeys) {
			x.Request.Transactions[0].Postings[0].BalanceRef = "@missing#default"
		}},
		{"empty postings", func(x *command.EngineExecution, _ *Limits, _ *resolvedExecutionKeys) {
			x.Request.Transactions[0].Postings = nil
		}},
		{"posting count", func(x *command.EngineExecution, l *Limits, _ *resolvedExecutionKeys) {
			second := x.Request.Transactions[0].Postings[0]
			second.Ref = "second"
			x.Request.Transactions[0].Postings = append(x.Request.Transactions[0].Postings, second)
			l.MaxPostings = 1
		}},
		{"unknown direction", func(x *command.EngineExecution, _ *Limits, _ *resolvedExecutionKeys) {
			x.Request.Balances[0].Direction = "unknown"
		}},
		{"negative version", func(x *command.EngineExecution, _ *Limits, _ *resolvedExecutionKeys) {
			x.Request.Balances[0].Version = -1
		}},
		{"negative onhold", func(x *command.EngineExecution, _ *Limits, _ *resolvedExecutionKeys) {
			x.Request.Balances[0].OnHold = decimal.NewFromInt(-1)
		}},
		{"negative debt", func(x *command.EngineExecution, _ *Limits, _ *resolvedExecutionKeys) {
			x.Request.Balances[0].OverdraftUsed = decimal.NewFromInt(-1)
		}},
		{"negative limit", func(x *command.EngineExecution, _ *Limits, _ *resolvedExecutionKeys) {
			x.Request.Balances[0].OverdraftLimit = decimal.NewFromInt(-1)
		}},
		{"negative nonexternal", func(x *command.EngineExecution, _ *Limits, _ *resolvedExecutionKeys) {
			x.Request.Balances[0].Available = decimal.NewFromInt(-1)
		}},
		{"unknown balance scope", func(x *command.EngineExecution, _ *Limits, _ *resolvedExecutionKeys) {
			x.Request.Balances[0].BalanceScope = "unknown"
		}},
		{"inconsistent alias", func(x *command.EngineExecution, _ *Limits, _ *resolvedExecutionKeys) {
			x.Request.Balances[0].Alias = "@different"
		}},
		{"zero balance identity", func(x *command.EngineExecution, _ *Limits, _ *resolvedExecutionKeys) {
			x.Request.Balances[0].ID = uuid.Nil
		}},
		{"physical reference", func(x *command.EngineExecution, _ *Limits, k *resolvedExecutionKeys) {
			old := x.Request.Balances[0].BalanceRef
			x.Request.Balances[0].Alias = "balance:{transactions}:@source"
			x.Request.Balances[0].BalanceRef = x.Request.Balances[0].Alias + "#default"
			k.Balances[x.Request.Balances[0].BalanceRef] = k.Balances[old]
			delete(k.Balances, old)
		}},
		{"missing resolved keys", func(_ *command.EngineExecution, _ *Limits, k *resolvedExecutionKeys) { k.Balances = nil }},
		{"wrong hash slot", func(_ *command.EngineExecution, _ *Limits, k *resolvedExecutionKeys) {
			k.Schedule = "schedule:{other}:sync"
		}},
		{"duplicate resolved key", func(_ *command.EngineExecution, _ *Limits, k *resolvedExecutionKeys) { k.Schedule = k.Recovery }},
		{"unrelated marker", func(_ *command.EngineExecution, _ *Limits, k *resolvedExecutionKeys) {
			pair := k.Balances["@source#default"]
			pair.Deleted += ":other"
			k.Balances["@source#default"] = pair
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			input, limits, resolved := validWireExecution()
			tt.mutate(&input, &limits, &resolved)
			prepared, err := prepareExecution(context.Background(), input, limits, resolved)
			require.Error(t, err)
			require.Nil(t, prepared)
			var failure *engine.Failure
			require.NotErrorAs(t, err, &failure, "invalid input is not a financial refusal")
		})
	}
}

func TestPrepareExecutionCanceledContext(t *testing.T) {
	t.Parallel()

	input, limits, resolved := validWireExecution()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	prepared, err := prepareExecution(ctx, input, limits, resolved)
	require.ErrorIs(t, err, context.Canceled)
	require.Nil(t, prepared)
}

func TestBoundedDecimal(t *testing.T) {
	t.Parallel()

	for _, text := range []string{"0", "-0", "1.2300", "1e3", "0.00001", "-15.5"} {
		t.Run(text, func(t *testing.T) {
			value := decimal.RequireFromString(text)
			got, err := boundedDecimal(value, 100)
			require.NoError(t, err)
			require.Equal(t, value.String(), got)
		})
	}
	got, err := boundedDecimal(decimal.New(0, math.MaxInt32), 1)
	require.NoError(t, err)
	require.Equal(t, "0", got)
}

func validWireExecution() (command.EngineExecution, Limits, resolvedExecutionKeys) {
	organizationID := uuid.MustParse("ef73c171-f889-4a9d-a5d3-421ee069d736")
	ledgerID := uuid.MustParse("fbe630e6-fde0-4396-bb3b-b5e5da509ae0")
	executionID := uuid.MustParse("f00b8ce5-fad0-45da-b4a8-0bcaec270b08")
	transactionID := uuid.MustParse("53d2c279-4d51-4274-8a3d-ee1955ddcaa0")
	input := command.EngineExecution{
		Request: engine.Request{
			OrganizationID: organizationID, LedgerID: ledgerID, ExecutionID: executionID,
			Transactions: []engine.Transaction{{ID: transactionID, Postings: []engine.Posting{{Ref: "debit-0", BalanceRef: "@source#default", Type: engine.PostingDebit, Amount: decimal.RequireFromString("0.0000000000000000001"), DrawPolicy: engine.DrawAllowed, OverdraftAmount: decimal.Zero}}}},
			Balances:     []engine.BalanceSnapshot{{BalanceRef: "@source#default", ID: uuid.MustParse("d9e5a2be-9128-43ab-9d3c-0c14e3a8b889"), AccountID: uuid.MustParse("1f3da5d4-1571-4619-938f-1aef51560726"), AccountType: "deposit", AssetCode: "USD", Alias: "@source", Key: "default", Direction: "credit", BalanceScope: "transactional", Available: decimal.RequireFromString("12345678901234567890.1234567890123456789"), OnHold: decimal.Zero, OverdraftUsed: decimal.Zero, OverdraftLimit: decimal.NewFromInt(1000), Version: math.MaxInt64, AllowSending: true, AllowReceiving: true}},
		},
		IntentFingerprint: "immutable-intent",
		Guards:            []command.ExecutionGuard{{TransactionID: transactionID, NextToken: "pending-token"}},
		Recovery:          []command.RecoveryIntent{{TransactionID: transactionID, Payload: json.RawMessage("{\n  \"version\": 9223372036854775807, \"amount\": \"123456789.123456789\"\n}")}},
	}
	prefix := "tenant:fixture:"
	scope := organizationID.String() + ":" + ledgerID.String()
	balanceKey := prefix + "balance:{transactions}:" + scope + ":@source#default"
	resolved := resolvedExecutionKeys{TenantID: "fixture", Schedule: prefix + "schedule:{transactions}:balance-sync-v2", Recovery: prefix + "backup_queue:{transactions}", Receipts: prefix + "engine:{transactions}:receipts:" + scope, Guards: prefix + "engine:{transactions}:guards:" + scope, Balances: map[string]resolvedBalanceKeys{"@source#default": {Balance: balanceKey, Deleted: balanceKey + ":deleted"}}}
	return input, Limits{MaxTransactions: 10, MaxPostings: 100, MaxBalances: 100, MaxRecoveryBytes: 4096, MaxRequestBytes: 16384, MaxPreparedBytes: 1048576}, resolved
}

func TestWireArraysAreNotNull(t *testing.T) {
	t.Parallel()

	input, limits, resolved := validWireExecution()
	prepared, err := prepareExecution(context.Background(), input, limits, resolved)
	require.NoError(t, err)
	require.False(t, strings.Contains(string(prepared.Payload), ":null"))
	var wire wireRequest
	require.NoError(t, json.Unmarshal(prepared.Payload, &wire))
	require.False(t, reflect.ValueOf(wire.Transactions).IsNil())
	require.False(t, reflect.ValueOf(wire.Balances).IsNil())
	require.False(t, reflect.ValueOf(wire.Transactions[0].Postings).IsNil())
}

// TestPreparedExecutionMeasurements keeps the engine bound below the largest
// currently accepted HTTP body. The wire is deterministic, so these are
// contract measurements rather than machine-dependent timing observations.
// The pool is intentionally measured separately from touched postings: v2
// permits a scoped pool larger than the legs, and the pool must not be treated
// as user-requested cardinality.
func TestPreparedExecutionMeasurements(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		postings   int
		pool       int
		wantBytes  int
		maxTouched int
	}{
		{name: "two postings", postings: 2, pool: 2, wantBytes: 2088, maxTouched: 2},
		{name: "ten postings", postings: 10, pool: 20, wantBytes: 12927, maxTouched: 10},
		{name: "fifty postings", postings: 50, pool: 100, wantBytes: 61912, maxTouched: 50},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			input, limits, resolved := measuredWireExecution(test.postings, test.pool)
			limits.MaxPostings, limits.MaxBalances, limits.MaxRequestBytes = 500, 500, 4*1024*1024
			prepared, err := prepareExecution(context.Background(), input, limits, resolved)
			require.NoError(t, err)
			require.Equal(t, test.wantBytes, len(prepared.Payload))
			require.LessOrEqual(t, test.postings, test.maxTouched)
			require.Equal(t, test.pool, len(input.Request.Balances))
			t.Logf("postings=%d full_pool=%d touched=%d request_bytes=%d", test.postings, test.pool, test.postings, len(prepared.Payload))
		})
	}
}

func measuredWireExecution(postingCount, poolCount int) (command.EngineExecution, Limits, resolvedExecutionKeys) {
	input, limits, resolved := validWireExecution()
	baseBalance := input.Request.Balances[0]
	input.Request.Balances = make([]engine.BalanceSnapshot, poolCount)
	resolved.Balances = make(map[string]resolvedBalanceKeys, poolCount)
	for i := 0; i < poolCount; i++ {
		index := strconv.Itoa(i)
		balance := baseBalance
		balance.ID = uuid.NewSHA1(uuid.NameSpaceOID, []byte("measurement-balance:"+index))
		balance.AccountID = uuid.NewSHA1(uuid.NameSpaceOID, []byte("measurement-account:"+index))
		balance.Alias = "@source-" + index
		balance.BalanceRef = balance.Alias + "#default"
		input.Request.Balances[i] = balance
		key := "tenant:fixture:balance:{transactions}:measurement:" + index
		resolved.Balances[balance.BalanceRef] = resolvedBalanceKeys{Balance: key, Deleted: key + ":deleted"}
	}
	input.Request.Transactions[0].Postings = make([]engine.Posting, postingCount)
	for i := 0; i < postingCount; i++ {
		input.Request.Transactions[0].Postings[i] = engine.Posting{Ref: "debit-" + strconv.Itoa(i), BalanceRef: input.Request.Balances[i].BalanceRef, Type: engine.PostingDebit, Amount: decimal.NewFromInt(1), DrawPolicy: engine.DrawAllowed}
	}
	return input, limits, resolved
}
