package in

import (
	"github.com/LerianStudio/lib-auth/v2/auth/middleware"
	libLog "github.com/LerianStudio/lib-commons/v2/commons/log"
	libHTTP "github.com/LerianStudio/lib-commons/v2/commons/net/http"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v2/commons/opentelemetry"
	poolmanager "github.com/LerianStudio/lib-commons/v2/commons/pool-manager"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/postgres/assetrate"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/postgres/operation"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/postgres/transaction"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/LerianStudio/midaz/v3/pkg/net/http"
	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
	fiberSwagger "github.com/swaggo/fiber-swagger"
)

const midazName = "midaz"
const routingName = "routing"

// NewRouter register NewRouter routes to the Server.
func NewRouter(lg libLog.Logger, tl *libOpentelemetry.Telemetry, auth *middleware.AuthClient, tenantsMiddleware poolmanager.Middleware, th *TransactionHandler, oh *OperationHandler, ah *AssetRateHandler, bh *BalanceHandler, orh *OperationRouteHandler, trh *TransactionRouteHandler, adminHandler *AdminHandler) *fiber.App {
	f := fiber.New(fiber.Config{
		DisableStartupMessage: true,
		ErrorHandler: func(ctx *fiber.Ctx, err error) error {
			return libHTTP.HandleFiberError(ctx, err)
		},
	})

	tlMid := libHTTP.NewTelemetryMiddleware(tl)

	f.Use(tlMid.WithTelemetry(tl))
	f.Use(cors.New())
	f.Use(libHTTP.WithHTTPLogging(libHTTP.WithCustomLogger(lg)))

	// Add multi-tenant middleware after auth middleware but before routes
	// The middleware extracts tenant ID from JWT and injects tenant-specific
	// database connections into the request context
	if tenantsMiddleware != nil && tenantsMiddleware.IsEnabled() {
		f.Use(tenantsMiddleware.Handler())
	}

	// Register all routes
	RegisterRoutesToApp(f, auth, th, oh, ah, bh, orh, trh, adminHandler)

	// Health
	f.Get("/health", libHTTP.Ping)

	// Version
	f.Get("/version", libHTTP.Version)

	// Doc
	f.Get("/swagger/*", WithSwaggerEnvConfig(), fiberSwagger.FiberWrapHandler(
		fiberSwagger.InstanceName("transaction"),
	))

	f.Use(tlMid.EndTracingSpans)

	return f
}

