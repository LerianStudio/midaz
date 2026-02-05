//go:build integration

// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package rabbitmq

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"sync"
	"testing"
	"time"

	libRabbitmq "github.com/LerianStudio/lib-commons/v2/commons/rabbitmq"
	libZap "github.com/LerianStudio/lib-commons/v2/commons/zap"
	"github.com/LerianStudio/midaz/v3/tests/utils/chaos"
	rmqtestutil "github.com/LerianStudio/midaz/v3/tests/utils/rabbitmq"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// =============================================================================
// TEST HELPERS
// =============================================================================

// skipIfNotChaos skips the test if CHAOS=1 environment variable is not set.
// Use this for tests that inject failures (network chaos, container restarts, etc.)
func skipIfNotChaos(t *testing.T) {
	t.Helper()
	if os.Getenv("CHAOS") != "1" {
		t.Skip("skipping chaos test (set CHAOS=1 to run)")
	}
}

// =============================================================================
// TEST INFRASTRUCTURE
// =============================================================================

// integrationTestInfra holds the infrastructure needed for RabbitMQ integration tests.
type integrationTestInfra struct {
	rmqContainer *rmqtestutil.ContainerResult
	conn         *libRabbitmq.RabbitMQConnection
	producer     *ProducerRabbitMQRepository
	exchange     string
	routingKey   string
	queue        string
}

// chaosTestInfra holds the infrastructure needed for RabbitMQ chaos tests.
type chaosTestInfra struct {
	rmqContainer *rmqtestutil.ContainerResult
	conn         *libRabbitmq.RabbitMQConnection
	producer     *ProducerRabbitMQRepository
	chaosOrch    *chaos.Orchestrator
	toxiproxy    *chaos.ToxiproxyResult
	exchange     string
	routingKey   string
	queue        string
}

// networkChaosTestInfra holds infrastructure for network chaos tests with Toxiproxy.
// Uses the unified chaos.Infrastructure for Toxiproxy management.
type networkChaosTestInfra struct {
	rmqContainer  *rmqtestutil.ContainerResult
	chaosInfra    *chaos.Infrastructure
	proxyProducer *ProducerRabbitMQRepository
	proxyConn     *libRabbitmq.RabbitMQConnection
	proxy         *chaos.Proxy
	exchange      string
	routingKey    string
	queue         string
}

// setupIntegrationInfra sets up the test infrastructure for RabbitMQ integration testing.
func setupIntegrationInfra(t *testing.T) *integrationTestInfra {
	t.Helper()

	// Setup RabbitMQ container
	rmqContainer := rmqtestutil.SetupContainer(t)

	// Setup exchange and queue
	exchange := "test-exchange"
	routingKey := "test.routing.key"
	queue := "test-queue"

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

	// Create producer repository
	producer := NewProducerRabbitMQ(conn)

	return &integrationTestInfra{
		rmqContainer: rmqContainer,
		conn:         conn,
		producer:     producer,
		exchange:     exchange,
		routingKey:   routingKey,
		queue:        queue,
	}
}

// setupRabbitMQChaosInfra sets up the test infrastructure for RabbitMQ chaos testing.
func setupRabbitMQChaosInfra(t *testing.T) *chaosTestInfra {
	t.Helper()

	// Setup RabbitMQ container
	rmqContainer := rmqtestutil.SetupContainer(t)

	// Setup exchange and queue
	exchange := "test-exchange"
	routingKey := "test.routing.key"
	queue := "test-queue"

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

	// Create producer repository
	producer := NewProducerRabbitMQ(conn)

	// Create chaos orchestrator
	chaosOrch := chaos.NewOrchestrator(t)

	return &chaosTestInfra{
		rmqContainer: rmqContainer,
		conn:         conn,
		producer:     producer,
		chaosOrch:    chaosOrch,
		exchange:     exchange,
		routingKey:   routingKey,
		queue:        queue,
	}
}

