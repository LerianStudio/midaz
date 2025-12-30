package mlog

import (
	"fmt"
	"sync"
	"testing"
	"time"

	libLog "github.com/LerianStudio/lib-commons/v2/commons/log"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// =============================================================================
// Setter Methods Tests
// =============================================================================

func TestWideEvent_SetOrganization(t *testing.T) {
	tests := []struct {
		name     string
		orgID    string
		expected string
	}{
		{
			name:     "sets valid organization ID",
			orgID:    "org-123-abc",
			expected: "org-123-abc",
		},
		{
			name:     "sets UUID format",
			orgID:    "550e8400-e29b-41d4-a716-446655440000",
			expected: "550e8400-e29b-41d4-a716-446655440000",
		},
		{
			name:     "sets empty string",
			orgID:    "",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			event := &WideEvent{}
			event.SetOrganization(tt.orgID)
			assert.Equal(t, tt.expected, event.OrganizationID)
		})
	}
}

func TestWideEvent_SetLedger(t *testing.T) {
	tests := []struct {
		name     string
		ledgerID string
		expected string
	}{
		{
			name:     "sets valid ledger ID",
			ledgerID: "ledger-456",
			expected: "ledger-456",
		},
		{
			name:     "sets empty string",
			ledgerID: "",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			event := &WideEvent{}
			event.SetLedger(tt.ledgerID)
			assert.Equal(t, tt.expected, event.LedgerID)
		})
	}
}

func TestWideEvent_SetTransaction(t *testing.T) {
	tests := []struct {
		name         string
		txnID        string
		txnType      string
		amount       string
		currency     string
		expectedID   string
		expectedType string
	}{
		{
			name:         "sets all transaction fields",
			txnID:        "txn-789",
			txnType:      "json",
			amount:       "1000.50",
			currency:     "USD",
			expectedID:   "txn-789",
			expectedType: "json",
		},
		{
			name:         "sets DSL transaction type",
			txnID:        "txn-dsl-001",
			txnType:      "dsl",
			amount:       "500.00",
			currency:     "EUR",
			expectedID:   "txn-dsl-001",
			expectedType: "dsl",
		},
		{
			name:         "handles empty values",
			txnID:        "",
			txnType:      "",
			amount:       "",
			currency:     "",
			expectedID:   "",
			expectedType: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			event := &WideEvent{}
			event.SetTransaction(tt.txnID, tt.txnType, tt.amount, tt.currency)

			assert.Equal(t, tt.expectedID, event.TransactionID)
			assert.Equal(t, tt.expectedType, event.TransactionType)
			assert.Equal(t, tt.amount, event.TransactionAmount)
			assert.Equal(t, tt.currency, event.TransactionCurrency)
		})
	}
}

func TestWideEvent_SetAccount(t *testing.T) {
	event := &WideEvent{}
	event.SetAccount("account-001")
	assert.Equal(t, "account-001", event.AccountID)
}

func TestWideEvent_SetBalance(t *testing.T) {
	event := &WideEvent{}
	event.SetBalance("balance-002")
	assert.Equal(t, "balance-002", event.BalanceID)
}

func TestWideEvent_SetOperation(t *testing.T) {
	event := &WideEvent{}
	event.SetOperation("op-123", 5)

	assert.Equal(t, "op-123", event.OperationID)
	assert.Equal(t, 5, event.OperationCount)
}

func TestWideEvent_SetAsset(t *testing.T) {
	event := &WideEvent{}
	event.SetAsset("BRL")
	assert.Equal(t, "BRL", event.AssetCode)
}

func TestWideEvent_SetHolder(t *testing.T) {
	event := &WideEvent{}
	event.SetHolder("holder-789")
	assert.Equal(t, "holder-789", event.HolderID)
}

func TestWideEvent_SetPortfolio(t *testing.T) {
	event := &WideEvent{}
	event.SetPortfolio("portfolio-abc")
	assert.Equal(t, "portfolio-abc", event.PortfolioID)
}

func TestWideEvent_SetSegment(t *testing.T) {
	event := &WideEvent{}
	event.SetSegment("segment-xyz")
	assert.Equal(t, "segment-xyz", event.SegmentID)
}

