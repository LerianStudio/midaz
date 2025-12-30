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

// getLocalString extracts a string value from Fiber Locals.
// It handles both types that implement String() method (like uuid.UUID) and plain strings.
func getLocalString(c *fiber.Ctx, key string) (string, bool) {
	if stringer, ok := c.Locals(key).(interface{ String() string }); ok {
		return stringer.String(), true
	}

	if str, ok := c.Locals(key).(string); ok {
		return str, true
	}

	return "", false
}

// enrichEntityIDs extracts and sets common entity IDs on the wide event.
func enrichEntityIDs(c *fiber.Ctx, event *WideEvent) {
	if orgID, ok := getLocalString(c, "organization_id"); ok {
		event.SetOrganization(orgID)
	}

	if ledgerID, ok := getLocalString(c, "ledger_id"); ok {
		event.SetLedger(ledgerID)
	}

	if txnID, ok := getLocalString(c, "transaction_id"); ok {
		event.SetTransactionID(txnID)
	}

	if accountID, ok := getLocalString(c, "account_id"); ok {
		event.SetAccount(accountID)
	}

	if balanceID, ok := getLocalString(c, "balance_id"); ok {
		event.SetBalance(balanceID)
	}

	if operationID, ok := getLocalString(c, "operation_id"); ok {
		event.SetOperation(operationID, 0)
	}
}

// enrichAdditionalIDs extracts and sets additional entity IDs on the wide event.
func enrichAdditionalIDs(c *fiber.Ctx, event *WideEvent) {
	if assetCode, ok := getLocalString(c, "asset_code"); ok {
		event.SetAsset(assetCode)
	}

	if holderID, ok := getLocalString(c, "holder_id"); ok {
		event.SetHolder(holderID)
	}

	if portfolioID, ok := getLocalString(c, "portfolio_id"); ok {
		event.SetPortfolio(portfolioID)
	}

	if segmentID, ok := getLocalString(c, "segment_id"); ok {
		event.SetSegment(segmentID)
	}

	if externalID, ok := getLocalString(c, "external_id"); ok {
		event.SetAssetRateExternalID(externalID)
	}

	if routeID, ok := getLocalString(c, "operation_route_id"); ok {
		event.SetOperationRoute(routeID)
	}

	if routeID, ok := getLocalString(c, "transaction_route_id"); ok {
		event.SetTransactionRoute(routeID)
	}
}

// EnrichFromLocals extracts common entity IDs from Fiber Locals and enriches the wide event.
// This should be called in handlers after the UUIDs have been parsed from path parameters.
func EnrichFromLocals(c *fiber.Ctx) {
	event := GetWideEvent(c)
	if event == nil {
		return
	}

	enrichEntityIDs(c, event)
	enrichAdditionalIDs(c, event)
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
	case OutcomeServerError, OutcomePanic:
		logger.WithFields(fields...).Error("wide_event")
	case OutcomeClientError:
		logger.WithFields(fields...).Warn("wide_event")
	case OutcomeRedirect:
		logger.WithFields(fields...).Info("wide_event")
	default:
		logger.WithFields(fields...).Info("wide_event")
	}
}

// appendRequestIdentification appends request identification fields to the slice.
func (e *WideEvent) appendRequestIdentification(fields []any) []any {
	if e.RequestID != "" {
		fields = append(fields, "request_id", e.RequestID)
	}

	if e.TraceID != "" {
		fields = append(fields, "trace_id", e.TraceID)
	}

	if e.SpanID != "" {
		fields = append(fields, "span_id", e.SpanID)
	}

	return fields
}

// appendHTTPRequestDetails appends HTTP request detail fields to the slice.
func (e *WideEvent) appendHTTPRequestDetails(fields []any) []any {
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

	return fields
}

