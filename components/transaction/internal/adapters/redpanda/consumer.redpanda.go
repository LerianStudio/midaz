// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package redpanda

import (
	"context"
	"fmt"
	"math"
	"strconv"
	"strings"
	"sync"
	"time"

	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	libConstants "github.com/LerianStudio/lib-commons/v2/commons/constants"
	libLog "github.com/LerianStudio/lib-commons/v2/commons/log"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v2/commons/opentelemetry"
	libZap "github.com/LerianStudio/lib-commons/v2/commons/zap"
	"github.com/twmb/franz-go/pkg/kgo"
	attribute "go.opentelemetry.io/otel/attribute"
)

const (
	defaultConsumerGroup    = "midaz-balance-projector"
	defaultConsumerWorkers  = 5
	defaultWorkQueueSize    = 1024
	defaultMaxRetryAttempts = 3
	defaultRerouteAttempts  = 3
	retryTopicSuffix        = ".retry"
	dltTopicSuffix          = ".dlt"
	retryAttemptHeader      = "x-midaz-retry-attempt"
)

// ConsumerRepository provides an interface for broker consumers.
type ConsumerRepository interface {
	Register(topicName string, handler QueueHandlerFunc)
	RunConsumers() error
}

// QueueHandlerFunc processes a specific topic payload.
type QueueHandlerFunc func(ctx context.Context, body []byte) error

type queuedRecord struct {
	handler QueueHandlerFunc
	record  *kgo.Record
}

// ConsumerRoutes runs topic handlers backed by a Redpanda consumer group.
type ConsumerRoutes struct {
	brokers         []string
	consumerGroup   string
	routes          map[string]QueueHandlerFunc
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
		logger = libZap.InitializeLogger()
	}

	telemetryValue := libOpentelemetry.Telemetry{}
	if telemetry != nil {
		telemetryValue = *telemetry
	}

	return &ConsumerRoutes{
		brokers:          brokers,
		consumerGroup:    consumerGroup,
		routes:           make(map[string]QueueHandlerFunc),
		NumbersOfWorker:  numbersOfWorkers,
		FetchMaxBytes:    fetchMaxBytes,
		Logger:           logger,
		Telemetry:        telemetryValue,
		ctx:              runCtx,
		cancel:           cancel,
		securityConfig:   securityConfig,
		maxRetryAttempts: maxRetryAttempts,
	}
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

// RunConsumers starts the consumer workers.
func (cr *ConsumerRoutes) RunConsumers() error {
	if cr == nil {
		return fmt.Errorf("consumer routes are nil")
	}

	if len(cr.routes) == 0 {
		cr.Warn("No Redpanda routes registered; skipping consumer startup")
		return nil
	}

	topicsSet := make(map[string]struct{}, len(cr.routes)*2)
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

	workCh := make(chan queuedRecord, defaultWorkQueueSize)
	for i := 0; i < cr.NumbersOfWorker; i++ {
		go cr.startWorker(i, workCh)
	}

	go cr.pollLoop(workCh)

	return nil
}

func (cr *ConsumerRoutes) pollLoop(workCh chan<- queuedRecord) {
	defer close(workCh)

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

			select {
			case workCh <- queuedRecord{handler: handler, record: record}:
			case <-cr.ctx.Done():
				return
			}
		}
	}
}

func (cr *ConsumerRoutes) startWorker(workerID int, workCh <-chan queuedRecord) {
	for job := range workCh {
		midazID := resolveHeader(job.record.Headers, libConstants.HeaderID)
		if midazID == "" {
			midazID = libCommons.GenerateUUIDv7().String()
		}

		log := cr.Logger.WithFields(
			libConstants.HeaderID, midazID,
		).WithDefaultMessageTemplate(midazID + libConstants.LoggerDefaultSeparator)

		ctx := libCommons.ContextWithLogger(
			libCommons.ContextWithHeaderID(context.Background(), midazID),
			log,
		)

		headerMap := make(map[string]any, len(job.record.Headers))
		for _, header := range job.record.Headers {
			headerMap[header.Key] = string(header.Value)
		}

		ctx = libOpentelemetry.ExtractTraceContextFromQueueHeaders(ctx, headerMap)

		logger, tracer, reqID, _ := libCommons.NewTrackingFromContext(ctx)
		ctx, spanConsumer := tracer.Start(ctx, "redpanda.consumer.process_message")
		ctx = libCommons.ContextWithSpanAttributes(ctx, attribute.String("app.request.request_id", reqID))
		spanConsumer.SetAttributes(
			attribute.String("app.request.redpanda.topic", job.record.Topic),
			attribute.Int64("app.request.redpanda.partition", int64(job.record.Partition)),
			attribute.Int64("app.request.redpanda.offset", job.record.Offset),
			attribute.Int("app.request.redpanda.payload_size_bytes", len(job.record.Value)),
			attribute.Int("app.request.redpanda.headers_count", len(job.record.Headers)),
		)

		err := job.handler(ctx, job.record.Value)
		if err != nil {
			libOpentelemetry.HandleSpanBusinessErrorEvent(&spanConsumer, "Error processing message", err)
			logger.Errorf("Worker %d: Error processing topic=%s err=%v", workerID, job.record.Topic, err)

			if routeErr := cr.routeFailedRecordWithRetry(ctx, job.record, err, logger); routeErr != nil {
				libOpentelemetry.HandleSpanError(&spanConsumer, "Failed to route message to retry/DLT", routeErr)
				spanConsumer.End()
				logger.Errorf("Worker %d: Failed to reroute message topic=%s partition=%d offset=%d err=%v", workerID, job.record.Topic, job.record.Partition, job.record.Offset, routeErr)
				cr.Stop()
				return
			}

			spanConsumer.End()
			cr.commitRecord(workerID, logger, job.record)

			continue
		}

		spanConsumer.End()
		cr.commitRecord(workerID, logger, job.record)
	}

	cr.Warnf("Worker %d stopped (consumer loop closed)", workerID)
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

func (cr *ConsumerRoutes) commitRecord(workerID int, logger libLog.Logger, record *kgo.Record) {
	commitCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if commitErr := cr.client.CommitRecords(commitCtx, record); commitErr != nil {
		logger.Errorf("Worker %d: Failed to commit topic=%s partition=%d offset=%d err=%v", workerID, record.Topic, record.Partition, record.Offset, commitErr)
	}
}

func (cr *ConsumerRoutes) routeFailedRecord(ctx context.Context, record *kgo.Record, handlerErr error, logger libLog.Logger) error {
	if record == nil {
		return fmt.Errorf("cannot route failed message: record is nil")
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
		Timestamp: time.Now(),
	}

	publishCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
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
		if err := cr.routeFailedRecord(ctx, record, handlerErr, logger); err == nil {
			return nil
		} else {
			lastErr = err
		}

		if attempt == defaultRerouteAttempts {
			break
		}

		delay := time.Duration(attempt) * 200 * time.Millisecond
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

func upsertHeader(headers []kgo.RecordHeader, key string, value []byte) []kgo.RecordHeader {
	for i := range headers {
		if headers[i].Key == key {
			headers[i].Value = value
			return headers
		}
	}

	return append(headers, kgo.RecordHeader{Key: key, Value: value})
}
