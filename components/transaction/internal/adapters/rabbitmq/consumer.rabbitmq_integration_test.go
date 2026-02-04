//go:build integration

package rabbitmq

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	libOpentelemetry "github.com/LerianStudio/lib-commons/v2/commons/opentelemetry"
	libRabbitmq "github.com/LerianStudio/lib-commons/v2/commons/rabbitmq"
	libZap "github.com/LerianStudio/lib-commons/v2/commons/zap"
	"github.com/LerianStudio/midaz/v3/tests/utils/chaos"
	rmqtestutil "github.com/LerianStudio/midaz/v3/tests/utils/rabbitmq"

	"github.com/google/uuid"
	amqp "github.com/rabbitmq/amqp091-go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

// Note: skipIfNotChaos is defined in producer.rabbitmq_integration_test.go

// Fixed ports for container restart chaos test (non-standard to avoid conflicts)
const (
	fixedAMQPPort = "35672"
	fixedMgmtPort = "35673"
)

// =============================================================================
// CONTAINER HELPERS
// =============================================================================

// setupContainerWithFixedPorts creates a RabbitMQ container with fixed port bindings.
// This is necessary for chaos tests that restart containers, as testcontainers normally
// assigns new random ports after restart. With fixed ports, the consumer can reconnect.
func setupContainerWithFixedPorts(t *testing.T, amqpPort, mgmtPort string) *rmqtestutil.ContainerResult {
	t.Helper()

	ctx := context.Background()

	// Use PortBindings to bind specific host ports
	req := testcontainers.ContainerRequest{
		Image:        "rabbitmq:4.1-management-alpine",
		ExposedPorts: []string{amqpPort + ":5672/tcp", mgmtPort + ":15672/tcp"},
		Env: map[string]string{
			"RABBITMQ_DEFAULT_USER": rmqtestutil.DefaultUser,
			"RABBITMQ_DEFAULT_PASS": rmqtestutil.DefaultPassword,
		},
		WaitingFor: wait.ForAll(
			wait.ForLog("Server startup complete").WithStartupTimeout(120*time.Second),
			wait.ForHTTP("/api/health/checks/alarms").
				WithPort("15672/tcp").
				WithBasicAuth(rmqtestutil.DefaultUser, rmqtestutil.DefaultPassword).
				WithStartupTimeout(60*time.Second),
		),
	}

	container, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: req,
		Started:          true,
	})
	require.NoError(t, err, "failed to start RabbitMQ container with fixed ports")

	host, err := container.Host(ctx)
	require.NoError(t, err, "failed to get RabbitMQ container host")

	uri := fmt.Sprintf("amqp://%s:%s@%s:%s/",
		rmqtestutil.DefaultUser, rmqtestutil.DefaultPassword, host, amqpPort)

	conn, err := amqp.Dial(uri)
	require.NoError(t, err, "failed to connect to RabbitMQ container")

	ch, err := conn.Channel()
	require.NoError(t, err, "failed to open RabbitMQ channel")

	t.Cleanup(func() {
		if ch != nil {
			ch.Close()
		}
		if conn != nil {
			conn.Close()
		}
		if err := container.Terminate(context.Background()); err != nil {
			t.Logf("failed to terminate RabbitMQ container: %v", err)
		}
	})

	return &rmqtestutil.ContainerResult{
		Container: container,
		Conn:      conn,
		Channel:   ch,
		Host:      host,
		AMQPPort:  amqpPort,
		MgmtPort:  mgmtPort,
		URI:       uri,
	}
}

// =============================================================================
// TEST INFRASTRUCTURE
// =============================================================================

// consumerTestInfra holds the infrastructure needed for RabbitMQ consumer integration tests.
type consumerTestInfra struct {
	rmqContainer *rmqtestutil.ContainerResult
	conn         *libRabbitmq.RabbitMQConnection
	consumer     *ConsumerRoutes
	producer     *ProducerRabbitMQRepository
	exchange     string
	routingKey   string
	queue        string
}

// consumerChaosTestInfra holds the infrastructure needed for RabbitMQ consumer chaos tests.
type consumerChaosTestInfra struct {
	rmqContainer *rmqtestutil.ContainerResult
	conn         *libRabbitmq.RabbitMQConnection
	consumer     *ConsumerRoutes
	producer     *ProducerRabbitMQRepository
	chaosOrch    *chaos.Orchestrator
	exchange     string
	routingKey   string
	queue        string
}

