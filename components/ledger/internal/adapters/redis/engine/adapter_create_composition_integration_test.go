//go:build integration

// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package engine

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	tmcore "github.com/LerianStudio/lib-commons/v6/commons/tenant-manager/core"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/redis/balancecache"
	txredis "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/redis/transaction"
	core "github.com/LerianStudio/midaz/v4/components/ledger/internal/engine"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/services/command"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	"github.com/LerianStudio/midaz/v4/pkg/mtransaction"
)

type adapterCreateReader struct {
	command.TransactionReader
	balances []*mmodel.Balance
	reads    int
}

func (r *adapterCreateReader) GetParsedLedgerSettings(context.Context, uuid.UUID, uuid.UUID) (mmodel.LedgerSettings, error) {
	return mmodel.LedgerSettings{}, nil
}

func (r *adapterCreateReader) GetBalances(_ context.Context, _, _ uuid.UUID, aliases []string) ([]*mmodel.Balance, error) {
	r.reads++
	out := make([]*mmodel.Balance, 0, len(aliases))
	for _, alias := range aliases {
		for _, balance := range r.balances {
			if mtransaction.AliasKey(balance.Alias, balance.Key) == alias {
				out = append(out, balance)
			}
		}
	}
	return out, nil
}

func (r *adapterCreateReader) GetBalanceEngineBalances(ctx context.Context, organizationID, ledgerID uuid.UUID, aliases []string) ([]*mmodel.Balance, []*mmodel.Balance, error) {
	pool, err := command.LoadBalanceEngineSnapshotPool(ctx, organizationID, ledgerID, aliases, r.GetBalances)
	return pool.ExplicitBalances, pool.Balances, err
}

func (r *adapterCreateReader) ValidateAccountingRules(context.Context, uuid.UUID, uuid.UUID, []mmodel.BalanceOperation, *mtransaction.Responses, string) (*mmodel.TransactionRouteCache, error) {
	return nil, nil
}

type recordingCreateAdapter struct {
	delegate command.BalanceEngine
	inputs   []command.EngineExecution
}

func (a *recordingCreateAdapter) Execute(ctx context.Context, input command.EngineExecution) (*core.Result, error) {
	a.inputs = append(a.inputs, input)
	return a.delegate.Execute(ctx, input)
}

type adapterCreateFinalizer struct {
	envelope *command.BalanceEngineRecoveryEnvelope
}

func (f *adapterCreateFinalizer) FinalizeWithOutcome(_ context.Context, envelope *command.BalanceEngineRecoveryEnvelope) (command.BalanceEngineFinalizationResult, error) {
	f.envelope = envelope
	payload, err := command.DecodeBalanceEngineRecoveryPayload([]byte(envelope.Payload))
	if err != nil {
		return command.BalanceEngineFinalizationResult{}, err
	}
	record, err := command.ComposeBalanceEnginePersistenceRecord(*payload, envelope.Result)
	if err != nil {
		return command.BalanceEngineFinalizationResult{}, err
	}

	return command.BalanceEngineFinalizationResult{
		Record:  record,
		Outcome: command.BalanceEngineRecoveryOutcome{TransactionStatus: constant.APPROVED},
	}, nil
}

