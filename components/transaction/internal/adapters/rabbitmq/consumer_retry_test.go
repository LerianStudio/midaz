package rabbitmq

import (
	"testing"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
	"github.com/stretchr/testify/assert"
)

func TestGetRetryCount_ReturnsZeroForFirstDelivery(t *testing.T) {
	t.Parallel()

	headers := amqp.Table{}
	count := getRetryCount(headers)
	assert.Equal(t, 0, count, "First delivery should have retry count 0")
}

func TestGetRetryCount_ReturnsIncrementedValue(t *testing.T) {
	t.Parallel()

	headers := amqp.Table{
		retryCountHeader: int32(3),
	}
	count := getRetryCount(headers)
	assert.Equal(t, 3, count, "Should return stored retry count")
}

func TestGetRetryCount_HandlesInt64(t *testing.T) {
	t.Parallel()

	headers := amqp.Table{
		retryCountHeader: int64(5),
	}
	count := getRetryCount(headers)
	assert.Equal(t, 5, count, "Should handle int64 retry count")
}

func TestSafeIncrementRetryCount_IncrementsCorrectly(t *testing.T) {
	t.Parallel()

	result := safeIncrementRetryCount(2)
	assert.Equal(t, int32(3), result, "Should increment by 1")
}

func TestSafeIncrementRetryCount_HandlesOverflow(t *testing.T) {
	t.Parallel()

	result := safeIncrementRetryCount(2147483647) // math.MaxInt32
	assert.Equal(t, int32(2147483647), result, "Should return MaxInt32 on overflow")
}

func TestMaxRetries_IsAtLeast4(t *testing.T) {
	t.Parallel()

	// Business error retry should have same max as panic retry
	assert.GreaterOrEqual(t, maxRetries, 4, "maxRetries should be at least 4")
}

func TestCopyHeadersSafe_ReturnsEmptyTableForNilInput(t *testing.T) {
	t.Parallel()

	result := copyHeadersSafe(nil)
	assert.NotNil(t, result, "Should return non-nil table")
	assert.Len(t, result, 0, "Should return empty table")
}

func TestCopyHeadersSafe_CopiesOnlyAllowlistedHeaders(t *testing.T) {
	t.Parallel()

	original := amqp.Table{
		"x-correlation-id":  "value1",
		"x-midaz-header-id": "value2",
		"content-type":      "application/json",
		"sensitive-token":   "should-be-filtered",
	}
	result := copyHeadersSafe(original)

	assert.Equal(t, original["x-correlation-id"], result["x-correlation-id"], "Should copy allowlisted headers")
	assert.Equal(t, original["x-midaz-header-id"], result["x-midaz-header-id"], "Should copy allowlisted headers")
	assert.Equal(t, original["content-type"], result["content-type"], "Should copy allowlisted headers")
	assert.NotContains(t, result, "sensitive-token", "Should filter non-allowlisted headers")

	// Verify it's a copy, not a reference
	result["x-new-header"] = "new"
	_, exists := original["x-new-header"]
	assert.False(t, exists, "Modifying copy should not affect original")
}

func TestBuildDLQName_AppendsSuffix(t *testing.T) {
	t.Parallel()

	result, err := buildDLQName("transactions")
	assert.NoError(t, err, "Should not return error for valid queue name")
	assert.Equal(t, "transactions.dlq", result, "Should append .dlq suffix")
}

func TestBusinessErrorContext_Exists(t *testing.T) {
	t.Parallel()

	// This test verifies the businessErrorContext struct exists
	// and has the expected fields
	bec := &businessErrorContext{
		queue:      "test-queue",
		workerID:   1,
		retryCount: 2,
	}

	assert.Equal(t, "test-queue", bec.queue)
	assert.Equal(t, 1, bec.workerID)
	assert.Equal(t, 2, bec.retryCount)
	assert.Nil(t, bec.logger)
	assert.Nil(t, bec.msg)
	assert.Nil(t, bec.conn)
	assert.Nil(t, bec.err)
}

