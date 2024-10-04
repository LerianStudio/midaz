package http

import (
	"github.com/LerianStudio/midaz/common/mcasdoor"
	lib "github.com/LerianStudio/midaz/common/net/http"
	l "github.com/LerianStudio/midaz/components/ledger/internal/domain/onboarding/ledger"
	o "github.com/LerianStudio/midaz/components/ledger/internal/domain/onboarding/organization"
	a "github.com/LerianStudio/midaz/components/ledger/internal/domain/portfolio/account"
	s "github.com/LerianStudio/midaz/components/ledger/internal/domain/portfolio/asset"
	p "github.com/LerianStudio/midaz/components/ledger/internal/domain/portfolio/portfolio"
	r "github.com/LerianStudio/midaz/components/ledger/internal/domain/portfolio/product"
	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
)

// NewRouter registerNewRouters routes to the Server.
func NewRouter(cc *mcasdoor.CasdoorConnection, ah *AccountHandler, ph *PortfolioHandler, lh *LedgerHandler, ih *AssetHandler, oh *OrganizationHandler, rh *ProductHandler) *fiber.App {
	f := fiber.New(fiber.Config{
		DisableStartupMessage: true,
	})

	f.Use(cors.New())
	f.Use(lib.WithCorrelationID())
	jwt := lib.NewJWTMiddleware(cc)

	// Organizations
	f.Post("/v1/organizations", jwt.ProtectHTTP(), jwt.WithPermissionHTTP("organization"), lib.WithBody(new(o.CreateOrganizationInput), oh.CreateOrganization))
	f.Patch("/v1/organizations/:id", jwt.ProtectHTTP(), jwt.WithPermissionHTTP("organization"), lib.WithBody(new(o.UpdateOrganizationInput), oh.UpdateOrganization))
	f.Get("/v1/organizations", jwt.ProtectHTTP(), jwt.WithPermissionHTTP("organization"), oh.GetAllOrganizations)
	f.Get("/v1/organizations/:id", jwt.ProtectHTTP(), jwt.WithPermissionHTTP("organization"), oh.GetOrganizationByID)
	f.Delete("/v1/organizations/:id", jwt.ProtectHTTP(), jwt.WithPermissionHTTP("organization"), oh.DeleteOrganizationByID)

	// Ledgers
	f.Post("/v1/organizations/:organization_id/ledgers", jwt.ProtectHTTP(), jwt.WithPermissionHTTP("ledger"), lib.WithBody(new(l.CreateLedgerInput), lh.CreateLedger))
	f.Patch("/v1/organizations/:organization_id/ledgers/:id", jwt.ProtectHTTP(), jwt.WithPermissionHTTP("ledger"), lib.WithBody(new(l.UpdateLedgerInput), lh.UpdateLedger))
	f.Get("/v1/organizations/:organization_id/ledgers", jwt.ProtectHTTP(), jwt.WithPermissionHTTP("ledger"), lh.GetAllLedgers)
	f.Get("/v1/organizations/:organization_id/ledgers/:id", jwt.ProtectHTTP(), jwt.WithPermissionHTTP("ledger"), lh.GetLedgerByID)
	f.Delete("/v1/organizations/:organization_id/ledgers/:id", jwt.ProtectHTTP(), jwt.WithPermissionHTTP("ledger"), lh.DeleteLedgerByID)

	// Assets
	f.Post("/v1/organizations/:organization_id/ledgers/:ledger_id/assets", jwt.ProtectHTTP(), jwt.WithPermissionHTTP("asset"), lib.WithBody(new(s.CreateAssetInput), ih.CreateAsset))
	f.Patch("/v1/organizations/:organization_id/ledgers/:ledger_id/assets/:id", jwt.ProtectHTTP(), jwt.WithPermissionHTTP("asset"), lib.WithBody(new(s.UpdateAssetInput), ih.UpdateAsset))
	f.Get("/v1/organizations/:organization_id/ledgers/:ledger_id/assets", jwt.ProtectHTTP(), jwt.WithPermissionHTTP("asset"), ih.GetAllAssets)
	f.Get("/v1/organizations/:organization_id/ledgers/:ledger_id/assets/:id", jwt.ProtectHTTP(), jwt.WithPermissionHTTP("asset"), ih.GetAssetByID)
	f.Delete("/v1/organizations/:organization_id/ledgers/:ledger_id/assets/:id", jwt.ProtectHTTP(), jwt.WithPermissionHTTP("asset"), ih.DeleteAssetByID)

	// Portfolios
	f.Post("/v1/organizations/:organization_id/ledgers/:ledger_id/portfolios", jwt.ProtectHTTP(), jwt.WithPermissionHTTP("portfolio"), lib.WithBody(new(p.CreatePortfolioInput), ph.CreatePortfolio))
	f.Patch("/v1/organizations/:organization_id/ledgers/:ledger_id/portfolios/:id", jwt.ProtectHTTP(), jwt.WithPermissionHTTP("portfolio"), lib.WithBody(new(p.UpdatePortfolioInput), ph.UpdatePortfolio))
	f.Get("/v1/organizations/:organization_id/ledgers/:ledger_id/portfolios", jwt.ProtectHTTP(), jwt.WithPermissionHTTP("portfolio"), ph.GetAllPortfolios)
	f.Get("/v1/organizations/:organization_id/ledgers/:ledger_id/portfolios/:id", jwt.ProtectHTTP(), jwt.WithPermissionHTTP("portfolio"), ph.GetPortfolioByID)
	f.Delete("/v1/organizations/:organization_id/ledgers/:ledger_id/portfolios/:id", jwt.ProtectHTTP(), jwt.WithPermissionHTTP("portfolio"), ph.DeletePortfolioByID)

	// Product
	f.Post("/v1/organizations/:organization_id/ledgers/:ledger_id/products", jwt.ProtectHTTP(), jwt.WithPermissionHTTP("product"), lib.WithBody(new(r.CreateProductInput), rh.CreateProduct))
	f.Patch("/v1/organizations/:organization_id/ledgers/:ledger_id/products/:id", jwt.ProtectHTTP(), jwt.WithPermissionHTTP("product"), lib.WithBody(new(r.UpdateProductInput), rh.UpdateProduct))
	f.Get("/v1/organizations/:organization_id/ledgers/:ledger_id/products", jwt.ProtectHTTP(), jwt.WithPermissionHTTP("product"), rh.GetAllProducts)
	f.Get("/v1/organizations/:organization_id/ledgers/:ledger_id/products/:id", jwt.ProtectHTTP(), jwt.WithPermissionHTTP("product"), rh.GetProductByID)
	f.Delete("/v1/organizations/:organization_id/ledgers/:ledger_id/products/:id", jwt.ProtectHTTP(), jwt.WithPermissionHTTP("product"), rh.DeleteProductByID)

	// Accounts
	f.Post("/v1/organizations/:organization_id/ledgers/:ledger_id/portfolios/:portfolio_id/accounts", jwt.ProtectHTTP(), jwt.WithPermissionHTTP("account"), lib.WithBody(new(a.CreateAccountInput), ah.CreateAccount))
	f.Patch("/v1/organizations/:organization_id/ledgers/:ledger_id/portfolios/:portfolio_id/accounts/:id", jwt.ProtectHTTP(), jwt.WithPermissionHTTP("account"), lib.WithBody(new(a.UpdateAccountInput), ah.UpdateAccount))
	f.Get("/v1/organizations/:organization_id/ledgers/:ledger_id/portfolios/:portfolio_id/accounts", jwt.ProtectHTTP(), jwt.WithPermissionHTTP("account"), ah.GetAllAccounts)
	f.Get("/v1/organizations/:organization_id/ledgers/:ledger_id/portfolios/:portfolio_id/accounts/:id", jwt.ProtectHTTP(), jwt.WithPermissionHTTP("account"), ah.GetAccountByID)
	f.Delete("/v1/organizations/:organization_id/ledgers/:ledger_id/portfolios/:portfolio_id/accounts/:id", jwt.ProtectHTTP(), jwt.WithPermissionHTTP("account"), ah.DeleteAccountByID)

	// Health
	f.Get("/health", lib.Ping)

	// Doc
	lib.DocAPI("ledger", "Ledger API", f)

	return f
}
