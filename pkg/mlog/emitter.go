package mlog

import (
	"time"

	libLog "github.com/LerianStudio/lib-commons/v2/commons/log"
	"github.com/gofiber/fiber/v2"
)

// contextKey is the type for context keys to avoid collisions.
type contextKey string

// wideEventKey is the key used to store WideEvent in Fiber's Locals.
const wideEventKey contextKey = "wide_event"

// defaultWideEventFieldsCap is the initial slice capacity for WideEvent fields.
// The WideEvent struct currently has ~50 fields, and each field produces two slice
// elements (key + value). This capacity (100) accommodates all current fields with
// minimal slack for future additions, avoiding reallocations in the common case.
const defaultWideEventFieldsCap = 100

// GetWideEvent retrieves the WideEvent from Fiber context.
// Returns nil if no event is found.
func GetWideEvent(c *fiber.Ctx) *WideEvent {
	if event, ok := c.Locals(string(wideEventKey)).(*WideEvent); ok {
		return event
	}

	return nil
}

// SetWideEvent stores a WideEvent in Fiber context.
func SetWideEvent(c *fiber.Ctx, event *WideEvent) {
	c.Locals(string(wideEventKey), event)
}

// EnrichFromLocals extracts common entity IDs from Fiber Locals and enriches the wide event.
// This should be called in handlers after the UUIDs have been parsed from path parameters.
func EnrichFromLocals(c *fiber.Ctx) {
	event := GetWideEvent(c)
	if event == nil {
		return
	}

	// Extract organization_id
	if orgID, ok := c.Locals("organization_id").(interface{ String() string }); ok {
		event.SetOrganization(orgID.String())
	}

	// Extract ledger_id
	if ledgerID, ok := c.Locals("ledger_id").(interface{ String() string }); ok {
		event.SetLedger(ledgerID.String())
	}

	// Extract transaction_id
	if txnID, ok := c.Locals("transaction_id").(interface{ String() string }); ok {
		event.SetTransaction(txnID.String(), "", "", "")
	}

	// Extract account_id
	if accountID, ok := c.Locals("account_id").(interface{ String() string }); ok {
		event.SetAccount(accountID.String())
	}

	// Extract balance_id
	if balanceID, ok := c.Locals("balance_id").(interface{ String() string }); ok {
		event.SetBalance(balanceID.String())
	}

	// Extract operation_id
	if operationID, ok := c.Locals("operation_id").(interface{ String() string }); ok {
		event.SetOperation(operationID.String(), 0)
	}

	// Extract asset_code (string, not UUID)
	if assetCode, ok := c.Locals("asset_code").(string); ok {
		event.SetAsset(assetCode)
	}

	// Extract holder_id
	if holderID, ok := c.Locals("holder_id").(interface{ String() string }); ok {
		event.SetHolder(holderID.String())
	}

	// Extract portfolio_id
	if portfolioID, ok := c.Locals("portfolio_id").(interface{ String() string }); ok {
		event.SetPortfolio(portfolioID.String())
	}

	// Extract segment_id
	if segmentID, ok := c.Locals("segment_id").(interface{ String() string }); ok {
		event.SetSegment(segmentID.String())
	}

	// Extract external_id for asset rates (check String() method first for uuid.UUID, then plain string)
	if stringer, ok := c.Locals("external_id").(interface{ String() string }); ok {
		event.SetAssetRateExternalID(stringer.String())
	} else if externalID, ok := c.Locals("external_id").(string); ok {
		event.SetAssetRateExternalID(externalID)
	}

	// Extract operation_route_id
	if routeID, ok := c.Locals("operation_route_id").(interface{ String() string }); ok {
		event.SetOperationRoute(routeID.String())
	}

	// Extract transaction_route_id
	if routeID, ok := c.Locals("transaction_route_id").(interface{ String() string }); ok {
		event.SetTransactionRoute(routeID.String())
	}
}

