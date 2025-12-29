package mlog

import (
	"sync"
	"time"

	"github.com/gofiber/fiber/v2"
	"go.opentelemetry.io/otel/trace"
)

// WideEvent represents a comprehensive structured log event for a single request.
// All fields are collected throughout the request lifecycle and emitted once at the end.
type WideEvent struct {
	mu sync.RWMutex

	// emitted guards against double-emission
	emitted bool

	// Request identification
	RequestID string `json:"request_id,omitempty"`
	TraceID   string `json:"trace_id,omitempty"`
	SpanID    string `json:"span_id,omitempty"`

	// HTTP request details
	Method        string `json:"method,omitempty"`
	Path          string `json:"path,omitempty"`
	Route         string `json:"route,omitempty"`
	QueryParams   string `json:"query_params,omitempty"`
	ContentType   string `json:"content_type,omitempty"`
	ContentLength int64  `json:"content_length,omitempty"`
	UserAgent     string `json:"user_agent,omitempty"`
	ClientIP      string `json:"client_ip,omitempty"`

	// Timing
	StartTime  time.Time `json:"start_time,omitempty"`
	StatusCode int       `json:"status_code,omitempty"`
	DurationMS float64   `json:"duration_ms,omitempty"`

	// Response
	ResponseSize int64  `json:"response_size,omitempty"`
	Outcome      string `json:"outcome,omitempty"` // "success", "client_error", "server_error", "panic"

	// Service identification
	Service     string `json:"service,omitempty"`
	Version     string `json:"version,omitempty"`
	Environment string `json:"environment,omitempty"`

	// Business context - Entity IDs
	OrganizationID      string `json:"organization_id,omitempty"`
	LedgerID            string `json:"ledger_id,omitempty"`
	TransactionID       string `json:"transaction_id,omitempty"`
	AccountID           string `json:"account_id,omitempty"`
	BalanceID           string `json:"balance_id,omitempty"`
	OperationID         string `json:"operation_id,omitempty"`
	AssetCode           string `json:"asset_code,omitempty"`
	HolderID            string `json:"holder_id,omitempty"`
	PortfolioID         string `json:"portfolio_id,omitempty"`
	SegmentID           string `json:"segment_id,omitempty"`
	AssetRateExternalID string `json:"asset_rate_external_id,omitempty"`
	OperationRouteID    string `json:"operation_route_id,omitempty"`
	TransactionRouteID  string `json:"transaction_route_id,omitempty"`

	// Business context - Transaction details
	TransactionType     string `json:"transaction_type,omitempty"`
	TransactionAmount   string `json:"transaction_amount,omitempty"`
	TransactionCurrency string `json:"transaction_currency,omitempty"`
	OperationCount      int    `json:"operation_count,omitempty"`
	SourceCount         int    `json:"source_count,omitempty"`
	DestinationCount    int    `json:"destination_count,omitempty"`

	// User context
	UserID     string `json:"user_id,omitempty"`
	UserRole   string `json:"user_role,omitempty"`
	AuthMethod string `json:"auth_method,omitempty"`

	// Error context
	ErrorOccurred  bool   `json:"error_occurred,omitempty"`
	ErrorType      string `json:"error_type,omitempty"`
	ErrorCode      string `json:"error_code,omitempty"`
	ErrorMessage   string `json:"error_message,omitempty"`
	ErrorRetryable bool   `json:"error_retryable,omitempty"`

	// Performance metrics
	DBQueryCount       int     `json:"db_query_count,omitempty"`
	DBQueryTimeMS      float64 `json:"db_query_time_ms,omitempty"`
	CacheHits          int     `json:"cache_hits,omitempty"`
	CacheMisses        int     `json:"cache_misses,omitempty"`
	ExternalCallCount  int     `json:"external_call_count,omitempty"`
	ExternalCallTimeMS float64 `json:"external_call_time_ms,omitempty"`

	// Idempotency
	IdempotencyKey string `json:"idempotency_key,omitempty"`
	IdempotencyHit bool   `json:"idempotency_hit,omitempty"`

	// Custom fields for handler-specific data
	// TODO(review): Consider deep sanitization for complex nested types in Custom map
	Custom map[string]any `json:"custom,omitempty"`
}

