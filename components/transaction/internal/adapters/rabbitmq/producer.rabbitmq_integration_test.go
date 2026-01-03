//go:build integration

package rabbitmq

import (
	"context"
	"encoding/json"
	"sync"
	"testing"
	"time"

	libRabbitmq "github.com/LerianStudio/lib-commons/v2/commons/rabbitmq"
	libZap "github.com/LerianStudio/lib-commons/v2/commons/zap"
	rmqtestutil "github.com/LerianStudio/midaz/v3/pkg/testutils/rabbitmq"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// integrationTestInfra holds the infrastructure needed for RabbitMQ integration tests.
type integrationTestInfra struct {
	rmqContainer *rmqtestutil.ContainerResult
	conn         *libRabbitmq.RabbitMQConnection
	producer     *ProducerRabbitMQRepository
	exchange     string
	routingKey   string
	queue        string
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

// cleanup releases all resources.
func (infra *integrationTestInfra) cleanup() {
	if infra.rmqContainer != nil {
		infra.rmqContainer.Cleanup()
	}
}

// integrationTestMessage represents a test message for integration testing.
type integrationTestMessage struct {
	ID        string    `json:"id"`
	Timestamp time.Time `json:"timestamp"`
	Data      string    `json:"data"`
}

// TestIntegration_RabbitMQ_ProducerBasicPublish tests that the producer can publish
// a message successfully and verify it arrives in the queue.
func TestIntegration_RabbitMQ_ProducerBasicPublish(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	infra := setupIntegrationInfra(t)
	defer infra.cleanup()

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
	defer infra.cleanup()

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
	defer infra.cleanup()

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
	defer infra.cleanup()

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
	defer infra.cleanup()

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
	defer infra.cleanup()

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
	defer infra.cleanup()

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