func TestWideEvent_SetAssetRateExternalID(t *testing.T) {
	event := &WideEvent{}
	event.SetAssetRateExternalID("ext-rate-001")
	assert.Equal(t, "ext-rate-001", event.AssetRateExternalID)
}

func TestWideEvent_SetOperationRoute(t *testing.T) {
	event := &WideEvent{}
	event.SetOperationRoute("op-route-001")
	assert.Equal(t, "op-route-001", event.OperationRouteID)
}

func TestWideEvent_SetTransactionRoute(t *testing.T) {
	event := &WideEvent{}
	event.SetTransactionRoute("txn-route-001")
	assert.Equal(t, "txn-route-001", event.TransactionRouteID)
}

func TestWideEvent_SetOperationCounts(t *testing.T) {
	event := &WideEvent{}
	event.SetOperationCounts(3, 5)

	assert.Equal(t, 3, event.SourceCount)
	assert.Equal(t, 5, event.DestinationCount)
}

func TestWideEvent_SetUser(t *testing.T) {
	event := &WideEvent{}
	event.SetUser("user-123", "admin", "jwt")

	assert.Equal(t, "user-123", event.UserID)
	assert.Equal(t, "admin", event.UserRole)
	assert.Equal(t, "jwt", event.AuthMethod)
}

func TestWideEvent_SetService(t *testing.T) {
	event := &WideEvent{}
	event.SetService("transaction", "v1.2.3", "production")

	assert.Equal(t, "transaction", event.Service)
	assert.Equal(t, "v1.2.3", event.Version)
	assert.Equal(t, "production", event.Environment)
}

func TestWideEvent_SetError(t *testing.T) {
	tests := []struct {
		name      string
		errType   string
		code      string
		message   string
		retryable bool
	}{
		{
			name:      "sets validation error",
			errType:   "ValidationError",
			code:      "INVALID_AMOUNT",
			message:   "Amount must be positive",
			retryable: false,
		},
		{
			name:      "sets retryable error",
			errType:   "DatabaseError",
			code:      "CONNECTION_TIMEOUT",
			message:   "Database connection timed out",
			retryable: true,
		},
		{
			name:      "sanitizes error message with connection string",
			errType:   "ConnectionError",
			code:      "DB_ERROR",
			message:   "failed to connect: postgres://user:pass@host:5432/db",
			retryable: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			event := &WideEvent{}
			event.SetError(tt.errType, tt.code, tt.message, tt.retryable)

			assert.True(t, event.ErrorOccurred)
			assert.Equal(t, tt.errType, event.ErrorType)
			assert.Equal(t, tt.code, event.ErrorCode)
			assert.Equal(t, tt.retryable, event.ErrorRetryable)
			// Message should be sanitized - not contain raw connection string
			if tt.name == "sanitizes error message with connection string" {
				assert.NotContains(t, event.ErrorMessage, "user:pass")
				assert.Contains(t, event.ErrorMessage, "[REDACTED]")
			}
		})
	}
}

func TestWideEvent_SetDBMetrics(t *testing.T) {
	event := &WideEvent{}
	event.SetDBMetrics(10, 150.5)

	assert.Equal(t, 10, event.DBQueryCount)
	assert.Equal(t, 150.5, event.DBQueryTimeMS)
}

func TestWideEvent_IncrementDBMetrics(t *testing.T) {
	event := &WideEvent{
		DBQueryCount:  5,
		DBQueryTimeMS: 100.0,
	}

	event.IncrementDBMetrics(3, 50.5)

	assert.Equal(t, 8, event.DBQueryCount)
	assert.Equal(t, 150.5, event.DBQueryTimeMS)
}

func TestWideEvent_SetCacheMetrics(t *testing.T) {
	event := &WideEvent{}
	event.SetCacheMetrics(10, 2)

	assert.Equal(t, 10, event.CacheHits)
	assert.Equal(t, 2, event.CacheMisses)
}

func TestWideEvent_IncrementCacheHit(t *testing.T) {
	event := &WideEvent{CacheHits: 5}
	event.IncrementCacheHit()
	assert.Equal(t, 6, event.CacheHits)
}

