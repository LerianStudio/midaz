// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"context"
	"errors"
	nethttp "net/http"

	"github.com/LerianStudio/midaz/v3/pkg/reporter/constant"
	"github.com/LerianStudio/midaz/v3/pkg/reporter/model"
	"github.com/LerianStudio/midaz/v3/pkg/reporter/mongodb/deadline"
	"github.com/LerianStudio/midaz/v3/pkg/reporter/net/http"
	"github.com/LerianStudio/midaz/v3/pkg/reporter/readyz"

	middlewareAuth "github.com/LerianStudio/lib-auth/v2/auth/middleware"
	commonsHttp "github.com/LerianStudio/lib-commons/v5/commons/net/http"
	libObservability "github.com/LerianStudio/lib-observability"
	"github.com/LerianStudio/lib-observability/log"
	libObsMiddleware "github.com/LerianStudio/lib-observability/middleware"
	opentelemetry "github.com/LerianStudio/lib-observability/tracing"
	"github.com/gofiber/contrib/otelfiber/v2"
	"github.com/gofiber/fiber/v2"
	fiberSwagger "github.com/swaggo/fiber-swagger"
	"go.opentelemetry.io/otel/trace"
)

const (
	deadlineResource   = "deadlines"
	templateResource   = "templates"
	reportResource     = "reports"
	dataSourceResource = "data-source"
	metricsResource    = "metrics"
)

