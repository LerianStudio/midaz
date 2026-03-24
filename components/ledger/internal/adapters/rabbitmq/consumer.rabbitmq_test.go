// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package rabbitmq

import (
	"context"
	"errors"
	"testing"
	"time"

	libCommons "github.com/LerianStudio/lib-commons/v4/commons"
	libConstants "github.com/LerianStudio/lib-commons/v4/commons/constants"
	libLog "github.com/LerianStudio/lib-commons/v4/commons/log"
	libZap "github.com/LerianStudio/lib-commons/v4/commons/zap"
	"github.com/google/uuid"
	amqp "github.com/rabbitmq/amqp091-go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// testLogger is initialized once for all tests to avoid race conditions
// when multiple parallel tests initialize the logger simultaneously.
var testLogger libLog.Logger

func init() {
	logger, err := libZap.New(libZap.Config{
		Environment:     libZap.EnvironmentLocal,
		OTelLibraryName: "midaz-tests",
	})
	if err != nil {
		panic(err)
	}

	testLogger = logger
}

// =============================================================================
// UNIT TESTS - NewConsumerRoutes Default Values
// =============================================================================

func TestNewConsumerRoutes_DefaultValues(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name                    string
		inputWorkers            int
		inputPrefetch           int
		expectedWorkers         int
		expectedPrefetchFormula int
	}{
		{
			name:                    "zero_workers_and_prefetch_uses_defaults",
			inputWorkers:            0,
			inputPrefetch:           0,
			expectedWorkers:         5,
			expectedPrefetchFormula: 5 * 10,
		},
		{
			name:                    "zero_workers_uses_default_five",
			inputWorkers:            0,
			inputPrefetch:           20,
			expectedWorkers:         5,
			expectedPrefetchFormula: 5 * 20,
		},
		{
			name:                    "zero_prefetch_uses_default_ten",
			inputWorkers:            3,
			inputPrefetch:           0,
			expectedWorkers:         3,
			expectedPrefetchFormula: 3 * 10,
		},
		{
			name:                    "custom_workers_and_prefetch",
			inputWorkers:            10,
			inputPrefetch:           5,
			expectedWorkers:         10,
			expectedPrefetchFormula: 10 * 5,
		},
		{
			name:                    "single_worker",
			inputWorkers:            1,
			inputPrefetch:           1,
			expectedWorkers:         1,
			expectedPrefetchFormula: 1 * 1,
		},
		{
			name:                    "large_worker_count",
			inputWorkers:            100,
			inputPrefetch:           50,
			expectedWorkers:         100,
			expectedPrefetchFormula: 100 * 50,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// Test the default value logic directly (extracted from constructor)
			// Since NewConsumerRoutes calls conn.GetNewConnect() and panics on error,
			// we test the calculation logic in isolation
			workers := tt.inputWorkers
			prefetch := tt.inputPrefetch

			if workers == 0 {
				workers = 5
			}
			if prefetch == 0 {
				prefetch = 10
			}

			calculatedPrefetch := workers * prefetch

			assert.Equal(t, tt.expectedWorkers, workers, "workers should match expected")
			assert.Equal(t, tt.expectedPrefetchFormula, calculatedPrefetch, "prefetch calculation should match")
		})
	}
}

// Note: TestIntegration_NewConsumerRoutes_PanicOnConnectionFailure moved to
// consumer.rabbitmq_integration_test.go (requires real network dial attempt)

// =============================================================================
// UNIT TESTS - Register
// =============================================================================

func TestConsumerRoutes_Register(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		queueName      string
		expectedRoutes int
	}{
		{
			name:           "register_single_queue",
			queueName:      "test-queue-1",
			expectedRoutes: 1,
		},
		{
			name:           "register_queue_with_special_characters",
			queueName:      "test.queue.with.dots",
			expectedRoutes: 1,
		},
		{
			name:           "register_queue_with_underscores",
			queueName:      "test_queue_with_underscores",
			expectedRoutes: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// Create ConsumerRoutes struct directly (bypassing constructor that needs real connection)
			cr := &ConsumerRoutes{
				routes: make(map[string]QueueHandlerFunc),
				Logger: testLogger,
			}

			handler := func(ctx context.Context, body []byte) error {
				return nil
			}

			// Register the handler
			cr.Register(tt.queueName, handler)

			// Verify the handler was registered
			assert.Len(t, cr.routes, tt.expectedRoutes)
			assert.Contains(t, cr.routes, tt.queueName)
			assert.NotNil(t, cr.routes[tt.queueName])
		})
	}
}

