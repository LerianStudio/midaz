package in

import (
	"fmt"
	"runtime/debug"

	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/LerianStudio/midaz/v3/pkg/net/http"

	"github.com/LerianStudio/lib-auth/v2/auth/middleware"
	libLog "github.com/LerianStudio/lib-commons/v2/commons/log"
	libHTTP "github.com/LerianStudio/lib-commons/v2/commons/net/http"
	libOpenTelemetry "github.com/LerianStudio/lib-commons/v2/commons/opentelemetry"
	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
	"github.com/gofiber/fiber/v2/middleware/recover"
	fiberSwagger "github.com/swaggo/fiber-swagger"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

const applicationName = "crm"

func NewRouter(lg libLog.Logger, tl *libOpenTelemetry.Telemetry, auth *middleware.AuthClient, hh *HolderHandler, ah *AliasHandler) *fiber.App {
	f := fiber.New(fiber.Config{
		DisableStartupMessage: true,
		ErrorHandler:          libHTTP.HandleFiberError,
	})

	// Panic recovery middleware - MUST be first to catch panics from all other middleware.
	// Note: HTTP recovery is always "keep running" in this phase.
	f.Use(recover.New(recover.Config{
		EnableStackTrace: true,
		StackTraceHandler: func(c *fiber.Ctx, e any) {
			stack := debug.Stack()
			panicValue := fmt.Sprintf("%v", e)

			// Record panic as a span event for observability
			span := trace.SpanFromContext(c.UserContext())
			span.AddEvent("panic.recovered", trace.WithAttributes(
				attribute.String("panic.value", panicValue),
				attribute.String("panic.stack", string(stack)),
				attribute.String("http.path", c.Path()),
				attribute.String("http.method", c.Method()),
			))

			lg.WithFields(
				"panic_value", panicValue,
				"stack_trace", string(stack),
				"path", c.Path(),
				"method", c.Method(),
			).Errorf("HTTP handler panic recovered: %v", e)
		},
	}))

	tlMid := libHTTP.NewTelemetryMiddleware(tl)

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

	// Health
	f.Get("/health", libHTTP.Ping)

	// Version
	f.Get("/version", libHTTP.Version)

	// Doc Swagger
	f.Get("/swagger/*", WithSwaggerEnvConfig(), fiberSwagger.WrapHandler)

	f.Use(tlMid.EndTracingSpans)

	return f
}
