// Package in provides HTTP inbound adapters for the transaction component.
//
// This package implements the HTTP transport layer for the transaction bounded context,
// exposing REST API endpoints for transaction processing, balance management, and
// routing configuration. It follows the hexagonal architecture pattern where HTTP
// handlers adapt external requests to internal use cases.
//
// Architecture Overview:
//
// The HTTP adapter layer provides:
//   - REST API endpoints for all transaction operations
//   - Request validation and parameter parsing
//   - Authentication and authorization middleware
//   - OpenTelemetry tracing integration
//   - CORS and security headers configuration
//   - Swagger documentation generation
//
// API Organization:
//
// Endpoints are organized by resource:
//   - /transactions: Create, read, update, commit, cancel, revert transactions
//   - /operations: Query and update individual operations
//   - /balances: Manage account balances
//   - /asset-rates: Currency exchange rate management
//   - /operation-routes: Operation routing rules
//   - /transaction-routes: Transaction routing configuration
//
// Security:
//
// All endpoints (except health/version) require authentication via the auth middleware.
// Authorization is enforced per resource and action (get, post, patch, delete).
//
// Related Packages:
//   - handlers: Request handlers implementing business logic delegation
//   - middleware: Auth client for JWT/API key validation
//   - versioning: API version management
package in

import (
	"os"
	"strings"

	"github.com/LerianStudio/lib-auth/v2/auth/middleware"
	libLog "github.com/LerianStudio/lib-commons/v2/commons/log"
	libHTTP "github.com/LerianStudio/lib-commons/v2/commons/net/http"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v2/commons/opentelemetry"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/postgres/assetrate"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/postgres/operation"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/postgres/transaction"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/LerianStudio/midaz/v3/pkg/net/http"
	"github.com/lerianstudio/monorepo/pkg/platform/versioning"
	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
	fiberSwagger "github.com/swaggo/fiber-swagger"
)

const midazName = "midaz"
const routingName = "routing"

// NewRouter creates and configures the HTTP router for the transaction component.
//
// This function sets up the complete HTTP infrastructure including:
//   - Fiber application with custom error handling
//   - OpenTelemetry middleware for distributed tracing
//   - CORS configuration for cross-origin requests
//   - Security headers middleware (HSTS, CSP, etc.)
//   - Structured logging middleware
//   - Versioned route registration
//   - Health and version endpoints
//   - Swagger documentation endpoint
//
// Router Configuration:
//
//	Step 1: Fiber App Initialization
//	  - Disable startup message for clean logs
//	  - Configure custom error handler for consistent responses
//
//	Step 2: Middleware Stack
//	  - Telemetry (tracing context propagation)
//	  - CORS (configurable allowed origins)
//	  - Security headers (HSTS, X-Frame-Options, CSP)
//	  - HTTP logging (structured logs)
//
//	Step 3: Route Registration
//	  - v1 routes: All transaction resources
//	  - Health endpoint: /health
//	  - Version endpoint: /version
//	  - Swagger endpoint: /swagger/*
//
// Parameters:
//   - lg: Structured logger for request logging
//   - tl: OpenTelemetry telemetry instance
//   - auth: Authentication client for JWT/API key validation
//   - th: Transaction handler
//   - oh: Operation handler
//   - ah: Asset rate handler
//   - bh: Balance handler
//   - orh: Operation route handler
//   - trh: Transaction route handler
//
// Returns:
//   - *fiber.App: Configured Fiber application ready to serve
//
// Environment Variables:
//   - ALLOWED_ORIGINS: Comma-separated list of allowed CORS origins
func NewRouter(lg libLog.Logger, tl *libOpentelemetry.Telemetry, auth *middleware.AuthClient, th *TransactionHandler, oh *OperationHandler, ah *AssetRateHandler, bh *BalanceHandler, orh *OperationRouteHandler, trh *TransactionRouteHandler) *fiber.App {
	f := fiber.New(fiber.Config{
		DisableStartupMessage: true,
		ErrorHandler: func(ctx *fiber.Ctx, err error) error {
			return libHTTP.HandleFiberError(ctx, err)
		},
	})

	tlMid := libHTTP.NewTelemetryMiddleware(tl)

	f.Use(tlMid.WithTelemetry(tl))

	// CORS configuration - restrict to allowed origins
	allowedOrigins := os.Getenv("ALLOWED_ORIGINS")
	if allowedOrigins == "" {
		// Default for development - should be overridden in production
		allowedOrigins = "http://localhost:3000,http://localhost:3001,http://localhost:8080"
	}

	f.Use(cors.New(cors.Config{
		AllowOrigins:     allowedOrigins,
		AllowMethods:     strings.Join([]string{fiber.MethodGet, fiber.MethodPost, fiber.MethodPut, fiber.MethodPatch, fiber.MethodDelete, fiber.MethodOptions, fiber.MethodHead}, ","),
		AllowHeaders:     "Origin,Content-Type,Accept,Authorization,X-Request-Id",
		AllowCredentials: true,
		MaxAge:           3600, // 1 hour
	}))

	// Security headers middleware
	f.Use(securityHeadersMiddleware)

	f.Use(libHTTP.WithHTTPLogging(libHTTP.WithCustomLogger(lg)))

	// Versioned route registration
	versioning.Register(f, "", map[string]func(fiber.Router){
		"v1": func(r fiber.Router) {
			registerTransactionRoutes(r, auth, th)
			registerOperationRoutes(r, auth, oh)
			registerAssetRateRoutes(r, auth, ah)
			registerBalanceRoutes(r, auth, bh)
			registerOperationRouteRoutes(r, auth, orh)
			registerTransactionRouteRoutes(r, auth, trh)
		},
	})

	// Health
	f.Get("/health", libHTTP.Ping)

	// Version
	f.Get("/version", libHTTP.Version)

	// Doc
	f.Get("/swagger/*", WithSwaggerEnvConfig(), fiberSwagger.WrapHandler)

	f.Use(tlMid.EndTracingSpans)

	return f
}