// appendTimingAndResponse appends timing and response fields to the slice.
func (e *WideEvent) appendTimingAndResponse(fields []any) []any {
	if !e.StartTime.IsZero() {
		fields = append(fields, "start_time", e.StartTime)
	}

	// Always include status_code, even if 0 (indicates incomplete request)
	fields = append(fields, "status_code", e.StatusCode)

	if e.DurationMS > 0 {
		fields = append(fields, "duration_ms", e.DurationMS)
	}

	if e.ResponseSize > 0 {
		fields = append(fields, "response_size", e.ResponseSize)
	}

	if e.Outcome != "" {
		fields = append(fields, "outcome", e.Outcome)
	}

	return fields
}

// appendServiceIdentification appends service identification fields to the slice.
func (e *WideEvent) appendServiceIdentification(fields []any) []any {
	if e.Service != "" {
		fields = append(fields, "service", e.Service)
	}

	if e.Version != "" {
		fields = append(fields, "version", e.Version)
	}

	if e.Environment != "" {
		fields = append(fields, "environment", e.Environment)
	}

	return fields
}

// appendCoreEntityIDs appends core entity ID fields to the slice.
func (e *WideEvent) appendCoreEntityIDs(fields []any) []any {
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

	return fields
}

// appendAdditionalEntityIDs appends additional entity ID fields to the slice.
func (e *WideEvent) appendAdditionalEntityIDs(fields []any) []any {
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

	if e.AssetRateExternalID != "" {
		fields = append(fields, "asset_rate_external_id", e.AssetRateExternalID)
	}

	if e.OperationRouteID != "" {
		fields = append(fields, "operation_route_id", e.OperationRouteID)
	}

	if e.TransactionRouteID != "" {
		fields = append(fields, "transaction_route_id", e.TransactionRouteID)
	}

	return fields
}

// appendTransactionDetails appends transaction detail fields to the slice.
func (e *WideEvent) appendTransactionDetails(fields []any) []any {
	if e.TransactionType != "" {
		fields = append(fields, "transaction_type", e.TransactionType)
	}

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

	return fields
}

// appendUserContext appends user context fields to the slice.
func (e *WideEvent) appendUserContext(fields []any) []any {
	if e.UserID != "" {
		fields = append(fields, "user_id", e.UserID)
	}

	if e.UserRole != "" {
		fields = append(fields, "user_role", e.UserRole)
	}

	if e.AuthMethod != "" {
		fields = append(fields, "auth_method", e.AuthMethod)
	}

	return fields
}

// appendErrorContext appends error context fields to the slice.
func (e *WideEvent) appendErrorContext(fields []any) []any {
	if !e.ErrorOccurred {
		return fields
	}

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

	return fields
}

// appendPerformanceMetrics appends performance metric fields to the slice.
func (e *WideEvent) appendPerformanceMetrics(fields []any) []any {
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

	return fields
}

// appendIdempotencyAndCustom appends idempotency and custom fields to the slice.
func (e *WideEvent) appendIdempotencyAndCustom(fields []any) []any {
	if e.IdempotencyKey != "" {
		fields = append(fields, "idempotency_key", e.IdempotencyKey)
		fields = append(fields, "idempotency_hit", e.IdempotencyHit)
	}

	if len(e.Custom) > 0 {
		fields = append(fields, "custom", e.Custom)
	}

	return fields
}

// toFieldsLocked converts the WideEvent to a slice of key-value pairs for logging.
// MUST be called with e.mu held. Does not acquire lock itself.
// Only non-zero/non-empty fields are included (except status_code which is always included).
func (e *WideEvent) toFieldsLocked() []any {
	fields := make([]any, 0, defaultWideEventFieldsCap)

	fields = e.appendRequestIdentification(fields)
	fields = e.appendHTTPRequestDetails(fields)
	fields = e.appendTimingAndResponse(fields)
	fields = e.appendServiceIdentification(fields)
	fields = e.appendCoreEntityIDs(fields)
	fields = e.appendAdditionalEntityIDs(fields)
	fields = e.appendTransactionDetails(fields)
	fields = e.appendUserContext(fields)
	fields = e.appendErrorContext(fields)
	fields = e.appendPerformanceMetrics(fields)
	fields = e.appendIdempotencyAndCustom(fields)

	return fields
}
