// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package redpanda

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/twmb/franz-go/pkg/kgo"
	attribute "go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
	"golang.org/x/time/rate"

	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	libConstants "github.com/LerianStudio/lib-commons/v2/commons/constants"
	libLog "github.com/LerianStudio/lib-commons/v2/commons/log"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v2/commons/opentelemetry"
	libZap "github.com/LerianStudio/lib-commons/v2/commons/zap"
)

const (
	defaultConsumerGroup    = "midaz-balance-projector"
	defaultConsumerWorkers  = 5
	defaultPartitionBufSize = 128
	defaultPartitionHint    = 8
	defaultWorkerTicker     = 5 * time.Millisecond
	defaultCommitInterval   = time.Second
	defaultMaxRetryAttempts = 3
	defaultRerouteAttempts  = 3
	retryTopicSuffix        = ".retry"
	dltTopicSuffix          = ".dlt"
	retryAttemptHeader      = "x-midaz-retry-attempt"

	// commitTimeout is the timeout for committing marked offsets.
	commitTimeout = 5 * time.Second
	// publishTimeout is the timeout for publishing a failed record.
	publishTimeout = 5 * time.Second
	// dispatchRetryAttempts is the number of times to retry dispatching to a partition pool.
	dispatchRetryAttempts = 2
	// rerouteDelayMultiplier is the base delay multiplied by attempt number for reroute backoff.
	rerouteDelayMultiplier = 200 * time.Millisecond
)

var (
	// ErrConsumerRoutesNil is returned when consumer routes are nil.
	ErrConsumerRoutesNil = errors.New("consumer routes are nil")
	// ErrCannotDispatchNilRecord is returned when a nil record is dispatched.
	ErrCannotDispatchNilRecord = errors.New("cannot dispatch nil record")
	// ErrCannotRouteNilRecord is returned when a nil record is routed.
	ErrCannotRouteNilRecord = errors.New("cannot route failed message: record is nil")
	// ErrPartitionPoolUnavailable is returned when a partition worker pool is not available for dispatch.
	ErrPartitionPoolUnavailable = errors.New("partition worker pool unavailable")
)

// workerAction represents the action the worker loop should take after handling a job.
type workerAction int

const (
	workerActionNone     workerAction = iota // proceed normally
	workerActionContinue                     // skip to next select iteration
	workerActionReturn                       // exit the worker loop
)

// ConsumerRepository provides an interface for broker consumers.
type ConsumerRepository interface {
	Register(topicName string, handler QueueHandlerFunc)
	RegisterBatch(topicName string, handler BatchQueueHandlerFunc)
	RunConsumers() error
}

// QueueHandlerFunc processes a specific topic payload.
type QueueHandlerFunc func(ctx context.Context, body []byte) error

// BatchQueueHandlerFunc processes a batch of payloads from the same topic.
type BatchQueueHandlerFunc func(ctx context.Context, bodies [][]byte) error

type queuedRecord struct {
	handler      QueueHandlerFunc
	batchHandler BatchQueueHandlerFunc
	record       *kgo.Record
}

// BatchFailedRecordIndexer allows batch handlers to signal which records failed
// so fallback can replay only those records individually.
type BatchFailedRecordIndexer interface {
	FailedRecordIndexes() []int
}

type partitionWorkerPool struct {
	partition int32
	ch        chan queuedRecord
	done      chan struct{}
	stopOnce  sync.Once
}

func (p *partitionWorkerPool) stop() {
	if p == nil {
		return
	}

	p.stopOnce.Do(func() {
		close(p.done)
	})
}

// ConsumerRoutes runs topic handlers backed by a Redpanda consumer group.
type ConsumerRoutes struct {
	brokers         []string
	consumerGroup   string
	routes          map[string]QueueHandlerFunc
	batchRoutes     map[string]BatchQueueHandlerFunc
	NumbersOfWorker int
	FetchMaxBytes   int
	libLog.Logger
	libOpentelemetry.Telemetry

	ctx      context.Context
	cancel   context.CancelFunc
	stopOnce sync.Once

	client *kgo.Client

	securityConfig   ClientSecurityConfig
	maxRetryAttempts int

	partitionPools   map[int32]*partitionWorkerPool
	partitionPoolsMu sync.RWMutex
	partitionHint    int
	partitionBufSize int

	commitInterval time.Duration
	dbLimiter      *rate.Limiter

	batchEnabled bool
	batchSize    int
	batchWindow  time.Duration
	idleFlush    time.Duration

	batchImmediateCommit bool
}