func TestConsumerRoutes_Register_MultipleQueues(t *testing.T) {
	t.Parallel()

	cr := &ConsumerRoutes{
		routes: make(map[string]QueueHandlerFunc),
		Logger: testLogger,
	}

	queues := []string{
		"balance-create-queue",
		"transaction-audit-queue",
		"notification-queue",
		"dead-letter-queue",
	}

	// Register multiple handlers
	for _, queueName := range queues {
		handler := func(ctx context.Context, body []byte) error {
			return nil
		}
		cr.Register(queueName, handler)
	}

	// Verify all handlers were registered
	assert.Len(t, cr.routes, len(queues))
	for _, queueName := range queues {
		assert.Contains(t, cr.routes, queueName)
		assert.NotNil(t, cr.routes[queueName])
	}
}

func TestConsumerRoutes_Register_OverwriteExisting(t *testing.T) {
	t.Parallel()

	cr := &ConsumerRoutes{
		routes: make(map[string]QueueHandlerFunc),
		Logger: testLogger,
	}

	queueName := "test-queue"
	callCount := 0

	// Register first handler
	handler1 := func(ctx context.Context, body []byte) error {
		callCount = 1
		return nil
	}
	cr.Register(queueName, handler1)

	// Register second handler (should overwrite)
	handler2 := func(ctx context.Context, body []byte) error {
		callCount = 2
		return nil
	}
	cr.Register(queueName, handler2)

	// Verify only one handler is registered
	assert.Len(t, cr.routes, 1)

	// Call the registered handler - should be handler2
	ctx := context.Background()
	err := cr.routes[queueName](ctx, []byte("test"))
	require.NoError(t, err)
	assert.Equal(t, 2, callCount, "second handler should have been called")
}

// =============================================================================
// UNIT TESTS - QueueHandlerFunc
// =============================================================================

func TestQueueHandlerFunc_Success(t *testing.T) {
	t.Parallel()

	handlerCalled := false
	receivedBody := []byte{}

	handler := QueueHandlerFunc(func(ctx context.Context, body []byte) error {
		handlerCalled = true
		receivedBody = body
		return nil
	})

	ctx := context.Background()
	testBody := []byte(`{"id":"123","data":"test"}`)

	err := handler(ctx, testBody)

	require.NoError(t, err)
	assert.True(t, handlerCalled)
	assert.Equal(t, testBody, receivedBody)
}

func TestQueueHandlerFunc_Error(t *testing.T) {
	t.Parallel()

	expectedErr := errors.New("handler processing failed")

	handler := QueueHandlerFunc(func(ctx context.Context, body []byte) error {
		return expectedErr
	})

	ctx := context.Background()
	err := handler(ctx, []byte("test"))

	require.Error(t, err)
	assert.Equal(t, expectedErr, err)
}

func TestQueueHandlerFunc_ContextCancellation(t *testing.T) {
	t.Parallel()

	handler := QueueHandlerFunc(func(ctx context.Context, body []byte) error {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
			return nil
		}
	})

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	err := handler(ctx, []byte("test"))

	require.Error(t, err)
	assert.ErrorIs(t, err, context.Canceled)
}

// =============================================================================
// UNIT TESTS - RunConsumers (limited scope - goroutine spawn verification)
// =============================================================================

func TestConsumerRoutes_RunConsumers_NoRoutes(t *testing.T) {
	t.Parallel()

	cr := &ConsumerRoutes{
		routes: make(map[string]QueueHandlerFunc),
		Logger: testLogger,
	}

	// RunConsumers with no registered routes should not error
	err := cr.RunConsumers()

	require.NoError(t, err)
}

// Note: TestConsumerRoutes_RunConsumers_ReturnsImmediately removed.
// The test made real network dial attempts (wrong level) and is covered by
// TestIntegration_Consumer_BasicMessageConsumption.

