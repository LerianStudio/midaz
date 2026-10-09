// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package rabbitmq

import (
	"context"
	"fmt"
	"time"

	libConstants "github.com/LerianStudio/lib-commons/v7/commons/constants"
	tmcore "github.com/LerianStudio/lib-commons/v7/commons/tenant-manager/core"
	libObservability "github.com/LerianStudio/lib-observability/v4"
	libLog "github.com/LerianStudio/lib-observability/v4/log"
	libOpentelemetry "github.com/LerianStudio/lib-observability/v4/tracing"
	amqp "github.com/rabbitmq/amqp091-go"
)

// headerTenantID is the AMQP header key used to propagate the tenant ID
// for audit trail and consumer context in multi-tenant messaging.
const headerTenantID = "X-Tenant-ID"

//go:generate go run go.uber.org/mock/mockgen@v0.6.0 -source=producer.multitenant.go -destination=producer.multitenant_mock.go -package=rabbitmq

// PublishableChannel abstracts the amqp.Channel operations used during message
// publishing. *amqp.Channel satisfies this interface, enabling unit-test mocking
// without a real RabbitMQ broker.
type PublishableChannel interface {
	PublishWithContext(ctx context.Context, exchange, key string, mandatory, immediate bool, msg amqp.Publishing) error
	Confirm(noWait bool) error
	NotifyPublish(confirm chan amqp.Confirmation) chan amqp.Confirmation
	Close() error
}

// ChannelProvider abstracts tenant-aware RabbitMQ channel and connection management.
// *tmrabbitmq.Manager satisfies this interface.
type ChannelProvider interface {
	GetChannel(ctx context.Context, tenantID string) (PublishableChannel, error)
	Close(ctx context.Context) error
}

// managerAdapter wraps *tmrabbitmq.Manager to satisfy ChannelProvider.
// This adapter converts the concrete *amqp.Channel returned by Manager.GetChannel
// into the PublishableChannel interface that publish() expects.
type managerAdapter struct {
	manager managerGetter
}

// managerGetter is the subset of *tmrabbitmq.Manager used by managerAdapter.
// Separated for testability of the adapter itself.
type managerGetter interface {
	GetChannel(ctx context.Context, tenantID string) (*amqp.Channel, error)
	Close(ctx context.Context) error
}

func (a *managerAdapter) GetChannel(ctx context.Context, tenantID string) (PublishableChannel, error) {
	return a.manager.GetChannel(ctx, tenantID)
}

func (a *managerAdapter) Close(ctx context.Context) error {
	return a.manager.Close(ctx)
}

// Compile-time interface check.
var _ ProducerRepository = (*MultiTenantProducerRepository)(nil)

// MultiTenantProducerRepository publishes messages to tenant-specific RabbitMQ
// vhosts using the tenant-manager RabbitMQ Manager for connection lifecycle.
type MultiTenantProducerRepository struct {
	channelProvider ChannelProvider
	logger          libLog.Logger
	confirmTimeout  time.Duration
}

// NewMultiTenantProducer creates a new MultiTenantProducerRepository.
// Accepts *tmrabbitmq.Manager (wrapped internally to satisfy ChannelProvider).
func NewMultiTenantProducer(manager managerGetter, logger libLog.Logger, opts ...ProducerOption) *MultiTenantProducerRepository {
	return NewMultiTenantProducerWithProvider(&managerAdapter{manager: manager}, logger, opts...)
}

// NewMultiTenantProducerWithProvider creates a new MultiTenantProducerRepository
// using an explicit ChannelProvider. Useful for testing with mock providers.
func NewMultiTenantProducerWithProvider(provider ChannelProvider, logger libLog.Logger, opts ...ProducerOption) *MultiTenantProducerRepository {
	return &MultiTenantProducerRepository{
		channelProvider: provider,
		logger:          logger,
		confirmTimeout:  newProducerSettings(opts).confirmTimeout,
	}
}

// ProducerDefault sends a message to the tenant-specific RabbitMQ vhost.
// The tenant ID is extracted from the context; an error is returned if absent.
// Each call owns a fresh channel, so its single confirmation is the message's.
func (p *MultiTenantProducerRepository) ProducerDefault(ctx context.Context, exchange, key string, message []byte) (*string, error) {
	_, tracer, reqID, _ := libObservability.NewTrackingFromContext(ctx)

	// Rebind ctx: the publish span's trace context is injected into the message
	// headers below so the consumer can continue the trace.
	ctx, span := tracer.Start(ctx, "rabbitmq.multi_tenant_producer.publish_message")
	defer span.End()

	tenantID := tmcore.GetTenantIDContext(ctx)
	if tenantID == "" {
		err := fmt.Errorf("tenant ID is required in context for multi-tenant producer")
		libOpentelemetry.HandleSpanError(span, "Missing tenant ID in context", err)

		return nil, err
	}

	ch, err := p.channelProvider.GetChannel(ctx, tenantID)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to get channel", err)

		return nil, fmt.Errorf("failed to get channel for tenant %s: %w", tenantID, err)
	}

	if ch == nil {
		err := fmt.Errorf("channel provider returned nil channel for tenant %s", tenantID)
		libOpentelemetry.HandleSpanError(span, "Nil channel returned", err)

		return nil, err
	}

	defer ch.Close()

	publishCtx, cancel := context.WithTimeout(ctx, p.confirmTimeout)
	defer cancel()

	if err := ch.Confirm(false); err != nil {
		err = fmt.Errorf("enable publisher confirms: %w", err)
		libOpentelemetry.HandleSpanError(span, "Failed to enable publisher confirms", err)

		return nil, err
	}

	confirmations := ch.NotifyPublish(make(chan amqp.Confirmation, 1))

	headers := amqp.Table{
		libConstants.HeaderID: reqID,
		headerTenantID:        tenantID,
	}

	libOpentelemetry.InjectTraceHeadersIntoQueue(ctx, (*map[string]any)(&headers))

	err = ch.PublishWithContext(publishCtx, exchange, key, false, false, amqp.Publishing{
		ContentType:  "application/json",
		DeliveryMode: amqp.Persistent,
		Headers:      headers,
		Body:         message,
	})
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to publish message", err)

		return nil, err
	}

	if err := awaitSingleConfirmation(publishCtx, confirmations); err != nil {
		libOpentelemetry.HandleSpanError(span, "Publish was not confirmed by the broker", err)

		return nil, err
	}

	return nil, nil
}

// CheckRabbitMQHealth returns true. The tenant-manager Manager handles its own
// connection lifecycle with LRU eviction; no external health check is needed.
func (p *MultiTenantProducerRepository) CheckRabbitMQHealth() bool {
	return true
}

// Close releases all RabbitMQ connections managed by the ChannelProvider.
func (p *MultiTenantProducerRepository) Close() error {
	if p == nil || p.channelProvider == nil {
		return nil
	}

	return p.channelProvider.Close(context.Background())
}
