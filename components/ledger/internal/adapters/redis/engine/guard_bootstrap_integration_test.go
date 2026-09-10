//go:build integration

// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package engine

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	tmcore "github.com/LerianStudio/lib-commons/v6/commons/tenant-manager/core"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"

	core "github.com/LerianStudio/midaz/v4/components/ledger/internal/domain/accounting"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/services/command"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	"github.com/LerianStudio/midaz/v4/pkg/mtransaction"
)

type guardBootstrapHook struct {
	hsetnx  atomic.Int32
	noRetry atomic.Int32
}

type guardClientProvider struct {
	client *redis.Client
}

func (provider guardClientProvider) GetClient(context.Context) (redis.UniversalClient, error) {
	return provider.client, nil
}

func (hook *guardBootstrapHook) DialHook(next redis.DialHook) redis.DialHook {
	return next
}

func (hook *guardBootstrapHook) ProcessHook(next redis.ProcessHook) redis.ProcessHook {
	return func(ctx context.Context, cmd redis.Cmder) error {
		if strings.EqualFold(cmd.Name(), "hsetnx") {
			hook.hsetnx.Add(1)
			if cmd.NoRetry() {
				hook.noRetry.Add(1)
			}
		}

		return next(ctx, cmd)
	}
}

func (hook *guardBootstrapHook) ProcessPipelineHook(next redis.ProcessPipelineHook) redis.ProcessPipelineHook {
	return next
}

func TestIntegrationEnsureTransactionGuardIsConditionalAndPersistent(t *testing.T) {
	ctx := context.Background()
	client, _, _ := newAdapterValkey(t)
	hook := &guardBootstrapHook{}
	client.AddHook(hook)
	adapter, err := newAdapterWithLimits(guardClientProvider{client: client}, guardBootstrapLimits())
	require.NoError(t, err)
	organizationID := uuid.MustParse("11111111-1111-4111-8111-111111111111")
	ledgerID := uuid.MustParse("22222222-2222-4222-8222-222222222222")
	transactionID := uuid.MustParse("33333333-3333-4333-8333-333333333333")

	const calls = 16
	errorsByCall := make(chan error, calls)
	var group sync.WaitGroup
	for range calls {
		group.Add(1)
		go func() {
			defer group.Done()
			errorsByCall <- adapter.EnsureTransactionGuard(ctx, organizationID, ledgerID, transactionID, constant.PENDING)
		}()
	}
	group.Wait()
	close(errorsByCall)
	for ensureErr := range errorsByCall {
		require.NoError(t, ensureErr)
	}

	key, err := resolveGuardKey(ctx, organizationID, ledgerID)
	require.NoError(t, err)
	guard, err := client.HGet(ctx, key, transactionID.String()).Result()
	require.NoError(t, err)
	require.Equal(t, constant.PENDING, guard)
	require.Equal(t, time.Duration(-1), client.TTL(ctx, key).Val(), "shared guard hash must not acquire a TTL")
	require.Equal(t, int32(calls), hook.hsetnx.Load())
	require.Equal(t, hook.hsetnx.Load(), hook.noRetry.Load(), "every mutating guard command must disable client retries")
	seedDump, seedExpiry := guardHashState(t, ctx, client, key)
	require.NoError(t, adapter.EnsureTransactionGuard(ctx, organizationID, ledgerID, transactionID, constant.PENDING))
	repeatedDump, repeatedExpiry := guardHashState(t, ctx, client, key)
	require.Equal(t, seedDump, repeatedDump, "idempotent bootstrap must not rewrite the guard hash")
	require.Equal(t, seedExpiry, repeatedExpiry)

	terminalIDs := map[uuid.UUID]string{
		uuid.MustParse("34444444-4444-4444-8444-444444444444"): constant.APPROVED,
		uuid.MustParse("35555555-5555-4555-8555-555555555555"): constant.CANCELED,
	}
	for id, terminal := range terminalIDs {
		require.NoError(t, client.HSet(ctx, key, id.String(), terminal).Err())
	}
	terminalDump, terminalExpiry := guardHashState(t, ctx, client, key)
	for id := range terminalIDs {
		require.NoError(t, adapter.EnsureTransactionGuard(ctx, organizationID, ledgerID, id, constant.PENDING))
	}
	preservedDump, preservedExpiry := guardHashState(t, ctx, client, key)
	require.Equal(t, terminalDump, preservedDump, "bootstrap must never rewrite terminal guards")
	require.Equal(t, terminalExpiry, preservedExpiry)
	for id, terminal := range terminalIDs {
		guard, err = client.HGet(ctx, key, id.String()).Result()
		require.NoError(t, err)
		require.Equal(t, terminal, guard)
	}
}

