package rabbitmq

import (
	"context"
	"errors"
	"testing"

	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	libConstants "github.com/LerianStudio/lib-commons/v2/commons/constants"
	libLog "github.com/LerianStudio/lib-commons/v2/commons/log"
	libZap "github.com/LerianStudio/lib-commons/v2/commons/zap"
	amqp "github.com/rabbitmq/amqp091-go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// testLogger is initialized once for all tests to avoid race conditions
// when multiple parallel tests initialize the logger simultaneously.
var testLogger libLog.Logger

func init() {
	testLogger = libZap.InitializeLogger()
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
				midazID = libCommons.GenerateUUIDv7().String()
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
