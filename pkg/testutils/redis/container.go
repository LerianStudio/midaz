//go:build integration

package redis

import (
	"context"
	"testing"
	"time"

	libRedis "github.com/LerianStudio/lib-commons/v2/commons/redis"
	libZap "github.com/LerianStudio/lib-commons/v2/commons/zap"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

// ContainerConfig holds configuration for Redis test container.
type ContainerConfig struct {
	Image string
}

// DefaultContainerConfig returns the default container configuration.
func DefaultContainerConfig() ContainerConfig {
	return ContainerConfig{
		Image: "valkey/valkey:8",
	}
}

// ContainerResult holds the result of starting a Redis container.
type ContainerResult struct {
	Client  *redis.Client
	Addr    string
	Cleanup func()
}

// SetupContainer starts a Redis container for integration testing.
// Returns client and connection info.
func SetupContainer(t *testing.T) *ContainerResult {
	return SetupContainerWithConfig(t, DefaultContainerConfig())
}

// SetupContainerWithConfig starts a Redis container with custom configuration.
func SetupContainerWithConfig(t *testing.T, cfg ContainerConfig) *ContainerResult {
	t.Helper()

	ctx := context.Background()

	req := testcontainers.ContainerRequest{
		Image:        cfg.Image,
		ExposedPorts: []string{"6379/tcp"},
		WaitingFor:   wait.ForLog("Ready to accept connections").WithStartupTimeout(60 * time.Second),
	}

	container, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: req,
		Started:          true,
	})
	require.NoError(t, err, "failed to start Redis container")

	host, err := container.Host(ctx)
	require.NoError(t, err, "failed to get Redis container host")

	port, err := container.MappedPort(ctx, "6379")
	require.NoError(t, err, "failed to get Redis container port")

	addr := host + ":" + port.Port()

	client := redis.NewClient(&redis.Options{
		Addr: addr,
	})

	// Verify connection
	_, err = client.Ping(ctx).Result()
	require.NoError(t, err, "failed to ping Redis container")

	cleanup := func() {
		client.Close()
		if err := container.Terminate(ctx); err != nil {
			t.Logf("failed to terminate Redis container: %v", err)
		}
	}

	return &ContainerResult{
		Client:  client,
		Addr:    addr,
		Cleanup: cleanup,
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
