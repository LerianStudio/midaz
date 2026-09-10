//go:build integration

// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package engine

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/redis/balancecache"
	core "github.com/LerianStudio/midaz/v4/components/ledger/internal/engine"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/services/command"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	"github.com/LerianStudio/midaz/v4/pkg/mtransaction"
)

func TestIntegration_ExecutePreparedBalanceEngine_UsesLiveValkeyState(t *testing.T) {
	ctx := context.Background()
	client, _, _ := newAdapterValkey(t)

	tests := []struct {
		name                     string
		seedAvailable            int64
		liveAvailable            int64
		wantPrimaryAvailable     string
		wantPrimaryOverdraftUsed string
		wantPrimaryVersion       int64
		wantCompanionAvailable   string
		wantCompanionVersion     int64
		wantMovementRoles        []string
		wantRows                 int
	}{
		{
			name:          "stale snapshot predicts deficit but live state eliminates it",
			seedAvailable: 10, liveAvailable: 40,
			wantPrimaryAvailable: "10", wantPrimaryOverdraftUsed: "0", wantPrimaryVersion: 9,
			wantCompanionAvailable: "0", wantCompanionVersion: 3,
			wantMovementRoles: []string{core.RolePrimary}, wantRows: 1,
		},
		{
			name:          "stale snapshot predicts no deficit but live state produces it",
			seedAvailable: 40, liveAvailable: 10,
			wantPrimaryAvailable: "0", wantPrimaryOverdraftUsed: "20", wantPrimaryVersion: 9,
			wantCompanionAvailable: "20", wantCompanionVersion: 4,
			wantMovementRoles: []string{core.RolePrimary, core.RoleOverdraftCompanion}, wantRows: 2,
		},
	}

	for scenarioIndex, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fixture := newLiveStateCompositionFixture(t, scenarioIndex, test.seedAvailable)
			keys, err := resolveAdapterKeys(ctx, core.Request{
				OrganizationID: fixture.organizationID,
				LedgerID:       fixture.ledgerID,
				Balances:       []core.BalanceSnapshot{fixture.primary, fixture.companion},
			})
			require.NoError(t, err)
			seedLiveStateCompositionBalance(t, ctx, client, keys, fixture.primary)
			seedLiveStateCompositionBalance(t, ctx, client, keys, fixture.companion)
			t.Cleanup(func() { deleteLiveStateCompositionState(t, client, keys) })

			adapter, err := NewAdapter(&integrationClientProvider{client: client}, guardBootstrapLimits())
			require.NoError(t, err)
			prepared, err := buildLiveStateCompositionExecution(ctx, t, client, keys, fixture)
			require.NoError(t, err)
			require.Equal(t, decimal.NewFromInt(test.seedAvailable), decimal.Decimal(prepared.CompletionPlan.OperationSpecs[0].Balance.Available))

			live := fixture.primary
			live.Available = decimal.NewFromInt(test.liveAvailable)
			live.Version = 8
			encoded, err := balancecache.Encode(live, balancecache.FormatDual)
			require.NoError(t, err)
			require.NoError(t, client.Set(ctx, keys.Balances[live.BalanceRef].Balance, encoded, 0).Err())

			got, err := command.ExecutePreparedBalanceEngine(ctx, adapter, prepared)
			require.NoError(t, err)

			require.NotNil(t, got.Result)
			require.Equal(t, test.wantMovementRoles, liveStateCompositionMovementRoles(got.Result.Movements))
			assertLiveStateCompositionFinal(t, got.Result, test.wantPrimaryAvailable, test.wantPrimaryOverdraftUsed,
				test.wantPrimaryVersion, test.wantCompanionAvailable, test.wantCompanionVersion)
			assertLiveStateCompositionPersistedBalances(t, ctx, client, keys, test.wantPrimaryAvailable,
				test.wantPrimaryOverdraftUsed, test.wantPrimaryVersion, test.wantCompanionAvailable, test.wantCompanionVersion)
			assertLiveStateCompositionSchedule(t, ctx, client, keys, test.wantRows == 2)

			rows, err := command.BuildOperationRecordsFromMovements(got.Prepared.CompletionPlan, *got.Result)
			require.NoError(t, err)
			require.Len(t, rows, test.wantRows)
			assertLiveStateCompositionStoredOutcome(t, ctx, client, keys, got, rows)
		})
	}
}

