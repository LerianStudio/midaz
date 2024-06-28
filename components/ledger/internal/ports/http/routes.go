package http

import (

	// "github.com/LerianStudio/midaz/common/mauth"
	"github.com/LerianStudio/midaz/common/mauth"
	lib "github.com/LerianStudio/midaz/common/net/http"
	l "github.com/LerianStudio/midaz/components/ledger/internal/domain/onboarding/ledger"
	o "github.com/LerianStudio/midaz/components/ledger/internal/domain/onboarding/organization"
	a "github.com/LerianStudio/midaz/components/ledger/internal/domain/portfolio/account"
	i "github.com/LerianStudio/midaz/components/ledger/internal/domain/portfolio/instrument"
	p "github.com/LerianStudio/midaz/components/ledger/internal/domain/portfolio/portfolio"
	r "github.com/LerianStudio/midaz/components/ledger/internal/domain/portfolio/product"
	"github.com/LerianStudio/midaz/components/ledger/internal/ports"
	"github.com/LerianStudio/midaz/components/ledger/internal/service"
	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
)

// NewRouter registers routes to the Server.
func NewRouter(ah *ports.AccountHandler, ph *ports.PortfolioHandler, lh *ports.LedgerHandler, ih *ports.InstrumentHandler, oh *ports.OrganizationHandler, rh *ports.ProductHandler) *fiber.App {
	f := fiber.New()

	_ = service.NewConfig()

	f.Use(cors.New())
	f.Use(lib.WithCorrelationID())

	// -- Middleware --
	lib.NewAuthnMiddleware(f, mauth.NewAuthClient())

	// -- Routes --

	// Organizations
	f.Post("/v1/organizations", lib.WithScope([]string{"organization:create"}), lib.WithBody(new(o.CreateOrganizationInput), oh.CreateOrganization))
	f.Patch("/v1/organizations/:id", lib.WithBody(new(o.UpdateOrganizationInput), oh.UpdateOrganization))
	f.Get("/v1/organizations", oh.GetAllOrganizations)
	f.Get("/v1/organizations/:id", oh.GetOrganizationByID)
	f.Delete("/v1/organizations/:id", oh.DeleteOrganizationByID)

	// Ledgers
	f.Post("/v1/organizations/:organization_id/ledgers", lib.WithBody(new(l.CreateLedgerInput), lh.CreateLedger))
	f.Patch("/v1/organizations/:organization_id/ledgers/:id", lib.WithBody(new(l.UpdateLedgerInput), lh.UpdateLedger))
	f.Get("/v1/organizations/:organization_id/ledgers", lh.GetAllLedgers)
	f.Get("/v1/organizations/:organization_id/ledgers/:id", lh.GetLedgerByID)
	f.Delete("/v1/organizations/:organization_id/ledgers/:id", lh.DeleteLedgerByID)

	// Instruments
	f.Post("/v1/organizations/:organization_id/ledgers/:ledger_id/instruments", lib.WithBody(new(i.CreateInstrumentInput), ih.CreateInstrument))
	f.Patch("/v1/organizations/:organization_id/ledgers/:ledger_id/instruments/:id", lib.WithBody(new(i.UpdateInstrumentInput), ih.UpdateInstrument))
	f.Get("/v1/organizations/:organization_id/ledgers/:ledger_id/instruments", ih.GetAllInstruments)
	f.Get("/v1/organizations/:organization_id/ledgers/:ledger_id/instruments/:id", ih.GetInstrumentByID)
	f.Delete("/v1/organizations/:organization_id/ledgers/:ledger_id/instruments/:id", ih.DeleteInstrumentByID)

	// Portfolios
	f.Post("/v1/organizations/:organization_id/ledgers/:ledger_id/portfolios", lib.WithBody(new(p.CreatePortfolioInput), ph.CreatePortfolio))
	f.Patch("/v1/organizations/:organization_id/ledgers/:ledger_id/portfolios/:id", lib.WithBody(new(p.UpdatePortfolioInput), ph.UpdatePortfolio))
	f.Get("/v1/organizations/:organization_id/ledgers/:ledger_id/portfolios", ph.GetAllPortfolios)
	f.Get("/v1/organizations/:organization_id/ledgers/:ledger_id/portfolios/:id", ph.GetPortfolioByID)
	f.Delete("/v1/organizations/:organization_id/ledgers/:ledger_id/portfolios/:id", ph.DeletePortfolioByID)

	// Product
	f.Post("/v1/organizations/:organization_id/ledgers/:ledger_id/products", lib.WithBody(new(r.CreateProductInput), rh.CreateProduct))
	f.Patch("/v1/organizations/:organization_id/ledgers/:ledger_id/products/:id", lib.WithBody(new(r.UpdateProductInput), rh.UpdateProduct))
	f.Get("/v1/organizations/:organization_id/ledgers/:ledger_id/products", rh.GetAllProducts)
	f.Get("/v1/organizations/:organization_id/ledgers/:ledger_id/products/:id", rh.GetProductByID)
	f.Delete("/v1/organizations/:organization_id/ledgers/:ledger_id/products/:id", rh.DeleteProductByID)

	// Accounts
	f.Post("/v1/organizations/:organization_id/ledgers/:ledger_id/portfolios/:portfolio_id/accounts", lib.WithBody(new(a.CreateAccountInput), ah.CreateAccount))
	f.Patch("/v1/organizations/:organization_id/ledgers/:ledger_id/portfolios/:portfolio_id/accounts/:id", lib.WithBody(new(a.UpdateAccountInput), ah.UpdateAccount))
	f.Get("/v1/organizations/:organization_id/ledgers/:ledger_id/portfolios/:portfolio_id/accounts", ah.GetAllAccounts)
	f.Get("/v1/organizations/:organization_id/ledgers/:ledger_id/portfolios/:portfolio_id/accounts/:id", ah.GetAccountByID)
	f.Delete("/v1/organizations/:organization_id/ledgers/:ledger_id/portfolios/:portfolio_id/accounts/:id", ah.DeleteAccountByID)

	// Health
	f.Get("/health", lib.Ping)

	// Doc
	lib.DocAPI("ledger", "Ledger API", f)

	return f
}
