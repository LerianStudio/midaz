//go:build integration

// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package engine

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	tmcore "github.com/LerianStudio/lib-commons/v7/commons/tenant-manager/core"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"

	core "github.com/LerianStudio/midaz/v4/components/ledger/internal/domain/accounting"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/services/command"
)

type atomicBatchBenchmarkHook struct {
	mu      sync.Mutex
	started map[redis.Cmder]time.Time
	samples []time.Duration
}

func (hook *atomicBatchBenchmarkHook) DialHook(next redis.DialHook) redis.DialHook { return next }

func (hook *atomicBatchBenchmarkHook) ProcessHook(next redis.ProcessHook) redis.ProcessHook {
	return func(ctx context.Context, cmd redis.Cmder) error {
		name := strings.ToUpper(cmd.Name())
		if name != "EVALSHA" && name != "EVAL" {
			return next(ctx, cmd)
		}

		started := time.Now()
		err := next(ctx, cmd)
		if err == nil {
			hook.mu.Lock()
			hook.samples = append(hook.samples, time.Since(started))
			hook.mu.Unlock()
		}

		return err
	}
}

func (hook *atomicBatchBenchmarkHook) ProcessPipelineHook(next redis.ProcessPipelineHook) redis.ProcessPipelineHook {
	return next
}

func (hook *atomicBatchBenchmarkHook) reset() {
	hook.mu.Lock()
	defer hook.mu.Unlock()
	hook.samples = nil
}

func (hook *atomicBatchBenchmarkHook) snapshot() []time.Duration {
	hook.mu.Lock()
	defer hook.mu.Unlock()

	return append([]time.Duration(nil), hook.samples...)
}

type atomicBatchBenchmarkCase struct {
	transactions int
	shared       bool
	warm         bool
	feeExpanded  bool
	tenantMix    bool
	concurrent   bool
}

const (
	atomicBatchBenchmarkMaxPostings = 100
	atomicBatchBenchmarkMaxBalances = 150
)

// BenchmarkAtomicTransactionBatchMatrix is the release-gate workload. It runs
// the production adapter and Lua against real Valkey and publishes both the
// indivisible script percentiles (Redis command hook) and the adapter end-to-end
// percentiles. Every fee-expanded/disjoint case carries the published maxima of
// 100 postings and 150 balances, independent of N. ReportAllocs supplies Go B/op
// and allocs/op; the custom metrics add serialized sizes, Valkey memory and
// unrelated singular-request latency.
func BenchmarkAtomicTransactionBatchMatrix(b *testing.B) {
	ctx := context.Background()
	inspector, address, password := newAdapterValkey(b)
	client := redis.NewClient(&redis.Options{Addr: address, Password: password, DB: 2, Protocol: 2, MaxRetries: -1})
	b.Cleanup(func() { require.NoError(b, client.Close()) })
	hook := &atomicBatchBenchmarkHook{}
	client.AddHook(hook)
	adapter, err := newAdapterWithLimits(&integrationClientProvider{client: client}, hardLimits())
	require.NoError(b, err)
	singularAdapter, err := newAdapterWithLimits(&integrationClientProvider{client: inspector}, hardLimits())
	require.NoError(b, err)

	for _, transactions := range []int{1, 10, 25, 50} {
		for _, shared := range []bool{true, false} {
			for _, warm := range []bool{true, false} {
				for _, feeExpanded := range []bool{false, true} {
					for _, tenantMix := range []bool{false, true} {
						for _, concurrent := range []bool{false, true} {
							cfg := atomicBatchBenchmarkCase{transactions, shared, warm, feeExpanded, tenantMix, concurrent}
							b.Run(cfg.name(), func(b *testing.B) {
								benchmarkAtomicTransactionBatchCase(b, ctx, inspector, adapter, singularAdapter, hook, cfg)
							})
						}
					}
				}
			}
		}
	}
}

func (cfg atomicBatchBenchmarkCase) name() string {
	shape := "disjoint"
	if cfg.shared {
		shape = "shared"
	}
	cache := "cold"
	if cfg.warm {
		cache = "warm"
	}
	fees := "base"
	if cfg.feeExpanded {
		fees = "fee_max"
	}
	tenants := "single_tenant"
	if cfg.tenantMix {
		tenants = "tenant_mix"
	}
	traffic := "isolated"
	if cfg.concurrent {
		traffic = "concurrent_singular"
	}

	return fmt.Sprintf("n_%d/%s/%s/%s/%s/%s", cfg.transactions, shape, cache, fees, tenants, traffic)
}

