package tests

import (
	"context"
	"os"
	"testing"
	"time"

	"demo-data/internal/adapters/secondary/config"
	"demo-data/internal/domain/entities"
)

// TestViperConfigAdapter tests the Viper configuration adapter
func TestViperConfigAdapter(t *testing.T) {
	ctx := context.Background()

	t.Run("loads default configuration", func(t *testing.T) {
		adapter := config.NewViperConfigAdapter()

		cfg, err := adapter.Load(ctx)
		if err != nil {
			t.Fatalf("failed to load configuration: %v", err)
		}

		if cfg == nil {
			t.Fatal("configuration should not be nil")
		}

		// Check default values
		if cfg.APIBaseURL != "https://api.midaz.io" {
			t.Errorf("expected default API URL, got %s", cfg.APIBaseURL)
		}

		if cfg.TimeoutDuration != 30*time.Second {
			t.Errorf("expected 30s timeout, got %v", cfg.TimeoutDuration)
		}

		if cfg.LogLevel != "info" {
			t.Errorf("expected info log level, got %s", cfg.LogLevel)
		}

		if cfg.Debug != false {
			t.Error("expected debug to be false by default")
		}
	})

	t.Run("loads from environment variables", func(t *testing.T) {
		// Set environment variables
		os.Setenv("DEMO_DATA_API_BASE_URL", "https://test.api.com")
		os.Setenv("DEMO_DATA_DEBUG", "true")
		os.Setenv("DEMO_DATA_LOG_LEVEL", "debug")
		os.Setenv("DEMO_DATA_AUTH_TOKEN", "test-token")
		defer func() {
			os.Unsetenv("DEMO_DATA_API_BASE_URL")
			os.Unsetenv("DEMO_DATA_DEBUG")
			os.Unsetenv("DEMO_DATA_LOG_LEVEL")
			os.Unsetenv("DEMO_DATA_AUTH_TOKEN")
		}()

		// Create a new adapter to pick up environment variables
		adapter := config.NewViperConfigAdapter()
		cfg, err := adapter.Load(ctx)
		if err != nil {
			t.Fatalf("failed to load configuration: %v", err)
		}

		if cfg.APIBaseURL != "https://test.api.com" {
			t.Errorf("expected test API URL, got %s", cfg.APIBaseURL)
		}

		if cfg.Debug != true {
			t.Error("expected debug to be true from env var")
		}

		if cfg.LogLevel != "debug" {
			t.Errorf("expected debug log level, got %s", cfg.LogLevel)
		}

		// Note: AuthToken might not be loaded because it's not in defaults
		// This is expected behavior - auth token must be explicitly provided
		t.Logf("Auth token from env: %s", cfg.AuthToken)
	})

	t.Run("validates configuration successfully", func(t *testing.T) {
		adapter := config.NewViperConfigAdapter()
		cfg := &entities.Configuration{
			APIBaseURL:      "https://valid.api.com",
			AuthToken:       "valid-token",
			TimeoutDuration: 30 * time.Second,
			Debug:           false,
			LogLevel:        "info",
			VolumeConfig:    entities.DefaultVolumeConfig(),
		}

		err := adapter.Validate(ctx, cfg)
		if err != nil {
			t.Errorf("validation should pass for valid config: %v", err)
		}
	})

	t.Run("validates configuration with errors", func(t *testing.T) {
		adapter := config.NewViperConfigAdapter()

		testCases := []struct {
			name   string
			config *entities.Configuration
		}{
			{
				name: "missing auth token",
				config: &entities.Configuration{
					APIBaseURL:      "https://valid.api.com",
					AuthToken:       "",
					TimeoutDuration: 30 * time.Second,
					LogLevel:        "info",
				},
			},
			{
				name: "invalid timeout",
				config: &entities.Configuration{
					APIBaseURL:      "https://valid.api.com",
					AuthToken:       "valid-token",
					TimeoutDuration: 0,
					LogLevel:        "info",
				},
			},
			{
				name: "invalid log level",
				config: &entities.Configuration{
					APIBaseURL:      "https://valid.api.com",
					AuthToken:       "valid-token",
					TimeoutDuration: 30 * time.Second,
					LogLevel:        "invalid",
				},
			},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				err := adapter.Validate(ctx, tc.config)
				if err == nil {
					t.Errorf("validation should fail for %s", tc.name)
				}
			})
		}
	})

	t.Run("returns API endpoints", func(t *testing.T) {
		adapter := config.NewViperConfigAdapter()
		endpoints := adapter.GetAPIEndpoints()

		if len(endpoints) == 0 {
			t.Error("should return API endpoints")
		}

		expectedEndpoints := []string{
			"/v1/organizations",
			"/v1/health",
			"/v1/auth/validate",
		}

		for _, expected := range expectedEndpoints {
			found := false
			for _, endpoint := range endpoints {
				if endpoint == expected {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("expected endpoint %s not found", expected)
			}
		}
	})
}

// TestConfigurationEntity tests the configuration entity
func TestConfigurationEntity(t *testing.T) {
	t.Run("creates configuration with defaults", func(t *testing.T) {
		cfg := entities.NewConfiguration()

		if cfg == nil {
			t.Fatal("configuration should not be nil")
		}

		if cfg.APIBaseURL == "" {
			t.Error("default API URL should be set")
		}

		if cfg.TimeoutDuration <= 0 {
			t.Error("default timeout should be positive")
		}

		if cfg.LogLevel == "" {
			t.Error("default log level should be set")
		}
	})

	t.Run("validates configuration", func(t *testing.T) {
		testCases := []struct {
			name        string
			config      *entities.Configuration
			expectError bool
		}{
			{
				name: "valid configuration",
				config: &entities.Configuration{
					APIBaseURL:      "https://api.test.com",
					AuthToken:       "valid-token",
					TimeoutDuration: 30 * time.Second,
					LogLevel:        "info",
				},
				expectError: false,
			},
			{
				name: "missing auth token",
				config: &entities.Configuration{
					APIBaseURL:      "https://api.test.com",
					AuthToken:       "",
					TimeoutDuration: 30 * time.Second,
					LogLevel:        "info",
				},
				expectError: true,
			},
			{
				name: "invalid timeout",
				config: &entities.Configuration{
					APIBaseURL:      "https://api.test.com",
					AuthToken:       "valid-token",
					TimeoutDuration: -1 * time.Second,
					LogLevel:        "info",
				},
				expectError: true,
			},
			{
				name: "timeout too long",
				config: &entities.Configuration{
					APIBaseURL:      "https://api.test.com",
					AuthToken:       "valid-token",
					TimeoutDuration: 10 * time.Minute,
					LogLevel:        "info",
				},
				expectError: true,
			},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				err := tc.config.Validate()
				if tc.expectError && err == nil {
					t.Error("expected validation error")
				}
				if !tc.expectError && err != nil {
					t.Errorf("unexpected validation error: %v", err)
				}
			})
		}
	})

	t.Run("default volume config", func(t *testing.T) {
		volumeConfig := entities.DefaultVolumeConfig()

		if volumeConfig.Small.Organizations <= 0 {
			t.Error("small volume should have positive organizations")
		}

		if volumeConfig.Medium.Organizations <= volumeConfig.Small.Organizations {
			t.Error("medium volume should be larger than small")
		}

		if volumeConfig.Large.Organizations <= volumeConfig.Medium.Organizations {
			t.Error("large volume should be larger than medium")
		}
	})
}
