// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package redpanda

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"github.com/twmb/franz-go/pkg/kgo"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"

	libLog "github.com/LerianStudio/lib-commons/v2/commons/log"
	libRedis "github.com/LerianStudio/lib-commons/v2/commons/redis"

	crossshardevents "github.com/LerianStudio/midaz/v3/pkg/crossshard/events"
	"github.com/LerianStudio/midaz/v3/pkg/utils"
)

// Defaults for the cache-invalidation consumer loop. Kept as package-level
// constants so tests can rewire via the exported fields on the runner.
const (
	// defaultCacheInvalidationConsumerGroup is the franz-go consumer group
	// used when the caller does not override it. Must NOT collide with the
	// MultiQueueConsumer group — this consumer is an independent worker
	// living in the transaction service purely to DEL stale Redis keys
	// after an authorizer 2PC recovery.
	defaultCacheInvalidationConsumerGroup = "transaction-cache-invalidation"

	// defaultCacheInvalidationPollTimeout bounds a single PollFetches call.
	// Short enough that stopping the runner observes cancellation quickly,
	// long enough to batch multiple events under steady-state load.
	defaultCacheInvalidationPollTimeout = time.Second

	// defaultCacheInvalidationMaxTransientRetries is the number of transient
	// Redis-side failures tolerated for a single record before the record
	// is routed to the DLQ. After exhaustion the offset is committed so
	// the consumer does not starve on a single poison payload.
	defaultCacheInvalidationMaxTransientRetries = 5

	// defaultCacheInvalidationTransientRetryBase is the first-attempt delay
	// for exponential backoff on transient Redis failures.
	defaultCacheInvalidationTransientRetryBase = 100 * time.Millisecond

	// defaultCacheInvalidationTransientRetryMax caps the exponential backoff
	// to avoid pathologically long per-record waits.
	defaultCacheInvalidationTransientRetryMax = 2 * time.Second

	// cacheInvalidationDLQSuffix is appended to the subscribed topic name
	// when producing DLQ records. Mirrors D6's dead-letter convention.
	cacheInvalidationDLQSuffix = ".dlq"

	// cacheInvalidationDLQReasonHeader carries the DLQ reason for operator
	// filtering without re-parsing the payload.
	cacheInvalidationDLQReasonHeader = "x-midaz-cache-invalidation-dlq-reason"

	// cacheInvalidationDLQCauseHeader carries the most-recent error text so
	// operators can triage repeat failures.
	cacheInvalidationDLQCauseHeader = "x-midaz-cache-invalidation-dlq-cause"

	// cacheInvalidationOriginalOffsetHeader preserves the original partition
	// offset for operator traceability once a record has been DLQ'd.
	cacheInvalidationOriginalOffsetHeader = "x-midaz-cache-invalidation-original-offset"
)

// DLQ reasons — stable labels for the DLQ counter and headers.
const (
	cacheInvalidationDLQReasonMalformed          = "malformed"
	cacheInvalidationDLQReasonRetriesExhausted   = "retries_exhausted"
	cacheInvalidationDLQReasonInvalidOrgOrLedger = "invalid_org_or_ledger"
)

// Metric result labels — stable values for the applied-counter.
const (
	cacheInvalidationResultSuccess = "success"
	cacheInvalidationResultMiss    = "miss"
	cacheInvalidationResultError   = "error"
)

// bounded per-event parallelism. Each event may carry many aliases; we cap
// the simultaneous in-flight DELs so a single large event cannot monopolize
// Redis. The same ceiling guards against fan-out explosion if a misbehaving
// producer emits events with thousands of targets.
const cacheInvalidationMaxAliasWorkers = 8