func TestIntegration_CreateTransactionV1_ComposesRealAdapterRecoveryAndFinalization(t *testing.T) {
	t.Setenv("AUDIT_LOG_ENABLED", "false")
	ctx := tmcore.ContextWithTenantID(context.Background(), "tenant-create-composition")
	client, _, _ := newAdapterValkey(t)
	orgID := uuid.MustParse("81111111-1111-4111-8111-111111111111")
	ledgerID := uuid.MustParse("82222222-2222-4222-8222-222222222222")
	reader := &adapterCreateReader{balances: []*mmodel.Balance{
		adapterCreateBalance(orgID, ledgerID, "83333333-3333-4333-8333-333333333333", "84444444-4444-4444-8444-444444444444", "@source", 100, 7),
		adapterCreateBalance(orgID, ledgerID, "85555555-5555-4555-8555-555555555555", "86666666-6666-4666-8666-666666666666", "@target", 20, 3),
	}}

	ctrl := gomock.NewController(t)
	idempotency := txredis.NewMockRedisRepository(ctrl)
	stored := make(chan struct{})
	idempotency.EXPECT().SetNX(gomock.Any(), gomock.Any(), "", time.Minute).Return(true, nil)
	idempotency.EXPECT().Set(gomock.Any(), gomock.Any(), gomock.Any(), time.Minute).DoAndReturn(
		func(context.Context, string, string, time.Duration) error { close(stored); return nil },
	)
	realAdapter, err := NewAdapter(&integrationClientProvider{client: client}, guardBootstrapLimits())
	require.NoError(t, err)
	executor := &recordingCreateAdapter{delegate: realAdapter}
	finalizer := &adapterCreateFinalizer{}
	uc := &command.UseCase{
		TransactionRedisRepo: idempotency, TransactionReader: reader,
		BalanceEngine: executor, BalanceEngineFinalizer: finalizer,
	}
	date := time.Date(2026, time.September, 8, 16, 0, 0, 0, time.UTC)
	amount := decimal.NewFromInt(30)
	input := mtransaction.Transaction{
		TransactionDate: (*mtransaction.TransactionDate)(&date),
		Send: mtransaction.Send{
			Asset: "USD", Value: amount,
			Source: mtransaction.Source{From: []mtransaction.FromTo{{
				AccountAlias: "@source", Amount: &mtransaction.Amount{Asset: "USD", Value: amount},
			}}},
			Distribute: mtransaction.Distribute{To: []mtransaction.FromTo{{
				AccountAlias: "@target", Amount: &mtransaction.Amount{Asset: "USD", Value: amount},
			}}},
		},
	}

	got, replayed, err := uc.CreateTransactionV1(ctx, command.CreateTransactionV1Input{
		OrganizationID: orgID, LedgerID: ledgerID, Transaction: input,
		TransactionStatus: constant.CREATED, IdempotencyTTL: time.Minute,
	})
	require.NoError(t, err)
	require.False(t, replayed)
	require.NotNil(t, got)
	require.Equal(t, constant.CREATED, got.Status.Code)
	require.Equal(t, date, got.CreatedAt)
	require.Len(t, got.Operations, 2)
	require.Equal(t, []string{"30", "30"}, []string{got.Operations[0].Amount.Value.String(), got.Operations[1].Amount.Value.String()})
	require.Equal(t, []int64{7, 3}, []int64{*got.Operations[0].Balance.Version, *got.Operations[1].Balance.Version})
	require.Equal(t, []int64{8, 4}, []int64{*got.Operations[0].BalanceAfter.Version, *got.Operations[1].BalanceAfter.Version})
	require.Equal(t, 2, reader.reads)
	require.Len(t, executor.inputs, 1)

	execution := executor.inputs[0]
	require.Equal(t, got.ID, execution.Request.Transactions[0].ID.String())
	require.Len(t, execution.Request.Transactions[0].Postings, 2)
	require.True(t, execution.Request.Transactions[0].Postings[0].Amount.Equal(amount))
	require.True(t, execution.Request.Transactions[0].Postings[1].Amount.Equal(amount))
	require.NotNil(t, finalizer.envelope)
	require.Equal(t, command.BalanceEngineRecoveryVersion, finalizer.envelope.FormatVersion)
	require.Equal(t, execution.Request.ExecutionID, finalizer.envelope.ExecutionID)
	require.Equal(t, execution.IntentFingerprint, finalizer.envelope.IntentFingerprint)

	payload, err := command.DecodeBalanceEngineRecoveryPayload([]byte(finalizer.envelope.Payload))
	require.NoError(t, err)
	require.Equal(t, command.BalanceEngineRecoveryVersion, payload.FormatVersion)
	require.Equal(t, "tenant-create-composition", payload.TenantID)
	projected, err := command.ProjectBalanceEngineOperations(*payload, finalizer.envelope.Result)
	require.NoError(t, err)
	requireJSONEqual(t, projected, got.Operations)

	keys, err := resolveAdapterKeys(ctx, execution.Request)
	require.NoError(t, err)
	t.Cleanup(func() { deleteMultiTransactionAcceptanceState(t, client, keys) })
	assertAdapterCreateBalances(t, ctx, client, keys)
	require.Equal(t, int64(1), client.HLen(ctx, keys.Guards).Val())
	require.Equal(t, constant.APPROVED, client.HGet(ctx, keys.Guards, got.ID).Val())
	require.Equal(t, int64(1), client.HLen(ctx, keys.Recovery).Val())
	require.Equal(t, int64(1), client.HLen(ctx, keys.Receipts).Val())

	recoveryRaw, err := client.HGet(ctx, keys.Recovery, got.ID+":"+execution.Request.ExecutionID.String()).Bytes()
	require.NoError(t, err)
	recovery, err := command.DecodeBalanceEngineRecoveryEnvelope(recoveryRaw)
	require.NoError(t, err)
	require.Equal(t, command.BalanceEngineRecoveryVersion, recovery.FormatVersion)
	require.Equal(t, execution.Request.ExecutionID, recovery.ExecutionID)
	require.Equal(t, execution.IntentFingerprint, recovery.IntentFingerprint)
	recoveredPayload, err := command.DecodeBalanceEngineRecoveryPayload([]byte(recovery.Payload))
	require.NoError(t, err)
	recovered, err := command.ProjectBalanceEngineOperations(*recoveredPayload, recovery.Result)
	require.NoError(t, err)
	requireJSONEqual(t, got.Operations, recovered)

	receiptRaw, err := client.HGet(ctx, keys.Receipts, execution.Request.ExecutionID.String()).Bytes()
	require.NoError(t, err)
	var receipt map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(receiptRaw, &receipt))
	require.JSONEq(t, "1", string(receipt["formatVersion"]))
	require.Equal(t, "\""+execution.Request.ExecutionID.String()+"\"", string(receipt["executionId"]))
	var response string
	require.NoError(t, json.Unmarshal(receipt["response"], &response))
	replayedResult, err := DecodeResult([]byte(response), execution.Request)
	require.NoError(t, err)
	requireJSONEqual(t, finalizer.envelope.Result, replayedResult)

	select {
	case <-stored:
	case <-time.After(time.Second):
		t.Fatal("durable create did not populate idempotency")
	}
}

