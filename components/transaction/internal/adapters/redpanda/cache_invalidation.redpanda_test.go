// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package redpanda

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/twmb/franz-go/pkg/kgo"

	libLog "github.com/LerianStudio/lib-commons/v2/commons/log"
	libZap "github.com/LerianStudio/lib-commons/v2/commons/zap"

	crossshardevents "github.com/LerianStudio/midaz/v3/pkg/crossshard/events"
	"github.com/LerianStudio/midaz/v3/pkg/utils"
)

// testLogger returns a zap-backed libLog.Logger suitable for consumer tests.
func cacheInvalTestLogger(t *testing.T) libLog.Logger {
	t.Helper()

	logger, err := libZap.InitializeLoggerWithError()
	require.NoError(t, err)

	return logger
}

// miniredisProvider adapts a miniredis instance to the provider interface
// used by CacheInvalidationRunner.
type miniredisProvider struct {
	addr string
}

func (p *miniredisProvider) GetClient(_ context.Context) (redis.UniversalClient, error) {
	return redis.NewClient(&redis.Options{Addr: p.addr}), nil
}

// fakeDLQPublisher captures ProduceSync invocations so tests can assert DLQ
// routing occurred with the expected reason / payload.
type fakeDLQPublisher struct {
	mu       sync.Mutex
	records  []*kgo.Record
	errNext  error
	produced atomic.Int64
}

func (f *fakeDLQPublisher) ProduceSync(_ context.Context, rs ...*kgo.Record) kgo.ProduceResults {
	f.mu.Lock()
	defer f.mu.Unlock()

	results := make(kgo.ProduceResults, 0, len(rs))

	for _, r := range rs {
		if f.errNext != nil {
			results = append(results, kgo.ProduceResult{Record: r, Err: f.errNext})

			continue
		}

		f.records = append(f.records, r)
		f.produced.Add(1)

		results = append(results, kgo.ProduceResult{Record: r})
	}

	return results
}

func (f *fakeDLQPublisher) count() int {
	return int(f.produced.Load())
}

func (f *fakeDLQPublisher) snapshot() []*kgo.Record {
	f.mu.Lock()
	defer f.mu.Unlock()

	out := make([]*kgo.Record, len(f.records))
	copy(out, f.records)

	return out
}

// failingOnceProvider wraps a real provider and returns a transient error
// on the first N calls, then delegates. Used to exercise backoff.
type failingOnceProvider struct {
	inner   cacheInvalidationRedisClientProvider
	failN   int32
	counter atomic.Int32
}

var errTestTransientRedisUnavailable = errors.New("transient redis unavailable")

func (p *failingOnceProvider) GetClient(ctx context.Context) (redis.UniversalClient, error) {
	if p.counter.Add(1) <= p.failN {
		return nil, errTestTransientRedisUnavailable
	}

	return p.inner.GetClient(ctx) //nolint:wrapcheck // test fixture
}

// newTestRunner builds a CacheInvalidationRunner pre-wired for unit tests
// (no kgo client — tests drive processRecord directly).
func newTestRunner(t *testing.T, provider cacheInvalidationRedisClientProvider, dlq cacheInvalidationKafkaPublisher, shardCount int) *CacheInvalidationRunner {
	t.Helper()

	r := &CacheInvalidationRunner{
		client:              nil, // tests bypass Run(); processRecord handles nil client for commitOffset
		dlqPublisher:        dlq,
		redisProvider:       provider,
		logger:              cacheInvalTestLogger(t),
		sourceTopic:         crossshardevents.TopicCacheInvalidation,
		dlqTopic:            crossshardevents.TopicCacheInvalidation + cacheInvalidationDLQSuffix,
		consumerGroup:       defaultCacheInvalidationConsumerGroup,
		shardCount:          shardCount,
		pollTimeout:         time.Second,
		maxRetries:          3,
		transientBase:       5 * time.Millisecond,
		transientMax:        50 * time.Millisecond,
		shutdownWaitTimeout: time.Second,
	}

	return r
}

