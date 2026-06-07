//go:build integration

// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package rabbitmq

import (
	"context"
	"fmt"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	pkg "github.com/LerianStudio/midaz/v4/pkg"
	pkgConstant "github.com/LerianStudio/midaz/v4/pkg/reporter/constant"
	pkgRabbitmq "github.com/LerianStudio/midaz/v4/pkg/reporter/rabbitmq"
	"github.com/LerianStudio/midaz/v4/tests/reporter/utils/containers"

	libConstant "github.com/LerianStudio/lib-commons/v5/commons/constants"
	libRabbitMQ "github.com/LerianStudio/lib-commons/v5/commons/rabbitmq"
	libOtel "github.com/LerianStudio/lib-observability/tracing"
	libZap "github.com/LerianStudio/lib-observability/zap"
	"github.com/google/uuid"
	amqp "github.com/rabbitmq/amqp091-go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/network"
)

// Package-level variables shared across all tests in this file.
var (
	rabbitContainer *containers.RabbitMQContainer
	testNetwork     *testcontainers.DockerNetwork
)

func TestMain(m *testing.M) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	fmt.Fprintf(os.Stderr, "Starting RabbitMQ testcontainer for handleFailedMessage integration tests...\n")

	var err error

	testNetwork, err = network.New(ctx, network.WithDriver("bridge"))
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to create network: %v\n", err)
		os.Exit(1)
	}

	rabbitContainer, err = containers.StartRabbitMQ(ctx, testNetwork.Name, "")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to start RabbitMQ: %v\n", err)
		_ = testNetwork.Remove(ctx)
		os.Exit(1)
	}

	fmt.Fprintf(os.Stderr, "RabbitMQ started at %s\n", rabbitContainer.AmqpURL)

	code := m.Run()

	cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cleanupCancel()

	if rabbitContainer != nil {
		_ = rabbitContainer.Terminate(cleanupCtx)
	}

	if testNetwork != nil {
		_ = testNetwork.Remove(cleanupCtx)
	}

	os.Exit(code)
}

// setupConsumer creates a ConsumerRoutes instance with the given handler registered
// on the generate-report queue. Cleanup cancels the context and waits for goroutines.
func setupConsumer(t *testing.T, handler pkgRabbitmq.QueueHandlerFunc) {
	t.Helper()

	logger, err := libZap.New(libZap.Config{Environment: libZap.EnvironmentLocal, OTelLibraryName: "reporter"})
	require.NoError(t, err, "initialize logger")

	telemetry, err := libOtel.NewTelemetry(libOtel.TelemetryConfig{
		LibraryName:     "test",
		ServiceName:     "test-worker",
		ServiceVersion:  "0.0.0",
		EnableTelemetry: false,
		Logger:          logger,
	})
	require.NoError(t, err, "initialize telemetry")

	healthCheckURL := fmt.Sprintf("http://%s:%s", rabbitContainer.Host, rabbitContainer.MgmtPort)

	conn := &libRabbitMQ.RabbitMQConnection{
		ConnectionStringSource: rabbitContainer.AmqpURL,
		HealthCheckURL:         healthCheckURL,
		Host:                   rabbitContainer.Host,
		Port:                   rabbitContainer.AmqpPort,
		User:                   containers.RabbitUser,
		Pass:                   containers.RabbitPassword,
		Queue:                  containers.QueueGenerateReport,
		Logger:                 logger,
	}

	cr, err := NewConsumerRoutes(conn, 1, logger, telemetry, nil)
	require.NoError(t, err, "NewConsumerRoutes should connect successfully")

	// Override sleepFunc to no-op for fast tests (eliminates real backoff delays)
	cr.retryManager.sleepFunc = func(_ time.Duration) {}

	cr.Register(containers.QueueGenerateReport, handler)

	ctx, cancel := context.WithCancel(context.Background())
	var wg sync.WaitGroup

	err = cr.RunConsumers(ctx, &wg)
	require.NoError(t, err, "RunConsumers should start without error")

	t.Cleanup(func() {
		cancel()

		done := make(chan struct{})
		go func() {
			wg.Wait()
			close(done)
		}()

		select {
		case <-done:
		case <-time.After(10 * time.Second):
			t.Log("Warning: consumer workers did not exit within 10s after cancel")
		}

		if conn.Channel != nil {
			_ = conn.Channel.Close()
		}

		if conn.Connection != nil && !conn.Connection.IsClosed() {
			_ = conn.Connection.Close()
		}
	})
}