// registerTransactionRoutes registers all transaction-related HTTP routes.
//
// Transaction endpoints follow RESTful conventions with additional action endpoints
// for lifecycle operations (commit, cancel, revert).
//
// Endpoints:
//   - POST /transactions/dsl: Create transaction from DSL format
//   - POST /transactions/json: Create transaction from JSON format
//   - POST /transactions/inflow: Create inflow transaction (credit only)
//   - POST /transactions/outflow: Create outflow transaction (debit only)
//   - POST /transactions/annotation: Create annotation transaction
//   - POST /transactions/:id/commit: Commit pending transaction
//   - POST /transactions/:id/cancel: Cancel pending transaction
//   - POST /transactions/:id/revert: Revert completed transaction
//   - PATCH /transactions/:id: Update transaction metadata
//   - GET /transactions/:id: Get transaction by ID
//   - GET /transactions: List all transactions with pagination
func registerTransactionRoutes(r fiber.Router, auth *middleware.AuthClient, h *TransactionHandler) {
	txns := r.Group("/organizations/:organization_id/ledgers/:ledger_id/transactions")

	// Transaction creation endpoints
	txns.Post("/dsl", auth.Authorize(midazName, "transactions", "post"), http.ParseUUIDPathParameters("transaction"), h.CreateTransactionDSL)
	txns.Post("/json", auth.Authorize(midazName, "transactions", "post"), http.ParseUUIDPathParameters("transaction"), http.WithBody(new(transaction.CreateTransactionInput), h.CreateTransactionJSON))
	txns.Post("/inflow", auth.Authorize(midazName, "transactions", "post"), http.ParseUUIDPathParameters("transaction"), http.WithBody(new(transaction.CreateTransactionInflowInput), h.CreateTransactionInflow))
	txns.Post("/outflow", auth.Authorize(midazName, "transactions", "post"), http.ParseUUIDPathParameters("transaction"), http.WithBody(new(transaction.CreateTransactionOutflowInput), h.CreateTransactionOutflow))
	txns.Post("/annotation", auth.Authorize(midazName, "transactions", "post"), http.ParseUUIDPathParameters("transaction"), http.WithBody(new(transaction.CreateTransactionInput), h.CreateTransactionAnnotation))

	// Transaction lifecycle endpoints
	txns.Post("/:transaction_id/commit", auth.Authorize(midazName, "transactions", "post"), http.ParseUUIDPathParameters("transaction"), h.CommitTransaction)
	txns.Post("/:transaction_id/cancel", auth.Authorize(midazName, "transactions", "post"), http.ParseUUIDPathParameters("transaction"), h.CancelTransaction)
	txns.Post("/:transaction_id/revert", auth.Authorize(midazName, "transactions", "post"), http.ParseUUIDPathParameters("transaction"), h.RevertTransaction)

	// Transaction CRUD endpoints
	txns.Patch("/:transaction_id", auth.Authorize(midazName, "transactions", "patch"), http.ParseUUIDPathParameters("transaction"), http.WithBody(new(transaction.UpdateTransactionInput), h.UpdateTransaction))
	txns.Get("/:transaction_id", auth.Authorize(midazName, "transactions", "get"), http.ParseUUIDPathParameters("transaction"), h.GetTransaction)
	txns.Get("", auth.Authorize(midazName, "transactions", "get"), http.ParseUUIDPathParameters("transaction"), h.GetAllTransactions)
}