// Timeouts and wait intervals used inside the runner.
const (
	// cacheInvalidationShutdownDefaultTimeout bounds Close() waiting on
	// Run() completion by default. Long enough to drain an in-flight DEL
	// burst, short enough that pod rollover is not blocked excessively.
	cacheInvalidationShutdownDefaultTimeout = 30 * time.Second

	// cacheInvalidationShutdownPollInterval is how often Close() polls
	// the running flag while waiting for Run() to exit.
	cacheInvalidationShutdownPollInterval = 50 * time.Millisecond

	// cacheInvalidationCommitTimeout bounds a single CommitRecords call.
	cacheInvalidationCommitTimeout = 5 * time.Second

	// cacheInvalidationDLQPublishTimeout bounds a single DLQ ProduceSync.
	cacheInvalidationDLQPublishTimeout = 5 * time.Second

	// cacheInvalidationDLQHeaderCapacity is the pre-allocated capacity for
	// DLQ headers (existing headers plus reason, cause, and original
	// offset). Keeps the slice growth predictable.
	cacheInvalidationDLQHeaderCapacity = 3
)

// Sentinel errors — static so err113 stays happy and callers can errors.Is.
var (
	errCacheInvalidationSeedBrokersRequired   = errors.New("cache invalidation runner: SeedBrokers required")
	errCacheInvalidationRedisProviderRequired = errors.New("cache invalidation runner: RedisProvider required")
	errCacheInvalidationLoggerRequired        = errors.New("cache invalidation runner: Logger required")
)

// cacheInvalidationRedisClientProvider abstracts the Redis handle so tests
// can inject a miniredis-backed UniversalClient without standing up the
// full lib-commons RedisConnection.
type cacheInvalidationRedisClientProvider interface {
	GetClient(ctx context.Context) (redis.UniversalClient, error)
}

// cacheInvalidationKafkaPublisher abstracts the DLQ producer so tests can
// avoid booting a real broker. Any type with ProduceSync on kgo.Record
// satisfies this (kgo.Client does).
type cacheInvalidationKafkaPublisher interface {
	ProduceSync(ctx context.Context, rs ...*kgo.Record) kgo.ProduceResults
}

// CacheInvalidationRunner consumes crossshardevents.CacheInvalidationEvent
// payloads from TopicCacheInvalidation and issues targeted DELs against the
// transaction service's Redis cache.
//
// Lifecycle:
//   - NewCacheInvalidationRunner wires dependencies and a franz-go consumer
//     group. The client is NOT started; Run() drives the poll loop.
//   - Close() signals Run() to exit (idempotent) and closes the underlying
//     kgo client. Mirrors the authorizer recovery runner's pattern.
//
// Offset semantics:
//   - Transient Redis errors DO NOT commit the offset; the record is
//     re-observed next poll (up to maxTransientRetries, then DLQ).
//   - Malformed payloads commit the offset immediately (poison record) and
//     are routed to the DLQ.
//   - Successful DEL (including miss) commits the offset.
type CacheInvalidationRunner struct {
	client        *kgo.Client
	dlqPublisher  cacheInvalidationKafkaPublisher
	redisProvider cacheInvalidationRedisClientProvider
	logger        libLog.Logger

	sourceTopic   string
	dlqTopic      string
	consumerGroup string
	shardCount    int
	pollTimeout   time.Duration
	maxRetries    int
	transientBase time.Duration
	transientMax  time.Duration

	// Metrics (optional — zero-value safe).
	appliedCounter metric.Int64Counter
	dlqCounter     metric.Int64Counter

	stopping atomic.Bool
	running  atomic.Bool

	// shutdownWaitTimeout bounds Close() waiting on Run() completion.
	shutdownWaitTimeout time.Duration

	mu sync.Mutex
}