// NewConsumerRoutes creates a new instance of ConsumerRoutes.
func NewConsumerRoutes(
	brokers []string,
	consumerGroup string,
	numbersOfWorkers int,
	fetchMaxBytes int,
	logger libLog.Logger,
	telemetry *libOpentelemetry.Telemetry,
) *ConsumerRoutes {
	return NewConsumerRoutesWithSecurity(
		brokers,
		consumerGroup,
		numbersOfWorkers,
		fetchMaxBytes,
		logger,
		telemetry,
		ClientSecurityConfig{},
		defaultMaxRetryAttempts,
	)
}

// NewConsumerRoutesWithSecurity creates a new instance of ConsumerRoutes with optional TLS/SASL.
func NewConsumerRoutesWithSecurity(
	brokers []string,
	consumerGroup string,
	numbersOfWorkers int,
	fetchMaxBytes int,
	logger libLog.Logger,
	telemetry *libOpentelemetry.Telemetry,
	securityConfig ClientSecurityConfig,
	maxRetryAttempts int,
) *ConsumerRoutes {
	if numbersOfWorkers <= 0 {
		numbersOfWorkers = defaultConsumerWorkers
	}

	if consumerGroup == "" {
		consumerGroup = defaultConsumerGroup
	}

	if maxRetryAttempts <= 0 {
		maxRetryAttempts = defaultMaxRetryAttempts
	}

	runCtx, cancel := context.WithCancel(context.Background())

	if logger == nil {
		if l, err := libZap.InitializeLoggerWithError(); err == nil {
			logger = l
		}
	}

	telemetryValue := libOpentelemetry.Telemetry{}
	if telemetry != nil {
		telemetryValue = *telemetry
	}

	const (
		defaultBatchSize   = 50
		defaultBatchWindow = 10 * time.Millisecond
		defaultIdleFlush   = 100 * time.Millisecond
	)

	return &ConsumerRoutes{
		brokers:              brokers,
		consumerGroup:        consumerGroup,
		routes:               make(map[string]QueueHandlerFunc),
		batchRoutes:          make(map[string]BatchQueueHandlerFunc),
		NumbersOfWorker:      numbersOfWorkers,
		FetchMaxBytes:        fetchMaxBytes,
		Logger:               logger,
		Telemetry:            telemetryValue,
		ctx:                  runCtx,
		cancel:               cancel,
		securityConfig:       securityConfig,
		maxRetryAttempts:     maxRetryAttempts,
		partitionPools:       make(map[int32]*partitionWorkerPool),
		partitionHint:        defaultPartitionHint,
		partitionBufSize:     defaultPartitionBufSize,
		commitInterval:       defaultCommitInterval,
		batchSize:            defaultBatchSize,
		batchWindow:          defaultBatchWindow,
		idleFlush:            defaultIdleFlush,
		batchImmediateCommit: true,
	}
}

// SetPartitionWorkerHint is retained for API compatibility but has no effect.
// Workers per partition is always 1 to preserve Kafka ordering guarantees.
func (cr *ConsumerRoutes) SetPartitionWorkerHint(hint int) {
	if cr == nil || hint <= 0 {
		return
	}

	cr.partitionHint = hint
}

// SetPartitionBufferSize configures channel buffer size for each partition queue.
func (cr *ConsumerRoutes) SetPartitionBufferSize(size int) {
	if cr == nil || size <= 0 {
		return
	}

	cr.partitionBufSize = size
}

// SetCommitInterval configures periodic offset commit cadence.
func (cr *ConsumerRoutes) SetCommitInterval(interval time.Duration) {
	if cr == nil || interval <= 0 {
		return
	}

	cr.commitInterval = interval
}

// SetDBRateLimiter enables consumer-side DB backpressure.
func (cr *ConsumerRoutes) SetDBRateLimiter(maxTPS, burst int) {
	if cr == nil {
		return
	}

	if maxTPS <= 0 {
		cr.dbLimiter = nil
		return
	}

	if burst <= 0 {
		burst = maxTPS
	}

	cr.dbLimiter = rate.NewLimiter(rate.Limit(maxTPS), burst)
}

// Stop requests all consumer goroutines to stop.
func (cr *ConsumerRoutes) Stop() {
	if cr == nil || cr.cancel == nil {
		return
	}

	cr.stopOnce.Do(func() {
		cr.cancel()

		if cr.client != nil {
			cr.client.Close()
		}
	})
}

// Register adds a new topic handler.
func (cr *ConsumerRoutes) Register(topicName string, handler QueueHandlerFunc) {
	if cr == nil {
		return
	}

	if cr.routes == nil {
		cr.routes = make(map[string]QueueHandlerFunc)
	}

	cr.routes[topicName] = handler
}