// registerOperationRoutes registers all operation-related HTTP routes.
//
// Operations are accessed via their parent account or transaction for proper
// scoping and authorization.
//
// Endpoints:
//   - GET /accounts/:id/operations: List operations by account
//   - GET /accounts/:id/operations/:op_id: Get operation by account
//   - PATCH /transactions/:id/operations/:op_id: Update operation metadata
func registerOperationRoutes(r fiber.Router, auth *middleware.AuthClient, h *OperationHandler) {
	// Operations by account
	accountOps := r.Group("/organizations/:organization_id/ledgers/:ledger_id/accounts/:account_id/operations")
	accountOps.Get("", auth.Authorize(midazName, "operations", "get"), http.ParseUUIDPathParameters("operation"), h.GetAllOperationsByAccount)
	accountOps.Get("/:operation_id", auth.Authorize(midazName, "operations", "get"), http.ParseUUIDPathParameters("operation"), h.GetOperationByAccount)

	// Operations by transaction
	txnOps := r.Group("/organizations/:organization_id/ledgers/:ledger_id/transactions/:transaction_id/operations")
	txnOps.Patch("/:operation_id", auth.Authorize(midazName, "operations", "patch"), http.ParseUUIDPathParameters("operation"), http.WithBody(new(operation.UpdateOperationInput), h.UpdateOperation))
}

// registerAssetRateRoutes registers all asset-rate-related HTTP routes.
//
// Asset rates define currency conversion rates for multi-currency transactions.
//
// Endpoints:
//   - PUT /asset-rates: Create or update asset rate (upsert)
//   - GET /asset-rates/:external_id: Get rate by external ID
//   - GET /asset-rates/from/:asset_code: List rates from a source asset
func registerAssetRateRoutes(r fiber.Router, auth *middleware.AuthClient, h *AssetRateHandler) {
	assetRates := r.Group("/organizations/:organization_id/ledgers/:ledger_id/asset-rates")
	assetRates.Put("", auth.Authorize(midazName, "asset-rates", "put"), http.ParseUUIDPathParameters("asset-rate"), http.WithBody(new(assetrate.CreateAssetRateInput), h.CreateOrUpdateAssetRate))
	assetRates.Get("/:external_id", auth.Authorize(midazName, "asset-rates", "get"), http.ParseUUIDPathParameters("asset-rate"), h.GetAssetRateByExternalID)
	assetRates.Get("/from/:asset_code", auth.Authorize(midazName, "asset-rates", "get"), http.ParseUUIDPathParameters("asset-rate"), h.GetAllAssetRatesByAssetCode)
}

