// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package rabbitmq

import (
	"context"
	"fmt"

	libCommons "github.com/LerianStudio/lib-commons/v3/commons"
	libConstants "github.com/LerianStudio/lib-commons/v3/commons/constants"
	libLog "github.com/LerianStudio/lib-commons/v3/commons/log"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v3/commons/opentelemetry"
	tmcore "github.com/LerianStudio/lib-commons/v3/commons/tenant-manager/core"
	amqp "github.com/rabbitmq/amqp091-go"
)

//go:generate mockgen -source=./components/transaction/internal/adapters/rabbitmq/producer.multitenant.go -destination=./components/transaction/internal/adapters/rabbitmq/producer.multitenant_mock.go -package=rabbitmq

// PublishableChannel abstracts the amqp.Channel operations used during message
// publishing. *amqp.Channel satisfies this interface, enabling unit-test mocking
// without a real RabbitMQ broker.
type PublishableChannel interface {
	PublishWithContext(ctx context.Context, exchange, key string, mandatory, immediate bool, msg amqp.Publishing) error
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
}

// NewMultiTenantProducer creates a new MultiTenantProducerRepository.
// Accepts *tmrabbitmq.Manager (wrapped internally to satisfy ChannelProvider).
func NewMultiTenantProducer(manager managerGetter, logger libLog.Logger) *MultiTenantProducerRepository {
	return &MultiTenantProducerRepository{
		channelProvider: &managerAdapter{manager: manager},
		logger:          logger,
	}
}

// NewMultiTenantProducerWithProvider creates a new MultiTenantProducerRepository
// using an explicit ChannelProvider. Useful for testing with mock providers.
func NewMultiTenantProducerWithProvider(provider ChannelProvider, logger libLog.Logger) *MultiTenantProducerRepository {
	return &MultiTenantProducerRepository{
		channelProvider: provider,
		logger:          logger,
	}
}

// ProducerDefault sends a message to the tenant-specific RabbitMQ vhost.
// The tenant ID is extracted from the context; an error is returned if absent.
func (p *MultiTenantProducerRepository) ProducerDefault(ctx context.Context, exchange, key string, message []byte) (*string, error) {
	return p.publish(ctx, exchange, key, message, "rabbitmq.multi_tenant_producer.publish_message")
}

// ProducerDefaultWithContext sends a message with explicit context timeout control.
// Behaves identically to ProducerDefault since the Manager handles connection lifecycle.
func (p *MultiTenantProducerRepository) ProducerDefaultWithContext(ctx context.Context, exchange, key string, message []byte) (*string, error) {
	return p.publish(ctx, exchange, key, message, "rabbitmq.multi_tenant_producer.publish_message_with_context")
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

// publish is the shared implementation for ProducerDefault and ProducerDefaultWithContext.
func (p *MultiTenantProducerRepository) publish(ctx context.Context, exchange, key string, message []byte, spanName string) (*string, error) {
	logger, tracer, reqID, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, spanName)
	defer span.End()

	tenantID := tmcore.GetTenantIDFromContext(ctx)
	if tenantID == "" {
		err := fmt.Errorf("tenant ID is required in context for multi-tenant producer")
		libOpentelemetry.HandleSpanError(&span, "Missing tenant ID in context", err)

		return nil, err
	}

	logger.Infof("Publishing message to exchange: %s, key: %s, tenant: %s", exchange, key, tenantID)

	ch, err := p.channelProvider.GetChannel(ctx, tenantID)
	if err != nil {
		logger.Errorf("Failed to get channel for tenant %s: %v", tenantID, err)
		libOpentelemetry.HandleSpanError(&span, "Failed to get channel", err)

		return nil, fmt.Errorf("failed to get channel for tenant %s: %w", tenantID, err)
	}

	if ch == nil {
		err := fmt.Errorf("channel provider returned nil channel for tenant %s", tenantID)
		libOpentelemetry.HandleSpanError(&span, "Nil channel returned", err)

		return nil, err
	}

	defer ch.Close()

	headers := amqp.Table{
		libConstants.HeaderID: reqID,
	}

	libOpentelemetry.InjectTraceHeadersIntoQueue(ctx, (*map[string]any)(&headers))

	err = ch.PublishWithContext(ctx, exchange, key, false, false, amqp.Publishing{
		ContentType:  "application/json",
		DeliveryMode: amqp.Persistent,
		Headers:      headers,
		Body:         message,
	})
	if err != nil {
		logger.Errorf("Failed to publish message to exchange: %s, key: %s, tenant: %s: %v", exchange, key, tenantID, err)
		libOpentelemetry.HandleSpanError(&span, "Failed to publish message", err)

		return nil, err
	}

	logger.Infof("Message sent successfully to exchange: %s, key: %s, tenant: %s", exchange, key, tenantID)

	return nil, nil
}
