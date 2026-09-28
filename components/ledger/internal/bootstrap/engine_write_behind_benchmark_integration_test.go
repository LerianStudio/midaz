// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

//go:build integration

package bootstrap

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	tmcore "github.com/LerianStudio/lib-commons/v7/commons/tenant-manager/core"
	"github.com/gofiber/fiber/v3"
	"github.com/google/uuid"
	amqp "github.com/rabbitmq/amqp091-go"
	"github.com/redis/go-redis/v9"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"

	httpin "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/http/in"
	ledgerRabbitMQ "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/rabbitmq"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/services/command"
	postgrestestutil "github.com/LerianStudio/midaz/v4/tests/utils/postgres"
)

type engineWriteBehindBenchmarkHook struct {
	mu      sync.Mutex
	samples []time.Duration
}

func (*engineWriteBehindBenchmarkHook) DialHook(next redis.DialHook) redis.DialHook { return next }

func (hook *engineWriteBehindBenchmarkHook) ProcessHook(next redis.ProcessHook) redis.ProcessHook {
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

func (*engineWriteBehindBenchmarkHook) ProcessPipelineHook(next redis.ProcessPipelineHook) redis.ProcessPipelineHook {
	return next
}

func (hook *engineWriteBehindBenchmarkHook) reset() {
	hook.mu.Lock()
	defer hook.mu.Unlock()

	hook.samples = nil
}

func (hook *engineWriteBehindBenchmarkHook) snapshot() []time.Duration {
	hook.mu.Lock()
	defer hook.mu.Unlock()

	return append([]time.Duration(nil), hook.samples...)
}

type engineWriteBehindBenchmarkMode string

const (
	engineBenchmarkBaselineSync  engineWriteBehindBenchmarkMode = "baseline_sync"
	engineBenchmarkCorrectedSync engineWriteBehindBenchmarkMode = "corrected_sync"
	engineBenchmarkAsyncSingle   engineWriteBehindBenchmarkMode = "async_single"
	engineBenchmarkAsyncBulk     engineWriteBehindBenchmarkMode = "async_bulk"
)

// benchmarkLegacySyncDispatcher reproduces the pre-write-behind boundary: the
// request calls the durable completer and recovery ACK directly before it may
// return. It is benchmark-only and deliberately bypasses the new dependency-
// aware fallback wrapper so the historical synchronous reference remains
// comparable while still using the current HTTP, engine, and persistence stack.
type benchmarkLegacySyncDispatcher struct {
	completer    command.AppliedTransactionCompleter
	acknowledger command.EngineRecoveryAcknowledger
}

func (dispatcher benchmarkLegacySyncDispatcher) DispatchTransactionWriteBehind(
	ctx context.Context,
	envelope *command.TransactionWriteBehindEnvelope,
) error {
	completion, err := dispatcher.completer.Complete(ctx, &envelope.Record)
	if err != nil {
		return err
	}
	if dispatcher.acknowledger == nil {
		return nil
	}

	return dispatcher.acknowledger.AcknowledgeEngineRecovery(ctx, &envelope.Record, completion)
}

type engineWriteBehindBenchmarkCase struct {
	mode      engineWriteBehindBenchmarkMode
	version   string
	size      int
	tenantID  string
	warm      bool
	batchHTTP bool
}

func (cfg engineWriteBehindBenchmarkCase) name() string {
	tenant := "single_tenant"
	if cfg.tenantID != "" {
		tenant = "multi_tenant"
	}
	cache := "cold"
	if cfg.warm {
		cache = "warm"
	}

	return fmt.Sprintf("%s/%s/n_%d/%s/%s", cfg.mode, cfg.version, cfg.size, tenant, cache)
}

func (cfg engineWriteBehindBenchmarkCase) asynchronous() bool {
	return cfg.mode == engineBenchmarkAsyncSingle || cfg.mode == engineBenchmarkAsyncBulk
}

// BenchmarkEngineWriteBehind is the reproducible full-stack write-path harness.
// It exercises Fiber/Huma, the production command path and engine Lua against
// real PostgreSQL, MongoDB, Valkey, and RabbitMQ. Singular requests cover the
// legacy synchronous baseline, corrected synchronous engine projection, and
// confirmed async admission for v1/v2 in ST/MT and warm/cold states. The v2-only
// atomic batch contract covers confirmed async bulk projection at N=1/10/25/50.
func BenchmarkEngineWriteBehind(b *testing.B) {
	if testing.Short() {
		b.Skip("requires PostgreSQL, MongoDB, Valkey, and RabbitMQ")
	}

	b.Setenv("ALLOW_INSECURE_TLS", "true")
	b.Setenv("AUDIT_LOG_ENABLED", "false")
	b.Setenv("RABBITMQ_TRANSACTION_ASYNC", "true")

	infra := setupEngineWriteBehindHTTPIntegration(b)
	tenantIDs := []string{"", "benchmark-tenant"}

	for _, mode := range []engineWriteBehindBenchmarkMode{
		engineBenchmarkBaselineSync,
		engineBenchmarkCorrectedSync,
		engineBenchmarkAsyncSingle,
	} {
		for _, version := range []string{"v1", "v2"} {
			for _, tenantID := range tenantIDs {
				for _, warm := range []bool{false, true} {
					cfg := engineWriteBehindBenchmarkCase{
						mode: mode, version: version, size: 1, tenantID: tenantID, warm: warm,
					}
					b.Run(cfg.name(), func(b *testing.B) { infra.benchmarkCase(b, cfg) })
				}
			}
		}
	}

	for _, size := range []int{1, 10, 25, 50} {
		for _, tenantID := range tenantIDs {
			for _, warm := range []bool{false, true} {
				cfg := engineWriteBehindBenchmarkCase{
					mode: engineBenchmarkAsyncBulk, version: "v2", size: size,
					tenantID: tenantID, warm: warm, batchHTTP: true,
				}
				b.Run(cfg.name(), func(b *testing.B) { infra.benchmarkCase(b, cfg) })
			}
		}
	}
}

func (infra *engineWriteBehindHTTPIntegration) benchmarkCase(b *testing.B, cfg engineWriteBehindBenchmarkCase) {
	b.Helper()
	require.NoError(b, infra.configureBenchmarkMode(cfg))
	app := infra.newHTTPApp(cfg.tenantID)
	_, err := infra.rabbit.Channel.QueuePurge(infra.queue, false)
	require.NoError(b, err)

	var warmAliases engineWriteBehindAliases
	if cfg.warm {
		warmAliases = infra.seedBenchmarkTransfer(b, "warm-"+uuid.NewString())
		infra.executeBenchmarkRequest(b, app, cfg, warmAliases, "warmup-"+uuid.NewString())
		if cfg.asynchronous() {
			infra.drainBenchmarkProjection(b, cfg)
		}
	}

	rowsBefore := infra.benchmarkSQLRows(b)
	memoryBefore := infra.benchmarkRedisMemory(b)
	infra.engineHook.reset()

	httpSamples := make([]time.Duration, 0, b.N)
	projectedSamples := make([]time.Duration, 0, b.N)
	drainSamples := make([]time.Duration, 0, b.N)
	backlogPeak := 0

	b.ReportAllocs()
	b.ResetTimer()
	for iteration := 0; iteration < b.N; iteration++ {
		aliases := warmAliases
		if !cfg.warm {
			b.StopTimer()
			aliases = infra.seedBenchmarkTransfer(b, fmt.Sprintf("cold-%s-%d", uuid.NewString(), iteration))
			b.StartTimer()
		}

		projectedStarted := time.Now()
		httpStarted := time.Now()
		infra.executeBenchmarkRequest(b, app, cfg, aliases, fmt.Sprintf("bench-%s-%d", uuid.NewString(), iteration))
		httpSamples = append(httpSamples, time.Since(httpStarted))

		if cfg.asynchronous() {
			queued, inspectErr := infra.rabbit.Channel.QueueInspect(infra.queue)
			require.NoError(b, inspectErr)
			backlogPeak = max(backlogPeak, queued.Messages)

			drainStarted := time.Now()
			infra.drainBenchmarkProjection(b, cfg)
			drainSamples = append(drainSamples, time.Since(drainStarted))
		}
		projectedSamples = append(projectedSamples, time.Since(projectedStarted))
	}
	b.StopTimer()

	queued, err := infra.rabbit.Channel.QueueInspect(infra.queue)
	require.NoError(b, err)
	rowsAfter := infra.benchmarkSQLRows(b)
	memoryAfter := infra.benchmarkRedisMemory(b)

	reportEngineWriteBehindPercentiles(b, "http", httpSamples)
	reportEngineWriteBehindPercentiles(b, "lua", infra.engineHook.snapshot())
	reportEngineWriteBehindPercentiles(b, "drain", drainSamples)
	reportEngineWriteBehindThroughput(b, cfg.size, httpSamples, projectedSamples)
	b.ReportMetric(float64(rowsAfter-rowsBefore)/float64(b.N), "rows_per_commit")
	b.ReportMetric(float64(backlogPeak), "backlog_peak_msgs")
	b.ReportMetric(float64(queued.Messages), "backlog_end_msgs")
	b.ReportMetric(float64(memoryAfter), "redis_memory_bytes")
	b.ReportMetric(float64(memoryAfter-memoryBefore)/float64(b.N), "redis_memory_delta_bytes/op")
}

func (infra *engineWriteBehindHTTPIntegration) configureBenchmarkMode(cfg engineWriteBehindBenchmarkCase) error {
	infra.command.TransactionWriteBehindDispatcher = nil
	infra.command.TransactionWriteBehindAsync = false
	if cfg.mode == engineBenchmarkBaselineSync {
		infra.command.Engine = infra.engine
		infra.command.TransactionWriteBehindAsync = true
		infra.command.TransactionWriteBehindDispatcher = benchmarkLegacySyncDispatcher{
			completer: infra.command.AppliedTransactionCompleter, acknowledger: infra.command.EngineRecoveryAcknowledger,
		}

		return nil
	}

	infra.command.Engine = infra.engine
	if !cfg.asynchronous() {
		return nil
	}

	var producer *ledgerRabbitMQ.EngineWriteBehindProducer
	var err error
	if cfg.tenantID == "" {
		if infra.benchmarkSTProducer == nil {
			infra.benchmarkSTProducer, err = ledgerRabbitMQ.NewSingleTenantEngineWriteBehindProducer(
				infra.rabbitConn, infra.exchange, infra.routingKey, time.Second,
			)
		}
		producer = infra.benchmarkSTProducer
	} else {
		if infra.benchmarkMTProducer == nil {
			infra.benchmarkMTProducer, err = ledgerRabbitMQ.NewMultiTenantEngineWriteBehindProducer(
				integrationTenantChannelProvider{connection: infra.rabbit.Conn}, infra.exchange, infra.routingKey, time.Second,
			)
		}
		producer = infra.benchmarkMTProducer
	}
	if err != nil {
		return err
	}

	infra.command.TransactionWriteBehindAsync = true
	infra.command.TransactionWriteBehindDispatcher = engineWriteBehindDispatcher{publisher: producer}

	return nil
}

func (infra *engineWriteBehindHTTPIntegration) executeBenchmarkRequest(
	b testing.TB,
	app *fiber.App,
	cfg engineWriteBehindBenchmarkCase,
	aliases engineWriteBehindAliases,
	idempotencyKey string,
) {
	b.Helper()

	var response engineWriteBehindHTTPResponse
	if cfg.batchHTTP {
		response = infra.postBenchmarkBatch(b, app, aliases, cfg.size, idempotencyKey)
	} else {
		response = infra.postBenchmarkCreate(b, app, cfg.version, aliases, idempotencyKey)
	}
	require.Equalf(b, http.StatusCreated, response.status, "benchmark request failed: %s", response.body)
}

func (infra *engineWriteBehindHTTPIntegration) drainBenchmarkProjection(b testing.TB, cfg engineWriteBehindBenchmarkCase) {
	b.Helper()

	deliveries := make([]amqp.Delivery, cfg.size)
	for index := range deliveries {
		delivery, queued, err := infra.rabbit.Channel.Get(infra.queue, false)
		require.NoError(b, err)
		require.Truef(b, queued, "expected async projection message %d of %d", index+1, cfg.size)
		deliveries[index] = delivery
	}

	ctx := context.Background()
	policy := rabbitEngineSingleTenant
	if cfg.tenantID != "" {
		ctx = tmcore.ContextWithTenantID(ctx, cfg.tenantID)
		policy = rabbitEngineMultiTenant
	}
	dispatcher := requireRabbitTransactionDispatcher(b, infra.command, policy, cfg.mode == engineBenchmarkAsyncBulk)
	if cfg.mode == engineBenchmarkAsyncBulk {
		results, err := dispatcher.handleBulk(ctx, deliveries)
		require.NoError(b, err)
		require.Len(b, results, len(deliveries))
		for index := range results {
			require.Truef(b, results[index].Success, "bulk projection %d failed: %s", index, results[index].Error)
		}
	} else {
		require.Len(b, deliveries, 1)
		require.NoError(b, dispatcher.handle(ctx, deliveries[0].Body))
	}

	for index := range deliveries {
		require.NoError(b, deliveries[index].Ack(false))
	}
}

func (infra *engineWriteBehindHTTPIntegration) seedBenchmarkTransfer(tb testing.TB, suffix string) engineWriteBehindAliases {
	tb.Helper()
	t := tb
	aliases := engineWriteBehindAliases{source: "@bench-source-" + suffix, destination: "@bench-destination-" + suffix}

	for alias, available := range map[string]decimal.Decimal{
		aliases.source: decimal.NewFromInt(1_000_000_000), aliases.destination: decimal.Zero,
	} {
		accountID := postgrestestutil.CreateTestAccount(t, infra.db, infra.organization, infra.ledger, nil, alias, alias, "USD", nil)
		params := postgrestestutil.DefaultBalanceParams()
		params.Alias, params.AssetCode, params.Available = alias, "USD", available
		postgrestestutil.CreateTestBalance(t, infra.db, infra.organization, infra.ledger, accountID, params)
	}

	return aliases
}

func (infra *engineWriteBehindHTTPIntegration) postBenchmarkCreate(
	tb testing.TB,
	app *fiber.App,
	version string,
	aliases engineWriteBehindAliases,
	idempotencyKey string,
) engineWriteBehindHTTPResponse {
	tb.Helper()
	t := tb
	path := "/v1/organizations/" + infra.organization.String() + "/ledgers/" + infra.ledger.String() + "/transactions/json"
	body := fmt.Sprintf(`{"description":"write-behind benchmark","metadata":{"benchmark":"true"},"send":{"asset":"USD","value":"1","source":{"from":[{"accountAlias":%q,"amount":{"asset":"USD","value":"1"}}]},"distribute":{"to":[{"accountAlias":%q,"amount":{"asset":"USD","value":"1"}}]}}}`, aliases.source, aliases.destination)
	if version == "v2" {
		path = "/v2/transactions/direct"
		body = fmt.Sprintf(`{"description":"write-behind benchmark","asset":"USD","amount":"1","metadata":{"benchmark":"true"},"debits":[{"alias":%q,"organizationId":%q,"ledgerId":%q,"amount":"1"}],"credits":[{"alias":%q,"organizationId":%q,"ledgerId":%q,"amount":"1"}]}`,
			aliases.source, infra.organization.String(), infra.ledger.String(), aliases.destination, infra.organization.String(), infra.ledger.String())
	}

	return performEngineWriteBehindHTTPRequest(t, app, benchmarkRequest(path, body, idempotencyKey))
}

func (infra *engineWriteBehindHTTPIntegration) postBenchmarkBatch(
	tb testing.TB,
	app *fiber.App,
	aliases engineWriteBehindAliases,
	size int,
	idempotencyKey string,
) engineWriteBehindHTTPResponse {
	tb.Helper()
	t := tb
	transactions := make([]httpin.CreateAtomicTransactionBatchV2ItemRequest, size)
	for index := range transactions {
		leg := func(alias string) httpin.TransactionV2LegRequest {
			return httpin.TransactionV2LegRequest{
				Alias: alias, OrganizationID: infra.organization.String(), LedgerID: infra.ledger.String(), Amount: "1",
			}
		}
		transactions[index] = httpin.CreateAtomicTransactionBatchV2ItemRequest{
			Action: "direct", Order: index + 1,
			CreateTransactionV2Request: httpin.CreateTransactionV2Request{
				Description: "write-behind batch benchmark", Asset: "USD", Amount: "1",
				Debits:   []httpin.TransactionV2LegRequest{leg(aliases.source)},
				Credits:  []httpin.TransactionV2LegRequest{leg(aliases.destination)},
				Metadata: map[string]any{"benchmark": "true"},
			},
		}
	}
	body, err := json.Marshal(httpin.CreateAtomicTransactionBatchV2Request{Transactions: transactions})
	require.NoError(t, err)

	return performEngineWriteBehindHTTPRequest(t, app, benchmarkRequest("/v2/transactions/batch", string(body), idempotencyKey))
}

func benchmarkRequest(path, body, idempotencyKey string) *http.Request {
	request := httptest.NewRequest(http.MethodPost, path, bytes.NewBufferString(body))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("X-Idempotency", idempotencyKey)
	request.Header.Set("X-TTL", "3600")

	return request
}

func (infra *engineWriteBehindHTTPIntegration) benchmarkSQLRows(tb testing.TB) int64 {
	tb.Helper()

	var rows int64
	require.NoError(tb, infra.db.QueryRow(`
		SELECT (SELECT COUNT(*) FROM transaction) + (SELECT COUNT(*) FROM operation)
	`).Scan(&rows))

	return rows
}

func (infra *engineWriteBehindHTTPIntegration) benchmarkRedisMemory(tb testing.TB) int64 {
	tb.Helper()

	info, err := infra.redis.Info(context.Background(), "memory").Result()
	require.NoError(tb, err)
	for _, line := range strings.Split(info, "\n") {
		value, ok := strings.CutPrefix(strings.TrimSpace(line), "used_memory:")
		if !ok {
			continue
		}
		memory, parseErr := strconv.ParseInt(value, 10, 64)
		require.NoError(tb, parseErr)

		return memory
	}

	tb.Fatal("Valkey INFO memory omitted used_memory")

	return 0
}

func reportEngineWriteBehindPercentiles(b *testing.B, prefix string, samples []time.Duration) {
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

func reportEngineWriteBehindThroughput(
	b *testing.B,
	transactionsPerRequest int,
	httpSamples []time.Duration,
	projectedSamples []time.Duration,
) {
	b.Helper()
	totalTransactions := float64(b.N * transactionsPerRequest)
	b.ReportMetric(totalTransactions/sumBenchmarkDurations(httpSamples).Seconds(), "admitted_tps")
	b.ReportMetric(totalTransactions/sumBenchmarkDurations(projectedSamples).Seconds(), "projected_tps")
}

func sumBenchmarkDurations(samples []time.Duration) time.Duration {
	var total time.Duration
	for _, sample := range samples {
		total += sample
	}

	return total
}
