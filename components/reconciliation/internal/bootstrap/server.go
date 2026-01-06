package bootstrap

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
	"github.com/gofiber/fiber/v2/middleware/helmet"
	"github.com/gofiber/fiber/v2/middleware/limiter"
	"github.com/gofiber/fiber/v2/middleware/recover"
	"github.com/gofiber/fiber/v2/middleware/requestid"

	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	libLog "github.com/LerianStudio/lib-commons/v2/commons/log"
	libHTTP "github.com/LerianStudio/lib-commons/v2/commons/net/http"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v2/commons/opentelemetry"

	"github.com/LerianStudio/midaz/v3/components/reconciliation/internal/domain"
	"github.com/LerianStudio/midaz/v3/components/reconciliation/internal/engine"
	"github.com/LerianStudio/midaz/v3/components/reconciliation/internal/reportstore"
	"github.com/shopspring/decimal"
)

// Sentinel errors for HTTP server operations.
var (
	ErrServerListen   = errors.New("server listen failed")
	ErrJSONResponse   = errors.New("JSON response failed")
	ErrStringResponse = errors.New("string response failed")
)

// HTTP server configuration constants.
const (
	httpReadTimeout        = 30 * time.Second
	httpWriteTimeout       = 30 * time.Second
	httpIdleTimeout        = 60 * time.Second
	httpBodyLimit          = 1 * 1024 * 1024 // 1MB
	rateLimitReadMax       = 60
	rateLimitReadExpiry    = 60 * time.Second
	rateLimitTriggerMax    = 1
	rateLimitTriggerExpiry = 60 * time.Second
	triggerRunTimeout      = 2 * time.Minute
	maxReportsLimit        = 100
	truncateIDMinLen       = 12
	truncateIDPrefixLen    = 8
	truncateIDSuffixLen    = 4
	statusUnknown          = "UNKNOWN"
)

// NOTE: truncateID constants (truncateIDMinLen=12, truncateIDPrefixLen=8, truncateIDSuffixLen=4)
// are compile-time values that satisfy the invariants:
// - All values are non-negative
// - truncateIDPrefixLen + truncateIDSuffixLen (12) <= truncateIDMinLen (12)
// These invariants are guaranteed by the constant definitions above.

// HTTPServer provides status endpoints
type HTTPServer struct {
	app       *fiber.App
	address   string
	engine    *engine.ReconciliationEngine
	logger    libLog.Logger
	telemetry *libOpentelemetry.Telemetry
	store     reportstore.Store
}

// NewHTTPServer creates a new HTTP server with security middleware
func NewHTTPServer(
	address string,
	eng *engine.ReconciliationEngine,
	logger libLog.Logger,
	telemetry *libOpentelemetry.Telemetry,
	version string,
	envName string,
	store reportstore.Store,
) *HTTPServer {
	app := fiber.New(fiber.Config{
		DisableStartupMessage: true,
		ErrorHandler:          libHTTP.HandleFiberError,
		ReadTimeout:           httpReadTimeout,
		WriteTimeout:          httpWriteTimeout,
		IdleTimeout:           httpIdleTimeout,
		BodyLimit:             httpBodyLimit,
	})

	// Telemetry middleware for distributed tracing
	tlMid := libHTTP.NewTelemetryMiddleware(telemetry)
	app.Use(tlMid.WithTelemetry(telemetry))

	// CORS and logging middleware
	app.Use(cors.New())
	app.Use(libHTTP.WithHTTPLogging(libHTTP.WithCustomLogger(logger)))

	// Security middleware
	app.Use(recover.New())
	app.Use(requestid.New())
	app.Use(helmet.New())

	server := &HTTPServer{
		app:       app,
		address:   address,
		engine:    eng,
		logger:    logger,
		telemetry: telemetry,
		store:     store,
	}

	// Public health endpoints (no auth required)
	app.Get("/health", func(c *fiber.Ctx) error {
		return c.JSON(fiber.Map{"status": "ok"})
	})

	app.Get("/version", func(c *fiber.Ctx) error {
		return c.JSON(fiber.Map{"version": version, "env": envName})
	})

	// Rate limiter for read endpoints (per-IP, 60 requests/minute)
	readLimiter := limiter.New(limiter.Config{
		Max:        rateLimitReadMax,
		Expiration: rateLimitReadExpiry,
		KeyGenerator: func(c *fiber.Ctx) string {
			return c.IP()
		},
		LimitReached: func(c *fiber.Ctx) error {
			return c.Status(http.StatusTooManyRequests).JSON(fiber.Map{
				"error": "Rate limit exceeded. Please try again later.",
			})
		},
	})

	// Reconciliation endpoints
	// TODO(review): Add authentication middleware for production deployment (security-reviewer, 2025-12-29, Critical)
	// Example: app.Use("/reconciliation", auth.Authorize("reconciliation", "status", "get"))
	app.Get("/reconciliation/status", readLimiter, server.getStatus)
	app.Get("/reconciliation/report", readLimiter, server.getReport)
	app.Get("/reconciliation/report/human", readLimiter, server.getHumanReport)
	app.Get("/reconciliation/reports", readLimiter, server.getReports)

	// Rate-limited manual trigger endpoint
	// Global limit: 1 request per minute to prevent DoS
	app.Post("/reconciliation/run",
		limiter.New(limiter.Config{
			Max:        rateLimitTriggerMax,
			Expiration: rateLimitTriggerExpiry,
			KeyGenerator: func(c *fiber.Ctx) string {
				return "global" // Global rate limit, not per-IP
			},
			LimitReached: func(c *fiber.Ctx) error {
				return c.Status(http.StatusTooManyRequests).JSON(fiber.Map{
					"error": "Rate limit exceeded. Reconciliation can only be triggered once per minute.",
				})
			},
		}),
		server.triggerRun,
	)

	// End tracing spans middleware (must be last to properly close spans)
	app.Use(tlMid.EndTracingSpans)

	return server
}