// consumerNetworkChaosTestInfra holds infrastructure for network chaos tests with Toxiproxy.
type consumerNetworkChaosTestInfra struct {
	rmqContainer  *rmqtestutil.ContainerResult
	chaosInfra    *chaos.Infrastructure
	proxyConn     *libRabbitmq.RabbitMQConnection
	proxyConsumer *ConsumerRoutes
	proxyProducer *ProducerRabbitMQRepository
	proxy         *chaos.Proxy
	exchange      string
	routingKey    string
	queue         string
}

// setupConsumerInfra sets up the test infrastructure for RabbitMQ consumer integration testing.
func setupConsumerInfra(t *testing.T, numWorkers, prefetch int) *consumerTestInfra {
	t.Helper()

	// Setup RabbitMQ container
	rmqContainer := rmqtestutil.SetupContainer(t)

	// Setup exchange and queue
	exchange := "test-consumer-exchange"
	routingKey := "test.consumer.key"
	queue := "test-consumer-queue"

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

	// Create telemetry for consumer (empty struct is sufficient for tests)
	telemetry := &libOpentelemetry.Telemetry{}

	// Create consumer routes
	consumer := NewConsumerRoutes(conn, numWorkers, prefetch, logger, telemetry)

	// Create producer for publishing test messages
	producer, err := NewProducerRabbitMQ(conn)
	require.NoError(t, err, "failed to create producer")

	return &consumerTestInfra{
		rmqContainer: rmqContainer,
		conn:         conn,
		consumer:     consumer,
		producer:     producer,
		exchange:     exchange,
		routingKey:   routingKey,
		queue:        queue,
	}
}

// setupConsumerChaosInfra sets up the test infrastructure for RabbitMQ consumer chaos testing.
// Uses fixed ports so the consumer can reconnect after container restart.
func setupConsumerChaosInfra(t *testing.T, numWorkers, prefetch int) *consumerChaosTestInfra {
	t.Helper()

	// Setup RabbitMQ container with FIXED PORTS (critical for restart tests)
	rmqContainer := setupContainerWithFixedPorts(t, fixedAMQPPort, fixedMgmtPort)

	// Setup exchange and queue
	exchange := "test-consumer-exchange"
	routingKey := "test.consumer.key"
	queue := "test-consumer-queue"

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

	// Create telemetry for consumer (empty struct is sufficient for tests)
	telemetry := &libOpentelemetry.Telemetry{}

	// Create consumer routes
	consumer := NewConsumerRoutes(conn, numWorkers, prefetch, logger, telemetry)

	// Create producer for publishing test messages
	producer, err := NewProducerRabbitMQ(conn)
	require.NoError(t, err, "failed to create producer")

	// Create chaos orchestrator
	chaosOrch := chaos.NewOrchestrator(t)

	return &consumerChaosTestInfra{
		rmqContainer: rmqContainer,
		conn:         conn,
		consumer:     consumer,
		producer:     producer,
		chaosOrch:    chaosOrch,
		exchange:     exchange,
		routingKey:   routingKey,
		queue:        queue,
	}
}

// setupConsumerNetworkChaosInfra sets up infrastructure for network chaos testing with Toxiproxy.
func setupConsumerNetworkChaosInfra(t *testing.T, numWorkers, prefetch int) *consumerNetworkChaosTestInfra {
	t.Helper()

	// Setup exchange and queue names
	exchange := "test-consumer-exchange"
	routingKey := "test.consumer.key"
	queue := "test-consumer-queue"

	// 1. Create chaos infrastructure (creates network + Toxiproxy)
	chaosInfra := chaos.NewInfrastructure(t)

	// 2. Create RabbitMQ container
	rmqContainer := rmqtestutil.SetupContainer(t)

	// 3. Setup exchange and queue
	rmqtestutil.SetupExchange(t, rmqContainer.Channel, exchange, "topic")
	rmqtestutil.SetupQueue(t, rmqContainer.Channel, queue, exchange, routingKey)

	// 4. Register RabbitMQ container with infrastructure for proxy creation
	_, err := chaosInfra.RegisterContainerWithPort("rabbitmq", rmqContainer.Container, "5672/tcp")
	require.NoError(t, err, "failed to register RabbitMQ container")

	// 5. Create proxy for RabbitMQ
	proxy, err := chaosInfra.CreateProxyFor("rabbitmq", "8667/tcp")
	require.NoError(t, err, "failed to create Toxiproxy proxy for RabbitMQ")

	// 6. Get proxy address for client connections
	containerInfo, ok := chaosInfra.GetContainer("rabbitmq")
	require.True(t, ok, "RabbitMQ container should be registered")
	require.NotEmpty(t, containerInfo.ProxyListen, "proxy address should be set")

	proxyAddr := containerInfo.ProxyListen

	// 7. Create connection through the proxy
	proxyURI := fmt.Sprintf("amqp://%s:%s@%s/",
		rmqtestutil.DefaultUser,
		rmqtestutil.DefaultPassword,
		proxyAddr,
	)

	logger := libZap.InitializeLogger()
	healthCheckURL := "http://" + rmqContainer.Host + ":" + rmqContainer.MgmtPort
	proxyConn := &libRabbitmq.RabbitMQConnection{
		ConnectionStringSource: proxyURI,
		HealthCheckURL:         healthCheckURL,
		Host:                   chaosInfra.ToxiproxyHost(),
		Port:                   "15672",
		User:                   rmqtestutil.DefaultUser,
		Pass:                   rmqtestutil.DefaultPassword,
		Logger:                 logger,
	}

	// Create telemetry for consumer (empty struct is sufficient for tests)
	telemetry := &libOpentelemetry.Telemetry{}

	// Create consumer through proxy
	proxyConsumer := NewConsumerRoutes(proxyConn, numWorkers, prefetch, logger, telemetry)

	// Create producer through proxy
	proxyProducer, err := NewProducerRabbitMQ(proxyConn)
	require.NoError(t, err, "failed to create proxy producer")

	return &consumerNetworkChaosTestInfra{
		rmqContainer:  rmqContainer,
		chaosInfra:    chaosInfra,
		proxyConn:     proxyConn,
		proxyConsumer: proxyConsumer,
		proxyProducer: proxyProducer,
		proxy:         proxy,
		exchange:      exchange,
		routingKey:    routingKey,
		queue:         queue,
	}
}