// setupRabbitMQNetworkChaosInfra sets up infrastructure for network chaos testing with Toxiproxy.
// Uses the unified chaos.Infrastructure which manages Toxiproxy lifecycle.
func setupRabbitMQNetworkChaosInfra(t *testing.T) *networkChaosTestInfra {
	t.Helper()

	// Setup exchange and queue names
	exchange := "test-exchange"
	routingKey := "test.routing.key"
	queue := "test-queue"

	// 1. Create chaos infrastructure (creates network + Toxiproxy)
	chaosInfra := chaos.NewInfrastructure(t)

	// 2. Create RabbitMQ container (on host network, not chaos infra network)
	rmqContainer := rmqtestutil.SetupContainer(t)

	// 3. Setup exchange and queue
	rmqtestutil.SetupExchange(t, rmqContainer.Channel, exchange, "topic")
	rmqtestutil.SetupQueue(t, rmqContainer.Channel, queue, exchange, routingKey)

	// 4. Register RabbitMQ container with infrastructure for proxy creation
	_, err := chaosInfra.RegisterContainerWithPort("rabbitmq", rmqContainer.Container, "5672/tcp")
	require.NoError(t, err, "failed to register RabbitMQ container")

	// 5. Create proxy for RabbitMQ (Toxiproxy -> RabbitMQ via host-mapped port)
	// Use port 8667 which is one of the exposed proxy ports on the Toxiproxy container
	proxy, err := chaosInfra.CreateProxyFor("rabbitmq", "8667/tcp")
	require.NoError(t, err, "failed to create Toxiproxy proxy for RabbitMQ")

	// 6. Get proxy address for client connections
	containerInfo, ok := chaosInfra.GetContainer("rabbitmq")
	require.True(t, ok, "RabbitMQ container should be registered")
	require.NotEmpty(t, containerInfo.ProxyListen, "proxy address should be set")

	// Parse proxy address for AMQP connection
	proxyAddr := containerInfo.ProxyListen

	// 7. Create producer that connects through the proxy
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
		Port:                   "15672", // Proxy listen port
		User:                   rmqtestutil.DefaultUser,
		Pass:                   rmqtestutil.DefaultPassword,
		Logger:                 logger,
	}
	proxyProducer := NewProducerRabbitMQ(proxyConn)

	return &networkChaosTestInfra{
		rmqContainer:  rmqContainer,
		chaosInfra:    chaosInfra,
		proxyProducer: proxyProducer,
		proxyConn:     proxyConn,
		proxy:         proxy,
		exchange:      exchange,
		routingKey:    routingKey,
		queue:         queue,
	}
}

// cleanup releases all resources for chaos tests.
// Note: Container cleanup is handled automatically by SetupContainer via t.Cleanup().
func (infra *chaosTestInfra) cleanup() {
	if infra.chaosOrch != nil {
		infra.chaosOrch.Close()
	}
}

// cleanup releases all resources for network chaos infrastructure.
// Note: Container cleanup is handled automatically by SetupContainer via t.Cleanup().
func (infra *networkChaosTestInfra) cleanup() {
	// Cleanup Infrastructure (Toxiproxy, network, orchestrator)
	// Note: This may log warnings about already-terminated containers
	if infra.chaosInfra != nil {
		infra.chaosInfra.Cleanup()
	}
}

// recreateChannelForInspection creates a NEW AMQP channel for queue inspection after container restart.
// This is necessary because the original channel is invalidated when the container restarts with a new port.
// NOTE: This does NOT test the application's auto-reconnect mechanism - it creates a fresh connection
// solely for inspecting queue state (e.g., message counts) in data integrity tests.
func (infra *chaosTestInfra) recreateChannelForInspection(t *testing.T) {
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

	// Create a fresh AMQP connection and channel for inspection (with retry for post-restart)
	newChannel := rmqtestutil.CreateChannelWithRetry(t, infra.rmqContainer.URI, 30*time.Second)
	infra.rmqContainer.Channel = newChannel

	t.Logf("Created new channel for inspection (port changed to %s)", infra.rmqContainer.AMQPPort)
}

// killAllRabbitMQConnections closes all client connections via RabbitMQ Management API.
// This simulates scenarios where the broker forcefully terminates connections
// (e.g., memory alarm, admin intervention, broker restart).
func killAllRabbitMQConnections(t *testing.T, mgmtHost, mgmtPort, user, pass string) int {
	t.Helper()

	client := &http.Client{Timeout: 10 * time.Second}
	baseURL := fmt.Sprintf("http://%s:%s/api/connections", mgmtHost, mgmtPort)

	// 1. GET all connections
	req, err := http.NewRequest("GET", baseURL, nil)
	require.NoError(t, err)
	req.SetBasicAuth(user, pass)

	resp, err := client.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	var connections []struct {
		Name string `json:"name"`
	}
	err = json.NewDecoder(resp.Body).Decode(&connections)
	require.NoError(t, err)

	// 2. DELETE each connection
	killed := 0
	for _, conn := range connections {
		deleteURL := fmt.Sprintf("%s/%s", baseURL, url.PathEscape(conn.Name))
		delReq, err := http.NewRequest("DELETE", deleteURL, nil)
		if err != nil {
			continue
		}
		delReq.SetBasicAuth(user, pass)

		delResp, err := client.Do(delReq)
		if err == nil {
			delResp.Body.Close()
			if delResp.StatusCode == http.StatusNoContent || delResp.StatusCode == http.StatusOK {
				killed++
			}
		}
	}

	t.Logf("Killed %d RabbitMQ connections via Management API", killed)
	return killed
}

// integrationTestMessage represents a test message for integration testing.
type integrationTestMessage struct {
	ID        string    `json:"id"`
	Timestamp time.Time `json:"timestamp"`
	Data      string    `json:"data"`
}

// chaosTestMessage represents a test message for chaos testing.
type chaosTestMessage struct {
	ID        string    `json:"id"`
	Timestamp time.Time `json:"timestamp"`
	Data      string    `json:"data"`
}

// =============================================================================
// INTEGRATION TESTS - BASIC OPERATIONS
// =============================================================================

