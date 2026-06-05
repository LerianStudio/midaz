// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package rabbitmq

import (
	"context"
	"fmt"
	"sync"

	pkg "github.com/LerianStudio/midaz/v4/pkg/reporter"
	pkgConstant "github.com/LerianStudio/midaz/v4/pkg/reporter/constant"
	"github.com/LerianStudio/midaz/v4/pkg/reporter/ctxutil"
	pkgRabbitmq "github.com/LerianStudio/midaz/v4/pkg/reporter/rabbitmq"

	"github.com/LerianStudio/lib-commons/v5/commons/rabbitmq"
	tmcore "github.com/LerianStudio/lib-commons/v5/commons/tenant-manager/core"
	tmmongo "github.com/LerianStudio/lib-commons/v5/commons/tenant-manager/mongo"
	"github.com/LerianStudio/lib-observability/log"
	libOtel "github.com/LerianStudio/lib-observability/tracing"
	"github.com/rabbitmq/amqp091-go"

	mongoRepository "github.com/LerianStudio/midaz/v4/pkg/reporter/mongodb/report"

	"go.opentelemetry.io/otel/attribute"
)

// TenantResolver resolves per-tenant context (e.g. MongoDB) from message headers.
// Implementations: MultiTenantResolver (resolves MongoDB per tenant), NoOpTenantResolver (single-tenant passthrough).
type TenantResolver interface {
	Resolve(ctx context.Context, headers amqp091.Table) (context.Context, error)
}

// MultiTenantResolver resolves per-tenant MongoDB connections from message headers.
type MultiTenantResolver struct {
	MongoManager *tmmongo.Manager
	Logger       log.Logger
}

// Resolve extracts tenant ID from headers and injects the per-tenant MongoDB connection into context.
func (r *MultiTenantResolver) Resolve(ctx context.Context, headers amqp091.Table) (context.Context, error) {
	tenantID := pkgRabbitmq.TenantIDFromHeaders(headers)
	if tenantID == "" {
		return ctx, nil
	}

	ctx = tmcore.ContextWithTenantID(ctx, tenantID)

	tenantDB, err := r.MongoManager.GetDatabaseForTenant(ctx, tenantID)
	if err != nil {
		return ctx, err
	}

	return tmcore.ContextWithMB(ctx, tenantDB), nil
}

// NoOpTenantResolver is a passthrough for single-tenant mode — returns context unchanged.
type NoOpTenantResolver struct{}

// Resolve returns context unchanged (single-tenant: no tenant resolution needed).
func (r *NoOpTenantResolver) Resolve(ctx context.Context, _ amqp091.Table) (context.Context, error) {
	return ctx, nil
}

// ConsumerRoutes manages RabbitMQ queue consumption with worker pools.
// It delegates error classification, retry logic, and tenant resolution to injected components.
type ConsumerRoutes struct {
	conn            *rabbitmq.RabbitMQConnection
	routes          map[string]pkgRabbitmq.QueueHandlerFunc
	numWorkers      int
	retryManager    *ConsumerRetryManager
	tenantResolver  TenantResolver
	mongoRepository *mongoRepository.ReportMongoDBRepository
	log.Logger
	libOtel.Telemetry
}

// Compile-time interface satisfaction check.
var _ pkgRabbitmq.ConsumerRepository = (*ConsumerRoutes)(nil)

// NewConsumerRoutes creates a ConsumerRoutes for single-tenant mode.
// When mongoManager is non-nil, per-tenant MongoDB resolution is enabled via MultiTenantResolver.
func NewConsumerRoutes(
	conn *rabbitmq.RabbitMQConnection,
	numWorkers int,
	logger log.Logger,
	telemetry *libOtel.Telemetry,
	mongoManager *tmmongo.Manager,
	reportMongoDBRepository *mongoRepository.ReportMongoDBRepository,
) (*ConsumerRoutes, error) {
	if telemetry == nil {
		return nil, fmt.Errorf("telemetry must not be nil")
	}

	if numWorkers == 0 {
		numWorkers = pkgConstant.DefaultWorkerCount
	}

	var resolver TenantResolver = &NoOpTenantResolver{}
	if mongoManager != nil {
		resolver = &MultiTenantResolver{MongoManager: mongoManager, Logger: logger}
	}

	retryMgr := NewConsumerRetryManager(
		pkgRabbitmq.NewDefaultErrorClassifier(),
		pkg.ConsumerBackoff,
		conn,
		nil, // no multi-tenant channel manager in single-tenant mode
		logger,
		*telemetry,
	)

	cr := &ConsumerRoutes{
		conn:            conn,
		routes:          make(map[string]pkgRabbitmq.QueueHandlerFunc),
		numWorkers:      numWorkers,
		retryManager:    retryMgr,
		tenantResolver:  resolver,
		Logger:          logger,
		Telemetry:       *telemetry,
		mongoRepository: reportMongoDBRepository,
	}

	_, err := conn.GetNewConnect()
	if err != nil {
		return nil, fmt.Errorf("failed to connect to rabbitmq: %w", err)
	}

	return cr, nil
}