// cleanup releases all resources for chaos tests.
func (infra *consumerChaosTestInfra) cleanup() {
	if infra.chaosOrch != nil {
		infra.chaosOrch.Close()
	}
}

// cleanup releases all resources for network chaos infrastructure.
func (infra *consumerNetworkChaosTestInfra) cleanup() {
	if infra.chaosInfra != nil {
		infra.chaosInfra.Cleanup()
	}
}

// recreateChannelForInspection creates a NEW AMQP channel for queue inspection after container restart.
func (infra *consumerChaosTestInfra) recreateChannelForInspection(t *testing.T) {
	t.Helper()

	ctx := context.Background()

	// Get the NEW port assigned after container restart
	newAMQPPort, err := infra.rmqContainer.Container.MappedPort(ctx, "5672")
	require.NoError(t, err, "should get new AMQP port after restart")

	newMgmtPort, err := infra.rmqContainer.Container.MappedPort(ctx, "15672")
	require.NoError(t, err, "should get new management port after restart")

	// Update container result with new ports
	infra.rmqContainer.AMQPPort = newAMQPPort.Port()
	infra.rmqContainer.MgmtPort = newMgmtPort.Port()
	infra.rmqContainer.URI = fmt.Sprintf("amqp://%s:%s@%s:%s/",
		rmqtestutil.DefaultUser,
		rmqtestutil.DefaultPassword,
		infra.rmqContainer.Host,
		infra.rmqContainer.AMQPPort,
	)

	// Create a fresh AMQP connection and channel for inspection
	newChannel := rmqtestutil.CreateChannelWithRetry(t, infra.rmqContainer.URI, 30*time.Second)
	infra.rmqContainer.Channel = newChannel

	t.Logf("Created new channel for inspection (port changed to %s)", infra.rmqContainer.AMQPPort)
}

// consumerTestMessage represents a test message for consumer integration testing.
type consumerTestMessage struct {
	ID        string    `json:"id"`
	Timestamp time.Time `json:"timestamp"`
	Data      string    `json:"data"`
	Sequence  int       `json:"sequence,omitempty"`
}

// publishTestMessage publishes a test message and returns its ID.
func publishTestMessage(t *testing.T, ctx context.Context, producer *ProducerRabbitMQRepository, exchange, routingKey, data string) string {
	t.Helper()

	msg := consumerTestMessage{
		ID:        uuid.New().String(),
		Timestamp: time.Now(),
		Data:      data,
	}
	msgBytes, err := json.Marshal(msg)
	require.NoError(t, err)

	_, err = producer.ProducerDefault(ctx, exchange, routingKey, msgBytes)
	require.NoError(t, err, "publish should succeed")

	return msg.ID
}