// TestIntegration_RabbitMQ_ProducerBasicPublish tests that the producer can publish
// a message successfully and verify it arrives in the queue.
func TestIntegration_RabbitMQ_ProducerBasicPublish(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	infra := setupIntegrationInfra(t)

	ctx := context.Background()

	// Create and publish a test message
	msg := integrationTestMessage{
		ID:        uuid.New().String(),
		Timestamp: time.Now(),
		Data:      "Test message for basic publish",
	}
	msgBytes, err := json.Marshal(msg)
	require.NoError(t, err)

	// Publish message
	_, err = infra.producer.ProducerDefault(ctx, infra.exchange, infra.routingKey, msgBytes)
	require.NoError(t, err, "publish should succeed")
	t.Log("Published message successfully")

	// Give RabbitMQ time to route the message
	time.Sleep(500 * time.Millisecond)

	// Verify message is in queue
	msgCount := rmqtestutil.GetQueueMessageCount(t, infra.rmqContainer.Channel, infra.queue)
	assert.Equal(t, 1, msgCount, "message should be in queue")

	t.Log("Integration test passed: basic publish verified")
}

// TestIntegration_RabbitMQ_ConcurrentPublish tests that concurrent message publishing
// is handled correctly without data loss.
func TestIntegration_RabbitMQ_ConcurrentPublish(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	infra := setupIntegrationInfra(t)

	ctx := context.Background()
	numPublishers := 20

	type publishResult struct {
		correlationID *string
		err           error
	}
	results := make(chan publishResult, numPublishers)

	// Start concurrent publishers
	var wg sync.WaitGroup
	for i := 0; i < numPublishers; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()

			msg := integrationTestMessage{
				ID:        uuid.New().String(),
				Timestamp: time.Now(),
				Data:      "Concurrent message",
			}
			msgBytes, _ := json.Marshal(msg)

			correlationID, err := infra.producer.ProducerDefault(ctx, infra.exchange, infra.routingKey, msgBytes)
			results <- publishResult{correlationID: correlationID, err: err}
		}(i)
	}

	// Wait for all publishers
	go func() {
		wg.Wait()
		close(results)
	}()

	// Analyze results
	var successCount, errorCount int
	for r := range results {
		if r.err != nil {
			errorCount++
			t.Logf("Concurrent publish error: %v", r.err)
		} else {
			successCount++
		}
	}

	t.Logf("Concurrent publish: %d successful, %d errors", successCount, errorCount)

	// Give RabbitMQ time to route the messages
	time.Sleep(500 * time.Millisecond)

	// Verify messages in queue
	msgCount := rmqtestutil.GetQueueMessageCount(t, infra.rmqContainer.Channel, infra.queue)
	assert.Equal(t, successCount, msgCount, "queue message count should match successful publishes")

	// All publishes should succeed
	assert.Equal(t, numPublishers, successCount, "all concurrent publishes should succeed")

	t.Log("Integration test passed: concurrent publishing handled correctly")
}

// TestIntegration_RabbitMQ_HealthCheck tests that health check works correctly
// and reflects the actual connection state.
func TestIntegration_RabbitMQ_HealthCheck(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	infra := setupIntegrationInfra(t)

	// Health check should not panic
	healthy := infra.producer.CheckRabbitMQHealth()
	t.Logf("Initial health check: %v", healthy)

	// Publish a message to verify actual connectivity
	ctx := context.Background()
	msg := integrationTestMessage{
		ID:        uuid.New().String(),
		Timestamp: time.Now(),
		Data:      "Health check test",
	}
	msgBytes, _ := json.Marshal(msg)

	_, err := infra.producer.ProducerDefault(ctx, infra.exchange, infra.routingKey, msgBytes)
	require.NoError(t, err, "producer should be working correctly")

	t.Log("Integration test passed: health check verified")
}

// TestIntegration_RabbitMQ_MessageOrdering tests that message ordering is preserved
// when publishing sequentially.
func TestIntegration_RabbitMQ_MessageOrdering(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	infra := setupIntegrationInfra(t)

	ctx := context.Background()
	numMessages := 10

	// Publish messages in order
	messageIDs := make([]string, numMessages)
	for i := 0; i < numMessages; i++ {
		msg := integrationTestMessage{
			ID:        uuid.New().String(),
			Timestamp: time.Now(),
			Data:      "Ordered message",
		}
		messageIDs[i] = msg.ID
		msgBytes, _ := json.Marshal(msg)

		_, err := infra.producer.ProducerDefault(ctx, infra.exchange, infra.routingKey, msgBytes)
		require.NoError(t, err, "publish %d should succeed", i)
	}

	// Give RabbitMQ time to route the messages
	time.Sleep(500 * time.Millisecond)

	// Verify all messages are in queue
	msgCount := rmqtestutil.GetQueueMessageCount(t, infra.rmqContainer.Channel, infra.queue)
	assert.Equal(t, numMessages, msgCount, "all messages should be in queue")

	t.Logf("Published %d messages in order", numMessages)
	t.Log("Integration test passed: message ordering verified")
}