// Run starts the HTTP server
func (s *HTTPServer) Run(l *libCommons.Launcher) error {
	s.logger.Infof("HTTP server starting on %s", s.address)

	if err := s.app.Listen(s.address); err != nil {
		return fmt.Errorf("%w: %w", ErrServerListen, err)
	}

	return nil
}

func (s *HTTPServer) getStatus(c *fiber.Ctx) error {
	report := s.engine.GetLastReport()
	if report == nil {
		return c.Status(http.StatusServiceUnavailable).JSON(fiber.Map{
			"status":  statusUnknown,
			"message": "No reconciliation run completed yet",
		})
	}

	statusCode := http.StatusOK
	if report.Status == domain.StatusCritical || report.Status == domain.StatusError {
		statusCode = http.StatusServiceUnavailable
	}

	// Build checks map with nil safety
	checks := fiber.Map{
		"balance":      getCheckStatus(report.BalanceCheck),
		"double_entry": getCheckStatus(report.DoubleEntryCheck),
		"referential":  getCheckStatus(report.ReferentialCheck),
		"sync":         getCheckStatus(report.SyncCheck),
		"orphans":      getCheckStatus(report.OrphanCheck),
		"metadata":     getCheckStatus(report.MetadataCheck),
		"dlq":          getCheckStatus(report.DLQCheck),
		"outbox":       getCheckStatus(report.OutboxCheck),
		"redis":        getCheckStatus(report.RedisCheck),
		"cross_db":     getCheckStatus(report.CrossDBCheck),
		"crm_alias":    getCheckStatus(report.CRMAliasCheck),
	}

	return c.Status(statusCode).JSON(fiber.Map{
		"status":    report.Status,
		"timestamp": report.Timestamp,
		"duration":  report.Duration,
		"checks":    checks,
	})
}

// checkStatusGetter is implemented by check result types that can expose their reconciliation status.
type checkStatusGetter interface {
	GetStatus() domain.ReconciliationStatus
}

// getCheckStatus returns the status from a check result or "UNKNOWN" if nil
func getCheckStatus(check checkStatusGetter) string {
	if check == nil {
		return statusUnknown
	}

	return string(check.GetStatus())
}

func (s *HTTPServer) getReport(c *fiber.Ctx) error {
	report := s.engine.GetLastReport()
	if report == nil && s.store != nil {
		loaded, err := s.store.LoadLatest(c.Context())
		if err == nil && loaded != nil {
			report = loaded
		}
	}

	if report == nil {
		return c.Status(http.StatusNotFound).JSON(fiber.Map{
			"error": "No reconciliation report available",
		})
	}

	return c.JSON(report)
}

func (s *HTTPServer) getReports(c *fiber.Ctx) error {
	if s.store == nil {
		return c.Status(http.StatusNotFound).JSON(fiber.Map{
			"error": "Report store not configured",
		})
	}

	limit := 10

	if value := c.Query("limit"); value != "" {
		parsed, err := strconv.Atoi(value)
		if err != nil || parsed <= 0 {
			return c.Status(http.StatusBadRequest).JSON(fiber.Map{
				"error": "Invalid limit; must be a positive integer",
			})
		}

		if parsed > maxReportsLimit {
			parsed = maxReportsLimit
		}

		limit = parsed
	}

	reports, err := s.store.LoadRecent(c.Context(), limit)
	if err != nil {
		return c.Status(http.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to load reports",
		})
	}

	return c.JSON(fiber.Map{
		"reports": reports,
		"count":   len(reports),
	})
}

func (s *HTTPServer) getHumanReport(c *fiber.Ctx) error {
	report := s.engine.GetLastReport()
	if report == nil {
		c.Set("Content-Type", "text/html; charset=utf-8")

		return c.Status(http.StatusNotFound).SendString(`<!DOCTYPE html>
<html><head><title>Reconciliation Report</title></head>
<body style="font-family: system-ui, sans-serif; max-width: 800px; margin: 40px auto; padding: 20px;">
<h1>No Report Available</h1>
<p>No reconciliation run has completed yet. Please wait for the next scheduled run or trigger one manually.</p>
</body></html>`)
	}

	c.Set("Content-Type", "text/html; charset=utf-8")

	return c.SendString(s.buildHumanReportHTML(report))
}

