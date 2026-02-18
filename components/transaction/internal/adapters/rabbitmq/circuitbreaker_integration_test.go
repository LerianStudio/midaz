//go:build integration

// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package rabbitmq

import (
	"context"
	"encoding/json"
	"os"
	"sync"
	"testing"
	"time"

	libCircuitBreaker "github.com/LerianStudio/lib-commons/v2/commons/circuitbreaker"
	libRabbitmq "github.com/LerianStudio/lib-commons/v2/commons/rabbitmq"
	libZap "github.com/LerianStudio/lib-commons/v2/commons/zap"
	"github.com/LerianStudio/midaz/v3/tests/utils/chaos"
	rmqtestutil "github.com/LerianStudio/midaz/v3/tests/utils/rabbitmq"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// =============================================================================
// CIRCUIT BREAKER INTEGRATION TEST INFRASTRUCTURE
// =============================================================================

// skipIfNotChaosIntegration skips the test if CHAOS=1 environment variable is not set.
func skipIfNotChaosIntegration(t *testing.T) {
	t.Helper()
	if os.Getenv("CHAOS") != "1" {
		t.Skip("skipping chaos integration test (set CHAOS=1 to run)")
	}
}

// circuitBreakerTestInfra holds infrastructure for circuit breaker integration tests.
type circuitBreakerTestInfra struct {
	rmqContainer        *rmqtestutil.ContainerResult
	conn                *libRabbitmq.RabbitMQConnection
	rawProducer         *ProducerRabbitMQRepository
	cbProducer          *CircuitBreakerProducer
	cbManager           libCircuitBreaker.Manager
	exchange            string
	routingKey          string
	queue               string
	stateChangeListener *cbTestStateChangeListener
}

// cbStateChangeRecord records a circuit breaker state transition for verification.
// Named with 'cb' prefix to avoid conflict with stateChangeRecord in healthcheck_test.go.
type cbStateChangeRecord struct {
	ServiceName string
	From        libCircuitBreaker.State
	To          libCircuitBreaker.State
	Timestamp   time.Time
}

// cbTestStateChangeListener implements StateChangeListener for circuit breaker integration testing.
// Named with 'cb' prefix to avoid conflict with similar types in healthcheck_test.go.
// Thread-safe: OnStateChange may be called from circuit breaker's internal goroutines.
type cbTestStateChangeListener struct {
	mu      sync.Mutex
	records []cbStateChangeRecord
}

func (l *cbTestStateChangeListener) OnStateChange(serviceName string, from, to libCircuitBreaker.State) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.records = append(l.records, cbStateChangeRecord{
		ServiceName: serviceName,
		From:        from,
		To:          to,
		Timestamp:   time.Now(),
	})
}

// GetRecords returns a copy of the state change records in a thread-safe manner.
func (l *cbTestStateChangeListener) GetRecords() []cbStateChangeRecord {
	l.mu.Lock()
	defer l.mu.Unlock()
	result := make([]cbStateChangeRecord, len(l.records))
	copy(result, l.records)
	return result
}

// circuitBreakerTestMessage represents a test message for circuit breaker tests.
type circuitBreakerTestMessage struct {
	ID        string    `json:"id"`
	Timestamp time.Time `json:"timestamp"`
	Data      string    `json:"data"`
}