// NewWideEvent creates a new WideEvent initialized with request context from Fiber.
func NewWideEvent(c *fiber.Ctx) *WideEvent {
	// Safely get route path (Route() can return nil for unmatched routes)
	routePath := ""
	if r := c.Route(); r != nil {
		routePath = r.Path
	}

	event := &WideEvent{
		StartTime:     time.Now(),
		Method:        c.Method(),
		Path:          sanitizePath(c.Path()),
		Route:         routePath,
		QueryParams:   sanitizeQueryParams(string(c.Request().URI().QueryString())),
		ContentType:   c.Get(fiber.HeaderContentType),
		ContentLength: int64(c.Request().Header.ContentLength()),
		UserAgent:     sanitizeHeader(c.Get(fiber.HeaderUserAgent)),
		ClientIP:      anonymizeIP(c.IP()),
		RequestID:     c.Get("X-Request-ID"),
		Custom:        make(map[string]any),
	}

	// Extract trace context if available
	spanCtx := trace.SpanContextFromContext(c.UserContext())
	if spanCtx.IsValid() {
		event.TraceID = spanCtx.TraceID().String()
		event.SpanID = spanCtx.SpanID().String()
	}

	return event
}

// SetService sets service identification fields.
func (e *WideEvent) SetService(name, version, env string) {
	e.mu.Lock()
	defer e.mu.Unlock()

	e.Service = name
	e.Version = version
	e.Environment = env
}

// SetOrganization sets the organization ID.
func (e *WideEvent) SetOrganization(orgID string) {
	e.mu.Lock()
	defer e.mu.Unlock()

	e.OrganizationID = orgID
}

// SetLedger sets the ledger ID.
func (e *WideEvent) SetLedger(ledgerID string) {
	e.mu.Lock()
	defer e.mu.Unlock()

	e.LedgerID = ledgerID
}

// SetTransaction sets transaction-related fields.
func (e *WideEvent) SetTransaction(txnID, txnType, amount, currency string) {
	e.mu.Lock()
	defer e.mu.Unlock()

	e.TransactionID = txnID
	e.TransactionType = txnType
	e.TransactionAmount = amount
	e.TransactionCurrency = currency
}

// SetAccount sets the account ID.
func (e *WideEvent) SetAccount(accountID string) {
	e.mu.Lock()
	defer e.mu.Unlock()

	e.AccountID = accountID
}

// SetBalance sets the balance ID.
func (e *WideEvent) SetBalance(balanceID string) {
	e.mu.Lock()
	defer e.mu.Unlock()

	e.BalanceID = balanceID
}

// SetOperation sets operation-related fields.
func (e *WideEvent) SetOperation(operationID string, count int) {
	e.mu.Lock()
	defer e.mu.Unlock()

	e.OperationID = operationID
	e.OperationCount = count
}

// SetAsset sets the asset code.
func (e *WideEvent) SetAsset(assetCode string) {
	e.mu.Lock()
	defer e.mu.Unlock()

	e.AssetCode = assetCode
}

// SetHolder sets the holder ID.
func (e *WideEvent) SetHolder(holderID string) {
	e.mu.Lock()
	defer e.mu.Unlock()

	e.HolderID = holderID
}

// SetPortfolio sets the portfolio ID.
func (e *WideEvent) SetPortfolio(portfolioID string) {
	e.mu.Lock()
	defer e.mu.Unlock()

	e.PortfolioID = portfolioID
}

// SetSegment sets the segment ID.
func (e *WideEvent) SetSegment(segmentID string) {
	e.mu.Lock()
	defer e.mu.Unlock()

	e.SegmentID = segmentID
}

// SetAssetRateExternalID sets the asset rate external ID.
func (e *WideEvent) SetAssetRateExternalID(externalID string) {
	e.mu.Lock()
	defer e.mu.Unlock()

	e.AssetRateExternalID = externalID
}

// SetOperationRoute sets the operation route ID.
func (e *WideEvent) SetOperationRoute(routeID string) {
	e.mu.Lock()
	defer e.mu.Unlock()

	e.OperationRouteID = routeID
}

// SetTransactionRoute sets the transaction route ID.
func (e *WideEvent) SetTransactionRoute(routeID string) {
	e.mu.Lock()
	defer e.mu.Unlock()

	e.TransactionRouteID = routeID
}

// SetOperationCounts sets source and destination counts.
func (e *WideEvent) SetOperationCounts(sources, destinations int) {
	e.mu.Lock()
	defer e.mu.Unlock()

	e.SourceCount = sources
	e.DestinationCount = destinations
}

// SetUser sets user identification fields.
func (e *WideEvent) SetUser(userID, role, authMethod string) {
	e.mu.Lock()
	defer e.mu.Unlock()

	e.UserID = userID
	e.UserRole = role
	e.AuthMethod = authMethod
}

// SetError sets error-related fields with sanitization.
func (e *WideEvent) SetError(errType, code, message string, retryable bool) {
	e.mu.Lock()
	defer e.mu.Unlock()

	e.ErrorOccurred = true
	e.ErrorType = errType
	e.ErrorCode = code
	e.ErrorMessage = sanitizeErrorMessage(message)
	e.ErrorRetryable = retryable
}