// statusColorMap maps reconciliation status to CSS colors.
//
// Colors are chosen from a Tailwind-like palette and MUST remain deterministic:
// these values are asserted in unit tests (see server_test.go) to avoid new
// statuses silently falling back to UNKNOWN.
var statusColorMap = map[domain.ReconciliationStatus]string{
	domain.StatusHealthy:  "#22c55e",
	domain.StatusWarning:  "#eab308",
	domain.StatusCritical: "#ef4444",
	// ERROR indicates the reconciliation process itself failed (distinct from CRITICAL checks).
	domain.StatusError: "#b91c1c",
	// SKIPPED indicates the check wasn't executed (neutral/informational).
	domain.StatusSkipped: "#3b82f6",
	domain.StatusUnknown: "#6b7280",
}

// getStatusColor returns the CSS color for a reconciliation status
func getStatusColor(status domain.ReconciliationStatus) string {
	if c, ok := statusColorMap[status]; ok {
		return c
	}

	return statusColorMap[domain.StatusUnknown]
}

func (s *HTTPServer) buildHumanReportHTML(report *domain.ReconciliationReport) string {
	var html strings.Builder

	writeHTMLHeader(&html)
	writeReportHeader(&html, report)
	writeSummaryCards(&html, report)
	writeBalanceCheckSection(&html, report.BalanceCheck)
	writeDoubleEntrySection(&html, report.DoubleEntryCheck)
	writeReferentialSection(&html, report.ReferentialCheck)
	writeSyncCheckSection(&html, report.SyncCheck)
	writeOrphanCheckSection(&html, report.OrphanCheck)
	writeMetadataSection(&html, report.MetadataCheck)
	writeDLQSection(&html, report.DLQCheck)
	writeOutboxSection(&html, report.OutboxCheck)
	writeRedisSection(&html, report.RedisCheck)
	writeCrossDBSection(&html, report.CrossDBCheck)
	writeCRMAliasSection(&html, report.CRMAliasCheck)
	writeEntityCountsSection(&html, report.EntityCounts)
	html.WriteString(`</body></html>`)

	return html.String()
}

func writeHTMLHeader(html *strings.Builder) {
	html.WriteString(`<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width, initial-scale=1.0">
<title>Reconciliation Report</title>
<style>
  * { box-sizing: border-box; }
  body { font-family: system-ui, -apple-system, sans-serif; max-width: 1200px; margin: 0 auto; padding: 20px; background: #f8fafc; color: #1e293b; }
  h1 { margin-bottom: 8px; }
  h2 { margin-top: 32px; border-bottom: 2px solid #e2e8f0; padding-bottom: 8px; }
  h3 { margin-top: 24px; color: #475569; }
  .badge { display: inline-block; padding: 4px 12px; border-radius: 20px; font-weight: 600; color: white; font-size: 14px; }
  .meta { color: #64748b; margin-bottom: 24px; }
  .summary { display: grid; grid-template-columns: repeat(auto-fit, minmax(200px, 1fr)); gap: 16px; margin: 24px 0; }
  .card { background: white; border-radius: 12px; padding: 20px; box-shadow: 0 1px 3px rgba(0,0,0,0.1); }
  .card-title { font-size: 14px; color: #64748b; margin-bottom: 8px; }
  .card-value { font-size: 28px; font-weight: 700; }
  .card-sub { font-size: 12px; color: #94a3b8; margin-top: 4px; }
  .section { background: white; border-radius: 12px; padding: 24px; margin: 16px 0; box-shadow: 0 1px 3px rgba(0,0,0,0.1); }
  .section-header { display: flex; justify-content: space-between; align-items: center; margin-bottom: 16px; }
  .section-title { font-size: 18px; font-weight: 600; margin: 0; }
  table { width: 100%; border-collapse: collapse; margin-top: 12px; font-size: 14px; }
  th, td { text-align: left; padding: 10px 12px; border-bottom: 1px solid #e2e8f0; }
  th { background: #f8fafc; font-weight: 600; color: #475569; }
  tr:hover { background: #f8fafc; }
  .mono { font-family: ui-monospace, monospace; font-size: 12px; }
  .num { text-align: right; font-variant-numeric: tabular-nums; }
  .empty { color: #94a3b8; font-style: italic; text-align: center; padding: 24px; }
  @media (max-width: 600px) { .summary { grid-template-columns: 1fr; } }
</style>
</head>
<body>
`)
}

func writeReportHeader(html *strings.Builder, report *domain.ReconciliationReport) {
	fmt.Fprintf(html, `<h1>Reconciliation Report</h1>
<p class="meta">Generated: %s &nbsp;|&nbsp; Duration: %s</p>
<span class="badge" style="background: %s;">%s</span>
`,
		report.Timestamp.Format("2006-01-02 15:04:05 MST"),
		report.Duration,
		getStatusColor(report.Status),
		report.Status)
}

