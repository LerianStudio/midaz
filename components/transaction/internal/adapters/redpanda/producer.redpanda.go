// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package redpanda

import (
	"context"
	"fmt"
	"strings"
	"time"

	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	libConstants "github.com/LerianStudio/lib-commons/v2/commons/constants"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v2/commons/opentelemetry"
	"github.com/twmb/franz-go/pkg/kgo"
)

const (
	defaultProducerLinger        = 5 * time.Millisecond
	defaultMaxBufferedRecords    = 10_000
	defaultRecordDeliveryTimeout = 30 * time.Second
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
	if len(brokers) == 0 {
		return nil, fmt.Errorf("at least one redpanda broker is required")
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

// CheckHealth checks broker connectivity.
func (p *ProducerRedpandaRepository) CheckHealth() bool {
	if p == nil || p.client == nil {
		return false
	}

	if !p.transactionAsync {
		return true
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
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
		return nil, fmt.Errorf("redpanda producer is nil")
	}

	topic = strings.TrimSpace(topic)
	if topic == "" {
		return nil, fmt.Errorf("redpanda topic cannot be empty")
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
		return nil, err
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