// RegisterBatch adds a topic batch handler.
func (cr *ConsumerRoutes) RegisterBatch(topicName string, handler BatchQueueHandlerFunc) {
	if cr == nil {
		return
	}

	if cr.batchRoutes == nil {
		cr.batchRoutes = make(map[string]BatchQueueHandlerFunc)
	}

	cr.batchRoutes[topicName] = handler
}

// SetBatchConfig controls micro-batching behavior.
func (cr *ConsumerRoutes) SetBatchConfig(enabled bool, size int, window, idle time.Duration) {
	if cr == nil {
		return
	}

	cr.batchEnabled = enabled

	if size > 0 {
		cr.batchSize = size
	}

	if window > 0 {
		cr.batchWindow = window
	}

	if idle > 0 {
		cr.idleFlush = idle
	}
}

// SetBatchImmediateCommit controls whether workers request an immediate offset
// commit after each successful batch flush. This narrows replay windows in
// batch mode by reducing dependency on the periodic commit ticker.
func (cr *ConsumerRoutes) SetBatchImmediateCommit(enabled bool) {
	if cr == nil {
		return
	}

	cr.batchImmediateCommit = enabled
}

// RunConsumers starts the consumer workers.
func (cr *ConsumerRoutes) RunConsumers() error {
	if cr == nil {
		return ErrConsumerRoutesNil
	}

	if len(cr.routes) == 0 {
		cr.Warn("No Redpanda routes registered; skipping consumer startup")
		return nil
	}

	const topicsPerRoute = 2

	topicsSet := make(map[string]struct{}, len(cr.routes)*topicsPerRoute)
	for topic := range cr.routes {
		topicsSet[topic] = struct{}{}
		topicsSet[topic+retryTopicSuffix] = struct{}{}
	}

	topics := make([]string, 0, len(topicsSet))
	for topic := range topicsSet {
		topics = append(topics, topic)
	}

	options := []kgo.Opt{
		kgo.SeedBrokers(cr.brokers...),
		kgo.ConsumerGroup(cr.consumerGroup),
		kgo.ConsumeTopics(topics...),
		kgo.DisableAutoCommit(),
		kgo.Balancers(kgo.CooperativeStickyBalancer()),
		kgo.FetchIsolationLevel(kgo.ReadCommitted()),
		kgo.OnPartitionsRevoked(func(_ context.Context, _ *kgo.Client, revoked map[string][]int32) {
			cr.closePartitionPoolsForAssignments(revoked)
		}),
		kgo.OnPartitionsLost(func(_ context.Context, _ *kgo.Client, lost map[string][]int32) {
			cr.closePartitionPoolsForAssignments(lost)
		}),
	}

	if cr.FetchMaxBytes > 0 {
		fetchMaxBytes := cr.FetchMaxBytes
		if fetchMaxBytes > math.MaxInt32 {
			cr.Warnf("REDPANDA_FETCH_MAX_BYTES=%d exceeds int32 max; clamping to %d", fetchMaxBytes, math.MaxInt32)
			fetchMaxBytes = math.MaxInt32
		}

		options = append(options, kgo.FetchMaxBytes(int32(fetchMaxBytes)))
	}

	securityOptions, err := BuildSecurityOptions(cr.securityConfig)
	if err != nil {
		return fmt.Errorf("invalid redpanda security configuration: %w", err)
	}

	options = append(options, securityOptions...)

	client, err := kgo.NewClient(options...)
	if err != nil {
		return fmt.Errorf("failed to initialize redpanda consumer: %w", err)
	}

	cr.client = client

	go cr.startCommitLoop()
	go cr.pollLoop()

	return nil
}

func (cr *ConsumerRoutes) pollLoop() {
	defer cr.closeAllPartitionPools()

	for {
		if cr.ctx.Err() != nil {
			return
		}

		fetches := cr.client.PollFetches(cr.ctx)
		if fetches.IsClientClosed() {
			return
		}

		fetches.EachError(func(topic string, partition int32, err error) {
			cr.Errorf("Consumer fetch error topic=%s partition=%d err=%v", topic, partition, err)
		})

		iter := fetches.RecordIter()
		for !iter.Done() {
			record := iter.Next()

			handler, ok := cr.resolveHandler(record.Topic)
			if !ok {
				cr.Warnf("No handler registered for topic=%s", record.Topic)
				continue
			}

			batchHandler, _ := cr.resolveBatchHandler(record.Topic)

			if err := cr.dispatchToPartitionQueue(queuedRecord{handler: handler, batchHandler: batchHandler, record: record}); err != nil {
				cr.Errorf("Consumer dispatch error topic=%s partition=%d offset=%d err=%v", record.Topic, record.Partition, record.Offset, err)
				return
			}
		}
	}
}