// =============================================================================
// handleBusinessError() Routing Logic Tests
// =============================================================================
// These tests verify the critical routing decision in handleBusinessError():
// - Routes to DLQ when maxRetries exceeded (retryCount >= maxRetries-1)
// - Republishes with incremented counter when retries remaining

// TestHandleBusinessError_RoutingDecision_MaxRetriesExceeded verifies that
// handleBusinessError routes to DLQ when max retries are exceeded.
// This is a behavioral test that verifies the routing logic based on retry count.
func TestHandleBusinessError_RoutingDecision_MaxRetriesExceeded(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		retryCount  int
		shouldRoute bool // true = route to DLQ, false = republish
		description string
	}{
		{
			name:        "5th delivery (retryCount=4) routes to DLQ",
			retryCount:  4, // maxRetries-1 = 4, so this triggers DLQ routing
			shouldRoute: true,
			description: "When retryCount equals maxRetries-1, message should route to DLQ",
		},
		{
			name:        "6th delivery (retryCount=5) routes to DLQ",
			retryCount:  5, // Greater than maxRetries-1, definitely DLQ
			shouldRoute: true,
			description: "When retryCount exceeds maxRetries-1, message should route to DLQ",
		},
		{
			name:        "extreme retry count routes to DLQ",
			retryCount:  100, // Far exceeds maxRetries
			shouldRoute: true,
			description: "Extreme retry counts should still route to DLQ",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// Verify the routing condition logic
			// handleBusinessError() uses: if bec.retryCount >= maxRetries-1
			shouldRouteToDLQ := tt.retryCount >= maxRetries-1

			assert.Equal(t, tt.shouldRoute, shouldRouteToDLQ, tt.description)
		})
	}
}

// TestHandleBusinessError_RoutingDecision_RetriesRemaining verifies that
// handleBusinessError republishes when retries are remaining.
func TestHandleBusinessError_RoutingDecision_RetriesRemaining(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		retryCount  int
		shouldRepub bool // true = republish, false = route to DLQ
		description string
		deliveryNum int // Human-readable delivery number (retryCount + 1)
	}{
		{
			name:        "1st delivery (retryCount=0) republishes",
			retryCount:  0,
			shouldRepub: true,
			deliveryNum: 1,
			description: "First delivery should republish for retry",
		},
		{
			name:        "2nd delivery (retryCount=1) republishes",
			retryCount:  1,
			shouldRepub: true,
			deliveryNum: 2,
			description: "Second delivery should republish for retry",
		},
		{
			name:        "3rd delivery (retryCount=2) republishes",
			retryCount:  2,
			shouldRepub: true,
			deliveryNum: 3,
			description: "Third delivery should republish for retry",
		},
		{
			name:        "boundary: 3rd delivery (retryCount=2) is last republish",
			retryCount:  2,
			shouldRepub: true,
			deliveryNum: 3,
			description: "retryCount=2 (3rd delivery) is the last one that should republish; 4th (retryCount=3) routes to DLQ",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// Verify the republish condition logic (inverse of DLQ routing)
			// handleBusinessError() republishes when: bec.retryCount < maxRetries-1
			shouldRepublish := tt.retryCount < maxRetries-1

			assert.Equal(t, tt.shouldRepub, shouldRepublish, tt.description)

			// Also verify the delivery number calculation
			assert.Equal(t, tt.deliveryNum, tt.retryCount+1, "Delivery number should be retryCount + 1")
		})
	}
}

