package in

import (
	"github.com/LerianStudio/midaz/pkg/mlog"
	"github.com/LerianStudio/midaz/pkg/mopentelemetry"
	"github.com/LerianStudio/midaz/pkg/net/http"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
)

func NewRouter(lg mlog.Logger, tl *mopentelemetry.Telemetry, th *TrillianHandler) *fiber.App {
	f := fiber.New(fiber.Config{
		DisableStartupMessage: true,
	})
	tlMid := http.NewTelemetryMiddleware(tl)

	f.Use(tlMid.WithTelemetry(tl))
	f.Use(cors.New())
	f.Use(http.WithCorrelationID())
	f.Use(http.WithHTTPLogging(http.WithCustomLogger(lg)))

	// -- Routes --

	// Trillian
	f.Get("/v1/organizations/:organization_id/ledgers/:ledger_id/transactions/:transaction_id/check-entry", http.ParseUUIDPathParameters, th.CheckEntry)
	f.Get("/v1/organizations/:organization_id/ledgers/:ledger_id/transactions/:transaction_id/audit-logs", http.ParseUUIDPathParameters, th.AuditLogs)
	f.Get("/v1/organizations/:organization_id/ledgers/:ledger_id/transactions/:transaction_id/read-logs", http.ParseUUIDPathParameters, th.ReadLogs)

	// Health
	f.Get("/health", http.Ping)

	// Version
	f.Get("/version", http.Version)

	f.Use(tlMid.EndTracingSpans)

	return f
}