// NewRoutes creates a new fiber router with the specified handlers and middleware.
// tenantMiddleware is optional: pass nil to disable multi-tenant DB resolution (single-tenant mode).
func NewRoutes(lg log.Logger, tl *opentelemetry.Telemetry, templateHandler *TemplateHandler, reportHandler *ReportHandler, dataSourceHandler *DataSourceHandler, deadlineHandler *DeadlineHandler, templateBuilderHandler *TemplateBuilderHandler, metricsHandler *MetricsHandler, notificationHandler *NotificationHandler, auth *middlewareAuth.AuthClient, deps *ManagerReadyzDeps, corsConfig CORSConfig, trustedProxies []string, ttMiddleware fiber.Handler) *fiber.App {
	fiberCfg := fiber.Config{
		DisableStartupMessage: true,
		ErrorHandler: func(ctx *fiber.Ctx, err error) error {
			return legacyFiberErrorHandler(ctx, err)
		},
	}

	if len(trustedProxies) > 0 {
		fiberCfg.EnableTrustedProxyCheck = true
		fiberCfg.TrustedProxies = trustedProxies
		fiberCfg.ProxyHeader = fiber.HeaderXForwardedFor
	}

	f := fiber.New(fiberCfg)
	tlMid := libObsMiddleware.NewTelemetryMiddleware(tl)

	f.Use(tlMid.WithTelemetry(tl))
	f.Use(RecoverMiddleware())
	f.Use(otelfiber.Middleware(
		otelfiber.WithNext(func(c *fiber.Ctx) bool {
			// Skip tracing for health/ready endpoints to reduce noise
			path := c.Path()
			return path == "/health" || path == "/readyz"
		}),
	))
	f.Use(SecurityHeaders())
	f.Use(CORSMiddleware(corsConfig))
	f.Use(libObsMiddleware.WithHTTPLogging(libObsMiddleware.WithCustomLogger(lg)))

	// Template builder routes (static paths before parameterized :id)
	f.Get("/v1/templates/blocks-config", auth.Authorize(constant.ApplicationName, templateResource, "get"), WhenEnabled(ttMiddleware), templateBuilderHandler.GetBlocksConfig)
	f.Get("/v1/templates/filters", auth.Authorize(constant.ApplicationName, templateResource, "get"), WhenEnabled(ttMiddleware), templateBuilderHandler.GetFiltersConfig)
	f.Post("/v1/templates/generate-code", auth.Authorize(constant.ApplicationName, templateResource, "post"), WhenEnabled(ttMiddleware), templateBuilderHandler.GenerateCode)
	f.Post("/v1/templates/validate", auth.Authorize(constant.ApplicationName, templateResource, "post"), WhenEnabled(ttMiddleware), templateBuilderHandler.ValidateBlocks)

	// Template routes
	f.Post("/v1/templates", auth.Authorize(constant.ApplicationName, templateResource, "post"), WhenEnabled(ttMiddleware), templateHandler.CreateTemplate)
	f.Patch("/v1/templates/:id", auth.Authorize(constant.ApplicationName, templateResource, "patch"), WhenEnabled(ttMiddleware), ParsePathParametersUUID, templateHandler.UpdateTemplateByID)
	f.Get("/v1/templates/:id", auth.Authorize(constant.ApplicationName, templateResource, "get"), WhenEnabled(ttMiddleware), ParsePathParametersUUID, templateHandler.GetTemplateByID)
	f.Get("/v1/templates", auth.Authorize(constant.ApplicationName, templateResource, "get"), WhenEnabled(ttMiddleware), templateHandler.GetAllTemplates)
	f.Delete("/v1/templates/:id", auth.Authorize(constant.ApplicationName, templateResource, "delete"), WhenEnabled(ttMiddleware), ParsePathParametersUUID, templateHandler.DeleteTemplateByID)

	// Report routes
	f.Post("/v1/reports", auth.Authorize(constant.ApplicationName, reportResource, "post"), WhenEnabled(ttMiddleware), http.WithBody(new(model.CreateReportInput), reportHandler.CreateReport))
	f.Get("/v1/reports/:id/download", auth.Authorize(constant.ApplicationName, reportResource, "get"), WhenEnabled(ttMiddleware), ParsePathParametersUUID, reportHandler.GetDownloadReport)
	f.Get("/v1/reports/:id", auth.Authorize(constant.ApplicationName, reportResource, "get"), WhenEnabled(ttMiddleware), ParsePathParametersUUID, reportHandler.GetReport)
	f.Get("/v1/reports", auth.Authorize(constant.ApplicationName, reportResource, "get"), WhenEnabled(ttMiddleware), reportHandler.GetAllReports)

	// Deadline routes
	f.Post("/v1/deadlines", auth.Authorize(constant.ApplicationName, deadlineResource, "post"), WhenEnabled(ttMiddleware), http.WithBody(new(deadline.CreateDeadlineInput), deadlineHandler.CreateDeadline))
	f.Get("/v1/deadlines", auth.Authorize(constant.ApplicationName, deadlineResource, "get"), WhenEnabled(ttMiddleware), deadlineHandler.GetAllDeadlines)
	f.Get("/v1/deadlines/notifications", auth.Authorize(constant.ApplicationName, deadlineResource, "get"), WhenEnabled(ttMiddleware), notificationHandler.GetNotifications)
	f.Patch("/v1/deadlines/:id", auth.Authorize(constant.ApplicationName, deadlineResource, "patch"), WhenEnabled(ttMiddleware), ParsePathParametersUUID, http.WithBody(new(deadline.UpdateDeadlineInput), deadlineHandler.UpdateDeadlineByID))
	f.Delete("/v1/deadlines/:id", auth.Authorize(constant.ApplicationName, deadlineResource, "delete"), WhenEnabled(ttMiddleware), ParsePathParametersUUID, deadlineHandler.DeleteDeadlineByID)
	f.Patch("/v1/deadlines/:id/deliver", auth.Authorize(constant.ApplicationName, deadlineResource, "patch"), WhenEnabled(ttMiddleware), ParsePathParametersUUID, http.WithBody(new(deadline.DeliverDeadlineInput), deadlineHandler.DeliverDeadline))

	// Data source routes
	f.Get("/v1/data-sources", auth.Authorize(constant.ApplicationName, dataSourceResource, "get"), WhenEnabled(ttMiddleware), dataSourceHandler.GetDataSourceInformation)
	f.Get("/v1/data-sources/:dataSourceId", auth.Authorize(constant.ApplicationName, dataSourceResource, "get"), WhenEnabled(ttMiddleware), ParseStringPathParam("dataSourceId"), dataSourceHandler.GetDataSourceInformationByID)

	// Metrics routes
	f.Get("/v1/metrics", auth.Authorize(constant.ApplicationName, metricsResource, "get"), WhenEnabled(ttMiddleware), metricsHandler.GetMetrics)

	// Doc Swagger
	f.Get("/swagger/*", WithSwaggerEnvConfig(), fiberSwagger.WrapHandler)

	// Health (liveness): gated on the startup self-probe state. Returns 503
	// until readyz.RunSelfProbe succeeds at bootstrap (Gate 7). When deps is
	// nil (legacy callers / partial-bootstrap tests) the handler defaults to
	// 200; production bootstrap always sets deps.SelfProbeState.
	var selfProbeState *readyz.SelfProbeState
	if deps != nil {
		selfProbeState = deps.SelfProbeState
	}

	f.Get("/health", NewManagerHealthHandler(selfProbeState))

	// Readiness: canonical /readyz contract. Gate 2 of ring:dev-readyz.
	// The legacy /ready alias is intentionally NOT registered — the contract
	// path is exactly /readyz.
	f.Get("/readyz", NewManagerReadyzHandler(deps))

	// Version
	f.Get("/version", commonsHttp.Version)

	f.Use(tlMid.EndTracingSpans)

	return f
}

func legacyFiberErrorHandler(c *fiber.Ctx, err error) error {
	ctx := c.UserContext()
	if ctx != nil {
		span := trace.SpanFromContext(ctx)
		opentelemetry.HandleSpanError(span, "handler error", err)
		span.End()
	}

	code := fiber.StatusInternalServerError

	var fiberErr *fiber.Error
	if errors.As(err, &fiberErr) {
		code = fiberErr.Code
	}

	if ctx == nil {
		ctx = context.Background()
	}

	if code == fiber.StatusInternalServerError {
		logger := libObservability.NewLoggerFromContext(ctx)
		logger.Log(ctx, log.LevelError,
			"handler error",
			log.String("method", c.Method()),
			log.String("path", c.Path()),
			log.Err(err),
		)

		return c.Status(code).JSON(fiber.Map{"error": nethttp.StatusText(code)})
	}

	return c.Status(code).JSON(fiber.Map{"error": err.Error()})
}

// WhenEnabled is a helper that conditionally applies a middleware if it's not nil.
func WhenEnabled(middleware fiber.Handler) fiber.Handler {
	return func(c *fiber.Ctx) error {
		if middleware == nil {
			return c.Next()
		}

		return middleware(c)
	}
}
