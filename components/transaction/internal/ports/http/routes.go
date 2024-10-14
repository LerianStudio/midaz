package http

import (
	"github.com/LerianStudio/midaz/common/mcasdoor"
	lib "github.com/LerianStudio/midaz/common/net/http"
	t "github.com/LerianStudio/midaz/components/transaction/internal/domain/transaction"
	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
)

func NewRouter(cc *mcasdoor.CasdoorConnection, th *TransactionHandler, oh *OperationHandler) *fiber.App {
	f := fiber.New(fiber.Config{
		DisableStartupMessage: true,
	})

	f.Use(cors.New())
	f.Use(lib.WithCorrelationID())
	jwt := lib.NewJWTMiddleware(cc)

	// -- Routes --

	// Transactions
	f.Post("/v1/organizations/:organization_id/ledgers/:ledger_id/transactions", jwt.ProtectHTTP(), jwt.WithPermissionHTTP("transactions"), th.CreateTransaction)
	f.Patch("/v1/organizations/:organization_id/ledgers/:ledger_id/transactions/:transaction_id", jwt.ProtectHTTP(), jwt.WithPermissionHTTP("transactions"), lib.WithBody(new(t.UpdateTransactionInput), th.UpdateTransaction))
	f.Post("/v1/organizations/:organization_id/ledgers/:ledger_id/transactions/:transaction_id/commit", jwt.ProtectHTTP(), jwt.WithPermissionHTTP("transactions"), th.CommitTransaction)
	f.Post("/v1/organizations/:organization_id/ledgers/:ledger_id/transactions/:transaction_id/revert", jwt.ProtectHTTP(), jwt.WithPermissionHTTP("transactions"), th.RevertTransaction)
	f.Get("/v1/organizations/:organization_id/ledgers/:ledger_id/transactions/:transaction_id", jwt.ProtectHTTP(), jwt.WithPermissionHTTP("transactions"), th.GetTransaction)
	f.Get("/v1/organizations/:organization_id/ledgers/:ledger_id/transactions", jwt.ProtectHTTP(), jwt.WithPermissionHTTP("transactions"), th.GetAllTransactions)

	// Transactions Templates
	f.Post("/v1/organizations/:organization_id/ledgers/:ledger_id/transaction-templates", jwt.ProtectHTTP(), jwt.WithPermissionHTTP("transaction-templates"), lib.WithBody(new(t.InputDSL), th.CreateTransactionTemplate))

	// Operations
	f.Get("/v1/organizations/:organization_id/ledgers/:ledger_id/accounts/:account_id/operations", jwt.ProtectHTTP(), jwt.WithPermissionHTTP("operations"), oh.GetAllOperationsByAccount)
	f.Get("/v1/organizations/:organization_id/ledgers/:ledger_id/portfolios/:portfolio_id/operations", jwt.ProtectHTTP(), jwt.WithPermissionHTTP("operations"), oh.GetAllOperationsByPortfolio)
	// f.Get("/v1/organizations/:organization_id/ledgers/:ledger_id/portfolios/:portfolio_id/operations/:operation_id", nil)
	// f.Patch("/v1/organizations/:organization_id/ledgers/:ledger_id/transactions/:transaction_id/operations/:operation_id", nil)

	// Health
	f.Get("/health", lib.Ping)

	// Doc
	lib.DocAPI("transaction", "Transaction API", f)

	return f
}