// Roundtrip JSON parity.

func TestCacheInvalidationEvent_RoundtripJSON(t *testing.T) {
	t.Parallel()

	orig := crossshardevents.CacheInvalidationEvent{
		TransactionID:  "tx-42",
		OrganizationID: uuid.NewString(),
		LedgerID:       uuid.NewString(),
		Aliases: []crossshardevents.CacheInvalidationTarget{
			{Alias: "@alice", BalanceKey: "default"},
			{Alias: "@bob", BalanceKey: "savings"},
		},
		Reason:    crossshardevents.ReasonRecoveryDriveToCompletion,
		EmittedAt: time.Date(2026, 4, 16, 12, 0, 0, 0, time.UTC),
	}

	payload, err := json.Marshal(orig)
	require.NoError(t, err)

	var decoded crossshardevents.CacheInvalidationEvent

	require.NoError(t, json.Unmarshal(payload, &decoded))

	assert.Equal(t, orig.TransactionID, decoded.TransactionID)
	assert.Equal(t, orig.OrganizationID, decoded.OrganizationID)
	assert.Equal(t, orig.LedgerID, decoded.LedgerID)
	assert.Equal(t, orig.Aliases, decoded.Aliases)
	assert.Equal(t, orig.Reason, decoded.Reason)
	assert.True(t, orig.EmittedAt.Equal(decoded.EmittedAt))
}

// Consumer behaviour.

func TestConsumer_DeletesExpectedKeys_ShardAwareAndNonSharded(t *testing.T) {
	t.Parallel()

	mini, err := miniredis.Run()
	require.NoError(t, err)

	defer mini.Close()

	provider := &miniredisProvider{addr: mini.Addr()}

	orgID := uuid.New()
	ledgerID := uuid.New()
	alias := "@alice"
	balanceKey := "default"
	aliasKey := alias + "#" + balanceKey

	nonSharded := utils.BalanceInternalKey(orgID, ledgerID, aliasKey)
	shard0 := utils.BalanceShardKey(0, orgID, ledgerID, aliasKey)
	shard1 := utils.BalanceShardKey(1, orgID, ledgerID, aliasKey)

	mini.Set(nonSharded, `{"balance":"stale"}`)
	mini.Set(shard0, `{"balance":"stale-0"}`)
	mini.Set(shard1, `{"balance":"stale-1"}`)

	dlq := &fakeDLQPublisher{}
	runner := newTestRunner(t, provider, dlq, 2) // shardCount=2 → shard0 + shard1 + non-sharded

	evt := crossshardevents.CacheInvalidationEvent{
		TransactionID:  "tx-delete",
		OrganizationID: orgID.String(),
		LedgerID:       ledgerID.String(),
		Aliases:        []crossshardevents.CacheInvalidationTarget{{Alias: alias, BalanceKey: balanceKey}},
		Reason:         crossshardevents.ReasonRecoveryDriveToCompletion,
		EmittedAt:      time.Now().UTC(),
	}

	payload, err := json.Marshal(evt)
	require.NoError(t, err)

	record := &kgo.Record{Topic: runner.sourceTopic, Value: payload}

	runner.processRecord(context.Background(), nil, record)

	// All three keys should be gone.
	assert.False(t, mini.Exists(nonSharded), "non-sharded key should be deleted")
	assert.False(t, mini.Exists(shard0), "shard 0 key should be deleted")
	assert.False(t, mini.Exists(shard1), "shard 1 key should be deleted")
	assert.Equal(t, 0, dlq.count(), "successful deletion should not DLQ")
}

