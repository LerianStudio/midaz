//go:build integration

// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package engine

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/redis/balancecache"
	core "github.com/LerianStudio/midaz/v4/components/ledger/internal/domain/accounting"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/services/command"
)

func TestIntegration_AdapterExecute_MultiTransactionAcceptance(t *testing.T) {
	ctx := context.Background()
	inspector, _, _ := newAdapterValkey(t)

	t.Run("ordered shared balance execution, recovery, and replay", func(t *testing.T) {
		input, limits := multiTransactionAcceptanceExecution(t)
		adapter, err := NewAdapter(&integrationClientProvider{client: inspector}, limits)
		require.NoError(t, err)

		result, err := adapter.Execute(ctx, input)
		require.NoError(t, err)
		requireJSONEqual(t, multiTransactionAcceptanceResult(), result)

		keys, err := resolveAdapterKeys(ctx, input.Execution)
		require.NoError(t, err)
		t.Cleanup(func() { deleteMultiTransactionAcceptanceState(t, inspector, keys) })
		assertMultiTransactionAcceptanceState(t, inspector, keys, input)

		committed := captureAdapterState(t, inspector, keys)
		replayed, err := adapter.Execute(ctx, input)
		require.NoError(t, err)
		requireJSONEqual(t, multiTransactionAcceptanceResult(), replayed)
		require.Equal(t, committed, captureAdapterState(t, inspector, keys), "whole-execution replay must not apply money or refresh expirations")

		conflict := refingerprintMultiTransactionAcceptance(t, input, "different immutable intent")
		refused, err := adapter.Execute(ctx, conflict)
		require.Nil(t, refused)
		require.ErrorContains(t, err, `"code":"execution_fingerprint_conflict"`)
		require.Equal(t, committed, captureAdapterState(t, inspector, keys), "a different intent under the same execution id must not mutate state")
	})

	t.Run("live physical balance overrides the cache-aside seed", func(t *testing.T) {
		input, limits := multiTransactionAcceptanceExecution(t)
		keys, err := resolveAdapterKeys(ctx, input.Execution)
		require.NoError(t, err)
		live := input.Execution.Balances[0]
		live.Available = decimal.NewFromInt(41)
		live.Version = 8
		encoded, err := balancecache.Encode(live, balancecache.FormatDual)
		require.NoError(t, err)
		require.NoError(t, inspector.Set(ctx, keys.Balances[live.BalanceRef].Balance, encoded, 0).Err())
		adapter, err := NewAdapter(&integrationClientProvider{client: inspector}, limits)
		require.NoError(t, err)
		result, err := adapter.Execute(ctx, input)
		require.NoError(t, err)
		require.NotNil(t, result)
		require.Equal(t, int64(8), result.Movements[0].Before.Version)
		require.Equal(t, "41", result.Movements[0].Before.Available.String())
		final := liveStateCompositionSnapshot(t, result.Final, "@source#default")
		require.Equal(t, int64(12), final.Version)
		require.Equal(t, "11", final.OverdraftUsed.String())
	})
}