// registerBalanceRoutes registers all balance-related HTTP routes.
//
// Balances can be accessed via ledger scope (all balances) or account scope
// (balances for specific account).
//
// Endpoints:
//   - GET /balances: List all balances in ledger
//   - GET /balances/:id: Get balance by ID
//   - PATCH /balances/:id: Update balance
//   - DELETE /balances/:id: Delete balance
//   - GET /accounts/:id/balances: List balances by account
//   - POST /accounts/:id/balances: Create additional balance
//   - GET /accounts/alias/:alias/balances: Get balances by alias
//   - GET /accounts/external/:code/balances: Get balances by external code
func registerBalanceRoutes(r fiber.Router, auth *middleware.AuthClient, h *BalanceHandler) {
	// Balances by ledger
	ledgerBalances := r.Group("/organizations/:organization_id/ledgers/:ledger_id/balances")
	ledgerBalances.Patch("/:balance_id", auth.Authorize(midazName, "balances", "patch"), http.ParseUUIDPathParameters("balance"), http.WithBody(new(mmodel.UpdateBalance), h.UpdateBalance))
	ledgerBalances.Delete("/:balance_id", auth.Authorize(midazName, "balances", "delete"), http.ParseUUIDPathParameters("balance"), h.DeleteBalanceByID)
	ledgerBalances.Get("", auth.Authorize(midazName, "balances", "get"), http.ParseUUIDPathParameters("balance"), h.GetAllBalances)
	ledgerBalances.Get("/:balance_id", auth.Authorize(midazName, "balances", "get"), http.ParseUUIDPathParameters("balance"), h.GetBalanceByID)

	// Balances by account
	accountBalances := r.Group("/organizations/:organization_id/ledgers/:ledger_id/accounts/:account_id/balances")
	accountBalances.Get("", auth.Authorize(midazName, "balances", "get"), http.ParseUUIDPathParameters("balance"), h.GetAllBalancesByAccountID)
	accountBalances.Post("", auth.Authorize(midazName, "balances", "post"), http.ParseUUIDPathParameters("balance"), http.WithBody(new(mmodel.CreateAdditionalBalance), h.CreateAdditionalBalance))

	// Balances by alias
	aliasBalances := r.Group("/organizations/:organization_id/ledgers/:ledger_id/accounts/alias/:alias/balances")
	aliasBalances.Get("", auth.Authorize(midazName, "balances", "get"), http.ParseUUIDPathParameters("balance"), h.GetBalancesByAlias)

	// Balances by external code
	externalBalances := r.Group("/organizations/:organization_id/ledgers/:ledger_id/accounts/external/:code/balances")
	externalBalances.Get("", auth.Authorize(midazName, "balances", "get"), http.ParseUUIDPathParameters("balance"), h.GetBalancesExternalByCode)
}

// registerOperationRouteRoutes registers all operation-route-related HTTP routes.
//
// Operation routes define validation rules for individual debit/credit operations.
//
// Endpoints:
//   - POST /operation-routes: Create operation route
//   - GET /operation-routes: List operation routes
//   - GET /operation-routes/:id: Get operation route by ID
//   - PATCH /operation-routes/:id: Update operation route
//   - DELETE /operation-routes/:id: Delete operation route
func registerOperationRouteRoutes(r fiber.Router, auth *middleware.AuthClient, h *OperationRouteHandler) {
	opRoutes := r.Group("/organizations/:organization_id/ledgers/:ledger_id/operation-routes")
	opRoutes.Post("", auth.Authorize(routingName, "operation-routes", "post"), http.ParseUUIDPathParameters("operation_route"), http.WithBody(new(mmodel.CreateOperationRouteInput), h.CreateOperationRoute))
	opRoutes.Get("/:operation_route_id", auth.Authorize(routingName, "operation-routes", "get"), http.ParseUUIDPathParameters("operation_route"), h.GetOperationRouteByID)
	opRoutes.Patch("/:operation_route_id", auth.Authorize(routingName, "operation-routes", "patch"), http.ParseUUIDPathParameters("operation_route"), http.WithBody(new(mmodel.UpdateOperationRouteInput), h.UpdateOperationRoute))
	opRoutes.Delete("/:operation_route_id", auth.Authorize(routingName, "operation-routes", "delete"), http.ParseUUIDPathParameters("operation_route"), h.DeleteOperationRouteByID)
	opRoutes.Get("", auth.Authorize(routingName, "operation-routes", "get"), http.ParseUUIDPathParameters("operation_route"), h.GetAllOperationRoutes)
}