// setupCircuitBreakerTestInfra creates test infrastructure with CircuitBreakerProducer.
func setupCircuitBreakerTestInfra(t *testing.T, cbConfig CircuitBreakerConfig) *circuitBreakerTestInfra {
	t.Helper()

	// Setup RabbitMQ container
	rmqContainer := rmqtestutil.SetupContainer(t)

	// Setup exchange and queue
	exchange := "cb-test-exchange"
	routingKey := "cb.test.routing.key"
	queue := "cb-test-queue"

	rmqtestutil.SetupExchange(t, rmqContainer.Channel, exchange, "topic")
	rmqtestutil.SetupQueue(t, rmqContainer.Channel, queue, exchange, routingKey)

	// Create lib-commons RabbitMQ connection
	logger := libZap.InitializeLogger()
	healthCheckURL := "http://" + rmqContainer.Host + ":" + rmqContainer.MgmtPort
	conn := &libRabbitmq.RabbitMQConnection{
		ConnectionStringSource: rmqContainer.URI,
		HealthCheckURL:         healthCheckURL,
		Host:                   rmqContainer.Host,
		Port:                   rmqContainer.AMQPPort,
		User:                   rmqtestutil.DefaultUser,
		Pass:                   rmqtestutil.DefaultPassword,
		Logger:                 logger,
	}

	// Create raw producer
	rawProducer, err := NewProducerRabbitMQ(conn)
	require.NoError(t, err, "failed to create raw producer")

	// Create circuit breaker manager
	cbManager := libCircuitBreaker.NewManager(logger)

	// Initialize circuit breaker with provided config
	cbManager.GetOrCreate(CircuitBreakerServiceName, RabbitMQCircuitBreakerConfig(cbConfig))

	// Create state change listener for verification
	stateListener := &cbTestStateChangeListener{
		records: make([]cbStateChangeRecord, 0),
	}
	cbManager.RegisterStateChangeListener(stateListener)

	// Create CircuitBreakerProducer
	cbProducer, err := NewCircuitBreakerProducer(rawProducer, cbManager, logger, cbConfig.OperationTimeout)
	require.NoError(t, err, "failed to create circuit breaker producer")

	// Register cleanup for AMQP resources
	// Note: Cleanup runs in LIFO order, so connection is closed after channel
	t.Cleanup(func() {
		if conn.Channel != nil {
			_ = conn.Channel.Close()
		}
	})
	t.Cleanup(func() {
		if conn.Connection != nil {
			_ = conn.Connection.Close()
		}
	})

	return &circuitBreakerTestInfra{
		rmqContainer:        rmqContainer,
		conn:                conn,
		rawProducer:         rawProducer,
		cbProducer:          cbProducer,
		cbManager:           cbManager,
		exchange:            exchange,
		routingKey:          routingKey,
		queue:               queue,
		stateChangeListener: stateListener,
	}
}

// defaultCircuitBreakerConfig returns a configuration suitable for integration testing.
// Uses shorter timeouts to make tests faster.
func defaultCircuitBreakerConfig() CircuitBreakerConfig {
	return CircuitBreakerConfig{
		ConsecutiveFailures: 3,   // Open after 3 consecutive failures
		FailureRatio:        0.5, // Or 50% failure rate
		MinRequests:         5,   // Need at least 5 requests to check ratio
		MaxRequests:         2,   // Allow 2 requests in half-open
		Interval:            30 * time.Second,
		Timeout:             5 * time.Second, // Short timeout for faster tests
		HealthCheckInterval: 2 * time.Second,
		HealthCheckTimeout:  1 * time.Second,
	}
}

// aggressiveCircuitBreakerConfig returns a configuration that opens circuit quickly.
// Useful for testing circuit opening behavior.
func aggressiveCircuitBreakerConfig() CircuitBreakerConfig {
	return CircuitBreakerConfig{
		ConsecutiveFailures: 2,   // Open after 2 consecutive failures
		FailureRatio:        0.3, // Or 30% failure rate
		MinRequests:         3,   // Need at least 3 requests to check ratio
		MaxRequests:         1,   // Allow 1 request in half-open
		Interval:            10 * time.Second,
		Timeout:             3 * time.Second, // Very short timeout
		HealthCheckInterval: 1 * time.Second,
		HealthCheckTimeout:  500 * time.Millisecond,
		OperationTimeout:    DefaultOperationTimeout, // Explicit for test clarity
	}
}

// =============================================================================
// INTEGRATION TESTS - NORMAL OPERATION
// =============================================================================