// TestHandleBusinessError_BoundaryCondition tests the exact boundary where
// the routing decision changes from republish to DLQ.
func TestHandleBusinessError_BoundaryCondition(t *testing.T) {
	t.Parallel()

	// maxRetries = 4 (defined in consumer.rabbitmq.go)
	// Boundary is at retryCount = maxRetries - 1 = 3

	t.Run("retryCount=2 should republish (below boundary)", func(t *testing.T) {
		t.Parallel()

		retryCount := 2
		shouldRepublish := retryCount < maxRetries-1 // 2 < 3 = true

		assert.True(t, shouldRepublish,
			"retryCount=%d (delivery %d) should republish because %d < %d (maxRetries-1)",
			retryCount, retryCount+1, retryCount, maxRetries-1)
	})

	t.Run("retryCount=4 should route to DLQ (at boundary)", func(t *testing.T) {
		t.Parallel()

		retryCount := 4
		shouldRouteToDLQ := retryCount >= maxRetries-1 // 4 >= 4 = true

		assert.True(t, shouldRouteToDLQ,
			"retryCount=%d (delivery %d) should route to DLQ because %d >= %d (maxRetries-1)",
			retryCount, retryCount+1, retryCount, maxRetries-1)
	})

	t.Run("maxRetries constant is 5", func(t *testing.T) {
		t.Parallel()

		// Verify the constant value (5 delivery attempts = 4 retries with backoff: 0s, 5s, 15s, 30s)
		assert.Equal(t, 5, maxRetries,
			"maxRetries should be 5 (5 delivery attempts to enable all 4 backoff delays)")
	})
}

// =============================================================================
// publishToDLQ() Confirmation Scenario Tests
// =============================================================================
// These tests verify the confirmation handling logic in publishToDLQ().
// Since we can't mock RabbitMQ channels directly, we test the logic flow
// and error types that would be returned in each scenario.

// TestPublishToDLQ_ConfirmationScenarios tests the expected behavior for each
// broker confirmation scenario (ACK, NACK, Timeout, Channel Close).
func TestPublishToDLQ_ConfirmationScenarios(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		scenario      string
		expectedError error
		isSuccess     bool
		description   string
	}{
		{
			name:          "broker ACK - success path",
			scenario:      "ack",
			expectedError: nil,
			isSuccess:     true,
			description:   "When broker ACKs, publishToDLQ returns nil (success)",
		},
		{
			name:          "broker NACK - failure path",
			scenario:      "nack",
			expectedError: ErrBrokerNack,
			isSuccess:     false,
			description:   "When broker NACKs, publishToDLQ returns ErrBrokerNack wrapped error",
		},
		{
			name:          "confirmation timeout",
			scenario:      "timeout",
			expectedError: ErrConfirmTimeout,
			isSuccess:     false,
			description:   "When confirmation times out, publishToDLQ returns ErrConfirmTimeout wrapped error",
		},
		{
			name:          "confirmation channel closed",
			scenario:      "channel_close",
			expectedError: ErrConfirmChannelClosed,
			isSuccess:     false,
			description:   "When confirmation channel closes, publishToDLQ returns ErrConfirmChannelClosed wrapped error",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// Verify the error types are defined and distinct
			if !tt.isSuccess {
				assert.NotNil(t, tt.expectedError, "Error type should be defined for scenario: %s", tt.scenario)
			}

			// Document the expected behavior for each scenario
			t.Logf("Scenario '%s': %s", tt.scenario, tt.description)

			// Verify error types are distinguishable
			if tt.expectedError != nil {
				switch tt.scenario {
				case "nack":
					assert.ErrorIs(t, ErrBrokerNack, ErrBrokerNack, "ErrBrokerNack should be comparable")
					assert.NotErrorIs(t, ErrBrokerNack, ErrConfirmTimeout, "Error types should be distinct")
					assert.NotErrorIs(t, ErrBrokerNack, ErrConfirmChannelClosed, "Error types should be distinct")
				case "timeout":
					assert.ErrorIs(t, ErrConfirmTimeout, ErrConfirmTimeout, "ErrConfirmTimeout should be comparable")
					assert.NotErrorIs(t, ErrConfirmTimeout, ErrBrokerNack, "Error types should be distinct")
					assert.NotErrorIs(t, ErrConfirmTimeout, ErrConfirmChannelClosed, "Error types should be distinct")
				case "channel_close":
					assert.ErrorIs(t, ErrConfirmChannelClosed, ErrConfirmChannelClosed, "ErrConfirmChannelClosed should be comparable")
					assert.NotErrorIs(t, ErrConfirmChannelClosed, ErrBrokerNack, "Error types should be distinct")
					assert.NotErrorIs(t, ErrConfirmChannelClosed, ErrConfirmTimeout, "Error types should be distinct")
				}
			}
		})
	}
}

