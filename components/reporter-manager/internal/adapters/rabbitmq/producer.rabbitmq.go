// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package rabbitmq

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/LerianStudio/midaz/v3/components/reporter/pkg"
	"github.com/LerianStudio/midaz/v3/components/reporter/pkg/constant"
	"github.com/LerianStudio/midaz/v3/components/reporter/pkg/ctxutil"
	"github.com/LerianStudio/midaz/v3/components/reporter/pkg/model"
	pkgRabbitmq "github.com/LerianStudio/midaz/v3/components/reporter/pkg/rabbitmq"

	libRabbitmq "github.com/LerianStudio/lib-commons/v5/commons/rabbitmq"
	tmcore "github.com/LerianStudio/lib-commons/v5/commons/tenant-manager/core"
	clog "github.com/LerianStudio/lib-observability/log"
	libOpentelemetry "github.com/LerianStudio/lib-observability/tracing"
	amqp "github.com/rabbitmq/amqp091-go"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

// sleepFunc is the function used for sleeping between retries.
// Overridable in tests for deterministic behavior.
var sleepFunc = time.Sleep

// backoff is the pre-configured producer backoff calculator.
var backoff = pkg.ProducerBackoff

// RabbitMQChannel is an interface for the AMQP channel used by the producer.
// This allows mocking the channel in tests for multi-tenant scenarios.
// It matches pkgRabbitmq.Channel — kept as local alias for bootstrap wiring compatibility.
type RabbitMQChannel = pkgRabbitmq.Channel

// RabbitMQManagerInterface is an interface for the tenant-manager RabbitMQ manager.
// It matches pkgRabbitmq.TenantChannelManager — kept as local alias for bootstrap wiring compatibility.
type RabbitMQManagerInterface = pkgRabbitmq.TenantChannelManager

// ProducerRabbitMQRepository is a rabbitmq implementation of the producer.
type ProducerRabbitMQRepository struct {
	conn            *libRabbitmq.RabbitMQConnection
	rabbitMQManager RabbitMQManagerInterface
	multiTenantMode bool
}

// Compile-time interface satisfaction check.
var _ pkgRabbitmq.ProducerRepository = (*ProducerRabbitMQRepository)(nil)

// NewProducerRabbitMQ returns a new instance of ProducerRabbitMQRepository using the given rabbitmq connection.
// Connection is established lazily on first use to avoid panic during initialization.
// This constructor is used for single-tenant mode.
func NewProducerRabbitMQ(c *libRabbitmq.RabbitMQConnection) *ProducerRabbitMQRepository {
	prmq := &ProducerRabbitMQRepository{
		conn:            c,
		multiTenantMode: false,
	}

	_, err := c.GetNewConnect()
	if err != nil {
		c.Logger.Log(context.Background(), clog.LevelError, "Failed to connect to RabbitMQ during initialization", clog.Err(err))
		c.Logger.Log(context.Background(), clog.LevelWarn, "RabbitMQ connection will be retried on first message publish")
	} else {
		c.Logger.Log(context.Background(), clog.LevelInfo, "RabbitMQ producer connected successfully")
	}

	return prmq
}

// NewProducerRabbitMQMultiTenant returns a new instance of ProducerRabbitMQRepository
// configured for multi-tenant mode using tmrabbitmq.Manager for per-tenant vhost isolation.
func NewProducerRabbitMQMultiTenant(manager RabbitMQManagerInterface) *ProducerRabbitMQRepository {
	return &ProducerRabbitMQRepository{
		rabbitMQManager: manager,
		multiTenantMode: true,
	}
}