// publishTestMessage publishes a message via a fresh AMQP connection.
func publishTestMessage(t *testing.T, exchange, routingKey string, headers amqp.Table, body []byte) {
	t.Helper()

	conn, err := amqp.Dial(rabbitContainer.AmqpURL)
	require.NoError(t, err, "dial for publish")

	defer conn.Close()

	ch, err := conn.Channel()
	require.NoError(t, err, "open channel for publish")

	defer ch.Close()

	err = ch.Publish(exchange, routingKey, false, false,
		amqp.Publishing{
			ContentType:  "application/json",
			DeliveryMode: amqp.Persistent,
			Headers:      headers,
			Body:         body,
		},
	)
	require.NoError(t, err, "publish test message")
}

// waitForDLQMessage polls the DLQ until a message with the given x-request-id is found.
func waitForDLQMessage(t *testing.T, requestID string, timeout time.Duration) amqp.Delivery {
	t.Helper()

	conn, err := amqp.Dial(rabbitContainer.AmqpURL)
	require.NoError(t, err, "dial for DLQ poll")

	defer conn.Close()

	ch, err := conn.Channel()
	require.NoError(t, err, "open channel for DLQ poll")

	defer ch.Close()

	deadline := time.Now().Add(timeout)

	for time.Now().Before(deadline) {
		msg, ok, err := ch.Get(containers.QueueDLQ, false)
		if err != nil {
			time.Sleep(200 * time.Millisecond)
			continue
		}

		if !ok {
			time.Sleep(200 * time.Millisecond)
			continue
		}

		if msgReqID, found := msg.Headers[libConstant.HeaderID]; found {
			if idStr, isStr := msgReqID.(string); isStr && idStr == requestID {
				_ = msg.Ack(false)
				return msg
			}
		}

		_ = msg.Nack(false, true)
		time.Sleep(100 * time.Millisecond)
	}

	t.Fatalf("Timed out waiting for DLQ message with x-request-id=%s (waited %v)", requestID, timeout)

	return amqp.Delivery{}
}

// purgeTestQueues removes all messages from both queues before a test.
func purgeTestQueues(t *testing.T) {
	t.Helper()
	err := rabbitContainer.PurgeQueues()
	require.NoError(t, err, "purge queues before test")
}

// getRetryCountFromHeaders extracts x-retry-count from headers, handling multiple numeric types.
func getRetryCountFromHeaders(t *testing.T, headers amqp.Table) int {
	t.Helper()

	val, exists := headers[pkgConstant.RetryCountHeader]
	if !exists {
		return 0
	}

	switch v := val.(type) {
	case int:
		return v
	case int32:
		return int(v)
	case int64:
		return int(v)
	case float64:
		return int(v)
	default:
		t.Fatalf("unexpected type for %s header: %T", pkgConstant.RetryCountHeader, val)
		return 0
	}
}

// assertNoDLQMessage polls the DLQ briefly and asserts no message with the given
// x-request-id exists. Used to verify a message was NOT routed to DLQ.
func assertNoDLQMessage(t *testing.T, requestID string, pollDuration time.Duration) {
	t.Helper()

	conn, err := amqp.Dial(rabbitContainer.AmqpURL)
	require.NoError(t, err, "dial for DLQ absence check")

	defer conn.Close()

	ch, err := conn.Channel()
	require.NoError(t, err, "open channel for DLQ absence check")

	defer ch.Close()

	deadline := time.Now().Add(pollDuration)

	for time.Now().Before(deadline) {
		msg, ok, err := ch.Get(containers.QueueDLQ, false)
		if err != nil {
			time.Sleep(200 * time.Millisecond)
			continue
		}

		if !ok {
			time.Sleep(200 * time.Millisecond)
			continue
		}

		if msgReqID, found := msg.Headers[libConstant.HeaderID]; found {
			if idStr, isStr := msgReqID.(string); isStr && idStr == requestID {
				t.Logf("Unexpected DLQ message details — headers: %v, body: %s", msg.Headers, string(msg.Body))
				_ = msg.Nack(false, false)
				t.Fatalf("Found unexpected DLQ message with x-request-id=%s", requestID)

				return
			}
		}

		_ = msg.Nack(false, true)
		time.Sleep(100 * time.Millisecond)
	}
}