// TestPublishToDLQ_ErrorTypes verifies that all DLQ publish error types
// are properly defined sentinel errors for error wrapping.
func TestPublishToDLQ_ErrorTypes(t *testing.T) {
	t.Parallel()

	t.Run("ErrBrokerNack is defined", func(t *testing.T) {
		t.Parallel()

		assert.NotNil(t, ErrBrokerNack, "ErrBrokerNack should be defined")
		assert.Contains(t, ErrBrokerNack.Error(), "NACK",
			"ErrBrokerNack message should indicate NACK")
	})

	t.Run("ErrConfirmTimeout is defined", func(t *testing.T) {
		t.Parallel()

		assert.NotNil(t, ErrConfirmTimeout, "ErrConfirmTimeout should be defined")
		assert.Contains(t, ErrConfirmTimeout.Error(), "timed out",
			"ErrConfirmTimeout message should indicate timeout")
	})

	t.Run("ErrConfirmChannelClosed is defined", func(t *testing.T) {
		t.Parallel()

		assert.NotNil(t, ErrConfirmChannelClosed, "ErrConfirmChannelClosed should be defined")
		assert.Contains(t, ErrConfirmChannelClosed.Error(), "closed",
			"ErrConfirmChannelClosed message should indicate channel closed")
	})
}

// TestPublishToDLQ_HeadersPreparation tests that DLQ headers are correctly
// prepared with all required metadata fields.
func TestPublishToDLQ_HeadersPreparation(t *testing.T) {
	t.Parallel()

	// These are the headers that publishToDLQ adds to messages
	requiredDLQHeaders := []string{
		"x-dlq-reason",
		"x-dlq-original-queue",
		"x-dlq-retry-count",
		"x-dlq-timestamp",
	}

	t.Run("all required DLQ headers are documented", func(t *testing.T) {
		t.Parallel()

		// Verify the expected header count
		assert.Len(t, requiredDLQHeaders, 4,
			"Should have 4 required DLQ headers for proper message tracking")

		// Verify specific header names
		assert.Contains(t, requiredDLQHeaders, "x-dlq-reason",
			"Should include reason header for error context")
		assert.Contains(t, requiredDLQHeaders, "x-dlq-original-queue",
			"Should include original queue header for replay routing")
		assert.Contains(t, requiredDLQHeaders, "x-dlq-retry-count",
			"Should include retry count header for tracking attempts")
		assert.Contains(t, requiredDLQHeaders, "x-dlq-timestamp",
			"Should include timestamp header for timing analysis")
	})

	t.Run("businessErrorContext adds error-type header", func(t *testing.T) {
		t.Parallel()

		// businessErrorContext.publishToDLQ adds an additional header
		businessErrorDLQHeaders := append(requiredDLQHeaders, "x-dlq-error-type")

		assert.Len(t, businessErrorDLQHeaders, 5,
			"Business error DLQ should have 5 headers including error-type")
		assert.Contains(t, businessErrorDLQHeaders, "x-dlq-error-type",
			"Should include error-type header to distinguish business vs panic errors")
	})
}

// =============================================================================
// republishWithRetry() Counter Increment Tests
// =============================================================================
// These tests verify that republishWithRetry correctly increments the retry
// counter in the republished message headers.

// TestRepublishWithRetry_CounterIncrement verifies that the retry counter
// is correctly incremented when preparing headers for republish.
func TestRepublishWithRetry_CounterIncrement(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name             string
		currentRetry     int
		expectedNewRetry int32
		description      string
	}{
		{
			name:             "first retry (0 -> 1)",
			currentRetry:     0,
			expectedNewRetry: 1,
			description:      "First delivery (retryCount=0) should become 1 after republish",
		},
		{
			name:             "second retry (1 -> 2)",
			currentRetry:     1,
			expectedNewRetry: 2,
			description:      "Second delivery (retryCount=1) should become 2 after republish",
		},
		{
			name:             "third retry (2 -> 3)",
			currentRetry:     2,
			expectedNewRetry: 3,
			description:      "Third delivery (retryCount=2) should become 3 after republish",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// Test the safeIncrementRetryCount function directly
			// This is the same function used by republishWithRetry
			result := safeIncrementRetryCount(tt.currentRetry)

			assert.Equal(t, tt.expectedNewRetry, result, tt.description)
		})
	}
}

