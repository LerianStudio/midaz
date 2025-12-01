package in

import (
	"os"
	"strings"

	"github.com/LerianStudio/lib-auth/v2/auth/middleware"
	libLog "github.com/LerianStudio/lib-commons/v2/commons/log"
	libHTTP "github.com/LerianStudio/lib-commons/v2/commons/net/http"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v2/commons/opentelemetry"
	_ "github.com/LerianStudio/midaz/v3/components/onboarding/api"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/LerianStudio/midaz/v3/pkg/net/http"
	"github.com/lerianstudio/monorepo/pkg/platform/versioning"
	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
	fiberSwagger "github.com/swaggo/fiber-swagger"
)

const midazName = "midaz"
const routingName = "routing"

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

	// CORS configuration - restrict to allowed origins
	allowedOrigins := os.Getenv("ALLOWED_ORIGINS")
	if allowedOrigins == "" {
		// Default for development - should be overridden in production
		allowedOrigins = "http://localhost:3000,http://localhost:3001,http://localhost:8080"
	}

	f.Use(cors.New(cors.Config{
		AllowOrigins:     allowedOrigins,
		AllowMethods:     strings.Join([]string{fiber.MethodGet, fiber.MethodPost, fiber.MethodPut, fiber.MethodPatch, fiber.MethodDelete, fiber.MethodOptions, fiber.MethodHead}, ","),
		AllowHeaders:     "Origin,Content-Type,Accept,Authorization,X-Request-Id",
		AllowCredentials: true,
		MaxAge:           3600, // 1 hour
	}))

	// Security headers middleware
	f.Use(securityHeadersMiddleware)

	f.Use(libHTTP.WithHTTPLogging(libHTTP.WithCustomLogger(lg)))

	// Versioned route registration
	versioning.Register(f, "", map[string]func(fiber.Router){
		"v1": func(r fiber.Router) {
			registerOrganizationRoutes(r, auth, oh)
			registerLedgerRoutes(r, auth, lh)
			registerAssetRoutes(r, auth, ih)
			registerPortfolioRoutes(r, auth, ph)
			registerSegmentRoutes(r, auth, sh)
			registerAccountRoutes(r, auth, ah)
			registerAccountTypeRoutes(r, auth, ath)
		},
	})

	// Health
	f.Get("/health", libHTTP.Ping)

	// Version
	f.Get("/version", libHTTP.Version)

	// Doc
	f.Get("/swagger/*", WithSwaggerEnvConfig(), fiberSwagger.WrapHandler)

	f.Use(tlMid.EndTracingSpans)

	return f
}

// registerOrganizationRoutes registers all organization-related routes.
func registerOrganizationRoutes(r fiber.Router, auth *middleware.AuthClient, h *OrganizationHandler) {
	orgs := r.Group("/organizations")
	orgs.Post("", auth.Authorize(midazName, "organizations", "post"), http.WithBody(new(mmodel.CreateOrganizationInput), h.CreateOrganization))
	orgs.Patch("/:id", auth.Authorize(midazName, "organizations", "patch"), http.ParseUUIDPathParameters("organization"), http.WithBody(new(mmodel.UpdateOrganizationInput), h.UpdateOrganization))
	orgs.Get("", auth.Authorize(midazName, "organizations", "get"), h.GetAllOrganizations)
	orgs.Get("/:id", auth.Authorize(midazName, "organizations", "get"), http.ParseUUIDPathParameters("organization"), h.GetOrganizationByID)
	orgs.Delete("/:id", auth.Authorize(midazName, "organizations", "delete"), http.ParseUUIDPathParameters("organization"), h.DeleteOrganizationByID)
	orgs.Head("/metrics/count", auth.Authorize(midazName, "organizations", "head"), h.CountOrganizations)
}