func TestIntegrationEnsureTransactionGuardIsolatesAuthenticatedScope(t *testing.T) {
	client, _, _ := newAdapterValkey(t)
	adapter, err := newAdapterWithLimits(guardClientProvider{client: client}, guardBootstrapLimits())
	require.NoError(t, err)
	transactionID := uuid.MustParse("63333333-3333-4333-8333-333333333333")

	tests := []struct {
		tenant, organization, ledger, token string
	}{
		{"tenant-a", "61111111-1111-4111-8111-111111111111", "62222222-2222-4222-8222-222222222222", "tenant-a-org-a-ledger-a"},
		{"tenant-a", "61111111-1111-4111-8111-111111111111", "62444444-4444-4444-8444-444444444444", "tenant-a-org-a-ledger-b"},
		{"tenant-a", "61555555-5555-4555-8555-555555555555", "62222222-2222-4222-8222-222222222222", "tenant-a-org-b-ledger-a"},
		{"tenant-b", "61111111-1111-4111-8111-111111111111", "62222222-2222-4222-8222-222222222222", "tenant-b-org-a-ledger-a"},
	}
	keys := make(map[string]bool, len(tests))
	for _, test := range tests {
		ctx := tmcore.ContextWithTenantID(context.Background(), test.tenant)
		organizationID := uuid.MustParse(test.organization)
		ledgerID := uuid.MustParse(test.ledger)
		require.NoError(t, adapter.EnsureTransactionGuard(ctx, organizationID, ledgerID, transactionID, test.token))
		key, resolveErr := resolveGuardKey(ctx, organizationID, ledgerID)
		require.NoError(t, resolveErr)
		require.False(t, keys[key], "tenant, organization, and ledger must define distinct guard hashes")
		keys[key] = true
		guard, getErr := client.HGet(ctx, key, transactionID.String()).Result()
		require.NoError(t, getErr)
		require.Equal(t, test.token, guard)
	}
}

func TestIntegrationEnsureTransactionGuardFencesCommitAndCancel(t *testing.T) {
	ctx := context.Background()
	client, _, _ := newAdapterValkey(t)
	adapter, err := newAdapterWithLimits(guardClientProvider{client: client}, guardBootstrapLimits())
	require.NoError(t, err)
	organizationID := uuid.MustParse("41111111-1111-4111-8111-111111111111")
	ledgerID := uuid.MustParse("42222222-2222-4222-8222-222222222222")
	transactionID := uuid.MustParse("43333333-3333-4333-8333-333333333333")
	balances, snapshots := guardTransitionBalances(t, organizationID, ledgerID)
	commit := guardTransitionExecution(t, organizationID, ledgerID, transactionID, constant.ActionCommit, constant.APPROVED, balances, snapshots)
	cancel := guardTransitionExecution(t, organizationID, ledgerID, transactionID, constant.ActionCancel, constant.CANCELED, balances, snapshots)

	require.NoError(t, adapter.EnsureTransactionGuard(ctx, organizationID, ledgerID, transactionID, constant.PENDING))

	type outcome struct {
		next   string
		result *core.ExecutionResult
		err    error
	}
	start := make(chan struct{})
	outcomes := make(chan outcome, 2)
	for _, contender := range []struct {
		next  string
		input command.EngineExecution
	}{
		{next: constant.APPROVED, input: commit},
		{next: constant.CANCELED, input: cancel},
	} {
		go func(next string, input command.EngineExecution) {
			<-start
			result, executeErr := adapter.Execute(ctx, input)
			outcomes <- outcome{next: next, result: result, err: executeErr}
		}(contender.next, contender.input)
	}
	close(start)

	winnerNext := ""
	failures := 0
	for range 2 {
		candidate := <-outcomes
		if candidate.err == nil {
			winnerNext = candidate.next
			require.NotNil(t, candidate.result)
			continue
		}

		failures++
		require.ErrorContains(t, candidate.err, "execution_guard_conflict")
		require.Nil(t, candidate.result)
	}
	require.NotEmpty(t, winnerNext)
	require.Equal(t, 1, failures)

	key, err := resolveGuardKey(ctx, organizationID, ledgerID)
	require.NoError(t, err)
	guard, err := client.HGet(ctx, key, transactionID.String()).Result()
	require.NoError(t, err)
	require.Equal(t, winnerNext, guard)
	assertGuardWinnerBalance(t, ctx, client, commit.Execution, winnerNext)
}

func guardBootstrapLimits() Limits {
	return Limits{
		MaxTransactions: 2, MaxPostings: 8, MaxBalances: 8,
		MaxCompletionPlanBytes: 1 << 20, MaxRequestBytes: 1 << 20, MaxPreparedBytes: 1 << 20,
	}
}

func guardHashState(t *testing.T, ctx context.Context, client *redis.Client, key string) (string, int64) {
	t.Helper()
	dump, err := client.Dump(ctx, key).Result()
	require.NoError(t, err)
	expiry, err := client.Do(ctx, "PEXPIRETIME", key).Int64()
	require.NoError(t, err)

	return dump, expiry
}

