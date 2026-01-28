//go:build integration

package rabbitmq

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	libCircuitBreaker "github.com/LerianStudio/lib-commons/v2/commons/circuitbreaker"
	libRabbitmq "github.com/LerianStudio/lib-commons/v2/commons/rabbitmq"
	libZap "github.com/LerianStudio/lib-commons/v2/commons/zap"
	rmqtestutil "github.com/LerianStudio/midaz/v3/tests/utils/rabbitmq"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// circuitBreakerTestMessage represents a test message for circuit breaker tests.
type circuitBreakerTestMessage struct {
	ID        string    `json:"id"`
	Timestamp time.Time `json:"timestamp"`
	Data      string    `json:"data"`
}

// TestIntegration_CircuitBreaker_NormalOperation tests that circuit breaker
// allows messages through when RabbitMQ is healthy.
func TestIntegration_CircuitBreaker_NormalOperation(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	rmqContainer := rmqtestutil.SetupContainer(t)

	exchange := "cb-test-exchange"
	routingKey := "cb.test.key"
	queue := "cb-test-queue"

	rmqtestutil.SetupExchange(t, rmqContainer.Channel, exchange, "topic")
	rmqtestutil.SetupQueue(t, rmqContainer.Channel, queue, exchange, routingKey)

	logger, err := libZap.InitializeLoggerWithError()
	require.NoError(t, err)
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

	baseProducer := NewProducerRabbitMQ(conn)
	cbManager := libCircuitBreaker.NewManager(logger)
	cb := cbManager.GetOrCreate("rabbitmq-integration-test", libCircuitBreaker.DefaultConfig())
	producer := NewProducerCircuitBreaker(baseProducer, cb)

	ctx := context.Background()

	msg := circuitBreakerTestMessage{
		ID:        uuid.New().String(),
		Timestamp: time.Now(),
		Data:      "Circuit breaker test message",
	}
	msgBytes, err := json.Marshal(msg)
	require.NoError(t, err)

	_, err = producer.ProducerDefault(ctx, exchange, routingKey, msgBytes)
	require.NoError(t, err, "publish through circuit breaker should succeed")

	state := cb.State()
	assert.Equal(t, libCircuitBreaker.StateClosed, state, "circuit should remain closed")

	require.Eventually(t, func() bool {
		return rmqtestutil.GetQueueMessageCount(t, rmqContainer.Channel, queue) == 1
	}, 5*time.Second, 100*time.Millisecond, "message should arrive in queue")

	t.Log("Integration test passed: circuit breaker normal operation verified")
}

// TestIntegration_CircuitBreaker_FastFail tests that circuit breaker
// returns errors immediately when circuit is open.
func TestIntegration_CircuitBreaker_FastFail(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	rmqContainer := rmqtestutil.SetupContainer(t)

	exchange := "cb-fastfail-exchange"
	routingKey := "cb.fastfail.key"
	queue := "cb-fastfail-queue"

	rmqtestutil.SetupExchange(t, rmqContainer.Channel, exchange, "topic")
	rmqtestutil.SetupQueue(t, rmqContainer.Channel, queue, exchange, routingKey)

	logger, err := libZap.InitializeLoggerWithError()
	require.NoError(t, err)
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

	baseProducer := NewProducerRabbitMQ(conn)
	cbManager := libCircuitBreaker.NewManager(logger)
	aggressiveConfig := libCircuitBreaker.Config{
		MaxRequests:         1,
		Interval:            1 * time.Minute,
		Timeout:             30 * time.Second,
		ConsecutiveFailures: 2,
		FailureRatio:        0.3,
		MinRequests:         1,
	}
	cb := cbManager.GetOrCreate("rabbitmq-fastfail-test", aggressiveConfig)
	producer := NewProducerCircuitBreaker(baseProducer, cb)

	ctx := context.Background()

	msg := circuitBreakerTestMessage{
		ID:        uuid.New().String(),
		Timestamp: time.Now(),
		Data:      "Initial message",
	}
	msgBytes, err := json.Marshal(msg)
	require.NoError(t, err, "should marshal message")

	_, err = producer.ProducerDefault(ctx, exchange, routingKey, msgBytes)
	require.NoError(t, err, "initial publish should succeed")

	t.Log("Stopping RabbitMQ container to trigger circuit open...")
	err = rmqContainer.Container.Stop(ctx, nil)
	require.NoError(t, err, "should be able to stop container")

	// Intentionally ignoring errors to trigger circuit opening in test
	for i := 0; i < 3; i++ {
		_, _ = producer.ProducerDefault(ctx, exchange, routingKey, msgBytes)
		t.Logf("Attempt %d - circuit state: %s", i+1, cb.State())
	}

	state := cb.State()
	assert.Equal(t, libCircuitBreaker.StateOpen, state, "circuit should be open after failures")

	start := time.Now()
	_, err = producer.ProducerDefault(ctx, exchange, routingKey, msgBytes)
	fastFailDuration := time.Since(start)

	assert.Error(t, err, "should return error when circuit is open")
	assert.Less(t, fastFailDuration, 100*time.Millisecond, "fast-fail should be <100ms")

	t.Logf("Fast-fail duration: %v", fastFailDuration)
	t.Log("Integration test passed: circuit breaker fast-fail verified")
}