// ProducerDefault publishes a message to RabbitMQ with retry logic.
// Retries up to ProducerMaxRetries with exponential backoff and full jitter.
func (prmq *ProducerRabbitMQRepository) ProducerDefault(ctx context.Context, exchange, key string, queueMessage model.ReportMessage) (*string, error) {
	logger := ctxutil.NewLoggerFromContext(ctx)
	tracer := ctxutil.NewTracerFromContext(ctx)
	reqID := ctxutil.HeaderIDFromContext(ctx)
	logger.Log(ctx, clog.LevelInfo, "Init sent message")

	ctx, spanProducer := tracer.Start(ctx, "repository.rabbitmq.publish_message")
	defer spanProducer.End()

	spanProducer.SetAttributes(
		attribute.String("app.request.request_id", reqID),
		attribute.String("app.request.exchange", exchange),
		attribute.String("app.request.key", key),
	)
	spanProducer.SetAttributes(queueMessageTelemetryAttributes(queueMessage)...)

	message, err := json.Marshal(queueMessage)
	if err != nil {
		libOpentelemetry.HandleSpanError(spanProducer, "Failed to marshal queue message struct", err)
		logger.Log(ctx, clog.LevelError, "Failed to marshal queue message struct", clog.Err(err))

		return nil, err
	}

	tenantID := tmcore.GetTenantIDContext(ctx)
	headers := pkgRabbitmq.NewProducerHeaders(reqID, tenantID)

	if prmq.multiTenantMode && tenantID == "" {
		logger.Log(ctx, clog.LevelWarn, "Multi-tenant mode enabled but no tenant ID in context — message will be published without X-Tenant-ID header",
			clog.String("exchange", exchange),
			clog.String("routing_key", key),
			clog.String("template_id", queueMessage.TemplateID.String()),
		)
	} else {
		logger.Log(ctx, clog.LevelInfo, "Publishing message to RabbitMQ",
			clog.String("tenant_id", tenantID),
			clog.Bool("multi_tenant_mode", prmq.multiTenantMode),
			clog.String("exchange", exchange),
			clog.String("routing_key", key),
			clog.String("template_id", queueMessage.TemplateID.String()),
		)
	}

	libOpentelemetry.InjectTraceHeadersIntoQueue(ctx, (*map[string]any)(&headers))

	if prmq.multiTenantMode {
		return nil, prmq.publishMultiTenant(ctx, exchange, key, headers, message, logger, spanProducer)
	}

	return nil, prmq.publishSingleTenant(ctx, exchange, key, headers, message, logger, spanProducer)
}

func queueMessageTelemetryAttributes(queueMessage model.ReportMessage) []attribute.KeyValue {
	return []attribute.KeyValue{
		attribute.String("app.request.template_id", queueMessage.TemplateID.String()),
		attribute.String("app.request.report_id", queueMessage.ReportID.String()),
		attribute.String("app.request.output_format", queueMessage.OutputFormat),
		attribute.Int("app.request.filter_datasource_count", len(queueMessage.Filters)),
		attribute.Int("app.request.mapped_field_datasource_count", len(queueMessage.MappedFields)),
	}
}

func (prmq *ProducerRabbitMQRepository) publishMultiTenant(
	ctx context.Context,
	exchange, key string,
	headers amqp.Table,
	message []byte,
	logger clog.Logger,
	spanProducer trace.Span,
) error {
	tenantID := tmcore.GetTenantIDContext(ctx)
	if tenantID == "" {
		libOpentelemetry.HandleSpanError(spanProducer, "Tenant ID is required in multi-tenant mode", fmt.Errorf("missing tenant ID"))
		return fmt.Errorf("tenant ID is required in multi-tenant mode")
	}

	spanProducer.SetAttributes(attribute.String("app.request.tenant_id", tenantID))

	currentBackoff := backoff.InitialDelay

	var publishErr error

	for attempt := 0; attempt <= constant.ProducerMaxRetries; attempt++ {
		if attempt > 0 {
			sleepDuration := backoff.Jitter(currentBackoff)
			logger.Log(ctx, clog.LevelInfo, "Retrying multi-tenant publish",
				clog.String("tenant_id", tenantID),
				clog.Int("attempt", attempt+1),
				clog.Int("max_attempts", constant.ProducerMaxRetries+1),
				clog.Any("backoff", sleepDuration),
			)

			spanProducer.SetAttributes(attribute.Int("app.request.rabbitmq.retry_attempt", attempt))
			sleepFunc(sleepDuration)

			currentBackoff = backoff.Next(currentBackoff)
		}

		channel, chanErr := prmq.rabbitMQManager.GetChannel(ctx, tenantID)
		if chanErr != nil {
			logger.Log(ctx, clog.LevelError, "Failed to get tenant channel",
				clog.String("tenant_id", tenantID),
				clog.Int("attempt", attempt+1),
				clog.Int("max_attempts", constant.ProducerMaxRetries+1),
				clog.Err(chanErr),
			)
			publishErr = chanErr

			if attempt == constant.ProducerMaxRetries {
				libOpentelemetry.HandleSpanError(spanProducer, "Failed to get tenant channel after all retries", chanErr)
				return fmt.Errorf("failed to get tenant channel after %d retries: %w", constant.ProducerMaxRetries, chanErr)
			}

			continue
		}

		publishErr = channel.PublishWithContext(ctx, exchange, key, false, false,
			amqp.Publishing{
				ContentType:  "application/json",
				DeliveryMode: amqp.Persistent,
				Headers:      headers,
				Body:         message,
			})

		if closeErr := channel.Close(); closeErr != nil {
			logger.Log(ctx, clog.LevelWarn, "Failed to close tenant channel", clog.String("tenant_id", tenantID), clog.Err(closeErr))
		}

		if publishErr == nil {
			logger.Log(ctx, clog.LevelInfo, "Message sent successfully to tenant vhost", clog.String("tenant_id", tenantID))
			return nil
		}

		logger.Log(ctx, clog.LevelError, "Multi-tenant publish failed",
			clog.String("tenant_id", tenantID),
			clog.Int("attempt", attempt+1),
			clog.Int("max_attempts", constant.ProducerMaxRetries+1),
			clog.Err(publishErr),
		)

		if attempt == constant.ProducerMaxRetries {
			libOpentelemetry.HandleSpanError(spanProducer, "Failed to publish message to tenant vhost after all retries", publishErr)
			return fmt.Errorf("failed to publish to tenant vhost after %d retries: %w", constant.ProducerMaxRetries, publishErr)
		}
	}

	return publishErr
}

