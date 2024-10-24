package http

import (
	"github.com/LerianStudio/midaz/common/mcasdoor"
	"github.com/LerianStudio/midaz/common/mlog"
	lib "github.com/LerianStudio/midaz/common/net/http"
	ar "github.com/LerianStudio/midaz/components/transaction/internal/domain/assetrate"
	o "github.com/LerianStudio/midaz/components/transaction/internal/domain/operation"
	t "github.com/LerianStudio/midaz/components/transaction/internal/domain/transaction"
	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
)

func NewRouter(lg mlog.Logger, cc *mcasdoor.CasdoorConnection, th *TransactionHandler, oh *OperationHandler, ah *AssetRateHandler) *fiber.App {
	f := fiber.New(fiber.Config{
		DisableStartupMessage: true,
	})

	f.Use(cors.New())
	f.Use(lib.WithCorrelationID())
	f.Use(lib.WithHTTPLogging(lib.WithCustomLogger(lg)))
	jwt := lib.NewJWTMiddleware(cc)

	// -- Routes --

	// Transactions
	f.Post("/v1/organizations/:organization_id/ledgers/:ledger_id/transactions/dsl", jwt.ProtectHTTP(), jwt.WithPermissionHTTP("transactions"), th.CreateTransactionDSL)
	f.Post("/v1/organizations/:organization_id/ledgers/:ledger_id/transactions/json", jwt.ProtectHTTP(), jwt.WithPermissionHTTP("transactions"), lib.WithBody(new(t.CreateTransactionInput), th.CreateTransactionJSON))
	f.Post("/v1/organizations/:organization_id/ledgers/:ledger_id/transactions/templates", jwt.ProtectHTTP(), jwt.WithPermissionHTTP("transaction"), lib.WithBody(new(t.InputDSL), th.CreateTransactionTemplate))

	f.Post("/v1/organizations/:organization_id/ledgers/:ledger_id/transactions/:transaction_id/commit", jwt.ProtectHTTP(), jwt.WithPermissionHTTP("transactions"), th.CommitTransaction)
	f.Post("/v1/organizations/:organization_id/ledgers/:ledger_id/transactions/:transaction_id/revert", jwt.ProtectHTTP(), jwt.WithPermissionHTTP("transactions"), th.RevertTransaction)

	f.Patch("/v1/organizations/:organization_id/ledgers/:ledger_id/transactions/:transaction_id", jwt.ProtectHTTP(), jwt.WithPermissionHTTP("transactions"), lib.WithBody(new(t.UpdateTransactionInput), th.UpdateTransaction))

	f.Get("/v1/organizations/:organization_id/ledgers/:ledger_id/transactions/:transaction_id", jwt.ProtectHTTP(), jwt.WithPermissionHTTP("transactions"), th.GetTransaction)
	f.Get("/v1/organizations/:organization_id/ledgers/:ledger_id/transactions", jwt.ProtectHTTP(), jwt.WithPermissionHTTP("transactions"), th.GetAllTransactions)

	// Operations
	f.Get("/v1/organizations/:organization_id/ledgers/:ledger_id/accounts/:account_id/operations", jwt.ProtectHTTP(), jwt.WithPermissionHTTP("operations"), oh.GetAllOperationsByAccount)
	f.Get("/v1/organizations/:organization_id/ledgers/:ledger_id/accounts/:account_id/operations/:operation_id", jwt.ProtectHTTP(), jwt.WithPermissionHTTP("operations"), oh.GetOperationByAccount)
	f.Get("/v1/organizations/:organization_id/ledgers/:ledger_id/portfolios/:portfolio_id/operations", jwt.ProtectHTTP(), jwt.WithPermissionHTTP("operations"), oh.GetAllOperationsByPortfolio)
	f.Get("/v1/organizations/:organization_id/ledgers/:ledger_id/portfolios/:portfolio_id/operations/:operation_id", jwt.ProtectHTTP(), jwt.WithPermissionHTTP("operations"), oh.GetOperationByPortfolio)

	f.Patch("/v1/organizations/:organization_id/ledgers/:ledger_id/transactions/:transaction_id/operations/:operation_id", jwt.ProtectHTTP(), jwt.WithPermissionHTTP("operations"), lib.WithBody(new(o.UpdateOperationInput), oh.UpdateOperation))

	// Asset-rate
	f.Post("/v1/organizations/:organization_id/ledgers/:ledger_id/asset-rates", jwt.ProtectHTTP(), jwt.WithPermissionHTTP("asset-rate"), lib.ParseUUIDPathParameters, lib.WithBody(new(ar.CreateAssetRateInput), ah.CreateAssetRate))
	f.Get("/v1/organizations/:organization_id/ledgers/:ledger_id/asset-rates/:asset_rate_id", jwt.ProtectHTTP(), jwt.WithPermissionHTTP("asset-rate"), lib.ParseUUIDPathParameters, ah.GetAssetRate)

	// Health
	f.Get("/health", lib.Ping)

	// Doc
	lib.DocAPI("transaction", "Transaction API", f)

	return f
}