func multiTransactionAcceptanceExecution(t *testing.T) (command.EngineExecution, Limits) {
	t.Helper()
	input, limits := richAdapterExecution(t)
	request := &input.Execution
	request.ExecutionID = uuid.MustParse("33333333-3333-4333-8333-333333333333")

	primary := &request.Balances[0]
	primary.Available = decimal.NewFromInt(40)
	primary.OnHold = decimal.Zero
	primary.OverdraftUsed = decimal.Zero
	primary.OverdraftLimit = decimal.NewFromInt(100)
	primary.Version = 7
	primary.AllowOverdraft = true
	primary.OverdraftLimitEnabled = true

	companion := *primary
	companion.BalanceRef = "@source#overdraft"
	companion.ID = uuid.MustParse("22222222-2222-4222-8222-222222222222")
	companion.Key = "overdraft"
	companion.Direction = "debit"
	companion.BalanceScope = "internal"
	companion.Available = decimal.Zero
	companion.OverdraftLimit = decimal.Zero
	companion.Version = 3
	companion.AllowOverdraft = false
	companion.OverdraftLimitEnabled = false
	request.Balances = []core.BalanceSnapshot{*primary, companion}

	t1 := core.Transaction{ID: uuid.MustParse("11111111-1111-4111-8111-111111111111"), Postings: []core.Posting{
		{Ref: "t1-debit-1", BalanceRef: primary.BalanceRef, Type: core.PostingDebit, Amount: decimal.NewFromInt(30), DrawPolicy: core.DrawAllowed},
		{Ref: "t1-debit-2", BalanceRef: primary.BalanceRef, Type: core.PostingDebit, Amount: decimal.NewFromInt(20), DrawPolicy: core.DrawAllowed},
	}}
	t2 := core.Transaction{ID: uuid.MustParse("12121212-1212-4212-8212-121212121212"), Postings: []core.Posting{
		{Ref: "t2-credit", BalanceRef: primary.BalanceRef, Type: core.PostingCredit, Amount: decimal.NewFromInt(6), DrawPolicy: core.DrawForbidden},
	}}
	t3 := core.Transaction{ID: uuid.MustParse("13131313-1313-4313-8313-131313131313"), Postings: []core.Posting{
		{Ref: "t3-debit", BalanceRef: primary.BalanceRef, Type: core.PostingDebit, Amount: decimal.NewFromInt(8), DrawPolicy: core.DrawAllowed},
	}}
	request.Transactions = []core.Transaction{t1, t2, t3}
	input.Guards = []command.ExecutionGuard{
		{TransactionID: t1.ID, NextToken: "accepted-t1"},
		{TransactionID: t2.ID, NextToken: "accepted-t2"},
		{TransactionID: t3.ID, NextToken: "accepted-t3"},
	}

	base, err := command.DecodeTransactionCompletionPlan(input.CompletionPlans[0].Payload)
	require.NoError(t, err)
	base.ExecutionID = request.ExecutionID
	primaryProjection := func(transactionID uuid.UUID, posting core.Posting, ordinal uint32) command.OperationRecordSpec {
		projection := base.OperationSpecs[0]
		projection.TransactionID = transactionID
		projection.PostingRef = posting.Ref
		projection.Ordinal = ordinal
		projection.BalanceRef = primary.BalanceRef
		projection.Balance.Available = primary.Available
		projection.Balance.Version = primary.Version
		projection.RequestedAmount = posting.Amount
		if posting.Type == core.PostingCredit {
			projection.RowType = "CREDIT"
			projection.Direction = "debit"
		}
		return projection
	}
	companionProjection := func(primaryContext command.OperationRecordSpec) command.OperationRecordSpec {
		projection := primaryContext
		projection.Role = core.RoleOverdraftCompanion
		projection.BalanceRef = companion.BalanceRef
		projection.RowType = "OVERDRAFT"
		projection.Metadata = nil
		projection.ChartOfAccounts = ""
		projection.Balance.ID = companion.ID.String()
		projection.Balance.Key = companion.Key
		projection.Balance.Direction = companion.Direction
		projection.Balance.Available = companion.Available
		projection.Balance.Version = companion.Version
		return projection
	}

	p11 := primaryProjection(t1.ID, t1.Postings[0], 0)
	p12 := primaryProjection(t1.ID, t1.Postings[1], 0)
	p2 := primaryProjection(t2.ID, t2.Postings[0], 0)
	p3 := primaryProjection(t3.ID, t3.Postings[0], 0)
	projections := [][]command.OperationRecordSpec{
		{p11, p12, companionProjection(p12)},
		{p2, companionProjection(p2)},
		{p3, companionProjection(p3)},
	}

	payloads := make([]command.TransactionCompletionPlan, len(request.Transactions))
	intents := make([]command.BalanceEngineTransactionIntent, len(request.Transactions))
	for index, transaction := range request.Transactions {
		payloads[index] = *base
		payloads[index].TransactionID = transaction.ID
		payloads[index].OperationSpecs = projections[index]
		postingRefs := make([]string, len(transaction.Postings))
		for postingIndex, posting := range transaction.Postings {
			postingRefs[postingIndex] = posting.Ref
		}
		projectionIntents := make([]command.OperationRecordIntent, len(projections[index]))
		for projectionIndex, projection := range projections[index] {
			projectionIntents[projectionIndex] = projection.Intent()
		}
		intents[index] = command.BalanceEngineTransactionIntent{
			TransactionID: transaction.ID, Action: base.Action, TransactionStatus: base.TransactionStatus,
			TransactionDate: base.TransactionDate, TransactionCreatedAt: base.TransactionCreatedAt,
			TransactionUpdatedAt: base.TransactionUpdatedAt, OperationUpdatedAt: base.OperationUpdatedAt,
			Input: base.TransactionInput, PostingRefs: postingRefs, OperationSpecs: projectionIntents,
		}
	}
	fingerprint, err := command.ComputeBalanceEngineIntentFingerprint(command.BalanceEngineIntent{
		TenantID: base.TenantID, OrganizationID: request.OrganizationID, LedgerID: request.LedgerID,
		ExecutionID: request.ExecutionID, Transactions: intents,
	})
	require.NoError(t, err)
	input.IntentFingerprint = fingerprint
	input.CompletionPlans = make([]command.CompletionPlanRecord, len(payloads))
	for index := range payloads {
		payloads[index].IntentFingerprint = fingerprint
		encoded, encodeErr := command.EncodeTransactionCompletionPlan(payloads[index])
		require.NoError(t, encodeErr)
		input.CompletionPlans[index] = command.CompletionPlanRecord{TransactionID: payloads[index].TransactionID, Payload: encoded}
	}
	require.NoError(t, command.ValidateTransactionCompletion(input))
	return input, limits
}

