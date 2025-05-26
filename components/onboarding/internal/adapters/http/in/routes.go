package in

import (
	"github.com/LerianStudio/lib-auth/auth/middleware"
	libLog "github.com/LerianStudio/lib-commons/commons/log"
	libHTTP "github.com/LerianStudio/lib-commons/commons/net/http"
	libOpentelemetry "github.com/LerianStudio/lib-commons/commons/opentelemetry"
	_ "github.com/LerianStudio/midaz/components/onboarding/api"
	"github.com/LerianStudio/midaz/pkg/mmodel"
	"github.com/LerianStudio/midaz/pkg/net/http"
	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
	fiberSwagger "github.com/swaggo/fiber-swagger"
)

const midazName = "midaz"

// NewRouter registerNewRouters routes to the Server.
func NewRouter(lg libLog.Logger, tl *libOpentelemetry.Telemetry, auth *middleware.AuthClient, ah *AccountHandler, ph *PortfolioHandler, lh *LedgerHandler, ih *AssetHandler, oh *OrganizationHandler, sh *SegmentHandler) *fiber.App {
	f := fiber.New(fiber.Config{
		DisableStartupMessage: true,
	})
	tlMid := libHTTP.NewTelemetryMiddleware(tl)

	f.Use(tlMid.WithTelemetry(tl))
	f.Use(cors.New())
	f.Use(libHTTP.WithHTTPLogging(libHTTP.WithCustomLogger(lg)))

	// Organizations
	f.Post("/v1/organizations", auth.Authorize(midazName, "organizations", "post"), http.WithBody(new(mmodel.CreateOrganizationInput), oh.CreateOrganization))
	f.Patch("/v1/organizations/:id", auth.Authorize(midazName, "organizations", "patch"), http.ParseUUIDPathParameters, http.WithBody(new(mmodel.UpdateOrganizationInput), oh.UpdateOrganization))
	f.Get("/v1/organizations", auth.Authorize(midazName, "organizations", "get"), oh.GetAllOrganizations)
	f.Get("/v1/organizations/:id", auth.Authorize(midazName, "organizations", "get"), http.ParseUUIDPathParameters, oh.GetOrganizationByID)
	f.Delete("/v1/organizations/:id", auth.Authorize(midazName, "organizations", "delete"), http.ParseUUIDPathParameters, oh.DeleteOrganizationByID)
	f.Head("/v1/organizations/metrics/count", auth.Authorize(midazName, "organizations", "head"), http.ParseUUIDPathParameters, oh.CountOrganizations)

	// Ledgers
	f.Post("/v1/organizations/:organization_id/ledgers", auth.Authorize(midazName, "ledgers", "post"), http.ParseUUIDPathParameters, http.WithBody(new(mmodel.CreateLedgerInput), lh.CreateLedger))
	f.Patch("/v1/organizations/:organization_id/ledgers/:id", auth.Authorize(midazName, "ledgers", "patch"), http.ParseUUIDPathParameters, http.WithBody(new(mmodel.UpdateLedgerInput), lh.UpdateLedger))
	f.Get("/v1/organizations/:organization_id/ledgers", auth.Authorize(midazName, "ledgers", "get"), http.ParseUUIDPathParameters, lh.GetAllLedgers)
	f.Get("/v1/organizations/:organization_id/ledgers/:id", auth.Authorize(midazName, "ledgers", "get"), http.ParseUUIDPathParameters, lh.GetLedgerByID)
	f.Delete("/v1/organizations/:organization_id/ledgers/:id", auth.Authorize(midazName, "ledgers", "delete"), http.ParseUUIDPathParameters, lh.DeleteLedgerByID)

	// Assets
	f.Post("/v1/organizations/:organization_id/ledgers/:ledger_id/assets", auth.Authorize(midazName, "assets", "post"), http.ParseUUIDPathParameters, http.WithBody(new(mmodel.CreateAssetInput), ih.CreateAsset))
	f.Patch("/v1/organizations/:organization_id/ledgers/:ledger_id/assets/:id", auth.Authorize(midazName, "assets", "patch"), http.ParseUUIDPathParameters, http.WithBody(new(mmodel.UpdateAssetInput), ih.UpdateAsset))
	f.Get("/v1/organizations/:organization_id/ledgers/:ledger_id/assets", auth.Authorize(midazName, "assets", "get"), http.ParseUUIDPathParameters, ih.GetAllAssets)
	f.Get("/v1/organizations/:organization_id/ledgers/:ledger_id/assets/:id", auth.Authorize(midazName, "assets", "get"), http.ParseUUIDPathParameters, ih.GetAssetByID)
	f.Delete("/v1/organizations/:organization_id/ledgers/:ledger_id/assets/:id", auth.Authorize(midazName, "assets", "delete"), http.ParseUUIDPathParameters, ih.DeleteAssetByID)

	// Portfolios
	f.Post("/v1/organizations/:organization_id/ledgers/:ledger_id/portfolios", auth.Authorize(midazName, "portfolios", "post"), http.ParseUUIDPathParameters, http.WithBody(new(mmodel.CreatePortfolioInput), ph.CreatePortfolio))
	f.Patch("/v1/organizations/:organization_id/ledgers/:ledger_id/portfolios/:id", auth.Authorize(midazName, "portfolios", "patch"), http.ParseUUIDPathParameters, http.WithBody(new(mmodel.UpdatePortfolioInput), ph.UpdatePortfolio))
	f.Get("/v1/organizations/:organization_id/ledgers/:ledger_id/portfolios", auth.Authorize(midazName, "portfolios", "get"), http.ParseUUIDPathParameters, ph.GetAllPortfolios)
	f.Get("/v1/organizations/:organization_id/ledgers/:ledger_id/portfolios/:id", auth.Authorize(midazName, "portfolios", "get"), http.ParseUUIDPathParameters, ph.GetPortfolioByID)
	f.Delete("/v1/organizations/:organization_id/ledgers/:ledger_id/portfolios/:id", auth.Authorize(midazName, "portfolios", "delete"), http.ParseUUIDPathParameters, ph.DeletePortfolioByID)

	// Segment
	f.Post("/v1/organizations/:organization_id/ledgers/:ledger_id/segments", auth.Authorize(midazName, "segments", "post"), http.ParseUUIDPathParameters, http.WithBody(new(mmodel.CreateSegmentInput), sh.CreateSegment))
	f.Patch("/v1/organizations/:organization_id/ledgers/:ledger_id/segments/:id", auth.Authorize(midazName, "segments", "patch"), http.ParseUUIDPathParameters, http.WithBody(new(mmodel.UpdateSegmentInput), sh.UpdateSegment))
	f.Get("/v1/organizations/:organization_id/ledgers/:ledger_id/segments", auth.Authorize(midazName, "segments", "get"), http.ParseUUIDPathParameters, sh.GetAllSegments)
	f.Get("/v1/organizations/:organization_id/ledgers/:ledger_id/segments/:id", auth.Authorize(midazName, "segments", "get"), http.ParseUUIDPathParameters, sh.GetSegmentByID)
	f.Delete("/v1/organizations/:organization_id/ledgers/:ledger_id/segments/:id", auth.Authorize(midazName, "segments", "delete"), http.ParseUUIDPathParameters, sh.DeleteSegmentByID)

	// Accounts
	f.Post("/v1/organizations/:organization_id/ledgers/:ledger_id/accounts", auth.Authorize(midazName, "accounts", "post"), http.ParseUUIDPathParameters, http.WithBody(new(mmodel.CreateAccountInput), ah.CreateAccount))
	f.Patch("/v1/organizations/:organization_id/ledgers/:ledger_id/accounts/:id", auth.Authorize(midazName, "accounts", "patch"), http.ParseUUIDPathParameters, http.WithBody(new(mmodel.UpdateAccountInput), ah.UpdateAccount))
	f.Get("/v1/organizations/:organization_id/ledgers/:ledger_id/accounts", auth.Authorize(midazName, "accounts", "get"), http.ParseUUIDPathParameters, ah.GetAllAccounts)
	f.Get("/v1/organizations/:organization_id/ledgers/:ledger_id/accounts/:id", auth.Authorize(midazName, "accounts", "get"), http.ParseUUIDPathParameters, ah.GetAccountByID)
	f.Get("/v1/organizations/:organization_id/ledgers/:ledger_id/accounts/alias/:alias", auth.Authorize(midazName, "accounts", "get"), http.ParseUUIDPathParameters, ah.GetAccountByAlias)
	f.Get("/v1/organizations/:organization_id/ledgers/:ledger_id/accounts/external/:code", auth.Authorize(midazName, "accounts", "get"), http.ParseUUIDPathParameters, ah.GetAccountExternalByCode)
	f.Delete("/v1/organizations/:organization_id/ledgers/:ledger_id/accounts/:id", auth.Authorize(midazName, "accounts", "delete"), http.ParseUUIDPathParameters, ah.DeleteAccountByID)

	// Health
	f.Get("/health", libHTTP.Ping)

	// Version
	f.Get("/version", libHTTP.Version)

	// Doc
	f.Get("/swagger/*", WithSwaggerEnvConfig(), fiberSwagger.WrapHandler)

	f.Use(tlMid.EndTracingSpans)

	return f
}