func (cr *ConsumerRoutes) dispatchToPartitionQueue(job queuedRecord) error {
	if job.record == nil {
		return ErrCannotDispatchNilRecord
	}

	for attempt := 0; attempt < dispatchRetryAttempts; attempt++ {
		pool := cr.getOrCreatePartitionPool(job.record.Partition)

		select {
		case pool.ch <- job:
			return nil
		case <-pool.done:
			continue
		case <-cr.ctx.Done():
			return fmt.Errorf("consumer context done while dispatching: %w", cr.ctx.Err())
		}
	}

	return fmt.Errorf("%w: partition=%d", ErrPartitionPoolUnavailable, job.record.Partition)
}

func (cr *ConsumerRoutes) getOrCreatePartitionPool(partition int32) *partitionWorkerPool {
	cr.partitionPoolsMu.RLock()
	pool, ok := cr.partitionPools[partition]
	cr.partitionPoolsMu.RUnlock()

	if ok {
		return pool
	}

	cr.partitionPoolsMu.Lock()
	defer cr.partitionPoolsMu.Unlock()

	if pool, ok = cr.partitionPools[partition]; ok {
		return pool
	}

	pool = &partitionWorkerPool{
		partition: partition,
		ch:        make(chan queuedRecord, cr.partitionBufSize),
		done:      make(chan struct{}),
	}

	go cr.startWorker(int(partition), pool.ch, pool.done)

	cr.partitionPools[partition] = pool
	cr.Infof("Started partition worker pool partition=%d queue_buffer=%d", partition, cr.partitionBufSize)

	return pool
}

func (cr *ConsumerRoutes) closeAllPartitionPools() {
	cr.partitionPoolsMu.Lock()
	defer cr.partitionPoolsMu.Unlock()

	for partition, pool := range cr.partitionPools {
		pool.stop()
		delete(cr.partitionPools, partition)
	}
}

func (cr *ConsumerRoutes) closePartitionPoolsForAssignments(assignments map[string][]int32) {
	if cr == nil || len(assignments) == 0 {
		return
	}

	seen := make(map[int32]struct{})

	for _, partitions := range assignments {
		for _, partition := range partitions {
			if _, ok := seen[partition]; ok {
				continue
			}

			seen[partition] = struct{}{}
			cr.closePartitionPool(partition)
		}
	}
}

func (cr *ConsumerRoutes) closePartitionPool(partition int32) {
	cr.partitionPoolsMu.Lock()
	defer cr.partitionPoolsMu.Unlock()

	pool, ok := cr.partitionPools[partition]
	if !ok {
		return
	}

	pool.stop()
	delete(cr.partitionPools, partition)

	cr.Infof("Stopped partition worker pool partition=%d", partition)
}

func (cr *ConsumerRoutes) startWorker(workerID int, workCh <-chan queuedRecord, stop <-chan struct{}) {
	var (
		ticker *time.Ticker
		tickCh <-chan time.Time
	)

	if cr.batchEnabled {
		ticker = time.NewTicker(cr.resolveWorkerTickerInterval())

		tickCh = ticker.C
		defer ticker.Stop()
	}

	batch := make([]queuedRecord, 0, cr.batchSize)

	var (
		oldestAt    time.Time
		lastArrival time.Time
	)

	flushBatch := func() bool {
		if len(batch) == 0 {
			return true
		}

		processed := cr.processBatchRecords(workerID, batch)
		batch = batch[:0]
		oldestAt = time.Time{}
		lastArrival = time.Time{}

		return processed
	}

	for {
		select {
		case <-cr.ctx.Done():
			if !flushBatch() {
				return
			}

			cr.Warnf("Worker %d stopped (consumer cancelled)", workerID)

			return
		case <-stop:
			if !flushBatch() {
				return
			}

			if !cr.drainStoppedPartitionQueue(workerID, workCh) {
				return
			}

			cr.Warnf("Worker %d stopped (partition revoked)", workerID)

			return
		case <-tickCh:
			if cr.shouldFlushOnTick(len(batch), oldestAt, lastArrival) {
				if !flushBatch() {
					return
				}
			}
		case job, ok := <-workCh:
			action := cr.handleJobArrival(
				workerID, job, ok,
				&batch, &oldestAt, &lastArrival,
				flushBatch,
			)

			if action == workerActionReturn {
				return
			}

			if action == workerActionContinue {
				continue
			}
		}
	}
}