func TestIntegration_AdapterExecute_UsesLiveOverdraftSettingsWithoutVersionBump(t *testing.T) {
	ctx := context.Background()
	client, _, _ := newAdapterValkey(t)
	fixture := newLiveStateCompositionFixture(t, 2, 10)
	fixture.primary.AllowOverdraft = false
	fixture.primary.OverdraftLimitEnabled = false
	fixture.primary.OverdraftLimit = decimal.Zero
	keys, err := resolveAdapterKeys(ctx, core.Request{
		OrganizationID: fixture.organizationID,
		LedgerID:       fixture.ledgerID,
		Balances:       []core.BalanceSnapshot{fixture.primary, fixture.companion},
	})
	require.NoError(t, err)
	seedLiveStateCompositionBalance(t, ctx, client, keys, fixture.primary)
	seedLiveStateCompositionBalance(t, ctx, client, keys, fixture.companion)
	t.Cleanup(func() { deleteLiveStateCompositionState(t, client, keys) })

	prepared, err := buildLiveStateCompositionExecution(ctx, t, client, keys, fixture)
	require.NoError(t, err)
	preparedPrimary := liveStateCompositionSnapshot(t, prepared.Execution.Request.Balances, "@source#default")
	require.False(t, preparedPrimary.AllowOverdraft)
	require.False(t, preparedPrimary.OverdraftLimitEnabled)
	require.Equal(t, int64(7), preparedPrimary.Version)
	require.Len(t, prepared.CompletionPlan.OperationSpecs, 2)
	require.Equal(t, core.RoleOverdraftCompanion, prepared.CompletionPlan.OperationSpecs[1].Role)
	require.Equal(t, "@source#overdraft", prepared.CompletionPlan.OperationSpecs[1].BalanceRef)

	livePrimary := fixture.primary
	livePrimary.AllowOverdraft = true
	livePrimary.OverdraftLimitEnabled = true
	livePrimary.OverdraftLimit = decimal.NewFromInt(100)
	require.Equal(t, int64(7), livePrimary.Version)
	encoded, err := balancecache.Encode(livePrimary, balancecache.FormatDual)
	require.NoError(t, err)
	require.NoError(t, client.Set(ctx, keys.Balances[livePrimary.BalanceRef].Balance, encoded, 0).Err())

	provider := &integrationClientProvider{client: client}
	adapter, err := NewAdapter(provider, guardBootstrapLimits())
	require.NoError(t, err)
	result, err := adapter.Execute(ctx, prepared.Execution)
	require.NoError(t, err)
	require.Equal(t, 1, provider.calls)
	require.Equal(t, []string{core.RolePrimary, core.RoleOverdraftCompanion}, liveStateCompositionMovementRoles(result.Movements))
	assertLiveStateCompositionFinal(t, result, "0", "20", 8, "20", 4)
	assertLiveStateCompositionPersistedBalances(t, ctx, client, keys, "0", "20", 8, "20", 4)
	assertLiveStateCompositionSchedule(t, ctx, client, keys, true)

	rows, err := command.BuildOperationRecordsFromMovements(prepared.CompletionPlan, *result)
	require.NoError(t, err)
	require.Len(t, rows, 2)
	require.Equal(t, []string{constant.DEBIT, constant.OVERDRAFT}, []string{rows[0].Type, rows[1].Type})
	require.Equal(t, []string{"10", "20"}, []string{rows[0].Amount.Value.String(), rows[1].Amount.Value.String()})
	assertLiveStateCompositionStoredOutcome(t, ctx, client, keys,
		command.BalanceEngineExecutionOutcome{Prepared: prepared, Result: result}, rows)
}

type liveStateCompositionFixture struct {
	organizationID uuid.UUID
	ledgerID       uuid.UUID
	transactionID  uuid.UUID
	executionID    uuid.UUID
	primary        core.BalanceSnapshot
	companion      core.BalanceSnapshot
	transaction    mtransaction.Transaction
	validation     *mtransaction.Responses
	date           time.Time
}

func liveStateCompositionSnapshot(t *testing.T, snapshots []core.BalanceSnapshot, balanceRef string) core.BalanceSnapshot {
	t.Helper()
	for _, snapshot := range snapshots {
		if snapshot.BalanceRef == balanceRef {
			return snapshot
		}
	}
	t.Fatalf("missing snapshot %q", balanceRef)

	return core.BalanceSnapshot{}
}