// registerLedgerRoutes registers all ledger-related routes.
func registerLedgerRoutes(r fiber.Router, auth *middleware.AuthClient, h *LedgerHandler) {
	ledgers := r.Group("/organizations/:organization_id/ledgers")
	ledgers.Post("", auth.Authorize(midazName, "ledgers", "post"), http.ParseUUIDPathParameters("ledger"), http.WithBody(new(mmodel.CreateLedgerInput), h.CreateLedger))
	ledgers.Patch("/:id", auth.Authorize(midazName, "ledgers", "patch"), http.ParseUUIDPathParameters("ledger"), http.WithBody(new(mmodel.UpdateLedgerInput), h.UpdateLedger))
	ledgers.Get("", auth.Authorize(midazName, "ledgers", "get"), http.ParseUUIDPathParameters("ledger"), h.GetAllLedgers)
	ledgers.Get("/:id", auth.Authorize(midazName, "ledgers", "get"), http.ParseUUIDPathParameters("ledger"), h.GetLedgerByID)
	ledgers.Delete("/:id", auth.Authorize(midazName, "ledgers", "delete"), http.ParseUUIDPathParameters("ledger"), h.DeleteLedgerByID)
	ledgers.Head("/metrics/count", auth.Authorize(midazName, "ledgers", "head"), http.ParseUUIDPathParameters("ledger"), h.CountLedgers)
}

// registerAssetRoutes registers all asset-related routes.
func registerAssetRoutes(r fiber.Router, auth *middleware.AuthClient, h *AssetHandler) {
	assets := r.Group("/organizations/:organization_id/ledgers/:ledger_id/assets")
	assets.Post("", auth.Authorize(midazName, "assets", "post"), http.ParseUUIDPathParameters("asset"), http.WithBody(new(mmodel.CreateAssetInput), h.CreateAsset))
	assets.Patch("/:id", auth.Authorize(midazName, "assets", "patch"), http.ParseUUIDPathParameters("asset"), http.WithBody(new(mmodel.UpdateAssetInput), h.UpdateAsset))
	assets.Get("", auth.Authorize(midazName, "assets", "get"), http.ParseUUIDPathParameters("asset"), h.GetAllAssets)
	assets.Get("/:id", auth.Authorize(midazName, "assets", "get"), http.ParseUUIDPathParameters("asset"), h.GetAssetByID)
	assets.Delete("/:id", auth.Authorize(midazName, "assets", "delete"), http.ParseUUIDPathParameters("asset"), h.DeleteAssetByID)
	assets.Head("/metrics/count", auth.Authorize(midazName, "assets", "head"), http.ParseUUIDPathParameters("asset"), h.CountAssets)
}

// registerPortfolioRoutes registers all portfolio-related routes.
func registerPortfolioRoutes(r fiber.Router, auth *middleware.AuthClient, h *PortfolioHandler) {
	portfolios := r.Group("/organizations/:organization_id/ledgers/:ledger_id/portfolios")
	portfolios.Post("", auth.Authorize(midazName, "portfolios", "post"), http.ParseUUIDPathParameters("portfolio"), http.WithBody(new(mmodel.CreatePortfolioInput), h.CreatePortfolio))
	portfolios.Patch("/:id", auth.Authorize(midazName, "portfolios", "patch"), http.ParseUUIDPathParameters("portfolio"), http.WithBody(new(mmodel.UpdatePortfolioInput), h.UpdatePortfolio))
	portfolios.Get("", auth.Authorize(midazName, "portfolios", "get"), http.ParseUUIDPathParameters("portfolio"), h.GetAllPortfolios)
	portfolios.Get("/:id", auth.Authorize(midazName, "portfolios", "get"), http.ParseUUIDPathParameters("portfolio"), h.GetPortfolioByID)
	portfolios.Delete("/:id", auth.Authorize(midazName, "portfolios", "delete"), http.ParseUUIDPathParameters("portfolio"), h.DeletePortfolioByID)
	portfolios.Head("/metrics/count", auth.Authorize(midazName, "portfolios", "head"), http.ParseUUIDPathParameters("portfolio"), h.CountPortfolios)
}

// registerSegmentRoutes registers all segment-related routes.
func registerSegmentRoutes(r fiber.Router, auth *middleware.AuthClient, h *SegmentHandler) {
	segments := r.Group("/organizations/:organization_id/ledgers/:ledger_id/segments")
	segments.Post("", auth.Authorize(midazName, "segments", "post"), http.ParseUUIDPathParameters("segment"), http.WithBody(new(mmodel.CreateSegmentInput), h.CreateSegment))
	segments.Patch("/:id", auth.Authorize(midazName, "segments", "patch"), http.ParseUUIDPathParameters("segment"), http.WithBody(new(mmodel.UpdateSegmentInput), h.UpdateSegment))
	segments.Get("", auth.Authorize(midazName, "segments", "get"), http.ParseUUIDPathParameters("segment"), h.GetAllSegments)
	segments.Get("/:id", auth.Authorize(midazName, "segments", "get"), http.ParseUUIDPathParameters("segment"), h.GetSegmentByID)
	segments.Delete("/:id", auth.Authorize(midazName, "segments", "delete"), http.ParseUUIDPathParameters("segment"), h.DeleteSegmentByID)
	segments.Head("/metrics/count", auth.Authorize(midazName, "segments", "head"), http.ParseUUIDPathParameters("segment"), h.CountSegments)
}