// publishTestMessageDirect publishes directly to the queue channel (for precise control).
func publishTestMessageDirect(t *testing.T, channel *amqp.Channel, exchange, routingKey string, msg consumerTestMessage) {
	t.Helper()

	msgBytes, err := json.Marshal(msg)
	require.NoError(t, err)

	err = channel.Publish(
		exchange,
		routingKey,
		false,
		false,
		amqp.Publishing{
			ContentType: "application/json",
			Body:        msgBytes,
		},
	)
	require.NoError(t, err, "direct publish should succeed")
}

// =============================================================================
// INTEGRATION TESTS - BASIC OPERATIONS
// =============================================================================

// TestIntegration_NewConsumerRoutes_PanicOnConnectionFailure tests that the constructor
// panics when RabbitMQ connection fails. This is an integration test because it makes
// a real network dial attempt to an invalid address.
func TestIntegration_NewConsumerRoutes_PanicOnConnectionFailure(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	// Create an invalid connection that will fail
	logger := libZap.InitializeLogger()
	conn := &libRabbitmq.RabbitMQConnection{
		ConnectionStringSource: "amqp://invalid:invalid@localhost:99999/",
		Logger:                 logger,
	}

	// The constructor should panic when connection fails
	assert.Panics(t, func() {
		NewConsumerRoutes(conn, 5, 10, logger, nil)
	}, "NewConsumerRoutes should panic when RabbitMQ connection fails")
}

// TestIntegration_Consumer_BasicMessageConsumption tests that the consumer receives
// messages from the queue and invokes the registered handler.
func TestIntegration_Consumer_BasicMessageConsumption(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	infra := setupConsumerInfra(t, 1, 10)

	ctx := context.Background()

	// Track received messages
	var receivedMu sync.Mutex
	receivedMessages := make([]consumerTestMessage, 0)

	// Register handler that records received messages
	infra.consumer.Register(infra.queue, func(ctx context.Context, body []byte) error {
		var msg consumerTestMessage
		if err := json.Unmarshal(body, &msg); err != nil {
			return err
		}

		receivedMu.Lock()
		receivedMessages = append(receivedMessages, msg)
		receivedMu.Unlock()

		t.Logf("Handler received message: %s", msg.ID)
		return nil
	})

	// Start consumers
	err := infra.consumer.RunConsumers()
	require.NoError(t, err, "RunConsumers should succeed")

	// Give consumer time to start
	time.Sleep(500 * time.Millisecond)

	// Publish test messages
	numMessages := 5
	publishedIDs := make([]string, numMessages)
	for i := 0; i < numMessages; i++ {
		publishedIDs[i] = publishTestMessage(t, ctx, infra.producer, infra.exchange, infra.routingKey,
			fmt.Sprintf("Test message %d", i+1))
	}
	t.Logf("Published %d messages", numMessages)

	// Wait for messages to be consumed
	require.Eventually(t, func() bool {
		receivedMu.Lock()
		defer receivedMu.Unlock()
		return len(receivedMessages) >= numMessages
	}, 10*time.Second, 100*time.Millisecond, "all messages should be consumed")

	// Verify all messages were received
	receivedMu.Lock()
	assert.Len(t, receivedMessages, numMessages, "should receive all published messages")
	receivedMu.Unlock()

	// Verify queue is empty (messages were acknowledged)
	time.Sleep(500 * time.Millisecond)
	msgCount := rmqtestutil.GetQueueMessageCount(t, infra.rmqContainer.Channel, infra.queue)
	assert.Equal(t, 0, msgCount, "queue should be empty after consumption")

	t.Log("Integration test passed: basic message consumption verified")
}

// TestIntegration_Consumer_HandlerErrorCausesNack tests that when a handler returns
// an error, the message is Nack'd and requeued.
func TestIntegration_Consumer_HandlerErrorCausesNack(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	infra := setupConsumerInfra(t, 1, 10)

	ctx := context.Background()

	// Track processing attempts
	var attemptsMu sync.Mutex
	attempts := make(map[string]int)
	successAfterRetry := make(chan string, 1)

	// Register handler that fails on first attempt, succeeds on retry
	infra.consumer.Register(infra.queue, func(ctx context.Context, body []byte) error {
		var msg consumerTestMessage
		if err := json.Unmarshal(body, &msg); err != nil {
			return err
		}

		attemptsMu.Lock()
		attempts[msg.ID]++
		currentAttempt := attempts[msg.ID]
		attemptsMu.Unlock()

		t.Logf("Handler processing message %s (attempt %d)", msg.ID, currentAttempt)

		if currentAttempt == 1 {
			// First attempt - fail
			return fmt.Errorf("simulated failure on first attempt")
		}

		// Second attempt - succeed
		successAfterRetry <- msg.ID
		return nil
	})

	// Start consumers
	err := infra.consumer.RunConsumers()
	require.NoError(t, err, "RunConsumers should succeed")

	// Give consumer time to start
	time.Sleep(500 * time.Millisecond)

	// Publish a single test message
	msgID := publishTestMessage(t, ctx, infra.producer, infra.exchange, infra.routingKey, "Test message for retry")
	t.Logf("Published message: %s", msgID)

	// Wait for successful retry
	select {
	case receivedID := <-successAfterRetry:
		assert.Equal(t, msgID, receivedID, "should receive the same message after retry")
	case <-time.After(15 * time.Second):
		t.Fatal("timeout waiting for message to be processed after retry")
	}

	// Verify message was processed twice (initial + retry)
	attemptsMu.Lock()
	assert.GreaterOrEqual(t, attempts[msgID], 2, "message should have been processed at least twice")
	attemptsMu.Unlock()

	t.Log("Integration test passed: handler error causes Nack and requeue")
}

