// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"sync"
	"syscall"

	"github.com/LerianStudio/midaz/v3/components/reporter-worker/internal/adapters/rabbitmq"
	"github.com/LerianStudio/midaz/v3/components/reporter-worker/internal/services"
	pkg "github.com/LerianStudio/midaz/v3/pkg/reporter"
	"github.com/LerianStudio/midaz/v3/pkg/reporter/ctxutil"
	pkgHTTP "github.com/LerianStudio/midaz/v3/pkg/reporter/net/http"
	"github.com/LerianStudio/midaz/v3/pkg/reporter/readyz"

	"github.com/LerianStudio/lib-commons/v5/commons"
	tmconsumer "github.com/LerianStudio/lib-commons/v5/commons/tenant-manager/consumer"
	tmcore "github.com/LerianStudio/lib-commons/v5/commons/tenant-manager/core"
	tmmongo "github.com/LerianStudio/lib-commons/v5/commons/tenant-manager/mongo"
	"github.com/LerianStudio/lib-observability/log"
	opentelemetry "github.com/LerianStudio/lib-observability/tracing"
	amqp "github.com/rabbitmq/amqp091-go"
	"go.opentelemetry.io/otel/attribute"
)

// MultiTenantConsumerInterface abstracts the tmconsumer.MultiTenantConsumer for testing.
type MultiTenantConsumerInterface interface {
	Register(queueName string, handler tmconsumer.HandlerFunc) error
	Run(ctx context.Context) error
	Close() error
}

// MultiQueueConsumer represents a multi-queue consumer.
// It supports two modes:
//   - Single-tenant: Uses consumerRoutes with static RabbitMQ connection
//   - Multi-tenant: Uses mtConsumer (tmconsumer.MultiTenantConsumer) with per-tenant vhost isolation
type MultiQueueConsumer struct {
	consumerRoutes *rabbitmq.ConsumerRoutes
	mtConsumer     MultiTenantConsumerInterface // Multi-tenant consumer (nil in single-tenant mode)
	UseCase        *services.UseCase
	logger         log.Logger
	queueName      string           // Stored for multi-tenant handler registration
	mongoManager   *tmmongo.Manager // For per-tenant MongoDB resolution (nil in single-tenant mode)
	// drainState is the shared graceful-shutdown flag. Run() flips it on
	// SIGTERM/SIGINT BEFORE cancelling the consumer context so /readyz
	// reports 503 draining while in-flight messages finish.
	drainState *readyz.DrainState
}

// NewMultiQueueConsumer create a new instance of MultiQueueConsumer for single-tenant mode.
//
// drainState is optional — pass nil to disable drain coordination (legacy
// behavior). Tests that exercise constructor wiring may pass nil; production
// code should always pass the shared DrainState.
func NewMultiQueueConsumer(routes *rabbitmq.ConsumerRoutes, useCase *services.UseCase, queueName string, logger log.Logger, drainState *readyz.DrainState) *MultiQueueConsumer {
	consumer := &MultiQueueConsumer{
		consumerRoutes: routes,
		mtConsumer:     nil, // Single-tenant mode
		UseCase:        useCase,
		logger:         logger,
		queueName:      queueName,
		drainState:     drainState,
	}

	// Register handlers for each queue
	if routes != nil {
		routes.Register(queueName, consumer.handlerGenerateReport)
	}

	return consumer
}

