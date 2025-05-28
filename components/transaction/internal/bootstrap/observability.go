package bootstrap

import (
	"context"
	"database/sql"
	"fmt"
	"os"

	libCommons "github.com/LerianStudio/lib-commons/commons"
	libHealth "github.com/LerianStudio/lib-commons/commons/health"
	libLog "github.com/LerianStudio/lib-commons/commons/log"
	libObservability "github.com/LerianStudio/lib-commons/commons/observability"
	libOpentelemetry "github.com/LerianStudio/lib-commons/commons/opentelemetry"
	"github.com/gofiber/fiber/v2"
	amqp "github.com/rabbitmq/amqp091-go"
	redisClient "github.com/redis/go-redis/v9"
	"go.mongodb.org/mongo-driver/mongo"
)

// SetupObservability configures comprehensive observability for the transaction service
func SetupObservability(
	app *fiber.App,
	cfg *Config,
	logger libLog.Logger,
	telemetry *libOpentelemetry.Telemetry,
	postgresDB *sql.DB,
	mongoDB *mongo.Database,
	redisDB *redisClient.Client,
	rabbitConn *amqp.Connection,
) error {
	// Create structured logger
	structuredLogger := libLog.NewStructuredLogger(logger)

	// Initialize business metrics
	businessMetrics, err := libObservability.NewBusinessMetrics(telemetry.MetricProvider.Meter(cfg.OtelServiceName))
	if err != nil {
		return fmt.Errorf("failed to create business metrics: %w", err)
	}

	// Store metrics in context for use in handlers
	// TODO: libCommons.SetBusinessMetrics(businessMetrics) - function needs to be implemented
	_ = businessMetrics

	// Create observability middleware
	obsMiddleware, err := libObservability.NewObservabilityMiddleware(
		cfg.OtelServiceName,
		telemetry.TracerProvider,
		telemetry.MetricProvider,
		structuredLogger,
	)
	if err != nil {
		return fmt.Errorf("failed to create observability middleware: %w", err)
	}

	// Apply middleware globally
	app.Use(obsMiddleware.Middleware())

	// Setup health checks
	healthService := libHealth.NewService(
		cfg.OtelServiceName,
		cfg.OtelServiceVersion,
		cfg.OtelDeploymentEnv,
		getHostname(),
	)

	// Register health checkers
	healthService.RegisterChecker("postgresql", libHealth.NewPostgresChecker(postgresDB))
	healthService.RegisterChecker("mongodb", libHealth.NewMongoChecker(mongoDB.Client()))
	healthService.RegisterChecker("redis", libHealth.NewRedisChecker(redisDB))
	healthService.RegisterChecker("rabbitmq", libHealth.NewRabbitMQChecker(rabbitConn))

	// Add custom health checks
	healthService.RegisterChecker("disk_space", libHealth.NewCustomChecker("disk_space", checkDiskSpace))
	healthService.RegisterChecker("memory", libHealth.NewCustomChecker("memory", checkMemoryUsage))

	// Register health endpoints
	app.Get("/health", healthService.Handler())
	app.Get("/health/live", func(c *fiber.Ctx) error {
		return c.JSON(fiber.Map{"status": "alive"})
	})
	app.Get("/health/ready", healthService.Handler())

	// Setup distributed tracing helper
	tracingHelper := libObservability.NewDistributedTracingHelper()
	// TODO: libCommons.SetDistributedTracingHelper(tracingHelper) - function needs to be implemented
	_ = tracingHelper

	// Log successful setup
	structuredLogger.WithService(cfg.OtelServiceName).Info("Observability setup completed successfully")

	return nil
}

// Enhanced middleware for transaction-specific operations
func TransactionObservabilityMiddleware(businessMetrics *libObservability.BusinessMetrics) fiber.Handler {
	return func(c *fiber.Ctx) error {
		ctx := c.UserContext()
		// TODO: logger := libCommons.NewStructuredLoggerFromContext(ctx) - function needs to be implemented
		logger := libLog.NewStructuredLogger(libCommons.NewLoggerFromContext(ctx))

		// Add transaction-specific context
		if c.Params("organization_id") != "" {
			logger = logger.WithBusinessContext(c.Params("organization_id"), c.Params("ledger_id"))
		}

		// Store enhanced logger in context
		ctx = context.WithValue(ctx, libCommons.LoggerKey, logger)
		c.SetUserContext(ctx)

		return c.Next()
	}
}

// InstrumentTransactionHandler adds observability to transaction handlers
func InstrumentTransactionHandler(
	handler fiber.Handler,
	operationName string,
	businessMetrics *libObservability.BusinessMetrics,
) fiber.Handler {
	return func(c *fiber.Ctx) error {
		ctx := c.UserContext()
		tracer := libCommons.NewTracerFromContext(ctx)
		// TODO: logger := libCommons.NewStructuredLoggerFromContext(ctx) - function needs to be implemented
		logger := libLog.NewStructuredLogger(libCommons.NewLoggerFromContext(ctx))

		// Start operation span
		ctx, span := tracer.Start(ctx, fmt.Sprintf("transaction.%s", operationName))
		defer span.End()

		// Update context
		c.SetUserContext(ctx)

		// Log operation start
		logger.WithOperation(operationName).Debug("Starting transaction operation")

		// Execute handler
		err := handler(c)

		// Record metrics based on response
		if err == nil && c.Response().StatusCode() < 400 {
			organizationID := c.Params("organization_id")
			ledgerID := c.Params("ledger_id")

			// Record success metrics
			businessMetrics.RecordTransaction(
				ctx,
				organizationID,
				ledgerID,
				"success",
				operationName,
				0, // Amount would be extracted from request/response
				"",
			)
		} else {
			// Record error metrics
			businessMetrics.RecordTransactionError(
				ctx,
				c.Params("organization_id"),
				c.Params("ledger_id"),
				"handler_error",
			)
		}

		return err
	}
}

// Helper functions for health checks
func checkDiskSpace(ctx context.Context) error {
	// Implement disk space check
	// This is a simplified example
	return nil
}

func checkMemoryUsage(ctx context.Context) error {
	// Implement memory usage check
	// This is a simplified example
	return nil
}

func getHostname() string {
	hostname, err := os.Hostname()
	if err != nil {
		return "unknown"
	}
	return hostname
}