// handleJobArrival processes a newly arrived job in the worker loop.
func (cr *ConsumerRoutes) handleJobArrival(
	workerID int,
	job queuedRecord,
	ok bool,
	batch *[]queuedRecord,
	oldestAt *time.Time,
	lastArrival *time.Time,
	flushBatch func() bool,
) workerAction {
	if !ok {
		if !flushBatch() {
			return workerActionReturn
		}

		cr.Warnf("Worker %d stopped (consumer loop closed)", workerID)

		return workerActionReturn
	}

	if !cr.batchEnabled || job.batchHandler == nil {
		if !flushBatch() {
			return workerActionReturn
		}

		if !cr.processSingleRecord(workerID, job) {
			return workerActionReturn
		}

		return workerActionContinue
	}

	if len(*batch) > 0 && (*batch)[0].record.Topic != job.record.Topic {
		if !flushBatch() {
			return workerActionReturn
		}
	}

	if len(*batch) == 0 {
		*oldestAt = time.Now().UTC()
	}

	*lastArrival = time.Now().UTC()

	*batch = append(*batch, job)

	if len(*batch) >= cr.batchSize {
		if !flushBatch() {
			return workerActionReturn
		}
	}

	return workerActionNone
}

// shouldFlushOnTick checks if the current batch should be flushed based on age or idle time.
func (cr *ConsumerRoutes) shouldFlushOnTick(batchLen int, oldestAt, lastArrival time.Time) bool {
	if !cr.batchEnabled || batchLen == 0 {
		return false
	}

	now := time.Now().UTC()
	batchTooOld := !oldestAt.IsZero() && now.Sub(oldestAt) >= cr.batchWindow
	idle := !lastArrival.IsZero() && now.Sub(lastArrival) >= cr.idleFlush

	return batchTooOld || idle
}

func (cr *ConsumerRoutes) resolveWorkerTickerInterval() time.Duration {
	if cr == nil {
		return defaultWorkerTicker
	}

	interval := defaultWorkerTicker

	if cr.batchWindow > 0 && cr.batchWindow < interval {
		interval = cr.batchWindow
	}

	if cr.idleFlush > 0 && cr.idleFlush < interval {
		interval = cr.idleFlush
	}

	if interval < time.Millisecond {
		return time.Millisecond
	}

	return interval
}

func (cr *ConsumerRoutes) drainStoppedPartitionQueue(workerID int, workCh <-chan queuedRecord) bool {
	if cr == nil {
		return true
	}

	for drained := 0; drained < cr.partitionBufSize; drained++ {
		select {
		case job, ok := <-workCh:
			if !ok {
				return true
			}

			if !cr.processSingleRecord(workerID, job) {
				return false
			}
		default:
			return true
		}
	}

	return true
}

func (cr *ConsumerRoutes) processSingleRecord(workerID int, job queuedRecord) bool {
	ctx, logger, spanConsumer := cr.startRecordSpan(job)

	if cr.dbLimiter != nil {
		if waitErr := cr.dbLimiter.Wait(ctx); waitErr != nil {
			spanConsumer.End()
			logger.Warnf("Worker %d: Backpressure wait interrupted: %v", workerID, waitErr)

			return false
		}
	}

	err := job.handler(ctx, job.record.Value)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(&spanConsumer, "Error processing message", err)
		logger.Errorf("Worker %d: Error processing topic=%s err=%v", workerID, job.record.Topic, err)

		if isNonRetryablePayloadError(err) {
			logger.Warnf(
				"Worker %d: Dropping non-retryable malformed payload topic=%s partition=%d offset=%d err=%v",
				workerID,
				job.record.Topic,
				job.record.Partition,
				job.record.Offset,
				err,
			)

			spanConsumer.End()
			cr.markRecord(workerID, logger, job.record)

			return true
		}

		if routeErr := cr.routeFailedRecordWithRetry(ctx, job.record, err, logger); routeErr != nil {
			libOpentelemetry.HandleSpanError(&spanConsumer, "Failed to route message to retry/DLT", routeErr)
			spanConsumer.End()
			logger.Errorf("Worker %d: Failed to reroute message topic=%s partition=%d offset=%d err=%v", workerID, job.record.Topic, job.record.Partition, job.record.Offset, routeErr)
			cr.Stop()

			return false
		}

		spanConsumer.End()
		cr.markRecord(workerID, logger, job.record)

		return true
	}

	spanConsumer.End()
	cr.markRecord(workerID, logger, job.record)

	return true
}