// NewMultiQueueConsumerMultiTenant creates a new instance of MultiQueueConsumer for multi-tenant mode.
// It uses tmconsumer.MultiTenantConsumer for per-tenant vhost isolation with lazy initialization.
// The handler is registered with the MultiTenantConsumer to process messages from per-tenant queues.
//
// drainState is optional — pass nil to disable drain coordination.
func NewMultiQueueConsumerMultiTenant(
	mtConsumer MultiTenantConsumerInterface,
	useCase *services.UseCase,
	queueName string,
	logger log.Logger,
	mongoManager *tmmongo.Manager,
	drainState *readyz.DrainState,
) (*MultiQueueConsumer, error) {
	if mtConsumer == nil {
		return nil, fmt.Errorf("NewMultiQueueConsumerMultiTenant: mtConsumer must not be nil in multi-tenant mode")
	}

	consumer := &MultiQueueConsumer{
		consumerRoutes: nil, // Multi-tenant mode uses mtConsumer
		mtConsumer:     mtConsumer,
		UseCase:        useCase,
		logger:         logger,
		queueName:      queueName,
		mongoManager:   mongoManager,
		drainState:     drainState,
	}

	// Register handler with MultiTenantConsumer
	// The handler signature is tmconsumer.HandlerFunc: func(ctx, amqp.Delivery) error
	if err := mtConsumer.Register(queueName, consumer.handlerGenerateReportDelivery); err != nil {
		if closeErr := mtConsumer.Close(); closeErr != nil {
			logger.Log(context.Background(), log.LevelError, "MultiTenantConsumer: failed to close consumer after register failure", log.Err(closeErr))
		}

		return nil, fmt.Errorf("MultiTenantConsumer: failed to register handler for queue %s: %w", queueName, err)
	}

	logger.Log(context.Background(), log.LevelInfo, "MultiTenantConsumer: handler registered for queue",
		log.String("queue", queueName))

	return consumer, nil
}

// Run starts consumers for all registered queues.
// In multi-tenant mode, uses mtConsumer.Run() which discovers tenants from Redis
// and spawns consumer goroutines per-tenant vhost.
// In single-tenant mode, uses consumerRoutes.RunConsumers() with static connection.
//
// On SIGTERM/SIGINT the listener flips drainState BEFORE cancelling the
// consumer context. This causes /readyz to report 503 draining while
// in-flight messages finish, so K8s and load balancers stop routing new
// work to this pod before the RabbitMQ consumers begin tearing down.
func (mq *MultiQueueConsumer) Run(l *commons.Launcher) error {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, os.Interrupt, syscall.SIGTERM)

	pkg.GoWithCleanup(mq.logger, func() {
		<-sigs

		if mq.drainState != nil {
			mq.drainState.StartDraining()
			mq.logger.Log(ctx, log.LevelInfo, "drain_started")
		}

		cancel()
	}, func(_ any) {
		cancel()
	})

	// Multi-tenant mode: use tmconsumer.MultiTenantConsumer
	if mq.mtConsumer != nil {
		// Enrich the root context with logger and tracer so that downstream handlers
		// (which extract them via ctxutil.NewLoggerFromContext / NewTracerFromContext)
		// receive real instances instead of NopLogger/NopTracer from a bare context.Background().
		ctx = ctxutil.ContextWithLogger(ctx, mq.logger)
		ctx = ctxutil.ContextWithTracer(ctx, mq.UseCase.Tracer)
		mq.logger.Log(ctx, log.LevelInfo, "MultiQueueConsumer: starting multi-tenant consumer with per-tenant vhost isolation")

		// Run starts tenant discovery and spawns consumer goroutines
		if err := mq.mtConsumer.Run(ctx); err != nil {
			return err
		}

		// Block until context is canceled (shutdown signal)
		<-ctx.Done()
		mq.logger.Log(ctx, log.LevelInfo, "MultiQueueConsumer: shutting down multi-tenant consumer")

		// Close gracefully stops all tenant consumers
		if err := mq.mtConsumer.Close(); err != nil {
			mq.logger.Log(ctx, log.LevelError, "MultiQueueConsumer: error closing multi-tenant consumer", log.Err(err))
		}

		return nil
	}

	// Single-tenant mode: use ConsumerRoutes with static connection
	wg := &sync.WaitGroup{}

	if err := mq.consumerRoutes.RunConsumers(ctx, wg); err != nil {
		return err
	}

	wg.Wait()

	return nil
}

