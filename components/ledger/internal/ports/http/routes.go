package http

import (
	"github.com/LerianStudio/midaz/common/mcasdoor"
	"github.com/LerianStudio/midaz/common/mlog"
	"github.com/LerianStudio/midaz/common/mmodel"
	"github.com/LerianStudio/midaz/common/mopentelemetry"
	lib "github.com/LerianStudio/midaz/common/net/http"
	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
)

// NewRouter registerNewRouters routes to the Server.
func NewRouter(lg mlog.Logger, tl *mopentelemetry.Telemetry, cc *mcasdoor.CasdoorConnection, ah *AccountHandler, ph *PortfolioHandler, lh *LedgerHandler, ih *AssetHandler, oh *OrganizationHandler, rh *ProductHandler) *fiber.App {
	f := fiber.New(fiber.Config{
		DisableStartupMessage: true,
	})
	tlMid := lib.NewTelemetryMiddleware(tl)

	f.Use(tlMid.WithTelemetry(tl))
	f.Use(cors.New())
	f.Use(lib.WithCorrelationID())
	f.Use(lib.WithHTTPLogging(lib.WithCustomLogger(lg)))
	jwt := lib.NewJWTMiddleware(cc)

	// Organizations
	f.Post("/v1/organizations", jwt.ProtectHTTP(), jwt.WithPermissionHTTP("organization"), lib.WithBody(new(mmodel.CreateOrganizationInput), oh.CreateOrganization))
	f.Patch("/v1/organizations/:id", jwt.ProtectHTTP(), jwt.WithPermissionHTTP("organization"), lib.ParseUUIDPathParameters, lib.WithBody(new(mmodel.UpdateOrganizationInput), oh.UpdateOrganization))
	f.Get("/v1/organizations", jwt.ProtectHTTP(), jwt.WithPermissionHTTP("organization"), oh.GetAllOrganizations)
	f.Get("/v1/organizations/:id", jwt.ProtectHTTP(), jwt.WithPermissionHTTP("organization"), lib.ParseUUIDPathParameters, oh.GetOrganizationByID)
	f.Delete("/v1/organizations/:id", jwt.ProtectHTTP(), jwt.WithPermissionHTTP("organization"), lib.ParseUUIDPathParameters, oh.DeleteOrganizationByID)

	// Ledgers
	f.Post("/v1/organizations/:organization_id/ledgers", jwt.ProtectHTTP(), jwt.WithPermissionHTTP("ledger"), lib.ParseUUIDPathParameters, lib.WithBody(new(mmodel.CreateLedgerInput), lh.CreateLedger))
	f.Patch("/v1/organizations/:organization_id/ledgers/:id", jwt.ProtectHTTP(), jwt.WithPermissionHTTP("ledger"), lib.ParseUUIDPathParameters, lib.WithBody(new(mmodel.UpdateLedgerInput), lh.UpdateLedger))
	f.Get("/v1/organizations/:organization_id/ledgers", jwt.ProtectHTTP(), jwt.WithPermissionHTTP("ledger"), lib.ParseUUIDPathParameters, lh.GetAllLedgers)
	f.Get("/v1/organizations/:organization_id/ledgers/:id", jwt.ProtectHTTP(), jwt.WithPermissionHTTP("ledger"), lib.ParseUUIDPathParameters, lh.GetLedgerByID)
	f.Delete("/v1/organizations/:organization_id/ledgers/:id", jwt.ProtectHTTP(), jwt.WithPermissionHTTP("ledger"), lib.ParseUUIDPathParameters, lh.DeleteLedgerByID)

	// Assets
	f.Post("/v1/organizations/:organization_id/ledgers/:ledger_id/assets", jwt.ProtectHTTP(), jwt.WithPermissionHTTP("asset"), lib.ParseUUIDPathParameters, lib.WithBody(new(mmodel.CreateAssetInput), ih.CreateAsset))
	f.Patch("/v1/organizations/:organization_id/ledgers/:ledger_id/assets/:id", jwt.ProtectHTTP(), jwt.WithPermissionHTTP("asset"), lib.ParseUUIDPathParameters, lib.WithBody(new(mmodel.UpdateAssetInput), ih.UpdateAsset))
	f.Get("/v1/organizations/:organization_id/ledgers/:ledger_id/assets", jwt.ProtectHTTP(), jwt.WithPermissionHTTP("asset"), lib.ParseUUIDPathParameters, ih.GetAllAssets)
	f.Get("/v1/organizations/:organization_id/ledgers/:ledger_id/assets/:id", jwt.ProtectHTTP(), jwt.WithPermissionHTTP("asset"), lib.ParseUUIDPathParameters, ih.GetAssetByID)
	f.Delete("/v1/organizations/:organization_id/ledgers/:ledger_id/assets/:id", jwt.ProtectHTTP(), jwt.WithPermissionHTTP("asset"), lib.ParseUUIDPathParameters, ih.DeleteAssetByID)

	// Portfolios
	f.Post("/v1/organizations/:organization_id/ledgers/:ledger_id/portfolios", jwt.ProtectHTTP(), jwt.WithPermissionHTTP("portfolio"), lib.ParseUUIDPathParameters, lib.WithBody(new(mmodel.CreatePortfolioInput), ph.CreatePortfolio))
	f.Patch("/v1/organizations/:organization_id/ledgers/:ledger_id/portfolios/:id", jwt.ProtectHTTP(), jwt.WithPermissionHTTP("portfolio"), lib.ParseUUIDPathParameters, lib.WithBody(new(mmodel.UpdatePortfolioInput), ph.UpdatePortfolio))
	f.Get("/v1/organizations/:organization_id/ledgers/:ledger_id/portfolios", jwt.ProtectHTTP(), jwt.WithPermissionHTTP("portfolio"), lib.ParseUUIDPathParameters, ph.GetAllPortfolios)
	f.Get("/v1/organizations/:organization_id/ledgers/:ledger_id/portfolios/:id", jwt.ProtectHTTP(), jwt.WithPermissionHTTP("portfolio"), lib.ParseUUIDPathParameters, ph.GetPortfolioByID)
	f.Delete("/v1/organizations/:organization_id/ledgers/:ledger_id/portfolios/:id", jwt.ProtectHTTP(), jwt.WithPermissionHTTP("portfolio"), lib.ParseUUIDPathParameters, ph.DeletePortfolioByID)

	// Product
	f.Post("/v1/organizations/:organization_id/ledgers/:ledger_id/products", jwt.ProtectHTTP(), jwt.WithPermissionHTTP("product"), lib.ParseUUIDPathParameters, lib.WithBody(new(mmodel.CreateProductInput), rh.CreateProduct))
	f.Patch("/v1/organizations/:organization_id/ledgers/:ledger_id/products/:id", jwt.ProtectHTTP(), jwt.WithPermissionHTTP("product"), lib.ParseUUIDPathParameters, lib.WithBody(new(mmodel.UpdateProductInput), rh.UpdateProduct))
	f.Get("/v1/organizations/:organization_id/ledgers/:ledger_id/products", jwt.ProtectHTTP(), jwt.WithPermissionHTTP("product"), lib.ParseUUIDPathParameters, rh.GetAllProducts)
	f.Get("/v1/organizations/:organization_id/ledgers/:ledger_id/products/:id", jwt.ProtectHTTP(), jwt.WithPermissionHTTP("product"), lib.ParseUUIDPathParameters, rh.GetProductByID)
	f.Delete("/v1/organizations/:organization_id/ledgers/:ledger_id/products/:id", jwt.ProtectHTTP(), jwt.WithPermissionHTTP("product"), lib.ParseUUIDPathParameters, rh.DeleteProductByID)

	// Accounts
	f.Post("/v1/organizations/:organization_id/ledgers/:ledger_id/accounts", jwt.ProtectHTTP(), jwt.WithPermissionHTTP("account"), lib.ParseUUIDPathParameters, lib.WithBody(new(mmodel.CreateAccountInput), ah.CreateAccount))
	f.Patch("/v1/organizations/:organization_id/ledgers/:ledger_id/accounts/:id", jwt.ProtectHTTP(), jwt.WithPermissionHTTP("account"), lib.ParseUUIDPathParameters, lib.WithBody(new(mmodel.UpdateAccountInput), ah.UpdateAccount))
	f.Get("/v1/organizations/:organization_id/ledgers/:ledger_id/accounts", jwt.ProtectHTTP(), jwt.WithPermissionHTTP("account"), lib.ParseUUIDPathParameters, ah.GetAllAccounts)
	f.Get("/v1/organizations/:organization_id/ledgers/:ledger_id/accounts/:id", jwt.ProtectHTTP(), jwt.WithPermissionHTTP("account"), lib.ParseUUIDPathParameters, ah.GetAccountByID)
	f.Delete("/v1/organizations/:organization_id/ledgers/:ledger_id/accounts/:id", jwt.ProtectHTTP(), jwt.WithPermissionHTTP("account"), lib.ParseUUIDPathParameters, ah.DeleteAccountByID)
	// Will be deprecated in the future. Use "POST /v1/organizations/:organization_id/ledgers/:ledger_id/accounts" instead.
	f.Post("/v1/organizations/:organization_id/ledgers/:ledger_id/portfolios/:portfolio_id/accounts", jwt.ProtectHTTP(), jwt.WithPermissionHTTP("account"), lib.ParseUUIDPathParameters, lib.WithBody(new(mmodel.CreateAccountInput), ah.CreateAccountFromPortfolio))
	// Will be deprecated in the future. Use "PATCH /v1/organizations/:organization_id/ledgers/:ledger_id/accounts/:id" instead.
	f.Patch("/v1/organizations/:organization_id/ledgers/:ledger_id/portfolios/:portfolio_id/accounts/:id", jwt.ProtectHTTP(), jwt.WithPermissionHTTP("account"), lib.ParseUUIDPathParameters, lib.WithBody(new(mmodel.UpdateAccountInput), ah.UpdateAccountFromPortfolio))
	// Will be deprecated in the future. Use "GET /v1/organizations/:organization_id/ledgers/:ledger_id/accounts" instead.
	f.Get("/v1/organizations/:organization_id/ledgers/:ledger_id/portfolios/:portfolio_id/accounts", jwt.ProtectHTTP(), jwt.WithPermissionHTTP("account"), lib.ParseUUIDPathParameters, ah.GetAllAccountsByIDFromPortfolio)
	// Will be deprecated in the future. Use "GET /v1/organizations/:organization_id/ledgers/:ledger_id/accounts/:id" instead.
	f.Get("/v1/organizations/:organization_id/ledgers/:ledger_id/portfolios/:portfolio_id/accounts/:id", jwt.ProtectHTTP(), jwt.WithPermissionHTTP("account"), lib.ParseUUIDPathParameters, ah.GetAccountByIDFromPortfolio)
	// Will be deprecated in the future. Use "DELETE /v1/organizations/:organization_id/ledgers/:ledger_id/accounts/:id" instead.
	f.Delete("/v1/organizations/:organization_id/ledgers/:ledger_id/portfolios/:portfolio_id/accounts/:id", jwt.ProtectHTTP(), jwt.WithPermissionHTTP("account"), lib.ParseUUIDPathParameters, ah.DeleteAccountByIDFromPortfolio)

	// Health
	f.Get("/health", lib.Ping)

	// Version
	f.Get("/version", lib.Version)

	// Doc
	lib.DocAPI("ledger", "Ledger API", f)

	f.Use(tlMid.EndTracingSpans)

	return f
}
