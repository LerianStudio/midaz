//go:build integration

// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package engine

import (
	"context"
	"testing"
	"time"

	tmcore "github.com/LerianStudio/lib-commons/v6/commons/tenant-manager/core"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/redis/balancecache"
	core "github.com/LerianStudio/midaz/v4/components/ledger/internal/engine"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/services/command"
)

func TestIntegration_AdapterExecute_IsolatesSameAliasAcrossAuthenticatedScopes(t *testing.T) {
	inspector, _, _ := newAdapterValkey(t)
	adapterInput := []struct {
		name      string
		tenantID  string
		orgID     string
		ledgerID  string
		amount    int64
		available int64
	}{
		{name: "tenant-a-scope-one", tenantID: "tenant-a", orgID: "10000000-0000-4000-8000-000000000001", ledgerID: "20000000-0000-4000-8000-000000000001", amount: 11, available: 89},
		{name: "tenant-a-same-org-other-ledger", tenantID: "tenant-a", orgID: "10000000-0000-4000-8000-000000000001", ledgerID: "20000000-0000-4000-8000-000000000002", amount: 22, available: 78},
		{name: "tenant-a-other-org-same-ledger", tenantID: "tenant-a", orgID: "10000000-0000-4000-8000-000000000002", ledgerID: "20000000-0000-4000-8000-000000000001", amount: 33, available: 67},
		{name: "tenant-b-same-scope", tenantID: "tenant-b", orgID: "10000000-0000-4000-8000-000000000001", ledgerID: "20000000-0000-4000-8000-000000000001", amount: 44, available: 56},
	}

	type committedScope struct {
		ctx       context.Context
		input     command.EngineExecution
		keys      resolvedExecutionKeys
		available int64
	}
	committed := make([]committedScope, 0, len(adapterInput))
	for _, tt := range adapterInput {
		t.Run(tt.name, func(t *testing.T) {
			ctx := tmcore.ContextWithTenantID(context.Background(), tt.tenantID)
			input, limits := scopedAdapterExecution(t, tt.tenantID, tt.orgID, tt.ledgerID, tt.amount)
			adapter, err := NewAdapter(&integrationClientProvider{client: inspector}, limits)
			require.NoError(t, err)

			result, err := adapter.Execute(ctx, input)
			require.NoError(t, err)
			require.Len(t, result.Final, 1)
			require.True(t, result.Final[0].Available.Equal(decimal.NewFromInt(tt.available)))

			keys, err := resolveAdapterKeys(ctx, input.Request)
			require.NoError(t, err)
			committed = append(committed, committedScope{ctx: ctx, input: input, keys: keys, available: tt.available})
		})
	}

	seenBalances := make(map[string]bool, len(committed))
	for _, scope := range committed {
		balanceKey := scope.keys.Balances[scope.input.Request.Balances[0].BalanceRef].Balance
		require.False(t, seenBalances[balanceKey], "tenant, organization, and ledger must contribute to the physical balance identity")
		seenBalances[balanceKey] = true
		raw, err := inspector.Get(scope.ctx, balanceKey).Bytes()
		require.NoError(t, err)
		cached, err := balancecache.Decode(raw)
		require.NoError(t, err)
		require.Equal(t, "@source", cached.Alias)
		require.True(t, cached.Available.Equal(decimal.NewFromInt(scope.available)))
		require.Equal(t, int64(1), inspector.HLen(scope.ctx, scope.keys.Receipts).Val())
		require.Equal(t, int64(1), inspector.HLen(scope.ctx, scope.keys.Guards).Val())
		require.Equal(t, int64(0), inspector.Exists(scope.ctx, scope.keys.Balances[scope.input.Request.Balances[0].BalanceRef].Deleted).Val())
	}

	require.Equal(t, int64(3), inspector.ZCard(committed[0].ctx, committed[0].keys.Schedule).Val())
	require.Equal(t, int64(3), inspector.HLen(committed[0].ctx, committed[0].keys.Recovery).Val())
	require.Equal(t, int64(1), inspector.ZCard(committed[3].ctx, committed[3].keys.Schedule).Val())
	require.Equal(t, int64(1), inspector.HLen(committed[3].ctx, committed[3].keys.Recovery).Val())
}