// registerAccountRoutes registers all account-related routes.
func registerAccountRoutes(r fiber.Router, auth *middleware.AuthClient, h *AccountHandler) {
	accounts := r.Group("/organizations/:organization_id/ledgers/:ledger_id/accounts")
	accounts.Post("", auth.Authorize(midazName, "accounts", "post"), http.ParseUUIDPathParameters("account"), http.WithBody(new(mmodel.CreateAccountInput), h.CreateAccount))
	accounts.Patch("/:id", auth.Authorize(midazName, "accounts", "patch"), http.ParseUUIDPathParameters("account"), http.WithBody(new(mmodel.UpdateAccountInput), h.UpdateAccount))
	accounts.Get("", auth.Authorize(midazName, "accounts", "get"), http.ParseUUIDPathParameters("account"), h.GetAllAccounts)
	accounts.Get("/:id", auth.Authorize(midazName, "accounts", "get"), http.ParseUUIDPathParameters("account"), h.GetAccountByID)
	accounts.Get("/alias/:alias", auth.Authorize(midazName, "accounts", "get"), http.ParseUUIDPathParameters("account"), h.GetAccountByAlias)
	accounts.Get("/external/:code", auth.Authorize(midazName, "accounts", "get"), http.ParseUUIDPathParameters("account"), h.GetAccountExternalByCode)
	accounts.Delete("/:id", auth.Authorize(midazName, "accounts", "delete"), http.ParseUUIDPathParameters("account"), h.DeleteAccountByID)
	accounts.Head("/metrics/count", auth.Authorize(midazName, "accounts", "head"), http.ParseUUIDPathParameters("account"), h.CountAccounts)
}

// registerAccountTypeRoutes registers all account-type-related routes.
func registerAccountTypeRoutes(r fiber.Router, auth *middleware.AuthClient, h *AccountTypeHandler) {
	accountTypes := r.Group("/organizations/:organization_id/ledgers/:ledger_id/account-types")
	accountTypes.Post("", auth.Authorize(routingName, "account-types", "post"), http.ParseUUIDPathParameters("account_type"), http.WithBody(new(mmodel.CreateAccountTypeInput), h.CreateAccountType))
	accountTypes.Patch("/:id", auth.Authorize(routingName, "account-types", "patch"), http.ParseUUIDPathParameters("account_type"), http.WithBody(new(mmodel.UpdateAccountTypeInput), h.UpdateAccountType))
	accountTypes.Get("/:id", auth.Authorize(routingName, "account-types", "get"), http.ParseUUIDPathParameters("account_type"), h.GetAccountTypeByID)
	accountTypes.Get("", auth.Authorize(routingName, "account-types", "get"), http.ParseUUIDPathParameters("account_type"), h.GetAllAccountTypes)
	accountTypes.Delete("/:id", auth.Authorize(routingName, "account-types", "delete"), http.ParseUUIDPathParameters("account_type"), h.DeleteAccountTypeByID)
}

// securityHeadersMiddleware adds security headers to all responses.
// These headers protect against common web vulnerabilities.
func securityHeadersMiddleware(c *fiber.Ctx) error {
	// HSTS - Force HTTPS for 1 year (31536000 seconds)
	c.Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains")

	// Prevent clickjacking attacks
	c.Set("X-Frame-Options", "DENY")

	// Prevent MIME type sniffing
	c.Set("X-Content-Type-Options", "nosniff")

	// XSS Protection for legacy browsers
	c.Set("X-XSS-Protection", "1; mode=block")

	// Control referrer information
	c.Set("Referrer-Policy", "strict-origin-when-cross-origin")

	// Content Security Policy - restrict resource loading
	c.Set("Content-Security-Policy", "default-src 'self'; frame-ancestors 'none'")

	return c.Next()
}