func guardTransitionBalances(t *testing.T, organizationID, ledgerID uuid.UUID) ([]*mmodel.Balance, []core.BalanceSnapshot) {
	t.Helper()
	balances := []*mmodel.Balance{
		{
			ID: "44444444-4444-4444-8444-444444444444", OrganizationID: organizationID.String(), LedgerID: ledgerID.String(),
			AccountID: "45555555-5555-4555-8555-555555555555", Alias: "@source", Key: constant.DefaultBalanceKey,
			AssetCode: "USD", Available: decimal.NewFromInt(40), OnHold: decimal.NewFromInt(60), Version: 7,
			AccountType: "deposit", Direction: constant.DirectionCredit, AllowSending: true, AllowReceiving: true,
		},
		{
			ID: "46666666-6666-4666-8666-666666666666", OrganizationID: organizationID.String(), LedgerID: ledgerID.String(),
			AccountID: "47777777-7777-4777-8777-777777777777", Alias: "@destination", Key: constant.DefaultBalanceKey,
			AssetCode: "USD", Version: 3, AccountType: "deposit", Direction: constant.DirectionCredit,
			AllowSending: true, AllowReceiving: true,
		},
	}
	aliases := []string{"@source#default", "@destination#default"}
	pool, err := command.LoadBalanceEngineSnapshotPool(context.Background(), organizationID, ledgerID, aliases,
		func(_ context.Context, _, _ uuid.UUID, requested []string) ([]*mmodel.Balance, error) {
			for _, alias := range requested {
				if strings.HasSuffix(alias, "#"+constant.OverdraftBalanceKey) {
					return nil, nil
				}
			}

			return balances, nil
		})
	require.NoError(t, err)

	return balances, pool.Snapshots
}

func guardTransitionExecution(
	t *testing.T,
	organizationID, ledgerID, transactionID uuid.UUID,
	action, status string,
	balances []*mmodel.Balance,
	snapshots []core.BalanceSnapshot,
) command.EngineExecution {
	t.Helper()
	amount := mtransaction.Amount{Asset: "USD", Value: decimal.NewFromInt(60), TransactionType: status}
	input := mtransaction.Transaction{Send: mtransaction.Send{
		Asset: "USD", Value: decimal.NewFromInt(60),
		Source: mtransaction.Source{From: []mtransaction.FromTo{{
			AccountAlias: "0#@source#default", BalanceKey: constant.DefaultBalanceKey, IsFrom: true,
		}}},
		Distribute: mtransaction.Distribute{To: []mtransaction.FromTo{{
			AccountAlias: "0#@destination#default", BalanceKey: constant.DefaultBalanceKey,
		}}},
	}}
	validate := &mtransaction.Responses{
		Asset: "USD", From: map[string]mtransaction.Amount{"0#@source#default": amount},
		To: map[string]mtransaction.Amount{"0#@destination#default": amount},
	}
	translated, projection, err := command.TranslateBalanceEngineTransaction(command.BalanceEngineTranslationInput{
		TransactionID: transactionID, Action: action, TransactionStatus: status,
		TransactionInput: input, Validate: validate, Balances: balances,
	})
	require.NoError(t, err)
	date := time.Date(2026, time.September, 8, 15, 0, 0, 0, time.UTC)
	executionID := uuid.NewSHA1(transactionID, []byte(action))
	payload := command.TransactionCompletionPlan{
		FormatVersion: 2, TransactionID: transactionID, OrganizationID: organizationID, LedgerID: ledgerID,
		ExecutionID: executionID, TransactionInput: input, Validate: validate, Action: action, TransactionStatus: status,
		TTL: date, TransactionDate: date, TransactionCreatedAt: date, TransactionUpdatedAt: date, OperationUpdatedAt: date,
		OperationSpecs: projection,
	}
	execution := command.EngineExecution{
		Execution: core.Execution{
			OrganizationID: organizationID, LedgerID: ledgerID, ExecutionID: executionID,
			Transactions: []core.Transaction{translated}, Balances: snapshots,
		},
		Guards: []command.ExecutionGuard{{TransactionID: transactionID, ExpectedToken: constant.PENDING, NextToken: status}},
	}
	execution.CompletionPlans = []command.CompletionPlanRecord{{TransactionID: transactionID, Payload: encodeAdapterRecovery(t, &execution, payload)}}
	require.NoError(t, command.ValidateTransactionCompletion(execution))

	return execution
}

func assertGuardWinnerBalance(t *testing.T, ctx context.Context, client *redis.Client, request core.Execution, winnerNext string) {
	t.Helper()
	keys, err := resolveAdapterKeys(ctx, request)
	require.NoError(t, err)
	raw, err := client.Get(ctx, keys.Balances["@source#default"].Balance).Bytes()
	require.NoError(t, err)
	var state map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(raw, &state))
	wantAvailable := `"40"`
	if winnerNext == constant.CANCELED {
		wantAvailable = `"100"`
	}
	require.JSONEq(t, wantAvailable, string(state["available"]))
	require.JSONEq(t, `"0"`, string(state["onHold"]))
}