func TestIntegration_AdapterExecute_TouchedDeletionMarkerAbortsMixedBatchWithoutWrites(t *testing.T) {
	ctx := context.Background()
	inspector, _, _ := newAdapterValkey(t)
	input, limits := multiTransactionAcceptanceExecution(t)
	keys, err := resolveAdapterKeys(ctx, input.Request)
	require.NoError(t, err)

	for _, balance := range input.Request.Balances {
		encoded, encodeErr := balancecache.Encode(balance, balancecache.FormatDual)
		require.NoError(t, encodeErr)
		require.NoError(t, inspector.Set(ctx, keys.Balances[balance.BalanceRef].Balance, encoded, time.Hour).Err())
	}
	deleted := keys.Balances["@source#overdraft"].Deleted
	require.NoError(t, inspector.Set(ctx, deleted, "1", time.Hour).Err())
	require.NoError(t, inspector.ZAdd(
		ctx, keys.Schedule,
		redis.Z{Score: 17, Member: keys.Balances["@source#default"].Balance},
		redis.Z{Score: 23, Member: keys.Balances["@source#overdraft"].Balance},
	).Err())
	for _, key := range []string{keys.Recovery, keys.Receipts, keys.Guards} {
		require.NoError(t, inspector.HSet(ctx, key, "unrelated", "preserve").Err())
	}
	for _, key := range []string{
		keys.Schedule, keys.Recovery, keys.Receipts, keys.Guards,
		keys.Balances["@source#default"].Balance, keys.Balances["@source#overdraft"].Balance, deleted,
	} {
		require.True(t, inspector.Expire(ctx, key, 45*time.Minute).Val())
	}

	before := captureAdapterState(t, inspector, keys)
	adapter, err := NewAdapter(&integrationClientProvider{client: inspector}, limits)
	require.NoError(t, err)
	result, err := adapter.Execute(ctx, input)
	require.Nil(t, result)
	var failure *core.Failure
	require.ErrorAs(t, err, &failure)
	require.Equal(t, core.FailureBalanceDeleted, failure.Code)
	require.Equal(t, 0, failure.TransactionIndex)
	require.Equal(t, 1, failure.PostingIndex)
	require.Equal(t, "@source#overdraft", failure.BalanceRef)
	require.Equal(t, before, captureAdapterState(t, inspector, keys), "touched deletion refusal must preserve balances, markers, schedule, backup, receipts, guards, and absolute expirations")
}

func TestIntegration_AdapterExecute_RejectsEmptyTransactionsBeforeProvider(t *testing.T) {
	input, limits := richAdapterExecution(t)
	input.Request.Transactions = nil
	input.Guards = nil
	input.CompletionPlans = nil
	provider := &integrationClientProvider{}
	adapter, err := NewAdapter(provider, limits)
	require.NoError(t, err)

	result, err := adapter.Execute(context.Background(), input)
	require.Nil(t, result)
	assertAdapterTechnical(t, err, "invalid_request", false)
	require.Zero(t, provider.calls)
}

func scopedAdapterExecution(t *testing.T, tenantID, organizationID, ledgerID string, amount int64) (command.EngineExecution, Limits) {
	t.Helper()
	input, limits := richAdapterExecution(t)
	input.Request.OrganizationID = uuid.MustParse(organizationID)
	input.Request.LedgerID = uuid.MustParse(ledgerID)
	input.Request.ExecutionID = uuid.NewSHA1(uuid.NameSpaceOID, []byte(tenantID+":"+organizationID+":"+ledgerID+":execution"))
	input.Request.Transactions[0].ID = uuid.NewSHA1(uuid.NameSpaceOID, []byte(tenantID+":"+organizationID+":"+ledgerID+":transaction"))
	input.Request.Transactions[0].Postings[0].Amount = decimal.NewFromInt(amount)
	input.Guards[0].TransactionID = input.Request.Transactions[0].ID
	input.CompletionPlans[0].TransactionID = input.Request.Transactions[0].ID

	payload, err := command.DecodeTransactionCompletionPlan(input.CompletionPlans[0].Payload)
	require.NoError(t, err)
	payload.TenantID = tenantID
	payload.OrganizationID = input.Request.OrganizationID
	payload.LedgerID = input.Request.LedgerID
	payload.ExecutionID = input.Request.ExecutionID
	payload.TransactionID = input.Request.Transactions[0].ID
	payload.OperationSpecs[0].TransactionID = input.Request.Transactions[0].ID
	payload.OperationSpecs[0].RequestedAmount = decimal.NewFromInt(amount)
	payload.OperationSpecs[0].Balance.OrganizationID = organizationID
	payload.OperationSpecs[0].Balance.LedgerID = ledgerID
	input.CompletionPlans[0].Payload = encodeAdapterRecovery(t, &input, *payload)
	require.NoError(t, command.ValidateTransactionCompletion(input))

	return input, limits
}