func (cr *ConsumerRoutes) processBatchRecords(workerID int, batch []queuedRecord) bool {
	batch = cr.filterValidBatchRecords(workerID, batch)
	if len(batch) == 0 {
		return true
	}

	if batch[0].batchHandler == nil {
		return cr.processBatchAsSingleRecords(workerID, batch)
	}

	ctx, logger, spanConsumer := cr.startRecordSpan(batch[0])
	spanConsumer.SetAttributes(
		attribute.Int("app.request.redpanda.batch_size", len(batch)),
	)

	if cr.dbLimiter != nil {
		if waitErr := cr.dbLimiter.Wait(ctx); waitErr != nil {
			spanConsumer.End()
			logger.Warnf("Worker %d: Backpressure wait interrupted before batch: %v", workerID, waitErr)

			return false
		}
	}

	bodies := extractBatchBodies(batch)

	err := batch[0].batchHandler(ctx, bodies)
	if err != nil {
		ok := cr.handleBatchProcessingError(workerID, logger, &spanConsumer, batch, err)
		return ok
	}

	cr.markBatchRecords(workerID, logger, batch)
	cr.commitBatchIfEnabled(workerID, logger, batch)

	spanConsumer.End()

	return true
}

// filterValidBatchRecords removes records with nil kgo.Record from the batch,
// logging a warning for each dropped record.
func (cr *ConsumerRoutes) filterValidBatchRecords(workerID int, batch []queuedRecord) []queuedRecord {
	if len(batch) == 0 {
		return nil
	}

	validBatch := make([]queuedRecord, 0, len(batch))

	for _, job := range batch {
		if job.record == nil {
			cr.Warnf("Worker %d: Dropping nil record from batch", workerID)
			continue
		}

		validBatch = append(validBatch, job)
	}

	return validBatch
}

// processBatchAsSingleRecords falls back to processing each record individually
// when no batch handler is available.
func (cr *ConsumerRoutes) processBatchAsSingleRecords(workerID int, batch []queuedRecord) bool {
	for _, job := range batch {
		if !cr.processSingleRecord(workerID, job) {
			return false
		}
	}

	cr.commitBatchIfEnabled(workerID, cr.Logger, batch)

	return true
}

// extractBatchBodies collects the raw message bodies from each record in the batch.
func extractBatchBodies(batch []queuedRecord) [][]byte {
	bodies := make([][]byte, 0, len(batch))
	for _, job := range batch {
		bodies = append(bodies, job.record.Value)
	}

	return bodies
}

// handleBatchProcessingError handles errors from batch processing by either
// dropping non-retryable records or falling back to individual record processing.
func (cr *ConsumerRoutes) handleBatchProcessingError(
	workerID int,
	logger libLog.Logger,
	spanConsumer *trace.Span,
	batch []queuedRecord,
	err error,
) bool {
	libOpentelemetry.HandleSpanBusinessErrorEvent(spanConsumer, "Error processing batch", err)
	logger.Errorf("Worker %d: Error processing batch topic=%s size=%d err=%v", workerID, batch[0].record.Topic, len(batch), err)

	if isNonRetryablePayloadError(err) {
		cr.markBatchRecords(workerID, logger, batch)
		(*spanConsumer).End()

		return true
	}

	failedBatch := cr.handleBatchFailedRecords(workerID, logger, batch, err)

	for _, job := range failedBatch {
		if !cr.processSingleRecord(workerID, job) {
			(*spanConsumer).End()
			return false
		}
	}

	cr.commitBatchIfEnabled(workerID, logger, batch)
	(*spanConsumer).End()

	return true
}

// markBatchRecords marks all records in a batch as consumed.
func (cr *ConsumerRoutes) markBatchRecords(workerID int, logger libLog.Logger, batch []queuedRecord) {
	for _, job := range batch {
		cr.markRecord(workerID, logger, job.record)
	}
}

// handleBatchFailedRecords processes failed batch records by identifying specific failed indexes
// and falling back to individual processing for those records.
func (cr *ConsumerRoutes) handleBatchFailedRecords(
	workerID int,
	logger libLog.Logger,
	batch []queuedRecord,
	err error,
) (failedBatch []queuedRecord) {
	failedBatch = batch

	var indexedErr BatchFailedRecordIndexer
	if !errors.As(err, &indexedErr) {
		return failedBatch
	}

	failedIndexes := indexedErr.FailedRecordIndexes()
	if len(failedIndexes) == 0 {
		return failedBatch
	}

	failedIndexSet := make(map[int]struct{}, len(failedIndexes))

	selected := make([]queuedRecord, 0, len(failedIndexes))
	for _, failedIndex := range failedIndexes {
		if failedIndex < 0 || failedIndex >= len(batch) {
			continue
		}

		failedIndexSet[failedIndex] = struct{}{}

		selected = append(selected, batch[failedIndex])
	}

	if len(selected) == 0 {
		return failedBatch
	}

	for index, job := range batch {
		if _, isFailed := failedIndexSet[index]; isFailed {
			continue
		}

		cr.markRecord(workerID, logger, job.record)
	}

	return selected
}