func adapterCreateBalance(orgID, ledgerID uuid.UUID, balanceID, accountID, alias string, available, version int64) *mmodel.Balance {
	return &mmodel.Balance{
		ID: balanceID, OrganizationID: orgID.String(), LedgerID: ledgerID.String(), AccountID: accountID,
		Alias: alias, Key: constant.DefaultBalanceKey, AssetCode: "USD", AccountType: "deposit",
		Available: decimal.NewFromInt(available), Version: version, Direction: constant.DirectionCredit,
		AllowSending: true, AllowReceiving: true,
	}
}

func assertAdapterCreateBalances(t *testing.T, ctx context.Context, client *redis.Client, keys resolvedExecutionKeys) {
	t.Helper()
	for _, expected := range []struct {
		ref       string
		available string
		version   int64
	}{
		{ref: "@source#default", available: "70", version: 8},
		{ref: "@target#default", available: "50", version: 4},
	} {
		raw, err := client.Get(ctx, keys.Balances[expected.ref].Balance).Bytes()
		require.NoError(t, err)
		balance, err := balancecache.Decode(raw)
		require.NoError(t, err)
		require.Equal(t, expected.available, balance.Available.String())
		require.Equal(t, expected.version, balance.Version)
	}
}

var (
	_ command.BalanceEngine                 = (*recordingCreateAdapter)(nil)
	_ command.BalanceEngineOutcomeFinalizer = (*adapterCreateFinalizer)(nil)
	_ command.TransactionReader             = (*adapterCreateReader)(nil)
)
