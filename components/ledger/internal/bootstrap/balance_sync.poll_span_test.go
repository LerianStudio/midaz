// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	tmcore "github.com/LerianStudio/lib-commons/v7/commons/tenant-manager/core"
	libObservability "github.com/LerianStudio/lib-observability/v4"
	libLog "github.com/LerianStudio/lib-observability/v4/log"
	"github.com/LerianStudio/lib-observability/v4/redisobs"
	"github.com/alicebob/miniredis/v2"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/metric/noop"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/mock/gomock"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/balance"
	redisTransaction "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/redis/transaction"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/services/command"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/testutil/spantest"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	"github.com/LerianStudio/midaz/v4/pkg/utils"
)

const pollSpanName = "redis.get_balance_sync_keys"

// pollRedisProvider hands the Redis adapter a fixed client, or a fixed error.
type pollRedisProvider struct {
	client redis.UniversalClient
	err    error
}

func (p *pollRedisProvider) GetClient(context.Context) (redis.UniversalClient, error) {
	if p.err != nil {
		return nil, p.err
	}

	return p.client, nil
}

// newPollTestWorker wires a worker to the real Redis adapter over miniredis and a mocked
// balance repository, so the poll and the flush run the production code paths.
func newPollTestWorker(t *testing.T, provider *pollRedisProvider) *BalanceSyncWorker {
	t.Helper()

	ctrl := gomock.NewController(t)

	balanceRepo := balance.NewMockRepository(ctrl)
	balanceRepo.EXPECT().
		UpdateMany(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, _, _ uuid.UUID, balances []mmodel.BalanceRedis) (int64, error) {
			return int64(len(balances)), nil
		}).
		AnyTimes()

	repo := &redisTransaction.RedisConsumerRepository{}
	if provider.err == nil {
		var err error

		repo, err = redisTransaction.NewConsumerRedis(provider)
		require.NoError(t, err)
	} else {
		// NewConsumerRedis refuses a provider that cannot connect; the down-Redis case
		// builds the adapter against a working provider and fails it afterwards.
		client := redis.NewClient(&redis.Options{Addr: miniredis.RunT(t).Addr()})
		t.Cleanup(func() { _ = client.Close() })

		working := &pollRedisProvider{client: client}

		var err error

		repo, err = redisTransaction.NewConsumerRedis(working)
		require.NoError(t, err)

		working.client = nil
		working.err = provider.err
	}

	return NewBalanceSyncWorker(newTestLogger(), &command.UseCase{TransactionRedisRepo: repo, BalanceRepo: balanceRepo}, BalanceSyncConfig{})
}

// pollTracing returns a recording context and the tracer provider behind it, so the Redis
// driver hook records onto the same recorder and under the same ParentBased sampler.
func pollTracing(t *testing.T) (context.Context, *tracetest.SpanRecorder, *sdktrace.TracerProvider) {
	t.Helper()

	recorder := tracetest.NewSpanRecorder()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(recorder))
	t.Cleanup(func() { _ = tp.Shutdown(context.Background()) })

	return libObservability.ContextWithTracer(context.Background(), tp.Tracer("balance-sync-poll-test")), recorder, tp
}

// miniredisPollProvider starts an in-memory Redis for one test. Its client carries the
// same driver hook production installs (lib-commons commons/redis → redisobs.Setup), bound
// to the test's tracer provider instead of the global one, so driver command spans are
// recorded and subject to the same sampling as the Midaz spans.
func miniredisPollProvider(t *testing.T, tp *sdktrace.TracerProvider) (*pollRedisProvider, *miniredis.Miniredis) {
	t.Helper()

	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	t.Cleanup(func() { _ = client.Close() })

	release, err := redisobs.Setup(client, redisobs.WithTracerProvider(tp), redisobs.WithMeterProvider(noop.NewMeterProvider()))
	require.NoError(t, err)

	// Registered after the close, so it runs first and releases the hook before the client goes.
	t.Cleanup(func() { _ = release() })

	return &pollRedisProvider{client: client}, server
}

// dialSpanName is the driver span go-redis opens when its pool dials a new connection.
// The pool dials outside the command's context, so it is a root of its own on every new
// connection whatever the poll does; it is not a poll span and is left out below.
const dialSpanName = "redis.dial"

// driverSpans returns the recorded command spans emitted by the Redis driver hook.
func driverSpans(recorder *tracetest.SpanRecorder) []sdktrace.ReadOnlySpan {
	var spans []sdktrace.ReadOnlySpan

	for _, s := range recorder.Ended() {
		if strings.Contains(s.InstrumentationScope().Name, "redisotel") && s.Name() != dialSpanName {
			spans = append(spans, s)
		}
	}

	return spans
}

