//go:build integration

package rabbitmq

import (
	"context"
	"fmt"
	"net/http"
	"testing"
	"time"

	testutils "github.com/LerianStudio/midaz/v3/tests/utils"

	"github.com/docker/docker/api/types/container"

	amqp "github.com/rabbitmq/amqp091-go"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

const (
	// DefaultUser is the default RabbitMQ user for test containers.
	DefaultUser = "test"
	// DefaultPassword is the default RabbitMQ password for test containers.
	DefaultPassword = "test"
)

// ContainerConfig holds configuration for RabbitMQ test container.
type ContainerConfig struct {
	User     string
	Password string
	Image    string
	MemoryMB int64   // Memory limit in MB (0 = no limit)
	CPULimit float64 // CPU limit in cores (0 = no limit)
}

// DefaultContainerConfig returns the default container configuration.
func DefaultContainerConfig() ContainerConfig {
	return ContainerConfig{
		User:     DefaultUser,
		Password: DefaultPassword,
		Image:    "rabbitmq:4.1-management-alpine",
		MemoryMB: 256, // 256MB - moderate for messaging
		CPULimit: 0.5, // 0.5 CPU core
	}
}

// ContainerResult holds the result of starting a RabbitMQ container.
type ContainerResult struct {
	Container testcontainers.Container
	Conn      *amqp.Connection
	Channel   *amqp.Channel
	Host      string
	AMQPPort  string
	MgmtPort  string
	URI       string
}

// SetupContainer starts a RabbitMQ container for integration testing.
// Returns connection and channel for queue operations.
func SetupContainer(t *testing.T) *ContainerResult {
	t.Helper()
	return SetupContainerWithConfig(t, DefaultContainerConfig())
}

// SetupContainerWithConfig starts a RabbitMQ container with custom configuration.
func SetupContainerWithConfig(t *testing.T, cfg ContainerConfig) *ContainerResult {
	t.Helper()

	ctx := context.Background()

	req := testcontainers.ContainerRequest{
		Image:        cfg.Image,
		ExposedPorts: []string{"5672/tcp", "15672/tcp"},
		Env: map[string]string{
			"RABBITMQ_DEFAULT_USER": cfg.User,
			"RABBITMQ_DEFAULT_PASS": cfg.Password,
		},
		WaitingFor: wait.ForAll(
			wait.ForLog("Server startup complete").WithStartupTimeout(120*time.Second),
			wait.ForHTTP("/api/health/checks/alarms").
				WithPort("15672/tcp").
				WithBasicAuth(cfg.User, cfg.Password).
				WithStartupTimeout(60*time.Second),
		),
		HostConfigModifier: func(hc *container.HostConfig) {
			testutils.ApplyResourceLimits(hc, cfg.MemoryMB, cfg.CPULimit)
		},
	}

	ctr, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: req,
		Started:          true,
	})
	require.NoError(t, err, "failed to start RabbitMQ container")

	host, err := ctr.Host(ctx)
	require.NoError(t, err, "failed to get RabbitMQ container host")

	amqpPort, err := ctr.MappedPort(ctx, "5672")
	require.NoError(t, err, "failed to get RabbitMQ AMQP port")

	mgmtPort, err := ctr.MappedPort(ctx, "15672")
	require.NoError(t, err, "failed to get RabbitMQ management port")

	uri := fmt.Sprintf("amqp://%s:%s@%s:%s/", cfg.User, cfg.Password, host, amqpPort.Port())

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

		if err := ctr.Terminate(context.Background()); err != nil {
			t.Logf("failed to terminate RabbitMQ container: %v", err)
		}
	})

	return &ContainerResult{
		Container: ctr,
		Conn:      conn,
		Channel:   ch,
		Host:      host,
		AMQPPort:  amqpPort.Port(),
		MgmtPort:  mgmtPort.Port(),
		URI:       uri,
	}
}