func newLiveStateCompositionFixture(t *testing.T, scenarioIndex int, seedAvailable int64) liveStateCompositionFixture {
	t.Helper()
	namespace := uuid.MustParse("8f5ae076-1db7-41e5-a330-861248541fac")
	suffix := []byte{byte(scenarioIndex + 1)}
	organizationID := uuid.NewSHA1(namespace, append([]byte("organization"), suffix...))
	ledgerID := uuid.NewSHA1(namespace, append([]byte("ledger"), suffix...))
	transactionID := uuid.NewSHA1(namespace, append([]byte("transaction"), suffix...))
	executionID := uuid.NewSHA1(namespace, append([]byte("execution"), suffix...))
	accountID := uuid.NewSHA1(namespace, append([]byte("account"), suffix...))
	primary := core.BalanceSnapshot{
		BalanceRef: "@source#default", ID: uuid.NewSHA1(namespace, append([]byte("primary"), suffix...)), AccountID: accountID,
		AccountType: "deposit", AssetCode: "USD", Alias: "@source", Key: constant.DefaultBalanceKey,
		Direction: constant.DirectionCredit, BalanceScope: mmodel.BalanceScopeTransactional,
		Available: decimal.NewFromInt(seedAvailable), OverdraftLimit: decimal.NewFromInt(100), Version: 7,
		AllowSending: true, AllowReceiving: true, AllowOverdraft: true, OverdraftLimitEnabled: true,
	}
	companion := core.BalanceSnapshot{
		BalanceRef: "@source#overdraft", ID: uuid.NewSHA1(namespace, append([]byte("companion"), suffix...)), AccountID: accountID,
		AccountType: "deposit", AssetCode: "USD", Alias: "@source", Key: constant.OverdraftBalanceKey,
		Direction: constant.DirectionDebit, BalanceScope: mmodel.BalanceScopeInternal,
		Version: 3, AllowSending: true, AllowReceiving: true,
	}
	amount := mtransaction.Amount{Asset: "USD", Value: decimal.NewFromInt(30), Operation: constant.DEBIT}
	transaction := mtransaction.Transaction{Description: "live state composition", Send: mtransaction.Send{
		Asset: "USD", Value: decimal.NewFromInt(30),
		Source: mtransaction.Source{From: []mtransaction.FromTo{{
			AccountAlias: "0#@source#default", BalanceKey: constant.DefaultBalanceKey, IsFrom: true,
			Description: "source debit", ChartOfAccounts: "customer", Metadata: map[string]any{"scenario": scenarioIndex + 1},
		}}},
	}}

	return liveStateCompositionFixture{
		organizationID: organizationID, ledgerID: ledgerID, transactionID: transactionID, executionID: executionID,
		primary: primary, companion: companion, transaction: transaction,
		validation: &mtransaction.Responses{Asset: "USD", Total: decimal.NewFromInt(30), From: map[string]mtransaction.Amount{"0#@source#default": amount}},
		date:       time.Date(2026, time.September, 8, 15, 0, 0, 0, time.UTC),
	}
}

func buildLiveStateCompositionExecution(
	ctx context.Context,
	t *testing.T,
	client *redis.Client,
	keys resolvedExecutionKeys,
	fixture liveStateCompositionFixture,
) (command.PreparedBalanceEngineExecution, error) {
	t.Helper()
	pool, err := command.LoadBalanceEngineSnapshotPool(ctx, fixture.organizationID, fixture.ledgerID,
		[]string{"@source#default"}, func(ctx context.Context, _, _ uuid.UUID, aliases []string) ([]*mmodel.Balance, error) {
			balances := make([]*mmodel.Balance, 0, len(aliases))
			for _, alias := range aliases {
				raw, getErr := client.Get(ctx, keys.Balances[alias].Balance).Bytes()
				if errors.Is(getErr, redis.Nil) {
					continue
				}
				if getErr != nil {
					return nil, getErr
				}
				snapshot, decodeErr := balancecache.DecodeForRead(raw)
				if decodeErr != nil {
					return nil, decodeErr
				}
				balances = append(balances, liveStateCompositionModel(fixture, snapshot))
			}

			return balances, nil
		})
	if err != nil {
		return command.PreparedBalanceEngineExecution{}, err
	}

	translated, projection, err := command.TranslateBalanceEngineTransaction(command.BalanceEngineTranslationInput{
		TransactionID: fixture.transactionID, Action: constant.ActionDirect, TransactionStatus: constant.CREATED,
		TransactionInput: fixture.transaction, Validate: fixture.validation, Balances: pool.Balances,
	})
	if err != nil {
		return command.PreparedBalanceEngineExecution{}, err
	}

	payload := command.TransactionCompletionPlan{
		FormatVersion: command.TransactionCompletionFormatVersion, HeaderID: "live-state-composition",
		TransactionID: fixture.transactionID, OrganizationID: fixture.organizationID, LedgerID: fixture.ledgerID,
		ExecutionID: fixture.executionID, TransactionInput: fixture.transaction, Validate: fixture.validation,
		TTL: fixture.date, TransactionStatus: constant.CREATED, Action: constant.ActionDirect,
		TransactionDate: fixture.date, TransactionCreatedAt: fixture.date, TransactionUpdatedAt: fixture.date, OperationUpdatedAt: fixture.date,
		OperationSpecs: projection,
	}
	execution := command.EngineExecution{
		Request: core.Request{
			OrganizationID: fixture.organizationID, LedgerID: fixture.ledgerID, ExecutionID: fixture.executionID,
			Transactions: []core.Transaction{translated}, Balances: pool.Snapshots,
		},
		Guards: []command.ExecutionGuard{{TransactionID: fixture.transactionID, NextToken: constant.APPROVED}},
	}
	execution.CompletionPlans = []command.CompletionPlanRecord{{TransactionID: fixture.transactionID, Payload: encodeAdapterRecovery(t, &execution, payload)}}
	payload.IntentFingerprint = execution.IntentFingerprint

	return command.PreparedBalanceEngineExecution{Execution: execution, CompletionPlan: payload}, nil
}