// TestIntegration_HandleFailedMessage_RetryExhaustion verifies the full retry-then-DLQ
// cycle: handler is invoked exactly MaxMessageRetries+1 times, custom headers survive
// all retries, retry headers are set correctly, and the body is preserved.
func TestIntegration_HandleFailedMessage_RetryExhaustion(t *testing.T) {
	// NOTE: Cannot use t.Parallel() because this test shares a RabbitMQ container.
	purgeTestQueues(t)

	requestID := uuid.New().String()
	customTraceID := uuid.New().String()
	testBody := []byte(`{"report_id":"` + uuid.New().String() + `","test":"retry-exhaustion"}`)

	var invocations atomic.Int32

	handler := func(ctx context.Context, body []byte) error {
		invocations.Add(1)
		return fmt.Errorf("transient network error: connection reset")
	}

	setupConsumer(t, handler)
	time.Sleep(500 * time.Millisecond)

	publishTestMessage(t, containers.ExchangeGenerateReport, containers.RoutingKeyGenerateReport,
		amqp.Table{
			libConstant.HeaderID: requestID,
			"x-custom-test":      "integration-test-value",
			"x-trace-id":         customTraceID,
		}, testBody)

	dlqMsg := waitForDLQMessage(t, requestID, 30*time.Second)

	// Verify exact handler invocation count
	expectedInvocations := int32(pkgConstant.MaxMessageRetries + 1)
	assert.Equal(t, expectedInvocations, invocations.Load(),
		"handler should be invoked exactly %d times (1 original + %d retries)",
		expectedInvocations, pkgConstant.MaxMessageRetries)

	// Verify retry headers
	retryCount := getRetryCountFromHeaders(t, dlqMsg.Headers)
	assert.Equal(t, pkgConstant.MaxMessageRetries, retryCount,
		"DLQ message should have x-retry-count equal to MaxMessageRetries")

	failureReason, ok := dlqMsg.Headers[pkgConstant.RetryFailureReasonHeader].(string)
	assert.True(t, ok, "x-failure-reason header should be a string")
	assert.Equal(t, "retryable_error", failureReason)

	// Verify body preserved
	assert.Equal(t, testBody, dlqMsg.Body, "DLQ message body should match original")

	// Verify original headers preserved through retry cycle
	assert.Equal(t, requestID, dlqMsg.Headers[libConstant.HeaderID])
	assert.Equal(t, "integration-test-value", dlqMsg.Headers["x-custom-test"])
	assert.Equal(t, customTraceID, dlqMsg.Headers["x-trace-id"])
}

// TestIntegration_HandleFailedMessage_NonRetryableError verifies that a non-retryable
// error (ValidationError) causes immediate Nack to DLQ without any republishing.
func TestIntegration_HandleFailedMessage_NonRetryableError(t *testing.T) {
	// NOTE: Cannot use t.Parallel() because this test shares a RabbitMQ container.
	purgeTestQueues(t)

	requestID := uuid.New().String()
	testBody := []byte(`{"report_id":"` + uuid.New().String() + `","test":"non-retryable"}`)

	handler := func(ctx context.Context, body []byte) error {
		return pkg.ValidationError{Code: "VAL-001", Message: "invalid report format"}
	}

	setupConsumer(t, handler)
	time.Sleep(500 * time.Millisecond)

	publishTestMessage(t, containers.ExchangeGenerateReport, containers.RoutingKeyGenerateReport,
		amqp.Table{libConstant.HeaderID: requestID}, testBody)

	dlqMsg := waitForDLQMessage(t, requestID, 15*time.Second)

	_, hasRetryCount := dlqMsg.Headers[pkgConstant.RetryCountHeader]
	assert.False(t, hasRetryCount, "Non-retryable error should NOT add x-retry-count")

	_, hasFailureReason := dlqMsg.Headers[pkgConstant.RetryFailureReasonHeader]
	assert.False(t, hasFailureReason, "Non-retryable error should NOT add x-failure-reason")

	assert.Equal(t, testBody, dlqMsg.Body, "DLQ message body should match original")
}