// SetupContainerOnNetwork starts a RabbitMQ container on a specific Docker network.
// The networkAlias is the hostname by which other containers on the network can reach this container.
// This is useful for chaos testing with Toxiproxy where containers need to communicate directly.
func SetupContainerOnNetwork(t *testing.T, networkName string, networkAlias string) *ContainerResult {
	t.Helper()
	return SetupContainerOnNetworkWithConfig(t, DefaultContainerConfig(), networkName, networkAlias)
}

// SetupContainerOnNetworkWithConfig starts a RabbitMQ container on a specific Docker network with custom configuration.
func SetupContainerOnNetworkWithConfig(t *testing.T, cfg ContainerConfig, networkName string, networkAlias string) *ContainerResult {
	t.Helper()

	ctx := context.Background()

	req := testcontainers.ContainerRequest{
		Image:        cfg.Image,
		ExposedPorts: []string{"5672/tcp", "15672/tcp"},
		Env: map[string]string{
			"RABBITMQ_DEFAULT_USER": cfg.User,
			"RABBITMQ_DEFAULT_PASS": cfg.Password,
		},
		Networks:       []string{networkName},
		NetworkAliases: map[string][]string{networkName: {networkAlias}},
		WaitingFor: wait.ForAll(
			wait.ForLog("Server startup complete").WithStartupTimeout(120*time.Second),
			wait.ForHTTP("/api/health/checks/alarms").
				WithPort("15672/tcp").
				WithBasicAuth(cfg.User, cfg.Password).
				WithStartupTimeout(60*time.Second),
		),
		HostConfigModifier: func(hc *container.HostConfig) {
			testutils.ApplyResourceLimits(hc, cfg.MemoryMB, cfg.CPULimit)
		},
	}

	rmqContainer, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: req,
		Started:          true,
	})
	require.NoError(t, err, "failed to start RabbitMQ container on network %s", networkName)

	host, err := rmqContainer.Host(ctx)
	require.NoError(t, err, "failed to get RabbitMQ container host")

	amqpPort, err := rmqContainer.MappedPort(ctx, "5672")
	require.NoError(t, err, "failed to get RabbitMQ AMQP port")

	mgmtPort, err := rmqContainer.MappedPort(ctx, "15672")
	require.NoError(t, err, "failed to get RabbitMQ management port")

	uri := fmt.Sprintf("amqp://%s:%s@%s:%s/", cfg.User, cfg.Password, host, amqpPort.Port())

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

		if err := rmqContainer.Terminate(context.Background()); err != nil {
			t.Logf("failed to terminate RabbitMQ container: %v", err)
		}
	})

	return &ContainerResult{
		Container: rmqContainer,
		Conn:      conn,
		Channel:   ch,
		Host:      host,
		AMQPPort:  amqpPort.Port(),
		MgmtPort:  mgmtPort.Port(),
		URI:       uri,
	}
}

// SetupExchange declares an exchange on the RabbitMQ container.
func SetupExchange(t *testing.T, ch *amqp.Channel, name, kind string) {
	t.Helper()

	err := ch.ExchangeDeclare(
		name,  // name
		kind,  // type (direct, fanout, topic, headers)
		true,  // durable
		false, // auto-deleted
		false, // internal
		false, // no-wait
		nil,   // arguments
	)
	require.NoError(t, err, "failed to declare exchange %s", name)
}

// SetupQueue declares a queue and binds it to an exchange.
func SetupQueue(t *testing.T, ch *amqp.Channel, queueName, exchangeName, routingKey string) {
	t.Helper()

	q, err := ch.QueueDeclare(
		queueName, // name
		true,      // durable
		false,     // delete when unused
		false,     // exclusive
		false,     // no-wait
		nil,       // arguments
	)
	require.NoError(t, err, "failed to declare queue %s", queueName)

	err = ch.QueueBind(
		q.Name,       // queue name
		routingKey,   // routing key
		exchangeName, // exchange
		false,        // no-wait
		nil,          // arguments
	)
	require.NoError(t, err, "failed to bind queue %s to exchange %s", queueName, exchangeName)
}

