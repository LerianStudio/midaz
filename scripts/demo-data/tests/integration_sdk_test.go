package tests

import (
	"context"
	"testing"
	"time"

	"demo-data/internal/adapters/secondary/config"
	"demo-data/internal/adapters/secondary/sdk"
	"demo-data/internal/domain/entities"
	"demo-data/internal/infrastructure/di"
)

// TestSDKIntegration tests the complete SDK integration workflow
func TestSDKIntegration(t *testing.T) {
	ctx := context.Background()

	t.Run("complete workflow integration", func(t *testing.T) {
		// Create container
		container := di.NewContainer()

		// Set up configuration
		configAdapter := config.NewViperConfigAdapter()
		container.SetConfigurationPort(configAdapter)

		// Load test configuration
		cfg := &entities.Configuration{
			APIBaseURL:      "https://api.test.com",
			AuthToken:       "test-token-123456",
			TimeoutDuration: 30 * time.Second,
			Debug:           true,
			LogLevel:        "debug",
		}
		container.SetConfiguration(cfg)

		// Get SDK client through container
		client, err := container.GetMidazClientPort()
		if err != nil {
			t.Fatalf("failed to get SDK client: %v", err)
		}

		// Test connection validation
		err = client.ValidateConnection(ctx)
		if err != nil {
			t.Fatalf("connection validation failed: %v", err)
		}

		// Test basic operations to ensure SDK is working
		// This validates that the client can be created and used
		if client == nil {
			t.Fatal("client should not be nil")
		}
	})

	t.Run("container caches client instance", func(t *testing.T) {
		container := di.NewContainer()

		cfg := &entities.Configuration{
			APIBaseURL:      "https://api.test.com",
			AuthToken:       "test-token-123456",
			TimeoutDuration: 30 * time.Second,
		}
		container.SetConfiguration(cfg)

		// Get client twice
		client1, err := container.GetMidazClientPort()
		if err != nil {
			t.Fatalf("failed to get first client: %v", err)
		}

		client2, err := container.GetMidazClientPort()
		if err != nil {
			t.Fatalf("failed to get second client: %v", err)
		}

		// Should be the same instance
		if client1 != client2 {
			t.Error("container should cache client instance")
		}
	})

	t.Run("fails without configuration", func(t *testing.T) {
		container := di.NewContainer()

		_, err := container.GetMidazClientPort()
		if err == nil {
			t.Error("should fail without configuration")
		}
	})

	t.Run("manual client injection", func(t *testing.T) {
		container := di.NewContainer()

		// Create mock client
		cfg := &entities.Configuration{
			APIBaseURL:      "https://api.test.com",
			AuthToken:       "test-token-123456",
			TimeoutDuration: 30 * time.Second,
		}

		mockClient, err := sdk.NewMidazClientAdapter(cfg)
		if err != nil {
			t.Fatalf("failed to create mock client: %v", err)
		}

		// Inject manually
		container.SetMidazClientPort(mockClient)

		// Should return the injected client
		client, err := container.GetMidazClientPort()
		if err != nil {
			t.Fatalf("failed to get injected client: %v", err)
		}

		if client != mockClient {
			t.Error("should return the manually injected client")
		}
	})
}
