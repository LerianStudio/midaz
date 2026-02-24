// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"github.com/LerianStudio/lib-auth/v2/auth/middleware"
	libLog "github.com/LerianStudio/lib-commons/v2/commons/log"
	libHTTP "github.com/LerianStudio/lib-commons/v2/commons/net/http"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v2/commons/opentelemetry"
	_ "github.com/LerianStudio/midaz/v3/components/onboarding/api"
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

// NewRouter register NewRouter routes to the Server.
func NewRouter(lg libLog.Logger, tl *libOpentelemetry.Telemetry, auth *middleware.AuthClient, ah *AccountHandler, ph *PortfolioHandler, lh *LedgerHandler, ih *AssetHandler, oh *OrganizationHandler, sh *SegmentHandler, ath *AccountTypeHandler) *fiber.App {
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

	// Register all routes
	RegisterRoutesToApp(f, auth, ah, ph, lh, ih, oh, sh, ath)

	// Health
	f.Get("/health", libHTTP.Ping)

	// Version
	f.Get("/version", libHTTP.Version)

	// Doc
	f.Get("/swagger/*", WithSwaggerEnvConfig(), fiberSwagger.FiberWrapHandler(
		fiberSwagger.InstanceName("onboarding"),
	))

	f.Use(tlMid.EndTracingSpans)

	return f
}

