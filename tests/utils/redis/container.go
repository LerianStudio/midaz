//go:build integration || chaos

package redis

import (
	"context"
	"testing"
	"time"

	"github.com/LerianStudio/midaz/v3/tests/utils"

	"github.com/docker/docker/api/types/container"

	libRedis "github.com/LerianStudio/lib-commons/v2/commons/redis"
	libZap "github.com/LerianStudio/lib-commons/v2/commons/zap"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

// ContainerConfig holds configuration for Redis test container.
type ContainerConfig struct {
	Image    string
	MemoryMB int64   // Memory limit in MB (0 = no limit)
	CPULimit float64 // CPU limit in cores (0 = no limit)
}

// DefaultContainerConfig returns the default container configuration.
func DefaultContainerConfig() ContainerConfig {
	return ContainerConfig{
		Image:    "valkey/valkey:8",
		MemoryMB: 128, // 128MB - lightweight in-memory store
		CPULimit: 0.5, // 0.5 CPU core
	}
}

// ContainerResult holds the result of starting a Redis container.
type ContainerResult struct {
	Container testcontainers.Container
	Client    *redis.Client
	Addr      string
}

// SetupContainer starts a Redis container for integration testing.
// Returns client and connection info.
func SetupContainer(t *testing.T) *ContainerResult {
	t.Helper()
	return SetupContainerWithConfig(t, DefaultContainerConfig())
}

// SetupContainerWithConfig starts a Redis container with custom configuration.
func SetupContainerWithConfig(t *testing.T, cfg ContainerConfig) *ContainerResult {
	t.Helper()

	ctx := context.Background()

	req := testcontainers.ContainerRequest{
		Image:        cfg.Image,
		ExposedPorts: []string{"6379/tcp"},
		WaitingFor: wait.ForAll(
			wait.ForLog("Ready to accept connections"),
			wait.ForListeningPort("6379/tcp"),
		).WithDeadline(60 * time.Second),
		HostConfigModifier: func(hc *container.HostConfig) {
			testutils.ApplyResourceLimits(hc, cfg.MemoryMB, cfg.CPULimit)
		},
	}

	ctr, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: req,
		Started:          true,
	})
	require.NoError(t, err, "failed to start Redis container")

	host, err := ctr.Host(ctx)
	require.NoError(t, err, "failed to get Redis container host")

	port, err := ctr.MappedPort(ctx, "6379")
	require.NoError(t, err, "failed to get Redis container port")

	addr := host + ":" + port.Port()

	client := redis.NewClient(&redis.Options{
		Addr: addr,
	})

	// Verify connection
	_, err = client.Ping(ctx).Result()
	require.NoError(t, err, "failed to ping Redis container")

	t.Cleanup(func() {
		client.Close()

		if err := ctr.Terminate(context.Background()); err != nil {
			t.Logf("failed to terminate Redis container: %v", err)
		}
	})

	return &ContainerResult{
		Container: ctr,
		Client:    client,
		Addr:      addr,
	}
}

// SetupContainerOnNetwork starts a Redis container on a specific Docker network.
// The networkAlias is the hostname by which other containers on the network can reach this container.
// This is useful for chaos testing with Toxiproxy where containers need to communicate directly.
func SetupContainerOnNetwork(t *testing.T, networkName string, networkAlias string) *ContainerResult {
	t.Helper()
	return SetupContainerOnNetworkWithConfig(t, DefaultContainerConfig(), networkName, networkAlias)
}

// SetupContainerOnNetworkWithConfig starts a Redis container on a specific Docker network with custom configuration.
func SetupContainerOnNetworkWithConfig(t *testing.T, cfg ContainerConfig, networkName string, networkAlias string) *ContainerResult {
	t.Helper()

	ctx := context.Background()

	req := testcontainers.ContainerRequest{
		Image:          cfg.Image,
		ExposedPorts:   []string{"6379/tcp"},
		Networks:       []string{networkName},
		NetworkAliases: map[string][]string{networkName: {networkAlias}},
		WaitingFor: wait.ForAll(
			wait.ForLog("Ready to accept connections"),
			wait.ForListeningPort("6379/tcp"),
		).WithDeadline(60 * time.Second),
		HostConfigModifier: func(hc *container.HostConfig) {
			testutils.ApplyResourceLimits(hc, cfg.MemoryMB, cfg.CPULimit)
		},
	}

	redisContainer, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: req,
		Started:          true,
	})
	require.NoError(t, err, "failed to start Redis container on network %s", networkName)

	host, err := redisContainer.Host(ctx)
	require.NoError(t, err, "failed to get Redis container host")

	port, err := redisContainer.MappedPort(ctx, "6379")
	require.NoError(t, err, "failed to get Redis container port")

	addr := host + ":" + port.Port()

	client := redis.NewClient(&redis.Options{
		Addr: addr,
	})

	// Verify connection
	_, err = client.Ping(ctx).Result()
	require.NoError(t, err, "failed to ping Redis container")

	t.Cleanup(func() {
		client.Close()

		if err := redisContainer.Terminate(context.Background()); err != nil {
			t.Logf("failed to terminate Redis container: %v", err)
		}
	})

	return &ContainerResult{
		Client:    client,
		Addr:      addr,
		Container: redisContainer,
	}
}

// CreateConnection creates a libRedis.RedisConnection wrapper for testing
// using the provided Redis address.
func CreateConnection(t *testing.T, addr string) *libRedis.RedisConnection {
	t.Helper()

	logger := libZap.InitializeLogger()

	return &libRedis.RedisConnection{
		Address: []string{addr},
		Logger:  logger,
	}
}

// CreateConnectionWithRetry creates a libRedis.RedisConnection wrapper with retry logic.
// Useful after container restart when Redis may still be initializing.
func CreateConnectionWithRetry(t *testing.T, addr string, timeout time.Duration) *libRedis.RedisConnection {
	t.Helper()

	deadline := time.Now().Add(timeout)

	var lastErr error

	for time.Now().Before(deadline) {
		client := redis.NewClient(&redis.Options{
			Addr: addr,
		})

		_, err := client.Ping(context.Background()).Result()
		client.Close()

		if err == nil {
			t.Log("Successfully connected to Redis after retry")
			return CreateConnection(t, addr)
		}

		lastErr = err

		time.Sleep(500 * time.Millisecond)
	}

	require.NoError(t, lastErr, "failed to connect to Redis at %s after %v", addr, timeout)

	return nil
}