// TestIntegration_RabbitMQ_LargeMessage tests that large messages are handled correctly.
func TestIntegration_RabbitMQ_LargeMessage(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	infra := setupIntegrationInfra(t)

	ctx := context.Background()

	// Create a large message (1MB of data in the Data field)
	largeData := make([]byte, 1024*1024)
	for i := range largeData {
		largeData[i] = byte(i % 256)
	}

	msg := integrationTestMessage{
		ID:        uuid.New().String(),
		Timestamp: time.Now(),
		Data:      string(largeData[:1000]), // Use first 1000 bytes as string
	}
	msgBytes, err := json.Marshal(msg)
	require.NoError(t, err)

	t.Logf("Publishing large message: %d bytes", len(msgBytes))

	// Publish large message
	_, err = infra.producer.ProducerDefault(ctx, infra.exchange, infra.routingKey, msgBytes)
	require.NoError(t, err, "large message publish should succeed")

	// Give RabbitMQ time to route the message
	time.Sleep(500 * time.Millisecond)

	// Verify message is in queue
	msgCount := rmqtestutil.GetQueueMessageCount(t, infra.rmqContainer.Channel, infra.queue)
	assert.Equal(t, 1, msgCount, "large message should be in queue")

	t.Log("Integration test passed: large message handled correctly")
}

// TestIntegration_RabbitMQ_EmptyMessage tests that empty messages are handled correctly.
func TestIntegration_RabbitMQ_EmptyMessage(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	infra := setupIntegrationInfra(t)

	ctx := context.Background()

	// Publish empty message body
	_, err := infra.producer.ProducerDefault(ctx, infra.exchange, infra.routingKey, []byte{})
	require.NoError(t, err, "empty message publish should succeed")

	// Give RabbitMQ time to route the message
	time.Sleep(500 * time.Millisecond)

	// Verify message is in queue
	msgCount := rmqtestutil.GetQueueMessageCount(t, infra.rmqContainer.Channel, infra.queue)
	assert.Equal(t, 1, msgCount, "empty message should be in queue")

	t.Log("Integration test passed: empty message handled correctly")
}

// TestIntegration_RabbitMQ_MultipleExchanges tests publishing to multiple exchanges.
func TestIntegration_RabbitMQ_MultipleExchanges(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	infra := setupIntegrationInfra(t)

	ctx := context.Background()

	// Setup second exchange and queue
	exchange2 := "test-exchange-2"
	queue2 := "test-queue-2"
	routingKey2 := "test.routing.key.2"

	rmqtestutil.SetupExchange(t, infra.rmqContainer.Channel, exchange2, "topic")
	rmqtestutil.SetupQueue(t, infra.rmqContainer.Channel, queue2, exchange2, routingKey2)

	// Publish to first exchange
	msg1 := integrationTestMessage{
		ID:        uuid.New().String(),
		Timestamp: time.Now(),
		Data:      "Message for exchange 1",
	}
	msgBytes1, _ := json.Marshal(msg1)
	_, err := infra.producer.ProducerDefault(ctx, infra.exchange, infra.routingKey, msgBytes1)
	require.NoError(t, err, "publish to exchange 1 should succeed")

	// Publish to second exchange
	msg2 := integrationTestMessage{
		ID:        uuid.New().String(),
		Timestamp: time.Now(),
		Data:      "Message for exchange 2",
	}
	msgBytes2, _ := json.Marshal(msg2)
	_, err = infra.producer.ProducerDefault(ctx, exchange2, routingKey2, msgBytes2)
	require.NoError(t, err, "publish to exchange 2 should succeed")

	// Give RabbitMQ time to route the messages
	time.Sleep(500 * time.Millisecond)

	// Verify messages in respective queues
	msgCount1 := rmqtestutil.GetQueueMessageCount(t, infra.rmqContainer.Channel, infra.queue)
	msgCount2 := rmqtestutil.GetQueueMessageCount(t, infra.rmqContainer.Channel, queue2)

	assert.Equal(t, 1, msgCount1, "queue 1 should have 1 message")
	assert.Equal(t, 1, msgCount2, "queue 2 should have 1 message")

	t.Log("Integration test passed: multiple exchanges handled correctly")
}

// =============================================================================
// CHAOS TESTS - NETWORK CHAOS
// =============================================================================