// pollCycleSpanNames lists the recorded span names other than the connection dial.
func pollCycleSpanNames(recorder *tracetest.SpanRecorder) []string {
	var names []string

	for _, name := range spantest.SpanNames(recorder) {
		if name != dialSpanName {
			names = append(names, name)
		}
	}

	return names
}

// scheduleBalance seeds one due balance for tenantID ("" in single-tenant): the cached
// value and its entry on the sync schedule, both under the tenant's key namespace.
func scheduleBalance(t *testing.T, server *miniredis.Miniredis, tenantID string, orgID, ledgerID uuid.UUID, alias string) {
	t.Helper()

	prefix := ""
	if tenantID != "" {
		prefix = "tenant:" + tenantID + ":"
	}

	member := syncKeyFor(orgID, ledgerID, alias)

	value, err := json.Marshal(mmodel.BalanceRedis{
		ID: uuid.NewString(), Alias: "@" + alias, Key: "default", AccountID: uuid.NewString(),
		AccountType: "deposit", AssetCode: "BRL", Available: decimal.NewFromInt(100), OnHold: decimal.Zero, Version: 2,
	})
	require.NoError(t, err)

	require.NoError(t, server.Set(prefix+member, string(value)))

	_, err = server.ZAdd(prefix+utils.BalanceSyncScheduleKey, 1, member)
	require.NoError(t, err)
}

// runOneCollectorCycle drives the production collector loop synchronously: it polls,
// flushes whatever it claimed (batch size 1 triggers on the first key), polls again and
// stops on the first idle wait. No goroutine and no sleep is involved.
func runOneCollectorCycle(ctx context.Context, w *BalanceSyncWorker, logger libLog.Logger) {
	collector := NewBalanceSyncCollector(1, time.Second, time.Millisecond, logger)
	collector.Run(ctx, w.flushBatchFn, w.fetchSyncKeys, func(context.Context) bool { return true })
}

func TestUnsampledPollContext_DropsThePollAndKeepsTheContextValues(t *testing.T) {
	t.Parallel()

	ctx, recorder := spantest.RecordingContext()
	ctx = tmcore.ContextWithTenantID(ctx, "tenant-a")
	_, wantTracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	pollCtx := unsampledPollContext(ctx)

	parent := trace.SpanContextFromContext(pollCtx)
	require.True(t, parent.IsValid(), "an invalid parent would make the poll a sampled root")
	assert.False(t, parent.IsSampled())

	_, tracer, _, _ := libObservability.NewTrackingFromContext(pollCtx)
	assert.Equal(t, wantTracer, tracer, "the poll keeps the service tracer")
	assert.Equal(t, "tenant-a", tmcore.GetTenantIDContext(pollCtx), "the poll keeps the tenant")

	_, pollSpan := tracer.Start(pollCtx, "poll")
	pollSpan.End()

	_, flushSpan := tracer.Start(ctx, "flush")
	flushSpan.End()

	assert.Equal(t, []string{"flush"}, spantest.SpanNames(recorder),
		"a span under the poll context is dropped; one under the original context is kept")

	assert.NotEqual(t, parent.TraceID(), trace.SpanContextFromContext(unsampledPollContext(ctx)).TraceID(),
		"every poll gets its own unsampled trace")
}

// Scenario: Poll vazio não exporta spans.
func TestBalanceSyncPoll_EmptyScheduleExportsNoSpan(t *testing.T) {
	t.Parallel()

	ctx, recorder, tp := pollTracing(t)
	provider, _ := miniredisPollProvider(t, tp)
	w := newPollTestWorker(t, provider)

	runOneCollectorCycle(ctx, w, newTestLogger())

	assert.Empty(t, pollCycleSpanNames(recorder), "an empty poll exports nothing, neither the Midaz span nor the driver's evalsha")
}