// TestIntegration_Consumer_MultipleWorkers tests that multiple workers process
// messages concurrently.
func TestIntegration_Consumer_MultipleWorkers(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	numWorkers := 5
	infra := setupConsumerInfra(t, numWorkers, 10)

	ctx := context.Background()

	// Track concurrent processing
	var activeWorkers int32
	var maxConcurrent int32
	var processedCount int32
	var wg sync.WaitGroup

	numMessages := 20
	wg.Add(numMessages)

	// Register handler that simulates work and tracks concurrency
	infra.consumer.Register(infra.queue, func(ctx context.Context, body []byte) error {
		// Increment active workers
		current := atomic.AddInt32(&activeWorkers, 1)

		// Track max concurrency
		for {
			old := atomic.LoadInt32(&maxConcurrent)
			if current <= old || atomic.CompareAndSwapInt32(&maxConcurrent, old, current) {
				break
			}
		}

		// Simulate work
		time.Sleep(100 * time.Millisecond)

		// Decrement active workers
		atomic.AddInt32(&activeWorkers, -1)
		atomic.AddInt32(&processedCount, 1)
		wg.Done()

		return nil
	})

	// Start consumers
	err := infra.consumer.RunConsumers()
	require.NoError(t, err, "RunConsumers should succeed")

	// Give consumer time to start
	time.Sleep(500 * time.Millisecond)

	// Publish messages rapidly
	for i := 0; i < numMessages; i++ {
		publishTestMessage(t, ctx, infra.producer, infra.exchange, infra.routingKey,
			fmt.Sprintf("Concurrent message %d", i+1))
	}
	t.Logf("Published %d messages", numMessages)

	// Wait for all messages to be processed
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		t.Logf("All messages processed")
	case <-time.After(30 * time.Second):
		t.Fatalf("timeout: processed %d/%d messages", atomic.LoadInt32(&processedCount), numMessages)
	}

	// Verify concurrency
	maxConc := atomic.LoadInt32(&maxConcurrent)
	t.Logf("Max concurrent workers observed: %d (configured: %d)", maxConc, numWorkers)

	assert.Greater(t, maxConc, int32(1), "should have multiple workers processing concurrently")
	assert.LessOrEqual(t, maxConc, int32(numWorkers), "should not exceed configured workers")

	t.Log("Integration test passed: multiple workers process messages concurrently")
}

