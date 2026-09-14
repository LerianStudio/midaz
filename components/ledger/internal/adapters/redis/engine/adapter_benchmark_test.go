//go:build integration

// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package engine

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"

	core "github.com/LerianStudio/midaz/v4/components/ledger/internal/domain/accounting"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/services/command"
)

// BenchmarkAdapterExecute characterizes the public adapter seam against a real
// Valkey server. Every measured execution starts from an empty Valkey database; the
// larger pool is input carried through Execute, not a cache lookup benchmark.
func BenchmarkAdapterExecute(b *testing.B) {
	for _, postings := range []int{2, 10, 50} {
		for _, pool := range []string{"touched", "larger"} {
			b.Run(fmt.Sprintf("postings_%d/pool_%s", postings, pool), func(b *testing.B) {
				ctx := context.Background()
				inspector, address, password := newAdapterValkey(b)
				balanceCount := postings
				if pool == "larger" {
					balanceCount = 2 * postings
				}
				client := redis.NewClient(&redis.Options{Addr: address, Password: password, DB: 2, Protocol: 2})
				b.Cleanup(func() { require.NoError(b, client.Close()) })
				limits := Limits{MaxTransactions: 4, MaxPostings: 64, MaxBalances: 128, MaxCompletionPlanBytes: 1 << 20, MaxRequestBytes: 1 << 20, MaxPreparedBytes: 1 << 20}
				adapter, err := newAdapterWithLimits(&integrationClientProvider{client: client}, limits)
				require.NoError(b, err)

				warmup := benchmarkExecution(b, postings, balanceCount, -1)
				require.Len(b, warmup.Execution.Balances, balanceCount)
				warmResult, err := adapter.Execute(ctx, warmup)
				require.NoError(b, err)
				assertBenchmarkResult(b, warmResult, postings)
				require.NoError(b, inspector.FlushDB(ctx).Err())

				measurementInput := benchmarkExecution(b, postings, balanceCount, 0)
				resolved, err := resolveAdapterKeys(ctx, measurementInput.Execution)
				require.NoError(b, err)
				prepared, err := prepareExecution(ctx, measurementInput, limits, resolved)
				require.NoError(b, err)
				b.ReportAllocs()
				b.ResetTimer()
				b.ReportMetric(float64(len(prepared.Payload)), "prepared_wire_bytes")
				for i := 0; i < b.N; i++ {
					b.StopTimer()
					input := benchmarkExecution(b, postings, balanceCount, i)
					require.Len(b, input.Execution.Balances, balanceCount)
					// Each iteration has unique IDs and empty Valkey state, so this
					// cannot hit an execution receipt or reuse live balance state.
					require.NoError(b, inspector.FlushDB(ctx).Err())
					b.StartTimer()
					result, err := adapter.Execute(ctx, input)
					b.StopTimer()
					require.NoError(b, err)
					assertBenchmarkResult(b, result, postings)
					assertUntouchedBenchmarkBalances(b, ctx, inspector, input, postings)
				}
			})
		}
	}
}