// Scenario: Poll com chaves exporta o flush.
func TestBalanceSyncPoll_ClaimedKeyExportsTheFlush(t *testing.T) {
	t.Parallel()

	ctx, recorder, tp := pollTracing(t)
	provider, server := miniredisPollProvider(t, tp)
	orgID, ledgerID := uuid.New(), uuid.New()
	scheduleBalance(t, server, "", orgID, ledgerID, "acc1")

	w := newPollTestWorker(t, provider)

	runOneCollectorCycle(ctx, w, newTestLogger())

	spantest.AssertNoSpan(t, recorder, pollSpanName)

	for _, name := range []string{
		"balance.worker.process_batch",
		"command.sync_balances_batch",
		"redis.get_balances_by_keys",
		"redis.remove_balance_sync_keys_batch",
	} {
		assert.True(t, spantest.HasSpan(recorder, name), "flush span %q missing; recorded: %v", name, spantest.SpanNames(recorder))
	}

	assert.False(t, server.Exists(utils.BalanceSyncScheduleKey), "the synced key leaves the schedule")

	// The driver spans recorded are the flush's own: the claim's driver span was dropped
	// with the poll, so every recorded one shares the flush trace.
	drivers := driverSpans(recorder)
	require.NotEmpty(t, drivers, "the flush's Redis commands are traced by the driver hook")

	flushTrace := spantest.FindSpan(t, recorder, "balance.worker.process_batch").SpanContext().TraceID()
	for _, s := range drivers {
		assert.Equal(t, flushTrace, s.SpanContext().TraceID(), "driver span %q is outside the flush trace", s.Name())
	}
}

// Scenario: Poll com falha do Redis continua reportado.
func TestBalanceSyncPoll_RedisFailureIsStillReported(t *testing.T) {
	t.Parallel()

	w := newPollTestWorker(t, &pollRedisProvider{err: errors.New("redis: connection refused")})

	ctx, recorder := spantest.RecordingContext()
	ctx = tmcore.ContextWithTenantID(ctx, "tenant-a")
	ctx, cancel := context.WithCancel(ctx)
	t.Cleanup(cancel)

	reportedTenant := make(chan string, 1)

	ctrl := gomock.NewController(t)
	logger := libLog.NewMockLogger(ctrl)
	logger.EXPECT().
		Log(gomock.Any(), libLog.LevelWarn, "BalanceSyncCollector: fetch keys error", gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, _ int, _ string, fields ...any) {
			for _, field := range fields {
				if f, ok := field.(libLog.Field); ok && f.Key == "tenant_id" {
					tenant, _ := f.Value.(string)
					reportedTenant <- tenant
				}
			}

			// Stop the loop: the collector's error backoff returns at once on a done context.
			cancel()
		}).
		Times(1)
	logger.EXPECT().Log(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).AnyTimes()

	runOneCollectorCycle(ctx, w, logger)

	select {
	case tenant := <-reportedTenant:
		assert.Equal(t, "tenant-a", tenant)
	default:
		t.Fatal("the fetch error was not reported with its tenant")
	}

	spantest.AssertNoSpan(t, recorder, pollSpanName)
}

// Scenario: Multi-tenant isola o poll por tenant. Two tenants share one Redis; only
// tenant-a has a due key. Each tenant's collector runs with its own tenant context, as
// the MT worker does.
func TestBalanceSyncPoll_MultiTenantIsolatesThePollPerTenant(t *testing.T) {
	t.Parallel()

	ctx, recorder, tp := pollTracing(t)
	provider, server := miniredisPollProvider(t, tp)
	scheduleBalance(t, server, "tenant-a", uuid.New(), uuid.New(), "acc1")

	w := newPollTestWorker(t, provider)

	runOneCollectorCycle(tmcore.ContextWithTenantID(ctx, "tenant-a"), w, newTestLogger())

	afterTenantA := len(pollCycleSpanNames(recorder))

	runOneCollectorCycle(tmcore.ContextWithTenantID(ctx, "tenant-b"), w, newTestLogger())

	spantest.AssertNoSpan(t, recorder, pollSpanName)
	assert.Equal(t, "tenant-a", spanAttributes(t, recorder, "balance.worker.process_batch")["app.tenant_id"])
	assert.Equal(t, "tenant-a", spanAttributes(t, recorder, "redis.remove_balance_sync_keys_batch")["app.tenant_id"],
		"the tenant's flush ran to the end")
	assert.Len(t, pollCycleSpanNames(recorder), afterTenantA, "the idle tenant exports nothing, driver spans included")
}

// The single-tenant path is also covered for the legacy drainer's poll.
func TestBalanceSyncPoll_LegacyDrainerPollExportsNoSpan(t *testing.T) {
	t.Parallel()

	ctx, recorder, tp := pollTracing(t)
	provider, _ := miniredisPollProvider(t, tp)
	w := newPollTestWorker(t, provider)
	d := NewLegacyBalanceSyncDrainer(newTestLogger(), w.useCase, BalanceSyncConfig{})

	collector := NewBalanceSyncCollector(1, time.Second, time.Millisecond, newTestLogger())
	collector.Run(ctx, w.flushBatchFn, d.fetchLegacySyncKeys, func(context.Context) bool { return true })

	assert.Empty(t, pollCycleSpanNames(recorder), "an empty legacy poll exports nothing")
}
