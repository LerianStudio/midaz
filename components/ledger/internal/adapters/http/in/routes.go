// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"github.com/LerianStudio/lib-auth/v2/auth/middleware"
	libLog "github.com/LerianStudio/lib-commons/v4/commons/log"
	libHTTP "github.com/LerianStudio/lib-commons/v4/commons/net/http"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v4/commons/opentelemetry"
	_ "github.com/LerianStudio/midaz/v3/components/ledger/api"
	"github.com/LerianStudio/midaz/v3/components/ledger/internal/adapters/postgres/assetrate"
	"github.com/LerianStudio/midaz/v3/components/ledger/internal/adapters/postgres/operation"
	"github.com/LerianStudio/midaz/v3/components/ledger/internal/adapters/postgres/transaction"
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

// SettingsMaxPayloadSize defines the maximum payload size for settings endpoints (64KB).
const SettingsMaxPayloadSize = 64 * 1024

// NewRouter registers routes for the ledger component HTTP server.
func NewRouter(lg libLog.Logger, tl *libOpentelemetry.Telemetry, auth *middleware.AuthClient, mdi *MetadataIndexHandler) *fiber.App {
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
	// Register metadata index routes
	RegisterMetadataRoutesToApp(f, auth, mdi, nil)

	// Health
	f.Get("/health", libHTTP.Ping)

	// Version
	f.Get("/version", libHTTP.Version)

	// Doc
	f.Get("/swagger/*", WithSwaggerEnvConfig(), fiberSwagger.FiberWrapHandler(
		fiberSwagger.InstanceName("ledger"),
	))

	f.Use(tlMid.EndTracingSpans)

	return f
}

// RegisterMetadataRoutesToApp registers ledger routes (metadata indexes) to an existing Fiber app.
// This is used by the unified ledger server to consolidate all routes in a single port.
func RegisterMetadataRoutesToApp(f fiber.Router, auth *middleware.AuthClient, mdi *MetadataIndexHandler, routeOptions *http.ProtectedRouteOptions) {
	// Metadata Indexes
	f.Post("/v1/settings/metadata-indexes/entities/:entity_name",
		http.ProtectedRouteChain(
			auth.Authorize(midazName, "settings", "post"),
			routeOptions,
			http.WithBody(new(mmodel.CreateMetadataIndexInput), mdi.CreateMetadataIndex),
		)...)

	f.Get("/v1/settings/metadata-indexes",
		http.ProtectedRouteChain(
			auth.Authorize(midazName, "settings", "get"),
			routeOptions,
			mdi.GetAllMetadataIndexes,
		)...)

	f.Delete("/v1/settings/metadata-indexes/entities/:entity_name/key/:index_key",
		http.ProtectedRouteChain(
			auth.Authorize(midazName, "settings", "delete"),
			routeOptions,
			mdi.DeleteMetadataIndex,
		)...)
}

// CreateRouteRegistrar returns a function that registers ledger routes to an existing Fiber app.
// This is used by the unified ledger server to consolidate all routes in a single port.
func CreateRouteRegistrar(auth *middleware.AuthClient, mdi *MetadataIndexHandler, routeOptions *http.ProtectedRouteOptions) func(fiber.Router) {
	return func(fiberRouter fiber.Router) {
		RegisterMetadataRoutesToApp(fiberRouter, auth, mdi, routeOptions)
	}
}

