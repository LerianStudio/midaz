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

// NewRouter register NewRouter routes to the Server.
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

// registerTransactionRoutes registers all transaction-related routes.
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

// registerOperationRoutes registers all operation-related routes.
func registerOperationRoutes(r fiber.Router, auth *middleware.AuthClient, h *OperationHandler) {
	// Operations by account
	accountOps := r.Group("/organizations/:organization_id/ledgers/:ledger_id/accounts/:account_id/operations")
	accountOps.Get("", auth.Authorize(midazName, "operations", "get"), http.ParseUUIDPathParameters("operation"), h.GetAllOperationsByAccount)
	accountOps.Get("/:operation_id", auth.Authorize(midazName, "operations", "get"), http.ParseUUIDPathParameters("operation"), h.GetOperationByAccount)

	// Operations by transaction
	txnOps := r.Group("/organizations/:organization_id/ledgers/:ledger_id/transactions/:transaction_id/operations")
	txnOps.Patch("/:operation_id", auth.Authorize(midazName, "operations", "patch"), http.ParseUUIDPathParameters("operation"), http.WithBody(new(operation.UpdateOperationInput), h.UpdateOperation))
}

// registerAssetRateRoutes registers all asset-rate-related routes.
func registerAssetRateRoutes(r fiber.Router, auth *middleware.AuthClient, h *AssetRateHandler) {
	assetRates := r.Group("/organizations/:organization_id/ledgers/:ledger_id/asset-rates")
	assetRates.Put("", auth.Authorize(midazName, "asset-rates", "put"), http.ParseUUIDPathParameters("asset-rate"), http.WithBody(new(assetrate.CreateAssetRateInput), h.CreateOrUpdateAssetRate))
	assetRates.Get("/:external_id", auth.Authorize(midazName, "asset-rates", "get"), http.ParseUUIDPathParameters("asset-rate"), h.GetAssetRateByExternalID)
	assetRates.Get("/from/:asset_code", auth.Authorize(midazName, "asset-rates", "get"), http.ParseUUIDPathParameters("asset-rate"), h.GetAllAssetRatesByAssetCode)
}

// registerBalanceRoutes registers all balance-related routes.
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

// registerOperationRouteRoutes registers all operation-route-related routes.
func registerOperationRouteRoutes(r fiber.Router, auth *middleware.AuthClient, h *OperationRouteHandler) {
	opRoutes := r.Group("/organizations/:organization_id/ledgers/:ledger_id/operation-routes")
	opRoutes.Post("", auth.Authorize(routingName, "operation-routes", "post"), http.ParseUUIDPathParameters("operation_route"), http.WithBody(new(mmodel.CreateOperationRouteInput), h.CreateOperationRoute))
	opRoutes.Get("/:operation_route_id", auth.Authorize(routingName, "operation-routes", "get"), http.ParseUUIDPathParameters("operation_route"), h.GetOperationRouteByID)
	opRoutes.Patch("/:operation_route_id", auth.Authorize(routingName, "operation-routes", "patch"), http.ParseUUIDPathParameters("operation_route"), http.WithBody(new(mmodel.UpdateOperationRouteInput), h.UpdateOperationRoute))
	opRoutes.Delete("/:operation_route_id", auth.Authorize(routingName, "operation-routes", "delete"), http.ParseUUIDPathParameters("operation_route"), h.DeleteOperationRouteByID)
	opRoutes.Get("", auth.Authorize(routingName, "operation-routes", "get"), http.ParseUUIDPathParameters("operation_route"), h.GetAllOperationRoutes)
}

// registerTransactionRouteRoutes registers all transaction-route-related routes.
func registerTransactionRouteRoutes(r fiber.Router, auth *middleware.AuthClient, h *TransactionRouteHandler) {
	txnRoutes := r.Group("/organizations/:organization_id/ledgers/:ledger_id/transaction-routes")
	txnRoutes.Post("", auth.Authorize(routingName, "transaction-routes", "post"), http.ParseUUIDPathParameters("transaction_route"), http.WithBody(new(mmodel.CreateTransactionRouteInput), h.CreateTransactionRoute))
	txnRoutes.Get("/:transaction_route_id", auth.Authorize(routingName, "transaction-routes", "get"), http.ParseUUIDPathParameters("transaction_route"), h.GetTransactionRouteByID)
	txnRoutes.Patch("/:transaction_route_id", auth.Authorize(routingName, "transaction-routes", "patch"), http.ParseUUIDPathParameters("transaction_route"), http.WithBody(new(mmodel.UpdateTransactionRouteInput), h.UpdateTransactionRoute))
	txnRoutes.Delete("/:transaction_route_id", auth.Authorize(routingName, "transaction-routes", "delete"), http.ParseUUIDPathParameters("transaction_route"), h.DeleteTransactionRouteByID)
	txnRoutes.Get("", auth.Authorize(routingName, "transaction-routes", "get"), http.ParseUUIDPathParameters("transaction_route"), h.GetAllTransactionRoutes)
}

// securityHeadersMiddleware adds security headers to all responses.
// These headers protect against common web vulnerabilities.
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
