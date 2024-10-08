package http

import (
	lib "github.com/LerianStudio/midaz/common/net/http"
	t "github.com/LerianStudio/midaz/components/transaction/internal/domain/transaction"
	"github.com/LerianStudio/midaz/components/transaction/internal/service"
	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
)

func NewRouter(th *TransactionHandler) *fiber.App {
	f := fiber.New()

	_ = service.NewConfig()

	f.Use(cors.New())
	f.Use(lib.WithCorrelationID())

	// jwt := lib.NewJWTMiddleware(config.JWKAddress)

	// -- Routes --

	// Transactions
	f.Post("/v1/organizations/:organization_id/ledgers/:ledger_id/transactions", th.CreateTransaction)
	f.Post("/v1/organizations/:organization_id/ledgers/:ledger_id/transactions/:transaction_id/commit", th.CommitTransaction)
	f.Post("/v1/organizations/:organization_id/ledgers/:ledger_id/transactions/:transaction_id/revert", th.RevertTransaction)
	f.Get("/v1/organizations/:organization_id/ledgers/:ledger_id/transactions/:transaction_id", th.GetTransaction)

	// Transactions Templates
	f.Post("/v1/organizations/:organization_id/ledgers/:ledger_id/transaction-templates", lib.WithBody(new(t.InputDSL), th.CreateTransactionTemplate))

	// f.Put("/v1/organizations/:organization_id/ledgers/:ledger_id/transaction-templates/:code", nil)
	// f.Delete("/v1/organizations/:organization_id/ledgers/:ledger_id/transaction-templates/:code", nil)

	// Operations

	// f.Get("/v1/organizations/:organization_id/ledgers/:ledger_id/portfolios/:portfolio_id/operations", nil)
	// f.Get("/v1/organizations/:organization_id/ledgers/:ledger_id/portfolios/:portfolio_id/operations/:operation_id", nil)
	// f.Patch("/v1/organizations/:organization_id/ledgers/:ledger_id/transactions/:transaction_id/operations/:operation_id", nil)

	// Health
	f.Get("/health", lib.Ping)

	// Doc
	lib.DocAPI("transaction", "Transaction API", f)

	return f
}