func (prmq *ProducerRabbitMQRepository) publishSingleTenant(
	ctx context.Context,
	exchange, key string,
	headers amqp.Table,
	message []byte,
	logger clog.Logger,
	spanProducer trace.Span,
) error {
	currentBackoff := backoff.InitialDelay

	var publishErr error

	for attempt := 0; attempt <= constant.ProducerMaxRetries; attempt++ {
		if chanErr := prmq.conn.EnsureChannel(); chanErr != nil {
			logger.Log(ctx, clog.LevelError, "EnsureChannel failed",
				clog.Int("attempt", attempt+1),
				clog.Int("max_attempts", constant.ProducerMaxRetries+1),
				clog.Err(chanErr),
			)

			spanProducer.SetAttributes(attribute.Int("app.request.rabbitmq.retry_attempt", attempt))

			if attempt == constant.ProducerMaxRetries {
				libOpentelemetry.HandleSpanError(spanProducer, "Failed to ensure RabbitMQ channel after all retries", chanErr)
				return chanErr
			}

			sleepDuration := backoff.Jitter(currentBackoff)
			logger.Log(ctx, clog.LevelInfo, "Retrying EnsureChannel",
				clog.Any("backoff", sleepDuration),
				clog.Int("attempt", attempt+1),
				clog.Int("max_attempts", constant.ProducerMaxRetries+1),
			)

			sleepFunc(sleepDuration)

			currentBackoff = backoff.Next(currentBackoff)

			continue
		}

		if prmq.conn.Channel == nil {
			logger.Log(ctx, clog.LevelError, "RabbitMQ channel is nil after EnsureChannel succeeded",
				clog.Int("attempt", attempt+1),
			)

			if attempt == constant.ProducerMaxRetries {
				return fmt.Errorf("RabbitMQ channel is nil after all retries")
			}

			continue
		}

		publishErr = prmq.conn.Channel.Publish(exchange, key, false, false,
			amqp.Publishing{
				ContentType:  "application/json",
				DeliveryMode: amqp.Persistent,
				Headers:      headers,
				Body:         message,
			})
		if publishErr == nil {
			logger.Log(ctx, clog.LevelInfo, "Messages sent successfully")
			return nil
		}

		logger.Log(ctx, clog.LevelError, "Publish failed",
			clog.Int("attempt", attempt+1),
			clog.Int("max_attempts", constant.ProducerMaxRetries+1),
			clog.Err(publishErr),
		)

		spanProducer.SetAttributes(attribute.Int("app.request.rabbitmq.retry_attempt", attempt))

		if attempt == constant.ProducerMaxRetries {
			libOpentelemetry.HandleSpanError(spanProducer, "Failed to publish message after all retries", publishErr)
			return publishErr
		}

		sleepDuration := backoff.Jitter(currentBackoff)
		logger.Log(ctx, clog.LevelInfo, "Retrying publish",
			clog.Any("backoff", sleepDuration),
			clog.Int("attempt", attempt+1),
			clog.Int("max_attempts", constant.ProducerMaxRetries+1),
		)

		sleepFunc(sleepDuration)

		currentBackoff = backoff.Next(currentBackoff)
	}

	return publishErr
}