// NewConsumerRoutesMultiTenant creates a ConsumerRoutes for multi-tenant mode.
// Uses rabbitMQManager for per-tenant vhost isolation during retry republishing.
func NewConsumerRoutesMultiTenant(
	conn *rabbitmq.RabbitMQConnection,
	numWorkers int,
	logger log.Logger,
	telemetry *libOtel.Telemetry,
	mongoManager *tmmongo.Manager,
	rabbitMQManager RabbitMQManagerConsumerInterface,
	reportMongoDBRepository *mongoRepository.ReportMongoDBRepository,
) (*ConsumerRoutes, error) {
	if telemetry == nil {
		return nil, fmt.Errorf("telemetry must not be nil")
	}

	if numWorkers == 0 {
		numWorkers = pkgConstant.DefaultWorkerCount
	}

	var resolver TenantResolver = &NoOpTenantResolver{}
	if mongoManager != nil {
		resolver = &MultiTenantResolver{MongoManager: mongoManager, Logger: logger}
	}

	retryMgr := NewConsumerRetryManager(
		pkgRabbitmq.NewDefaultErrorClassifier(),
		pkg.ConsumerBackoff,
		conn,
		rabbitMQManager,
		logger,
		*telemetry,
	)

	cr := &ConsumerRoutes{
		conn:            conn,
		routes:          make(map[string]pkgRabbitmq.QueueHandlerFunc),
		numWorkers:      numWorkers,
		retryManager:    retryMgr,
		tenantResolver:  resolver,
		Logger:          logger,
		Telemetry:       *telemetry,
		mongoRepository: reportMongoDBRepository,
	}

	_, err := conn.GetNewConnect()
	if err != nil {
		return nil, fmt.Errorf("failed to connect to rabbitmq: %w", err)
	}

	return cr, nil
}

// Register adds a new queue handler mapping.
func (cr *ConsumerRoutes) Register(queueName string, handler pkgRabbitmq.QueueHandlerFunc) {
	cr.routes[queueName] = handler
}

// Info logs an informational message.
func (cr *ConsumerRoutes) Info(message string) {
	cr.Log(context.Background(), log.LevelInfo, message)
}

// RunConsumers starts worker pools for all registered queues.
func (cr *ConsumerRoutes) RunConsumers(ctx context.Context, wg *sync.WaitGroup) error {
	for queueName, handler := range cr.routes {
		cr.Log(ctx, log.LevelInfo, "Starting consumer for queue", log.String("queue", queueName))

		if err := cr.setupQos(); err != nil {
			return err
		}

		messages, err := cr.consumeMessages(queueName)
		if err != nil {
			return err
		}

		cr.startWorkers(ctx, wg, messages, queueName, handler)
	}

	return nil
}

// startWorkers spawns numWorkers goroutines to process messages from a queue.
func (cr *ConsumerRoutes) startWorkers(ctx context.Context, wg *sync.WaitGroup, messages <-chan amqp091.Delivery, queueName string, handler pkgRabbitmq.QueueHandlerFunc) {
	for i := range cr.numWorkers {
		wg.Add(1)

		go func(workerID int, queue string, handlerFunc pkgRabbitmq.QueueHandlerFunc) {
			defer wg.Done()
			defer recoverWorkerPanic(cr.Logger, workerID, queue)

			for {
				select {
				case <-ctx.Done():
					cr.Log(ctx, log.LevelInfo, "Worker shutting down gracefully", log.Int("worker_id", workerID))
					return
				case message, ok := <-messages:
					if !ok {
						cr.Log(ctx, log.LevelInfo, "Worker message channel closed", log.Int("worker_id", workerID))
						return
					}

					cr.processMessage(workerID, queue, handlerFunc, message)
				}
			}
		}(i, queueName, handler)
	}
}

