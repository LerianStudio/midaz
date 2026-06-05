// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"context"
	"errors"
	"fmt"
	"os"

	libHTTP "github.com/LerianStudio/lib-commons/v5/commons/net/http"
	tmcore "github.com/LerianStudio/lib-commons/v5/commons/tenant-manager/core"
	tmmiddleware "github.com/LerianStudio/lib-commons/v5/commons/tenant-manager/middleware"
	tmpostgres "github.com/LerianStudio/lib-commons/v5/commons/tenant-manager/postgres"
	libLog "github.com/LerianStudio/lib-observability/log"
	libObsMiddleware "github.com/LerianStudio/lib-observability/middleware"
	libOtel "github.com/LerianStudio/lib-observability/tracing"
	"github.com/gofiber/adaptor/v2"
	"github.com/gofiber/contrib/otelfiber/v2"
	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
	"github.com/gofiber/fiber/v2/middleware/recover"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	fiberSwagger "github.com/swaggo/fiber-swagger"

	"github.com/LerianStudio/midaz/v4/components/tracer/internal/adapters/http/in/middleware"
	"github.com/LerianStudio/midaz/v4/components/tracer/internal/services/workers"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/clock"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/constant"
	pkgHTTP "github.com/LerianStudio/midaz/v4/components/tracer/pkg/net/http"
)

// writeTenantCapReached emits the canonical {code,title,message} envelope on
// HTTP 503 when WorkerSupervisor.EnsureWorkers returns ErrTenantCapReached.
// Extracted from the inline middleware so the response shape can be unit
// tested directly (and so we never bypass libCommons.Response again — the
// previous inline `c.JSON(fiber.Map{...})` advertised a code that was not in
// pkg/constant/errors.go and skipped the envelope).
//
// Caller is responsible for setting the Retry-After header before invoking
// this helper; the helper only writes status + body.
func writeTenantCapReached(c *fiber.Ctx) error {
	return pkgHTTP.ServiceUnavailable(
		c,
		constant.ErrTenantCapReached.Error(),
		"Tenant Capacity Reached",
		"Tenant capacity reached; please retry shortly",
	)
}

// WorkerEnsurer is the narrow hook the tenant middleware uses to spawn
// per-tenant workers on the first request for a tenant ID. In production this
// is satisfied by *workers.WorkerSupervisor; keeping the interface local here
// avoids pulling the workers package into the http/in package (and lets tests
// swap in a recorder).
type WorkerEnsurer interface {
	EnsureWorkers(ctx context.Context, tenantID string) error
}

// defaultCORSOrigins is the restrictive default when CORS_ALLOWED_ORIGINS is not set.
// This prevents accidental exposure in production. Operators must explicitly configure origins.
const defaultCORSOrigins = ""

// getCORSAllowedOrigins returns the configured CORS origins or empty string (restrictive).
// Operators should set CORS_ALLOWED_ORIGINS explicitly:
// - Production: "https://app.example.com,https://admin.example.com"
// - Development: "*" (only if explicitly needed)
func getCORSAllowedOrigins(configured string) string {
	if configured == "" {
		return defaultCORSOrigins
	}

	return configured
}

// RouteConfig holds non-auth configuration for route setup.
// Auth configuration lives in middleware.AuthGuardConfig.
type RouteConfig struct {
	// CORSAllowedOrigins is a comma-separated list of allowed origins.
	// If empty, defaults to restrictive behavior (no wildcard in production).
	// Set to "*" explicitly for development environments only.
	CORSAllowedOrigins string

	// APIKeyOnlyValidation enables API-key-only auth for the validation endpoint.
	// When true AND PluginAuthEnabled=true (dual mode):
	// - Routes registered with guard.With(..., true) use API key auth only, bypassing plugin auth.
	// - Routes registered with guard.With(..., false) use plugin auth exclusively (no fallback).
	// When PluginAuthEnabled=false, all routes use API key auth regardless of this flag.
	APIKeyOnlyValidation bool
}

// skipTelemetryPaths returns true for paths that should skip detailed telemetry.
// Health/readiness probes generate high-frequency, low-value spans.
func skipTelemetryPaths(c *fiber.Ctx) bool {
	switch c.Path() {
	case "/health", "/readyz", "/metrics":
		return true
	default:
		return false
	}
}