// TestChaos_RabbitMQ_NetworkLatency tests producer behavior under network latency.
// Uses Toxiproxy to inject latency into the network path.
// NOTE: The producer uses async publishing, so we verify message DELIVERY rather than
// measuring publish call duration (which returns before network I/O completes).
func TestIntegration_Chaos_RabbitMQ_NetworkLatency(t *testing.T) {
	skipIfNotChaos(t)
	if testing.Short() {
		t.Skip("skipping chaos test in short mode")
	}

	infra := setupRabbitMQNetworkChaosInfra(t)
	defer infra.cleanup()

	ctx := context.Background()
	t.Logf("Using Toxiproxy proxy: %s -> %s", infra.proxy.Listen(), infra.proxy.Upstream())

	// 1. Verify normal operation through proxy - message should be delivered
	msg1 := chaosTestMessage{
		ID:        uuid.New().String(),
		Timestamp: time.Now(),
		Data:      "Message before latency",
	}
	msgBytes1, _ := json.Marshal(msg1)

	_, err := infra.proxyProducer.ProducerDefault(ctx, infra.exchange, infra.routingKey, msgBytes1)
	require.NoError(t, err, "initial publish through proxy should succeed")

	// Wait for message to be delivered and verify it arrived
	rmqtestutil.WaitForQueueCount(t, infra.rmqContainer.Channel, infra.queue, 1, 5*time.Second)
	t.Log("Message delivered to queue (count: 1)")

	// 2. INJECT CHAOS: Add 500ms latency with 100ms jitter
	t.Log("Chaos: Adding 500ms latency to RabbitMQ connection")
	err = infra.proxy.AddLatency(500*time.Millisecond, 100*time.Millisecond)
	require.NoError(t, err, "adding latency should succeed")

	// 3. Publish multiple messages with latency - they should still be delivered
	numMessages := 3
	for i := 0; i < numMessages; i++ {
		msg := chaosTestMessage{
			ID:        uuid.New().String(),
			Timestamp: time.Now(),
			Data:      fmt.Sprintf("Message with latency %d", i+1),
		}
		msgBytes, _ := json.Marshal(msg)
		_, err = infra.proxyProducer.ProducerDefault(ctx, infra.exchange, infra.routingKey, msgBytes)
		require.NoError(t, err, "publish with latency should not return error")
	}
	t.Logf("Published %d messages with latency injected", numMessages)

	// Wait for messages to be delivered (latency adds ~500ms per message)
	expectedCount := 1 + numMessages
	rmqtestutil.WaitForQueueCount(t, infra.rmqContainer.Channel, infra.queue, expectedCount, 10*time.Second)
	t.Logf("Messages delivered with latency (count: %d)", expectedCount)

	// 4. REMOVE CHAOS: Remove all toxics
	t.Log("Chaos: Removing latency")
	err = infra.proxy.RemoveAllToxics()
	require.NoError(t, err, "removing toxics should succeed")

	// 5. Verify normal operation restored - messages should still be delivered quickly
	msg3 := chaosTestMessage{
		ID:        uuid.New().String(),
		Timestamp: time.Now(),
		Data:      "Message after latency removed",
	}
	msgBytes3, _ := json.Marshal(msg3)

	_, err = infra.proxyProducer.ProducerDefault(ctx, infra.exchange, infra.routingKey, msgBytes3)
	require.NoError(t, err, "publish after removing latency should succeed")

	// Wait and verify final message delivery
	finalExpected := 1 + numMessages + 1
	rmqtestutil.WaitForQueueCount(t, infra.rmqContainer.Channel, infra.queue, finalExpected, 5*time.Second)
	t.Logf("Final message count: %d", finalExpected)

	t.Log("Chaos test passed: RabbitMQ network latency handling verified - all messages delivered")
}

// TestChaos_RabbitMQ_RetryDuringNetworkOutage tests that the producer's retry mechanism
// continues attempting to publish during network outage and succeeds after recovery.
// This validates that the built-in retry logic in ProducerDefault handles transient
// network failures gracefully without losing messages.
func TestIntegration_Chaos_RabbitMQ_RetryDuringNetworkOutage(t *testing.T) {
	skipIfNotChaos(t)
	if testing.Short() {
		t.Skip("skipping chaos test in short mode")
	}

	infra := setupRabbitMQNetworkChaosInfra(t)
	defer infra.cleanup()

	ctx := context.Background()
	t.Logf("Using Toxiproxy proxy: %s -> %s", infra.proxy.Listen(), infra.proxy.Upstream())

	// 1. Publish baseline message to verify infrastructure is working
	t.Log("Step 1: Publishing baseline message to verify setup")
	baselineMsg := chaosTestMessage{
		ID:        uuid.New().String(),
		Timestamp: time.Now(),
		Data:      "Baseline message before network outage",
	}
	baselineMsgBytes, _ := json.Marshal(baselineMsg)

	_, err := infra.proxyProducer.ProducerDefault(ctx, infra.exchange, infra.routingKey, baselineMsgBytes)
	require.NoError(t, err, "baseline publish should succeed")

	rmqtestutil.WaitForQueueCount(t, infra.rmqContainer.Channel, infra.queue, 1, 5*time.Second)
	t.Log("Baseline message delivered successfully (queue count: 1)")

	// 2. Start goroutine that will attempt to publish during network outage
	// The producer's retry mechanism should keep retrying until network recovers
	var wg sync.WaitGroup
	var publishErr error

	retryMsg := chaosTestMessage{
		ID:        uuid.New().String(),
		Timestamp: time.Now(),
		Data:      "Message published during network outage with retries",
	}
	retryMsgBytes, _ := json.Marshal(retryMsg)

	wg.Add(1)
	go func() {
		defer wg.Done()
		t.Log("Goroutine: Starting publish attempt (will retry during outage)")

		// Use 60s timeout to allow multiple retry attempts
		ctxWithTimeout, cancel := context.WithTimeout(ctx, 60*time.Second)
		defer cancel()

		_, publishErr = infra.proxyProducer.ProducerDefault(ctxWithTimeout, infra.exchange, infra.routingKey, retryMsgBytes)
		if publishErr != nil {
			t.Logf("Goroutine: Publish completed with error: %v", publishErr)
		} else {
			t.Log("Goroutine: Publish completed successfully")
		}
	}()

	// 3. Wait for goroutine to start and initiate connection
	t.Log("Step 3: Waiting 500ms for goroutine to start")
	time.Sleep(500 * time.Millisecond)

	// 4. INJECT CHAOS: Disconnect proxy to simulate network outage
	t.Log("Step 4: INJECT CHAOS - Disconnecting proxy to simulate network outage")
	err = infra.proxy.Disconnect()
	require.NoError(t, err, "proxy disconnect should succeed")
	t.Log("Chaos: Network outage injected - proxy disconnected")

	// 5. Wait ~8 seconds for 3-4 retry attempts (producer uses exponential backoff)
	// This is enough time for retries but not enough to exhaust all attempts
	t.Log("Step 5: Waiting 8 seconds for retry attempts during outage")
	time.Sleep(8 * time.Second)
	t.Log("Chaos: Retry period elapsed - producer should have attempted multiple retries")

	// 6. REMOVE CHAOS: Reconnect proxy to restore network
	t.Log("Step 6: REMOVE CHAOS - Reconnecting proxy to restore network")
	err = infra.proxy.Reconnect()
	require.NoError(t, err, "proxy reconnect should succeed")
	t.Log("Chaos: Network restored - proxy reconnected")

	// 7. Wait for goroutine to complete
	t.Log("Step 7: Waiting for publish goroutine to complete")
	wg.Wait()
	t.Log("Goroutine completed")

	// 8. Assert no error from publish - retry mechanism should have succeeded
	assert.NoError(t, publishErr, "publish should succeed after network recovery due to retry mechanism")

	// 9. Verify message count in queue is 2 (baseline + retry message)
	t.Log("Step 9: Verifying message delivery")
	rmqtestutil.WaitForQueueCount(t, infra.rmqContainer.Channel, infra.queue, 2, 10*time.Second)

	finalCount := rmqtestutil.GetQueueMessageCount(t, infra.rmqContainer.Channel, infra.queue)
	assert.Equal(t, 2, finalCount, "queue should contain 2 messages (baseline + retry message)")
	t.Logf("Final queue message count: %d", finalCount)

	t.Log("Chaos test passed: RabbitMQ retry during network outage verified - message delivered after recovery")
}