func (cr *ConsumerRoutes) commitBatchIfEnabled(workerID int, logger libLog.Logger, batch []queuedRecord) {
	if cr == nil || !cr.batchEnabled || !cr.batchImmediateCommit || len(batch) == 0 {
		return
	}

	cr.commitMarkedOffsets()

	if logger != nil && batch[0].record != nil {
		logger.Debugf("Worker %d: Requested immediate commit for batch topic=%s size=%d", workerID, batch[0].record.Topic, len(batch))
	}
}

func (cr *ConsumerRoutes) startRecordSpan(job queuedRecord) (context.Context, libLog.Logger, trace.Span) {
	record := job.record
	if record == nil {
		record = &kgo.Record{}
	}

	midazID := resolveHeader(record.Headers, libConstants.HeaderID)
	if midazID == "" {
		midazID = libCommons.GenerateUUIDv7().String()
	}

	baseLogger := cr.Logger
	if baseLogger == nil {
		if l, err := libZap.InitializeLoggerWithError(); err == nil {
			baseLogger = l
		}
	}

	log := baseLogger.WithFields(
		libConstants.HeaderID, midazID,
	).WithDefaultMessageTemplate(midazID + libConstants.LoggerDefaultSeparator)

	ctx := libCommons.ContextWithLogger(
		libCommons.ContextWithHeaderID(context.Background(), midazID),
		log,
	)

	headerMap := make(map[string]any, len(record.Headers))
	for _, header := range record.Headers {
		headerMap[header.Key] = string(header.Value)
	}

	ctx = libOpentelemetry.ExtractTraceContextFromQueueHeaders(ctx, headerMap)

	logger, tracer, reqID, _ := libCommons.NewTrackingFromContext(ctx)
	ctx, spanConsumer := tracer.Start(ctx, "redpanda.consumer.process_message")
	ctx = libCommons.ContextWithSpanAttributes(ctx, attribute.String("app.request.request_id", reqID))
	spanConsumer.SetAttributes(
		attribute.String("app.request.redpanda.topic", record.Topic),
		attribute.Int64("app.request.redpanda.partition", int64(record.Partition)),
		attribute.Int64("app.request.redpanda.offset", record.Offset),
		attribute.Int("app.request.redpanda.payload_size_bytes", len(record.Value)),
		attribute.Int("app.request.redpanda.headers_count", len(record.Headers)),
	)

	return ctx, logger, spanConsumer
}

func (cr *ConsumerRoutes) markRecord(workerID int, logger libLog.Logger, record *kgo.Record) {
	if cr == nil || cr.client == nil || record == nil {
		return
	}

	cr.client.MarkCommitRecords(record)
	logger.Debugf("Worker %d: Marked topic=%s partition=%d offset=%d for commit", workerID, record.Topic, record.Partition, record.Offset)
}

func resolveHeader(headers []kgo.RecordHeader, key string) string {
	for _, header := range headers {
		if header.Key != key {
			continue
		}

		resolved := string(header.Value)
		if resolved == "" {
			continue
		}

		return resolved
	}

	return ""
}

func (cr *ConsumerRoutes) resolveHandler(topic string) (QueueHandlerFunc, bool) {
	handler, ok := cr.routes[topic]
	if ok {
		return handler, true
	}

	if !strings.HasSuffix(topic, retryTopicSuffix) {
		return nil, false
	}

	baseTopic := strings.TrimSuffix(topic, retryTopicSuffix)

	handler, ok = cr.routes[baseTopic]

	return handler, ok
}

func (cr *ConsumerRoutes) resolveBatchHandler(topic string) (BatchQueueHandlerFunc, bool) {
	handler, ok := cr.batchRoutes[topic]
	if ok {
		return handler, true
	}

	if !strings.HasSuffix(topic, retryTopicSuffix) {
		return nil, false
	}

	baseTopic := strings.TrimSuffix(topic, retryTopicSuffix)

	handler, ok = cr.batchRoutes[baseTopic]

	return handler, ok
}

func (cr *ConsumerRoutes) startCommitLoop() {
	commitInterval := cr.commitInterval
	if commitInterval <= 0 {
		commitInterval = defaultCommitInterval
	}

	ticker := time.NewTicker(commitInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			cr.commitMarkedOffsets()
		case <-cr.ctx.Done():
			cr.commitMarkedOffsets()
			return
		}
	}
}