func refingerprintMultiTransactionAcceptance(t *testing.T, input command.EngineExecution, description string) command.EngineExecution {
	t.Helper()
	payloads := make([]command.TransactionCompletionPlan, len(input.CompletionPlans))
	intents := make([]command.BalanceEngineTransactionIntent, len(input.CompletionPlans))
	for index, recovery := range input.CompletionPlans {
		payload, err := command.DecodeTransactionCompletionPlan(recovery.Payload)
		require.NoError(t, err)
		payload.OperationSpecs[0].Description = description
		payloads[index] = *payload
		postingRefs := make([]string, len(input.Execution.Transactions[index].Postings))
		for postingIndex, posting := range input.Execution.Transactions[index].Postings {
			postingRefs[postingIndex] = posting.Ref
		}
		projectionIntents := make([]command.OperationRecordIntent, len(payload.OperationSpecs))
		for projectionIndex, projection := range payload.OperationSpecs {
			projectionIntents[projectionIndex] = projection.Intent()
		}
		intents[index] = command.BalanceEngineTransactionIntent{
			TransactionID: payload.TransactionID, Action: payload.Action, TransactionStatus: payload.TransactionStatus,
			TransactionDate: payload.TransactionDate, TransactionCreatedAt: payload.TransactionCreatedAt,
			TransactionUpdatedAt: payload.TransactionUpdatedAt, OperationUpdatedAt: payload.OperationUpdatedAt,
			Input: payload.TransactionInput, PostingRefs: postingRefs, OperationSpecs: projectionIntents,
		}
	}
	fingerprint, err := command.ComputeBalanceEngineIntentFingerprint(command.BalanceEngineIntent{
		TenantID: payloads[0].TenantID, OrganizationID: input.Execution.OrganizationID, LedgerID: input.Execution.LedgerID,
		ExecutionID: input.Execution.ExecutionID, Transactions: intents,
	})
	require.NoError(t, err)
	input.IntentFingerprint = fingerprint
	for index := range payloads {
		payloads[index].IntentFingerprint = fingerprint
		encoded, encodeErr := command.EncodeTransactionCompletionPlan(payloads[index])
		require.NoError(t, encodeErr)
		input.CompletionPlans[index].Payload = encoded
	}
	require.NoError(t, command.ValidateTransactionCompletion(input))
	return input
}