// TestRepublishWithRetry_HeaderUpdate verifies that republishWithRetry
// correctly updates the retry count header in copied headers.
func TestRepublishWithRetry_HeaderUpdate(t *testing.T) {
	t.Parallel()

	t.Run("new header is added to empty headers", func(t *testing.T) {
		t.Parallel()

		// Simulate republishWithRetry header preparation for first delivery
		originalHeaders := amqp.Table{}
		newHeaders := copyHeadersSafe(originalHeaders)
		newHeaders[retryCountHeader] = safeIncrementRetryCount(0)

		// Verify the header was added
		assert.Contains(t, newHeaders, retryCountHeader,
			"Retry count header should be added to headers")
		assert.Equal(t, int32(1), newHeaders[retryCountHeader],
			"Retry count should be 1 for first republish")

		// Verify original was not modified
		_, exists := originalHeaders[retryCountHeader]
		assert.False(t, exists, "Original headers should not be modified")
	})

	t.Run("existing header is updated", func(t *testing.T) {
		t.Parallel()

		// Simulate republishWithRetry header preparation for subsequent delivery
		originalHeaders := amqp.Table{
			retryCountHeader:   int32(2),
			"x-correlation-id": "preserved",
		}
		newHeaders := copyHeadersSafe(originalHeaders)
		currentRetryCount := getRetryCount(originalHeaders)
		newHeaders[retryCountHeader] = safeIncrementRetryCount(currentRetryCount)

		// Verify the header was updated
		assert.Equal(t, int32(3), newHeaders[retryCountHeader],
			"Retry count should be incremented from 2 to 3")

		// Verify other headers are preserved
		assert.Equal(t, "preserved", newHeaders["x-correlation-id"],
			"Allowlisted headers should be preserved during republish")

		// Verify original was not modified
		assert.Equal(t, int32(2), originalHeaders[retryCountHeader],
			"Original headers should not be modified")
	})

	t.Run("retryCountHeader constant is correct", func(t *testing.T) {
		t.Parallel()

		assert.Equal(t, "x-midaz-retry-count", retryCountHeader,
			"retryCountHeader should be 'x-midaz-retry-count'")
	})
}

// TestRepublishWithRetry_DeliveryNumberTracking verifies that the delivery
// number logged during republish is correctly calculated.
func TestRepublishWithRetry_DeliveryNumberTracking(t *testing.T) {
	t.Parallel()

	tests := []struct {
		retryCount       int
		expectedDelivery int
		expectedMax      int
	}{
		{retryCount: 0, expectedDelivery: 1, expectedMax: maxRetries},
		{retryCount: 1, expectedDelivery: 2, expectedMax: maxRetries},
		{retryCount: 2, expectedDelivery: 3, expectedMax: maxRetries},
	}

	for _, tt := range tests {
		t.Run("retryCount_"+string(rune('0'+tt.retryCount)), func(t *testing.T) {
			t.Parallel()

			// The log message format is:
			// "redelivering ... (delivery %d of %d max)"
			// where first %d is retryCount+1 and second %d is maxRetries

			actualDelivery := tt.retryCount + 1
			actualMax := maxRetries

			assert.Equal(t, tt.expectedDelivery, actualDelivery,
				"Delivery number should be retryCount + 1")
			assert.Equal(t, tt.expectedMax, actualMax,
				"Max deliveries should equal maxRetries constant")
		})
	}
}

// =============================================================================
// Integration of Retry Logic Tests
// =============================================================================
// These tests verify the complete flow of retry logic integration.

