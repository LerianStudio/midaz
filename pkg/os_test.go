package pkg

import (
	"os"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestGetenvOrDefault(t *testing.T) {
	tests := []struct {
		name          string
		key           string
		envValue      string
		defaultValue  string
		expectedValue string
	}{
		{
			name:          "Key exists with value",
			key:           "EXISTING_KEY",
			envValue:      "some_value",
			defaultValue:  "default_value",
			expectedValue: "some_value",
		},
		{
			name:          "Key does not exist",
			key:           "NON_EXISTING_KEY",
			envValue:      "",
			defaultValue:  "default_value",
			expectedValue: "default_value",
		},
		{
			name:          "Key exists with empty value",
			key:           "EMPTY_KEY",
			envValue:      "",
			defaultValue:  "default_value",
			expectedValue: "default_value",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv(tt.key, tt.envValue)
			result := GetenvOrDefault(tt.key, tt.defaultValue)
			assert.Equal(t, tt.expectedValue, result)
		})
	}
}

func TestGetenvBoolOrDefault(t *testing.T) {
	tests := []struct {
		name          string
		key           string
		envValue      string
		defaultValue  bool
		expectedValue bool
	}{
		{
			name:          "Key exists with 'true' value",
			key:           "BOOL_KEY",
			envValue:      "true",
			defaultValue:  false,
			expectedValue: true,
		},
		{
			name:          "Key exists with 'false' value",
			key:           "BOOL_KEY",
			envValue:      "false",
			defaultValue:  true,
			expectedValue: false,
		},
		{
			name:          "Key exists with invalid value",
			key:           "BOOL_KEY",
			envValue:      "not_a_bool",
			defaultValue:  true,
			expectedValue: true,
		},
		{
			name:          "Key does not exist",
			key:           "NON_EXISTING_KEY",
			envValue:      "",
			defaultValue:  true,
			expectedValue: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv(tt.key, tt.envValue)
			result := GetenvBoolOrDefault(tt.key, tt.defaultValue)
			assert.Equal(t, tt.expectedValue, result)
		})
	}
}

func TestGetenvIntOrDefault(t *testing.T) {
	tests := []struct {
		name          string
		key           string
		envValue      string
		defaultValue  int64
		expectedValue int64
	}{
		{
			name:          "Key exists with valid int value",
			key:           "INT_KEY",
			envValue:      "42",
			defaultValue:  0,
			expectedValue: 42,
		},
		{
			name:          "Key exists with invalid int value",
			key:           "INT_KEY",
			envValue:      "invalid",
			defaultValue:  10,
			expectedValue: 10,
		},
		{
			name:          "Key does not exist",
			key:           "NON_EXISTING_KEY",
			envValue:      "",
			defaultValue:  100,
			expectedValue: 100,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv(tt.key, tt.envValue)
			result := GetenvIntOrDefault(tt.key, tt.defaultValue)
			assert.Equal(t, tt.expectedValue, result)
		})
	}
}

func TestInitLocalEnvConfig_WithoutEnvFile(t *testing.T) {
	t.Run("without envfile", func(t *testing.T) {
		localEnvConfig = nil
		localEnvConfigOnce = sync.Once{}

		os.Remove(".env")

		os.Setenv("VERSION", "1.0.0")
		os.Setenv("ENV_NAME", "local")
		defer os.Unsetenv("VERSION")
		defer os.Unsetenv("ENV_NAME")

		cfg := InitLocalEnvConfig()

		if cfg == nil {
			t.Fatal("expected non-nil LocalEnvConfig, got nil")
		}

		if cfg.Initialized {
			t.Errorf("expected Initialized to be false, got true")
		}
	})

	t.Run("non local environment", func(t *testing.T) {
		localEnvConfig = nil
		localEnvConfigOnce = sync.Once{}

		os.Setenv("VERSION", "1.0.0")
		os.Setenv("ENV_NAME", "production")
		defer os.Unsetenv("VERSION")
		defer os.Unsetenv("ENV_NAME")

		cfg := InitLocalEnvConfig()

		if cfg != nil {
			t.Fatalf("expected nil LocalEnvConfig for non-local env, got %v", cfg)
		}
	})
}

func TestSetConfigFromEnvVars(t *testing.T) {
	type Config struct {
		Host     string `env:"APP_HOST"`
		Port     int    `env:"APP_PORT"`
		UseHTTPS bool   `env:"APP_USE_HTTPS"`
	}

	os.Setenv("APP_HOST", "localhost")
	os.Setenv("APP_PORT", "8080")
	os.Setenv("APP_USE_HTTPS", "true")

	defer func() {
		os.Unsetenv("APP_HOST")
		os.Unsetenv("APP_PORT")
		os.Unsetenv("APP_USE_HTTPS")
	}()

	cfg := &Config{}
	err := SetConfigFromEnvVars(cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.Host != "localhost" {
		t.Errorf("expected Host to be 'localhost', got '%s'", cfg.Host)
	}

	if cfg.Port != 8080 {
		t.Errorf("expected Port to be 8080, got '%d'", cfg.Port)
	}

	if !cfg.UseHTTPS {
		t.Errorf("expected UseHTTPS to be true, got false")
	}
}

func TestEnsureConfigFromEnvVars(t *testing.T) {
	t.Run("Valid environment variables", func(t *testing.T) {

		type Config struct {
			Host string `env:"HOST"`
			Port string `env:"PORT"`
		}

		os.Setenv("HOST", "localhost")
		os.Setenv("PORT", "8080")
		defer os.Unsetenv("HOST")
		defer os.Unsetenv("PORT")

		config := &Config{}
		result := EnsureConfigFromEnvVars(config).(*Config)

		if result.Host != "localhost" {
			t.Errorf("Expected Host to be 'localhost', got '%s'", result.Host)
		}
		if result.Port != "8080" {
			t.Errorf("Expected Port to be '8080', got '%s'", result.Port)
		}
	})

	t.Run("Missing environment variables", func(t *testing.T) {
		os.Unsetenv("HOST")
		os.Unsetenv("PORT")

		type Config struct {
			Host string    `env:"HOST"`
			Port string    `env:"PORT"`
			Time time.Time `env:"TIME"`
		}

		defer func() {
			if r := recover(); r == nil {
				t.Errorf("Expected panic but did not occur")
			}
		}()

		config := &Config{}
		EnsureConfigFromEnvVars(config)
	})
}