func writeSummaryCards(html *strings.Builder, report *domain.ReconciliationReport) {
	html.WriteString(`<div class="summary">`)

	if report.EntityCounts != nil {
		fmt.Fprintf(html, `<div class="card"><div class="card-title">Transactions</div><div class="card-value">%d</div><div class="card-sub">Settled: %d | Unsettled: %d</div></div>`,
			report.EntityCounts.Transactions, report.SettledTransactions, report.UnsettledTransactions)
		fmt.Fprintf(html, `<div class="card"><div class="card-title">Balances</div><div class="card-value">%d</div></div>`, report.EntityCounts.Balances)
		fmt.Fprintf(html, `<div class="card"><div class="card-title">Accounts</div><div class="card-value">%d</div></div>`, report.EntityCounts.Accounts)
		fmt.Fprintf(html, `<div class="card"><div class="card-title">Operations</div><div class="card-value">%d</div></div>`, report.EntityCounts.Operations)
	}

	if report.DLQCheck != nil {
		fmt.Fprintf(html, `<div class="card"><div class="card-title">DLQ Entries</div><div class="card-value">%d</div><div class="card-sub">Txn: %d | Op: %d</div></div>`,
			report.DLQCheck.Total, report.DLQCheck.TransactionEntries, report.DLQCheck.OperationEntries)
	}

	if report.OutboxCheck != nil {
		fmt.Fprintf(html, `<div class="card"><div class="card-title">Outbox Backlog</div><div class="card-value">%d</div><div class="card-sub">Pending: %d | Failed: %d</div></div>`,
			report.OutboxCheck.Pending+report.OutboxCheck.Failed+report.OutboxCheck.Processing,
			report.OutboxCheck.Pending, report.OutboxCheck.Failed)
	}

	if report.RedisCheck != nil {
		redisIssues := report.RedisCheck.MissingRedis + report.RedisCheck.ValueMismatches + report.RedisCheck.VersionMismatches
		fmt.Fprintf(html, `<div class="card"><div class="card-title">Redis Mismatches</div><div class="card-value">%d</div><div class="card-sub">Sampled: %d</div></div>`,
			redisIssues, report.RedisCheck.SampledBalances)
	}

	html.WriteString(`</div>`)
}

func writeBalanceCheckSection(html *strings.Builder, bc *domain.BalanceCheckResult) {
	html.WriteString(`<div class="section">`)

	if bc == nil {
		html.WriteString(`<p class="empty">Balance check not available</p></div>`)
		return
	}

	fmt.Fprintf(html, `<div class="section-header"><h3 class="section-title">Balance Check</h3><span class="badge" style="background: %s;">%s</span></div>`, getStatusColor(bc.Status), bc.Status)
	fmt.Fprintf(html, `<p>Checked <strong>%d</strong> balances. Found <strong>%d</strong> discrepancies (%.2f%%).</p>`,
		bc.TotalBalances, bc.BalancesWithDiscrepancy, bc.DiscrepancyPercentage)
	fmt.Fprintf(html, `<p>On-hold discrepancies: <strong>%d</strong> (%.2f%%). Negative balances: <strong>%d</strong> available, <strong>%d</strong> on-hold.</p>`,
		bc.OnHoldWithDiscrepancy, bc.OnHoldDiscrepancyPct, bc.NegativeAvailable, bc.NegativeOnHold)

	if len(bc.Discrepancies) > 0 {
		html.WriteString(`<table><thead><tr><th>Account ID</th><th>Asset</th><th class="num">Current</th><th class="num">Expected</th><th class="num">Diff</th><th class="num">Ops</th></tr></thead><tbody>`)

		for _, d := range bc.Discrepancies {
			fmt.Fprintf(html, "<tr><td class=\"mono\">%s</td><td>%s</td><td class=\"num\">%s</td><td class=\"num\" style=\"color:%s;\">%s</td><td class=\"num\">%s</td><td class=\"num\">%d</td></tr>",
				truncateID(d.AccountID), d.AssetCode, formatDecimal(d.CurrentBalance), formatDecimal(d.ExpectedBalance),
				ternary(d.Discrepancy.IsNegative(), "#ef4444", "#22c55e"), formatSignedDecimal(d.Discrepancy), d.OperationCount)
		}

		html.WriteString(`</tbody></table>`)
	}

	if len(bc.OnHoldDiscrepancies) > 0 {
		html.WriteString(`<table><thead><tr><th>Account ID</th><th>Asset</th><th class="num">On-Hold</th><th class="num">Expected</th><th class="num">Diff</th><th class="num">Ops</th></tr></thead><tbody>`)

		for _, d := range bc.OnHoldDiscrepancies {
			fmt.Fprintf(html, "<tr><td class=\"mono\">%s</td><td>%s</td><td class=\"num\">%s</td><td class=\"num\" style=\"color:%s;\">%s</td><td class=\"num\">%s</td><td class=\"num\">%d</td></tr>",
				truncateID(d.AccountID), d.AssetCode, formatDecimal(d.CurrentOnHold), formatDecimal(d.ExpectedOnHold),
				ternary(d.Discrepancy.IsNegative(), "#ef4444", "#22c55e"), formatSignedDecimal(d.Discrepancy), d.OperationCount)
		}

		html.WriteString(`</tbody></table>`)
	}

	if len(bc.NegativeBalances) > 0 {
		html.WriteString(`<table><thead><tr><th>Account ID</th><th>Asset</th><th class="num">Available</th><th class="num">On-Hold</th></tr></thead><tbody>`)

		for _, n := range bc.NegativeBalances {
			fmt.Fprintf(html, "<tr><td class=\"mono\">%s</td><td>%s</td><td class=\"num\" style=\"color:%s;\">%s</td><td class=\"num\" style=\"color:%s;\">%s</td></tr>",
				truncateID(n.AccountID), n.AssetCode,
				ternary(n.Available.IsNegative(), "#ef4444", "#22c55e"), formatDecimal(n.Available),
				ternary(n.OnHold.IsNegative(), "#ef4444", "#22c55e"), formatDecimal(n.OnHold))
		}

		html.WriteString(`</tbody></table>`)
	}

	html.WriteString(`</div>`)
}