func TestConsumer_HandlesMalformedSkipsOffset(t *testing.T) {
	t.Parallel()

	mini, err := miniredis.Run()
	require.NoError(t, err)

	defer mini.Close()

	provider := &miniredisProvider{addr: mini.Addr()}
	dlq := &fakeDLQPublisher{}
	runner := newTestRunner(t, provider, dlq, 0)

	record := &kgo.Record{
		Topic: runner.sourceTopic,
		Value: []byte("this-is-not-json{{{"),
	}

	runner.processRecord(context.Background(), nil, record)

	require.Equal(t, 1, dlq.count(), "malformed payload must route to DLQ")

	dlqRecords := dlq.snapshot()
	require.Len(t, dlqRecords, 1)

	var reasonHeader string

	for _, h := range dlqRecords[0].Headers {
		if h.Key == cacheInvalidationDLQReasonHeader {
			reasonHeader = string(h.Value)
		}
	}

	assert.Equal(t, cacheInvalidationDLQReasonMalformed, reasonHeader)
	assert.Equal(t, runner.dlqTopic, dlqRecords[0].Topic, "DLQ record must target the .dlq topic")
}

func TestConsumer_HandlesInvalidUUIDSkipsOffset(t *testing.T) {
	t.Parallel()

	mini, err := miniredis.Run()
	require.NoError(t, err)

	defer mini.Close()

	provider := &miniredisProvider{addr: mini.Addr()}
	dlq := &fakeDLQPublisher{}
	runner := newTestRunner(t, provider, dlq, 0)

	evt := crossshardevents.CacheInvalidationEvent{
		TransactionID:  "tx-bad-uuid",
		OrganizationID: "not-a-uuid",
		LedgerID:       uuid.NewString(),
		Aliases:        []crossshardevents.CacheInvalidationTarget{{Alias: "@x", BalanceKey: "default"}},
	}

	payload, err := json.Marshal(evt)
	require.NoError(t, err)

	record := &kgo.Record{Topic: runner.sourceTopic, Value: payload}

	runner.processRecord(context.Background(), nil, record)

	require.Equal(t, 1, dlq.count(), "invalid UUID must route to DLQ")

	var reasonHeader string

	for _, h := range dlq.snapshot()[0].Headers {
		if h.Key == cacheInvalidationDLQReasonHeader {
			reasonHeader = string(h.Value)
		}
	}

	assert.Equal(t, cacheInvalidationDLQReasonInvalidOrgOrLedger, reasonHeader)
}

func TestConsumer_TransientRedisRetriesThenSucceeds(t *testing.T) {
	t.Parallel()

	mini, err := miniredis.Run()
	require.NoError(t, err)

	defer mini.Close()

	innerProvider := &miniredisProvider{addr: mini.Addr()}
	failing := &failingOnceProvider{inner: innerProvider, failN: 2}
	dlq := &fakeDLQPublisher{}

	runner := newTestRunner(t, failing, dlq, 0)
	runner.maxRetries = 5 // enough budget to cover 2 failures

	orgID := uuid.New()
	ledgerID := uuid.New()
	aliasKey := "@alice#default"

	mini.Set(utils.BalanceInternalKey(orgID, ledgerID, aliasKey), `{"balance":"stale"}`)

	evt := crossshardevents.CacheInvalidationEvent{
		TransactionID:  "tx-retry",
		OrganizationID: orgID.String(),
		LedgerID:       ledgerID.String(),
		Aliases:        []crossshardevents.CacheInvalidationTarget{{Alias: "@alice", BalanceKey: "default"}},
		Reason:         crossshardevents.ReasonRecoveryDriveToCompletion,
	}

	payload, err := json.Marshal(evt)
	require.NoError(t, err)

	record := &kgo.Record{Topic: runner.sourceTopic, Value: payload}

	runner.processRecord(context.Background(), nil, record)

	assert.False(t, mini.Exists(utils.BalanceInternalKey(orgID, ledgerID, aliasKey)),
		"key should be deleted after successful retry")
	assert.Equal(t, 0, dlq.count(), "successful retry should not DLQ")
	assert.GreaterOrEqual(t, int(failing.counter.Load()), 3, "provider must have been retried")
}