func TestWideEvent_IncrementCacheMiss(t *testing.T) {
	event := &WideEvent{CacheMisses: 3}
	event.IncrementCacheMiss()
	assert.Equal(t, 4, event.CacheMisses)
}

func TestWideEvent_SetExternalCallMetrics(t *testing.T) {
	event := &WideEvent{}
	event.SetExternalCallMetrics(5, 200.0)

	assert.Equal(t, 5, event.ExternalCallCount)
	assert.Equal(t, 200.0, event.ExternalCallTimeMS)
}

func TestWideEvent_IncrementExternalCallMetrics(t *testing.T) {
	event := &WideEvent{
		ExternalCallCount:  2,
		ExternalCallTimeMS: 100.0,
	}

	event.IncrementExternalCallMetrics(1, 50.0)

	assert.Equal(t, 3, event.ExternalCallCount)
	assert.Equal(t, 150.0, event.ExternalCallTimeMS)
}

func TestWideEvent_SetIdempotency(t *testing.T) {
	t.Run("sets idempotency with hashed key", func(t *testing.T) {
		event := &WideEvent{}
		event.SetIdempotency("idem-key-123", true)

		// Key should be hashed (hex string), not raw
		assert.NotEqual(t, "idem-key-123", event.IdempotencyKey)
		assert.NotEmpty(t, event.IdempotencyKey)
		assert.True(t, event.IdempotencyHit)
		// Verify it's a valid hex string
		for _, c := range event.IdempotencyKey {
			assert.True(t, (c >= '0' && c <= '9') || (c >= 'a' && c <= 'f'),
				"Expected hex character in hash, got %c", c)
		}
	})

	t.Run("consistent hashing - same input produces same hash", func(t *testing.T) {
		event1 := &WideEvent{}
		event2 := &WideEvent{}

		event1.SetIdempotency("same-key", false)
		event2.SetIdempotency("same-key", true)

		assert.Equal(t, event1.IdempotencyKey, event2.IdempotencyKey)
	})

	t.Run("different keys produce different hashes", func(t *testing.T) {
		event1 := &WideEvent{}
		event2 := &WideEvent{}

		event1.SetIdempotency("key-1", false)
		event2.SetIdempotency("key-2", false)

		assert.NotEqual(t, event1.IdempotencyKey, event2.IdempotencyKey)
	})

	t.Run("handles empty key", func(t *testing.T) {
		event := &WideEvent{}
		event.SetIdempotency("", false)
		assert.Empty(t, event.IdempotencyKey)
	})
}

// =============================================================================
// SetCustom with Bounds Checking Tests
// =============================================================================

func TestWideEvent_SetCustom(t *testing.T) {
	t.Run("sets custom field on nil map", func(t *testing.T) {
		event := &WideEvent{} // Custom is nil
		event.SetCustom("key", "value")

		require.NotNil(t, event.Custom)
		assert.Equal(t, "value", event.Custom["key"])
	})

	t.Run("sets multiple custom fields", func(t *testing.T) {
		event := &WideEvent{Custom: make(map[string]any)}
		event.SetCustom("string_field", "string_value")
		event.SetCustom("int_field", 42)
		event.SetCustom("bool_field", true)
		event.SetCustom("float_field", 3.14)

		assert.Equal(t, "string_value", event.Custom["string_field"])
		assert.Equal(t, 42, event.Custom["int_field"])
		assert.Equal(t, true, event.Custom["bool_field"])
		assert.Equal(t, 3.14, event.Custom["float_field"])
	})

	t.Run("respects max keys limit", func(t *testing.T) {
		event := &WideEvent{}

		// Add maxCustomKeys (50) keys
		for i := 0; i < maxCustomKeys+10; i++ {
			event.SetCustom(fmt.Sprintf("key_%d", i), "value")
		}

		// Should have at most maxCustomKeys regular keys + overflow indicator
		assert.LessOrEqual(t, len(event.Custom), maxCustomKeys+1)
		// Should have overflow indicator
		assert.True(t, event.Custom["_overflow"].(bool))
	})

	t.Run("truncates long keys", func(t *testing.T) {
		event := &WideEvent{}
		longKey := make([]byte, 100)
		for i := range longKey {
			longKey[i] = 'a'
		}
		event.SetCustom(string(longKey), "value")

		// Check that no key exceeds maxCustomKeyLen
		for k := range event.Custom {
			assert.LessOrEqual(t, len(k), maxCustomKeyLen)
		}
	})

	t.Run("truncates long string values", func(t *testing.T) {
		event := &WideEvent{}
		longValue := make([]byte, 2000)
		for i := range longValue {
			longValue[i] = 'b'
		}
		event.SetCustom("key", string(longValue))

		if strVal, ok := event.Custom["key"].(string); ok {
			assert.LessOrEqual(t, len(strVal), maxCustomValueLen)
		} else {
			t.Fatal("expected string value")
		}
	})

	t.Run("does not truncate non-string values", func(t *testing.T) {
		event := &WideEvent{}
		event.SetCustom("number", 123456789)
		event.SetCustom("struct", struct{ Name string }{"test"})

		assert.Equal(t, 123456789, event.Custom["number"])
	})
}