func writeDoubleEntrySection(html *strings.Builder, de *domain.DoubleEntryCheckResult) {
	html.WriteString(`<div class="section">`)

	if de == nil {
		html.WriteString(`<p class="empty">Double-entry check not available</p></div>`)
		return
	}

	fmt.Fprintf(html, `<div class="section-header"><h3 class="section-title">Double-Entry Check</h3><span class="badge" style="background: %s;">%s</span></div>`, getStatusColor(de.Status), de.Status)
	fmt.Fprintf(html, `<p>Verified <strong>%d</strong> transactions. Found <strong>%d</strong> unbalanced (%.2f%%), <strong>%d</strong> without operations.</p>`,
		de.TotalTransactions, de.UnbalancedTransactions, de.UnbalancedPercentage, de.TransactionsNoOperations)

	if len(de.Imbalances) > 0 {
		html.WriteString(`<table><thead><tr><th>Transaction ID</th><th>Status</th><th>Asset</th><th class="num">Credits</th><th class="num">Debits</th><th class="num">Imbalance</th></tr></thead><tbody>`)

		for _, i := range de.Imbalances {
			fmt.Fprintf(html, "<tr><td class=\"mono\">%s</td><td>%s</td><td>%s</td><td class=\"num\">%d</td><td class=\"num\">%d</td><td class=\"num\" style=\"color:%s;\">%+d</td></tr>",
				truncateID(i.TransactionID),
				i.Status,
				i.AssetCode,
				i.TotalCredits,
				i.TotalDebits,
				ternary(i.Imbalance != 0, "#ef4444", "#22c55e"),
				i.Imbalance,
			)
		}

		html.WriteString(`</tbody></table>`)
	}

	html.WriteString(`</div>`)
}

func writeReferentialSection(html *strings.Builder, rc *domain.ReferentialCheckResult) {
	html.WriteString(`<div class="section">`)

	if rc == nil {
		html.WriteString(`<p class="empty">Referential check not available</p></div>`)
		return
	}

	fmt.Fprintf(html, `<div class="section-header"><h3 class="section-title">Referential Integrity</h3><span class="badge" style="background: %s;">%s</span></div>`, getStatusColor(rc.Status), rc.Status)
	fmt.Fprintf(html, `<p>Orphans: Ledgers=%d, Assets=%d, Accounts=%d, Operations=%d, Portfolios=%d, OpsWithoutBalance=%d</p>`,
		rc.OrphanLedgers, rc.OrphanAssets, rc.OrphanAccounts, rc.OrphanOperations, rc.OrphanPortfolios, rc.OperationsWithoutBalance)

	if len(rc.Orphans) > 0 {
		html.WriteString(`<table><thead><tr><th>Entity Type</th><th>Entity ID</th><th>Missing Reference</th><th>Reference ID</th></tr></thead><tbody>`)

		for _, o := range rc.Orphans {
			fmt.Fprintf(html, `<tr><td>%s</td><td class="mono">%s</td><td>%s</td><td class="mono">%s</td></tr>`,
				o.EntityType, truncateID(o.EntityID), o.ReferenceType, truncateID(o.ReferenceID))
		}

		html.WriteString(`</tbody></table>`)
	}

	html.WriteString(`</div>`)
}

func writeSyncCheckSection(html *strings.Builder, sc *domain.SyncCheckResult) {
	html.WriteString(`<div class="section">`)

	if sc == nil {
		html.WriteString(`<p class="empty">Sync check not available</p></div>`)
		return
	}

	fmt.Fprintf(html, `<div class="section-header"><h3 class="section-title">Sync Check (Redis ↔ PostgreSQL)</h3><span class="badge" style="background: %s;">%s</span></div>`, getStatusColor(sc.Status), sc.Status)
	fmt.Fprintf(html, `<p>Version mismatches: <strong>%d</strong>, Stale balances: <strong>%d</strong></p>`, sc.VersionMismatches, sc.StaleBalances)

	if len(sc.Issues) > 0 {
		html.WriteString(`<table><thead><tr><th>Balance ID</th><th>Asset</th><th class="num">DB Ver</th><th class="num">Op Ver</th><th class="num">Stale (sec)</th></tr></thead><tbody>`)

		for _, i := range sc.Issues {
			fmt.Fprintf(html, "<tr><td class=\"mono\">%s</td><td>%s</td><td class=\"num\">%d</td><td class=\"num\">%d</td><td class=\"num\">%d</td></tr>",
				truncateID(i.BalanceID),
				i.AssetCode,
				i.DBVersion,
				i.MaxOpVersion,
				i.StalenessSeconds,
			)
		}

		html.WriteString(`</tbody></table>`)
	}

	html.WriteString(`</div>`)
}