// TestIntegration_CircuitBreaker_Recovery tests that circuit breaker
// recovers when RabbitMQ becomes available again (with manual reset).
func TestIntegration_CircuitBreaker_Recovery(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	rmqContainer := rmqtestutil.SetupContainer(t)

	exchange := "cb-recovery-exchange"
	routingKey := "cb.recovery.key"
	queue := "cb-recovery-queue"

	rmqtestutil.SetupExchange(t, rmqContainer.Channel, exchange, "topic")
	rmqtestutil.SetupQueue(t, rmqContainer.Channel, queue, exchange, routingKey)

	logger, err := libZap.InitializeLoggerWithError()
	require.NoError(t, err)
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

	baseProducer := NewProducerRabbitMQ(conn)
	cbManager := libCircuitBreaker.NewManager(logger)
	recoveryConfig := libCircuitBreaker.Config{
		MaxRequests:         3,
		Interval:            1 * time.Minute,
		Timeout:             2 * time.Second,
		ConsecutiveFailures: 2,
		FailureRatio:        0.3,
		MinRequests:         1,
	}
	cb := cbManager.GetOrCreate("rabbitmq-recovery-test", recoveryConfig)
	producer := NewProducerCircuitBreaker(baseProducer, cb)

	ctx := context.Background()

	msg := circuitBreakerTestMessage{
		ID:        uuid.New().String(),
		Timestamp: time.Now(),
		Data:      "Recovery test message",
	}
	msgBytes, err := json.Marshal(msg)
	require.NoError(t, err, "should marshal message")

	_, err = producer.ProducerDefault(ctx, exchange, routingKey, msgBytes)
	require.NoError(t, err, "initial publish should succeed")
	t.Log("Initial message published successfully")

	t.Log("Stopping RabbitMQ container...")
	err = rmqContainer.Container.Stop(ctx, nil)
	require.NoError(t, err)

	for i := 0; i < 3; i++ {
		_, _ = producer.ProducerDefault(ctx, exchange, routingKey, msgBytes)
	}

	assert.Equal(t, libCircuitBreaker.StateOpen, cb.State(), "circuit should be open")
	t.Log("Circuit is now open")

	t.Log("Waiting for circuit to transition to half-open...")
	require.Eventually(t, func() bool {
		return cb.State() == libCircuitBreaker.StateHalfOpen
	}, 10*time.Second, 200*time.Millisecond, "circuit should transition to half-open")

	t.Log("Restarting RabbitMQ container...")
	err = rmqContainer.Container.Start(ctx)
	require.NoError(t, err)

	require.Eventually(t, func() bool {
		return rmqtestutil.IsRabbitMQHealthy(rmqContainer.Host, rmqContainer.MgmtPort)
	}, 30*time.Second, 500*time.Millisecond, "RabbitMQ should become healthy")

	cbManager.Reset("rabbitmq-recovery-test")
	t.Log("Circuit breaker reset")

	state := cb.State()
	assert.Equal(t, libCircuitBreaker.StateClosed, state, "circuit should be closed after reset")

	t.Log("Integration test passed: circuit breaker recovery verified")
}

