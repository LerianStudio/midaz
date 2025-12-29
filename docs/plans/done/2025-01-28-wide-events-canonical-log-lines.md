# Wide Events / Canonical Log Lines Implementation Plan

## Goal

Implement the Wide Events pattern in Midaz to emit ONE comprehensive structured log event per HTTP request, containing all context needed for debugging. This is additive - existing logging continues to work.

**Pattern Reference:** https://loggingsucks.com/

## Architecture Overview

```
┌─────────────────────────────────────────────────────────────────┐
│                     HTTP Request Flow                            │
├─────────────────────────────────────────────────────────────────┤
│                                                                  │
│  Request In                                                      │
│      │                                                           │
│      ▼                                                           │
│  ┌─────────────────┐                                            │
│  │ Panic Recovery  │  (existing)                                │
│  └────────┬────────┘                                            │
│           │                                                      │
│           ▼                                                      │
│  ┌─────────────────┐                                            │
│  │   Telemetry     │  (existing - creates spans)                │
│  └────────┬────────┘                                            │
│           │                                                      │
│           ▼                                                      │
│  ┌─────────────────┐                                            │
│  │  Wide Event MW  │  ◄── NEW: Initialize event, emit at end   │
│  └────────┬────────┘                                            │
│           │                                                      │
│           ▼                                                      │
│  ┌─────────────────┐                                            │
│  │     CORS        │  (existing)                                │
│  └────────┬────────┘                                            │
│           │                                                      │
│           ▼                                                      │
│  ┌─────────────────┐                                            │
│  │  HTTP Logging   │  (existing - lib-commons)                  │
│  └────────┬────────┘                                            │
│           │                                                      │
│           ▼                                                      │
│  ┌─────────────────┐                                            │
│  │  Route Handler  │  Enriches event via c.Locals("wide_event") │
│  └────────┬────────┘                                            │
│           │                                                      │
│           ▼                                                      │
│  Response Out (Wide Event emitted in deferred middleware)        │
│                                                                  │
└─────────────────────────────────────────────────────────────────┘
```

## Tech Stack

- **Language:** Go 1.24+
- **HTTP Framework:** Fiber v2
- **Logging:** Uber Zap via lib-commons wrapper
- **Tracing:** OpenTelemetry
- **Pattern:** Wide Events / Canonical Log Lines

## Prerequisites

- [ ] Midaz codebase cloned and buildable
- [ ] Go 1.24+ installed
- [ ] Understanding of Fiber middleware chain

## Constraints (lib-commons limitations)

- Cannot modify lib-commons library
- Logger interface: `WithFields(key, value...)` available
- Cannot add custom span processors
- Cannot change what HTTP logging middleware captures

---

## Task Breakdown

### Batch 1: Core Wide Event Infrastructure (pkg/mlog)

#### Task 1.1: Create Wide Event Package Structure

**File:** `pkg/mlog/doc.go`

**Action:** Create new file

```go
// Package mlog provides wide event / canonical log line utilities for Midaz.
//
// Wide Events Pattern:
// Instead of scattered log statements throughout a request lifecycle,
// emit ONE comprehensive structured event per request containing all
// context needed for debugging.
//
// Benefits:
//   - Single queryable event per request
//   - All business context in one place
//   - Supports queries like "show failed transactions for premium users"
//   - Correlates with OpenTelemetry traces via trace_id
//
// Usage:
//
//	// In middleware - initialize and defer emission
//	event := mlog.NewWideEvent(c)
//	defer event.Emit(logger)
//
//	// In handlers - enrich with business context
//	event := mlog.GetWideEvent(c)
//	event.SetTransaction(txnID, amount, assetCode)
//	event.SetUser(userID, orgID, role)
//
// Reference: https://loggingsucks.com/
package mlog
```

**Verification:**
```bash
cat /Users/fredamaral/repos/lerianstudio/midaz/pkg/mlog/doc.go
# Expected: Package documentation displayed
```

---

#### Task 1.2: Create Wide Event Types

**File:** `pkg/mlog/wide_event.go`

**Action:** Create new file

