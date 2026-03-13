// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"github.com/LerianStudio/lib-auth/v2/auth/middleware"
	libLog "github.com/LerianStudio/lib-commons/v4/commons/log"
	libHTTP "github.com/LerianStudio/lib-commons/v4/commons/net/http"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v4/commons/opentelemetry"
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
			return legacyFiberErrorHandler(ctx, err)
		},
	})

	tlMid := libHTTP.NewTelemetryMiddleware(tl)

	f.Use(tlMid.WithTelemetry(tl))
	f.Use(cors.New())
	f.Use(libHTTP.WithHTTPLogging(libHTTP.WithCustomLogger(lg)))
	// Register all routes
	RegisterRoutesToApp(f, auth, ah, ph, lh, ih, oh, sh, ath, nil)

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
func RegisterRoutesToApp(f fiber.Router, auth *middleware.AuthClient, ah *AccountHandler, ph *PortfolioHandler, lh *LedgerHandler, ih *AssetHandler, oh *OrganizationHandler, sh *SegmentHandler, ath *AccountTypeHandler, routeOptions *http.ProtectedRouteOptions) {
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
	f.Post("/v1/organizations/:organization_id/ledgers/:ledger_id/account-types", protectedRouting(auth, "post", routeOptions, http.ParseUUIDPathParameters("account_type"), http.WithBody(new(mmodel.CreateAccountTypeInput), ath.CreateAccountType))...)
	f.Patch("/v1/organizations/:organization_id/ledgers/:ledger_id/account-types/:id", protectedRouting(auth, "patch", routeOptions, http.ParseUUIDPathParameters("account_type"), http.WithBody(new(mmodel.UpdateAccountTypeInput), ath.UpdateAccountType))...)
	f.Get("/v1/organizations/:organization_id/ledgers/:ledger_id/account-types/:id", protectedRouting(auth, "get", routeOptions, http.ParseUUIDPathParameters("account_type"), ath.GetAccountTypeByID)...)
	f.Get("/v1/organizations/:organization_id/ledgers/:ledger_id/account-types", protectedRouting(auth, "get", routeOptions, http.ParseUUIDPathParameters("account_type"), ath.GetAllAccountTypes)...)
	f.Delete("/v1/organizations/:organization_id/ledgers/:ledger_id/account-types/:id", protectedRouting(auth, "delete", routeOptions, http.ParseUUIDPathParameters("account_type"), ath.DeleteAccountTypeByID)...)
}

func protectedMidaz(auth *middleware.AuthClient, resource, action string, routeOptions *http.ProtectedRouteOptions, handlers ...fiber.Handler) []fiber.Handler {
	return http.ProtectedRouteChain(auth.Authorize(midazName, resource, action), routeOptions, handlers...)
}

func protectedRouting(auth *middleware.AuthClient, action string, routeOptions *http.ProtectedRouteOptions, handlers ...fiber.Handler) []fiber.Handler {
	return http.ProtectedRouteChain(auth.Authorize(routingName, "account-types", action), routeOptions, handlers...)
}