// SetPanic sets panic-related fields. Used when a panic is recovered.
func (e *WideEvent) SetPanic(panicValue string) {
	e.mu.Lock()
	defer e.mu.Unlock()

	e.Outcome = "panic"
	e.ErrorOccurred = true
	e.ErrorType = "panic"
	e.ErrorMessage = sanitizeErrorMessage(panicValue)
}

// SetDBMetrics sets database performance metrics.
func (e *WideEvent) SetDBMetrics(queryCount int, queryTimeMS float64) {
	e.mu.Lock()
	defer e.mu.Unlock()

	e.DBQueryCount = queryCount
	e.DBQueryTimeMS = queryTimeMS
}

// IncrementDBMetrics atomically increments database metrics.
func (e *WideEvent) IncrementDBMetrics(queryCount int, queryTimeMS float64) {
	e.mu.Lock()
	defer e.mu.Unlock()

	e.DBQueryCount += queryCount
	e.DBQueryTimeMS += queryTimeMS
}

// SetCacheMetrics sets cache performance metrics.
func (e *WideEvent) SetCacheMetrics(hits, misses int) {
	e.mu.Lock()
	defer e.mu.Unlock()

	e.CacheHits = hits
	e.CacheMisses = misses
}

// IncrementCacheHit atomically increments cache hits.
func (e *WideEvent) IncrementCacheHit() {
	e.mu.Lock()
	defer e.mu.Unlock()

	e.CacheHits++
}

// IncrementCacheMiss atomically increments cache misses.
func (e *WideEvent) IncrementCacheMiss() {
	e.mu.Lock()
	defer e.mu.Unlock()

	e.CacheMisses++
}

// SetExternalCallMetrics sets external call performance metrics.
func (e *WideEvent) SetExternalCallMetrics(callCount int, callTimeMS float64) {
	e.mu.Lock()
	defer e.mu.Unlock()

	e.ExternalCallCount = callCount
	e.ExternalCallTimeMS = callTimeMS
}

// IncrementExternalCallMetrics atomically increments external call metrics.
func (e *WideEvent) IncrementExternalCallMetrics(callCount int, callTimeMS float64) {
	e.mu.Lock()
	defer e.mu.Unlock()

	e.ExternalCallCount += callCount
	e.ExternalCallTimeMS += callTimeMS
}

// SetIdempotency sets idempotency-related fields with key hashing.
func (e *WideEvent) SetIdempotency(key string, hit bool) {
	e.mu.Lock()
	defer e.mu.Unlock()

	e.IdempotencyKey = hashIdempotencyKey(key)
	e.IdempotencyHit = hit
}

// SetCustom sets a custom field with bounds checking.
// TODO(review): Consider returning bool from SetCustom to indicate if value was dropped
func (e *WideEvent) SetCustom(key string, value any) {
	e.mu.Lock()
	defer e.mu.Unlock()

	if e.Custom == nil {
		e.Custom = make(map[string]any)
	}

	// Bounds checking - set overflow indicator when limit reached
	if len(e.Custom) >= maxCustomKeys {
		e.Custom["_overflow"] = true
		return
	}

	if len(key) > maxCustomKeyLen {
		key = key[:maxCustomKeyLen]
	}

	// Truncate string values
	if strVal, ok := value.(string); ok && len(strVal) > maxCustomValueLen {
		value = strVal[:maxCustomValueLen]
	}

	e.Custom[key] = value
}

// SetResponse sets response-related fields.
// TODO(review): Handle 1xx informational status codes with dedicated outcome
func (e *WideEvent) SetResponse(statusCode int, responseSize int64) {
	e.mu.Lock()
	defer e.mu.Unlock()

	// Don't overwrite panic outcome if already set
	if e.Outcome == "panic" {
		e.StatusCode = statusCode
		e.ResponseSize = responseSize
		e.DurationMS = float64(time.Since(e.StartTime).Milliseconds())

		return
	}

	e.StatusCode = statusCode
	e.ResponseSize = responseSize
	e.DurationMS = float64(time.Since(e.StartTime).Milliseconds())

	// Determine outcome based on status code
	switch {
	case statusCode >= 200 && statusCode < 400:
		e.Outcome = "success"
	case statusCode >= 400 && statusCode < 500:
		e.Outcome = "client_error"
	case statusCode >= 500:
		e.Outcome = "server_error"
	default:
		e.Outcome = "unknown"
	}
}

// Finalize calculates final metrics. Called automatically before emission.
func (e *WideEvent) Finalize() {
	e.mu.Lock()
	defer e.mu.Unlock()

	if e.DurationMS == 0 {
		e.DurationMS = float64(time.Since(e.StartTime).Milliseconds())
	}
}