func benchmarkExecution(tb testing.TB, postingCount, balanceCount, iteration int) command.EngineExecution {
	tb.Helper()
	require.GreaterOrEqual(tb, balanceCount, postingCount)
	ns := uuid.NameSpaceOID
	request := core.Execution{
		ExecutionID:    uuid.NewSHA1(ns, []byte(fmt.Sprintf("benchmark:execution:%d:%d", postingCount, iteration))),
		OrganizationID: uuid.NewSHA1(ns, []byte("benchmark:organization")),
		LedgerID:       uuid.NewSHA1(ns, []byte("benchmark:ledger")),
		Transactions:   []core.Transaction{{ID: uuid.NewSHA1(ns, []byte(fmt.Sprintf("benchmark:transaction:%d:%d", postingCount, iteration)))}},
	}
	request.Transactions[0].Postings = make([]core.Posting, postingCount)
	request.Balances = make([]core.BalanceSnapshot, balanceCount)
	for i := 0; i < balanceCount; i++ {
		balanceRef := fmt.Sprintf("@source-%02d#default", i)
		request.Balances[i] = core.BalanceSnapshot{
			ID: uuid.NewSHA1(ns, []byte(fmt.Sprintf("benchmark:balance:%d", i))), AccountID: uuid.NewSHA1(ns, []byte(fmt.Sprintf("benchmark:account:%d", i))),
			BalanceRef: balanceRef, Alias: fmt.Sprintf("@source-%02d", i), Key: "default", AccountType: "deposit", AssetCode: "BRL", Direction: "credit", BalanceScope: "transactional",
			Available: decimal.NewFromInt(100), OverdraftLimit: decimal.NewFromInt(1000), Version: 7, AllowSending: true, AllowReceiving: true,
		}
	}
	for i := 0; i < postingCount; i++ {
		request.Transactions[0].Postings[i] = core.Posting{Ref: fmt.Sprintf("posting-%02d", i), BalanceRef: request.Balances[i].BalanceRef, Type: core.PostingDebit, Amount: decimal.NewFromInt(1), DrawPolicy: core.DrawForbidden}
	}
	projection := make([]command.OperationRecordSpec, postingCount)
	for i, balance := range request.Balances[:postingCount] {
		projection[i] = command.OperationRecordSpec{TransactionID: request.Transactions[0].ID, PostingRef: request.Transactions[0].Postings[i].Ref, BalanceRef: balance.BalanceRef, Role: core.RolePrimary, Side: command.OperationSpecSideFrom, RowType: "DEBIT", Direction: "credit", RequestedAmount: decimal.NewFromInt(1), CompatibilityPath: command.OperationRecordStandard}
		projection[i].Balance.ID, projection[i].Balance.AccountID = balance.ID.String(), balance.AccountID.String()
		projection[i].Balance.OrganizationID, projection[i].Balance.LedgerID = request.OrganizationID.String(), request.LedgerID.String()
		projection[i].Balance.Alias, projection[i].Balance.Key, projection[i].Balance.AssetCode = balance.Alias, balance.Key, balance.AssetCode
		projection[i].Balance.AccountType = balance.AccountType
		projection[i].Balance.Available, projection[i].Balance.Version = balance.Available, balance.Version
	}
	payload := command.TransactionCompletionPlan{FormatVersion: 2, TransactionID: request.Transactions[0].ID, OrganizationID: request.OrganizationID, LedgerID: request.LedgerID, ExecutionID: request.ExecutionID, TTL: benchmarkDate().Add(24 * 60 * 60 * 1e9), TransactionDate: benchmarkDate(), TransactionCreatedAt: benchmarkDate(), TransactionUpdatedAt: benchmarkDate(), OperationUpdatedAt: benchmarkDate(), Action: "CREATE", TransactionStatus: "APPROVED", OperationSpecs: projection}
	input := command.EngineExecution{Execution: request, Guards: []command.ExecutionGuard{{TransactionID: request.Transactions[0].ID, NextToken: "executed-once"}}}
	input.CompletionPlans = []command.CompletionPlanRecord{{TransactionID: request.Transactions[0].ID, Payload: encodeAdapterRecovery(tb, &input, payload)}}
	date := benchmarkDate()
	fingerprint, err := command.ComputeEngineIntentFingerprint(command.EngineIntent{OrganizationID: request.OrganizationID, LedgerID: request.LedgerID, ExecutionID: request.ExecutionID, Transactions: []command.EngineTransactionIntent{{TransactionID: request.Transactions[0].ID, PostingRefs: postingRefs(request.Transactions[0].Postings), Action: "CREATE", TransactionStatus: "APPROVED", TransactionDate: date, TransactionCreatedAt: date, TransactionUpdatedAt: date, OperationUpdatedAt: date, OperationSpecs: frozenProjectionIntents(projection)}}})
	require.NoError(tb, err)
	input.IntentFingerprint = fingerprint
	return input
}

func assertBenchmarkResult(tb testing.TB, result *core.ExecutionResult, postingCount int) {
	tb.Helper()
	require.Len(tb, result.Movements, postingCount)
	require.Len(tb, result.Final, postingCount)
	for _, balance := range result.Final {
		require.True(tb, balance.Available.Equal(decimal.NewFromInt(99)))
		require.Equal(tb, int64(8), balance.Version)
	}
}

func assertUntouchedBenchmarkBalances(tb testing.TB, ctx context.Context, inspector *redis.Client, input command.EngineExecution, postingCount int) {
	tb.Helper()
	keys, err := resolveAdapterKeys(ctx, input.Execution)
	require.NoError(tb, err)
	for _, balance := range input.Execution.Balances[postingCount:] {
		resolved := keys.Balances[balance.BalanceRef]
		exists, err := inspector.Exists(ctx, resolved.Balance, resolved.Deleted, resolved.LegacyDeleted).Result()
		require.NoError(tb, err)
		require.Zero(tb, exists, "untouched balance %s was persisted", balance.BalanceRef)
	}
}

func benchmarkDate() (d time.Time) { return time.Date(2026, time.September, 4, 12, 0, 0, 0, time.UTC) }

func postingRefs(postings []core.Posting) []string {
	refs := make([]string, len(postings))
	for i := range postings {
		refs[i] = postings[i].Ref
	}
	return refs
}

func frozenProjectionIntents(items []command.OperationRecordSpec) []command.OperationRecordIntent {
	out := make([]command.OperationRecordIntent, len(items))
	for i := range items {
		out[i] = items[i].Intent()
	}
	return out
}