// RegisterRoutesToApp registers onboarding routes to an existing Fiber app.
// This is used by the unified ledger server to consolidate all routes in a single port.
// The app should already have middleware configured (telemetry, cors, logging).
func RegisterRoutesToApp(f *fiber.App, auth *middleware.AuthClient, ah *AccountHandler, ph *PortfolioHandler, lh *LedgerHandler, ih *AssetHandler, oh *OrganizationHandler, sh *SegmentHandler, ath *AccountTypeHandler) {
	// Organizations
	f.Post("/v1/organizations", auth.Authorize(midazName, "organizations", "post"), http.WithBody(new(mmodel.CreateOrganizationInput), oh.CreateOrganization))
	f.Patch("/v1/organizations/:id", auth.Authorize(midazName, "organizations", "patch"), http.ParseUUIDPathParameters("organization"), http.WithBody(new(mmodel.UpdateOrganizationInput), oh.UpdateOrganization))
	f.Get("/v1/organizations", auth.Authorize(midazName, "organizations", "get"), oh.GetAllOrganizations)
	f.Get("/v1/organizations/:id", auth.Authorize(midazName, "organizations", "get"), http.ParseUUIDPathParameters("organization"), oh.GetOrganizationByID)
	f.Delete("/v1/organizations/:id", auth.Authorize(midazName, "organizations", "delete"), http.ParseUUIDPathParameters("organization"), oh.DeleteOrganizationByID)
	f.Head("/v1/organizations/metrics/count", auth.Authorize(midazName, "organizations", "head"), oh.CountOrganizations)

	// Ledgers
	f.Post("/v1/organizations/:organization_id/ledgers", auth.Authorize(midazName, "ledgers", "post"), http.ParseUUIDPathParameters("ledger"), http.WithBody(new(mmodel.CreateLedgerInput), lh.CreateLedger))
	f.Patch("/v1/organizations/:organization_id/ledgers/:id", auth.Authorize(midazName, "ledgers", "patch"), http.ParseUUIDPathParameters("ledger"), http.WithBody(new(mmodel.UpdateLedgerInput), lh.UpdateLedger))
	f.Get("/v1/organizations/:organization_id/ledgers", auth.Authorize(midazName, "ledgers", "get"), http.ParseUUIDPathParameters("ledger"), lh.GetAllLedgers)
	f.Get("/v1/organizations/:organization_id/ledgers/:id", auth.Authorize(midazName, "ledgers", "get"), http.ParseUUIDPathParameters("ledger"), lh.GetLedgerByID)
	f.Get("/v1/organizations/:organization_id/ledgers/:id/settings", auth.Authorize(midazName, "ledgers", "get"), http.ParseUUIDPathParameters("ledger"), lh.GetLedgerSettings)
	f.Patch("/v1/organizations/:organization_id/ledgers/:id/settings", auth.Authorize(midazName, "ledgers", "patch"), http.ParseUUIDPathParameters("ledger"), http.WithBodyLimit(SettingsMaxPayloadSize), http.WithBody(new(map[string]any), lh.UpdateLedgerSettings))
	f.Delete("/v1/organizations/:organization_id/ledgers/:id", auth.Authorize(midazName, "ledgers", "delete"), http.ParseUUIDPathParameters("ledger"), lh.DeleteLedgerByID)
	f.Head("/v1/organizations/:organization_id/ledgers/metrics/count", auth.Authorize(midazName, "ledgers", "head"), http.ParseUUIDPathParameters("ledger"), lh.CountLedgers)

	// Assets
	f.Post("/v1/organizations/:organization_id/ledgers/:ledger_id/assets", auth.Authorize(midazName, "assets", "post"), http.ParseUUIDPathParameters("asset"), http.WithBody(new(mmodel.CreateAssetInput), ih.CreateAsset))
	f.Patch("/v1/organizations/:organization_id/ledgers/:ledger_id/assets/:id", auth.Authorize(midazName, "assets", "patch"), http.ParseUUIDPathParameters("asset"), http.WithBody(new(mmodel.UpdateAssetInput), ih.UpdateAsset))
	f.Get("/v1/organizations/:organization_id/ledgers/:ledger_id/assets", auth.Authorize(midazName, "assets", "get"), http.ParseUUIDPathParameters("asset"), ih.GetAllAssets)
	f.Get("/v1/organizations/:organization_id/ledgers/:ledger_id/assets/:id", auth.Authorize(midazName, "assets", "get"), http.ParseUUIDPathParameters("asset"), ih.GetAssetByID)
	f.Delete("/v1/organizations/:organization_id/ledgers/:ledger_id/assets/:id", auth.Authorize(midazName, "assets", "delete"), http.ParseUUIDPathParameters("asset"), ih.DeleteAssetByID)
	f.Head("/v1/organizations/:organization_id/ledgers/:ledger_id/assets/metrics/count", auth.Authorize(midazName, "assets", "head"), http.ParseUUIDPathParameters("asset"), ih.CountAssets)

	// Portfolios
	f.Post("/v1/organizations/:organization_id/ledgers/:ledger_id/portfolios", auth.Authorize(midazName, "portfolios", "post"), http.ParseUUIDPathParameters("portfolio"), http.WithBody(new(mmodel.CreatePortfolioInput), ph.CreatePortfolio))
	f.Patch("/v1/organizations/:organization_id/ledgers/:ledger_id/portfolios/:id", auth.Authorize(midazName, "portfolios", "patch"), http.ParseUUIDPathParameters("portfolio"), http.WithBody(new(mmodel.UpdatePortfolioInput), ph.UpdatePortfolio))
	f.Get("/v1/organizations/:organization_id/ledgers/:ledger_id/portfolios", auth.Authorize(midazName, "portfolios", "get"), http.ParseUUIDPathParameters("portfolio"), ph.GetAllPortfolios)
	f.Get("/v1/organizations/:organization_id/ledgers/:ledger_id/portfolios/:id", auth.Authorize(midazName, "portfolios", "get"), http.ParseUUIDPathParameters("portfolio"), ph.GetPortfolioByID)
	f.Delete("/v1/organizations/:organization_id/ledgers/:ledger_id/portfolios/:id", auth.Authorize(midazName, "portfolios", "delete"), http.ParseUUIDPathParameters("portfolio"), ph.DeletePortfolioByID)
	f.Head("/v1/organizations/:organization_id/ledgers/:ledger_id/portfolios/metrics/count", auth.Authorize(midazName, "portfolios", "head"), http.ParseUUIDPathParameters("portfolio"), ph.CountPortfolios)

	// Segment
	f.Post("/v1/organizations/:organization_id/ledgers/:ledger_id/segments", auth.Authorize(midazName, "segments", "post"), http.ParseUUIDPathParameters("segment"), http.WithBody(new(mmodel.CreateSegmentInput), sh.CreateSegment))
	f.Patch("/v1/organizations/:organization_id/ledgers/:ledger_id/segments/:id", auth.Authorize(midazName, "segments", "patch"), http.ParseUUIDPathParameters("segment"), http.WithBody(new(mmodel.UpdateSegmentInput), sh.UpdateSegment))
	f.Get("/v1/organizations/:organization_id/ledgers/:ledger_id/segments", auth.Authorize(midazName, "segments", "get"), http.ParseUUIDPathParameters("segment"), sh.GetAllSegments)
	f.Get("/v1/organizations/:organization_id/ledgers/:ledger_id/segments/:id", auth.Authorize(midazName, "segments", "get"), http.ParseUUIDPathParameters("segment"), sh.GetSegmentByID)
	f.Delete("/v1/organizations/:organization_id/ledgers/:ledger_id/segments/:id", auth.Authorize(midazName, "segments", "delete"), http.ParseUUIDPathParameters("segment"), sh.DeleteSegmentByID)
	f.Head("/v1/organizations/:organization_id/ledgers/:ledger_id/segments/metrics/count", auth.Authorize(midazName, "segments", "head"), http.ParseUUIDPathParameters("segment"), sh.CountSegments)

	// Accounts
	f.Post("/v1/organizations/:organization_id/ledgers/:ledger_id/accounts", auth.Authorize(midazName, "accounts", "post"), http.ParseUUIDPathParameters("account"), http.WithBody(new(mmodel.CreateAccountInput), ah.CreateAccount))
	f.Patch("/v1/organizations/:organization_id/ledgers/:ledger_id/accounts/:id", auth.Authorize(midazName, "accounts", "patch"), http.ParseUUIDPathParameters("account"), http.WithBody(new(mmodel.UpdateAccountInput), ah.UpdateAccount))
	f.Get("/v1/organizations/:organization_id/ledgers/:ledger_id/accounts", auth.Authorize(midazName, "accounts", "get"), http.ParseUUIDPathParameters("account"), ah.GetAllAccounts)
	f.Get("/v1/organizations/:organization_id/ledgers/:ledger_id/accounts/:id", auth.Authorize(midazName, "accounts", "get"), http.ParseUUIDPathParameters("account"), ah.GetAccountByID)
	f.Get("/v1/organizations/:organization_id/ledgers/:ledger_id/accounts/alias/:alias", auth.Authorize(midazName, "accounts", "get"), http.ParseUUIDPathParameters("account"), ah.GetAccountByAlias)
	f.Get("/v1/organizations/:organization_id/ledgers/:ledger_id/accounts/external/:code", auth.Authorize(midazName, "accounts", "get"), http.ParseUUIDPathParameters("account"), ah.GetAccountExternalByCode)
	f.Delete("/v1/organizations/:organization_id/ledgers/:ledger_id/accounts/:id", auth.Authorize(midazName, "accounts", "delete"), http.ParseUUIDPathParameters("account"), ah.DeleteAccountByID)
	f.Head("/v1/organizations/:organization_id/ledgers/:ledger_id/accounts/metrics/count", auth.Authorize(midazName, "accounts", "head"), http.ParseUUIDPathParameters("account"), ah.CountAccounts)

	// Account Types
	f.Post("/v1/organizations/:organization_id/ledgers/:ledger_id/account-types", auth.Authorize(routingName, "account-types", "post"), http.ParseUUIDPathParameters("account_type"), http.WithBody(new(mmodel.CreateAccountTypeInput), ath.CreateAccountType))
	f.Patch("/v1/organizations/:organization_id/ledgers/:ledger_id/account-types/:id", auth.Authorize(routingName, "account-types", "patch"), http.ParseUUIDPathParameters("account_type"), http.WithBody(new(mmodel.UpdateAccountTypeInput), ath.UpdateAccountType))
	f.Get("/v1/organizations/:organization_id/ledgers/:ledger_id/account-types/:id", auth.Authorize(routingName, "account-types", "get"), http.ParseUUIDPathParameters("account_type"), ath.GetAccountTypeByID)
	f.Get("/v1/organizations/:organization_id/ledgers/:ledger_id/account-types", auth.Authorize(routingName, "account-types", "get"), http.ParseUUIDPathParameters("account_type"), ath.GetAllAccountTypes)
	f.Delete("/v1/organizations/:organization_id/ledgers/:ledger_id/account-types/:id", auth.Authorize(routingName, "account-types", "delete"), http.ParseUUIDPathParameters("account_type"), ath.DeleteAccountTypeByID)
}