func TestConsumer_RoutesToDLQAfterKFailures(t *testing.T) {
	t.Parallel()

	mini, err := miniredis.Run()
	require.NoError(t, err)

	defer mini.Close()

	// Provider always fails — exhausts the retry budget.
	failing := &failingOnceProvider{
		inner: &miniredisProvider{addr: mini.Addr()},
		failN: 1_000, // way beyond retry budget
	}
	dlq := &fakeDLQPublisher{}

	runner := newTestRunner(t, failing, dlq, 0)
	runner.maxRetries = 2 // small for speed

	evt := crossshardevents.CacheInvalidationEvent{
		TransactionID:  "tx-dlq",
		OrganizationID: uuid.NewString(),
		LedgerID:       uuid.NewString(),
		Aliases:        []crossshardevents.CacheInvalidationTarget{{Alias: "@zombie", BalanceKey: "default"}},
	}

	payload, err := json.Marshal(evt)
	require.NoError(t, err)

	record := &kgo.Record{Topic: runner.sourceTopic, Value: payload}

	runner.processRecord(context.Background(), nil, record)

	require.Equal(t, 1, dlq.count(), "persistent failure must DLQ")

	var reasonHeader string

	for _, h := range dlq.snapshot()[0].Headers {
		if h.Key == cacheInvalidationDLQReasonHeader {
			reasonHeader = string(h.Value)
		}
	}

	assert.Equal(t, cacheInvalidationDLQReasonRetriesExhausted, reasonHeader)
}

func TestConsumer_EmptyAliasesIsNoOp(t *testing.T) {
	t.Parallel()

	mini, err := miniredis.Run()
	require.NoError(t, err)

	defer mini.Close()

	provider := &miniredisProvider{addr: mini.Addr()}
	dlq := &fakeDLQPublisher{}
	runner := newTestRunner(t, provider, dlq, 0)

	evt := crossshardevents.CacheInvalidationEvent{
		TransactionID:  "tx-empty",
		OrganizationID: uuid.NewString(),
		LedgerID:       uuid.NewString(),
		Aliases:        nil,
	}

	payload, err := json.Marshal(evt)
	require.NoError(t, err)

	record := &kgo.Record{Topic: runner.sourceTopic, Value: payload}

	runner.processRecord(context.Background(), nil, record)

	assert.Equal(t, 0, dlq.count(), "empty-aliases fallback must NOT DLQ; consumer logs + ACKs")
}

func TestConsumer_MultipleAliasesAllDeleted(t *testing.T) {
	t.Parallel()

	mini, err := miniredis.Run()
	require.NoError(t, err)

	defer mini.Close()

	provider := &miniredisProvider{addr: mini.Addr()}
	dlq := &fakeDLQPublisher{}
	runner := newTestRunner(t, provider, dlq, 0)

	orgID := uuid.New()
	ledgerID := uuid.New()

	targets := []crossshardevents.CacheInvalidationTarget{
		{Alias: "@a", BalanceKey: "default"},
		{Alias: "@b", BalanceKey: "savings"},
		{Alias: "@c", BalanceKey: "default"},
	}

	for _, tgt := range targets {
		mini.Set(utils.BalanceInternalKey(orgID, ledgerID, tgt.Alias+"#"+tgt.BalanceKey), "x")
	}

	evt := crossshardevents.CacheInvalidationEvent{
		TransactionID:  "tx-multi",
		OrganizationID: orgID.String(),
		LedgerID:       ledgerID.String(),
		Aliases:        targets,
	}

	payload, err := json.Marshal(evt)
	require.NoError(t, err)

	runner.processRecord(context.Background(), nil, &kgo.Record{Topic: runner.sourceTopic, Value: payload})

	for _, tgt := range targets {
		key := utils.BalanceInternalKey(orgID, ledgerID, tgt.Alias+"#"+tgt.BalanceKey)
		assert.False(t, mini.Exists(key), "key %s should be deleted", key)
	}

	assert.Equal(t, 0, dlq.count())
}

// --- Producer-side helper (buildCacheInvalidationTargets) asserted in the
// authorizer bootstrap test suite, not here.