// TestIntegration_Consumer_ReconnectionOnChannelClose tests that consumers reconnect
// automatically when the channel is closed.
func TestIntegration_Consumer_ReconnectionOnChannelClose(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	infra := setupConsumerInfra(t, 1, 10)

	ctx := context.Background()

	// Track received messages with phase information
	var receivedMu sync.Mutex
	phase1Messages := make([]string, 0)
	phase2Messages := make([]string, 0)
	currentPhase := 1

	// Register handler
	infra.consumer.Register(infra.queue, func(ctx context.Context, body []byte) error {
		var msg consumerTestMessage
		if err := json.Unmarshal(body, &msg); err != nil {
			return err
		}

		receivedMu.Lock()
		if currentPhase == 1 {
			phase1Messages = append(phase1Messages, msg.ID)
		} else {
			phase2Messages = append(phase2Messages, msg.ID)
		}
		receivedMu.Unlock()

		t.Logf("Handler received message in phase %d: %s", currentPhase, msg.ID)
		return nil
	})

	// Start consumers
	err := infra.consumer.RunConsumers()
	require.NoError(t, err, "RunConsumers should succeed")

	// Give consumer time to start
	time.Sleep(500 * time.Millisecond)

	// Phase 1: Publish and consume messages
	t.Log("Phase 1: Publishing messages before channel close")
	for i := 0; i < 3; i++ {
		publishTestMessage(t, ctx, infra.producer, infra.exchange, infra.routingKey, "Phase 1 message")
	}

	require.Eventually(t, func() bool {
		receivedMu.Lock()
		defer receivedMu.Unlock()
		return len(phase1Messages) >= 3
	}, 10*time.Second, 100*time.Millisecond, "phase 1 messages should be consumed")

	t.Logf("Phase 1: Received %d messages", len(phase1Messages))

	// Force channel close by closing the underlying connection's channel
	// The consumer should detect this and reconnect
	t.Log("Closing channel to trigger reconnection...")
	receivedMu.Lock()
	currentPhase = 2
	receivedMu.Unlock()

	// Close the connection to force reconnection
	// Note: We're using the connection's Close method which will trigger NotifyClose
	if infra.conn.Connection != nil {
		_ = infra.conn.Connection.Close()
	}

	// Wait for consumer to reconnect using condition-based polling
	// (avoids arbitrary sleep that causes flakiness)
	t.Log("Waiting for consumer to reconnect...")
	reconnected := make(chan struct{})
	go func() {
		// Poll until we can publish and receive a message
		for i := 0; i < 30; i++ {
			time.Sleep(500 * time.Millisecond)
			// Try to check if connection is back by inspecting queue
			if infra.rmqContainer.Channel != nil && !infra.rmqContainer.Channel.IsClosed() {
				close(reconnected)
				return
			}
		}
	}()

	select {
	case <-reconnected:
		t.Log("Consumer reconnection detected")
	case <-time.After(20 * time.Second):
		t.Log("Proceeding after timeout (consumer may still be reconnecting)")
	}

	// Phase 2: Publish more messages after reconnection
	t.Log("Phase 2: Publishing messages after reconnection")
	for i := 0; i < 3; i++ {
		// Use direct publish since our producer might also need to reconnect
		msg := consumerTestMessage{
			ID:        uuid.New().String(),
			Timestamp: time.Now(),
			Data:      "Phase 2 message",
		}
		publishTestMessageDirect(t, infra.rmqContainer.Channel, infra.exchange, infra.routingKey, msg)
	}

	// Wait for phase 2 messages
	require.Eventually(t, func() bool {
		receivedMu.Lock()
		defer receivedMu.Unlock()
		return len(phase2Messages) >= 3
	}, 15*time.Second, 100*time.Millisecond, "phase 2 messages should be consumed after reconnection")

	receivedMu.Lock()
	t.Logf("Phase 2: Received %d messages after reconnection", len(phase2Messages))
	receivedMu.Unlock()

	t.Log("Integration test passed: consumer reconnects after channel close")
}

// TestIntegration_Consumer_QoSRespected tests that the prefetch count (QoS) limits
// concurrent message processing.
func TestIntegration_Consumer_QoSRespected(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	// Set low prefetch to test QoS
	prefetch := 2
	infra := setupConsumerInfra(t, 1, prefetch)

	ctx := context.Background()

	// Track in-flight messages
	var inFlightMu sync.Mutex
	var inFlight int
	var maxInFlight int
	processed := make(chan struct{}, 10)

	// Register handler that holds messages for a bit
	infra.consumer.Register(infra.queue, func(ctx context.Context, body []byte) error {
		inFlightMu.Lock()
		inFlight++
		if inFlight > maxInFlight {
			maxInFlight = inFlight
		}
		currentInFlight := inFlight
		inFlightMu.Unlock()

		t.Logf("Processing message (in-flight: %d)", currentInFlight)

		// Hold the message for a bit to create backpressure
		time.Sleep(500 * time.Millisecond)

		inFlightMu.Lock()
		inFlight--
		inFlightMu.Unlock()

		processed <- struct{}{}
		return nil
	})

	// Start consumers
	err := infra.consumer.RunConsumers()
	require.NoError(t, err, "RunConsumers should succeed")

	// Give consumer time to start
	time.Sleep(500 * time.Millisecond)

	// Publish more messages than prefetch allows
	numMessages := 6
	for i := 0; i < numMessages; i++ {
		publishTestMessage(t, ctx, infra.producer, infra.exchange, infra.routingKey,
			fmt.Sprintf("QoS test message %d", i+1))
	}
	t.Logf("Published %d messages (prefetch: %d)", numMessages, prefetch)

	// Wait for all messages to be processed
	for i := 0; i < numMessages; i++ {
		select {
		case <-processed:
		case <-time.After(10 * time.Second):
			t.Fatalf("timeout waiting for message %d", i+1)
		}
	}

	inFlightMu.Lock()
	t.Logf("Max in-flight messages observed: %d (prefetch: %d)", maxInFlight, prefetch)
	inFlightMu.Unlock()

	// The actual NumbersOfPrefetch is calculated as workers * prefetch
	// With 1 worker and prefetch=2, NumbersOfPrefetch = 1 * 2 = 2
	expectedMaxPrefetch := 1 * prefetch
	assert.LessOrEqual(t, maxInFlight, expectedMaxPrefetch+1, // +1 for timing tolerance
		"in-flight messages should respect QoS prefetch limit")

	t.Log("Integration test passed: QoS prefetch is respected")
}

