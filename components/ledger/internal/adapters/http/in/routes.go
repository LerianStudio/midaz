package in

import (
	"github.com/LerianStudio/lib-auth/v2/auth/middleware"
	libLog "github.com/LerianStudio/lib-commons/v2/commons/log"
	libHTTP "github.com/LerianStudio/lib-commons/v2/commons/net/http"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v2/commons/opentelemetry"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/LerianStudio/midaz/v3/pkg/net/http"
	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
)

const midazName = "midaz"

// NewRouter registers routes for the ledger component HTTP server.
func NewRouter(lg libLog.Logger, tl *libOpentelemetry.Telemetry, auth *middleware.AuthClient, mdi *MetadataIndexHandler) *fiber.App {
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

	// Register metadata index routes
	RegisterRoutesToApp(f, auth, mdi)

	// Health
	f.Get("/health", libHTTP.Ping)

	// Version
	f.Get("/version", libHTTP.Version)

	f.Use(tlMid.EndTracingSpans)

	return f
}

// RegisterRoutesToApp registers ledger routes (metadata indexes) to an existing Fiber app.
// This is used by the unified ledger server to consolidate all routes in a single port.
func RegisterRoutesToApp(f *fiber.App, auth *middleware.AuthClient, mdi *MetadataIndexHandler) {
	// Metadata Indexes
	f.Post("/v1/settings/metadata-indexes",
		auth.Authorize(midazName, "metadata-indexes", "post"),
		http.WithBody(new(mmodel.CreateMetadataIndexInput), mdi.CreateMetadataIndex))

	f.Get("/v1/settings/metadata-indexes",
		auth.Authorize(midazName, "metadata-indexes", "get"),
		mdi.GetAllMetadataIndexes)

	f.Delete("/v1/settings/metadata-indexes/:index_name",
		auth.Authorize(midazName, "metadata-indexes", "delete"),
		mdi.DeleteMetadataIndex)
}

// CreateRouteRegistrar returns a function that registers ledger routes to an existing Fiber app.
// This is used by the unified ledger server to consolidate all routes in a single port.
func CreateRouteRegistrar(auth *middleware.AuthClient, mdi *MetadataIndexHandler) func(*fiber.App) {
	return func(fiberApp *fiber.App) {
		RegisterRoutesToApp(fiberApp, auth, mdi)
	}
}
