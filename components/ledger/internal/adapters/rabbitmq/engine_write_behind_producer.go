// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package rabbitmq

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	libConstants "github.com/LerianStudio/lib-commons/v7/commons/constants"
	libRabbitmq "github.com/LerianStudio/lib-commons/v7/commons/rabbitmq"
	tmcore "github.com/LerianStudio/lib-commons/v7/commons/tenant-manager/core"
	tmrabbitmq "github.com/LerianStudio/lib-commons/v7/commons/tenant-manager/rabbitmq"
	libObservability "github.com/LerianStudio/lib-observability/v4"
	libOpentelemetry "github.com/LerianStudio/lib-observability/v4/tracing"
	"github.com/google/uuid"
	amqp "github.com/rabbitmq/amqp091-go"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

const defaultEngineWriteBehindConfirmTimeout = 5 * time.Second

const (
	engineWriteBehindFailureContextCancelled      = "context_cancelled"
	engineWriteBehindFailureInvalidMessage        = "invalid_message"
	engineWriteBehindFailureTenantRequired        = "tenant_required"
	engineWriteBehindFailureTenantMismatch        = "tenant_mismatch"
	engineWriteBehindFailureTenantUnexpected      = "tenant_unexpected"
	engineWriteBehindFailureConfiguration         = "configuration"
	engineWriteBehindFailureChannelAcquire        = "channel_acquire"
	engineWriteBehindFailurePublish               = "publish"
	engineWriteBehindFailureNack                  = "nack"
	engineWriteBehindFailureUnroutable            = "unroutable"
	engineWriteBehindFailureConfirmationUncertain = "confirmation_uncertain"
)

var (
	ErrEngineWriteBehindPublishNacked      = errors.New("engine write-behind publish nacked")
	ErrEngineWriteBehindPublishUnroutable  = errors.New("engine write-behind publish unroutable")
	ErrEngineWriteBehindPublishUnconfirmed = errors.New("engine write-behind publish unconfirmed")
	ErrEngineWriteBehindInvalidMessage     = errors.New("invalid engine write-behind message")
	ErrEngineWriteBehindTenantRequired     = errors.New("engine write-behind tenant is required")
	ErrEngineWriteBehindTenantMismatch     = errors.New("engine write-behind tenant differs from authenticated context")
	ErrEngineWriteBehindTenantUnexpected   = errors.New("engine write-behind tenant is not allowed in single-tenant mode")
)

type engineWriteBehindTenantPolicy uint8

const (
	engineWriteBehindSingleTenant engineWriteBehindTenantPolicy = iota
	engineWriteBehindMultiTenant
)

// EngineWriteBehindMessage carries already encoded, immutable engine evidence.
// MessageID is derived from transaction/execution identity by the producer.
type EngineWriteBehindMessage struct {
	TenantID      string
	TransactionID uuid.UUID
	ExecutionID   uuid.UUID
	Body          []byte
}

func (message EngineWriteBehindMessage) MessageID() string {
	return "engine-write-behind:" + message.TransactionID.String() + ":" + message.ExecutionID.String()
}

type engineWriteBehindConfirmableChannel interface {
	PublishWithContext(context.Context, string, string, bool, bool, amqp.Publishing) error
	Confirm(bool) error
	NotifyPublish(chan amqp.Confirmation) chan amqp.Confirmation
	NotifyReturn(chan amqp.Return) chan amqp.Return
	NotifyClose(chan *amqp.Error) chan *amqp.Error
	Close() error
}

type engineWriteBehindChannelLease struct {
	channel           engineWriteBehindConfirmableChannel
	identity          string
	closeAfterPublish bool
	release           func(bool)
}

type engineWriteBehindChannelProvider interface {
	acquire(context.Context, string) (engineWriteBehindChannelLease, error)
}

// EngineWriteBehindProducer is isolated from legacy producers because an
// applied accounting result requires mandatory routing and an explicit broker
// confirmation before transport can be considered successful.
type EngineWriteBehindProducer struct {
	provider       engineWriteBehindChannelProvider
	exchange       string
	routingKey     string
	confirmTimeout time.Duration
	tenantPolicy   engineWriteBehindTenantPolicy

	mu              sync.Mutex
	channelIdentity string
	confirmations   <-chan amqp.Confirmation
	returns         <-chan amqp.Return
	closures        <-chan *amqp.Error
}