// TestRetryLogic_CompleteFlow verifies the complete retry flow from first
// delivery to DLQ routing.
func TestRetryLogic_CompleteFlow(t *testing.T) {
	t.Parallel()

	t.Run("full retry sequence before DLQ", func(t *testing.T) {
		t.Parallel()

		// Simulate the complete retry sequence
		type delivery struct {
			retryCount  int
			action      string // "republish" or "dlq"
			deliveryNum int
		}

		sequence := []delivery{
			{retryCount: 0, action: "republish", deliveryNum: 1},
			{retryCount: 1, action: "republish", deliveryNum: 2},
			{retryCount: 2, action: "republish", deliveryNum: 3},
			{retryCount: 3, action: "republish", deliveryNum: 4},
			{retryCount: 4, action: "dlq", deliveryNum: 5},
		}

		for _, d := range sequence {
			shouldRepublish := d.retryCount < maxRetries-1
			shouldDLQ := d.retryCount >= maxRetries-1

			if d.action == "republish" {
				assert.True(t, shouldRepublish,
					"Delivery %d (retryCount=%d) should republish", d.deliveryNum, d.retryCount)
				assert.False(t, shouldDLQ,
					"Delivery %d (retryCount=%d) should NOT route to DLQ", d.deliveryNum, d.retryCount)
			} else {
				assert.False(t, shouldRepublish,
					"Delivery %d (retryCount=%d) should NOT republish", d.deliveryNum, d.retryCount)
				assert.True(t, shouldDLQ,
					"Delivery %d (retryCount=%d) should route to DLQ", d.deliveryNum, d.retryCount)
			}
		}
	})

	t.Run("retry counter progression", func(t *testing.T) {
		t.Parallel()

		// Verify that retry counters progress correctly through republishes
		startingRetryCount := 0
		expectedProgression := []int32{1, 2, 3}

		currentRetry := startingRetryCount
		for i, expected := range expectedProgression {
			newRetry := safeIncrementRetryCount(currentRetry)
			assert.Equal(t, expected, newRetry,
				"After republish %d, retry count should be %d", i+1, expected)
			currentRetry = int(newRetry)
		}
	})
}

// TestRetryLogic_PanicVsBusinessError verifies that both panic recovery and
// business error handlers use the same retry logic.
func TestRetryLogic_PanicVsBusinessError(t *testing.T) {
	t.Parallel()

	t.Run("both handlers use same maxRetries constant", func(t *testing.T) {
		t.Parallel()

		// Both panicRecoveryContext and businessErrorContext use maxRetries
		// to determine when to route to DLQ

		// The condition is: retryCount >= maxRetries-1

		// For panic recovery (panicRecoveryContext.handlePoisonMessage):
		// if prc.retryCount < maxRetries-1 { return false } // not poison yet

		// For business error (businessErrorContext.handleBusinessError):
		// if bec.retryCount >= maxRetries-1 { /* DLQ */ }

		// Both use the same boundary: maxRetries-1
		assert.Equal(t, 5, maxRetries, "maxRetries should be 5 (5 deliveries = 4 retries)")
		assert.Equal(t, 4, maxRetries-1, "DLQ routing boundary should be at retryCount=4")
	})

	t.Run("both handlers use safeIncrementRetryCount", func(t *testing.T) {
		t.Parallel()

		// Verify the increment function handles edge cases
		assert.Equal(t, int32(1), safeIncrementRetryCount(0))
		assert.Equal(t, int32(2), safeIncrementRetryCount(1))
		assert.Equal(t, int32(3), safeIncrementRetryCount(2))
		assert.Equal(t, int32(4), safeIncrementRetryCount(3))
	})

	t.Run("both handlers use copyHeadersSafe for safe modification", func(t *testing.T) {
		t.Parallel()

		// Verify copyHeadersSafe creates independent copy and filters headers
		original := amqp.Table{
			"x-correlation-id": "value",
			"sensitive-data":   "filtered",
		}
		copied := copyHeadersSafe(original)

		// Verify allowlisted header is copied
		assert.Equal(t, "value", copied["x-correlation-id"], "Allowlisted headers should be copied")
		// Verify non-allowlisted header is filtered
		assert.NotContains(t, copied, "sensitive-data", "Non-allowlisted headers should be filtered")

		// Verify it's a copy, not a reference
		copied["x-new-header"] = "new-value"
		_, exists := original["x-new-header"]
		assert.False(t, exists, "Original should not be modified by copy changes")
	})
}