func liveStateCompositionModel(fixture liveStateCompositionFixture, snapshot core.BalanceSnapshot) *mmodel.Balance {
	model := &mmodel.Balance{
		ID: snapshot.ID.String(), OrganizationID: fixture.organizationID.String(), LedgerID: fixture.ledgerID.String(),
		AccountID: snapshot.AccountID.String(), Alias: snapshot.Alias, Key: snapshot.Key, AssetCode: snapshot.AssetCode,
		Available: snapshot.Available, OnHold: snapshot.OnHold, Version: snapshot.Version, AccountType: snapshot.AccountType,
		AllowSending: snapshot.AllowSending, AllowReceiving: snapshot.AllowReceiving, Direction: snapshot.Direction,
		OverdraftUsed: snapshot.OverdraftUsed, CreatedAt: fixture.date, UpdatedAt: fixture.date,
		Settings: &mmodel.BalanceSettings{
			BalanceScope: snapshot.BalanceScope, AllowOverdraft: snapshot.AllowOverdraft,
			OverdraftLimitEnabled: snapshot.OverdraftLimitEnabled,
		},
	}
	if snapshot.OverdraftLimitEnabled {
		limit := snapshot.OverdraftLimit.String()
		model.Settings.OverdraftLimit = &limit
	}

	return model
}

func seedLiveStateCompositionBalance(t *testing.T, ctx context.Context, client *redis.Client, keys resolvedExecutionKeys, balance core.BalanceSnapshot) {
	t.Helper()
	encoded, err := balancecache.Encode(balance, balancecache.FormatDual)
	require.NoError(t, err)
	require.NoError(t, client.Set(ctx, keys.Balances[balance.BalanceRef].Balance, encoded, 0).Err())
}

func liveStateCompositionMovementRoles(movements []core.Movement) []string {
	roles := make([]string, len(movements))
	for index := range movements {
		roles[index] = movements[index].Role
	}

	return roles
}

func assertLiveStateCompositionFinal(
	t *testing.T,
	result *core.Result,
	wantPrimaryAvailable, wantPrimaryOverdraftUsed string,
	wantPrimaryVersion int64,
	wantCompanionAvailable string,
	wantCompanionVersion int64,
) {
	t.Helper()
	final := make(map[string]core.BalanceSnapshot, len(result.Final))
	for _, balance := range result.Final {
		final[balance.BalanceRef] = balance
	}
	require.Equal(t, wantPrimaryAvailable, final["@source#default"].Available.String())
	require.Equal(t, wantPrimaryOverdraftUsed, final["@source#default"].OverdraftUsed.String())
	require.Equal(t, wantPrimaryVersion, final["@source#default"].Version)
	if companion, ok := final["@source#overdraft"]; ok {
		require.Equal(t, wantCompanionAvailable, companion.Available.String())
		require.Equal(t, wantCompanionVersion, companion.Version)
	}
}

