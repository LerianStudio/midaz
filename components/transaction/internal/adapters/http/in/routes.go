package in

import (
	"github.com/LerianStudio/midaz/components/transaction/internal/adapters/postgres/assetrate"
	"github.com/LerianStudio/midaz/components/transaction/internal/adapters/postgres/operation"
	"github.com/LerianStudio/midaz/components/transaction/internal/adapters/postgres/transaction"
	"github.com/LerianStudio/midaz/pkg/mlog"
	"github.com/LerianStudio/midaz/pkg/mmodel"
	"github.com/LerianStudio/midaz/pkg/mopentelemetry"
	"github.com/LerianStudio/midaz/pkg/net/http"
	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
	fiberSwagger "github.com/swaggo/fiber-swagger"
)

func NewRouter(lg mlog.Logger, tl *mopentelemetry.Telemetry, th *TransactionHandler, oh *OperationHandler, ah *AssetRateHandler, bh *BalanceHandler) *fiber.App {
	f := fiber.New(fiber.Config{
		DisableStartupMessage: true,
	})
	tlMid := http.NewTelemetryMiddleware(tl)

	f.Use(tlMid.WithTelemetry(tl))
	f.Use(cors.New())
	f.Use(http.WithHTTPLogging(http.WithCustomLogger(lg)))
	jwt := http.NewJWTMiddleware()

	// -- Routes --

	// Transactions
	f.Post("/v1/organizations/:organization_id/ledgers/:ledger_id/transactions/dsl", jwt.WithPermissionHTTP("transaction"), http.ParseUUIDPathParameters, th.CreateTransactionDSL)
	f.Post("/v1/organizations/:organization_id/ledgers/:ledger_id/transactions/json", jwt.WithPermissionHTTP("transaction"), http.ParseUUIDPathParameters, http.WithBody(new(transaction.CreateTransactionInput), th.CreateTransactionJSON))
	f.Post("/v1/organizations/:organization_id/ledgers/:ledger_id/transactions/templates", jwt.WithPermissionHTTP("transaction"), http.ParseUUIDPathParameters, http.WithBody(new(transaction.InputDSL), th.CreateTransactionTemplate))

	f.Post("/v1/organizations/:organization_id/ledgers/:ledger_id/transactions/:transaction_id/commit", jwt.WithPermissionHTTP("transaction"), http.ParseUUIDPathParameters, th.CommitTransaction)
	f.Post("/v1/organizations/:organization_id/ledgers/:ledger_id/transactions/:transaction_id/revert", jwt.WithPermissionHTTP("transaction"), http.ParseUUIDPathParameters, th.RevertTransaction)

	f.Patch("/v1/organizations/:organization_id/ledgers/:ledger_id/transactions/:transaction_id", jwt.WithPermissionHTTP("transaction"), http.ParseUUIDPathParameters, http.WithBody(new(transaction.UpdateTransactionInput), th.UpdateTransaction))

	f.Get("/v1/organizations/:organization_id/ledgers/:ledger_id/transactions/:transaction_id", jwt.WithPermissionHTTP("transaction"), http.ParseUUIDPathParameters, th.GetTransaction)
	f.Get("/v1/organizations/:organization_id/ledgers/:ledger_id/transactions", jwt.WithPermissionHTTP("transaction"), http.ParseUUIDPathParameters, th.GetAllTransactions)

	// Operations
	f.Get("/v1/organizations/:organization_id/ledgers/:ledger_id/accounts/:account_id/operations", jwt.WithPermissionHTTP("operation"), http.ParseUUIDPathParameters, oh.GetAllOperationsByAccount)
	f.Get("/v1/organizations/:organization_id/ledgers/:ledger_id/accounts/:account_id/operations/:operation_id", jwt.WithPermissionHTTP("operation"), http.ParseUUIDPathParameters, oh.GetOperationByAccount)

	f.Patch("/v1/organizations/:organization_id/ledgers/:ledger_id/transactions/:transaction_id/operations/:operation_id", jwt.WithPermissionHTTP("operation"), http.ParseUUIDPathParameters, http.WithBody(new(operation.UpdateOperationInput), oh.UpdateOperation))

	// Asset-rate
	f.Put("/v1/organizations/:organization_id/ledgers/:ledger_id/asset-rates", jwt.WithPermissionHTTP("asset-rate"), http.ParseUUIDPathParameters, http.WithBody(new(assetrate.CreateAssetRateInput), ah.CreateOrUpdateAssetRate))
	f.Get("/v1/organizations/:organization_id/ledgers/:ledger_id/asset-rates/:external_id", jwt.WithPermissionHTTP("asset-rate"), http.ParseUUIDPathParameters, ah.GetAssetRateByExternalID)
	f.Get("/v1/organizations/:organization_id/ledgers/:ledger_id/asset-rates/from/:asset_code", jwt.WithPermissionHTTP("asset-rate"), http.ParseUUIDPathParameters, ah.GetAllAssetRatesByAssetCode)

	//Balance
	f.Get("/v1/organizations/:organization_id/ledgers/:ledger_id/balances", jwt.WithPermissionHTTP("balance"), http.ParseUUIDPathParameters, bh.GetAllBalances)
	f.Get("/v1/organizations/:organization_id/ledgers/:ledger_id/balances/:balance_id", jwt.WithPermissionHTTP("balance"), http.ParseUUIDPathParameters, bh.GetBalanceByID)
	f.Get("/v1/organizations/:organization_id/ledgers/:ledger_id/accounts/:account_id/balances", jwt.WithPermissionHTTP("balance"), http.ParseUUIDPathParameters, bh.GetAllBalancesByAccountID)
	f.Delete("/v1/organizations/:organization_id/ledgers/:ledger_id/balances/:balance_id", jwt.WithPermissionHTTP("balance"), http.ParseUUIDPathParameters, bh.DeleteBalanceByID)
	f.Patch("/v1/organizations/:organization_id/ledgers/:ledger_id/balances/:balance_id", jwt.WithPermissionHTTP("balance"), http.ParseUUIDPathParameters, http.WithBody(new(mmodel.UpdateBalance), bh.UpdateBalance))

	// Health
	f.Get("/health", http.Ping)

	// Version
	f.Get("/version", http.Version)

	// Doc
	f.Get("/swagger/*", WithSwaggerEnvConfig(), fiberSwagger.WrapHandler)

	f.Use(tlMid.EndTracingSpans)

	return f
}
