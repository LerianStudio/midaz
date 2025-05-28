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
	"go.opentelemetry.io/otel"
)

// SetupObservability configures comprehensive observability for the onboarding service
func SetupObservability(
	app *fiber.App,
	cfg *Config,
	logger libLog.Logger,
	telemetry *libOpentelemetry.Telemetry,
	postgresDB *sql.DB,
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

	// Add custom health checks
	healthService.RegisterChecker("disk_space", libHealth.NewCustomChecker("disk_space", checkDiskSpace))
	healthService.RegisterChecker("memory", libHealth.NewCustomChecker("memory", checkMemoryUsage))

	// Check connection to transaction service
	healthService.RegisterChecker("transaction_service", libHealth.NewHTTPChecker(
		"transaction_service",
		"http://transaction:8080/health", // TODO: Make this configurable
		nil,
	))

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

// OnboardingObservabilityMiddleware adds onboarding-specific observability
func OnboardingObservabilityMiddleware(businessMetrics *libObservability.BusinessMetrics) fiber.Handler {
	return func(c *fiber.Ctx) error {
		ctx := c.UserContext()
		// TODO: logger := libCommons.NewStructuredLoggerFromContext(ctx) - function needs to be implemented
		logger := libLog.NewStructuredLogger(libCommons.NewLoggerFromContext(ctx))

		// Add onboarding-specific context
		if c.Params("organization_id") != "" {
			logger = logger.WithBusinessContext(c.Params("organization_id"), c.Params("ledger_id"))
		}

		// Store enhanced logger in context
		ctx = context.WithValue(ctx, libCommons.LoggerKey, logger)
		c.SetUserContext(ctx)

		return c.Next()
	}
}

// InstrumentOnboardingHandler adds observability to onboarding handlers
func InstrumentOnboardingHandler(
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
		ctx, span := tracer.Start(ctx, fmt.Sprintf("onboarding.%s", operationName))
		defer span.End()

		// Update context
		c.SetUserContext(ctx)

		// Log operation start
		logger.WithOperation(operationName).Debug("Starting onboarding operation")

		// Execute handler
		err := handler(c)

		// Record metrics based on operation
		if err == nil && c.Response().StatusCode() < 400 {
			organizationID := c.Params("organization_id")
			ledgerID := c.Params("ledger_id")

			// Record specific metrics based on operation
			switch operationName {
			case "create_organization":
				businessMetrics.RecordAccount(ctx, organizationID, "", "organization")
			case "create_ledger":
				businessMetrics.RecordLedger(ctx, organizationID)
			case "create_asset":
				businessMetrics.RecordAsset(ctx, organizationID, ledgerID, "custom")
			case "create_account":
				businessMetrics.RecordAccount(ctx, organizationID, ledgerID, "standard")
			}
		}

		return err
	}
}

// TraceServiceCall traces a call to another service with distributed tracing
func TraceServiceCall(
	ctx context.Context,
	serviceName string,
	operationName string,
	call func(context.Context) error,
) error {
	tracer := otel.Tracer("onboarding-service")
	// TODO: tracingHelper := libCommons.GetDistributedTracingHelper() - function needs to be implemented
	tracingHelper := libObservability.NewDistributedTracingHelper()

	return tracingHelper.PropagateServiceCall(
		ctx,
		tracer,
		serviceName,
		operationName,
		call,
	)
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