```go
package mlog

import (
	"sync"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"go.opentelemetry.io/otel/trace"
)

// WideEventKey is the key used to store the wide event in Fiber locals.
const WideEventKey = "wide_event"

// WideEvent represents a canonical log line with all request context.
// It accumulates context throughout the request lifecycle and is
// emitted once at request completion.
type WideEvent struct {
	mu sync.RWMutex

	// Request context (set by middleware)
	RequestID     string    `json:"request_id,omitempty"`
	TraceID       string    `json:"trace_id,omitempty"`
	SpanID        string    `json:"span_id,omitempty"`
	Method        string    `json:"method"`
	Path          string    `json:"path"`
	Route         string    `json:"route,omitempty"` // Pattern like /v1/organizations/:org_id/...
	QueryParams   string    `json:"query_params,omitempty"`
	ContentType   string    `json:"content_type,omitempty"`
	ContentLength int64     `json:"content_length,omitempty"`
	UserAgent     string    `json:"user_agent,omitempty"`
	ClientIP      string    `json:"client_ip,omitempty"`
	StartTime     time.Time `json:"start_time"`

	// Response context (set at completion)
	StatusCode   int           `json:"status_code"`
	DurationMS   int64         `json:"duration_ms"`
	ResponseSize int           `json:"response_size,omitempty"`
	Outcome      string        `json:"outcome"` // "success", "error", "panic"

	// Service context (set by middleware)
	Service     string `json:"service"`
	Version     string `json:"version,omitempty"`
	Environment string `json:"environment,omitempty"`

	// Business context - IDs (set by handlers)
	OrganizationID string `json:"organization_id,omitempty"`
	LedgerID       string `json:"ledger_id,omitempty"`
	TransactionID  string `json:"transaction_id,omitempty"`
	AccountID      string `json:"account_id,omitempty"`
	BalanceID      string `json:"balance_id,omitempty"`
	OperationID    string `json:"operation_id,omitempty"`
	AssetCode      string `json:"asset_code,omitempty"`

	// Business context - Extended IDs (set by handlers)
	HolderID             string `json:"holder_id,omitempty"`
	PortfolioID          string `json:"portfolio_id,omitempty"`
	SegmentID            string `json:"segment_id,omitempty"`
	AssetRateExternalID  string `json:"asset_rate_external_id,omitempty"`
	OperationRouteID     string `json:"operation_route_id,omitempty"`
	TransactionRouteID   string `json:"transaction_route_id,omitempty"`

	// Business context - Transaction details (set by handlers)
	// TransactionType values: "json", "dsl", "annotation", "inflow", "outflow"
	TransactionType     string `json:"transaction_type,omitempty"`
	TransactionAmount   string `json:"transaction_amount,omitempty"`    // String to preserve precision
	TransactionCurrency string `json:"transaction_currency,omitempty"`
	OperationCount      int    `json:"operation_count,omitempty"`
	SourceCount         int    `json:"source_count,omitempty"`
	DestinationCount    int    `json:"destination_count,omitempty"`

	// User/Auth context (set by auth middleware or handlers)
	UserID       string `json:"user_id,omitempty"`
	UserRole     string `json:"user_role,omitempty"`
	AuthMethod   string `json:"auth_method,omitempty"` // "api_key", "jwt", "oauth"

	// Error context (set on error)
	ErrorOccurred bool   `json:"error_occurred"`
	ErrorType     string `json:"error_type,omitempty"`
	ErrorCode     string `json:"error_code,omitempty"`
	ErrorMessage  string `json:"error_message,omitempty"`
	ErrorRetryable bool  `json:"error_retryable,omitempty"`

	// Performance context (set by handlers)
	DBQueryCount      int   `json:"db_query_count,omitempty"`
	DBQueryTimeMS     int64 `json:"db_query_time_ms,omitempty"`
	CacheHits         int   `json:"cache_hits,omitempty"`
	CacheMisses       int   `json:"cache_misses,omitempty"`
	ExternalCallCount int   `json:"external_call_count,omitempty"`
	ExternalCallTimeMS int64 `json:"external_call_time_ms,omitempty"`

	// Idempotency context
	IdempotencyKey string `json:"idempotency_key,omitempty"`
	IdempotencyHit bool   `json:"idempotency_hit,omitempty"`

	// Custom fields for extensibility
	Custom map[string]any `json:"custom,omitempty"`
}

// NewWideEvent creates a new wide event initialized with request context.
// All user-controlled inputs are sanitized to prevent log injection and sensitive data exposure.
func NewWideEvent(c *fiber.Ctx, cfg Config) *WideEvent {
	// Safely extract route path (may be nil before route matching)
	routePath := ""
	if r := c.Route(); r != nil {
		routePath = r.Path
	}

	// Get client IP with optional GDPR anonymization
	clientIP := c.IP()
	if cfg.AnonymizeIP {
		clientIP = anonymizeIP(clientIP)
	}

	event := &WideEvent{
		RequestID:     sanitizeHeader(c.Get("X-Request-Id")),
		Method:        c.Method(),
		Path:          c.Path(),
		Route:         routePath,
		QueryParams:   sanitizeQueryParams(string(c.Request().URI().QueryString())),
		ContentType:   sanitizeHeader(c.Get("Content-Type")),
		ContentLength: int64(len(c.Body())),
		UserAgent:     sanitizeHeader(c.Get("User-Agent")),
		ClientIP:      clientIP,
		StartTime:     time.Now(),
		Service:       cfg.ServiceName,
		Version:       cfg.Version,
		Environment:   cfg.Environment,
		Custom:        make(map[string]any),
	}

	// Extract trace context if available
	ctx := c.UserContext()
	span := trace.SpanFromContext(ctx)
	if span.SpanContext().IsValid() {
		event.TraceID = span.SpanContext().TraceID().String()
		event.SpanID = span.SpanContext().SpanID().String()
	}

	return event
}

// SetOrganization sets organization context.
func (e *WideEvent) SetOrganization(orgID uuid.UUID) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if orgID != uuid.Nil {
		e.OrganizationID = orgID.String()
	}
}

// SetLedger sets ledger context.
func (e *WideEvent) SetLedger(ledgerID uuid.UUID) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if ledgerID != uuid.Nil {
		e.LedgerID = ledgerID.String()
	}
}

// SetTransaction sets transaction context.
func (e *WideEvent) SetTransaction(txnID uuid.UUID, txnType string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if txnID != uuid.Nil {
		e.TransactionID = txnID.String()
	}
	e.TransactionType = txnType
}

// SetTransactionDetails sets detailed transaction information.
func (e *WideEvent) SetTransactionDetails(amount, currency string, opCount, srcCount, destCount int) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.TransactionAmount = amount
	e.TransactionCurrency = currency
	e.OperationCount = opCount
	e.SourceCount = srcCount
	e.DestinationCount = destCount
}

// SetAccount sets account context.
func (e *WideEvent) SetAccount(accountID uuid.UUID) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if accountID != uuid.Nil {
		e.AccountID = accountID.String()
	}
}

// SetBalance sets balance context.
func (e *WideEvent) SetBalance(balanceID uuid.UUID) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if balanceID != uuid.Nil {
		e.BalanceID = balanceID.String()
	}
}

// SetOperation sets operation context.
func (e *WideEvent) SetOperation(operationID uuid.UUID) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if operationID != uuid.Nil {
		e.OperationID = operationID.String()
	}
}

// SetAsset sets asset context.
func (e *WideEvent) SetAsset(assetCode string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.AssetCode = assetCode
}

// SetHolder sets holder context.
func (e *WideEvent) SetHolder(holderID uuid.UUID) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if holderID != uuid.Nil {
		e.HolderID = holderID.String()
	}
}

// SetPortfolio sets portfolio context.
func (e *WideEvent) SetPortfolio(portfolioID uuid.UUID) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if portfolioID != uuid.Nil {
		e.PortfolioID = portfolioID.String()
	}
}

// SetSegment sets segment context.
func (e *WideEvent) SetSegment(segmentID uuid.UUID) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if segmentID != uuid.Nil {
		e.SegmentID = segmentID.String()
	}
}

// SetOperationRoute sets operation route context.
func (e *WideEvent) SetOperationRoute(routeID uuid.UUID) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if routeID != uuid.Nil {
		e.OperationRouteID = routeID.String()
	}
}

// SetTransactionRoute sets transaction route context.
func (e *WideEvent) SetTransactionRoute(routeID uuid.UUID) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if routeID != uuid.Nil {
		e.TransactionRouteID = routeID.String()
	}
}

// SetAssetRateExternalID sets asset rate external ID context.
func (e *WideEvent) SetAssetRateExternalID(externalID string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.AssetRateExternalID = externalID
}

// SetUser sets user/auth context.
func (e *WideEvent) SetUser(userID, role, authMethod string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.UserID = userID
	e.UserRole = role
	e.AuthMethod = authMethod
}

// SetError sets error context with sanitized message.
// Error messages are sanitized to remove connection strings, file paths, and other sensitive info.
func (e *WideEvent) SetError(errType, errCode, errMessage string, retryable bool) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.ErrorOccurred = true
	e.ErrorType = errType
	e.ErrorCode = errCode
	e.ErrorMessage = sanitizeErrorMessage(errMessage)
	e.ErrorRetryable = retryable
}

// SetDBStats sets database performance stats.
func (e *WideEvent) SetDBStats(queryCount int, queryTimeMS int64) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.DBQueryCount = queryCount
	e.DBQueryTimeMS = queryTimeMS
}

// SetCacheStats sets cache performance stats.
func (e *WideEvent) SetCacheStats(hits, misses int) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.CacheHits = hits
	e.CacheMisses = misses
}

// SetIdempotency sets idempotency context with hashed key.
// The key is hashed to preserve uniqueness while preventing pattern analysis attacks.
func (e *WideEvent) SetIdempotency(key string, hit bool) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.IdempotencyKey = hashIdempotencyKey(key)
	e.IdempotencyHit = hit
}

// SetCustom sets a custom field with bounds checking to prevent DoS.
// Limits: max 50 keys, 64-char key length, 1KB string values.
func (e *WideEvent) SetCustom(key string, value any) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.Custom == nil {
		e.Custom = make(map[string]any)
	}

	// Limit number of custom keys to prevent memory exhaustion
	if len(e.Custom) >= maxCustomKeys {
		return // Silently drop - log warning in production if needed
	}

	// Truncate key length
	if len(key) > maxCustomKeyLen {
		key = key[:maxCustomKeyLen]
	}

	// Truncate string values
	if strVal, ok := value.(string); ok && len(strVal) > maxCustomValueLen {
		value = strVal[:maxCustomValueLen] + "...[TRUNCATED]"
	}

	e.Custom[key] = value
}

// Complete finalizes the event with response data.
func (e *WideEvent) Complete(statusCode int, responseSize int) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.StatusCode = statusCode
	e.ResponseSize = responseSize
	e.DurationMS = time.Since(e.StartTime).Milliseconds()

	// Determine outcome
	switch {
	case statusCode >= 500:
		e.Outcome = "error"
	case statusCode >= 400:
		e.Outcome = "client_error"
	default:
		e.Outcome = "success"
	}
}

// SetPanic marks the event as a panic recovery.
// Panic values are sanitized to remove sensitive information like file paths and connection strings.
func (e *WideEvent) SetPanic(panicValue string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.Outcome = "panic"
	e.ErrorOccurred = true
	e.ErrorType = "panic"
	e.ErrorMessage = sanitizeErrorMessage(panicValue) // Sanitize to remove internal paths/secrets
}
```

**Verification:**
```bash
cd /Users/fredamaral/repos/lerianstudio/midaz && go build ./pkg/mlog/...
# Expected: No errors
```

---

#### Task 1.3: Create Wide Event Emission Logic

**File:** `pkg/mlog/emitter.go`

**Action:** Create new file