func writeOrphanCheckSection(html *strings.Builder, oc *domain.OrphanCheckResult) {
	html.WriteString(`<div class="section">`)

	if oc == nil {
		html.WriteString(`<p class="empty">Orphan check not available</p></div>`)
		return
	}

	fmt.Fprintf(html, `<div class="section-header"><h3 class="section-title">Orphan Transactions</h3><span class="badge" style="background: %s;">%s</span></div>`, getStatusColor(oc.Status), oc.Status)
	fmt.Fprintf(html, `<p>Orphan transactions: <strong>%d</strong>, Partial transactions: <strong>%d</strong></p>`, oc.OrphanTransactions, oc.PartialTransactions)

	if len(oc.Orphans) > 0 {
		html.WriteString(`<table><thead><tr><th>Transaction ID</th><th>Status</th><th>Asset</th><th class="num">Amount</th><th class="num">Ops</th><th>Created</th></tr></thead><tbody>`)

		for _, o := range oc.Orphans {
			fmt.Fprintf(html, `<tr><td class="mono">%s</td><td>%s</td><td>%s</td><td class="num">%d</td><td class="num">%d</td><td>%s</td></tr>`,
				truncateID(o.TransactionID), o.Status, o.AssetCode, o.Amount, o.OperationCount, o.CreatedAt.Format("2006-01-02 15:04"))
		}

		html.WriteString(`</tbody></table>`)
	}

	html.WriteString(`</div>`)
}

func writeMetadataSection(html *strings.Builder, mc *domain.MetadataCheckResult) {
	html.WriteString(`<div class="section">`)

	if mc == nil {
		html.WriteString(`<p class="empty">Metadata check not available</p></div>`)
		return
	}

	fmt.Fprintf(html, `<div class="section-header"><h3 class="section-title">Metadata Sync (PostgreSQL ↔ MongoDB)</h3><span class="badge" style="background: %s;">%s</span></div>`, getStatusColor(mc.Status), mc.Status)
	fmt.Fprintf(html, `<p>PostgreSQL: <strong>%d</strong> records, MongoDB: <strong>%d</strong> records, Missing: <strong>%d</strong></p>`,
		mc.PostgreSQLCount, mc.MongoDBCount, mc.MissingCount)
	fmt.Fprintf(html, `<p>Missing entity IDs: <strong>%d</strong>, Duplicates: <strong>%d</strong>, Missing required fields: <strong>%d</strong>, Empty metadata: <strong>%d</strong></p>`,
		mc.MissingEntityIDs, mc.DuplicateEntityIDs, mc.MissingRequiredFields, mc.EmptyMetadata)

	if len(mc.CollectionSummaries) > 0 {
		html.WriteString(`<table><thead><tr><th>Collection</th><th class="num">Total</th><th class="num">Empty</th><th class="num">Missing ID</th><th class="num">Duplicates</th></tr></thead><tbody>`)

		for _, s := range mc.CollectionSummaries {
			fmt.Fprintf(html, "<tr><td>%s</td><td class=\"num\">%d</td><td class=\"num\">%d</td><td class=\"num\">%d</td><td class=\"num\">%d</td></tr>",
				s.Collection,
				s.TotalDocuments,
				s.EmptyMetadata,
				s.MissingEntityIDs,
				s.DuplicateEntityIDs,
			)
		}

		html.WriteString(`</tbody></table>`)
	}

	if len(mc.MissingEntities) > 0 {
		html.WriteString(`<table><thead><tr><th>Entity Type</th><th>Entity ID</th><th>Reason</th></tr></thead><tbody>`)

		for _, m := range mc.MissingEntities {
			fmt.Fprintf(html, `<tr><td>%s</td><td class="mono">%s</td><td>%s</td></tr>`, m.EntityType, truncateID(m.EntityID), m.Reason)
		}

		html.WriteString(`</tbody></table>`)
	}

	html.WriteString(`</div>`)
}

