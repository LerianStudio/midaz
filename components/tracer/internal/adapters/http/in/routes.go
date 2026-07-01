// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"strings"

	openapi "github.com/LerianStudio/lib-commons/v5/commons/net/http/openapi"
	problem "github.com/LerianStudio/lib-commons/v5/commons/net/http/problem"
	tmcore "github.com/LerianStudio/lib-commons/v5/commons/tenant-manager/core"
	tmmiddleware "github.com/LerianStudio/lib-commons/v5/commons/tenant-manager/middleware"
	tmpostgres "github.com/LerianStudio/lib-commons/v5/commons/tenant-manager/postgres"
	libLog "github.com/LerianStudio/lib-observability/log"
	libObsMiddleware "github.com/LerianStudio/lib-observability/middleware"
	libOtel "github.com/LerianStudio/lib-observability/tracing"
	"github.com/danielgtaylor/huma/v2"
	"github.com/gofiber/adaptor/v2"
	"github.com/gofiber/contrib/otelfiber/v2"
	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
	"github.com/gofiber/fiber/v2/middleware/recover"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	fiberSwagger "github.com/swaggo/fiber-swagger"

	"github.com/LerianStudio/midaz/v4/components/tracer/internal/adapters/http/in/middleware"
	"github.com/LerianStudio/midaz/v4/components/tracer/internal/adapters/seamtenant"
	"github.com/LerianStudio/midaz/v4/components/tracer/internal/services/workers"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/clock"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	pkgHTTP "github.com/LerianStudio/midaz/v4/pkg/net/http"
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

	// TrustedProxyCIDRs is the boot-parsed set of reverse-proxy networks used by
	// the client-IP middleware to derive the audit client IP from
	// X-Forwarded-For. Empty (nil) means XFF is ignored and the socket peer IP
	// is recorded. Parsed once at boot in bootstrap; never re-parsed per request.
	TrustedProxyCIDRs []*net.IPNet

	// SwaggerEnabled gates the native Huma OpenAPI 3.1 spec + Scalar docs surface
	// (openapi.ServeSpec: /v1/openapi.{json,yaml}, /v1/docs). Independent of the
	// legacy swaggo /swagger/* mount, which is always on. Default false.
	SwaggerEnabled bool
}

// reservationPathPrefix is the full mounted prefix of the reservation surface
// (the /v1 group + the /reservations resource).
const reservationPathPrefix = "/v1/reservations"