// TestIntegration_CircuitBreaker_NormalOperation verifies that the circuit breaker
// allows messages through when RabbitMQ is healthy.
func TestIntegration_CircuitBreaker_NormalOperation(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	infra := setupCircuitBreakerTestInfra(t, defaultCircuitBreakerConfig())
	ctx := context.Background()

	// Verify circuit starts closed
	assert.Equal(t, libCircuitBreaker.StateClosed, infra.cbProducer.GetCircuitState(),
		"circuit should start in closed state")

	// Publish multiple messages
	numMessages := 10
	for i := 0; i < numMessages; i++ {
		msg := circuitBreakerTestMessage{
			ID:        uuid.New().String(),
			Timestamp: time.Now(),
			Data:      "Normal operation test message",
		}
		msgBytes, err := json.Marshal(msg)
		require.NoError(t, err, "failed to marshal message")

		_, err = infra.cbProducer.ProducerDefault(ctx, infra.exchange, infra.routingKey, msgBytes)
		require.NoError(t, err, "publish %d should succeed", i+1)
	}

	// Verify all messages arrived using eventual consistency check
	// This avoids flaky tests by polling instead of using fixed sleep
	require.Eventually(t, func() bool {
		return rmqtestutil.GetQueueMessageCount(t, infra.rmqContainer.Channel, infra.queue) == numMessages
	}, 5*time.Second, 100*time.Millisecond, "all messages should be in queue")

	// Circuit should remain closed
	assert.Equal(t, libCircuitBreaker.StateClosed, infra.cbProducer.GetCircuitState(),
		"circuit should remain closed after successful publishes")

	// No state changes should have occurred
	assert.Empty(t, infra.stateChangeListener.GetRecords(),
		"no state changes should occur during normal operation")

	t.Log("Integration test passed: normal operation through circuit breaker verified")
}

// TestIntegration_CircuitBreaker_HealthCheck verifies health check integration.
func TestIntegration_CircuitBreaker_HealthCheck(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	infra := setupCircuitBreakerTestInfra(t, defaultCircuitBreakerConfig())

	// Health check should pass
	healthy := infra.cbProducer.CheckRabbitMQHealth()
	assert.True(t, healthy, "health check should pass when RabbitMQ is healthy")

	// Circuit should be healthy
	assert.True(t, infra.cbProducer.IsCircuitHealthy(),
		"circuit should report healthy when closed")

	t.Log("Integration test passed: health check verified")
}

// =============================================================================
// CHAOS TESTS - CIRCUIT OPENING
// =============================================================================

// TestIntegration_Chaos_CircuitBreaker_OpensOnFailure verifies that the circuit
// opens after consecutive failures when RabbitMQ becomes unavailable.
func TestIntegration_Chaos_CircuitBreaker_OpensOnFailure(t *testing.T) {
	skipIfNotChaosIntegration(t)
	if testing.Short() {
		t.Skip("skipping chaos integration test in short mode")
	}

	// Use aggressive config to open circuit quickly
	infra := setupCircuitBreakerTestInfra(t, aggressiveCircuitBreakerConfig())
	ctx := context.Background()

	// Step 1: Verify baseline - publish one message successfully
	t.Log("Step 1: Verifying baseline connectivity")
	msg := circuitBreakerTestMessage{
		ID:        uuid.New().String(),
		Timestamp: time.Now(),
		Data:      "Baseline message before chaos",
	}
	msgBytes, err := json.Marshal(msg)
	require.NoError(t, err, "failed to marshal baseline message")

	_, err = infra.cbProducer.ProducerDefault(ctx, infra.exchange, infra.routingKey, msgBytes)
	require.NoError(t, err, "baseline publish should succeed")
	assert.Equal(t, libCircuitBreaker.StateClosed, infra.cbProducer.GetCircuitState())
	t.Log("Baseline verified - circuit closed, publish succeeded")

	// Step 2: Stop RabbitMQ container to simulate failure
	t.Log("Step 2: Stopping RabbitMQ container to simulate failure")
	containerID := infra.rmqContainer.Container.GetContainerID()
	chaosOrch := chaos.NewOrchestrator(t)
	defer func() {
		if closeErr := chaosOrch.Close(); closeErr != nil {
			t.Logf("Warning: failed to close chaos orchestrator: %v", closeErr)
		}
	}()

	err = chaosOrch.StopContainer(ctx, containerID, 5*time.Second)
	require.NoError(t, err, "stopping container should succeed")
	t.Log("RabbitMQ container stopped")

	// Step 3: Attempt multiple publishes - should fail and eventually open circuit
	t.Log("Step 3: Attempting publishes to trigger circuit opening")
	failureCount := 0
	for i := 0; i < 5; i++ {
		failMsg := circuitBreakerTestMessage{
			ID:        uuid.New().String(),
			Timestamp: time.Now(),
			Data:      "Message during failure",
		}
		failMsgBytes, marshalErr := json.Marshal(failMsg)
		require.NoError(t, marshalErr, "failed to marshal failure message")

		ctxWithTimeout, cancel := context.WithTimeout(ctx, 2*time.Second)
		_, publishErr := infra.cbProducer.ProducerDefault(ctxWithTimeout, infra.exchange, infra.routingKey, failMsgBytes)
		cancel()

		if publishErr != nil {
			failureCount++
			t.Logf("Publish %d failed (expected): %v", i+1, publishErr)
		}
	}

	assert.Greater(t, failureCount, 0, "some publishes should have failed")

	// Step 4: Verify circuit has opened
	t.Log("Step 4: Verifying circuit state")
	state := infra.cbProducer.GetCircuitState()
	t.Logf("Circuit state after failures: %s", state)

	// Circuit should be open or half-open (depending on timing)
	assert.NotEqual(t, libCircuitBreaker.StateClosed, state,
		"circuit should not be closed after failures")

	// Verify state change was recorded
	records := infra.stateChangeListener.GetRecords()
	assert.NotEmpty(t, records,
		"state change should have been recorded")

	// Find the state change to open
	foundOpenTransition := false
	for _, record := range records {
		if record.To == libCircuitBreaker.StateOpen {
			foundOpenTransition = true
			t.Logf("State change recorded: %s -> %s at %v",
				record.From, record.To, record.Timestamp)
		}
	}
	assert.True(t, foundOpenTransition, "should have recorded transition to open state")

	t.Log("Chaos test passed: circuit opens on consecutive failures")
}

