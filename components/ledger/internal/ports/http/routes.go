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
	orgRoutes := f.Group("/v1/organizations", jwt.Protect(), jwt.WithPermission("organization"))
	orgRoutes.Post("/", lib.WithBody(new(o.CreateOrganizationInput), oh.CreateOrganization))
	orgRoutes.Patch("/:id", lib.WithBody(new(o.UpdateOrganizationInput), oh.UpdateOrganization))
	orgRoutes.Get("/", oh.GetAllOrganizations)
	orgRoutes.Get("/:id", oh.GetOrganizationByID)
	orgRoutes.Delete("/:id", oh.DeleteOrganizationByID)

	// Ledgers
	ledgerRoutes := f.Group("/v1/organizations/:organization_id/ledgers", jwt.Protect())
	ledgerRoutes.Post("/", lib.WithBody(new(l.CreateLedgerInput), lh.CreateLedger))
	ledgerRoutes.Patch("/:id", lib.WithBody(new(l.UpdateLedgerInput), lh.UpdateLedger))
	ledgerRoutes.Get("/", lh.GetAllLedgers)
	ledgerRoutes.Get("/:id", lh.GetLedgerByID)
	ledgerRoutes.Delete("/:id", lh.DeleteLedgerByID)

	// Assets
	assetRoutes := f.Group("/v1/organizations/:organization_id/ledgers/:ledger_id/assets", jwt.Protect())
	assetRoutes.Post("/", lib.WithBody(new(s.CreateAssetInput), ih.CreateAsset))
	assetRoutes.Patch("/:id", lib.WithBody(new(s.UpdateAssetInput), ih.UpdateAsset))
	assetRoutes.Get("/", ih.GetAllAssets)
	assetRoutes.Get("/:id", ih.GetAssetByID)
	assetRoutes.Delete("/:id", ih.DeleteAssetByID)

	// Portfolios
	portfolioRoutes := f.Group("/v1/organizations/:organization_id/ledgers/:ledger_id/portfolios", jwt.Protect())
	portfolioRoutes.Post("/", lib.WithBody(new(p.CreatePortfolioInput), ph.CreatePortfolio))
	portfolioRoutes.Patch("/:id", lib.WithBody(new(p.UpdatePortfolioInput), ph.UpdatePortfolio))
	portfolioRoutes.Get("/", ph.GetAllPortfolios)
	portfolioRoutes.Get("/:id", ph.GetPortfolioByID)
	portfolioRoutes.Delete("/:id", ph.DeletePortfolioByID)

	// Product
	productRoutes := f.Group("/v1/organizations/:organization_id/ledgers/:ledger_id/products", jwt.Protect())
	productRoutes.Post("/", lib.WithBody(new(r.CreateProductInput), rh.CreateProduct))
	productRoutes.Patch("/:id", lib.WithBody(new(r.UpdateProductInput), rh.UpdateProduct))
	productRoutes.Get("/", rh.GetAllProducts)
	productRoutes.Get("/:id", rh.GetProductByID)
	productRoutes.Delete("/:id", rh.DeleteProductByID)

	// Accounts
	accountRoutes := f.Group("/v1/organizations/:organization_id/ledgers/:ledger_id/portfolios/:portfolio_id/accounts", jwt.Protect())
	accountRoutes.Post("/", lib.WithBody(new(a.CreateAccountInput), ah.CreateAccount))
	accountRoutes.Patch("/:id", lib.WithBody(new(a.UpdateAccountInput), ah.UpdateAccount))
	accountRoutes.Get("/", ah.GetAllAccounts)
	accountRoutes.Get("/:id", ah.GetAccountByID)
	accountRoutes.Delete("/:id", ah.DeleteAccountByID)

	// Health
	f.Get("/health", lib.Ping)

	// Doc
	lib.DocAPI("ledger", "Ledger API", f)

	return f
}