// CacheInvalidationRunnerConfig captures the wiring inputs. Using a struct
// keeps the constructor signature stable as new dependencies are added.
type CacheInvalidationRunnerConfig struct {
	// SeedBrokers MUST be non-empty.
	SeedBrokers []string

	// SecurityOptions carries the TLS/SASL options built elsewhere (matches
	// the producer / other consumers in this service). Can be nil when
	// running against a plain-text local broker.
	SecurityOptions []kgo.Opt

	// ConsumerGroup overrides defaultCacheInvalidationConsumerGroup.
	ConsumerGroup string

	// SourceTopic defaults to crossshardevents.TopicCacheInvalidation.
	// Tests may override.
	SourceTopic string

	// RedisProvider returns the Redis client used for DEL operations.
	RedisProvider cacheInvalidationRedisClientProvider

	// Logger MUST be non-nil.
	Logger libLog.Logger

	// ShardCount informs the shard-aware key derivation. Zero / negative
	// disables the shard-aware key (only the non-sharded key is DEL'd).
	ShardCount int

	// PollTimeout overrides defaultCacheInvalidationPollTimeout.
	PollTimeout time.Duration

	// MaxTransientRetries overrides the default retry budget.
	MaxTransientRetries int

	// MeterProvider is used to register metric instruments. Nil disables
	// metrics (instruments default to nil and are no-ops on record).
	Meter metric.Meter
}

// NewCacheInvalidationRunner constructs the runner. Returns an error if
// mandatory fields are missing OR the underlying kgo client cannot be
// created.
func NewCacheInvalidationRunner(cfg CacheInvalidationRunnerConfig) (*CacheInvalidationRunner, error) {
	if len(cfg.SeedBrokers) == 0 {
		return nil, errCacheInvalidationSeedBrokersRequired
	}

	if cfg.RedisProvider == nil {
		return nil, errCacheInvalidationRedisProviderRequired
	}

	if cfg.Logger == nil {
		return nil, errCacheInvalidationLoggerRequired
	}

	group := strings.TrimSpace(cfg.ConsumerGroup)
	if group == "" {
		group = defaultCacheInvalidationConsumerGroup
	}

	source := strings.TrimSpace(cfg.SourceTopic)
	if source == "" {
		source = crossshardevents.TopicCacheInvalidation
	}

	pollTimeout := cfg.PollTimeout
	if pollTimeout <= 0 {
		pollTimeout = defaultCacheInvalidationPollTimeout
	}

	maxRetries := cfg.MaxTransientRetries
	if maxRetries <= 0 {
		maxRetries = defaultCacheInvalidationMaxTransientRetries
	}

	baseOpts := []kgo.Opt{
		kgo.SeedBrokers(cfg.SeedBrokers...),
		kgo.ConsumerGroup(group),
		kgo.ConsumeTopics(source),
		kgo.DisableAutoCommit(),
		kgo.ConsumeResetOffset(kgo.NewOffset().AtEnd()),
	}

	opts := make([]kgo.Opt, 0, len(baseOpts)+len(cfg.SecurityOptions))
	opts = append(opts, baseOpts...)
	opts = append(opts, cfg.SecurityOptions...)

	client, err := kgo.NewClient(opts...)
	if err != nil {
		return nil, fmt.Errorf("cache invalidation runner: create kgo client: %w", err)
	}

	r := &CacheInvalidationRunner{
		client:              client,
		dlqPublisher:        client,
		redisProvider:       cfg.RedisProvider,
		logger:              cfg.Logger,
		sourceTopic:         source,
		dlqTopic:            source + cacheInvalidationDLQSuffix,
		consumerGroup:       group,
		shardCount:          cfg.ShardCount,
		pollTimeout:         pollTimeout,
		maxRetries:          maxRetries,
		transientBase:       defaultCacheInvalidationTransientRetryBase,
		transientMax:        defaultCacheInvalidationTransientRetryMax,
		shutdownWaitTimeout: cacheInvalidationShutdownDefaultTimeout,
	}

	if cfg.Meter != nil {
		applied, applyErr := cfg.Meter.Int64Counter(
			"transaction_cache_invalidation_applied_total",
			metric.WithDescription("Cache invalidation operations applied per source/result."),
		)
		if applyErr == nil {
			r.appliedCounter = applied
		}

		dlq, dlqErr := cfg.Meter.Int64Counter(
			"transaction_cache_invalidation_dlq_total",
			metric.WithDescription("Cache invalidation records routed to the DLQ per reason."),
		)
		if dlqErr == nil {
			r.dlqCounter = dlq
		}
	}

	return r, nil
}