```go
package mlog

import (
	"github.com/gofiber/fiber/v2"
	libLog "github.com/LerianStudio/lib-commons/v2/commons/log"
)

// GetWideEvent retrieves the wide event from Fiber context.
// Returns nil if no wide event is set (middleware not active).
func GetWideEvent(c *fiber.Ctx) *WideEvent {
	if event, ok := c.Locals(WideEventKey).(*WideEvent); ok {
		return event
	}
	return nil
}

// SetWideEvent stores the wide event in Fiber context.
func SetWideEvent(c *fiber.Ctx, event *WideEvent) {
	c.Locals(WideEventKey, event)
}

// Emit logs the wide event as a single structured log line.
// This should be called once at the end of request processing.
func (e *WideEvent) Emit(logger libLog.Logger) {
	e.mu.RLock()
	defer e.mu.RUnlock()

	// Build field slice for WithFields
	fields := e.toFields()

	// Emit single canonical log line
	logger.WithFields(fields...).Info("wide_event")
}

// toFields converts the wide event to a slice of key-value pairs
// compatible with lib-commons logger.WithFields().
func (e *WideEvent) toFields() []any {
	fields := make([]any, 0, 80) // Pre-allocate for performance

	// Request context
	appendIfNotEmpty(&fields, "request_id", e.RequestID)
	appendIfNotEmpty(&fields, "trace_id", e.TraceID)
	appendIfNotEmpty(&fields, "span_id", e.SpanID)
	fields = append(fields, "method", e.Method)
	fields = append(fields, "path", e.Path)
	appendIfNotEmpty(&fields, "route", e.Route)
	appendIfNotEmpty(&fields, "query_params", e.QueryParams)
	appendIfNotEmpty(&fields, "content_type", e.ContentType)
	if e.ContentLength > 0 {
		fields = append(fields, "content_length", e.ContentLength)
	}
	appendIfNotEmpty(&fields, "user_agent", e.UserAgent)
	appendIfNotEmpty(&fields, "client_ip", e.ClientIP)

	// Response context
	fields = append(fields, "status_code", e.StatusCode)
	fields = append(fields, "duration_ms", e.DurationMS)
	if e.ResponseSize > 0 {
		fields = append(fields, "response_size", e.ResponseSize)
	}
	fields = append(fields, "outcome", e.Outcome)

	// Service context
	fields = append(fields, "service", e.Service)
	appendIfNotEmpty(&fields, "version", e.Version)
	appendIfNotEmpty(&fields, "environment", e.Environment)

	// Business context - IDs
	appendIfNotEmpty(&fields, "organization_id", e.OrganizationID)
	appendIfNotEmpty(&fields, "ledger_id", e.LedgerID)
	appendIfNotEmpty(&fields, "transaction_id", e.TransactionID)
	appendIfNotEmpty(&fields, "account_id", e.AccountID)
	appendIfNotEmpty(&fields, "balance_id", e.BalanceID)
	appendIfNotEmpty(&fields, "operation_id", e.OperationID)
	appendIfNotEmpty(&fields, "asset_code", e.AssetCode)

	// Business context - Extended IDs
	appendIfNotEmpty(&fields, "holder_id", e.HolderID)
	appendIfNotEmpty(&fields, "portfolio_id", e.PortfolioID)
	appendIfNotEmpty(&fields, "segment_id", e.SegmentID)
	appendIfNotEmpty(&fields, "asset_rate_external_id", e.AssetRateExternalID)
	appendIfNotEmpty(&fields, "operation_route_id", e.OperationRouteID)
	appendIfNotEmpty(&fields, "transaction_route_id", e.TransactionRouteID)

	// Business context - Transaction details
	appendIfNotEmpty(&fields, "transaction_type", e.TransactionType)
	appendIfNotEmpty(&fields, "transaction_amount", e.TransactionAmount)
	appendIfNotEmpty(&fields, "transaction_currency", e.TransactionCurrency)
	if e.OperationCount > 0 {
		fields = append(fields, "operation_count", e.OperationCount)
	}
	if e.SourceCount > 0 {
		fields = append(fields, "source_count", e.SourceCount)
	}
	if e.DestinationCount > 0 {
		fields = append(fields, "destination_count", e.DestinationCount)
	}

	// User/Auth context
	appendIfNotEmpty(&fields, "user_id", e.UserID)
	appendIfNotEmpty(&fields, "user_role", e.UserRole)
	appendIfNotEmpty(&fields, "auth_method", e.AuthMethod)

	// Error context
	if e.ErrorOccurred {
		fields = append(fields, "error_occurred", true)
		appendIfNotEmpty(&fields, "error_type", e.ErrorType)
		appendIfNotEmpty(&fields, "error_code", e.ErrorCode)
		appendIfNotEmpty(&fields, "error_message", e.ErrorMessage)
		if e.ErrorRetryable {
			fields = append(fields, "error_retryable", true)
		}
	}

	// Performance context
	if e.DBQueryCount > 0 {
		fields = append(fields, "db_query_count", e.DBQueryCount)
		fields = append(fields, "db_query_time_ms", e.DBQueryTimeMS)
	}
	if e.CacheHits > 0 || e.CacheMisses > 0 {
		fields = append(fields, "cache_hits", e.CacheHits)
		fields = append(fields, "cache_misses", e.CacheMisses)
	}
	if e.ExternalCallCount > 0 {
		fields = append(fields, "external_call_count", e.ExternalCallCount)
		fields = append(fields, "external_call_time_ms", e.ExternalCallTimeMS)
	}

	// Idempotency context
	appendIfNotEmpty(&fields, "idempotency_key", e.IdempotencyKey)
	if e.IdempotencyHit {
		fields = append(fields, "idempotency_hit", true)
	}

	// Custom fields
	for k, v := range e.Custom {
		fields = append(fields, "custom."+k, v)
	}

	return fields
}

// appendIfNotEmpty adds a field only if the value is not empty.
func appendIfNotEmpty(fields *[]any, key, value string) {
	if value != "" {
		*fields = append(*fields, key, value)
	}
}
```

**Verification:**
```bash
cd /Users/fredamaral/repos/lerianstudio/midaz && go build ./pkg/mlog/...
# Expected: No errors
```

---

#### Task 1.4: Create Wide Event Middleware

**File:** `pkg/mlog/middleware.go`

**Action:** Create new file

```go
package mlog

import (
	"fmt"
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	libLog "github.com/LerianStudio/lib-commons/v2/commons/log"
)

// Config holds configuration for the wide event middleware.
type Config struct {
	// ServiceName is the name of the service (e.g., "transaction", "ledger").
	ServiceName string

	// Version is the service version.
	Version string

	// Environment is the deployment environment (e.g., "production", "staging").
	Environment string

	// Logger is the logger instance to use for emitting events.
	Logger libLog.Logger

	// Skip defines a function to skip middleware for certain requests.
	// Return true to skip wide event logging for a request.
	Skip func(c *fiber.Ctx) bool

	// AnonymizeIP enables GDPR-compliant IP anonymization.
	// When true, the last octet of IPv4 addresses is zeroed.
	AnonymizeIP bool
}

// DefaultSkipPaths returns paths that should skip wide event logging.
func DefaultSkipPaths() func(c *fiber.Ctx) bool {
	skipPaths := map[string]bool{
		"/health":  true,
		"/version": true,
		"/metrics": true,
	}

	return func(c *fiber.Ctx) bool {
		path := c.Path()
		if skipPaths[path] {
			return true
		}
		// Skip swagger paths (using strings.HasPrefix for clarity)
		if strings.HasPrefix(path, "/swagger") {
			return true
		}
		return false
	}
}

// NewWideEventMiddleware creates Fiber middleware that implements wide events.
// It initializes a WideEvent at request start and emits it at request end.
// Also checks for panic recovery context from upstream middleware.
func NewWideEventMiddleware(cfg Config) fiber.Handler {
	// Set defaults
	if cfg.Skip == nil {
		cfg.Skip = DefaultSkipPaths()
	}

	return func(c *fiber.Ctx) error {
		// Check if we should skip this request
		if cfg.Skip(c) {
			return c.Next()
		}

		// Initialize wide event with full config (includes sanitization settings)
		event := NewWideEvent(c, cfg)

		// Store in context for handlers to enrich
		SetWideEvent(c, event)

		// Process request
		err := c.Next()

		// Check for panic context set by panic recovery middleware
		if panicVal := c.Locals("panic_value"); panicVal != nil {
			event.SetPanic(fmt.Sprintf("%v", panicVal))
		}

		// Complete the event with response data
		event.Complete(c.Response().StatusCode(), len(c.Response().Body()))

		// If there was an error, capture it
		if err != nil {
			// Try to extract error details
			if fiberErr, ok := err.(*fiber.Error); ok {
				event.SetError("fiber_error", "", fiberErr.Message, false)
			} else {
				event.SetError("handler_error", "", err.Error(), false)
			}
		}

		// Emit the wide event
		event.Emit(cfg.Logger)

		return err
	}
}

// EnrichFromLocals extracts common IDs from Fiber locals and adds them to the event.
// Call this in handlers after ParseUUIDPathParameters middleware has run.
// Uses setter methods for consistent thread safety and encapsulation.
func EnrichFromLocals(c *fiber.Ctx) {
	event := GetWideEvent(c)
	if event == nil {
		return
	}

	// Extract UUIDs using setter methods (thread-safe, encapsulated)
	if orgID, ok := c.Locals("organization_id").(uuid.UUID); ok && orgID != uuid.Nil {
		event.SetOrganization(orgID)
	}

	if ledgerID, ok := c.Locals("ledger_id").(uuid.UUID); ok && ledgerID != uuid.Nil {
		event.SetLedger(ledgerID)
	}

	if txnID, ok := c.Locals("transaction_id").(uuid.UUID); ok && txnID != uuid.Nil {
		event.SetTransaction(txnID, "")
	}

	if accountID, ok := c.Locals("account_id").(uuid.UUID); ok && accountID != uuid.Nil {
		event.SetAccount(accountID)
	}

	if balanceID, ok := c.Locals("balance_id").(uuid.UUID); ok && balanceID != uuid.Nil {
		event.SetBalance(balanceID)
	}

	if operationID, ok := c.Locals("operation_id").(uuid.UUID); ok && operationID != uuid.Nil {
		event.SetOperation(operationID)
	}

	// Extended ID extractions for routes and asset rates
	if operationRouteID, ok := c.Locals("operation_route_id").(uuid.UUID); ok && operationRouteID != uuid.Nil {
		event.SetOperationRoute(operationRouteID)
	}

	if transactionRouteID, ok := c.Locals("transaction_route_id").(uuid.UUID); ok && transactionRouteID != uuid.Nil {
		event.SetTransactionRoute(transactionRouteID)
	}

	if externalID, ok := c.Locals("external_id").(string); ok && externalID != "" {
		event.SetAssetRateExternalID(externalID)
	}
}
```

**Verification:**
```bash
cd /Users/fredamaral/repos/lerianstudio/midaz && go build ./pkg/mlog/...
# Expected: No errors
```

---

#### Task 1.5: Create Security Sanitization Functions

**File:** `pkg/mlog/sanitize.go`

**Action:** Create new file

```go
package mlog

import (
	"crypto/sha256"
	"encoding/hex"
	"html"
	"net"
	"regexp"
	"strings"
)

// Security constants for sanitization
const (
	maxHeaderLength     = 256
	maxCustomKeys       = 50
	maxCustomKeyLen     = 64
	maxCustomValueLen   = 1024
	maxErrorMessageLen  = 500
)

// sensitiveQueryParams matches common sensitive parameter names
var sensitiveQueryParams = regexp.MustCompile(
	`(?i)(token|secret|key|password|auth|api_key|access_token|session|credential|bearer)[^&]*`,
)

// connectionStringPattern matches database connection strings
var connectionStringPattern = regexp.MustCompile(
	`(?i)(postgres|mysql|mongodb|redis|amqp)://[^\s]+`,
)