// TestIntegration_Chaos_CircuitBreaker_FastFailWhenOpen verifies that requests
// are rejected immediately (fast-fail) when the circuit is open.
func TestIntegration_Chaos_CircuitBreaker_FastFailWhenOpen(t *testing.T) {
	skipIfNotChaosIntegration(t)
	if testing.Short() {
		t.Skip("skipping chaos integration test in short mode")
	}

	infra := setupCircuitBreakerTestInfra(t, aggressiveCircuitBreakerConfig())
	ctx := context.Background()

	// Step 1: Stop container and open circuit
	t.Log("Step 1: Opening circuit by inducing failures")
	containerID := infra.rmqContainer.Container.GetContainerID()
	chaosOrch := chaos.NewOrchestrator(t)
	defer func() {
		if closeErr := chaosOrch.Close(); closeErr != nil {
			t.Logf("Warning: failed to close chaos orchestrator: %v", closeErr)
		}
	}()

	err := chaosOrch.StopContainer(ctx, containerID, 5*time.Second)
	require.NoError(t, err)

	// Trigger circuit opening with failures
	for i := 0; i < 5; i++ {
		msg := circuitBreakerTestMessage{ID: uuid.New().String(), Timestamp: time.Now(), Data: "trigger"}
		msgBytes, _ := json.Marshal(msg)
		ctxWithTimeout, cancel := context.WithTimeout(ctx, 2*time.Second)
		infra.cbProducer.ProducerDefault(ctxWithTimeout, infra.exchange, infra.routingKey, msgBytes)
		cancel()
	}

	// Wait a moment for state to stabilize
	time.Sleep(500 * time.Millisecond)

	// Verify circuit is open - circuit must have opened after 5 failures with aggressive config
	state := infra.cbProducer.GetCircuitState()
	require.NotEqual(t, libCircuitBreaker.StateClosed, state,
		"circuit should have opened after consecutive failures with aggressive config")
	t.Logf("Circuit state: %s", state)

	// Step 2: Measure response time for rejected requests
	t.Log("Step 2: Measuring fast-fail response time")

	const numRequests = 10

	// Use a CI-friendly threshold: max(200ms, operationTimeout/4)
	// This scales with configuration and avoids flakiness under load
	operationTimeout := aggressiveCircuitBreakerConfig().OperationTimeout
	maxAcceptableLatency := 200 * time.Millisecond
	if quarterTimeout := operationTimeout / 4; quarterTimeout > maxAcceptableLatency {
		maxAcceptableLatency = quarterTimeout
	}

	var totalLatency time.Duration

	for i := 0; i < numRequests; i++ {
		msg := circuitBreakerTestMessage{
			ID:        uuid.New().String(),
			Timestamp: time.Now(),
			Data:      "Fast-fail test message",
		}
		msgBytes, _ := json.Marshal(msg)

		start := time.Now()
		_, err := infra.cbProducer.ProducerDefault(ctx, infra.exchange, infra.routingKey, msgBytes)
		latency := time.Since(start)
		totalLatency += latency

		// Request should fail with service unavailable error
		require.Error(t, err, "request should be rejected when circuit is open")
		assert.ErrorIs(t, err, ErrServiceUnavailable,
			"error should be ErrServiceUnavailable")

		t.Logf("Request %d: latency=%v, error=%v", i+1, latency, err)

		// Each request should be fast
		assert.Less(t, latency, maxAcceptableLatency,
			"fast-fail latency should be under %v", maxAcceptableLatency)
	}

	avgLatency := totalLatency / numRequests
	t.Logf("Average fast-fail latency: %v", avgLatency)

	assert.Less(t, avgLatency, maxAcceptableLatency,
		"average latency should be under %v for fast-fail", maxAcceptableLatency)

	t.Log("Chaos test passed: fast-fail response time verified")
}

