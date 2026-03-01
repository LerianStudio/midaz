// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package redpanda

import (
	"context"
	"errors"
	"fmt"
	"hash/fnv"
	"strconv"
	"strings"
	"time"

	"github.com/twmb/franz-go/pkg/kgo"

	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	libConstants "github.com/LerianStudio/lib-commons/v2/commons/constants"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v2/commons/opentelemetry"
)

const (
	defaultProducerLinger        = 5 * time.Millisecond
	defaultMaxBufferedRecords    = 10_000
	defaultRecordDeliveryTimeout = 30 * time.Second
	// healthCheckTimeout is the timeout for the async health-check ping.
	healthCheckTimeout = 2 * time.Second
)

var (
	// ErrNoBrokersProvided is returned when no broker addresses are supplied.
	ErrNoBrokersProvided = errors.New("at least one redpanda broker is required")
	// ErrProducerNil is returned when the producer client is nil.
	ErrProducerNil = errors.New("redpanda producer is nil")
	// ErrTopicEmpty is returned when the topic string is empty.
	ErrTopicEmpty = errors.New("redpanda topic cannot be empty")
)

// ProducerRepository provides an abstraction for broker producer operations.
type ProducerRepository interface {
	ProducerDefault(ctx context.Context, topic, key string, message []byte) (*string, error)
	ProducerDefaultWithContext(ctx context.Context, topic, key string, message []byte) (*string, error)
	CheckHealth() bool
	Close() error
}

// ProducerRedpandaRepository is a Redpanda implementation of ProducerRepository.
type ProducerRedpandaRepository struct {
	client           *kgo.Client
	transactionAsync bool
}

// NewProducerRedpanda creates a producer backed by franz-go.
func NewProducerRedpanda(brokers []string, linger time.Duration, maxBufferedRecords int, transactionAsync bool) (*ProducerRedpandaRepository, error) {
	return NewProducerRedpandaWithSecurity(brokers, linger, maxBufferedRecords, transactionAsync, ClientSecurityConfig{})
}

// NewProducerRedpandaWithSecurity creates a producer backed by franz-go with optional TLS/SASL.
func NewProducerRedpandaWithSecurity(
	brokers []string,
	linger time.Duration,
	maxBufferedRecords int,
	transactionAsync bool,
	securityConfig ClientSecurityConfig,
) (*ProducerRedpandaRepository, error) {
	return NewProducerRedpandaWithSecurityAndShardPartitioning(
		brokers,
		linger,
		maxBufferedRecords,
		transactionAsync,
		securityConfig,
		0,
	)
}

// NewProducerRedpandaWithSecurityAndShardPartitioning creates a producer backed
// by franz-go with optional TLS/SASL and optional shard-aware partitioning.
func NewProducerRedpandaWithSecurityAndShardPartitioning(
	brokers []string,
	linger time.Duration,
	maxBufferedRecords int,
	transactionAsync bool,
	securityConfig ClientSecurityConfig,
	shardCount int,
) (*ProducerRedpandaRepository, error) {
	if len(brokers) == 0 {
		return nil, ErrNoBrokersProvided
	}

	if linger <= 0 {
		linger = defaultProducerLinger
	}

	if maxBufferedRecords <= 0 {
		maxBufferedRecords = defaultMaxBufferedRecords
	}

	options := []kgo.Opt{
		kgo.SeedBrokers(brokers...),
		kgo.RequiredAcks(kgo.AllISRAcks()),
		kgo.ProducerLinger(linger),
		kgo.MaxBufferedRecords(maxBufferedRecords),
		kgo.ProducerBatchCompression(kgo.Lz4Compression()),
		kgo.RecordDeliveryTimeout(defaultRecordDeliveryTimeout),
	}

	if shardCount > 0 {
		options = append(options, kgo.RecordPartitioner(&ShardPartitioner{shardCount: shardCount}))
	}

	securityOptions, err := BuildSecurityOptions(securityConfig)
	if err != nil {
		return nil, fmt.Errorf("invalid redpanda security configuration: %w", err)
	}

	options = append(options, securityOptions...)

	client, err := kgo.NewClient(options...)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize redpanda producer: %w", err)
	}

	return &ProducerRedpandaRepository{
		client:           client,
		transactionAsync: transactionAsync,
	}, nil
}

