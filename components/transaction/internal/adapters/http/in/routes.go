package in

import (
	"github.com/LerianStudio/lib-auth/auth/middleware"
	libLog "github.com/LerianStudio/lib-commons/commons/log"
	libHTTP "github.com/LerianStudio/lib-commons/commons/net/http"
	libOpentelemetry "github.com/LerianStudio/lib-commons/commons/opentelemetry"
	"github.com/LerianStudio/midaz/components/transaction/internal/adapters/postgres/assetrate"
	"github.com/LerianStudio/midaz/components/transaction/internal/adapters/postgres/operation"
	"github.com/LerianStudio/midaz/components/transaction/internal/adapters/postgres/transaction"
	"github.com/LerianStudio/midaz/pkg/mmodel"
	"github.com/LerianStudio/midaz/pkg/net/http"
	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
	fiberSwagger "github.com/swaggo/fiber-swagger"
)

const midazName = "midaz"

func NewRouter(lg libLog.Logger, tl *libOpentelemetry.Telemetry, auth *middleware.AuthClient, th *TransactionHandler, oh *OperationHandler, ah *AssetRateHandler, bh *BalanceHandler, orh *OperationRouteHandler) *fiber.App {
	f := fiber.New(fiber.Config{
		DisableStartupMessage: true,
	})
	tlMid := libHTTP.NewTelemetryMiddleware(tl)

	f.Use(tlMid.WithTelemetry(tl))
	f.Use(cors.New())
	f.Use(libHTTP.WithHTTPLogging(libHTTP.WithCustomLogger(lg)))

	// -- Routes --

	// Transactions
	f.Post("/v1/organizations/:organization_id/ledgers/:ledger_id/transactions/dsl", auth.Authorize(midazName, "transactions", "post"), http.ParseUUIDPathParameters, th.CreateTransactionDSL)
	f.Post("/v1/organizations/:organization_id/ledgers/:ledger_id/transactions/json", auth.Authorize(midazName, "transactions", "post"), http.ParseUUIDPathParameters, http.WithBody(new(transaction.CreateTransactionInput), th.CreateTransactionJSON))
	f.Post("/v1/organizations/:organization_id/ledgers/:ledger_id/transactions/inflow", auth.Authorize(midazName, "transactions", "post"), http.ParseUUIDPathParameters, http.WithBody(new(transaction.CreateTransactionInflowInput), th.CreateTransactionInflow))
	f.Post("/v1/organizations/:organization_id/ledgers/:ledger_id/transactions/outflow", auth.Authorize(midazName, "transactions", "post"), http.ParseUUIDPathParameters, http.WithBody(new(transaction.CreateTransactionOutflowInput), th.CreateTransactionOutflow))

	f.Post("/v1/organizations/:organization_id/ledgers/:ledger_id/transactions/:transaction_id/commit", auth.Authorize(midazName, "transactions", "post"), http.ParseUUIDPathParameters, th.CommitTransaction)
	f.Post("/v1/organizations/:organization_id/ledgers/:ledger_id/transactions/:transaction_id/cancel", auth.Authorize(midazName, "transactions", "post"), http.ParseUUIDPathParameters, th.CancelTransaction)
	f.Post("/v1/organizations/:organization_id/ledgers/:ledger_id/transactions/:transaction_id/revert", auth.Authorize(midazName, "transactions", "post"), http.ParseUUIDPathParameters, th.RevertTransaction)

	f.Patch("/v1/organizations/:organization_id/ledgers/:ledger_id/transactions/:transaction_id", auth.Authorize(midazName, "transactions", "patch"), http.ParseUUIDPathParameters, http.WithBody(new(transaction.UpdateTransactionInput), th.UpdateTransaction))

	f.Get("/v1/organizations/:organization_id/ledgers/:ledger_id/transactions/:transaction_id", auth.Authorize(midazName, "transactions", "get"), http.ParseUUIDPathParameters, th.GetTransaction)
	f.Get("/v1/organizations/:organization_id/ledgers/:ledger_id/transactions", auth.Authorize(midazName, "transactions", "get"), http.ParseUUIDPathParameters, th.GetAllTransactions)

	// Operations
	f.Get("/v1/organizations/:organization_id/ledgers/:ledger_id/accounts/:account_id/operations", auth.Authorize(midazName, "operations", "get"), http.ParseUUIDPathParameters, oh.GetAllOperationsByAccount)
	f.Get("/v1/organizations/:organization_id/ledgers/:ledger_id/accounts/:account_id/operations/:operation_id", auth.Authorize(midazName, "operations", "get"), http.ParseUUIDPathParameters, oh.GetOperationByAccount)
	f.Patch("/v1/organizations/:organization_id/ledgers/:ledger_id/transactions/:transaction_id/operations/:operation_id", auth.Authorize(midazName, "operations", "patch"), http.ParseUUIDPathParameters, http.WithBody(new(operation.UpdateOperationInput), oh.UpdateOperation))

	// Asset-rate
	f.Put("/v1/organizations/:organization_id/ledgers/:ledger_id/asset-rates", auth.Authorize(midazName, "asset-rates", "put"), http.ParseUUIDPathParameters, http.WithBody(new(assetrate.CreateAssetRateInput), ah.CreateOrUpdateAssetRate))
	f.Get("/v1/organizations/:organization_id/ledgers/:ledger_id/asset-rates/:external_id", auth.Authorize(midazName, "asset-rates", "get"), http.ParseUUIDPathParameters, ah.GetAssetRateByExternalID)
	f.Get("/v1/organizations/:organization_id/ledgers/:ledger_id/asset-rates/from/:asset_code", auth.Authorize(midazName, "asset-rates", "get"), http.ParseUUIDPathParameters, ah.GetAllAssetRatesByAssetCode)

	//Balance
	f.Patch("/v1/organizations/:organization_id/ledgers/:ledger_id/balances/:balance_id", auth.Authorize(midazName, "balances", "patch"), http.ParseUUIDPathParameters, http.WithBody(new(mmodel.UpdateBalance), bh.UpdateBalance))
	f.Delete("/v1/organizations/:organization_id/ledgers/:ledger_id/balances/:balance_id", auth.Authorize(midazName, "balances", "delete"), http.ParseUUIDPathParameters, bh.DeleteBalanceByID)
	f.Get("/v1/organizations/:organization_id/ledgers/:ledger_id/balances", auth.Authorize(midazName, "balances", "get"), http.ParseUUIDPathParameters, bh.GetAllBalances)
	f.Get("/v1/organizations/:organization_id/ledgers/:ledger_id/balances/:balance_id", auth.Authorize(midazName, "balances", "get"), http.ParseUUIDPathParameters, bh.GetBalanceByID)
	f.Get("/v1/organizations/:organization_id/ledgers/:ledger_id/accounts/:account_id/balances", auth.Authorize(midazName, "balances", "get"), http.ParseUUIDPathParameters, bh.GetAllBalancesByAccountID)
	f.Get("/v1/organizations/:organization_id/ledgers/:ledger_id/accounts/alias/:alias/balances", auth.Authorize(midazName, "balances", "get"), http.ParseUUIDPathParameters, bh.GetBalancesByAlias)
	f.Get("/v1/organizations/:organization_id/ledgers/:ledger_id/accounts/external/:code/balances", auth.Authorize(midazName, "balances", "get"), http.ParseUUIDPathParameters, bh.GetBalancesExternalByCode)

	// Operation-route
	f.Post("/v1/organizations/:organization_id/ledgers/:ledger_id/operation-routes", auth.Authorize(midazName, "operation-routes", "post"), http.ParseUUIDPathParameters, http.WithBody(new(mmodel.CreateOperationRouteInput), orh.CreateOperationRoute))
	f.Get("/v1/organizations/:organization_id/ledgers/:ledger_id/operation-routes/:operation_route_id", auth.Authorize(midazName, "operation-routes", "get"), http.ParseUUIDPathParameters, orh.GetOperationRouteByID)
	f.Patch("/v1/organizations/:organization_id/ledgers/:ledger_id/operation-routes/:operation_route_id", auth.Authorize(midazName, "operation-routes", "patch"), http.ParseUUIDPathParameters, http.WithBody(new(mmodel.UpdateOperationRouteInput), orh.UpdateOperationRoute))
	f.Delete("/v1/organizations/:organization_id/ledgers/:ledger_id/operation-routes/:operation_route_id", auth.Authorize(midazName, "operation-routes", "delete"), http.ParseUUIDPathParameters, orh.DeleteOperationRouteByID)
	f.Get("/v1/organizations/:organization_id/ledgers/:ledger_id/operation-routes", auth.Authorize(midazName, "operation-routes", "get"), http.ParseUUIDPathParameters, orh.GetAllOperationRoutes)

	// Health
	f.Get("/health", libHTTP.Ping)

	// Version
	f.Get("/version", libHTTP.Version)

	// Doc
	f.Get("/swagger/*", WithSwaggerEnvConfig(), fiberSwagger.WrapHandler)

	f.Use(tlMid.EndTracingSpans)

	return f
}
