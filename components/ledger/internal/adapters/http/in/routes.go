package in

import (
	"github.com/LerianStudio/lib-auth/v2/auth/middleware"
	libLog "github.com/LerianStudio/lib-commons/v2/commons/log"
	libHTTP "github.com/LerianStudio/lib-commons/v2/commons/net/http"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v2/commons/opentelemetry"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/assetrate"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/operation"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/transaction"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	"github.com/LerianStudio/midaz/v4/pkg/net/http"
	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
	fiberSwagger "github.com/swaggo/fiber-swagger"
)

const (
	midazName   = "midaz"
	routingName = "routing"
)

// NewRouter register NewRouter routes to the Server.
func NewRouter(lg libLog.Logger, tl *libOpentelemetry.Telemetry, auth *middleware.AuthClient, h *Handlers) *fiber.App {
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

	// -- Routes --

	// Organizations
	f.Post("/v1/organizations", auth.Authorize(midazName, "organizations", "post"), http.WithBody(new(mmodel.CreateOrganizationInput), h.Organization.CreateOrganization))
	f.Patch("/v1/organizations/:id", auth.Authorize(midazName, "organizations", "patch"), http.ParseUUIDPathParameters("organization"), http.WithBody(new(mmodel.UpdateOrganizationInput), h.Organization.UpdateOrganization))
	f.Get("/v1/organizations", auth.Authorize(midazName, "organizations", "get"), h.Organization.GetAllOrganizations)
	f.Get("/v1/organizations/:id", auth.Authorize(midazName, "organizations", "get"), http.ParseUUIDPathParameters("organization"), h.Organization.GetOrganizationByID)
	f.Delete("/v1/organizations/:id", auth.Authorize(midazName, "organizations", "delete"), http.ParseUUIDPathParameters("organization"), h.Organization.DeleteOrganizationByID)
	f.Head("/v1/organizations/metrics/count", auth.Authorize(midazName, "organizations", "head"), h.Organization.CountOrganizations)

	// Ledgers
	f.Post("/v1/organizations/:organization_id/ledgers", auth.Authorize(midazName, "ledgers", "post"), http.ParseUUIDPathParameters("ledger"), http.WithBody(new(mmodel.CreateLedgerInput), h.Ledger.CreateLedger))
	f.Patch("/v1/organizations/:organization_id/ledgers/:id", auth.Authorize(midazName, "ledgers", "patch"), http.ParseUUIDPathParameters("ledger"), http.WithBody(new(mmodel.UpdateLedgerInput), h.Ledger.UpdateLedger))
	f.Get("/v1/organizations/:organization_id/ledgers", auth.Authorize(midazName, "ledgers", "get"), http.ParseUUIDPathParameters("ledger"), h.Ledger.GetAllLedgers)
	f.Get("/v1/organizations/:organization_id/ledgers/:id", auth.Authorize(midazName, "ledgers", "get"), http.ParseUUIDPathParameters("ledger"), h.Ledger.GetLedgerByID)
	f.Delete("/v1/organizations/:organization_id/ledgers/:id", auth.Authorize(midazName, "ledgers", "delete"), http.ParseUUIDPathParameters("ledger"), h.Ledger.DeleteLedgerByID)
	f.Head("/v1/organizations/:organization_id/ledgers/metrics/count", auth.Authorize(midazName, "ledgers", "head"), http.ParseUUIDPathParameters("ledger"), h.Ledger.CountLedgers)

	// Assets
	f.Post("/v1/organizations/:organization_id/ledgers/:ledger_id/assets", auth.Authorize(midazName, "assets", "post"), http.ParseUUIDPathParameters("asset"), http.WithBody(new(mmodel.CreateAssetInput), h.Asset.CreateAsset))
	f.Patch("/v1/organizations/:organization_id/ledgers/:ledger_id/assets/:id", auth.Authorize(midazName, "assets", "patch"), http.ParseUUIDPathParameters("asset"), http.WithBody(new(mmodel.UpdateAssetInput), h.Asset.UpdateAsset))
	f.Get("/v1/organizations/:organization_id/ledgers/:ledger_id/assets", auth.Authorize(midazName, "assets", "get"), http.ParseUUIDPathParameters("asset"), h.Asset.GetAllAssets)
	f.Get("/v1/organizations/:organization_id/ledgers/:ledger_id/assets/:id", auth.Authorize(midazName, "assets", "get"), http.ParseUUIDPathParameters("asset"), h.Asset.GetAssetByID)
	f.Delete("/v1/organizations/:organization_id/ledgers/:ledger_id/assets/:id", auth.Authorize(midazName, "assets", "delete"), http.ParseUUIDPathParameters("asset"), h.Asset.DeleteAssetByID)
	f.Head("/v1/organizations/:organization_id/ledgers/:ledger_id/assets/metrics/count", auth.Authorize(midazName, "assets", "head"), http.ParseUUIDPathParameters("asset"), h.Asset.CountAssets)

	// Portfolios
	f.Post("/v1/organizations/:organization_id/ledgers/:ledger_id/portfolios", auth.Authorize(midazName, "portfolios", "post"), http.ParseUUIDPathParameters("portfolio"), http.WithBody(new(mmodel.CreatePortfolioInput), h.Portfolio.CreatePortfolio))
	f.Patch("/v1/organizations/:organization_id/ledgers/:ledger_id/portfolios/:id", auth.Authorize(midazName, "portfolios", "patch"), http.ParseUUIDPathParameters("portfolio"), http.WithBody(new(mmodel.UpdatePortfolioInput), h.Portfolio.UpdatePortfolio))
	f.Get("/v1/organizations/:organization_id/ledgers/:ledger_id/portfolios", auth.Authorize(midazName, "portfolios", "get"), http.ParseUUIDPathParameters("portfolio"), h.Portfolio.GetAllPortfolios)
	f.Get("/v1/organizations/:organization_id/ledgers/:ledger_id/portfolios/:id", auth.Authorize(midazName, "portfolios", "get"), http.ParseUUIDPathParameters("portfolio"), h.Portfolio.GetPortfolioByID)
	f.Delete("/v1/organizations/:organization_id/ledgers/:ledger_id/portfolios/:id", auth.Authorize(midazName, "portfolios", "delete"), http.ParseUUIDPathParameters("portfolio"), h.Portfolio.DeletePortfolioByID)
	f.Head("/v1/organizations/:organization_id/ledgers/:ledger_id/portfolios/metrics/count", auth.Authorize(midazName, "portfolios", "head"), http.ParseUUIDPathParameters("portfolio"), h.Portfolio.CountPortfolios)

	// Segment
	f.Post("/v1/organizations/:organization_id/ledgers/:ledger_id/segments", auth.Authorize(midazName, "segments", "post"), http.ParseUUIDPathParameters("segment"), http.WithBody(new(mmodel.CreateSegmentInput), h.Segment.CreateSegment))
	f.Patch("/v1/organizations/:organization_id/ledgers/:ledger_id/segments/:id", auth.Authorize(midazName, "segments", "patch"), http.ParseUUIDPathParameters("segment"), http.WithBody(new(mmodel.UpdateSegmentInput), h.Segment.UpdateSegment))
	f.Get("/v1/organizations/:organization_id/ledgers/:ledger_id/segments", auth.Authorize(midazName, "segments", "get"), http.ParseUUIDPathParameters("segment"), h.Segment.GetAllSegments)
	f.Get("/v1/organizations/:organization_id/ledgers/:ledger_id/segments/:id", auth.Authorize(midazName, "segments", "get"), http.ParseUUIDPathParameters("segment"), h.Segment.GetSegmentByID)
	f.Delete("/v1/organizations/:organization_id/ledgers/:ledger_id/segments/:id", auth.Authorize(midazName, "segments", "delete"), http.ParseUUIDPathParameters("segment"), h.Segment.DeleteSegmentByID)
	f.Head("/v1/organizations/:organization_id/ledgers/:ledger_id/segments/metrics/count", auth.Authorize(midazName, "segments", "head"), http.ParseUUIDPathParameters("segment"), h.Segment.CountSegments)

	// Accounts
	f.Post("/v1/organizations/:organization_id/ledgers/:ledger_id/accounts", auth.Authorize(midazName, "accounts", "post"), http.ParseUUIDPathParameters("account"), http.WithBody(new(mmodel.CreateAccountInput), h.Account.CreateAccount))
	f.Patch("/v1/organizations/:organization_id/ledgers/:ledger_id/accounts/:id", auth.Authorize(midazName, "accounts", "patch"), http.ParseUUIDPathParameters("account"), http.WithBody(new(mmodel.UpdateAccountInput), h.Account.UpdateAccount))
	f.Get("/v1/organizations/:organization_id/ledgers/:ledger_id/accounts", auth.Authorize(midazName, "accounts", "get"), http.ParseUUIDPathParameters("account"), h.Account.GetAllAccounts)
	f.Get("/v1/organizations/:organization_id/ledgers/:ledger_id/accounts/:id", auth.Authorize(midazName, "accounts", "get"), http.ParseUUIDPathParameters("account"), h.Account.GetAccountByID)
	f.Get("/v1/organizations/:organization_id/ledgers/:ledger_id/accounts/alias/:alias", auth.Authorize(midazName, "accounts", "get"), http.ParseUUIDPathParameters("account"), h.Account.GetAccountByAlias)
	f.Get("/v1/organizations/:organization_id/ledgers/:ledger_id/accounts/external/:code", auth.Authorize(midazName, "accounts", "get"), http.ParseUUIDPathParameters("account"), h.Account.GetAccountExternalByCode)
	f.Delete("/v1/organizations/:organization_id/ledgers/:ledger_id/accounts/:id", auth.Authorize(midazName, "accounts", "delete"), http.ParseUUIDPathParameters("account"), h.Account.DeleteAccountByID)
	f.Head("/v1/organizations/:organization_id/ledgers/:ledger_id/accounts/metrics/count", auth.Authorize(midazName, "accounts", "head"), http.ParseUUIDPathParameters("account"), h.Account.CountAccounts)

	// Account Types
	f.Post("/v1/organizations/:organization_id/ledgers/:ledger_id/account-types", auth.Authorize(routingName, "account-types", "post"), http.ParseUUIDPathParameters("account_type"), http.WithBody(new(mmodel.CreateAccountTypeInput), h.AccountType.CreateAccountType))
	f.Patch("/v1/organizations/:organization_id/ledgers/:ledger_id/account-types/:id", auth.Authorize(routingName, "account-types", "patch"), http.ParseUUIDPathParameters("account_type"), http.WithBody(new(mmodel.UpdateAccountTypeInput), h.AccountType.UpdateAccountType))
	f.Get("/v1/organizations/:organization_id/ledgers/:ledger_id/account-types/:id", auth.Authorize(routingName, "account-types", "get"), http.ParseUUIDPathParameters("account_type"), h.AccountType.GetAccountTypeByID)
	f.Get("/v1/organizations/:organization_id/ledgers/:ledger_id/account-types", auth.Authorize(routingName, "account-types", "get"), http.ParseUUIDPathParameters("account_type"), h.AccountType.GetAllAccountTypes)
	f.Delete("/v1/organizations/:organization_id/ledgers/:ledger_id/account-types/:id", auth.Authorize(routingName, "account-types", "delete"), http.ParseUUIDPathParameters("account_type"), h.AccountType.DeleteAccountTypeByID)

	// Transactions
	f.Post("/v1/organizations/:organization_id/ledgers/:ledger_id/transactions/dsl", auth.Authorize(midazName, "transactions", "post"), http.ParseUUIDPathParameters("transaction"), h.Transaction.CreateTransactionDSL)
	f.Post("/v1/organizations/:organization_id/ledgers/:ledger_id/transactions/json", auth.Authorize(midazName, "transactions", "post"), http.ParseUUIDPathParameters("transaction"), http.WithBody(new(transaction.CreateTransactionInput), h.Transaction.CreateTransactionJSON))
	f.Post("/v1/organizations/:organization_id/ledgers/:ledger_id/transactions/inflow", auth.Authorize(midazName, "transactions", "post"), http.ParseUUIDPathParameters("transaction"), http.WithBody(new(transaction.CreateTransactionInflowInput), h.Transaction.CreateTransactionInflow))
	f.Post("/v1/organizations/:organization_id/ledgers/:ledger_id/transactions/outflow", auth.Authorize(midazName, "transactions", "post"), http.ParseUUIDPathParameters("transaction"), http.WithBody(new(transaction.CreateTransactionOutflowInput), h.Transaction.CreateTransactionOutflow))
	f.Post("/v1/organizations/:organization_id/ledgers/:ledger_id/transactions/annotation", auth.Authorize(midazName, "transactions", "post"), http.ParseUUIDPathParameters("transaction"), http.WithBody(new(transaction.CreateTransactionInput), h.Transaction.CreateTransactionAnnotation))

	f.Post("/v1/organizations/:organization_id/ledgers/:ledger_id/transactions/:transaction_id/commit", auth.Authorize(midazName, "transactions", "post"), http.ParseUUIDPathParameters("transaction"), h.Transaction.CommitTransaction)
	f.Post("/v1/organizations/:organization_id/ledgers/:ledger_id/transactions/:transaction_id/cancel", auth.Authorize(midazName, "transactions", "post"), http.ParseUUIDPathParameters("transaction"), h.Transaction.CancelTransaction)
	f.Post("/v1/organizations/:organization_id/ledgers/:ledger_id/transactions/:transaction_id/revert", auth.Authorize(midazName, "transactions", "post"), http.ParseUUIDPathParameters("transaction"), h.Transaction.RevertTransaction)

	f.Patch("/v1/organizations/:organization_id/ledgers/:ledger_id/transactions/:transaction_id", auth.Authorize(midazName, "transactions", "patch"), http.ParseUUIDPathParameters("transaction"), http.WithBody(new(transaction.UpdateTransactionInput), h.Transaction.UpdateTransaction))

	f.Get("/v1/organizations/:organization_id/ledgers/:ledger_id/transactions/:transaction_id", auth.Authorize(midazName, "transactions", "get"), http.ParseUUIDPathParameters("transaction"), h.Transaction.GetTransaction)
	f.Get("/v1/organizations/:organization_id/ledgers/:ledger_id/transactions", auth.Authorize(midazName, "transactions", "get"), http.ParseUUIDPathParameters("transaction"), h.Transaction.GetAllTransactions)

	// Operations
	f.Get("/v1/organizations/:organization_id/ledgers/:ledger_id/accounts/:account_id/operations", auth.Authorize(midazName, "operations", "get"), http.ParseUUIDPathParameters("operation"), h.Operation.GetAllOperationsByAccount)
	f.Get("/v1/organizations/:organization_id/ledgers/:ledger_id/accounts/:account_id/operations/:operation_id", auth.Authorize(midazName, "operations", "get"), http.ParseUUIDPathParameters("operation"), h.Operation.GetOperationByAccount)
	f.Patch("/v1/organizations/:organization_id/ledgers/:ledger_id/transactions/:transaction_id/operations/:operation_id", auth.Authorize(midazName, "operations", "patch"), http.ParseUUIDPathParameters("operation"), http.WithBody(new(operation.UpdateOperationInput), h.Operation.UpdateOperation))

	// Asset-rate
	f.Put("/v1/organizations/:organization_id/ledgers/:ledger_id/asset-rates", auth.Authorize(midazName, "asset-rates", "put"), http.ParseUUIDPathParameters("asset-rate"), http.WithBody(new(assetrate.CreateAssetRateInput), h.AssetRate.CreateOrUpdateAssetRate))
	f.Get("/v1/organizations/:organization_id/ledgers/:ledger_id/asset-rates/:external_id", auth.Authorize(midazName, "asset-rates", "get"), http.ParseUUIDPathParameters("asset-rate"), h.AssetRate.GetAssetRateByExternalID)
	f.Get("/v1/organizations/:organization_id/ledgers/:ledger_id/asset-rates/from/:asset_code", auth.Authorize(midazName, "asset-rates", "get"), http.ParseUUIDPathParameters("asset-rate"), h.AssetRate.GetAllAssetRatesByAssetCode)

	// Balance
	f.Patch("/v1/organizations/:organization_id/ledgers/:ledger_id/balances/:balance_id", auth.Authorize(midazName, "balances", "patch"), http.ParseUUIDPathParameters("balance"), http.WithBody(new(mmodel.UpdateBalance), h.Balance.UpdateBalance))
	f.Delete("/v1/organizations/:organization_id/ledgers/:ledger_id/balances/:balance_id", auth.Authorize(midazName, "balances", "delete"), http.ParseUUIDPathParameters("balance"), h.Balance.DeleteBalanceByID)
	f.Get("/v1/organizations/:organization_id/ledgers/:ledger_id/balances", auth.Authorize(midazName, "balances", "get"), http.ParseUUIDPathParameters("balance"), h.Balance.GetAllBalances)
	f.Get("/v1/organizations/:organization_id/ledgers/:ledger_id/balances/:balance_id", auth.Authorize(midazName, "balances", "get"), http.ParseUUIDPathParameters("balance"), h.Balance.GetBalanceByID)
	f.Get("/v1/organizations/:organization_id/ledgers/:ledger_id/accounts/:account_id/balances", auth.Authorize(midazName, "balances", "get"), http.ParseUUIDPathParameters("balance"), h.Balance.GetAllBalancesByAccountID)
	f.Get("/v1/organizations/:organization_id/ledgers/:ledger_id/accounts/alias/:alias/balances", auth.Authorize(midazName, "balances", "get"), http.ParseUUIDPathParameters("balance"), h.Balance.GetBalancesByAlias)
	f.Get("/v1/organizations/:organization_id/ledgers/:ledger_id/accounts/external/:code/balances", auth.Authorize(midazName, "balances", "get"), http.ParseUUIDPathParameters("balance"), h.Balance.GetBalancesExternalByCode)
	f.Post("/v1/organizations/:organization_id/ledgers/:ledger_id/accounts/:account_id/balances", auth.Authorize(midazName, "balances", "post"), http.ParseUUIDPathParameters("balance"), http.WithBody(new(mmodel.CreateAdditionalBalance), h.Balance.CreateAdditionalBalance))

	// Operation-route
	f.Post("/v1/organizations/:organization_id/ledgers/:ledger_id/operation-routes", auth.Authorize(routingName, "operation-routes", "post"), http.ParseUUIDPathParameters("operation_route"), http.WithBody(new(mmodel.CreateOperationRouteInput), h.OperationRoute.CreateOperationRoute))
	f.Get("/v1/organizations/:organization_id/ledgers/:ledger_id/operation-routes/:operation_route_id", auth.Authorize(routingName, "operation-routes", "get"), http.ParseUUIDPathParameters("operation_route"), h.OperationRoute.GetOperationRouteByID)
	f.Patch("/v1/organizations/:organization_id/ledgers/:ledger_id/operation-routes/:operation_route_id", auth.Authorize(routingName, "operation-routes", "patch"), http.ParseUUIDPathParameters("operation_route"), http.WithBody(new(mmodel.UpdateOperationRouteInput), h.OperationRoute.UpdateOperationRoute))
	f.Delete("/v1/organizations/:organization_id/ledgers/:ledger_id/operation-routes/:operation_route_id", auth.Authorize(routingName, "operation-routes", "delete"), http.ParseUUIDPathParameters("operation_route"), h.OperationRoute.DeleteOperationRouteByID)
	f.Get("/v1/organizations/:organization_id/ledgers/:ledger_id/operation-routes", auth.Authorize(routingName, "operation-routes", "get"), http.ParseUUIDPathParameters("operation_route"), h.OperationRoute.GetAllOperationRoutes)

	// Transaction-route
	f.Post("/v1/organizations/:organization_id/ledgers/:ledger_id/transaction-routes", auth.Authorize(routingName, "transaction-routes", "post"), http.ParseUUIDPathParameters("transaction_route"), http.WithBody(new(mmodel.CreateTransactionRouteInput), h.TransactionRoute.CreateTransactionRoute))
	f.Get("/v1/organizations/:organization_id/ledgers/:ledger_id/transaction-routes/:transaction_route_id", auth.Authorize(routingName, "transaction-routes", "get"), http.ParseUUIDPathParameters("transaction_route"), h.TransactionRoute.GetTransactionRouteByID)
	f.Patch("/v1/organizations/:organization_id/ledgers/:ledger_id/transaction-routes/:transaction_route_id", auth.Authorize(routingName, "transaction-routes", "patch"), http.ParseUUIDPathParameters("transaction_route"), http.WithBody(new(mmodel.UpdateTransactionRouteInput), h.TransactionRoute.UpdateTransactionRoute))
	f.Delete("/v1/organizations/:organization_id/ledgers/:ledger_id/transaction-routes/:transaction_route_id", auth.Authorize(routingName, "transaction-routes", "delete"), http.ParseUUIDPathParameters("transaction_route"), h.TransactionRoute.DeleteTransactionRouteByID)
	f.Get("/v1/organizations/:organization_id/ledgers/:ledger_id/transaction-routes", auth.Authorize(routingName, "transaction-routes", "get"), http.ParseUUIDPathParameters("transaction_route"), h.TransactionRoute.GetAllTransactionRoutes)

	// Health
	f.Get("/health", libHTTP.Ping)

	// Version
	f.Get("/version", libHTTP.Version)

	// Doc
	f.Get("/swagger/*", WithSwaggerEnvConfig(), fiberSwagger.WrapHandler)

	f.Use(tlMid.EndTracingSpans)

	return f
}