func (cr *ConsumerRoutes) commitMarkedOffsets() {
	if cr == nil || cr.client == nil {
		return
	}

	commitCtx, cancel := context.WithTimeout(context.Background(), commitTimeout)
	defer cancel()

	if err := cr.client.CommitMarkedOffsets(commitCtx); err != nil {
		cr.Errorf("Failed to commit marked offsets: %v", err)
	}
}

func (cr *ConsumerRoutes) routeFailedRecord(ctx context.Context, record *kgo.Record, handlerErr error, logger libLog.Logger) error {
	if record == nil {
		return ErrCannotRouteNilRecord
	}

	attempt := parseRetryAttempt(record.Headers)
	nextAttempt := attempt + 1
	targetTopic := resolveFailedRecordTargetTopic(record.Topic, nextAttempt, cr.maxRetryAttempts)

	headers := cloneHeaders(record.Headers)
	headers = upsertHeader(headers, retryAttemptHeader, []byte(strconv.Itoa(nextAttempt)))
	headers = upsertHeader(headers, "x-midaz-handler-error", []byte(handlerErr.Error()))

	failedRecord := &kgo.Record{
		Topic:     targetTopic,
		Key:       record.Key,
		Value:     record.Value,
		Headers:   headers,
		Timestamp: time.Now().UTC(),
	}

	publishCtx, cancel := context.WithTimeout(ctx, publishTimeout)
	defer cancel()

	if err := cr.client.ProduceSync(publishCtx, failedRecord).FirstErr(); err != nil {
		return fmt.Errorf("publish failed message to topic %s: %w", targetTopic, err)
	}

	logger.Warnf(
		"Rerouted failed message from topic=%s partition=%d offset=%d to topic=%s attempt=%d",
		record.Topic,
		record.Partition,
		record.Offset,
		targetTopic,
		nextAttempt,
	)

	return nil
}

func (cr *ConsumerRoutes) routeFailedRecordWithRetry(ctx context.Context, record *kgo.Record, handlerErr error, logger libLog.Logger) error {
	var lastErr error

	topic := "<nil>"
	partition := int32(-1)
	offset := int64(-1)

	if record != nil {
		topic = record.Topic
		partition = record.Partition
		offset = record.Offset
	}

	for attempt := 1; attempt <= defaultRerouteAttempts; attempt++ {
		routeErr := cr.routeFailedRecord(ctx, record, handlerErr, logger)
		if routeErr == nil {
			return nil
		}

		lastErr = routeErr

		if attempt == defaultRerouteAttempts {
			break
		}

		delay := time.Duration(attempt) * rerouteDelayMultiplier
		logger.Warnf(
			"Reroute attempt %d/%d failed for topic=%s partition=%d offset=%d err=%v; retrying in %s",
			attempt,
			defaultRerouteAttempts,
			topic,
			partition,
			offset,
			lastErr,
			delay,
		)

		timer := time.NewTimer(delay)
		if cr == nil || cr.ctx == nil {
			<-timer.C
			continue
		}

		select {
		case <-timer.C:
		case <-cr.ctx.Done():
			timer.Stop()
			return fmt.Errorf("consumer shutting down while rerouting failed message: %w", cr.ctx.Err())
		}
	}

	return fmt.Errorf("failed to reroute message after %d attempts: %w", defaultRerouteAttempts, lastErr)
}

func resolveFailedRecordTargetTopic(topic string, nextAttempt, maxRetryAttempts int) string {
	baseTopic := strings.TrimSuffix(topic, retryTopicSuffix)

	if strings.HasSuffix(topic, retryTopicSuffix) {
		if nextAttempt > maxRetryAttempts {
			return baseTopic + dltTopicSuffix
		}

		return topic
	}

	return baseTopic + retryTopicSuffix
}

func parseRetryAttempt(headers []kgo.RecordHeader) int {
	raw := resolveHeader(headers, retryAttemptHeader)
	if raw == "" {
		return 0
	}

	parsed, err := strconv.Atoi(raw)
	if err != nil || parsed < 0 {
		return 0
	}

	return parsed
}

func cloneHeaders(headers []kgo.RecordHeader) []kgo.RecordHeader {
	cloned := make([]kgo.RecordHeader, len(headers))
	copy(cloned, headers)

	return cloned
}

func isNonRetryablePayloadError(err error) bool {
	if err == nil {
		return false
	}

	msg := err.Error()

	return strings.Contains(msg, "invalid queue payload:") ||
		strings.Contains(msg, "invalid transaction payload:")
}

func upsertHeader(headers []kgo.RecordHeader, key string, value []byte) []kgo.RecordHeader {
	for i := range headers {
		if headers[i].Key == key {
			headers[i].Value = value
			return headers
		}
	}

	return append(headers, kgo.RecordHeader{Key: key, Value: value})
}
