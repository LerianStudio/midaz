package in

import (
	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
	fiberSwagger "github.com/swaggo/fiber-swagger"

	_ "github.com/LerianStudio/midaz/components/ledger/api"
	"github.com/LerianStudio/midaz/pkg/mcasdoor"
	"github.com/LerianStudio/midaz/pkg/mlog"
	"github.com/LerianStudio/midaz/pkg/mmodel"
	"github.com/LerianStudio/midaz/pkg/mopentelemetry"
	"github.com/LerianStudio/midaz/pkg/net/http"
)

// NewRouter registerNewRouters routes to the Server.
func NewRouter(lg mlog.Logger, tl *mopentelemetry.Telemetry, cc *mcasdoor.CasdoorConnection, ah *AccountHandler, ph *PortfolioHandler, lh *LedgerHandler, ih *AssetHandler, oh *OrganizationHandler, rh *ProductHandler) *fiber.App {
	f := fiber.New(fiber.Config{
		DisableStartupMessage: true,
	})
	tlMid := http.NewTelemetryMiddleware(tl)

	f.Use(tlMid.WithTelemetry(tl))
	f.Use(cors.New())
	f.Use(http.WithHTTPLogging(http.WithCustomLogger(lg)))
	jwt := http.NewJWTMiddleware(cc)

	// Organizations
	f.Post("/v1/organizations", jwt.ProtectHTTP(), jwt.WithPermissionHTTP("organization"), http.WithBody(new(mmodel.CreateOrganizationInput), oh.CreateOrganization))
	f.Patch("/v1/organizations/:id", jwt.ProtectHTTP(), jwt.WithPermissionHTTP("organization"), http.ParseUUIDPathParameters, http.WithBody(new(mmodel.UpdateOrganizationInput), oh.UpdateOrganization))
	f.Get("/v1/organizations", jwt.ProtectHTTP(), jwt.WithPermissionHTTP("organization"), oh.GetAllOrganizations)
	f.Get("/v1/organizations/:id", jwt.ProtectHTTP(), jwt.WithPermissionHTTP("organization"), http.ParseUUIDPathParameters, oh.GetOrganizationByID)
	f.Delete("/v1/organizations/:id", jwt.ProtectHTTP(), jwt.WithPermissionHTTP("organization"), http.ParseUUIDPathParameters, oh.DeleteOrganizationByID)

	// Ledgers
	f.Post("/v1/organizations/:organization_id/ledgers", jwt.ProtectHTTP(), jwt.WithPermissionHTTP("ledger"), http.ParseUUIDPathParameters, http.WithBody(new(mmodel.CreateLedgerInput), lh.CreateLedger))
	f.Patch("/v1/organizations/:organization_id/ledgers/:id", jwt.ProtectHTTP(), jwt.WithPermissionHTTP("ledger"), http.ParseUUIDPathParameters, http.WithBody(new(mmodel.UpdateLedgerInput), lh.UpdateLedger))
	f.Get("/v1/organizations/:organization_id/ledgers", jwt.ProtectHTTP(), jwt.WithPermissionHTTP("ledger"), http.ParseUUIDPathParameters, lh.GetAllLedgers)
	f.Get("/v1/organizations/:organization_id/ledgers/:id", jwt.ProtectHTTP(), jwt.WithPermissionHTTP("ledger"), http.ParseUUIDPathParameters, lh.GetLedgerByID)
	f.Delete("/v1/organizations/:organization_id/ledgers/:id", jwt.ProtectHTTP(), jwt.WithPermissionHTTP("ledger"), http.ParseUUIDPathParameters, lh.DeleteLedgerByID)

	// Assets
	f.Post("/v1/organizations/:organization_id/ledgers/:ledger_id/assets", jwt.ProtectHTTP(), jwt.WithPermissionHTTP("asset"), http.ParseUUIDPathParameters, http.WithBody(new(mmodel.CreateAssetInput), ih.CreateAsset))
	f.Patch("/v1/organizations/:organization_id/ledgers/:ledger_id/assets/:id", jwt.ProtectHTTP(), jwt.WithPermissionHTTP("asset"), http.ParseUUIDPathParameters, http.WithBody(new(mmodel.UpdateAssetInput), ih.UpdateAsset))
	f.Get("/v1/organizations/:organization_id/ledgers/:ledger_id/assets", jwt.ProtectHTTP(), jwt.WithPermissionHTTP("asset"), http.ParseUUIDPathParameters, ih.GetAllAssets)
	f.Get("/v1/organizations/:organization_id/ledgers/:ledger_id/assets/:id", jwt.ProtectHTTP(), jwt.WithPermissionHTTP("asset"), http.ParseUUIDPathParameters, ih.GetAssetByID)
	f.Delete("/v1/organizations/:organization_id/ledgers/:ledger_id/assets/:id", jwt.ProtectHTTP(), jwt.WithPermissionHTTP("asset"), http.ParseUUIDPathParameters, ih.DeleteAssetByID)

	// Portfolios
	f.Post("/v1/organizations/:organization_id/ledgers/:ledger_id/portfolios", jwt.ProtectHTTP(), jwt.WithPermissionHTTP("portfolio"), http.ParseUUIDPathParameters, http.WithBody(new(mmodel.CreatePortfolioInput), ph.CreatePortfolio))
	f.Patch("/v1/organizations/:organization_id/ledgers/:ledger_id/portfolios/:id", jwt.ProtectHTTP(), jwt.WithPermissionHTTP("portfolio"), http.ParseUUIDPathParameters, http.WithBody(new(mmodel.UpdatePortfolioInput), ph.UpdatePortfolio))
	f.Get("/v1/organizations/:organization_id/ledgers/:ledger_id/portfolios", jwt.ProtectHTTP(), jwt.WithPermissionHTTP("portfolio"), http.ParseUUIDPathParameters, ph.GetAllPortfolios)
	f.Get("/v1/organizations/:organization_id/ledgers/:ledger_id/portfolios/:id", jwt.ProtectHTTP(), jwt.WithPermissionHTTP("portfolio"), http.ParseUUIDPathParameters, ph.GetPortfolioByID)
	f.Delete("/v1/organizations/:organization_id/ledgers/:ledger_id/portfolios/:id", jwt.ProtectHTTP(), jwt.WithPermissionHTTP("portfolio"), http.ParseUUIDPathParameters, ph.DeletePortfolioByID)

	// Cluster
	f.Post("/v1/organizations/:organization_id/ledgers/:ledger_id/clusters", jwt.ProtectHTTP(), jwt.WithPermissionHTTP("cluster"), http.ParseUUIDPathParameters, http.WithBody(new(mmodel.CreateClusterInput), rh.CreateCluster))
	f.Patch("/v1/organizations/:organization_id/ledgers/:ledger_id/clusters/:id", jwt.ProtectHTTP(), jwt.WithPermissionHTTP("cluster"), http.ParseUUIDPathParameters, http.WithBody(new(mmodel.UpdateClusterInput), rh.UpdateCluster))
	f.Get("/v1/organizations/:organization_id/ledgers/:ledger_id/clusters", jwt.ProtectHTTP(), jwt.WithPermissionHTTP("cluster"), http.ParseUUIDPathParameters, rh.GetAllClusters)
	f.Get("/v1/organizations/:organization_id/ledgers/:ledger_id/clusters/:id", jwt.ProtectHTTP(), jwt.WithPermissionHTTP("cluster"), http.ParseUUIDPathParameters, rh.GetClusterByID)
	f.Delete("/v1/organizations/:organization_id/ledgers/:ledger_id/clusters/:id", jwt.ProtectHTTP(), jwt.WithPermissionHTTP("cluster"), http.ParseUUIDPathParameters, rh.DeleteClusterByID)

	// Accounts
	f.Post("/v1/organizations/:organization_id/ledgers/:ledger_id/accounts", jwt.ProtectHTTP(), jwt.WithPermissionHTTP("account"), http.ParseUUIDPathParameters, http.WithBody(new(mmodel.CreateAccountInput), ah.CreateAccount))
	f.Patch("/v1/organizations/:organization_id/ledgers/:ledger_id/accounts/:id", jwt.ProtectHTTP(), jwt.WithPermissionHTTP("account"), http.ParseUUIDPathParameters, http.WithBody(new(mmodel.UpdateAccountInput), ah.UpdateAccount))
	f.Get("/v1/organizations/:organization_id/ledgers/:ledger_id/accounts", jwt.ProtectHTTP(), jwt.WithPermissionHTTP("account"), http.ParseUUIDPathParameters, ah.GetAllAccounts)
	f.Get("/v1/organizations/:organization_id/ledgers/:ledger_id/accounts/:id", jwt.ProtectHTTP(), jwt.WithPermissionHTTP("account"), http.ParseUUIDPathParameters, ah.GetAccountByID)
	f.Get("/v1/organizations/:organization_id/ledgers/:ledger_id/accounts/alias/:alias",
		jwt.ProtectHTTP(), jwt.WithPermissionHTTP("account"), http.ParseUUIDPathParameters, ah.GetAccountByAlias)
	f.Delete("/v1/organizations/:organization_id/ledgers/:ledger_id/accounts/:id", jwt.ProtectHTTP(), jwt.WithPermissionHTTP("account"), http.ParseUUIDPathParameters, ah.DeleteAccountByID)
	// Will be deprecated in the future. Use "POST /v1/organizations/:organization_id/ledgers/:ledger_id/accounts" instead.
	f.Post("/v1/organizations/:organization_id/ledgers/:ledger_id/portfolios/:portfolio_id/accounts", jwt.ProtectHTTP(), jwt.WithPermissionHTTP("account"), http.ParseUUIDPathParameters, http.WithBody(new(mmodel.CreateAccountInput), ah.CreateAccountFromPortfolio))
	// Will be deprecated in the future. Use "PATCH /v1/organizations/:organization_id/ledgers/:ledger_id/accounts/:id" instead.
	f.Patch("/v1/organizations/:organization_id/ledgers/:ledger_id/portfolios/:portfolio_id/accounts/:id", jwt.ProtectHTTP(), jwt.WithPermissionHTTP("account"), http.ParseUUIDPathParameters, http.WithBody(new(mmodel.UpdateAccountInput), ah.UpdateAccountFromPortfolio))
	// Will be deprecated in the future. Use "GET /v1/organizations/:organization_id/ledgers/:ledger_id/accounts" instead.
	f.Get("/v1/organizations/:organization_id/ledgers/:ledger_id/portfolios/:portfolio_id/accounts", jwt.ProtectHTTP(), jwt.WithPermissionHTTP("account"), http.ParseUUIDPathParameters, ah.GetAllAccountsByIDFromPortfolio)
	// Will be deprecated in the future. Use "GET /v1/organizations/:organization_id/ledgers/:ledger_id/accounts/:id" instead.
	f.Get("/v1/organizations/:organization_id/ledgers/:ledger_id/portfolios/:portfolio_id/accounts/:id", jwt.ProtectHTTP(), jwt.WithPermissionHTTP("account"), http.ParseUUIDPathParameters, ah.GetAccountByIDFromPortfolio)
	// Will be deprecated in the future. Use "DELETE /v1/organizations/:organization_id/ledgers/:ledger_id/accounts/:id" instead.
	f.Delete("/v1/organizations/:organization_id/ledgers/:ledger_id/portfolios/:portfolio_id/accounts/:id", jwt.ProtectHTTP(), jwt.WithPermissionHTTP("account"), http.ParseUUIDPathParameters, ah.DeleteAccountByIDFromPortfolio)

	// Health
	f.Get("/health", http.Ping)

	// Version
	f.Get("/version", http.Version)

	// Doc
	f.Get("/swagger/*", WithSwaggerEnvConfig(), fiberSwagger.WrapHandler)

	f.Use(tlMid.EndTracingSpans)

	return f
}