// RegisterOnboardingRoutesToApp registers onboarding routes to an existing Fiber app.
// This is used by the unified ledger server to consolidate all routes in a single port.
// The app should already have middleware configured (telemetry, cors, logging).
func RegisterOnboardingRoutesToApp(f fiber.Router, auth *middleware.AuthClient, ah *AccountHandler, ph *PortfolioHandler, lh *LedgerHandler, ih *AssetHandler, oh *OrganizationHandler, sh *SegmentHandler, ath *AccountTypeHandler, routeOptions *http.ProtectedRouteOptions) {
	// Organizations
	f.Post("/v1/organizations", protectedMidaz(auth, "organizations", "post", routeOptions, http.WithBody(new(mmodel.CreateOrganizationInput), oh.CreateOrganization))...)
	f.Patch("/v1/organizations/:id", protectedMidaz(auth, "organizations", "patch", routeOptions, http.ParseUUIDPathParameters("organization"), http.WithBody(new(mmodel.UpdateOrganizationInput), oh.UpdateOrganization))...)
	f.Get("/v1/organizations", protectedMidaz(auth, "organizations", "get", routeOptions, oh.GetAllOrganizations)...)
	f.Get("/v1/organizations/:id", protectedMidaz(auth, "organizations", "get", routeOptions, http.ParseUUIDPathParameters("organization"), oh.GetOrganizationByID)...)
	f.Delete("/v1/organizations/:id", protectedMidaz(auth, "organizations", "delete", routeOptions, http.ParseUUIDPathParameters("organization"), oh.DeleteOrganizationByID)...)
	f.Head("/v1/organizations/metrics/count", protectedMidaz(auth, "organizations", "head", routeOptions, oh.CountOrganizations)...)

	// Ledgers
	f.Post("/v1/organizations/:organization_id/ledgers", protectedMidaz(auth, "ledgers", "post", routeOptions, http.ParseUUIDPathParameters("ledger"), http.WithBody(new(mmodel.CreateLedgerInput), lh.CreateLedger))...)
	f.Patch("/v1/organizations/:organization_id/ledgers/:id", protectedMidaz(auth, "ledgers", "patch", routeOptions, http.ParseUUIDPathParameters("ledger"), http.WithBody(new(mmodel.UpdateLedgerInput), lh.UpdateLedger))...)
	f.Get("/v1/organizations/:organization_id/ledgers", protectedMidaz(auth, "ledgers", "get", routeOptions, http.ParseUUIDPathParameters("ledger"), lh.GetAllLedgers)...)
	f.Get("/v1/organizations/:organization_id/ledgers/:id", protectedMidaz(auth, "ledgers", "get", routeOptions, http.ParseUUIDPathParameters("ledger"), lh.GetLedgerByID)...)
	f.Get("/v1/organizations/:organization_id/ledgers/:id/settings", protectedMidaz(auth, "ledgers", "get", routeOptions, http.ParseUUIDPathParameters("ledger"), lh.GetLedgerSettings)...)
	f.Patch("/v1/organizations/:organization_id/ledgers/:id/settings", protectedMidaz(auth, "ledgers", "patch", routeOptions, http.ParseUUIDPathParameters("ledger"), http.WithBodyLimit(SettingsMaxPayloadSize), http.WithBody(new(map[string]any), lh.UpdateLedgerSettings))...)
	f.Delete("/v1/organizations/:organization_id/ledgers/:id", protectedMidaz(auth, "ledgers", "delete", routeOptions, http.ParseUUIDPathParameters("ledger"), lh.DeleteLedgerByID)...)
	f.Head("/v1/organizations/:organization_id/ledgers/metrics/count", protectedMidaz(auth, "ledgers", "head", routeOptions, http.ParseUUIDPathParameters("ledger"), lh.CountLedgers)...)

	// Assets
	f.Post("/v1/organizations/:organization_id/ledgers/:ledger_id/assets", protectedMidaz(auth, "assets", "post", routeOptions, http.ParseUUIDPathParameters("asset"), http.WithBody(new(mmodel.CreateAssetInput), ih.CreateAsset))...)
	f.Patch("/v1/organizations/:organization_id/ledgers/:ledger_id/assets/:id", protectedMidaz(auth, "assets", "patch", routeOptions, http.ParseUUIDPathParameters("asset"), http.WithBody(new(mmodel.UpdateAssetInput), ih.UpdateAsset))...)
	f.Get("/v1/organizations/:organization_id/ledgers/:ledger_id/assets", protectedMidaz(auth, "assets", "get", routeOptions, http.ParseUUIDPathParameters("asset"), ih.GetAllAssets)...)
	f.Get("/v1/organizations/:organization_id/ledgers/:ledger_id/assets/:id", protectedMidaz(auth, "assets", "get", routeOptions, http.ParseUUIDPathParameters("asset"), ih.GetAssetByID)...)
	f.Delete("/v1/organizations/:organization_id/ledgers/:ledger_id/assets/:id", protectedMidaz(auth, "assets", "delete", routeOptions, http.ParseUUIDPathParameters("asset"), ih.DeleteAssetByID)...)
	f.Head("/v1/organizations/:organization_id/ledgers/:ledger_id/assets/metrics/count", protectedMidaz(auth, "assets", "head", routeOptions, http.ParseUUIDPathParameters("asset"), ih.CountAssets)...)

	// Portfolios
	f.Post("/v1/organizations/:organization_id/ledgers/:ledger_id/portfolios", protectedMidaz(auth, "portfolios", "post", routeOptions, http.ParseUUIDPathParameters("portfolio"), http.WithBody(new(mmodel.CreatePortfolioInput), ph.CreatePortfolio))...)
	f.Patch("/v1/organizations/:organization_id/ledgers/:ledger_id/portfolios/:id", protectedMidaz(auth, "portfolios", "patch", routeOptions, http.ParseUUIDPathParameters("portfolio"), http.WithBody(new(mmodel.UpdatePortfolioInput), ph.UpdatePortfolio))...)
	f.Get("/v1/organizations/:organization_id/ledgers/:ledger_id/portfolios", protectedMidaz(auth, "portfolios", "get", routeOptions, http.ParseUUIDPathParameters("portfolio"), ph.GetAllPortfolios)...)
	f.Get("/v1/organizations/:organization_id/ledgers/:ledger_id/portfolios/:id", protectedMidaz(auth, "portfolios", "get", routeOptions, http.ParseUUIDPathParameters("portfolio"), ph.GetPortfolioByID)...)
	f.Delete("/v1/organizations/:organization_id/ledgers/:ledger_id/portfolios/:id", protectedMidaz(auth, "portfolios", "delete", routeOptions, http.ParseUUIDPathParameters("portfolio"), ph.DeletePortfolioByID)...)
	f.Head("/v1/organizations/:organization_id/ledgers/:ledger_id/portfolios/metrics/count", protectedMidaz(auth, "portfolios", "head", routeOptions, http.ParseUUIDPathParameters("portfolio"), ph.CountPortfolios)...)

	// Segment
	f.Post("/v1/organizations/:organization_id/ledgers/:ledger_id/segments", protectedMidaz(auth, "segments", "post", routeOptions, http.ParseUUIDPathParameters("segment"), http.WithBody(new(mmodel.CreateSegmentInput), sh.CreateSegment))...)
	f.Patch("/v1/organizations/:organization_id/ledgers/:ledger_id/segments/:id", protectedMidaz(auth, "segments", "patch", routeOptions, http.ParseUUIDPathParameters("segment"), http.WithBody(new(mmodel.UpdateSegmentInput), sh.UpdateSegment))...)
	f.Get("/v1/organizations/:organization_id/ledgers/:ledger_id/segments", protectedMidaz(auth, "segments", "get", routeOptions, http.ParseUUIDPathParameters("segment"), sh.GetAllSegments)...)
	f.Get("/v1/organizations/:organization_id/ledgers/:ledger_id/segments/:id", protectedMidaz(auth, "segments", "get", routeOptions, http.ParseUUIDPathParameters("segment"), sh.GetSegmentByID)...)
	f.Delete("/v1/organizations/:organization_id/ledgers/:ledger_id/segments/:id", protectedMidaz(auth, "segments", "delete", routeOptions, http.ParseUUIDPathParameters("segment"), sh.DeleteSegmentByID)...)
	f.Head("/v1/organizations/:organization_id/ledgers/:ledger_id/segments/metrics/count", protectedMidaz(auth, "segments", "head", routeOptions, http.ParseUUIDPathParameters("segment"), sh.CountSegments)...)

	// Accounts
	f.Post("/v1/organizations/:organization_id/ledgers/:ledger_id/accounts", protectedMidaz(auth, "accounts", "post", routeOptions, http.ParseUUIDPathParameters("account"), http.WithBody(new(mmodel.CreateAccountInput), ah.CreateAccount))...)
	f.Patch("/v1/organizations/:organization_id/ledgers/:ledger_id/accounts/:id", protectedMidaz(auth, "accounts", "patch", routeOptions, http.ParseUUIDPathParameters("account"), http.WithBody(new(mmodel.UpdateAccountInput), ah.UpdateAccount))...)
	f.Get("/v1/organizations/:organization_id/ledgers/:ledger_id/accounts", protectedMidaz(auth, "accounts", "get", routeOptions, http.ParseUUIDPathParameters("account"), ah.GetAllAccounts)...)
	f.Get("/v1/organizations/:organization_id/ledgers/:ledger_id/accounts/:id", protectedMidaz(auth, "accounts", "get", routeOptions, http.ParseUUIDPathParameters("account"), ah.GetAccountByID)...)
	f.Get("/v1/organizations/:organization_id/ledgers/:ledger_id/accounts/alias/:alias", protectedMidaz(auth, "accounts", "get", routeOptions, http.ParseUUIDPathParameters("account"), ah.GetAccountByAlias)...)
	f.Get("/v1/organizations/:organization_id/ledgers/:ledger_id/accounts/external/:code", protectedMidaz(auth, "accounts", "get", routeOptions, http.ParseUUIDPathParameters("account"), ah.GetAccountExternalByCode)...)
	f.Delete("/v1/organizations/:organization_id/ledgers/:ledger_id/accounts/:id", protectedMidaz(auth, "accounts", "delete", routeOptions, http.ParseUUIDPathParameters("account"), ah.DeleteAccountByID)...)
	f.Head("/v1/organizations/:organization_id/ledgers/:ledger_id/accounts/metrics/count", protectedMidaz(auth, "accounts", "head", routeOptions, http.ParseUUIDPathParameters("account"), ah.CountAccounts)...)

	// Account Types
	f.Post("/v1/organizations/:organization_id/ledgers/:ledger_id/account-types", protectedRouting(auth, "account-types", "post", routeOptions, http.ParseUUIDPathParameters("account_type"), http.WithBody(new(mmodel.CreateAccountTypeInput), ath.CreateAccountType))...)
	f.Patch("/v1/organizations/:organization_id/ledgers/:ledger_id/account-types/:id", protectedRouting(auth, "account-types", "patch", routeOptions, http.ParseUUIDPathParameters("account_type"), http.WithBody(new(mmodel.UpdateAccountTypeInput), ath.UpdateAccountType))...)
	f.Get("/v1/organizations/:organization_id/ledgers/:ledger_id/account-types/:id", protectedRouting(auth, "account-types", "get", routeOptions, http.ParseUUIDPathParameters("account_type"), ath.GetAccountTypeByID)...)
	f.Get("/v1/organizations/:organization_id/ledgers/:ledger_id/account-types", protectedRouting(auth, "account-types", "get", routeOptions, http.ParseUUIDPathParameters("account_type"), ath.GetAllAccountTypes)...)
	f.Delete("/v1/organizations/:organization_id/ledgers/:ledger_id/account-types/:id", protectedRouting(auth, "account-types", "delete", routeOptions, http.ParseUUIDPathParameters("account_type"), ath.DeleteAccountTypeByID)...)
}

// RegisterTransactionRoutesToApp registers transaction routes to an existing Fiber app.
// This is used by the unified ledger server to consolidate all routes in a single port.
// The app should already have middleware configured (telemetry, cors, logging).
func RegisterTransactionRoutesToApp(f fiber.Router, auth *middleware.AuthClient, th *TransactionHandler, oh *OperationHandler, ah *AssetRateHandler, bh *BalanceHandler, orh *OperationRouteHandler, trh *TransactionRouteHandler, routeOptions *http.ProtectedRouteOptions) {
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

	f.Head("/v1/organizations/:organization_id/ledgers/:ledger_id/transactions/metrics/count", protectedMidaz(auth, "transactions", "head", routeOptions, http.ParseUUIDPathParameters("transaction"), th.CountTransactionsByFilters)...)

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