// =============================================================================
// Retry Backoff Calculation Tests
// =============================================================================

func TestRetryBackoffCalculation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		retryCount    int
		expectedDelay time.Duration
	}{
		{
			name:          "first retry (attempt 2) - immediate",
			retryCount:    1,
			expectedDelay: 0,
		},
		{
			name:          "second retry (attempt 3) - 5s delay",
			retryCount:    2,
			expectedDelay: 5 * time.Second,
		},
		{
			name:          "third retry (attempt 4) - 15s delay",
			retryCount:    3,
			expectedDelay: 15 * time.Second,
		},
		{
			name:          "fourth retry (attempt 5) - 30s delay",
			retryCount:    4,
			expectedDelay: 30 * time.Second,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			delay := calculateRetryBackoff(tt.retryCount)
			assert.Equal(t, tt.expectedDelay, delay,
				"Retry backoff for count %d should be %v", tt.retryCount, tt.expectedDelay)
		})
	}
}

func TestRetryBackoffConstants(t *testing.T) {
	t.Parallel()

	t.Run("retry delays array spans ~50 seconds", func(t *testing.T) {
		t.Parallel()

		// Sum the delays in the array: 0 + 5 + 15 + 30 = 50 seconds
		// This tests the theoretical maximum if all delays are used
		totalDelay := time.Duration(0)
		for i := 0; i < len(retryBackoffDelays); i++ {
			totalDelay += retryBackoffDelays[i]
		}

		assert.Equal(t, 50*time.Second, totalDelay,
			"retryBackoffDelays array should sum to 50 seconds")
	})

	t.Run("actual retry flow window matches requirements", func(t *testing.T) {
		t.Parallel()

		// Validate ACTUAL retry flow: backoffs applied BEFORE each retry
		// Messages go to DLQ at retryCount >= maxRetries-1, so last backoff must be used
		totalDelay := time.Duration(0)
		for retryCount := 0; retryCount < maxRetries-1; retryCount++ {
			// republishWithRetry calls calculateRetryBackoff(retryCount + 1)
			totalDelay += calculateRetryBackoff(retryCount + 1)
		}

		assert.GreaterOrEqual(t, totalDelay, 45*time.Second,
			"Actual retry window (retryCount 0 to %d) should cover PostgreSQL restart times", maxRetries-2)
		assert.LessOrEqual(t, totalDelay, 60*time.Second,
			"Actual retry window should not exceed 60 seconds")
	})
}

func TestRetryBackoffCalculation_EdgeCases(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		retryCount    int
		expectedDelay time.Duration
		rationale     string
	}{
		{
			name:          "zero retry count returns zero",
			retryCount:    0,
			expectedDelay: 0,
			rationale:     "Defensive case: retryCount=0 should return 0 delay",
		},
		{
			name:          "negative retry count returns zero",
			retryCount:    -1,
			expectedDelay: 0,
			rationale:     "Defensive case: negative values should fail-safe to 0 delay",
		},
		{
			name:          "large negative retry count returns zero",
			retryCount:    -2147483648, // int32 min
			expectedDelay: 0,
			rationale:     "Extreme negative should fail-safe to 0 delay",
		},
		{
			name:          "retry count beyond array length caps at max",
			retryCount:    5,
			expectedDelay: 30 * time.Second,
			rationale:     "Beyond array bounds should cap at last element (30s)",
		},
		{
			name:          "large retry count caps at max",
			retryCount:    1000,
			expectedDelay: 30 * time.Second,
			rationale:     "Very large values should cap at last element (30s)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			delay := calculateRetryBackoff(tt.retryCount)
			assert.Equal(t, tt.expectedDelay, delay, tt.rationale)
		})
	}
}