// =============================================================================
// CHAOS TESTS
// =============================================================================

// TestIntegration_Chaos_Consumer_NetworkLatency tests consumer behavior under network latency.
func TestIntegration_Chaos_Consumer_NetworkLatency(t *testing.T) {
	skipIfNotChaos(t)
	if testing.Short() {
		t.Skip("skipping chaos test in short mode")
	}

	infra := setupConsumerNetworkChaosInfra(t, 2, 10)
	defer infra.cleanup()

	t.Logf("Using Toxiproxy proxy: %s -> %s", infra.proxy.Listen(), infra.proxy.Upstream())

	// Track received messages
	var receivedMu sync.Mutex
	receivedMessages := make([]string, 0)

	// Register handler
	infra.proxyConsumer.Register(infra.queue, func(ctx context.Context, body []byte) error {
		var msg consumerTestMessage
		if err := json.Unmarshal(body, &msg); err != nil {
			return err
		}

		receivedMu.Lock()
		receivedMessages = append(receivedMessages, msg.ID)
		receivedMu.Unlock()

		t.Logf("Handler received message: %s", msg.ID)
		return nil
	})

	// Start consumers through proxy
	err := infra.proxyConsumer.RunConsumers()
	require.NoError(t, err, "RunConsumers should succeed")

	// Give consumer time to start
	time.Sleep(1 * time.Second)

	// Phase 1: Normal operation - publish messages directly to queue
	t.Log("Phase 1: Publishing messages without latency")
	for i := 0; i < 3; i++ {
		msg := consumerTestMessage{
			ID:        uuid.New().String(),
			Timestamp: time.Now(),
			Data:      fmt.Sprintf("Pre-latency message %d", i+1),
		}
		publishTestMessageDirect(t, infra.rmqContainer.Channel, infra.exchange, infra.routingKey, msg)
	}

	require.Eventually(t, func() bool {
		receivedMu.Lock()
		defer receivedMu.Unlock()
		return len(receivedMessages) >= 3
	}, 10*time.Second, 100*time.Millisecond, "pre-latency messages should be consumed")

	t.Logf("Phase 1: Received %d messages", len(receivedMessages))

	// Phase 2: INJECT CHAOS - Add 500ms latency
	t.Log("Chaos: Adding 500ms latency to RabbitMQ connection")
	err = infra.proxy.AddLatency(500*time.Millisecond, 100*time.Millisecond)
	require.NoError(t, err, "adding latency should succeed")

	// Publish messages with latency
	t.Log("Phase 2: Publishing messages with latency")
	for i := 0; i < 3; i++ {
		msg := consumerTestMessage{
			ID:        uuid.New().String(),
			Timestamp: time.Now(),
			Data:      fmt.Sprintf("With-latency message %d", i+1),
		}
		publishTestMessageDirect(t, infra.rmqContainer.Channel, infra.exchange, infra.routingKey, msg)
	}

	// Messages should still be consumed (with higher latency)
	require.Eventually(t, func() bool {
		receivedMu.Lock()
		defer receivedMu.Unlock()
		return len(receivedMessages) >= 6
	}, 20*time.Second, 100*time.Millisecond, "messages with latency should be consumed")

	// Phase 3: Remove latency
	t.Log("Chaos: Removing latency")
	err = infra.proxy.RemoveAllToxics()
	require.NoError(t, err, "removing toxics should succeed")

	// Publish final messages
	t.Log("Phase 3: Publishing messages after latency removed")
	for i := 0; i < 2; i++ {
		msg := consumerTestMessage{
			ID:        uuid.New().String(),
			Timestamp: time.Now(),
			Data:      fmt.Sprintf("Post-latency message %d", i+1),
		}
		publishTestMessageDirect(t, infra.rmqContainer.Channel, infra.exchange, infra.routingKey, msg)
	}

	require.Eventually(t, func() bool {
		receivedMu.Lock()
		defer receivedMu.Unlock()
		return len(receivedMessages) >= 8
	}, 10*time.Second, 100*time.Millisecond, "post-latency messages should be consumed")

	receivedMu.Lock()
	t.Logf("Total messages received: %d", len(receivedMessages))
	receivedMu.Unlock()

	t.Log("Chaos test passed: consumer handles network latency correctly")
}