func benchmarkAtomicTransactionBatchCase(
	b *testing.B,
	ctx context.Context,
	inspector *redis.Client,
	adapter *Adapter,
	singularAdapter *Adapter,
	hook *atomicBatchBenchmarkHook,
	cfg atomicBatchBenchmarkCase,
) {
	b.Helper()
	require.NoError(b, inspector.FlushDB(ctx).Err())

	tenantIDs := []string{""}
	if cfg.tenantMix {
		tenantIDs = []string{"benchmark-tenant-a", "benchmark-tenant-b", "benchmark-tenant-c", "benchmark-tenant-d"}
	}
	for index, tenantID := range tenantIDs {
		warmCtx := atomicBatchBenchmarkTenantContext(ctx, tenantID)
		warmInput := atomicBatchBenchmarkExecution(b, cfg, tenantID, -100-index, "batch")
		_, err := adapter.Execute(warmCtx, warmInput)
		require.NoError(b, err)
		if !cfg.warm {
			require.NoError(b, inspector.FlushDB(ctx).Err())
		}
	}

	representativeTenant := tenantIDs[0]
	representative := atomicBatchBenchmarkExecution(b, cfg, representativeTenant, 0, "batch")
	resolved, err := resolveAdapterKeys(atomicBatchBenchmarkTenantContext(ctx, representativeTenant), representative.Execution)
	require.NoError(b, err)
	prepared, err := prepareExecution(atomicBatchBenchmarkTenantContext(ctx, representativeTenant), representative, hardLimits(), resolved)
	require.NoError(b, err)
	requestBytes, err := json.Marshal(representative)
	require.NoError(b, err)
	recoveryBytes := 0
	for _, plan := range representative.CompletionPlans {
		recoveryBytes += len(plan.Payload)
	}

	hook.reset()
	e2e := make([]time.Duration, 0, b.N)
	unrelated := make([]time.Duration, 0, b.N)
	b.ReportAllocs()
	b.ResetTimer()
	for iteration := 0; iteration < b.N; iteration++ {
		b.StopTimer()
		if !cfg.warm {
			require.NoError(b, inspector.FlushDB(ctx).Err())
		}
		tenantID := tenantIDs[iteration%len(tenantIDs)]
		iterationCtx := atomicBatchBenchmarkTenantContext(ctx, tenantID)
		input := atomicBatchBenchmarkExecution(b, cfg, tenantID, iteration+1, "batch")
		var singularInput command.EngineExecution
		if cfg.concurrent {
			singularCfg := cfg
			singularCfg.transactions = 1
			singularCfg.shared = false
			singularCfg.feeExpanded = false
			singularInput = atomicBatchBenchmarkExecution(b, singularCfg, tenantID, iteration+1, "singular")
		}
		b.StartTimer()

		type singularResult struct {
			duration time.Duration
			err      error
		}
		var singularDone chan singularResult
		if cfg.concurrent {
			singularDone = make(chan singularResult, 1)
			go func() {
				started := time.Now()
				_, singularErr := singularAdapter.Execute(iterationCtx, singularInput)
				singularDone <- singularResult{duration: time.Since(started), err: singularErr}
			}()
		}

		started := time.Now()
		result, err := adapter.Execute(iterationCtx, input)
		e2e = append(e2e, time.Since(started))
		require.NoError(b, err)
		require.NotNil(b, result)
		if singularDone != nil {
			singular := <-singularDone
			require.NoError(b, singular.err)
			unrelated = append(unrelated, singular.duration)
		}
	}
	b.StopTimer()

	atomicBatchReportPercentiles(b, "e2e", e2e)
	atomicBatchReportPercentiles(b, "lua", hook.snapshot())
	atomicBatchReportPercentiles(b, "unrelated", unrelated)
	b.ReportMetric(float64(len(prepared.Payload)), "prepared_wire_bytes")
	b.ReportMetric(float64(len(requestBytes)), "engine_input_bytes")
	b.ReportMetric(float64(recoveryBytes), "recovery_bytes")
	if response, marshalErr := json.Marshal(atomicBatchBenchmarkResultEnvelope(cfg, representative)); marshalErr == nil {
		b.ReportMetric(float64(len(response)), "prepared_response_bytes")
	}
	if usedMemory, memoryErr := atomicBatchBenchmarkRedisMemory(ctx, inspector); memoryErr == nil {
		b.ReportMetric(float64(usedMemory), "redis_memory_bytes")
	}
}

