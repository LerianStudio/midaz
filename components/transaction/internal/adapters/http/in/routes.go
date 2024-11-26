package in

import (
	"github.com/LerianStudio/midaz/pkg/mcasdoor"
	"github.com/LerianStudio/midaz/pkg/mlog"
	"github.com/LerianStudio/midaz/pkg/mopentelemetry"
	"github.com/LerianStudio/midaz/pkg/net/http"
	"github.com/LerianStudio/midaz/components/transaction/internal/adapters/postgres/assetrate"
	"github.com/LerianStudio/midaz/components/transaction/internal/adapters/postgres/operation"
	"github.com/LerianStudio/midaz/components/transaction/internal/adapters/postgres/transaction"
	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
	fiberSwagger "github.com/swaggo/fiber-swagger"
)

func NewRouter(lg mlog.Logger, tl *mopentelemetry.Telemetry, cc *mcasdoor.CasdoorConnection, th *TransactionHandler, oh *OperationHandler, ah *AssetRateHandler) *fiber.App {
	f := fiber.New(fiber.Config{
		DisableStartupMessage: true,
	})
	tlMid := http.NewTelemetryMiddleware(tl)

	f.Use(tlMid.WithTelemetry(tl))
	f.Use(cors.New())
	f.Use(http.WithCorrelationID())
	f.Use(http.WithHTTPLogging(http.WithCustomLogger(lg)))
	jwt := http.NewJWTMiddleware(cc)

	// -- Routes --

	// Transactions
	f.Post("/v1/organizations/:organization_id/ledgers/:ledger_id/transactions/dsl", jwt.ProtectHTTP(), jwt.WithPermissionHTTP("transactions"), http.ParseUUIDPathParameters, th.CreateTransactionDSL)
	f.Post("/v1/organizations/:organization_id/ledgers/:ledger_id/transactions/json", jwt.ProtectHTTP(), jwt.WithPermissionHTTP("transactions"), http.ParseUUIDPathParameters, http.WithBody(new(transaction.CreateTransactionInput), th.CreateTransactionJSON))
	f.Post("/v1/organizations/:organization_id/ledgers/:ledger_id/transactions/templates", jwt.ProtectHTTP(), jwt.WithPermissionHTTP("transaction"), http.ParseUUIDPathParameters, http.WithBody(new(transaction.InputDSL), th.CreateTransactionTemplate))

	f.Post("/v1/organizations/:organization_id/ledgers/:ledger_id/transactions/:transaction_id/commit", jwt.ProtectHTTP(), jwt.WithPermissionHTTP("transactions"), http.ParseUUIDPathParameters, th.CommitTransaction)
	f.Post("/v1/organizations/:organization_id/ledgers/:ledger_id/transactions/:transaction_id/revert", jwt.ProtectHTTP(), jwt.WithPermissionHTTP("transactions"), http.ParseUUIDPathParameters, th.RevertTransaction)

	f.Patch("/v1/organizations/:organization_id/ledgers/:ledger_id/transactions/:transaction_id", jwt.ProtectHTTP(), jwt.WithPermissionHTTP("transactions"), http.ParseUUIDPathParameters, http.WithBody(new(transaction.UpdateTransactionInput), th.UpdateTransaction))

	f.Get("/v1/organizations/:organization_id/ledgers/:ledger_id/transactions/:transaction_id", jwt.ProtectHTTP(), jwt.WithPermissionHTTP("transactions"), http.ParseUUIDPathParameters, th.GetTransaction)
	f.Get("/v1/organizations/:organization_id/ledgers/:ledger_id/transactions", jwt.ProtectHTTP(), jwt.WithPermissionHTTP("transactions"), http.ParseUUIDPathParameters, th.GetAllTransactions)

	// Operations
	f.Get("/v1/organizations/:organization_id/ledgers/:ledger_id/accounts/:account_id/operations", jwt.ProtectHTTP(), jwt.WithPermissionHTTP("operations"), http.ParseUUIDPathParameters, oh.GetAllOperationsByAccount)
	f.Get("/v1/organizations/:organization_id/ledgers/:ledger_id/accounts/:account_id/operations/:operation_id", jwt.ProtectHTTP(), jwt.WithPermissionHTTP("operations"), http.ParseUUIDPathParameters, oh.GetOperationByAccount)
	f.Get("/v1/organizations/:organization_id/ledgers/:ledger_id/portfolios/:portfolio_id/operations", jwt.ProtectHTTP(), jwt.WithPermissionHTTP("operations"), http.ParseUUIDPathParameters, oh.GetAllOperationsByPortfolio)
	f.Get("/v1/organizations/:organization_id/ledgers/:ledger_id/portfolios/:portfolio_id/operations/:operation_id", jwt.ProtectHTTP(), jwt.WithPermissionHTTP("operations"), http.ParseUUIDPathParameters, oh.GetOperationByPortfolio)

	f.Patch("/v1/organizations/:organization_id/ledgers/:ledger_id/transactions/:transaction_id/operations/:operation_id", jwt.ProtectHTTP(), jwt.WithPermissionHTTP("operations"), http.ParseUUIDPathParameters, http.WithBody(new(operation.UpdateOperationInput), oh.UpdateOperation))

	// Asset-rate
	f.Post("/v1/organizations/:organization_id/ledgers/:ledger_id/asset-rates", jwt.ProtectHTTP(), jwt.WithPermissionHTTP("asset-rate"), http.ParseUUIDPathParameters, http.WithBody(new(assetrate.CreateAssetRateInput), ah.CreateAssetRate))
	f.Get("/v1/organizations/:organization_id/ledgers/:ledger_id/asset-rates/:asset_rate_id", jwt.ProtectHTTP(), jwt.WithPermissionHTTP("asset-rate"), http.ParseUUIDPathParameters, ah.GetAssetRate)

	// Health
	f.Get("/health", http.Ping)

	// Version
	f.Get("/version", http.Version)

	// Doc
	f.Get("/swagger/*", WithSwaggerEnvConfig(), fiberSwagger.WrapHandler)

	f.Use(tlMid.EndTracingSpans)

	return f
}