func writeDLQSection(html *strings.Builder, dc *domain.DLQCheckResult) {
	html.WriteString(`<div class="section">`)

	if dc == nil {
		html.WriteString(`<p class="empty">DLQ check not available</p></div>`)
		return
	}

	fmt.Fprintf(html, `<div class="section-header"><h3 class="section-title">Metadata Outbox DLQ</h3><span class="badge" style="background: %s;">%s</span></div>`, getStatusColor(dc.Status), dc.Status)
	fmt.Fprintf(html, `<p>Total DLQ entries: <strong>%d</strong> (Transactions: <strong>%d</strong>, Operations: <strong>%d</strong>)</p>`,
		dc.Total, dc.TransactionEntries, dc.OperationEntries)

	if len(dc.Entries) > 0 {
		html.WriteString(`<table><thead><tr><th>Entry ID</th><th>Entity Type</th><th>Entity ID</th><th class="num">Retries</th><th>Created</th><th>Last Error</th></tr></thead><tbody>`)

		for _, e := range dc.Entries {
			fmt.Fprintf(html, `<tr><td class="mono">%s</td><td>%s</td><td class="mono">%s</td><td class="num">%d</td><td>%s</td><td>%s</td></tr>`,
				truncateID(e.ID), e.EntityType, truncateID(e.EntityID), e.RetryCount, e.CreatedAt.Format("2006-01-02 15:04"), e.LastError)
		}

		html.WriteString(`</tbody></table>`)
	}

	html.WriteString(`</div>`)
}

func writeOutboxSection(html *strings.Builder, oc *domain.OutboxCheckResult) {
	html.WriteString(`<div class="section">`)

	if oc == nil {
		html.WriteString(`<p class="empty">Outbox check not available</p></div>`)
		return
	}

	fmt.Fprintf(html, `<div class="section-header"><h3 class="section-title">Metadata Outbox Health</h3><span class="badge" style="background: %s;">%s</span></div>`, getStatusColor(oc.Status), oc.Status)
	fmt.Fprintf(html, `<p>Pending: <strong>%d</strong>, Processing: <strong>%d</strong>, Failed: <strong>%d</strong>, Stale: <strong>%d</strong></p>`,
		oc.Pending, oc.Processing, oc.Failed, oc.StaleProcessing)

	if len(oc.Entries) > 0 {
		html.WriteString(`<table><thead><tr><th>Entry ID</th><th>Status</th><th>Entity</th><th class="num">Retries</th><th>Created</th><th>Last Error</th></tr></thead><tbody>`)

		for _, e := range oc.Entries {
			fmt.Fprintf(html, `<tr><td class="mono">%s</td><td>%s</td><td class="mono">%s</td><td class="num">%d</td><td>%s</td><td>%s</td></tr>`,
				truncateID(e.ID), e.Status, truncateID(e.EntityID), e.RetryCount, e.CreatedAt.Format("2006-01-02 15:04"), e.LastError)
		}

		html.WriteString(`</tbody></table>`)
	}

	html.WriteString(`</div>`)
}

func writeRedisSection(html *strings.Builder, rc *domain.RedisCheckResult) {
	html.WriteString(`<div class="section">`)

	if rc == nil {
		html.WriteString(`<p class="empty">Redis check not available</p></div>`)
		return
	}

	fmt.Fprintf(html, `<div class="section-header"><h3 class="section-title">Redis ↔ PostgreSQL Balance</h3><span class="badge" style="background: %s;">%s</span></div>`, getStatusColor(rc.Status), rc.Status)
	fmt.Fprintf(html, `<p>Sampled: <strong>%d</strong>, Missing: <strong>%d</strong>, Value mismatches: <strong>%d</strong>, Version mismatches: <strong>%d</strong></p>`,
		rc.SampledBalances, rc.MissingRedis, rc.ValueMismatches, rc.VersionMismatches)
	fmt.Fprintf(html, `<p>Sync queue depth: <strong>%d</strong></p>`, rc.SyncQueueDepth)

	if len(rc.Discrepancies) > 0 {
		html.WriteString(`<table><thead><tr><th>Account ID</th><th>Asset</th><th class="num">DB Avail</th><th class="num">Redis Avail</th><th class="num">DB Hold</th><th class="num">Redis Hold</th></tr></thead><tbody>`)

		for _, d := range rc.Discrepancies {
			fmt.Fprintf(html, "<tr><td class=\"mono\">%s</td><td>%s</td><td class=\"num\">%s</td><td class=\"num\">%s</td><td class=\"num\">%s</td><td class=\"num\">%s</td></tr>",
				truncateID(d.AccountID),
				d.AssetCode,
				formatDecimal(d.DBAvailable),
				formatDecimal(d.RedisAvailable),
				formatDecimal(d.DBOnHold),
				formatDecimal(d.RedisOnHold),
			)
		}

		html.WriteString(`</tbody></table>`)
	}

	html.WriteString(`</div>`)
}

func writeCrossDBSection(html *strings.Builder, cc *domain.CrossDBCheckResult) {
	html.WriteString(`<div class="section">`)

	if cc == nil {
		html.WriteString(`<p class="empty">Cross-DB check not available</p></div>`)
		return
	}

	fmt.Fprintf(html, `<div class="section-header"><h3 class="section-title">Cross-DB References</h3><span class="badge" style="background: %s;">%s</span></div>`, getStatusColor(cc.Status), cc.Status)
	fmt.Fprintf(html, `<p>Missing Accounts: <strong>%d</strong>, Ledgers: <strong>%d</strong>, Organizations: <strong>%d</strong></p>`,
		cc.MissingAccounts, cc.MissingLedgers, cc.MissingOrganizations)

	if len(cc.Samples) > 0 {
		html.WriteString(`<table><thead><tr><th>Type</th><th>Reference ID</th><th>Source</th></tr></thead><tbody>`)

		for _, s := range cc.Samples {
			fmt.Fprintf(html, `<tr><td>%s</td><td class="mono">%s</td><td>%s</td></tr>`,
				s.RefType, truncateID(s.RefID), s.Source)
		}

		html.WriteString(`</tbody></table>`)
	}

	html.WriteString(`</div>`)
}