// isReservationPath reports whether the request path targets the reservation
// service-to-service seam. Used to exempt those routes from the JWT-claim tenant
// middleware: the seam resolves its tenant from the trusted X-Tenant-Id header
// instead. Matches both the collection ("/v1/reservations") and its
// sub-resources ("/v1/reservations/...").
func isReservationPath(path string) bool {
	return path == reservationPathPrefix || strings.HasPrefix(path, reservationPathPrefix+"/")
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
		ErrorHandler:          pkgHTTP.CanonicalFiberErrorHandler,
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

	// 5. Client IP - Fifth: extract and inject client IP into context for audit trail.
	// XFF is honored only for hops behind the configured trusted proxies; with
	// none configured the client-controlled header is ignored and the socket
	// peer IP is recorded.
	f.Use(middleware.ClientIPMiddlewareWithTrustedProxies(cfg.TrustedProxyCIDRs))

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

		// The reservation surface is the service-to-service seam: the ledger
		// authenticates over mTLS (not a user JWT) and forwards a TRUSTED
		// X-Tenant-Id header. Those routes resolve their tenant via their own
		// reservationTenantMiddleware, so the JWT-claim path must NOT gate them
		// (it would 401 the seam for lacking a Bearer token). Skip the shared
		// middleware on /v1/reservations* and leave it intact for every other
		// /v1 user route.
		api.Use(func(c *fiber.Ctx) error {
			if isReservationPath(c.Path()) {
				return c.Next()
			}

			return tenantMW.WithTenantDB(c)
		})

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

	// Huma bootstrap (Phase 2a). problem.Install() overrides the process-global
	// huma.NewError to the org-wide RFC 9457 model; it MUST run before any
	// huma.Register (runtime + spec-gen) and is idempotent (sync.Once). The Huma
	// API binds to the SAME /v1 group `api` that carries the tenant middleware —
	// so the humafiber v2 adapter's ctx (built from c.UserContext()) reaches the
	// migrated handlers with the tenant/DB intact, no bridge needed.
	problem.Install()

	humaAPI := openapi.New(f, api, openapi.Config{
		Title:   "Midaz Tracer API",
		Version: os.Getenv("VERSION"),
		Servers: []string{"/v1"},
	})

	// Rename the shared problem.Detail error body to "Error" before any
	// huma.Register (the registry namer is captured on first registration). Same
	// shared namer the ledger uses, tracer variant (no plane-specific package
	// qualifications). See pkgHTTP.InstallSchemaNamer.
	pkgHTTP.InstallSchemaNamer(humaAPI)

	// Declare the security schemes referenced by per-op Security metadata so the
	// generated spec resolves them instead of dangling. SPEC metadata only —
	// runtime auth stays the Fiber guard.With middleware. BearerAuth comes from
	// the shared lib-commons helper (http bearer/JWT, idempotent, nil-safe).
	// ApiKeyAuth has no lib-commons helper, so declare it locally, mirroring the
	// helper's nil-guard on the SecuritySchemes map.
	openapi.DeclareBearerAuth(humaAPI)

	components := humaAPI.OpenAPI().Components
	if components.SecuritySchemes == nil {
		components.SecuritySchemes = map[string]*huma.SecurityScheme{}
	}

	components.SecuritySchemes["ApiKeyAuth"] = &huma.SecurityScheme{
		Type:        "apiKey",
		In:          "header",
		Name:        "X-API-Key",
		Description: "Static API key presented in the X-API-Key header.",
	}

	// Handlers whose constructors can fail are built here, in NewRoutes, so a bad
	// clock / wiring fails boot loudly rather than inside the route seam. The
	// error-free handlers are built inside registerTracerHumaRoutes from the
	// services carried on tracerHumaHandlers.
	validationHandler, err := NewValidationHandler(validationService, clk)
	if err != nil {
		return nil, fmt.Errorf("failed to create validation handler: %w", err)
	}

	// Reservation handler + its dedicated tenant middleware are wired ONLY when the
	// reservation service is present — the API is additive, so a build without it
	// simply does not expose /v1/reservations. resTenantMW needs pgManager +
	// multiTenantEnabled (production-only inputs), so it is built here and handed to
	// the seam; in single-tenant mode the resolver is a no-op. A nil reservation
	// handler tells the seam to skip the reservation routes entirely.
	var (
		reservationHandler *ReservationHandler
		resTenantMW        fiber.Handler
	)

	if reservationService != nil {
		reservationHandler, err = NewReservationHandler(reservationService, clk)
		if err != nil {
			return nil, fmt.Errorf("failed to create reservation handler: %w", err)
		}

		resTenantMW = reservationTenantMiddleware(seamtenant.NewResolver(pgManager, multiTenantEnabled))
	}

	// Single seam that mounts every Huma route (and its pre-Huma Fiber auth chain)
	// on the shared /v1 group + Huma API. Production (here) and the http/in tests
	// call the SAME function, so the registered surface is byte-for-byte identical
	// without a running server or DB. See registerTracerHumaRoutes.
	registerTracerHumaRoutes(api, humaAPI, tracerHumaHandlers{
		Guard:                 guard,
		APIKeyOnlyValidation:  cfg.APIKeyOnlyValidation,
		Rule:                  NewHandler(ruleService),
		Limit:                 NewLimitHandler(limitService),
		TransactionValidation: NewTransactionValidationHandler(transactionValidationService),
		Validation:            validationHandler,
		Reservation:           reservationHandler,
		ResTenantMW:           resTenantMW,
		AuditEvent:            NewAuditEventHandler(auditEventService),
	})

	// Native Huma OpenAPI 3.1 spec + Scalar docs, gated on SwaggerEnabled. Mounted
	// AFTER every huma.Register above so the snapshotted spec is complete. These
	// routes are off the auth/tenant chain (public-within-the-gate) and are
	// independent of the legacy swaggo /swagger/* mount. Never registered when the
	// flag is false.
	if cfg.SwaggerEnabled {
		openapi.ServeSpec(f, humaAPI, lg, "/v1", "Midaz Tracer API")
	}

	// End tracing spans middleware - skipped when telemetry is disabled
	if !skipTelemetry {
		f.Use(tlMid.EndTracingSpans)
	}

	return f, nil
}

