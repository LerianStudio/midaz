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

	// Metadata Indexes
	f.Post("/v1/organizations/:organization_id/ledgers/:ledger_id/metadata-indexes",
		auth.Authorize(midazName, "metadata-indexes", "post"),
		http.ParseUUIDPathParameters("metadata_index"),
		http.WithBody(new(mmodel.CreateMetadataIndexInput), mdi.CreateMetadataIndex))

	f.Get("/v1/organizations/:organization_id/ledgers/:ledger_id/metadata-indexes",
		auth.Authorize(midazName, "metadata-indexes", "get"),
		http.ParseUUIDPathParameters("metadata_index"),
		mdi.GetAllMetadataIndexes)

	f.Delete("/v1/organizations/:organization_id/ledgers/:ledger_id/metadata-indexes/:index_name",
		auth.Authorize(midazName, "metadata-indexes", "delete"),
		http.ParseUUIDPathParameters("metadata_index"),
		mdi.DeleteMetadataIndex)

	// Health
	f.Get("/health", libHTTP.Ping)

	// Version
	f.Get("/version", libHTTP.Version)

	f.Use(tlMid.EndTracingSpans)

	return f
}