func newEngineWriteBehindProducer(provider engineWriteBehindChannelProvider, exchange, routingKey string, confirmTimeout time.Duration, tenantPolicy engineWriteBehindTenantPolicy) (*EngineWriteBehindProducer, error) {
	if provider == nil || exchange == "" || routingKey == "" {
		return nil, fmt.Errorf("engine write-behind RabbitMQ producer is not configured")
	}

	if confirmTimeout <= 0 {
		confirmTimeout = defaultEngineWriteBehindConfirmTimeout
	}

	return &EngineWriteBehindProducer{
		provider: provider, exchange: exchange, routingKey: routingKey,
		confirmTimeout: confirmTimeout, tenantPolicy: tenantPolicy,
	}, nil
}

func NewSingleTenantEngineWriteBehindProducer(connection *libRabbitmq.RabbitMQConnection, exchange, routingKey string, confirmTimeout time.Duration) (*EngineWriteBehindProducer, error) {
	if connection == nil {
		return nil, fmt.Errorf("engine write-behind RabbitMQ connection is nil")
	}

	return newEngineWriteBehindProducer(singleTenantEngineChannelProvider{connection: connection}, exchange, routingKey, confirmTimeout, engineWriteBehindSingleTenant)
}

func NewMultiTenantEngineWriteBehindProducer(provider ChannelProvider, exchange, routingKey string, confirmTimeout time.Duration) (*EngineWriteBehindProducer, error) {
	if provider == nil {
		return nil, fmt.Errorf("engine write-behind tenant channel provider is nil")
	}

	return newEngineWriteBehindProducer(&multiTenantEngineChannelProvider{provider: provider}, exchange, routingKey, confirmTimeout, engineWriteBehindMultiTenant)
}

func NewMultiTenantEngineWriteBehindProducerFromManager(manager *tmrabbitmq.Manager, exchange, routingKey string, confirmTimeout time.Duration) (*EngineWriteBehindProducer, error) {
	if manager == nil {
		return nil, fmt.Errorf("engine write-behind tenant RabbitMQ manager is nil")
	}

	return NewMultiTenantEngineWriteBehindProducer(&managerAdapter{manager: manager}, exchange, routingKey, confirmTimeout)
}