// GetQueueMessageCount returns the current message count in a queue.
// Useful for waiting until queue is empty after processing.
func GetQueueMessageCount(t *testing.T, ch *amqp.Channel, queueName string) int {
	t.Helper()

	q, err := ch.QueueDeclarePassive(queueName, false, false, false, false, nil)
	require.NoError(t, err, "failed to inspect queue %s", queueName)

	return q.Messages
}

// WaitForQueueEmpty waits until the queue has no messages or timeout expires.
func WaitForQueueEmpty(t *testing.T, ch *amqp.Channel, queueName string, timeout time.Duration) {
	t.Helper()

	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		count := GetQueueMessageCount(t, ch, queueName)
		if count == 0 {
			return
		}

		time.Sleep(100 * time.Millisecond)
	}

	t.Fatalf("timeout waiting for queue %s to be empty", queueName)
}

// WaitForQueueCount waits until the queue has exactly the expected number of messages or timeout expires.
func WaitForQueueCount(t *testing.T, ch *amqp.Channel, queueName string, expected int, timeout time.Duration) {
	t.Helper()

	deadline := time.Now().Add(timeout)

	var lastCount int
	for time.Now().Before(deadline) {
		lastCount = GetQueueMessageCount(t, ch, queueName)
		if lastCount == expected {
			return
		}

		time.Sleep(100 * time.Millisecond)
	}

	t.Fatalf("timeout waiting for queue %s to have %d messages (last count: %d)", queueName, expected, lastCount)
}

// BuildURI builds a RabbitMQ URI from host, port and config.
func BuildURI(host, port string, cfg ContainerConfig) string {
	return fmt.Sprintf("amqp://%s:%s@%s:%s/", cfg.User, cfg.Password, host, port)
}

// CreateChannel creates a new AMQP channel from a connection URI.
// Useful for creating fresh channels after container restart when the port may have changed.
// The caller is responsible for closing the returned channel and connection.
func CreateChannel(t *testing.T, uri string) *amqp.Channel {
	t.Helper()

	conn, err := amqp.Dial(uri)
	require.NoError(t, err, "failed to connect to RabbitMQ at %s", uri)

	ch, err := conn.Channel()
	require.NoError(t, err, "failed to open RabbitMQ channel")

	t.Cleanup(func() {
		if ch != nil {
			ch.Close()
		}

		if conn != nil {
			conn.Close()
		}
	})

	return ch
}

// CreateChannelWithRetry creates a new AMQP channel with retry logic.
// Useful after container restart when RabbitMQ may still be initializing.
// Polls until connection succeeds or timeout expires.
func CreateChannelWithRetry(t *testing.T, uri string, timeout time.Duration) *amqp.Channel {
	t.Helper()

	deadline := time.Now().Add(timeout)

	var lastErr error

	for time.Now().Before(deadline) {
		conn, err := amqp.Dial(uri)
		if err != nil {
			lastErr = err

			time.Sleep(500 * time.Millisecond)

			continue
		}

		ch, err := conn.Channel()
		if err != nil {
			conn.Close()

			lastErr = err

			time.Sleep(500 * time.Millisecond)

			continue
		}

		t.Cleanup(func() {
			if ch != nil {
				ch.Close()
			}

			if conn != nil {
				conn.Close()
			}
		})

		t.Log("Successfully connected to RabbitMQ after retry")

		return ch
	}

	require.NoError(t, lastErr, "failed to connect to RabbitMQ at %s after %v", uri, timeout)

	return nil // unreachable, require.NoError fails the test
}

// IsRabbitMQHealthy checks if RabbitMQ management API is responding.
// Returns true if the health check endpoint responds with HTTP 200.
func IsRabbitMQHealthy(host, mgmtPort string) bool {
	url := fmt.Sprintf("http://%s:%s@%s:%s/api/health/checks/alarms", DefaultUser, DefaultPassword, host, mgmtPort)

	client := &http.Client{Timeout: 2 * time.Second}

	resp, err := client.Get(url)
	if err != nil {
		return false
	}
	defer resp.Body.Close()

	return resp.StatusCode == http.StatusOK
}