// resolveMultiTenantMongo resolves the per-tenant MongoDB connection and injects
// it into the context. It is a no-op when mongoManager is nil (single-tenant mode)
// or when the context has no tenant ID.
func (mq *MultiQueueConsumer) resolveMultiTenantMongo(ctx context.Context) (context.Context, error) {
	if mq.mongoManager == nil {
		return ctx, nil
	}

	tenantID := tmcore.GetTenantIDContext(ctx)
	if tenantID == "" {
		return ctx, nil
	}

	tenantDB, err := mq.mongoManager.GetDatabaseForTenant(ctx, tenantID)
	if err != nil {
		// Differentiate permanent vs transient tenant errors for observability.
		// The error is wrapped with %w to preserve the error chain for
		// upstream callers (e.g., isRetryable, isPermanentTenantError).
		if tmcore.IsTenantSuspendedError(err) {
			mq.logger.Log(ctx, log.LevelError, "Tenant is suspended/purged, permanent failure",
				log.String("tenant_id", tenantID), log.Err(err))
		} else if errors.Is(err, tmcore.ErrTenantNotFound) {
			mq.logger.Log(ctx, log.LevelError, "Tenant not found, permanent failure",
				log.String("tenant_id", tenantID), log.Err(err))
		} else if errors.Is(err, tmcore.ErrServiceNotConfigured) {
			mq.logger.Log(ctx, log.LevelError, "Service not configured for tenant, permanent failure",
				log.String("tenant_id", tenantID), log.Err(err))
		} else if tmcore.IsCircuitBreakerOpenError(err) {
			mq.logger.Log(ctx, log.LevelError, "Circuit breaker open for tenant, transient failure",
				log.String("tenant_id", tenantID), log.Err(err))
		} else {
			mq.logger.Log(ctx, log.LevelError, "Failed to resolve tenant MongoDB",
				log.String("tenant_id", tenantID), log.Err(err))
		}

		return ctx, fmt.Errorf("resolve tenant mongo for tenant %s: %w", tenantID, err)
	}

	return tmcore.ContextWithMB(ctx, tenantDB), nil
}

// handlerGenerateReportDelivery is the tmconsumer.HandlerFunc adapter for multi-tenant mode.
// It resolves per-tenant MongoDB if mongoManager is available, then delegates to handlerGenerateReport.
//
// DEFENSIVE RETRY GUARD: lib-commons multi-tenant consumer calls msg.Nack(false, true)
// for any non-nil handler error, which causes infinite redelivery for permanent errors.
// This handler returns nil for non-retryable errors (after logging) so that lib-commons
// Acks the message instead of requeuing it indefinitely.
func (mq *MultiQueueConsumer) handlerGenerateReportDelivery(ctx context.Context, delivery amqp.Delivery) error {
	ctx, err := mq.resolveMultiTenantMongo(ctx)
	if err != nil {
		if isNonRetryableHandlerError(err) {
			mq.logger.Log(ctx, log.LevelWarn, "Permanent tenant resolution failure (message will be dropped)",
				log.Err(err))

			return nil
		}

		return err
	}

	err = mq.handlerGenerateReport(ctx, delivery.Body)
	if err != nil {
		if isNonRetryableHandlerError(err) {
			mq.logger.Log(ctx, log.LevelWarn, "Non-retryable handler error (message will be dropped)",
				log.Err(err))

			return nil
		}

		return err
	}

	return nil
}

// handlerGenerateReport processes messages from the generate report queue.
func (mq *MultiQueueConsumer) handlerGenerateReport(ctx context.Context, body []byte) error {
	reqId := ctxutil.HeaderIDFromContext(ctx)

	ctx, span := mq.UseCase.Tracer.Start(ctx, "handler.report.generate")
	defer span.End()

	span.SetAttributes(
		attribute.String("app.request.request_id", reqId),
	)
	mq.UseCase.Logger.Log(ctx, log.LevelInfo, "Processing message from generate report queue")

	err := mq.UseCase.GenerateReport(ctx, body)
	if err != nil {
		if pkgHTTP.IsBusinessError(err) {
			opentelemetry.HandleSpanBusinessErrorEvent(span, "Error generating report.", err)
		} else {
			opentelemetry.HandleSpanError(span, "Error generating report.", err)
		}

		mq.UseCase.Logger.Log(ctx, log.LevelError, "Error generating report", log.Err(err))

		return err
	}

	return nil
}