// processMessage processes a single message: builds context, resolves tenant, invokes handler, handles errors.
func (cr *ConsumerRoutes) processMessage(workerID int, queue string, handlerFunc pkgRabbitmq.QueueHandlerFunc, message amqp091.Delivery) {
	ctx, requestIDStr := buildMessageContext(cr.Logger, message)

	tracer := ctxutil.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "repository.rabbitmq.process_message")
	defer span.End()

	ctx = libOtel.ExtractTraceContextFromQueueHeaders(ctx, message.Headers)

	// Resolve tenant context (MongoDB per-tenant in MT mode, no-op in ST mode)
	ctx, tenantErr := cr.tenantResolver.Resolve(ctx, message.Headers)
	if tenantErr != nil {
		retryCount := pkgRabbitmq.GetRetryCount(message.Headers)
		classifier := cr.retryManager.classifier

		if classifier.IsPermanentTenantError(tenantErr) {
			cr.Log(ctx, log.LevelError, "Permanent tenant error - will not retry",
				log.Int("worker_id", workerID),
				log.Err(tenantErr),
			)
			libOtel.HandleSpanBusinessErrorEvent(span, "Permanent tenant error, routing to DLQ", tenantErr)
		} else {
			cr.Log(ctx, log.LevelError, "Transient tenant error",
				log.Int("worker_id", workerID),
				log.Int("attempt", retryCount+1),
				log.Int("max_retries", pkgConstant.MaxMessageRetries),
				log.Err(tenantErr),
			)
			libOtel.HandleSpanError(span, "Transient tenant error, will retry", tenantErr)
		}

		cr.retryManager.HandleFailure(ctx, workerID, queue, message, tenantErr, retryCount, span)

		return
	}

	span.SetAttributes(attribute.String("app.request.request_id", requestIDStr))
	span.SetAttributes(deliveryTelemetryAttributes(message)...)

	retryCount := pkgRabbitmq.GetRetryCount(message.Headers)
	span.SetAttributes(attribute.Int("app.request.rabbitmq.consumer.retry_count", retryCount))

	// Extract tenant IDs for observability — diagnose S3 path mismatches (TPL-0034)
	headerTenantID := pkgRabbitmq.TenantIDFromHeaders(message.Headers)
	resolvedTenantID := tmcore.GetTenantIDContext(ctx)

	if headerTenantID == "" {
		cr.Log(ctx, log.LevelWarn, "Message missing X-Tenant-ID header — S3 paths will not include tenant prefix",
			log.Int("worker_id", workerID),
			log.String("queue", queue),
		)
	}

	if resolvedTenantID != "" {
		span.SetAttributes(attribute.String("app.request.tenant_id", resolvedTenantID))
	}

	cr.Log(ctx, log.LevelInfo, "Starting message processing",
		log.Int("worker_id", workerID),
		log.String("queue", queue),
		log.Int("attempt", retryCount+1),
		log.String("header_tenant_id", headerTenantID),
		log.String("resolved_tenant_id", resolvedTenantID),
	)

	err := handlerFunc(ctx, message.Body)
	if err != nil {
		cr.Log(ctx, log.LevelError, "Error processing message",
			log.Int("worker_id", workerID),
			log.String("queue", queue),
			log.Err(err),
		)
		libOtel.HandleSpanError(span, "Error processing message", err)

		cr.retryManager.HandleFailure(ctx, workerID, queue, message, err, retryCount, span)

		return
	}

	_ = message.Ack(false)

	cr.Log(ctx, log.LevelInfo, "Successfully processed message",
		log.Int("worker_id", workerID),
		log.String("queue", queue),
	)
}

// consumeMessages establishes a consumer for the specified queue.
func (cr *ConsumerRoutes) consumeMessages(queueName string) (<-chan amqp091.Delivery, error) {
	if cr.conn.Channel == nil {
		return nil, fmt.Errorf("rabbitmq channel is nil, cannot consume from queue %s", queueName)
	}

	return cr.conn.Channel.Consume(queueName, "", false, false, false, false, nil)
}

// setupQos configures QoS for the RabbitMQ channel.
func (cr *ConsumerRoutes) setupQos() error {
	if cr.conn.Channel == nil {
		return fmt.Errorf("rabbitmq channel is nil, cannot setup QoS")
	}

	return cr.conn.Channel.Qos(pkgConstant.DefaultPrefetchCount, 0, false)
}

// RabbitMQConnectionChannel abstracts the AMQP channel for retry republishing.
type RabbitMQConnectionChannel interface {
	Publish(exchange, key string, mandatory, immediate bool, msg amqp091.Publishing) error
}

// RabbitMQManagerConsumerInterface provides per-tenant vhost channels for retry republishing.
type RabbitMQManagerConsumerInterface interface {
	GetConnection(ctx context.Context, tenantID string) (RabbitMQConnectionChannel, error)
}