// =============================================================================
// SetResponse / Complete Tests - Outcome Classification
// =============================================================================

func TestWideEvent_SetResponse_OutcomeClassification(t *testing.T) {
	tests := []struct {
		name           string
		statusCode     int
		expectedResult string
	}{
		// Success outcomes (2xx)
		{name: "200 OK is success", statusCode: 200, expectedResult: "success"},
		{name: "201 Created is success", statusCode: 201, expectedResult: "success"},
		{name: "204 No Content is success", statusCode: 204, expectedResult: "success"},

		// Redirect outcomes (3xx)
		{name: "301 Redirect is redirect", statusCode: 301, expectedResult: "redirect"},
		{name: "304 Not Modified is redirect", statusCode: 304, expectedResult: "redirect"},

		// Client error outcomes (4xx)
		{name: "400 Bad Request is client_error", statusCode: 400, expectedResult: "client_error"},
		{name: "401 Unauthorized is client_error", statusCode: 401, expectedResult: "client_error"},
		{name: "403 Forbidden is client_error", statusCode: 403, expectedResult: "client_error"},
		{name: "404 Not Found is client_error", statusCode: 404, expectedResult: "client_error"},
		{name: "422 Unprocessable Entity is client_error", statusCode: 422, expectedResult: "client_error"},
		{name: "429 Too Many Requests is client_error", statusCode: 429, expectedResult: "client_error"},
		{name: "499 Client Closed is client_error", statusCode: 499, expectedResult: "client_error"},

		// Server error outcomes (5xx)
		{name: "500 Internal Server Error is server_error", statusCode: 500, expectedResult: "server_error"},
		{name: "502 Bad Gateway is server_error", statusCode: 502, expectedResult: "server_error"},
		{name: "503 Service Unavailable is server_error", statusCode: 503, expectedResult: "server_error"},
		{name: "504 Gateway Timeout is server_error", statusCode: 504, expectedResult: "server_error"},

		// Edge cases
		{name: "100 Continue is unknown", statusCode: 100, expectedResult: "unknown"},
		{name: "0 is unknown", statusCode: 0, expectedResult: "unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			event := &WideEvent{StartTime: time.Now()}
			event.SetResponse(tt.statusCode, 1024)

			assert.Equal(t, tt.expectedResult, event.Outcome)
			assert.Equal(t, tt.statusCode, event.StatusCode)
			assert.Equal(t, int64(1024), event.ResponseSize)
		})
	}
}

func TestWideEvent_SetResponse_CalculatesDuration(t *testing.T) {
	startTime := time.Now().Add(-100 * time.Millisecond)
	event := &WideEvent{StartTime: startTime}

	event.SetResponse(200, 512)

	assert.GreaterOrEqual(t, event.DurationMS, float64(100))
}

func TestWideEvent_SetResponse_PreservesPanicOutcome(t *testing.T) {
	event := &WideEvent{StartTime: time.Now()}

	// Set panic first
	event.SetPanic("nil pointer dereference")
	assert.Equal(t, "panic", event.Outcome)

	// SetResponse should NOT overwrite panic outcome
	event.SetResponse(500, 0)
	assert.Equal(t, "panic", event.Outcome)
	assert.Equal(t, 500, event.StatusCode)
}