// =============================================================================
// CHAOS TESTS - FULL LIFECYCLE
// =============================================================================

// TestIntegration_Chaos_CircuitBreaker_FullLifecycle tests the complete circuit
// breaker lifecycle: normal -> failure -> open -> recovery -> closed
func TestIntegration_Chaos_CircuitBreaker_FullLifecycle(t *testing.T) {
	skipIfNotChaosIntegration(t)
	if testing.Short() {
		t.Skip("skipping chaos integration test in short mode")
	}

	// Use config with very short timeout for faster recovery testing
	config := CircuitBreakerConfig{
		ConsecutiveFailures: 2,
		FailureRatio:        0.5,
		MinRequests:         3,
		MaxRequests:         1,
		Interval:            10 * time.Second,
		Timeout:             2 * time.Second, // Very short for quick half-open transition
		HealthCheckInterval: 1 * time.Second,
		HealthCheckTimeout:  500 * time.Millisecond,
	}

	infra := setupCircuitBreakerTestInfra(t, config)
	ctx := context.Background()
	chaosOrch := chaos.NewOrchestrator(t)
	defer func() {
		if closeErr := chaosOrch.Close(); closeErr != nil {
			t.Logf("Warning: failed to close chaos orchestrator: %v", closeErr)
		}
	}()

	containerID := infra.rmqContainer.Container.GetContainerID()

	// Phase 1: Normal operation
	t.Log("Phase 1: Normal operation - circuit closed")
	assert.Equal(t, libCircuitBreaker.StateClosed, infra.cbProducer.GetCircuitState())

	msg := circuitBreakerTestMessage{ID: uuid.New().String(), Timestamp: time.Now(), Data: "phase1"}
	msgBytes, _ := json.Marshal(msg)
	_, err := infra.cbProducer.ProducerDefault(ctx, infra.exchange, infra.routingKey, msgBytes)
	require.NoError(t, err, "Phase 1: publish should succeed")
	t.Log("Phase 1 complete: message published successfully")

	// Phase 2: Induce failure
	t.Log("Phase 2: Inducing failure - stopping RabbitMQ")
	err = chaosOrch.StopContainer(ctx, containerID, 5*time.Second)
	require.NoError(t, err)

	// Trigger failures to open circuit
	for i := 0; i < 5; i++ {
		msg := circuitBreakerTestMessage{ID: uuid.New().String(), Timestamp: time.Now(), Data: "phase2"}
		msgBytes, _ := json.Marshal(msg)
		ctxWithTimeout, cancel := context.WithTimeout(ctx, 2*time.Second)
		infra.cbProducer.ProducerDefault(ctxWithTimeout, infra.exchange, infra.routingKey, msgBytes)
		cancel()
	}

	// Wait for circuit to open
	time.Sleep(500 * time.Millisecond)
	state := infra.cbProducer.GetCircuitState()
	t.Logf("Phase 2 complete: circuit state = %s", state)

	// Phase 3: Fast-fail while open
	t.Log("Phase 3: Verifying fast-fail while circuit is open/half-open")
	if state != libCircuitBreaker.StateClosed {
		start := time.Now()
		msg := circuitBreakerTestMessage{ID: uuid.New().String(), Timestamp: time.Now(), Data: "phase3"}
		msgBytes, _ := json.Marshal(msg)
		_, err = infra.cbProducer.ProducerDefault(ctx, infra.exchange, infra.routingKey, msgBytes)
		fastFailLatency := time.Since(start)

		require.Error(t, err, "Phase 3: should fail when circuit is not closed")
		t.Logf("Phase 3 complete: fast-fail latency = %v", fastFailLatency)
	} else {
		t.Log("Phase 3 skipped: circuit already closed")
	}

	// Phase 4: Recovery
	t.Log("Phase 4: Recovery - restarting RabbitMQ")
	err = chaosOrch.StartContainer(ctx, containerID)
	require.NoError(t, err)

	// Wait for container to be healthy (Docker state)
	err = chaosOrch.WaitForContainerRunning(ctx, containerID, 60*time.Second)
	require.NoError(t, err)

	// Wait for RabbitMQ to be ready to accept connections (not just Docker running state)
	t.Log("Phase 4: Waiting for RabbitMQ to be ready to accept connections")
	_ = rmqtestutil.CreateChannelWithRetry(t, infra.rmqContainer.URI, 30*time.Second)
	t.Log("Phase 4: RabbitMQ is ready to accept connections")

	// Wait for circuit timeout to allow half-open
	t.Logf("Phase 4: Waiting for circuit timeout (%v) to allow recovery", config.Timeout)
	time.Sleep(config.Timeout + 1*time.Second)

	// Phase 5: Circuit should recover
	t.Log("Phase 5: Verifying circuit recovery")

	// Try to publish - this should trigger half-open test and potentially close circuit
	maxRetries := 10
	var lastErr error
	for i := 0; i < maxRetries; i++ {
		msg := circuitBreakerTestMessage{
			ID:        uuid.New().String(),
			Timestamp: time.Now(),
			Data:      "phase5-recovery",
		}
		msgBytes, _ := json.Marshal(msg)

		_, err = infra.cbProducer.ProducerDefault(ctx, infra.exchange, infra.routingKey, msgBytes)
		lastErr = err

		currentState := infra.cbProducer.GetCircuitState()
		t.Logf("Phase 5 attempt %d: state=%s, error=%v", i+1, currentState, err)

		if currentState == libCircuitBreaker.StateClosed && err == nil {
			t.Log("Phase 5 complete: circuit recovered to closed state")
			break
		}

		time.Sleep(1 * time.Second)
	}

	// Final verification
	finalState := infra.cbProducer.GetCircuitState()
	t.Logf("Final circuit state: %s", finalState)

	// Circuit must have transitioned from open state after recovery
	// Closed = fully recovered, HalfOpen = recovery in progress, both are acceptable
	// Open = recovery failed, which is a test failure
	require.NotEqual(t, libCircuitBreaker.StateOpen, finalState,
		"circuit should have transitioned from open state after recovery")

	if finalState == libCircuitBreaker.StateClosed {
		assert.NoError(t, lastErr, "final publish should succeed when circuit is closed")
		t.Log("Full lifecycle test passed: circuit fully recovered")
	} else {
		// Half-open is acceptable - recovery is in progress
		t.Log("Circuit is in half-open state - recovery in progress (acceptable)")
	}

	// Verify state transitions were recorded
	t.Log("State transitions recorded:")
	finalRecords := infra.stateChangeListener.GetRecords()
	for _, record := range finalRecords {
		t.Logf("  %s: %s -> %s", record.ServiceName, record.From, record.To)
	}

	t.Log("Chaos test passed: full lifecycle verified")
}