// filePathPattern matches file paths that could reveal internal structure
var filePathPattern = regexp.MustCompile(
	`(/[a-zA-Z0-9_/-]+)+\.(go|json|yaml|yml|env|conf|cfg)`,
)

// sanitizeQueryParams redacts sensitive parameters from query strings.
// This prevents API tokens, session IDs, and credentials from being logged.
func sanitizeQueryParams(query string) string {
	if query == "" {
		return ""
	}
	return sensitiveQueryParams.ReplaceAllString(query, "$1=REDACTED")
}

// sanitizeErrorMessage removes potentially sensitive information from error messages.
// This prevents database credentials, file paths, and internal details from leaking.
func sanitizeErrorMessage(msg string) string {
	if msg == "" {
		return ""
	}
	// Remove potential connection strings
	msg = connectionStringPattern.ReplaceAllString(msg, "[CONNECTION_REDACTED]")
	// Remove file paths
	msg = filePathPattern.ReplaceAllString(msg, "[PATH_REDACTED]")
	// Truncate to prevent log bloat
	if len(msg) > maxErrorMessageLen {
		return msg[:maxErrorMessageLen] + "...[TRUNCATED]"
	}
	return msg
}

// sanitizeHeader prevents log injection and truncates excessive length.
// This protects against CRLF injection and XSS in log viewers.
func sanitizeHeader(value string) string {
	if value == "" {
		return ""
	}
	// Remove newlines (log injection prevention)
	value = strings.ReplaceAll(value, "\n", " ")
	value = strings.ReplaceAll(value, "\r", " ")
	// Truncate excessive length
	if len(value) > maxHeaderLength {
		value = value[:maxHeaderLength] + "...[TRUNCATED]"
	}
	// Escape HTML entities for log viewer safety
	value = html.EscapeString(value)
	return value
}

// anonymizeIP zeroes the last octet of IPv4 or last 80 bits of IPv6.
// This provides GDPR-compliant IP anonymization while preserving network identification.
func anonymizeIP(ip string) string {
	parsedIP := net.ParseIP(ip)
	if parsedIP == nil {
		return "invalid"
	}
	if ipv4 := parsedIP.To4(); ipv4 != nil {
		ipv4[3] = 0
		return ipv4.String()
	}
	// IPv6: zero last 80 bits (keep /48 prefix)
	for i := 6; i < 16; i++ {
		parsedIP[i] = 0
	}
	return parsedIP.String()
}

// hashIdempotencyKey hashes the key using SHA256 to preserve uniqueness without exposing patterns.
// This prevents attackers from predicting or analyzing idempotency key patterns.
// Uses cryptographic hash for security rather than simple polynomial hash.
func hashIdempotencyKey(key string) string {
	if key == "" {
		return ""
	}
	h := sha256.Sum256([]byte(key))
	// Use first 16 bytes (128 bits) for reasonable uniqueness + brevity
	return "idem_" + hex.EncodeToString(h[:16])
}
```

**Verification:**
```bash
cd /Users/fredamaral/repos/lerianstudio/midaz && go build ./pkg/mlog/...
# Expected: No errors
```

---

### ✅ Code Review Checkpoint 1

**After completing Batch 1, run:**
```bash
cd /Users/fredamaral/repos/lerianstudio/midaz

# Build the new package
go build ./pkg/mlog/...

# Run any tests
go test ./pkg/mlog/... -v

# Check for linting issues
golangci-lint run ./pkg/mlog/...
```

**Expected:** All commands pass with no errors.

**Review Focus:**
- Thread safety (mutex usage)
- Field naming consistency
- Logger interface compatibility
- Security sanitization coverage

**Severity Handling:**
- Critical (build fails): Fix before proceeding
- High (lint errors): Fix before proceeding
- Medium (style issues): Note and continue
- Low (suggestions): Optional

---

### Batch 2: Unit Tests for Wide Event Package

#### Task 2.1: Create Wide Event Unit Tests

**File:** `pkg/mlog/wide_event_test.go`

**Action:** Create new file

```go
package mlog

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	libLog "github.com/LerianStudio/lib-commons/v2/commons/log"
)

func TestWideEvent_SetOrganization(t *testing.T) {
	event := &WideEvent{}
	orgID := uuid.New()

	event.SetOrganization(orgID)

	assert.Equal(t, orgID.String(), event.OrganizationID)
}

func TestWideEvent_SetOrganization_Nil(t *testing.T) {
	event := &WideEvent{}

	event.SetOrganization(uuid.Nil)

	assert.Empty(t, event.OrganizationID)
}

func TestWideEvent_SetLedger(t *testing.T) {
	event := &WideEvent{}
	ledgerID := uuid.New()

	event.SetLedger(ledgerID)

	assert.Equal(t, ledgerID.String(), event.LedgerID)
}

func TestWideEvent_SetTransaction(t *testing.T) {
	event := &WideEvent{}
	txnID := uuid.New()

	event.SetTransaction(txnID, "json")

	assert.Equal(t, txnID.String(), event.TransactionID)
	assert.Equal(t, "json", event.TransactionType)
}

func TestWideEvent_SetTransactionDetails(t *testing.T) {
	event := &WideEvent{}

	event.SetTransactionDetails("1000.50", "USD", 5, 2, 3)

	assert.Equal(t, "1000.50", event.TransactionAmount)
	assert.Equal(t, "USD", event.TransactionCurrency)
	assert.Equal(t, 5, event.OperationCount)
	assert.Equal(t, 2, event.SourceCount)
	assert.Equal(t, 3, event.DestinationCount)
}

func TestWideEvent_SetUser(t *testing.T) {
	event := &WideEvent{}

	event.SetUser("user-123", "admin", "jwt")

	assert.Equal(t, "user-123", event.UserID)
	assert.Equal(t, "admin", event.UserRole)
	assert.Equal(t, "jwt", event.AuthMethod)
}

func TestWideEvent_SetError(t *testing.T) {
	event := &WideEvent{}

	event.SetError("ValidationError", "INVALID_AMOUNT", "Amount must be positive", true)

	assert.True(t, event.ErrorOccurred)
	assert.Equal(t, "ValidationError", event.ErrorType)
	assert.Equal(t, "INVALID_AMOUNT", event.ErrorCode)
	assert.Equal(t, "Amount must be positive", event.ErrorMessage)
	assert.True(t, event.ErrorRetryable)
}

func TestWideEvent_SetDBStats(t *testing.T) {
	event := &WideEvent{}

	event.SetDBStats(5, 150)

	assert.Equal(t, 5, event.DBQueryCount)
	assert.Equal(t, int64(150), event.DBQueryTimeMS)
}

func TestWideEvent_SetCacheStats(t *testing.T) {
	event := &WideEvent{}

	event.SetCacheStats(10, 2)

	assert.Equal(t, 10, event.CacheHits)
	assert.Equal(t, 2, event.CacheMisses)
}

func TestWideEvent_SetIdempotency(t *testing.T) {
	event := &WideEvent{}

	event.SetIdempotency("idem-key-123", true)

	// Verify key is hashed (not exposed raw) - uses SHA256 prefix
	assert.True(t, strings.HasPrefix(event.IdempotencyKey, "idem_"))
	assert.NotEqual(t, "idem-key-123", event.IdempotencyKey) // Raw key should never be stored
	assert.True(t, event.IdempotencyHit)

	// Verify consistent hashing - same input produces same hash
	event2 := &WideEvent{}
	event2.SetIdempotency("idem-key-123", false)
	assert.Equal(t, event.IdempotencyKey, event2.IdempotencyKey) // Same key = same hash
}

func TestWideEvent_SetCustom(t *testing.T) {
	event := &WideEvent{Custom: make(map[string]any)}

	event.SetCustom("custom_field", "custom_value")
	event.SetCustom("custom_number", 42)

	assert.Equal(t, "custom_value", event.Custom["custom_field"])
	assert.Equal(t, 42, event.Custom["custom_number"])
}

func TestWideEvent_SetCustom_NilMap(t *testing.T) {
	event := &WideEvent{} // Custom is nil

	event.SetCustom("key", "value")

	require.NotNil(t, event.Custom)
	assert.Equal(t, "value", event.Custom["key"])
}

func TestWideEvent_Complete_Success(t *testing.T) {
	event := &WideEvent{StartTime: time.Now().Add(-100 * time.Millisecond)}

	event.Complete(200, 1024)

	assert.Equal(t, 200, event.StatusCode)
	assert.Equal(t, 1024, event.ResponseSize)
	assert.Equal(t, "success", event.Outcome)
	assert.GreaterOrEqual(t, event.DurationMS, int64(100))
}

func TestWideEvent_Complete_ClientError(t *testing.T) {
	event := &WideEvent{StartTime: time.Now()}

	event.Complete(400, 100)

	assert.Equal(t, "client_error", event.Outcome)
}

func TestWideEvent_Complete_ServerError(t *testing.T) {
	event := &WideEvent{StartTime: time.Now()}

	event.Complete(500, 50)

	assert.Equal(t, "error", event.Outcome)
}

