// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"github.com/LerianStudio/lib-auth/v2/auth/middleware"
	libLog "github.com/LerianStudio/lib-commons/v4/commons/log"
	libHTTP "github.com/LerianStudio/lib-commons/v4/commons/net/http"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v4/commons/opentelemetry"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/postgres/assetrate"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/postgres/operation"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/postgres/transaction"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/LerianStudio/midaz/v3/pkg/net/http"
	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
	fiberSwagger "github.com/swaggo/fiber-swagger"
)

const (
	midazName   = "midaz"
	routingName = "routing"
)

// NewRouter register NewRouter routes to the Server.
func NewRouter(lg libLog.Logger, tl *libOpentelemetry.Telemetry, auth *middleware.AuthClient, th *TransactionHandler, oh *OperationHandler, ah *AssetRateHandler, bh *BalanceHandler, orh *OperationRouteHandler, trh *TransactionRouteHandler) *fiber.App {
	f := fiber.New(fiber.Config{
		DisableStartupMessage: true,
		ErrorHandler: func(ctx *fiber.Ctx, err error) error {
			return libHTTP.FiberErrorHandler(ctx, err)
		},
	})

	tlMid := libHTTP.NewTelemetryMiddleware(tl)

	f.Use(tlMid.WithTelemetry(tl))
	f.Use(cors.New())
	f.Use(libHTTP.WithHTTPLogging(libHTTP.WithCustomLogger(lg)))
	// Register all routes
	RegisterRoutesToApp(f, auth, th, oh, ah, bh, orh, trh, nil)

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
func RegisterRoutesToApp(f fiber.Router, auth *middleware.AuthClient, th *TransactionHandler, oh *OperationHandler, ah *AssetRateHandler, bh *BalanceHandler, orh *OperationRouteHandler, trh *TransactionRouteHandler, routeOptions *http.ProtectedRouteOptions) {
	// Transactions
	f.Post("/v1/organizations/:organization_id/ledgers/:ledger_id/transactions/dsl", protectedMidaz(auth, "transactions", "post", routeOptions, http.ParseUUIDPathParameters("transaction"), th.CreateTransactionDSL)...)
	f.Post("/v1/organizations/:organization_id/ledgers/:ledger_id/transactions/json", protectedMidaz(auth, "transactions", "post", routeOptions, http.ParseUUIDPathParameters("transaction"), http.WithBody(new(transaction.CreateTransactionInput), th.CreateTransactionJSON))...)
	f.Post("/v1/organizations/:organization_id/ledgers/:ledger_id/transactions/inflow", protectedMidaz(auth, "transactions", "post", routeOptions, http.ParseUUIDPathParameters("transaction"), http.WithBody(new(transaction.CreateTransactionInflowInput), th.CreateTransactionInflow))...)
	f.Post("/v1/organizations/:organization_id/ledgers/:ledger_id/transactions/outflow", protectedMidaz(auth, "transactions", "post", routeOptions, http.ParseUUIDPathParameters("transaction"), http.WithBody(new(transaction.CreateTransactionOutflowInput), th.CreateTransactionOutflow))...)
	f.Post("/v1/organizations/:organization_id/ledgers/:ledger_id/transactions/annotation", protectedMidaz(auth, "transactions", "post", routeOptions, http.ParseUUIDPathParameters("transaction"), http.WithBody(new(transaction.CreateTransactionInput), th.CreateTransactionAnnotation))...)

	f.Post("/v1/organizations/:organization_id/ledgers/:ledger_id/transactions/:transaction_id/commit", protectedMidaz(auth, "transactions", "post", routeOptions, http.ParseUUIDPathParameters("transaction"), th.CommitTransaction)...)
	f.Post("/v1/organizations/:organization_id/ledgers/:ledger_id/transactions/:transaction_id/cancel", protectedMidaz(auth, "transactions", "post", routeOptions, http.ParseUUIDPathParameters("transaction"), th.CancelTransaction)...)
	f.Post("/v1/organizations/:organization_id/ledgers/:ledger_id/transactions/:transaction_id/revert", protectedMidaz(auth, "transactions", "post", routeOptions, http.ParseUUIDPathParameters("transaction"), th.RevertTransaction)...)

	f.Patch("/v1/organizations/:organization_id/ledgers/:ledger_id/transactions/:transaction_id", protectedMidaz(auth, "transactions", "patch", routeOptions, http.ParseUUIDPathParameters("transaction"), http.WithBody(new(transaction.UpdateTransactionInput), th.UpdateTransaction))...)

	f.Get("/v1/organizations/:organization_id/ledgers/:ledger_id/transactions/count", protectedMidaz(auth, "transactions", "get", routeOptions, http.ParseUUIDPathParameters("transaction"), th.CountTransactionsByRoute)...)
	f.Get("/v1/organizations/:organization_id/ledgers/:ledger_id/transactions/:transaction_id", protectedMidaz(auth, "transactions", "get", routeOptions, http.ParseUUIDPathParameters("transaction"), th.GetTransaction)...)
	f.Get("/v1/organizations/:organization_id/ledgers/:ledger_id/transactions", protectedMidaz(auth, "transactions", "get", routeOptions, http.ParseUUIDPathParameters("transaction"), th.GetAllTransactions)...)

	// Operations
	f.Get("/v1/organizations/:organization_id/ledgers/:ledger_id/accounts/:account_id/operations", protectedMidaz(auth, "operations", "get", routeOptions, http.ParseUUIDPathParameters("operation"), oh.GetAllOperationsByAccount)...)
	f.Get("/v1/organizations/:organization_id/ledgers/:ledger_id/accounts/:account_id/operations/:operation_id", protectedMidaz(auth, "operations", "get", routeOptions, http.ParseUUIDPathParameters("operation"), oh.GetOperationByAccount)...)
	f.Patch("/v1/organizations/:organization_id/ledgers/:ledger_id/transactions/:transaction_id/operations/:operation_id", protectedMidaz(auth, "operations", "patch", routeOptions, http.ParseUUIDPathParameters("operation"), http.WithBody(new(operation.UpdateOperationInput), oh.UpdateOperation))...)

	// Asset-rate
	f.Put("/v1/organizations/:organization_id/ledgers/:ledger_id/asset-rates", protectedMidaz(auth, "asset-rates", "put", routeOptions, http.ParseUUIDPathParameters("asset-rate"), http.WithBody(new(assetrate.CreateAssetRateInput), ah.CreateOrUpdateAssetRate))...)
	f.Get("/v1/organizations/:organization_id/ledgers/:ledger_id/asset-rates/:external_id", protectedMidaz(auth, "asset-rates", "get", routeOptions, http.ParseUUIDPathParameters("asset-rate"), ah.GetAssetRateByExternalID)...)
	f.Get("/v1/organizations/:organization_id/ledgers/:ledger_id/asset-rates/from/:asset_code", protectedMidaz(auth, "asset-rates", "get", routeOptions, http.ParseUUIDPathParameters("asset-rate"), ah.GetAllAssetRatesByAssetCode)...)

	// Balance
	f.Patch("/v1/organizations/:organization_id/ledgers/:ledger_id/balances/:balance_id", protectedMidaz(auth, "balances", "patch", routeOptions, http.ParseUUIDPathParameters("balance"), http.WithBody(new(mmodel.UpdateBalance), bh.UpdateBalance))...)
	f.Delete("/v1/organizations/:organization_id/ledgers/:ledger_id/balances/:balance_id", protectedMidaz(auth, "balances", "delete", routeOptions, http.ParseUUIDPathParameters("balance"), bh.DeleteBalanceByID)...)
	f.Get("/v1/organizations/:organization_id/ledgers/:ledger_id/balances", protectedMidaz(auth, "balances", "get", routeOptions, http.ParseUUIDPathParameters("balance"), bh.GetAllBalances)...)
	f.Get("/v1/organizations/:organization_id/ledgers/:ledger_id/balances/:balance_id", protectedMidaz(auth, "balances", "get", routeOptions, http.ParseUUIDPathParameters("balance"), bh.GetBalanceByID)...)
	f.Get("/v1/organizations/:organization_id/ledgers/:ledger_id/balances/:balance_id/history", protectedMidaz(auth, "balances", "get", routeOptions, http.ParseUUIDPathParameters("balance"), bh.GetBalanceAtTimestamp)...)
	f.Get("/v1/organizations/:organization_id/ledgers/:ledger_id/accounts/:account_id/balances", protectedMidaz(auth, "balances", "get", routeOptions, http.ParseUUIDPathParameters("balance"), bh.GetAllBalancesByAccountID)...)
	f.Get("/v1/organizations/:organization_id/ledgers/:ledger_id/accounts/:account_id/balances/history", protectedMidaz(auth, "balances", "get", routeOptions, http.ParseUUIDPathParameters("balance"), bh.GetAccountBalancesAtTimestamp)...)
	f.Get("/v1/organizations/:organization_id/ledgers/:ledger_id/accounts/alias/:alias/balances", protectedMidaz(auth, "balances", "get", routeOptions, http.ParseUUIDPathParameters("balance"), bh.GetBalancesByAlias)...)
	f.Get("/v1/organizations/:organization_id/ledgers/:ledger_id/accounts/external/:code/balances", protectedMidaz(auth, "balances", "get", routeOptions, http.ParseUUIDPathParameters("balance"), bh.GetBalancesExternalByCode)...)
	f.Post("/v1/organizations/:organization_id/ledgers/:ledger_id/accounts/:account_id/balances", protectedMidaz(auth, "balances", "post", routeOptions, http.ParseUUIDPathParameters("balance"), http.WithBody(new(mmodel.CreateAdditionalBalance), bh.CreateAdditionalBalance))...)

	// Operation-route
	f.Post("/v1/organizations/:organization_id/ledgers/:ledger_id/operation-routes", protectedRouting(auth, "operation-routes", "post", routeOptions, http.ParseUUIDPathParameters("operation_route"), http.WithBody(new(mmodel.CreateOperationRouteInput), orh.CreateOperationRoute))...)
	f.Get("/v1/organizations/:organization_id/ledgers/:ledger_id/operation-routes/:operation_route_id", protectedRouting(auth, "operation-routes", "get", routeOptions, http.ParseUUIDPathParameters("operation_route"), orh.GetOperationRouteByID)...)
	f.Patch("/v1/organizations/:organization_id/ledgers/:ledger_id/operation-routes/:operation_route_id", protectedRouting(auth, "operation-routes", "patch", routeOptions, http.ParseUUIDPathParameters("operation_route"), http.WithBody(new(mmodel.UpdateOperationRouteInput), orh.UpdateOperationRoute))...)
	f.Delete("/v1/organizations/:organization_id/ledgers/:ledger_id/operation-routes/:operation_route_id", protectedRouting(auth, "operation-routes", "delete", routeOptions, http.ParseUUIDPathParameters("operation_route"), orh.DeleteOperationRouteByID)...)
	f.Get("/v1/organizations/:organization_id/ledgers/:ledger_id/operation-routes", protectedRouting(auth, "operation-routes", "get", routeOptions, http.ParseUUIDPathParameters("operation_route"), orh.GetAllOperationRoutes)...)

	// Transaction-route
	f.Post("/v1/organizations/:organization_id/ledgers/:ledger_id/transaction-routes", protectedRouting(auth, "transaction-routes", "post", routeOptions, http.ParseUUIDPathParameters("transaction_route"), http.WithBody(new(mmodel.CreateTransactionRouteInput), trh.CreateTransactionRoute))...)
	f.Get("/v1/organizations/:organization_id/ledgers/:ledger_id/transaction-routes/:transaction_route_id", protectedRouting(auth, "transaction-routes", "get", routeOptions, http.ParseUUIDPathParameters("transaction_route"), trh.GetTransactionRouteByID)...)
	f.Patch("/v1/organizations/:organization_id/ledgers/:ledger_id/transaction-routes/:transaction_route_id", protectedRouting(auth, "transaction-routes", "patch", routeOptions, http.ParseUUIDPathParameters("transaction_route"), http.WithBody(new(mmodel.UpdateTransactionRouteInput), trh.UpdateTransactionRoute))...)
	f.Delete("/v1/organizations/:organization_id/ledgers/:ledger_id/transaction-routes/:transaction_route_id", protectedRouting(auth, "transaction-routes", "delete", routeOptions, http.ParseUUIDPathParameters("transaction_route"), trh.DeleteTransactionRouteByID)...)
	f.Get("/v1/organizations/:organization_id/ledgers/:ledger_id/transaction-routes", protectedRouting(auth, "transaction-routes", "get", routeOptions, http.ParseUUIDPathParameters("transaction_route"), trh.GetAllTransactionRoutes)...)
}

func protectedMidaz(auth *middleware.AuthClient, resource, action string, routeOptions *http.ProtectedRouteOptions, handlers ...fiber.Handler) []fiber.Handler {
	return http.ProtectedRouteChain(auth.Authorize(midazName, resource, action), routeOptions, handlers...)
}

func protectedRouting(auth *middleware.AuthClient, resource, action string, routeOptions *http.ProtectedRouteOptions, handlers ...fiber.Handler) []fiber.Handler {
	return http.ProtectedRouteChain(auth.Authorize(routingName, resource, action), routeOptions, handlers...)
}