func atomicBatchBenchmarkExecution(
	tb testing.TB,
	cfg atomicBatchBenchmarkCase,
	tenantID string,
	iteration int,
	traffic string,
) command.EngineExecution {
	tb.Helper()

	postingsPerTransaction := 2
	if cfg.feeExpanded {
		postingsPerTransaction = atomicBatchBenchmarkMaxPostings / cfg.transactions
	}
	scopeSeed := fmt.Sprintf("atomic-batch:%s:%s:%s", cfg.name(), tenantID, traffic)
	executionSeed := fmt.Sprintf("%s:%d", scopeSeed, iteration)
	organizationID := uuid.NewSHA1(uuid.NameSpaceOID, []byte(scopeSeed+":organization"))
	ledgerID := uuid.NewSHA1(uuid.NameSpaceOID, []byte(scopeSeed+":ledger"))
	executionID := uuid.NewSHA1(uuid.NameSpaceOID, []byte(executionSeed+":execution"))
	request := core.Execution{OrganizationID: organizationID, LedgerID: ledgerID, ExecutionID: executionID}

	balanceIndex := make(map[string]int)
	addBalance := func(alias string) int {
		if index, ok := balanceIndex[alias]; ok {
			return index
		}
		index := len(request.Balances)
		balanceIndex[alias] = index
		request.Balances = append(request.Balances, core.BalanceSnapshot{
			ID:         uuid.NewSHA1(uuid.NameSpaceOID, []byte(scopeSeed+":balance:"+alias)),
			AccountID:  uuid.NewSHA1(uuid.NameSpaceOID, []byte(scopeSeed+":account:"+alias)),
			BalanceRef: alias + "#default", Alias: alias, Key: "default", AccountType: "deposit",
			AssetCode: "BRL", Direction: "credit", BalanceScope: "transactional",
			Available: decimal.NewFromInt(1_000_000_000), Version: 1, AllowSending: true, AllowReceiving: true,
		})

		return index
	}

	request.Transactions = make([]core.Transaction, cfg.transactions)
	guards := make([]command.ExecutionGuard, cfg.transactions)
	plans := make([]command.TransactionCompletionPlan, cfg.transactions)
	intents := make([]command.EngineTransactionIntent, cfg.transactions)
	date := benchmarkDate()
	for transactionIndex := 0; transactionIndex < cfg.transactions; transactionIndex++ {
		transactionID := uuid.NewSHA1(uuid.NameSpaceOID, []byte(fmt.Sprintf("%s:transaction:%d", executionSeed, transactionIndex)))
		transaction := core.Transaction{ID: transactionID, Postings: make([]core.Posting, postingsPerTransaction)}
		projections := make([]command.OperationRecordSpec, postingsPerTransaction)
		for postingIndex := 0; postingIndex < postingsPerTransaction; postingIndex++ {
			owner := transactionIndex
			if cfg.shared {
				owner = 0
			}
			alias := fmt.Sprintf("@%s-%d-%d", traffic, owner, postingIndex)
			balance := request.Balances[addBalance(alias)]
			postingType := core.PostingDebit
			rowType := "DEBIT"
			direction := "credit"
			if postingIndex%2 == 1 {
				postingType = core.PostingCredit
				rowType = "CREDIT"
				direction = "debit"
			}
			posting := core.Posting{
				Ref:        fmt.Sprintf("t%02d-p%02d", transactionIndex, postingIndex),
				BalanceRef: balance.BalanceRef, Type: postingType, Amount: decimal.NewFromInt(1), DrawPolicy: core.DrawForbidden,
			}
			transaction.Postings[postingIndex] = posting
			projections[postingIndex] = command.OperationRecordSpec{
				TransactionID: transactionID, PostingRef: posting.Ref, BalanceRef: balance.BalanceRef,
				Role: core.RolePrimary, Side: command.OperationSpecSideFrom, RowType: rowType,
				Direction: direction, RequestedAmount: posting.Amount, CompatibilityPath: command.OperationRecordStandard,
			}
			projections[postingIndex].Balance.ID = balance.ID.String()
			projections[postingIndex].Balance.AccountID = balance.AccountID.String()
			projections[postingIndex].Balance.OrganizationID = organizationID.String()
			projections[postingIndex].Balance.LedgerID = ledgerID.String()
			projections[postingIndex].Balance.Alias = balance.Alias
			projections[postingIndex].Balance.Key = balance.Key
			projections[postingIndex].Balance.AssetCode = balance.AssetCode
			projections[postingIndex].Balance.AccountType = balance.AccountType
			projections[postingIndex].Balance.Available = balance.Available
			projections[postingIndex].Balance.Version = balance.Version
		}
		request.Transactions[transactionIndex] = transaction
		guards[transactionIndex] = command.ExecutionGuard{TransactionID: transactionID, NextToken: "approved"}
		plans[transactionIndex] = command.TransactionCompletionPlan{
			FormatVersion: 2, TenantID: tenantID, TransactionID: transactionID,
			OrganizationID: organizationID, LedgerID: ledgerID, ExecutionID: executionID,
			TTL: date.Add(time.Hour), TransactionDate: date, TransactionCreatedAt: date,
			TransactionUpdatedAt: date, OperationUpdatedAt: date, Action: "CREATE",
			TransactionStatus: "APPROVED", OperationSpecs: projections,
		}
		intents[transactionIndex] = command.EngineTransactionIntent{
			TransactionID: transactionID, Action: "CREATE", TransactionStatus: "APPROVED",
			TransactionDate: date, TransactionCreatedAt: date, TransactionUpdatedAt: date,
			OperationUpdatedAt: date, PostingRefs: postingRefs(transaction.Postings),
			OperationSpecs: frozenProjectionIntents(projections),
		}
	}
	// Maximum fee expansion is also the final release profile: every explicit
	// disjoint balance carries an overdraft companion snapshot, reaching the
	// published 150-balance ceiling without inventing extra postings or touching
	// the companions.
	if cfg.feeExpanded && !cfg.shared {
		primaryCount := len(request.Balances)
		companionCount := atomicBatchBenchmarkMaxBalances - primaryCount
		for index := 0; index < companionCount; index++ {
			companion := request.Balances[index]
			companion.ID = uuid.NewSHA1(uuid.NameSpaceOID, []byte(scopeSeed+":overdraft:"+companion.Alias))
			companion.BalanceRef = companion.Alias + "#overdraft"
			companion.Key = "overdraft"
			companion.Direction = "debit"
			companion.BalanceScope = "internal"
			companion.Available = decimal.Zero
			companion.AllowOverdraft = false
			companion.OverdraftLimitEnabled = false
			request.Balances = append(request.Balances, companion)
		}
	}
	if cfg.feeExpanded {
		require.Len(tb, postingRefsForExecution(request), atomicBatchBenchmarkMaxPostings)
		if !cfg.shared {
			require.Len(tb, request.Balances, atomicBatchBenchmarkMaxBalances)
		}
	}
	fingerprint, err := command.ComputeEngineIntentFingerprint(command.EngineIntent{
		TenantID: tenantID, OrganizationID: organizationID, LedgerID: ledgerID,
		ExecutionID: executionID, Transactions: intents,
	})
	require.NoError(tb, err)
	input := command.EngineExecution{
		Execution: request, IntentFingerprint: fingerprint, Guards: guards,
		CompletionPlans: make([]command.CompletionPlanRecord, cfg.transactions),
	}
	for index := range plans {
		plans[index].IntentFingerprint = fingerprint
		payload, encodeErr := command.EncodeTransactionCompletionPlan(plans[index])
		require.NoError(tb, encodeErr)
		input.CompletionPlans[index] = command.CompletionPlanRecord{TransactionID: plans[index].TransactionID, Payload: payload}
	}
	require.NoError(tb, command.ValidateTransactionCompletion(input))

	return input
}

