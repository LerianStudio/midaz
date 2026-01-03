//go:build chaos

package rabbitmq

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"sync"
	"testing"
	"time"

	libRabbitmq "github.com/LerianStudio/lib-commons/v2/commons/rabbitmq"
	libZap "github.com/LerianStudio/lib-commons/v2/commons/zap"
	"github.com/LerianStudio/midaz/v3/pkg/testutils/chaos"
	rmqtestutil "github.com/LerianStudio/midaz/v3/pkg/testutils/rabbitmq"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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
// Uses a shared Docker network for proper container-to-container communication.
type networkChaosTestInfra struct {
	rmqContainer   *rmqtestutil.ContainerResult
	toxiproxy      *chaos.ToxiproxyWithProxyResult
	chaosOrch      *chaos.Orchestrator
	proxyProducer  *ProducerRabbitMQRepository
	proxyConn      *libRabbitmq.RabbitMQConnection
	exchange       string
	routingKey     string
	queue          string
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
// This creates both RabbitMQ and Toxiproxy on a shared Docker network, enabling proper
// container-to-container communication through the proxy.
func setupRabbitMQNetworkChaosInfra(t *testing.T) *networkChaosTestInfra {
	t.Helper()

	// Constants for network configuration
	const (
		rabbitmqAlias = "rabbitmq"
		proxyName     = "rabbitmq-proxy"
	)

	// Setup exchange and queue names
	exchange := "test-exchange"
	routingKey := "test.routing.key"
	queue := "test-queue"

	// 1. Create Toxiproxy with a pre-configured proxy for RabbitMQ
	// The upstream uses the network alias (rabbitmq:5672) since both containers
	// will be on the same Docker network
	toxiproxy := chaos.SetupToxiproxyWithProxy(t, chaos.ProxyConfig{
		Name:     proxyName,
		Upstream: fmt.Sprintf("%s:5672", rabbitmqAlias),
	})

	// 2. Create RabbitMQ container on the same network
	rmqContainer := rmqtestutil.SetupContainerOnNetwork(t, toxiproxy.NetworkName(), rabbitmqAlias)

	// 3. Setup exchange and queue
	rmqtestutil.SetupExchange(t, rmqContainer.Channel, exchange, "topic")
	rmqtestutil.SetupQueue(t, rmqContainer.Channel, queue, exchange, routingKey)

	// 4. Create chaos orchestrator with Toxiproxy client
	chaosOrch := chaos.NewOrchestratorWithConfig(t, chaos.OrchestratorConfig{
		ToxiproxyAddr: fmt.Sprintf("http://%s:%s", toxiproxy.Host, toxiproxy.APIPort),
	})

	// 5. Create producer that connects through the proxy
	// The proxy endpoint is accessible from the test (host machine)
	proxyURI := fmt.Sprintf("amqp://%s:%s@%s:%s/",
		rmqtestutil.DefaultUser,
		rmqtestutil.DefaultPassword,
		toxiproxy.ProxyHost,
		toxiproxy.ProxyPort,
	)

	logger := libZap.InitializeLogger()
	healthCheckURL := "http://" + rmqContainer.Host + ":" + rmqContainer.MgmtPort
	proxyConn := &libRabbitmq.RabbitMQConnection{
		ConnectionStringSource: proxyURI,
		HealthCheckURL:         healthCheckURL,
		Host:                   toxiproxy.ProxyHost,
		Port:                   toxiproxy.ProxyPort,
		User:                   rmqtestutil.DefaultUser,
		Pass:                   rmqtestutil.DefaultPassword,
		Logger:                 logger,
	}
	proxyProducer := NewProducerRabbitMQ(proxyConn)

	return &networkChaosTestInfra{
		rmqContainer:  rmqContainer,
		toxiproxy:     toxiproxy,
		chaosOrch:     chaosOrch,
		proxyProducer: proxyProducer,
		proxyConn:     proxyConn,
		exchange:      exchange,
		routingKey:    routingKey,
		queue:         queue,
	}
}

// cleanup releases all resources for network chaos infrastructure.
func (infra *networkChaosTestInfra) cleanup() {
	if infra.rmqContainer != nil {
		infra.rmqContainer.Cleanup()
	}
	if infra.toxiproxy != nil {
		infra.toxiproxy.Cleanup()
	}
}

// cleanup releases all resources.
func (infra *chaosTestInfra) cleanup() {
	if infra.chaosOrch != nil {
		infra.chaosOrch.Close()
	}
	if infra.toxiproxy != nil {
		infra.toxiproxy.Cleanup()
	}
	if infra.rmqContainer != nil {
		infra.rmqContainer.Cleanup()
	}
}

// chaosTestMessage represents a test message for chaos testing.
type chaosTestMessage struct {
	ID        string    `json:"id"`
	Timestamp time.Time `json:"timestamp"`
	Data      string    `json:"data"`
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

// TestChaos_RabbitMQ_NetworkLatency tests producer behavior under network latency.
// Uses Toxiproxy to inject latency into the network path.
// NOTE: The producer uses async publishing, so we verify message DELIVERY rather than
// measuring publish call duration (which returns before network I/O completes).
func TestChaos_RabbitMQ_NetworkLatency(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping chaos test in short mode")
	}

	infra := setupRabbitMQNetworkChaosInfra(t)
	defer infra.cleanup()

	ctx := context.Background()

	// Get the pre-configured proxy from the chaos orchestrator
	proxy, err := infra.chaosOrch.GetProxy("rabbitmq-proxy")
	require.NoError(t, err, "should get pre-configured proxy")
	t.Logf("Using Toxiproxy proxy: %s -> %s", proxy.Listen(), proxy.Upstream())

	// 1. Verify normal operation through proxy - message should be delivered
	msg1 := chaosTestMessage{
		ID:        uuid.New().String(),
		Timestamp: time.Now(),
		Data:      "Message before latency",
	}
	msgBytes1, _ := json.Marshal(msg1)

	_, err = infra.proxyProducer.ProducerDefault(ctx, infra.exchange, infra.routingKey, msgBytes1)
	require.NoError(t, err, "initial publish through proxy should succeed")

	// Wait for message to be delivered and verify it arrived
	rmqtestutil.WaitForQueueCount(t, infra.rmqContainer.Channel, infra.queue, 1, 5*time.Second)
	t.Log("Message delivered to queue (count: 1)")

	// 2. INJECT CHAOS: Add 500ms latency with 100ms jitter
	t.Log("Chaos: Adding 500ms latency to RabbitMQ connection")
	err = proxy.AddLatency(500*time.Millisecond, 100*time.Millisecond)
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
	err = proxy.RemoveAllToxics()
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
func TestChaos_RabbitMQ_RetryDuringNetworkOutage(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping chaos test in short mode")
	}

	infra := setupRabbitMQNetworkChaosInfra(t)
	defer infra.cleanup()

	ctx := context.Background()

	// Get the pre-configured proxy from the chaos orchestrator
	proxy, err := infra.chaosOrch.GetProxy("rabbitmq-proxy")
	require.NoError(t, err, "should get pre-configured proxy")
	t.Logf("Using Toxiproxy proxy: %s -> %s", proxy.Listen(), proxy.Upstream())

	// 1. Publish baseline message to verify infrastructure is working
	t.Log("Step 1: Publishing baseline message to verify setup")
	baselineMsg := chaosTestMessage{
		ID:        uuid.New().String(),
		Timestamp: time.Now(),
		Data:      "Baseline message before network outage",
	}
	baselineMsgBytes, _ := json.Marshal(baselineMsg)

	_, err = infra.proxyProducer.ProducerDefault(ctx, infra.exchange, infra.routingKey, baselineMsgBytes)
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
	err = proxy.Disconnect()
	require.NoError(t, err, "proxy disconnect should succeed")
	t.Log("Chaos: Network outage injected - proxy disconnected")

	// 5. Wait ~8 seconds for 3-4 retry attempts (producer uses exponential backoff)
	// This is enough time for retries but not enough to exhaust all attempts
	t.Log("Step 5: Waiting 8 seconds for retry attempts during outage")
	time.Sleep(8 * time.Second)
	t.Log("Chaos: Retry period elapsed - producer should have attempted multiple retries")

	// 6. REMOVE CHAOS: Reconnect proxy to restore network
	t.Log("Step 6: REMOVE CHAOS - Reconnecting proxy to restore network")
	err = proxy.Reconnect()
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

// TestChaos_RabbitMQ_ConnectionKilledByBroker tests producer recovery when the broker
// forcefully closes all connections. This simulates real-world scenarios like:
// - RabbitMQ memory/disk alarm triggered
// - Broker restart during deployment
// - Admin manually closing problematic connections
//
// The producer's EnsureChannel should detect the closed connection and reconnect.
func TestChaos_RabbitMQ_ConnectionKilledByBroker(t *testing.T) {
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

// TestChaos_RabbitMQ_DataIntegrityAfterRestart tests that no messages are lost
// during container restart (for durable queues).
func TestChaos_RabbitMQ_DataIntegrityAfterRestart(t *testing.T) {
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