func TestWideEvent_SetPanic(t *testing.T) {
	event := &WideEvent{}

	event.SetPanic("runtime error: nil pointer dereference")

	assert.Equal(t, "panic", event.Outcome)
	assert.True(t, event.ErrorOccurred)
	assert.Equal(t, "panic", event.ErrorType)
	assert.Equal(t, "runtime error: nil pointer dereference", event.ErrorMessage)
}

func TestWideEvent_ToFields_MinimalEvent(t *testing.T) {
	event := &WideEvent{
		Method:     "GET",
		Path:       "/test",
		StatusCode: 200,
		DurationMS: 50,
		Outcome:    "success",
		Service:    "test-service",
	}

	fields := event.toFields()

	// Convert to map for easier assertion
	fieldMap := make(map[string]any)
	for i := 0; i < len(fields); i += 2 {
		key := fields[i].(string)
		fieldMap[key] = fields[i+1]
	}

	assert.Equal(t, "GET", fieldMap["method"])
	assert.Equal(t, "/test", fieldMap["path"])
	assert.Equal(t, 200, fieldMap["status_code"])
	assert.Equal(t, int64(50), fieldMap["duration_ms"])
	assert.Equal(t, "success", fieldMap["outcome"])
	assert.Equal(t, "test-service", fieldMap["service"])
}

func TestWideEvent_ToFields_FullEvent(t *testing.T) {
	orgID := uuid.New()
	ledgerID := uuid.New()
	txnID := uuid.New()

	event := &WideEvent{
		RequestID:           "req-123",
		TraceID:             "trace-456",
		Method:              "POST",
		Path:                "/v1/transactions",
		StatusCode:          201,
		DurationMS:          150,
		Outcome:             "success",
		Service:             "transaction",
		OrganizationID:      orgID.String(),
		LedgerID:            ledgerID.String(),
		TransactionID:       txnID.String(),
		TransactionType:     "json",
		TransactionAmount:   "1000.00",
		TransactionCurrency: "USD",
		OperationCount:      3,
		DBQueryCount:        5,
		DBQueryTimeMS:       45,
		Custom:              map[string]any{"extra": "value"},
	}

	fields := event.toFields()

	// Convert to map
	fieldMap := make(map[string]any)
	for i := 0; i < len(fields); i += 2 {
		key := fields[i].(string)
		fieldMap[key] = fields[i+1]
	}

	assert.Equal(t, "req-123", fieldMap["request_id"])
	assert.Equal(t, "trace-456", fieldMap["trace_id"])
	assert.Equal(t, orgID.String(), fieldMap["organization_id"])
	assert.Equal(t, ledgerID.String(), fieldMap["ledger_id"])
	assert.Equal(t, txnID.String(), fieldMap["transaction_id"])
	assert.Equal(t, "json", fieldMap["transaction_type"])
	assert.Equal(t, "1000.00", fieldMap["transaction_amount"])
	assert.Equal(t, "USD", fieldMap["transaction_currency"])
	assert.Equal(t, 3, fieldMap["operation_count"])
	assert.Equal(t, 5, fieldMap["db_query_count"])
	assert.Equal(t, int64(45), fieldMap["db_query_time_ms"])
	assert.Equal(t, "value", fieldMap["custom.extra"])
}

func TestWideEvent_ConcurrentAccess(t *testing.T) {
	event := &WideEvent{Custom: make(map[string]any)}

	// Simulate concurrent access from multiple goroutines
	done := make(chan bool)

	for i := 0; i < 10; i++ {
		go func(id int) {
			event.SetOrganization(uuid.New())
			event.SetLedger(uuid.New())
			event.SetTransaction(uuid.New(), "json")
			event.SetError("test", "CODE", "message", false)
			event.SetCustom("key", id)
			done <- true
		}(i)
	}

	// Wait for all goroutines
	for i := 0; i < 10; i++ {
		<-done
	}

	// Should not panic - just verify we got through safely
	assert.NotEmpty(t, event.OrganizationID)
}

// mockLogger implements libLog.Logger for testing Emit()
type mockLogger struct {
	infoCalled bool
	fields     []any
}

// Verify mockLogger implements libLog.Logger at compile time
var _ libLog.Logger = (*mockLogger)(nil)

func (m *mockLogger) Info(args ...any)                               { m.infoCalled = true }
func (m *mockLogger) Infof(format string, args ...any)               {}
func (m *mockLogger) Infoln(args ...any)                             {}
func (m *mockLogger) Error(args ...any)                              {}
func (m *mockLogger) Errorf(format string, args ...any)              {}
func (m *mockLogger) Errorln(args ...any)                            {}
func (m *mockLogger) Warn(args ...any)                               {}
func (m *mockLogger) Warnf(format string, args ...any)               {}
func (m *mockLogger) Warnln(args ...any)                             {}
func (m *mockLogger) Debug(args ...any)                              {}
func (m *mockLogger) Debugf(format string, args ...any)              {}
func (m *mockLogger) Debugln(args ...any)                            {}
func (m *mockLogger) Fatal(args ...any)                              {}
func (m *mockLogger) Fatalf(format string, args ...any)              {}
func (m *mockLogger) Fatalln(args ...any)                            {}
func (m *mockLogger) WithFields(fields ...any) libLog.Logger {
	m.fields = fields
	return m
}
func (m *mockLogger) WithDefaultMessageTemplate(message string) libLog.Logger { return m }
func (m *mockLogger) Sync() error                                             { return nil }

func TestWideEvent_Emit(t *testing.T) {
	mockLog := &mockLogger{}
	event := &WideEvent{
		Method:     "GET",
		Path:       "/test",
		StatusCode: 200,
		DurationMS: 50,
		Outcome:    "success",
		Service:    "test-service",
	}

	event.Emit(mockLog)

	assert.True(t, mockLog.infoCalled)
	assert.NotEmpty(t, mockLog.fields)

	// Verify expected fields are present
	fieldMap := make(map[string]any)
	for i := 0; i < len(mockLog.fields); i += 2 {
		if key, ok := mockLog.fields[i].(string); ok {
			fieldMap[key] = mockLog.fields[i+1]
		}
	}

	assert.Equal(t, "GET", fieldMap["method"])
	assert.Equal(t, "/test", fieldMap["path"])
	assert.Equal(t, 200, fieldMap["status_code"])
	assert.Equal(t, "test-service", fieldMap["service"])
}

func TestWideEvent_OutcomeClassification(t *testing.T) {
	tests := []struct {
		statusCode int
		expected   string
	}{
		{200, "success"},
		{201, "success"},
		{204, "success"},
		{301, "success"}, // Redirects classified as success (expected behavior)
		{400, "client_error"},
		{404, "client_error"},
		{422, "client_error"},
		{499, "client_error"},
		{500, "error"},
		{502, "error"},
		{503, "error"},
	}
	for _, tt := range tests {
		t.Run(fmt.Sprintf("status_%d", tt.statusCode), func(t *testing.T) {
			event := &WideEvent{StartTime: time.Now()}
			event.Complete(tt.statusCode, 0)
			assert.Equal(t, tt.expected, event.Outcome)
		})
	}
}

func TestWideEvent_SetCustom_BoundsChecking(t *testing.T) {
	event := &WideEvent{}

	// Test max keys limit
	for i := 0; i < 60; i++ {
		event.SetCustom(fmt.Sprintf("key_%d", i), "value")
	}
	assert.LessOrEqual(t, len(event.Custom), maxCustomKeys)

	// Test key truncation
	event2 := &WideEvent{}
	longKey := strings.Repeat("a", 100)
	event2.SetCustom(longKey, "value")
	for k := range event2.Custom {
		assert.LessOrEqual(t, len(k), maxCustomKeyLen)
	}

	// Test value truncation
	event3 := &WideEvent{}
	longValue := strings.Repeat("b", 2000)
	event3.SetCustom("key", longValue)
	if strVal, ok := event3.Custom["key"].(string); ok {
		assert.LessOrEqual(t, len(strVal), maxCustomValueLen+20) // +20 for truncation suffix
	}
}
```

**Verification:**
```bash
cd /Users/fredamaral/repos/lerianstudio/midaz && go test ./pkg/mlog/... -v -race
# Expected: All tests pass, no race conditions detected
```

---

#### Task 2.2: Create Security Sanitization Tests

**File:** `pkg/mlog/sanitize_test.go`

**Action:** Create new file

```go
package mlog

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSanitizeQueryParams(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "redacts token parameter",
			input:    "token=secret123&normal=value",
			expected: "token=REDACTED&normal=value",
		},
		{
			name:     "redacts api_key parameter",
			input:    "api_key=xyz789&page=1",
			expected: "api_key=REDACTED&page=1",
		},
		{
			name:     "redacts password parameter",
			input:    "password=hunter2&user=admin",
			expected: "password=REDACTED&user=admin",
		},
		{
			name:     "redacts multiple sensitive params",
			input:    "token=abc&secret=xyz&auth=123",
			expected: "token=REDACTED&secret=REDACTED&auth=REDACTED",
		},
		{
			name:     "preserves normal parameters",
			input:    "page=1&limit=10&sort=asc",
			expected: "page=1&limit=10&sort=asc",
		},
		{
			name:     "handles empty string",
			input:    "",
			expected: "",
		},
		{
			name:     "case insensitive matching",
			input:    "TOKEN=secret&API_KEY=xyz",
			expected: "TOKEN=REDACTED&API_KEY=REDACTED",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := sanitizeQueryParams(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestSanitizeErrorMessage(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		contains string
		notContains string
	}{
		{
			name:        "redacts postgres connection string",
			input:       "pq: connection failed: postgres://user:pass@host:5432/db",
			contains:    "[CONNECTION_REDACTED]",
			notContains: "user:pass",
		},
		{
			name:        "redacts mongodb connection string",
			input:       "connection error: mongodb://admin:secret@localhost:27017/mydb",
			contains:    "[CONNECTION_REDACTED]",
			notContains: "admin:secret",
		},
		{
			name:        "redacts file paths",
			input:       "failed to open /home/user/app/config/secrets.json",
			contains:    "[PATH_REDACTED]",
			notContains: "/home/user",
		},
		{
			name:        "truncates long messages",
			input:       strings.Repeat("a", 1000),
			contains:    "[TRUNCATED]",
		},
		{
			name:        "preserves normal error messages",
			input:       "validation failed: amount must be positive",
			contains:    "validation failed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := sanitizeErrorMessage(tt.input)
			if tt.contains != "" {
				assert.Contains(t, result, tt.contains)
			}
			if tt.notContains != "" {
				assert.NotContains(t, result, tt.notContains)
			}
		})
	}
}

