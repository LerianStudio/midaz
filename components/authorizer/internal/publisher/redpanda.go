// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package publisher

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/twmb/franz-go/pkg/kgo"

	libLog "github.com/LerianStudio/lib-commons/v2/commons/log"

	"github.com/LerianStudio/midaz/v3/pkg/constant"
)

const (
	defaultProducerLinger        = 5 * time.Millisecond
	defaultMaxBufferedRecords    = 10_000
	defaultRecordDeliveryTimeout = 30 * time.Second
	defaultPublishTimeout        = 5 * time.Second

	// BackpressurePolicyBoundedWait applies a bounded timeout to publish operations.
	BackpressurePolicyBoundedWait = "bounded_wait"

	// BackpressurePolicyFailFast fails immediately when the publish deadline is exceeded.
	BackpressurePolicyFailFast = "fail_fast"
)

// Config holds tuning parameters for the Redpanda publisher.
type Config struct {
	ProducerLinger        time.Duration
	MaxBufferedRecords    int
	RecordRetries         int
	RecordDeliveryTimeout time.Duration
	PublishTimeout        time.Duration
	BackpressurePolicy    string
}

func normalizeConfig(cfg Config) Config {
	if cfg.ProducerLinger <= 0 {
		cfg.ProducerLinger = defaultProducerLinger
	}

	if cfg.MaxBufferedRecords <= 0 {
		cfg.MaxBufferedRecords = defaultMaxBufferedRecords
	}

	if cfg.RecordDeliveryTimeout <= 0 {
		cfg.RecordDeliveryTimeout = defaultRecordDeliveryTimeout
	}

	if cfg.PublishTimeout <= 0 {
		cfg.PublishTimeout = defaultPublishTimeout
	}

	policy := strings.ToLower(strings.TrimSpace(cfg.BackpressurePolicy))
	if policy != BackpressurePolicyFailFast && policy != BackpressurePolicyBoundedWait {
		policy = BackpressurePolicyBoundedWait
	}

	cfg.BackpressurePolicy = policy

	if cfg.RecordRetries < 0 {
		cfg.RecordRetries = 0
	}

	return cfg
}

// RedpandaPublisher publishes authorizer-approved operations to Redpanda.
type RedpandaPublisher struct {
	client *kgo.Client
	logger libLog.Logger
	config Config
}

// NewRedpandaPublisher creates a franz-go publisher.
func NewRedpandaPublisher(brokers []string, logger libLog.Logger) (*RedpandaPublisher, error) {
	return NewRedpandaPublisherWithSecurity(brokers, logger, Config{}, SecurityConfig{})
}

// NewRedpandaPublisherWithSecurity creates a franz-go publisher with optional TLS/SASL.
func NewRedpandaPublisherWithSecurity(
	brokers []string,
	logger libLog.Logger,
	config Config,
	securityConfig SecurityConfig,
) (*RedpandaPublisher, error) {
	if len(brokers) == 0 {
		return nil, constant.ErrRedpandaBrokersEmpty
	}

	config = normalizeConfig(config)

	securityOptions, err := buildSecurityOptions(securityConfig)
	if err != nil {
		return nil, fmt.Errorf("invalid redpanda security configuration: %w", err)
	}

	options := make([]kgo.Opt, 0, len(securityOptions)+6) //nolint:mnd // 6 base options below
	options = append(options,
		kgo.SeedBrokers(brokers...),
		kgo.RequiredAcks(kgo.AllISRAcks()),
		kgo.ProducerLinger(config.ProducerLinger),
		kgo.MaxBufferedRecords(config.MaxBufferedRecords),
		kgo.RecordRetries(config.RecordRetries),
		kgo.RecordDeliveryTimeout(config.RecordDeliveryTimeout),
	)

	options = append(options, securityOptions...)

	client, err := kgo.NewClient(options...)
	if err != nil {
		return nil, fmt.Errorf("initialize redpanda publisher: %w", err)
	}

	return &RedpandaPublisher{client: client, logger: logger, config: config}, nil
}

// Publish writes a record synchronously and only returns nil after broker ack.
func (p *RedpandaPublisher) Publish(ctx context.Context, message Message) error {
	if p == nil || p.client == nil {
		return constant.ErrRedpandaPublisherNil
	}

	if len(message.Payload) == 0 {
		return constant.ErrMessagePayloadEmpty
	}

	topic := strings.TrimSpace(message.Topic)
	if topic == "" {
		return constant.ErrMessageTopicEmpty
	}

	record := &kgo.Record{
		Topic: topic,
		Key:   []byte(message.PartitionKey),
		Value: message.Payload,
	}

	for key, value := range message.Headers {
		record.Headers = append(record.Headers, kgo.RecordHeader{Key: key, Value: []byte(value)})
	}

	contentType := strings.TrimSpace(message.ContentType)
	if contentType != "" {
		record.Headers = append(record.Headers, kgo.RecordHeader{Key: "content-type", Value: []byte(contentType)})
	}

	publishCtx, cancel := p.newPublishContext(ctx)
	defer cancel()

	if err := p.client.ProduceSync(publishCtx, record).FirstErr(); err != nil {
		return p.handlePublishError(err, topic, message.PartitionKey)
	}

	return nil
}

// handlePublishError wraps and logs publish errors.
func (p *RedpandaPublisher) handlePublishError(err error, topic, partitionKey string) error {
	if errors.Is(err, context.DeadlineExceeded) {
		wrapped := fmt.Errorf("redpanda publish timeout (policy=%s timeout=%s): %w", p.config.BackpressurePolicy, p.config.PublishTimeout, err)
		if p.logger != nil {
			p.logger.Warnf("Authorizer publisher timeout topic=%s partition_key=%s policy=%s: %v", topic, partitionKey, p.config.BackpressurePolicy, wrapped)
		}

		return wrapped
	}

	if p.logger != nil {
		p.logger.Warnf("Authorizer publisher failed topic=%s partition_key=%s policy=%s: %v", topic, partitionKey, p.config.BackpressurePolicy, err)
	}

	return fmt.Errorf("redpanda produce sync: %w", err)
}

func (p *RedpandaPublisher) newPublishContext(ctx context.Context) (context.Context, context.CancelFunc) {
	if ctx == nil {
		ctx = context.Background()
	}

	if _, hasDeadline := ctx.Deadline(); hasDeadline {
		return ctx, func() {}
	}

	timeout := p.config.PublishTimeout
	if p.config.BackpressurePolicy == BackpressurePolicyFailFast && timeout > time.Second {
		timeout = time.Second
	}

	if timeout <= 0 {
		return ctx, func() {}
	}

	return context.WithTimeout(ctx, timeout)
}

// Close releases producer resources.
func (p *RedpandaPublisher) Close() error {
	if p == nil || p.client == nil {
		return nil
	}

	p.client.Close()

	return nil
}