// Emit logs the WideEvent using the provided logger.
// It finalizes the event, converts it to fields, and logs at the appropriate level.
// Thread-safe: holds lock through entire emit operation and prevents double-emission.
func (e *WideEvent) Emit(logger libLog.Logger) {
	if e == nil || logger == nil {
		return
	}

	e.mu.Lock()

	// Guard against double-emission
	if e.emitted {
		e.mu.Unlock()
		return
	}

	e.emitted = true

	// Finalize duration if not set
	if e.DurationMS == 0 && !e.StartTime.IsZero() {
		e.DurationMS = float64(time.Since(e.StartTime).Milliseconds())
	}

	// Convert to fields while holding lock
	fields := e.toFieldsLocked()
	outcome := e.Outcome

	e.mu.Unlock()

	// Log at appropriate level based on outcome
	switch outcome {
	case "server_error", "panic":
		logger.WithFields(fields...).Error("wide_event")
	case "client_error":
		logger.WithFields(fields...).Warn("wide_event")
	default:
		logger.WithFields(fields...).Info("wide_event")
	}
}

// toFieldsLocked converts the WideEvent to a slice of key-value pairs for logging.
// MUST be called with e.mu held. Does not acquire lock itself.
// Only non-zero/non-empty fields are included (except status_code which is always included).
//
//nolint:gocyclo,gocognit // Field assembly is intentionally explicit for clarity and debuggability
func (e *WideEvent) toFieldsLocked() []any {
	fields := make([]any, 0, defaultWideEventFieldsCap)

	// Request identification
	if e.RequestID != "" {
		fields = append(fields, "request_id", e.RequestID)
	}

	if e.TraceID != "" {
		fields = append(fields, "trace_id", e.TraceID)
	}

	if e.SpanID != "" {
		fields = append(fields, "span_id", e.SpanID)
	}

	// HTTP request details
	// TODO(review): Validate HTTP method against known methods
	if e.Method != "" {
		fields = append(fields, "method", e.Method)
	}

	if e.Path != "" {
		fields = append(fields, "path", e.Path)
	}

	if e.Route != "" {
		fields = append(fields, "route", e.Route)
	}

	if e.QueryParams != "" {
		fields = append(fields, "query_params", e.QueryParams)
	}

	if e.ContentType != "" {
		fields = append(fields, "content_type", e.ContentType)
	}

	if e.ContentLength > 0 {
		fields = append(fields, "content_length", e.ContentLength)
	}

	if e.UserAgent != "" {
		fields = append(fields, "user_agent", e.UserAgent)
	}

	if e.ClientIP != "" {
		fields = append(fields, "client_ip", e.ClientIP)
	}

	// Timing
	if !e.StartTime.IsZero() {
		fields = append(fields, "start_time", e.StartTime)
	}
	// Always include status_code, even if 0 (indicates incomplete request)
	fields = append(fields, "status_code", e.StatusCode)
	if e.DurationMS > 0 {
		fields = append(fields, "duration_ms", e.DurationMS)
	}

	// Response
	if e.ResponseSize > 0 {
		fields = append(fields, "response_size", e.ResponseSize)
	}

	if e.Outcome != "" {
		fields = append(fields, "outcome", e.Outcome)
	}

	// Service identification
	if e.Service != "" {
		fields = append(fields, "service", e.Service)
	}

	if e.Version != "" {
		fields = append(fields, "version", e.Version)
	}

	if e.Environment != "" {
		fields = append(fields, "environment", e.Environment)
	}

	// Business context - Entity IDs
	if e.OrganizationID != "" {
		fields = append(fields, "organization_id", e.OrganizationID)
	}

	if e.LedgerID != "" {
		fields = append(fields, "ledger_id", e.LedgerID)
	}

	if e.TransactionID != "" {
		fields = append(fields, "transaction_id", e.TransactionID)
	}

	if e.AccountID != "" {
		fields = append(fields, "account_id", e.AccountID)
	}

	if e.BalanceID != "" {
		fields = append(fields, "balance_id", e.BalanceID)
	}

	if e.OperationID != "" {
		fields = append(fields, "operation_id", e.OperationID)
	}

	if e.AssetCode != "" {
		fields = append(fields, "asset_code", e.AssetCode)
	}

	if e.HolderID != "" {
		fields = append(fields, "holder_id", e.HolderID)
	}

	if e.PortfolioID != "" {
		fields = append(fields, "portfolio_id", e.PortfolioID)
	}

	if e.SegmentID != "" {
		fields = append(fields, "segment_id", e.SegmentID)
	}

	// TODO(review): Consider renaming AssetRateExternalID to AssetRateID for consistency
	if e.AssetRateExternalID != "" {
		fields = append(fields, "asset_rate_external_id", e.AssetRateExternalID)
	}

	if e.OperationRouteID != "" {
		fields = append(fields, "operation_route_id", e.OperationRouteID)
	}

	if e.TransactionRouteID != "" {
		fields = append(fields, "transaction_route_id", e.TransactionRouteID)
	}

	// Business context - Transaction details
	if e.TransactionType != "" {
		fields = append(fields, "transaction_type", e.TransactionType)
	}

	// TODO(review): Validate transaction amount format
	if e.TransactionAmount != "" {
		fields = append(fields, "transaction_amount", e.TransactionAmount)
	}

	if e.TransactionCurrency != "" {
		fields = append(fields, "transaction_currency", e.TransactionCurrency)
	}

	if e.OperationCount > 0 {
		fields = append(fields, "operation_count", e.OperationCount)
	}

	if e.SourceCount > 0 {
		fields = append(fields, "source_count", e.SourceCount)
	}

	if e.DestinationCount > 0 {
		fields = append(fields, "destination_count", e.DestinationCount)
	}

	// User context
	if e.UserID != "" {
		fields = append(fields, "user_id", e.UserID)
	}

	if e.UserRole != "" {
		fields = append(fields, "user_role", e.UserRole)
	}

	if e.AuthMethod != "" {
		fields = append(fields, "auth_method", e.AuthMethod)
	}

	// Error context
	if e.ErrorOccurred {
		fields = append(fields, "error_occurred", e.ErrorOccurred)

		if e.ErrorType != "" {
			fields = append(fields, "error_type", e.ErrorType)
		}

		if e.ErrorCode != "" {
			fields = append(fields, "error_code", e.ErrorCode)
		}

		if e.ErrorMessage != "" {
			fields = append(fields, "error_message", e.ErrorMessage)
		}

		fields = append(fields, "error_retryable", e.ErrorRetryable)
	}

	// Performance metrics
	if e.DBQueryCount > 0 {
		fields = append(fields, "db_query_count", e.DBQueryCount)
	}

	if e.DBQueryTimeMS > 0 {
		fields = append(fields, "db_query_time_ms", e.DBQueryTimeMS)
	}

	if e.CacheHits > 0 {
		fields = append(fields, "cache_hits", e.CacheHits)
	}

	if e.CacheMisses > 0 {
		fields = append(fields, "cache_misses", e.CacheMisses)
	}

	if e.ExternalCallCount > 0 {
		fields = append(fields, "external_call_count", e.ExternalCallCount)
	}

	if e.ExternalCallTimeMS > 0 {
		fields = append(fields, "external_call_time_ms", e.ExternalCallTimeMS)
	}

	// Idempotency
	if e.IdempotencyKey != "" {
		fields = append(fields, "idempotency_key", e.IdempotencyKey)
		fields = append(fields, "idempotency_hit", e.IdempotencyHit)
	}

	// Custom fields
	if len(e.Custom) > 0 {
		fields = append(fields, "custom", e.Custom)
	}

	return fields
}