// tracerHumaHandlers bundles everything registerTracerHumaRoutes needs to mount
// the tracer's Huma surface: the auth guard, the per-op config flag, and the
// already-constructed resource handlers. It is the tracer analogue of the
// ledger's humaMount closure inputs — a struct instead of captured locals so the
// http/in tests can hand in zero-value handlers (e.g. &Handler{}) and exercise
// the exact production registration path without a running server or DB.
//
// Zero-value semantics:
//   - Reservation: if nil, the /v1/reservations routes are not mounted (the API
//     is additive). ResTenantMW is only consulted when Reservation is non-nil.
//   - ResTenantMW: the reservation-scoped tenant Fiber middleware, built in
//     NewRoutes from pgManager+multiTenantEnabled. Tests may pass nil (the
//     reservation routes are skipped when Reservation is nil anyway).
type tracerHumaHandlers struct {
	Guard                 *middleware.AuthGuard
	APIKeyOnlyValidation  bool
	Rule                  *Handler
	Limit                 *LimitHandler
	TransactionValidation *TransactionValidationHandler
	Validation            *ValidationHandler
	Reservation           *ReservationHandler
	ResTenantMW           fiber.Handler
	AuditEvent            *AuditEventHandler
}

// registerTracerHumaRoutes mounts all 28 tracer Huma operations on the given
// Huma API, attaching each op's pre-Huma Fiber auth chain to the SAME /v1 group
// first. It is the single registration seam shared by production (NewRoutes) and
// the http/in tests, so the mounted surface is identical without a running
// server or DB — the tracer analogue of the ledger's humaMount closure in
// components/ledger/internal/bootstrap/config.go.
//
// Route/middleware ordering and every (resource, verb, forceAPIKey) tuple are
// preserved byte-for-byte from the pre-Huma inline routes; this func only moves
// the registration into a testable seam, it changes no auth, handler, or spec
// behavior.
func registerTracerHumaRoutes(api fiber.Router, humaAPI huma.API, h tracerHumaHandlers) {
	guard := h.Guard

	// Rule endpoints — ALL eight ops migrated to Huma (Phase 2b-1). Auth stays a
	// Fiber middleware attached to the exact method+path BEFORE the Huma
	// registration, so guard.With runs first and c.Next() advances into the Huma
	// handler — byte-identical auth behavior, no Huma per-op Security yet. The
	// (resource, verb, forceAPIKey) tuples are preserved verbatim from the
	// pre-Huma inline routes.
	//
	// INTENTIONAL DIVERGENCE (auth 401): guard.With is a pre-Huma Fiber
	// middleware, so a rejected request never reaches Huma. Its 401 is emitted by
	// pkgHTTP.Unauthorized as the legacy FLAT envelope {code,title,message}, NOT
	// the RFC 9457 problem+json that problem.Install() applies to Huma responses.
	// This is preserved for byte-for-byte parity; migrating 401 → problem+json is
	// a contract change out of scope for this migration wave.
	api.Post("/rules", guard.With("rules", "post", false))
	api.Get("/rules", guard.With("rules", "get", false))
	api.Get("/rules/:id", guard.With("rules", "get", false))
	api.Patch("/rules/:id", guard.With("rules", "patch", false))
	api.Delete("/rules/:id", guard.With("rules", "delete", false))
	api.Post("/rules/:id/activate", guard.With("rules", "post", false))
	api.Post("/rules/:id/deactivate", guard.With("rules", "post", false))
	api.Post("/rules/:id/draft", guard.With("rules", "post", false))
	RegisterRuleRoutes(humaAPI, h.Rule)

	// Limit endpoints — migrated to Huma (Phase 2b). Same pattern as rules above:
	// guard.With stays a Fiber middleware on the exact method+path (no terminal
	// handler) so it runs first, then c.Next() advances into the Huma-registered
	// handler. Fiber routes keep :id; Huma registers the same paths as {id}.
	// (resource, verb, forceAPIKey) tuples preserved verbatim.
	api.Post("/limits", guard.With("limits", "post", false))
	api.Get("/limits", guard.With("limits", "get", false))
	api.Get("/limits/:id", guard.With("limits", "get", false))
	api.Get("/limits/:id/usage", guard.With("limits", "get", false))
	api.Patch("/limits/:id", guard.With("limits", "patch", false))
	api.Delete("/limits/:id", guard.With("limits", "delete", false))
	api.Post("/limits/:id/activate", guard.With("limits", "post", false))
	api.Post("/limits/:id/deactivate", guard.With("limits", "post", false))
	api.Post("/limits/:id/draft", guard.With("limits", "post", false))
	RegisterLimitRoutes(humaAPI, h.Limit)

	// Transaction Validation endpoints (read-only per SOX/GLBA requirements) — Huma.
	api.Get("/validations", guard.With("validations", "get", false))
	api.Get("/validations/:id", guard.With("validations", "get", false))
	RegisterTransactionValidationRoutes(humaAPI, h.TransactionValidation)

	// Validation endpoint POST — Huma.
	// When APIKeyOnlyValidation=true, uses API key auth only (bypasses plugin auth).
	// The 3rd guard arg is APIKeyOnlyValidation (config-driven), NOT a literal.
	api.Post("/validations", guard.With("validations", "post", h.APIKeyOnlyValidation))
	RegisterValidationRoutes(humaAPI, h.Validation)

	// Reservation endpoints (two-phase capacity hold) — Huma. Mounted only when the
	// reservation handler is wired — the API is additive, so a build without it
	// simply does not expose /v1/reservations. The "reservations" resource is the
	// tracer's OWN authz resource string (API-key / Access-Manager guard), not a
	// ledger plugin namespace.
	if h.Reservation != nil {
		// Reservation-scoped tenant resolution: on the mTLS/mesh-verified seam
		// the ledger forwards a TRUSTED X-Tenant-Id header. resTenantMW (built in
		// NewRoutes) resolves the per-tenant PG pool from it here, on the
		// reservation routes ONLY — the shared JWT-claim tenant middleware on the
		// other /v1 user routes is left intact, and no header-trust path is opened
		// elsewhere. In single-tenant mode the resolver is a no-op.
		//
		// TWO Fiber middlewares per route (resTenantMW THEN guard.With), both
		// middleware-only: resTenantMW resolves the per-tenant DB, guard.With
		// authenticates, then c.Next() advances into the Huma handler. The
		// by-transaction routes are declared BEFORE the "/reservations/:id/..."
		// param routes so Fiber matches the static "transaction" segment first
		// (otherwise it binds the literal "transaction" to :id). Ordering and both
		// middlewares are preserved exactly from the pre-Huma inline routes.
		resTenantMW := h.ResTenantMW

		api.Post("/reservations", resTenantMW, guard.With("reservations", "post", false))
		api.Post("/reservations/transaction/:transaction_id/confirm", resTenantMW, guard.With("reservations", "post", false))
		api.Post("/reservations/transaction/:transaction_id/release", resTenantMW, guard.With("reservations", "post", false))
		api.Post("/reservations/:id/confirm", resTenantMW, guard.With("reservations", "post", false))
		api.Post("/reservations/:id/release", resTenantMW, guard.With("reservations", "post", false))
		RegisterReservationRoutes(humaAPI, h.Reservation)
	}

	// Audit Event endpoints (read-only per SOX/GLBA requirements) — Huma.
	api.Get("/audit-events", guard.With("audit-events", "get", false))
	api.Get("/audit-events/:id", guard.With("audit-events", "get", false))
	api.Get("/audit-events/:id/verify", guard.With("audit-events", "get", false))
	RegisterAuditEventRoutes(humaAPI, h.AuditEvent)
}