// Publish returns successfully only after an ACK and no mandatory return. A
// timeout, channel closure, NACK, or unroutable return is intentionally
// ambiguous to the caller and must enter synchronous fallback/recovery.
//
//nolint:gocognit,gocyclo // publish confirms, returns, channel closure, and timeout form one outcome-classification boundary
func (producer *EngineWriteBehindProducer) Publish(ctx context.Context, message EngineWriteBehindMessage) (err error) {
	_, tracer, requestID, _ := libObservability.NewTrackingFromContext(ctx)

	// The publish span intentionally becomes the sequential context for local
	// validation, broker trace injection, and confirmation handling.
	ctx, span := tracer.Start(ctx, "rabbitmq.engine_write_behind.publish")
	defer span.End()

	if err := ctx.Err(); err != nil {
		return recordEngineWriteBehindPublishFailure(span, engineWriteBehindFailureContextCancelled, false, "Engine write-behind publish context is unavailable", err)
	}

	if producer == nil || producer.provider == nil {
		err := fmt.Errorf("engine write-behind RabbitMQ producer is not configured")

		return recordEngineWriteBehindPublishFailure(span, engineWriteBehindFailureConfiguration, false, "Engine write-behind producer is not configured", err)
	}

	if err := producer.validateTenantAndMessage(ctx, message); err != nil {
		return recordEngineWriteBehindPublishFailure(span, engineWriteBehindValidationFailureCause(err), false, "Engine write-behind publish validation failed", err)
	}

	producer.mu.Lock()
	defer producer.mu.Unlock()

	publishCtx, cancel := context.WithTimeout(ctx, producer.confirmTimeout)
	defer cancel()

	lease, err := producer.provider.acquire(publishCtx, message.TenantID)
	if err != nil {
		return recordEngineWriteBehindPublishFailure(span, engineWriteBehindFailureChannelAcquire, true, "Failed to acquire engine write-behind channel", err)
	}

	if lease.channel == nil || lease.release == nil {
		err := fmt.Errorf("engine write-behind channel provider returned an invalid lease")

		return recordEngineWriteBehindPublishFailure(span, engineWriteBehindFailureChannelAcquire, true, "Engine write-behind channel lease is invalid", err)
	}

	success := false
	defer func() {
		lease.release(success)

		if lease.closeAfterPublish || !success {
			producer.resetChannel(lease.identity)
		}
	}()

	if producer.channelIdentity != lease.identity {
		if err := lease.channel.Confirm(false); err != nil {
			err = fmt.Errorf("enable engine write-behind publisher confirms: %w", err)

			return recordEngineWriteBehindPublishFailure(span, engineWriteBehindFailureConfirmationUncertain, true, "Failed to enable engine write-behind publisher confirms", err)
		}

		producer.channelIdentity = lease.identity
		producer.confirmations = lease.channel.NotifyPublish(make(chan amqp.Confirmation, 1))
		producer.returns = lease.channel.NotifyReturn(make(chan amqp.Return, 1))
		producer.closures = lease.channel.NotifyClose(make(chan *amqp.Error, 1))
	}

	headers := amqp.Table{libConstants.HeaderID: requestID, headerTenantID: message.TenantID}
	libOpentelemetry.InjectTraceHeadersIntoQueue(ctx, (*map[string]any)(&headers))

	publishing := amqp.Publishing{
		ContentType:   "application/json",
		DeliveryMode:  amqp.Persistent,
		MessageId:     message.MessageID(),
		CorrelationId: message.ExecutionID.String(),
		Headers:       headers,
		Body:          append([]byte(nil), message.Body...),
	}
	if err := lease.channel.PublishWithContext(publishCtx, producer.exchange, producer.routingKey, true, false, publishing); err != nil {
		err = fmt.Errorf("publish engine write-behind evidence: %w", err)

		return recordEngineWriteBehindPublishFailure(span, engineWriteBehindFailurePublish, true, "Failed to publish engine write-behind evidence", err)
	}

	for {
		select {
		case returned, ok := <-producer.returns:
			if !ok {
				err = fmt.Errorf("%w: mandatory return channel closed", ErrEngineWriteBehindPublishUnconfirmed)

				return recordEngineWriteBehindPublishFailure(span, engineWriteBehindFailureConfirmationUncertain, true, "Engine write-behind confirmation is uncertain", err)
			}

			err = fmt.Errorf("%w: code=%d text=%s", ErrEngineWriteBehindPublishUnroutable, returned.ReplyCode, returned.ReplyText)

			return recordEngineWriteBehindPublishFailure(span, engineWriteBehindFailureUnroutable, true, "Engine write-behind evidence was unroutable", err)
		case confirmation, ok := <-producer.confirmations:
			if !ok {
				err = fmt.Errorf("%w: confirmation channel closed", ErrEngineWriteBehindPublishUnconfirmed)

				return recordEngineWriteBehindPublishFailure(span, engineWriteBehindFailureConfirmationUncertain, true, "Engine write-behind confirmation is uncertain", err)
			}

			if !confirmation.Ack {
				return recordEngineWriteBehindPublishFailure(span, engineWriteBehindFailureNack, true, "Engine write-behind evidence was negatively acknowledged", ErrEngineWriteBehindPublishNacked)
			}

			select {
			case returned, ok := <-producer.returns:
				if !ok {
					err = fmt.Errorf("%w: mandatory return channel closed", ErrEngineWriteBehindPublishUnconfirmed)

					return recordEngineWriteBehindPublishFailure(span, engineWriteBehindFailureConfirmationUncertain, true, "Engine write-behind confirmation is uncertain", err)
				}

				err = fmt.Errorf("%w: code=%d text=%s", ErrEngineWriteBehindPublishUnroutable, returned.ReplyCode, returned.ReplyText)

				return recordEngineWriteBehindPublishFailure(span, engineWriteBehindFailureUnroutable, true, "Engine write-behind evidence was unroutable", err)
			default:
			}

			success = true

			span.SetAttributes(
				attribute.String("app.response.outcome", "confirmed"),
				attribute.Bool("app.response.broker_io_attempted", true),
			)

			return nil
		case closeErr, ok := <-producer.closures:
			if !ok || closeErr == nil {
				err = fmt.Errorf("%w: RabbitMQ channel closed", ErrEngineWriteBehindPublishUnconfirmed)

				return recordEngineWriteBehindPublishFailure(span, engineWriteBehindFailureConfirmationUncertain, true, "Engine write-behind confirmation is uncertain", err)
			}

			err = fmt.Errorf("%w: RabbitMQ channel closed: %s", ErrEngineWriteBehindPublishUnconfirmed, closeErr.Reason)

			return recordEngineWriteBehindPublishFailure(span, engineWriteBehindFailureConfirmationUncertain, true, "Engine write-behind confirmation is uncertain", err)
		case <-publishCtx.Done():
			err = fmt.Errorf("%w: %w", ErrEngineWriteBehindPublishUnconfirmed, publishCtx.Err())

			return recordEngineWriteBehindPublishFailure(span, engineWriteBehindFailureConfirmationUncertain, true, "Engine write-behind confirmation is uncertain", err)
		}
	}
}