// TestIntegration_CircuitBreaker_NaturalRecovery tests that circuit breaker
// naturally recovers from Open -> Half-Open -> Closed without calling Reset().
func TestIntegration_CircuitBreaker_NaturalRecovery(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	rmqContainer := rmqtestutil.SetupContainer(t)

	exchange := "cb-natural-recovery-exchange"
	routingKey := "cb.natural.recovery.key"
	queue := "cb-natural-recovery-queue"

	rmqtestutil.SetupExchange(t, rmqContainer.Channel, exchange, "topic")
	rmqtestutil.SetupQueue(t, rmqContainer.Channel, queue, exchange, routingKey)

	logger, err := libZap.InitializeLoggerWithError()
	require.NoError(t, err)
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

	baseProducer := NewProducerRabbitMQ(conn)
	cbManager := libCircuitBreaker.NewManager(logger)
	naturalRecoveryConfig := libCircuitBreaker.Config{
		MaxRequests:         2,
		Interval:            1 * time.Minute,
		Timeout:             3 * time.Second,
		ConsecutiveFailures: 2,
		FailureRatio:        0.3,
		MinRequests:         1,
	}
	cb := cbManager.GetOrCreate("rabbitmq-natural-recovery-test", naturalRecoveryConfig)
	producer := NewProducerCircuitBreaker(baseProducer, cb)

	ctx := context.Background()

	msg := circuitBreakerTestMessage{
		ID:        uuid.New().String(),
		Timestamp: time.Now(),
		Data:      "Natural recovery test message",
	}
	msgBytes, err := json.Marshal(msg)
	require.NoError(t, err, "should marshal message")

	_, err = producer.ProducerDefault(ctx, exchange, routingKey, msgBytes)
	require.NoError(t, err, "initial publish should succeed")
	t.Log("Initial message published successfully")

	t.Log("Stopping RabbitMQ container to trigger failures...")
	err = rmqContainer.Container.Stop(ctx, nil)
	require.NoError(t, err)

	for i := 0; i < 3; i++ {
		_, _ = producer.ProducerDefault(ctx, exchange, routingKey, msgBytes)
	}

	assert.Equal(t, libCircuitBreaker.StateOpen, cb.State(), "circuit should be open")
	t.Log("Circuit is now open")

	t.Log("Restarting RabbitMQ container...")
	err = rmqContainer.Container.Start(ctx)
	require.NoError(t, err)

	require.Eventually(t, func() bool {
		return rmqtestutil.IsRabbitMQHealthy(rmqContainer.Host, rmqContainer.MgmtPort)
	}, 30*time.Second, 500*time.Millisecond, "RabbitMQ should become healthy")

	t.Logf("Waiting for circuit timeout (%v) to transition to half-open...", naturalRecoveryConfig.Timeout)
	time.Sleep(naturalRecoveryConfig.Timeout + 1*time.Second)

	newURI := fmt.Sprintf("amqp://%s:%s@%s:%s/",
		rmqtestutil.DefaultUser, rmqtestutil.DefaultPassword,
		rmqContainer.Host, rmqContainer.AMQPPort)

	newConn := &libRabbitmq.RabbitMQConnection{
		ConnectionStringSource: newURI,
		HealthCheckURL:         healthCheckURL,
		Host:                   rmqContainer.Host,
		Port:                   rmqContainer.AMQPPort,
		User:                   rmqtestutil.DefaultUser,
		Pass:                   rmqtestutil.DefaultPassword,
		Logger:                 logger,
	}

	newBaseProducer := NewProducerRabbitMQ(newConn)
	newProducer := NewProducerCircuitBreaker(newBaseProducer, cb)

	newCh := rmqtestutil.CreateChannelWithRetry(t, newURI, 30*time.Second)
	rmqtestutil.SetupExchange(t, newCh, exchange, "topic")
	rmqtestutil.SetupQueue(t, newCh, queue, exchange, routingKey)

	t.Log("Attempting requests to allow circuit to recover naturally...")
	var successCount int
	for i := 0; i < int(naturalRecoveryConfig.MaxRequests)+1; i++ {
		msg.ID = uuid.New().String()
		msg.Timestamp = time.Now()
		msgBytes, err = json.Marshal(msg)
		require.NoError(t, err, "should marshal message")

		_, err = newProducer.ProducerDefault(ctx, exchange, routingKey, msgBytes)
		if err == nil {
			successCount++
			t.Logf("Request %d succeeded, circuit state: %s", i+1, cb.State())
		} else {
			t.Logf("Request %d failed: %v, circuit state: %s", i+1, err, cb.State())
		}

		time.Sleep(100 * time.Millisecond)
	}

	state := cb.State()
	t.Logf("Final circuit state: %s, successful requests: %d", state, successCount)

	assert.Equal(t, libCircuitBreaker.StateClosed, state, "circuit should naturally recover to closed state")
	assert.GreaterOrEqual(t, successCount, int(naturalRecoveryConfig.MaxRequests),
		"should have enough successful requests to close circuit")

	t.Log("Integration test passed: circuit breaker natural recovery verified")
}