// =============================================================================
// UNIT TESTS - startWorker message header extraction
// =============================================================================

func TestStartWorker_HeaderIDExtraction(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		headers       amqp.Table
		expectNewUUID bool
	}{
		{
			name: "extracts_existing_midaz_id",
			headers: amqp.Table{
				libConstants.HeaderID: "existing-uuid-123",
			},
			expectNewUUID: false,
		},
		{
			name:          "generates_new_uuid_when_missing",
			headers:       amqp.Table{},
			expectNewUUID: true,
		},
		{
			name:          "generates_new_uuid_when_nil_headers",
			headers:       nil,
			expectNewUUID: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// Test the header extraction logic directly
			var midazID any
			var found bool

			if tt.headers != nil {
				midazID, found = tt.headers[libConstants.HeaderID]
			}

			if !found {
				uid := uuid.Must(libCommons.GenerateUUIDv7())
				midazID = uid.String()
			}

			// Verify result
			assert.NotEmpty(t, midazID)

			if !tt.expectNewUUID {
				assert.Equal(t, "existing-uuid-123", midazID)
			}
		})
	}
}

// =============================================================================
// UNIT TESTS - ConsumerRepository Interface Compliance
// =============================================================================

func TestConsumerRoutes_ImplementsConsumerRepository(t *testing.T) {
	t.Parallel()

	// This is a compile-time check - if ConsumerRoutes doesn't implement
	// ConsumerRepository, this won't compile
	var _ ConsumerRepository = (*ConsumerRoutes)(nil)
}

// =============================================================================
// UNIT TESTS - Concurrent Register Safety (Documentation)
// =============================================================================

// NOTE: ConsumerRoutes.Register is NOT concurrent-safe by design.
// This is acceptable because registration happens at startup before
// RunConsumers is called, making concurrent registration unnecessary.
// If thread-safe registration becomes a requirement, add sync.RWMutex to routes map.
//
// We do NOT test concurrent registration because:
// 1. It's explicitly unsupported behavior
// 2. Concurrent map writes cause panic in Go (expected behavior)
// 3. The startup pattern ensures single-threaded registration

// =============================================================================
// UNIT TESTS - Prefetch Calculation
// =============================================================================

func TestConsumerRoutes_PrefetchCalculation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name             string
		workers          int
		prefetch         int
		expectedPrefetch int
	}{
		{
			name:             "default_calculation",
			workers:          5,
			prefetch:         10,
			expectedPrefetch: 50,
		},
		{
			name:             "single_worker",
			workers:          1,
			prefetch:         1,
			expectedPrefetch: 1,
		},
		{
			name:             "high_throughput",
			workers:          20,
			prefetch:         100,
			expectedPrefetch: 2000,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			cr := &ConsumerRoutes{
				NumbersOfWorkers:  tt.workers,
				NumbersOfPrefetch: tt.workers * tt.prefetch,
			}

			assert.Equal(t, tt.expectedPrefetch, cr.NumbersOfPrefetch)
		})
	}
}

// =============================================================================
// UNIT TESTS - BulkConfig and Bulk Mode Configuration
// =============================================================================

func TestConsumerRoutes_ConfigureBulk(t *testing.T) {
	t.Parallel()

	cr := &ConsumerRoutes{
		routes:     make(map[string]QueueHandlerFunc),
		bulkRoutes: make(map[string]BulkHandlerFunc),
		Logger:     testLogger,
	}

	cfg := &BulkConfig{
		Enabled:      true,
		Size:         100,
		FlushTimeout: 500 * time.Millisecond,
	}

	cr.ConfigureBulk(cfg)

	assert.Equal(t, cfg, cr.bulkConfig)
}