// RoutesDeps bundles every dependency NewRoutes needs. Using a struct keeps
// the call site readable (13+ positional parameters became unmanageable as the
// multi-tenant branch grew) and makes adding future dependencies — e.g. a
// rate-limit middleware or a feature-flag backend — non-breaking for existing
// test fixtures.
//
// Zero-value semantics:
//   - Cfg: if nil, an empty &RouteConfig{} is used (no CORS, no auth hints).
//   - Clk: if nil, NewRoutes rejects with an error — every handler depends on
//     a live clock, so falling back to clock.New() would mask misconfiguration.
//   - MultiTenantEnabled + PgManager + Supervisor are a tri-state per the
//     guard inside NewRoutes (`MultiTenantEnabled && PgManager != nil`). All
//     three may be nil in single-tenant mode.
//   - ReservationService: if nil, the /v1/reservations routes are not mounted.
//     The two-phase reservation API is additive; a build that has not wired the
//     reservation service simply does not expose it.
type RoutesDeps struct {
	Logger                       libLog.Logger
	Telemetry                    *libOtel.Telemetry
	HealthChecker                *HealthChecker
	Cfg                          *RouteConfig
	RuleService                  RuleService
	LimitService                 LimitService
	ValidationService            ValidationService
	ReservationService           ReservationService
	TransactionValidationService TransactionValidationService
	AuditEventService            AuditEventService
	Guard                        *middleware.AuthGuard
	Clock                        clock.Clock
	MultiTenantEnabled           bool
	PgManager                    *tmpostgres.Manager
	Supervisor                   WorkerEnsurer
}