// TestIntegration_Chaos_Consumer_ContainerRestart tests that consumers recover
// after a RabbitMQ container restart.
func TestIntegration_Chaos_Consumer_ContainerRestart(t *testing.T) {
	skipIfNotChaos(t)
	if testing.Short() {
		t.Skip("skipping chaos test in short mode")
	}

	infra := setupConsumerChaosInfra(t, 2, 10)
	defer infra.cleanup()

	ctx := context.Background()

	// Track received messages
	var receivedMu sync.Mutex
	receivedMessages := make([]string, 0)

	// Register handler
	infra.consumer.Register(infra.queue, func(ctx context.Context, body []byte) error {
		var msg consumerTestMessage
		if err := json.Unmarshal(body, &msg); err != nil {
			return err
		}

		receivedMu.Lock()
		receivedMessages = append(receivedMessages, msg.ID)
		receivedMu.Unlock()

		t.Logf("Handler received message: %s", msg.ID)
		return nil
	})

	// Start consumers
	err := infra.consumer.RunConsumers()
	require.NoError(t, err, "RunConsumers should succeed")

	// Give consumer time to start
	time.Sleep(1 * time.Second)

	// Phase 1: Consume messages before restart
	t.Log("Phase 1: Publishing messages before container restart")
	for i := 0; i < 3; i++ {
		publishTestMessage(t, ctx, infra.producer, infra.exchange, infra.routingKey,
			fmt.Sprintf("Pre-restart message %d", i+1))
	}

	require.Eventually(t, func() bool {
		receivedMu.Lock()
		defer receivedMu.Unlock()
		return len(receivedMessages) >= 3
	}, 10*time.Second, 100*time.Millisecond, "pre-restart messages should be consumed")

	receivedMu.Lock()
	preRestartCount := len(receivedMessages)
	receivedMu.Unlock()
	t.Logf("Phase 1: Received %d messages before restart", preRestartCount)

	// Phase 2: INJECT CHAOS - Restart container
	containerID := infra.rmqContainer.Container.GetContainerID()
	t.Logf("Chaos: Restarting RabbitMQ container %s", containerID)

	err = infra.chaosOrch.RestartContainer(ctx, containerID, 10*time.Second)
	require.NoError(t, err, "container restart should succeed")

	err = infra.chaosOrch.WaitForContainerRunning(ctx, containerID, 60*time.Second)
	require.NoError(t, err, "container should be running after restart")

	t.Log("Container restarted, waiting for RabbitMQ to be ready...")

	// Recreate channel for inspection and publishing after restart
	infra.recreateChannelForInspection(t)

	// Re-setup exchange and queue (they may not persist after restart)
	rmqtestutil.SetupExchange(t, infra.rmqContainer.Channel, infra.exchange, "topic")
	rmqtestutil.SetupQueue(t, infra.rmqContainer.Channel, infra.queue, infra.exchange, infra.routingKey)

	// Wait for consumer to reconnect using condition-based polling
	// (avoids arbitrary sleep that causes flakiness)
	t.Log("Waiting for consumer to reconnect...")
	require.Eventually(t, func() bool {
		// Check if we can communicate with RabbitMQ
		_, err := infra.rmqContainer.Channel.QueueInspect(infra.queue)
		return err == nil
	}, 30*time.Second, 500*time.Millisecond, "RabbitMQ should be ready after restart")

	// Phase 3: Publish messages after restart
	t.Log("Phase 3: Publishing messages after container restart")
	for i := 0; i < 3; i++ {
		msg := consumerTestMessage{
			ID:        uuid.New().String(),
			Timestamp: time.Now(),
			Data:      fmt.Sprintf("Post-restart message %d", i+1),
		}
		publishTestMessageDirect(t, infra.rmqContainer.Channel, infra.exchange, infra.routingKey, msg)
	}

	// Wait for new messages to be consumed
	require.Eventually(t, func() bool {
		receivedMu.Lock()
		defer receivedMu.Unlock()
		return len(receivedMessages) >= preRestartCount+3
	}, 30*time.Second, 500*time.Millisecond, "post-restart messages should be consumed")

	receivedMu.Lock()
	t.Logf("Total messages received: %d (pre-restart: %d)", len(receivedMessages), preRestartCount)
	receivedMu.Unlock()

	t.Log("Chaos test passed: consumer recovers after container restart")
}