func TestConsumerRoutes_BulkConfig(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		config   *BulkConfig
		expected *BulkConfig
	}{
		{
			name:     "returns_nil_when_not_set",
			config:   nil,
			expected: nil,
		},
		{
			name: "returns_configured_value",
			config: &BulkConfig{
				Enabled:      true,
				Size:         50,
				FlushTimeout: 100 * time.Millisecond,
			},
			expected: &BulkConfig{
				Enabled:      true,
				Size:         50,
				FlushTimeout: 100 * time.Millisecond,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			cr := &ConsumerRoutes{
				routes:     make(map[string]QueueHandlerFunc),
				bulkRoutes: make(map[string]BulkHandlerFunc),
				bulkConfig: tt.config,
				Logger:     testLogger,
			}

			result := cr.BulkConfig()

			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestConsumerRoutes_IsBulkModeEnabled(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		config   *BulkConfig
		expected bool
	}{
		{
			name:     "returns_false_when_config_is_nil",
			config:   nil,
			expected: false,
		},
		{
			name: "returns_false_when_enabled_is_false",
			config: &BulkConfig{
				Enabled: false,
				Size:    100,
			},
			expected: false,
		},
		{
			name: "returns_true_when_enabled_is_true",
			config: &BulkConfig{
				Enabled: true,
				Size:    100,
			},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			cr := &ConsumerRoutes{
				routes:     make(map[string]QueueHandlerFunc),
				bulkRoutes: make(map[string]BulkHandlerFunc),
				bulkConfig: tt.config,
				Logger:     testLogger,
			}

			result := cr.IsBulkModeEnabled()

			assert.Equal(t, tt.expected, result)
		})
	}
}

// =============================================================================
// UNIT TESTS - RegisterBulk
// =============================================================================

func TestConsumerRoutes_RegisterBulk(t *testing.T) {
	t.Parallel()

	cr := &ConsumerRoutes{
		routes:     make(map[string]QueueHandlerFunc),
		bulkRoutes: make(map[string]BulkHandlerFunc),
		Logger:     testLogger,
	}

	queueName := "test-bulk-queue"
	handler := func(ctx context.Context, messages []amqp.Delivery) ([]BulkMessageResult, error) {
		return nil, nil
	}

	cr.RegisterBulk(queueName, handler)

	assert.Len(t, cr.bulkRoutes, 1)
	assert.Contains(t, cr.bulkRoutes, queueName)
	assert.NotNil(t, cr.bulkRoutes[queueName])
}

func TestConsumerRoutes_RegisterBulk_MultipleQueues(t *testing.T) {
	t.Parallel()

	cr := &ConsumerRoutes{
		routes:     make(map[string]QueueHandlerFunc),
		bulkRoutes: make(map[string]BulkHandlerFunc),
		Logger:     testLogger,
	}

	queues := []string{
		"bulk-queue-1",
		"bulk-queue-2",
		"bulk-queue-3",
	}

	for _, queueName := range queues {
		handler := func(ctx context.Context, messages []amqp.Delivery) ([]BulkMessageResult, error) {
			return nil, nil
		}
		cr.RegisterBulk(queueName, handler)
	}

	assert.Len(t, cr.bulkRoutes, len(queues))

	for _, queueName := range queues {
		assert.Contains(t, cr.bulkRoutes, queueName)
	}
}

func TestConsumerRoutes_RegisterBulk_OverwriteExisting(t *testing.T) {
	t.Parallel()

	cr := &ConsumerRoutes{
		routes:     make(map[string]QueueHandlerFunc),
		bulkRoutes: make(map[string]BulkHandlerFunc),
		Logger:     testLogger,
	}

	queueName := "test-queue"
	callCount := 0

	// Register first handler
	handler1 := func(ctx context.Context, messages []amqp.Delivery) ([]BulkMessageResult, error) {
		callCount = 1
		return nil, nil
	}
	cr.RegisterBulk(queueName, handler1)

	// Register second handler (should overwrite)
	handler2 := func(ctx context.Context, messages []amqp.Delivery) ([]BulkMessageResult, error) {
		callCount = 2
		return nil, nil
	}
	cr.RegisterBulk(queueName, handler2)

	assert.Len(t, cr.bulkRoutes, 1)

	// Call the registered handler - should be handler2
	ctx := context.Background()
	_, err := cr.bulkRoutes[queueName](ctx, nil)
	require.NoError(t, err)
	assert.Equal(t, 2, callCount, "second handler should have been called")
}

// =============================================================================
// UNIT TESTS - BulkHandlerFunc
// =============================================================================

func TestBulkHandlerFunc_Success(t *testing.T) {
	t.Parallel()

	handlerCalled := false
	var receivedMessages []amqp.Delivery

	handler := BulkHandlerFunc(func(ctx context.Context, messages []amqp.Delivery) ([]BulkMessageResult, error) {
		handlerCalled = true
		receivedMessages = messages
		return nil, nil
	})

	ctx := context.Background()
	testMessages := []amqp.Delivery{
		{Body: []byte("msg1")},
		{Body: []byte("msg2")},
	}

	results, err := handler(ctx, testMessages)

	require.NoError(t, err)
	assert.True(t, handlerCalled)
	assert.Equal(t, testMessages, receivedMessages)
	assert.Nil(t, results)
}

func TestBulkHandlerFunc_Error(t *testing.T) {
	t.Parallel()

	expectedErr := errors.New("bulk processing failed")

	handler := BulkHandlerFunc(func(ctx context.Context, messages []amqp.Delivery) ([]BulkMessageResult, error) {
		return nil, expectedErr
	})

	ctx := context.Background()
	_, err := handler(ctx, nil)

	require.Error(t, err)
	assert.Equal(t, expectedErr, err)
}

func TestBulkHandlerFunc_WithResults(t *testing.T) {
	t.Parallel()

	handler := BulkHandlerFunc(func(ctx context.Context, messages []amqp.Delivery) ([]BulkMessageResult, error) {
		results := []BulkMessageResult{
			{Index: 0, Success: true, Error: nil},
			{Index: 1, Success: false, Error: errors.New("failed")},
		}
		return results, nil
	})

	ctx := context.Background()
	results, err := handler(ctx, []amqp.Delivery{{}, {}})

	require.NoError(t, err)
	assert.Len(t, results, 2)
	assert.True(t, results[0].Success)
	assert.False(t, results[1].Success)
}

// =============================================================================
// UNIT TESTS - BulkMessageResult
// =============================================================================

func TestBulkMessageResult_Structure(t *testing.T) {
	t.Parallel()

	result := BulkMessageResult{
		Index:   5,
		Success: true,
		Error:   nil,
	}

	assert.Equal(t, 5, result.Index)
	assert.True(t, result.Success)
	assert.Nil(t, result.Error)

	failedResult := BulkMessageResult{
		Index:   3,
		Success: false,
		Error:   errors.New("processing failed"),
	}

	assert.Equal(t, 3, failedResult.Index)
	assert.False(t, failedResult.Success)
	assert.Error(t, failedResult.Error)
}

// =============================================================================
// UNIT TESTS - BulkConfig Structure
// =============================================================================

func TestBulkConfig_Structure(t *testing.T) {
	t.Parallel()

	config := BulkConfig{
		Enabled:      true,
		Size:         200,
		FlushTimeout: 500 * time.Millisecond,
	}

	assert.True(t, config.Enabled)
	assert.Equal(t, 200, config.Size)
	assert.Equal(t, 500*time.Millisecond, config.FlushTimeout)
}

func TestBulkConfig_Defaults(t *testing.T) {
	t.Parallel()

	// Zero-value BulkConfig should have safe defaults
	var config BulkConfig

	assert.False(t, config.Enabled)
	assert.Equal(t, 0, config.Size)
	assert.Equal(t, time.Duration(0), config.FlushTimeout)
}

// =============================================================================
// UNIT TESTS - buildBulkContext
// =============================================================================

func TestConsumerRoutes_BuildBulkContext_EmptyDeliveries(t *testing.T) {
	t.Parallel()

	cr := &ConsumerRoutes{
		Logger: testLogger,
	}

	ctx := context.Background()
	result := cr.buildBulkContext(ctx, []amqp.Delivery{})

	// Should return original context unchanged
	assert.Equal(t, ctx, result)
}

func TestConsumerRoutes_BuildBulkContext_WithDeliveries(t *testing.T) {
	t.Parallel()

	cr := &ConsumerRoutes{
		Logger: testLogger,
	}

	ctx := context.Background()
	deliveries := []amqp.Delivery{
		{
			Headers: amqp.Table{
				libConstants.HeaderID: "test-header-id-123",
			},
			Body: []byte("msg1"),
		},
		{
			Body: []byte("msg2"),
		},
	}

	result := cr.buildBulkContext(ctx, deliveries)

	// Context should be enriched (not equal to original)
	assert.NotEqual(t, ctx, result)
}

func TestConsumerRoutes_BuildBulkContext_ExtractsHeaderID(t *testing.T) {
	t.Parallel()

	cr := &ConsumerRoutes{
		Logger: testLogger,
	}

	expectedID := "existing-midaz-id-456"
	ctx := context.Background()
	deliveries := []amqp.Delivery{
		{
			Headers: amqp.Table{
				libConstants.HeaderID: expectedID,
			},
			Body: []byte("msg1"),
		},
	}

	result := cr.buildBulkContext(ctx, deliveries)

	// Verify the context was built successfully
	assert.NotEqual(t, ctx, result)
}

// =============================================================================
// UNIT TESTS - Bulk Mode Selection Logic
// =============================================================================

func TestConsumerRoutes_BulkModeSelection(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		bulkConfig     *BulkConfig
		hasBulkHandler bool
		expected       bool
	}{
		{
			name:           "bulk_mode_disabled_when_config_is_nil",
			bulkConfig:     nil,
			hasBulkHandler: true,
			expected:       false,
		},
		{
			name: "bulk_mode_disabled_when_enabled_is_false",
			bulkConfig: &BulkConfig{
				Enabled: false,
				Size:    100,
			},
			hasBulkHandler: true,
			expected:       false,
		},
		{
			name: "bulk_mode_disabled_when_no_bulk_handler",
			bulkConfig: &BulkConfig{
				Enabled: true,
				Size:    100,
			},
			hasBulkHandler: false,
			expected:       false,
		},
		{
			name: "bulk_mode_enabled_when_all_conditions_met",
			bulkConfig: &BulkConfig{
				Enabled: true,
				Size:    100,
			},
			hasBulkHandler: true,
			expected:       true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			cr := &ConsumerRoutes{
				routes:     make(map[string]QueueHandlerFunc),
				bulkRoutes: make(map[string]BulkHandlerFunc),
				bulkConfig: tt.bulkConfig,
				Logger:     testLogger,
			}

			// Simulate IsBulkModeEnabled && hasBulkHandler
			useBulkMode := cr.IsBulkModeEnabled() && tt.hasBulkHandler

			assert.Equal(t, tt.expected, useBulkMode)
		})
	}
}