func TestSanitizeHeader(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		notContains string
		maxLen      int
	}{
		{
			name:        "removes newlines (CRLF injection)",
			input:       "value\nFAKE LOG ENTRY",
			notContains: "\n",
		},
		{
			name:        "removes carriage returns",
			input:       "value\rFAKE LOG",
			notContains: "\r",
		},
		{
			name:        "escapes HTML (XSS prevention)",
			input:       "<script>alert('xss')</script>",
			notContains: "<script>",
		},
		{
			name:   "truncates long values",
			input:  strings.Repeat("a", 500),
			maxLen: maxHeaderLength + 20, // Allow for truncation suffix
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := sanitizeHeader(tt.input)
			if tt.notContains != "" {
				assert.NotContains(t, result, tt.notContains)
			}
			if tt.maxLen > 0 {
				assert.LessOrEqual(t, len(result), tt.maxLen)
			}
		})
	}
}

func TestAnonymizeIP(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "anonymizes IPv4",
			input:    "192.168.1.123",
			expected: "192.168.1.0",
		},
		{
			name:     "anonymizes IPv4 localhost",
			input:    "127.0.0.1",
			expected: "127.0.0.0",
		},
		{
			name:     "handles invalid IP",
			input:    "not-an-ip",
			expected: "invalid",
		},
		{
			name:     "handles empty string",
			input:    "",
			expected: "invalid",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := anonymizeIP(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestHashIdempotencyKey(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{name: "hashes normal key", input: "user-123-txn-456"},
		{name: "handles empty key", input: ""},
		{name: "hashes UUID-like key", input: "550e8400-e29b-41d4-a716-446655440000"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := hashIdempotencyKey(tt.input)

			if tt.input == "" {
				assert.Empty(t, result)
			} else {
				// Verify hash format
				assert.True(t, strings.HasPrefix(result, "idem_"))
				// Verify original key is not exposed
				assert.NotContains(t, result, tt.input)
				// Verify consistent hashing
				result2 := hashIdempotencyKey(tt.input)
				assert.Equal(t, result, result2)
			}
		})
	}
}
```

**Verification:**
```bash
cd /Users/fredamaral/repos/lerianstudio/midaz && go test ./pkg/mlog/... -v -run TestSanitize
# Expected: All sanitization tests pass
```

---

### ✅ Code Review Checkpoint 2

**After completing Batch 2, run:**
```bash
cd /Users/fredamaral/repos/lerianstudio/midaz

# Run tests with race detection
go test ./pkg/mlog/... -v -race -coverprofile=coverage.out