// RegisterRoutesToApp registers transaction routes to an existing Fiber app.
// This is used by the unified ledger server to consolidate all routes in a single port.
// The app should already have middleware configured (telemetry, cors, logging).
func RegisterRoutesToApp(f *fiber.App, auth *middleware.AuthClient, th *TransactionHandler, oh *OperationHandler, ah *AssetRateHandler, bh *BalanceHandler, orh *OperationRouteHandler, trh *TransactionRouteHandler, adminHandler *AdminHandler) {
	// Transactions
	f.Post("/v1/organizations/:organization_id/ledgers/:ledger_id/transactions/dsl", auth.Authorize(midazName, "transactions", "post"), http.ParseUUIDPathParameters("transaction"), th.CreateTransactionDSL)
	f.Post("/v1/organizations/:organization_id/ledgers/:ledger_id/transactions/json", auth.Authorize(midazName, "transactions", "post"), http.ParseUUIDPathParameters("transaction"), http.WithBody(new(transaction.CreateTransactionInput), th.CreateTransactionJSON))
	f.Post("/v1/organizations/:organization_id/ledgers/:ledger_id/transactions/inflow", auth.Authorize(midazName, "transactions", "post"), http.ParseUUIDPathParameters("transaction"), http.WithBody(new(transaction.CreateTransactionInflowInput), th.CreateTransactionInflow))
	f.Post("/v1/organizations/:organization_id/ledgers/:ledger_id/transactions/outflow", auth.Authorize(midazName, "transactions", "post"), http.ParseUUIDPathParameters("transaction"), http.WithBody(new(transaction.CreateTransactionOutflowInput), th.CreateTransactionOutflow))
	f.Post("/v1/organizations/:organization_id/ledgers/:ledger_id/transactions/annotation", auth.Authorize(midazName, "transactions", "post"), http.ParseUUIDPathParameters("transaction"), http.WithBody(new(transaction.CreateTransactionInput), th.CreateTransactionAnnotation))

	f.Post("/v1/organizations/:organization_id/ledgers/:ledger_id/transactions/:transaction_id/commit", auth.Authorize(midazName, "transactions", "post"), http.ParseUUIDPathParameters("transaction"), th.CommitTransaction)
	f.Post("/v1/organizations/:organization_id/ledgers/:ledger_id/transactions/:transaction_id/cancel", auth.Authorize(midazName, "transactions", "post"), http.ParseUUIDPathParameters("transaction"), th.CancelTransaction)
	f.Post("/v1/organizations/:organization_id/ledgers/:ledger_id/transactions/:transaction_id/revert", auth.Authorize(midazName, "transactions", "post"), http.ParseUUIDPathParameters("transaction"), th.RevertTransaction)

	f.Patch("/v1/organizations/:organization_id/ledgers/:ledger_id/transactions/:transaction_id", auth.Authorize(midazName, "transactions", "patch"), http.ParseUUIDPathParameters("transaction"), http.WithBody(new(transaction.UpdateTransactionInput), th.UpdateTransaction))

	f.Get("/v1/organizations/:organization_id/ledgers/:ledger_id/transactions/:transaction_id", auth.Authorize(midazName, "transactions", "get"), http.ParseUUIDPathParameters("transaction"), th.GetTransaction)
	f.Get("/v1/organizations/:organization_id/ledgers/:ledger_id/transactions", auth.Authorize(midazName, "transactions", "get"), http.ParseUUIDPathParameters("transaction"), th.GetAllTransactions)

	// Operations
	f.Get("/v1/organizations/:organization_id/ledgers/:ledger_id/accounts/:account_id/operations", auth.Authorize(midazName, "operations", "get"), http.ParseUUIDPathParameters("operation"), oh.GetAllOperationsByAccount)
	f.Get("/v1/organizations/:organization_id/ledgers/:ledger_id/accounts/:account_id/operations/:operation_id", auth.Authorize(midazName, "operations", "get"), http.ParseUUIDPathParameters("operation"), oh.GetOperationByAccount)
	f.Patch("/v1/organizations/:organization_id/ledgers/:ledger_id/transactions/:transaction_id/operations/:operation_id", auth.Authorize(midazName, "operations", "patch"), http.ParseUUIDPathParameters("operation"), http.WithBody(new(operation.UpdateOperationInput), oh.UpdateOperation))

	// Asset-rate
	f.Put("/v1/organizations/:organization_id/ledgers/:ledger_id/asset-rates", auth.Authorize(midazName, "asset-rates", "put"), http.ParseUUIDPathParameters("asset-rate"), http.WithBody(new(assetrate.CreateAssetRateInput), ah.CreateOrUpdateAssetRate))
	f.Get("/v1/organizations/:organization_id/ledgers/:ledger_id/asset-rates/:external_id", auth.Authorize(midazName, "asset-rates", "get"), http.ParseUUIDPathParameters("asset-rate"), ah.GetAssetRateByExternalID)
	f.Get("/v1/organizations/:organization_id/ledgers/:ledger_id/asset-rates/from/:asset_code", auth.Authorize(midazName, "asset-rates", "get"), http.ParseUUIDPathParameters("asset-rate"), ah.GetAllAssetRatesByAssetCode)

	//Balance
	f.Patch("/v1/organizations/:organization_id/ledgers/:ledger_id/balances/:balance_id", auth.Authorize(midazName, "balances", "patch"), http.ParseUUIDPathParameters("balance"), http.WithBody(new(mmodel.UpdateBalance), bh.UpdateBalance))
	f.Delete("/v1/organizations/:organization_id/ledgers/:ledger_id/balances/:balance_id", auth.Authorize(midazName, "balances", "delete"), http.ParseUUIDPathParameters("balance"), bh.DeleteBalanceByID)
	f.Get("/v1/organizations/:organization_id/ledgers/:ledger_id/balances", auth.Authorize(midazName, "balances", "get"), http.ParseUUIDPathParameters("balance"), bh.GetAllBalances)
	f.Get("/v1/organizations/:organization_id/ledgers/:ledger_id/balances/:balance_id", auth.Authorize(midazName, "balances", "get"), http.ParseUUIDPathParameters("balance"), bh.GetBalanceByID)
	f.Get("/v1/organizations/:organization_id/ledgers/:ledger_id/accounts/:account_id/balances", auth.Authorize(midazName, "balances", "get"), http.ParseUUIDPathParameters("balance"), bh.GetAllBalancesByAccountID)
	f.Get("/v1/organizations/:organization_id/ledgers/:ledger_id/accounts/alias/:alias/balances", auth.Authorize(midazName, "balances", "get"), http.ParseUUIDPathParameters("balance"), bh.GetBalancesByAlias)
	f.Get("/v1/organizations/:organization_id/ledgers/:ledger_id/accounts/external/:code/balances", auth.Authorize(midazName, "balances", "get"), http.ParseUUIDPathParameters("balance"), bh.GetBalancesExternalByCode)
	f.Post("/v1/organizations/:organization_id/ledgers/:ledger_id/accounts/:account_id/balances", auth.Authorize(midazName, "balances", "post"), http.ParseUUIDPathParameters("balance"), http.WithBody(new(mmodel.CreateAdditionalBalance), bh.CreateAdditionalBalance))

	// Operation-route
	f.Post("/v1/organizations/:organization_id/ledgers/:ledger_id/operation-routes", auth.Authorize(routingName, "operation-routes", "post"), http.ParseUUIDPathParameters("operation_route"), http.WithBody(new(mmodel.CreateOperationRouteInput), orh.CreateOperationRoute))
	f.Get("/v1/organizations/:organization_id/ledgers/:ledger_id/operation-routes/:operation_route_id", auth.Authorize(routingName, "operation-routes", "get"), http.ParseUUIDPathParameters("operation_route"), orh.GetOperationRouteByID)
	f.Patch("/v1/organizations/:organization_id/ledgers/:ledger_id/operation-routes/:operation_route_id", auth.Authorize(routingName, "operation-routes", "patch"), http.ParseUUIDPathParameters("operation_route"), http.WithBody(new(mmodel.UpdateOperationRouteInput), orh.UpdateOperationRoute))
	f.Delete("/v1/organizations/:organization_id/ledgers/:ledger_id/operation-routes/:operation_route_id", auth.Authorize(routingName, "operation-routes", "delete"), http.ParseUUIDPathParameters("operation_route"), orh.DeleteOperationRouteByID)
	f.Get("/v1/organizations/:organization_id/ledgers/:ledger_id/operation-routes", auth.Authorize(routingName, "operation-routes", "get"), http.ParseUUIDPathParameters("operation_route"), orh.GetAllOperationRoutes)

	// Transaction-route
	f.Post("/v1/organizations/:organization_id/ledgers/:ledger_id/transaction-routes", auth.Authorize(routingName, "transaction-routes", "post"), http.ParseUUIDPathParameters("transaction_route"), http.WithBody(new(mmodel.CreateTransactionRouteInput), trh.CreateTransactionRoute))
	f.Get("/v1/organizations/:organization_id/ledgers/:ledger_id/transaction-routes/:transaction_route_id", auth.Authorize(routingName, "transaction-routes", "get"), http.ParseUUIDPathParameters("transaction_route"), trh.GetTransactionRouteByID)
	f.Patch("/v1/organizations/:organization_id/ledgers/:ledger_id/transaction-routes/:transaction_route_id", auth.Authorize(routingName, "transaction-routes", "patch"), http.ParseUUIDPathParameters("transaction_route"), http.WithBody(new(mmodel.UpdateTransactionRouteInput), trh.UpdateTransactionRoute))
	f.Delete("/v1/organizations/:organization_id/ledgers/:ledger_id/transaction-routes/:transaction_route_id", auth.Authorize(routingName, "transaction-routes", "delete"), http.ParseUUIDPathParameters("transaction_route"), trh.DeleteTransactionRouteByID)
	f.Get("/v1/organizations/:organization_id/ledgers/:ledger_id/transaction-routes", auth.Authorize(routingName, "transaction-routes", "get"), http.ParseUUIDPathParameters("transaction_route"), trh.GetAllTransactionRoutes)

	// Admin routes (require special authorization)
	adminGroup := f.Group("/admin")
	adminGroup.Post("/cache/tenants/:id/invalidate", auth.Authorize(midazName, "admin", "post"), adminHandler.InvalidateTenantCache)
	adminGroup.Post("/cache/tenants/invalidate", auth.Authorize(midazName, "admin", "post"), adminHandler.InvalidateAllTenantCache)
}