// =============================================================================
// SetPanic Tests
// =============================================================================

func TestWideEvent_SetPanic(t *testing.T) {
	t.Run("sets panic outcome and error fields", func(t *testing.T) {
		event := &WideEvent{}
		event.SetPanic("runtime error: nil pointer dereference")

		assert.Equal(t, "panic", event.Outcome)
		assert.True(t, event.ErrorOccurred)
		assert.Equal(t, "panic", event.ErrorType)
		assert.Contains(t, event.ErrorMessage, "nil pointer dereference")
	})

	t.Run("sanitizes file paths in panic message", func(t *testing.T) {
		event := &WideEvent{}
		event.SetPanic("panic at /home/user/app/internal/handler.go:42: something failed")

		assert.NotContains(t, event.ErrorMessage, "/home/user")
		assert.Contains(t, event.ErrorMessage, "[REDACTED]")
	})

	t.Run("sanitizes connection strings in panic message", func(t *testing.T) {
		event := &WideEvent{}
		event.SetPanic("database error: postgres://admin:secret@localhost:5432/db failed")

		assert.NotContains(t, event.ErrorMessage, "admin:secret")
		assert.Contains(t, event.ErrorMessage, "[REDACTED]")
	})
}

// =============================================================================
// toFieldsLocked Tests
// =============================================================================

func TestWideEvent_toFieldsLocked_MinimalEvent(t *testing.T) {
	event := &WideEvent{
		Method:     "GET",
		Path:       "/test",
		StatusCode: 200,
		DurationMS: 50.5,
		Outcome:    "success",
		Service:    "test-service",
	}

	event.mu.RLock()
	fields := event.toFieldsLocked()
	event.mu.RUnlock()

	// Convert to map for easier assertions
	fieldMap := fieldsToMap(fields)

	assert.Equal(t, "GET", fieldMap["method"])
	assert.Equal(t, "/test", fieldMap["path"])
	assert.Equal(t, 200, fieldMap["status_code"])
	assert.Equal(t, 50.5, fieldMap["duration_ms"])
	assert.Equal(t, "success", fieldMap["outcome"])
	assert.Equal(t, "test-service", fieldMap["service"])
}

func TestWideEvent_toFieldsLocked_FullEvent(t *testing.T) {
	event := &WideEvent{
		RequestID:           "req-123",
		TraceID:             "trace-456",
		SpanID:              "span-789",
		Method:              "POST",
		Path:                "/v1/transactions",
		Route:               "/v1/organizations/:org_id/ledgers/:ledger_id/transactions",
		StatusCode:          201,
		DurationMS:          150.0,
		Outcome:             "success",
		Service:             "transaction",
		Version:             "v1.0.0",
		Environment:         "production",
		OrganizationID:      "org-001",
		LedgerID:            "ledger-001",
		TransactionID:       "txn-001",
		TransactionType:     "json",
		TransactionAmount:   "1000.00",
		TransactionCurrency: "USD",
		OperationCount:      3,
		SourceCount:         2,
		DestinationCount:    1,
		DBQueryCount:        5,
		DBQueryTimeMS:       45.0,
		CacheHits:           2,
		CacheMisses:         1,
		Custom:              map[string]any{"handler": "create_transaction"},
	}

	event.mu.RLock()
	fields := event.toFieldsLocked()
	event.mu.RUnlock()

	fieldMap := fieldsToMap(fields)

	// Verify all fields are present
	assert.Equal(t, "req-123", fieldMap["request_id"])
	assert.Equal(t, "trace-456", fieldMap["trace_id"])
	assert.Equal(t, "span-789", fieldMap["span_id"])
	assert.Equal(t, "org-001", fieldMap["organization_id"])
	assert.Equal(t, "ledger-001", fieldMap["ledger_id"])
	assert.Equal(t, "txn-001", fieldMap["transaction_id"])
	assert.Equal(t, "json", fieldMap["transaction_type"])
	assert.Equal(t, "1000.00", fieldMap["transaction_amount"])
	assert.Equal(t, "USD", fieldMap["transaction_currency"])
	assert.Equal(t, 3, fieldMap["operation_count"])
	assert.Equal(t, 2, fieldMap["source_count"])
	assert.Equal(t, 1, fieldMap["destination_count"])
	assert.Equal(t, 5, fieldMap["db_query_count"])
	assert.Equal(t, 45.0, fieldMap["db_query_time_ms"])
	assert.Equal(t, 2, fieldMap["cache_hits"])
	assert.Equal(t, 1, fieldMap["cache_misses"])
	assert.NotNil(t, fieldMap["custom"])
}