# Check coverage
go tool cover -func=coverage.out | grep total
```

**Expected:**
- All tests pass
- No race conditions
- Coverage > 70%

---

### Batch 3: Integrate Wide Events into Transaction Component

#### Task 3.1: Add Wide Event Middleware to Transaction Routes

**File:** `components/transaction/internal/adapters/http/in/routes.go`

**Action:** Modify existing file

**CRITICAL: The actual NewRouter signature has 9 parameters. We add 2 more for wide events.**

**Find the current NewRouter signature (around line 37):**
```go
func NewRouter(
	lg libLog.Logger,
	tl *libOpentelemetry.Telemetry,
	auth *middleware.AuthClient,
	th *TransactionHandler,
	oh *OperationHandler,
	ah *AssetRateHandler,
	bh *BalanceHandler,
	orh *OperationRouteHandler,
	trh *TransactionRouteHandler,
) *fiber.App {
```

**Update to add version and envName parameters AFTER telemetry:**
```go
func NewRouter(
	lg libLog.Logger,
	tl *libOpentelemetry.Telemetry,
	version string,
	envName string,
	auth *middleware.AuthClient,
	th *TransactionHandler,
	oh *OperationHandler,
	ah *AssetRateHandler,
	bh *BalanceHandler,
	orh *OperationRouteHandler,
	trh *TransactionRouteHandler,
) *fiber.App {
```

**Find the imports section (around line 3-20):**
```go
import (
	"fmt"
	"runtime/debug"
	"time"
```

**Add import:**
```go
import (
	"fmt"
	"runtime/debug"
	"strings"
	"time"

	"github.com/LerianStudio/midaz/v3/pkg/mlog"
```

**Find the panic recovery middleware (around line 53-75):**
```go
f.Use(recover.New(recover.Config{
	EnableStackTrace: true,
	StackTraceHandler: func(c *fiber.Ctx, e any) {
		// ... existing code ...
	},
}))
```

**Update to store panic value for wide event middleware:**
```go
f.Use(recover.New(recover.Config{
	EnableStackTrace: true,
	StackTraceHandler: func(c *fiber.Ctx, e any) {
		// Store panic value for wide event middleware to capture
		c.Locals("panic_value", fmt.Sprintf("%v", e))

		// ... existing span recording code ...
	},
}))
```

**Find the CORS middleware (around line 80):**
```go
	f.Use(cors.New())

	// HTTP logging (with custom logger)
	f.Use(libHTTP.WithHTTPLogging(libHTTP.WithCustomLogger(lg)))
```

**Add wide event middleware AFTER CORS but BEFORE HTTP logging:**
```go
	f.Use(cors.New())

	// Wide event middleware - emits ONE canonical log line per request
	// Position: after telemetry (trace context available), before HTTP logging
	f.Use(mlog.NewWideEventMiddleware(mlog.Config{
		ServiceName: "transaction",
		Version:     version,
		Environment: envName,
		Logger:      lg,
		Skip:        mlog.DefaultSkipPaths(),
		AnonymizeIP: false, // Set to true for GDPR compliance if needed
	}))

	// HTTP logging (with custom logger)
	f.Use(libHTTP.WithHTTPLogging(libHTTP.WithCustomLogger(lg)))
```

**Verification:**
```bash
cd /Users/fredamaral/repos/lerianstudio/midaz && go build ./components/transaction/...
# Expected: Build errors due to missing parameters at call site - we fix in next task
```

---

#### Task 3.2: Update Config to Pass Version and Environment

**File:** `components/transaction/internal/bootstrap/config.go`

**Action:** Modify existing file

**CRITICAL: NewRouter is called in config.go (around line 369), NOT service.go.**
**CRITICAL: Use cfg.OtelServiceVersion, NOT cfg.Version (which doesn't exist).**

**Find where NewRouter is called (around line 369 in config.go):**
```go
app := in.NewRouter(
	logger,
	telemetry,
	auth,
	transactionHandler,
	operationHandler,
	assetRateHandler,
	balanceHandler,
	operationRouteHandler,
	transactionRouteHandler,
)
```

**Update the call to include version and environment AFTER telemetry:**
```go
app := in.NewRouter(
	logger,
	telemetry,
	cfg.OtelServiceVersion,  // Use OtelServiceVersion (cfg.Version doesn't exist)
	cfg.EnvName,
	auth,
	transactionHandler,
	operationHandler,
	assetRateHandler,
	balanceHandler,
	operationRouteHandler,
	transactionRouteHandler,
)
```

**Note:** The Config struct already has these fields:
- `OtelServiceVersion` (line ~123) - the service version
- `EnvName` (line ~47) - the environment name

**Verification:**
```bash
cd /Users/fredamaral/repos/lerianstudio/midaz && go build ./components/transaction/...
# Expected: No build errors
```

---

#### Task 3.3: Enrich Wide Events in Transaction Handler

**File:** `components/transaction/internal/adapters/http/in/transaction.go`

**Action:** Modify existing file

**CRITICAL: The ID extraction happens in the PRIVATE `createTransaction` method (line ~1183), NOT the public handler.**

**Add import (around line 3-15):**
```go
import (
	// ... existing imports ...
	"github.com/LerianStudio/midaz/v3/pkg/mlog"
)
```

**Find the PRIVATE createTransaction method (around line 1183):**
```go
func (handler *TransactionHandler) createTransaction(
	c *fiber.Ctx,
	parserDSL pkgTransaction.Transaction,
	transactionStatus string,
) error {
	ctx := c.UserContext()
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)
	// ...
	organizationID := http.LocalUUID(c, "organization_id")
	ledgerID := http.LocalUUID(c, "ledger_id")
```

**Add wide event enrichment AFTER the ID extraction (around line 1192):**
```go
	organizationID := http.LocalUUID(c, "organization_id")
	ledgerID := http.LocalUUID(c, "ledger_id")

	// Enrich wide event with business context
	mlog.EnrichTransaction(c, organizationID, ledgerID, transactionStatus)
	mlog.SetHandler(c, "create_transaction")
```

**For the PUBLIC CreateTransactionJSON handler (around line 74), add handler-specific context:**
```go
func (handler *TransactionHandler) CreateTransactionJSON(p any, c *fiber.Ctx) error {
	// Set handler name early for wide event tracking
	mlog.SetHandler(c, "create_transaction_json")

	// ... existing code ...
```

**Also update CreateTransactionDSL, CreateTransactionAnnotation, CreateTransactionInflow, CreateTransactionOutflow similarly with their respective handler names:**
- `mlog.SetHandler(c, "create_transaction_dsl")`
- `mlog.SetHandler(c, "create_transaction_annotation")`
- `mlog.SetHandler(c, "create_transaction_inflow")`
- `mlog.SetHandler(c, "create_transaction_outflow")`

**After successfully creating the transaction (around line 1230-1240), update with the result:**
```go
	// Before returning success, update wide event with created transaction
	mlog.EnrichTransactionResult(c, transaction.ID, string(transaction.Status), len(transaction.Operations))

	return http.Created(c, transaction)
```

**Add idempotency enrichment in handleIdempotency method (around line 1302):**

**Find:**
```go
	c.Set(libConstants.IdempotencyReplayed, "true")
	return &t, nil
```

**Add before return:**
```go
	c.Set(libConstants.IdempotencyReplayed, "true")

	// Enrich wide event with idempotency hit
	if event := mlog.GetWideEvent(c); event != nil {
		event.SetIdempotency(key, true)
	}

	return &t, nil
```

**Verification:**
```bash
cd /Users/fredamaral/repos/lerianstudio/midaz && go build ./components/transaction/...
# Expected: No build errors
```

---

#### Task 3.4: Enrich Wide Events on Error Paths

**File:** `components/transaction/internal/adapters/http/in/transaction.go`

**Action:** Modify existing file

**Find error handling patterns in CreateTransactionJSON, typically:**
```go
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to create transaction", err)
		logger.Errorf("Failed to create transaction: %v", err)
		return http.ErrorDispatcher(c, err)
	}
```

**Add wide event error enrichment before the return:**
```go
	if err != nil {
		// Enrich wide event with error context
		if event := mlog.GetWideEvent(c); event != nil {
			errType := fmt.Sprintf("%T", err)
			event.SetError(errType, "", err.Error(), false)
		}
		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to create transaction", err)
		logger.Errorf("Failed to create transaction: %v", err)
		return http.ErrorDispatcher(c, err)
	}
```

**Verification:**
```bash
cd /Users/fredamaral/repos/lerianstudio/midaz && go build ./components/transaction/...
# Expected: No build errors
```

---

### ✅ Code Review Checkpoint 3

**After completing Batch 3, run:**
```bash
cd /Users/fredamaral/repos/lerianstudio/midaz

# Build entire project
go build ./...

# Run transaction component tests
go test ./components/transaction/... -v

# Start the service locally and make a test request
# (This requires the full infrastructure - optional)
```

**Expected:**
- Build succeeds
- Existing tests pass
- No regressions

**Review Focus:**
- Middleware ordering correct
- Wide event not breaking existing functionality
- Error paths properly enriched

---

### Batch 4: Add Wide Event Helper for Common Patterns

#### Task 4.1: Create Handler Helpers

**File:** `pkg/mlog/helpers.go`

**Action:** Create new file

```go
package mlog

import (
	"fmt"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
)

// EnrichTransaction is a convenience function to enrich wide event with transaction context.
// Call this at the start of transaction handlers after extracting IDs.
func EnrichTransaction(c *fiber.Ctx, orgID, ledgerID uuid.UUID, txnType string) {
	event := GetWideEvent(c)
	if event == nil {
		return
	}

	event.SetOrganization(orgID)
	event.SetLedger(ledgerID)
	event.SetTransaction(uuid.Nil, txnType)
}

// EnrichTransactionResult updates the wide event with the created/updated transaction.
func EnrichTransactionResult(c *fiber.Ctx, txnID uuid.UUID, status string, opCount int) {
	event := GetWideEvent(c)
	if event == nil {
		return
	}

	event.mu.Lock()
	if txnID != uuid.Nil {
		event.TransactionID = txnID.String()
	}
	event.OperationCount = opCount
	event.mu.Unlock()

	// Use SetCustom for consistent bounds checking
	event.SetCustom("transaction_status", status)
}

// EnrichAccount is a convenience function to enrich wide event with account context.
func EnrichAccount(c *fiber.Ctx, orgID, ledgerID, accountID uuid.UUID) {
	event := GetWideEvent(c)
	if event == nil {
		return
	}

	event.SetOrganization(orgID)
	event.SetLedger(ledgerID)
	event.SetAccount(accountID)
}

// EnrichBalance is a convenience function to enrich wide event with balance context.
func EnrichBalance(c *fiber.Ctx, orgID, ledgerID, accountID, balanceID uuid.UUID) {
	event := GetWideEvent(c)
	if event == nil {
		return
	}

	event.SetOrganization(orgID)
	event.SetLedger(ledgerID)
	event.SetAccount(accountID)
	event.SetBalance(balanceID)
}

// EnrichOperation is a convenience function to enrich wide event with operation context.
func EnrichOperation(c *fiber.Ctx, orgID, ledgerID, txnID, opID uuid.UUID) {
	event := GetWideEvent(c)
	if event == nil {
		return
	}

	event.SetOrganization(orgID)
	event.SetLedger(ledgerID)
	event.SetTransaction(txnID, "")
	event.SetOperation(opID)
}

// EnrichError adds error context to the wide event.
// Call this before returning an error response.
func EnrichError(c *fiber.Ctx, err error, retryable bool) {
	event := GetWideEvent(c)
	if event == nil || err == nil {
		return
	}

	errType := fmt.Sprintf("%T", err)
	event.SetError(errType, "", err.Error(), retryable)
}

// EnrichErrorWithCode adds error context with a specific code.
func EnrichErrorWithCode(c *fiber.Ctx, err error, code string, retryable bool) {
	event := GetWideEvent(c)
	if event == nil || err == nil {
		return
	}

	errType := fmt.Sprintf("%T", err)
	event.SetError(errType, code, err.Error(), retryable)
}

// SetHandler sets a custom field indicating which handler processed the request.
func SetHandler(c *fiber.Ctx, handlerName string) {
	event := GetWideEvent(c)
	if event == nil {
		return
	}

	event.SetCustom("handler", handlerName)
}

// TrackDBQuery increments the DB query counter.
// Call this after each database operation for performance tracking.
func TrackDBQuery(c *fiber.Ctx, durationMS int64) {
	event := GetWideEvent(c)
	if event == nil {
		return
	}

	event.mu.Lock()
	defer event.mu.Unlock()
	event.DBQueryCount++
	event.DBQueryTimeMS += durationMS
}

// TrackCacheAccess records cache hit/miss.
func TrackCacheAccess(c *fiber.Ctx, hit bool) {
	event := GetWideEvent(c)
	if event == nil {
		return
	}

	event.mu.Lock()
	defer event.mu.Unlock()
	if hit {
		event.CacheHits++
	} else {
		event.CacheMisses++
	}
}

// TrackExternalCall tracks external service calls.
func TrackExternalCall(c *fiber.Ctx, durationMS int64) {
	event := GetWideEvent(c)
	if event == nil {
		return
	}

	event.mu.Lock()
	defer event.mu.Unlock()
	event.ExternalCallCount++
	event.ExternalCallTimeMS += durationMS
}

// SetIdempotency sets idempotency context on the wide event.
// The key is automatically hashed for security.
func SetIdempotency(c *fiber.Ctx, key string, hit bool) {
	event := GetWideEvent(c)
	if event == nil {
		return
	}
	event.SetIdempotency(key, hit)
}

// EnrichTransactionAction sets context for transaction state changes.
// Use for commit, cancel, revert operations on existing transactions.
func EnrichTransactionAction(c *fiber.Ctx, orgID, ledgerID, txnID uuid.UUID, action string) {
	event := GetWideEvent(c)
	if event == nil {
		return
	}

	event.SetOrganization(orgID)
	event.SetLedger(ledgerID)
	event.SetTransaction(txnID, "")
	event.SetCustom("transaction_action", action)
}

// EnrichHolder sets holder context on the wide event.
func EnrichHolder(c *fiber.Ctx, holderID uuid.UUID) {
	event := GetWideEvent(c)
	if event == nil {
		return
	}
	event.SetHolder(holderID)
}

// EnrichPortfolio sets portfolio context on the wide event.
func EnrichPortfolio(c *fiber.Ctx, portfolioID uuid.UUID) {
	event := GetWideEvent(c)
	if event == nil {
		return
	}
	event.SetPortfolio(portfolioID)
}

// EnrichAssetRate sets asset rate context on the wide event.
func EnrichAssetRate(c *fiber.Ctx, externalID string) {
	event := GetWideEvent(c)
	if event == nil {
		return
	}
	event.SetAssetRateExternalID(externalID)
}

// EnrichOperationRoute sets operation route context on the wide event.
func EnrichOperationRoute(c *fiber.Ctx, orgID, ledgerID, routeID uuid.UUID) {
	event := GetWideEvent(c)
	if event == nil {
		return
	}

	event.SetOrganization(orgID)
	event.SetLedger(ledgerID)
	event.SetOperationRoute(routeID)
}

// EnrichTransactionRoute sets transaction route context on the wide event.
func EnrichTransactionRoute(c *fiber.Ctx, orgID, ledgerID, routeID uuid.UUID) {
	event := GetWideEvent(c)
	if event == nil {
		return
	}

	event.SetOrganization(orgID)
	event.SetLedger(ledgerID)
	event.SetTransactionRoute(routeID)
}
```

**Verification:**
```bash
cd /Users/fredamaral/repos/lerianstudio/midaz && go build ./pkg/mlog/...
# Expected: No build errors
```

---

#### Task 4.2: Simplify Handler Enrichment Using Helpers

**File:** `components/transaction/internal/adapters/http/in/transaction.go`

**Action:** Modify existing file

**Replace the verbose enrichment code added in Task 3.3 with the helper functions:**

**Find:**
```go
	// Enrich wide event with business context
	if event := mlog.GetWideEvent(c); event != nil {
		event.SetOrganization(organizationID)
		event.SetLedger(ledgerID)
		event.SetCustom("handler", "create_transaction_json")
	}
```

**Replace with:**
```go
	// Enrich wide event with business context
	mlog.EnrichTransaction(c, organizationID, ledgerID, "json")
	mlog.SetHandler(c, "create_transaction_json")
```

**Find:**
```go
	// Update wide event with created transaction ID
	if event := mlog.GetWideEvent(c); event != nil {
		event.SetTransaction(result.ID, "json")
		event.SetCustom("transaction_status", result.Status)
	}
```

**Replace with:**
```go
	// Update wide event with created transaction ID
	mlog.EnrichTransactionResult(c, result.ID, string(result.Status), len(result.Operations))
```

**Find:**
```go
		// Enrich wide event with error context
		if event := mlog.GetWideEvent(c); event != nil {
			errType := fmt.Sprintf("%T", err)
			event.SetError(errType, "", err.Error(), false)
		}
```

**Replace with:**
```go
		// Enrich wide event with error context
		mlog.EnrichError(c, err, false)
```

**Verification:**
```bash
cd /Users/fredamaral/repos/lerianstudio/midaz && go build ./components/transaction/...
# Expected: No build errors
```

---

### ✅ Code Review Checkpoint 4

**After completing Batch 4, run:**
```bash
cd /Users/fredamaral/repos/lerianstudio/midaz

# Full build
go build ./...

# Run all tests
go test ./... -v -short

# Lint
golangci-lint run ./pkg/mlog/... ./components/transaction/...
```

**Expected:**
- All builds pass
- All tests pass
- No lint errors

---

### Batch 5: Apply to Additional Handlers and Components

#### Task 5.1: Add Wide Events to Balance Handler

**File:** `components/transaction/internal/adapters/http/in/balance.go`

**Action:** Modify existing file

**Add import:**
```go
import (
	// ... existing imports ...
	"github.com/LerianStudio/midaz/v3/pkg/mlog"
)
```

**In GetAllBalances handler, after extracting IDs:**
```go
	organizationID := http.LocalUUID(c, "organization_id")
	ledgerID := http.LocalUUID(c, "ledger_id")

	// Enrich wide event
	mlog.EnrichBalance(c, organizationID, ledgerID, uuid.Nil, uuid.Nil)
	mlog.SetHandler(c, "get_all_balances")
```

**In GetBalanceByID handler, after extracting IDs:**
```go
	organizationID := http.LocalUUID(c, "organization_id")
	ledgerID := http.LocalUUID(c, "ledger_id")
	accountID := http.LocalUUID(c, "account_id")
	balanceID := http.LocalUUID(c, "balance_id")

	// Enrich wide event
	mlog.EnrichBalance(c, organizationID, ledgerID, accountID, balanceID)
	mlog.SetHandler(c, "get_balance_by_id")
```

**Verification:**
```bash
cd /Users/fredamaral/repos/lerianstudio/midaz && go build ./components/transaction/...
# Expected: No build errors
```

---

#### Task 5.2: Add Wide Events to Account Handler

**File:** `components/transaction/internal/adapters/http/in/account.go`

**Action:** Modify existing file

**Add import:**
```go
import (
	// ... existing imports ...
	"github.com/LerianStudio/midaz/v3/pkg/mlog"
)
```

**In each handler, after extracting IDs, add:**
```go
	// Enrich wide event
	mlog.EnrichAccount(c, organizationID, ledgerID, accountID)
	mlog.SetHandler(c, "handler_name")
```

**Verification:**
```bash
cd /Users/fredamaral/repos/lerianstudio/midaz && go build ./components/transaction/...
# Expected: No build errors
```

---

#### Task 5.3: Add Wide Events to Operation Handler

**File:** `components/transaction/internal/adapters/http/in/operation.go`

**Action:** Modify existing file

**Add import:**
```go
import (
	// ... existing imports ...
	"github.com/LerianStudio/midaz/v3/pkg/mlog"
)
```

**In each handler, after extracting IDs, add:**
```go
	// Enrich wide event
	mlog.EnrichOperation(c, organizationID, ledgerID, transactionID, operationID)
	mlog.SetHandler(c, "handler_name")
```

**Verification:**
```bash
cd /Users/fredamaral/repos/lerianstudio/midaz && go build ./components/transaction/...
# Expected: No build errors
```

---

### ✅ Code Review Checkpoint 5 (Final)

**After completing all batches, run:**
```bash
cd /Users/fredamaral/repos/lerianstudio/midaz

# Full build
go build ./...

# Run all tests with race detection
go test ./... -race -short

# Full lint check
golangci-lint run ./...

# Check for any TODO or FIXME comments added
rg "TODO|FIXME" pkg/mlog/ components/transaction/internal/adapters/http/in/
```

**Expected:**
- All builds pass
- All tests pass with no race conditions
- No lint errors
- No unresolved TODOs

---

## Verification Commands Summary

### Build Verification
```bash
cd /Users/fredamaral/repos/lerianstudio/midaz
go build ./...
```

### Test Verification
```bash
cd /Users/fredamaral/repos/lerianstudio/midaz
go test ./pkg/mlog/... -v -race -coverprofile=coverage.out
go tool cover -func=coverage.out | grep total
```

### Lint Verification
```bash
cd /Users/fredamaral/repos/lerianstudio/midaz
golangci-lint run ./pkg/mlog/... ./components/transaction/...
```

### Integration Test (Manual)
```bash
# Start the transaction service
cd /Users/fredamaral/repos/lerianstudio/midaz
make run-transaction

# In another terminal, make a test request
curl -X POST http://localhost:3002/v1/organizations/{org_id}/ledgers/{ledger_id}/transactions/json \
  -H "Content-Type: application/json" \
  -H "X-Request-Id: test-123" \
  -d '{"send": {"asset": "USD", "value": "100.00", ...}}'

# Check logs for wide_event entries
# Expected: Single log line with all context fields
```

---

## Failure Recovery

### Build Fails

1. Check import paths are correct (v3 module path)
2. Ensure all new files are created in correct locations
3. Run `go mod tidy` to resolve dependency issues

### Tests Fail

1. Check test assertions match actual struct field names
2. Ensure mutex is properly used for concurrent tests
3. Run tests individually to isolate failures: `go test ./pkg/mlog/... -run TestName -v`

### Lint Fails

1. Run `gofmt -w pkg/mlog/` to fix formatting
2. Address any unused variable/import warnings
3. Check for missing error handling

### Wide Events Not Appearing in Logs

1. Verify middleware is added in correct order (after telemetry, before HTTP logging)
2. Check that `DefaultSkipPaths()` is not skipping your test path
3. Ensure logger is properly passed to middleware config
4. Verify handler is calling enrichment functions

---

## Agent Recommendations

| Task Batch | Recommended Agent |
|------------|-------------------|
| Batch 1: Core Infrastructure | `backend-engineer-golang` |
| Batch 2: Unit Tests | `qa-analyst` |
| Batch 3: Integration | `backend-engineer-golang` |
| Batch 4: Helpers | `backend-engineer-golang` |
| Batch 5: Rollout | `backend-engineer-golang` |
| Code Review | `code-reviewer` + `business-logic-reviewer` (parallel) |

---

## Future Enhancements (Out of Scope)

1. **Tail Sampling** - Implement sampling logic to reduce log volume while keeping all errors
2. **Onboarding/Ledger Components** - Apply same pattern to other services
3. **OTLP Collector Integration** - Export wide events to observability platform
4. **Dashboard Queries** - Create example queries for common debugging scenarios
5. **lib-commons Enhancement** - PR to add span processor registration API