func assertLiveStateCompositionPersistedBalances(
	t *testing.T,
	ctx context.Context,
	client *redis.Client,
	keys resolvedExecutionKeys,
	wantPrimaryAvailable, wantPrimaryOverdraftUsed string,
	wantPrimaryVersion int64,
	wantCompanionAvailable string,
	wantCompanionVersion int64,
) {
	t.Helper()
	primaryRaw, err := client.Get(ctx, keys.Balances["@source#default"].Balance).Bytes()
	require.NoError(t, err)
	primary, err := balancecache.Decode(primaryRaw)
	require.NoError(t, err)
	require.Equal(t, wantPrimaryAvailable, primary.Available.String())
	require.Equal(t, wantPrimaryOverdraftUsed, primary.OverdraftUsed.String())
	require.Equal(t, wantPrimaryVersion, primary.Version)

	companionRaw, err := client.Get(ctx, keys.Balances["@source#overdraft"].Balance).Bytes()
	require.NoError(t, err)
	companion, err := balancecache.Decode(companionRaw)
	require.NoError(t, err)
	require.Equal(t, wantCompanionAvailable, companion.Available.String())
	require.Equal(t, wantCompanionVersion, companion.Version)
}

func assertLiveStateCompositionSchedule(t *testing.T, ctx context.Context, client *redis.Client, keys resolvedExecutionKeys, companionMoved bool) {
	t.Helper()
	members, err := client.ZRange(ctx, keys.Schedule, 0, -1).Result()
	require.NoError(t, err)
	want := []string{keys.Balances["@source#default"].Balance}
	if companionMoved {
		want = append(want, keys.Balances["@source#overdraft"].Balance)
	}
	require.ElementsMatch(t, want, members)
}

func assertLiveStateCompositionStoredOutcome(
	t *testing.T,
	ctx context.Context,
	client *redis.Client,
	keys resolvedExecutionKeys,
	got command.BalanceEngineExecutionOutcome,
	rows any,
) {
	t.Helper()
	transactionID := got.Prepared.CompletionPlan.TransactionID
	executionID := got.Prepared.Execution.Request.ExecutionID
	require.Equal(t, int64(1), client.HLen(ctx, keys.Guards).Val())
	require.Equal(t, constant.APPROVED, client.HGet(ctx, keys.Guards, transactionID.String()).Val())
	require.Equal(t, int64(1), client.HLen(ctx, keys.Recovery).Val())
	require.Equal(t, int64(1), client.HLen(ctx, keys.Receipts).Val())

	raw, err := client.HGet(ctx, keys.Recovery, transactionID.String()+":"+executionID.String()).Bytes()
	require.NoError(t, err)
	envelope, err := command.DecodeTransactionCompletionRecord(raw)
	require.NoError(t, err)
	require.Equal(t, executionID, envelope.ExecutionID)
	require.Equal(t, transactionID, envelope.TransactionID)
	require.Equal(t, got.Prepared.Execution.IntentFingerprint, envelope.IntentFingerprint)
	require.JSONEq(t, string(got.Prepared.Execution.CompletionPlans[0].Payload), envelope.Payload)

	recoveredPayload, err := command.DecodeTransactionCompletionPlan([]byte(envelope.Payload))
	require.NoError(t, err)
	recoveredRows, err := command.BuildOperationRecordsFromMovements(*recoveredPayload, envelope.Result)
	require.NoError(t, err)
	wantJSON, err := json.Marshal(rows)
	require.NoError(t, err)
	gotJSON, err := json.Marshal(recoveredRows)
	require.NoError(t, err)
	require.JSONEq(t, string(wantJSON), string(gotJSON))

	receiptRaw, err := client.HGet(ctx, keys.Receipts, executionID.String()).Bytes()
	require.NoError(t, err)
	var receipt struct {
		FormatVersion     int    `json:"formatVersion"`
		ExecutionID       string `json:"executionId"`
		IntentFingerprint string `json:"intentFingerprint"`
		Response          string `json:"response"`
	}
	require.NoError(t, json.Unmarshal(receiptRaw, &receipt))
	require.Equal(t, 1, receipt.FormatVersion)
	require.Equal(t, executionID.String(), receipt.ExecutionID)
	require.Equal(t, got.Prepared.Execution.IntentFingerprint, receipt.IntentFingerprint)
	replayed, err := DecodeResult([]byte(receipt.Response), got.Prepared.Execution.Request)
	require.NoError(t, err)
	requireJSONEqual(t, got.Result, replayed)
}

func deleteLiveStateCompositionState(t *testing.T, client *redis.Client, keys resolvedExecutionKeys) {
	t.Helper()
	inventory := []string{keys.Schedule, keys.Recovery, keys.Receipts, keys.Guards}
	for _, balance := range keys.Balances {
		inventory = append(inventory, balance.Balance, balance.Deleted)
	}
	require.NoError(t, client.Del(context.Background(), inventory...).Err())
}