// NewRoutes builds the Fiber app with every middleware and route wired. The
// struct-based parameter bundle (RoutesDeps) is the single entry point — all
// callers (bootstrap + tests) hand in a fully-populated struct rather than
// positional args. Fields left at their zero value follow the documented
// zero-value semantics on RoutesDeps.
func NewRoutes(deps RoutesDeps) (*fiber.App, error) {
	cfg := deps.Cfg
	if cfg == nil {
		cfg = &RouteConfig{}
	}

	lg := deps.Logger
	tl := deps.Telemetry
	hc := deps.HealthChecker
	ruleService := deps.RuleService
	limitService := deps.LimitService
	validationService := deps.ValidationService
	reservationService := deps.ReservationService
	transactionValidationService := deps.TransactionValidationService
	auditEventService := deps.AuditEventService
	guard := deps.Guard
	clk := deps.Clock
	multiTenantEnabled := deps.MultiTenantEnabled
	pgManager := deps.PgManager
	supervisor := deps.Supervisor

	f := fiber.New(fiber.Config{
		DisableStartupMessage: true,
		ErrorHandler: func(ctx *fiber.Ctx, err error) error {
			return libHTTP.FiberErrorHandler(ctx, err)
		},
	})
	// Check if telemetry should be skipped to avoid data race in lib-commons ContextWithLogger.
	// The race occurs when multiple goroutines call WithTelemetry concurrently in tests.
	skipTelemetry := os.Getenv("SKIP_LIB_COMMONS_TELEMETRY") == "true"

	tlMid := libObsMiddleware.NewTelemetryMiddleware(tl)

	// Middleware order is CRITICAL per Ring Standards:
	// 1. WithTelemetry - First: injects tracer/logger into context
	// Skipped when SKIP_LIB_COMMONS_TELEMETRY=true to avoid data race in lib-commons ContextWithLogger.
	if !skipTelemetry {
		f.Use(tlMid.WithTelemetry(tl))
	}

	// 2. Recover - Second: captures panics before they propagate
	// Stack trace disabled in production to prevent information leakage (OWASP).
	// Panics are still logged via telemetry for debugging.
	f.Use(recover.New(recover.Config{
		EnableStackTrace: false,
	}))

	// 3. CORS - Third: handles preflight before auth
	f.Use(cors.New(cors.Config{
		AllowOrigins:     getCORSAllowedOrigins(cfg.CORSAllowedOrigins),
		AllowMethods:     "GET,POST,PUT,PATCH,DELETE,OPTIONS",
		AllowHeaders:     "Origin,Content-Type,Accept,Authorization,X-Request-ID,X-API-Key",
		AllowCredentials: false,
		MaxAge:           3600,
	}))

	// 4. OTel Fiber - Fourth: HTTP metrics and request tracing
	f.Use(otelfiber.Middleware(
		otelfiber.WithNext(skipTelemetryPaths),
	))

	// 5. Client IP - Fifth: extract and inject client IP into context for audit trail
	f.Use(middleware.ClientIPMiddleware())

	// 6. HTTP Logging - Sixth: structured request/response logging
	// Skipped when SKIP_LIB_COMMONS_TELEMETRY=true to avoid data race in lib-commons ContextWithLogger.
	if !skipTelemetry {
		f.Use(libObsMiddleware.WithHTTPLogging(libObsMiddleware.WithCustomLogger(lg)))
	}

	// 7. Fault Injection - Seventh: ONLY for integration tests
	// Enabled via FAULT_INJECTION_ENABLED=true environment variable.
	// NEVER enable in production - allows simulating 504/503 errors.
	f.Use(middleware.FaultInjection())

	// Public endpoints (no auth required). /readyz is the Lerian-canonical
	// readiness probe — registered BEFORE the /v1 group so the AuthGuard
	// never sees it (K8s probes are unauthenticated; a 401 here would be
	// interpreted by the kubelet as "not ready" and kill the pod).
	//
	// Public endpoints: /health, /readyz, /metrics, /version, /swagger/*
	f.Get("/health", hc.LivenessHandler())
	f.Get("/readyz", hc.ReadyzHandler())
	// /metrics MUST be mounted BEFORE the /v1 group so Prometheus scrapes
	// (typically unauthenticated, mesh-internal) are not blocked by AuthGuard.
	f.Get("/metrics", adaptor.HTTPHandler(promhttp.Handler()))
	f.Get("/version", Version)

	// Doc Swagger
	f.Get("/swagger/*", WithSwaggerEnvConfig(), fiberSwagger.WrapHandler)

	// Protected API group (uses /v1/ prefix per API Design v1.3.0)
	// Auth is handled per-endpoint by AuthGuard based on configuration flags.
	api := f.Group("/v1")

	// Multi-tenant middleware (single-module shape): extracts `tenantId` from
	// the JWT bearer token, resolves a tenant-specific *sql.DB via pgManager,
	// and stashes both into the request context. Repos pick them up through
	// tmcore.GetPGContext(ctx) / tmcore.GetTenantIDContext(ctx).
	//
	// Registered ONLY on /v1, keeping /health, /readyz, /version, /swagger/*
	// callable without a token (public endpoints).
	if multiTenantEnabled && pgManager != nil {
		tenantMW := tmmiddleware.NewTenantMiddleware(
			tmmiddleware.WithPG(pgManager),
		)
		api.Use(tenantMW.WithTenantDB)

		// Second middleware: lazy-spawn per-tenant workers on the first request
		// that surfaces a tenant. Covers pod restarts where the Pub/Sub
		// tenant-created event was missed, and new-tenant sign-ups that arrive
		// before the listener has delivered the add event.
		//
		// M3: log EnsureWorkers failures at Warn so silent degradation is
		// visible to operators. The request proceeds — background sync may be
		// unavailable for this tenant but the validation path can still serve
		// from the DB directly.
		//
		// M18: when the supervisor declines because MaxTenants is reached,
		// surface 503 + Retry-After to the client. Cap events must be visible,
		// not swallowed.
		if supervisor != nil {
			api.Use(func(c *fiber.Ctx) error {
				tid := tmcore.GetTenantIDContext(c.UserContext())
				if tid == "" {
					return c.Next()
				}

				if err := supervisor.EnsureWorkers(c.UserContext(), tid); err != nil {
					if errors.Is(err, workers.ErrTenantCapReached) {
						lg.With(
							libLog.String("operation", "routes.lazy_spawn_workers"),
							libLog.String("tenant_id", tid),
							libLog.String("error.message", err.Error()),
						).Log(c.UserContext(), libLog.LevelWarn,
							"Tenant worker cap reached; responding 503 so client backs off")

						c.Set("Retry-After", tenantCapRetryAfterHeader())

						return writeTenantCapReached(c)
					}

					lg.With(
						libLog.String("operation", "routes.lazy_spawn_workers"),
						libLog.String("tenant_id", tid),
						libLog.String("error.message", err.Error()),
					).Log(c.UserContext(), libLog.LevelWarn,
						"Failed to ensure workers for tenant; request will proceed but background sync may be unavailable")
				}

				return c.Next()
			})
		}
	}

	// Rule endpoints
	ruleHandler := NewHandler(ruleService)
	api.Post("/rules", guard.With("rules", "post", false), ruleHandler.CreateRule)
	api.Get("/rules", guard.With("rules", "get", false), ruleHandler.ListRules)
	api.Get("/rules/:id", guard.With("rules", "get", false), ruleHandler.GetRule)
	api.Patch("/rules/:id", guard.With("rules", "patch", false), ruleHandler.UpdateRule)
	api.Delete("/rules/:id", guard.With("rules", "delete", false), ruleHandler.DeleteRule)
	api.Post("/rules/:id/activate", guard.With("rules", "post", false), ruleHandler.ActivateRule)
	api.Post("/rules/:id/deactivate", guard.With("rules", "post", false), ruleHandler.DeactivateRule)
	api.Post("/rules/:id/draft", guard.With("rules", "post", false), ruleHandler.DraftRule)

	// Limit endpoints
	limitHandler := NewLimitHandler(limitService)
	api.Post("/limits", guard.With("limits", "post", false), limitHandler.CreateLimit)
	api.Get("/limits", guard.With("limits", "get", false), limitHandler.ListLimits)
	api.Get("/limits/:id", guard.With("limits", "get", false), limitHandler.GetLimit)
	api.Get("/limits/:id/usage", guard.With("limits", "get", false), limitHandler.GetLimitUsage)
	api.Patch("/limits/:id", guard.With("limits", "patch", false), limitHandler.UpdateLimit)
	api.Delete("/limits/:id", guard.With("limits", "delete", false), limitHandler.DeleteLimit)
	api.Post("/limits/:id/activate", guard.With("limits", "post", false), limitHandler.ActivateLimit)
	api.Post("/limits/:id/deactivate", guard.With("limits", "post", false), limitHandler.DeactivateLimit)
	api.Post("/limits/:id/draft", guard.With("limits", "post", false), limitHandler.DraftLimit)

	// Transaction Validation endpoints (read-only per SOX/GLBA requirements)
	transactionValidationHandler := NewTransactionValidationHandler(transactionValidationService)
	api.Get("/validations", guard.With("validations", "get", false), transactionValidationHandler.ListTransactionValidations)
	api.Get("/validations/:id", guard.With("validations", "get", false), transactionValidationHandler.GetTransactionValidation)

	// Validation endpoint POST
	// When APIKeyOnlyValidation=true, uses API key auth only (bypasses plugin auth)
	validationHandler, err := NewValidationHandler(validationService, clk)
	if err != nil {
		return nil, fmt.Errorf("failed to create validation handler: %w", err)
	}

	api.Post("/validations", guard.With("validations", "post", cfg.APIKeyOnlyValidation), validationHandler.Validate)

	// Reservation endpoints (two-phase capacity hold). Mounted only when the
	// reservation service is wired — the API is additive, so a build without it
	// simply does not expose /v1/reservations. The "reservations" resource is the
	// tracer's OWN authz resource string (API-key / Access-Manager guard), not a
	// ledger plugin namespace.
	if reservationService != nil {
		reservationHandler, err := NewReservationHandler(reservationService, clk)
		if err != nil {
			return nil, fmt.Errorf("failed to create reservation handler: %w", err)
		}

		api.Post("/reservations", guard.With("reservations", "post", false), reservationHandler.Reserve)
		// By-transaction transitions FIRST: the static "transaction" segment must
		// be matched before the "/reservations/:id/..." param routes, or Fiber binds
		// the literal "transaction" to :id. The ledger /commit and /cancel address
		// reservations by transaction id (the per-reservation handle does not survive
		// the separate state-transition request), so these are the lifecycle drivers
		// for PENDING transactions.
		api.Post("/reservations/transaction/:transaction_id/confirm", guard.With("reservations", "post", false), reservationHandler.ConfirmByTransaction)
		api.Post("/reservations/transaction/:transaction_id/release", guard.With("reservations", "post", false), reservationHandler.ReleaseByTransaction)
		api.Post("/reservations/:id/confirm", guard.With("reservations", "post", false), reservationHandler.Confirm)
		api.Post("/reservations/:id/release", guard.With("reservations", "post", false), reservationHandler.Release)
	}

	// Audit Event endpoints (read-only per SOX/GLBA requirements)
	auditEventHandler := NewAuditEventHandler(auditEventService)
	api.Get("/audit-events", guard.With("audit-events", "get", false), auditEventHandler.ListAuditEvents)
	api.Get("/audit-events/:id", guard.With("audit-events", "get", false), auditEventHandler.GetAuditEvent)
	api.Get("/audit-events/:id/verify", guard.With("audit-events", "get", false), auditEventHandler.VerifyHashChain)

	// End tracing spans middleware - skipped when telemetry is disabled
	if !skipTelemetry {
		f.Use(tlMid.EndTracingSpans)
	}

	return f, nil
}