func TestWideEvent_toFieldsLocked_ErrorEvent(t *testing.T) {
	event := &WideEvent{
		Method:         "POST",
		Path:           "/v1/transactions",
		StatusCode:     400,
		DurationMS:     25.0,
		Outcome:        "client_error",
		Service:        "transaction",
		ErrorOccurred:  true,
		ErrorType:      "ValidationError",
		ErrorCode:      "INVALID_AMOUNT",
		ErrorMessage:   "Amount must be positive",
		ErrorRetryable: false,
	}

	event.mu.RLock()
	fields := event.toFieldsLocked()
	event.mu.RUnlock()

	fieldMap := fieldsToMap(fields)

	assert.Equal(t, true, fieldMap["error_occurred"])
	assert.Equal(t, "ValidationError", fieldMap["error_type"])
	assert.Equal(t, "INVALID_AMOUNT", fieldMap["error_code"])
	assert.Equal(t, "Amount must be positive", fieldMap["error_message"])
	assert.Equal(t, false, fieldMap["error_retryable"])
}

func TestWideEvent_toFieldsLocked_OmitsEmptyFields(t *testing.T) {
	event := &WideEvent{
		Method:     "GET",
		Path:       "/test",
		StatusCode: 200,
		// All other fields are empty/zero
	}

	event.mu.RLock()
	fields := event.toFieldsLocked()
	event.mu.RUnlock()

	fieldMap := fieldsToMap(fields)

	// These fields should NOT be present when empty
	_, hasOrgID := fieldMap["organization_id"]
	_, hasLedgerID := fieldMap["ledger_id"]
	_, hasDBCount := fieldMap["db_query_count"]
	_, hasErrorOccurred := fieldMap["error_occurred"]

	assert.False(t, hasOrgID, "organization_id should not be present when empty")
	assert.False(t, hasLedgerID, "ledger_id should not be present when empty")
	assert.False(t, hasDBCount, "db_query_count should not be present when 0")
	assert.False(t, hasErrorOccurred, "error_occurred should not be present when false")

	// status_code should always be present
	_, hasStatusCode := fieldMap["status_code"]
	assert.True(t, hasStatusCode, "status_code should always be present")
}

func TestWideEvent_toFieldsLocked_IdempotencyFields(t *testing.T) {
	event := &WideEvent{
		Method:         "POST",
		Path:           "/test",
		StatusCode:     200,
		IdempotencyKey: "hashed_key_abc",
		IdempotencyHit: true,
	}

	event.mu.RLock()
	fields := event.toFieldsLocked()
	event.mu.RUnlock()

	fieldMap := fieldsToMap(fields)

	assert.Equal(t, "hashed_key_abc", fieldMap["idempotency_key"])
	assert.Equal(t, true, fieldMap["idempotency_hit"])
}

// =============================================================================
// Concurrent Access Tests
// =============================================================================

func TestWideEvent_ConcurrentAccess(t *testing.T) {
	event := &WideEvent{
		Custom:    make(map[string]any),
		StartTime: time.Now(),
	}

	var wg sync.WaitGroup
	numGoroutines := 100

	// Launch multiple goroutines that all write to the event
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()

			// Perform various operations
			event.SetOrganization("org-" + string(rune('0'+id%10)))
			event.SetLedger("ledger-" + string(rune('0'+id%10)))
			event.SetTransaction("txn-"+string(rune('0'+id%10)), "json", "100.00", "USD")
			event.SetAccount("account-" + string(rune('0'+id%10)))
			event.SetUser("user-"+string(rune('0'+id%10)), "role", "jwt")
			event.SetError("TestError", "CODE", "message", false)
			event.SetCustom("key_"+string(rune('0'+id%10)), id)
			event.IncrementDBMetrics(1, 10.0)
			event.IncrementCacheHit()
			event.IncrementCacheMiss()
			event.IncrementExternalCallMetrics(1, 5.0)

			// Also test SetResponse
			event.SetResponse(200, 100)
		}(i)
	}

	wg.Wait()

	// If we got here without a race condition panic, the test passes
	// The race detector (-race flag) will catch actual data races
	assert.NotEmpty(t, event.OrganizationID)
	assert.NotEmpty(t, event.LedgerID)
	assert.Greater(t, event.DBQueryCount, 0)
	assert.Greater(t, event.CacheHits, 0)
}