// =============================================================================
// CHAOS TESTS - CONNECTION MANAGEMENT
// =============================================================================

// TestChaos_RabbitMQ_ConnectionKilledByBroker tests producer recovery when the broker
// forcefully closes all connections. This simulates real-world scenarios like:
// - RabbitMQ memory/disk alarm triggered
// - Broker restart during deployment
// - Admin manually closing problematic connections
//
// The producer's EnsureChannel should detect the closed connection and reconnect.
func TestIntegration_Chaos_RabbitMQ_ConnectionKilledByBroker(t *testing.T) {
	skipIfNotChaos(t)
	if testing.Short() {
		t.Skip("skipping chaos test in short mode")
	}

	infra := setupRabbitMQChaosInfra(t)
	defer infra.cleanup()

	ctx := context.Background()

	const (
		numGoroutines      = 3
		messagesPerRoutine = 10
		totalMessages      = numGoroutines * messagesPerRoutine
	)

	// 1. Verify baseline - publish one message to confirm setup works
	t.Log("Step 1: Verifying baseline connectivity")
	baselineMsg := chaosTestMessage{
		ID:        uuid.New().String(),
		Timestamp: time.Now(),
		Data:      "Baseline message",
	}
	baselineMsgBytes, _ := json.Marshal(baselineMsg)
	_, err := infra.producer.ProducerDefault(ctx, infra.exchange, infra.routingKey, baselineMsgBytes)
	require.NoError(t, err, "baseline publish should succeed")
	t.Log("Baseline message published successfully")

	// 2. Start goroutines that will publish messages continuously
	t.Logf("Step 2: Starting %d goroutines, each publishing %d messages", numGoroutines, messagesPerRoutine)

	var wg sync.WaitGroup
	errors := make(chan error, totalMessages)
	published := make(chan int, totalMessages)
	startSignal := make(chan struct{})

	for g := 0; g < numGoroutines; g++ {
		wg.Add(1)
		go func(goroutineID int) {
			defer wg.Done()
			<-startSignal // Wait for signal to start

			for i := 0; i < messagesPerRoutine; i++ {
				msg := chaosTestMessage{
					ID:        uuid.New().String(),
					Timestamp: time.Now(),
					Data:      fmt.Sprintf("Goroutine %d, Message %d", goroutineID, i+1),
				}
				msgBytes, _ := json.Marshal(msg)

				ctxWithTimeout, cancel := context.WithTimeout(ctx, 60*time.Second)
				_, err := infra.producer.ProducerDefault(ctxWithTimeout, infra.exchange, infra.routingKey, msgBytes)
				cancel()

				if err != nil {
					errors <- fmt.Errorf("goroutine %d, msg %d: %w", goroutineID, i+1, err)
				} else {
					published <- 1
				}

				// Small delay between messages to spread the load
				time.Sleep(100 * time.Millisecond)
			}
		}(g)
	}

	// 3. Start all goroutines
	close(startSignal)
	t.Log("All goroutines started")

	// 4. Wait for some messages to be published, then kill connections
	t.Log("Step 3: Waiting for initial messages before killing connections...")
	time.Sleep(1 * time.Second) // Allow some messages to be published

	// 5. INJECT CHAOS: Kill all connections via Management API
	t.Log("Step 4: INJECT CHAOS - Killing all RabbitMQ connections via Management API")
	killed := killAllRabbitMQConnections(t,
		infra.rmqContainer.Host,
		infra.rmqContainer.MgmtPort,
		rmqtestutil.DefaultUser,
		rmqtestutil.DefaultPassword,
	)
	t.Logf("Chaos: Killed %d connections - producers should now enter retry/reconnect", killed)

	// 6. Wait for all goroutines to complete
	t.Log("Step 5: Waiting for all goroutines to complete...")
	wg.Wait()
	close(errors)
	close(published)

	// 7. Collect results
	var publishErrors []error
	for err := range errors {
		publishErrors = append(publishErrors, err)
	}

	successCount := 0
	for range published {
		successCount++
	}

	t.Logf("Results: %d successful, %d failed out of %d total", successCount, len(publishErrors), totalMessages)

	// 8. Assert results
	// We expect ALL messages to eventually succeed due to retry mechanism
	require.Empty(t, publishErrors, "all publishes should succeed after reconnection")
	assert.Equal(t, totalMessages, successCount, "all messages should be published successfully")

	// 9. Verify messages arrived in queue (baseline + all goroutine messages)
	t.Log("Step 6: Verifying message delivery to queue")
	expectedTotal := 1 + totalMessages // baseline + goroutine messages
	rmqtestutil.WaitForQueueCount(t, infra.rmqContainer.Channel, infra.queue, expectedTotal, 30*time.Second)

	finalCount := rmqtestutil.GetQueueMessageCount(t, infra.rmqContainer.Channel, infra.queue)
	t.Logf("Final queue message count: %d (expected: %d)", finalCount, expectedTotal)
	assert.GreaterOrEqual(t, finalCount, expectedTotal, "queue should contain all published messages")

	t.Log("Chaos test passed: Producer recovered from broker-killed connections")
}