// TestIntegration_HandleFailedMessage_PreExhaustedRetries verifies that a message
// arriving with x-retry-count already at MaxMessageRetries is immediately Nack'd to DLQ.
func TestIntegration_HandleFailedMessage_PreExhaustedRetries(t *testing.T) {
	// NOTE: Cannot use t.Parallel() because this test shares a RabbitMQ container.
	purgeTestQueues(t)

	requestID := uuid.New().String()
	testBody := []byte(`{"report_id":"` + uuid.New().String() + `","test":"pre-exhausted"}`)

	var invocations atomic.Int32

	handler := func(ctx context.Context, body []byte) error {
		invocations.Add(1)
		return fmt.Errorf("transient error that won't be retried")
	}

	setupConsumer(t, handler)
	time.Sleep(500 * time.Millisecond)

	publishTestMessage(t, containers.ExchangeGenerateReport, containers.RoutingKeyGenerateReport,
		amqp.Table{
			libConstant.HeaderID:                 requestID,
			pkgConstant.RetryCountHeader:         int32(pkgConstant.MaxMessageRetries),
			pkgConstant.RetryFailureReasonHeader: "previous transient error",
		}, testBody)

	dlqMsg := waitForDLQMessage(t, requestID, 15*time.Second)

	assert.Equal(t, int32(1), invocations.Load(),
		"handler should be invoked exactly once for pre-exhausted message")

	retryCount := getRetryCountFromHeaders(t, dlqMsg.Headers)
	assert.Equal(t, pkgConstant.MaxMessageRetries, retryCount,
		"should still be %d, no extra republish", pkgConstant.MaxMessageRetries)

	assert.Equal(t, testBody, dlqMsg.Body, "DLQ message body should match original")
}

// TestIntegration_HandleFailedMessage_RetryThenSucceed verifies that when a handler
// succeeds on a retry attempt, the message is Ack'd and does NOT reach the DLQ.
func TestIntegration_HandleFailedMessage_RetryThenSucceed(t *testing.T) {
	// NOTE: Cannot use t.Parallel() because this test shares a RabbitMQ container.
	purgeTestQueues(t)

	requestID := uuid.New().String()
	testBody := []byte(`{"report_id":"` + uuid.New().String() + `","test":"retry-then-succeed"}`)

	const succeedOnAttempt = 3

	var invocations atomic.Int32

	handler := func(ctx context.Context, body []byte) error {
		count := invocations.Add(1)
		if count >= succeedOnAttempt {
			return nil
		}

		return fmt.Errorf("transient error: attempt %d", count)
	}

	setupConsumer(t, handler)
	time.Sleep(500 * time.Millisecond)

	publishTestMessage(t, containers.ExchangeGenerateReport, containers.RoutingKeyGenerateReport,
		amqp.Table{libConstant.HeaderID: requestID}, testBody)

	// Wait for the handler to be invoked the expected number of times
	deadline := time.Now().Add(15 * time.Second)

	for time.Now().Before(deadline) {
		if invocations.Load() >= succeedOnAttempt {
			break
		}

		time.Sleep(100 * time.Millisecond)
	}

	assert.Equal(t, int32(succeedOnAttempt), invocations.Load(),
		"handler should be invoked exactly %d times", succeedOnAttempt)

	// Verify message did NOT reach DLQ (poll for 3s to be sure)
	assertNoDLQMessage(t, requestID, 3*time.Second)
}