// =============================================================================
// Emit Tests with Mock Logger
// =============================================================================

// mockLogger implements libLog.Logger for testing
type mockLogger struct {
	mu          sync.Mutex
	infoCalled  bool
	warnCalled  bool
	errorCalled bool
	fields      []any
	message     string
}

// Verify mockLogger implements libLog.Logger at compile time
var _ libLog.Logger = (*mockLogger)(nil)

func (m *mockLogger) Info(args ...any) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.infoCalled = true
	if len(args) > 0 {
		if msg, ok := args[0].(string); ok {
			m.message = msg
		}
	}
}

func (m *mockLogger) Infof(format string, args ...any)  {}
func (m *mockLogger) Infoln(args ...any)                {}
func (m *mockLogger) Error(args ...any)                 { m.mu.Lock(); m.errorCalled = true; m.mu.Unlock() }
func (m *mockLogger) Errorf(format string, args ...any) {}
func (m *mockLogger) Errorln(args ...any)               {}
func (m *mockLogger) Warn(args ...any)                  { m.mu.Lock(); m.warnCalled = true; m.mu.Unlock() }
func (m *mockLogger) Warnf(format string, args ...any)  {}
func (m *mockLogger) Warnln(args ...any)                {}
func (m *mockLogger) Debug(args ...any)                 {}
func (m *mockLogger) Debugf(format string, args ...any) {}
func (m *mockLogger) Debugln(args ...any)               {}
func (m *mockLogger) Fatal(args ...any)                 {}
func (m *mockLogger) Fatalf(format string, args ...any) {}
func (m *mockLogger) Fatalln(args ...any)               {}

func (m *mockLogger) WithFields(fields ...any) libLog.Logger {
	m.mu.Lock()
	m.fields = fields
	m.mu.Unlock()
	return m
}

func (m *mockLogger) WithDefaultMessageTemplate(message string) libLog.Logger { return m }
func (m *mockLogger) Sync() error                                             { return nil }

// countingLogger counts the number of times each log method is called
type countingLogger struct {
	mockLogger
	infoCallCount int
}

func (c *countingLogger) Info(args ...any) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.infoCallCount++
	c.infoCalled = true
}

func (c *countingLogger) getInfoCallCount() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.infoCallCount
}

func (c *countingLogger) WithFields(fields ...any) libLog.Logger {
	c.mu.Lock()
	c.fields = fields
	c.mu.Unlock()
	return c
}

