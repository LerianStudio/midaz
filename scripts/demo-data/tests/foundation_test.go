package tests

import (
	"context"
	"testing"

	"demo-data/internal/domain/entities"
	"demo-data/internal/domain/ports"
	"demo-data/internal/infrastructure/di"
)

// TestPackageImports validates that all foundation packages can be imported
func TestPackageImports(t *testing.T) {
	t.Run("domain entities can be imported", func(t *testing.T) {
		_ = entities.ErrEntityNotFound
		_ = entities.Status{}
	})

	t.Run("domain ports can be imported", func(t *testing.T) {
		var _ ports.ConfigurationPort = nil
		_ = ports.Configuration{}
	})

	t.Run("dependency injection container works", func(t *testing.T) {
		container := di.NewContainer()
		if container == nil {
			t.Error("container should not be nil")
		}
	})
}

// TestDomainErrors validates domain error definitions
func TestDomainErrors(t *testing.T) {
	tests := []struct {
		name string
		err  error
	}{
		{"EntityNotFound", entities.ErrEntityNotFound},
		{"ValidationFailed", entities.ErrValidationFailed},
		{"AuthenticationFailed", entities.ErrAuthenticationFailed},
		{"RateLimitExceeded", entities.ErrRateLimitExceeded},
		{"ConfigurationInvalid", entities.ErrConfigurationInvalid},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.err == nil {
				t.Errorf("%s error should not be nil", tt.name)
			}
			if tt.err.Error() == "" {
				t.Errorf("%s error message should not be empty", tt.name)
			}
		})
	}
}

// TestContainerDependencyInjection tests the dependency injection container
func TestContainerDependencyInjection(t *testing.T) {
	container := di.NewContainer()

	// Test that we can set and get configuration port
	t.Run("configuration port injection", func(t *testing.T) {
		// Initially should be nil
		configPort := container.GetConfigurationPort()
		if configPort != nil {
			t.Error("configuration port should be nil initially")
		}

		// Mock configuration port for testing
		mockConfigPort := &mockConfigurationPort{}
		container.SetConfigurationPort(mockConfigPort)

		// Should now return the mock
		retrievedPort := container.GetConfigurationPort()
		if retrievedPort == nil {
			t.Error("configuration port should not be nil after setting")
		}
		if retrievedPort != mockConfigPort {
			t.Error("retrieved configuration port should be the same as the one set")
		}
	})
}

// mockConfigurationPort is a mock implementation for testing
type mockConfigurationPort struct{}

func (m *mockConfigurationPort) Load(ctx context.Context) (*ports.Configuration, error) {
	return nil, nil
}

func (m *mockConfigurationPort) Validate(ctx context.Context, config *ports.Configuration) error {
	return nil
}

func (m *mockConfigurationPort) GetAPIEndpoints() []string {
	return []string{"/test"}
}