func postingRefsForExecution(request core.Execution) []string {
	refs := make([]string, 0)
	for _, transaction := range request.Transactions {
		refs = append(refs, postingRefs(transaction.Postings)...)
	}

	return refs
}

func atomicBatchBenchmarkTenantContext(ctx context.Context, tenantID string) context.Context {
	if tenantID == "" {
		return ctx
	}

	return tmcore.ContextWithTenantID(ctx, tenantID)
}

func atomicBatchReportPercentiles(b *testing.B, prefix string, samples []time.Duration) {
	b.Helper()
	if len(samples) == 0 {
		return
	}
	sort.Slice(samples, func(i, j int) bool { return samples[i] < samples[j] })
	percentile := func(value float64) float64 {
		index := int(value * float64(len(samples)-1))

		return float64(samples[index]) / float64(time.Millisecond)
	}
	b.ReportMetric(percentile(0.50), prefix+"_p50_ms")
	b.ReportMetric(percentile(0.95), prefix+"_p95_ms")
	b.ReportMetric(percentile(0.99), prefix+"_p99_ms")
}

func atomicBatchBenchmarkRedisMemory(ctx context.Context, client *redis.Client) (int64, error) {
	info, err := client.Info(ctx, "memory").Result()
	if err != nil {
		return 0, err
	}
	for _, line := range strings.Split(info, "\n") {
		value, ok := strings.CutPrefix(strings.TrimSpace(line), "used_memory:")
		if !ok {
			continue
		}

		return strconv.ParseInt(value, 10, 64)
	}

	return 0, fmt.Errorf("Valkey INFO memory omitted used_memory")
}

func atomicBatchBenchmarkResultEnvelope(
	cfg atomicBatchBenchmarkCase,
	input command.EngineExecution,
) any {
	return struct {
		TransactionCount int                            `json:"transactionCount"`
		Transactions     []core.Transaction             `json:"transactions"`
		CompletionPlans  []command.CompletionPlanRecord `json:"completionPlans"`
	}{
		TransactionCount: cfg.transactions,
		Transactions:     input.Execution.Transactions,
		CompletionPlans:  input.CompletionPlans,
	}
}

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