func engineWriteBehindValidationFailureCause(err error) string {
	switch {
	case errors.Is(err, ErrEngineWriteBehindInvalidMessage):
		return engineWriteBehindFailureInvalidMessage
	case errors.Is(err, ErrEngineWriteBehindTenantRequired):
		return engineWriteBehindFailureTenantRequired
	case errors.Is(err, ErrEngineWriteBehindTenantMismatch):
		return engineWriteBehindFailureTenantMismatch
	case errors.Is(err, ErrEngineWriteBehindTenantUnexpected):
		return engineWriteBehindFailureTenantUnexpected
	default:
		return engineWriteBehindFailureConfiguration
	}
}

func recordEngineWriteBehindPublishFailure(span trace.Span, cause string, brokerIOAttempted bool, message string, err error) error {
	span.SetAttributes(
		attribute.String("app.response.outcome", "failed"),
		attribute.String("app.response.failure_cause", cause),
		attribute.Bool("app.response.broker_io_attempted", brokerIOAttempted),
	)
	libOpentelemetry.HandleSpanError(span, message, err)

	return err
}

func (producer *EngineWriteBehindProducer) validateTenantAndMessage(ctx context.Context, message EngineWriteBehindMessage) error {
	if message.TransactionID == uuid.Nil || message.ExecutionID == uuid.Nil || len(message.Body) == 0 {
		return ErrEngineWriteBehindInvalidMessage
	}

	tenantID := tmcore.GetTenantIDContext(ctx)

	switch producer.tenantPolicy {
	case engineWriteBehindSingleTenant:
		if tenantID != "" || message.TenantID != "" {
			return ErrEngineWriteBehindTenantUnexpected
		}
	case engineWriteBehindMultiTenant:
		if tenantID == "" || message.TenantID == "" {
			return ErrEngineWriteBehindTenantRequired
		}

		if tenantID != message.TenantID {
			return ErrEngineWriteBehindTenantMismatch
		}
	default:
		return fmt.Errorf("engine write-behind tenant policy is not configured")
	}

	return nil
}

func (producer *EngineWriteBehindProducer) resetChannel(identity string) {
	if producer.channelIdentity != identity {
		return
	}

	producer.channelIdentity = ""
	producer.confirmations = nil
	producer.returns = nil
	producer.closures = nil
}

type singleTenantEngineChannelProvider struct {
	connection *libRabbitmq.RabbitMQConnection
}

func (provider singleTenantEngineChannelProvider) acquire(ctx context.Context, _ string) (engineWriteBehindChannelLease, error) {
	if err := provider.connection.EnsureChannelContext(ctx); err != nil {
		return engineWriteBehindChannelLease{}, err
	}

	channel := provider.connection.ChannelSnapshot()
	if channel == nil {
		return engineWriteBehindChannelLease{}, fmt.Errorf("RabbitMQ channel unavailable after ensure")
	}

	return engineWriteBehindChannelLease{
		channel:  channel,
		identity: fmt.Sprintf("%p", channel),
		release: func(success bool) {
			if !success {
				_ = channel.Close()
			}
		},
	}, nil
}

type multiTenantEngineChannelProvider struct {
	provider ChannelProvider
	mu       sync.Mutex
	nextID   uint64
}

func (provider *multiTenantEngineChannelProvider) acquire(ctx context.Context, tenantID string) (engineWriteBehindChannelLease, error) {
	channel, err := provider.provider.GetChannel(ctx, tenantID)
	if err != nil {
		return engineWriteBehindChannelLease{}, err
	}

	confirmable, ok := channel.(engineWriteBehindConfirmableChannel)
	if !ok {
		if channel != nil {
			_ = channel.Close()
		}

		return engineWriteBehindChannelLease{}, fmt.Errorf("tenant RabbitMQ channel does not support publisher confirms")
	}

	provider.mu.Lock()
	provider.nextID++
	identity := fmt.Sprintf("tenant:%s:%d", tenantID, provider.nextID)
	provider.mu.Unlock()

	return engineWriteBehindChannelLease{
		channel:           confirmable,
		identity:          identity,
		closeAfterPublish: true,
		release:           func(bool) { _ = confirmable.Close() },
	}, nil
}