// Close stops the runner. Idempotent.
func (r *CacheInvalidationRunner) Close() {
	if r == nil {
		return
	}

	r.stopping.Store(true)

	deadline := time.Now().Add(r.shutdownWaitTimeout)
	for r.running.Load() && time.Now().Before(deadline) {
		time.Sleep(cacheInvalidationShutdownPollInterval)
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if r.client != nil {
		r.client.Close()
		r.client = nil
	}
}

// Run starts the consumer loop. Blocks until ctx is cancelled or Close() is
// invoked. Safe to call only once per runner instance.
func (r *CacheInvalidationRunner) Run(ctx context.Context) {
	if r == nil {
		return
	}

	r.mu.Lock()
	client := r.client
	r.mu.Unlock()

	if client == nil {
		return
	}

	r.running.Store(true)
	defer r.running.Store(false)

	r.logger.Infof(
		"cache-invalidation consumer started: group=%s topic=%s",
		r.consumerGroup, r.sourceTopic,
	)

	for {
		if ctx.Err() != nil {
			return
		}

		if r.stopping.Load() {
			r.logger.Infof("cache-invalidation consumer stopping (Close signalled)")
			return
		}

		pollCtx, cancel := context.WithTimeout(ctx, r.pollTimeout)

		fetches := client.PollFetches(pollCtx)

		cancel()

		if ctx.Err() != nil {
			return
		}

		r.logFetchErrors(fetches)

		r.processFetches(ctx, client, fetches)
	}
}

func (r *CacheInvalidationRunner) logFetchErrors(fetches kgo.Fetches) {
	for _, fetchErr := range fetches.Errors() {
		if errors.Is(fetchErr.Err, context.Canceled) || errors.Is(fetchErr.Err, context.DeadlineExceeded) {
			continue
		}

		r.logger.Warnf(
			"cache-invalidation poll error: topic=%s partition=%d err=%v",
			fetchErr.Topic, fetchErr.Partition, fetchErr.Err,
		)
	}
}

func (r *CacheInvalidationRunner) processFetches(ctx context.Context, client *kgo.Client, fetches kgo.Fetches) {
	iter := fetches.RecordIter()
	for !iter.Done() {
		record := iter.Next()
		r.processRecord(ctx, client, record)
	}
}

// processRecord handles a single Kafka record end-to-end. Flow:
//
//  1. Unmarshal the event. Malformed → DLQ + commit offset.
//  2. Parse org/ledger UUIDs. Invalid → DLQ + commit offset.
//  3. For each (alias, balance_key) target, DEL both the non-sharded and
//     shard-aware keys. Transient Redis errors are retried in-place with
//     exponential backoff up to maxTransientRetries; on exhaustion the
//     record is DLQ'd and the offset is committed so the consumer can
//     continue with newer offsets.
//  4. On total success (all aliases applied), commit the offset.
func (r *CacheInvalidationRunner) processRecord(ctx context.Context, client *kgo.Client, record *kgo.Record) {
	if record == nil || len(record.Value) == 0 {
		r.commitOffset(ctx, client, record)
		return
	}

	var evt crossshardevents.CacheInvalidationEvent
	if err := json.Unmarshal(record.Value, &evt); err != nil {
		r.recordDLQ(ctx, cacheInvalidationDLQReasonMalformed)
		r.logger.Warnf(
			"cache-invalidation malformed payload: partition=%d offset=%d err=%v",
			record.Partition, record.Offset, err,
		)

		r.routeToDLQ(ctx, client, record, cacheInvalidationDLQReasonMalformed, err)
		r.commitOffset(ctx, client, record)

		return
	}

	orgID, ledgerID, idErr := parseOrgLedgerIDs(evt.OrganizationID, evt.LedgerID)
	if idErr != nil {
		r.recordDLQ(ctx, cacheInvalidationDLQReasonInvalidOrgOrLedger)
		r.logger.Warnf(
			"cache-invalidation invalid org/ledger: tx=%s org=%q ledger=%q err=%v",
			evt.TransactionID, evt.OrganizationID, evt.LedgerID, idErr,
		)

		r.routeToDLQ(ctx, client, record, cacheInvalidationDLQReasonInvalidOrgOrLedger, idErr)
		r.commitOffset(ctx, client, record)

		return
	}

	if len(evt.Aliases) == 0 {
		// Empty Aliases is a documented safety-net: producer could not
		// derive targets. Consumer logs + ACKs without scanning — keeping
		// a ledger-wide scan out of the steady-state path. Operators can
		// alert on this via the log line.
		r.logger.Infof(
			"cache-invalidation event with empty Aliases (no-op): tx=%s org=%s ledger=%s reason=%s",
			evt.TransactionID, evt.OrganizationID, evt.LedgerID, evt.Reason,
		)
		r.commitOffset(ctx, client, record)

		return
	}

	applyErr := r.applyEventWithRetry(ctx, &evt, orgID, ledgerID)
	if applyErr != nil {
		r.recordDLQ(ctx, cacheInvalidationDLQReasonRetriesExhausted)
		r.logger.Errorf(
			"cache-invalidation retries exhausted, routing to DLQ: tx=%s org=%s ledger=%s err=%v",
			evt.TransactionID, evt.OrganizationID, evt.LedgerID, applyErr,
		)

		r.routeToDLQ(ctx, client, record, cacheInvalidationDLQReasonRetriesExhausted, applyErr)
	}

	r.commitOffset(ctx, client, record)
}

// applyEventWithRetry runs the DEL fan-out with bounded per-event
// parallelism and exponential backoff between attempts. Returns nil on
// success (including all-miss), or the last error after retries are
// exhausted.
func (r *CacheInvalidationRunner) applyEventWithRetry(
	ctx context.Context,
	evt *crossshardevents.CacheInvalidationEvent,
	orgID, ledgerID uuid.UUID,
) error {
	var lastErr error

	for attempt := 0; attempt <= r.maxRetries; attempt++ {
		if ctx.Err() != nil {
			return fmt.Errorf("cache-invalidation apply aborted: %w", ctx.Err())
		}

		err := r.applyEventOnce(ctx, evt, orgID, ledgerID)
		if err == nil {
			return nil
		}

		lastErr = err

		if attempt == r.maxRetries {
			break
		}

		delay := r.backoffDelay(attempt)
		r.logger.Warnf(
			"cache-invalidation transient failure, retrying: tx=%s attempt=%d/%d delay=%s err=%v",
			evt.TransactionID, attempt+1, r.maxRetries+1, delay, err,
		)

		select {
		case <-time.After(delay):
		case <-ctx.Done():
			return fmt.Errorf("cache-invalidation apply aborted: %w", ctx.Err())
		}
	}

	return lastErr
}

// backoffDelay returns the delay for a given 0-based attempt index.
func (r *CacheInvalidationRunner) backoffDelay(attempt int) time.Duration {
	if attempt < 0 {
		attempt = 0
	}

	delay := r.transientBase * (1 << attempt)
	if delay > r.transientMax {
		delay = r.transientMax
	}

	return delay
}

// applyEventOnce does one pass of DELs. On any error it aborts and returns;
// the caller retries the whole pass. DEL is idempotent so partial progress
// is harmless on retry.
func (r *CacheInvalidationRunner) applyEventOnce(
	ctx context.Context,
	evt *crossshardevents.CacheInvalidationEvent,
	orgID, ledgerID uuid.UUID,
) error {
	rds, err := r.redisProvider.GetClient(ctx)
	if err != nil {
		return fmt.Errorf("get redis client: %w", err)
	}

	// Bounded parallelism. A small worker pool avoids both serial latency
	// (one RTT per alias) and fan-out explosion if a producer bug emits
	// thousands of aliases per event.
	workerCount := cacheInvalidationMaxAliasWorkers
	if len(evt.Aliases) < workerCount {
		workerCount = len(evt.Aliases)
	}

	jobs := make(chan crossshardevents.CacheInvalidationTarget, len(evt.Aliases))
	errs := make(chan error, len(evt.Aliases))

	var wg sync.WaitGroup

	for i := 0; i < workerCount; i++ {
		wg.Add(1)

		go func() {
			defer wg.Done()

			for target := range jobs {
				if err := r.deleteTarget(ctx, rds, orgID, ledgerID, target); err != nil {
					errs <- err
					return
				}
			}
		}()
	}

	for _, t := range evt.Aliases {
		jobs <- t
	}

	close(jobs)
	wg.Wait()
	close(errs)

	for e := range errs {
		if e != nil {
			return e
		}
	}

	return nil
}

// deleteTarget DELs both the non-sharded and (when shard-aware routing is
// enabled) every shard-aware key for (orgID, ledgerID, alias#balance_key).
//
// The composite "{alias}#{balance_key}" matches the key shape used by
// BalanceInternalKey call sites throughout the transaction service (see
// get-id-balance.go, shard_routing.go, stale_balance_recovery_flow.go).
//
// Shard-aware deletion iterates across shardCount because at invalidation
// time the consumer does not know which shard currently owns the balance —
// DEL is cheap and idempotent, so issuing N DELs across all shards is
// acceptable. If a key exists on exactly one shard, N-1 are misses.
func (r *CacheInvalidationRunner) deleteTarget(
	ctx context.Context,
	rds redis.UniversalClient,
	orgID, ledgerID uuid.UUID,
	target crossshardevents.CacheInvalidationTarget,
) error {
	aliasKey := target.Alias + "#" + target.BalanceKey

	// Non-sharded key (always emitted — present in legacy / non-shard-aware
	// deployments and in cases where a write path emitted the "{transactions}"
	// hash-tag).
	nonShardedKey := utils.BalanceInternalKey(orgID, ledgerID, aliasKey)
	if err := r.delKey(ctx, rds, nonShardedKey); err != nil {
		return fmt.Errorf("del non-sharded key %s: %w", nonShardedKey, err)
	}

	if r.shardCount <= 0 {
		return nil
	}

	// Shard-aware keys. We do not know which shardID holds this alias, so
	// we issue DEL against every shard. The worst case is N-1 misses per
	// target — acceptable because DEL is cheap and correctness > efficiency
	// (a stale hit on the wrong shard is very rare in practice).
	//
	// Defense in depth: cap the shard iteration to an upper bound so a
	// misconfigured shardCount cannot cause pathological DEL fan-out. 256
	// is chosen because it exceeds every realistic deployment (current
	// production max is ~32 shards per the sharding docs).
	const shardIterationCeiling = 256

	maxShards := r.shardCount
	if maxShards > shardIterationCeiling {
		maxShards = shardIterationCeiling
	}

	for shardID := 0; shardID < maxShards; shardID++ {
		shardKey := utils.BalanceShardKey(shardID, orgID, ledgerID, aliasKey)
		if err := r.delKey(ctx, rds, shardKey); err != nil {
			return fmt.Errorf("del shard-aware key %s: %w", shardKey, err)
		}
	}

	return nil
}

// delKey issues a single DEL and records the applied metric. A miss (deleted
// count == 0) is a valid outcome — the TTL expired or the cache never held
// the entry — and is recorded as result=miss.
func (r *CacheInvalidationRunner) delKey(ctx context.Context, rds redis.UniversalClient, key string) error {
	res := rds.Del(ctx, key)
	if err := res.Err(); err != nil {
		r.recordApplied(ctx, cacheInvalidationResultError)
		return err //nolint:wrapcheck // caller wraps with key context
	}

	if res.Val() > 0 {
		r.recordApplied(ctx, cacheInvalidationResultSuccess)
	} else {
		r.recordApplied(ctx, cacheInvalidationResultMiss)
	}

	return nil
}

func (r *CacheInvalidationRunner) recordApplied(ctx context.Context, result string) {
	if r == nil || r.appliedCounter == nil {
		return
	}

	r.appliedCounter.Add(
		ctx,
		1,
		metric.WithAttributes(
			attribute.String("source", "authorizer"),
			attribute.String("result", result),
		),
	)
}

func (r *CacheInvalidationRunner) recordDLQ(ctx context.Context, reason string) {
	if r == nil || r.dlqCounter == nil {
		return
	}

	r.dlqCounter.Add(
		ctx,
		1,
		metric.WithAttributes(attribute.String("reason", reason)),
	)
}

// commitOffset marks and commits the record's offset. Best-effort: a commit
// failure logs but does not block the consumer loop.
func (r *CacheInvalidationRunner) commitOffset(ctx context.Context, client *kgo.Client, record *kgo.Record) {
	if record == nil || client == nil {
		return
	}

	commitCtx, cancel := context.WithTimeout(ctx, cacheInvalidationCommitTimeout)
	defer cancel()

	if err := client.CommitRecords(commitCtx, record); err != nil {
		r.logger.Warnf(
			"cache-invalidation commit offset failed: partition=%d offset=%d err=%v",
			record.Partition, record.Offset, err,
		)
	}
}

// routeToDLQ publishes the poison record to the DLQ topic. Best-effort.
func (r *CacheInvalidationRunner) routeToDLQ(ctx context.Context, client *kgo.Client, record *kgo.Record, reason string, cause error) {
	if r == nil || r.dlqPublisher == nil || record == nil {
		return
	}

	headers := make([]kgo.RecordHeader, 0, len(record.Headers)+cacheInvalidationDLQHeaderCapacity)
	for _, h := range record.Headers {
		headers = append(headers, kgo.RecordHeader{Key: h.Key, Value: h.Value})
	}

	headers = append(headers, kgo.RecordHeader{
		Key: cacheInvalidationDLQReasonHeader, Value: []byte(reason),
	})

	if cause != nil {
		headers = append(headers, kgo.RecordHeader{
			Key:   cacheInvalidationDLQCauseHeader,
			Value: []byte(strings.TrimSpace(cause.Error())),
		})
	}

	headers = append(headers, kgo.RecordHeader{
		Key:   cacheInvalidationOriginalOffsetHeader,
		Value: []byte(strconv.FormatInt(record.Offset, 10)),
	})

	dlqRecord := &kgo.Record{
		Topic:     r.dlqTopic,
		Key:       record.Key,
		Value:     record.Value,
		Headers:   headers,
		Timestamp: time.Now().UTC(),
	}

	publishCtx, cancel := context.WithTimeout(ctx, cacheInvalidationDLQPublishTimeout)
	defer cancel()

	_ = client // publisher is client by default; kept for tests overriding dlqPublisher

	if err := r.dlqPublisher.ProduceSync(publishCtx, dlqRecord).FirstErr(); err != nil {
		r.logger.Errorf(
			"cache-invalidation DLQ publish failed: topic=%s partition=%d offset=%d err=%v",
			r.dlqTopic, record.Partition, record.Offset, err,
		)
	}
}

// parseOrgLedgerIDs validates and parses the two UUID strings carried on
// the event envelope. A parse failure is permanent: the record is DLQ'd.
func parseOrgLedgerIDs(orgStr, ledgerStr string) (uuid.UUID, uuid.UUID, error) {
	var zero uuid.UUID

	orgID, err := uuid.Parse(strings.TrimSpace(orgStr))
	if err != nil {
		return zero, zero, fmt.Errorf("parse organization_id: %w", err)
	}

	ledgerID, err := uuid.Parse(strings.TrimSpace(ledgerStr))
	if err != nil {
		return zero, zero, fmt.Errorf("parse ledger_id: %w", err)
	}

	return orgID, ledgerID, nil
}

// Compile-time assertion that *libRedis.RedisConnection satisfies the
// provider interface. Prevents a silent break if lib-commons changes the
// GetClient signature.
var _ cacheInvalidationRedisClientProvider = (*libRedis.RedisConnection)(nil)