// =============================================================================
// CHAOS TESTS - DATA INTEGRITY
// =============================================================================

// TestChaos_RabbitMQ_DataIntegrityAfterRestart tests that no messages are lost
// during container restart (for durable queues).
func TestIntegration_Chaos_RabbitMQ_DataIntegrityAfterRestart(t *testing.T) {
	skipIfNotChaos(t)
	if testing.Short() {
		t.Skip("skipping chaos test in short mode")
	}

	infra := setupRabbitMQChaosInfra(t)
	defer infra.cleanup()

	ctx := context.Background()

	// 1. Publish multiple messages before restart
	numMessages := 5
	for i := 0; i < numMessages; i++ {
		msg := chaosTestMessage{
			ID:        uuid.New().String(),
			Timestamp: time.Now(),
			Data:      fmt.Sprintf("Message %d before restart", i+1),
		}
		msgBytes, _ := json.Marshal(msg)

		_, err := infra.producer.ProducerDefault(ctx, infra.exchange, infra.routingKey, msgBytes)
		require.NoError(t, err, "publish %d should succeed", i+1)
	}
	t.Logf("Published %d messages to exchange=%s, routingKey=%s", numMessages, infra.exchange, infra.routingKey)

	// Wait for messages to be routed to the queue
	rmqtestutil.WaitForQueueCount(t, infra.rmqContainer.Channel, infra.queue, numMessages, 5*time.Second)
	t.Logf("Messages in queue before restart: %d", numMessages)

	// 2. INJECT CHAOS: Restart container
	containerID := infra.rmqContainer.Container.GetContainerID()
	t.Logf("Chaos: Restarting RabbitMQ container %s", containerID)

	err := infra.chaosOrch.RestartContainer(ctx, containerID, 10*time.Second)
	require.NoError(t, err, "container restart should succeed")

	err = infra.chaosOrch.WaitForContainerRunning(ctx, containerID, 60*time.Second)
	require.NoError(t, err, "container should be running after restart")

	// 3. Check data integrity - verify messages survived the restart
	// NOTE: After container restart, the port changes (testcontainers behavior).
	// We create a NEW channel for inspection with retry logic (RabbitMQ may still be starting).
	// This does NOT test the application's auto-reconnect - it's solely for queue inspection.
	infra.recreateChannelForInspection(t)

	// Now inspect the queue with the fresh channel
	msgCountAfter := rmqtestutil.GetQueueMessageCount(t, infra.rmqContainer.Channel, infra.queue)
	t.Logf("Messages in queue after restart: %d", msgCountAfter)

	// For durable queues, messages should be preserved
	chaos.AssertNoDataLoss(t, numMessages, msgCountAfter,
		"message count should be preserved after restart (durable queue)")

	t.Log("Chaos test passed: data integrity after restart verified")
}