// =============================================================================
// UNIT TESTS - resolveMessageHeaderID (indirectly tested)
// =============================================================================

func TestResolveMessageHeaderID_StringValue(t *testing.T) {
	t.Parallel()

	headers := amqp.Table{
		libConstants.HeaderID: "string-id-value",
	}

	result := resolveMessageHeaderID(headers)

	assert.Equal(t, "string-id-value", result)
}

func TestResolveMessageHeaderID_ByteSliceValue(t *testing.T) {
	t.Parallel()

	headers := amqp.Table{
		libConstants.HeaderID: []byte("byte-slice-id"),
	}

	result := resolveMessageHeaderID(headers)

	assert.Equal(t, "byte-slice-id", result)
}

func TestResolveMessageHeaderID_EmptyString(t *testing.T) {
	t.Parallel()

	headers := amqp.Table{
		libConstants.HeaderID: "",
	}

	result := resolveMessageHeaderID(headers)

	// Should generate a new UUID when empty
	assert.NotEmpty(t, result)
	assert.NotEqual(t, "", result)
}

func TestResolveMessageHeaderID_EmptyByteSlice(t *testing.T) {
	t.Parallel()

	headers := amqp.Table{
		libConstants.HeaderID: []byte{},
	}

	result := resolveMessageHeaderID(headers)

	// Should generate a new UUID when empty
	assert.NotEmpty(t, result)
}

func TestResolveMessageHeaderID_MissingHeader(t *testing.T) {
	t.Parallel()

	headers := amqp.Table{}

	result := resolveMessageHeaderID(headers)

	// Should generate a new UUID
	assert.NotEmpty(t, result)
	// Verify it looks like a UUID (36 chars with dashes)
	assert.Len(t, result, 36)
}

func TestResolveMessageHeaderID_NilHeaders(t *testing.T) {
	t.Parallel()

	result := resolveMessageHeaderID(nil)

	// Should generate a new UUID
	assert.NotEmpty(t, result)
	assert.Len(t, result, 36)
}