func multiTransactionAcceptanceResult() *core.ExecutionResult {
	primary := multiTransactionAcceptancePrimary()
	companion := multiTransactionAcceptanceCompanion()
	t1 := uuid.MustParse("11111111-1111-4111-8111-111111111111")
	t2 := uuid.MustParse("12121212-1212-4212-8212-121212121212")
	t3 := uuid.MustParse("13131313-1313-4313-8313-131313131313")
	state := func(available, debt int64, version int64) core.BalanceState {
		return core.BalanceState{Available: decimal.NewFromInt(available), OverdraftUsed: decimal.NewFromInt(debt), Version: version}
	}
	movements := []core.Movement{
		{Ref: "11111111-1111-4111-8111-111111111111:10:t1-debit-1:primary:0", TransactionID: t1, PostingRef: "t1-debit-1", Role: core.RolePrimary, BalanceRef: primary.BalanceRef, Type: core.PostingDebit, Amount: decimal.NewFromInt(30), Before: state(40, 0, 7), After: state(10, 0, 8)},
		{Ref: "11111111-1111-4111-8111-111111111111:10:t1-debit-2:primary:0", TransactionID: t1, PostingRef: "t1-debit-2", Role: core.RolePrimary, BalanceRef: primary.BalanceRef, Type: core.PostingDebit, Amount: decimal.NewFromInt(10), OverdraftDelta: decimal.NewFromInt(10), Before: state(10, 0, 8), After: state(0, 10, 9)},
		{Ref: "11111111-1111-4111-8111-111111111111:10:t1-debit-2:overdraft_companion:0", TransactionID: t1, PostingRef: "t1-debit-2", Role: core.RoleOverdraftCompanion, BalanceRef: companion.BalanceRef, Type: core.PostingDebit, Amount: decimal.NewFromInt(10), Before: state(0, 0, 3), After: state(10, 0, 4)},
		{Ref: "12121212-1212-4212-8212-121212121212:9:t2-credit:primary:0", TransactionID: t2, PostingRef: "t2-credit", Role: core.RolePrimary, BalanceRef: primary.BalanceRef, Type: core.PostingCredit, OverdraftDelta: decimal.NewFromInt(-6), Before: state(0, 10, 9), After: state(0, 4, 10)},
		{Ref: "12121212-1212-4212-8212-121212121212:9:t2-credit:overdraft_companion:0", TransactionID: t2, PostingRef: "t2-credit", Role: core.RoleOverdraftCompanion, BalanceRef: companion.BalanceRef, Type: core.PostingCredit, Amount: decimal.NewFromInt(6), Before: state(10, 0, 4), After: state(4, 0, 5)},
		{Ref: "13131313-1313-4313-8313-131313131313:8:t3-debit:primary:0", TransactionID: t3, PostingRef: "t3-debit", Role: core.RolePrimary, BalanceRef: primary.BalanceRef, Type: core.PostingDebit, OverdraftDelta: decimal.NewFromInt(8), Before: state(0, 4, 10), After: state(0, 12, 11)},
		{Ref: "13131313-1313-4313-8313-131313131313:8:t3-debit:overdraft_companion:0", TransactionID: t3, PostingRef: "t3-debit", Role: core.RoleOverdraftCompanion, BalanceRef: companion.BalanceRef, Type: core.PostingDebit, Amount: decimal.NewFromInt(8), Before: state(4, 0, 5), After: state(12, 0, 6)},
	}
	primary.Available, primary.OverdraftUsed, primary.Version = decimal.Zero, decimal.NewFromInt(12), 11
	companion.Available, companion.Version = decimal.NewFromInt(12), 6
	return &core.ExecutionResult{Movements: movements, Final: []core.BalanceSnapshot{primary, companion}}
}

func multiTransactionAcceptancePrimary() core.BalanceSnapshot {
	return core.BalanceSnapshot{
		BalanceRef: "@source#default", ID: uuid.MustParse("44444444-4444-4444-8444-444444444444"),
		AccountID: uuid.MustParse("55555555-5555-4555-8555-555555555555"), AccountType: "deposit", AssetCode: "BRL",
		Alias: "@source", Key: "default", Direction: "credit", BalanceScope: "transactional",
		Available: decimal.NewFromInt(40), OverdraftLimit: decimal.NewFromInt(100), Version: 7,
		AllowSending: true, AllowReceiving: true, AllowOverdraft: true, OverdraftLimitEnabled: true,
	}
}

func multiTransactionAcceptanceCompanion() core.BalanceSnapshot {
	balance := multiTransactionAcceptancePrimary()
	balance.BalanceRef = "@source#overdraft"
	balance.ID = uuid.MustParse("22222222-2222-4222-8222-222222222222")
	balance.Key = "overdraft"
	balance.Direction = "debit"
	balance.BalanceScope = "internal"
	balance.Available = decimal.Zero
	balance.OverdraftLimit = decimal.Zero
	balance.Version = 3
	balance.AllowOverdraft = false
	balance.OverdraftLimitEnabled = false
	return balance
}