// ShardPartitioner routes records to specific partitions when the record key is
// a numeric shard ID ("0", "1", ...). Non-numeric keys fall back to hash-based
// partitioning.
type ShardPartitioner struct {
	shardCount int
}

// ForTopic returns a TopicPartitioner for the given topic backed by shard-aware logic.
func (p *ShardPartitioner) ForTopic(topic string) kgo.TopicPartitioner {
	return &shardTopicPartitioner{topic: topic, shardCount: p.shardCount}
}

type shardTopicPartitioner struct {
	topic      string
	shardCount int
}

// RequiresConsistency always returns true to ensure record ordering per shard.
func (tp *shardTopicPartitioner) RequiresConsistency(_ *kgo.Record) bool { return true }

// Partition returns the target partition for the given record.
func (tp *shardTopicPartitioner) Partition(record *kgo.Record, partitions int) int {
	if partitions <= 0 {
		return 0
	}

	if record == nil || record.Key == nil {
		return 0
	}

	shardID, err := strconv.Atoi(string(record.Key))
	if err == nil && shardID >= 0 && (tp.shardCount <= 0 || shardID < tp.shardCount) {
		return shardID % partitions
	}

	return hashPartition(record.Key, partitions)
}

func hashPartition(key []byte, partitions int) int {
	if partitions <= 0 {
		return 0
	}

	h := fnv.New32a()
	_, _ = h.Write(key)

	return int(uint64(h.Sum32()) % uint64(partitions)) //nolint:gosec
}

// CheckHealth checks broker connectivity.
func (p *ProducerRedpandaRepository) CheckHealth() bool {
	if p == nil || p.client == nil {
		return false
	}

	if !p.transactionAsync {
		return true
	}

	ctx, cancel := context.WithTimeout(context.Background(), healthCheckTimeout)
	defer cancel()

	return p.client.Ping(ctx) == nil
}

// ProducerDefault sends a message asynchronously and waits for callback completion.
func (p *ProducerRedpandaRepository) ProducerDefault(ctx context.Context, topic, key string, message []byte) (*string, error) {
	return p.produceSync(ctx, topic, key, message, "redpanda.producer.publish_message")
}

// ProducerDefaultWithContext sends a message synchronously respecting caller context.
func (p *ProducerRedpandaRepository) ProducerDefaultWithContext(ctx context.Context, topic, key string, message []byte) (*string, error) {
	return p.produceSync(ctx, topic, key, message, "redpanda.producer.publish_message_with_context")
}

func (p *ProducerRedpandaRepository) produceSync(ctx context.Context, topic, key string, message []byte, spanName string) (*string, error) {
	if p == nil || p.client == nil {
		return nil, ErrProducerNil
	}

	topic = strings.TrimSpace(topic)
	if topic == "" {
		return nil, ErrTopicEmpty
	}

	logger, tracer, reqID, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, spanProducer := tracer.Start(ctx, spanName)
	defer spanProducer.End()

	headers := map[string]any{}
	if reqID != "" {
		headers[libConstants.HeaderID] = reqID
	}

	libOpentelemetry.InjectTraceHeadersIntoQueue(ctx, &headers)

	record := &kgo.Record{
		Topic:   topic,
		Key:     []byte(key),
		Value:   message,
		Headers: buildRecordHeaders(headers),
	}

	if err := p.client.ProduceSync(ctx, record).FirstErr(); err != nil {
		logger.Errorf("Failed to publish message topic=%s key=%s err=%v", topic, key, err)
		libOpentelemetry.HandleSpanError(&spanProducer, "Failed to publish message", err)

		return nil, fmt.Errorf("produce sync failed: %w", err)
	}

	return nil, nil
}

// Close releases producer resources.
func (p *ProducerRedpandaRepository) Close() error {
	if p == nil || p.client == nil {
		return nil
	}

	p.client.Close()

	return nil
}

func buildRecordHeaders(input map[string]any) []kgo.RecordHeader {
	headers := make([]kgo.RecordHeader, 0, len(input))

	for key, value := range input {
		headers = append(headers, kgo.RecordHeader{Key: key, Value: toHeaderBytes(value)})
	}

	return headers
}

func toHeaderBytes(value any) []byte {
	switch typed := value.(type) {
	case []byte:
		return typed
	case string:
		return []byte(typed)
	case fmt.Stringer:
		return []byte(typed.String())
	default:
		return []byte(fmt.Sprint(typed))
	}
}