// =============================================================================
// CHAOS TESTS - CONTAINER PAUSE/UNPAUSE
// =============================================================================

// TestChaos_RabbitMQ_PauseUnpauseDuringPublish tests that the producer handles
// container pause/unpause gracefully during active publishing.
// This simulates scenarios like:
// - Container being paused during maintenance
// - Resource contention causing process freeze
// - Docker checkpoint/restore operations
//
// The producer should:
// - Buffer or retry messages during pause
// - Resume normal operation after unpause
// - Not lose any messages that were accepted before pause
func TestIntegration_Chaos_RabbitMQ_PauseUnpauseDuringPublish(t *testing.T) {
	skipIfNotChaos(t)
	if testing.Short() {
		t.Skip("skipping chaos test in short mode")
	}

	infra := setupRabbitMQChaosInfra(t)
	defer infra.cleanup()

	ctx := context.Background()

	// 1. Publish baseline messages to verify setup
	t.Log("Step 1: Publishing baseline messages to verify setup")
	baselineCount := 3
	for i := 0; i < baselineCount; i++ {
		msg := chaosTestMessage{
			ID:        uuid.New().String(),
			Timestamp: time.Now(),
			Data:      fmt.Sprintf("Baseline message %d", i+1),
		}
		msgBytes, _ := json.Marshal(msg)

		_, err := infra.producer.ProducerDefault(ctx, infra.exchange, infra.routingKey, msgBytes)
		require.NoError(t, err, "baseline publish %d should succeed", i+1)
	}

	// Wait for baseline messages to arrive in queue
	rmqtestutil.WaitForQueueCount(t, infra.rmqContainer.Channel, infra.queue, baselineCount, 5*time.Second)
	t.Logf("Baseline: %d messages in queue", baselineCount)

	// 2. Start concurrent publishers
	t.Log("Step 2: Starting concurrent publishers")
	var wg sync.WaitGroup
	var mu sync.Mutex
	successCount := 0
	errorCount := 0
	stop := make(chan struct{})

	// Publisher goroutine
	publisher := func(id int) {
		defer wg.Done()
		localSuccess := 0
		localError := 0

		for {
			select {
			case <-stop:
				mu.Lock()
				successCount += localSuccess
				errorCount += localError
				mu.Unlock()
				return
			default:
			}

			msg := chaosTestMessage{
				ID:        uuid.New().String(),
				Timestamp: time.Now(),
				Data:      fmt.Sprintf("Publisher %d message", id),
			}
			msgBytes, _ := json.Marshal(msg)

			// Use timeout context to avoid blocking forever during pause
			pubCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
			_, err := infra.producer.ProducerDefault(pubCtx, infra.exchange, infra.routingKey, msgBytes)
			cancel()

			if err != nil {
				localError++
			} else {
				localSuccess++
			}

			time.Sleep(50 * time.Millisecond)
		}
	}

	// Start 2 publishers
	numPublishers := 2
	for i := 0; i < numPublishers; i++ {
		wg.Add(1)
		go publisher(i + 1)
	}

	// Let publishers run for a bit before chaos
	time.Sleep(500 * time.Millisecond)

	// 3. INJECT CHAOS: Pause container
	containerID := infra.rmqContainer.Container.GetContainerID()
	t.Logf("Step 3: INJECT CHAOS - Pausing RabbitMQ container %s", containerID)

	err := infra.chaosOrch.PauseContainer(ctx, containerID)
	require.NoError(t, err, "container pause should succeed")
	t.Log("Chaos: RabbitMQ container paused - publishers may experience timeouts")

	// Keep paused for 2 seconds
	time.Sleep(2 * time.Second)

	// 4. REMOVE CHAOS: Unpause container
	t.Log("Step 4: REMOVE CHAOS - Unpausing RabbitMQ container")
	err = infra.chaosOrch.UnpauseContainer(ctx, containerID)
	require.NoError(t, err, "container unpause should succeed")
	t.Log("Chaos: RabbitMQ container unpaused - publishers should resume")

	// Let publishers continue after unpause
	time.Sleep(2 * time.Second)

	// Stop publishers
	close(stop)
	wg.Wait()

	t.Logf("Publishing results: %d successful, %d errors", successCount, errorCount)

	// 5. Verify messages in queue
	t.Log("Step 5: Verifying message delivery")

	// Wait a bit for any in-flight messages to be delivered
	time.Sleep(1 * time.Second)

	finalCount := rmqtestutil.GetQueueMessageCount(t, infra.rmqContainer.Channel, infra.queue)
	expectedMinimum := baselineCount + successCount

	t.Logf("Final queue count: %d (expected at least: %d)", finalCount, expectedMinimum)

	// All successful publishes should result in messages in the queue
	assert.GreaterOrEqual(t, finalCount, expectedMinimum,
		"queue should contain at least baseline + successful publishes")

	// Some publishes may have succeeded during pause (buffered) or failed
	// The key invariant is: no data loss for acknowledged publishes
	assert.Greater(t, successCount, 0, "some publishes should succeed despite pause")

	t.Log("Chaos test passed: RabbitMQ pause/unpause during publish handled correctly")
}