// registerTransactionRouteRoutes registers all transaction-route-related HTTP routes.
//
// Transaction routes group operation routes into complete routing configurations.
//
// Endpoints:
//   - POST /transaction-routes: Create transaction route
//   - GET /transaction-routes: List transaction routes
//   - GET /transaction-routes/:id: Get transaction route by ID
//   - PATCH /transaction-routes/:id: Update transaction route
//   - DELETE /transaction-routes/:id: Delete transaction route
func registerTransactionRouteRoutes(r fiber.Router, auth *middleware.AuthClient, h *TransactionRouteHandler) {
	txnRoutes := r.Group("/organizations/:organization_id/ledgers/:ledger_id/transaction-routes")
	txnRoutes.Post("", auth.Authorize(routingName, "transaction-routes", "post"), http.ParseUUIDPathParameters("transaction_route"), http.WithBody(new(mmodel.CreateTransactionRouteInput), h.CreateTransactionRoute))
	txnRoutes.Get("/:transaction_route_id", auth.Authorize(routingName, "transaction-routes", "get"), http.ParseUUIDPathParameters("transaction_route"), h.GetTransactionRouteByID)
	txnRoutes.Patch("/:transaction_route_id", auth.Authorize(routingName, "transaction-routes", "patch"), http.ParseUUIDPathParameters("transaction_route"), http.WithBody(new(mmodel.UpdateTransactionRouteInput), h.UpdateTransactionRoute))
	txnRoutes.Delete("/:transaction_route_id", auth.Authorize(routingName, "transaction-routes", "delete"), http.ParseUUIDPathParameters("transaction_route"), h.DeleteTransactionRouteByID)
	txnRoutes.Get("", auth.Authorize(routingName, "transaction-routes", "get"), http.ParseUUIDPathParameters("transaction_route"), h.GetAllTransactionRoutes)
}

// securityHeadersMiddleware adds security headers to all HTTP responses.
//
// This middleware implements defense-in-depth by adding headers that protect
// against common web vulnerabilities:
//
// Headers Applied:
//   - Strict-Transport-Security: Force HTTPS for 1 year
//   - X-Frame-Options: DENY (prevent clickjacking)
//   - X-Content-Type-Options: nosniff (prevent MIME sniffing)
//   - X-XSS-Protection: 1; mode=block (legacy XSS protection)
//   - Referrer-Policy: strict-origin-when-cross-origin
//   - Content-Security-Policy: default-src 'self'; frame-ancestors 'none'
//
// Security Rationale:
//
// These headers provide defense against:
//   - Clickjacking attacks (X-Frame-Options, CSP frame-ancestors)
//   - MIME type confusion (X-Content-Type-Options)
//   - XSS attacks (X-XSS-Protection, CSP)
//   - Protocol downgrade (HSTS)
//   - Information leakage (Referrer-Policy)
func securityHeadersMiddleware(c *fiber.Ctx) error {
	// HSTS - Force HTTPS for 1 year (31536000 seconds)
	c.Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains")

	// Prevent clickjacking attacks
	c.Set("X-Frame-Options", "DENY")

	// Prevent MIME type sniffing
	c.Set("X-Content-Type-Options", "nosniff")

	// XSS Protection for legacy browsers
	c.Set("X-XSS-Protection", "1; mode=block")

	// Control referrer information
	c.Set("Referrer-Policy", "strict-origin-when-cross-origin")

	// Content Security Policy - restrict resource loading
	c.Set("Content-Security-Policy", "default-src 'self'; frame-ancestors 'none'")

	return c.Next()
}
