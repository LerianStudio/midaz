package in

import (
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/LerianStudio/midaz/v3/pkg/net/http"

	"github.com/LerianStudio/lib-auth/v2/auth/middleware"
	libLog "github.com/LerianStudio/lib-commons/v2/commons/log"
	libHTTP "github.com/LerianStudio/lib-commons/v2/commons/net/http"
	libOpenTelemetry "github.com/LerianStudio/lib-commons/v2/commons/opentelemetry"
	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
	fiberSwagger "github.com/swaggo/fiber-swagger"
)

const applicationName = "plugin-crm"

func NewRouter(lg libLog.Logger, tl *libOpenTelemetry.Telemetry, auth *middleware.AuthClient, hh *HolderHandler, ah *AliasHandler) *fiber.App {
	f := fiber.New(fiber.Config{
		DisableStartupMessage: true,
		ErrorHandler: func(ctx *fiber.Ctx, err error) error {
			return libHTTP.HandleFiberError(ctx, err)
		},
	})
	tlMid := libHTTP.NewTelemetryMiddleware(tl)

	f.Use(ErrorCodeTransformer()) // Transform generic error codes to CRM-specific codes
	f.Use(http.WithRecover(http.WithRecoverLogger(lg)))
	f.Use(tlMid.WithTelemetry(tl))
	f.Use(cors.New())
	f.Use(libHTTP.WithHTTPLogging(libHTTP.WithCustomLogger(lg)))

	// Holders
	f.Post("/v1/holders", auth.Authorize(applicationName, "holders", "post"), http.WithBody(new(mmodel.CreateHolderInput), hh.CreateHolder))
	f.Get("/v1/holders/:id", auth.Authorize(applicationName, "holders", "get"), http.ParseUUIDPathParameters("holder"), hh.GetHolderByID)
	f.Patch("/v1/holders/:id", auth.Authorize(applicationName, "holders", "patch"), http.ParseUUIDPathParameters("holder"), http.WithBody(new(mmodel.UpdateHolderInput), hh.UpdateHolder))
	f.Delete("/v1/holders/:id", auth.Authorize(applicationName, "holders", "delete"), http.ParseUUIDPathParameters("holder"), hh.DeleteHolderByID)
	f.Get("/v1/holders", auth.Authorize(applicationName, "holders", "get"), hh.GetAllHolders)

	// Aliases
	f.Get("/v1/aliases", auth.Authorize(applicationName, "aliases", "get"), ah.GetAllAliases)
	f.Post("/v1/holders/:holder_id/aliases", auth.Authorize(applicationName, "aliases", "post"), http.ParseUUIDPathParameters("aliases"), http.WithBody(new(mmodel.CreateAliasInput), ah.CreateAlias))
	f.Get("/v1/holders/:holder_id/aliases/:id", auth.Authorize(applicationName, "aliases", "get"), http.ParseUUIDPathParameters("aliases"), ah.GetAliasByID)
	f.Patch("/v1/holders/:holder_id/aliases/:id", auth.Authorize(applicationName, "aliases", "patch"), http.ParseUUIDPathParameters("aliases"), http.WithBody(new(mmodel.UpdateAliasInput), ah.UpdateAlias))
	f.Delete("/v1/holders/:holder_id/aliases/:id", auth.Authorize(applicationName, "aliases", "delete"), http.ParseUUIDPathParameters("aliases"), ah.DeleteAliasByID)
	f.Delete("/v1/holders/:holder_id/aliases/:alias_id/related-parties/:related_party_id", auth.Authorize(applicationName, "aliases", "delete"), http.ParseUUIDPathParameters("related-parties"), ah.DeleteRelatedParty)

	// Health
	f.Get("/health", libHTTP.Ping)

	// Version
	f.Get("/version", libHTTP.Version)

	// Doc Swagger
	f.Get("/swagger/*", WithSwaggerEnvConfig(), fiberSwagger.WrapHandler)

	f.Use(tlMid.EndTracingSpans)

	return f
}