func TestWideEvent_Emit(t *testing.T) {
	t.Run("emits success event with Info level", func(t *testing.T) {
		mockLog := &mockLogger{}
		event := &WideEvent{
			Method:     "GET",
			Path:       "/test",
			StatusCode: 200,
			DurationMS: 50.0,
			Outcome:    "success",
			Service:    "test-service",
			StartTime:  time.Now(),
		}

		event.Emit(mockLog)

		assert.True(t, mockLog.infoCalled)
		assert.False(t, mockLog.warnCalled)
		assert.False(t, mockLog.errorCalled)
		assert.NotEmpty(t, mockLog.fields)
	})

	t.Run("emits client_error event with Warn level", func(t *testing.T) {
		mockLog := &mockLogger{}
		event := &WideEvent{
			Method:     "POST",
			Path:       "/test",
			StatusCode: 400,
			DurationMS: 25.0,
			Outcome:    "client_error",
			Service:    "test-service",
			StartTime:  time.Now(),
		}

		event.Emit(mockLog)

		assert.True(t, mockLog.warnCalled)
		assert.False(t, mockLog.infoCalled)
		assert.False(t, mockLog.errorCalled)
	})

	t.Run("emits server_error event with Error level", func(t *testing.T) {
		mockLog := &mockLogger{}
		event := &WideEvent{
			Method:     "POST",
			Path:       "/test",
			StatusCode: 500,
			DurationMS: 100.0,
			Outcome:    "server_error",
			Service:    "test-service",
			StartTime:  time.Now(),
		}

		event.Emit(mockLog)

		assert.True(t, mockLog.errorCalled)
		assert.False(t, mockLog.infoCalled)
		assert.False(t, mockLog.warnCalled)
	})

	t.Run("emits panic event with Error level", func(t *testing.T) {
		mockLog := &mockLogger{}
		event := &WideEvent{
			Method:    "POST",
			Path:      "/test",
			Outcome:   "panic",
			Service:   "test-service",
			StartTime: time.Now(),
		}

		event.Emit(mockLog)

		assert.True(t, mockLog.errorCalled)
	})

	t.Run("handles nil event", func(t *testing.T) {
		mockLog := &mockLogger{}
		var event *WideEvent = nil

		// Should not panic
		event.Emit(mockLog)

		assert.False(t, mockLog.infoCalled)
	})

	t.Run("handles nil logger", func(t *testing.T) {
		event := &WideEvent{
			Method:    "GET",
			Path:      "/test",
			StartTime: time.Now(),
		}

		// Should not panic
		event.Emit(nil)
	})

	t.Run("calculates duration if not set", func(t *testing.T) {
		mockLog := &mockLogger{}
		event := &WideEvent{
			Method:    "GET",
			Path:      "/test",
			Outcome:   "success",
			StartTime: time.Now().Add(-50 * time.Millisecond),
		}

		event.Emit(mockLog)

		assert.GreaterOrEqual(t, event.DurationMS, float64(50))
	})
}

// =============================================================================
// Double-Emission Guard Tests
// =============================================================================

func TestWideEvent_Emit_DoubleEmissionGuard(t *testing.T) {
	t.Run("prevents double emission", func(t *testing.T) {
		mockLog := &countingLogger{}

		event := &WideEvent{
			Method:    "GET",
			Path:      "/test",
			Outcome:   "success",
			StartTime: time.Now(),
		}

		// Emit twice
		event.Emit(mockLog)
		event.Emit(mockLog)

		// Info should only be called once
		assert.True(t, event.emitted)
		assert.Equal(t, 1, mockLog.getInfoCallCount())
	})

	t.Run("concurrent emission calls are safe", func(t *testing.T) {
		mockLog := &mockLogger{}
		event := &WideEvent{
			Method:    "GET",
			Path:      "/test",
			Outcome:   "success",
			StartTime: time.Now(),
		}

		var wg sync.WaitGroup
		for i := 0; i < 10; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				event.Emit(mockLog)
			}()
		}

		wg.Wait()

		// Should only be emitted once
		assert.True(t, event.emitted)
	})
}

// =============================================================================
// Finalize Tests
// =============================================================================

func TestWideEvent_Finalize(t *testing.T) {
	t.Run("calculates duration if not set", func(t *testing.T) {
		event := &WideEvent{
			StartTime: time.Now().Add(-100 * time.Millisecond),
		}

		event.Finalize()

		assert.GreaterOrEqual(t, event.DurationMS, float64(100))
	})

	t.Run("does not overwrite existing duration", func(t *testing.T) {
		event := &WideEvent{
			StartTime:  time.Now().Add(-100 * time.Millisecond),
			DurationMS: 50.0, // Already set
		}

		event.Finalize()

		assert.Equal(t, 50.0, event.DurationMS)
	})
}

// =============================================================================
// Helper Functions
// =============================================================================

// fieldsToMap converts a slice of key-value pairs to a map for easier assertions
func fieldsToMap(fields []any) map[string]any {
	result := make(map[string]any)
	for i := 0; i < len(fields); i += 2 {
		if key, ok := fields[i].(string); ok {
			result[key] = fields[i+1]
		}
	}
	return result
}