func writeCRMAliasSection(html *strings.Builder, cc *domain.CRMAliasCheckResult) {
	html.WriteString(`<div class="section">`)

	if cc == nil {
		html.WriteString(`<p class="empty">CRM alias check not available</p></div>`)
		return
	}

	fmt.Fprintf(html, `<div class="section-header"><h3 class="section-title">CRM Alias References</h3><span class="badge" style="background: %s;">%s</span></div>`, getStatusColor(cc.Status), cc.Status)
	fmt.Fprintf(html, `<p>Missing Ledgers: <strong>%d</strong>, Missing Accounts: <strong>%d</strong></p>`,
		cc.MissingLedgerIDs, cc.MissingAccountIDs)

	if len(cc.Samples) > 0 {
		html.WriteString(`<table><thead><tr><th>Alias ID</th><th>Issue</th><th>Ledger ID</th><th>Account ID</th></tr></thead><tbody>`)

		for _, s := range cc.Samples {
			fmt.Fprintf(html, `<tr><td class="mono">%s</td><td>%s</td><td class="mono">%s</td><td class="mono">%s</td></tr>`,
				truncateID(s.AliasID), s.Issue, truncateID(s.LedgerID), truncateID(s.AccountID))
		}

		html.WriteString(`</tbody></table>`)
	}

	html.WriteString(`</div>`)
}

func writeEntityCountsSection(html *strings.Builder, ec *domain.EntityCounts) {
	if ec == nil {
		return
	}

	html.WriteString(`<div class="section">`)
	html.WriteString(`<div class="section-header"><h3 class="section-title">Entity Counts</h3></div>`)
	html.WriteString(`<table><thead><tr><th>Entity</th><th class="num">Count</th></tr></thead><tbody>`)
	fmt.Fprintf(html, `<tr><td>Organizations</td><td class="num">%d</td></tr>`, ec.Organizations)
	fmt.Fprintf(html, `<tr><td>Ledgers</td><td class="num">%d</td></tr>`, ec.Ledgers)
	fmt.Fprintf(html, `<tr><td>Assets</td><td class="num">%d</td></tr>`, ec.Assets)
	fmt.Fprintf(html, `<tr><td>Accounts</td><td class="num">%d</td></tr>`, ec.Accounts)
	fmt.Fprintf(html, `<tr><td>Portfolios</td><td class="num">%d</td></tr>`, ec.Portfolios)
	fmt.Fprintf(html, `<tr><td>Segments</td><td class="num">%d</td></tr>`, ec.Segments)
	fmt.Fprintf(html, `<tr><td>Transactions</td><td class="num">%d</td></tr>`, ec.Transactions)
	fmt.Fprintf(html, `<tr><td>Operations</td><td class="num">%d</td></tr>`, ec.Operations)
	fmt.Fprintf(html, `<tr><td>Balances</td><td class="num">%d</td></tr>`, ec.Balances)
	fmt.Fprintf(html, `<tr><td>Asset Rates</td><td class="num">%d</td></tr>`, ec.AssetRates)
	html.WriteString(`</tbody></table></div>`)
}

func truncateID(id string) string {
	if len(id) <= truncateIDMinLen {
		return id
	}

	return id[:truncateIDPrefixLen] + "..." + id[len(id)-truncateIDSuffixLen:]
}

//nolint:unparam // a is intentionally always "#ef4444" (red) for negative/error states; b varies for positive states
func ternary(cond bool, a, b string) string {
	if cond {
		return a
	}

	return b
}

func formatDecimal(value decimal.Decimal) string {
	return value.String()
}

func formatSignedDecimal(value decimal.Decimal) string {
	if value.Sign() >= 0 {
		return "+" + value.String()
	}

	return value.String()
}

func (s *HTTPServer) triggerRun(c *fiber.Ctx) error {
	requestID := c.Locals("requestid")
	s.logger.Infof("Manual reconciliation triggered via API (request_id=%v)", requestID)

	// Add timeout for HTTP-triggered runs
	ctx, cancel := context.WithTimeout(c.Context(), triggerRunTimeout)
	defer cancel()

	report, err := s.engine.RunReconciliation(ctx)
	if err != nil {
		// Log full error internally
		s.logger.Errorf("Reconciliation failed (request_id=%v): %v", requestID, err)

		// Return sanitized error to client
		if errors.Is(err, context.DeadlineExceeded) {
			return c.Status(http.StatusGatewayTimeout).JSON(fiber.Map{
				"error":      "Reconciliation timed out",
				"request_id": requestID,
			})
		}

		return c.Status(http.StatusInternalServerError).JSON(fiber.Map{
			"error":      "Reconciliation failed. Check server logs for details.",
			"request_id": requestID,
		})
	}

	return c.JSON(report)
}