func assertMultiTransactionAcceptanceState(t *testing.T, inspector *redis.Client, keys resolvedExecutionKeys, input command.EngineExecution) {
	t.Helper()
	ctx := context.Background()
	expected := multiTransactionAcceptanceResult()
	for _, balance := range expected.Final {
		raw, err := inspector.Get(ctx, keys.Balances[balance.BalanceRef].Balance).Bytes()
		require.NoError(t, err)
		cached, err := balancecache.Decode(raw)
		require.NoError(t, err)
		requireJSONEqual(t, balance, cached)
	}

	members, err := inspector.ZRange(ctx, keys.Schedule, 0, -1).Result()
	require.NoError(t, err)
	require.Equal(t, []string{keys.Balances["@source#default"].Balance, keys.Balances["@source#overdraft"].Balance}, members)
	require.Equal(t, int64(3), inspector.HLen(ctx, keys.Guards).Val())
	for index, token := range []string{"accepted-t1", "accepted-t2", "accepted-t3"} {
		actual, getErr := inspector.HGet(ctx, keys.Guards, input.Execution.Transactions[index].ID.String()).Result()
		require.NoError(t, getErr)
		require.Equal(t, token, actual)
	}

	require.Equal(t, int64(3), inspector.HLen(ctx, keys.Recovery).Val())
	transactionMovementRanges := [][2]int{{0, 3}, {3, 5}, {5, 7}}
	intermediatePrimary := []core.BalanceSnapshot{expected.Final[0], expected.Final[0], expected.Final[0]}
	intermediateCompanion := []core.BalanceSnapshot{expected.Final[1], expected.Final[1], expected.Final[1]}
	intermediatePrimary[0].OverdraftUsed, intermediatePrimary[0].Version = decimal.NewFromInt(10), 9
	intermediateCompanion[0].Available, intermediateCompanion[0].Version = decimal.NewFromInt(10), 4
	intermediatePrimary[1].OverdraftUsed, intermediatePrimary[1].Version = decimal.NewFromInt(4), 10
	intermediateCompanion[1].Available, intermediateCompanion[1].Version = decimal.NewFromInt(4), 5
	for index, transaction := range input.Execution.Transactions {
		field := transaction.ID.String() + ":" + input.Execution.ExecutionID.String()
		raw, getErr := inspector.HGet(ctx, keys.Recovery, field).Bytes()
		require.NoError(t, getErr)
		envelope, decodeErr := command.DecodeTransactionCompletionRecord(raw)
		require.NoError(t, decodeErr)
		require.Equal(t, input.Execution.ExecutionID, envelope.ExecutionID)
		require.Equal(t, transaction.ID, envelope.TransactionID)
		require.Equal(t, input.IntentFingerprint, envelope.IntentFingerprint)
		require.Equal(t, string(input.CompletionPlans[index].Payload), envelope.Payload)
		movementRange := transactionMovementRanges[index]
		requireJSONEqual(t, expected.Movements[movementRange[0]:movementRange[1]], envelope.Result.Movements)
		requireJSONEqual(t, []core.BalanceSnapshot{intermediatePrimary[index], intermediateCompanion[index]}, envelope.Result.Final)
	}

	require.Equal(t, int64(1), inspector.HLen(ctx, keys.Receipts).Val())
	receipt, err := inspector.HGet(ctx, keys.Receipts, input.Execution.ExecutionID.String()).Bytes()
	require.NoError(t, err)
	var saved struct {
		FormatVersion     int    `json:"formatVersion"`
		ExecutionID       string `json:"executionId"`
		IntentFingerprint string `json:"intentFingerprint"`
		Response          string `json:"response"`
	}
	require.NoError(t, json.Unmarshal(receipt, &saved))
	require.Equal(t, 1, saved.FormatVersion)
	require.Equal(t, input.Execution.ExecutionID.String(), saved.ExecutionID)
	require.Equal(t, input.IntentFingerprint, saved.IntentFingerprint)
	replayed, err := DecodeResult([]byte(saved.Response), input.Execution)
	require.NoError(t, err)
	requireJSONEqual(t, expected, replayed)
}

func requireJSONEqual(t *testing.T, expected, actual any) {
	t.Helper()
	want, err := json.Marshal(expected)
	require.NoError(t, err)
	got, err := json.Marshal(actual)
	require.NoError(t, err)
	require.JSONEq(t, string(want), string(got))
}

func deleteMultiTransactionAcceptanceState(t *testing.T, inspector *redis.Client, keys resolvedExecutionKeys) {
	t.Helper()
	inventory := []string{keys.Schedule, keys.Recovery, keys.Receipts, keys.Guards}
	for _, balance := range keys.Balances {
		inventory = append(inventory, balance.Balance, balance.Deleted)
	}
	require.NoError(t, inspector.Del(context.Background(), inventory...).Err())
}
