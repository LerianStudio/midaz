package in

import (
	"github.com/LerianStudio/midaz/pkg/mcasdoor"
	"github.com/LerianStudio/midaz/pkg/mlog"
	"github.com/LerianStudio/midaz/pkg/mopentelemetry"
	"github.com/LerianStudio/midaz/pkg/net/http"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
	fiberSwagger "github.com/swaggo/fiber-swagger"
)

func NewRouter(lg mlog.Logger, tl *mopentelemetry.Telemetry, cc *mcasdoor.CasdoorConnection, th *TrillianHandler) *fiber.App {
	f := fiber.New(fiber.Config{
		DisableStartupMessage: true,
	})
	tlMid := http.NewTelemetryMiddleware(tl)

	f.Use(tlMid.WithTelemetry(tl))
	f.Use(cors.New())
	f.Use(http.WithHTTPLogging(http.WithCustomLogger(lg)))
	jwt := http.NewJWTMiddleware(cc)

	// -- Routes --

	// Trillian
	f.Get("/v1/organizations/:organization_id/ledgers/:ledger_id/audit/:audit_id/audit-logs", jwt.ProtectHTTP(), jwt.WithPermissionHTTP("audit"), http.ParseUUIDPathParameters, th.AuditLogs)
	f.Get("/v1/organizations/:organization_id/ledgers/:ledger_id/audit/:audit_id/read-logs", jwt.ProtectHTTP(), jwt.WithPermissionHTTP("audit"), http.ParseUUIDPathParameters, th.ReadLogs)

	// Health
	f.Get("/health", http.Ping)

	// Version
	f.Get("/version", http.Version)

	// Doc
	f.Get("/swagger/*", WithSwaggerEnvConfig(), fiberSwagger.WrapHandler)

	f.Use(tlMid.EndTracingSpans)

	return f
}
